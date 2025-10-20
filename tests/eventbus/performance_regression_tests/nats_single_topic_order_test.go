package performance_tests

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

// TestNATSSingleTopicOrder 测试单个 topic 的消息顺序
func TestNATSSingleTopicOrder(t *testing.T) {
	t.Log("🔍 ===== NATS 单 Topic 顺序测试 =====")

	// 创建 NATS EventBus
	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("SINGLE_TOPIC_%d", timestamp)
	topic := fmt.Sprintf("single.%d.test", timestamp)

	natsConfig := &eventbus.NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-single-%d", timestamp),
		MaxReconnects:       10,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   30 * time.Second,
		HealthCheckInterval: 60 * time.Second,
		JetStream: eventbus.JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 10 * time.Second,
			AckWait:        15 * time.Second,
			MaxDeliver:     3,
			Stream: eventbus.StreamConfig{
				Name:      streamName,
				Subjects:  []string{fmt.Sprintf("single.%d.>", timestamp)},
				Storage:   "file",
				Retention: "limits",
				MaxAge:    1 * time.Hour,
				MaxBytes:  1024 * 1024 * 1024,
				MaxMsgs:   100000,
				Replicas:  1,
				Discard:   "old",
			},
			Consumer: eventbus.NATSConsumerConfig{
				DurableName:   fmt.Sprintf("single_%d", timestamp),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 1000,
				MaxWaiting:    500,
				MaxDeliver:    3,
			},
		},
	}

	eb, err := eventbus.NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eb.Close()

	ctx := context.Background()

	// 测试参数
	aggregateCount := 10       // 10 个聚合
	messagesPerAggregate := 10 // 每个聚合 10 条消息
	totalMessages := aggregateCount * messagesPerAggregate

	t.Logf("📊 测试参数:")
	t.Logf("   聚合数量: %d", aggregateCount)
	t.Logf("   每个聚合消息数: %d", messagesPerAggregate)
	t.Logf("   总消息数: %d", totalMessages)
	t.Logf("   Topic: %s", topic)

	// 生成聚合ID列表
	aggregateIDs := make([]string, aggregateCount)
	for i := 0; i < aggregateCount; i++ {
		aggregateIDs[i] = fmt.Sprintf("agg-%d", i)
	}

	var receivedCount int64
	var orderViolations int64

	// 每个聚合ID的状态
	type AggregateState struct {
		lastVersion int64
	}
	aggregateStates := make(map[string]*AggregateState)
	var statesMu sync.Mutex

	// 创建消息处理器
	handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
		seq := atomic.AddInt64(&receivedCount, 1)

		// 获取或创建聚合状态
		statesMu.Lock()
		state, exists := aggregateStates[envelope.AggregateID]
		if !exists {
			state = &AggregateState{}
			aggregateStates[envelope.AggregateID] = state
		}
		statesMu.Unlock()

		// 检查顺序
		currentVersion := envelope.EventVersion
		if currentVersion != state.lastVersion+1 {
			atomic.AddInt64(&orderViolations, 1)
			t.Logf("❌ 顺序违反: AggregateID=%s, LastVersion=%d, CurrentVersion=%d, ReceivedSeq=%d",
				envelope.AggregateID, state.lastVersion, currentVersion, seq)
		}

		state.lastVersion = currentVersion
		return nil
	}

	// 订阅
	err = eb.SubscribeEnvelope(ctx, topic, handler)
	require.NoError(t, err, "Failed to subscribe")

	// 等待订阅建立
	time.Sleep(2 * time.Second)

	t.Logf("📤 开始发送消息...")

	// 为每个聚合ID串行发送消息
	for aggIndex := 0; aggIndex < aggregateCount; aggIndex++ {
		aggregateID := aggregateIDs[aggIndex]

		for version := int64(1); version <= int64(messagesPerAggregate); version++ {
			envelope := &eventbus.Envelope{
				AggregateID:  aggregateID,
				EventType:    "TestEvent",
				EventVersion: version,
				Timestamp:    time.Now(),
				Payload:      []byte(fmt.Sprintf("aggregate %s message %d", aggregateID, version)),
			}

			err := eb.PublishEnvelope(ctx, topic, envelope)
			require.NoError(t, err, "Failed to publish message")
		}
	}

	t.Logf("✅ 发送完成: %d 条消息", totalMessages)

	t.Logf("⏳ 等待消息处理完成...")

	// 等待所有消息接收完成
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️  超时: 接收 %d/%d", receivedCount, totalMessages)
			goto ANALYZE
		case <-ticker.C:
			current := atomic.LoadInt64(&receivedCount)
			violations := atomic.LoadInt64(&orderViolations)
			t.Logf("📊 进度: 接收 %d/%d, 顺序违反 %d", current, totalMessages, violations)
			if current >= int64(totalMessages) {
				t.Logf("✅ 所有消息已接收")
				goto ANALYZE
			}
		}
	}

ANALYZE:
	time.Sleep(1 * time.Second)

	finalReceived := atomic.LoadInt64(&receivedCount)
	finalViolations := atomic.LoadInt64(&orderViolations)

	t.Logf("\n================================================================================")
	t.Logf("📊 测试结果")
	t.Logf("================================================================================")
	t.Logf("发送消息数: %d", totalMessages)
	t.Logf("接收消息数: %d", finalReceived)
	t.Logf("成功率: %.2f%%", float64(finalReceived)/float64(totalMessages)*100)
	t.Logf("顺序违反: %d", finalViolations)
	t.Logf("顺序违反率: %.2f%%", float64(finalViolations)/float64(totalMessages)*100)
	t.Logf("================================================================================")

	// 断言：不应该有顺序违反
	require.Equal(t, int64(0), finalViolations, "发现顺序违反")
	require.Equal(t, int64(totalMessages), finalReceived, "消息数量不匹配")
}
