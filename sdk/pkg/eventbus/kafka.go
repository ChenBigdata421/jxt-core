package eventbus

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// 动态订阅更新类型
type subscriptionUpdate struct {
	action  string // "add" or "remove"
	topic   string
	handler MessageHandler
}

// ReconnectConfig 重连配置
type ReconnectConfig struct {
	MaxAttempts      int           // 最大重连次数
	InitialBackoff   time.Duration // 初始退避时间
	MaxBackoff       time.Duration // 最大退避时间
	BackoffFactor    float64       // 退避因子
	FailureThreshold int           // 触发重连的连续失败次数
}

// DefaultReconnectConfig 默认重连配置
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		MaxAttempts:      DefaultMaxReconnectAttempts,
		InitialBackoff:   DefaultReconnectInitialBackoff,
		MaxBackoff:       DefaultReconnectMaxBackoff,
		BackoffFactor:    DefaultReconnectBackoffFactor,
		FailureThreshold: DefaultReconnectFailureThreshold,
	}
}

// kafkaEventBus Kafka事件总线实现
// 企业级增强版本，集成积压检测、流量控制、聚合处理等特性
// 支持方案A（Envelope）消息包络
//
// 🔥 性能优化：高频路径免锁，低频路径保留锁
// - 高频路径（Publish、ConsumeClaim、配置读取）：使用 atomic.Value、sync.Map 实现无锁读取
// - 低频路径（Subscribe、Close、配置设置）：保留 sync.Mutex，保持代码清晰
type kafkaEventBus struct {
	config *KafkaConfig // 使用内部配置结构，实现解耦
	logger *zap.Logger

	// ✅ 低频路径：保留 mu（用于 Subscribe、Close 等低频操作）
	mu     sync.Mutex
	closed atomic.Bool // 🔥 P0修复：改为 atomic.Bool，热路径无锁读取

	// 🔥 高频路径：改为 atomic.Value（发布时无锁读取）
	asyncProducer atomic.Value // stores sarama.AsyncProducer
	consumer      atomic.Value // stores sarama.Consumer
	client        atomic.Value // stores sarama.Client
	admin         atomic.Value // stores sarama.ClusterAdmin

	// 企业级特性
	backlogDetector          *BacklogDetector          // 订阅端积压检测器
	publisherBacklogDetector *PublisherBacklogDetector // 发送端积压检测器
	rateLimiter              *RateLimiter              // 订阅端流量控制器
	publishRateLimiter       *RateLimiter              // 发布端流量控制器

	messageFormatter MessageFormatter
	publishCallback  PublishCallback
	errorHandler     ErrorHandler
	messageRouter    MessageRouter

	// 统计信息
	publishedMessages atomic.Int64
	consumedMessages  atomic.Int64
	errorCount        atomic.Int64

	// 健康检查控制
	healthCheckCancel context.CancelFunc
	healthCheckDone   chan struct{}

	// 健康检查订阅监控器
	healthCheckSubscriber *HealthCheckSubscriber
	// 健康检查发布器
	healthChecker *HealthChecker
	// 健康检查配置（从 Enterprise.HealthCheck 转换而来）
	healthCheckConfig config.HealthCheckConfig

	// 自动重连控制
	reconnectConfig   ReconnectConfig
	failureCount      atomic.Int32
	lastReconnectTime atomic.Value // time.Time
	reconnectCallback ReconnectCallback

	// 预订阅模式 - 统一消费者组管理
	unifiedConsumerGroup atomic.Value // stores sarama.ConsumerGroup

	// Round-Robin 轮询计数器（用于无聚合ID消息的负载均衡）
	roundRobinCounter atomic.Uint64

	// 🔥 P0修复：预订阅topic管理 - 使用 atomic.Value 存储不可变切片快照
	allPossibleTopicsMu sync.Mutex   // 保护写入
	allPossibleTopics   []string     // 主副本（仅在持有锁时修改）
	topicsSnapshot      atomic.Value // 只读快照（[]string），消费goroutine无锁读取

	// 🔥 P0修复：改为 sync.Map（消息路由时无锁查找）
	activeTopicHandlers sync.Map // key: string (topic), value: *handlerWrapper

	// 预订阅消费控制
	consumerCtx     context.Context
	consumerCancel  context.CancelFunc
	consumerDone    chan struct{}
	consumerMu      sync.Mutex
	consumerStarted bool

	// 预订阅配置
	preSubscriptionEnabled bool
	maxTopicsPerGroup      int

	// 优化4：预热状态监控
	warmupCompleted bool
	warmupStartTime time.Time
	warmupMu        sync.RWMutex

	// 异步发布结果通道（用于Outbox模式）
	publishResultChan chan *PublishResult

	// 多租户 ACK 通道支持
	tenantPublishResultChans map[string]chan *PublishResult // key: tenantID, value: ACK channel
	tenantChannelsMu         sync.RWMutex                   // 保护 tenantPublishResultChans 的读写锁

	// 🔥 高频路径：改为 sync.Map（消息处理时无锁查找）
	// 订阅管理（用于重连后恢复订阅）- 保持兼容性
	subscriptions sync.Map // key: string (topic), value: MessageHandler

	// 全局 Hollywood Actor Pool（所有 topic 共享）
	globalActorPool *HollywoodActorPool // ⭐ 全局 Hollywood Actor Pool（统一架构）

	// 🔥 高频路径：改为 sync.Map（发布时无锁读取配置）
	// 主题配置管理
	topicConfigs          sync.Map                  // key: string (topic), value: TopicOptions
	topicConfigStrategy   TopicConfigStrategy       // 配置策略
	topicConfigOnMismatch TopicConfigMismatchAction // 配置不一致时的行为
}

// 🔥 高频路径辅助方法：无锁读取 atomic.Value 存储的对象

// getAsyncProducer 无锁读取 AsyncProducer
func (k *kafkaEventBus) getAsyncProducer() (sarama.AsyncProducer, error) {
	producerAny := k.asyncProducer.Load()
	if producerAny == nil {
		return nil, fmt.Errorf("async producer not initialized")
	}
	producer, ok := producerAny.(sarama.AsyncProducer)
	if !ok {
		return nil, fmt.Errorf("invalid async producer type")
	}
	return producer, nil
}

// getConsumer 无锁读取 Consumer
func (k *kafkaEventBus) getConsumer() (sarama.Consumer, error) {
	consumerAny := k.consumer.Load()
	if consumerAny == nil {
		return nil, fmt.Errorf("consumer not initialized")
	}
	consumer, ok := consumerAny.(sarama.Consumer)
	if !ok {
		return nil, fmt.Errorf("invalid consumer type")
	}
	return consumer, nil
}

// getClient 无锁读取 Client
func (k *kafkaEventBus) getClient() (sarama.Client, error) {
	clientAny := k.client.Load()
	if clientAny == nil {
		return nil, fmt.Errorf("client not initialized")
	}
	client, ok := clientAny.(sarama.Client)
	if !ok {
		return nil, fmt.Errorf("invalid client type")
	}
	return client, nil
}

// getAdmin 无锁读取 ClusterAdmin
func (k *kafkaEventBus) getAdmin() (sarama.ClusterAdmin, error) {
	adminAny := k.admin.Load()
	if adminAny == nil {
		return nil, fmt.Errorf("admin not initialized")
	}
	admin, ok := adminAny.(sarama.ClusterAdmin)
	if !ok {
		return nil, fmt.Errorf("invalid admin type")
	}
	return admin, nil
}

// getUnifiedConsumerGroup 无锁读取 ConsumerGroup
func (k *kafkaEventBus) getUnifiedConsumerGroup() (sarama.ConsumerGroup, error) {
	groupAny := k.unifiedConsumerGroup.Load()
	if groupAny == nil {
		return nil, fmt.Errorf("consumer group not initialized")
	}
	group, ok := groupAny.(sarama.ConsumerGroup)
	if !ok {
		return nil, fmt.Errorf("invalid consumer group type")
	}
	return group, nil
}

// NewKafkaEventBus 创建企业级Kafka事件总线
// 使用内部配置结构，实现配置解耦
func NewKafkaEventBus(cfg *KafkaConfig) (EventBus, error) {
	if cfg == nil {
		return nil, fmt.Errorf("kafka config cannot be nil")
	}

	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers cannot be empty")
	}

	// 创建Sarama配置
	saramaConfig := sarama.NewConfig()

	// 优化1：AsyncProducer配置（Confluent官方推荐）
	saramaConfig.Producer.RequiredAcks = sarama.RequiredAcks(cfg.Producer.RequiredAcks)

	// 🔥 重构：移除 Producer 级别的压缩配置，改为 topic 级别配置
	// 压缩配置现在通过 TopicBuilder 在 topic 级别设置
	// 参考：createKafkaTopic() 函数中的 compression.type 配置
	saramaConfig.Producer.Compression = sarama.CompressionNone // 默认不压缩，由 topic 配置决定

	// 优化2：批处理配置（Confluent官方推荐值）
	if cfg.Producer.FlushFrequency > 0 {
		saramaConfig.Producer.Flush.Frequency = cfg.Producer.FlushFrequency
	} else {
		saramaConfig.Producer.Flush.Frequency = 10 * time.Millisecond // Confluent推荐：10ms
	}

	if cfg.Producer.FlushMessages > 0 {
		saramaConfig.Producer.Flush.Messages = cfg.Producer.FlushMessages
	} else {
		saramaConfig.Producer.Flush.Messages = 100 // Confluent推荐：100条消息
	}

	if cfg.Producer.FlushBytes > 0 {
		saramaConfig.Producer.Flush.Bytes = cfg.Producer.FlushBytes
	} else {
		saramaConfig.Producer.Flush.Bytes = 100000 // Confluent推荐：100KB
	}

	saramaConfig.Producer.Retry.Max = cfg.Producer.RetryMax
	saramaConfig.Producer.Timeout = cfg.Producer.Timeout
	saramaConfig.Producer.MaxMessageBytes = cfg.Producer.MaxMessageBytes
	saramaConfig.Producer.Idempotent = cfg.Producer.Idempotent

	// 配置 Hash Partitioner（保证相同 key 路由到同一 partition，确保顺序）
	saramaConfig.Producer.Partitioner = sarama.NewHashPartitioner

	// AsyncProducer需要配置成功和错误channel
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Return.Errors = true

	// 优化4：并发请求数（业界最佳实践）
	if cfg.Producer.MaxInFlight <= 0 {
		saramaConfig.Net.MaxOpenRequests = 100 // 业界推荐：100并发请求
	} else {
		saramaConfig.Net.MaxOpenRequests = cfg.Producer.MaxInFlight
	}

	// 优化5：Consumer配置（Confluent官方推荐）
	saramaConfig.Consumer.Group.Session.Timeout = cfg.Consumer.SessionTimeout
	saramaConfig.Consumer.Group.Heartbeat.Interval = cfg.Consumer.HeartbeatInterval
	if cfg.Consumer.MaxProcessingTime > 0 {
		saramaConfig.Consumer.MaxProcessingTime = cfg.Consumer.MaxProcessingTime
	} else {
		saramaConfig.Consumer.MaxProcessingTime = 1 * time.Second // 默认1秒
	}

	// 优化6：Fetch配置（Confluent官方推荐值）
	if cfg.Consumer.FetchMinBytes > 0 {
		saramaConfig.Consumer.Fetch.Min = int32(cfg.Consumer.FetchMinBytes)
	} else {
		saramaConfig.Consumer.Fetch.Min = 100 * 1024 // Confluent推荐：100KB
	}
	if cfg.Consumer.FetchMaxBytes > 0 {
		saramaConfig.Consumer.Fetch.Max = int32(cfg.Consumer.FetchMaxBytes)
	} else {
		saramaConfig.Consumer.Fetch.Max = 10 * 1024 * 1024 // Confluent推荐：10MB
	}
	if cfg.Consumer.FetchMaxWait > 0 {
		saramaConfig.Consumer.MaxWaitTime = cfg.Consumer.FetchMaxWait
	} else {
		saramaConfig.Consumer.MaxWaitTime = 500 * time.Millisecond // Confluent推荐：500ms
	}

	// 优化7：预取缓冲（业界最佳实践）
	saramaConfig.ChannelBufferSize = 1000 // 预取1000条消息

	saramaConfig.Consumer.Offsets.Initial = getOffsetInitial(cfg.Consumer.AutoOffsetReset)
	saramaConfig.Consumer.Group.Rebalance.Strategy = getRebalanceStrategy(cfg.Consumer.RebalanceStrategy)
	saramaConfig.Consumer.IsolationLevel = getIsolationLevel(cfg.Consumer.IsolationLevel)

	// 优化Consumer稳定性配置
	saramaConfig.Consumer.Return.Errors = true                            // 启用错误返回
	saramaConfig.Consumer.Offsets.Retry.Max = 3                           // 增加重试次数
	saramaConfig.Consumer.Group.Rebalance.Timeout = 60 * time.Second      // 增加重平衡超时
	saramaConfig.Consumer.Group.Rebalance.Retry.Max = 4                   // 增加重平衡重试次数
	saramaConfig.Consumer.Group.Rebalance.Retry.Backoff = 2 * time.Second // 重平衡重试间隔

	// 优化8：网络超时配置（Confluent官方推荐）
	// 根据部署环境调整超时时间
	if cfg.Net.DialTimeout > 0 {
		saramaConfig.Net.DialTimeout = cfg.Net.DialTimeout
	} else {
		saramaConfig.Net.DialTimeout = 10 * time.Second // 业界推荐：本地/云部署10秒
	}
	if cfg.Net.ReadTimeout > 0 {
		saramaConfig.Net.ReadTimeout = cfg.Net.ReadTimeout
	} else {
		saramaConfig.Net.ReadTimeout = 30 * time.Second // 保持30秒（适合云部署）
	}
	if cfg.Net.WriteTimeout > 0 {
		saramaConfig.Net.WriteTimeout = cfg.Net.WriteTimeout
	} else {
		saramaConfig.Net.WriteTimeout = 30 * time.Second // 保持30秒（适合云部署）
	}
	if cfg.Net.KeepAlive > 0 {
		saramaConfig.Net.KeepAlive = cfg.Net.KeepAlive
	} else {
		saramaConfig.Net.KeepAlive = 30 * time.Second // 保持30秒
	}

	// 配置客户端
	saramaConfig.ClientID = cfg.ClientID
	saramaConfig.Metadata.RefreshFrequency = cfg.MetadataRefreshFreq
	saramaConfig.Metadata.Retry.Max = cfg.MetadataRetryMax
	saramaConfig.Metadata.Retry.Backoff = cfg.MetadataRetryBackoff

	// 配置安全认证
	if cfg.Security.Enabled {
		// TODO: 实现安全配置
		logger.Warn("Security configuration not yet implemented")
	}

	// 创建Kafka客户端
	client, err := sarama.NewClient(cfg.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka client: %w", err)
	}

	// 优化1：创建AsyncProducer（Confluent官方推荐）
	asyncProducer, err := sarama.NewAsyncProducerFromClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create async producer: %w", err)
	}

	// 创建统一消费者组
	unifiedConsumerGroup, err := sarama.NewConsumerGroupFromClient(cfg.Consumer.GroupID, client)
	if err != nil {
		client.Close()
		asyncProducer.Close()
		return nil, fmt.Errorf("failed to create unified consumer group: %w", err)
	}

	// 创建管理客户端（用于主题配置管理）
	admin, err := sarama.NewClusterAdminFromClient(client)
	if err != nil {
		unifiedConsumerGroup.Close()
		asyncProducer.Close()
		client.Close()
		return nil, fmt.Errorf("failed to create kafka admin: %w", err)
	}

	// 转换健康检查配置（从 eventbus.HealthCheckConfig 转换为 config.HealthCheckConfig）
	healthCheckConfig := convertHealthCheckConfig(cfg.Enterprise.HealthCheck)

	// 创建事件总线实例
	bus := &kafkaEventBus{
		config: cfg,
		logger: zap.NewNop(), // 使用无操作logger

		// 健康检查配置
		healthCheckConfig: healthCheckConfig,

		// 🔥 P0修复：预订阅模式字段
		allPossibleTopics:      make([]string, 0),
		preSubscriptionEnabled: true,
		maxTopicsPerGroup:      100, // 默认最大100个topic
		consumerDone:           make(chan struct{}),

		// 🚀 异步发布结果通道（缓冲区大小：10000）
		publishResultChan: make(chan *PublishResult, 10000),
	}

	// 🔥 P0修复：初始化 atomic 字段
	bus.closed.Store(false)
	bus.topicsSnapshot.Store([]string{})

	// ✅ 使用 atomic.Value 存储（高频路径无锁读取）
	bus.client.Store(client)
	bus.asyncProducer.Store(asyncProducer)
	bus.unifiedConsumerGroup.Store(unifiedConsumerGroup)
	bus.admin.Store(admin)

	// 注意：subscriptions 和 topicConfigs 是 sync.Map，零值可用，不需要初始化

	// ⭐ 创建全局 Hollywood Actor Pool（所有 topic 共享）
	// 创建 Prometheus 监控收集器
	// 使用 ClientID 作为命名空间，确保每个实例的指标不冲突
	// 注意：Prometheus 指标名称只能包含 [a-zA-Z0-9_]，需要替换 - 为 _
	metricsNamespace := fmt.Sprintf("kafka_eventbus_%s", strings.ReplaceAll(cfg.ClientID, "-", "_"))
	actorPoolMetrics := NewPrometheusActorPoolMetricsCollector(metricsNamespace)

	bus.globalActorPool = NewHollywoodActorPool(HollywoodActorPoolConfig{
		PoolSize:    256,  // 固定 Actor 数量
		InboxSize:   1000, // Inbox 队列大小
		MaxRestarts: 3,    // Supervisor 最大重启次数
	}, actorPoolMetrics)

	bus.logger.Info("Kafka EventBus using Hollywood Actor Pool",
		zap.Int("poolSize", 256),
		zap.Int("inboxSize", 1000),
		zap.Int("maxRestarts", 3))

	// 初始化发布端流量控制器
	if cfg.Enterprise.Publisher.RateLimit.Enabled {
		bus.publishRateLimiter = NewRateLimiter(cfg.Enterprise.Publisher.RateLimit)
		logger.Info("Publisher rate limiter enabled",
			"ratePerSecond", cfg.Enterprise.Publisher.RateLimit.RatePerSecond,
			"burstSize", cfg.Enterprise.Publisher.RateLimit.BurstSize)
	}

	// 优化1：启动AsyncProducer的成功和错误处理goroutine
	go bus.handleAsyncProducerSuccess()
	go bus.handleAsyncProducerErrors()

	logger.Info("Kafka EventBus created successfully (AsyncProducer mode)",
		"brokers", cfg.Brokers,
		"clientId", cfg.ClientID,
		"healthCheckInterval", cfg.HealthCheckInterval,
		"compressionMode", "topic-level", // 🔥 重构：压缩配置改为 topic 级别
		"flushFrequency", saramaConfig.Producer.Flush.Frequency,
		"flushMessages", saramaConfig.Producer.Flush.Messages,
		"flushBytes", saramaConfig.Producer.Flush.Bytes)

	return bus, nil
}

// 优化1：处理AsyncProducer成功消息
func (k *kafkaEventBus) handleAsyncProducerSuccess() {
	// ✅ 无锁读取 asyncProducer
	producerAny := k.asyncProducer.Load()
	if producerAny == nil {
		k.logger.Warn("AsyncProducer not initialized, cannot handle success messages")
		return
	}
	producer := producerAny.(sarama.AsyncProducer)

	for success := range producer.Successes() {
		// 记录成功指标
		k.publishedMessages.Add(1)

		// ✅ 提取 EventID（从 Header 中）
		var eventID string
		var aggregateID string
		var eventType string
		var tenantID string
		for _, header := range success.Headers {
			switch string(header.Key) {
			case "X-Event-ID":
				eventID = string(header.Value)
			case "X-Aggregate-ID":
				aggregateID = string(header.Value)
			case "X-Event-Type":
				eventType = string(header.Value)
			case "X-Tenant-ID":
				tenantID = string(header.Value) // ← 租户ID（多租户支持，用于Outbox ACK路由）
			}
		}

		// ✅ 如果有 EventID，发送成功结果到 publishResultChan（用于 Outbox 模式）
		if eventID != "" {
			result := &PublishResult{
				EventID:     eventID,
				Topic:       success.Topic,
				Success:     true,
				Error:       nil,
				Timestamp:   time.Now(),
				AggregateID: aggregateID,
				EventType:   eventType,
				TenantID:    tenantID, // ← 租户ID（多租户支持，用于Outbox ACK路由）
			}

			// ✅ 发送到租户专属通道或全局通道
			k.sendResultToChannel(result)
		}

		// 如果配置了回调，执行回调
		if k.publishCallback != nil {
			// 提取消息内容
			var message []byte
			if success.Value != nil {
				message, _ = success.Value.Encode()
			}
			k.publishCallback(context.Background(), success.Topic, message, nil)
		}
	}
}

// 优化1：处理AsyncProducer错误
func (k *kafkaEventBus) handleAsyncProducerErrors() {
	// ✅ 无锁读取 asyncProducer
	producer, err := k.getAsyncProducer()
	if err != nil {
		k.logger.Warn("AsyncProducer not initialized, cannot handle error messages", zap.Error(err))
		return
	}

	for err := range producer.Errors() {
		// 记录错误
		k.errorCount.Add(1)
		k.logger.Error("Async producer error",
			zap.String("topic", err.Msg.Topic),
			zap.Error(err.Err))

		// ✅ 提取 EventID（从 Header 中）
		var eventID string
		var aggregateID string
		var eventType string
		var tenantID string
		for _, header := range err.Msg.Headers {
			switch string(header.Key) {
			case "X-Event-ID":
				eventID = string(header.Value)
			case "X-Aggregate-ID":
				aggregateID = string(header.Value)
			case "X-Event-Type":
				eventType = string(header.Value)
			case "X-Tenant-ID":
				tenantID = string(header.Value) // ← 租户ID（多租户支持，用于Outbox ACK路由）
			}
		}

		// ✅ 如果有 EventID，发送失败结果到 publishResultChan（用于 Outbox 模式）
		if eventID != "" {
			result := &PublishResult{
				EventID:     eventID,
				Topic:       err.Msg.Topic,
				Success:     false,
				Error:       err.Err,
				Timestamp:   time.Now(),
				AggregateID: aggregateID,
				EventType:   eventType,
				TenantID:    tenantID, // ← 租户ID（多租户支持，用于Outbox ACK路由）
			}

			// ✅ 发送到租户专属通道或全局通道
			k.sendResultToChannel(result)
		}

		// 提取消息内容
		var message []byte
		if err.Msg.Value != nil {
			message, _ = err.Msg.Value.Encode()
		}

		// 如果配置了回调，执行回调
		if k.publishCallback != nil {
			k.publishCallback(context.Background(), err.Msg.Topic, message, err.Err)
		}

		// 如果配置了错误处理器，执行错误处理
		if k.errorHandler != nil {
			k.errorHandler.HandleError(context.Background(), err.Err, message, err.Msg.Topic)
		}

		// 发布端错误处理：发送到死信队列
		if k.config.Enterprise.Publisher.ErrorHandling.DeadLetterTopic != "" {
			k.sendToPublisherDeadLetter(err.Msg.Topic, message, err.Err)
		}
	}
}

// sendToPublisherDeadLetter 将发布失败的消息发送到死信队列
func (k *kafkaEventBus) sendToPublisherDeadLetter(originalTopic string, message []byte, publishErr error) {
	deadLetterTopic := k.config.Enterprise.Publisher.ErrorHandling.DeadLetterTopic

	// 创建死信消息，包含原始主题和错误信息
	dlqMsg := &sarama.ProducerMessage{
		Topic: deadLetterTopic,
		Value: sarama.ByteEncoder(message),
		Headers: []sarama.RecordHeader{
			{Key: []byte("X-Original-Topic"), Value: []byte(originalTopic)},
			{Key: []byte("X-Error-Message"), Value: []byte(publishErr.Error())},
			{Key: []byte("X-Timestamp"), Value: []byte(time.Now().Format(time.RFC3339))},
		},
	}

	// 应用重试策略发送到死信队列
	retryPolicy := k.config.Enterprise.Publisher.RetryPolicy
	if retryPolicy.Enabled && retryPolicy.MaxRetries > 0 {
		k.sendWithRetry(dlqMsg, retryPolicy, originalTopic, deadLetterTopic)
	} else {
		// 不使用重试，直接发送
		k.sendMessageNonBlocking(dlqMsg, originalTopic, deadLetterTopic)
	}
}

// sendWithRetry 使用重试策略发送消息
func (k *kafkaEventBus) sendWithRetry(msg *sarama.ProducerMessage, retryPolicy RetryPolicyConfig, originalTopic, targetTopic string) {
	// ✅ 无锁读取 asyncProducer
	producer, err := k.getAsyncProducer()
	if err != nil {
		k.logger.Error("Failed to get async producer for retry",
			zap.String("originalTopic", originalTopic),
			zap.String("targetTopic", targetTopic),
			zap.Error(err))
		return
	}

	var lastErr error
	backoff := retryPolicy.InitialInterval

	for attempt := 0; attempt <= retryPolicy.MaxRetries; attempt++ {
		// 尝试发送
		select {
		case producer.Input() <- msg:
			k.logger.Info("Message sent successfully with retry",
				zap.String("originalTopic", originalTopic),
				zap.String("targetTopic", targetTopic),
				zap.Int("attempt", attempt+1))
			return
		case <-time.After(100 * time.Millisecond):
			lastErr = fmt.Errorf("async producer input queue full")
		}

		// 如果不是最后一次尝试，等待后重试
		if attempt < retryPolicy.MaxRetries {
			k.logger.Warn("Retry sending message to dead letter queue",
				zap.String("originalTopic", originalTopic),
				zap.String("targetTopic", targetTopic),
				zap.Int("attempt", attempt+1),
				zap.Int("maxRetries", retryPolicy.MaxRetries),
				zap.Duration("backoff", backoff))

			time.Sleep(backoff)

			// 计算下一次退避时间
			backoff = time.Duration(float64(backoff) * retryPolicy.Multiplier)
			if backoff > retryPolicy.MaxInterval {
				backoff = retryPolicy.MaxInterval
			}
		}
	}

	// 所有重试都失败
	k.logger.Error("Failed to send message after all retries",
		zap.String("originalTopic", originalTopic),
		zap.String("targetTopic", targetTopic),
		zap.Int("maxRetries", retryPolicy.MaxRetries),
		zap.Error(lastErr))
}

// sendMessageNonBlocking 非阻塞发送消息
func (k *kafkaEventBus) sendMessageNonBlocking(msg *sarama.ProducerMessage, originalTopic, targetTopic string) {
	// ✅ 无锁读取 asyncProducer
	producer, err := k.getAsyncProducer()
	if err != nil {
		k.logger.Error("Failed to get async producer for non-blocking send",
			zap.String("originalTopic", originalTopic),
			zap.String("targetTopic", targetTopic),
			zap.Error(err))
		return
	}

	select {
	case producer.Input() <- msg:
		k.logger.Info("Message sent to target topic",
			zap.String("originalTopic", originalTopic),
			zap.String("targetTopic", targetTopic))
	case <-time.After(100 * time.Millisecond):
		k.logger.Error("Failed to send message to target topic",
			zap.String("originalTopic", originalTopic),
			zap.String("targetTopic", targetTopic))
	}
}

// ========== 配置转换辅助函数 ==========

// convertHealthCheckConfig 将 eventbus.HealthCheckConfig 转换为 config.HealthCheckConfig
func convertHealthCheckConfig(ebConfig HealthCheckConfig) config.HealthCheckConfig {
	// 如果配置为空或未启用，返回默认配置
	if !ebConfig.Enabled {
		return GetDefaultHealthCheckConfig()
	}

	// 转换配置
	return config.HealthCheckConfig{
		Enabled: ebConfig.Enabled,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:            ebConfig.Topic,
			Interval:         ebConfig.Interval,
			Timeout:          ebConfig.Timeout,
			FailureThreshold: ebConfig.FailureThreshold,
			MessageTTL:       ebConfig.MessageTTL,
		},
		Subscriber: config.HealthCheckSubscriberConfig{
			Topic:             ebConfig.Topic,
			MonitorInterval:   30 * time.Second, // 使用默认值
			WarningThreshold:  3,                // 使用默认值
			ErrorThreshold:    5,                // 使用默认值
			CriticalThreshold: 10,               // 使用默认值
		},
	}
}

// getCompressionCodec 获取压缩编解码器
func getCompressionCodec(compression string) sarama.CompressionCodec {
	switch strings.ToLower(compression) {
	case "gzip":
		return sarama.CompressionGZIP
	case "snappy":
		return sarama.CompressionSnappy
	case "lz4":
		return sarama.CompressionLZ4
	case "zstd":
		return sarama.CompressionZSTD
	default:
		return sarama.CompressionNone
	}
}

// getOffsetInitial 获取初始偏移量策略
func getOffsetInitial(autoOffsetReset string) int64 {
	switch strings.ToLower(autoOffsetReset) {
	case "earliest":
		return sarama.OffsetOldest
	case "latest":
		return sarama.OffsetNewest
	default:
		return sarama.OffsetNewest
	}
}

// getRebalanceStrategy 获取再平衡策略
func getRebalanceStrategy(strategy string) sarama.BalanceStrategy {
	switch strings.ToLower(strategy) {
	case "range":
		return sarama.BalanceStrategyRange
	case "roundrobin":
		return sarama.BalanceStrategyRoundRobin
	case "sticky":
		return sarama.BalanceStrategySticky
	default:
		return sarama.BalanceStrategyRange
	}
}

// getIsolationLevel 获取隔离级别
func getIsolationLevel(level string) sarama.IsolationLevel {
	switch strings.ToLower(level) {
	case "read_uncommitted":
		return sarama.ReadUncommitted
	case "read_committed":
		return sarama.ReadCommitted
	default:
		return sarama.ReadCommitted
	}
}

// kafkaConsumerHandler Kafka消费者处理器
type kafkaConsumerHandler struct {
	eventBus *kafkaEventBus
	handler  MessageHandler
	topic    string
}

// Setup 消费者组设置
func (h *kafkaConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

// Cleanup 消费者组清理
func (h *kafkaConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 消费消息
func (h *kafkaConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// 处理消息
			if err := h.processMessage(session.Context(), message); err != nil {
				h.eventBus.logger.Error("Failed to process message",
					zap.String("topic", message.Topic),
					zap.Int32("partition", message.Partition),
					zap.Int64("offset", message.Offset),
					zap.Error(err))
				h.eventBus.errorCount.Add(1)
			} else {
				h.eventBus.consumedMessages.Add(1)
				session.MarkMessage(message, "")
			}

		case <-session.Context().Done():
			return nil
		}
	}
}

// processMessage 处理单个消息
func (h *kafkaConsumerHandler) processMessage(ctx context.Context, message *sarama.ConsumerMessage) error {
	// 流量控制
	if h.eventBus.rateLimiter != nil {
		if err := h.eventBus.rateLimiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit error: %w", err)
		}
	}

	// 智能路由决策：根据聚合ID提取结果决定处理模式
	// 构建headers映射
	headersMap := make(map[string]string, len(message.Headers))
	for _, header := range message.Headers {
		headersMap[string(header.Key)] = string(header.Value)
	}

	// 尝试提取聚合ID（优先级：Envelope > Header > Kafka Key）
	aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")
	
	// 如果无法提取聚合ID，使用 Round-Robin 生成路由键
	if aggregateID == "" {
		index := h.eventBus.roundRobinCounter.Add(1)
		aggregateID = fmt.Sprintf("rr-%d", index)
	}

	// ⭐ 统一处理：所有消息都通过 Hollywood Actor Pool 路由
	// - 有聚合ID：使用一致性哈希路由到固定 Actor（顺序处理）
	// - 无聚合ID：使用 Round-Robin 生成的路由键分散到不同 Actor（并发处理）
	pool := h.eventBus.globalActorPool
	if pool != nil {
		aggMsg := &AggregateMessage{
			Topic:       message.Topic,
			Partition:   message.Partition,
			Offset:      message.Offset,
			Key:         message.Key,
			Value:       message.Value,
			Headers:     make(map[string][]byte),
			Timestamp:   message.Timestamp,
			AggregateID: aggregateID,
			Context:     ctx,
			Done:        make(chan error, 1),
			Handler:     h.handler,
		}
		for _, header := range message.Headers {
			aggMsg.Headers[string(header.Key)] = header.Value
		}
		if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
			return err
		}
		select {
		case err := <-aggMsg.Done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	
	// 备用方案：如果 Actor Pool 不可用，直接调用 handler
	return h.handler(ctx, message.Value)
}

// preSubscriptionConsumerHandler 预订阅消费者处理器 - 预订阅模式
type preSubscriptionConsumerHandler struct {
	eventBus *kafkaEventBus
}

// Setup 消费者组设置
func (h *preSubscriptionConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	h.eventBus.logger.Info("Pre-subscription consumer group session started")
	return nil
}

// Cleanup 消费者组清理
func (h *preSubscriptionConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	h.eventBus.logger.Info("Pre-subscription consumer group session ended")
	return nil
}

// ConsumeClaim 预订阅消费消息 - 根据topic路由到激活的handler
func (h *preSubscriptionConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	ctx := session.Context()

	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// 🔥 P0修复：无锁读取 handler（使用 sync.Map）
			wrapperAny, exists := h.eventBus.activeTopicHandlers.Load(message.Topic)
			if !exists {
				// 未激活的 topic，跳过
				session.MarkMessage(message, "")
				continue
			}
			wrapper := wrapperAny.(*handlerWrapper)

			if true {
				// 优化：增强消息处理错误处理和监控
				h.eventBus.logger.Debug("Processing message",
					zap.String("topic", message.Topic),
					zap.Int64("offset", message.Offset),
					zap.Int32("partition", message.Partition),
					zap.Bool("isEnvelope", wrapper.isEnvelope))

				// 使用 Hollywood Actor Pool 处理消息（有聚合ID用哈希路由，无聚合ID用轮询路由）
				if err := h.processMessageWithKeyedPool(ctx, message, wrapper, session); err != nil {
					h.eventBus.logger.Error("Failed to process message",
						zap.String("topic", message.Topic),
						zap.Int64("offset", message.Offset),
						zap.Error(err))
					// 处理失败，仍然标记消息（避免阻塞）
					session.MarkMessage(message, "")
				}
			} else {
				// 未激活的topic，直接标记为已处理（预订阅模式的优势）
				h.eventBus.logger.Debug("Topic not activated, skipping message",
					zap.String("topic", message.Topic),
					zap.Int64("offset", message.Offset))
				session.MarkMessage(message, "")
			}

		case <-ctx.Done():
			return nil
		}
	}
}

// processMessageWithKeyedPool 使用 Hollywood Actor Pool 处理消息（统一架构）
func (h *preSubscriptionConsumerHandler) processMessageWithKeyedPool(ctx context.Context, message *sarama.ConsumerMessage, wrapper *handlerWrapper, session sarama.ConsumerGroupSession) error {
	// 转换 Headers 为 map
	headersMap := make(map[string]string, len(message.Headers))
	for _, header := range message.Headers {
		headersMap[string(header.Key)] = string(header.Value)
	}

	// 尝试提取聚合ID（优先级：Envelope > Header > Kafka Key）
	aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")

	// ⭐ 核心变更：统一使用 Hollywood Actor Pool
	// 路由策略：
	// - 有聚合ID：使用 aggregateID 作为路由键（保持有序）
	// - 无聚合ID：使用 Round-Robin 轮询（保持并发）
	routingKey := aggregateID
	if routingKey == "" {
		// 使用轮询计数器生成路由键
		index := h.eventBus.roundRobinCounter.Add(1)
		routingKey = fmt.Sprintf("rr-%d", index)
	}

	pool := h.eventBus.globalActorPool
	if pool == nil {
		return fmt.Errorf("hollywood actor pool not initialized")
	}

	aggMsg := &AggregateMessage{
		Topic:       message.Topic,
		Partition:   message.Partition,
		Offset:      message.Offset,
		Key:         message.Key,
		Value:       message.Value,
		Headers:     make(map[string][]byte),
		Timestamp:   message.Timestamp,
		AggregateID: routingKey, // ⭐ 使用 routingKey（可能是 aggregateID 或 Round-Robin 索引）
		Context:     ctx,
		Done:        make(chan error, 1),
		Handler:     wrapper.handler,
		IsEnvelope:  wrapper.isEnvelope, // ⭐ 设置 Envelope 标记
	}
	for _, header := range message.Headers {
		aggMsg.Headers[string(header.Key)] = header.Value
	}

	// 提交到 Actor Pool
	if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
		return err
	}

	// 等待处理完成
	select {
	case err := <-aggMsg.Done:
		if err != nil {
			if wrapper.isEnvelope {
				// ⭐ Envelope 消息：不 MarkMessage，让 Kafka 重新投递（at-least-once 语义）
				h.eventBus.logger.Warn("Envelope message processing failed, will be redelivered",
					zap.String("topic", message.Topic),
					zap.Int64("offset", message.Offset),
					zap.Error(err))
				return err
			} else {
				// ⭐ 普通消息：MarkMessage，避免重复投递（at-most-once 语义）
				h.eventBus.logger.Warn("Regular message processing failed, marking as processed",
					zap.String("topic", message.Topic),
					zap.Int64("offset", message.Offset),
					zap.Error(err))
				session.MarkMessage(message, "")
				return err
			}
		}
		// 成功：MarkMessage
		session.MarkMessage(message, "")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// initEnterpriseFeatures 初始化企业级特性
func (k *kafkaEventBus) initEnterpriseFeatures() error {
	// 使用程序员配置层的企业级特性配置
	enterpriseConfig := k.config.Enterprise

	// 初始化订阅端积压检测器
	if enterpriseConfig.Subscriber.BacklogDetection.Enabled {
		// ✅ 无锁读取 client 和 admin
		client, err := k.getClient()
		if err != nil {
			return fmt.Errorf("failed to get client for backlog detector: %w", err)
		}
		admin, err := k.getAdmin()
		if err != nil {
			return fmt.Errorf("failed to get admin for backlog detector: %w", err)
		}

		backlogConfig := BacklogDetectionConfig{
			MaxLagThreshold:  enterpriseConfig.Subscriber.BacklogDetection.MaxLagThreshold,
			MaxTimeThreshold: enterpriseConfig.Subscriber.BacklogDetection.MaxTimeThreshold,
			CheckInterval:    enterpriseConfig.Subscriber.BacklogDetection.CheckInterval,
		}
		k.backlogDetector = NewBacklogDetector(client, admin, k.config.Consumer.GroupID, backlogConfig)
		k.logger.Info("Subscriber backlog detector initialized",
			zap.Int64("maxLagThreshold", backlogConfig.MaxLagThreshold),
			zap.Duration("maxTimeThreshold", backlogConfig.MaxTimeThreshold),
			zap.Duration("checkInterval", backlogConfig.CheckInterval))
	}

	// 初始化发送端积压检测器
	if enterpriseConfig.Publisher.BacklogDetection.Enabled {
		// ✅ 无锁读取 client 和 admin
		client, err := k.getClient()
		if err != nil {
			return fmt.Errorf("failed to get client for publisher backlog detector: %w", err)
		}
		admin, err := k.getAdmin()
		if err != nil {
			return fmt.Errorf("failed to get admin for publisher backlog detector: %w", err)
		}

		k.publisherBacklogDetector = NewPublisherBacklogDetector(client, admin, enterpriseConfig.Publisher.BacklogDetection)
		k.logger.Info("Publisher backlog detector initialized",
			zap.Int64("maxQueueDepth", enterpriseConfig.Publisher.BacklogDetection.MaxQueueDepth),
			zap.Duration("maxPublishLatency", enterpriseConfig.Publisher.BacklogDetection.MaxPublishLatency),
			zap.Float64("rateThreshold", enterpriseConfig.Publisher.BacklogDetection.RateThreshold),
			zap.Duration("checkInterval", enterpriseConfig.Publisher.BacklogDetection.CheckInterval))
	}

	k.logger.Info("Enterprise features initialized successfully")
	return nil
}

// Kafka EventBus 实现

// Publish 发布消息
// 🔥 高频路径优化：无锁检查关闭状态，无锁读取 producer
func (k *kafkaEventBus) Publish(ctx context.Context, topic string, message []byte) error {
	// 🔥 P0修复：无锁检查关闭状态
	if k.closed.Load() {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 发布端流量控制
	if k.publishRateLimiter != nil {
		if err := k.publishRateLimiter.Wait(ctx); err != nil {
			k.errorCount.Add(1)
			return fmt.Errorf("publisher rate limit error: %w", err)
		}
	}

	// 订阅端流量控制（保留兼容性）
	if k.rateLimiter != nil {
		if err := k.rateLimiter.Wait(ctx); err != nil {
			k.errorCount.Add(1)
			return fmt.Errorf("subscriber rate limit error: %w", err)
		}
	}

	// ✅ 无锁读取 asyncProducer
	producer, err := k.getAsyncProducer()
	if err != nil {
		k.errorCount.Add(1)
		return fmt.Errorf("failed to get async producer: %w", err)
	}

	// 创建Kafka消息
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(message),
	}

	// 优化1：使用AsyncProducer异步发送（非阻塞）
	select {
	case producer.Input() <- msg:
		// 消息已提交到发送队列，成功/失败由后台goroutine处理
		k.logger.Debug("Message queued for async publishing",
			zap.String("topic", topic))
		return nil
	case <-time.After(100 * time.Millisecond):
		// 发送队列满，应用背压
		k.errorCount.Add(1)
		k.logger.Warn("Async producer input queue full, applying backpressure",
			zap.String("topic", topic))
		// 阻塞等待，确保消息不丢失
		producer.Input() <- msg
		return nil
	}
}

// startPreSubscriptionConsumer 启动预订阅消费循环
func (k *kafkaEventBus) startPreSubscriptionConsumer(ctx context.Context) error {
	k.consumerMu.Lock()
	defer k.consumerMu.Unlock()

	// 如果已经启动，直接返回
	if k.consumerStarted {
		return nil
	}

	// 创建消费者上下文
	k.consumerCtx, k.consumerCancel = context.WithCancel(ctx)
	k.consumerDone = make(chan struct{})

	// 创建预订阅消费者处理器
	handler := &preSubscriptionConsumerHandler{
		eventBus: k,
	}

	// 业界最佳实践：在循环中调用 Consume，支持重平衡后重新订阅
	// 参考：https://pkg.go.dev/github.com/IBM/sarama#ConsumerGroup
	// "Consume should be called inside an infinite loop, when a server-side rebalance happens,
	// the consumer session will need to be recreated to get the new claims"
	go func() {
		defer close(k.consumerDone)

		// 🔥 P0修复：无锁读取 topicsSnapshot
		topics := k.topicsSnapshot.Load().([]string)
		k.logger.Info("Pre-subscription consumer started",
			zap.Int("topicCount", len(topics)),
			zap.Strings("topics", topics))

		for {
			// 检查 context 是否已取消
			if k.consumerCtx.Err() != nil {
				k.logger.Info("Pre-subscription consumer context cancelled")
				return
			}

			// 🔥 P0修复：无锁读取 topicsSnapshot
			topics := k.topicsSnapshot.Load().([]string)

			// 检查是否有 topic 需要订阅
			if len(topics) == 0 {
				k.logger.Warn("No topics configured for pre-subscription, consumer will wait")
				// 等待 context 取消
				<-k.consumerCtx.Done()
				return
			}

			// 关键：调用 Consume 订阅所有预配置的 topic
			// 这会阻塞直到发生重平衡或 context 取消
			k.logger.Info("Pre-subscription consumer consuming topics",
				zap.Strings("topics", topics),
				zap.Int("topicCount", len(topics)))

			// ✅ 无锁读取 unifiedConsumerGroup
			consumerGroup, err := k.getUnifiedConsumerGroup()
			if err != nil {
				k.logger.Error("Failed to get unified consumer group",
					zap.Error(err))
				// 短暂等待后重试
				select {
				case <-k.consumerCtx.Done():
					return
				case <-time.After(2 * time.Second):
					continue
				}
			}

			// ✅ 安全：topics 是不可变副本，不会被修改
			err = consumerGroup.Consume(k.consumerCtx, topics, handler)
			if err != nil {
				if k.consumerCtx.Err() != nil {
					k.logger.Info("Pre-subscription consumer context cancelled during Consume")
					return
				}

				k.logger.Error("Pre-subscription consumer error, will retry",
					zap.Error(err))

				// 短暂等待后重试
				select {
				case <-k.consumerCtx.Done():
					return
				case <-time.After(2 * time.Second):
					continue
				}
			}

			// Consume 正常返回（通常是因为重平衡），继续循环
			k.logger.Info("Pre-subscription consumer session ended, restarting...")
		}
	}()

	k.consumerStarted = true

	// 🔥 P0修复：无锁读取 topicsSnapshot
	topics := k.topicsSnapshot.Load().([]string)
	k.logger.Info("Pre-subscription consumer system started",
		zap.Int("topicCount", len(topics)))

	// 优化1&4：添加3秒Consumer预热机制 + 状态监控
	k.warmupMu.Lock()
	k.warmupStartTime = time.Now()
	k.warmupCompleted = false
	k.warmupMu.Unlock()

	k.logger.Info("Consumer warming up for 3 seconds...",
		zap.Time("warmupStartTime", k.warmupStartTime))
	time.Sleep(3 * time.Second)

	k.warmupMu.Lock()
	k.warmupCompleted = true
	warmupDuration := time.Since(k.warmupStartTime)
	k.warmupMu.Unlock()

	k.logger.Info("Consumer warmup completed, ready for optimal performance",
		zap.Duration("warmupDuration", warmupDuration),
		zap.Bool("warmupCompleted", true))

	return nil
}

// 优化4：IsWarmupCompleted 检查预热状态
func (k *kafkaEventBus) IsWarmupCompleted() bool {
	k.warmupMu.RLock()
	defer k.warmupMu.RUnlock()
	return k.warmupCompleted
}

// 优化4：GetWarmupInfo 获取预热信息
func (k *kafkaEventBus) GetWarmupInfo() (completed bool, duration time.Duration) {
	k.warmupMu.RLock()
	defer k.warmupMu.RUnlock()

	completed = k.warmupCompleted
	if !k.warmupStartTime.IsZero() {
		if completed {
			duration = 3 * time.Second // 预热完成，返回固定时长
		} else {
			duration = time.Since(k.warmupStartTime) // 预热中，返回已用时长
		}
	}
	return
}

// activateTopicHandler 激活topic处理器（预订阅模式）
// 🔥 P0修复：使用 sync.Map 存储，无锁操作
func (k *kafkaEventBus) activateTopicHandler(topic string, handler MessageHandler, isEnvelope bool) {
	wrapper := &handlerWrapper{
		handler:    handler,
		isEnvelope: isEnvelope,
	}
	k.activeTopicHandlers.Store(topic, wrapper)

	// 计算当前激活的 topic 数量
	count := 0
	k.activeTopicHandlers.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	k.logger.Info("Topic handler activated",
		zap.String("topic", topic),
		zap.Bool("isEnvelope", isEnvelope),
		zap.Int("totalActiveTopics", count))
}

// deactivateTopicHandler 停用topic处理器（预订阅模式）
// 🔥 P0修复：使用 sync.Map 存储，无锁操作
func (k *kafkaEventBus) deactivateTopicHandler(topic string) {
	k.activeTopicHandlers.Delete(topic)

	// 计算当前激活的 topic 数量
	count := 0
	k.activeTopicHandlers.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	k.logger.Info("Topic handler deactivated",
		zap.String("topic", topic),
		zap.Int("totalActiveTopics", count))
}

// SetPreSubscriptionTopics 设置预订阅topic列表（企业级生产环境API）
//
// 业界最佳实践：在调用 Subscribe 之前，先设置所有需要订阅的 topic
// 这样可以避免 Kafka Consumer Group 的频繁重平衡，提高性能和稳定性
//
// 使用场景：
//  1. 创建 EventBus 实例
//  2. 调用 SetPreSubscriptionTopics 设置所有 topic
//  3. 调用 Subscribe 订阅各个 topic（消费者会一次性订阅所有 topic）
//
// 示例：
//
//	eventBus, _ := NewKafkaEventBus(config)
//	eventBus.SetPreSubscriptionTopics([]string{"topic1", "topic2", "topic3"})
//	eventBus.Subscribe(ctx, "topic1", handler1)
//	eventBus.Subscribe(ctx, "topic2", handler2)
//	eventBus.Subscribe(ctx, "topic3", handler3)
//
// 参考：
//   - Confluent 官方文档：预订阅模式避免重平衡
//   - LinkedIn 实践：一次性订阅所有 topic
//   - Uber 实践：预配置 topic 列表
func (k *kafkaEventBus) SetPreSubscriptionTopics(topics []string) {
	k.allPossibleTopicsMu.Lock()
	defer k.allPossibleTopicsMu.Unlock()

	// 🔥 P0修复：创建不可变副本并更新快照
	newTopics := make([]string, len(topics))
	copy(newTopics, topics)

	k.allPossibleTopics = newTopics
	k.topicsSnapshot.Store(newTopics)

	k.logger.Info("Pre-subscription topics configured",
		zap.Strings("topics", newTopics),
		zap.Int("topicCount", len(newTopics)))
}

// addTopicToPreSubscription 添加topic到预订阅列表（内部方法）
// 🔥 P0修复：使用 atomic.Value 存储不可变切片快照，消除数据竞态
func (k *kafkaEventBus) addTopicToPreSubscription(topic string) {
	k.allPossibleTopicsMu.Lock()
	defer k.allPossibleTopicsMu.Unlock()

	// 检查是否已存在
	for _, existingTopic := range k.allPossibleTopics {
		if existingTopic == topic {
			return // 已存在，无需添加
		}
	}

	// 创建新的不可变副本
	newTopics := make([]string, len(k.allPossibleTopics)+1)
	copy(newTopics, k.allPossibleTopics)
	newTopics[len(k.allPossibleTopics)] = topic

	// 更新主副本和快照
	k.allPossibleTopics = newTopics
	k.topicsSnapshot.Store(newTopics)

	k.logger.Info("Topic added to pre-subscription list",
		zap.String("topic", topic),
		zap.Int("totalTopics", len(newTopics)))
}

// stopPreSubscriptionConsumer 停止预订阅消费循环
func (k *kafkaEventBus) stopPreSubscriptionConsumer() {
	k.consumerMu.Lock()
	defer k.consumerMu.Unlock()

	if k.consumerCancel != nil {
		k.consumerCancel()
		<-k.consumerDone
		k.consumerCancel = nil
		k.consumerStarted = false
		k.logger.Info("Pre-subscription consumer stopped")
	}
}

// containsString 检查slice中是否包含指定元素
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Subscribe 订阅原始消息（使用 Hollywood Actor Pool + Round-Robin 轮询）
//
// 特点：
// - 消息格式：原始字节数据
// - 处理模式：Round-Robin 轮询路由到 Hollywood Actor Pool，无顺序保证
// - 性能：极致性能，微秒级延迟
// - 聚合ID：通常无法从原始消息中提取聚合ID
// - 路由策略：Round-Robin 轮询（与原全局 Worker Pool 行为一致）
//
// 适用场景：
// - 简单消息传递（通知、提醒）
// - 缓存失效消息
// - 系统监控指标
// - 不需要顺序保证的业务场景
//
// 示例：
//
//	bus.Subscribe(ctx, "notifications", func(ctx context.Context, data []byte) error {
//	    var notification Notification
//	    json.Unmarshal(data, &notification)
//	    return processNotification(notification) // 直接并发处理
//	})
//
// Subscribe 订阅主题
// ✅ 低频路径：保留锁，保持代码清晰
func (k *kafkaEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	k.mu.Lock()

	// 🔥 P0修复：使用 atomic.Bool 读取关闭状态
	if k.closed.Load() {
		k.mu.Unlock()
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 预订阅模式 - 激活topic处理器

	// ✅ 使用 sync.Map 存储订阅信息（用于重连后恢复）
	// 使用 LoadOrStore 检查重复订阅
	if _, loaded := k.subscriptions.LoadOrStore(topic, handler); loaded {
		k.mu.Unlock()
		return fmt.Errorf("already subscribed to topic: %s", topic)
	}

	// 全局 Hollywood Actor Pool 已在初始化时创建，所有 topic 共享同一个 Actor Pool

	// 关键修复：添加 topic 到预订阅列表
	k.addTopicToPreSubscription(topic)

	// 激活topic处理器（立即生效）
	// ⭐ 普通 Subscribe：isEnvelope = false（at-most-once 语义）
	k.activateTopicHandler(topic, handler, false)

	// 🔧 修复死锁：在释放锁之前记录日志，然后释放锁再启动consumer
	k.logger.Info("Subscribed to topic via pre-subscription consumer",
		zap.String("topic", topic),
		zap.String("groupID", k.config.Consumer.GroupID))

	// 释放锁，避免在启动consumer时持有锁导致死锁
	k.mu.Unlock()

	// 启动预订阅消费者（如果还未启动）
	// 注意：这里不持有k.mu锁，避免在sleep期间阻塞其他操作（如Publish）
	if err := k.startPreSubscriptionConsumer(ctx); err != nil {
		return fmt.Errorf("failed to start pre-subscription consumer: %w", err)
	}

	return nil
}

// healthCheck 内部健康检查（不对外暴露）
func (k *kafkaEventBus) healthCheck(ctx context.Context) error {
	// 🔥 P0修复：无锁检查关闭状态
	if k.closed.Load() {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// ✅ 无锁读取 client
	client, err := k.getClient()
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	// 检查客户端连接
	if !client.Closed() {
		// 尝试获取broker信息
		brokers := client.Brokers()
		if len(brokers) == 0 {
			return fmt.Errorf("no available brokers")
		}

		// 检查至少一个broker是否可达
		for _, broker := range brokers {
			if connected, _ := broker.Connected(); connected {
				k.logger.Debug("Kafka eventbus health check passed",
					zap.Int("brokers", len(brokers)),
					zap.String("connectedBroker", broker.Addr()))
				return nil
			}
		}

		return fmt.Errorf("no connected brokers available")
	}

	return fmt.Errorf("kafka client is closed")
}

// CheckConnection 检查 Kafka 连接状态
func (k *kafkaEventBus) CheckConnection(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	// 🔥 P0修复：使用 atomic.Bool 读取关闭状态
	if k.closed.Load() {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// ✅ 无锁读取 client
	client, err := k.getClient()
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	if client.Closed() {
		return fmt.Errorf("kafka client is closed")
	}

	// 检查 broker 连接
	brokers := client.Brokers()
	if len(brokers) == 0 {
		return fmt.Errorf("no available brokers")
	}

	connectedBrokers := 0
	for _, broker := range brokers {
		if connected, _ := broker.Connected(); connected {
			connectedBrokers++
		}
	}

	if connectedBrokers == 0 {
		return fmt.Errorf("no connected brokers available")
	}

	k.logger.Debug("Kafka connection check passed",
		zap.Int("totalBrokers", len(brokers)),
		zap.Int("connectedBrokers", connectedBrokers))

	return nil
}

// CheckMessageTransport 检查端到端消息传输
func (k *kafkaEventBus) CheckMessageTransport(ctx context.Context) error {
	testTopic := "health_check_topic"
	testMessage := fmt.Sprintf("health-check-%d", time.Now().UnixNano())

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// ✅ 检查是否支持端到端测试
	consumer, err := k.getConsumer()
	if err == nil && consumer != nil {
		return k.performKafkaEndToEndTest(ctx, testTopic, testMessage)
	}

	// 如果没有消费者组，只测试发布能力
	start := time.Now()
	err = k.Publish(ctx, testTopic, []byte(testMessage))
	publishLatency := time.Since(start)

	if err != nil {
		k.logger.Error("Kafka health check message transport failed",
			zap.Error(err),
			zap.Duration("publishLatency", publishLatency))
		return fmt.Errorf("failed to publish health check message: %w", err)
	}

	k.logger.Debug("Kafka health check message transport successful",
		zap.Duration("publishLatency", publishLatency))

	return nil
}

// performKafkaEndToEndTest 执行 Kafka 端到端测试
func (k *kafkaEventBus) performKafkaEndToEndTest(ctx context.Context, testTopic, testMessage string) error {
	// 创建接收通道
	receiveChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	// ✅ 无锁读取 consumer
	consumer, err := k.getConsumer()
	if err != nil {
		return fmt.Errorf("failed to get consumer: %w", err)
	}

	// 创建分区消费者来接收健康检查消息
	partitionConsumer, err := consumer.ConsumePartition(testTopic, 0, sarama.OffsetNewest)
	if err != nil {
		k.logger.Warn("Failed to create partition consumer for health check, falling back to publish-only test",
			zap.Error(err))
		// 回退到只测试发布
		start := time.Now()
		if err := k.Publish(ctx, testTopic, []byte(testMessage)); err != nil {
			return fmt.Errorf("failed to publish health check message: %w", err)
		}
		publishLatency := time.Since(start)
		k.logger.Debug("Kafka health check message published (no consumer test)",
			zap.Duration("publishLatency", publishLatency))
		return nil
	}
	defer partitionConsumer.Close()

	// 启动消费者（在后台）
	go func() {
		defer close(receiveChan)
		defer close(errorChan)

		for {
			select {
			case message := <-partitionConsumer.Messages():
				if message != nil {
					receivedMsg := string(message.Value)
					if receivedMsg == testMessage {
						select {
						case receiveChan <- receivedMsg:
							return
						case <-ctx.Done():
							return
						}
					}
				}
			case err := <-partitionConsumer.Errors():
				if err != nil {
					select {
					case errorChan <- err:
						return
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 等待一小段时间确保消费者准备就绪
	time.Sleep(200 * time.Millisecond)

	// 发布健康检查消息
	start := time.Now()
	if err := k.Publish(ctx, testTopic, []byte(testMessage)); err != nil {
		return fmt.Errorf("failed to publish health check message: %w", err)
	}
	publishLatency := time.Since(start)

	// 等待接收消息或超时
	select {
	case receivedMsg := <-receiveChan:
		totalLatency := time.Since(start)
		k.logger.Debug("Kafka end-to-end health check successful",
			zap.Duration("publishLatency", publishLatency),
			zap.Duration("totalLatency", totalLatency),
			zap.String("message", receivedMsg))
		return nil

	case err := <-errorChan:
		return fmt.Errorf("kafka health check consumer error: %w", err)

	case <-ctx.Done():
		return fmt.Errorf("kafka health check timeout: message not received within timeout period")

	case <-time.After(8 * time.Second):
		return fmt.Errorf("kafka health check timeout: message not received within 8 seconds")
	}
}

// GetEventBusMetrics 获取 Kafka EventBus 性能指标
func (k *kafkaEventBus) GetEventBusMetrics() EventBusHealthMetrics {
	k.mu.Lock()
	defer k.mu.Unlock()

	connectionStatus := "disconnected"
	brokerCount := 0
	topicCount := 0

	// ✅ 无锁读取 client
	client, err := k.getClient()
	// 🔥 P0修复：使用 atomic.Bool 读取关闭状态
	if err == nil && !k.closed.Load() && !client.Closed() {
		brokers := client.Brokers()
		brokerCount = len(brokers)

		connectedBrokers := 0
		for _, broker := range brokers {
			if connected, _ := broker.Connected(); connected {
				connectedBrokers++
			}
		}

		if connectedBrokers > 0 {
			connectionStatus = "connected"
		}

		// 获取 topic 数量
		if topics, err := client.Topics(); err == nil {
			topicCount = len(topics)
		}
	}

	return EventBusHealthMetrics{
		ConnectionStatus:    connectionStatus,
		PublishLatency:      0,                          // TODO: 实际测量并缓存
		SubscribeLatency:    0,                          // TODO: 实际测量并缓存
		LastSuccessTime:     time.Now(),                 // TODO: 实际跟踪
		LastFailureTime:     time.Time{},                // TODO: 实际跟踪
		ConsecutiveFailures: 0,                          // TODO: 实际统计
		ThroughputPerSecond: k.publishedMessages.Load(), // 简化实现
		MessageBacklog:      0,                          // TODO: 实际计算
		ReconnectCount:      0,                          // TODO: 实际统计
		BrokerCount:         brokerCount,
		TopicCount:          topicCount,
	}
}

// Close 关闭连接
func (k *kafkaEventBus) Close() error {
	k.mu.Lock()

	// 停止预订阅消费者组
	k.stopPreSubscriptionConsumer()

	// ⭐ 关闭全局 Hollywood Actor Pool
	if k.globalActorPool != nil {
		k.globalActorPool.Stop()
	}

	defer k.mu.Unlock()

	// 🔥 P0修复：使用 atomic.Bool 读取关闭状态
	if k.closed.Load() {
		return nil
	}

	var errors []error

	// 关闭顺序很重要：
	// 1. 先关闭从 client 创建的组件（unifiedConsumerGroup, admin, consumer, asyncProducer）
	// 2. 最后关闭 client（因为其他组件依赖它）
	// 注意：unifiedConsumerGroup.Close() 不会关闭底层的 client

	// 关闭统一消费者组
	if consumerGroup, err := k.getUnifiedConsumerGroup(); err == nil {
		if err := consumerGroup.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close unified consumer group: %w", err))
		}
	}

	// 关闭消费者
	if consumer, err := k.getConsumer(); err == nil {
		if err := consumer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka consumer: %w", err))
		}
	}

	// 优化1：关闭AsyncProducer
	if producer, err := k.getAsyncProducer(); err == nil {
		if err := producer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka async producer: %w", err))
		}
	}

	// 关闭管理客户端（在关闭 client 之前）
	if admin, err := k.getAdmin(); err == nil {
		if err := admin.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka admin: %w", err))
		}
	}

	// 最后关闭客户端（所有其他组件都依赖它）
	// 注意：某些版本的 sarama 可能在 ConsumerGroup.Close() 时已经关闭了 client
	// 因此我们需要检查 client 是否已经关闭
	if client, err := k.getClient(); err == nil {
		if err := client.Close(); err != nil {
			// 忽略 "client already closed" 错误
			if err.Error() != "kafka: tried to use a client that was closed" {
				errors = append(errors, fmt.Errorf("failed to close kafka client: %w", err))
			}
		}
	}

	// 🔥 P0修复：使用 atomic.Bool 设置关闭状态
	k.closed.Store(true)
	k.logger.Info("Kafka eventbus closed successfully")

	if len(errors) > 0 {
		return fmt.Errorf("errors during kafka eventbus close: %v", errors)
	}

	return nil
}

// RegisterReconnectCallback 注册重连回调
func (k *kafkaEventBus) RegisterReconnectCallback(callback ReconnectCallback) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.reconnectCallback = callback
	k.logger.Info("Reconnect callback registered for kafka eventbus")
	return nil
}

// reconnect 执行重连逻辑
func (k *kafkaEventBus) reconnect(ctx context.Context) error {
	k.logger.Warn("Kafka connection failed, attempting to reconnect...")

	// 记录重连时间
	k.lastReconnectTime.Store(time.Now())

	backoff := k.reconnectConfig.InitialBackoff

	for attempt := 1; attempt <= k.reconnectConfig.MaxAttempts; attempt++ {
		k.logger.Info("Reconnection attempt",
			zap.Int("attempt", attempt),
			zap.Int("maxAttempts", k.reconnectConfig.MaxAttempts))

		// 尝试重新初始化连接
		if err := k.reinitializeConnection(); err != nil {
			k.logger.Error("Reconnection attempt failed",
				zap.Int("attempt", attempt),
				zap.Error(err))

			// 如果不是最后一次尝试，等待后重试
			if attempt < k.reconnectConfig.MaxAttempts {
				select {
				case <-ctx.Done():
					return fmt.Errorf("reconnection cancelled: %w", ctx.Err())
				case <-time.After(backoff):
					// 指数退避
					backoff = time.Duration(float64(backoff) * k.reconnectConfig.BackoffFactor)
					if backoff > k.reconnectConfig.MaxBackoff {
						backoff = k.reconnectConfig.MaxBackoff
					}
				}
			}
			continue
		}

		// 重连成功，恢复订阅
		if err := k.restoreSubscriptions(ctx); err != nil {
			k.logger.Error("Failed to restore subscriptions after reconnect", zap.Error(err))
			return fmt.Errorf("failed to restore subscriptions: %w", err)
		}

		// 重置失败计数
		k.failureCount.Store(0)

		k.logger.Info("Kafka reconnection successful", zap.Int("attempt", attempt))

		// 调用重连回调
		if k.reconnectCallback != nil {
			k.reconnectCallback(ctx)
		}

		return nil
	}

	return fmt.Errorf("failed to reconnect after %d attempts", k.reconnectConfig.MaxAttempts)
}

// reinitializeConnection 重新初始化连接
func (k *kafkaEventBus) reinitializeConnection() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	// ✅ 关闭现有连接
	if producer, err := k.getAsyncProducer(); err == nil && producer != nil {
		producer.Close()
	}
	if consumer, err := k.getConsumer(); err == nil && consumer != nil {
		consumer.Close()
	}
	if admin, err := k.getAdmin(); err == nil && admin != nil {
		admin.Close()
	}
	if client, err := k.getClient(); err == nil && client != nil {
		client.Close()
	}

	// 创建Sarama配置
	saramaConfig := sarama.NewConfig()
	// 已废弃：使用新的直接配置方式
	// if err := configureSarama(saramaConfig, k.config); err != nil {
	//	return fmt.Errorf("failed to configure sarama: %w", err)
	// }

	// 重新创建客户端
	client, err := sarama.NewClient(k.config.Brokers, saramaConfig)
	if err != nil {
		return fmt.Errorf("failed to create kafka client: %w", err)
	}

	// 优化1：重新创建AsyncProducer
	asyncProducer, err := sarama.NewAsyncProducerFromClient(client)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to create kafka async producer: %w", err)
	}

	// 重新创建消费者
	consumer, err := sarama.NewConsumerFromClient(client)
	if err != nil {
		asyncProducer.Close()
		client.Close()
		return fmt.Errorf("failed to create kafka consumer: %w", err)
	}

	// 重新创建管理客户端
	admin, err := sarama.NewClusterAdminFromClient(client)
	if err != nil {
		consumer.Close()
		asyncProducer.Close()
		client.Close()
		return fmt.Errorf("failed to create kafka admin: %w", err)
	}

	// ✅ 使用 atomic.Value 存储
	k.client.Store(client)
	k.asyncProducer.Store(asyncProducer)
	k.consumer.Store(consumer)
	k.admin.Store(admin)

	// 优化1：重新启动AsyncProducer处理goroutine
	go k.handleAsyncProducerSuccess()
	go k.handleAsyncProducerErrors()

	k.logger.Info("Kafka connection reinitialized successfully (AsyncProducer mode)")
	return nil
}

// restoreSubscriptions 恢复订阅
func (k *kafkaEventBus) restoreSubscriptions(ctx context.Context) error {
	// ✅ 使用 sync.Map 遍历订阅
	subscriptions := make(map[string]MessageHandler)
	k.subscriptions.Range(func(key, value interface{}) bool {
		topic := key.(string)
		handler := value.(MessageHandler)
		subscriptions[topic] = handler
		return true
	})

	for topic, handler := range subscriptions {
		k.logger.Info("Restoring subscription", zap.String("topic", topic))
		if err := k.Subscribe(ctx, topic, handler); err != nil {
			k.logger.Error("Failed to restore subscription",
				zap.String("topic", topic),
				zap.Error(err))
			return fmt.Errorf("failed to restore subscription for topic %s: %w", topic, err)
		}
	}

	k.logger.Info("All subscriptions restored successfully",
		zap.Int("count", len(subscriptions)))
	return nil
}

// SetReconnectConfig 设置重连配置
func (k *kafkaEventBus) SetReconnectConfig(config ReconnectConfig) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.reconnectConfig = config
	k.logger.Info("Reconnect config updated",
		zap.Int("maxAttempts", config.MaxAttempts),
		zap.Duration("initialBackoff", config.InitialBackoff),
		zap.Duration("maxBackoff", config.MaxBackoff),
		zap.Float64("backoffFactor", config.BackoffFactor),
		zap.Int("failureThreshold", config.FailureThreshold))
	return nil
}

// GetReconnectStatus 获取重连状态
func (k *kafkaEventBus) GetReconnectStatus() ReconnectStatus {
	failures := k.failureCount.Load()
	lastReconnect := k.lastReconnectTime.Load().(time.Time)

	return ReconnectStatus{
		FailureCount:      int(failures),
		LastReconnectTime: lastReconnect,
		IsReconnecting:    false, // 简化实现，实际可以添加状态跟踪
		Config:            k.reconnectConfig,
	}
}

// ReconnectStatus 重连状态
type ReconnectStatus struct {
	FailureCount      int             `json:"failure_count"`
	LastReconnectTime time.Time       `json:"last_reconnect_time"`
	IsReconnecting    bool            `json:"is_reconnecting"`
	Config            ReconnectConfig `json:"config"`
}

// ========== 生命周期管理 ==========

// Start 启动事件总线
func (k *kafkaEventBus) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	// 🔥 P0修复：使用 atomic.Bool 读取关闭状态
	if k.closed.Load() {
		return fmt.Errorf("kafka eventbus is closed")
	}

	k.logger.Info("Kafka eventbus started successfully")
	return nil
}

// Stop 停止事件总线
func (k *kafkaEventBus) Stop() error {
	return k.Close()
}

// ========== 高级发布功能 ==========

// PublishWithOptions 使用选项发布消息
func (k *kafkaEventBus) PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error {
	start := time.Now()

	// 应用消息格式化器
	if k.messageFormatter != nil {
		// 生成消息UUID
		msgUUID := fmt.Sprintf("%d", time.Now().UnixNano())
		formattedMsg, err := k.messageFormatter.FormatMessage(msgUUID, opts.AggregateID, message)
		if err != nil {
			k.logger.Error("Failed to format message", zap.Error(err))
			return fmt.Errorf("failed to format message: %w", err)
		}

		// 设置元数据
		if opts.Metadata != nil {
			if err := k.messageFormatter.SetMetadata(formattedMsg, opts.Metadata); err != nil {
				k.logger.Error("Failed to set metadata", zap.Error(err))
				return fmt.Errorf("failed to set metadata: %w", err)
			}
		}

		// 将格式化后的消息转换为字节
		if formattedMsg.Payload != nil {
			message = formattedMsg.Payload
		}
	}

	// 构建 Kafka 消息
	kafkaMsg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(message),
	}

	// 设置消息头
	if opts.Metadata != nil {
		headers := make([]sarama.RecordHeader, 0, len(opts.Metadata))
		for key, value := range opts.Metadata {
			headers = append(headers, sarama.RecordHeader{
				Key:   []byte(key),
				Value: []byte(value),
			})
		}
		kafkaMsg.Headers = headers
	}

	// 设置分区键（如果提供了 AggregateID）
	if opts.AggregateID != nil {
		var aggregateKey string
		if k.messageFormatter != nil {
			aggregateKey = k.messageFormatter.ExtractAggregateID(opts.AggregateID)
		} else {
			aggregateKey = fmt.Sprintf("%v", opts.AggregateID)
		}
		if aggregateKey != "" {
			kafkaMsg.Key = sarama.StringEncoder(aggregateKey)
		}
	}

	// ✅ 无锁读取 asyncProducer
	producer, err := k.getAsyncProducer()
	if err != nil {
		k.errorCount.Add(1)
		return fmt.Errorf("failed to get async producer: %w", err)
	}

	// 优化1：使用AsyncProducer异步发布
	select {
	case producer.Input() <- kafkaMsg:
		// 消息已提交到发送队列
		duration := time.Since(start)
		k.logger.Debug("Message with options queued for async publishing",
			zap.String("topic", topic),
			zap.Duration("duration", duration))
		return nil
	case <-time.After(100 * time.Millisecond):
		// 发送队列满，应用背压
		k.logger.Warn("Async producer input queue full for message with options",
			zap.String("topic", topic))
		// 阻塞等待
		producer.Input() <- kafkaMsg
		return nil
	}
}

// SetMessageFormatter 设置消息格式化器
func (k *kafkaEventBus) SetMessageFormatter(formatter MessageFormatter) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.messageFormatter = formatter
	k.logger.Info("Message formatter set for kafka eventbus")
	return nil
}

// RegisterPublishCallback 注册发布回调
func (k *kafkaEventBus) RegisterPublishCallback(callback PublishCallback) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.publishCallback = callback
	k.logger.Debug("Publish callback registered for kafka eventbus")
	return nil
}

// ========== 高级订阅功能 ==========

// SubscribeWithOptions 使用选项订阅消息
func (k *kafkaEventBus) SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error {
	// 包装处理器以支持企业特性
	wrappedHandler := func(ctx context.Context, message []byte) error {
		start := time.Now()

		// 应用流量控制
		if k.rateLimiter != nil {
			if !k.rateLimiter.Allow() {
				k.logger.Warn("Message dropped due to rate limiting", zap.String("topic", topic))
				return fmt.Errorf("rate limit exceeded")
			}
		}

		// 直接处理消息
		err := handler(ctx, message)

		// 记录指标
		duration := time.Since(start)
		if err != nil {
			k.errorCount.Add(1)
			k.logger.Error("Message processing failed",
				zap.String("topic", topic),
				zap.Error(err),
				zap.Duration("duration", duration))

			// 调用错误处理器
			if k.errorHandler != nil {
				go func() {
					errorAction := k.errorHandler.HandleError(ctx, err, message, topic)
					k.logger.Info("Error handler executed",
						zap.String("action", string(errorAction.Action)),
						zap.Bool("deadLetter", errorAction.DeadLetter),
						zap.Bool("skipMessage", errorAction.SkipMessage))
				}()
			}
		} else {
			k.consumedMessages.Add(1)
			k.logger.Debug("Message processed successfully",
				zap.String("topic", topic),
				zap.Duration("duration", duration))
		}

		return err
	}

	// 使用包装后的处理器订阅
	return k.Subscribe(ctx, topic, wrappedHandler)
}

// RegisterSubscriberBacklogCallback 注册订阅端积压回调
func (k *kafkaEventBus) RegisterSubscriberBacklogCallback(callback BacklogStateCallback) error {
	if k.backlogDetector != nil {
		return k.backlogDetector.RegisterCallback(callback)
	}
	k.logger.Info("Subscriber backlog callback registered (detector not available)")
	return nil
}

// StartSubscriberBacklogMonitoring 启动订阅端积压监控
func (k *kafkaEventBus) StartSubscriberBacklogMonitoring(ctx context.Context) error {
	if k.backlogDetector != nil {
		return k.backlogDetector.Start(ctx)
	}
	k.logger.Info("Subscriber backlog monitoring not available")
	return nil
}

// StopSubscriberBacklogMonitoring 停止订阅端积压监控
func (k *kafkaEventBus) StopSubscriberBacklogMonitoring() error {
	if k.backlogDetector != nil {
		return k.backlogDetector.Stop()
	}
	k.logger.Info("Subscriber backlog monitoring not available")
	return nil
}

// RegisterPublisherBacklogCallback 注册发送端积压回调
func (k *kafkaEventBus) RegisterPublisherBacklogCallback(callback PublisherBacklogCallback) error {
	if k.publisherBacklogDetector != nil {
		return k.publisherBacklogDetector.RegisterCallback(callback)
	}
	k.logger.Debug("Publisher backlog callback registered (detector not available)")
	return nil
}

// StartPublisherBacklogMonitoring 启动发送端积压监控
func (k *kafkaEventBus) StartPublisherBacklogMonitoring(ctx context.Context) error {
	if k.publisherBacklogDetector != nil {
		return k.publisherBacklogDetector.Start(ctx)
	}
	k.logger.Debug("Publisher backlog monitoring not available (not configured)")
	return nil
}

// StopPublisherBacklogMonitoring 停止发送端积压监控
func (k *kafkaEventBus) StopPublisherBacklogMonitoring() error {
	if k.publisherBacklogDetector != nil {
		return k.publisherBacklogDetector.Stop()
	}
	k.logger.Debug("Publisher backlog monitoring not available (not configured)")
	return nil
}

// StartAllBacklogMonitoring 根据配置启动所有积压监控
func (k *kafkaEventBus) StartAllBacklogMonitoring(ctx context.Context) error {
	var errs []error

	// 启动订阅端积压监控
	if err := k.StartSubscriberBacklogMonitoring(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to start subscriber backlog monitoring: %w", err))
	}

	// 启动发送端积压监控
	if err := k.StartPublisherBacklogMonitoring(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to start publisher backlog monitoring: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to start some backlog monitoring: %v", errs)
	}

	k.logger.Info("All backlog monitoring started successfully")
	return nil
}

// StopAllBacklogMonitoring 停止所有积压监控
func (k *kafkaEventBus) StopAllBacklogMonitoring() error {
	var errs []error

	// 停止订阅端积压监控
	if err := k.StopSubscriberBacklogMonitoring(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop subscriber backlog monitoring: %w", err))
	}

	// 停止发送端积压监控
	if err := k.StopPublisherBacklogMonitoring(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop publisher backlog monitoring: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop some backlog monitoring: %v", errs)
	}

	k.logger.Info("All backlog monitoring stopped successfully")
	return nil
}

// SetMessageRouter 设置消息路由器
func (k *kafkaEventBus) SetMessageRouter(router MessageRouter) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.messageRouter = router
	k.logger.Info("Message router set for kafka eventbus")
	return nil
}

// SetErrorHandler 设置错误处理器
func (k *kafkaEventBus) SetErrorHandler(handler ErrorHandler) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.errorHandler = handler
	k.logger.Info("Error handler set for kafka eventbus")
	return nil
}

// RegisterSubscriptionCallback 注册订阅回调
func (k *kafkaEventBus) RegisterSubscriptionCallback(callback SubscriptionCallback) error {
	k.logger.Info("Subscription callback registered for kafka eventbus")
	return nil
}

// ========== 统一健康检查和监控 ==========

// StartHealthCheck 启动健康检查
func (k *kafkaEventBus) StartHealthCheck(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	// 如果已经启动，先停止之前的
	if k.healthCheckCancel != nil {
		k.healthCheckCancel()
		if k.healthCheckDone != nil {
			<-k.healthCheckDone // 等待之前的健康检查完全停止
		}
	}

	// 创建新的控制 context
	healthCtx, cancel := context.WithCancel(ctx)
	k.healthCheckCancel = cancel
	k.healthCheckDone = make(chan struct{})

	// 启动健康检查协程
	go func() {
		defer close(k.healthCheckDone)

		ticker := time.NewTicker(2 * time.Minute) // 默认健康检查间隔
		defer ticker.Stop()

		for {
			select {
			case <-healthCtx.Done():
				k.logger.Info("Health check stopped for kafka eventbus")
				return
			case <-ticker.C:
				if err := k.healthCheck(healthCtx); err != nil {
					k.logger.Error("Health check failed", zap.Error(err))

					// 增加失败计数
					failures := k.failureCount.Add(1)
					k.logger.Warn("Health check failure count increased",
						zap.Int32("failures", failures),
						zap.Int("threshold", k.reconnectConfig.FailureThreshold))

					// 检查是否需要触发重连
					if int(failures) >= k.reconnectConfig.FailureThreshold {
						k.logger.Warn("Health check failure threshold reached, triggering reconnection",
							zap.Int32("failures", failures),
							zap.Int("threshold", k.reconnectConfig.FailureThreshold))

						// 执行重连
						if reconnectErr := k.reconnect(healthCtx); reconnectErr != nil {
							k.logger.Error("Automatic reconnection failed", zap.Error(reconnectErr))
						} else {
							k.logger.Info("Automatic reconnection successful")
						}
					}
				} else {
					// 健康检查成功，重置失败计数
					if k.failureCount.Load() > 0 {
						k.failureCount.Store(0)
						k.logger.Debug("Health check passed, failure count reset")
					}
				}
			}
		}
	}()

	k.logger.Info("Health check started for kafka eventbus")
	return nil
}

// StopHealthCheck 停止健康检查
func (k *kafkaEventBus) StopHealthCheck() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.healthCheckCancel != nil {
		// 取消健康检查 context
		k.healthCheckCancel()

		// 等待健康检查 goroutine 完全停止
		if k.healthCheckDone != nil {
			<-k.healthCheckDone
		}

		// 清理资源
		k.healthCheckCancel = nil
		k.healthCheckDone = nil

		k.logger.Info("Health check stopped for kafka eventbus")
	} else {
		k.logger.Debug("Health check was not running")
	}

	return nil
}

// GetHealthStatus 获取健康状态
func (k *kafkaEventBus) GetHealthStatus() HealthCheckStatus {
	// ✅ 无锁检查 client 是否可用
	_, clientErr := k.getClient()
	// 🔥 P0修复：使用 atomic.Bool 读取关闭状态
	isHealthy := !k.closed.Load() && clientErr == nil

	return HealthCheckStatus{
		IsHealthy:           isHealthy,
		ConsecutiveFailures: 0,
		LastSuccessTime:     time.Now(),
		LastFailureTime:     time.Time{},
		IsRunning:           !k.closed.Load(),
		EventBusType:        "kafka",
		Source:              "kafka-eventbus",
	}
}

// RegisterHealthCheckCallback 注册健康检查回调
func (k *kafkaEventBus) RegisterHealthCheckCallback(callback HealthCheckCallback) error {
	k.logger.Info("Health check callback registered for kafka eventbus")
	return nil
}

// StartHealthCheckSubscriber 启动健康检查消息订阅监控
func (k *kafkaEventBus) StartHealthCheckSubscriber(ctx context.Context) error {
	k.mu.Lock()

	if k.healthCheckSubscriber != nil {
		k.mu.Unlock()
		return nil // 已经启动
	}

	// 🔧 修复：使用保存的健康检查配置（如果未配置，则使用默认配置）
	// 与 StartHealthCheckPublisher 保持一致
	config := k.healthCheckConfig
	if !config.Enabled {
		config = GetDefaultHealthCheckConfig()
	}
	k.healthCheckSubscriber = NewHealthCheckSubscriber(config, k, "kafka-eventbus", "kafka")

	// 🔧 修复死锁：在调用 Start 之前释放锁
	// Start 方法内部会调用 Subscribe，而 Subscribe 也需要获取 k.mu 锁
	// 如果不释放锁，会导致死锁
	subscriber := k.healthCheckSubscriber
	k.mu.Unlock()

	// 启动监控器（不持有锁）
	if err := subscriber.Start(ctx); err != nil {
		// 启动失败，需要清理
		k.mu.Lock()
		k.healthCheckSubscriber = nil
		k.mu.Unlock()
		return fmt.Errorf("failed to start health check subscriber: %w", err)
	}

	k.logger.Info("Health check subscriber started for kafka eventbus")
	return nil
}

// StopHealthCheckSubscriber 停止健康检查消息订阅监控
func (k *kafkaEventBus) StopHealthCheckSubscriber() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.healthCheckSubscriber == nil {
		return nil
	}

	if err := k.healthCheckSubscriber.Stop(); err != nil {
		k.logger.Error("Failed to stop health check subscriber", zap.Error(err))
		return err
	}

	k.healthCheckSubscriber = nil
	k.logger.Info("Health check subscriber stopped for kafka eventbus")
	return nil
}

// RegisterHealthCheckAlertCallback 注册健康检查告警回调
func (k *kafkaEventBus) RegisterHealthCheckAlertCallback(callback HealthCheckAlertCallback) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.healthCheckSubscriber == nil {
		return fmt.Errorf("health check subscriber not started")
	}

	return k.healthCheckSubscriber.RegisterAlertCallback(callback)
}

// GetHealthCheckSubscriberStats 获取健康检查订阅监控统计信息
func (k *kafkaEventBus) GetHealthCheckSubscriberStats() HealthCheckSubscriberStats {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.healthCheckSubscriber == nil {
		return HealthCheckSubscriberStats{}
	}

	return k.healthCheckSubscriber.GetStats()
}

// GetConnectionState 获取连接状态
func (k *kafkaEventBus) GetConnectionState() ConnectionState {
	// ✅ 无锁检查 client 是否可用
	_, clientErr := k.getClient()
	// 🔥 P0修复：使用 atomic.Bool 读取关闭状态
	isConnected := !k.closed.Load() && clientErr == nil

	return ConnectionState{
		IsConnected:       isConnected,
		LastConnectedTime: time.Now(),
		ReconnectCount:    0,
		LastError:         "",
	}
}

// GetMetrics 获取监控指标
func (k *kafkaEventBus) GetMetrics() Metrics {
	return Metrics{
		MessagesPublished: k.publishedMessages.Load(),
		MessagesConsumed:  k.consumedMessages.Load(),
		PublishErrors:     k.errorCount.Load(),
		ConsumeErrors:     0,
		ConnectionErrors:  0,
		LastHealthCheck:   time.Now(),
		HealthCheckStatus: "healthy",
		ActiveConnections: 1,
		MessageBacklog:    0,
	}
}

// ========== 方案A：Envelope 支持 ==========

// PublishEnvelope 发布Envelope消息（方案A）
func (k *kafkaEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
	// 🔥 P0修复：高频路径优化 - 无锁检查关闭状态
	if k.closed.Load() {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 校验Envelope
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("invalid envelope: %w", err)
	}

	// 发布端流量控制
	if k.publishRateLimiter != nil {
		if err := k.publishRateLimiter.Wait(ctx); err != nil {
			k.errorCount.Add(1)
			return fmt.Errorf("publisher rate limit error: %w", err)
		}
	}

	// 订阅端流量控制（保留兼容性）
	if k.rateLimiter != nil {
		if err := k.rateLimiter.Wait(ctx); err != nil {
			k.errorCount.Add(1)
			return fmt.Errorf("subscriber rate limit error: %w", err)
		}
	}

	// 序列化Envelope
	envelopeBytes, err := envelope.ToBytes()
	if err != nil {
		k.errorCount.Add(1)
		return fmt.Errorf("failed to serialize envelope: %w", err)
	}

	// 创建Kafka消息（方案A + 传输层镜像）
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(envelope.AggregateID), // 镜像到Kafka Key
		Value: sarama.ByteEncoder(envelopeBytes),
		Headers: []sarama.RecordHeader{
			{Key: []byte("X-Event-ID"), Value: []byte(envelope.EventID)}, // ← 添加 EventID
			{Key: []byte("X-Aggregate-ID"), Value: []byte(envelope.AggregateID)},
			{Key: []byte("X-Event-Version"), Value: []byte(fmt.Sprintf("%d", envelope.EventVersion))},
			{Key: []byte("X-Event-Type"), Value: []byte(envelope.EventType)},
		},
	}

	// 添加可选字段到Header
	if envelope.TraceID != "" {
		msg.Headers = append(msg.Headers, sarama.RecordHeader{
			Key: []byte("X-Trace-ID"), Value: []byte(envelope.TraceID),
		})
	}
	if envelope.CorrelationID != "" {
		msg.Headers = append(msg.Headers, sarama.RecordHeader{
			Key: []byte("X-Correlation-ID"), Value: []byte(envelope.CorrelationID),
		})
	}
	if envelope.TenantID != "" {
		msg.Headers = append(msg.Headers, sarama.RecordHeader{
			Key: []byte("X-Tenant-ID"), Value: []byte(envelope.TenantID), // ← 租户ID（多租户支持，用于Outbox ACK路由）
		})
	}

	// ✅ 无锁读取 asyncProducer
	producer, err := k.getAsyncProducer()
	if err != nil {
		k.errorCount.Add(1)
		return fmt.Errorf("failed to get async producer: %w", err)
	}

	// 优化1：使用AsyncProducer异步发送
	select {
	case producer.Input() <- msg:
		// 消息已提交到发送队列
		k.logger.Debug("Envelope message queued for async publishing",
			zap.String("topic", topic),
			zap.String("eventID", envelope.EventID),
			zap.String("aggregateID", envelope.AggregateID),
			zap.String("eventType", envelope.EventType),
			zap.Int64("eventVersion", envelope.EventVersion))
		return nil
	case <-time.After(100 * time.Millisecond):
		// 发送队列满，应用背压
		k.logger.Warn("Async producer input queue full for envelope message",
			zap.String("topic", topic),
			zap.String("eventID", envelope.EventID),
			zap.String("aggregateID", envelope.AggregateID))
		// 阻塞等待
		producer.Input() <- msg
		return nil
	}
}

// SubscribeEnvelope 订阅Envelope消息（使用 Hollywood Actor Pool + 智能路由）
//
// 特点：
// - 消息格式：Envelope包装格式（包含聚合ID、事件类型、版本等元数据）
// - 处理模式：按聚合ID路由到 Hollywood Actor Pool，同聚合ID严格顺序处理
// - 性能：顺序保证，毫秒级延迟
// - 聚合ID：从Envelope.AggregateID字段提取
// - 路由策略：
//   - 有聚合ID：一致性哈希路由到固定 Actor（保证顺序）
//   - 无聚合ID：Round-Robin 轮询路由（保证并发）
//
// 核心机制：
// 1. 消息必须是Envelope格式，包含AggregateID
// 2. ExtractAggregateID成功提取聚合ID
// 3. 使用一致性哈希将相同聚合ID路由到固定 Actor
// 4. 确保同一聚合的事件严格按序处理
//
// 适用场景：
// - 领域事件处理（订单状态变更、用户行为）
// - 事件溯源（Event Sourcing）
// - 聚合管理（DDD聚合根）
// - 需要顺序保证的业务场景
//
// 示例：
//
//	bus.SubscribeEnvelope(ctx, "orders.events", func(ctx context.Context, env *Envelope) error {
//	    // env.AggregateID = "order-123"
//	    // 同一订单的所有事件会路由到同一个 Actor，确保顺序处理
//	    return processDomainEvent(env)
//	})
func (k *kafkaEventBus) SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error {
	// 包装EnvelopeHandler为MessageHandler
	wrappedHandler := func(ctx context.Context, message []byte) error {
		// 尝试解析为Envelope
		envelope, err := FromBytes(message)
		if err != nil {
			k.logger.Error("Failed to parse envelope message",
				zap.String("topic", topic),
				zap.Error(err))
			return fmt.Errorf("failed to parse envelope: %w", err)
		}

		// 调用业务处理器
		return handler(ctx, envelope)
	}

	// ⭐ 重要：不能直接调用 Subscribe，因为需要设置 isEnvelope = true
	// 复制 Subscribe 的逻辑，但使用 isEnvelope = true
	k.mu.Lock()

	// 🔥 P0修复：使用 atomic.Bool 读取关闭状态
	if k.closed.Load() {
		k.mu.Unlock()
		return fmt.Errorf("kafka eventbus is closed")
	}

	// ✅ 使用 sync.Map 存储订阅信息（用于重连后恢复）
	// 使用 LoadOrStore 检查重复订阅
	if _, loaded := k.subscriptions.LoadOrStore(topic, wrappedHandler); loaded {
		k.mu.Unlock()
		return fmt.Errorf("already subscribed to topic: %s", topic)
	}

	// 关键修复：添加 topic 到预订阅列表
	k.addTopicToPreSubscription(topic)

	// 激活topic处理器（立即生效）
	// ⭐ SubscribeEnvelope：isEnvelope = true（at-least-once 语义）
	k.activateTopicHandler(topic, wrappedHandler, true)

	// 🔧 修复死锁：在释放锁之前记录日志，然后释放锁再启动consumer
	k.logger.Info("Subscribed to envelope topic via pre-subscription consumer",
		zap.String("topic", topic),
		zap.String("groupID", k.config.Consumer.GroupID))

	// 释放锁，避免在启动consumer时持有锁导致死锁
	k.mu.Unlock()

	// 启动预订阅消费者（如果还未启动）
	if err := k.startPreSubscriptionConsumer(ctx); err != nil {
		return fmt.Errorf("failed to start pre-subscription consumer: %w", err)
	}

	return nil
}

// ========== 新的分离式健康检查接口实现 ==========

// StartHealthCheckPublisher 启动健康检查发布器
func (k *kafkaEventBus) StartHealthCheckPublisher(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.healthChecker != nil {
		return nil // 已经启动
	}

	// 使用保存的健康检查配置（如果未配置，则使用默认配置）
	config := k.healthCheckConfig
	if !config.Enabled {
		config = GetDefaultHealthCheckConfig()
	}

	k.healthChecker = NewHealthChecker(config, k, "kafka-eventbus", "kafka")

	// 启动健康检查发布器
	if err := k.healthChecker.Start(ctx); err != nil {
		k.healthChecker = nil
		return fmt.Errorf("failed to start health check publisher: %w", err)
	}

	k.logger.Info("Health check publisher started for kafka eventbus",
		zap.Duration("interval", config.Publisher.Interval),
		zap.String("topic", config.Publisher.Topic))
	return nil
}

// StopHealthCheckPublisher 停止健康检查发布器
func (k *kafkaEventBus) StopHealthCheckPublisher() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.healthChecker == nil {
		return nil
	}

	if err := k.healthChecker.Stop(); err != nil {
		return fmt.Errorf("failed to stop health check publisher: %w", err)
	}

	k.healthChecker = nil
	k.logger.Info("Health check publisher stopped for kafka eventbus")
	return nil
}

// GetHealthCheckPublisherStatus 获取健康检查发布器状态
func (k *kafkaEventBus) GetHealthCheckPublisherStatus() HealthCheckStatus {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.healthChecker == nil {
		return HealthCheckStatus{
			IsHealthy:           false,
			ConsecutiveFailures: 0,
			LastSuccessTime:     time.Time{},
			LastFailureTime:     time.Now(),
			IsRunning:           false,
			EventBusType:        "kafka",
			Source:              "kafka-eventbus",
		}
	}

	return k.healthChecker.GetStatus()
}

// RegisterHealthCheckPublisherCallback 注册健康检查发布器回调
func (k *kafkaEventBus) RegisterHealthCheckPublisherCallback(callback HealthCheckCallback) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.healthChecker == nil {
		return fmt.Errorf("health check publisher not started")
	}

	return k.healthChecker.RegisterCallback(callback)
}

// RegisterHealthCheckSubscriberCallback 注册健康检查订阅器回调
func (k *kafkaEventBus) RegisterHealthCheckSubscriberCallback(callback HealthCheckAlertCallback) error {
	return k.RegisterHealthCheckAlertCallback(callback)
}

// StartAllHealthCheck 根据配置启动所有健康检查
func (k *kafkaEventBus) StartAllHealthCheck(ctx context.Context) error {
	// 这里可以根据配置决定启动哪些健康检查
	// 为了演示，我们启动发布器和订阅器
	if err := k.StartHealthCheckPublisher(ctx); err != nil {
		return fmt.Errorf("failed to start health check publisher: %w", err)
	}

	if err := k.StartHealthCheckSubscriber(ctx); err != nil {
		return fmt.Errorf("failed to start health check subscriber: %w", err)
	}

	return nil
}

// StopAllHealthCheck 停止所有健康检查
func (k *kafkaEventBus) StopAllHealthCheck() error {
	var errs []error

	if err := k.StopHealthCheckPublisher(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop health check publisher: %w", err))
	}

	if err := k.StopHealthCheckSubscriber(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop health check subscriber: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping health checks: %v", errs)
	}

	return nil
}

// ========== 主题持久化管理实现 ==========

// ConfigureTopic 配置主题的持久化策略和其他选项（幂等操作）
func (k *kafkaEventBus) ConfigureTopic(ctx context.Context, topic string, options TopicOptions) error {
	start := time.Now()

	// 验证主题名称
	if err := ValidateTopicName(topic); err != nil {
		return err
	}

	// ✅ 低频路径：保留锁，保持代码清晰
	k.mu.Lock()
	// 🔥 P0修复：使用 atomic.Bool 读取关闭状态
	if k.closed.Load() {
		k.mu.Unlock()
		return fmt.Errorf("kafka eventbus is closed")
	}
	k.mu.Unlock()

	// 检查是否已有配置
	_, exists := k.topicConfigs.Load(topic)
	// 缓存配置
	k.topicConfigs.Store(topic, options)

	// 根据策略决定是否需要同步到消息中间件
	shouldCreate, shouldUpdate := shouldCreateOrUpdate(k.topicConfigStrategy, exists)

	var action string
	var err error
	var mismatches []TopicConfigMismatch

	switch {
	case k.topicConfigStrategy == StrategySkip:
		// 跳过模式：不检查
		action = "skipped"

	case k.topicConfigStrategy == StrategyValidateOnly:
		// 验证模式：只验证，不修改
		action = "validated"
		if exists {
			actualConfig, validateErr := k.getActualTopicConfig(ctx, topic)
			if validateErr == nil {
				mismatches = compareTopicOptions(topic, options, actualConfig)
				if len(mismatches) > 0 {
					err = handleConfigMismatches(mismatches, k.topicConfigOnMismatch)
				}
			}
		}

	case shouldCreate:
		// 创建模式：创建新配置
		action = "created"
		err = k.ensureKafkaTopicIdempotent(ctx, topic, options, false)

	case shouldUpdate:
		// 更新模式：更新现有配置
		action = "updated"
		// 先验证配置差异
		actualConfig, validateErr := k.getActualTopicConfig(ctx, topic)
		if validateErr == nil {
			mismatches = compareTopicOptions(topic, options, actualConfig)
		}
		// 执行更新
		err = k.ensureKafkaTopicIdempotent(ctx, topic, options, true)

	default:
		// 默认：创建或更新
		action = "configured"
		err = k.ensureKafkaTopicIdempotent(ctx, topic, options, exists)
	}

	duration := time.Since(start)

	// 记录结果
	if err != nil {
		k.logger.Error("Topic configuration failed",
			zap.String("topic", topic),
			zap.String("action", action),
			zap.String("strategy", string(k.topicConfigStrategy)),
			zap.Error(err),
			zap.Duration("duration", duration))
		return fmt.Errorf("failed to configure topic %s: %w", topic, err)
	}

	k.logger.Info("Topic configured successfully",
		zap.String("topic", topic),
		zap.String("action", action),
		zap.String("strategy", string(k.topicConfigStrategy)),
		zap.String("persistenceMode", string(options.PersistenceMode)),
		zap.Duration("retentionTime", options.RetentionTime),
		zap.Int64("maxSize", options.MaxSize),
		zap.Int("mismatches", len(mismatches)),
		zap.Duration("duration", duration))

	return nil
}

// SetTopicPersistence 设置主题是否持久化（简化接口）
func (k *kafkaEventBus) SetTopicPersistence(ctx context.Context, topic string, persistent bool) error {
	mode := TopicEphemeral
	if persistent {
		mode = TopicPersistent
	}

	options := DefaultTopicOptions()
	options.PersistenceMode = mode

	return k.ConfigureTopic(ctx, topic, options)
}

// GetTopicConfig 获取主题的当前配置
func (k *kafkaEventBus) GetTopicConfig(topic string) (TopicOptions, error) {
	// 🔥 高频路径：无锁读取配置
	if configAny, exists := k.topicConfigs.Load(topic); exists {
		return configAny.(TopicOptions), nil
	}

	// 返回默认配置
	return DefaultTopicOptions(), nil
}

// ListConfiguredTopics 列出所有已配置的主题
func (k *kafkaEventBus) ListConfiguredTopics() []string {
	topics := make([]string, 0)
	k.topicConfigs.Range(func(key, value interface{}) bool {
		topics = append(topics, key.(string))
		return true
	})

	return topics
}

// RemoveTopicConfig 移除主题配置（恢复为默认行为）
func (k *kafkaEventBus) RemoveTopicConfig(topic string) error {
	k.topicConfigs.Delete(topic)

	k.logger.Info("Topic configuration removed", zap.String("topic", topic))
	return nil
}

// ensureKafkaTopic 确保Kafka主题存在并配置正确
func (k *kafkaEventBus) ensureKafkaTopic(topic string, options TopicOptions) error {
	// ✅ 无锁读取 admin
	admin, err := k.getAdmin()
	if err != nil {
		return fmt.Errorf("Kafka admin client not available: %w", err)
	}

	// 检查主题是否已存在
	metadata, err := admin.DescribeTopics([]string{topic})
	if err != nil {
		// 主题不存在，创建新主题
		return k.createKafkaTopic(topic, options)
	}

	// 检查是否找到主题
	if len(metadata) == 0 {
		return k.createKafkaTopic(topic, options)
	}

	// 主题已存在，可以在这里添加配置更新逻辑
	k.logger.Info("Kafka topic already exists", zap.String("topic", topic))
	return nil
}

// createKafkaTopic 创建新的Kafka主题
func (k *kafkaEventBus) createKafkaTopic(topic string, options TopicOptions) error {
	// ✅ 无锁读取 admin
	admin, err := k.getAdmin()
	if err != nil {
		return fmt.Errorf("Kafka admin client not available: %w", err)
	}

	// 🔥 性能优化：支持多分区配置
	numPartitions := int32(1) // 默认1个分区
	if options.Partitions > 0 {
		numPartitions = int32(options.Partitions)
	}

	// 🔥 高可用优化：支持副本因子配置
	replicationFactor := int16(1) // 默认1个副本
	if options.ReplicationFactor > 0 {
		replicationFactor = int16(options.ReplicationFactor)
	} else if options.Replicas > 0 {
		replicationFactor = int16(options.Replicas)
	}

	// 构建主题配置
	topicDetail := &sarama.TopicDetail{
		NumPartitions:     numPartitions,
		ReplicationFactor: replicationFactor,
		ConfigEntries:     make(map[string]*string),
	}

	// 根据持久化模式设置配置
	if options.IsPersistent(true) { // Kafka默认是持久化的
		// 持久化配置
		if options.RetentionTime > 0 {
			retentionMs := fmt.Sprintf("%d", options.RetentionTime.Milliseconds())
			topicDetail.ConfigEntries["retention.ms"] = &retentionMs
		}
		if options.MaxSize > 0 {
			retentionBytes := fmt.Sprintf("%d", options.MaxSize)
			topicDetail.ConfigEntries["retention.bytes"] = &retentionBytes
		}
		// 设置为持久化存储
		cleanupPolicy := "delete"
		topicDetail.ConfigEntries["cleanup.policy"] = &cleanupPolicy
	} else {
		// 非持久化配置（短期保留）
		shortRetention := "60000" // 1分钟
		topicDetail.ConfigEntries["retention.ms"] = &shortRetention
		cleanupPolicy := "delete"
		topicDetail.ConfigEntries["cleanup.policy"] = &cleanupPolicy
	}

	// 🔥 新增：应用 topic 级别的压缩配置
	if options.Compression != "" && options.Compression != "none" {
		compressionType := options.Compression
		topicDetail.ConfigEntries["compression.type"] = &compressionType
		k.logger.Debug("Applying topic-level compression",
			zap.String("topic", topic),
			zap.String("compression", compressionType))
	}

	// 创建主题
	err = admin.CreateTopic(topic, topicDetail, false)
	if err != nil {
		return fmt.Errorf("failed to create Kafka topic %s: %w", topic, err)
	}

	k.logger.Info("Created Kafka topic",
		zap.String("topic", topic),
		zap.Int("partitions", int(topicDetail.NumPartitions)),
		zap.Int("replicas", int(topicDetail.ReplicationFactor)),
		zap.Bool("persistent", options.IsPersistent(true)),
		zap.String("compression", options.Compression))

	return nil
}

// ensureKafkaTopicIdempotent 幂等地确保Kafka主题存在（支持创建和更新）
func (k *kafkaEventBus) ensureKafkaTopicIdempotent(ctx context.Context, topic string, options TopicOptions, allowUpdate bool) error {
	// ✅ 无锁读取 admin
	admin, err := k.getAdmin()
	if err != nil {
		return fmt.Errorf("Kafka admin client not available: %w", err)
	}

	// 检查主题是否已存在
	metadata, err := admin.DescribeTopics([]string{topic})

	if err != nil || len(metadata) == 0 {
		// 主题不存在，创建新主题
		k.logger.Info("Creating new Kafka topic", zap.String("topic", topic))
		return k.createKafkaTopic(topic, options)
	}

	// 主题已存在
	k.logger.Info("Kafka topic already exists", zap.String("topic", topic))

	// 如果允许更新，更新主题配置
	if allowUpdate {
		configEntries := make(map[string]*string)

		// 构建配置更新
		if options.RetentionTime > 0 {
			retentionMs := fmt.Sprintf("%d", options.RetentionTime.Milliseconds())
			configEntries["retention.ms"] = &retentionMs
		}
		if options.MaxSize > 0 {
			retentionBytes := fmt.Sprintf("%d", options.MaxSize)
			configEntries["retention.bytes"] = &retentionBytes
		}

		// 🔥 新增：更新压缩配置
		if options.Compression != "" && options.Compression != "none" {
			compressionType := options.Compression
			configEntries["compression.type"] = &compressionType
		}

		if len(configEntries) > 0 {
			k.logger.Info("Updating Kafka topic configuration",
				zap.String("topic", topic),
				zap.String("compression", options.Compression))

			err := admin.AlterConfig(sarama.TopicResource, topic, configEntries, false)
			if err != nil {
				k.logger.Warn("Failed to update topic config, using existing config",
					zap.String("topic", topic),
					zap.Error(err))
				// 不返回错误，使用现有配置
			}
		}
	}

	return nil
}

// getActualTopicConfig 获取主题在Kafka中的实际配置
func (k *kafkaEventBus) getActualTopicConfig(ctx context.Context, topic string) (TopicOptions, error) {
	// ✅ 无锁读取 admin
	admin, err := k.getAdmin()
	if err != nil {
		return TopicOptions{}, fmt.Errorf("Kafka admin client not available: %w", err)
	}

	// 获取主题配置
	configs, err := admin.DescribeConfig(sarama.ConfigResource{
		Type: sarama.TopicResource,
		Name: topic,
	})
	if err != nil {
		return TopicOptions{}, fmt.Errorf("failed to describe topic config: %w", err)
	}

	// 解析配置
	actualConfig := TopicOptions{
		PersistenceMode: TopicPersistent, // Kafka默认持久化
	}

	for _, entry := range configs {
		switch entry.Name {
		case "retention.ms":
			if entry.Value != "" {
				if ms, err := time.ParseDuration(entry.Value + "ms"); err == nil {
					actualConfig.RetentionTime = ms
				}
			}
		case "retention.bytes":
			if entry.Value != "" {
				if bytes, err := fmt.Sscanf(entry.Value, "%d", &actualConfig.MaxSize); err == nil {
					_ = bytes
				}
			}
		}
	}

	// ✅ 无锁读取 admin
	admin, adminErr := k.getAdmin()
	if adminErr == nil {
		// 获取分区和副本信息
		metadata, err := admin.DescribeTopics([]string{topic})
		if err == nil && len(metadata) > 0 {
			topicMeta := metadata[0]
			if len(topicMeta.Partitions) > 0 {
				actualConfig.Replicas = len(topicMeta.Partitions[0].Replicas)
			}
		}
	}

	return actualConfig, nil
}

// GetPublishResultChannel 获取异步发布结果通道
// 用于Outbox Processor监听发布结果并更新Outbox状态
//
// ✅ Kafka 实现说明：
// - PublishEnvelope() 会在 Envelope 中生成 EventID 并添加到 Kafka Message Header
// - handleAsyncProducerSuccess() 从 Header 提取 EventID 并发送成功结果到 publishResultChan
// - handleAsyncProducerErrors() 从 Header 提取 EventID 并发送失败结果到 publishResultChan
// - Outbox Processor 可通过此通道获取 ACK 结果
//
// ⚠️ 注意：只有通过 PublishEnvelope() 发布的消息才会发送 ACK 结果
//
//	通过 Publish() 发布的普通消息不会发送 ACK 结果
func (k *kafkaEventBus) GetPublishResultChannel() <-chan *PublishResult {
	return k.publishResultChan
}

// SetTopicConfigStrategy 设置主题配置策略
func (k *kafkaEventBus) SetTopicConfigStrategy(strategy TopicConfigStrategy) {
	// ✅ 低频路径：保留锁
	k.mu.Lock()
	defer k.mu.Unlock()
	k.topicConfigStrategy = strategy
	k.logger.Info("Topic config strategy updated", zap.String("strategy", string(strategy)))
}

// GetTopicConfigStrategy 获取当前主题配置策略
func (k *kafkaEventBus) GetTopicConfigStrategy() TopicConfigStrategy {
	// ✅ 低频路径：保留锁
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.topicConfigStrategy
}

// ==========================================================================
// 多租户 ACK 支持
// ==========================================================================

// sendResultToChannel 发送 ACK 结果到租户专属通道或全局通道
func (k *kafkaEventBus) sendResultToChannel(result *PublishResult) {
	// 优先发送到租户专属通道
	if result.TenantID != "" {
		k.tenantChannelsMu.RLock()
		tenantChan, exists := k.tenantPublishResultChans[result.TenantID]
		k.tenantChannelsMu.RUnlock()

		if exists {
			select {
			case tenantChan <- result:
				// 成功发送到租户通道
				return
			default:
				// 租户通道满，记录警告
				k.logger.Warn("Tenant ACK channel full, falling back to global channel",
					zap.String("tenantID", result.TenantID),
					zap.String("eventID", result.EventID),
					zap.String("topic", result.Topic))
			}
		} else {
			// 租户未注册，记录警告
			k.logger.Warn("Tenant not registered, falling back to global channel",
				zap.String("tenantID", result.TenantID),
				zap.String("eventID", result.EventID))
		}
	}

	// 降级：发送到全局通道（向后兼容）
	select {
	case k.publishResultChan <- result:
		// 成功发送到全局通道
	default:
		// 全局通道也满，记录错误
		k.logger.Error("Both tenant and global ACK channels full, dropping result",
			zap.String("tenantID", result.TenantID),
			zap.String("eventID", result.EventID),
			zap.String("topic", result.Topic),
			zap.Bool("success", result.Success))
	}
}

// RegisterTenant 注册租户（创建租户专属的 ACK Channel）
func (k *kafkaEventBus) RegisterTenant(tenantID string, bufferSize int) error {
	if tenantID == "" {
		return fmt.Errorf("tenantID cannot be empty")
	}

	if bufferSize <= 0 {
		bufferSize = 100000 // 默认缓冲区大小
	}

	k.tenantChannelsMu.Lock()
	defer k.tenantChannelsMu.Unlock()

	// 延迟初始化 map
	if k.tenantPublishResultChans == nil {
		k.tenantPublishResultChans = make(map[string]chan *PublishResult)
	}

	// 检查租户是否已注册
	if _, exists := k.tenantPublishResultChans[tenantID]; exists {
		return fmt.Errorf("tenant %s already registered", tenantID)
	}

	// 创建租户专属 ACK Channel
	k.tenantPublishResultChans[tenantID] = make(chan *PublishResult, bufferSize)

	k.logger.Info("Tenant ACK channel registered",
		zap.String("tenantID", tenantID),
		zap.Int("bufferSize", bufferSize))

	return nil
}

// UnregisterTenant 注销租户（关闭并清理租户的 ACK Channel）
func (k *kafkaEventBus) UnregisterTenant(tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("tenantID cannot be empty")
	}

	k.tenantChannelsMu.Lock()
	defer k.tenantChannelsMu.Unlock()

	// 检查租户是否已注册
	ch, exists := k.tenantPublishResultChans[tenantID]
	if !exists {
		return fmt.Errorf("tenant %s not registered", tenantID)
	}

	// 关闭并删除租户 Channel
	close(ch)
	delete(k.tenantPublishResultChans, tenantID)

	k.logger.Info("Tenant ACK channel unregistered",
		zap.String("tenantID", tenantID))

	return nil
}

// GetTenantPublishResultChannel 获取租户专属的异步发布结果通道
func (k *kafkaEventBus) GetTenantPublishResultChannel(tenantID string) <-chan *PublishResult {
	if tenantID == "" {
		// 返回全局通道（向后兼容）
		return k.publishResultChan
	}

	k.tenantChannelsMu.RLock()
	defer k.tenantChannelsMu.RUnlock()

	if ch, exists := k.tenantPublishResultChans[tenantID]; exists {
		return ch
	}

	// 租户未注册，返回 nil
	k.logger.Warn("Tenant not registered, returning nil channel",
		zap.String("tenantID", tenantID))
	return nil
}

// GetRegisteredTenants 获取所有已注册的租户ID列表
func (k *kafkaEventBus) GetRegisteredTenants() []string {
	k.tenantChannelsMu.RLock()
	defer k.tenantChannelsMu.RUnlock()

	tenants := make([]string, 0, len(k.tenantPublishResultChans))
	for tenantID := range k.tenantPublishResultChans {
		tenants = append(tenants, tenantID)
	}

	return tenants
}
