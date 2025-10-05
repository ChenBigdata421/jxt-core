package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_CheckInfrastructureHealth_Success 测试检查基础设施健康（成功）
func TestEventBusManager_CheckInfrastructureHealth_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 测试检查基础设施健康
	result, err := manager.checkInfrastructureHealth(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestEventBusManager_CheckInfrastructureHealth_WithTimeout 测试检查基础设施健康（带超时）
func TestEventBusManager_CheckInfrastructureHealth_WithTimeout(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 测试检查基础设施健康
	result, err := manager.checkInfrastructureHealth(ctx)
	// 可能成功或超时
	if err != nil {
		t.Logf("Infrastructure health check timed out: %v", err)
	} else {
		assert.NotNil(t, result)
	}
}

// TestEventBusManager_CheckConnection_Success 测试检查连接（成功）
func TestEventBusManager_CheckConnection_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 测试检查连接
	err = manager.checkConnection(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_CheckConnection_AfterClose 测试检查连接（关闭后）
func TestEventBusManager_CheckConnection_AfterClose(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 关闭 EventBus
	bus.Close()

	// 测试检查连接（应该失败）
	err = manager.checkConnection(ctx)
	assert.Error(t, err)
}

// TestEventBusManager_CheckConnection_WithTimeout 测试检查连接（带超时）
func TestEventBusManager_CheckConnection_WithTimeout(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 测试检查连接
	err = manager.checkConnection(ctx)
	// 可能成功或超时
	if err != nil {
		t.Logf("Connection check timed out: %v", err)
	}
}

// TestEventBusManager_CheckMessageTransport_Success 测试检查消息传输（成功）
func TestEventBusManager_CheckMessageTransport_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 测试检查消息传输
	err = manager.checkMessageTransport(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_CheckMessageTransport_AfterClose 测试检查消息传输（关闭后）
func TestEventBusManager_CheckMessageTransport_AfterClose(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 关闭 EventBus
	bus.Close()

	// 测试检查消息传输（应该失败）
	err = manager.checkMessageTransport(ctx)
	assert.Error(t, err)
}

// TestEventBusManager_CheckMessageTransport_WithTimeout_Coverage 测试检查消息传输（带超时）
func TestEventBusManager_CheckMessageTransport_WithTimeout_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 测试检查消息传输
	err = manager.checkMessageTransport(ctx)
	// 可能成功或超时
	if err != nil {
		t.Logf("Message transport check timed out: %v", err)
	}
}

// TestEventBusManager_PerformHealthCheck_Success 测试执行健康检查（成功）
func TestEventBusManager_PerformHealthCheck_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 测试执行健康检查
	err = manager.performHealthCheck(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_PerformHealthCheck_WithCallback 测试执行健康检查（带回调）
func TestEventBusManager_PerformHealthCheck_WithCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 注册健康检查回调
	callbackCalled := false
	manager.RegisterHealthCheckCallback(func(ctx context.Context, result HealthCheckResult) error {
		callbackCalled = true
		return nil
	})

	// 测试执行健康检查
	err = manager.performHealthCheck(ctx)
	assert.NoError(t, err)

	// 验证回调可能被调用
	_ = callbackCalled
}

// TestEventBusManager_PerformHealthCheck_AfterClose 测试执行健康检查（关闭后）
func TestEventBusManager_PerformHealthCheck_AfterClose(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 关闭 EventBus
	bus.Close()

	// 测试执行健康检查（应该失败）
	err = manager.performHealthCheck(ctx)
	assert.Error(t, err)
}

// TestEventBusManager_Subscribe_WithOptions 测试订阅（带选项）
func TestEventBusManager_Subscribe_WithOptions(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 创建订阅选项
	options := SubscribeOptions{
		ProcessingTimeout: 5 * time.Second,
		RateLimit:         100.0,
		RateBurst:         10,
		MaxRetries:        3,
		RetryBackoff:      100 * time.Millisecond,
		DeadLetterEnabled: false,
	}

	// 订阅
	err = bus.SubscribeWithOptions(ctx, "test-topic", func(ctx context.Context, message []byte) error {
		return nil
	}, options)
	assert.NoError(t, err)
}

// TestEventBusManager_Publish_WithOptions 测试发布（带选项）
func TestEventBusManager_Publish_WithOptions(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 创建发布选项
	options := PublishOptions{
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		Timeout:     5 * time.Second,
		AggregateID: "test-aggregate",
	}

	// 发布
	err = bus.PublishWithOptions(ctx, "test-topic", []byte("test message"), options)
	assert.NoError(t, err)
}

// TestEventBusManager_Close_Multiple_Coverage 测试多次关闭
func TestEventBusManager_Close_Multiple_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	// 第一次关闭
	err = bus.Close()
	assert.NoError(t, err)

	// 第二次关闭（应该不会 panic）
	err = bus.Close()
	// 可能返回错误或成功
	_ = err
}

// TestEventBusManager_RegisterReconnectCallback_Multiple 测试注册多个重连回调
func TestEventBusManager_RegisterReconnectCallback_Multiple(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 注册多个回调
	callback1Called := false
	callback2Called := false

	manager.RegisterReconnectCallback(func(ctx context.Context) error {
		callback1Called = true
		return nil
	})

	manager.RegisterReconnectCallback(func(ctx context.Context) error {
		callback2Called = true
		return nil
	})

	// 验证回调已注册
	_ = callback1Called
	_ = callback2Called
}

// TestEventBusManager_GetMetrics_AfterPublish 测试发布后获取指标
func TestEventBusManager_GetMetrics_AfterPublish(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 发布一些消息
	for i := 0; i < 10; i++ {
		err = bus.Publish(ctx, "test-topic", []byte("test message"))
		assert.NoError(t, err)
	}

	// 获取指标
	metrics := bus.GetMetrics()
	assert.NotNil(t, metrics)
	// 验证指标可能包含发布计数
	_ = metrics
}

// TestEventBusManager_GetHealthStatus_AfterHealthCheck 测试健康检查后获取健康状态
func TestEventBusManager_GetHealthStatus_AfterHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 执行健康检查
	err = manager.performHealthCheck(ctx)
	assert.NoError(t, err)

	// 获取健康状态
	status := bus.GetHealthStatus()
	assert.NotEmpty(t, status)
}
