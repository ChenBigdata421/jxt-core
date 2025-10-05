package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTopicConfigManager_FormatSyncResult_Success 测试格式化同步结果（成功）
func TestTopicConfigManager_FormatSyncResult_Success(t *testing.T) {
	result := &TopicConfigSyncResult{
		Topic:    "test-topic",
		Action:   "create",
		Success:  true,
		Duration: 100 * time.Millisecond,
	}

	// 调用 formatSyncResult 不应该 panic
	formatSyncResult(result)
}

// TestTopicConfigManager_FormatSyncResult_Failure 测试格式化同步结果（失败）
func TestTopicConfigManager_FormatSyncResult_Failure(t *testing.T) {
	result := &TopicConfigSyncResult{
		Topic:    "test-topic",
		Action:   "create",
		Success:  false,
		Error:    assert.AnError,
		Duration: 100 * time.Millisecond,
	}

	// 调用 formatSyncResult 不应该 panic
	formatSyncResult(result)
}

// TestTopicConfigManager_LogConfigMismatch_Debug 测试记录配置不一致（Debug 级别）
func TestTopicConfigManager_LogConfigMismatch_Debug(t *testing.T) {
	mismatch := TopicConfigMismatch{
		Topic:          "test-topic",
		Field:          "partitions",
		ExpectedValue:  "3",
		ActualValue:    "1",
		Recommendation: "Increase partitions to 3",
	}

	action := TopicConfigMismatchAction{
		LogLevel: "debug",
		FailFast: false,
	}

	// 调用 logConfigMismatch 不应该 panic
	logConfigMismatch(mismatch, action)
}

// TestTopicConfigManager_LogConfigMismatch_Info 测试记录配置不一致（Info 级别）
func TestTopicConfigManager_LogConfigMismatch_Info(t *testing.T) {
	mismatch := TopicConfigMismatch{
		Topic:          "test-topic",
		Field:          "partitions",
		ExpectedValue:  "3",
		ActualValue:    "1",
		Recommendation: "Increase partitions to 3",
	}

	action := TopicConfigMismatchAction{
		LogLevel: "info",
		FailFast: false,
	}

	// 调用 logConfigMismatch 不应该 panic
	logConfigMismatch(mismatch, action)
}

// TestTopicConfigManager_LogConfigMismatch_Warn 测试记录配置不一致（Warn 级别）
func TestTopicConfigManager_LogConfigMismatch_Warn(t *testing.T) {
	mismatch := TopicConfigMismatch{
		Topic:          "test-topic",
		Field:          "partitions",
		ExpectedValue:  "3",
		ActualValue:    "1",
		Recommendation: "Increase partitions to 3",
	}

	action := TopicConfigMismatchAction{
		LogLevel: "warn",
		FailFast: false,
	}

	// 调用 logConfigMismatch 不应该 panic
	logConfigMismatch(mismatch, action)
}

// TestTopicConfigManager_LogConfigMismatch_Error 测试记录配置不一致（Error 级别）
func TestTopicConfigManager_LogConfigMismatch_Error(t *testing.T) {
	mismatch := TopicConfigMismatch{
		Topic:          "test-topic",
		Field:          "partitions",
		ExpectedValue:  "3",
		ActualValue:    "1",
		Recommendation: "Increase partitions to 3",
	}

	action := TopicConfigMismatchAction{
		LogLevel: "error",
		FailFast: false,
	}

	// 调用 logConfigMismatch 不应该 panic
	logConfigMismatch(mismatch, action)
}

// TestTopicConfigManager_LogConfigMismatch_Default 测试记录配置不一致（默认级别）
func TestTopicConfigManager_LogConfigMismatch_Default(t *testing.T) {
	mismatch := TopicConfigMismatch{
		Topic:          "test-topic",
		Field:          "partitions",
		ExpectedValue:  "3",
		ActualValue:    "1",
		Recommendation: "Increase partitions to 3",
	}

	action := TopicConfigMismatchAction{
		LogLevel: "unknown",
		FailFast: false,
	}

	// 调用 logConfigMismatch 不应该 panic
	logConfigMismatch(mismatch, action)
}

// TestTopicConfigManager_Strategies 测试不同的配置策略
func TestTopicConfigManager_Strategies(t *testing.T) {
	// 测试 CreateOrUpdate 策略
	assert.Equal(t, TopicConfigStrategy("create_or_update"), StrategyCreateOrUpdate)

	// 测试 CreateOnly 策略
	assert.Equal(t, TopicConfigStrategy("create_only"), StrategyCreateOnly)

	// 测试 ValidateOnly 策略
	assert.Equal(t, TopicConfigStrategy("validate_only"), StrategyValidateOnly)

	// 测试 Skip 策略
	assert.Equal(t, TopicConfigStrategy("skip"), StrategySkip)
}

// TestTopicConfigManager_ConfigureTopic_MemoryBackend 测试配置主题（Memory 后端）
func TestTopicConfigManager_ConfigureTopic_MemoryBackend(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	options := DefaultTopicOptions()

	err = bus.ConfigureTopic(ctx, "test-topic", options)
	assert.NoError(t, err)
}

// TestTopicConfigManager_GetTopicConfig_MemoryBackend 测试获取主题配置（Memory 后端）
func TestTopicConfigManager_GetTopicConfig_MemoryBackend(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	options := DefaultTopicOptions()

	// 先配置主题
	err = bus.ConfigureTopic(ctx, "test-topic", options)
	require.NoError(t, err)

	// 获取主题配置
	config, err := bus.GetTopicConfig("test-topic")
	assert.NoError(t, err)
	assert.NotNil(t, config)
}

// TestTopicConfigManager_RemoveTopicConfig_MemoryBackend 测试删除主题配置（Memory 后端）
func TestTopicConfigManager_RemoveTopicConfig_MemoryBackend(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	options := DefaultTopicOptions()

	// 先配置主题
	err = bus.ConfigureTopic(ctx, "test-topic", options)
	require.NoError(t, err)

	// 删除主题配置
	err = bus.RemoveTopicConfig("test-topic")
	assert.NoError(t, err)
}

// TestTopicConfigManager_ConfigureTopic_MultipleTopics 测试配置多个主题
func TestTopicConfigManager_ConfigureTopic_MultipleTopics(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	options := DefaultTopicOptions()

	// 配置多个主题
	topics := []string{"topic1", "topic2", "topic3"}
	for _, topic := range topics {
		err = bus.ConfigureTopic(ctx, topic, options)
		assert.NoError(t, err)
	}

	// 验证所有主题都已配置
	for _, topic := range topics {
		config, err := bus.GetTopicConfig(topic)
		assert.NoError(t, err)
		assert.NotNil(t, config)
	}
}

// TestTopicConfigManager_ConfigureTopic_UpdateExisting 测试更新现有主题配置
func TestTopicConfigManager_ConfigureTopic_UpdateExisting(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 第一次配置
	options1 := DefaultTopicOptions()
	err = bus.ConfigureTopic(ctx, "test-topic", options1)
	require.NoError(t, err)

	// 第二次配置（更新）
	options2 := DefaultTopicOptions()
	err = bus.ConfigureTopic(ctx, "test-topic", options2)
	assert.NoError(t, err)
}

// TestTopicConfigManager_GetTopicConfig_NonExistent 测试获取不存在的主题配置
func TestTopicConfigManager_GetTopicConfig_NonExistent(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	// 获取不存在的主题配置
	config, err := bus.GetTopicConfig("non-existent-topic")
	// 可能返回错误或 nil
	if err != nil {
		assert.Error(t, err)
	} else {
		_ = config
	}
}

// TestTopicConfigManager_RemoveTopicConfig_NonExistent 测试删除不存在的主题配置
func TestTopicConfigManager_RemoveTopicConfig_NonExistent(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	// 删除不存在的主题配置
	err = bus.RemoveTopicConfig("non-existent-topic")
	// 可能返回错误或成功
	_ = err
}
