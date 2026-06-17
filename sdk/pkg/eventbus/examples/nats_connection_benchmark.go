//go:build ignore

package main

import (
    "context"
    "fmt"
    "log"
    "runtime"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// NATS连接资源消耗测试
func main() {
    fmt.Println("=== NATS 连接资源消耗测试 ===\n")

    // 1. 基准测试：无连接状态
    baseline := getMemStats()
    fmt.Printf("📊 基准状态 (无连接):\n")
    printMemStats("基准", baseline)

    // 2. 测试单连接消耗
    fmt.Printf("\n🔗 创建单个 NATS 连接...\n")
    singleBus := createNATSEventBus("single-connection")
    defer singleBus.Close()
    
    time.Sleep(100 * time.Millisecond) // 等待连接建立
    runtime.GC() // 强制垃圾回收，获得准确数据
    
    singleConnStats := getMemStats()
    printMemStats("单连接", singleConnStats)
    printMemDiff("单连接增量", baseline, singleConnStats)

    // 3. 测试双连接消耗
    fmt.Printf("\n🔗🔗 创建第二个 NATS 连接...\n")
    dualBus := createNATSEventBus("dual-connection")
    defer dualBus.Close()
    
    time.Sleep(100 * time.Millisecond)
    runtime.GC()
    
    dualConnStats := getMemStats()
    printMemStats("双连接", dualConnStats)
    printMemDiff("双连接增量", baseline, dualConnStats)
    printMemDiff("第二连接增量", singleConnStats, dualConnStats)

    // 4. 测试多连接场景
    fmt.Printf("\n🔗🔗🔗🔗🔗 创建多个连接 (模拟微服务场景)...\n")
    var buses []eventbus.EventBus
    
    for i := 0; i < 5; i++ {
        bus := createNATSEventBus(fmt.Sprintf("service-%d", i))
        buses = append(buses, bus)
        time.Sleep(20 * time.Millisecond)
    }
    
    runtime.GC()
    multiConnStats := getMemStats()
    printMemStats("7连接总计", multiConnStats)
    printMemDiff("7连接增量", baseline, multiConnStats)
    
    // 计算平均每连接消耗
    avgPerConn := (multiConnStats.Alloc - baseline.Alloc) / 7
    fmt.Printf("📈 平均每连接内存消耗: %s\n", formatBytes(avgPerConn))

    // 5. 清理资源
    for _, bus := range buses {
        bus.Close()
    }

    // 6. 连接数量对性能的影响测试
    fmt.Printf("\n⚡ 性能影响测试...\n")
    testConnectionPerformance()

    // 7. 总结和建议
    fmt.Printf("\n=== 总结 ===\n")
    printConnectionAnalysis(avgPerConn)
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
                    Storage:  "memory", // 使用内存存储减少磁盘影响
                },
            },
        },
    }

    bus, err := eventbus.NewEventBus(config)
    if err != nil {
        log.Printf("Failed to create EventBus for %s: %v", clientID, err)
        return nil
    }
    return bus
}

type MemStats struct {
    Alloc      uint64 // 当前分配的内存
    TotalAlloc uint64 // 累计分配的内存
    Sys        uint64 // 系统内存
    NumGC      uint32 // GC次数
    Goroutines int    // Goroutine数量
}

func getMemStats() MemStats {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    
    return MemStats{
        Alloc:      m.Alloc,
        TotalAlloc: m.TotalAlloc,
        Sys:        m.Sys,
        NumGC:      m.NumGC,
        Goroutines: runtime.NumGoroutine(),
    }
}

func printMemStats(label string, stats MemStats) {
    fmt.Printf("  %s:\n", label)
    fmt.Printf("    内存分配: %s\n", formatBytes(stats.Alloc))
    fmt.Printf("    系统内存: %s\n", formatBytes(stats.Sys))
    fmt.Printf("    Goroutines: %d\n", stats.Goroutines)
    fmt.Printf("    GC次数: %d\n", stats.NumGC)
}

func printMemDiff(label string, before, after MemStats) {
    allocDiff := int64(after.Alloc) - int64(before.Alloc)
    sysDiff := int64(after.Sys) - int64(before.Sys)
    goroutineDiff := after.Goroutines - before.Goroutines
    
    fmt.Printf("  %s:\n", label)
    fmt.Printf("    内存增量: %s\n", formatBytesDiff(allocDiff))
    fmt.Printf("    系统内存增量: %s\n", formatBytesDiff(sysDiff))
    fmt.Printf("    Goroutine增量: %+d\n", goroutineDiff)
}

func formatBytes(bytes uint64) string {
    if bytes < 1024 {
        return fmt.Sprintf("%d B", bytes)
    } else if bytes < 1024*1024 {
        return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
    } else {
        return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
    }
}

func formatBytesDiff(bytes int64) string {
    sign := ""
    if bytes > 0 {
        sign = "+"
    }
    
    absBytes := uint64(bytes)
    if bytes < 0 {
        absBytes = uint64(-bytes)
        sign = "-"
    }
    
    return sign + formatBytes(absBytes)
}

func testConnectionPerformance() {
    // 测试单连接性能
    singleBus := createNATSEventBus("perf-single")
    defer singleBus.Close()
    
    ctx := context.Background()
    message := []byte("performance test message")
    
    // 预热
    for i := 0; i < 100; i++ {
        singleBus.Publish(ctx, "perf-single.test", message)
    }
    
    // 测试单连接
    start := time.Now()
    for i := 0; i < 1000; i++ {
        singleBus.Publish(ctx, "perf-single.test", message)
    }
    singleDuration := time.Since(start)
    
    // 测试双连接
    dualBus := createNATSEventBus("perf-dual")
    defer dualBus.Close()
    
    start = time.Now()
    for i := 0; i < 500; i++ {
        singleBus.Publish(ctx, "perf-single.test", message)
        dualBus.Publish(ctx, "perf-dual.test", message)
    }
    dualDuration := time.Since(start)
    
    fmt.Printf("  单连接1000条消息: %v (%.2f msg/s)\n", 
        singleDuration, 1000.0/singleDuration.Seconds())
    fmt.Printf("  双连接1000条消息: %v (%.2f msg/s)\n", 
        dualDuration, 1000.0/dualDuration.Seconds())
    
    overhead := (dualDuration.Seconds() - singleDuration.Seconds()) / singleDuration.Seconds() * 100
    fmt.Printf("  双连接性能开销: %.2f%%\n", overhead)
}

func printConnectionAnalysis(avgPerConn uint64) {
    fmt.Printf("📋 NATS连接资源分析:\n\n")
    
    fmt.Printf("💾 内存消耗:\n")
    fmt.Printf("  • 每连接平均: %s\n", formatBytes(avgPerConn))
    fmt.Printf("  • 100个连接: %s\n", formatBytes(avgPerConn*100))
    fmt.Printf("  • 1000个连接: %s\n", formatBytes(avgPerConn*1000))
    
    fmt.Printf("\n🎯 结论:\n")
    if avgPerConn < 1024*1024 { // < 1MB
        fmt.Printf("  ✅ NATS连接资源消耗很轻量 (<%s/连接)\n", formatBytes(1024*1024))
        fmt.Printf("  ✅ 即使100个连接也只需要 %s 内存\n", formatBytes(avgPerConn*100))
        fmt.Printf("  ✅ 连接数量不是性能瓶颈\n")
    } else {
        fmt.Printf("  ⚠️  NATS连接有一定资源消耗 (>1MB/连接)\n")
        fmt.Printf("  ⚠️  大量连接时需要考虑资源限制\n")
    }
    
    fmt.Printf("\n💡 建议:\n")
    fmt.Printf("  • 微服务场景: 每服务1-2个连接完全可接受\n")
    fmt.Printf("  • 单体应用: 可以考虑连接复用\n")
    fmt.Printf("  • 高并发场景: 连接池比智能路由更高效\n")
}
