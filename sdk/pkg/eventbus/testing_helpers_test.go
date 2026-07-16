package eventbus

import (
	"testing"

	"go.uber.org/zap"
)

// testing_helpers_test.go provides broker-free constructors for registry-lifecycle
// tests (PR2 plan, decision D4). These helpers build the driver struct DIRECTLY —
// no real NATS connection, no dial — so ACK admission / register / unregister
// lifecycle can be exercised without a broker. The registry layer must NEVER
// touch the broker connection; if that invariant breaks, tests here will surface it.

// newTestNATSEventBus constructs a *natsEventBus sufficient for registry-lifecycle
// tests: nop logger, an open global ACK channel, and an initialized tenant-channel
// map. conn/js (atomic.Value) are left at zero value (nil); tests MUST NOT touch
// the broker connection.
func newTestNATSEventBus(t *testing.T) *natsEventBus {
	t.Helper()
	return &natsEventBus{
		logger:                   zap.NewNop(),
		publishResultChan:        make(chan *PublishResult, 16),
		tenantPublishResultChans: make(map[int]chan *PublishResult),
		// closeDone MUST be initialized for Close() to work (Task 3): Close()
		// blocks every caller on <-closeDone and closes it inside closeOnce.Do.
		// closeOnce/terminalErr/closed stay at their correct zero values.
		closeDone: make(chan struct{}),
	}
}

// newTestKafkaEventBus constructs a *kafkaEventBus sufficient for registry-lifecycle
// tests (PR2 plan, decision D4 — byte-parallel with newTestNATSEventBus). Builds the
// driver struct DIRECTLY: no real sarama broker, no dial. asyncProducer/consumer/
// client/admin (atomic.Value) are left at zero value (nil); tests MUST NOT touch the
// broker connection. The registry layer must NEVER touch the broker connection; if
// that invariant breaks, tests here will surface it.
func newTestKafkaEventBus(t *testing.T) *kafkaEventBus {
	t.Helper()
	return &kafkaEventBus{
		logger:                   zap.NewNop(),
		publishResultChan:        make(chan *PublishResult, 16),
		tenantPublishResultChans: make(map[int]chan *PublishResult),
		// closeDone MUST be initialized for Close() to work (Task 3): Close()
		// blocks every caller on <-closeDone and closes it inside closeOnce.Do.
		// closeOnce/terminalErr/closed stay at their correct zero values.
		closeDone: make(chan struct{}),
	}
}

// newTestMemoryEventBus constructs an *eventBusManager sufficient for registry-
// lifecycle tests (PR2 plan, decision D5 — memory-backend only; byte-parallel with
// newTestNATSEventBus/newTestKafkaEventBus). Builds the manager struct DIRECTLY:
// no publisher/subscriber/health-checker wiring (all nil), so Close() runs only the
// tenant-channel teardown + terminalErr path — exactly the registry contract under test.
//
// D5 note: eventBusManager is the runtime type ONLY for the memory backend
// (NewEventBus returns *natsEventBus/*kafkaEventBus for nats/kafka). This is memory-
// backend correctness coverage, NOT NATS/Kafka prod coverage — does not count toward
// spec §3.9. The material adaptation: eventBusManager.closed is a plain bool guarded
// by m.mu (NOT atomic.Bool), so this helper verifies the snapshot-under-RLock gate.
func newTestMemoryEventBus(t *testing.T) *eventBusManager {
	t.Helper()
	return &eventBusManager{
		publishResultChan:        make(chan *PublishResult, 16),
		tenantPublishResultChans: make(map[int]chan *PublishResult),
		// closeDone MUST be initialized for Close() to work (Task 2 mirror): Close()
		// blocks every caller on <-closeDone and closes it inside closeOnce.Do.
		// closeOnce/terminalErr/closed stay at their correct zero values (closed=false).
		closeDone: make(chan struct{}),
	}
}

// pubResult builds a minimal *PublishResult for admission tests. Only the fields
// the registry routing logic depends on (TenantID, EventID) are set.
func pubResult(tenantID int, eventID string) *PublishResult {
	return &PublishResult{
		TenantID: tenantID,
		EventID:  eventID,
	}
}
