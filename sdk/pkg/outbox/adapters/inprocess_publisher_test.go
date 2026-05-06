package adapters

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

func newTestEnvelope(eventID string) *outbox.Envelope {
	return &outbox.Envelope{
		EventID:       eventID,
		AggregateID:   "agg-001",
		EventType:     "TestCreated",
		EventVersion:  1,
		Payload:       []byte(`{"test": true}`),
		Timestamp:     time.Now(),
		TraceID:       "trace-001",
		CorrelationID: "corr-001",
		TenantID:      42,
	}
}

func TestInProcessEventPublisher_RegisterAndDispatch(t *testing.T) {
	publisher := NewInProcessEventPublisher()
	defer publisher.Close()

	var called int32
	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		atomic.AddInt32(&called, 1)
		return nil
	})

	envelope := newTestEnvelope("evt-001")
	err := publisher.PublishEnvelope(context.Background(), "test.topic", envelope)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if atomic.LoadInt32(&called) != 1 {
		t.Fatalf("expected handler called 1 time, got: %d", atomic.LoadInt32(&called))
	}

	result := <-publisher.GetPublishResultChannel()
	if !result.Success {
		t.Fatalf("expected success ACK, got failure: %v", result.Error)
	}
	if result.EventID != "evt-001" {
		t.Fatalf("expected EventID evt-001, got: %s", result.EventID)
	}
}

func TestInProcessEventPublisher_MultipleHandlers(t *testing.T) {
	publisher := NewInProcessEventPublisher()
	defer publisher.Close()

	var callCount int32
	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})
	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	envelope := newTestEnvelope("evt-002")
	err := publisher.PublishEnvelope(context.Background(), "test.topic", envelope)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 2 {
		t.Fatalf("expected handlers called 2 times, got: %d", atomic.LoadInt32(&callCount))
	}

	result := <-publisher.GetPublishResultChannel()
	if !result.Success {
		t.Fatalf("expected success ACK, got failure: %v", result.Error)
	}
}

func TestInProcessEventPublisher_HandlerError(t *testing.T) {
	publisher := NewInProcessEventPublisher()
	defer publisher.Close()

	handlerErr := errors.New("handler failed")
	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		return handlerErr
	})

	envelope := newTestEnvelope("evt-003")
	err := publisher.PublishEnvelope(context.Background(), "test.topic", envelope)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != handlerErr {
		t.Fatalf("expected handlerErr, got: %v", err)
	}

	result := <-publisher.GetPublishResultChannel()
	if result.Success {
		t.Fatal("expected failure ACK, got success")
	}
	if result.Error != handlerErr {
		t.Fatalf("expected ACK error to be handlerErr, got: %v", result.Error)
	}
}

func TestInProcessEventPublisher_PartialFailure(t *testing.T) {
	publisher := NewInProcessEventPublisher()
	defer publisher.Close()

	var calls [3]int32
	handlerErr := errors.New("middle handler failed")

	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		atomic.StoreInt32(&calls[0], 1)
		return nil
	})
	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		atomic.StoreInt32(&calls[1], 1)
		return handlerErr
	})
	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		atomic.StoreInt32(&calls[2], 1)
		return nil
	})

	envelope := newTestEnvelope("evt-004")
	err := publisher.PublishEnvelope(context.Background(), "test.topic", envelope)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != handlerErr {
		t.Fatalf("expected handlerErr, got: %v", err)
	}

	for i, called := range calls {
		if called != 1 {
			t.Fatalf("expected handler %d to be called (no short-circuit), but it was not", i)
		}
	}

	result := <-publisher.GetPublishResultChannel()
	if result.Success {
		t.Fatal("expected failure ACK, got success")
	}
}

func TestInProcessEventPublisher_NoHandler(t *testing.T) {
	publisher := NewInProcessEventPublisher()
	defer publisher.Close()

	envelope := newTestEnvelope("evt-005")
	err := publisher.PublishEnvelope(context.Background(), "unknown.topic", envelope)
	if err != nil {
		t.Fatalf("expected no error for unregistered topic, got: %v", err)
	}

	result := <-publisher.GetPublishResultChannel()
	if !result.Success {
		t.Fatalf("expected success ACK for no-op, got failure: %v", result.Error)
	}
}

func TestInProcessEventPublisher_WithBufferSize(t *testing.T) {
	publisher := NewInProcessEventPublisher(WithBufferSize(50))
	defer publisher.Close()

	for i := 0; i < 50; i++ {
		envelope := newTestEnvelope("evt-buf")
		err := publisher.PublishEnvelope(context.Background(), "test.topic", envelope)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	}

	resultChan := publisher.GetPublishResultChannel()
	count := 0
	for {
		select {
		case <-resultChan:
			count++
		default:
			goto done
		}
	}
done:
	if count != 50 {
		t.Fatalf("expected 50 results in channel, got: %d", count)
	}
}

func TestInProcessEventPublisher_ContextCancellation(t *testing.T) {
	publisher := NewInProcessEventPublisher()
	defer publisher.Close()

	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	envelope := newTestEnvelope("evt-006")
	err := publisher.PublishEnvelope(ctx, "test.topic", envelope)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	result := <-publisher.GetPublishResultChannel()
	if result.Success {
		t.Fatal("expected failure ACK, got success")
	}
}

func TestInProcessEventPublisher_TenantIDPassthrough(t *testing.T) {
	publisher := NewInProcessEventPublisher()
	defer publisher.Close()

	var receivedTenantID int
	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		receivedTenantID = envelope.TenantID
		return nil
	})

	envelope := &outbox.Envelope{
		EventID:   "evt-007",
		TenantID:  99,
		Payload:   []byte(`{}`),
		Timestamp: time.Now(),
	}

	err := publisher.PublishEnvelope(context.Background(), "test.topic", envelope)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if receivedTenantID != 99 {
		t.Fatalf("expected handler to receive TenantID 99, got: %d", receivedTenantID)
	}

	result := <-publisher.GetPublishResultChannel()
	if result.TenantID != 99 {
		t.Fatalf("expected ACK TenantID 99, got: %d", result.TenantID)
	}
}

func TestInProcessEventPublisher_ConcurrentRegister(t *testing.T) {
	publisher := NewInProcessEventPublisher()
	defer publisher.Close()

	var wg sync.WaitGroup
	const goroutines = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			publisher.RegisterHandler("concurrent.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
				return nil
			})
		}(i)
	}
	wg.Wait()

	envelope := newTestEnvelope("evt-concurrent")
	err := publisher.PublishEnvelope(context.Background(), "concurrent.topic", envelope)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	result := <-publisher.GetPublishResultChannel()
	if !result.Success {
		t.Fatalf("expected success ACK, got failure: %v", result.Error)
	}

	publisher.mu.RLock()
	handlerCount := len(publisher.handlers["concurrent.topic"])
	publisher.mu.RUnlock()

	if handlerCount != goroutines {
		t.Fatalf("expected %d handlers, got: %d", goroutines, handlerCount)
	}
}

func TestInProcessEventPublisher_DoubleClose(t *testing.T) {
	publisher := NewInProcessEventPublisher()

	err := publisher.Close()
	if err != nil {
		t.Fatalf("first Close() returned error: %v", err)
	}

	err = publisher.Close()
	if err != nil {
		t.Fatalf("second Close() returned error: %v", err)
	}

	resultChan := publisher.GetPublishResultChannel()
	_, ok := <-resultChan
	if ok {
		t.Fatal("expected resultChan to be closed after Close()")
	}
}

func TestInProcessEventPublisher_CloseDuringPublish(t *testing.T) {
	publisher := NewInProcessEventPublisher()

	publisher.RegisterHandler("test.topic", func(ctx context.Context, envelope *outbox.Envelope) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	const goroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			envelope := newTestEnvelope("evt-close-race")
			publisher.PublishEnvelope(context.Background(), "test.topic", envelope)
		}(i)
	}

	time.Sleep(2 * time.Millisecond)
	publisher.Close()

	wg.Wait()
	// 不应 panic — 如果到达这里说明 sendResult 的 recover() 正常工作
}

func TestInProcessEventPublisher_NilEnvelope(t *testing.T) {
	publisher := NewInProcessEventPublisher()
	defer publisher.Close()

	err := publisher.PublishEnvelope(context.Background(), "test.topic", nil)
	if err == nil {
		t.Fatal("expected error for nil envelope, got nil")
	}
	if err.Error() != "envelope must not be nil" {
		t.Fatalf("unexpected error message: %v", err)
	}
}
