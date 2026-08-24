//go:build system
// +build system

// opsprobe 系统测试（PR-7 Task 1，spec §10 ①④）：三条 ops 只读查询对真实 MySQL + PG 的
// SQL 语义验证。真实库、真 DDL（repotest.Setup 每测试 drop+recreate + 双跑迁移验证幂等）。
//
// 外部测试包 gormshared_test：需要 repotest.Setup（env DSN 逃生舱），而 repotest 经
// mysql/postgres 薄包依赖 gormshared——内部测试包会成环，Go 不允许。
//
// 运行（专用 scratch 容器，carryover 181-183：勿用 testcontainers 的 false-green 陷阱；
// setup.go 会按需自动补 parseTime/multiStatements/loc）：
//
//	RELIABLE_MYSQL_DSN='test:test@tcp(127.0.0.1:3381)/reliable_test?charset=utf8mb4' \
//	RELIABLE_PG_DSN='postgres://test:test@127.0.0.1:5481/reliable_test?sslmode=disable' \
//	go test -tags system ./pkg/reliable/store/gormshared/ -run TestOpsProbe -v
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

var opsDialects = []repotest.Dialect{repotest.DialectMySQL, repotest.DialectPostgres}

// seedOpsRow 直插一行 event_consumption（形态复刻 repotest.seedRowWithStatus——该 helper
// 未导出，外部测试包够不着，故内联同源版本。差异：causal_seq/src_partition/src_offset 恒
// NULL，让排序判定落到三段式第 3 臂 first_seen_at，正好是被测路径）。RETRY_SCHEDULED 行带
// next_attempt_at 满足 chk_retry_due；DEAD_LETTER 行的 payload/error_class 满足
// chk_dead_payload（status 字面量同 D22 口径，与被测 SQL 的写法一致）。
func seedOpsRow(t *testing.T, db *gorm.DB, ev string, handler reliable.HandlerID, status, aggType, aggID string, firstSeen time.Time) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, db.Exec(
		`INSERT INTO event_consumption
		   (event_id,item_key,handler_id,tenant_id,event_type,aggregate_type,aggregate_id,causal_seq,topic,
		    status,attempt,replay_mode,payload,next_attempt_at,error_class,first_seen_at,created_at,updated_at)
		 VALUES (?,'',?,1,'FileUploaded',?,?,NULL,'domain.media',?,1,'AUTO',?,?,'POISON',?,?,?)`,
		ev, string(handler), aggType, aggID, status, []byte("p"), now.Add(time.Hour), firstSeen, now, now,
	).Error)
}

// opsRowID 读回种子行的自增 id（FrozenAggregates 断言要钉死 DeadLetterID）。
func opsRowID(t *testing.T, db *gorm.DB, ev string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(`SELECT id FROM event_consumption WHERE event_id = ?`, ev).Scan(&id).Error)
	require.NotZero(t, id, "seeded row %s must exist", ev)
	return id
}

// TestOpsProbe_RetryAge（§10 ①，v2.10 age-alert 数据源）：每 handler 取最老
// RETRY_SCHEDULED 行的年龄（秒）；SUCCEEDED / DEAD_LETTER 不计入——age 要在 head-block /
// attempt 耗尽之前暴露单行停滞，口径必须只盯未重放完的行。
func TestOpsProbe_RetryAge(t *testing.T) {
	for _, dialect := range opsDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			db, cleanup := repotest.Setup(t, dialect)
			defer cleanup()

			// ms 边界对齐 DATETIME(3)/TIMESTAMP(3)：now 由调用方显式传入（Go 侧折算年龄，
			// 不做方言侧 EXTRACT/EPOCH），种子时间同精度截断后年龄是精确的 60/300。
			base := time.Now().UTC().Truncate(time.Millisecond)
			const h1, h2 = reliable.HandlerID("ops-age-h1"), reliable.HandlerID("ops-age-h2")
			seedOpsRow(t, db, "age-r1", h1, "RETRY_SCHEDULED", "Media", "m1", base.Add(-60*time.Second))
			seedOpsRow(t, db, "age-r2", h2, "RETRY_SCHEDULED", "Media", "m1", base.Add(-300*time.Second))
			// 干扰项：终态与死信即使更老也不得出现在 age 口径里。
			seedOpsRow(t, db, "age-s1", h1, "SUCCEEDED", "Media", "m2", base.Add(-1000*time.Second))
			seedOpsRow(t, db, "age-d1", h2, "DEAD_LETTER", "Media", "m2", base.Add(-2000*time.Second))

			ages, err := gormshared.RetryAgeSeconds(context.Background(), db, base)
			require.NoError(t, err)
			require.Len(t, ages, 2, "only RETRY_SCHEDULED rows count (§10 ①)")
			require.InDelta(t, 60.0, ages[h1], 1.0, "h1 oldest RETRY row is 60s old")
			require.InDelta(t, 300.0, ages[h2], 1.0, "h2 oldest RETRY row is 300s old")
		})
	}
}

// TestOpsProbe_FrozenAggregates（§10 ④，v2.12 聚合冻结告警 P1）：
//   - A：DEAD_LETTER @ (Media,m1) 10:00；B：RETRY_SCHEDULED 同聚合 10:05（B 排序在後）——
//     A 是 earlier-dead ⇒ 冻结命中恰为 [{m1, A.id, A.handler}]。
//   - C：RETRY_SCHEDULED @ (Media,m2) 09:00——无死信兄弟 ⇒ 不命中。
//   - X/Y（review OV②）：aggregate-less 死信 + 更晚的无聚合 RETRY 兄弟（两行 aggregate_id=''）——
//     ''='' 恒真会让任意两条无聚合行互判冻结（永久误报），守卫必须排除，不得命中。
func TestOpsProbe_FrozenAggregates(t *testing.T) {
	for _, dialect := range opsDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			db, cleanup := repotest.Setup(t, dialect)
			defer cleanup()

			base := time.Now().UTC().Truncate(time.Millisecond)
			const hA, hB, hC = reliable.HandlerID("ops-fz-a"), reliable.HandlerID("ops-fz-b"), reliable.HandlerID("ops-fz-c")
			seedOpsRow(t, db, "fz-a", hA, "DEAD_LETTER", "Media", "m1", base)                        // 10:00
			seedOpsRow(t, db, "fz-b", hB, "RETRY_SCHEDULED", "Media", "m1", base.Add(5*time.Minute)) // 10:05
			seedOpsRow(t, db, "fz-c", hC, "RETRY_SCHEDULED", "Media", "m2", base.Add(-time.Hour))    // 09:00
			// OV②：无聚合（通知类）行——不参与冻结判定。
			seedOpsRow(t, db, "fz-x", reliable.HandlerID("ops-fz-x"), "DEAD_LETTER", "", "", base.Add(-30*time.Minute))
			seedOpsRow(t, db, "fz-y", reliable.HandlerID("ops-fz-y"), "RETRY_SCHEDULED", "", "", base.Add(10*time.Minute))

			frozen, err := gormshared.FrozenAggregates(context.Background(), db, 10)
			require.NoError(t, err)
			require.Equal(t, []gormshared.FrozenAggregate{{
				TenantID: 1, AggregateType: "Media", AggregateID: "m1",
				DeadLetterID: opsRowID(t, db, "fz-a"), DeadLetterHandlerID: hA,
			}}, frozen, "exactly A frozen via later sibling B; C (no dead sibling) and aggregate-less X (OV②) excluded")
		})
	}
}

// TestOpsProbe_PendingCounts（§10 深度类指标口径）：status×handler 分组计数，仅统计
// 未终结两态 RETRY_SCHEDULED / DEAD_LETTER——SUCCEEDED/DISCARDED 是终态，不算 backlog。
func TestOpsProbe_PendingCounts(t *testing.T) {
	for _, dialect := range opsDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			db, cleanup := repotest.Setup(t, dialect)
			defer cleanup()

			base := time.Now().UTC().Truncate(time.Millisecond)
			const h1, h2 = reliable.HandlerID("ops-pc-h1"), reliable.HandlerID("ops-pc-h2")
			seedOpsRow(t, db, "pc-r1", h1, "RETRY_SCHEDULED", "Media", "m1", base)
			seedOpsRow(t, db, "pc-r2", h2, "RETRY_SCHEDULED", "Media", "m1", base)
			seedOpsRow(t, db, "pc-d1", h1, "DEAD_LETTER", "Media", "m2", base)
			seedOpsRow(t, db, "pc-s1", h1, "SUCCEEDED", "Media", "m3", base) // 终态不计

			counts, err := gormshared.PendingCounts(context.Background(), db)
			require.NoError(t, err)
			require.Equal(t, map[reliable.Status]map[reliable.HandlerID]int64{
				reliable.StatusRetryScheduled: {h1: 1, h2: 1},
				reliable.StatusDeadLetter:     {h1: 1},
			}, counts)
		})
	}
}

// TestOpsProbe_Empty：空库契约——RetryAgeSeconds/PendingCounts 返回空 map（非 nil）、
// FrozenAggregates 返回空切片，均不报错；limit<=0 走默认 100 分支。
func TestOpsProbe_Empty(t *testing.T) {
	for _, dialect := range opsDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			db, cleanup := repotest.Setup(t, dialect)
			defer cleanup()

			ctx := context.Background()
			ages, err := gormshared.RetryAgeSeconds(ctx, db, time.Now().UTC())
			require.NoError(t, err)
			require.NotNil(t, ages)
			require.Empty(t, ages)

			frozen, err := gormshared.FrozenAggregates(ctx, db, 0)
			require.NoError(t, err)
			require.Empty(t, frozen)

			counts, err := gormshared.PendingCounts(ctx, db)
			require.NoError(t, err)
			require.NotNil(t, counts)
			require.Empty(t, counts)
		})
	}
}
