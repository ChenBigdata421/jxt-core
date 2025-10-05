package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	loggerPkg "github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// ========== 业务模块A：使用 Envelope（事件溯源场景） ==========

// OrderCreatedEvent 订单创建事件（需要事件溯源）
type OrderCreatedEvent struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
}

// OrderService 订单服务（使用 Envelope）
type OrderService struct {
	eventBus eventbus.EventBus
}

func (s *OrderService) CreateOrder(ctx context.Context, orderID, customerID string, amount float64) error {
	// 创建业务事件
	event := OrderCreatedEvent{
		OrderID:    orderID,
		CustomerID: customerID,
		Amount:     amount,
		Currency:   "USD",
	}

	// 序列化事件
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal order event: %w", err)
	}

	// 使用 Envelope 发布（支持事件溯源）
	envelope := eventbus.NewEnvelope(orderID, "OrderCreated", 1, payload)
	envelope.TraceID = "trace-" + orderID
	envelope.CorrelationID = "corr-" + orderID

	return s.eventBus.PublishEnvelope(ctx, "orders", envelope)
}

func (s *OrderService) SubscribeToOrderEvents(ctx context.Context) error {
	// 使用 Envelope 订阅
	handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
		fmt.Printf("📦 [订单服务] 收到Envelope事件:\n")
		fmt.Printf("  AggregateID: %s\n", envelope.AggregateID)
		fmt.Printf("  EventType: %s\n", envelope.EventType)
		fmt.Printf("  EventVersion: %d\n", envelope.EventVersion)
		fmt.Printf("  TraceID: %s\n", envelope.TraceID)

		// 解析业务负载
		var event OrderCreatedEvent
		if err := json.Unmarshal(envelope.Payload, &event); err != nil {
			return fmt.Errorf("failed to unmarshal order event: %w", err)
		}

		fmt.Printf("  订单详情: OrderID=%s, CustomerID=%s, Amount=%.2f %s\n\n",
			event.OrderID, event.CustomerID, event.Amount, event.Currency)
		return nil
	}

	return s.eventBus.SubscribeEnvelope(ctx, "orders", handler)
}

// ========== 业务模块B：不使用 Envelope（通知场景） ==========

// NotificationMessage 通知消息（简单通知，不需要事件溯源）
type NotificationMessage struct {
	UserID  string `json:"user_id"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// NotificationService 通知服务（不使用 Envelope）
type NotificationService struct {
	eventBus eventbus.EventBus
}

func (s *NotificationService) SendNotification(ctx context.Context, userID, title, content string) error {
	// 创建通知消息
	notification := NotificationMessage{
		UserID:  userID,
		Type:    "info",
		Title:   title,
		Content: content,
	}

	// 序列化消息
	message, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// 使用普通发布（不需要事件溯源）
	return s.eventBus.Publish(ctx, "notifications", message)
}

func (s *NotificationService) SendNotificationWithOptions(ctx context.Context, userID, title, content string) error {
	// 创建通知消息
	notification := NotificationMessage{
		UserID:  userID,
		Type:    "urgent",
		Title:   title,
		Content: content,
	}

	// 序列化消息
	message, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// 使用高级发布选项（需要更多控制）
	opts := eventbus.PublishOptions{
		AggregateID: userID, // 用于分区
		Metadata: map[string]string{
			"priority": "high",
			"source":   "notification-service",
		},
		Timeout: 30 * time.Second,
	}

	return s.eventBus.PublishWithOptions(ctx, "urgent-notifications", message, opts)
}

func (s *NotificationService) SubscribeToNotifications(ctx context.Context) error {
	// 使用普通订阅
	handler := func(ctx context.Context, message []byte) error {
		fmt.Printf("📧 [通知服务] 收到普通消息:\n")

		var notification NotificationMessage
		if err := json.Unmarshal(message, &notification); err != nil {
			return fmt.Errorf("failed to unmarshal notification: %w", err)
		}

		fmt.Printf("  通知详情: UserID=%s, Type=%s, Title=%s\n",
			notification.UserID, notification.Type, notification.Title)
		fmt.Printf("  内容: %s\n\n", notification.Content)
		return nil
	}

	return s.eventBus.Subscribe(ctx, "notifications", handler)
}

func (s *NotificationService) SubscribeToUrgentNotifications(ctx context.Context) error {
	// 使用高级订阅选项
	handler := func(ctx context.Context, message []byte) error {
		fmt.Printf("🚨 [通知服务] 收到紧急消息:\n")

		var notification NotificationMessage
		if err := json.Unmarshal(message, &notification); err != nil {
			return fmt.Errorf("failed to unmarshal urgent notification: %w", err)
		}

		fmt.Printf("  紧急通知: UserID=%s, Title=%s\n", notification.UserID, notification.Title)
		fmt.Printf("  内容: %s\n\n", notification.Content)
		return nil
	}

	opts := eventbus.SubscribeOptions{
		ProcessingTimeout: 10 * time.Second,
		RateLimit:         100,
		MaxRetries:        3,
	}

	return s.eventBus.SubscribeWithOptions(ctx, "urgent-notifications", handler, opts)
}

// ========== 主程序：演示混合使用 ==========

func main() {
	// 初始化全局logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)
	loggerPkg.Logger = logger
	loggerPkg.DefaultLogger = logger.Sugar()

	fmt.Println("=== EventBus 混合使用示例 ===")
	fmt.Println("业务模块A（订单）：使用 Envelope")
	fmt.Println("业务模块B（通知）：不使用 Envelope")
	fmt.Println()

	// 创建EventBus
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}

	bus, err := eventbus.NewEventBus(cfg)
	if err != nil {
		log.Fatalf("Failed to create EventBus: %v", err)
	}
	defer bus.Close()

	// 创建业务服务
	orderService := &OrderService{eventBus: bus}
	notificationService := &NotificationService{eventBus: bus}

	// 启动订阅
	ctx := context.Background()

	// 订单服务：使用 Envelope 订阅
	if err := orderService.SubscribeToOrderEvents(ctx); err != nil {
		log.Fatalf("Failed to subscribe to order events: %v", err)
	}

	// 通知服务：使用普通订阅
	if err := notificationService.SubscribeToNotifications(ctx); err != nil {
		log.Fatalf("Failed to subscribe to notifications: %v", err)
	}

	// 通知服务：使用高级订阅
	if err := notificationService.SubscribeToUrgentNotifications(ctx); err != nil {
		log.Fatalf("Failed to subscribe to urgent notifications: %v", err)
	}

	// 等待订阅建立
	time.Sleep(100 * time.Millisecond)

	// 发布事件演示
	fmt.Println("开始发布事件...")
	fmt.Println()

	// 1. 订单服务：使用 Envelope 发布
	if err := orderService.CreateOrder(ctx, "order-123", "customer-456", 99.99); err != nil {
		log.Printf("Failed to create order: %v", err)
	}

	// 2. 通知服务：使用普通发布
	if err := notificationService.SendNotification(ctx, "user-789", "订单确认", "您的订单已创建成功"); err != nil {
		log.Printf("Failed to send notification: %v", err)
	}

	// 3. 通知服务：使用高级发布选项
	if err := notificationService.SendNotificationWithOptions(ctx, "user-789", "紧急通知", "系统维护通知"); err != nil {
		log.Printf("Failed to send urgent notification: %v", err)
	}

	// 等待消息处理
	time.Sleep(500 * time.Millisecond)

	fmt.Println("=== 混合使用示例完成 ===")
	fmt.Println("✅ 同一个 EventBus 接口支持:")
	fmt.Println("   - Envelope 方式（事件溯源）")
	fmt.Println("   - 普通方式（简单消息）")
	fmt.Println("   - 高级选项方式（企业特性）")
}
