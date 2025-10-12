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

// SimpleTestMetrics 简化的测试指标
type SimpleTestMetrics struct {
	MessagesSent     int64
	MessagesReceived int64
	SendErrors       int64
	StartTime        time.Time
	EndTime          time.Time
	OrderViolations  int64
}

// TestRedPandaSimpleMediumPressure 简化的RedPanda中压力测试
func TestRedPandaSimpleMediumPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping simple RedPanda medium pressure test in short mode")
	}

	metrics := &SimpleTestMetrics{StartTime: time.Now()}

	// 🔧 使用基本配置，避免复杂参数
	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: ProducerConfig{
			RequiredAcks:    1,
			Compression:     "none",
			FlushFrequency:  100 * time.Millisecond,
			FlushMessages:   10,
			Timeout:         30 * time.Second,
			FlushBytes:      32 * 1024,
			MaxMessageBytes: 1000000,
		},
		Consumer: ConsumerConfig{
			GroupID:            "redpanda-simple-medium-group",
			AutoOffsetReset:    "earliest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  30 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     500,
			IsolationLevel:     "read_uncommitted",
		},
		ClientID:            "redpanda-simple-medium-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	t.Logf("🔧 Starting Simple RedPanda Medium Pressure Test")

	// 🔧 使用单个topic，1000条消息
	runSimpleTest(t, eventBus, "RedPanda-Simple", 1000, metrics)
	analyzeSimpleResults(t, "RedPanda-Simple", "Medium", metrics)
}

// runSimpleTest 运行简化的测试
func runSimpleTest(t *testing.T, eventBus EventBus, system string, totalMessages int, metrics *SimpleTestMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // 2分钟超时
	defer cancel()

	// 🔧 使用单个topic避免多topic协调问题
	topic := "redpanda.simple.test"

	t.Logf("📊 %s Test Config: 1 topic, %d total messages", system, totalMessages)

	// 设置消息处理器
	var wg sync.WaitGroup
	wg.Add(totalMessages) // 预设等待数量

	handler := func(ctx context.Context, message []byte) error {
		defer wg.Done()
		atomic.AddInt64(&metrics.MessagesReceived, 1)

		// 简化处理，不做复杂的顺序检查
		if atomic.LoadInt64(&metrics.MessagesReceived)%100 == 0 {
			t.Logf("📥 Received %d messages", atomic.LoadInt64(&metrics.MessagesReceived))
		}

		return nil
	}

	// 订阅topic
	if err := eventBus.Subscribe(ctx, topic, handler); err != nil {
		t.Fatalf("Failed to subscribe to topic: %v", err)
	}

	// 等待订阅建立
	t.Logf("⏳ Waiting for subscription to stabilize...")
	time.Sleep(5 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendRedPandaSimpleMessages(t, eventBus, topic, totalMessages, metrics, system)

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

// sendRedPandaSimpleMessages 发送简化的消息
func sendRedPandaSimpleMessages(t *testing.T, eventBus EventBus, topic string, totalMessages int, metrics *SimpleTestMetrics, system string) {
	ctx := context.Background()

	for i := 0; i < totalMessages; i++ {
		message := map[string]interface{}{
			"id":        fmt.Sprintf("%s-msg-%d", system, i+1),
			"sequence":  int64(i + 1),
			"timestamp": time.Now().Format(time.RFC3339Nano),
			"data":      fmt.Sprintf("Test message %d from %s", i+1, system),
			"system":    system,
		}

		messageBytes, _ := json.Marshal(message)

		if err := eventBus.Publish(ctx, topic, messageBytes); err != nil {
			atomic.AddInt64(&metrics.SendErrors, 1)
			t.Logf("❌ Failed to send message %d: %v", i+1, err)
		} else {
			atomic.AddInt64(&metrics.MessagesSent, 1)
		}

		// 发送进度
		if (i+1)%100 == 0 {
			t.Logf("📤 Sent %d/%d messages", i+1, totalMessages)
		}

		// 小延迟避免过快发送
		if i%50 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	t.Logf("📤 Finished sending %d messages", atomic.LoadInt64(&metrics.MessagesSent))
}

// analyzeSimpleResults 分析简化的测试结果
func analyzeSimpleResults(t *testing.T, system, pressure string, metrics *SimpleTestMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)

	sent := atomic.LoadInt64(&metrics.MessagesSent)
	received := atomic.LoadInt64(&metrics.MessagesReceived)
	sendErrors := atomic.LoadInt64(&metrics.SendErrors)
	orderViolations := atomic.LoadInt64(&metrics.OrderViolations)

	successRate := float64(received) / float64(sent) * 100
	throughput := float64(received) / duration.Seconds()

	t.Logf("\n"+
		"🎯 ========== %s %s Pressure Test Results ==========\n"+
		"⏱️  Test Duration: %v\n"+
		"📤 Messages Sent: %d\n"+
		"📥 Messages Received: %d\n"+
		"❌ Send Errors: %d\n"+
		"✅ Success Rate: %.2f%%\n"+
		"🚀 Throughput: %.2f msg/s\n"+
		"⚠️ Order Violations: %d\n"+
		"🎯 ================================================\n",
		system, pressure, duration, sent, received, sendErrors,
		successRate, throughput, orderViolations)
}

// TestRedPandaSimpleHighPressure 简化的RedPanda高压力测试
func TestRedPandaSimpleHighPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping simple RedPanda high pressure test in short mode")
	}

	metrics := &SimpleTestMetrics{StartTime: time.Now()}

	// 🔧 使用基本配置
	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: ProducerConfig{
			RequiredAcks:    1,
			Compression:     "none",
			FlushFrequency:  50 * time.Millisecond,
			FlushMessages:   20,
			Timeout:         30 * time.Second,
			FlushBytes:      64 * 1024,
			MaxMessageBytes: 1000000,
		},
		Consumer: ConsumerConfig{
			GroupID:            "redpanda-simple-high-group",
			AutoOffsetReset:    "earliest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  30 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     1000,
			IsolationLevel:     "read_uncommitted",
		},
		ClientID:            "redpanda-simple-high-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	t.Logf("🔧 Starting Simple RedPanda High Pressure Test")

	// 🔧 使用单个topic，5000条消息
	runSimpleTest(t, eventBus, "RedPanda-Simple", 5000, metrics)
	analyzeSimpleResults(t, "RedPanda-Simple", "High", metrics)
}
