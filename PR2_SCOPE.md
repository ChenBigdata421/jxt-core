# PR-2 Scope & PR-3 / PR-7 Carry-over

Plan: `docs/superpowers/plans/2026-07-28-pr2-jxt-core-reliable-kernel-dual-dialect-store.md`
Branch: `feat/reliable-kernel-pr2` (base master = `5611d7c`, 12 commits …`785b604` + this task).
Spec: `docs/opus5-可靠消费契约-v2-20260726.md` §2 (schema), §3.1/§3.3 (Store/TryClaim), §6 (replay
safety), §8.4 (MarkFailed matrix), §10 (metrics), §11 (PR-2 acceptance).

## What PR-2 delivered

- **Kernel root** (`sdk/pkg/reliable/`) — zero infra deps (J2 green):
  - `errors.go` — sentinels (`ErrSkip`/`ErrDuplicateKey`/`ErrRetryLater`/`ErrNotPermitted`/
    `ErrNotSelfReplayable`/`ErrConflict`) + typed wrappers (`PermanentError`/`RetryableError`/
    `DuplicateKeyError`) so dup-detection is `errors.As` not string-match (D3).
  - `key.go` — `HandlerID`/`Key`/`Meta`/`ClaimInput`/`ClaimToken`/`Decision`/`AggregateGateKey`.
  - `state.go` — five-state machine (`PROCESSING`/`SUCCEEDED`/`RETRY_SCHEDULED`/`DEAD_LETTER`/
    `DISCARDED`) + attempt/backoff oracle (capped exponential + jitter seeded by attempt).
  - `safety.go` — `ReplaySafety` enum (`Idempotent`/`Deterministic`/`ExternalEffect`/`NotSelfReplayable`).
  - `classify.go` — two-level `ErrorClassifier` (`ErrorClass` × `ReplaySafety`) + `IsDuplicateKey`.
  - `metrics.go` — metric-name constants aligned verbatim with §10; `NoOpMetrics` default.
- **Store abstraction** (`sdk/pkg/reliable/store/`):
  - `store.go` — `Store` + `QuarantineStore` interfaces; `Row` model (`store/row.go`).
  - Signatures documented inline with the spec deviations they carry (see "Spec deviations" below).
- **gormshared core** (`sdk/pkg/reliable/store/gormshared/`) — the shared ~500-line heart
  (D17): `store.go` (TryClaim with inline CAS reclaim + D4 LEASE_ORPHAN anomaly, FindEligibleHeads
  with D5 dead-branch removal, MarkFailed/MarkSucceeded/ScheduleReplay/Discard, ObserveExpiredLeases
  batch insert (LEASE_ORPHAN only — STUCK_PROCESSING reverted to PR-7, review R3), AcquireAggregateGate/ReleaseAggregateGate,
  QuarantineStore), `model.go` (GORM tags + anomaly model with `uk_anomaly_once`), `fingerprint.go`
  (D10 sha256 64-hex + D11 PII redactor), `now.go` (UTC oracle), `quarantine.go`.
- **Thin dialect packages** (D17):
  - `store/mysql/` — `classify.go` (MySQL `1062` dup-key → `IsDuplicateKey`), `migration.go`
    (DDL with `idx_due`/`idx_ops`/`idx_aggregate` D22 index design), `NewStore`.
  - `store/postgres/` — `classify.go` (PG `23505` dup-key + `25P02` tx-in-abort mapping),
    `migration.go` (partial `idx_due WHERE status='RETRY_SCHEDULED'`, snake_case
    `broker_timestamp`), `NewStore`.
- **Conformance** (`sdk/pkg/reliable/store/repotest/`) — D7 single-`ConformanceDeps` rewrite;
  dual-dialect (MySQL + PostgreSQL) invariant suite + §3.3 bidirectional (independent-commit
  pooled-visible + forbidden-case tx-join) + quarantine + `explain_test.go` (D22 deterministic
  EXPLAIN gate, 2K-row seed). **Controller-verified 48/48 pass on real MySQL+PG (2026-07-28).**
- **Lease runner** (`sdk/pkg/reliable/lease/`) — D20: `ObserveExpiredLeases` only records
  anomalies (`LEASE_ORPHAN` batch only — STUCK_PROCESSING + `RecoverStuckProcessing` reverted to
  PR-7, review R3); does NOT mutate row status or ownership. The sole re-claim path is
  `TryClaim`'s inline CAS.
- **Replay scheduler** (`sdk/pkg/reliable/replay/`) — eligible-head `Tick`/`Run` loop;
  aggregate gate acquired BEFORE `ClaimForReplay` (A3); three post-claim branches via Store
  methods (D8 — no raw SQL bypass): `MarkSucceeded` / `MarkFailed` (matrix decides
  RETRY_SCHEDULED vs DEAD_LETTER) / `MoveToDeadLetterWithToken`; pre-claim `RetryLater`→
  `AdvanceDue`, post-claim `RetryLater`→`ReleaseClaim` (A3); `NotPermitted`→`MoveToDeadLetter`
  with payload/error_class guard (A2 — no empty spin).
- **CI gates** (`sdk/pkg/reliable/gates_test.go` + `scripts/reliable_deps_gate.sh`) —
  Go-native cross-platform gate suite (J2 kernel-zero-deps / M14 no-context-key /
  §3.3 TryClaim-no-`*gorm.DB` / D9 no-placeholder) with `TestGate_SelfCheck` (injects violation
  samples, asserts the patterns fire — guards against a gate whose judgment is inverted or whose
  path is broken silently passing). Bash fallback for Linux CI.

## Verification (Task 12, 2026-07-28)

Commands run from `jxt-core/` on Windows (Git Bash); CGO_ENABLED=0 (no `-race`; reliable is pure Go).

- `go build ./sdk/pkg/reliable/...` → **EXIT 0**.
- `go vet ./sdk/pkg/reliable/...` → **EXIT 0** (clean).
- `go test -run '^$' ./sdk/pkg/reliable/store/repotest/...` → **EXIT 0** (compile-check; DB
  suite needs Docker/env DSNs — controller already verified 48/48 pass on real MySQL+PG; not
  re-run here to avoid the broker/DB hang that bare `go test ./sdk/pkg/eventbus/` exhibits).
- Non-DB unit tests → **EXIT 0**, 44 tests PASS:
  - reliable root 20 (15 kernel + 5 gates incl. SelfCheck),
  - gormshared 4 (fingerprint sha256 stability, secret-invariance, PII redaction 14 sub-cases, UTF-8 truncation),
  - mysql 5 + postgres 5 (classifier mapping + ErrorCode + IsDuplicateKey + kernel-plug-in),
  - lease 3, replay 5 (three-branch + gate-miss-no-side-effect + ReleaseClaim post-claim),
  - internal/crosspackage 2 (C5 sentinel + typed-wrapper cross-package).
- **Go gates** (`go test ./sdk/pkg/reliable/ -run TestGate_ -v`) → 5/5 PASS.
- **Bash gate** (`bash scripts/reliable_deps_gate.sh`) → `ALL GATES GREEN`, EXIT 0.
- **D23 dependency pin**: `go list -m gorm.io/gorm` = `v1.24.2`, `gorm.io/driver/postgres` =
  `v1.4.5`, `gorm.io/driver/mysql` = `v1.4.4` — gorm not bumped by PR-2.
- **J2 kernel root**: `go list -deps ./sdk/pkg/reliable` shows no `gorm.io`/`prometheus`/
  `gin-gonic`/`IBM/sarama`/`nats-io`.
- **Cross-repo compile** (temporary `replace github.com/ChenBigdata421/jxt-core => ../jxt-core`,
  `go build ./...`, then precise revert): `evidence-management/shared` **EXIT 0**,
  `security-management` **EXIT 0** (the latter needed a one-shot `go mod tidy` to pull local
  jxt-core's transitive sums; go.mod/go.sum restored to HEAD via `git checkout` since security
  had no pre-existing dirt). Neither sibling imports `sdk/pkg/reliable` yet (PR-3 wires it), so
  this is a regression check (adding reliable didn't break the sibling build), not consumption.
- **Pre-existing out-of-scope breakage**: `sdk/pkg/contextpool` does not compile (sync.Map vs
  map) and bare `go test ./sdk/pkg/eventbus/` hangs without a broker. Both pre-date PR-2;
  scoped gates above are unaffected.

## Spec §11 PR-2 acceptance mapping

Covered in PR-2: state machine kernel, `Store`/`QuarantineStore` abstraction, gormshared +
thin MySQL/PostgreSQL dialect packages, dual-dialect conformance, lease orphan observation
runner, eligible-head scheduler with three-branch disposition, aggregate gate, J2/M14/§3.3/D9
gates.

**Explicitly deferred to PR-3** (D18#3 — these reference consumers/wiring that don't exist until
PR-3 lays the §4 skeleton):
- ⑮ `EventBusDLQAdapter` — bridges jxt-core EventBus DLQ to reliable quarantine store.
- ⑰ `OutboxRecordingHandler` — records outbox events into `event_consumption` on the write side.
- ⑯/⑳ batch replay + opsvc dead-letter administration endpoints.
- Multi-tenant fan-out orchestration (D6 — per-Store core; PR-3 wires the tenant → Store cache).
- Scheduler live-behavior verification (real broker end-to-end) — needs the PR-3 wiring.

**Deferred to PR-7** (D15): `TryClaim` single-RTT optimization — the current implementation is
correctness-first (bounded retry ≤3 on dup-key/serialization conflict); the perf gate lands in
PR-7 once a production delivery subscriber exists to anchor the benchmark.

## Carry-over to PR-3

- §4 skeleton (the `reliable` consumer package that ties kernel + store + scheduler together
  for service modules).
- `EventBusDLQAdapter` (⑮) + `OutboxRecordingHandler` (⑰).
- Batch replay (⑯) + opsvc dead-letter admin (⑳).
- Multi-tenant fan-out: tenant → `*gormshared.GormStore` cache + lifecycle (D6).
- Scheduler live-behavior verification against a real broker (CI DSN contract —
  `TestConformance_AllDialects` runs on real MySQL+PG via env DSNs; PR-3 adds the broker leg).
- **New `tests/reliable/` integration/regression dir** (mirrors `tests/outbox` + `tests/eventbus`),
  deferred to PR-3 because the end-to-end surface it exercises doesn't exist until the §4 consumer
  + adapters land. PR-2's own surface is already covered by `sdk/pkg/reliable/store/repotest/`
  dual-dialect conformance (the store-level regression suite, runs on real MySQL+PG via
  testcontainers) + inline unit tests + `gates_test.go`; repotest is a reusable harness and stays
  in `sdk/pkg/`, not `tests/`. Two tiers to add in PR-3:
  - `tests/reliable/system_tests/` (`//go:build system`, skip-if-down, **not** in default
    `go test ./...`): full pipeline broker → outbox → reliable consumer → store → quarantine,
    against docker-compose RedPanda + DB — the only layer that catches cross-component wiring
    drift (EventBusDLQAdapter / OutboxRecordingHandler, lease reclaim under real redelivery).
  - `tests/reliable/reliability_regression_tests/` (mirrors `tests/eventbus/reliability_regression_tests`):
    broker fault injection — consumer crash mid-processing, lease orphan + inline reclaim,
    double-delivery idempotency, dead-letter/quarantine under real broker redelivery.
  - Infra choice for PR-3: prefer testcontainers (consistent with repotest — one Docker daemon,
    no `docker-compose up` prerequisite) over docker-compose services; decide when the dir is created.
- `MarkFailed` repo-wide UTC timestamp normalization (carry-over from PR-1 TODOS; PR-2's new
  methods already use `nowUTC()`).

## Carry-over to PR-7

- `TryClaim` single-RTT optimization (D15) — current bounded-retry implementation is correct
  but not single-RTT; perf gate lands with the first production delivery subscriber.
- Deterministic EXPLAIN gate hardening (D22 seeded `explain_test.go` at 2K rows; PR-7 raises the
  seed toward production volume and adds buffer-ratio assertions once the index design is
  validated under load).

## Spec deviations carried into the immutable tag (R2)

These are **PR-2 authorized deviations pending spec revision** — the controller presents the
`v1.1.68` (final) vs `v1.1.68-rc1` (pre-release) decision from this list. Each is documented
inline in the source file it originates from.

1. **Store interface — 4 methods not in spec §3.1/§8.4 closed enumeration** (T6/T7):
   - `ReleaseClaim(ctx, db, id, tok)` — A3 (gate-before-claim leaves PROCESSING rows that must
     return to RETRY_SCHEDULED without waiting for lease expiry).
   - `AdvanceDue(ctx, db, id)` — pre-claim `ErrRetryLater` path advances `next_attempt_at`.
   - `MoveToDeadLetter(ctx, db, id, reason)` — A2 (unscheduled RETRY_SCHEDULED rows moved out;
     accepts `RETRY_SCHEDULED` + payload/error_class guard so the three branches never spin empty).
   - `MoveToDeadLetterWithToken(ctx, db, id, tok, reason)` — A5 fencing symmetry with
     `MarkSucceeded`/`MarkFailed` for already-claimed PROCESSING rows.
   - **Spec revision needed**: §3.1 enumeration + fencing rationale for the two token-bearing
     methods. Until revised, the tag is arguably `v1.1.68-rc1` (interface shape not yet in spec).
2. **`MarkFailed` 9th param `maxAttempts int`** (T6/T7) — spec §3.1/§8.4 is 8-param; added so
   the OutcomeFor matrix can decide RETRY_SCHEDULED vs DEAD_LETTER without re-reading the row.
3. **`consumption_anomalies` schema** (T7/T8 migrations, both dialects):
   - `claim_id VARCHAR(36) NOT NULL DEFAULT ''` column added (spec §2.3 has no `claim_id`).
   - `uk_anomaly_once UNIQUE (kind, event_id, handler_id, claim_id)` — idempotent anomaly insert
     (D14 batch + D4 inline reclaim both rely on it).
   - `tenant_id INT NOT NULL DEFAULT 0` (B8 — D18#8 RecordAnomaly passes tenantID; spec §2.3
     has no tenant_id on anomalies).
4. **`idx_aggregate` tail column `first_seen_at`** (T7/T8, D22) — appended so the
   `FindEligibleHeads NOT EXISTS` subquery has a deterministic tie-breaker when events carry no
   `causal_seq`. Spec §2.1 index definition lacks it.
5. **`STUCK_PROCESSING` anomaly kind — REVERTED to PR-7 (review R3)**: PR-2 originally shipped a
   third anomaly kind for lease-orphan rows past a 2h `stuckProcessingThreshold` (P1 alert,
   separable from transient P2 `LEASE_ORPHAN`), extending spec §2.3's closed enum. The spec is not
   in-repo, so §2.3/§10 could not be revised alongside PR-2; rather than ship a spec-undefined kind
   (ops would see a Grafana signal with no spec meaning), the escalation was removed from
   `ObserveExpiredLeases` (now LEASE_ORPHAN only) and the whole stuck-row closure moves to PR-7 as
   one unit: `STUCK_PROCESSING` kind + `RecoverStuckProcessing(id, reconstructedPayload)` ops API +
   spec §2.3/§10 revision. See the R3 comment in `gormshared/store.go`.

**Other deviations (lower-stakes, documentation-only)**:
- `idx_due` is partial on PostgreSQL (`WHERE status = 'RETRY_SCHEDULED'`) and non-partial on
  MySQL (MySQL has no partial indexes — intentional, documented in migration comments).
- `RecordAnomaly` signature carries `tenantID int` (D18#8) — spec §3.1 has no tenantID param.
- `consumption_anomaly` table name is singular in the model tag; both migrations create it as
  `consumption_anomalies` (plural) — the GORM `TableName` override reconciles this.

## Review Minor findings (not fixed; non-blocking)

- `gates_test.go` excludes itself from `scanReliable` (and the bash script uses
  `--exclude=gates_test.go`) — the gate file legitimately contains the pattern vocabulary
  (placeholder regex literal + SelfCheck injection samples); scanning it would self-trigger.
  This is standard self-contained-gate practice and is documented in the `scanReliable` doc comment.
- `scheduler_test.go` fakeStore methods are no-op stubs for Store methods the Scheduler doesn't
  exercise (e.g. `List`, `GetByID`); kept to satisfy the compile-time `var _ store.Store =
  (*schedulerFakeStore)(nil)` interface completeness assertion (D21).

## Seams PR-2 laid for PR-3

- `Store` / `QuarantineStore` interfaces are the consumer-side contract; PR-3's
  `EventBusDLQAdapter` and `OutboxRecordingHandler` depend only on these interfaces, not on
  gormshared internals.
- `ErrorClassifier` is the dialect plug-point; services compose `gormshared.NewStore` with their
  local driver's classifier (mysql/postgres classifiers shipped; a service using a different
  driver adds only one file).
- `ReplaySafety` is the single switch the scheduler honors — adding a new safety class is a
  kernel-only change (state.go + classify.go), not a scheduler change.
- `ClaimToken` opaque-string contract (`holder:uuid`) is the fencing-token wire format; PR-3's
  adapter does not need to know its internal structure.

## §8.5 upper-layer resolution (PR-2 completion, delivered v1.7.4)

Spec §8.5 lists four upper-layer packages under sdk/pkg/reliable/. Resolution:

- **adapters/eventbus/ (DLQSender bridge) — DELIVERED v1.7.4.** Core `EventBusDLQAdapter`
  at `sdk/pkg/reliable/adapters/eventbus/` (pkg `eventbusdlq`), sourced from
  file-storage-service's hardened copy (1 MiB payload cap + P1 retryable-refusal fix —
  refuses to terminalize a RETRYABLE/ErrRetryLater cause, fail-closed). `TenantStoreResolver`
  lives in `sdk/pkg/reliable/store` (NOT adapters/eventbus) so opsvc can reuse it without
  importing sarama. A minimal `LogSink` is injected (nil → no-op; no global logger in core).
  evidence-management + file-storage-service adopted the core symbol and deleted their local
  copies (M10). evidence-management GAINED the P1 fix + payload cap (its local copy lacked
  both); file-storage was pure dedup.

- **opsvc/ (ops service layer + DTOs) — DELIVERED v1.7.4.** `sdk/pkg/reliable/opsvc/`
  backs spec §10's consumption/quarantine/anomalies API (List/GetDetail/ReplayOne/
  BatchReplay/Discard/Stats/QuarantineList/QuarantineDetail/QuarantineResolve/Anomalies).
  Reuses `store.TenantStoreResolver` (one-DB-per-tenant). Decisions applied:
  - §6.2.1 manual-replay 409 (Q1=A): `ReplayOne` returns ConflictError when
    `store.HasEarlierUnsolvedSibling` is true (mirrors EligibleHeadsSQL's NOT-EXISTS);
    check + ScheduleReplay run in one transaction. `BatchReplay` per-row.
  - Access audit (Q4=A): `NewService` takes a REQUIRED non-nil `AccessAuditor`;
    includePayload/includeRaw=true invokes it before returning gated data, fail-closed
    (audit error → withhold data). Caller identity is filled service-side (PR-7) — core
    never reads identity from context (M14).
  - Stats uses `store.Count` (F6), not list-then-count.

- **batch/ (M11 HandleBatch decorator) — DEFERRED (YAGNI).** No producer emits per-item
  sub-event envelopes (media batch = one envelope / atomic-in-tx per the PR-4 completion
  plan). Trigger to build: a producer that emits (source_event_id, item_key) per-item
  sub-event envelopes.

- **adapters/outbox/ (OutboxRecordingHandler) — NOT BUILT (§8.3 J3).** Spec §8.3 J3 makes
  the outbox DLQHandler IMPLEMENTATION service-side ("死信落点是服务基础设施决策"); §8.3 is
  the authoritative boundary over §8.5's listing. PR-6 delivered per-service handlers.
  Building would also violate M2 (copying outbox DL into event_consumption = PII
  double-landing).

- **Store additions:** ListAnomalies + Count + HasEarlierUnsolvedSibling (+ AnomalyFilter/
  AnomalyRow/CountFilter) added to `store.Store`; GormStore impl in gormshared (anomaly.go
  + replay.go). HasEarlierUnsolvedSibling mirrors EligibleHeadsSQL's NOT-EXISTS for the
  §6.2.1 manual-replay gate.

- **Deferred out of v1.7.4 (tracked for PR-7):** `POST /api/v1/quarantine/:id/replay`
  (needs service-side HandlerRegistry, §8.3 J3, to re-invoke the handler after mapping
  fixes); `/outbox/dead-lettered` (belongs to the outbox package's own ops).

- **Split to a separate PR (Q5=A):** the kernel `gormshared.sanitizeMsg` delegation +
  ReDoS fix (Unix-path regex linearization) were intentionally NOT done here — v1.7.4 only
  ADDS root `SanitizeForLog`/`SanitizeForStorage` for the adapter's use. The kernel
  delegation + fingerprint-stability regression will land in a focused follow-up PR.

Services NOT bumped for Phase A/B: security-management / process-management / tenant-service
do not import sdk/pkg/reliable at all → they need no bump until they adopt opsvc (PR-7+).
