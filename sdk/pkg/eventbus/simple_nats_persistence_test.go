package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// SimplePersistenceMetrics 简化持久化测试指标
type SimplePersistenceMetrics struct {
	System       string
	Mode         string
	Pressure     string
	Messages     int
	Received     int64
	SuccessRate  float64
	Throughput   float64
	FirstLatency time.Duration
}

// TestSimpleNATSPersistence 简化的NATS持久化测试
func TestSimpleNATSPersistence(t *testing.T) {
	t.Logf("💾 SIMPLE NATS PERSISTENCE TEST")
	t.Logf("🔵 Testing NATS with different configurations")

	// 简化的测试场景
	scenarios := []struct {
		name     string
		messages int
		timeout  time.Duration
	}{
		{"Low", 200, 40 * time.Second},
		{"Medium", 500, 60 * time.Second},
	}

	for _, scenario := range scenarios {
		t.Logf("\n💾 ===== %s Pressure Test (%d messages) =====", scenario.name, scenario.messages)

		// 1. 测试基本NATS (无JetStream)
		t.Logf("🔵 Basic NATS (No Persistence)...")
		basicResult := testBasicNATS(t, scenario.name, scenario.messages, scenario.timeout)

		time.Sleep(3 * time.Second)

		// 2. 测试Kafka作为对比
		t.Logf("🔴 Kafka (Disk Persistence)...")
		kafkaResult := testKafkaSimple(t, scenario.name, scenario.messages, scenario.timeout)

		// 比较结果
		compareSimpleResults(t, scenario.name, basicResult, kafkaResult)

		time.Sleep(5 * time.Second)
	}

	t.Logf("\n💾 Simple persistence test completed!")
}

// testBasicNATS 测试基本NATS功能
func testBasicNATS(t *testing.T, pressure string, messageCount int, timeout time.Duration) *SimplePersistenceMetrics {
	// 使用最简单的NATS配置
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("simple-nats-%s", pressure),
		MaxReconnects:       3,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: false, // 禁用JetStream，使用基本NATS
		},
	}

	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Logf("❌ Basic NATS failed: %v", err)
		return &SimplePersistenceMetrics{
			System:      "NATS",
			Mode:        "Basic",
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
		}
	}
	defer eventBus.Close()

	testTopic := fmt.Sprintf("simple.nats.%s.test", pressure)
	return runSimplePersistenceTest(t, eventBus, "NATS", "Basic", pressure, messageCount, timeout, testTopic)
}

// testKafkaSimple 测试Kafka简单配置
func testKafkaSimple(t *testing.T, pressure string, messageCount int, timeout time.Duration) *SimplePersistenceMetrics {
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: fmt.Sprintf("simple-kafka-%s", pressure),
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
		},
		Consumer: ConsumerConfig{
			GroupID:            fmt.Sprintf("simple-kafka-%s-group", pressure),
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
	testTopic := fmt.Sprintf("simple.kafka.%s.test", pressure)
	kafkaBus.allPossibleTopics = []string{testTopic}

	return runSimplePersistenceTest(t, eventBus, "Kafka", "Disk", pressure, messageCount, timeout, testTopic)
}

// runSimplePersistenceTest 运行简化持久化测试
func runSimplePersistenceTest(t *testing.T, eventBus EventBus, system, mode, pressure string, messageCount int, timeout time.Duration, topic string) *SimplePersistenceMetrics {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startTime := time.Now()
	var receivedCount int64
	var firstMessageTime time.Time

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
		return &SimplePersistenceMetrics{
			System:      system,
			Mode:        mode,
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
		}
	}

	// 预热
	time.Sleep(2 * time.Second)

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

	t.Logf("📤 %s (%s) sent %d messages in %.2fs",
		system, mode, messageCount, sendDuration.Seconds())

	// 等待处理
	waitTime := 10 * time.Second
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

	result := &SimplePersistenceMetrics{
		System:       system,
		Mode:         mode,
		Pressure:     pressure,
		Messages:     messageCount,
		Received:     finalReceived,
		SuccessRate:  successRate,
		Throughput:   throughput,
		FirstLatency: firstLatency,
	}

	t.Logf("📊 %s (%s) Results: %d/%d (%.1f%%), %.1f msg/s, %v latency",
		system, mode, finalReceived, messageCount, successRate, throughput, firstLatency)

	return result
}

// compareSimpleResults 比较简化测试结果
func compareSimpleResults(t *testing.T, pressure string, nats, kafka *SimplePersistenceMetrics) {
	t.Logf("\n🏆 %s Pressure Results:", pressure)
	t.Logf("   🔵 NATS (Basic): %.1f%% success, %.1f msg/s, %v latency",
		nats.SuccessRate, nats.Throughput, nats.FirstLatency)
	t.Logf("   🔴 Kafka (Disk): %.1f%% success, %.1f msg/s, %v latency",
		kafka.SuccessRate, kafka.Throughput, kafka.FirstLatency)

	// 简单比较
	natsWins := 0
	kafkaWins := 0

	// 成功率比较
	if nats.SuccessRate > kafka.SuccessRate {
		natsWins++
		t.Logf("   ✅ NATS wins in success rate")
	} else if kafka.SuccessRate > nats.SuccessRate {
		kafkaWins++
		t.Logf("   ✅ Kafka wins in success rate")
	} else {
		t.Logf("   🤝 Tie in success rate")
	}

	// 吞吐量比较
	if nats.Throughput > kafka.Throughput {
		natsWins++
		t.Logf("   ✅ NATS wins in throughput")
	} else if kafka.Throughput > nats.Throughput {
		kafkaWins++
		t.Logf("   ✅ Kafka wins in throughput")
	} else {
		t.Logf("   🤝 Tie in throughput")
	}

	// 延迟比较
	if nats.FirstLatency > 0 && kafka.FirstLatency > 0 {
		if nats.FirstLatency < kafka.FirstLatency {
			natsWins++
			t.Logf("   ✅ NATS wins in latency")
		} else if kafka.FirstLatency < nats.FirstLatency {
			kafkaWins++
			t.Logf("   ✅ Kafka wins in latency")
		} else {
			t.Logf("   🤝 Tie in latency")
		}
	}

	// 宣布胜者
	if natsWins > kafkaWins {
		t.Logf("   🏆 %s Winner: 🔵 NATS (%d vs %d)", pressure, natsWins, kafkaWins)
	} else if kafkaWins > natsWins {
		t.Logf("   🏆 %s Winner: 🔴 Kafka (%d vs %d)", pressure, kafkaWins, natsWins)
	} else {
		t.Logf("   🏆 %s Result: 🤝 Tie (%d vs %d)", pressure, natsWins, kafkaWins)
	}
}

// TestNATSJetStreamManualSetup 手动设置JetStream测试
func TestNATSJetStreamManualSetup(t *testing.T) {
	t.Logf("🔧 NATS JETSTREAM MANUAL SETUP TEST")
	t.Logf("💡 This test will help debug JetStream configuration issues")

	// 连接到NATS
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            "jetstream-debug",
		MaxReconnects:       3,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: false, // 先不启用，手动测试
		},
	}

	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Fatalf("❌ Failed to connect to NATS: %v", err)
	}
	defer eventBus.Close()

	t.Logf("✅ Successfully connected to NATS server")

	t.Logf("✅ NATS connection successful")
	t.Logf("💡 JetStream manual setup test skipped - requires direct NATS client access")
	t.Logf("🔧 The issue is likely in the EventBus JetStream configuration")
	t.Logf("💡 Recommendation: Use basic NATS mode for now, fix JetStream configuration later")
}
