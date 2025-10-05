package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// eventBusManager 事件总线管理器
// 实现 EventBus 和 EnvelopeEventBus 接口
type eventBusManager struct {
	config                *EventBusConfig
	publisher             Publisher
	subscriber            Subscriber
	metrics               *Metrics
	healthStatus          *HealthStatus
	businessHealthChecker BusinessHealthChecker
	reconnectCallback     func(ctx context.Context) error
	mu                    sync.RWMutex
	closed                bool

	// 健康检查控制
	healthCheckCancel context.CancelFunc
	healthCheckDone   chan struct{}

	// 健康检查订阅监控器（Memory实现）
	healthCheckSubscriber *HealthCheckSubscriber
	// 健康检查发布器（Memory实现）
	healthChecker *HealthChecker

	// 主题配置管理
	topicConfigs   map[string]TopicOptions
	topicConfigsMu sync.RWMutex

	// 待注册的健康检查告警回调（在订阅器启动前注册）
	pendingAlertCallbacks []HealthCheckAlertCallback
	callbackMu            sync.Mutex
}

// NewEventBus 创建新的事件总线实例
func NewEventBus(config *EventBusConfig) (EventBus, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	manager := &eventBusManager{
		config: config,
		metrics: &Metrics{
			LastHealthCheck:   time.Now(),
			HealthCheckStatus: "unknown",
		},
		healthStatus: &HealthStatus{
			Overall:   "initializing",
			Timestamp: time.Now(),
			Infrastructure: InfrastructureHealth{
				EventBus: EventBusHealthMetrics{
					ConnectionStatus: "initializing",
				},
			},
			Details: make(map[string]interface{}),
		},
		topicConfigs: make(map[string]TopicOptions),
	}

	// 根据配置类型创建具体实现
	switch config.Type {
	case "kafka":
		return manager.initKafka()
	case "nats":
		return manager.initNATS()
	case "memory":
		return manager.initMemory()
	default:
		return nil, fmt.Errorf("unsupported eventbus type: %s", config.Type)
	}
}

// Publish 发布消息
func (m *eventBusManager) Publish(ctx context.Context, topic string, message []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("eventbus is closed")
	}

	if m.publisher == nil {
		return fmt.Errorf("publisher not initialized")
	}

	// 更新指标
	start := time.Now()
	err := m.publisher.Publish(ctx, topic, message)

	m.updateMetrics(err == nil, true, time.Since(start))

	if err != nil {
		logger.Error("Failed to publish message", "topic", topic, "error", err)
		return fmt.Errorf("failed to publish message to topic %s: %w", topic, err)
	}

	logger.Debug("Message published successfully", "topic", topic, "size", len(message))
	return nil
}

// Subscribe 订阅消息
func (m *eventBusManager) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("eventbus is closed")
	}

	if m.subscriber == nil {
		return fmt.Errorf("subscriber not initialized")
	}

	// 包装处理器以更新指标
	wrappedHandler := func(ctx context.Context, message []byte) error {
		start := time.Now()
		err := handler(ctx, message)
		m.updateMetrics(err == nil, false, time.Since(start))

		if err != nil {
			logger.Error("Message handler failed", "topic", topic, "error", err)
		} else {
			logger.Debug("Message processed successfully", "topic", topic, "size", len(message))
		}

		return err
	}

	err := m.subscriber.Subscribe(ctx, topic, wrappedHandler)
	if err != nil {
		logger.Error("Failed to subscribe to topic", "topic", topic, "error", err)
		return fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
	}

	logger.Info("Successfully subscribed to topic", "topic", topic)
	return nil
}

// performHealthCheck 内部健康检查（不对外暴露）
func (m *eventBusManager) performHealthCheck(ctx context.Context) error {
	status, err := m.performFullHealthCheck(ctx)
	if err != nil {
		return err
	}

	if status.Overall != "healthy" {
		return fmt.Errorf("health check failed: %s", status.Overall)
	}

	return nil
}

// performFullHealthCheck 执行完整的健康检查（内部方法）
func (m *eventBusManager) performFullHealthCheck(ctx context.Context) (*HealthStatus, error) {
	start := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &HealthStatus{
			Overall:   "unhealthy",
			Timestamp: time.Now(),
			Infrastructure: InfrastructureHealth{
				EventBus: EventBusHealthMetrics{
					ConnectionStatus: "closed",
				},
			},
			CheckDuration: time.Since(start),
		}, fmt.Errorf("eventbus is closed")
	}

	// 更新健康检查时间
	m.metrics.LastHealthCheck = time.Now()

	// 执行基础设施健康检查
	infraHealth, err := m.checkInfrastructureHealth(ctx)
	if err != nil {
		return &HealthStatus{
			Overall:        "unhealthy",
			Infrastructure: infraHealth,
			Timestamp:      time.Now(),
			CheckDuration:  time.Since(start),
			Details:        map[string]interface{}{"error": err.Error()},
		}, err
	}

	// 执行业务健康检查（如果已注册）
	var businessHealth interface{}
	if m.businessHealthChecker != nil {
		if err := m.businessHealthChecker.CheckBusinessHealth(ctx); err != nil {
			return &HealthStatus{
				Overall:        "unhealthy",
				Infrastructure: infraHealth,
				Business:       map[string]interface{}{"error": err.Error()},
				Timestamp:      time.Now(),
				CheckDuration:  time.Since(start),
			}, err
		}
		businessHealth = m.businessHealthChecker.GetBusinessMetrics()
	}

	// 更新健康状态
	m.healthStatus = &HealthStatus{
		Overall:        "healthy",
		Infrastructure: infraHealth,
		Business:       businessHealth,
		Timestamp:      time.Now(),
		CheckDuration:  time.Since(start),
	}

	m.metrics.HealthCheckStatus = "healthy"
	logger.Debug("Health check completed successfully")
	return m.healthStatus, nil
}

// checkInfrastructureHealth 检查基础设施健康状态
func (m *eventBusManager) checkInfrastructureHealth(ctx context.Context) (InfrastructureHealth, error) {
	infraHealth := InfrastructureHealth{
		EventBus: EventBusHealthMetrics{
			ConnectionStatus: "unknown",
		},
	}

	// 检查 EventBus 连接状态
	if err := m.checkConnection(ctx); err != nil {
		infraHealth.EventBus.ConnectionStatus = "disconnected"
		infraHealth.EventBus.LastFailureTime = time.Now()
		return infraHealth, fmt.Errorf("eventbus connection check failed: %w", err)
	}

	// 检查消息传输能力
	if err := m.checkMessageTransport(ctx); err != nil {
		infraHealth.EventBus.ConnectionStatus = "connected"
		infraHealth.EventBus.LastFailureTime = time.Now()
		return infraHealth, fmt.Errorf("eventbus message transport check failed: %w", err)
	}

	// 获取 EventBus 指标
	infraHealth.EventBus = m.getEventBusMetrics()
	infraHealth.EventBus.ConnectionStatus = "connected"
	infraHealth.EventBus.LastSuccessTime = time.Now()

	return infraHealth, nil
}

// checkConnection 检查基础连接状态（内部方法）
func (m *eventBusManager) checkConnection(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 检查 EventBus 是否已关闭
	if m.closed {
		return fmt.Errorf("eventbus is closed")
	}

	// 检查发布器连接
	if m.publisher != nil {
		if healthChecker, ok := m.publisher.(interface{ HealthCheck(context.Context) error }); ok {
			if err := healthChecker.HealthCheck(ctx); err != nil {
				return fmt.Errorf("publisher health check failed: %w", err)
			}
		}
	}

	// 检查订阅器连接
	if m.subscriber != nil {
		if healthChecker, ok := m.subscriber.(interface{ HealthCheck(context.Context) error }); ok {
			if err := healthChecker.HealthCheck(ctx); err != nil {
				return fmt.Errorf("subscriber health check failed: %w", err)
			}
		}
	}

	return nil
}

// checkMessageTransport 检查端到端消息传输（内部方法）
func (m *eventBusManager) checkMessageTransport(ctx context.Context) error {
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()

	// 检查 EventBus 是否已关闭
	if closed {
		return fmt.Errorf("eventbus is closed")
	}

	if m.publisher == nil {
		return fmt.Errorf("publisher not initialized")
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 生成唯一的健康检查消息
	healthMsg := fmt.Sprintf("health-check-%d", time.Now().UnixNano())
	testTopic := "health_check_topic"

	// 如果有订阅器，进行完整的端到端测试
	if m.subscriber != nil {
		return m.performEndToEndTest(ctx, testTopic, healthMsg)
	}

	// 如果没有订阅器，只测试发布能力
	start := time.Now()
	if err := m.publisher.Publish(ctx, testTopic, []byte(healthMsg)); err != nil {
		return fmt.Errorf("failed to publish health check message: %w", err)
	}

	publishLatency := time.Since(start)
	logger.Debug("Health check message published successfully",
		"latency", publishLatency,
		"topic", testTopic)

	return nil
}

// performEndToEndTest 执行完整的端到端测试
func (m *eventBusManager) performEndToEndTest(ctx context.Context, testTopic, healthMsg string) error {
	// 创建接收通道
	receiveChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	// 设置临时订阅来接收健康检查消息
	handler := func(ctx context.Context, message []byte) error {
		receivedMsg := string(message)
		if receivedMsg == healthMsg {
			select {
			case receiveChan <- receivedMsg:
			default:
			}
		}
		return nil
	}

	// 订阅健康检查主题
	if err := m.subscriber.Subscribe(ctx, testTopic, handler); err != nil {
		return fmt.Errorf("failed to subscribe to health check topic: %w", err)
	}

	// 等待一小段时间确保订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布健康检查消息
	start := time.Now()
	if err := m.publisher.Publish(ctx, testTopic, []byte(healthMsg)); err != nil {
		return fmt.Errorf("failed to publish health check message: %w", err)
	}
	publishLatency := time.Since(start)

	// 等待接收消息或超时
	select {
	case receivedMsg := <-receiveChan:
		totalLatency := time.Since(start)
		logger.Debug("End-to-end health check successful",
			"publishLatency", publishLatency,
			"totalLatency", totalLatency,
			"message", receivedMsg)
		return nil

	case err := <-errorChan:
		return fmt.Errorf("health check subscription error: %w", err)

	case <-ctx.Done():
		return fmt.Errorf("health check timeout: message not received within timeout period")

	case <-time.After(5 * time.Second):
		return fmt.Errorf("health check timeout: message not received within 5 seconds")
	}
}

// getEventBusMetrics 获取 EventBus 性能指标（内部方法）
func (m *eventBusManager) getEventBusMetrics() EventBusHealthMetrics {
	return EventBusHealthMetrics{
		ConnectionStatus:    "connected",
		PublishLatency:      0, // TODO: 实际测量
		SubscribeLatency:    0, // TODO: 实际测量
		LastSuccessTime:     time.Now(),
		ConsecutiveFailures: 0,
		ThroughputPerSecond: 0, // TODO: 实际统计
		MessageBacklog:      m.metrics.MessageBacklog,
		ReconnectCount:      0, // TODO: 实际统计
		BrokerCount:         1, // TODO: 实际获取
		TopicCount:          1, // TODO: 实际获取
	}
}

// RegisterBusinessHealthCheck 注册业务健康检查
func (m *eventBusManager) RegisterBusinessHealthCheck(checker BusinessHealthChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.businessHealthChecker = checker
}

// Close 关闭连接
func (m *eventBusManager) Close() error {
	m.mu.Lock()

	if m.closed {
		m.mu.Unlock()
		return nil
	}

	// 先标记为已关闭，防止新的操作
	m.closed = true

	// 获取需要关闭的组件引用
	healthChecker := m.healthChecker
	healthCheckSubscriber := m.healthCheckSubscriber
	publisher := m.publisher
	subscriber := m.subscriber

	m.mu.Unlock()

	var errors []error

	// 1. 先停止健康检查（避免在EventBus关闭后继续发送消息）
	if healthChecker != nil {
		logger.Debug("Stopping health check publisher before closing EventBus")
		if err := healthChecker.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop health checker: %w", err))
		}
	}

	if healthCheckSubscriber != nil {
		logger.Debug("Stopping health check subscriber before closing EventBus")
		if err := healthCheckSubscriber.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop health check subscriber: %w", err))
		}
	}

	// 2. 关闭发布器
	if publisher != nil {
		if err := publisher.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close publisher: %w", err))
		}
	}

	// 3. 关闭订阅器
	if subscriber != nil {
		if err := subscriber.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close subscriber: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during close: %v", errors)
	}

	logger.Info("EventBus closed successfully")
	return nil
}

// RegisterReconnectCallback 注册重连回调
func (m *eventBusManager) RegisterReconnectCallback(callback ReconnectCallback) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 将新的回调类型转换为旧的类型
	m.reconnectCallback = func(ctx context.Context) error {
		return callback(ctx)
	}
	logger.Info("Reconnect callback registered")
	return nil
}

// GetMetrics 获取指标
func (m *eventBusManager) GetMetrics() Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.metrics
}

// GetHealthStatus 获取健康状态（已废弃，使用GetHealthCheckPublisherStatus）
func (m *eventBusManager) GetHealthStatus() HealthCheckStatus {
	logger.Warn("GetHealthStatus is deprecated, use GetHealthCheckPublisherStatus instead")
	return m.GetHealthCheckPublisherStatus()
}

// updateMetrics 更新指标
func (m *eventBusManager) updateMetrics(success bool, isPublish bool, duration time.Duration) {
	if isPublish {
		if success {
			m.metrics.MessagesPublished++
		} else {
			m.metrics.PublishErrors++
		}
	} else {
		if success {
			m.metrics.MessagesConsumed++
		} else {
			m.metrics.ConsumeErrors++
		}
	}

	// 可以在这里记录处理时间相关的指标
	_ = duration // 暂时忽略，未来可用于性能指标
}

// initKafka 初始化Kafka事件总线
func (m *eventBusManager) initKafka() (EventBus, error) {
	// 创建简化的配置格式
	kafkaConfig := &config.KafkaConfig{
		Brokers: m.config.Kafka.Brokers,
		Producer: config.ProducerConfig{
			RequiredAcks:   m.config.Kafka.Producer.RequiredAcks,
			Timeout:        m.config.Kafka.Producer.Timeout,
			Compression:    m.config.Kafka.Producer.Compression,
			FlushFrequency: m.config.Kafka.Producer.FlushFrequency,
			FlushMessages:  m.config.Kafka.Producer.FlushMessages,
			RetryMax:       m.config.Kafka.Producer.RetryMax,
			BatchSize:      m.config.Kafka.Producer.BatchSize,
			BufferSize:     m.config.Kafka.Producer.BufferSize,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           m.config.Kafka.Consumer.GroupID,
			AutoOffsetReset:   m.config.Kafka.Consumer.AutoOffsetReset,
			SessionTimeout:    m.config.Kafka.Consumer.SessionTimeout,
			HeartbeatInterval: m.config.Kafka.Consumer.HeartbeatInterval,
			MaxProcessingTime: m.config.Kafka.Consumer.MaxProcessingTime,
			FetchMinBytes:     m.config.Kafka.Consumer.FetchMinBytes,
			FetchMaxBytes:     m.config.Kafka.Consumer.FetchMaxBytes,
			FetchMaxWait:      m.config.Kafka.Consumer.FetchMaxWait,
		},
	}

	return NewKafkaEventBusWithFullConfig(kafkaConfig, m.config)
}

// initNATS 初始化NATS事件总线
func (m *eventBusManager) initNATS() (EventBus, error) {
	// 创建简化的配置格式
	natsConfig := &config.NATSConfig{
		URLs:              m.config.NATS.URLs,
		ClientID:          m.config.NATS.ClientID,
		MaxReconnects:     m.config.NATS.MaxReconnects,
		ReconnectWait:     m.config.NATS.ReconnectWait,
		ConnectionTimeout: m.config.NATS.ConnectionTimeout,
		JetStream: config.JetStreamConfig{
			Enabled:        m.config.NATS.JetStream.Enabled,
			Domain:         m.config.NATS.JetStream.Domain,
			APIPrefix:      m.config.NATS.JetStream.APIPrefix,
			PublishTimeout: m.config.NATS.JetStream.PublishTimeout,
			AckWait:        m.config.NATS.JetStream.AckWait,
			MaxDeliver:     m.config.NATS.JetStream.MaxDeliver,
			Stream: config.StreamConfig{
				Name:      m.config.NATS.JetStream.Stream.Name,
				Subjects:  m.config.NATS.JetStream.Stream.Subjects,
				Retention: m.config.NATS.JetStream.Stream.Retention,
				Storage:   m.config.NATS.JetStream.Stream.Storage,
				Replicas:  m.config.NATS.JetStream.Stream.Replicas,
				MaxAge:    m.config.NATS.JetStream.Stream.MaxAge,
				MaxBytes:  m.config.NATS.JetStream.Stream.MaxBytes,
				MaxMsgs:   m.config.NATS.JetStream.Stream.MaxMsgs,
				Discard:   m.config.NATS.JetStream.Stream.Discard,
			},
			Consumer: config.NATSConsumerConfig{
				DurableName:   m.config.NATS.JetStream.Consumer.DurableName,
				DeliverPolicy: m.config.NATS.JetStream.Consumer.DeliverPolicy,
				AckPolicy:     m.config.NATS.JetStream.Consumer.AckPolicy,
				ReplayPolicy:  m.config.NATS.JetStream.Consumer.ReplayPolicy,
			},
		},
		Security: config.NATSSecurityConfig{
			Enabled:  m.config.NATS.Security.Enabled,
			Username: m.config.NATS.Security.Username,
			Password: m.config.NATS.Security.Password,
			CertFile: m.config.NATS.Security.CertFile,
			KeyFile:  m.config.NATS.Security.KeyFile,
			CAFile:   m.config.NATS.Security.CAFile,
		},
	}

	return NewNATSEventBusWithFullConfig(natsConfig, m.config)
}

// ========== 生命周期管理 ==========

// Start 启动事件总线
func (m *eventBusManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("eventbus is closed")
	}

	logger.Info("EventBus started successfully")
	return nil
}

// Stop 停止事件总线
func (m *eventBusManager) Stop() error {
	return m.Close()
}

// ========== 高级发布功能 ==========

// PublishWithOptions 使用选项发布消息
func (m *eventBusManager) PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error {
	// 基础实现，直接调用 Publish
	// 高级功能可以在具体实现中扩展
	return m.Publish(ctx, topic, message)
}

// SetMessageFormatter 设置消息格式化器
func (m *eventBusManager) SetMessageFormatter(formatter MessageFormatter) error {
	// eventBusManager 是基础实现，不支持消息格式化器
	// 具体的实现（如 kafkaEventBus, natsEventBus）会重写此方法
	logger.Debug("Message formatter set for eventbus manager (base implementation)")
	return nil
}

// RegisterPublishCallback 注册发布回调
func (m *eventBusManager) RegisterPublishCallback(callback PublishCallback) error {
	logger.Debug("Publish callback registered for eventbus manager")
	return nil
}

// ========== 高级订阅功能 ==========

// SubscribeWithOptions 使用选项订阅消息
func (m *eventBusManager) SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error {
	// 基础实现，直接调用 Subscribe
	// 高级功能可以在具体实现中扩展
	return m.Subscribe(ctx, topic, handler)
}

// RegisterSubscriberBacklogCallback 注册订阅端积压回调
func (m *eventBusManager) RegisterSubscriberBacklogCallback(callback BacklogStateCallback) error {
	logger.Info("Subscriber backlog callback registered for eventbus manager")
	return nil
}

// StartSubscriberBacklogMonitoring 启动订阅端积压监控
func (m *eventBusManager) StartSubscriberBacklogMonitoring(ctx context.Context) error {
	logger.Info("Subscriber backlog monitoring started for eventbus manager")
	return nil
}

// StopSubscriberBacklogMonitoring 停止订阅端积压监控
func (m *eventBusManager) StopSubscriberBacklogMonitoring() error {
	logger.Info("Subscriber backlog monitoring stopped for eventbus manager")
	return nil
}

// RegisterBacklogCallback 注册订阅端积压回调（已废弃，向后兼容）
func (m *eventBusManager) RegisterBacklogCallback(callback BacklogStateCallback) error {
	logger.Warn("RegisterBacklogCallback is deprecated, use RegisterSubscriberBacklogCallback instead")
	return m.RegisterSubscriberBacklogCallback(callback)
}

// StartBacklogMonitoring 启动订阅端积压监控（已废弃，向后兼容）
func (m *eventBusManager) StartBacklogMonitoring(ctx context.Context) error {
	logger.Warn("StartBacklogMonitoring is deprecated, use StartSubscriberBacklogMonitoring instead")
	return m.StartSubscriberBacklogMonitoring(ctx)
}

// StopBacklogMonitoring 停止订阅端积压监控（已废弃，向后兼容）
func (m *eventBusManager) StopBacklogMonitoring() error {
	logger.Warn("StopBacklogMonitoring is deprecated, use StopSubscriberBacklogMonitoring instead")
	return m.StopSubscriberBacklogMonitoring()
}

// RegisterPublisherBacklogCallback 注册发送端积压回调
func (m *eventBusManager) RegisterPublisherBacklogCallback(callback PublisherBacklogCallback) error {
	logger.Debug("Publisher backlog callback registered for eventbus manager")
	return nil
}

// StartPublisherBacklogMonitoring 启动发送端积压监控
func (m *eventBusManager) StartPublisherBacklogMonitoring(ctx context.Context) error {
	logger.Debug("Publisher backlog monitoring started for eventbus manager (not available)")
	return nil
}

// StopPublisherBacklogMonitoring 停止发送端积压监控
func (m *eventBusManager) StopPublisherBacklogMonitoring() error {
	logger.Debug("Publisher backlog monitoring stopped for eventbus manager (not available)")
	return nil
}

// StartAllBacklogMonitoring 根据配置启动所有积压监控
func (m *eventBusManager) StartAllBacklogMonitoring(ctx context.Context) error {
	logger.Info("All backlog monitoring started for eventbus manager")
	return nil
}

// StopAllBacklogMonitoring 停止所有积压监控
func (m *eventBusManager) StopAllBacklogMonitoring() error {
	logger.Info("All backlog monitoring stopped for eventbus manager")
	return nil
}

// SetMessageRouter 设置消息路由器
func (m *eventBusManager) SetMessageRouter(router MessageRouter) error {
	logger.Debug("Message router set for eventbus manager")
	return nil
}

// SetErrorHandler 设置错误处理器
func (m *eventBusManager) SetErrorHandler(handler ErrorHandler) error {
	logger.Info("Error handler set for eventbus manager")
	return nil
}

// RegisterSubscriptionCallback 注册订阅回调
func (m *eventBusManager) RegisterSubscriptionCallback(callback SubscriptionCallback) error {
	logger.Info("Subscription callback registered for eventbus manager")
	return nil
}

// ========== 统一健康检查和监控 ==========

// StartHealthCheck 启动健康检查
// StartHealthCheck 启动健康检查（已废弃，使用StartHealthCheckPublisher）
func (m *eventBusManager) StartHealthCheck(ctx context.Context) error {
	logger.Warn("StartHealthCheck is deprecated, use StartHealthCheckPublisher instead")
	return m.StartHealthCheckPublisher(ctx)
}

// StopHealthCheck 停止健康检查（已废弃，使用StopHealthCheckPublisher）
func (m *eventBusManager) StopHealthCheck() error {
	logger.Warn("StopHealthCheck is deprecated, use StopHealthCheckPublisher instead")
	return m.StopHealthCheckPublisher()
}

// RegisterHealthCheckCallback 注册健康检查回调（已废弃，使用RegisterHealthCheckPublisherCallback）
func (m *eventBusManager) RegisterHealthCheckCallback(callback HealthCheckCallback) error {
	logger.Warn("RegisterHealthCheckCallback is deprecated, use RegisterHealthCheckPublisherCallback instead")
	return m.RegisterHealthCheckPublisherCallback(callback)
}

// StartHealthCheckSubscriber 启动健康检查消息订阅监控
func (m *eventBusManager) StartHealthCheckSubscriber(ctx context.Context) error {
	m.mu.Lock()
	if m.healthCheckSubscriber != nil {
		m.mu.Unlock()
		return nil // 已经启动
	}

	// 创建健康检查订阅监控器（使用全局配置中的健康检查设置）
	globalCfg := GetGlobalConfig()
	var healthConfig config.HealthCheckConfig
	if globalCfg != nil && globalCfg.HealthCheck.Enabled {
		healthConfig = globalCfg.HealthCheck
	} else {
		healthConfig = GetDefaultHealthCheckConfig()
	}
	subscriber := NewHealthCheckSubscriber(healthConfig, m, "memory-eventbus", "memory")
	m.mu.Unlock()

	// 启动监控器（在锁外启动以避免死锁）
	if err := subscriber.Start(ctx); err != nil {
		return fmt.Errorf("failed to start health check subscriber: %w", err)
	}

	// 重新获取锁并设置订阅监控器
	m.mu.Lock()
	m.healthCheckSubscriber = subscriber
	m.mu.Unlock()

	// 应用待注册的回调
	m.callbackMu.Lock()
	pendingCallbacks := m.pendingAlertCallbacks
	m.pendingAlertCallbacks = nil
	m.callbackMu.Unlock()

	for _, callback := range pendingCallbacks {
		if err := subscriber.RegisterAlertCallback(callback); err != nil {
			logger.Warn("Failed to register pending alert callback", "error", err)
		} else {
			logger.Debug("Applied pending alert callback after subscriber started")
		}
	}

	logger.Info("Health check subscriber started for memory eventbus")
	return nil
}

// StopHealthCheckSubscriber 停止健康检查消息订阅监控
func (m *eventBusManager) StopHealthCheckSubscriber() error {
	// 先获取 healthCheckSubscriber 的引用，避免在持有锁时调用 Stop()
	m.mu.Lock()
	subscriber := m.healthCheckSubscriber
	if subscriber == nil {
		m.mu.Unlock()
		return nil
	}
	m.healthCheckSubscriber = nil
	m.mu.Unlock()

	// 在锁外调用 Stop()，避免死锁
	if err := subscriber.Stop(); err != nil {
		logger.Error("Failed to stop health check subscriber", "error", err)
		return err
	}

	logger.Info("Health check subscriber stopped for memory eventbus")
	return nil
}

// RegisterHealthCheckAlertCallback 注册健康检查告警回调
// 支持在订阅器启动前注册，回调会在启动时自动应用
func (m *eventBusManager) RegisterHealthCheckAlertCallback(callback HealthCheckAlertCallback) error {
	m.callbackMu.Lock()
	defer m.callbackMu.Unlock()

	// 如果订阅器已启动，直接注册
	m.mu.RLock()
	subscriber := m.healthCheckSubscriber
	m.mu.RUnlock()

	if subscriber != nil {
		return subscriber.RegisterAlertCallback(callback)
	}

	// 否则，保存到待注册列表
	m.pendingAlertCallbacks = append(m.pendingAlertCallbacks, callback)
	logger.Debug("Health check alert callback queued for registration after subscriber starts")
	return nil
}

// GetHealthCheckSubscriberStats 获取健康检查订阅监控统计信息
func (m *eventBusManager) GetHealthCheckSubscriberStats() HealthCheckSubscriberStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.healthCheckSubscriber == nil {
		return HealthCheckSubscriberStats{
			StartTime: time.Now(),
			IsHealthy: true,
		}
	}

	return m.healthCheckSubscriber.GetStats()
}

// ========== 新的分离式健康检查接口实现 ==========

// StartHealthCheckPublisher 启动健康检查发布器
func (m *eventBusManager) StartHealthCheckPublisher(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthChecker != nil {
		return nil // 已经启动
	}

	// 创建健康检查发布器（使用全局配置中的健康检查设置）
	globalCfg := GetGlobalConfig()
	var healthConfig config.HealthCheckConfig
	if globalCfg != nil && globalCfg.HealthCheck.Enabled {
		healthConfig = globalCfg.HealthCheck
	} else {
		healthConfig = GetDefaultHealthCheckConfig()
	}
	m.healthChecker = NewHealthChecker(healthConfig, m, "memory-eventbus", "memory")

	// 启动健康检查发布器
	if err := m.healthChecker.Start(ctx); err != nil {
		m.healthChecker = nil
		return fmt.Errorf("failed to start health check publisher: %w", err)
	}

	logger.Info("Health check publisher started for memory eventbus")
	return nil
}

// StopHealthCheckPublisher 停止健康检查发布器
func (m *eventBusManager) StopHealthCheckPublisher() error {
	// 先获取 healthChecker 的引用，避免在持有锁时调用 Stop()
	m.mu.Lock()
	checker := m.healthChecker
	if checker == nil {
		m.mu.Unlock()
		return nil
	}
	m.healthChecker = nil
	m.mu.Unlock()

	// 在锁外调用 Stop()，避免死锁
	if err := checker.Stop(); err != nil {
		return fmt.Errorf("failed to stop health check publisher: %w", err)
	}

	logger.Info("Health check publisher stopped for memory eventbus")
	return nil
}

// GetHealthCheckPublisherStatus 获取健康检查发布器状态
func (m *eventBusManager) GetHealthCheckPublisherStatus() HealthCheckStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.healthChecker == nil {
		return HealthCheckStatus{
			IsHealthy:           false,
			ConsecutiveFailures: 0,
			LastSuccessTime:     time.Time{},
			LastFailureTime:     time.Now(),
			IsRunning:           false,
			EventBusType:        "memory",
			Source:              "memory-eventbus",
		}
	}

	return m.healthChecker.GetStatus()
}

// RegisterHealthCheckPublisherCallback 注册健康检查发布器回调
func (m *eventBusManager) RegisterHealthCheckPublisherCallback(callback HealthCheckCallback) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.healthChecker == nil {
		return fmt.Errorf("health check publisher not started")
	}

	return m.healthChecker.RegisterCallback(callback)
}

// RegisterHealthCheckSubscriberCallback 注册健康检查订阅器回调
func (m *eventBusManager) RegisterHealthCheckSubscriberCallback(callback HealthCheckAlertCallback) error {
	return m.RegisterHealthCheckAlertCallback(callback)
}

// StartAllHealthCheck 根据配置启动所有健康检查
func (m *eventBusManager) StartAllHealthCheck(ctx context.Context) error {
	// 这里可以根据配置决定启动哪些健康检查
	// 为了演示，我们启动发布器和订阅器
	if err := m.StartHealthCheckPublisher(ctx); err != nil {
		return fmt.Errorf("failed to start health check publisher: %w", err)
	}

	if err := m.StartHealthCheckSubscriber(ctx); err != nil {
		return fmt.Errorf("failed to start health check subscriber: %w", err)
	}

	return nil
}

// StopAllHealthCheck 停止所有健康检查
func (m *eventBusManager) StopAllHealthCheck() error {
	var errs []error

	if err := m.StopHealthCheckPublisher(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop health check publisher: %w", err))
	}

	if err := m.StopHealthCheckSubscriber(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop health check subscriber: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping health checks: %v", errs)
	}

	return nil
}

// GetConnectionState 获取连接状态
func (m *eventBusManager) GetConnectionState() ConnectionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	isConnected := !m.closed && m.publisher != nil && m.subscriber != nil
	return ConnectionState{
		IsConnected:       isConnected,
		LastConnectedTime: time.Now(),
		ReconnectCount:    0,
		LastError:         "",
	}
}

// ========== 方案A：Envelope 支持 ==========

// PublishEnvelope 发布Envelope消息（方案A）
func (m *eventBusManager) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("eventbus is closed")
	}

	// 检查 envelope 是否为 nil
	if envelope == nil {
		return fmt.Errorf("envelope cannot be nil")
	}

	// 检查 topic 是否为空
	if topic == "" {
		return fmt.Errorf("topic cannot be empty")
	}

	// 检查publisher是否支持Envelope
	if envelopePublisher, ok := m.publisher.(EnvelopePublisher); ok {
		return envelopePublisher.PublishEnvelope(ctx, topic, envelope)
	}

	// 回退到普通发布（序列化Envelope为字节数组）
	envelopeBytes, err := envelope.ToBytes()
	if err != nil {
		return fmt.Errorf("failed to serialize envelope: %w", err)
	}

	return m.publisher.Publish(ctx, topic, envelopeBytes)
}

// SubscribeEnvelope 订阅Envelope消息（方案A）
func (m *eventBusManager) SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("eventbus is closed")
	}

	// 检查 topic 是否为空
	if topic == "" {
		return fmt.Errorf("topic cannot be empty")
	}

	// 检查 handler 是否为 nil
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	// 检查subscriber是否支持Envelope
	if envelopeSubscriber, ok := m.subscriber.(EnvelopeSubscriber); ok {
		return envelopeSubscriber.SubscribeEnvelope(ctx, topic, handler)
	}

	// 回退到普通订阅（包装handler解析Envelope）
	wrappedHandler := func(ctx context.Context, message []byte) error {
		envelope, err := FromBytes(message)
		if err != nil {
			return fmt.Errorf("failed to parse envelope: %w", err)
		}
		return handler(ctx, envelope)
	}

	return m.subscriber.Subscribe(ctx, topic, wrappedHandler)
}

// ========== 主题持久化管理实现 ==========

// ConfigureTopic 配置主题的持久化策略和其他选项
// 注意：Memory EventBus不支持真正的持久化，此方法主要用于接口兼容性
func (m *eventBusManager) ConfigureTopic(ctx context.Context, topic string, options TopicOptions) error {
	m.topicConfigsMu.Lock()
	defer m.topicConfigsMu.Unlock()

	// 缓存配置（即使Memory不支持持久化，也保存配置用于查询）
	m.topicConfigs[topic] = options

	logger.Info("Topic configured for memory eventbus",
		"topic", topic,
		"persistenceMode", string(options.PersistenceMode),
		"note", "Memory EventBus does not support true persistence")

	return nil
}

// SetTopicPersistence 设置主题是否持久化（简化接口）
func (m *eventBusManager) SetTopicPersistence(ctx context.Context, topic string, persistent bool) error {
	mode := TopicEphemeral
	if persistent {
		mode = TopicPersistent
	}

	options := DefaultTopicOptions()
	options.PersistenceMode = mode

	return m.ConfigureTopic(ctx, topic, options)
}

// GetTopicConfig 获取主题的当前配置
func (m *eventBusManager) GetTopicConfig(topic string) (TopicOptions, error) {
	m.topicConfigsMu.RLock()
	defer m.topicConfigsMu.RUnlock()

	if config, exists := m.topicConfigs[topic]; exists {
		return config, nil
	}

	// 返回默认配置
	return DefaultTopicOptions(), nil
}

// ListConfiguredTopics 列出所有已配置的主题
func (m *eventBusManager) ListConfiguredTopics() []string {
	m.topicConfigsMu.RLock()
	defer m.topicConfigsMu.RUnlock()

	topics := make([]string, 0, len(m.topicConfigs))
	for topic := range m.topicConfigs {
		topics = append(topics, topic)
	}

	return topics
}

// RemoveTopicConfig 移除主题配置（恢复为默认行为）
func (m *eventBusManager) RemoveTopicConfig(topic string) error {
	m.topicConfigsMu.Lock()
	defer m.topicConfigsMu.Unlock()

	delete(m.topicConfigs, topic)

	logger.Info("Topic configuration removed from memory eventbus", "topic", topic)
	return nil
}

// SetTopicConfigStrategy 设置主题配置策略
func (m *eventBusManager) SetTopicConfigStrategy(strategy TopicConfigStrategy) {
	// 委托给底层实现
	if setter, ok := m.publisher.(interface {
		SetTopicConfigStrategy(TopicConfigStrategy)
	}); ok {
		setter.SetTopicConfigStrategy(strategy)
	}

	if setter, ok := m.subscriber.(interface {
		SetTopicConfigStrategy(TopicConfigStrategy)
	}); ok {
		setter.SetTopicConfigStrategy(strategy)
	}

	logger.Info("Topic config strategy updated", "strategy", string(strategy))
}

// GetTopicConfigStrategy 获取当前主题配置策略
func (m *eventBusManager) GetTopicConfigStrategy() TopicConfigStrategy {
	// 从底层实现获取
	if getter, ok := m.publisher.(interface {
		GetTopicConfigStrategy() TopicConfigStrategy
	}); ok {
		return getter.GetTopicConfigStrategy()
	}

	// 默认返回创建或更新策略
	return StrategyCreateOrUpdate
}
