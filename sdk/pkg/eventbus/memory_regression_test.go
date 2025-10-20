package eventbus

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMemoryEventBus_ClosedPublish 测试关闭后发布
func TestMemoryEventBus_ClosedPublish(t *testing.T) {
	bus := NewMemoryEventBus()
	ctx := context.Background()

	// 关闭 EventBus
	err := bus.Close()
	require.NoError(t, err)

	// 尝试发布消息
	err = bus.Publish(ctx, "test-topic", []byte("message"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestMemoryEventBus_ClosedSubscribe 测试关闭后订阅
func TestMemoryEventBus_ClosedSubscribe(t *testing.T) {
	bus := NewMemoryEventBus()
	ctx := context.Background()

	// 关闭 EventBus
	err := bus.Close()
	require.NoError(t, err)

	// 尝试订阅
	handler := func(ctx context.Context, data []byte) error {
		return nil
	}
	err = bus.Subscribe(ctx, "test-topic", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestMemoryEventBus_PublishNoSubscribers 测试没有订阅者时发布
func TestMemoryEventBus_PublishNoSubscribers(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()

	// 发布到没有订阅者的主题
	err := bus.Publish(ctx, "no-subscribers", []byte("message"))
	assert.NoError(t, err) // 应该成功，只是没有订阅者
}

// TestMemoryEventBus_HandlerPanic 测试 handler panic 恢复
func TestMemoryEventBus_HandlerPanic(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "panic-topic"

	var panicCount atomic.Int32
	var normalCount atomic.Int32

	// 会 panic 的 handler
	panicHandler := func(ctx context.Context, data []byte) error {
		panicCount.Add(1)
		panic("intentional panic")
	}

	// 正常的 handler
	normalHandler := func(ctx context.Context, data []byte) error {
		normalCount.Add(1)
		return nil
	}

	// 订阅两个 handler
	err := bus.Subscribe(ctx, topic, panicHandler)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, topic, normalHandler)
	require.NoError(t, err)

	// 发布消息
	err = bus.Publish(ctx, topic, []byte("test"))
	require.NoError(t, err)

	// 等待处理
	time.Sleep(200 * time.Millisecond)

	// panic handler 应该被调用，但不应该影响 normal handler
	assert.Equal(t, int32(1), panicCount.Load())
	assert.Equal(t, int32(1), normalCount.Load())
}

// TestMemoryEventBus_HandlerError 测试 handler 返回错误
func TestMemoryEventBus_HandlerError(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "error-topic"

	var errorCount atomic.Int32
	var successCount atomic.Int32

	// 返回错误的 handler
	errorHandler := func(ctx context.Context, data []byte) error {
		errorCount.Add(1)
		return errors.New("handler error")
	}

	// 成功的 handler
	successHandler := func(ctx context.Context, data []byte) error {
		successCount.Add(1)
		return nil
	}

	// 订阅两个 handler
	err := bus.Subscribe(ctx, topic, errorHandler)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, topic, successHandler)
	require.NoError(t, err)

	// 发布消息
	err = bus.Publish(ctx, topic, []byte("test"))
	require.NoError(t, err)

	// 等待处理
	time.Sleep(200 * time.Millisecond)

	// 两个 handler 都应该被调用
	assert.Equal(t, int32(1), errorCount.Load())
	assert.Equal(t, int32(1), successCount.Load())

	// 检查指标
	metrics := bus.GetMetrics()
	assert.Greater(t, metrics.ConsumeErrors, int64(0))
}

// TestMemoryEventBus_DoubleClose 测试重复关闭
func TestMemoryEventBus_DoubleClose(t *testing.T) {
	bus := NewMemoryEventBus()

	// 第一次关闭
	err := bus.Close()
	require.NoError(t, err)

	// 第二次关闭应该也成功（幂等）
	err = bus.Close()
	assert.NoError(t, err)
}

// TestMemoryEventBus_HealthCheckClosed 测试关闭后的健康检查
func TestMemoryEventBus_HealthCheckClosed(t *testing.T) {
	bus := &memoryEventBus{
		subscribers: make(map[string][]MessageHandler),
		metrics: &Metrics{
			LastHealthCheck:   time.Now(),
			HealthCheckStatus: "healthy",
		},
	}

	ctx := context.Background()

	// 关闭
	err := bus.Close()
	require.NoError(t, err)

	// 健康检查应该失败
	err = bus.HealthCheck(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestMemoryEventBus_RegisterReconnectCallback 测试注册重连回调
func TestMemoryEventBus_RegisterReconnectCallback(t *testing.T) {
	bus := &memoryEventBus{
		subscribers: make(map[string][]MessageHandler),
		metrics:     &Metrics{},
	}

	// 注册回调（对于 memory 实现是 no-op）
	err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
		return nil
	})
	assert.NoError(t, err)
}

// TestMemoryEventBus_ConcurrentOperations 测试并发操作
func TestMemoryEventBus_ConcurrentOperations(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	var wg sync.WaitGroup

	// 并发订阅
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			handler := func(ctx context.Context, data []byte) error {
				return nil
			}
			err := bus.Subscribe(ctx, "concurrent-topic", handler)
			assert.NoError(t, err)
		}(i)
	}

	// 并发发布
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := bus.Publish(ctx, "concurrent-topic", []byte("message"))
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
}

// TestMemoryPublisher_Close 测试发布器关闭
func TestMemoryPublisher_Close(t *testing.T) {
	bus := &memoryEventBus{
		subscribers: make(map[string][]MessageHandler),
		metrics:     &Metrics{},
	}
	publisher := &memoryPublisher{eventBus: bus}

	err := publisher.Close()
	assert.NoError(t, err)
}

// TestMemorySubscriber_Close 测试订阅器关闭
func TestMemorySubscriber_Close(t *testing.T) {
	bus := &memoryEventBus{
		subscribers: make(map[string][]MessageHandler),
		metrics:     &Metrics{},
	}
	subscriber := &memorySubscriber{eventBus: bus}

	err := subscriber.Close()
	assert.NoError(t, err)
}

// TestMemoryEventBus_Integration 测试 Memory EventBus 集成
func TestMemoryEventBus_Integration(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	// Test publish and subscribe
	var receivedCount atomic.Int32
	var receivedData []byte

	handler := func(ctx context.Context, data []byte) error {
		receivedCount.Add(1)
		receivedData = data
		return nil
	}

	// Subscribe
	err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// Publish
	testData := []byte("test message")
	err = bus.Publish(ctx, topic, testData)
	require.NoError(t, err)

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify
	assert.Equal(t, int32(1), receivedCount.Load())
	assert.Equal(t, testData, receivedData)
}

// TestMemoryEventBus_MultipleSubscribers 测试多个订阅者
func TestMemoryEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	var count1, count2, count3 atomic.Int32

	handler1 := func(ctx context.Context, data []byte) error {
		count1.Add(1)
		return nil
	}

	handler2 := func(ctx context.Context, data []byte) error {
		count2.Add(1)
		return nil
	}

	handler3 := func(ctx context.Context, data []byte) error {
		count3.Add(1)
		return nil
	}

	// Subscribe multiple handlers
	err := bus.Subscribe(ctx, topic, handler1)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, topic, handler2)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, topic, handler3)
	require.NoError(t, err)

	// Publish
	err = bus.Publish(ctx, topic, []byte("test"))
	require.NoError(t, err)

	// Wait for messages to be processed
	time.Sleep(100 * time.Millisecond)

	// All handlers should receive the message
	assert.Equal(t, int32(1), count1.Load())
	assert.Equal(t, int32(1), count2.Load())
	assert.Equal(t, int32(1), count3.Load())
}

// TestMemoryEventBus_MultipleTopics 测试多个主题
func TestMemoryEventBus_MultipleTopics(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()

	var topic1Count, topic2Count atomic.Int32

	handler1 := func(ctx context.Context, data []byte) error {
		topic1Count.Add(1)
		return nil
	}

	handler2 := func(ctx context.Context, data []byte) error {
		topic2Count.Add(1)
		return nil
	}

	// Subscribe to different topics
	err := bus.Subscribe(ctx, "topic1", handler1)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, "topic2", handler2)
	require.NoError(t, err)

	// Publish to topic1
	err = bus.Publish(ctx, "topic1", []byte("message1"))
	require.NoError(t, err)

	// Publish to topic2
	err = bus.Publish(ctx, "topic2", []byte("message2"))
	require.NoError(t, err)

	// Wait for messages to be processed
	time.Sleep(100 * time.Millisecond)

	// Each handler should only receive messages from its topic
	assert.Equal(t, int32(1), topic1Count.Load())
	assert.Equal(t, int32(1), topic2Count.Load())
}

// TestMemoryEventBus_ConcurrentPublish 测试并发发布
func TestMemoryEventBus_ConcurrentPublish(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	var receivedCount atomic.Int32

	handler := func(ctx context.Context, data []byte) error {
		receivedCount.Add(1)
		return nil
	}

	// Subscribe
	err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// Publish concurrently
	messageCount := 100
	done := make(chan bool, messageCount)

	for i := 0; i < messageCount; i++ {
		go func(idx int) {
			err := bus.Publish(ctx, topic, []byte("message"))
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all publishes to complete
	for i := 0; i < messageCount; i++ {
		<-done
	}

	// Wait for messages to be processed
	time.Sleep(500 * time.Millisecond)

	// All messages should be received
	assert.Equal(t, int32(messageCount), receivedCount.Load())
}

// TestMemoryEventBus_Close 测试关闭
func TestMemoryEventBus_Close(t *testing.T) {
	bus := NewMemoryEventBus()

	ctx := context.Background()
	topic := "test-topic"

	handler := func(ctx context.Context, data []byte) error {
		return nil
	}

	// Subscribe
	err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// Close
	err = bus.Close()
	require.NoError(t, err)

	// Publishing after close should fail or be ignored
	err = bus.Publish(ctx, topic, []byte("message"))
	// The behavior depends on implementation, but it should not panic
}

// TestEnvelope_Integration 测试 Envelope 集成
func TestEnvelope_Integration(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	var receivedEnvelope *Envelope

	handler := func(ctx context.Context, env *Envelope) error {
		receivedEnvelope = env
		return nil
	}

	// Subscribe with envelope
	err := bus.SubscribeEnvelope(ctx, topic, handler)
	require.NoError(t, err)

	// Create envelope
	testData := []byte("test message")
	envelope := NewEnvelopeWithAutoID("test-aggregate-1", "test.event", 1, testData)

	// Publish with envelope
	err = bus.PublishEnvelope(ctx, topic, envelope)
	require.NoError(t, err)

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify
	assert.NotNil(t, receivedEnvelope)
	assert.Equal(t, "test-aggregate-1", receivedEnvelope.AggregateID)
	assert.Equal(t, "test.event", receivedEnvelope.EventType)
	assert.Equal(t, testData, []byte(receivedEnvelope.Payload))
}

// TestReconnectConfig_Defaults 测试重连配置默认值
func TestReconnectConfig_Defaults(t *testing.T) {
	config := DefaultReconnectConfig()

	assert.Equal(t, DefaultMaxReconnectAttempts, config.MaxAttempts)
	assert.Equal(t, DefaultReconnectInitialBackoff, config.InitialBackoff)
	assert.Equal(t, DefaultReconnectMaxBackoff, config.MaxBackoff)
	assert.Equal(t, DefaultReconnectBackoffFactor, config.BackoffFactor)
	assert.Equal(t, DefaultReconnectFailureThreshold, config.FailureThreshold)
}
