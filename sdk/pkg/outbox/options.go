package outbox

import (
	"time"
)

// SchedulerOption 调度器选项
type SchedulerOption func(*schedulerOptions)

// schedulerOptions 调度器选项结构
type schedulerOptions struct {
	repo            OutboxRepository
	eventPublisher  EventPublisher
	topicMapper     TopicMapper
	schedulerConfig *SchedulerConfig
	publisherConfig *PublisherConfig
}

// WithRepository 设置仓储
func WithRepository(repo OutboxRepository) SchedulerOption {
	return func(opts *schedulerOptions) {
		opts.repo = repo
	}
}

// WithEventPublisher 设置事件发布器
func WithEventPublisher(eventPublisher EventPublisher) SchedulerOption {
	return func(opts *schedulerOptions) {
		opts.eventPublisher = eventPublisher
	}
}

// WithTopicMapper 设置 Topic 映射器
func WithTopicMapper(topicMapper TopicMapper) SchedulerOption {
	return func(opts *schedulerOptions) {
		opts.topicMapper = topicMapper
	}
}

// WithSchedulerConfig 设置调度器配置
func WithSchedulerConfig(config *SchedulerConfig) SchedulerOption {
	return func(opts *schedulerOptions) {
		opts.schedulerConfig = config
	}
}

// WithPublisherConfig 设置发布器配置
func WithPublisherConfig(config *PublisherConfig) SchedulerOption {
	return func(opts *schedulerOptions) {
		opts.publisherConfig = config
	}
}

// WithPollInterval 设置轮询间隔
func WithPollInterval(interval time.Duration) SchedulerOption {
	return func(opts *schedulerOptions) {
		if opts.schedulerConfig == nil {
			opts.schedulerConfig = DefaultSchedulerConfig()
		}
		opts.schedulerConfig.PollInterval = interval
	}
}

// WithBatchSize 设置批量大小
func WithBatchSize(size int) SchedulerOption {
	return func(opts *schedulerOptions) {
		if opts.schedulerConfig == nil {
			opts.schedulerConfig = DefaultSchedulerConfig()
		}
		opts.schedulerConfig.BatchSize = size
	}
}

// WithTenantID 设置租户 ID
func WithTenantID(tenantID string) SchedulerOption {
	return func(opts *schedulerOptions) {
		if opts.schedulerConfig == nil {
			opts.schedulerConfig = DefaultSchedulerConfig()
		}
		opts.schedulerConfig.TenantID = tenantID
	}
}

// WithCleanupEnabled 设置是否启用清理
func WithCleanupEnabled(enabled bool) SchedulerOption {
	return func(opts *schedulerOptions) {
		if opts.schedulerConfig == nil {
			opts.schedulerConfig = DefaultSchedulerConfig()
		}
		opts.schedulerConfig.EnableCleanup = enabled
	}
}

// WithHealthCheckEnabled 设置是否启用健康检查
func WithHealthCheckEnabled(enabled bool) SchedulerOption {
	return func(opts *schedulerOptions) {
		if opts.schedulerConfig == nil {
			opts.schedulerConfig = DefaultSchedulerConfig()
		}
		opts.schedulerConfig.EnableHealthCheck = enabled
	}
}

// WithMetricsEnabled 设置是否启用指标收集
func WithMetricsEnabled(enabled bool) SchedulerOption {
	return func(opts *schedulerOptions) {
		if opts.schedulerConfig == nil {
			opts.schedulerConfig = DefaultSchedulerConfig()
		}
		opts.schedulerConfig.EnableMetrics = enabled
	}
}
