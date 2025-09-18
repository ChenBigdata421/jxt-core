package eventbus

import (
	"context"
	"fmt"
	"sync"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// kafkaAdvancedEventBus Kafka高级事件总线实现
type kafkaAdvancedEventBus struct {
	*kafkaEventBus // 继承基础实现

	// 高级组件
	advancedPublisher  *AdvancedPublisher
	advancedSubscriber *AdvancedSubscriber
	healthChecker      *HealthChecker
	backlogDetector    *BacklogDetector
	recoveryManager    *RecoveryManager
	aggregateManager   *AggregateProcessorManager

	// 配置
	advancedConfig config.AdvancedEventBusConfig

	// 业务组件（可插拔）
	messageRouter    MessageRouter
	errorHandler     ErrorHandler
	messageFormatter MessageFormatter

	// 状态管理
	isStarted bool
	mu        sync.RWMutex
}

// NewKafkaAdvancedEventBus 创建Kafka高级事件总线
func NewKafkaAdvancedEventBus(config config.AdvancedEventBusConfig) (AdvancedEventBus, error) {
	// 创建基础 Kafka EventBus
	baseEventBus, err := NewKafkaEventBus(&config.Kafka)
	if err != nil {
		return nil, fmt.Errorf("failed to create base kafka eventbus: %w", err)
	}

	kafkaBase, ok := baseEventBus.(*kafkaEventBus)
	if !ok {
		return nil, fmt.Errorf("unexpected eventbus type")
	}

	advanced := &kafkaAdvancedEventBus{
		kafkaEventBus:  kafkaBase,
		advancedConfig: config,
	}

	// 初始化高级组件
	if err := advanced.initAdvancedComponents(); err != nil {
		return nil, fmt.Errorf("failed to initialize advanced components: %w", err)
	}

	logger.Info("Kafka advanced event bus created",
		"serviceName", config.ServiceName,
		"healthCheckEnabled", config.HealthCheck.Enabled)

	return advanced, nil
}

// initAdvancedComponents 初始化高级组件
func (k *kafkaAdvancedEventBus) initAdvancedComponents() error {
	// 初始化聚合处理器管理器
	if k.advancedConfig.Subscriber.AggregateProcessor.Enabled {
		// 转换配置类型
		rateLimitConfig := RateLimitConfig{
			Enabled:       k.advancedConfig.Subscriber.RateLimit.Enabled,
			RatePerSecond: k.advancedConfig.Subscriber.RateLimit.RatePerSecond,
			BurstSize:     k.advancedConfig.Subscriber.RateLimit.BurstSize,
		}
		rateLimiter := NewRateLimiter(rateLimitConfig)
		var err error
		k.aggregateManager, err = NewAggregateProcessorManager(
			k.advancedConfig.Subscriber.AggregateProcessor.CacheSize,
			k.advancedConfig.Subscriber.AggregateProcessor.IdleTimeout,
			rateLimiter,
			nil, // handler will be set later
		)
		if err != nil {
			return fmt.Errorf("failed to create aggregate processor manager: %w", err)
		}
	}

	// 初始化积压检测器
	if k.advancedConfig.Subscriber.BacklogDetection.Enabled {
		// 这里需要从 kafkaEventBus 获取 client 和 admin
		// 暂时跳过，需要扩展 kafkaEventBus 接口
		// k.backlogDetector = NewBacklogDetector(client, admin, consumerGroup, k.advancedConfig.Subscriber.BacklogDetection)
	}

	// 初始化恢复模式管理器
	if k.advancedConfig.Subscriber.RecoveryMode.Enabled {
		k.recoveryManager = NewRecoveryManager(
			k.advancedConfig.Subscriber.RecoveryMode,
			k.backlogDetector,
			k.aggregateManager,
		)
	}

	// 初始化高级发布器
	k.advancedPublisher = NewAdvancedPublisher(k.advancedConfig.Publisher, k.kafkaEventBus)

	// 初始化高级订阅器
	k.advancedSubscriber = NewAdvancedSubscriber(k.advancedConfig.Subscriber, k.kafkaEventBus)

	// 设置组件引用
	if k.backlogDetector != nil {
		k.advancedSubscriber.SetBacklogDetector(k.backlogDetector)
	}
	if k.recoveryManager != nil {
		k.advancedSubscriber.SetRecoveryManager(k.recoveryManager)
	}
	if k.aggregateManager != nil {
		k.advancedSubscriber.SetAggregateManager(k.aggregateManager)
	}

	// 初始化健康检查器
	if k.advancedConfig.HealthCheck.Enabled {
		k.healthChecker = NewHealthChecker(
			k.advancedConfig.HealthCheck,
			k.kafkaEventBus,
			k.advancedConfig.ServiceName,
			"kafka",
		)
	}

	// 设置默认组件
	k.messageFormatter = &DefaultMessageFormatter{}

	return nil
}

// Start 启动高级事件总线
func (k *kafkaAdvancedEventBus) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isStarted {
		return nil
	}

	// 启动高级发布器
	if err := k.advancedPublisher.Start(ctx); err != nil {
		return fmt.Errorf("failed to start advanced publisher: %w", err)
	}

	// 启动高级订阅器
	if err := k.advancedSubscriber.Start(ctx); err != nil {
		return fmt.Errorf("failed to start advanced subscriber: %w", err)
	}

	// 启动健康检查器
	if k.healthChecker != nil {
		if err := k.healthChecker.Start(ctx); err != nil {
			return fmt.Errorf("failed to start health checker: %w", err)
		}
	}

	// 启动积压检测器
	if k.backlogDetector != nil {
		if err := k.backlogDetector.Start(ctx); err != nil {
			return fmt.Errorf("failed to start backlog detector: %w", err)
		}
	}

	// 启动恢复模式管理器
	if k.recoveryManager != nil {
		if err := k.recoveryManager.Start(ctx); err != nil {
			return fmt.Errorf("failed to start recovery manager: %w", err)
		}
	}

	k.isStarted = true
	logger.Info("Kafka advanced event bus started")

	return nil
}

// Stop 停止高级事件总线
func (k *kafkaAdvancedEventBus) Stop() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.isStarted {
		return nil
	}

	var errors []error

	// 停止恢复模式管理器
	if k.recoveryManager != nil {
		if err := k.recoveryManager.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop recovery manager: %w", err))
		}
	}

	// 停止积压检测器
	if k.backlogDetector != nil {
		if err := k.backlogDetector.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop backlog detector: %w", err))
		}
	}

	// 停止健康检查器
	if k.healthChecker != nil {
		if err := k.healthChecker.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop health checker: %w", err))
		}
	}

	// 停止高级发布器
	if err := k.advancedPublisher.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop advanced publisher: %w", err))
	}

	// 停止高级订阅器
	if err := k.advancedSubscriber.Stop(); err != nil {
		errors = append(errors, fmt.Errorf("failed to stop advanced subscriber: %w", err))
	}

	// 关闭聚合处理器管理器
	if k.aggregateManager != nil {
		k.aggregateManager.Stop() // 使用 Stop 方法而不是 Close
	}

	// 关闭基础事件总线
	if err := k.kafkaEventBus.Close(); err != nil {
		errors = append(errors, fmt.Errorf("failed to close base eventbus: %w", err))
	}

	k.isStarted = false

	if len(errors) > 0 {
		return fmt.Errorf("errors during stop: %v", errors)
	}

	logger.Info("Kafka advanced event bus stopped")
	return nil
}

// SubscribeWithOptions 使用选项订阅
func (k *kafkaAdvancedEventBus) SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error {
	// 如果使用聚合处理器，包装处理器
	if opts.UseAggregateProcessor && k.aggregateManager != nil {
		handler = k.wrapWithAggregateProcessor(handler, opts)
	}

	// 如果设置了流量控制，包装处理器
	if opts.RateLimit > 0 {
		handler = k.wrapWithRateLimit(handler, opts)
	}

	// 包装错误处理
	handler = k.wrapWithErrorHandler(handler, topic, opts)

	// 使用高级订阅器进行订阅（包含统计和监控）
	return k.advancedSubscriber.Subscribe(ctx, topic, handler, opts)
}

// PublishWithOptions 使用选项发布
func (k *kafkaAdvancedEventBus) PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error {
	return k.advancedPublisher.PublishWithOptions(ctx, topic, message, opts)
}

// SetRecoveryMode 设置恢复模式
func (k *kafkaAdvancedEventBus) SetRecoveryMode(enabled bool) error {
	if k.recoveryManager == nil {
		return fmt.Errorf("recovery manager not initialized")
	}

	mode := RecoveryModeNormal
	if enabled {
		mode = RecoveryModeActive
	}

	return k.recoveryManager.SetRecoveryMode(mode, "manual")
}

// IsInRecoveryMode 检查是否在恢复模式
func (k *kafkaAdvancedEventBus) IsInRecoveryMode() bool {
	if k.recoveryManager == nil {
		return false
	}
	return k.recoveryManager.IsInRecoveryMode()
}

// RegisterBacklogCallback 注册积压回调
func (k *kafkaAdvancedEventBus) RegisterBacklogCallback(callback BacklogStateCallback) error {
	if k.backlogDetector == nil {
		return fmt.Errorf("backlog detector not initialized")
	}
	return k.backlogDetector.RegisterCallback(callback)
}

// StartBacklogMonitoring 启动积压监控
func (k *kafkaAdvancedEventBus) StartBacklogMonitoring(ctx context.Context) error {
	if k.backlogDetector == nil {
		return fmt.Errorf("backlog detector not initialized")
	}
	return k.backlogDetector.Start(ctx)
}

// StopBacklogMonitoring 停止积压监控
func (k *kafkaAdvancedEventBus) StopBacklogMonitoring() error {
	if k.backlogDetector == nil {
		return fmt.Errorf("backlog detector not initialized")
	}
	return k.backlogDetector.Stop()
}

// SetMessageRouter 设置消息路由器
func (k *kafkaAdvancedEventBus) SetMessageRouter(router MessageRouter) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.messageRouter = router
	return nil
}

// SetErrorHandler 设置错误处理器
func (k *kafkaAdvancedEventBus) SetErrorHandler(handler ErrorHandler) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.errorHandler = handler
	return nil
}

// SetMessageFormatter 设置消息格式化器
func (k *kafkaAdvancedEventBus) SetMessageFormatter(formatter MessageFormatter) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.messageFormatter = formatter
	return k.advancedPublisher.SetMessageFormatter(formatter)
}

// RegisterPublishCallback 注册发布回调
func (k *kafkaAdvancedEventBus) RegisterPublishCallback(callback PublishCallback) error {
	return k.advancedPublisher.RegisterPublishCallback(callback)
}

// StartHealthCheck 启动健康检查
func (k *kafkaAdvancedEventBus) StartHealthCheck(ctx context.Context) error {
	if k.healthChecker == nil {
		return fmt.Errorf("health checker not initialized")
	}
	return k.healthChecker.Start(ctx)
}

// StopHealthCheck 停止健康检查
func (k *kafkaAdvancedEventBus) StopHealthCheck() error {
	if k.healthChecker == nil {
		return fmt.Errorf("health checker not initialized")
	}
	return k.healthChecker.Stop()
}

// GetHealthStatus 获取健康状态
func (k *kafkaAdvancedEventBus) GetHealthStatus() HealthCheckStatus {
	if k.healthChecker == nil {
		return HealthCheckStatus{
			IsHealthy:    false,
			EventBusType: "kafka",
			Source:       k.advancedConfig.ServiceName,
		}
	}
	return k.healthChecker.GetStatus()
}

// RegisterHealthCheckCallback 注册健康检查回调
func (k *kafkaAdvancedEventBus) RegisterHealthCheckCallback(callback HealthCheckCallback) error {
	if k.healthChecker == nil {
		return fmt.Errorf("health checker not initialized")
	}
	return k.healthChecker.RegisterCallback(callback)
}

// GetConnectionState 获取连接状态
func (k *kafkaAdvancedEventBus) GetConnectionState() ConnectionState {
	return k.advancedPublisher.GetConnectionState()
}

// GetAdvancedSubscriber 获取高级订阅器
func (k *kafkaAdvancedEventBus) GetAdvancedSubscriber() *AdvancedSubscriber {
	return k.advancedSubscriber
}

// RegisterSubscriptionCallback 注册订阅回调
func (k *kafkaAdvancedEventBus) RegisterSubscriptionCallback(callback SubscriptionCallback) error {
	return k.advancedSubscriber.RegisterSubscriptionCallback(callback)
}

// 辅助方法

// wrapWithAggregateProcessor 使用聚合处理器包装处理器
func (k *kafkaAdvancedEventBus) wrapWithAggregateProcessor(handler MessageHandler, opts SubscribeOptions) MessageHandler {
	return func(ctx context.Context, message []byte) error {
		// 提取聚合ID
		aggregateID := k.extractAggregateID(message)
		if aggregateID == "" {
			// 如果没有聚合ID，直接处理
			return handler(ctx, message)
		}

		// 获取或创建聚合处理器
		processor, err := k.aggregateManager.GetOrCreateProcessor(aggregateID)
		if err != nil {
			// 聚合处理器创建失败，直接处理
			return handler(ctx, message)
		}
		if processor == nil {
			// 聚合处理器未启用，直接处理
			return handler(ctx, message)
		}

		// 使用聚合处理器处理消息
		// 这里需要将 []byte 转换为 *AggregateMessage
		// 简化实现，直接调用处理器
		return handler(ctx, message)
	}
}

// wrapWithRateLimit 使用流量控制包装处理器
func (k *kafkaAdvancedEventBus) wrapWithRateLimit(handler MessageHandler, opts SubscribeOptions) MessageHandler {
	rateLimitConfig := RateLimitConfig{
		Enabled:       true,
		RatePerSecond: opts.RateLimit,
		BurstSize:     opts.RateBurst,
	}
	rateLimiter := NewRateLimiter(rateLimitConfig)

	return func(ctx context.Context, message []byte) error {
		if err := rateLimiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit error: %w", err)
		}
		return handler(ctx, message)
	}
}

// wrapWithErrorHandler 使用错误处理器包装处理器
func (k *kafkaAdvancedEventBus) wrapWithErrorHandler(handler MessageHandler, topic string, opts SubscribeOptions) MessageHandler {
	return func(ctx context.Context, message []byte) error {
		err := handler(ctx, message)
		if err != nil && k.errorHandler != nil {
			action := k.errorHandler.HandleError(ctx, err, message, topic)
			// 根据错误处理动作决定如何处理
			switch action.Action {
			case ErrorActionSkip:
				return nil // 跳过错误
			case ErrorActionRetry:
				// 这里可以实现重试逻辑
				return err
			case ErrorActionDeadLetter:
				// 发送到死信队列
				return err
			default:
				return err
			}
		}
		return err
	}
}

// extractAggregateID 提取聚合ID
func (k *kafkaAdvancedEventBus) extractAggregateID(message []byte) string {
	// 这里应该根据消息格式提取聚合ID
	// 简化实现，返回空字符串
	return ""
}
