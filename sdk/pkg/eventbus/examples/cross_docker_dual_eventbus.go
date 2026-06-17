//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// ========== 业务A：领域事件服务（NATS JetStream + Envelope + Keyed-Worker池） ==========

type DomainEventService struct {
	eventBus eventbus.EventBus // NATS JetStream EventBus实例
}

// 订单领域事件
type OrderDomainEvent struct {
	OrderID       string    `json:"order_id"`
	CustomerID    string    `json:"customer_id"`
	EventType     string    `json:"event_type"`
	Amount        float64   `json:"amount"`
	Status        string    `json:"status"`
	Timestamp     time.Time `json:"timestamp"`
	BusinessRules []string  `json:"business_rules"`
}

// 发布领域事件（使用Envelope + JetStream持久化）
func (s *DomainEventService) PublishOrderEvent(ctx context.Context, orderID, customerID, eventType string, amount float64, status string, version int64) error {
	event := OrderDomainEvent{
		OrderID:       orderID,
		CustomerID:    customerID,
		EventType:     eventType,
		Amount:        amount,
		Status:        status,
		Timestamp:     time.Now(),
		BusinessRules: []string{"amount_validation", "customer_verification", "inventory_check"},
	}

	payload, _ := json.Marshal(event)

	// 创建Envelope（包含聚合ID和事件版本）
	envelope := eventbus.NewEnvelope(orderID, eventType, version, payload)
	envelope.TraceID = fmt.Sprintf("domain-trace-%s-%d", orderID, version)
	envelope.CorrelationID = fmt.Sprintf("order-flow-%s", orderID)

	// 发布到JetStream持久化流
	return s.eventBus.PublishEnvelope(ctx, "domain.orders.events", envelope)
}

// 订阅领域事件（使用SubscribeEnvelope + Keyed-Worker池）
func (s *DomainEventService) SubscribeToDomainEvents(ctx context.Context) error {
	handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
		fmt.Printf("🏛️ [领域事件服务] 收到JetStream持久化事件:\n")
		fmt.Printf("  聚合ID: %s (路由到固定Worker)\n", envelope.AggregateID)
		fmt.Printf("  事件类型: %s\n", envelope.EventType)
		fmt.Printf("  事件版本: %d\n", envelope.EventVersion)
		fmt.Printf("  追踪ID: %s\n", envelope.TraceID)
		fmt.Printf("  处理模式: Keyed-Worker池 (顺序保证 + JetStream持久化)\n")

		var event OrderDomainEvent
		json.Unmarshal(envelope.Payload, &event)
		fmt.Printf("  事件详情: %+v\n", event)

		// 模拟领域事件处理：更新读模型、触发业务流程等
		return s.handleDomainEvent(envelope.AggregateID, event)
	}

	// SubscribeEnvelope 自动启用Keyed-Worker池
	return s.eventBus.SubscribeEnvelope(ctx, "domain.orders.events", handler)
}

func (s *DomainEventService) handleDomainEvent(aggregateID string, event OrderDomainEvent) error {
	fmt.Printf("   🔄 处理领域事件: %s - %s\n", event.OrderID, event.EventType)
	fmt.Printf("   📋 业务规则: %v\n", event.BusinessRules)

	// 模拟复杂的领域事件处理
	time.Sleep(200 * time.Millisecond)

	fmt.Printf("   ✅ 领域事件 %s 处理完成\n\n", event.OrderID)
	return nil
}

// ========== 业务B：简单消息服务（NATS Core + 普通消息 + 并发处理） ==========

type SimpleMessageService struct {
	eventBus eventbus.EventBus // NATS Core EventBus实例
}

// 系统通知消息
type SystemNotification struct {
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Priority  string    `json:"priority"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}

// 缓存失效消息
type CacheInvalidation struct {
	CacheKey  string    `json:"cache_key"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
	TTL       int       `json:"ttl"`
}

// 发布系统通知（使用NATS Core，无持久化）
func (s *SimpleMessageService) SendNotification(ctx context.Context, notificationType, title, content, priority, source string) error {
	notification := SystemNotification{
		Type:      notificationType,
		Title:     title,
		Content:   content,
		Priority:  priority,
		Timestamp: time.Now(),
		Source:    source,
	}

	message, _ := json.Marshal(notification)

	// 使用NATS Core发布，跨Docker但无持久化
	return s.eventBus.Publish(ctx, "simple.notifications", message)
}

// 发布缓存失效消息
func (s *SimpleMessageService) InvalidateCache(ctx context.Context, cacheKey, reason string, ttl int) error {
	invalidation := CacheInvalidation{
		CacheKey:  cacheKey,
		Reason:    reason,
		Timestamp: time.Now(),
		TTL:       ttl,
	}

	message, _ := json.Marshal(invalidation)

	// 使用NATS Core发布，跨Docker高性能
	return s.eventBus.Publish(ctx, "simple.cache.invalidation", message)
}

// 订阅系统通知（使用Subscribe，直接并发处理）
func (s *SimpleMessageService) SubscribeToNotifications(ctx context.Context) error {
	handler := func(ctx context.Context, message []byte) error {
		fmt.Printf("📢 [简单消息服务] 收到NATS Core通知:\n")

		var notification SystemNotification
		json.Unmarshal(message, &notification)
		fmt.Printf("  类型: %s, 优先级: %s, 来源: %s\n", notification.Type, notification.Priority, notification.Source)
		fmt.Printf("  处理模式: NATS Core直接并发 (跨Docker高性能)\n")
		fmt.Printf("  通知内容: %s - %s\n", notification.Title, notification.Content)

		// 模拟快速处理
		time.Sleep(50 * time.Millisecond)
		fmt.Printf("   ⚡ 通知处理完成\n\n")
		return nil
	}

	// Subscribe 使用NATS Core，直接并发处理
	return s.eventBus.Subscribe(ctx, "simple.notifications", handler)
}

// 订阅缓存失效消息
func (s *SimpleMessageService) SubscribeToCacheInvalidation(ctx context.Context) error {
	handler := func(ctx context.Context, message []byte) error {
		fmt.Printf("🗑️ [简单消息服务] 收到NATS Core缓存失效:\n")

		var invalidation CacheInvalidation
		json.Unmarshal(message, &invalidation)
		fmt.Printf("  缓存键: %s, TTL: %d秒\n", invalidation.CacheKey, invalidation.TTL)
		fmt.Printf("  处理模式: NATS Core直接并发 (跨Docker)\n")
		fmt.Printf("  失效原因: %s\n", invalidation.Reason)

		// 模拟缓存清理
		time.Sleep(30 * time.Millisecond)
		fmt.Printf("   🧹 缓存清理完成\n\n")
		return nil
	}

	return s.eventBus.Subscribe(ctx, "simple.cache.invalidation", handler)
}

// ========== 配置创建函数 ==========

func createDomainEventsConfig() *eventbus.EventBusConfig {
	// 业务A：NATS JetStream配置（持久化）
	return &eventbus.EventBusConfig{
		Type: "nats",
		NATS: eventbus.NATSConfig{
			URLs: []string{"nats://localhost:4222"},
			JetStream: eventbus.JetStreamConfig{
				Enabled: true,
				Stream: eventbus.StreamConfig{
					Name:      "domain-events-stream",
					Subjects:  []string{"domain.orders.events", "domain.users.events", "domain.payments.events"},
					Retention: "limits",
					Storage:   "file",             // 文件存储，确保持久化
					Replicas:  1,                  // 单节点演示，生产环境建议3副本
					MaxAge:    24 * time.Hour,     // 保留24小时
					MaxBytes:  1024 * 1024 * 1024, // 1GB存储
					Discard:   "old",              // 超限时丢弃旧消息
				},
				Consumer: eventbus.NATSConsumerConfig{
					DurableName:   "domain-events-processor",
					AckPolicy:     "explicit",
					ReplayPolicy:  "instant",
					MaxAckPending: 1000,
				},
			},
		},
	}
}

func createSimpleMessagesConfig() *eventbus.EventBusConfig {
	// 业务B：NATS Core配置（无持久化，高性能）
	return &eventbus.EventBusConfig{
		Type: "nats",
		NATS: eventbus.NATSConfig{
			URLs: []string{"nats://localhost:4222"},
			JetStream: eventbus.JetStreamConfig{
				Enabled: false, // 关闭JetStream，使用NATS Core
			},
		},
	}
}

// ========== 主程序：演示跨Docker双NATS实例架构 ==========

func main() {
	fmt.Println("=== 跨Docker双NATS EventBus实例架构演示 ===")
	fmt.Println("业务A: NATS JetStream (持久化 + Envelope + Keyed-Worker池)")
	fmt.Println("业务B: NATS Core (跨Docker + 高性能 + 并发处理)\n")

	// 0. 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	// 1. 创建领域事件EventBus（NATS JetStream）
	domainEventsCfg := createDomainEventsConfig()
	domainEventsBus, err := eventbus.NewEventBus(domainEventsCfg)
	if err != nil {
		log.Fatalf("Failed to create domain events bus: %v", err)
	}
	defer domainEventsBus.Close()

	// 2. 创建简单消息EventBus（NATS Core）
	simpleMessagesCfg := createSimpleMessagesConfig()
	simpleMessagesBus, err := eventbus.NewEventBus(simpleMessagesCfg)
	if err != nil {
		log.Fatalf("Failed to create simple messages bus: %v", err)
	}
	defer simpleMessagesBus.Close()

	// 3. 创建业务服务
	domainEventService := &DomainEventService{eventBus: domainEventsBus}
	simpleMessageService := &SimpleMessageService{eventBus: simpleMessagesBus}

	ctx := context.Background()

	// 4. 启动订阅
	fmt.Println("🚀 启动跨Docker双NATS订阅...")

	// 领域事件订阅：JetStream + SubscribeEnvelope
	if err := domainEventService.SubscribeToDomainEvents(ctx); err != nil {
		log.Fatalf("Failed to subscribe to domain events: %v", err)
	}

	// 简单消息订阅：NATS Core + Subscribe
	if err := simpleMessageService.SubscribeToNotifications(ctx); err != nil {
		log.Fatalf("Failed to subscribe to notifications: %v", err)
	}

	if err := simpleMessageService.SubscribeToCacheInvalidation(ctx); err != nil {
		log.Fatalf("Failed to subscribe to cache invalidation: %v", err)
	}

	time.Sleep(200 * time.Millisecond) // 等待订阅建立

	// 5. 演示跨Docker消息传递
	fmt.Println("📨 开始发布消息，演示跨Docker架构...\n")

	// 业务A：领域事件（JetStream持久化）
	fmt.Println("--- 业务A：领域事件（NATS JetStream + 持久化） ---")
	domainEventService.PublishOrderEvent(ctx, "order-001", "customer-123", "OrderCreated", 99.99, "CREATED", 1)
	domainEventService.PublishOrderEvent(ctx, "order-001", "customer-123", "OrderPaid", 99.99, "PAID", 2)
	domainEventService.PublishOrderEvent(ctx, "order-002", "customer-456", "OrderCreated", 299.99, "CREATED", 1)

	time.Sleep(800 * time.Millisecond) // 等待领域事件处理

	// 业务B：简单消息（NATS Core高性能）
	fmt.Println("--- 业务B：简单消息（NATS Core + 跨Docker） ---")
	simpleMessageService.SendNotification(ctx, "info", "系统维护", "系统将于今晚进行维护", "low", "system-service")
	simpleMessageService.InvalidateCache(ctx, "user:123:profile", "user_updated", 300)
	simpleMessageService.SendNotification(ctx, "warning", "磁盘空间", "磁盘使用率超过80%", "high", "monitoring-service")
	simpleMessageService.InvalidateCache(ctx, "product:456:details", "price_changed", 600)

	time.Sleep(300 * time.Millisecond) // 等待简单消息处理

	// 6. 架构优势总结
	fmt.Println("\n=== 跨Docker双NATS EventBus架构优势 ===")
	fmt.Println("✅ 跨Docker通信:")
	fmt.Println("  🏛️ 领域事件：NATS JetStream (持久化 + 可靠性)")
	fmt.Println("  📢 简单消息：NATS Core (高性能 + 低延迟)")
	fmt.Println("✅ 业务隔离:")
	fmt.Println("  🔒 不同流/主题，完全隔离")
	fmt.Println("  ⚡ 不同处理模式，精确优化")
	fmt.Println("✅ 技术优势:")
	fmt.Println("  📊 JetStream：事务一致性 + 事件重放")
	fmt.Println("  🚀 NATS Core：极致性能 + 简单可靠")

	fmt.Println("\n✅ 演示完成！推荐在跨Docker环境使用此架构。")
}
