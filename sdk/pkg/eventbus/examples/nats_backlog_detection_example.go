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

// NATSBacklogDetectionExample 演示 NATS JetStream 积压检测功能
func main() {
	fmt.Println("=== NATS JetStream 积压检测示例 ===")

	// 1. 创建 NATS JetStream 配置
	natsConfig := &config.NATSConfig{
		URLs:                []string{"nats://localhost:4222"},
		ClientID:            "backlog-detection-example",
		MaxReconnects:       5,
		ReconnectWait:       2 * time.Second,
		ConnectionTimeout:   10 * time.Second,
		HealthCheckInterval: 30 * time.Second,

		// JetStream 配置
		JetStream: config.JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 5 * time.Second,
			AckWait:        30 * time.Second,
			MaxDeliver:     3,

			// 流配置
			Stream: config.StreamConfig{
				Name:      "example-stream",
				Subjects:  []string{"events.>", "notifications.>"},
				Retention: "limits",
				Storage:   "file",
				Replicas:  1,
				MaxAge:    24 * time.Hour,
				MaxBytes:  1024 * 1024 * 100, // 100MB
				MaxMsgs:   10000,
				Discard:   "old",
			},

			// 消费者配置
			Consumer: config.NATSConsumerConfig{
				DurableName:   "example-consumer",
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 100,
				MaxWaiting:    500,
				MaxDeliver:    3,
			},
		},

		// 积压检测配置
		BacklogDetection: config.BacklogDetectionConfig{
			MaxLagThreshold:  50,             // 超过50条消息认为有积压
			MaxTimeThreshold: 2 * time.Minute, // 超过2分钟认为有积压
			CheckInterval:    10 * time.Second, // 每10秒检查一次
		},

		// 安全配置（可选）
		Security: config.NATSSecurityConfig{
			Enabled: false,
		},
	}

	// 2. 创建 EventBus
	bus, err := eventbus.NewNATSEventBus(natsConfig)
	if err != nil {
		log.Fatalf("Failed to create NATS EventBus: %v", err)
	}
	defer bus.Close()

	fmt.Println("✅ NATS JetStream EventBus 创建成功")

	// 3. 注册积压状态回调
	err = bus.RegisterBacklogCallback(handleBacklogStateChange)
	if err != nil {
		log.Fatalf("Failed to register backlog callback: %v", err)
	}

	fmt.Println("✅ 积压状态回调注册成功")

	// 4. 启动积压监控
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = bus.StartBacklogMonitoring(ctx)
	if err != nil {
		log.Fatalf("Failed to start backlog monitoring: %v", err)
	}

	fmt.Println("✅ 积压监控启动成功")

	// 5. 订阅消息（这会创建消费者并注册到积压检测器）
	err = bus.Subscribe(ctx, "events.user.created", handleUserCreatedEvent)
	if err != nil {
		log.Fatalf("Failed to subscribe to events.user.created: %v", err)
	}

	err = bus.Subscribe(ctx, "notifications.email", handleEmailNotification)
	if err != nil {
		log.Fatalf("Failed to subscribe to notifications.email: %v", err)
	}

	fmt.Println("✅ 消息订阅成功")

	// 6. 发布一些测试消息
	go publishTestMessages(ctx, bus)

	// 7. 等待中断信号
	fmt.Println("🚀 积压检测示例运行中... (按 Ctrl+C 退出)")
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n🛑 收到退出信号，正在优雅关闭...")

	// 8. 停止积压监控
	err = bus.StopBacklogMonitoring()
	if err != nil {
		log.Printf("Failed to stop backlog monitoring: %v", err)
	} else {
		fmt.Println("✅ 积压监控已停止")
	}

	fmt.Println("👋 积压检测示例已退出")
}

// handleBacklogStateChange 处理积压状态变化
func handleBacklogStateChange(ctx context.Context, state eventbus.BacklogState) error {
	timestamp := state.Timestamp.Format("15:04:05")
	
	if state.HasBacklog {
		fmt.Printf("⚠️  [%s] 检测到消息积压！\n", timestamp)
		fmt.Printf("   📊 积压数量: %d 条消息\n", state.LagCount)
		fmt.Printf("   ⏱️  积压时间: %v\n", state.LagTime)
		fmt.Printf("   📝 主题: %s\n", state.Topic)
		fmt.Printf("   👥 消费者组: %s\n", state.ConsumerGroup)
		
		// 这里可以添加告警逻辑
		// 例如：发送邮件、Slack 通知、写入监控系统等
		
	} else {
		fmt.Printf("✅ [%s] 消息积压已清除\n", timestamp)
		fmt.Printf("   📊 当前积压: %d 条消息\n", state.LagCount)
		fmt.Printf("   📝 主题: %s\n", state.Topic)
	}
	
	return nil
}

// handleUserCreatedEvent 处理用户创建事件
func handleUserCreatedEvent(ctx context.Context, message []byte) error {
	fmt.Printf("📨 收到用户创建事件: %s\n", string(message))
	
	// 模拟处理时间
	time.Sleep(100 * time.Millisecond)
	
	return nil
}

// handleEmailNotification 处理邮件通知
func handleEmailNotification(ctx context.Context, message []byte) error {
	fmt.Printf("📧 收到邮件通知: %s\n", string(message))
	
	// 模拟处理时间
	time.Sleep(200 * time.Millisecond)
	
	return nil
}

// publishTestMessages 发布测试消息
func publishTestMessages(ctx context.Context, bus eventbus.EventBus) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	counter := 0
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			counter++
			
			// 发布用户创建事件
			userEvent := fmt.Sprintf(`{"userId": %d, "username": "user%d", "email": "user%d@example.com"}`, counter, counter, counter)
			err := bus.Publish(ctx, "events.user.created", []byte(userEvent))
			if err != nil {
				fmt.Printf("❌ 发布用户事件失败: %v\n", err)
			}
			
			// 发布邮件通知
			emailNotification := fmt.Sprintf(`{"to": "user%d@example.com", "subject": "Welcome!", "body": "Welcome to our service!"}`, counter)
			err = bus.Publish(ctx, "notifications.email", []byte(emailNotification))
			if err != nil {
				fmt.Printf("❌ 发布邮件通知失败: %v\n", err)
			}
			
			// 每10条消息后暂停一下，模拟积压情况
			if counter%10 == 0 {
				fmt.Printf("⏸️  暂停发布，模拟积压情况...\n")
				time.Sleep(30 * time.Second)
				fmt.Printf("▶️  恢复发布消息\n")
			}
		}
	}
}

/*
运行示例的步骤：

1. 启动 NATS 服务器（带 JetStream）：
   nats-server -js

2. 运行示例：
   go run nats_backlog_detection_example.go

3. 观察输出：
   - 正常情况下应该看到消息发布和消费
   - 当发布暂停时，应该看到积压告警
   - 当恢复发布时，应该看到积压清除

示例输出：
=== NATS JetStream 积压检测示例 ===
✅ NATS JetStream EventBus 创建成功
✅ 积压状态回调注册成功
✅ 积压监控启动成功
✅ 消息订阅成功
🚀 积压检测示例运行中... (按 Ctrl+C 退出)
📨 收到用户创建事件: {"userId": 1, "username": "user1", "email": "user1@example.com"}
📧 收到邮件通知: {"to": "user1@example.com", "subject": "Welcome!", "body": "Welcome to our service!"}
...
⏸️  暂停发布，模拟积压情况...
⚠️  [14:30:15] 检测到消息积压！
   📊 积压数量: 52 条消息
   ⏱️  积压时间: 2m15s
   📝 主题: example-stream
   👥 消费者组: example-stream
▶️  恢复发布消息
✅ [14:30:45] 消息积压已清除
   📊 当前积压: 3 条消息
   📝 主题: example-stream
*/
