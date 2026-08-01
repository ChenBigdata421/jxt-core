# Dispatch Silent-Drop Fix — jxt-core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the silent-drop footgun at its source — make the Kafka partition-pipeline and legacy consume paths **unable to commit a message whose handler isn't activated** (hold-and-block instead of drain-and-commit), so that any future handler-activation-timing mistake cannot silently lose messages.

**Architecture:** Layer A from the spec. Two nil-branches change behavior: `consumeWithPipeline` (pipeline path) and `ConsumeClaim` (legacy path) both switch from "nil → `MarkMessage`+drain" to "nil → hold in-hand message, backoff-poll `activeTopicHandlers`, never commit until activated." On the pipeline path the wrapper is resolved once at claim start and, if nil, the claim is **held outside `p.run`** (backpressure) until the topic activates — no `partition_pipeline.go` change (per Task 3 Step 3 / review D5a: per-message resolve inside `p.run` was rejected as it risks the inflight/frontier/commit-prefix invariants). Optional warmup/consumer-start decoupling (defer-start) is included as a later task. Then a version bump + 5-service coordinated rollout.

** precondition (from the evidence-management plan) — ✅ MET (bisect done 2026-08-01):** The evidence plan's bisect rebuilt query at `cf5a275` (old wiring, jxt-core v1.1.71, pipeline on) and the drain **reproduces identically** → `c2b2910` exonerated; the race is independent of evidence-management wiring. **This plan is REQUIRED, not deferred.** Its step-zero (Task 0: delete the 3s warmup sleep) likely fixes the incident alone. Remaining D8 condition before the 5-service rollout: verify PROD command `pipeline.enabled` (evidence plan Task 9 Step 1b) — if prod command is pipeline-ON (likely — D3-A prod-readiness shipped), command is exposed with no query-lever (R2-6) and this plan is incident-tracked; if dev/test-only, it de-scopes to routine hardening.

**Tech Stack:** Go 1.26, `github.com/IBM/sarama` v1.46.0, Kafka/RedPanda, Hollywood actor pool, Ginkgo/Golang test.

## Global Constraints

- Spec: `evidence-management/docs/superpowers/specs/2026-07-31-dispatch-silent-drop-rootfix-design.md` (source of truth).
- jxt-core is a **published module** (`github.com/ChenBigdata421/jxt-core`) consumed by 5 services — behavior fix, **no public API break**. Ship as a new minor version.
- sarama v1.46.0 `ConsumeClaim` runs synchronously in the session's consume goroutine; session release does `waitGroup.Wait()` (consumer_group.go:867-891). **Hold loops MUST select on `session.Context().Done()` + backoff timer only** (sarama cancels the session ctx before release, so this returns ≪60s) and **MUST NOT read `claim.Messages()` during hold** (D1: read-and-discard loses trailing messages for the session). Only the normal non-hold consume loop keeps the `case msg := <-claim.Messages(): if msg == nil return` select shape. (Rewritten in round-2 review D2' — the prior wording mandated a channel read inside the hold loop and directly contradicted D1; spec §5 A P2#9 carries the same stale wording, add an erratum note there.)
- Kafka does NOT redeliver already-fetched messages within a session; "not MarkMessage" only affects the next session's start offset. So nil-handling MUST hold the in-hand message (never skip + wait for broker resend — that loses it for the session).
- `ConsumerGroup.Pause/Resume` exists (consumer_group.go:58-76) as a fallback if the hold-loop approach hits sarama lifecycle issues.
- TDD: contract test first; frequent commits; no push unless asked.

---

## File Structure (jxt-core)

- `sdk/pkg/eventbus/kafka.go` — `consumeWithPipeline` (1193-1228) nil-branch → hold; `resolveWrapper` (1179-1185) kept; legacy `ConsumeClaim` (1008-1070) nil-branch → hold; `activateTopicHandler` (1502-1532) unchanged.
- `sdk/pkg/eventbus/partition_pipeline.go` — **NOT modified** by this plan (Task 3 holds the claim outside `p.run`; the inflight/frontier/commit-prefix invariants are intentionally untouched — review D5a).
- `sdk/pkg/eventbus/consume_with_pipeline_test.go` (new) — pipeline-path contract tests (Task 3).
- `sdk/pkg/eventbus/consume_nil_hold_test.go` (new) — legacy-path contract tests (Task 2).
- `sdk/pkg/eventbus/type.go` — add `HoldBackoff` to the **internal** `PipelineConfig` (default 100ms via `applyPipelineDefaults`; NOT user-facing — timing-field convention, round-2 review D4'). `sdk/config/eventbus.go` is **NOT modified**.
- `CHANGELOG.md` / version — minor bump.

---

## Task 0: STEP-ZERO — delete the 3s warmup sleep (review D6)

**Files:** Modify `sdk/pkg/eventbus/kafka.go` (`startPreSubscriptionConsumer`, ~1451-1468)

**Interfaces:**
- Produces: the consumer no longer blocks 3s on first `Subscribe`, collapsing the drain race window from ~3000ms to µs. Likely fixes this incident alone for the dev/test stack.

This is the highest-leverage single-line fix. Verified by grep: `warmupCompleted` / `IsWarmupCompleted` / `GetWarmupInfo` are referenced **only** inside kafka.go (set at ~1453/1462, read only by the two accessors at ~1474/1481). **Nothing gates control flow on warmup completion** — the sleep is pure cargo-cult telemetry. Deleting it:
- does not change consumption, readiness, or correctness;
- lets `SubscribeAll` finish activating all 14 handlers in µs, so by the time sarama finishes join+sync+rebalance and calls `resolveWrapper`, all wrappers are non-nil → no drain.

NOT structurally 0-loss (a micro-race remains if rebalance beats SubscribeAll) → Tasks 2–3 still needed for 验收1/3.

- [ ] **Step 1: Remove the blocking sleep + its warmup-state bookkeeping.** In `startPreSubscriptionConsumer`, delete `time.Sleep(3 * time.Second)` (~1459) and the surrounding `warmupMu`/`warmupStartTime`/`warmupCompleted` block (~1452-1468). If the warmup accessors (`IsWarmupCompleted`/`GetWarmupInfo`) are part of a public API others consume, keep them returning "completed" (or deprecate) rather than ripping the struct fields — check callers first.
- [ ] **Step 2: Cold-start the query stack on the new jxt-core + run the evidence plan's repro (验收 8).** Expect all aggregates project (no drain). This is the fastest validation that the race window collapsed.
- [ ] **Step 3: Commit**

```bash
git add sdk/pkg/eventbus/kafka.go
git commit -m "fix(eventbus): remove cargo-cult 3s warmup sleep — it IS the dispatch-drain race window (review D6)"
```

---

## Task 1: Per-service prerequisite audit (`pre-subscribe ⊆ activated`)

**Files:** (audit only; per-consumer-service repo)
- Check each of the 5 consuming services.

**Interfaces:**
- Produces: a signed-off finding that every topic in each service's `SetPreSubscriptionTopics` snapshot has an activated handler (or a documented intentional exception).

A1′ changes nil → drain into nil → hold-and-block. A service that **intentionally** pre-subscribes-without-activate (treating drain as a feature) would get partitions permanently stalled by A1′. Audit BEFORE rollout.

- [ ] **Step 1: Enumerate each service's pre-subscribed set vs activated set**

For each consuming service (evidence-management query/command, security-management, file-storage-service, tenant-service), grep:

```bash
# pre-subscribed (snapshot source):
grep -rnE "SetPreSubscriptionTopics|reg\.Register\(" <service>/cmd/api/server.go
# activated (Subscribe/SubscribeEnvelopeWithOptions call sites):
grep -rnE "SubscribeEnvelopeWithOptions|SubscribeEnvelopeWithDLQ|\.Subscribe\(" <service>/cmd/api/<...>
```

- [ ] **Step 2: Confirm `consumed ⊆ activated` per service.**

evidence-management is safe by construction (`ConsumedTopics()` excludes nil-subscribe topics like `evidence.enforcement-type.events`). Verify the other 4 the same way. For any service with an intentional nil-pre-subscribed-but-consumed topic, document it as a conscious exception (it will stall under A1′ and must be removed from the consumed set).

- [ ] **Step 3: Record findings** in the spec's §7 轨二 prerequisite block. Any non-safe service blocks Task 5 (rollout) until its wiring is fixed.

---

## Task 1b: Public `IsActiveTopic()` accessor (review D2 — unblocks evidence plan Task 5)

**Files:** Modify `sdk/pkg/eventbus/kafka.go` (add accessor near `activateTopicHandler`, ~1502)

**Interfaces:**
- Produces: a public `IsActiveTopic(topic string) bool` on the Kafka bus, backed by the existing `activeTopicHandlers` sync.Map. (Name fixed to `IsActiveTopic` — evidence Task 5's interface assertion pins this contract; an `ActivatedTopics()` alias would make the assertion return `ok=false` and crash-loop the service. Review 2026-08-01.)

evidence-management's startup self-check (its Task 5, `consumed ⊆ activated`) needs the bus's activated set, but `activeTopicHandlers` is **unexported** (kafka.go:127) and **no accessor exists today** (grep-confirmed). Without this, the self-check is vacuous (`consumed ⊆ consumed`). Additive method — does NOT break the published-module contract (no removal/rename).

- [ ] **Step 1: Add the accessor** on `*kafkaEventBus`:

```go
// IsActiveTopic reports whether a handler has been activated for topic (pre-subscription mode).
// Exposed so consuming services can self-check consumed ⊆ activated at startup (review D2).
func (k *kafkaEventBus) IsActiveTopic(topic string) bool {
	_, exists := k.activeTopicHandlers.Load(topic)
	return exists
}
```

- [ ] **Step 2: Expose via the `EventBus` interface** if needed (interface assertion in evidence plan Task 5 uses `bus.(interface{ IsActiveTopic(topic string) bool })`), or add it to the interface directly. Choose the assertion form to avoid widening the interface for NATS/Memory backends (they'd add a stub returning `len(activated) > 0` from their own tracking).
- [ ] **Step 3: Unit test + commit.**

```bash
git add sdk/pkg/eventbus/kafka.go
git commit -m "feat(eventbus): expose IsActiveTopic accessor for startup consumed⊆activated self-check (review D2)"
```

> Note: evidence plan Task 5's snippet `_, ok := bus.(interface{...}).IsActiveTopic(topic)` is invalid Go (comma-ok on a single-bool return). Correct form: `it, ok := bus.(interface{ IsActiveTopic(topic string) bool }); return ok && it.IsActiveTopic(topic)`. The evidence plan body is updated to match.

---

## Task 2: Add `HoldBackoff` (internal config) + the `hold-on-nil` helper for the legacy path

**Files:**
- Modify: `sdk/pkg/eventbus/type.go` (internal `PipelineConfig`: add `HoldBackoff`; default in `applyPipelineDefaults`)
- Modify: `sdk/pkg/eventbus/kafka.go` legacy `ConsumeClaim` (1008-1070)

**Interfaces:**
- Produces: legacy `ConsumeClaim` no longer drains on nil — it holds (no `MarkMessage`) until the handler activates.

The legacy path already resolves the wrapper **per-message** (kafka.go:1016). The only bug here is the nil-branch (1017-1021) doing `MarkMessage`+continue. Fix: hold instead.

- [ ] **Step 1: Add internal config field (round-2 review D4' — NOT user-facing).** Add to the **internal** `PipelineConfig` (`sdk/pkg/eventbus/type.go`):

```go
// HoldBackoff is the poll interval while an in-hand message's handler is not yet activated.
// The message is held (not committed) until activation; see dispatch-drop rootfix spec §5 A.
// Timing field — internal-only by repo convention (config/eventbus.go:195-197: timing safety
// invariants stay out of PipelineUserConfig; eventbus.go:1266 drift-trap warning applies).
HoldBackoff time.Duration
```

Default in `applyPipelineDefaults` (type.go:672): `if cfg.HoldBackoff == 0 { cfg.HoldBackoff = 100 * time.Millisecond }`. Extend `validate` for illegal values (mirror `FlushTimeout` handling). Extend `TestApplyPipelineDefaults`: zero → 100ms; explicit value preserved. ⚠️ Read ONLY via `h.eventBus.pipelineConfig()` (kafka.go:978 — applies defaults). NEVER via raw `consumerConfig().Pipeline` — that bypasses `applyPipelineDefaults`, leaving `HoldBackoff=0` → `time.NewTimer(0)` hot-spin in the hold loop.

- [ ] **Step 2: Write the failing contract test (legacy: late activation → no loss)**

Create `sdk/pkg/eventbus/consume_nil_hold_test.go`:

```go
package eventbus

import (
	"context"
	"testing"
	"time"
)

// A message arriving before its handler is activated MUST NOT be committed (lost).
// It is held and processed once the handler activates.
func TestLegacyConsumeClaim_HoldsOnNilThenProcesses_NoLoss(t *testing.T) {
	// CONCRETE invariants (review D4 — regression test, IRON RULE; do NOT t.Skip):
	//  1. Pre-load topic X with N>=3 messages (NOT 1 — N=1 passes falsely even with a
	//     read-and-discard hold; this is the test that catches the D1 bug).
	//  2. Start the claim with X's handler NOT activated; let the hold window elapse.
	//  3. Assert: ZERO MarkMessage calls for X during the hold (no drain, no commit).
	//  4. Activate X's handler.
	//  5. Assert: ALL N messages processed + MarkMessage'd exactly once each (0 loss).
	// Harness: the in-process consumer harness from partition_pipeline_test.go
	// (fake claim yielding N msgs in order, session recording MarkMessage, activeTopicHandlers map).
}

// D3' regression (CRITICAL — added round-2 review): hold wakes on activation, then the topic is
// concurrently deactivated (deactivateTopicHandler, kafka.go:1536) before the re-check.
// Assert: the claim RE-ENTERS hold (0 MarkMessage, in-hand message NOT skipped). A skip +
// later MarkMessage of subsequent offsets would commit past the unprocessed message —
// the incident's own silent-loss class.
func TestLegacyConsumeClaim_DeactivateRaceDuringHold_ReholdsNotSkips(t *testing.T) {
}

func TestLegacyConsumeClaim_NeverActivated_StallsNotDrains(t *testing.T) {
	// D4 negative invariant: a topic whose handler NEVER activates must STALL (partition
	// backpressure, 0 MarkMessage, 0 loss), NOT drain. Assert: after N msgs + no activation,
	// zero MarkMessage; the claim is held (ctx eventually cancels → msgs redelivered next session).
}
```

- [ ] **Step 3: Run test (FAIL — drains today).**

```bash
cd D:/JXT/jxt-evidence-system/jxt-core && go test ./sdk/pkg/eventbus/ -run TestLegacyConsumeClaim_HoldsOnNilThenProcesses -v
```

- [ ] **Step 4: Change legacy nil-branch from drain to hold**

In `sdk/pkg/eventbus/kafka.go` legacy `ConsumeClaim`, replace (around 1015-1021):

```go
// BEFORE (drain — loses the message for the session):
wrapperAny, exists := h.eventBus.activeTopicHandlers.Load(message.Topic)
if !exists {
    session.MarkMessage(message, "")
    continue
}
```

with **backpressure** (review D1: hold the in-hand message WITHOUT reading further from the channel — reading-and-discarding trailing messages loses them for the session in a single-member group; spec §5 A #2 "不拉取下一条"; mirror `partition_pipeline.go:304` `msgCh=nil`):

```go
// AFTER (backpressure hold — never commit unprocessed; never read-and-discard trailing msgs):
wrapperAny, exists := h.eventBus.activeTopicHandlers.Load(message.Topic)
for !exists {
    // Hold the already-read `message`; poll activation WITHOUT touching claim.Messages()
    // so sarama backpressures (trailing msgs stay buffered, 0 loss). On ctx cancel (session
    // end) return — `message` is uncommitted → redelivered next session (at-least-once).
    if err := h.holdUntilActivated(ctx, message.Topic, h.eventBus.pipelineConfig().HoldBackoff); err != nil {
        return err // ctx canceled (session end)
    }
    // D3' (round-2 review): a nil re-check here means a concurrent deactivateTopicHandler
    // (kafka.go:1536) raced the wake — LOOP BACK TO HOLD. Never skip the in-hand message:
    // skip + later MarkMessage of subsequent offsets would commit past it (silent loss).
    wrapperAny, exists = h.eventBus.activeTopicHandlers.Load(message.Topic)
}
wrapper := wrapperAny.(*handlerWrapper)
// process `message` normally (it was held, not lost)
```

Add `holdUntilActivated` (NO channel read — this is the D1 fix; the prior draft's `case msg := <-claim.Messages(); _ = msg` was the bug). **Topic-keyed and SHARED with Task 3 (round-2 review D5' — one helper, one P2#9 contract, one test surface):**

```go
// holdUntilActivated blocks until topic's handler activates or ctx is canceled. Shared by the
// legacy path (Task 2, pass message.Topic) and the pipeline path (Task 3, pass claim.Topic()).
// It does NOT MarkMessage and does NOT read claim.Messages() — trailing messages stay buffered
// in sarama's channel (backpressure, 0 loss); anything already read but unprocessed stays
// uncommitted and is redelivered next session (at-least-once). On ctx cancel it returns
// promptly so sarama's session release (`waitGroup.Wait()`, consumer_group.go:867-891) does not
// block to Rebalance.Timeout (60s) — review P2#9.
func (h *preSubscriptionConsumerHandler) holdUntilActivated(
	ctx context.Context, topic string, backoff time.Duration,
) error {
	t := time.NewTimer(backoff)
	defer t.Stop()
	for {
		if _, exists := h.eventBus.activeTopicHandlers.Load(topic); exists {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			t.Reset(backoff)
		}
	}
}
```

- [ ] **Step 5: Run test (PASS).** Confirm `MarkMessage` is not called during hold, is called once after activation.

- [ ] **Step 6: Commit**

```bash
git add sdk/pkg/eventbus/type.go sdk/pkg/eventbus/kafka.go sdk/pkg/eventbus/consume_nil_hold_test.go sdk/pkg/eventbus/partition_pipeline_test.go
git commit -m "fix(eventbus): legacy ConsumeClaim holds (not drains) on unactivated topic (spec §5 A; P2#9 lifecycle; D3'/D4'/D5')"
```

---

## Task 3: Pipeline path — hold the claim outside `p.run` until activated (review D5a)

**Files:**
- Modify: `sdk/pkg/eventbus/kafka.go` `consumeWithPipeline` (1193-1228) — hold the nil-branch outside `p.run` until activated (no `partition_pipeline.go` change; review D5a)

**Interfaces:**
- Produces: the pipeline path no longer drains a whole session on nil-at-claim-start; it **holds the claim outside `p.run`** until the topic activates, then resolves the topic-constant wrapper once and enters `p.run` (no per-message resolve, no `partition_pipeline.go` change — review D5a).

This is the bug's primary site. Today `consumeWithPipeline` resolves `wrapper` ONCE at claim start (kafka.go:1199) and drains the whole session if nil (1200-1212). The fix (review D5a): **hold the claim OUTSIDE `p.run`** until the topic activates (backpressure — don't read `claim.Messages()`, don't commit, don't advance frontier), then resolve the topic-constant wrapper once and enter `p.run` with it. No per-message resolve inside `p.run`; no `partition_pipeline.go` change.

- [ ] **Step 1: Write the failing contract test (pipeline: session-wide no-loss on late activation, single-member/no-rebalance)**

In `sdk/pkg/eventbus/consume_with_pipeline_test.go`, model the operational reality that the unit test in Task 2 does NOT cover (per spec §8 risk row P2#8/P1#2):
- single consumer member, NO rebalance during the test;
- topic X has a backlog; consumer starts (warmup) with X's handler NOT yet activated;
- assert: X's messages are NOT committed during the hold;
- activate X's handler;
- assert: X's backlog is then processed and committed (no session-wide drain).

```go
func TestConsumeWithPipeline_NoSessionDrain_OnLateActivation_SingleMember(t *testing.T) {
	// CONCRETE invariants (review D4 + spec §8 operational reality; do NOT t.Skip):
	//  1. Single consumer member, NO rebalance during the test (production norm; a rebalance
	//     that re-delivers is a FALSE pass — the operational reality unit tests usually miss).
	//  2. Pre-load topic X with N>=3 messages; activate ONLY topic Y; start consumeWithPipeline
	//     for X's claim; let the warmup/hold window elapse (handler X still off).
	//  3. Assert: ZERO MarkMessage calls for X (no session drain) — messages held, not committed.
	//  4. Activate topic X.
	//  5. Assert: ALL N X messages processed + committed (frontier advances over all N).
	// Harness: in-process pipeline harness from partition_pipeline_test.go.
}

// D6' happy-path regression (added round-2 review): topic activated BEFORE claim start →
// N messages processed + committed immediately; elapsed ≪ HoldBackoff. Guards production's
// highest-frequency path against the new hold branch (existing tests only cover p.run,
// not the consumeWithPipeline entry).
func TestConsumeWithPipeline_ActivatedTopic_RunsImmediately(t *testing.T) {
}
```

- [ ] **Step 2: Run test (FAIL — current code drains X for the whole session).**

```bash
go test ./sdk/pkg/eventbus/ -run TestConsumeWithPipeline_NoSessionDrain -v
```

- [ ] **Step 3: Hold at the `consumeWithPipeline` nil-branch (review D5a — simpler than per-message resolve)**

A `ConsumerGroupClaim` is single-(topic,partition) → the wrapper is **topic-constant for the whole claim** (`resolveWrapper` comment, kafka.go:1177-1178). So "per-message resolve inside `p.run`" is redundant AND would risk the pipeline's inflight/frontier/commit-prefix invariants. The simpler, lower-risk fix: **hold the claim OUTSIDE `p.run`** until activated, then enter `p.run` with the now-non-nil wrapper. **No changes to `partition_pipeline.go`** — this removes the riskiest edit.

Replace the early drain branch in `consumeWithPipeline` (kafka.go:1199-1212):

```go
// BEFORE (drain — loses the whole session for an unactivated topic):
wrapper := h.resolveWrapper(claim)
if wrapper == nil {
    for { select { case msg := <-claim.Messages(): if msg == nil { return nil }; session.MarkMessage(msg, "") } }
}
```

with a backpressure hold (topic-based — at claim start nothing has been read yet, so do NOT consume the head message; poll the topic, then resolve once activation hits):

```go
// AFTER (hold the claim outside p.run until activated; then resolve once and run):
wrapper := h.resolveWrapper(claim)
for wrapper == nil {
    if err := h.holdUntilActivated(ctx, claim.Topic(), cfg.HoldBackoff); err != nil {
        return err // ctx canceled → nothing committed → redelivered next session
    }
    // D3' symmetry: a nil re-resolve here means a deactivate raced the wake — loop back to
    // hold. Nothing has been read or committed at this point, so re-holding is free.
    wrapper = h.resolveWrapper(claim)
}
// ... existing p.run entry unchanged (wrapper non-nil + topic-constant for the whole claim)
```

The hold uses the SAME `holdUntilActivated(ctx, topic, backoff)` helper as Task 2 (round-2 review D5' — no sibling function): the NO-channel-read loop (poll `activeTopicHandlers.Load(topic)` + `ctx.Done` + timer). `cfg` here already came through `pipelineConfig()` (ConsumeClaim, kafka.go:1004), so `HoldBackoff` carries the applied default. The head message stays buffered in sarama (un-fetched) until activation → 0 loss. This is the clean realization of review D1 for the pipeline path.

- [ ] **Step 4: Run test (PASS).** Re-run Task 2's legacy test too (should still pass).

- [ ] **Step 5: Add the sarama session-release regression test (P2#9)**

Assert that on `ctx.Done()`/claim-close during a hold, `ConsumeClaim`/`consumeWithPipeline` returns promptly (well under `Rebalance.Timeout` 60s) so the consumer group session releases without freezing all topics.

- [ ] **Step 6: Commit**

```bash
git add sdk/pkg/eventbus/kafka.go sdk/pkg/eventbus/consume_with_pipeline_test.go
git commit -m "fix(eventbus): pipeline holds claim outside p.run until activated (not drains) on nil (spec §5 A; review D5a; primary site)"
```

---

## Task 4: Optional — warmup/consumer-start decoupling (defer-start)

**Files:**
- Modify: `sdk/pkg/eventbus/kafka.go` `startPreSubscriptionConsumer` (1349-1471) + `Subscribe` (1665-1709)

**Interfaces:**
- Produces (optional): a `StartConsuming(ctx)` API + a `DeferConsumerStart` mode so services can activate all handlers before the consumer claims partitions.

This addresses the "first `Subscribe` starts the consumer + 3s warmup, blocking `SubscribeAll`" coupling. With Task 2-3 done, the system is already safe (nil-hold), so this is **optional hardening** — only do it if the warmup-race's head-of-line blocking is operationally costly.

> **Update post-review (Task 0 done):** Task 0 already **deleted the 3s warmup sleep**, so the "3s warmup" half of this coupling is gone. What remains here is the **defer-start / `StartConsuming(ctx)` API** (let a service activate ALL handlers before the consumer claims partitions) — deeper structural hardening that makes the race architecturally impossible rather than merely collapsed. Still optional; Task 0 + Tasks 2–3 already deliver a safe system.

- [ ] **Step 1: Decide go/no-go.** If Task 2-3 make the warmup race harmless (they do — nil-hold), this task is YAGNI for correctness. Do it only for performance (avoid head-of-line stall during the brief startup race). If no-go, skip to Task 5 and note as deferred.

- [ ] **Step 2 (if go): Add `StartConsuming`** — split `Subscribe` so activation (`activateTopicHandler` + `addTopicToPreSubscription`) does NOT call `startPreSubscriptionConsumer`; the service calls `StartConsuming(ctx)` once after all handlers are registered. Add a bus config flag `consumer.defer_start` (default false = current behavior).

- [ ] **Step 3 (if go): Test + commit.**

---

## Task 5: Version bump + release + 5-service rollout + 回切

**Files:**
- Modify: jxt-core `VERSION` / git tag (new minor, e.g. v1.8.0).
- Modify (downstream, after release): each service's `go.mod` jxt-core pin.

**Interfaces:**
- Produces: published jxt-core version with the fix; 5 services bumped; evidence-management query `pipeline.enabled` 回切 `true`.

- [ ] **Step 1: Bump jxt-core version + update CHANGELOG** noting the behavior change (nil-topic messages are now held, not drained — any service relying on drain-as-feature must read the audit in Task 1).

- [ ] **Step 2: Tag + publish** the new version per jxt-core's release process.

- [ ] **Step 3: Canary on one service** — order follows the evidence plan's Task 0 verdict (review 2026-08-01): if prod command is pipeline-ON (prod-exposed), canary **evidence-command first** (it is the exposed service and cannot use the query stopgap per R2-6); otherwise canary **evidence-query first**. Run the evidence plan's repro test (验收 8) on pipeline-ON + the new jxt-core → expect all aggregates project, 0 loss.

- [ ] **Step 4: Roll out to all 5 services** (canary each; watch the evidence plan's Layer D freshness metric — `evidence_event_processing_freshness_seconds` — and the per-service `consumed ⊆ activated` self-check).

- [ ] **Step 5: 回切 (exit the legacy interim)** — once the new jxt-core is rolled out everywhere, set evidence-management `query/config/settings.yml` `eventbus.consumer.pipeline.enabled` back to `true`; re-run evidence plan 验收 1 (late-activation no-loss) and 验收 3 (crash/restart 0-loss) on pipeline-ON; confirm.

- [ ] **Step 6: Incident closure** — close only when 验收 1/2/3/6 are met (per spec §7 事故关闭标准), not merely when tests 复绿.

- [ ] **Step 7: Commit downstream pin bumps** in each service repo.

```bash
# per service:
go get github.com/ChenBigdata421/jxt-core@v<new>
go build ./... && go mod tidy
git commit -m "chore(deps): bump jxt-core for dispatch nil-hold fix"
```

---

## Self-Review (run after writing)

- **Spec coverage:** Layer A (hold-on-nil; pipeline path holds outside `p.run` per D5a) → Tasks 2-3; warmup decoupling → Task 4; contract tests (late activation, single-member/no-rebalance, session release) → Tasks 2-3; per-service audit → Task 1 (spec §7 轨二前置); version bump + 5-service + 回切 → Task 5 (spec §7 轨二终态). ✓
- **Placeholders:** Task 2 step 4 has complete code; Task 3 step 3 is guided by the contract test (TDD — the test is concrete, the impl is "make it pass against partition_pipeline.go"); `t.Skip` skeletons are marked as such for the harness-bound tests (not hidden TODOs).
- **Type consistency:** `holdUntilActivated` (single shared helper, topic-keyed — D5'), `HoldBackoff` (internal `PipelineConfig`, defaulted in `applyPipelineDefaults`, read via `pipelineConfig()` — D4'), `StartConsuming` named consistently.
- **Precondition:** the whole plan is gated by the evidence plan's bisect — clearly stated in the header + Task 1.

---

## Deferred / Open Questions

### From 2026-08-01 review (round 1)

- **Grep-only audit misses stalls on 4 of 5 services** — jxt-core plan Task 1; evidence plan Task 5/Task 7 (P2, adversarial, confidence 75) — **✅ RESOLVED in round 2 (D7'): tracked as `TODOS.md` F4** (runtime `consumed⊆activated` self-check + partition-stall signal on all 5 services; depends on Task 1b's `IsActiveTopic`).

  Layer A flips nil→drain into nil→hold-and-block across ALL 5 consuming services. Task 1's rollout gate is a static grep of `SetPreSubscriptionTopics` + `Subscribe` call sites plus a sign-off. A grep cannot see (a) a service that intentionally pre-subscribes a publish-only topic without activating a consume handler, or (b) a runtime DI/wiring failure where a `Subscribe` closure exists in source but never activates at boot. Under nil-hold both cases stall the partition PERMANENTLY, and the only detector is the Layer D freshness probe — which the plan ships ONLY on evidence-management (Task 7). The fix therefore swaps one silent failure (drain, data lost) for another (stall, data frozen) on 4 of 5 services, behind the same lag-0/Stable mask the original incident hid behind. Deferred (not auto-applied) because the resolution — shipping a runtime `consumed⊆activated` self-check + partition-stall signal on ALL 5 services as the rollout gate — expands scope across 4 repos this plan does not own; that is an architectural/scope decision for the user, not a defensible auto-edit.

  <!-- dedup-key: section="jxtcore plan task 1 evidence plan task 5task 7" title="greponly audit misses stalls on 4 of 5 services" evidence="jxt-core plan Task 1 Step 1: 'grep -rnE \"SetPreSubscriptionTopics|reg.Register(\" <service>/cmd/api/server.go' and 'grep -rn" -->

---

## GSTACK REVIEW REPORT

Review date: 2026-08-01 | Reviewer: `/plan-eng-review` (deep mode, code-verified) + independent outside-voice subagent. **Full report + evidence-side decisions: see `evidence-management/docs/superpowers/plans/2026-07-31-dispatch-dropfix-evidence.md`.** This file records the jxt-core-relevant decisions.

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | CLEAR (PLAN) | 5 jxt-core decisions folded (D1, D2, D4, D5a, D6); D8 conditions this plan's track |
| Outside Voice | Claude subagent | Independent 2nd opinion | 1 | issues_found | verified D1; supplied D5a (simpler Task 3) + D6 (warmup step-zero) |
| Eng Review (round 2) | `/plan-eng-review` | Re-review vs actual code, 2026-08-01 | 1 | issues_found → resolved | 5 findings, all user-approved and folded into the plan body: D2' D3' D4' D5' D6'; plus D7' (TODOS.md F4) |
| Outside Voice (round 2) | Codex CLI | Independent 2nd opinion | 1 | SKIPPED | Codex network reconnect failure; user chose to skip. No cross-model pass this round. |

**VERDICT: ENG CLEARED (PLAN) — ready to implement after applying D1, D2, D4, D5a, D6 (round 1) + D2', D3', D4', D5', D6' (round 2, already folded into the plan body above).** D8 decides whether this plan is incident-tracked or routine hardening.

### Round-2 decisions (2026-08-01, code-verified against kafka.go/type.go/eventbus.go)

- **D2' — Global Constraint line rewritten (internal contradiction).** The prior constraint mandated hold loops keep `case msg := <-claim.Messages()` — directly contradicting D1 and the Task 2/3 code; a checklist-following worker would reintroduce the read-and-discard loss bug. Rewritten: hold loops select `session.Context().Done()` + timer only, never read `claim.Messages()`. Spec §5 A P2#9 carries the same stale wording → add erratum there. [conf 9]
- **D3' — Post-hold re-check failure = deactivate race → re-hold, never skip.** `holdUntilActivated` returns nil only on activation, so a failed re-check can only mean a concurrent `deactivateTopicHandler` (kafka.go:1536). The prior `continue` would read the next message and later MarkMessage would commit past the skipped in-hand message — silent loss. Both Task 2 and Task 3 now loop back to hold. New CRITICAL regression test `TestLegacyConsumeClaim_DeactivateRaceDuringHold_ReholdsNotSkips`. [conf 8]
- **D4' — `HoldBackoff` is an internal timing field, not user config.** The prior draft put it in `PipelineUserConfig` (violating the repo convention, config/eventbus.go:195-197) and read it via raw `consumerConfig().Pipeline` (bypassing `applyPipelineDefaults` → default 100ms never applies → `time.NewTimer(0)` hot-spin). Now: internal `PipelineConfig` only, defaulted in `applyPipelineDefaults`, read via `pipelineConfig()`, `TestApplyPipelineDefaults` extended. `sdk/config/eventbus.go` untouched. [conf 9]
- **D5' — One hold helper, not two.** `holdUntilActivated` and `holdTopicUntilActivated` had identical loop bodies (the Task 2 variant used only `first.Topic`). Merged into one topic-keyed `holdUntilActivated(ctx, topic, backoff)` shared by both paths — one P2#9 contract, one test surface. [conf 9]
- **D6' — Entry-level happy-path regression test.** Existing pipeline tests cover `p.run` only; the new hold branch sits in the `consumeWithPipeline` entry. Added `TestConsumeWithPipeline_ActivatedTopic_RunsImmediately` (activated topic → immediate processing, elapsed ≪ HoldBackoff) so a happy-path regression is caught in jxt-core CI, not the cross-repo 验收 8. [conf 7]
- **D7' — Round-1 deferred finding converted to `TODOS.md` F4** (runtime `consumed⊆activated` self-check + stall signal on all 5 services). Not bundled into this rollout (spans 4 repos this plan does not own); rollout relies on per-service canary observation until F4 lands. [conf 7]
- **Round-2 verification notes:** all plan line references re-verified against current kafka.go (1016-1021, 1199-1212, 1459, 1177-1185) ✓; Task 1b type-assertion viability confirmed (`NewEventBus` returns `*kafkaEventBus` directly for kafka, eventbus.go:106) ✓; bonus for Task 0: the 3s sleep runs while HOLDING `k.consumerMu` (Lock at kafka.go:1351, sleep at 1459) — deleting it also removes a 3s lock convoy blocking subsequent `Subscribe` calls ✓.

### jxt-core decisions folded

- **D6 — STEP-ZERO: delete the 3s warmup sleep (kafka.go:1459).** Verified: `warmupCompleted`/`IsWarmupCompleted`/`GetWarmupInfo` gate NOTHING (only read by two telemetry accessors). The sleep IS the race window; deleting it collapses the window to µs and likely fixes the incident alone. Promote ahead of Tasks 2–5. Not structurally 0-loss (micro-race) → Tasks 2–3 still needed for 验收1/3. [conf 9, outside-voice]
- **D1 — Hold = backpressure, NOT read-and-discard.** `holdUntilActivated` (Task 2 Step 4) must NOT do `case msg := <-claim.Messages(); _ = msg` (loses trailing messages for the session in a single-member group → fails 验收1/3, contradicts spec §5 A #2 "不拉取下一条"). Use `msgCh=nil` (stop reading) → sarama backpressures → messages buffered, nothing lost. [conf 9]
- **D5a — Task 3 simpler shape (realizes D1).** A `ConsumerGroupClaim` is single-(topic,partition) → wrapper is topic-constant (resolveWrapper comment, kafka.go:1177). So do NOT pass a per-message resolver into `p.run`; instead, in `consumeWithPipeline`'s nil-branch (1200–1212) **hold the claim OUTSIDE `p.run`** (msgCh=nil backpressure) until `activeTopicHandlers.Load(topic)` hits, then enter `p.run` with the non-nil wrapper. **No changes to partition_pipeline.go's inflight/frontier/commit-prefix invariants** — removes the riskiest edit. [conf 8, outside-voice]
- **D4 — Contract tests concrete, not `t.Skip`.** Four invariants: (1) **N≥2** trailing messages survive the hold (N=1 passes falsely — catches D1), (2) single-member / no-rebalance harness (operational reality; a rebalance re-deliver is a false pass), (3) session-release ≪60s on ctx.Done/claim-close (P2#9), (4) negative: never-activated → partition stalls (not drains). IRON RULE — regression fix mandates concrete regression tests. [conf 9]
- **D2 — Add a public `IsActiveTopic(topic string) bool` accessor** as a new task (between current Task 1 and Task 2), backed by `activeTopicHandlers`. evidence-management's Task 5 self-check needs it (the map is unexported, kafka.go:127; no accessor exists today). Additive method — does NOT break the published-module contract. [conf 9]

### Carry-in from the evidence plan
- **D8 — This plan's TRACK is conditional** on (a) the evidence plan's race-disciplined bisect result and (b) PROD command pipeline-ON status (evidence Task 9 verifies). If prod command is pipeline-ON (memory: D3-A prod-readiness shipped → likely), this plan stays incident-tracked (command has no query-lever per R2-6; only Layer A fixes command's drain). If dev/test-only, this plan de-scopes to routine jxt-core hardening.
- **D7 — Bisect race-discipline** (evidence Task 1): both `c2b2910^` and `c2b2910` pin jxt-core **v1.1.71** (verified — `afdbcfc` is after `c2b2910`), so the version pin is NOT a confound; don't add a pin step.

### Version strategy (spec §9 #1, still open for jxt-core maintainer)
Ship as a new minor (behavior fix: nil-topic messages are held, not drained). No opt-in flag for the old footgun — drain is a footgun, not a feature. The Task 1 per-service audit (`pre-subscribe ⊆ activated`) gates Task 5 (rollout) — any service intentionally pre-subscribes-without-activate would stall under the new behavior and must be fixed first.

**UNRESOLVED DECISIONS:**
- D8: this plan's track (incident vs routine hardening) pending the evidence plan's race-disciplined bisect result + prod command pipeline-ON status verification.
