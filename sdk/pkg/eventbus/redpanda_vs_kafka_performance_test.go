package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// RedPandaTestMessage RedPanda测试消息
type RedPandaTestMessage struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
	System    string    `json:"system"` // "redpanda" or "kafka"
}

// RedPandaTestMetrics RedPanda测试指标
type RedPandaTestMetrics struct {
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

// TestRedPandaLowPressure RedPanda低压力测试
func TestRedPandaLowPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping RedPanda low pressure test in short mode")
	}

	metrics := &RedPandaTestMetrics{StartTime: time.Now()}

	// 🔴 RedPanda配置 (使用Kafka兼容API)
	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"}, // RedPanda端口
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   1 * time.Millisecond, // 更激进的配置
			FlushMessages:    1,
			Timeout:          10 * time.Second,
			FlushBytes:       16 * 1024,
			RetryMax:         2,
			BatchSize:        1024,
			BufferSize:       4 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  64 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         1 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      2,
		},
		Consumer: ConsumerConfig{
			GroupID:            "redpanda-low-pressure-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  45 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      2 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     25,
			EnableAutoCommit:   true,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		HealthCheckInterval:  30 * time.Second,
		ClientID:             "redpanda-low-pressure-client",
		MetadataRefreshFreq:  5 * time.Minute,
		MetadataRetryMax:     2,
		MetadataRetryBackoff: 100 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			KeepAlive:    10 * time.Second,
			MaxIdleConns: 5,
			MaxOpenConns: 50,
		},
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	require.NoError(t, err, "Failed to create RedPanda EventBus")
	defer eventBus.Close()

	time.Sleep(3 * time.Second)

	t.Logf("🔴 Starting RedPanda Low Pressure Test")

	// 低压力：2个topic，每个300条消息，总计600条
	runRedPandaTest(t, eventBus, "RedPanda", 2, 300, metrics)
	analyzeRedPandaResults(t, "RedPanda", "Low", metrics)
}

// TestKafkaLowPressureComparison Kafka低压力对比测试
func TestKafkaLowPressureComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka low pressure comparison test in short mode")
	}

	metrics := &RedPandaTestMetrics{StartTime: time.Now()}

	// 🟠 Kafka配置 (相同的优化配置)
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29093"}, // Kafka端口
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   1 * time.Millisecond,
			FlushMessages:    1,
			Timeout:          10 * time.Second,
			FlushBytes:       16 * 1024,
			RetryMax:         2,
			BatchSize:        1024,
			BufferSize:       4 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  64 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         1 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      2,
		},
		Consumer: ConsumerConfig{
			GroupID:            "kafka-comparison-low-pressure-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  45 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      2 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     25,
			EnableAutoCommit:   true,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		HealthCheckInterval:  30 * time.Second,
		ClientID:             "kafka-comparison-low-pressure-client",
		MetadataRefreshFreq:  5 * time.Minute,
		MetadataRetryMax:     2,
		MetadataRetryBackoff: 100 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			KeepAlive:    10 * time.Second,
			MaxIdleConns: 5,
			MaxOpenConns: 50,
		},
	}

	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err, "Failed to create Kafka EventBus")
	defer eventBus.Close()

	time.Sleep(3 * time.Second)

	t.Logf("🟠 Starting Kafka Low Pressure Comparison Test")

	// 低压力：2个topic，每个300条消息，总计600条
	runRedPandaTest(t, eventBus, "Kafka", 2, 300, metrics)
	analyzeRedPandaResults(t, "Kafka", "Low", metrics)
}

// TestRedPandaMediumPressure RedPanda中压力测试
func TestRedPandaMediumPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping RedPanda medium pressure test in short mode")
	}

	metrics := &RedPandaTestMetrics{StartTime: time.Now()}

	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   1 * time.Millisecond,
			FlushMessages:    5,
			Timeout:          15 * time.Second,
			FlushBytes:       32 * 1024,
			RetryMax:         2,
			BatchSize:        2048,
			BufferSize:       8 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  128 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         1 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      3,
		},
		Consumer: ConsumerConfig{
			GroupID:            "redpanda-medium-pressure-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     45 * time.Second,
			HeartbeatInterval:  15 * time.Second,
			MaxProcessingTime:  60 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      5 * 1024 * 1024,
			FetchMaxWait:       1 * time.Second,
			MaxPollRecords:     50,
			EnableAutoCommit:   true,
			AutoCommitInterval: 10 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		HealthCheckInterval:  45 * time.Second,
		ClientID:             "redpanda-medium-pressure-client",
		MetadataRefreshFreq:  10 * time.Minute,
		MetadataRetryMax:     2,
		MetadataRetryBackoff: 200 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:  15 * time.Second,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			KeepAlive:    15 * time.Second,
			MaxIdleConns: 10,
			MaxOpenConns: 100,
		},
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	require.NoError(t, err, "Failed to create RedPanda EventBus")
	defer eventBus.Close()

	time.Sleep(5 * time.Second)

	t.Logf("🔴 Starting RedPanda Medium Pressure Test")

	// 中压力：3个topic，每个1000条消息，总计3,000条
	runRedPandaTest(t, eventBus, "RedPanda", 3, 1000, metrics)
	analyzeRedPandaResults(t, "RedPanda", "Medium", metrics)
}

// TestRedPandaHighPressure RedPanda高压力测试
func TestRedPandaHighPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping RedPanda high pressure test in short mode")
	}

	metrics := &RedPandaTestMetrics{StartTime: time.Now()}

	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   1 * time.Millisecond,
			FlushMessages:    10,
			Timeout:          30 * time.Second,
			FlushBytes:       64 * 1024,
			RetryMax:         3,
			BatchSize:        4096,
			BufferSize:       16 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  256 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         1 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      5,
		},
		Consumer: ConsumerConfig{
			GroupID:            "redpanda-high-pressure-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     60 * time.Second,
			HeartbeatInterval:  20 * time.Second,
			MaxProcessingTime:  90 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      10 * 1024 * 1024,
			FetchMaxWait:       2 * time.Second,
			MaxPollRecords:     100,
			EnableAutoCommit:   true,
			AutoCommitInterval: 15 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		HealthCheckInterval:  60 * time.Second,
		ClientID:             "redpanda-high-pressure-client",
		MetadataRefreshFreq:  15 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 500 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:  30 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			KeepAlive:    30 * time.Second,
			MaxIdleConns: 20,
			MaxOpenConns: 200,
		},
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	require.NoError(t, err, "Failed to create RedPanda EventBus")
	defer eventBus.Close()

	time.Sleep(8 * time.Second)

	t.Logf("🔴 Starting RedPanda High Pressure Test")

	// 高压力：5个topic，每个2000条消息，总计10,000条
	runRedPandaTest(t, eventBus, "RedPanda", 5, 2000, metrics)
	analyzeRedPandaResults(t, "RedPanda", "High", metrics)
}

// runRedPandaTest 运行RedPanda测试
func runRedPandaTest(t *testing.T, eventBus EventBus, system string, topicCount, messagesPerTopic int, metrics *RedPandaTestMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // 2分钟超时
	defer cancel()

	// 生成topic列表
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("redpanda.test.topic.%d", i+1)
	}

	totalMessages := topicCount * messagesPerTopic
	t.Logf("📊 %s Test Config: %d topics, %d msgs/topic, total: %d messages",
		system, topicCount, messagesPerTopic, totalMessages)

	// 设置消息处理器
	var wg sync.WaitGroup
	setupRedPandaHandlers(t, eventBus, topics, metrics, &wg, system)

	// 等待订阅建立
	t.Logf("⏳ Waiting for %s subscriptions to stabilize...", system)
	time.Sleep(2 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendRedPandaMessages(t, eventBus, topics, messagesPerTopic, metrics, system)

	// 等待消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("✅ All %s messages processed successfully", system)
	case <-time.After(90 * time.Second): // 1.5分钟等待
		t.Logf("⏰ %s test timeout reached", system)
	case <-ctx.Done():
		t.Logf("🛑 %s context cancelled", system)
	}

	metrics.EndTime = time.Now()
}

// setupRedPandaHandlers 设置RedPanda处理器
func setupRedPandaHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *RedPandaTestMetrics, wg *sync.WaitGroup, system string) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()

			var lastSequence int64 = -1

			handler := func(ctx context.Context, message []byte) error {
				// 解析消息
				var testMsg RedPandaTestMessage
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

// sendRedPandaMessages 发送RedPanda消息
func sendRedPandaMessages(t *testing.T, eventBus EventBus, topics []string, messagesPerTopic int, metrics *RedPandaTestMetrics, system string) {
	var sendWg sync.WaitGroup

	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()

			for i := 0; i < messagesPerTopic; i++ {
				testMsg := RedPandaTestMessage{
					ID:        fmt.Sprintf("%s-%s-%d", system, topicName, i),
					Topic:     topicName,
					Sequence:  int64(i),
					Timestamp: time.Now(),
					Data:      fmt.Sprintf("%s-performance-test-data-%d", system, i),
					System:    system,
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

				// 控制发送速率：每秒约1000条消息
				time.Sleep(1 * time.Millisecond)
			}
		}(topic)
	}

	sendWg.Wait()
	t.Logf("📤 Finished sending %s messages", system)
}

// analyzeRedPandaResults 分析RedPanda结果
func analyzeRedPandaResults(t *testing.T, system, pressure string, metrics *RedPandaTestMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	successRate := float64(metrics.MessagesReceived) / float64(metrics.MessagesSent) * 100
	throughput := float64(metrics.MessagesReceived) / duration.Seconds()

	avgSendLatency := float64(0)
	if metrics.SendLatencyCount > 0 {
		avgSendLatency = float64(metrics.SendLatencySum) / float64(metrics.SendLatencyCount) / 1000.0 // ms
	}

	t.Logf("\n🎯 ===== %s %s Pressure Performance Results =====", system, pressure)
	t.Logf("⏱️  Test Duration: %v", duration)
	t.Logf("📤 Messages Sent: %d", metrics.MessagesSent)
	t.Logf("📥 Messages Received: %d", metrics.MessagesReceived)
	t.Logf("❌ Send Errors: %d", metrics.SendErrors)
	t.Logf("❌ Process Errors: %d", metrics.ProcessErrors)
	t.Logf("✅ Success Rate: %.2f%%", successRate)
	t.Logf("🚀 Throughput: %.2f msg/s", throughput)
	t.Logf("⚡ Avg Send Latency: %.3f ms", avgSendLatency)
	t.Logf("⚠️ Order Violations: %d", metrics.OrderViolations)

	// 🏆 性能评估
	t.Logf("\n🏆 %s %s Pressure Performance Evaluation:", system, pressure)
	if successRate >= 95.0 {
		t.Logf("🎉 优秀! %s在%s压力下表现卓越!", system, pressure)
		t.Logf("   ✅ 成功率: %.2f%%", successRate)
		t.Logf("   ✅ 吞吐量: %.0f msg/s", throughput)
		t.Logf("   ✅ 延迟: %.3f ms", avgSendLatency)
	} else if successRate >= 80.0 {
		t.Logf("⚠️ 良好! %s在%s压力下表现良好", system, pressure)
		t.Logf("   ✅ 成功率: %.2f%%", successRate)
	} else if successRate >= 50.0 {
		t.Logf("🔶 一般! %s在%s压力下表现一般", system, pressure)
		t.Logf("   ⚠️ 成功率: %.2f%%", successRate)
	} else {
		t.Logf("❌ 差! %s在%s压力下需要重大优化", system, pressure)
		t.Logf("   ❌ 成功率仅为: %.2f%%", successRate)
	}

	t.Logf("✅ %s %s Pressure Test Completed!", system, pressure)
}
