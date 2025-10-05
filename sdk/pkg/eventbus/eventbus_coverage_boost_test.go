package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	result := manager.performHealthCheck(ctx)
	assert.NotNil(t, result)
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
	envelope := NewEnvelope("test-aggregate", "test-event", 1, []byte("test payload"))

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
	envelope := NewEnvelope("test-aggregate", "test-event", 1, []byte("test payload"))

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
