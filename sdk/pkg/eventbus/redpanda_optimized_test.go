package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// OptimizedTestMetrics 优化的测试指标
type OptimizedTestMetrics struct {
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

// TestRedPandaOptimizedMediumPressure 优化的RedPanda中压力测试
func TestRedPandaOptimizedMediumPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping optimized RedPanda medium pressure test in short mode")
	}

	metrics := &OptimizedTestMetrics{StartTime: time.Now()}

	// 🔧 优化的RedPanda配置
	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: ProducerConfig{
			RequiredAcks:    1,
			Compression:     "none",
			FlushFrequency:  10 * time.Millisecond, // 增加批处理
			FlushMessages:   50,                    // 增加批大小
			Timeout:         30 * time.Second,      // 增加超时
			FlushBytes:      128 * 1024,            // 增加缓冲区
			MaxMessageBytes: 1000000,
		},
		Consumer: ConsumerConfig{
			GroupID:            "redpanda-optimized-medium-group",
			AutoOffsetReset:    "earliest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
			SessionTimeout:     30 * time.Second, // 增加会话超时
			HeartbeatInterval:  3 * time.Second,  // 增加心跳间隔
			MaxProcessingTime:  60 * time.Second, // 增加处理时间
			FetchMinBytes:      1024,             // 增加最小获取字节
			FetchMaxBytes:      1024 * 1024,      // 增加最大获取字节
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     100, // 增加批处理大小
			IsolationLevel:     "read_uncommitted",
		},
		ClientID:            "redpanda-optimized-medium-client",
		HealthCheckInterval: 1 * time.Minute,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	t.Logf("🔧 Starting Optimized RedPanda Medium Pressure Test")

	// 🔧 优化策略：使用单个topic，多分区模拟
	runOptimizedTest(t, eventBus, "RedPanda-Optimized", 3000, metrics)
	analyzeOptimizedResults(t, "RedPanda-Optimized", "Medium", metrics)
}

// runOptimizedTest 运行优化的测试
func runOptimizedTest(t *testing.T, eventBus EventBus, system string, totalMessages int, metrics *OptimizedTestMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second) // 3分钟超时
	defer cancel()

	// 🔧 使用单个topic避免多topic协调问题
	topic := "redpanda.optimized.test"

	t.Logf("📊 %s Test Config: 1 topic, %d total messages", system, totalMessages)

	// 设置消息处理器
	var wg sync.WaitGroup
	var lastSequence int64 = 0

	handler := func(ctx context.Context, message []byte) error {
		defer wg.Done()

		atomic.AddInt64(&metrics.MessagesReceived, 1)

		// 简化的顺序检查
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			if seq, ok := msg["sequence"].(float64); ok {
				currentSeq := int64(seq)
				if currentSeq <= atomic.LoadInt64(&lastSequence) {
					atomic.AddInt64(&metrics.OrderViolations, 1)
				}
				atomic.StoreInt64(&lastSequence, currentSeq)
			}
		}

		return nil
	}

	// 订阅topic
	if err := eventBus.Subscribe(ctx, topic, handler); err != nil {
		t.Fatalf("Failed to subscribe to topic: %v", err)
	}

	// 等待订阅建立
	t.Logf("⏳ Waiting for subscription to stabilize...")
	time.Sleep(2 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendOptimizedMessages(t, eventBus, topic, totalMessages, metrics, system)

	// 等待消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		metrics.EndTime = time.Now()
		t.Logf("✅ All messages processed successfully")
	case <-ctx.Done():
		metrics.EndTime = time.Now()
		t.Logf("⏰ Test timed out, processed %d/%d messages",
			atomic.LoadInt64(&metrics.MessagesReceived), totalMessages)
	}
}

// sendOptimizedMessages 发送优化的消息
func sendOptimizedMessages(t *testing.T, eventBus EventBus, topic string, totalMessages int, metrics *OptimizedTestMetrics, system string) {
	ctx := context.Background()

	// 🔧 批量发送以提高性能
	batchSize := 100
	var wg sync.WaitGroup

	for batch := 0; batch < totalMessages; batch += batchSize {
		wg.Add(1)
		go func(batchStart int) {
			defer wg.Done()

			batchEnd := batchStart + batchSize
			if batchEnd > totalMessages {
				batchEnd = totalMessages
			}

			for i := batchStart; i < batchEnd; i++ {
				message := map[string]interface{}{
					"id":        fmt.Sprintf("%s-msg-%d", system, i+1),
					"sequence":  int64(i + 1),
					"timestamp": time.Now().Format(time.RFC3339Nano),
					"data":      fmt.Sprintf("Test message %d from %s", i+1, system),
					"system":    system,
				}

				messageBytes, _ := json.Marshal(message)

				start := time.Now()
				if err := eventBus.Publish(ctx, topic, messageBytes); err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
					t.Logf("❌ Failed to send message %d: %v", i+1, err)
				} else {
					atomic.AddInt64(&metrics.MessagesSent, 1)
					latency := time.Since(start).Nanoseconds()
					atomic.AddInt64(&metrics.SendLatencySum, latency)
					atomic.AddInt64(&metrics.SendLatencyCount, 1)
				}
			}
		}(batch)
	}

	wg.Wait()
	t.Logf("📤 Finished sending %d messages", atomic.LoadInt64(&metrics.MessagesSent))
}

// analyzeOptimizedResults 分析优化的测试结果
func analyzeOptimizedResults(t *testing.T, system, pressure string, metrics *OptimizedTestMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)

	sent := atomic.LoadInt64(&metrics.MessagesSent)
	received := atomic.LoadInt64(&metrics.MessagesReceived)
	sendErrors := atomic.LoadInt64(&metrics.SendErrors)
	processErrors := atomic.LoadInt64(&metrics.ProcessErrors)
	orderViolations := atomic.LoadInt64(&metrics.OrderViolations)

	successRate := float64(received) / float64(sent) * 100
	throughput := float64(received) / duration.Seconds()

	var avgLatency float64
	if latencyCount := atomic.LoadInt64(&metrics.SendLatencyCount); latencyCount > 0 {
		avgLatency = float64(atomic.LoadInt64(&metrics.SendLatencySum)) / float64(latencyCount) / 1e6 // 转换为毫秒
	}

	t.Logf("\n"+
		"🎯 ========== %s %s Pressure Test Results ==========\n"+
		"⏱️  Test Duration: %v\n"+
		"📤 Messages Sent: %d\n"+
		"📥 Messages Received: %d\n"+
		"❌ Send Errors: %d\n"+
		"❌ Process Errors: %d\n"+
		"✅ Success Rate: %.2f%%\n"+
		"🚀 Throughput: %.2f msg/s\n"+
		"⚡ Average Latency: %.3fms\n"+
		"⚠️ Order Violations: %d\n"+
		"🎯 ================================================\n",
		system, pressure, duration, sent, received, sendErrors, processErrors,
		successRate, throughput, avgLatency, orderViolations)
}
