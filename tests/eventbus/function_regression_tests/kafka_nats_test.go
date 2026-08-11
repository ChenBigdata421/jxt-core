//go:build integration
// +build integration

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

// ============================================================================
// 基础发布订阅测试 (来自 basic_test.go)
// ============================================================================

// TestKafkaBasicPublishSubscribe 测试Kafka基本的发布订阅功能
func TestKafkaBasicPublishSubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka basic test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.basic.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-basic-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 订阅消息
	var received int64
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		t.Logf("📨 Received message: %s", string(message))
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	// 等待订阅建立
	time.Sleep(2 * time.Second)

	// 发布消息
	err = bus.Publish(ctx, topic, []byte("Hello Kafka!"))
	helper.AssertNoError(err, "Publish should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 5*time.Second)
	helper.AssertTrue(success, "Should receive message within timeout")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&received), "Should receive exactly 1 message")

	t.Logf("✅ Kafka basic publish/subscribe test passed")
}

// TestNATSBasicPublishSubscribe 测试NATS基本的发布订阅功能
func TestNATSBasicPublishSubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS basic test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-basic-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	// 🔧 修复：topic 必须匹配 Stream subjects 模式 (clientID.>)
	topic := fmt.Sprintf("%s.test", clientID)

	// 订阅消息
	var received int64
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		t.Logf("📨 Received message: %s", string(message))
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	// 等待订阅建立
	time.Sleep(2 * time.Second)

	// 发布消息
	err = bus.Publish(ctx, topic, []byte("Hello NATS!"))
	helper.AssertNoError(err, "Publish should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 5*time.Second)
	helper.AssertTrue(success, "Should receive message within timeout")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&received), "Should receive exactly 1 message")

	t.Logf("✅ NATS basic publish/subscribe test passed")
}

// TestKafkaMultipleMessages 测试Kafka发送多条消息
func TestKafkaMultipleMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka multiple messages test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.multiple.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-multiple-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	messageCount := 10
	var receivedCount int64

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&receivedCount, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(2 * time.Second)

	for i := 0; i < messageCount; i++ {
		message := []byte(fmt.Sprintf("Message %d", i+1))
		err = bus.Publish(ctx, topic, message)
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待所有消息接收
	success := helper.WaitForMessages(&receivedCount, int64(messageCount), 5*time.Second)
	helper.AssertTrue(success, "Should receive all messages within timeout")
	helper.AssertEqual(int64(messageCount), atomic.LoadInt64(&receivedCount), "Should receive all messages")

	t.Logf("✅ Kafka multiple messages test passed")
}

// TestNATSMultipleMessages 测试NATS发送多条消息
func TestNATSMultipleMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS multiple messages test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-multiple-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	// 🔧 修复：topic 必须匹配 Stream subjects 模式 (clientID.>)
	topic := fmt.Sprintf("%s.test", clientID)

	messageCount := 10
	var receivedCount int64

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&receivedCount, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(2 * time.Second)

	for i := 0; i < messageCount; i++ {
		message := []byte(fmt.Sprintf("Message %d", i+1))
		err = bus.Publish(ctx, topic, message)
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待所有消息接收
	success := helper.WaitForMessages(&receivedCount, int64(messageCount), 5*time.Second)
	helper.AssertTrue(success, "Should receive all messages within timeout")
	helper.AssertEqual(int64(messageCount), atomic.LoadInt64(&receivedCount), "Should receive all messages")

	t.Logf("✅ NATS multiple messages test passed")
}

// TestKafkaPublishWithOptions 测试Kafka带选项的发布
func TestKafkaPublishWithOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka publish with options test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.options.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-options-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	var received int64
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(2 * time.Second)

	opts := eventbus.PublishOptions{
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		Timeout: 5 * time.Second,
	}

	err = bus.PublishWithOptions(ctx, topic, []byte("Test message"), opts)
	helper.AssertNoError(err, "PublishWithOptions should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 5*time.Second)
	helper.AssertTrue(success, "Should receive message within timeout")

	t.Logf("✅ Kafka PublishWithOptions test passed")
}

// TestNATSPublishWithOptions 测试NATS带选项的发布
func TestNATSPublishWithOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS publish with options test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-options-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	// 🔧 修复：topic 必须匹配 Stream subjects 模式 (clientID.>)
	topic := fmt.Sprintf("%s.test", clientID)

	var received int64
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(2 * time.Second)

	opts := eventbus.PublishOptions{
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		Timeout: 5 * time.Second,
	}

	err = bus.PublishWithOptions(ctx, topic, []byte("Test message"), opts)
	helper.AssertNoError(err, "PublishWithOptions should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 5*time.Second)
	helper.AssertTrue(success, "Should receive message within timeout")

	t.Logf("✅ NATS PublishWithOptions test passed")
}

// ============================================================================
// Envelope 测试 (来自 envelope_test.go)
// ============================================================================

// TestKafkaEnvelopePublishSubscribe 测试 Kafka Envelope 发布订阅
func TestKafkaEnvelopePublishSubscribe(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.envelope.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-envelope-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// ✅ 关键修复：设置预订阅 topics（Kafka EventBus 预订阅模式要求）
	if kafkaBus, ok := bus.(interface {
		SetPreSubscriptionTopics([]string)
	}); ok {
		kafkaBus.SetPreSubscriptionTopics([]string{topic})
		t.Logf("✅ Set pre-subscription topics: %s", topic)
	}

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
	// ✅ 修复：Payload 必须是有效的 JSON（RawMessage 要求）
	envelope := &eventbus.Envelope{
		EventID:      "evt-kafka-envelope-001",
		AggregateID:  "test-aggregate-1",
		EventType:    "TestEvent",
		EventVersion: 1,
		Timestamp:    time.Now(),
		Payload:      []byte(`{"message":"Test payload"}`), // 有效的 JSON
	}

	err = bus.PublishEnvelope(ctx, topic, envelope)
	helper.AssertNoError(err, "PublishEnvelope should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 10*time.Second)
	helper.AssertTrue(success, "Should receive envelope within timeout")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&received), "Should receive exactly 1 envelope")

	// 检查 lastEnvelope 不为 nil
	if lastEnvelope != nil {
		helper.AssertEqual(envelope.AggregateID, lastEnvelope.AggregateID, "AggregateID should match")
		helper.AssertEqual(envelope.EventType, lastEnvelope.EventType, "EventType should match")
		helper.AssertEqual(envelope.EventVersion, lastEnvelope.EventVersion, "EventVersion should match")
	} else {
		t.Errorf("❌ lastEnvelope is nil, no envelope was received")
	}

	t.Logf("✅ Kafka Envelope publish/subscribe test passed")
}

// TestNATSEnvelopePublishSubscribe 测试 NATS Envelope 发布订阅
func TestNATSEnvelopePublishSubscribe(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-envelope-%d", helper.GetTimestamp())
	// 🔧 修复：topic 必须匹配 Stream subjects 模式
	// Stream subjects 是 "nats-envelope-*.>"，所以 topic 必须以 clientID 开头
	topic := fmt.Sprintf("%s.envelope.%d", clientID, helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_STREAM_%s", clientID))

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
	// ✅ 修复：Payload 必须是有效的 JSON（RawMessage 要求）
	envelope := &eventbus.Envelope{
		EventID:      "evt-nats-envelope-001",
		AggregateID:  "test-aggregate-1",
		EventType:    "TestEvent",
		EventVersion: 1,
		Timestamp:    time.Now(),
		Payload:      []byte(`{"message":"Test payload"}`), // 有效的 JSON
	}

	err = bus.PublishEnvelope(ctx, topic, envelope)
	helper.AssertNoError(err, "PublishEnvelope should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 10*time.Second)
	helper.AssertTrue(success, "Should receive envelope within timeout")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&received), "Should receive exactly 1 envelope")

	// 检查 lastEnvelope 是否为 nil，避免 panic
	if lastEnvelope != nil {
		helper.AssertEqual(envelope.AggregateID, lastEnvelope.AggregateID, "AggregateID should match")
		helper.AssertEqual(envelope.EventType, lastEnvelope.EventType, "EventType should match")
		helper.AssertEqual(envelope.EventVersion, lastEnvelope.EventVersion, "EventVersion should match")
	} else {
		t.Errorf("lastEnvelope is nil, no message was received")
	}

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

	// ✅ 关键修复：设置预订阅 topics（Kafka EventBus 预订阅模式要求）
	if kafkaBus, ok := bus.(interface {
		SetPreSubscriptionTopics([]string)
	}); ok {
		kafkaBus.SetPreSubscriptionTopics([]string{topic})
		t.Logf("✅ Set pre-subscription topics: %s", topic)
	}

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
		// ✅ 修复：Payload 必须是有效的 JSON（RawMessage 要求）
		envelope := &eventbus.Envelope{
			EventID:      fmt.Sprintf("evt-kafka-multi-%03d", i),
			AggregateID:  aggregateID,
			EventType:    "TestEvent",
			EventVersion: int64(i),
			Timestamp:    time.Now(),
			Payload:      []byte(fmt.Sprintf(`{"message":"Payload %d"}`, i)), // 有效的 JSON
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

	clientID := fmt.Sprintf("nats-envelope-order-%d", helper.GetTimestamp())
	// 🔧 修复：topic 必须匹配 Stream subjects 模式
	topic := fmt.Sprintf("%s.envelope.order.%d", clientID, helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_STREAM_%s", clientID))

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
		// ✅ 修复：Payload 必须是有效的 JSON（RawMessage 要求）
		envelope := &eventbus.Envelope{
			EventID:      fmt.Sprintf("evt-nats-multi-%03d", i),
			AggregateID:  aggregateID,
			EventType:    "TestEvent",
			EventVersion: int64(i),
			Timestamp:    time.Now(),
			Payload:      []byte(fmt.Sprintf(`{"message":"Payload %d"}`, i)), // 有效的 JSON
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

	// ✅ 关键修复：设置预订阅 topics（Kafka EventBus 预订阅模式要求）
	if kafkaBus, ok := bus.(interface {
		SetPreSubscriptionTopics([]string)
	}); ok {
		kafkaBus.SetPreSubscriptionTopics([]string{topic})
		t.Logf("✅ Set pre-subscription topics: %s", topic)
	}

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
			// ✅ 修复：Payload 必须是有效的 JSON（RawMessage 要求）
			envelope := &eventbus.Envelope{
				EventID:      fmt.Sprintf("evt-kafka-agg-%d-v%d", aggID, version),
				AggregateID:  fmt.Sprintf("aggregate-%d", aggID),
				EventType:    "TestEvent",
				EventVersion: int64(version),
				Timestamp:    time.Now(),
				Payload:      []byte(fmt.Sprintf(`{"aggregate_id":%d,"version":%d}`, aggID, version)),
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

// TestNATSMultipleAggregates 测试 NATS 多聚合并发处理（Hollywood Actor Pool）
func TestNATSMultipleAggregates(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-multi-agg-%d", helper.GetTimestamp())
	topic := fmt.Sprintf("%s.test", clientID)

	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	var received int64
	aggregateVersions := make(map[string][]int64)
	var mu sync.Mutex
	ctx := context.Background()

	// 订阅 Envelope
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		atomic.AddInt64(&received, 1)
		mu.Lock()
		aggregateVersions[envelope.AggregateID] = append(aggregateVersions[envelope.AggregateID], envelope.EventVersion)
		mu.Unlock()
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
			// ✅ 修复：Payload 必须是有效的 JSON（RawMessage 要求）
			envelope := &eventbus.Envelope{
				EventID:      fmt.Sprintf("evt-nats-agg-%d-v%d", aggID, version),
				AggregateID:  fmt.Sprintf("aggregate-%d", aggID),
				EventType:    "TestEvent",
				EventVersion: int64(version),
				Timestamp:    time.Now(),
				Payload:      []byte(fmt.Sprintf(`{"aggregate_id":%d,"version":%d}`, aggID, version)),
			}
			err = bus.PublishEnvelope(ctx, topic, envelope)
			helper.AssertNoError(err, "PublishEnvelope should not return error")
		}
	}

	// 等待所有消息接收
	success := helper.WaitForMessages(&received, int64(totalMessages), 20*time.Second)
	helper.AssertTrue(success, "Should receive all messages within timeout")
	helper.AssertEqual(int64(totalMessages), atomic.LoadInt64(&received), "Should receive all messages")

	// 验证每个聚合的顺序
	mu.Lock()
	for aggID := 1; aggID <= aggregateCount; aggID++ {
		aggregateIDStr := fmt.Sprintf("aggregate-%d", aggID)
		versions := aggregateVersions[aggregateIDStr]
		helper.AssertEqual(messagesPerAggregate, len(versions), fmt.Sprintf("Aggregate %s should have %d messages", aggregateIDStr, messagesPerAggregate))

		// 验证顺序
		for i := 0; i < len(versions); i++ {
			expected := int64(i + 1)
			actual := versions[i]
			helper.AssertEqual(expected, actual, fmt.Sprintf("Aggregate %s: order violation at index %d", aggregateIDStr, i))
		}
	}
	mu.Unlock()

	t.Logf("✅ NATS multiple aggregates test passed (Hollywood Actor Pool)")
}

// ============================================================================
// 生命周期测试 (来自 lifecycle_test.go)
// ============================================================================

// TestKafkaClose 测试 Kafka Close
func TestKafkaClose(t *testing.T) {
	helper := NewTestHelper(t)
	// 注意：不使用 defer helper.Cleanup()，因为我们要手动测试 Close()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-close-%d", helper.GetTimestamp()))

	// 关闭 EventBus
	err := bus.Close()
	helper.AssertNoError(err, "Close should not return error")

	// 验证关闭后无法发布消息
	ctx := context.Background()
	topic := fmt.Sprintf("test.kafka.close.%d", helper.GetTimestamp())
	err = bus.Publish(ctx, topic, []byte("test"))
	helper.AssertTrue(err != nil, "Publish should return error after Close")

	// 验证重复关闭是幂等的（不应该报错）
	err = bus.Close()
	helper.AssertNoError(err, "Second Close should not return error (idempotent)")

	t.Logf("✅ Kafka Close test passed")
}

// TestNATSClose 测试 NATS Close
func TestNATSClose(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-close-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)

	// 关闭 EventBus
	err := bus.Close()
	helper.AssertNoError(err, "Close should not return error")

	// 验证关闭后无法发布消息
	ctx := context.Background()
	topic := fmt.Sprintf("test.nats.close.%d", helper.GetTimestamp())
	err = bus.Publish(ctx, topic, []byte("test"))
	helper.AssertTrue(err != nil, "Publish should return error after Close")

	t.Logf("✅ NATS Close test passed")
}

// TestKafkaPublishCallback 测试 Kafka 发布回调
func TestKafkaPublishCallback(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.pub.callback.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-pub-callback-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// 注册发布回调
	callbackCalled := false
	err := bus.RegisterPublishCallback(func(ctx context.Context, topic string, message []byte, err error) error {
		callbackCalled = true
		t.Logf("📞 Publish callback called: Topic=%s, Error=%v", topic, err)
		return nil
	})
	helper.AssertNoError(err, "RegisterPublishCallback should not return error")

	ctx := context.Background()

	// 发布消息
	err = bus.Publish(ctx, topic, []byte("test message"))
	helper.AssertNoError(err, "Publish should not return error")

	// 等待回调
	time.Sleep(2 * time.Second)

	t.Logf("✅ Kafka RegisterPublishCallback test passed (callback called: %v)", callbackCalled)
}

// ============================================================================
// 主题配置测试 (来自 topic_config_test.go)
// ============================================================================

// TestKafkaTopicConfiguration 测试 Kafka 主题配置
func TestKafkaTopicConfiguration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.topic.config.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-topic-config-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 配置主题
	options := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		Replicas:        1,
		RetentionTime:   24 * time.Hour,
		MaxMessages:     10000,
	}

	err := bus.ConfigureTopic(ctx, topic, options)
	helper.AssertNoError(err, "ConfigureTopic should not return error")

	// 获取主题配置
	config, err := bus.GetTopicConfig(topic)
	helper.AssertNoError(err, "GetTopicConfig should not return error")
	helper.AssertEqual(eventbus.TopicPersistent, config.PersistenceMode, "PersistenceMode should match")

	// 列出已配置的主题
	topics := bus.ListConfiguredTopics()
	found := false
	for _, t := range topics {
		if t == topic {
			found = true
			break
		}
	}
	helper.AssertTrue(found, "Topic should be in configured topics list")

	t.Logf("✅ Kafka topic configuration test passed")
}

// TestNATSTopicConfiguration 测试 NATS 主题配置
func TestNATSTopicConfiguration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.topic.config.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-topic-config-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_TOPIC_CONFIG_%d", helper.GetTimestamp()/1000))

	ctx := context.Background()

	// 配置主题
	options := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		Replicas:        1,
		RetentionTime:   24 * time.Hour,
		MaxMessages:     10000,
	}

	err := bus.ConfigureTopic(ctx, topic, options)
	helper.AssertNoError(err, "ConfigureTopic should not return error")

	// 获取主题配置
	config, err := bus.GetTopicConfig(topic)
	helper.AssertNoError(err, "GetTopicConfig should not return error")
	helper.AssertEqual(eventbus.TopicPersistent, config.PersistenceMode, "PersistenceMode should match")

	// 列出已配置的主题
	topics := bus.ListConfiguredTopics()
	found := false
	for _, t := range topics {
		if t == topic {
			found = true
			break
		}
	}
	helper.AssertTrue(found, "Topic should be in configured topics list")

	t.Logf("✅ NATS topic configuration test passed")
}

// TestKafkaSetTopicPersistence 测试 Kafka 设置主题持久化
func TestKafkaSetTopicPersistence(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.persistence.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-persistence-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 设置持久化
	err := bus.SetTopicPersistence(ctx, topic, true)
	helper.AssertNoError(err, "SetTopicPersistence should not return error")

	// 验证配置
	config, err := bus.GetTopicConfig(topic)
	helper.AssertNoError(err, "GetTopicConfig should not return error")
	helper.AssertEqual(eventbus.TopicPersistent, config.PersistenceMode, "Should be persistent")

	t.Logf("✅ Kafka SetTopicPersistence test passed")
}

// TestNATSSetTopicPersistence 测试 NATS 设置主题持久化
func TestNATSSetTopicPersistence(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.persistence.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-persistence-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_PERSISTENCE_%d", helper.GetTimestamp()/1000))

	ctx := context.Background()

	// 设置持久化
	err := bus.SetTopicPersistence(ctx, topic, true)
	helper.AssertNoError(err, "SetTopicPersistence should not return error")

	// 验证配置
	config, err := bus.GetTopicConfig(topic)
	helper.AssertNoError(err, "GetTopicConfig should not return error")
	helper.AssertEqual(eventbus.TopicPersistent, config.PersistenceMode, "Should be persistent")

	t.Logf("✅ NATS SetTopicPersistence test passed")
}

// TestKafkaRemoveTopicConfig 测试 Kafka 移除主题配置
func TestKafkaRemoveTopicConfig(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.remove.config.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-remove-config-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 配置主题
	options := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
	}
	err := bus.ConfigureTopic(ctx, topic, options)
	helper.AssertNoError(err, "ConfigureTopic should not return error")

	// 移除配置
	err = bus.RemoveTopicConfig(topic)
	helper.AssertNoError(err, "RemoveTopicConfig should not return error")

	// 验证配置已移除
	topics := bus.ListConfiguredTopics()
	found := false
	for _, t := range topics {
		if t == topic {
			found = true
			break
		}
	}
	helper.AssertTrue(!found, "Topic should not be in configured topics list after removal")

	t.Logf("✅ Kafka RemoveTopicConfig test passed")
}

// TestNATSRemoveTopicConfig 测试 NATS 移除主题配置
func TestNATSRemoveTopicConfig(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.remove.config.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-remove-config-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_REMOVE_CONFIG_%d", helper.GetTimestamp()/1000))

	ctx := context.Background()

	// 配置主题
	options := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
	}
	err := bus.ConfigureTopic(ctx, topic, options)
	helper.AssertNoError(err, "ConfigureTopic should not return error")

	// 移除配置
	err = bus.RemoveTopicConfig(topic)
	helper.AssertNoError(err, "RemoveTopicConfig should not return error")

	// 验证配置已移除
	topics := bus.ListConfiguredTopics()
	found := false
	for _, t := range topics {
		if t == topic {
			found = true
			break
		}
	}
	helper.AssertTrue(!found, "Topic should not be in configured topics list after removal")

	t.Logf("✅ NATS RemoveTopicConfig test passed")
}

// ============================================================================
// 启动时配置主题测试（新增）
// ============================================================================

// TestKafkaStartupTopicConfiguration 测试 Kafka 启动时配置多个主题（最佳实践）
func TestKafkaStartupTopicConfiguration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	timestamp := helper.GetTimestamp()

	// 定义所有主题配置（模拟应用启动时的配置）
	topicConfigs := map[string]eventbus.TopicOptions{
		fmt.Sprintf("business.orders.%d", timestamp): {
			PersistenceMode: eventbus.TopicPersistent,
			RetentionTime:   7 * 24 * time.Hour,
			MaxMessages:     10000,
			Replicas:        1,
			Description:     "订单事件，需要持久化",
		},
		fmt.Sprintf("system.notifications.%d", timestamp): {
			PersistenceMode: eventbus.TopicEphemeral,
			RetentionTime:   1 * time.Hour,
			MaxMessages:     1000,
			Replicas:        1,
			Description:     "系统通知，临时消息",
		},
		fmt.Sprintf("audit.logs.%d", timestamp): {
			PersistenceMode: eventbus.TopicPersistent,
			RetentionTime:   90 * 24 * time.Hour,
			MaxMessages:     100000,
			Replicas:        1,
			Description:     "审计日志，长期保留",
		},
	}

	// 创建所有主题
	topics := make([]string, 0, len(topicConfigs))
	for topic := range topicConfigs {
		topics = append(topics, topic)
	}
	helper.CreateKafkaTopics(topics, 3)

	// 创建 EventBus
	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-startup-%d", timestamp))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 模拟应用启动时配置所有主题（最佳实践）
	for topic, options := range topicConfigs {
		err := bus.ConfigureTopic(ctx, topic, options)
		helper.AssertNoError(err, fmt.Sprintf("ConfigureTopic for %s should not return error", topic))
		t.Logf("✅ Configured topic: %s", topic)
	}

	// 验证所有主题都已配置
	configuredTopics := bus.ListConfiguredTopics()
	helper.AssertEqual(len(topicConfigs), len(configuredTopics), "Should have configured all topics")

	// 验证每个主题的配置
	for topic, expectedOptions := range topicConfigs {
		config, err := bus.GetTopicConfig(topic)
		helper.AssertNoError(err, fmt.Sprintf("GetTopicConfig for %s should not return error", topic))
		helper.AssertEqual(expectedOptions.PersistenceMode, config.PersistenceMode,
			fmt.Sprintf("PersistenceMode for %s should match", topic))
		helper.AssertEqual(expectedOptions.Description, config.Description,
			fmt.Sprintf("Description for %s should match", topic))
	}

	t.Logf("✅ Kafka startup topic configuration test passed (configured %d topics)", len(topicConfigs))
}

// TestKafkaIdempotentTopicConfiguration 测试 Kafka 幂等配置（可以多次调用）
func TestKafkaIdempotentTopicConfiguration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.idempotent.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-idempotent-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 第一次配置
	options1 := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   24 * time.Hour,
		MaxMessages:     10000,
		Description:     "First configuration",
	}
	err := bus.ConfigureTopic(ctx, topic, options1)
	helper.AssertNoError(err, "First ConfigureTopic should not return error")

	// 第二次配置（幂等操作）
	options2 := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   48 * time.Hour, // 修改保留时间
		MaxMessages:     20000,          // 修改最大消息数
		Description:     "Second configuration",
	}
	err = bus.ConfigureTopic(ctx, topic, options2)
	helper.AssertNoError(err, "Second ConfigureTopic should not return error (idempotent)")

	// 验证配置已更新
	config, err := bus.GetTopicConfig(topic)
	helper.AssertNoError(err, "GetTopicConfig should not return error")
	helper.AssertEqual(options2.RetentionTime, config.RetentionTime, "RetentionTime should be updated")
	helper.AssertEqual(options2.Description, config.Description, "Description should be updated")

	t.Logf("✅ Kafka idempotent topic configuration test passed")
}

// TestNATSStartupTopicConfiguration 测试 NATS 启动时配置多个主题
func TestNATSStartupTopicConfiguration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	timestamp := helper.GetTimestamp()
	clientID := fmt.Sprintf("nats-startup-%d", timestamp)
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 定义所有主题配置
	topicConfigs := map[string]eventbus.TopicOptions{
		fmt.Sprintf("business.orders.%d", timestamp): {
			PersistenceMode: eventbus.TopicPersistent,
			RetentionTime:   7 * 24 * time.Hour,
			MaxMessages:     10000,
			Description:     "订单事件，使用 JetStream",
		},
		fmt.Sprintf("system.notifications.%d", timestamp): {
			PersistenceMode: eventbus.TopicEphemeral,
			RetentionTime:   1 * time.Hour,
			Description:     "系统通知，使用 Core NATS",
		},
	}

	// 配置所有主题
	for topic, options := range topicConfigs {
		err := bus.ConfigureTopic(ctx, topic, options)
		helper.AssertNoError(err, fmt.Sprintf("ConfigureTopic for %s should not return error", topic))
		t.Logf("✅ Configured topic: %s (mode: %s)", topic, options.PersistenceMode)
	}

	// 验证配置
	configuredTopics := bus.ListConfiguredTopics()
	helper.AssertEqual(len(topicConfigs), len(configuredTopics), "Should have configured all topics")

	t.Logf("✅ NATS startup topic configuration test passed (configured %d topics)", len(topicConfigs))
}

// TestKafkaTopicConfigStrategy 测试 Kafka 主题配置策略
func TestKafkaTopicConfigStrategy(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-config-strategy-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// 设置策略
	bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)

	// 获取策略
	strategy := bus.GetTopicConfigStrategy()
	helper.AssertEqual(eventbus.StrategyCreateOrUpdate, strategy, "Strategy should match")

	// 设置其他策略
	bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
	strategy = bus.GetTopicConfigStrategy()
	helper.AssertEqual(eventbus.StrategyValidateOnly, strategy, "Strategy should match")

	t.Logf("✅ Kafka topic config strategy test passed")
}

// TestNATSTopicConfigStrategy 测试 NATS 主题配置策略
func TestNATSTopicConfigStrategy(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-config-strategy-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	// 设置策略
	bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)

	// 获取策略
	strategy := bus.GetTopicConfigStrategy()
	helper.AssertEqual(eventbus.StrategyCreateOrUpdate, strategy, "Strategy should match")

	// 设置其他策略
	bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
	strategy = bus.GetTopicConfigStrategy()
	helper.AssertEqual(eventbus.StrategyValidateOnly, strategy, "Strategy should match")

	t.Logf("✅ NATS topic config strategy test passed")
}
