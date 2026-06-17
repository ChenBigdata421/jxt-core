//go:build ignore

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

func main() {
	fmt.Println("=== EventBus 主题持久化管理示例 ===\n")

	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	// 创建支持主题级持久化配置的EventBus（使用内存模式演示）
	config := &eventbus.EventBusConfig{
		Type: "memory", // 使用内存模式演示，实际生产环境可以使用NATS或Kafka
	}

	bus, err := eventbus.NewEventBus(config)
	if err != nil {
		log.Fatalf("Failed to create EventBus: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()

	// 1. 配置不同主题的持久化策略
	fmt.Println("📋 配置主题持久化策略...")

	// 订单事件 - 需要持久化
	orderOptions := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   7 * 24 * time.Hour, // 保留7天
		MaxSize:         100 * 1024 * 1024,  // 100MB
		MaxMessages:     10000,              // 1万条消息
		Description:     "订单相关事件，需要持久化存储",
	}
	if err := bus.ConfigureTopic(ctx, "orders", orderOptions); err != nil {
		log.Fatalf("Failed to configure orders topic: %v", err)
	}
	fmt.Println("✅ 订单主题配置为持久化")

	// 实时通知 - 不需要持久化
	notificationOptions := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicEphemeral,
		Description:     "实时通知消息，无需持久化",
	}
	if err := bus.ConfigureTopic(ctx, "notifications", notificationOptions); err != nil {
		log.Fatalf("Failed to configure notifications topic: %v", err)
	}
	fmt.Println("✅ 通知主题配置为非持久化")

	// 系统监控 - 自动选择（根据全局配置）
	metricsOptions := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicAuto,
		Description:     "系统监控指标，自动选择持久化策略",
	}
	if err := bus.ConfigureTopic(ctx, "metrics", metricsOptions); err != nil {
		log.Fatalf("Failed to configure metrics topic: %v", err)
	}
	fmt.Println("✅ 监控主题配置为自动选择")

	// 使用简化接口设置主题持久化
	if err := bus.SetTopicPersistence(ctx, "audit", true); err != nil {
		log.Fatalf("Failed to set audit topic persistence: %v", err)
	}
	fmt.Println("✅ 审计主题配置为持久化（简化接口）")

	// 2. 查看配置的主题
	fmt.Println("\n📋 已配置的主题列表:")
	topics := bus.ListConfiguredTopics()
	for _, topic := range topics {
		config, _ := bus.GetTopicConfig(topic)
		fmt.Printf("  - %s: %s (%s)\n", topic, config.PersistenceMode, config.Description)
	}

	// 3. 订阅消息
	fmt.Println("\n📡 设置消息订阅...")

	// 订阅订单事件
	err = bus.Subscribe(ctx, "orders", func(ctx context.Context, message []byte) error {
		fmt.Printf("📦 [订单] 收到持久化消息: %s\n", string(message))
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to orders: %v", err)
	}

	// 订阅通知消息
	err = bus.Subscribe(ctx, "notifications", func(ctx context.Context, message []byte) error {
		fmt.Printf("📢 [通知] 收到非持久化消息: %s\n", string(message))
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to notifications: %v", err)
	}

	// 订阅监控消息
	err = bus.Subscribe(ctx, "metrics", func(ctx context.Context, message []byte) error {
		fmt.Printf("📊 [监控] 收到自动选择消息: %s\n", string(message))
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to metrics: %v", err)
	}

	// 订阅审计消息
	err = bus.Subscribe(ctx, "audit", func(ctx context.Context, message []byte) error {
		fmt.Printf("🔍 [审计] 收到持久化消息: %s\n", string(message))
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to audit: %v", err)
	}

	// 4. 发布消息（自动根据配置选择持久化模式）
	fmt.Println("\n📤 发布测试消息...")

	// 发布订单消息（将使用JetStream持久化）
	orderMsg := `{"order_id": "order-123", "amount": 99.99, "status": "created"}`
	if err := bus.Publish(ctx, "orders", []byte(orderMsg)); err != nil {
		log.Printf("Failed to publish order message: %v", err)
	}

	// 发布通知消息（将使用Core NATS非持久化）
	notificationMsg := `{"type": "info", "title": "系统通知", "content": "欢迎使用EventBus"}`
	if err := bus.Publish(ctx, "notifications", []byte(notificationMsg)); err != nil {
		log.Printf("Failed to publish notification message: %v", err)
	}

	// 发布监控消息（根据全局配置自动选择）
	metricsMsg := `{"cpu_usage": 45.2, "memory_usage": 67.8, "timestamp": "2024-01-01T12:00:00Z"}`
	if err := bus.Publish(ctx, "metrics", []byte(metricsMsg)); err != nil {
		log.Printf("Failed to publish metrics message: %v", err)
	}

	// 发布审计消息（使用简化接口配置的持久化）
	auditMsg := `{"user_id": "user-456", "action": "login", "timestamp": "2024-01-01T12:00:00Z"}`
	if err := bus.Publish(ctx, "audit", []byte(auditMsg)); err != nil {
		log.Printf("Failed to publish audit message: %v", err)
	}

	// 等待消息处理
	time.Sleep(1 * time.Second)

	// 5. 动态修改主题配置
	fmt.Println("\n🔄 动态修改主题配置...")

	// 将通知主题改为持久化
	if err := bus.SetTopicPersistence(ctx, "notifications", true); err != nil {
		log.Printf("Failed to change notifications persistence: %v", err)
	} else {
		fmt.Println("✅ 通知主题已改为持久化")
	}

	// 发布更多通知消息（现在将使用持久化）
	notificationMsg2 := `{"type": "warning", "title": "配置变更", "content": "通知主题已改为持久化模式"}`
	if err := bus.Publish(ctx, "notifications", []byte(notificationMsg2)); err != nil {
		log.Printf("Failed to publish notification message: %v", err)
	}

	// 6. 移除主题配置
	fmt.Println("\n🗑️ 移除主题配置...")
	if err := bus.RemoveTopicConfig("metrics"); err != nil {
		log.Printf("Failed to remove metrics config: %v", err)
	} else {
		fmt.Println("✅ 监控主题配置已移除，将使用默认行为")
	}

	// 等待最后的消息处理
	time.Sleep(1 * time.Second)

	fmt.Println("\n=== 示例完成 ===")
	fmt.Println("✨ 主要特性演示:")
	fmt.Println("  🎯 按主题配置持久化策略")
	fmt.Println("  🔄 动态修改主题配置")
	fmt.Println("  📋 查询主题配置信息")
	fmt.Println("  🚀 自动选择发布模式")
	fmt.Println("  🛠️ 简化接口支持")
}
