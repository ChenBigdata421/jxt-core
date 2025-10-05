package eventbus

import (
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
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
