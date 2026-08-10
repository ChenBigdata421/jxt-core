package gate

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"gorm.io/gorm"
)

// MarkFailedFn is the live caller's adapter to the skeleton's
// st.MarkFailed(ctx, gdb, key, tok, class, safety, maxAttempts, cause, rawValue).
//
// It deliberately carries NO maxAttempts parameter (F11): the adapter closes over the
// service's single budget value, making the unsound per-branch inflation unrepresentable
// in the API. This keeps RunLive service-agnostic while enforcing F1/F8/F9/F11 in one place.
type MarkFailedFn func(class reliable.ErrorClass, cause error) error

// DefaultSpinDelays is the convenience default spin schedule (F8): 20ms, 40ms, 80ms.
// A caller MAY pass it as RunLive's spinDelays, but nil/empty means "no spin" (park on the
// first contention). This default is a convenience var, NOT baked into RunLive's behaviour —
// the caller's choice to spin or not is load-bearing and must stay explicit.
var DefaultSpinDelays = []time.Duration{20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond}

var ErrInvalidSpinDelay = fmt.Errorf("reliable/gate: spin delays must be positive")

// AssertMaxAttemptsSymmetry is the startup check adopters call to guarantee the live
// handler's MarkFailed budget equals the replay scheduler's budget (F11). It returns nil
// when the two match, or an error describing the divergence.
//
// The helper itself carries no budget — this function exists precisely because the budget
// lives in the adopting service and must be identical for MarkFailed and the scheduler (a
// handler that always returns ErrRetryLater must hit the same ceiling both paths enforce,
// else it retries forever on one path while the other dead-letters).
func AssertMaxAttemptsSymmetry(handlerMax, schedulerMax int) error {
	if handlerMax != schedulerMax {
		return fmt.Errorf("reliable/gate: max-attempts asymmetry — handler=%d scheduler=%d (F11 violation: MarkFailed and scheduler must share one budget)", handlerMax, schedulerMax)
	}
	return nil
}

// RunLive is the live-path aggregate-gate entry point, called by the live §4 skeleton
// AFTER a Claimed TryClaim token (F3 — claim-then-gate, the OPPOSITE of the replay
// scheduler's gate-then-claim ordering).
//
// Precondition (F12): the caller's Meta.AggregateID MUST equal the envelope.AggregateID
// the eventbus actor pool routes on. If they diverge, same-aggregate work is NOT already
// serialized in-process and the gate becomes load-bearing for live-vs-live too.
//
// Behaviour:
//   - key.Empty() → run fn directly (no gate, no fail).
//   - Acquired → defer release (independent ctx) → run fn → propagate fn's result.
//   - Contention (gate.IsContention) → bounded spin over spinDelays (ctx-aware, jittered);
//     if a retry wins, proceed as acquired. If every spin loses → fail(ClassRetryable)+ACK
//     (return nil) — F1: never ErrRetryLater.
//   - Real DB error (not contention) → no spin → fail(ClassRetryable)+ACK — F9.
//   - spinDelays nil/empty → park on the first contention (no spin); a gated path rejects non-positive entries with ErrInvalidSpinDelay.
//   - If fail itself returns a non-nil error, it is surfaced (never ACK on a failed
//     MarkFailed — otherwise the row stays PROCESSING and the helper ACKs = silent loss).
//
// F8-bis parked-row overtake hazard:
//
// Calling this on a gate-busy delivery parks the row and ACKs, so a later live event for
// the same aggregate can apply before this one replays. The read model MUST carry a
// last-applied broker position and reject a stale re-apply, or state silently regresses
// (contract §9 line 986). Reference implementation: PR-4 Task 1b (applied_topic /
// applied_partition / applied_offset + conditional update). Two traps: NULL on legacy rows
// must PASS the guard, and offsets are only comparable within one (topic, partition).
//
// Intentional release-error swallow (asymmetry vs replay):
//
// `defer release()` discards the release closure's error return — this is deliberate, NOT a
// bug to "fix" by surfacing it. fn() has already returned (whether nil/success OR a business
// error) by the time release runs; if RunLive then returned the release error it would corrupt
// its own return either way — on success the caller would treat a successfully-handled event as
// a failure and re-deliver an already-applied event, on fn-error it would mask fn's business
// error. Replay CAN surface this error (it raises REPLAY_GATE_RELEASE_FAILED, see Acquire's
// doc) because replay's failure semantics permit re-processing, but live cannot. The leaked-
// gate-row consequence is bounded: a failed release (e.g. DB unreachable during the
// independent-ctx release) leaves the gate row held until its TTL; reclamation is then LAZY —
// AcquireAggregateGate's step-1 CAS overwrites the expired row on the next same-key acquire
// (ReclaimExpiredAggregateGates exists as an active DELETE sweep but is NOT currently wired
// into any periodic tick), so same-aggregate live traffic spins/parks for at most that TTL
// window. If live-path release-failure observability is later wanted, the extension is an
// OPTIONAL alerter-shaped callback param on RunLive (default nil) — NOT returning the release error.
//
// v1.7.4 compensation note:
//
// Returning ErrRetryLater is no longer silent loss on the reliable path (v1.7.4's core DLQ
// adapter fail-closes retryable causes → partition-block + redelivery), but this helper's
// contract — MarkFailed(retryable)+ACK on gate-busy, never ErrRetryLater — is still
// preferable because it parks ONE row instead of stalling the whole partition.
func RunLive(ctx context.Context, st aggregateGateStore, db *gorm.DB, key reliable.AggregateGateKey,
	holder string, ttl time.Duration, spinDelays []time.Duration,
	fail MarkFailedFn, fn func() error) error {

	// Empty aggregate identity (notification events with no serial constraint) → skip the
	// gate entirely: no acquire, no fail, just run the business function.
	if key.Empty() {
		return fn()
	}
	if ttl <= 0 {
		return ErrInvalidLeaseTTL
	}
	for _, delay := range spinDelays {
		if delay <= 0 {
			return ErrInvalidSpinDelay
		}
	}

	for i := 0; ; i++ {
		release, err := Acquire(ctx, st, db, key, holder, ttl)
		if err == nil {
			// Won the gate (possibly after spinning). The release closure owns the
			// independent-ctx discipline (see Acquire); just defer it and run fn.
			defer release()
			return fn()
		}

		if !IsContention(err) {
			// Real DB error (F9) — do NOT spin: fail immediately and ACK. Only one acquire
			// attempt is made on this path.
			return failOrACK(fail, err)
		}

		// Contention. If no spins remain, give up: MarkFailed(retryable)+ACK (F1 — never
		// ErrRetryLater, which would stall the whole partition).
		if i >= len(spinDelays) {
			return failOrACK(fail, err)
		}

		// Spin: ctx-aware sleep with jitter, then re-acquire. If the business ctx is
		// canceled mid-spin, return ctx.Err() promptly with no further acquire attempts.
		if serr := spinSleep(ctx, spinDelays[i]); serr != nil {
			return serr
		}
	}
}

// failOrACK calls fail(ClassRetryable, cause). If fail returns a non-nil error it is
// surfaced (a failed MarkFailed must NOT become a silent ACK); otherwise RunLive ACKs
// by returning nil.
func failOrACK(fail MarkFailedFn, cause error) error {
	if err := fail(reliable.ClassRetryable, cause); err != nil {
		return err
	}
	return nil
}

// spinSleep blocks for ~d (with modest additive jitter) or returns ctx.Err() if the
// business context is canceled first. It deliberately never uses bare time.Sleep so a
// mid-spin cancel is prompt — the select races the timer against ctx.Done().
func spinSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(jittered(d))
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// jittered adds 0–25% jitter to d. It never returns less than d, so callers may assert a
// lower bound of the base delay (jitter only ever extends the wait).
func jittered(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d + time.Duration(rand.Int63n(int64(d/4)+1))
}
