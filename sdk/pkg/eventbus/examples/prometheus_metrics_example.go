//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusMetricsExample 演示如何为 EventBus 集成 Prometheus 监控
//
// 运行步骤：
// 1. 启动 Kafka（或使用 Memory EventBus）
// 2. 运行此示例程序
// 3. 访问 http://localhost:9090/metrics 查看 Prometheus 指标
// 4. 配置 Prometheus 抓取此端点
//
// Prometheus 配置示例：
//
//	scrape_configs:
//	  - job_name: 'eventbus'
//	    static_configs:
//	      - targets: ['localhost:9090']
func main() {
	// ========== 1. 创建 Prometheus 指标收集器 ==========

	// 创建 Prometheus 收集器（使用服务名称作为命名空间）
	metricsCollector := eventbus.NewPrometheusMetricsCollector("my_service")

	fmt.Println("✅ Prometheus 指标收集器已创建")

	// ========== 2. 配置 EventBus ==========

	// 使用 Memory EventBus（也可以使用 Kafka 或 NATS）
	config := eventbus.GetDefaultConfig("memory")

	// 注入 Prometheus 指标收集器
	config.MetricsCollector = metricsCollector

	// 创建 EventBus
	bus, err := eventbus.NewEventBus(config)
	if err != nil {
		log.Fatal("Failed to create EventBus:", err)
	}
	defer bus.Close()

	fmt.Println("✅ EventBus 已创建并集成 Prometheus 监控")

	// ========== 3. 启动 Prometheus HTTP 服务器 ==========

	// 启动 HTTP 服务器，暴露 /metrics 端点
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		fmt.Println("🚀 Prometheus metrics server started at http://localhost:9090/metrics")
		if err := http.ListenAndServe(":9090", nil); err != nil {
			log.Fatal("Failed to start metrics server:", err)
		}
	}()

	// 等待 HTTP 服务器启动
	time.Sleep(1 * time.Second)

	// ========== 4. 订阅消息 ==========

	ctx := context.Background()
	topic := "user_events"

	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		fmt.Printf("📨 Received: %s\n", string(message))

		// 模拟处理时间
		time.Sleep(10 * time.Millisecond)

		return nil
	})
	if err != nil {
		log.Fatal("Failed to subscribe:", err)
	}

	fmt.Println("✅ 已订阅主题:", topic)

	// ========== 5. 发布消息 ==========

	fmt.Println("\n📤 开始发布消息...")

	for i := 1; i <= 100; i++ {
		message := []byte(fmt.Sprintf(`{"event": "user_created", "user_id": "%d"}`, i))

		if err := bus.Publish(ctx, topic, message); err != nil {
			log.Printf("Failed to publish message %d: %v", i, err)
			// 发布失败也会被记录到 Prometheus 指标中
		}

		// 控制发布速率
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Println("✅ 已发布 100 条消息")

	// ========== 6. 查看 Prometheus 指标 ==========

	fmt.Println("\n📊 Prometheus 指标已记录，可以通过以下方式查看：")
	fmt.Println("   1. 浏览器访问: http://localhost:9090/metrics")
	fmt.Println("   2. 使用 curl: curl http://localhost:9090/metrics")
	fmt.Println("\n可用的指标包括：")
	fmt.Println("   - my_service_eventbus_publish_total{topic=\"user_events\"}")
	fmt.Println("   - my_service_eventbus_publish_success_total{topic=\"user_events\"}")
	fmt.Println("   - my_service_eventbus_publish_failed_total{topic=\"user_events\"}")
	fmt.Println("   - my_service_eventbus_publish_latency_seconds{topic=\"user_events\"}")
	fmt.Println("   - my_service_eventbus_consume_total{topic=\"user_events\"}")
	fmt.Println("   - my_service_eventbus_consume_success_total{topic=\"user_events\"}")
	fmt.Println("   - my_service_eventbus_consume_failed_total{topic=\"user_events\"}")
	fmt.Println("   - my_service_eventbus_consume_latency_seconds{topic=\"user_events\"}")
	fmt.Println("   - my_service_eventbus_connected")
	fmt.Println("   - my_service_eventbus_errors_total{error_type=\"...\"}")

	// 保持程序运行，以便查看指标
	fmt.Println("\n⏳ 程序将运行 60 秒，以便查看 Prometheus 指标...")
	fmt.Println("   按 Ctrl+C 可提前退出")
	time.Sleep(60 * time.Second)

	fmt.Println("\n✅ 示例程序结束")
}

// KafkaPrometheusExample 演示如何为 Kafka EventBus 集成 Prometheus 监控
func KafkaPrometheusExample() {
	// ========== 1. 创建 Prometheus 指标收集器 ==========

	metricsCollector := eventbus.NewPrometheusMetricsCollector("order_service")

	// ========== 2. 配置 Kafka EventBus ==========

	config := &eventbus.EventBusConfig{
		Type: "kafka",
		Kafka: eventbus.KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Consumer: eventbus.ConsumerConfig{
				GroupID: "order-service-group",
			},
		},
		MetricsCollector: metricsCollector, // 注入 Prometheus 收集器
	}

	bus, err := eventbus.NewEventBus(config)
	if err != nil {
		log.Fatal("Failed to create Kafka EventBus:", err)
	}
	defer bus.Close()

	// ========== 3. 启动 Prometheus HTTP 服务器 ==========

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Println("Prometheus metrics server started at :9090")
		if err := http.ListenAndServe(":9090", nil); err != nil {
			log.Fatal("Failed to start metrics server:", err)
		}
	}()

	// ========== 4. 使用 EventBus ==========

	ctx := context.Background()

	// 订阅订单事件
	err = bus.Subscribe(ctx, "orders.events", func(ctx context.Context, message []byte) error {
		// 处理订单事件
		// 所有指标（成功/失败/延迟）都会自动记录到 Prometheus
		return processOrderEvent(message)
	})
	if err != nil {
		log.Fatal("Failed to subscribe:", err)
	}

	// 发布订单事件
	orderEvent := []byte(`{"order_id": "123", "status": "created"}`)
	if err := bus.Publish(ctx, "orders.events", orderEvent); err != nil {
		log.Fatal("Failed to publish:", err)
	}

	// 保持程序运行
	select {}
}

func processOrderEvent(message []byte) error {
	// 处理订单事件的业务逻辑
	fmt.Printf("Processing order event: %s\n", string(message))
	return nil
}

// InMemoryMetricsExample 演示如何使用内存指标收集器（用于测试和调试）
func InMemoryMetricsExample() {
	// ========== 1. 创建内存指标收集器 ==========

	metricsCollector := eventbus.NewInMemoryMetricsCollector()

	// ========== 2. 配置 EventBus ==========

	config := eventbus.GetDefaultConfig("memory")
	config.MetricsCollector = metricsCollector

	bus, err := eventbus.NewEventBus(config)
	if err != nil {
		log.Fatal("Failed to create EventBus:", err)
	}
	defer bus.Close()

	// ========== 3. 使用 EventBus ==========

	ctx := context.Background()
	topic := "test_events"

	// 订阅
	bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		return nil
	})

	// 发布
	for i := 0; i < 10; i++ {
		bus.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
	}

	// ========== 4. 查看内存指标 ==========

	metrics := metricsCollector.GetMetrics()
	fmt.Printf("📊 内存指标:\n")
	fmt.Printf("   发布总数: %v\n", metrics["publish_total"])
	fmt.Printf("   发布成功: %v\n", metrics["publish_success"])
	fmt.Printf("   发布失败: %v\n", metrics["publish_failed"])
	fmt.Printf("   消费总数: %v\n", metrics["consume_total"])
	fmt.Printf("   消费成功: %v\n", metrics["consume_success"])
	fmt.Printf("   消费失败: %v\n", metrics["consume_failed"])
}
