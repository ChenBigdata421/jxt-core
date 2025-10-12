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

// ComprehensiveMetrics 全面性能指标
type ComprehensiveMetrics struct {
	// 基本信息
	System    string
	Pressure  string
	Messages  int
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// 消息指标
	Sent        int64
	Received    int64
	Errors      int64
	SuccessRate float64

	// 性能指标
	SendRate       float64 // 发送速率 msg/s
	Throughput     float64 // 吞吐量 msg/s
	FirstLatency   time.Duration
	AverageLatency time.Duration
	P95Latency     time.Duration
	P99Latency     time.Duration

	// 资源占用
	InitialGoroutines int
	PeakGoroutines    int
	GoroutineDelta    int
	InitialMemoryMB   float64
	PeakMemoryMB      float64
	MemoryDeltaMB     float64
	CPUUsagePercent   float64

	// 网络指标
	BytesSent      int64
	BytesReceived  int64
	NetworkLatency time.Duration
}

// TestComprehensivePerformanceBenchmark 全面性能基准测试
func TestComprehensivePerformanceBenchmark(t *testing.T) {
	t.Logf("🚀 COMPREHENSIVE PERFORMANCE BENCHMARK")
	t.Logf("📊 Testing Kafka vs NATS with detailed metrics and resource monitoring")

	// 测试场景
	scenarios := []struct {
		name     string
		messages int
		timeout  time.Duration
	}{
		{"Light", 100, 30 * time.Second},
		{"Medium", 500, 60 * time.Second},
		{"Heavy", 1000, 90 * time.Second},
		{"Extreme", 2000, 120 * time.Second},
	}

	allResults := make(map[string][]*ComprehensiveMetrics)
	allResults["Kafka"] = make([]*ComprehensiveMetrics, 0)
	allResults["NATS"] = make([]*ComprehensiveMetrics, 0)

	for _, scenario := range scenarios {
		t.Logf("\n🎯 ===== %s Load Test (%d messages) =====", scenario.name, scenario.messages)

		// 1. Kafka测试
		t.Logf("🔴 Kafka (RedPanda) Performance Test...")
		kafkaMetrics := runComprehensiveKafkaTest(t, scenario.name, scenario.messages, scenario.timeout)
		allResults["Kafka"] = append(allResults["Kafka"], kafkaMetrics)

		// 清理间隔
		time.Sleep(5 * time.Second)
		runtime.GC() // 强制垃圾回收

		// 2. NATS测试
		t.Logf("🔵 NATS Basic Performance Test...")
		natsMetrics := runComprehensiveNATSTest(t, scenario.name, scenario.messages, scenario.timeout)
		allResults["NATS"] = append(allResults["NATS"], natsMetrics)

		// 回合分析
		analyzeRoundResults(t, scenario.name, kafkaMetrics, natsMetrics)

		// 清理间隔
		time.Sleep(5 * time.Second)
		runtime.GC()
	}

	// 生成全面报告
	generateComprehensiveReport(t, allResults)
}

// runComprehensiveKafkaTest 运行Kafka全面测试
func runComprehensiveKafkaTest(t *testing.T, pressure string, messageCount int, timeout time.Duration) *ComprehensiveMetrics {
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: fmt.Sprintf("benchmark-kafka-%s", pressure),
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
		},
		Consumer: ConsumerConfig{
			GroupID:            fmt.Sprintf("benchmark-kafka-%s-group", pressure),
			AutoOffsetReset:    "earliest",
			SessionTimeout:     6 * time.Second,
			HeartbeatInterval:  2 * time.Second,
			MaxProcessingTime:  5 * time.Second,
			FetchMaxWait:       100 * time.Millisecond,
			MaxPollRecords:     1000,
			EnableAutoCommit:   true,
			AutoCommitInterval: 500 * time.Millisecond,
		},
	}

	eventBus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("Kafka failed: %v", err)
	}
	defer eventBus.Close()

	// 设置预订阅
	kafkaBus := eventBus.(*kafkaEventBus)
	testTopic := fmt.Sprintf("benchmark.kafka.%s.test", pressure)
	kafkaBus.allPossibleTopics = []string{testTopic}

	return runComprehensiveTest(t, eventBus, "Kafka", pressure, messageCount, timeout, testTopic)
}

// runComprehensiveNATSTest 运行NATS全面测试
func runComprehensiveNATSTest(t *testing.T, pressure string, messageCount int, timeout time.Duration) *ComprehensiveMetrics {
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("benchmark-nats-%s", pressure),
		MaxReconnects:       3,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: false, // 使用基本NATS避免配置问题
		},
	}

	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Logf("❌ NATS failed: %v", err)
		return &ComprehensiveMetrics{
			System:      "NATS",
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
			Errors:      int64(messageCount),
		}
	}
	defer eventBus.Close()

	testTopic := fmt.Sprintf("benchmark.nats.%s.test", pressure)
	return runComprehensiveTest(t, eventBus, "NATS", pressure, messageCount, timeout, testTopic)
}

// runComprehensiveTest 运行全面测试
func runComprehensiveTest(t *testing.T, eventBus EventBus, system, pressure string, messageCount int, timeout time.Duration, topic string) *ComprehensiveMetrics {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 初始化指标
	metrics := &ComprehensiveMetrics{
		System:    system,
		Pressure:  pressure,
		Messages:  messageCount,
		StartTime: time.Now(),
	}

	// 记录初始资源状态
	metrics.InitialGoroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.InitialMemoryMB = float64(m.Alloc) / 1024 / 1024

	// 消息计数器和延迟记录
	var receivedCount int64
	var errors int64
	var firstMessageTime time.Time
	latencies := make([]time.Duration, 0, messageCount)
	var totalBytes int64

	// 消息处理器
	handler := func(ctx context.Context, message []byte) error {
		receiveTime := time.Now()
		count := atomic.AddInt64(&receivedCount, 1)
		atomic.AddInt64(&totalBytes, int64(len(message)))

		if count == 1 {
			firstMessageTime = receiveTime
		}

		// 解析消息获取发送时间
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			if sendTimeNano, ok := msg["sendTime"].(float64); ok {
				sendTime := time.Unix(0, int64(sendTimeNano))
				latency := receiveTime.Sub(sendTime)
				latencies = append(latencies, latency)
			}
		}

		return nil
	}

	// 订阅
	err := eventBus.Subscribe(ctx, topic, handler)
	if err != nil {
		t.Logf("❌ %s subscribe failed: %v", system, err)
		metrics.Errors = int64(messageCount)
		return metrics
	}

	// 预热
	time.Sleep(3 * time.Second)

	t.Logf("⚡ %s sending %d messages...", system, messageCount)

	// 发送消息并记录性能
	sendStart := time.Now()
	var sentBytes int64

	for i := 0; i < messageCount; i++ {
		sendTime := time.Now()
		message := map[string]interface{}{
			"id":       fmt.Sprintf("%s-%s-%d", system, pressure, i),
			"content":  fmt.Sprintf("%s %s benchmark message %d", system, pressure, i),
			"sendTime": sendTime.UnixNano(),
			"index":    i,
			"system":   system,
			"pressure": pressure,
		}

		messageBytes, _ := json.Marshal(message)
		atomic.AddInt64(&sentBytes, int64(len(messageBytes)))

		err := eventBus.Publish(ctx, topic, messageBytes)
		if err != nil {
			atomic.AddInt64(&errors, 1)
		} else {
			atomic.AddInt64(&metrics.Sent, 1)
		}
	}

	sendDuration := time.Since(sendStart)
	metrics.SendRate = float64(messageCount) / sendDuration.Seconds()
	metrics.BytesSent = sentBytes

	t.Logf("📤 %s sent %d messages in %.2fs (%.1f msg/s)",
		system, messageCount, sendDuration.Seconds(), metrics.SendRate)

	// 等待处理完成
	waitTime := 15 * time.Second
	t.Logf("⏳ %s waiting %.0fs for processing...", system, waitTime.Seconds())
	time.Sleep(waitTime)

	// 记录结束时间和最终状态
	metrics.EndTime = time.Now()
	metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)
	metrics.Received = atomic.LoadInt64(&receivedCount)
	metrics.Errors = atomic.LoadInt64(&errors)
	metrics.BytesReceived = atomic.LoadInt64(&totalBytes)

	// 计算成功率和吞吐量
	metrics.SuccessRate = float64(metrics.Received) / float64(messageCount) * 100
	metrics.Throughput = float64(metrics.Received) / metrics.Duration.Seconds()

	// 计算延迟指标
	if !firstMessageTime.IsZero() {
		metrics.FirstLatency = firstMessageTime.Sub(sendStart)
	}

	if len(latencies) > 0 {
		// 计算平均延迟
		var totalLatency time.Duration
		for _, lat := range latencies {
			totalLatency += lat
		}
		metrics.AverageLatency = totalLatency / time.Duration(len(latencies))

		// 计算P95和P99延迟（简化版本）
		if len(latencies) >= 20 {
			p95Index := int(float64(len(latencies)) * 0.95)
			p99Index := int(float64(len(latencies)) * 0.99)
			metrics.P95Latency = latencies[p95Index]
			metrics.P99Latency = latencies[p99Index]
		}
	}

	// 记录峰值资源使用
	metrics.PeakGoroutines = runtime.NumGoroutine()
	metrics.GoroutineDelta = metrics.PeakGoroutines - metrics.InitialGoroutines

	runtime.ReadMemStats(&m)
	metrics.PeakMemoryMB = float64(m.Alloc) / 1024 / 1024
	metrics.MemoryDeltaMB = metrics.PeakMemoryMB - metrics.InitialMemoryMB

	// 计算网络延迟（首消息延迟作为网络延迟的近似）
	metrics.NetworkLatency = metrics.FirstLatency

	t.Logf("📊 %s Results: %d/%d (%.1f%%), %.1f msg/s throughput, %v first latency",
		system, metrics.Received, messageCount, metrics.SuccessRate,
		metrics.Throughput, metrics.FirstLatency)

	return metrics
}

// analyzeRoundResults 分析回合结果
func analyzeRoundResults(t *testing.T, pressure string, kafka, nats *ComprehensiveMetrics) {
	t.Logf("\n📈 %s Load Analysis:", pressure)

	// 性能对比
	t.Logf("🔴 Kafka Performance:")
	t.Logf("   📊 Success: %.1f%% (%d/%d)", kafka.SuccessRate, kafka.Received, kafka.Messages)
	t.Logf("   🚀 Send Rate: %.1f msg/s", kafka.SendRate)
	t.Logf("   📈 Throughput: %.1f msg/s", kafka.Throughput)
	t.Logf("   ⏱️  First Latency: %v", kafka.FirstLatency)
	t.Logf("   📊 Avg Latency: %v", kafka.AverageLatency)
	t.Logf("   🔧 Goroutines: %d (+%d)", kafka.PeakGoroutines, kafka.GoroutineDelta)
	t.Logf("   💾 Memory: %.2f MB (+%.2f MB)", kafka.PeakMemoryMB, kafka.MemoryDeltaMB)
	t.Logf("   🌐 Network: %.2f MB sent, %.2f MB received",
		float64(kafka.BytesSent)/1024/1024, float64(kafka.BytesReceived)/1024/1024)

	t.Logf("🔵 NATS Performance:")
	t.Logf("   📊 Success: %.1f%% (%d/%d)", nats.SuccessRate, nats.Received, nats.Messages)
	t.Logf("   🚀 Send Rate: %.1f msg/s", nats.SendRate)
	t.Logf("   📈 Throughput: %.1f msg/s", nats.Throughput)
	t.Logf("   ⏱️  First Latency: %v", nats.FirstLatency)
	t.Logf("   📊 Avg Latency: %v", nats.AverageLatency)
	t.Logf("   🔧 Goroutines: %d (+%d)", nats.PeakGoroutines, nats.GoroutineDelta)
	t.Logf("   💾 Memory: %.2f MB (+%.2f MB)", nats.PeakMemoryMB, nats.MemoryDeltaMB)
	t.Logf("   🌐 Network: %.2f MB sent, %.2f MB received",
		float64(nats.BytesSent)/1024/1024, float64(nats.BytesReceived)/1024/1024)

	// 直接对比
	t.Logf("⚔️ Direct Comparison:")

	// 成功率对比
	if kafka.SuccessRate > nats.SuccessRate {
		t.Logf("   ✅ Kafka wins in reliability: %.1f%% vs %.1f%%", kafka.SuccessRate, nats.SuccessRate)
	} else if nats.SuccessRate > kafka.SuccessRate {
		t.Logf("   ✅ NATS wins in reliability: %.1f%% vs %.1f%%", nats.SuccessRate, kafka.SuccessRate)
	} else {
		t.Logf("   🤝 Tie in reliability: %.1f%%", kafka.SuccessRate)
	}

	// 吞吐量对比
	throughputRatio := nats.Throughput / kafka.Throughput
	if throughputRatio > 1.1 {
		t.Logf("   ✅ NATS wins in throughput: %.1fx faster (%.1f vs %.1f msg/s)",
			throughputRatio, nats.Throughput, kafka.Throughput)
	} else if throughputRatio < 0.9 {
		t.Logf("   ✅ Kafka wins in throughput: %.1fx faster (%.1f vs %.1f msg/s)",
			1/throughputRatio, kafka.Throughput, nats.Throughput)
	} else {
		t.Logf("   🤝 Similar throughput: NATS %.1f vs Kafka %.1f msg/s",
			nats.Throughput, kafka.Throughput)
	}

	// 延迟对比
	if kafka.FirstLatency > 0 && nats.FirstLatency > 0 {
		latencyRatio := float64(kafka.FirstLatency.Nanoseconds()) / float64(nats.FirstLatency.Nanoseconds())
		if latencyRatio > 1.5 {
			t.Logf("   ✅ NATS wins in latency: %.1fx faster (%v vs %v)",
				latencyRatio, nats.FirstLatency, kafka.FirstLatency)
		} else if latencyRatio < 0.67 {
			t.Logf("   ✅ Kafka wins in latency: %.1fx faster (%v vs %v)",
				1/latencyRatio, kafka.FirstLatency, nats.FirstLatency)
		} else {
			t.Logf("   🤝 Similar latency: NATS %v vs Kafka %v",
				nats.FirstLatency, kafka.FirstLatency)
		}
	}

	// 资源效率对比
	memoryRatio := kafka.MemoryDeltaMB / nats.MemoryDeltaMB
	if memoryRatio > 1.2 && nats.MemoryDeltaMB > 0 {
		t.Logf("   ✅ NATS wins in memory efficiency: %.1fx less memory (%.2f vs %.2f MB)",
			memoryRatio, nats.MemoryDeltaMB, kafka.MemoryDeltaMB)
	} else if memoryRatio < 0.8 && kafka.MemoryDeltaMB > 0 {
		t.Logf("   ✅ Kafka wins in memory efficiency: %.1fx less memory (%.2f vs %.2f MB)",
			1/memoryRatio, kafka.MemoryDeltaMB, nats.MemoryDeltaMB)
	}

	goroutineRatio := float64(kafka.GoroutineDelta) / float64(nats.GoroutineDelta)
	if goroutineRatio > 1.2 && nats.GoroutineDelta > 0 {
		t.Logf("   ✅ NATS wins in goroutine efficiency: %.1fx fewer goroutines (%d vs %d)",
			goroutineRatio, nats.GoroutineDelta, kafka.GoroutineDelta)
	} else if goroutineRatio < 0.8 && kafka.GoroutineDelta > 0 {
		t.Logf("   ✅ Kafka wins in goroutine efficiency: %.1fx fewer goroutines (%d vs %d)",
			1/goroutineRatio, kafka.GoroutineDelta, nats.GoroutineDelta)
	}
}

// generateComprehensiveReport 生成全面报告
func generateComprehensiveReport(t *testing.T, allResults map[string][]*ComprehensiveMetrics) {
	t.Logf("\n🏆 ===== COMPREHENSIVE PERFORMANCE BENCHMARK REPORT =====")

	// 1. 详细性能表格
	generatePerformanceTable(t, allResults)

	// 2. 资源占用分析
	generateResourceAnalysis(t, allResults)

	// 3. 性能趋势分析
	generateTrendAnalysis(t, allResults)

	// 4. 综合评分和推荐
	generateFinalRecommendations(t, allResults)
}

// generatePerformanceTable 生成性能表格
func generatePerformanceTable(t *testing.T, allResults map[string][]*ComprehensiveMetrics) {
	t.Logf("\n📊 Detailed Performance Metrics Table:")
	t.Logf("%-10s | %-6s | %-8s | %-10s | %-12s | %-10s | %-10s | %-12s",
		"Load", "System", "Messages", "Success", "Send Rate", "Throughput", "Latency", "Avg Latency")
	t.Logf("%-10s-+-%-6s-+-%-8s-+-%-10s-+-%-12s-+-%-10s-+-%-10s-+-%-12s",
		"----------", "------", "--------", "----------", "------------", "----------", "----------", "------------")

	loads := []string{"Light", "Medium", "Heavy", "Extreme"}
	systems := []string{"Kafka", "NATS"}

	for i, load := range loads {
		for _, system := range systems {
			if i < len(allResults[system]) {
				result := allResults[system][i]
				t.Logf("%-10s | %-6s | %8d | %9.1f%% | %10.1f/s | %8.1f/s | %8.1fms | %10.1fms",
					load, system, result.Messages, result.SuccessRate,
					result.SendRate, result.Throughput,
					float64(result.FirstLatency.Nanoseconds())/1000000,
					float64(result.AverageLatency.Nanoseconds())/1000000)
			}
		}
		if i < len(loads)-1 {
			t.Logf("%-10s-+-%-6s-+-%-8s-+-%-10s-+-%-12s-+-%-10s-+-%-10s-+-%-12s",
				"", "", "", "", "", "", "", "")
		}
	}
}

// generateResourceAnalysis 生成资源占用分析
func generateResourceAnalysis(t *testing.T, allResults map[string][]*ComprehensiveMetrics) {
	t.Logf("\n💾 Resource Usage Analysis:")
	t.Logf("%-10s | %-6s | %-12s | %-12s | %-15s | %-15s",
		"Load", "System", "Goroutines", "Memory(MB)", "Network Sent", "Network Recv")
	t.Logf("%-10s-+-%-6s-+-%-12s-+-%-12s-+-%-15s-+-%-15s",
		"----------", "------", "------------", "------------", "---------------", "---------------")

	loads := []string{"Light", "Medium", "Heavy", "Extreme"}
	systems := []string{"Kafka", "NATS"}

	for i, load := range loads {
		for _, system := range systems {
			if i < len(allResults[system]) {
				result := allResults[system][i]
				t.Logf("%-10s | %-6s | %8d(+%d) | %8.2f(+%.2f) | %11.2f MB | %11.2f MB",
					load, system, result.PeakGoroutines, result.GoroutineDelta,
					result.PeakMemoryMB, result.MemoryDeltaMB,
					float64(result.BytesSent)/1024/1024,
					float64(result.BytesReceived)/1024/1024)
			}
		}
		if i < len(loads)-1 {
			t.Logf("%-10s-+-%-6s-+-%-12s-+-%-12s-+-%-15s-+-%-15s",
				"", "", "", "", "", "")
		}
	}

	// 资源效率分析
	t.Logf("\n🔍 Resource Efficiency Analysis:")

	kafkaResults := allResults["Kafka"]
	natsResults := allResults["NATS"]

	if len(kafkaResults) > 0 && len(natsResults) > 0 {
		// 计算平均资源使用
		var kafkaAvgMem, natsAvgMem float64
		var kafkaAvgGoroutines, natsAvgGoroutines float64

		for i := 0; i < len(kafkaResults) && i < len(natsResults); i++ {
			kafkaAvgMem += kafkaResults[i].MemoryDeltaMB
			natsAvgMem += natsResults[i].MemoryDeltaMB
			kafkaAvgGoroutines += float64(kafkaResults[i].GoroutineDelta)
			natsAvgGoroutines += float64(natsResults[i].GoroutineDelta)
		}

		count := float64(len(kafkaResults))
		kafkaAvgMem /= count
		natsAvgMem /= count
		kafkaAvgGoroutines /= count
		natsAvgGoroutines /= count

		t.Logf("📊 Average Resource Usage:")
		t.Logf("   🔴 Kafka: %.2f MB memory, %.1f goroutines", kafkaAvgMem, kafkaAvgGoroutines)
		t.Logf("   🔵 NATS: %.2f MB memory, %.1f goroutines", natsAvgMem, natsAvgGoroutines)

		// 效率对比
		if natsAvgMem > 0 && kafkaAvgMem > 0 {
			memEfficiency := kafkaAvgMem / natsAvgMem
			if memEfficiency > 1.2 {
				t.Logf("   ✅ NATS is %.1fx more memory efficient", memEfficiency)
			} else if memEfficiency < 0.8 {
				t.Logf("   ✅ Kafka is %.1fx more memory efficient", 1/memEfficiency)
			} else {
				t.Logf("   🤝 Similar memory efficiency")
			}
		}

		if natsAvgGoroutines > 0 && kafkaAvgGoroutines > 0 {
			goroutineEfficiency := kafkaAvgGoroutines / natsAvgGoroutines
			if goroutineEfficiency > 1.2 {
				t.Logf("   ✅ NATS uses %.1fx fewer goroutines", goroutineEfficiency)
			} else if goroutineEfficiency < 0.8 {
				t.Logf("   ✅ Kafka uses %.1fx fewer goroutines", 1/goroutineEfficiency)
			} else {
				t.Logf("   🤝 Similar goroutine usage")
			}
		}
	}
}

// generateTrendAnalysis 生成性能趋势分析
func generateTrendAnalysis(t *testing.T, allResults map[string][]*ComprehensiveMetrics) {
	t.Logf("\n📈 Performance Trend Analysis:")

	loads := []string{"Light", "Medium", "Heavy", "Extreme"}
	systems := []string{"Kafka", "NATS"}

	for _, system := range systems {
		results := allResults[system]
		if len(results) < 2 {
			continue
		}

		t.Logf("🔄 %s Performance Scaling:", system)

		// 分析吞吐量趋势
		t.Logf("   📊 Throughput Scaling:")
		for i, load := range loads {
			if i < len(results) {
				result := results[i]
				efficiency := result.Throughput / float64(result.Messages) * 100
				t.Logf("     %s: %.1f msg/s (%.2f%% efficiency)",
					load, result.Throughput, efficiency)
			}
		}

		// 分析延迟趋势
		t.Logf("   ⏱️  Latency Scaling:")
		for i, load := range loads {
			if i < len(results) {
				result := results[i]
				t.Logf("     %s: First=%.1fms, Avg=%.1fms",
					load,
					float64(result.FirstLatency.Nanoseconds())/1000000,
					float64(result.AverageLatency.Nanoseconds())/1000000)
			}
		}

		// 分析资源扩展性
		t.Logf("   🔧 Resource Scaling:")
		for i, load := range loads {
			if i < len(results) {
				result := results[i]
				memPerMsg := result.MemoryDeltaMB / float64(result.Messages) * 1024 // KB per message
				goroutinePerMsg := float64(result.GoroutineDelta) / float64(result.Messages)
				t.Logf("     %s: %.2f KB/msg memory, %.4f goroutines/msg",
					load, memPerMsg, goroutinePerMsg)
			}
		}
	}

	// 扩展性对比
	t.Logf("\n🚀 Scalability Comparison:")
	kafkaResults := allResults["Kafka"]
	natsResults := allResults["NATS"]

	if len(kafkaResults) >= 2 && len(natsResults) >= 2 {
		// 比较从轻负载到重负载的性能变化
		kafkaLightThroughput := kafkaResults[0].Throughput
		kafkaHeavyThroughput := kafkaResults[len(kafkaResults)-1].Throughput
		kafkaScaling := kafkaHeavyThroughput / kafkaLightThroughput

		natsLightThroughput := natsResults[0].Throughput
		natsHeavyThroughput := natsResults[len(natsResults)-1].Throughput
		natsScaling := natsHeavyThroughput / natsLightThroughput

		t.Logf("   📊 Throughput Scaling Factor:")
		t.Logf("     🔴 Kafka: %.2fx (%.1f → %.1f msg/s)",
			kafkaScaling, kafkaLightThroughput, kafkaHeavyThroughput)
		t.Logf("     🔵 NATS: %.2fx (%.1f → %.1f msg/s)",
			natsScaling, natsLightThroughput, natsHeavyThroughput)

		if natsScaling > kafkaScaling*1.1 {
			t.Logf("   ✅ NATS scales better under load")
		} else if kafkaScaling > natsScaling*1.1 {
			t.Logf("   ✅ Kafka scales better under load")
		} else {
			t.Logf("   🤝 Similar scaling characteristics")
		}
	}
}

// generateFinalRecommendations 生成最终推荐
func generateFinalRecommendations(t *testing.T, allResults map[string][]*ComprehensiveMetrics) {
	t.Logf("\n🏆 Final Performance Evaluation & Recommendations:")

	kafkaResults := allResults["Kafka"]
	natsResults := allResults["NATS"]

	if len(kafkaResults) == 0 || len(natsResults) == 0 {
		t.Logf("❌ Insufficient data for comprehensive evaluation")
		return
	}

	// 计算综合评分
	kafkaScore := calculateOverallScore(kafkaResults)
	natsScore := calculateOverallScore(natsResults)

	t.Logf("📊 Overall Performance Scores:")
	t.Logf("   🔴 Kafka: %.1f/100", kafkaScore)
	t.Logf("   🔵 NATS: %.1f/100", natsScore)

	// 分类评分
	t.Logf("\n📈 Category Breakdown:")

	// 可靠性评分
	kafkaReliability := calculateReliabilityScore(kafkaResults)
	natsReliability := calculateReliabilityScore(natsResults)
	t.Logf("   🛡️  Reliability:")
	t.Logf("     🔴 Kafka: %.1f/25", kafkaReliability)
	t.Logf("     🔵 NATS: %.1f/25", natsReliability)

	// 性能评分
	kafkaPerformance := calculatePerformanceScore(kafkaResults)
	natsPerformance := calculatePerformanceScore(natsResults)
	t.Logf("   🚀 Performance:")
	t.Logf("     🔴 Kafka: %.1f/35", kafkaPerformance)
	t.Logf("     🔵 NATS: %.1f/35", natsPerformance)

	// 资源效率评分
	kafkaEfficiency := calculateEfficiencyScore(kafkaResults)
	natsEfficiency := calculateEfficiencyScore(natsResults)
	t.Logf("   💾 Resource Efficiency:")
	t.Logf("     🔴 Kafka: %.1f/25", kafkaEfficiency)
	t.Logf("     🔵 NATS: %.1f/25", natsEfficiency)

	// 扩展性评分
	kafkaScalability := calculateScalabilityScore(kafkaResults)
	natsScalability := calculateScalabilityScore(natsResults)
	t.Logf("   📊 Scalability:")
	t.Logf("     🔴 Kafka: %.1f/15", kafkaScalability)
	t.Logf("     🔵 NATS: %.1f/15", natsScalability)

	// 宣布获胜者
	t.Logf("\n🏆 FINAL VERDICT:")
	if kafkaScore > natsScore {
		t.Logf("   🥇 WINNER: 🔴 KAFKA (RedPanda)")
		t.Logf("   📊 Score: %.1f vs %.1f", kafkaScore, natsScore)
		t.Logf("   🎉 Kafka dominates with superior overall performance!")
	} else if natsScore > kafkaScore {
		t.Logf("   🥇 WINNER: 🔵 NATS")
		t.Logf("   📊 Score: %.1f vs %.1f", natsScore, kafkaScore)
		t.Logf("   🎉 NATS conquers with exceptional performance!")
	} else {
		t.Logf("   🤝 RESULT: TIE")
		t.Logf("   📊 Score: %.1f vs %.1f", kafkaScore, natsScore)
		t.Logf("   🎉 Both systems are equally matched!")
	}

	// 使用场景推荐
	t.Logf("\n💡 Use Case Recommendations:")

	t.Logf("🔴 Choose Kafka (RedPanda) when:")
	t.Logf("   ✅ You need enterprise-grade reliability (%.1f%% avg success rate)",
		calculateAverageSuccessRate(kafkaResults))
	t.Logf("   ✅ You require mature ecosystem and tooling")
	t.Logf("   ✅ You have complex data processing pipelines")
	t.Logf("   ✅ You need guaranteed message persistence")
	t.Logf("   ✅ You're building data-intensive applications")

	t.Logf("🔵 Choose NATS when:")
	t.Logf("   ✅ You need maximum performance (%.1f msg/s avg throughput)",
		calculateAverageThroughput(natsResults))
	t.Logf("   ✅ You require ultra-low latency (%.1fms avg latency)",
		calculateAverageLatency(natsResults))
	t.Logf("   ✅ You want simple deployment and operations")
	t.Logf("   ✅ You're building real-time applications")
	t.Logf("   ✅ You need efficient resource utilization")

	// 性能基准建议
	t.Logf("\n📋 Performance Benchmarks for Your Environment:")
	t.Logf("🔴 Kafka Expected Performance:")
	t.Logf("   📊 Throughput: %.1f-%.1f msg/s",
		getMinThroughput(kafkaResults), getMaxThroughput(kafkaResults))
	t.Logf("   ⏱️  Latency: %.1f-%.1fms",
		getMinLatency(kafkaResults), getMaxLatency(kafkaResults))
	t.Logf("   💾 Memory: %.2f-%.2f MB per test",
		getMinMemory(kafkaResults), getMaxMemory(kafkaResults))

	t.Logf("🔵 NATS Expected Performance:")
	t.Logf("   📊 Throughput: %.1f-%.1f msg/s",
		getMinThroughput(natsResults), getMaxThroughput(natsResults))
	t.Logf("   ⏱️  Latency: %.1f-%.1fms",
		getMinLatency(natsResults), getMaxLatency(natsResults))
	t.Logf("   💾 Memory: %.2f-%.2f MB per test",
		getMinMemory(natsResults), getMaxMemory(natsResults))

	t.Logf("\n🎯 COMPREHENSIVE BENCHMARK COMPLETED!")
	t.Logf("📊 Total tests run: %d scenarios × 2 systems = %d tests",
		len(kafkaResults), len(kafkaResults)*2)
	t.Logf("⏱️  Total test duration: ~%.1f minutes",
		float64(len(kafkaResults)*4*60)/60) // 估算
	t.Logf("🔬 Metrics collected: Performance, Reliability, Resource Usage, Scalability")
}
