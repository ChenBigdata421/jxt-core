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
	"github.com/stretchr/testify/require"
)

// ============================================================================
// NATS Actor Pool 集成测试
// ============================================================================

// TestNATSActorPool_MultipleAggregates_Integration 测试 NATS Actor Pool 多聚合并发处理的端到端集成
func TestNATSActorPool_MultipleAggregates_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS Actor Pool integration test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-multi-agg-int-%d", helper.GetTimestamp())
	topic := fmt.Sprintf("%s.events", clientID)
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_STREAM_%s", clientID))

	ctx := context.Background()

	// 跟踪每个聚合的接收顺序
	aggregateCount := 5
	messagesPerAggregate := 10
	totalMessages := aggregateCount * messagesPerAggregate

	var receivedCount int64
	receivedVersions := make(map[string][]int64)
	var mu sync.Mutex

	// 订阅 Envelope
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		mu.Lock()
		receivedVersions[envelope.AggregateID] = append(receivedVersions[envelope.AggregateID], envelope.EventVersion)
		mu.Unlock()

		atomic.AddInt64(&receivedCount, 1)
		t.Logf("📨 Received: AggregateID=%s, Version=%d", envelope.AggregateID, envelope.EventVersion)
		return nil
	})
	require.NoError(t, err, "SubscribeEnvelope should not return error")

	// 等待订阅建立
	time.Sleep(2 * time.Second)

	// 发布多个聚合的消息
	for aggID := 1; aggID <= aggregateCount; aggID++ {
		for version := 1; version <= messagesPerAggregate; version++ {
			envelope := &eventbus.Envelope{
				EventID:      fmt.Sprintf("evt-agg-%d-v%d", aggID, version),
				AggregateID:  fmt.Sprintf("aggregate-%d", aggID),
				EventType:    "TestEvent",
				EventVersion: int64(version),
				Timestamp:    time.Now(),
				Payload:      []byte(fmt.Sprintf(`{"aggregate_id":%d,"version":%d}`, aggID, version)),
			}
			err = bus.PublishEnvelope(ctx, topic, envelope)
			require.NoError(t, err, "PublishEnvelope should not return error")
		}
	}

	// 等待所有消息接收
	success := helper.WaitForMessages(&receivedCount, int64(totalMessages), 30*time.Second)
	require.True(t, success, "Should receive all messages within timeout")

	// 验证接收数量
	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, aggregateCount, len(receivedVersions), "Should have all aggregates")

	// 验证每个聚合的顺序
	orderViolations := 0
	for aggID := 1; aggID <= aggregateCount; aggID++ {
		aggregateKey := fmt.Sprintf("aggregate-%d", aggID)
		versions := receivedVersions[aggregateKey]

		require.Equal(t, messagesPerAggregate, len(versions), "Aggregate %s should have all messages", aggregateKey)

		// 验证顺序
		for i := 0; i < len(versions); i++ {
			expected := int64(i + 1)
			if versions[i] != expected {
				orderViolations++
				t.Errorf("❌ Order violation in %s: expected version %d, got %d at position %d",
					aggregateKey, expected, versions[i], i)
			}
		}
	}

	require.Equal(t, 0, orderViolations, "Should have no order violations")

	t.Logf("✅ NATS Actor Pool multiple aggregates integration test passed")
	t.Logf("   - Total messages: %d", totalMessages)
	t.Logf("   - Aggregates: %d", aggregateCount)
	t.Logf("   - Messages per aggregate: %d", messagesPerAggregate)
	t.Logf("   - Order violations: %d", orderViolations)
}

// TestNATSActorPool_RoundRobin_Integration 测试 NATS Actor Pool Round-Robin 路由的端到端集成
func TestNATSActorPool_RoundRobin_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS Actor Pool Round-Robin integration test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-rr-int-%d", helper.GetTimestamp())
	topic := fmt.Sprintf("%s.messages", clientID)
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_STREAM_%s", clientID))

	ctx := context.Background()

	// 跟踪接收的消息
	messageCount := 100
	var receivedCount int64
	receivedMessages := make([]string, 0, messageCount)
	var mu sync.Mutex

	// 订阅普通消息
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		mu.Lock()
		receivedMessages = append(receivedMessages, string(message))
		mu.Unlock()

		atomic.AddInt64(&receivedCount, 1)
		return nil
	})
	require.NoError(t, err, "Subscribe should not return error")

	// 等待订阅建立
	time.Sleep(2 * time.Second)

	// 发布多条普通消息
	for i := 1; i <= messageCount; i++ {
		message := []byte(fmt.Sprintf("message-%d", i))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err, "Publish should not return error")
	}

	// 等待所有消息接收
	success := helper.WaitForMessages(&receivedCount, int64(messageCount), 30*time.Second)
	require.True(t, success, "Should receive all messages within timeout")

	// 验证接收数量
	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, messageCount, len(receivedMessages), "Should receive all messages")

	t.Logf("✅ NATS Actor Pool Round-Robin integration test passed")
	t.Logf("   - Total messages: %d", messageCount)
	t.Logf("   - Received messages: %d", len(receivedMessages))
}

// TestNATSActorPool_MixedTopics_Integration 测试 NATS Actor Pool 同时处理领域事件和普通消息
func TestNATSActorPool_MixedTopics_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS Actor Pool mixed topics integration test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-mixed-int-%d", helper.GetTimestamp())
	eventTopic := fmt.Sprintf("%s.events", clientID)
	messageTopic := fmt.Sprintf("%s.messages", clientID)
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_STREAM_%s", clientID))

	ctx := context.Background()

	// 跟踪领域事件
	eventCount := 20
	var receivedEventCount int64
	receivedEventVersions := make(map[string][]int64)
	var eventMu sync.Mutex

	// 跟踪普通消息
	messageCount := 30
	var receivedMessageCount int64
	receivedMessages := make([]string, 0, messageCount)
	var messageMu sync.Mutex

	// 订阅领域事件
	err := bus.SubscribeEnvelope(ctx, eventTopic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		eventMu.Lock()
		receivedEventVersions[envelope.AggregateID] = append(receivedEventVersions[envelope.AggregateID], envelope.EventVersion)
		eventMu.Unlock()

		atomic.AddInt64(&receivedEventCount, 1)
		t.Logf("📨 Event: AggregateID=%s, Version=%d", envelope.AggregateID, envelope.EventVersion)
		return nil
	})
	require.NoError(t, err, "SubscribeEnvelope should not return error")

	// 订阅普通消息
	err = bus.Subscribe(ctx, messageTopic, func(ctx context.Context, message []byte) error {
		messageMu.Lock()
		receivedMessages = append(receivedMessages, string(message))
		messageMu.Unlock()

		atomic.AddInt64(&receivedMessageCount, 1)
		t.Logf("📨 Message: %s", string(message))
		return nil
	})
	require.NoError(t, err, "Subscribe should not return error")

	// 等待订阅建立
	time.Sleep(2 * time.Second)

	// 并发发布领域事件和普通消息
	var wg sync.WaitGroup

	// 发布领域事件（2个聚合，每个10条消息）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for aggID := 1; aggID <= 2; aggID++ {
			for version := 1; version <= 10; version++ {
				envelope := &eventbus.Envelope{
					EventID:      fmt.Sprintf("evt-agg-%d-v%d", aggID, version),
					AggregateID:  fmt.Sprintf("aggregate-%d", aggID),
					EventType:    "TestEvent",
					EventVersion: int64(version),
					Timestamp:    time.Now(),
					Payload:      []byte(fmt.Sprintf(`{"aggregate_id":%d,"version":%d}`, aggID, version)),
				}
				err := bus.PublishEnvelope(ctx, eventTopic, envelope)
				require.NoError(t, err, "PublishEnvelope should not return error")
			}
		}
	}()

	// 发布普通消息
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= messageCount; i++ {
			message := []byte(fmt.Sprintf("message-%d", i))
			err := bus.Publish(ctx, messageTopic, message)
			require.NoError(t, err, "Publish should not return error")
		}
	}()

	wg.Wait()

	// 等待所有消息接收
	eventSuccess := helper.WaitForMessages(&receivedEventCount, int64(eventCount), 30*time.Second)
	messageSuccess := helper.WaitForMessages(&receivedMessageCount, int64(messageCount), 30*time.Second)

	require.True(t, eventSuccess, "Should receive all events within timeout")
	require.True(t, messageSuccess, "Should receive all messages within timeout")

	// 验证领域事件
	eventMu.Lock()
	require.Equal(t, 2, len(receivedEventVersions), "Should have 2 aggregates")

	// 验证每个聚合的顺序
	orderViolations := 0
	for aggID := 1; aggID <= 2; aggID++ {
		aggregateKey := fmt.Sprintf("aggregate-%d", aggID)
		versions := receivedEventVersions[aggregateKey]

		require.Equal(t, 10, len(versions), "Aggregate %s should have 10 messages", aggregateKey)

		// 验证顺序
		for i := 0; i < len(versions); i++ {
			expected := int64(i + 1)
			if versions[i] != expected {
				orderViolations++
				t.Errorf("❌ Order violation in %s: expected version %d, got %d at position %d",
					aggregateKey, expected, versions[i], i)
			}
		}
	}
	eventMu.Unlock()

	// 验证普通消息
	messageMu.Lock()
	require.Equal(t, messageCount, len(receivedMessages), "Should receive all messages")
	messageMu.Unlock()

	require.Equal(t, 0, orderViolations, "Should have no order violations in events")

	t.Logf("✅ NATS Actor Pool mixed topics integration test passed")
	t.Logf("   - Events: %d (2 aggregates × 10 messages)", eventCount)
	t.Logf("   - Messages: %d", messageCount)
	t.Logf("   - Order violations: %d", orderViolations)
}

