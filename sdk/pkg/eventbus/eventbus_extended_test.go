package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_GetHealthStatus 测试获取健康状态（已废弃方法）
func TestEventBusManager_GetHealthStatus(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	status := bus.GetHealthStatus()
	assert.NotNil(t, status)
}

// TestEventBusManager_Start 测试启动事件总线
func TestEventBusManager_Start(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	err = bus.Start(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_Start_Closed 测试启动已关闭的事件总线
func TestEventBusManager_Start_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	// 先关闭
	bus.Close()

	// 尝试启动
	ctx := context.Background()
	err = bus.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestEventBusManager_Stop 测试停止事件总线
func TestEventBusManager_Stop(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	err = bus.Stop()
	assert.NoError(t, err)
}

// TestEventBusManager_PublishWithOptions 测试使用选项发布消息
func TestEventBusManager_PublishWithOptions(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	opts := PublishOptions{
		Metadata: map[string]string{"key": "value"},
		Timeout:  10 * time.Second,
	}

	err = bus.PublishWithOptions(ctx, "test-topic", []byte("test"), opts)
	assert.NoError(t, err)
}

// TestEventBusManager_SetMessageFormatter 测试设置消息格式化器
func TestEventBusManager_SetMessageFormatter(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	formatter := &DefaultMessageFormatter{}
	err = bus.SetMessageFormatter(formatter)
	assert.NoError(t, err)
}

// TestEventBusManager_RegisterPublishCallback 测试注册发布回调
func TestEventBusManager_RegisterPublishCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	callback := func(ctx context.Context, topic string, message []byte, err error) error {
		return nil
	}

	err = bus.RegisterPublishCallback(callback)
	assert.NoError(t, err)
}

// TestEventBusManager_SubscribeWithOptions 测试使用选项订阅消息
func TestEventBusManager_SubscribeWithOptions(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	handler := func(ctx context.Context, message []byte) error {
		return nil
	}
	opts := SubscribeOptions{
		ProcessingTimeout: 30 * time.Second,
		RateLimit:         100.0,
	}

	err = bus.SubscribeWithOptions(ctx, "test-topic", handler, opts)
	assert.NoError(t, err)
}

// TestEventBusManager_RegisterSubscriberBacklogCallback 测试注册订阅端积压回调
func TestEventBusManager_RegisterSubscriberBacklogCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	callback := func(ctx context.Context, state BacklogState) error {
		return nil
	}

	err = bus.RegisterSubscriberBacklogCallback(callback)
	assert.NoError(t, err)
}

// TestEventBusManager_StartSubscriberBacklogMonitoring 测试启动订阅端积压监控
func TestEventBusManager_StartSubscriberBacklogMonitoring(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	err = bus.StartSubscriberBacklogMonitoring(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_StopSubscriberBacklogMonitoring 测试停止订阅端积压监控
func TestEventBusManager_StopSubscriberBacklogMonitoring(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	err = bus.StopSubscriberBacklogMonitoring()
	assert.NoError(t, err)
}

// TestEventBusManager_RegisterBacklogCallback_Deprecated 测试注册积压回调（已废弃）
func TestEventBusManager_RegisterBacklogCallback_Deprecated(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	callback := func(ctx context.Context, state BacklogState) error {
		return nil
	}

	err = bus.RegisterBacklogCallback(callback)
	assert.NoError(t, err)
}

// TestEventBusManager_StartBacklogMonitoring_Deprecated 测试启动积压监控（已废弃）
func TestEventBusManager_StartBacklogMonitoring_Deprecated(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	err = bus.StartBacklogMonitoring(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_StopBacklogMonitoring_Deprecated 测试停止积压监控（已废弃）
func TestEventBusManager_StopBacklogMonitoring_Deprecated(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	err = bus.StopBacklogMonitoring()
	assert.NoError(t, err)
}

// TestEventBusManager_RegisterPublisherBacklogCallback 测试注册发送端积压回调
func TestEventBusManager_RegisterPublisherBacklogCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	callback := func(ctx context.Context, state PublisherBacklogState) error {
		return nil
	}

	err = bus.RegisterPublisherBacklogCallback(callback)
	assert.NoError(t, err)
}

// TestEventBusManager_RegisterBusinessHealthCheck 测试注册业务健康检查
func TestEventBusManager_RegisterBusinessHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	// 创建一个模拟的业务健康检查器
	checker := &mockBusinessHealthChecker{}

	manager := bus.(*eventBusManager)
	manager.RegisterBusinessHealthCheck(checker)

	// 验证已注册
	assert.NotNil(t, manager.businessHealthChecker)
}

// mockBusinessHealthChecker 模拟业务健康检查器
type mockBusinessHealthChecker struct{}

func (m *mockBusinessHealthChecker) CheckBusinessHealth(ctx context.Context) error {
	return nil
}

func (m *mockBusinessHealthChecker) GetBusinessMetrics() interface{} {
	return map[string]interface{}{
		"test": "metrics",
	}
}

func (m *mockBusinessHealthChecker) GetBusinessConfig() interface{} {
	return map[string]interface{}{
		"test": "config",
	}
}

// TestEventBusManager_GetEventBusMetrics 测试获取EventBus指标
func TestEventBusManager_GetEventBusMetrics(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	metrics := manager.getEventBusMetrics()

	assert.Equal(t, "connected", metrics.ConnectionStatus)
	assert.NotZero(t, metrics.LastSuccessTime)
}

// TestEventBusManager_UpdateMetrics_Extended 测试更新指标（扩展）
func TestEventBusManager_UpdateMetrics_Extended(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 测试发布成功
	manager.updateMetrics(true, true, 10*time.Millisecond)
	assert.Greater(t, manager.metrics.MessagesPublished, int64(0))

	// 测试发布失败
	manager.updateMetrics(false, true, 10*time.Millisecond)
	assert.Greater(t, manager.metrics.PublishErrors, int64(0))

	// 测试消费成功
	manager.updateMetrics(true, false, 10*time.Millisecond)
	assert.Greater(t, manager.metrics.MessagesConsumed, int64(0))

	// 测试消费失败
	manager.updateMetrics(false, false, 10*time.Millisecond)
	assert.Greater(t, manager.metrics.ConsumeErrors, int64(0))
}
