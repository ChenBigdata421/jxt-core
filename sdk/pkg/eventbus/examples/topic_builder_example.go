package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// TopicBuilderExample 演示 TopicBuilder 的各种使用方式
func main() {
	fmt.Println("=== Kafka Topic Builder 使用示例 ===\n")

	_ = context.Background() // ctx 用于实际调用 Build 时使用

	// 模拟创建 EventBus（实际使用时需要真实的 Kafka 配置）
	// bus, _ := eventbus.NewKafkaEventBus(config)

	// ========== 示例1：使用预设配置 ==========
	fmt.Println("示例1：使用预设配置")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("orders").
    ForHighThroughput().
    Build(ctx, bus)`)
	fmt.Println("\n配置效果：")
	builder1 := eventbus.NewTopicBuilder("orders").ForHighThroughput()
	printBuilderConfig(builder1)

	// ========== 示例2：预设 + 自定义覆盖 ==========
	fmt.Println("\n示例2：预设 + 自定义覆盖")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("orders").
    ForHighThroughput().
    WithPartitions(15).      // 覆盖预设的10个分区
    WithRetention(14 * 24 * time.Hour).  // 覆盖预设的7天
    Build(ctx, bus)`)
	fmt.Println("\n配置效果：")
	builder2 := eventbus.NewTopicBuilder("orders").
		ForHighThroughput().
		WithPartitions(15).
		WithRetention(14 * 24 * time.Hour)
	printBuilderConfig(builder2)

	// ========== 示例3：完全自定义配置 ==========
	fmt.Println("\n示例3：完全自定义配置")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("custom-topic").
    WithPartitions(20).
    WithReplication(3).
    WithRetention(30 * 24 * time.Hour).
    WithMaxSize(5 * 1024 * 1024 * 1024).  // 5GB
    WithDescription("自定义高性能主题").
    Build(ctx, bus)`)
	fmt.Println("\n配置效果：")
	builder3 := eventbus.NewTopicBuilder("custom-topic").
		WithPartitions(20).
		WithReplication(3).
		WithRetention(30 * 24 * time.Hour).
		WithMaxSize(5 * 1024 * 1024 * 1024).
		WithDescription("自定义高性能主题")
	printBuilderConfig(builder3)

	// ========== 示例4：环境特定配置 ==========
	fmt.Println("\n示例4：环境特定配置")

	fmt.Println("\n4.1 生产环境配置：")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("prod-orders").
    ForProduction().
    WithPartitions(10).
    Build(ctx, bus)`)
	fmt.Println("\n配置效果：")
	builder4a := eventbus.NewTopicBuilder("prod-orders").
		ForProduction().
		WithPartitions(10)
	printBuilderConfig(builder4a)

	fmt.Println("\n4.2 开发环境配置：")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("dev-orders").
    ForDevelopment().
    WithPartitions(3).
    Build(ctx, bus)`)
	fmt.Println("\n配置效果：")
	builder4b := eventbus.NewTopicBuilder("dev-orders").
		ForDevelopment().
		WithPartitions(3)
	printBuilderConfig(builder4b)

	fmt.Println("\n4.3 测试环境配置：")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("test-orders").
    ForTesting().
    WithPartitions(1).
    Build(ctx, bus)`)
	fmt.Println("\n配置效果：")
	builder4c := eventbus.NewTopicBuilder("test-orders").
		ForTesting().
		WithPartitions(1)
	printBuilderConfig(builder4c)

	// ========== 示例5：快捷构建函数 ==========
	fmt.Println("\n示例5：快捷构建函数")
	fmt.Println("代码：")
	fmt.Println(`  // 一行代码构建高吞吐量主题
  err := eventbus.BuildHighThroughputTopic(ctx, bus, "orders")
  
  // 一行代码构建中等吞吐量主题
  err := eventbus.BuildMediumThroughputTopic(ctx, bus, "notifications")
  
  // 一行代码构建低吞吐量主题
  err := eventbus.BuildLowThroughputTopic(ctx, bus, "logs")`)

	// ========== 示例6：链式调用的灵活性 ==========
	fmt.Println("\n示例6：链式调用的灵活性")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("flexible-topic").
    ForMediumThroughput().    // 从中等吞吐量开始
    WithPartitions(8).        // 调整分区数
    Persistent().             // 确保持久化
    WithDescription("灵活配置的主题").
    Build(ctx, bus)`)
	fmt.Println("\n配置效果：")
	builder6 := eventbus.NewTopicBuilder("flexible-topic").
		ForMediumThroughput().
		WithPartitions(8).
		Persistent().
		WithDescription("灵活配置的主题")
	printBuilderConfig(builder6)

	// ========== 示例7：配置验证 ==========
	fmt.Println("\n示例7：配置验证")
	fmt.Println("\n7.1 有效配置：")
	validBuilder := eventbus.NewTopicBuilder("valid-topic").
		WithPartitions(10).
		WithReplication(3)
	if err := validBuilder.Validate(); err != nil {
		fmt.Printf("  ❌ 验证失败: %v\n", err)
	} else {
		fmt.Println("  ✅ 验证通过")
	}

	fmt.Println("\n7.2 无效配置（分区数为0）：")
	invalidBuilder1 := eventbus.NewTopicBuilder("invalid-topic").
		WithPartitions(0)
	if err := invalidBuilder1.Validate(); err != nil {
		fmt.Printf("  ❌ 验证失败: %v\n", err)
	} else {
		fmt.Println("  ✅ 验证通过")
	}

	fmt.Println("\n7.3 无效配置（副本因子超过分区数）：")
	invalidBuilder2 := eventbus.NewTopicBuilder("invalid-topic").
		WithPartitions(3).
		WithReplication(5)
	if err := invalidBuilder2.Validate(); err != nil {
		fmt.Printf("  ❌ 验证失败: %v\n", err)
	} else {
		fmt.Println("  ✅ 验证通过")
	}

	// ========== 示例8：实际使用场景 ==========
	fmt.Println("\n示例8：实际使用场景")
	fmt.Println("\n8.1 订单系统（高吞吐量 + 长保留）：")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("order-events").
    ForHighThroughput().
    WithRetention(30 * 24 * time.Hour).  // 保留30天用于审计
    WithDescription("订单事件流").
    Build(ctx, bus)`)

	fmt.Println("\n8.2 实时通知（中等吞吐量 + 短保留）：")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("notifications").
    ForMediumThroughput().
    WithRetention(24 * time.Hour).  // 只保留1天
    WithDescription("实时通知").
    Build(ctx, bus)`)

	fmt.Println("\n8.3 系统日志（低吞吐量 + 中等保留）：")
	fmt.Println("代码：")
	fmt.Println(`  err := eventbus.NewTopicBuilder("system-logs").
    ForLowThroughput().
    WithRetention(7 * 24 * time.Hour).  // 保留7天
    WithDescription("系统日志").
    Build(ctx, bus)`)

	// ========== 总结 ==========
	fmt.Println("\n=== 总结 ===")
	fmt.Println("\nTopicBuilder 的优势：")
	fmt.Println("✅ 链式调用，代码可读性强")
	fmt.Println("✅ 预设配置 + 自定义覆盖，灵活性高")
	fmt.Println("✅ 类型安全，编译时检查")
	fmt.Println("✅ 内置验证，避免无效配置")
	fmt.Println("✅ 易于扩展，添加新配置项不影响现有代码")

	fmt.Println("\n推荐使用方式：")
	fmt.Println("1. 简单场景：使用预设配置（ForHighThroughput/ForMediumThroughput/ForLowThroughput）")
	fmt.Println("2. 常见场景：预设配置 + 局部覆盖（如调整分区数）")
	fmt.Println("3. 复杂场景：完全自定义配置")
	fmt.Println("4. 快速原型：使用快捷构建函数（BuildHighThroughputTopic等）")

	fmt.Println("\n注意事项：")
	fmt.Println("⚠️  分区数一旦设置，只能增加不能减少")
	fmt.Println("⚠️  副本因子一旦设置，不能修改（需要重建topic）")
	fmt.Println("⚠️  单个topic建议不超过100个分区")
	fmt.Println("⚠️  生产环境建议至少3个副本")
}

// printBuilderConfig 打印 Builder 的配置
func printBuilderConfig(builder *eventbus.TopicBuilder) {
	opts := builder.GetOptions()
	topic := builder.GetTopic()

	fmt.Printf("  主题名称: %s\n", topic)
	fmt.Printf("  分区数: %d\n", opts.Partitions)
	fmt.Printf("  副本因子: %d\n", opts.ReplicationFactor)
	fmt.Printf("  持久化模式: %s\n", opts.PersistenceMode)
	fmt.Printf("  保留时间: %v\n", opts.RetentionTime)
	fmt.Printf("  最大大小: %d bytes (%.2f MB)\n", opts.MaxSize, float64(opts.MaxSize)/(1024*1024))
	fmt.Printf("  最大消息数: %d\n", opts.MaxMessages)
	if opts.Description != "" {
		fmt.Printf("  描述: %s\n", opts.Description)
	}
}
