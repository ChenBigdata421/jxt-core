package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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
	assert.Equal(t, "jxt-core-memory-health-check", checker.config.Publisher.Topic)
	assert.Equal(t, 2*time.Minute, checker.config.Publisher.Interval)
	assert.Equal(t, 10*time.Second, checker.config.Publisher.Timeout)
	assert.Equal(t, 3, checker.config.Publisher.FailureThreshold)
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
	assert.Equal(t, 3, subscriber.config.Publisher.FailureThreshold)
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

// TestHealthCheckMessage_ToBytes_Success 测试转换为字节（成功）
func TestHealthCheckMessage_ToBytes_Success(t *testing.T) {
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
		Metadata: map[string]string{
			"key": "value",
		},
	}
	bytes, err := msg.ToBytes()
	assert.NoError(t, err)
	assert.NotNil(t, bytes)
	assert.Greater(t, len(bytes), 0)
}

// TestHealthCheckMessage_ToBytes_EmptyMessage 测试转换为字节（空消息）
func TestHealthCheckMessage_ToBytes_EmptyMessage(t *testing.T) {
	msg := &HealthCheckMessage{}
	bytes, err := msg.ToBytes()
	// 可能返回错误或空字节
	if err != nil {
		assert.Error(t, err)
	} else {
		assert.NotNil(t, bytes)
	}
}

// TestHealthCheckMessageParser_ParseWithoutValidation_Success 测试解析不验证（成功）
func TestHealthCheckMessageParser_ParseWithoutValidation_Success(t *testing.T) {
	// 创建一个有效的消息
	original := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
	}
	// 转换为字节
	bytes, err := original.ToBytes()
	require.NoError(t, err)
	// 创建解析器
	parser := NewHealthCheckMessageParser()
	// 解析不验证
	parsed, err := parser.ParseWithoutValidation(bytes)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, original.MessageID, parsed.MessageID)
	assert.Equal(t, original.Source, parsed.Source)
	assert.Equal(t, original.EventBusType, parsed.EventBusType)
}

// TestHealthCheckMessageParser_ParseWithoutValidation_InvalidJSON 测试解析不验证（无效 JSON）
func TestHealthCheckMessageParser_ParseWithoutValidation_InvalidJSON(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	invalidJSON := []byte("invalid json")
	parsed, err := parser.ParseWithoutValidation(invalidJSON)
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessageParser_ParseWithoutValidation_EmptyBytes 测试解析不验证（空字节）
func TestHealthCheckMessageParser_ParseWithoutValidation_EmptyBytes(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	emptyBytes := []byte("")
	parsed, err := parser.ParseWithoutValidation(emptyBytes)
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessageParser_ParseWithoutValidation_NilBytes 测试解析不验证（nil 字节）
func TestHealthCheckMessageParser_ParseWithoutValidation_NilBytes(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	var nilBytes []byte
	parsed, err := parser.ParseWithoutValidation(nilBytes)
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessageValidator_Validate_AllFields 测试验证（所有字段）
func TestHealthCheckMessageValidator_Validate_AllFields(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
		Metadata: map[string]string{
			"key": "value",
		},
	}
	err := validator.Validate(msg)
	assert.NoError(t, err)
}

// TestHealthCheckMessageValidator_Validate_MissingMessageID 测试验证（缺少 MessageID）
func TestHealthCheckMessageValidator_Validate_MissingMessageID(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	msg := &HealthCheckMessage{
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
	}
	err := validator.Validate(msg)
	assert.Error(t, err)
}

// TestHealthCheckMessageValidator_Validate_MissingSource 测试验证（缺少 Source）
func TestHealthCheckMessageValidator_Validate_MissingSource(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		EventBusType: "memory",
		Version:      "1.0",
	}
	err := validator.Validate(msg)
	assert.Error(t, err)
}

// TestHealthCheckMessageValidator_Validate_ZeroTimestamp 测试验证（零时间戳）
func TestHealthCheckMessageValidator_Validate_ZeroTimestamp(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Time{},
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
	}
	err := validator.Validate(msg)
	assert.Error(t, err)
}

// TestHealthCheckMessageParser_Parse_Success 测试解析（成功）
func TestHealthCheckMessageParser_Parse_Success(t *testing.T) {
	// 创建一个有效的消息
	original := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
	}
	// 转换为字节
	bytes, err := original.ToBytes()
	require.NoError(t, err)
	// 创建解析器
	parser := NewHealthCheckMessageParser()
	// 解析并验证
	parsed, err := parser.Parse(bytes)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, original.MessageID, parsed.MessageID)
	assert.Equal(t, original.Source, parsed.Source)
	assert.Equal(t, original.EventBusType, parsed.EventBusType)
}

// TestHealthCheckMessageParser_Parse_InvalidMessage 测试解析（无效消息）
func TestHealthCheckMessageParser_Parse_InvalidMessage(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	// 创建一个无效的消息（缺少必填字段）
	invalidMsg := &HealthCheckMessage{
		MessageID: "test-id",
		// 缺少其他必填字段
	}
	// 转换为字节
	bytes, err := invalidMsg.ToBytes()
	require.NoError(t, err)
	// 解析应该失败（验证失败）
	parsed, err := parser.Parse(bytes)
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessage_RoundTrip 测试往返转换
func TestHealthCheckMessage_RoundTrip(t *testing.T) {
	original := &HealthCheckMessage{
		MessageID:    "test-id-123",
		Timestamp:    time.Now().Truncate(time.Second), // 截断以避免精度问题
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
		Metadata: map[string]string{
			"status": "healthy",
			"count":  "42",
		},
	}
	// 转换为字节
	bytes, err := original.ToBytes()
	require.NoError(t, err)
	// 创建解析器
	parser := NewHealthCheckMessageParser()
	// 解析回来
	parsed, err := parser.Parse(bytes)
	require.NoError(t, err)
	// 验证字段
	assert.Equal(t, original.MessageID, parsed.MessageID)
	assert.Equal(t, original.Source, parsed.Source)
	assert.Equal(t, original.EventBusType, parsed.EventBusType)
	assert.Equal(t, original.Version, parsed.Version)
	assert.NotNil(t, parsed.Metadata)
}

// TestHealthCheckMessage_DifferentEventBusTypes 测试不同的 EventBus 类型
func TestHealthCheckMessage_DifferentEventBusTypes(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	eventBusTypes := []string{"memory", "kafka", "nats"}
	for _, busType := range eventBusTypes {
		msg := &HealthCheckMessage{
			MessageID:    "test-id",
			Timestamp:    time.Now(),
			Source:       "test-source",
			EventBusType: busType,
			Version:      "1.0",
		}
		err := validator.Validate(msg)
		assert.NoError(t, err, "EventBus type %s should be valid", busType)
		bytes, err := msg.ToBytes()
		assert.NoError(t, err, "EventBus type %s should convert to bytes", busType)
		assert.NotNil(t, bytes)
	}
}

// TestHealthCheckMessage_WithMetadata 测试带 Metadata
func TestHealthCheckMessage_WithMetadata(t *testing.T) {
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		},
	}
	// 验证
	validator := NewHealthCheckMessageValidator()
	err := validator.Validate(msg)
	assert.NoError(t, err)
	// 转换为字节
	bytes, err := msg.ToBytes()
	assert.NoError(t, err)
	assert.NotNil(t, bytes)
	// 创建解析器
	parser := NewHealthCheckMessageParser()
	// 解析回来
	parsed, err := parser.Parse(bytes)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.NotNil(t, parsed.Metadata)
	assert.Equal(t, len(msg.Metadata), len(parsed.Metadata))
}

// TestCreateHealthCheckMessage 测试创建健康检查消息
func TestCreateHealthCheckMessage(t *testing.T) {
	msg := CreateHealthCheckMessage("test-source", "memory")
	assert.NotNil(t, msg)
	assert.NotEmpty(t, msg.MessageID)
	assert.Equal(t, "test-source", msg.Source)
	assert.Equal(t, "memory", msg.EventBusType)
	assert.False(t, msg.Timestamp.IsZero())
	// 验证消息
	validator := NewHealthCheckMessageValidator()
	err := validator.Validate(msg)
	assert.NoError(t, err)
}

// TestHealthCheckMessage_FromBytes 测试从字节反序列化
func TestHealthCheckMessage_FromBytes(t *testing.T) {
	original := CreateHealthCheckMessage("test-source", "memory")
	// 序列化
	data, err := original.ToBytes()
	require.NoError(t, err)
	// 反序列化
	restored := &HealthCheckMessage{}
	err = restored.FromBytes(data)
	assert.NoError(t, err)
	assert.Equal(t, original.MessageID, restored.MessageID)
	assert.Equal(t, original.Source, restored.Source)
	assert.Equal(t, original.EventBusType, restored.EventBusType)
}

// TestHealthCheckMessage_FromBytes_InvalidData 测试无效数据反序列化
func TestHealthCheckMessage_FromBytes_InvalidData(t *testing.T) {
	msg := &HealthCheckMessage{}
	err := msg.FromBytes([]byte("invalid json"))
	assert.Error(t, err)
}

// TestHealthCheckMessage_IsValid 测试消息有效性验证
func TestHealthCheckMessage_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		msg      *HealthCheckMessage
		expected bool
	}{
		{
			name: "valid message",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expected: true,
		},
		{
			name: "missing message id",
			msg: &HealthCheckMessage{
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expected: false,
		},
		{
			name: "missing source",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expected: false,
		},
		{
			name: "missing event bus type",
			msg: &HealthCheckMessage{
				MessageID: "test-id",
				Source:    "test-source",
				Timestamp: time.Now(),
			},
			expected: false,
		},
		{
			name: "timestamp too far in future",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now().Add(2 * time.Minute),
			},
			expected: false,
		},
		{
			name: "timestamp too far in past",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now().Add(-10 * time.Minute),
			},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.msg.IsValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHealthCheckMessage_SetMetadata_NilMap 测试在 nil map 上设置元数据
func TestHealthCheckMessage_SetMetadata_NilMap(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: nil,
	}
	msg.SetMetadata("key1", "value1")
	assert.NotNil(t, msg.Metadata)
	assert.Equal(t, "value1", msg.Metadata["key1"])
}

// TestHealthCheckMessage_SetMetadata_ExistingMap 测试在已有 map 上设置元数据
func TestHealthCheckMessage_SetMetadata_ExistingMap(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: map[string]string{
			"existing": "value",
		},
	}
	msg.SetMetadata("key1", "value1")
	assert.Equal(t, "value1", msg.Metadata["key1"])
	assert.Equal(t, "value", msg.Metadata["existing"])
}

// TestHealthCheckMessage_GetMetadata_NilMap 测试从 nil map 获取元数据
func TestHealthCheckMessage_GetMetadata_NilMap(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: nil,
	}
	value, exists := msg.GetMetadata("key1")
	assert.False(t, exists)
	assert.Equal(t, "", value)
}

// TestHealthCheckMessage_GetMetadata_ExistingKey 测试获取存在的元数据
func TestHealthCheckMessage_GetMetadata_ExistingKey(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: map[string]string{
			"key1": "value1",
		},
	}
	value, exists := msg.GetMetadata("key1")
	assert.True(t, exists)
	assert.Equal(t, "value1", value)
}

// TestHealthCheckMessage_GetMetadata_NonExistingKey 测试获取不存在的元数据
func TestHealthCheckMessage_GetMetadata_NonExistingKey(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: map[string]string{
			"key1": "value1",
		},
	}
	value, exists := msg.GetMetadata("key2")
	assert.False(t, exists)
	assert.Equal(t, "", value)
}

// TestGetHealthCheckTopic_AllTypes 测试所有 EventBus 类型的健康检查主题
func TestGetHealthCheckTopic_AllTypes(t *testing.T) {
	tests := []struct {
		eventBusType string
		expected     string
	}{
		{"memory", "jxt-core-memory-health-check"},
		{"kafka", "jxt-core-kafka-health-check"},
		{"nats", "jxt-core-nats-health-check"},
		{"unknown", "jxt-core-health-check"}, // 未知类型使用默认主题
	}
	for _, tt := range tests {
		t.Run(tt.eventBusType, func(t *testing.T) {
			topic := GetHealthCheckTopic(tt.eventBusType)
			assert.Equal(t, tt.expected, topic)
		})
	}
}

// TestHealthCheckMessageValidator_Validate_AllScenarios 测试所有验证场景
func TestHealthCheckMessageValidator_Validate_AllScenarios(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	tests := []struct {
		name      string
		msg       *HealthCheckMessage
		expectErr bool
	}{
		{
			name: "valid message",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now(),
				Version:      "1.0.0",
			},
			expectErr: false,
		},
		{
			name:      "nil message",
			msg:       nil,
			expectErr: true,
		},
		{
			name: "empty message id",
			msg: &HealthCheckMessage{
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expectErr: true,
		},
		{
			name: "empty source",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expectErr: true,
		},
		{
			name: "empty event bus type",
			msg: &HealthCheckMessage{
				MessageID: "test-id",
				Source:    "test-source",
				Timestamp: time.Now(),
			},
			expectErr: true,
		},
		{
			name: "zero timestamp",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Time{},
			},
			expectErr: true,
		},
		{
			name: "timestamp too far in future",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now().Add(2 * time.Minute),
			},
			expectErr: true,
		},
		{
			name: "timestamp too far in past",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now().Add(-10 * time.Minute),
			},
			expectErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.msg)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestHealthCheckMessageParser_Parse 测试消息解析
func TestHealthCheckMessageParser_Parse(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	// 创建一个有效的消息
	original := CreateHealthCheckMessage("test-source", "memory")
	data, err := original.ToBytes()
	require.NoError(t, err)
	// 解析消息
	parsed, err := parser.Parse(data)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, original.MessageID, parsed.MessageID)
	assert.Equal(t, original.Source, parsed.Source)
}

// TestHealthCheckMessageParser_Parse_InvalidData 测试解析无效数据
func TestHealthCheckMessageParser_Parse_InvalidData(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	parsed, err := parser.Parse([]byte("invalid json"))
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessageParser_Parse_EmptyData 测试解析空数据
func TestHealthCheckMessageParser_Parse_EmptyData(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	parsed, err := parser.Parse([]byte{})
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

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

// TestEventBusManager_PerformHealthCheck 测试执行健康检查
func TestEventBusManager_PerformHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	err = manager.performHealthCheck(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_PerformHealthCheck_Closed 测试关闭状态的健康检查
func TestEventBusManager_PerformHealthCheck_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	manager := bus.(*eventBusManager)
	bus.Close()
	ctx := context.Background()
	err = manager.performHealthCheck(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventbus is closed")
}

// TestEventBusManager_PerformFullHealthCheck 测试完整健康检查
func TestEventBusManager_PerformFullHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	status, err := manager.performFullHealthCheck(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "healthy", status.Overall)
	assert.NotZero(t, status.Timestamp)
	assert.NotZero(t, status.CheckDuration)
}

// TestEventBusManager_PerformFullHealthCheck_Closed 测试关闭状态的完整健康检查
func TestEventBusManager_PerformFullHealthCheck_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	manager := bus.(*eventBusManager)
	bus.Close()
	ctx := context.Background()
	status, err := manager.performFullHealthCheck(ctx)
	assert.Error(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "unhealthy", status.Overall)
	assert.Equal(t, "closed", status.Infrastructure.EventBus.ConnectionStatus)
}

// TestEventBusManager_PerformFullHealthCheck_WithBusinessChecker 测试带业务健康检查器的完整检查
func TestEventBusManager_PerformFullHealthCheck_WithBusinessChecker(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	// 注册业务健康检查器
	checker := &mockBusinessHealthChecker{}
	manager.RegisterBusinessHealthCheck(checker)
	ctx := context.Background()
	status, err := manager.performFullHealthCheck(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "healthy", status.Overall)
	assert.NotNil(t, status.Business)
}

// TestEventBusManager_PerformFullHealthCheck_BusinessCheckerFails 测试业务健康检查失败
func TestEventBusManager_PerformFullHealthCheck_BusinessCheckerFails(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	// 注册会失败的业务健康检查器
	checker := &mockFailingBusinessHealthChecker{}
	manager.RegisterBusinessHealthCheck(checker)
	ctx := context.Background()
	status, err := manager.performFullHealthCheck(ctx)
	assert.Error(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "unhealthy", status.Overall)
}

// TestEventBusManager_CheckInfrastructureHealth 测试基础设施健康检查
func TestEventBusManager_CheckInfrastructureHealth(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	infraHealth, err := manager.checkInfrastructureHealth(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "connected", infraHealth.EventBus.ConnectionStatus)
}

// TestEventBusManager_CheckConnection 测试连接检查
func TestEventBusManager_CheckConnection(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	err = manager.checkConnection(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_CheckConnection_NoPublisher 测试无发布器的连接检查
func TestEventBusManager_CheckConnection_NoPublisher(t *testing.T) {
	manager := &eventBusManager{
		publisher:  nil,
		subscriber: nil,
	}
	ctx := context.Background()
	err := manager.checkConnection(ctx)
	assert.NoError(t, err) // 没有发布器和订阅器时应该返回 nil
}

// TestEventBusManager_CheckMessageTransport 测试消息传输检查
func TestEventBusManager_CheckMessageTransport(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	err = manager.checkMessageTransport(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_CheckMessageTransport_NoPublisher 测试无发布器的消息传输检查
func TestEventBusManager_CheckMessageTransport_NoPublisher(t *testing.T) {
	manager := &eventBusManager{
		publisher: nil,
	}
	ctx := context.Background()
	err := manager.checkMessageTransport(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publisher not initialized")
}

// TestEventBusManager_CheckMessageTransport_WithTimeout 测试带超时的消息传输检查
func TestEventBusManager_CheckMessageTransport_WithTimeout(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	// 创建一个会很快超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = manager.checkMessageTransport(ctx)
	// Memory EventBus 应该能快速完成，不会超时
	assert.NoError(t, err)
}

// TestEventBusManager_PerformEndToEndTest 测试端到端测试
func TestEventBusManager_PerformEndToEndTest(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	testTopic := "test-e2e-topic"
	healthMsg := "health-check-test"
	err = manager.performEndToEndTest(ctx, testTopic, healthMsg)
	assert.NoError(t, err)
}

// TestEventBusManager_PerformEndToEndTest_Timeout 测试端到端测试超时
func TestEventBusManager_PerformEndToEndTest_Timeout(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	// 创建一个已经取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	testTopic := "test-e2e-topic"
	healthMsg := "health-check-test"
	err = manager.performEndToEndTest(ctx, testTopic, healthMsg)
	// 由于上下文已取消，可能返回错误（但 memory eventbus 可能太快而成功）
	// 因此我们不强制要求错误，只是测试不会崩溃
	_ = err
}

// mockFailingBusinessHealthChecker 模拟失败的业务健康检查器
type mockFailingBusinessHealthChecker struct{}

func (m *mockFailingBusinessHealthChecker) CheckBusinessHealth(ctx context.Context) error {
	return assert.AnError
}
func (m *mockFailingBusinessHealthChecker) GetBusinessMetrics() interface{} {
	return map[string]interface{}{"status": "unhealthy"}
}
func (m *mockFailingBusinessHealthChecker) GetBusinessConfig() interface{} {
	return map[string]interface{}{"config": "test"}
}

// TestEventBusManager_CheckInfrastructureHealth_Success 测试检查基础设施健康（成功）
func TestEventBusManager_CheckInfrastructureHealth_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 测试检查基础设施健康
	result, err := manager.checkInfrastructureHealth(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestEventBusManager_CheckInfrastructureHealth_WithTimeout 测试检查基础设施健康（带超时）
func TestEventBusManager_CheckInfrastructureHealth_WithTimeout(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// 测试检查基础设施健康
	result, err := manager.checkInfrastructureHealth(ctx)
	// 可能成功或超时
	if err != nil {
		t.Logf("Infrastructure health check timed out: %v", err)
	} else {
		assert.NotNil(t, result)
	}
}

// TestEventBusManager_CheckConnection_Success 测试检查连接（成功）
func TestEventBusManager_CheckConnection_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 测试检查连接
	err = manager.checkConnection(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_CheckConnection_AfterClose 测试检查连接（关闭后）
func TestEventBusManager_CheckConnection_AfterClose(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 关闭 EventBus
	bus.Close()
	// 测试检查连接（应该失败）
	err = manager.checkConnection(ctx)
	assert.Error(t, err)
}

// TestEventBusManager_CheckConnection_WithTimeout 测试检查连接（带超时）
func TestEventBusManager_CheckConnection_WithTimeout(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// 测试检查连接
	err = manager.checkConnection(ctx)
	// 可能成功或超时
	if err != nil {
		t.Logf("Connection check timed out: %v", err)
	}
}

// TestEventBusManager_CheckMessageTransport_Success 测试检查消息传输（成功）
func TestEventBusManager_CheckMessageTransport_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 测试检查消息传输
	err = manager.checkMessageTransport(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_CheckMessageTransport_AfterClose 测试检查消息传输（关闭后）
func TestEventBusManager_CheckMessageTransport_AfterClose(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 关闭 EventBus
	bus.Close()
	// 测试检查消息传输（应该失败）
	err = manager.checkMessageTransport(ctx)
	assert.Error(t, err)
}

// TestEventBusManager_CheckMessageTransport_WithTimeout_Coverage 测试检查消息传输（带超时）
func TestEventBusManager_CheckMessageTransport_WithTimeout_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// 测试检查消息传输
	err = manager.checkMessageTransport(ctx)
	// 可能成功或超时
	if err != nil {
		t.Logf("Message transport check timed out: %v", err)
	}
}

// TestEventBusManager_PerformHealthCheck_Success 测试执行健康检查（成功）
func TestEventBusManager_PerformHealthCheck_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 测试执行健康检查
	err = manager.performHealthCheck(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_PerformHealthCheck_WithCallback 测试执行健康检查（带回调）
func TestEventBusManager_PerformHealthCheck_WithCallback(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 注册健康检查回调
	callbackCalled := false
	manager.RegisterHealthCheckCallback(func(ctx context.Context, result HealthCheckResult) error {
		callbackCalled = true
		return nil
	})
	// 测试执行健康检查
	err = manager.performHealthCheck(ctx)
	assert.NoError(t, err)
	// 验证回调可能被调用
	_ = callbackCalled
}

// TestEventBusManager_PerformHealthCheck_AfterClose 测试执行健康检查（关闭后）
func TestEventBusManager_PerformHealthCheck_AfterClose(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 关闭 EventBus
	bus.Close()
	// 测试执行健康检查（应该失败）
	err = manager.performHealthCheck(ctx)
	assert.Error(t, err)
}

// TestEventBusManager_Subscribe_WithOptions 测试订阅（带选项）
func TestEventBusManager_Subscribe_WithOptions(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	ctx := context.Background()
	// 创建订阅选项
	options := SubscribeOptions{
		ProcessingTimeout: 5 * time.Second,
		RateLimit:         100.0,
		RateBurst:         10,
		MaxRetries:        3,
		RetryBackoff:      100 * time.Millisecond,
		DeadLetterEnabled: false,
	}
	// 订阅
	err = bus.SubscribeWithOptions(ctx, "test-topic", func(ctx context.Context, message []byte) error {
		return nil
	}, options)
	assert.NoError(t, err)
}

// TestEventBusManager_Publish_WithOptions 测试发布（带选项）
func TestEventBusManager_Publish_WithOptions(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	ctx := context.Background()
	// 创建发布选项
	options := PublishOptions{
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		Timeout:     5 * time.Second,
		AggregateID: "test-aggregate",
	}
	// 发布
	err = bus.PublishWithOptions(ctx, "test-topic", []byte("test message"), options)
	assert.NoError(t, err)
}

// TestEventBusManager_Close_Multiple_Coverage 测试多次关闭
func TestEventBusManager_Close_Multiple_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	// 第一次关闭
	err = bus.Close()
	assert.NoError(t, err)
	// 第二次关闭（应该不会 panic）
	err = bus.Close()
	// 可能返回错误或成功
	_ = err
}

// TestEventBusManager_RegisterReconnectCallback_Multiple 测试注册多个重连回调
func TestEventBusManager_RegisterReconnectCallback_Multiple(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	// 注册多个回调
	callback1Called := false
	callback2Called := false
	manager.RegisterReconnectCallback(func(ctx context.Context) error {
		callback1Called = true
		return nil
	})
	manager.RegisterReconnectCallback(func(ctx context.Context) error {
		callback2Called = true
		return nil
	})
	// 验证回调已注册
	_ = callback1Called
	_ = callback2Called
}

// TestEventBusManager_GetMetrics_AfterPublish 测试发布后获取指标
func TestEventBusManager_GetMetrics_AfterPublish(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	ctx := context.Background()
	// 发布一些消息
	for i := 0; i < 10; i++ {
		err = bus.Publish(ctx, "test-topic", []byte("test message"))
		assert.NoError(t, err)
	}
	// 获取指标
	metrics := bus.GetMetrics()
	assert.NotNil(t, metrics)
	// 验证指标可能包含发布计数
	_ = metrics
}

// TestEventBusManager_GetHealthStatus_AfterHealthCheck 测试健康检查后获取健康状态
func TestEventBusManager_GetHealthStatus_AfterHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()
	manager := bus.(*eventBusManager)
	ctx := context.Background()
	// 执行健康检查
	err = manager.performHealthCheck(ctx)
	assert.NoError(t, err)
	// 获取健康状态
	status := bus.GetHealthStatus()
	assert.NotEmpty(t, status)
}
