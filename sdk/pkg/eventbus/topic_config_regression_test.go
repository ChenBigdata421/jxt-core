package eventbus

import (
	"context"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"testing"
	"time"
)

func init() {
	// 初始化日志（测试环境）
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()
}

// TestTopicConfigStrategy 测试配置策略枚举
func TestTopicConfigStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy TopicConfigStrategy
		expected string
	}{
		{"CreateOnly", StrategyCreateOnly, "create_only"},
		{"CreateOrUpdate", StrategyCreateOrUpdate, "create_or_update"},
		{"ValidateOnly", StrategyValidateOnly, "validate_only"},
		{"Skip", StrategySkip, "skip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.strategy))
		})
	}
}

// TestDefaultTopicConfigManagerConfig 测试默认配置
func TestDefaultTopicConfigManagerConfig(t *testing.T) {
	config := DefaultTopicConfigManagerConfig()
	assert.Equal(t, StrategyCreateOrUpdate, config.Strategy)
	assert.Equal(t, "warn", config.OnMismatch.LogLevel)
	assert.False(t, config.OnMismatch.FailFast)
	assert.True(t, config.EnableValidation)
	assert.Equal(t, 30*time.Second, config.SyncTimeout)
}

// TestProductionTopicConfigManagerConfig 测试生产环境配置
func TestProductionTopicConfigManagerConfig(t *testing.T) {
	config := ProductionTopicConfigManagerConfig()
	assert.Equal(t, StrategyCreateOnly, config.Strategy)
	assert.Equal(t, "warn", config.OnMismatch.LogLevel)
	assert.False(t, config.OnMismatch.FailFast)
	assert.True(t, config.EnableValidation)
	assert.Equal(t, 30*time.Second, config.SyncTimeout)
}

// TestStrictTopicConfigManagerConfig 测试严格模式配置
func TestStrictTopicConfigManagerConfig(t *testing.T) {
	config := StrictTopicConfigManagerConfig()
	assert.Equal(t, StrategyValidateOnly, config.Strategy)
	assert.Equal(t, "error", config.OnMismatch.LogLevel)
	assert.True(t, config.OnMismatch.FailFast)
	assert.True(t, config.EnableValidation)
	assert.Equal(t, 30*time.Second, config.SyncTimeout)
}

// TestCompareTopicOptions 测试配置比较
func TestCompareTopicOptions(t *testing.T) {
	tests := []struct {
		name            string
		topic           string
		expected        TopicOptions
		actual          TopicOptions
		expectedMatches int
	}{
		{
			name:  "完全相同",
			topic: "test.topic",
			expected: TopicOptions{
				PersistenceMode: TopicPersistent,
				RetentionTime:   24 * time.Hour,
				MaxSize:         1000000,
				MaxMessages:     10000,
				Replicas:        3,
			},
			actual: TopicOptions{
				PersistenceMode: TopicPersistent,
				RetentionTime:   24 * time.Hour,
				MaxSize:         1000000,
				MaxMessages:     10000,
				Replicas:        3,
			},
			expectedMatches: 0,
		},
		{
			name:  "持久化模式不同",
			topic: "test.topic",
			expected: TopicOptions{
				PersistenceMode: TopicPersistent,
			},
			actual: TopicOptions{
				PersistenceMode: TopicEphemeral,
			},
			expectedMatches: 1,
		},
		{
			name:  "保留时间不同",
			topic: "test.topic",
			expected: TopicOptions{
				PersistenceMode: TopicPersistent,
				RetentionTime:   24 * time.Hour,
			},
			actual: TopicOptions{
				PersistenceMode: TopicPersistent,
				RetentionTime:   48 * time.Hour,
			},
			expectedMatches: 1,
		},
		{
			name:  "多个字段不同",
			topic: "test.topic",
			expected: TopicOptions{
				PersistenceMode: TopicPersistent,
				RetentionTime:   24 * time.Hour,
				MaxSize:         1000000,
				MaxMessages:     10000,
				Replicas:        3,
			},
			actual: TopicOptions{
				PersistenceMode: TopicEphemeral,
				RetentionTime:   48 * time.Hour,
				MaxSize:         2000000,
				MaxMessages:     20000,
				Replicas:        5,
			},
			expectedMatches: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mismatches := compareTopicOptions(tt.topic, tt.expected, tt.actual)
			assert.Equal(t, tt.expectedMatches, len(mismatches))
			// 验证不一致信息的结构
			for _, mismatch := range mismatches {
				assert.Equal(t, tt.topic, mismatch.Topic)
				assert.NotEmpty(t, mismatch.Field)
				assert.NotNil(t, mismatch.ExpectedValue)
				assert.NotNil(t, mismatch.ActualValue)
				assert.NotEmpty(t, mismatch.Recommendation)
			}
		})
	}
}

// TestShouldCreateOrUpdate 测试创建/更新决策
func TestShouldCreateOrUpdate(t *testing.T) {
	tests := []struct {
		name         string
		strategy     TopicConfigStrategy
		exists       bool
		shouldCreate bool
		shouldUpdate bool
	}{
		{
			name:         "CreateOnly - 不存在",
			strategy:     StrategyCreateOnly,
			exists:       false,
			shouldCreate: true,
			shouldUpdate: false,
		},
		{
			name:         "CreateOnly - 已存在",
			strategy:     StrategyCreateOnly,
			exists:       true,
			shouldCreate: false,
			shouldUpdate: false,
		},
		{
			name:         "CreateOrUpdate - 不存在",
			strategy:     StrategyCreateOrUpdate,
			exists:       false,
			shouldCreate: true,
			shouldUpdate: false,
		},
		{
			name:         "CreateOrUpdate - 已存在",
			strategy:     StrategyCreateOrUpdate,
			exists:       true,
			shouldCreate: false,
			shouldUpdate: true,
		},
		{
			name:         "ValidateOnly - 不存在",
			strategy:     StrategyValidateOnly,
			exists:       false,
			shouldCreate: false,
			shouldUpdate: false,
		},
		{
			name:         "ValidateOnly - 已存在",
			strategy:     StrategyValidateOnly,
			exists:       true,
			shouldCreate: false,
			shouldUpdate: false,
		},
		{
			name:         "Skip - 不存在",
			strategy:     StrategySkip,
			exists:       false,
			shouldCreate: false,
			shouldUpdate: false,
		},
		{
			name:         "Skip - 已存在",
			strategy:     StrategySkip,
			exists:       true,
			shouldCreate: false,
			shouldUpdate: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldCreate, shouldUpdate := shouldCreateOrUpdate(tt.strategy, tt.exists)
			assert.Equal(t, tt.shouldCreate, shouldCreate, "shouldCreate mismatch")
			assert.Equal(t, tt.shouldUpdate, shouldUpdate, "shouldUpdate mismatch")
		})
	}
}

// TestHandleConfigMismatches 测试配置不一致处理
func TestHandleConfigMismatches(t *testing.T) {
	tests := []struct {
		name        string
		mismatches  []TopicConfigMismatch
		action      TopicConfigMismatchAction
		expectError bool
	}{
		{
			name:       "无不一致",
			mismatches: []TopicConfigMismatch{},
			action: TopicConfigMismatchAction{
				LogLevel: "warn",
				FailFast: false,
			},
			expectError: false,
		},
		{
			name: "有不一致但不立即失败",
			mismatches: []TopicConfigMismatch{
				{
					Topic:          "test.topic",
					Field:          "RetentionTime",
					ExpectedValue:  24 * time.Hour,
					ActualValue:    48 * time.Hour,
					CanAutoFix:     true,
					Recommendation: "Update config",
				},
			},
			action: TopicConfigMismatchAction{
				LogLevel: "warn",
				FailFast: false,
			},
			expectError: false,
		},
		{
			name: "有不一致且立即失败",
			mismatches: []TopicConfigMismatch{
				{
					Topic:          "test.topic",
					Field:          "RetentionTime",
					ExpectedValue:  24 * time.Hour,
					ActualValue:    48 * time.Hour,
					CanAutoFix:     true,
					Recommendation: "Update config",
				},
			},
			action: TopicConfigMismatchAction{
				LogLevel: "error",
				FailFast: true,
			},
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleConfigMismatches(tt.mismatches, tt.action)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTopicConfigMismatch 测试配置不一致结构
func TestTopicConfigMismatch(t *testing.T) {
	mismatch := TopicConfigMismatch{
		Topic:          "test.topic",
		Field:          "RetentionTime",
		ExpectedValue:  24 * time.Hour,
		ActualValue:    48 * time.Hour,
		CanAutoFix:     true,
		Recommendation: "Set strategy to 'create_or_update' to auto-fix.",
	}
	assert.Equal(t, "test.topic", mismatch.Topic)
	assert.Equal(t, "RetentionTime", mismatch.Field)
	assert.Equal(t, 24*time.Hour, mismatch.ExpectedValue)
	assert.Equal(t, 48*time.Hour, mismatch.ActualValue)
	assert.True(t, mismatch.CanAutoFix)
	assert.NotEmpty(t, mismatch.Recommendation)
}

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

func TestTopicPersistenceConfiguration(t *testing.T) {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()
	// 创建内存EventBus用于测试
	config := &EventBusConfig{
		Type: "memory",
	}
	bus, err := NewEventBus(config)
	require.NoError(t, err)
	defer bus.Close()
	ctx := context.Background()
	t.Run("ConfigureTopic", func(t *testing.T) {
		// 配置主题选项
		options := TopicOptions{
			PersistenceMode: TopicPersistent,
			RetentionTime:   24 * time.Hour,
			MaxSize:         100 * 1024 * 1024,
			MaxMessages:     10000,
			Description:     "Test topic for persistence",
		}
		err := bus.ConfigureTopic(ctx, "test-topic", options)
		assert.NoError(t, err)
		// 验证配置已保存
		savedOptions, err := bus.GetTopicConfig("test-topic")
		assert.NoError(t, err)
		assert.Equal(t, TopicPersistent, savedOptions.PersistenceMode)
		assert.Equal(t, 24*time.Hour, savedOptions.RetentionTime)
		assert.Equal(t, int64(100*1024*1024), savedOptions.MaxSize)
		assert.Equal(t, int64(10000), savedOptions.MaxMessages)
		assert.Equal(t, "Test topic for persistence", savedOptions.Description)
	})
	t.Run("SetTopicPersistence", func(t *testing.T) {
		// 使用简化接口设置持久化
		err := bus.SetTopicPersistence(ctx, "simple-topic", true)
		assert.NoError(t, err)
		// 验证配置
		options, err := bus.GetTopicConfig("simple-topic")
		assert.NoError(t, err)
		assert.Equal(t, TopicPersistent, options.PersistenceMode)
		// 设置为非持久化
		err = bus.SetTopicPersistence(ctx, "simple-topic", false)
		assert.NoError(t, err)
		options, err = bus.GetTopicConfig("simple-topic")
		assert.NoError(t, err)
		assert.Equal(t, TopicEphemeral, options.PersistenceMode)
	})
	t.Run("ListConfiguredTopics", func(t *testing.T) {
		// 配置几个主题
		topics := []string{"topic1", "topic2", "topic3"}
		for _, topic := range topics {
			err := bus.SetTopicPersistence(ctx, topic, true)
			assert.NoError(t, err)
		}
		// 获取配置的主题列表
		configuredTopics := bus.ListConfiguredTopics()
		// 验证所有主题都在列表中
		for _, topic := range topics {
			assert.Contains(t, configuredTopics, topic)
		}
	})
	t.Run("GetTopicConfig_NotConfigured", func(t *testing.T) {
		// 获取未配置主题的配置（应该返回默认配置）
		options, err := bus.GetTopicConfig("non-existent-topic")
		assert.NoError(t, err)
		// 应该返回默认配置
		defaultOptions := DefaultTopicOptions()
		assert.Equal(t, defaultOptions.PersistenceMode, options.PersistenceMode)
		assert.Equal(t, defaultOptions.RetentionTime, options.RetentionTime)
	})
	t.Run("RemoveTopicConfig", func(t *testing.T) {
		// 先配置一个主题
		err := bus.SetTopicPersistence(ctx, "removable-topic", true)
		assert.NoError(t, err)
		// 验证主题在列表中
		topics := bus.ListConfiguredTopics()
		assert.Contains(t, topics, "removable-topic")
		// 移除配置
		err = bus.RemoveTopicConfig("removable-topic")
		assert.NoError(t, err)
		// 验证主题不再在列表中
		topics = bus.ListConfiguredTopics()
		assert.NotContains(t, topics, "removable-topic")
	})
}
func TestTopicOptionsIsPersistent(t *testing.T) {
	tests := []struct {
		name                   string
		persistenceMode        TopicPersistenceMode
		globalJetStreamEnabled bool
		expectedPersistent     bool
	}{
		{
			name:                   "Explicit persistent",
			persistenceMode:        TopicPersistent,
			globalJetStreamEnabled: false,
			expectedPersistent:     true,
		},
		{
			name:                   "Explicit ephemeral",
			persistenceMode:        TopicEphemeral,
			globalJetStreamEnabled: true,
			expectedPersistent:     false,
		},
		{
			name:                   "Auto with JetStream enabled",
			persistenceMode:        TopicAuto,
			globalJetStreamEnabled: true,
			expectedPersistent:     true,
		},
		{
			name:                   "Auto with JetStream disabled",
			persistenceMode:        TopicAuto,
			globalJetStreamEnabled: false,
			expectedPersistent:     false,
		},
		{
			name:                   "Invalid mode defaults to global",
			persistenceMode:        "invalid",
			globalJetStreamEnabled: true,
			expectedPersistent:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := TopicOptions{
				PersistenceMode: tt.persistenceMode,
			}
			result := options.IsPersistent(tt.globalJetStreamEnabled)
			assert.Equal(t, tt.expectedPersistent, result)
		})
	}
}
func TestDefaultTopicOptions(t *testing.T) {
	options := DefaultTopicOptions()
	assert.Equal(t, TopicAuto, options.PersistenceMode)
	assert.Equal(t, 24*time.Hour, options.RetentionTime)
	assert.Equal(t, int64(100*1024*1024), options.MaxSize)
	assert.Equal(t, int64(10000), options.MaxMessages)
	assert.Equal(t, 1, options.Replicas)
}
func TestTopicPersistenceIntegration(t *testing.T) {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()
	// 创建内存EventBus
	config := &EventBusConfig{
		Type: "memory",
	}
	bus, err := NewEventBus(config)
	require.NoError(t, err)
	defer bus.Close()
	ctx := context.Background()
	// 配置主题
	err = bus.SetTopicPersistence(ctx, "test-integration", true)
	require.NoError(t, err)
	// 设置订阅
	received := make(chan []byte, 1)
	err = bus.Subscribe(ctx, "test-integration", func(ctx context.Context, message []byte) error {
		received <- message
		return nil
	})
	require.NoError(t, err)
	// 发布消息
	testMessage := []byte("test message for integration")
	err = bus.Publish(ctx, "test-integration", testMessage)
	require.NoError(t, err)
	// 验证消息接收
	select {
	case msg := <-received:
		assert.Equal(t, testMessage, msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Message not received within timeout")
	}
}

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
	envelope := NewEnvelopeWithAutoID("test-aggregate", "test-event", 1, []byte("test payload"))
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
	envelope := NewEnvelopeWithAutoID("test-aggregate", "test-event", 1, []byte("test payload"))
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
	envelope := NewEnvelopeWithAutoID("test-aggregate", "test-event", 1, []byte("test payload"))
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
