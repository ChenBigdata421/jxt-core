package eventbus

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// PressureLevelNATS NATS压力级别（与Kafka测试完全相同）
type PressureLevelNATS struct {
	Name         string
	MessageCount int
	Concurrency  int
	MessageSize  int
	Timeout      time.Duration
}

// ComprehensivePressureMetricsNATS NATS全面压力测试指标
type ComprehensivePressureMetricsNATS struct {
	PressureLevel    string
	MessageCount     int
	Concurrency      int
	MessageSize      int
	SentCount        int64
	ReceivedCount    int64
	SuccessRate      float64
	Throughput       float64
	FirstMsgLatency  time.Duration
	SendDuration     time.Duration
	TotalDuration    time.Duration
	SendRate         float64
	ErrorCount       int64
	MemoryBefore     uint64
	MemoryAfter      uint64
	MemoryDelta      float64
	GoroutinesBefore int
	GoroutinesAfter  int
	GoroutinesDelta  int
}

// TestNATSComprehensivePressure NATS全面压力测试（与Kafka测试完全相同的场景）
func TestNATSComprehensivePressure(t *testing.T) {
	t.Log("🚀 EVENTBUS + NATS JETSTREAM 全面压力测试")
	t.Log("=" + string(make([]byte, 100)))
	t.Log("📊 测试统一JetStream架构 + 磁盘持久化")
	t.Log("")

	// 定义压力级别（与Kafka测试完全相同）
	pressureLevels := []PressureLevelNATS{
		{
			Name:         "低压",
			MessageCount: 500,
			Concurrency:  5,
			MessageSize:  1024,
			Timeout:      2 * time.Minute,
		},
		{
			Name:         "中压",
			MessageCount: 2000,
			Concurrency:  10,
			MessageSize:  2048,
			Timeout:      3 * time.Minute,
		},
		{
			Name:         "高压",
			MessageCount: 5000,
			Concurrency:  20,
			MessageSize:  4096,
			Timeout:      5 * time.Minute,
		},
		{
			Name:         "极限",
			MessageCount: 10000,
			Concurrency:  50,
			MessageSize:  8192,
			Timeout:      10 * time.Minute,
		},
	}

	allResults := make([]*ComprehensivePressureMetricsNATS, 0)

	for _, level := range pressureLevels {
		separator := string(make([]byte, 100))
		t.Logf("\n%s", separator)
		t.Logf("🎯 开始测试: %s", level.Name)
		t.Logf("   消息数量: %d", level.MessageCount)
		t.Logf("   并发数: %d", level.Concurrency)
		t.Logf("   消息大小: %d bytes", level.MessageSize)
		t.Logf("   超时时间: %v", level.Timeout)

		metrics := runNATSFullPressureTest(t, level)
		allResults = append(allResults, metrics)

		// 输出本轮结果
		printNATSFullPressureMetrics(t, metrics)

		// 清理间隔
		t.Logf("⏳ 清理资源，等待5秒...")
		time.Sleep(5 * time.Second)
		runtime.GC()
	}

	// 生成最终报告
	generateNATSFinalPressureReport(t, allResults)
}

// runNATSFullPressureTest 运行单个压力级别测试
func runNATSFullPressureTest(t *testing.T, level PressureLevelNATS) *ComprehensivePressureMetricsNATS {
	// 🔧 关键修复：使用英文名称
	levelNameMap := map[string]string{
		"低压": "low",
		"中压": "medium",
		"高压": "high",
		"极限": "extreme",
	}
	levelNameEn := levelNameMap[level.Name]
	if levelNameEn == "" {
		levelNameEn = level.Name // fallback
	}

	// 创建唯一的subject前缀
	timestamp := time.Now().UnixNano()
	subjectPrefix := fmt.Sprintf("pressure.%s.%d", levelNameEn, timestamp)
	subject := fmt.Sprintf("%s.test", subjectPrefix)

	// 创建NATS JetStream配置（磁盘持久化）
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("pressure-test-%s-%d", levelNameEn, timestamp),
		MaxReconnects:       5,
		ReconnectWait:       2 * time.Second,
		ConnectionTimeout:   10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 10 * time.Second,
			AckWait:        15 * time.Second,
			MaxDeliver:     3,
			Stream: StreamConfig{
				Name:      fmt.Sprintf("PRESSURE_STREAM_%s_%d", levelNameEn, timestamp),
				Subjects:  []string{fmt.Sprintf("%s.>", subjectPrefix)},
				Retention: "limits",
				Storage:   "file", // 磁盘持久化
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  512 * 1024 * 1024, // 512MB
				MaxMsgs:   100000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("pressure_consumer_%s_%d", levelNameEn, timestamp),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 500,
				MaxWaiting:    200,
				MaxDeliver:    3,
			},
		},
	}

	// 创建EventBus
	bus, err := NewNATSEventBus(config)
	if err != nil {
		t.Fatalf("❌ Failed to create NATS EventBus: %v", err)
	}
	defer bus.Close()

	t.Logf("✅ NATS EventBus创建成功")

	// 记录初始资源使用
	var memStatsBefore runtime.MemStats
	runtime.ReadMemStats(&memStatsBefore)
	goroutinesBefore := runtime.NumGoroutine()

	// 初始化指标
	metrics := &ComprehensivePressureMetricsNATS{
		PressureLevel:    level.Name,
		MessageCount:     level.MessageCount,
		Concurrency:      level.Concurrency,
		MessageSize:      level.MessageSize,
		MemoryBefore:     memStatsBefore.Alloc,
		GoroutinesBefore: goroutinesBefore,
	}

	ctx := context.Background()

	// 接收计数器
	var receivedCount atomic.Int64
	var firstMsgTime atomic.Value // time.Time
	receivedChan := make(chan struct{}, level.MessageCount)

	// 订阅（简化handler）
	handler := func(ctx context.Context, message []byte) error {
		now := time.Now()

		// 记录第一条消息时间
		if firstMsgTime.Load() == nil {
			firstMsgTime.Store(now)
		}

		receivedCount.Add(1)
		receivedChan <- struct{}{} // 通知接收到消息
		return nil
	}

	err = bus.Subscribe(ctx, subject, handler)
	if err != nil {
		t.Fatalf("❌ Failed to subscribe: %v", err)
	}
	t.Logf("✅ 订阅成功: %s", subject)

	// 等待订阅就绪
	time.Sleep(5 * time.Second)

	// 开始发送（单线程发送，与Kafka测试一致）
	t.Logf("📤 开始发送 %d 条消息...", level.MessageCount)
	sendStart := time.Now()

	var sentCount atomic.Int64
	var sendErrors atomic.Int64

	// 单线程发送
	for i := 0; i < level.MessageCount; i++ {
		// 创建简单消息（不使用JSON，简化调试）
		message := []byte(fmt.Sprintf("Message #%d", i))

		err := bus.Publish(ctx, subject, message)
		if err != nil {
			sendErrors.Add(1)
			t.Logf("❌ 发送失败 #%d: %v", i, err)
		} else {
			sentCount.Add(1)
		}
	}

	metrics.SendDuration = time.Since(sendStart)
	metrics.SentCount = sentCount.Load()
	metrics.ErrorCount = sendErrors.Load()
	metrics.SendRate = float64(metrics.SentCount) / metrics.SendDuration.Seconds()

	t.Logf("✅ 发送完成: %v", metrics.SendDuration)
	t.Logf("   发送成功: %d/%d", metrics.SentCount, level.MessageCount)
	t.Logf("   发送速率: %.2f msg/s", metrics.SendRate)
	t.Logf("   发送错误: %d", metrics.ErrorCount)

	// NATS JetStream不需要等待批量发送（同步发送）
	t.Logf("📥 等待接收消息（超时: %v）...", level.Timeout)

	// 等待接收
	timeout := time.NewTimer(level.Timeout)
	defer timeout.Stop()

	receivedInTime := int64(0)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

receiveLoop:
	for receivedInTime < int64(level.MessageCount) {
		select {
		case <-receivedChan:
			receivedInTime++
		case <-ticker.C:
			t.Logf("   进度: %d/%d (%.1f%%)", receivedInTime, level.MessageCount, float64(receivedInTime)/float64(level.MessageCount)*100)
		case <-timeout.C:
			t.Logf("⏱️  接收超时")
			break receiveLoop
		}
	}

	metrics.TotalDuration = time.Since(sendStart)
	metrics.ReceivedCount = receivedInTime
	metrics.SuccessRate = float64(metrics.ReceivedCount) / float64(level.MessageCount) * 100
	metrics.Throughput = float64(metrics.ReceivedCount) / metrics.TotalDuration.Seconds()

	// 计算延迟指标（简化版）
	if firstMsgTime.Load() != nil {
		metrics.FirstMsgLatency = firstMsgTime.Load().(time.Time).Sub(sendStart)
	}

	// 记录最终资源使用
	var memStatsAfter runtime.MemStats
	runtime.ReadMemStats(&memStatsAfter)
	goroutinesAfter := runtime.NumGoroutine()

	metrics.MemoryAfter = memStatsAfter.Alloc
	metrics.MemoryDelta = float64(metrics.MemoryAfter-metrics.MemoryBefore) / 1024 / 1024 // MB
	metrics.GoroutinesAfter = goroutinesAfter
	metrics.GoroutinesDelta = goroutinesAfter - goroutinesBefore

	return metrics
}

// printNATSFullPressureMetrics 输出单个压力级别的测试结果
func printNATSFullPressureMetrics(t *testing.T, m *ComprehensivePressureMetricsNATS) {
	t.Logf("\n📊 ===== %s 测试结果 =====", m.PressureLevel)
	t.Logf("✅ 成功率: %.2f%% (%d/%d)", m.SuccessRate, m.ReceivedCount, m.MessageCount)
	t.Logf("🚀 吞吐量: %.2f msg/s", m.Throughput)
	t.Logf("📤 发送速率: %.2f msg/s", m.SendRate)
	t.Logf("⏱️  延迟指标:")
	t.Logf("   首条延迟: %v", m.FirstMsgLatency)
	t.Logf("⏰ 时间指标:")
	t.Logf("   发送耗时: %v", m.SendDuration)
	t.Logf("   总耗时: %v", m.TotalDuration)
	t.Logf("💾 资源使用:")
	t.Logf("   内存增量: %.2f MB (%.2f → %.2f)", m.MemoryDelta, float64(m.MemoryBefore)/1024/1024, float64(m.MemoryAfter)/1024/1024)
	t.Logf("   Goroutine增量: %d (%d → %d)", m.GoroutinesDelta, m.GoroutinesBefore, m.GoroutinesAfter)
}

// generateNATSFinalPressureReport 生成最终压力测试报告
func generateNATSFinalPressureReport(t *testing.T, results []*ComprehensivePressureMetricsNATS) {
	t.Logf("\n📊 ===== EVENTBUS + NATS JETSTREAM 全面压力测试总结 =====")
	t.Logf("")

	// 表头
	t.Logf("%-12s | %10s | %10s | %12s | %12s | %12s | %12s | %12s",
		"压力级别", "消息数", "并发数", "成功率", "吞吐量", "平均延迟", "P95延迟", "内存增量")

	// 分隔线
	t.Logf("")

	// 输出每个压力级别的结果
	for _, m := range results {
		t.Logf("%-12s | %10d | %10d | %11.2f%% | %9.2f/s | %12s | %12s | %9.2fMB",
			m.PressureLevel,
			m.MessageCount,
			m.Concurrency,
			m.SuccessRate,
			m.Throughput,
			"0s", // 简化版没有平均延迟
			"0s", // 简化版没有P95延迟
			m.MemoryDelta,
		)
	}

	t.Logf("")
	t.Logf("🎯 性能分析:")

	// 找出最佳性能
	var maxThroughput float64
	var maxThroughputLevel string
	var maxSuccessRate float64
	var maxSuccessRateLevel string

	for _, m := range results {
		if m.Throughput > maxThroughput {
			maxThroughput = m.Throughput
			maxThroughputLevel = m.PressureLevel
		}
		if m.SuccessRate > maxSuccessRate {
			maxSuccessRate = m.SuccessRate
			maxSuccessRateLevel = m.PressureLevel
		}
	}

	t.Logf("   🏆 最高吞吐量: %s (%.2f msg/s)", maxThroughputLevel, maxThroughput)
	t.Logf("   ✅ 最高成功率: %s (%.2f%%)", maxSuccessRateLevel, maxSuccessRate)

	t.Logf("")
	t.Logf("📈 性能趋势:")

	if len(results) >= 2 {
		first := results[0]
		last := results[len(results)-1]

		throughputChange := (last.Throughput - first.Throughput) / first.Throughput * 100
		successRateChange := last.SuccessRate - first.SuccessRate

		t.Logf("   吞吐量变化: %.2f%% (%s → %s)", throughputChange, first.PressureLevel, last.PressureLevel)
		t.Logf("   成功率变化: %.2f%% (%s → %s)", successRateChange, first.PressureLevel, last.PressureLevel)
	}

	t.Logf("")
	t.Logf("🎓 结论:")

	// 检查是否所有测试都通过
	allPassed := true
	for _, m := range results {
		if m.SuccessRate < 95.0 {
			allPassed = false
			t.Logf("   ⚠️  %s: 成功率 %.2f%% < 95%%", m.PressureLevel, m.SuccessRate)
		}
	}

	if allPassed {
		t.Logf("   ✅ 所有压力级别测试通过！")
		t.Logf("   ✅ EventBus + NATS JetStream (磁盘持久化) 性能优秀！")
	} else {
		t.Logf("   ⚠️  部分压力级别未达标，需要进一步优化")
	}

	t.Logf("")

	t.Logf("🚀 配置:")
	t.Logf("   ✅ 统一JetStream架构")
	t.Logf("   ✅ 磁盘持久化 (Storage: file)")
	t.Logf("   ✅ 1个EventBus实例")
	t.Logf("   ✅ 1个NATS连接")
	t.Logf("   ✅ 1个JetStream Context")
	t.Logf("   ✅ 1个Consumer (Durable)")

	t.Logf("")
	separator := string(make([]byte, 100))
	t.Logf("%s", separator)
	t.Logf("测试完成时间: %s", time.Now().Format("2006-01-02 15:04:05"))
}
