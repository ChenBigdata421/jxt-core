//go:build integration
// +build integration

package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
)

// TestPreSubscriptionBasic 测试预订阅模式基本功能
func TestPreSubscriptionBasic(t *testing.T) {
	// 🔧 预订阅功能需要 Kafka 持久化支持，在 short 模式下跳过
	if testing.Short() {
		t.Skip("Skipping pre-subscription test in short mode (requires Kafka)")
	}

	// 创建测试配置
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"}, // RedPanda
		ClientID: "pre-subscription-basic-test",
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         10 * time.Second,
			MaxInFlight:     2,
		},
		Consumer: ConsumerConfig{
			GroupID:            "pre-subscription-basic-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  45 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      2 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     50,
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
		},
	}

	// 创建EventBus
	eventBus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("Failed to create EventBus: %v", err)
	}
	defer eventBus.Close()

	// 设置预订阅topic列表
	kafkaBus := eventBus.(*kafkaEventBus)
	// 走公共方法（加锁 + 设 topicsSnapshot），勿直接戳 allPossibleTopics 字段
	kafkaBus.SetPreSubscriptionTopics([]string{"test.topic.1", "test.topic.2", "test.topic.3"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 测试消息计数
	var receivedCount int64
	var processedMsgs sync.Map

	// 订阅处理器
	handler := func(ctx context.Context, message []byte) error {
		var msg map[string]interface{}
		if err := jxtjson.Unmarshal(message, &msg); err != nil {
			return err
		}

		// 防止重复计数
		if msgID, ok := msg["id"].(string); ok {
			if _, exists := processedMsgs.LoadOrStore(msgID, true); !exists {
				atomic.AddInt64(&receivedCount, 1)
			}
		}
		return nil
	}

	// 订阅topic
	err = eventBus.Subscribe(ctx, "test.topic.1", handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 等待消费者启动
	time.Sleep(2 * time.Second)

	// 发送测试消息
	totalMessages := 50
	for i := 0; i < totalMessages; i++ {
		message := map[string]interface{}{
			"id":        fmt.Sprintf("msg-%d", i),
			"content":   fmt.Sprintf("Test message %d", i),
			"timestamp": time.Now().Unix(),
		}

		messageBytes, _ := jxtjson.Marshal(message)
		err := eventBus.Publish(ctx, "test.topic.1", messageBytes)
		if err != nil {
			t.Errorf("Failed to publish message %d: %v", i, err)
		}
	}

	// 等待消息处理
	time.Sleep(30 * time.Second)

	// 验证结果
	received := atomic.LoadInt64(&receivedCount)
	successRate := float64(received) / float64(totalMessages) * 100

	t.Logf("📊 Pre-subscription Basic Test Results:")
	t.Logf("📤 Messages sent: %d", totalMessages)
	t.Logf("📥 Messages received: %d", received)
	t.Logf("✅ Success rate: %.2f%%", successRate)

	if received == 0 {
		t.Errorf("No messages received")
	}

	if successRate < 80 {
		t.Errorf("Success rate too low: %.2f%% (expected >= 80%%)", successRate)
	}
}

// TestPreSubscriptionMultiTopic 测试预订阅模式多topic功能
func TestPreSubscriptionMultiTopic(t *testing.T) {
	// 🔧 预订阅功能需要 Kafka 持久化支持，在 short 模式下跳过
	if testing.Short() {
		t.Skip("Skipping pre-subscription multi-topic test in short mode (requires Kafka)")
	}

	// 创建测试配置
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"}, // RedPanda
		ClientID: "pre-subscription-multi-test",
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         10 * time.Second,
			MaxInFlight:     2,
		},
		Consumer: ConsumerConfig{
			GroupID:            "pre-subscription-multi-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  10 * time.Second,
			MaxProcessingTime:  45 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      2 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     100,
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
		},
	}

	// 创建EventBus
	eventBus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("Failed to create EventBus: %v", err)
	}
	defer eventBus.Close()

	// 设置预订阅topic列表
	kafkaBus := eventBus.(*kafkaEventBus)
	// 走公共方法（加锁 + 设 topicsSnapshot），勿直接戳 allPossibleTopics 字段
	kafkaBus.SetPreSubscriptionTopics([]string{"multi.topic.1", "multi.topic.2", "multi.topic.3"})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// 测试消息计数
	var receivedCount int64
	var processedMsgs sync.Map
	topicCounts := make(map[string]int64)
	var topicMu sync.Mutex

	// 订阅处理器
	handler := func(topicName string) MessageHandler {
		return func(ctx context.Context, message []byte) error {
			var msg map[string]interface{}
			if err := jxtjson.Unmarshal(message, &msg); err != nil {
				return err
			}

			// 防止重复计数
			if msgID, ok := msg["id"].(string); ok {
				if _, exists := processedMsgs.LoadOrStore(msgID, true); !exists {
					atomic.AddInt64(&receivedCount, 1)

					// 按topic计数
					topicMu.Lock()
					topicCounts[topicName]++
					topicMu.Unlock()
				}
			}
			return nil
		}
	}

	// 订阅多个topic
	topics := []string{"multi.topic.1", "multi.topic.2", "multi.topic.3"}
	for _, topic := range topics {
		err = eventBus.Subscribe(ctx, topic, handler(topic))
		if err != nil {
			t.Fatalf("Failed to subscribe to %s: %v", topic, err)
		}
		topicCounts[topic] = 0
	}

	// 等待消费者启动
	time.Sleep(3 * time.Second)

	// 发送测试消息到每个topic
	messagesPerTopic := 30
	totalMessages := len(topics) * messagesPerTopic

	for _, topic := range topics {
		for i := 0; i < messagesPerTopic; i++ {
			message := map[string]interface{}{
				"id":        fmt.Sprintf("%s-msg-%d", topic, i),
				"topic":     topic,
				"content":   fmt.Sprintf("Test message %d for %s", i, topic),
				"timestamp": time.Now().Unix(),
			}

			messageBytes, _ := jxtjson.Marshal(message)
			err := eventBus.Publish(ctx, topic, messageBytes)
			if err != nil {
				t.Errorf("Failed to publish message %d to %s: %v", i, topic, err)
			}
		}
	}

	// 等待消息处理
	time.Sleep(60 * time.Second)

	// 验证结果
	received := atomic.LoadInt64(&receivedCount)
	successRate := float64(received) / float64(totalMessages) * 100

	t.Logf("📊 Pre-subscription Multi-Topic Test Results:")
	t.Logf("📤 Total messages sent: %d", totalMessages)
	t.Logf("📥 Total messages received: %d", received)
	t.Logf("✅ Overall success rate: %.2f%%", successRate)

	// 按topic显示结果
	topicMu.Lock()
	for _, topic := range topics {
		count := topicCounts[topic]
		rate := float64(count) / float64(messagesPerTopic) * 100
		t.Logf("📊 %s: %d/%d (%.2f%%)", topic, count, messagesPerTopic, rate)
	}
	topicMu.Unlock()

	if received == 0 {
		t.Errorf("No messages received")
	}

	if successRate < 50 {
		t.Errorf("Success rate too low: %.2f%% (expected >= 50%%)", successRate)
	}
}
