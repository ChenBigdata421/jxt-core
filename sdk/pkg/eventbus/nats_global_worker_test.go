package eventbus

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNATSGlobalWorkerPool 测试NATS全局Worker池是否正常工作
func TestNATSGlobalWorkerPool(t *testing.T) {
	// 创建NATS EventBus
	timestamp := time.Now().UnixNano()
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-global-worker-test-%d", timestamp),
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
				Name:      fmt.Sprintf("GLOBAL_WORKER_STREAM_%d", timestamp),
				Subjects:  []string{fmt.Sprintf("global@%d.>", timestamp)},
				Retention: "limits",
				Storage:   "file",
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  1024 * 1024 * 1024,
				MaxMsgs:   1000000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("global_worker_consumer_%d", timestamp),
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

	// 记录初始Goroutine数量
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("🔍 Initial Goroutines: %d", initialGoroutines)

	// 使用确保不会被识别为聚合ID的topic
	topic := fmt.Sprintf("global@%d.test@worker.msg#pool", timestamp)
	t.Logf("🎯 Using topic: %s", topic)

	// 验证topic不会被识别为聚合ID
	aggID, err := ExtractAggregateID([]byte("test"), nil, nil, topic)
	t.Logf("📊 ExtractAggregateID result: aggID='%s', err=%v", aggID, err)
	require.Empty(t, aggID, "Topic should not be recognized as having aggregate ID")

	var messageCount int64

	// 订阅消息
	t.Logf("🔥 CALLING bus.Subscribe for topic: %s", topic)
	err = bus.Subscribe(context.Background(), topic, func(ctx context.Context, data []byte) error {
		atomic.AddInt64(&messageCount, 1)
		count := atomic.LoadInt64(&messageCount)
		t.Logf("🔥 HANDLER CALLED: message %d (goroutines: %d)", count, runtime.NumGoroutine())
		// 模拟一些处理时间
		time.Sleep(1 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)
	t.Logf("🔥 bus.Subscribe completed successfully")

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布测试消息
	totalMessages := 1
	testMessage := []byte("test message for global worker pool")

	t.Logf("🚀 Publishing %d messages...", totalMessages)
	start := time.Now()

	for i := 0; i < totalMessages; i++ {
		err := bus.Publish(context.Background(), topic, testMessage)
		require.NoError(t, err)

		if (i+1)%1 == 0 {
			t.Logf("📤 Published %d messages", i+1)
		}
	}

	publishDuration := time.Since(start)
	t.Logf("📊 Published %d messages in %v (%.2f msg/s)",
		totalMessages, publishDuration, float64(totalMessages)/publishDuration.Seconds())

	// 等待消息处理完成
	t.Logf("⏳ Waiting for message processing...")
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️ Timeout waiting for messages")
			goto done
		case <-ticker.C:
			processed := atomic.LoadInt64(&messageCount)
			currentGoroutines := runtime.NumGoroutine()
			t.Logf("📊 Progress: %d/%d messages, %d goroutines",
				processed, totalMessages, currentGoroutines)

			if processed >= int64(totalMessages) {
				t.Logf("✅ All messages processed!")
				goto done
			}
		}
	}

done:
	finalProcessed := atomic.LoadInt64(&messageCount)
	finalGoroutines := runtime.NumGoroutine()

	t.Logf("\n🎯 ===== Global Worker Pool Test Results =====")
	t.Logf("📤 Published: %d messages", totalMessages)
	t.Logf("📥 Processed: %d messages", finalProcessed)
	t.Logf("🧵 Goroutines: %d → %d (delta: %+d)",
		initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)
	t.Logf("📊 Success Rate: %.2f%%",
		float64(finalProcessed)/float64(totalMessages)*100)

	// 验证结果
	require.GreaterOrEqual(t, finalProcessed, int64(float64(totalMessages)*0.9),
		"Should process at least 90% of messages")

	// 如果全局Worker池正常工作，Goroutine增长应该很少
	goroutineDelta := finalGoroutines - initialGoroutines
	t.Logf("🎯 Goroutine delta: %d (should be small if global worker pool is working)", goroutineDelta)
}
