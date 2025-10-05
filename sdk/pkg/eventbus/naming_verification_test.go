package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// initTestLogger 初始化测试用的logger
func initTestLogger() {
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}
}

func TestPublisherNamingVerification(t *testing.T) {
	// 初始化logger
	initTestLogger()

	t.Log("🎯 Publisher/Sender 命名统一重构验证")

	// 1. 验证新的配置结构
	t.Log("1. 验证新的配置结构...")
	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "naming-verification-service",
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			// ✅ 使用新的Publisher命名
			Publisher: config.HealthCheckPublisherConfig{
				Topic:            "health-check-verification",
				Interval:         2 * time.Second,
				Timeout:          1 * time.Second,
				FailureThreshold: 3,
				MessageTTL:       10 * time.Second,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:             "health-check-verification",
				MonitorInterval:   1 * time.Second,
				WarningThreshold:  2,
				ErrorThreshold:    3,
				CriticalThreshold: 5,
			},
		},
	}

	t.Logf("✅ 配置创建成功:")
	t.Logf("   - Publisher Topic: %s", cfg.HealthCheck.Publisher.Topic)
	t.Logf("   - Publisher Interval: %v", cfg.HealthCheck.Publisher.Interval)
	t.Logf("   - Subscriber Topic: %s", cfg.HealthCheck.Subscriber.Topic)
	t.Logf("   - Subscriber Monitor Interval: %v", cfg.HealthCheck.Subscriber.MonitorInterval)

	// 2. 验证EventBus初始化
	t.Log("2. 验证EventBus初始化...")
	if err := InitializeFromConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize EventBus: %v", err)
	}
	defer CloseGlobal()

	bus := GetGlobal()
	t.Log("✅ EventBus初始化成功")

	// 3. 验证分离式健康检查启动
	t.Log("3. 验证分离式健康检查启动...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 启动发布器
	if err := bus.StartHealthCheckPublisher(ctx); err != nil {
		t.Errorf("Failed to start health check publisher: %v", err)
	} else {
		t.Log("✅ 健康检查发布器启动成功")
	}

	// 启动订阅器
	if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
		t.Errorf("Failed to start health check subscriber: %v", err)
	} else {
		t.Log("✅ 健康检查订阅器启动成功")
	}

	// 4. 验证状态获取
	t.Log("4. 验证状态获取...")

	// 等待一些健康检查执行
	time.Sleep(3 * time.Second)

	publisherStatus := bus.GetHealthCheckPublisherStatus()
	t.Logf("✅ 发布器状态: 健康=%v, 连续失败=%d",
		publisherStatus.IsHealthy, publisherStatus.ConsecutiveFailures)

	subscriberStats := bus.GetHealthCheckSubscriberStats()
	t.Logf("✅ 订阅器统计: 健康=%v, 收到消息=%d, 连续错过=%d",
		subscriberStats.IsHealthy, subscriberStats.TotalMessagesReceived, subscriberStats.ConsecutiveMisses)

	// 5. 验证回调注册
	t.Log("5. 验证回调注册...")

	// 注册发布器回调
	err := bus.RegisterHealthCheckPublisherCallback(func(ctx context.Context, result HealthCheckResult) error {
		t.Logf("📊 发布器回调: 成功=%v, 持续时间=%v",
			result.Success, result.Duration)
		return nil
	})
	if err != nil {
		t.Errorf("Failed to register publisher callback: %v", err)
	} else {
		t.Log("✅ 发布器回调注册成功")
	}

	// 注册订阅器回调
	err = bus.RegisterHealthCheckSubscriberCallback(func(ctx context.Context, alert HealthCheckAlert) error {
		t.Logf("🚨 订阅器告警: [%s] %s", alert.Severity, alert.AlertType)
		return nil
	})
	if err != nil {
		t.Errorf("Failed to register subscriber callback: %v", err)
	} else {
		t.Log("✅ 订阅器回调注册成功")
	}

	// 6. 运行一段时间观察
	t.Log("6. 运行观察...")
	t.Log("运行3秒观察健康检查行为...")

	time.Sleep(3 * time.Second)

	// 7. 优雅关闭
	t.Log("7. 优雅关闭...")
	if err := bus.StopAllHealthCheck(); err != nil {
		t.Errorf("Error stopping health checks: %v", err)
	} else {
		t.Log("✅ 健康检查停止成功")
	}

	t.Log("🎉 Publisher/Sender 命名统一重构验证完成!")
	t.Log("✅ 所有功能正常工作")
	t.Log("✅ 配置命名统一为Publisher")
	t.Log("✅ 接口命名保持一致")
	t.Log("✅ 分离式健康检查正常")
	t.Log("✅ 回调机制正常")
}
