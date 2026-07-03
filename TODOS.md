# TODOS — jxt-core

Tracked follow-ups. Each item: what / why / context / depends on.

## F1 — Consumer-side idempotency audit (before/parallel to v2.1.0 adoption)

- **What:** Confirm every consumer of `domain.*` events is idempotent. In
  evidence-management: incident/case/writ/task/media `BatchCreated` handlers and
  any other `domain.*` subscriber.
- **Why:** jxt-core v2.1.0 makes async ACK-mark batching default-on
  (`ACKBatchSize=50`), widening the duplicate-delivery window from ≤1 to ≤50
  events on hard crash, and a transient `MarkBatchAsPublished` failure causes one
  spurious re-publish + duplicate (Decision 6 / amendment A7 of the plan). The
  feature's safety rests on consumer idempotency. v2.0.0's at-least-once already
  required it in principle; v2.1.0 materially widens the window.
- **Context:** Originated in the eng review of
  `docs/superpowers/plans/2026-07-03-outbox-ack-listener-batching.md`
  (Decision 7 / Followup F1). The plan declares the requirement (Task 5 Step 1
  doc comment) but does not verify it — the audit is cross-repo
  (evidence-management) and will not happen unless owned.
- **Depends on / blocked by:** Nothing in jxt-core. Blocks safe *adoption* of
  v2.1.0 in evidence-management.
- **Verify:** for each `domain.*` handler, processing the same event twice yields
  the same end state (idempotent write through the aggregate, or dedup by event
  id / idempotency key).
