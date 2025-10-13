package function_tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// TestKafkaEnvelopePublishSubscribe 测试 Kafka Envelope 发布订阅
func TestKafkaEnvelopePublishSubscribe(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.envelope.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-envelope-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	var received int64
	var lastEnvelope *eventbus.Envelope
	ctx := context.Background()

	// 订阅 Envelope
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		atomic.AddInt64(&received, 1)
		lastEnvelope = envelope
		t.Logf("📨 Received envelope: AggregateID=%s, EventType=%s, Version=%d",
			envelope.AggregateID, envelope.EventType, envelope.EventVersion)
		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(2 * time.Second)

	// 发布 Envelope
	envelope := &eventbus.Envelope{
		AggregateID:  "test-aggregate-1",
		EventType:    "TestEvent",
		EventVersion: 1,
		Timestamp:    time.Now(),
		Payload:      []byte("Test payload"),
	}

	err = bus.PublishEnvelope(ctx, topic, envelope)
	helper.AssertNoError(err, "PublishEnvelope should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 10*time.Second)
	helper.AssertTrue(success, "Should receive envelope within timeout")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&received), "Should receive exactly 1 envelope")
	helper.AssertEqual(envelope.AggregateID, lastEnvelope.AggregateID, "AggregateID should match")
	helper.AssertEqual(envelope.EventType, lastEnvelope.EventType, "EventType should match")
	helper.AssertEqual(envelope.EventVersion, lastEnvelope.EventVersion, "EventVersion should match")

	t.Logf("✅ Kafka Envelope publish/subscribe test passed")
}

// TestNATSEnvelopePublishSubscribe 测试 NATS Envelope 发布订阅
func TestNATSEnvelopePublishSubscribe(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.envelope.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-envelope-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_ENVELOPE_%d", helper.GetTimestamp()/1000))

	var received int64
	var lastEnvelope *eventbus.Envelope
	ctx := context.Background()

	// 订阅 Envelope
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		atomic.AddInt64(&received, 1)
		lastEnvelope = envelope
		t.Logf("📨 Received envelope: AggregateID=%s, EventType=%s, Version=%d",
			envelope.AggregateID, envelope.EventType, envelope.EventVersion)
		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(2 * time.Second)

	// 发布 Envelope
	envelope := &eventbus.Envelope{
		AggregateID:  "test-aggregate-1",
		EventType:    "TestEvent",
		EventVersion: 1,
		Timestamp:    time.Now(),
		Payload:      []byte("Test payload"),
	}

	err = bus.PublishEnvelope(ctx, topic, envelope)
	helper.AssertNoError(err, "PublishEnvelope should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 10*time.Second)
	helper.AssertTrue(success, "Should receive envelope within timeout")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&received), "Should receive exactly 1 envelope")
	helper.AssertEqual(envelope.AggregateID, lastEnvelope.AggregateID, "AggregateID should match")
	helper.AssertEqual(envelope.EventType, lastEnvelope.EventType, "EventType should match")
	helper.AssertEqual(envelope.EventVersion, lastEnvelope.EventVersion, "EventVersion should match")

	t.Logf("✅ NATS Envelope publish/subscribe test passed")
}

// TestKafkaEnvelopeOrdering 测试 Kafka Envelope 顺序保证
func TestKafkaEnvelopeOrdering(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.envelope.order.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-envelope-order-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	aggregateID := "test-aggregate-order"
	var receivedVersions []int64
	var mu sync.Mutex
	ctx := context.Background()

	// 订阅 Envelope
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		if envelope.AggregateID == aggregateID {
			mu.Lock()
			receivedVersions = append(receivedVersions, envelope.EventVersion)
			mu.Unlock()
		}
		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(2 * time.Second)

	// 发布多个版本的 Envelope
	messageCount := 10
	for i := 1; i <= messageCount; i++ {
		envelope := &eventbus.Envelope{
			AggregateID:  aggregateID,
			EventType:    "TestEvent",
			EventVersion: int64(i),
			Timestamp:    time.Now(),
			Payload:      []byte(fmt.Sprintf("Payload %d", i)),
		}
		err = bus.PublishEnvelope(ctx, topic, envelope)
		helper.AssertNoError(err, "PublishEnvelope should not return error")
	}

	// 等待所有消息接收
	success := helper.WaitForCondition(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedVersions) == messageCount
	}, 15*time.Second, "waiting for all envelopes")
	helper.AssertTrue(success, "Should receive all envelopes within timeout")

	// 验证顺序
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < len(receivedVersions); i++ {
		expected := int64(i + 1)
		if receivedVersions[i] != expected {
			t.Errorf("Order violation: expected version %d, got %d at position %d",
				expected, receivedVersions[i], i)
		}
	}

	t.Logf("✅ Kafka Envelope ordering test passed")
}

// TestNATSEnvelopeOrdering 测试 NATS Envelope 顺序保证
func TestNATSEnvelopeOrdering(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.envelope.order.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-envelope-order-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_ENVELOPE_ORDER_%d", helper.GetTimestamp()/1000))

	aggregateID := "test-aggregate-order"
	var receivedVersions []int64
	var mu sync.Mutex
	ctx := context.Background()

	// 订阅 Envelope
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		if envelope.AggregateID == aggregateID {
			mu.Lock()
			receivedVersions = append(receivedVersions, envelope.EventVersion)
			mu.Unlock()
		}
		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(2 * time.Second)

	// 发布多个版本的 Envelope
	messageCount := 10
	for i := 1; i <= messageCount; i++ {
		envelope := &eventbus.Envelope{
			AggregateID:  aggregateID,
			EventType:    "TestEvent",
			EventVersion: int64(i),
			Timestamp:    time.Now(),
			Payload:      []byte(fmt.Sprintf("Payload %d", i)),
		}
		err = bus.PublishEnvelope(ctx, topic, envelope)
		helper.AssertNoError(err, "PublishEnvelope should not return error")
	}

	// 等待所有消息接收
	success := helper.WaitForCondition(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(receivedVersions) == messageCount
	}, 15*time.Second, "waiting for all envelopes")
	helper.AssertTrue(success, "Should receive all envelopes within timeout")

	// 验证顺序
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < len(receivedVersions); i++ {
		expected := int64(i + 1)
		if receivedVersions[i] != expected {
			t.Errorf("Order violation: expected version %d, got %d at position %d",
				expected, receivedVersions[i], i)
		}
	}

	t.Logf("✅ NATS Envelope ordering test passed")
}

// TestKafkaMultipleAggregates 测试 Kafka 多聚合并发处理
func TestKafkaMultipleAggregates(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.multi.agg.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-multi-agg-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	var received int64
	ctx := context.Background()

	// 订阅 Envelope
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(2 * time.Second)

	// 发布多个聚合的消息
	aggregateCount := 5
	messagesPerAggregate := 10
	totalMessages := aggregateCount * messagesPerAggregate

	for aggID := 1; aggID <= aggregateCount; aggID++ {
		for version := 1; version <= messagesPerAggregate; version++ {
			envelope := &eventbus.Envelope{
				AggregateID:  fmt.Sprintf("aggregate-%d", aggID),
				EventType:    "TestEvent",
				EventVersion: int64(version),
				Timestamp:    time.Now(),
				Payload:      []byte(fmt.Sprintf("Aggregate %d, Version %d", aggID, version)),
			}
			err = bus.PublishEnvelope(ctx, topic, envelope)
			helper.AssertNoError(err, "PublishEnvelope should not return error")
		}
	}

	// 等待所有消息接收
	success := helper.WaitForMessages(&received, int64(totalMessages), 20*time.Second)
	helper.AssertTrue(success, "Should receive all messages within timeout")
	helper.AssertEqual(int64(totalMessages), atomic.LoadInt64(&received), "Should receive all messages")

	t.Logf("✅ Kafka multiple aggregates test passed")
}

