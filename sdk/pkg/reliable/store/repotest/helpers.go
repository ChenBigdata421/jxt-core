package repotest

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedRetryRow 直接 INSERT 一条 RETRY_SCHEDULED 行（用于 ⑦ 有序用例，绕过 TryClaim）。
// 注：payload / next_attempt_at / error_class 三列必须非空，否则撞 chk_retry_due。
// 放在非 _test 文件里——conformance.go（非测试）调用它。
func seedRetryRow(t *testing.T, d *ConformanceDeps, in reliable.ClaimInput, causalSeq int64, due time.Time) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, d.DB.Exec(
		`INSERT INTO event_consumption (event_id,item_key,handler_id,tenant_id,event_type,aggregate_type,aggregate_id,causal_seq,topic,status,attempt,replay_mode,payload,next_attempt_at,error_class,first_seen_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,1,'AUTO',?,?,?, ?, ?, ?)`,
		in.Key.EventID, in.Key.ItemKey, string(in.Key.Handler), in.TenantID, in.Meta.EventType, in.Meta.AggregateType, in.Meta.AggregateID, causalSeq, in.Delivery.Topic,
		"RETRY_SCHEDULED", []byte("p"), due, "RETRYABLE", now, now, now,
	).Error)
}

// mustGetFullRow 读整行到 store.Row（用于 ⑫ header 校验）。
// 放在非 _test 文件里——conformance.go（非测试）调用它。
func (d *ConformanceDeps) mustGetFullRow(t *testing.T, key reliable.Key) store.Row {
	t.Helper()
	rows, err := d.Store.List(context.Background(), store.ListFilter{TenantID: 1, Limit: 50}) // S3：List 强制 tenant 作用域
	require.NoError(t, err)
	for _, r := range rows {
		if r.EventID == key.EventID {
			return r
		}
	}
	t.Fatalf("row not found for %s", key.EventID)
	return store.Row{}
}

// ConformanceDeps（D7 单一类型；不再有 Deps/gormDBAlias/gormDB 多套）。
// repotest → store 无循环依赖（store 不 import repotest），直接用真实类型。
type ConformanceDeps struct {
	DB      *gorm.DB
	Store   store.Store
	QStore  store.QuarantineStore
	Dialect Dialect
}

var nameSeq int64

func newClaimInput(t *testing.T, name string) reliable.ClaimInput {
	t.Helper()
	seq := atomic.AddInt64(&nameSeq, 1)
	ev := fmt.Sprintf("%s-%d-%d", name, seq, time.Now().UnixNano())
	return reliable.ClaimInput{
		Key:      reliable.Key{EventID: ev, Handler: reliable.HandlerID("test-handler")},
		Meta:     reliable.Meta{EventType: "FileUploaded", AggregateType: "Media", AggregateID: "agg-" + ev, CausalSeq: ptrI64(seq)},
		TenantID: 1,
		Delivery: reliable.DeliveryMeta{Topic: "domain.media", Partition: 0, Offset: seq,
			BrokerTimestamp: time.Now().UTC(), PayloadHash: fmt.Sprintf("hash-%s", ev),
			RawKey: []byte("rk"), Headers: []reliable.HeaderPair{{Key: "k", Value: []byte("v")}}},
	}
}

func reliableErr(msg string) error { return fmt.Errorf("%s", msg) }

func ptrI64(v int64) *int64 { return &v }

// ptrI32 同 ptrI64 语义（A③：seedRetryRowNoCausal 的 src_partition 是 *int32）。
func ptrI32(v int32) *int32 { return &v }

// mustGetByEvent 用 raw SQL 读 event_consumption（方言无关）。返回关键字段。
func mustGetByEvent(t *testing.T, d *ConformanceDeps, key reliable.Key) rowSnapshot {
	t.Helper()
	var r rowSnapshot
	require.NoError(t, d.DB.WithContext(context.Background()).Raw(
		`SELECT id, status, attempt, claim_id, lease_expires_at, next_attempt_at, error_class, payload, replay_mode, raw_payload_hash, row_version FROM event_consumption WHERE event_id=? AND handler_id=? AND item_key=?`,
		key.EventID, string(key.Handler), key.ItemKey).Scan(&r).Error, "row not found")
	return r
}

// rowSnapshot：raw SQL 读的列子集（足够 conformance 断言）。
type rowSnapshot struct {
	ID             int64
	Status         string
	Attempt        int
	ClaimID        *string
	LeaseExpiresAt *time.Time
	NextAttemptAt  *time.Time
	ErrorClass     *string
	Payload        []byte
	ReplayMode     string
	RawPayloadHash string
	RowVersion     int64
}

func rowCount(t *testing.T, d *ConformanceDeps, key reliable.Key) int64 {
	t.Helper()
	var n int64
	require.NoError(t, d.DB.WithContext(context.Background()).Raw(
		`SELECT COUNT(*) FROM event_consumption WHERE event_id=? AND handler_id=? AND item_key=?`,
		key.EventID, string(key.Handler), key.ItemKey).Scan(&n).Error)
	return n
}

// forceExpireLease 把行的 lease 设为「刚过期」——绑定一个 Go time.Time（两方言均接受），
// 避开 D7 担心的 DATEADD 占位分歧。
//
// C7：用「近期过期」（1 分钟前）而非字面 '1970-01-01 00:00:00'，保留对未来 STUCK_PROCESSING 分级的
// 前瞻兼容——PR-7 会按 `now - lease_expires_at > 2h ⇒ STUCK_PROCESSING` 重新引入 kind 分级（见
// store.ObserveExpiredLeases 的 PR-7 carry-over 注释 + PR2_SCOPE deviation #5）；届时 1970 会恒落
// STUCK_PROCESSING，本用例测的 LEASE_ORPHAN 路径就会被 misclassify。PR-2 本方法只产 LEASE_ORPHAN
// （分级已撤），任何过期量级都命中同一 kind，但保持近期过期让本用例在 PR-7 引入分级后无需再改。
func forceExpireLease(t *testing.T, d *ConformanceDeps, key reliable.Key) {
	t.Helper()
	expired := time.Now().UTC().Add(-1 * time.Minute) // 1 分钟前过期：足够 now > lease，又远不及 PR-7 的 2h stuck 阈值
	require.NoError(t, d.DB.WithContext(context.Background()).Exec(
		`UPDATE event_consumption SET lease_expires_at = ? WHERE event_id=? AND handler_id=? AND item_key=?`,
		expired, key.EventID, string(key.Handler), key.ItemKey).Error)
}

// anomalyCount 返回指定 kind + key 的 anomaly 行数（用于断言幂等：同一次占位只记一条）。
func anomalyCount(t *testing.T, d *ConformanceDeps, kind string, key reliable.Key) int64 {
	t.Helper()
	var n int64
	require.NoError(t, d.DB.WithContext(context.Background()).Raw(
		`SELECT COUNT(*) FROM consumption_anomalies WHERE kind=? AND event_id=? AND handler_id=?`,
		kind, key.EventID, string(key.Handler)).Scan(&n).Error)
	return n
}

func assertAnomalyExists(t *testing.T, d *ConformanceDeps, kind string, key reliable.Key) {
	t.Helper()
	require.GreaterOrEqual(t, anomalyCount(t, d, kind, key), int64(1), "anomaly %s must be recorded", kind)
}

// seedAnomalyAt 直接 INSERT 一条 consumption_anomalies（指定 created_at）——conformance 用例
// 按时间窗 + kind 过滤、按 created_at DESC 排序，需要可控的时间戳。绕过 RecordAnomaly 的 nowUTC()。
// 放在非 _test 文件里——conformance.go（非测试）调用它。
func seedAnomalyAt(t *testing.T, d *ConformanceDeps, tenantID int, kind, eventID, claimID string, createdAt time.Time) {
	t.Helper()
	require.NoError(t, d.DB.Exec(
		`INSERT INTO consumption_anomalies (kind, event_id, handler_id, tenant_id, claim_id, detail, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		kind, eventID, "test-handler", tenantID, claimID, "seeded-by-conformance", createdAt,
	).Error)
}

// seedRetryRowNoCausal 直接 INSERT 一条 causal_seq 恒 NULL 的 RETRY_SCHEDULED 行，且
// src_partition / src_offset / first_seen_at 可控——三段式 earlier-than 谓词
// （EarlierUnsolvedSiblingSQL / EligibleHeadsSQL 同源）的第 2 臂（partition/offset 次序）与
// 第 3 臂（first_seen_at 兜底）只有 causal_seq 双侧为 NULL 才可达：seedRetryRow 恒绑非空
// causal_seq，永远锁死在第 1 臂。nil 指针 = 该列写 NULL（第 3 臂要求至少一侧 partition 为
// NULL；两侧都 NOT NULL 时第 3 臂不可达，正是第 2 臂的封闭性来源）。chk_retry_due 由恒绑
// 的 payload / next_attempt_at / error_class 满足。放非 _test 文件——conformance.go 调用。
//
// ⚠ VALUES 的列序刻意与 seedRetryRow 不同（replay_mode 前移、AUTO 保留字面量）：两个写法
// 不能共享同一个占位符串。VALUES 里也不能写裸 NULL 字面量——gorm/pgx 的绑定位置记账会把
// 后续参数整体左移一位（实测 PG 报 `invalid input syntax for type bigint: "domain.media"`，
// topic 的值落到 src_offset 上）；NULL 一律走类型化 nil 指针占位符（A③ 复盘）。
func seedRetryRowNoCausal(t *testing.T, d *ConformanceDeps, in reliable.ClaimInput, srcPartition *int32, srcOffset *int64, firstSeen, due time.Time) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, d.DB.Exec(
		`INSERT INTO event_consumption (event_id,item_key,handler_id,tenant_id,event_type,aggregate_type,aggregate_id,causal_seq,topic,status,attempt,replay_mode,payload,next_attempt_at,error_class,src_partition,src_offset,first_seen_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,'AUTO',?,?,?,?,?,?,?,?)`,
		in.Key.EventID, in.Key.ItemKey, string(in.Key.Handler), in.TenantID, in.Meta.EventType,
		in.Meta.AggregateType, in.Meta.AggregateID, (*int64)(nil), in.Delivery.Topic,
		"RETRY_SCHEDULED", 1, []byte("p"), due, "RETRYABLE", srcPartition, srcOffset, firstSeen, now, now,
	).Error)
}

// seedRowWithStatus 直接 INSERT 一条 event_consumption 为指定 status——Count conformance 需要跨
// 状态分布的行（seedRetryRow 只能产 RETRY_SCHEDULED）。
//
// 列约束遵循 §2.4：DEAD_LETTER 必须有 payload + error_class（chk_dead_payload）；RETRY_SCHEDULED
// 还需 next_attempt_at（chk_retry_due）。本 helper 恒绑 payload + error_class——DEAD_LETTER 行因此
// 满足 chk_dead_payload（PR-2 leftover A②：旧注释「只产无 chk 约束的终态」不实——DEAD_LETTER 有
// 约束，是本 helper 的恒绑值满足了它）；SUCCEEDED/DISCARDED 无 chk。RETRY_SCHEDULED 需
// next_attempt_at，走 seedRetryRow。放非 _test 文件——conformance.go 调用。
func seedRowWithStatus(t *testing.T, d *ConformanceDeps, in reliable.ClaimInput, status string, firstSeen time.Time) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, d.DB.Exec(
		`INSERT INTO event_consumption (event_id,item_key,handler_id,tenant_id,event_type,aggregate_type,aggregate_id,causal_seq,topic,status,attempt,replay_mode,payload,error_class,first_seen_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,1,'AUTO',?,?, ?, ?, ?)`,
		in.Key.EventID, in.Key.ItemKey, string(in.Key.Handler), in.TenantID, in.Meta.EventType,
		in.Meta.AggregateType, in.Meta.AggregateID, in.Meta.CausalSeq, in.Delivery.Topic,
		status, []byte("p"), "POISON", firstSeen, now, now,
	).Error)
}
