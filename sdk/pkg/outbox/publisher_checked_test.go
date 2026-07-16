package outbox

import "testing"

// Task 6 (PR2-hardening): NewOutboxPublisherChecked returns an error instead of
// panicking on invalid config. The constructor only stores repo/eventPublisher/
// topicMapper (it does not call any of their methods during construction), so
// passing nil for those args is safe here.

func TestNewOutboxPublisherChecked_RejectsInvalidConfig(t *testing.T) {
	p, err := NewOutboxPublisherChecked(nil, nil, nil, &PublisherConfig{ACKBatchSize: -1})
	if err == nil {
		t.Fatal("expected error for ACKBatchSize=-1, got nil")
	}
	if p != nil {
		t.Fatalf("expected nil publisher on error, got non-nil %v", p)
	}
}

func TestNewOutboxPublisherChecked_AcceptsValidConfig(t *testing.T) {
	p, err := NewOutboxPublisherChecked(nil, nil, nil, &PublisherConfig{ACKBatchSize: 100})
	if err != nil {
		t.Fatalf("expected nil error for valid config, got %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil publisher for valid config, got nil")
	}
}

// TestNewOutboxPublisherChecked_NilConfigUsesDefault guards the nil-config branch:
// a nil config must not panic/error — it must fall back to DefaultPublisherConfig()
// (which has ACKBatchSize=0, the PR2 default-off value).
func TestNewOutboxPublisherChecked_NilConfigUsesDefault(t *testing.T) {
	p, err := NewOutboxPublisherChecked(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected nil error for nil config, got %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil publisher for nil config, got nil")
	}
}
