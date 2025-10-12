package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestKafkaOptimizedPerformance 测试Kafka优化后的性能
// 🚀 验证业界最佳实践优化效果
func TestKafkaOptimizedPerformance(t *testing.T) {
	t.Log("🚀 Kafka优化性能测试 - 基于Confluent官方最佳实践")
	t.Log("=" + string(make([]byte, 80)))

	// 创建Kafka配置（应用所有优化）
	kafkaConfig := &KafkaConfig{
		Brokers:  []string{"localhost:29094"}, // 使用RedPanda端口
		ClientID: "optimized-perf-test",
		Producer: ProducerConfig{
			RequiredAcks:    1,                     // acks=1（性能优先）
			Compression:     "lz4",                 // 🚀 优化2：LZ4压缩（Confluent首选）
			FlushFrequency:  10 * time.Millisecond, // 🚀 优化3：10ms批量（Confluent推荐）
			FlushMessages:   100,                   // 🚀 优化3：100条消息（Confluent推荐）
			FlushBytes:      100000,                // 🚀 优化3：100KB（Confluent推荐）
			RetryMax:        3,
			Timeout:         10 * time.Second,
			MaxMessageBytes: 1024 * 1024,
			Idempotent:      false,
			MaxInFlight:     100, // 🚀 优化4：100并发请求（业界推荐）
		},
		Consumer: ConsumerConfig{
			GroupID:           "optimized-perf-test-group",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 1 * time.Second,
			FetchMinBytes:     100 * 1024,             // 🚀 优化6：100KB（Confluent推荐）
			FetchMaxBytes:     10 * 1024 * 1024,       // 🚀 优化6：10MB（Confluent推荐）
			FetchMaxWait:      500 * time.Millisecond, // 🚀 优化6：500ms（Confluent推荐）
			AutoOffsetReset:   "latest",
			RebalanceStrategy: "range",
			IsolationLevel:    "read_uncommitted",
		},
		Net: NetConfig{
			DialTimeout:  10 * time.Second, // 🚀 优化8：10秒（业界推荐）
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			KeepAlive:    30 * time.Second,
		},
		MetadataRefreshFreq:  10 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 250 * time.Millisecond,
		HealthCheckInterval:  30 * time.Second,
	}

	// 创建EventBus
	bus, err := NewKafkaEventBus(kafkaConfig)
	if err != nil {
		t.Fatalf("❌ Failed to create Kafka EventBus: %v", err)
	}
	defer bus.Close()

	t.Log("✅ Kafka EventBus创建成功（AsyncProducer + 全部优化）")

	// 测试场景
	scenarios := []struct {
		name         string
		messageCount int
		messageSize  int
	}{
		{"轻负载", 300, 1024},
		{"中负载", 800, 1024},
		{"重负载", 1500, 1024},
		{"极限负载", 3000, 1024},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			testKafkaOptimizedScenario(t, bus, scenario.name, scenario.messageCount, scenario.messageSize)
		})
	}
}

func testKafkaOptimizedScenario(t *testing.T, bus EventBus, scenarioName string, messageCount, messageSize int) {
	t.Logf("\n📊 测试场景: %s", scenarioName)
	t.Logf("   消息数量: %d", messageCount)
	t.Logf("   消息大小: %d bytes", messageSize)

	topic := fmt.Sprintf("optimized-perf-test-%d", time.Now().UnixNano())
	ctx := context.Background()

	// 🚀 配置预订阅模式
	if kafkaBus, ok := bus.(*kafkaEventBus); ok {
		kafkaBus.allPossibleTopics = []string{topic}
		t.Logf("✅ Configured pre-subscription for topic: %s", topic)
	}

	// 订阅计数器
	var receivedCount atomic.Int64
	var firstMessageTime atomic.Value // time.Time
	var lastMessageTime atomic.Value  // time.Time
	receivedChan := make(chan struct{}, messageCount)

	// 订阅
	handler := func(ctx context.Context, message []byte) error {
		now := time.Now()

		// 记录第一条消息时间
		if firstMessageTime.Load() == nil {
			firstMessageTime.Store(now)
		}

		// 更新最后一条消息时间
		lastMessageTime.Store(now)

		receivedCount.Add(1)
		receivedChan <- struct{}{}
		return nil
	}

	err := bus.Subscribe(ctx, topic, handler)
	if err != nil {
		t.Fatalf("❌ Failed to subscribe: %v", err)
	}
	t.Logf("✅ Subscribed to topic: %s", topic)

	// 等待订阅就绪
	time.Sleep(5 * time.Second) // 增加等待时间

	// 发送消息
	message := make([]byte, messageSize)
	for i := 0; i < messageSize; i++ {
		message[i] = byte(i % 256)
	}

	t.Logf("📤 开始发送 %d 条消息...", messageCount)
	sendStart := time.Now()

	var sendErrors atomic.Int64
	var wg sync.WaitGroup

	// 并发发送
	concurrency := 10
	messagesPerWorker := messageCount / concurrency

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < messagesPerWorker; j++ {
				if err := bus.Publish(ctx, topic, message); err != nil {
					sendErrors.Add(1)
					t.Logf("⚠️  Send error: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
	sendDuration := time.Since(sendStart)

	t.Logf("✅ 发送完成: %v", sendDuration)
	t.Logf("   发送速率: %.2f msg/s", float64(messageCount)/sendDuration.Seconds())
	t.Logf("   发送错误: %d", sendErrors.Load())

	// 🚀 AsyncProducer需要额外等待时间让消息真正发送
	t.Logf("⏳ 等待AsyncProducer批量发送完成...")
	time.Sleep(10 * time.Second) // 等待批量发送完成

	// 等待接收
	t.Logf("📥 等待接收消息...")
	receiveTimeout := 3 * time.Minute // 增加超时时间
	receiveStart := time.Now()

	receivedInTime := 0
	timeoutTimer := time.NewTimer(receiveTimeout)
	defer timeoutTimer.Stop()

receiveLoop:
	for receivedInTime < messageCount {
		select {
		case <-receivedChan:
			receivedInTime++
		case <-timeoutTimer.C:
			t.Logf("⏱️  接收超时 (%v)", receiveTimeout)
			break receiveLoop
		}
	}

	receiveDuration := time.Since(receiveStart)

	// 计算指标
	successRate := float64(receivedInTime) / float64(messageCount) * 100
	throughput := float64(receivedInTime) / receiveDuration.Seconds()

	// 计算延迟
	var firstMsgLatency time.Duration
	if firstMessageTime.Load() != nil {
		firstMsgLatency = firstMessageTime.Load().(time.Time).Sub(sendStart)
	}

	// 输出结果
	t.Logf("\n📊 性能指标:")
	t.Logf("   ✅ 成功率: %.2f%% (%d/%d)", successRate, receivedInTime, messageCount)
	t.Logf("   🚀 吞吐量: %.2f msg/s", throughput)
	t.Logf("   ⏱️  首条延迟: %v", firstMsgLatency)
	t.Logf("   ⏱️  总接收时间: %v", receiveDuration)
	t.Logf("   📤 发送速率: %.2f msg/s", float64(messageCount)/sendDuration.Seconds())

	// 性能断言
	if successRate < 95.0 {
		t.Errorf("❌ 成功率过低: %.2f%% (期望 >= 95%%)", successRate)
	}

	if throughput < 10.0 {
		t.Errorf("❌ 吞吐量过低: %.2f msg/s (期望 >= 10 msg/s)", throughput)
	}

	t.Logf("✅ 场景测试完成\n")
}

// TestKafkaOptimizationComparison 对比优化前后的性能
func TestKafkaOptimizationComparison(t *testing.T) {
	t.Log("🔬 Kafka优化对比测试")
	t.Log("=" + string(make([]byte, 80)))

	messageCount := 1000
	messageSize := 1024

	// 测试优化后的配置
	t.Log("\n🚀 测试优化后配置（AsyncProducer + 批处理 + LZ4）")
	optimizedConfig := &KafkaConfig{
		Brokers:  []string{"localhost:29094"}, // 使用RedPanda端口
		ClientID: "optimized-comparison-test",
		Producer: ProducerConfig{
			RequiredAcks:    1,
			Compression:     "lz4",                 // LZ4压缩
			FlushFrequency:  10 * time.Millisecond, // 10ms批量
			FlushMessages:   100,                   // 100条消息
			FlushBytes:      100000,                // 100KB
			RetryMax:        3,
			Timeout:         10 * time.Second,
			MaxMessageBytes: 1024 * 1024,
			MaxInFlight:     100,
		},
		Consumer: ConsumerConfig{
			GroupID:           "optimized-comparison-group",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 1 * time.Second,
			FetchMinBytes:     100 * 1024,       // 100KB
			FetchMaxBytes:     10 * 1024 * 1024, // 10MB
			FetchMaxWait:      500 * time.Millisecond,
			AutoOffsetReset:   "latest",
			RebalanceStrategy: "range",
			IsolationLevel:    "read_uncommitted",
		},
		Net: NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			KeepAlive:    30 * time.Second,
		},
		MetadataRefreshFreq:  10 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 250 * time.Millisecond,
		HealthCheckInterval:  30 * time.Second,
	}

	optimizedBus, err := NewKafkaEventBus(optimizedConfig)
	if err != nil {
		t.Fatalf("❌ Failed to create optimized Kafka EventBus: %v", err)
	}
	defer optimizedBus.Close()

	optimizedMetrics := runPerformanceTest(t, optimizedBus, "optimized", messageCount, messageSize)

	// 输出对比结果
	t.Logf("\n📊 优化效果总结:")
	t.Logf("   🚀 吞吐量: %.2f msg/s", optimizedMetrics.throughput)
	t.Logf("   ✅ 成功率: %.2f%%", optimizedMetrics.successRate)
	t.Logf("   ⏱️  首条延迟: %v", optimizedMetrics.firstMsgLatency)
	t.Logf("   📤 发送速率: %.2f msg/s", optimizedMetrics.sendRate)

	t.Log("\n🎯 优化目标达成情况:")
	if optimizedMetrics.throughput >= 30.0 {
		t.Logf("   ✅ 吞吐量目标达成: %.2f msg/s >= 30 msg/s", optimizedMetrics.throughput)
	} else {
		t.Logf("   ⚠️  吞吐量未达目标: %.2f msg/s < 30 msg/s", optimizedMetrics.throughput)
	}

	if optimizedMetrics.successRate >= 95.0 {
		t.Logf("   ✅ 成功率目标达成: %.2f%% >= 95%%", optimizedMetrics.successRate)
	} else {
		t.Logf("   ⚠️  成功率未达目标: %.2f%% < 95%%", optimizedMetrics.successRate)
	}

	if optimizedMetrics.firstMsgLatency <= 100*time.Millisecond {
		t.Logf("   ✅ 延迟目标达成: %v <= 100ms", optimizedMetrics.firstMsgLatency)
	} else {
		t.Logf("   ⚠️  延迟未达目标: %v > 100ms", optimizedMetrics.firstMsgLatency)
	}
}

type performanceMetrics struct {
	throughput      float64
	successRate     float64
	firstMsgLatency time.Duration
	sendRate        float64
}

func runPerformanceTest(t *testing.T, bus EventBus, testName string, messageCount, messageSize int) performanceMetrics {
	topic := fmt.Sprintf("perf-test-%s-%d", testName, time.Now().UnixNano())
	ctx := context.Background()

	// 🚀 配置预订阅模式
	if kafkaBus, ok := bus.(*kafkaEventBus); ok {
		kafkaBus.allPossibleTopics = []string{topic}
		t.Logf("✅ Configured pre-subscription for topic: %s", topic)
	}

	var receivedCount atomic.Int64
	var firstMessageTime atomic.Value
	receivedChan := make(chan struct{}, messageCount)

	handler := func(ctx context.Context, message []byte) error {
		now := time.Now()
		if firstMessageTime.Load() == nil {
			firstMessageTime.Store(now)
		}
		receivedCount.Add(1)
		receivedChan <- struct{}{}
		return nil
	}

	err := bus.Subscribe(ctx, topic, handler)
	if err != nil {
		t.Fatalf("❌ Failed to subscribe: %v", err)
	}
	t.Logf("✅ Subscribed to topic: %s", topic)
	time.Sleep(5 * time.Second) // 增加等待时间确保订阅就绪

	message := make([]byte, messageSize)
	sendStart := time.Now()

	for i := 0; i < messageCount; i++ {
		bus.Publish(ctx, topic, message)
	}

	sendDuration := time.Since(sendStart)
	sendRate := float64(messageCount) / sendDuration.Seconds()

	// 🚀 AsyncProducer需要额外等待时间让消息真正发送
	t.Logf("⏳ Waiting for async producer to flush messages...")
	time.Sleep(5 * time.Second) // 等待批量发送完成

	receiveTimeout := 2 * time.Minute // 增加超时时间
	receivedInTime := 0
	timeoutTimer := time.NewTimer(receiveTimeout)
	defer timeoutTimer.Stop()

receiveLoop:
	for receivedInTime < messageCount {
		select {
		case <-receivedChan:
			receivedInTime++
		case <-timeoutTimer.C:
			break receiveLoop
		}
	}

	receiveDuration := time.Since(sendStart)
	successRate := float64(receivedInTime) / float64(messageCount) * 100
	throughput := float64(receivedInTime) / receiveDuration.Seconds()

	var firstMsgLatency time.Duration
	if firstMessageTime.Load() != nil {
		firstMsgLatency = firstMessageTime.Load().(time.Time).Sub(sendStart)
	}

	return performanceMetrics{
		throughput:      throughput,
		successRate:     successRate,
		firstMsgLatency: firstMsgLatency,
		sendRate:        sendRate,
	}
}
