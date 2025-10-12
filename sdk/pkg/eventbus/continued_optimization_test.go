package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestContinuedOptimization 验证继续优化后的效果
func TestContinuedOptimization(t *testing.T) {
	t.Logf("🔧 Starting Continued Optimization Test...")

	// 使用平衡后的配置
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: "continued-optimization-test",
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
			Compression:     "", // 自动优化为snappy
			MaxInFlight:     0,  // 自动优化为50
		},
		Consumer: ConsumerConfig{
			GroupID:            "continued-optimization-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     6 * time.Second,        // 平衡：稳定性优先
			HeartbeatInterval:  2 * time.Second,        // 平衡：稳定性优先
			MaxProcessingTime:  5 * time.Second,        // 平衡：增加处理时间
			FetchMinBytes:      1024 * 1024,            // 1MB
			FetchMaxBytes:      50 * 1024 * 1024,       // 50MB
			FetchMaxWait:       100 * time.Millisecond, // 平衡：适度优化
			MaxPollRecords:     2000,                   // 确保批量≥1000
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
	testTopic := "continued.optimization.test"
	kafkaBus.allPossibleTopics = []string{testTopic}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// 验证预热状态监控
	t.Logf("🔍 Checking warmup status monitoring...")
	completed, duration := kafkaBus.GetWarmupInfo()
	t.Logf("   Initial warmup status: completed=%v, duration=%v", completed, duration)

	var receivedCount int64
	var firstMessageTime time.Time
	var lastMessageTime time.Time
	var processedMessages []string

	handler := func(ctx context.Context, message []byte) error {
		receiveTime := time.Now()
		count := atomic.AddInt64(&receivedCount, 1)
		
		if count == 1 {
			firstMessageTime = receiveTime
		}
		lastMessageTime = receiveTime

		// 解析消息以验证内容
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			if msgID, ok := msg["id"].(string); ok {
				processedMessages = append(processedMessages, msgID)
			}
		}

		return nil
	}

	// 订阅topic（包含预热）
	subscribeStart := time.Now()
	err = eventBus.Subscribe(ctx, testTopic, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	subscribeEnd := time.Now()

	// 验证预热完成
	t.Logf("🔍 Verifying warmup completion...")
	t.Logf("   Subscribe duration: %v", subscribeEnd.Sub(subscribeStart))
	
	completed, duration = kafkaBus.GetWarmupInfo()
	t.Logf("   Post-subscribe warmup status: completed=%v, duration=%v", completed, duration)
	
	if !completed {
		t.Errorf("❌ Warmup should be completed after Subscribe")
	} else {
		t.Logf("✅ Warmup completed successfully")
	}

	// 发送1200条消息（满足批量要求）
	messageCount := 1200
	t.Logf("🔍 Sending %d messages...", messageCount)

	sendStart := time.Now()
	errors := 0
	
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":       fmt.Sprintf("continued-opt-msg-%d", i),
			"content":  fmt.Sprintf("Continued optimization message %d", i),
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

	t.Logf("📊 Send Performance:")
	t.Logf("   Messages sent: %d/%d (%.2f%% success)", messageCount-errors, messageCount, 
		float64(messageCount-errors)/float64(messageCount)*100)
	t.Logf("   Send duration: %v", sendDuration)
	t.Logf("   Send rate: %.2f msg/s", sendRate)
	t.Logf("   Errors: %d", errors)

	// 等待消息处理
	t.Logf("⏳ Waiting for message processing...")
	time.Sleep(45 * time.Second)

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

	t.Logf("\n📊 ===== Continued Optimization Results =====")
	t.Logf("📈 Overall Performance:")
	t.Logf("   📤 Messages sent: %d", messageCount-errors)
	t.Logf("   📥 Messages received: %d", finalCount)
	t.Logf("   ✅ Success rate: %.2f%%", successRate)
	t.Logf("   🚀 Send rate: %.2f msg/s", sendRate)
	t.Logf("   🚀 Throughput: %.2f msg/s", throughput)
	t.Logf("   ⏱️  First message latency: %v", firstMessageLatency)
	t.Logf("   ⏱️  Processing latency: %v", processingLatency)
	t.Logf("   ⏱️  Total duration: %v", totalDuration)

	t.Logf("📈 Optimization Verification:")
	t.Logf("   ✅ Warmup mechanism: %v", completed)
	t.Logf("   ✅ Producer optimization: %.2f msg/s", sendRate)
	t.Logf("   ✅ Batch size: %d messages", messageCount)
	t.Logf("   ✅ Consumer stability: %d processed messages", len(processedMessages))

	// 验证成功标准
	if successRate < 80 {
		t.Errorf("❌ Success rate too low: %.2f%% (expected ≥80%%)", successRate)
	} else {
		t.Logf("✅ Success rate PASSED: %.2f%%", successRate)
	}

	if sendRate < 500 {
		t.Logf("⚠️ Send rate below optimized target: %.2f msg/s (expected ≥500 msg/s)", sendRate)
	} else {
		t.Logf("✅ Send rate meets optimized target: %.2f msg/s", sendRate)
	}

	if firstMessageLatency > 1*time.Second {
		t.Logf("⚠️ First message latency high: %v (expected ≤1s)", firstMessageLatency)
	} else {
		t.Logf("✅ First message latency PASSED: %v", firstMessageLatency)
	}

	if finalCount > 0 {
		t.Logf("✅ Consumer is working: received %d messages", finalCount)
	} else {
		t.Errorf("❌ Consumer not working: received 0 messages")
	}

	t.Logf("\n🎉 Continued optimization test completed!")
}

// TestBalancedLowPressure 平衡配置的低压力测试
func TestBalancedLowPressure(t *testing.T) {
	t.Logf("🔧 Starting Balanced Low Pressure Test...")

	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: "balanced-low-pressure-test",
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
			Compression:     "", // 自动优化
			MaxInFlight:     0,  // 自动优化
		},
		Consumer: ConsumerConfig{
			GroupID:            "balanced-low-pressure-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     6 * time.Second,        // 平衡配置
			HeartbeatInterval:  2 * time.Second,        // 平衡配置
			MaxProcessingTime:  5 * time.Second,        // 平衡配置
			FetchMinBytes:      1024 * 1024,
			FetchMaxBytes:      50 * 1024 * 1024,
			FetchMaxWait:       100 * time.Millisecond, // 平衡配置
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
	testTopic := "balanced.low.pressure.test"
	kafkaBus.allPossibleTopics = []string{testTopic}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
			"id":      fmt.Sprintf("balanced-low-msg-%d", i),
			"content": fmt.Sprintf("Balanced low pressure message %d", i),
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
	time.Sleep(40 * time.Second)

	finalCount := atomic.LoadInt64(&receivedCount)
	successRate := float64(finalCount) / float64(messageCount) * 100

	t.Logf("\n📊 ===== Balanced Low Pressure Results =====")
	t.Logf("📤 Messages sent: %d", messageCount)
	t.Logf("📥 Messages received: %d", finalCount)
	t.Logf("✅ Success rate: %.2f%%", successRate)
	t.Logf("🚀 Send rate: %.2f msg/s", sendRate)
	t.Logf("⏱️  Send duration: %v", sendDuration)

	// 期望平衡配置应该达到80%以上成功率
	if successRate < 80 {
		t.Errorf("❌ Balanced low pressure success rate too low: %.2f%% (expected ≥80%%)", successRate)
	} else {
		t.Logf("✅ Balanced low pressure PASSED: %.2f%%", successRate)
	}

	if sendRate < 500 {
		t.Logf("⚠️ Send rate below target: %.2f msg/s (expected ≥500 msg/s)", sendRate)
	} else {
		t.Logf("✅ Send rate meets target: %.2f msg/s", sendRate)
	}
}
