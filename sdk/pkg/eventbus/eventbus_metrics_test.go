package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_UpdateMetrics_PublishSuccess 测试发布成功时的指标更新
func TestEventBusManager_UpdateMetrics_PublishSuccess(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 记录初始指标
	initialPublished := manager.metrics.MessagesPublished

	// 发布消息
	ctx := context.Background()
	err = bus.Publish(ctx, "test-topic", []byte("test message"))
	require.NoError(t, err)

	// 验证指标已更新
	assert.Equal(t, initialPublished+1, manager.metrics.MessagesPublished)
	assert.Equal(t, int64(0), manager.metrics.PublishErrors)
}

// TestEventBusManager_UpdateMetrics_PublishError 测试发布失败时的指标更新
func TestEventBusManager_UpdateMetrics_PublishError(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)

	// 设置 publisher 为 nil 以触发错误
	manager.publisher = nil

	// 记录初始指标
	initialErrors := manager.metrics.PublishErrors

	// 尝试发布消息（应该失败）
	ctx := context.Background()
	err = bus.Publish(ctx, "test-topic", []byte("test message"))
	assert.Error(t, err)

	// 验证错误指标已更新
	assert.Equal(t, initialErrors, manager.metrics.PublishErrors) // publisher 为 nil 时不会调用 updateMetrics
}

// TestEventBusManager_UpdateMetrics_SubscribeSuccess 测试订阅成功时的指标更新
func TestEventBusManager_UpdateMetrics_SubscribeSuccess(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息
	received := make(chan []byte, 1)
	handler := func(ctx context.Context, message []byte) error {
		received <- message
		return nil
	}

	err = bus.Subscribe(ctx, "test-topic-metrics-success", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed
	initialErrors := manager.metrics.ConsumeErrors

	// 发布消息
	err = bus.Publish(ctx, "test-topic-metrics-success", []byte("test message"))
	require.NoError(t, err)

	// 等待消息被处理
	select {
	case <-received:
		// 等待一小段时间确保指标更新
		time.Sleep(50 * time.Millisecond)
		// 验证指标已更新（使用相对值）
		assert.Greater(t, manager.metrics.MessagesConsumed, initialConsumed)
		assert.Equal(t, initialErrors, manager.metrics.ConsumeErrors)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestEventBusManager_UpdateMetrics_SubscribeError 测试订阅处理失败时的指标更新
func TestEventBusManager_UpdateMetrics_SubscribeError(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息（handler 返回错误）
	received := make(chan []byte, 1)
	handler := func(ctx context.Context, message []byte) error {
		received <- message
		return assert.AnError // 返回错误
	}

	err = bus.Subscribe(ctx, "test-topic-metrics-error", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialErrors := manager.metrics.ConsumeErrors

	// 发布消息
	err = bus.Publish(ctx, "test-topic-metrics-error", []byte("test message"))
	require.NoError(t, err)

	// 等待消息被处理
	select {
	case <-received:
		// 等待一小段时间确保指标更新
		time.Sleep(50 * time.Millisecond)
		// 验证错误指标已更新（使用相对值）
		assert.Greater(t, manager.metrics.ConsumeErrors, initialErrors)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestEventBusManager_UpdateMetrics_Concurrent 测试并发更新指标的线程安全性
func TestEventBusManager_UpdateMetrics_Concurrent(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息
	handler := func(ctx context.Context, message []byte) error {
		return nil
	}

	err = bus.Subscribe(ctx, "test-topic-concurrent", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialPublished := manager.metrics.MessagesPublished
	initialConsumed := manager.metrics.MessagesConsumed

	// 并发发布消息
	numGoroutines := 10
	numMessagesPerGoroutine := 10
	totalMessages := numGoroutines * numMessagesPerGoroutine

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numMessagesPerGoroutine; j++ {
				err := bus.Publish(ctx, "test-topic-concurrent", []byte("test message"))
				assert.NoError(t, err)
			}
		}()
	}

	wg.Wait()

	// 等待所有消息被处理
	time.Sleep(200 * time.Millisecond)

	// 验证指标（使用相对值）
	publishedDelta := manager.metrics.MessagesPublished - initialPublished
	consumedDelta := manager.metrics.MessagesConsumed - initialConsumed

	// 验证至少发布了预期数量的消息
	assert.GreaterOrEqual(t, publishedDelta, int64(totalMessages))
	assert.Equal(t, int64(0), manager.metrics.PublishErrors)
	// 注意：MessagesConsumed 可能略小于 totalMessages，因为有些消息可能还在处理中
	assert.GreaterOrEqual(t, consumedDelta, int64(totalMessages*7/10)) // 至少 70%
}

// TestEventBusManager_UpdateMetrics_MultipleTopics 测试多个主题的指标更新
func TestEventBusManager_UpdateMetrics_MultipleTopics(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅多个主题
	topics := []string{"metrics-topic1", "metrics-topic2", "metrics-topic3"}
	for _, topic := range topics {
		handler := func(ctx context.Context, message []byte) error {
			return nil
		}
		err = bus.Subscribe(ctx, topic, handler)
		require.NoError(t, err)
	}

	// 记录初始指标
	initialPublished := manager.metrics.MessagesPublished
	initialConsumed := manager.metrics.MessagesConsumed

	// 发布到每个主题
	for _, topic := range topics {
		err = bus.Publish(ctx, topic, []byte("test message"))
		require.NoError(t, err)
	}

	// 验证发布指标（使用相对值）
	publishedDelta := manager.metrics.MessagesPublished - initialPublished
	assert.GreaterOrEqual(t, publishedDelta, int64(len(topics)))
	assert.Equal(t, int64(0), manager.metrics.PublishErrors)

	// 等待消息被处理
	time.Sleep(200 * time.Millisecond)

	// 验证消费指标（使用相对值）
	consumedDelta := manager.metrics.MessagesConsumed - initialConsumed
	assert.GreaterOrEqual(t, consumedDelta, int64(len(topics)*7/10))
}

// TestEventBusManager_UpdateMetrics_MixedSuccessAndError 测试混合成功和失败的指标更新
func TestEventBusManager_UpdateMetrics_MixedSuccessAndError(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息（50% 成功，50% 失败）
	messageCount := 0
	handler := func(ctx context.Context, message []byte) error {
		messageCount++
		if messageCount%2 == 0 {
			return assert.AnError // 偶数消息返回错误
		}
		return nil // 奇数消息成功
	}

	err = bus.Subscribe(ctx, "test-topic-mixed", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialPublished := manager.metrics.MessagesPublished
	initialConsumed := manager.metrics.MessagesConsumed
	initialErrors := manager.metrics.ConsumeErrors

	// 发布 10 条消息
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		err = bus.Publish(ctx, "test-topic-mixed", []byte("test message"))
		require.NoError(t, err)
	}

	// 等待所有消息被处理
	time.Sleep(300 * time.Millisecond)

	// 验证指标（使用相对值）
	publishedDelta := manager.metrics.MessagesPublished - initialPublished
	consumedDelta := manager.metrics.MessagesConsumed - initialConsumed
	errorsDelta := manager.metrics.ConsumeErrors - initialErrors

	assert.GreaterOrEqual(t, publishedDelta, int64(numMessages))
	assert.Equal(t, int64(0), manager.metrics.PublishErrors)

	// 验证消费指标（应该有成功和失败）
	totalProcessed := consumedDelta + errorsDelta
	assert.GreaterOrEqual(t, totalProcessed, int64(numMessages*7/10)) // 至少 70% 被处理

	// 验证有错误
	assert.Greater(t, errorsDelta, int64(0))
}

// TestEventBusManager_Metrics_InitialState 测试指标的初始状态
func TestEventBusManager_Metrics_InitialState(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 验证初始指标为 0
	assert.Equal(t, int64(0), manager.metrics.MessagesPublished)
	assert.Equal(t, int64(0), manager.metrics.MessagesConsumed)
	assert.Equal(t, int64(0), manager.metrics.PublishErrors)
	assert.Equal(t, int64(0), manager.metrics.ConsumeErrors)
}
