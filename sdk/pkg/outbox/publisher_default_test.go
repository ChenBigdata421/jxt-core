package outbox

import "testing"

func TestDefaultPublisherConfig_BatchingOffByDefault(t *testing.T) {
	cfg := DefaultPublisherConfig()
	if cfg.ACKBatchSize != 0 {
		t.Fatalf("nil-config default must be ACKBatchSize=0 (off), got %d", cfg.ACKBatchSize)
	}
}
