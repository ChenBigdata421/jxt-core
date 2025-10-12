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

// DynamicPressureMetrics 动态订阅压力测试指标
type DynamicPressureMetrics struct {
	StartTime        time.Time
	EndTime          time.Time
	MessagesSent     int64
	MessagesReceived int64
	Errors           int64
	Duration         time.Duration
	Throughput       float64
	SuccessRate      float64
	AvgLatency       time.Duration
}

// TestDynamicSubscriptionLowPressure 动态订阅低压力测试
func TestDynamicSubscriptionLowPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping dynamic subscription low pressure test in short mode")
	}

	// 使用RedPanda配置
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
			GroupID:            "dynamic-low-pressure-group",
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
		ClientID:            "dynamic-low-pressure-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	runDynamicPressureTest(t, eventBus, "低压力", 600, &DynamicPressureMetrics{})
}

// TestDynamicSubscriptionMediumPressure 动态订阅中压力测试
func TestDynamicSubscriptionMediumPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping dynamic subscription medium pressure test in short mode")
	}

	// 使用RedPanda配置
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
			GroupID:            "dynamic-medium-pressure-group",
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
		ClientID:            "dynamic-medium-pressure-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	runDynamicPressureTest(t, eventBus, "中压力", 1500, &DynamicPressureMetrics{})
}

// TestDynamicSubscriptionHighPressure 动态订阅高压力测试
func TestDynamicSubscriptionHighPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping dynamic subscription high pressure test in short mode")
	}

	// 使用RedPanda配置
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
			GroupID:            "dynamic-high-pressure-group",
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
		ClientID:            "dynamic-high-pressure-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	runDynamicPressureTest(t, eventBus, "高压力", 3000, &DynamicPressureMetrics{})
}

// runDynamicPressureTest 运行动态订阅压力测试
func runDynamicPressureTest(t *testing.T, eventBus EventBus, pressureLevel string, totalMessages int, metrics *DynamicPressureMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	topic := "dynamic.pressure.test"
	
	// 使用原子计数器避免WaitGroup问题
	var receivedCount int64
	var processedMsgs sync.Map // 防止重复计数

	t.Logf("🚀 开始动态订阅%s测试 - %d条消息", pressureLevel, totalMessages)

	// 消息处理器
	handler := func(ctx context.Context, message []byte) error {
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			atomic.AddInt64(&metrics.Errors, 1)
			return err
		}
		
		// 防止重复计数
		if msgID, ok := msg["id"].(string); ok {
			if _, exists := processedMsgs.LoadOrStore(msgID, true); !exists {
				atomic.AddInt64(&receivedCount, 1)
				received := atomic.LoadInt64(&receivedCount)
				
				if received%100 == 0 {
					t.Logf("📥 已接收 %d 条消息", received)
				}
			}
		}
		
		return nil
	}

	// 🚀 动态订阅
	t.Logf("📝 动态订阅topic: %s", topic)
	if err := eventBus.Subscribe(ctx, topic, handler); err != nil {
		t.Fatalf("Failed to subscribe to topic: %v", err)
	}

	// 等待订阅建立
	t.Logf("⏳ 等待动态订阅建立...")
	time.Sleep(5 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	t.Logf("📤 开始发送 %d 条消息", totalMessages)
	
	for i := 0; i < totalMessages; i++ {
		message := map[string]interface{}{
			"id":        fmt.Sprintf("dynamic-%s-msg-%d", pressureLevel, i+1),
			"sequence":  int64(i + 1),
			"timestamp": time.Now().Format(time.RFC3339Nano),
			"pressure":  pressureLevel,
			"data":      fmt.Sprintf("Dynamic %s pressure test message %d", pressureLevel, i+1),
		}
		
		messageBytes, _ := json.Marshal(message)
		
		if err := eventBus.Publish(ctx, topic, messageBytes); err != nil {
			atomic.AddInt64(&metrics.Errors, 1)
			t.Logf("❌ 发送消息 %d 失败: %v", i+1, err)
		} else {
			atomic.AddInt64(&metrics.MessagesSent, 1)
		}
		
		// 适当的发送间隔
		if i%50 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	t.Logf("📤 完成发送 %d 条消息", totalMessages)

	// 等待消息处理完成
	timeout := time.After(120 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			metrics.EndTime = time.Now()
			metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)
			metrics.MessagesReceived = atomic.LoadInt64(&receivedCount)
			metrics.SuccessRate = float64(metrics.MessagesReceived) / float64(totalMessages) * 100
			metrics.Throughput = float64(metrics.MessagesReceived) / metrics.Duration.Seconds()
			
			t.Logf("⏰ 动态订阅%s测试完成（超时）", pressureLevel)
			printDynamicPressureResults(t, pressureLevel, totalMessages, metrics)
			return
			
		case <-ticker.C:
			received := atomic.LoadInt64(&receivedCount)
			if received >= int64(totalMessages) {
				metrics.EndTime = time.Now()
				metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)
				metrics.MessagesReceived = received
				metrics.SuccessRate = 100.0
				metrics.Throughput = float64(received) / metrics.Duration.Seconds()
				
				t.Logf("✅ 动态订阅%s测试完成（全部接收）", pressureLevel)
				printDynamicPressureResults(t, pressureLevel, totalMessages, metrics)
				return
			}
		}
	}
}

// printDynamicPressureResults 打印动态订阅压力测试结果
func printDynamicPressureResults(t *testing.T, pressureLevel string, totalMessages int, metrics *DynamicPressureMetrics) {
	t.Logf("\n" +
		"🎯 ===== 动态订阅%s测试结果 =====\n" +
		"⏱️  测试时长: %.2f秒\n" +
		"📤 消息发送: %d条\n" +
		"📥 消息接收: %d条\n" +
		"❌ 错误数量: %d个\n" +
		"✅ 成功率: %.2f%%\n" +
		"🚀 吞吐量: %.2f msg/s\n" +
		"⚡ 平均延迟: %.3fms\n" +
		"================================",
		pressureLevel,
		metrics.Duration.Seconds(),
		metrics.MessagesSent,
		metrics.MessagesReceived,
		metrics.Errors,
		metrics.SuccessRate,
		metrics.Throughput,
		float64(metrics.Duration.Nanoseconds())/float64(metrics.MessagesReceived)/1000000)

	// 评估测试结果
	if metrics.SuccessRate >= 95.0 {
		t.Logf("🎉 动态订阅%s测试 PASSED (成功率 >= 95%%)", pressureLevel)
	} else if metrics.SuccessRate >= 80.0 {
		t.Logf("⚠️ 动态订阅%s测试 PARTIAL (成功率 >= 80%%)", pressureLevel)
	} else {
		t.Logf("❌ 动态订阅%s测试 FAILED (成功率 < 80%%)", pressureLevel)
	}
}
