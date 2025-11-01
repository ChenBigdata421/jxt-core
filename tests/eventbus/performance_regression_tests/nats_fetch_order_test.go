package performance_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
	"github.com/stretchr/testify/require"
)

// TestNATSFetchOrder 测试 NATS Fetch() 返回的消息顺序
func TestNATSFetchOrder(t *testing.T) {
	t.Log("🔍 ===== NATS Fetch() 顺序测试 =====")

	// 创建 NATS EventBus
	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("FETCH_ORDER_%d", timestamp)
	topic := fmt.Sprintf("fetch.order.%d.test", timestamp)

	natsConfig := &eventbus.NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-fetch-order-%d", timestamp),
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
				Subjects:  []string{fmt.Sprintf("fetch.order.%d.>", timestamp)},
				Storage:   "file",
				Retention: "limits",
				MaxAge:    1 * time.Hour,
				MaxBytes:  1024 * 1024 * 1024,
				MaxMsgs:   100000,
				Replicas:  1,
				Discard:   "old",
			},
			Consumer: eventbus.NATSConsumerConfig{
				DurableName:   fmt.Sprintf("fetch_order_%d", timestamp),
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

	// 发送 100 条消息（同一个聚合ID）
	aggregateID := "test-aggregate"
	messageCount := 100

	t.Logf("📤 发送 %d 条消息（聚合ID: %s）...", messageCount, aggregateID)

	for version := int64(1); version <= int64(messageCount); version++ {
		envelope := &eventbus.Envelope{
			EventID:      fmt.Sprintf("event-%d-%d", timestamp, version),
			AggregateID:  aggregateID,
			EventType:    "TestEvent",
			EventVersion: version,
			Timestamp:    time.Now(),
			Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"message":"message %d"}`, version)),
		}

		err := eb.PublishEnvelope(ctx, topic, envelope)
		require.NoError(t, err, "Failed to publish message")
	}

	t.Logf("✅ 发送完成")

	// 等待消息持久化
	time.Sleep(2 * time.Second)

	// 现在直接使用 NATS 客户端拉取消息，检查顺序
	t.Logf("📥 开始拉取消息...")

	// 获取 NATS EventBus 的内部连接（这里需要访问私有字段，仅用于测试）
	// 由于无法直接访问，我们通过订阅来验证

	receivedVersions := make([]int64, 0, messageCount)
	orderViolations := 0

	handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
		receivedVersions = append(receivedVersions, envelope.EventVersion)

		if len(receivedVersions) > 1 {
			lastVersion := receivedVersions[len(receivedVersions)-2]
			currentVersion := envelope.EventVersion

			if currentVersion != lastVersion+1 {
				orderViolations++
				t.Logf("❌ 顺序违反: LastVersion=%d, CurrentVersion=%d, ReceivedSeq=%d",
					lastVersion, currentVersion, len(receivedVersions))
			}
		}

		return nil
	}

	err = eb.SubscribeEnvelope(ctx, topic, handler)
	require.NoError(t, err, "Failed to subscribe")

	// 等待所有消息接收完成
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️  超时: 接收 %d/%d", len(receivedVersions), messageCount)
			goto ANALYZE
		case <-ticker.C:
			if len(receivedVersions) >= messageCount {
				t.Logf("✅ 所有消息已接收")
				goto ANALYZE
			}
		}
	}

ANALYZE:
	time.Sleep(1 * time.Second)

	t.Logf("\n================================================================================")
	t.Logf("📊 测试结果")
	t.Logf("================================================================================")
	t.Logf("发送消息数: %d", messageCount)
	t.Logf("接收消息数: %d", len(receivedVersions))
	t.Logf("顺序违反: %d", orderViolations)

	if orderViolations > 0 {
		t.Logf("\n接收顺序（前 20 条）:")
		for i := 0; i < 20 && i < len(receivedVersions); i++ {
			t.Logf("  [%d] Version=%d", i+1, receivedVersions[i])
		}
	}

	t.Logf("================================================================================")

	// 断言：不应该有顺序违反
	require.Equal(t, 0, orderViolations, "发现顺序违反")
	require.Equal(t, messageCount, len(receivedVersions), "消息数量不匹配")
}
