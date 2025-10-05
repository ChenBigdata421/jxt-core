package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFactory_ValidateKafkaConfig_EmptyBrokers 测试验证 Kafka 配置（空 Brokers）
func TestFactory_ValidateKafkaConfig_EmptyBrokers(t *testing.T) {
	cfg := &EventBusConfig{
		Type:  "kafka",
		Kafka: KafkaConfig{},
	}

	factory := &Factory{config: cfg}
	err := factory.validateKafkaConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "brokers")
}

// TestFactory_ValidateKafkaConfig_WithBrokers 测试验证 Kafka 配置（有 Brokers）
func TestFactory_ValidateKafkaConfig_WithBrokers(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "kafka",
		Kafka: KafkaConfig{
			Brokers: []string{"localhost:9092"},
		},
	}

	factory := &Factory{config: cfg}
	err := factory.validateKafkaConfig()
	assert.NoError(t, err)

	// 验证默认值已设置
	assert.Equal(t, 5*time.Minute, cfg.Kafka.HealthCheckInterval)
	assert.Equal(t, 1, cfg.Kafka.Producer.RequiredAcks)
	assert.Equal(t, 500*time.Millisecond, cfg.Kafka.Producer.FlushFrequency)
	assert.Equal(t, 100, cfg.Kafka.Producer.FlushMessages)
	assert.Equal(t, 3, cfg.Kafka.Producer.RetryMax)
	assert.Equal(t, 10*time.Second, cfg.Kafka.Producer.Timeout)
	assert.Equal(t, "jxt-eventbus-group", cfg.Kafka.Consumer.GroupID)
	assert.Equal(t, "earliest", cfg.Kafka.Consumer.AutoOffsetReset)
	assert.Equal(t, 30*time.Second, cfg.Kafka.Consumer.SessionTimeout)
	assert.Equal(t, 3*time.Second, cfg.Kafka.Consumer.HeartbeatInterval)
}

// TestFactory_ValidateKafkaConfig_WithCustomValues 测试验证 Kafka 配置（自定义值）
func TestFactory_ValidateKafkaConfig_WithCustomValues(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "kafka",
		Kafka: KafkaConfig{
			Brokers:             []string{"localhost:9092"},
			HealthCheckInterval: 10 * time.Minute,
			Producer: ProducerConfig{
				RequiredAcks:   2,
				FlushFrequency: 1 * time.Second,
				FlushMessages:  200,
				RetryMax:       5,
				Timeout:        20 * time.Second,
			},
			Consumer: ConsumerConfig{
				GroupID:           "custom-group",
				AutoOffsetReset:   "latest",
				SessionTimeout:    60 * time.Second,
				HeartbeatInterval: 5 * time.Second,
			},
		},
	}

	factory := &Factory{config: cfg}
	err := factory.validateKafkaConfig()
	assert.NoError(t, err)

	// 验证自定义值未被覆盖
	assert.Equal(t, 10*time.Minute, cfg.Kafka.HealthCheckInterval)
	assert.Equal(t, 2, cfg.Kafka.Producer.RequiredAcks)
	assert.Equal(t, 1*time.Second, cfg.Kafka.Producer.FlushFrequency)
	assert.Equal(t, 200, cfg.Kafka.Producer.FlushMessages)
	assert.Equal(t, 5, cfg.Kafka.Producer.RetryMax)
	assert.Equal(t, 20*time.Second, cfg.Kafka.Producer.Timeout)
	assert.Equal(t, "custom-group", cfg.Kafka.Consumer.GroupID)
	assert.Equal(t, "latest", cfg.Kafka.Consumer.AutoOffsetReset)
	assert.Equal(t, 60*time.Second, cfg.Kafka.Consumer.SessionTimeout)
	assert.Equal(t, 5*time.Second, cfg.Kafka.Consumer.HeartbeatInterval)
}

// TestFactory_CreateEventBus_Memory_Coverage 测试创建 Memory EventBus
func TestFactory_CreateEventBus_Memory_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	factory := NewFactory(cfg)

	bus, err := factory.CreateEventBus()
	assert.NoError(t, err)
	assert.NotNil(t, bus)
	defer bus.Close()
}

// TestFactory_CreateEventBus_InvalidType 测试创建无效类型的 EventBus
func TestFactory_CreateEventBus_InvalidType(t *testing.T) {
	cfg := &EventBusConfig{Type: "invalid"}
	factory := NewFactory(cfg)

	bus, err := factory.CreateEventBus()
	assert.Error(t, err)
	assert.Nil(t, bus)
}

// TestFactory_CreateEventBus_Kafka_MissingBrokers 测试创建 Kafka EventBus（缺少 Brokers）
func TestFactory_CreateEventBus_Kafka_MissingBrokers(t *testing.T) {
	cfg := &EventBusConfig{
		Type:  "kafka",
		Kafka: KafkaConfig{},
	}
	factory := NewFactory(cfg)

	bus, err := factory.CreateEventBus()
	assert.Error(t, err)
	assert.Nil(t, bus)
	assert.Contains(t, err.Error(), "brokers")
}

// TestInitializeGlobal_Success 测试初始化全局 EventBus（成功）
func TestInitializeGlobal_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}

	err := InitializeGlobal(cfg)
	assert.NoError(t, err)

	// 验证全局实例已创建
	bus := GetGlobal()
	assert.NotNil(t, bus)

	// 清理
	CloseGlobal()
}

// TestInitializeGlobal_AlreadyInitialized_Coverage 测试初始化全局 EventBus（已初始化）
func TestInitializeGlobal_AlreadyInitialized_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}

	// 第一次初始化
	err := InitializeGlobal(cfg)
	require.NoError(t, err)

	// 第二次初始化（应该返回错误）
	err = InitializeGlobal(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already initialized")

	// 清理
	CloseGlobal()
}

// TestGetGlobal_NotInitialized_Coverage 测试获取全局 EventBus（未初始化）
func TestGetGlobal_NotInitialized_Coverage(t *testing.T) {
	// 确保全局实例未初始化
	CloseGlobal()

	bus := GetGlobal()
	assert.Nil(t, bus)
}

// TestGetGlobal_Initialized_Coverage 测试获取全局 EventBus（已初始化）
func TestGetGlobal_Initialized_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}

	err := InitializeGlobal(cfg)
	require.NoError(t, err)

	bus := GetGlobal()
	assert.NotNil(t, bus)

	// 清理
	CloseGlobal()
}

// TestCloseGlobal_Success 测试关闭全局 EventBus（成功）
func TestCloseGlobal_Success(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}

	err := InitializeGlobal(cfg)
	require.NoError(t, err)

	err = CloseGlobal()
	assert.NoError(t, err)

	// 验证全局实例已清空
	bus := GetGlobal()
	assert.Nil(t, bus)
}

// TestCloseGlobal_NotInitialized_Coverage 测试关闭全局 EventBus（未初始化）
func TestCloseGlobal_NotInitialized_Coverage(t *testing.T) {
	// 确保全局实例未初始化
	CloseGlobal()

	err := CloseGlobal()
	assert.NoError(t, err)
}

// TestCloseGlobal_Multiple 测试多次关闭全局 EventBus
func TestCloseGlobal_Multiple(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}

	err := InitializeGlobal(cfg)
	require.NoError(t, err)

	// 第一次关闭
	err = CloseGlobal()
	assert.NoError(t, err)

	// 第二次关闭（应该不会出错）
	err = CloseGlobal()
	assert.NoError(t, err)
}

// TestNewFactory_Coverage 测试创建工厂
func TestNewFactory_Coverage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	factory := NewFactory(cfg)

	assert.NotNil(t, factory)
	assert.Equal(t, cfg, factory.config)
}

// TestFactory_CreateEventBus_WithMetrics 测试创建带指标的 EventBus
func TestFactory_CreateEventBus_WithMetrics(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "memory",
		Metrics: MetricsConfig{
			Enabled:         true,
			CollectInterval: 10 * time.Second,
		},
	}
	factory := NewFactory(cfg)

	bus, err := factory.CreateEventBus()
	assert.NoError(t, err)
	assert.NotNil(t, bus)
	defer bus.Close()

	// 验证指标功能
	metrics := bus.GetMetrics()
	assert.NotNil(t, metrics)
}

// TestFactory_CreateEventBus_WithTracing 测试创建带追踪的 EventBus
func TestFactory_CreateEventBus_WithTracing(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "memory",
		Tracing: TracingConfig{
			Enabled:     true,
			ServiceName: "test-service",
			SampleRate:  1.0,
		},
	}
	factory := NewFactory(cfg)

	bus, err := factory.CreateEventBus()
	assert.NoError(t, err)
	assert.NotNil(t, bus)
	defer bus.Close()
}

// TestFactory_ValidateKafkaConfig_PartialDefaults 测试验证 Kafka 配置（部分默认值）
func TestFactory_ValidateKafkaConfig_PartialDefaults(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "kafka",
		Kafka: KafkaConfig{
			Brokers:             []string{"localhost:9092"},
			HealthCheckInterval: 10 * time.Minute,
			Producer: ProducerConfig{
				RequiredAcks: 2,
				// 其他字段使用默认值
			},
			Consumer: ConsumerConfig{
				GroupID: "custom-group",
				// 其他字段使用默认值
			},
		},
	}

	factory := &Factory{config: cfg}
	err := factory.validateKafkaConfig()
	assert.NoError(t, err)

	// 验证自定义值
	assert.Equal(t, 10*time.Minute, cfg.Kafka.HealthCheckInterval)
	assert.Equal(t, 2, cfg.Kafka.Producer.RequiredAcks)
	assert.Equal(t, "custom-group", cfg.Kafka.Consumer.GroupID)

	// 验证默认值
	assert.Equal(t, 500*time.Millisecond, cfg.Kafka.Producer.FlushFrequency)
	assert.Equal(t, 100, cfg.Kafka.Producer.FlushMessages)
	assert.Equal(t, 3, cfg.Kafka.Producer.RetryMax)
	assert.Equal(t, 10*time.Second, cfg.Kafka.Producer.Timeout)
	assert.Equal(t, "earliest", cfg.Kafka.Consumer.AutoOffsetReset)
	assert.Equal(t, 30*time.Second, cfg.Kafka.Consumer.SessionTimeout)
	assert.Equal(t, 3*time.Second, cfg.Kafka.Consumer.HeartbeatInterval)
}
