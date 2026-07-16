package eventbus

import (
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
