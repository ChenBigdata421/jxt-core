package performance_tests

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// TestHighScenarioInvestigation 调查 High 场景异常
func TestHighScenarioInvestigation(t *testing.T) {
	// 初始化全局 logger
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	t.Log("========================================================================")
	t.Log("🔍 High 场景异常调查 - 多次运行测试")
	t.Log("========================================================================")

	const (
		messageCount = 5000
		topicCount   = 5
		runCount     = 3 // 运行 3 次
	)

	results := make([]TestResult, 0, runCount)

	for i := 1; i <= runCount; i++ {
		t.Logf("\n========================================================================")
		t.Logf("🚀 第 %d 次运行 (共 %d 次)", i, runCount)
		t.Logf("========================================================================")

		result := runHighScenarioTest(t, i, messageCount, topicCount)
		results = append(results, result)

		// 等待一段时间，让系统恢复
		if i < runCount {
			t.Log("⏳ 等待 5 秒，让系统恢复...")
			time.Sleep(5 * time.Second)
		}
	}

	// 分析结果
	t.Log("\n========================================================================")
	t.Log("📊 多次运行结果分析")
	t.Log("========================================================================")

	analyzeResults(t, results)
}

type TestResult struct {
	RunNumber      int
	Throughput     float64
	AvgLatency     float64
	TestDuration   float64
	PeakGoroutines int
	MemoryDeltaMB  float64
	Success        bool
	ErrorMessage   string
}

func runHighScenarioTest(t *testing.T, runNumber int, messageCount int, topicCount int) TestResult {
	result := TestResult{
		RunNumber: runNumber,
		Success:   true,
	}

	// 清理 NATS 数据
	cleanupNATSData(t)

	// 创建 topics
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("test.high.investigation.topic%d", i+1)
	}

	// 创建 NATS EventBus
	cfg := &eventbus.NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: fmt.Sprintf("high-investigation-run%d", runNumber),
		JetStream: eventbus.JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 30 * time.Second,
			AckWait:        30 * time.Second,
			MaxDeliver:     3,
			Stream: eventbus.StreamConfig{
				Name:      "TEST_HIGH_INVESTIGATION",
				Subjects:  []string{"test.high.investigation.>"},
				Storage:   "memory",
				Retention: "limits",
				MaxAge:    24 * time.Hour,
				MaxBytes:  104857600, // 100 MB
				MaxMsgs:   100000,    // 100,000
				Replicas:  1,
				Discard:   "old",
			},
			Consumer: eventbus.NATSConsumerConfig{
				DurableName:   fmt.Sprintf("high-investigation-run%d-consumer", runNumber),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 10000,
				MaxWaiting:    512,
				MaxDeliver:    3,
			},
		},
	}

	bus, err := eventbus.NewNATSEventBus(cfg)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("创建 NATS EventBus 失败: %v", err)
		return result
	}
	defer bus.Close()

	// 记录初始资源
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	initialMemoryMB := getMemoryUsageMB()
	peakGoroutines := initialGoroutines

	// 订阅消息
	receivedCount := 0
	ctx := context.Background()

	for _, topic := range topics {
		err = bus.Subscribe(ctx, topic, func(ctx context.Context, msg []byte) error {
			receivedCount++
			return nil
		})
		if err != nil {
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("订阅失败: %v", err)
			return result
		}
	}

	// 等待订阅就绪
	time.Sleep(2 * time.Second)

	// 发送消息
	t.Logf("📤 开始发送 %d 条消息到 %d 个 topics...", messageCount, topicCount)
	startTime := time.Now()

	for i := 0; i < messageCount; i++ {
		topic := topics[i%topicCount]
		message := []byte(fmt.Sprintf("test-message-%d", i))

		err = bus.Publish(ctx, topic, message)
		if err != nil {
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("发送失败: %v", err)
			return result
		}

		// 监控协程数
		if i%100 == 0 {
			currentGoroutines := runtime.NumGoroutine()
			if currentGoroutines > peakGoroutines {
				peakGoroutines = currentGoroutines
			}
		}
	}

	sendDuration := time.Since(startTime).Seconds()
	t.Logf("✅ 发送完成，耗时: %.2f 秒", sendDuration)

	// 等待接收完成
	t.Log("⏳ 等待接收完成...")
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			result.Success = false
			result.ErrorMessage = fmt.Sprintf("接收超时，已接收 %d/%d", receivedCount, messageCount)
			return result
		case <-ticker.C:
			if receivedCount >= messageCount {
				goto done
			}
		}
	}

done:
	totalDuration := time.Since(startTime).Seconds()
	t.Logf("✅ 接收完成: %d/%d，总耗时: %.2f 秒", receivedCount, messageCount, totalDuration)

	// 计算指标
	result.Throughput = float64(messageCount) / totalDuration
	result.AvgLatency = (totalDuration / float64(messageCount)) * 1000 // ms
	result.TestDuration = totalDuration
	result.PeakGoroutines = peakGoroutines

	// 记录最终资源
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	finalMemoryMB := getMemoryUsageMB()
	result.MemoryDeltaMB = finalMemoryMB - initialMemoryMB

	return result
}

func analyzeResults(t *testing.T, results []TestResult) {
	if len(results) == 0 {
		t.Log("⚠️ 没有测试结果")
		return
	}

	// 统计成功率
	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	t.Logf("\n成功率: %d/%d (%.1f%%)", successCount, len(results), float64(successCount)/float64(len(results))*100)

	// 计算平均值和标准差
	var totalThroughput, totalLatency, totalDuration float64
	var minThroughput, maxThroughput float64 = 1e9, 0
	var minLatency, maxLatency float64 = 1e9, 0

	for _, r := range results {
		if !r.Success {
			t.Logf("\n第 %d 次运行失败: %s", r.RunNumber, r.ErrorMessage)
			continue
		}

		totalThroughput += r.Throughput
		totalLatency += r.AvgLatency
		totalDuration += r.TestDuration

		if r.Throughput < minThroughput {
			minThroughput = r.Throughput
		}
		if r.Throughput > maxThroughput {
			maxThroughput = r.Throughput
		}
		if r.AvgLatency < minLatency {
			minLatency = r.AvgLatency
		}
		if r.AvgLatency > maxLatency {
			maxLatency = r.AvgLatency
		}
	}

	if successCount == 0 {
		t.Log("⚠️ 所有测试都失败了")
		return
	}

	avgThroughput := totalThroughput / float64(successCount)
	avgLatency := totalLatency / float64(successCount)
	avgDuration := totalDuration / float64(successCount)

	// 计算标准差
	var varianceThroughput, varianceLatency float64
	for _, r := range results {
		if !r.Success {
			continue
		}
		varianceThroughput += (r.Throughput - avgThroughput) * (r.Throughput - avgThroughput)
		varianceLatency += (r.AvgLatency - avgLatency) * (r.AvgLatency - avgLatency)
	}
	stdDevThroughput := 0.0
	stdDevLatency := 0.0
	if successCount > 1 {
		stdDevThroughput = varianceThroughput / float64(successCount-1)
		stdDevLatency = varianceLatency / float64(successCount-1)
	}

	// 输出统计结果
	t.Log("\n📊 统计结果:")
	t.Log("----------------------------------------")
	t.Logf("吞吐量 (msg/s):")
	t.Logf("  平均值: %.2f", avgThroughput)
	t.Logf("  最小值: %.2f", minThroughput)
	t.Logf("  最大值: %.2f", maxThroughput)
	t.Logf("  标准差: %.2f", stdDevThroughput)
	t.Logf("  变异系数: %.2f%%", (stdDevThroughput/avgThroughput)*100)

	t.Log("\n延迟 (ms):")
	t.Logf("  平均值: %.2f", avgLatency)
	t.Logf("  最小值: %.2f", minLatency)
	t.Logf("  最大值: %.2f", maxLatency)
	t.Logf("  标准差: %.2f", stdDevLatency)
	t.Logf("  变异系数: %.2f%%", (stdDevLatency/avgLatency)*100)

	t.Log("\n测试时长 (s):")
	t.Logf("  平均值: %.2f", avgDuration)

	// 详细结果
	t.Log("\n📋 详细结果:")
	t.Log("----------------------------------------")
	t.Log("运行次数 | 吞吐量(msg/s) | 延迟(ms) | 时长(s) | 峰值协程 | 内存增量(MB)")
	t.Log("--------|--------------|---------|---------|---------|------------")
	for _, r := range results {
		if r.Success {
			t.Logf("   %d    | %10.2f   | %7.2f | %7.2f | %7d | %10.2f",
				r.RunNumber, r.Throughput, r.AvgLatency, r.TestDuration,
				r.PeakGoroutines, r.MemoryDeltaMB)
		} else {
			t.Logf("   %d    | 失败: %s", r.RunNumber, r.ErrorMessage)
		}
	}

	// 稳定性分析
	t.Log("\n🎯 稳定性分析:")
	t.Log("----------------------------------------")
	throughputCV := (stdDevThroughput / avgThroughput) * 100
	latencyCV := (stdDevLatency / avgLatency) * 100

	if throughputCV < 10 {
		t.Logf("✅ 吞吐量稳定 (变异系数: %.2f%% < 10%%)", throughputCV)
	} else if throughputCV < 20 {
		t.Logf("⚠️ 吞吐量较稳定 (变异系数: %.2f%% < 20%%)", throughputCV)
	} else {
		t.Logf("❌ 吞吐量不稳定 (变异系数: %.2f%% >= 20%%)", throughputCV)
	}

	if latencyCV < 10 {
		t.Logf("✅ 延迟稳定 (变异系数: %.2f%% < 10%%)", latencyCV)
	} else if latencyCV < 20 {
		t.Logf("⚠️ 延迟较稳定 (变异系数: %.2f%% < 20%%)", latencyCV)
	} else {
		t.Logf("❌ 延迟不稳定 (变异系数: %.2f%% >= 20%%)", latencyCV)
	}
}

func cleanupNATSData(t *testing.T) {
	t.Log("🧹 清理 NATS 测试数据...")
	// 这里可以添加清理逻辑
}
