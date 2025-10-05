package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	// 初始化日志
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	fmt.Println("=== EventBus 主题配置策略示例 ===\n")

	// 示例1：开发环境 - 使用创建或更新策略
	fmt.Println("📝 示例1: 开发环境 - 自动创建和更新配置")
	developmentExample()

	fmt.Println("\n" + "=".repeat(60) + "\n")

	// 示例2：生产环境 - 使用只创建策略
	fmt.Println("🏭 示例2: 生产环境 - 只创建不更新")
	productionExample()

	fmt.Println("\n" + "=".repeat(60) + "\n")

	// 示例3：严格模式 - 使用验证策略
	fmt.Println("🔒 示例3: 严格模式 - 只验证不修改")
	strictExample()

	fmt.Println("\n" + "=".repeat(60) + "\n")

	// 示例4：性能优先 - 跳过检查
	fmt.Println("⚡ 示例4: 性能优先 - 跳过配置检查")
	performanceExample()
}

// 开发环境示例：自动创建和更新配置
func developmentExample() {
	ctx := context.Background()

	// 创建NATS EventBus
	cfg := &config.EventBusConfig{
		Type:        "nats",
		ServiceName: "dev-service",
		NATS: config.NATSConfig{
			URL: "nats://localhost:4222",
			JetStream: config.JetStreamConfig{
				Enabled: true,
			},
		},
	}

	bus, err := eventbus.NewEventBus(eventbus.ConvertConfig(cfg))
	if err != nil {
		log.Fatalf("Failed to create eventbus: %v", err)
	}
	defer bus.Close()

	// 设置为创建或更新策略（开发环境推荐）
	bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
	fmt.Println("✅ 配置策略: StrategyCreateOrUpdate")

	// 第一次配置主题
	fmt.Println("\n📌 第一次配置主题...")
	err = bus.ConfigureTopic(ctx, "dev.orders", eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   1 * time.Hour,
		MaxMessages:     1000,
	})
	if err != nil {
		log.Printf("配置失败: %v", err)
	} else {
		fmt.Println("✅ 主题配置成功（创建）")
	}

	// 等待一下
	time.Sleep(1 * time.Second)

	// 第二次配置主题（修改配置）
	fmt.Println("\n📌 第二次配置主题（修改保留时间）...")
	err = bus.ConfigureTopic(ctx, "dev.orders", eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   2 * time.Hour, // 修改保留时间
		MaxMessages:     1000,
	})
	if err != nil {
		log.Printf("配置失败: %v", err)
	} else {
		fmt.Println("✅ 主题配置成功（更新）")
	}

	// 验证配置
	config, err := bus.GetTopicConfig("dev.orders")
	if err == nil {
		fmt.Printf("\n📊 当前配置:\n")
		fmt.Printf("   - 持久化模式: %s\n", config.PersistenceMode)
		fmt.Printf("   - 保留时间: %v\n", config.RetentionTime)
		fmt.Printf("   - 最大消息数: %d\n", config.MaxMessages)
	}
}

// 生产环境示例：只创建不更新
func productionExample() {
	ctx := context.Background()

	// 创建NATS EventBus
	cfg := &config.EventBusConfig{
		Type:        "nats",
		ServiceName: "prod-service",
		NATS: config.NATSConfig{
			URL: "nats://localhost:4222",
			JetStream: config.JetStreamConfig{
				Enabled: true,
			},
		},
	}

	bus, err := eventbus.NewEventBus(eventbus.ConvertConfig(cfg))
	if err != nil {
		log.Fatalf("Failed to create eventbus: %v", err)
	}
	defer bus.Close()

	// 设置为只创建策略（生产环境推荐）
	bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)
	fmt.Println("✅ 配置策略: StrategyCreateOnly")

	// 第一次配置主题
	fmt.Println("\n📌 第一次配置主题...")
	err = bus.ConfigureTopic(ctx, "prod.orders", eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   24 * time.Hour,
		MaxMessages:     10000,
	})
	if err != nil {
		log.Printf("配置失败: %v", err)
	} else {
		fmt.Println("✅ 主题配置成功（创建）")
	}

	// 等待一下
	time.Sleep(1 * time.Second)

	// 第二次配置主题（尝试修改配置）
	fmt.Println("\n📌 第二次配置主题（尝试修改保留时间）...")
	err = bus.ConfigureTopic(ctx, "prod.orders", eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   48 * time.Hour, // 尝试修改保留时间
		MaxMessages:     10000,
	})
	if err != nil {
		log.Printf("配置失败: %v", err)
	} else {
		fmt.Println("⚠️  主题配置成功（但不会更新，使用现有配置）")
	}

	// 验证配置
	config, err := bus.GetTopicConfig("prod.orders")
	if err == nil {
		fmt.Printf("\n📊 当前配置:\n")
		fmt.Printf("   - 持久化模式: %s\n", config.PersistenceMode)
		fmt.Printf("   - 保留时间: %v\n", config.RetentionTime)
		fmt.Printf("   - 最大消息数: %d\n", config.MaxMessages)
	}
}

// 严格模式示例：只验证不修改
func strictExample() {
	ctx := context.Background()

	// 创建NATS EventBus
	cfg := &config.EventBusConfig{
		Type:        "nats",
		ServiceName: "strict-service",
		NATS: config.NATSConfig{
			URL: "nats://localhost:4222",
			JetStream: config.JetStreamConfig{
				Enabled: true,
			},
		},
	}

	bus, err := eventbus.NewEventBus(eventbus.ConvertConfig(cfg))
	if err != nil {
		log.Fatalf("Failed to create eventbus: %v", err)
	}
	defer bus.Close()

	// 先用创建策略创建主题
	bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
	fmt.Println("✅ 临时使用 StrategyCreateOrUpdate 创建主题")

	err = bus.ConfigureTopic(ctx, "strict.orders", eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   24 * time.Hour,
		MaxMessages:     10000,
	})
	if err != nil {
		log.Printf("配置失败: %v", err)
		return
	}
	fmt.Println("✅ 主题创建成功")

	time.Sleep(1 * time.Second)

	// 切换到验证模式
	bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
	fmt.Println("\n✅ 切换到 StrategyValidateOnly")

	// 尝试配置（实际上只会验证）
	fmt.Println("\n📌 验证主题配置...")
	err = bus.ConfigureTopic(ctx, "strict.orders", eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   24 * time.Hour,
		MaxMessages:     10000,
	})
	if err != nil {
		log.Printf("❌ 验证失败: %v", err)
	} else {
		fmt.Println("✅ 配置验证通过")
	}

	// 尝试配置不一致的值
	fmt.Println("\n📌 验证不一致的配置...")
	err = bus.ConfigureTopic(ctx, "strict.orders", eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		RetentionTime:   48 * time.Hour, // 不一致
		MaxMessages:     10000,
	})
	if err != nil {
		fmt.Printf("⚠️  验证失败（预期）: %v\n", err)
	} else {
		fmt.Println("⚠️  配置验证通过（但检测到不一致，查看日志）")
	}
}

// 性能优先示例：跳过配置检查
func performanceExample() {
	ctx := context.Background()

	// 创建Memory EventBus（最快）
	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "perf-service",
	}

	bus, err := eventbus.NewEventBus(eventbus.ConvertConfig(cfg))
	if err != nil {
		log.Fatalf("Failed to create eventbus: %v", err)
	}
	defer bus.Close()

	// 设置为跳过策略（性能优先）
	bus.SetTopicConfigStrategy(eventbus.StrategySkip)
	fmt.Println("✅ 配置策略: StrategySkip")

	// 配置主题（会跳过所有检查）
	fmt.Println("\n📌 配置主题（跳过检查）...")
	start := time.Now()

	for i := 0; i < 100; i++ {
		topic := fmt.Sprintf("perf.topic.%d", i)
		err = bus.ConfigureTopic(ctx, topic, eventbus.TopicOptions{
			PersistenceMode: eventbus.TopicEphemeral,
		})
		if err != nil {
			log.Printf("配置失败: %v", err)
		}
	}

	duration := time.Since(start)
	fmt.Printf("✅ 配置100个主题耗时: %v\n", duration)
	fmt.Printf("   平均每个主题: %v\n", duration/100)
}

// repeat 辅助函数
func repeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

