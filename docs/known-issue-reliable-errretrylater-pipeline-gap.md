# Known Issue — `ErrRetryLater` pipeline gap (P2)

> Tracked kernel issue. Filed per `docs/superpowers/plans/2026-08-07-jxtcore-live-aggregate-gate.md`
> Task 4 Step 4. Kept as a repo doc (not a comment, not a public tracker entry) so it is not lost.

## Summary

The Kafka partition pipeline treats **any** non-nil return from an Envelope-delivery handler —
including `reliable.ErrRetryLater` — as a commit-failure and sends the message to the DLQ. There is
no `ErrRetryLater` special case on the envelope path, so a handler that yields with "retry later"
(e.g. `AlreadyProcessing`) is terminally disposed instead of redelivered.

## Location

`sdk/pkg/eventbus/partition_pipeline.go:44-53` — `decideCommitable`:

```go
func decideCommitable(e *inflightEntry, err error) commitDecision {
	if err == nil {
		return commitSuccess
	}
	if e.isEnvelope {
		return commitEnvelopeFail   // ← any non-nil err on an envelope delivery → DLQ
	}
	return commitRegularFail
}
```

`commitEnvelopeFail` flows to `sendDLQ` (consumed at the commit-decision site), so an envelope
handler returning `ErrRetryLater` is committed-via-DLQ rather than held for redelivery.

## Effect / spec tension

This contradicts the reliable-consumption contract (spec §3.1 / §6.2 — the
`AlreadyProcessing → ErrRetryLater` "don't ACK, let the broker redeliver" path): on the envelope
path `ErrRetryLater` is not a redelivery signal, it is a terminal disposition.

## Root-cause linkage to the aggregate-gate work

This gap is exactly why the live aggregate-gate helper (`sdk/pkg/reliable/gate.RunLive`,
added in v1.7.6) cannot use the kernel's natural **gate-then-claim + `ErrRetryLater`** ordering and
instead must use **claim-then-gate + `MarkFailed(retryable)` + ACK**:

- Before `TryClaim`, returning `ErrRetryLater` on gate-busy would reach `RecordTerminal`, which
  conditional-INSERTs a spurious `DEAD_LETTER` (no row exists yet) = silent loss.
- After `TryClaim`, the pipeline has no `ErrRetryLater` parking either — it would DLQ — so the
  helper parks the just-claimed row itself via `MarkFailed(retryable)` + ACK and lets the replay
  scheduler drain it.

See plan findings **F1**, **F3**, **F8**, and **F8-bis** for the full chain. Closing this gap would
let the live path adopt the kernel's gate-then-claim ordering and remove both the F8 "让路 written as
a failure row" deviation and the F8-bis parked-row stale-overwrite hazard's largest producer.

## Severity — P2 (downgraded 2026-08-08)

Originally P1/P0 framing. Downgraded because **v1.7.4 added a core DLQ adapter that compensates for
the gap on any service wiring it**: `sdk/pkg/reliable/adapters/eventbus/adapter.go:190-194`

```go
if class == reliable.ClassRetryable || errors.Is(cause, reliable.ErrRetryLater) {
	// ... log ...
	return cause // ← fail closed: dlqResult.ok=false → Strategy A blocks the frontier
}
```

This refuses to `RecordTerminal` a retryable / `ErrRetryLater` cause and returns the cause →
`dlqResult.ok=false` → **Strategy A blocks the partition frontier + alerts** → the message is
redelivered on the next rebalance. So on the reliable path, returning `ErrRetryLater` is **no longer
silent loss** and a spurious `DEAD_LETTER` from `ErrRetryLater` is no longer reachable there.

## Why it is still open

1. **Partition-stall cost.** A retryable cause blocks the *whole partition* until rebalance, instead
   of parking one ledger row. Closing the gap removes that stall.
2. **Legacy path uncompensated.** The legacy `dlqsender.Adapter` path does not fail-closed; services
   not on the core reliable DLQ adapter are still exposed.
3. **It unlocks the cleaner live-gate design** (gate-then-claim, removing the F8 parking deviation
   and the F8-bis overtake hazard).

## Fix direction

Teach `decideCommitable` (or its caller) to treat `ErrRetryLater` — and more generally
`ClassRetryable` — on an Envelope delivery as a **non-committing hold / redelivery** (Strategy A:
block the frontier, redeliver on rebalance) rather than `commitEnvelopeFail → sendDLQ`. Then revisit
`gate.RunLive`'s gate-busy branch together with the replay scheduler's `IncReplayBlocked + skip`
symmetry (see plan Global Constraints — the two are coupled and should be revisited together).

## Verification citations (all confirmed against v1.7.5 source, 2026-08-09)

- `sdk/pkg/eventbus/partition_pipeline.go:44-53` — `decideCommitable` envelope arm.
- `sdk/pkg/reliable/adapters/eventbus/adapter.go:190-194` — v1.7.4 fail-close compensation.
- `sdk/pkg/reliable/gate/live.go` — `RunLive` (the helper constrained by this gap); doc comment
  records the v1.7.4 compensation and the F8-bis overtake hazard.
