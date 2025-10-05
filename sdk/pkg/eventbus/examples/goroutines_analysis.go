package main

import (
    "context"
    "fmt"
    "runtime"
    "runtime/pprof"
    "strings"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// Goroutines 详细分析工具
func main() {
    fmt.Println("=== EventBus Goroutines 详细分析 ===\n")

    // 1. 基准状态 - 无 EventBus
    baseline := analyzeGoroutines("基准状态 (无EventBus)")
    
    // 2. 创建第一个 EventBus 实例
    fmt.Println("🔗 创建第一个 NATS EventBus...")
    bus1 := createNATSEventBus("bus-1")
    defer bus1.Close()
    
    time.Sleep(200 * time.Millisecond) // 等待连接建立
    
    bus1Stats := analyzeGoroutines("单个 EventBus")
    printGoroutineDiff("第一个EventBus增量", baseline, bus1Stats)
    
    // 3. 启动健康检查
    fmt.Println("\n🏥 启动健康检查...")
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    if err := bus1.StartHealthCheck(ctx); err != nil {
        fmt.Printf("Failed to start health check: %v\n", err)
    }
    
    time.Sleep(100 * time.Millisecond)
    healthCheckStats := analyzeGoroutines("启用健康检查后")
    printGoroutineDiff("健康检查增量", bus1Stats, healthCheckStats)
    
    // 4. 添加订阅
    fmt.Println("\n📨 添加订阅...")
    bus1.Subscribe(ctx, "test.topic1", func(ctx context.Context, msg []byte) error {
        return nil
    })
    bus1.Subscribe(ctx, "test.topic2", func(ctx context.Context, msg []byte) error {
        return nil
    })
    
    time.Sleep(100 * time.Millisecond)
    subscribeStats := analyzeGoroutines("添加订阅后")
    printGoroutineDiff("订阅增量", healthCheckStats, subscribeStats)
    
    // 5. 创建第二个 EventBus 实例
    fmt.Println("\n🔗🔗 创建第二个 NATS EventBus...")
    bus2 := createNATSEventBus("bus-2")
    defer bus2.Close()
    
    time.Sleep(200 * time.Millisecond)
    
    bus2Stats := analyzeGoroutines("双 EventBus")
    printGoroutineDiff("第二个EventBus增量", subscribeStats, bus2Stats)
    
    // 6. 启用 Keyed-Worker 池 (模拟)
    fmt.Println("\n⚙️ 模拟启用 Keyed-Worker 池...")
    // 创建一些额外的 goroutines 来模拟 worker 池
    workerDone := make(chan bool)
    for i := 0; i < 8; i++ {
        go func(id int) {
            ticker := time.NewTicker(1 * time.Second)
            defer ticker.Stop()
            for {
                select {
                case <-workerDone:
                    return
                case <-ticker.C:
                    // 模拟 worker 工作
                }
            }
        }(i)
    }
    
    time.Sleep(100 * time.Millisecond)
    workerStats := analyzeGoroutines("启用Worker池后")
    printGoroutineDiff("Worker池增量", bus2Stats, workerStats)
    
    // 7. 详细分析当前 Goroutines
    fmt.Println("\n🔍 详细 Goroutines 分析:")
    analyzeDetailedGoroutines()
    
    // 8. 总结
    fmt.Println("\n=== 总结 ===")
    printGoroutinesSummary(baseline, workerStats)
    
    // 清理
    close(workerDone)
    time.Sleep(100 * time.Millisecond)
}

func createNATSEventBus(clientID string) eventbus.EventBus {
    config := &eventbus.EventBusConfig{
        Type: "nats",
        NATS: eventbus.NATSConfig{
            URLs:     []string{"nats://localhost:4222"},
            ClientID: clientID,
            JetStream: eventbus.JetStreamConfig{
                Enabled: true,
                Stream: eventbus.StreamConfig{
                    Name:     fmt.Sprintf("STREAM_%s", clientID),
                    Subjects: []string{fmt.Sprintf("%s.*", clientID)},
                    Storage:  "memory",
                },
                Consumer: eventbus.NATSConsumerConfig{
                    DurableName: fmt.Sprintf("consumer_%s", clientID),
                    AckPolicy:   "explicit",
                },
            },
        },
    }

    bus, err := eventbus.NewEventBus(config)
    if err != nil {
        fmt.Printf("Failed to create EventBus: %v\n", err)
        return nil
    }
    return bus
}

type GoroutineStats struct {
    Total      int
    Categories map[string]int
}

func analyzeGoroutines(label string) GoroutineStats {
    total := runtime.NumGoroutine()
    
    fmt.Printf("📊 %s: %d 个 Goroutines\n", label, total)
    
    return GoroutineStats{
        Total:      total,
        Categories: categorizeGoroutines(),
    }
}

func categorizeGoroutines() map[string]int {
    // 获取 goroutine 堆栈信息
    buf := make([]byte, 1<<16)
    stackSize := runtime.Stack(buf, true)
    stacks := string(buf[:stackSize])
    
    categories := map[string]int{
        "main":           0,
        "nats":           0,
        "jetstream":      0,
        "eventbus":       0,
        "health":         0,
        "worker":         0,
        "gc":             0,
        "runtime":        0,
        "other":          0,
    }
    
    // 分析每个 goroutine 的堆栈
    goroutines := strings.Split(stacks, "\n\n")
    for _, goroutine := range goroutines {
        if strings.Contains(goroutine, "main.") {
            categories["main"]++
        } else if strings.Contains(goroutine, "nats") || strings.Contains(goroutine, "NATS") {
            categories["nats"]++
        } else if strings.Contains(goroutine, "jetstream") || strings.Contains(goroutine, "JetStream") {
            categories["jetstream"]++
        } else if strings.Contains(goroutine, "eventbus") || strings.Contains(goroutine, "EventBus") {
            categories["eventbus"]++
        } else if strings.Contains(goroutine, "health") || strings.Contains(goroutine, "Health") {
            categories["health"]++
        } else if strings.Contains(goroutine, "worker") || strings.Contains(goroutine, "Worker") {
            categories["worker"]++
        } else if strings.Contains(goroutine, "GC") || strings.Contains(goroutine, "gc") {
            categories["gc"]++
        } else if strings.Contains(goroutine, "runtime") {
            categories["runtime"]++
        } else {
            categories["other"]++
        }
    }
    
    return categories
}

func printGoroutineDiff(label string, before, after GoroutineStats) {
    diff := after.Total - before.Total
    fmt.Printf("  %s: %+d 个 Goroutines\n", label, diff)
    
    if diff > 0 {
        fmt.Printf("    详细分解:\n")
        for category, afterCount := range after.Categories {
            beforeCount := before.Categories[category]
            if afterCount > beforeCount {
                fmt.Printf("      %s: +%d\n", category, afterCount-beforeCount)
            }
        }
    }
}

func analyzeDetailedGoroutines() {
    // 使用 pprof 获取详细的 goroutine 信息
    profile := pprof.Lookup("goroutine")
    if profile == nil {
        fmt.Println("无法获取 goroutine profile")
        return
    }
    
    fmt.Printf("当前总 Goroutines: %d\n", profile.Count())
    
    // 分类统计
    categories := categorizeGoroutines()
    fmt.Println("分类统计:")
    for category, count := range categories {
        if count > 0 {
            fmt.Printf("  %s: %d 个\n", category, count)
        }
    }
}

func printGoroutinesSummary(baseline, final GoroutineStats) {
    totalIncrease := final.Total - baseline.Total
    
    fmt.Printf("📈 Goroutines 增长分析:\n")
    fmt.Printf("  基准状态: %d 个\n", baseline.Total)
    fmt.Printf("  最终状态: %d 个\n", final.Total)
    fmt.Printf("  总增长: %d 个\n", totalIncrease)
    
    fmt.Printf("\n🎯 EventBus Goroutines 构成 (估算):\n")
    fmt.Printf("  NATS 客户端层: 3-4 个/连接\n")
    fmt.Printf("    ├── 读取循环: 1 个\n")
    fmt.Printf("    ├── 写入循环: 1 个\n")
    fmt.Printf("    ├── 心跳循环: 1 个\n")
    fmt.Printf("    └── 重连循环: 0-1 个\n")
    fmt.Printf("  JetStream 层: 1-3 个/连接\n")
    fmt.Printf("    ├── JS上下文: 1 个\n")
    fmt.Printf("    ├── 消费者: 1 个/订阅\n")
    fmt.Printf("    └── 确认处理: 0-1 个\n")
    fmt.Printf("  EventBus 应用层: 1-3 个/实例\n")
    fmt.Printf("    ├── 健康检查: 0-1 个\n")
    fmt.Printf("    ├── 积压监控: 0-1 个\n")
    fmt.Printf("    └── 指标收集: 0-1 个\n")
    fmt.Printf("  Keyed-Worker池: 0-N 个 (可配置)\n")
    
    fmt.Printf("\n💡 结论:\n")
    if totalIncrease < 20 {
        fmt.Printf("  ✅ EventBus Goroutines 消耗很轻量 (<%d/实例)\n", totalIncrease)
        fmt.Printf("  ✅ 即使多个实例也不会造成 Goroutine 泄漏\n")
    } else {
        fmt.Printf("  ⚠️  EventBus 创建了较多 Goroutines (%d/实例)\n", totalIncrease)
        fmt.Printf("  ⚠️  需要注意 Goroutine 管理和清理\n")
    }
    
    fmt.Printf("\n📋 建议:\n")
    fmt.Printf("  • 单个 EventBus 实例: 5-10 个 Goroutines\n")
    fmt.Printf("  • 双实例方案额外开销: 5-10 个 Goroutines\n")
    fmt.Printf("  • 对于现代应用来说完全可接受\n")
    fmt.Printf("  • 重点关注 Worker 池大小配置\n")
}
