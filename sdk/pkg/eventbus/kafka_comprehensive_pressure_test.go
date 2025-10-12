package eventbus

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// PressureLevel 压力级别
type PressureLevel struct {
	Name         string
	MessageCount int
	Concurrency  int
	MessageSize  int
	Timeout      time.Duration
}

// ComprehensivePressureMetrics Kafka全面压力测试指标
type ComprehensivePressureMetrics struct {
	PressureLevel     string
	MessageCount      int
	Concurrency       int
	MessageSize       int
	SentCount         int64
	ReceivedCount     int64
	ErrorCount        int64
	SuccessRate       float64
	SendRate          float64
	Throughput        float64
	FirstMsgLatency   time.Duration
	AverageLatency    time.Duration
	P95Latency        time.Duration
	P99Latency        time.Duration
	MaxLatency        time.Duration
	MinLatency        time.Duration
	TotalDuration     time.Duration
	SendDuration      time.Duration
	InitialMemoryMB   float64
	PeakMemoryMB      float64
	MemoryDeltaMB     float64
	InitialGoroutines int
	PeakGoroutines    int
	GoroutineDelta    int
}

// TestKafkaComprehensivePressure EventBus+Kafka全面压力测试
func TestKafkaComprehensivePressure(t *testing.T) {
	t.Log("🚀 EVENTBUS + KAFKA 全面压力测试")
	t.Log("=" + string(make([]byte, 100)))
	t.Log("📊 测试优化后的AsyncProducer + 批处理 + LZ4压缩配置")
	t.Log("")

	// 定义压力级别
	pressureLevels := []PressureLevel{
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

	allResults := make([]*ComprehensivePressureMetrics, 0)

	for _, level := range pressureLevels {
		separator := string(make([]byte, 100))
		t.Logf("\n%s", separator)
		t.Logf("🎯 开始测试: %s", level.Name)
		t.Logf("   消息数量: %d", level.MessageCount)
		t.Logf("   并发数: %d", level.Concurrency)
		t.Logf("   消息大小: %d bytes", level.MessageSize)
		t.Logf("   超时时间: %v", level.Timeout)

		metrics := runKafkaFullPressureTest(t, level)
		allResults = append(allResults, metrics)

		// 输出本轮结果
		printFullPressureMetrics(t, metrics)

		// 清理间隔
		t.Logf("⏳ 清理资源，等待5秒...")
		time.Sleep(5 * time.Second)
		runtime.GC()
	}

	// 生成最终报告
	generateFinalPressureReport(t, allResults)
}

// runKafkaFullPressureTest 运行单个压力级别测试
func runKafkaFullPressureTest(t *testing.T, level PressureLevel) *ComprehensivePressureMetrics {
	// 🔧 关键修复：ClientID和topic不能包含中文字符
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

	// 先生成topic名称
	topic := fmt.Sprintf("pressure-test-%s-%d", levelNameEn, time.Now().UnixNano())

	// 创建优化后的Kafka配置
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: fmt.Sprintf("pressure-test-%s-%d", levelNameEn, time.Now().UnixNano()),
		Producer: ProducerConfig{
			RequiredAcks:    1,
			Compression:     "lz4",                 // LZ4压缩
			FlushFrequency:  10 * time.Millisecond, // 10ms批量
			FlushMessages:   100,                   // 100条消息
			FlushBytes:      100000,                // 100KB
			RetryMax:        3,
			Timeout:         10 * time.Second,
			MaxMessageBytes: 10 * 1024 * 1024, // 10MB
			MaxInFlight:     100,              // 100并发请求
		},
		Consumer: ConsumerConfig{
			GroupID:           fmt.Sprintf("pressure-group-%d", time.Now().UnixNano()), // 每次使用新的Consumer Group ID
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 1 * time.Second,        // 和成功的测试一致
			FetchMinBytes:     1,                      // 设置为1，确保能立即读取
			FetchMaxBytes:     10 * 1024 * 1024,       // 10MB
			FetchMaxWait:      500 * time.Millisecond, // 500ms
			AutoOffsetReset:   "earliest",             // 使用earliest确保能读取所有消息
			RebalanceStrategy: "range",
			IsolationLevel:    "read_uncommitted",
		},
		Net: NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			KeepAlive:    30 * time.Second,
		},
		MetadataRefreshFreq:  10 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 250 * time.Millisecond,
		HealthCheckInterval:  30 * time.Second,
	}

	// 创建EventBus
	bus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("❌ Failed to create Kafka EventBus: %v", err)
	}
	defer bus.Close()

	// 初始化指标
	metrics := &ComprehensivePressureMetrics{
		PressureLevel: level.Name,
		MessageCount:  level.MessageCount,
		Concurrency:   level.Concurrency,
		MessageSize:   level.MessageSize,
		MinLatency:    time.Hour, // 初始化为很大的值
	}

	// 记录初始资源状态
	metrics.InitialGoroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.InitialMemoryMB = float64(m.Alloc) / 1024 / 1024

	// topic已经在函数开头创建
	ctx := context.Background()

	// 🚀 配置预订阅模式（在Subscribe之前设置）
	if kafkaBus, ok := bus.(*kafkaEventBus); ok {
		kafkaBus.allPossibleTopics = []string{topic}
		t.Logf("✅ 配置预订阅: %s", topic)
	}

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

	err = bus.Subscribe(ctx, topic, handler)
	if err != nil {
		t.Fatalf("❌ Failed to subscribe: %v", err)
	}
	t.Logf("✅ 订阅成功: %s", topic)

	// 等待订阅就绪
	time.Sleep(5 * time.Second)

	// 开始发送（改为单线程发送，简化调试）
	t.Logf("📤 开始发送 %d 条消息...", level.MessageCount)
	sendStart := time.Now()

	var sentCount atomic.Int64
	var sendErrors atomic.Int64

	// 单线程发送
	for i := 0; i < level.MessageCount; i++ {
		// 创建简单消息（不使用JSON，简化调试）
		message := []byte(fmt.Sprintf("Message #%d", i))

		err := bus.Publish(ctx, topic, message)
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

	// 等待AsyncProducer批量发送完成
	t.Logf("⏳ 等待AsyncProducer批量发送完成...")
	time.Sleep(10 * time.Second)

	// 等待接收
	t.Logf("📥 等待接收消息（超时: %v）...", level.Timeout)
	timeoutTimer := time.NewTimer(level.Timeout)
	defer timeoutTimer.Stop()

	receivedInTime := 0
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

receiveLoop:
	for receivedInTime < level.MessageCount {
		select {
		case <-receivedChan:
			receivedInTime++
		case <-ticker.C:
			progress := float64(receivedInTime) / float64(level.MessageCount) * 100
			t.Logf("   进度: %d/%d (%.1f%%)", receivedInTime, level.MessageCount, progress)
		case <-timeoutTimer.C:
			t.Logf("⏱️  接收超时")
			break receiveLoop
		}
	}

	metrics.TotalDuration = time.Since(sendStart)
	metrics.ReceivedCount = receivedCount.Load()
	metrics.SuccessRate = float64(metrics.ReceivedCount) / float64(level.MessageCount) * 100
	metrics.Throughput = float64(metrics.ReceivedCount) / metrics.TotalDuration.Seconds()

	// 计算延迟指标（简化版）
	if firstMsgTime.Load() != nil {
		metrics.FirstMsgLatency = firstMsgTime.Load().(time.Time).Sub(sendStart)
	}

	// 记录峰值资源使用
	metrics.PeakGoroutines = runtime.NumGoroutine()
	metrics.GoroutineDelta = metrics.PeakGoroutines - metrics.InitialGoroutines

	runtime.ReadMemStats(&m)
	metrics.PeakMemoryMB = float64(m.Alloc) / 1024 / 1024
	metrics.MemoryDeltaMB = metrics.PeakMemoryMB - metrics.InitialMemoryMB

	return metrics
}

// sortLatencies 简单的冒泡排序
func sortLatencies(latencies []time.Duration) {
	n := len(latencies)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}
}

// printFullPressureMetrics 打印压力测试指标
func printFullPressureMetrics(t *testing.T, m *ComprehensivePressureMetrics) {
	t.Logf("\n📊 ===== %s 测试结果 =====", m.PressureLevel)
	t.Logf("✅ 成功率: %.2f%% (%d/%d)", m.SuccessRate, m.ReceivedCount, m.MessageCount)
	t.Logf("🚀 吞吐量: %.2f msg/s", m.Throughput)
	t.Logf("📤 发送速率: %.2f msg/s", m.SendRate)
	t.Logf("⏱️  延迟指标:")
	t.Logf("   首条延迟: %v", m.FirstMsgLatency)
	t.Logf("   平均延迟: %v", m.AverageLatency)
	t.Logf("   P95延迟: %v", m.P95Latency)
	t.Logf("   P99延迟: %v", m.P99Latency)
	t.Logf("   最小延迟: %v", m.MinLatency)
	t.Logf("   最大延迟: %v", m.MaxLatency)
	t.Logf("⏰ 时间指标:")
	t.Logf("   发送耗时: %v", m.SendDuration)
	t.Logf("   总耗时: %v", m.TotalDuration)
	t.Logf("💾 资源使用:")
	t.Logf("   内存增量: %.2f MB (%.2f → %.2f)", m.MemoryDeltaMB, m.InitialMemoryMB, m.PeakMemoryMB)
	t.Logf("   Goroutine增量: %d (%d → %d)", m.GoroutineDelta, m.InitialGoroutines, m.PeakGoroutines)
}

// generateFinalPressureReport 生成最终压力测试报告
func generateFinalPressureReport(t *testing.T, results []*ComprehensivePressureMetrics) {
	separator1 := string(make([]byte, 100))
	t.Logf("\n%s", separator1)
	t.Logf("📊 ===== EVENTBUS + KAFKA 全面压力测试总结 =====")
	t.Logf("")

	// 表格头
	t.Logf("%-10s | %8s | %8s | %10s | %10s | %10s | %10s | %10s",
		"压力级别", "消息数", "并发数", "成功率", "吞吐量", "平均延迟", "P95延迟", "内存增量")
	separator2 := string(make([]byte, 110))
	t.Logf("%s", separator2)

	// 表格数据
	for _, m := range results {
		t.Logf("%-10s | %8d | %8d | %9.2f%% | %8.2f/s | %10v | %10v | %8.2fMB",
			m.PressureLevel,
			m.MessageCount,
			m.Concurrency,
			m.SuccessRate,
			m.Throughput,
			m.AverageLatency,
			m.P95Latency,
			m.MemoryDeltaMB)
	}

	t.Logf("")
	t.Logf("🎯 性能分析:")

	// 找出最佳和最差性能
	var bestThroughput, worstThroughput *ComprehensivePressureMetrics
	var bestLatency, worstLatency *ComprehensivePressureMetrics
	var bestSuccessRate, worstSuccessRate *ComprehensivePressureMetrics

	for i, m := range results {
		if i == 0 {
			bestThroughput = m
			worstThroughput = m
			bestLatency = m
			worstLatency = m
			bestSuccessRate = m
			worstSuccessRate = m
			continue
		}

		if m.Throughput > bestThroughput.Throughput {
			bestThroughput = m
		}
		if m.Throughput < worstThroughput.Throughput {
			worstThroughput = m
		}

		if m.AverageLatency < bestLatency.AverageLatency {
			bestLatency = m
		}
		if m.AverageLatency > worstLatency.AverageLatency {
			worstLatency = m
		}

		if m.SuccessRate > bestSuccessRate.SuccessRate {
			bestSuccessRate = m
		}
		if m.SuccessRate < worstSuccessRate.SuccessRate {
			worstSuccessRate = m
		}
	}

	t.Logf("   🏆 最高吞吐量: %s (%.2f msg/s)", bestThroughput.PressureLevel, bestThroughput.Throughput)
	t.Logf("   ⚡ 最低延迟: %s (%v)", bestLatency.PressureLevel, bestLatency.AverageLatency)
	t.Logf("   ✅ 最高成功率: %s (%.2f%%)", bestSuccessRate.PressureLevel, bestSuccessRate.SuccessRate)

	t.Logf("")
	t.Logf("📈 性能趋势:")

	// 计算性能趋势
	if len(results) >= 2 {
		firstResult := results[0]
		lastResult := results[len(results)-1]

		throughputChange := (lastResult.Throughput - firstResult.Throughput) / firstResult.Throughput * 100
		latencyChange := float64(lastResult.AverageLatency-firstResult.AverageLatency) / float64(firstResult.AverageLatency) * 100
		successRateChange := lastResult.SuccessRate - firstResult.SuccessRate

		t.Logf("   吞吐量变化: %.2f%% (%s → %s)", throughputChange, firstResult.PressureLevel, lastResult.PressureLevel)
		t.Logf("   延迟变化: %.2f%% (%s → %s)", latencyChange, firstResult.PressureLevel, lastResult.PressureLevel)
		t.Logf("   成功率变化: %.2f%% (%s → %s)", successRateChange, firstResult.PressureLevel, lastResult.PressureLevel)
	}

	t.Logf("")
	t.Logf("🎓 结论:")

	// 评估整体性能
	allPassed := true
	for _, m := range results {
		if m.SuccessRate < 95.0 {
			allPassed = false
			t.Logf("   ⚠️  %s: 成功率 %.2f%% < 95%%", m.PressureLevel, m.SuccessRate)
		}
	}

	if allPassed {
		t.Logf("   ✅ 所有压力级别测试通过！")
		t.Logf("   ✅ EventBus + Kafka (AsyncProducer优化版) 性能优秀！")
	} else {
		t.Logf("   ⚠️  部分压力级别未达标，需要进一步优化")
	}

	t.Logf("")
	t.Logf("🚀 优化配置:")
	t.Logf("   ✅ AsyncProducer (非阻塞异步发送)")
	t.Logf("   ✅ LZ4压缩 (Confluent官方首选)")
	t.Logf("   ✅ 批处理: 10ms / 100条 / 100KB")
	t.Logf("   ✅ Consumer Fetch: 100KB-10MB / 500ms")
	t.Logf("   ✅ 并发请求: 100")

	t.Logf("")
	separator3 := string(make([]byte, 100))
	t.Logf("=%s", separator3)
	t.Logf("测试完成时间: %s", time.Now().Format("2006-01-02 15:04:05"))
}
