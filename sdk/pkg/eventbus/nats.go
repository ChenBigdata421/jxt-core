package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// natsEventBus NATS JetStream事件总线实现
// 企业级增强版本，使用JetStream提供高性能和可靠性
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
	messageCount    atomic.Int64
	errorCount      atomic.Int64
	lastHealthCheck atomic.Value // time.Time
	healthStatus    atomic.Bool

	// 增强的企业级特性
	metricsCollector *time.Ticker
}

// NewNATSEventBus 创建NATS JetStream事件总线
func NewNATSEventBus(config *config.NATSConfig) (EventBus, error) {
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

	// 创建JetStream上下文
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

	eventBus := &natsEventBus{
		conn:             nc,
		js:               js,
		config:           config,
		subscriptions:    make(map[string]*nats.Subscription),
		consumers:        make(map[string]nats.ConsumerInfo),
		logger:           logger.Logger,
		metricsCollector: time.NewTicker(30 * time.Second),
	}

	// 设置重连处理器来执行重连回调
	nc.SetReconnectHandler(func(nc *nats.Conn) {
		eventBus.logger.Info("NATS reconnected", zap.String("url", nc.ConnectedUrl()))
		eventBus.executeReconnectCallbacks()
	})

	// 初始化健康状态
	eventBus.lastHealthCheck.Store(time.Now())
	eventBus.healthStatus.Store(true)

	// 启动指标收集协程
	go eventBus.collectMetrics()

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

	var err error
	jetstream := n.config.JetStream.Enabled && n.js != nil

	if jetstream {
		// 使用JetStream发布消息
		var pubOpts []nats.PubOpt
		if n.config.JetStream.PublishTimeout > 0 {
			pubOpts = append(pubOpts, nats.AckWait(n.config.JetStream.PublishTimeout))
		}

		_, err = n.js.Publish(topic, message, pubOpts...)
		if err != nil {
			n.errorCount.Add(1)
			n.logger.Error("Failed to publish message to NATS JetStream",
				zap.String("topic", topic),
				zap.Error(err))
		}
	} else {
		// 使用核心NATS发布消息
		err = n.conn.Publish(topic, message)
		if err != nil {
			n.errorCount.Add(1)
			n.logger.Error("Failed to publish message to NATS",
				zap.String("topic", topic),
				zap.Error(err))
		}
	}

	// 记录指标
	duration := time.Since(start)
	// TODO: 实现指标记录

	if err == nil {
		n.messageCount.Add(1)
		n.logger.Debug("Message published to NATS",
			zap.String("topic", topic),
			zap.Int("message_size", len(message)),
			zap.Bool("jetstream", jetstream),
			zap.Duration("duration", duration))
		return nil
	}

	return fmt.Errorf("failed to publish message: %w", err)
}

// Subscribe 订阅指定主题的消息
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

	var sub *nats.Subscription
	var err error

	if n.config.JetStream.Enabled && n.js != nil {
		// 使用JetStream订阅
		err = n.subscribeJetStream(ctx, topic, handler)
	} else {
		// 使用核心NATS订阅
		msgHandler := func(msg *nats.Msg) {
			n.handleMessage(ctx, topic, msg.Data, handler, func() error {
				return nil // 核心NATS不需要手动确认
			})
		}

		sub, err = n.conn.Subscribe(topic, msgHandler)
		if err != nil {
			return fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
		}

		n.subscriptions[topic] = sub
	}

	if err != nil {
		return err
	}

	n.logger.Info("Subscribed to NATS topic",
		zap.String("topic", topic),
		zap.Bool("jetstream", n.config.JetStream.Enabled))

	return nil
}

// subscribeJetStream 使用JetStream订阅
func (n *natsEventBus) subscribeJetStream(ctx context.Context, topic string, handler MessageHandler) error {
	// 构建消费者配置
	consumerConfig := &nats.ConsumerConfig{
		Durable:       n.config.JetStream.Consumer.DurableName,
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

// handleMessage 处理单个消息
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

	// 调用用户处理函数
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
		n.messageCount.Add(1)
	}
}

// HealthCheck 健康检查
func (n *natsEventBus) HealthCheck(ctx context.Context) error {
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
		zap.Int64("message_count", n.messageCount.Load()),
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
func (n *natsEventBus) RegisterReconnectCallback(callback func(ctx context.Context) error) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("eventbus is closed")
	}

	n.reconnectCallbacks = append(n.reconnectCallbacks, callback)
	n.logger.Info("NATS reconnect callback registered")
	return nil
}

// executeReconnectCallbacks 执行重连回调
func (n *natsEventBus) executeReconnectCallbacks() {
	n.mu.RLock()
	callbacks := make([]func(ctx context.Context) error, len(n.reconnectCallbacks))
	copy(callbacks, n.reconnectCallbacks)
	n.mu.RUnlock()

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
	if !n.config.JetStream.Enabled || n.js == nil {
		return
	}

	// 获取流信息
	streamName := n.config.JetStream.Stream.Name
	if streamName != "" {
		if streamInfo, err := n.js.StreamInfo(streamName); err == nil {
			// TODO: 实现JetStream指标更新
			_ = streamInfo
		}
	}

	// 获取消费者信息
	n.mu.RLock()
	for consumerName := range n.consumers {
		if consumerInfo, err := n.js.ConsumerInfo(streamName, consumerName); err == nil {
			// TODO: 实现消费者指标更新
			_ = consumerInfo
		}
	}
	n.mu.RUnlock()
}
