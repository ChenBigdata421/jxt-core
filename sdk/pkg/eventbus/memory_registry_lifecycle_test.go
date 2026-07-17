package eventbus

import (
	"sync"
	"testing"
)

// memory_registry_lifecycle_test.go (PR2-core, Task 2, decision D5 — memory-backend only).
//
// Byte-parallel with nats_registry_lifecycle_test.go and nats_close_lifecycle_test.go,
// but exercises *eventBusManager (the runtime type ONLY for the memory backend). This is
// memory-backend correctness coverage — it does NOT count toward spec §3.9 (NATS/Kafka
// dual coverage). The material adaptation under test: eventBusManager.closed is a plain
// bool guarded by m.mu (NOT atomic.Bool); admission snapshots it under m.mu.RLock().
//
// Uses newTestMemoryEventBus (D4/D5 helper) — no broker, no dial. Close() on that helper
// runs only the tenant-channel teardown + terminalErr path (publisher/subscriber/health-
// checker are nil), which is exactly the registry contract under test.

// PR2-core (Task 2, D5): a full tenant channel must NOT fall back to global or drop.
func TestMemorySendResult_NoFallbackNoDropWhenFull(t *testing.T) {
	bus := newTestMemoryEventBus(t)
	if err := bus.RegisterTenant(1, 1); err != nil { // buffer=1 → fills immediately
		t.Fatalf("register: %v", err)
	}
	// fill the 1-slot channel (no listener draining)
	bus.sendResultToChannel(pubResult(1, "e1"))
	res := bus.sendResultToChannel(pubResult(1, "e2")) // channel full
	if res != AdmissionRejectedFull {
		t.Fatalf("expected AdmissionRejectedFull, got %v", res)
	}
	// global channel must NOT have received the fallback
	if len(bus.publishResultChan) != 0 {
		t.Fatalf("fallback-to-global leaked %d result(s)", len(bus.publishResultChan))
	}
}

// PR2-core (Task 2, D5): a send for an unregistered tenant must NOT fall back to global.
func TestMemorySendResult_UnregisteredNoFallback(t *testing.T) {
	bus := newTestMemoryEventBus(t)
	res := bus.sendResultToChannel(pubResult(999, "e1")) // tenant 999 never registered
	if res != AdmissionRejectedUnregistered {
		t.Fatalf("expected AdmissionRejectedUnregistered, got %v", res)
	}
	if len(bus.publishResultChan) != 0 {
		t.Fatalf("fallback-to-global leaked %d result(s)", len(bus.publishResultChan))
	}
}

// PR2-core (Task 2, D1): UnregisterTenant concurrent with in-flight sends must NOT
// panic (delete-before-close + RLock-held send eliminates send-on-closed structurally).
func TestMemoryUnregisterTenant_NoSendOnClosedPanic(t *testing.T) {
	bus := newTestMemoryEventBus(t)
	_ = bus.RegisterTenant(1, 16)
	var wg sync.WaitGroup
	stop := make(chan struct{})
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
	_ = bus.UnregisterTenant(1) // old code panics here under -race/-count
	close(stop)
	wg.Wait()
}

// PR2-core (Task 2, §3.3): concurrent/repeated Close converges on the SAME cached
// terminal error. On the broker-free helper Close() returns nil (no teardown errors),
// so this is a CONSISTENCY GUARD pinning the invariant for the future.
func TestMemoryClose_StableTerminalError(t *testing.T) {
	bus := newTestMemoryEventBus(t)
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

// PR2-core (Task 2): idempotent second Close returns the same value, no re-run/panic.
func TestMemoryClose_IdempotentSecondCall(t *testing.T) {
	bus := newTestMemoryEventBus(t)
	first := bus.Close()
	second := bus.Close()
	if (first == nil) != (second == nil) {
		t.Fatalf("idempotent Close diverged: first=%v second=%v", first, second)
	}
}

// PR2-core (Task 2): after Close(), RegisterTenant/UnregisterTenant reject and a send
// returns AdmissionRejectedFrozen. Exercises the plain-bool closed snapshot-under-RLock.
func TestMemoryRegistry_RejectedAfterClose(t *testing.T) {
	bus := newTestMemoryEventBus(t)
	_ = bus.RegisterTenant(1, 16) // pre-register so not-registered is not the reason
	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := bus.RegisterTenant(2, 16); err == nil {
		t.Fatal("expected RegisterTenant to fail after Close, got nil")
	}
	if err := bus.UnregisterTenant(1); err == nil {
		t.Fatal("expected UnregisterTenant to fail after Close, got nil")
	}
	if out := bus.sendResultToChannel(pubResult(1, "e")); out != AdmissionRejectedFrozen {
		t.Fatalf("expected AdmissionRejectedFrozen after Close, got %v", out)
	}
}

// Concurrent RegisterTenant vs Close must not leak an orphan tenant channel (one a
// RegisterTenant installs AFTER Close's clear loop, so nobody closes it). Pre-fix the
// outer closed snapshot raced the teardown; post-fix RegisterTenant HOLDS m.mu.RLock
// across the tenantChannelsMu install, pinning closed (Close needs m.mu.Lock to flip
// it) so it cannot run between the check and the install. Stress: hammer many concurrent
// registers + one Close, then assert every channel still in the map is CLOSED.
// Run under `-race -count=N`.
func TestMemoryRegisterTenant_ConcurrentClose_NoOrphan(t *testing.T) {
	bus := newTestMemoryEventBus(t)
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				_ = bus.RegisterTenant(gid*100000+i, 16)
			}
		}(g)
	}
	wg.Add(1)
	go func() { defer wg.Done(); _ = bus.Close() }()
	wg.Wait()

	for id, ch := range bus.tenantPublishResultChans {
		select {
		case <-ch:
		default:
			t.Errorf("tenant %d: ACK channel still open after Close (TOCTOU orphan leak)", id)
		}
	}
}
