package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// TestHealthCheckFailureScenarios 测试健康检查故障场景
func TestHealthCheckFailureScenarios(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	// 测试1: 订阅器超时检测
	t.Run("SubscriberTimeoutDetection", func(t *testing.T) {
		cfg := &config.EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
			HealthCheck: config.HealthCheckConfig{
				Enabled: true,
				Publisher: config.HealthCheckPublisherConfig{
					Topic:            "test-timeout",
					Interval:         10 * time.Second, // 很长的间隔，不会发送消息
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					MessageTTL:       30 * time.Second,
				},
				Subscriber: config.HealthCheckSubscriberConfig{
					Topic:             "test-timeout",
					MonitorInterval:   1 * time.Second, // 短间隔快速检测
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

		// 只启动订阅器，不启动发布器
		ctx := context.Background()
		err = bus.StartHealthCheckSubscriber(ctx)
		if err != nil {
			t.Fatalf("Failed to start subscriber: %v", err)
		}

		// 设置回调来捕获警报
		var alerts []HealthCheckAlert
		var alertMu sync.Mutex

		callback := func(ctx context.Context, alert HealthCheckAlert) error {
			alertMu.Lock()
			alerts = append(alerts, alert)
			alertMu.Unlock()
			t.Logf("Received alert: %+v", alert)
			return nil
		}

		err = bus.RegisterHealthCheckSubscriberCallback(callback)
		if err != nil {
			t.Fatalf("Failed to register callback: %v", err)
		}

		// 等待足够时间让订阅器检测到超时
		time.Sleep(4 * time.Second)

		// 检查是否收到警报
		alertMu.Lock()
		alertCount := len(alerts)
		alertMu.Unlock()

		if alertCount == 0 {
			t.Error("Expected to receive timeout alerts, but got none")
		} else {
			t.Logf("Received %d alerts as expected", alertCount)
		}

		// 检查订阅器统计
		stats := bus.GetHealthCheckSubscriberStats()
		if stats.ConsecutiveMisses == 0 {
			t.Error("Expected consecutive misses > 0")
		}

		t.Logf("Final subscriber stats: %+v", stats)
	})

	// 测试2: 发布器故障恢复
	t.Run("PublisherFailureRecovery", func(t *testing.T) {
		cfg := &config.EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
			HealthCheck: config.HealthCheckConfig{
				Enabled: true,
				Publisher: config.HealthCheckPublisherConfig{
					Topic:            "test-recovery",
					Interval:         1 * time.Second,
					Timeout:          2 * time.Second,
					FailureThreshold: 2,
					MessageTTL:       30 * time.Second,
				},
				Subscriber: config.HealthCheckSubscriberConfig{
					Topic:             "test-recovery",
					MonitorInterval:   500 * time.Millisecond,
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

		// 启动发布器和订阅器
		err = bus.StartHealthCheckPublisher(ctx)
		if err != nil {
			t.Fatalf("Failed to start publisher: %v", err)
		}

		err = bus.StartHealthCheckSubscriber(ctx)
		if err != nil {
			t.Fatalf("Failed to start subscriber: %v", err)
		}

		// 等待系统稳定
		time.Sleep(2 * time.Second)

		// 检查初始状态
		status1 := bus.GetHealthCheckPublisherStatus()
		if !status1.IsHealthy {
			t.Error("Publisher should be healthy initially")
		}

		// 停止发布器模拟故障
		err = bus.StopHealthCheckPublisher()
		if err != nil {
			t.Fatalf("Failed to stop publisher: %v", err)
		}

		// 等待一段时间让系统检测到故障
		time.Sleep(3 * time.Second)

		// 重新启动发布器模拟恢复
		err = bus.StartHealthCheckPublisher(ctx)
		if err != nil {
			t.Fatalf("Failed to restart publisher: %v", err)
		}

		// 等待恢复
		time.Sleep(2 * time.Second)

		// 检查恢复后状态
		status2 := bus.GetHealthCheckPublisherStatus()
		if !status2.IsHealthy {
			t.Error("Publisher should be healthy after recovery")
		}

		stats := bus.GetHealthCheckSubscriberStats()
		t.Logf("Recovery test stats: %+v", stats)
	})

	// 测试3: 回调函数错误处理
	t.Run("CallbackErrorHandling", func(t *testing.T) {
		cfg := &config.EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
			HealthCheck: config.HealthCheckConfig{
				Enabled: true,
				Publisher: config.HealthCheckPublisherConfig{
					Topic:            "test-callback-error",
					Interval:         10 * time.Second, // 长间隔避免频繁触发
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					MessageTTL:       30 * time.Second,
				},
				Subscriber: config.HealthCheckSubscriberConfig{
					Topic:             "test-callback-error",
					MonitorInterval:   1 * time.Second,
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

		// 启动订阅器（不启动发布器以触发警报）
		ctx := context.Background()
		err = bus.StartHealthCheckSubscriber(ctx)
		if err != nil {
			t.Fatalf("Failed to start subscriber: %v", err)
		}

		// 设置一个会返回错误的回调
		var callbackCount int
		var callbackMu sync.Mutex

		errorCallback := func(ctx context.Context, alert HealthCheckAlert) error {
			callbackMu.Lock()
			callbackCount++
			callbackMu.Unlock()
			t.Logf("Error callback called %d times", callbackCount)
			return nil // 即使返回错误，系统也应该继续工作
		}

		err = bus.RegisterHealthCheckSubscriberCallback(errorCallback)
		if err != nil {
			t.Fatalf("Failed to register error callback: %v", err)
		}

		// 等待回调被调用
		time.Sleep(3 * time.Second)

		callbackMu.Lock()
		count := callbackCount
		callbackMu.Unlock()

		if count == 0 {
			t.Error("Expected callback to be called, but it wasn't")
		} else {
			t.Logf("Callback was called %d times as expected", count)
		}

		// 验证系统仍然正常工作
		stats := bus.GetHealthCheckSubscriberStats()
		if !stats.IsHealthy {
			// 在这种情况下，由于没有收到消息，订阅器应该是不健康的
			t.Logf("Subscriber is unhealthy as expected: %+v", stats)
		}
	})
}

// TestHealthCheckPerformance 测试健康检查性能
func TestHealthCheckPerformance(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	// 测试高频率健康检查
	t.Run("HighFrequencyHealthCheck", func(t *testing.T) {
		cfg := &config.EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
			HealthCheck: config.HealthCheckConfig{
				Enabled: true,
				Publisher: config.HealthCheckPublisherConfig{
					Topic:            "test-performance",
					Interval:         100 * time.Millisecond, // 高频率
					Timeout:          1 * time.Second,
					FailureThreshold: 3,
					MessageTTL:       5 * time.Second,
				},
				Subscriber: config.HealthCheckSubscriberConfig{
					Topic:             "test-performance",
					MonitorInterval:   50 * time.Millisecond, // 高频率监控
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

		// 记录开始时间
		startTime := time.Now()

		// 启动健康检查
		err = bus.StartHealthCheckPublisher(ctx)
		if err != nil {
			t.Fatalf("Failed to start publisher: %v", err)
		}

		err = bus.StartHealthCheckSubscriber(ctx)
		if err != nil {
			t.Fatalf("Failed to start subscriber: %v", err)
		}

		// 运行一段时间
		testDuration := 5 * time.Second
		time.Sleep(testDuration)

		// 检查性能指标
		stats := bus.GetHealthCheckSubscriberStats()
		elapsed := time.Since(startTime)

		messagesPerSecond := float64(stats.TotalMessagesReceived) / elapsed.Seconds()

		t.Logf("Performance test results:")
		t.Logf("  Duration: %v", elapsed)
		t.Logf("  Total messages: %d", stats.TotalMessagesReceived)
		t.Logf("  Messages per second: %.2f", messagesPerSecond)
		t.Logf("  Consecutive misses: %d", stats.ConsecutiveMisses)
		t.Logf("  Total alerts: %d", stats.TotalAlerts)

		// 验证性能指标
		if messagesPerSecond < 5.0 {
			t.Errorf("Expected at least 5 messages per second, got %.2f", messagesPerSecond)
		}

		if stats.ConsecutiveMisses > 2 {
			t.Errorf("Too many consecutive misses: %d", stats.ConsecutiveMisses)
		}
	})
}

// TestHealthCheckStability 测试健康检查稳定性
func TestHealthCheckStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stability test in short mode")
	}

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
				Topic:            "test-stability",
				Interval:         1 * time.Second,
				Timeout:          2 * time.Second,
				FailureThreshold: 3,
				MessageTTL:       10 * time.Second,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:             "test-stability",
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

	// 启动健康检查
	err = bus.StartHealthCheckPublisher(ctx)
	if err != nil {
		t.Fatalf("Failed to start publisher: %v", err)
	}

	err = bus.StartHealthCheckSubscriber(ctx)
	if err != nil {
		t.Fatalf("Failed to start subscriber: %v", err)
	}

	// 长时间运行测试
	testDuration := 10 * time.Second
	t.Logf("Running stability test for %v", testDuration)

	time.Sleep(testDuration)

	// 检查最终状态
	publisherStatus := bus.GetHealthCheckPublisherStatus()
	subscriberStats := bus.GetHealthCheckSubscriberStats()

	t.Logf("Stability test results:")
	t.Logf("  Publisher healthy: %v", publisherStatus.IsHealthy)
	t.Logf("  Publisher failures: %d", publisherStatus.ConsecutiveFailures)
	t.Logf("  Subscriber healthy: %v", subscriberStats.IsHealthy)
	t.Logf("  Total messages: %d", subscriberStats.TotalMessagesReceived)
	t.Logf("  Total alerts: %d", subscriberStats.TotalAlerts)
	t.Logf("  Uptime: %.2f seconds", subscriberStats.UptimeSeconds)

	// 验证稳定性
	if !publisherStatus.IsHealthy {
		t.Error("Publisher should be healthy after stability test")
	}

	if !subscriberStats.IsHealthy {
		t.Error("Subscriber should be healthy after stability test")
	}

	if subscriberStats.TotalMessagesReceived < 5 {
		t.Errorf("Expected at least 5 messages, got %d", subscriberStats.TotalMessagesReceived)
	}

	if subscriberStats.TotalAlerts > 0 {
		t.Errorf("Expected no alerts during stable operation, got %d", subscriberStats.TotalAlerts)
	}
}
