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

// ========== 业务A：领域事件服务（持久化 + Envelope + Keyed-Worker池） ==========

type DomainEventService struct {
	eventBus eventbus.EventBus // 持久化EventBus实例
}

// 订单领域事件
type OrderCreatedDomainEvent struct {
	OrderID       string    `json:"order_id"`
	CustomerID    string    `json:"customer_id"`
	Amount        float64   `json:"amount"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	BusinessRules []string  `json:"business_rules"` // 业务规则验证记录
}

type OrderStatusChangedDomainEvent struct {
	OrderID   string    `json:"order_id"`
	OldStatus string    `json:"old_status"`
	NewStatus string    `json:"new_status"`
	ChangedAt time.Time `json:"changed_at"`
	ChangedBy string    `json:"changed_by"`
	Reason    string    `json:"reason"`
}

// 发布领域事件（使用Envelope + 持久化）
func (s *DomainEventService) PublishOrderCreated(ctx context.Context, orderID, customerID string, amount float64) error {
	event := OrderCreatedDomainEvent{
		OrderID:       orderID,
		CustomerID:    customerID,
		Amount:        amount,
		Status:        "CREATED",
		CreatedAt:     time.Now(),
		BusinessRules: []string{"amount_validation", "customer_verification"},
	}

	payload, _ := json.Marshal(event)

	// 创建Envelope（包含聚合ID和事件版本）
	envelope := eventbus.NewEnvelope(orderID, "OrderCreated", 1, payload)
	envelope.TraceID = fmt.Sprintf("domain-trace-%s", orderID)
	envelope.CorrelationID = fmt.Sprintf("order-flow-%s", orderID)

	// 发布到持久化流，确保事件不丢失
	return s.eventBus.PublishEnvelope(ctx, "domain.orders.events", envelope)
}

func (s *DomainEventService) PublishOrderStatusChanged(ctx context.Context, orderID, oldStatus, newStatus, changedBy, reason string, version int64) error {
	event := OrderStatusChangedDomainEvent{
		OrderID:   orderID,
		OldStatus: oldStatus,
		NewStatus: newStatus,
		ChangedAt: time.Now(),
		ChangedBy: changedBy,
		Reason:    reason,
	}

	payload, _ := json.Marshal(event)

	// 版本递增，确保事件顺序
	envelope := eventbus.NewEnvelope(orderID, "OrderStatusChanged", version, payload)
	envelope.TraceID = fmt.Sprintf("domain-trace-%s", orderID)

	return s.eventBus.PublishEnvelope(ctx, "domain.orders.events", envelope)
}

// 订阅领域事件（使用SubscribeEnvelope + Keyed-Worker池）
func (s *DomainEventService) SubscribeToDomainEvents(ctx context.Context) error {
	handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
		fmt.Printf("🏛️ [领域事件服务] 收到持久化领域事件:\n")
		fmt.Printf("  聚合ID: %s (路由到固定Worker，确保顺序)\n", envelope.AggregateID)
		fmt.Printf("  事件类型: %s\n", envelope.EventType)
		fmt.Printf("  事件版本: %d\n", envelope.EventVersion)
		fmt.Printf("  追踪ID: %s\n", envelope.TraceID)
		fmt.Printf("  处理模式: Keyed-Worker池 (顺序保证 + 持久化)\n")

		// 根据事件类型处理
		switch envelope.EventType {
		case "OrderCreated":
			var event OrderCreatedDomainEvent
			json.Unmarshal(envelope.Payload, &event)
			return s.handleOrderCreated(envelope.AggregateID, event)
		case "OrderStatusChanged":
			var event OrderStatusChangedDomainEvent
			json.Unmarshal(envelope.Payload, &event)
			return s.handleOrderStatusChanged(envelope.AggregateID, event)
		}

		return nil
	}

	// SubscribeEnvelope 自动启用Keyed-Worker池，确保同一聚合ID的事件顺序处理
	return s.eventBus.SubscribeEnvelope(ctx, "domain.orders.events", handler)
}

func (s *DomainEventService) handleOrderCreated(aggregateID string, event OrderCreatedDomainEvent) error {
	fmt.Printf("   🔄 处理订单创建领域事件: %s, 金额: %.2f\n", event.OrderID, event.Amount)
	fmt.Printf("   📋 业务规则验证: %v\n", event.BusinessRules)

	// 模拟领域事件处理：更新读模型、触发业务流程等
	time.Sleep(150 * time.Millisecond) // 模拟复杂业务逻辑

	fmt.Printf("   ✅ 订单 %s 领域事件处理完成\n\n", event.OrderID)
	return nil
}

func (s *DomainEventService) handleOrderStatusChanged(aggregateID string, event OrderStatusChangedDomainEvent) error {
	fmt.Printf("   🔄 处理订单状态变更领域事件: %s (%s -> %s)\n", event.OrderID, event.OldStatus, event.NewStatus)
	fmt.Printf("   👤 变更人: %s, 原因: %s\n", event.ChangedBy, event.Reason)

	// 模拟领域事件处理
	time.Sleep(120 * time.Millisecond)

	fmt.Printf("   ✅ 订单 %s 状态变更事件处理完成\n\n", event.OrderID)
	return nil
}

// ========== 业务B：简单消息服务（无持久化 + 普通消息 + 并发处理） ==========

type SimpleMessageService struct {
	eventBus eventbus.EventBus // 非持久化EventBus实例
}

// 简单通知消息
type SystemNotification struct {
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Priority  string    `json:"priority"`
}

type CacheInvalidation struct {
	CacheKey  string    `json:"cache_key"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// 发布简单消息（无Envelope，无持久化）
func (s *SimpleMessageService) SendSystemNotification(ctx context.Context, notificationType, title, content, priority string) error {
	notification := SystemNotification{
		Type:      notificationType,
		Title:     title,
		Content:   content,
		Timestamp: time.Now(),
		Priority:  priority,
	}

	message, _ := json.Marshal(notification)

	// 直接发布，无持久化，追求高性能
	return s.eventBus.Publish(ctx, "simple.notifications", message)
}

func (s *SimpleMessageService) InvalidateCache(ctx context.Context, cacheKey, reason string) error {
	invalidation := CacheInvalidation{
		CacheKey:  cacheKey,
		Reason:    reason,
		Timestamp: time.Now(),
	}

	message, _ := json.Marshal(invalidation)

	// 直接发布，无需顺序保证
	return s.eventBus.Publish(ctx, "simple.cache.invalidation", message)
}

// 订阅简单消息（使用Subscribe，直接并发处理）
func (s *SimpleMessageService) SubscribeToNotifications(ctx context.Context) error {
	handler := func(ctx context.Context, message []byte) error {
		fmt.Printf("📢 [简单消息服务] 收到系统通知:\n")

		var notification SystemNotification
		json.Unmarshal(message, &notification)
		fmt.Printf("  类型: %s, 优先级: %s\n", notification.Type, notification.Priority)
		fmt.Printf("  处理模式: 直接并发处理 (高性能，无持久化)\n")
		fmt.Printf("  通知内容: %s - %s\n", notification.Title, notification.Content)

		// 模拟快速处理
		time.Sleep(30 * time.Millisecond)
		fmt.Printf("   ⚡ 通知处理完成\n\n")
		return nil
	}

	// Subscribe 直接并发处理，无Keyed-Worker池
	return s.eventBus.Subscribe(ctx, "simple.notifications", handler)
}

func (s *SimpleMessageService) SubscribeToCacheInvalidation(ctx context.Context) error {
	handler := func(ctx context.Context, message []byte) error {
		fmt.Printf("🗑️ [简单消息服务] 收到缓存失效消息:\n")

		var invalidation CacheInvalidation
		json.Unmarshal(message, &invalidation)
		fmt.Printf("  缓存键: %s\n", invalidation.CacheKey)
		fmt.Printf("  处理模式: 直接并发处理 (高性能)\n")
		fmt.Printf("  失效原因: %s\n", invalidation.Reason)

		// 模拟缓存清理
		time.Sleep(20 * time.Millisecond)
		fmt.Printf("   🧹 缓存清理完成\n\n")
		return nil
	}

	return s.eventBus.Subscribe(ctx, "simple.cache.invalidation", handler)
}

// ========== 配置创建函数 ==========

func createDomainEventsConfig() *eventbus.EventBusConfig {
	// 演示环境使用内存模式
	return &eventbus.EventBusConfig{
		Type: "memory", // 演示用内存模式
	}
}

func createSimpleMessagesConfig() *eventbus.EventBusConfig {
	// 简单消息使用内存模式，追求高性能
	return &eventbus.EventBusConfig{
		Type: "memory", // 无持久化，高性能
	}
}

// ========== 主程序：演示双实例架构 ==========

func main() {
	fmt.Println("=== 领域事件 vs 简单消息：双EventBus实例架构演示 ===\n")

	// 0. 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	// 1. 创建领域事件EventBus（持久化 + Envelope + Keyed-Worker池）
	domainEventsCfg := createDomainEventsConfig()

	// 生产环境配置示例（注释掉，演示时使用内存模式）
	// domainEventsCfg := &eventbus.EventBusConfig{
	//     Type: "nats",
	//     NATS: eventbus.NATSConfig{
	//         URLs: []string{"nats://localhost:4222"},
	//         JetStream: eventbus.JetStreamConfig{
	//             Enabled: true,
	//             Stream: eventbus.StreamConfig{
	//                 Name:      "domain-events-stream",
	//                 Subjects:  []string{"domain.orders.events", "domain.users.events"},
	//                 Retention: "limits",
	//                 Storage:   "file",     // 文件存储，确保持久化
	//                 Replicas:  3,          // 3副本高可用
	//                 MaxAge:    30 * 24 * time.Hour, // 保留30天
	//                 MaxBytes:  10 * 1024 * 1024 * 1024, // 10GB
	//             },
	//         },
	//         Consumer: eventbus.ConsumerConfig{
	//             DurableName:   "domain-events-processor",
	//             AckPolicy:     "explicit",
	//             AckWait:       30 * time.Second,
	//             MaxDeliver:    3,
	//             ReplayPolicy:  "instant",
	//         },
	//     },
	// }

	domainEventsBus, err := eventbus.NewEventBus(domainEventsCfg)
	if err != nil {
		log.Fatalf("Failed to create domain events bus: %v", err)
	}
	defer domainEventsBus.Close()

	// 2. 创建简单消息EventBus（无持久化 + 普通消息 + 并发处理）
	simpleMessagesCfg := createSimpleMessagesConfig()

	simpleMessagesBus, err := eventbus.NewEventBus(simpleMessagesCfg)
	if err != nil {
		log.Fatalf("Failed to create simple messages bus: %v", err)
	}
	defer simpleMessagesBus.Close()

	// 3. 创建业务服务（使用不同的EventBus实例）
	domainEventService := &DomainEventService{eventBus: domainEventsBus}
	simpleMessageService := &SimpleMessageService{eventBus: simpleMessagesBus}

	ctx := context.Background()

	// 4. 启动订阅
	fmt.Println("🚀 启动双实例订阅...")

	// 领域事件订阅：SubscribeEnvelope -> Keyed-Worker池
	if err := domainEventService.SubscribeToDomainEvents(ctx); err != nil {
		log.Fatalf("Failed to subscribe to domain events: %v", err)
	}

	// 简单消息订阅：Subscribe -> 直接并发处理
	if err := simpleMessageService.SubscribeToNotifications(ctx); err != nil {
		log.Fatalf("Failed to subscribe to notifications: %v", err)
	}

	if err := simpleMessageService.SubscribeToCacheInvalidation(ctx); err != nil {
		log.Fatalf("Failed to subscribe to cache invalidation: %v", err)
	}

	time.Sleep(100 * time.Millisecond) // 等待订阅建立

	// 5. 演示双实例架构效果
	fmt.Println("📨 开始发布消息，演示双实例架构...\n")

	// 业务A：领域事件（持久化 + 顺序处理）
	fmt.Println("--- 业务A：领域事件（持久化 + Envelope + Keyed-Worker池） ---")
	domainEventService.PublishOrderCreated(ctx, "order-001", "customer-123", 99.99)
	domainEventService.PublishOrderStatusChanged(ctx, "order-001", "CREATED", "PAID", "system", "payment_received", 2)
	domainEventService.PublishOrderCreated(ctx, "order-002", "customer-456", 299.99)
	domainEventService.PublishOrderStatusChanged(ctx, "order-001", "PAID", "SHIPPED", "admin", "manual_ship", 3)

	time.Sleep(800 * time.Millisecond) // 等待领域事件处理

	// 业务B：简单消息（无持久化 + 并发处理）
	fmt.Println("--- 业务B：简单消息（无持久化 + 普通消息 + 并发处理） ---")
	simpleMessageService.SendSystemNotification(ctx, "info", "系统维护", "系统将于今晚进行维护", "low")
	simpleMessageService.InvalidateCache(ctx, "user:123:profile", "user_updated")
	simpleMessageService.SendSystemNotification(ctx, "warning", "磁盘空间", "磁盘使用率超过80%", "high")
	simpleMessageService.InvalidateCache(ctx, "product:456:details", "price_changed")

	time.Sleep(200 * time.Millisecond) // 等待简单消息处理

	// 6. 架构优势总结
	fmt.Println("\n=== 双EventBus实例架构优势 ===")
	fmt.Println("✅ 业务隔离:")
	fmt.Println("  🏛️ 领域事件：持久化 + 顺序保证 + 事务一致性")
	fmt.Println("  📢 简单消息：高性能 + 并发处理 + 无持久化开销")
	fmt.Println("✅ 技术选型:")
	fmt.Println("  🔒 领域事件：NATS JetStream/Kafka (可靠性优先)")
	fmt.Println("  ⚡ 简单消息：内存/Redis (性能优先)")
	fmt.Println("✅ 资源优化:")
	fmt.Println("  📊 按需配置，避免过度设计")
	fmt.Println("  🎯 精确匹配业务需求")

	fmt.Println("\n✅ 演示完成！推荐在生产环境使用双实例架构。")
}
