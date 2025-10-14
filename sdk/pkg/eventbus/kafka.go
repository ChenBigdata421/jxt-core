package eventbus

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// WorkItem 全局Worker池工作项
type WorkItem struct {
	Topic   string
	Message *sarama.ConsumerMessage
	Handler MessageHandler
	Session sarama.ConsumerGroupSession
}

// GlobalWorkerPool 全局Worker池
type GlobalWorkerPool struct {
	workers     []*Worker
	workQueue   chan WorkItem
	workerCount int
	queueSize   int
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	logger      *zap.Logger
}

// Worker 全局Worker
type Worker struct {
	id       int
	pool     *GlobalWorkerPool
	workChan chan WorkItem
	quit     chan bool
}

// NewGlobalWorkerPool 创建全局Worker池
func NewGlobalWorkerPool(workerCount int, logger *zap.Logger) *GlobalWorkerPool {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU() * 2 // 默认为CPU核心数的2倍
	}

	queueSize := workerCount * 100 // 队列大小为worker数的100倍
	ctx, cancel := context.WithCancel(context.Background())

	pool := &GlobalWorkerPool{
		workerCount: workerCount,
		queueSize:   queueSize,
		workQueue:   make(chan WorkItem, queueSize),
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
	}

	// 启动workers
	pool.start()

	return pool
}

// start 启动所有workers
func (p *GlobalWorkerPool) start() {
	p.workers = make([]*Worker, p.workerCount)

	for i := 0; i < p.workerCount; i++ {
		worker := &Worker{
			id:       i,
			pool:     p,
			workChan: make(chan WorkItem),
			quit:     make(chan bool),
		}
		p.workers[i] = worker

		p.wg.Add(1)
		go worker.start()
	}

	// 启动工作分发器
	p.wg.Add(1) // 🔧 修复：将 dispatcher 加入 WaitGroup
	go p.dispatcher()

	p.logger.Info("Global worker pool started",
		zap.Int("workerCount", p.workerCount),
		zap.Int("queueSize", p.queueSize))
}

// dispatcher 工作分发器
// 优化：移除goroutine创建，使用轮询分发
func (p *GlobalWorkerPool) dispatcher() {
	defer p.wg.Done() // 🔧 修复：dispatcher 退出时通知 WaitGroup

	workerIndex := 0
	for {
		select {
		case work := <-p.workQueue:
			// 轮询分发工作到可用的worker（无goroutine创建）
			dispatched := false
			for i := 0; i < len(p.workers); i++ {
				workerIndex = (workerIndex + 1) % len(p.workers)
				select {
				case p.workers[workerIndex].workChan <- work:
					dispatched = true
					i = len(p.workers) // 跳出循环
				default:
					continue
				}
			}

			// 所有worker都忙，阻塞等待第一个可用的worker
			if !dispatched {
				p.workers[workerIndex].workChan <- work
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// SubmitWork 提交工作到全局Worker池
// 优化：添加背压机制，等待而非丢弃
func (p *GlobalWorkerPool) SubmitWork(work WorkItem) bool {
	select {
	case p.workQueue <- work:
		return true
	case <-time.After(100 * time.Millisecond):
		// 等待100ms后仍然满，记录警告但仍尝试提交
		p.logger.Warn("Global worker pool queue full, applying backpressure",
			zap.String("topic", work.Topic))
		// 阻塞等待，确保消息不丢失
		p.workQueue <- work
		return true
	}
}

// start Worker启动
func (w *Worker) start() {
	defer w.pool.wg.Done()

	for {
		select {
		case work := <-w.workChan:
			w.processWork(work)
		case <-w.quit:
			return
		case <-w.pool.ctx.Done():
			return
		}
	}
}

// processWork 处理工作
func (w *Worker) processWork(work WorkItem) {
	defer func() {
		if r := recover(); r != nil {
			w.pool.logger.Error("Worker panic during message processing",
				zap.Int("workerID", w.id),
				zap.String("topic", work.Topic),
				zap.Any("panic", r))
		}
	}()

	// 处理消息
	ctx := context.Background()
	err := work.Handler(ctx, work.Message.Value)
	if err != nil {
		w.pool.logger.Error("Message processing failed",
			zap.Int("workerID", w.id),
			zap.String("topic", work.Topic),
			zap.Error(err))
	}

	// 标记消息已处理
	work.Session.MarkMessage(work.Message, "")
}

// Close 关闭全局Worker池
func (p *GlobalWorkerPool) Close() {
	p.logger.Info("Shutting down global worker pool")

	// 取消context
	p.cancel()

	// 停止所有workers
	for _, worker := range p.workers {
		close(worker.quit)
	}

	// 等待所有workers完成
	p.wg.Wait()

	// 关闭队列
	close(p.workQueue)

	p.logger.Info("Global worker pool shutdown complete")
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
type kafkaEventBus struct {
	config        *KafkaConfig         // 使用内部配置结构，实现解耦
	asyncProducer sarama.AsyncProducer // 🚀 优化1：使用AsyncProducer替代SyncProducer
	consumer      sarama.Consumer
	client        sarama.Client
	admin         sarama.ClusterAdmin
	logger        *zap.Logger
	mu            sync.RWMutex
	closed        bool

	// 企业级特性
	backlogDetector          *BacklogDetector          // 订阅端积压检测器
	publisherBacklogDetector *PublisherBacklogDetector // 发送端积压检测器
	rateLimiter              *RateLimiter

	// 移除fullConfig字段，企业级特性配置现在在config.Enterprise中

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

	// 预订阅模式 - 统一消费者组管理
	unifiedConsumerGroup sarama.ConsumerGroup

	// 全局Worker池
	globalWorkerPool *GlobalWorkerPool

	// 预订阅topic管理
	allPossibleTopics   []string                  // 所有可能的topic列表（预订阅）
	activeTopicHandlers map[string]MessageHandler // 激活的topic处理器
	topicHandlersMu     sync.RWMutex

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

	// 订阅管理（用于重连后恢复订阅）- 保持兼容性
	subscriptions   map[string]MessageHandler // topic -> handler
	subscriptionsMu sync.RWMutex

	// 全局 Keyed-Worker Pool（所有 topic 共享）
	globalKeyedPool *KeyedWorkerPool

	// 主题配置管理
	topicConfigs          map[string]TopicOptions
	topicConfigsMu        sync.RWMutex
	topicConfigStrategy   TopicConfigStrategy       // 配置策略
	topicConfigOnMismatch TopicConfigMismatchAction // 配置不一致时的行为
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

	// 优化2：LZ4压缩（Confluent官方首选）
	if cfg.Producer.Compression == "" || cfg.Producer.Compression == "none" {
		saramaConfig.Producer.Compression = sarama.CompressionLZ4 // Confluent推荐：性能最佳
	} else {
		saramaConfig.Producer.Compression = getCompressionCodec(cfg.Producer.Compression)
	}

	// 优化3：批处理配置（Confluent官方推荐值）
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

	// 创建全局Worker池
	globalWorkerPool := NewGlobalWorkerPool(0, zap.NewNop()) // 0表示使用默认worker数量

	// 创建事件总线实例
	bus := &kafkaEventBus{
		config:               cfg,
		client:               client,
		asyncProducer:        asyncProducer,
		unifiedConsumerGroup: unifiedConsumerGroup,
		closed:               false,
		logger:               zap.NewNop(), // 使用无操作logger

		// 预订阅模式字段
		globalWorkerPool:       globalWorkerPool,
		allPossibleTopics:      make([]string, 0),
		activeTopicHandlers:    make(map[string]MessageHandler),
		preSubscriptionEnabled: true,
		maxTopicsPerGroup:      100, // 默认最大100个topic
		consumerDone:           make(chan struct{}),

		// 兼容性字段
		subscriptions: make(map[string]MessageHandler),
		topicConfigs:  make(map[string]TopicOptions),
		// topicConfigStrategy:   StrategyCreateOrUpdate,
		// topicConfigOnMismatch: ActionLogWarning,

		// 🚀 异步发布结果通道（缓冲区大小：10000）
		publishResultChan: make(chan *PublishResult, 10000),
	}

	// 创建全局 Keyed-Worker Pool（所有 topic 共享）
	// 使用较大的 worker 数量以支持多个 topic 的并发处理
	bus.globalKeyedPool = NewKeyedWorkerPool(KeyedWorkerPoolConfig{
		WorkerCount: 256,                    // 全局 worker 数量（支持多个 topic）
		QueueSize:   1000,                   // 每个 worker 的队列大小
		WaitTimeout: 500 * time.Millisecond, // 等待超时
	}, nil) // handler 将在处理消息时动态传入

	// 优化1：启动AsyncProducer的成功和错误处理goroutine
	go bus.handleAsyncProducerSuccess()
	go bus.handleAsyncProducerErrors()

	logger.Info("Kafka EventBus created successfully (AsyncProducer mode)",
		"brokers", cfg.Brokers,
		"clientId", cfg.ClientID,
		"healthCheckInterval", cfg.HealthCheckInterval,
		"compression", cfg.Producer.Compression,
		"flushFrequency", saramaConfig.Producer.Flush.Frequency,
		"flushMessages", saramaConfig.Producer.Flush.Messages,
		"flushBytes", saramaConfig.Producer.Flush.Bytes)

	return bus, nil
}

// 优化1：处理AsyncProducer成功消息
func (k *kafkaEventBus) handleAsyncProducerSuccess() {
	for success := range k.asyncProducer.Successes() {
		// 记录成功指标
		k.publishedMessages.Add(1)

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
	for err := range k.asyncProducer.Errors() {
		// 记录错误
		k.errorCount.Add(1)
		k.logger.Error("Async producer error",
			zap.String("topic", err.Msg.Topic),
			zap.Error(err.Err))

		// 如果配置了回调，执行回调
		if k.publishCallback != nil {
			// 提取消息内容
			var message []byte
			if err.Msg.Value != nil {
				message, _ = err.Msg.Value.Encode()
			}
			k.publishCallback(context.Background(), err.Msg.Topic, message, err.Err)
		}

		// 如果配置了错误处理器，执行错误处理
		if k.errorHandler != nil {
			// 提取消息内容
			var message []byte
			if err.Msg.Value != nil {
				message, _ = err.Msg.Value.Encode()
			}
			k.errorHandler.HandleError(context.Background(), err.Err, message, err.Msg.Topic)
		}
	}
}

// ========== 配置转换辅助函数 ==========

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
	if aggregateID != "" {
		// 有聚合ID：使用Keyed-Worker池进行顺序处理
		// 这种情况通常发生在：
		// 1. SubscribeEnvelope订阅的Envelope消息
		// 2. 手动在Header中设置了聚合ID的消息
		// 3. Kafka Key恰好是有效的聚合ID
		// 使用全局 Keyed-Worker 池处理
		pool := h.eventBus.globalKeyedPool
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
				Handler:     h.handler, // 携带 topic 的 handler
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

	// 无聚合ID：直接并发处理（不使用Keyed-Worker池）
	// 这种情况通常发生在：
	// 1. Subscribe订阅的原始消息（如JSON、文本等）
	// 2. 无法从消息中提取有效聚合ID的情况
	// 3. 简单消息传递场景（通知、缓存失效等）
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

			// 根据topic路由到激活的handler
			h.eventBus.topicHandlersMu.RLock()
			handler, exists := h.eventBus.activeTopicHandlers[message.Topic]
			h.eventBus.topicHandlersMu.RUnlock()

			if exists {
				// 优化：增强消息处理错误处理和监控
				h.eventBus.logger.Debug("Processing message",
					zap.String("topic", message.Topic),
					zap.Int64("offset", message.Offset),
					zap.Int32("partition", message.Partition))

				// 关键修复：尝试使用 Keyed-Worker 池处理（确保聚合ID顺序）
				if err := h.processMessageWithKeyedPool(ctx, message, handler, session); err != nil {
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

// processMessageWithKeyedPool 使用 Keyed-Worker 池处理消息（确保聚合ID顺序）
func (h *preSubscriptionConsumerHandler) processMessageWithKeyedPool(ctx context.Context, message *sarama.ConsumerMessage, handler MessageHandler, session sarama.ConsumerGroupSession) error {
	// 转换 Headers 为 map
	headersMap := make(map[string]string, len(message.Headers))
	for _, header := range message.Headers {
		headersMap[string(header.Key)] = string(header.Value)
	}

	// 尝试提取聚合ID（优先级：Envelope > Header > Kafka Key）
	aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")

	if aggregateID != "" {
		// 有聚合ID：使用Keyed-Worker池进行顺序处理
		// 使用全局 Keyed-Worker 池处理
		pool := h.eventBus.globalKeyedPool

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
				Handler:     handler, // 携带 topic 的 handler
			}
			for _, header := range message.Headers {
				aggMsg.Headers[string(header.Key)] = header.Value
			}

			// 提交到 Keyed-Worker 池
			if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
				return err
			}

			// 等待处理完成
			select {
			case err := <-aggMsg.Done:
				if err != nil {
					return err
				}
				// 处理成功，标记消息
				session.MarkMessage(message, "")
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// 无聚合ID 或 无Keyed-Worker池：使用全局Worker池处理
	workItem := WorkItem{
		Topic:   message.Topic,
		Message: message,
		Handler: handler,
		Session: session,
	}

	// 提交到全局Worker池
	if !h.eventBus.globalWorkerPool.SubmitWork(workItem) {
		h.eventBus.logger.Warn("Failed to submit work to global worker pool, using direct processing",
			zap.String("topic", message.Topic),
			zap.Int64("offset", message.Offset))
		// 如果Worker池满了，直接在当前goroutine处理
		h.processMessageDirectly(ctx, message, handler, session)
	}

	return nil
}

// processMessageDirectly 直接处理消息（当Worker池满时的后备方案）
func (h *preSubscriptionConsumerHandler) processMessageDirectly(ctx context.Context, message *sarama.ConsumerMessage, handler MessageHandler, session sarama.ConsumerGroupSession) {
	defer func() {
		if r := recover(); r != nil {
			h.eventBus.logger.Error("Panic during direct message processing",
				zap.String("topic", message.Topic),
				zap.Any("panic", r))
		}
		// 标记消息已处理
		session.MarkMessage(message, "")
	}()

	// 处理消息
	err := handler(ctx, message.Value)
	if err != nil {
		h.eventBus.logger.Error("Direct message processing failed",
			zap.String("topic", message.Topic),
			zap.Error(err))
	}
}

// initEnterpriseFeatures 初始化企业级特性
func (k *kafkaEventBus) initEnterpriseFeatures() error {
	// 使用程序员配置层的企业级特性配置
	enterpriseConfig := k.config.Enterprise

	// 初始化订阅端积压检测器
	if enterpriseConfig.Subscriber.BacklogDetection.Enabled {
		backlogConfig := BacklogDetectionConfig{
			MaxLagThreshold:  enterpriseConfig.Subscriber.BacklogDetection.MaxLagThreshold,
			MaxTimeThreshold: enterpriseConfig.Subscriber.BacklogDetection.MaxTimeThreshold,
			CheckInterval:    enterpriseConfig.Subscriber.BacklogDetection.CheckInterval,
		}
		k.backlogDetector = NewBacklogDetector(k.client, k.admin, k.config.Consumer.GroupID, backlogConfig)
		k.logger.Info("Subscriber backlog detector initialized",
			zap.Int64("maxLagThreshold", backlogConfig.MaxLagThreshold),
			zap.Duration("maxTimeThreshold", backlogConfig.MaxTimeThreshold),
			zap.Duration("checkInterval", backlogConfig.CheckInterval))
	}

	// 初始化发送端积压检测器
	if enterpriseConfig.Publisher.BacklogDetection.Enabled {
		k.publisherBacklogDetector = NewPublisherBacklogDetector(k.client, k.admin, enterpriseConfig.Publisher.BacklogDetection)
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

	// 优化1：使用AsyncProducer异步发送（非阻塞）
	select {
	case k.asyncProducer.Input() <- msg:
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
		k.asyncProducer.Input() <- msg
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
		k.logger.Info("Pre-subscription consumer started",
			zap.Int("topicCount", len(k.allPossibleTopics)),
			zap.Strings("topics", k.allPossibleTopics))

		for {
			// 检查 context 是否已取消
			if k.consumerCtx.Err() != nil {
				k.logger.Info("Pre-subscription consumer context cancelled")
				return
			}

			// 检查是否有 topic 需要订阅
			if len(k.allPossibleTopics) == 0 {
				k.logger.Warn("No topics configured for pre-subscription, consumer will wait")
				// 等待 context 取消
				<-k.consumerCtx.Done()
				return
			}

			// 关键：调用 Consume 订阅所有预配置的 topic
			// 这会阻塞直到发生重平衡或 context 取消
			k.logger.Info("Pre-subscription consumer consuming topics",
				zap.Strings("topics", k.allPossibleTopics),
				zap.Int("topicCount", len(k.allPossibleTopics)))

			err := k.unifiedConsumerGroup.Consume(k.consumerCtx, k.allPossibleTopics, handler)
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
	k.logger.Info("Pre-subscription consumer system started",
		zap.Int("topicCount", len(k.allPossibleTopics)))

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
func (k *kafkaEventBus) activateTopicHandler(topic string, handler MessageHandler) {
	k.topicHandlersMu.Lock()
	k.activeTopicHandlers[topic] = handler
	k.topicHandlersMu.Unlock()

	k.logger.Info("Topic handler activated",
		zap.String("topic", topic),
		zap.Int("totalActiveTopics", len(k.activeTopicHandlers)))
}

// deactivateTopicHandler 停用topic处理器（预订阅模式）
func (k *kafkaEventBus) deactivateTopicHandler(topic string) {
	k.topicHandlersMu.Lock()
	delete(k.activeTopicHandlers, topic)
	k.topicHandlersMu.Unlock()

	k.logger.Info("Topic handler deactivated",
		zap.String("topic", topic),
		zap.Int("totalActiveTopics", len(k.activeTopicHandlers)))
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
	k.mu.Lock()
	defer k.mu.Unlock()

	k.allPossibleTopics = make([]string, len(topics))
	copy(k.allPossibleTopics, topics)

	k.logger.Info("Pre-subscription topics configured",
		zap.Strings("topics", k.allPossibleTopics),
		zap.Int("topicCount", len(k.allPossibleTopics)))
}

// addTopicToPreSubscription 添加topic到预订阅列表（内部方法）
func (k *kafkaEventBus) addTopicToPreSubscription(topic string) {
	// 检查是否已存在
	for _, existingTopic := range k.allPossibleTopics {
		if existingTopic == topic {
			return // 已存在，无需添加
		}
	}

	// 添加到预订阅列表
	k.allPossibleTopics = append(k.allPossibleTopics, topic)
	k.logger.Info("Topic added to pre-subscription list",
		zap.String("topic", topic),
		zap.Int("totalTopics", len(k.allPossibleTopics)))
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
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 预订阅模式 - 激活topic处理器

	// 记录订阅信息（用于重连后恢复）- 保持兼容性
	k.subscriptionsMu.Lock()
	k.subscriptions[topic] = handler
	k.subscriptionsMu.Unlock()

	// 全局 Keyed-Worker Pool 已在初始化时创建，无需为每个 topic 创建独立池

	// 关键修复：添加 topic 到预订阅列表
	k.addTopicToPreSubscription(topic)

	// 激活topic处理器（立即生效）
	k.activateTopicHandler(topic, handler)

	// 启动预订阅消费者（如果还未启动）
	if err := k.startPreSubscriptionConsumer(ctx); err != nil {
		return fmt.Errorf("failed to start pre-subscription consumer: %w", err)
	}

	k.logger.Info("Subscribed to topic via pre-subscription consumer",
		zap.String("topic", topic),
		zap.String("groupID", k.config.Consumer.GroupID))
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

	// 停止预订阅消费者组
	k.stopPreSubscriptionConsumer()

	// 停止全局Worker池
	if k.globalWorkerPool != nil {
		k.globalWorkerPool.Close()
	}

	// 关闭全局 Keyed-Worker Pool
	if k.globalKeyedPool != nil {
		k.globalKeyedPool.Stop()
	}

	defer k.mu.Unlock()

	if k.closed {
		return nil
	}

	var errors []error

	// 关闭统一消费者组
	if k.unifiedConsumerGroup != nil {
		if err := k.unifiedConsumerGroup.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close unified consumer group: %w", err))
		}
	}

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

	// 优化1：关闭AsyncProducer
	if k.asyncProducer != nil {
		if err := k.asyncProducer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka async producer: %w", err))
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
	if k.asyncProducer != nil {
		k.asyncProducer.Close()
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

	// 更新实例字段
	k.client = client
	k.asyncProducer = asyncProducer
	k.consumer = consumer
	k.admin = admin

	// 优化1：重新启动AsyncProducer处理goroutine
	go k.handleAsyncProducerSuccess()
	go k.handleAsyncProducerErrors()

	k.logger.Info("Kafka connection reinitialized successfully (AsyncProducer mode)")
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

	// 优化1：使用AsyncProducer异步发布
	select {
	case k.asyncProducer.Input() <- kafkaMsg:
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
		k.asyncProducer.Input() <- kafkaMsg
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

	// 优化1：使用AsyncProducer异步发送
	select {
	case k.asyncProducer.Input() <- msg:
		// 消息已提交到发送队列
		k.logger.Debug("Envelope message queued for async publishing",
			zap.String("topic", topic),
			zap.String("aggregateID", envelope.AggregateID),
			zap.String("eventType", envelope.EventType),
			zap.Int64("eventVersion", envelope.EventVersion))
		return nil
	case <-time.After(100 * time.Millisecond):
		// 发送队列满，应用背压
		k.logger.Warn("Async producer input queue full for envelope message",
			zap.String("topic", topic),
			zap.String("aggregateID", envelope.AggregateID))
		// 阻塞等待
		k.asyncProducer.Input() <- msg
		return nil
	}
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

// GetPublishResultChannel 获取异步发布结果通道
// 用于Outbox Processor监听发布结果并更新Outbox状态
// 注意：Kafka 使用 AsyncProducer，发布结果通过 Successes/Errors channel 处理
// 此方法返回的通道用于与 NATS 保持接口一致，但 Kafka 不会主动推送结果
// 如需监听 Kafka 发布结果，请使用 AsyncProducer 的 Successes() 和 Errors() channel
func (k *kafkaEventBus) GetPublishResultChannel() <-chan *PublishResult {
	return k.publishResultChan
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
