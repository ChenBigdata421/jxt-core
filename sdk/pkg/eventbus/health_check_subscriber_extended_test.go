package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHealthCheckSubscriber 测试创建健康检查订阅器
func TestNewHealthCheckSubscriber(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Subscriber: config.HealthCheckSubscriberConfig{
			Topic: "health-check",
		},
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	assert.NotNil(t, subscriber)
	assert.Equal(t, "test-service", subscriber.source)
	assert.Equal(t, "memory", subscriber.eventBusType)
}

// TestHealthCheckSubscriber_DefaultConfig 测试默认配置
func TestHealthCheckSubscriber_DefaultConfig(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	
	// 检查默认值
	assert.NotEmpty(t, subscriber.config.Subscriber.Topic)
	assert.Equal(t, 2*time.Minute, subscriber.config.Publisher.Interval)
	assert.Equal(t, 10*time.Second, subscriber.config.Publisher.Timeout)
	assert.Equal(t, int32(3), subscriber.config.Publisher.FailureThreshold)
	assert.Equal(t, 5*time.Minute, subscriber.config.Publisher.MessageTTL)
}

// TestHealthCheckSubscriber_RegisterAlertCallback 测试注册告警回调
func TestHealthCheckSubscriber_RegisterAlertCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	
	callback := func(ctx context.Context, alert HealthCheckAlert) error {
		return nil
	}
	
	err = subscriber.RegisterAlertCallback(callback)
	assert.NoError(t, err)
	assert.Len(t, subscriber.alertCallbacks, 1)
}

// TestHealthCheckSubscriber_MultipleCallbacks 测试多个回调
func TestHealthCheckSubscriber_MultipleCallbacks(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	
	callback1 := func(ctx context.Context, alert HealthCheckAlert) error {
		return nil
	}
	
	callback2 := func(ctx context.Context, alert HealthCheckAlert) error {
		return nil
	}
	
	subscriber.RegisterAlertCallback(callback1)
	subscriber.RegisterAlertCallback(callback2)
	
	assert.Len(t, subscriber.alertCallbacks, 2)
}

// TestHealthCheckSubscriber_GetStats 测试获取统计信息
func TestHealthCheckSubscriber_GetStats(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	
	stats := subscriber.GetStats()
	assert.NotNil(t, stats)
	assert.False(t, stats.StartTime.IsZero())
}

// TestHealthCheckSubscriber_IsHealthy 测试健康状态检查
func TestHealthCheckSubscriber_IsHealthy(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	
	// 初始状态应该是健康的
	isHealthy := subscriber.IsHealthy()
	assert.True(t, isHealthy)
}

// TestHealthCheckAlert_Structure 测试告警结构
func TestHealthCheckAlert_Structure(t *testing.T) {
	alert := HealthCheckAlert{
		AlertType:         "no_messages",
		Severity:          "warning",
		Source:            "test-service",
		EventBusType:      "memory",
		Topic:             "health-check",
		LastMessageTime:   time.Now(),
		TimeSinceLastMsg:  5 * time.Minute,
		ExpectedInterval:  2 * time.Minute,
		ConsecutiveMisses: 2,
		Timestamp:         time.Now(),
		Metadata: map[string]string{
			"key": "value",
		},
	}

	assert.Equal(t, "no_messages", alert.AlertType)
	assert.Equal(t, "warning", alert.Severity)
	assert.Equal(t, "test-service", alert.Source)
	assert.Equal(t, 2, alert.ConsecutiveMisses)
}

// TestHealthCheckSubscriberStats_Structure 测试统计信息结构
func TestHealthCheckSubscriberStats_Structure(t *testing.T) {
	stats := HealthCheckSubscriberStats{
		StartTime:             time.Now(),
		LastMessageTime:       time.Now(),
		TotalMessagesReceived: 100,
		ConsecutiveMisses:     0,
		TotalAlerts:           5,
		LastAlertTime:         time.Now(),
		IsHealthy:             true,
		UptimeSeconds:         3600.0,
	}

	assert.Equal(t, int64(100), stats.TotalMessagesReceived)
	assert.Equal(t, int32(0), stats.ConsecutiveMisses)
	assert.Equal(t, int64(5), stats.TotalAlerts)
	assert.True(t, stats.IsHealthy)
}

// TestHealthCheckSubscriber_Start 测试启动订阅器
func TestHealthCheckSubscriber_Start(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Subscriber: config.HealthCheckSubscriberConfig{
			Topic: "health-check",
		},
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	
	ctx := context.Background()
	err = subscriber.Start(ctx)
	assert.NoError(t, err)
	
	// 等待一小段时间
	time.Sleep(100 * time.Millisecond)
	
	// 停止
	err = subscriber.Stop()
	assert.NoError(t, err)
}

// TestHealthCheckSubscriber_Start_AlreadyRunning 测试重复启动
func TestHealthCheckSubscriber_Start_AlreadyRunning(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	
	ctx := context.Background()
	err = subscriber.Start(ctx)
	assert.NoError(t, err)
	
	// 再次启动应该返回 nil（已经在运行）
	err = subscriber.Start(ctx)
	assert.NoError(t, err)
	
	subscriber.Stop()
}

// TestHealthCheckSubscriber_Stop_NotRunning 测试停止未运行的订阅器
func TestHealthCheckSubscriber_Stop_NotRunning(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	
	err = subscriber.Stop()
	assert.NoError(t, err)
}

// TestHealthCheckSubscriber_DifferentEventBusTypes 测试不同的EventBus类型
func TestHealthCheckSubscriber_DifferentEventBusTypes(t *testing.T) {
	tests := []struct {
		name         string
		eventBusType string
	}{
		{
			name:         "Memory",
			eventBusType: "memory",
		},
		{
			name:         "Kafka",
			eventBusType: "kafka",
		},
		{
			name:         "NATS",
			eventBusType: "nats",
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

			subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", tt.eventBusType)
			assert.Equal(t, tt.eventBusType, subscriber.eventBusType)
		})
	}
}

// TestHealthCheckSubscriber_CustomTopic 测试自定义主题
func TestHealthCheckSubscriber_CustomTopic(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	healthConfig := config.HealthCheckConfig{
		Enabled: true,
		Subscriber: config.HealthCheckSubscriberConfig{
			Topic: "custom-health-topic",
		},
	}

	subscriber := NewHealthCheckSubscriber(healthConfig, bus, "test-service", "memory")
	assert.Equal(t, "custom-health-topic", subscriber.config.Subscriber.Topic)
}

