package eventbus

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// natsEventBus NATS JetStream事件总线实现
// 企业级增强版本，专注于JetStream持久化消息
// 支持方案A（Envelope）消息包络
// 🔥 优化架构：1个连接，1个JetStream Context，1个Consumer，多个Pull Subscription
// 🔥 免锁优化版本 - 参考 Kafka EventBus 优化方案
type natsEventBus struct {
	// 🔥 P0修复：改为 atomic.Value（发布时无锁读取）
	conn atomic.Value // stores *nats.Conn
	js   atomic.Value // stores nats.JetStreamContext

	config        *NATSConfig // 使用内部配置结构，实现解耦
	subscriptions map[string]*nats.Subscription
	logger        *zap.Logger

	// ✅ 低频路径：保留 mu（用于 Subscribe、Close 等低频操作）
	mu     sync.Mutex  // 🔥 改为 Mutex（不再需要读写锁）
	closed atomic.Bool // 🔥 P0修复：改为 atomic.Bool，热路径无锁读取

	// PR2-core (Task 3, spec §3.3): stable terminal error on concurrent/repeated Close.
	// closeOnce guarantees teardown runs exactly once; closeDone is closed when teardown
	// completes so every waiting repeat-Close caller converges on the same terminalErr.
	// terminalErr is written once inside closeOnce.Do and read only after <-closeDone.
	closeOnce   sync.Once
	closeDone   chan struct{}
	terminalErr error

	reconnectCallbacks []func(ctx context.Context) error

	// 🔥 统一Consumer管理 - 优化架构
	unifiedConsumer nats.ConsumerInfo // 单一Consumer

	// 🔥 P0修复：改为 sync.Map（消息路由时无锁查找）
	topicHandlers sync.Map // key: string (topic), value: MessageHandler

	// ✅ 低频路径：保留 slice + mu（订阅是低频操作）
	subscribedTopics   []string
	subscribedTopicsMu sync.Mutex // 🔥 改为 Mutex

	// 企业级特性
	publishedMessages atomic.Int64
	consumedMessages  atomic.Int64
	errorCount        atomic.Int64
	lastHealthCheck   atomic.Value // time.Time
	healthStatus      atomic.Bool

	// ⭐ Actor Pool 迁移：Round-Robin 计数器（用于普通消息）
	roundRobinCounter atomic.Uint64

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
	subscriptionHandlers map[string]*handlerWrapper // ⭐ 修改：topic -> handlerWrapper（支持 at-least-once）
	subscriptionsMu      sync.RWMutex

	// 积压检测器
	backlogDetector          *NATSBacklogDetector      // 订阅端积压检测器
	publisherBacklogDetector *PublisherBacklogDetector // 发送端积压检测器

	// 移除fullConfig字段，企业级特性配置现在在config.Enterprise中

	// 🔥 Hollywood Actor Pool（所有 topic 共享，与 Kafka 保持一致）
	// 直接使用 Hollywood Actor Pool，无需配置开关
	actorPool *HollywoodActorPool

	// 🔥 P1优化：主题配置管理改为 sync.Map（无锁读取）
	topicConfigs          sync.Map                  // key: string (topic), value: TopicOptions
	topicConfigStrategy   TopicConfigStrategy       // 配置策略
	topicConfigOnMismatch TopicConfigMismatchAction // 配置不一致时的行为
	topicConfigStrategyMu sync.RWMutex              // 🔥 P1优化：保护 topicConfigStrategy 和 topicConfigOnMismatch

	// 🔥 P0修复：改为 sync.Map（发布时无锁读取）
	createdStreams sync.Map // key: string (streamName), value: bool

	// 🔥 P1优化：单飞抑制（防止并发创建 Stream 风暴）
	streamCreateGroup singleflight.Group

	// 健康检查订阅监控器
	healthCheckSubscriber *HealthCheckSubscriber
	// 健康检查发布器
	healthChecker *HealthChecker
	// 健康检查配置（从 Enterprise.HealthCheck 转换而来）
	healthCheckConfig config.HealthCheckConfig

	// 异步发布结果通道（用于Outbox模式）
	publishResultChan chan *PublishResult
	// 异步发布结果处理控制
	publishResultWg     sync.WaitGroup
	publishResultCancel context.CancelFunc
	// 是否启用发布结果通道（性能优化：默认禁用）
	enablePublishResult bool

	// 多租户 ACK 通道支持
	tenantPublishResultChans map[int]chan *PublishResult // key: tenantID, value: ACK channel
	tenantChannelsMu         sync.RWMutex                // 保护 tenantPublishResultChans 的读写锁

	// ✅ 方案2：共享 ACK 处理器（避免 per-message goroutine）
	ackChan        chan *ackTask  // ACK 任务通道
	ackWorkerWg    sync.WaitGroup // ACK worker 等待组
	ackWorkerStop  chan struct{}  // ACK worker 停止信号
	ackWorkerCount int            // ACK worker 数量（可配置）
}

// ackTask ACK 处理任务
type ackTask struct {
	future      nats.PubAckFuture
	eventID     string
	topic       string
	aggregateID string
	eventType   string
	tenantID    int // 租户ID（多租户支持，用于Outbox ACK路由）
}

// NewNATSEventBus 创建NATS JetStream事件总线
// 使用内部配置结构，实现配置解耦
func NewNATSEventBus(config *NATSConfig) (EventBus, error) {
	if config == nil {
		return nil, fmt.Errorf("nats config cannot be nil")
	}

	if len(config.URLs) == 0 {
		return nil, fmt.Errorf("nats URLs cannot be empty")
	}

	// 构建连接选项
	opts := buildNATSOptionsInternal(config)

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

	// 创建JetStream上下文（配置异步发布优化）
	var js nats.JetStreamContext
	if config.JetStream.Enabled {
		// ✅ 优化 1: 配置异步发布选项
		jsOpts := []nats.JSOpt{
			// ✅ 优化：增加未确认消息数量限制到 100000
			// 从 50000 增加到 100000，减少极限场景阻塞概率
			nats.PublishAsyncMaxPending(100000),
		}

		js, err = nc.JetStream(jsOpts...)
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to create JetStream context: %w", err)
		}
	}

	// 转换健康检查配置（从 eventbus.HealthCheckConfig 转换为 config.HealthCheckConfig）
	healthCheckConfig := convertHealthCheckConfig(config.Enterprise.HealthCheck)

	// 创建事件总线实例
	bus := &natsEventBus{
		config:        config,
		subscriptions: make(map[string]*nats.Subscription),
		// 临时开启开发日志便于定位问题（后续可改回 zap.NewNop()）
		logger:             zap.NewExample(),
		reconnectCallbacks: make([]func(ctx context.Context) error, 0),
		// 🔥 P1优化：topicConfigs 改为 sync.Map，不需要初始化
		// topicConfigs: sync.Map 零值可用
		// 健康检查配置
		healthCheckConfig: healthCheckConfig,
		// 🔥 P0修复：topicHandlers 改为 sync.Map，不需要初始化
		// topicHandlers: sync.Map 零值可用
		subscribedTopics:     make([]string, 0),
		subscriptionHandlers: make(map[string]*handlerWrapper), // ⭐ 修改类型
		// 🚀 初始化异步发布结果通道（缓冲区大小：100000）
		publishResultChan: make(chan *PublishResult, 100000),
		// 🔥 P0修复：createdStreams 改为 sync.Map，不需要初始化
		// createdStreams: sync.Map 零值可用
		// 🔥 P1优化：streamCreateGroup 零值可用，不需要初始化
		// ✅ 方案2：初始化 ACK 处理器
		ackChan:        make(chan *ackTask, 100000), // ACK 任务通道（大缓冲区）
		ackWorkerStop:  make(chan struct{}),
		ackWorkerCount: runtime.NumCPU() * 2, // 🔥 P1验证：默认 CPU核心数 * 2（已验证合理）
		// PR2-core (Task 3): closeDone closed at end of Close() teardown to release
		// concurrent/repeat callers. closeOnce/terminalErr use correct zero values.
		closeDone: make(chan struct{}),
	}

	// 🔥 P0修复：使用 atomic.Value 存储连接对象
	bus.conn.Store(nc)
	// 🔥 P0修复：只在 JetStream 启用时才存储 js，避免存储 nil 导致 panic
	if js != nil {
		bus.js.Store(js)
	}
	bus.closed.Store(false)

	// 🔥 创建 Hollywood Actor Pool（所有 topic 共享，与 Kafka 保持一致）
	// 直接使用 Hollywood Actor Pool，无需配置开关
	// 使用 ClientID 作为命名空间，确保每个实例的指标不冲突
	// 注意：Prometheus 指标名称只能包含 [a-zA-Z0-9_]，需要替换 - 为 _
	metricsNamespace := fmt.Sprintf("nats_eventbus_%s", strings.ReplaceAll(config.ClientID, "-", "_"))
	actorPoolMetrics := NewPrometheusActorPoolMetricsCollector(metricsNamespace)

	bus.actorPool = NewHollywoodActorPool(HollywoodActorPoolConfig{
		PoolSize:    256,  // 固定 Actor 数量（与 Kafka 一致）
		InboxSize:   1000, // Inbox 队列大小
		MaxRestarts: 3,    // Supervisor 最大重启次数
	}, actorPoolMetrics)

	bus.logger.Info("NATS EventBus using Hollywood Actor Pool",
		zap.Int("poolSize", 256),
		zap.Int("inboxSize", 1000),
		zap.Int("maxRestarts", 3))

	// ✅ 重构：配置全局异步发布处理器（业界最佳实践）
	if config.JetStream.Enabled && js != nil {
		// 重新创建JetStream上下文，添加全局错误处理器
		jsOpts := []nats.JSOpt{
			// ✅ 优化：增加未确认消息数量限制到 100000
			// 从 50000 增加到 100000，减少极限场景阻塞概率
			// 注意：这个值需要根据实际场景调整
			// - 低并发场景：256 足够
			// - 高并发场景：需要更大的值（如 10000）
			// - 极限场景：需要非常大的值（如 100000）
			nats.PublishAsyncMaxPending(100000),

			// ✅ 全局错误处理器（业界最佳实践）
			// 只处理错误，成功的 ACK 由 NATS 内部自动处理
			// 这样可以避免为每条消息创建 goroutine
			nats.PublishAsyncErrHandler(func(js nats.JetStream, originalMsg *nats.Msg, err error) {
				bus.errorCount.Add(1)
				bus.logger.Error("Async publish failed (global handler)",
					zap.String("subject", originalMsg.Subject),
					zap.Int("dataSize", len(originalMsg.Data)),
					zap.Error(err))
			}),
		}

		// 重新创建带错误处理器的JetStream上下文
		js, err = nc.JetStream(jsOpts...)
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to create JetStream context with error handler: %w", err)
		}
		// 🔥 P0修复：使用 atomic.Value 存储
		bus.js.Store(js)

		bus.logger.Info("NATS JetStream configured with global async publish handler",
			zap.Int("maxPending", 100000))
	}

	bus.logger.Info("NATS EventBus created successfully",
		zap.String("urls", fmt.Sprintf("%v", config.URLs)),
		zap.String("clientId", config.ClientID))

	// 🔥 初始化统一Consumer（如果启用JetStream）
	if config.JetStream.Enabled {
		// 首先确保Stream存在
		if err := bus.ensureStreamExists(); err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to ensure stream exists: %w", err)
		}

		if err := bus.initUnifiedConsumer(); err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to initialize unified consumer: %w", err)
		}

		// ✅ 方案2：启动 ACK worker 池
		bus.startACKWorkers()
		bus.logger.Info("NATS ACK worker pool started",
			zap.Int("workerCount", bus.ackWorkerCount),
			zap.Int("ackChanSize", cap(bus.ackChan)),
			zap.Int("resultChanSize", cap(bus.publishResultChan)))
	}

	return bus, nil
}

// 🔥 P0修复：Helper 方法 - 无锁读取 NATS Connection
func (n *natsEventBus) getConn() (*nats.Conn, error) {
	connAny := n.conn.Load()
	if connAny == nil {
		return nil, fmt.Errorf("nats connection not initialized")
	}
	conn, ok := connAny.(*nats.Conn)
	if !ok {
		return nil, fmt.Errorf("invalid nats connection type")
	}
	return conn, nil
}

// 🔥 P0修复：Helper 方法 - 无锁读取 JetStream Context
func (n *natsEventBus) getJetStreamContext() (nats.JetStreamContext, error) {
	jsAny := n.js.Load()
	if jsAny == nil {
		return nil, fmt.Errorf("jetstream context not initialized")
	}
	js, ok := jsAny.(nats.JetStreamContext)
	if !ok {
		return nil, fmt.Errorf("invalid jetstream context type")
	}
	return js, nil
}

// ensureStreamExists 确保配置的Stream存在
func (n *natsEventBus) ensureStreamExists() error {
	// 🔥 P0修复：无锁读取 JetStream Context
	js, err := n.getJetStreamContext()
	if err != nil {
		return err
	}

	streamName := n.config.JetStream.Stream.Name
	if streamName == "" {
		return fmt.Errorf("stream name not configured")
	}

	// 🔥 P0修复：使用 js 变量而不是 n.js
	// 检查Stream是否已存在
	_, err = js.StreamInfo(streamName)
	if err == nil {
		// Stream已存在
		n.logger.Info("JetStream stream already exists", zap.String("stream", streamName))

		// 🔥 P0修复：使用 sync.Map 存储
		n.createdStreams.Store(streamName, true)

		return nil
	}

	// Stream不存在，创建新的
	streamConfig := &nats.StreamConfig{
		Name:      streamName,
		Subjects:  n.config.JetStream.Stream.Subjects,
		Retention: parseRetentionPolicy(n.config.JetStream.Stream.Retention),
		Storage:   parseStorageType(n.config.JetStream.Stream.Storage),
		Replicas:  n.config.JetStream.Stream.Replicas,
		MaxAge:    n.config.JetStream.Stream.MaxAge,
		MaxBytes:  n.config.JetStream.Stream.MaxBytes,
		MaxMsgs:   n.config.JetStream.Stream.MaxMsgs,
		Discard:   parseDiscardPolicy(n.config.JetStream.Stream.Discard),
	}

	_, err = js.AddStream(streamConfig)
	if err != nil {
		return fmt.Errorf("failed to create stream %s: %w", streamName, err)
	}

	n.logger.Info("Created JetStream stream",
		zap.String("stream", streamName),
		zap.Strings("subjects", streamConfig.Subjects),
		zap.String("storage", streamConfig.Storage.String()))

	// 🔥 P0修复：使用 sync.Map 存储
	n.createdStreams.Store(streamName, true)

	return nil
}

// subjectMatches 检查 NATS subject pattern 是否匹配 topic
// 支持通配符：* (匹配单个 token), > (匹配多个 token)
func (n *natsEventBus) subjectMatches(pattern, topic string) bool {
	// 完全匹配
	if pattern == topic {
		return true
	}

	// 分割 pattern 和 topic
	patternTokens := strings.Split(pattern, ".")
	topicTokens := strings.Split(topic, ".")

	// 检查通配符匹配
	pi, ti := 0, 0
	for pi < len(patternTokens) && ti < len(topicTokens) {
		pToken := patternTokens[pi]
		tToken := topicTokens[ti]

		if pToken == ">" {
			// > 匹配剩余所有 tokens
			return true
		} else if pToken == "*" {
			// * 匹配单个 token
			pi++
			ti++
		} else if pToken == tToken {
			// 精确匹配
			pi++
			ti++
		} else {
			// 不匹配
			return false
		}
	}

	// 检查是否完全匹配
	return pi == len(patternTokens) && ti == len(topicTokens)
}

// ensureTopicStreamExists 确保 topic 专用的 Stream 存在（使用指定的存储类型）
func (n *natsEventBus) ensureTopicStreamExists(js nats.JetStreamContext, streamName, topic string, storageType nats.StorageType) error {
	// 检查 Stream 是否已存在
	streamInfo, err := js.StreamInfo(streamName)
	if err == nil {
		// Stream 已存在，检查存储类型是否匹配
		if streamInfo.Config.Storage != storageType {
			n.logger.Warn("Stream storage type mismatch",
				zap.String("stream", streamName),
				zap.String("expected", storageType.String()),
				zap.String("actual", streamInfo.Config.Storage.String()))
			// 注意：NATS 不支持修改已存在 Stream 的存储类型
			// 如果需要修改，必须先删除 Stream 再重建（会丢失数据）
			// 这里我们选择使用已存在的 Stream
		}
		n.logger.Info("JetStream stream already exists",
			zap.String("stream", streamName),
			zap.String("topic", topic),
			zap.String("storage", streamInfo.Config.Storage.String()))
		n.createdStreams.Store(streamName, true)
		return nil
	}

	// Stream 不存在，创建新的
	streamConfig := &nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{topic}, // ⭐ 使用 topic 作为 subject
		Retention: parseRetentionPolicy(n.config.JetStream.Stream.Retention),
		Storage:   storageType, // ⭐ 使用指定的存储类型
		Replicas:  n.config.JetStream.Stream.Replicas,
		MaxAge:    n.config.JetStream.Stream.MaxAge,
		MaxBytes:  n.config.JetStream.Stream.MaxBytes,
		MaxMsgs:   n.config.JetStream.Stream.MaxMsgs,
		Discard:   parseDiscardPolicy(n.config.JetStream.Stream.Discard),
	}

	_, err = js.AddStream(streamConfig)
	if err != nil {
		return fmt.Errorf("failed to create stream %s: %w", streamName, err)
	}

	n.logger.Info("Created JetStream stream for topic",
		zap.String("stream", streamName),
		zap.String("topic", topic),
		zap.String("storage", storageType.String()))

	n.createdStreams.Store(streamName, true)
	return nil
}

// 🔥 initUnifiedConsumer 初始化统一Consumer
func (n *natsEventBus) initUnifiedConsumer() error {
	// 🔥 P0修复：无锁读取 JetStream Context
	js, err := n.getJetStreamContext()
	if err != nil {
		return err
	}

	// 构建统一Consumer配置
	durableName := fmt.Sprintf("%s-unified", n.config.JetStream.Consumer.DurableName)

	// 不设置FilterSubject，让每个Pull Subscription自己指定subject过滤
	// 这样一个统一的Consumer可以支持多个不同的topic订阅

	// 🔥 优化 ACK 机制：增加 AckWait，减少 MaxAckPending
	ackWait := n.config.JetStream.AckWait
	if ackWait <= 0 {
		ackWait = 60 * time.Second // 默认 60 秒（从 30 秒增加）
	}

	maxAckPending := n.config.JetStream.Consumer.MaxAckPending
	if maxAckPending <= 0 || maxAckPending > 1000 {
		maxAckPending = 1000 // 默认 1000（从 65536 减少）
	}

	consumerConfig := &nats.ConsumerConfig{
		Durable:       durableName,
		DeliverPolicy: parseDeliverPolicy(n.config.JetStream.Consumer.DeliverPolicy),
		AckPolicy:     parseAckPolicy(n.config.JetStream.Consumer.AckPolicy),
		ReplayPolicy:  parseReplayPolicy(n.config.JetStream.Consumer.ReplayPolicy),
		AckWait:       ackWait,
		MaxAckPending: maxAckPending,
		MaxWaiting:    n.config.JetStream.Consumer.MaxWaiting,
		MaxDeliver:    n.config.JetStream.Consumer.MaxDeliver,
		BackOff:       n.config.JetStream.Consumer.BackOff,
		// FilterSubject留空，允许多个topic订阅
	}

	// 🔥 P0修复：使用 js 变量
	// 创建统一Consumer
	consumer, err := js.AddConsumer(n.config.JetStream.Stream.Name, consumerConfig)
	if err != nil {
		return fmt.Errorf("failed to create unified consumer: %w", err)
	}

	n.unifiedConsumer = *consumer
	n.logger.Info("Unified NATS consumer initialized",
		zap.String("durableName", durableName),
		zap.String("stream", n.config.JetStream.Stream.Name))

	return nil
}

// buildNATSOptionsInternal 构建NATS连接选项（内部配置版本）
func buildNATSOptionsInternal(config *NATSConfig) []nats.Option {
	var opts []nats.Option

	// 基础配置
	if config.ClientID != "" {
		opts = append(opts, nats.Name(config.ClientID))
	}

	if config.MaxReconnects > 0 {
		opts = append(opts, nats.MaxReconnects(config.MaxReconnects))
	}

	if config.ReconnectWait > 0 {
		opts = append(opts, nats.ReconnectWait(config.ReconnectWait))
	}

	if config.ConnectionTimeout > 0 {
		opts = append(opts, nats.Timeout(config.ConnectionTimeout))
	}

	// ✅ 修复问题1: 增加写入刷新超时配置（防止 I/O timeout）
	// 默认10秒，高压场景下足够处理TCP写缓冲区满的情况
	opts = append(opts, nats.FlusherTimeout(10*time.Second))

	// ✅ 修复问题1: 增加心跳配置（保持连接活跃）
	opts = append(opts, nats.PingInterval(20*time.Second))

	// ✅ 修复问题1: 增加重连缓冲区大小（默认32KB -> 1MB）
	// 高并发场景下可以缓冲更多待发送消息
	opts = append(opts, nats.ReconnectBufSize(1024*1024))

	// 安全配置
	if config.Security.Enabled {
		if config.Security.Username != "" && config.Security.Password != "" {
			opts = append(opts, nats.UserInfo(config.Security.Username, config.Security.Password))
		}

		if config.Security.CertFile != "" && config.Security.KeyFile != "" {
			opts = append(opts, nats.ClientCert(config.Security.CertFile, config.Security.KeyFile))
		}

		if config.Security.CAFile != "" {
			opts = append(opts, nats.RootCAs(config.Security.CAFile))
		}
	}

	return opts
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

// Publish 发布普通消息到指定主题
// ⚠️ 注意：不支持 Outbox 模式，消息容许丢失
// 适用场景：通知、缓存失效、系统事件等可容忍丢失的消息
// 如需可靠投递和 Outbox 模式支持，请使用 PublishEnvelope()
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
	start := time.Now()

	// 🔥 P0修复：无锁检查关闭状态
	if n.closed.Load() {
		return fmt.Errorf("eventbus is closed")
	}

	// 获取主题配置
	topicConfig, _ := n.GetTopicConfig(topic)

	// 决定发布模式：优先使用主题配置，其次使用全局配置
	shouldUsePersistent := topicConfig.IsPersistent(n.config.JetStream.Enabled)

	var err error

	// 🔥 P0修复：无锁读取 JetStream Context
	js, jsErr := n.getJetStreamContext()
	jsAvailable := jsErr == nil

	if shouldUsePersistent && jsAvailable {
		// ✅ Stream预创建优化：根据策略决定是否检查Stream
		// 策略说明：
		// - StrategySkip: 跳过检查（性能最优，适用于预创建场景）
		// - 其他策略: 检查Stream是否存在（兼容动态创建场景）
		shouldCheckStream := n.topicConfigStrategy != StrategySkip

		// 🔥 P0修复：无锁检查本地缓存（使用 sync.Map）
		streamName := n.getStreamNameForTopic(topic)
		_, streamExists := n.createdStreams.Load(streamName)

		// 只有在需要检查且缓存中不存在时，才调用ensureTopicInJetStream
		if shouldCheckStream && !streamExists {
			// 确保主题在JetStream中存在（如果需要持久化）
			if err := n.ensureTopicInJetStream(topic, topicConfig); err != nil {
				n.logger.Warn("Failed to ensure topic in JetStream, falling back to Core NATS",
					zap.String("topic", topic),
					zap.Error(err))
				// 降级到Core NATS
				shouldUsePersistent = false
			} else {
				// 🔥 P0修复：成功创建/验证Stream后，添加到本地缓存（使用 sync.Map）
				n.createdStreams.Store(streamName, true)
			}
		}
	}

	if shouldUsePersistent && jsAvailable {
		// ✅ 优化 1: 使用JetStream异步发布（持久化）
		// 注意：不要同时设置 Context 和 Timeout，否则 nats 会报错
		// 这里不设置 AckWait，采用全局 PublishAsyncErrHandler 处理失败 ACK
		// 需要自定义超时时，可改为同步 Publish 并传入 nats.Context(ctx) 或 nats.AckWait，但二者不可同时设置

		// 🔥 P0修复：使用 js 变量
		// ✅ 异步发布（不等待ACK，由统一错误处理器处理失败）
		_, err = js.PublishAsync(topic, message)
		if err != nil {
			n.errorCount.Add(1)
			n.logger.Error("Failed to publish message to NATS JetStream",
				zap.String("topic", topic),
				zap.Bool("persistent", true),
				zap.String("persistenceMode", string(topicConfig.PersistenceMode)),
				zap.Error(err))
			return err
		}
		// ✅ 成功的ACK由NATS内部自动处理
		// ✅ 错误的ACK由PublishAsyncErrHandler统一处理
	} else {
		// 🔥 P0修复：无锁读取 NATS Connection
		conn, connErr := n.getConn()
		if connErr != nil {
			return fmt.Errorf("failed to get nats connection: %w", connErr)
		}

		// 使用Core NATS发布（非持久化）
		err = conn.Publish(topic, message)
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
				// 🔥 P0修复：检查 JetStream 是否可用
				_, jsErr := n.getJetStreamContext()
				if n.config.JetStream.Enabled && jsErr == nil {
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
	n.logger.Debug("Subscribe called",
		zap.String("topic", topic))

	n.mu.Lock()
	defer n.mu.Unlock()

	// 🔥 P0修复：无锁检查关闭状态
	if n.closed.Load() {
		return fmt.Errorf("eventbus is closed")
	}

	// 检查是否已经订阅了该主题
	if _, exists := n.subscriptions[topic]; exists {
		return fmt.Errorf("already subscribed to topic: %s", topic)
	}

	// ⭐ 包装 handler 为 handlerWrapper（at-most-once 语义）
	wrapper := &handlerWrapper{
		handler:    handler,
		isEnvelope: false, // 普通 Subscribe：at-most-once 语义
	}

	// 保存订阅处理器（用于重连后恢复）
	n.subscriptionsMu.Lock()
	n.subscriptionHandlers[topic] = wrapper
	n.subscriptionsMu.Unlock()

	// 根据配置选择订阅模式
	var err error

	// 🔥 P0修复：无锁读取 JetStream Context
	_, jsErr := n.getJetStreamContext()
	jsAvailable := jsErr == nil

	n.logger.Error("🔥 SUBSCRIPTION MODE CHECK",
		zap.Bool("jetStreamEnabled", n.config.JetStream.Enabled),
		zap.Bool("jsAvailable", jsAvailable))

	if n.config.JetStream.Enabled && jsAvailable {
		// 使用JetStream订阅（持久化）
		n.logger.Debug("Using JetStream subscription",
			zap.String("topic", topic))
		err = n.subscribeJetStream(ctx, topic, handler, false) // ⭐ 普通 Subscribe：at-most-once
	} else {
		// 使用Core NATS订阅（非持久化）
		n.logger.Error("🔥 USING CORE NATS SUBSCRIPTION",
			zap.String("topic", topic))
		msgHandler := func(msg *nats.Msg) {
			n.handleMessageWithWrapper(ctx, topic, msg.Data, wrapper, func() error {
				return nil // Core NATS不需要手动确认
			}, func() error {
				return nil // Core NATS不支持 Nak
			})
		}

		// 🔥 P0修复：无锁读取 NATS Connection
		conn, connErr := n.getConn()
		if connErr != nil {
			return fmt.Errorf("failed to get nats connection: %w", connErr)
		}

		sub, err := conn.Subscribe(topic, msgHandler)
		if err != nil {
			return fmt.Errorf("failed to subscribe to topic %s with Core NATS: %w", topic, err)
		}

		n.subscriptions[topic] = sub
	}

	if err != nil {
		return err
	}

	n.logger.Info("Subscribed to NATS topic",
		zap.String("topic", topic),
		zap.Bool("persistent", n.config.JetStream.Enabled),
		zap.String("mode", func() string {
			// 🔥 P0修复：检查 JetStream 是否可用
			_, jsErr := n.getJetStreamContext()
			if n.config.JetStream.Enabled && jsErr == nil {
				return "JetStream"
			}
			return "Core"
		}()))

	return nil
}

// subscribeJetStream 使用统一Consumer和Pull Subscription订阅
// 默认使用配置文件中的存储类型
func (n *natsEventBus) subscribeJetStream(ctx context.Context, topic string, handler MessageHandler, isEnvelope bool) error {
	// 根据 isEnvelope 决定存储类型
	var storageType nats.StorageType
	if isEnvelope {
		// SubscribeEnvelope: 使用 file storage（at-least-once）
		storageType = nats.FileStorage
	} else {
		// Subscribe: 使用 memory storage（at-most-once）
		storageType = nats.MemoryStorage
	}

	return n.subscribeJetStreamWithStorage(ctx, topic, handler, isEnvelope, storageType)
}

// subscribeJetStreamWithStorage 使用指定的存储类型订阅 JetStream
func (n *natsEventBus) subscribeJetStreamWithStorage(ctx context.Context, topic string, handler MessageHandler, isEnvelope bool, storageType nats.StorageType) error {
	// ⭐ 包装 handler 为 handlerWrapper
	wrapper := &handlerWrapper{
		handler:    handler,
		isEnvelope: isEnvelope,
	}

	// 🔥 P0修复：使用 sync.Map 存储 handlerWrapper
	n.topicHandlers.Store(topic, wrapper)

	// 🔥 添加到订阅topic列表
	n.subscribedTopicsMu.Lock()
	needNewSubscription := true
	for _, t := range n.subscribedTopics {
		if t == topic {
			needNewSubscription = false
			break
		}
	}
	if needNewSubscription {
		n.subscribedTopics = append(n.subscribedTopics, topic)
	}
	n.subscribedTopicsMu.Unlock()

	// 🔥 为每个 topic 创建独立的 Durable Consumer（避免跨 topic 消息混淆）
	// 格式：{base_durable_name}_{topic}
	baseDurableName := n.unifiedConsumer.Config.Durable
	// 将 topic 中的特殊字符替换为下划线，避免 consumer 名称冲突
	topicSuffix := strings.ReplaceAll(topic, ".", "_")
	topicSuffix = strings.ReplaceAll(topicSuffix, "*", "wildcard")
	topicSuffix = strings.ReplaceAll(topicSuffix, ">", "all")
	durableName := fmt.Sprintf("%s_%s", baseDurableName, topicSuffix)

	// 🔥 P0修复：无锁读取 JetStream Context
	js, err := n.getJetStreamContext()
	if err != nil {
		// 回滚更改
		n.topicHandlers.Delete(topic)
		return fmt.Errorf("failed to get jetstream context: %w", err)
	}

	// ⭐ Stream 预建立优化：优先使用已存在的统一 Stream
	// 检查配置中的统一 Stream 是否已存在且能匹配当前 topic
	streamName := n.config.JetStream.Stream.Name
	streamInfo, streamErr := js.StreamInfo(streamName)

	if streamErr == nil {
		// 统一 Stream 存在，检查是否能匹配当前 topic
		canUseUnifiedStream := false
		for _, subject := range streamInfo.Config.Subjects {
			// 检查 subject pattern 是否能匹配当前 topic
			if n.subjectMatches(subject, topic) {
				canUseUnifiedStream = true
				n.logger.Info("Using pre-created unified stream",
					zap.String("stream", streamName),
					zap.String("topic", topic),
					zap.String("subjectPattern", subject))
				break
			}
		}

		if !canUseUnifiedStream {
			// 统一 Stream 不能匹配当前 topic，需要创建专用 Stream
			streamName = fmt.Sprintf("%s_%s", n.config.JetStream.Stream.Name, topicSuffix)
			if err := n.ensureTopicStreamExists(js, streamName, topic, storageType); err != nil {
				n.topicHandlers.Delete(topic)
				return fmt.Errorf("failed to ensure stream exists for topic %s: %w", topic, err)
			}
		}
	} else {
		// 统一 Stream 不存在，为 topic 创建专用的 Stream
		streamName = fmt.Sprintf("%s_%s", n.config.JetStream.Stream.Name, topicSuffix)
		if err := n.ensureTopicStreamExists(js, streamName, topic, storageType); err != nil {
			n.topicHandlers.Delete(topic)
			return fmt.Errorf("failed to ensure stream exists for topic %s: %w", topic, err)
		}
	}

	sub, err := js.PullSubscribe(topic, durableName)
	if err != nil {
		// 🔥 P0修复：回滚更改（使用 sync.Map）
		n.topicHandlers.Delete(topic)

		if needNewSubscription {
			n.subscribedTopicsMu.Lock()
			for i, t := range n.subscribedTopics {
				if t == topic {
					n.subscribedTopics = append(n.subscribedTopics[:i], n.subscribedTopics[i+1:]...)
					break
				}
			}
			n.subscribedTopicsMu.Unlock()
		}

		return fmt.Errorf("failed to create pull subscription for topic %s: %w", topic, err)
	}

	n.subscriptions[topic] = sub

	// 注册消费者到积压检测器
	if n.backlogDetector != nil {
		consumerName := fmt.Sprintf("unified-%s", topic)
		n.backlogDetector.RegisterConsumer(consumerName, durableName)
		n.logger.Debug("Topic registered to NATS backlog detector",
			zap.String("topic", topic),
			zap.String("consumer", consumerName),
			zap.String("durable", durableName))
	}

	// 🔥 启动统一消息处理协程（每个topic一个Pull Subscription）
	n.logger.Error("🔥 STARTING processUnifiedPullMessages",
		zap.String("topic", topic))
	go n.processUnifiedPullMessages(ctx, topic, sub)

	n.logger.Info("JetStream subscription created via unified consumer",
		zap.String("topic", topic),
		zap.String("durableName", durableName),
		zap.Int("totalTopics", len(n.subscribedTopics)))

	return nil
}

// 🔥 processUnifiedPullMessages 使用统一Consumer处理拉取的消息
func (n *natsEventBus) processUnifiedPullMessages(ctx context.Context, topic string, sub *nats.Subscription) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// ✅ 优化 2: 增大批量拉取大小（10 → 500）
			// ✅ 优化 3: 缩短 MaxWait 时间（1s → 100ms）
			// 拉取消息
			msgs, err := sub.Fetch(500, nats.MaxWait(100*time.Millisecond))
			if err != nil {
				if err == nats.ErrTimeout {
					continue // 超时是正常的，继续拉取
				}
				// ⭐ 优化：订阅关闭错误降为 debug 级别（正常清理行为）
				if strings.Contains(err.Error(), "subscription closed") || strings.Contains(err.Error(), "invalid subscription") {
					n.logger.Debug("Subscription closed, stopping message fetch",
						zap.String("topic", topic),
						zap.Error(err))
					return // 订阅已关闭，退出协程
				}
				n.logger.Error("Failed to fetch messages from unified consumer",
					zap.String("topic", topic),
					zap.Error(err))
				time.Sleep(time.Second)
				continue
			}

			// 处理消息
			n.logger.Error("🔥 PROCESSING MESSAGES",
				zap.String("topic", topic),
				zap.Int("msgCount", len(msgs)))

			for _, msg := range msgs {
				// 🔥 P0修复：无锁读取 handlerWrapper（使用 sync.Map）
				wrapperAny, exists := n.topicHandlers.Load(topic)

				n.logger.Error("🔥 HANDLER LOOKUP",
					zap.String("topic", topic),
					zap.Bool("exists", exists))

				if !exists {
					n.logger.Warn("No handler found for topic",
						zap.String("topic", topic))
					msg.Ack() // 确认消息以避免重复投递
					continue
				}

				wrapper := wrapperAny.(*handlerWrapper) // ⭐ 修改类型断言

				n.logger.Error("🔥 CALLING handleMessage",
					zap.String("topic", topic),
					zap.Int("dataLen", len(msg.Data)))

				// ⭐ 传递 wrapper 和 isEnvelope 标记
				n.handleMessageWithWrapper(ctx, topic, msg.Data, wrapper, func() error {
					return msg.Ack()
				}, func() error {
					return msg.Nak()
				})
			}
		}
	}
}

// handleMessageWithWrapper 处理单个消息（支持 at-least-once 语义）
// ⭐ Actor Pool 迁移：按 Topic 类型区分路由策略
func (n *natsEventBus) handleMessageWithWrapper(ctx context.Context, topic string, data []byte, wrapper *handlerWrapper, ackFunc func() error, nakFunc func() error) {
	n.logger.Error("🔥 handleMessageWithWrapper CALLED",
		zap.String("topic", topic),
		zap.Int("dataLen", len(data)),
		zap.Bool("isEnvelope", wrapper.isEnvelope))

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

	// ⭐ 提取聚合ID（用于路由决策）
	aggregateID, _ := ExtractAggregateID(data, nil, nil, topic)

	// ⭐ 按 Topic 类型确定路由键（核心变更）
	var routingKey string
	if wrapper.isEnvelope {
		// ⭐ 领域事件 Topic：必须使用聚合ID路由（保证顺序）
		routingKey = aggregateID
		if routingKey == "" {
			// ⚠️ 异常情况：领域事件没有聚合ID
			n.errorCount.Add(1)
			n.logger.Error("Domain event missing aggregate ID",
				zap.String("topic", topic))
			// Nak 重投，等待修复
			if nakFunc != nil {
				if nakErr := nakFunc(); nakErr != nil {
					n.logger.Error("Failed to nak NATS message",
						zap.String("topic", topic),
						zap.Error(nakErr))
				}
			}
			return
		}
	} else {
		// ⭐ 普通消息 Topic：总是使用 Round-Robin（忽略聚合ID）
		index := n.roundRobinCounter.Add(1)
		routingKey = fmt.Sprintf("rr-%d", index)
	}

	// ⭐ 使用 Hollywood Actor Pool 处理（统一路由）
	if n.actorPool != nil {
		aggMsg := &AggregateMessage{
			Topic:       topic,
			Partition:   0,
			Offset:      0,
			Key:         []byte(routingKey),
			Value:       data,
			Headers:     make(map[string][]byte),
			Timestamp:   time.Now(),
			AggregateID: routingKey, // ⭐ 使用计算出的路由键
			Context:     handlerCtx,
			Done:        make(chan error, 1),
			Handler:     wrapper.handler,
			IsEnvelope:  wrapper.isEnvelope,
		}

		// 提交到 Actor Pool
		if err := n.actorPool.ProcessMessage(handlerCtx, aggMsg); err != nil {
			n.errorCount.Add(1)
			n.logger.Error("Failed to submit message to Hollywood Actor Pool",
				zap.String("topic", topic),
				zap.String("routingKey", routingKey),
				zap.Error(err))
			// ⭐ 按 Topic 类型处理错误
			if wrapper.isEnvelope && nakFunc != nil {
				nakFunc()
			} else {
				// 普通消息：Ack（不重投）
				if ackFunc != nil {
					ackFunc()
				}
			}
			return
		}

		// ⭐ 等待 Actor 处理完成（Done Channel）
		select {
		case err := <-aggMsg.Done:
			if err != nil {
				n.errorCount.Add(1)
				n.logger.Error("Failed to handle NATS message in Hollywood Actor Pool",
					zap.String("topic", topic),
					zap.String("routingKey", routingKey),
					zap.Error(err))
				// ⭐ 按 Topic 类型处理错误
				if wrapper.isEnvelope {
					// 领域事件：Nak 重投（at-least-once）
					if nakFunc != nil {
						if nakErr := nakFunc(); nakErr != nil {
							n.logger.Error("Failed to nak NATS message",
								zap.String("topic", topic),
								zap.Error(nakErr))
						}
					}
				} else {
					// 普通消息：Ack（at-most-once）
					if ackFunc != nil {
						if ackErr := ackFunc(); ackErr != nil {
							n.logger.Error("Failed to ack NATS message",
								zap.String("topic", topic),
								zap.Error(ackErr))
						}
					}
				}
				return
			}
			// 成功：Ack
			if err := ackFunc(); err != nil {
				n.logger.Error("Failed to ack NATS message",
					zap.String("topic", topic),
					zap.Error(err))
			} else {
				n.consumedMessages.Add(1)
			}
			return
		case <-handlerCtx.Done():
			n.errorCount.Add(1)
			n.logger.Error("Context cancelled while waiting for Actor Pool",
				zap.String("topic", topic),
				zap.String("routingKey", routingKey),
				zap.Error(handlerCtx.Err()))
			// ⭐ 超时也按 Topic 类型处理
			if wrapper.isEnvelope && nakFunc != nil {
				nakFunc()
			} else if ackFunc != nil {
				ackFunc()
			}
			return
		}
	}

	// 降级：直接处理（Actor Pool 未初始化）
	if err := wrapper.handler(handlerCtx, data); err != nil {
		n.errorCount.Add(1)
		n.logger.Error("Failed to handle NATS message (fallback)",
			zap.String("topic", topic),
			zap.Error(err))
		// ⭐ 按 Topic 类型处理错误
		if wrapper.isEnvelope {
			// 领域事件：Nak 重投
			if nakFunc != nil {
				if nakErr := nakFunc(); nakErr != nil {
					n.logger.Error("Failed to nak NATS message",
						zap.String("topic", topic),
						zap.Error(nakErr))
				}
			}
		} else {
			// 普通消息：Ack
			if ackFunc != nil {
				if ackErr := ackFunc(); ackErr != nil {
					n.logger.Error("Failed to ack NATS message",
						zap.String("topic", topic),
						zap.Error(ackErr))
				}
			}
		}
		return
	}

	// 成功：Ack
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
	// 🔥 P0修复：无锁检查关闭状态
	if n.closed.Load() {
		n.healthStatus.Store(false)
		return fmt.Errorf("eventbus is closed")
	}

	// 🔥 P0修复：无锁读取 NATS Connection
	conn, err := n.getConn()
	if err != nil {
		n.healthStatus.Store(false)
		return fmt.Errorf("failed to get nats connection: %w", err)
	}

	// 检查NATS连接状态
	if !conn.IsConnected() {
		n.healthStatus.Store(false)
		return fmt.Errorf("NATS connection is not active")
	}

	// 检查JetStream连接状态（如果启用）
	if n.config.JetStream.Enabled {
		js, jsErr := n.getJetStreamContext()
		if jsErr == nil {
			// 尝试获取账户信息来验证JetStream连接
			_, err := js.AccountInfo()
			if err != nil {
				n.healthStatus.Store(false)
				return fmt.Errorf("JetStream connection is not active: %w", err)
			}
		}
	}

	// 发送ping测试连接
	if err := conn.Flush(); err != nil {
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

// Close 关闭连接。
//
// PR2-core (Task 3, spec §3.3): concurrent/repeated Close must converge on the
// SAME cached terminal error. The first caller runs teardown inside closeOnce.Do;
// every other caller blocks on <-closeDone and reads the identical terminalErr.
//
// Ordering invariants (load-bearing):
//   - Freeze FIRST: n.closed.Store(true) is the first statement so in-flight senders
//     take the Frozen path (Task 1) and RegisterTenant/UnregisterTenant closed-gates reject.
//   - defer close(n.closeDone) is registered BEFORE defer n.mu.Unlock(). Defers run
//     LIFO, so closeDone fires LAST — after terminalErr is assigned and mu released.
//     Every <-closeDone waiter observes the fully-written terminalErr without holding mu.
//
// Accepted trade-off (spec §3.3): a repeat-Close can block up to the 30s
// PublishAsyncComplete budget. Shutdown is not latency-sensitive; a stable terminal
// error is the contract.
func (n *natsEventBus) Close() error {
	n.closeOnce.Do(func() {
		// Freeze admission FIRST so in-flight senders take the Frozen path (Task 1)
		// and RegisterTenant's closed-gate (#3) rejects new tenants.
		n.closed.Store(true)
		// defer order: closeDone registered before mu.Unlock → runs LAST (LIFO),
		// after terminalErr is set and mu released. Waiters observe the full write.
		defer close(n.closeDone)
		n.mu.Lock()
		defer n.mu.Unlock()

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

		// ⭐ 停止 Hollywood Actor Pool
		if n.actorPool != nil {
			n.actorPool.Stop()
			n.logger.Debug("Stopped Hollywood Actor Pool")
		}

		// 🔥 P0修复：清空统一Consumer管理的映射（使用 sync.Map）
		// sync.Map 没有 Clear 方法，需要逐个删除或重新创建
		n.topicHandlers.Range(func(key, value interface{}) bool {
			n.topicHandlers.Delete(key)
			return true
		})

		n.subscribedTopicsMu.Lock()
		n.subscribedTopics = make([]string, 0)
		n.subscribedTopicsMu.Unlock()

		// 清空订阅映射
		n.subscriptions = make(map[string]*nats.Subscription)

		// ✅ 方案2：停止 ACK worker 池
		if n.ackWorkerStop != nil {
			n.logger.Info("Stopping ACK worker pool...")
			close(n.ackWorkerStop) // 发送停止信号
			n.ackWorkerWg.Wait()   // 等待所有 worker 退出
			n.logger.Info("ACK worker pool stopped")
		}

		// 🔥 P0修复：等待所有异步发布完成（优雅关闭）
		js, jsErr := n.getJetStreamContext()
		if jsErr == nil {
			n.logger.Info("Waiting for async publishes to complete...")
			select {
			case <-js.PublishAsyncComplete():
				n.logger.Info("All async publishes completed")
			case <-time.After(30 * time.Second):
				n.logger.Warn("Timeout waiting for async publishes to complete")
			}
		}

		// 🔥 P0修复：关闭NATS连接
		conn, connErr := n.getConn()
		if connErr == nil {
			conn.Close()
		}

		n.healthStatus.Store(false)

		// D3: close tenant channels AFTER the ACK-worker join (no processACKTask
		// sender is alive now), synchronously, NO drain goroutine. A still-ranging
		// consumer drains the buffer via range; otherwise the buffer is abandoned
		// (at-least-once outbox sweep self-heals — spec §3.2 shutdown carve-out).
		// Lock order: mu held, then tenantChannelsMu — NEVER the reverse.
		n.tenantChannelsMu.Lock()
		for id, ch := range n.tenantPublishResultChans {
			delete(n.tenantPublishResultChans, id) // detach first
			close(ch)                              // safe: senders gate on closed.Load() (frozen) + RLock
		}
		n.tenantChannelsMu.Unlock()

		// terminalErr assigned BEFORE closeDone fires (defer ordering guarantees it).
		n.terminalErr = errors.Join(errs...)
		if n.terminalErr != nil {
			n.logger.Warn("Some errors occurred during NATS EventBus close", zap.Errors("errors", errs))
		} else {
			n.logger.Info("NATS EventBus closed successfully")
		}
	})
	<-n.closeDone        // concurrent/repeat callers block here until teardown completed
	return n.terminalErr // identical value for every caller — satisfies §3.3 byte-equality
}

// RegisterReconnectCallback 注册重连回调
func (n *natsEventBus) RegisterReconnectCallback(callback ReconnectCallback) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 🔥 P0修复：无锁检查关闭状态
	if n.closed.Load() {
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
	// 🔥 P0修复：改为 Mutex（因为需要读取 reconnectCallbacks）
	n.mu.Lock()
	callbacks := make([]func(ctx context.Context) error, len(n.reconnectCallbacks))
	copy(callbacks, n.reconnectCallbacks)
	n.mu.Unlock()

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
			// 🔥 P0修复：无锁检查关闭状态（使用 atomic.Bool）
			if n.closed.Load() {
				return
			}
		}
	}
}

// updateJetStreamMetrics 更新JetStream指标
func (n *natsEventBus) updateJetStreamMetrics() {
	// 🔥 P0修复：无锁读取 JetStream Context
	js, err := n.getJetStreamContext()
	if err != nil {
		return
	}

	// 获取流信息
	streamName := n.config.JetStream.Stream.Name
	if streamName != "" {
		if streamInfo, err := js.StreamInfo(streamName); err == nil {
			// 更新JetStream指标
			if n.metrics != nil {
				n.metrics.MessageBacklog = int64(streamInfo.State.Msgs)
				n.metrics.ActiveConnections = int(streamInfo.State.Consumers)
			}
		}
	}

	// 获取统一消费者信息
	// 🔥 P0修复：改为 Mutex（需要读取 unifiedConsumer）
	n.mu.Lock()
	consumerName := n.unifiedConsumer.Name
	n.mu.Unlock()

	if consumerName != "" {
		if consumerInfo, err := js.ConsumerInfo(streamName, consumerName); err == nil {
			// 更新消费者指标
			if n.metrics != nil {
				n.metrics.MessagesConsumed += int64(consumerInfo.Delivered.Consumer)
			}
		}
	}
}

// ========== 生命周期管理 ==========

// Start 启动事件总线
func (n *natsEventBus) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 🔥 P0修复：无锁检查关闭状态
	if n.closed.Load() {
		return fmt.Errorf("nats eventbus is closed")
	}

	n.logger.Info("NATS eventbus started successfully")
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
	n.logger.Debug("Message formatter set for nats eventbus")
	return nil
}

// RegisterPublishCallback 注册发布回调
func (n *natsEventBus) RegisterPublishCallback(callback PublishCallback) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.publishCallback = callback
	n.logger.Debug("Publish callback registered for nats eventbus")
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
	n.logger.Debug("Message router set for nats eventbus")
	return nil
}

// SetErrorHandler 设置错误处理器
func (n *natsEventBus) SetErrorHandler(handler ErrorHandler) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.errorHandler = handler
	n.logger.Info("Error handler set for nats eventbus")
	return nil
}

// RegisterSubscriptionCallback 注册订阅回调
func (n *natsEventBus) RegisterSubscriptionCallback(callback SubscriptionCallback) error {
	n.logger.Info("Subscription callback registered for nats eventbus")
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
	// 🔥 P0修复：无锁读取状态（使用 atomic 字段）
	isClosed := n.closed.Load()
	conn, connErr := n.getConn()
	isConnected := connErr == nil && conn.IsConnected()

	isHealthy := !isClosed && isConnected
	return HealthCheckStatus{
		IsHealthy:           isHealthy,
		ConsecutiveFailures: 0,
		LastSuccessTime:     time.Now(),
		LastFailureTime:     time.Time{},
		IsRunning:           !isClosed,
		EventBusType:        "nats",
		Source:              "nats-eventbus",
	}
}

// RegisterHealthCheckCallback 注册健康检查回调
func (n *natsEventBus) RegisterHealthCheckCallback(callback HealthCheckCallback) error {
	n.logger.Info("Health check callback registered for nats eventbus")
	return nil
}

// StartHealthCheckSubscriber 启动健康检查消息订阅监控
func (n *natsEventBus) StartHealthCheckSubscriber(ctx context.Context) error {
	n.mu.Lock()

	if n.healthCheckSubscriber != nil {
		n.mu.Unlock()
		return nil // 已经启动
	}

	// 🔧 修复：使用保存的健康检查配置（如果未配置，则使用默认配置）
	// 与 StartHealthCheckPublisher 保持一致
	config := n.healthCheckConfig
	if !config.Enabled {
		config = GetDefaultHealthCheckConfig()
	}
	n.healthCheckSubscriber = NewHealthCheckSubscriber(config, n, "nats-eventbus", "nats")

	// 🔧 修复死锁：在调用 Start 之前释放锁
	// Start 方法内部会调用 Subscribe，而 Subscribe 也需要获取 n.mu 锁
	// 如果不释放锁，会导致死锁
	subscriber := n.healthCheckSubscriber
	n.mu.Unlock()

	// 启动监控器（不持有锁）
	if err := subscriber.Start(ctx); err != nil {
		// 启动失败，需要清理
		n.mu.Lock()
		n.healthCheckSubscriber = nil
		n.mu.Unlock()
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
	// 🔥 P0修复：改为 Mutex
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.healthCheckSubscriber == nil {
		return fmt.Errorf("health check subscriber not started")
	}

	return n.healthCheckSubscriber.RegisterAlertCallback(callback)
}

// GetHealthCheckSubscriberStats 获取健康检查订阅监控统计信息
func (n *natsEventBus) GetHealthCheckSubscriberStats() HealthCheckSubscriberStats {
	// 🔥 P0修复：改为 Mutex
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.healthCheckSubscriber == nil {
		return HealthCheckSubscriberStats{}
	}

	return n.healthCheckSubscriber.GetStats()
}

// GetConnectionState 获取连接状态
func (n *natsEventBus) GetConnectionState() ConnectionState {
	// 🔥 P0修复：无锁读取状态
	isClosed := n.closed.Load()
	conn, connErr := n.getConn()
	isConnected := connErr == nil && conn.IsConnected()

	return ConnectionState{
		IsConnected:       !isClosed && isConnected,
		LastConnectedTime: time.Now(),
		ReconnectCount:    0,
		LastError:         "",
	}
}

// GetMetrics 获取监控指标
func (n *natsEventBus) GetMetrics() Metrics {
	// 🔥 P0修复：无需锁（所有字段都是 atomic）
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
	// 🔥 P0修复：无锁检查关闭状态
	if n.closed.Load() {
		return fmt.Errorf("nats eventbus is closed")
	}

	// 🔥 P0修复：无锁读取连接
	conn, err := n.getConn()
	if err != nil {
		return fmt.Errorf("nats connection is nil: %w", err)
	}

	if !conn.IsConnected() {
		return fmt.Errorf("nats connection is not connected")
	}

	// 检查服务器信息
	if !conn.IsReconnecting() && conn.ConnectedUrl() != "" {
		n.logger.Debug("NATS connection check passed",
			zap.String("connectedUrl", conn.ConnectedUrl()),
			zap.String("status", conn.Status().String()))
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

	// 🔥 P0修复：无锁读取连接
	conn, connErr := n.getConn()
	if connErr == nil && conn.IsConnected() {
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
	// 🔥 P0修复：无锁读取连接
	conn, err := n.getConn()
	if err != nil {
		return fmt.Errorf("failed to get nats connection: %w", err)
	}

	// 创建接收通道
	receiveChan := make(chan string, 1)

	// 创建临时订阅来接收健康检查消息
	sub, err := conn.Subscribe(testSubject, func(msg *nats.Msg) {
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
	// 🔥 P0修复：无锁读取状态
	isClosed := n.closed.Load()
	conn, connErr := n.getConn()
	isConnected := connErr == nil && conn.IsConnected()

	connectionStatus := "disconnected"
	if !isClosed && isConnected {
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

	if n.closed.Load() {
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
		if err := n.reinitializeConnectionInternal(); err != nil {
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

// reinitializeConnectionInternal 重新初始化 NATS 连接（使用内部配置）
func (n *natsEventBus) reinitializeConnectionInternal() error {
	// 🔥 P0修复：关闭现有连接
	conn, connErr := n.getConn()
	if connErr == nil {
		conn.Close()
	}

	// 构建连接选项
	opts := buildNATSOptionsInternal(n.config)

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

	// 🔥 P0修复：使用 atomic.Value 更新连接
	n.conn.Store(nc)

	// 重新创建JetStream上下文
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}
	// 🔥 P0修复：使用 atomic.Value 更新 JetStream Context
	n.js.Store(js)

	// 重新初始化统一Consumer
	if err := n.initUnifiedConsumer(); err != nil {
		nc.Close()
		return fmt.Errorf("failed to reinitialize unified consumer: %w", err)
	}

	// 恢复订阅
	n.subscriptionsMu.RLock()
	handlers := make(map[string]*handlerWrapper) // ⭐ 修改类型
	for topic, wrapper := range n.subscriptionHandlers {
		handlers[topic] = wrapper
	}
	n.subscriptionsMu.RUnlock()

	// 重新订阅所有topic
	for topic, wrapper := range handlers {
		if err := n.Subscribe(context.Background(), topic, wrapper.handler); err != nil {
			n.logger.Warn("Failed to restore subscription during reconnection",
				zap.String("topic", topic),
				zap.Error(err))
		}
	}

	n.logger.Info("NATS connection reinitialized successfully")
	return nil
}

// 🔥 restoreSubscriptions 恢复所有订阅（使用统一Consumer架构）
func (n *natsEventBus) restoreSubscriptions(ctx context.Context) error {
	n.subscriptionsMu.RLock()
	handlers := make(map[string]*handlerWrapper) // ⭐ 修改类型
	for topic, wrapper := range n.subscriptionHandlers {
		handlers[topic] = wrapper
	}
	n.subscriptionsMu.RUnlock()

	if len(handlers) == 0 {
		n.logger.Debug("No subscriptions to restore")
		return nil
	}

	n.logger.Info("Restoring NATS subscriptions with unified consumer", zap.Int("count", len(handlers)))

	// 🔥 重新初始化统一Consumer
	if n.config.JetStream.Enabled {
		if err := n.initUnifiedConsumer(); err != nil {
			n.logger.Error("Failed to reinitialize unified consumer", zap.Error(err))
			return fmt.Errorf("failed to reinitialize unified consumer: %w", err)
		}
	}

	// 🔥 P0修复：清空现有映射
	n.subscriptions = make(map[string]*nats.Subscription)
	// 清空 sync.Map（逐个删除）
	n.topicHandlers.Range(func(key, value interface{}) bool {
		n.topicHandlers.Delete(key)
		return true
	})
	n.subscribedTopicsMu.Lock()
	n.subscribedTopics = make([]string, 0)
	n.subscribedTopicsMu.Unlock()

	var errors []error
	restoredCount := 0

	// 🔥 重新建立每个订阅（使用统一Consumer）
	for topic, wrapper := range handlers {
		err := n.subscribeJetStream(ctx, topic, wrapper.handler, wrapper.isEnvelope) // ⭐ 传递 isEnvelope 标记

		if err != nil {
			errors = append(errors, fmt.Errorf("failed to restore subscription for topic %s: %w", topic, err))
			n.logger.Error("Failed to restore JetStream subscription",
				zap.String("topic", topic),
				zap.Error(err))
		} else {
			restoredCount++
			n.logger.Debug("JetStream subscription restored via unified consumer",
				zap.String("topic", topic),
				zap.String("consumer", n.unifiedConsumer.Config.Durable))
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

// PublishEnvelope 发布Envelope消息（领域事件）
// ✅ 支持 Outbox 模式：通过 GetPublishResultChannel() 获取 ACK 结果
// ✅ 可靠投递：不容许丢失的领域事件必须使用此方法
// 适用场景：订单创建、支付完成、库存变更等关键业务事件
// 与 Publish() 的区别：
//   - PublishEnvelope(): 支持 Outbox 模式，发送 ACK 结果到 publishResultChan
//   - Publish(): 不支持 Outbox 模式，消息容许丢失
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed.Load() {
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

	// 🔥 P0修复：无锁读取 JetStream Context
	js, jsErr := n.getJetStreamContext()
	if jsErr == nil {
		// ✅ 方案2：异步发布，获取 Future
		pubAckFuture, err := js.PublishAsync(topic, envelopeBytes)
		if err != nil {
			n.errorCount.Add(1)
			n.logger.Error("Failed to submit async publish for envelope message",
				zap.String("subject", topic),
				zap.String("eventID", envelope.EventID),
				zap.String("aggregateID", envelope.AggregateID),
				zap.String("eventType", envelope.EventType),
				zap.Int64("eventVersion", envelope.EventVersion),
				zap.Error(err))
			return fmt.Errorf("failed to submit async publish: %w", err)
		}

		// ✅ 方案2：发送 ACK 任务到共享 worker 池
		// 使用 Envelope 中的 EventID
		task := &ackTask{
			future:      pubAckFuture,
			eventID:     envelope.EventID, // ← 使用 Envelope 的 EventID
			topic:       topic,
			aggregateID: envelope.AggregateID,
			eventType:   envelope.EventType,
			tenantID:    envelope.TenantID, // ← 租户ID（多租户支持，用于Outbox ACK路由）
		}

		select {
		case n.ackChan <- task:
			// 成功发送到 ACK 处理队列
			return nil
		case <-ctx.Done():
			// Context 取消
			return ctx.Err()
		default:
			// ACK 通道满，记录警告但仍然返回成功
			// 这样可以避免阻塞发布流程
			n.logger.Warn("ACK channel full, ACK processing may be delayed",
				zap.String("eventID", envelope.EventID),
				zap.String("topic", topic),
				zap.Int("ackChanLen", len(n.ackChan)),
				zap.Int("ackChanCap", cap(n.ackChan)))
			return nil
		}
	}

	// 🔥 P0修复：如果 JetStream 未启用，使用 NATS Core 发布
	conn, connErr := n.getConn()
	if connErr != nil {
		return fmt.Errorf("failed to get nats connection: %w", connErr)
	}
	err = conn.Publish(topic, envelopeBytes)
	if err != nil {
		n.errorCount.Add(1)
		n.logger.Error("Failed to publish envelope message",
			zap.String("subject", topic),
			zap.String("aggregateID", envelope.AggregateID),
			zap.String("eventType", envelope.EventType),
			zap.Int64("eventVersion", envelope.EventVersion),
			zap.Error(err))
		return fmt.Errorf("failed to publish: %w", err)
	}

	return nil
}

// PublishEnvelopeSync 同步发布Envelope消息（等待ACK确认）
//
// 使用场景：
// - 需要立即知道发布结果的场景
// - 关键业务消息，必须确认发布成功
// - 测试场景
//
// 性能：比 PublishEnvelope 慢，但提供即时反馈
func (n *natsEventBus) PublishEnvelopeSync(ctx context.Context, topic string, envelope *Envelope) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed.Load() {
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

	// 🔥 P0修复：无锁读取 JetStream Context
	js, jsErr := n.getJetStreamContext()
	if jsErr != nil {
		return fmt.Errorf("failed to get jetstream context: %w", jsErr)
	}

	// ✅ 同步发布（等待ACK）
	_, err = js.Publish(topic, envelopeBytes)
	if err != nil {
		n.errorCount.Add(1)
		n.logger.Error("Sync publish failed for envelope message",
			zap.String("subject", topic),
			zap.String("aggregateID", envelope.AggregateID),
			zap.String("eventType", envelope.EventType),
			zap.Int64("eventVersion", envelope.EventVersion),
			zap.Error(err))
		return fmt.Errorf("failed to publish: %w", err)
	}

	n.publishedMessages.Add(1)
	n.logger.Debug("Envelope message published successfully (sync)",
		zap.String("subject", topic),
		zap.String("aggregateID", envelope.AggregateID),
		zap.String("eventType", envelope.EventType),
		zap.Int64("eventVersion", envelope.EventVersion))

	return nil
}

// PublishEnvelopeBatch 批量发布Envelope消息（批量等待ACK）
//
// 使用场景：
// - 批量导入数据
// - 需要确认所有消息都发布成功
// - 性能和可靠性的平衡
//
// 性能：比单条同步发布快，比完全异步慢，但提供批量确认
//
// 参考：NATS bench 的批量 ACK 检查实现
func (n *natsEventBus) PublishEnvelopeBatch(ctx context.Context, topic string, envelopes []*Envelope) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed.Load() {
		return fmt.Errorf("nats eventbus is closed")
	}

	if len(envelopes) == 0 {
		return nil
	}

	// 🔥 P0修复：无锁读取 JetStream Context
	js, jsErr := n.getJetStreamContext()
	if jsErr != nil {
		return fmt.Errorf("failed to get jetstream context: %w", jsErr)
	}

	// ✅ 批量异步发布
	futures := make([]nats.PubAckFuture, 0, len(envelopes))
	for _, envelope := range envelopes {
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

		// 异步发布
		future, err := js.PublishAsync(topic, envelopeBytes)
		if err != nil {
			n.errorCount.Add(1)
			return fmt.Errorf("failed to submit async publish: %w", err)
		}
		futures = append(futures, future)
	}

	// ✅ 批量检查 ACK（参考 nats bench）
	timeout := 30 * time.Second
	if n.config.JetStream.PublishTimeout > 0 {
		timeout = n.config.JetStream.PublishTimeout
	}

	var errs []error
	for i, future := range futures {
		select {
		case <-future.Ok():
			n.publishedMessages.Add(1)
		case err := <-future.Err():
			n.errorCount.Add(1)
			errs = append(errs, fmt.Errorf("message %d failed: %w", i, err))
		case <-time.After(timeout):
			n.errorCount.Add(1)
			errs = append(errs, fmt.Errorf("message %d ACK timeout", i))
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to publish %d/%d messages: %v", len(errs), len(envelopes), errs)
	}

	n.logger.Debug("Batch envelope messages published successfully",
		zap.String("subject", topic),
		zap.Int("count", len(envelopes)))

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

	// ⭐ 重要：不能直接调用 Subscribe，需要设置 isEnvelope = true
	n.mu.Lock()
	defer n.mu.Unlock()

	// 检查关闭状态
	if n.closed.Load() {
		return fmt.Errorf("eventbus is closed")
	}

	// 检查是否已经订阅了该主题
	if _, exists := n.subscriptions[topic]; exists {
		return fmt.Errorf("already subscribed to topic: %s", topic)
	}

	// ⭐ 包装 handler 为 handlerWrapper（at-least-once 语义）
	wrapper := &handlerWrapper{
		handler:    wrappedHandler,
		isEnvelope: true, // ⭐ SubscribeEnvelope：at-least-once 语义
	}

	// 保存订阅处理器（用于重连后恢复）
	n.subscriptionsMu.Lock()
	n.subscriptionHandlers[topic] = wrapper
	n.subscriptionsMu.Unlock()

	// 根据配置选择订阅模式
	var err error

	// 检查 JetStream 是否可用
	_, jsErr := n.getJetStreamContext()
	jsAvailable := jsErr == nil

	if n.config.JetStream.Enabled && jsAvailable {
		// 使用JetStream订阅（持久化）
		n.logger.Info("Using JetStream subscription for Envelope",
			zap.String("topic", topic))
		err = n.subscribeJetStream(ctx, topic, wrappedHandler, true) // ⭐ isEnvelope = true
	} else {
		// 使用Core NATS订阅（非持久化）
		n.logger.Warn("Using Core NATS subscription for Envelope (no persistence)",
			zap.String("topic", topic))
		msgHandler := func(msg *nats.Msg) {
			// Core NATS 不支持 Nak，只能使用 at-most-once 语义
			n.handleMessageWithWrapper(ctx, topic, msg.Data, wrapper, func() error {
				return nil // Core NATS不需要手动确认
			}, func() error {
				return nil // Core NATS不支持 Nak
			})
		}

		conn, connErr := n.getConn()
		if connErr != nil {
			return fmt.Errorf("failed to get nats connection: %w", connErr)
		}

		sub, err := conn.Subscribe(topic, msgHandler)
		if err != nil {
			return fmt.Errorf("failed to subscribe to topic %s with Core NATS: %w", topic, err)
		}

		n.subscriptions[topic] = sub
	}

	if err != nil {
		return err
	}

	n.logger.Info("Subscribed to NATS Envelope topic",
		zap.String("topic", topic),
		zap.Bool("persistent", n.config.JetStream.Enabled),
		zap.Bool("atLeastOnce", n.config.JetStream.Enabled && jsAvailable))

	return nil
}

// GetPublishResultChannel 获取异步发布结果通道
// 用于Outbox Processor监听发布结果并更新Outbox状态
func (n *natsEventBus) GetPublishResultChannel() <-chan *PublishResult {
	return n.publishResultChan
}

// ========== 新的分离式健康检查接口实现 ==========

// StartHealthCheckPublisher 启动健康检查发布器
func (n *natsEventBus) StartHealthCheckPublisher(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.healthChecker != nil {
		return nil // 已经启动
	}

	// 使用保存的健康检查配置（如果未配置，则使用默认配置）
	config := n.healthCheckConfig
	if !config.Enabled {
		config = GetDefaultHealthCheckConfig()
	}

	n.healthChecker = NewHealthChecker(config, n, "nats-eventbus", "nats")

	// 启动健康检查发布器
	if err := n.healthChecker.Start(ctx); err != nil {
		n.healthChecker = nil
		return fmt.Errorf("failed to start health check publisher: %w", err)
	}

	n.logger.Info("Health check publisher started for nats eventbus",
		zap.Duration("interval", config.Publisher.Interval),
		zap.String("topic", config.Publisher.Topic))
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
	n.mu.Lock()
	defer n.mu.Unlock()

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
	n.mu.Lock()
	defer n.mu.Unlock()

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

	// 验证主题名称
	if err := ValidateTopicName(topic); err != nil {
		return err
	}

	// 🔥 P1优化：使用 sync.Map 无锁读写
	_, exists := n.topicConfigs.LoadOrStore(topic, options)
	if exists {
		// 如果已存在，更新配置
		n.topicConfigs.Store(topic, options)
	}

	// 根据策略决定是否需要同步到消息中间件
	shouldCreate, shouldUpdate := shouldCreateOrUpdate(n.topicConfigStrategy, exists)

	var action string
	var err error
	var mismatches []TopicConfigMismatch

	// 🔥 P0修复：检查 JetStream 是否可用
	_, jsErr := n.getJetStreamContext()
	jsAvailable := jsErr == nil

	// 如果是持久化模式且JetStream可用
	if options.IsPersistent(n.config.JetStream.Enabled) && jsAvailable {
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

	// 🔥 P0修复：Stream预创建优化：成功创建/配置Stream后，添加到本地缓存
	if options.IsPersistent(n.config.JetStream.Enabled) && jsAvailable && err == nil {
		streamName := n.getStreamNameForTopic(topic)
		// 使用 sync.Map 存储
		n.createdStreams.Store(streamName, true)
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
	// 🔥 P1优化：使用 sync.Map 无锁读取
	if config, exists := n.topicConfigs.Load(topic); exists {
		return config.(TopicOptions), nil
	}

	// 返回默认配置
	return DefaultTopicOptions(), nil
}

// ListConfiguredTopics 列出所有已配置的主题
func (n *natsEventBus) ListConfiguredTopics() []string {
	// 🔥 P1优化：使用 sync.Map 无锁遍历
	topics := make([]string, 0)
	n.topicConfigs.Range(func(key, value interface{}) bool {
		topics = append(topics, key.(string))
		return true // 继续遍历
	})

	return topics
}

// RemoveTopicConfig 移除主题配置（恢复为默认行为）
func (n *natsEventBus) RemoveTopicConfig(topic string) error {
	// 🔥 P1优化：使用 sync.Map 无锁删除
	n.topicConfigs.Delete(topic)

	n.logger.Info("Topic configuration removed", zap.String("topic", topic))
	return nil
}

// ensureTopicInJetStream 确保主题在JetStream中存在
func (n *natsEventBus) ensureTopicInJetStream(topic string, options TopicOptions) error {
	// 🔥 P0修复：无锁读取 JetStream Context
	js, err := n.getJetStreamContext()
	if err != nil {
		return fmt.Errorf("JetStream not enabled: %w", err)
	}

	// 获取或创建适合该主题的Stream名称
	streamName := n.getStreamNameForTopic(topic)

	// 尝试获取Stream信息
	streamInfo, err := js.StreamInfo(streamName)
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
	// 🔥 P0修复：无锁读取 JetStream Context
	js, err := n.getJetStreamContext()
	if err != nil {
		return fmt.Errorf("failed to get jetstream context: %w", err)
	}

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

	_, err = js.AddStream(streamConfig)
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
	// 🔥 P0修复：无锁读取 JetStream Context
	js, err := n.getJetStreamContext()
	if err != nil {
		return fmt.Errorf("failed to get jetstream context: %w", err)
	}

	// 获取现有Stream配置
	streamInfo, err := js.StreamInfo(streamName)
	if err != nil {
		return fmt.Errorf("failed to get stream info: %w", err)
	}

	// 添加新主题到subjects列表
	newSubjects := append(streamInfo.Config.Subjects, topic)
	streamInfo.Config.Subjects = newSubjects

	// 更新Stream配置
	_, err = js.UpdateStream(&streamInfo.Config)
	if err != nil {
		return fmt.Errorf("failed to update stream with new topic: %w", err)
	}

	n.logger.Info("Added topic to existing stream",
		zap.String("topic", topic),
		zap.String("stream", streamName))

	return nil
}

// ensureTopicInJetStreamIdempotent 幂等地确保主题在JetStream中存在（支持创建和更新）
// 🔥 P1优化：使用单飞抑制防止并发创建 Stream 风暴
func (n *natsEventBus) ensureTopicInJetStreamIdempotent(ctx context.Context, topic string, options TopicOptions, allowUpdate bool) error {
	streamName := n.getStreamNameForTopic(topic)

	// 🔥 P1优化：使用单飞抑制，确保同一个 stream 只创建一次
	// 即使有 1000 个并发请求，也只会执行一次创建操作
	_, err, _ := n.streamCreateGroup.Do(streamName, func() (interface{}, error) {
		// 🔥 P0修复：无锁读取 JetStream Context
		js, err := n.getJetStreamContext()
		if err != nil {
			return nil, fmt.Errorf("JetStream not enabled: %w", err)
		}

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
		streamInfo, err := js.StreamInfo(streamName)

		if err != nil {
			if err == nats.ErrStreamNotFound {
				// Stream不存在，创建新的
				n.logger.Info("Creating new JetStream stream",
					zap.String("stream", streamName),
					zap.String("topic", topic))

				_, err := js.AddStream(expectedConfig)
				if err != nil {
					return nil, fmt.Errorf("failed to create stream: %w", err)
				}

				n.logger.Info("Created JetStream stream",
					zap.String("stream", streamName),
					zap.String("topic", topic))
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get stream info: %w", err)
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
			// 🔥 P0修复：使用 js 变量
			_, err = js.UpdateStream(&streamInfo.Config)
			if err != nil {
				return nil, fmt.Errorf("failed to add topic to stream: %w", err)
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

				// 🔥 P0修复：使用 js 变量
				_, err = js.UpdateStream(&streamInfo.Config)
				if err != nil {
					n.logger.Warn("Failed to update stream config, using existing config",
						zap.String("stream", streamName),
						zap.Error(err))
					// 不返回错误，使用现有配置
				}
			}
		}

		return nil, nil
	})

	return err
}

// getActualTopicConfig 获取主题在JetStream中的实际配置
func (n *natsEventBus) getActualTopicConfig(ctx context.Context, topic string) (TopicOptions, error) {
	// 🔥 P0修复：无锁读取 JetStream Context
	js, err := n.getJetStreamContext()
	if err != nil {
		return TopicOptions{}, fmt.Errorf("JetStream not enabled: %w", err)
	}

	streamName := n.getStreamNameForTopic(topic)

	// 获取Stream信息
	streamInfo, err := js.StreamInfo(streamName)
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
	// 🔥 P1优化：使用 topicConfigStrategyMu 保护策略字段
	n.topicConfigStrategyMu.Lock()
	defer n.topicConfigStrategyMu.Unlock()
	n.topicConfigStrategy = strategy
	n.logger.Info("Topic config strategy updated", zap.String("strategy", string(strategy)))
}

// GetTopicConfigStrategy 获取当前主题配置策略
func (n *natsEventBus) GetTopicConfigStrategy() TopicConfigStrategy {
	// 🔥 P1优化：使用 topicConfigStrategyMu 保护策略字段
	n.topicConfigStrategyMu.RLock()
	defer n.topicConfigStrategyMu.RUnlock()
	return n.topicConfigStrategy
}

// ========== 方案2：共享 ACK 处理器实现 ==========

// startACKWorkers 启动 ACK worker 池
func (n *natsEventBus) startACKWorkers() {
	for i := 0; i < n.ackWorkerCount; i++ {
		n.ackWorkerWg.Add(1)
		go n.ackWorker(i)
	}
}

// ackWorker ACK 处理 worker
func (n *natsEventBus) ackWorker(workerID int) {
	defer n.ackWorkerWg.Done()

	n.logger.Debug("ACK worker started", zap.Int("workerID", workerID))

	for {
		select {
		case task := <-n.ackChan:
			// 处理 ACK 任务
			n.processACKTask(task)

		case <-n.ackWorkerStop:
			// 收到停止信号，退出
			n.logger.Debug("ACK worker stopping", zap.Int("workerID", workerID))
			return
		}
	}
}

// processACKTask 处理单个 ACK 任务
func (n *natsEventBus) processACKTask(task *ackTask) {
	// 🔥 P0修复：添加超时处理，避免 Worker 永久阻塞
	// 默认超时时间为 30 秒，可通过配置调整
	timeout := 30 * time.Second
	if n.config.JetStream.PublishTimeout > 0 {
		timeout = n.config.JetStream.PublishTimeout
	}

	select {
	case <-task.future.Ok():
		// ✅ 发布成功
		n.publishedMessages.Add(1)

		// 发送成功结果到通道（用于Outbox Processor）
		result := &PublishResult{
			EventID:     task.eventID,
			Topic:       task.topic,
			Success:     true,
			Error:       nil,
			Timestamp:   time.Now(),
			AggregateID: task.aggregateID,
			EventType:   task.eventType,
			TenantID:    task.tenantID, // ← 租户ID（多租户支持，用于Outbox ACK路由）
		}

		// ✅ 发送到租户专属通道或全局通道
		if n.sendResultToChannel(result) == AdmissionRejectedFrozen {
			return // registry is closing; stop producing ack results
		}

	case err := <-task.future.Err():
		// ❌ 发布失败
		n.errorCount.Add(1)
		n.logger.Error("Async publish ACK failed",
			zap.String("eventID", task.eventID),
			zap.String("topic", task.topic),
			zap.String("aggregateID", task.aggregateID),
			zap.String("eventType", task.eventType),
			zap.Error(err))

		// 发送失败结果到通道（用于Outbox Processor）
		result := &PublishResult{
			EventID:     task.eventID,
			Topic:       task.topic,
			Success:     false,
			Error:       err,
			Timestamp:   time.Now(),
			AggregateID: task.aggregateID,
			EventType:   task.eventType,
			TenantID:    task.tenantID, // ← 租户ID（多租户支持，用于Outbox ACK路由）
		}

		// ✅ 发送到租户专属通道或全局通道
		if n.sendResultToChannel(result) == AdmissionRejectedFrozen {
			return // registry is closing; stop producing ack results
		}

	case <-time.After(timeout):
		// ⏰ 超时：NATS JetStream ACK 响应超时
		n.errorCount.Add(1)
		n.logger.Error("Async publish ACK timeout",
			zap.String("eventID", task.eventID),
			zap.String("topic", task.topic),
			zap.String("aggregateID", task.aggregateID),
			zap.String("eventType", task.eventType),
			zap.Duration("timeout", timeout))

		// 发送超时结果到通道（用于Outbox Processor）
		result := &PublishResult{
			EventID:     task.eventID,
			Topic:       task.topic,
			Success:     false,
			Error:       fmt.Errorf("ACK timeout after %v", timeout),
			Timestamp:   time.Now(),
			AggregateID: task.aggregateID,
			EventType:   task.eventType,
			TenantID:    task.tenantID, // ← 租户ID（多租户支持，用于Outbox ACK路由）
		}

		// ✅ 发送到租户专属通道或全局通道
		if n.sendResultToChannel(result) == AdmissionRejectedFrozen {
			return // registry is closing; stop producing ack results
		}
	}
}

// ==========================================================================
// 多租户 ACK 支持
// ==========================================================================

// sendResultToChannel admits a broker ACK result (D1: lock-across-send). Holds
// tenantChannelsMu.RLock() ACROSS the non-blocking send so UnregisterTenant's write Lock
// cannot close the channel while a send is in flight — eliminates send-on-closed
// structurally, NO recover. Never falls back to the unmonitored global channel (spec §2.2).
func (n *natsEventBus) sendResultToChannel(result *PublishResult) AdmissionOutcome {
	if n.closed.Load() {
		return AdmissionRejectedFrozen
	}
	if result.TenantID != 0 {
		n.tenantChannelsMu.RLock()
		tenantChan, exists := n.tenantPublishResultChans[result.TenantID]
		if !exists {
			n.tenantChannelsMu.RUnlock()
			return AdmissionRejectedUnregistered // do NOT fall back to global
		}
		select {
		case tenantChan <- result:
			n.tenantChannelsMu.RUnlock()
			return AdmissionAccepted
		default:
			n.tenantChannelsMu.RUnlock()
			return AdmissionRejectedFull
		}
	}
	// TenantID == 0: legitimately global. Accepted only if it actually goes in.
	select {
	case n.publishResultChan <- result:
		return AdmissionAccepted
	default:
		return AdmissionRejectedFull
	}
}

// RegisterTenant 注册租户（创建租户专属的 ACK Channel）
func (n *natsEventBus) RegisterTenant(tenantID int, bufferSize int) error {
	// PR2-core (Task 3, ce-doc-review #3): reject after Close(). Because Close
	// flips `closed` BEFORE the tenant-channel close loop, a RegisterTenant racing
	// teardown either fails fast here or is serialized — it cannot install a fresh
	// channel that nobody closes (the late-registration leak).
	if n.closed.Load() {
		return fmt.Errorf("eventbus closed")
	}

	if tenantID == 0 {
		return fmt.Errorf("tenantID cannot be zero")
	}

	if bufferSize <= 0 {
		bufferSize = 100000 // 默认缓冲区大小
	}

	n.tenantChannelsMu.Lock()
	defer n.tenantChannelsMu.Unlock()

	// 延迟初始化 map
	if n.tenantPublishResultChans == nil {
		n.tenantPublishResultChans = make(map[int]chan *PublishResult)
	}

	// 检查租户是否已注册
	if _, exists := n.tenantPublishResultChans[tenantID]; exists {
		return fmt.Errorf("tenant %d already registered", tenantID)
	}

	// 创建租户专属 ACK Channel
	n.tenantPublishResultChans[tenantID] = make(chan *PublishResult, bufferSize)

	n.logger.Info("Tenant ACK channel registered",
		zap.Int("tenantID", tenantID),
		zap.Int("bufferSize", bufferSize))

	return nil
}

// UnregisterTenant 注销租户（关闭并清理租户的 ACK Channel）
func (n *natsEventBus) UnregisterTenant(tenantID int) error {
	// PR2-core (Task 3, ce-doc-review #3): reject after Close() (takes precedence
	// over the not-registered check — once Close started, the registry is frozen).
	if n.closed.Load() {
		return fmt.Errorf("eventbus closed")
	}

	if tenantID == 0 {
		return fmt.Errorf("tenantID cannot be zero")
	}

	n.tenantChannelsMu.Lock()

	// 检查租户是否已注册
	ch, exists := n.tenantPublishResultChans[tenantID]
	if !exists {
		n.tenantChannelsMu.Unlock()
		return fmt.Errorf("tenant %d not registered", tenantID)
	}

	// D1: detach FIRST, then close. Combined with Task 1's RLock-held send,
	// the write Lock waits out any in-flight sender, so close(ch) is safe.
	// NO recover — D1 makes send-on-closed structurally impossible.
	delete(n.tenantPublishResultChans, tenantID) // detach
	close(ch)                                    // safe: no sender holds the RLock now
	n.tenantChannelsMu.Unlock()

	n.logger.Info("Tenant ACK channel unregistered",
		zap.Int("tenantID", tenantID))

	return nil
}

// GetTenantPublishResultChannel 获取租户专属的异步发布结果通道
func (n *natsEventBus) GetTenantPublishResultChannel(tenantID int) <-chan *PublishResult {
	if tenantID == 0 {
		// 返回全局通道（向后兼容）
		return n.publishResultChan
	}

	n.tenantChannelsMu.RLock()
	defer n.tenantChannelsMu.RUnlock()

	if ch, exists := n.tenantPublishResultChans[tenantID]; exists {
		return ch
	}

	// 租户未注册，返回 nil
	n.logger.Warn("Tenant not registered, returning nil channel",
		zap.Int("tenantID", tenantID))
	return nil
}

// GetRegisteredTenants 获取所有已注册的租户ID列表
func (n *natsEventBus) GetRegisteredTenants() []int {
	n.tenantChannelsMu.RLock()
	defer n.tenantChannelsMu.RUnlock()

	tenants := make([]int, 0, len(n.tenantPublishResultChans))
	for tenantID := range n.tenantPublishResultChans {
		tenants = append(tenants, tenantID)
	}

	return tenants
}
