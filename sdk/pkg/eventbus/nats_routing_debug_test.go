package eventbus

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNATSRoutingDebug 调试NATS消息路由逻辑
func TestNATSRoutingDebug(t *testing.T) {
	// 创建NATS EventBus
	timestamp := time.Now().UnixNano()
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-routing-debug-%d", timestamp),
		MaxReconnects:       5,
		ReconnectWait:       2 * time.Second,
		ConnectionTimeout:   10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 100 * time.Millisecond,
			AckWait:        30 * time.Second,
			MaxDeliver:     3,
			Stream: StreamConfig{
				Name:      fmt.Sprintf("DEBUG_STREAM_%d", timestamp),
				Subjects:  []string{fmt.Sprintf("debug.%d.>", timestamp)},
				Retention: "limits",
				Storage:   "file",
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  1024 * 1024 * 1024,
				MaxMsgs:   1000000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("debug_consumer_%d", timestamp),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 10000,
				MaxWaiting:    1000,
				MaxDeliver:    3,
			},
		},
	}

	bus, err := NewNATSEventBus(config)
	require.NoError(t, err)
	defer bus.Close()

	// 等待连接稳定
	time.Sleep(2 * time.Second)

	// 测试不同的topic名称，观察路由行为
	testCases := []struct {
		name  string
		topic string
	}{
		{"Simple topic", fmt.Sprintf("debug.%d.simple", timestamp)},
		{"Topic with underscore", fmt.Sprintf("debug.%d.test_1", timestamp)},
		{"Topic with dash", fmt.Sprintf("debug.%d.test-1", timestamp)},
		{"Topic with special char", fmt.Sprintf("debug.%d.test@1", timestamp)},
		{"Topic with numbers", fmt.Sprintf("debug.%d.123", timestamp)},
	}

	messageCount := 0
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 订阅消息
			err := bus.Subscribe(context.Background(), tc.topic, func(ctx context.Context, data []byte) error {
				messageCount++
				t.Logf("Received message %d on topic %s", messageCount, tc.topic)
				return nil
			})
			require.NoError(t, err)

			// 等待订阅生效
			time.Sleep(1 * time.Second)

			// 发布测试消息
			testMessage := []byte(fmt.Sprintf("test message for %s", tc.topic))
			err = bus.Publish(context.Background(), tc.topic, testMessage)
			require.NoError(t, err)

			t.Logf("Published message to topic: %s", tc.topic)
		})
	}

	// 等待消息处理
	time.Sleep(5 * time.Second)

	t.Logf("Total messages processed: %d", messageCount)
}
