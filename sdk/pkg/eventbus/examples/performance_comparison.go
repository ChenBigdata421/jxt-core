package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

type TestMessage struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

func main() {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	fmt.Println("=== NATS EventBus 性能对比测试 ===\n")

	// 1. 创建持久化实例
	persistentBus, err := eventbus.NewPersistentNATSEventBus(
		[]string{"nats://localhost:4222"},
		"perf-persistent",
	)
	if err != nil {
		log.Fatalf("Failed to create persistent EventBus: %v", err)
	}
	defer persistentBus.Close()

	// 2. 创建非持久化实例
	ephemeralBus, err := eventbus.NewEphemeralNATSEventBus(
		[]string{"nats://localhost:4222"},
		"perf-ephemeral",
	)
	if err != nil {
		log.Fatalf("Failed to create ephemeral EventBus: %v", err)
	}
	defer ephemeralBus.Close()

	// 3. 创建内存实例
	memoryBus, err := eventbus.NewEventBus(eventbus.GetDefaultConfig("memory"))
	if err != nil {
		log.Fatalf("Failed to create memory EventBus: %v", err)
	}
	defer memoryBus.Close()

	// 测试参数
	messageCount := 1000
	ctx := context.Background()

	fmt.Printf("📊 测试参数: %d 条消息\n\n", messageCount)

	// 测试持久化实例
	fmt.Println("🔄 测试持久化NATS实例 (JetStream)...")
	persistentDuration := testPublishPerformance(ctx, persistentBus, "persistent.test", messageCount)
	fmt.Printf("   ⏱️  耗时: %v\n", persistentDuration)
	fmt.Printf("   📈 吞吐量: %.2f msg/s\n\n", float64(messageCount)/persistentDuration.Seconds())

	// 测试非持久化实例
	fmt.Println("⚡ 测试非持久化NATS实例 (Core NATS)...")
	ephemeralDuration := testPublishPerformance(ctx, ephemeralBus, "ephemeral.test", messageCount)
	fmt.Printf("   ⏱️  耗时: %v\n", ephemeralDuration)
	fmt.Printf("   📈 吞吐量: %.2f msg/s\n\n", float64(messageCount)/ephemeralDuration.Seconds())

	// 测试内存实例
	fmt.Println("🚀 测试内存实例...")
	memoryDuration := testPublishPerformance(ctx, memoryBus, "memory.test", messageCount)
	fmt.Printf("   ⏱️  耗时: %v\n", memoryDuration)
	fmt.Printf("   📈 吞吐量: %.2f msg/s\n\n", float64(messageCount)/memoryDuration.Seconds())

	// 性能对比
	fmt.Println("=== 性能对比结果 ===")
	fmt.Printf("📊 持久化NATS:   %.2f msg/s\n", float64(messageCount)/persistentDuration.Seconds())
	fmt.Printf("⚡ 非持久化NATS: %.2f msg/s (%.1fx)\n",
		float64(messageCount)/ephemeralDuration.Seconds(),
		persistentDuration.Seconds()/ephemeralDuration.Seconds())
	fmt.Printf("🚀 内存实例:     %.2f msg/s (%.1fx)\n",
		float64(messageCount)/memoryDuration.Seconds(),
		persistentDuration.Seconds()/memoryDuration.Seconds())

	fmt.Println("\n💡 结论:")
	fmt.Println("   - 持久化NATS: 适合关键业务数据，有持久化保证")
	fmt.Println("   - 非持久化NATS: 适合实时通信，性能更高")
	fmt.Println("   - 内存实例: 适合进程内通信，性能最高")
}

func testPublishPerformance(ctx context.Context, bus eventbus.EventBus, topic string, count int) time.Duration {
	var wg sync.WaitGroup
	wg.Add(count)

	// 订阅消息（用于确保消息被处理）
	bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		wg.Done()
		return nil
	})

	// 等待订阅建立
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	publishStart := time.Now()
	for i := 0; i < count; i++ {
		msg := TestMessage{
			ID:        fmt.Sprintf("msg-%d", i),
			Timestamp: time.Now(),
			Data:      fmt.Sprintf("test data %d", i),
		}

		data, _ := json.Marshal(msg)
		bus.Publish(ctx, topic, data)
	}

	// 等待所有消息被处理
	wg.Wait()

	return time.Since(publishStart)
}
