package performance_tests

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 🎯 Memory EventBus 性能对比测试
//
// 测试要求：
// 1. 创建 Memory EventBus 实例
// 2. 测试两种模式：
//    a) 使用 Envelope（PublishEnvelope/SubscribeEnvelope）- 有序处理
//    b) 不使用 Envelope（Publish/Subscribe）- 无序处理
// 3. 测试覆盖低压500、中压2000、高压5000、极限10000
// 4. Topic 数量为 5
// 5. 输出报告包括性能指标、资源占用情况
// 6. 检查协程泄漏和内存使用
// 7. 对比 Envelope 和非 Envelope 的性能差异

// MemoryEventBusPerfMetrics Memory EventBus 性能指标
type MemoryEventBusPerfMetrics struct {
	// 基本信息
	Mode         string        // 模式 (Envelope/NonEnvelope)
	Pressure     string        // 压力级别
	MessageCount int           // 消息总数
	StartTime    time.Time     // 开始时间
	EndTime      time.Time     // 结束时间
	Duration     time.Duration // 持续时间

	// 消息指标
	MessagesSent     int64   // 发送消息数
	MessagesReceived int64   // 接收消息数
	SendErrors       int64   // 发送错误数
	ProcessErrors    int64   // 处理错误数
	SuccessRate      float64 // 成功率

	// 性能指标
	SendThroughput    float64 // 发送吞吐量 (msg/s)
	ReceiveThroughput float64 // 接收吞吐量 (msg/s)
	AvgSendLatency    float64 // 平均发送延迟 (ms)
	AvgProcessLatency float64 // 平均处理延迟 (ms)

	// 资源占用
	InitialGoroutines int     // 初始协程数
	PeakGoroutines    int32   // 峰值协程数
	FinalGoroutines   int     // 最终协程数
	GoroutineLeak     int     // 协程泄漏数
	InitialMemoryMB   float64 // 初始内存 (MB)
	PeakMemoryMB      uint64  // 峰值内存 (MB)
	FinalMemoryMB     float64 // 最终内存 (MB)
	MemoryDeltaMB     float64 // 内存增量 (MB)

	// 连接和消费者组统计
	TopicCount      int      // Topic 数量
	TopicList       []string // Topic 列表

	// 顺序性指标（仅 Envelope 模式）
	OrderViolations int64 // 顺序违反次数

	// 内部统计
	sendLatencySum   int64 // 发送延迟总和 (微秒)
	sendLatencyCount int64 // 发送延迟计数
	procLatencySum   int64 // 处理延迟总和 (微秒)
	procLatencyCount int64 // 处理延迟计数
}

// MemoryOrderChecker 内存 EventBus 顺序检查器
type MemoryOrderChecker struct {
	shards     [256]*memoryOrderShard // 256 个分片
	violations int64                  // 顺序违反计数
}

type memoryOrderShard struct {
	mu        sync.Mutex
	sequences map[string]int64
}

// NewMemoryOrderChecker 创建顺序检查器
func NewMemoryOrderChecker() *MemoryOrderChecker {
	oc := &MemoryOrderChecker{}
	for i := 0; i < 256; i++ {
		oc.shards[i] = &memoryOrderShard{
			sequences: make(map[string]int64),
		}
	}
	return oc
}

// Check 检查顺序
func (oc *MemoryOrderChecker) Check(aggregateID string, version int64) bool {
	h := fnv.New32a()
	h.Write([]byte(aggregateID))
	shardIndex := h.Sum32() % 256

	shard := oc.shards[shardIndex]
	shard.mu.Lock()
	defer shard.mu.Unlock()

	lastSeq, exists := shard.sequences[aggregateID]
	if exists && version <= lastSeq {
		atomic.AddInt64(&oc.violations, 1)
		return false
	}

	shard.sequences[aggregateID] = version
	return true
}

// GetViolations 获取顺序违反次数
func (oc *MemoryOrderChecker) GetViolations() int64 {
	return atomic.LoadInt64(&oc.violations)
}

// TestMemoryEventBusPerformance 内存 EventBus 性能测试
func TestMemoryEventBusPerformance(t *testing.T) {
	// 初始化 logger（如果还没有初始化）
	if logger.DefaultLogger == nil {
		zapLogger, err := zap.NewDevelopment()
		if err != nil {
			t.Fatalf("Failed to initialize logger: %v", err)
		}
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	t.Logf("🚀 开始 Memory EventBus 性能测试")

	// 压力级别定义
	pressureLevels := []struct {
		name  string
		count int
	}{
		{"Low", 500},
		{"Medium", 2000},
		{"High", 5000},
		{"Extreme", 10000},
	}

	// 存储所有测试结果
	var allResults []*MemoryEventBusPerfMetrics

	// 测试两种模式
	modes := []struct {
		name      string
		useEnvelope bool
	}{
		{"Envelope", true},
		{"NonEnvelope", false},
	}

	for _, mode := range modes {
		t.Logf("\n📋 开始测试模式: %s", mode.name)

		for _, pressure := range pressureLevels {
			t.Logf("\n⏳ 测试场景: %s - %s (%d 条消息)", mode.name, pressure.name, pressure.count)

			// 创建 Memory EventBus
			config := &eventbus.EventBusConfig{
				Type: "memory",
			}
			bus, err := eventbus.NewEventBus(config)
			require.NoError(t, err)
			require.NotNil(t, bus)

			// 运行性能测试
			metrics := runMemoryEventBusTest(t, bus, mode.useEnvelope, pressure.name, pressure.count)
			allResults = append(allResults, metrics)

			// 关闭 EventBus
			bus.Close()

			// 等待资源释放
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 生成综合报告
	generateMemoryEventBusReport(t, allResults)
}

// runMemoryEventBusTest 运行单个内存 EventBus 测试
func runMemoryEventBusTest(t *testing.T, bus eventbus.EventBus, useEnvelope bool, pressure string, messageCount int) *MemoryEventBusPerfMetrics {
	metrics := &MemoryEventBusPerfMetrics{
		Mode:         map[bool]string{true: "Envelope", false: "NonEnvelope"}[useEnvelope],
		Pressure:     pressure,
		MessageCount: messageCount,
		TopicCount:   5,
	}

	// 记录初始资源状态
	metrics.InitialGoroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.InitialMemoryMB = float64(m.Alloc) / 1024 / 1024

	// 创建 Topics
	topics := make([]string, metrics.TopicCount)
	for i := 0; i < metrics.TopicCount; i++ {
		topics[i] = fmt.Sprintf("memory.perf.%s.topic%d", strings.ToLower(pressure), i+1)
	}
	metrics.TopicList = topics

	// 初始化计数器
	var (
		sentCount  int64
		receivedCount int64
		sendErrors int64
	)

	// 初始化顺序检查器（仅 Envelope 模式）
	var orderChecker *MemoryOrderChecker
	if useEnvelope {
		orderChecker = NewMemoryOrderChecker()
	}

	// 创建基础消息处理器
	messageHandler := func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&receivedCount, 1)

		// 更新峰值协程数
		currentGoroutines := int32(runtime.NumGoroutine())
		for {
			peak := atomic.LoadInt32(&metrics.PeakGoroutines)
			if currentGoroutines <= peak || atomic.CompareAndSwapInt32(&metrics.PeakGoroutines, peak, currentGoroutines) {
				break
			}
		}

		// 更新峰值内存
		runtime.ReadMemStats(&m)
		currentMemory := math.Float64bits(float64(m.Alloc) / 1024 / 1024)
		for {
			peak := atomic.LoadUint64(&metrics.PeakMemoryMB)
			if currentMemory <= peak || atomic.CompareAndSwapUint64(&metrics.PeakMemoryMB, peak, currentMemory) {
				break
			}
		}

		return nil
	}

	// 创建 Envelope 处理器
	envelopeHandler := func(ctx context.Context, envelope *eventbus.Envelope) error {
		atomic.AddInt64(&receivedCount, 1)

		// 检查顺序
		if orderChecker != nil {
			orderChecker.Check(envelope.AggregateID, envelope.EventVersion)
		}

		// 更新峰值协程数
		currentGoroutines := int32(runtime.NumGoroutine())
		for {
			peak := atomic.LoadInt32(&metrics.PeakGoroutines)
			if currentGoroutines <= peak || atomic.CompareAndSwapInt32(&metrics.PeakGoroutines, peak, currentGoroutines) {
				break
			}
		}

		// 更新峰值内存
		runtime.ReadMemStats(&m)
		currentMemory := math.Float64bits(float64(m.Alloc) / 1024 / 1024)
		for {
			peak := atomic.LoadUint64(&metrics.PeakMemoryMB)
			if currentMemory <= peak || atomic.CompareAndSwapUint64(&metrics.PeakMemoryMB, peak, currentMemory) {
				break
			}
		}

		return nil
	}

	// 订阅所有 Topics
	for _, topic := range topics {
		if useEnvelope {
			err := bus.SubscribeEnvelope(context.Background(), topic, envelopeHandler)
			require.NoError(t, err, "订阅 Envelope 失败: %s", topic)
		} else {
			err := bus.Subscribe(context.Background(), topic, messageHandler)
			require.NoError(t, err, "订阅失败: %s", topic)
		}
	}

	// 等待订阅建立
	time.Sleep(100 * time.Millisecond)

	// 开始发送消息
	metrics.StartTime = time.Now()

	// 计算每个 topic 的消息数
	messagesPerTopic := messageCount / metrics.TopicCount
	if messageCount%metrics.TopicCount != 0 {
		messagesPerTopic++
	}

	// 并发发送消息
	var wg sync.WaitGroup
	for topicIdx, topic := range topics {
		wg.Add(1)
		go func(idx int, topicName string) {
			defer wg.Done()

			// 计算该 topic 应该发送的消息数
			startMsg := idx * messagesPerTopic
			endMsg := startMsg + messagesPerTopic
			if endMsg > messageCount {
				endMsg = messageCount
			}

			for msgIdx := startMsg; msgIdx < endMsg; msgIdx++ {
				aggregateID := fmt.Sprintf("agg-%d", msgIdx%100)
				version := int64(msgIdx)
				payload := []byte(fmt.Sprintf("message-%d", msgIdx))

				if useEnvelope {
					envelope := eventbus.NewEnvelopeWithAutoID(aggregateID, "TestEvent", version, payload)
					err := bus.PublishEnvelope(context.Background(), topicName, envelope)
					if err != nil {
						atomic.AddInt64(&sendErrors, 1)
					} else {
						atomic.AddInt64(&sentCount, 1)
					}
				} else {
					err := bus.Publish(context.Background(), topicName, payload)
					if err != nil {
						atomic.AddInt64(&sendErrors, 1)
					} else {
						atomic.AddInt64(&sentCount, 1)
					}
				}
			}
		}(topicIdx, topic)
	}

	wg.Wait()

	// 等待所有消息被处理
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️  等待消息处理超时")
			goto done
		case <-ticker.C:
			if atomic.LoadInt64(&receivedCount) >= atomic.LoadInt64(&sentCount) {
				goto done
			}
		}
	}

done:
	metrics.EndTime = time.Now()
	metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)

	// 记录最终资源状态
	metrics.FinalGoroutines = runtime.NumGoroutine()
	runtime.ReadMemStats(&m)
	metrics.FinalMemoryMB = float64(m.Alloc) / 1024 / 1024

	// 计算指标
	metrics.MessagesSent = atomic.LoadInt64(&sentCount)
	metrics.MessagesReceived = atomic.LoadInt64(&receivedCount)
	metrics.SendErrors = atomic.LoadInt64(&sendErrors)

	metrics.SuccessRate = 100.0
	if metrics.MessagesSent > 0 {
		metrics.SuccessRate = float64(metrics.MessagesReceived) / float64(metrics.MessagesSent) * 100
	}

	if metrics.Duration.Seconds() > 0 {
		metrics.SendThroughput = float64(metrics.MessagesSent) / metrics.Duration.Seconds()
		metrics.ReceiveThroughput = float64(metrics.MessagesReceived) / metrics.Duration.Seconds()
	}

	metrics.MemoryDeltaMB = metrics.FinalMemoryMB - metrics.InitialMemoryMB
	metrics.GoroutineLeak = metrics.FinalGoroutines - metrics.InitialGoroutines

	// 获取顺序违反次数（仅 Envelope 模式）
	if useEnvelope && orderChecker != nil {
		metrics.OrderViolations = orderChecker.GetViolations()
	}

	// 转换峰值内存
	if metrics.PeakMemoryMB > 0 {
		metrics.PeakMemoryMB = math.Float64bits(math.Float64frombits(metrics.PeakMemoryMB))
	}

	t.Logf("✅ 测试完成")
	t.Logf("   📊 发送: %d, 接收: %d, 成功率: %.2f%%", metrics.MessagesSent, metrics.MessagesReceived, metrics.SuccessRate)
	t.Logf("   ⚡ 吞吐量: %.2f msg/s", metrics.SendThroughput)
	t.Logf("   💾 内存增量: %.2f MB", metrics.MemoryDeltaMB)
	t.Logf("   🔗 协程泄漏: %d", metrics.GoroutineLeak)

	return metrics
}

// generateMemoryEventBusReport 生成综合报告
func generateMemoryEventBusReport(t *testing.T, allResults []*MemoryEventBusPerfMetrics) {
	t.Logf("\n%s", strings.Repeat("=", 100))
	t.Logf("🏆 Memory EventBus 性能对比报告")
	t.Logf("%s\n", strings.Repeat("=", 100))

	// 按模式分组结果
	envelopeResults := make([]*MemoryEventBusPerfMetrics, 0)
	nonEnvelopeResults := make([]*MemoryEventBusPerfMetrics, 0)

	for _, result := range allResults {
		if result.Mode == "Envelope" {
			envelopeResults = append(envelopeResults, result)
		} else {
			nonEnvelopeResults = append(nonEnvelopeResults, result)
		}
	}

	// 打印 Envelope 模式结果
	t.Logf("\n📋 Envelope 模式性能指标:")
	t.Logf("%-10s | %-15s | %-15s | %-15s | %-15s | %-15s", "压力级别", "吞吐量(msg/s)", "延迟(ms)", "成功率(%)", "内存增量(MB)", "协程泄漏")
	t.Logf("%s", strings.Repeat("-", 100))

	var envelopeThroughputSum, envelopeLatencySum, envelopeMemorySum float64
	for _, result := range envelopeResults {
		avgLatency := 0.0
		if result.MessagesReceived > 0 {
			avgLatency = float64(result.procLatencySum) / float64(result.procLatencyCount) / 1000.0
		}
		t.Logf("%-10s | %15.2f | %15.2f | %15.2f | %15.2f | %15d",
			result.Pressure,
			result.SendThroughput,
			avgLatency,
			result.SuccessRate,
			result.MemoryDeltaMB,
			result.GoroutineLeak)

		envelopeThroughputSum += result.SendThroughput
		envelopeLatencySum += avgLatency
		envelopeMemorySum += result.MemoryDeltaMB
	}

	// 打印 NonEnvelope 模式结果
	t.Logf("\n📋 NonEnvelope 模式性能指标:")
	t.Logf("%-10s | %-15s | %-15s | %-15s | %-15s | %-15s", "压力级别", "吞吐量(msg/s)", "延迟(ms)", "成功率(%)", "内存增量(MB)", "协程泄漏")
	t.Logf("%s", strings.Repeat("-", 100))

	var nonEnvelopeThroughputSum, nonEnvelopeLatencySum, nonEnvelopeMemorySum float64
	for _, result := range nonEnvelopeResults {
		avgLatency := 0.0
		if result.MessagesReceived > 0 {
			avgLatency = float64(result.procLatencySum) / float64(result.procLatencyCount) / 1000.0
		}
		t.Logf("%-10s | %15.2f | %15.2f | %15.2f | %15.2f | %15d",
			result.Pressure,
			result.SendThroughput,
			avgLatency,
			result.SuccessRate,
			result.MemoryDeltaMB,
			result.GoroutineLeak)

		nonEnvelopeThroughputSum += result.SendThroughput
		nonEnvelopeLatencySum += avgLatency
		nonEnvelopeMemorySum += result.MemoryDeltaMB
	}

	// 计算平均值
	if len(envelopeResults) > 0 {
		t.Logf("\n📊 Envelope 模式平均值:")
		t.Logf("   平均吞吐量: %.2f msg/s", envelopeThroughputSum/float64(len(envelopeResults)))
		t.Logf("   平均延迟: %.2f ms", envelopeLatencySum/float64(len(envelopeResults)))
		t.Logf("   平均内存增量: %.2f MB", envelopeMemorySum/float64(len(envelopeResults)))
	}

	if len(nonEnvelopeResults) > 0 {
		t.Logf("\n📊 NonEnvelope 模式平均值:")
		t.Logf("   平均吞吐量: %.2f msg/s", nonEnvelopeThroughputSum/float64(len(nonEnvelopeResults)))
		t.Logf("   平均延迟: %.2f ms", nonEnvelopeLatencySum/float64(len(nonEnvelopeResults)))
		t.Logf("   平均内存增量: %.2f MB", nonEnvelopeMemorySum/float64(len(nonEnvelopeResults)))
	}

	// 性能对比
	if len(envelopeResults) > 0 && len(nonEnvelopeResults) > 0 {
		t.Logf("\n🏆 性能对比分析:")

		avgEnvelopeThroughput := envelopeThroughputSum / float64(len(envelopeResults))
		avgNonEnvelopeThroughput := nonEnvelopeThroughputSum / float64(len(nonEnvelopeResults))
		throughputDiff := (avgNonEnvelopeThroughput - avgEnvelopeThroughput) / avgEnvelopeThroughput * 100

		avgEnvelopeLatency := envelopeLatencySum / float64(len(envelopeResults))
		avgNonEnvelopeLatency := nonEnvelopeLatencySum / float64(len(nonEnvelopeResults))
		latencyDiff := (avgNonEnvelopeLatency - avgEnvelopeLatency) / avgEnvelopeLatency * 100

		avgEnvelopeMemory := envelopeMemorySum / float64(len(envelopeResults))
		avgNonEnvelopeMemory := nonEnvelopeMemorySum / float64(len(nonEnvelopeResults))
		memoryDiff := (avgNonEnvelopeMemory - avgEnvelopeMemory) / avgEnvelopeMemory * 100

		t.Logf("   吞吐量: NonEnvelope 比 Envelope %+.2f%%", throughputDiff)
		t.Logf("   延迟: NonEnvelope 比 Envelope %+.2f%%", latencyDiff)
		t.Logf("   内存: NonEnvelope 比 Envelope %+.2f%%", memoryDiff)

		if throughputDiff > 0 {
			t.Logf("\n   ✅ NonEnvelope 模式吞吐量更高（无序处理的优势）")
		} else {
			t.Logf("\n   ✅ Envelope 模式吞吐量更高（有序处理的代价）")
		}
	}

	t.Logf("\n%s", strings.Repeat("=", 100))
	t.Logf("✅ 测试完成！共执行 %d 个场景", len(allResults))
	t.Logf("%s\n", strings.Repeat("=", 100))
}
