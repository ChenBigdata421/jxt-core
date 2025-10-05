package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// memoryEventBus 内存事件总线实现（用于测试和开发）
type memoryEventBus struct {
	subscribers map[string][]MessageHandler
	mu          sync.RWMutex
	closed      bool
	metrics     *Metrics
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
		subscribers: make(map[string][]MessageHandler),
		metrics: &Metrics{
			LastHealthCheck:   time.Now(),
			HealthCheckStatus: "healthy",
		},
	}

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
	}
}

// Publish 发布消息
func (m *memoryEventBus) Publish(ctx context.Context, topic string, message []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("memory eventbus is closed")
	}

	handlers, exists := m.subscribers[topic]
	if !exists || len(handlers) == 0 {
		logger.Debug("No subscribers for topic", "topic", topic)
		return nil
	}

	// 异步处理消息，避免阻塞发布者
	go func() {
		for _, handler := range handlers {
			go func(h MessageHandler) {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("Message handler panicked", "topic", topic, "panic", r)
						m.metrics.ConsumeErrors++
					}
				}()

				if err := h(ctx, message); err != nil {
					logger.Error("Message handler failed", "topic", topic, "error", err)
					m.metrics.ConsumeErrors++
				} else {
					m.metrics.MessagesConsumed++
				}
			}(handler)
		}
	}()

	m.metrics.MessagesPublished++
	logger.Debug("Message published to memory eventbus", "topic", topic, "subscribers", len(handlers))
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

	m.subscribers[topic] = append(m.subscribers[topic], handler)
	logger.Info("Subscribed to topic in memory eventbus", "topic", topic, "totalSubscribers", len(m.subscribers[topic]))
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
	m.subscribers = make(map[string][]MessageHandler)
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
		subscribers: make(map[string][]MessageHandler),
		metrics: &Metrics{
			LastHealthCheck:   time.Now(),
			HealthCheckStatus: "healthy",
		},
	}

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
