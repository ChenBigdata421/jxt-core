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

// JetStreamPerformanceMetrics JetStream性能指标
type JetStreamPerformanceMetrics struct {
	System            string
	StorageMode       string // "memory", "file"
	Pressure          string
	Messages          int
	Sent              int64
	Received          int64
	Errors            int64
	SuccessRate       float64
	SendRate          float64
	Throughput        float64
	FirstLatency      time.Duration
	AverageLatency    time.Duration
	P95Latency        time.Duration
	P99Latency        time.Duration
	InitialMemoryMB   float64
	PeakMemoryMB      float64
	MemoryDeltaMB     float64
	InitialGoroutines int
	PeakGoroutines    int
	GoroutineDelta    int
	Duration          time.Duration
	BytesSent         int64
	BytesReceived     int64
}

// TestRealJetStreamPerformance 真正的JetStream性能测试
func TestRealJetStreamPerformance(t *testing.T) {
	t.Logf("🚀 REAL NATS JETSTREAM PERFORMANCE TEST")
	t.Logf("💾 Testing Memory vs Disk persistence with proper EventBus configuration")

	// 测试场景
	scenarios := []struct {
		name     string
		messages int
		timeout  time.Duration
	}{
		{"Light", 300, 60 * time.Second},
		{"Medium", 800, 90 * time.Second},
		{"Heavy", 1500, 120 * time.Second},
		{"Extreme", 2500, 150 * time.Second},
	}

	allResults := make(map[string][]*JetStreamPerformanceMetrics)
	allResults["JetStream-Memory"] = make([]*JetStreamPerformanceMetrics, 0)
	allResults["JetStream-Disk"] = make([]*JetStreamPerformanceMetrics, 0)
	allResults["Kafka-Disk"] = make([]*JetStreamPerformanceMetrics, 0)

	for _, scenario := range scenarios {
		t.Logf("\n🎯 ===== %s Load JetStream Test (%d messages) =====", scenario.name, scenario.messages)

		// 1. JetStream Memory持久化
		t.Logf("🟡 NATS JetStream Memory Persistence...")
		memoryResult := testJetStreamMemoryPersistence(t, scenario.name, scenario.messages, scenario.timeout)
		allResults["JetStream-Memory"] = append(allResults["JetStream-Memory"], memoryResult)

		// 清理间隔
		time.Sleep(5 * time.Second)
		runtime.GC()

		// 2. JetStream Disk持久化
		t.Logf("🟠 NATS JetStream Disk Persistence...")
		diskResult := testJetStreamDiskPersistence(t, scenario.name, scenario.messages, scenario.timeout)
		allResults["JetStream-Disk"] = append(allResults["JetStream-Disk"], diskResult)

		// 清理间隔
		time.Sleep(5 * time.Second)
		runtime.GC()

		// 3. Kafka对比
		t.Logf("🔴 Kafka Disk Persistence...")
		kafkaResult := testKafkaForComparison(t, scenario.name, scenario.messages, scenario.timeout)
		allResults["Kafka-Disk"] = append(allResults["Kafka-Disk"], kafkaResult)

		// 分析回合结果
		analyzeJetStreamRound(t, scenario.name, memoryResult, diskResult, kafkaResult)

		// 清理间隔
		time.Sleep(8 * time.Second)
		runtime.GC()
	}

	// 生成最终报告
	generateJetStreamFinalReport(t, allResults)
}

// testJetStreamMemoryPersistence 测试JetStream内存持久化
func testJetStreamMemoryPersistence(t *testing.T, pressure string, messageCount int, timeout time.Duration) *JetStreamPerformanceMetrics {
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("jetstream-memory-%s-%d", pressure, time.Now().Unix()),
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
				Name:      fmt.Sprintf("MEMORY_PERF_%s_%d", pressure, time.Now().Unix()),
				Subjects:  []string{fmt.Sprintf("memory.perf.%s.>", pressure)},
				Retention: "limits",
				Storage:   "memory", // 内存持久化
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  512 * 1024 * 1024, // 512MB
				MaxMsgs:   100000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("memory_perf_%s_%d", pressure, time.Now().Unix()),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 500,
				MaxWaiting:    200,
				MaxDeliver:    3,
			},
		},
	}

	testTopic := fmt.Sprintf("memory.perf.%s.test", pressure)
	return runJetStreamPerformanceTest(t, config, "JetStream", "memory", pressure, messageCount, timeout, testTopic)
}

// testJetStreamDiskPersistence 测试JetStream磁盘持久化
func testJetStreamDiskPersistence(t *testing.T, pressure string, messageCount int, timeout time.Duration) *JetStreamPerformanceMetrics {
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("jetstream-disk-%s-%d", pressure, time.Now().Unix()),
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
				Name:      fmt.Sprintf("DISK_PERF_%s_%d", pressure, time.Now().Unix()),
				Subjects:  []string{fmt.Sprintf("disk.perf.%s.>", pressure)},
				Retention: "limits",
				Storage:   "file", // 磁盘持久化
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  512 * 1024 * 1024, // 512MB
				MaxMsgs:   100000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("disk_perf_%s_%d", pressure, time.Now().Unix()),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 500,
				MaxWaiting:    200,
				MaxDeliver:    3,
			},
		},
	}

	testTopic := fmt.Sprintf("disk.perf.%s.test", pressure)
	return runJetStreamPerformanceTest(t, config, "JetStream", "file", pressure, messageCount, timeout, testTopic)
}

// testKafkaForComparison 测试Kafka作为对比
func testKafkaForComparison(t *testing.T, pressure string, messageCount int, timeout time.Duration) *JetStreamPerformanceMetrics {
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: fmt.Sprintf("jetstream-compare-kafka-%s", pressure),
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1, // 确保持久化
			Timeout:         10 * time.Second,
		},
		Consumer: ConsumerConfig{
			GroupID:            fmt.Sprintf("jetstream-compare-kafka-%s-group", pressure),
			AutoOffsetReset:    "earliest",
			SessionTimeout:     10 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  8 * time.Second,
			FetchMaxWait:       200 * time.Millisecond,
			MaxPollRecords:     2000,
			EnableAutoCommit:   true,
			AutoCommitInterval: 1000 * time.Millisecond,
		},
	}

	eventBus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("Kafka failed: %v", err)
	}
	defer eventBus.Close()

	// 设置预订阅
	kafkaBus := eventBus.(*kafkaEventBus)
	testTopic := fmt.Sprintf("jetstream.compare.kafka.%s.test", pressure)
	kafkaBus.allPossibleTopics = []string{testTopic}

	return runKafkaPerformanceTest(t, eventBus, "Kafka", "file", pressure, messageCount, timeout, testTopic)
}

// runJetStreamPerformanceTest 运行JetStream性能测试
func runJetStreamPerformanceTest(t *testing.T, config *NATSConfig, system, storageMode, pressure string, messageCount int, timeout time.Duration, topic string) *JetStreamPerformanceMetrics {
	// 初始化指标
	metrics := &JetStreamPerformanceMetrics{
		System:      system,
		StorageMode: storageMode,
		Pressure:    pressure,
		Messages:    messageCount,
	}

	// 记录初始资源状态
	metrics.InitialGoroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.InitialMemoryMB = float64(m.Alloc) / 1024 / 1024

	// 创建EventBus
	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Logf("❌ %s %s failed: %v", system, storageMode, err)
		metrics.Errors = int64(messageCount)
		return metrics
	}
	defer eventBus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startTime := time.Now()
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
	err = eventBus.Subscribe(ctx, topic, handler)
	if err != nil {
		t.Logf("❌ %s %s subscribe failed: %v", system, storageMode, err)
		metrics.Errors = int64(messageCount)
		return metrics
	}

	// 预热
	t.Logf("🔥 %s (%s) warming up for 5 seconds...", system, storageMode)
	time.Sleep(5 * time.Second)

	t.Logf("⚡ %s (%s) sending %d messages...", system, storageMode, messageCount)

	// 发送消息并记录性能
	sendStart := time.Now()
	var sentBytes int64

	for i := 0; i < messageCount; i++ {
		sendTime := time.Now()
		message := map[string]interface{}{
			"id":        fmt.Sprintf("%s-%s-%s-%d", system, storageMode, pressure, i),
			"content":   fmt.Sprintf("%s %s %s performance test message %d", system, storageMode, pressure, i),
			"sendTime":  sendTime.UnixNano(),
			"index":     i,
			"system":    system,
			"storage":   storageMode,
			"pressure":  pressure,
			"timestamp": sendTime.Format(time.RFC3339Nano),
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

	t.Logf("📤 %s (%s) sent %d messages in %.2fs (%.1f msg/s)",
		system, storageMode, messageCount, sendDuration.Seconds(), metrics.SendRate)

	// 等待处理完成
	waitTime := 20 * time.Second
	t.Logf("⏳ %s (%s) waiting %.0fs for processing...", system, storageMode, waitTime.Seconds())
	time.Sleep(waitTime)

	// 记录结束时间和最终状态
	endTime := time.Now()
	metrics.Duration = endTime.Sub(startTime)
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

		// 计算P95和P99延迟
		if len(latencies) >= 20 {
			// 简单排序取百分位
			sortedLatencies := make([]time.Duration, len(latencies))
			copy(sortedLatencies, latencies)
			// 简化版排序
			for i := 0; i < len(sortedLatencies); i++ {
				for j := i + 1; j < len(sortedLatencies); j++ {
					if sortedLatencies[i] > sortedLatencies[j] {
						sortedLatencies[i], sortedLatencies[j] = sortedLatencies[j], sortedLatencies[i]
					}
				}
			}

			p95Index := int(float64(len(sortedLatencies)) * 0.95)
			p99Index := int(float64(len(sortedLatencies)) * 0.99)
			if p95Index < len(sortedLatencies) {
				metrics.P95Latency = sortedLatencies[p95Index]
			}
			if p99Index < len(sortedLatencies) {
				metrics.P99Latency = sortedLatencies[p99Index]
			}
		}
	}

	// 记录峰值资源使用
	metrics.PeakGoroutines = runtime.NumGoroutine()
	metrics.GoroutineDelta = metrics.PeakGoroutines - metrics.InitialGoroutines

	runtime.ReadMemStats(&m)
	metrics.PeakMemoryMB = float64(m.Alloc) / 1024 / 1024
	metrics.MemoryDeltaMB = metrics.PeakMemoryMB - metrics.InitialMemoryMB

	t.Logf("📊 %s (%s) Results: %d/%d (%.1f%%), %.1f msg/s throughput, %v first latency",
		system, storageMode, metrics.Received, messageCount, metrics.SuccessRate,
		metrics.Throughput, metrics.FirstLatency)

	return metrics
}

// runKafkaPerformanceTest 运行Kafka性能测试
func runKafkaPerformanceTest(t *testing.T, eventBus EventBus, system, storageMode, pressure string, messageCount int, timeout time.Duration, topic string) *JetStreamPerformanceMetrics {
	// 初始化指标
	metrics := &JetStreamPerformanceMetrics{
		System:      system,
		StorageMode: storageMode,
		Pressure:    pressure,
		Messages:    messageCount,
	}

	// 记录初始资源状态
	metrics.InitialGoroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.InitialMemoryMB = float64(m.Alloc) / 1024 / 1024

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startTime := time.Now()
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
		t.Logf("❌ %s %s subscribe failed: %v", system, storageMode, err)
		metrics.Errors = int64(messageCount)
		return metrics
	}

	// 预热
	t.Logf("🔥 %s (%s) warming up for 5 seconds...", system, storageMode)
	time.Sleep(5 * time.Second)

	t.Logf("⚡ %s (%s) sending %d messages...", system, storageMode, messageCount)

	// 发送消息并记录性能
	sendStart := time.Now()
	var sentBytes int64

	for i := 0; i < messageCount; i++ {
		sendTime := time.Now()
		message := map[string]interface{}{
			"id":        fmt.Sprintf("%s-%s-%s-%d", system, storageMode, pressure, i),
			"content":   fmt.Sprintf("%s %s %s performance test message %d", system, storageMode, pressure, i),
			"sendTime":  sendTime.UnixNano(),
			"index":     i,
			"system":    system,
			"storage":   storageMode,
			"pressure":  pressure,
			"timestamp": sendTime.Format(time.RFC3339Nano),
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

	t.Logf("📤 %s (%s) sent %d messages in %.2fs (%.1f msg/s)",
		system, storageMode, messageCount, sendDuration.Seconds(), metrics.SendRate)

	// 等待处理完成
	waitTime := 20 * time.Second
	t.Logf("⏳ %s (%s) waiting %.0fs for processing...", system, storageMode, waitTime.Seconds())
	time.Sleep(waitTime)

	// 记录结束时间和最终状态
	endTime := time.Now()
	metrics.Duration = endTime.Sub(startTime)
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

		// 计算P95和P99延迟
		if len(latencies) >= 20 {
			// 简单排序取百分位
			sortedLatencies := make([]time.Duration, len(latencies))
			copy(sortedLatencies, latencies)
			// 简化版排序
			for i := 0; i < len(sortedLatencies); i++ {
				for j := i + 1; j < len(sortedLatencies); j++ {
					if sortedLatencies[i] > sortedLatencies[j] {
						sortedLatencies[i], sortedLatencies[j] = sortedLatencies[j], sortedLatencies[i]
					}
				}
			}

			p95Index := int(float64(len(sortedLatencies)) * 0.95)
			p99Index := int(float64(len(sortedLatencies)) * 0.99)
			if p95Index < len(sortedLatencies) {
				metrics.P95Latency = sortedLatencies[p95Index]
			}
			if p99Index < len(sortedLatencies) {
				metrics.P99Latency = sortedLatencies[p99Index]
			}
		}
	}

	// 记录峰值资源使用
	metrics.PeakGoroutines = runtime.NumGoroutine()
	metrics.GoroutineDelta = metrics.PeakGoroutines - metrics.InitialGoroutines

	runtime.ReadMemStats(&m)
	metrics.PeakMemoryMB = float64(m.Alloc) / 1024 / 1024
	metrics.MemoryDeltaMB = metrics.PeakMemoryMB - metrics.InitialMemoryMB

	t.Logf("📊 %s (%s) Results: %d/%d (%.1f%%), %.1f msg/s throughput, %v first latency",
		system, storageMode, metrics.Received, messageCount, metrics.SuccessRate,
		metrics.Throughput, metrics.FirstLatency)

	return metrics
}

// analyzeJetStreamRound 分析单轮JetStream测试结果
func analyzeJetStreamRound(t *testing.T, pressure string, memoryResult, diskResult, kafkaResult *JetStreamPerformanceMetrics) {
	t.Logf("\n📈 %s Load JetStream Analysis:", pressure)

	// 详细结果展示
	t.Logf("🟡 JetStream Memory:")
	t.Logf("   📊 Success: %.1f%% (%d/%d)", memoryResult.SuccessRate, memoryResult.Received, memoryResult.Messages)
	t.Logf("   🚀 Send Rate: %.1f msg/s", memoryResult.SendRate)
	t.Logf("   📈 Throughput: %.1f msg/s", memoryResult.Throughput)
	t.Logf("   ⏱️  First Latency: %v", memoryResult.FirstLatency)
	t.Logf("   📊 Avg Latency: %v", memoryResult.AverageLatency)
	t.Logf("   🔧 Goroutines: %d (+%d)", memoryResult.PeakGoroutines, memoryResult.GoroutineDelta)
	t.Logf("   💾 Memory: %.2f MB (+%.2f MB)", memoryResult.PeakMemoryMB, memoryResult.MemoryDeltaMB)

	t.Logf("🟠 JetStream Disk:")
	t.Logf("   📊 Success: %.1f%% (%d/%d)", diskResult.SuccessRate, diskResult.Received, diskResult.Messages)
	t.Logf("   🚀 Send Rate: %.1f msg/s", diskResult.SendRate)
	t.Logf("   📈 Throughput: %.1f msg/s", diskResult.Throughput)
	t.Logf("   ⏱️  First Latency: %v", diskResult.FirstLatency)
	t.Logf("   📊 Avg Latency: %v", diskResult.AverageLatency)
	t.Logf("   🔧 Goroutines: %d (+%d)", diskResult.PeakGoroutines, diskResult.GoroutineDelta)
	t.Logf("   💾 Memory: %.2f MB (+%.2f MB)", diskResult.PeakMemoryMB, diskResult.MemoryDeltaMB)

	t.Logf("🔴 Kafka Disk:")
	t.Logf("   📊 Success: %.1f%% (%d/%d)", kafkaResult.SuccessRate, kafkaResult.Received, kafkaResult.Messages)
	t.Logf("   🚀 Send Rate: %.1f msg/s", kafkaResult.SendRate)
	t.Logf("   📈 Throughput: %.1f msg/s", kafkaResult.Throughput)
	t.Logf("   ⏱️  First Latency: %v", kafkaResult.FirstLatency)
	t.Logf("   📊 Avg Latency: %v", kafkaResult.AverageLatency)
	t.Logf("   🔧 Goroutines: %d (+%d)", kafkaResult.PeakGoroutines, kafkaResult.GoroutineDelta)
	t.Logf("   💾 Memory: %.2f MB (+%.2f MB)", kafkaResult.PeakMemoryMB, kafkaResult.MemoryDeltaMB)

	// 直接对比分析
	t.Logf("\n⚔️ Direct Comparison:")

	// 可靠性对比
	if memoryResult.SuccessRate >= kafkaResult.SuccessRate && diskResult.SuccessRate >= kafkaResult.SuccessRate {
		t.Logf("   ✅ JetStream wins in reliability: Memory %.1f%%, Disk %.1f%% vs Kafka %.1f%%",
			memoryResult.SuccessRate, diskResult.SuccessRate, kafkaResult.SuccessRate)
	} else if kafkaResult.SuccessRate > memoryResult.SuccessRate && kafkaResult.SuccessRate > diskResult.SuccessRate {
		t.Logf("   ✅ Kafka wins in reliability: %.1f%% vs JetStream Memory %.1f%%, Disk %.1f%%",
			kafkaResult.SuccessRate, memoryResult.SuccessRate, diskResult.SuccessRate)
	}

	// 吞吐量对比
	if memoryResult.Throughput > kafkaResult.Throughput {
		ratio := memoryResult.Throughput / kafkaResult.Throughput
		t.Logf("   ✅ JetStream Memory wins in throughput: %.1fx faster (%.1f vs %.1f msg/s)",
			ratio, memoryResult.Throughput, kafkaResult.Throughput)
	}

	if diskResult.Throughput > kafkaResult.Throughput {
		ratio := diskResult.Throughput / kafkaResult.Throughput
		t.Logf("   ✅ JetStream Disk wins in throughput: %.1fx faster (%.1f vs %.1f msg/s)",
			ratio, diskResult.Throughput, kafkaResult.Throughput)
	}

	// 延迟对比
	if memoryResult.FirstLatency > 0 && kafkaResult.FirstLatency > 0 {
		if memoryResult.FirstLatency < kafkaResult.FirstLatency {
			ratio := float64(kafkaResult.FirstLatency.Nanoseconds()) / float64(memoryResult.FirstLatency.Nanoseconds())
			t.Logf("   ✅ JetStream Memory wins in latency: %.1fx faster (%v vs %v)",
				ratio, memoryResult.FirstLatency, kafkaResult.FirstLatency)
		}
	}

	if diskResult.FirstLatency > 0 && kafkaResult.FirstLatency > 0 {
		if diskResult.FirstLatency < kafkaResult.FirstLatency {
			ratio := float64(kafkaResult.FirstLatency.Nanoseconds()) / float64(diskResult.FirstLatency.Nanoseconds())
			t.Logf("   ✅ JetStream Disk wins in latency: %.1fx faster (%v vs %v)",
				ratio, diskResult.FirstLatency, kafkaResult.FirstLatency)
		}
	}

	// 持久化模式对比
	if memoryResult.Throughput > 0 && diskResult.Throughput > 0 {
		impact := (memoryResult.Throughput - diskResult.Throughput) / memoryResult.Throughput * 100
		t.Logf("   📊 JetStream Disk persistence impact: %.1f%% throughput reduction vs Memory", impact)
	}

	// 资源效率对比
	if kafkaResult.MemoryDeltaMB > 0 && diskResult.MemoryDeltaMB > 0 {
		if kafkaResult.MemoryDeltaMB < diskResult.MemoryDeltaMB {
			ratio := diskResult.MemoryDeltaMB / kafkaResult.MemoryDeltaMB
			t.Logf("   ✅ Kafka wins in memory efficiency: %.1fx less memory (%.2f vs %.2f MB)",
				ratio, kafkaResult.MemoryDeltaMB, diskResult.MemoryDeltaMB)
		}
	}
}

// generateJetStreamFinalReport 生成JetStream最终报告
func generateJetStreamFinalReport(t *testing.T, allResults map[string][]*JetStreamPerformanceMetrics) {
	t.Logf("\n🏆 ===== EVENTBUS + NATS JETSTREAM FINAL PERFORMANCE REPORT =====")

	// 性能表格
	t.Logf("\n📊 Detailed Performance Metrics Table:")
	t.Logf("Load       | System           | Storage | Messages | Success    | Send Rate    | Throughput | First Latency | Avg Latency")
	t.Logf("-----------+------------------+---------+----------+------------+--------------+------------+---------------+-------------")

	scenarios := []string{"Light", "Medium", "Heavy", "Extreme"}
	systems := []string{"JetStream-Memory", "JetStream-Disk", "Kafka-Disk"}

	for i, scenario := range scenarios {
		for _, system := range systems {
			if i < len(allResults[system]) {
				result := allResults[system][i]
				t.Logf("%-10s | %-16s | %-7s | %8d | %9.1f%% | %11.1f/s | %9.1f/s | %12v | %10v",
					scenario, result.System, result.StorageMode, result.Messages,
					result.SuccessRate, result.SendRate, result.Throughput,
					result.FirstLatency, result.AverageLatency)
			}
		}
		if i < len(scenarios)-1 {
			t.Logf("           +                  +         +          +            +              +            +               +")
		}
	}

	// 资源使用分析
	t.Logf("\n💾 Resource Usage Analysis:")
	t.Logf("Load       | System           | Storage | Goroutines   | Memory(MB)   | Network Sent | Network Recv")
	t.Logf("-----------+------------------+---------+--------------+--------------+--------------+--------------")

	for i, scenario := range scenarios {
		for _, system := range systems {
			if i < len(allResults[system]) {
				result := allResults[system][i]
				t.Logf("%-10s | %-16s | %-7s | %8d(+%d) | %8.2f(+%.2f) | %11.2f MB | %11.2f MB",
					scenario, result.System, result.StorageMode,
					result.PeakGoroutines, result.GoroutineDelta,
					result.PeakMemoryMB, result.MemoryDeltaMB,
					float64(result.BytesSent)/1024/1024, float64(result.BytesReceived)/1024/1024)
			}
		}
		if i < len(scenarios)-1 {
			t.Logf("           +                  +         +              +              +              +")
		}
	}

	// 计算平均性能
	avgMetrics := make(map[string]*JetStreamPerformanceMetrics)
	for system, results := range allResults {
		if len(results) == 0 {
			continue
		}

		avg := &JetStreamPerformanceMetrics{
			System:      results[0].System,
			StorageMode: results[0].StorageMode,
		}

		var totalSuccess, totalThroughput, totalSendRate float64
		var totalFirstLatency, totalAvgLatency time.Duration
		var totalMemory, totalGoroutines float64
		validResults := 0

		for _, result := range results {
			if result.SuccessRate > 0 {
				totalSuccess += result.SuccessRate
				totalThroughput += result.Throughput
				totalSendRate += result.SendRate
				totalFirstLatency += result.FirstLatency
				totalAvgLatency += result.AverageLatency
				totalMemory += result.MemoryDeltaMB
				totalGoroutines += float64(result.GoroutineDelta)
				validResults++
			}
		}

		if validResults > 0 {
			avg.SuccessRate = totalSuccess / float64(validResults)
			avg.Throughput = totalThroughput / float64(validResults)
			avg.SendRate = totalSendRate / float64(validResults)
			avg.FirstLatency = totalFirstLatency / time.Duration(validResults)
			avg.AverageLatency = totalAvgLatency / time.Duration(validResults)
			avg.MemoryDeltaMB = totalMemory / float64(validResults)
			avg.GoroutineDelta = int(totalGoroutines / float64(validResults))
			avgMetrics[system] = avg
		}
	}

	// 性能对比分析
	t.Logf("\n🔍 EventBus Performance Analysis:")

	jetStreamMemory := avgMetrics["JetStream-Memory"]
	jetStreamDisk := avgMetrics["JetStream-Disk"]
	kafka := avgMetrics["Kafka-Disk"]

	if jetStreamMemory != nil && kafka != nil {
		t.Logf("📊 JetStream Memory vs Kafka:")
		if jetStreamMemory.SuccessRate > 0 && kafka.SuccessRate > 0 {
			reliabilityRatio := jetStreamMemory.SuccessRate / kafka.SuccessRate
			throughputRatio := jetStreamMemory.Throughput / kafka.Throughput
			latencyRatio := float64(kafka.FirstLatency.Nanoseconds()) / float64(jetStreamMemory.FirstLatency.Nanoseconds())

			t.Logf("   🛡️  Reliability: %.2fx (%.1f%% vs %.1f%%)", reliabilityRatio, jetStreamMemory.SuccessRate, kafka.SuccessRate)
			t.Logf("   🚀 Throughput: %.2fx (%.1f vs %.1f msg/s)", throughputRatio, jetStreamMemory.Throughput, kafka.Throughput)
			t.Logf("   ⏱️  Latency: %.2fx faster (%v vs %v)", latencyRatio, jetStreamMemory.FirstLatency, kafka.FirstLatency)
			t.Logf("   💾 Memory: %.2fx more (%.2f vs %.2f MB)", jetStreamMemory.MemoryDeltaMB/kafka.MemoryDeltaMB, jetStreamMemory.MemoryDeltaMB, kafka.MemoryDeltaMB)
		}
	}

	if jetStreamDisk != nil && kafka != nil {
		t.Logf("📊 JetStream Disk vs Kafka:")
		if jetStreamDisk.SuccessRate > 0 && kafka.SuccessRate > 0 {
			reliabilityRatio := jetStreamDisk.SuccessRate / kafka.SuccessRate
			throughputRatio := jetStreamDisk.Throughput / kafka.Throughput
			latencyRatio := float64(kafka.FirstLatency.Nanoseconds()) / float64(jetStreamDisk.FirstLatency.Nanoseconds())

			t.Logf("   🛡️  Reliability: %.2fx (%.1f%% vs %.1f%%)", reliabilityRatio, jetStreamDisk.SuccessRate, kafka.SuccessRate)
			t.Logf("   🚀 Throughput: %.2fx (%.1f vs %.1f msg/s)", throughputRatio, jetStreamDisk.Throughput, kafka.Throughput)
			t.Logf("   ⏱️  Latency: %.2fx faster (%v vs %v)", latencyRatio, jetStreamDisk.FirstLatency, kafka.FirstLatency)
			t.Logf("   💾 Memory: %.2fx more (%.2f vs %.2f MB)", jetStreamDisk.MemoryDeltaMB/kafka.MemoryDeltaMB, jetStreamDisk.MemoryDeltaMB, kafka.MemoryDeltaMB)
		}
	}

	if jetStreamMemory != nil && jetStreamDisk != nil {
		t.Logf("📊 JetStream Memory vs Disk:")
		if jetStreamMemory.SuccessRate > 0 && jetStreamDisk.SuccessRate > 0 {
			persistenceImpact := (jetStreamMemory.Throughput - jetStreamDisk.Throughput) / jetStreamMemory.Throughput * 100
			latencyImpact := (jetStreamDisk.FirstLatency.Nanoseconds() - jetStreamMemory.FirstLatency.Nanoseconds()) / jetStreamMemory.FirstLatency.Nanoseconds() * 100

			t.Logf("   📊 Disk persistence impact: %.1f%% throughput reduction", persistenceImpact)
			t.Logf("   ⏱️  Disk persistence impact: %.1f%% latency increase", float64(latencyImpact))
		}
	}

	// 最终结论
	t.Logf("\n🏆 FINAL EVENTBUS PERFORMANCE VERDICT:")

	// 确定获胜者
	var winner string
	var winnerScore float64

	if jetStreamMemory != nil && jetStreamMemory.SuccessRate > 0 {
		score := jetStreamMemory.SuccessRate*0.3 + jetStreamMemory.Throughput*0.5 + (1000.0/float64(jetStreamMemory.FirstLatency.Milliseconds()))*0.2
		if score > winnerScore {
			winnerScore = score
			winner = "🟡 EventBus + NATS JetStream Memory"
		}
	}

	if jetStreamDisk != nil && jetStreamDisk.SuccessRate > 0 {
		score := jetStreamDisk.SuccessRate*0.3 + jetStreamDisk.Throughput*0.5 + (1000.0/float64(jetStreamDisk.FirstLatency.Milliseconds()))*0.2
		if score > winnerScore {
			winnerScore = score
			winner = "🟠 EventBus + NATS JetStream Disk"
		}
	}

	if kafka != nil && kafka.SuccessRate > 0 {
		score := kafka.SuccessRate*0.3 + kafka.Throughput*0.5 + (1000.0/float64(kafka.FirstLatency.Milliseconds()))*0.2
		if score > winnerScore {
			winnerScore = score
			winner = "🔴 EventBus + Kafka"
		}
	}

	if winner != "" {
		t.Logf("🥇 WINNER: %s", winner)
		t.Logf("📊 Performance Score: %.2f", winnerScore)
	}

	// 使用场景建议
	t.Logf("\n💡 EventBus Implementation Recommendations:")

	if jetStreamMemory != nil && jetStreamMemory.SuccessRate > 80 {
		t.Logf("🟡 Choose EventBus + NATS JetStream Memory when:")
		t.Logf("   ✅ You need maximum performance (%.1f msg/s avg throughput)", jetStreamMemory.Throughput)
		t.Logf("   ✅ You require ultra-low latency (%v avg latency)", jetStreamMemory.FirstLatency)
		t.Logf("   ✅ You can accept memory-only persistence")
		t.Logf("   ✅ You're building real-time applications")
		t.Logf("   ✅ You have sufficient memory resources")
	}

	if jetStreamDisk != nil && jetStreamDisk.SuccessRate > 80 {
		t.Logf("🟠 Choose EventBus + NATS JetStream Disk when:")
		t.Logf("   ✅ You need high performance with persistence (%.1f msg/s avg throughput)", jetStreamDisk.Throughput)
		t.Logf("   ✅ You require low latency (%v avg latency)", jetStreamDisk.FirstLatency)
		t.Logf("   ✅ You need durable message storage")
		t.Logf("   ✅ You want modern cloud-native architecture")
		t.Logf("   ✅ You prefer simpler deployment than Kafka")
	}

	if kafka != nil && kafka.SuccessRate > 70 {
		t.Logf("🔴 Choose EventBus + Kafka when:")
		t.Logf("   ✅ You need enterprise-grade reliability (%.1f%% avg success rate)", kafka.SuccessRate)
		t.Logf("   ✅ You require mature ecosystem and tooling")
		t.Logf("   ✅ You have complex data processing pipelines")
		t.Logf("   ✅ You need guaranteed message persistence")
		t.Logf("   ✅ You're building data-intensive applications")
		t.Logf("   ✅ You have existing Kafka expertise")
	}

	// 性能基准
	t.Logf("\n📋 EventBus Performance Benchmarks for Your Environment:")

	if jetStreamMemory != nil {
		t.Logf("🟡 EventBus + JetStream Memory Expected Performance:")
		t.Logf("   📊 Throughput: %.1f msg/s", jetStreamMemory.Throughput)
		t.Logf("   ⏱️  Latency: %v", jetStreamMemory.FirstLatency)
		t.Logf("   💾 Memory: %.2f MB per test", jetStreamMemory.MemoryDeltaMB)
		t.Logf("   🛡️  Reliability: %.1f%% success rate", jetStreamMemory.SuccessRate)
	}

	if jetStreamDisk != nil {
		t.Logf("🟠 EventBus + JetStream Disk Expected Performance:")
		t.Logf("   📊 Throughput: %.1f msg/s", jetStreamDisk.Throughput)
		t.Logf("   ⏱️  Latency: %v", jetStreamDisk.FirstLatency)
		t.Logf("   💾 Memory: %.2f MB per test", jetStreamDisk.MemoryDeltaMB)
		t.Logf("   🛡️  Reliability: %.1f%% success rate", jetStreamDisk.SuccessRate)
	}

	if kafka != nil {
		t.Logf("🔴 EventBus + Kafka Expected Performance:")
		t.Logf("   📊 Throughput: %.1f msg/s", kafka.Throughput)
		t.Logf("   ⏱️  Latency: %v", kafka.FirstLatency)
		t.Logf("   💾 Memory: %.2f MB per test", kafka.MemoryDeltaMB)
		t.Logf("   🛡️  Reliability: %.1f%% success rate", kafka.SuccessRate)
	}

	t.Logf("\n🎯 EVENTBUS + NATS JETSTREAM PERFORMANCE TEST COMPLETED!")
	t.Logf("📊 Total tests run: 4 scenarios × 3 systems = 12 tests")
	t.Logf("🔬 Metrics collected: Performance, Reliability, Resource Usage, Persistence Impact")
	t.Logf("💾 Persistence modes tested: Memory, Disk")
}
