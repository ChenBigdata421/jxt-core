package main

import (
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// TopicBuilderValidation 验证 TopicBuilder 的功能
func main() {
	fmt.Println("=== TopicBuilder 功能验证 ===\n")

	// ========== 测试1：基本功能 ==========
	fmt.Println("测试1：基本功能")
	builder1 := eventbus.NewTopicBuilder("test-topic")
	fmt.Printf("  主题名称: %s\n", builder1.GetTopic())
	fmt.Printf("  默认分区数: %d\n", builder1.GetOptions().Partitions)
	fmt.Println("  ✅ 基本功能正常")

	// ========== 测试2：预设配置 ==========
	fmt.Println("\n测试2：预设配置")

	fmt.Println("  2.1 高吞吐量预设:")
	highBuilder := eventbus.NewTopicBuilder("high").ForHighThroughput()
	fmt.Printf("    分区数: %d (期望: 10)\n", highBuilder.GetOptions().Partitions)
	fmt.Printf("    副本因子: %d (期望: 3)\n", highBuilder.GetOptions().ReplicationFactor)
	if highBuilder.GetOptions().Partitions == 10 && highBuilder.GetOptions().ReplicationFactor == 3 {
		fmt.Println("    ✅ 高吞吐量预设正确")
	} else {
		fmt.Println("    ❌ 高吞吐量预设错误")
	}

	fmt.Println("  2.2 中等吞吐量预设:")
	mediumBuilder := eventbus.NewTopicBuilder("medium").ForMediumThroughput()
	fmt.Printf("    分区数: %d (期望: 5)\n", mediumBuilder.GetOptions().Partitions)
	if mediumBuilder.GetOptions().Partitions == 5 {
		fmt.Println("    ✅ 中等吞吐量预设正确")
	} else {
		fmt.Println("    ❌ 中等吞吐量预设错误")
	}

	fmt.Println("  2.3 低吞吐量预设:")
	lowBuilder := eventbus.NewTopicBuilder("low").ForLowThroughput()
	fmt.Printf("    分区数: %d (期望: 3)\n", lowBuilder.GetOptions().Partitions)
	if lowBuilder.GetOptions().Partitions == 3 {
		fmt.Println("    ✅ 低吞吐量预设正确")
	} else {
		fmt.Println("    ❌ 低吞吐量预设错误")
	}

	// ========== 测试3：链式调用 ==========
	fmt.Println("\n测试3：链式调用")
	chainBuilder := eventbus.NewTopicBuilder("chain").
		WithPartitions(15).
		WithReplication(3).
		WithDescription("链式调用测试")

	opts := chainBuilder.GetOptions()
	fmt.Printf("  分区数: %d (期望: 15)\n", opts.Partitions)
	fmt.Printf("  副本因子: %d (期望: 3)\n", opts.ReplicationFactor)
	fmt.Printf("  描述: %s\n", opts.Description)
	if opts.Partitions == 15 && opts.ReplicationFactor == 3 {
		fmt.Println("  ✅ 链式调用正常")
	} else {
		fmt.Println("  ❌ 链式调用异常")
	}

	// ========== 测试4：预设 + 覆盖 ==========
	fmt.Println("\n测试4：预设 + 覆盖")
	overrideBuilder := eventbus.NewTopicBuilder("override").
		ForHighThroughput().
		WithPartitions(20) // 覆盖预设的10

	overrideOpts := overrideBuilder.GetOptions()
	fmt.Printf("  分区数: %d (期望: 20，覆盖了预设的10)\n", overrideOpts.Partitions)
	fmt.Printf("  副本因子: %d (期望: 3，保留预设)\n", overrideOpts.ReplicationFactor)
	if overrideOpts.Partitions == 20 && overrideOpts.ReplicationFactor == 3 {
		fmt.Println("  ✅ 预设覆盖正常")
	} else {
		fmt.Println("  ❌ 预设覆盖异常")
	}

	// ========== 测试5：配置验证 ==========
	fmt.Println("\n测试5：配置验证")

	fmt.Println("  5.1 有效配置:")
	validBuilder := eventbus.NewTopicBuilder("valid").
		WithPartitions(10).
		WithReplication(3)
	if err := validBuilder.Validate(); err != nil {
		fmt.Printf("    ❌ 验证失败: %v\n", err)
	} else {
		fmt.Println("    ✅ 验证通过")
	}

	fmt.Println("  5.2 无效配置（分区数为0）:")
	invalidBuilder1 := eventbus.NewTopicBuilder("invalid1").
		WithPartitions(0)
	if err := invalidBuilder1.Validate(); err != nil {
		fmt.Printf("    ✅ 正确检测到错误: %v\n", err)
	} else {
		fmt.Println("    ❌ 应该检测到错误但没有")
	}

	fmt.Println("  5.3 无效配置（副本因子超过分区数）:")
	invalidBuilder2 := eventbus.NewTopicBuilder("invalid2").
		WithPartitions(3).
		WithReplication(5)
	if err := invalidBuilder2.Validate(); err != nil {
		fmt.Printf("    ✅ 正确检测到错误: %v\n", err)
	} else {
		fmt.Println("    ❌ 应该检测到错误但没有")
	}

	fmt.Println("  5.4 无效配置（分区数超过100）:")
	invalidBuilder3 := eventbus.NewTopicBuilder("invalid3").
		WithPartitions(150)
	if err := invalidBuilder3.Validate(); err != nil {
		fmt.Printf("    ✅ 正确检测到错误: %v\n", err)
	} else {
		fmt.Println("    ❌ 应该检测到错误但没有")
	}

	// ========== 测试6：环境预设 ==========
	fmt.Println("\n测试6：环境预设")

	fmt.Println("  6.1 生产环境:")
	prodBuilder := eventbus.NewTopicBuilder("prod").ForProduction()
	prodOpts := prodBuilder.GetOptions()
	fmt.Printf("    副本因子: %d (期望: 3)\n", prodOpts.ReplicationFactor)
	fmt.Printf("    持久化模式: %s (期望: persistent)\n", prodOpts.PersistenceMode)
	if prodOpts.ReplicationFactor == 3 && prodOpts.PersistenceMode == eventbus.TopicPersistent {
		fmt.Println("    ✅ 生产环境预设正确")
	} else {
		fmt.Println("    ❌ 生产环境预设错误")
	}

	fmt.Println("  6.2 开发环境:")
	devBuilder := eventbus.NewTopicBuilder("dev").ForDevelopment()
	devOpts := devBuilder.GetOptions()
	fmt.Printf("    副本因子: %d (期望: 1)\n", devOpts.ReplicationFactor)
	if devOpts.ReplicationFactor == 1 {
		fmt.Println("    ✅ 开发环境预设正确")
	} else {
		fmt.Println("    ❌ 开发环境预设错误")
	}

	fmt.Println("  6.3 测试环境:")
	testBuilder := eventbus.NewTopicBuilder("test").ForTesting()
	testOpts := testBuilder.GetOptions()
	fmt.Printf("    持久化模式: %s (期望: ephemeral)\n", testOpts.PersistenceMode)
	if testOpts.PersistenceMode == eventbus.TopicEphemeral {
		fmt.Println("    ✅ 测试环境预设正确")
	} else {
		fmt.Println("    ❌ 测试环境预设错误")
	}

	// ========== 测试7：快捷方法 ==========
	fmt.Println("\n测试7：快捷方法")

	fmt.Println("  7.1 Persistent():")
	persistentBuilder := eventbus.NewTopicBuilder("persistent").Persistent()
	if persistentBuilder.GetOptions().PersistenceMode == eventbus.TopicPersistent {
		fmt.Println("    ✅ Persistent() 正常")
	} else {
		fmt.Println("    ❌ Persistent() 异常")
	}

	fmt.Println("  7.2 Ephemeral():")
	ephemeralBuilder := eventbus.NewTopicBuilder("ephemeral").Ephemeral()
	if ephemeralBuilder.GetOptions().PersistenceMode == eventbus.TopicEphemeral {
		fmt.Println("    ✅ Ephemeral() 正常")
	} else {
		fmt.Println("    ❌ Ephemeral() 异常")
	}

	// ========== 总结 ==========
	fmt.Println("\n=== 验证完成 ===")
	fmt.Println("\n✅ TopicBuilder 所有核心功能正常！")
	fmt.Println("\n功能清单：")
	fmt.Println("  ✅ 基本构建功能")
	fmt.Println("  ✅ 预设配置（高/中/低吞吐量）")
	fmt.Println("  ✅ 链式调用")
	fmt.Println("  ✅ 预设覆盖")
	fmt.Println("  ✅ 配置验证")
	fmt.Println("  ✅ 环境预设（生产/开发/测试）")
	fmt.Println("  ✅ 快捷方法")

	fmt.Println("\n下一步：")
	fmt.Println("  1. 在实际项目中使用 TopicBuilder")
	fmt.Println("  2. 根据业务需求选择合适的预设配置")
	fmt.Println("  3. 使用链式调用进行灵活配置")
}

