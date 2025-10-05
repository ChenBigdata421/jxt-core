package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// TestHealthCheckConfigurationApplication 测试健康检查配置应用
func TestHealthCheckConfigurationApplication(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	// 测试自定义配置
	t.Run("CustomConfiguration", func(t *testing.T) {
		cfg := &config.EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
			HealthCheck: config.HealthCheckConfig{
				Enabled: true,
				Publisher: config.HealthCheckPublisherConfig{
					Topic:            "custom-topic",
					Interval:         5 * time.Second,
					Timeout:          3 * time.Second,
					FailureThreshold: 2,
					MessageTTL:       60 * time.Second,
				},
				Subscriber: config.HealthCheckSubscriberConfig{
					Topic:             "custom-topic",
					MonitorInterval:   2 * time.Second,
					WarningThreshold:  1,
					ErrorThreshold:    2,
					CriticalThreshold: 3,
				},
			},
		}

		err := InitializeFromConfig(cfg)
		if err != nil {
			t.Fatalf("Failed to initialize EventBus: %v", err)
		}
		defer CloseGlobal()

		bus := GetGlobal()
		ctx := context.Background()

		// 启动健康检查
		err = bus.StartHealthCheckPublisher(ctx)
		if err != nil {
			t.Fatalf("Failed to start publisher: %v", err)
		}

		err = bus.StartHealthCheckSubscriber(ctx)
		if err != nil {
			t.Fatalf("Failed to start subscriber: %v", err)
		}

		// 等待一段时间让系统运行
		time.Sleep(3 * time.Second)

		// 检查统计信息
		stats := bus.GetHealthCheckSubscriberStats()
		t.Logf("Subscriber stats: %+v", stats)

		// 验证配置是否正确应用
		if stats.TotalMessagesReceived == 0 {
			t.Error("Expected to receive at least one message")
		}

		// 停止健康检查
		err = bus.StopHealthCheckPublisher()
		if err != nil {
			t.Errorf("Failed to stop publisher: %v", err)
		}

		err = bus.StopHealthCheckSubscriber()
		if err != nil {
			t.Errorf("Failed to stop subscriber: %v", err)
		}
	})

	// 测试默认配置
	t.Run("DefaultConfiguration", func(t *testing.T) {
		cfg := &config.EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
			HealthCheck: config.HealthCheckConfig{
				Enabled: true,
				Publisher: config.HealthCheckPublisherConfig{
					Interval: 2 * time.Minute, // 必须设置正数
					Timeout:  10 * time.Second,
				},
				Subscriber: config.HealthCheckSubscriberConfig{
					MonitorInterval: 30 * time.Second, // 必须设置正数
				},
			},
		}

		err := InitializeFromConfig(cfg)
		if err != nil {
			t.Fatalf("Failed to initialize EventBus: %v", err)
		}
		defer CloseGlobal()

		bus := GetGlobal()
		ctx := context.Background()

		// 启动健康检查
		err = bus.StartHealthCheckPublisher(ctx)
		if err != nil {
			t.Fatalf("Failed to start publisher: %v", err)
		}

		err = bus.StartHealthCheckSubscriber(ctx)
		if err != nil {
			t.Fatalf("Failed to start subscriber: %v", err)
		}

		// 等待一段时间让系统运行
		time.Sleep(3 * time.Second)

		// 检查统计信息
		stats := bus.GetHealthCheckSubscriberStats()
		t.Logf("Default config subscriber stats: %+v", stats)

		// 验证默认配置下系统正常工作
		if stats.TotalMessagesReceived == 0 {
			t.Error("Expected to receive at least one message with default config")
		}

		// 停止健康检查
		err = bus.StopHealthCheckPublisher()
		if err != nil {
			t.Errorf("Failed to stop publisher: %v", err)
		}

		err = bus.StopHealthCheckSubscriber()
		if err != nil {
			t.Errorf("Failed to stop subscriber: %v", err)
		}
	})
}

// TestHealthCheckTimeoutDetection 测试超时检测
func TestHealthCheckTimeoutDetection(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	// 使用快速配置进行超时测试
	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "test-service",
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:            "timeout-test",
				Interval:         10 * time.Second, // 长间隔，不会发送消息
				Timeout:          2 * time.Second,
				FailureThreshold: 3,
				MessageTTL:       30 * time.Second,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:             "timeout-test",
				MonitorInterval:   500 * time.Millisecond, // 快速监控
				WarningThreshold:  1,
				ErrorThreshold:    2,
				CriticalThreshold: 3,
			},
		},
	}

	err := InitializeFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize EventBus: %v", err)
	}
	defer CloseGlobal()

	bus := GetGlobal()
	ctx := context.Background()

	// 只启动订阅器，不启动发布器
	err = bus.StartHealthCheckSubscriber(ctx)
	if err != nil {
		t.Fatalf("Failed to start subscriber: %v", err)
	}

	// 等待足够时间让监控器检测到超时
	time.Sleep(2 * time.Second)

	// 检查统计信息
	stats := bus.GetHealthCheckSubscriberStats()
	t.Logf("Timeout test stats: %+v", stats)

	// 验证超时检测
	if stats.IsHealthy {
		t.Error("Subscriber should be unhealthy when no messages are received")
	}

	// 停止订阅器
	err = bus.StopHealthCheckSubscriber()
	if err != nil {
		t.Errorf("Failed to stop subscriber: %v", err)
	}
}

// TestHealthCheckMessageFlow 测试消息流
func TestHealthCheckMessageFlow(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "test-service",
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:            "message-flow-test",
				Interval:         1 * time.Second,
				Timeout:          2 * time.Second,
				FailureThreshold: 3,
				MessageTTL:       30 * time.Second,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:             "message-flow-test",
				MonitorInterval:   500 * time.Millisecond,
				WarningThreshold:  2,
				ErrorThreshold:    3,
				CriticalThreshold: 5,
			},
		},
	}

	err := InitializeFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize EventBus: %v", err)
	}
	defer CloseGlobal()

	bus := GetGlobal()
	ctx := context.Background()

	// 启动发布器和订阅器
	err = bus.StartHealthCheckPublisher(ctx)
	if err != nil {
		t.Fatalf("Failed to start publisher: %v", err)
	}

	err = bus.StartHealthCheckSubscriber(ctx)
	if err != nil {
		t.Fatalf("Failed to start subscriber: %v", err)
	}

	// 等待消息流建立
	time.Sleep(3 * time.Second)

	// 检查发布器状态
	publisherStatus := bus.GetHealthCheckPublisherStatus()
	t.Logf("Publisher status: %+v", publisherStatus)

	// 检查订阅器统计
	subscriberStats := bus.GetHealthCheckSubscriberStats()
	t.Logf("Subscriber stats: %+v", subscriberStats)

	// 验证消息流
	if !publisherStatus.IsHealthy {
		t.Error("Publisher should be healthy")
	}

	if !subscriberStats.IsHealthy {
		t.Error("Subscriber should be healthy")
	}

	if subscriberStats.TotalMessagesReceived < 2 {
		t.Errorf("Expected at least 2 messages, got %d", subscriberStats.TotalMessagesReceived)
	}

	if subscriberStats.ConsecutiveMisses > 0 {
		t.Errorf("Expected no consecutive misses, got %d", subscriberStats.ConsecutiveMisses)
	}

	// 停止健康检查
	err = bus.StopHealthCheckPublisher()
	if err != nil {
		t.Errorf("Failed to stop publisher: %v", err)
	}

	err = bus.StopHealthCheckSubscriber()
	if err != nil {
		t.Errorf("Failed to stop subscriber: %v", err)
	}
}
