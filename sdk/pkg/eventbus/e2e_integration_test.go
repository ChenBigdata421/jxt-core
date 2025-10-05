package eventbus

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_MemoryEventBus_WithEnvelope 测试 Memory EventBus 端到端流程（使用 Envelope）
func TestE2E_MemoryEventBus_WithEnvelope(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "memory",
	}

	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "orders.events"

	// 订阅 Envelope 消息
	var received atomic.Int32
	var receivedEnvelopes []*Envelope
	var mu sync.Mutex

	handler := func(ctx context.Context, env *Envelope) error {
		mu.Lock()
		receivedEnvelopes = append(receivedEnvelopes, env)
		mu.Unlock()
		received.Add(1)
		return nil
	}

	err = bus.SubscribeEnvelope(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布多个 Envelope 消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		envelope := NewEnvelope(
			"order-123",
			"OrderCreated",
			int64(i+1),
			[]byte(`{"orderId":"order-123","amount":100}`),
		)

		data, err := json.Marshal(envelope)
		require.NoError(t, err)

		err = bus.Publish(ctx, topic, data)
		require.NoError(t, err)
	}

	// 等待所有消息被接收
	time.Sleep(500 * time.Millisecond)

	// 验证接收
	assert.Equal(t, int32(messageCount), received.Load())
	assert.Equal(t, messageCount, len(receivedEnvelopes))

	// 验证所有消息都被接收（不验证顺序，因为是并发处理）
	versions := make(map[int64]bool)
	for _, env := range receivedEnvelopes {
		assert.Equal(t, "order-123", env.AggregateID)
		assert.Equal(t, "OrderCreated", env.EventType)
		versions[env.EventVersion] = true
	}

	// 验证所有版本都存在
	for i := 1; i <= messageCount; i++ {
		assert.True(t, versions[int64(i)], "Version %d should exist", i)
	}
}

// TestE2E_MemoryEventBus_MultipleTopics 测试多主题端到端流程
func TestE2E_MemoryEventBus_MultipleTopics(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "memory",
	}

	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅多个主题
	topics := []string{"topic1", "topic2", "topic3"}
	counters := make(map[string]*atomic.Int32)
	var mu sync.Mutex

	for _, topic := range topics {
		counter := &atomic.Int32{}
		counters[topic] = counter

		handler := func(t string, c *atomic.Int32) MessageHandler {
			return func(ctx context.Context, data []byte) error {
				c.Add(1)
				return nil
			}
		}(topic, counter)

		err = bus.Subscribe(ctx, topic, handler)
		require.NoError(t, err)
	}

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 向每个主题发布消息
	messagesPerTopic := 3
	for _, topic := range topics {
		for i := 0; i < messagesPerTopic; i++ {
			message := []byte("message " + string(rune('0'+i)))
			err = bus.Publish(ctx, topic, message)
			require.NoError(t, err)
		}
	}

	// 等待所有消息被接收
	time.Sleep(500 * time.Millisecond)

	// 验证每个主题都收到了正确数量的消息
	mu.Lock()
	defer mu.Unlock()
	for topic, counter := range counters {
		assert.Equal(t, int32(messagesPerTopic), counter.Load(), "Topic: %s", topic)
	}
}

// TestE2E_MemoryEventBus_ConcurrentPublishSubscribe 测试并发发布订阅
func TestE2E_MemoryEventBus_ConcurrentPublishSubscribe(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "memory",
	}

	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "concurrent.test"

	// 订阅消息
	var received atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		received.Add(1)
		// 模拟处理时间
		time.Sleep(1 * time.Millisecond)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 并发发布消息
	goroutines := 10
	messagesPerGoroutine := 10
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				message := []byte("message")
				err := bus.Publish(ctx, topic, message)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// 等待所有消息被接收
	time.Sleep(2 * time.Second)

	// 验证接收到所有消息
	expectedCount := int32(goroutines * messagesPerGoroutine)
	assert.Equal(t, expectedCount, received.Load())
}

// TestE2E_MemoryEventBus_ErrorRecovery 测试错误恢复
func TestE2E_MemoryEventBus_ErrorRecovery(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "memory",
	}

	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "error.recovery"

	// 订阅消息 - 前几次返回错误，然后成功
	var attemptCount atomic.Int32
	var successCount atomic.Int32

	handler := func(ctx context.Context, data []byte) error {
		count := attemptCount.Add(1)
		if count <= 3 {
			// 前3次返回错误
			return assert.AnError
		}
		// 之后成功
		successCount.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		message := []byte("message " + string(rune('0'+i)))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待所有消息被处理
	time.Sleep(500 * time.Millisecond)

	// 验证处理器被调用
	assert.Greater(t, attemptCount.Load(), int32(0))
	// 验证有成功的处理
	assert.Greater(t, successCount.Load(), int32(0))
}

// TestE2E_MemoryEventBus_ContextCancellation 测试上下文取消
func TestE2E_MemoryEventBus_ContextCancellation(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "memory",
	}

	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	topic := "context.cancel"

	// 订阅消息
	var received atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		received.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布一些消息
	for i := 0; i < 3; i++ {
		message := []byte("message " + string(rune('0'+i)))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待消息被接收
	time.Sleep(200 * time.Millisecond)

	// 取消上下文
	cancel()

	// 尝试发布更多消息（应该失败或被取消）
	message := []byte("message after cancel")
	err = bus.Publish(ctx, topic, message)
	// 上下文已取消，发布可能失败
	if err != nil {
		assert.Contains(t, err.Error(), "context")
	}

	// 验证之前的消息被接收
	assert.Greater(t, received.Load(), int32(0))
}

// TestE2E_MemoryEventBus_Metrics 测试指标收集
func TestE2E_MemoryEventBus_Metrics(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "memory",
	}

	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "metrics.test"

	// 订阅消息
	handler := func(ctx context.Context, data []byte) error {
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		message := []byte("message " + string(rune('0'+i)))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待消息被处理
	time.Sleep(500 * time.Millisecond)

	// 获取指标
	metrics := bus.GetMetrics()
	require.NotNil(t, metrics)

	// 验证指标
	assert.Greater(t, metrics.MessagesPublished, int64(0))
	assert.Greater(t, metrics.MessagesConsumed, int64(0))
}
