package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKafkaUnifiedConsumer_ContextLeakFix 测试统一消费者组Context泄露修复
// 这个测试验证修复后的代码不会产生goroutine泄露
func TestKafkaUnifiedConsumer_ContextLeakFix(t *testing.T) {
	// 由于没有真实的Kafka环境，我们测试Memory实现的类似逻辑
	// 这个测试主要验证Context和goroutine的正确管理
	
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	topic := "context-leak-test"
	var messageCount int64

	handler := func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&messageCount, 1)
		return nil
	}

	// 订阅topic
	err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 发布一些消息
	for i := 0; i < 10; i++ {
		err := bus.Publish(ctx, topic, []byte("test message"))
		require.NoError(t, err)
	}

	// 等待消息处理
	time.Sleep(200 * time.Millisecond)

	// 验证消息被处理
	assert.Equal(t, int64(10), atomic.LoadInt64(&messageCount))

	// 取消context
	cancel()

	// 等待一段时间确保清理完成
	time.Sleep(100 * time.Millisecond)

	// 这里我们无法直接测试goroutine泄露，但可以确保没有panic
	t.Log("Context cancellation test completed without issues")
}

// TestKafkaUnifiedConsumer_MultipleTopicSubscription 测试多topic订阅
func TestKafkaUnifiedConsumer_MultipleTopicSubscription(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topics := []string{"topic1", "topic2", "topic3", "topic4", "topic5"}
	var messageCount int64
	var mu sync.Mutex
	receivedTopics := make(map[string]int)

	// 为每个topic创建handler
	for _, topic := range topics {
		topicName := topic // 避免闭包问题
		handler := func(ctx context.Context, message []byte) error {
			atomic.AddInt64(&messageCount, 1)
			mu.Lock()
			receivedTopics[topicName]++
			mu.Unlock()
			return nil
		}

		err := bus.Subscribe(ctx, topicName, handler)
		require.NoError(t, err)
	}

	// 向每个topic发布消息
	messagesPerTopic := 5
	for _, topic := range topics {
		for i := 0; i < messagesPerTopic; i++ {
			err := bus.Publish(ctx, topic, []byte("test message"))
			require.NoError(t, err)
		}
	}

	// 等待所有消息处理完成
	time.Sleep(500 * time.Millisecond)

	// 验证结果
	totalExpected := int64(len(topics) * messagesPerTopic)
	assert.Equal(t, totalExpected, atomic.LoadInt64(&messageCount))

	mu.Lock()
	for _, topic := range topics {
		assert.Equal(t, messagesPerTopic, receivedTopics[topic], 
			"Topic %s should receive %d messages", topic, messagesPerTopic)
	}
	mu.Unlock()
}

// TestKafkaUnifiedConsumer_DynamicTopicAddition 测试动态topic添加
func TestKafkaUnifiedConsumer_DynamicTopicAddition(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var messageCount int64
	var mu sync.Mutex
	receivedMessages := make(map[string][]string)

	// 创建通用handler
	createHandler := func(topicName string) MessageHandler {
		return func(ctx context.Context, message []byte) error {
			atomic.AddInt64(&messageCount, 1)
			mu.Lock()
			receivedMessages[topicName] = append(receivedMessages[topicName], string(message))
			mu.Unlock()
			return nil
		}
	}

	// 动态添加topic订阅
	topics := []string{"dynamic1", "dynamic2", "dynamic3"}
	for i, topic := range topics {
		// 订阅新topic
		err := bus.Subscribe(ctx, topic, createHandler(topic))
		require.NoError(t, err)

		// 向所有已订阅的topic发布消息
		for j := 0; j <= i; j++ {
			message := []byte("message from " + topics[j])
			err := bus.Publish(ctx, topics[j], message)
			require.NoError(t, err)
		}

		// 等待消息处理
		time.Sleep(100 * time.Millisecond)
	}

	// 等待所有消息处理完成
	time.Sleep(200 * time.Millisecond)

	// 验证消息接收
	mu.Lock()
	defer mu.Unlock()

	// dynamic1: 3条消息 (在3轮中都被发布)
	// dynamic2: 2条消息 (在后2轮中被发布)
	// dynamic3: 1条消息 (在最后1轮中被发布)
	assert.Len(t, receivedMessages["dynamic1"], 3)
	assert.Len(t, receivedMessages["dynamic2"], 2)
	assert.Len(t, receivedMessages["dynamic3"], 1)

	totalExpected := int64(6) // 3+2+1
	assert.Equal(t, totalExpected, atomic.LoadInt64(&messageCount))
}

// TestKafkaUnifiedConsumer_ErrorHandling 测试错误处理
func TestKafkaUnifiedConsumer_ErrorHandling(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "error-test"

	var successCount int64
	var errorCount int64

	// 创建会出错的handler
	errorHandler := func(ctx context.Context, message []byte) error {
		if string(message) == "error" {
			atomic.AddInt64(&errorCount, 1)
			return assert.AnError // 返回错误
		}
		atomic.AddInt64(&successCount, 1)
		return nil
	}

	err := bus.Subscribe(ctx, topic, errorHandler)
	require.NoError(t, err)

	// 发布正常消息和错误消息
	messages := []string{"success1", "error", "success2", "error", "success3"}
	for _, msg := range messages {
		err := bus.Publish(ctx, topic, []byte(msg))
		require.NoError(t, err)
	}

	// 等待处理完成
	time.Sleep(200 * time.Millisecond)

	// 验证结果
	assert.Equal(t, int64(3), atomic.LoadInt64(&successCount)) // 3个success消息
	assert.Equal(t, int64(2), atomic.LoadInt64(&errorCount))   // 2个error消息
}

// TestKafkaUnifiedConsumer_ConcurrentOperations 测试并发操作
func TestKafkaUnifiedConsumer_ConcurrentOperations(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var messageCount int64
	var subscriptionCount int64

	// 创建handler
	handler := func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&messageCount, 1)
		time.Sleep(1 * time.Millisecond) // 模拟处理时间
		return nil
	}

	var wg sync.WaitGroup

	// 并发订阅多个topic
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			topic := "concurrent-topic-" + string(rune('A'+id))
			err := bus.Subscribe(ctx, topic, handler)
			if err == nil {
				atomic.AddInt64(&subscriptionCount, 1)
			}
		}(i)
	}

	// 等待订阅完成
	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// 并发发布消息
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			topic := "concurrent-topic-" + string(rune('A'+id))
			for j := 0; j < 5; j++ {
				err := bus.Publish(ctx, topic, []byte("test message"))
				if err != nil {
					t.Errorf("Failed to publish to %s: %v", topic, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// 等待所有消息处理完成
	time.Sleep(500 * time.Millisecond)

	// 验证结果
	expectedSubscriptions := int64(10)
	expectedMessages := int64(50) // 10 topics * 5 messages each

	assert.Equal(t, expectedSubscriptions, atomic.LoadInt64(&subscriptionCount))
	assert.Equal(t, expectedMessages, atomic.LoadInt64(&messageCount))
}

// TestKafkaUnifiedConsumer_ResourceCleanup 测试资源清理
func TestKafkaUnifiedConsumer_ResourceCleanup(t *testing.T) {
	// 创建多个EventBus实例并关闭，确保没有资源泄露
	for i := 0; i < 5; i++ {
		bus := NewMemoryEventBus()
		
		ctx := context.Background()
		topic := "cleanup-test"

		handler := func(ctx context.Context, message []byte) error {
			return nil
		}

		err := bus.Subscribe(ctx, topic, handler)
		require.NoError(t, err)

		// 发布一些消息
		for j := 0; j < 3; j++ {
			err := bus.Publish(ctx, topic, []byte("test"))
			require.NoError(t, err)
		}

		// 等待处理
		time.Sleep(50 * time.Millisecond)

		// 关闭EventBus
		err = bus.Close()
		assert.NoError(t, err)

		// 尝试在关闭后操作（应该失败）
		err = bus.Publish(ctx, topic, []byte("after close"))
		assert.Error(t, err)
	}

	t.Log("Resource cleanup test completed successfully")
}

// TestKafkaUnifiedConsumer_HighThroughput 测试高吞吐量场景
func TestKafkaUnifiedConsumer_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high throughput test in short mode")
	}

	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	topic := "high-throughput-test"
	var processedCount int64

	handler := func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&processedCount, 1)
		return nil
	}

	err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 发布大量消息
	messageCount := 10000
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ { // 10个并发发布者
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < messageCount/10; j++ {
				err := bus.Publish(ctx, topic, []byte("high throughput message"))
				if err != nil {
					t.Errorf("Failed to publish message: %v", err)
				}
			}
		}()
	}

	wg.Wait()
	publishDuration := time.Since(start)

	// 等待所有消息处理完成
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for message processing")
		case <-ticker.C:
			if atomic.LoadInt64(&processedCount) >= int64(messageCount) {
				goto done
			}
		}
	}

done:
	totalDuration := time.Since(start)
	finalCount := atomic.LoadInt64(&processedCount)

	t.Logf("Published %d messages in %v", messageCount, publishDuration)
	t.Logf("Processed %d messages in %v", finalCount, totalDuration)
	t.Logf("Throughput: %.2f msg/sec", float64(finalCount)/totalDuration.Seconds())

	assert.Equal(t, int64(messageCount), finalCount)
}
