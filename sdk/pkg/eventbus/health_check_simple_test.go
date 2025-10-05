package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// TestHealthCheckMessageCreation 测试健康检查消息创建
func TestHealthCheckMessageCreation(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	// 测试1: 基本消息创建
	t.Run("BasicMessageCreation", func(t *testing.T) {
		msg := CreateHealthCheckMessage("test-service", "memory")
		if msg == nil {
			t.Fatal("Failed to create health check message")
		}

		if msg.Source != "test-service" {
			t.Errorf("Expected source 'test-service', got '%s'", msg.Source)
		}

		if msg.EventBusType != "memory" {
			t.Errorf("Expected eventBusType 'memory', got '%s'", msg.EventBusType)
		}

		if msg.Metadata == nil {
			t.Error("Metadata should not be nil")
		}
	})

	// 测试2: 消息序列化
	t.Run("MessageSerialization", func(t *testing.T) {
		msg := CreateHealthCheckMessage("test-service", "memory")

		// 添加一些元数据
		msg.SetMetadata("testKey", "testValue")
		msg.SetMetadata("environment", "test")

		// 测试序列化
		data, err := msg.ToBytes()
		if err != nil {
			t.Fatalf("Failed to serialize message: %v", err)
		}

		if len(data) == 0 {
			t.Error("Serialized data should not be empty")
		}

		t.Logf("Serialized message size: %d bytes", len(data))
	})

	// 测试3: 消息构建器
	t.Run("MessageBuilder", func(t *testing.T) {
		builder := NewHealthCheckMessageBuilder("test-service", "memory")
		msg := builder.
			WithCheckType("periodic").
			WithInstanceID("test-instance-123").
			WithEnvironment("test").
			WithMetadata("custom", "value").
			Build()

		if msg == nil {
			t.Fatal("Builder should return a message")
		}

		// 验证元数据
		if value, exists := msg.GetMetadata("checkType"); !exists || value != "periodic" {
			t.Error("checkType metadata not set correctly")
		}

		if value, exists := msg.GetMetadata("instanceId"); !exists || value != "test-instance-123" {
			t.Error("instanceId metadata not set correctly")
		}

		// 测试序列化
		data, err := msg.ToBytes()
		if err != nil {
			t.Fatalf("Failed to serialize builder message: %v", err)
		}

		if len(data) == 0 {
			t.Error("Serialized data should not be empty")
		}
	})
}

// TestHealthCheckBasicStartStop 测试健康检查基本启停
func TestHealthCheckBasicStartStop(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	// 创建简单配置
	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "test-service",
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:            "test-health-check",
				Interval:         5 * time.Second, // 较长间隔避免频繁触发
				Timeout:          5 * time.Second,
				FailureThreshold: 3,
				MessageTTL:       30 * time.Second,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:             "test-health-check",
				MonitorInterval:   2 * time.Second,
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
	if bus == nil {
		t.Fatal("Failed to get global EventBus")
	}

	// 测试1: 启动发布器
	t.Run("StartPublisher", func(t *testing.T) {
		ctx := context.Background()
		err := bus.StartHealthCheckPublisher(ctx)
		if err != nil {
			t.Errorf("Failed to start health check publisher: %v", err)
		}

		// 检查状态
		status := bus.GetHealthCheckPublisherStatus()
		if !status.IsHealthy {
			t.Error("Publisher should be healthy after start")
		}

		t.Logf("Publisher status: %+v", status)
	})

	// 测试2: 启动订阅器
	t.Run("StartSubscriber", func(t *testing.T) {
		ctx := context.Background()
		err := bus.StartHealthCheckSubscriber(ctx)
		if err != nil {
			t.Errorf("Failed to start health check subscriber: %v", err)
		}

		// 等待一小段时间让系统稳定
		time.Sleep(1 * time.Second)

		// 检查统计信息
		stats := bus.GetHealthCheckSubscriberStats()
		t.Logf("Subscriber stats: %+v", stats)
	})

	// 测试3: 停止健康检查
	t.Run("StopHealthCheck", func(t *testing.T) {
		// 停止发布器
		err := bus.StopHealthCheckPublisher()
		if err != nil {
			t.Errorf("Failed to stop health check publisher: %v", err)
		}

		// 停止订阅器
		err = bus.StopHealthCheckSubscriber()
		if err != nil {
			t.Errorf("Failed to stop health check subscriber: %v", err)
		}

		// 验证状态
		status := bus.GetHealthCheckPublisherStatus()
		if status.IsHealthy {
			t.Error("Publisher should not be healthy after stop")
		}

		t.Logf("Final publisher status: %+v", status)
	})
}

// TestHealthCheckConfigurationSimple 测试健康检查配置
func TestHealthCheckConfigurationSimple(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

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

// TestHealthCheckMessageParser 测试健康检查消息解析
func TestHealthCheckMessageParser(t *testing.T) {
	// 创建消息
	msg := CreateHealthCheckMessage("test-service", "memory")
	msg.SetMetadata("testKey", "testValue")

	// 序列化
	data, err := msg.ToBytes()
	if err != nil {
		t.Fatalf("Failed to serialize message: %v", err)
	}

	// 解析
	parser := NewHealthCheckMessageParser()
	parsedMsg, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	// 验证
	if parsedMsg.Source != msg.Source {
		t.Errorf("Source mismatch: expected %s, got %s", msg.Source, parsedMsg.Source)
	}

	if parsedMsg.EventBusType != msg.EventBusType {
		t.Errorf("EventBusType mismatch: expected %s, got %s", msg.EventBusType, parsedMsg.EventBusType)
	}

	if value, exists := parsedMsg.GetMetadata("testKey"); !exists || value != "testValue" {
		t.Error("Metadata not preserved during parsing")
	}
}
