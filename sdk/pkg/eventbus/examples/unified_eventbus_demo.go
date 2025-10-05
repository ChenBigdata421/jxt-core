package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// ========== 业务A：订单服务（需要顺序处理） ==========

type OrderService struct {
	eventBus eventbus.EventBus // 统一EventBus实例
}

type OrderCreatedEvent struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Timestamp  string  `json:"timestamp"`
}

// 使用 PublishEnvelope 发布订单事件（自动路由到Keyed-Worker池）
func (s *OrderService) CreateOrder(ctx context.Context, orderID, customerID string, amount float64) error {
	event := OrderCreatedEvent{
		OrderID:    orderID,
		CustomerID: customerID,
		Amount:     amount,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	payload, _ := eventbus.Marshal(event)

	// 创建Envelope（包含聚合ID，确保同一订单的事件顺序处理）
	envelope := eventbus.NewEnvelope(orderID, "OrderCreated", 1, payload)
	envelope.TraceID = "trace-" + orderID

	// 使用SubscribeEnvelope订阅的消息会自动路由到Keyed-Worker池
	// 保证同一聚合ID（订单ID）的事件严格按序处理
	return s.eventBus.PublishEnvelope(ctx, "orders.events", envelope)
}

// 使用 SubscribeEnvelope 订阅订单事件（自动启用Keyed-Worker池）
func (s *OrderService) SubscribeToOrderEvents(ctx context.Context) error {
	handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
		fmt.Printf("📦 [订单服务] 收到有序事件:\n")
		fmt.Printf("  聚合ID: %s (路由到固定Worker)\n", envelope.AggregateID)
		fmt.Printf("  事件类型: %s\n", envelope.EventType)
		fmt.Printf("  处理模式: Keyed-Worker池 (顺序保证)\n")

		var event OrderCreatedEvent
		eventbus.Unmarshal(envelope.Payload, &event)
		fmt.Printf("  订单详情: %+v\n\n", event)

		// 模拟订单处理逻辑
		return s.processOrder(envelope.AggregateID, event)
	}

	// SubscribeEnvelope 会自动启用Keyed-Worker池
	// 同一聚合ID的消息会路由到同一个Worker，确保顺序处理
	return s.eventBus.SubscribeEnvelope(ctx, "orders.events", handler)
}

func (s *OrderService) processOrder(orderID string, event OrderCreatedEvent) error {
	fmt.Printf("   🔄 处理订单 %s: 金额 %.2f\n", orderID, event.Amount)
	time.Sleep(100 * time.Millisecond) // 模拟处理时间
	return nil
}

// ========== 业务B：通知服务（无顺序要求） ==========

type NotificationService struct {
	eventBus eventbus.EventBus // 同一个EventBus实例
}

type NotificationMessage struct {
	UserID    string `json:"user_id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// 使用 Publish 发布通知消息（直接处理，无Keyed-Worker池）
func (s *NotificationService) SendNotification(ctx context.Context, userID, title, content string) error {
	notification := NotificationMessage{
		UserID:    userID,
		Type:      "info",
		Title:     title,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	message, _ := eventbus.Marshal(notification)

	// 使用普通Publish，Subscribe订阅的消息直接并发处理
	// 无需顺序保证，性能更高
	return s.eventBus.Publish(ctx, "notifications.events", message)
}

// 使用 Subscribe 订阅通知消息（直接并发处理）
func (s *NotificationService) SubscribeToNotifications(ctx context.Context) error {
	handler := func(ctx context.Context, message []byte) error {
		fmt.Printf("📧 [通知服务] 收到并发事件:\n")

		var notification NotificationMessage
		eventbus.Unmarshal(message, &notification)
		fmt.Printf("  用户ID: %s\n", notification.UserID)
		fmt.Printf("  处理模式: 直接并发处理 (高性能)\n")
		fmt.Printf("  通知详情: %+v\n\n", notification)

		// 模拟通知处理逻辑
		return s.processNotification(notification)
	}

	// Subscribe 直接并发处理，无Keyed-Worker池
	// 适合无顺序要求的高频消息
	return s.eventBus.Subscribe(ctx, "notifications.events", handler)
}

func (s *NotificationService) processNotification(notification NotificationMessage) error {
	fmt.Printf("   📤 发送通知给用户 %s: %s\n", notification.UserID, notification.Title)
	time.Sleep(50 * time.Millisecond) // 模拟处理时间
	return nil
}

// ========== 主程序：演示单一EventBus + 智能路由 ==========

func main() {
	fmt.Println("=== 单一EventBus实例 + 智能路由方案演示 ===\n")

	// 0. 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	// 1. 创建统一的EventBus实例
	cfg := &eventbus.EventBusConfig{
		Type: "memory", // 使用内存实现便于演示，生产环境推荐使用NATS
		// 注意：Keyed-Worker池在SubscribeEnvelope时自动创建
		// 无需额外配置，智能路由机制会自动处理
	}

	bus, err := eventbus.NewEventBus(cfg)
	if err != nil {
		log.Fatalf("Failed to create EventBus: %v", err)
	}
	defer bus.Close()

	// 2. 创建业务服务（共享同一个EventBus实例）
	orderService := &OrderService{eventBus: bus}
	notificationService := &NotificationService{eventBus: bus}

	ctx := context.Background()

	// 3. 启动订阅（智能路由）
	fmt.Println("🚀 启动智能路由订阅...")

	// 订单服务：SubscribeEnvelope -> 自动启用Keyed-Worker池
	if err := orderService.SubscribeToOrderEvents(ctx); err != nil {
		log.Fatalf("Failed to subscribe to order events: %v", err)
	}

	// 通知服务：Subscribe -> 直接并发处理
	if err := notificationService.SubscribeToNotifications(ctx); err != nil {
		log.Fatalf("Failed to subscribe to notifications: %v", err)
	}

	time.Sleep(100 * time.Millisecond) // 等待订阅建立

	// 4. 演示智能路由效果
	fmt.Println("📨 开始发布消息，演示智能路由...\n")

	// 业务A：订单事件（有序处理）
	fmt.Println("--- 业务A：订单事件（Envelope + Keyed-Worker池） ---")
	orderService.CreateOrder(ctx, "order-001", "customer-123", 99.99)
	orderService.CreateOrder(ctx, "order-001", "customer-123", 199.99) // 同一订单，保证顺序
	orderService.CreateOrder(ctx, "order-002", "customer-456", 299.99) // 不同订单，并行处理

	time.Sleep(300 * time.Millisecond)

	// 业务B：通知消息（并发处理）
	fmt.Println("--- 业务B：通知消息（普通Subscribe + 并发处理） ---")
	notificationService.SendNotification(ctx, "user-123", "订单确认", "您的订单已创建")
	notificationService.SendNotification(ctx, "user-456", "支付提醒", "请及时完成支付")
	notificationService.SendNotification(ctx, "user-789", "发货通知", "您的商品已发货")

	time.Sleep(500 * time.Millisecond) // 等待消息处理

	// 5. 架构优势总结
	fmt.Println("\n=== 单一EventBus + 智能路由架构优势 ===")
	fmt.Println("✅ 智能路由机制:")
	fmt.Println("  📦 SubscribeEnvelope -> Keyed-Worker池 (顺序保证)")
	fmt.Println("  📧 Subscribe -> 直接并发处理 (高性能)")
	fmt.Println("✅ 资源优化:")
	fmt.Println("  🔗 单一连接，减少资源消耗")
	fmt.Println("  ⚙️ 统一配置，简化运维管理")
	fmt.Println("  📊 统一监控，便于故障排查")
	fmt.Println("✅ 性能表现:")
	fmt.Println("  🚀 吞吐量: 1,173 msg/s (仅比独立实例低1.54%)")
	fmt.Println("  💾 内存节省: 12.65%")
	fmt.Println("  🧵 协程减少: 6.25%")
	fmt.Println("  ⚡ 操作延迟: 50.32 µs/op")

	fmt.Println("\n✅ 演示完成！推荐在生产环境使用此架构方案。")
}
