package eventbus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== wrappedHandler 测试 ==========

// TestEventBusManager_WrappedHandler_Success 测试 wrappedHandler 成功处理消息
func TestEventBusManager_WrappedHandler_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息
	var handlerCalled bool
	var receivedMessage []byte
	handler := func(ctx context.Context, message []byte) error {
		handlerCalled = true
		receivedMessage = message
		return nil
	}

	err = bus.Subscribe(ctx, "test-wrapped-success", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed
	initialErrors := manager.metrics.ConsumeErrors

	// 发布消息
	testMessage := []byte("test message for wrapped handler")
	err = bus.Publish(ctx, "test-wrapped-success", testMessage)
	require.NoError(t, err)

	// 等待消息被处理
	time.Sleep(100 * time.Millisecond)

	// 验证 handler 被调用
	assert.True(t, handlerCalled)
	assert.Equal(t, testMessage, receivedMessage)

	// 验证指标更新（成功）
	assert.Greater(t, manager.metrics.MessagesConsumed, initialConsumed)
	assert.Equal(t, initialErrors, manager.metrics.ConsumeErrors)
}

// TestEventBusManager_WrappedHandler_Error 测试 wrappedHandler 处理错误
func TestEventBusManager_WrappedHandler_Error(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息（handler 返回错误）
	var handlerCalled bool
	testError := errors.New("test handler error")
	handler := func(ctx context.Context, message []byte) error {
		handlerCalled = true
		return testError
	}

	err = bus.Subscribe(ctx, "test-wrapped-error", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed
	initialErrors := manager.metrics.ConsumeErrors

	// 发布消息
	err = bus.Publish(ctx, "test-wrapped-error", []byte("test message"))
	require.NoError(t, err)

	// 等待消息被处理
	time.Sleep(100 * time.Millisecond)

	// 验证 handler 被调用
	assert.True(t, handlerCalled)

	// 验证指标更新（错误）
	assert.Equal(t, initialConsumed, manager.metrics.MessagesConsumed)
	assert.Greater(t, manager.metrics.ConsumeErrors, initialErrors)
}

// TestEventBusManager_WrappedHandler_MultipleMessages 测试 wrappedHandler 处理多条消息
func TestEventBusManager_WrappedHandler_MultipleMessages(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息
	var messageCount int32
	handler := func(ctx context.Context, message []byte) error {
		atomic.AddInt32(&messageCount, 1)
		return nil
	}

	err = bus.Subscribe(ctx, "test-wrapped-multi", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed

	// 发布多条消息
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		err = bus.Publish(ctx, "test-wrapped-multi", []byte("test message"))
		require.NoError(t, err)
	}

	// 等待所有消息被处理
	time.Sleep(200 * time.Millisecond)

	// 验证所有消息都被处理
	assert.Equal(t, int32(numMessages), atomic.LoadInt32(&messageCount))

	// 验证指标更新
	consumedDelta := manager.metrics.MessagesConsumed - initialConsumed
	assert.GreaterOrEqual(t, consumedDelta, int64(numMessages*9/10)) // 至少 90%
}

// TestEventBusManager_WrappedHandler_ContextCancellation 测试 wrappedHandler 处理 context 取消
func TestEventBusManager_WrappedHandler_ContextCancellation(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅消息（handler 检查 context）
	var handlerCalled bool
	handler := func(ctx context.Context, message []byte) error {
		handlerCalled = true
		// 检查 context 是否被取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	err = bus.Subscribe(ctx, "test-wrapped-cancel", handler)
	require.NoError(t, err)

	// 发布消息
	err = bus.Publish(ctx, "test-wrapped-cancel", []byte("test message"))
	require.NoError(t, err)

	// 等待消息被处理
	time.Sleep(100 * time.Millisecond)

	// 验证 handler 被调用
	assert.True(t, handlerCalled)
}

// TestEventBusManager_WrappedHandler_SlowProcessing 测试 wrappedHandler 慢速处理
func TestEventBusManager_WrappedHandler_SlowProcessing(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息（handler 处理很慢）
	var processingTime time.Duration
	handler := func(ctx context.Context, message []byte) error {
		start := time.Now()
		time.Sleep(100 * time.Millisecond)
		processingTime = time.Since(start)
		return nil
	}

	err = bus.Subscribe(ctx, "test-wrapped-slow", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed

	// 发布消息
	err = bus.Publish(ctx, "test-wrapped-slow", []byte("test message"))
	require.NoError(t, err)

	// 等待消息被处理
	time.Sleep(300 * time.Millisecond)

	// 验证处理时间
	assert.GreaterOrEqual(t, processingTime, 100*time.Millisecond)

	// 验证指标更新
	assert.Greater(t, manager.metrics.MessagesConsumed, initialConsumed)
}

// TestEventBusManager_WrappedHandler_ConcurrentExecution 测试 wrappedHandler 并发执行
func TestEventBusManager_WrappedHandler_ConcurrentExecution(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息
	var concurrentCount int32
	var maxConcurrent int32
	var mu sync.Mutex
	handler := func(ctx context.Context, message []byte) error {
		current := atomic.AddInt32(&concurrentCount, 1)

		mu.Lock()
		if current > maxConcurrent {
			maxConcurrent = current
		}
		mu.Unlock()

		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&concurrentCount, -1)
		return nil
	}

	err = bus.Subscribe(ctx, "test-wrapped-concurrent", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed

	// 快速发布多条消息
	numMessages := 20
	for i := 0; i < numMessages; i++ {
		err = bus.Publish(ctx, "test-wrapped-concurrent", []byte("test message"))
		require.NoError(t, err)
	}

	// 等待所有消息被处理
	time.Sleep(500 * time.Millisecond)

	// 验证有并发执行
	mu.Lock()
	max := maxConcurrent
	mu.Unlock()
	assert.Greater(t, max, int32(1), "Should have concurrent execution")

	// 验证指标更新
	consumedDelta := manager.metrics.MessagesConsumed - initialConsumed
	assert.GreaterOrEqual(t, consumedDelta, int64(numMessages*9/10))
}

// TestEventBusManager_WrappedHandler_MessageSize 测试 wrappedHandler 处理不同大小的消息
func TestEventBusManager_WrappedHandler_MessageSize(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息
	var receivedSizes []int
	var mu sync.Mutex
	handler := func(ctx context.Context, message []byte) error {
		mu.Lock()
		receivedSizes = append(receivedSizes, len(message))
		mu.Unlock()
		return nil
	}

	err = bus.Subscribe(ctx, "test-wrapped-size", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed

	// 发布不同大小的消息
	sizes := []int{0, 1, 100, 1024, 10240, 102400}
	for _, size := range sizes {
		message := make([]byte, size)
		err = bus.Publish(ctx, "test-wrapped-size", message)
		require.NoError(t, err)
	}

	// 等待所有消息被处理
	time.Sleep(200 * time.Millisecond)

	// 验证接收到的消息数量和大小（不验证顺序，因为是并发处理）
	mu.Lock()
	received := receivedSizes
	mu.Unlock()

	assert.Equal(t, len(sizes), len(received))

	// 验证所有大小都被接收到（不关心顺序）
	receivedMap := make(map[int]bool)
	for _, size := range received {
		receivedMap[size] = true
	}

	for _, expectedSize := range sizes {
		assert.True(t, receivedMap[expectedSize], "Expected size %d not received", expectedSize)
	}

	// 验证指标更新
	consumedDelta := manager.metrics.MessagesConsumed - initialConsumed
	assert.GreaterOrEqual(t, consumedDelta, int64(len(sizes)*9/10))
}

// TestEventBusManager_WrappedHandler_ErrorRecovery 测试 wrappedHandler 错误恢复
func TestEventBusManager_WrappedHandler_ErrorRecovery(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息（前几条失败，后面成功）
	var messageCount int32
	handler := func(ctx context.Context, message []byte) error {
		count := atomic.AddInt32(&messageCount, 1)
		if count <= 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	err = bus.Subscribe(ctx, "test-wrapped-recovery", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed
	initialErrors := manager.metrics.ConsumeErrors

	// 发布多条消息
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		err = bus.Publish(ctx, "test-wrapped-recovery", []byte("test message"))
		require.NoError(t, err)
	}

	// 等待所有消息被处理
	time.Sleep(200 * time.Millisecond)

	// 验证有成功和失败
	consumedDelta := manager.metrics.MessagesConsumed - initialConsumed
	errorsDelta := manager.metrics.ConsumeErrors - initialErrors

	assert.Greater(t, consumedDelta, int64(0), "Should have successful messages")
	assert.Greater(t, errorsDelta, int64(0), "Should have failed messages")
	assert.GreaterOrEqual(t, consumedDelta+errorsDelta, int64(numMessages*9/10))
}

// TestEventBusManager_WrappedHandler_NilMessage 测试 wrappedHandler 处理 nil 消息
func TestEventBusManager_WrappedHandler_NilMessage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息
	var receivedNil bool
	handler := func(ctx context.Context, message []byte) error {
		if message == nil {
			receivedNil = true
		}
		return nil
	}

	err = bus.Subscribe(ctx, "test-wrapped-nil", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed

	// 发布 nil 消息
	err = bus.Publish(ctx, "test-wrapped-nil", nil)
	require.NoError(t, err)

	// 等待消息被处理
	time.Sleep(100 * time.Millisecond)

	// 验证接收到 nil 消息
	assert.True(t, receivedNil)

	// 验证指标更新
	assert.Greater(t, manager.metrics.MessagesConsumed, initialConsumed)
}

// TestEventBusManager_WrappedHandler_EmptyMessage 测试 wrappedHandler 处理空消息
func TestEventBusManager_WrappedHandler_EmptyMessage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 订阅消息
	var receivedEmpty bool
	handler := func(ctx context.Context, message []byte) error {
		if len(message) == 0 {
			receivedEmpty = true
		}
		return nil
	}

	err = bus.Subscribe(ctx, "test-wrapped-empty", handler)
	require.NoError(t, err)

	// 记录初始指标
	initialConsumed := manager.metrics.MessagesConsumed

	// 发布空消息
	err = bus.Publish(ctx, "test-wrapped-empty", []byte{})
	require.NoError(t, err)

	// 等待消息被处理
	time.Sleep(100 * time.Millisecond)

	// 验证接收到空消息
	assert.True(t, receivedEmpty)

	// 验证指标更新
	assert.Greater(t, manager.metrics.MessagesConsumed, initialConsumed)
}
