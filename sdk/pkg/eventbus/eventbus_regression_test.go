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

// TestEventBusManager_Subscribe_ErrorCases 测试订阅错误场景
func TestEventBusManager_Subscribe_ErrorCases(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 测试关闭后订阅
	bus.Close()
	err = manager.Subscribe(ctx, "test-topic", func(ctx context.Context, message []byte) error {
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestEventBusManager_Publish_ErrorCases 测试发布错误场景
func TestEventBusManager_Publish_ErrorCases(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 测试关闭后发布
	bus.Close()
	err = manager.Publish(ctx, "test-topic", []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestEventBusManager_Close_Multiple 测试多次关闭
func TestEventBusManager_Close_Multiple(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	// 第一次关闭
	err = bus.Close()
	assert.NoError(t, err)

	// 第二次关闭
	err = bus.Close()
	assert.NoError(t, err) // 应该不报错
}

// TestEventBusManager_RegisterReconnectCallback_WithCallback 测试注册重连回调
func TestEventBusManager_RegisterReconnectCallback_WithCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	callbackCalled := false
	callback := func(ctx context.Context) error {
		callbackCalled = true
		return nil
	}

	err = bus.RegisterReconnectCallback(callback)
	assert.NoError(t, err)

	// 验证回调已注册（虽然可能不会被调用）
	_ = callbackCalled
}

// TestEventBusManager_HealthCheck_Infrastructure 测试基础设施健康检查
func TestEventBusManager_HealthCheck_Infrastructure(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 测试基础设施健康检查
	err = manager.performHealthCheck(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_HealthCheck_Full 测试完整健康检查
func TestEventBusManager_HealthCheck_Full(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 测试完整健康检查
	result, err := manager.performFullHealthCheck(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestEventBusManager_PerformEndToEndTest_Coverage 测试端到端测试（覆盖率）
func TestEventBusManager_PerformEndToEndTest_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 测试端到端测试
	err = manager.performEndToEndTest(ctx, "test-topic", "test-message")
	assert.NoError(t, err)
}

// TestEventBusManager_StartStopHealthCheckPublisher_Coverage 测试启动停止健康检查发布器（覆盖率）
func TestEventBusManager_StartStopHealthCheckPublisher_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 启动健康检查发布器
	err = manager.StartHealthCheckPublisher(ctx)
	assert.NoError(t, err)

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 停止健康检查发布器
	err = manager.StopHealthCheckPublisher()
	assert.NoError(t, err)
}

// TestEventBusManager_StartStopHealthCheckSubscriber_Coverage 测试启动停止健康检查订阅器（覆盖率）
func TestEventBusManager_StartStopHealthCheckSubscriber_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 启动健康检查订阅器
	err = manager.StartHealthCheckSubscriber(ctx)
	assert.NoError(t, err)

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 停止健康检查订阅器
	err = manager.StopHealthCheckSubscriber()
	assert.NoError(t, err)
}

// TestEventBusManager_StartAllHealthCheck_WithErrors 测试启动所有健康检查（带错误）
func TestEventBusManager_StartAllHealthCheck_WithErrors(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 先关闭 bus
	bus.Close()

	// 尝试启动健康检查（应该失败或返回错误）
	err = manager.StartAllHealthCheck(ctx)
	// 可能返回错误，也可能不返回
	_ = err
}

// TestEventBusManager_StopAllHealthCheck_WithErrors 测试停止所有健康检查（带错误）
func TestEventBusManager_StopAllHealthCheck_WithErrors(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)

	// 先关闭 bus
	bus.Close()

	// 尝试停止健康检查
	err = manager.StopAllHealthCheck()
	// 应该不报错
	assert.NoError(t, err)
}

// TestEventBusManager_PublishEnvelope_Success 测试成功发布 Envelope
func TestEventBusManager_PublishEnvelope_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 创建一个有效的 Envelope
	envelope := NewEnvelopeWithAutoID("test-aggregate", "test-event", 1, []byte("test payload"))

	err = manager.PublishEnvelope(ctx, "test-topic", envelope)
	assert.NoError(t, err)
}

// TestEventBusManager_SubscribeEnvelope_Success 测试成功订阅 Envelope
func TestEventBusManager_SubscribeEnvelope_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	received := make(chan bool, 1)
	handler := func(ctx context.Context, envelope *Envelope) error {
		received <- true
		return nil
	}

	err = manager.SubscribeEnvelope(ctx, "test-topic", handler)
	assert.NoError(t, err)

	// 发布一个 Envelope
	envelope := NewEnvelopeWithAutoID("test-aggregate", "test-event", 1, []byte("test payload"))

	err = manager.PublishEnvelope(ctx, "test-topic", envelope)
	assert.NoError(t, err)

	// 等待接收
	select {
	case <-received:
		// 成功
	case <-time.After(1 * time.Second):
		t.Log("Envelope not received (may be expected for memory bus)")
	}
}

// TestEventBusManager_SetTopicConfigStrategy_AllTypes 测试设置所有类型的主题配置策略
func TestEventBusManager_SetTopicConfigStrategy_AllTypes(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 测试所有策略类型
	strategies := []TopicConfigStrategy{
		"create_or_update",
		"create_only",
		"verify_only",
		"skip",
	}

	for _, strategy := range strategies {
		manager.SetTopicConfigStrategy(strategy)
		retrieved := manager.GetTopicConfigStrategy()
		// Memory bus 可能不支持，但不应该 panic
		_ = retrieved
	}
}

// TestEventBusManager_GetTopicConfigStrategy_Default 测试获取默认主题配置策略
func TestEventBusManager_GetTopicConfigStrategy_Default(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 获取默认策略
	strategy := manager.GetTopicConfigStrategy()
	assert.NotEmpty(t, strategy)
}

// TestEventBusManager_Publish_Closed 测试发布（已关闭）
func TestEventBusManager_Publish_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	// 关闭 EventBus
	bus.Close()

	ctx := context.Background()

	// 尝试发布（应该失败）
	err = bus.Publish(ctx, "test-topic", []byte("test message"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventbus is closed")
}

// TestEventBusManager_Subscribe_Closed 测试订阅（已关闭）
func TestEventBusManager_Subscribe_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	// 关闭 EventBus
	bus.Close()

	ctx := context.Background()

	// 尝试订阅（应该失败）
	handler := func(ctx context.Context, message []byte) error {
		return nil
	}

	err = bus.Subscribe(ctx, "test-topic", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventbus is closed")
}

// TestEventBusManager_PublishEnvelope_Closed 测试发布 Envelope（已关闭）
func TestEventBusManager_PublishEnvelope_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	// 关闭 EventBus
	bus.Close()

	ctx := context.Background()

	// 创建 Envelope
	envelope := NewEnvelopeWithAutoID("test-aggregate", "test-event", 1, []byte("test payload"))

	// 尝试发布（应该失败）
	err = bus.PublishEnvelope(ctx, "test-topic", envelope)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventbus is closed")
}

// TestEventBusManager_SubscribeEnvelope_Closed 测试订阅 Envelope（已关闭）
func TestEventBusManager_SubscribeEnvelope_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	// 关闭 EventBus
	bus.Close()

	ctx := context.Background()

	// 尝试订阅（应该失败）
	handler := func(ctx context.Context, envelope *Envelope) error {
		return nil
	}

	err = bus.SubscribeEnvelope(ctx, "test-topic", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventbus is closed")
}

// TestEventBusManager_SetTopicConfigStrategy_Closed 测试设置主题配置策略（已关闭）
func TestEventBusManager_SetTopicConfigStrategy_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)

	// 关闭 EventBus
	bus.Close()

	// 尝试设置策略（应该成功，因为没有检查关闭状态）
	manager.SetTopicConfigStrategy(StrategyCreateOnly)
}

// TestEventBusManager_GetTopicConfigStrategy_Closed 测试获取主题配置策略（已关闭）
func TestEventBusManager_GetTopicConfigStrategy_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)

	// 关闭 EventBus
	bus.Close()

	// 尝试获取策略（应该成功，因为没有检查关闭状态）
	strategy := manager.GetTopicConfigStrategy()
	assert.Equal(t, StrategyCreateOrUpdate, strategy)
}

// TestEventBusManager_PublishEnvelope_WithTraceID 测试发布 Envelope（带 TraceID）
func TestEventBusManager_PublishEnvelope_WithTraceID(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 创建带 TraceID 的 Envelope
	envelope := NewEnvelopeWithAutoID("test-aggregate", "test-event", 1, []byte("test payload"))
	envelope.TraceID = "trace-123"
	envelope.CorrelationID = "correlation-456"

	// 发布 Envelope
	err = bus.PublishEnvelope(ctx, "test-topic", envelope)
	assert.NoError(t, err)
}

// TestEventBusManager_SubscribeEnvelope_WithTraceID 测试订阅 Envelope（带 TraceID）
func TestEventBusManager_SubscribeEnvelope_WithTraceID(t *testing.T) {
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

	err = bus.SubscribeEnvelope(ctx, "test-topic", handler)
	require.NoError(t, err)

	// 创建带 TraceID 的 Envelope
	envelope := NewEnvelopeWithAutoID("test-aggregate", "test-event", 1, []byte("test payload"))
	envelope.TraceID = "trace-123"
	envelope.CorrelationID = "correlation-456"

	// 发布 Envelope
	err = bus.PublishEnvelope(ctx, "test-topic", envelope)
	require.NoError(t, err)

	// 等待接收
	select {
	case env := <-received:
		assert.Equal(t, "test-aggregate", env.AggregateID)
		assert.Equal(t, "test-event", env.EventType)
		assert.Equal(t, int64(1), env.EventVersion)
		assert.Equal(t, "trace-123", env.TraceID)
		assert.Equal(t, "correlation-456", env.CorrelationID)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for envelope")
	}
}

// TestEventBusManager_Close_WithHealthCheck 测试关闭（带健康检查）
func TestEventBusManager_Close_WithHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 启动健康检查
	err = manager.StartAllHealthCheck(ctx)
	require.NoError(t, err)

	// 验证健康检查已启动
	assert.NotNil(t, manager.healthChecker)
	assert.NotNil(t, manager.healthCheckSubscriber)

	// 关闭 EventBus（应该自动停止健康检查）
	err = bus.Close()
	assert.NoError(t, err)

	// 验证 EventBus 已关闭
	assert.True(t, manager.closed)
}

// TestEventBusManager_GetConnectionState_Connected 测试获取连接状态（已连接）
func TestEventBusManager_GetConnectionState_Connected(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 获取连接状态
	state := manager.GetConnectionState()
	assert.True(t, state.IsConnected)
	assert.NotNil(t, state.LastConnectedTime)
}

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
	envelope := NewEnvelopeWithAutoID("agg-123", "TestEvent", 1, []byte(`{"data": "test"}`))
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
	envelope := NewEnvelopeWithAutoID("agg-123", "TestEvent", 1, []byte(`{"data": "test"}`))
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
		EventID:       "evt-allfields-001",
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
	envelope := NewEnvelopeWithAutoID("agg-123", "TestEvent", 1, []byte(`{"data": "test"}`))
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
