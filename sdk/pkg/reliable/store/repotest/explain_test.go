package repotest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/gormshared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// explainSeedAggregates × explainSeedPerAggregate = 2000 行。
const (
	explainSeedAggregates   = 200
	explainSeedPerAggregate = 10
)

// eligibleHeadsExplainSQL 直接取自 gormshared 的导出常量（同一个字符串，零复制、零漂移），
// 只去掉尾部 FOR UPDATE SKIP LOCKED——EXPLAIN 不需要也不应该真去加锁。
var eligibleHeadsExplainSQL = strings.TrimSuffix(
	strings.TrimSpace(gormshared.EligibleHeadsSQL), "FOR UPDATE SKIP LOCKED")

// TestExplainEligibleHeads 是 D22 索引门禁：双方言断言 eligible-head 查询不退化为全表扫描。
func TestExplainEligibleHeads(t *testing.T) {
	if testing.Short() {
		t.Skip("explain gate seeds 2K rows; skipped in -short")
	}
	for _, dialect := range []Dialect{DialectMySQL, DialectPostgres} {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			db, cleanup := Setup(t, dialect)
			defer cleanup()
			seedExplainRows(t, db)
			now := time.Now().UTC()
			switch dialect {
			case DialectMySQL:
				assertMySQLUsesIndex(t, db, now)
			case DialectPostgres:
				assertPostgresNoSeqScan(t, db, now)
			}
			// SKIP LOCKED 必须仍在真实 SQL 里（准入 ⑭ 点名）。注意：它只是并发扫描者之间的
			// best-effort 去重，正确性靠 ClaimForReplay 的 CAS（见 gormshared 注释 A4）。
			assert.Contains(t, gormshared.EligibleHeadsSQL, "FOR UPDATE SKIP LOCKED")
		})
	}
}

// seedExplainRows 种 2000 行 RETRY_SCHEDULED：一半带 causal_seq，一半不带
// （无 causal_seq 是子查询最坏情形，依赖 idx_aggregate 尾部的 first_seen_at）。
func seedExplainRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Now().UTC().Add(-time.Hour)
	for a := 0; a < explainSeedAggregates; a++ {
		for i := 0; i < explainSeedPerAggregate; i++ {
			var causal any
			if a%2 == 0 {
				causal = int64(i)
			} // 奇数聚合留 NULL
			require.NoError(t, db.Exec(
				`INSERT INTO event_consumption (event_id,item_key,handler_id,tenant_id,event_type,aggregate_type,aggregate_id,causal_seq,topic,status,attempt,replay_mode,payload,next_attempt_at,error_class,src_partition,src_offset,first_seen_at,created_at,updated_at)
				 VALUES (?,'',?,1,'FileUploaded','Media',?,?,'domain.media','RETRY_SCHEDULED',1,'AUTO',?,?,'RETRYABLE',?,?,?,?,?)`,
				fmt.Sprintf("explain-%d-%d", a, i), "explain-handler", fmt.Sprintf("agg-%d", a), causal,
				[]byte("p"), now.Add(time.Duration(i)*time.Second), int32(a%4), int64(a*100+i),
				now.Add(time.Duration(i)*time.Second), now, now,
			).Error)
		}
	}
	// 让优化器拿到真实统计（否则空统计下计划不代表生产行为）。
	if db.Dialector.Name() == "postgres" {
		require.NoError(t, db.Exec(`ANALYZE event_consumption`).Error)
	} else {
		require.NoError(t, db.Exec(`ANALYZE TABLE event_consumption`).Error)
	}
}

// assertMySQLUsesIndex：EXPLAIN FORMAT=JSON 断言 access_type != ALL 且用了 idx_due。
func assertMySQLUsesIndex(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()
	var plan string
	require.NoError(t, db.Raw("EXPLAIN FORMAT=JSON "+eligibleHeadsExplainSQL, now, 50).Scan(&plan).Error)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(plan), &parsed), "EXPLAIN JSON must parse")
	assert.NotContains(t, plan, `"access_type": "ALL"`, "D22: eligible-head query must not full-scan event_consumption")
	assert.Contains(t, plan, "idx_due", "D22: outer query must use idx_due")
	assert.Contains(t, plan, "idx_aggregate", "D22: NOT EXISTS subquery must use idx_aggregate")
}

// assertPostgresNoSeqScan：EXPLAIN (FORMAT JSON) 断言计划树里没有对 event_consumption 的 Seq Scan。
// 不用 ANALYZE（不真跑），也不断言 buffer hit ratio（那依赖 cache 预热，属 PR-7 perf gate）。
func assertPostgresNoSeqScan(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()
	var plan string
	require.NoError(t, db.Raw("EXPLAIN (ANALYZE false, FORMAT JSON) "+eligibleHeadsExplainSQL, now, 50).Scan(&plan).Error)
	var parsed []map[string]any
	require.NoError(t, json.Unmarshal([]byte(plan), &parsed), "EXPLAIN JSON must parse")
	assert.NotContains(t, plan, `"Seq Scan"`, "D22: no sequential scan on event_consumption")
	assert.Contains(t, plan, "idx_due", "D22: outer query must use partial idx_due (status is a literal, not a bind param)")
	assert.Contains(t, plan, "idx_aggregate", "D22: NOT EXISTS subquery must use idx_aggregate")
}
