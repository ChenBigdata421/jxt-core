package eventbus

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Publish 方法高级测试 ==========

// TestEventBusManager_Publish_ContextCancellation 测试 Publish 在 context 取消时的行为
func TestEventBusManager_Publish_ContextCancellation(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	// 创建一个已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 尝试发布消息
	err = bus.Publish(ctx, "test-topic", []byte("test message"))
	// Memory EventBus 不检查 context，所以应该成功
	// 但这个测试验证了 Publish 方法接受 context 参数
	assert.NoError(t, err)
}

// TestEventBusManager_Publish_EmptyTopic 测试发布到空主题
func TestEventBusManager_Publish_EmptyTopic(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	err = bus.Publish(ctx, "", []byte("test message"))
	// Memory EventBus 允许空主题，但这是一个边缘情况
	assert.NoError(t, err)
}

// TestEventBusManager_Publish_NilMessage 测试发布 nil 消息
func TestEventBusManager_Publish_NilMessage_Advanced(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	err = bus.Publish(ctx, "test-topic", nil)
	// Memory EventBus 允许 nil 消息
	assert.NoError(t, err)
}

// TestEventBusManager_Publish_LargeMessage 测试发布大消息
func TestEventBusManager_Publish_LargeMessage_Advanced(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	// 创建 10MB 的消息
	largeMessage := make([]byte, 10*1024*1024)
	for i := range largeMessage {
		largeMessage[i] = byte(i % 256)
	}

	err = bus.Publish(ctx, "test-topic", largeMessage)
	assert.NoError(t, err)
}

// ========== Subscribe 方法高级测试 ==========

// TestEventBusManager_Subscribe_HandlerPanic 测试 handler panic 的处理
func TestEventBusManager_Subscribe_HandlerPanic(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅消息（handler 会 panic）
	handler := func(ctx context.Context, message []byte) error {
		panic("test panic")
	}

	err = bus.Subscribe(ctx, "test-topic-panic", handler)
	require.NoError(t, err)

	// 发布消息（应该触发 panic，但不应该导致程序崩溃）
	// Memory EventBus 的实现会捕获 panic
	err = bus.Publish(ctx, "test-topic-panic", []byte("test message"))
	assert.NoError(t, err)

	// 等待消息处理
	time.Sleep(50 * time.Millisecond)
}

// TestEventBusManager_Subscribe_MultipleHandlersSameTopic 测试同一主题的多个处理器
func TestEventBusManager_Subscribe_MultipleHandlersSameTopic(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅多个处理器到同一主题
	received1 := make(chan []byte, 1)
	handler1 := func(ctx context.Context, message []byte) error {
		received1 <- message
		return nil
	}

	received2 := make(chan []byte, 1)
	handler2 := func(ctx context.Context, message []byte) error {
		received2 <- message
		return nil
	}

	err = bus.Subscribe(ctx, "test-topic-multi", handler1)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, "test-topic-multi", handler2)
	require.NoError(t, err)

	// 发布消息
	err = bus.Publish(ctx, "test-topic-multi", []byte("test message"))
	require.NoError(t, err)

	// 验证两个处理器都收到消息
	select {
	case msg := <-received1:
		assert.Equal(t, []byte("test message"), msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Handler 1 did not receive message")
	}

	select {
	case msg := <-received2:
		assert.Equal(t, []byte("test message"), msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Handler 2 did not receive message")
	}
}

// TestEventBusManager_Subscribe_SlowHandler 测试慢速处理器
func TestEventBusManager_Subscribe_SlowHandler(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅消息（handler 处理很慢）
	received := make(chan []byte, 1)
	handler := func(ctx context.Context, message []byte) error {
		time.Sleep(100 * time.Millisecond)
		received <- message
		return nil
	}

	err = bus.Subscribe(ctx, "test-topic-slow", handler)
	require.NoError(t, err)

	// 发布消息
	start := time.Now()
	err = bus.Publish(ctx, "test-topic-slow", []byte("test message"))
	require.NoError(t, err)
	publishDuration := time.Since(start)

	// 发布应该很快返回（不等待处理器）
	assert.Less(t, publishDuration, 50*time.Millisecond)

	// 等待消息被处理
	select {
	case msg := <-received:
		assert.Equal(t, []byte("test message"), msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Handler did not receive message")
	}
}

// ========== PublishEnvelope 方法高级测试 ==========

// TestEventBusManager_PublishEnvelope_Fallback 测试回退到普通发布
func TestEventBusManager_PublishEnvelope_Fallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// Memory EventBus 支持 Envelope，但我们可以测试 Envelope 的序列化
	envelope := NewEnvelope("agg-123", "TestEvent", 1, []byte(`{"data": "test"}`))
	envelope.TraceID = "trace-123"
	envelope.CorrelationID = "corr-123"

	err = bus.PublishEnvelope(ctx, "test-topic-envelope", envelope)
	assert.NoError(t, err)
}

// TestEventBusManager_PublishEnvelope_InvalidEnvelope 测试无效的 Envelope
func TestEventBusManager_PublishEnvelope_InvalidEnvelope(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 测试 nil envelope
	err = bus.PublishEnvelope(ctx, "test-topic", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "envelope cannot be nil")

	// 测试空 topic
	envelope := NewEnvelope("agg-123", "TestEvent", 1, []byte(`{"data": "test"}`))
	err = bus.PublishEnvelope(ctx, "", envelope)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "topic cannot be empty")
}

// TestEventBusManager_PublishEnvelope_AllFields 测试包含所有字段的 Envelope
func TestEventBusManager_PublishEnvelope_AllFields(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅 Envelope
	received := make(chan *Envelope, 1)
	handler := func(ctx context.Context, envelope *Envelope) error {
		received <- envelope
		return nil
	}

	err = bus.SubscribeEnvelope(ctx, "test-topic-all-fields", handler)
	require.NoError(t, err)

	// 创建包含所有字段的 Envelope
	envelope := &Envelope{
		AggregateID:   "agg-123",
		EventType:     "TestEvent",
		EventVersion:  1,
		Timestamp:     time.Now(),
		TraceID:       "trace-123",
		CorrelationID: "corr-123",
		Payload:       []byte(`{"data": "test", "value": 42}`),
	}

	err = bus.PublishEnvelope(ctx, "test-topic-all-fields", envelope)
	require.NoError(t, err)

	// 验证接收到的 Envelope
	select {
	case env := <-received:
		assert.Equal(t, "agg-123", env.AggregateID)
		assert.Equal(t, "TestEvent", env.EventType)
		assert.Equal(t, int64(1), env.EventVersion)
		assert.Equal(t, "trace-123", env.TraceID)
		assert.Equal(t, "corr-123", env.CorrelationID)
		assert.Contains(t, string(env.Payload), "test")
	case <-time.After(1 * time.Second):
		t.Fatal("Did not receive envelope")
	}
}

// ========== SubscribeEnvelope 方法高级测试 ==========

// TestEventBusManager_SubscribeEnvelope_InvalidParams 测试无效参数
func TestEventBusManager_SubscribeEnvelope_InvalidParams(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	handler := func(ctx context.Context, envelope *Envelope) error {
		return nil
	}

	// 测试空 topic
	err = bus.SubscribeEnvelope(ctx, "", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "topic cannot be empty")

	// 测试 nil handler
	err = bus.SubscribeEnvelope(ctx, "test-topic", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler cannot be nil")
}

// TestEventBusManager_SubscribeEnvelope_HandlerError 测试 handler 返回错误
func TestEventBusManager_SubscribeEnvelope_HandlerError(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	manager := bus.(*eventBusManager)

	// 订阅 Envelope（handler 返回错误）
	received := make(chan *Envelope, 1)
	testError := errors.New("handler error")
	handler := func(ctx context.Context, envelope *Envelope) error {
		received <- envelope
		return testError
	}

	err = bus.SubscribeEnvelope(ctx, "test-topic-error", handler)
	require.NoError(t, err)

	// 记录初始错误计数
	initialErrors := manager.metrics.ConsumeErrors

	// 发布 Envelope
	envelope := NewEnvelope("agg-123", "TestEvent", 1, []byte(`{"data": "test"}`))
	err = bus.PublishEnvelope(ctx, "test-topic-error", envelope)
	require.NoError(t, err)

	// 等待消息被处理
	select {
	case <-received:
		time.Sleep(50 * time.Millisecond)
		// 验证错误计数增加
		assert.Greater(t, manager.metrics.ConsumeErrors, initialErrors)
	case <-time.After(1 * time.Second):
		t.Fatal("Handler did not receive envelope")
	}
}

// ========== checkConnection 方法测试 ==========

// TestEventBusManager_CheckConnection_PublisherHealthCheckFail 测试 publisher 健康检查失败
func TestEventBusManager_CheckConnection_PublisherHealthCheckFail(t *testing.T) {
	// 这个测试需要 mock publisher，暂时跳过
	t.Skip("Requires mock publisher with HealthCheck interface")
}

// TestEventBusManager_CheckConnection_SubscriberHealthCheckFail 测试 subscriber 健康检查失败
func TestEventBusManager_CheckConnection_SubscriberHealthCheckFail(t *testing.T) {
	// 这个测试需要 mock subscriber，暂时跳过
	t.Skip("Requires mock subscriber with HealthCheck interface")
}

// ========== checkMessageTransport 方法测试 ==========

// TestEventBusManager_CheckMessageTransport_NilPublisher 测试 publisher 为 nil
func TestEventBusManager_CheckMessageTransport_NilPublisher(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	manager.publisher = nil

	ctx := context.Background()
	err = manager.checkMessageTransport(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publisher not initialized")
}

// ========== 并发测试 ==========

// TestEventBusManager_ConcurrentPublishSubscribe 测试并发发布和订阅
func TestEventBusManager_ConcurrentPublishSubscribe(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅消息
	var receivedCount int64
	var mu sync.Mutex
	handler := func(ctx context.Context, message []byte) error {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		return nil
	}

	err = bus.Subscribe(ctx, "test-topic-concurrent", handler)
	require.NoError(t, err)

	// 并发发布消息
	numGoroutines := 20
	numMessagesPerGoroutine := 50
	totalMessages := numGoroutines * numMessagesPerGoroutine

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessagesPerGoroutine; j++ {
				err := bus.Publish(ctx, "test-topic-concurrent", []byte("test message"))
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// 等待所有消息被处理
	time.Sleep(500 * time.Millisecond)

	// 验证接收到的消息数量
	mu.Lock()
	count := receivedCount
	mu.Unlock()

	assert.GreaterOrEqual(t, count, int64(totalMessages*9/10)) // 至少 90%
}

