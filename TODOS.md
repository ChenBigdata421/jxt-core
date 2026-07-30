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

## F2 — process-management ↔ evidence-management/command consumer-group collision (cross-service)

- **What:** `process-management/config/settings.yml:102` and
  `evidence-management/command/config/settings.yml:102` both declare
  `groupId: "evidence-command-consumer-group"` (verbatim copy-paste). One service
  must change its groupId so Kafka doesn't split the command topic's partitions
  across two unrelated consumers.
- **Why:** A consumer group is the unit of partition assignment. Two distinct
  services in the same group consuming overlapping topics means Kafka balances
  partitions across both — command events can be delivered to process-management
  instead of evidence-management/command (or split between them), breaking
  command-side consumption. Live correctness hazard, independent of jxt-core.
- **Context:** Surfaced during the 2026-07-30 dead-switch eng review's
  outside-voice pass (spec §6 / P2,
  `docs/superpowers/specs/2026-07-30-kafka-pipeline-dead-switch-design.md`). It
  also explains why process-management is tracked as a future pipeline-adoption
  hazard: it already copy-pastes command's config, so a future `pipeline:` block
  copy is plausible. The collision is process-management's to fix (it is the
  copying/newer service); jxt-core owns no consumer-group names.
- **Depends on / blocked by:** Nothing in jxt-core.
- **Verify:** after the rename, `process-management` and
  `evidence-management/command` have distinct groupIds and each receives 100% of
  its own topic's partitions (check consumer-group membership / lag via
  Kafka/RedPanda admin).
