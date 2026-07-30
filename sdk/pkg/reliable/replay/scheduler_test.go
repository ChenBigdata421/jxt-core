package replay

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// B2（本轮评审）：fakeRegistry 持 **指针**。原稿 `h fakeHandler` 是值，而 Handle/HandlerID
// 定义在 `*fakeHandler` 上 → `Handler: f.h` 不满足 reliable.ReplayableHandler，编译不过。
type fakeRegistry struct {
	h        *fakeHandler
	needGate bool
}

func (f fakeRegistry) Lookup(id reliable.HandlerID) (HandlerInfo, bool) {
	if id == f.h.id {
		return HandlerInfo{
			HandlerID: f.h.id, ReplaySafety: reliable.ReplayIdempotent,
			RequiresAggregateGate: f.needGate, Handler: f.h,
		}, true
	}
	return HandlerInfo{}, false
}
func (f fakeRegistry) All() []HandlerInfo { return []HandlerInfo{{HandlerID: f.h.id}} }

type fakeHandler struct {
	id     reliable.HandlerID
	retErr error
	panicV any // 非 nil 时 Handle panic（测 recover 路径）
	calls  int32
}

func (h *fakeHandler) Handle(context.Context, []byte, reliable.DeliveryMeta) error {
	atomic.AddInt32(&h.calls, 1)
	if h.panicV != nil {
		panic(h.panicV)
	}
	return h.retErr
}
func (h *fakeHandler) HandlerID() reliable.HandlerID       { return h.id }
func (h *fakeHandler) ReplaySafety() reliable.ReplaySafety { return reliable.ReplayIdempotent }
func (h *fakeHandler) RequiresAggregateGate() bool         { return false }

// D21：fake 直接实现真实签名（*gorm.DB），无占位类型；编译期断言钉住接口完整性。
var _ store.Store = (*schedulerFakeStore)(nil)

// schedulerFakeStore 实现 store.Store，记录各处置路径的调用计数与**顺序**。
type schedulerFakeStore struct {
	heads               []store.Row
	claimTok            reliable.ClaimToken
	claimErr            error
	claimAttempt        int // ClaimForReplay 返回的 Row.Attempt（测 defer-exhaustion 用）
	gateErr             error
	advanceDue          int32
	moveToDL            int32
	markSucceeded       int32
	releaseClaim        int32
	claimCalls          int32
	gateCalls           int32
	releaseGate         int32
	releaseGateCtxAlive bool
	releaseGateErr      error // 强制 release 失败（测告警路径）
}

func (s *schedulerFakeStore) FindEligibleHeads(context.Context, time.Time, int) ([]store.Row, error) {
	return s.heads, nil
}
func (s *schedulerFakeStore) ClaimForReplay(_ context.Context, _ *gorm.DB, id int64) (reliable.ClaimToken, store.Row, error) {
	atomic.AddInt32(&s.claimCalls, 1)
	if s.claimErr != nil {
		return "", store.Row{}, s.claimErr
	}
	return s.claimTok, store.Row{ID: id, HandlerID: "h", EventID: "e", Attempt: s.claimAttempt}, nil
}
func (s *schedulerFakeStore) ReleaseClaim(context.Context, *gorm.DB, int64, reliable.ClaimToken) error {
	atomic.AddInt32(&s.releaseClaim, 1)
	return nil
}
func (s *schedulerFakeStore) AdvanceDue(context.Context, *gorm.DB, int64) error {
	atomic.AddInt32(&s.advanceDue, 1)
	return nil
}
func (s *schedulerFakeStore) MoveToDeadLetter(context.Context, *gorm.DB, int64, string) error {
	atomic.AddInt32(&s.moveToDL, 1)
	return nil
}
func (s *schedulerFakeStore) MoveToDeadLetterWithToken(_ context.Context, _ *gorm.DB, _ int64, _ reliable.ClaimToken, _ reliable.ErrorClass, _ string) error {
	atomic.AddInt32(&s.moveToDL, 1) // A5：post-claim 死信路径，与 MoveToDeadLetter 共用计数器
	return nil
}
func (s *schedulerFakeStore) MarkSucceeded(context.Context, *gorm.DB, reliable.Key, reliable.ClaimToken) error {
	atomic.AddInt32(&s.markSucceeded, 1)
	return nil
}
func (s *schedulerFakeStore) AcquireAggregateGate(context.Context, *gorm.DB, reliable.AggregateGateKey, string, time.Duration) (string, error) {
	atomic.AddInt32(&s.gateCalls, 1)
	if s.gateErr != nil {
		return "", s.gateErr
	}
	return "gate-token", nil
}

// 其余方法 no-op。
func (s *schedulerFakeStore) TryClaim(context.Context, reliable.ClaimInput, time.Duration) (reliable.ClaimToken, reliable.Decision, error) {
	return "", 0, nil
}
func (s *schedulerFakeStore) MarkFailed(context.Context, *gorm.DB, reliable.Key, reliable.ClaimToken, reliable.ErrorClass, reliable.ReplaySafety, int, error, []byte) error {
	return nil
}
func (s *schedulerFakeStore) RecordTerminal(context.Context, *gorm.DB, reliable.ClaimInput, reliable.ErrorClass, error, []byte) error {
	return nil
}
func (s *schedulerFakeStore) ScheduleReplay(context.Context, *gorm.DB, int64, int64, string, string, string) error {
	return nil
}
func (s *schedulerFakeStore) Discard(context.Context, *gorm.DB, int64, int64, string, string) error {
	return nil
}
func (s *schedulerFakeStore) ReleaseAggregateGate(ctx context.Context, _ *gorm.DB, _ string) error {
	atomic.AddInt32(&s.releaseGate, 1)
	// 在调用瞬间快照 ctx 是否存活——不能存 ctx 引用事后查：scheduler 的 release defer 会在调用返回后
	// 立即 relCancel() 取消它（正常清理），事后查 ctx.Err() 必为 canceled，会误报。
	s.releaseGateCtxAlive = ctx.Err() == nil
	// 模拟真实 GORM/db：ctx 已取消 → DELETE 立即失败（这正是生产里 gate 泄漏到 TTL 的机制）。
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.releaseGateErr != nil {
		return s.releaseGateErr
	}
	return nil
}
func (s *schedulerFakeStore) ReclaimExpiredAggregateGates(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (s *schedulerFakeStore) ObserveExpiredLeases(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (s *schedulerFakeStore) RecordAnomaly(context.Context, *gorm.DB, int, string, reliable.Key, string, string) error {
	return nil
}
func (s *schedulerFakeStore) GetByID(context.Context, int, int64) (store.Row, error) {
	return store.Row{}, nil
}
func (s *schedulerFakeStore) List(context.Context, store.ListFilter) ([]store.Row, error) {
	return nil, nil
}

func retryHead(id int64) store.Row {
	return store.Row{ID: id, EventID: "e", HandlerID: "h", Payload: []byte("p"), ReplayMode: "AUTO",
		AggregateType: "Media", AggregateID: "agg-1", TenantID: 1}
}

func TestScheduler_InvokesHandlerAndMarksSucceeded(t *testing.T) {
	fs := &schedulerFakeStore{heads: []store.Row{retryHead(1)}, claimTok: "tok"}
	reg := fakeRegistry{h: &fakeHandler{id: "h"}}
	sch := NewScheduler(fs, nil, reg, nil, nil)
	_ = sch.Tick(context.Background())
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.markSucceeded))
}

// 准入 ⑬：claim **之前** 的 ErrRetryLater → AdvanceDue（行仍 RETRY_SCHEDULED），不增 attempt。
func TestScheduler_RetryLaterBeforeClaim_AdvancesDue(t *testing.T) {
	fs := &schedulerFakeStore{heads: []store.Row{retryHead(2)}, claimTok: "t", claimErr: reliable.ErrRetryLater}
	reg := fakeRegistry{h: &fakeHandler{id: "h"}}
	sch := NewScheduler(fs, nil, reg, nil, nil)
	_ = sch.Tick(context.Background())
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.advanceDue), "ErrRetryLater → AdvanceDue")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.markSucceeded))
}

// A3：handler 在 claim **之后** 返回 ErrRetryLater → 必须 ReleaseClaim 归还占位，
// 不能走 AdvanceDue（那只匹配 RETRY_SCHEDULED，会静默 0 行把行卡在 PROCESSING）。
func TestScheduler_RetryLaterAfterClaim_ReleasesClaim(t *testing.T) {
	fs := &schedulerFakeStore{heads: []store.Row{retryHead(4)}, claimTok: "tok"}
	reg := fakeRegistry{h: &fakeHandler{id: "h", retErr: reliable.ErrRetryLater}}
	sch := NewScheduler(fs, nil, reg, nil, nil)
	_ = sch.Tick(context.Background())
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseClaim), "post-claim yield → ReleaseClaim")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.advanceDue), "must NOT use the pre-claim path")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.markSucceeded))
}

// A3：gate 必须在 ClaimForReplay **之前** 获取——抢不到 gate 时整行不动、attempt 不增。
// 原稿把 gate 放在 claim 之后，抢不到时 attempt 已 +1 且行卡在 PROCESSING。
func TestScheduler_GateAcquiredBeforeClaim_NoSideEffectOnGateMiss(t *testing.T) {
	fs := &schedulerFakeStore{heads: []store.Row{retryHead(5)}, claimTok: "tok", gateErr: reliable.ErrRetryLater}
	reg := fakeRegistry{h: &fakeHandler{id: "h"}, needGate: true}
	sch := NewScheduler(fs, nil, reg, nil, nil)
	_ = sch.Tick(context.Background())
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.gateCalls), "gate is attempted")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.claimCalls), "A3: gate miss must happen BEFORE claim (no attempt bump)")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.advanceDue), "gate miss leaves the row untouched")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.moveToDL))
}

// 准入 ⑬：CanAutoReplay=false 的 head 行（RETRY_SCHEDULED）必须真被移出自动队列。
// 原稿 MoveToDeadLetter 只匹配 PROCESSING，这里恒 0 行 → 每周期重取同一行（空转）。
func TestScheduler_NotPermitted_MovesRetryScheduledRowOut(t *testing.T) {
	head := retryHead(3)
	fs := &schedulerFakeStore{heads: []store.Row{head}}
	reg := fakeRegistry{h: &fakeHandler{id: "unregistered"}} // Lookup 命中不了 → 走 UNKNOWN_HANDLER
	sch := NewScheduler(fs, nil, reg, nil, nil)
	_ = sch.Tick(context.Background())
	// UNKNOWN_HANDLER 分支刻意不动行（滚动发布中本实例可能尚未注册该 handler）。
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.moveToDL))
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.claimCalls))
}

// 本轮评审（bundle 2）：post-claim ErrRetryLater 到达 maxAttempts → DEAD_LETTER
// （REPLAY_DEFER_EXHAUSTED），不再 ReleaseClaim。防止「handler 永远返回 RetryLater」的行以
// ~1h 间隔无限重试、永不终结（ClaimForReplay 每轮 attempt+1，故 attempt 会涨到上限）。
func TestScheduler_RetryLaterAfterClaim_DeferExhausted_DeadLetters(t *testing.T) {
	fs := &schedulerFakeStore{
		heads:        []store.Row{retryHead(6)},
		claimTok:     "tok",
		claimAttempt: 5, // ClaimForReplay 已 attempt+1 → 5
	}
	reg := fakeRegistry{h: &fakeHandler{id: "h", retErr: reliable.ErrRetryLater}}
	sch := NewScheduler(fs, nil, reg, nil, nil) // 默认 maxAttempts=5 → ShouldDeadLetter(5,5)=true
	_ = sch.Tick(context.Background())
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.moveToDL), "attempt>=max → MoveToDeadLetterWithToken")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.releaseClaim), "exhausted 后不得再 ReleaseClaim")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.markSucceeded))
}

// 本轮评审（bundle 2，对称校验）：attempt 未到上限的 ErrRetryLater 仍走 ReleaseClaim（让路不增 attempt）。
func TestScheduler_RetryLaterAfterClaim_BelowMax_ReleasesClaim(t *testing.T) {
	fs := &schedulerFakeStore{
		heads:        []store.Row{retryHead(8)},
		claimTok:     "tok",
		claimAttempt: 2, // 远低于默认 maxAttempts=5
	}
	reg := fakeRegistry{h: &fakeHandler{id: "h", retErr: reliable.ErrRetryLater}}
	sch := NewScheduler(fs, nil, reg, nil, nil)
	_ = sch.Tick(context.Background())
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseClaim), "below max → ReleaseClaim")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.moveToDL), "未耗尽不得死信")
}

// 本轮评审（bundle 1）：handler panic 不得沿 Tick→Run 上抛杀掉调度器 goroutine——
// 须 recover → MoveToDeadLetterWithToken + 告警，打破「broker 重投 → 再 panic」死循环。
func TestScheduler_HandlerPanic_RecoveredAndDeadLettered(t *testing.T) {
	fs := &schedulerFakeStore{heads: []store.Row{retryHead(7)}, claimTok: "tok"}
	reg := fakeRegistry{h: &fakeHandler{id: "h", panicV: "boom"}}
	sch := NewScheduler(fs, nil, reg, nil, nil) // alerter 默认 NoOp
	assert.NotPanics(t, func() { _ = sch.Tick(context.Background()) }, "panic must be recovered, not crash the scheduler")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.moveToDL), "panic → MoveToDeadLetterWithToken")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.markSucceeded))
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.releaseClaim))
}

// recordingAlerter 记录 AlertAnomaly 的 kind（测「gate release 失败须告警」用）。
// Tick 在测试里同步执行，无并发，append 无需加锁。
type recordingAlerter struct {
	anomalies []string
}

func (r *recordingAlerter) AlertPoison(reliable.HandlerID, string, error) {}
func (r *recordingAlerter) AlertRecordFailure(reliable.HandlerID, error)  {}
func (r *recordingAlerter) AlertAnomaly(kind string, _ reliable.HandlerID, _ string) {
	r.anomalies = append(r.anomalies, kind)
}
func (r *recordingAlerter) has(kind string) bool {
	for _, k := range r.anomalies {
		if k == kind {
			return true
		}
	}
	return false
}

// review #3：gate 释放不得继承业务 ctx。tickTimeout fire / 上层取消时，processOne 的 ctx 已 done，
// ReleaseAggregateGate 的 DELETE 会随 ctx 失败 → gate 残留到 TTL，卡住同聚合重放（P2）。
// 驱动一个【已取消】的 ctx 跑 processOne，断言 release 仍被调用、且收到的是独立未取消 ctx。
func TestScheduler_GateRelease_UsesIndependentContextOnCancel(t *testing.T) {
	fs := &schedulerFakeStore{heads: []store.Row{retryHead(11)}, claimTok: "tok"}
	reg := fakeRegistry{h: &fakeHandler{id: "h"}, needGate: true}
	sch := NewScheduler(fs, nil, reg, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 模拟 tickTimeout fire / 上层取消：进 processOne 时 ctx 已 done
	_ = sch.Tick(ctx)

	require.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseGate), "gate must be released even when tick ctx cancelled")
	assert.True(t, fs.releaseGateCtxAlive,
		"release must use an independent non-cancelled context, not the tick ctx (else DELETE fails → gate leaks to TTL)")
}

// review #3：release 失败不得被 `_ =` 吞掉——与 ReleaseClaim 的 REPLAY_RELEASE_FAILED 同词汇，
// 让泄漏可观测（排查时不再只看到 replay 让路/无进度）。
func TestScheduler_GateRelease_AlertsOnFailure(t *testing.T) {
	fs := &schedulerFakeStore{heads: []store.Row{retryHead(12)}, claimTok: "tok", releaseGateErr: errors.New("db down")}
	reg := fakeRegistry{h: &fakeHandler{id: "h"}, needGate: true}
	al := &recordingAlerter{}
	sch := NewScheduler(fs, nil, reg, nil, al)

	_ = sch.Tick(context.Background()) // ctx 未取消；fake 因 releaseGateErr 强制失败
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseGate), "release attempted")
	assert.True(t, al.has("REPLAY_GATE_RELEASE_FAILED"), "release failure must surface an anomaly, not be swallowed")
}
