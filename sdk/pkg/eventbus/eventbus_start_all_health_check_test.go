package eventbus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_StartAllHealthCheck_Success_Coverage 测试启动所有健康检查（成功）
func TestEventBusManager_StartAllHealthCheck_Success_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 启动所有健康检查
	err = manager.StartAllHealthCheck(ctx)
	assert.NoError(t, err)

	// 验证健康检查已启动
	assert.NotNil(t, manager.healthChecker)
	assert.NotNil(t, manager.healthCheckSubscriber)

	// 停止健康检查
	manager.StopAllHealthCheck()
}

// TestEventBusManager_StartAllHealthCheck_AlreadyStarted_Coverage 测试启动所有健康检查（已启动）
func TestEventBusManager_StartAllHealthCheck_AlreadyStarted_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 第一次启动
	err = manager.StartAllHealthCheck(ctx)
	require.NoError(t, err)

	// 第二次启动（应该成功，因为会先停止）
	err = manager.StartAllHealthCheck(ctx)
	assert.NoError(t, err)

	// 停止健康检查
	manager.StopAllHealthCheck()
}

// TestEventBusManager_StartAllHealthCheck_Closed_Coverage 测试启动所有健康检查（已关闭）
func TestEventBusManager_StartAllHealthCheck_Closed_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 关闭 EventBus
	bus.Close()

	// 尝试启动健康检查（应该失败）
	err = manager.StartAllHealthCheck(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventbus is closed")
}

// TestEventBusManager_Publish_NilMessage_Coverage 测试发布（nil 消息）
func TestEventBusManager_Publish_NilMessage_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 发布 nil 消息（应该成功，因为 nil 是有效的字节数组）
	err = bus.Publish(ctx, "test-topic", nil)
	assert.NoError(t, err)
}

// TestEventBusManager_GetConnectionState_Closed_Coverage 测试获取连接状态（已关闭）
func TestEventBusManager_GetConnectionState_Closed_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)

	// 关闭 EventBus
	bus.Close()

	// 获取连接状态
	state := manager.GetConnectionState()
	assert.False(t, state.IsConnected)
}

// TestEventBusManager_SetTopicConfigStrategy_AllStrategies_Coverage 测试设置主题配置策略（所有策略）
func TestEventBusManager_SetTopicConfigStrategy_AllStrategies_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	strategies := []TopicConfigStrategy{
		StrategyCreateOnly,
		StrategyCreateOrUpdate,
		StrategyValidateOnly,
		StrategySkip,
	}

	for _, strategy := range strategies {
		manager.SetTopicConfigStrategy(strategy)
		// 验证策略已设置（Memory EventBus 支持）
		currentStrategy := manager.GetTopicConfigStrategy()
		assert.Equal(t, strategy, currentStrategy, "Strategy should be set to %s", strategy)
	}
}
