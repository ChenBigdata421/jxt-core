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

// PersistenceTestMetrics 持久化测试指标
type PersistenceTestMetrics struct {
	System      string
	Mode        string // "Memory", "Disk", "Kafka"
	Pressure    string
	Messages    int
	Received    int64
	Errors      int64
	Duration    time.Duration
	SendRate    float64
	SuccessRate float64
	Throughput  float64
	FirstLatency time.Duration
	MemoryMB    float64
}

// TestNATSPersistenceVsKafka NATS持久化模式 vs Kafka对比测试
func TestNATSPersistenceVsKafka(t *testing.T) {
	t.Logf("💾 NATS JETSTREAM PERSISTENCE vs KAFKA BATTLE!")
	t.Logf("🔵 NATS Memory Mode vs 🟡 NATS Disk Mode vs 🔴 Kafka")

	// 测试场景
	battles := []struct {
		name     string
		messages int
		timeout  time.Duration
	}{
		{"Low", 300, 50 * time.Second},
		{"Medium", 800, 80 * time.Second},
		{"High", 1500, 120 * time.Second},
	}

	allResults := make(map[string][]*PersistenceTestMetrics)
	allResults["Memory"] = make([]*PersistenceTestMetrics, 0)
	allResults["Disk"] = make([]*PersistenceTestMetrics, 0)
	allResults["Kafka"] = make([]*PersistenceTestMetrics, 0)

	for _, battle := range battles {
		t.Logf("\n💾 ===== %s Pressure Battle (%d messages) =====", battle.name, battle.messages)
		
		// 1. NATS Memory模式
		t.Logf("🔵 NATS JetStream (Memory) Round...")
		memoryMetrics := testNATSWithPersistence(t, battle.name, battle.messages, battle.timeout, "memory")
		allResults["Memory"] = append(allResults["Memory"], memoryMetrics)
		
		time.Sleep(3 * time.Second)
		
		// 2. NATS Disk模式
		t.Logf("🟡 NATS JetStream (Disk) Round...")
		diskMetrics := testNATSWithPersistence(t, battle.name, battle.messages, battle.timeout, "file")
		allResults["Disk"] = append(allResults["Disk"], diskMetrics)
		
		time.Sleep(3 * time.Second)
		
		// 3. Kafka对比
		t.Logf("🔴 Kafka (RedPanda) Round...")
		kafkaMetrics := testKafkaForPersistence(t, battle.name, battle.messages, battle.timeout)
		allResults["Kafka"] = append(allResults["Kafka"], kafkaMetrics)
		
		// 回合结果
		winner := comparePersistenceRound(memoryMetrics, diskMetrics, kafkaMetrics)
		t.Logf("🏆 %s Winner: %s", battle.name, winner)
		
		time.Sleep(5 * time.Second)
	}

	// 最终结果分析
	analyzePersistenceResults(t, allResults)
}

// testNATSWithPersistence 测试NATS不同持久化模式
func testNATSWithPersistence(t *testing.T, pressure string, messageCount int, timeout time.Duration, storageType string) *PersistenceTestMetrics {
	// 根据存储类型配置NATS
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-%s-%s", storageType, pressure),
		MaxReconnects:       5,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 5 * time.Second,
			AckWait:        10 * time.Second,
			MaxDeliver:     3,
			Stream: StreamConfig{
				Name:      fmt.Sprintf("PERSIST_%s_%s_STREAM", storageType, pressure),
				Subjects:  []string{fmt.Sprintf("persist.%s.%s.*", storageType, pressure)},
				Retention: "limits",
				Storage:   storageType, // "memory" 或 "file"
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  256 * 1024 * 1024, // 256MB
				MaxMsgs:   50000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("persist_%s_%s_consumer", storageType, pressure),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 200,
				MaxWaiting:    100,
				MaxDeliver:    3,
			},
		},
	}

	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Logf("❌ NATS %s failed: %v", storageType, err)
		return &PersistenceTestMetrics{
			System:      "NATS",
			Mode:        storageType,
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
			Errors:      int64(messageCount),
		}
	}
	defer eventBus.Close()

	testTopic := fmt.Sprintf("persist.%s.%s.test", storageType, pressure)
	return runPersistenceTest(t, eventBus, "NATS", storageType, pressure, messageCount, timeout, testTopic)
}

// testKafkaForPersistence 测试Kafka持久化性能
func testKafkaForPersistence(t *testing.T, pressure string, messageCount int, timeout time.Duration) *PersistenceTestMetrics {
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: fmt.Sprintf("persist-kafka-%s", pressure),
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1, // 等待leader确认，确保持久化
			Timeout:         5 * time.Second,
		},
		Consumer: ConsumerConfig{
			GroupID:            fmt.Sprintf("persist-kafka-%s-group", pressure),
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
	testTopic := fmt.Sprintf("persist.kafka.%s.test", pressure)
	kafkaBus.allPossibleTopics = []string{testTopic}

	return runPersistenceTest(t, eventBus, "Kafka", "Disk", pressure, messageCount, timeout, testTopic)
}

// runPersistenceTest 运行持久化测试
func runPersistenceTest(t *testing.T, eventBus EventBus, system, mode, pressure string, messageCount int, timeout time.Duration, topic string) *PersistenceTestMetrics {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startTime := time.Now()
	var receivedCount int64
	var errors int64
	var firstMessageTime time.Time

	// 获取初始内存
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	initialMemory := m.Alloc

	// 消息处理器
	handler := func(ctx context.Context, message []byte) error {
		receiveTime := time.Now()
		count := atomic.AddInt64(&receivedCount, 1)
		
		if count == 1 {
			firstMessageTime = receiveTime
		}
		
		return nil
	}

	// 订阅
	err := eventBus.Subscribe(ctx, topic, handler)
	if err != nil {
		t.Logf("❌ %s %s subscribe failed: %v", system, mode, err)
		return &PersistenceTestMetrics{
			System:      system,
			Mode:        mode,
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
			Errors:      int64(messageCount),
		}
	}

	// 预热
	time.Sleep(3 * time.Second)

	t.Logf("⚡ %s (%s) sending %d messages...", system, mode, messageCount)

	// 发送消息
	sendStart := time.Now()
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":      fmt.Sprintf("%s-%s-%s-%d", system, mode, pressure, i),
			"content": fmt.Sprintf("%s %s %s test message %d", system, mode, pressure, i),
			"time":    time.Now().UnixNano(),
			"index":   i,
		}

		messageBytes, _ := json.Marshal(message)
		err := eventBus.Publish(ctx, topic, messageBytes)
		if err != nil {
			atomic.AddInt64(&errors, 1)
		}
	}
	sendDuration := time.Since(sendStart)
	sendRate := float64(messageCount) / sendDuration.Seconds()

	t.Logf("📤 %s (%s) sent %d messages in %.2fs (%.1f msg/s)", 
		system, mode, messageCount, sendDuration.Seconds(), sendRate)

	// 等待处理
	waitTime := 15 * time.Second
	t.Logf("⏳ %s (%s) waiting %.0fs...", system, mode, waitTime.Seconds())
	time.Sleep(waitTime)

	// 计算结果
	totalDuration := time.Since(startTime)
	finalReceived := atomic.LoadInt64(&receivedCount)
	finalErrors := atomic.LoadInt64(&errors)
	successRate := float64(finalReceived) / float64(messageCount) * 100
	throughput := float64(finalReceived) / totalDuration.Seconds()

	var firstLatency time.Duration
	if !firstMessageTime.IsZero() {
		firstLatency = firstMessageTime.Sub(sendStart)
	}

	// 获取内存使用
	runtime.ReadMemStats(&m)
	memoryMB := float64(m.Alloc-initialMemory) / 1024 / 1024

	metrics := &PersistenceTestMetrics{
		System:       system,
		Mode:         mode,
		Pressure:     pressure,
		Messages:     messageCount,
		Received:     finalReceived,
		Errors:       finalErrors,
		Duration:     totalDuration,
		SendRate:     sendRate,
		SuccessRate:  successRate,
		Throughput:   throughput,
		FirstLatency: firstLatency,
		MemoryMB:     memoryMB,
	}

	t.Logf("📊 %s (%s) Results: %d/%d (%.1f%%), %.1f msg/s, %v latency", 
		system, mode, finalReceived, messageCount, successRate, throughput, firstLatency)

	return metrics
}

// comparePersistenceRound 比较持久化回合结果
func comparePersistenceRound(memory, disk, kafka *PersistenceTestMetrics) string {
	systems := []*PersistenceTestMetrics{memory, disk, kafka}
	names := []string{"NATS(Memory)", "NATS(Disk)", "Kafka"}
	scores := make([]int, 3)

	// 成功率比较 (40%)
	maxSuccess := 0.0
	for _, s := range systems {
		if s.SuccessRate > maxSuccess {
			maxSuccess = s.SuccessRate
		}
	}
	for i, s := range systems {
		if s.SuccessRate == maxSuccess && maxSuccess > 0 {
			scores[i] += 40
		}
	}

	// 吞吐量比较 (35%)
	maxThroughput := 0.0
	for _, s := range systems {
		if s.Throughput > maxThroughput {
			maxThroughput = s.Throughput
		}
	}
	for i, s := range systems {
		if s.Throughput == maxThroughput && maxThroughput > 0 {
			scores[i] += 35
		}
	}

	// 延迟比较 (25%)
	minLatency := time.Hour
	for _, s := range systems {
		if s.FirstLatency > 0 && s.FirstLatency < minLatency {
			minLatency = s.FirstLatency
		}
	}
	for i, s := range systems {
		if s.FirstLatency == minLatency && minLatency < time.Hour {
			scores[i] += 25
		}
	}

	// 找到最高分
	maxScore := 0
	winner := 0
	for i, score := range scores {
		if score > maxScore {
			maxScore = score
			winner = i
		}
	}

	return fmt.Sprintf("%s (Score: %d)", names[winner], maxScore)
}

// analyzePersistenceResults 分析持久化测试结果
func analyzePersistenceResults(t *testing.T, allResults map[string][]*PersistenceTestMetrics) {
	t.Logf("\n💾 ===== PERSISTENCE BATTLE FINAL ANALYSIS =====")
	
	// 详细对比表
	t.Logf("📊 Detailed Performance Comparison:")
	t.Logf("%-12s | %-12s | %-8s | %-10s | %-10s | %-8s", 
		"Pressure", "System", "Messages", "Success", "Throughput", "Latency")
	t.Logf("%-12s-+-%-12s-+-%-8s-+-%-10s-+-%-10s-+-%-8s", 
		"------------", "------------", "--------", "----------", "----------", "--------")
	
	pressures := []string{"Low", "Medium", "High"}
	systems := []string{"Memory", "Disk", "Kafka"}
	systemNames := map[string]string{
		"Memory": "NATS(Mem)",
		"Disk":   "NATS(Disk)",
		"Kafka":  "Kafka",
	}
	
	for i, pressure := range pressures {
		for _, system := range systems {
			if i < len(allResults[system]) {
				result := allResults[system][i]
				t.Logf("%-12s | %-12s | %8d | %9.1f%% | %8.1f/s | %6.0fms", 
					pressure, systemNames[system], result.Messages, result.SuccessRate, 
					result.Throughput, float64(result.FirstLatency.Nanoseconds())/1000000)
			}
		}
		if i < len(pressures)-1 {
			t.Logf("%-12s-+-%-12s-+-%-8s-+-%-10s-+-%-10s-+-%-8s", 
				"", "", "", "", "", "")
		}
	}
	
	// 计算平均性能
	avgResults := make(map[string]*PersistenceTestMetrics)
	for system, results := range allResults {
		if len(results) == 0 {
			continue
		}
		
		avg := &PersistenceTestMetrics{System: results[0].System, Mode: results[0].Mode}
		for _, result := range results {
			avg.SuccessRate += result.SuccessRate
			avg.Throughput += result.Throughput
			if result.FirstLatency > 0 {
				avg.FirstLatency += result.FirstLatency
			}
		}
		avg.SuccessRate /= float64(len(results))
		avg.Throughput /= float64(len(results))
		avg.FirstLatency /= time.Duration(len(results))
		
		avgResults[system] = avg
	}
	
	t.Logf("\n📈 Average Performance Summary:")
	for system, avg := range avgResults {
		t.Logf("   %s: %.1f%% success, %.1f msg/s, %.1fms latency", 
			systemNames[system], avg.SuccessRate, avg.Throughput, 
			float64(avg.FirstLatency.Nanoseconds())/1000000)
	}
	
	// 性能对比分析
	t.Logf("\n🔍 Performance Analysis:")
	
	if memAvg, ok := avgResults["Memory"]; ok {
		if diskAvg, ok2 := avgResults["Disk"]; ok2 {
			throughputRatio := memAvg.Throughput / diskAvg.Throughput
			latencyRatio := float64(diskAvg.FirstLatency.Nanoseconds()) / float64(memAvg.FirstLatency.Nanoseconds())
			
			t.Logf("   🔵 NATS Memory vs Disk:")
			t.Logf("     📊 Throughput: Memory is %.1fx faster than Disk", throughputRatio)
			t.Logf("     ⏱️  Latency: Memory is %.1fx faster than Disk", latencyRatio)
			
			if throughputRatio > 1.5 {
				t.Logf("     💡 Memory mode shows significant performance advantage")
			} else {
				t.Logf("     💡 Disk persistence has acceptable performance overhead")
			}
		}
	}
	
	if memAvg, ok := avgResults["Memory"]; ok {
		if kafkaAvg, ok2 := avgResults["Kafka"]; ok2 {
			throughputRatio := memAvg.Throughput / kafkaAvg.Throughput
			latencyRatio := float64(kafkaAvg.FirstLatency.Nanoseconds()) / float64(memAvg.FirstLatency.Nanoseconds())
			
			t.Logf("   🔵 NATS Memory vs 🔴 Kafka:")
			t.Logf("     📊 Throughput: NATS is %.1fx faster than Kafka", throughputRatio)
			t.Logf("     ⏱️  Latency: NATS is %.1fx faster than Kafka", latencyRatio)
		}
	}
	
	if diskAvg, ok := avgResults["Disk"]; ok {
		if kafkaAvg, ok2 := avgResults["Kafka"]; ok2 {
			throughputRatio := diskAvg.Throughput / kafkaAvg.Throughput
			latencyRatio := float64(kafkaAvg.FirstLatency.Nanoseconds()) / float64(diskAvg.FirstLatency.Nanoseconds())
			
			t.Logf("   🟡 NATS Disk vs 🔴 Kafka:")
			t.Logf("     📊 Throughput: NATS Disk is %.1fx faster than Kafka", throughputRatio)
			t.Logf("     ⏱️  Latency: NATS Disk is %.1fx faster than Kafka", latencyRatio)
		}
	}
	
	// 最终推荐
	t.Logf("\n💡 Final Recommendations:")
	t.Logf("   🔵 Choose NATS Memory for: Maximum performance, temporary data")
	t.Logf("   🟡 Choose NATS Disk for: High performance + durability")
	t.Logf("   🔴 Choose Kafka for: Enterprise ecosystem, complex processing")
	
	t.Logf("\n💾 Persistence battle completed!")
}
