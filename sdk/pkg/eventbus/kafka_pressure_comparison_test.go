package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// KafkaPressureMetrics Kafka压力测试指标
type KafkaPressureMetrics struct {
	TestName          string
	MessageCount      int64
	ReceivedCount     int64
	Errors            int64
	Duration          time.Duration
	Throughput        float64
	SuccessRate       float64
	SendRate          float64
	SendDuration      time.Duration
	FirstMessageTime  time.Time
	LastMessageTime   time.Time
	ProcessingLatency time.Duration
	InitialGoroutines int
	PeakGoroutines    int
	InitialMemory     uint64
	PeakMemory        uint64
}

// TestKafkaPressureComparison Kafka低压、中压、高压对比测试
func TestKafkaPressureComparison(t *testing.T) {
	t.Logf("🚀 Starting Kafka Pressure Comparison Test...")

	// 测试场景定义
	testScenarios := []struct {
		name         string
		messageCount int
		timeout      time.Duration
		description  string
	}{
		{"Low", 1000, 3 * time.Minute, "低压力测试 - 1000条消息"},
		{"Medium", 3000, 5 * time.Minute, "中压力测试 - 3000条消息"},
		{"High", 6000, 8 * time.Minute, "高压力测试 - 6000条消息"},
	}

	// 存储所有测试结果
	results := make(map[string]*KafkaPressureMetrics)

	for _, scenario := range testScenarios {
		t.Logf("\n🎯 ===== %s =====", scenario.description)

		// 运行Kafka压力测试
		metrics := runKafkaPressureTest(t, scenario.name, scenario.messageCount, scenario.timeout)
		results[scenario.name] = metrics

		// 等待一下，准备下一个测试
		time.Sleep(10 * time.Second)
	}

	// 生成压力测试对比报告
	generatePressureComparisonReport(t, results)
}

// runKafkaPressureTest 运行Kafka压力测试
func runKafkaPressureTest(t *testing.T, testName string, messageCount int, timeout time.Duration) *KafkaPressureMetrics {
	// Kafka配置 - 使用优化后的平衡配置
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"}, // RedPanda
		ClientID: fmt.Sprintf("kafka-pressure-%s", testName),
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
			Compression:     "", // 自动优化为snappy
			MaxInFlight:     0,  // 自动优化为50
		},
		Consumer: ConsumerConfig{
			GroupID:            fmt.Sprintf("kafka-pressure-%s-group", testName),
			AutoOffsetReset:    "earliest",
			SessionTimeout:     6 * time.Second,        // 平衡配置
			HeartbeatInterval:  2 * time.Second,        // 平衡配置
			MaxProcessingTime:  5 * time.Second,        // 平衡配置
			FetchMinBytes:      1024 * 1024,            // 1MB
			FetchMaxBytes:      50 * 1024 * 1024,       // 50MB
			FetchMaxWait:       100 * time.Millisecond, // 平衡配置
			MaxPollRecords:     2000,                   // 大批量
			EnableAutoCommit:   true,
			AutoCommitInterval: 500 * time.Millisecond,
		},
	}

	eventBus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("Failed to create Kafka EventBus: %v", err)
	}
	defer eventBus.Close()

	// 设置预订阅topic
	kafkaBus := eventBus.(*kafkaEventBus)
	testTopic := fmt.Sprintf("kafka.pressure.%s.test", testName)
	kafkaBus.allPossibleTopics = []string{testTopic}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 初始化指标
	metrics := &KafkaPressureMetrics{
		TestName:          testName,
		MessageCount:      int64(messageCount),
		InitialGoroutines: runtime.NumGoroutine(),
	}

	// 获取初始内存
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.InitialMemory = m.Alloc

	// 消息处理统计
	var receivedCount int64

	// 消息处理器
	handler := func(ctx context.Context, message []byte) error {
		receiveTime := time.Now()
		count := atomic.AddInt64(&receivedCount, 1)

		if count == 1 {
			metrics.FirstMessageTime = receiveTime
		}
		metrics.LastMessageTime = receiveTime

		return nil
	}

	// 订阅topic（包含预热）
	err = eventBus.Subscribe(ctx, testTopic, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe to Kafka: %v", err)
	}

	t.Logf("🚀 Starting Kafka %s pressure test with %d messages", testName, messageCount)

	// 发送消息
	sendStart := time.Now()
	errors := 0
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":       fmt.Sprintf("kafka-%s-msg-%d", testName, i),
			"content":  fmt.Sprintf("Kafka %s pressure test message %d", testName, i),
			"sendTime": time.Now().Format(time.RFC3339Nano),
			"index":    i,
			"testName": testName,
		}

		messageBytes, _ := json.Marshal(message)
		err := eventBus.Publish(ctx, testTopic, messageBytes)
		if err != nil {
			errors++
			if errors <= 5 {
				t.Logf("Kafka publish error %d: %v", errors, err)
			}
		}
	}
	sendEnd := time.Now()
	metrics.SendDuration = sendEnd.Sub(sendStart)
	metrics.SendRate = float64(messageCount-errors) / metrics.SendDuration.Seconds()

	t.Logf("📤 Kafka sent %d messages in %.2f seconds (%.2f msg/s)",
		messageCount-errors, metrics.SendDuration.Seconds(), metrics.SendRate)

	// 等待消息处理
	waitTime := timeout - time.Since(sendStart) - 15*time.Second
	if waitTime > 0 {
		t.Logf("⏳ Kafka waiting %.0f seconds for message processing...", waitTime.Seconds())
		time.Sleep(waitTime)
	}

	// 收集最终指标
	endTime := time.Now()
	metrics.Duration = endTime.Sub(sendStart)
	metrics.ReceivedCount = atomic.LoadInt64(&receivedCount)
	metrics.Errors = int64(errors)
	metrics.SuccessRate = float64(metrics.ReceivedCount) / float64(metrics.MessageCount) * 100
	metrics.Throughput = float64(metrics.ReceivedCount) / metrics.Duration.Seconds()

	// 计算处理延迟
	if !metrics.FirstMessageTime.IsZero() && !metrics.LastMessageTime.IsZero() {
		metrics.ProcessingLatency = metrics.LastMessageTime.Sub(metrics.FirstMessageTime)
	}

	// 获取峰值资源使用
	metrics.PeakGoroutines = runtime.NumGoroutine()
	runtime.ReadMemStats(&m)
	metrics.PeakMemory = m.Alloc

	// 输出测试结果
	t.Logf("📊 Kafka %s Pressure Results:", testName)
	t.Logf("   📤 Messages sent: %d", messageCount-errors)
	t.Logf("   📥 Messages received: %d", metrics.ReceivedCount)
	t.Logf("   ✅ Success rate: %.2f%%", metrics.SuccessRate)
	t.Logf("   🚀 Send rate: %.2f msg/s", metrics.SendRate)
	t.Logf("   🚀 Throughput: %.2f msg/s", metrics.Throughput)
	t.Logf("   ⏱️  Processing latency: %v", metrics.ProcessingLatency)
	t.Logf("   ⚠️ Errors: %d", metrics.Errors)
	t.Logf("   🔧 Goroutines: %d → %d (+%d)",
		metrics.InitialGoroutines, metrics.PeakGoroutines,
		metrics.PeakGoroutines-metrics.InitialGoroutines)
	t.Logf("   💾 Memory: %.2f MB → %.2f MB (+%.2f MB)",
		float64(metrics.InitialMemory)/1024/1024,
		float64(metrics.PeakMemory)/1024/1024,
		float64(metrics.PeakMemory-metrics.InitialMemory)/1024/1024)

	return metrics
}

// generatePressureComparisonReport 生成压力测试对比报告
func generatePressureComparisonReport(t *testing.T, results map[string]*KafkaPressureMetrics) {
	t.Logf("\n🏆 ===== KAFKA PRESSURE COMPARISON REPORT =====")

	scenarios := []string{"Low", "Medium", "High"}

	// 表格头部
	t.Logf("📊 Kafka Pressure Performance Table:")
	t.Logf("%-12s | %-12s | %-12s | %-12s | %-12s | %-12s | %-12s",
		"Pressure", "Messages", "Success Rate", "Send Rate", "Throughput", "Latency", "Goroutines")
	t.Logf("%-12s-+-%-12s-+-%-12s-+-%-12s-+-%-12s-+-%-12s-+-%-12s",
		"------------", "------------", "------------", "------------", "------------", "------------", "------------")

	for _, scenario := range scenarios {
		metrics := results[scenario]
		if metrics != nil {
			goroutineIncrease := metrics.PeakGoroutines - metrics.InitialGoroutines
			t.Logf("%-12s | %10d | %10.2f%% | %9.2f/s | %9.2f/s | %9.0fms | %9d",
				scenario,
				metrics.MessageCount,
				metrics.SuccessRate,
				metrics.SendRate,
				metrics.Throughput,
				float64(metrics.ProcessingLatency.Nanoseconds())/1000000,
				goroutineIncrease)
		}
	}

	// 详细分析
	t.Logf("\n📈 Detailed Pressure Analysis:")

	for _, scenario := range scenarios {
		metrics := results[scenario]
		if metrics != nil {
			t.Logf("\n🎯 %s Pressure Analysis:", scenario)

			// 性能评估
			if metrics.SuccessRate >= 90 {
				t.Logf("   ✅ Success Rate: Excellent (%.2f%%)", metrics.SuccessRate)
			} else if metrics.SuccessRate >= 70 {
				t.Logf("   ⚠️ Success Rate: Good (%.2f%%)", metrics.SuccessRate)
			} else {
				t.Logf("   ❌ Success Rate: Poor (%.2f%%)", metrics.SuccessRate)
			}

			if metrics.SendRate >= 100 {
				t.Logf("   🚀 Send Rate: High Performance (%.2f msg/s)", metrics.SendRate)
			} else if metrics.SendRate >= 10 {
				t.Logf("   📊 Send Rate: Moderate Performance (%.2f msg/s)", metrics.SendRate)
			} else {
				t.Logf("   ⚠️ Send Rate: Low Performance (%.2f msg/s)", metrics.SendRate)
			}

			if metrics.Throughput >= 10 {
				t.Logf("   📈 Throughput: Good (%.2f msg/s)", metrics.Throughput)
			} else if metrics.Throughput >= 5 {
				t.Logf("   📊 Throughput: Moderate (%.2f msg/s)", metrics.Throughput)
			} else {
				t.Logf("   ⚠️ Throughput: Low (%.2f msg/s)", metrics.Throughput)
			}

			// 资源使用分析
			goroutineIncrease := metrics.PeakGoroutines - metrics.InitialGoroutines
			memoryIncrease := float64(metrics.PeakMemory-metrics.InitialMemory) / 1024 / 1024

			if goroutineIncrease < 100 {
				t.Logf("   🔧 Resource Usage: Efficient (+%d goroutines, +%.2f MB)", goroutineIncrease, memoryIncrease)
			} else {
				t.Logf("   ⚠️ Resource Usage: High (+%d goroutines, +%.2f MB)", goroutineIncrease, memoryIncrease)
			}
		}
	}

	// 压力级别对比
	t.Logf("\n📊 Pressure Level Comparison:")

	lowMetrics := results["Low"]
	mediumMetrics := results["Medium"]
	highMetrics := results["High"]

	if lowMetrics != nil && mediumMetrics != nil && highMetrics != nil {
		t.Logf("   📈 Success Rate Trend: Low(%.1f%%) → Medium(%.1f%%) → High(%.1f%%)",
			lowMetrics.SuccessRate, mediumMetrics.SuccessRate, highMetrics.SuccessRate)

		t.Logf("   🚀 Send Rate Trend: Low(%.1f) → Medium(%.1f) → High(%.1f) msg/s",
			lowMetrics.SendRate, mediumMetrics.SendRate, highMetrics.SendRate)

		t.Logf("   📊 Throughput Trend: Low(%.1f) → Medium(%.1f) → High(%.1f) msg/s",
			lowMetrics.Throughput, mediumMetrics.Throughput, highMetrics.Throughput)

		// 性能趋势分析
		if highMetrics.SuccessRate >= lowMetrics.SuccessRate*0.8 {
			t.Logf("   ✅ Scalability: Good - maintains performance under pressure")
		} else {
			t.Logf("   ⚠️ Scalability: Limited - performance degrades under pressure")
		}

		// 最佳压力级别推荐
		bestScenario := "Low"
		bestScore := lowMetrics.SuccessRate + lowMetrics.Throughput

		mediumScore := mediumMetrics.SuccessRate + mediumMetrics.Throughput
		if mediumScore > bestScore {
			bestScenario = "Medium"
			bestScore = mediumScore
		}

		highScore := highMetrics.SuccessRate + highMetrics.Throughput
		if highScore > bestScore {
			bestScenario = "High"
		}

		t.Logf("   🏆 Recommended Pressure Level: %s (best overall performance)", bestScenario)
	}

	t.Logf("\n🎯 Kafka pressure comparison test completed!")
}
