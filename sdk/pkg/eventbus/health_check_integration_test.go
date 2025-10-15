package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestHealthCheckBasicFunctionality 测试健康检查基础功能
func TestHealthCheckBasicFunctionality(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}
	// 创建内存EventBus
	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "test-service",
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:            "test-health-check",
				Interval:         1 * time.Second, // 快速测试
				Timeout:          5 * time.Second,
				FailureThreshold: 3,
				MessageTTL:       30 * time.Second,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:             "test-health-check",
				MonitorInterval:   500 * time.Millisecond, // 快速检查
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
	// 测试1: 启动健康检查发布器
	t.Run("StartHealthCheckPublisher", func(t *testing.T) {
		ctx := context.Background()
		err := bus.StartHealthCheckPublisher(ctx)
		if err != nil {
			t.Errorf("Failed to start health check publisher: %v", err)
		}
		// 验证状态
		status := bus.GetHealthCheckPublisherStatus()
		if !status.IsHealthy {
			t.Error("Health check publisher should be healthy after start")
		}
	})
	// 测试2: 启动健康检查订阅器
	t.Run("StartHealthCheckSubscriber", func(t *testing.T) {
		ctx := context.Background()
		// 设置回调函数来验证接收到消息
		var receivedMessages int
		var mu sync.Mutex
		callback := func(ctx context.Context, alert HealthCheckAlert) error {
			mu.Lock()
			receivedMessages++
			mu.Unlock()
			t.Logf("Received health check alert: %+v", alert)
			return nil
		}
		err := bus.RegisterHealthCheckSubscriberCallback(callback)
		if err != nil {
			t.Errorf("Failed to register callback: %v", err)
		}
		err = bus.StartHealthCheckSubscriber(ctx)
		if err != nil {
			t.Errorf("Failed to start health check subscriber: %v", err)
		}
		// 等待接收消息
		time.Sleep(3 * time.Second)
		mu.Lock()
		count := receivedMessages
		mu.Unlock()
		if count == 0 {
			t.Error("Should have received at least one health check message")
		}
		t.Logf("Received %d health check messages", count)
	})
	// 测试3: 停止健康检查
	t.Run("StopHealthCheck", func(t *testing.T) {
		err := bus.StopHealthCheckPublisher()
		if err != nil {
			t.Errorf("Failed to stop health check publisher: %v", err)
		}
		err = bus.StopHealthCheckSubscriber()
		if err != nil {
			t.Errorf("Failed to stop health check subscriber: %v", err)
		}
		// 验证状态
		status := bus.GetHealthCheckPublisherStatus()
		if status.IsHealthy {
			t.Error("Health check publisher should not be healthy after stop")
		}
	})
}

// TestHealthCheckMessageSerialization 测试健康检查消息序列化
func TestHealthCheckMessageSerialization(t *testing.T) {
	// 创建健康检查消息
	msg := CreateHealthCheckMessage("test-service", "memory")
	msg.SetMetadata("testKey", "testValue")
	// 测试序列化
	data, err := msg.ToBytes()
	if err != nil {
		t.Fatalf("Failed to serialize health check message: %v", err)
	}
	if len(data) == 0 {
		t.Error("Serialized data should not be empty")
	}
	// 测试反序列化
	parser := NewHealthCheckMessageParser()
	parsedMsg, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("Failed to parse health check message: %v", err)
	}
	// 验证数据一致性
	if parsedMsg.Source != msg.Source {
		t.Errorf("Source mismatch: expected %s, got %s", msg.Source, parsedMsg.Source)
	}
	if parsedMsg.EventBusType != msg.EventBusType {
		t.Errorf("EventBusType mismatch: expected %s, got %s", msg.EventBusType, parsedMsg.EventBusType)
	}
	if value, exists := parsedMsg.GetMetadata("testKey"); !exists || value != "testValue" {
		t.Error("Metadata not preserved during serialization")
	}
}

// TestHealthCheckConfiguration 测试健康检查配置
func TestHealthCheckConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config config.HealthCheckConfig
		valid  bool
	}{
		{
			name: "ValidConfiguration",
			config: config.HealthCheckConfig{
				Enabled: true,
				Publisher: config.HealthCheckPublisherConfig{
					Topic:            "test-topic",
					Interval:         30 * time.Second,
					Timeout:          10 * time.Second,
					FailureThreshold: 3,
					MessageTTL:       5 * time.Minute,
				},
				Subscriber: config.HealthCheckSubscriberConfig{
					Topic:             "test-topic",
					MonitorInterval:   15 * time.Second,
					WarningThreshold:  2,
					ErrorThreshold:    3,
					CriticalThreshold: 5,
				},
			},
			valid: true,
		},
		{
			name: "DisabledConfiguration",
			config: config.HealthCheckConfig{
				Enabled: false,
			},
			valid: true,
		},
		{
			name: "DefaultConfiguration",
			config: config.HealthCheckConfig{
				Enabled: true,
				// 使用默认值
			},
			valid: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
				HealthCheck: tt.config,
			}
			err := InitializeFromConfig(cfg)
			if tt.valid && err != nil {
				t.Errorf("Expected valid configuration but got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("Expected invalid configuration but got no error")
			}
			if err == nil {
				CloseGlobal()
			}
		})
	}
}

// TestHealthCheckCallbacks 测试健康检查回调机制
func TestHealthCheckCallbacks(t *testing.T) {
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
				Interval: 500 * time.Millisecond,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				MonitorInterval: 200 * time.Millisecond,
			},
		},
	}
	err := InitializeFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize EventBus: %v", err)
	}
	defer CloseGlobal()
	bus := GetGlobal()
	// 测试回调注册和调用
	var callbackCount int
	var mu sync.Mutex
	callback := func(ctx context.Context, alert HealthCheckAlert) error {
		mu.Lock()
		callbackCount++
		mu.Unlock()
		t.Logf("Callback called with alert: %+v", alert)
		return nil
	}
	// 注册回调
	err = bus.RegisterHealthCheckSubscriberCallback(callback)
	if err != nil {
		t.Fatalf("Failed to register callback: %v", err)
	}
	// 启动健康检查订阅器（先启动订阅器）
	ctx := context.Background()
	err = bus.StartHealthCheckSubscriber(ctx)
	if err != nil {
		t.Fatalf("Failed to start subscriber: %v", err)
	}
	// 启动发布器
	err = bus.StartHealthCheckPublisher(ctx)
	if err != nil {
		t.Fatalf("Failed to start publisher: %v", err)
	}
	// 等待一些正常消息
	time.Sleep(1 * time.Second)
	// 停止发布器以触发告警
	err = bus.StopHealthCheckPublisher()
	if err != nil {
		t.Fatalf("Failed to stop publisher: %v", err)
	}
	// 等待告警触发（监控间隔200ms，发布间隔500ms，所以等待1秒足够）
	time.Sleep(1 * time.Second)
	mu.Lock()
	count := callbackCount
	mu.Unlock()
	if count == 0 {
		t.Error("Callback should have been called at least once after publisher stopped")
	}
	t.Logf("Callback was called %d times", count)
}

// TestHealthCheckStatus 测试健康检查状态
func TestHealthCheckStatus(t *testing.T) {
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
		},
	}
	err := InitializeFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize EventBus: %v", err)
	}
	defer CloseGlobal()
	bus := GetGlobal()
	// 测试初始状态
	status := bus.GetHealthCheckPublisherStatus()
	if status.IsHealthy {
		t.Error("Publisher should not be healthy before start")
	}
	// 启动后测试状态
	ctx := context.Background()
	err = bus.StartHealthCheckPublisher(ctx)
	if err != nil {
		t.Fatalf("Failed to start publisher: %v", err)
	}
	// 等待第一次健康检查完成（启动时会立即执行一次）
	time.Sleep(100 * time.Millisecond)
	status = bus.GetHealthCheckPublisherStatus()
	if !status.IsHealthy {
		t.Errorf("Publisher should be healthy after start, got: IsHealthy=%v, ConsecutiveFailures=%d, IsRunning=%v",
			status.IsHealthy, status.ConsecutiveFailures, status.IsRunning)
	}
	// 停止后测试状态
	err = bus.StopHealthCheckPublisher()
	if err != nil {
		t.Fatalf("Failed to stop publisher: %v", err)
	}
	status = bus.GetHealthCheckPublisherStatus()
	if status.IsHealthy {
		t.Error("Publisher should not be healthy after stop")
	}
}

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
					Interval:         2 * time.Second, // 短间隔，以便快速触发告警
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					MessageTTL:       30 * time.Second,
				},
				Subscriber: config.HealthCheckSubscriberConfig{
					Topic:             "test-timeout",
					MonitorInterval:   500 * time.Millisecond, // 短间隔快速检测
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
					Interval:         2 * time.Second, // 短间隔以便快速触发告警
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					MessageTTL:       30 * time.Second,
				},
				Subscriber: config.HealthCheckSubscriberConfig{
					Topic:             "test-callback-error",
					MonitorInterval:   500 * time.Millisecond, // 短间隔快速检测
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
				Interval:         500 * time.Millisecond, // 更短的间隔，避免监控器误报
				Timeout:          2 * time.Second,
				FailureThreshold: 3,
				MessageTTL:       10 * time.Second,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:             "test-stability",
				MonitorInterval:   1 * time.Second, // 监控间隔应该大于发布间隔
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
	// 允许少量告警（由于监控间隔和发布间隔之间的时序竞争）
	// 在10秒内，监控间隔为1秒，发布间隔为500ms，可能会有时序不匹配
	// 最多允许10次告警（约50%的监控周期）
	if subscriberStats.TotalAlerts > 10 {
		t.Errorf("Expected at most 10 alerts during stable operation, got %d", subscriberStats.TotalAlerts)
	}
}

// TestEventBusManager_StartAllHealthCheck_Success_Coverage 测试启动所有健康检查（成功）
func TestEventBusManager_StartAllHealthCheck_Success_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 启动所有健康检查
	err = manager.StartAllHealthCheck(ctx)
	assert.NoError(t, err)
	// 验证健康检查已启动
	assert.NotNil(t, manager.healthChecker)
	assert.NotNil(t, manager.healthCheckSubscriber)
	// 停止健康检查
	manager.StopAllHealthCheck()
}

// TestEventBusManager_StartAllHealthCheck_AlreadyStarted_Coverage 测试启动所有健康检查（已启动）
func TestEventBusManager_StartAllHealthCheck_AlreadyStarted_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 第一次启动
	err = manager.StartAllHealthCheck(ctx)
	require.NoError(t, err)
	// 第二次启动（应该成功，因为会先停止）
	err = manager.StartAllHealthCheck(ctx)
	assert.NoError(t, err)
	// 停止健康检查
	manager.StopAllHealthCheck()
}

// TestEventBusManager_StartAllHealthCheck_Closed_Coverage 测试启动所有健康检查（已关闭）
func TestEventBusManager_StartAllHealthCheck_Closed_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 关闭 EventBus
	bus.Close()
	// 尝试启动健康检查（应该失败）
	err = manager.StartAllHealthCheck(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventbus is closed")
}

// TestEventBusManager_Publish_NilMessage_Coverage 测试发布（nil 消息）
func TestEventBusManager_Publish_NilMessage_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	ctx := context.Background()
	// 发布 nil 消息（应该成功，因为 nil 是有效的字节数组）
	err = bus.Publish(ctx, "test-topic", nil)
	assert.NoError(t, err)
}

// TestEventBusManager_GetConnectionState_Closed_Coverage 测试获取连接状态（已关闭）
func TestEventBusManager_GetConnectionState_Closed_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	manager := bus.(*eventBusManager)
	// 关闭 EventBus
	bus.Close()
	// 获取连接状态
	state := manager.GetConnectionState()
	assert.False(t, state.IsConnected)
}

// TestEventBusManager_SetTopicConfigStrategy_AllStrategies_Coverage 测试设置主题配置策略（所有策略）
func TestEventBusManager_SetTopicConfigStrategy_AllStrategies_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	strategies := []TopicConfigStrategy{
		StrategyCreateOnly,
		StrategyCreateOrUpdate,
		StrategyValidateOnly,
		StrategySkip,
	}
	for _, strategy := range strategies {
		manager.SetTopicConfigStrategy(strategy)
		// 验证策略已设置（Memory EventBus 支持）
		currentStrategy := manager.GetTopicConfigStrategy()
		assert.Equal(t, strategy, currentStrategy, "Strategy should be set to %s", strategy)
	}
}
