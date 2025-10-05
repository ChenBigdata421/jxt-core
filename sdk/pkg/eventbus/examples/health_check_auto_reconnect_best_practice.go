package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// 业务应用示例：健康检查 + 自动重连最佳实践
func main() {
	log.Println("🚀 Starting EventBus Health Check & Auto-Reconnect Demo")

	// 1. 初始化 EventBus（支持 Kafka 和 NATS）
	cfg := eventbus.GetDefaultKafkaConfig([]string{"localhost:9092"})
	// cfg := eventbus.GetDefaultNATSConfig([]string{"nats://localhost:4222"}) // 或使用 NATS
	
	if err := eventbus.InitializeFromConfig(cfg); err != nil {
		log.Fatal("Failed to initialize EventBus:", err)
	}

	bus := eventbus.GetGlobal()
	log.Println("✅ EventBus initialized successfully")

	// 2. 注册重连回调（处理业务状态）
	err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
		log.Printf("🔄 EventBus reconnected at %v", time.Now().Format("15:04:05"))

		// EventBus 已自动完成基础设施恢复：
		// ✅ 重新建立连接
		// ✅ 恢复所有订阅
		// ✅ 重置健康状态

		// 业务层只需处理业务相关状态：
		if err := handleBusinessReconnect(); err != nil {
			log.Printf("⚠️ Business reconnect handling failed: %v", err)
			// 不返回错误，避免影响 EventBus 重连成功状态
		}

		log.Println("✅ Business state recovery completed")
		return nil
	})
	if err != nil {
		log.Printf("Failed to register reconnect callback: %v", err)
	} else {
		log.Println("✅ Reconnect callback registered")
	}

	// 3. 创建应用 context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. 启动健康检查（包含自动重连）
	if err := bus.StartHealthCheck(ctx); err != nil {
		log.Printf("Failed to start health check: %v", err)
	} else {
		log.Println("✅ Health check with auto-reconnect started")
	}

	// 5. 设置业务订阅
	topic := "demo.health-check"
	handler := func(ctx context.Context, message []byte) error {
		log.Printf("📨 Received: %s", string(message))
		return nil
	}

	if err := bus.Subscribe(ctx, topic, handler); err != nil {
		log.Printf("Failed to subscribe: %v", err)
	} else {
		log.Printf("✅ Subscribed to topic: %s", topic)
	}

	// 6. 设置优雅关闭
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 7. 启动消息发送
	go publishMessages(ctx, bus, topic)

	// 8. 启动状态监控
	go monitorStatus(ctx, bus)

	// 9. 主循环
	log.Println("📋 Application running. Key features:")
	log.Println("   🔍 Periodic health checks every 30 seconds")
	log.Println("   🔄 Auto-reconnect on connection failure")
	log.Println("   📦 Automatic subscription recovery")
	log.Println("   🎯 Business state handling via callbacks")
	log.Println("")
	log.Println("💡 Try stopping Kafka/NATS server to see auto-reconnect in action!")
	log.Println("📴 Press Ctrl+C to stop")

	// 等待退出信号
	<-sigChan
	log.Println("\n📴 Received shutdown signal, shutting down gracefully...")

	// 10. 优雅关闭序列
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// 停止健康检查（推荐方式：同步等待）
	log.Println("🛑 Stopping health check...")
	if err := bus.StopHealthCheck(); err != nil {
		log.Printf("Error stopping health check: %v", err)
	} else {
		log.Println("✅ Health check stopped")
	}

	// 取消应用 context
	cancel()

	// 关闭 EventBus
	log.Println("🔒 Closing EventBus...")
	if err := eventbus.CloseGlobal(); err != nil {
		log.Printf("Error closing EventBus: %v", err)
	} else {
		log.Println("✅ EventBus closed")
	}

	log.Println("✅ Application stopped gracefully")
}

// 发送消息的 goroutine
func publishMessages(ctx context.Context, bus eventbus.EventBus, topic string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	messageCount := 0
	for {
		select {
		case <-ctx.Done():
			log.Println("📤 Message publisher stopped")
			return
		case <-ticker.C:
			messageCount++
			message := []byte(fmt.Sprintf("Health check demo message #%d at %v",
				messageCount, time.Now().Format("15:04:05")))

			if err := bus.Publish(ctx, topic, message); err != nil {
				log.Printf("❌ Failed to publish message #%d: %v", messageCount, err)
			} else {
				log.Printf("📤 Published message #%d", messageCount)
			}
		}
	}
}

// 监控状态的 goroutine
func monitorStatus(ctx context.Context, bus eventbus.EventBus) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("📊 Status monitor stopped")
			return
		case <-ticker.C:
			// 获取连接状态
			connState := bus.GetConnectionState()

			// 获取健康状态
			healthStatus := bus.GetHealthStatus()

			log.Printf("📊 Status Report:")
			log.Printf("   🔗 Connected: %v", connState.IsConnected)
			log.Printf("   💚 Healthy: %v", healthStatus.IsHealthy)
			log.Printf("   ❌ Consecutive Failures: %d", healthStatus.ConsecutiveFailures)
			log.Printf("   🕐 Last Check: %v", healthStatus.LastCheckTime.Format("15:04:05"))
		}
	}
}

// 业务重连处理函数
func handleBusinessReconnect() error {
	log.Println("🔄 Handling business reconnect...")

	// 1. 重新加载应用缓存
	if err := reloadCache(); err != nil {
		return fmt.Errorf("failed to reload cache: %w", err)
	}

	// 2. 同步业务状态
	if err := syncBusinessState(); err != nil {
		return fmt.Errorf("failed to sync business state: %w", err)
	}

	// 3. 发送监控指标
	recordReconnectMetrics()

	// 4. 通知其他服务（可选）
	notifyDependentServices()

	return nil
}

// 模拟业务函数
func reloadCache() error {
	log.Println("   📦 Reloading application cache...")
	time.Sleep(100 * time.Millisecond) // 模拟处理时间
	return nil
}

func syncBusinessState() error {
	log.Println("   🔄 Syncing business state...")
	time.Sleep(150 * time.Millisecond) // 模拟处理时间
	return nil
}

func recordReconnectMetrics() {
	log.Println("   📊 Recording reconnect metrics...")
	// 实现监控指标记录
}

func notifyDependentServices() {
	log.Println("   📢 Notifying dependent services...")
	// 实现服务通知逻辑
}
