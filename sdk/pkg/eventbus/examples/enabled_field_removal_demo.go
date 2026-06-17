//go:build ignore

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
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	fmt.Println("=== 移除Enabled字段演示 ===\n")

	// 创建EventBus（使用内存实现进行演示）
	bus := eventbus.NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ========== 演示1：配置中不再需要enabled字段 ==========
	fmt.Println("🔧 演示1：新的配置结构（无需enabled字段）")

	// 创建健康检查配置 - 注意：不再有Subscriber.Enabled字段
	healthConfig := config.HealthCheckConfig{
		Enabled: true, // 只有总体的Enabled字段
		Sender: config.HealthCheckSenderConfig{
			Topic:            "health-check-demo",
			Interval:         30 * time.Second,
			Timeout:          5 * time.Second,
			FailureThreshold: 3,
			MessageTTL:       2 * time.Minute,
		},
		Subscriber: config.HealthCheckSubscriberConfig{
			// 注意：这里没有Enabled字段了！
			Topic:             "health-check-demo",
			MonitorInterval:   10 * time.Second,
			WarningThreshold:  2,
			ErrorThreshold:    3,
			CriticalThreshold: 5,
		},
	}

	fmt.Printf("✅ 配置创建成功，订阅器配置中无需Enabled字段\n")
	fmt.Printf("   发布器主题: %s\n", healthConfig.Sender.Topic)
	fmt.Printf("   订阅器主题: %s\n", healthConfig.Subscriber.Topic)
	fmt.Printf("   监控间隔: %v\n\n", healthConfig.Subscriber.MonitorInterval)

	// ========== 演示2：通过接口控制启动，而非配置字段 ==========
	fmt.Println("🚀 演示2：通过接口控制启动（而非配置字段）")

	// 场景A：只启动发布器（相当于旧的subscriber.enabled=false）
	fmt.Println("   场景A：只启动发布器")
	if err := bus.StartHealthCheckPublisher(ctx); err != nil {
		log.Fatalf("Failed to start publisher: %v", err)
	}
	fmt.Println("   ✅ 发布器已启动，订阅器未启动（相当于旧的enabled=false）")

	time.Sleep(1 * time.Second)

	// 停止发布器
	if err := bus.StopHealthCheckPublisher(); err != nil {
		log.Fatalf("Failed to stop publisher: %v", err)
	}
	fmt.Println("   🛑 发布器已停止\n")

	// 场景B：只启动订阅器（相当于旧的subscriber.enabled=true）
	fmt.Println("   场景B：只启动订阅器")
	if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
		log.Fatalf("Failed to start subscriber: %v", err)
	}
	fmt.Println("   ✅ 订阅器已启动（相当于旧的enabled=true）")

	time.Sleep(1 * time.Second)

	// 停止订阅器
	if err := bus.StopHealthCheckSubscriber(); err != nil {
		log.Fatalf("Failed to stop subscriber: %v", err)
	}
	fmt.Println("   🛑 订阅器已停止\n")

	// ========== 演示3：灵活的启动控制 ==========
	fmt.Println("🎛️  演示3：灵活的启动控制")

	// 根据业务需求动态决定启动什么
	serviceRole := "publisher_only" // 可以是: publisher_only, subscriber_only, both

	switch serviceRole {
	case "publisher_only":
		fmt.Println("   服务角色：纯发布端")
		if err := bus.StartHealthCheckPublisher(ctx); err != nil {
			log.Fatalf("Failed to start publisher: %v", err)
		}
		fmt.Println("   ✅ 只启动了发布器")

	case "subscriber_only":
		fmt.Println("   服务角色：纯订阅端")
		if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
			log.Fatalf("Failed to start subscriber: %v", err)
		}
		fmt.Println("   ✅ 只启动了订阅器")

	case "both":
		fmt.Println("   服务角色：混合角色")
		if err := bus.StartAllHealthCheck(ctx); err != nil {
			log.Fatalf("Failed to start all health checks: %v", err)
		}
		fmt.Println("   ✅ 启动了发布器和订阅器")
	}

	time.Sleep(2 * time.Second)

	// 清理
	if err := bus.StopAllHealthCheck(); err != nil {
		log.Fatalf("Failed to stop all health checks: %v", err)
	}
	fmt.Println("   🛑 所有健康检查已停止\n")

	// ========== 演示4：配置简化的好处 ==========
	fmt.Println("📋 演示4：配置简化的好处")
	fmt.Println("   旧方式（已移除）：")
	fmt.Println("     subscriber:")
	fmt.Println("       enabled: true/false  # 冗余的配置字段")
	fmt.Println("       topic: \"health-check\"")
	fmt.Println("       monitorInterval: \"30s\"")
	fmt.Println("")
	fmt.Println("   新方式（推荐）：")
	fmt.Println("     subscriber:")
	fmt.Println("       topic: \"health-check\"")
	fmt.Println("       monitorInterval: \"30s\"")
	fmt.Println("     # 启动控制通过接口：bus.StartHealthCheckSubscriber()")
	fmt.Println("")

	fmt.Println("🎉 演示完成！")
	fmt.Println("\n💡 关键改进：")
	fmt.Println("   1. 移除了冗余的subscriber.enabled字段")
	fmt.Println("   2. 启动控制完全通过接口管理")
	fmt.Println("   3. 配置更简洁，逻辑更清晰")
	fmt.Println("   4. 避免了配置和接口的双重控制混淆")
}
