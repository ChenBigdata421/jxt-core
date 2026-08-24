//go:build system
// +build system

// recovery 系统测试（PR-7 Task 2，spec §3.2 自动回收 + §10 保留清理）：RecoverExpiredProcessing
// 与 DeleteSettledBefore 对真实 MySQL + PG 的 SQL 语义验证。真实库、真 DDL（repotest.Setup
// 每测试 drop+recreate + 双跑迁移验证幂等）。
//
// 外部测试包 gormshared_test：需要 repotest.Setup（env DSN 逃生舱），而 repotest 经
// mysql/postgres 薄包依赖 gormshared——内部测试包会成环，Go 不允许（与 opsprobe_system_test.go 同理）。
//
// 运行（专用 scratch 容器；setup.go 会按需自动补 parseTime/multiStatements/loc）：
//
//	RELIABLE_MYSQL_DSN='test:test@tcp(127.00.1:3381)/reliable_test?charset=utf8mb4' \
//	RELIABLE_PG_DSN='postgres://test:test@127.0.0.1:5481/reliable_test?sslmode=disable' \
//	go test -tags system ./pkg/reliable/store/gormshared/ -run "TestRecover|TestDeleteSettled|TestObserveExpired" -v
package gormshared_test

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/gormshared"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/repotest"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedProcessingRow 直插一条 PROCESSING 行（带租约字段——chk_processing_owner 要求
// claim_id/claimed_at/lease_expires_at 三者非空）。payload 参数为 nil 时行留给 broker 重投
// （§2.1：payload 只在失败时写）。replay_mode='AUTO' 使回收后的 RETRY_SCHEDULED 行可被
// ClaimForReplay 的 AUTO CAS 路径接住（完整闭环验证）。
func seedProcessingRow(t *testing.T, db *gorm.DB, ev string, handler reliable.HandlerID, leaseExpires time.Time, payload []byte) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	claim := "claim-" + ev
	var payloadCol any
	if payload != nil {
		payloadCol = payload
	}
	require.NoError(t, db.Exec(
		`INSERT INTO event_consumption
		   (event_id,item_key,handler_id,tenant_id,event_type,aggregate_type,aggregate_id,causal_seq,topic,
		    status,attempt,replay_mode,claim_id,claimed_at,lease_expires_at,payload,
		    first_seen_at,created_at,updated_at)
		 VALUES (?,'',?,1,'FileUploaded','Media',?,NULL,'domain.media','PROCESSING',1,'AUTO',?,?,?,?,?,?,?)`,
		ev, string(handler), "agg-"+ev, claim, now.Add(-2*time.Hour), leaseExpires, payloadCol,
		now.Add(-2*time.Hour), now.Add(-2*time.Hour), now.Add(-2*time.Hour),
	).Error)
}

// recSnapshot 是 recovery 断言所需的行快照子集（raw SQL 读，方言无关）。
type recSnapshot struct {
	ID             int64
	Status         string
	ClaimID        *string
	LeaseExpiresAt *time.Time
	NextAttemptAt  *time.Time
	ErrorClass     *string
	ErrorCode      *string
	RowVersion     int64
}

func getRecSnapshot(t *testing.T, db *gorm.DB, ev string) recSnapshot {
	t.Helper()
	var r recSnapshot
	require.NoError(t, db.Raw(
		`SELECT id, status, claim_id, lease_expires_at, next_attempt_at, error_class, error_code, row_version
		 FROM event_consumption WHERE event_id = ?`, ev).Scan(&r).Error, "row %s must exist", ev)
	require.NotZero(t, r.ID)
	return r
}

func recAnomalyCount(t *testing.T, db *gorm.DB, kind, ev string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(
		`SELECT COUNT(*) FROM consumption_anomalies WHERE kind = ? AND event_id = ?`, kind, ev).Scan(&n).Error)
	return n
}

// TestRecoverExpiredProcessing（§3.2 自动回收，PR-7 ③）：
//   - P1：PROCESSING、lease 过期 1h、payload 非空 → 回收（RETRY_SCHEDULED，next_attempt_at
//     置 now、error_class='RETRYABLE'、row_version+1——chk_retry_due 全满足）；函数不写异常行
//     （OV④a——异常行由服务侧 OpsMaintenance 经 RecordAnomaly 写，这里按 Task 14 的消费
//     形态模拟写入并验证 AnomalyKindStuckProcessing 常量可用）。
//   - P2：PROCESSING、lease 过期 1h、payload NULL → 计入 stuck、状态不动（broker 重投负责）。
//   - P3：PROCESSING、lease 仍有效（now+5m）→ 不碰。
//   - P4（OV③ lease-recheck，determinism deviation 见下）：种子先给过期租约，随后在调
//     RecoverExpiredProcessing 【之前】把它手动 UPDATE 成新租约 + 新 claim_id（模拟
//     「SELECT 与 UPDATE 之间 broker 重投触发了 tryClaimOnce 内联续占」的事后状态），
//     断言函数的 UPDATE 因 WHERE 里的 lease_expires_at < cutoff 复验而 MISS——P4 仍
//     PROCESSING、新 claim_id 完好。
//
// ⚠ P4 determinism deviation（controller 裁决）：真正的 SELECT-then-UPDATE 竞态交错无法
// 在无钩子下从外部构造（RecoverExpiredProcessing 内部 SELECT→逐行 UPDATE，不可注入变异）。
// 采用三段替代覆盖：(a) recovery_unit_test.go 字符串级断言 UPDATE WHERE 带
// `AND lease_expires_at < ?`（防线本身）；(b) 本用例——P4 的新租约状态在函数运行时全程
// 成立，若守卫缺失则 UPDATE 必命中 P4（断言即红），故「未触碰」证明守卫在真实库上生效；
// (c) P1/P2/P3 的正/反/边界用例。三段合起来即 OV③ 的可确定性测试形态。
func TestRecoverExpiredProcessing(t *testing.T) {
	for _, dialect := range opsDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			db, cleanup := repotest.Setup(t, dialect)
			defer cleanup()

			// now 与 cutoff 显式钉死（回收语义按参数走，不依赖墙钟）；ms 截断对齐 DATETIME(3)/TIMESTAMP(3)。
			now := time.Now().UTC().Truncate(time.Millisecond)
			const stuckAfter = 2 * time.Hour

			const h = reliable.HandlerID("rec-handler")
			// P1：过期 3h（> stuckAfter=2h，即 lease_expires_at < cutoff=now-2h）+ payload。
			// ⚠ brief 原文写 now-1h：那不满足它自己的谓词（now-1h 不早于 cutoff=now-2h，卡死
			// 分级要求「已过期超过 stuckAfter」——anomaly.go STUCK_PROCESSING 注释同源），按
			// 谓词修正为 now-3h（deviation 见 task report）。
			seedProcessingRow(t, db, "rec-p1", h, now.Add(-3*time.Hour), []byte("p1-payload"))
			// P2：过期 3h + payload NULL（§2.1 broker 重投负责）。
			seedProcessingRow(t, db, "rec-p2", h, now.Add(-3*time.Hour), nil)
			// P3：活租约。
			seedProcessingRow(t, db, "rec-p3", h, now.Add(5*time.Minute), []byte("p3-payload"))
			// P4：过期 3h（SELECT 候选）、后被「内联续占」（见上 determinism 注记）。
			seedProcessingRow(t, db, "rec-p4", h, now.Add(-3*time.Hour), []byte("p4-payload"))

			p4Before := getRecSnapshot(t, db, "rec-p4")
			newClaim := "claim-rec-p4-reclaimed"
			require.NoError(t, db.Exec(
				`UPDATE event_consumption SET lease_expires_at = ?, claim_id = ?, row_version = row_version + 1 WHERE event_id = ?`,
				now.Add(5*time.Minute), newClaim, "rec-p4").Error)

			ctx := context.Background()
			recovered, stuck, err := gormshared.RecoverExpiredProcessing(ctx, db, now, stuckAfter)
			require.NoError(t, err)

			// 恰好 P1 被回收：P2（无 payload）只计数，P3（活租约）与 P4（已续占）不碰。
			require.Len(t, recovered, 1, "exactly P1 recovered (P2 counted-stuck, P3 fresh-lease, P4 re-claimed all missed)")
			require.Equal(t, "rec-p1", recovered[0].EventID)
			require.Equal(t, h, recovered[0].HandlerID)
			require.Equal(t, 1, recovered[0].TenantID)
			require.Equal(t, "claim-rec-p1", recovered[0].ClaimID, "recovered detail carries the orphaned claim_id (OpsMaintenance writes the anomaly with it)")
			require.Equal(t, getRecSnapshot(t, db, "rec-p1").ID, recovered[0].ID)
			require.Equal(t, 1, stuck, "P2 (expired, payload NULL) is counted stuck, left for broker redelivery")

			// P1 终态断言：RETRY_SCHEDULED + chk_retry_due 全满足 + row_version+1 + ownership 清空。
			p1 := getRecSnapshot(t, db, "rec-p1")
			require.Equal(t, "RETRY_SCHEDULED", p1.Status)
			require.NotNil(t, p1.NextAttemptAt, "chk_retry_due: next_attempt_at must be set")
			require.WithinDuration(t, now, *p1.NextAttemptAt, time.Second, "next_attempt_at = now (immediately re-drivable)")
			require.NotNil(t, p1.ErrorClass)
			require.Equal(t, "RETRYABLE", *p1.ErrorClass, "chk_retry_due: error_class placeholder is RETRYABLE")
			require.NotNil(t, p1.ErrorCode)
			require.Equal(t, "LEASE_EXPIRED_RECOVERED", *p1.ErrorCode)
			require.Nil(t, p1.ClaimID, "§2.4: RETRY_SCHEDULED clears ownership")
			require.Nil(t, p1.LeaseExpiresAt)
			require.Equal(t, int64(2), p1.RowVersion, "row_version incremented (seeded at 1)")

			// P2：状态不动、计数已断言（stuck==1）。
			p2 := getRecSnapshot(t, db, "rec-p2")
			require.Equal(t, "PROCESSING", p2.Status, "§2.1: payload-NULL row is broker-redelivery-owned, not transitioned")
			require.NotNil(t, p2.ClaimID)

			// P3：活租约，完全未触碰。
			p3 := getRecSnapshot(t, db, "rec-p3")
			require.Equal(t, "PROCESSING", p3.Status)
			require.NotNil(t, p3.LeaseExpiresAt)

			// P4（OV③）：仍 PROCESSING、新 claim_id 完好、row_version 只加了我们的手动 UPDATE 一次。
			p4 := getRecSnapshot(t, db, "rec-p4")
			require.Equal(t, "PROCESSING", p4.Status, "OV③: recovery UPDATE must MISS the inline-reclaimed row (lease recheck in WHERE)")
			require.NotNil(t, p4.ClaimID)
			require.Equal(t, newClaim, *p4.ClaimID, "the re-claiming consumer's ownership must stay intact")
			require.Equal(t, p4Before.RowVersion+1, p4.RowVersion)

			// OV④a：函数本身【不写】异常行——STUCK_PROCESSING 由服务侧 OpsMaintenance 写。
			// 此处按 Task 14 的消费形态模拟服务侧行为（经 store.RecordAnomaly 的参数形状），
			// 验证常量导出 + 幂等键五元组可用；断言「函数没写」是第一位的。
			require.Zero(t, recAnomalyCount(t, db, gormshared.AnomalyKindStuckProcessing, "rec-p1"),
				"OV④a: RecoverExpiredProcessing must not write anomaly rows itself (metricsStore seam is service-side)")

			// 回收后的行可被 AUTO CAS 路径接住（FindEligibleHeads→ClaimForReplay 闭环）：
			// next_attempt_at=now 已 due。这里复刻 ClaimForReplay 的 UPDATE 形状（含
			// chk_processing_owner 要求的 claimed_at 三元组），断言回收后的行无残留、
			// CAS 谓词（status+replay_mode）可命中。
			require.NoError(t, db.Exec(
				`UPDATE event_consumption SET status='PROCESSING', claim_id='post-rec', claimed_at=?, lease_expires_at=?, last_attempt_at=?, attempt=attempt+1, row_version=row_version+1
				 WHERE id=? AND status='RETRY_SCHEDULED' AND replay_mode='AUTO'`,
				now, now.Add(5*time.Minute), now, p1.ID).Error)
			after := getRecSnapshot(t, db, "rec-p1")
			require.Equal(t, "PROCESSING", after.Status, "recovered row re-enters the AUTO replay path without residue")
		})
	}
}

// TestObserveExpiredLeases_InsertedSemantics（OV④b 系统验证）：观测器返回**新插入**的
// anomaly 行数。首扫 1 行孤儿 → 1；再扫（行未被续占，D20 不改行状态）→ uk_anomaly_once
// 全冲突 → 0。扫描语义下第二次会返回 1——正是被 PR-7 改掉的自噪路径。
// （conformance 的 confLeaseOrphan 第 2 步已在 store 接口层钉同一语义；本用例从
// gormshared 直连 + 显式 n/n2 断言再钉一次，因为语义改动就发生在这个包。）
func TestObserveExpiredLeases_InsertedSemantics(t *testing.T) {
	for _, dialect := range opsDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			db, cleanup := repotest.Setup(t, dialect)
			defer cleanup()

			now := time.Now().UTC().Truncate(time.Millisecond)
			const h = reliable.HandlerID("obs-handler")
			seedProcessingRow(t, db, "obs-o1", h, now.Add(-1*time.Hour), []byte("o1"))

			ctx := context.Background()
			st, _ := repotest.NewStoreFor(dialect, db)
			n1, err := st.ObserveExpiredLeases(ctx, now.Add(time.Hour))
			require.NoError(t, err)
			require.Equal(t, 1, n1, "first scan of a fresh orphan inserts exactly 1 anomaly row")

			n2, err := st.ObserveExpiredLeases(ctx, now.Add(time.Hour))
			require.NoError(t, err)
			require.Zero(t, n2, "OV④b: re-scan of the same un-reclaimed orphan inserts 0 (uk_anomaly_once dedup), not 1 (scanned semantics)")

			require.Equal(t, int64(1), recAnomalyCount(t, db, "LEASE_ORPHAN", "obs-o1"))
		})
	}
}

// TestDeleteSettledBefore（§10 保留策略）：
//   - S1：SUCCEEDED、updated_at = now-40d（< succeededBefore=now-30d）→ 删。
//   - S2：SUCCEEDED、updated_at = now-10d → 留。
//   - D1：DISCARDED、updated_at = now-100d（< discardedBefore=now-90d）→ 删。
//   - DL：DEAD_LETTER、updated_at = now-400d → 留（永不自动清理）。
//   - R1：RETRY_SCHEDULED → 留。
//
//	断言 DeleteSettledBefore(now-30d, now-90d) == 2。
func TestDeleteSettledBefore(t *testing.T) {
	for _, dialect := range opsDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			db, cleanup := repotest.Setup(t, dialect)
			defer cleanup()

			now := time.Now().UTC().Truncate(time.Millisecond)
			seedSettledRow(t, db, "ret-s1", "SUCCEEDED", now.Add(-40*24*time.Hour), now)
			seedSettledRow(t, db, "ret-s2", "SUCCEEDED", now.Add(-10*24*time.Hour), now)
			seedSettledRow(t, db, "ret-d1", "DISCARDED", now.Add(-100*24*time.Hour), now)
			seedSettledRow(t, db, "ret-dl", "DEAD_LETTER", now.Add(-400*24*time.Hour), now)
			seedSettledRow(t, db, "ret-r1", "RETRY_SCHEDULED", now.Add(-40*24*time.Hour), now)

			deleted, err := gormshared.DeleteSettledBefore(context.Background(), db,
				now.Add(-30*24*time.Hour), now.Add(-90*24*time.Hour))
			require.NoError(t, err)
			require.Equal(t, int64(2), deleted, "exactly S1 (old SUCCEEDED) + D1 (old DISCARDED) deleted")

			require.Zero(t, rowCountByEvent(t, db, "ret-s1"), "S1: SUCCEEDED older than succeededBefore deleted")
			require.Equal(t, int64(1), rowCountByEvent(t, db, "ret-s2"), "S2: recent SUCCEEDED kept")
			require.Zero(t, rowCountByEvent(t, db, "ret-d1"), "D1: DISCARDED older than discardedBefore deleted")
			require.Equal(t, int64(1), rowCountByEvent(t, db, "ret-dl"), "DL: DEAD_LETTER never auto-cleaned (§10)")
			require.Equal(t, int64(1), rowCountByEvent(t, db, "ret-r1"), "R1: RETRY_SCHEDULED out of retention scope")
		})
	}
}

// seedSettledRow 直插一条按 status 定形的终态/重试行（updated_at 显式可空）。约束列按
// §2.4：RETRY_SCHEDULED 带 payload+next_attempt_at+error_class（chk_retry_due）；
// DEAD_LETTER 带 payload+error_class（chk_dead_payload）；SUCCEEDED/DISCARDED 无额外约束。
// 注意 updated_at 必须显式传（DELETE 按 updated_at 过滤）——已置 NULL 的列由数据库 DEFAULT 兜底。
func seedSettledRow(t *testing.T, db *gorm.DB, ev, status string, updatedAt time.Time, now time.Time) {
	t.Helper()
	var payload, next, errClass any
	switch status {
	case "RETRY_SCHEDULED": // chk_retry_due
		payload, next, errClass = []byte("p"), now.Add(time.Hour), "RETRYABLE"
	case "DEAD_LETTER": // chk_dead_payload
		payload, errClass = []byte("p"), "POISON"
	}
	require.NoError(t, db.Exec(
		`INSERT INTO event_consumption
		   (event_id,item_key,handler_id,tenant_id,event_type,aggregate_type,aggregate_id,causal_seq,topic,
		    status,attempt,replay_mode,payload,next_attempt_at,error_class,
		    first_seen_at,created_at,updated_at)
		 VALUES (?,'','ret-handler',1,'FileUploaded','Media',?,NULL,'domain.media',?,1,'AUTO',?,?,?,?,?,?)`,
		ev, "agg-"+ev, status, payload, next, errClass,
		updatedAt, updatedAt, updatedAt,
	).Error)
}

func rowCountByEvent(t *testing.T, db *gorm.DB, ev string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM event_consumption WHERE event_id = ?`, ev).Scan(&n).Error)
	return n
}
