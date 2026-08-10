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

// liveFakeStore is a controllable fake store for RunLive tests.
// AcquireAggregateGate consumes errors from acquireErrs one per call (front to back);
// once the queue is exhausted (or nil) it returns success. This lets a test encode
// "fail N times then succeed" / "always fail" / "fail once with a DB error" scenarios.
//
// ReleaseAggregateGate snapshots whether its ctx is alive at call time — mirroring real
// GORM where a canceled ctx makes the DELETE fail immediately (cannot check after the
// release closure returns: it cancels its own independent ctx on the way out).

type liveFakeStore struct {
	acquireCalls    int32
	releaseCalls    int32
	acquireErrs     []error // queued errors, consumed one per acquire; exhausted/nil → success
	releaseErr      error
	releaseCtxAlive bool
}

func (s *liveFakeStore) AcquireAggregateGate(_ context.Context, _ *gorm.DB, _ reliable.AggregateGateKey, _ string, _ time.Duration) (string, error) {
	n := int(atomic.AddInt32(&s.acquireCalls, 1))
	if n <= len(s.acquireErrs) {
		return "", s.acquireErrs[n-1]
	}
	return "gate-token", nil
}

func (s *liveFakeStore) ReleaseAggregateGate(ctx context.Context, _ *gorm.DB, _ string) error {
	atomic.AddInt32(&s.releaseCalls, 1)
	s.releaseCtxAlive = ctx.Err() == nil
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.releaseErr
}

// failRecorder captures MarkFailedFn invocations and optionally returns a forced error.
type failRecorder struct {
	calls   int32
	classes []reliable.ErrorClass
	causes  []error
	retErr  error
}

func (f *failRecorder) fn(class reliable.ErrorClass, cause error) error {
	atomic.AddInt32(&f.calls, 1)
	f.classes = append(f.classes, class)
	f.causes = append(f.causes, cause)
	return f.retErr
}

// (a) non-empty key, acquired on the FIRST try → fn runs, release called, no sleep.
func TestRunLive_AcquireOnFirstTry_FnRunsNoSleep(t *testing.T) {
	fs := &liveFakeStore{}
	fr := &failRecorder{}
	spin := []time.Duration{50 * time.Millisecond}
	fnRan := false
	start := time.Now()
	err := RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute, spin, fr.fn, func() error {
		fnRan = true
		return nil
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.True(t, fnRan, "fn must run on first-try acquire")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fr.calls), "fail must not be called")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.acquireCalls), "exactly one acquire")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseCalls), "release called after fn")
	assert.Less(t, elapsed, spin[0], "no spin sleep on first-try acquire")
}

func TestRunLive_RejectsNonPositiveSpinDelay(t *testing.T) {
	for _, spin := range [][]time.Duration{{0}, {-time.Millisecond}} {
		fs := &liveFakeStore{}
		fr := &failRecorder{}
		err := RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute, spin, fr.fn, func() error {
			t.Fatal("fn must not run with invalid spin delay")
			return nil
		})

		require.ErrorIs(t, err, ErrInvalidSpinDelay, "spin=%v must be rejected", spin)
		assert.Equal(t, int32(0), atomic.LoadInt32(&fs.acquireCalls), "invalid spin delay must not acquire")
		assert.Equal(t, int32(0), atomic.LoadInt32(&fr.calls), "invalid spin delay must not park")
	}
}

func TestRunLive_EmptyKeyIgnoresSpinDelay(t *testing.T) {
	fs := &liveFakeStore{}
	fr := &failRecorder{}
	fnRan := false

	err := RunLive(context.Background(), fs, nil, reliable.AggregateGateKey{}, "h", time.Minute,
		[]time.Duration{0}, fr.fn, func() error {
			fnRan = true
			return nil
		})

	require.NoError(t, err)
	assert.True(t, fnRan, "empty key must bypass gate-only spin validation")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.acquireCalls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&fr.calls))
}

func TestRunLive_RejectsNonPositiveTTL(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		fs := &liveFakeStore{}
		fr := &failRecorder{}
		err := RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", ttl, nil, fr.fn, func() error {
			t.Fatal("fn must not run with invalid ttl")
			return nil
		})

		require.ErrorIs(t, err, ErrInvalidLeaseTTL, "ttl=%s must be rejected", ttl)
		assert.Equal(t, int32(0), atomic.LoadInt32(&fs.acquireCalls), "invalid ttl must not acquire")
		assert.Equal(t, int32(0), atomic.LoadInt32(&fr.calls), "invalid ttl must not park")
	}
}

func TestRunLive_AcquiredFnErrorPropagates(t *testing.T) {
	fnErr := errors.New("handler failed")
	fs := &liveFakeStore{}
	fr := &failRecorder{}

	err := RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute, nil, fr.fn, func() error {
		return fnErr
	})

	require.ErrorIs(t, err, fnErr)
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseCalls), "release must run after a handler error")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fr.calls), "handler errors must not park the claimed row")
}

func TestRunLive_ReleaseErrorDoesNotOverrideFnResult(t *testing.T) {
	fnErr := errors.New("handler failed")
	releaseErr := errors.New("release failed")

	for _, tc := range []struct {
		name    string
		fnErr   error
		wantErr error
	}{
		{name: "successful handler", wantErr: nil},
		{name: "failed handler", fnErr: fnErr, wantErr: fnErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := &liveFakeStore{releaseErr: releaseErr}
			fr := &failRecorder{}
			err := RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute, nil, fr.fn, func() error {
				return tc.fnErr
			})

			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
			assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseCalls))
			assert.Equal(t, int32(0), atomic.LoadInt32(&fr.calls))
		})
	}
}

// (b) transient contention — ErrRetryLater once then success → fn runs after exactly one
// spin, fail NOT called, total elapsed ≈ spinDelays[0] (F8: the spin absorbs the wait).
func TestRunLive_TransientContention_SpinsOnceThenSucceeds(t *testing.T) {
	fs := &liveFakeStore{acquireErrs: []error{reliable.ErrRetryLater}} // 1st: contention, 2nd: success
	fr := &failRecorder{}
	spin := []time.Duration{10 * time.Millisecond}
	fnRan := false
	start := time.Now()
	err := RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute, spin, fr.fn, func() error {
		fnRan = true
		return nil
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.True(t, fnRan, "fn must run after one spin")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fr.calls), "fail must NOT be called on transient contention")
	assert.Equal(t, int32(2), atomic.LoadInt32(&fs.acquireCalls), "initial + one retry")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseCalls))
	// Spin absorbs the wait (F8): lower bound = base delay (jitter only adds), upper bound
	// absorbs jitter + scheduling.
	assert.GreaterOrEqual(t, elapsed, spin[0], "must have slept ~spinDelays[0]")
	assert.Less(t, elapsed, spin[0]+50*time.Millisecond, "only one spin, not more")
}

// (c) sustained contention — always ErrRetryLater → fail called once with ClassRetryable,
// fn NOT called, returns nil (ACK). F1 regression guard: ErrRetryLater NOT returned.
// Asserts len(spinDelays)+1 acquire attempts (initial + one per spin).
func TestRunLive_SustainedContention_FailsRetryableACKs(t *testing.T) {
	spin := []time.Duration{5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond}
	fs := &liveFakeStore{acquireErrs: []error{
		reliable.ErrRetryLater, reliable.ErrRetryLater, reliable.ErrRetryLater,
		reliable.ErrRetryLater, reliable.ErrRetryLater, reliable.ErrRetryLater,
	}}
	fr := &failRecorder{}
	fnRan := false
	err := RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute, spin, fr.fn, func() error {
		fnRan = true
		return nil
	})

	assert.NoError(t, err, "F1: sustained contention ACKs (nil), never ErrRetryLater")
	assert.False(t, errors.Is(err, reliable.ErrRetryLater), "F1 regression: ErrRetryLater must NOT be returned")
	assert.False(t, fnRan, "fn must NOT run when never acquired")
	require.Equal(t, int32(1), fr.calls, "fail called exactly once")
	assert.Equal(t, reliable.ClassRetryable, fr.classes[0])
	assert.Equal(t, int32(len(spin)+1), atomic.LoadInt32(&fs.acquireCalls),
		"initial + one per spin delay = len(spinDelays)+1 attempts")
}

// (d) real DB error (not contention) → NO spin (exactly one acquire) → fail(ClassRetryable,
// err) → nil. F9 regression guard: a DB error must not be retried three times.
func TestRunLive_DBError_NoSpin_FailsImmediately(t *testing.T) {
	dbErr := errors.New("connection refused")
	fs := &liveFakeStore{acquireErrs: []error{dbErr}}
	fr := &failRecorder{}
	spin := []time.Duration{10 * time.Millisecond, 10 * time.Millisecond}
	fnRan := false
	start := time.Now()
	err := RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute, spin, fr.fn, func() error {
		fnRan = true
		return nil
	})
	elapsed := time.Since(start)

	assert.NoError(t, err, "DB error → MarkFailed(retryable)+ACK → nil")
	assert.False(t, fnRan, "fn must NOT run on DB error")
	require.Equal(t, int32(1), atomic.LoadInt32(&fs.acquireCalls), "F9: exactly ONE acquire (no spin)")
	require.Equal(t, int32(1), fr.calls, "fail called once")
	assert.Equal(t, reliable.ClassRetryable, fr.classes[0])
	assert.ErrorIs(t, fr.causes[0], dbErr, "fail cause is the DB error")
	assert.Less(t, elapsed, spin[0], "no spin on DB error")
}

// (e) empty key → fn runs directly, no acquire, no fail.
func TestRunLive_EmptyKey_FnRunsNoAcquireNoFail(t *testing.T) {
	fs := &liveFakeStore{}
	fr := &failRecorder{}
	fnRan := false
	err := RunLive(context.Background(), fs, nil, reliable.AggregateGateKey{}, "h", time.Minute,
		[]time.Duration{10 * time.Millisecond}, fr.fn, func() error {
			fnRan = true
			return nil
		})

	require.NoError(t, err)
	assert.True(t, fnRan, "fn must run for empty key")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fs.acquireCalls), "empty key → no acquire")
	assert.Equal(t, int32(0), fr.calls, "empty key → no fail")
}

// (f) release uses an independent ctx even when the business ctx is canceled mid-fn.
func TestRunLive_ReleaseUsesIndependentContext(t *testing.T) {
	fs := &liveFakeStore{}
	fr := &failRecorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := RunLive(ctx, fs, nil, nonEmptyKey(), "h", time.Minute, []time.Duration{10 * time.Millisecond}, fr.fn, func() error {
		cancel() // cancel business ctx while holding the gate
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseCalls), "release must run")
	assert.True(t, fs.releaseCtxAlive, "release must use an independent ctx, not the canceled business ctx")
}

// (g) business ctx canceled mid-spin → returns ctx.Err() promptly, no further acquire
// attempts, fn not run, fail not called.
func TestRunLive_ContextCanceledMidSpin_ReturnsCtxErr(t *testing.T) {
	spin := []time.Duration{200 * time.Millisecond}
	fs := &liveFakeStore{acquireErrs: []error{reliable.ErrRetryLater, reliable.ErrRetryLater, reliable.ErrRetryLater}}
	fr := &failRecorder{}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	fnRan := false
	start := time.Now()
	err := RunLive(ctx, fs, nil, nonEmptyKey(), "h", time.Minute, spin, fr.fn, func() error {
		fnRan = true
		return nil
	})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled, "must return ctx.Err() promptly")
	assert.False(t, fnRan, "fn must not run")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.acquireCalls), "no further acquire after ctx cancel")
	assert.Equal(t, int32(0), fr.calls, "fail must not be called on ctx cancel")
	assert.Less(t, elapsed, spin[0], "must return before the full spin delay")
}

// (h) fail itself returning an error → surfaced to the caller, not swallowed (otherwise a
// failed MarkFailed leaves the row PROCESSING and the helper ACKs = silent loss).
func TestRunLive_FailCallbackError_SurfacedNotSwallowed(t *testing.T) {
	failErr := errors.New("MarkFailed blew up")
	fs := &liveFakeStore{acquireErrs: []error{reliable.ErrRetryLater, reliable.ErrRetryLater}}
	fr := &failRecorder{retErr: failErr}

	err := RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute,
		[]time.Duration{5 * time.Millisecond}, fr.fn, func() error { return nil })

	require.Error(t, err, "fail's error must be surfaced, not swallowed into nil ACK")
	assert.ErrorIs(t, err, failErr)
	assert.Equal(t, int32(1), fr.calls, "fail called once")
}

// (i) API guard (F11): MarkFailedFn has NO maxAttempts parameter; AssertMaxAttemptsSymmetry
// returns nil for equal args and non-nil for differing; no DefaultGateMaxAttempts exported.
func TestRunLive_APIGuard_F11(t *testing.T) {
	// Compile-time type assertion: if anyone adds a maxAttempts parameter to MarkFailedFn,
	// this line stops compiling.
	var _ MarkFailedFn = func(reliable.ErrorClass, error) error { return nil }

	// AssertMaxAttemptsSymmetry: nil for equal, non-nil for differing.
	assert.NoError(t, AssertMaxAttemptsSymmetry(5, 5))
	assert.Error(t, AssertMaxAttemptsSymmetry(5, 3))

	// No DefaultGateMaxAttempts symbol is exported from this package — enforced by import
	// discipline + this comment. Go cannot negatively assert symbol absence; deliberately do
	// NOT over-engineer a reflection test for it.
}

// (panic) fn panics while holding the gate → release MUST still run during the defer unwind.
// This is the package's core guarantee ("acquired → release always runs"); a refactor that
// moved release off a defer would leak the gate on panic and only this test would catch it.
func TestRunLive_PanicInFn_ReleaseStillRuns(t *testing.T) {
	fs := &liveFakeStore{}
	fr := &failRecorder{}
	spin := []time.Duration{50 * time.Millisecond}

	var rec any
	func() {
		defer func() { rec = recover() }()
		_ = RunLive(context.Background(), fs, nil, nonEmptyKey(), "h", time.Minute, spin, fr.fn, func() error {
			panic("boom")
		})
	}()

	require.Equal(t, "boom", rec, "panic must propagate unchanged through RunLive")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.acquireCalls), "exactly one acquire")
	assert.Equal(t, int32(1), atomic.LoadInt32(&fs.releaseCalls), "release must run during panic unwind")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fr.calls), "fail must not be called on the fn path")
}

// jittered invariant: never returns less than d (additive jitter only) and d<=0 passes through
// unchanged (the early-return branch). Guards against a subtractive-jitter regression that the
// timing-based spin tests would not reliably catch.
func TestJittered_NeverShortensDelay_AndZeroNegativePassThrough(t *testing.T) {
	assert.Equal(t, time.Duration(0), jittered(0))
	assert.Equal(t, -5*time.Millisecond, jittered(-5*time.Millisecond))

	for _, d := range []time.Duration{1, time.Millisecond, 20 * time.Millisecond, time.Second} {
		upper := d + d/4
		for i := 0; i < 1000; i++ {
			j := jittered(d)
			assert.GreaterOrEqual(t, j, d, "jitter must never shorten the delay")
			assert.LessOrEqual(t, j, upper, "jitter upper bound is d + d/4")
		}
	}
}
