package gormshared

import (
	"context"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"gorm.io/gorm"
)

// AnomalyKindStuckProcessing —— §2.3/v2.13：lease 过期超过 stuckAfter 且带 payload 的卡死行，
// 自动回收（§3.2）时升级记录的异常 kind。DDL 的 kind 列无 CHECK，无需迁移（anomaly.go 预留）。
const AnomalyKindStuckProcessing = "STUCK_PROCESSING"

// RecoveredRow 是一次自动回收带走的行明细（§3.2）。
// 签名返回行明细而非只有计数（review OV④a）：异常行由调用方（服务侧 OpsMaintenance）经
// store.RecordAnomaly 写入，metricsStore 装饰器（唯一把 anomaly 行变成
// consumption_anomaly_total 的 seam）因此可见——本函数若直接 Create(&AnomalyModel) 会绕过
// seam，STUCK_PROCESSING 就是死指标（Task 14 的断言也会红）。
type RecoveredRow struct {
	ID        int64
	EventID   string
	HandlerID reliable.HandlerID
	TenantID  int
	ClaimID   string
}

// recoverCandidateSQL 选出「租约过期超过 cutoff 且可自助重放」的候选行。
// 只投影调用方写 STUCK_PROCESSING anomaly 所需列 + 定位 id——PROCESSING 行的 payload 可能
// 很大（review #12 同源：不把 LONGBLOB/BYTEA 拉进堆，payload 只用 IS NOT NULL 判定）。
// status 写字面量（D22 纪律）：两方言稳定命中 idx_lease (status, lease_expires_at)。
const recoverCandidateSQL = `
SELECT id, event_id, handler_id, tenant_id, claim_id
FROM event_consumption
WHERE status = 'PROCESSING' AND lease_expires_at < ? AND payload IS NOT NULL`

// recoverRowSQL 逐行 CAS 回收。WHERE 保留 lease_expires_at < cutoff 复验（review OV③）：
// SELECT 与 UPDATE 之间 broker 重投可触发 tryClaimOnce 的内联续占（claim.go：租约刷新但
// status 仍 PROCESSING）——不带此守卫会砸掉在途消费者的活租约，其 MarkSucceeded 将 ErrConflict。
// error_class/error_code 用 COALESCE（终评 minors）：经 ClaimForReplay 重放过的 PROCESSING 行
// 仍带上一轮 MarkFailed 的失败元数据——无条件覆盖会让崩溃循环消费的行最终带着
// LEASE_EXPIRED_RECOVERED 占位符烧完 attempt 上限，§10 fingerprint 聚合指向症状而非真实缺陷。
// 首轮回收（NULL 元数据）才写占位值；chk_retry_due 只要求 error_class 非空，COALESCE 满足。
// 被 recovery_unit_test.go 做字符串级钉住（竞态真交错无法在无钩子下外部构造，WHERE 子句
// 即防线——见 recovery_system_test.go 的 P4 determinism 注记）。
const recoverRowSQL = `
UPDATE event_consumption
SET status = 'RETRY_SCHEDULED', next_attempt_at = ?, claim_id = NULL, claimed_at = NULL,
    lease_expires_at = NULL, row_version = row_version + 1, updated_at = ?,
    error_class = COALESCE(error_class, 'RETRYABLE'), error_code = COALESCE(error_code, 'LEASE_EXPIRED_RECOVERED')
WHERE id = ? AND status = 'PROCESSING' AND lease_expires_at < ?`

// RecoverExpiredProcessing 自动化 §3.2（PR-7 ③，v1.1.68 D20 延迟项）：
// 把「租约已过期超过 stuckAfter 且可自助重放」的 PROCESSING 行转回 RETRY_SCHEDULED。
// payload IS NULL 的行交给 broker 重投（§2.1），只计数不迁移。
// ⚠ 指标路径（review OV④a）：本函数【不直接写异常行】——异常行经返回的行信息由
// 调用方（服务侧 OpsMaintenance）通过 store.RecordAnomaly(kind=AnomalyKindStuckProcessing)
// 写入，使 metricsStore 装饰器可见（见 RecoveredRow 注释）。
func RecoverExpiredProcessing(ctx context.Context, db *gorm.DB, now time.Time, stuckAfter time.Duration) (recovered []RecoveredRow, stuck int, err error) {
	cutoff := now.Add(-stuckAfter)
	// 候选行：expired + payload；异常行【不在此写】（OV④a——见函数头注释）
	var ids []RecoveredRow
	if err := db.WithContext(ctx).Raw(recoverCandidateSQL, cutoff).Scan(&ids).Error; err != nil {
		return nil, 0, err
	}
	out := make([]RecoveredRow, 0, len(ids))
	for _, r := range ids {
		// error_class/error_code 是 chk_retry_due（migration.go 双方言）要求的占位值：
		// RETRY_SCHEDULED 必有 payload（SELECT 已过滤）+ next_attempt_at + error_class。
		res := db.WithContext(ctx).Exec(recoverRowSQL, now, now, r.ID, cutoff)
		if res.Error != nil {
			return out, stuck, res.Error
		}
		if res.RowsAffected == 0 {
			continue // 并发续占/回收竞态：他人已处理（OV③ 守卫拦截），不覆盖
		}
		out = append(out, r)
	}
	// stuck 计数在回收循环之后跑（brief 定序）：只数「交给 broker 重投」的 payload-NULL 行。
	var n int64
	if err := db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM event_consumption WHERE status='PROCESSING' AND lease_expires_at < ? AND payload IS NULL`,
		cutoff).Scan(&n).Error; err != nil {
		return out, 0, err
	}
	return out, int(n), nil
}

// DeleteSettledBefore 保留策略清理（§10）：SUCCEEDED 按 succeededBefore、DISCARDED 按
// discardedBefore。DEAD_LETTER 永不自动清理；RETRY_SCHEDULED/PROCESSING 不在清理范围。
// 索引（终评 #4）：MySQL idx_retention(status,updated_at) / PG 两条 partial
// idx_retention_*（updated_at WHERE status=...）——无索引则每次清理全表扫；单条无界
// DELETE 的批量化（MVCC 膨胀/复制延迟）留待服务侧 sweep 按需分批，本内核函数语义保持简单。
// ⚠ 幂等台账语义：本删除会移除已终结消费行——被删事件若被 broker 重投，TryClaim 视为
// 首见并重新执行（标准 at-least-once 姿态）；依赖消费行做幂等去重的业务须在保留窗外。
func DeleteSettledBefore(ctx context.Context, db *gorm.DB, succeededBefore, discardedBefore time.Time) (int64, error) {
	res := db.WithContext(ctx).Exec(`
DELETE FROM event_consumption
WHERE (status = 'SUCCEEDED' AND updated_at < ?) OR (status = 'DISCARDED' AND updated_at < ?)`,
		succeededBefore, discardedBefore)
	return res.RowsAffected, res.Error
}
