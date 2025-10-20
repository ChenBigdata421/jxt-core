package eventbus

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// TestNewFactory 测试创建工厂
func TestNewFactory(t *testing.T) {
	config := &EventBusConfig{
		Type: "memory",
	}

	factory := NewFactory(config)
	assert.NotNil(t, factory)
	assert.Equal(t, config, factory.config)
}

// TestFactory_CreateEventBus_NilConfig 测试 nil 配置
func TestFactory_CreateEventBus_NilConfig(t *testing.T) {
	factory := &Factory{config: nil}

	bus, err := factory.CreateEventBus()
	assert.Error(t, err)
	assert.Nil(t, bus)
	assert.Contains(t, err.Error(), "config is required")
}

// TestFactory_CreateEventBus_EmptyType 测试空类型
func TestFactory_CreateEventBus_EmptyType(t *testing.T) {
	config := &EventBusConfig{
		Type: "",
	}
	factory := NewFactory(config)

	bus, err := factory.CreateEventBus()
	assert.Error(t, err)
	assert.Nil(t, bus)
	assert.Contains(t, err.Error(), "type is required")
}

// TestFactory_CreateEventBus_UnsupportedType 测试不支持的类型
func TestFactory_CreateEventBus_UnsupportedType(t *testing.T) {
	config := &EventBusConfig{
		Type: "redis",
	}
	factory := NewFactory(config)

	bus, err := factory.CreateEventBus()
	assert.Error(t, err)
	assert.Nil(t, bus)
	assert.Contains(t, err.Error(), "unsupported eventbus type")
}

// TestFactory_CreateEventBus_Memory 测试创建内存 EventBus
func TestFactory_CreateEventBus_Memory(t *testing.T) {
	config := &EventBusConfig{
		Type: "memory",
	}
	factory := NewFactory(config)

	bus, err := factory.CreateEventBus()
	assert.NoError(t, err)
	assert.NotNil(t, bus)
	defer bus.Close()
}

// TestFactory_ValidateKafkaConfig_NoBrokers 测试 Kafka 配置验证 - 无 brokers
func TestFactory_ValidateKafkaConfig_NoBrokers(t *testing.T) {
	config := &EventBusConfig{
		Type: "kafka",
		Kafka: KafkaConfig{
			Brokers: []string{},
		},
	}
	factory := NewFactory(config)

	bus, err := factory.CreateEventBus()
	assert.Error(t, err)
	assert.Nil(t, bus)
	assert.Contains(t, err.Error(), "brokers are required")
}

// TestFactory_ValidateNATSConfig_NoURLs 测试 NATS 配置验证 - 无 URLs
func TestFactory_ValidateNATSConfig_NoURLs(t *testing.T) {
	config := &EventBusConfig{
		Type: "nats",
		NATS: NATSConfig{
			URLs: []string{},
		},
	}
	factory := NewFactory(config)

	bus, err := factory.CreateEventBus()
	assert.Error(t, err)
	assert.Nil(t, bus)
	assert.Contains(t, err.Error(), "urls are required")
}

// TestFactory_ValidateNATSConfig_Defaults 测试 NATS 配置默认值
func TestFactory_ValidateNATSConfig_Defaults(t *testing.T) {
	config := &EventBusConfig{
		Type: "nats",
		NATS: NATSConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}
	factory := NewFactory(config)

	err := factory.validateConfig()
	assert.NoError(t, err)

	// 检查默认值
	assert.Equal(t, "jxt-client", config.NATS.ClientID)
	assert.Equal(t, 10, config.NATS.MaxReconnects)
	assert.Equal(t, 2*time.Second, config.NATS.ReconnectWait)
	assert.Equal(t, 10*time.Second, config.NATS.ConnectionTimeout)
	assert.Equal(t, 5*time.Minute, config.NATS.HealthCheckInterval)
}

// TestFactory_ValidateMemoryConfig 测试内存配置验证
func TestFactory_ValidateMemoryConfig(t *testing.T) {
	config := &EventBusConfig{
		Type: "memory",
	}
	factory := NewFactory(config)

	err := factory.validateConfig()
	assert.NoError(t, err)
}

// TestGetDefaultConfig_Kafka 测试获取 Kafka 默认配置
func TestGetDefaultConfig_Kafka(t *testing.T) {
	config := GetDefaultConfig("kafka")

	assert.NotNil(t, config)
	assert.Equal(t, "kafka", config.Type)
	assert.Equal(t, []string{"localhost:9092"}, config.Kafka.Brokers)
	assert.True(t, config.Metrics.Enabled)
	assert.False(t, config.Tracing.Enabled)
}

// TestGetDefaultConfig_NATS 测试获取 NATS 默认配置
func TestGetDefaultConfig_NATS(t *testing.T) {
	config := GetDefaultConfig("nats")

	assert.NotNil(t, config)
	assert.Equal(t, "nats", config.Type)
	assert.Equal(t, []string{"nats://localhost:4222"}, config.NATS.URLs)
	assert.Equal(t, "jxt-client", config.NATS.ClientID)
}

// TestGetDefaultConfig_Memory 测试获取内存默认配置
func TestGetDefaultConfig_Memory(t *testing.T) {
	config := GetDefaultConfig("memory")

	assert.NotNil(t, config)
	assert.Equal(t, "memory", config.Type)
}

// TestGetDefaultPersistentNATSConfig 测试获取持久化 NATS 配置
func TestGetDefaultPersistentNATSConfig(t *testing.T) {
	urls := []string{"nats://server1:4222", "nats://server2:4222"}
	clientID := "test-client"

	config := GetDefaultPersistentNATSConfig(urls, clientID)

	assert.NotNil(t, config)
	assert.Equal(t, "nats", config.Type)
	assert.Equal(t, urls, config.NATS.URLs)
	assert.Equal(t, clientID, config.NATS.ClientID)
	assert.True(t, config.NATS.JetStream.Enabled)
	assert.Equal(t, "file", config.NATS.JetStream.Stream.Storage)
}

// TestGetDefaultPersistentNATSConfig_EmptyParams 测试空参数的持久化 NATS 配置
func TestGetDefaultPersistentNATSConfig_EmptyParams(t *testing.T) {
	config := GetDefaultPersistentNATSConfig(nil, "")

	assert.NotNil(t, config)
	assert.Equal(t, []string{"nats://localhost:4222"}, config.NATS.URLs)
	assert.Equal(t, "jxt-persistent-client", config.NATS.ClientID)
}

// TestGetDefaultEphemeralNATSConfig 测试获取非持久化 NATS 配置
func TestGetDefaultEphemeralNATSConfig(t *testing.T) {
	urls := []string{"nats://server1:4222"}
	clientID := "ephemeral-client"

	config := GetDefaultEphemeralNATSConfig(urls, clientID)

	assert.NotNil(t, config)
	assert.Equal(t, "nats", config.Type)
	assert.Equal(t, urls, config.NATS.URLs)
	assert.Equal(t, clientID, config.NATS.ClientID)
	assert.False(t, config.NATS.JetStream.Enabled)
}

// TestGetDefaultEphemeralNATSConfig_EmptyParams 测试空参数的非持久化 NATS 配置
func TestGetDefaultEphemeralNATSConfig_EmptyParams(t *testing.T) {
	config := GetDefaultEphemeralNATSConfig(nil, "")

	assert.NotNil(t, config)
	assert.Equal(t, []string{"nats://localhost:4222"}, config.NATS.URLs)
	assert.Equal(t, "jxt-ephemeral-client", config.NATS.ClientID)
}

// TestInitializeGlobal 测试初始化全局 EventBus
func TestInitializeGlobal(t *testing.T) {
	// 确保清理
	defer CloseGlobal()

	config := &EventBusConfig{
		Type: "memory",
	}

	err := InitializeGlobal(config)
	assert.NoError(t, err)
	assert.True(t, IsInitialized())

	bus := GetGlobal()
	assert.NotNil(t, bus)
}

// TestInitializeGlobal_AlreadyInitialized 测试重复初始化
func TestInitializeGlobal_AlreadyInitialized(t *testing.T) {
	defer CloseGlobal()

	config := &EventBusConfig{
		Type: "memory",
	}

	// 第一次初始化
	err := InitializeGlobal(config)
	require.NoError(t, err)

	// 第二次初始化应该失败
	err = InitializeGlobal(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already initialized")
}

// TestGetGlobal_NotInitialized 测试未初始化时获取全局实例
func TestGetGlobal_NotInitialized(t *testing.T) {
	// 确保未初始化
	CloseGlobal()

	bus := GetGlobal()
	assert.Nil(t, bus)
}

// TestCloseGlobal 测试关闭全局 EventBus
func TestCloseGlobal(t *testing.T) {
	config := &EventBusConfig{
		Type: "memory",
	}

	err := InitializeGlobal(config)
	require.NoError(t, err)

	err = CloseGlobal()
	assert.NoError(t, err)
	assert.False(t, IsInitialized())

	bus := GetGlobal()
	assert.Nil(t, bus)
}

// TestCloseGlobal_NotInitialized 测试关闭未初始化的全局 EventBus
func TestCloseGlobal_NotInitialized(t *testing.T) {
	CloseGlobal() // 确保未初始化

	err := CloseGlobal()
	assert.NoError(t, err) // 应该成功（幂等）
}

// TestIsInitialized 测试检查初始化状态
func TestIsInitialized(t *testing.T) {
	defer CloseGlobal()

	// 未初始化
	assert.False(t, IsInitialized())

	// 初始化
	config := &EventBusConfig{
		Type: "memory",
	}
	err := InitializeGlobal(config)
	require.NoError(t, err)

	assert.True(t, IsInitialized())

	// 关闭
	err = CloseGlobal()
	require.NoError(t, err)

	assert.False(t, IsInitialized())
}

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
