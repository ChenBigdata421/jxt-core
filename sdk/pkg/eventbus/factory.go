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

	// 全局配置存储
	globalConfig *config.EventBusConfig
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
			CollectInterval: DefaultMetricsCollectInterval,
		},
		Tracing: TracingConfig{
			Enabled:    false,
			SampleRate: DefaultTracingSampleRate,
		},
	}

	switch eventBusType {
	case "kafka":
		config.Kafka = KafkaConfig{
			Brokers:             []string{"localhost:9092"},
			HealthCheckInterval: DefaultKafkaHealthCheckInterval,
			Producer: ProducerConfig{
				RequiredAcks:   DefaultKafkaProducerRequiredAcks,
				Compression:    "snappy",
				FlushFrequency: DefaultKafkaProducerFlushFrequency,
				FlushMessages:  DefaultKafkaProducerFlushMessages,
				RetryMax:       DefaultKafkaProducerRetryMax,
				Timeout:        DefaultKafkaProducerTimeout,
				BatchSize:      DefaultKafkaProducerBatchSize,
				BufferSize:     DefaultKafkaProducerBufferSize,
			},
			Consumer: ConsumerConfig{
				GroupID:           "jxt-eventbus-group",
				AutoOffsetReset:   "earliest",
				SessionTimeout:    DefaultKafkaConsumerSessionTimeout,
				HeartbeatInterval: DefaultKafkaConsumerHeartbeatInterval,
				MaxProcessingTime: DefaultKafkaConsumerMaxProcessingTime,
				FetchMinBytes:     DefaultKafkaConsumerFetchMinBytes,
				FetchMaxBytes:     DefaultKafkaConsumerFetchMaxBytes,
				FetchMaxWait:      DefaultKafkaConsumerFetchMaxWait,
			},
		}
	case "nats":
		config.NATS = NATSConfig{
			URLs:                []string{"nats://localhost:4222"},
			ClientID:            "jxt-client",
			MaxReconnects:       DefaultNATSMaxReconnects,
			ReconnectWait:       DefaultNATSReconnectWait,
			ConnectionTimeout:   DefaultNATSConnectionTimeout,
			HealthCheckInterval: DefaultNATSHealthCheckInterval,
		}
	}

	return config
}

// GetDefaultPersistentNATSConfig 获取持久化NATS配置
func GetDefaultPersistentNATSConfig(urls []string, clientID string) *EventBusConfig {
	if len(urls) == 0 {
		urls = []string{"nats://localhost:4222"}
	}
	if clientID == "" {
		clientID = "jxt-persistent-client"
	}

	return &EventBusConfig{
		Type: "nats",
		NATS: NATSConfig{
			URLs:                urls,
			ClientID:            clientID,
			MaxReconnects:       DefaultNATSMaxReconnects,
			ReconnectWait:       DefaultNATSReconnectWait,
			ConnectionTimeout:   DefaultNATSConnectionTimeout,
			HealthCheckInterval: DefaultNATSHealthCheckInterval,
			JetStream: JetStreamConfig{
				Enabled:        true, // 启用持久化
				PublishTimeout: DefaultNATSPublishTimeout,
				AckWait:        DefaultNATSAckWait,
				MaxDeliver:     DefaultNATSMaxDeliver,
				Stream: StreamConfig{
					Name:      "PERSISTENT_STREAM",
					Subjects:  []string{"*"}, // 接受所有主题
					Retention: "limits",
					Storage:   "file", // 文件存储，持久化
					Replicas:  DefaultNATSStreamReplicas,
					MaxAge:    DefaultNATSStreamMaxAge,
					MaxBytes:  DefaultNATSStreamMaxBytes,
					MaxMsgs:   DefaultNATSStreamMaxMsgs,
					Discard:   "old",
				},
				Consumer: NATSConsumerConfig{
					DurableName:   "persistent-consumer",
					DeliverPolicy: "all",
					AckPolicy:     "explicit",
					ReplayPolicy:  "instant",
					MaxAckPending: DefaultNATSConsumerMaxAckPending,
					MaxWaiting:    DefaultNATSConsumerMaxWaiting,
					MaxDeliver:    DefaultNATSMaxDeliver,
				},
			},
		},
		Metrics: MetricsConfig{
			Enabled:         true,
			CollectInterval: DefaultMetricsCollectInterval,
		},
		Tracing: TracingConfig{
			Enabled:    false,
			SampleRate: DefaultTracingSampleRate,
		},
	}
}

// GetDefaultEphemeralNATSConfig 获取非持久化NATS配置
func GetDefaultEphemeralNATSConfig(urls []string, clientID string) *EventBusConfig {
	if len(urls) == 0 {
		urls = []string{"nats://localhost:4222"}
	}
	if clientID == "" {
		clientID = "jxt-ephemeral-client"
	}

	return &EventBusConfig{
		Type: "nats",
		NATS: NATSConfig{
			URLs:                urls,
			ClientID:            clientID,
			MaxReconnects:       DefaultNATSMaxReconnects,
			ReconnectWait:       DefaultNATSReconnectWait,
			ConnectionTimeout:   DefaultNATSConnectionTimeout,
			HealthCheckInterval: DefaultNATSHealthCheckInterval,
			JetStream: JetStreamConfig{
				Enabled: false, // 禁用持久化，使用Core NATS
			},
		},
		Metrics: MetricsConfig{
			Enabled:         true,
			CollectInterval: DefaultMetricsCollectInterval,
		},
		Tracing: TracingConfig{
			Enabled:    false,
			SampleRate: DefaultTracingSampleRate,
		},
	}
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
	globalConfig = nil
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

// SetGlobalConfig 设置全局配置
func SetGlobalConfig(cfg *config.EventBusConfig) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	globalConfig = cfg
}

// GetGlobalConfig 获取全局配置
func GetGlobalConfig() *config.EventBusConfig {
	globalMutex.RLock()
	defer globalMutex.RUnlock()
	return globalConfig
}

// ========== 便捷工厂方法 ==========

// NewPersistentNATSEventBus 创建持久化NATS EventBus实例
func NewPersistentNATSEventBus(urls []string, clientID string) (EventBus, error) {
	config := GetDefaultPersistentNATSConfig(urls, clientID)
	return NewEventBus(config)
}

// NewEphemeralNATSEventBus 创建非持久化NATS EventBus实例
func NewEphemeralNATSEventBus(urls []string, clientID string) (EventBus, error) {
	config := GetDefaultEphemeralNATSConfig(urls, clientID)
	return NewEventBus(config)
}

// NewPersistentKafkaEventBus 创建持久化Kafka EventBus实例
func NewPersistentKafkaEventBus(brokers []string) (EventBus, error) {
	config := GetDefaultConfig("kafka")
	config.Kafka.Brokers = brokers
	// Kafka默认就是持久化的
	return NewEventBus(config)
}
