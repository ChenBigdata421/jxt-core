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
