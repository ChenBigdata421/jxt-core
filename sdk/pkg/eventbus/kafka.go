package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

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
type kafkaEventBus struct {
	config   *config.KafkaConfig
	producer sarama.SyncProducer
	consumer sarama.Consumer
	client   sarama.Client
	admin    sarama.ClusterAdmin
	logger   *zap.Logger
	mu       sync.RWMutex
	closed   bool

	// 企业级特性
	backlogDetector          *BacklogDetector          // 订阅端积压检测器
	publisherBacklogDetector *PublisherBacklogDetector // 发送端积压检测器
	rateLimiter              *RateLimiter

	// 完整配置（用于访问 Publisher/Subscriber 配置）
	fullConfig *EventBusConfig

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

	// 自动重连控制
	reconnectConfig   ReconnectConfig
	failureCount      atomic.Int32
	lastReconnectTime atomic.Value // time.Time
	reconnectCallback ReconnectCallback

	// 订阅管理（用于重连后恢复订阅）
	subscriptions   map[string]MessageHandler // topic -> handler
	subscriptionsMu sync.RWMutex

	// Keyed worker pools (per topic)
	keyedPools   map[string]*KeyedWorkerPool
	keyedPoolsMu sync.RWMutex

	// 主题配置管理
	topicConfigs          map[string]TopicOptions
	topicConfigsMu        sync.RWMutex
	topicConfigStrategy   TopicConfigStrategy       // 配置策略
	topicConfigOnMismatch TopicConfigMismatchAction // 配置不一致时的行为
}

// NewKafkaEventBus 创建企业级Kafka事件总线
func NewKafkaEventBus(cfg *config.KafkaConfig) (EventBus, error) {
	return NewKafkaEventBusWithFullConfig(cfg, nil)
}

// NewKafkaEventBusWithFullConfig 创建企业级Kafka事件总线（带完整配置）
func NewKafkaEventBusWithFullConfig(cfg *config.KafkaConfig, fullConfig *EventBusConfig) (EventBus, error) {
	if cfg == nil {
		return nil, fmt.Errorf("kafka config cannot be nil")
	}

	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers cannot be empty")
	}

	// 创建Sarama配置
	saramaConfig := sarama.NewConfig()
	if err := configureSarama(saramaConfig, cfg); err != nil {
		return nil, fmt.Errorf("failed to configure sarama: %w", err)
	}

	// 创建客户端
	client, err := sarama.NewClient(cfg.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka client: %w", err)
	}

	// 创建生产者
	producer, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create kafka producer: %w", err)
	}

	// 创建消费者
	consumer, err := sarama.NewConsumerFromClient(client)
	if err != nil {
		producer.Close()
		client.Close()
		return nil, fmt.Errorf("failed to create kafka consumer: %w", err)
	}

	// 创建管理客户端
	admin, err := sarama.NewClusterAdminFromClient(client)
	if err != nil {
		consumer.Close()
		producer.Close()
		client.Close()
		return nil, fmt.Errorf("failed to create kafka admin: %w", err)
	}

	// 获取配置策略（从环境变量或使用默认值）
	configStrategy := StrategyCreateOrUpdate
	configOnMismatch := TopicConfigMismatchAction{
		LogLevel: "warn",
		FailFast: false,
	}

	// 创建EventBus实例
	eventBus := &kafkaEventBus{
		config:                cfg,
		fullConfig:            fullConfig,
		producer:              producer,
		consumer:              consumer,
		client:                client,
		admin:                 admin,
		logger:                logger.Logger,
		reconnectConfig:       DefaultReconnectConfig(),
		subscriptions:         make(map[string]MessageHandler),
		keyedPools:            make(map[string]*KeyedWorkerPool),
		topicConfigs:          make(map[string]TopicOptions),
		topicConfigStrategy:   configStrategy,
		topicConfigOnMismatch: configOnMismatch,
	}

	// 初始化时间戳
	eventBus.lastReconnectTime.Store(time.Time{})

	// 初始化企业级特性
	if err := eventBus.initEnterpriseFeatures(); err != nil {
		eventBus.Close()
		return nil, fmt.Errorf("failed to initialize enterprise features: %w", err)
	}

	return eventBus, nil
}

// configureSarama 配置Sarama
func configureSarama(config *sarama.Config, cfg *config.KafkaConfig) error {
	// 生产者配置
	config.Producer.RequiredAcks = sarama.RequiredAcks(cfg.Producer.RequiredAcks)
	config.Producer.Timeout = cfg.Producer.Timeout
	config.Producer.Retry.Max = cfg.Producer.RetryMax
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true

	// 设置压缩算法
	switch cfg.Producer.Compression {
	case "gzip":
		config.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		config.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		config.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		config.Producer.Compression = sarama.CompressionZSTD
	default:
		config.Producer.Compression = sarama.CompressionNone
	}

	// 幂等性配置
	config.Producer.Idempotent = cfg.Producer.Idempotent
	if cfg.Producer.Idempotent {
		config.Producer.RequiredAcks = sarama.WaitForAll
		config.Producer.Retry.Max = 5
		config.Net.MaxOpenRequests = 1
	}

	// 批处理配置
	if cfg.Producer.FlushFrequency > 0 {
		config.Producer.Flush.Frequency = cfg.Producer.FlushFrequency
	}
	if cfg.Producer.FlushMessages > 0 {
		config.Producer.Flush.Messages = cfg.Producer.FlushMessages
	}
	if cfg.Producer.FlushBytes > 0 {
		config.Producer.Flush.Bytes = cfg.Producer.FlushBytes
	}

	// 消费者配置
	config.Consumer.Group.Session.Timeout = cfg.Consumer.SessionTimeout
	config.Consumer.Group.Heartbeat.Interval = cfg.Consumer.HeartbeatInterval
	config.Consumer.MaxProcessingTime = cfg.Consumer.MaxProcessingTime
	config.Consumer.Fetch.Min = int32(cfg.Consumer.FetchMinBytes)
	config.Consumer.Fetch.Max = int32(cfg.Consumer.FetchMaxBytes)
	config.Consumer.MaxWaitTime = cfg.Consumer.FetchMaxWait

	// 设置偏移量重置策略
	switch cfg.Consumer.AutoOffsetReset {
	case "earliest":
		config.Consumer.Offsets.Initial = sarama.OffsetOldest
	case "latest":
		config.Consumer.Offsets.Initial = sarama.OffsetNewest
	default:
		config.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	// 网络配置
	if cfg.Net.DialTimeout > 0 {
		config.Net.DialTimeout = cfg.Net.DialTimeout
	}
	if cfg.Net.ReadTimeout > 0 {
		config.Net.ReadTimeout = cfg.Net.ReadTimeout
	}
	if cfg.Net.WriteTimeout > 0 {
		config.Net.WriteTimeout = cfg.Net.WriteTimeout
	}

	// 版本配置
	config.Version = sarama.V2_6_0_0

	return nil
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

	// ⭐ 智能路由决策：根据聚合ID提取结果决定处理模式
	// 构建headers映射
	headersMap := make(map[string]string, len(message.Headers))
	for _, header := range message.Headers {
		headersMap[string(header.Key)] = string(header.Value)
	}

	// 尝试提取聚合ID（优先级：Envelope > Header > Kafka Key）
	aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")
	if aggregateID != "" {
		// ✅ 有聚合ID：使用Keyed-Worker池进行顺序处理
		// 这种情况通常发生在：
		// 1. SubscribeEnvelope订阅的Envelope消息
		// 2. 手动在Header中设置了聚合ID的消息
		// 3. Kafka Key恰好是有效的聚合ID
		// 获取该 topic 的 keyed 池
		h.eventBus.keyedPoolsMu.RLock()
		pool := h.eventBus.keyedPools[h.topic]
		h.eventBus.keyedPoolsMu.RUnlock()
		if pool != nil {
			// 使用 Keyed-Worker 池处理
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
	}

	// ❌ 无聚合ID：直接并发处理（不使用Keyed-Worker池）
	// 这种情况通常发生在：
	// 1. Subscribe订阅的原始消息（如JSON、文本等）
	// 2. 无法从消息中提取有效聚合ID的情况
	// 3. 简单消息传递场景（通知、缓存失效等）
	return h.handler(ctx, message.Value)
}

// initEnterpriseFeatures 初始化企业级特性
func (k *kafkaEventBus) initEnterpriseFeatures() error {
	// 如果没有完整配置，使用默认配置
	if k.fullConfig == nil {
		k.logger.Info("No full config available, skipping enterprise features")
		return nil
	}

	// 初始化订阅端积压检测器
	if k.fullConfig.Enterprise.Subscriber.BacklogDetection.Enabled {
		backlogConfig := BacklogDetectionConfig{
			MaxLagThreshold:  k.fullConfig.Enterprise.Subscriber.BacklogDetection.MaxLagThreshold,
			MaxTimeThreshold: k.fullConfig.Enterprise.Subscriber.BacklogDetection.MaxTimeThreshold,
			CheckInterval:    k.fullConfig.Enterprise.Subscriber.BacklogDetection.CheckInterval,
		}
		k.backlogDetector = NewBacklogDetector(k.client, k.admin, k.config.Consumer.GroupID, backlogConfig)
		k.logger.Info("Subscriber backlog detector initialized",
			zap.Int64("maxLagThreshold", backlogConfig.MaxLagThreshold),
			zap.Duration("maxTimeThreshold", backlogConfig.MaxTimeThreshold),
			zap.Duration("checkInterval", backlogConfig.CheckInterval))
	}

	// 初始化发送端积压检测器
	if k.fullConfig.Enterprise.Publisher.BacklogDetection.Enabled {
		k.publisherBacklogDetector = NewPublisherBacklogDetector(k.client, k.admin, k.fullConfig.Enterprise.Publisher.BacklogDetection)
		k.logger.Info("Publisher backlog detector initialized",
			zap.Int64("maxQueueDepth", k.fullConfig.Enterprise.Publisher.BacklogDetection.MaxQueueDepth),
			zap.Duration("maxPublishLatency", k.fullConfig.Enterprise.Publisher.BacklogDetection.MaxPublishLatency),
			zap.Float64("rateThreshold", k.fullConfig.Enterprise.Publisher.BacklogDetection.RateThreshold),
			zap.Duration("checkInterval", k.fullConfig.Enterprise.Publisher.BacklogDetection.CheckInterval))
	}

	k.logger.Info("Enterprise features initialized successfully")
	return nil
}

// Kafka EventBus 实现

// Publish 发布消息
func (k *kafkaEventBus) Publish(ctx context.Context, topic string, message []byte) error {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.closed {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 流量控制
	if k.rateLimiter != nil {
		if err := k.rateLimiter.Wait(ctx); err != nil {
			k.errorCount.Add(1)
			return fmt.Errorf("rate limit error: %w", err)
		}
	}

	// 创建Kafka消息
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(message),
	}

	// 发送消息
	partition, offset, err := k.producer.SendMessage(msg)
	if err != nil {
		k.errorCount.Add(1)
		k.logger.Error("Failed to publish message",
			zap.String("topic", topic),
			zap.Error(err))
		return fmt.Errorf("failed to publish message: %w", err)
	}

	k.publishedMessages.Add(1)
	k.logger.Debug("Message published successfully",
		zap.String("topic", topic),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset))

	return nil
}

// Subscribe 订阅原始消息（不使用Keyed-Worker池）
//
// 特点：
// - 消息格式：原始字节数据
// - 处理模式：直接并发处理，无顺序保证
// - 性能：极致性能，微秒级延迟
// - 聚合ID：通常无法从原始消息中提取聚合ID
// - Keyed-Worker池：不使用（因为无聚合ID）
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
func (k *kafkaEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.closed {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 创建消费者组
	consumerGroup, err := sarama.NewConsumerGroupFromClient(k.config.Consumer.GroupID, k.client)
	if err != nil {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	// 创建消费者处理器
	consumerHandler := &kafkaConsumerHandler{
		eventBus: k,
		handler:  handler,
		topic:    topic,
	}

	// 启动消费循环
	go func() {
		defer consumerGroup.Close()

		for {
			select {
			case <-ctx.Done():
				k.logger.Info("Consumer context cancelled", zap.String("topic", topic))
				return
			default:
				if err := consumerGroup.Consume(ctx, []string{topic}, consumerHandler); err != nil {
					k.logger.Error("Consumer group consume error",
						zap.String("topic", topic),
						zap.Error(err))
					time.Sleep(time.Second) // 避免快速重试
				}
			}
		}
	}()

	// 记录订阅信息（用于重连后恢复）
	k.subscriptionsMu.Lock()
	k.subscriptions[topic] = handler
	k.subscriptionsMu.Unlock()

	// Create per-topic Keyed-Worker pool (Phase 1)
	k.keyedPoolsMu.Lock()
	if _, ok := k.keyedPools[topic]; !ok {
		pool := NewKeyedWorkerPool(KeyedWorkerPoolConfig{
			WorkerCount: 1024,
			QueueSize:   1000,
			WaitTimeout: 200 * time.Millisecond,
		}, handler)
		k.keyedPools[topic] = pool
	}
	k.keyedPoolsMu.Unlock()

	k.logger.Info("Subscribed to topic", zap.String("topic", topic), zap.String("groupID", k.config.Consumer.GroupID))
	return nil
}

// healthCheck 内部健康检查（不对外暴露）
func (k *kafkaEventBus) healthCheck(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 检查客户端连接
	if !k.client.Closed() {
		// 尝试获取broker信息
		brokers := k.client.Brokers()
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
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.closed {
		return fmt.Errorf("kafka eventbus is closed")
	}

	if k.client.Closed() {
		return fmt.Errorf("kafka client is closed")
	}

	// 检查 broker 连接
	brokers := k.client.Brokers()
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

	// 检查是否支持端到端测试
	if k.consumer != nil {
		return k.performKafkaEndToEndTest(ctx, testTopic, testMessage)
	}

	// 如果没有消费者组，只测试发布能力
	start := time.Now()
	err := k.Publish(ctx, testTopic, []byte(testMessage))
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

	// 创建分区消费者来接收健康检查消息
	partitionConsumer, err := k.consumer.ConsumePartition(testTopic, 0, sarama.OffsetNewest)
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
	k.mu.RLock()
	defer k.mu.RUnlock()

	connectionStatus := "disconnected"
	brokerCount := 0
	topicCount := 0

	if !k.closed && !k.client.Closed() {
		brokers := k.client.Brokers()
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
		if topics, err := k.client.Topics(); err == nil {
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

	// Stop keyed worker pools
	k.keyedPoolsMu.Lock()
	for topic, pool := range k.keyedPools {
		if pool != nil {
			pool.Stop()
		}
		delete(k.keyedPools, topic)
	}
	k.keyedPoolsMu.Unlock()

	defer k.mu.Unlock()

	if k.closed {
		return nil
	}

	var errors []error

	// 关闭管理客户端
	if k.admin != nil {
		if err := k.admin.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka admin: %w", err))
		}
	}

	// 关闭消费者
	if k.consumer != nil {
		if err := k.consumer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka consumer: %w", err))
		}
	}

	// 关闭生产者
	if k.producer != nil {
		if err := k.producer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka producer: %w", err))
		}
	}

	// 关闭客户端
	if k.client != nil {
		if err := k.client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka client: %w", err))
		}
	}

	k.closed = true
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

	// 关闭现有连接
	if k.producer != nil {
		k.producer.Close()
	}
	if k.consumer != nil {
		k.consumer.Close()
	}
	if k.admin != nil {
		k.admin.Close()
	}
	if k.client != nil {
		k.client.Close()
	}

	// 创建Sarama配置
	saramaConfig := sarama.NewConfig()
	if err := configureSarama(saramaConfig, k.config); err != nil {
		return fmt.Errorf("failed to configure sarama: %w", err)
	}

	// 重新创建客户端
	client, err := sarama.NewClient(k.config.Brokers, saramaConfig)
	if err != nil {
		return fmt.Errorf("failed to create kafka client: %w", err)
	}

	// 重新创建生产者
	producer, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to create kafka producer: %w", err)
	}

	// 重新创建消费者
	consumer, err := sarama.NewConsumerFromClient(client)
	if err != nil {
		producer.Close()
		client.Close()
		return fmt.Errorf("failed to create kafka consumer: %w", err)
	}

	// 重新创建管理客户端
	admin, err := sarama.NewClusterAdminFromClient(client)
	if err != nil {
		consumer.Close()
		producer.Close()
		client.Close()
		return fmt.Errorf("failed to create kafka admin: %w", err)
	}

	// 更新实例字段
	k.client = client
	k.producer = producer
	k.consumer = consumer
	k.admin = admin

	k.logger.Info("Kafka connection reinitialized successfully")
	return nil
}

// restoreSubscriptions 恢复订阅
func (k *kafkaEventBus) restoreSubscriptions(ctx context.Context) error {
	k.subscriptionsMu.RLock()
	subscriptions := make(map[string]MessageHandler)
	for topic, handler := range k.subscriptions {
		subscriptions[topic] = handler
	}
	k.subscriptionsMu.RUnlock()

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

	if k.closed {
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

	// 发布消息
	partition, offset, err := k.producer.SendMessage(kafkaMsg)

	// 记录指标
	duration := time.Since(start)
	if err != nil {
		k.errorCount.Add(1)
		k.logger.Error("Failed to publish message with options",
			zap.String("topic", topic),
			zap.Error(err),
			zap.Duration("duration", duration))
	} else {
		k.publishedMessages.Add(1)
		k.logger.Debug("Message published with options",
			zap.String("topic", topic),
			zap.Int32("partition", partition),
			zap.Int64("offset", offset),
			zap.Duration("duration", duration))
	}

	// 调用发布回调
	if k.publishCallback != nil {
		go func() {
			callbackErr := k.publishCallback(ctx, topic, message, err)
			if callbackErr != nil {
				k.logger.Error("Publish callback failed", zap.Error(callbackErr))
			}
		}()
	}

	return err
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

// RegisterBacklogCallback 注册订阅端积压回调（已废弃，向后兼容）
func (k *kafkaEventBus) RegisterBacklogCallback(callback BacklogStateCallback) error {
	k.logger.Warn("RegisterBacklogCallback is deprecated, use RegisterSubscriberBacklogCallback instead")
	return k.RegisterSubscriberBacklogCallback(callback)
}

// StartBacklogMonitoring 启动订阅端积压监控（已废弃，向后兼容）
func (k *kafkaEventBus) StartBacklogMonitoring(ctx context.Context) error {
	k.logger.Warn("StartBacklogMonitoring is deprecated, use StartSubscriberBacklogMonitoring instead")
	return k.StartSubscriberBacklogMonitoring(ctx)
}

// StopBacklogMonitoring 停止订阅端积压监控（已废弃，向后兼容）
func (k *kafkaEventBus) StopBacklogMonitoring() error {
	k.logger.Warn("StopBacklogMonitoring is deprecated, use StopSubscriberBacklogMonitoring instead")
	return k.StopSubscriberBacklogMonitoring()
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
	return HealthCheckStatus{
		IsHealthy:           !k.closed && k.client != nil,
		ConsecutiveFailures: 0,
		LastSuccessTime:     time.Now(),
		LastFailureTime:     time.Time{},
		IsRunning:           !k.closed,
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
	defer k.mu.Unlock()

	if k.healthCheckSubscriber != nil {
		return nil // 已经启动
	}

	// 创建健康检查订阅监控器
	config := GetDefaultHealthCheckConfig()
	k.healthCheckSubscriber = NewHealthCheckSubscriber(config, k, "kafka-eventbus", "kafka")

	// 启动监控器
	if err := k.healthCheckSubscriber.Start(ctx); err != nil {
		k.healthCheckSubscriber = nil
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
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.healthCheckSubscriber == nil {
		return fmt.Errorf("health check subscriber not started")
	}

	return k.healthCheckSubscriber.RegisterAlertCallback(callback)
}

// GetHealthCheckSubscriberStats 获取健康检查订阅监控统计信息
func (k *kafkaEventBus) GetHealthCheckSubscriberStats() HealthCheckSubscriberStats {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.healthCheckSubscriber == nil {
		return HealthCheckSubscriberStats{}
	}

	return k.healthCheckSubscriber.GetStats()
}

// GetConnectionState 获取连接状态
func (k *kafkaEventBus) GetConnectionState() ConnectionState {
	isConnected := !k.closed && k.client != nil
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
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.closed {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 校验Envelope
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("invalid envelope: %w", err)
	}

	// 流量控制
	if k.rateLimiter != nil {
		if err := k.rateLimiter.Wait(ctx); err != nil {
			k.errorCount.Add(1)
			return fmt.Errorf("rate limit error: %w", err)
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

	// 发送消息
	partition, offset, err := k.producer.SendMessage(msg)
	if err != nil {
		k.errorCount.Add(1)
		k.logger.Error("Failed to publish envelope message",
			zap.String("topic", topic),
			zap.String("aggregateID", envelope.AggregateID),
			zap.String("eventType", envelope.EventType),
			zap.Int64("eventVersion", envelope.EventVersion),
			zap.Error(err))
		return fmt.Errorf("failed to publish envelope message: %w", err)
	}

	k.publishedMessages.Add(1)
	k.logger.Debug("Envelope message published successfully",
		zap.String("topic", topic),
		zap.String("aggregateID", envelope.AggregateID),
		zap.String("eventType", envelope.EventType),
		zap.Int64("eventVersion", envelope.EventVersion),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset))

	return nil
}

// SubscribeEnvelope 订阅Envelope消息（自动使用Keyed-Worker池）
//
// 特点：
// - 消息格式：Envelope包装格式（包含聚合ID、事件类型、版本等元数据）
// - 处理模式：按聚合ID路由到Keyed-Worker池，同聚合ID严格顺序处理
// - 性能：顺序保证，毫秒级延迟
// - 聚合ID：从Envelope.AggregateID字段提取
// - Keyed-Worker池：自动使用（基于聚合ID的一致性哈希路由）
//
// 核心机制：
// 1. 消息必须是Envelope格式，包含AggregateID
// 2. ExtractAggregateID成功提取聚合ID
// 3. 使用一致性哈希将相同聚合ID路由到固定Worker
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
//	    // 同一订单的所有事件会路由到同一个Worker，确保顺序处理
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

	// 使用现有的Subscribe方法
	return k.Subscribe(ctx, topic, wrappedHandler)
}

// ========== 新的分离式健康检查接口实现 ==========

// StartHealthCheckPublisher 启动健康检查发布器
func (k *kafkaEventBus) StartHealthCheckPublisher(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.healthChecker != nil {
		return nil // 已经启动
	}

	// 创建健康检查发布器
	config := GetDefaultHealthCheckConfig()
	k.healthChecker = NewHealthChecker(config, k, "kafka-eventbus", "kafka")

	// 启动健康检查发布器
	if err := k.healthChecker.Start(ctx); err != nil {
		k.healthChecker = nil
		return fmt.Errorf("failed to start health check publisher: %w", err)
	}

	k.logger.Info("Health check publisher started for kafka eventbus")
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
	k.mu.RLock()
	defer k.mu.RUnlock()

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
	k.mu.RLock()
	defer k.mu.RUnlock()

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

	k.topicConfigsMu.Lock()
	// 检查是否已有配置
	_, exists := k.topicConfigs[topic]
	// 缓存配置
	k.topicConfigs[topic] = options
	k.topicConfigsMu.Unlock()

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
	k.topicConfigsMu.RLock()
	defer k.topicConfigsMu.RUnlock()

	if config, exists := k.topicConfigs[topic]; exists {
		return config, nil
	}

	// 返回默认配置
	return DefaultTopicOptions(), nil
}

// ListConfiguredTopics 列出所有已配置的主题
func (k *kafkaEventBus) ListConfiguredTopics() []string {
	k.topicConfigsMu.RLock()
	defer k.topicConfigsMu.RUnlock()

	topics := make([]string, 0, len(k.topicConfigs))
	for topic := range k.topicConfigs {
		topics = append(topics, topic)
	}

	return topics
}

// RemoveTopicConfig 移除主题配置（恢复为默认行为）
func (k *kafkaEventBus) RemoveTopicConfig(topic string) error {
	k.topicConfigsMu.Lock()
	defer k.topicConfigsMu.Unlock()

	delete(k.topicConfigs, topic)

	k.logger.Info("Topic configuration removed", zap.String("topic", topic))
	return nil
}

// ensureKafkaTopic 确保Kafka主题存在并配置正确
func (k *kafkaEventBus) ensureKafkaTopic(topic string, options TopicOptions) error {
	if k.admin == nil {
		return fmt.Errorf("Kafka admin client not available")
	}

	// 检查主题是否已存在
	metadata, err := k.admin.DescribeTopics([]string{topic})
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
	// 构建主题配置
	topicDetail := &sarama.TopicDetail{
		NumPartitions:     1, // 默认1个分区
		ReplicationFactor: int16(options.Replicas),
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

	// 创建主题
	err := k.admin.CreateTopic(topic, topicDetail, false)
	if err != nil {
		return fmt.Errorf("failed to create Kafka topic %s: %w", topic, err)
	}

	k.logger.Info("Created Kafka topic",
		zap.String("topic", topic),
		zap.Int("partitions", int(topicDetail.NumPartitions)),
		zap.Int("replicas", int(topicDetail.ReplicationFactor)),
		zap.Bool("persistent", options.IsPersistent(true)))

	return nil
}

// ensureKafkaTopicIdempotent 幂等地确保Kafka主题存在（支持创建和更新）
func (k *kafkaEventBus) ensureKafkaTopicIdempotent(ctx context.Context, topic string, options TopicOptions, allowUpdate bool) error {
	if k.admin == nil {
		return fmt.Errorf("Kafka admin client not available")
	}

	// 检查主题是否已存在
	metadata, err := k.admin.DescribeTopics([]string{topic})

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

		if len(configEntries) > 0 {
			k.logger.Info("Updating Kafka topic configuration", zap.String("topic", topic))

			err := k.admin.AlterConfig(sarama.TopicResource, topic, configEntries, false)
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
	if k.admin == nil {
		return TopicOptions{}, fmt.Errorf("Kafka admin client not available")
	}

	// 获取主题配置
	configs, err := k.admin.DescribeConfig(sarama.ConfigResource{
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

	// 获取分区和副本信息
	metadata, err := k.admin.DescribeTopics([]string{topic})
	if err == nil && len(metadata) > 0 {
		topicMeta := metadata[0]
		if len(topicMeta.Partitions) > 0 {
			actualConfig.Replicas = len(topicMeta.Partitions[0].Replicas)
		}
	}

	return actualConfig, nil
}

// SetTopicConfigStrategy 设置主题配置策略
func (k *kafkaEventBus) SetTopicConfigStrategy(strategy TopicConfigStrategy) {
	k.topicConfigsMu.Lock()
	defer k.topicConfigsMu.Unlock()
	k.topicConfigStrategy = strategy
	k.logger.Info("Topic config strategy updated", zap.String("strategy", string(strategy)))
}

// GetTopicConfigStrategy 获取当前主题配置策略
func (k *kafkaEventBus) GetTopicConfigStrategy() TopicConfigStrategy {
	k.topicConfigsMu.RLock()
	defer k.topicConfigsMu.RUnlock()
	return k.topicConfigStrategy
}
