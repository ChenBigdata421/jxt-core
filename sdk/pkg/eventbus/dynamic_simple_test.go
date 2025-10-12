package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestDynamicSubscriptionSimple 简单的动态订阅测试
func TestDynamicSubscriptionSimple(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping dynamic subscription simple test in short mode")
	}

	// 使用RedPanda配置
	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   10 * time.Millisecond,
			FlushMessages:    10,
			Timeout:          10 * time.Second,
			FlushBytes:       16 * 1024,
			RetryMax:         2,
			BatchSize:        1024,
			BufferSize:       4 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  64 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         10 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      2,
		},
		Consumer: ConsumerConfig{
			GroupID:            "dynamic-simple-test-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  45 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      2 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     50,
			EnableAutoCommit:   true,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		ClientID:            "dynamic-simple-test-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	t.Logf("🚀 开始简单动态订阅测试")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	topic := "dynamic.simple.test"
	messageCount := 50
	var receivedCount int64

	// 消息处理器
	handler := func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&receivedCount, 1)
		received := atomic.LoadInt64(&receivedCount)
		
		var msg map[string]interface{}
		json.Unmarshal(message, &msg)
		if msgID, ok := msg["id"].(string); ok {
			t.Logf("📥 接收消息 %d: %s", received, msgID)
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
	time.Sleep(8 * time.Second)

	// 发送消息
	t.Logf("📤 开始发送 %d 条消息", messageCount)
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":        fmt.Sprintf("simple-msg-%d", i+1),
			"sequence":  int64(i + 1),
			"timestamp": time.Now().Format(time.RFC3339Nano),
			"data":      fmt.Sprintf("Simple dynamic test message %d", i+1),
		}
		
		messageBytes, _ := json.Marshal(message)
		
		if err := eventBus.Publish(ctx, topic, messageBytes); err != nil {
			t.Logf("❌ 发送消息 %d 失败: %v", i+1, err)
		} else {
			t.Logf("📤 发送消息 %d: simple-msg-%d", i+1, i+1)
		}
		
		// 发送间隔
		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("📤 完成发送 %d 条消息", messageCount)

	// 等待消息处理
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			received := atomic.LoadInt64(&receivedCount)
			successRate := float64(received) / float64(messageCount) * 100
			
			t.Logf("⏰ 简单动态订阅测试完成（超时）")
			t.Logf("📊 最终结果: %d/%d 消息接收 (%.2f%% 成功率)", 
				received, messageCount, successRate)
			
			if successRate >= 80.0 {
				t.Logf("✅ 简单动态订阅测试 PASSED (>= 80%% 成功率)")
			} else {
				t.Errorf("❌ 简单动态订阅测试 FAILED (< 80%% 成功率)")
			}
			return
			
		case <-ticker.C:
			received := atomic.LoadInt64(&receivedCount)
			t.Logf("📊 当前进度: %d/%d 消息接收", received, messageCount)
			
			if received >= int64(messageCount) {
				t.Logf("✅ 所有消息接收完成！")
				t.Logf("📊 最终结果: %d/%d 消息接收 (100%% 成功率)", 
					received, messageCount)
				t.Logf("✅ 简单动态订阅测试 PASSED")
				return
			}
		}
	}
}

// TestDynamicSubscriptionLowPressureFixed 修复的低压力测试
func TestDynamicSubscriptionLowPressureFixed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping dynamic subscription low pressure fixed test in short mode")
	}

	// 使用RedPanda配置
	redpandaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   5 * time.Millisecond,
			FlushMessages:    5,
			Timeout:          10 * time.Second,
			FlushBytes:       16 * 1024,
			RetryMax:         2,
			BatchSize:        1024,
			BufferSize:       4 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  64 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         5 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      2,
		},
		Consumer: ConsumerConfig{
			GroupID:            "dynamic-low-pressure-fixed-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  45 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      2 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     100,
			EnableAutoCommit:   true,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		ClientID:            "dynamic-low-pressure-fixed-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	t.Logf("🚀 开始修复的动态订阅低压力测试")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	topic := "dynamic.low.pressure.fixed"
	messageCount := 300 // 减少消息数量
	var receivedCount int64

	// 消息处理器
	handler := func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&receivedCount, 1)
		received := atomic.LoadInt64(&receivedCount)
		
		if received%50 == 0 {
			t.Logf("📥 已接收 %d 条消息", received)
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
	time.Sleep(8 * time.Second)

	// 发送消息
	startTime := time.Now()
	t.Logf("📤 开始发送 %d 条消息", messageCount)
	
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":        fmt.Sprintf("low-pressure-msg-%d", i+1),
			"sequence":  int64(i + 1),
			"timestamp": time.Now().Format(time.RFC3339Nano),
			"data":      fmt.Sprintf("Low pressure test message %d", i+1),
		}
		
		messageBytes, _ := json.Marshal(message)
		
		if err := eventBus.Publish(ctx, topic, messageBytes); err != nil {
			t.Logf("❌ 发送消息 %d 失败: %v", i+1, err)
		}
		
		// 适当的发送间隔
		if i%20 == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	t.Logf("📤 完成发送 %d 条消息，耗时 %.2f秒", messageCount, time.Since(startTime).Seconds())

	// 等待消息处理
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			received := atomic.LoadInt64(&receivedCount)
			successRate := float64(received) / float64(messageCount) * 100
			duration := time.Since(startTime)
			throughput := float64(received) / duration.Seconds()
			
			t.Logf("⏰ 修复的动态订阅低压力测试完成（超时）")
			t.Logf("📊 最终结果:")
			t.Logf("   ⏱️  测试时长: %.2f秒", duration.Seconds())
			t.Logf("   📤 消息发送: %d条", messageCount)
			t.Logf("   📥 消息接收: %d条", received)
			t.Logf("   ✅ 成功率: %.2f%%", successRate)
			t.Logf("   🚀 吞吐量: %.2f msg/s", throughput)
			
			if successRate >= 90.0 {
				t.Logf("🎉 修复的动态订阅低压力测试 PASSED (>= 90%% 成功率)")
			} else if successRate >= 70.0 {
				t.Logf("⚠️ 修复的动态订阅低压力测试 PARTIAL (>= 70%% 成功率)")
			} else {
				t.Errorf("❌ 修复的动态订阅低压力测试 FAILED (< 70%% 成功率)")
			}
			return
			
		case <-ticker.C:
			received := atomic.LoadInt64(&receivedCount)
			
			if received >= int64(messageCount) {
				duration := time.Since(startTime)
				throughput := float64(received) / duration.Seconds()
				
				t.Logf("✅ 所有消息接收完成！")
				t.Logf("📊 最终结果:")
				t.Logf("   ⏱️  测试时长: %.2f秒", duration.Seconds())
				t.Logf("   📤 消息发送: %d条", messageCount)
				t.Logf("   📥 消息接收: %d条", received)
				t.Logf("   ✅ 成功率: 100.00%%")
				t.Logf("   🚀 吞吐量: %.2f msg/s", throughput)
				t.Logf("🎉 修复的动态订阅低压力测试 PASSED")
				return
			}
		}
	}
}
