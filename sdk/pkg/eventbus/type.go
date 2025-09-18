package eventbus

import (
	"context"
	"time"
)

// Topic常量定义 - 只定义技术基础设施相关的Topic
const (
	// 技术基础设施相关的Topic常量
	HealthCheckTopic = "health_check_topic" // 用于监控eventbus组件健康状态（已废弃，使用统一主题）

	// 可能的其他技术性Topic（根据需要添加）
	// DeadLetterTopic = "dead_letter_topic"  // 死信队列
	// MetricsTopic    = "metrics_topic"      // 指标收集
	// TracingTopic    = "tracing_topic"      // 链路追踪
)

// MessageHandler 消息处理器函数类型
type MessageHandler func(ctx context.Context, message []byte) error

// EventBus 技术层事件总线接口（基础设施层使用）
type EventBus interface {
	// 发布消息到指定主题
	Publish(ctx context.Context, topic string, message []byte) error

	// 订阅指定主题的消息
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error

	// 健康检查
	HealthCheck(ctx context.Context) error

	// 关闭连接
	Close() error

	// 注册重连回调
	RegisterReconnectCallback(callback func(ctx context.Context) error) error
}

// Publisher 发布器接口
type Publisher interface {
	// 发布消息
	Publish(ctx context.Context, topic string, message []byte) error
	// 关闭发布器
	Close() error
}

// Subscriber 订阅器接口
type Subscriber interface {
	// 订阅消息
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error
	// 关闭订阅器
	Close() error
}

// EventBusConfig 事件总线配置
type EventBusConfig struct {
	Type    string        `mapstructure:"type"` // kafka, nats, memory
	Kafka   KafkaConfig   `mapstructure:"kafka"`
	NATS    NATSConfig    `mapstructure:"nats"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Tracing TracingConfig `mapstructure:"tracing"`
}

// KafkaConfig Kafka配置
type KafkaConfig struct {
	Brokers             []string       `mapstructure:"brokers"`
	HealthCheckInterval time.Duration  `mapstructure:"healthCheckInterval"`
	Producer            ProducerConfig `mapstructure:"producer"`
	Consumer            ConsumerConfig `mapstructure:"consumer"`
	Security            SecurityConfig `mapstructure:"security"`
}

// ProducerConfig 生产者配置
type ProducerConfig struct {
	RequiredAcks   int           `mapstructure:"requiredAcks"`
	Compression    string        `mapstructure:"compression"`
	FlushFrequency time.Duration `mapstructure:"flushFrequency"`
	FlushMessages  int           `mapstructure:"flushMessages"`
	RetryMax       int           `mapstructure:"retryMax"`
	Timeout        time.Duration `mapstructure:"timeout"`
	BatchSize      int           `mapstructure:"batchSize"`
	BufferSize     int           `mapstructure:"bufferSize"`
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	GroupID           string        `mapstructure:"groupId"`
	AutoOffsetReset   string        `mapstructure:"autoOffsetReset"`
	SessionTimeout    time.Duration `mapstructure:"sessionTimeout"`
	HeartbeatInterval time.Duration `mapstructure:"heartbeatInterval"`
	MaxProcessingTime time.Duration `mapstructure:"maxProcessingTime"`
	FetchMinBytes     int           `mapstructure:"fetchMinBytes"`
	FetchMaxBytes     int           `mapstructure:"fetchMaxBytes"`
	FetchMaxWait      time.Duration `mapstructure:"fetchMaxWait"`
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

// Metrics 监控指标
type Metrics struct {
	MessagesPublished int64     `json:"messagesPublished"`
	MessagesConsumed  int64     `json:"messagesConsumed"`
	PublishErrors     int64     `json:"publishErrors"`
	ConsumeErrors     int64     `json:"consumeErrors"`
	ConnectionErrors  int64     `json:"connectionErrors"`
	LastHealthCheck   time.Time `json:"lastHealthCheck"`
	HealthCheckStatus string    `json:"healthCheckStatus"`
	ActiveConnections int       `json:"activeConnections"`
	MessageBacklog    int64     `json:"messageBacklog"`
}

// HealthStatus 健康状态
type HealthStatus struct {
	Status    string                 `json:"status"` // healthy, unhealthy, degraded
	LastCheck time.Time              `json:"lastCheck"`
	Errors    []string               `json:"errors,omitempty"`
	Metrics   Metrics                `json:"metrics"`
	Details   map[string]interface{} `json:"details,omitempty"`
}
