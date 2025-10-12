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
