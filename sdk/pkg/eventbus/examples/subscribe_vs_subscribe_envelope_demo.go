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

// 演示Subscribe vs SubscribeEnvelope的核心区别

func main() {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	fmt.Println("=== Subscribe vs SubscribeEnvelope 核心区别演示 ===\n")

	// 创建EventBus配置
	cfg := &eventbus.EventBusConfig{
		Type: "nats",
		NATS: eventbus.NATSConfig{
			URLs: []string{"nats://localhost:4222"},
			JetStream: eventbus.JetStreamConfig{
				Enabled: true,
				Stream: eventbus.StreamConfig{
					Name:      "demo-stream",
					Subjects:  []string{"demo.*"},
					Retention: "limits",
					Storage:   "memory",
					Replicas:  1,
					MaxAge:    time.Hour,
					MaxBytes:  1024 * 1024,
					Discard:   "old",
				},
				Consumer: eventbus.NATSConsumerConfig{
					DurableName:   "demo-consumer",
					AckPolicy:     "explicit",
					ReplayPolicy:  "instant",
					MaxAckPending: 100,
				},
			},
		},
	}

	// 创建EventBus
	bus, err := eventbus.NewEventBus(cfg)
	if err != nil {
		log.Fatalf("Failed to create EventBus: %v", err)
	}
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 演示1：Subscribe - 原始消息，不使用Keyed-Worker池
	fmt.Println("🔍 演示1：Subscribe - 原始消息处理")
	fmt.Println("特点：直接并发处理，无顺序保证，极致性能")

	// 订阅原始消息
	err = bus.Subscribe(ctx, "demo.notifications", func(ctx context.Context, data []byte) error {
		var notification NotificationMessage
		if err := json.Unmarshal(data, &notification); err != nil {
			return err
		}

		fmt.Printf("📢 [Subscribe] 收到通知: %s - %s\n", notification.Type, notification.Message)
		fmt.Printf("   处理模式: 直接并发处理（不使用Keyed-Worker池）\n")
		fmt.Printf("   聚合ID提取: ❌ 无法从原始JSON提取聚合ID\n\n")
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// 演示2：SubscribeEnvelope - Envelope消息，自动使用Keyed-Worker池
	fmt.Println("🔍 演示2：SubscribeEnvelope - Envelope消息处理")
	fmt.Println("特点：按聚合ID顺序处理，事件溯源支持")

	// 订阅Envelope消息
	err = bus.SubscribeEnvelope(ctx, "demo.orders", func(ctx context.Context, envelope *eventbus.Envelope) error {
		var order OrderEvent
		if err := json.Unmarshal(envelope.Payload, &order); err != nil {
			return err
		}

		fmt.Printf("🏛️ [SubscribeEnvelope] 收到领域事件:\n")
		fmt.Printf("   聚合ID: %s\n", envelope.AggregateID)
		fmt.Printf("   事件类型: %s\n", envelope.EventType)
		fmt.Printf("   事件版本: %d\n", envelope.EventVersion)
		fmt.Printf("   处理模式: Keyed-Worker池（聚合ID: %s 路由到固定Worker）\n", envelope.AggregateID)
		fmt.Printf("   聚合ID提取: ✅ 从Envelope.AggregateID成功提取\n")
		fmt.Printf("   订单详情: %+v\n\n", order)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe envelope: %v", err)
	}

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	fmt.Println("📨 开始发布消息，观察处理差异...\n")

	// 发布原始消息（Subscribe处理）
	fmt.Println("--- 发布原始通知消息 ---")
	notifications := []NotificationMessage{
		{Type: "info", Message: "系统维护通知", UserID: "user-123"},
		{Type: "warning", Message: "磁盘空间不足", UserID: "user-456"},
		{Type: "error", Message: "服务异常", UserID: "user-789"},
	}

	for _, notification := range notifications {
		data, _ := json.Marshal(notification)
		if err := bus.Publish(ctx, "demo.notifications", data); err != nil {
			log.Printf("Failed to publish notification: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	time.Sleep(1 * time.Second)

	// 发布Envelope消息（SubscribeEnvelope处理）
	fmt.Println("--- 发布Envelope领域事件 ---")
	orders := []OrderEvent{
		{OrderID: "order-001", CustomerID: "customer-123", Status: "CREATED", Amount: 99.99},
		{OrderID: "order-001", CustomerID: "customer-123", Status: "PAID", Amount: 99.99},
		{OrderID: "order-002", CustomerID: "customer-456", Status: "CREATED", Amount: 199.99},
		{OrderID: "order-001", CustomerID: "customer-123", Status: "SHIPPED", Amount: 99.99},
		{OrderID: "order-002", CustomerID: "customer-456", Status: "PAID", Amount: 199.99},
	}

	for i, order := range orders {
		payload, _ := json.Marshal(order)
		envelope := eventbus.NewEnvelope(order.OrderID, "OrderStatusChanged", int64(i+1), payload)
		envelope.TraceID = fmt.Sprintf("trace-%s-%d", order.OrderID, i+1)

		if err := bus.PublishEnvelope(ctx, "demo.orders", envelope); err != nil {
			log.Printf("Failed to publish order event: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// 等待消息处理完成
	time.Sleep(3 * time.Second)

	fmt.Println("=== 总结：Subscribe vs SubscribeEnvelope ===")
	fmt.Println("✅ Subscribe:")
	fmt.Println("  - 消息格式：原始字节数据（JSON、文本等）")
	fmt.Println("  - 聚合ID：❌ 通常无法提取")
	fmt.Println("  - 处理模式：直接并发处理")
	fmt.Println("  - Keyed-Worker池：❌ 不使用")
	fmt.Println("  - 性能：极致性能，微秒级延迟")
	fmt.Println("  - 适用场景：通知、缓存失效、监控指标")
	fmt.Println("")
	fmt.Println("✅ SubscribeEnvelope:")
	fmt.Println("  - 消息格式：Envelope包装格式")
	fmt.Println("  - 聚合ID：✅ 从Envelope.AggregateID提取")
	fmt.Println("  - 处理模式：按聚合ID顺序处理")
	fmt.Println("  - Keyed-Worker池：✅ 自动使用")
	fmt.Println("  - 性能：顺序保证，毫秒级延迟")
	fmt.Println("  - 适用场景：领域事件、事件溯源、聚合管理")
	fmt.Println("")
	fmt.Println("🎯 关键区别：聚合ID提取能力决定了是否使用Keyed-Worker池！")
}

// NotificationMessage 通知消息结构
type NotificationMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	UserID  string `json:"user_id"`
}

// OrderEvent 订单事件结构
type OrderEvent struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Status     string  `json:"status"`
	Amount     float64 `json:"amount"`
}
