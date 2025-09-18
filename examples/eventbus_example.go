//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.uber.org/zap"

	"github.com/ChenBigdata421/jxt-core/sdk"
	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

func main() {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	fmt.Println("=== JXT-Core EventBus 示例 ===")

	// 示例1: 直接使用EventBus
	fmt.Println("\n1. 直接使用EventBus")
	directEventBusExample()

	// 示例2: 通过SDK Runtime使用EventBus
	fmt.Println("\n2. 通过SDK Runtime使用EventBus")
	sdkRuntimeExample()

	// 示例3: 使用配置文件初始化
	fmt.Println("\n3. 使用配置初始化EventBus")
	configExample()

	fmt.Println("\n=== 示例完成 ===")
}

// directEventBusExample 直接使用EventBus的示例
func directEventBusExample() {
	// 创建EventBus配置
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
		Metrics: eventbus.MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
	}

	// 创建工厂
	factory := eventbus.NewFactory(cfg)

	// 创建EventBus实例
	bus, err := factory.CreateEventBus()
	if err != nil {
		log.Fatalf("Failed to create eventbus: %v", err)
	}
	defer bus.Close()

	// 订阅消息
	ctx := context.Background()
	topic := "user_events"

	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		fmt.Printf("  收到消息: %s\n", string(message))
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// 发布消息
	messages := []string{
		`{"event": "user_created", "user_id": "123"}`,
		`{"event": "user_updated", "user_id": "123"}`,
		`{"event": "user_deleted", "user_id": "123"}`,
	}

	for _, msg := range messages {
		if err := bus.Publish(ctx, topic, []byte(msg)); err != nil {
			log.Printf("Failed to publish message: %v", err)
		}
	}

	// 等待消息处理
	time.Sleep(100 * time.Millisecond)

	// 健康检查
	if err := bus.HealthCheck(ctx); err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		fmt.Println("  健康检查通过")
	}
}

// sdkRuntimeExample 通过SDK Runtime使用EventBus的示例
func sdkRuntimeExample() {
	// 创建EventBus配置
	cfg := &config.EventBus{
		Type: "memory",
		Metrics: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
	}

	// 初始化全局EventBus
	if err := eventbus.InitializeFromConfig(cfg); err != nil {
		log.Fatalf("Failed to initialize global eventbus: %v", err)
	}
	defer eventbus.CloseGlobal()

	// 设置到SDK Runtime
	bus := eventbus.GetGlobal()
	sdk.Runtime.SetEventBus(bus)

	// 从SDK Runtime获取EventBus
	runtimeBus := sdk.Runtime.GetEventBus()
	if runtimeBus == nil {
		log.Fatal("Failed to get eventbus from runtime")
	}

	// 使用EventBus
	ctx := context.Background()
	topic := "order_events"

	err := runtimeBus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		fmt.Printf("  Runtime收到消息: %s\n", string(message))
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// 发布消息
	orderEvents := []string{
		`{"event": "order_created", "order_id": "ORD001"}`,
		`{"event": "order_paid", "order_id": "ORD001"}`,
		`{"event": "order_shipped", "order_id": "ORD001"}`,
	}

	for _, msg := range orderEvents {
		if err := runtimeBus.Publish(ctx, topic, []byte(msg)); err != nil {
			log.Printf("Failed to publish message: %v", err)
		}
	}

	// 等待消息处理
	time.Sleep(100 * time.Millisecond)

	fmt.Println("  Runtime EventBus示例完成")
}

// configExample 使用配置初始化EventBus的示例
func configExample() {
	// 创建配置
	cfg := &config.EventBus{
		Type: "memory",
		Metrics: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
		Tracing: config.TracingConfig{
			Enabled:    false,
			SampleRate: 0.1,
		},
	}

	// 从配置初始化
	if err := eventbus.InitializeFromConfig(cfg); err != nil {
		log.Fatalf("Failed to initialize from config: %v", err)
	}
	defer eventbus.CloseGlobal()

	// 获取EventBus实例
	bus := eventbus.GetGlobal()
	if bus == nil {
		log.Fatal("EventBus not available")
	}

	// 测试多个主题
	ctx := context.Background()
	topics := []string{"notifications", "analytics", "audit"}

	// 为每个主题订阅
	for _, topic := range topics {
		topicName := topic // 避免闭包问题
		err := bus.Subscribe(ctx, topicName, func(ctx context.Context, message []byte) error {
			fmt.Printf("  [%s] 收到消息: %s\n", topicName, string(message))
			return nil
		})
		if err != nil {
			log.Printf("Failed to subscribe to %s: %v", topicName, err)
		}
	}

	// 发布到不同主题
	events := map[string][]string{
		"notifications": {
			`{"type": "email", "to": "user@example.com"}`,
			`{"type": "sms", "to": "+1234567890"}`,
		},
		"analytics": {
			`{"event": "page_view", "page": "/home"}`,
			`{"event": "click", "element": "button"}`,
		},
		"audit": {
			`{"action": "login", "user": "admin"}`,
			`{"action": "logout", "user": "admin"}`,
		},
	}

	for topic, messages := range events {
		for _, msg := range messages {
			if err := bus.Publish(ctx, topic, []byte(msg)); err != nil {
				log.Printf("Failed to publish to %s: %v", topic, err)
			}
		}
	}

	// 等待消息处理
	time.Sleep(200 * time.Millisecond)

	// 健康检查
	if err := bus.HealthCheck(ctx); err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		fmt.Println("  配置示例健康检查通过")
	}
}
