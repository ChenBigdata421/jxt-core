package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// memoryEventBus 内存事件总线实现（用于测试和开发）
type memoryEventBus struct {
	subscribers map[string][]*handlerWrapper // ⭐ 修改：存储 handlerWrapper 而非 MessageHandler
	mu          sync.RWMutex
	closed      bool
	metrics     *Metrics

	// ⭐ 新增：Hollywood Actor Pool（统一架构）
	globalActorPool *HollywoodActorPool

	// ⭐ 新增：Round-Robin 计数器（用于无聚合ID的消息）
	roundRobinCounter atomic.Uint64
}

// memoryPublisher 内存发布器
type memoryPublisher struct {
	eventBus              *memoryEventBus
	topicConfigStrategy   TopicConfigStrategy
	topicConfigStrategyMu sync.RWMutex
}

// memorySubscriber 内存订阅器
type memorySubscriber struct {
	eventBus              *memoryEventBus
	topicConfigStrategy   TopicConfigStrategy
	topicConfigStrategyMu sync.RWMutex
}

// NewMemoryEventBus 创建内存事件总线
func NewMemoryEventBus() EventBus {
	bus := &memoryEventBus{
		subscribers: make(map[string][]*handlerWrapper),
		metrics: &Metrics{
			LastHealthCheck:   time.Now(),
			HealthCheckStatus: "healthy",
		},
	}

	// ⭐ 初始化 Hollywood Actor Pool
	// ⭐ 使用唯一的 namespace 避免 Prometheus 指标冲突
	// ⭐ 注意：Prometheus 指标名称只能包含字母、数字和下划线，且不能以数字开头
	namespace := fmt.Sprintf("memory_eventbus_n%d", time.Now().UnixNano())

	poolConfig := HollywoodActorPoolConfig{
		PoolSize:    256,
		InboxSize:   1000,
		MaxRestarts: 3,
	}

	metricsCollector := NewPrometheusActorPoolMetricsCollector(namespace)
	pool := NewHollywoodActorPool(poolConfig, metricsCollector)

	bus.globalActorPool = pool
	logger.Info("Memory EventBus using Hollywood Actor Pool",
		"poolSize", poolConfig.PoolSize,
		"inboxSize", poolConfig.InboxSize,
		"maxRestarts", poolConfig.MaxRestarts,
		"namespace", namespace)

	return &eventBusManager{
		publisher: &memoryPublisher{
			eventBus:            bus,
			topicConfigStrategy: StrategyCreateOrUpdate, // 默认策略
		},
		subscriber: &memorySubscriber{
			eventBus:            bus,
			topicConfigStrategy: StrategyCreateOrUpdate, // 默认策略
		},
		metrics: bus.metrics,
		healthStatus: &HealthStatus{
			Overall:   "healthy",
			Timestamp: time.Now(),
			Infrastructure: InfrastructureHealth{
				EventBus: EventBusHealthMetrics{
					ConnectionStatus: "connected",
				},
			},
			Details: make(map[string]interface{}),
		},
		// 🚀 初始化异步发布结果通道（缓冲区大小：10000）
		publishResultChan: make(chan *PublishResult, 10000),
		// 🔧 初始化主题配置映射
		topicConfigs: make(map[string]TopicOptions),
		// ⭐ 修复：初始化 metricsCollector（NewMemoryEventBus 不接受配置，使用 NoOp）
		metricsCollector: &NoOpMetricsCollector{},
	}
}

// Publish 发布消息
func (m *memoryEventBus) Publish(ctx context.Context, topic string, message []byte) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return fmt.Errorf("memory eventbus is closed")
	}

	handlers, exists := m.subscribers[topic]
	if !exists || len(handlers) == 0 {
		m.mu.RUnlock()
		logger.Debug("No subscribers for topic", "topic", topic)
		return nil
	}

	// 🔧 修复并发安全问题：创建handlers的副本，避免在异步goroutine中使用可能被修改的切片
	handlersCopy := make([]*handlerWrapper, len(handlers))
	copy(handlersCopy, handlers)
	m.mu.RUnlock()

	// ⭐ 使用 Hollywood Actor Pool 处理消息
	return m.publishWithActorPool(ctx, topic, message, handlersCopy)
}

// publishWithActorPool 使用 Hollywood Actor Pool 处理消息
func (m *memoryEventBus) publishWithActorPool(ctx context.Context, topic string, message []byte, handlers []*handlerWrapper) error {
	// 提取 aggregateID（如果是 Envelope）
	// ⭐ 使用与 Kafka/NATS 一致的提取逻辑
	aggregateID, _ := ExtractAggregateID(message, nil, nil, "")

	// ⭐ 确定 routingKey（所有 handler 共享同一个 routingKey）
	// 策略：
	// - 有聚合ID：使用 aggregateID（保证同一聚合的消息顺序）
	// - 无聚合ID：使用 Round-Robin（最大并发，但同一条消息的所有 handler 路由到同一 Actor）
	routingKey := aggregateID
	if routingKey == "" {
		// 无聚合ID：使用 Round-Robin（每条消息递增一次）
		index := m.roundRobinCounter.Add(1)
		routingKey = fmt.Sprintf("rr-%d", index)
	}

	// 对每个 handler 提交到 Actor Pool
	for _, wrapper := range handlers {
		// 创建 AggregateMessage
		aggMsg := &AggregateMessage{
			Topic:       topic,
			Value:       message,
			AggregateID: routingKey, // ⭐ 所有 handler 使用相同的 routingKey
			Context:     ctx,
			Done:        make(chan error, 1),
			Handler:     wrapper.handler,
			IsEnvelope:  wrapper.isEnvelope, // ⭐ 设置 Envelope 标记
		}

		// 提交到 Actor Pool
		if err := m.globalActorPool.ProcessMessage(ctx, aggMsg); err != nil {
			logger.Error("Failed to submit message to actor pool", "error", err)
			m.metrics.ConsumeErrors++
			continue
		}

		// ⭐ 修复：使用 time.NewTimer 避免泄漏
		// 异步等待结果（不阻塞发布者）
		go func(msg *AggregateMessage, isEnvelope bool) {
			timer := time.NewTimer(30 * time.Second)
			defer timer.Stop()

			select {
			case err := <-msg.Done:
				if err != nil {
					// ⚠️ Memory EventBus 限制：无法实现 at-least-once 语义
					// 原因：缺乏持久化机制，消息处理失败后无法重新投递
					// 重试会导致无限循环（如果 handler 总是失败）
					if isEnvelope {
						logger.Warn("Envelope message processing failed (at-most-once semantics)", "topic", topic, "error", err)
					} else {
						logger.Error("Regular message handler failed", "topic", topic, "error", err)
					}
					m.metrics.ConsumeErrors++
				} else {
					m.metrics.MessagesConsumed++
				}
			case <-timer.C:
				logger.Error("Message processing timeout", "topic", topic)
				m.metrics.ConsumeErrors++
			}
		}(aggMsg, wrapper.isEnvelope)
	}

	m.metrics.MessagesPublished++
	logger.Debug("Message published to memory eventbus via actor pool", "topic", topic, "handlers", len(handlers), "routingKey", routingKey)
	return nil
}

// Subscribe 订阅消息
func (m *memoryEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("memory eventbus is closed")
	}

	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	// ⭐ 普通 Subscribe：isEnvelope = false（at-most-once 语义）
	wrapper := &handlerWrapper{
		handler:    handler,
		isEnvelope: false,
	}
	m.subscribers[topic] = append(m.subscribers[topic], wrapper)
	logger.Info("Subscribed to topic in memory eventbus", "topic", topic, "totalSubscribers", len(m.subscribers[topic]))
	return nil
}

// SubscribeEnvelope 订阅 Envelope 消息
func (m *memoryEventBus) SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error {
	// 包装EnvelopeHandler为MessageHandler
	wrappedHandler := func(ctx context.Context, message []byte) error {
		// 尝试解析为Envelope
		envelope, err := FromBytes(message)
		if err != nil {
			logger.Error("Failed to parse envelope message", "topic", topic, "error", err)
			return fmt.Errorf("failed to parse envelope: %w", err)
		}

		// 调用业务处理器
		return handler(ctx, envelope)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("memory eventbus is closed")
	}

	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	// ⭐ SubscribeEnvelope：isEnvelope = true（at-least-once 语义）
	wrapper := &handlerWrapper{
		handler:    wrappedHandler,
		isEnvelope: true,
	}
	m.subscribers[topic] = append(m.subscribers[topic], wrapper)
	logger.Info("Subscribed to envelope topic in memory eventbus", "topic", topic, "totalSubscribers", len(m.subscribers[topic]))
	return nil
}

// HealthCheck 健康检查
func (m *memoryEventBus) HealthCheck(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("memory eventbus is closed")
	}

	m.metrics.LastHealthCheck = time.Now()
	m.metrics.HealthCheckStatus = "healthy"
	m.metrics.ActiveConnections = 1 // 内存实现始终有一个连接

	logger.Debug("Memory eventbus health check passed")
	return nil
}

// Close 关闭
func (m *memoryEventBus) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	// ⭐ 关闭 Hollywood Actor Pool
	if m.globalActorPool != nil {
		m.globalActorPool.Stop()
	}

	m.subscribers = make(map[string][]*handlerWrapper)
	logger.Info("Memory eventbus closed")
	return nil
}

// RegisterReconnectCallback 注册重连回调（内存实现不需要重连）
func (m *memoryEventBus) RegisterReconnectCallback(callback func(ctx context.Context) error) error {
	logger.Debug("Reconnect callback registered for memory eventbus (no-op)")
	return nil
}

// memoryPublisher 实现

// Publish 发布消息
func (p *memoryPublisher) Publish(ctx context.Context, topic string, message []byte) error {
	return p.eventBus.Publish(ctx, topic, message)
}

// Close 关闭发布器
func (p *memoryPublisher) Close() error {
	logger.Debug("Memory publisher closed")
	return nil
}

// memorySubscriber 实现

// Subscribe 订阅消息
func (s *memorySubscriber) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	return s.eventBus.Subscribe(ctx, topic, handler)
}

// Close 关闭订阅器
func (s *memorySubscriber) Close() error {
	logger.Debug("Memory subscriber closed")
	return nil
}

// 更新 eventBusManager 的 initMemory 方法
func (m *eventBusManager) initMemory() (EventBus, error) {
	bus := &memoryEventBus{
		subscribers: make(map[string][]*handlerWrapper),
		metrics: &Metrics{
			LastHealthCheck:   time.Now(),
			HealthCheckStatus: "healthy",
		},
	}

	// ⭐ 初始化 Hollywood Actor Pool
	// ⭐ 使用唯一的 namespace 避免 Prometheus 指标冲突
	// ⭐ 注意：Prometheus 指标名称只能包含字母、数字和下划线，且不能以数字开头
	namespace := fmt.Sprintf("memory_eventbus_n%d", time.Now().UnixNano())

	poolConfig := HollywoodActorPoolConfig{
		PoolSize:    256,
		InboxSize:   1000,
		MaxRestarts: 3,
	}

	metricsCollector := NewPrometheusActorPoolMetricsCollector(namespace)
	pool := NewHollywoodActorPool(poolConfig, metricsCollector)

	bus.globalActorPool = pool
	logger.Info("Memory EventBus using Hollywood Actor Pool",
		"poolSize", poolConfig.PoolSize,
		"inboxSize", poolConfig.InboxSize,
		"maxRestarts", poolConfig.MaxRestarts,
		"namespace", namespace)

	m.publisher = &memoryPublisher{
		eventBus:            bus,
		topicConfigStrategy: StrategyCreateOrUpdate, // 默认策略
	}
	m.subscriber = &memorySubscriber{
		eventBus:            bus,
		topicConfigStrategy: StrategyCreateOrUpdate, // 默认策略
	}
	m.metrics = bus.metrics
	m.healthStatus = &HealthStatus{
		Overall:   "healthy",
		Timestamp: time.Now(),
		Infrastructure: InfrastructureHealth{
			EventBus: EventBusHealthMetrics{
				ConnectionStatus: "connected",
			},
		},
		Details: map[string]interface{}{"type": "memory"},
	}

	// ⭐ 修复：确保 metricsCollector 被保留（NewEventBus 已经初始化了）
	// 如果 metricsCollector 为 nil（不应该发生），则使用 NoOp
	// 注意：NewEventBus 在调用 initMemory 之前已经设置了 m.metricsCollector
	// 所以这里不需要重新设置，除非它为 nil
	if m.metricsCollector == nil {
		m.metricsCollector = &NoOpMetricsCollector{}
		logger.Warn("metricsCollector was nil in initMemory, using NoOp")
	}

	logger.Info("Memory eventbus initialized successfully")
	return m, nil
}

// SetTopicConfigStrategy 设置主题配置策略（memoryPublisher）
func (m *memoryPublisher) SetTopicConfigStrategy(strategy TopicConfigStrategy) {
	m.topicConfigStrategyMu.Lock()
	defer m.topicConfigStrategyMu.Unlock()
	m.topicConfigStrategy = strategy
	logger.Debug("Memory publisher topic config strategy updated", "strategy", string(strategy))
}

// GetTopicConfigStrategy 获取主题配置策略（memoryPublisher）
func (m *memoryPublisher) GetTopicConfigStrategy() TopicConfigStrategy {
	m.topicConfigStrategyMu.RLock()
	defer m.topicConfigStrategyMu.RUnlock()
	return m.topicConfigStrategy
}

// SetTopicConfigStrategy 设置主题配置策略（memorySubscriber）
func (m *memorySubscriber) SetTopicConfigStrategy(strategy TopicConfigStrategy) {
	m.topicConfigStrategyMu.Lock()
	defer m.topicConfigStrategyMu.Unlock()
	m.topicConfigStrategy = strategy
	logger.Debug("Memory subscriber topic config strategy updated", "strategy", string(strategy))
}

// GetTopicConfigStrategy 获取主题配置策略（memorySubscriber）
func (m *memorySubscriber) GetTopicConfigStrategy() TopicConfigStrategy {
	m.topicConfigStrategyMu.RLock()
	defer m.topicConfigStrategyMu.RUnlock()
	return m.topicConfigStrategy
}
