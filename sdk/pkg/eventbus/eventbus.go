package eventbus

import (
	"context"
	"errors"
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

	// PR2-core (Task 2, D5 — memory-backend only, spec §3.3): stable terminal error on
	// concurrent/repeated Close. closeOnce guarantees teardown runs exactly once;
	// closeDone is closed when teardown completes so every waiting repeat-Close caller
	// converges on the same terminalErr. Adapted for the plain-bool `closed` (guarded
	// by m.mu, not atomic.Bool): Close flips closed to true under m.mu.Lock() inside
	// closeOnce.Do as the FIRST teardown step (freeze admission before touching channels).
	closeOnce   sync.Once
	closeDone   chan struct{}
	terminalErr error

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

	// 异步发布结果通道（用于Outbox模式）
	publishResultChan chan *PublishResult

	// 多租户 ACK 通道支持
	tenantPublishResultChans map[int]chan *PublishResult // key: tenantID, value: ACK channel
	tenantChannelsMu         sync.RWMutex                // 保护 tenantPublishResultChans 的读写锁

	// 指标收集器（用于 Prometheus 等监控系统）
	metricsCollector MetricsCollector
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

		// PR2-core (Task 2, D5): closeDone closed at end of Close() teardown to release
		// concurrent/repeat callers. Only the memory backend returns this manager as the
		// runtime type (kafka/nats return their own concrete structs), so this only
		// matters for memory — but initializing it unconditionally is harmless.
		closeDone: make(chan struct{}),
	}

	// 初始化指标收集器
	if config.MetricsCollector != nil {
		manager.metricsCollector = config.MetricsCollector
	} else {
		manager.metricsCollector = &NoOpMetricsCollector{}
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
	duration := time.Since(start)

	m.updateMetrics(err == nil, true, duration)

	// 记录到外部指标收集器（Prometheus 等）
	if m.metricsCollector != nil {
		m.metricsCollector.RecordPublish(topic, err == nil, duration)
		if err != nil {
			m.metricsCollector.RecordError("publish", topic)
		}
	}

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
		duration := time.Since(start)

		m.updateMetrics(err == nil, false, duration)

		// 记录到外部指标收集器（Prometheus 等）
		if m.metricsCollector != nil {
			m.metricsCollector.RecordConsume(topic, err == nil, duration)
			if err != nil {
				m.metricsCollector.RecordError("consume", topic)
			}
		}

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

	// 🔧 修复死锁问题：先检查关闭状态，避免在持有锁时调用其他需要锁的方法
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()

	if closed {
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

	// 执行基础设施健康检查（不持有锁）
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
	m.mu.RLock()
	businessHealthChecker := m.businessHealthChecker
	m.mu.RUnlock()

	if businessHealthChecker != nil {
		if err := businessHealthChecker.CheckBusinessHealth(ctx); err != nil {
			return &HealthStatus{
				Overall:        "unhealthy",
				Infrastructure: infraHealth,
				Business:       map[string]interface{}{"error": err.Error()},
				Timestamp:      time.Now(),
				CheckDuration:  time.Since(start),
			}, err
		}
		businessHealth = businessHealthChecker.GetBusinessMetrics()
	}

	// 🔧 修复死锁问题：在更新状态时获取锁
	healthStatus := &HealthStatus{
		Overall:        "healthy",
		Infrastructure: infraHealth,
		Business:       businessHealth,
		Timestamp:      time.Now(),
		CheckDuration:  time.Since(start),
	}

	m.mu.Lock()
	m.healthStatus = healthStatus
	m.metrics.LastHealthCheck = time.Now()
	m.metrics.HealthCheckStatus = "healthy"
	m.mu.Unlock()
	logger.Debug("Health check completed successfully")
	return healthStatus, nil
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

// Close 关闭连接。
//
// PR2-core (Task 2, D5 — memory-backend only, spec §3.3): concurrent/repeated Close
// converges on the SAME cached terminal error. The first caller runs teardown inside
// closeOnce.Do; every other caller blocks on <-closeDone and reads the identical
// terminalErr.
//
// Ordering invariants (load-bearing):
//   - Freeze FIRST: m.closed = true is the first statement under m.mu.Lock() so in-flight
//     senders snapshot the frozen state (Task 2) and RegisterTenant/UnregisterTenant
//     closed-gates reject. Plain bool → MUST be set under the lock (no atomic here).
//   - D8 DEVIATION from nats/kafka shape (CORRECT here, see note below): m.mu is RELEASED
//     before any component teardown. nats/kafka hold their own mu across teardown because
//     their components do NOT re-enter that mu. The memory eventBusManager's
//     healthCheckLoop calls eventBus.Publish → m.mu.RLock(), and HealthChecker.Stop joins
//     healthCheckLoop; likewise sendResultToChannel takes m.mu.RLock() to snapshot `closed`.
//     Holding m.mu.Lock() across healthChecker.Stop() therefore deadlocks (Close waits for
//     the loop to exit; the loop waits for RLock). So memory must release mu before joining
//     any goroutine that can re-enter m.mu.
//   - defer close(m.closeDone) is registered AFTER the explicit m.mu.Unlock() and is the
//     ONLY defer in the closure. Defers run LIFO, so closeDone fires LAST — after
//     terminalErr is assigned. Every <-closeDone waiter observes the fully-written
//     terminalErr. Do NOT add defer m.mu.Unlock() back.
//   - D3: tenant channels are detached (snapshot + replace map) under tenantChannelsMu
//     while mu is still held, then closed OUTSIDE any lock. Admission is frozen
//     (closed=true set before Unlock), so sendResultToChannel sees closed==true under RLock
//     and returns AdmissionRejectedFrozen without touching a channel — closing here is safe.
func (m *eventBusManager) Close() error {
	m.closeOnce.Do(func() {
		// 1. Freeze admission + snapshot component refs + detach tenant chans, all under the locks.
		m.mu.Lock()
		m.closed = true
		healthChecker := m.healthChecker
		healthCheckSubscriber := m.healthCheckSubscriber
		publisher := m.publisher
		subscriber := m.subscriber
		m.tenantChannelsMu.Lock()
		tenantChans := m.tenantPublishResultChans
		m.tenantPublishResultChans = make(map[int]chan *PublishResult) // detach under lock
		m.tenantChannelsMu.Unlock()
		m.mu.Unlock() // <<< RELEASE before any teardown that may re-enter m.mu (D8 deviate)

		// closeDone fires LAST (LIFO, only defer) — after terminalErr is assigned below.
		defer close(m.closeDone)

		// 2. Teardown OUTSIDE m.mu. healthChecker.Stop joins healthCheckLoop, which Publishes
		//    (m.mu.RLock); healthCheckSubscriber.Stop joins its Subscribe goroutine (also
		//    m.mu.RLock). Doing either under m.mu.Lock() deadlocks. (nats/kafka Close hold
		//    their own mu across teardown, but their components do NOT re-enter that mu —
		//    memory's do, so memory must deviate from byte-parity here. See D8 note above.)
		var errs []error

		// 1. 先停止健康检查（避免在EventBus关闭后继续发送消息）
		if healthChecker != nil {
			logger.Debug("Stopping health check publisher before closing EventBus")
			if err := healthChecker.Stop(); err != nil {
				errs = append(errs, fmt.Errorf("failed to stop health checker: %w", err))
			}
		}

		if healthCheckSubscriber != nil {
			logger.Debug("Stopping health check subscriber before closing EventBus")
			if err := healthCheckSubscriber.Stop(); err != nil {
				errs = append(errs, fmt.Errorf("failed to stop health check subscriber: %w", err))
			}
		}

		// 2. 关闭发布器
		if publisher != nil {
			if err := publisher.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close publisher: %w", err))
			}
		}

		// 3. 关闭订阅器
		if subscriber != nil {
			if err := subscriber.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close subscriber: %w", err))
			}
		}

		// 3. Close detached tenant chans. Admission is frozen (closed=true set before Unlock);
		//    sendResultToChannel sees closed under RLock and returns AdmissionRejectedFrozen
		//    without touching a channel, so closing here is safe. No live sender remains.
		for _, ch := range tenantChans {
			close(ch)
		}

		// terminalErr assigned BEFORE closeDone fires (defer ordering guarantees it).
		m.terminalErr = errors.Join(errs...)
		if m.terminalErr != nil {
			logger.Error("Errors during EventBus close", "errors", errs)
		} else {
			logger.Info("EventBus closed successfully")
		}
	})
	<-m.closeDone        // concurrent/repeat callers block here until teardown completed
	return m.terminalErr // identical value for every caller — satisfies §3.3 byte-equality
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
	// m.config.Kafka 已经是程序员配置层的配置，直接使用
	return NewKafkaEventBus(&m.config.Kafka)
}

// initNATS 初始化NATS事件总线
func (m *eventBusManager) initNATS() (EventBus, error) {
	// 使用程序员配置层的配置，直接使用
	// m.config.NATS 已经是程序员配置层的配置，直接使用
	return NewNATSEventBus(&m.config.NATS)
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

	publishErr := m.publisher.Publish(ctx, topic, envelopeBytes)

	// ✅ 发送 ACK 结果到 publishResultChan（用于 Outbox 模式）
	// 对于 Memory EventBus，需要手动发送 ACK 结果
	if envelope.EventID != "" {
		// 创建 ACK 结果（在 goroutine 外部，避免竞态条件）
		result := &PublishResult{
			EventID:     envelope.EventID,
			Topic:       topic,
			Success:     publishErr == nil,
			Error:       publishErr,
			Timestamp:   time.Now(),
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			TenantID:    envelope.TenantID,
		}

		// 异步发送到租户专属通道或全局通道
		go m.sendResultToChannel(result)
	}

	return publishErr
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
	// 验证主题名称
	if err := ValidateTopicName(topic); err != nil {
		return err
	}

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

// ========== 配置转换函数 ==========

// ConvertUserConfigToInternalKafkaConfig 将用户配置转换为程序员内部配置
// 这是配置分层设计的核心：用户只需要配置核心字段，程序员控制技术细节
// 导出此函数以便在演示和测试中使用
func ConvertUserConfigToInternalKafkaConfig(userConfig *config.KafkaConfig) *KafkaConfig {
	return convertUserConfigToInternalKafkaConfig(userConfig)
}

// convertUserConfigToInternalKafkaConfig 内部转换函数
func convertUserConfigToInternalKafkaConfig(userConfig *config.KafkaConfig) *KafkaConfig {
	internalConfig := &KafkaConfig{
		// 基础配置 (从用户配置直接映射)
		Brokers: userConfig.Brokers,

		// 生产者配置转换
		Producer: ProducerConfig{
			// 用户配置字段 (直接映射，但需要确保与幂等性兼容)
			RequiredAcks:   -1, // 强制设置为WaitForAll，幂等性生产者要求
			FlushFrequency: userConfig.Producer.FlushFrequency,
			FlushMessages:  userConfig.Producer.FlushMessages,
			Timeout:        userConfig.Producer.Timeout,

			// 程序员设定的默认值 (用户不需要关心的技术细节)
			FlushBytes:      1024 * 1024,      // 1MB - 批量字节数
			RetryMax:        3,                // 3次重试
			BatchSize:       16 * 1024,        // 16KB - 批量大小
			BufferSize:      32 * 1024 * 1024, // 32MB - 缓冲区大小
			Idempotent:      true,             // 启用幂等性，确保消息不重复
			MaxMessageBytes: 1024 * 1024,      // 1MB - 最大消息大小
			PartitionerType: "hash",           // 哈希分区器，确保相同key的消息到同一分区

			// 高级技术字段 (程序员专用优化)
			LingerMs:    5 * time.Millisecond, // 5ms延迟发送，提高批处理效率
			MaxInFlight: 1,                    // 幂等性生产者要求MaxInFlight=1
			// 注意：压缩配置已从 Producer 级别移到 Topic 级别，通过 TopicBuilder 配置
		},

		// 消费者配置转换
		Consumer: ConsumerConfig{
			// 用户配置字段 (直接映射)
			GroupID:           userConfig.Consumer.GroupID,
			AutoOffsetReset:   userConfig.Consumer.AutoOffsetReset,
			SessionTimeout:    userConfig.Consumer.SessionTimeout,
			HeartbeatInterval: userConfig.Consumer.HeartbeatInterval,

			// 程序员设定的默认值 (用户不需要关心的技术细节)
			MaxProcessingTime: 30 * time.Second,       // 30s最大处理时间
			FetchMinBytes:     1024,                   // 1KB最小获取字节数
			FetchMaxBytes:     50 * 1024 * 1024,       // 50MB最大获取字节数
			FetchMaxWait:      500 * time.Millisecond, // 500ms最大等待时间

			// 高级技术字段 (程序员专用优化)
			MaxPollRecords:     500,              // 最大轮询记录数
			EnableAutoCommit:   false,            // 禁用自动提交，手动控制
			AutoCommitInterval: 5 * time.Second,  // 自动提交间隔
			IsolationLevel:     "read_committed", // 读已提交，确保数据一致性
			RebalanceStrategy:  "range",          // 范围分区策略
		},

		// 程序员专用配置 (用户完全不需要关心)
		HealthCheckInterval:  30 * time.Second,       // 健康检查间隔
		ClientID:             "jxt-eventbus",         // 客户端标识
		MetadataRefreshFreq:  10 * time.Minute,       // 元数据刷新频率
		MetadataRetryMax:     3,                      // 元数据重试次数
		MetadataRetryBackoff: 250 * time.Millisecond, // 元数据重试退避时间

		// 网络配置 (程序员专用，用户不需要配置)
		Net: NetConfig{
			DialTimeout:  30 * time.Second, // 连接超时
			ReadTimeout:  30 * time.Second, // 读取超时
			WriteTimeout: 30 * time.Second, // 写入超时
			KeepAlive:    30 * time.Second, // 保活时间
			MaxIdleConns: 10,               // 最大空闲连接数
			MaxOpenConns: 100,              // 最大打开连接数
		},

		// 安全配置 (默认不启用，可根据需要调整)
		Security: SecurityConfig{
			Enabled: false, // 默认不启用安全认证
		},

		// 企业级特性配置 (需要从完整配置中获取，这里先设置默认值)
		Enterprise: EnterpriseConfig{
			// 这些值将在convertConfig函数中被正确设置
		},
	}

	return internalConfig
}

// convertUserConfigToInternalNATSConfig 将用户NATS配置转换为程序员内部配置
func convertUserConfigToInternalNATSConfig(userConfig *NATSConfig) *NATSConfig {
	internalConfig := &NATSConfig{
		// 基础配置 (从用户配置直接映射)
		URLs:              userConfig.URLs,
		ClientID:          userConfig.ClientID,
		MaxReconnects:     userConfig.MaxReconnects,
		ReconnectWait:     userConfig.ReconnectWait,
		ConnectionTimeout: userConfig.ConnectionTimeout,

		// 程序员专用配置 (设定合理默认值)
		HealthCheckInterval: 5 * time.Minute, // 默认5分钟健康检查

		// JetStream配置 - 需要完整转换
		JetStream: convertJetStreamConfig(userConfig.JetStream),

		// 安全配置
		Security: userConfig.Security,

		// 企业级特性配置 (需要从完整配置中获取，这里先设置默认值)
		Enterprise: EnterpriseConfig{
			// 这些值将在convertConfig函数中被正确设置
		},
	}

	return internalConfig
}

// convertJetStreamConfig 转换JetStream配置，添加程序员专用字段
func convertJetStreamConfig(userJetStream JetStreamConfig) JetStreamConfig {
	internalJetStream := userJetStream

	// 设置程序员专用的默认值
	if internalJetStream.PublishTimeout == 0 {
		internalJetStream.PublishTimeout = 5 * time.Second
	}
	if internalJetStream.AckWait == 0 {
		internalJetStream.AckWait = 30 * time.Second
	}
	if internalJetStream.MaxDeliver == 0 {
		internalJetStream.MaxDeliver = 3
	}

	// 设置Stream配置的程序员默认值
	if internalJetStream.Stream.Name == "" {
		internalJetStream.Stream.Name = "BUSINESS_STREAM"
	}
	if len(internalJetStream.Stream.Subjects) == 0 {
		internalJetStream.Stream.Subjects = []string{"business.>"}
	}
	if internalJetStream.Stream.Retention == "" {
		internalJetStream.Stream.Retention = "limits"
	}
	if internalJetStream.Stream.Storage == "" {
		internalJetStream.Stream.Storage = "file"
	}
	if internalJetStream.Stream.Replicas == 0 {
		internalJetStream.Stream.Replicas = 1
	}
	if internalJetStream.Stream.MaxAge == 0 {
		internalJetStream.Stream.MaxAge = 24 * time.Hour
	}
	if internalJetStream.Stream.MaxBytes == 0 {
		internalJetStream.Stream.MaxBytes = 100 * 1024 * 1024 // 100MB
	}
	if internalJetStream.Stream.MaxMsgs == 0 {
		internalJetStream.Stream.MaxMsgs = 10000
	}
	if internalJetStream.Stream.Discard == "" {
		internalJetStream.Stream.Discard = "old"
	}

	// 设置Consumer配置的程序员默认值
	if internalJetStream.Consumer.DurableName == "" {
		internalJetStream.Consumer.DurableName = "business-consumer"
	}
	if internalJetStream.Consumer.DeliverPolicy == "" {
		internalJetStream.Consumer.DeliverPolicy = "all"
	}
	if internalJetStream.Consumer.AckPolicy == "" {
		internalJetStream.Consumer.AckPolicy = "explicit"
	}
	if internalJetStream.Consumer.ReplayPolicy == "" {
		internalJetStream.Consumer.ReplayPolicy = "instant"
	}
	if internalJetStream.Consumer.MaxAckPending == 0 {
		internalJetStream.Consumer.MaxAckPending = 100
	}
	if internalJetStream.Consumer.MaxWaiting == 0 {
		internalJetStream.Consumer.MaxWaiting = 500
	}
	if internalJetStream.Consumer.MaxDeliver == 0 {
		internalJetStream.Consumer.MaxDeliver = 3
	}

	return internalJetStream
}

// GetPublishResultChannel 获取异步发布结果通道
// 用于Outbox Processor监听发布结果并更新Outbox状态
func (m *eventBusManager) GetPublishResultChannel() <-chan *PublishResult {
	return m.publishResultChan
}

// ==========================================================================
// 多租户 ACK 支持（Memory EventBus）
// ==========================================================================

// RegisterTenant 注册租户（创建租户专属的 ACK Channel）
func (m *eventBusManager) RegisterTenant(tenantID int, bufferSize int) error {
	// Reject after Close(). Memory's `closed` is a plain bool guarded by m.mu, so we
	// HOLD m.mu.RLock across the tenantChannelsMu section (lock order m.mu ->
	// tenantChannelsMu matches Close). That pins `closed` for the whole install: Close
	// needs m.mu.Lock to flip it, so it cannot run between the check and the install —
	// which prevents the late-registration leak (a channel nobody closes). The held
	// RLock is shared, so concurrent RegisterTenant calls do not block each other.
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return fmt.Errorf("eventbus closed")
	}

	if tenantID <= 0 {
		return fmt.Errorf("tenantID must be positive, got %d", tenantID)
	}

	if bufferSize <= 0 {
		bufferSize = 10000 // Memory EventBus 默认缓冲区较小（测试用）
	}

	m.tenantChannelsMu.Lock()
	defer m.tenantChannelsMu.Unlock()

	// 延迟初始化 map
	if m.tenantPublishResultChans == nil {
		m.tenantPublishResultChans = make(map[int]chan *PublishResult)
	}

	// 检查租户是否已注册
	if _, exists := m.tenantPublishResultChans[tenantID]; exists {
		return fmt.Errorf("tenant %d already registered", tenantID)
	}

	// 创建租户专属 ACK Channel
	m.tenantPublishResultChans[tenantID] = make(chan *PublishResult, bufferSize)

	logger.Info("Memory EventBus: Tenant ACK channel registered",
		"tenantID", tenantID,
		"bufferSize", bufferSize)

	return nil
}

// UnregisterTenant 注销租户（关闭并清理租户的 ACK Channel）
func (m *eventBusManager) UnregisterTenant(tenantID int) error {
	// PR2-core (Task 2, D5): reject after Close() — takes precedence over the
	// not-registered check. Plain-bool closed guarded by m.mu; snapshot under RLock.
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return fmt.Errorf("eventbus closed")
	}

	if tenantID <= 0 {
		return fmt.Errorf("tenantID must be positive, got %d", tenantID)
	}

	m.tenantChannelsMu.Lock()

	// 检查租户是否已注册
	ch, exists := m.tenantPublishResultChans[tenantID]
	if !exists {
		m.tenantChannelsMu.Unlock()
		return fmt.Errorf("tenant %d not registered", tenantID)
	}

	// D1: detach FIRST, then close. Combined with sendResultToChannel's RLock-held send,
	// the write Lock waits out any in-flight sender, so close(ch) is safe.
	// NO recover — D1 makes send-on-closed structurally impossible.
	delete(m.tenantPublishResultChans, tenantID) // detach
	close(ch)                                    // safe: no sender holds the RLock now
	m.tenantChannelsMu.Unlock()

	logger.Info("Memory EventBus: Tenant ACK channel unregistered",
		"tenantID", tenantID)

	return nil
}

// GetTenantPublishResultChannel 获取租户专属的异步发布结果通道
func (m *eventBusManager) GetTenantPublishResultChannel(tenantID int) <-chan *PublishResult {
	if tenantID == 0 {
		// 返回全局通道（向后兼容）
		return m.publishResultChan
	}

	m.tenantChannelsMu.RLock()
	defer m.tenantChannelsMu.RUnlock()

	if ch, exists := m.tenantPublishResultChans[tenantID]; exists {
		return ch
	}

	// 租户未注册，返回 nil
	logger.Warn("Memory EventBus: Tenant not registered, returning nil channel",
		"tenantID", tenantID)
	return nil
}

// GetRegisteredTenants 获取所有已注册的租户ID列表
func (m *eventBusManager) GetRegisteredTenants() []int {
	m.tenantChannelsMu.RLock()
	defer m.tenantChannelsMu.RUnlock()

	tenants := make([]int, 0, len(m.tenantPublishResultChans))
	for tenantID := range m.tenantPublishResultChans {
		tenants = append(tenants, tenantID)
	}

	return tenants
}

// sendResultToChannel admits a broker ACK result.
//
// PR2-core (Task 2, D5 — memory-backend only): byte-parallel with nats/kafka drivers.
// Holds tenantChannelsMu.RLock() ACROSS the non-blocking send so UnregisterTenant's
// write Lock cannot close the channel while a send is in flight — eliminates
// send-on-closed structurally, NO recover. Never falls back to the unmonitored global
// channel for a tenant result (spec §2.2).
//
// Adaptation vs nats/kafka: eventBusManager.closed is a plain bool guarded by m.mu
// (NOT an atomic.Bool). We snapshot it under m.mu.RLock() ONCE at entry and treat that
// snapshot as the admission gate for the whole call — the only consistent way to read
// a plain-bool closed without introducing a race. (Close flips closed to true under
// m.mu.Lock(), so a snapshot taken under RLock observes a stable value.)
func (m *eventBusManager) sendResultToChannel(result *PublishResult) AdmissionOutcome {
	// Snapshot closed under the same lock Close uses to flip it. Plain bool → must not
	// read it without the lock (would race with Close's write).
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return AdmissionRejectedFrozen
	}

	// 优先发送到租户专属通道
	if result.TenantID != 0 {
		m.tenantChannelsMu.RLock()
		tenantChan, exists := m.tenantPublishResultChans[result.TenantID]
		if !exists {
			m.tenantChannelsMu.RUnlock()
			return AdmissionRejectedUnregistered // do NOT fall back to global
		}
		// Lock held ACROSS the send (D1): UnregisterTenant's write Lock waits out this
		// critical section, so it cannot close(ch) while a send is pending.
		select {
		case tenantChan <- result:
			m.tenantChannelsMu.RUnlock()
			return AdmissionAccepted
		default:
			m.tenantChannelsMu.RUnlock()
			logger.Warn("Tenant ACK channel full, result not delivered",
				"tenantID", result.TenantID,
				"eventID", result.EventID,
				"topic", result.Topic)
			return AdmissionRejectedFull
		}
	}

	// TenantID == 0: legitimately global. Accepted only if it actually goes in.
	if m.publishResultChan != nil {
		select {
		case m.publishResultChan <- result:
			return AdmissionAccepted
		default:
			logger.Error("Global ACK channel full, ACK result not delivered",
				"eventID", result.EventID,
				"topic", result.Topic,
				"tenantID", result.TenantID)
			return AdmissionRejectedFull
		}
	}
	return AdmissionRejectedUnregistered
}
