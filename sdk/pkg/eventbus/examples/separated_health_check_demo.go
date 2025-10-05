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
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	fmt.Println("=== 分离式健康检查演示 ===\n")

	// 创建EventBus（使用内存实现进行演示）
	bus := eventbus.NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ========== 场景1：只启动发布端健康检查 ==========
	fmt.Println("🚀 场景1：微服务A - 只作为发布端")
	fmt.Println("   (例如：用户服务发布用户事件，但不订阅其他服务的健康检查)")

	// 启动健康检查发布器
	if err := bus.StartHealthCheckPublisher(ctx); err != nil {
		log.Fatalf("Failed to start health check publisher: %v", err)
	}
	fmt.Println("✅ 健康检查发布器已启动")

	// 获取发布器状态
	publisherStatus := bus.GetHealthCheckPublisherStatus()
	fmt.Printf("📊 发布器状态: 健康=%v, 运行中=%v, 来源=%s\n\n",
		publisherStatus.IsHealthy, publisherStatus.IsRunning, publisherStatus.Source)

	time.Sleep(2 * time.Second)

	// 停止发布器
	if err := bus.StopHealthCheckPublisher(); err != nil {
		log.Fatalf("Failed to stop health check publisher: %v", err)
	}
	fmt.Println("🛑 健康检查发布器已停止\n")

	// ========== 场景2：只启动订阅端健康检查 ==========
	fmt.Println("📡 场景2：微服务B - 只作为订阅端")
	fmt.Println("   (例如：监控服务只监控其他服务的健康状态，自己不发布健康检查)")

	// 启动健康检查订阅器
	if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
		log.Fatalf("Failed to start health check subscriber: %v", err)
	}
	fmt.Println("✅ 健康检查订阅器已启动")

	// 获取订阅器统计信息
	subscriberStats := bus.GetHealthCheckSubscriberStats()
	fmt.Printf("📊 订阅器统计: 健康=%v, 启动时间=%v\n\n",
		subscriberStats.IsHealthy, subscriberStats.StartTime.Format("15:04:05"))

	time.Sleep(2 * time.Second)

	// 停止订阅器
	if err := bus.StopHealthCheckSubscriber(); err != nil {
		log.Fatalf("Failed to stop health check subscriber: %v", err)
	}
	fmt.Println("🛑 健康检查订阅器已停止\n")

	// ========== 场景3：同时启动发布端和订阅端 ==========
	fmt.Println("🔄 场景3：微服务C - 既是发布端又是订阅端")
	fmt.Println("   (例如：订单服务发布订单事件，同时监控用户服务的健康状态)")

	// 使用新的分离式接口
	if err := bus.StartHealthCheckPublisher(ctx); err != nil {
		log.Fatalf("Failed to start health check publisher: %v", err)
	}
	fmt.Println("✅ 健康检查发布器已启动")

	if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
		log.Fatalf("Failed to start health check subscriber: %v", err)
	}
	fmt.Println("✅ 健康检查订阅器已启动")

	// 或者使用便捷方法一次性启动所有健康检查
	// if err := bus.StartAllHealthCheck(ctx); err != nil {
	//     log.Fatalf("Failed to start all health checks: %v", err)
	// }

	fmt.Println("🔄 发布器和订阅器都在运行中...")
	time.Sleep(3 * time.Second)

	// 停止所有健康检查
	if err := bus.StopAllHealthCheck(); err != nil {
		log.Fatalf("Failed to stop all health checks: %v", err)
	}
	fmt.Println("🛑 所有健康检查已停止\n")

	// ========== 场景4：向后兼容性演示 ==========
	fmt.Println("⚠️  场景4：向后兼容性 - 使用旧接口（已废弃）")

	// 使用旧的统一接口（会显示废弃警告）
	if err := bus.StartHealthCheck(ctx); err != nil {
		log.Fatalf("Failed to start health check (deprecated): %v", err)
	}
	fmt.Println("✅ 旧接口启动成功（实际调用了新的发布器接口）")

	oldStatus := bus.GetHealthStatus()
	fmt.Printf("📊 旧接口状态: 健康=%v, 运行中=%v\n",
		oldStatus.IsHealthy, oldStatus.IsRunning)

	if err := bus.StopHealthCheck(); err != nil {
		log.Fatalf("Failed to stop health check (deprecated): %v", err)
	}
	fmt.Println("🛑 旧接口停止成功\n")

	fmt.Println("🎉 分离式健康检查演示完成！")
	fmt.Println("\n💡 关键优势：")
	fmt.Println("   1. 微服务可以根据实际角色选择启动发布端或订阅端")
	fmt.Println("   2. 一个服务可以在不同业务中扮演不同角色")
	fmt.Println("   3. 更精确的资源使用和监控")
	fmt.Println("   4. 保持向后兼容性")
}
