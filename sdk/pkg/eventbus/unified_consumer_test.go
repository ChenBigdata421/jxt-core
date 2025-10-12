package eventbus

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnifiedConsumerArchitecture 测试统一消费者组架构
func TestUnifiedConsumerArchitecture(t *testing.T) {
	// 使用内存实现进行基础架构测试
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 测试多topic订阅
	var mu sync.Mutex
	receivedMessages := make(map[string][]string)

	// 订阅多个topic
	topics := []string{"topic1", "topic2", "topic3"}
	for _, topic := range topics {
		topicName := topic // 避免闭包问题
		err := bus.Subscribe(ctx, topicName, func(ctx context.Context, data []byte) error {
			mu.Lock()
			defer mu.Unlock()
			receivedMessages[topicName] = append(receivedMessages[topicName], string(data))
			return nil
		})
		require.NoError(t, err)
	}

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发送消息到各个topic
	for i, topic := range topics {
		for j := 0; j < 3; j++ {
			message := fmt.Sprintf("message-%d-%d", i, j)
			err := bus.Publish(ctx, topic, []byte(message))
			require.NoError(t, err)
		}
	}

	// 等待消息处理
	time.Sleep(500 * time.Millisecond)

	// 验证消息接收
	mu.Lock()
	defer mu.Unlock()

	for _, topic := range topics {
		assert.Len(t, receivedMessages[topic], 3, "Topic %s should receive 3 messages", topic)
	}
}

// TestUnifiedConsumerDynamicSubscription 测试动态订阅
func TestUnifiedConsumerDynamicSubscription(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mu sync.Mutex
	receivedMessages := make(map[string][]string)

	// 先订阅一个topic
	err := bus.Subscribe(ctx, "topic1", func(ctx context.Context, data []byte) error {
		mu.Lock()
		defer mu.Unlock()
		receivedMessages["topic1"] = append(receivedMessages["topic1"], string(data))
		return nil
	})
	require.NoError(t, err)

	// 发送消息
	err = bus.Publish(ctx, "topic1", []byte("message1"))
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// 动态添加新的topic订阅
	err = bus.Subscribe(ctx, "topic2", func(ctx context.Context, data []byte) error {
		mu.Lock()
		defer mu.Unlock()
		receivedMessages["topic2"] = append(receivedMessages["topic2"], string(data))
		return nil
	})
	require.NoError(t, err)

	// 发送消息到两个topic
	err = bus.Publish(ctx, "topic1", []byte("message2"))
	require.NoError(t, err)
	err = bus.Publish(ctx, "topic2", []byte("message3"))
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// 验证消息接收
	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedMessages["topic1"], 2, "Topic1 should receive 2 messages")
	assert.Len(t, receivedMessages["topic2"], 1, "Topic2 should receive 1 message")
}

// TestUnifiedConsumerErrorHandling 测试错误处理
func TestUnifiedConsumerErrorHandling(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mu sync.Mutex
	errorCount := 0
	successCount := 0

	// 订阅一个会产生错误的handler
	err := bus.Subscribe(ctx, "error-topic", func(ctx context.Context, data []byte) error {
		mu.Lock()
		defer mu.Unlock()
		
		message := string(data)
		if message == "error" {
			errorCount++
			return fmt.Errorf("simulated error")
		}
		successCount++
		return nil
	})
	require.NoError(t, err)

	// 发送正常消息和错误消息
	err = bus.Publish(ctx, "error-topic", []byte("success"))
	require.NoError(t, err)
	err = bus.Publish(ctx, "error-topic", []byte("error"))
	require.NoError(t, err)
	err = bus.Publish(ctx, "error-topic", []byte("success"))
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, 2, successCount, "Should have 2 successful messages")
	assert.Equal(t, 1, errorCount, "Should have 1 error message")
}

// TestUnifiedConsumerConcurrency 测试并发处理
func TestUnifiedConsumerConcurrency(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var mu sync.Mutex
	processedCount := 0

	// 订阅topic
	err := bus.Subscribe(ctx, "concurrent-topic", func(ctx context.Context, data []byte) error {
		// 模拟处理时间
		time.Sleep(10 * time.Millisecond)
		
		mu.Lock()
		processedCount++
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)

	// 并发发送大量消息
	messageCount := 100
	var wg sync.WaitGroup
	
	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			message := fmt.Sprintf("message-%d", i)
			err := bus.Publish(ctx, "concurrent-topic", []byte(message))
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
	
	// 等待所有消息处理完成
	time.Sleep(2 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, messageCount, processedCount, "All messages should be processed")
}
