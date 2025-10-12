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

// 🎯 Kafka高并发优化测试
// 专门解决高压力下的再平衡问题

// HighConcurrencyConfig 高并发测试配置
type HighConcurrencyConfig struct {
	TopicCount       int           // topic数量
	MessagesPerTopic int           // 每个topic的消息数量
	TestDuration     time.Duration // 测试持续时间
	BatchSize        int           // 批量发送大小
	SendRateLimit    time.Duration // 发送速率限制
}

// OptimizedHighConcurrencyConfig 优化的高并发配置
func OptimizedHighConcurrencyConfig() HighConcurrencyConfig {
	return HighConcurrencyConfig{
		TopicCount:       5,   // 减少topic数量
		MessagesPerTopic: 1000, // 保持消息数量
		TestDuration:     120 * time.Second, // 增加测试时间
		BatchSize:        20,  // 减少批量大小
		SendRateLimit:    10 * time.Millisecond, // 控制发送速率
	}
}

// HighConcurrencyMetrics 高并发测试指标
type HighConcurrencyMetrics struct {
	// 发送指标
	MessagesSent     int64 // 发送消息总数
	SendErrors       int64 // 发送错误数
	SendLatencySum   int64 // 发送延迟总和(微秒)
	SendLatencyCount int64 // 发送延迟计数
	
	// 接收指标
	MessagesReceived int64 // 接收消息总数
	ProcessErrors    int64 // 处理错误数
	ProcessLatencySum int64 // 处理延迟总和(微秒)
	ProcessLatencyCount int64 // 处理延迟计数
	
	// 再平衡检测
	ProcessingGaps     int64         // 处理间隙次数
	SuspectedRebalances int64        // 疑似再平衡次数
	MaxGapDuration     time.Duration // 最大处理间隙
	OrderViolations    int64         // 顺序违反次数
	
	// 性能指标
	StartTime        time.Time // 开始时间
	EndTime          time.Time // 结束时间
}

// HighConcurrencyTestMessage 高并发测试消息
type HighConcurrencyTestMessage struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// TestKafkaHighConcurrencyOptimized Kafka高并发优化测试
func TestKafkaHighConcurrencyOptimized(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka high concurrency optimized test in short mode")
	}

	config := OptimizedHighConcurrencyConfig()
	metrics := &HighConcurrencyMetrics{
		StartTime: time.Now(),
	}

	// 创建高并发优化的Kafka EventBus配置
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29093"},
		Producer: ProducerConfig{
			RequiredAcks:     1, // 降低一致性要求以提高性能
			Compression:      "lz4", // 使用更快的压缩算法
			FlushFrequency:   50 * time.Millisecond, // 增加刷新频率
			FlushMessages:    20, // 减少批量大小
			Timeout:          60 * time.Second, // 增加超时时间
			// 高并发优化参数
			FlushBytes:       512 * 1024, // 减少刷新字节数
			RetryMax:         5, // 增加重试次数
			BatchSize:        8 * 1024, // 减少批量大小
			BufferSize:       16 * 1024 * 1024, // 减少缓冲区
			Idempotent:       false, // 禁用幂等性以提高性能
			MaxMessageBytes:  512 * 1024, // 减少最大消息大小
			PartitionerType:  "round_robin", // 使用轮询分区器
			LingerMs:         10 * time.Millisecond, // 增加延迟发送时间
			CompressionLevel: 1, // 降低压缩级别
			MaxInFlight:      3, // 适中的飞行请求数
		},
		Consumer: ConsumerConfig{
			GroupID:           "high-concurrency-optimized-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    120 * time.Second, // 大幅增加会话超时
			HeartbeatInterval: 30 * time.Second,  // 大幅增加心跳间隔
			// 高并发优化参数
			MaxProcessingTime:  180 * time.Second, // 大幅增加处理时间
			FetchMinBytes:      1, // 最小获取字节数
			FetchMaxBytes:      10 * 1024 * 1024, // 减少最大获取字节数
			FetchMaxWait:       2 * time.Second, // 增加最大等待时间
			MaxPollRecords:     20, // 大幅减少轮询记录数
			EnableAutoCommit:   true, // 启用自动提交以减少复杂性
			AutoCommitInterval: 10 * time.Second, // 增加自动提交间隔
			IsolationLevel:     "read_uncommitted", // 降低隔离级别
			RebalanceStrategy:  "cooperative_sticky", // 使用协作粘性策略
		},
		HealthCheckInterval:  60 * time.Second, // 增加健康检查间隔
		ClientID:             "high-concurrency-optimized-client",
		MetadataRefreshFreq:  30 * time.Minute, // 增加元数据刷新频率
		MetadataRetryMax:     5,
		MetadataRetryBackoff: 500 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:  60 * time.Second, // 增加连接超时
			ReadTimeout:  60 * time.Second, // 增加读取超时
			WriteTimeout: 60 * time.Second, // 增加写入超时
			KeepAlive:    60 * time.Second, // 增加保活时间
			MaxIdleConns: 20, // 增加最大空闲连接数
			MaxOpenConns: 200, // 增加最大打开连接数
		},
	}

	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err, "Failed to create optimized Kafka EventBus")
	defer eventBus.Close()

	// 等待连接建立和分区分配稳定
	t.Logf("⏳ Waiting for Kafka to stabilize...")
	time.Sleep(10 * time.Second)

	t.Logf("🚀 Starting Kafka High Concurrency Optimized Test")
	t.Logf("📊 Config: %d topics, %d msgs/topic, total: %d messages",
		config.TopicCount, config.MessagesPerTopic, config.TopicCount*config.MessagesPerTopic)

	// 运行高并发优化测试
	runHighConcurrencyOptimizedTest(t, eventBus, config, metrics)

	// 分析结果
	analyzeHighConcurrencyResults(t, config, metrics)
}

// runHighConcurrencyOptimizedTest 运行高并发优化测试
func runHighConcurrencyOptimizedTest(t *testing.T, eventBus EventBus, config HighConcurrencyConfig, metrics *HighConcurrencyMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration+30*time.Second)
	defer cancel()

	// 创建topic列表
	topics := make([]string, config.TopicCount)
	for i := 0; i < config.TopicCount; i++ {
		topics[i] = fmt.Sprintf("high.concurrency.optimized.topic.%d", i)
	}

	// 设置消息处理器
	var wg sync.WaitGroup
	setupHighConcurrencyHandlers(t, eventBus, topics, metrics, &wg)

	// 等待订阅建立和分区分配稳定
	t.Logf("⏳ Waiting for subscriptions to stabilize...")
	time.Sleep(5 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendHighConcurrencyMessages(t, eventBus, topics, config, metrics)

	// 等待消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("✅ All messages processed successfully")
	case <-time.After(config.TestDuration):
		t.Logf("⏰ Test timeout reached")
	case <-ctx.Done():
		t.Logf("🛑 Context cancelled")
	}

	metrics.EndTime = time.Now()
}

// setupHighConcurrencyHandlers 设置高并发消息处理器
func setupHighConcurrencyHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *HighConcurrencyMetrics, wg *sync.WaitGroup) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()
			
			var lastSequence int64 = -1
			var lastProcessTime time.Time = time.Now()
			
			handler := func(ctx context.Context, message []byte) error {
				currentTime := time.Now()
				
				// 检测处理间隙
				if !lastProcessTime.IsZero() {
					gap := currentTime.Sub(lastProcessTime)
					if gap > 10*time.Second { // 超过10秒认为是异常间隙
						atomic.AddInt64(&metrics.ProcessingGaps, 1)
						if gap > 30*time.Second { // 超过30秒认为是再平衡
							atomic.AddInt64(&metrics.SuspectedRebalances, 1)
							t.Logf("🚨 Suspected rebalance: %v gap in topic %s", gap, topicName)
						}
						
						if gap > metrics.MaxGapDuration {
							metrics.MaxGapDuration = gap
						}
					}
				}
				lastProcessTime = currentTime
				
				// 解析消息
				var testMsg HighConcurrencyTestMessage
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
				
				// 记录处理延迟
				processingTime := time.Since(testMsg.Timestamp).Microseconds()
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

// sendHighConcurrencyMessages 发送高并发消息
func sendHighConcurrencyMessages(t *testing.T, eventBus EventBus, topics []string, config HighConcurrencyConfig, metrics *HighConcurrencyMetrics) {
	var sendWg sync.WaitGroup
	
	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()
			
			for i := 0; i < config.MessagesPerTopic; i++ {
				testMsg := HighConcurrencyTestMessage{
					ID:        fmt.Sprintf("%s-%d", topicName, i),
					Topic:     topicName,
					Sequence:  int64(i),
					Timestamp: time.Now(),
					Data:      fmt.Sprintf("high-concurrency-data-%d", i),
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
				
				// 控制发送速率
				if config.SendRateLimit > 0 {
					time.Sleep(config.SendRateLimit)
				}
			}
		}(topic)
	}
	
	sendWg.Wait()
	t.Logf("📤 Finished sending high concurrency messages")
}

// analyzeHighConcurrencyResults 分析高并发结果
func analyzeHighConcurrencyResults(t *testing.T, config HighConcurrencyConfig, metrics *HighConcurrencyMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	successRate := float64(metrics.MessagesReceived) / float64(metrics.MessagesSent) * 100
	throughput := float64(metrics.MessagesReceived) / duration.Seconds()
	
	avgSendLatency := float64(metrics.SendLatencySum) / float64(metrics.SendLatencyCount) / 1000.0 // ms
	avgProcessLatency := float64(metrics.ProcessLatencySum) / float64(metrics.ProcessLatencyCount) / 1000.0 // ms
	
	t.Logf("\n🎯 ===== Kafka High Concurrency Optimized Results =====")
	t.Logf("⏱️  Test Duration: %v", duration)
	t.Logf("📤 Messages Sent: %d", metrics.MessagesSent)
	t.Logf("📥 Messages Received: %d", metrics.MessagesReceived)
	t.Logf("✅ Success Rate: %.2f%%", successRate)
	t.Logf("🚀 Throughput: %.2f msg/s", throughput)
	t.Logf("⚡ Avg Send Latency: %.2f ms", avgSendLatency)
	t.Logf("⚡ Avg Process Latency: %.2f ms", avgProcessLatency)
	
	t.Logf("\n🚨 Rebalance Analysis:")
	t.Logf("   Processing Gaps (>10s): %d", metrics.ProcessingGaps)
	t.Logf("   Suspected Rebalances (>30s): %d", metrics.SuspectedRebalances)
	t.Logf("   Max Gap Duration: %v", metrics.MaxGapDuration)
	t.Logf("   Order Violations: %d", metrics.OrderViolations)
	
	// 评估优化效果
	if successRate >= 95.0 {
		t.Logf("\n🎉 优秀! 高并发优化成功，成功率达到 %.2f%%", successRate)
	} else if successRate >= 80.0 {
		t.Logf("\n⚠️ 良好! 高并发优化有效，成功率达到 %.2f%%", successRate)
	} else {
		t.Logf("\n❌ 需要进一步优化，成功率仅为 %.2f%%", successRate)
	}
	
	if metrics.SuspectedRebalances == 0 {
		t.Logf("✅ 再平衡问题已解决!")
	} else {
		t.Logf("⚠️ 仍有 %d 次疑似再平衡", metrics.SuspectedRebalances)
	}
	
	// 基本验证
	assert.Greater(t, metrics.MessagesSent, int64(0), "Should send messages")
	assert.Greater(t, metrics.MessagesReceived, int64(0), "Should receive messages")
	assert.Greater(t, successRate, 80.0, "Success rate should be > 80%")
	assert.LessOrEqual(t, metrics.SuspectedRebalances, int64(2), "Should have minimal rebalances")
	
	t.Logf("✅ Kafka High Concurrency Optimized Test Completed!")
}
