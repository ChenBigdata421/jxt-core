package gormshared

import (
	"context"
	"errors"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// —— FindEligibleHeads（§6.2.1；D5：删死分支 + 无聚合行跳过分组）——

// EligibleHeadsSQL 是 eligible-head 查询（§6.2.1）。无聚合行（通知类）跳过 NOT EXISTS，自由并行；
// 有聚合行才排除「同聚合存在更早未解决行」。
//
// **包级导出常量（D22 本轮评审）**：repotest 的 EXPLAIN 门禁直接引用同一个字符串，零复制、零漂移
// ——若把 SQL 复制一份进测试，门禁很快就会变成在测一个没人跑的查询。
//
// D22（本轮评审）：status 写**字面量**而非参数。PG 的 idx_due 是 partial index
// （`WHERE status = 'RETRY_SCHEDULED'`），参数化 `status = $1` 在 generic plan 下无法蕴含该谓词
// → 索引失效退化 Seq Scan，且是「执行 5 次后才切 generic plan」的间歇性退化。本查询只查这一个
// 状态，写死字面量既消除歧义又让两方言都稳定命中索引（EXPLAIN 门禁才可能是确定性的）。
//
// A4（本轮评审）：`FOR UPDATE SKIP LOCKED` 在 autocommit 的 claimDB 上，行锁只存活于本语句期间，
// 语句一结束即释放。**它不是跨 claim 的持有**，只是并发扫描者之间的 best-effort 去重
// （同一瞬间两个 scheduler 各取不同批次）。真正防止重复重放的是 ClaimForReplay 的 CAS
// （WHERE status='RETRY_SCHEDULED' AND ...）——不要误以为这里的锁提供了正确性保证。
//
// review #2：SELECT 投影收窄为 scheduler 入场前决策所需列（id/tenant/handler/aggregate/mode）——
// payload 等大列由 ClaimForReplay 竞得后的 First(&m,id) 懒加载（scheduler.processOne 改用 claimed.* 调 handler），
// 避免每个 tick 把整批 RETRY_SCHEDULED 行的 LONGBLOB/BYTEA 拉进堆。
const EligibleHeadsSQL = `
SELECT c.id, c.tenant_id, c.event_id, c.item_key, c.handler_id, c.aggregate_type, c.aggregate_id, c.replay_mode
FROM event_consumption c
WHERE c.status = 'RETRY_SCHEDULED' AND c.next_attempt_at <= ?
  AND (c.aggregate_type IS NULL OR c.aggregate_id = ''
       OR NOT EXISTS (
         SELECT 1 FROM event_consumption e
         WHERE e.tenant_id = c.tenant_id
           AND e.aggregate_type = c.aggregate_type AND e.aggregate_id = c.aggregate_id
           AND e.status IN ('RETRY_SCHEDULED','PROCESSING','DEAD_LETTER')
           AND (
             (e.causal_seq IS NOT NULL AND c.causal_seq IS NOT NULL AND e.causal_seq < c.causal_seq)
             OR (e.causal_seq IS NULL AND c.causal_seq IS NULL
                 AND e.src_partition IS NOT NULL AND c.src_partition IS NOT NULL
                 AND (e.src_partition < c.src_partition OR (e.src_partition = c.src_partition AND e.src_offset < c.src_offset)))
             OR (e.causal_seq IS NULL AND c.causal_seq IS NULL
                 AND (e.src_partition IS NULL OR c.src_partition IS NULL)
                 AND e.first_seen_at < c.first_seen_at)
           )
       ))
ORDER BY c.next_attempt_at ASC
LIMIT ?
FOR UPDATE SKIP LOCKED`

func (s *GormStore) FindEligibleHeads(ctx context.Context, now time.Time, limit int) ([]store.Row, error) {
	if limit <= 0 {
		limit = 50
	}
	var models []EventConsumptionModel
	// D22：status 已写字面量，参数只剩 now 与 limit。
	if err := s.claimDB.WithContext(ctx).Raw(EligibleHeadsSQL, now, limit).Scan(&models).Error; err != nil {
		return nil, err
	}
	out := make([]store.Row, len(models))
	for i := range models {
		out[i] = models[i].ToRow()
	}
	return out, nil
}

// —— ClaimForReplay（RETRY_SCHEDULED→PROCESSING，attempt+1；含 MANUAL 授权消费）——

func (s *GormStore) ClaimForReplay(ctx context.Context, db *gorm.DB, id int64) (reliable.ClaimToken, store.Row, error) {
	claimID := uuid.NewString()
	now := nowUTC()
	lease := 5 * time.Minute
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND replay_mode = ?", id, reliable.StatusRetryScheduled, "AUTO").
		Updates(map[string]any{
			"status": reliable.StatusProcessing, "claim_id": claimID,
			"claimed_at": now, "lease_expires_at": now.Add(lease),
			"last_attempt_at": now, "attempt": gorm.Expr("attempt + 1"),
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 1 {
		var m EventConsumptionModel
		if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
			return "", store.Row{}, err
		}
		return reliable.ClaimToken(claimID), m.ToRow(), nil
	}
	// AUTO CAS 失败：区分「该行是 MANUAL（走人工授权 claim）」与「AUTO 行被别实例抢占/已变状态」。
	// **本轮评审 A1（P0）**：原稿无条件 fall-through 到 claimManualReplay——AUTO 行被别实例 claim 后，
	// 在 claimManualReplay 里因 ReplayMode!=MANUAL 返回 ErrNotPermitted，scheduler 把它送进 MoveToDeadLetter，
	// 毒掉别实例在途的 claim → DEAD_LETTER → 后续重投命中 AlreadySettled 静默永久丢数据（spec §4 v2.7 警告）。
	// 现仅 MANUAL 行走人工路径；其余一律 ErrRetryLater（让路 AdvanceDue，不增 attempt、绝不进死信）。
	var cur EventConsumptionModel
	if err := db.WithContext(ctx).First(&cur, id).Error; err != nil {
		return "", store.Row{}, err
	}
	if cur.ReplayMode == "MANUAL" {
		return s.claimManualReplay(ctx, db, id, claimID, now, lease)
	}
	return "", store.Row{}, reliable.ErrRetryLater
}

func (s *GormStore) claimManualReplay(ctx context.Context, db *gorm.DB, id int64, claimID string, now time.Time, lease time.Duration) (reliable.ClaimToken, store.Row, error) {
	var m EventConsumptionModel
	if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
		return "", store.Row{}, err
	}
	if m.ReplayMode != "MANUAL" || m.ReplayAuthID == "" || m.ReplayAuthConsumedAt != nil {
		return "", store.Row{}, reliable.ErrNotPermitted
	}
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND replay_auth_id = ? AND replay_auth_consumed_at IS NULL",
			id, reliable.StatusRetryScheduled, m.ReplayAuthID).
		Updates(map[string]any{
			"status": reliable.StatusProcessing, "claim_id": claimID,
			"claimed_at": now, "lease_expires_at": now.Add(lease),
			"last_attempt_at": now, "attempt": gorm.Expr("attempt + 1"),
			"replay_auth_consumed_at": now, "row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return "", store.Row{}, reliable.ErrNotPermitted
	}
	if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
		return "", store.Row{}, err
	}
	return reliable.ClaimToken(claimID), m.ToRow(), nil
}

// —— D8：scheduler 三分支处置经 Store 方法（不绕过 §2.4）——

// AdvanceDue 推进 next_attempt_at，**不增 attempt**（§6.2 「让路不是失败」）。
// A3（本轮评审）：加 status 守卫。本方法只对 RETRY_SCHEDULED 行有意义——原稿无守卫，
// scheduler 在 gate 抢不到时对已进 PROCESSING 的行调 AdvanceDue，结果行永远停在 PROCESSING
// （FindEligibleHeads 再也取不到）。现在 scheduler 已把 gate 前置于 ClaimForReplay（见 T11），
// 本方法只会作用于 RETRY_SCHEDULED 行；守卫把这一前提变成可执行的断言。
func (s *GormStore) AdvanceDue(ctx context.Context, db *gorm.DB, id int64) error {
	var m EventConsumptionModel
	if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
		return err
	}
	next := nowUTC().Add(reliable.Backoff(m.Attempt, reliable.DefaultBackoffBase, reliable.DefaultBackoffCap, s.jitter()))
	// review #4（TOCTOU）：Backoff(m.Attempt) 基于 SELECT 读到的 attempt。SELECT-UPDATE 之间若并发
	// ClaimForReplay→MarkFailed 改了 attempt 且把行放回 RETRY_SCHEDULED，无 attempt 守卫会用过期退避覆盖
	// 新 due（行 attempt=3 却按 attempt=2 排程，退避被静默缩短）。加 attempt = m.Attempt 乐观锁（与
	// MarkFailed 同模式）；0 行返回 ErrConflict，scheduler 调用方已把它当良性（handleNonExecution）。
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND attempt = ?", id, reliable.StatusRetryScheduled, m.Attempt).
		Updates(map[string]any{
			"next_attempt_at": next, "updated_at": nowUTC(),
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

// ReleaseClaim 归还已占位的行（A3 本轮评审新增）：PROCESSING → RETRY_SCHEDULED。
// 清 ownership + 按退避推进 due，**不改 attempt**（让路不是失败）。必须出示 tok（fencing）。
//
// chk_retry_due 要求 RETRY_SCHEDULED 必有 payload / next_attempt_at / error_class：
// 前两者由本方法保证（payload 在 claim 期间未被清，due 在这里写）；error_class 因为能被
// ClaimForReplay 拿到的行必定曾是 RETRY_SCHEDULED，所以也非空。WHERE 里仍显式守卫，
// 宁可 ErrConflict 也不招约束错误。
func (s *GormStore) ReleaseClaim(ctx context.Context, db *gorm.DB, id int64, tok reliable.ClaimToken) error {
	var m EventConsumptionModel
	if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
		return err
	}
	now := nowUTC()
	next := now.Add(reliable.Backoff(m.Attempt, reliable.DefaultBackoffBase, reliable.DefaultBackoffCap, s.jitter()))
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND claim_id = ? AND payload IS NOT NULL AND error_class IS NOT NULL",
			id, reliable.StatusProcessing, string(tok)).
		Updates(map[string]any{
			"status":   reliable.StatusRetryScheduled,
			"claim_id": nil, "claimed_at": nil, "lease_expires_at": nil,
			"next_attempt_at": next, "updated_at": now,
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

// MoveToDeadLetter 把【未占位】的 RETRY_SCHEDULED 行移出自动重放队列（§6.2 表：pre-claim 的
// ErrNotPermitted / ErrNotSelfReplayable）。
//
// **本轮评审 A5**：原 A2 把源态放宽到含 PROCESSING 却不校验 claim token——任何调用方都能毒掉别实例
// 在途的 claim（也是 A1 竞态的 exploiting 机制，且独立违反 spec §3.1「Mark* 须带 claim_id」的 fencing
// 对称性——MarkSucceeded/MarkFailed 都要 token，唯独原 MoveToDeadLetter 不要）。现仅认 RETRY_SCHEDULED
// （pre-claim，无需 token）；已占位的 PROCESSING 行走 MoveToDeadLetterWithToken。
//
// chk_dead_payload 要求 DEAD_LETTER 必有 payload 与 error_class，WHERE 里显式加两个 IS NOT NULL 守卫：
// 宁可返回 ErrConflict 让调用方告警，也不要招来约束错误。离开 RETRY_SCHEDULED 时统一清 ownership
// （§2.4 不变量表：DEAD_LETTER 必清 claim_id/claimed_at/lease_expires_at/next_attempt_at）。
func (s *GormStore) MoveToDeadLetter(ctx context.Context, db *gorm.DB, id int64, reason string) error {
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND payload IS NOT NULL AND error_class IS NOT NULL",
			id, reliable.StatusRetryScheduled).
		Updates(map[string]any{
			"status": reliable.StatusDeadLetter, "next_attempt_at": nil,
			"claim_id": nil, "claimed_at": nil, "lease_expires_at": nil,
			"error_message": sanitizeMsg(reason), "updated_at": nowUTC(),
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 1 {
		return nil
	}
	// **R2**：rows==0 区分「状态被别实例抢走（预期，不报错）」与「guard 未命中（真失败）」。原稿一律 ErrConflict
	// → scheduler 对良性竞态也发 REPLAY_DISPOSE_FAILED，训炼运维忽视该通道（boy-who-cried-wolf）。
	// re-SELECT：状态已离开 RETRY_SCHEDULED（被并行 ClaimForReplay 或别实例 MoveToDeadLetter 处理）→ nil（良性）；
	// 仍 RETRY_SCHEDULED 却 0 行 = payload/error_class NULL（违反 chk_retry_due，真异常）→ ErrConflict 告警。
	var cur EventConsumptionModel
	if err := db.WithContext(ctx).First(&cur, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 行消失：无现存 Store 路径删除 RETRY_SCHEDULED 行（Discard/ScheduleReplay 均要求 DEAD_LETTER 源）——属 out-of-band DELETE；返回 nil 视为良性，若未来加 purge API 须重审
		}
		return err // 真 DB 错误，上抛
	}
	if cur.Status != string(reliable.StatusRetryScheduled) {
		return nil // 状态已迁移，别实例在处理/已处置，不告警
	}
	return reliable.ErrConflict // 仍 RETRY_SCHEDULED 却未更新 = guard 未命中，真失败
}

// MoveToDeadLetterWithToken 把【已占位】的 PROCESSING 行移到 DEAD_LETTER，须出示 claim token（A5 fencing）。
// 供 scheduler 在 ClaimForReplay 之后命中 ErrNotPermitted/ErrNotSelfReplayable 时调用——此时行是 PROCESSING、
// 我们持有 tok，payload 与 error_class 在 ClaimForReplay 后仍在（DEAD_LETTER 硬条件满足）。
// WHERE claim_id=tok 保证不会毒掉别实例在途的 claim（与 MarkSucceeded/MarkFailed 对称）；0 行返回 ErrConflict。
func (s *GormStore) MoveToDeadLetterWithToken(ctx context.Context, db *gorm.DB, id int64, tok reliable.ClaimToken, errorClass reliable.ErrorClass, reason string) error {
	updates := map[string]any{
		"status": reliable.StatusDeadLetter, "next_attempt_at": nil,
		"claim_id": nil, "claimed_at": nil, "lease_expires_at": nil,
		"error_message": sanitizeMsg(reason), "updated_at": nowUTC(),
		"row_version": gorm.Expr("row_version + 1"),
	}
	// review #10：调用方可显式覆盖 error_class（defer-exhaustion / panic / not-permitted），避免 DEAD_LETTER
	// 行残留旧 MarkFailed 的 class（如 RETRYABLE）误导 §10 按 error_class 的聚合定位。空串保留原值。
	if errorClass != "" {
		updates["error_class"] = errorClass
	}
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND claim_id = ? AND payload IS NOT NULL AND error_class IS NOT NULL",
			id, reliable.StatusProcessing, string(tok)).
		Updates(updates).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

// —— ScheduleReplay（DEAD_LETTER→RETRY_SCHEDULED，CAS；D12：requester≠approver 强制）——

func (s *GormStore) ScheduleReplay(ctx context.Context, db *gorm.DB, id, expectedVersion int64,
	requester, approver, reason string) error {
	if requester == approver {
		// D12：双人确认由 Store 强制（纵深防御），不只靠服务运维层。
		return reliable.ErrConflict
	}
	now := nowUTC()
	authID := uuid.NewString()
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND row_version = ?", id, reliable.StatusDeadLetter, expectedVersion).
		Updates(map[string]any{
			"status":              reliable.StatusRetryScheduled,
			"replay_generation":   gorm.Expr("replay_generation + 1"),
			"replay_mode":         "MANUAL",
			"replay_requested_by": requester, "replay_approved_by": approver, "replay_reason": reason,
			"replay_auth_id": authID, "replay_auth_consumed_at": nil,
			"next_attempt_at": now, "updated_at": now,
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

func (s *GormStore) Discard(ctx context.Context, db *gorm.DB, id, expectedVersion int64, by, reason string) error {
	now := nowUTC()
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND row_version = ?", id, reliable.StatusDeadLetter, expectedVersion).
		Updates(map[string]any{
			"status": reliable.StatusDiscarded, "resolved_at": now, "resolved_by": by,
			"discard_reason": reason, "claim_id": nil, "claimed_at": nil,
			"lease_expires_at": nil, "next_attempt_at": nil,
			"updated_at": now, "row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}
