package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEventBus_NilConfig 测试 nil 配置
func TestNewEventBus_NilConfig(t *testing.T) {
	bus, err := NewEventBus(nil)
	assert.Error(t, err)
	assert.Nil(t, bus)
	assert.Contains(t, err.Error(), "config cannot be nil")
}

// TestNewEventBus_UnsupportedType 测试不支持的类型
func TestNewEventBus_UnsupportedType(t *testing.T) {
	config := &EventBusConfig{
		Type: "unsupported",
	}

	bus, err := NewEventBus(config)
	assert.Error(t, err)
	assert.Nil(t, bus)
	assert.Contains(t, err.Error(), "unsupported eventbus type")
}

// TestEventBusManager_PublishClosed 测试关闭后发布
func TestEventBusManager_PublishClosed(t *testing.T) {
	bus := NewMemoryEventBus()
	ctx := context.Background()

	// 关闭
	err := bus.Close()
	require.NoError(t, err)

	// 尝试发布
	err = bus.Publish(ctx, "test-topic", []byte("message"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestEventBusManager_SubscribeClosed 测试关闭后订阅
func TestEventBusManager_SubscribeClosed(t *testing.T) {
	bus := NewMemoryEventBus()
	ctx := context.Background()

	// 关闭
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

// TestEventBusManager_RegisterReconnectCallback 测试注册重连回调
func TestEventBusManager_RegisterReconnectCallback(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	callbackCalled := false
	callback := func(ctx context.Context) error {
		callbackCalled = true
		return nil
	}

	err := bus.RegisterReconnectCallback(callback)
	assert.NoError(t, err)

	// 对于 memory 实现，回调不会被自动调用
	// 但应该能成功注册
	assert.False(t, callbackCalled)
}

// TestEventBusManager_GetMetrics 测试获取指标
func TestEventBusManager_GetMetrics(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "metrics-topic"

	// 订阅
	handler := func(ctx context.Context, data []byte) error {
		return nil
	}
	err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 发布消息
	err = bus.Publish(ctx, topic, []byte("test"))
	require.NoError(t, err)

	// 等待处理
	time.Sleep(100 * time.Millisecond)

	// 获取指标
	metrics := bus.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Greater(t, metrics.MessagesPublished, int64(0))
}

// TestEventBusManager_GetConnectionState 测试获取连接状态
func TestEventBusManager_GetConnectionState(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	state := bus.GetConnectionState()
	assert.NotNil(t, state)
	assert.True(t, state.IsConnected)
}

// TestEventBusManager_StartStopHealthCheckPublisher 测试启动停止健康检查发布器
func TestEventBusManager_StartStopHealthCheckPublisher(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()

	// 启动健康检查发布器
	err := bus.StartHealthCheckPublisher(ctx)
	assert.NoError(t, err)

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 停止健康检查发布器
	err = bus.StopHealthCheckPublisher()
	assert.NoError(t, err)
}

// TestEventBusManager_StartHealthCheckPublisherTwice 测试重复启动健康检查发布器
func TestEventBusManager_StartHealthCheckPublisherTwice(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()

	// 第一次启动
	err := bus.StartHealthCheckPublisher(ctx)
	assert.NoError(t, err)

	// 第二次启动应该返回 nil（已经启动）
	err = bus.StartHealthCheckPublisher(ctx)
	assert.NoError(t, err)

	// 清理
	bus.StopHealthCheckPublisher()
}

// TestEventBusManager_StartStopHealthCheckSubscriber 测试启动停止健康检查订阅器
func TestEventBusManager_StartStopHealthCheckSubscriber(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()

	// 启动健康检查订阅器
	err := bus.StartHealthCheckSubscriber(ctx)
	assert.NoError(t, err)

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 停止健康检查订阅器
	err = bus.StopHealthCheckSubscriber()
	assert.NoError(t, err)
}

// TestEventBusManager_RegisterHealthCheckSubscriberCallback 测试注册健康检查订阅器回调
func TestEventBusManager_RegisterHealthCheckSubscriberCallback(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	callbackCalled := false
	callback := func(ctx context.Context, alert HealthCheckAlert) error {
		callbackCalled = true
		return nil
	}

	err := bus.RegisterHealthCheckSubscriberCallback(callback)
	assert.NoError(t, err)

	// 回调应该被注册，但不会立即调用
	assert.False(t, callbackCalled)
}
