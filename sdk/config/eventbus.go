package config

import (
	"fmt"
	"time"
)

// ==========================================================================
// 核心配置结构 - 统一的EventBus配置入口
// ==========================================================================

// EventBusConfig 事件总线配置
type EventBusConfig struct {
	// 基础配置
	Type        string `mapstructure:"type"`        // kafka, nats, memory
	ServiceName string `mapstructure:"serviceName"` // 微服务名称

	// 统一特性配置（适用于所有EventBus类型）
	HealthCheck HealthCheckConfig `mapstructure:"healthCheck"` // 健康检查
	Monitoring  MetricsConfig     `mapstructure:"monitoring"`  // 监控（复用现有的MetricsConfig）
	Security    SecurityConfig    `mapstructure:"security"`    // 安全

	// 发布端配置
	Publisher PublisherConfig `mapstructure:"publisher"`

	// 订阅端配置
	Subscriber SubscriberConfig `mapstructure:"subscriber"`

	// 具体实现配置
	Kafka  KafkaConfig  `mapstructure:"kafka"`  // Kafka配置
	NATS   NATSConfig   `mapstructure:"nats"`   // NATS配置
	Memory MemoryConfig `mapstructure:"memory"` // Memory配置
}

// ==========================================================================
// 特定实现配置 - 只包含各实现特有的配置
// ==========================================================================

// KafkaConfig Kafka配置 - 用户配置层（简化）
// 只包含用户需要关心的核心配置字段
type KafkaConfig struct {
	Brokers  []string       `mapstructure:"brokers"`  // Kafka集群地址
	Producer ProducerConfig `mapstructure:"producer"` // 生产者配置
	Consumer ConsumerConfig `mapstructure:"consumer"` // 消费者配置
	// 移除了程序员应该控制的字段: Net (网络配置由程序员在内部处理)
}

// NATSConfig NATS配置
type NATSConfig struct {
	URLs              []string           `mapstructure:"urls"`              // NATS服务器地址
	ClientID          string             `mapstructure:"clientId"`          // 客户端ID
	MaxReconnects     int                `mapstructure:"maxReconnects"`     // 最大重连次数
	ReconnectWait     time.Duration      `mapstructure:"reconnectWait"`     // 重连等待时间
	ConnectionTimeout time.Duration      `mapstructure:"connectionTimeout"` // 连接超时
	JetStream         JetStreamConfig    `mapstructure:"jetstream"`         // JetStream配置
	Security          NATSSecurityConfig `mapstructure:"security"`          // NATS安全配置
}

// MemoryConfig Memory配置
type MemoryConfig struct {
	MaxChannelSize int `mapstructure:"maxChannelSize"` // 最大通道大小
	BufferSize     int `mapstructure:"bufferSize"`     // 缓冲区大小
}

// ==========================================================================
// 统一特性配置 - 适用于所有EventBus类型
// ==========================================================================

// HealthCheckConfig 统一健康检查配置
type HealthCheckConfig struct {
	// 基础配置
	Enabled bool `mapstructure:"enabled"` // 是否启用健康检查

	// 发布器配置
	Publisher HealthCheckPublisherConfig `mapstructure:"publisher"`

	// 订阅监控器配置
	Subscriber HealthCheckSubscriberConfig `mapstructure:"subscriber"`
}

// HealthCheckPublisherConfig 健康检查发布器配置
type HealthCheckPublisherConfig struct {
	Topic            string        `mapstructure:"topic"`            // 健康检查发布主题（可选，默认自动生成）
	Interval         time.Duration `mapstructure:"interval"`         // 发布间隔（默认2分钟）
	Timeout          time.Duration `mapstructure:"timeout"`          // 发布超时（默认10秒）
	FailureThreshold int           `mapstructure:"failureThreshold"` // 连续失败阈值，触发重连（默认3次）
	MessageTTL       time.Duration `mapstructure:"messageTTL"`       // 消息存活时间（默认5分钟）
}

// HealthCheckSubscriberConfig 健康检查订阅监控器配置
type HealthCheckSubscriberConfig struct {
	Topic             string        `mapstructure:"topic"`             // 健康检查订阅主题（可选，默认自动生成）
	MonitorInterval   time.Duration `mapstructure:"monitorInterval"`   // 监控检查间隔（默认30秒）
	WarningThreshold  int           `mapstructure:"warningThreshold"`  // 警告阈值（默认3次）
	ErrorThreshold    int           `mapstructure:"errorThreshold"`    // 错误阈值（默认5次）
	CriticalThreshold int           `mapstructure:"criticalThreshold"` // 严重阈值（默认10次）
}

// ==========================================================================
// 发布端和订阅端配置
// ==========================================================================

// PublisherConfig 发布端配置
type PublisherConfig struct {
	// 重连配置
	MaxReconnectAttempts int           `mapstructure:"maxReconnectAttempts"` // 最大重连尝试次数（默认5次）
	InitialBackoff       time.Duration `mapstructure:"initialBackoff"`       // 初始退避时间（默认1秒）
	MaxBackoff           time.Duration `mapstructure:"maxBackoff"`           // 最大退避时间（默认30秒）

	// 发布配置
	PublishTimeout time.Duration `mapstructure:"publishTimeout"` // 发布超时（默认10秒）

	// 企业特性
	BacklogDetection PublisherBacklogDetectionConfig `mapstructure:"backlogDetection"` // 发送端积压检测
	RateLimit        RateLimitConfig                 `mapstructure:"rateLimit"`        // 流量控制
	ErrorHandling    ErrorHandlingConfig             `mapstructure:"errorHandling"`    // 错误处理
}

// SubscriberConfig 订阅端配置
type SubscriberConfig struct {
	// 消费配置
	MaxConcurrency int           `mapstructure:"maxConcurrency"` // 最大并发数（默认10）
	ProcessTimeout time.Duration `mapstructure:"processTimeout"` // 处理超时（默认30秒）

	// 企业特性
	BacklogDetection SubscriberBacklogDetectionConfig `mapstructure:"backlogDetection"` // 订阅端积压检测
	RateLimit        RateLimitConfig                  `mapstructure:"rateLimit"`        // 流量控制
	ErrorHandling    ErrorHandlingConfig              `mapstructure:"errorHandling"`    // 错误处理
}

// PublisherBacklogDetectionConfig 发送端积压检测配置
type PublisherBacklogDetectionConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	MaxQueueDepth     int64         `mapstructure:"maxQueueDepth"`     // 最大队列深度
	MaxPublishLatency time.Duration `mapstructure:"maxPublishLatency"` // 最大发送延迟
	RateThreshold     float64       `mapstructure:"rateThreshold"`     // 发送速率阈值 (msg/sec)
	CheckInterval     time.Duration `mapstructure:"checkInterval"`     // 检测间隔
}

// SubscriberBacklogDetectionConfig 订阅端积压检测配置
type SubscriberBacklogDetectionConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	MaxLagThreshold  int64         `mapstructure:"maxLagThreshold"`  // 最大消息积压数量
	MaxTimeThreshold time.Duration `mapstructure:"maxTimeThreshold"` // 最大积压时间
	CheckInterval    time.Duration `mapstructure:"checkInterval"`    // 检测间隔
}

// BacklogDetectionConfig 通用积压检测配置（向后兼容）
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

// ProducerConfig 生产者配置 - 用户配置层（简化）
// 只包含用户需要关心的核心配置字段
type ProducerConfig struct {
	RequiredAcks    int           `mapstructure:"requiredAcks"`    // 消息确认级别 (0=不确认, 1=leader确认, -1=所有副本确认)
	Compression     string        `mapstructure:"compression"`     // 压缩算法 (none, gzip, snappy, lz4, zstd)
	FlushFrequency  time.Duration `mapstructure:"flushFrequency"`  // 刷新频率
	FlushMessages   int           `mapstructure:"flushMessages"`   // 批量消息数
	Timeout         time.Duration `mapstructure:"timeout"`         // 发送超时时间
	// 移除了程序员应该控制的字段: FlushBytes, RetryMax, BatchSize, BufferSize, Idempotent, MaxMessageBytes, PartitionerType
}

// ConsumerConfig 消费者配置 - 用户配置层（简化）
// 只包含用户需要关心的核心配置字段
type ConsumerConfig struct {
	GroupID            string        `mapstructure:"groupId"`            // 消费者组ID
	AutoOffsetReset    string        `mapstructure:"autoOffsetReset"`    // 偏移量重置策略 (earliest, latest, none)
	SessionTimeout     time.Duration `mapstructure:"sessionTimeout"`     // 会话超时时间
	HeartbeatInterval  time.Duration `mapstructure:"heartbeatInterval"`  // 心跳间隔
	// 移除了程序员应该控制的字段: MaxProcessingTime, FetchMinBytes, FetchMaxBytes, FetchMaxWait,
	// RebalanceStrategy, IsolationLevel, MaxPollRecords, EnableAutoCommit, AutoCommitInterval
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

// ==========================================================================
// 配置验证和默认值设置
// ==========================================================================

// SetDefaults 为EventBusConfig设置默认值
func (c *EventBusConfig) SetDefaults() {
	// 健康检查默认值
	if c.HealthCheck.Enabled {
		if c.HealthCheck.Publisher.Interval == 0 {
			c.HealthCheck.Publisher.Interval = 2 * time.Minute
		}
		if c.HealthCheck.Publisher.Timeout == 0 {
			c.HealthCheck.Publisher.Timeout = 10 * time.Second
		}
		if c.HealthCheck.Publisher.FailureThreshold == 0 {
			c.HealthCheck.Publisher.FailureThreshold = 3
		}
		if c.HealthCheck.Publisher.MessageTTL == 0 {
			c.HealthCheck.Publisher.MessageTTL = 5 * time.Minute
		}

		// 订阅监控器默认值
		if c.HealthCheck.Subscriber.MonitorInterval == 0 {
			c.HealthCheck.Subscriber.MonitorInterval = 30 * time.Second
		}
		if c.HealthCheck.Subscriber.WarningThreshold == 0 {
			c.HealthCheck.Subscriber.WarningThreshold = 3
		}
		if c.HealthCheck.Subscriber.ErrorThreshold == 0 {
			c.HealthCheck.Subscriber.ErrorThreshold = 5
		}
		if c.HealthCheck.Subscriber.CriticalThreshold == 0 {
			c.HealthCheck.Subscriber.CriticalThreshold = 10
		}
	}

	// 发布端默认值
	if c.Publisher.MaxReconnectAttempts == 0 {
		c.Publisher.MaxReconnectAttempts = 5
	}
	if c.Publisher.InitialBackoff == 0 {
		c.Publisher.InitialBackoff = 1 * time.Second
	}
	if c.Publisher.MaxBackoff == 0 {
		c.Publisher.MaxBackoff = 30 * time.Second
	}
	if c.Publisher.PublishTimeout == 0 {
		c.Publisher.PublishTimeout = 10 * time.Second
	}

	// 订阅端默认值
	if c.Subscriber.MaxConcurrency == 0 {
		c.Subscriber.MaxConcurrency = 10
	}
	if c.Subscriber.ProcessTimeout == 0 {
		c.Subscriber.ProcessTimeout = 30 * time.Second
	}
}

// Validate 验证EventBusConfig配置
func (c *EventBusConfig) Validate() error {
	if c.Type == "" {
		return fmt.Errorf("eventbus type is required")
	}

	if c.Type != "kafka" && c.Type != "nats" && c.Type != "memory" {
		return fmt.Errorf("unsupported eventbus type: %s", c.Type)
	}

	if c.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}

	// 验证健康检查配置
	if c.HealthCheck.Enabled {
		if c.HealthCheck.Publisher.Interval <= 0 {
			return fmt.Errorf("health check publisher interval must be positive")
		}
		if c.HealthCheck.Publisher.Timeout <= 0 {
			return fmt.Errorf("health check publisher timeout must be positive")
		}
		if c.HealthCheck.Subscriber.MonitorInterval <= 0 {
			return fmt.Errorf("health check subscriber monitor interval must be positive")
		}
	}

	return nil
}
