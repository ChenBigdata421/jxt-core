package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 🔥 Kafka UnifiedConsumer 压力测试
// 验证在高并发、大数据量场景下的性能和稳定性

// StressTestConfig 压力测试配置
type StressTestConfig struct {
	TopicCount        int           // topic数量
	ProducerCount     int           // 生产者数量
	ConsumerCount     int           // 消费者数量（worker池大小）
	MessagesPerSecond int           // 每秒消息数
	TestDuration      time.Duration // 测试持续时间
	MessageSize       int           // 消息大小(字节)
	AggregateCount    int           // 聚合ID数量
}

// StressTestMetrics 压力测试指标
type StressTestMetrics struct {
	// 实时统计
	MessagesSent      int64 // 发送消息数
	MessagesReceived  int64 // 接收消息数
	SendErrors        int64 // 发送错误数
	ProcessErrors     int64 // 处理错误数
	OrderViolations   int64 // 顺序违反数
	
	// 性能统计
	MaxSendLatency    int64 // 最大发送延迟(μs)
	MaxProcessLatency int64 // 最大处理延迟(μs)
	TotalSendLatency  int64 // 总发送延迟
	TotalProcessLatency int64 // 总处理延迟
	LatencyCount      int64 // 延迟计数
	
	// 系统资源
	MaxMemoryUsage    uint64 // 最大内存使用(字节)
	MaxGoroutines     int    // 最大协程数
	
	// 时间戳
	StartTime         time.Time
	EndTime           time.Time
	
	// 聚合处理跟踪
	AggregateSequences sync.Map // map[string]int64 - 跟踪每个聚合的序列号
}

// TestKafkaUnifiedConsumerStressTest 主压力测试
func TestKafkaUnifiedConsumerStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	config := StressTestConfig{
		TopicCount:        10,
		ProducerCount:     5,
		ConsumerCount:     20,
		MessagesPerSecond: 5000,
		TestDuration:      60 * time.Second,
		MessageSize:       1024, // 1KB
		AggregateCount:    200,
	}

	metrics := &StressTestMetrics{
		StartTime: time.Now(),
	}

	t.Logf("🔥 开始Kafka UnifiedConsumer压力测试")
	t.Logf("📊 配置: %d topics, %d producers, %d consumers, %d msg/s, %d seconds",
		config.TopicCount, config.ProducerCount, config.ConsumerCount, 
		config.MessagesPerSecond, int(config.TestDuration.Seconds()))

	// 创建高性能Kafka配置
	kafkaConfig := createHighPerformanceKafkaConfig()
	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err)
	defer eventBus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration+30*time.Second)
	defer cancel()

	// 启动系统监控
	stopMonitoring := startSystemMonitoring(metrics)
	defer stopMonitoring()

	// 设置消费者
	err = setupStressTestConsumers(ctx, eventBus, config, metrics)
	require.NoError(t, err)

	// 等待消费者准备就绪
	time.Sleep(3 * time.Second)

	// 启动生产者
	err = runStressTestProducers(ctx, eventBus, config, metrics)
	require.NoError(t, err)

	// 等待测试完成
	time.Sleep(config.TestDuration)
	cancel() // 停止生产者

	// 等待剩余消息处理完成
	waitForRemainingMessages(metrics, 30*time.Second, t)

	// 分析压力测试结果
	analyzeStressTestResults(config, metrics, t)
}

// createHighPerformanceKafkaConfig 创建高性能Kafka配置
func createHighPerformanceKafkaConfig() *KafkaConfig {
	return &KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Producer: ProducerConfig{
			RequiredAcks:     -1, // WaitForAll，幂等性生产者要求
			Compression:      "snappy",
			FlushFrequency:   5 * time.Millisecond,  // 更频繁刷新
			FlushMessages:    200,                   // 更大批次
			Timeout:          30 * time.Second,
			FlushBytes:       2 * 1024 * 1024,      // 2MB
			RetryMax:         5,
			BatchSize:        32 * 1024,            // 32KB
			BufferSize:       64 * 1024 * 1024,     // 64MB
			Idempotent:       true,
			MaxMessageBytes:  2 * 1024 * 1024,      // 2MB
			PartitionerType:  "hash",
			LingerMs:         2 * time.Millisecond,  // 更短等待
			CompressionLevel: 6,
			MaxInFlight:      1,                     // 幂等性生产者要求MaxInFlight=1
		},
		Consumer: ConsumerConfig{
			GroupID:            "stress-test-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  60 * time.Second,
			FetchMinBytes:      1024,
			FetchMaxBytes:      100 * 1024 * 1024,  // 100MB
			FetchMaxWait:       100 * time.Millisecond, // 更短等待
			MaxPollRecords:     1000,               // 更大批次
			EnableAutoCommit:   false,
			AutoCommitInterval: 1 * time.Second,    // 更频繁提交
			IsolationLevel:     "read_committed",
			RebalanceStrategy:  "range",
		},
		HealthCheckInterval:  30 * time.Second,
		ClientID:            "stress-test-client",
		MetadataRefreshFreq: 5 * time.Minute,
		MetadataRetryMax:    5,
		MetadataRetryBackoff: 100 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:   10 * time.Second,
			ReadTimeout:   30 * time.Second,
			WriteTimeout:  30 * time.Second,
			KeepAlive:     30 * time.Second,
			MaxIdleConns:  20,
			MaxOpenConns:  200,
		},
		Security: SecurityConfig{Enabled: false},
	}
}

// setupStressTestConsumers 设置压力测试消费者
func setupStressTestConsumers(ctx context.Context, eventBus EventBus, config StressTestConfig, metrics *StressTestMetrics) error {
	topics := make([]string, config.TopicCount)
	for i := 0; i < config.TopicCount; i++ {
		topics[i] = fmt.Sprintf("stress-topic-%d", i)
	}

	var wg sync.WaitGroup
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()
			
			handler := createStressTestHandler(topicName, metrics)
			err := eventBus.Subscribe(ctx, topicName, handler)
			if err != nil {
				panic(fmt.Sprintf("Failed to subscribe to %s: %v", topicName, err))
			}
		}(topic)
	}
	
	wg.Wait()
	return nil
}

// createStressTestHandler 创建压力测试处理器
func createStressTestHandler(topic string, metrics *StressTestMetrics) MessageHandler {
	return func(ctx context.Context, message []byte) error {
		startTime := time.Now()
		
		// 解析消息
		var envelope TestEnvelope
		if err := json.Unmarshal(message, &envelope); err != nil {
			atomic.AddInt64(&metrics.ProcessErrors, 1)
			return err
		}
		
		// 检查消息顺序
		checkStressTestOrder(envelope, metrics)
		
		// 模拟处理时间（1-5ms随机）
		processingTime := time.Duration(1+rand.Intn(4)) * time.Millisecond
		time.Sleep(processingTime)
		
		// 更新指标
		atomic.AddInt64(&metrics.MessagesReceived, 1)
		
		// 记录处理延迟
		latency := time.Since(startTime).Microseconds()
		atomic.AddInt64(&metrics.TotalProcessLatency, latency)
		atomic.AddInt64(&metrics.LatencyCount, 1)
		
		// 更新最大延迟
		for {
			current := atomic.LoadInt64(&metrics.MaxProcessLatency)
			if latency <= current || atomic.CompareAndSwapInt64(&metrics.MaxProcessLatency, current, latency) {
				break
			}
		}
		
		return nil
	}
}

// checkStressTestOrder 检查压力测试中的消息顺序
func checkStressTestOrder(envelope TestEnvelope, metrics *StressTestMetrics) {
	aggregateID := envelope.AggregateID
	expectedSeq := envelope.Sequence
	
	// 获取或初始化聚合序列号
	actualInterface, _ := metrics.AggregateSequences.LoadOrStore(aggregateID, int64(-1))
	lastSeq := actualInterface.(int64)
	
	// 检查顺序
	if lastSeq >= 0 && expectedSeq != lastSeq+1 {
		atomic.AddInt64(&metrics.OrderViolations, 1)
	}
	
	// 更新序列号
	metrics.AggregateSequences.Store(aggregateID, expectedSeq)
}

// runStressTestProducers 运行压力测试生产者
func runStressTestProducers(ctx context.Context, eventBus EventBus, config StressTestConfig, metrics *StressTestMetrics) error {
	topics := make([]string, config.TopicCount)
	for i := 0; i < config.TopicCount; i++ {
		topics[i] = fmt.Sprintf("stress-topic-%d", i)
	}

	// 计算每个生产者的发送速率
	messagesPerProducer := config.MessagesPerSecond / config.ProducerCount
	sendInterval := time.Second / time.Duration(messagesPerProducer)

	var wg sync.WaitGroup
	for i := 0; i < config.ProducerCount; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			runSingleProducer(ctx, eventBus, topics, config, metrics, producerID, sendInterval)
		}(i)
	}

	go func() {
		wg.Wait()
	}()

	return nil
}

// runSingleProducer 运行单个生产者
func runSingleProducer(ctx context.Context, eventBus EventBus, topics []string, config StressTestConfig, metrics *StressTestMetrics, producerID int, sendInterval time.Duration) {
	ticker := time.NewTicker(sendInterval)
	defer ticker.Stop()
	
	messageCounter := int64(0)
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 选择topic和聚合ID
			topic := topics[rand.Intn(len(topics))]
			aggregateID := fmt.Sprintf("agg-%d", rand.Intn(config.AggregateCount))
			
			// 创建消息
			envelope := TestEnvelope{
				AggregateID: aggregateID,
				EventType:   fmt.Sprintf("StressEvent-%d", messageCounter%10),
				Sequence:    messageCounter,
				Timestamp:   time.Now(),
				Payload: map[string]interface{}{
					"producerID": producerID,
					"counter":    messageCounter,
					"data":       generateRandomData(config.MessageSize),
				},
				Metadata: map[string]string{
					"producer": fmt.Sprintf("producer-%d", producerID),
					"topic":    topic,
				},
			}
			
			messageBytes, err := json.Marshal(envelope)
			if err != nil {
				atomic.AddInt64(&metrics.SendErrors, 1)
				continue
			}
			
			// 发送消息
			sendStart := time.Now()
			err = eventBus.Publish(ctx, topic, messageBytes)
			sendLatency := time.Since(sendStart).Microseconds()
			
			if err != nil {
				atomic.AddInt64(&metrics.SendErrors, 1)
			} else {
				atomic.AddInt64(&metrics.MessagesSent, 1)
				atomic.AddInt64(&metrics.TotalSendLatency, sendLatency)
				
				// 更新最大发送延迟
				for {
					current := atomic.LoadInt64(&metrics.MaxSendLatency)
					if sendLatency <= current || atomic.CompareAndSwapInt64(&metrics.MaxSendLatency, current, sendLatency) {
						break
					}
				}
			}
			
			messageCounter++
		}
	}
}

// generateRandomData 生成指定大小的随机数据
func generateRandomData(size int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, size)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// startSystemMonitoring 启动系统监控
func startSystemMonitoring(metrics *StressTestMetrics) func() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 监控内存使用
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.Alloc > metrics.MaxMemoryUsage {
					metrics.MaxMemoryUsage = m.Alloc
				}

				// 监控协程数量
				goroutines := runtime.NumGoroutine()
				if goroutines > metrics.MaxGoroutines {
					metrics.MaxGoroutines = goroutines
				}
			}
		}
	}()

	return cancel
}

// waitForRemainingMessages 等待剩余消息处理完成
func waitForRemainingMessages(metrics *StressTestMetrics, timeout time.Duration, t *testing.T) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastReceived := atomic.LoadInt64(&metrics.MessagesReceived)
	stableCount := 0

	for time.Now().Before(deadline) {
		select {
		case <-ticker.C:
			currentReceived := atomic.LoadInt64(&metrics.MessagesReceived)
			sent := atomic.LoadInt64(&metrics.MessagesSent)

			t.Logf("📊 等待处理完成: %d/%d (%.1f%%)",
				currentReceived, sent, float64(currentReceived)/float64(sent)*100)

			// 如果接收数量没有变化，认为处理完成
			if currentReceived == lastReceived {
				stableCount++
				if stableCount >= 3 { // 连续3次检查都没有变化
					t.Log("✅ 消息处理完成")
					return
				}
			} else {
				stableCount = 0
				lastReceived = currentReceived
			}
		}
	}

	t.Log("⚠️  等待超时，可能还有消息在处理中")
}

// analyzeStressTestResults 分析压力测试结果
func analyzeStressTestResults(config StressTestConfig, metrics *StressTestMetrics, t *testing.T) {
	metrics.EndTime = time.Now()
	duration := metrics.EndTime.Sub(metrics.StartTime)

	// 基本统计
	sent := atomic.LoadInt64(&metrics.MessagesSent)
	received := atomic.LoadInt64(&metrics.MessagesReceived)
	sendErrors := atomic.LoadInt64(&metrics.SendErrors)
	processErrors := atomic.LoadInt64(&metrics.ProcessErrors)
	orderViolations := atomic.LoadInt64(&metrics.OrderViolations)

	// 性能计算
	sendThroughput := float64(sent) / duration.Seconds()
	receiveThroughput := float64(received) / duration.Seconds()

	// 延迟计算
	latencyCount := atomic.LoadInt64(&metrics.LatencyCount)
	var avgSendLatency, avgProcessLatency float64
	if latencyCount > 0 {
		avgSendLatency = float64(atomic.LoadInt64(&metrics.TotalSendLatency)) / float64(latencyCount)
		avgProcessLatency = float64(atomic.LoadInt64(&metrics.TotalProcessLatency)) / float64(latencyCount)
	}

	maxSendLatency := atomic.LoadInt64(&metrics.MaxSendLatency)
	maxProcessLatency := atomic.LoadInt64(&metrics.MaxProcessLatency)

	// 输出详细结果
	t.Log("\n" + strings.Repeat("=", 100))
	t.Log("🔥 Kafka UnifiedConsumer 压力测试结果")
	t.Log(strings.Repeat("=", 100))

	t.Logf("📊 基本统计:")
	t.Logf("   测试持续时间: %.2f 秒", duration.Seconds())
	t.Logf("   发送消息数: %d", sent)
	t.Logf("   接收消息数: %d", received)
	t.Logf("   消息成功率: %.2f%%", float64(received)/float64(sent)*100)
	t.Logf("   发送错误: %d", sendErrors)
	t.Logf("   处理错误: %d", processErrors)

	t.Logf("\n🚀 性能指标:")
	t.Logf("   发送吞吐量: %.2f msg/s", sendThroughput)
	t.Logf("   接收吞吐量: %.2f msg/s", receiveThroughput)
	t.Logf("   目标吞吐量: %d msg/s", config.MessagesPerSecond)
	t.Logf("   吞吐量达成率: %.1f%%", sendThroughput/float64(config.MessagesPerSecond)*100)

	t.Logf("\n⏱️  延迟统计:")
	t.Logf("   平均发送延迟: %.2f μs", avgSendLatency)
	t.Logf("   最大发送延迟: %d μs", maxSendLatency)
	t.Logf("   平均处理延迟: %.2f μs", avgProcessLatency)
	t.Logf("   最大处理延迟: %d μs", maxProcessLatency)

	t.Logf("\n🔍 顺序性检查:")
	t.Logf("   顺序违反次数: %d", orderViolations)
	t.Logf("   顺序正确率: %.4f%%", (1.0-float64(orderViolations)/float64(received))*100)

	t.Logf("\n💾 系统资源:")
	t.Logf("   最大内存使用: %.2f MB", float64(metrics.MaxMemoryUsage)/1024/1024)
	t.Logf("   最大协程数: %d", metrics.MaxGoroutines)

	// 聚合处理统计
	analyzeAggregateDistribution(metrics, t)

	// 性能断言
	assert.True(t, sendThroughput > float64(config.MessagesPerSecond)*0.8,
		"发送吞吐量应该达到目标的80%以上")
	assert.True(t, receiveThroughput > float64(config.MessagesPerSecond)*0.8,
		"接收吞吐量应该达到目标的80%以上")
	assert.True(t, float64(received)/float64(sent) > 0.95,
		"消息成功率应该 > 95%")
	assert.Equal(t, int64(0), orderViolations,
		"不应该有任何顺序违反")
	assert.True(t, avgSendLatency < 10000,
		"平均发送延迟应该 < 10ms")
	assert.True(t, avgProcessLatency < 20000,
		"平均处理延迟应该 < 20ms")

	t.Log(strings.Repeat("=", 100))
}

// analyzeAggregateDistribution 分析聚合分布情况
func analyzeAggregateDistribution(metrics *StressTestMetrics, t *testing.T) {
	t.Logf("\n📈 聚合ID分布分析:")

	var aggregateCount int64
	var minMessages, maxMessages int64 = 999999, 0
	var totalMessages int64

	metrics.AggregateSequences.Range(func(key, value interface{}) bool {
		sequence := value.(int64)
		messageCount := sequence + 1 // 序列号从0开始

		aggregateCount++
		totalMessages += messageCount

		if messageCount < minMessages {
			minMessages = messageCount
		}
		if messageCount > maxMessages {
			maxMessages = messageCount
		}

		return true
	})

	if aggregateCount > 0 {
		avgMessages := float64(totalMessages) / float64(aggregateCount)
		t.Logf("   聚合ID总数: %d", aggregateCount)
		t.Logf("   消息分布: %d - %d (最少-最多)", minMessages, maxMessages)
		t.Logf("   平均每聚合: %.2f 消息", avgMessages)

		// 计算分布均匀性
		variance := float64(maxMessages - minMessages)
		uniformity := (1.0 - variance/avgMessages) * 100
		if uniformity < 0 {
			uniformity = 0
		}
		t.Logf("   分布均匀性: %.1f%%", uniformity)
	}
}
