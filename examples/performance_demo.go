//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	fmt.Println("=== JXT-Core EventBus 性能测试 ===")

	// 初始化logger
	logger.Logger = zap.NewExample()
	logger.DefaultLogger = logger.Logger.Sugar()

	// 创建内存事件总线
	config := &eventbus.EventBusConfig{
		Type: "memory",
	}

	bus, err := eventbus.NewEventBus(config)
	if err != nil {
		panic(fmt.Sprintf("创建EventBus失败: %v", err))
	}
	defer bus.Close()

	// 性能测试参数
	numMessages := 10000
	numWorkers := 10
	topic := "performance_test"

	fmt.Printf("测试参数: %d 条消息, %d 个工作协程\n", numMessages, numWorkers)

	// 消息计数器
	var receivedCount int64
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 订阅消息
	err = bus.Subscribe(context.Background(), topic, func(ctx context.Context, message []byte) error {
		mu.Lock()
		receivedCount++
		current := receivedCount
		mu.Unlock()

		if current%1000 == 0 {
			fmt.Printf("已处理 %d 条消息\n", current)
		}
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("订阅失败: %v", err))
	}

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 开始性能测试
	startTime := time.Now()

	// 启动多个发送协程
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			messagesPerWorker := numMessages / numWorkers

			for j := 0; j < messagesPerWorker; j++ {
				message := fmt.Sprintf(`{"worker_id": %d, "message_id": %d, "timestamp": "%s"}`,
					workerID, j, time.Now().Format(time.RFC3339))

				err := bus.Publish(context.Background(), topic, []byte(message))
				if err != nil {
					fmt.Printf("发送消息失败: %v\n", err)
				}
			}
		}(i)
	}

	// 等待所有发送完成
	wg.Wait()
	sendDuration := time.Since(startTime)

	// 等待所有消息处理完成
	for {
		mu.Lock()
		current := receivedCount
		mu.Unlock()

		if current >= int64(numMessages) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	totalDuration := time.Since(startTime)

	// 输出性能统计
	fmt.Println("\n=== 性能测试结果 ===")
	fmt.Printf("总消息数: %d\n", numMessages)
	fmt.Printf("工作协程数: %d\n", numWorkers)
	fmt.Printf("发送耗时: %v\n", sendDuration)
	fmt.Printf("总耗时: %v\n", totalDuration)
	fmt.Printf("发送速率: %.2f 消息/秒\n", float64(numMessages)/sendDuration.Seconds())
	fmt.Printf("处理速率: %.2f 消息/秒\n", float64(numMessages)/totalDuration.Seconds())
	fmt.Printf("平均延迟: %v\n", totalDuration/time.Duration(numMessages))

	// 健康检查
	err = bus.HealthCheck(context.Background())
	if err == nil {
		fmt.Println("健康检查: 通过")
	} else {
		fmt.Printf("健康检查: 失败 - %v\n", err)
	}

	fmt.Println("\n=== 性能测试完成 ===")
}
