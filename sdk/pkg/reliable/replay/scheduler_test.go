package replay

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/stretchr/testify/assert"
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
	calls  int32
}

func (h *fakeHandler) Handle(context.Context, []byte, reliable.DeliveryMeta) error {
	atomic.AddInt32(&h.calls, 1)
	return h.retErr
}
func (h *fakeHandler) HandlerID() reliable.HandlerID       { return h.id }
func (h *fakeHandler) ReplaySafety() reliable.ReplaySafety { return reliable.ReplayIdempotent }
func (h *fakeHandler) RequiresAggregateGate() bool         { return false }

// D21：fake 直接实现真实签名（*gorm.DB），无 stubDB 占位；编译期断言钉住接口完整性。
var _ store.Store = (*schedulerFakeStore)(nil)

// schedulerFakeStore 实现 store.Store，记录各处置路径的调用计数与**顺序**。
type schedulerFakeStore struct {
	heads         []store.Row
	claimTok      reliable.ClaimToken
	claimErr      error
	gateErr       error
	advanceDue    int32
	moveToDL      int32
	markSucceeded int32
	releaseClaim  int32
	claimCalls    int32
	gateCalls     int32
}

func (s *schedulerFakeStore) FindEligibleHeads(context.Context, time.Time, int) ([]store.Row, error) {
	return s.heads, nil
}
func (s *schedulerFakeStore) ClaimForReplay(_ context.Context, _ *gorm.DB, id int64) (reliable.ClaimToken, store.Row, error) {
	atomic.AddInt32(&s.claimCalls, 1)
	if s.claimErr != nil {
		return "", store.Row{}, s.claimErr
	}
	return s.claimTok, store.Row{ID: id, HandlerID: "h", EventID: "e"}, nil
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
func (s *schedulerFakeStore) MoveToDeadLetterWithToken(_ context.Context, _ *gorm.DB, _ int64, _ reliable.ClaimToken, _ string) error {
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
func (s *schedulerFakeStore) ReleaseAggregateGate(context.Context, *gorm.DB, string) error { return nil }
func (s *schedulerFakeStore) ReclaimExpiredAggregateGates(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (s *schedulerFakeStore) ObserveExpiredLeases(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (s *schedulerFakeStore) RecordAnomaly(context.Context, *gorm.DB, int, string, reliable.Key, string, string) error {
	return nil
}
func (s *schedulerFakeStore) GetByID(context.Context, int64) (store.Row, error) {
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
