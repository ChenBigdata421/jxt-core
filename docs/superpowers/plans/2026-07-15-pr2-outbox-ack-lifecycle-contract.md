# jxt-core PR2 — Outbox ACK Lifecycle/Registry Contract (v1.1.62) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the verified v1.1.61 ACK lifecycle/registry defects (send-on-closed panic, silent drop, fallback-to-global, Sleep-join) and ship jxt-core **v1.1.62** with a stable, default-off, drained contract that file-storage PR5 can consume.

**Architecture:** Two layers, both broken. (1) The eventbus tenant registry (`nats.go`/`kafka.go`) admits broker ACK results into per-tenant channels via `sendResultToChannel` — currently it silently drops on full and falls back to an unmonitored global channel, and `UnregisterTenant` closes the channel while sends are in flight (panic). (2) The outbox `EventBusAdapter.Close()` joins its conversion goroutine with `time.Sleep`, not a real join. PR2-core (P1) fixes both with a lock-across-send admission + close-after-sender-join lifecycle (D1/D3) and explicit admission outcomes. PR2-hardening (P2) adds checked constructors, default-off, and EventID dedupe.

**Tech Stack:** Go (jxt-core `go 1.26.0`), NATS JetStream + Kafka (sarama), sync primitives (`atomic.Bool`, `sync.RWMutex`), Ginkgo/stdlib `testing` with `-race`.

## Global Constraints

- jxt-core is a shared library consumed by file-storage, evidence-management/command, and shared — **backward compatibility is mandatory**. Keep old panic constructors; do not break existing signatures. command must compile + pass on v1.1.62 with zero behavior regression.
- Verified defect locations (this plan fixes these):
  - `sdk/pkg/eventbus/nats.go`: `sendResultToChannel` (L3358, called from L3299/3324/3349) has `default:` discard (L3370) + fallback-to-global (L3385-3396); `UnregisterTenant` (L3433) does `close(ch)` under Lock with no join/freeze → send-on-closed panic vs `sendResultToChannel` (RLock release L3363 → send L3367).
  - `sdk/pkg/eventbus/kafka.go`: parallel structure (`sendResultToChannel` L3472, `RegisterTenant` L3514, `UnregisterTenant` L3547, `Close` L1889) — same defects, same fix.
  - `sdk/pkg/eventbus/eventbus.go`: memory impl isomorphic (L1396-1418 / L1454-1494) — same fix (test-only, but keep consistent).
  - `sdk/pkg/outbox/adapters/eventbus_adapter.go`: `Close()` (L197-217) uses `time.Sleep(100ms)` instead of join; `tenantResultConversionLoop` (L177-186) has `default:` discard.
  - `sdk/pkg/outbox/publisher.go`: `DefaultPublisherConfig()` (L118-130) sets `ACKBatchSize=defaultACKBatchSize`(=50); `NewOutboxPublisher` (L207) panics on invalid config (L218-220).
  - `sdk/pkg/outbox/ack_marker_batcher.go`: `Add` (L55) does not dedupe by EventID.
- NATS struct fields available: `closed atomic.Bool`, `mu sync.Mutex`, `tenantChannelsMu` (RWMutex), `tenantPublishResultChans map[int]chan *PublishResult`, `publishResultChan chan *PublishResult`, `logger`.
- **No new framework.** Reuse jxt-core's existing patterns; do not rewrite the batcher/`MarkBatchAsPublished`.
- v1.1.62 does NOT raise the Go floor (already `go 1.26.0`).
- **ACK-drop safety assumption (D3, ce-doc-review #9):** `Close`/`UnregisterTenant` may abandon buffered-but-unconsumed ACKs. The dropped ACK was for an event already published to the broker, so the outbox sweep re-publishes it → a **duplicate** broker publish for that aggregate (per-aggregate ordering via `SubscribeEnvelope` is preserved). This is only safe under **consumer-side idempotency** — the evidence-management PR3 deliverable. The no-drain decision is gated on PR3; until then, document the duplicate-publish consequence explicitly wherever D3 is cited.

---

## File Structure

**PR2-core (P1):**
- Create: `sdk/pkg/eventbus/admission.go` — shared `AdmissionOutcome` type + sentinels.
- Create: `sdk/pkg/eventbus/testing_helpers_test.go` (D4) — broker-free `newTestNATSEventBus`/`newTestKafkaEventBus`/`pubResult`.
- Modify: `sdk/pkg/eventbus/nats.go` — `sendResultToChannel` (no drop, no fallback, gated on `closed`), `UnregisterTenant` (freeze+drain+close), terminal-error cache.
- Modify: `sdk/pkg/eventbus/kafka.go` — same changes as nats.go.
- Modify: `sdk/pkg/eventbus/eventbus.go` — memory impl same changes.
- Modify: `sdk/pkg/outbox/adapters/eventbus_adapter.go` — `Close()` real join; `tenantResultConversionLoop` no-drop.
- Test: `sdk/pkg/eventbus/nats_registry_lifecycle_test.go` (NEW), `kafka_registry_lifecycle_test.go` (NEW). (Both use the D4 broker-free helpers.)

**PR2-hardening (P2):**
- Modify: `sdk/pkg/outbox/publisher.go` — `DefaultPublisherConfig` default-off; add `NewOutboxPublisherChecked`.
- Modify: `sdk/pkg/outbox/ack_marker_batcher.go` — EventID dedupe + counters.

---

# Phase 1 — PR2-core (P1): lifecycle, registry, terminal error

> **⚠️ REVIEW-GOVERNED:** The step bodies below were written pre-review. They are **superseded by the `## GSTACK REVIEW REPORT` at the end of this file** — Resolved decisions **D1-D9** and the **Remaining fixes** list. Key overrides: (D1) `sendResultToChannel` holds the RLock across the send and drops `recover`; `UnregisterTenant` is write-Lock delete+close with NO detach-first/drain. (D2) Kafka is the prod path and is first-class. (D3) `Close` closes tenant channels after the sender join with NO drain goroutine. (D4) the `newTestNATSEventBus`/`pubResult` helpers do NOT exist and MUST be created first. (D5) the `eventBusManager` port is memory-backend-only. **(2026-07-16 re-review)** (D6/D7) terminal error uses `sync.Once` + `closeDone` — NOT `atomic.Pointer` "publish-before-teardown" (Task 3 Step 3 already rewritten); spec §3.2 accepted-loss gate is steady-state-only with a shutdown carve-out. (D8) the registry fix is triplicated across nats/kafka/memory on purpose (shared-registry extraction deferred to a TODO). (D9) `EventBusAdapter.Close()` returns its OWN error and does NOT close the injected bus; the orchestrator calls `bus.Close()`. **Where a step body below still shows the old approach, the report wins.**
>
> **Prerequisite (D4): broker-free test helpers.** Before any Task 1-3 test compiles, create `sdk/pkg/eventbus/testing_helpers_test.go` with `newTestNATSEventBus(t)` / `newTestKafkaEventBus(t)` (construct the driver struct directly: `zap.NewNop()` logger, fresh `publishResultChan` + `tenantPublishResultChans` maps, nil conn/js/producer) and `pubResult(tenantID, eventID)`. Registry lifecycle tests touch only map/mutex/closed/logger, never the broker connection — so no real broker and no hang.

### Task 1: Admission outcome type + lossless, no-fallback `sendResultToChannel` (NATS)

**Files:**
- Create: `sdk/pkg/eventbus/admission.go`
- Modify: `sdk/pkg/eventbus/nats.go:3358-3397` (`sendResultToChannel`)
- Modify: `sdk/pkg/eventbus/nats.go:3299,3324,3349` (3 ack call sites)

**Interfaces:**
- Produces: `type AdmissionOutcome` + `sendResultToChannel` returns it; never drops, never falls back to global.

- [ ] **Step 1: Write the failing test**

`sdk/pkg/eventbus/nats_registry_lifecycle_test.go`:

```go
package eventbus

import (
	"sync"
	"testing"
)

// PR2-core: a full tenant channel must NOT fall back to global or silently drop.
func TestSendResult_NoFallbackNoDropWhenFull(t *testing.T) {
	bus := newTestNATSEventBus(t)            // D4 helper (see Phase 1 prerequisite); in-process, no broker
	if err := bus.RegisterTenant(1, 1); err != nil { // buffer=1 → fills immediately
		t.Fatalf("register: %v", err)
	}
	// fill the 1-slot channel (no listener draining)
	bus.sendResultToChannel(pubResult(1, "e1"))
	res := bus.sendResultToChannel(pubResult(1, "e2")) // channel full
	if res == AdmissionAccepted {
		t.Fatal("expected non-accepted outcome when tenant channel is full, got Accepted")
	}
	if res != AdmissionRejectedFull {
		t.Fatalf("expected AdmissionRejectedFull, got %v", res)
	}
	// global channel must NOT have received the fallback
	if len(bus.publishResultChan) != 0 {
		t.Fatalf("fallback-to-global leaked %d result(s)", len(bus.publishResultChan))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/eventbus/ -run TestSendResult_NoFallbackNoDropWhenFull -race`
Expected: FAIL — current code returns nothing (no outcome) and pushes to global.

- [ ] **Step 3: Create the admission outcome type**

`sdk/pkg/eventbus/admission.go`:

```go
package eventbus

// AdmissionOutcome is the result of admitting a broker ACK result into the registry.
// The registry is the ONLY layer allowed to non-blockingly reject; everywhere downstream
// of an Accepted result must drain losslessly.
type AdmissionOutcome int

const (
	AdmissionRejectedUnspecified AdmissionOutcome = iota // must not be emitted
	AdmissionAccepted                                     // reached a tenant or global listener channel
	AdmissionRejectedFull                                 // tenant channel full (listener too slow)
	AdmissionRejectedFrozen                               // registry is shutting down (closed)
	AdmissionRejectedUnregistered                         // tenant not registered AND no global listener
)
```

- [ ] **Step 4: Rewrite `sendResultToChannel` (NATS)**

Replace `nats.go:3358-3397` with a version that returns `AdmissionOutcome`, never uses `default:` to drop, and never falls back to global:

```go
// sendResultToChannel admits a broker ACK result (D1: lock-across-send). Holds
// tenantChannelsMu.RLock() ACROSS the non-blocking send so UnregisterTenant's write Lock
// cannot close the channel while a send is in flight — eliminates send-on-closed
// structurally, NO recover. Never falls back to the unmonitored global channel (spec §2.2).
func (n *natsEventBus) sendResultToChannel(result *PublishResult) AdmissionOutcome {
	if n.closed.Load() {
		return AdmissionRejectedFrozen
	}
	if result.TenantID != 0 {
		n.tenantChannelsMu.RLock()
		tenantChan, exists := n.tenantPublishResultChans[result.TenantID]
		if !exists {
			n.tenantChannelsMu.RUnlock()
			return AdmissionRejectedUnregistered // do NOT fall back to global
		}
		select {
		case tenantChan <- result:
			n.tenantChannelsMu.RUnlock()
			return AdmissionAccepted
		default:
			n.tenantChannelsMu.RUnlock()
			return AdmissionRejectedFull
		}
	}
	// TenantID == 0: legitimately global. Accepted only if it actually goes in.
	select {
	case n.publishResultChan <- result:
		return AdmissionAccepted
	default:
		return AdmissionRejectedFull
	}
}
```

- [ ] **Step 5: Update the 3 ack call sites to consume the outcome**

At `nats.go:3299,3324,3349`, change `n.sendResultToChannel(result)` to discard-free accounting. Minimal correct change (stop ignoring the result so a frozen shutdown halts further sends):

```go
if n.sendResultToChannel(result) == AdmissionRejectedFrozen {
    return // registry is closing; stop producing ack results
}
```

(If the surrounding function's signature/return doesn't allow `return`, wrap with a `break`/labeled break appropriate to that loop — each of the 3 sites is inside an ack-handling path.)

> **P3 correction (verified):** each of the 3 call sites at `nats.go:3299/3324/3349` is already the **last statement of its `case`** in `processACKTask`'s `select`. There is no loop to `break`, so do NOT add a `break`/labeled-`break` — the `return` is case-final and effectively documents intent only.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./sdk/pkg/eventbus/ -run TestSendResult_NoFallbackNoDropWhenFull -race`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add sdk/pkg/eventbus/admission.go sdk/pkg/eventbus/nats.go sdk/pkg/eventbus/nats_registry_lifecycle_test.go
git commit -m "fix(eventbus): lossless ACK admission, no fallback-to-global (PR2-core, nats)"
```

---

### Task 2: Write-Lock delete+close (lock-across-send, D1) — eliminate send-on-closed panic (NATS)

**Files:**
- Modify: `sdk/pkg/eventbus/nats.go:3433-3455` (`UnregisterTenant`)

**Interfaces:**
- Consumes: Task 1's `closed` gating.
- Produces: `UnregisterTenant` no longer closes a channel while a send is in flight.

- [ ] **Step 1: Write the failing race test**

Append to `nats_registry_lifecycle_test.go`:

```go
// PR2-core: UnregisterTenant concurrent with in-flight sends must NOT panic.
func TestUnregisterTenant_NoSendOnClosedPanic(t *testing.T) {
	bus := newTestNATSEventBus(t)
	_ = bus.RegisterTenant(1, 16)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	// sender: hammer sendResultToChannel
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = bus.sendResultToChannel(pubResult(1, "e"))
				}
			}
		}()
	}
	// unregister concurrently (the old code panics here under -race/-count)
	_ = bus.UnregisterTenant(1)
	close(stop)
	wg.Wait()
	// A send-on-closed is a PANIC that crashes this test binary — the `-race`
	// detector does NOT flag it. The real signal is "process survived N iterations
	// under -count" (see Remaining fixes: run with -count=N, fail-on-panic).
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/eventbus/ -run TestUnregisterTenant_NoSendOnClosedPanic -race -count=20`
Expected: FAIL or flaky panic (`send on closed channel`) — the race window is real.

- [ ] **Step 3: Rewrite `UnregisterTenant` to write-Lock delete+close (D1)**

Replace `nats.go:3433-3455`. The fix: remove the channel from the registry FIRST (under Lock), so new sends take the "unregistered → reject" path before we close; then drain + close the detached channel. Combined with Task 1's `closed.Load()` gate and the registry-check-then-send window, this removes the close-vs-send race:

```go
func (n *natsEventBus) UnregisterTenant(tenantID int) error {
	if tenantID == 0 {
		return fmt.Errorf("tenantID cannot be zero")
	}
	n.tenantChannelsMu.Lock()
	ch, exists := n.tenantPublishResultChans[tenantID]
	if !exists {
		n.tenantChannelsMu.Unlock()
		return fmt.Errorf("tenant %d not registered", tenantID)
	}
	delete(n.tenantPublishResultChans, tenantID) // detach
	close(ch) // D1: safe — every sender holds RLock across send (Task 1); write Lock waited them out. NO recover.
	n.tenantChannelsMu.Unlock()
	n.logger.Info("Tenant ACK channel unregistered", zap.Int("tenantID", tenantID))
	return nil
}
```

> Why this is race-free (D1): a sender does `RLock → lookup → send → RUnlock` with the **send inside the RLock** (Task 1). `UnregisterTenant`'s write Lock is mutually exclusive with every RLock, so by the time `close(ch)` runs, no sender can be inside its send critical section. A sender that hasn't taken the RLock yet misses the (already deleted) lookup and returns `AdmissionRejectedUnregistered`. No `recover` needed — the original detach-first+drain+recover reasoning is superseded.

- [ ] **Step 4: (D1 — DROPPED) No recover safeguard**

D1 chose lock-across-send (Task 1) over recover. Do **NOT** add a `defer recover()` to `sendResultToChannel`. The write-Lock in `UnregisterTenant` (Step 3) plus the RLock-held send (Task 1) make send-on-closed structurally impossible, so recover is unnecessary and would only mask unrelated panics (nil map, nil logger).

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./sdk/pkg/eventbus/ -run TestUnregisterTenant_NoSendOnClosedPanic -race -count=50`
Expected: PASS — no panic, no data race across 50 iterations.

- [ ] **Step 6: Commit**

```bash
git add sdk/pkg/eventbus/nats.go sdk/pkg/eventbus/nats_registry_lifecycle_test.go
git commit -m "fix(eventbus): freeze+drain+close, eliminate send-on-closed panic (PR2-core, nats)"
```

---

### Task 3: Stable terminal error (NATS Close/UnregisterTenant)

**Files:**
- Modify: `sdk/pkg/eventbus/nats.go` (`Close` ~L1336, `UnregisterTenant` from Task 2)

**Interfaces:**
- Produces: repeated/concurrent `Close`/`UnregisterTenant` return the same cached terminal error.

- [ ] **Step 1: Write the failing test**

```go
// PR2-core: concurrent/repeated Stop must return the same terminal error.
func TestClose_StableTerminalError(t *testing.T) {
	bus := newTestNATSEventBus(t)
	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = bus.Close()
		}(i)
	}
	wg.Wait()
	// all non-nil errors must be identical (same cached terminal error)
	first := errs[0]
	for _, e := range errs[1:] {
		if (e == nil) != (first == nil) {
			t.Fatalf("mixed nil/non-nil terminal errors: %+v", errs)
		}
		if e != nil && e.Error() != first.Error() {
			t.Fatalf("divergent terminal errors: %q vs %q", first, e)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/eventbus/ -run TestClose_StableTerminalError -race`
Expected: FAIL or flaky — current `Close` returns `nil` always and isn't idempotent-safe under contention.

- [ ] **Step 3: Cache the terminal error (`sync.Once` + `closeDone`)**

> **(2026-07-16 re-review — P1-#1 CORRECTION)** The earlier `atomic.Pointer[error]` "publish BEFORE teardown" sketch is **physically impossible**: the terminal error is `errors.Join(errs...)` accumulated *through* teardown (unsubscribe, `ackWorkerWg.Wait`, the ≤30s `PublishAsyncComplete`, `conn.Close`), so it is unknown until teardown finishes. A pointer published-after-teardown means a repeat-Close during the teardown window sees `closed==true && terminalErr==nil` → returns `nil`, while a post-teardown caller returns the real error — **violating spec §3.3's "byte-equal same terminal error"**. `TestClose_StableTerminalError` uses the nil-broker D4 helper (instant teardown), so it is BLIND to this window.
>
> **Decision (spec §3.3 wins over "return fast"):** use `sync.Once` + a `closeDone` channel. The first caller runs teardown; every concurrent/repeat caller **blocks on `<-closeDone`** and then reads the same `terminalErr`. Trade-off: a repeat-Close can block up to the 30s `PublishAsyncComplete` budget — accepted, because a stable terminal error is the §3.3 contract and shutdown is not latency-sensitive.

Add fields to `natsEventBus` (near `closed`) and init `closeDone` in the constructor (`make(chan struct{})`, alongside `ackWorkerStop` at nats.go:208):

```go
closeOnce   sync.Once     // guarantees teardown runs exactly once
closeDone   chan struct{} // closed when teardown completes; repeat-Close waits on it
terminalErr error         // written once inside closeOnce.Do, read only after <-closeDone
```

Rewrite `Close()` so admission freezes immediately, teardown runs once, and all callers converge on the same error:

```go
func (n *natsEventBus) Close() error {
	n.closeOnce.Do(func() {
		// Freeze admission FIRST so in-flight senders take the Frozen/unregistered
		// path (Task 1) and RegisterTenant's closed-gate (#3) rejects new tenants.
		n.closed.Store(true)
		defer close(n.closeDone) // unblock every waiting repeat-Close AFTER terminalErr is set
		n.mu.Lock()
		defer n.mu.Unlock()
		var errs []error
		// ... existing teardown: unsubscribe, backlogDetector.Stop, actorPool.Stop,
		//     topicHandlers clear, subscribedTopics reset ...
		// ACK worker join — AFTER this, no processACKTask sender is alive:
		close(n.ackWorkerStop)
		n.ackWorkerWg.Wait()
		// ... js.PublishAsyncComplete (≤30s), conn.Close() ...

		// D3: close tenant channels AFTER the sender join, synchronously, NO drain goroutine.
		// A still-ranging consumer drains the buffer via range; otherwise the buffer is abandoned
		// (at-least-once outbox sweep self-heals — see Global Constraints D3 + spec §3.2 shutdown carve-out).
		// Lock order: mu held, then tenantChannelsMu.
		n.tenantChannelsMu.Lock()
		for id, ch := range n.tenantPublishResultChans {
			delete(n.tenantPublishResultChans, id)
			close(ch)
		}
		n.tenantChannelsMu.Unlock()

		n.terminalErr = errors.Join(errs...) // set BEFORE closeDone fires (defer order)
	})
	<-n.closeDone       // concurrent/repeat callers block here until teardown completed
	return n.terminalErr // identical value for every caller — satisfies §3.3 byte-equality
}
```

> **Defer order matters:** `defer close(n.closeDone)` is registered before `defer n.mu.Unlock()`, so it runs LAST — after `terminalErr` is assigned and the lock released. Every `<-n.closeDone` waiter therefore observes the fully-written `terminalErr` without holding `n.mu`.

> **(ce-doc-review #3) RegisterTenant closed-gate — apply to BOTH drivers.** Add `if n.closed.Load() { return fmt.Errorf("eventbus closed") }` at the top of `RegisterTenant` (nats.go:3400 / kafka.go:3514) and `UnregisterTenant`. Because `Close` flips `closed` BEFORE the tenant-channel close loop (above), a `RegisterTenant` racing teardown either fails fast or is serialized — it cannot install a fresh channel that nobody closes (the late-registration leak).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./sdk/pkg/eventbus/ -run TestClose_StableTerminalError -race -count=20`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/pkg/eventbus/nats.go sdk/pkg/eventbus/nats_registry_lifecycle_test.go
git commit -m "fix(eventbus): stable terminal error on concurrent Close (PR2-core, nats)"
```

---

### Task 4: Replicate Tasks 1-3 on Kafka + Memory; fix adapter `Close()` join

**Files:**
- Modify: `sdk/pkg/eventbus/kafka.go` (same 3 changes as nats)
- Modify: `sdk/pkg/eventbus/eventbus.go` (memory impl — same)
- Modify: `sdk/pkg/outbox/adapters/eventbus_adapter.go:177-217`

**Interfaces:**
- Produces: identical contract on Kafka/Memory; adapter `Close()` joins deterministically.

- [ ] **Step 1: Port the NATS changes to `kafka.go` (PRODUCTION path — D2, first-class)**

Apply Task 1 (`sendResultToChannel` → lock-across-send, no drop/fallback), Task 2 (`UnregisterTenant` write-Lock delete+close, NO recover), Task 3 (terminal error via `sync.Once` + `closeDone` chan — **NOT** `atomic.Pointer[error]`; see the P1-#1 correction in Task 3) to `kafkaEventBus` (functions at L3472/L3547/L1889, struct at L52). Use the same `AdmissionOutcome` type from `admission.go`. Kafka guards `tenantID <= 0` (NATS guards `== 0`) — keep kafka's existing guard.

**[P0] Kafka has NO `ackWorkerWg` — add a sender WaitGroup.** Kafka's ACK senders are two free goroutines `handleAsyncProducerSuccess`/`handleAsyncProducerErrors` (spawned at init `kafka.go:442-443` AND at reconnect `kafka.go:2096-2097`) with no join. Add a `sync.WaitGroup` (e.g. `producerResultWg`) that these goroutines `Done` on exit (they exit only after `producer.Close()` closes `Successes()`/`Errors()`). Wrap `Close()` (`kafka.go:1889`) in the SAME `sync.Once`+`closeDone` shape as NATS Task 3, and inside `closeOnce.Do`: freeze `closed.Store(true)` FIRST (fixes the ordering bug — kafka currently stops consumer/actorpool before any closed-check → double-teardown on a racing second Close), then sequence `producer.Close()` → `producerResultWg.Wait()` → close tenant channels (D3: synchronously, NO drain goroutine) → set `terminalErr` → `defer close(closeDone)`.

**[P0] Reconnect path re-spawns the sender goroutines (ce-doc-review #1 — two personas).** `reinitializeKafkaConnection` (`kafka.go:2034-2101`) ALSO spawns `handleAsyncProducerSuccess/Errors` at `2096-2097`, and it runs on the **healthcheck reconnect path** (`kafka.go:2518`) concurrently with `Close`. So: `producerResultWg.Add(2)` at **both** spawn sites (init AND inside `reinitializeKafkaConnection`); gate the reconnect spawn on `if k.closed.Load() { return }` under the same lock `Close` uses for teardown; verify the old `producer.Close()` (`kafka.go:2040`) fires the old goroutines' `Done` before the new `Add`. Without this, `Close`'s `Wait()` either never returns (Add only at init) or panics (`sync: WaitGroup is reused before previous Wait has returned`). Reconnect is now a joined lifecycle, not just `Close`.

**[T4] Kafka lifecycle test (spec §3.9 dual coverage):** write `sdk/pkg/eventbus/kafka_registry_lifecycle_test.go` mirroring the NATS tests (`TestSendResult_*`, `TestUnregisterTenant_*`, `TestClose_*`) against `newTestKafkaEventBus(t)` (add it to `testing_helpers_test.go`, same shape as NATS).

- [ ] **Step 2: Port to the memory impl in `eventbus.go` — MEMORY-BACKEND ONLY (D5)**

Same changes to `eventBusManager` (`sendResultToChannel` L1454, `UnregisterTenant` L1396, `Close`).

> **D5 scope:** `eventBusManager` is the runtime type ONLY for the `memory` backend (`NewEventBus` returns the raw `*natsEventBus`/`*kafkaEventBus` for nats/kafka — verified eventbus.go:87-96). This fix is for memory-backend correctness, NOT NATS/Kafka prod coverage; do NOT count it toward spec §3.9. Note `eventBusManager.closed` is a plain `bool` (not `atomic.Bool`) — guard it under the manager's `mu` consistently.

- [ ] **Step 3: Write the adapter `Close()` join test**

`sdk/pkg/outbox/adapters/eventbus_adapter_close_test.go`:

```go
package adapters

import "testing"

// PR2-core: Close must join the conversion goroutine, not sleep.
func TestAdapterClose_JoinsDeterministically(t *testing.T) {
	a := NewEventBusAdapter(stubEventBus()) // stubEventBus() defined inline in this test file (D4 pattern); satisfies eventbus.EventBus with no-op methods, nil broker
	// push a result so the conversion loop is active
	// ...
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// assert the loop goroutine has exited (e.g., via an exposed done channel or by
	// observing the output channel is closed promptly without relying on a 100ms sleep)
}
```

- [ ] **Step 4: Replace `time.Sleep(100ms)` with a real join**

In `eventbus_adapter.go:197-217`, replace the sleep with a real join over ALL conversion loops (ce-doc-review #2: `GetTenantPublishResultChannel` spawns one `tenantResultConversionLoop` per call on the prod path — a single done-channel only joins the global loop). Add a `loopsWg sync.WaitGroup` to `EventBusAdapter`; `Add(1)` in `start()` and in `GetTenantPublishResultChannel` (L149); `defer a.loopsWg.Done()` at the top of each conversion loop; `a.loopsWg.Wait()` in `Close()`:

```go
// D9: adapter returns its OWN teardown error and does NOT close the injected bus
// ("don't close what you don't own" — the bus owner/orchestrator calls bus.Close()
// and surfaces the terminal error for the non-zero exit, spec §3.3).
func (a *EventBusAdapter) Close() error {
	a.mu.Lock()
	if !a.started {
		a.mu.Unlock()
		return nil
	}
	a.started = false
	close(a.stopChan)
	a.mu.Unlock()
	a.loopsWg.Wait() // deterministic join of ALL conversion loops (global + per-tenant) — no time.Sleep
	close(a.outboxResultChan)
	// Return this adapter's own teardown error (nil today since join+close can't fail,
	// but the signature stays honest — never swallow a future join/close error to nil).
	// Do NOT call a.eventBus.Close(): the bus is injected, the orchestrator owns it.
	return nil
}
```

Also remove the `default:` discard in `tenantResultConversionLoop` (L183-186): on full, block on `<-a.stopChan` or block until the listener drains (no silent drop).

- [ ] **Step 5: Build + run all eventbus/outbox tests with -race**

Run: `go test ./sdk/pkg/eventbus/ ./sdk/pkg/outbox/... -race -count=10`
Expected: PASS on NATS + Kafka + Memory + adapter.

- [ ] **Step 6: Commit**

```bash
git add sdk/pkg/eventbus/kafka.go sdk/pkg/eventbus/eventbus.go sdk/pkg/outbox/adapters/eventbus_adapter.go sdk/pkg/outbox/adapters/eventbus_adapter_close_test.go
git commit -m "fix(eventbus,outbox): port lifecycle fix to kafka/memory + adapter join (PR2-core)"
```

---

# Phase 2 — PR2-hardening (P2): checked constructors, default-off, dedupe

> These do not block file-storage PR5 (PR5 sets ACK params explicitly). They can ship in the same v1.1.62 tag or a follow-up v1.1.63.

### Task 5: default-off for nil-config consumers

**Files:**
- Modify: `sdk/pkg/outbox/publisher.go:118-130` (`DefaultPublisherConfig`)

**Interfaces:**
- Produces: `DefaultPublisherConfig().ACKBatchSize == 0` (batching OFF for nil-config), while `defaultACKBatchSize`(=50) stays as the batcher's internal floor.

- [ ] **Step 1: Write the failing test**

```go
package outbox

import "testing"

func TestDefaultPublisherConfig_BatchingOffByDefault(t *testing.T) {
	cfg := DefaultPublisherConfig()
	if cfg.ACKBatchSize != 0 {
		t.Fatalf("nil-config default must be ACKBatchSize=0 (off), got %d", cfg.ACKBatchSize)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/outbox/ -run TestDefaultPublisherConfig_BatchingOffByDefault`
Expected: FAIL — current default is 50.

- [ ] **Step 3: Set the field, not the const**

In `publisher.go:118-130`, change `ACKBatchSize: defaultACKBatchSize,` to `ACKBatchSize: 0,` (leave the `defaultACKBatchSize` const at 50 — it is the batcher's internal floor for `ACKBatchSize<1` sendinels, used by `newAckMarkerBatcher`). Add a comment:

```go
// nil-config default: batching OFF (fail-safe). Callers that want batching must
// set ACKBatchSize explicitly. defaultACKBatchSize (50) remains the batcher's
// internal floor for ACKBatchSize<1, NOT the library default.
ACKBatchSize:             0,
```

- [ ] **Step 4: Run test to verify it passes + confirm command unaffected**

Run: `go test ./sdk/pkg/outbox/ -run TestDefaultPublisherConfig_BatchingOffByDefault`
Expected: PASS.

Run (in evidence-management/command): `cd ../../evidence-management/command && go build ./... && go test ./...`
Expected: command compiles + passes (it sets ACKBatchSize=500 explicitly, so default-off is a no-op for it).

- [ ] **Step 5: Commit**

```bash
git add sdk/pkg/outbox/publisher.go
git commit -m "feat(outbox): default ACKBatchSize=0 for nil-config (PR2-hardening, default-off)"
```

---

### Task 6: Checked constructor (returns error, no panic)

**Files:**
- Modify: `sdk/pkg/outbox/publisher.go` (add `NewOutboxPublisherChecked`; keep `NewOutboxPublisher`)

**Interfaces:**
- Produces: `func NewOutboxPublisherChecked(repo, pub, mapper, config) (*OutboxPublisher, error)`. Old `NewOutboxPublisher` stays (calls the checked one, panics on error — backward compatible).

> **Reverse-order rollback (spec §2.3) is out of jxt-core scope.** "On multi-construct failure, roll back already-constructed components in reverse order" is consumer-side startup orchestration (the service's DI wiring destructs on partial failure), not a per-constructor jxt-core responsibility. It belongs to the consumer plan (file-storage PR5 / evidence) that wires multiple jxt-core components. This task only delivers the checked constructor that makes such rollback possible.

- [ ] **Step 1: Write the failing test**

```go
func TestNewOutboxPublisherChecked_RejectsInvalidConfig(t *testing.T) {
	if _, err := NewOutboxPublisherChecked(nilRepo(), nilPub(), nilMapper(), &PublisherConfig{ACKBatchSize: -1}); err == nil {
		t.Fatal("expected error for ACKBatchSize=-1, got nil")
	}
}
func TestNewOutboxPublisherChecked_AcceptsValidConfig(t *testing.T) {
	p, err := NewOutboxPublisherChecked(nilRepo(), nilPub(), nilMapper(), &PublisherConfig{ACKBatchSize: 100})
	if err != nil || p == nil {
		t.Fatalf("expected ok publisher, got %v %v", p, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/outbox/ -run TestNewOutboxPublisherChecked`
Expected: FAIL — undefined.

- [ ] **Step 3: Add the checked constructor; make the panic ctor delegate**

In `publisher.go` (near `NewOutboxPublisher` L207):

```go
// NewOutboxPublisherChecked builds a publisher and returns a validation error
// instead of panicking. New code should prefer this.
func NewOutboxPublisherChecked(repo OutboxRepository, eventPublisher EventPublisher, topicMapper TopicMapper, config *PublisherConfig) (*OutboxPublisher, error) {
	if config == nil {
		config = DefaultPublisherConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid publisher config: %w", err)
	}
	publisher := &OutboxPublisher{
		repo: repo, eventPublisher: eventPublisher, topicMapper: topicMapper, config: config,
	}
	if config.EnableMetrics {
		publisher.metrics = &PublisherMetrics{}
	}
	if config.MetricsCollector != nil {
		publisher.metricsCollector = config.MetricsCollector
	} else {
		publisher.metricsCollector = &NoOpMetricsCollector{}
	}
	return publisher, nil
}
```

Refactor `NewOutboxPublisher` to delegate + keep the panic behavior:

> ⚠️ **P2:** preserve the FULL existing body (publisher.go:207-241). The sketch below is the delegation shape only — first read L207-241 and merge any init logic there; do NOT overwrite the body with the minimal version (it could drop L221-241 setup).

```go
func NewOutboxPublisher(repo OutboxRepository, eventPublisher EventPublisher, topicMapper TopicMapper, config *PublisherConfig) *OutboxPublisher {
	p, err := NewOutboxPublisherChecked(repo, eventPublisher, topicMapper, config)
	if err != nil {
		panic(fmt.Sprintf("invalid publisher config: %v", err))
	}
	return p
}
```

- [ ] **Step 4: Run test to verify it passes + build command**

Run: `go test ./sdk/pkg/outbox/ -run TestNewOutboxPublisherChecked`
Expected: PASS. Run: `go build ./...` Expected: clean (old ctor unchanged behavior).

- [ ] **Step 5: Commit**

```bash
git add sdk/pkg/outbox/publisher.go
git commit -m "feat(outbox): NewOutboxPublisherChecked returns error (PR2-hardening)"
```

---

### Task 7: EventID dedupe + counters in the batcher

**Files:**
- Modify: `sdk/pkg/outbox/ack_marker_batcher.go` (`Add` L55, struct L15)

**Interfaces:**
- **(ce-doc-review #6) DEFERRED to PR4.** The ledger/counters (`Received`/`Collapsed`/`UniqueFlush`) exist only for PR4's conservation proof — spec §2.5 states `MarkBatchAsPublished` is DB-idempotent (`UPDATE ... WHERE id IN (...) AND status=pending`), so dedupe is NOT a runtime-correctness need. Drop `Stats()`/`ackBatchStats` and the dedupe from PR2; move the whole task to the jxt-benchmark PR4 plan. **Amend spec §2.5/§3.7** to remove dedupe/ledger from PR2-hardening. (If a v1.1.61 duplicate-ACK defect is later found, re-introduce dedupe here with that justification, not the PR4 ledger.)

- [ ] **Step 1: Write the failing test**

```go
func TestBatcher_DedupesDuplicateEventIDs(t *testing.T) {
	b := newAckMarkerBatcher(100, time.Millisecond, 5,
		func(ctx context.Context, ids []string) error { return nil },
		nil)
	b.Add("e1"); b.Add("e1"); b.Add("e2")
	// flush happens async; wait for it then assert unique count
	// (use a controlled flush via Close)
	_ = b.Close()
	// expose counters: b.stats() -> {Received:3, Collapsed:1, UniqueFlush:2}
	s := b.Stats()
	if s.Received != 3 || s.Collapsed != 1 || s.UniqueFlush != 2 {
		t.Fatalf("dedupe counters wrong: %+v", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/outbox/ -run TestBatcher_DedupesDuplicateEventIDs`
Expected: FAIL — no dedupe, no Stats.

- [ ] **Step 3: Add dedupe + counters**

In the batcher struct add a `seen map[string]struct{}` (reset each flush) and counter fields:

```go
type ackBatchStats struct{ Received, Collapsed, UniqueFlush int64 }
// add to struct:
seen    map[string]struct{}
stats   ackBatchStats
```

In `Add` (L55), dedupe:

```go
func (b *ackMarkerBatcher) Add(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	atomic.AddInt64(&b.stats.Received, 1)
	if b.seen == nil {
		b.seen = make(map[string]struct{})
	}
	if _, dup := b.seen[id]; dup {
		atomic.AddInt64(&b.stats.Collapsed, 1)
		return
	}
	b.seen[id] = struct{}{}
	b.ids = append(b.ids, id)
}
```

In `flush` (L109), reset `b.seen = nil` **co-located with `b.ids = nil` under `mu`** at the batch-extract point (verified ack_marker_batcher.go:115-116: `batch := b.ids; b.ids = nil`), and add `atomic.AddInt64(&b.stats.UniqueFlush, int64(len(batch)))`. Add:

```go
func (b *ackMarkerBatcher) Stats() ackBatchStats {
	return ackBatchStats{Received: atomic.LoadInt64(&b.stats.Received), Collapsed: atomic.LoadInt64(&b.stats.Collapsed), UniqueFlush: atomic.LoadInt64(&b.stats.UniqueFlush)}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./sdk/pkg/outbox/ -run TestBatcher_DedupesDuplicateEventIDs -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sdk/pkg/outbox/ack_marker_batcher.go
git commit -m "feat(outbox): EventID dedupe + admission counters (PR2-hardening)"
```

---

### Task 8: Release v1.1.62

**Files:** release metadata / git tag only.

- [ ] **Step 1: Full regression with -race**

Run: `go test ./... -race -count=10`
Expected: PASS (eventbus + outbox + existing suites). Confirm zero `send on closed channel` panics, zero data races.

- [ ] **Step 2: Cross-consumer regression**

Run: `cd ../evidence-management/command && go build ./... && go test ./...`
Expected: command compiles + passes on the new jxt-core (default-off + checked ctor are no-ops for command's explicit config).

- [ ] **Step 2b: F3 cross-service audit (ce-doc-review #7 residual)**

Before tagging: confirm `shared` and every other nil-config consumer does NOT depend on the library default of 50 (now 0). Record which services were checked, by whom, when. Spec §2.4 (F3) opens this obligation; default-off (Task 5, now in the gate) means any nil-config consumer opts OUT of batching — verify no service silently regresses to per-event `MarkAsPublished`.

- [ ] **Step 3: Tag the release (core + default-off gate met)**

```bash
git tag v1.1.62 -m "v1.1.62: Outbox ACK lifecycle/registry contract (PR2-core) + hardening"
# push tag per repo convention when ready
```

> Per spec §6 + ce-doc-review #4, the v1.1.62 tag is gated on PR2-core (Tasks 1-4) **AND Task 5 (default-off)**. Promoting Task 5 into the gate makes the Goal's "default-off" contract real for all consumers (not just file-storage PR5) and closes the spec §2.4 F3 audit obligation. Tasks 6-7 (checked ctor, dedupe) may ship in the same tag or a follow-up v1.1.63; they do not block file-storage PR5.

- [ ] **Step 4: Notify downstream**

file-storage PR5 (the file-storage plan) unblocks on this tag: bump `go.mod` to v1.1.62 (file-storage plan Task 8).

---

## Sequencing & dependencies

- **Tasks 1-4 (PR2-core, P1)** are the v1.1.62 gate. Create the D4 broker-free test helpers first, then do NATS (Tasks 1-3) as the iteration shape, then port to Kafka (Task 4 — **the production path per D2**, incl. the new sender WaitGroup) + Memory (memory-only, D5) + adapter. Kafka is first-class, not an afterthought. The Task 4 Kafka sender-WaitGroup sub-task is **non-descopeable** — it is the sole prod-critical, net-new concurrency work in this PR.
- **Tasks 5-7 (PR2-hardening, P2)** are independent of core and of each other; can parallelize. None blocks PR5.
- **Task 8** tags v1.1.62 after core passes `-race`; unblocks the file-storage plan's PR5.
- Each task ends with a commit. TDD throughout: failing `-race` test → fix → green → commit.

## Out of scope (other plans)

- file-storage PR1 + PR5 — file-storage plan.
- evidence-management PR3 (consumer hard-idempotency) — evidence-management plan.
- jxt-benchmark PR4 (conservation gate) — consumes Task 7's counters; separate plan.
- NATS/Kafka real-broker E2E (beyond in-process `-race` unit tests) — belongs with PR4's broker coverage.

---

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| Eng Review | `/plan-eng-review` | Architecture, concurrency, tests (required) | 1 | issues_open | 13 findings, 2 P0 (race-fix design; Kafka has no sender WG) |
| Outside Voice | Claude subagent (independent) | Cross-model 2nd opinion | 1 | issues_found | Confirmed core findings; added Kafka-no-WG P0 + drain-steals-from-consumer |
| Eng Re-review | `/plan-eng-review` (2026-07-16) | Re-verify plan vs current spec + live code | 1 | issues_found | All code facts re-verified accurate. 2 NEW P1 contradictions (terminal-error timing impossible; §3.2 vs D3); 3 P2 (DRY/D15, PR3 gate, adapter nil-return). "NO UNRESOLVED DECISIONS" was overstated. |

**CROSS-MODEL:** Review and outside voice independently agree the plan's Task 2 (detach-first + recover) does NOT eliminate the send-on-closed race and that lock-across-send is the correct fix; agree Task 3's fire-and-forget drain is wrong. Outside voice surfaced two issues the first pass missed: Kafka has no `ackWorkerWg` (its ACK senders are two unjoined goroutines at kafka.go:2096-2097), and the drain goroutine competes with the legitimate consumer for channel reads.

**VERDICT:** ENG REVIEW — issues_open. Plan targets the correct LIVE code (verified: driver structs `natsEventBus`/`kafkaEventBus` are the runtime ACK registry; `eventBusManager` in eventbus.go is DEAD for NATS/Kafka prod, live only for the memory test backend) and the PR2-core/hardening split is sound. But the central lifecycle fix (Task 2) and the Kafka port (Task 4) must be rewritten per D1-D5 before implementation. All issues are fixable in-plan; no rework of the overall approach. **(2026-07-16: Task 1-4 step bodies rewritten per D1-D5; spec §0.2 corrected NATS→Kafka. The Remaining fixes below are still to be implemented.)**

**RE-REVIEW VERDICT (2026-07-16):** issues_found, all fixable in-plan — the overall approach still stands and every code fact re-verified accurate against live source. Two NEW P1 contradictions resolved in this pass: **(P1-#1)** the `atomic.Pointer[error]` "publish-before-teardown" terminal-error model is physically impossible and blind-spotted by the nil-broker test → replaced with `sync.Once` + `closeDone` (Task 3 Step 3 rewritten; NATS + Kafka + Remaining-fix updated). **(P1-#2)** spec §3.2 acceptance ("accepted 丢失=0 / 账本守恒" including shutdown) directly contradicts D3 (Close abandons buffered accepted ACKs → duplicate republish) → spec §3.2/§2.1/§2.2/§3.3 amended with a steady-state scope + shutdown carve-out. Three P2 items added to Remaining fixes (DRY/D15 triplication, PR3 sequencing gate on Task 8, adapter nil-return vs §3.3). **D8 (keep three copies + TODO) and D9 (ownership-based adapter Close — orchestrator owns the bus) resolved with user sign-off 2026-07-16; shared-registry extraction captured in root `TODOS.md`.** No decisions remain open — implementation may proceed.

**Verified code facts grounding this review:**
- `sendResultToChannel` is void today (nats.go:3358-3397; kafka.go:3472-3511); pattern is `RLock → lookup → RUnlock → send` (race window), with `default:` discard + fallback-to-global.
- `UnregisterTenant` returns `error`; does `close(ch)+delete` under Lock with no drain/join (nats.go:3433-3455; kafka.go:3547-3569).
- `Close()` does NOT touch tenant channels today (nats.go:1336-1418; kafka.go:1889-1958). NATS has `ackWorkerWg` (joined at nats.go:1382-1387); Kafka has NO worker WG.
- `NewEventBus` returns the raw driver for nats/kafka (eventbus.go:87-96, initNATS/initKafka at 554-565); only memory keeps `eventBusManager`. So the manager's tenant methods are dead for prod.
- `ACKBatchSize == 0` correctly disables batching (publisher.go:737-766 gates batcher creation on `>0`; falls back to per-event `MarkAsPublished`). Default-off (Task 5) is safe.
- `newTestNATSEventBus`/`pubResult` helpers do NOT exist; existing NATS tests use a real broker (nats://localhost:4223).

**Resolved decisions (D1-D5):**
- **D1 — race fix:** Hold `tenantChannelsMu.RLock()` across the non-blocking send in `sendResultToChannel`; `UnregisterTenant` takes the write Lock then `delete`+`close`. Drop `recover`. Unregistered tenants return `AdmissionRejectedUnregistered` (no global fallback). Replaces Task 2's detach-first+recover. (Both reviewers agree.)
- **D2 — prod broker:** Production ACK path is RedPanda/Kafka, NOT NATS. **Spec §0.2 ("NATS is file-storage's runtime path") is WRONG and must be corrected to Kafka.** Kafka is the prod-critical deliverable; NATS-first iteration is acceptable since the fix shape is identical.
- **D3 — Close drain:** `Close()` closes tenant channels after the sender join with NO drain goroutine (Go-native close; a still-ranging consumer drains via range; ACK at-least-once self-heals any drop). Replaces Task 3's fire-and-forget drain.
- **D4 — test infra:** Add broker-free helpers that construct `*natsEventBus`/`*kafkaEventBus` directly (nil conn/js, `zap.NewNop()` logger). The plan's `newTestNATSEventBus`/`pubResult` do not exist; without this the Task 1-3 TDD loop does not compile and bare `go test ./sdk/pkg/eventbus/` hangs (known project gotcha).
- **D5 — manager scope:** Apply the same fix to `eventBusManager` (eventbus.go) but label it memory-backend-only; it is NOT NATS/Kafka prod coverage and must not be counted toward §3.9 dual-broker coverage.

**Remaining fixes to apply (determined, not open decisions):**
- **[P0] Kafka sender join:** Add `sync.WaitGroup` for `handleAsyncProducerSuccess`/`handleAsyncProducerErrors` (kafka.go:2096-2097); `Close()` does `producer.Close()` → `wg.Wait()` → close tenant channels. Mirror NATS `ackWorkerWg`.
- **[P0] terminal-error memory model (2026-07-16 re-review — CORRECTED, was wrong):** ~~Use `atomic.Pointer[error]`, published BEFORE teardown~~. The terminal error is unknown until teardown completes (`errors.Join` accumulates through the ≤30s `PublishAsyncComplete`), so "publish before teardown" is impossible and a pointer-published-after would let a teardown-window repeat-Close return `nil` while a later caller returns the error — violating §3.3 byte-equality. **Use `sync.Once` + a `closeDone` channel:** first caller runs teardown, sets `terminalErr`, then `defer close(closeDone)`; concurrent/repeat callers block on `<-closeDone` and read the same `terminalErr`. Trade-off (accepted): repeat-Close blocks ≤30s. Document lock order: `mu` then `tenantChannelsMu`. See Task 3 Step 3.
- **[P1] Close ordering (both drivers):** `closed.Load()` check must precede sub-component teardown; kafka currently stops consumer/actorpool before checking closed → double-teardown on a racing second Close.
- **[P1] adapter Close (Task 4):** Replace `time.Sleep(100ms)` with a real join (`loopsWg`); remove the `default:` discard in conversion loops (block on `stopChan` or room). Per **D9**, `adapter.Close()` returns its OWN teardown error (join + close errors), NOT nil, and does NOT call `eventBus.Close()` (bus is injected; orchestrator owns it). Spec §0.1/§3.3 updated.
- **[P1] stress tests:** Replace "0 panic under -race" (the race detector does NOT catch send-on-closed) with a no-recover stress loop (`go test -count=N`, fail-on-panic); add a test where a consumer ranges over a tenant channel concurrent with `UnregisterTenant`/`Close`; raise concurrency toward §3.1's N≥1000. **(2026-07-16 re-review)** Fix the misleading Task 2 Step 5 comment "reaching here under -race means no panic" → the signal is the panic crashing the test binary under `-count`, not a race report. **Add a teardown-window terminal-error test** that `TestClose_StableTerminalError` cannot cover: inject a teardown that blocks (fake slow `PublishAsyncComplete`), fire the first `Close()` in a goroutine, then fire N repeat-Closes DURING the teardown window and assert all N + the first return the SAME error (proves the `closeDone` block, not the old return-nil-then-error split). The nil-broker D4 helper has instant teardown and is blind to this.
- **[P2] DRY / prior-decision D15 conflict (2026-07-16 re-review):** the plan **triplicates** the identical admission/lifecycle fix across `nats.go`, `kafka.go`, and `eventbus.go` (memory), while a prior recorded decision (D15) called for a package-internal shared `tenantACKRegistry` (shared map/mutex/send/unregister primitives; protocol logging + fallback stay per-driver). Triplicating subtle concurrency code invites fix-drift (a later patch lands in nats but not kafka). **Decision needed:** either (a) confirm D15 is intentionally reversed for this PR (boring/incremental — keep three copies to minimize blast radius) and record it as a deferred TODO to extract post-v1.1.62, or (b) extract the shared registry now. Recommendation: (a) for v1.1.62 (three near-identical, well-tested copies beat a mid-flight refactor of the prod ACK path), with the extraction captured as a TODO. Flagged, not decided — needs user sign-off.
- **[P2] D3 duplicate-publish safety is gated on PR3, but Task 8 does not restate the gate:** D3's no-drain correctness rests entirely on consumer-side idempotency (evidence-management PR3). Task 8 tags v1.1.62 and "unblocks PR5" without noting that **enabling batching in PR5 must wait until PR3 is deployed** — otherwise shutdown-window duplicate publishes hit a non-idempotent consumer (duplicate Media). The tag itself is safe (default-off); add an explicit sequencing note to Task 8 / §6 so nobody flips `ACKBatchSize>0` before PR3 lands.
- **[P2] adapter `Close()` nil-return vs §3.3 process-exit contract:** ✅ RESOLVED as **D9** (ownership-based split — adapter returns its own error + does NOT close the injected bus; orchestrator calls `bus.Close()` directly and aggregates). See Resolved decisions D8-D9 below.
- **[P2] Task 6 refactor:** Preserve the full `NewOutboxPublisher` body (publisher.go:207-241); delegate only Validate+panic, do not drop init logic in L221-241.
- **[P2] Task 7 dedupe:** Co-locate the `seen` map reset with `b.ids=nil` under `mu` at the flush batch-extract point (ack_marker_batcher.go:115-116).
- **[P3] stale refs:** `DefaultPublisherConfig` is L118 (not 117); batcher `Close()` is L73-84 (not 73-94); no ticker struct field (ticker is local to `loop()` L88); Task 1 Step 5 `return`/`break` is unnecessary (the 3 call sites at nats.go:3299/3324/3349 are already case-final).
- **[P2] spec §0.2:** ✅ DONE — corrected NATS → Kafka as the production ACK path (2026-07-16).
- **[D6/D7] spec §2.1/§2.2/§3.2/§3.3:** ✅ DONE (2026-07-16) — amended to scope "accepted 丢失=0 / 账本守恒" to steady-state and exempt the shutdown window (D3 abandon + at-least-once republish + PR3 idempotency); §3.3 now states repeat-Close blocks on `closeDone` and returns a byte-equal terminal error.

**Resolved decisions (D8-D9, 2026-07-16 re-review — user sign-off):**
- **[D8] DRY vs prior D15 — RESOLVED: keep three copies for v1.1.62, defer extraction.** D15 (shared `tenantACKRegistry`) is intentionally deferred, not applied in this PR. Rationale: triplicating a small, well-tested admission/lifecycle fix across nats/kafka/memory keeps the prod ACK-path change minimal and low blast-radius; a mid-flight registry extraction on the critical path is higher risk than three near-identical copies with dual-broker test coverage (§3.9). **Mitigation against fix-drift:** the three implementations MUST stay byte-parallel (same function shape, same comments), and the extraction is captured as a post-v1.1.62 TODO in root `TODOS.md`. Any future patch to one driver's registry logic must be applied to all three until the shared type lands.
- **[D9] adapter terminal-error ownership — RESOLVED: ownership-based split (orchestrator owns the bus).** Best practice: "don't close what you don't own" + io.Closer errors propagate + single owner per resource (oklog/run, uber fx, controller-runtime Manager shapes). The bus is INJECTED into `NewEventBusAdapter`, so the adapter does NOT own or close it.
  - **jxt-core (this PR):** `EventBusAdapter.Close()` returns its OWN teardown error honestly (the `loopsWg` join + any close error), never swallows to nil; it does NOT call `eventBus.Close()`. Doc comment states the terminal error lives on the bus and is surfaced by the bus owner. Spec §0.1/§3.3 updated accordingly.
  - **consumer (file-storage PR5 / command):** the shutdown orchestrator calls `bus.Close()` DIRECTLY in reverse dependency order (stop scheduler/producer → `bus.Close()` → `adapter.Close()` → `publisher.StopACKListener()`), `errors.Join`s all Close/Stop errors, and exits non-zero on any non-nil. §3.3's "非零退出" is enforced HERE, not at the adapter boundary. (Rejected alternative: adapter calling `bus.Close()` — inverts ownership, creates a double-closer, and muddies who owns the non-zero exit.)

NO UNRESOLVED DECISIONS
