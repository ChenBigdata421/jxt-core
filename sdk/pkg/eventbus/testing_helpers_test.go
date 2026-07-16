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
