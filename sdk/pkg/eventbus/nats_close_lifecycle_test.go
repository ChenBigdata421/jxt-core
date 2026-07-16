package eventbus

import (
	"sync"
	"testing"
)

// PR2-core (Task 3): concurrent/repeated Close must converge on the SAME cached
// terminal error (spec §3.3 — byte-equal terminal error).
//
// On the broker-free D4 helper, Close() returns nil (no teardown errors), so all
// callers return nil consistently. This test is therefore a CONSISTENCY GUARD:
// it pins the invariant (all concurrent callers observe an identical value) for
// the future. The error-vs-nil divergence is only observable when teardown
// produces a non-nil error and is deferred to broker E2E (see task-3-report).
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

// PR2-core (Task 3): a second Close() must return the SAME value as the first
// (nil == nil here) and must NOT re-run teardown or panic.
func TestClose_IdempotentSecondCall(t *testing.T) {
	bus := newTestNATSEventBus(t)
	first := bus.Close()
	second := bus.Close()
	if (first == nil) != (second == nil) {
		t.Fatalf("idempotent Close diverged: first=%v second=%v", first, second)
	}
	if first != nil && second != nil && first.Error() != second.Error() {
		t.Fatalf("idempotent Close returned divergent errors: %q vs %q", first, second)
	}
}

// PR2-core (Task 3, ce-doc-review #3): after Close(), RegisterTenant must reject
// with an "eventbus closed" error. RED on current code (RegisterTenant succeeds
// after Close today) → GREEN after the closed-gate.
func TestRegisterTenant_RejectedAfterClose(t *testing.T) {
	bus := newTestNATSEventBus(t)
	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	err := bus.RegisterTenant(1, 16)
	if err == nil {
		t.Fatal("expected RegisterTenant to fail after Close, got nil")
	}
}

// PR2-core (Task 3, ce-doc-review #3): after Close(), UnregisterTenant must
// reject with an "eventbus closed" error (new closed-gate, takes precedence
// over the not-registered check).
func TestUnregisterTenant_RejectedAfterClose(t *testing.T) {
	bus := newTestNATSEventBus(t)
	_ = bus.RegisterTenant(1, 16) // pre-register so the not-registered path is not the reason
	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	err := bus.UnregisterTenant(1)
	if err == nil {
		t.Fatal("expected UnregisterTenant to fail after Close, got nil")
	}
}

// PR2-core (Task 1 gate, now reachable post-Close): after Close(), a send to a
// tenant channel must return AdmissionRejectedFrozen (Close flipped `closed`
// first). Asserts Task 1's frozen-admission gate is reachable via Close.
func TestSendResult_FrozenAfterClose(t *testing.T) {
	bus := newTestNATSEventBus(t)
	_ = bus.RegisterTenant(1, 16)
	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	out := bus.sendResultToChannel(pubResult(1, "e"))
	if out != AdmissionRejectedFrozen {
		t.Fatalf("expected AdmissionRejectedFrozen after Close, got %v", out)
	}
}
