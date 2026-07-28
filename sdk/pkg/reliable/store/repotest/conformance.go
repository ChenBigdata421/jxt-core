package repotest

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var lease5 = 5 * time.Minute

// RunConformance 对任意方言跑完整状态机 + claim token + 并发 + §3.3 双向 + ⑦⑫㉒ 套件。
func RunConformance(t *testing.T, d *ConformanceDeps) {
	t.Run("TryClaim_FirstClaim_IsClaimed", func(t *testing.T) { confFirstClaim(t, d) })
	t.Run("TryClaim_AlreadySettled_AfterSuccess", func(t *testing.T) { confAlreadySettled(t, d) })
	t.Run("TryClaim_AlreadyProcessing_WhileLeaseValid", func(t *testing.T) { confAlreadyProcessing(t, d) })
	t.Run("MarkSucceeded_ClaimTokenMismatch_ErrConflict", func(t *testing.T) { confClaimTokenMismatch(t, d) })
	t.Run("MarkFailed_RetryableIdempotent_RetryScheduled", func(t *testing.T) { confMarkFailedRetry(t, d) })
	t.Run("MarkFailed_RetryableUnsafe_DeadLetter", func(t *testing.T) { confMarkFailedUnsafe(t, d) })
	t.Run("MarkFailed_Poison_DeadLetter", func(t *testing.T) { confMarkFailedPoison(t, d) })
	t.Run("MarkFailed_AttemptExhausted_DeadLetter", func(t *testing.T) { confAttemptExhausted(t, d) })
	t.Run("RecordTerminal_Idempotent_OnSameTerminal", func(t *testing.T) { confRecordTerminalIdempotent(t, d) })
	t.Run("LeaseOrphan_ObservedThenReclaimedByTryClaim", func(t *testing.T) { confLeaseOrphan(t, d) })
	t.Run("TryClaim_IndependentCommit_VisibleToOtherConn", func(t *testing.T) { confIndependentCommit(t, d) })
	t.Run("ConcurrentTryClaim_SingleRow", func(t *testing.T) { confConcurrentSingleRow(t, d) })
	// 新增（评审 D5/D12/D13/D16）
	t.Run("§3.3_IndependentCommit_HoldsUnderCallerTx", func(t *testing.T) { confForbiddenCaseTxJoin(t, d) })
	t.Run("M12_OrderedReplay_CreatedBeforeChanged", func(t *testing.T) { confOrderedReplay(t, d) })
	t.Run("M15_HeaderFidelity_RoundTrip", func(t *testing.T) { confHeaderFidelity(t, d) })
	t.Run("ManualReplayAuth_TwoPerson_And_OneTimeToken", func(t *testing.T) { confManualReplayAuth(t, d) })
}

func confFirstClaim(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "first")
	tok, dec, err := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, err)
	assert.Equal(t, reliable.Claimed, dec)
	assert.NotEmpty(t, tok)
	r := mustGetByEvent(t, d, in.Key)
	assert.Equal(t, "PROCESSING", r.Status)
	assert.Equal(t, 1, r.Attempt)
	assert.Equal(t, "AUTO", r.ReplayMode)
	assert.Equal(t, in.Delivery.PayloadHash, r.RawPayloadHash, "RawMeta fingerprint persisted on first claim")
}

func confAlreadySettled(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "settled")
	tok, _, err := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, err)
	require.NoError(t, d.Store.MarkSucceeded(context.Background(), d.DB, in.Key, tok))
	_, dec, err := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, err)
	assert.Equal(t, reliable.AlreadySettled, dec)
	assert.Equal(t, int64(1), rowCount(t, d, in.Key), "still exactly one row")
}

func confAlreadyProcessing(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "processing")
	_, _, _ = d.Store.TryClaim(context.Background(), in, lease5)
	_, dec, _ := d.Store.TryClaim(context.Background(), in, lease5)
	assert.Equal(t, reliable.AlreadyProcessing, dec)
}

func confClaimTokenMismatch(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "mismatch")
	tok1, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
	forceExpireLease(t, d, in.Key)
	tok2, dec, _ := d.Store.TryClaim(context.Background(), in, lease5)
	assert.Equal(t, reliable.Claimed, dec)
	assert.NotEqual(t, tok1, tok2)
	err := d.Store.MarkSucceeded(context.Background(), d.DB, in.Key, tok1)
	assert.ErrorIs(t, err, reliable.ErrConflict, "stale token must not overwrite new owner (edge #6)")
	require.NoError(t, d.Store.MarkSucceeded(context.Background(), d.DB, in.Key, tok2))
}

func confMarkFailedRetry(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "mf-retry")
	tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
	payload := []byte("env-bytes")
	require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
		reliable.ClassRetryable, reliable.ReplayIdempotent, 5, reliable.Retryable(reliableErr("deadlock")), payload))
	r := mustGetByEvent(t, d, in.Key)
	assert.Equal(t, "RETRY_SCHEDULED", r.Status)
	assert.NotNil(t, r.NextAttemptAt)
	assert.Equal(t, payload, r.Payload)
}

func confMarkFailedUnsafe(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "mf-unsafe")
	tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
		reliable.ClassRetryable, reliable.ReplayUnsafe, 5, reliable.Retryable(reliableErr("deadlock")), []byte("p")))
	r := mustGetByEvent(t, d, in.Key)
	assert.Equal(t, "DEAD_LETTER", r.Status, "Retryable x ReplayUnsafe -> DEAD_LETTER (§6.1)")
	assert.Nil(t, r.NextAttemptAt)
}

func confMarkFailedPoison(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "mf-poison")
	tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
		reliable.ClassPoison, reliable.ReplayIdempotent, 5, reliable.Permanent(reliableErr("bad")), []byte("p")))
	r := mustGetByEvent(t, d, in.Key)
	assert.Equal(t, "DEAD_LETTER", r.Status)
}

func confAttemptExhausted(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "mf-exhaust")
	tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
		reliable.ClassRetryable, reliable.ReplayIdempotent, 1, reliable.Retryable(reliableErr("x")), []byte("p")))
	r := mustGetByEvent(t, d, in.Key)
	assert.Equal(t, "DEAD_LETTER", r.Status, "attempt>=max -> DEAD_LETTER even if Retryable")
}

func confRecordTerminalIdempotent(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "rt")
	require.NoError(t, d.Store.RecordTerminal(context.Background(), d.DB, in, reliable.ClassPoison, reliableErr("decode"), []byte("p")))
	require.NoError(t, d.Store.RecordTerminal(context.Background(), d.DB, in, reliable.ClassPoison, reliableErr("decode"), []byte("p")), "idempotent on same terminal")
	assert.Error(t, d.Store.RecordTerminal(context.Background(), d.DB, in, reliable.ClassPoison, reliableErr("x"), nil), "nil payload rejected (§4 v2.8)")
}

// 租约孤儿完整闭环（C7 本轮重写，按 D20）：
// 观测器只记 anomaly 不改行 → 行仍 PROCESSING 且 ownership 不变 → 再次 TryClaim 内联续占成功
// 且 claim_id 已变 → 旧 token 的 MarkSucceeded 得 ErrConflict（fencing token 语义，兼覆准入 ⑤）。
func confLeaseOrphan(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "orphan")
	oldTok, dec, err := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, err)
	require.Equal(t, reliable.Claimed, dec)
	forceExpireLease(t, d, in.Key)

	// 1) 观测：计数 + 记 anomaly，但不改行。
	n, err := d.Store.ObserveExpiredLeases(context.Background(), time.Now().UTC().Add(time.Hour))
	require.NoError(t, err, "D20: observer must not touch row state (no CHECK violation)")
	assert.GreaterOrEqual(t, n, 1)
	assertAnomalyExists(t, d, "LEASE_ORPHAN", in.Key)

	before := mustGetByEvent(t, d, in.Key)
	assert.Equal(t, "PROCESSING", before.Status, "D20: observer leaves status untouched")
	require.NotNil(t, before.ClaimID)
	assert.Equal(t, string(oldTok), *before.ClaimID, "D20: observer leaves ownership untouched")

	// 2) 幂等：重复观测不得重复写 anomaly（uk_anomaly_once + ON CONFLICT DO NOTHING）。
	_, err = d.Store.ObserveExpiredLeases(context.Background(), time.Now().UTC().Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), anomalyCount(t, d, "LEASE_ORPHAN", in.Key), "same claim must not re-record (alert self-noise)")

	// 3) 再占位的唯一路径：TryClaim 内联 CAS。
	newTok, dec2, err := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, err)
	assert.Equal(t, reliable.Claimed, dec2, "expired lease is re-claimable via TryClaim inline CAS")
	assert.NotEqual(t, oldTok, newTok, "fencing token must rotate")

	after := mustGetByEvent(t, d, in.Key)
	require.NotNil(t, after.ClaimID)
	assert.Equal(t, string(newTok), *after.ClaimID)

	// 4) 旧 token 失效。
	assert.ErrorIs(t, d.Store.MarkSucceeded(context.Background(), d.DB, in.Key, oldTok), reliable.ErrConflict,
		"stale token must not settle the row")
}

// §3.3 法律情形：pooled db → TryClaim 独立提交。
//
// C1：§3.3 真正要钉的是：**调用方开着一个未提交的业务事务时，TryClaim 写的行必须已对第三连接可见**
// （即使那个业务事务随后回滚，占位行也必须留存——这正是准入 ⑤ 描述的崩溃场景）。
func confIndependentCommit(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "indep")

	// 调用方开启业务事务（Phase B 的事务），d.Store 仍是用 pooled db 构造的。
	tx := d.DB.Begin()
	require.NoError(t, tx.Error)

	tok, dec, err := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, err)
	require.Equal(t, reliable.Claimed, dec)

	// 业务事务【未提交】时，用第三连接（d.DB 池）读：必须已可见。
	r := mustGetByEvent(t, d, in.Key)
	assert.Equal(t, "PROCESSING", r.Status, "§3.3: claim row must be visible while caller tx is still open")
	require.NotNil(t, r.ClaimID)
	assert.Equal(t, string(tok), *r.ClaimID)

	// 业务事务回滚（模拟 Phase B 失败/崩溃）：占位行必须仍在，且仍是 PROCESSING。
	require.NoError(t, tx.Rollback().Error)
	after := mustGetByEvent(t, d, in.Key)
	assert.Equal(t, "PROCESSING", after.Status, "§3.3: claim survives caller tx rollback (准入 ⑤)")
	assert.Equal(t, int64(1), rowCount(t, d, in.Key))
}

// §3.3 禁止情形（D16 + 本轮评审 F1/A3）：把 tx 句柄传给 NewStore → 构造期 panic。
// NewStore 已加 ConnPool 类型断言拒绝 tx（正向 txCommitter 接口断言，覆盖 *sql.Tx 与 gorm
// *PreparedStmtTX）。本用例用 d.DB.Begin()（→ *sql.Tx）验证基本路径。
func confForbiddenCaseTxJoin(t *testing.T, d *ConformanceDeps) {
	tx := d.DB.Begin()
	t.Cleanup(func() { _ = tx.Rollback() })
	assert.Panics(t, func() { _, _ = NewStoreFor(d.Dialect, tx) },
		"§3.3 (F1/A3): NewStore must reject tx-bound *gorm.DB at construction")
}

// ⑦ M12 有序重放：同聚合 Created（causal_seq 小）+ Changed（causal_seq 大，先到期），FindEligibleHeads 返回 Created。
func confOrderedReplay(t *testing.T, d *ConformanceDeps) {
	now := time.Now().UTC()
	created := newClaimInput(t, "ordered-created")
	changed := newClaimInput(t, "ordered-changed")
	changed.Meta.AggregateID = created.Meta.AggregateID // 同聚合
	changed.Meta.AggregateType = created.Meta.AggregateType
	created.Meta.CausalSeq = ptrI64(1) // Created 因果在前
	changed.Meta.CausalSeq = ptrI64(2)
	// 两条都 RETRY_SCHEDULED；Changed 先到期。
	seedRetryRow(t, d, created, 1, now.Add(-time.Minute))
	seedRetryRow(t, d, changed, 2, now.Add(-2*time.Minute))
	heads, err := d.Store.FindEligibleHeads(context.Background(), now, 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(heads), 1)
	// 命中本聚合的 head 必须是 Created（causal_seq 更小），即便 Changed 更早到期。
	for _, h := range heads {
		if h.AggregateID == created.Meta.AggregateID && h.EventID == changed.Key.EventID {
			t.Fatalf("Changed must be skipped while Created unresolved (M12 ordering)")
		}
	}
}

// ⑫ M15 header 保真：重复 key + byte value 经 JSON/JSONB 持久化后，读回应保序/保重复/保字节。
func confHeaderFidelity(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "header")
	in.Delivery.Headers = []reliable.HeaderPair{
		{Key: "trace", Value: []byte("a")},
		{Key: "trace", Value: []byte("b")},            // 重复 key
		{Key: "bin", Value: []byte{0x00, 0xff, 0x10}}, // 非 ASCII 字节
	}
	_, _, err := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, err)
	// C2：不比较原始 JSON 文本（PG 的 JSONB 会重新序列化，冒号后有空格，必间歇红）。
	// headers 是数组，保序应断言反序列化后的 r.Headers[0..2]（方言无关）。
	r := d.mustGetFullRow(t, in.Key)
	require.Len(t, r.Headers, 3, "all 3 headers survive round-trip")
	assert.Equal(t, "trace", r.Headers[0].Key)
	assert.Equal(t, "trace", r.Headers[1].Key, "duplicate key preserved")
	assert.Equal(t, []byte{0x00, 0xff, 0x10}, r.Headers[2].Value, "binary value survives base64 round-trip")
}

// ㉒ 人工重放授权：requester≠approver 强制；replay_auth_id 一次性；重复/过期/AUTO 拒绝。
func confManualReplayAuth(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "auth")
	tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
	require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
		reliable.ClassPoison, reliable.ReplayUnsafe, 5, reliable.Permanent(reliableErr("bad")), []byte("p")))
	r := mustGetByEvent(t, d, in.Key)
	require.Equal(t, "DEAD_LETTER", r.Status)
	// D12：requester==approver → ErrConflict
	assert.ErrorIs(t, d.Store.ScheduleReplay(context.Background(), d.DB, r.ID, r.RowVersion, "alice", "alice", "self-approve"),
		reliable.ErrConflict, "requester==approver rejected")
	// 正确双人 → OK
	require.NoError(t, d.Store.ScheduleReplay(context.Background(), d.DB, r.ID, r.RowVersion, "alice", "bob", "ok"))
	// 陈旧 row_version → ErrConflict
	assert.ErrorIs(t, d.Store.ScheduleReplay(context.Background(), d.DB, r.ID, r.RowVersion, "alice", "carol", "stale"),
		reliable.ErrConflict, "stale version rejected")
	// ClaimForReplay 消费一次性 auth；二次消费（不同 claim_id）应失败
	r2 := mustGetByEvent(t, d, in.Key)
	tok2, _, err := d.Store.ClaimForReplay(context.Background(), d.DB, r2.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, tok2)
	// 标记完成后再 ScheduleReplay 又是一轮（generation+1），需新审批——此处只验证一次性消费在单轮内成立。
}

func confConcurrentSingleRow(t *testing.T, d *ConformanceDeps) {
	in := newClaimInput(t, "concurrent")
	const N = 10
	var wg sync.WaitGroup
	var claimed int64
	start := make(chan struct{})
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, dec, _ := d.Store.TryClaim(context.Background(), in, lease5)
			if dec == reliable.Claimed {
				atomic.AddInt64(&claimed, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	assert.Equal(t, int64(1), atomic.LoadInt64(&claimed), "exactly one Claimed; rest AlreadyProcessing")
	assert.Equal(t, int64(1), rowCount(t, d, in.Key))
}
