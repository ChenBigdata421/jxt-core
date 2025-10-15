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
	RequiredAcks   int           `mapstructure:"requiredAcks"`   // 消息确认级别 (0=不确认, 1=leader确认, -1=所有副本确认)
	Compression    string        `mapstructure:"compression"`    // 压缩算法 (none, gzip, snappy, lz4, zstd)
	FlushFrequency time.Duration `mapstructure:"flushFrequency"` // 刷新频率
	FlushMessages  int           `mapstructure:"flushMessages"`  // 批量消息数
	Timeout        time.Duration `mapstructure:"timeout"`        // 发送超时时间
	// 移除了程序员应该控制的字段: FlushBytes, RetryMax, BatchSize, BufferSize, Idempotent, MaxMessageBytes, PartitionerType
}

// ConsumerConfig 消费者配置 - 用户配置层（简化）
// 只包含用户需要关心的核心配置字段
type ConsumerConfig struct {
	GroupID           string        `mapstructure:"groupId"`           // 消费者组ID
	AutoOffsetReset   string        `mapstructure:"autoOffsetReset"`   // 偏移量重置策略 (earliest, latest, none)
	SessionTimeout    time.Duration `mapstructure:"sessionTimeout"`    // 会话超时时间
	HeartbeatInterval time.Duration `mapstructure:"heartbeatInterval"` // 心跳间隔
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
	// 1. 基础配置默认值
	if c.Type == "" {
		c.Type = "memory" // 默认使用内存实现
	}

	// 2. 健康检查默认值
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

	// 3. 发布端默认值
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

	// 发布端积压检测默认值
	if c.Publisher.BacklogDetection.Enabled {
		if c.Publisher.BacklogDetection.MaxQueueDepth == 0 {
			c.Publisher.BacklogDetection.MaxQueueDepth = 10000
		}
		if c.Publisher.BacklogDetection.MaxPublishLatency == 0 {
			c.Publisher.BacklogDetection.MaxPublishLatency = 1 * time.Second
		}
		if c.Publisher.BacklogDetection.RateThreshold == 0 {
			c.Publisher.BacklogDetection.RateThreshold = 1000.0 // 1000 msg/sec
		}
		if c.Publisher.BacklogDetection.CheckInterval == 0 {
			c.Publisher.BacklogDetection.CheckInterval = 10 * time.Second
		}
	}

	// 发布端流量控制默认值
	if c.Publisher.RateLimit.Enabled {
		if c.Publisher.RateLimit.RatePerSecond == 0 {
			c.Publisher.RateLimit.RatePerSecond = 1000.0 // 1000 msg/sec
		}
		if c.Publisher.RateLimit.BurstSize == 0 {
			c.Publisher.RateLimit.BurstSize = 100
		}
	}

	// 发布端错误处理默认值
	if c.Publisher.ErrorHandling.DeadLetterTopic != "" {
		if c.Publisher.ErrorHandling.MaxRetryAttempts == 0 {
			c.Publisher.ErrorHandling.MaxRetryAttempts = 3
		}
		if c.Publisher.ErrorHandling.RetryBackoffBase == 0 {
			c.Publisher.ErrorHandling.RetryBackoffBase = 1 * time.Second
		}
		if c.Publisher.ErrorHandling.RetryBackoffMax == 0 {
			c.Publisher.ErrorHandling.RetryBackoffMax = 30 * time.Second
		}
	}

	// 4. 订阅端默认值
	if c.Subscriber.MaxConcurrency == 0 {
		c.Subscriber.MaxConcurrency = 10
	}
	if c.Subscriber.ProcessTimeout == 0 {
		c.Subscriber.ProcessTimeout = 30 * time.Second
	}

	// 订阅端积压检测默认值
	if c.Subscriber.BacklogDetection.Enabled {
		if c.Subscriber.BacklogDetection.MaxLagThreshold == 0 {
			c.Subscriber.BacklogDetection.MaxLagThreshold = 10000
		}
		if c.Subscriber.BacklogDetection.MaxTimeThreshold == 0 {
			c.Subscriber.BacklogDetection.MaxTimeThreshold = 5 * time.Minute
		}
		if c.Subscriber.BacklogDetection.CheckInterval == 0 {
			c.Subscriber.BacklogDetection.CheckInterval = 30 * time.Second
		}
	}

	// 订阅端流量控制默认值
	if c.Subscriber.RateLimit.Enabled {
		if c.Subscriber.RateLimit.RatePerSecond == 0 {
			c.Subscriber.RateLimit.RatePerSecond = 1000.0 // 1000 msg/sec
		}
		if c.Subscriber.RateLimit.BurstSize == 0 {
			c.Subscriber.RateLimit.BurstSize = 100
		}
	}

	// 订阅端错误处理默认值
	if c.Subscriber.ErrorHandling.DeadLetterTopic != "" {
		if c.Subscriber.ErrorHandling.MaxRetryAttempts == 0 {
			c.Subscriber.ErrorHandling.MaxRetryAttempts = 3
		}
		if c.Subscriber.ErrorHandling.RetryBackoffBase == 0 {
			c.Subscriber.ErrorHandling.RetryBackoffBase = 1 * time.Second
		}
		if c.Subscriber.ErrorHandling.RetryBackoffMax == 0 {
			c.Subscriber.ErrorHandling.RetryBackoffMax = 30 * time.Second
		}
	}

	// 5. 监控配置默认值
	if c.Monitoring.Enabled {
		if c.Monitoring.CollectInterval == 0 {
			c.Monitoring.CollectInterval = 30 * time.Second
		}
	}

	// 6. 根据EventBus类型设置特定默认值
	switch c.Type {
	case "kafka":
		c.setKafkaDefaults()
	case "nats":
		c.setNATSDefaults()
	case "memory":
		c.setMemoryDefaults()
	}
}

// setKafkaDefaults 设置Kafka特定的默认值
func (c *EventBusConfig) setKafkaDefaults() {
	// Producer默认值
	if c.Kafka.Producer.RequiredAcks == 0 {
		c.Kafka.Producer.RequiredAcks = 1 // Leader确认
	}
	if c.Kafka.Producer.Compression == "" {
		c.Kafka.Producer.Compression = "snappy"
	}
	if c.Kafka.Producer.FlushFrequency == 0 {
		c.Kafka.Producer.FlushFrequency = 500 * time.Millisecond
	}
	if c.Kafka.Producer.FlushMessages == 0 {
		c.Kafka.Producer.FlushMessages = 100
	}
	if c.Kafka.Producer.Timeout == 0 {
		c.Kafka.Producer.Timeout = 10 * time.Second
	}

	// Consumer默认值
	if c.Kafka.Consumer.AutoOffsetReset == "" {
		c.Kafka.Consumer.AutoOffsetReset = "latest"
	}
	if c.Kafka.Consumer.SessionTimeout == 0 {
		c.Kafka.Consumer.SessionTimeout = 30 * time.Second
	}
	if c.Kafka.Consumer.HeartbeatInterval == 0 {
		c.Kafka.Consumer.HeartbeatInterval = 3 * time.Second
	}
}

// setNATSDefaults 设置NATS特定的默认值
func (c *EventBusConfig) setNATSDefaults() {
	// 基础连接默认值
	if c.NATS.MaxReconnects == 0 {
		c.NATS.MaxReconnects = 10
	}
	if c.NATS.ReconnectWait == 0 {
		c.NATS.ReconnectWait = 2 * time.Second
	}
	if c.NATS.ConnectionTimeout == 0 {
		c.NATS.ConnectionTimeout = 10 * time.Second
	}

	// JetStream默认值
	if c.NATS.JetStream.Enabled {
		if c.NATS.JetStream.PublishTimeout == 0 {
			c.NATS.JetStream.PublishTimeout = 10 * time.Second
		}
		if c.NATS.JetStream.AckWait == 0 {
			c.NATS.JetStream.AckWait = 30 * time.Second
		}
		if c.NATS.JetStream.MaxDeliver == 0 {
			c.NATS.JetStream.MaxDeliver = 5
		}

		// Stream默认值
		if c.NATS.JetStream.Stream.Retention == "" {
			c.NATS.JetStream.Stream.Retention = "limits"
		}
		if c.NATS.JetStream.Stream.Storage == "" {
			c.NATS.JetStream.Stream.Storage = "file"
		}
		if c.NATS.JetStream.Stream.Replicas == 0 {
			c.NATS.JetStream.Stream.Replicas = 1
		}
		if c.NATS.JetStream.Stream.MaxAge == 0 {
			c.NATS.JetStream.Stream.MaxAge = 24 * time.Hour
		}
		if c.NATS.JetStream.Stream.MaxBytes == 0 {
			c.NATS.JetStream.Stream.MaxBytes = 1024 * 1024 * 1024 // 1GB
		}
		if c.NATS.JetStream.Stream.MaxMsgs == 0 {
			c.NATS.JetStream.Stream.MaxMsgs = 1000000 // 1M messages
		}
		if c.NATS.JetStream.Stream.Discard == "" {
			c.NATS.JetStream.Stream.Discard = "old"
		}

		// Consumer默认值
		if c.NATS.JetStream.Consumer.DeliverPolicy == "" {
			c.NATS.JetStream.Consumer.DeliverPolicy = "all"
		}
		if c.NATS.JetStream.Consumer.AckPolicy == "" {
			c.NATS.JetStream.Consumer.AckPolicy = "explicit"
		}
		if c.NATS.JetStream.Consumer.ReplayPolicy == "" {
			c.NATS.JetStream.Consumer.ReplayPolicy = "instant"
		}
		if c.NATS.JetStream.Consumer.MaxAckPending == 0 {
			c.NATS.JetStream.Consumer.MaxAckPending = 1000
		}
		if c.NATS.JetStream.Consumer.MaxWaiting == 0 {
			c.NATS.JetStream.Consumer.MaxWaiting = 512
		}
		if c.NATS.JetStream.Consumer.MaxDeliver == 0 {
			c.NATS.JetStream.Consumer.MaxDeliver = 5
		}
	}
}

// setMemoryDefaults 设置Memory特定的默认值
func (c *EventBusConfig) setMemoryDefaults() {
	if c.Memory.MaxChannelSize == 0 {
		c.Memory.MaxChannelSize = 1000
	}
	if c.Memory.BufferSize == 0 {
		c.Memory.BufferSize = 100
	}
}

// Validate 验证EventBusConfig配置
func (c *EventBusConfig) Validate() error {
	// 1. 验证基础配置
	if c.Type == "" {
		return fmt.Errorf("eventbus type is required")
	}

	if c.Type != "kafka" && c.Type != "nats" && c.Type != "memory" {
		return fmt.Errorf("unsupported eventbus type: %s", c.Type)
	}

	if c.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}

	// 2. 验证健康检查配置
	if c.HealthCheck.Enabled {
		if c.HealthCheck.Publisher.Interval <= 0 {
			return fmt.Errorf("health check publisher interval must be positive")
		}
		if c.HealthCheck.Publisher.Timeout <= 0 {
			return fmt.Errorf("health check publisher timeout must be positive")
		}
		if c.HealthCheck.Publisher.FailureThreshold <= 0 {
			return fmt.Errorf("health check publisher failure threshold must be positive")
		}
		if c.HealthCheck.Publisher.MessageTTL <= 0 {
			return fmt.Errorf("health check publisher message TTL must be positive")
		}

		if c.HealthCheck.Subscriber.MonitorInterval <= 0 {
			return fmt.Errorf("health check subscriber monitor interval must be positive")
		}
		if c.HealthCheck.Subscriber.WarningThreshold <= 0 {
			return fmt.Errorf("health check subscriber warning threshold must be positive")
		}
		if c.HealthCheck.Subscriber.ErrorThreshold <= 0 {
			return fmt.Errorf("health check subscriber error threshold must be positive")
		}
		if c.HealthCheck.Subscriber.CriticalThreshold <= 0 {
			return fmt.Errorf("health check subscriber critical threshold must be positive")
		}
		// 验证阈值递增关系
		if c.HealthCheck.Subscriber.WarningThreshold >= c.HealthCheck.Subscriber.ErrorThreshold {
			return fmt.Errorf("warning threshold must be less than error threshold")
		}
		if c.HealthCheck.Subscriber.ErrorThreshold >= c.HealthCheck.Subscriber.CriticalThreshold {
			return fmt.Errorf("error threshold must be less than critical threshold")
		}
	}

	// 3. 验证发布端配置
	if c.Publisher.MaxReconnectAttempts < 0 {
		return fmt.Errorf("publisher max reconnect attempts must be non-negative")
	}
	if c.Publisher.InitialBackoff < 0 {
		return fmt.Errorf("publisher initial backoff must be non-negative")
	}
	if c.Publisher.MaxBackoff < 0 {
		return fmt.Errorf("publisher max backoff must be non-negative")
	}
	if c.Publisher.InitialBackoff > c.Publisher.MaxBackoff && c.Publisher.MaxBackoff > 0 {
		return fmt.Errorf("publisher initial backoff must be less than or equal to max backoff")
	}
	if c.Publisher.PublishTimeout <= 0 {
		return fmt.Errorf("publisher publish timeout must be positive")
	}

	// 验证发布端积压检测
	if c.Publisher.BacklogDetection.Enabled {
		if c.Publisher.BacklogDetection.MaxQueueDepth <= 0 {
			return fmt.Errorf("publisher backlog detection max queue depth must be positive")
		}
		if c.Publisher.BacklogDetection.MaxPublishLatency <= 0 {
			return fmt.Errorf("publisher backlog detection max publish latency must be positive")
		}
		if c.Publisher.BacklogDetection.RateThreshold <= 0 {
			return fmt.Errorf("publisher backlog detection rate threshold must be positive")
		}
		if c.Publisher.BacklogDetection.CheckInterval <= 0 {
			return fmt.Errorf("publisher backlog detection check interval must be positive")
		}
	}

	// 验证发布端流量控制
	if c.Publisher.RateLimit.Enabled {
		if c.Publisher.RateLimit.RatePerSecond <= 0 {
			return fmt.Errorf("publisher rate limit rate per second must be positive")
		}
		if c.Publisher.RateLimit.BurstSize <= 0 {
			return fmt.Errorf("publisher rate limit burst size must be positive")
		}
	}

	// 验证发布端错误处理
	if c.Publisher.ErrorHandling.DeadLetterTopic != "" {
		if c.Publisher.ErrorHandling.MaxRetryAttempts < 0 {
			return fmt.Errorf("publisher error handling max retry attempts must be non-negative")
		}
		if c.Publisher.ErrorHandling.RetryBackoffBase < 0 {
			return fmt.Errorf("publisher error handling retry backoff base must be non-negative")
		}
		if c.Publisher.ErrorHandling.RetryBackoffMax < 0 {
			return fmt.Errorf("publisher error handling retry backoff max must be non-negative")
		}
		if c.Publisher.ErrorHandling.RetryBackoffBase > c.Publisher.ErrorHandling.RetryBackoffMax && c.Publisher.ErrorHandling.RetryBackoffMax > 0 {
			return fmt.Errorf("publisher error handling retry backoff base must be less than or equal to max")
		}
	}

	// 4. 验证订阅端配置
	if c.Subscriber.MaxConcurrency <= 0 {
		return fmt.Errorf("subscriber max concurrency must be positive")
	}
	if c.Subscriber.ProcessTimeout <= 0 {
		return fmt.Errorf("subscriber process timeout must be positive")
	}

	// 验证订阅端积压检测
	if c.Subscriber.BacklogDetection.Enabled {
		if c.Subscriber.BacklogDetection.MaxLagThreshold <= 0 {
			return fmt.Errorf("subscriber backlog detection max lag threshold must be positive")
		}
		if c.Subscriber.BacklogDetection.MaxTimeThreshold <= 0 {
			return fmt.Errorf("subscriber backlog detection max time threshold must be positive")
		}
		if c.Subscriber.BacklogDetection.CheckInterval <= 0 {
			return fmt.Errorf("subscriber backlog detection check interval must be positive")
		}
	}

	// 验证订阅端流量控制
	if c.Subscriber.RateLimit.Enabled {
		if c.Subscriber.RateLimit.RatePerSecond <= 0 {
			return fmt.Errorf("subscriber rate limit rate per second must be positive")
		}
		if c.Subscriber.RateLimit.BurstSize <= 0 {
			return fmt.Errorf("subscriber rate limit burst size must be positive")
		}
	}

	// 验证订阅端错误处理
	if c.Subscriber.ErrorHandling.DeadLetterTopic != "" {
		if c.Subscriber.ErrorHandling.MaxRetryAttempts < 0 {
			return fmt.Errorf("subscriber error handling max retry attempts must be non-negative")
		}
		if c.Subscriber.ErrorHandling.RetryBackoffBase < 0 {
			return fmt.Errorf("subscriber error handling retry backoff base must be non-negative")
		}
		if c.Subscriber.ErrorHandling.RetryBackoffMax < 0 {
			return fmt.Errorf("subscriber error handling retry backoff max must be non-negative")
		}
		if c.Subscriber.ErrorHandling.RetryBackoffBase > c.Subscriber.ErrorHandling.RetryBackoffMax && c.Subscriber.ErrorHandling.RetryBackoffMax > 0 {
			return fmt.Errorf("subscriber error handling retry backoff base must be less than or equal to max")
		}
	}

	// 5. 验证安全配置
	if c.Security.Enabled {
		if c.Security.Protocol == "" {
			return fmt.Errorf("security protocol is required when security is enabled")
		}
		// 验证协议类型
		validProtocols := map[string]bool{"SASL_PLAINTEXT": true, "SASL_SSL": true, "SSL": true}
		if !validProtocols[c.Security.Protocol] {
			return fmt.Errorf("unsupported security protocol: %s (supported: SASL_PLAINTEXT, SASL_SSL, SSL)", c.Security.Protocol)
		}
	}

	// 6. 根据EventBus类型验证特定配置
	switch c.Type {
	case "kafka":
		if err := c.validateKafkaConfig(); err != nil {
			return err
		}
	case "nats":
		if err := c.validateNATSConfig(); err != nil {
			return err
		}
	case "memory":
		if err := c.validateMemoryConfig(); err != nil {
			return err
		}
	}

	return nil
}

// validateKafkaConfig 验证Kafka配置
func (c *EventBusConfig) validateKafkaConfig() error {
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}

	// 验证生产者配置
	if c.Kafka.Producer.RequiredAcks < -1 || c.Kafka.Producer.RequiredAcks > 1 {
		return fmt.Errorf("kafka producer required acks must be -1, 0, or 1")
	}

	if c.Kafka.Producer.Compression != "" {
		validCompressions := map[string]bool{"none": true, "gzip": true, "snappy": true, "lz4": true, "zstd": true}
		if !validCompressions[c.Kafka.Producer.Compression] {
			return fmt.Errorf("unsupported kafka compression: %s (supported: none, gzip, snappy, lz4, zstd)", c.Kafka.Producer.Compression)
		}
	}

	if c.Kafka.Producer.FlushFrequency < 0 {
		return fmt.Errorf("kafka producer flush frequency must be non-negative")
	}
	if c.Kafka.Producer.FlushMessages < 0 {
		return fmt.Errorf("kafka producer flush messages must be non-negative")
	}
	if c.Kafka.Producer.Timeout < 0 {
		return fmt.Errorf("kafka producer timeout must be non-negative")
	}

	// 验证消费者配置
	if c.Kafka.Consumer.GroupID == "" {
		return fmt.Errorf("kafka consumer group ID is required")
	}

	if c.Kafka.Consumer.AutoOffsetReset != "" {
		validResets := map[string]bool{"earliest": true, "latest": true, "none": true}
		if !validResets[c.Kafka.Consumer.AutoOffsetReset] {
			return fmt.Errorf("unsupported kafka auto offset reset: %s (supported: earliest, latest, none)", c.Kafka.Consumer.AutoOffsetReset)
		}
	}

	if c.Kafka.Consumer.SessionTimeout < 0 {
		return fmt.Errorf("kafka consumer session timeout must be non-negative")
	}
	if c.Kafka.Consumer.HeartbeatInterval < 0 {
		return fmt.Errorf("kafka consumer heartbeat interval must be non-negative")
	}

	return nil
}

// validateNATSConfig 验证NATS配置
func (c *EventBusConfig) validateNATSConfig() error {
	if len(c.NATS.URLs) == 0 {
		return fmt.Errorf("nats URLs are required")
	}

	if c.NATS.MaxReconnects < -1 {
		return fmt.Errorf("nats max reconnects must be -1 (unlimited) or non-negative")
	}
	if c.NATS.ReconnectWait < 0 {
		return fmt.Errorf("nats reconnect wait must be non-negative")
	}
	if c.NATS.ConnectionTimeout < 0 {
		return fmt.Errorf("nats connection timeout must be non-negative")
	}

	// 验证JetStream配置
	if c.NATS.JetStream.Enabled {
		if c.NATS.JetStream.PublishTimeout < 0 {
			return fmt.Errorf("nats jetstream publish timeout must be non-negative")
		}
		if c.NATS.JetStream.AckWait < 0 {
			return fmt.Errorf("nats jetstream ack wait must be non-negative")
		}
		if c.NATS.JetStream.MaxDeliver < 0 {
			return fmt.Errorf("nats jetstream max deliver must be non-negative")
		}

		// 验证流配置
		if c.NATS.JetStream.Stream.Name == "" {
			return fmt.Errorf("nats jetstream stream name is required")
		}
		if len(c.NATS.JetStream.Stream.Subjects) == 0 {
			return fmt.Errorf("nats jetstream stream subjects are required")
		}

		if c.NATS.JetStream.Stream.Retention != "" {
			validRetentions := map[string]bool{"limits": true, "interest": true, "workqueue": true}
			if !validRetentions[c.NATS.JetStream.Stream.Retention] {
				return fmt.Errorf("unsupported nats jetstream retention: %s (supported: limits, interest, workqueue)", c.NATS.JetStream.Stream.Retention)
			}
		}

		if c.NATS.JetStream.Stream.Storage != "" {
			validStorages := map[string]bool{"file": true, "memory": true}
			if !validStorages[c.NATS.JetStream.Stream.Storage] {
				return fmt.Errorf("unsupported nats jetstream storage: %s (supported: file, memory)", c.NATS.JetStream.Stream.Storage)
			}
		}

		if c.NATS.JetStream.Stream.Replicas < 0 {
			return fmt.Errorf("nats jetstream stream replicas must be non-negative")
		}
		if c.NATS.JetStream.Stream.MaxAge < 0 {
			return fmt.Errorf("nats jetstream stream max age must be non-negative")
		}
		if c.NATS.JetStream.Stream.MaxBytes < 0 {
			return fmt.Errorf("nats jetstream stream max bytes must be non-negative")
		}
		if c.NATS.JetStream.Stream.MaxMsgs < 0 {
			return fmt.Errorf("nats jetstream stream max msgs must be non-negative")
		}

		if c.NATS.JetStream.Stream.Discard != "" {
			validDiscards := map[string]bool{"old": true, "new": true}
			if !validDiscards[c.NATS.JetStream.Stream.Discard] {
				return fmt.Errorf("unsupported nats jetstream discard: %s (supported: old, new)", c.NATS.JetStream.Stream.Discard)
			}
		}

		// 验证消费者配置
		if c.NATS.JetStream.Consumer.DeliverPolicy != "" {
			validPolicies := map[string]bool{"all": true, "last": true, "new": true, "by_start_sequence": true, "by_start_time": true}
			if !validPolicies[c.NATS.JetStream.Consumer.DeliverPolicy] {
				return fmt.Errorf("unsupported nats jetstream deliver policy: %s (supported: all, last, new, by_start_sequence, by_start_time)", c.NATS.JetStream.Consumer.DeliverPolicy)
			}
		}

		if c.NATS.JetStream.Consumer.AckPolicy != "" {
			validAckPolicies := map[string]bool{"none": true, "all": true, "explicit": true}
			if !validAckPolicies[c.NATS.JetStream.Consumer.AckPolicy] {
				return fmt.Errorf("unsupported nats jetstream ack policy: %s (supported: none, all, explicit)", c.NATS.JetStream.Consumer.AckPolicy)
			}
		}

		if c.NATS.JetStream.Consumer.ReplayPolicy != "" {
			validReplayPolicies := map[string]bool{"instant": true, "original": true}
			if !validReplayPolicies[c.NATS.JetStream.Consumer.ReplayPolicy] {
				return fmt.Errorf("unsupported nats jetstream replay policy: %s (supported: instant, original)", c.NATS.JetStream.Consumer.ReplayPolicy)
			}
		}

		if c.NATS.JetStream.Consumer.MaxAckPending < 0 {
			return fmt.Errorf("nats jetstream consumer max ack pending must be non-negative")
		}
		if c.NATS.JetStream.Consumer.MaxWaiting < 0 {
			return fmt.Errorf("nats jetstream consumer max waiting must be non-negative")
		}
		if c.NATS.JetStream.Consumer.MaxDeliver < 0 {
			return fmt.Errorf("nats jetstream consumer max deliver must be non-negative")
		}
	}

	// 验证安全配置
	if c.NATS.Security.Enabled {
		// 至少需要一种认证方式
		hasAuth := c.NATS.Security.Token != "" ||
			(c.NATS.Security.Username != "" && c.NATS.Security.Password != "") ||
			c.NATS.Security.NKeyFile != "" ||
			c.NATS.Security.CredFile != ""

		if !hasAuth {
			return fmt.Errorf("nats security enabled but no authentication method provided (token, username/password, nkey, or cred file)")
		}
	}

	return nil
}

// validateMemoryConfig 验证Memory配置
func (c *EventBusConfig) validateMemoryConfig() error {
	if c.Memory.MaxChannelSize < 0 {
		return fmt.Errorf("memory max channel size must be non-negative")
	}
	if c.Memory.BufferSize < 0 {
		return fmt.Errorf("memory buffer size must be non-negative")
	}

	return nil
}
