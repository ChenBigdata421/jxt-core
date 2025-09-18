package eventbus

import (
	"fmt"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

var (
	// 全局事件总线实例
	globalEventBus EventBus
	globalMutex    sync.RWMutex
	initialized    bool
)

// Factory 事件总线工厂
type Factory struct {
	config *EventBusConfig
}

// NewFactory 创建事件总线工厂
func NewFactory(config *EventBusConfig) *Factory {
	return &Factory{config: config}
}

// CreateEventBus 创建事件总线实例
func (f *Factory) CreateEventBus() (EventBus, error) {
	if f.config == nil {
		return nil, fmt.Errorf("eventbus config is required")
	}

	// 验证配置
	if err := f.validateConfig(); err != nil {
		return nil, fmt.Errorf("invalid eventbus config: %w", err)
	}

	// 创建事件总线实例
	eventBus, err := NewEventBus(f.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create eventbus: %w", err)
	}

	logger.Info("EventBus created successfully", "type", f.config.Type)
	return eventBus, nil
}

// validateConfig 验证配置
func (f *Factory) validateConfig() error {
	if f.config.Type == "" {
		return fmt.Errorf("eventbus type is required")
	}

	switch f.config.Type {
	case "kafka":
		return f.validateKafkaConfig()
	case "nats":
		return f.validateNATSConfig()
	case "memory":
		return f.validateMemoryConfig()
	default:
		return fmt.Errorf("unsupported eventbus type: %s", f.config.Type)
	}
}

// validateKafkaConfig 验证Kafka配置
func (f *Factory) validateKafkaConfig() error {
	kafka := &f.config.Kafka

	if len(kafka.Brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}

	// 设置默认值
	if kafka.HealthCheckInterval == 0 {
		kafka.HealthCheckInterval = 5 * time.Minute
	}

	if kafka.Producer.RequiredAcks == 0 {
		kafka.Producer.RequiredAcks = 1 // WaitForLocal
	}

	if kafka.Producer.FlushFrequency == 0 {
		kafka.Producer.FlushFrequency = 500 * time.Millisecond
	}

	if kafka.Producer.FlushMessages == 0 {
		kafka.Producer.FlushMessages = 100
	}

	if kafka.Producer.RetryMax == 0 {
		kafka.Producer.RetryMax = 3
	}

	if kafka.Producer.Timeout == 0 {
		kafka.Producer.Timeout = 10 * time.Second
	}

	if kafka.Consumer.GroupID == "" {
		kafka.Consumer.GroupID = "jxt-eventbus-group"
	}

	if kafka.Consumer.AutoOffsetReset == "" {
		kafka.Consumer.AutoOffsetReset = "earliest"
	}

	if kafka.Consumer.SessionTimeout == 0 {
		kafka.Consumer.SessionTimeout = 30 * time.Second
	}

	if kafka.Consumer.HeartbeatInterval == 0 {
		kafka.Consumer.HeartbeatInterval = 3 * time.Second
	}

	return nil
}

// validateNATSConfig 验证NATS配置
func (f *Factory) validateNATSConfig() error {
	nats := &f.config.NATS

	if len(nats.URLs) == 0 {
		return fmt.Errorf("nats urls are required")
	}

	// 设置默认值
	if nats.ClientID == "" {
		nats.ClientID = "jxt-client"
	}

	if nats.MaxReconnects == 0 {
		nats.MaxReconnects = 10
	}

	if nats.ReconnectWait == 0 {
		nats.ReconnectWait = 2 * time.Second
	}

	if nats.ConnectionTimeout == 0 {
		nats.ConnectionTimeout = 10 * time.Second
	}

	if nats.HealthCheckInterval == 0 {
		nats.HealthCheckInterval = 5 * time.Minute
	}

	return nil
}

// validateMemoryConfig 验证内存配置
func (f *Factory) validateMemoryConfig() error {
	// 内存实现不需要特殊配置验证
	return nil
}

// GetDefaultConfig 获取默认配置
func GetDefaultConfig(eventBusType string) *EventBusConfig {
	config := &EventBusConfig{
		Type: eventBusType,
		Metrics: MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
		Tracing: TracingConfig{
			Enabled:    false,
			SampleRate: 0.1,
		},
	}

	switch eventBusType {
	case "kafka":
		config.Kafka = KafkaConfig{
			Brokers:             []string{"localhost:9092"},
			HealthCheckInterval: 5 * time.Minute,
			Producer: ProducerConfig{
				RequiredAcks:   1,
				Compression:    "snappy",
				FlushFrequency: 500 * time.Millisecond,
				FlushMessages:  100,
				RetryMax:       3,
				Timeout:        10 * time.Second,
				BatchSize:      16384,
				BufferSize:     32768,
			},
			Consumer: ConsumerConfig{
				GroupID:           "jxt-eventbus-group",
				AutoOffsetReset:   "earliest",
				SessionTimeout:    30 * time.Second,
				HeartbeatInterval: 3 * time.Second,
				MaxProcessingTime: 5 * time.Minute,
				FetchMinBytes:     1,
				FetchMaxBytes:     1048576,
				FetchMaxWait:      500 * time.Millisecond,
			},
		}
	case "nats":
		config.NATS = NATSConfig{
			URLs:                []string{"nats://localhost:4222"},
			ClientID:            "jxt-client",
			MaxReconnects:       10,
			ReconnectWait:       2 * time.Second,
			ConnectionTimeout:   10 * time.Second,
			HealthCheckInterval: 5 * time.Minute,
		}
	}

	return config
}

// InitializeGlobal 初始化全局事件总线
func InitializeGlobal(config *EventBusConfig) error {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	if initialized {
		return fmt.Errorf("global eventbus already initialized")
	}

	factory := NewFactory(config)
	eventBus, err := factory.CreateEventBus()
	if err != nil {
		return fmt.Errorf("failed to initialize global eventbus: %w", err)
	}

	globalEventBus = eventBus
	initialized = true

	logger.Info("Global EventBus initialized successfully", "type", config.Type)
	return nil
}

// GetGlobal 获取全局事件总线实例
func GetGlobal() EventBus {
	globalMutex.RLock()
	defer globalMutex.RUnlock()

	if !initialized {
		logger.Warn("Global eventbus not initialized, returning nil")
		return nil
	}

	return globalEventBus
}

// CloseGlobal 关闭全局事件总线
func CloseGlobal() error {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	if !initialized {
		return nil
	}

	if globalEventBus != nil {
		if err := globalEventBus.Close(); err != nil {
			return fmt.Errorf("failed to close global eventbus: %w", err)
		}
	}

	globalEventBus = nil
	initialized = false

	logger.Info("Global EventBus closed successfully")
	return nil
}

// IsInitialized 检查全局事件总线是否已初始化
func IsInitialized() bool {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	return initialized
}

// AdvancedFactory 高级事件总线工厂
type AdvancedFactory struct {
	config *config.AdvancedEventBusConfig
}

// NewAdvancedFactory 创建高级事件总线工厂
func NewAdvancedFactory(config *config.AdvancedEventBusConfig) *AdvancedFactory {
	return &AdvancedFactory{config: config}
}

// CreateAdvancedEventBus 创建高级事件总线实例
func (f *AdvancedFactory) CreateAdvancedEventBus() (AdvancedEventBus, error) {
	if f.config == nil {
		return nil, fmt.Errorf("advanced eventbus config is required")
	}

	// 验证配置
	if err := f.validateAdvancedConfig(); err != nil {
		return nil, fmt.Errorf("invalid advanced eventbus config: %w", err)
	}

	// 创建高级事件总线实例
	eventBus, err := CreateAdvancedEventBus(f.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create advanced eventbus: %w", err)
	}

	logger.Info("Advanced EventBus created successfully",
		"type", f.config.Type,
		"serviceName", f.config.ServiceName)
	return eventBus, nil
}

// validateAdvancedConfig 验证高级配置
func (f *AdvancedFactory) validateAdvancedConfig() error {
	if f.config.ServiceName == "" {
		return fmt.Errorf("serviceName is required for advanced event bus")
	}

	if f.config.Type == "" {
		return fmt.Errorf("eventbus type is required")
	}

	// 设置默认值
	if f.config.HealthCheck.Interval == 0 {
		f.config.HealthCheck.Interval = 2 * time.Minute
	}
	if f.config.HealthCheck.Timeout == 0 {
		f.config.HealthCheck.Timeout = 10 * time.Second
	}
	if f.config.HealthCheck.FailureThreshold == 0 {
		f.config.HealthCheck.FailureThreshold = 3
	}

	if f.config.Publisher.MaxReconnectAttempts == 0 {
		f.config.Publisher.MaxReconnectAttempts = 5
	}
	if f.config.Publisher.MaxBackoff == 0 {
		f.config.Publisher.MaxBackoff = 1 * time.Minute
	}
	if f.config.Publisher.InitialBackoff == 0 {
		f.config.Publisher.InitialBackoff = 1 * time.Second
	}
	if f.config.Publisher.PublishTimeout == 0 {
		f.config.Publisher.PublishTimeout = 30 * time.Second
	}

	return nil
}

// CreateAdvancedEventBus 创建高级事件总线实例（全局函数）
func CreateAdvancedEventBus(cfg *config.AdvancedEventBusConfig) (AdvancedEventBus, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// 验证必需的配置
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("serviceName is required for advanced event bus")
	}

	// 根据类型创建具体实现
	switch cfg.Type {
	case "kafka":
		return NewKafkaAdvancedEventBus(*cfg)
	case "nats":
		// TODO: 实现 NATS 高级事件总线
		return nil, fmt.Errorf("NATS advanced event bus not implemented yet")
	case "memory":
		// TODO: 实现 Memory 高级事件总线
		return nil, fmt.Errorf("Memory advanced event bus not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported advanced eventbus type: %s", cfg.Type)
	}
}

// GetDefaultAdvancedEventBusConfig 获取默认高级事件总线配置
func GetDefaultAdvancedEventBusConfig() config.AdvancedEventBusConfig {
	return config.AdvancedEventBusConfig{
		EventBus: config.EventBus{
			Type: "kafka",
			Kafka: config.KafkaConfig{
				Brokers: []string{"localhost:9092"},
			},
		},
		ServiceName: "default-service",
		HealthCheck: GetDefaultHealthCheckConfig(),
		Publisher: config.PublisherConfig{
			MaxReconnectAttempts: 5,
			MaxBackoff:           1 * time.Minute,
			InitialBackoff:       1 * time.Second,
			PublishTimeout:       30 * time.Second,
		},
		Subscriber: config.SubscriberConfig{
			RecoveryMode: config.RecoveryModeConfig{
				Enabled:             true,
				AutoDetection:       true,
				TransitionThreshold: 3,
			},
			BacklogDetection: config.BacklogDetectionConfig{
				Enabled:         true,
				MaxLagThreshold: 1000,
				CheckInterval:   1 * time.Minute,
			},
			AggregateProcessor: config.AggregateProcessorConfig{
				Enabled:     true,
				CacheSize:   1000,
				IdleTimeout: 5 * time.Minute,
			},
			RateLimit: config.RateLimitConfig{
				Enabled:       true,
				RatePerSecond: 1000,
				BurstSize:     1000,
			},
		},
	}
}
