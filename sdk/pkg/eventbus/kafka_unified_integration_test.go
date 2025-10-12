// 暂时禁用旧的集成测试，使用新的性能测试
// +build ignore

package eventbus

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKafkaUnifiedConsumerGroup 测试Kafka统一消费者组架构
// 注意：此测试需要运行的Kafka实例
func TestKafkaUnifiedConsumerGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka integration test in short mode")
	}

	// 创建Kafka配置
	kafkaConfig := &config.KafkaConfig{
		Brokers: []string{"localhost:29092"}, // 使用Docker Kafka端口
		Producer: config.ProducerConfig{
			RequiredAcks:   1,
			Timeout:        5 * time.Second,
			RetryMax:       2,
			Compression:    "none",
			FlushFrequency: 100 * time.Millisecond,
			BatchSize:      1024,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           "unified-test-group",
			SessionTimeout:    15 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 2 * time.Minute,
			AutoOffsetReset:   "earliest",
			FetchMinBytes:     1,
			FetchMaxBytes:     1024 * 1024,
			FetchMaxWait:      500 * time.Millisecond,
		},
	}

	// 创建EventBus配置
	eventBusConfig := &EventBusConfig{
		Type: "kafka",
	}

	bus, err := NewKafkaEventBusWithFullConfig(kafkaConfig, eventBusConfig)
	if err != nil {
		t.Skipf("Failed to create Kafka EventBus (Kafka may not be available): %v", err)
	}
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 测试多topic统一消费
	t.Run("MultiTopicUnifiedConsumption", func(t *testing.T) {
		testMultiTopicUnifiedConsumption(t, bus, ctx)
	})

	// 测试动态topic添加
	t.Run("DynamicTopicAddition", func(t *testing.T) {
		testDynamicTopicAddition(t, bus, ctx)
	})

	// 测试消费者组稳定性
	t.Run("ConsumerGroupStability", func(t *testing.T) {
		testConsumerGroupStability(t, bus, ctx)
	})
}

func testMultiTopicUnifiedConsumption(t *testing.T, bus EventBus, ctx context.Context) {
	var mu sync.Mutex
	receivedMessages := make(map[string][]string)

	// 订阅多个topic
	topics := []string{"unified-topic-1", "unified-topic-2", "unified-topic-3"}
	for _, topic := range topics {
		topicName := topic // 避免闭包问题
		err := bus.Subscribe(ctx, topicName, func(ctx context.Context, data []byte) error {
			mu.Lock()
			defer mu.Unlock()
			receivedMessages[topicName] = append(receivedMessages[topicName], string(data))
			t.Logf("Received message on %s: %s", topicName, string(data))
			return nil
		})
		require.NoError(t, err)
	}

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发送消息到各个topic
	for i, topic := range topics {
		for j := 0; j < 3; j++ {
			message := fmt.Sprintf("unified-message-%d-%d", i, j)
			err := bus.Publish(ctx, topic, []byte(message))
			require.NoError(t, err)
			t.Logf("Published message to %s: %s", topic, message)
		}
	}

	// 等待消息处理
	time.Sleep(5 * time.Second)

	// 验证消息接收
	mu.Lock()
	defer mu.Unlock()

	for _, topic := range topics {
		assert.GreaterOrEqual(t, len(receivedMessages[topic]), 3, 
			"Topic %s should receive at least 3 messages, got %d", 
			topic, len(receivedMessages[topic]))
	}

	t.Logf("Successfully tested unified consumption across %d topics", len(topics))
}

func testDynamicTopicAddition(t *testing.T, bus EventBus, ctx context.Context) {
	var mu sync.Mutex
	receivedMessages := make(map[string][]string)

	// 先订阅一个topic
	topic1 := "dynamic-topic-1"
	err := bus.Subscribe(ctx, topic1, func(ctx context.Context, data []byte) error {
		mu.Lock()
		defer mu.Unlock()
		receivedMessages[topic1] = append(receivedMessages[topic1], string(data))
		t.Logf("Received on %s: %s", topic1, string(data))
		return nil
	})
	require.NoError(t, err)

	// 发送消息到第一个topic
	err = bus.Publish(ctx, topic1, []byte("message-before-dynamic"))
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	// 动态添加第二个topic
	topic2 := "dynamic-topic-2"
	err = bus.Subscribe(ctx, topic2, func(ctx context.Context, data []byte) error {
		mu.Lock()
		defer mu.Unlock()
		receivedMessages[topic2] = append(receivedMessages[topic2], string(data))
		t.Logf("Received on %s: %s", topic2, string(data))
		return nil
	})
	require.NoError(t, err)

	// 等待动态订阅生效
	time.Sleep(2 * time.Second)

	// 发送消息到两个topic
	err = bus.Publish(ctx, topic1, []byte("message-after-dynamic"))
	require.NoError(t, err)
	err = bus.Publish(ctx, topic2, []byte("message-new-topic"))
	require.NoError(t, err)

	time.Sleep(3 * time.Second)

	// 验证消息接收
	mu.Lock()
	defer mu.Unlock()

	assert.GreaterOrEqual(t, len(receivedMessages[topic1]), 2, 
		"Topic1 should receive at least 2 messages")
	assert.GreaterOrEqual(t, len(receivedMessages[topic2]), 1, 
		"Topic2 should receive at least 1 message")

	t.Log("Successfully tested dynamic topic addition")
}

func testConsumerGroupStability(t *testing.T, bus EventBus, ctx context.Context) {
	var mu sync.Mutex
	messageCount := 0

	topic := "stability-topic"
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, data []byte) error {
		mu.Lock()
		defer mu.Unlock()
		messageCount++
		t.Logf("Processed message %d: %s", messageCount, string(data))
		return nil
	})
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 持续发送消息，测试消费者组稳定性
	totalMessages := 20
	for i := 0; i < totalMessages; i++ {
		message := fmt.Sprintf("stability-message-%d", i)
		err := bus.Publish(ctx, topic, []byte(message))
		require.NoError(t, err)
		
		// 间隔发送，模拟真实场景
		time.Sleep(100 * time.Millisecond)
	}

	// 等待所有消息处理完成
	time.Sleep(5 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	// 验证所有消息都被处理（允许一些延迟）
	assert.GreaterOrEqual(t, messageCount, totalMessages-2, 
		"Should process most messages, expected ~%d, got %d", 
		totalMessages, messageCount)

	t.Logf("Successfully tested consumer group stability with %d/%d messages processed", 
		messageCount, totalMessages)
}

// TestKafkaUnifiedConsumerGroupRebalance 测试统一消费者组不会产生再平衡问题
func TestKafkaUnifiedConsumerGroupRebalance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka rebalance test in short mode")
	}

	// 这个测试验证多个topic订阅不会导致再平衡
	// 在旧架构中，每个topic都会创建新的消费者组实例，导致持续再平衡
	// 在新架构中，所有topic共享一个消费者组实例，避免再平衡

	// 创建Kafka配置
	kafkaConfig := &config.KafkaConfig{
		Brokers: []string{"localhost:29092"},
		Producer: config.ProducerConfig{
			RequiredAcks:   1,
			Timeout:        5 * time.Second,
			RetryMax:       2,
			Compression:    "none",
			FlushFrequency: 100 * time.Millisecond,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           "rebalance-test-group",
			SessionTimeout:    15 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			AutoOffsetReset:   "earliest",
		},
	}

	// 创建EventBus配置
	eventBusConfig := &EventBusConfig{
		Type: "kafka",
	}

	bus, err := NewKafkaEventBusWithFullConfig(kafkaConfig, eventBusConfig)
	if err != nil {
		t.Skipf("Failed to create Kafka EventBus: %v", err)
	}
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var mu sync.Mutex
	processedMessages := 0

	// 快速连续订阅多个topic（在旧架构中这会导致大量再平衡）
	topics := []string{"rebalance-topic-1", "rebalance-topic-2", "rebalance-topic-3", 
		"rebalance-topic-4", "rebalance-topic-5"}

	for i, topic := range topics {
		topicName := topic
		err := bus.Subscribe(ctx, topicName, func(ctx context.Context, data []byte) error {
			mu.Lock()
			defer mu.Unlock()
			processedMessages++
			t.Logf("Processed message from %s: %s (total: %d)", topicName, string(data), processedMessages)
			return nil
		})
		require.NoError(t, err)
		
		t.Logf("Subscribed to topic %d: %s", i+1, topicName)
		
		// 短暂等待，模拟实际应用中的订阅时序
		time.Sleep(500 * time.Millisecond)
	}

	// 等待所有订阅生效
	time.Sleep(3 * time.Second)

	// 向所有topic发送消息
	totalMessages := 0
	for _, topic := range topics {
		for j := 0; j < 3; j++ {
			message := fmt.Sprintf("rebalance-test-%s-%d", topic, j)
			err := bus.Publish(ctx, topic, []byte(message))
			require.NoError(t, err)
			totalMessages++
		}
	}

	t.Logf("Published %d messages across %d topics", totalMessages, len(topics))

	// 等待消息处理（如果有再平衡问题，消息处理会被频繁中断）
	time.Sleep(10 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	// 验证大部分消息都被成功处理（证明没有严重的再平衡问题）
	expectedMinimum := int(float64(totalMessages) * 0.8) // 至少80%的消息应该被处理
	assert.GreaterOrEqual(t, processedMessages, expectedMinimum,
		"Should process at least %d messages (80%% of %d), but only processed %d. "+
			"This may indicate rebalancing issues.", 
		expectedMinimum, totalMessages, processedMessages)

	t.Logf("✅ Rebalance test passed: %d/%d messages processed (%.1f%%)", 
		processedMessages, totalMessages, 
		float64(processedMessages)/float64(totalMessages)*100)
}
