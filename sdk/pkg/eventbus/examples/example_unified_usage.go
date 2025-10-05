package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// UnifiedEventBusExample 展示如何使用统一的 EventBus 接口
func UnifiedEventBusExample() error {
	logger.Info("Starting unified EventBus example")

	// 1. 创建配置（可以从 YAML 文件加载）
	config := createExampleConfig()

	// 2. 创建统一的 EventBus 实例
	bus, err := NewEventBus(config)
	if err != nil {
		return fmt.Errorf("failed to create eventbus: %w", err)
	}
	defer bus.Close()

	// 3. 启动 EventBus（根据配置启用相应的企业特性）
	ctx := context.Background()
	if err := bus.Start(ctx); err != nil {
		return fmt.Errorf("failed to start eventbus: %w", err)
	}

	// 4. 根据配置启用企业特性
	if err := setupEnterpriseFeatures(bus, config); err != nil {
		return fmt.Errorf("failed to setup enterprise features: %w", err)
	}

	// 5. 基础发布和订阅
	if err := demonstrateBasicUsage(ctx, bus); err != nil {
		return fmt.Errorf("basic usage failed: %w", err)
	}

	// 6. 高级发布和订阅（如果启用）
	if err := demonstrateAdvancedUsage(ctx, bus, config); err != nil {
		return fmt.Errorf("advanced usage failed: %w", err)
	}

	// 7. 监控和健康检查
	if err := demonstrateMonitoring(ctx, bus, config); err != nil {
		return fmt.Errorf("monitoring failed: %w", err)
	}

	// 8. 优雅停止
	if err := bus.Stop(); err != nil {
		return fmt.Errorf("failed to stop eventbus: %w", err)
	}

	logger.Info("Unified EventBus example completed successfully")
	return nil
}

// createExampleConfig 创建示例配置
func createExampleConfig() *EventBusConfig {
	return &EventBusConfig{
		Type: "memory", // 使用内存实现进行演示
		Enterprise: EnterpriseConfig{
			Publisher: PublisherEnterpriseConfig{
				MessageFormatter: MessageFormatterConfig{
					Enabled: true,
					Type:    "json",
				},
				PublishCallback: PublishCallbackConfig{
					Enabled: true,
				},
				RetryPolicy: RetryPolicyConfig{
					Enabled:         true,
					MaxRetries:      3,
					InitialInterval: 1 * time.Second,
					MaxInterval:     30 * time.Second,
					Multiplier:      2.0,
				},
			},
			Subscriber: SubscriberEnterpriseConfig{
				DeadLetter: DeadLetterConfig{
					Enabled:    true,
					Topic:      "dead-letter-queue",
					MaxRetries: 3,
				},
			},
			HealthCheck: HealthCheckConfig{
				Enabled:  true,
				Interval: 30 * time.Second,
				Timeout:  10 * time.Second,
			},
			Monitoring: MonitoringConfig{
				Enabled:         true,
				MetricsInterval: 60 * time.Second,
				ExportEndpoint:  "http://localhost:8080/metrics",
			},
		},
	}
}

// setupEnterpriseFeatures 根据配置设置企业特性
func setupEnterpriseFeatures(bus EventBus, config *EventBusConfig) error {
	// 设置发布端企业特性
	if config.Enterprise.Publisher.PublishCallback.Enabled {
		callback := func(ctx context.Context, topic string, message []byte, err error) error {
			if err != nil {
				logger.Error("Publish failed", "topic", topic, "error", err)
			} else {
				logger.Debug("Message published", "topic", topic, "size", len(message))
			}
			return nil
		}
		if err := bus.RegisterPublishCallback(callback); err != nil {
			return fmt.Errorf("failed to register publish callback: %w", err)
		}
	}

	// 设置健康检查
	if config.Enterprise.HealthCheck.Enabled {
		if err := bus.StartHealthCheck(context.Background()); err != nil {
			return fmt.Errorf("failed to start health check: %w", err)
		}

		// 注册健康检查回调
		healthCallback := func(ctx context.Context, result HealthCheckResult) error {
			if result.Success {
				logger.Debug("Health check passed", "duration", result.Duration)
			} else {
				logger.Warn("Health check failed", "error", result.Error, "failures", result.ConsecutiveFailures)
			}
			return nil
		}
		if err := bus.RegisterHealthCheckCallback(healthCallback); err != nil {
			return fmt.Errorf("failed to register health check callback: %w", err)
		}
	}

	return nil
}

// demonstrateBasicUsage 演示基础用法
func demonstrateBasicUsage(ctx context.Context, bus EventBus) error {
	logger.Info("Demonstrating basic usage")

	// 基础订阅
	handler := func(ctx context.Context, message []byte) error {
		logger.Info("Received message", "content", string(message))
		return nil
	}

	if err := bus.Subscribe(ctx, "basic-topic", handler); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	// 基础发布
	message := []byte("Hello, unified EventBus!")
	if err := bus.Publish(ctx, "basic-topic", message); err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}

	// 等待消息处理
	time.Sleep(1 * time.Second)
	return nil
}

// demonstrateAdvancedUsage 演示高级用法
func demonstrateAdvancedUsage(ctx context.Context, bus EventBus, config *EventBusConfig) error {
	logger.Info("Demonstrating advanced usage")

	// 高级订阅示例
	advancedHandler := func(ctx context.Context, message []byte) error {
		logger.Info("Processing advanced message", "content", string(message))
		// 模拟处理时间
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	opts := SubscribeOptions{
		ProcessingTimeout: 30 * time.Second,
		RateLimit:         100,
		RateBurst:         200,
		MaxRetries:        3,
		RetryBackoff:      1 * time.Second,
		DeadLetterEnabled: config.Enterprise.Subscriber.DeadLetter.Enabled,
	}

	if err := bus.SubscribeWithOptions(ctx, "advanced-topic", advancedHandler, opts); err != nil {
		return fmt.Errorf("failed to subscribe with options: %w", err)
	}

	// 高级发布
	publishOpts := PublishOptions{
		Timeout: 30 * time.Second,
		RetryPolicy: RetryPolicy{
			MaxRetries:      3,
			InitialInterval: 1 * time.Second,
			MaxInterval:     30 * time.Second,
			Multiplier:      2.0,
		},
		Metadata:    map[string]string{"source": "unified-example"},
		AggregateID: "user-123",
	}

	advancedMessage := []byte("Advanced message with options")
	if err := bus.PublishWithOptions(ctx, "advanced-topic", advancedMessage, publishOpts); err != nil {
		return fmt.Errorf("failed to publish with options: %w", err)
	}

	// 等待消息处理
	time.Sleep(2 * time.Second)
	return nil
}

// demonstrateMonitoring 演示监控功能
func demonstrateMonitoring(ctx context.Context, bus EventBus, config *EventBusConfig) error {
	logger.Info("Demonstrating monitoring")

	// 暂时忽略未使用的参数
	_ = ctx
	_ = config

	// 获取监控指标
	metrics := bus.GetMetrics()
	logger.Info("Current metrics",
		"messagesPublished", metrics.MessagesPublished,
		"messagesConsumed", metrics.MessagesConsumed,
		"publishErrors", metrics.PublishErrors,
		"consumeErrors", metrics.ConsumeErrors)

	// 获取健康状态
	healthStatus := bus.GetHealthStatus()
	logger.Info("Health status",
		"isHealthy", healthStatus.IsHealthy,
		"consecutiveFailures", healthStatus.ConsecutiveFailures,
		"isRunning", healthStatus.IsRunning,
		"eventBusType", healthStatus.EventBusType)

	// 获取连接状态
	connectionState := bus.GetConnectionState()
	logger.Info("Connection state",
		"isConnected", connectionState.IsConnected,
		"reconnectCount", connectionState.ReconnectCount)

	return nil
}

// 使用示例
func ExampleUnifiedEventBus() {
	if err := UnifiedEventBusExample(); err != nil {
		logger.Error("Unified EventBus example failed", "error", err)
	}
}
