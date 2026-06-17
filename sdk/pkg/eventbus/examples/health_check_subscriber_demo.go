//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// 演示健康检查订阅监控功能

func main() {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	fmt.Println("=== 健康检查订阅监控演示 ===\n")

	// 创建EventBus（使用内存实现进行演示）
	bus := eventbus.NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. 启动健康检查发送端（模拟服务A）
	fmt.Println("🚀 启动健康检查发送端...")

	healthConfig := eventbus.GetDefaultHealthCheckConfig()
	healthConfig.Sender.Interval = 3 * time.Second // 每3秒发送一次
	healthConfig.Sender.FailureThreshold = 2
	healthConfig.Subscriber.MonitorInterval = 1 * time.Second // 每秒检查一次

	healthChecker := eventbus.NewHealthChecker(
		healthConfig,
		bus,
		"demo-service-a",
		"memory",
	)

	if err := healthChecker.Start(ctx); err != nil {
		log.Fatalf("Failed to start health checker: %v", err)
	}
	defer healthChecker.Stop()

	fmt.Printf("✅ 健康检查发送端已启动 (间隔: %v)\n\n", healthConfig.Sender.Interval)

	// 2. 启动健康检查订阅监控
	fmt.Println("📡 启动健康检查订阅监控...")

	if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
		log.Fatalf("Failed to start health check subscriber: %v", err)
	}
	defer bus.StopHealthCheckSubscriber()

	fmt.Println("✅ 健康检查订阅监控已启动\n")

	// 3. 注册告警回调
	fmt.Println("🔔 注册告警回调...")

	alertCallback := func(ctx context.Context, alert eventbus.HealthCheckAlert) error {
		fmt.Printf("\n🚨 【告警触发】\n")
		fmt.Printf("   告警类型: %s\n", alert.AlertType)
		fmt.Printf("   严重程度: %s\n", alert.Severity)
		fmt.Printf("   来源服务: %s\n", alert.Source)
		fmt.Printf("   EventBus类型: %s\n", alert.EventBusType)
		fmt.Printf("   最后消息时间: %v\n", alert.LastMessageTime.Format(time.RFC3339))
		fmt.Printf("   距离最后消息: %v\n", alert.TimeSinceLastMsg)
		fmt.Printf("   连续错过次数: %d\n", alert.ConsecutiveMisses)
		fmt.Printf("   告警消息: %s\n", alert.Metadata["message"])
		fmt.Printf("   告警时间: %v\n\n", alert.Timestamp.Format(time.RFC3339))
		return nil
	}

	if err := bus.RegisterHealthCheckAlertCallback(alertCallback); err != nil {
		log.Fatalf("Failed to register alert callback: %v", err)
	}

	fmt.Println("✅ 告警回调已注册\n")

	// 4. 启动状态监控协程
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := bus.GetHealthCheckSubscriberStats()
				fmt.Printf("📊 【监控统计】\n")
				fmt.Printf("   运行时间: %.1f秒\n", stats.UptimeSeconds)
				fmt.Printf("   接收消息总数: %d\n", stats.TotalMessagesReceived)
				fmt.Printf("   连续错过次数: %d\n", stats.ConsecutiveMisses)
				fmt.Printf("   健康状态: %v\n", stats.IsHealthy)
				fmt.Printf("   总告警次数: %d\n", stats.TotalAlerts)
				if !stats.LastMessageTime.IsZero() {
					fmt.Printf("   最后消息时间: %v\n", stats.LastMessageTime.Format("15:04:05"))
				}
				if !stats.LastAlertTime.IsZero() {
					fmt.Printf("   最后告警时间: %v\n", stats.LastAlertTime.Format("15:04:05"))
				}
				fmt.Println()
			}
		}
	}()

	// 5. 演示场景
	fmt.Println("🎭 开始演示场景...\n")

	// 场景1：正常运行
	fmt.Println("📍 场景1：正常健康检查消息传输")
	fmt.Println("   等待健康检查消息...")
	time.Sleep(8 * time.Second)

	// 场景2：停止发送端，观察告警
	fmt.Println("📍 场景2：停止健康检查发送端，观察告警触发")
	fmt.Println("   停止健康检查发送端...")
	healthChecker.Stop()

	fmt.Println("   等待告警触发...")
	time.Sleep(healthConfig.Sender.Interval + 2*time.Second)

	// 场景3：重启发送端，观察恢复
	fmt.Println("📍 场景3：重启健康检查发送端，观察恢复")
	fmt.Println("   重启健康检查发送端...")

	newHealthChecker := eventbus.NewHealthChecker(
		healthConfig,
		bus,
		"demo-service-a-restarted",
		"memory",
	)

	if err := newHealthChecker.Start(ctx); err != nil {
		log.Printf("Failed to restart health checker: %v", err)
	} else {
		defer newHealthChecker.Stop()
		fmt.Println("   健康检查发送端已重启")
	}

	fmt.Println("   等待恢复...")
	time.Sleep(6 * time.Second)

	// 场景4：模拟多服务环境
	fmt.Println("📍 场景4：模拟多服务环境")
	fmt.Println("   启动额外的服务...")

	additionalServices := []string{"service-b", "service-c"}
	var additionalCheckers []*eventbus.HealthChecker

	for _, serviceName := range additionalServices {
		serviceConfig := healthConfig
		serviceConfig.Sender.Interval = 2 * time.Second // 不同的间隔
		serviceConfig.Subscriber.MonitorInterval = 1 * time.Second

		checker := eventbus.NewHealthChecker(
			serviceConfig,
			bus,
			serviceName,
			"memory",
		)

		if err := checker.Start(ctx); err != nil {
			log.Printf("Failed to start health checker for %s: %v", serviceName, err)
		} else {
			additionalCheckers = append(additionalCheckers, checker)
			fmt.Printf("   ✅ %s 健康检查已启动\n", serviceName)
		}
	}

	// 清理额外服务
	defer func() {
		for _, checker := range additionalCheckers {
			checker.Stop()
		}
	}()

	fmt.Println("   等待多服务消息传输...")
	time.Sleep(8 * time.Second)

	// 6. 等待用户中断
	fmt.Println("📍 演示完成，按 Ctrl+C 退出...")

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		fmt.Println("\n👋 收到退出信号，正在清理...")
	case <-time.After(30 * time.Second):
		fmt.Println("\n⏰ 演示时间结束，自动退出...")
	}

	// 最终统计
	finalStats := bus.GetHealthCheckSubscriberStats()
	fmt.Printf("\n📈 【最终统计】\n")
	fmt.Printf("   总运行时间: %.1f秒\n", finalStats.UptimeSeconds)
	fmt.Printf("   总接收消息: %d\n", finalStats.TotalMessagesReceived)
	fmt.Printf("   总告警次数: %d\n", finalStats.TotalAlerts)
	fmt.Printf("   最终健康状态: %v\n", finalStats.IsHealthy)

	fmt.Println("\n🎉 健康检查订阅监控演示结束！")
}

// 创建NATS演示配置
func createNATSDemo() {
	fmt.Println("=== NATS 健康检查订阅监控演示 ===\n")

	// NATS配置
	natsConfig := &config.NATSConfig{
		URLs:     []string{"nats://localhost:4222"},
		ClientID: "health-check-demo",
		JetStream: config.JetStreamConfig{
			Enabled: false, // 使用Core NATS
		},
	}

	// 创建NATS EventBus
	bus, err := eventbus.NewNATSEventBus(natsConfig)
	if err != nil {
		log.Printf("Failed to create NATS EventBus: %v", err)
		log.Println("请确保NATS服务器正在运行: nats-server --port 4222")
		return
	}
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动健康检查
	healthConfig := eventbus.GetDefaultHealthCheckConfig()
	healthConfig.Sender.Interval = 5 * time.Second
	healthConfig.Subscriber.MonitorInterval = 2 * time.Second

	healthChecker := eventbus.NewHealthChecker(
		healthConfig,
		bus,
		"nats-demo-service",
		"nats",
	)

	if err := healthChecker.Start(ctx); err != nil {
		log.Fatalf("Failed to start NATS health checker: %v", err)
	}
	defer healthChecker.Stop()

	// 启动订阅监控
	if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
		log.Fatalf("Failed to start NATS health check subscriber: %v", err)
	}
	defer bus.StopHealthCheckSubscriber()

	fmt.Println("✅ NATS 健康检查演示已启动")
	fmt.Println("📡 监控NATS健康检查消息传输...")

	// 运行演示
	time.Sleep(15 * time.Second)

	stats := bus.GetHealthCheckSubscriberStats()
	fmt.Printf("📊 NATS 健康检查统计: %+v\n", stats)
}
