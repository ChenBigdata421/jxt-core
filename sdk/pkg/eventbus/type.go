package eventbus

import (
	"context"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
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

// PublishResult 异步发布结果
type PublishResult struct {
	// EventID 事件ID（来自Envelope.EventID或自定义ID）
	EventID string
	// Topic 主题
	Topic string
	// Success 是否成功
	Success bool
	// Error 错误信息（失败时）
	Error error
	// Timestamp 发布时间戳
	Timestamp time.Time
	// AggregateID 聚合ID（可选，来自Envelope）
	AggregateID string
	// EventType 事件类型（可选，来自Envelope）
	EventType string
}

// EventBus 统一事件总线接口（合并基础功能和企业特性）
type EventBus interface {
	// ========== 基础功能 ==========
	// Publish 发布普通消息到指定主题
	// ⚠️ 注意：不支持 Outbox 模式，消息容许丢失
	// 适用场景：通知、缓存失效、系统事件等可容忍丢失的消息
	// 如需可靠投递和 Outbox 模式支持，请使用 PublishEnvelope()
	Publish(ctx context.Context, topic string, message []byte) error

	// Subscribe 订阅原始消息（不使用Keyed-Worker池）
	// 特点：直接并发处理，极致性能，无顺序保证
	// 适用：简单消息、通知、缓存失效等不需要顺序的场景
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error
	// 关闭连接
	Close() error
	// 注册重连回调
	RegisterReconnectCallback(callback ReconnectCallback) error

	// ========== 生命周期管理 ==========
	// 启动事件总线（根据配置启用相应功能）
	Start(ctx context.Context) error
	// 停止事件总线
	Stop() error

	// ========== 高级发布功能（可选启用） ==========
	// 使用选项发布消息
	PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error
	// 设置消息格式化器
	SetMessageFormatter(formatter MessageFormatter) error
	// 注册发布回调
	RegisterPublishCallback(callback PublishCallback) error

	// ========== 高级订阅功能（可选启用） ==========
	// 使用选项订阅消息
	SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error

	// ========== 积压检测功能 ==========
	// 注册订阅端积压回调
	RegisterSubscriberBacklogCallback(callback BacklogStateCallback) error
	// 启动订阅端积压监控
	StartSubscriberBacklogMonitoring(ctx context.Context) error
	// 停止订阅端积压监控
	StopSubscriberBacklogMonitoring() error

	// 注册发送端积压回调
	RegisterPublisherBacklogCallback(callback PublisherBacklogCallback) error
	// 启动发送端积压监控
	StartPublisherBacklogMonitoring(ctx context.Context) error
	// 停止发送端积压监控
	StopPublisherBacklogMonitoring() error

	// 根据配置启动所有积压监控（发送端和/或订阅端）
	StartAllBacklogMonitoring(ctx context.Context) error
	// 停止所有积压监控
	StopAllBacklogMonitoring() error

	// 设置消息路由器
	SetMessageRouter(router MessageRouter) error
	// 设置错误处理器
	SetErrorHandler(handler ErrorHandler) error
	// 注册订阅回调
	RegisterSubscriptionCallback(callback SubscriptionCallback) error

	// ========== 健康检查功能（分离发布端和订阅端） ==========
	// 发布端健康检查（发送健康检查消息）
	StartHealthCheckPublisher(ctx context.Context) error
	StopHealthCheckPublisher() error
	GetHealthCheckPublisherStatus() HealthCheckStatus
	RegisterHealthCheckPublisherCallback(callback HealthCheckCallback) error

	// 订阅端健康检查（监控健康检查消息）
	StartHealthCheckSubscriber(ctx context.Context) error
	StopHealthCheckSubscriber() error
	GetHealthCheckSubscriberStats() HealthCheckSubscriberStats
	RegisterHealthCheckSubscriberCallback(callback HealthCheckAlertCallback) error

	// 根据配置启动所有健康检查（发布端和/或订阅端）
	StartAllHealthCheck(ctx context.Context) error
	// 停止所有健康检查
	StopAllHealthCheck() error

	// 向后兼容的统一接口（已废弃，建议使用分离的接口）
	StartHealthCheck(ctx context.Context) error // 已废弃，使用StartHealthCheckPublisher
	StopHealthCheck() error                     // 已废弃，使用StopHealthCheckPublisher
	GetHealthStatus() HealthCheckStatus         // 已废弃，使用GetHealthCheckPublisherStatus
	// 获取连接状态
	GetConnectionState() ConnectionState
	// 获取监控指标
	GetMetrics() Metrics

	// ========== 主题持久化管理 ==========
	// ConfigureTopic 配置主题的持久化策略和其他选项
	// 必须在首次使用主题前调用，用于设置主题的持久化行为
	ConfigureTopic(ctx context.Context, topic string, options TopicOptions) error

	// SetTopicPersistence 设置主题是否持久化（简化接口）
	// persistent: true=持久化存储, false=内存存储（非持久化）
	SetTopicPersistence(ctx context.Context, topic string, persistent bool) error

	// GetTopicConfig 获取主题的当前配置
	GetTopicConfig(topic string) (TopicOptions, error)

	// ListConfiguredTopics 列出所有已配置的主题
	ListConfiguredTopics() []string

	// RemoveTopicConfig 移除主题配置（恢复为默认行为）
	RemoveTopicConfig(topic string) error

	// ========== 主题配置策略管理 ==========
	// SetTopicConfigStrategy 设置主题配置策略
	// 策略类型：
	//   - StrategyCreateOnly: 只创建，不更新（生产环境推荐）
	//   - StrategyCreateOrUpdate: 创建或更新（开发环境推荐）
	//   - StrategyValidateOnly: 只验证，不修改（严格模式）
	//   - StrategySkip: 跳过检查（性能优先）
	SetTopicConfigStrategy(strategy TopicConfigStrategy)

	// GetTopicConfigStrategy 获取当前主题配置策略
	GetTopicConfigStrategy() TopicConfigStrategy

	// ========== Envelope 支持（可选使用） ==========
	// PublishEnvelope 发布Envelope消息（领域事件）
	// ✅ 支持 Outbox 模式：通过 GetPublishResultChannel() 获取 ACK 结果
	// ✅ 可靠投递：不容许丢失的领域事件必须使用此方法
	// 适用场景：订单创建、支付完成、库存变更等关键业务事件
	// 与 Publish() 的区别：
	//   - PublishEnvelope(): 支持 Outbox 模式，发送 ACK 结果到 publishResultChan
	//   - Publish(): 不支持 Outbox 模式，消息容许丢失
	PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error

	// SubscribeEnvelope 订阅Envelope消息（自动使用Keyed-Worker池）
	// 特点：按聚合ID顺序处理，事件溯源支持，毫秒级延迟
	// 适用：领域事件、事件溯源、聚合管理等需要顺序保证的场景
	SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error

	// ========== 异步发布结果处理（用于Outbox模式） ==========
	// GetPublishResultChannel 获取异步发布结果通道
	// ⚠️ 仅 PublishEnvelope() 发送 ACK 结果到此通道
	// ⚠️ Publish() 不发送 ACK 结果（不支持 Outbox 模式）
	// 用于 Outbox Processor 监听发布结果并更新 Outbox 状态
	GetPublishResultChannel() <-chan *PublishResult
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

// TopicPersistenceMode 主题持久化模式
type TopicPersistenceMode string

const (
	// TopicPersistent 持久化存储（使用JetStream/Kafka等持久化机制）
	TopicPersistent TopicPersistenceMode = "persistent"
	// TopicEphemeral 非持久化存储（使用Core NATS/内存等非持久化机制）
	TopicEphemeral TopicPersistenceMode = "ephemeral"
	// TopicAuto 自动选择（根据EventBus全局配置决定）
	TopicAuto TopicPersistenceMode = "auto"
)

// TopicOptions 主题配置选项
type TopicOptions struct {
	// PersistenceMode 持久化模式
	PersistenceMode TopicPersistenceMode `json:"persistenceMode"`
	// RetentionTime 消息保留时间（仅持久化模式有效）
	RetentionTime time.Duration `json:"retentionTime,omitempty"`
	// MaxSize 主题最大存储大小（仅持久化模式有效）
	MaxSize int64 `json:"maxSize,omitempty"`
	// MaxMessages 主题最大消息数量（仅持久化模式有效）
	MaxMessages int64 `json:"maxMessages,omitempty"`
	// Replicas 副本数量（仅分布式存储有效，如Kafka）
	Replicas int `json:"replicas,omitempty"`
	// Description 主题描述（可选）
	Description string `json:"description,omitempty"`
}

// DefaultTopicOptions 返回默认的主题配置
func DefaultTopicOptions() TopicOptions {
	return TopicOptions{
		PersistenceMode: TopicAuto,
		RetentionTime:   24 * time.Hour,    // 默认保留24小时
		MaxSize:         100 * 1024 * 1024, // 默认100MB
		MaxMessages:     10000,             // 默认1万条消息
		Replicas:        1,                 // 默认单副本
	}
}

// IsPersistent 判断是否为持久化模式
func (opts TopicOptions) IsPersistent(globalJetStreamEnabled bool) bool {
	switch opts.PersistenceMode {
	case TopicPersistent:
		return true
	case TopicEphemeral:
		return false
	case TopicAuto:
		return globalJetStreamEnabled
	default:
		return globalJetStreamEnabled
	}
}

// EventBusConfig 事件总线配置
type EventBusConfig struct {
	Type       string           `mapstructure:"type"` // kafka, nats, memory
	Kafka      KafkaConfig      `mapstructure:"kafka"`
	NATS       NATSConfig       `mapstructure:"nats"`
	Metrics    MetricsConfig    `mapstructure:"metrics"`
	Tracing    TracingConfig    `mapstructure:"tracing"`
	Enterprise EnterpriseConfig `mapstructure:"enterprise"` // 企业特性配置
}

// EnterpriseConfig 企业特性配置
type EnterpriseConfig struct {
	// 发布端企业特性
	Publisher PublisherEnterpriseConfig `mapstructure:"publisher"`

	// 订阅端企业特性
	Subscriber SubscriberEnterpriseConfig `mapstructure:"subscriber"`

	// 统一企业特性
	HealthCheck HealthCheckConfig `mapstructure:"healthCheck"`
	Monitoring  MonitoringConfig  `mapstructure:"monitoring"`
}

// PublisherEnterpriseConfig 发布端企业特性配置
type PublisherEnterpriseConfig struct {
	// 发送端积压检测
	BacklogDetection config.PublisherBacklogDetectionConfig `mapstructure:"backlogDetection"`

	// 消息格式化
	MessageFormatter MessageFormatterConfig `mapstructure:"messageFormatter"`

	// 发布回调
	PublishCallback PublishCallbackConfig `mapstructure:"publishCallback"`

	// 重试策略
	RetryPolicy RetryPolicyConfig `mapstructure:"retryPolicy"`

	// 流量控制
	RateLimit RateLimitConfig `mapstructure:"rateLimit"`

	// 错误处理
	ErrorHandling config.ErrorHandlingConfig `mapstructure:"errorHandling"`
}

// SubscriberEnterpriseConfig 订阅端企业特性配置
type SubscriberEnterpriseConfig struct {
	// 积压检测
	BacklogDetection config.SubscriberBacklogDetectionConfig `mapstructure:"backlogDetection"`

	// 流量控制
	RateLimit RateLimitConfig `mapstructure:"rateLimit"`

	// 死信队列
	DeadLetter DeadLetterConfig `mapstructure:"deadLetter"`

	// 消息路由
	MessageRouter MessageRouterConfig `mapstructure:"messageRouter"`

	// 错误处理
	ErrorHandler ErrorHandlerConfig `mapstructure:"errorHandler"`
}

// MessageFormatterConfig 消息格式化器配置
type MessageFormatterConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Type    string `mapstructure:"type"` // json, protobuf, avro
}

// PublishCallbackConfig 发布回调配置
type PublishCallbackConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// RetryPolicyConfig 重试策略配置
type RetryPolicyConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	MaxRetries      int           `mapstructure:"maxRetries"`
	InitialInterval time.Duration `mapstructure:"initialInterval"`
	MaxInterval     time.Duration `mapstructure:"maxInterval"`
	Multiplier      float64       `mapstructure:"multiplier"`
}

// BacklogDetectionConfig 积压检测配置（使用现有定义）
// 定义在 backlog_detector.go 中

// RateLimitConfig 流量控制配置（使用现有定义）
// 定义在 rate_limiter.go 中

// DeadLetterConfig 死信队列配置
type DeadLetterConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Topic      string `mapstructure:"topic"`      // 死信队列主题
	MaxRetries int    `mapstructure:"maxRetries"` // 最大重试次数
}

// MessageRouterConfig 消息路由配置
type MessageRouterConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Type    string `mapstructure:"type"` // hash, round_robin, custom
}

// ErrorHandlerConfig 错误处理配置
type ErrorHandlerConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Type    string `mapstructure:"type"` // retry, deadletter, skip, fail
}

// HealthCheckConfig 健康检查配置
type HealthCheckConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Topic            string        `mapstructure:"topic"`            // 健康检查主题（可选，默认使用统一主题）
	Interval         time.Duration `mapstructure:"interval"`         // 检查间隔
	Timeout          time.Duration `mapstructure:"timeout"`          // 检查超时
	FailureThreshold int           `mapstructure:"failureThreshold"` // 失败阈值
	MessageTTL       time.Duration `mapstructure:"messageTTL"`       // 消息存活时间
}

// MonitoringConfig 监控配置
type MonitoringConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	MetricsInterval time.Duration `mapstructure:"metricsInterval"`
	ExportEndpoint  string        `mapstructure:"exportEndpoint"`
}

// KafkaConfig Kafka配置 - 程序员配置层（完整）
// 包含所有技术细节和程序员控制的字段
type KafkaConfig struct {
	// 基础配置 (从用户配置转换而来)
	Brokers  []string       `mapstructure:"brokers"`  // Kafka集群地址
	Producer ProducerConfig `mapstructure:"producer"` // 生产者配置
	Consumer ConsumerConfig `mapstructure:"consumer"` // 消费者配置

	// 程序员控制字段 (有合理默认值)
	HealthCheckInterval time.Duration  `mapstructure:"healthCheckInterval"` // 健康检查间隔 (默认: 30s)
	Security            SecurityConfig `mapstructure:"security"`            // 安全配置
	Net                 NetConfig      `mapstructure:"net"`                 // 网络配置

	// 高级功能配置 (程序员专用)
	ClientID             string        `mapstructure:"clientId"`             // 客户端ID (默认: "jxt-eventbus")
	MetadataRefreshFreq  time.Duration `mapstructure:"metadataRefreshFreq"`  // 元数据刷新频率 (默认: 10m)
	MetadataRetryMax     int           `mapstructure:"metadataRetryMax"`     // 元数据重试次数 (默认: 3)
	MetadataRetryBackoff time.Duration `mapstructure:"metadataRetryBackoff"` // 元数据重试退避时间 (默认: 250ms)

	// 企业级特性配置 (从用户配置转换而来)
	Enterprise EnterpriseConfig `mapstructure:"enterprise"` // 企业级特性配置
}

// ProducerConfig 生产者配置 - 程序员配置层（完整）
// 包含所有技术细节和程序员控制的字段
type ProducerConfig struct {
	// 用户配置字段 (从用户配置转换而来)
	RequiredAcks   int           `mapstructure:"requiredAcks"`   // 消息确认级别
	Compression    string        `mapstructure:"compression"`    // 压缩算法
	FlushFrequency time.Duration `mapstructure:"flushFrequency"` // 刷新频率
	FlushMessages  int           `mapstructure:"flushMessages"`  // 批量消息数
	Timeout        time.Duration `mapstructure:"timeout"`        // 发送超时时间

	// 程序员控制字段 (有合理默认值)
	FlushBytes      int    `mapstructure:"flushBytes"`      // 批量字节数 (默认: 1MB)
	RetryMax        int    `mapstructure:"retryMax"`        // 最大重试次数 (默认: 3)
	BatchSize       int    `mapstructure:"batchSize"`       // 批量大小 (默认: 16KB)
	BufferSize      int    `mapstructure:"bufferSize"`      // 缓冲区大小 (默认: 32MB)
	Idempotent      bool   `mapstructure:"idempotent"`      // 幂等性 (默认: true)
	MaxMessageBytes int    `mapstructure:"maxMessageBytes"` // 最大消息字节数 (默认: 1MB)
	PartitionerType string `mapstructure:"partitionerType"` // 分区器类型 (默认: "hash")

	// 高级技术字段 (程序员专用)
	LingerMs         time.Duration `mapstructure:"lingerMs"`         // 延迟发送时间 (默认: 5ms)
	CompressionLevel int           `mapstructure:"compressionLevel"` // 压缩级别 (默认: 6)
	MaxInFlight      int           `mapstructure:"maxInFlight"`      // 最大飞行请求数 (默认: 5)
}

// ConsumerConfig 消费者配置 - 程序员配置层（完整）
// 包含所有技术细节和程序员控制的字段
type ConsumerConfig struct {
	// 用户配置字段 (从用户配置转换而来)
	GroupID           string        `mapstructure:"groupId"`           // 消费者组ID
	AutoOffsetReset   string        `mapstructure:"autoOffsetReset"`   // 偏移量重置策略
	SessionTimeout    time.Duration `mapstructure:"sessionTimeout"`    // 会话超时时间
	HeartbeatInterval time.Duration `mapstructure:"heartbeatInterval"` // 心跳间隔

	// 程序员控制字段 (有合理默认值)
	MaxProcessingTime time.Duration `mapstructure:"maxProcessingTime"` // 最大处理时间 (默认: 30s)
	FetchMinBytes     int           `mapstructure:"fetchMinBytes"`     // 最小获取字节数 (默认: 1KB)
	FetchMaxBytes     int           `mapstructure:"fetchMaxBytes"`     // 最大获取字节数 (默认: 50MB)
	FetchMaxWait      time.Duration `mapstructure:"fetchMaxWait"`      // 最大等待时间 (默认: 500ms)

	// 高级技术字段 (程序员专用)
	MaxPollRecords     int           `mapstructure:"maxPollRecords"`     // 最大轮询记录数 (默认: 500)
	EnableAutoCommit   bool          `mapstructure:"enableAutoCommit"`   // 启用自动提交 (默认: false)
	AutoCommitInterval time.Duration `mapstructure:"autoCommitInterval"` // 自动提交间隔 (默认: 5s)
	IsolationLevel     string        `mapstructure:"isolationLevel"`     // 隔离级别 (默认: "read_committed")
	RebalanceStrategy  string        `mapstructure:"rebalanceStrategy"`  // 再平衡策略 (默认: "range")
}

// NetConfig 网络配置 - 程序员专用配置
// 用户不需要关心这些底层网络参数，由程序员设定合理默认值
type NetConfig struct {
	DialTimeout  time.Duration `mapstructure:"dialTimeout"`  // 连接超时 (默认: 30s)
	ReadTimeout  time.Duration `mapstructure:"readTimeout"`  // 读取超时 (默认: 30s)
	WriteTimeout time.Duration `mapstructure:"writeTimeout"` // 写入超时 (默认: 30s)
	KeepAlive    time.Duration `mapstructure:"keepAlive"`    // 保活时间 (默认: 30s)
	MaxIdleConns int           `mapstructure:"maxIdleConns"` // 最大空闲连接数 (默认: 10)
	MaxOpenConns int           `mapstructure:"maxOpenConns"` // 最大打开连接数 (默认: 100)
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

	// 企业级特性配置 (从用户配置转换而来)
	Enterprise EnterpriseConfig `mapstructure:"enterprise"` // 企业级特性配置
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

// ========== 企业特性相关类型定义 ==========

// SubscribeOptions 订阅选项
type SubscribeOptions struct {
	ProcessingTimeout time.Duration `json:"processingTimeout"` // 处理超时时间
	RateLimit         float64       `json:"rateLimit"`         // 速率限制（每秒）
	RateBurst         int           `json:"rateBurst"`         // 突发大小
	MaxRetries        int           `json:"maxRetries"`        // 最大重试次数
	RetryBackoff      time.Duration `json:"retryBackoff"`      // 重试退避时间
	DeadLetterEnabled bool          `json:"deadLetterEnabled"` // 是否启用死信队列
	RetryPolicy       RetryPolicy   `json:"retryPolicy"`       // 重试策略
}

// PublishOptions 发布选项
type PublishOptions struct {
	MessageFormatter MessageFormatter  `json:"-"`           // 消息格式化器
	Metadata         map[string]string `json:"metadata"`    // 元数据
	RetryPolicy      RetryPolicy       `json:"retryPolicy"` // 重试策略
	Timeout          time.Duration     `json:"timeout"`     // 超时时间
	AggregateID      interface{}       `json:"aggregateId"` // 聚合ID
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries      int           `json:"maxRetries"`      // 最大重试次数
	InitialInterval time.Duration `json:"initialInterval"` // 初始间隔
	MaxInterval     time.Duration `json:"maxInterval"`     // 最大间隔
	Multiplier      float64       `json:"multiplier"`      // 倍数
}

// ConnectionState 连接状态
type ConnectionState struct {
	IsConnected       bool      `json:"isConnected"`
	LastConnectedTime time.Time `json:"lastConnectedTime"`
	ReconnectCount    int       `json:"reconnectCount"`
	LastError         string    `json:"lastError,omitempty"`
}

// HealthCheckStatus 健康检查状态
type HealthCheckStatus struct {
	IsHealthy           bool      `json:"isHealthy"`
	ConsecutiveFailures int       `json:"consecutiveFailures"`
	LastSuccessTime     time.Time `json:"lastSuccessTime"`
	LastFailureTime     time.Time `json:"lastFailureTime"`
	IsRunning           bool      `json:"isRunning"`
	EventBusType        string    `json:"eventBusType"`
	Source              string    `json:"source"`
}

// PublisherBacklogState 发送端积压状态
type PublisherBacklogState struct {
	HasBacklog        bool          `json:"hasBacklog"`
	QueueDepth        int64         `json:"queueDepth"`
	PublishRate       float64       `json:"publishRate"`
	AvgPublishLatency time.Duration `json:"avgPublishLatency"`
	BacklogRatio      float64       `json:"backlogRatio"` // 积压比例 (0.0-1.0)
	Timestamp         time.Time     `json:"timestamp"`
	Severity          string        `json:"severity"` // LOW, MEDIUM, HIGH, CRITICAL
}

// 回调函数类型
type BacklogStateCallback func(ctx context.Context, state BacklogState) error
type PublisherBacklogCallback func(ctx context.Context, state PublisherBacklogState) error
type HealthCheckCallback func(ctx context.Context, result HealthCheckResult) error
type ReconnectCallback func(ctx context.Context) error
type PublishCallback func(ctx context.Context, topic string, message []byte, err error) error
type SubscriptionCallback func(ctx context.Context, event SubscriptionEvent) error

// BacklogState 积压状态
type BacklogState struct {
	HasBacklog    bool          `json:"hasBacklog"`
	LagCount      int64         `json:"lagCount"`
	LagTime       time.Duration `json:"lagTime"`
	Timestamp     time.Time     `json:"timestamp"`
	Topic         string        `json:"topic"`
	ConsumerGroup string        `json:"consumerGroup"`
}

// HealthCheckResult 健康检查结果
type HealthCheckResult struct {
	Success             bool          `json:"success"`
	Timestamp           time.Time     `json:"timestamp"`
	Duration            time.Duration `json:"duration"`
	Error               error         `json:"error,omitempty"`
	ConsecutiveFailures int           `json:"consecutiveFailures"`
	EventBusType        string        `json:"eventBusType"`
	Source              string        `json:"source"`
}

// SubscriptionEvent 订阅事件
type SubscriptionEvent struct {
	Type      SubscriptionEventType `json:"type"`
	Topic     string                `json:"topic"`
	Timestamp time.Time             `json:"timestamp"`
	Message   string                `json:"message"`
	Error     error                 `json:"error,omitempty"`
}

// SubscriptionEventType 订阅事件类型
type SubscriptionEventType string

const (
	SubscriptionStarted      SubscriptionEventType = "started"
	SubscriptionStopped      SubscriptionEventType = "stopped"
	SubscriptionError        SubscriptionEventType = "error"
	SubscriptionEventStarted SubscriptionEventType = "started"
	SubscriptionEventStopped SubscriptionEventType = "stopped"
	SubscriptionEventError   SubscriptionEventType = "error"
	SubscriptionEventMessage SubscriptionEventType = "message"
)

// MessageRouter 消息路由器接口
type MessageRouter interface {
	// Route 根据消息内容决定路由策略
	Route(ctx context.Context, topic string, message []byte) (RouteDecision, error)
}

// RouteDecision 路由决策
type RouteDecision struct {
	ShouldProcess bool              `json:"shouldProcess"` // 是否应该处理这个消息
	AggregateID   string            `json:"aggregateId"`   // 聚合ID（用于聚合处理器）
	ProcessorKey  string            `json:"processorKey"`  // 处理器键（用于负载均衡）
	Priority      int               `json:"priority"`      // 优先级
	Metadata      map[string]string `json:"metadata"`      // 额外的元数据
}

// ErrorHandler 错误处理器接口
type ErrorHandler interface {
	// HandleError 处理错误
	HandleError(ctx context.Context, err error, message []byte, topic string) ErrorAction
}

// ErrorAction 错误处理动作
type ErrorAction struct {
	Action      ErrorActionType   `json:"action"`      // 动作类型
	RetryAfter  time.Duration     `json:"retryAfter"`  // 重试延迟
	DeadLetter  bool              `json:"deadLetter"`  // 是否发送到死信队列
	SkipMessage bool              `json:"skipMessage"` // 是否跳过消息
	Metadata    map[string]string `json:"metadata"`    // 额外的元数据
}

// ErrorActionType 错误动作类型
type ErrorActionType string

const (
	ErrorActionRetry      ErrorActionType = "retry"      // 重试
	ErrorActionDeadLetter ErrorActionType = "deadletter" // 死信队列
	ErrorActionSkip       ErrorActionType = "skip"       // 跳过
	ErrorActionFail       ErrorActionType = "fail"       // 失败
)

// MessageFormatter 消息格式化器接口
type MessageFormatter interface {
	// FormatMessage 格式化消息
	FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error)
	// ExtractAggregateID 提取聚合ID
	ExtractAggregateID(aggregateID interface{}) string
	// SetMetadata 设置元数据
	SetMetadata(msg *Message, metadata map[string]string) error
}

// Message 消息结构
type Message struct {
	UUID     string            `json:"uuid"`
	Payload  []byte            `json:"payload"`
	Metadata map[string]string `json:"metadata"`
}

// NewMessage 创建新消息
func NewMessage(uuid string, payload []byte) *Message {
	return &Message{
		UUID:     uuid,
		Payload:  payload,
		Metadata: make(map[string]string),
	}
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

// HealthStatus 综合健康状态
type HealthStatus struct {
	Overall        string                 `json:"overall"` // healthy, unhealthy, degraded
	Infrastructure InfrastructureHealth   `json:"infrastructure"`
	Business       interface{}            `json:"business,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
	CheckDuration  time.Duration          `json:"checkDuration"`
	Details        map[string]interface{} `json:"details,omitempty"`
}

// InfrastructureHealth 基础设施健康状态
type InfrastructureHealth struct {
	EventBus EventBusHealthMetrics `json:"eventBus"`
	Database *DatabaseHealth       `json:"database,omitempty"`
	Cache    *CacheHealth          `json:"cache,omitempty"`
}

// EventBusHealthMetrics EventBus 健康指标
type EventBusHealthMetrics struct {
	ConnectionStatus    string        `json:"connectionStatus"`    // connected, disconnected, reconnecting
	PublishLatency      time.Duration `json:"publishLatency"`      // 发布延迟
	SubscribeLatency    time.Duration `json:"subscribeLatency"`    // 订阅延迟
	LastSuccessTime     time.Time     `json:"lastSuccessTime"`     // 最后成功时间
	LastFailureTime     time.Time     `json:"lastFailureTime"`     // 最后失败时间
	ConsecutiveFailures int           `json:"consecutiveFailures"` // 连续失败次数
	ThroughputPerSecond int64         `json:"throughputPerSecond"` // 每秒吞吐量
	MessageBacklog      int64         `json:"messageBacklog"`      // 消息积压
	ReconnectCount      int           `json:"reconnectCount"`      // 重连次数
	BrokerCount         int           `json:"brokerCount"`         // 可用 Broker 数量
	TopicCount          int           `json:"topicCount"`          // 可用 Topic 数量
}

// DatabaseHealth 数据库健康状态
type DatabaseHealth struct {
	Status          string        `json:"status"`
	ResponseTime    time.Duration `json:"responseTime"`
	ConnectionCount int           `json:"connectionCount"`
}

// CacheHealth 缓存健康状态
type CacheHealth struct {
	Status       string        `json:"status"`
	ResponseTime time.Duration `json:"responseTime"`
	HitRate      float64       `json:"hitRate"`
}

// BusinessHealthChecker 业务健康检查接口
type BusinessHealthChecker interface {
	// 检查业务健康状态
	CheckBusinessHealth(ctx context.Context) error
	// 获取业务指标
	GetBusinessMetrics() interface{}
	// 获取业务配置
	GetBusinessConfig() interface{}
}

// EventBusHealthChecker EventBus 健康检查器接口（仅周期性健康检查）
type EventBusHealthChecker interface {
	// 注册业务健康检查
	RegisterBusinessHealthCheck(checker BusinessHealthChecker)
	// 获取健康状态（聚合状态）
	GetHealthStatus() HealthCheckStatus
}
