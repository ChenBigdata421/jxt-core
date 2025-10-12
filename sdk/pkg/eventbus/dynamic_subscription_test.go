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

// TestDynamicSubscriptionBasic 测试动态订阅基本功能
func TestDynamicSubscriptionBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping dynamic subscription basic test in short mode")
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
			GroupID:            "dynamic-subscription-basic-group",
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
		ClientID:            "dynamic-subscription-basic-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	t.Logf("🚀 Starting Dynamic Subscription Basic Test")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 测试数据
	topic := "dynamic.subscription.test"
	messageCount := 100
	var receivedCount int64

	// 消息处理器
	handler := func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&receivedCount, 1)
		received := atomic.LoadInt64(&receivedCount)
		if received%10 == 0 {
			t.Logf("📥 Received %d messages", received)
		}
		return nil
	}

	// 🚀 测试动态订阅
	t.Logf("📝 Subscribing to topic: %s", topic)
	if err := eventBus.Subscribe(ctx, topic, handler); err != nil {
		t.Fatalf("Failed to subscribe to topic: %v", err)
	}

	// 等待订阅建立
	t.Logf("⏳ Waiting for subscription to stabilize...")
	time.Sleep(5 * time.Second)

	// 发送消息
	t.Logf("📤 Sending %d messages", messageCount)
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":        fmt.Sprintf("dynamic-msg-%d", i+1),
			"sequence":  int64(i + 1),
			"timestamp": time.Now().Format(time.RFC3339Nano),
			"data":      fmt.Sprintf("Dynamic subscription test message %d", i+1),
		}
		
		messageBytes, _ := json.Marshal(message)
		
		if err := eventBus.Publish(ctx, topic, messageBytes); err != nil {
			t.Logf("❌ Failed to send message %d: %v", i+1, err)
		}
		
		// 小延迟
		if i%10 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	t.Logf("📤 Finished sending %d messages", messageCount)

	// 等待消息处理
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			received := atomic.LoadInt64(&receivedCount)
			successRate := float64(received) / float64(messageCount) * 100
			t.Logf("⏰ Test completed with timeout")
			t.Logf("📊 Final Results: %d/%d messages received (%.2f%% success rate)", 
				received, messageCount, successRate)
			
			if successRate >= 80.0 {
				t.Logf("✅ Dynamic subscription test PASSED (>= 80%% success rate)")
			} else {
				t.Errorf("❌ Dynamic subscription test FAILED (< 80%% success rate)")
			}
			return
			
		case <-ticker.C:
			received := atomic.LoadInt64(&receivedCount)
			if received >= int64(messageCount) {
				t.Logf("✅ All messages received successfully!")
				t.Logf("📊 Final Results: %d/%d messages received (100%% success rate)", 
					received, messageCount)
				t.Logf("✅ Dynamic subscription test PASSED")
				return
			}
		}
	}
}

// TestDynamicSubscriptionMultiTopic 测试动态订阅多topic功能
func TestDynamicSubscriptionMultiTopic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping dynamic subscription multi-topic test in short mode")
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
			GroupID:            "dynamic-subscription-multi-group",
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
		ClientID:            "dynamic-subscription-multi-client",
		HealthCheckInterval: 60 * time.Second,
	}

	eventBus, err := NewKafkaEventBus(redpandaConfig)
	if err != nil {
		t.Fatalf("Failed to create RedPanda EventBus: %v", err)
	}
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(3 * time.Second)

	t.Logf("🚀 Starting Dynamic Subscription Multi-Topic Test")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 测试数据
	topics := []string{
		"dynamic.multi.topic1",
		"dynamic.multi.topic2",
		"dynamic.multi.topic3",
	}
	messagesPerTopic := 50
	totalMessages := len(topics) * messagesPerTopic
	
	var receivedCount int64
	var mu sync.Mutex
	receivedByTopic := make(map[string]int64)

	// 消息处理器
	createHandler := func(topicName string) MessageHandler {
		return func(ctx context.Context, message []byte) error {
			atomic.AddInt64(&receivedCount, 1)
			
			mu.Lock()
			receivedByTopic[topicName]++
			mu.Unlock()
			
			received := atomic.LoadInt64(&receivedCount)
			if received%20 == 0 {
				t.Logf("📥 Received %d total messages", received)
			}
			return nil
		}
	}

	// 🚀 动态订阅多个topic（无需重启）
	for i, topic := range topics {
		t.Logf("📝 Subscribing to topic %d: %s", i+1, topic)
		handler := createHandler(topic)
		
		if err := eventBus.Subscribe(ctx, topic, handler); err != nil {
			t.Fatalf("Failed to subscribe to topic %s: %v", topic, err)
		}
		
		// 小延迟，观察动态添加效果
		time.Sleep(2 * time.Second)
	}

	// 等待所有订阅建立
	t.Logf("⏳ Waiting for all subscriptions to stabilize...")
	time.Sleep(5 * time.Second)

	// 发送消息到所有topic
	t.Logf("📤 Sending %d messages to %d topics (%d total)", 
		messagesPerTopic, len(topics), totalMessages)
	
	for _, topic := range topics {
		for i := 0; i < messagesPerTopic; i++ {
			message := map[string]interface{}{
				"id":        fmt.Sprintf("%s-msg-%d", topic, i+1),
				"topic":     topic,
				"sequence":  int64(i + 1),
				"timestamp": time.Now().Format(time.RFC3339Nano),
				"data":      fmt.Sprintf("Multi-topic test message %d for %s", i+1, topic),
			}
			
			messageBytes, _ := json.Marshal(message)
			
			if err := eventBus.Publish(ctx, topic, messageBytes); err != nil {
				t.Logf("❌ Failed to send message %d to topic %s: %v", i+1, topic, err)
			}
		}
		t.Logf("📤 Sent %d messages to topic: %s", messagesPerTopic, topic)
	}

	// 等待消息处理
	timeout := time.After(45 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			received := atomic.LoadInt64(&receivedCount)
			successRate := float64(received) / float64(totalMessages) * 100
			
			t.Logf("⏰ Test completed with timeout")
			t.Logf("📊 Final Results: %d/%d messages received (%.2f%% success rate)", 
				received, totalMessages, successRate)
			
			mu.Lock()
			for topic, count := range receivedByTopic {
				topicRate := float64(count) / float64(messagesPerTopic) * 100
				t.Logf("📊 Topic %s: %d/%d messages (%.2f%%)", 
					topic, count, messagesPerTopic, topicRate)
			}
			mu.Unlock()
			
			if successRate >= 70.0 {
				t.Logf("✅ Dynamic multi-topic subscription test PASSED (>= 70%% success rate)")
			} else {
				t.Errorf("❌ Dynamic multi-topic subscription test FAILED (< 70%% success rate)")
			}
			return
			
		case <-ticker.C:
			received := atomic.LoadInt64(&receivedCount)
			if received >= int64(totalMessages) {
				t.Logf("✅ All messages received successfully!")
				t.Logf("📊 Final Results: %d/%d messages received (100%% success rate)", 
					received, totalMessages)
				
				mu.Lock()
				for topic, count := range receivedByTopic {
					t.Logf("📊 Topic %s: %d/%d messages (100%%)", 
						topic, count, messagesPerTopic)
				}
				mu.Unlock()
				
				t.Logf("✅ Dynamic multi-topic subscription test PASSED")
				return
			}
		}
	}
}
