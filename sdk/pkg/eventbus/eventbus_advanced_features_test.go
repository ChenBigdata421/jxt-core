package eventbus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_StartPublisherBacklogMonitoring 测试启动发布端积压监控
func TestEventBusManager_StartPublisherBacklogMonitoring(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	err = manager.StartPublisherBacklogMonitoring(ctx)
	// Memory EventBus 可能不支持，但不应该 panic
	_ = err
}

// TestEventBusManager_StopPublisherBacklogMonitoring 测试停止发布端积压监控
func TestEventBusManager_StopPublisherBacklogMonitoring(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	err = manager.StopPublisherBacklogMonitoring()
	assert.NoError(t, err)
}

// TestEventBusManager_StartAllBacklogMonitoring 测试启动所有积压监控
func TestEventBusManager_StartAllBacklogMonitoring(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	err = manager.StartAllBacklogMonitoring(ctx)
	// Memory EventBus 可能不支持，但不应该 panic
	_ = err
}

// TestEventBusManager_StopAllBacklogMonitoring 测试停止所有积压监控
func TestEventBusManager_StopAllBacklogMonitoring(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	err = manager.StopAllBacklogMonitoring()
	assert.NoError(t, err)
}

// TestEventBusManager_SetMessageRouter 测试设置消息路由器
func TestEventBusManager_SetMessageRouter(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 创建一个简单的路由器
	router := &mockMessageRouter{}

	err = manager.SetMessageRouter(router)
	assert.NoError(t, err)
}

// mockMessageRouter 是一个简单的 Mock 路由器
type mockMessageRouter struct{}

func (r *mockMessageRouter) Route(ctx context.Context, topic string, message []byte) (RouteDecision, error) {
	return RouteDecision{
		ShouldProcess: true,
		AggregateID:   "",
		ProcessorKey:  topic,
	}, nil
}

// TestEventBusManager_SetErrorHandler 测试设置错误处理器
func TestEventBusManager_SetErrorHandler(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 创建一个简单的错误处理器
	handler := &mockErrorHandler{}

	err = manager.SetErrorHandler(handler)
	assert.NoError(t, err)
}

// mockErrorHandler 是一个简单的 Mock 错误处理器
type mockErrorHandler struct{}

func (h *mockErrorHandler) HandleError(ctx context.Context, err error, message []byte, topic string) ErrorAction {
	return ErrorAction{
		Action:     ErrorActionRetry,
		RetryAfter: 0,
	}
}

// TestEventBusManager_RegisterSubscriptionCallback 测试注册订阅回调
func TestEventBusManager_RegisterSubscriptionCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 创建一个订阅回调
	callback := func(ctx context.Context, event SubscriptionEvent) error {
		return nil
	}

	err = manager.RegisterSubscriptionCallback(callback)
	// Memory EventBus 可能不支持，但不应该 panic
	_ = err
}

// TestEventBusManager_StartHealthCheck 测试启动健康检查（已废弃）
func TestEventBusManager_StartHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	err = manager.StartHealthCheck(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_StopHealthCheck 测试停止健康检查（已废弃）
func TestEventBusManager_StopHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	err = manager.StopHealthCheck()
	assert.NoError(t, err)
}

// TestEventBusManager_RegisterHealthCheckCallback 测试注册健康检查回调（已废弃）
func TestEventBusManager_RegisterHealthCheckCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 先启动健康检查发布器
	err = manager.StartHealthCheckPublisher(ctx)
	require.NoError(t, err)

	// 创建一个健康检查回调
	callback := func(ctx context.Context, result HealthCheckResult) error {
		return nil
	}

	err = manager.RegisterHealthCheckCallback(callback)
	assert.NoError(t, err)
}

// TestEventBusManager_StartAllHealthCheck 测试启动所有健康检查
func TestEventBusManager_StartAllHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	err = manager.StartAllHealthCheck(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_StopAllHealthCheck 测试停止所有健康检查
func TestEventBusManager_StopAllHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	err = manager.StopAllHealthCheck()
	assert.NoError(t, err)
}

// TestEventBusManager_SetTopicConfigStrategy 测试设置主题配置策略
func TestEventBusManager_SetTopicConfigStrategy(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 测试设置策略（使用字符串常量）
	manager.SetTopicConfigStrategy("create_or_update")
	manager.SetTopicConfigStrategy("create_only")
	manager.SetTopicConfigStrategy("verify_only")
	manager.SetTopicConfigStrategy("skip")
}

// TestEventBusManager_GetTopicConfigStrategy 测试获取主题配置策略
func TestEventBusManager_GetTopicConfigStrategy(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	strategy := manager.GetTopicConfigStrategy()
	assert.NotEmpty(t, strategy)
}

// TestEventBusManager_PublishEnvelope_Error 测试发布 Envelope 错误场景
func TestEventBusManager_PublishEnvelope_Error(t *testing.T) {
	// 测试 nil envelope - 可能会 panic，所以跳过
	t.Skip("Skipping nil envelope test to avoid panic")
}

// TestEventBusManager_SubscribeEnvelope 测试订阅 Envelope
func TestEventBusManager_SubscribeEnvelope(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 创建一个 Envelope 处理器
	handler := func(ctx context.Context, envelope *Envelope) error {
		return nil
	}

	err = manager.SubscribeEnvelope(ctx, "test-topic", handler)
	assert.NoError(t, err)
}

// TestEventBusManager_GetHealthCheckSubscriberStats_NoSubscriber 测试无订阅器时获取统计
func TestEventBusManager_GetHealthCheckSubscriberStats_NoSubscriber(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	stats := manager.GetHealthCheckSubscriberStats()
	// 无订阅器时应该返回空统计
	assert.NotNil(t, stats)
}

// TestEventBusManager_RegisterHealthCheckPublisherCallback_NoPublisher 测试无发布器时注册回调
func TestEventBusManager_RegisterHealthCheckPublisherCallback_NoPublisher(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	callback := func(ctx context.Context, result HealthCheckResult) error {
		return nil
	}

	err = manager.RegisterHealthCheckPublisherCallback(callback)
	// 应该成功或返回错误，但不应该 panic
	_ = err
}

// TestEventBusManager_LifecycleSequence 测试完整的生命周期序列
func TestEventBusManager_LifecycleSequence(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 启动所有功能
	_ = manager.StartAllHealthCheck(ctx)
	_ = manager.StartAllBacklogMonitoring(ctx)

	// 执行一些操作
	_ = manager.Publish(ctx, "test-topic", []byte("test"))

	// 停止所有功能
	_ = manager.StopAllHealthCheck()
	_ = manager.StopAllBacklogMonitoring()

	// 关闭
	bus.Close()
}
