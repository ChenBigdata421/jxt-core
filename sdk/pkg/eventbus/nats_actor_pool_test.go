//go:build integration
// +build integration

package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNATSActorPool_BasicProcessing 测试基本的消息处理
func TestNATSActorPool_BasicProcessing(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	// 创建 NATS EventBus
	clientID := fmt.Sprintf("test-nats-actor-pool-%d", time.Now().UnixNano())
	streamName := fmt.Sprintf("TEST_STREAM_%s", clientID)
	subjectPrefix := fmt.Sprintf("%s.>", clientID)

	config := &NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: clientID,
		JetStream: JetStreamConfig{
			Enabled: true,
			Stream: StreamConfig{
				Name:     streamName,
				Subjects: []string{subjectPrefix},
			},
		},
	}

	bus, err := NewNATSEventBus(config)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer bus.Close()

	natsEB, ok := bus.(*natsEventBus)
	require.True(t, ok, "Failed to cast to natsEventBus")
	require.NotNil(t, natsEB.actorPool, "Actor pool should be initialized")

	t.Logf("✅ NATS EventBus created with Hollywood Actor Pool")
}

// TestNATSActorPool_EnvelopeProcessing 测试 Envelope 消息处理
func TestNATSActorPool_EnvelopeProcessing(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	// 创建 NATS EventBus
	clientID := fmt.Sprintf("test-nats-envelope-%d", time.Now().UnixNano())
	streamName := fmt.Sprintf("TEST_STREAM_%s", clientID)
	subjectPrefix := fmt.Sprintf("%s.>", clientID)

	config := &NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: clientID,
		JetStream: JetStreamConfig{
			Enabled: true,
			Stream: StreamConfig{
				Name:     streamName,
				Subjects: []string{subjectPrefix},
			},
		},
	}

	bus, err := NewNATSEventBus(config)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer bus.Close()

	ctx := context.Background()
	topic := fmt.Sprintf("%s.test", config.ClientID)

	// 订阅 Envelope 消息
	var received atomic.Int64
	var receivedEnvelopes sync.Map

	err = bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *Envelope) error {
		received.Add(1)
		receivedEnvelopes.Store(envelope.EventID, envelope)
		t.Logf("📨 Received envelope: EventID=%s, AggregateID=%s", envelope.EventID, envelope.AggregateID)
		return nil
	})
	require.NoError(t, err, "Failed to subscribe to envelope")

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布多个 Envelope 消息
	messageCount := 10
	aggregateID := "test-aggregate-123"

	for i := 0; i < messageCount; i++ {
		envelope := &Envelope{
			EventID:      fmt.Sprintf("evt-%d", i),
			AggregateID:  aggregateID,
			EventType:    "TestEvent",
			EventVersion: int64(i + 1),
			Timestamp:    time.Now(),
			Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"index":%d}`, i)),
		}

		err = bus.PublishEnvelope(ctx, topic, envelope)
		require.NoError(t, err, "Failed to publish envelope")
	}

	// 等待消息接收
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for messages. Received: %d/%d", received.Load(), messageCount)
		case <-ticker.C:
			if received.Load() >= int64(messageCount) {
				t.Logf("✅ All messages received: %d/%d", received.Load(), messageCount)
				return
			}
		}
	}
}

// TestNATSActorPool_OrderGuarantee 测试同一聚合ID的消息顺序保证
func TestNATSActorPool_OrderGuarantee(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	// 创建 NATS EventBus
	clientID := fmt.Sprintf("test-nats-order-%d", time.Now().UnixNano())
	streamName := fmt.Sprintf("TEST_STREAM_%s", clientID)
	subjectPrefix := fmt.Sprintf("%s.>", clientID)

	config := &NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: clientID,
		JetStream: JetStreamConfig{
			Enabled: true,
			Stream: StreamConfig{
				Name:     streamName,
				Subjects: []string{subjectPrefix},
			},
		},
	}

	bus, err := NewNATSEventBus(config)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer bus.Close()

	ctx := context.Background()
	topic := fmt.Sprintf("%s.test", config.ClientID)

	// 订阅 Envelope 消息
	var receivedVersions []int64
	var mu sync.Mutex

	err = bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *Envelope) error {
		mu.Lock()
		receivedVersions = append(receivedVersions, envelope.EventVersion)
		mu.Unlock()
		// 模拟处理延迟
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	require.NoError(t, err, "Failed to subscribe to envelope")

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布多个 Envelope 消息（同一聚合ID）
	messageCount := 20
	aggregateID := "test-aggregate-456"

	for i := 0; i < messageCount; i++ {
		envelope := &Envelope{
			EventID:      fmt.Sprintf("evt-%d", i),
			AggregateID:  aggregateID,
			EventType:    "TestEvent",
			EventVersion: int64(i + 1),
			Timestamp:    time.Now(),
			Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"index":%d}`, i)),
		}

		err = bus.PublishEnvelope(ctx, topic, envelope)
		require.NoError(t, err, "Failed to publish envelope")
	}

	// 等待消息接收
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			mu.Lock()
			count := len(receivedVersions)
			mu.Unlock()
			t.Fatalf("Timeout waiting for messages. Received: %d/%d", count, messageCount)
		case <-ticker.C:
			mu.Lock()
			count := len(receivedVersions)
			mu.Unlock()
			if count >= messageCount {
				// 验证顺序
				mu.Lock()
				for i := 0; i < messageCount; i++ {
					expected := int64(i + 1)
					actual := receivedVersions[i]
					assert.Equal(t, expected, actual, "Message order violation at index %d", i)
				}
				mu.Unlock()
				t.Logf("✅ All messages received in order: %d/%d", count, messageCount)
				return
			}
		}
	}
}

// TestNATSActorPool_MultipleAggregates 测试多个聚合ID的并发处理
func TestNATSActorPool_MultipleAggregates(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	// 创建 NATS EventBus
	clientID := fmt.Sprintf("test-nats-multi-agg-%d", time.Now().UnixNano())
	streamName := fmt.Sprintf("TEST_STREAM_%s", clientID)
	subjectPrefix := fmt.Sprintf("%s.>", clientID)

	config := &NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: clientID,
		JetStream: JetStreamConfig{
			Enabled: true,
			Stream: StreamConfig{
				Name:     streamName,
				Subjects: []string{subjectPrefix},
			},
		},
	}

	bus, err := NewNATSEventBus(config)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer bus.Close()

	ctx := context.Background()
	topic := fmt.Sprintf("%s.test", config.ClientID)

	// 订阅 Envelope 消息
	var received atomic.Int64
	aggregateVersions := make(map[string][]int64)
	var mu sync.Mutex

	err = bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *Envelope) error {
		received.Add(1)
		mu.Lock()
		aggregateVersions[envelope.AggregateID] = append(aggregateVersions[envelope.AggregateID], envelope.EventVersion)
		mu.Unlock()
		return nil
	})
	require.NoError(t, err, "Failed to subscribe to envelope")

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布多个聚合的消息
	aggregateCount := 5
	messagesPerAggregate := 10
	totalMessages := aggregateCount * messagesPerAggregate

	for aggID := 1; aggID <= aggregateCount; aggID++ {
		for version := 1; version <= messagesPerAggregate; version++ {
			envelope := &Envelope{
				EventID:      fmt.Sprintf("evt-agg-%d-v%d", aggID, version),
				AggregateID:  fmt.Sprintf("aggregate-%d", aggID),
				EventType:    "TestEvent",
				EventVersion: int64(version),
				Timestamp:    time.Now(),
				Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"aggregate_id":%d,"version":%d}`, aggID, version)),
			}

			err = bus.PublishEnvelope(ctx, topic, envelope)
			require.NoError(t, err, "Failed to publish envelope")
		}
	}

	// 等待消息接收
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for messages. Received: %d/%d", received.Load(), totalMessages)
		case <-ticker.C:
			if received.Load() >= int64(totalMessages) {
				// 验证每个聚合的顺序
				mu.Lock()
				for aggID := 1; aggID <= aggregateCount; aggID++ {
					aggregateIDStr := fmt.Sprintf("aggregate-%d", aggID)
					versions := aggregateVersions[aggregateIDStr]
					assert.Equal(t, messagesPerAggregate, len(versions), "Aggregate %s should have %d messages", aggregateIDStr, messagesPerAggregate)

					// 验证顺序
					for i := 0; i < len(versions); i++ {
						expected := int64(i + 1)
						actual := versions[i]
						assert.Equal(t, expected, actual, "Aggregate %s: order violation at index %d", aggregateIDStr, i)
					}
				}
				mu.Unlock()
				t.Logf("✅ All messages received and ordered correctly: %d/%d", received.Load(), totalMessages)
				return
			}
		}
	}
}
