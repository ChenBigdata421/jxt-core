package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 🎯 NATS简单性能基准测试
// 测试场景：
// - 基本的NATS发布/订阅（不使用JetStream）
// - 测试吞吐量和延迟
// - 与Kafka性能对比
// SimpleBenchmarkConfig 简单基准测试配置
type SimpleBenchmarkConfig struct {
	TopicCount       int           // topic数量
	MessagesPerTopic int           // 每个topic的消息数量
	TestDuration     time.Duration // 测试持续时间
	BatchSize        int           // 批量发送大小
}

// DefaultSimpleBenchmarkConfig 默认简单基准测试配置
func DefaultSimpleBenchmarkConfig() SimpleBenchmarkConfig {
	return SimpleBenchmarkConfig{
		TopicCount:       5,
		MessagesPerTopic: 1000,
		TestDuration:     30 * time.Second,
		BatchSize:        50,
	}
}

// SimpleBenchmarkMetrics 简单基准测试指标
type SimpleBenchmarkMetrics struct {
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

// SimpleTestMessage 简单测试消息
type SimpleTestMessage struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// TestNATSSimpleBenchmark NATS简单性能基准测试
func TestNATSSimpleBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS simple benchmark test in short mode")
	}
	// 简化配置
	config := SimpleBenchmarkConfig{
		TopicCount:       3,
		MessagesPerTopic: 500,
		TestDuration:     10 * time.Second,
		BatchSize:        25,
	}
	metrics := &SimpleBenchmarkMetrics{
		StartTime: time.Now(),
	}
	// 创建简单的NATS EventBus配置（不使用JetStream）
	natsConfig := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            "nats-simple-benchmark",
		MaxReconnects:       5,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: false, // 禁用JetStream，使用基本NATS
		},
	}
	eventBus, err := NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eventBus.Close()
	// 等待连接建立
	time.Sleep(2 * time.Second)
	t.Logf("🚀 Starting NATS Simple Performance Benchmark")
	t.Logf("📊 Config: %d topics, %d msgs/topic",
		config.TopicCount, config.MessagesPerTopic)
	// 运行基准测试
	runSimpleBenchmark(t, eventBus, config, metrics)
	// 计算并输出结果
	printSimpleBenchmarkResults(t, config, metrics)
}

// runSimpleBenchmark 运行简单基准测试
func runSimpleBenchmark(t *testing.T, eventBus EventBus, config SimpleBenchmarkConfig, metrics *SimpleBenchmarkMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration+10*time.Second)
	defer cancel()
	// 创建topic列表
	topics := make([]string, config.TopicCount)
	for i := 0; i < config.TopicCount; i++ {
		topics[i] = fmt.Sprintf("simple.benchmark.topic.%d", i)
	}
	// 设置消息处理器
	var wg sync.WaitGroup
	setupSimpleMessageHandlers(t, eventBus, topics, metrics, &wg)
	// 等待订阅建立
	time.Sleep(1 * time.Second)
	// 开始发送消息
	metrics.StartTime = time.Now()
	sendSimpleMessages(t, eventBus, topics, config, metrics)
	// 等待所有消息处理完成
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

// setupSimpleMessageHandlers 设置简单消息处理器
func setupSimpleMessageHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *SimpleBenchmarkMetrics, wg *sync.WaitGroup) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()
			handler := func(ctx context.Context, message []byte) error {
				startTime := time.Now()
				// 解析消息
				var testMsg SimpleTestMessage
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

// sendSimpleMessages 发送简单消息
func sendSimpleMessages(t *testing.T, eventBus EventBus, topics []string, config SimpleBenchmarkConfig, metrics *SimpleBenchmarkMetrics) {
	var sendWg sync.WaitGroup
	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()
			for i := 0; i < config.MessagesPerTopic; i++ {
				// 创建测试消息
				testMsg := SimpleTestMessage{
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
					t.Logf("❌ Failed to send message to %s: %v", topicName, err)
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
	t.Logf("📤 Finished sending all simple messages")
}

// printSimpleBenchmarkResults 打印简单基准测试结果
func printSimpleBenchmarkResults(t *testing.T, config SimpleBenchmarkConfig, metrics *SimpleBenchmarkMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	// 计算吞吐量
	throughput := float64(metrics.MessagesReceived) / duration.Seconds()
	// 计算延迟
	avgSendLatency := float64(metrics.SendLatencySum) / float64(metrics.SendLatencyCount) / 1000.0          // ms
	avgProcessLatency := float64(metrics.ProcessLatencySum) / float64(metrics.ProcessLatencyCount) / 1000.0 // ms
	t.Logf("\n🎯 ===== NATS Simple Performance Results =====")
	t.Logf("⏱️  Test Duration: %v", duration)
	t.Logf("📤 Messages Sent: %d", metrics.MessagesSent)
	t.Logf("📥 Messages Received: %d", metrics.MessagesReceived)
	t.Logf("❌ Send Errors: %d", metrics.SendErrors)
	t.Logf("❌ Process Errors: %d", metrics.ProcessErrors)
	t.Logf("🚀 Throughput: %.2f msg/s", throughput)
	t.Logf("⚡ Avg Send Latency: %.2f ms", avgSendLatency)
	t.Logf("⚡ Avg Process Latency: %.2f ms", avgProcessLatency)
	// 验证基本指标
	assert.Greater(t, metrics.MessagesSent, int64(0), "Should send messages")
	assert.Greater(t, metrics.MessagesReceived, int64(0), "Should receive messages")
	assert.LessOrEqual(t, metrics.SendErrors, metrics.MessagesSent/10, "Send error rate should be < 10%")
	assert.LessOrEqual(t, metrics.ProcessErrors, metrics.MessagesReceived/10, "Process error rate should be < 10%")
	assert.Greater(t, throughput, 100.0, "Throughput should be > 100 msg/s")
	t.Logf("✅ NATS Simple Performance Test Completed!")
}

// 🎯 NATS JetStream UnifiedConsumer性能基准测试
// 测试场景：
// - 1个NATS连接，1个JetStream Context，1个统一Consumer
// - 订阅10个topic主题（通过FilterSubject: ">"）
// - 发送端发送envelope格式消息
// - 订阅端根据聚合ID hash到keyed-worker池
// - 确保同一聚合ID的事件严格顺序处理
// - NATS JetStream消息持久化
// NATSBenchmarkConfig NATS基准测试配置
type NATSBenchmarkConfig struct {
	TopicCount       int           // topic数量
	AggregateCount   int           // 聚合ID数量
	MessagesPerTopic int           // 每个topic的消息数量
	WorkerPoolSize   int           // worker池大小
	TestDuration     time.Duration // 测试持续时间
	BatchSize        int           // 批量发送大小
}

// DefaultNATSBenchmarkConfig 默认NATS基准测试配置
func DefaultNATSBenchmarkConfig() NATSBenchmarkConfig {
	return NATSBenchmarkConfig{
		TopicCount:       10,
		AggregateCount:   100,
		MessagesPerTopic: 1000,
		WorkerPoolSize:   20,
		TestDuration:     30 * time.Second,
		BatchSize:        50,
	}
}

// NATSBenchmarkMetrics NATS基准测试指标
type NATSBenchmarkMetrics struct {
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
	// 顺序性指标
	OrderViolations   int64 // 顺序违反次数
	DuplicateMessages int64 // 重复消息数
	// 性能指标
	StartTime      time.Time // 开始时间
	EndTime        time.Time // 结束时间
	PeakThroughput int64     // 峰值吞吐量(msg/s)
	// 聚合ID处理统计
	AggregateStats sync.Map // map[string]*NATSAggregateProcessingStats
}

// NATSAggregateProcessingStats NATS聚合ID处理统计
type NATSAggregateProcessingStats struct {
	MessageCount     int64     // 消息数量
	LastSequence     int64     // 最后处理的序列号
	OrderViolations  int64     // 顺序违反次数
	FirstMessageTime time.Time // 第一条消息时间
	LastMessageTime  time.Time // 最后一条消息时间
}

// NATSTestEnvelope 测试用的NATS Envelope消息格式
type NATSTestEnvelope struct {
	AggregateID string                 `json:"aggregateId"`
	EventType   string                 `json:"eventType"`
	Sequence    int64                  `json:"sequence"`
	Timestamp   time.Time              `json:"timestamp"`
	Payload     map[string]interface{} `json:"payload"`
	Metadata    map[string]string      `json:"metadata"`
}

// TestBenchmarkNATSUnifiedConsumerPerformance 主要的NATS性能基准测试
func TestBenchmarkNATSUnifiedConsumerPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS benchmark test in short mode")
	}
	// 简化配置，减少消息数量以便快速测试
	config := NATSBenchmarkConfig{
		TopicCount:       3,   // 减少到3个topic
		AggregateCount:   10,  // 减少到10个聚合
		MessagesPerTopic: 100, // 减少到100条消息
		WorkerPoolSize:   5,
		TestDuration:     15 * time.Second, // 减少测试时间
		BatchSize:        10,
	}
	metrics := &NATSBenchmarkMetrics{
		StartTime: time.Now(),
	}
	// 创建简化的NATS EventBus配置
	natsConfig := &NATSConfig{
		URLs:                []string{"nats://localhost:4222"},
		ClientID:            "nats-benchmark-client",
		MaxReconnects:       5,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 10 * time.Second,
			AckWait:        10 * time.Second,
			MaxDeliver:     3,
			// 不预定义Stream，让NATS自动创建
		},
	}
	eventBus, err := NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eventBus.Close()
	// 等待连接建立
	time.Sleep(2 * time.Second)
	t.Logf("🚀 Starting NATS Unified Consumer Performance Benchmark")
	t.Logf("📊 Config: %d topics, %d aggregates, %d msgs/topic, %d workers",
		config.TopicCount, config.AggregateCount, config.MessagesPerTopic, config.WorkerPoolSize)
	// 运行基准测试
	runNATSBenchmark(t, eventBus, config, metrics)
	// 计算并输出结果
	printNATSBenchmarkResults(t, config, metrics)
}

// runNATSBenchmark 运行NATS基准测试
func runNATSBenchmark(t *testing.T, eventBus EventBus, config NATSBenchmarkConfig, metrics *NATSBenchmarkMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration+10*time.Second)
	defer cancel()
	// 创建topic列表
	topics := make([]string, config.TopicCount)
	for i := 0; i < config.TopicCount; i++ {
		topics[i] = fmt.Sprintf("benchmark.topic.%d", i)
	}
	// 创建聚合ID列表
	aggregateIDs := make([]string, config.AggregateCount)
	for i := 0; i < config.AggregateCount; i++ {
		aggregateIDs[i] = fmt.Sprintf("aggregate-%d", i)
	}
	// 设置消息处理器
	var wg sync.WaitGroup
	setupNATSMessageHandlers(t, eventBus, topics, metrics, &wg)
	// 等待订阅建立
	time.Sleep(2 * time.Second)
	// 开始发送消息
	metrics.StartTime = time.Now()
	sendNATSMessages(t, eventBus, topics, aggregateIDs, config, metrics)
	// 等待所有消息处理完成
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

// setupNATSMessageHandlers 设置NATS消息处理器
func setupNATSMessageHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *NATSBenchmarkMetrics, wg *sync.WaitGroup) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()
			handler := func(ctx context.Context, message []byte) error {
				startTime := time.Now()
				// 解析消息
				var envelope NATSTestEnvelope
				if err := json.Unmarshal(message, &envelope); err != nil {
					atomic.AddInt64(&metrics.ProcessErrors, 1)
					return err
				}
				// 更新接收指标
				atomic.AddInt64(&metrics.MessagesReceived, 1)
				// 检查顺序性
				checkNATSMessageOrder(envelope, metrics)
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

// sendNATSMessages 发送NATS消息
func sendNATSMessages(t *testing.T, eventBus EventBus, topics []string, aggregateIDs []string, config NATSBenchmarkConfig, metrics *NATSBenchmarkMetrics) {
	var sendWg sync.WaitGroup
	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()
			for i := 0; i < config.MessagesPerTopic; i++ {
				// 随机选择聚合ID
				aggregateID := aggregateIDs[rand.Intn(len(aggregateIDs))]
				// 创建测试消息
				envelope := NATSTestEnvelope{
					AggregateID: aggregateID,
					EventType:   fmt.Sprintf("TestEvent-%s", topicName),
					Sequence:    int64(i),
					Timestamp:   time.Now(),
					Payload: map[string]interface{}{
						"data":    fmt.Sprintf("test-data-%d", i),
						"topic":   topicName,
						"counter": i,
					},
					Metadata: map[string]string{
						"source": "benchmark-test",
						"topic":  topicName,
					},
				}
				// 序列化消息
				messageBytes, err := json.Marshal(envelope)
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
					t.Logf("❌ Failed to send message to %s: %v", topicName, err)
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
	t.Logf("📤 Finished sending all NATS messages")
}

// checkNATSMessageOrder 检查NATS消息顺序
func checkNATSMessageOrder(envelope NATSTestEnvelope, metrics *NATSBenchmarkMetrics) {
	statsInterface, _ := metrics.AggregateStats.LoadOrStore(envelope.AggregateID, &NATSAggregateProcessingStats{
		FirstMessageTime: envelope.Timestamp,
	})
	stats := statsInterface.(*NATSAggregateProcessingStats)
	// 检查顺序
	if stats.MessageCount > 0 && envelope.Sequence <= stats.LastSequence {
		atomic.AddInt64(&stats.OrderViolations, 1)
		atomic.AddInt64(&metrics.OrderViolations, 1)
	}
	// 更新统计
	atomic.AddInt64(&stats.MessageCount, 1)
	stats.LastSequence = envelope.Sequence
	stats.LastMessageTime = envelope.Timestamp
}

// printNATSBenchmarkResults 打印NATS基准测试结果
func printNATSBenchmarkResults(t *testing.T, config NATSBenchmarkConfig, metrics *NATSBenchmarkMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	// 计算吞吐量
	throughput := float64(metrics.MessagesReceived) / duration.Seconds()
	// 计算延迟
	avgSendLatency := float64(metrics.SendLatencySum) / float64(metrics.SendLatencyCount) / 1000.0          // ms
	avgProcessLatency := float64(metrics.ProcessLatencySum) / float64(metrics.ProcessLatencyCount) / 1000.0 // ms
	t.Logf("\n🎯 ===== NATS JetStream Unified Consumer Performance Results =====")
	t.Logf("⏱️  Test Duration: %v", duration)
	t.Logf("📤 Messages Sent: %d", metrics.MessagesSent)
	t.Logf("📥 Messages Received: %d", metrics.MessagesReceived)
	t.Logf("❌ Send Errors: %d", metrics.SendErrors)
	t.Logf("❌ Process Errors: %d", metrics.ProcessErrors)
	t.Logf("🚀 Throughput: %.2f msg/s", throughput)
	t.Logf("⚡ Avg Send Latency: %.2f ms", avgSendLatency)
	t.Logf("⚡ Avg Process Latency: %.2f ms", avgProcessLatency)
	t.Logf("🔄 Order Violations: %d", metrics.OrderViolations)
	t.Logf("🔁 Duplicate Messages: %d", metrics.DuplicateMessages)
	// 验证基本指标
	assert.Greater(t, metrics.MessagesSent, int64(0), "Should send messages")
	assert.Greater(t, metrics.MessagesReceived, int64(0), "Should receive messages")
	assert.LessOrEqual(t, metrics.SendErrors, metrics.MessagesSent/10, "Send error rate should be < 10%")
	assert.LessOrEqual(t, metrics.ProcessErrors, metrics.MessagesReceived/10, "Process error rate should be < 10%")
	assert.Greater(t, throughput, 100.0, "Throughput should be > 100 msg/s")
	t.Logf("✅ NATS JetStream Unified Consumer Performance Test Completed!")
}
