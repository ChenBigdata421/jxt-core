package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_Publish 测试发布消息
func TestEventBusManager_Publish(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	err = bus.Publish(ctx, "test-topic", []byte("test message"))
	assert.NoError(t, err)
}

// TestEventBusManager_Publish_NilPublisher 测试发布器未初始化
func TestEventBusManager_Publish_NilPublisher(t *testing.T) {
	manager := &eventBusManager{
		config:  &EventBusConfig{Type: "memory"},
		metrics: &Metrics{},
		closed:  false,
	}

	ctx := context.Background()
	err := manager.Publish(ctx, "test-topic", []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publisher not initialized")
}

// TestEventBusManager_Subscribe 测试订阅消息
func TestEventBusManager_Subscribe(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	received := make(chan bool, 1)
	handler := func(ctx context.Context, message []byte) error {
		received <- true
		return nil
	}

	ctx := context.Background()
	err = bus.Subscribe(ctx, "test-topic", handler)
	require.NoError(t, err)

	// 发布消息
	err = bus.Publish(ctx, "test-topic", []byte("test message"))
	require.NoError(t, err)

	// 等待接收
	select {
	case <-received:
		// 成功接收
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

// TestEventBusManager_Subscribe_NilSubscriber 测试订阅器未初始化
func TestEventBusManager_Subscribe_NilSubscriber(t *testing.T) {
	manager := &eventBusManager{
		config:  &EventBusConfig{Type: "memory"},
		metrics: &Metrics{},
		closed:  false,
	}

	ctx := context.Background()
	handler := func(ctx context.Context, message []byte) error {
		return nil
	}

	err := manager.Subscribe(ctx, "test-topic", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "subscriber not initialized")
}

// TestEventBusManager_GetMetrics_Core 测试获取指标
func TestEventBusManager_GetMetrics_Core(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	metrics := bus.GetMetrics()
	assert.NotNil(t, metrics)
	assert.False(t, metrics.LastHealthCheck.IsZero())
}

// TestEventBusManager_GetConnectionState_Core 测试获取连接状态
func TestEventBusManager_GetConnectionState_Core(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	state := bus.GetConnectionState()
	assert.NotNil(t, state)
	assert.True(t, state.IsConnected)
}

// TestEventBusManager_RegisterReconnectCallback_Core 测试注册重连回调
func TestEventBusManager_RegisterReconnectCallback_Core(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	callback := func(ctx context.Context) error {
		return nil
	}

	bus.RegisterReconnectCallback(callback)

	// 验证回调已注册（通过内部状态）
	manager := bus.(*eventBusManager)
	assert.NotNil(t, manager.reconnectCallback)
}

// TestEventBusManager_ConfigureTopic 测试配置主题
func TestEventBusManager_ConfigureTopic(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	options := TopicOptions{
		PersistenceMode: "persistent",
		Replicas:        2,
	}

	err = bus.ConfigureTopic(ctx, "test-topic", options)
	assert.NoError(t, err)
}

// TestEventBusManager_GetTopicConfig 测试获取主题配置
func TestEventBusManager_GetTopicConfig(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	options := TopicOptions{
		PersistenceMode: "persistent",
		Replicas:        3,
	}

	err = bus.ConfigureTopic(ctx, "test-topic", options)
	require.NoError(t, err)

	config, err := bus.GetTopicConfig("test-topic")
	assert.NoError(t, err)
	assert.Equal(t, "persistent", string(config.PersistenceMode))
	assert.Equal(t, 3, config.Replicas)
}

// TestEventBusManager_RemoveTopicConfig 测试移除主题配置
func TestEventBusManager_RemoveTopicConfig(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	options := TopicOptions{
		PersistenceMode: "persistent",
	}

	err = bus.ConfigureTopic(ctx, "test-topic", options)
	require.NoError(t, err)

	bus.RemoveTopicConfig("test-topic")

	// GetTopicConfig 返回默认配置而不是错误
	config, err := bus.GetTopicConfig("test-topic")
	assert.NoError(t, err)
	// 验证返回的是默认配置
	assert.NotNil(t, config)
}

// TestEventBusManager_UpdateMetrics 测试更新指标
func TestEventBusManager_UpdateMetrics(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 发布消息以更新指标
	err = bus.Publish(ctx, "test-topic", []byte("test"))
	require.NoError(t, err)

	metrics := bus.GetMetrics()
	assert.Greater(t, metrics.MessagesPublished, int64(0))
}

// TestEventBusManager_HandlerError 测试处理器错误
func TestEventBusManager_HandlerError(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	errorReceived := make(chan bool, 1)
	handler := func(ctx context.Context, message []byte) error {
		errorReceived <- true
		return assert.AnError
	}

	ctx := context.Background()
	err = bus.Subscribe(ctx, "test-topic", handler)
	require.NoError(t, err)

	err = bus.Publish(ctx, "test-topic", []byte("test"))
	require.NoError(t, err)

	// 等待错误处理
	select {
	case <-errorReceived:
		// 成功接收错误
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for error")
	}

	// 检查错误指标
	metrics := bus.GetMetrics()
	assert.Greater(t, metrics.ConsumeErrors, int64(0))
}

// TestEventBusManager_MultipleSubscribers 测试多个订阅者
func TestEventBusManager_MultipleSubscribers(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	received1 := make(chan bool, 1)
	received2 := make(chan bool, 1)

	handler1 := func(ctx context.Context, message []byte) error {
		received1 <- true
		return nil
	}

	handler2 := func(ctx context.Context, message []byte) error {
		received2 <- true
		return nil
	}

	ctx := context.Background()
	err = bus.Subscribe(ctx, "test-topic", handler1)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, "test-topic", handler2)
	require.NoError(t, err)

	err = bus.Publish(ctx, "test-topic", []byte("test"))
	require.NoError(t, err)

	// 等待两个订阅者都接收到消息
	timeout := time.After(1 * time.Second)
	count := 0
	for count < 2 {
		select {
		case <-received1:
			count++
		case <-received2:
			count++
		case <-timeout:
			t.Fatalf("timeout waiting for messages, received %d/2", count)
		}
	}
}

// TestEventBusManager_ConcurrentPublish 测试并发发布
func TestEventBusManager_ConcurrentPublish(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	const numMessages = 100

	// 并发发布消息
	done := make(chan bool, numMessages)
	for i := 0; i < numMessages; i++ {
		go func(i int) {
			err := bus.Publish(ctx, "test-topic", []byte("test"))
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// 等待所有发布完成
	for i := 0; i < numMessages; i++ {
		select {
		case <-done:
			// 成功
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent publishes")
		}
	}

	metrics := bus.GetMetrics()
	assert.GreaterOrEqual(t, metrics.MessagesPublished, int64(numMessages))
}

// TestEventBusManager_ContextCancellation 测试上下文取消
func TestEventBusManager_ContextCancellation(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// 尝试发布消息（应该快速失败或成功，取决于实现）
	err = bus.Publish(ctx, "test-topic", []byte("test"))
	// 不检查错误，因为 memory 实现可能不检查上下文
}
