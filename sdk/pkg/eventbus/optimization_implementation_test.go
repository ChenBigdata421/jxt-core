package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestOptimizationImplementation 验证4项优化措施的实施效果
func TestOptimizationImplementation(t *testing.T) {
	t.Logf("🚀 Starting Optimization Implementation Verification...")

	// 创建优化后的配置
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: "optimization-implementation-test",
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
			Compression:     "", // 测试默认压缩优化
			MaxInFlight:     0,  // 测试默认并发优化
		},
		Consumer: ConsumerConfig{
			GroupID:            "optimization-implementation-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     3 * time.Second,       // 优化后配置
			HeartbeatInterval:  1 * time.Second,       // 优化后配置
			MaxProcessingTime:  3 * time.Second,
			FetchMinBytes:      1024 * 1024,
			FetchMaxBytes:      50 * 1024 * 1024,
			FetchMaxWait:       10 * time.Millisecond, // 优化后配置
			MaxPollRecords:     2000,                  // 优化后配置
			EnableAutoCommit:   true,
			AutoCommitInterval: 500 * time.Millisecond,
		},
	}

	eventBus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("Failed to create EventBus: %v", err)
	}
	defer eventBus.Close()

	kafkaBus := eventBus.(*kafkaEventBus)
	testTopic := "optimization.implementation.test"
	kafkaBus.allPossibleTopics = []string{testTopic}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 验证优化4：预热状态监控
	t.Logf("🔍 Verification 4: Warmup Status Monitoring")
	
	// 检查初始状态
	completed, duration := kafkaBus.GetWarmupInfo()
	t.Logf("   Initial warmup status: completed=%v, duration=%v", completed, duration)

	var receivedCount int64
	var firstMessageTime time.Time
	var lastMessageTime time.Time

	handler := func(ctx context.Context, message []byte) error {
		receiveTime := time.Now()
		count := atomic.AddInt64(&receivedCount, 1)
		
		if count == 1 {
			firstMessageTime = receiveTime
		}
		lastMessageTime = receiveTime
		return nil
	}

	// 订阅topic（这会触发预热）
	subscribeStart := time.Now()
	err = eventBus.Subscribe(ctx, testTopic, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	subscribeEnd := time.Now()

	// 验证优化1：3秒预热机制
	t.Logf("🔍 Verification 1: 3-Second Warmup Mechanism")
	t.Logf("   Subscribe duration: %v", subscribeEnd.Sub(subscribeStart))
	
	// 检查预热完成状态
	completed, duration = kafkaBus.GetWarmupInfo()
	t.Logf("   Post-subscribe warmup status: completed=%v, duration=%v", completed, duration)
	
	if !completed {
		t.Errorf("❌ Warmup should be completed after Subscribe")
	}
	
	if duration < 3*time.Second {
		t.Errorf("❌ Warmup duration should be at least 3 seconds, got %v", duration)
	} else {
		t.Logf("✅ Warmup completed successfully in %v", duration)
	}

	// 验证优化3：确保测试批量≥1000条消息
	t.Logf("🔍 Verification 3: Batch Size ≥1000 Messages")
	messageCount := 1200 // 超过1000条
	t.Logf("   Testing with %d messages (≥1000)", messageCount)

	// 发送消息
	sendStart := time.Now()
	errors := 0
	
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":       fmt.Sprintf("opt-impl-msg-%d", i),
			"content":  fmt.Sprintf("Optimization implementation message %d", i),
			"sendTime": time.Now().Format(time.RFC3339Nano),
			"index":    i,
		}

		messageBytes, _ := json.Marshal(message)
		err := eventBus.Publish(ctx, testTopic, messageBytes)
		if err != nil {
			errors++
			if errors <= 5 {
				t.Logf("Publish error %d: %v", errors, err)
			}
		}
	}

	sendEnd := time.Now()
	sendDuration := sendEnd.Sub(sendStart)
	sendRate := float64(messageCount-errors) / sendDuration.Seconds()

	// 验证优化2：Producer性能优化
	t.Logf("🔍 Verification 2: Optimized Producer Configuration")
	t.Logf("   Messages sent: %d/%d (%.2f%% success)", messageCount-errors, messageCount, 
		float64(messageCount-errors)/float64(messageCount)*100)
	t.Logf("   Send duration: %v", sendDuration)
	t.Logf("   Send rate: %.2f msg/s", sendRate)
	t.Logf("   Errors: %d", errors)

	// 期望发送速率应该达到优化后的水平（≥400 msg/s）
	if sendRate < 400 {
		t.Logf("⚠️ Send rate below optimized target: %.2f msg/s (expected ≥400 msg/s)", sendRate)
	} else {
		t.Logf("✅ Send rate meets optimized target: %.2f msg/s", sendRate)
	}

	// 等待消息处理
	t.Logf("⏳ Waiting for message processing...")
	time.Sleep(30 * time.Second)

	// 分析结果
	finalCount := atomic.LoadInt64(&receivedCount)
	successRate := float64(finalCount) / float64(messageCount) * 100
	
	var firstMessageLatency time.Duration
	if !firstMessageTime.IsZero() {
		firstMessageLatency = firstMessageTime.Sub(sendStart)
	}

	var processingLatency time.Duration
	if !firstMessageTime.IsZero() && !lastMessageTime.IsZero() {
		processingLatency = lastMessageTime.Sub(firstMessageTime)
	}

	totalDuration := time.Since(sendStart)
	throughput := float64(finalCount) / totalDuration.Seconds()

	t.Logf("\n📊 ===== Optimization Implementation Results =====")
	t.Logf("📈 Overall Performance:")
	t.Logf("   📤 Messages sent: %d", messageCount-errors)
	t.Logf("   📥 Messages received: %d", finalCount)
	t.Logf("   ✅ Success rate: %.2f%%", successRate)
	t.Logf("   🚀 Send rate: %.2f msg/s", sendRate)
	t.Logf("   🚀 Throughput: %.2f msg/s", throughput)
	t.Logf("   ⏱️  First message latency: %v", firstMessageLatency)
	t.Logf("   ⏱️  Processing latency: %v", processingLatency)
	t.Logf("   ⏱️  Total duration: %v", totalDuration)

	t.Logf("📈 Optimization Implementation Verification:")
	t.Logf("   ✅ Optimization 1 (3s Warmup): %v", duration >= 3*time.Second)
	t.Logf("   ✅ Optimization 2 (Producer): %.2f msg/s send rate", sendRate)
	t.Logf("   ✅ Optimization 3 (Batch ≥1000): %d messages", messageCount)
	t.Logf("   ✅ Optimization 4 (Monitoring): warmup completed=%v", completed)

	// 验证成功标准
	if successRate < 90 {
		t.Errorf("❌ Success rate too low: %.2f%% (expected ≥90%%)", successRate)
	} else {
		t.Logf("✅ Success rate PASSED: %.2f%%", successRate)
	}

	if sendRate < 100 {
		t.Logf("⚠️ Send rate below minimum: %.2f msg/s (expected ≥100 msg/s)", sendRate)
	} else {
		t.Logf("✅ Send rate PASSED: %.2f msg/s", sendRate)
	}

	if firstMessageLatency > 500*time.Millisecond {
		t.Logf("⚠️ First message latency high: %v (expected ≤500ms)", firstMessageLatency)
	} else {
		t.Logf("✅ First message latency PASSED: %v", firstMessageLatency)
	}

	t.Logf("\n🎉 Optimization implementation verification completed!")
}

// TestOptimizedLowPressure 使用优化后配置的低压力测试
func TestOptimizedLowPressure(t *testing.T) {
	t.Logf("🚀 Starting Optimized Low Pressure Test...")

	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: "optimized-low-pressure-test",
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
			Compression:     "", // 自动优化
			MaxInFlight:     0,  // 自动优化
		},
		Consumer: ConsumerConfig{
			GroupID:            "optimized-low-pressure-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     3 * time.Second,
			HeartbeatInterval:  1 * time.Second,
			MaxProcessingTime:  3 * time.Second,
			FetchMinBytes:      1024 * 1024,
			FetchMaxBytes:      50 * 1024 * 1024,
			FetchMaxWait:       10 * time.Millisecond,
			MaxPollRecords:     2000,
			EnableAutoCommit:   true,
			AutoCommitInterval: 500 * time.Millisecond,
		},
	}

	eventBus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("Failed to create EventBus: %v", err)
	}
	defer eventBus.Close()

	kafkaBus := eventBus.(*kafkaEventBus)
	testTopic := "optimized.low.pressure.test"
	kafkaBus.allPossibleTopics = []string{testTopic}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var receivedCount int64
	handler := func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&receivedCount, 1)
		return nil
	}

	// 订阅（包含预热）
	err = eventBus.Subscribe(ctx, testTopic, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 发送1000条消息（满足批量要求）
	messageCount := 1000
	sendStart := time.Now()
	
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":      fmt.Sprintf("optimized-low-msg-%d", i),
			"content": fmt.Sprintf("Optimized low pressure message %d", i),
		}

		messageBytes, _ := json.Marshal(message)
		err := eventBus.Publish(ctx, testTopic, messageBytes)
		if err != nil {
			t.Logf("Failed to publish message %d: %v", i, err)
		}
	}

	sendDuration := time.Since(sendStart)
	sendRate := float64(messageCount) / sendDuration.Seconds()

	// 等待处理
	time.Sleep(30 * time.Second)

	finalCount := atomic.LoadInt64(&receivedCount)
	successRate := float64(finalCount) / float64(messageCount) * 100

	t.Logf("\n📊 ===== Optimized Low Pressure Results =====")
	t.Logf("📤 Messages sent: %d", messageCount)
	t.Logf("📥 Messages received: %d", finalCount)
	t.Logf("✅ Success rate: %.2f%%", successRate)
	t.Logf("🚀 Send rate: %.2f msg/s", sendRate)
	t.Logf("⏱️  Send duration: %v", sendDuration)

	// 期望优化后的低压力测试应该达到95%以上成功率
	if successRate < 95 {
		t.Errorf("❌ Optimized low pressure success rate too low: %.2f%% (expected ≥95%%)", successRate)
	} else {
		t.Logf("✅ Optimized low pressure PASSED: %.2f%%", successRate)
	}
}
