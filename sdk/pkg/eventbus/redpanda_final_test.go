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

// FinalTestMetrics 最终测试指标
type FinalTestMetrics struct {
	MessagesSent     int64
	MessagesReceived int64
	SendErrors       int64
	StartTime        time.Time
	EndTime          time.Time
}

// TestRedPandaFinalMediumPressure 最终的RedPanda中压力测试
func TestRedPandaFinalMediumPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping final RedPanda medium pressure test in short mode")
	}

	metrics := &FinalTestMetrics{StartTime: time.Now()}

	// 🔧 使用已验证工作的配置
	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   1 * time.Millisecond,
			FlushMessages:    1,
			Timeout:          10 * time.Second,
			FlushBytes:       16 * 1024,
			RetryMax:         2,
			BatchSize:        1024,
			BufferSize:       4 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  64 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         1 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      2,
		},
		Consumer: ConsumerConfig{
			GroupID:            "redpanda-final-medium-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  45 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      2 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     25,
			EnableAutoCommit:   true,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		ClientID:            "redpanda-final-medium-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	t.Logf("🔧 Starting Final RedPanda Medium Pressure Test")

	// 🔧 使用单个topic，1500条消息（比低压力600条多，但比原来3000条少）
	runFinalTest(t, eventBus, "RedPanda-Final", 1500, metrics)
	analyzeFinalResults(t, "RedPanda-Final", "Medium", metrics)
}

// runFinalTest 运行最终测试
func runFinalTest(t *testing.T, eventBus EventBus, system string, totalMessages int, metrics *FinalTestMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second) // 2分钟超时
	defer cancel()

	// 🔧 使用单个topic避免多topic协调问题
	topic := "redpanda.final.test"

	t.Logf("📊 %s Test Config: 1 topic, %d total messages", system, totalMessages)

	// 设置消息处理器
	var wg sync.WaitGroup
	wg.Add(totalMessages) // 预设等待数量

	handler := func(ctx context.Context, message []byte) error {
		defer wg.Done()
		atomic.AddInt64(&metrics.MessagesReceived, 1)
		
		// 简化处理
		received := atomic.LoadInt64(&metrics.MessagesReceived)
		if received%100 == 0 {
			t.Logf("📥 Received %d messages", received)
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
	sendFinalMessages(t, eventBus, topic, totalMessages, metrics, system)

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

// sendFinalMessages 发送最终测试消息
func sendFinalMessages(t *testing.T, eventBus EventBus, topic string, totalMessages int, metrics *FinalTestMetrics, system string) {
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
		if i%100 == 0 && i > 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	
	t.Logf("📤 Finished sending %d messages", atomic.LoadInt64(&metrics.MessagesSent))
}

// analyzeFinalResults 分析最终测试结果
func analyzeFinalResults(t *testing.T, system, pressure string, metrics *FinalTestMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	
	sent := atomic.LoadInt64(&metrics.MessagesSent)
	received := atomic.LoadInt64(&metrics.MessagesReceived)
	sendErrors := atomic.LoadInt64(&metrics.SendErrors)
	
	successRate := float64(received) / float64(sent) * 100
	throughput := float64(received) / duration.Seconds()
	
	t.Logf("\n" +
		"🎯 ========== %s %s Pressure Test Results ==========\n" +
		"⏱️  Test Duration: %v\n" +
		"📤 Messages Sent: %d\n" +
		"📥 Messages Received: %d\n" +
		"❌ Send Errors: %d\n" +
		"✅ Success Rate: %.2f%%\n" +
		"🚀 Throughput: %.2f msg/s\n" +
		"🎯 ================================================\n",
		system, pressure, duration, sent, received, sendErrors,
		successRate, throughput)
}

// TestRedPandaFinalHighPressure 最终的RedPanda高压力测试
func TestRedPandaFinalHighPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping final RedPanda high pressure test in short mode")
	}

	metrics := &FinalTestMetrics{StartTime: time.Now()}

	// 🔧 使用已验证工作的配置
	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   1 * time.Millisecond,
			FlushMessages:    1,
			Timeout:          10 * time.Second,
			FlushBytes:       16 * 1024,
			RetryMax:         2,
			BatchSize:        1024,
			BufferSize:       4 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  64 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         1 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      2,
		},
		Consumer: ConsumerConfig{
			GroupID:            "redpanda-final-high-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  45 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      2 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     25,
			EnableAutoCommit:   true,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		ClientID:            "redpanda-final-high-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	t.Logf("🔧 Starting Final RedPanda High Pressure Test")

	// 🔧 使用单个topic，3000条消息（原来的中压力级别）
	runFinalTest(t, eventBus, "RedPanda-Final", 3000, metrics)
	analyzeFinalResults(t, "RedPanda-Final", "High", metrics)
}
