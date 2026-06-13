package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ChenBigdata421/jxt-core/storage"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupMiniredis creates an in-memory Redis server and a connected client.
// Both are cleaned up automatically when the test finishes.
func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})
	return mr, client
}

// defaultConsumerCfg returns a ConsumerConfig tuned for fast tests.
func defaultConsumerCfg() *ConsumerConfig {
	return &ConsumerConfig{
		VisibilityTimeout: 2, // seconds — short for reclaim test
		BlockingTimeout:   1, // seconds — short so poll returns quickly
		ReclaimInterval:   1, // seconds — reclaim ticks fast
		BufferSize:        100,
		Concurrency:       2,
	}
}

// wait blocks up to d for cond to return true, polling every 20ms.
func wait(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", d)
}

// ---------------------------------------------------------------------------
// Test 1: Callback succeeds → message XACK'd, pending cleared
// ---------------------------------------------------------------------------

func TestConsumer_XACKAfterSuccess(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-ack"
	group := "test-group-ack"

	var handlerCalled atomic.Int32

	handler := func(msg storage.Messager) error {
		handlerCalled.Add(1)
		return nil
	}

	cfg := defaultConsumerCfg()
	consumer, err := NewStreamConsumer(client, stream, group, cfg, handler)
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}
	consumer.Run()
	defer consumer.Shutdown()

	// Produce a message directly into the stream.
	_, err = client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"hello": "world"},
		ID:     "*",
	}).Result()
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}

	// Wait for the message to be processed AND ACKed.
	// We poll the pending count rather than just the handler counter,
	// because XACK happens after the handler returns.
	wait(t, 3*time.Second, func() bool {
		if handlerCalled.Load() < 1 {
			return false
		}
		pending, err := client.XPending(context.Background(), stream, group).Result()
		if err != nil {
			return false
		}
		return pending.Count == 0
	})
}

// ---------------------------------------------------------------------------
// Test 2: Callback returns error → NO XACK, message stays in pending
// ---------------------------------------------------------------------------

func TestConsumer_NoACKOnCallbackError(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-noack"
	group := "test-group-noack"

	var handlerCalled atomic.Int32

	handler := func(msg storage.Messager) error {
		handlerCalled.Add(1)
		return errors.New("intentional callback failure")
	}

	cfg := defaultConsumerCfg()
	// Use a long visibility timeout so reclaim doesn't pick it up during the test.
	cfg.VisibilityTimeout = 300

	consumer, err := NewStreamConsumer(client, stream, group, cfg, handler)
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}
	consumer.Run()
	defer consumer.Shutdown()

	// Produce a message.
	_, err = client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"key": "value"},
		ID:     "*",
	}).Result()
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}

	// Wait for the handler to process the message.
	wait(t, 3*time.Second, func() bool {
		return handlerCalled.Load() >= 1
	})

	// Give a small window for any async ACK to happen.
	time.Sleep(200 * time.Millisecond)

	// Verify the message was NOT ACKed — pending count should be > 0.
	pending, err := client.XPending(context.Background(), stream, group).Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count == 0 {
		t.Error("expected pending messages after callback error, got 0")
	}

	// Verify the error was pushed to the error channel.
	select {
	case consumerErr := <-consumer.Errors():
		if consumerErr == nil {
			t.Error("expected non-nil error on error channel")
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for error on error channel")
	}
}

// ---------------------------------------------------------------------------
// Test 3: Happy-path message processed exactly once — no double delivery
// ---------------------------------------------------------------------------

func TestConsumer_NoDoubleProcessOnHappyPath(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-happy"
	group := "test-group-happy"

	var handlerCalled atomic.Int32

	handler := func(msg storage.Messager) error {
		handlerCalled.Add(1)
		return nil
	}

	cfg := defaultConsumerCfg()
	cfg.VisibilityTimeout = 1 // short timeout — reclaim runs but finds nothing to reclaim
	cfg.ReclaimInterval = 1
	cfg.BlockingTimeout = 1

	consumer, err := NewStreamConsumer(client, stream, group, cfg, handler)
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}
	consumer.Run()
	defer consumer.Shutdown()

	// Produce a single message.
	_, err = client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"happy": "path"},
		ID:     "*",
	}).Result()
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}

	// Wait for the handler to process the message.
	wait(t, 5*time.Second, func() bool {
		return handlerCalled.Load() >= 1
	})

	// Give reclaim time to run at least once — it should find nothing to reclaim
	// because the message was already ACKed.
	time.Sleep(2 * time.Second)

	count := handlerCalled.Load()
	if count != 1 {
		t.Errorf("expected exactly 1 handler call (no double processing), got %d", count)
	}

	// Verify message is fully ACKed.
	pending, err := client.XPending(context.Background(), stream, group).Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("expected 0 pending after happy-path ACK, got %d", pending.Count)
	}
}

// TestConsumer_ReclaimPendingMessage tests that a message left pending by a
// different consumer (simulating a crash) gets reclaimed and processed.
func TestConsumer_ReclaimPendingMessage(t *testing.T) {
	mr, client := setupMiniredis(t)

	stream := "test-stream-reclaim-pending"
	group := "test-group-reclaim-pending"

	// Step 1: Create the stream and group, then add a message.
	err := client.XGroupCreateMkStream(context.Background(), stream, group, "0").Err()
	if err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}

	msgID, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"reclaim": "pending"},
		ID:     "*",
	}).Result()
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}

	// Step 2: Read the message as a "phantom" consumer that never ACKs.
	// This leaves the message in the pending list.
	phantomResults, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "phantom-consumer",
		Streams:  []string{stream, ">"},
		Count:    1,
	}).Result()
	if err != nil {
		t.Fatalf("phantom XReadGroup: %v", err)
	}
	if len(phantomResults) == 0 || len(phantomResults[0].Messages) == 0 {
		t.Fatal("phantom consumer did not read any messages")
	}
	_ = phantomResults // message is now pending, phantom never ACKs

	// Step 3: Advance miniredis time so the message exceeds visibility timeout.
	mr.FastForward(3 * time.Second)

	// Step 4: Start the consumer — it should reclaim the pending message.
	var handlerCalled atomic.Int32
	var receivedID atomic.Value

	handler := func(msg storage.Messager) error {
		handlerCalled.Add(1)
		receivedID.Store(msg.GetID())
		return nil
	}

	cfg := defaultConsumerCfg()
	cfg.VisibilityTimeout = 2 // 2 seconds
	cfg.ReclaimInterval = 1   // check every second
	cfg.BlockingTimeout = 1

	consumer, err := NewStreamConsumer(client, stream, group, cfg, handler)
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}
	consumer.Run()
	defer consumer.Shutdown()

	// Wait for reclaim to pick up the pending message.
	wait(t, 8*time.Second, func() bool {
		return handlerCalled.Load() >= 1
	})

	// The message should have been reclaimed and processed exactly once.
	count := handlerCalled.Load()
	if count != 1 {
		t.Errorf("expected exactly 1 handler call via reclaim, got %d", count)
	}

	// Verify the reclaimed message has the correct ID.
	gotID, _ := receivedID.Load().(string)
	if gotID != msgID {
		t.Errorf("expected message ID %q, got %q", msgID, gotID)
	}

	// After ACK, no messages should be pending.
	time.Sleep(500 * time.Millisecond)
	pending, err := client.XPending(context.Background(), stream, group).Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("expected 0 pending after reclaim ACK, got %d", pending.Count)
	}
}

// ---------------------------------------------------------------------------
// Test 4: Shutdown waits for in-flight messages to finish (WaitGroup)
// ---------------------------------------------------------------------------

func TestConsumer_ShutdownDrains(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-drain"
	group := "test-group-drain"

	var (
		handlerStarted  atomic.Int32
		handlerFinished atomic.Int32
		handlerBlock    = make(chan struct{}) // block handler until we signal
	)

	handler := func(msg storage.Messager) error {
		handlerStarted.Add(1)
		<-handlerBlock // block until test signals
		handlerFinished.Add(1)
		return nil
	}

	cfg := defaultConsumerCfg()
	consumer, err := NewStreamConsumer(client, stream, group, cfg, handler)
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}
	consumer.Run()

	// Produce a message.
	_, err = client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"drain": "test"},
		ID:     "*",
	}).Result()
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}

	// Wait for the handler to start processing.
	wait(t, 3*time.Second, func() bool {
		return handlerStarted.Load() >= 1
	})

	// At this point, the handler is blocked. Shutdown should not return
	// until the handler finishes.
	shutdownDone := make(chan struct{})
	go func() {
		consumer.Shutdown()
		close(shutdownDone)
	}()

	// Give Shutdown a moment to be called.
	time.Sleep(200 * time.Millisecond)

	// Shutdown should NOT be done yet because handler is still running.
	select {
	case <-shutdownDone:
		t.Error("Shutdown returned before in-flight handler finished")
	default:
		// Good — Shutdown is still waiting.
	}

	// Unblock the handler.
	close(handlerBlock)

	// Now Shutdown should complete.
	select {
	case <-shutdownDone:
		// Success — Shutdown waited for the handler.
	case <-time.After(5 * time.Second):
		t.Error("Shutdown did not complete within 5s after handler unblocked")
	}

	if handlerFinished.Load() != 1 {
		t.Errorf("expected handler to finish exactly once, got %d", handlerFinished.Load())
	}
}

// ---------------------------------------------------------------------------
// Test 5: Buffered errCh doesn't block consumer when full
// ---------------------------------------------------------------------------

func TestConsumer_ErrChNeverBlocks(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-errch"
	group := "test-group-errch"

	var handlerCalled atomic.Int32

	// Handler that always fails — generates an error per message.
	handler := func(msg storage.Messager) error {
		handlerCalled.Add(1)
		return errors.New("always fails")
	}

	cfg := defaultConsumerCfg()
	cfg.BufferSize = 2 // tiny buffer for errCh
	cfg.VisibilityTimeout = 300

	consumer, err := NewStreamConsumer(client, stream, group, cfg, handler)
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}
	consumer.Run()

	// Produce multiple messages — more than the errCh buffer size.
	const msgCount = 10
	for i := 0; i < msgCount; i++ {
		_, err := client.XAdd(context.Background(), &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{"idx": fmt.Sprintf("%d", i)},
			ID:     "*",
		}).Result()
		if err != nil {
			t.Fatalf("XAdd %d: %v", i, err)
		}
	}

	// Wait for all messages to be processed.
	wait(t, 10*time.Second, func() bool {
		return handlerCalled.Load() >= int32(msgCount)
	})

	// If pushErr blocks, the consumer goroutines would deadlock and the
	// handler count would never reach msgCount. Getting here means
	// errCh never blocked the consumer.
	handlerCount := handlerCalled.Load()
	if handlerCount < int32(msgCount) {
		t.Errorf("expected at least %d handler calls, got %d — consumer may have blocked on errCh",
			msgCount, handlerCount)
	}

	consumer.Shutdown()

	// Drain the error channel — should have at most BufferSize errors.
	errCount := 0
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case <-consumer.Errors():
			errCount++
		case <-timeout:
			goto done
		}
	}
done:
	// The error channel is small (BufferSize=2) but some errors may have been
	// drained by the consumer itself. We just verify there are some errors and
	// the count is bounded.
	t.Logf("drained %d errors from errCh (buffer size %d)", errCount, cfg.BufferSize)
	if errCount == 0 {
		t.Error("expected at least 1 error in errCh")
	}
}

// ---------------------------------------------------------------------------
// Test 6: XADD succeeds + BUSYGROUP is idempotent
// ---------------------------------------------------------------------------

func TestProducer_EnqueueAndEnsureStream(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-producer"
	group := "test-group-producer"

	// --- Producer tests ---

	producer := NewStreamProducer(client, &ProducerConfig{})

	// XADD should succeed and return an ID.
	id, err := producer.Send(context.Background(), stream, map[string]interface{}{
		"action": "enqueue",
		"count":  42,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty message ID from Send")
	}
	t.Logf("Send returned ID: %s", id)

	// XADD with MAXLEN.
	producerMax := NewStreamProducer(client, &ProducerConfig{
		StreamMaxLength:      10,
		ApproximateMaxLength: true,
	})
	for i := 0; i < 20; i++ {
		_, err := producerMax.Send(context.Background(), stream, map[string]interface{}{
			"i": i,
		})
		if err != nil {
			t.Fatalf("Send %d with MAXLEN: %v", i, err)
		}
	}

	// --- ensureGroup idempotency ---

	// First call creates the group.
	err = ensureGroup(client, stream, group)
	if err != nil {
		t.Fatalf("ensureGroup (first call): %v", err)
	}

	// Second call should succeed (BUSYGROUP is treated as success).
	err = ensureGroup(client, stream, group)
	if err != nil {
		t.Fatalf("ensureGroup (second call, should be idempotent): %v", err)
	}

	// Verify the group exists by reading new messages from it.
	results, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "test-consumer",
		Streams:  []string{stream, ">"}, // ">" = only new, undelivered messages
		Count:    100,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup after ensureGroup: %v", err)
	}
	// Should have at least the messages we added.
	total := 0
	for _, xs := range results {
		total += len(xs.Messages)
	}
	if total == 0 {
		t.Error("expected to read messages from the stream via consumer group")
	}
	t.Logf("read %d messages from consumer group", total)
}

// ---------------------------------------------------------------------------
// Extra: Test NewStreamConsumer default config handling
// ---------------------------------------------------------------------------

func TestNewStreamConsumer_DefaultConfig(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-defaults"
	group := "test-group-defaults"

	// Pass nil config — should apply defaults.
	consumer, err := NewStreamConsumer(client, stream, group, nil, func(msg storage.Messager) error {
		return nil
	})
	if err != nil {
		t.Fatalf("NewStreamConsumer with nil config: %v", err)
	}
	if consumer.cfg.VisibilityTimeout != 60 {
		t.Errorf("expected default VisibilityTimeout=60, got %d", consumer.cfg.VisibilityTimeout)
	}
	if consumer.cfg.BlockingTimeout != 5 {
		t.Errorf("expected default BlockingTimeout=5, got %d", consumer.cfg.BlockingTimeout)
	}
	if consumer.cfg.ReclaimInterval != 1 {
		t.Errorf("expected default ReclaimInterval=1, got %d", consumer.cfg.ReclaimInterval)
	}
	if consumer.cfg.BufferSize != 100 {
		t.Errorf("expected default BufferSize=100, got %d", consumer.cfg.BufferSize)
	}
	if consumer.cfg.Concurrency != 10 {
		t.Errorf("expected default Concurrency=10, got %d", consumer.cfg.Concurrency)
	}
}

// ---------------------------------------------------------------------------
// Extra: Test Redis adapter Append + Register + Run + Shutdown lifecycle
// ---------------------------------------------------------------------------

func TestRedis_Lifecycle(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-lifecycle"

	var handlerCalled atomic.Int32

	r, err := NewRedis(client, &ProducerConfig{}, defaultConsumerCfg())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}

	r.Register(stream, func(msg storage.Messager) error {
		handlerCalled.Add(1)
		return nil
	})

	r.Run()

	// Append a message through the adapter.
	msg := &Message{
		stream: stream,
		values: map[string]interface{}{"lifecycle": "test"},
	}
	if err := r.Append(msg); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if msg.GetID() == "" {
		t.Error("expected message ID to be set after Append")
	}

	// Wait for the handler to process.
	wait(t, 5*time.Second, func() bool {
		return handlerCalled.Load() >= 1
	})

	if handlerCalled.Load() != 1 {
		t.Errorf("expected 1 handler call, got %d", handlerCalled.Load())
	}

	// Shutdown should complete without hanging.
	done := make(chan struct{})
	go func() {
		r.Shutdown()
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Error("Shutdown hung")
	}
}

// ---------------------------------------------------------------------------
// Extra: Test pushErr does not block
// ---------------------------------------------------------------------------

func TestPushErr_NonBlocking(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-pusherr"
	group := "test-pusherr"

	cfg := defaultConsumerCfg()
	cfg.BufferSize = 1 // tiny buffer

	consumer, err := NewStreamConsumer(client, stream, group, cfg, func(msg storage.Messager) error {
		return nil
	})
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}

	// Push more errors than the buffer can hold.
	// This should never block — the overflow is silently dropped.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			consumer.pushErr(fmt.Errorf("error %d", i))
		}
	}()

	select {
	case <-done:
		// OK — pushErr never blocked.
	case <-time.After(5 * time.Second):
		t.Error("pushErr blocked — errCh overflow caused deadlock")
	}
}

// ---------------------------------------------------------------------------
// Extra: Test concurrent message processing
// ---------------------------------------------------------------------------

func TestConsumer_ConcurrentProcessing(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-concurrent"
	group := "test-group-concurrent"

	var (
		handlerCalled atomic.Int32
		mu            sync.Mutex
		ids           = make(map[string]bool)
	)

	handler := func(msg storage.Messager) error {
		handlerCalled.Add(1)
		mu.Lock()
		ids[msg.GetID()] = true
		mu.Unlock()
		return nil
	}

	cfg := defaultConsumerCfg()
	cfg.Concurrency = 4

	consumer, err := NewStreamConsumer(client, stream, group, cfg, handler)
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}
	consumer.Run()
	defer consumer.Shutdown()

	// Produce multiple messages.
	const msgCount = 20
	for i := 0; i < msgCount; i++ {
		_, err := client.XAdd(context.Background(), &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{"i": fmt.Sprintf("%d", i)},
			ID:     "*",
		}).Result()
		if err != nil {
			t.Fatalf("XAdd %d: %v", i, err)
		}
	}

	// Wait for all messages.
	wait(t, 10*time.Second, func() bool {
		return handlerCalled.Load() >= int32(msgCount)
	})

	count := handlerCalled.Load()
	if count != int32(msgCount) {
		t.Errorf("expected %d handler calls, got %d", msgCount, count)
	}

	// Verify each message was processed exactly once.
	mu.Lock()
	defer mu.Unlock()
	if len(ids) != msgCount {
		t.Errorf("expected %d unique message IDs, got %d", msgCount, len(ids))
	}
}

// ---------------------------------------------------------------------------
// C5 Gap 1: nil client → NewRedis returns error
// ---------------------------------------------------------------------------

func TestNewRedis_NilClient(t *testing.T) {
	_, err := NewRedis(nil, nil, nil)
	if err == nil {
		t.Error("expected error for nil client, got nil")
	}
	t.Logf("nil client error: %v", err)
}

// ---------------------------------------------------------------------------
// C5 Gap 2: Run() without Register() → InitError reports the problem
// ---------------------------------------------------------------------------

func TestRedis_RunWithoutRegister(t *testing.T) {
	_, client := setupMiniredis(t)

	r, err := NewRedis(client, &ProducerConfig{}, nil)
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}

	// Call Run without ever calling Register.
	r.Run()

	// Run() returns nothing (void), so check InitError().
	if err := r.InitError(); err == nil {
		t.Error("expected InitError after Run() with no Register(), got nil")
	} else {
		t.Logf("InitError (expected): %v", err)
	}

	// Shutdown should be safe even with no consumer.
	r.Shutdown()
}

// ---------------------------------------------------------------------------
// C5 Gap 3: double Run() → only one consumer created (sync.Once)
// ---------------------------------------------------------------------------

func TestRedis_DoubleRun(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-doublerun"

	var handlerCalled atomic.Int32

	r, err := NewRedis(client, &ProducerConfig{}, defaultConsumerCfg())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}

	r.Register(stream, func(msg storage.Messager) error {
		handlerCalled.Add(1)
		return nil
	})

	// Call Run twice — only the first should take effect.
	r.Run()
	r.Run()

	// Produce one message.
	msg := &Message{
		stream: stream,
		values: map[string]interface{}{"double": "run"},
	}
	if err := r.Append(msg); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Wait for processing — should be called exactly once.
	wait(t, 5*time.Second, func() bool {
		return handlerCalled.Load() >= 1
	})

	if count := handlerCalled.Load(); count != 1 {
		t.Errorf("expected exactly 1 handler call (no double-start), got %d", count)
	}

	r.Shutdown()
}

// ---------------------------------------------------------------------------
// C5 Gap 4: Append with empty stream → falls back to r.stream
// ---------------------------------------------------------------------------

func TestRedis_AppendFallbackStream(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-fallback"

	var received atomic.Value

	r, err := NewRedis(client, &ProducerConfig{}, defaultConsumerCfg())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}

	r.Register(stream, func(msg storage.Messager) error {
		received.Store(msg.GetStream())
		return nil
	})

	r.Run()
	defer r.Shutdown()

	// Append a message with NO stream set — should use the registered stream.
	msg := &Message{
		stream: "", // empty → fallback to r.stream
		values: map[string]interface{}{"fallback": "test"},
	}
	if err := r.Append(msg); err != nil {
		t.Fatalf("Append: %v", err)
	}

	wait(t, 5*time.Second, func() bool {
		v := received.Load()
		return v != nil && v.(string) == stream
	})

	got, _ := received.Load().(string)
	if got != stream {
		t.Errorf("expected stream %q, got %q", stream, got)
	}
}

// ---------------------------------------------------------------------------
// C5 Gap 5: MAXLEN trimming — stream stays within configured limit
// ---------------------------------------------------------------------------

func TestProducer_MAXLENTrimming(t *testing.T) {
	_, client := setupMiniredis(t)

	stream := "test-stream-maxlen"

	const maxLen = 10

	producer := NewStreamProducer(client, &ProducerConfig{
		StreamMaxLength:      maxLen,
		ApproximateMaxLength: true,
	})

	// Send more messages than MAXLEN.
	const total = 50
	for i := 0; i < total; i++ {
		_, err := producer.Send(context.Background(), stream, map[string]interface{}{
			"i": i,
		})
		if err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}

	// Check stream length with XRANGE.
	msgs, err := client.XRange(context.Background(), stream, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}

	// With approximate trimming the stream may temporarily exceed maxLen slightly,
	// but should be close. Allow up to 2× tolerance for approximation.
	actual := len(msgs)
	t.Logf("stream length after %d messages with MAXLEN=%d (approx): %d", total, maxLen, actual)

	if actual > maxLen*2 {
		t.Errorf("expected stream length ≤ %d (2× tolerance), got %d", maxLen*2, actual)
	}
}

// ---------------------------------------------------------------------------
// C5 Gap 5a: NewRedisWithConsumer routes producer/consumer clients correctly
// ---------------------------------------------------------------------------

func TestNewRedisWithConsumer_RoutesClientsCorrectly(t *testing.T) {
	// Two distinct clients, both pointing at the same miniredis so that
	// messages appended via the producer flow to the consumer.
	mr := miniredis.RunT(t)
	t.Cleanup(mr.Close)

	producerClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	consumerClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		producerClient.Close()
		consumerClient.Close()
	})

	r, err := NewRedisWithConsumer(producerClient, consumerClient, &ProducerConfig{}, defaultConsumerCfg())
	if err != nil {
		t.Fatalf("NewRedisWithConsumer: %v", err)
	}

	// Assert the clients are routed to the correct fields.
	if r.client != producerClient {
		t.Error("expected r.client == producerClient (producer field)")
	}
	if r.consumerClient != consumerClient {
		t.Error("expected r.consumerClient == consumerClient (consumer field)")
	}
	// The producer must use the producer client.
	if r.producer.client != producerClient {
		t.Error("expected producer to use producerClient")
	}

	// End-to-end: Register + Append on the producer side, handler consumes it.
	var handlerCalled atomic.Int32
	stream := "test-stream-twoclient"

	r.Register(stream, func(msg storage.Messager) error {
		handlerCalled.Add(1)
		return nil
	})
	r.Run()
	defer r.Shutdown()

	msg := &Message{
		stream: stream,
		values: map[string]interface{}{"route": "two-client"},
	}
	if err := r.Append(msg); err != nil {
		t.Fatalf("Append: %v", err)
	}

	wait(t, 5*time.Second, func() bool {
		return handlerCalled.Load() >= 1
	})
	if got := handlerCalled.Load(); got != 1 {
		t.Errorf("expected exactly 1 handler call, got %d", got)
	}
}

// TestNewRedisWithConsumer_NilGuards verifies both nil-client guards fire.
func TestNewRedisWithConsumer_NilGuards(t *testing.T) {
	mr := miniredis.RunT(t)
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	// nil producer client → error.
	if _, err := NewRedisWithConsumer(nil, client, nil, nil); err == nil {
		t.Error("expected error for nil producer client, got nil")
	}
	// nil consumer client → error.
	if _, err := NewRedisWithConsumer(client, nil, nil, nil); err == nil {
		t.Error("expected error for nil consumer client, got nil")
	}
}

// ---------------------------------------------------------------------------
// C5 Gap 5b: poll() exponential backoff prevents CPU-spin / errCh flood on
// persistent errors (commit 01f46d2).
// ---------------------------------------------------------------------------

func TestPoll_PersistentErrorBackoff(t *testing.T) {
	mr, client := setupMiniredis(t)

	stream := "test-stream-backoff"
	group := "test-group-backoff"

	// Create the consumer (this runs ensureGroup while Redis is healthy).
	cfg := defaultConsumerCfg()
	cfg.BufferSize = 1000 // large errCh so we can count every error

	consumer, err := NewStreamConsumer(client, stream, group, cfg, func(msg storage.Messager) error {
		return nil
	})
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}

	// Now make every Redis command fail persistently, then start the consumer.
	// poll()'s XREADGROUP will error every iteration; without backoff this
	// would spin at CPU speed and flood errCh.
	mr.SetError("LOADING test is loading")

	consumer.Run()

	// Continuously drain errCh so the buffer never fills (we want to count
	// every error the poll loop pushes, not measure buffer capacity).
	var errCount atomic.Int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-consumer.ctx.Done():
				return
			case _, ok := <-consumer.Errors():
				if !ok {
					return
				}
				errCount.Add(1)
			}
		}
	}()

	// Observe over a 2-second window.
	//
	// Backoff schedule (per poll failure): 100ms -> 200ms -> 400ms -> 800ms ->
	// 1600ms -> 3200ms ... With each failed attempt also blocked by
	// BlockingTimeout (cfg.BlockingTimeout=1s) PLUS the growing backoff, the
	// number of poll errors over 2s is small (well under 10). Without backoff
	// the loop would emit hundreds-to-thousands of errors in the same window.
	time.Sleep(2 * time.Second)

	// Clear the error so Shutdown can complete cleanly.
	mr.SetError("")

	consumer.Shutdown()
	<-done

	count := errCount.Load()
	t.Logf("poll error count over 2s window: %d", count)
	// Bound: 2s / ~100ms minimum backoff ~= 20 worst-case attempts; real
	// number is far lower because backoff grows. < 30 proves backoff is
	// active (a tight spin would be hundreds+).
	if count >= 30 {
		t.Errorf("expected bounded poll error count (< 30) proving backoff, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// F1: Poison message — handler always fails, MaxDeliveries discards after N retries
// ---------------------------------------------------------------------------

func TestConsumer_PoisonMessageDiscarded(t *testing.T) {
	mr, client := setupMiniredis(t)

	stream := "test-stream-poison"
	group := "test-group-poison"

	// Step 1: Create the stream and group, add a message.
	err := client.XGroupCreateMkStream(context.Background(), stream, group, "0").Err()
	if err != nil {
		t.Fatalf("XGroupCreateMkStream: %v", err)
	}

	msgID, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"poison": "test"},
		ID:     "*",
	}).Result()
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}

	// Step 2: Simulate prior reclaim attempts using XCLAIM.
	// Each XCLAIM transfers ownership and increments the delivery counter
	// (RetryCount in XPENDING). We need RetryCount > MaxDeliveries for the
	// poison check to trigger.
	const maxDeliveries int64 = 3

	// First, a phantom consumer reads the message so it becomes pending.
	_, err = client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "phantom-seed",
		Streams:  []string{stream, ">"},
		Count:    1,
	}).Result()
	if err != nil {
		t.Fatalf("phantom seed XReadGroup: %v", err)
	}

	// Now XCLAIM the message multiple times to bump RetryCount past MaxDeliveries.
	for i := 0; i < int(maxDeliveries); i++ {
		_, err := client.XClaim(context.Background(), &redis.XClaimArgs{
			Stream:   stream,
			Group:    group,
			Consumer: fmt.Sprintf("phantom-claim-%d", i),
			Messages: []string{msgID},
			MinIdle:  0, // claim immediately regardless of idle time
		}).Result()
		if err != nil {
			t.Fatalf("phantom XClaim %d: %v", i, err)
		}
	}

	// Verify the message's retry count exceeds MaxDeliveries.
	pending, err := client.XPendingExt(context.Background(), &redis.XPendingExtArgs{
		Stream: stream,
		Group:  group,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err != nil {
		t.Fatalf("XPendingExt: %v", err)
	}
	if len(pending) == 0 {
		t.Fatal("expected pending message, got none")
	}
	t.Logf("message %s retryCount=%d (maxDeliveries=%d)", msgID, pending[0].RetryCount, maxDeliveries)

	// Step 3: Advance miniredis time so the message exceeds visibility timeout.
	mr.FastForward(3 * time.Second)

	// Step 4: Start the consumer — reclaim should detect the poison message
	// and discard it (ACK without redelivery).
	var handlerCalled atomic.Int32

	handler := func(msg storage.Messager) error {
		handlerCalled.Add(1)
		return errors.New("always fails")
	}

	cfg := defaultConsumerCfg()
	cfg.VisibilityTimeout = 1
	cfg.ReclaimInterval = 1
	cfg.BlockingTimeout = 1
	cfg.MaxDeliveries = maxDeliveries

	consumer, err := NewStreamConsumer(client, stream, group, cfg, handler)
	if err != nil {
		t.Fatalf("NewStreamConsumer: %v", err)
	}
	consumer.Run()
	defer consumer.Shutdown()

	// Step 5: Wait for pending to clear (poison ACK).
	wait(t, 10*time.Second, func() bool {
		p, err := client.XPending(context.Background(), stream, group).Result()
		if err != nil {
			return false
		}
		return p.Count == 0
	})

	// The handler should NOT have been called — the message was discarded
	// by reclaim before being re-delivered.
	count := handlerCalled.Load()
	t.Logf("handler called %d times", count)
	if count > 0 {
		t.Errorf("expected 0 handler calls (poison discarded before delivery), got %d", count)
	}

	// Verify the poison error was pushed to the error channel.
	var foundPoison bool
	timeout := time.After(3 * time.Second)
	for !foundPoison {
		select {
		case e := <-consumer.Errors():
			if e != nil && strings.Contains(e.Error(), "poison message discarded") {
				foundPoison = true
				t.Logf("poison error: %v", e)
			}
		case <-timeout:
			goto checkPoison
		}
	}
checkPoison:
	if !foundPoison {
		t.Error("expected 'poison message discarded' error on errCh")
	}

	// Advance time again and verify no more handler calls
	// (proving the infinite retry loop is broken).
	countBefore := handlerCalled.Load()
	mr.FastForward(3 * time.Second)
	time.Sleep(500 * time.Millisecond)

	finalCount := handlerCalled.Load()
	if finalCount != countBefore {
		t.Errorf("handler called again after poison discard: was %d, now %d — retry loop not stopped",
			countBefore, finalCount)
	}
}
