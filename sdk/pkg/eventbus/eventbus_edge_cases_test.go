package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	envelope := NewEnvelope("test-aggregate", "test-event", 1, []byte("test payload"))

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
	envelope := NewEnvelope("test-aggregate", "test-event", 1, []byte("test payload"))
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
	envelope := NewEnvelope("test-aggregate", "test-event", 1, []byte("test payload"))
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
