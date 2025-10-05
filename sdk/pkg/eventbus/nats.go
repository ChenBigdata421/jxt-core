package eventbus

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// natsEventBus NATS JetStream事件总线实现
// 企业级增强版本，专注于JetStream持久化消息
// 支持方案A（Envelope）消息包络
type natsEventBus struct {
	conn               *nats.Conn
	js                 nats.JetStreamContext
	config             *config.NATSConfig
	subscriptions      map[string]*nats.Subscription
	consumers          map[string]nats.ConsumerInfo
	logger             *zap.Logger
	mu                 sync.RWMutex
	closed             bool
	reconnectCallbacks []func(ctx context.Context) error

	// 企业级特性
	publishedMessages atomic.Int64
	consumedMessages  atomic.Int64
	errorCount        atomic.Int64
	lastHealthCheck   atomic.Value // time.Time
	healthStatus      atomic.Bool

	// 增强的企业级特性
	metricsCollector *time.Ticker
	metrics          *Metrics
	messageFormatter MessageFormatter
	publishCallback  PublishCallback
	errorHandler     ErrorHandler
	messageRouter    MessageRouter

	// 健康检查控制
	healthCheckCancel context.CancelFunc
	healthCheckDone   chan struct{}

	// 自动重连控制
	reconnectConfig   ReconnectConfig
	failureCount      atomic.Int32
	lastReconnectTime atomic.Value // time.Time
	reconnectCallback ReconnectCallback

	// 订阅管理（用于重连后恢复订阅）
	subscriptionHandlers map[string]MessageHandler // topic -> handler
	subscriptionsMu      sync.RWMutex

	// 积压检测器
	backlogDetector          *NATSBacklogDetector      // 订阅端积压检测器
	publisherBacklogDetector *PublisherBacklogDetector // 发送端积压检测器

	// 完整配置（用于访问 Publisher/Subscriber 配置）
	fullConfig *EventBusConfig

	// Keyed-Worker池管理（与Kafka保持一致）
	keyedPools   map[string]*KeyedWorkerPool // topic -> pool
	keyedPoolsMu sync.RWMutex

	// 主题配置管理
	topicConfigs          map[string]TopicOptions
	topicConfigsMu        sync.RWMutex
	topicConfigStrategy   TopicConfigStrategy       // 配置策略
	topicConfigOnMismatch TopicConfigMismatchAction // 配置不一致时的行为

	// 健康检查订阅监控器
	healthCheckSubscriber *HealthCheckSubscriber
	// 健康检查发布器
	healthChecker *HealthChecker
}

// NewNATSEventBus 创建NATS JetStream事件总线
func NewNATSEventBus(config *config.NATSConfig) (EventBus, error) {
	return NewNATSEventBusWithFullConfig(config, nil)
}

// NewNATSEventBusWithFullConfig 创建NATS JetStream事件总线（带完整配置）
func NewNATSEventBusWithFullConfig(config *config.NATSConfig, fullConfig *EventBusConfig) (EventBus, error) {
	// 构建连接选项
	opts := buildNATSOptions(config)

	// 连接到NATS服务器
	var nc *nats.Conn
	var err error

	if len(config.URLs) > 0 {
		nc, err = nats.Connect(config.URLs[0], opts...)
	} else {
		nc, err = nats.Connect(nats.DefaultURL, opts...)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// 创建JetStream上下文（如果启用）
	var js nats.JetStreamContext
	if config.JetStream.Enabled {
		jsOpts := buildJetStreamOptions(config)
		js, err = nc.JetStream(jsOpts...)
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to create JetStream context: %w", err)
		}

		// 确保流存在
		if err := ensureStream(js, config); err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to ensure stream: %w", err)
		}
	}

	// 初始化指标收集器（简化版本）

	// 获取配置策略（从环境变量或使用默认值）
	configStrategy := StrategyCreateOrUpdate
	configOnMismatch := TopicConfigMismatchAction{
		LogLevel: "warn",
		FailFast: false,
	}

	eventBus := &natsEventBus{
		conn:                  nc,
		js:                    js,
		config:                config,
		fullConfig:            fullConfig,
		subscriptions:         make(map[string]*nats.Subscription),
		consumers:             make(map[string]nats.ConsumerInfo),
		logger:                logger.Logger,
		metricsCollector:      time.NewTicker(DefaultMetricsCollectInterval),
		reconnectConfig:       DefaultReconnectConfig(),
		subscriptionHandlers:  make(map[string]MessageHandler),
		keyedPools:            make(map[string]*KeyedWorkerPool), // 初始化Keyed-Worker池映射
		topicConfigs:          make(map[string]TopicOptions),     // 初始化主题配置映射
		topicConfigStrategy:   configStrategy,                    // 设置配置策略
		topicConfigOnMismatch: configOnMismatch,                  // 设置不一致处理行为
		metrics: &Metrics{
			LastHealthCheck:   time.Now(),
			HealthCheckStatus: "healthy",
		},
	}

	// 设置重连处理器来执行重连回调
	nc.SetReconnectHandler(func(nc *nats.Conn) {
		eventBus.logger.Info("NATS reconnected", zap.String("url", nc.ConnectedUrl()))
		// 重置失败计数
		eventBus.failureCount.Store(0)
		// 更新重连时间
		eventBus.lastReconnectTime.Store(time.Now())
		// 恢复订阅
		eventBus.restoreSubscriptions(context.Background())
		// 执行重连回调
		eventBus.executeReconnectCallbacks()
	})

	// 初始化健康状态
	eventBus.lastHealthCheck.Store(time.Now())
	eventBus.healthStatus.Store(true)

	// 启动指标收集协程
	go eventBus.collectMetrics()

	// 根据配置初始化积压检测器
	if fullConfig != nil {
		// 初始化订阅端积压检测器
		if fullConfig.Enterprise.Subscriber.BacklogDetection.Enabled {
			backlogConfig := BacklogDetectionConfig{
				MaxLagThreshold:  fullConfig.Enterprise.Subscriber.BacklogDetection.MaxLagThreshold,
				MaxTimeThreshold: fullConfig.Enterprise.Subscriber.BacklogDetection.MaxTimeThreshold,
				CheckInterval:    fullConfig.Enterprise.Subscriber.BacklogDetection.CheckInterval,
			}
			eventBus.backlogDetector = NewNATSBacklogDetector(js, nc, config.JetStream.Stream.Name, backlogConfig)
			logger.Logger.Info("NATS JetStream subscriber backlog detector initialized",
				zap.String("stream", config.JetStream.Stream.Name),
				zap.Int64("maxLagThreshold", backlogConfig.MaxLagThreshold),
				zap.Duration("maxTimeThreshold", backlogConfig.MaxTimeThreshold),
				zap.Duration("checkInterval", backlogConfig.CheckInterval))
		}

		// 初始化发送端积压检测器
		if fullConfig.Enterprise.Publisher.BacklogDetection.Enabled {
			// NATS 发送端积压检测器需要特殊的实现，这里暂时使用通用的
			// 注意：NATS 的发送端积压检测可能需要不同的实现方式
			eventBus.publisherBacklogDetector = NewPublisherBacklogDetector(nil, nil, fullConfig.Enterprise.Publisher.BacklogDetection)
			logger.Logger.Info("NATS JetStream publisher backlog detector initialized",
				zap.Int64("maxQueueDepth", fullConfig.Enterprise.Publisher.BacklogDetection.MaxQueueDepth),
				zap.Duration("maxPublishLatency", fullConfig.Enterprise.Publisher.BacklogDetection.MaxPublishLatency),
				zap.Float64("rateThreshold", fullConfig.Enterprise.Publisher.BacklogDetection.RateThreshold),
				zap.Duration("checkInterval", fullConfig.Enterprise.Publisher.BacklogDetection.CheckInterval))
		}
	} else {
		// 如果没有完整配置，使用默认的订阅端积压检测器（向后兼容）
		backlogConfig := BacklogDetectionConfig{
			MaxLagThreshold:  1000,             // 默认最大延迟阈值
			MaxTimeThreshold: 5 * time.Minute,  // 默认最大时间阈值
			CheckInterval:    30 * time.Second, // 默认检查间隔
		}
		eventBus.backlogDetector = NewNATSBacklogDetector(js, nc, config.JetStream.Stream.Name, backlogConfig)
		logger.Logger.Info("NATS JetStream backlog detector initialized (default config)",
			zap.String("stream", config.JetStream.Stream.Name))
	}

	logger.Logger.Info("NATS JetStream EventBus initialized successfully",
		zap.String("client_id", config.ClientID),
		zap.Bool("jetstream_enabled", config.JetStream.Enabled))

	return eventBus, nil
}

// buildNATSOptions 构建NATS连接选项
func buildNATSOptions(config *config.NATSConfig) []nats.Option {
	opts := []nats.Option{
		nats.MaxReconnects(config.MaxReconnects),
		nats.ReconnectWait(config.ReconnectWait),
		nats.Timeout(config.ConnectionTimeout),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Logger.Warn("NATS disconnected", zap.Error(err))
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logger.Logger.Info("NATS connection closed")
		}),
	}

	// 添加安全配置
	if config.Security.Enabled {
		if config.Security.Token != "" {
			opts = append(opts, nats.Token(config.Security.Token))
		}
		if config.Security.Username != "" && config.Security.Password != "" {
			opts = append(opts, nats.UserInfo(config.Security.Username, config.Security.Password))
		}
		if config.Security.NKeyFile != "" {
			opts = append(opts, nats.UserCredentials(config.Security.NKeyFile))
		}
		if config.Security.CredFile != "" {
			opts = append(opts, nats.UserCredentials(config.Security.CredFile))
		}
		if config.Security.CertFile != "" && config.Security.KeyFile != "" {
			opts = append(opts, nats.ClientCert(config.Security.CertFile, config.Security.KeyFile))
		}
		if config.Security.CAFile != "" {
			opts = append(opts, nats.RootCAs(config.Security.CAFile))
		}
		if config.Security.SkipVerify {
			opts = append(opts, nats.Secure())
		}
	}

	return opts
}

// buildJetStreamOptions 构建JetStream选项
func buildJetStreamOptions(config *config.NATSConfig) []nats.JSOpt {
	var opts []nats.JSOpt

	if config.JetStream.Domain != "" {
		opts = append(opts, nats.Domain(config.JetStream.Domain))
	}
	if config.JetStream.APIPrefix != "" {
		opts = append(opts, nats.APIPrefix(config.JetStream.APIPrefix))
	}
	if config.JetStream.PublishTimeout > 0 {
		opts = append(opts, nats.PublishAsyncMaxPending(256))
	}

	return opts
}

// ensureStream 确保流存在
func ensureStream(js nats.JetStreamContext, config *config.NATSConfig) error {
	streamConfig := &nats.StreamConfig{
		Name:      config.JetStream.Stream.Name,
		Subjects:  config.JetStream.Stream.Subjects,
		Retention: parseRetentionPolicy(config.JetStream.Stream.Retention),
		Storage:   parseStorageType(config.JetStream.Stream.Storage),
		Replicas:  config.JetStream.Stream.Replicas,
		MaxAge:    config.JetStream.Stream.MaxAge,
		MaxBytes:  config.JetStream.Stream.MaxBytes,
		MaxMsgs:   config.JetStream.Stream.MaxMsgs,
		Discard:   parseDiscardPolicy(config.JetStream.Stream.Discard),
	}

	// 尝试获取流信息
	_, err := js.StreamInfo(streamConfig.Name)
	if err != nil {
		// 流不存在，创建新流
		_, err = js.AddStream(streamConfig)
		if err != nil {
			return fmt.Errorf("failed to create stream %s: %w", streamConfig.Name, err)
		}
		logger.Logger.Info("Created JetStream stream", zap.String("name", streamConfig.Name))
	} else {
		// 流已存在，更新配置
		_, err = js.UpdateStream(streamConfig)
		if err != nil {
			return fmt.Errorf("failed to update stream %s: %w", streamConfig.Name, err)
		}
		logger.Logger.Info("Updated JetStream stream", zap.String("name", streamConfig.Name))
	}

	return nil
}

// parseRetentionPolicy 解析保留策略
func parseRetentionPolicy(policy string) nats.RetentionPolicy {
	switch policy {
	case "limits":
		return nats.LimitsPolicy
	case "interest":
		return nats.InterestPolicy
	case "workqueue":
		return nats.WorkQueuePolicy
	default:
		return nats.LimitsPolicy
	}
}

// parseStorageType 解析存储类型
func parseStorageType(storage string) nats.StorageType {
	switch storage {
	case "file":
		return nats.FileStorage
	case "memory":
		return nats.MemoryStorage
	default:
		return nats.FileStorage
	}
}

// parseDiscardPolicy 解析丢弃策略
func parseDiscardPolicy(policy string) nats.DiscardPolicy {
	switch policy {
	case "old":
		return nats.DiscardOld
	case "new":
		return nats.DiscardNew
	default:
		return nats.DiscardOld
	}
}

// parseDeliverPolicy 解析投递策略
func parseDeliverPolicy(policy string) nats.DeliverPolicy {
	switch policy {
	case "all":
		return nats.DeliverAllPolicy
	case "last":
		return nats.DeliverLastPolicy
	case "new":
		return nats.DeliverNewPolicy
	case "by_start_sequence":
		return nats.DeliverByStartSequencePolicy
	case "by_start_time":
		return nats.DeliverByStartTimePolicy
	default:
		return nats.DeliverAllPolicy
	}
}

// parseAckPolicy 解析确认策略
func parseAckPolicy(policy string) nats.AckPolicy {
	switch policy {
	case "none":
		return nats.AckNonePolicy
	case "all":
		return nats.AckAllPolicy
	case "explicit":
		return nats.AckExplicitPolicy
	default:
		return nats.AckExplicitPolicy
	}
}

// parseReplayPolicy 解析重放策略
func parseReplayPolicy(policy string) nats.ReplayPolicy {
	switch policy {
	case "instant":
		return nats.ReplayInstantPolicy
	case "original":
		return nats.ReplayOriginalPolicy
	default:
		return nats.ReplayInstantPolicy
	}
}

// Publish 发布消息到指定主题
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
	start := time.Now()

	n.mu.RLock()
	if n.closed {
		n.mu.RUnlock()
		return fmt.Errorf("eventbus is closed")
	}
	n.mu.RUnlock()

	// 获取主题配置
	topicConfig, _ := n.GetTopicConfig(topic)

	// 决定发布模式：优先使用主题配置，其次使用全局配置
	shouldUsePersistent := topicConfig.IsPersistent(n.config.JetStream.Enabled)

	var err error

	if shouldUsePersistent && n.js != nil {
		// 确保主题在JetStream中存在（如果需要持久化）
		if err := n.ensureTopicInJetStream(topic, topicConfig); err != nil {
			n.logger.Warn("Failed to ensure topic in JetStream, falling back to Core NATS",
				zap.String("topic", topic),
				zap.Error(err))
			// 降级到Core NATS
			shouldUsePersistent = false
		}
	}

	if shouldUsePersistent && n.js != nil {
		// 使用JetStream发布（持久化）
		var pubOpts []nats.PubOpt
		if n.config.JetStream.PublishTimeout > 0 {
			pubOpts = append(pubOpts, nats.AckWait(n.config.JetStream.PublishTimeout))
		}

		_, err = n.js.Publish(topic, message, pubOpts...)
		if err != nil {
			n.errorCount.Add(1)
			n.logger.Error("Failed to publish message to NATS JetStream",
				zap.String("topic", topic),
				zap.Bool("persistent", true),
				zap.String("persistenceMode", string(topicConfig.PersistenceMode)),
				zap.Error(err))
			return err
		}
	} else {
		// 使用Core NATS发布（非持久化）
		err = n.conn.Publish(topic, message)
		if err != nil {
			n.errorCount.Add(1)
			n.logger.Error("Failed to publish message to NATS Core",
				zap.String("topic", topic),
				zap.Bool("persistent", false),
				zap.String("persistenceMode", string(topicConfig.PersistenceMode)),
				zap.Error(err))
			return err
		}
	}

	// 记录指标
	duration := time.Since(start)

	// 记录发布指标
	if n.metrics != nil {
		n.metrics.LastHealthCheck = time.Now()
		if err != nil {
			n.metrics.PublishErrors++
		} else {
			n.metrics.MessagesPublished++
		}
	}

	if err == nil {
		n.publishedMessages.Add(1)
		n.logger.Debug("Message published to NATS",
			zap.String("topic", topic),
			zap.Int("message_size", len(message)),
			zap.Bool("persistent", n.config.JetStream.Enabled),
			zap.String("mode", func() string {
				if n.config.JetStream.Enabled && n.js != nil {
					return "JetStream"
				}
				return "Core"
			}()),
			zap.Duration("duration", duration))
		return nil
	}

	return fmt.Errorf("failed to publish message: %w", err)
}

// Subscribe 订阅原始消息（不使用Keyed-Worker池）
//
// 特点：
// - 消息格式：原始字节数据
// - 处理模式：直接并发处理，无顺序保证
// - 性能：极致性能，微秒级延迟（NATS Core: 7.86µs - 136µs）
// - 聚合ID：通常无法从原始消息中提取聚合ID
// - Keyed-Worker池：不使用（因为无聚合ID）
//
// 适用场景：
// - 简单消息传递（通知、提醒）
// - 缓存失效消息
// - 系统监控指标
// - 跨Docker容器通信
// - 不需要顺序保证的业务场景
//
// 示例：
//
//	bus.Subscribe(ctx, "notifications", func(ctx context.Context, data []byte) error {
//	    var notification Notification
//	    json.Unmarshal(data, &notification)
//	    return processNotification(notification) // 直接并发处理
//	})
func (n *natsEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("eventbus is closed")
	}

	// 检查是否已经订阅了该主题
	if _, exists := n.subscriptions[topic]; exists {
		return fmt.Errorf("already subscribed to topic: %s", topic)
	}

	// 保存订阅处理器（用于重连后恢复）
	n.subscriptionsMu.Lock()
	n.subscriptionHandlers[topic] = handler
	n.subscriptionsMu.Unlock()

	// 根据配置选择订阅模式
	var err error

	if n.config.JetStream.Enabled && n.js != nil {
		// 使用JetStream订阅（持久化）
		err = n.subscribeJetStream(ctx, topic, handler)
	} else {
		// 使用Core NATS订阅（非持久化）
		msgHandler := func(msg *nats.Msg) {
			n.handleMessage(ctx, topic, msg.Data, handler, func() error {
				return nil // Core NATS不需要手动确认
			})
		}

		sub, err := n.conn.Subscribe(topic, msgHandler)
		if err != nil {
			return fmt.Errorf("failed to subscribe to topic %s with Core NATS: %w", topic, err)
		}

		n.subscriptions[topic] = sub
	}

	if err != nil {
		return err
	}

	// ⭐ 创建per-topic Keyed-Worker池（与Kafka保持一致）
	n.keyedPoolsMu.Lock()
	if _, ok := n.keyedPools[topic]; !ok {
		pool := NewKeyedWorkerPool(KeyedWorkerPoolConfig{
			WorkerCount: 1024, // 与Kafka保持一致
			QueueSize:   1000, // 与Kafka保持一致
			WaitTimeout: 200 * time.Millisecond,
		}, handler)
		n.keyedPools[topic] = pool
	}
	n.keyedPoolsMu.Unlock()

	n.logger.Info("Subscribed to NATS topic",
		zap.String("topic", topic),
		zap.Bool("persistent", n.config.JetStream.Enabled),
		zap.String("mode", func() string {
			if n.config.JetStream.Enabled && n.js != nil {
				return "JetStream"
			}
			return "Core"
		}()))

	return nil
}

// subscribeJetStream 使用JetStream订阅
func (n *natsEventBus) subscribeJetStream(ctx context.Context, topic string, handler MessageHandler) error {
	// 构建消费者配置
	// 为每个主题生成唯一的消费者名称，避免冲突
	topicSafeName := strings.ReplaceAll(topic, ".", "_")
	durableName := fmt.Sprintf("%s-%s", n.config.JetStream.Consumer.DurableName, topicSafeName)

	consumerConfig := &nats.ConsumerConfig{
		Durable:       durableName,
		DeliverPolicy: parseDeliverPolicy(n.config.JetStream.Consumer.DeliverPolicy),
		AckPolicy:     parseAckPolicy(n.config.JetStream.Consumer.AckPolicy),
		ReplayPolicy:  parseReplayPolicy(n.config.JetStream.Consumer.ReplayPolicy),
		MaxAckPending: n.config.JetStream.Consumer.MaxAckPending,
		MaxWaiting:    n.config.JetStream.Consumer.MaxWaiting,
		MaxDeliver:    n.config.JetStream.Consumer.MaxDeliver,
		BackOff:       n.config.JetStream.Consumer.BackOff,
		FilterSubject: topic,
	}

	if n.config.JetStream.AckWait > 0 {
		consumerConfig.AckWait = n.config.JetStream.AckWait
	}

	// 创建或获取消费者
	consumer, err := n.js.AddConsumer(n.config.JetStream.Stream.Name, consumerConfig)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	n.consumers[topic] = *consumer

	// 创建订阅
	sub, err := n.js.PullSubscribe(topic, consumerConfig.Durable)
	if err != nil {
		return fmt.Errorf("failed to create pull subscription: %w", err)
	}

	n.subscriptions[topic] = sub

	// 注册消费者到积压检测器
	if n.backlogDetector != nil {
		consumerName := fmt.Sprintf("%s-%s", topic, consumerConfig.Durable)
		n.backlogDetector.RegisterConsumer(consumerName, consumerConfig.Durable)
		n.logger.Debug("Consumer registered to NATS backlog detector",
			zap.String("topic", topic),
			zap.String("consumer", consumerName),
			zap.String("durable", consumerConfig.Durable))
	}

	// 启动消息处理协程
	go n.processPullMessages(ctx, topic, sub, handler)

	return nil
}

// processPullMessages 处理拉取的消息
func (n *natsEventBus) processPullMessages(ctx context.Context, topic string, sub *nats.Subscription, handler MessageHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 拉取消息
			msgs, err := sub.Fetch(10, nats.MaxWait(time.Second))
			if err != nil {
				if err == nats.ErrTimeout {
					continue // 超时是正常的，继续拉取
				}
				n.logger.Error("Failed to fetch messages",
					zap.String("topic", topic),
					zap.Error(err))
				time.Sleep(time.Second)
				continue
			}

			// 处理消息
			for _, msg := range msgs {
				n.handleMessage(ctx, topic, msg.Data, handler, func() error {
					return msg.Ack()
				})
			}
		}
	}
}

// handleMessage 处理单个消息（支持方案A：Envelope优先级提取）
func (n *natsEventBus) handleMessage(ctx context.Context, topic string, data []byte, handler MessageHandler, ackFunc func() error) {
	defer func() {
		if r := recover(); r != nil {
			n.errorCount.Add(1)
			n.logger.Error("Panic in NATS message handler",
				zap.String("topic", topic),
				zap.Any("panic", r))
		}
	}()

	// 创建带超时的上下文
	handlerCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// ⭐ 智能路由决策：根据聚合ID提取结果决定处理模式
	// 优先级：Envelope > Header > NATS Subject
	aggregateID, _ := ExtractAggregateID(data, nil, nil, topic)

	if aggregateID != "" {
		// ✅ 有聚合ID：使用Keyed-Worker池进行顺序处理
		// 这种情况通常发生在：
		// 1. SubscribeEnvelope订阅的Envelope消息
		// 2. NATS Subject中包含有效聚合ID的情况
		// 获取该topic的Keyed-Worker池
		n.keyedPoolsMu.RLock()
		pool := n.keyedPools[topic]
		n.keyedPoolsMu.RUnlock()

		if pool != nil {
			// ⭐ 使用Keyed-Worker池处理（与Kafka保持一致）
			aggMsg := &AggregateMessage{
				Topic:       topic,
				Partition:   0, // NATS没有分区概念
				Offset:      0, // NATS没有偏移量概念
				Key:         []byte(aggregateID),
				Value:       data,
				Headers:     make(map[string][]byte),
				Timestamp:   time.Now(),
				AggregateID: aggregateID,
				Context:     handlerCtx,
				Done:        make(chan error, 1),
			}

			// 路由到Keyed-Worker池处理
			if err := pool.ProcessMessage(handlerCtx, aggMsg); err != nil {
				n.errorCount.Add(1)
				n.logger.Error("Failed to process message with Keyed-Worker pool",
					zap.String("topic", topic),
					zap.String("aggregateID", aggregateID),
					zap.Error(err))
				// 不确认消息，让它重新投递
				return
			}

			// 等待Worker处理完成
			if err := <-aggMsg.Done; err != nil {
				n.errorCount.Add(1)
				n.logger.Error("Failed to handle NATS message in Keyed-Worker",
					zap.String("topic", topic),
					zap.String("aggregateID", aggregateID),
					zap.Error(err))
				// 不确认消息，让它重新投递
				return
			}

			// Worker处理成功，确认消息
			if err := ackFunc(); err != nil {
				n.logger.Error("Failed to ack NATS message",
					zap.String("topic", topic),
					zap.String("aggregateID", aggregateID),
					zap.Error(err))
			} else {
				n.consumedMessages.Add(1)
			}
			return
		}
	}

	// ❌ 无聚合ID：直接并发处理（不使用Keyed-Worker池）
	// 这种情况通常发生在：
	// 1. Subscribe订阅的原始消息（如JSON、文本等）
	// 2. 无法从消息中提取有效聚合ID的情况
	// 3. 简单消息传递场景（通知、缓存失效等）
	if err := handler(handlerCtx, data); err != nil {
		n.errorCount.Add(1)
		n.logger.Error("Failed to handle NATS message",
			zap.String("topic", topic),
			zap.Error(err))
		// 不确认消息，让它重新投递
		return
	}

	// 确认消息
	if err := ackFunc(); err != nil {
		n.logger.Error("Failed to ack NATS message",
			zap.String("topic", topic),
			zap.Error(err))
	} else {
		n.consumedMessages.Add(1)
	}
}

// healthCheck 内部健康检查（不对外暴露）
func (n *natsEventBus) healthCheck(ctx context.Context) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.closed {
		n.healthStatus.Store(false)
		return fmt.Errorf("eventbus is closed")
	}

	// 检查NATS连接状态
	if !n.conn.IsConnected() {
		n.healthStatus.Store(false)
		return fmt.Errorf("NATS connection is not active")
	}

	// 检查JetStream连接状态（如果启用）
	if n.config.JetStream.Enabled && n.js != nil {
		// 尝试获取账户信息来验证JetStream连接
		_, err := n.js.AccountInfo()
		if err != nil {
			n.healthStatus.Store(false)
			return fmt.Errorf("JetStream connection is not active: %w", err)
		}
	}

	// 发送ping测试连接
	if err := n.conn.Flush(); err != nil {
		n.healthStatus.Store(false)
		return fmt.Errorf("NATS flush failed: %w", err)
	}

	// 更新健康状态
	n.healthStatus.Store(true)
	n.lastHealthCheck.Store(time.Now())

	n.logger.Debug("NATS eventbus health check passed",
		zap.Bool("jetstream_enabled", n.config.JetStream.Enabled),
		zap.Int64("published_messages", n.publishedMessages.Load()),
		zap.Int64("consumed_messages", n.consumedMessages.Load()),
		zap.Int64("error_count", n.errorCount.Load()))

	return nil
}

// Close 关闭连接
func (n *natsEventBus) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return nil
	}

	var errs []error

	// 关闭所有订阅
	for topic, sub := range n.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			errs = append(errs, fmt.Errorf("failed to unsubscribe from topic %s: %w", topic, err))
		}
	}

	// 停止积压检测器
	if n.backlogDetector != nil {
		if err := n.backlogDetector.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop backlog detector: %w", err))
		}
	}

	// ⭐ 停止所有Keyed-Worker池
	n.keyedPoolsMu.Lock()
	for topic, pool := range n.keyedPools {
		pool.Stop()
		n.logger.Debug("Stopped keyed worker pool", zap.String("topic", topic))
	}
	n.keyedPools = make(map[string]*KeyedWorkerPool)
	n.keyedPoolsMu.Unlock()

	// 清空订阅和消费者映射
	n.subscriptions = make(map[string]*nats.Subscription)
	n.consumers = make(map[string]nats.ConsumerInfo)

	// 关闭NATS连接
	if n.conn != nil {
		n.conn.Close()
	}

	n.closed = true
	n.healthStatus.Store(false)

	if len(errs) > 0 {
		n.logger.Warn("Some errors occurred during NATS EventBus close", zap.Errors("errors", errs))
		return fmt.Errorf("errors during close: %v", errs)
	}

	n.logger.Info("NATS EventBus closed successfully")
	return nil
}

// RegisterReconnectCallback 注册重连回调
func (n *natsEventBus) RegisterReconnectCallback(callback ReconnectCallback) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("eventbus is closed")
	}

	// 保存新的回调类型
	n.reconnectCallback = callback
	// 同时保持对旧的回调列表的兼容性
	n.reconnectCallbacks = append(n.reconnectCallbacks, callback)
	n.logger.Info("NATS reconnect callback registered")
	return nil
}

// executeReconnectCallbacks 执行重连回调
// 注意：这个函数在 NATS 重连回调中调用，没有父 context
// 因此使用 Background context 是合理的
func (n *natsEventBus) executeReconnectCallbacks() {
	n.mu.RLock()
	callbacks := make([]func(ctx context.Context) error, len(n.reconnectCallbacks))
	copy(callbacks, n.reconnectCallbacks)
	n.mu.RUnlock()

	// 使用 Background context，因为这是在 NATS 重连回调中调用的
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, callback := range callbacks {
		if err := callback(ctx); err != nil {
			n.logger.Error("Reconnect callback failed", zap.Error(err))
		}
	}
}

// collectMetrics 收集JetStream指标
func (n *natsEventBus) collectMetrics() {
	defer n.metricsCollector.Stop()

	for {
		select {
		case <-n.metricsCollector.C:
			n.updateJetStreamMetrics()
		case <-time.After(time.Minute):
			// 防止协程泄漏，定期检查是否已关闭
			n.mu.RLock()
			if n.closed {
				n.mu.RUnlock()
				return
			}
			n.mu.RUnlock()
		}
	}
}

// updateJetStreamMetrics 更新JetStream指标
func (n *natsEventBus) updateJetStreamMetrics() {
	if n.js == nil {
		return
	}

	// 获取流信息
	streamName := n.config.JetStream.Stream.Name
	if streamName != "" {
		if streamInfo, err := n.js.StreamInfo(streamName); err == nil {
			// 更新JetStream指标
			if n.metrics != nil {
				n.metrics.MessageBacklog = int64(streamInfo.State.Msgs)
				n.metrics.ActiveConnections = int(streamInfo.State.Consumers)
			}
		}
	}

	// 获取消费者信息
	n.mu.RLock()
	for consumerName := range n.consumers {
		if consumerInfo, err := n.js.ConsumerInfo(streamName, consumerName); err == nil {
			// 更新消费者指标
			if n.metrics != nil {
				n.metrics.MessagesConsumed += int64(consumerInfo.Delivered.Consumer)
			}
		}
	}
	n.mu.RUnlock()
}

// ========== 生命周期管理 ==========

// Start 启动事件总线
func (n *natsEventBus) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("nats eventbus is closed")
	}

	logger.Info("NATS eventbus started successfully")
	return nil
}

// Stop 停止事件总线
func (n *natsEventBus) Stop() error {
	return n.Close()
}

// ========== 高级发布功能 ==========

// PublishWithOptions 使用选项发布消息
func (n *natsEventBus) PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error {
	// 基础实现，直接调用 Publish
	return n.Publish(ctx, topic, message)
}

// SetMessageFormatter 设置消息格式化器
func (n *natsEventBus) SetMessageFormatter(formatter MessageFormatter) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.messageFormatter = formatter
	logger.Debug("Message formatter set for nats eventbus")
	return nil
}

// RegisterPublishCallback 注册发布回调
func (n *natsEventBus) RegisterPublishCallback(callback PublishCallback) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.publishCallback = callback
	logger.Debug("Publish callback registered for nats eventbus")
	return nil
}

// ========== 高级订阅功能 ==========

// SubscribeWithOptions 使用选项订阅消息
func (n *natsEventBus) SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error {
	// 基础实现，直接调用 Subscribe
	return n.Subscribe(ctx, topic, handler)
}

// RegisterSubscriberBacklogCallback 注册订阅端积压回调
func (n *natsEventBus) RegisterSubscriberBacklogCallback(callback BacklogStateCallback) error {
	if n.backlogDetector != nil {
		return n.backlogDetector.RegisterCallback(callback)
	}
	n.logger.Info("Subscriber backlog callback registered for NATS eventbus (detector not available)")
	return nil
}

// StartSubscriberBacklogMonitoring 启动订阅端积压监控
func (n *natsEventBus) StartSubscriberBacklogMonitoring(ctx context.Context) error {
	if n.backlogDetector != nil {
		return n.backlogDetector.Start(ctx)
	}
	n.logger.Info("Subscriber backlog monitoring not available for NATS eventbus")
	return nil
}

// StopSubscriberBacklogMonitoring 停止订阅端积压监控
func (n *natsEventBus) StopSubscriberBacklogMonitoring() error {
	if n.backlogDetector != nil {
		return n.backlogDetector.Stop()
	}
	n.logger.Info("Subscriber backlog monitoring not available for NATS eventbus")
	return nil
}

// RegisterBacklogCallback 注册订阅端积压回调（已废弃，向后兼容）
func (n *natsEventBus) RegisterBacklogCallback(callback BacklogStateCallback) error {
	n.logger.Warn("RegisterBacklogCallback is deprecated, use RegisterSubscriberBacklogCallback instead")
	return n.RegisterSubscriberBacklogCallback(callback)
}

// StartBacklogMonitoring 启动订阅端积压监控（已废弃，向后兼容）
func (n *natsEventBus) StartBacklogMonitoring(ctx context.Context) error {
	n.logger.Warn("StartBacklogMonitoring is deprecated, use StartSubscriberBacklogMonitoring instead")
	return n.StartSubscriberBacklogMonitoring(ctx)
}

// StopBacklogMonitoring 停止订阅端积压监控（已废弃，向后兼容）
func (n *natsEventBus) StopBacklogMonitoring() error {
	n.logger.Warn("StopBacklogMonitoring is deprecated, use StopSubscriberBacklogMonitoring instead")
	return n.StopSubscriberBacklogMonitoring()
}

// RegisterPublisherBacklogCallback 注册发送端积压回调
func (n *natsEventBus) RegisterPublisherBacklogCallback(callback PublisherBacklogCallback) error {
	if n.publisherBacklogDetector != nil {
		return n.publisherBacklogDetector.RegisterCallback(callback)
	}
	n.logger.Debug("Publisher backlog callback registered for NATS eventbus (detector not available)")
	return nil
}

// StartPublisherBacklogMonitoring 启动发送端积压监控
func (n *natsEventBus) StartPublisherBacklogMonitoring(ctx context.Context) error {
	if n.publisherBacklogDetector != nil {
		return n.publisherBacklogDetector.Start(ctx)
	}
	n.logger.Debug("Publisher backlog monitoring not available for NATS eventbus (not configured)")
	return nil
}

// StopPublisherBacklogMonitoring 停止发送端积压监控
func (n *natsEventBus) StopPublisherBacklogMonitoring() error {
	if n.publisherBacklogDetector != nil {
		return n.publisherBacklogDetector.Stop()
	}
	n.logger.Debug("Publisher backlog monitoring not available for NATS eventbus (not configured)")
	return nil
}

// StartAllBacklogMonitoring 根据配置启动所有积压监控
func (n *natsEventBus) StartAllBacklogMonitoring(ctx context.Context) error {
	var errs []error

	// 启动订阅端积压监控
	if err := n.StartSubscriberBacklogMonitoring(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to start subscriber backlog monitoring: %w", err))
	}

	// 启动发送端积压监控
	if err := n.StartPublisherBacklogMonitoring(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to start publisher backlog monitoring: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to start some backlog monitoring: %v", errs)
	}

	n.logger.Info("All backlog monitoring started successfully for NATS eventbus")
	return nil
}

// StopAllBacklogMonitoring 停止所有积压监控
func (n *natsEventBus) StopAllBacklogMonitoring() error {
	var errs []error

	// 停止订阅端积压监控
	if err := n.StopSubscriberBacklogMonitoring(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop subscriber backlog monitoring: %w", err))
	}

	// 停止发送端积压监控
	if err := n.StopPublisherBacklogMonitoring(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop publisher backlog monitoring: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop some backlog monitoring: %v", errs)
	}

	n.logger.Info("All backlog monitoring stopped successfully for NATS eventbus")
	return nil
}

// SetMessageRouter 设置消息路由器
func (n *natsEventBus) SetMessageRouter(router MessageRouter) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.messageRouter = router
	logger.Debug("Message router set for nats eventbus")
	return nil
}

// SetErrorHandler 设置错误处理器
func (n *natsEventBus) SetErrorHandler(handler ErrorHandler) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.errorHandler = handler
	logger.Info("Error handler set for nats eventbus")
	return nil
}

// RegisterSubscriptionCallback 注册订阅回调
func (n *natsEventBus) RegisterSubscriptionCallback(callback SubscriptionCallback) error {
	logger.Info("Subscription callback registered for nats eventbus")
	return nil
}

// ========== 统一健康检查和监控 ==========

// StartHealthCheck 启动健康检查
func (n *natsEventBus) StartHealthCheck(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 如果已经启动，先停止之前的
	if n.healthCheckCancel != nil {
		n.healthCheckCancel()
		if n.healthCheckDone != nil {
			<-n.healthCheckDone // 等待之前的健康检查完全停止
		}
	}

	// 创建新的控制 context
	healthCtx, cancel := context.WithCancel(ctx)
	n.healthCheckCancel = cancel
	n.healthCheckDone = make(chan struct{})

	// 启动健康检查协程
	go func() {
		defer close(n.healthCheckDone)

		ticker := time.NewTicker(30 * time.Second) // 默认30秒检查一次
		defer ticker.Stop()

		for {
			select {
			case <-healthCtx.Done():
				n.logger.Info("Health check stopped for nats eventbus")
				return
			case <-ticker.C:
				if err := n.healthCheck(healthCtx); err != nil {
					n.logger.Error("Health check failed", zap.Error(err))

					// 增加失败计数
					failureCount := n.failureCount.Add(1)

					// 检查是否达到重连阈值
					if failureCount >= int32(n.reconnectConfig.FailureThreshold) {
						n.logger.Warn("Health check failure threshold reached, attempting reconnect",
							zap.Int32("failureCount", failureCount),
							zap.Int("threshold", n.reconnectConfig.FailureThreshold))

						// 触发自动重连
						if reconnectErr := n.reconnect(healthCtx); reconnectErr != nil {
							n.logger.Error("Auto-reconnect failed", zap.Error(reconnectErr))
						} else {
							n.logger.Info("Auto-reconnect successful")
							n.failureCount.Store(0) // 重置失败计数
						}
					}
				} else {
					// 健康检查成功，重置失败计数
					n.failureCount.Store(0)
				}
			}
		}
	}()

	n.logger.Info("Health check started for nats eventbus")
	return nil
}

// StopHealthCheck 停止健康检查
func (n *natsEventBus) StopHealthCheck() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.healthCheckCancel != nil {
		// 取消健康检查 context
		n.healthCheckCancel()

		// 等待健康检查 goroutine 完全停止
		if n.healthCheckDone != nil {
			<-n.healthCheckDone
		}

		// 清理资源
		n.healthCheckCancel = nil
		n.healthCheckDone = nil

		n.logger.Info("Health check stopped for nats eventbus")
	} else {
		n.logger.Debug("Health check was not running")
	}

	return nil
}

// GetHealthStatus 获取健康状态
func (n *natsEventBus) GetHealthStatus() HealthCheckStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()

	isHealthy := !n.closed && n.conn != nil && n.conn.IsConnected()
	return HealthCheckStatus{
		IsHealthy:           isHealthy,
		ConsecutiveFailures: 0,
		LastSuccessTime:     time.Now(),
		LastFailureTime:     time.Time{},
		IsRunning:           !n.closed,
		EventBusType:        "nats",
		Source:              "nats-eventbus",
	}
}

// RegisterHealthCheckCallback 注册健康检查回调
func (n *natsEventBus) RegisterHealthCheckCallback(callback HealthCheckCallback) error {
	logger.Info("Health check callback registered for nats eventbus")
	return nil
}

// StartHealthCheckSubscriber 启动健康检查消息订阅监控
func (n *natsEventBus) StartHealthCheckSubscriber(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.healthCheckSubscriber != nil {
		return nil // 已经启动
	}

	// 创建健康检查订阅监控器
	config := GetDefaultHealthCheckConfig()
	n.healthCheckSubscriber = NewHealthCheckSubscriber(config, n, "nats-eventbus", "nats")

	// 启动监控器
	if err := n.healthCheckSubscriber.Start(ctx); err != nil {
		n.healthCheckSubscriber = nil
		return fmt.Errorf("failed to start health check subscriber: %w", err)
	}

	n.logger.Info("Health check subscriber started for nats eventbus")
	return nil
}

// StopHealthCheckSubscriber 停止健康检查消息订阅监控
func (n *natsEventBus) StopHealthCheckSubscriber() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.healthCheckSubscriber == nil {
		return nil
	}

	if err := n.healthCheckSubscriber.Stop(); err != nil {
		n.logger.Error("Failed to stop health check subscriber", zap.Error(err))
		return err
	}

	n.healthCheckSubscriber = nil
	n.logger.Info("Health check subscriber stopped for nats eventbus")
	return nil
}

// RegisterHealthCheckAlertCallback 注册健康检查告警回调
func (n *natsEventBus) RegisterHealthCheckAlertCallback(callback HealthCheckAlertCallback) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.healthCheckSubscriber == nil {
		return fmt.Errorf("health check subscriber not started")
	}

	return n.healthCheckSubscriber.RegisterAlertCallback(callback)
}

// GetHealthCheckSubscriberStats 获取健康检查订阅监控统计信息
func (n *natsEventBus) GetHealthCheckSubscriberStats() HealthCheckSubscriberStats {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.healthCheckSubscriber == nil {
		return HealthCheckSubscriberStats{}
	}

	return n.healthCheckSubscriber.GetStats()
}

// GetConnectionState 获取连接状态
func (n *natsEventBus) GetConnectionState() ConnectionState {
	n.mu.RLock()
	defer n.mu.RUnlock()

	isConnected := !n.closed && n.conn != nil && n.conn.IsConnected()
	return ConnectionState{
		IsConnected:       isConnected,
		LastConnectedTime: time.Now(),
		ReconnectCount:    0,
		LastError:         "",
	}
}

// GetMetrics 获取监控指标
func (n *natsEventBus) GetMetrics() Metrics {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return Metrics{
		MessagesPublished: n.publishedMessages.Load(),
		MessagesConsumed:  n.consumedMessages.Load(),
		PublishErrors:     n.errorCount.Load(),
		ConsumeErrors:     0,
		ConnectionErrors:  0,
		LastHealthCheck:   time.Now(),
		HealthCheckStatus: "healthy",
		ActiveConnections: 1,
		MessageBacklog:    0,
	}
}

// CheckConnection 检查 NATS 连接状态
func (n *natsEventBus) CheckConnection(ctx context.Context) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.closed {
		return fmt.Errorf("nats eventbus is closed")
	}

	if n.conn == nil {
		return fmt.Errorf("nats connection is nil")
	}

	if !n.conn.IsConnected() {
		return fmt.Errorf("nats connection is not connected")
	}

	// 检查服务器信息
	if !n.conn.IsReconnecting() && n.conn.ConnectedUrl() != "" {
		n.logger.Debug("NATS connection check passed",
			zap.String("connectedUrl", n.conn.ConnectedUrl()),
			zap.String("status", n.conn.Status().String()))
		return nil
	}

	return fmt.Errorf("nats connection is in invalid state")
}

// CheckMessageTransport 检查端到端消息传输
func (n *natsEventBus) CheckMessageTransport(ctx context.Context) error {
	testSubject := "health.check"
	testMessage := fmt.Sprintf("health-check-%d", time.Now().UnixNano())

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 如果连接支持订阅，进行端到端测试
	if n.conn != nil && n.conn.IsConnected() {
		return n.performNATSEndToEndTest(ctx, testSubject, testMessage)
	}

	// 如果没有连接，只测试发布能力
	start := time.Now()
	err := n.Publish(ctx, testSubject, []byte(testMessage))
	publishLatency := time.Since(start)

	if err != nil {
		n.logger.Error("NATS health check message transport failed",
			zap.Error(err),
			zap.Duration("publishLatency", publishLatency))
		return fmt.Errorf("failed to publish health check message: %w", err)
	}

	n.logger.Debug("NATS health check message transport successful",
		zap.Duration("publishLatency", publishLatency))

	return nil
}

// performNATSEndToEndTest 执行 NATS 端到端测试
func (n *natsEventBus) performNATSEndToEndTest(ctx context.Context, testSubject, testMessage string) error {
	// 创建接收通道
	receiveChan := make(chan string, 1)

	// 创建临时订阅来接收健康检查消息
	sub, err := n.conn.Subscribe(testSubject, func(msg *nats.Msg) {
		receivedMsg := string(msg.Data)
		if receivedMsg == testMessage {
			select {
			case receiveChan <- receivedMsg:
			default:
			}
		}
	})
	if err != nil {
		n.logger.Warn("Failed to create subscription for health check, falling back to publish-only test",
			zap.Error(err))
		// 回退到只测试发布
		start := time.Now()
		if err := n.Publish(ctx, testSubject, []byte(testMessage)); err != nil {
			return fmt.Errorf("failed to publish health check message: %w", err)
		}
		publishLatency := time.Since(start)
		n.logger.Debug("NATS health check message published (no subscription test)",
			zap.Duration("publishLatency", publishLatency))
		return nil
	}
	defer sub.Unsubscribe()

	// 等待一小段时间确保订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布健康检查消息
	start := time.Now()
	if err := n.Publish(ctx, testSubject, []byte(testMessage)); err != nil {
		return fmt.Errorf("failed to publish health check message: %w", err)
	}
	publishLatency := time.Since(start)

	// 等待接收消息或超时
	select {
	case receivedMsg := <-receiveChan:
		totalLatency := time.Since(start)
		n.logger.Debug("NATS end-to-end health check successful",
			zap.Duration("publishLatency", publishLatency),
			zap.Duration("totalLatency", totalLatency),
			zap.String("message", receivedMsg))
		return nil

	case <-ctx.Done():
		return fmt.Errorf("nats health check timeout: message not received within timeout period")

	case <-time.After(8 * time.Second):
		return fmt.Errorf("nats health check timeout: message not received within 8 seconds")
	}
}

// GetEventBusMetrics 获取 NATS EventBus 性能指标
func (n *natsEventBus) GetEventBusMetrics() EventBusHealthMetrics {
	n.mu.RLock()
	defer n.mu.RUnlock()

	connectionStatus := "disconnected"
	if !n.closed && n.conn != nil && n.conn.IsConnected() {
		connectionStatus = "connected"
	}

	return EventBusHealthMetrics{
		ConnectionStatus:    connectionStatus,
		PublishLatency:      0,                          // TODO: 实际测量并缓存
		SubscribeLatency:    0,                          // TODO: 实际测量并缓存
		LastSuccessTime:     time.Now(),                 // TODO: 实际跟踪
		LastFailureTime:     time.Time{},                // TODO: 实际跟踪
		ConsecutiveFailures: 0,                          // TODO: 实际统计
		ThroughputPerSecond: n.publishedMessages.Load(), // 简化实现
		MessageBacklog:      0,                          // TODO: 实际计算
		ReconnectCount:      0,                          // TODO: 实际统计
		BrokerCount:         1,                          // NATS 通常是单个服务器或集群
		TopicCount:          len(n.subscriptions),       // 当前订阅的主题数量
	}
}

// ========== 自动重连功能 ==========

// SetReconnectConfig 设置重连配置
func (n *natsEventBus) SetReconnectConfig(config ReconnectConfig) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.reconnectConfig = config
	n.logger.Info("NATS reconnect config updated",
		zap.Int("maxAttempts", config.MaxAttempts),
		zap.Duration("initialBackoff", config.InitialBackoff),
		zap.Duration("maxBackoff", config.MaxBackoff),
		zap.Float64("backoffFactor", config.BackoffFactor),
		zap.Int("failureThreshold", config.FailureThreshold))
	return nil
}

// GetReconnectStatus 获取重连状态
func (n *natsEventBus) GetReconnectStatus() ReconnectStatus {
	failureCount := n.failureCount.Load()

	var lastReconnectTime time.Time
	if t := n.lastReconnectTime.Load(); t != nil {
		lastReconnectTime = t.(time.Time)
	}

	return ReconnectStatus{
		FailureCount:      int(failureCount),
		LastReconnectTime: lastReconnectTime,
		IsReconnecting:    false, // NATS 客户端内部处理重连状态
		Config:            n.reconnectConfig,
	}
}

// reconnect 执行重连逻辑
func (n *natsEventBus) reconnect(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("nats eventbus is closed")
	}

	n.logger.Info("Starting NATS reconnection process")

	// 使用指数退避算法进行重连尝试
	for attempt := 1; attempt <= n.reconnectConfig.MaxAttempts; attempt++ {
		// 计算退避时间
		backoff := n.calculateBackoff(attempt)

		n.logger.Info("Attempting NATS reconnection",
			zap.Int("attempt", attempt),
			zap.Int("maxAttempts", n.reconnectConfig.MaxAttempts),
			zap.Duration("backoff", backoff))

		// 等待退避时间
		select {
		case <-ctx.Done():
			return fmt.Errorf("reconnect cancelled: %w", ctx.Err())
		case <-time.After(backoff):
		}

		// 尝试重新初始化连接
		if err := n.reinitializeConnection(); err != nil {
			n.logger.Warn("NATS reconnection attempt failed",
				zap.Int("attempt", attempt),
				zap.Error(err))

			if attempt == n.reconnectConfig.MaxAttempts {
				return fmt.Errorf("failed to reconnect after %d attempts: %w", attempt, err)
			}
			continue
		}

		// 重连成功，恢复订阅
		if err := n.restoreSubscriptions(ctx); err != nil {
			n.logger.Error("Failed to restore subscriptions after reconnect", zap.Error(err))
			// 不返回错误，因为连接已经成功，订阅可以稍后重试
		}

		// 更新重连时间
		n.lastReconnectTime.Store(time.Now())

		// 调用重连回调
		if n.reconnectCallback != nil {
			go func() {
				// 从父 context 派生，支持取消传播
				callbackCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				if err := n.reconnectCallback(callbackCtx); err != nil {
					n.logger.Error("Reconnect callback failed", zap.Error(err))
				}
			}()
		}

		n.logger.Info("NATS reconnection successful", zap.Int("attempt", attempt))
		return nil
	}

	return fmt.Errorf("failed to reconnect after %d attempts", n.reconnectConfig.MaxAttempts)
}

// calculateBackoff 计算指数退避时间
func (n *natsEventBus) calculateBackoff(attempt int) time.Duration {
	backoff := float64(n.reconnectConfig.InitialBackoff)

	// 指数退避算法
	for i := 1; i < attempt; i++ {
		backoff *= n.reconnectConfig.BackoffFactor
	}

	// 限制最大退避时间
	if backoff > float64(n.reconnectConfig.MaxBackoff) {
		backoff = float64(n.reconnectConfig.MaxBackoff)
	}

	return time.Duration(backoff)
}

// reinitializeConnection 重新初始化 NATS 连接
func (n *natsEventBus) reinitializeConnection() error {
	// 关闭现有连接
	if n.conn != nil {
		n.conn.Close()
	}

	// 构建连接选项
	opts := buildNATSOptions(n.config)

	// 重新连接到NATS服务器
	var nc *nats.Conn
	var err error

	if len(n.config.URLs) > 0 {
		nc, err = nats.Connect(n.config.URLs[0], opts...)
	} else {
		nc, err = nats.Connect(nats.DefaultURL, opts...)
	}

	if err != nil {
		return fmt.Errorf("failed to reconnect to NATS: %w", err)
	}

	// 更新连接
	n.conn = nc

	// 重新创建JetStream上下文（如果启用）
	if n.config.JetStream.Enabled {
		jsOpts := buildJetStreamOptions(n.config)
		js, err := nc.JetStream(jsOpts...)
		if err != nil {
			nc.Close()
			return fmt.Errorf("failed to recreate JetStream context: %w", err)
		}
		n.js = js

		// 确保流存在
		if err := ensureStream(js, n.config); err != nil {
			nc.Close()
			return fmt.Errorf("failed to ensure stream after reconnect: %w", err)
		}
	}

	// 设置重连处理器
	nc.SetReconnectHandler(func(nc *nats.Conn) {
		n.logger.Info("NATS reconnected", zap.String("url", nc.ConnectedUrl()))
		// 重置失败计数
		n.failureCount.Store(0)
		// 更新重连时间
		n.lastReconnectTime.Store(time.Now())
		// 恢复订阅
		n.restoreSubscriptions(context.Background())
		// 执行重连回调
		n.executeReconnectCallbacks()
	})

	n.logger.Info("NATS connection reinitialized successfully")
	return nil
}

// restoreSubscriptions 恢复所有订阅
func (n *natsEventBus) restoreSubscriptions(ctx context.Context) error {
	n.subscriptionsMu.RLock()
	handlers := make(map[string]MessageHandler)
	for topic, handler := range n.subscriptionHandlers {
		handlers[topic] = handler
	}
	n.subscriptionsMu.RUnlock()

	if len(handlers) == 0 {
		n.logger.Debug("No subscriptions to restore")
		return nil
	}

	n.logger.Info("Restoring NATS subscriptions", zap.Int("count", len(handlers)))

	// 清空现有订阅映射
	n.subscriptions = make(map[string]*nats.Subscription)
	n.consumers = make(map[string]nats.ConsumerInfo)

	var errors []error
	restoredCount := 0

	// 重新建立每个订阅（只使用JetStream）
	for topic, handler := range handlers {
		err := n.subscribeJetStream(ctx, topic, handler)

		if err != nil {
			errors = append(errors, fmt.Errorf("failed to restore subscription for topic %s: %w", topic, err))
			n.logger.Error("Failed to restore JetStream subscription",
				zap.String("topic", topic),
				zap.Error(err))
		} else {
			restoredCount++
			n.logger.Debug("JetStream subscription restored",
				zap.String("topic", topic),
				zap.Bool("jetstream", true))
		}
	}

	n.logger.Info("NATS subscriptions restoration completed",
		zap.Int("total", len(handlers)),
		zap.Int("restored", restoredCount),
		zap.Int("failed", len(errors)))

	if len(errors) > 0 {
		return fmt.Errorf("failed to restore %d subscriptions: %v", len(errors), errors)
	}

	return nil
}

// ========== 方案A：Envelope 支持 ==========

// PublishEnvelope 发布Envelope消息（方案A）
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.closed {
		return fmt.Errorf("nats eventbus is closed")
	}

	// 校验Envelope
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("invalid envelope: %w", err)
	}

	// 序列化Envelope
	envelopeBytes, err := envelope.ToBytes()
	if err != nil {
		n.errorCount.Add(1)
		return fmt.Errorf("failed to serialize envelope: %w", err)
	}

	// 创建NATS消息（方案A + 传输层镜像）
	msg := &nats.Msg{
		Subject: topic,
		Data:    envelopeBytes,
		Header: nats.Header{
			"X-Aggregate-ID":  []string{envelope.AggregateID},
			"X-Event-Version": []string{fmt.Sprintf("%d", envelope.EventVersion)},
			"X-Event-Type":    []string{envelope.EventType},
			"X-Timestamp":     []string{envelope.Timestamp.Format(time.RFC3339)},
		},
	}

	// 添加可选字段到Header
	if envelope.TraceID != "" {
		msg.Header.Set("X-Trace-ID", envelope.TraceID)
	}
	if envelope.CorrelationID != "" {
		msg.Header.Set("X-Correlation-ID", envelope.CorrelationID)
	}

	// 发送消息
	_, err = n.js.PublishMsg(msg)
	if err != nil {
		n.errorCount.Add(1)
		n.logger.Error("Failed to publish envelope message",
			zap.String("subject", topic),
			zap.String("aggregateID", envelope.AggregateID),
			zap.String("eventType", envelope.EventType),
			zap.Int64("eventVersion", envelope.EventVersion),
			zap.Error(err))
		return fmt.Errorf("failed to publish envelope message: %w", err)
	}

	n.publishedMessages.Add(1)
	n.logger.Debug("Envelope message published successfully",
		zap.String("subject", topic),
		zap.String("aggregateID", envelope.AggregateID),
		zap.String("eventType", envelope.EventType),
		zap.Int64("eventVersion", envelope.EventVersion))

	return nil
}

// SubscribeEnvelope 订阅Envelope消息（自动使用Keyed-Worker池）
//
// 特点：
// - 消息格式：Envelope包装格式（包含聚合ID、事件类型、版本等元数据）
// - 处理模式：按聚合ID路由到Keyed-Worker池，同聚合ID严格顺序处理
// - 性能：顺序保证，毫秒级延迟（NATS JetStream持久化）
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
// - 跨Docker容器的有序事件处理
// - 需要顺序保证的业务场景
//
// 示例：
//
//	bus.SubscribeEnvelope(ctx, "orders.events", func(ctx context.Context, env *Envelope) error {
//	    // env.AggregateID = "order-123"
//	    // 同一订单的所有事件会路由到同一个Worker，确保顺序处理
//	    return processDomainEvent(env)
//	})
func (n *natsEventBus) SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error {
	// 包装EnvelopeHandler为MessageHandler
	wrappedHandler := func(ctx context.Context, message []byte) error {
		// 尝试解析为Envelope
		envelope, err := FromBytes(message)
		if err != nil {
			n.logger.Error("Failed to parse envelope message",
				zap.String("subject", topic),
				zap.Error(err))
			return fmt.Errorf("failed to parse envelope: %w", err)
		}

		// 调用业务处理器
		return handler(ctx, envelope)
	}

	// 使用现有的Subscribe方法
	return n.Subscribe(ctx, topic, wrappedHandler)
}

// ========== 新的分离式健康检查接口实现 ==========

// StartHealthCheckPublisher 启动健康检查发布器
func (n *natsEventBus) StartHealthCheckPublisher(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.healthChecker != nil {
		return nil // 已经启动
	}

	// 创建健康检查发布器
	config := GetDefaultHealthCheckConfig()
	n.healthChecker = NewHealthChecker(config, n, "nats-eventbus", "nats")

	// 启动健康检查发布器
	if err := n.healthChecker.Start(ctx); err != nil {
		n.healthChecker = nil
		return fmt.Errorf("failed to start health check publisher: %w", err)
	}

	n.logger.Info("Health check publisher started for nats eventbus")
	return nil
}

// StopHealthCheckPublisher 停止健康检查发布器
func (n *natsEventBus) StopHealthCheckPublisher() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.healthChecker == nil {
		return nil
	}

	if err := n.healthChecker.Stop(); err != nil {
		return fmt.Errorf("failed to stop health check publisher: %w", err)
	}

	n.healthChecker = nil
	n.logger.Info("Health check publisher stopped for nats eventbus")
	return nil
}

// GetHealthCheckPublisherStatus 获取健康检查发布器状态
func (n *natsEventBus) GetHealthCheckPublisherStatus() HealthCheckStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.healthChecker == nil {
		return HealthCheckStatus{
			IsHealthy:           false,
			ConsecutiveFailures: 0,
			LastSuccessTime:     time.Time{},
			LastFailureTime:     time.Now(),
			IsRunning:           false,
			EventBusType:        "nats",
			Source:              "nats-eventbus",
		}
	}

	return n.healthChecker.GetStatus()
}

// RegisterHealthCheckPublisherCallback 注册健康检查发布器回调
func (n *natsEventBus) RegisterHealthCheckPublisherCallback(callback HealthCheckCallback) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.healthChecker == nil {
		return fmt.Errorf("health check publisher not started")
	}

	return n.healthChecker.RegisterCallback(callback)
}

// RegisterHealthCheckSubscriberCallback 注册健康检查订阅器回调
func (n *natsEventBus) RegisterHealthCheckSubscriberCallback(callback HealthCheckAlertCallback) error {
	return n.RegisterHealthCheckAlertCallback(callback)
}

// StartAllHealthCheck 根据配置启动所有健康检查
func (n *natsEventBus) StartAllHealthCheck(ctx context.Context) error {
	// 这里可以根据配置决定启动哪些健康检查
	// 为了演示，我们启动发布器和订阅器
	if err := n.StartHealthCheckPublisher(ctx); err != nil {
		return fmt.Errorf("failed to start health check publisher: %w", err)
	}

	if err := n.StartHealthCheckSubscriber(ctx); err != nil {
		return fmt.Errorf("failed to start health check subscriber: %w", err)
	}

	return nil
}

// StopAllHealthCheck 停止所有健康检查
func (n *natsEventBus) StopAllHealthCheck() error {
	var errs []error

	if err := n.StopHealthCheckPublisher(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop health check publisher: %w", err))
	}

	if err := n.StopHealthCheckSubscriber(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop health check subscriber: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping health checks: %v", errs)
	}

	return nil
}

// ========== 主题持久化管理实现 ==========

// ConfigureTopic 配置主题的持久化策略和其他选项（幂等操作）
func (n *natsEventBus) ConfigureTopic(ctx context.Context, topic string, options TopicOptions) error {
	start := time.Now()

	n.topicConfigsMu.Lock()
	// 检查是否已有配置
	_, exists := n.topicConfigs[topic]
	// 缓存配置
	n.topicConfigs[topic] = options
	n.topicConfigsMu.Unlock()

	// 根据策略决定是否需要同步到消息中间件
	shouldCreate, shouldUpdate := shouldCreateOrUpdate(n.topicConfigStrategy, exists)

	var action string
	var err error
	var mismatches []TopicConfigMismatch

	// 如果是持久化模式且JetStream可用
	if options.IsPersistent(n.config.JetStream.Enabled) && n.js != nil {
		switch {
		case n.topicConfigStrategy == StrategySkip:
			// 跳过模式：不检查
			action = "skipped"

		case n.topicConfigStrategy == StrategyValidateOnly:
			// 验证模式：只验证，不修改
			action = "validated"
			if exists {
				actualConfig, validateErr := n.getActualTopicConfig(ctx, topic)
				if validateErr == nil {
					mismatches = compareTopicOptions(topic, options, actualConfig)
					if len(mismatches) > 0 {
						err = handleConfigMismatches(mismatches, n.topicConfigOnMismatch)
					}
				}
			}

		case shouldCreate:
			// 创建模式：创建新配置
			action = "created"
			err = n.ensureTopicInJetStreamIdempotent(ctx, topic, options, false)

		case shouldUpdate:
			// 更新模式：更新现有配置
			action = "updated"
			// 先验证配置差异
			actualConfig, validateErr := n.getActualTopicConfig(ctx, topic)
			if validateErr == nil {
				mismatches = compareTopicOptions(topic, options, actualConfig)
			}
			// 执行更新
			err = n.ensureTopicInJetStreamIdempotent(ctx, topic, options, true)

		default:
			// 默认：创建或更新
			action = "configured"
			err = n.ensureTopicInJetStreamIdempotent(ctx, topic, options, exists)
		}
	} else {
		// 非持久化模式或JetStream不可用
		action = "configured_ephemeral"
	}

	duration := time.Since(start)

	// 记录结果
	if err != nil {
		n.logger.Error("Topic configuration failed",
			zap.String("topic", topic),
			zap.String("action", action),
			zap.String("strategy", string(n.topicConfigStrategy)),
			zap.Error(err),
			zap.Duration("duration", duration))
		return fmt.Errorf("failed to configure topic %s: %w", topic, err)
	}

	n.logger.Info("Topic configured successfully",
		zap.String("topic", topic),
		zap.String("action", action),
		zap.String("strategy", string(n.topicConfigStrategy)),
		zap.String("persistenceMode", string(options.PersistenceMode)),
		zap.Duration("retentionTime", options.RetentionTime),
		zap.Int64("maxSize", options.MaxSize),
		zap.Int("mismatches", len(mismatches)),
		zap.Duration("duration", duration))

	return nil
}

// SetTopicPersistence 设置主题是否持久化（简化接口）
func (n *natsEventBus) SetTopicPersistence(ctx context.Context, topic string, persistent bool) error {
	mode := TopicEphemeral
	if persistent {
		mode = TopicPersistent
	}

	options := DefaultTopicOptions()
	options.PersistenceMode = mode

	return n.ConfigureTopic(ctx, topic, options)
}

// GetTopicConfig 获取主题的当前配置
func (n *natsEventBus) GetTopicConfig(topic string) (TopicOptions, error) {
	n.topicConfigsMu.RLock()
	defer n.topicConfigsMu.RUnlock()

	if config, exists := n.topicConfigs[topic]; exists {
		return config, nil
	}

	// 返回默认配置
	return DefaultTopicOptions(), nil
}

// ListConfiguredTopics 列出所有已配置的主题
func (n *natsEventBus) ListConfiguredTopics() []string {
	n.topicConfigsMu.RLock()
	defer n.topicConfigsMu.RUnlock()

	topics := make([]string, 0, len(n.topicConfigs))
	for topic := range n.topicConfigs {
		topics = append(topics, topic)
	}

	return topics
}

// RemoveTopicConfig 移除主题配置（恢复为默认行为）
func (n *natsEventBus) RemoveTopicConfig(topic string) error {
	n.topicConfigsMu.Lock()
	defer n.topicConfigsMu.Unlock()

	delete(n.topicConfigs, topic)

	n.logger.Info("Topic configuration removed", zap.String("topic", topic))
	return nil
}

// ensureTopicInJetStream 确保主题在JetStream中存在
func (n *natsEventBus) ensureTopicInJetStream(topic string, options TopicOptions) error {
	if n.js == nil {
		return fmt.Errorf("JetStream not enabled")
	}

	// 获取或创建适合该主题的Stream名称
	streamName := n.getStreamNameForTopic(topic)

	// 尝试获取Stream信息
	streamInfo, err := n.js.StreamInfo(streamName)
	if err != nil {
		// Stream不存在，创建新的
		return n.createStreamForTopic(topic, options)
	}

	// 检查主题是否已在Stream的subjects中
	for _, subject := range streamInfo.Config.Subjects {
		if subject == topic || subject == topic+".*" {
			return nil // 已存在
		}
	}

	// 添加主题到现有Stream
	return n.addTopicToStream(streamName, topic, options)
}

// getStreamNameForTopic 为主题生成Stream名称
func (n *natsEventBus) getStreamNameForTopic(topic string) string {
	// 使用配置的Stream名称，或者基于主题生成
	if n.config.JetStream.Stream.Name != "" {
		return n.config.JetStream.Stream.Name
	}
	// 生成基于主题的Stream名称
	return fmt.Sprintf("STREAM_%s", strings.ReplaceAll(topic, ".", "_"))
}

// createStreamForTopic 为主题创建新的Stream
func (n *natsEventBus) createStreamForTopic(topic string, options TopicOptions) error {
	streamConfig := &nats.StreamConfig{
		Name:      n.getStreamNameForTopic(topic),
		Subjects:  []string{topic},
		Storage:   nats.FileStorage, // 持久化存储
		Retention: nats.LimitsPolicy,
		Replicas:  1,
	}

	// 根据选项设置配置
	if options.RetentionTime > 0 {
		streamConfig.MaxAge = options.RetentionTime
	}
	if options.MaxSize > 0 {
		streamConfig.MaxBytes = options.MaxSize
	}
	if options.MaxMessages > 0 {
		streamConfig.MaxMsgs = options.MaxMessages
	}
	if options.Replicas > 0 {
		streamConfig.Replicas = options.Replicas
	}

	_, err := n.js.AddStream(streamConfig)
	if err != nil {
		return fmt.Errorf("failed to create stream for topic %s: %w", topic, err)
	}

	n.logger.Info("Created JetStream stream for topic",
		zap.String("topic", topic),
		zap.String("stream", streamConfig.Name))

	return nil
}

// addTopicToStream 将主题添加到现有Stream
func (n *natsEventBus) addTopicToStream(streamName, topic string, options TopicOptions) error {
	// 获取现有Stream配置
	streamInfo, err := n.js.StreamInfo(streamName)
	if err != nil {
		return fmt.Errorf("failed to get stream info: %w", err)
	}

	// 添加新主题到subjects列表
	newSubjects := append(streamInfo.Config.Subjects, topic)
	streamInfo.Config.Subjects = newSubjects

	// 更新Stream配置
	_, err = n.js.UpdateStream(&streamInfo.Config)
	if err != nil {
		return fmt.Errorf("failed to update stream with new topic: %w", err)
	}

	n.logger.Info("Added topic to existing stream",
		zap.String("topic", topic),
		zap.String("stream", streamName))

	return nil
}

// ensureTopicInJetStreamIdempotent 幂等地确保主题在JetStream中存在（支持创建和更新）
func (n *natsEventBus) ensureTopicInJetStreamIdempotent(ctx context.Context, topic string, options TopicOptions, allowUpdate bool) error {
	if n.js == nil {
		return fmt.Errorf("JetStream not enabled")
	}

	streamName := n.getStreamNameForTopic(topic)

	// 构建期望的Stream配置
	expectedConfig := &nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{topic},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		Replicas:  1,
	}

	// 应用选项
	if options.RetentionTime > 0 {
		expectedConfig.MaxAge = options.RetentionTime
	}
	if options.MaxSize > 0 {
		expectedConfig.MaxBytes = options.MaxSize
	}
	if options.MaxMessages > 0 {
		expectedConfig.MaxMsgs = options.MaxMessages
	}
	if options.Replicas > 0 {
		expectedConfig.Replicas = options.Replicas
	}

	// 检查Stream是否存在
	streamInfo, err := n.js.StreamInfo(streamName)

	if err != nil {
		if err == nats.ErrStreamNotFound {
			// Stream不存在，创建新的
			n.logger.Info("Creating new JetStream stream",
				zap.String("stream", streamName),
				zap.String("topic", topic))

			_, err := n.js.AddStream(expectedConfig)
			if err != nil {
				return fmt.Errorf("failed to create stream: %w", err)
			}

			n.logger.Info("Created JetStream stream",
				zap.String("stream", streamName),
				zap.String("topic", topic))
			return nil
		}
		return fmt.Errorf("failed to get stream info: %w", err)
	}

	// Stream已存在
	// 检查主题是否已在Stream的subjects中
	topicExists := false
	for _, subject := range streamInfo.Config.Subjects {
		if subject == topic || subject == topic+".*" {
			topicExists = true
			break
		}
	}

	if !topicExists {
		// 主题不在Stream中，添加主题
		n.logger.Info("Adding topic to existing stream",
			zap.String("stream", streamName),
			zap.String("topic", topic))

		streamInfo.Config.Subjects = append(streamInfo.Config.Subjects, topic)
		_, err = n.js.UpdateStream(&streamInfo.Config)
		if err != nil {
			return fmt.Errorf("failed to add topic to stream: %w", err)
		}
	}

	// 如果允许更新，检查配置是否需要更新
	if allowUpdate {
		needsUpdate := false

		// 比较配置
		if expectedConfig.MaxAge != streamInfo.Config.MaxAge {
			streamInfo.Config.MaxAge = expectedConfig.MaxAge
			needsUpdate = true
		}
		if expectedConfig.MaxBytes != streamInfo.Config.MaxBytes {
			streamInfo.Config.MaxBytes = expectedConfig.MaxBytes
			needsUpdate = true
		}
		if expectedConfig.MaxMsgs != streamInfo.Config.MaxMsgs {
			streamInfo.Config.MaxMsgs = expectedConfig.MaxMsgs
			needsUpdate = true
		}

		if needsUpdate {
			n.logger.Info("Updating stream configuration",
				zap.String("stream", streamName),
				zap.String("topic", topic))

			_, err = n.js.UpdateStream(&streamInfo.Config)
			if err != nil {
				n.logger.Warn("Failed to update stream config, using existing config",
					zap.String("stream", streamName),
					zap.Error(err))
				// 不返回错误，使用现有配置
			}
		}
	}

	return nil
}

// getActualTopicConfig 获取主题在JetStream中的实际配置
func (n *natsEventBus) getActualTopicConfig(ctx context.Context, topic string) (TopicOptions, error) {
	if n.js == nil {
		return TopicOptions{}, fmt.Errorf("JetStream not enabled")
	}

	streamName := n.getStreamNameForTopic(topic)

	// 获取Stream信息
	streamInfo, err := n.js.StreamInfo(streamName)
	if err != nil {
		return TopicOptions{}, fmt.Errorf("failed to get stream info: %w", err)
	}

	// 转换为TopicOptions
	actualConfig := TopicOptions{
		PersistenceMode: TopicPersistent,
		RetentionTime:   streamInfo.Config.MaxAge,
		MaxSize:         streamInfo.Config.MaxBytes,
		MaxMessages:     streamInfo.Config.MaxMsgs,
		Replicas:        streamInfo.Config.Replicas,
	}

	return actualConfig, nil
}

// SetTopicConfigStrategy 设置主题配置策略
func (n *natsEventBus) SetTopicConfigStrategy(strategy TopicConfigStrategy) {
	n.topicConfigsMu.Lock()
	defer n.topicConfigsMu.Unlock()
	n.topicConfigStrategy = strategy
	n.logger.Info("Topic config strategy updated", zap.String("strategy", string(strategy)))
}

// GetTopicConfigStrategy 获取当前主题配置策略
func (n *natsEventBus) GetTopicConfigStrategy() TopicConfigStrategy {
	n.topicConfigsMu.RLock()
	defer n.topicConfigsMu.RUnlock()
	return n.topicConfigStrategy
}
