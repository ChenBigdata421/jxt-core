package config

import (
	"time"
)

// EventBus 基础事件总线配置
type EventBus struct {
	Type    string        `mapstructure:"type"` // kafka, nats, memory
	Kafka   KafkaConfig   `mapstructure:"kafka"`
	NATS    NATSConfig    `mapstructure:"nats"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Tracing TracingConfig `mapstructure:"tracing"`
}

// AdvancedEventBusConfig 高级事件总线配置
type AdvancedEventBusConfig struct {
	EventBus    `mapstructure:",squash"`
	ServiceName string `mapstructure:"serviceName"` // 微服务名称

	// 统一健康检查配置
	HealthCheck HealthCheckConfig `mapstructure:"healthCheck"`

	// 发布端配置
	Publisher PublisherConfig `mapstructure:"publisher"`

	// 订阅端配置
	Subscriber SubscriberConfig `mapstructure:"subscriber"`
}

// HealthCheckConfig 统一健康检查配置
type HealthCheckConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Topic            string        `mapstructure:"topic"`            // 健康检查主题（可选，默认使用统一主题）
	Interval         time.Duration `mapstructure:"interval"`         // 检查间隔
	Timeout          time.Duration `mapstructure:"timeout"`          // 检查超时
	FailureThreshold int           `mapstructure:"failureThreshold"` // 失败阈值
	MessageTTL       time.Duration `mapstructure:"messageTTL"`       // 消息存活时间
}

// PublisherConfig 发布端配置
type PublisherConfig struct {
	MaxReconnectAttempts int           `mapstructure:"maxReconnectAttempts"`
	MaxBackoff           time.Duration `mapstructure:"maxBackoff"`
	InitialBackoff       time.Duration `mapstructure:"initialBackoff"`
	PublishTimeout       time.Duration `mapstructure:"publishTimeout"`
}

// SubscriberConfig 订阅端配置
type SubscriberConfig struct {
	// 恢复模式配置
	RecoveryMode RecoveryModeConfig `mapstructure:"recoveryMode"`

	// 积压检测配置
	BacklogDetection BacklogDetectionConfig `mapstructure:"backlogDetection"`

	// 聚合处理器配置
	AggregateProcessor AggregateProcessorConfig `mapstructure:"aggregateProcessor"`

	// 流量控制配置
	RateLimit RateLimitConfig `mapstructure:"rateLimit"`
}

// RecoveryModeConfig 恢复模式配置
type RecoveryModeConfig struct {
	Enabled             bool `mapstructure:"enabled"`
	AutoDetection       bool `mapstructure:"autoDetection"`       // 自动检测是否进入恢复模式
	TransitionThreshold int  `mapstructure:"transitionThreshold"` // 进入恢复模式的阈值
}

// AggregateProcessorConfig 聚合处理器配置
type AggregateProcessorConfig struct {
	Enabled     bool          `mapstructure:"enabled"`
	CacheSize   int           `mapstructure:"cacheSize"`   // LRU缓存大小
	IdleTimeout time.Duration `mapstructure:"idleTimeout"` // 空闲超时时间
}

// KafkaConfig Kafka配置
type KafkaConfig struct {
	Brokers             []string       `mapstructure:"brokers"`
	HealthCheckInterval time.Duration  `mapstructure:"healthCheckInterval"`
	Producer            ProducerConfig `mapstructure:"producer"`
	Consumer            ConsumerConfig `mapstructure:"consumer"`
	Security            SecurityConfig `mapstructure:"security"`
	Net                 NetConfig      `mapstructure:"net"`

	// 企业级特性配置
	BacklogDetection BacklogDetectionConfig `mapstructure:"backlogDetection"`
	RateLimit        RateLimitConfig        `mapstructure:"rateLimit"`
	ErrorHandling    ErrorHandlingConfig    `mapstructure:"errorHandling"`
}

// NetConfig 网络配置
type NetConfig struct {
	DialTimeout  time.Duration `mapstructure:"dialTimeout"`
	ReadTimeout  time.Duration `mapstructure:"readTimeout"`
	WriteTimeout time.Duration `mapstructure:"writeTimeout"`
}

// BacklogDetectionConfig 积压检测配置
type BacklogDetectionConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	MaxLagThreshold  int64         `mapstructure:"maxLagThreshold"`
	MaxTimeThreshold time.Duration `mapstructure:"maxTimeThreshold"`
	CheckInterval    time.Duration `mapstructure:"checkInterval"`
}

// RateLimitConfig 流量控制配置
type RateLimitConfig struct {
	Enabled       bool    `mapstructure:"enabled"`
	RatePerSecond float64 `mapstructure:"ratePerSecond"`
	BurstSize     int     `mapstructure:"burstSize"`
}

// ErrorHandlingConfig 错误处理配置
type ErrorHandlingConfig struct {
	DeadLetterTopic  string        `mapstructure:"deadLetterTopic"`
	MaxRetryAttempts int           `mapstructure:"maxRetryAttempts"`
	RetryBackoffBase time.Duration `mapstructure:"retryBackoffBase"`
	RetryBackoffMax  time.Duration `mapstructure:"retryBackoffMax"`
}

// ProducerConfig 生产者配置
type ProducerConfig struct {
	RequiredAcks    int           `mapstructure:"requiredAcks"`
	Compression     string        `mapstructure:"compression"`
	FlushFrequency  time.Duration `mapstructure:"flushFrequency"`
	FlushMessages   int           `mapstructure:"flushMessages"`
	FlushBytes      int           `mapstructure:"flushBytes"`
	RetryMax        int           `mapstructure:"retryMax"`
	Timeout         time.Duration `mapstructure:"timeout"`
	BatchSize       int           `mapstructure:"batchSize"`
	BufferSize      int           `mapstructure:"bufferSize"`
	Idempotent      bool          `mapstructure:"idempotent"`
	MaxMessageBytes int           `mapstructure:"maxMessageBytes"`
	PartitionerType string        `mapstructure:"partitionerType"`
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	GroupID            string        `mapstructure:"groupId"`
	AutoOffsetReset    string        `mapstructure:"autoOffsetReset"`
	SessionTimeout     time.Duration `mapstructure:"sessionTimeout"`
	HeartbeatInterval  time.Duration `mapstructure:"heartbeatInterval"`
	MaxProcessingTime  time.Duration `mapstructure:"maxProcessingTime"`
	FetchMinBytes      int           `mapstructure:"fetchMinBytes"`
	FetchMaxBytes      int           `mapstructure:"fetchMaxBytes"`
	FetchMaxWait       time.Duration `mapstructure:"fetchMaxWait"`
	RebalanceStrategy  string        `mapstructure:"rebalanceStrategy"`
	IsolationLevel     string        `mapstructure:"isolationLevel"`
	MaxPollRecords     int           `mapstructure:"maxPollRecords"`
	EnableAutoCommit   bool          `mapstructure:"enableAutoCommit"`
	AutoCommitInterval time.Duration `mapstructure:"autoCommitInterval"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Protocol string `mapstructure:"protocol"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	CertFile string `mapstructure:"certFile"`
	KeyFile  string `mapstructure:"keyFile"`
	CAFile   string `mapstructure:"caFile"`
}

// NATSConfig NATS JetStream配置
type NATSConfig struct {
	URLs                []string      `mapstructure:"urls"`
	ClientID            string        `mapstructure:"clientId"`
	MaxReconnects       int           `mapstructure:"maxReconnects"`
	ReconnectWait       time.Duration `mapstructure:"reconnectWait"`
	ConnectionTimeout   time.Duration `mapstructure:"connectionTimeout"`
	HealthCheckInterval time.Duration `mapstructure:"healthCheckInterval"`

	// JetStream配置
	JetStream JetStreamConfig `mapstructure:"jetstream"`

	// 安全配置
	Security NATSSecurityConfig `mapstructure:"security"`
}

// JetStreamConfig JetStream配置
type JetStreamConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	Domain         string        `mapstructure:"domain"`
	APIPrefix      string        `mapstructure:"apiPrefix"`
	PublishTimeout time.Duration `mapstructure:"publishTimeout"`
	AckWait        time.Duration `mapstructure:"ackWait"`
	MaxDeliver     int           `mapstructure:"maxDeliver"`

	// 流配置
	Stream StreamConfig `mapstructure:"stream"`

	// 消费者配置
	Consumer NATSConsumerConfig `mapstructure:"consumer"`
}

// StreamConfig 流配置
type StreamConfig struct {
	Name      string        `mapstructure:"name"`
	Subjects  []string      `mapstructure:"subjects"`
	Retention string        `mapstructure:"retention"` // limits, interest, workqueue
	Storage   string        `mapstructure:"storage"`   // file, memory
	Replicas  int           `mapstructure:"replicas"`
	MaxAge    time.Duration `mapstructure:"maxAge"`
	MaxBytes  int64         `mapstructure:"maxBytes"`
	MaxMsgs   int64         `mapstructure:"maxMsgs"`
	Discard   string        `mapstructure:"discard"` // old, new
}

// NATSConsumerConfig NATS消费者配置
type NATSConsumerConfig struct {
	DurableName   string          `mapstructure:"durableName"`
	DeliverPolicy string          `mapstructure:"deliverPolicy"` // all, last, new, by_start_sequence, by_start_time
	AckPolicy     string          `mapstructure:"ackPolicy"`     // none, all, explicit
	ReplayPolicy  string          `mapstructure:"replayPolicy"`  // instant, original
	MaxAckPending int             `mapstructure:"maxAckPending"`
	MaxWaiting    int             `mapstructure:"maxWaiting"`
	MaxDeliver    int             `mapstructure:"maxDeliver"`
	BackOff       []time.Duration `mapstructure:"backOff"`
}

// NATSSecurityConfig NATS安全配置
type NATSSecurityConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Token      string `mapstructure:"token"`
	Username   string `mapstructure:"username"`
	Password   string `mapstructure:"password"`
	NKeyFile   string `mapstructure:"nkeyFile"`
	CredFile   string `mapstructure:"credFile"`
	CertFile   string `mapstructure:"certFile"`
	KeyFile    string `mapstructure:"keyFile"`
	CAFile     string `mapstructure:"caFile"`
	SkipVerify bool   `mapstructure:"skipVerify"`
}

// MetricsConfig 指标配置
type MetricsConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	CollectInterval time.Duration `mapstructure:"collectInterval"`
	ExportEndpoint  string        `mapstructure:"exportEndpoint"`
}

// TracingConfig 链路追踪配置
type TracingConfig struct {
	Enabled     bool    `mapstructure:"enabled"`
	ServiceName string  `mapstructure:"serviceName"`
	Endpoint    string  `mapstructure:"endpoint"`
	SampleRate  float64 `mapstructure:"sampleRate"`
}

var EventBusConfig = new(EventBus)
