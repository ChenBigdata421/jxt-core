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

// ComparisonMetrics 对比测试指标
type ComparisonMetrics struct {
	// 基础指标
	TotalMessages int64
	ReceivedCount int64
	Errors        int64
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	Throughput    float64
	SuccessRate   float64

	// 延迟指标
	FirstMessageTime  time.Time
	LastMessageTime   time.Time
	ProcessingLatency time.Duration
	SendRate          float64
	SendDuration      time.Duration

	// 资源指标
	InitialGoroutines int
	PeakGoroutines    int
	InitialMemory     uint64
	PeakMemory        uint64

	// 错误分析
	PublishErrors  int64
	ConsumerErrors int64
}

// TestComprehensiveKafkaNatsComparison 全面的Kafka vs NATS JetStream对比测试
func TestComprehensiveKafkaNatsComparison(t *testing.T) {
	t.Logf("🚀 Starting Comprehensive Kafka vs NATS JetStream Comparison Test...")

	// 测试场景定义
	testScenarios := []struct {
		name         string
		messageCount int
		timeout      time.Duration
		description  string
	}{
		{"Low Pressure", 1000, 2 * time.Minute, "低压力测试 - 1000条消息"},
		{"Medium Pressure", 3000, 4 * time.Minute, "中压力测试 - 3000条消息"},
		{"High Pressure", 6000, 6 * time.Minute, "高压力测试 - 6000条消息"},
	}

	// 存储所有测试结果
	kafkaResults := make(map[string]*ComparisonMetrics)
	natsResults := make(map[string]*ComparisonMetrics)

	for _, scenario := range testScenarios {
		t.Logf("\n🎯 ===== %s =====", scenario.description)

		// 测试Kafka
		t.Logf("📊 Testing Kafka - %s", scenario.name)
		kafkaMetrics := runKafkaComparisonTest(t, scenario.name, scenario.messageCount, scenario.timeout)
		kafkaResults[scenario.name] = kafkaMetrics

		// 等待一下，避免资源冲突
		time.Sleep(5 * time.Second)

		// 测试NATS JetStream
		t.Logf("📊 Testing NATS JetStream - %s", scenario.name)
		natsMetrics := runNatsComparisonTest(t, scenario.name, scenario.messageCount, scenario.timeout)
		natsResults[scenario.name] = natsMetrics

		// 等待一下，准备下一个测试
		time.Sleep(10 * time.Second)
	}

	// 生成对比报告
	generateComparisonReport(t, kafkaResults, natsResults)
}

// runKafkaComparisonTest 运行Kafka对比测试
func runKafkaComparisonTest(t *testing.T, testName string, messageCount int, timeout time.Duration) *ComparisonMetrics {
	// Kafka配置 - 使用优化后的配置
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"}, // RedPanda
		ClientID: fmt.Sprintf("kafka-comparison-%s", testName),
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
			Compression:     "", // 自动优化为snappy
			MaxInFlight:     0,  // 自动优化为50
		},
		Consumer: ConsumerConfig{
			GroupID:            fmt.Sprintf("kafka-comparison-%s-group", testName),
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

	return runComparisonTest(t, eventBus, "kafka", testName, messageCount, timeout)
}

// runNatsComparisonTest 运行NATS JetStream对比测试
func runNatsComparisonTest(t *testing.T, testName string, messageCount int, timeout time.Duration) *ComparisonMetrics {
	// NATS JetStream配置
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4222"},
		ClientID:            fmt.Sprintf("nats-comparison-%s", testName),
		MaxReconnects:       10,
		ReconnectWait:       2 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 5 * time.Second,
			AckWait:        30 * time.Second,
			MaxDeliver:     3,
			Stream: StreamConfig{
				Name:      fmt.Sprintf("comparison-%s-stream", testName),
				Subjects:  []string{fmt.Sprintf("comparison.nats.%s.>", testName)},
				Retention: "limits",
				Storage:   "memory",
				Replicas:  1,
				MaxAge:    24 * time.Hour,
				MaxBytes:  256 * 1024 * 1024, // 256MB
				MaxMsgs:   100000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("comparison-%s-consumer", testName),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 1000,
				MaxWaiting:    512,
				MaxDeliver:    3,
			},
		},
	}

	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Skipf("NATS server not available for %s: %v", testName, err)
		return &ComparisonMetrics{} // 返回空指标
	}
	defer eventBus.Close()

	return runComparisonTest(t, eventBus, "nats", testName, messageCount, timeout)
}

// runComparisonTest 运行通用对比测试
func runComparisonTest(t *testing.T, eventBus EventBus, busType, testName string, messageCount int, timeout time.Duration) *ComparisonMetrics {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 初始化指标
	metrics := &ComparisonMetrics{
		TotalMessages:     int64(messageCount),
		StartTime:         time.Now(),
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

	// 订阅topic
	testTopic := fmt.Sprintf("comparison.%s.%s.test", busType, testName)
	err := eventBus.Subscribe(ctx, testTopic, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe to %s: %v", busType, err)
	}

	// 等待订阅生效
	time.Sleep(3 * time.Second)

	t.Logf("🚀 Starting %s %s test with %d messages", busType, testName, messageCount)

	// 发送消息
	sendStart := time.Now()
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":       fmt.Sprintf("%s-%s-msg-%d", busType, testName, i),
			"content":  fmt.Sprintf("%s comparison test message %d", busType, i),
			"sendTime": time.Now().Format(time.RFC3339Nano),
			"index":    i,
			"busType":  busType,
			"testName": testName,
		}

		messageBytes, _ := json.Marshal(message)
		err := eventBus.Publish(ctx, testTopic, messageBytes)
		if err != nil {
			atomic.AddInt64(&metrics.PublishErrors, 1)
			if metrics.PublishErrors <= 5 {
				t.Logf("%s publish error %d: %v", busType, metrics.PublishErrors, err)
			}
		}
	}
	sendEnd := time.Now()
	metrics.SendDuration = sendEnd.Sub(sendStart)
	metrics.SendRate = float64(messageCount) / metrics.SendDuration.Seconds()

	t.Logf("📤 %s sent %d messages in %.2f seconds (%.2f msg/s)",
		busType, messageCount, metrics.SendDuration.Seconds(), metrics.SendRate)

	// 等待消息处理
	waitTime := timeout - time.Since(metrics.StartTime) - 10*time.Second
	if waitTime > 0 {
		t.Logf("⏳ %s waiting %.0f seconds for message processing...", busType, waitTime.Seconds())
		time.Sleep(waitTime)
	}

	// 收集最终指标
	metrics.EndTime = time.Now()
	metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)
	metrics.ReceivedCount = atomic.LoadInt64(&receivedCount)
	metrics.Errors = atomic.LoadInt64(&metrics.PublishErrors) + atomic.LoadInt64(&metrics.ConsumerErrors)
	metrics.SuccessRate = float64(metrics.ReceivedCount) / float64(metrics.TotalMessages) * 100
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
	t.Logf("📊 %s %s Results:", busType, testName)
	t.Logf("   📤 Messages sent: %d", messageCount)
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

// generateComparisonReport 生成对比报告
func generateComparisonReport(t *testing.T, kafkaResults, natsResults map[string]*ComparisonMetrics) {
	t.Logf("\n🏆 ===== COMPREHENSIVE COMPARISON REPORT =====")

	scenarios := []string{"Low Pressure", "Medium Pressure", "High Pressure"}

	// 表格头部
	t.Logf("📊 Performance Comparison Table:")
	t.Logf("%-15s | %-12s | %-12s | %-12s | %-12s | %-12s | %-12s",
		"Scenario", "Kafka Success", "NATS Success", "Kafka Send", "NATS Send", "Kafka Thru", "NATS Thru")
	t.Logf("%-15s-+-%-12s-+-%-12s-+-%-12s-+-%-12s-+-%-12s-+-%-12s",
		"---------------", "------------", "------------", "------------", "------------", "------------", "------------")

	for _, scenario := range scenarios {
		kafkaMetrics := kafkaResults[scenario]
		natsMetrics := natsResults[scenario]

		if kafkaMetrics != nil && natsMetrics != nil {
			t.Logf("%-15s | %10.2f%% | %10.2f%% | %9.2f/s | %9.2f/s | %9.2f/s | %9.2f/s",
				scenario,
				kafkaMetrics.SuccessRate, natsMetrics.SuccessRate,
				kafkaMetrics.SendRate, natsMetrics.SendRate,
				kafkaMetrics.Throughput, natsMetrics.Throughput)
		}
	}

	// 详细对比分析
	t.Logf("\n📈 Detailed Analysis:")

	for _, scenario := range scenarios {
		kafkaMetrics := kafkaResults[scenario]
		natsMetrics := natsResults[scenario]

		if kafkaMetrics != nil && natsMetrics != nil {
			t.Logf("\n🎯 %s Analysis:", scenario)

			// 成功率对比
			if kafkaMetrics.SuccessRate > natsMetrics.SuccessRate {
				improvement := kafkaMetrics.SuccessRate - natsMetrics.SuccessRate
				t.Logf("   ✅ Success Rate: Kafka wins by %.2f%% (%.2f%% vs %.2f%%)",
					improvement, kafkaMetrics.SuccessRate, natsMetrics.SuccessRate)
			} else if natsMetrics.SuccessRate > kafkaMetrics.SuccessRate {
				improvement := natsMetrics.SuccessRate - kafkaMetrics.SuccessRate
				t.Logf("   ✅ Success Rate: NATS wins by %.2f%% (%.2f%% vs %.2f%%)",
					improvement, natsMetrics.SuccessRate, kafkaMetrics.SuccessRate)
			} else {
				t.Logf("   ⚖️ Success Rate: Tie (%.2f%% vs %.2f%%)",
					kafkaMetrics.SuccessRate, natsMetrics.SuccessRate)
			}

			// 发送速率对比
			if kafkaMetrics.SendRate > natsMetrics.SendRate {
				improvement := (kafkaMetrics.SendRate - natsMetrics.SendRate) / natsMetrics.SendRate * 100
				t.Logf("   🚀 Send Rate: Kafka wins by %.2f%% (%.2f vs %.2f msg/s)",
					improvement, kafkaMetrics.SendRate, natsMetrics.SendRate)
			} else if natsMetrics.SendRate > kafkaMetrics.SendRate {
				improvement := (natsMetrics.SendRate - kafkaMetrics.SendRate) / kafkaMetrics.SendRate * 100
				t.Logf("   🚀 Send Rate: NATS wins by %.2f%% (%.2f vs %.2f msg/s)",
					improvement, natsMetrics.SendRate, kafkaMetrics.SendRate)
			}

			// 吞吐量对比
			if kafkaMetrics.Throughput > natsMetrics.Throughput {
				improvement := (kafkaMetrics.Throughput - natsMetrics.Throughput) / natsMetrics.Throughput * 100
				t.Logf("   📊 Throughput: Kafka wins by %.2f%% (%.2f vs %.2f msg/s)",
					improvement, kafkaMetrics.Throughput, natsMetrics.Throughput)
			} else if natsMetrics.Throughput > kafkaMetrics.Throughput {
				improvement := (natsMetrics.Throughput - kafkaMetrics.Throughput) / kafkaMetrics.Throughput * 100
				t.Logf("   📊 Throughput: NATS wins by %.2f%% (%.2f vs %.2f msg/s)",
					improvement, natsMetrics.Throughput, kafkaMetrics.Throughput)
			}

			// 资源使用对比
			kafkaGoroutineIncrease := kafkaMetrics.PeakGoroutines - kafkaMetrics.InitialGoroutines
			natsGoroutineIncrease := natsMetrics.PeakGoroutines - natsMetrics.InitialGoroutines

			if kafkaGoroutineIncrease < natsGoroutineIncrease {
				t.Logf("   🔧 Resource Usage: Kafka more efficient (+%d vs +%d goroutines)",
					kafkaGoroutineIncrease, natsGoroutineIncrease)
			} else if natsGoroutineIncrease < kafkaGoroutineIncrease {
				t.Logf("   🔧 Resource Usage: NATS more efficient (+%d vs +%d goroutines)",
					natsGoroutineIncrease, kafkaGoroutineIncrease)
			}
		}
	}

	// 总体结论
	t.Logf("\n🏆 Overall Conclusion:")

	// 计算总体胜负
	kafkaWins := 0
	natsWins := 0
	ties := 0

	for _, scenario := range scenarios {
		kafkaMetrics := kafkaResults[scenario]
		natsMetrics := natsResults[scenario]

		if kafkaMetrics != nil && natsMetrics != nil {
			kafkaScore := 0
			natsScore := 0

			// 成功率权重最高
			if kafkaMetrics.SuccessRate > natsMetrics.SuccessRate {
				kafkaScore += 3
			} else if natsMetrics.SuccessRate > kafkaMetrics.SuccessRate {
				natsScore += 3
			}

			// 吞吐量权重中等
			if kafkaMetrics.Throughput > natsMetrics.Throughput {
				kafkaScore += 2
			} else if natsMetrics.Throughput > kafkaMetrics.Throughput {
				natsScore += 2
			}

			// 发送速率权重较低
			if kafkaMetrics.SendRate > natsMetrics.SendRate {
				kafkaScore += 1
			} else if natsMetrics.SendRate > kafkaMetrics.SendRate {
				natsScore += 1
			}

			if kafkaScore > natsScore {
				kafkaWins++
			} else if natsScore > kafkaScore {
				natsWins++
			} else {
				ties++
			}
		}
	}

	t.Logf("   📊 Score Summary: Kafka %d wins, NATS %d wins, %d ties", kafkaWins, natsWins, ties)

	if kafkaWins > natsWins {
		t.Logf("   🏆 Overall Winner: Kafka (better performance in %d out of %d scenarios)", kafkaWins, len(scenarios))
	} else if natsWins > kafkaWins {
		t.Logf("   🏆 Overall Winner: NATS JetStream (better performance in %d out of %d scenarios)", natsWins, len(scenarios))
	} else {
		t.Logf("   ⚖️ Overall Result: Tie (both systems perform equally well)")
	}

	t.Logf("\n🎯 Comprehensive comparison test completed!")
}
