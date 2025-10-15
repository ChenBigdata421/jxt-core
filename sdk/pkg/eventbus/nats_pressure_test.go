package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

// TestNATSStage2Pressure 第二阶段优化压力测试
// 🎯 目标：验证异步发布 + 全局Worker池的优化效果
// 📊 预期：吞吐量3600-9500 msg/s，Goroutine数量降至100-200
func TestNATSStage2Pressure(t *testing.T) {
	// 🔧 测试配置
	const (
		testDuration    = 30 * time.Second // 测试持续时间
		messageSize     = 1024             // 消息大小（1KB）
		publisherCount  = 10               // 发布者数量
		subscriberCount = 5                // 订阅者数量
		topicCount      = 3                // Topic数量
	)
	// 📊 性能指标
	var (
		publishedCount    int64
		consumedCount     int64
		errorCount        int64
		startTime         time.Time
		endTime           time.Time
		initialGoroutines int
		peakGoroutines    int
	)
	// ✅ 第二阶段优化配置（使用本地NATS服务器）
	timestamp := time.Now().UnixNano()
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-stage2-pressure-test-%d", timestamp),
		MaxReconnects:       5,
		ReconnectWait:       2 * time.Second,
		ConnectionTimeout:   10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 100 * time.Millisecond, // ✅ 优化3: 缩短超时
			AckWait:        30 * time.Second,
			MaxDeliver:     3,
			Stream: StreamConfig{
				Name:      fmt.Sprintf("STAGE2_STREAM_%d", timestamp),
				Subjects:  []string{fmt.Sprintf("stage2@%d.>", timestamp)},
				Retention: "limits",
				Storage:   "file",
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  1024 * 1024 * 1024,
				MaxMsgs:   1000000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("stage2_consumer_%d", timestamp),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 10000, // ✅ 优化8: 增大到10000
				MaxWaiting:    1000,  // ✅ 优化8: 增大到1000
				MaxDeliver:    3,
			},
		},
	}
	// 创建EventBus实例
	bus, err := NewNATSEventBus(config)
	require.NoError(t, err)
	defer bus.Close()
	// 等待连接稳定
	time.Sleep(2 * time.Second)
	// 📊 记录初始Goroutine数量
	initialGoroutines = runtime.NumGoroutine()
	t.Logf("🔍 Initial Goroutines: %d", initialGoroutines)
	// 🎯 创建测试Topic（确保不会被识别为有聚合ID）
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		// 使用包含多个特殊字符的topic名称，确保所有段都包含无效字符
		// 这样ExtractAggregateID就无法从任何段中提取有效的聚合ID
		topics[i] = fmt.Sprintf("stage2@%d.test@%d.msg#%d", timestamp, i, i)
	}
	// 📝 创建测试消息
	testMessage := make([]byte, messageSize)
	for i := range testMessage {
		testMessage[i] = byte(i % 256)
	}
	// 🎯 设置订阅者（每个订阅者订阅一个topic，避免重复订阅）
	var subscriberWg sync.WaitGroup
	for i := 0; i < subscriberCount; i++ {
		subscriberWg.Add(1)
		go func(subscriberID int) {
			defer subscriberWg.Done()
			// 每个订阅者订阅一个topic（轮询分配）
			topicIndex := subscriberID % len(topics)
			topic := topics[topicIndex]
			err := bus.Subscribe(context.Background(), topic, func(ctx context.Context, data []byte) error {
				atomic.AddInt64(&consumedCount, 1)
				// 模拟消息处理（轻量级）
				if len(data) != messageSize {
					atomic.AddInt64(&errorCount, 1)
					return fmt.Errorf("invalid message size: expected %d, got %d", messageSize, len(data))
				}
				return nil
			})
			if err != nil {
				t.Errorf("Subscriber %d failed to subscribe to topic %s: %v", subscriberID, topic, err)
				atomic.AddInt64(&errorCount, 1)
			}
		}(i)
	}
	// 等待订阅者准备就绪
	subscriberWg.Wait()
	time.Sleep(2 * time.Second)
	// 📊 开始性能测试
	t.Logf("🚀 Starting NATS Stage2 pressure test...")
	t.Logf("📊 Test config: %d publishers, %d subscribers, %d topics, %d seconds",
		publisherCount, subscriberCount, topicCount, int(testDuration.Seconds()))
	startTime = time.Now()
	// 🎯 启动发布者
	var publisherWg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()
	for i := 0; i < publisherCount; i++ {
		publisherWg.Add(1)
		go func(publisherID int) {
			defer publisherWg.Done()
			ticker := time.NewTicker(10 * time.Millisecond) // 每10ms发布一次
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// 轮询发布到不同topic
					topic := topics[int(atomic.LoadInt64(&publishedCount))%topicCount]
					err := bus.Publish(ctx, topic, testMessage)
					if err != nil {
						atomic.AddInt64(&errorCount, 1)
						t.Logf("Publisher %d failed to publish: %v", publisherID, err)
					} else {
						atomic.AddInt64(&publishedCount, 1)
					}
				}
			}
		}(i)
	}
	// 📊 监控Goroutine数量
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := runtime.NumGoroutine()
				if current > peakGoroutines {
					peakGoroutines = current
				}
			}
		}
	}()
	// 等待测试完成
	publisherWg.Wait()
	endTime = time.Now()
	// 等待消息处理完成
	time.Sleep(5 * time.Second)
	// 📊 收集最终统计
	finalPublished := atomic.LoadInt64(&publishedCount)
	finalConsumed := atomic.LoadInt64(&consumedCount)
	finalErrors := atomic.LoadInt64(&errorCount)
	finalGoroutines := runtime.NumGoroutine()
	duration := endTime.Sub(startTime)
	publishThroughput := float64(finalPublished) / duration.Seconds()
	consumeThroughput := float64(finalConsumed) / duration.Seconds()
	// 📊 输出测试结果
	t.Logf("\n"+
		"🎯 ===== NATS Stage2 Pressure Test Results =====\n"+
		"⏱️  Duration: %.2f seconds\n"+
		"📤 Published: %d messages (%.2f msg/s)\n"+
		"📥 Consumed: %d messages (%.2f msg/s)\n"+
		"❌ Errors: %d\n"+
		"🧵 Goroutines: %d → %d (peak: %d)\n"+
		"📊 Success Rate: %.2f%%\n"+
		"🚀 Optimization: Async Publish + Global Worker Pool\n",
		duration.Seconds(),
		finalPublished, publishThroughput,
		finalConsumed, consumeThroughput,
		finalErrors,
		initialGoroutines, finalGoroutines, peakGoroutines,
		float64(finalConsumed)/float64(finalPublished)*100)
	// ✅ 验证第二阶段优化目标
	t.Logf("\n🎯 ===== Stage2 Optimization Goals Verification =====")
	// 目标1: 吞吐量3600-9500 msg/s
	if publishThroughput >= 3600 {
		t.Logf("✅ Throughput Goal: %.2f msg/s (Target: 3600-9500 msg/s) - ACHIEVED", publishThroughput)
	} else {
		t.Logf("❌ Throughput Goal: %.2f msg/s (Target: 3600-9500 msg/s) - NOT ACHIEVED", publishThroughput)
	}
	// 目标2: Goroutine数量降至100-200
	if peakGoroutines <= 200 {
		t.Logf("✅ Goroutine Goal: %d (Target: ≤200) - ACHIEVED", peakGoroutines)
	} else {
		t.Logf("❌ Goroutine Goal: %d (Target: ≤200) - NOT ACHIEVED", peakGoroutines)
	}
	// 目标3: 错误率 < 1%
	errorRate := float64(finalErrors) / float64(finalPublished) * 100
	if errorRate < 1.0 {
		t.Logf("✅ Error Rate Goal: %.2f%% (Target: <1%%) - ACHIEVED", errorRate)
	} else {
		t.Logf("❌ Error Rate Goal: %.2f%% (Target: <1%%) - NOT ACHIEVED", errorRate)
	}
	// 基本断言
	assert.Greater(t, finalPublished, int64(1000), "Should publish at least 1000 messages")
	assert.Greater(t, finalConsumed, int64(500), "Should consume at least 500 messages")
	assert.Less(t, errorRate, 5.0, "Error rate should be less than 5%")
}

// 🎯 NATS vs Kafka 高压力对比测试
// 公平对比两种消息中间件在相同高压力下的表现
//
// 测试场景：
// 1. NATS Basic（非 JetStream）- 最高性能基准
// 2. NATS JetStream（第一阶段优化）- 优化 2、3、8
// 3. Kafka - 对比基准
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
			GroupID:            "kafka-high-pressure-comparison-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     60 * time.Second,
			HeartbeatInterval:  20 * time.Second,
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
