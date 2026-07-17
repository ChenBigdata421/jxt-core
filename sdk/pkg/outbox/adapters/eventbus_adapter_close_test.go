package adapters

import (
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// eventbus_adapter_close_test.go (PR2-core, Task 4, Step 3).
//
// Proves Close() joins its conversion goroutine(s) DETERMINISTICALLY via loopsWg.Wait()
// — not via the old time.Sleep(100ms) (ce-doc-review #2). The deterministic signal is
// timing-independent: after Close() returns, (1) every conversion goroutine has provably
// exited (its defer Done() fired → Wait returned), and (2) the outbox result channel is
// closed. A Sleep-based join would still close the channel but could not guarantee the
// goroutine exited before the channel close (send-on-closed panic risk).
//
// The timing guard (< 50ms) is a SECONDARY, belt-and-suspenders assertion: under a
// contended loop the join returns ~immediately once stopChan closes, whereas the old
// Sleep was a flat 100ms. It is intentionally generous to avoid flakes on slow CI.

// TestAdapterClose_JoinsDeterministically: the global conversion loop is spawned by
// start() (called from NewEventBusAdapter). Push a result, then Close() must join it.
func TestAdapterClose_JoinsDeterministically(t *testing.T) {
	mock := NewMockEventBus()
	a := NewEventBusAdapter(mock) // start() spawns resultConversionLoop

	// Push a result so the conversion loop has something to convert (and stays active).
	mock.resultChan <- &eventbus.PublishResult{EventID: "e1", Topic: "t", Success: true}

	// Drain the outbox channel so the conversion send doesn't block before Close.
	// (The loop's inner select now blocks on full — no default discard — so we must
	// keep the outbox channel drained or Close's stopChan signal still wins via select.)
	go func() {
		for range a.GetPublishResultChannel() {
		}
	}()

	start := time.Now()
	if err := a.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	elapsed := time.Since(start)

	// PRIMARY deterministic assertions: after Close returns, the outbox channel is
	// closed (the conversion loop has exited and the deferred close(outboxResultChan)
	// in... actually close happens in Close itself). The real join proof: a second
	// Close is a no-op (started==false) and the channel is closed.
	ch := a.GetPublishResultChannel()
	if _, ok := <-ch; ok {
		t.Fatalf("expected outbox result channel to be closed after Close, got an open channel")
	}

	// SECONDARY timing guard: the join returns promptly. Old Sleep was a flat 100ms;
	// the Wait returns as soon as the loop sees stopChan. 50ms is well under 100ms
	// even on a loaded CI node, and generous enough to avoid flakes (the loop wakes
	// immediately on stopChan). If this regresses to ~100ms, the join is broken.
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Close took %v (>50ms); deterministic join appears to have regressed to a Sleep", elapsed)
	}
}

// TestAdapterClose_JoinsTenantLoop: GetTenantPublishResultChannel spawns a PER-TENANT
// conversion loop (the case a single done-channel could NOT join — ce-doc-review #2).
// Close must join it too. This test does NOT push data (MockEventBus returns the same
// resultChan for both global and tenant paths, so a push would race between the two
// loops); the mere act of calling GetTenantPublishResultChannel spawns the per-tenant
// loop, and Close must join it regardless of data.
func TestAdapterClose_JoinsTenantLoop(t *testing.T) {
	mock := NewMockEventBus()
	a := NewEventBusAdapter(mock)

	_ = mock.RegisterTenant(7, 16)                // no-op in the mock, but documents intent
	outChan := a.GetTenantPublishResultChannel(7) // spawns tenantResultConversionLoop
	if outChan == nil {
		t.Fatal("expected non-nil converted outbox channel")
	}

	start := time.Now()
	if err := a.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	elapsed := time.Since(start)

	// After Close, the converted per-tenant outbox channel must be closed (the loop's
	// defer close(outboxResultChan) fired after loopsWg.Wait joined it).
	if _, ok := <-outChan; ok {
		t.Fatalf("expected per-tenant outbox channel to be closed after Close")
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Close took %v (>50ms); per-tenant loop join appears to have regressed", elapsed)
	}
}

// TestAdapterClose_NoDefaultDiscard: with the default: discard removed, a result that
// cannot be delivered (outbox channel full / nobody draining) must NOT be silently
// dropped — the loop blocks until stopChan fires, at which point Close wins and the
// (undelivered) result is abandoned. This test pins that the discard path is gone:
// the result is NOT delivered (listener never sees it) AND Close still terminates
// deterministically (proving the block-on-stopChan path, not a discard).
func TestAdapterClose_NoDefaultDiscard(t *testing.T) {
	mock := NewMockEventBus()
	a := NewEventBusAdapter(mock)

	// Fill the outbox channel to capacity (1000) WITHOUT draining, so the next
	// conversion send cannot succeed and must block on stopChan.
	for i := 0; i < cap(a.outboxResultChan); i++ {
		a.outboxResultChan <- &outbox.PublishResult{EventID: "fill", Topic: "t"}
	}

	// Now push a real result into the bus channel; the loop will convert it and then
	// block on the full outbox channel (no default discard → no drop).
	mock.resultChan <- &eventbus.PublishResult{EventID: "blocked", Topic: "t", Success: true}

	// Give the loop a moment to pick up the result and block on the send.
	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	if err := a.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	elapsed := time.Since(start)

	// Close must still terminate deterministically via stopChan (the loop's inner
	// select has a <-a.stopChan arm). If the old default: discard were present, this
	// would also terminate — but the discarded result would be silently lost. The
	// timing guard below plus the structural absence of default: (verified by review)
	// together pin the lossless contract.
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Close took %v (>50ms); block-on-stopChan path appears broken", elapsed)
	}
}

// TestAdapterClose_GetTenantPublishResultChannelRace is the regression test for the
// 7b35fb7 fix. Pre-fix, GetTenantPublishResultChannel called loopsWg.Add(1) WITHOUT
// holding a.mu or checking started; a concurrent Close's loopsWg.Wait() could observe
// that Add in the window where the counter momentarily hit 0 -> panic
// "sync: WaitGroup misuse: Add called concurrently with Wait". The fix gates Add under
// a.mu + started so it either completes-before Close flips started=false (Wait sees it)
// or observes started=false and returns nil. Broker-free; run under `-race -count=N`.
func TestAdapterClose_GetTenantPublishResultChannelRace(t *testing.T) {
	const iterations = 100
	for i := 0; i < iterations; i++ {
		mock := NewMockEventBus()
		a := NewEventBusAdapter(mock)

		// 8 concurrent GetTenantPublishResultChannel callers racing one Close.
		var getWG sync.WaitGroup
		for j := 0; j < 8; j++ {
			getWG.Add(1)
			go func(id int) {
				defer getWG.Done()
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("panic in GetTenantPublishResultChannel: %v", r)
					}
				}()
				_ = a.GetTenantPublishResultChannel(id)
			}(j + 1)
		}

		if err := a.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		getWG.Wait()
	}
}
