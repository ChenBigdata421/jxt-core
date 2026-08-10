package gate

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// gateFakeStore implements the gate package's narrow acquire/release dependency.
//
// ReleaseAggregateGate 在调用瞬间快照 ctx 是否存活——不能事后查 ctx：release 闭包返回后独立 ctx
// 立刻被 relCancel() 取消，事后查必为 canceled 而误报。模拟真实 GORM：ctx 已取消 → DELETE 立即失败。

type gateFakeStore struct {
	acquireCalls      int32
	releaseCalls      int32
	releaseCtxAlive   bool
	acquireErr        error // 强制 acquire 失败（测让路 / DB 错误分支）
	releaseErr        error // 强制 release 失败（测错误传播）
	releaseTokenSeen  string
	acquireKeySeen    reliable.AggregateGateKey
	acquireHolderSeen string
	acquireTTLSeen    time.Duration
}

func (s *gateFakeStore) AcquireAggregateGate(_ context.Context, _ *gorm.DB, key reliable.AggregateGateKey, holder string, ttl time.Duration) (string, error) {
	atomic.AddInt32(&s.acquireCalls, 1)
	s.acquireKeySeen = key
	s.acquireHolderSeen = holder
	s.acquireTTLSeen = ttl
	if s.acquireErr != nil {
		return "", s.acquireErr
	}
	return "gate-token", nil
}

func (s *gateFakeStore) ReleaseAggregateGate(ctx context.Context, _ *gorm.DB, token string) error {
	atomic.AddInt32(&s.releaseCalls, 1)
	s.releaseTokenSeen = token
	s.releaseCtxAlive = ctx.Err() == nil
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.releaseErr != nil {
		return s.releaseErr
	}
	return nil
}

func nonEmptyKey() reliable.AggregateGateKey {
	return reliable.AggregateGateKey{TenantID: 1, AggregateType: "Media", AggregateID: "agg-1"}
}

// (a) acquire→release 顺序：Acquire 触发 AcquireAggregateGate 一次、release 尚未调用；
// 调 release() 后 ReleaseAggregateGate 调一次。无 fn 回调——caller 在 Acquire 与 release 之间自办业务。
func TestGate_AcquireThenRelease_OrderAndArgs(t *testing.T) {
	fs := &gateFakeStore{}
	ttl := 5 * time.Minute
	holder := "replay-42"
	release, err := Acquire(context.Background(), fs, nil, nonEmptyKey(), holder, ttl)

	require.NoError(t, err)
	require.NotNil(t, release, "release closure must be non-nil on success")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.acquireCalls), "AcquireAggregateGate called once")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.releaseCalls), "release not yet called")
	assert.Equal(t, "", fs.releaseTokenSeen, "no release yet → token unseen")
	assert.Equal(t, nonEmptyKey(), fs.acquireKeySeen)
	assert.Equal(t, holder, fs.acquireHolderSeen)
	assert.Equal(t, ttl, fs.acquireTTLSeen)

	assert.NoError(t, release())
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseCalls), "ReleaseAggregateGate called once after release()")
	assert.Equal(t, "gate-token", fs.releaseTokenSeen, "release must pass the acquired token")
}

// (b) release() returns the ReleaseAggregateGate error (propagates to caller) —— 这正是 replay
// 用来 raise REPLAY_GATE_RELEASE_FAILED 的属性。
func TestGate_Release_PropagatesError(t *testing.T) {
	fs := &gateFakeStore{releaseErr: errors.New("db down")}
	release, err := Acquire(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, release)

	errRelease := release()
	require.Error(t, errRelease)
	assert.Equal(t, "db down", errRelease.Error(), "release must surface ReleaseAggregateGate error")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseCalls), "release still called even though it errors")
}

// (c) 业务 ctx 取消后 release 仍成功：release 用自己的独立 ctx，不继承业务 ctx。
// 驱动一个 live ctx 拿到 release 闭包（acquire 用 live ctx 完成），取消后调 release()，
// 断言 ReleaseAggregateGate 收到的是独立未取消 ctx。
func TestGate_Release_UsesIndependentContext(t *testing.T) {
	fs := &gateFakeStore{}
	ctx, cancel := context.WithCancel(context.Background())
	release, err := Acquire(ctx, fs, nil, nonEmptyKey(), "h", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, release)

	cancel() // 模拟 tickTimeout fire / 上层取消：业务 ctx 已 done

	assert.NoError(t, release(), "release must succeed via its own ctx despite canceled business ctx")
	require.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseCalls), "release called")
	assert.True(t, fs.releaseCtxAlive,
		"release must use an independent non-cancelled context, not the business ctx")
}

// (d) key.Empty() → 既不 acquire 也不 release，err==nil，且 release 是 non-nil 的 no-op。
func TestGate_EmptyKey_NoopRelease(t *testing.T) {
	fs := &gateFakeStore{}
	emptyKey := reliable.AggregateGateKey{} // AggregateType="" → Empty()==true
	release, err := Acquire(context.Background(), fs, nil, emptyKey, "h", time.Minute)

	require.NoError(t, err, "empty key must not error")
	require.NotNil(t, release, "empty key must still return a non-nil no-op release")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.acquireCalls), "empty key must not acquire")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.releaseCalls), "no acquire → no release expected yet")

	assert.NoError(t, release(), "no-op release returns nil")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.releaseCalls), "no-op release must NOT call ReleaseAggregateGate")
}

func TestGate_AcquireRejectsNonPositiveTTL(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		fs := &gateFakeStore{}
		release, err := Acquire(context.Background(), fs, nil, nonEmptyKey(), "h", ttl)

		require.ErrorIs(t, err, ErrInvalidLeaseTTL, "ttl=%s must be rejected", ttl)
		assert.Nil(t, release)
		assert.Equal(t, int32(0), atomic.LoadInt32(&fs.acquireCalls), "invalid ttl must not reach the store")
	}
}

// (e) acquire 失败 → 返回 (nil, err)：caller 不持有 release，无需 defer。
func TestGate_AcquireError_ReturnsNilRelease(t *testing.T) {
	fs := &gateFakeStore{acquireErr: reliable.ErrRetryLater}
	release, err := Acquire(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute)

	require.Error(t, err)
	assert.Nil(t, release, "on acquire error release must be nil (caller must not defer it)")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.acquireCalls), "AcquireAggregateGate attempted")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.releaseCalls), "no release on acquire failure")
}

// (e.b) acquire 真失败（非 contention）同样返回 nil release，错误透传。
func TestGate_AcquireDBError_PropagatesNonContention(t *testing.T) {
	dbErr := errors.New("connection refused")
	fs := &gateFakeStore{acquireErr: dbErr}
	release, err := Acquire(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute)
	require.ErrorIs(t, err, dbErr)
	assert.Nil(t, release)
}

// (f) IsContention：对 ErrRetryLater 为 true，对任意 DB 错误为 false（F9）。
func TestGate_IsContention(t *testing.T) {
	assert.True(t, IsContention(reliable.ErrRetryLater), "ErrRetryLater is contention")
	assert.True(t, IsContention(wrapErr(reliable.ErrRetryLater)), "wrapped ErrRetryLater is still contention")
	assert.False(t, IsContention(errors.New("connection refused")), "arbitrary DB error is not contention")
	assert.False(t, IsContention(nil), "nil is not contention")
}

// wrapErr produces a wrapped sentinel for the IsContention unwrapping test.
func wrapErr(err error) error { return &wrapped{cause: err} }

type wrapped struct{ cause error }

func (w *wrapped) Error() string { return "wrapped: " + w.cause.Error() }
func (w *wrapped) Unwrap() error { return w.cause }
