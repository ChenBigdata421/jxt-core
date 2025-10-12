package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 🎯 Kafka UnifiedConsumer性能基准测试
// 测试场景：
// - 1个消费者组，1个Kafka连接
// - 订阅10个topic主题
// - 发送端发送envelope格式消息
// - 订阅端根据聚合ID hash到keyed-worker池
// - 确保同一聚合ID的事件严格顺序处理
// - Kafka消息持久化

// BenchmarkConfig 基准测试配置
type BenchmarkConfig struct {
	TopicCount       int           // topic数量
	AggregateCount   int           // 聚合ID数量
	MessagesPerTopic int           // 每个topic的消息数量
	WorkerPoolSize   int           // worker池大小
	TestDuration     time.Duration // 测试持续时间
	BatchSize        int           // 批量发送大小
}

// DefaultBenchmarkConfig 默认基准测试配置
func DefaultBenchmarkConfig() BenchmarkConfig {
	return BenchmarkConfig{
		TopicCount:       10,
		AggregateCount:   100,
		MessagesPerTopic: 1000,
		WorkerPoolSize:   20,
		TestDuration:     30 * time.Second,
		BatchSize:        50,
	}
}

// BenchmarkMetrics 基准测试指标
type BenchmarkMetrics struct {
	// 发送指标
	MessagesSent     int64         // 发送消息总数
	SendErrors       int64         // 发送错误数
	SendLatencySum   int64         // 发送延迟总和(微秒)
	SendLatencyCount int64         // 发送延迟计数
	
	// 接收指标
	MessagesReceived int64         // 接收消息总数
	ProcessErrors    int64         // 处理错误数
	ProcessLatencySum int64        // 处理延迟总和(微秒)
	ProcessLatencyCount int64      // 处理延迟计数
	
	// 顺序性指标
	OrderViolations  int64         // 顺序违反次数
	DuplicateMessages int64        // 重复消息数
	
	// 性能指标
	StartTime        time.Time     // 开始时间
	EndTime          time.Time     // 结束时间
	PeakThroughput   int64         // 峰值吞吐量(msg/s)
	
	// 聚合ID处理统计
	AggregateStats   sync.Map      // map[string]*AggregateProcessingStats
}

// AggregateProcessingStats 聚合ID处理统计
type AggregateProcessingStats struct {
	MessageCount    int64     // 消息数量
	LastSequence    int64     // 最后处理的序列号
	OrderViolations int64     // 顺序违反次数
	FirstMessageTime time.Time // 第一条消息时间
	LastMessageTime  time.Time // 最后一条消息时间
}

// TestEnvelope 测试用的Envelope消息格式
type TestEnvelope struct {
	AggregateID   string                 `json:"aggregateId"`
	EventType     string                 `json:"eventType"`
	Sequence      int64                  `json:"sequence"`
	Timestamp     time.Time              `json:"timestamp"`
	Payload       map[string]interface{} `json:"payload"`
	Metadata      map[string]string      `json:"metadata"`
}

// TestBenchmarkKafkaUnifiedConsumerPerformance 主要的性能基准测试
func TestBenchmarkKafkaUnifiedConsumerPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark test in short mode")
	}

	config := DefaultBenchmarkConfig()
	metrics := &BenchmarkMetrics{
		StartTime: time.Now(),
	}

	// 创建Kafka EventBus
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29092"},
		Producer: ProducerConfig{
			RequiredAcks:   -1, // WaitForAll，幂等性生产者要求
			Compression:    "snappy",
			FlushFrequency: 10 * time.Millisecond,
			FlushMessages:  100,
			Timeout:        30 * time.Second,
			// 程序员控制的性能优化字段
			FlushBytes:      1024 * 1024,     // 1MB
			RetryMax:        3,
			BatchSize:       16 * 1024,       // 16KB
			BufferSize:      32 * 1024 * 1024, // 32MB
			Idempotent:      true,
			MaxMessageBytes: 1024 * 1024,
			PartitionerType: "hash",
			LingerMs:        5 * time.Millisecond,
			CompressionLevel: 6,
			MaxInFlight:     1, // 幂等性生产者要求MaxInFlight=1
		},
		Consumer: ConsumerConfig{
			GroupID:           "benchmark-test-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    30 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			// 程序员控制的性能优化字段
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
		HealthCheckInterval: 30 * time.Second,
		ClientID:           "benchmark-client",
		MetadataRefreshFreq: 10 * time.Minute,
		MetadataRetryMax:   3,
		MetadataRetryBackoff: 250 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:   30 * time.Second,
			ReadTimeout:   30 * time.Second,
			WriteTimeout:  30 * time.Second,
			KeepAlive:     30 * time.Second,
			MaxIdleConns:  10,
			MaxOpenConns:  100,
		},
		Security: SecurityConfig{Enabled: false},
	}

	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err)
	defer eventBus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration+30*time.Second)
	defer cancel()

	// 生成测试topic列表
	topics := make([]string, config.TopicCount)
	for i := 0; i < config.TopicCount; i++ {
		topics[i] = fmt.Sprintf("benchmark-topic-%d", i)
	}

	t.Logf("🚀 开始Kafka UnifiedConsumer性能基准测试")
	t.Logf("📊 测试配置: %d topics, %d aggregates, %d messages/topic, %d workers",
		config.TopicCount, config.AggregateCount, config.MessagesPerTopic, config.WorkerPoolSize)

	// 第一阶段：设置订阅者
	t.Log("📡 阶段1: 设置UnifiedConsumer订阅者...")
	err = setupUnifiedConsumerSubscriptions(ctx, eventBus, topics, config, metrics)
	require.NoError(t, err)

	// 等待订阅者准备就绪
	time.Sleep(2 * time.Second)

	// 第二阶段：发送测试消息
	t.Log("📤 阶段2: 发送测试消息...")
	err = sendBenchmarkMessages(ctx, eventBus, topics, config, metrics)
	require.NoError(t, err)

	// 第三阶段：等待消息处理完成
	t.Log("⏳ 阶段3: 等待消息处理完成...")
	waitForMessageProcessing(ctx, config, metrics, t)

	// 第四阶段：分析结果
	t.Log("📈 阶段4: 分析性能结果...")
	analyzePerformanceResults(config, metrics, t)
}

// setupUnifiedConsumerSubscriptions 设置UnifiedConsumer订阅
func setupUnifiedConsumerSubscriptions(ctx context.Context, eventBus EventBus, topics []string, config BenchmarkConfig, metrics *BenchmarkMetrics) error {
	var wg sync.WaitGroup
	
	// 为每个topic设置订阅处理器
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()
			
			// 创建消息处理器
			handler := createBenchmarkMessageHandler(topicName, config, metrics)
			
			// 使用基本的Subscribe方法（UnifiedConsumer内部会处理Keyed-Worker池）
			err := eventBus.Subscribe(ctx, topicName, handler)
			if err != nil {
				panic(fmt.Sprintf("Failed to subscribe to topic %s: %v", topicName, err))
			}
		}(topic)
	}
	
	wg.Wait()
	return nil
}

// createBenchmarkMessageHandler 创建基准测试消息处理器
func createBenchmarkMessageHandler(topic string, config BenchmarkConfig, metrics *BenchmarkMetrics) MessageHandler {
	return func(ctx context.Context, message []byte) error {
		startTime := time.Now()
		
		// 解析Envelope消息
		var envelope TestEnvelope
		if err := json.Unmarshal(message, &envelope); err != nil {
			atomic.AddInt64(&metrics.ProcessErrors, 1)
			return fmt.Errorf("failed to unmarshal envelope: %w", err)
		}
		
		// 更新接收指标
		atomic.AddInt64(&metrics.MessagesReceived, 1)
		
		// 检查消息顺序性
		checkMessageOrder(envelope, metrics)
		
		// 模拟业务处理（可配置的处理时间）
		processingTime := time.Duration(rand.Intn(5)) * time.Millisecond
		time.Sleep(processingTime)
		
		// 更新处理延迟指标
		latency := time.Since(startTime).Microseconds()
		atomic.AddInt64(&metrics.ProcessLatencySum, latency)
		atomic.AddInt64(&metrics.ProcessLatencyCount, 1)
		
		return nil
	}
}

// checkMessageOrder 检查消息顺序性
func checkMessageOrder(envelope TestEnvelope, metrics *BenchmarkMetrics) {
	statsInterface, _ := metrics.AggregateStats.LoadOrStore(envelope.AggregateID, &AggregateProcessingStats{
		FirstMessageTime: envelope.Timestamp,
	})
	
	stats := statsInterface.(*AggregateProcessingStats)
	
	// 原子操作更新统计
	currentCount := atomic.AddInt64(&stats.MessageCount, 1)
	
	// 检查序列号顺序
	if envelope.Sequence <= stats.LastSequence && currentCount > 1 {
		atomic.AddInt64(&stats.OrderViolations, 1)
		atomic.AddInt64(&metrics.OrderViolations, 1)
	}
	
	stats.LastSequence = envelope.Sequence
	stats.LastMessageTime = envelope.Timestamp
}

// sendBenchmarkMessages 发送基准测试消息
func sendBenchmarkMessages(ctx context.Context, eventBus EventBus, topics []string, config BenchmarkConfig, metrics *BenchmarkMetrics) error {
	var wg sync.WaitGroup

	// 为每个topic启动发送协程
	for topicIndex, topic := range topics {
		wg.Add(1)
		go func(topicName string, tIndex int) {
			defer wg.Done()

			// 为该topic生成消息
			for msgIndex := 0; msgIndex < config.MessagesPerTopic; msgIndex++ {
				// 选择聚合ID（确保分布均匀）
				aggregateID := fmt.Sprintf("aggregate-%d", (tIndex*config.MessagesPerTopic+msgIndex)%config.AggregateCount)

				// 创建Envelope消息
				envelope := TestEnvelope{
					AggregateID: aggregateID,
					EventType:   fmt.Sprintf("TestEvent-%d", msgIndex%5),
					Sequence:    int64(msgIndex),
					Timestamp:   time.Now(),
					Payload: map[string]interface{}{
						"topicIndex":   tIndex,
						"messageIndex": msgIndex,
						"data":         fmt.Sprintf("test-data-%d-%d", tIndex, msgIndex),
						"value":        rand.Intn(1000),
					},
					Metadata: map[string]string{
						"source":    "benchmark-test",
						"topic":     topicName,
						"aggregate": aggregateID,
					},
				}

				// 序列化消息
				messageBytes, err := json.Marshal(envelope)
				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
					continue
				}

				// 发送消息并测量延迟
				sendStart := time.Now()
				err = eventBus.Publish(ctx, topicName, messageBytes)
				sendLatency := time.Since(sendStart).Microseconds()

				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
				} else {
					atomic.AddInt64(&metrics.MessagesSent, 1)
					atomic.AddInt64(&metrics.SendLatencySum, sendLatency)
					atomic.AddInt64(&metrics.SendLatencyCount, 1)
				}

				// 批量发送控制
				if msgIndex%config.BatchSize == 0 {
					time.Sleep(1 * time.Millisecond) // 小延迟避免过载
				}
			}
		}(topic, topicIndex)
	}

	wg.Wait()
	return nil
}

// waitForMessageProcessing 等待消息处理完成
func waitForMessageProcessing(ctx context.Context, config BenchmarkConfig, metrics *BenchmarkMetrics, t *testing.T) {
	expectedMessages := int64(config.TopicCount * config.MessagesPerTopic)
	timeout := time.After(60 * time.Second) // 最大等待时间
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️  等待超时，已处理 %d/%d 消息",
				atomic.LoadInt64(&metrics.MessagesReceived), expectedMessages)
			return
		case <-ticker.C:
			received := atomic.LoadInt64(&metrics.MessagesReceived)
			t.Logf("📊 处理进度: %d/%d (%.1f%%)",
				received, expectedMessages, float64(received)/float64(expectedMessages)*100)

			if received >= expectedMessages {
				t.Log("✅ 所有消息处理完成")
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// analyzePerformanceResults 分析性能结果
func analyzePerformanceResults(config BenchmarkConfig, metrics *BenchmarkMetrics, t *testing.T) {
	metrics.EndTime = time.Now()
	duration := metrics.EndTime.Sub(metrics.StartTime)

	// 基本统计
	messagesSent := atomic.LoadInt64(&metrics.MessagesSent)
	messagesReceived := atomic.LoadInt64(&metrics.MessagesReceived)
	sendErrors := atomic.LoadInt64(&metrics.SendErrors)
	processErrors := atomic.LoadInt64(&metrics.ProcessErrors)
	orderViolations := atomic.LoadInt64(&metrics.OrderViolations)

	// 计算吞吐量
	sendThroughput := float64(messagesSent) / duration.Seconds()
	receiveThroughput := float64(messagesReceived) / duration.Seconds()

	// 计算平均延迟
	var avgSendLatency, avgProcessLatency float64
	if sendLatencyCount := atomic.LoadInt64(&metrics.SendLatencyCount); sendLatencyCount > 0 {
		avgSendLatency = float64(atomic.LoadInt64(&metrics.SendLatencySum)) / float64(sendLatencyCount)
	}
	if processLatencyCount := atomic.LoadInt64(&metrics.ProcessLatencyCount); processLatencyCount > 0 {
		avgProcessLatency = float64(atomic.LoadInt64(&metrics.ProcessLatencySum)) / float64(processLatencyCount)
	}

	// 输出详细结果
	t.Log("\n" + strings.Repeat("=", 80))
	t.Log("🎯 Kafka UnifiedConsumer 性能基准测试结果")
	t.Log(strings.Repeat("=", 80))

	t.Logf("📊 基本统计:")
	t.Logf("   测试持续时间: %.2f 秒", duration.Seconds())
	t.Logf("   发送消息数: %d", messagesSent)
	t.Logf("   接收消息数: %d", messagesReceived)
	t.Logf("   消息成功率: %.2f%%", float64(messagesReceived)/float64(messagesSent)*100)

	t.Logf("\n🚀 性能指标:")
	t.Logf("   发送吞吐量: %.2f msg/s", sendThroughput)
	t.Logf("   接收吞吐量: %.2f msg/s", receiveThroughput)
	t.Logf("   平均发送延迟: %.2f μs", avgSendLatency)
	t.Logf("   平均处理延迟: %.2f μs", avgProcessLatency)

	t.Logf("\n❌ 错误统计:")
	t.Logf("   发送错误: %d", sendErrors)
	t.Logf("   处理错误: %d", processErrors)
	t.Logf("   顺序违反: %d", orderViolations)

	// 聚合ID处理统计
	analyzeAggregateProcessing(metrics, t)

	// 性能断言
	assert.True(t, sendThroughput > 1000, "发送吞吐量应该 > 1000 msg/s")
	assert.True(t, receiveThroughput > 1000, "接收吞吐量应该 > 1000 msg/s")
	assert.Equal(t, int64(0), orderViolations, "不应该有顺序违反")
	assert.True(t, float64(messagesReceived)/float64(messagesSent) > 0.99, "消息成功率应该 > 99%")

	t.Log(strings.Repeat("=", 80))
}

// analyzeAggregateProcessing 分析聚合ID处理情况
func analyzeAggregateProcessing(metrics *BenchmarkMetrics, t *testing.T) {
	t.Logf("\n🔍 聚合ID处理分析:")

	var totalAggregates int64
	var totalOrderViolations int64
	var minMessages, maxMessages int64 = 999999, 0

	metrics.AggregateStats.Range(func(key, value interface{}) bool {
		aggregateID := key.(string)
		stats := value.(*AggregateProcessingStats)

		messageCount := atomic.LoadInt64(&stats.MessageCount)
		violations := atomic.LoadInt64(&stats.OrderViolations)

		totalAggregates++
		totalOrderViolations += violations

		if messageCount < minMessages {
			minMessages = messageCount
		}
		if messageCount > maxMessages {
			maxMessages = messageCount
		}

		// 输出前5个聚合的详细信息
		if totalAggregates <= 5 {
			duration := stats.LastMessageTime.Sub(stats.FirstMessageTime)
			t.Logf("   聚合 %s: %d 消息, %d 违反, 处理时长 %.2f ms",
				aggregateID, messageCount, violations, float64(duration.Nanoseconds())/1e6)
		}

		return true
	})

	t.Logf("   总聚合数: %d", totalAggregates)
	t.Logf("   消息分布: %d - %d (最少-最多)", minMessages, maxMessages)
	t.Logf("   总顺序违反: %d", totalOrderViolations)
}
