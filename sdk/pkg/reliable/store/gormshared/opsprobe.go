package gormshared

import (
	"context"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"gorm.io/gorm"
)

// RetryAgeSeconds 按 handler 返回最老 RETRY_SCHEDULED 行的年龄（秒，now - first_seen_at）。
// §10 ①（v2.10 age-alert 的数据源）：age 在 head-block/attempt 耗尽之前暴露单行停滞。
// 状态字面量不参数化（D22：两方言稳定命中索引）。
// 可移植性：年龄计算放 Go 侧（MIN(first_seen_at) 折算），不做方言侧 EXTRACT(EPOCH)。
//
// RetryAgeSecondsSQL 是包级导出常量（PR-7 Task 21，沿 EligibleHeadsSQL 的 D22 先例）：
// repotest 的 EXPLAIN 门禁直接引用同一字符串，零复制、零漂移——若把 SQL 复制一份进测试，
// 门禁很快就会变成在测一个没人跑的查询。字面量 status 让 PG 的 partial idx_unresolved
// 谓词可被蕴含（参数化 status = $1 在 generic plan 下无法蕴含，间歇退化 Seq Scan，D22 同源）。
const RetryAgeSecondsSQL = `
SELECT handler_id, MIN(first_seen_at) AS oldest
FROM event_consumption
WHERE status = 'RETRY_SCHEDULED'
GROUP BY handler_id`

func RetryAgeSeconds(ctx context.Context, db *gorm.DB, now time.Time) (map[reliable.HandlerID]float64, error) {
	type row struct {
		HandlerID string    `gorm:"column:handler_id"`
		Oldest    time.Time `gorm:"column:oldest"`
	}
	var rows []row
	err := db.WithContext(ctx).Raw(RetryAgeSecondsSQL).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[reliable.HandlerID]float64, len(rows))
	for _, r := range rows {
		out[reliable.HandlerID(r.HandlerID)] = now.Sub(r.Oldest).Seconds()
	}
	return out, nil
}

// FrozenAggregate 是聚合冻结告警的一条命中（§10 ④，v2.12）。
type FrozenAggregate struct {
	TenantID            int
	AggregateType       string
	AggregateID         string
	DeadLetterID        int64
	DeadLetterHandlerID reliable.HandlerID
}

// FrozenAggregatesSQL 与 EarlierUnsolvedSiblingSQL（replay.go）同源的三段式 earlier-than 谓词，
// 但方向相反：找出「排序在先且未解决」的 DEAD_LETTER 行 e（它冻结了同聚合其它行的重放）。
// 与 EligibleHeadsSQL/EarlierUnsolvedSiblingSQL 的锁步耦合约定相同：任何一方改 earlier-than
// 谓词，三方必须同步（R4-H：禁止服务侧手抄）。
// aggregate-less 守卫（review OV②）：与 EligibleHeadsSQL 外层/replay.go:39 同源——无聚合的
// 通知类行不参与冻结判定（两行 aggregate_id 同为 '' 时 ''='' 恒真，会让任意两条 aggregate-less
// 行互判冻结，永久误报）。
const FrozenAggregatesSQL = `
SELECT e.tenant_id, e.aggregate_type, e.aggregate_id, e.id AS dead_letter_id, e.handler_id AS dead_letter_handler_id
FROM event_consumption e
WHERE e.status = 'DEAD_LETTER'
  AND (e.aggregate_type IS NOT NULL AND e.aggregate_id <> '')
  AND EXISTS (
    SELECT 1 FROM event_consumption c
    WHERE c.tenant_id = e.tenant_id
      AND c.aggregate_type = e.aggregate_type AND c.aggregate_id = e.aggregate_id
      AND c.status IN ('RETRY_SCHEDULED','PROCESSING','DEAD_LETTER')
      AND c.id <> e.id
      AND (
        (c.causal_seq IS NOT NULL AND e.causal_seq IS NOT NULL AND c.causal_seq > e.causal_seq)
        OR (c.causal_seq IS NULL AND e.causal_seq IS NULL
            AND c.src_partition IS NOT NULL AND e.src_partition IS NOT NULL
            AND (c.src_partition > e.src_partition OR (c.src_partition = e.src_partition AND c.src_offset > e.src_offset)))
        OR (c.causal_seq IS NULL AND e.causal_seq IS NULL
            AND (c.src_partition IS NULL OR e.src_partition IS NULL)
            AND c.first_seen_at > e.first_seen_at)
      )
  )
ORDER BY e.id
LIMIT ?`

// FrozenAggregates 列出被「排序在先的死信」冻结的聚合（§10 ④ P1）。limit<=0 → 100。
func FrozenAggregates(ctx context.Context, db *gorm.DB, limit int) ([]FrozenAggregate, error) {
	if limit <= 0 {
		limit = 100
	}
	var out []FrozenAggregate
	err := db.WithContext(ctx).Raw(FrozenAggregatesSQL, limit).Scan(&out).Error
	return out, err
}

// PendingCountsSQL 是包级导出常量（PR-7 Task 21，沿 EligibleHeadsSQL 的 D22 先例）：
// repotest 的 EXPLAIN 门禁直接引用同一字符串，零复制、零漂移。status IN 写两态字面量
// 而非参数，理由同 RetryAgeSecondsSQL（PG partial idx_unresolved 谓词蕴含，D22 同源）。
const PendingCountsSQL = `
SELECT status, handler_id, COUNT(*) AS n
FROM event_consumption
WHERE status IN ('RETRY_SCHEDULED','DEAD_LETTER')
GROUP BY status, handler_id`

// PendingCounts 返回 status×handler 分组计数，仅统计未终结两态（§10 深度类指标口径）。
func PendingCounts(ctx context.Context, db *gorm.DB) (map[reliable.Status]map[reliable.HandlerID]int64, error) {
	type row struct {
		Status    string
		HandlerID string
		N         int64
	}
	var rows []row
	err := db.WithContext(ctx).Raw(PendingCountsSQL).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[reliable.Status]map[reliable.HandlerID]int64)
	for _, r := range rows {
		if out[reliable.Status(r.Status)] == nil {
			out[reliable.Status(r.Status)] = make(map[reliable.HandlerID]int64)
		}
		out[reliable.Status(r.Status)][reliable.HandlerID(r.HandlerID)] = r.N
	}
	return out, err
}
