//go:build ignore

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

// 验证独立实例的创建和基本功能
func main() {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	fmt.Println("=== 独立EventBus实例验证 ===\n")

	// 1. 验证便捷工厂方法
	fmt.Println("📦 验证便捷工厂方法...")

	// 验证持久化NATS实例配置
	fmt.Println("  - 验证持久化NATS配置...")
	persistentConfig := eventbus.GetDefaultPersistentNATSConfig(
		[]string{"nats://localhost:4222"},
		"test-persistent",
	)
	fmt.Printf("    ✅ 持久化配置: JetStream.Enabled=%v\n", persistentConfig.NATS.JetStream.Enabled)
	fmt.Printf("    ✅ 存储类型: %s\n", persistentConfig.NATS.JetStream.Stream.Storage)

	// 验证非持久化NATS实例配置
	fmt.Println("  - 验证非持久化NATS配置...")
	ephemeralConfig := eventbus.GetDefaultEphemeralNATSConfig(
		[]string{"nats://localhost:4222"},
		"test-ephemeral",
	)
	fmt.Printf("    ✅ 非持久化配置: JetStream.Enabled=%v\n", ephemeralConfig.NATS.JetStream.Enabled)

	// 尝试创建NATS实例（需要NATS服务器运行）
	fmt.Println("\n🔌 测试NATS连接（需要NATS服务器）...")
	fmt.Println("   注意：如果NATS服务器未运行，以下测试将失败")
	fmt.Println("   启动命令：nats-server -js")

	// 测试持久化NATS实例
	fmt.Println("  - 测试持久化NATS实例创建...")
	persistentBus, err := eventbus.NewEventBus(persistentConfig)
	if err != nil {
		fmt.Printf("    ❌ 持久化NATS实例创建失败: %v\n", err)
		fmt.Println("    💡 请确保NATS服务器正在运行: nats-server -js")
	} else {
		fmt.Println("    ✅ 持久化NATS实例创建成功")
		persistentBus.Close()
	}

	// 测试非持久化NATS实例
	fmt.Println("  - 测试非持久化NATS实例创建...")
	ephemeralBus, err := eventbus.NewEventBus(ephemeralConfig)
	if err != nil {
		fmt.Printf("    ❌ 非持久化NATS实例创建失败: %v\n", err)
		fmt.Println("    💡 请确保NATS服务器正在运行: nats-server -js")
	} else {
		fmt.Println("    ✅ 非持久化NATS实例创建成功")
		ephemeralBus.Close()
	}

	// 2. 验证内存实例（作为非持久化的替代方案）
	fmt.Println("\n📦 验证内存实例（非持久化替代方案）...")
	memoryConfig := eventbus.GetDefaultConfig("memory")
	fmt.Printf("    ✅ 内存配置: Type=%s\n", memoryConfig.Type)

	memoryBus, err := eventbus.NewEventBus(memoryConfig)
	if err != nil {
		log.Fatalf("Failed to create memory EventBus: %v", err)
	}
	defer memoryBus.Close()

	// 3. 验证内存实例的基本功能
	fmt.Println("\n🚀 验证内存实例基本功能...")
	ctx := context.Background()

	// 订阅消息
	messageReceived := make(chan bool, 1)
	err = memoryBus.Subscribe(ctx, "test.topic", func(ctx context.Context, message []byte) error {
		var data map[string]interface{}
		json.Unmarshal(message, &data)
		fmt.Printf("    📨 收到消息: %+v\n", data)
		messageReceived <- true
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// 发布消息
	testMessage := map[string]interface{}{
		"type":    "test",
		"message": "Hello from memory EventBus",
		"time":    time.Now().Format("15:04:05"),
	}
	messageBytes, _ := json.Marshal(testMessage)

	err = memoryBus.Publish(ctx, "test.topic", messageBytes)
	if err != nil {
		log.Fatalf("Failed to publish: %v", err)
	}

	// 等待消息处理
	select {
	case <-messageReceived:
		fmt.Println("    ✅ 消息发布和订阅验证成功")
	case <-time.After(1 * time.Second):
		fmt.Println("    ❌ 消息处理超时")
	}

	// 4. 配置对比总结
	fmt.Println("\n=== 配置对比总结 ===")
	fmt.Println("📊 持久化方案对比:")
	fmt.Println("  1. NATS JetStream (持久化)")
	fmt.Printf("     - JetStream.Enabled: %v\n", persistentConfig.NATS.JetStream.Enabled)
	fmt.Printf("     - Storage: %s\n", persistentConfig.NATS.JetStream.Stream.Storage)
	fmt.Printf("     - AckPolicy: %s\n", persistentConfig.NATS.JetStream.Consumer.AckPolicy)
	fmt.Println("     - 特点: 消息持久化存储，支持重放和恢复")

	fmt.Println("\n  2. NATS Core (非持久化)")
	fmt.Printf("     - JetStream.Enabled: %v\n", ephemeralConfig.NATS.JetStream.Enabled)
	fmt.Println("     - 特点: 高性能，无持久化开销")

	fmt.Println("\n  3. Memory (非持久化)")
	fmt.Printf("     - Type: %s\n", memoryConfig.Type)
	fmt.Println("     - 特点: 最高性能，进程内通信")

	// 5. 使用建议
	fmt.Println("\n💡 使用建议:")
	fmt.Println("  📦 业务A (需要持久化): 使用 NewPersistentNATSEventBus()")
	fmt.Println("  ⚡ 业务B (不需要持久化): 选择以下之一:")
	fmt.Println("     - NewEphemeralNATSEventBus() - 如果需要跨进程通信")
	fmt.Println("     - NewEventBus(GetDefaultMemoryConfig()) - 如果只需进程内通信")

	fmt.Println("\n✅ 独立实例验证完成")
}
