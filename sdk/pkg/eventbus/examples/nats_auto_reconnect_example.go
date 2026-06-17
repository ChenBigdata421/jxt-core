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

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
	// 1. 初始化 NATS EventBus
	cfg := eventbus.GetDefaultNATSConfig([]string{"nats://localhost:4222"})
	if err := eventbus.InitializeFromConfig(cfg); err != nil {
		log.Fatal("Failed to initialize EventBus:", err)
	}

	bus := eventbus.GetGlobal()
	log.Println("NATS EventBus initialized successfully")

	// 2. 注意：自动重连功能已内置在 NATS EventBus 中
	// NATS 客户端内置重连 + 应用层自动重连双重保障

	// 3. 注册重连回调
	err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
		log.Printf("🔄 NATS EventBus reconnected successfully at %v", time.Now().Format("15:04:05"))
		
		// 在这里可以执行重连后的初始化逻辑
		// 例如：重新注册某些状态、发送监控指标等
		
		return nil
	})
	if err != nil {
		log.Printf("Failed to register reconnect callback: %v", err)
	} else {
		log.Println("Reconnect callback registered")
	}

	// 4. 创建应用级别的 context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 5. 启动健康检查（包含自动重连）
	if err := bus.StartHealthCheck(ctx); err != nil {
		log.Printf("Failed to start health check: %v", err)
	} else {
		log.Println("Health check with auto-reconnect started")
	}

	// 6. 订阅主题
	topic := "nats.auto-reconnect.demo"
	handler := func(ctx context.Context, message []byte) error {
		log.Printf("📨 Received message: %s", string(message))
		return nil
	}

	if err := bus.Subscribe(ctx, topic, handler); err != nil {
		log.Printf("Failed to subscribe to topic: %v", err)
	} else {
		log.Printf("Subscribed to topic: %s", topic)
	}

	// 7. 设置优雅关闭信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 8. 启动消息发送 goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		
		messageCount := 0
		for {
			select {
			case <-ctx.Done():
				log.Println("Message sender stopped")
				return
			case <-ticker.C:
				messageCount++
				message := []byte(fmt.Sprintf("NATS auto-reconnect test message #%d at %v", 
					messageCount, time.Now().Format("15:04:05")))
				
				if err := bus.Publish(ctx, topic, message); err != nil {
					log.Printf("❌ Failed to publish message: %v", err)
				} else {
					log.Printf("📤 Published message #%d", messageCount)
				}
			}
		}
	}()

	// 9. 启动状态监控 goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				log.Println("Status monitor stopped")
				return
			case <-ticker.C:
				// 获取连接状态
				connState := bus.GetConnectionState()
				
				// 获取健康状态
				healthStatus := bus.GetHealthStatus()
				
				log.Printf("📊 Status - Connected: %v, Healthy: %v, Failures: %d",
					connState.IsConnected,
					healthStatus.IsHealthy,
					healthStatus.ConsecutiveFailures)
			}
		}
	}()

	// 10. 主循环
	log.Println("🚀 NATS Application started. Press Ctrl+C to stop.")
	log.Println("💡 Try stopping NATS server to see auto-reconnect in action!")
	log.Println("💡 NATS provides both client-level and application-level reconnection!")
	
	// 等待退出信号
	<-sigChan
	log.Println("📴 Received shutdown signal, shutting down gracefully...")

	// 11. 优雅关闭序列
	
	// 停止健康检查和自动重连
	log.Println("Stopping health check and auto-reconnect...")
	if err := bus.StopHealthCheck(); err != nil {
		log.Printf("Error stopping health check: %v", err)
	} else {
		log.Println("Health check stopped successfully")
	}

	// 取消应用 context
	cancel()

	// 关闭 EventBus
	log.Println("Closing EventBus...")
	if err := eventbus.CloseGlobal(); err != nil {
		log.Printf("Error closing EventBus: %v", err)
	} else {
		log.Println("EventBus closed successfully")
	}

	log.Println("✅ NATS Application stopped gracefully")
}
