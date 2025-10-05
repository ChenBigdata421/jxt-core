package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_SetTopicConfigStrategy_CreateOnly 测试设置主题配置策略（仅创建）
func TestEventBusManager_SetTopicConfigStrategy_CreateOnly(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 设置策略
	manager.SetTopicConfigStrategy(StrategyCreateOnly)

	// 验证策略已设置
	strategy := manager.GetTopicConfigStrategy()
	assert.Equal(t, StrategyCreateOnly, strategy)
}

// TestEventBusManager_SetTopicConfigStrategy_CreateOrUpdate 测试设置主题配置策略（创建或更新）
func TestEventBusManager_SetTopicConfigStrategy_CreateOrUpdate(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 设置策略
	manager.SetTopicConfigStrategy(StrategyCreateOrUpdate)

	// 验证策略已设置
	strategy := manager.GetTopicConfigStrategy()
	assert.Equal(t, StrategyCreateOrUpdate, strategy)
}

// TestEventBusManager_SetTopicConfigStrategy_ValidateOnly 测试设置主题配置策略（仅验证）
func TestEventBusManager_SetTopicConfigStrategy_ValidateOnly(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 设置策略
	manager.SetTopicConfigStrategy(StrategyValidateOnly)

	// 验证策略已设置
	strategy := manager.GetTopicConfigStrategy()
	assert.Equal(t, StrategyValidateOnly, strategy)
}

// TestEventBusManager_SetTopicConfigStrategy_Skip 测试设置主题配置策略（跳过）
func TestEventBusManager_SetTopicConfigStrategy_Skip(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 设置策略
	manager.SetTopicConfigStrategy(StrategySkip)

	// 验证策略已设置
	strategy := manager.GetTopicConfigStrategy()
	assert.Equal(t, StrategySkip, strategy)
}

// TestEventBusManager_GetTopicConfigStrategy_Default_Coverage 测试获取主题配置策略（默认）
func TestEventBusManager_GetTopicConfigStrategy_Default_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 获取默认策略（Memory EventBus 默认是 StrategyCreateOrUpdate）
	strategy := manager.GetTopicConfigStrategy()
	assert.Equal(t, StrategyCreateOrUpdate, strategy)
}

// TestEventBusManager_SetTopicConfigStrategy_Multiple 测试多次设置主题配置策略
func TestEventBusManager_SetTopicConfigStrategy_Multiple(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 第一次设置
	manager.SetTopicConfigStrategy(StrategyCreateOnly)
	assert.Equal(t, StrategyCreateOnly, manager.GetTopicConfigStrategy())

	// 第二次设置
	manager.SetTopicConfigStrategy(StrategyCreateOrUpdate)
	assert.Equal(t, StrategyCreateOrUpdate, manager.GetTopicConfigStrategy())

	// 第三次设置
	manager.SetTopicConfigStrategy(StrategyValidateOnly)
	assert.Equal(t, StrategyValidateOnly, manager.GetTopicConfigStrategy())
}

// TestEventBusManager_StopAllHealthCheck_Success 测试停止所有健康检查（成功）
func TestEventBusManager_StopAllHealthCheck_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 启动所有健康检查
	ctx := context.Background()
	err = manager.StartAllHealthCheck(ctx)
	assert.NoError(t, err)

	// 等待一下
	time.Sleep(100 * time.Millisecond)

	// 停止所有健康检查
	err = manager.StopAllHealthCheck()
	assert.NoError(t, err)
}

// TestEventBusManager_StopAllHealthCheck_NotStarted 测试停止所有健康检查（未启动）
func TestEventBusManager_StopAllHealthCheck_NotStarted(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 直接停止（未启动）
	err = manager.StopAllHealthCheck()
	assert.NoError(t, err)
}

// TestEventBusManager_StopAllHealthCheck_Multiple 测试多次停止所有健康检查
func TestEventBusManager_StopAllHealthCheck_Multiple(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 启动所有健康检查
	ctx := context.Background()
	err = manager.StartAllHealthCheck(ctx)
	assert.NoError(t, err)

	// 第一次停止
	err = manager.StopAllHealthCheck()
	assert.NoError(t, err)

	// 第二次停止
	err = manager.StopAllHealthCheck()
	assert.NoError(t, err)
}

// TestEventBusManager_PublishEnvelope_Success_Coverage 测试发布 Envelope（成功）
func TestEventBusManager_PublishEnvelope_Success_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 创建 Envelope
	envelope := NewEnvelope("test-aggregate", "test-event", 1, []byte("test payload"))
	envelope.Timestamp = time.Now()
	envelope.TraceID = "test-trace-id"

	// 发布 Envelope
	err = bus.PublishEnvelope(ctx, "test-topic", envelope)
	assert.NoError(t, err)
}

// TestEventBusManager_PublishEnvelope_NilEnvelope 测试发布 Envelope（nil）
func TestEventBusManager_PublishEnvelope_NilEnvelope(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 发布 nil Envelope
	err = bus.PublishEnvelope(ctx, "test-topic", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "envelope cannot be nil")
}

// TestEventBusManager_PublishEnvelope_EmptyTopic 测试发布 Envelope（空主题）
func TestEventBusManager_PublishEnvelope_EmptyTopic(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 创建 Envelope
	envelope := NewEnvelope("test-aggregate", "test-event", 1, []byte("test payload"))

	// 发布 Envelope（空主题）
	err = bus.PublishEnvelope(ctx, "", envelope)
	assert.Error(t, err)
}

// TestEventBusManager_SubscribeEnvelope_Success_Coverage 测试订阅 Envelope（成功）
func TestEventBusManager_SubscribeEnvelope_Success_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅 Envelope
	received := make(chan *Envelope, 1)
	err = bus.SubscribeEnvelope(ctx, "test-topic", func(ctx context.Context, envelope *Envelope) error {
		received <- envelope
		return nil
	})
	assert.NoError(t, err)

	// 发布 Envelope
	envelope := NewEnvelope("test-aggregate", "test-event", 1, []byte("test payload"))
	envelope.Timestamp = time.Now()

	err = bus.PublishEnvelope(ctx, "test-topic", envelope)
	assert.NoError(t, err)

	// 等待接收
	select {
	case env := <-received:
		assert.NotNil(t, env)
		assert.Equal(t, "test-aggregate", env.AggregateID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for envelope")
	}
}

// TestEventBusManager_SubscribeEnvelope_EmptyTopic 测试订阅 Envelope（空主题）
func TestEventBusManager_SubscribeEnvelope_EmptyTopic(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅空主题
	err = bus.SubscribeEnvelope(ctx, "", func(ctx context.Context, envelope *Envelope) error {
		return nil
	})
	assert.Error(t, err)
}

// TestEventBusManager_SubscribeEnvelope_NilHandler 测试订阅 Envelope（nil 处理器）
func TestEventBusManager_SubscribeEnvelope_NilHandler(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅 nil 处理器
	err = bus.SubscribeEnvelope(ctx, "test-topic", nil)
	assert.Error(t, err)
}
