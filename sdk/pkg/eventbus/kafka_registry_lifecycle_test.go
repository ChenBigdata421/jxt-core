package eventbus

import (
	"sync"
	"testing"
)

// kafka_registry_lifecycle_test.go mirrors nats_registry_lifecycle_test.go for the
// Kafka driver (PR2-core, Task 4, spec §3.9 dual coverage). Kafka is the PRODUCTION
// ACK path (plan decision D2), so the admission / unregister / Close contract must be
// byte-parallel with the NATS driver (decision D8).
//
// Reframe (same as NATS Tasks 2-3): on this broker-free helper the send-on-closed
// panic is structurally eliminated by lock-across-send, so the stress test is a guard,
// not a repro. Close() returns nil (no teardown errors on a struct with nil broker
// handles), so the §3.3 terminal-error divergence is not reproducible here either —
// the closed-gate tests below are the real RED→GREEN signal (they fail the pre-fix
// code which had no closed-check in RegisterTenant/UnregisterTenant/sendResultToChannel
// and which deferred closed.Store(true) until AFTER teardown).

// PR2-core: a full tenant channel must NOT fall back to global or silently drop.
func TestKafkaSendResult_NoFallbackNoDropWhenFull(t *testing.T) {
	bus := newTestKafkaEventBus(t)                   // D4 helper; in-process, no broker
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

// PR2-core: an unregistered tenant must NOT fall back to global.
func TestKafkaSendResult_UnregisteredNoFallback(t *testing.T) {
	bus := newTestKafkaEventBus(t)
	res := bus.sendResultToChannel(pubResult(999, "e1")) // tenant 999 never registered
	if res != AdmissionRejectedUnregistered {
		t.Fatalf("expected AdmissionRejectedUnregistered, got %v", res)
	}
	if len(bus.publishResultChan) != 0 {
		t.Fatalf("unregistered tenant leaked to global: %d result(s)", len(bus.publishResultChan))
	}
}

// PR2-core: UnregisterTenant concurrent with in-flight sends must NOT panic.
// On this broker-free helper the panic is structurally eliminated by lock-across-send
// (stress guard, not a repro — see file header). A send-on-closed is a PANIC that
// crashes the test binary; the real signal is "process survived N iterations under
// -count" (run with -count=N; -race deferred to CI — no C compiler on this Windows env).
func TestKafkaUnregisterTenant_NoSendOnClosedPanic(t *testing.T) {
	bus := newTestKafkaEventBus(t)
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
	_ = bus.UnregisterTenant(1)
	close(stop)
	wg.Wait()
}

// PR2-core (Task 3, §3.3): concurrent/repeated Close must converge on the SAME
// terminal error. On this broker-free helper there are no teardown errors, so the
// terminal error is nil for every caller — the guard is that all callers return
// byte-identical (nil) and the process survives.
func TestKafkaClose_StableTerminalError(t *testing.T) {
	bus := newTestKafkaEventBus(t)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = bus.Close()
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(errs); i++ {
		if errs[i] != errs[0] {
			t.Fatalf("divergent terminal error: caller[0]=%v caller[%d]=%v", errs[0], i, errs[i])
		}
	}
}

// PR2-core: after Close, admission is Frozen (the real RED→GREEN signal — pre-fix
// sendResultToChannel had no closed-check and would attempt the send).
func TestKafkaSendResult_RejectedFrozenAfterClose(t *testing.T) {
	bus := newTestKafkaEventBus(t)
	_ = bus.RegisterTenant(1, 16)
	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if res := bus.sendResultToChannel(pubResult(1, "e1")); res != AdmissionRejectedFrozen {
		t.Fatalf("expected AdmissionRejectedFrozen after Close, got %v", res)
	}
	// global path must also be frozen
	if res := bus.sendResultToChannel(pubResult(0, "e2")); res != AdmissionRejectedFrozen {
		t.Fatalf("expected AdmissionRejectedFrozen for global after Close, got %v", res)
	}
}

// PR2-core: RegisterTenant must reject after Close (prevents the late-registration
// leak — a fresh channel nobody closes).
func TestKafkaRegisterTenant_RejectedAfterClose(t *testing.T) {
	bus := newTestKafkaEventBus(t)
	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := bus.RegisterTenant(1, 16); err == nil {
		t.Fatal("expected RegisterTenant to fail after Close, got nil")
	}
}

// PR2-core: UnregisterTenant must reject after Close (closed-gate takes precedence
// over the not-registered check).
func TestKafkaUnregisterTenant_RejectedAfterClose(t *testing.T) {
	bus := newTestKafkaEventBus(t)
	_ = bus.RegisterTenant(1, 16)
	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := bus.UnregisterTenant(1); err == nil {
		t.Fatal("expected UnregisterTenant to fail after Close, got nil")
	}
}
