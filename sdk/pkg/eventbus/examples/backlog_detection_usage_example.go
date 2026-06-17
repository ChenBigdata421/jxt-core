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
)

// 发送端积压回调处理器
func handlePublisherBacklogState(ctx context.Context, state eventbus.PublisherBacklogState) error {
	log.Printf("📊 Publisher Backlog State: HasBacklog=%v, QueueDepth=%d, PublishRate=%.2f msg/s, AvgLatency=%v, Severity=%s",
		state.HasBacklog, state.QueueDepth, state.PublishRate, state.AvgPublishLatency, state.Severity)

	// 根据积压状态采取不同的应对策略
	switch state.Severity {
	case "CRITICAL":
		log.Printf("🚨 CRITICAL: Publisher backlog is critical! Taking emergency action...")
		// 实施紧急措施：暂停发布、切换到备用队列等
		return handleCriticalPublisherBacklog(ctx, state)
	case "HIGH":
		log.Printf("⚠️  HIGH: Publisher backlog is high, implementing throttling...")
		// 实施限流措施
		return handleHighPublisherBacklog(ctx, state)
	case "MEDIUM":
		log.Printf("⚡ MEDIUM: Publisher backlog detected, optimizing batch size...")
		// 优化发布策略
		return handleMediumPublisherBacklog(ctx, state)
	case "LOW":
		log.Printf("💡 LOW: Minor publisher backlog, monitoring...")
		// 继续监控
		return nil
	default:
		log.Printf("✅ NORMAL: Publisher performance is normal")
		return nil
	}
}

// 订阅端积压回调处理器
func handleSubscriberBacklogState(ctx context.Context, state eventbus.BacklogState) error {
	log.Printf("📈 Subscriber Backlog State: HasBacklog=%v, LagCount=%d, LagTime=%v, Topic=%s",
		state.HasBacklog, state.LagCount, state.LagTime, state.Topic)

	// 根据积压状态采取不同的应对策略
	// 简单的严重程度判断逻辑
	var severity string
	if state.LagCount > 5000 || state.LagTime > 10*time.Minute {
		severity = "CRITICAL"
	} else if state.LagCount > 2000 || state.LagTime > 5*time.Minute {
		severity = "HIGH"
	} else if state.LagCount > 1000 || state.LagTime > 2*time.Minute {
		severity = "MEDIUM"
	} else if state.LagCount > 100 || state.LagTime > 30*time.Second {
		severity = "LOW"
	} else {
		severity = "NORMAL"
	}

	switch severity {
	case "CRITICAL":
		log.Printf("🚨 CRITICAL: Subscriber backlog is critical! Scaling up consumers...")
		// 实施紧急措施：增加消费者实例、优化处理逻辑等
		return handleCriticalSubscriberBacklog(ctx, state)
	case "HIGH":
		log.Printf("⚠️  HIGH: Subscriber backlog is high, increasing concurrency...")
		// 增加并发处理
		return handleHighSubscriberBacklog(ctx, state)
	case "MEDIUM":
		log.Printf("⚡ MEDIUM: Subscriber backlog detected, optimizing processing...")
		// 优化处理策略
		return handleMediumSubscriberBacklog(ctx, state)
	case "LOW":
		log.Printf("💡 LOW: Minor subscriber backlog, monitoring...")
		// 继续监控
		return nil
	default:
		log.Printf("✅ NORMAL: Subscriber performance is normal")
		return nil
	}
}

// 处理关键级别的发送端积压
func handleCriticalPublisherBacklog(ctx context.Context, state eventbus.PublisherBacklogState) error {
	log.Printf("🔥 Implementing critical publisher backlog mitigation:")
	log.Printf("   - Pausing non-critical message publishing")
	log.Printf("   - Switching to emergency batch mode")
	log.Printf("   - Alerting operations team")

	// 这里可以实现具体的应对措施
	// 例如：暂停发布、切换到备用队列、发送告警等

	return nil
}

// 处理高级别的发送端积压
func handleHighPublisherBacklog(ctx context.Context, state eventbus.PublisherBacklogState) error {
	log.Printf("🔧 Implementing high publisher backlog mitigation:")
	log.Printf("   - Reducing publish rate by 50%%")
	log.Printf("   - Increasing batch size to %d", int(float64(state.QueueDepth)*0.1))
	log.Printf("   - Enabling compression")

	return nil
}

// 处理中等级别的发送端积压
func handleMediumPublisherBacklog(ctx context.Context, state eventbus.PublisherBacklogState) error {
	log.Printf("⚙️  Implementing medium publisher backlog optimization:")
	log.Printf("   - Adjusting batch size based on queue depth")
	log.Printf("   - Optimizing message serialization")

	return nil
}

// 处理关键级别的订阅端积压
func handleCriticalSubscriberBacklog(ctx context.Context, state eventbus.BacklogState) error {
	log.Printf("🔥 Implementing critical subscriber backlog mitigation:")
	log.Printf("   - Scaling up consumer instances")
	log.Printf("   - Enabling parallel processing")
	log.Printf("   - Alerting operations team")

	return nil
}

// 处理高级别的订阅端积压
func handleHighSubscriberBacklog(ctx context.Context, state eventbus.BacklogState) error {
	log.Printf("🔧 Implementing high subscriber backlog mitigation:")
	log.Printf("   - Increasing consumer concurrency")
	log.Printf("   - Optimizing message processing logic")
	log.Printf("   - Enabling batch processing")

	return nil
}

// 处理中等级别的订阅端积压
func handleMediumSubscriberBacklog(ctx context.Context, state eventbus.BacklogState) error {
	log.Printf("⚙️  Implementing medium subscriber backlog optimization:")
	log.Printf("   - Fine-tuning processing parameters")
	log.Printf("   - Optimizing database queries")

	return nil
}

func main() {
	log.Println("🚀 Starting EventBus Backlog Detection Example")

	// 创建配置
	cfg := &config.EventBusConfig{
		Type:        "kafka", // 可以改为 "nats" 或 "memory"
		ServiceName: "backlog-detection-example",

		// 发布端配置
		Publisher: config.PublisherConfig{
			PublishTimeout: 10 * time.Second,
			BacklogDetection: config.PublisherBacklogDetectionConfig{
				Enabled:           true,
				MaxQueueDepth:     1000,
				MaxPublishLatency: 5 * time.Second,
				RateThreshold:     500.0,
				CheckInterval:     30 * time.Second,
			},
		},

		// 订阅端配置
		Subscriber: config.SubscriberConfig{
			MaxConcurrency: 10,
			ProcessTimeout: 30 * time.Second,
			BacklogDetection: config.SubscriberBacklogDetectionConfig{
				Enabled:          true,
				MaxLagThreshold:  1000,
				MaxTimeThreshold: 5 * time.Minute,
				CheckInterval:    30 * time.Second,
			},
		},

		// Kafka 配置
		Kafka: config.KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Producer: config.ProducerConfig{
				RequiredAcks: 1,
				Timeout:      10 * time.Second,
				Compression:  "snappy",
			},
			Consumer: config.ConsumerConfig{
				GroupID:           "backlog-detection-group",
				SessionTimeout:    30 * time.Second,
				HeartbeatInterval: 3 * time.Second,
			},
		},

		// 健康检查配置
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Sender: config.HealthCheckSenderConfig{
				Interval:         30 * time.Second,
				Timeout:          10 * time.Second,
				FailureThreshold: 3,
			},
		},

		// 监控配置
		Monitoring: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 60 * time.Second,
		},
	}

	// 初始化 EventBus
	if err := eventbus.InitializeFromConfig(cfg); err != nil {
		log.Fatalf("❌ Failed to initialize EventBus: %v", err)
	}
	defer eventbus.CloseGlobal()

	bus := eventbus.GetGlobal()
	log.Println("✅ EventBus initialized successfully")

	// 注册积压回调
	if err := bus.RegisterPublisherBacklogCallback(handlePublisherBacklogState); err != nil {
		log.Printf("⚠️  Failed to register publisher backlog callback: %v", err)
	} else {
		log.Println("📝 Publisher backlog callback registered")
	}

	if err := bus.RegisterBacklogCallback(handleSubscriberBacklogState); err != nil {
		log.Printf("⚠️  Failed to register subscriber backlog callback: %v", err)
	} else {
		log.Println("📝 Subscriber backlog callback registered")
	}

	// 创建应用级别的 context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动所有积压监控
	if err := bus.StartAllBacklogMonitoring(ctx); err != nil {
		log.Printf("⚠️  Failed to start all backlog monitoring: %v", err)
	} else {
		log.Println("🔍 All backlog monitoring started")
	}

	// 设置订阅处理器
	messageHandler := func(ctx context.Context, message []byte) error {
		log.Printf("📨 Received message: %s", string(message))
		// 模拟处理时间
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	// 订阅消息
	if err := bus.Subscribe(ctx, "test-topic", messageHandler); err != nil {
		log.Printf("⚠️  Failed to subscribe: %v", err)
	} else {
		log.Println("👂 Subscribed to test-topic")
	}

	// 启动消息发布协程
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		counter := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				counter++
				message := fmt.Sprintf("Test message %d at %s", counter, time.Now().Format(time.RFC3339))

				if err := bus.Publish(ctx, "test-topic", []byte(message)); err != nil {
					log.Printf("⚠️  Failed to publish message: %v", err)
				} else {
					log.Printf("📤 Published: %s", message)
				}
			}
		}
	}()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("🔄 EventBus is running. Press Ctrl+C to stop...")
	<-sigChan

	log.Println("🛑 Shutting down...")

	// 停止所有积压监控
	if err := bus.StopAllBacklogMonitoring(); err != nil {
		log.Printf("⚠️  Failed to stop all backlog monitoring: %v", err)
	} else {
		log.Println("🔍 All backlog monitoring stopped")
	}

	log.Println("👋 EventBus Backlog Detection Example completed")
}
