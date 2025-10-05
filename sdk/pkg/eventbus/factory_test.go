package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
