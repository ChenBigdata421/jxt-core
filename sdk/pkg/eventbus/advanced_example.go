package eventbus

import (
	"context"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// ExampleAdvancedEventBusUsage 高级事件总线使用示例
func ExampleAdvancedEventBusUsage() {
	// 1. 创建高级事件总线配置
	config := GetDefaultAdvancedEventBusConfig()
	config.ServiceName = "example-service"
	config.Type = "kafka"
	config.Kafka.Brokers = []string{"localhost:9092"}

	// 启用健康检查
	config.HealthCheck.Enabled = true
	config.HealthCheck.Interval = 1 * time.Minute

	// 启用积压检测
	config.Subscriber.BacklogDetection.Enabled = true
	config.Subscriber.BacklogDetection.MaxLagThreshold = 1000

	// 启用恢复模式
	config.Subscriber.RecoveryMode.Enabled = true
	config.Subscriber.RecoveryMode.AutoDetection = true

	// 启用聚合处理器
	config.Subscriber.AggregateProcessor.Enabled = true
	config.Subscriber.AggregateProcessor.CacheSize = 500

	// 2. 创建高级事件总线
	bus, err := CreateAdvancedEventBus(&config)
	if err != nil {
		logger.Error("Failed to create advanced event bus", "error", err)
		return
	}

	// 3. 设置业务组件
	bus.SetMessageRouter(&ExampleMessageRouter{})
	bus.SetErrorHandler(&ExampleErrorHandler{})
	bus.SetMessageFormatter(&ExampleMessageFormatter{})

	// 4. 注册回调
	bus.RegisterHealthCheckCallback(handleHealthCheck)
	bus.RegisterBacklogCallback(handleBacklogChange)
	bus.RegisterPublishCallback(handlePublishResult)
	bus.RegisterSubscriptionCallback(handleSubscriptionEvent)

	// 5. 启动事件总线
	ctx := context.Background()
	if err := bus.Start(ctx); err != nil {
		logger.Error("Failed to start advanced event bus", "error", err)
		return
	}

	// 6. 启动健康检查
	if err := bus.StartHealthCheck(ctx); err != nil {
		logger.Error("Failed to start health check", "error", err)
		return
	}

	// 7. 启动积压监控
	if err := bus.StartBacklogMonitoring(ctx); err != nil {
		logger.Error("Failed to start backlog monitoring", "error", err)
		return
	}

	// 8. 订阅消息（使用高级选项）
	subscribeOpts := SubscribeOptions{
		UseAggregateProcessor: true,
		ProcessingTimeout:     30 * time.Second,
		RateLimit:             100, // 每秒100条消息
		RateBurst:             200,
		MaxRetries:            3,
		RetryBackoff:          1 * time.Second,
	}

	err = bus.SubscribeWithOptions(ctx, "example-topic", handleMessage, subscribeOpts)
	if err != nil {
		logger.Error("Failed to subscribe", "error", err)
		return
	}

	// 9. 发布消息（使用高级选项）
	publishOpts := PublishOptions{
		AggregateID: "user-123",
		Metadata: map[string]string{
			"eventType": "UserCreated",
			"version":   "1.0",
		},
		Timeout: 10 * time.Second,
		RetryPolicy: RetryPolicy{
			MaxRetries:      3,
			InitialInterval: 1 * time.Second,
			MaxInterval:     10 * time.Second,
			Multiplier:      2.0,
		},
	}

	message := []byte(`{"userId": "123", "name": "John Doe", "email": "john@example.com"}`)
	err = bus.PublishWithOptions(ctx, "user-events", message, publishOpts)
	if err != nil {
		logger.Error("Failed to publish message", "error", err)
		return
	}

	// 10. 运行一段时间
	time.Sleep(30 * time.Second)

	// 获取订阅器统计信息
	subscriber := bus.GetAdvancedSubscriber()
	if subscriber != nil {
		stats := subscriber.GetStats()
		logger.Info("Subscriber statistics",
			"totalSubscriptions", stats.TotalSubscriptions,
			"activeSubscriptions", stats.ActiveSubscriptions,
			"messagesProcessed", stats.MessagesProcessed,
			"processingErrors", stats.ProcessingErrors,
			"uptime", stats.Uptime)

		// 获取所有订阅信息
		subscriptions := subscriber.GetAllSubscriptions()
		for topic, info := range subscriptions {
			logger.Info("Subscription info",
				"topic", topic,
				"messagesCount", info.MessagesCount,
				"errorsCount", info.ErrorsCount,
				"isActive", info.IsActive)
		}
	}

	// 11. 停止事件总线
	if err := bus.Stop(); err != nil {
		logger.Error("Failed to stop advanced event bus", "error", err)
	}

	logger.Info("Advanced event bus example completed")
}

// ExampleMessageRouter 示例消息路由器
type ExampleMessageRouter struct{}

func (r *ExampleMessageRouter) Route(ctx context.Context, topic string, message []byte) (RouteDecision, error) {
	// 简单的路由逻辑示例
	decision := RouteDecision{
		ShouldProcess: true,
		Priority:      1,
		Metadata:      make(map[string]string),
	}

	// 根据主题设置不同的处理策略
	switch topic {
	case "user-events":
		decision.AggregateID = "user"
		decision.ProcessorKey = "user-processor"
	case "order-events":
		decision.AggregateID = "order"
		decision.ProcessorKey = "order-processor"
		decision.Priority = 2 // 订单事件优先级更高
	default:
		decision.ProcessorKey = "default-processor"
	}

	return decision, nil
}

// ExampleErrorHandler 示例错误处理器
type ExampleErrorHandler struct{}

func (h *ExampleErrorHandler) HandleError(ctx context.Context, err error, message []byte, topic string) ErrorAction {
	logger.Error("Message processing error",
		"error", err,
		"topic", topic,
		"messageSize", len(message))

	// 根据错误类型决定处理策略
	if isRetryableError(err) {
		return ErrorAction{
			Action:     ErrorActionRetry,
			RetryAfter: 5 * time.Second,
		}
	}

	if isCriticalError(err) {
		return ErrorAction{
			Action:     ErrorActionDeadLetter,
			DeadLetter: true,
		}
	}

	// 默认跳过错误消息
	return ErrorAction{
		Action:      ErrorActionSkip,
		SkipMessage: true,
	}
}

// ExampleMessageFormatter 示例消息格式化器
type ExampleMessageFormatter struct {
	DefaultMessageFormatter
}

func (f *ExampleMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
	msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
	if err != nil {
		return nil, err
	}

	// 添加业务特定的元数据
	msg.Metadata["service"] = "example-service"
	msg.Metadata["version"] = "1.0.0"
	msg.Metadata["environment"] = "development"

	return msg, nil
}

// 回调函数示例

func handleHealthCheck(ctx context.Context, result HealthCheckResult) error {
	if result.Success {
		logger.Info("Health check succeeded",
			"source", result.Source,
			"duration", result.Duration)
	} else {
		logger.Error("Health check failed",
			"source", result.Source,
			"error", result.Error,
			"consecutiveFailures", result.ConsecutiveFailures)
	}
	return nil
}

func handleBacklogChange(ctx context.Context, state BacklogState) error {
	if state.HasBacklog {
		logger.Warn("Backlog detected",
			"topic", state.Topic,
			"lagCount", state.LagCount,
			"lagTime", state.LagTime)
	} else {
		logger.Info("Backlog cleared",
			"topic", state.Topic)
	}
	return nil
}

func handlePublishResult(ctx context.Context, topic string, message []byte, err error) error {
	if err != nil {
		logger.Error("Publish failed",
			"topic", topic,
			"error", err,
			"messageSize", len(message))
	} else {
		logger.Debug("Publish succeeded",
			"topic", topic,
			"messageSize", len(message))
	}
	return nil
}

func handleSubscriptionEvent(ctx context.Context, event SubscriptionEvent) error {
	switch event.Type {
	case SubscriptionEventStarted:
		logger.Info("Subscription started", "topic", event.Topic)
	case SubscriptionEventStopped:
		logger.Info("Subscription stopped", "topic", event.Topic)
	case SubscriptionEventError:
		logger.Error("Subscription error",
			"topic", event.Topic,
			"error", event.Error)
	case SubscriptionEventMessage:
		logger.Debug("Message processed", "topic", event.Topic)
	}
	return nil
}

func handleMessage(ctx context.Context, message []byte) error {
	logger.Info("Processing message", "size", len(message))

	// 模拟消息处理
	time.Sleep(100 * time.Millisecond)

	// 模拟偶尔的处理错误
	if len(message)%10 == 0 {
		return fmt.Errorf("simulated processing error")
	}

	return nil
}

// 辅助函数

func isRetryableError(err error) bool {
	// 判断是否为可重试的错误
	// 这里可以根据具体的错误类型进行判断
	return err.Error() != "permanent error"
}

func isCriticalError(err error) bool {
	// 判断是否为严重错误
	return err.Error() == "critical error"
}

// ExampleBusinessAdapter 业务适配器示例（类似 evidence-management 的适配器）
type ExampleBusinessAdapter struct {
	advancedBus AdvancedEventBus
}

func NewExampleBusinessAdapter(bus AdvancedEventBus) *ExampleBusinessAdapter {
	adapter := &ExampleBusinessAdapter{
		advancedBus: bus,
	}

	// 设置业务特定的组件
	bus.SetMessageFormatter(&ExampleMessageFormatter{})
	bus.RegisterHealthCheckCallback(adapter.handleHealthCheck)
	bus.RegisterPublishCallback(adapter.handlePublishResult)

	return adapter
}

func (a *ExampleBusinessAdapter) PublishBusinessEvent(ctx context.Context, eventType string, aggregateID interface{}, payload []byte) error {
	opts := PublishOptions{
		AggregateID: aggregateID,
		Metadata: map[string]string{
			"eventType": eventType,
			"source":    "example-service",
		},
		Timeout: 30 * time.Second,
	}

	return a.advancedBus.PublishWithOptions(ctx, "business-events", payload, opts)
}

func (a *ExampleBusinessAdapter) handleHealthCheck(ctx context.Context, result HealthCheckResult) error {
	// 业务特定的健康检查处理
	if !result.Success {
		// 发送告警、更新监控指标等
		logger.Error("Business health check failed", "error", result.Error)
	}
	return nil
}

func (a *ExampleBusinessAdapter) handlePublishResult(ctx context.Context, topic string, message []byte, err error) error {
	// 业务特定的发布结果处理
	if err != nil {
		// 记录业务日志、发送告警等
		logger.Error("Business publish failed", "topic", topic, "error", err)
	}
	return nil
}
