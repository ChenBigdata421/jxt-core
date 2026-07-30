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

## F3 — Kafka consumer pipeline activation prerequisites (gate Task 8 canary)

Three timing-invariant gaps surfaced in the 2026-07-30 `/review` of the v1.1.70
dead-switch fix (commits `7812717`, `9e130ef`). All assume values the user config
layer does not guarantee. None bite while the pipeline ships default-OFF (v1.1.70
no-op); all must be closed **before** Task 8 flips `pipeline.enabled:true` in any
canary.

- **What:**
  1. **`sessionTimeout=0` skips the flush-timeout fail-fast.**
     `NewKafkaEventBus` calls `effective.validate(cfg.Consumer.SessionTimeout)`
     (`sdk/pkg/eventbus/kafka.go:254`) and `validate` guards the invariant on
     `sessionTimeout > 0` (`sdk/pkg/eventbus/type.go:650`). The natural minimal
     enablement (`pipeline:{enabled:true}`, no `sessionTimeout`) →
     `SessionTimeout=0` → invariant skipped → sarama receives
     `session.timeout=0` (`kafka.go:311`, no `>0` guard) and fails late with a
     non-pipeline error. The PR's "fail-fast, network-free validation" contract
     does not hold for this shape. Fix: validate against an *effective* session
     timeout (apply a default, e.g. 10s mirroring sarama, before `validate`) and
     add an `enabled=true + sessionTimeout=0` regression test asserting it now
     fails fast with the wrapped pipeline error.
  2. **Flush-timeout error points at a value the operator cannot set.**
     `validate` returns `pipeline.flushTimeout (4s) must be < sessionTimeout/2`
     (`type.go:651`), but `flushTimeout` is internal-only (`PipelineUserConfig`
     strips it — `sdk/config/eventbus.go:188-193`); the 4s comes from
     `applyPipelineDefaults`. The operator's only lever is to **raise**
     `sessionTimeout`, which the message never says. Fix: mention "raise
     sessionTimeout" in the message, or carry a user-facing hint in the wrap at
     `kafka.go:255`.
  3. **Bare-`Unmarshal` silent-drop still live for timing keys.**
     `sdk/config/config.go:56` is still `v.Unmarshal(AppConfig)` with no
     `ErrorUnused`. A user who copies a `pipeline.flushTimeout`/`dlqTimeout` line
     from an internal-struct example or older doc gets it silently dropped — same
     mechanism as the original dead switch, narrowed to timing keys (including
     *invalid* values the invariant would otherwise have caught). Fix: at minimum
     warn on unused keys under `pipeline.`; ideally add
     `viper.DecoderConfigOption(...WithErrorUnused)` (or a targeted check) so the
     dead switch can't recur for the next field.
- **Why:** v1.1.70 ships the pipeline dark (default `Enabled=false`), so there is
  no live hazard today. Task 8 (pipeline canary) is the first time `enabled:true`
  is set in prod; the fail-fast contract must actually hold by then, or a
  misconfigured canary fails late and confusingly instead of at construction.
- **Context:** 2026-07-30 `/review` of the 3-hour dead-switch window
  (`25c0f72..c141b8c`). The dead-switch decode fix itself is now pinned
  end-to-end by `sdk/config/eventbus_pipeline_decode_test.go` (added in the same
  review). Findings 1 and 2 share a root cause: timing-invariant enforcement
  assumes values the user layer doesn't default.
- **Depends on / blocked by:** Task 8 (pipeline canary activation). Nothing in
  v1.1.70.
- **Verify:**
  1. `enabled=true + sessionTimeout=0` construction returns the wrapped
     `"invalid kafka consumer pipeline config"` error (not a sarama dial/validate
     error) — pinned by a regression test.
  2. The error message tells the operator to raise `sessionTimeout`.
  3. An unknown `pipeline.flushTimeout`/`pipeline.dlqTimeout` key is surfaced
     (warn/error), not silently dropped.
