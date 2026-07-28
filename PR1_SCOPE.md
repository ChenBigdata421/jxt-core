# PR-1 Scope & PR-2 Carry-over

Plan: `docs/superpowers/plans/2026-07-27-pr1-jxt-core-delivery-contract.md`
Branch: `feat/pr1-delivery-contract` (base master = 455c365, 8 commits b351afd..9169072).
Spec: `docs/opus5-可靠消费契约-v2-20260726.md` §1 (M15), §2.2 (C1/M2), §7, §8.5 (C1~C6), §11.

## What PR-1 delivered

- **M15 — Delivery contract** (EventBus): `MessageHeader` / `RawMeta` / `EnvelopeDelivery` /
  `EnvelopeDeliveryHandler` / `toRawMeta` / `invokeDelivery` / `errInvalidEnvelope` +
  `EnvelopeDeliveryOptionsSubscriber` capability interface; Kafka fills every `RawMeta` field
  (raw key/value, ordered duplicate-preserving headers, topic/partition/offset/timestamp,
  sha256 payload hash). Actor pool threads `Raw` + `DeliveryHandler` as explicit struct fields
  (no context key — M14).
- **C6 — PoisonMessage fidelity**: `Headers` changed from `map[string]string` to `[]MessageHeader`
  (preserves order + duplicate keys); `Timestamp time.Time` added; Key/Value defensively copied.
- **C4 — fail-fast contract**: only `kafkaEventBus` implements `EnvelopeDeliveryOptionsSubscriber`;
  memory/nats do not — reliable subscription fails fast rather than silently downgrading.
- **C3 — NoOpDLQHandler removed**: deleted; `DefaultSchedulerConfig` now `EnableDLQ=false` +
  `DLQHandler=nil`; `Validate()` rejects `EnableDLQ=true` with nil handler (fail-fast at
  `NewScheduler` construction). `NoOpDLQAlertHandler` retained.
- **C2 — alert on Handle failure**: `processDLQ` no longer `continue`s past `Alert` on `Handle`
  error; the alert always fires. Preserved through the Task 8 C1 rewrite (Alert `if` is a sibling
  of the Handle `if`, gated only by `notifyOK` for the notify-mark).
- **C1 — dead_lettered terminal + notification split**: `EventStatusDeadLettered` +
  `DeadLetteredAt`/`DlqNotifiedAt` columns + migration 003; `OutboxRepository` gains
  `MarkAsDeadLettered` (CAS once) / `FindUnnotifiedDeadLettered` / `MarkDeadLetterNotified` (CAS).
  `processDLQ` rewritten as two-step: CAS `max_retry→dead_lettered` (terminal fact), then
  notification against `dlq_notified_at IS NULL` (Handle+Alert), marked only on success — a crash
  during notification is recovered next loop, with no orphaning intermediate state.

## Verification (Task 9)

- `go build ./...` clean; `go vet ./...` clean (only pre-existing warnings: `storage/cache`
  lock-copy, `sdk/pkg/ws` context-leak — unrelated).
- outbox full suite PASS; eventbus new broker-free tests PASS (13 new tests) + existing
  `TestRun_` partition-pipeline suite PASS (production path intact).
- **Cross-repo compile gate** (uncomment `replace => ../jxt-core`, build, revert): file-storage-service
  EXIT=0, security-management EXIT=0, evidence-management (command/shared/query) EXIT=0 — all
  build clean against the local jxt-core with every breaking change applied. process-management
  refused on pre-existing v1.1.46 version-skew (`go mod tidy` needed); it wraps jxt-core's own
  `GormOutboxRepository` so it cannot be broken by the interface expansion.
- **Dependency hygiene (J2)**: outbox has no prometheus/gin deps. eventbus DOES transitively pull
  gin-gonic + prometheus, but these are **pre-existing** (from `actor_pool_metrics.go`,
  `nats_metrics.go`, `metrics_prometheus_example.go`, none touched by this PR) — NOT introduced
  by PR-1, so no J2 violation. (Plan Task 9 Step 3's "OK: no prometheus/gin in eventbus"
  expectation was inaccurate about the pre-existing state.)
- **M14**: no `context.WithValue` in any new code (`delivery.go`, `headers.go`, etc.).

## Spec §11 PR-1 acceptance items deferred to PR-2

These 5 items reference `sdk/pkg/reliable` artifacts that PR-2 creates — explicitly out of PR-1:

- Dependency-hygiene gate `go list -deps ./sdk/pkg/reliable` → PR-2 (reliable package not yet created).
- No-context-key gate `sdk/pkg/reliable/**` → PR-2.
- `TryClaim` rejects external transactions gate → PR-2 (TryClaim is T4).
- Inline-invariant tests (event_consumption 4-state field combinations) → PR-2 (T5).
- C5 skip-package tests (`ErrSkip`/`PermanentError`/`RetryableError`) → PR-2 (T4).
- **C5 body** (`ErrDuplicateKey` cross-service split): both definitions live in service modules
  (`evidence-management/shared/domain/idempotency/...` and `process-management/shared/domain/idempotency/...`),
  not jxt-core; their single home is `sdk/pkg/reliable` (PR-2). PR-2 defines `reliable.ErrDuplicateKey`
  and migrates the evidence-management live reference; PR-6 deletes the process-management dead copy.

## PR-2 / follow-up carry-over (from review)

- **OV#6 — multi-instance double-Handle**: `processDLQ` step 2 (`FindUnnotifiedDeadLettered`) has
  no claim/lease; two scheduler instances would double-Handle dead letters. PR-1 guarantees
  single-instance Handle-once only; the limitation is documented in the `processDLQ` comment.
  Multi-instance needs Handle/Alert idempotency by EventID (PR-2 `OutboxRecordingHandler`) or a
  durable DB claim/lease (`SELECT FOR UPDATE SKIP LOCKED` — see TODOS). PR-1 deliberately did not
  fail-fast here (unlike C3/C4) — runbook must state the single-instance DLQ requirement.
- **eager sha256 (D9 / Finding 7)**: `toRawMeta` computes `PayloadHash` + full payload copy on
  every message (happy path). Baseline below. For large payloads the 1MB case is ~1.2ms; if a
  future delivery subscriber shows throughput regression, make `PayloadHash` lazy (compute only
  on the failure path that persists it). Fidelity contract unchanged.
- **MessageHeader JSON contract (OV#9)**: when PR-2 persists `RawMeta.Headers` to a `headers JSON`
  column, `Value []byte` will encode as base64 via `encoding/json`. Order + duplicate keys survive,
  but add a MarshalJSON/round-trip contract test so consumers don't misread it as raw bytes.
- **delivery reconnect restore (D10) — FIXED in PR-1** (was a PR-2 carry-over): `restoreSubscriptions`
  now stores a **restorer closure** per topic in `k.subscriptions` (capturing the original handler +
  opts), and restores by **snapshot -> Delete -> call restorer**. This dispatches plain/envelope/delivery
  by the captured subscribe path, so delivery topics are no longer skipped + Error-logged. Verified by
  `TestRestoreSubscriptions_RestoresAllKinds` (broker-free seam).
- **Pre-existing reconnect restore defect (F6) — also FIXED**: the same Delete-before-restore change
  closes the historic `LoadOrStore` -> "already subscribed" guard bug (reconnect restoration was broken
  for any non-empty subscription set). Both fixes ride the same `restoreSubscriptions` rewrite.
- **reconnect consumer-group rebuild (D10-complete)**: `reinitializeConnection` now also closes the old
  `unifiedConsumerGroup` and rebuilds it from the new client via `sarama.NewConsumerGroupFromClient`
  (stored in `k.unifiedConsumerGroup`). Without this the consume loop kept re-`Consume`-ing the dead
  group bound to the closed client, so handler re-registration alone did not resume real consumption.
  Verified by `TestConsumeLoop_ReadsReplacedConsumerGroup` (fake ConsumerGroup + real loop, broker-free).
- **D11 — `NewSchedulerChecked`** (panic → error): not in PR-1; logged to root `TODOS.md`.
- **D12 — gorm repository UTC timestamps**: PR-1's new repo methods use `time.Now().UTC()`; the
  pre-existing `MarkAsMaxRetry`/`MarkBatchAsPublished` use bare `time.Now()`. Normalize repo-wide
  to UTC in a separate pass; logged to root `TODOS.md`.

## Review Minor findings (not fixed; non-blocking)

- Task 1: optional direct unit test for `saramaToMessageHeaders` (nil-skip / empty input); current
  coverage via `toPoisonMessage` is sufficient.
- Task 5: `scheduler.go` `DLQHandler` field comment still says "可选"/optional — now
  required-when-`EnableDLQ`; 1-line doc fix.
- Task 7: `MockRepository.FindUnnotifiedDeadLettered` iterates a map without sorting (non-deterministic
  order vs gorm's `id ASC`); no current test exercises multi-event ordering. MySQL migration index is
  non-partial (MySQL lacks partial indexes — intentional).
- Task 8: redundant `len(unnotified)==0` early-exit; tests discard `FindUnnotifiedDeadLettered` error
  (stub never errors).

## Seams PR-1 laid for PR-2

- `MessageHeader` / `RawMeta` / `EnvelopeDelivery` / `EnvelopeDeliveryOptionsSubscriber` (eventbus).
- Actor pool already threads `Raw` + `DeliveryHandler` (no context key — M14 held).
- `dispatchMessage` / `deliveryRouting` are pure and unit-tested without a broker.
- Outbox `dead_lettered` terminal + notification split (PR-6 security/process/evidence/file-storage
  adoption points).
- `errInvalidEnvelope` sentinel distinguishes poison-message (→ DLQ) from handler-failure (→ retry) —
  the PR-2 reliable-kernel fork contract.

## Performance baseline (D9, `go test -bench=BenchmarkToRawMeta ./sdk/pkg/eventbus/`)

Machine: AMD Ryzen AI 9 HX 370 (windows/amd64), Go 1.26.

| payload | ns/op     | B/op     | allocs/op |
|---------|-----------|----------|-----------|
| 1KB     | 1482      | 1216     | 6         |
| 64KB    | 72796     | 65728    | 6         |
| 1MB     | 1190181   | 1048770  | 6         |

Judgment: PR-1 has no delivery subscriber, so this table is only a pre-PR-2-grayroll baseline, not a
regression gate. Cost scales linearly with payload (defensive copy of `Value`); allocs are constant
(6). If the 1MB case (~1.2ms) shows up as throughput regression once a delivery subscriber goes live,
make `PayloadHash` lazy before enabling delivery in production.
