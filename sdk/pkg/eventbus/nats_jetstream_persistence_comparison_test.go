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

// PersistenceComparisonMetrics 持久化对比指标
type PersistenceComparisonMetrics struct {
	System      string
	Mode        string // "Basic", "Memory", "Disk"
	Messages    int
	Received    int64
	SuccessRate float64
	Throughput  float64
	FirstLatency time.Duration
	MemoryMB    float64
	SendRate    float64
}

// TestNATSPersistenceModeComparison NATS持久化模式对比测试
func TestNATSPersistenceModeComparison(t *testing.T) {
	t.Logf("💾 NATS PERSISTENCE MODE COMPARISON")
	t.Logf("🔵 Basic NATS vs 🟡 JetStream Memory vs 🟠 JetStream Disk vs 🔴 Kafka")

	scenarios := []struct {
		name     string
		messages int
		timeout  time.Duration
	}{
		{"Light", 200, 45 * time.Second},
		{"Medium", 500, 60 * time.Second},
		{"Heavy", 1000, 90 * time.Second},
	}

	for _, scenario := range scenarios {
		t.Logf("\n💾 ===== %s Load Persistence Comparison (%d messages) =====", 
			scenario.name, scenario.messages)
		
		results := make([]*PersistenceComparisonMetrics, 0)
		
		// 1. Basic NATS (无持久化)
		t.Logf("🔵 Basic NATS (No Persistence)...")
		basicResult := testNATSBasic(t, scenario.name, scenario.messages, scenario.timeout)
		results = append(results, basicResult)
		
		time.Sleep(3 * time.Second)
		
		// 2. NATS JetStream Memory (内存持久化)
		t.Logf("🟡 NATS JetStream Memory Persistence...")
		memoryResult := testNATSJetStreamMemory(t, scenario.name, scenario.messages, scenario.timeout)
		results = append(results, memoryResult)
		
		time.Sleep(3 * time.Second)
		
		// 3. NATS JetStream Disk (磁盘持久化)
		t.Logf("🟠 NATS JetStream Disk Persistence...")
		diskResult := testNATSJetStreamDisk(t, scenario.name, scenario.messages, scenario.timeout)
		results = append(results, diskResult)
		
		time.Sleep(3 * time.Second)
		
		// 4. Kafka (磁盘持久化)
		t.Logf("🔴 Kafka Disk Persistence...")
		kafkaResult := testKafkaPersistence(t, scenario.name, scenario.messages, scenario.timeout)
		results = append(results, kafkaResult)
		
		// 分析结果
		analyzePersistenceComparison(t, scenario.name, results)
		
		time.Sleep(5 * time.Second)
	}
	
	t.Logf("\n💾 Persistence mode comparison completed!")
}

// testNATSBasic 测试基本NATS
func testNATSBasic(t *testing.T, pressure string, messageCount int, timeout time.Duration) *PersistenceComparisonMetrics {
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("basic-nats-%s", pressure),
		MaxReconnects:       3,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: false, // 基本NATS，无持久化
		},
	}

	return runPersistenceComparisonTest(t, config, "NATS", "Basic", pressure, messageCount, timeout)
}

// testNATSJetStreamMemory 测试NATS JetStream内存持久化
func testNATSJetStreamMemory(t *testing.T, pressure string, messageCount int, timeout time.Duration) *PersistenceComparisonMetrics {
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("jetstream-memory-%s", pressure),
		MaxReconnects:       3,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 5 * time.Second,
			AckWait:        10 * time.Second,
			MaxDeliver:     3,
			Stream: StreamConfig{
				Name:      fmt.Sprintf("MEMORY_%s_STREAM", pressure),
				Subjects:  []string{fmt.Sprintf("memory.%s.*", pressure)},
				Retention: "limits",
				Storage:   "memory", // 内存持久化
				Replicas:  1,
				MaxAge:    10 * time.Minute,
				MaxBytes:  64 * 1024 * 1024,
				MaxMsgs:   10000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("memory_%s_consumer", pressure),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 100,
				MaxWaiting:    50,
				MaxDeliver:    3,
			},
		},
	}

	return runPersistenceComparisonTest(t, config, "NATS", "Memory", pressure, messageCount, timeout)
}

// testNATSJetStreamDisk 测试NATS JetStream磁盘持久化
func testNATSJetStreamDisk(t *testing.T, pressure string, messageCount int, timeout time.Duration) *PersistenceComparisonMetrics {
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("jetstream-disk-%s", pressure),
		MaxReconnects:       3,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 5 * time.Second,
			AckWait:        10 * time.Second,
			MaxDeliver:     3,
			Stream: StreamConfig{
				Name:      fmt.Sprintf("DISK_%s_STREAM", pressure),
				Subjects:  []string{fmt.Sprintf("disk.%s.*", pressure)},
				Retention: "limits",
				Storage:   "file", // 磁盘持久化
				Replicas:  1,
				MaxAge:    10 * time.Minute,
				MaxBytes:  64 * 1024 * 1024,
				MaxMsgs:   10000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("disk_%s_consumer", pressure),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 100,
				MaxWaiting:    50,
				MaxDeliver:    3,
			},
		},
	}

	return runPersistenceComparisonTest(t, config, "NATS", "Disk", pressure, messageCount, timeout)
}

// testKafkaPersistence 测试Kafka持久化
func testKafkaPersistence(t *testing.T, pressure string, messageCount int, timeout time.Duration) *PersistenceComparisonMetrics {
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: fmt.Sprintf("persistence-kafka-%s", pressure),
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1, // 等待leader确认，确保持久化
			Timeout:         5 * time.Second,
		},
		Consumer: ConsumerConfig{
			GroupID:            fmt.Sprintf("persistence-kafka-%s-group", pressure),
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
	testTopic := fmt.Sprintf("persistence.kafka.%s.test", pressure)
	kafkaBus.allPossibleTopics = []string{testTopic}

	return runKafkaPersistenceTest(t, eventBus, "Kafka", "Disk", pressure, messageCount, timeout, testTopic)
}

// runPersistenceComparisonTest 运行NATS持久化对比测试
func runPersistenceComparisonTest(t *testing.T, config *NATSConfig, system, mode, pressure string, messageCount int, timeout time.Duration) *PersistenceComparisonMetrics {
	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Logf("❌ %s %s failed: %v", system, mode, err)
		return &PersistenceComparisonMetrics{
			System:      system,
			Mode:        mode,
			Messages:    messageCount,
			SuccessRate: 0,
		}
	}
	defer eventBus.Close()

	var testTopic string
	if mode == "Basic" {
		testTopic = fmt.Sprintf("basic.%s.test", pressure)
	} else if mode == "Memory" {
		testTopic = fmt.Sprintf("memory.%s.test", pressure)
	} else {
		testTopic = fmt.Sprintf("disk.%s.test", pressure)
	}

	return runGeneralPersistenceTest(t, eventBus, system, mode, pressure, messageCount, timeout, testTopic)
}

// runKafkaPersistenceTest 运行Kafka持久化测试
func runKafkaPersistenceTest(t *testing.T, eventBus EventBus, system, mode, pressure string, messageCount int, timeout time.Duration, topic string) *PersistenceComparisonMetrics {
	return runGeneralPersistenceTest(t, eventBus, system, mode, pressure, messageCount, timeout, topic)
}

// runGeneralPersistenceTest 运行通用持久化测试
func runGeneralPersistenceTest(t *testing.T, eventBus EventBus, system, mode, pressure string, messageCount int, timeout time.Duration, topic string) *PersistenceComparisonMetrics {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startTime := time.Now()
	var receivedCount int64
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
		return &PersistenceComparisonMetrics{
			System:      system,
			Mode:        mode,
			Messages:    messageCount,
			SuccessRate: 0,
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
		}

		messageBytes, _ := json.Marshal(message)
		err := eventBus.Publish(ctx, topic, messageBytes)
		if err != nil {
			t.Logf("❌ Send error: %v", err)
		}
	}
	sendDuration := time.Since(sendStart)
	sendRate := float64(messageCount) / sendDuration.Seconds()

	t.Logf("📤 %s (%s) sent %d messages in %.2fs (%.1f msg/s)", 
		system, mode, messageCount, sendDuration.Seconds(), sendRate)

	// 等待处理
	waitTime := 12 * time.Second
	t.Logf("⏳ %s (%s) waiting %.0fs...", system, mode, waitTime.Seconds())
	time.Sleep(waitTime)

	// 计算结果
	totalDuration := time.Since(startTime)
	finalReceived := atomic.LoadInt64(&receivedCount)
	successRate := float64(finalReceived) / float64(messageCount) * 100
	throughput := float64(finalReceived) / totalDuration.Seconds()

	var firstLatency time.Duration
	if !firstMessageTime.IsZero() {
		firstLatency = firstMessageTime.Sub(sendStart)
	}

	// 获取内存使用
	runtime.ReadMemStats(&m)
	memoryMB := float64(m.Alloc-initialMemory) / 1024 / 1024

	result := &PersistenceComparisonMetrics{
		System:       system,
		Mode:         mode,
		Messages:     messageCount,
		Received:     finalReceived,
		SuccessRate:  successRate,
		Throughput:   throughput,
		FirstLatency: firstLatency,
		MemoryMB:     memoryMB,
		SendRate:     sendRate,
	}

	t.Logf("📊 %s (%s) Results: %d/%d (%.1f%%), %.1f msg/s, %v latency", 
		system, mode, finalReceived, messageCount, successRate, throughput, firstLatency)

	return result
}

// analyzePersistenceComparison 分析持久化对比结果
func analyzePersistenceComparison(t *testing.T, pressure string, results []*PersistenceComparisonMetrics) {
	t.Logf("\n📈 %s Load Persistence Analysis:", pressure)
	
	// 详细对比
	for _, result := range results {
		t.Logf("   %s (%s): %.1f%% success, %.1f msg/s throughput, %v latency, %.2f MB memory", 
			result.System, result.Mode, result.SuccessRate, result.Throughput, 
			result.FirstLatency, result.MemoryMB)
	}
	
	// 性能对比
	if len(results) >= 4 {
		basic := results[0]
		memory := results[1]
		disk := results[2]
		kafka := results[3]
		
		t.Logf("\n🔍 Performance Impact Analysis:")
		
		// NATS模式对比
		if basic.Throughput > 0 && memory.Throughput > 0 {
			memoryImpact := (basic.Throughput - memory.Throughput) / basic.Throughput * 100
			t.Logf("   📊 Memory Persistence Impact: %.1f%% throughput reduction", memoryImpact)
		}
		
		if basic.Throughput > 0 && disk.Throughput > 0 {
			diskImpact := (basic.Throughput - disk.Throughput) / basic.Throughput * 100
			t.Logf("   💾 Disk Persistence Impact: %.1f%% throughput reduction", diskImpact)
		}
		
		// 与Kafka对比
		if disk.Throughput > 0 && kafka.Throughput > 0 {
			diskVsKafka := disk.Throughput / kafka.Throughput
			t.Logf("   ⚔️ NATS Disk vs Kafka: %.1fx faster", diskVsKafka)
		}
		
		// 延迟对比
		if disk.FirstLatency > 0 && kafka.FirstLatency > 0 {
			latencyRatio := float64(kafka.FirstLatency.Nanoseconds()) / float64(disk.FirstLatency.Nanoseconds())
			t.Logf("   ⏱️ NATS Disk vs Kafka latency: %.1fx faster", latencyRatio)
		}
	}
}
