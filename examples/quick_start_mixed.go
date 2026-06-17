//go:build ignore

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

// 订单事件（使用 Envelope）
type OrderEvent struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

// 通知消息（使用普通消息）
type NotificationMsg struct {
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}

func main() {
	// 初始化日志
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)
	loggerPkg.Logger = logger
	loggerPkg.DefaultLogger = logger.Sugar()

	fmt.Println("=== EventBus 混合使用快速开始 ===")

	// 1. 创建 EventBus
	cfg := &eventbus.EventBusConfig{Type: "memory"}
	bus, err := eventbus.NewEventBus(cfg)
	if err != nil {
		log.Fatal("Failed to create EventBus:", err)
	}
	defer bus.Close()

	ctx := context.Background()

	// 2. 订阅 Envelope 消息（事件溯源场景）
	err = bus.SubscribeEnvelope(ctx, "orders", func(ctx context.Context, envelope *eventbus.Envelope) error {
		fmt.Printf("📦 收到订单事件: AggregateID=%s, EventType=%s, Version=%d\n",
			envelope.AggregateID, envelope.EventType, envelope.EventVersion)

		var event OrderEvent
		json.Unmarshal(envelope.Payload, &event)
		fmt.Printf("   订单详情: %+v\n", event)
		return nil
	})
	if err != nil {
		log.Fatal("Failed to subscribe to orders:", err)
	}

	// 3. 订阅普通消息（简单消息传递）
	err = bus.Subscribe(ctx, "notifications", func(ctx context.Context, message []byte) error {
		fmt.Printf("📧 收到通知消息: %s\n", string(message))

		var notification NotificationMsg
		json.Unmarshal(message, &notification)
		fmt.Printf("   通知详情: %+v\n", notification)
		return nil
	})
	if err != nil {
		log.Fatal("Failed to subscribe to notifications:", err)
	}

	// 等待订阅建立
	time.Sleep(100 * time.Millisecond)

	// 4. 发布 Envelope 消息（事件溯源）
	fmt.Println("\n--- 发布事件 ---")

	orderEvent := OrderEvent{OrderID: "order-123", Amount: 99.99}
	payload, _ := json.Marshal(orderEvent)

	envelope := eventbus.NewEnvelope("order-123", "OrderCreated", 1, payload)
	envelope.TraceID = "trace-123"

	if err := bus.PublishEnvelope(ctx, "orders", envelope); err != nil {
		log.Printf("Failed to publish order event: %v", err)
	} else {
		fmt.Println("✅ 订单事件已发布（Envelope）")
	}

	// 5. 发布普通消息（简单消息传递）
	notification := NotificationMsg{UserID: "user-456", Message: "订单创建成功"}
	message, _ := json.Marshal(notification)

	if err := bus.Publish(ctx, "notifications", message); err != nil {
		log.Printf("Failed to publish notification: %v", err)
	} else {
		fmt.Println("✅ 通知消息已发布（普通消息）")
	}

	// 6. 使用高级发布选项（企业特性）
	urgentNotification := NotificationMsg{UserID: "user-456", Message: "紧急系统维护通知"}
	urgentMessage, _ := json.Marshal(urgentNotification)

	opts := eventbus.PublishOptions{
		AggregateID: "user-456",
		Metadata: map[string]string{
			"priority": "high",
			"source":   "system",
		},
		Timeout: 30 * time.Second,
	}

	if err := bus.PublishWithOptions(ctx, "notifications", urgentMessage, opts); err != nil {
		log.Printf("Failed to publish urgent notification: %v", err)
	} else {
		fmt.Println("✅ 紧急通知已发布（高级选项）")
	}

	// 等待消息处理
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\n=== 总结 ===")
	fmt.Println("✅ 同一个 EventBus 接口成功支持:")
	fmt.Println("   📦 Envelope 方式 - 适用于事件溯源和聚合管理")
	fmt.Println("   📧 普通消息方式 - 适用于简单消息传递")
	fmt.Println("   ⚙️  高级选项方式 - 适用于需要企业特性的场景")
	fmt.Println("\n💡 业务模块可以根据需求灵活选择最适合的方式！")
}
