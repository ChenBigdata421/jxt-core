package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go.uber.org/zap"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// ========== 独立实例方案示例 ==========

// 业务A：订单服务（使用持久化实例）
type OrderService struct {
	eventBus eventbus.EventBus // 持久化实例
}

// 业务B：通知服务（使用非持久化实例）
type NotificationService struct {
	eventBus eventbus.EventBus // 非持久化实例
}

type OrderCreatedEvent struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
}

type NotificationMessage struct {
	UserID  string `json:"user_id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

func main() {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	fmt.Println("=== 独立EventBus实例方案演示 ===\n")

	// 1. 创建持久化EventBus实例（业务A使用）
	persistentConfig := &eventbus.EventBusConfig{
		Type: "nats",
		NATS: eventbus.NATSConfig{
			URLs:     []string{"nats://localhost:4222"},
			ClientID: "persistent-bus",
			JetStream: eventbus.JetStreamConfig{
				Enabled: true, // 启用持久化
				Stream: eventbus.StreamConfig{
					Name:     "PERSISTENT_STREAM",
					Subjects: []string{"orders", "payments", "audit"},
					Storage:  "file",
				},
				Consumer: eventbus.NATSConsumerConfig{
					DurableName: "persistent-consumer",
					AckPolicy:   "explicit",
				},
			},
		},
	}

	persistentBus, err := eventbus.NewEventBus(persistentConfig)
	if err != nil {
		log.Fatalf("Failed to create persistent EventBus: %v", err)
	}
	defer persistentBus.Close()

	// 2. 创建非持久化EventBus实例（业务B使用）
	ephemeralConfig := &eventbus.EventBusConfig{
		Type: "nats",
		NATS: eventbus.NATSConfig{
			URLs:     []string{"nats://localhost:4222"},
			ClientID: "ephemeral-bus",
			JetStream: eventbus.JetStreamConfig{
				Enabled: false, // 禁用持久化，使用Core NATS
			},
		},
	}

	ephemeralBus, err := eventbus.NewEventBus(ephemeralConfig)
	if err != nil {
		log.Fatalf("Failed to create ephemeral EventBus: %v", err)
	}
	defer ephemeralBus.Close()

	// 3. 创建业务服务（注入不同的EventBus实例）
	orderService := &OrderService{eventBus: persistentBus}              // 使用持久化实例
	notificationService := &NotificationService{eventBus: ephemeralBus} // 使用非持久化实例

	ctx := context.Background()

	// 4. 启动订阅
	fmt.Println("🚀 启动业务订阅...")

	if err := orderService.SubscribeToOrderEvents(ctx); err != nil {
		log.Fatalf("Failed to subscribe to order events: %v", err)
	}

	if err := notificationService.SubscribeToNotifications(ctx); err != nil {
		log.Fatalf("Failed to subscribe to notifications: %v", err)
	}

	time.Sleep(100 * time.Millisecond) // 等待订阅建立

	// 5. 演示独立实例使用
	fmt.Println("📨 开始发布消息...\n")

	// 业务A：使用持久化实例
	fmt.Println("--- 业务A：订单事件（持久化实例） ---")
	orderService.CreateOrder(ctx, "order-12345", "customer-67890", 299.99)

	time.Sleep(200 * time.Millisecond)

	// 业务B：使用非持久化实例
	fmt.Println("--- 业务B：通知消息（非持久化实例） ---")
	notificationService.SendNotification(ctx, "user-123", "订单确认", "您的订单已创建成功")

	time.Sleep(500 * time.Millisecond)

	// 6. 总结
	fmt.Println("\n=== 独立实例方案特点 ===")
	fmt.Println("✅ 优势:")
	fmt.Println("  🚀 性能最优：零路由开销")
	fmt.Println("  🔧 实现简单：无需路由逻辑")
	fmt.Println("  🎯 职责清晰：实例与业务一一对应")
	fmt.Println("❌ 劣势:")
	fmt.Println("  💾 资源消耗：双倍连接和内存")
	fmt.Println("  ⚙️ 配置复杂：需要管理多个配置")
	fmt.Println("  📊 监控复杂：需要监控多个实例")
}

// ========== 业务A：订单服务方法 ==========

func (s *OrderService) CreateOrder(ctx context.Context, orderID, customerID string, amount float64) error {
	event := OrderCreatedEvent{
		OrderID:    orderID,
		CustomerID: customerID,
		Amount:     amount,
	}

	message, _ := json.Marshal(event)

	// 直接发布到持久化实例，无需路由判断
	return s.eventBus.Publish(ctx, "orders", message)
}

func (s *OrderService) SubscribeToOrderEvents(ctx context.Context) error {
	handler := func(ctx context.Context, message []byte) error {
		var event OrderCreatedEvent
		json.Unmarshal(message, &event)

		fmt.Printf("💾 [订单服务-持久化实例] 收到订单事件: %+v\n", event)
		fmt.Printf("   ✅ 使用专用持久化实例，性能最优\n\n")

		return nil
	}

	// 订阅持久化实例
	return s.eventBus.Subscribe(ctx, "orders", handler)
}

// ========== 业务B：通知服务方法 ==========

func (s *NotificationService) SendNotification(ctx context.Context, userID, title, content string) error {
	notification := NotificationMessage{
		UserID:  userID,
		Title:   title,
		Content: content,
	}

	message, _ := json.Marshal(notification)

	// 直接发布到非持久化实例，无需路由判断
	return s.eventBus.Publish(ctx, "notifications", message)
}

func (s *NotificationService) SubscribeToNotifications(ctx context.Context) error {
	handler := func(ctx context.Context, message []byte) error {
		var notification NotificationMessage
		json.Unmarshal(message, &notification)

		fmt.Printf("🔔 [通知服务-非持久化实例] 收到通知: %+v\n", notification)
		fmt.Printf("   ⚡ 使用专用非持久化实例，性能最优\n\n")

		return nil
	}

	// 订阅非持久化实例
	return s.eventBus.Subscribe(ctx, "notifications", handler)
}
