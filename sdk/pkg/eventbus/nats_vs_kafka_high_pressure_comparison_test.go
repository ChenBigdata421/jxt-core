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

// 🎯 NATS vs Kafka 高压力对比测试
// 公平对比两种消息中间件在相同高压力下的表现

// HighPressureComparisonMetrics 高压力对比测试指标
type HighPressureComparisonMetrics struct {
	MessagesSent     int64
	MessagesReceived int64
	SendErrors       int64
	ProcessErrors    int64
	StartTime        time.Time
	EndTime          time.Time
	OrderViolations  int64
	SendLatencySum   int64
	SendLatencyCount int64
}

// HighPressureTestMessage 高压力测试消息
type HighPressureTestMessage struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// TestNATSHighPressureBasic NATS基本高压力测试 (非JetStream)
func TestNATSHighPressureBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS high pressure basic test in short mode")
	}

	metrics := &HighPressureComparisonMetrics{
		StartTime: time.Now(),
	}

	// 🚀 NATS基本配置 - 最高性能
	natsConfig := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            "nats-high-pressure-basic-client",
		MaxReconnects:       5,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: false, // 禁用JetStream，使用基本NATS获得最高性能
		},
	}

	eventBus, err := NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(2 * time.Second)

	t.Logf("🚀 Starting NATS High Pressure Basic Test")
	
	// 运行高压力测试
	runHighPressureComparisonTest(t, eventBus, "NATS", metrics)

	// 分析结果
	analyzeHighPressureComparisonResults(t, "NATS", metrics)
}

// TestKafkaHighPressureComparison Kafka高压力对比测试
func TestKafkaHighPressureComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka high pressure comparison test in short mode")
	}

	metrics := &HighPressureComparisonMetrics{
		StartTime: time.Now(),
	}

	// 🔧 Kafka优化配置
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29093"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none", // 禁用压缩以获得最高性能
			FlushFrequency:   10 * time.Millisecond,
			FlushMessages:    10,
			Timeout:          30 * time.Second,
			FlushBytes:       64 * 1024,
			RetryMax:         3,
			BatchSize:        4 * 1024,
			BufferSize:       8 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  256 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         5 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      3,
		},
		Consumer: ConsumerConfig{
			GroupID:           "kafka-high-pressure-comparison-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    60 * time.Second,
			HeartbeatInterval: 20 * time.Second,
			MaxProcessingTime:  90 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      5 * 1024 * 1024,
			FetchMaxWait:       1 * time.Second,
			MaxPollRecords:     50, // 减少批量大小
			EnableAutoCommit:   true,
			AutoCommitInterval: 10 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		HealthCheckInterval:  60 * time.Second,
		ClientID:             "kafka-high-pressure-comparison-client",
		MetadataRefreshFreq:  10 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 250 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:  30 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			KeepAlive:    30 * time.Second,
			MaxIdleConns: 10,
			MaxOpenConns: 100,
		},
	}

	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err, "Failed to create Kafka EventBus")
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(5 * time.Second)

	t.Logf("🚀 Starting Kafka High Pressure Comparison Test")
	
	// 运行高压力测试
	runHighPressureComparisonTest(t, eventBus, "Kafka", metrics)

	// 分析结果
	analyzeHighPressureComparisonResults(t, "Kafka", metrics)
}

// runHighPressureComparisonTest 运行高压力对比测试
func runHighPressureComparisonTest(t *testing.T, eventBus EventBus, system string, metrics *HighPressureComparisonMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 🔥 高压力测试参数
	topics := []string{
		"high.pressure.comparison.topic.1",
		"high.pressure.comparison.topic.2",
		"high.pressure.comparison.topic.3",
	}
	messagesPerTopic := 1000 // 每个topic 1000条消息，总计3,000条

	t.Logf("📊 %s Test Config: %d topics, %d msgs/topic, total: %d messages",
		system, len(topics), messagesPerTopic, len(topics)*messagesPerTopic)

	// 设置消息处理器
	var wg sync.WaitGroup
	setupHighPressureComparisonHandlers(t, eventBus, topics, metrics, &wg)

	// 等待订阅建立
	t.Logf("⏳ Waiting for %s subscriptions to stabilize...", system)
	time.Sleep(3 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendHighPressureComparisonMessages(t, eventBus, topics, messagesPerTopic, metrics)

	// 等待消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("✅ All %s messages processed successfully", system)
	case <-time.After(60 * time.Second):
		t.Logf("⏰ %s test timeout reached", system)
	case <-ctx.Done():
		t.Logf("🛑 %s context cancelled", system)
	}

	metrics.EndTime = time.Now()
}

// setupHighPressureComparisonHandlers 设置高压力对比处理器
func setupHighPressureComparisonHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *HighPressureComparisonMetrics, wg *sync.WaitGroup) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()
			
			var lastSequence int64 = -1
			
			handler := func(ctx context.Context, message []byte) error {
				// 解析消息
				var testMsg HighPressureTestMessage
				if err := json.Unmarshal(message, &testMsg); err != nil {
					atomic.AddInt64(&metrics.ProcessErrors, 1)
					return err
				}
				
				// 检测顺序违反
				if lastSequence >= 0 && testMsg.Sequence <= lastSequence {
					atomic.AddInt64(&metrics.OrderViolations, 1)
				}
				lastSequence = testMsg.Sequence
				
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

// sendHighPressureComparisonMessages 发送高压力对比消息
func sendHighPressureComparisonMessages(t *testing.T, eventBus EventBus, topics []string, messagesPerTopic int, metrics *HighPressureComparisonMetrics) {
	var sendWg sync.WaitGroup
	
	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()
			
			for i := 0; i < messagesPerTopic; i++ {
				testMsg := HighPressureTestMessage{
					ID:        fmt.Sprintf("%s-%d", topicName, i),
					Topic:     topicName,
					Sequence:  int64(i),
					Timestamp: time.Now(),
					Data:      fmt.Sprintf("high-pressure-comparison-data-%d", i),
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
	t.Logf("📤 Finished sending high pressure comparison messages")
}

// analyzeHighPressureComparisonResults 分析高压力对比结果
func analyzeHighPressureComparisonResults(t *testing.T, system string, metrics *HighPressureComparisonMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	successRate := float64(metrics.MessagesReceived) / float64(metrics.MessagesSent) * 100
	throughput := float64(metrics.MessagesReceived) / duration.Seconds()
	
	avgSendLatency := float64(metrics.SendLatencySum) / float64(metrics.SendLatencyCount) / 1000.0 // ms
	
	t.Logf("\n🎯 ===== %s High Pressure Comparison Results =====", system)
	t.Logf("⏱️  Test Duration: %v", duration)
	t.Logf("📤 Messages Sent: %d", metrics.MessagesSent)
	t.Logf("📥 Messages Received: %d", metrics.MessagesReceived)
	t.Logf("❌ Send Errors: %d", metrics.SendErrors)
	t.Logf("❌ Process Errors: %d", metrics.ProcessErrors)
	t.Logf("✅ Success Rate: %.2f%%", successRate)
	t.Logf("🚀 Throughput: %.2f msg/s", throughput)
	t.Logf("⚡ Avg Send Latency: %.2f ms", avgSendLatency)
	t.Logf("⚠️ Order Violations: %d", metrics.OrderViolations)
	
	// 🏆 性能评估
	t.Logf("\n🏆 %s Performance Evaluation:", system)
	if successRate >= 95.0 {
		t.Logf("🎉 优秀! %s在高压力下表现卓越!", system)
		t.Logf("   ✅ 成功率: %.2f%%", successRate)
		t.Logf("   ✅ 吞吐量: %.0f msg/s", throughput)
		t.Logf("   ✅ 延迟: %.2f ms", avgSendLatency)
	} else if successRate >= 80.0 {
		t.Logf("⚠️ 良好! %s表现良好", system)
		t.Logf("   ✅ 成功率: %.2f%%", successRate)
	} else {
		t.Logf("❌ %s需要优化，成功率仅为 %.2f%%", system, successRate)
	}
	
	// 基本验证
	assert.Greater(t, metrics.MessagesSent, int64(0), "Should send messages")
	assert.Greater(t, metrics.MessagesReceived, int64(0), "Should receive messages")
	
	t.Logf("✅ %s High Pressure Comparison Test Completed!", system)
}
