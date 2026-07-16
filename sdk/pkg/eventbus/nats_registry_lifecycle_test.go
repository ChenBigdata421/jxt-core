package eventbus

import (
	"sync"
	"testing"
)

// PR2-core: a full tenant channel must NOT fall back to global or silently drop.
func TestSendResult_NoFallbackNoDropWhenFull(t *testing.T) {
	bus := newTestNATSEventBus(t)                    // D4 helper (see Phase 1 prerequisite); in-process, no broker
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
