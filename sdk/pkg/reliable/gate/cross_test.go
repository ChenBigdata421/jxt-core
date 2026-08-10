package gate

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This file pins the gate's cross-instance + lifecycle contract (spec ⑰, F8/F11) via a
// faithful shared-state fake. The gate package owns the ORCHESTRATION
// (acquire / spin / fail-callback / ACK / release ordering); the store owns the SQL
// (CAS-overwrite vs ON CONFLICT, TTL expiry, the PG 25P02 trap) and the row state after
// MarkFailed. Those SQL/row-state concerns are covered by sdk/pkg/reliable/store/repotest
// (conformance.go's confAggregateGate at ~line 392-436 for the gate acquire CAS/INSERT
// path + PG-trap avoidance; confMarkFailedRetry at ~line 96-106 and confAttemptExhausted
// at ~line 127-134 for the MarkFailed row-state). This file cites those by file:line in
// comments and asserts the ORCHESTRATION here, keeping the gate suite fast / race-clean
// and free of testcontainers.

// —— shared-state fake ——

// gateHeld is a single live lease entry inside the shared state.
type gateHeld struct {
	token     string
	expiresAt time.Time
	holder    string
}

// sharedGateState models one logical "PG table" shared by N crossFakeStore instances.
// Two crossFakeStore values wrapping the SAME *sharedGateState model "two store.Store
// instances sharing one PG" (spec ⑰a) — the contention is enforced at the shared-state
// level, faithfully mirroring how the real gormshared.AcquireAggregateGate serializes
// via the shared DB row.
//
// All map access is mutex-protected; call counters are atomics so the suite is -race-clean
// even with 2+ goroutines (⑰a/⑰c). An injectable now() lets TTL tests advance time
// instantly instead of sleeping.
type sharedGateState struct {
	mu            sync.Mutex
	now           func() time.Time // injectable clock (default time.Now) for TTL tests
	held          map[reliable.AggregateGateKey]gateHeld
	acquireCalls  atomic.Int32
	releaseCalls  atomic.Int32
	freshInserts  atomic.Int32 // no prior entry → INSERT path
	casOverwrites atomic.Int32 // prior expired entry → CAS-overwrite path (gate.go:34-39)
	tokenSeq      atomic.Int64 // monotonic token counter — guarantees uniqueness across holders

	// release-observation fields (mutex-protected): snapshot at ReleaseAggregateGate call
	// time, mirroring gateFakeStore / liveFakeStore's releaseCtxAlive trick.
	releaseTokenSeen string
	releaseCtxAlive  bool
}

func newSharedState() *sharedGateState {
	return &sharedGateState{
		held: make(map[reliable.AggregateGateKey]gateHeld),
		now:  time.Now,
	}
}

// Two crossFakeStore values constructed against the SAME *sharedGateState model two
// instances sharing one PG. Token uniqueness comes from the shared monotonic counter
// (tokenSeq), so two distinct holders can never collide — mirroring the real
// holder+uuid scheme (D18#7).

type crossFakeStore struct {
	state *sharedGateState
}

// AcquireAggregateGate mirrors gormshared.AcquireAggregateGate's semantics against the
// shared state: empty key → ("", nil); a live (non-expired) entry → ErrRetryLater; an
// absent OR expired entry → mint a unique token and take the lease. The expired-entry
// branch increments casOverwrites (the CAS-overwrite path), the absent branch increments
// freshInserts (the INSERT ON CONFLICT path) — both real-SQL branches are verified
// cross-dialect by repotest confAggregateGate (conformance.go:392-436).
func (s *crossFakeStore) AcquireAggregateGate(_ context.Context, _ *gorm.DB, key reliable.AggregateGateKey, holder string, ttl time.Duration) (string, error) {
	s.state.acquireCalls.Add(1)
	if key.Empty() {
		return "", nil
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	now := s.state.now()
	if existing, ok := s.state.held[key]; ok && existing.expiresAt.After(now) {
		// Held by a live (non-expired) lease → contention (gormshared returns ErrRetryLater).
		return "", reliable.ErrRetryLater
	}
	// No entry, or an expired entry → mint a fresh unique token and take the lease.
	// Token uniqueness comes from the shared monotonic counter (tokenSeq), mirroring the
	// real holder+uuid scheme (D18#7) — two holders can never collide.
	token := fmt.Sprintf("%s:%d", holder, s.state.tokenSeq.Add(1))
	if _, hadPrior := s.state.held[key]; hadPrior {
		s.state.casOverwrites.Add(1) // expired-entry reclaim → CAS-overwrite branch
	} else {
		s.state.freshInserts.Add(1) // absent → INSERT branch
	}
	s.state.held[key] = gateHeld{token: token, expiresAt: now.Add(ttl), holder: holder}
	return token, nil
}

// ReleaseAggregateGate deletes the entry whose token matches (real GORM: DELETE WHERE
// holder_id = token). It snapshots ctx liveness at call time — a cancelled ctx would make
// the DELETE fail and leave the row (modelled by NOT deleting when ctx is dead), matching
// gateFakeStore / liveFakeStore's releaseCtxAlive trick.
func (s *crossFakeStore) ReleaseAggregateGate(ctx context.Context, _ *gorm.DB, token string) error {
	s.state.releaseCalls.Add(1)
	ctxAlive := ctx.Err() == nil
	s.state.mu.Lock()
	s.state.releaseTokenSeen = token
	s.state.releaseCtxAlive = ctxAlive
	if ctxAlive {
		for k, v := range s.state.held {
			if v.token == token {
				delete(s.state.held, k)
				break
			}
		}
	}
	s.state.mu.Unlock()
	if !ctxAlive {
		return ctx.Err()
	}
	return nil
}

// crossKey builds a non-empty aggregate key with a distinguishing suffix.
func crossKey(suffix string) reliable.AggregateGateKey {
	return reliable.AggregateGateKey{TenantID: 1, AggregateType: "Media", AggregateID: "agg-" + suffix}
}

// heldCount snapshots the number of live leases (caller must not hold the lock).
func (s *sharedGateState) heldCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.held)
}

// ⑰a — two store.Store instances sharing one logical PG (fakeA + fakeB over the same
// *sharedGateState). Replay caller = Acquire on fakeA; live caller = RunLive on
// fakeB. Same aggregate key → only one wins; the other gets ErrRetryLater (replay) or
// spins-then-parks (live). After release, the loser succeeds.
func TestGate_CrossInstance_LiveVsReplay_Serialize(t *testing.T) {
	state := newSharedState()
	fakeA := &crossFakeStore{state: state} // replay-side instance
	fakeB := &crossFakeStore{state: state} // live-side instance
	key := crossKey("17a")
	ctx := context.Background()
	ttl := time.Minute

	// Step 1: A (replay caller) holds the gate.
	releaseA, err := Acquire(ctx, fakeA, nil, key, "replay-1", ttl)
	require.NoError(t, err)
	require.NotNil(t, releaseA)

	// Step 1a: B's replay-style acquire (Acquire on fakeB) → ErrRetryLater.
	// (A replay caller would IncReplayBlocked+skip on this; model that by asserting the err.)
	relB, errB := Acquire(ctx, fakeB, nil, key, "replay-2", ttl)
	require.ErrorIs(t, errB, reliable.ErrRetryLater, "second instance sees the same held gate")
	assert.Nil(t, relB, "contention → nil release (caller must not defer it)")

	// Step 1b: B's live caller (RunLive on fakeB) spins then parks via fail(ClassRetryable).
	fr := &failRecorder{}
	bodyRan := false
	spin := []time.Duration{3 * time.Millisecond, 3 * time.Millisecond, 3 * time.Millisecond}
	err = RunLive(ctx, fakeB, nil, key, "live-1", ttl, spin, fr.fn, func() error {
		bodyRan = true
		return nil
	})
	assert.NoError(t, err, "F1: sustained contention ACKs (nil), never ErrRetryLater")
	assert.False(t, bodyRan, "live body must NOT run while the gate is held by another")
	require.Equal(t, int32(1), fr.calls, "fail called exactly once after spinning out")
	assert.Equal(t, reliable.ClassRetryable, fr.classes[0])
	assert.ErrorIs(t, fr.causes[0], reliable.ErrRetryLater, "fail cause is the contention sentinel")

	// Step 2: release A → B's next acquire / RunLive succeeds, body runs.
	require.NoError(t, releaseA())

	relB2, err := Acquire(ctx, fakeB, nil, key, "replay-3", ttl)
	require.NoError(t, err, "after release, the gate is re-acquirable (TTL/CAS path)")
	require.NotNil(t, relB2)
	assert.NoError(t, relB2())

	// Live path now also succeeds.
	body2Ran := false
	err = RunLive(ctx, fakeB, nil, key, "live-2", ttl, spin, fr.fn, func() error {
		body2Ran = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, body2Ran, "live body runs once the gate is free")
}

// ⑰a hold-shorter-than-spin variant: A holds, then releases after a delay SHORTER than
// B's first spin. B's RunLive should acquire on a spin retry (not park), fail NOT called,
// body runs.
func TestGate_CrossInstance_TransientHold_AcquiredOnSpinRetry(t *testing.T) {
	state := newSharedState()
	fakeA := &crossFakeStore{state: state}
	fakeB := &crossFakeStore{state: state}
	key := crossKey("17a-transient")
	ctx := context.Background()

	releaseA, err := Acquire(ctx, fakeA, nil, key, "holder-A", time.Minute)
	require.NoError(t, err)

	// Release A shortly AFTER B's first acquire attempt fails, but BEFORE B's first spin
	// delay elapses — so B's retry wins the gate.
	go func() {
		time.Sleep(5 * time.Millisecond)
		_ = releaseA()
	}()

	// Baseline after fakeA's hold-acquire (acquireCalls is shared across both fakes).
	baseline := state.acquireCalls.Load()

	fr := &failRecorder{}
	bodyRan := false
	spin := []time.Duration{20 * time.Millisecond} // > release delay
	err = RunLive(ctx, fakeB, nil, key, "live-B", time.Minute, spin, fr.fn, func() error {
		bodyRan = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, bodyRan, "transient contention: body must run after acquiring on a spin retry")
	assert.Equal(t, int32(0), fr.calls, "fail must NOT be called when the gate frees within the spin window")
	assert.GreaterOrEqual(t, state.acquireCalls.Load()-baseline, int32(2), "live caller: initial attempt + at least one retry")
}

// ⑰b — owner kill / TTL recovery: an acquired gate whose TTL has elapsed is re-acquirable
// by a second caller. Asserts the ORCHESTRATION (re-acquire succeeds post-TTL) and that
// the fake's shared state recorded the CAS-overwrite branch (the expired-entry path).
//
// The REAL SQL behind this — gormshared.AcquireAggregateGate's two-step CAS-then-INSERT
// (gate.go:34-50), engineered to avoid the PG 25P02 aborted-transaction trap
// (gate.go:24-33) — is verified against both dialects by repotest/conformance.go's
// confAggregateGate (sdk/pkg/reliable/store/repotest/conformance.go:392-436). This test
// does not duplicate the raw-SQL assertion; it pins the orchestration-level contract.
func TestGate_CrossInstance_TTLRecovery(t *testing.T) {
	state := newSharedState()
	fake := &crossFakeStore{state: state}
	key := crossKey("17b")
	ctx := context.Background()

	// Injectable clock: start frozen, advance manually to expire the lease WITHOUT sleeping.
	clock := time.Unix(1_700_000_000, 0).UTC()
	state.now = func() time.Time { return clock }

	ttl := 20 * time.Millisecond
	release1, err := Acquire(ctx, fake, nil, key, "owner-1", ttl)
	require.NoError(t, err)
	require.NotNil(t, release1)
	assert.Equal(t, int32(1), state.freshInserts.Load(), "first acquire is a fresh insert")
	assert.Equal(t, int32(0), state.casOverwrites.Load())

	// Advance the clock past the lease's expires_at (now > acquired_at + ttl).
	clock = clock.Add(ttl + time.Millisecond)

	// Second acquire succeeds via the CAS-overwrite branch (expired entry present).
	release2, err := Acquire(ctx, fake, nil, key, "owner-2", ttl)
	require.NoError(t, err, "TTL-expired gate is re-acquirable (CAS-overwrite path)")
	require.NotNil(t, release2)
	assert.Equal(t, int32(1), state.casOverwrites.Load(), "re-acquire post-TTL takes the CAS-overwrite branch")
	assert.Equal(t, int32(1), state.freshInserts.Load(), "no second fresh insert")

	// release1's token is now stale (entry holds release2's token) → no-op delete, correct.
	assert.NoError(t, release1())
	assert.NoError(t, release2())
}

// ⑰c — two DIFFERENT aggregate keys acquired concurrently must BOTH succeed (the gate
// serializes per-key, not globally). Runs under -race to verify the shared-state fake's
// mutex protection.
func TestGate_CrossInstance_DifferentAggregates_Parallel(t *testing.T) {
	state := newSharedState()
	fake := &crossFakeStore{state: state}
	key1 := crossKey("17c-1")
	key2 := crossKey("17c-2")
	ctx := context.Background()

	var err1, err2 error
	var rel1, rel2 func() error
	var wg sync.WaitGroup
	wg.Add(2)
	start := make(chan struct{})
	go func() {
		defer wg.Done()
		<-start
		rel1, err1 = Acquire(ctx, fake, nil, key1, "h1", time.Minute)
	}()
	go func() {
		defer wg.Done()
		<-start
		rel2, err2 = Acquire(ctx, fake, nil, key2, "h2", time.Minute)
	}()
	close(start)
	wg.Wait()

	require.NoError(t, err1, "distinct aggregate key1 must acquire freely")
	require.NoError(t, err2, "distinct aggregate key2 must acquire freely")
	require.NotNil(t, rel1)
	require.NotNil(t, rel2)

	assert.Equal(t, 2, state.heldCount(), "both distinct aggregates hold simultaneously (per-key serialization, not global)")
	assert.Equal(t, int32(2), state.acquireCalls.Load())
	assert.Equal(t, int32(2), state.freshInserts.Load(), "both are fresh inserts (distinct keys)")
}

// release-on-cancel: the business ctx is canceled AFTER acquire; release() must still
// delete the gate row via its own independent ctx (ReleaseTimeout). Mirrors the
// gate_test.go / live_test.go release-ctx-alive checks but at the cross-test level.
func TestGate_CrossInstance_ReleaseOnCancel(t *testing.T) {
	state := newSharedState()
	fake := &crossFakeStore{state: state}
	key := crossKey("rel-cancel")
	ctx, cancel := context.WithCancel(context.Background())

	release, err := Acquire(ctx, fake, nil, key, "holder", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, release)
	require.Equal(t, 1, state.heldCount(), "gate is held after acquire")

	cancel() // simulate tickTimeout fire / parent cancel: business ctx now done

	assert.NoError(t, release(), "release must succeed via its own ctx despite canceled business ctx")
	assert.True(t, state.releaseCtxAlive, "release must use an independent non-cancelled context")
	assert.Equal(t, int32(1), state.releaseCalls.Load())
	assert.Equal(t, 0, state.heldCount(), "release deleted the gate row even though business ctx was canceled")
}

// F8/F11 (i) — transient contention (held < spin window): RunLive acquires on a retry,
// fail NOT called, body runs. The gate package asserts failFn NON-invocation; the
// row-state outcome when fail IS called (RETRY_SCHEDULED, attempt==1, RETRYABLE, no
// DEAD_LETTER) is MarkFailed's domain, verified by repotest confMarkFailedRetry
// (sdk/pkg/reliable/store/repotest/conformance.go:96-106) and confAttemptExhausted
// (sdk/pkg/reliable/store/repotest/conformance.go:127-134).
func TestGate_F8_AttemptAccounting_Transient_NoFail(t *testing.T) {
	state := newSharedState()
	fakeA := &crossFakeStore{state: state}
	fakeB := &crossFakeStore{state: state}
	key := crossKey("f8-transient")
	ctx := context.Background()

	releaseA, err := Acquire(ctx, fakeA, nil, key, "holder-A", time.Minute)
	require.NoError(t, err)

	// Release A within the spin window (B's first spin is 20ms; release at 5ms).
	go func() {
		time.Sleep(5 * time.Millisecond)
		_ = releaseA()
	}()

	// Baseline after fakeA's hold-acquire (acquireCalls is shared across both fakes).
	baseline := state.acquireCalls.Load()

	fr := &failRecorder{}
	bodyRan := false
	spin := []time.Duration{20 * time.Millisecond}
	err = RunLive(ctx, fakeB, nil, key, "live-B", time.Minute, spin, fr.fn, func() error {
		bodyRan = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, bodyRan, "body runs after acquiring on retry")
	assert.Equal(t, int32(0), fr.calls, "F8: transient contention must NOT call fail")
	assert.GreaterOrEqual(t, state.acquireCalls.Load()-baseline, int32(2), "live caller: initial attempt + retry")
}

// F8/F11 (ii) — sustained contention (held past the whole spin window): RunLive spins
// out and calls fail(ClassRetryable, contention-err) EXACTLY ONCE, body does NOT run,
// returns nil (ACK, F1). Row-state (RETRY_SCHEDULED / attempt==1 / RETRYABLE / no
// DEAD_LETTER) is MarkFailed's domain — cited at repotest confMarkFailedRetry
// (conformance.go:96-106) + confAttemptExhausted (conformance.go:127-134); per round-4
// correction the gate package does NOT assert attempt increments (TryClaim sets
// Attempt=1 at INSERT, claim.go:98; neither the inline reclaim CAS, claim.go:63-69, nor
// MarkFailed, mark.go:66-79, touch it).
func TestGate_F8_AttemptAccounting_Sustained_FailsOnce(t *testing.T) {
	state := newSharedState()
	fakeA := &crossFakeStore{state: state}
	fakeB := &crossFakeStore{state: state}
	key := crossKey("f8-sustained")
	ctx := context.Background()

	releaseA, err := Acquire(ctx, fakeA, nil, key, "holder-A", time.Minute)
	require.NoError(t, err)
	defer func() { _ = releaseA() }()

	// Baseline after fakeA's hold-acquire: state.acquireCalls is shared across both fakes,
	// so measure the live-caller (fakeB) delta rather than the absolute count.
	baseline := state.acquireCalls.Load()

	fr := &failRecorder{}
	bodyRan := false
	spin := []time.Duration{3 * time.Millisecond, 3 * time.Millisecond, 3 * time.Millisecond}
	err = RunLive(ctx, fakeB, nil, key, "live-B", time.Minute, spin, fr.fn, func() error {
		bodyRan = true
		return nil
	})

	assert.NoError(t, err, "F1: sustained contention ACKs (nil), never ErrRetryLater")
	assert.False(t, bodyRan, "body must NOT run when never acquired")
	require.Equal(t, int32(1), fr.calls, "fail called exactly once")
	assert.Equal(t, reliable.ClassRetryable, fr.classes[0])
	assert.ErrorIs(t, fr.causes[0], reliable.ErrRetryLater, "cause is the contention sentinel")
	// Live-caller attempts only: initial + one retry per spin delay = len(spinDelays)+1.
	assert.Equal(t, int32(len(spin)+1), state.acquireCalls.Load()-baseline,
		"len(spinDelays)+1 live-caller acquire attempts before parking")
}

// F8 nil/empty spinDelays → park on the FIRST contention (no spin). fail called once,
// body not called, exactly ONE acquire attempt.
func TestGate_F8_NilSpinDelays_ParksOnFirstContention(t *testing.T) {
	state := newSharedState()
	fakeA := &crossFakeStore{state: state}
	fakeB := &crossFakeStore{state: state}
	key := crossKey("f8-nilspin")
	ctx := context.Background()

	releaseA, err := Acquire(ctx, fakeA, nil, key, "holder-A", time.Minute)
	require.NoError(t, err)
	defer func() { _ = releaseA() }()

	// Baseline after fakeA's hold-acquire (acquireCalls is shared across both fakes).
	baseline := state.acquireCalls.Load()

	fr := &failRecorder{}
	bodyRan := false
	start := time.Now()
	err = RunLive(ctx, fakeB, nil, key, "live-B", time.Minute, nil, fr.fn, func() error {
		bodyRan = true
		return nil
	})
	elapsed := time.Since(start)

	assert.NoError(t, err, "parks via fail(retryable)+ACK")
	assert.False(t, bodyRan)
	require.Equal(t, int32(1), fr.calls, "fail called once")
	assert.Equal(t, reliable.ClassRetryable, fr.classes[0])
	assert.Equal(t, int32(1), state.acquireCalls.Load()-baseline, "nil spins → exactly one live-caller acquire attempt (no spin)")
	assert.Less(t, elapsed, 30*time.Millisecond, "no spin sleeping on nil spinDelays")
}

// F8-bis parked-row overtake hazard (round 4): the gate package CANNOT test the
// read-model stale-overwrite guard (no read model here). This orchestration-level case
// demonstrates the hazard SHAPE:
//  1. gate busy → live delivery of event@offset 100 parks (fail+ACK), body does NOT run;
//  2. release the gate;
//  3. later live delivery of event@offset 101 (same aggregate) → body RUNS.
//
// So event@101 applies BEFORE event@100's parked row replays. The read model MUST carry
// a last-applied (topic,partition,offset) and reject the stale re-apply of event@100, or
// state silently regresses (contract §9 line 986). That monotonic guard lives in the
// adopting service (PR-4 Task 1b), NOT in this helper — see live.go's RunLive doc
// comment "F8-bis parked-row overtake hazard". Shipping the case here makes the contract
// visible in core's own suite per the plan's intent.
func TestGate_F8Bis_ParkedRowOvertakeHazard(t *testing.T) {
	state := newSharedState()
	fakeA := &crossFakeStore{state: state} // holder
	fakeB := &crossFakeStore{state: state} // live consumer
	key := crossKey("f8bis")
	ctx := context.Background()

	// 1. fakeA holds the gate.
	releaseA, err := Acquire(ctx, fakeA, nil, key, "holder-A", time.Minute)
	require.NoError(t, err)

	// 2. Live delivery of event@offset 100 → gate busy → park + ACK.
	fr := &failRecorder{}
	body100Ran := false
	err = RunLive(ctx, fakeB, nil, key, "live-100", time.Minute,
		[]time.Duration{3 * time.Millisecond}, fr.fn, func() error {
			body100Ran = true
			return nil
		})
	assert.NoError(t, err, "parked delivery ACKs (nil)")
	assert.False(t, body100Ran, "event@100 body must NOT run (parked)")
	require.Equal(t, int32(1), fr.calls, "event@100 parked via fail(ClassRetryable)")
	assert.Equal(t, reliable.ClassRetryable, fr.classes[0])

	// 3. Release the gate.
	require.NoError(t, releaseA())

	// 4. Later live delivery of event@offset 101 (same aggregate) → applies.
	body101Ran := false
	err = RunLive(ctx, fakeB, nil, key, "live-101", time.Minute,
		[]time.Duration{3 * time.Millisecond}, fr.fn, func() error {
			body101Ran = true
			return nil
		})
	require.NoError(t, err)
	assert.True(t, body101Ran, "event@101 body runs after release")
	assert.Equal(t, int32(1), fr.calls, "event@101 did NOT call fail (acquired on first try)")
}
