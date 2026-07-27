# Outbox system tests (real MySQL)

Tier-1 system tests for the outbox **DLQ lifecycle (C1)** and **publisher idempotency**, run
against a real MySQL 8 database. They exercise paths the unit suite structurally cannot: the real
`Start`/`dlqLoop` ticker driving the full DLQ state machine, and the MySQL unique index as the
actual idempotency serialization point (the unit `MockRepository` has no index, so
`TestIdempotency_ConcurrentPublish` explicitly refuses to assert single-publish).

These tests are build-tagged `system` and `t.Skip` when MySQL is unreachable, so they never run in
the default `go test ./...` gate.

## Run

```bash
# 1. Start MySQL (the same compose file the RedPanda/NATS broker suites use):
docker-compose -f docker-compose-nats.yml up -d mysql

# 2. Run the system tests:
go test -tags=system ./tests/outbox/system_tests/ -v -count=1
```

MySQL DSN: `root:test@tcp(127.0.0.1:13306)/outbox_system_test` (DB auto-created by
`MYSQL_DATABASE` in the compose file).

## What they cover

| Test | Verifies |
|---|---|
| `DLQ_FullLifecycle` (S1) | real `dlqLoop` drives `max_retry -> dead_lettered -> notified`; Handle-once (OV#6 single-instance guarantee) |
| `DLQ_CrashBetweenStepsRecovers` (S2) | a scheduler that died after step1 (terminalize) but before step2 (notify) is recovered by a fresh scheduler — orphaned `dead_lettered` row gets re-notified |
| `DLQ_NotifyMarkFailureCausesDoubleHandle` (S3) | a failed `MarkDeadLetterNotified` is non-terminal: the row is re-Handled on the next tick and eventually notified |
| `DLQ_StopDoesNotInterruptInflightHandle` (S4) | **R1 characterization** — `Stop()` does not cooperatively cancel an in-flight `Handle`; it waits `ShutdownTimeout`. Flips when R1 is fixed (derive cancellable ctx in `Start`, cancel in `Stop`) |
| `Idempotency_UniqueIndexRejectsDuplicateSave` (S1) | MySQL unique index rejects a duplicate same-key `Save` (not silent) |
| `Idempotency_ConcurrentSameKeySaveSingleRow` (S2) | N concurrent same-key `Save` calls serialize to exactly one row — the race the unit suite couldn't write |
| `Idempotency_SaveBatchRejectsIntraBatchCollision` (S3) | `SaveBatch` with an intra-batch key collision is rejected; no partial persistence |

## Why these aren't unit tests

- **DLQ-S1/S2/S3** drive the *real* scheduler loop (`Start`/`dlqLoop`/`runDLQWithRecover`) — the
  unit `scheduler_dlq_test.go` only calls `processDLQ` directly on an in-memory `stubRepo`, and
  `runDLQWithRecover` had zero coverage.
- **DLQ + idempotency on SQLite ≠ MySQL.** SQLite serializes all writers globally, so row-level
  locking and concurrent-insert serialization (the actual guarantees here) are untested against the
  real dialect without these.
- **Consumer-side idempotency is out of scope for jxt-core** (it lives in the consuming services;
  see `TODOS.md` F1 and the planned PR-2 `OutboxRecordingHandler`). These tests cover the
  **publisher-side** guarantee only.

## Not yet covered (Tier 2/3, deferred)

- End-to-end outbox → broker → consumer redelivery + dedup (needs RedPanda; Tier 2).
- Multi-instance double-Handle across two schedulers (OV#6; characterize before PR-2's durable claim).
- PostgreSQL parity (would catch the `idx_outbox_dlq_notify` PG partial-index divergence).
