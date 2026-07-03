package outbox

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// recordingFlush 是纯 happy-path 记录器：记录每次 flush 收到的 ID，便于断言。
// 失败注入由测试内联闭包（failAlways/flex）负责，不在本类型里（A3 精简）。
type recordingFlush struct {
	mu    sync.Mutex
	calls [][]string
}

func (r *recordingFlush) flush(_ context.Context, ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := make([]string, len(ids))
	copy(snap, ids)
	r.calls = append(r.calls, snap)
	return nil
}

func (r *recordingFlush) allCalls() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]string, len(r.calls))
	copy(out, r.calls)
	return out
}

var errBoom = fmt.Errorf("simulated flush failure")

// collectingOnError 收集 onError 收到的告警，便于断言。
type collectingOnError struct {
	mu   sync.Mutex
	msgs []string
}

func (c *collectingOnError) handler(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.msgs = append(c.msgs, err.Error())
	}
}

func (c *collectingOnError) messages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.msgs))
	copy(out, c.msgs)
	return out
}

// === Happy path ===

// TestBatcher_FillAtK_TriggersFlush：满 K 立即 flush，缓冲清空。
func TestBatcher_FillAtK_TriggersFlush(t *testing.T) {
	rf := &recordingFlush{}
	b := newAckMarkerBatcher(3, 50*time.Millisecond, 5, rf.flush, nil)
	defer b.Close()

	for _, id := range []string{"a", "b", "c"} {
		b.Add(id)
	}
	require.Eventually(t, func() bool { return len(rf.allCalls()) >= 1 },
		200*time.Millisecond, 5*time.Millisecond, "fill-at-K should flush")
	calls := rf.allCalls()
	require.Len(t, calls[0], 3)
	require.Equal(t, []string{"a", "b", "c"}, calls[0])
}

// TestBatcher_TimerFlush_BelowK：不足 K，等 ticker flush。
func TestBatcher_TimerFlush_BelowK(t *testing.T) {
	rf := &recordingFlush{}
	b := newAckMarkerBatcher(50, 20*time.Millisecond, 5, rf.flush, nil)
	defer b.Close()

	b.Add("x")
	require.Eventually(t, func() bool { return len(rf.allCalls()) >= 1 },
		200*time.Millisecond, 5*time.Millisecond, "timer should flush below K")
	require.Equal(t, []string{"x"}, rf.allCalls()[0])
}

// TestBatcher_EmptyFlush_Noop：空缓冲触发 flush 时不调 flushFunc。
func TestBatcher_EmptyFlush_Noop(t *testing.T) {
	rf := &recordingFlush{}
	b := newAckMarkerBatcher(50, 20*time.Millisecond, 5, rf.flush, nil)
	time.Sleep(60 * time.Millisecond) // 等一个以上 ticker 周期（空缓冲 flush 应 no-op）
	require.Empty(t, rf.allCalls(), "empty buffer must not invoke flushFunc")
	require.NoError(t, b.Close())
}

// TestBatcher_Close_FlushesRemaining：关停时冲刷剩余缓冲。
func TestBatcher_Close_FlushesRemaining(t *testing.T) {
	rf := &recordingFlush{}
	b := newAckMarkerBatcher(50, 5*time.Second, 5, rf.flush, nil) // 长 ticker，确保只有 Close 触发
	b.Add("r1")
	b.Add("r2")
	require.NoError(t, b.Close())
	calls := rf.allCalls()
	require.Len(t, calls, 1, "Close must flush remaining exactly once")
	require.Equal(t, []string{"r1", "r2"}, calls[0])
}

// TestBatcher_AddAfterClose_Discarded：Close 后 Add 静默丢弃，不 panic。
func TestBatcher_AddAfterClose_Discarded(t *testing.T) {
	rf := &recordingFlush{}
	b := newAckMarkerBatcher(50, 5*time.Second, 5, rf.flush, nil)
	require.NoError(t, b.Close())
	require.NotPanics(t, func() { b.Add("late") })
	require.Empty(t, rf.allCalls(), "Add after Close must be discarded")
}

// === Failure ceiling (§9.1) ===
//
// 注意：drop-on-failure（Decision 6 / plan A7）下，每次失败 flush 后缓冲清空，
// 空缓冲的 ticker flush 是 no-op。因此这些用例必须**逐条 Add 驱动多次 flush**，
// 不能像 re-buffer 语义那样 Add 一次后靠 ticker 累计失败次数。

// addAndWaitFlushing 驱动一次 Add 并等待对应 flush 完成（calls 自增到 want）。
func addAndWaitFlushing(t *testing.T, b *ackMarkerBatcher, id string, calls *int32) {
	t.Helper()
	want := atomic.LoadInt32(calls) + 1
	b.Add(id)
	require.Eventually(t, func() bool { return atomic.LoadInt32(calls) >= want },
		300*time.Millisecond, time.Millisecond, "flush for %q should fire", id)
}

// waitForMsgs waits until at least n alerts have been emitted, bridging the
// gap between flushFunc completion (calls) and the alert that fires after
// fails++ later in the same flush.
func waitForMsgs(t *testing.T, onErr *collectingOnError, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return len(onErr.messages()) >= n },
		300*time.Millisecond, time.Millisecond, "expected >= %d alert(s)", n)
}

// TestBatcher_FailureCeiling_AlertsAtThreshold：threshold=3，连续失败 3 次才告警一次。
func TestBatcher_FailureCeiling_AlertsAtThreshold(t *testing.T) {
	var calls int32
	failAlways := func(_ context.Context, _ []string) error { atomic.AddInt32(&calls, 1); return errBoom }
	onErr := &collectingOnError{}
	b := newAckMarkerBatcher(1, 5*time.Second, 3, failAlways, onErr.handler) // 长 ticker：只靠 size-K 触发
	defer b.Close()

	addAndWaitFlushing(t, b, "e1", &calls) // fails=1（静默）
	addAndWaitFlushing(t, b, "e2", &calls) // fails=2（静默）
	addAndWaitFlushing(t, b, "e3", &calls) // fails=3 → 告警

	waitForMsgs(t, onErr, 1)
	msgs := onErr.messages()
	require.Len(t, msgs, 1, "alert only at fails%%threshold==0 (3), not at 1 or 2")
	require.Contains(t, msgs[0], "persistent outbox mark failure")
	require.Contains(t, msgs[0], "3 consecutive")
}

// TestBatcher_FailureCeiling_NoSpamEveryThresholdMultiple：threshold=2，fails=2,4 各告警一次。
func TestBatcher_FailureCeiling_NoSpamEveryThresholdMultiple(t *testing.T) {
	var calls int32
	failAlways := func(_ context.Context, _ []string) error { atomic.AddInt32(&calls, 1); return errBoom }
	onErr := &collectingOnError{}
	b := newAckMarkerBatcher(1, 5*time.Second, 2, failAlways, onErr.handler)
	defer b.Close()

	for i := 0; i < 5; i++ {
		addAndWaitFlushing(t, b, fmt.Sprintf("e%d", i), &calls) // fails=1..5；告警在 2、4
	}
	waitForMsgs(t, onErr, 2)
	msgs := onErr.messages()
	require.Len(t, msgs, 2, "alert at every threshold multiple (2,4), no per-failure spam")
}

// TestBatcher_SuccessResetsCounter：成功重置计数；再次累计阈值才告警。
func TestBatcher_SuccessResetsCounter(t *testing.T) {
	var (
		stateMu sync.Mutex
		fail    = true
		calls   int32
	)
	flex := func(_ context.Context, _ []string) error {
		atomic.AddInt32(&calls, 1)
		stateMu.Lock()
		f := fail
		stateMu.Unlock()
		if f {
			return errBoom
		}
		return nil
	}
	onErr := &collectingOnError{}
	b := newAckMarkerBatcher(1, 5*time.Second, 3, flex, onErr.handler)
	defer b.Close()

	// Phase 1：3 次失败 → 1 次告警（fails=3）
	addAndWaitFlushing(t, b, "f1", &calls)
	addAndWaitFlushing(t, b, "f2", &calls)
	addAndWaitFlushing(t, b, "f3", &calls)
	waitForMsgs(t, onErr, 1)
	require.Len(t, onErr.messages(), 1, "alert after 3 consecutive failures")

	// Phase 2：1 次成功 → fails 归零
	stateMu.Lock()
	fail = false
	stateMu.Unlock()
	addAndWaitFlushing(t, b, "s1", &calls)

	// Phase 3：再 2 次失败 → 还不应告警（重置后需满 3）
	stateMu.Lock()
	fail = true
	stateMu.Unlock()
	addAndWaitFlushing(t, b, "g1", &calls)
	addAndWaitFlushing(t, b, "g2", &calls)
	waitForMsgs(t, onErr, 1)
	require.Len(t, onErr.messages(), 1, "no new alert after only 2 failures post-reset")

	// 第 3 次失败 → 第二次告警
	addAndWaitFlushing(t, b, "g3", &calls)
	waitForMsgs(t, onErr, 2)
	require.Len(t, onErr.messages(), 2, "second alert after 3rd failure post-reset")
}

// TestBatcher_ThresholdZero_PerFailure：threshold=0 = 每次失败都告警（baseline）。
func TestBatcher_ThresholdZero_PerFailure(t *testing.T) {
	var calls int32
	failAlways := func(_ context.Context, _ []string) error { atomic.AddInt32(&calls, 1); return errBoom }
	onErr := &collectingOnError{}
	b := newAckMarkerBatcher(1, 5*time.Second, 0, failAlways, onErr.handler)
	defer b.Close()

	for i := 0; i < 3; i++ {
		addAndWaitFlushing(t, b, fmt.Sprintf("e%d", i), &calls)
	}
	waitForMsgs(t, onErr, int(atomic.LoadInt32(&calls)))
	require.Equal(t, int(atomic.LoadInt32(&calls)), len(onErr.messages()),
		"threshold=0 must alert every failure")
}

// === Concurrency (-race) ===

// TestBatcher_ConcurrentAdd_RaceClean：多 goroutine 并发 Add + ticker flush + Close，
// -race 下无数据竞争/panic，且不丢不重。
func TestBatcher_ConcurrentAdd_RaceClean(t *testing.T) {
	rf := &recordingFlush{}
	b := newAckMarkerBatcher(10, 5*time.Millisecond, 3, rf.flush, func(error) {})

	const writers = 8
	const perWriter = 200
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				b.Add(fmt.Sprintf("w%d-%d", w, i))
			}
		}(w)
	}
	wg.Wait()
	require.NoError(t, b.Close()) // 若内部有 race，-race 在此暴露

	total := 0
	for _, c := range rf.allCalls() {
		total += len(c)
	}
	require.Equal(t, writers*perWriter, total, "no Add may be lost; every ID must be flushed exactly once")
}

// === Panic recovery (Fix 1) ===

// TestBatcher_FlushFuncPanic_LoopSurvives：flushFunc panic 必须被 recover 转为 error，
// 走正常的 fails/onError 路径；loop goroutine 存活，后续 Add+flush 正常工作。
// threshold=1 → panic-error 立即告警，断言 onError 收到 "flushFunc panic"。
func TestBatcher_FlushFuncPanic_LoopSurvives(t *testing.T) {
	var (
		stateMu   sync.Mutex
		willPanic = true
		calls     int32
		panicVal  = "distinct-driver-bug-94217"
		rf        recordingFlush
	)
	flex := func(_ context.Context, ids []string) error {
		atomic.AddInt32(&calls, 1)
		stateMu.Lock()
		wp := willPanic
		stateMu.Unlock()
		if wp {
			panicpanic(panicVal)
		}
		return rf.flush(context.Background(), ids)
	}
	onErr := &collectingOnError{}
	// threshold=0 → 每次失败都告警，且 alert 文案用 %w 包裹 err，能直接断言 panic 文本。
	b := newAckMarkerBatcher(1, 5*time.Second, 0, flex, onErr.handler) // K=1：每 Add 立即 flush
	defer b.Close()

	// 1st Add → flushFunc panics → recover 转 error → 走 fails++/onError（threshold=0 立即告警，
	// 文案含 "flushFunc panic" + 原始 panic 值）。
	addAndWaitFlushing(t, b, "p1", &calls)
	waitForMsgs(t, onErr, 1)
	{
		msgs := onErr.messages()
		require.Len(t, msgs, 1, "panic-converted error must alert (threshold=0)")
		require.Contains(t, msgs[0], "flushFunc panic", "alert must surface the recovered panic")
		require.Contains(t, msgs[0], panicVal, "alert must carry the original panic value")
	}

	// 2nd Add → flushFunc 不再 panic → 成功 flush；证明 loop 存活。
	stateMu.Lock()
	willPanic = false
	stateMu.Unlock()
	addAndWaitFlushing(t, b, "s1", &calls)
	require.Eventually(t, func() bool { return len(rf.allCalls()) >= 1 },
		300*time.Millisecond, time.Millisecond, "loop survived: post-panic flush must succeed")
	calls0 := rf.allCalls()
	require.Equal(t, []string{"s1"}, calls0[len(calls0)-1], "the post-panic flush recorded s1")
}

// panicpanic is a tiny indirection so the panic site is a real call (not inline),
// keeping the recover path realistic.
func panicpanic(v string) { panic(v) }

// === Reentrant self-deadlock guard (async+recover onError) ===

// TestBatcher_OnErrorCallingClose_NoDeadlock verifies the async+recover fix:
// if onError (→ publisher.ErrorHandler) calls batcher.Close() — exactly what
// StopACKListener does — the loop goroutine must NOT be stuck inside a
// synchronous onError while Close blocks on <-b.exited. Before the fix
// (synchronous onError), this self-deadlocked permanently: Close waited for
// the loop to exit, but the loop was blocked inside onError→Close.
func TestBatcher_OnErrorCallingClose_NoDeadlock(t *testing.T) {
	var calls int32
	failAlways := func(_ context.Context, _ []string) error { atomic.AddInt32(&calls, 1); return errBoom }

	closeDone := make(chan struct{})
	var b *ackMarkerBatcher
	onErr := func(error) {
		// Simulate a user ErrorHandler that calls StopACKListener -> batcher.Close().
		_ = b.Close()
		close(closeDone)
	}
	b = newAckMarkerBatcher(1, 5*time.Second, 1, failAlways, onErr)

	b.Add("e1") // flush fails -> fails=1, threshold=1 -> alert -> onError

	select {
	case <-closeDone:
		// success: with async onError, the loop spawned onErr in a goroutine and
		// moved on; onErr's Close() drained + waited on <-b.exited, which fired
		// once the loop exited. No deadlock.
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock: onError -> Close blocked on <-b.exited (loop stuck in synchronous onError)")
	}
}
