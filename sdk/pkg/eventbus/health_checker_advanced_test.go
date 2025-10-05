package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHealthChecker 测试创建健康检查器
func TestNewHealthChecker(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:            "health",
			Interval:         30 * time.Second,
			Timeout:          5 * time.Second,
			FailureThreshold: 3,
			MessageTTL:       5 * time.Minute,
		},
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")
	assert.NotNil(t, checker)
	assert.Equal(t, "test-service", checker.source)
	assert.Equal(t, "memory", checker.eventBusType)
	assert.Equal(t, "health", checker.config.Publisher.Topic)
}

// TestNewHealthChecker_DefaultConfig 测试默认配置
func TestNewHealthChecker_DefaultConfig(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")
	assert.NotNil(t, checker)

	// 检查默认值
	assert.Equal(t, "health-check-memory", checker.config.Publisher.Topic)
	assert.Equal(t, 2*time.Minute, checker.config.Publisher.Interval)
	assert.Equal(t, 10*time.Second, checker.config.Publisher.Timeout)
	assert.Equal(t, int32(3), checker.config.Publisher.FailureThreshold)
	assert.Equal(t, 5*time.Minute, checker.config.Publisher.MessageTTL)
}

// TestHealthChecker_Start 测试启动健康检查
func TestHealthChecker_Start(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:    "health",
			Interval: 100 * time.Millisecond,
		},
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	ctx := context.Background()
	err = checker.Start(ctx)
	assert.NoError(t, err)

	// 等待一段时间确保健康检查运行
	time.Sleep(150 * time.Millisecond)

	// 停止健康检查
	err = checker.Stop()
	assert.NoError(t, err)
}

// TestHealthChecker_Start_AlreadyRunning 测试重复启动
func TestHealthChecker_Start_AlreadyRunning(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:    "health",
			Interval: 1 * time.Second,
		},
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	ctx := context.Background()
	err = checker.Start(ctx)
	assert.NoError(t, err)

	// 再次启动应该返回 nil（已经在运行）
	err = checker.Start(ctx)
	assert.NoError(t, err)

	checker.Stop()
}

// TestHealthChecker_Start_Disabled 测试禁用时启动
func TestHealthChecker_Start_Disabled(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: false,
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	ctx := context.Background()
	err = checker.Start(ctx)
	assert.NoError(t, err)

	// 检查是否真的没有启动
	assert.False(t, checker.isRunning.Load())
}

// TestHealthChecker_Stop 测试停止健康检查
func TestHealthChecker_Stop(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:    "health",
			Interval: 100 * time.Millisecond,
		},
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	ctx := context.Background()
	err = checker.Start(ctx)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	err = checker.Stop()
	assert.NoError(t, err)
	assert.False(t, checker.isRunning.Load())
}

// TestHealthChecker_Stop_NotRunning 测试停止未运行的检查器
func TestHealthChecker_Stop_NotRunning(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	err = checker.Stop()
	assert.NoError(t, err)
}

// TestHealthChecker_RegisterCallback 测试注册回调
func TestHealthChecker_RegisterCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	callback := func(ctx context.Context, result HealthCheckResult) error {
		return nil
	}

	err = checker.RegisterCallback(callback)
	assert.NoError(t, err)

	// 验证回调已注册
	assert.Len(t, checker.callbacks, 1)
}

// TestHealthChecker_GetStatus 测试获取状态
func TestHealthChecker_GetStatus(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:    "health",
			Interval: 100 * time.Millisecond,
		},
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	ctx := context.Background()
	err = checker.Start(ctx)
	require.NoError(t, err)
	defer checker.Stop()

	time.Sleep(150 * time.Millisecond)

	status := checker.GetStatus()
	assert.NotNil(t, status)
	assert.False(t, status.LastSuccessTime.IsZero())
}

// TestHealthChecker_IsHealthy 测试健康状态检查
func TestHealthChecker_IsHealthy(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:            "health",
			Interval:         100 * time.Millisecond,
			FailureThreshold: 3,
		},
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	ctx := context.Background()
	err = checker.Start(ctx)
	require.NoError(t, err)
	defer checker.Stop()

	time.Sleep(150 * time.Millisecond)

	isHealthy := checker.IsHealthy()
	assert.True(t, isHealthy)
}

// TestHealthChecker_MultipleCallbacks 测试多个回调
func TestHealthChecker_MultipleCallbacks(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	callback1 := func(ctx context.Context, result HealthCheckResult) error {
		return nil
	}

	callback2 := func(ctx context.Context, result HealthCheckResult) error {
		return nil
	}

	checker.RegisterCallback(callback1)
	checker.RegisterCallback(callback2)

	assert.Len(t, checker.callbacks, 2)
}

// TestHealthChecker_DifferentEventBusTypes 测试不同的EventBus类型
func TestHealthChecker_DifferentEventBusTypes(t *testing.T) {
	tests := []struct {
		name          string
		eventBusType  string
		expectedTopic string
	}{
		{
			name:          "Memory",
			eventBusType:  "memory",
			expectedTopic: "jxt-core-memory-health-check",
		},
		{
			name:          "Kafka",
			eventBusType:  "kafka",
			expectedTopic: "jxt-core-kafka-health-check",
		},
		{
			name:          "NATS",
			eventBusType:  "nats",
			expectedTopic: "jxt-core-nats-health-check",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &EventBusConfig{Type: "memory"}
			bus, err := NewEventBus(cfg)
			require.NoError(t, err)
			defer bus.Close()

			healthConfig := config.HealthCheckConfig{
				Enabled: true,
			}

			checker := NewHealthChecker(healthConfig, bus, "test-service", tt.eventBusType)
			assert.Equal(t, tt.expectedTopic, checker.config.Publisher.Topic)
		})
	}
}

// TestHealthChecker_CustomTopic 测试自定义主题
func TestHealthChecker_CustomTopic(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic: "custom-health-topic",
		},
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")
	assert.Equal(t, "custom-health-topic", checker.config.Publisher.Topic)
}

// TestHealthChecker_StartStop_Multiple 测试多次启动停止
func TestHealthChecker_StartStop_Multiple(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:    "health",
			Interval: 100 * time.Millisecond,
		},
	}

	checker := NewHealthChecker(healthConfig, bus, "test-service", "memory")

	ctx := context.Background()

	// 第一次启动停止
	err = checker.Start(ctx)
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	err = checker.Stop()
	assert.NoError(t, err)

	// 第二次启动停止
	err = checker.Start(ctx)
	assert.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	err = checker.Stop()
	assert.NoError(t, err)
}
