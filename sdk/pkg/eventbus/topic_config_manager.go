package eventbus

import (
	"context"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// TopicConfigStrategy 主题配置策略
type TopicConfigStrategy string

const (
	// StrategyCreateOnly 创建模式：只创建，不更新（生产环境推荐）
	StrategyCreateOnly TopicConfigStrategy = "create_only"

	// StrategyCreateOrUpdate 更新模式：创建或更新（开发环境推荐）
	StrategyCreateOrUpdate TopicConfigStrategy = "create_or_update"

	// StrategyValidateOnly 验证模式：只验证，不修改（严格模式）
	StrategyValidateOnly TopicConfigStrategy = "validate_only"

	// StrategySkip 跳过模式：不检查（性能优先）
	StrategySkip TopicConfigStrategy = "skip"
)

// TopicConfigMismatchAction 配置不一致时的行为
type TopicConfigMismatchAction struct {
	LogLevel string // debug, info, warn, error
	FailFast bool   // 是否立即失败
}

// TopicConfigManagerConfig 主题配置管理器配置
type TopicConfigManagerConfig struct {
	// 配置策略
	Strategy TopicConfigStrategy

	// 配置不一致时的行为
	OnMismatch TopicConfigMismatchAction

	// 是否启用配置验证
	EnableValidation bool

	// 配置同步超时
	SyncTimeout time.Duration
}

// DefaultTopicConfigManagerConfig 返回默认配置
func DefaultTopicConfigManagerConfig() TopicConfigManagerConfig {
	return TopicConfigManagerConfig{
		Strategy: StrategyCreateOrUpdate,
		OnMismatch: TopicConfigMismatchAction{
			LogLevel: "warn",
			FailFast: false,
		},
		EnableValidation: true,
		SyncTimeout:      30 * time.Second,
	}
}

// ProductionTopicConfigManagerConfig 返回生产环境推荐配置
func ProductionTopicConfigManagerConfig() TopicConfigManagerConfig {
	return TopicConfigManagerConfig{
		Strategy: StrategyCreateOnly,
		OnMismatch: TopicConfigMismatchAction{
			LogLevel: "warn",
			FailFast: false,
		},
		EnableValidation: true,
		SyncTimeout:      30 * time.Second,
	}
}

// StrictTopicConfigManagerConfig 返回严格模式配置
func StrictTopicConfigManagerConfig() TopicConfigManagerConfig {
	return TopicConfigManagerConfig{
		Strategy: StrategyValidateOnly,
		OnMismatch: TopicConfigMismatchAction{
			LogLevel: "error",
			FailFast: true,
		},
		EnableValidation: true,
		SyncTimeout:      30 * time.Second,
	}
}

// TopicConfigMismatch 配置不一致信息
type TopicConfigMismatch struct {
	Topic          string
	Field          string
	ExpectedValue  interface{}
	ActualValue    interface{}
	CanAutoFix     bool
	Recommendation string
}

// TopicConfigSyncResult 配置同步结果
type TopicConfigSyncResult struct {
	Topic      string
	Action     string // created, updated, validated, skipped
	Success    bool
	Error      error
	Mismatches []TopicConfigMismatch
	Duration   time.Duration
}

// TopicConfigManager 主题配置管理器接口
type TopicConfigManager interface {
	// ConfigureTopic 配置主题（幂等操作）
	ConfigureTopic(ctx context.Context, topic string, options TopicOptions) (*TopicConfigSyncResult, error)

	// ValidateTopicConfig 验证主题配置
	ValidateTopicConfig(ctx context.Context, topic string, options TopicOptions) ([]TopicConfigMismatch, error)

	// GetTopicConfig 获取主题配置
	GetTopicConfig(topic string) (TopicOptions, error)

	// ListConfiguredTopics 列出所有已配置的主题
	ListConfiguredTopics() []string

	// RemoveTopicConfig 移除主题配置
	RemoveTopicConfig(topic string) error

	// SetStrategy 设置配置策略
	SetStrategy(strategy TopicConfigStrategy)

	// GetStrategy 获取当前策略
	GetStrategy() TopicConfigStrategy
}

// logConfigMismatch 记录配置不一致
func logConfigMismatch(mismatch TopicConfigMismatch, action TopicConfigMismatchAction) {
	msg := fmt.Sprintf("Topic config mismatch: topic=%s, field=%s, expected=%v, actual=%v",
		mismatch.Topic, mismatch.Field, mismatch.ExpectedValue, mismatch.ActualValue)

	switch action.LogLevel {
	case "debug":
		logger.Debug(msg)
	case "info":
		logger.Info(msg)
	case "warn":
		logger.Warn(msg, "recommendation", mismatch.Recommendation)
	case "error":
		logger.Error(msg, "recommendation", mismatch.Recommendation)
	default:
		logger.Warn(msg)
	}
}

// handleConfigMismatches 处理配置不一致
func handleConfigMismatches(mismatches []TopicConfigMismatch, action TopicConfigMismatchAction) error {
	if len(mismatches) == 0 {
		return nil
	}

	// 记录所有不一致
	for _, mismatch := range mismatches {
		logConfigMismatch(mismatch, action)
	}

	// 如果配置为立即失败，返回错误
	if action.FailFast {
		return fmt.Errorf("topic config validation failed: found %d mismatches", len(mismatches))
	}

	return nil
}

// compareTopicOptions 比较两个主题配置
func compareTopicOptions(topic string, expected, actual TopicOptions) []TopicConfigMismatch {
	var mismatches []TopicConfigMismatch

	// 比较持久化模式
	if expected.PersistenceMode != actual.PersistenceMode {
		mismatches = append(mismatches, TopicConfigMismatch{
			Topic:          topic,
			Field:          "PersistenceMode",
			ExpectedValue:  expected.PersistenceMode,
			ActualValue:    actual.PersistenceMode,
			CanAutoFix:     false,
			Recommendation: "Persistence mode cannot be changed after creation. Consider creating a new topic.",
		})
	}

	// 比较保留时间（允许一定误差）
	if expected.RetentionTime > 0 && actual.RetentionTime > 0 {
		diff := expected.RetentionTime - actual.RetentionTime
		if diff < 0 {
			diff = -diff
		}
		// 允许1秒的误差
		if diff > time.Second {
			mismatches = append(mismatches, TopicConfigMismatch{
				Topic:          topic,
				Field:          "RetentionTime",
				ExpectedValue:  expected.RetentionTime,
				ActualValue:    actual.RetentionTime,
				CanAutoFix:     true,
				Recommendation: "Retention time can be updated. Set strategy to 'create_or_update' to auto-fix.",
			})
		}
	}

	// 比较最大大小
	if expected.MaxSize > 0 && actual.MaxSize > 0 && expected.MaxSize != actual.MaxSize {
		mismatches = append(mismatches, TopicConfigMismatch{
			Topic:          topic,
			Field:          "MaxSize",
			ExpectedValue:  expected.MaxSize,
			ActualValue:    actual.MaxSize,
			CanAutoFix:     true,
			Recommendation: "Max size can be updated. Set strategy to 'create_or_update' to auto-fix.",
		})
	}

	// 比较最大消息数
	if expected.MaxMessages > 0 && actual.MaxMessages > 0 && expected.MaxMessages != actual.MaxMessages {
		mismatches = append(mismatches, TopicConfigMismatch{
			Topic:          topic,
			Field:          "MaxMessages",
			ExpectedValue:  expected.MaxMessages,
			ActualValue:    actual.MaxMessages,
			CanAutoFix:     true,
			Recommendation: "Max messages can be updated. Set strategy to 'create_or_update' to auto-fix.",
		})
	}

	// 比较副本数
	if expected.Replicas > 0 && actual.Replicas > 0 && expected.Replicas != actual.Replicas {
		mismatches = append(mismatches, TopicConfigMismatch{
			Topic:          topic,
			Field:          "Replicas",
			ExpectedValue:  expected.Replicas,
			ActualValue:    actual.Replicas,
			CanAutoFix:     false,
			Recommendation: "Replicas cannot be changed after creation. Consider creating a new topic.",
		})
	}

	return mismatches
}

// shouldCreateOrUpdate 判断是否应该创建或更新
func shouldCreateOrUpdate(strategy TopicConfigStrategy, exists bool) (shouldCreate, shouldUpdate bool) {
	switch strategy {
	case StrategyCreateOnly:
		return !exists, false
	case StrategyCreateOrUpdate:
		return !exists, exists
	case StrategyValidateOnly:
		return false, false
	case StrategySkip:
		return false, false
	default:
		return !exists, exists
	}
}

// formatSyncResult 格式化同步结果日志
func formatSyncResult(result *TopicConfigSyncResult) {
	if result.Success {
		logger.Info("Topic config sync completed",
			"topic", result.Topic,
			"action", result.Action,
			"duration", result.Duration)
	} else {
		logger.Error("Topic config sync failed",
			"topic", result.Topic,
			"action", result.Action,
			"error", result.Error,
			"duration", result.Duration)
	}

	// 记录配置不一致
	if len(result.Mismatches) > 0 {
		logger.Warn("Topic config mismatches detected",
			"topic", result.Topic,
			"count", len(result.Mismatches))
	}
}

