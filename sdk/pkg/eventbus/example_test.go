package eventbus_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// Example_memoryEventBus 演示内存事件总线的使用
func Example_memoryEventBus() {
	// 创建内存事件总线配置
	cfg := eventbus.GetDefaultMemoryConfig()

	// 初始化事件总线
	if err := eventbus.InitializeFromConfig(cfg); err != nil {
		log.Fatalf("Failed to initialize eventbus: %v", err)
	}
	defer eventbus.CloseGlobal()

	// 获取事件总线实例
	bus := eventbus.GetGlobal()
	if bus == nil {
		log.Fatal("EventBus not initialized")
	}

	// 订阅消息
	ctx := context.Background()
	topic := "test_topic"

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		fmt.Printf("Received message: %s\n", string(message))
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// 发布消息
	message := []byte("Hello, EventBus!")
	if err := bus.Publish(ctx, topic, message); err != nil {
		log.Fatalf("Failed to publish: %v", err)
	}

	// 等待消息处理
	time.Sleep(100 * time.Millisecond)

	// 健康检查
	if err := bus.HealthCheck(ctx); err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Println("EventBus example completed successfully")
	// Output: Received message: Hello, EventBus!
	// EventBus example completed successfully
}

// Example_kafkaEventBus 演示Kafka事件总线的使用（需要Kafka服务）
func Example_kafkaEventBus() {
	// 创建Kafka事件总线配置
	cfg := eventbus.GetDefaultKafkaConfig([]string{"localhost:9092"})

	// 初始化事件总线
	if err := eventbus.InitializeFromConfig(cfg); err != nil {
		log.Printf("Failed to initialize Kafka eventbus (this is expected if Kafka is not running): %v", err)
		return
	}
	defer eventbus.CloseGlobal()

	// 获取事件总线实例
	bus := eventbus.GetGlobal()
	if bus == nil {
		log.Fatal("EventBus not initialized")
	}

	// 订阅消息
	ctx := context.Background()
	topic := "kafka_test_topic"

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		fmt.Printf("Received Kafka message: %s\n", string(message))
		return nil
	})
	if err != nil {
		log.Printf("Failed to subscribe to Kafka: %v", err)
		return
	}

	// 发布消息
	message := []byte("Hello, Kafka EventBus!")
	if err := bus.Publish(ctx, topic, message); err != nil {
		log.Printf("Failed to publish to Kafka: %v", err)
		return
	}

	// 等待消息处理
	time.Sleep(1 * time.Second)

	// 健康检查
	if err := bus.HealthCheck(ctx); err != nil {
		log.Printf("Kafka health check failed: %v", err)
		return
	}

	fmt.Println("Kafka EventBus example completed successfully")
}

// Example_eventBusFactory 演示事件总线工厂的使用
func Example_eventBusFactory() {
	// 创建工厂
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
		Metrics: eventbus.MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
	}

	factory := eventbus.NewFactory(cfg)

	// 创建事件总线实例
	bus, err := factory.CreateEventBus()
	if err != nil {
		log.Fatalf("Failed to create eventbus: %v", err)
	}
	defer bus.Close()

	// 使用事件总线
	ctx := context.Background()
	topic := "factory_test_topic"

	// 订阅
	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		fmt.Printf("Factory example received: %s\n", string(message))
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// 发布
	message := []byte("Hello from Factory!")
	if err := bus.Publish(ctx, topic, message); err != nil {
		log.Fatalf("Failed to publish: %v", err)
	}

	// 等待处理
	time.Sleep(100 * time.Millisecond)

	fmt.Println("Factory example completed successfully")
	// Output: Factory example received: Hello from Factory!
	// Factory example completed successfully
}

// Example_configIntegration 演示配置集成的使用
func Example_configIntegration() {
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

	// 检查是否初始化成功
	if !eventbus.IsInitialized() {
		log.Fatal("EventBus should be initialized")
	}

	// 获取实例
	bus := eventbus.GetGlobal()
	if bus == nil {
		log.Fatal("EventBus not available")
	}

	// 简单测试
	ctx := context.Background()
	if err := bus.HealthCheck(ctx); err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Println("Config integration example completed successfully")
	// Output: Config integration example completed successfully
}

// Example_healthCheck 演示健康检查的使用
func Example_healthCheck() {
	// 初始化内存事件总线
	cfg := eventbus.GetDefaultMemoryConfig()
	if err := eventbus.InitializeFromConfig(cfg); err != nil {
		log.Fatalf("Failed to initialize eventbus: %v", err)
	}
	defer eventbus.CloseGlobal()

	bus := eventbus.GetGlobal()
	ctx := context.Background()

	// 执行健康检查
	if err := bus.HealthCheck(ctx); err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Println("Health check passed")
	// Output: Health check passed
}

// Example_reconnectCallback 演示重连回调的使用
func Example_reconnectCallback() {
	// 初始化内存事件总线
	cfg := eventbus.GetDefaultMemoryConfig()
	if err := eventbus.InitializeFromConfig(cfg); err != nil {
		log.Fatalf("Failed to initialize eventbus: %v", err)
	}
	defer eventbus.CloseGlobal()

	bus := eventbus.GetGlobal()

	// 注册重连回调
	err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
		fmt.Println("Reconnect callback triggered")
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to register reconnect callback: %v", err)
	}

	fmt.Println("Reconnect callback registered successfully")
	// Output: Reconnect callback registered successfully
}
