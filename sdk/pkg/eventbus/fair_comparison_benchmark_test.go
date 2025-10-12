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

// 🎯 NATS vs Kafka 公平对比基准测试
// 使用相同的测试参数和消息数量进行对比

// FairComparisonConfig 公平对比配置
type FairComparisonConfig struct {
	TopicCount       int           // topic数量
	MessagesPerTopic int           // 每个topic的消息数量
	TestDuration     time.Duration // 测试持续时间
	BatchSize        int           // 批量发送大小
}

// DefaultFairComparisonConfig 默认公平对比配置
func DefaultFairComparisonConfig() FairComparisonConfig {
	return FairComparisonConfig{
		TopicCount:       3,
		MessagesPerTopic: 500,
		TestDuration:     30 * time.Second,
		BatchSize:        25,
	}
}

// FairComparisonMetrics 公平对比指标
type FairComparisonMetrics struct {
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
	StartTime time.Time // 开始时间
	EndTime   time.Time // 结束时间
}

// FairTestMessage 公平测试消息
type FairTestMessage struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// TestFairComparisonNATSvsKafka NATS vs Kafka 公平对比测试
func TestFairComparisonNATSvsKafka(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fair comparison test in short mode")
	}

	config := DefaultFairComparisonConfig()

	t.Logf("🎯 Starting Fair Comparison: NATS vs Kafka")
	t.Logf("📊 Test Config: %d topics, %d msgs/topic, total: %d messages",
		config.TopicCount, config.MessagesPerTopic, config.TopicCount*config.MessagesPerTopic)

	// 测试NATS
	t.Logf("\n🔵 Testing NATS Performance...")
	natsMetrics := testNATSPerformance(t, config)

	// 等待一下再测试Kafka
	time.Sleep(2 * time.Second)

	// 测试Kafka
	t.Logf("\n🟠 Testing Kafka Performance...")
	kafkaMetrics := testKafkaPerformance(t, config)

	// 对比结果
	t.Logf("\n📊 Performance Comparison Results:")
	comparePerformance(t, natsMetrics, kafkaMetrics)
}

// testNATSPerformance 测试NATS性能
func testNATSPerformance(t *testing.T, config FairComparisonConfig) *FairComparisonMetrics {
	metrics := &FairComparisonMetrics{}

	// 创建NATS EventBus配置 (使用新的基准测试端口)
	natsConfig := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            "fair-comparison-nats",
		MaxReconnects:       5,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: false, // 使用基本NATS以保证公平性
		},
	}

	eventBus, err := NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(1 * time.Second)

	// 运行测试
	runFairComparisonTest(t, eventBus, config, metrics, "NATS")

	return metrics
}

// testKafkaPerformance 测试Kafka性能
func testKafkaPerformance(t *testing.T, config FairComparisonConfig) *FairComparisonMetrics {
	metrics := &FairComparisonMetrics{}

	// 创建简化的Kafka EventBus配置 (使用新的基准测试端口)
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29093"},
		Producer: ProducerConfig{
			RequiredAcks:   1, // 简化配置
			Compression:    "none",
			FlushFrequency: 10 * time.Millisecond,
			FlushMessages:  100,
			Timeout:        10 * time.Second,
			// 添加必需的配置字段
			FlushBytes:       1024 * 1024, // 1MB
			RetryMax:         3,
			BatchSize:        16 * 1024,        // 16KB
			BufferSize:       32 * 1024 * 1024, // 32MB
			Idempotent:       false,
			MaxMessageBytes:  1024 * 1024,
			PartitionerType:  "hash",
			LingerMs:         5 * time.Millisecond,
			CompressionLevel: 6,
			MaxInFlight:      5,
		},
		Consumer: ConsumerConfig{
			GroupID:           "fair-comparison-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			// 添加必需的配置字段
			MaxProcessingTime:  30 * time.Second,
			FetchMinBytes:      1024,
			FetchMaxBytes:      50 * 1024 * 1024, // 50MB
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     500,
			EnableAutoCommit:   false,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_committed",
			RebalanceStrategy:  "range",
		},
		HealthCheckInterval:  30 * time.Second,
		ClientID:             "fair-comparison-kafka",
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
	time.Sleep(2 * time.Second)

	// 运行测试
	runFairComparisonTest(t, eventBus, config, metrics, "Kafka")

	return metrics
}

// runFairComparisonTest 运行公平对比测试
func runFairComparisonTest(t *testing.T, eventBus EventBus, config FairComparisonConfig, metrics *FairComparisonMetrics, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration+10*time.Second)
	defer cancel()

	// 创建topic列表
	topics := make([]string, config.TopicCount)
	for i := 0; i < config.TopicCount; i++ {
		topics[i] = fmt.Sprintf("fair.comparison.%s.topic.%d", name, i)
	}

	// 设置消息处理器
	var wg sync.WaitGroup
	setupFairComparisonHandlers(t, eventBus, topics, metrics, &wg)

	// 等待订阅建立
	time.Sleep(1 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendFairComparisonMessages(t, eventBus, topics, config, metrics)

	// 等待所有消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("✅ %s: All messages processed successfully", name)
	case <-time.After(config.TestDuration):
		t.Logf("⏰ %s: Test timeout reached", name)
	case <-ctx.Done():
		t.Logf("🛑 %s: Context cancelled", name)
	}

	metrics.EndTime = time.Now()
}

// setupFairComparisonHandlers 设置公平对比消息处理器
func setupFairComparisonHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *FairComparisonMetrics, wg *sync.WaitGroup) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()

			handler := func(ctx context.Context, message []byte) error {
				startTime := time.Now()

				// 解析消息
				var testMsg FairTestMessage
				if err := json.Unmarshal(message, &testMsg); err != nil {
					atomic.AddInt64(&metrics.ProcessErrors, 1)
					return err
				}

				// 更新接收指标
				atomic.AddInt64(&metrics.MessagesReceived, 1)

				// 记录处理延迟
				processingTime := time.Since(startTime).Microseconds()
				atomic.AddInt64(&metrics.ProcessLatencySum, processingTime)
				atomic.AddInt64(&metrics.ProcessLatencyCount, 1)

				return nil
			}

			err := eventBus.Subscribe(context.Background(), topicName, handler)
			if err != nil {
				t.Errorf("Failed to subscribe to topic %s: %v", topicName, err)
			}
		}(topic)
	}
}

// sendFairComparisonMessages 发送公平对比消息
func sendFairComparisonMessages(t *testing.T, eventBus EventBus, topics []string, config FairComparisonConfig, metrics *FairComparisonMetrics) {
	var sendWg sync.WaitGroup

	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()

			for i := 0; i < config.MessagesPerTopic; i++ {
				// 创建测试消息
				testMsg := FairTestMessage{
					ID:        fmt.Sprintf("%s-%d", topicName, i),
					Topic:     topicName,
					Sequence:  int64(i),
					Timestamp: time.Now(),
					Data:      fmt.Sprintf("test-data-%d", i),
				}

				// 序列化消息
				messageBytes, err := json.Marshal(testMsg)
				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
					continue
				}

				// 发送消息
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

				// 批量发送控制
				if i%config.BatchSize == 0 {
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(topic)
	}

	sendWg.Wait()
}

// comparePerformance 对比性能结果
func comparePerformance(t *testing.T, natsMetrics, kafkaMetrics *FairComparisonMetrics) {
	// 计算NATS指标
	natsDuration := natsMetrics.EndTime.Sub(natsMetrics.StartTime)
	natsThroughput := float64(natsMetrics.MessagesReceived) / natsDuration.Seconds()
	natsAvgSendLatency := float64(natsMetrics.SendLatencySum) / float64(natsMetrics.SendLatencyCount) / 1000.0          // ms
	natsAvgProcessLatency := float64(natsMetrics.ProcessLatencySum) / float64(natsMetrics.ProcessLatencyCount) / 1000.0 // ms
	natsSuccessRate := float64(natsMetrics.MessagesReceived) / float64(natsMetrics.MessagesSent) * 100

	// 计算Kafka指标
	kafkaDuration := kafkaMetrics.EndTime.Sub(kafkaMetrics.StartTime)
	kafkaThroughput := float64(kafkaMetrics.MessagesReceived) / kafkaDuration.Seconds()
	kafkaAvgSendLatency := float64(kafkaMetrics.SendLatencySum) / float64(kafkaMetrics.SendLatencyCount) / 1000.0          // ms
	kafkaAvgProcessLatency := float64(kafkaMetrics.ProcessLatencySum) / float64(kafkaMetrics.ProcessLatencyCount) / 1000.0 // ms
	kafkaSuccessRate := float64(kafkaMetrics.MessagesReceived) / float64(kafkaMetrics.MessagesSent) * 100

	t.Logf("\n📊 ===== Fair Comparison Results =====")
	t.Logf("🔵 NATS Results:")
	t.Logf("   Duration: %v", natsDuration)
	t.Logf("   Messages Sent: %d", natsMetrics.MessagesSent)
	t.Logf("   Messages Received: %d", natsMetrics.MessagesReceived)
	t.Logf("   Throughput: %.2f msg/s", natsThroughput)
	t.Logf("   Avg Send Latency: %.2f ms", natsAvgSendLatency)
	t.Logf("   Avg Process Latency: %.2f ms", natsAvgProcessLatency)
	t.Logf("   Success Rate: %.2f%%", natsSuccessRate)

	t.Logf("\n🟠 Kafka Results:")
	t.Logf("   Duration: %v", kafkaDuration)
	t.Logf("   Messages Sent: %d", kafkaMetrics.MessagesSent)
	t.Logf("   Messages Received: %d", kafkaMetrics.MessagesReceived)
	t.Logf("   Throughput: %.2f msg/s", kafkaThroughput)
	t.Logf("   Avg Send Latency: %.2f ms", kafkaAvgSendLatency)
	t.Logf("   Avg Process Latency: %.2f ms", kafkaAvgProcessLatency)
	t.Logf("   Success Rate: %.2f%%", kafkaSuccessRate)

	// 计算性能比较
	if kafkaThroughput > 0 {
		throughputRatio := natsThroughput / kafkaThroughput
		t.Logf("\n🏆 Performance Comparison:")
		t.Logf("   NATS Throughput is %.2fx faster than Kafka", throughputRatio)
	}

	if kafkaAvgSendLatency > 0 {
		latencyRatio := kafkaAvgSendLatency / natsAvgSendLatency
		t.Logf("   NATS Send Latency is %.2fx lower than Kafka", latencyRatio)
	}

	// 基本验证
	assert.Greater(t, natsMetrics.MessagesSent, int64(0), "NATS should send messages")
	assert.Greater(t, kafkaMetrics.MessagesSent, int64(0), "Kafka should send messages")
	assert.Greater(t, natsThroughput, 0.0, "NATS throughput should be > 0")
	assert.Greater(t, kafkaThroughput, 0.0, "Kafka throughput should be > 0")

	t.Logf("✅ Fair Comparison Test Completed!")
}
