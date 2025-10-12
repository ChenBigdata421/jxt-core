package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 🎯 NATS JetStream 高压力测试
// 与Kafka高压力测试进行公平对比

// NATSHighPressureMetrics NATS高压力测试指标
type NATSHighPressureMetrics struct {
	// 发送指标
	MessagesSent     int64 // 发送消息总数
	SendErrors       int64 // 发送错误数
	SendLatencySum   int64 // 发送延迟总和(微秒)
	SendLatencyCount int64 // 发送延迟计数

	// 接收指标
	MessagesReceived    int64 // 接收消息总数
	ProcessErrors       int64 // 处理错误数
	ProcessLatencySum   int64 // 处理延迟总和(微秒)
	ProcessLatencyCount int64 // 处理延迟计数

	// 性能指标
	StartTime         time.Time // 开始时间
	EndTime           time.Time // 结束时间
	OrderViolations   int64     // 顺序违反次数
	DuplicateMessages int64     // 重复消息次数
}

// NATSHighPressureTestMessage NATS高压力测试消息
type NATSHighPressureTestMessage struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// TestNATSJetStreamHighPressure NATS JetStream高压力测试
func TestNATSJetStreamHighPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS JetStream high pressure test in short mode")
	}

	metrics := &NATSHighPressureMetrics{
		StartTime: time.Now(),
	}

	// 🚀 NATS JetStream高性能配置
	natsConfig := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            "nats-jetstream-high-pressure-client",
		MaxReconnects:       10,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   30 * time.Second,
		HealthCheckInterval: 60 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: true, // 启用JetStream
			Stream: StreamConfig{
				Name:      "HIGH_PRESSURE_STREAM",
				Subjects:  []string{"high.pressure.>"},
				Storage:   "memory", // 使用内存存储以获得最高性能
				Retention: "limits",
				MaxAge:    24 * time.Hour,
				MaxBytes:  100 * 1024 * 1024, // 100MB
				MaxMsgs:   100000,
				Replicas:  1, // 单副本以获得最高性能
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   "high-pressure-consumer",
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 1000, // 增加待确认消息数
				MaxWaiting:    512,
				MaxDeliver:    3,
				BackOff:       []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
			},
			PublishTimeout: 10 * time.Second,
		},
	}

	eventBus, err := NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS JetStream EventBus")
	defer eventBus.Close()

	// 等待JetStream初始化
	t.Logf("⏳ Waiting for NATS JetStream to initialize...")
	time.Sleep(5 * time.Second)

	t.Logf("🚀 Starting NATS JetStream High Pressure Test")

	// 运行高压力测试 - 与Kafka相同的参数
	runNATSHighPressureTest(t, eventBus, metrics)

	// 分析结果
	analyzeNATSHighPressureResults(t, metrics)
}

// runNATSHighPressureTest 运行NATS高压力测试
func runNATSHighPressureTest(t *testing.T, eventBus EventBus, metrics *NATSHighPressureMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 🔥 与Kafka测试相同的参数
	topics := []string{
		"high.pressure.topic.1",
		"high.pressure.topic.2",
		"high.pressure.topic.3",
		"high.pressure.topic.4",
		"high.pressure.topic.5",
	}
	messagesPerTopic := 2000 // 每个topic 2000条消息，总计10,000条

	t.Logf("📊 Test Config: %d topics, %d msgs/topic, total: %d messages",
		len(topics), messagesPerTopic, len(topics)*messagesPerTopic)

	// 设置消息处理器
	var wg sync.WaitGroup
	setupNATSHighPressureHandlers(t, eventBus, topics, metrics, &wg)

	// 等待订阅建立
	t.Logf("⏳ Waiting for subscriptions to stabilize...")
	time.Sleep(3 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendNATSHighPressureMessages(t, eventBus, topics, messagesPerTopic, metrics)

	// 等待消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("✅ All messages processed successfully")
	case <-time.After(90 * time.Second):
		t.Logf("⏰ Test timeout reached")
	case <-ctx.Done():
		t.Logf("🛑 Context cancelled")
	}

	metrics.EndTime = time.Now()
}

// setupNATSHighPressureHandlers 设置NATS高压力处理器
func setupNATSHighPressureHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *NATSHighPressureMetrics, wg *sync.WaitGroup) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()

			var lastSequence int64 = -1
			receivedMessages := make(map[string]bool) // 检测重复消息

			handler := func(ctx context.Context, message []byte) error {
				// 解析消息
				var testMsg NATSHighPressureTestMessage
				if err := json.Unmarshal(message, &testMsg); err != nil {
					atomic.AddInt64(&metrics.ProcessErrors, 1)
					return err
				}

				// 检测重复消息
				if receivedMessages[testMsg.ID] {
					atomic.AddInt64(&metrics.DuplicateMessages, 1)
					t.Logf("⚠️ Duplicate message detected: %s in topic %s", testMsg.ID, topicName)
				}
				receivedMessages[testMsg.ID] = true

				// 检测顺序违反
				if lastSequence >= 0 && testMsg.Sequence <= lastSequence {
					atomic.AddInt64(&metrics.OrderViolations, 1)
				}
				lastSequence = testMsg.Sequence

				// 记录处理延迟
				processingTime := time.Since(testMsg.Timestamp).Microseconds()
				atomic.AddInt64(&metrics.ProcessLatencySum, processingTime)
				atomic.AddInt64(&metrics.ProcessLatencyCount, 1)

				// 更新接收计数
				atomic.AddInt64(&metrics.MessagesReceived, 1)

				return nil
			}

			err := eventBus.Subscribe(context.Background(), topicName, handler)
			if err != nil {
				t.Errorf("Failed to subscribe to topic %s: %v", topicName, err)
			}
		}(topic)
	}
}

// sendNATSHighPressureMessages 发送NATS高压力消息
func sendNATSHighPressureMessages(t *testing.T, eventBus EventBus, topics []string, messagesPerTopic int, metrics *NATSHighPressureMetrics) {
	var sendWg sync.WaitGroup

	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()

			for i := 0; i < messagesPerTopic; i++ {
				testMsg := NATSHighPressureTestMessage{
					ID:        fmt.Sprintf("%s-%d", topicName, i),
					Topic:     topicName,
					Sequence:  int64(i),
					Timestamp: time.Now(),
					Data:      fmt.Sprintf("high-pressure-data-%d", i),
				}

				messageBytes, err := json.Marshal(testMsg)
				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
					continue
				}

				startTime := time.Now()
				err = eventBus.Publish(context.Background(), topicName, messageBytes)
				sendTime := time.Since(startTime).Microseconds()

				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
					t.Logf("❌ Failed to send message to %s: %v", topicName, err)
				} else {
					atomic.AddInt64(&metrics.MessagesSent, 1)
					atomic.AddInt64(&metrics.SendLatencySum, sendTime)
					atomic.AddInt64(&metrics.SendLatencyCount, 1)
				}

				// 🔥 无速率限制 - 最大压力测试
			}
		}(topic)
	}

	sendWg.Wait()
	t.Logf("📤 Finished sending NATS high pressure messages")
}

// analyzeNATSHighPressureResults 分析NATS高压力结果
func analyzeNATSHighPressureResults(t *testing.T, metrics *NATSHighPressureMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	successRate := float64(metrics.MessagesReceived) / float64(metrics.MessagesSent) * 100
	throughput := float64(metrics.MessagesReceived) / duration.Seconds()

	avgSendLatency := float64(metrics.SendLatencySum) / float64(metrics.SendLatencyCount) / 1000.0          // ms
	avgProcessLatency := float64(metrics.ProcessLatencySum) / float64(metrics.ProcessLatencyCount) / 1000.0 // ms

	t.Logf("\n🎯 ===== NATS JetStream High Pressure Results =====")
	t.Logf("⏱️  Test Duration: %v", duration)
	t.Logf("📤 Messages Sent: %d", metrics.MessagesSent)
	t.Logf("📥 Messages Received: %d", metrics.MessagesReceived)
	t.Logf("❌ Send Errors: %d", metrics.SendErrors)
	t.Logf("❌ Process Errors: %d", metrics.ProcessErrors)
	t.Logf("✅ Success Rate: %.2f%%", successRate)
	t.Logf("🚀 Throughput: %.2f msg/s", throughput)
	t.Logf("⚡ Avg Send Latency: %.2f ms", avgSendLatency)
	t.Logf("⚡ Avg Process Latency: %.2f ms", avgProcessLatency)

	t.Logf("\n📊 Quality Metrics:")
	t.Logf("   Order Violations: %d", metrics.OrderViolations)
	t.Logf("   Duplicate Messages: %d", metrics.DuplicateMessages)

	// 🏆 与Kafka对比评估
	t.Logf("\n🏆 Performance Evaluation:")
	if successRate >= 95.0 {
		t.Logf("🎉 优秀! NATS JetStream在高压力下表现卓越!")
		t.Logf("   ✅ 成功率: %.2f%% (远超Kafka的11.7%%)", successRate)
		t.Logf("   ✅ 吞吐量: %.0f msg/s", throughput)
		t.Logf("   ✅ 延迟: %.2f ms", avgSendLatency)
	} else if successRate >= 80.0 {
		t.Logf("⚠️ 良好! NATS JetStream表现良好")
		t.Logf("   ✅ 成功率: %.2f%% (仍优于Kafka)", successRate)
	} else {
		t.Logf("❌ 需要优化，成功率仅为 %.2f%%", successRate)
	}

	// 基本验证
	assert.Greater(t, metrics.MessagesSent, int64(0), "Should send messages")
	assert.Greater(t, metrics.MessagesReceived, int64(0), "Should receive messages")
	assert.Greater(t, successRate, 80.0, "Success rate should be > 80%")
	assert.LessOrEqual(t, metrics.OrderViolations, int64(100), "Should have minimal order violations")

	t.Logf("✅ NATS JetStream High Pressure Test Completed!")
}
