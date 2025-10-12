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

// RealBattleMetrics 真实对战指标
type RealBattleMetrics struct {
	System      string
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

// TestRealKafkaVsNatsBattle 真实的Kafka vs NATS对决
func TestRealKafkaVsNatsBattle(t *testing.T) {
	t.Logf("⚔️ REAL KAFKA vs NATS JETSTREAM BATTLE!")
	t.Logf("🔴 Kafka (RedPanda): localhost:29094")
	t.Logf("🔵 NATS JetStream: localhost:4223")

	// 简化的测试场景
	battles := []struct {
		name     string
		messages int
		timeout  time.Duration
	}{
		{"Low", 300, 45 * time.Second},
		{"Medium", 800, 60 * time.Second},
		{"High", 1500, 90 * time.Second},
	}

	kafkaResults := make([]*RealBattleMetrics, 0)
	natsResults := make([]*RealBattleMetrics, 0)

	for _, battle := range battles {
		t.Logf("\n⚔️ ===== %s Pressure Battle (%d messages) =====", battle.name, battle.messages)
		
		// Kafka回合
		t.Logf("🔴 Kafka (RedPanda) Round...")
		kafkaMetrics := testKafkaReal(t, battle.name, battle.messages, battle.timeout)
		kafkaResults = append(kafkaResults, kafkaMetrics)
		
		time.Sleep(3 * time.Second)
		
		// NATS回合
		t.Logf("🔵 NATS JetStream Round...")
		natsMetrics := testNatsReal(t, battle.name, battle.messages, battle.timeout)
		natsResults = append(natsResults, natsMetrics)
		
		// 回合结果
		winner := compareRound(kafkaMetrics, natsMetrics)
		t.Logf("🏆 %s Winner: %s", battle.name, winner)
		
		time.Sleep(5 * time.Second)
	}

	// 最终对决结果
	declareRealWinner(t, kafkaResults, natsResults)
}

// testKafkaReal 测试Kafka真实性能
func testKafkaReal(t *testing.T, pressure string, messageCount int, timeout time.Duration) *RealBattleMetrics {
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: fmt.Sprintf("real-kafka-%s", pressure),
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
		},
		Consumer: ConsumerConfig{
			GroupID:            fmt.Sprintf("real-kafka-%s-group", pressure),
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
	testTopic := fmt.Sprintf("real.kafka.%s.test", pressure)
	kafkaBus.allPossibleTopics = []string{testTopic}

	return runRealTest(t, eventBus, "Kafka", pressure, messageCount, timeout, testTopic)
}

// testNatsReal 测试NATS真实性能
func testNatsReal(t *testing.T, pressure string, messageCount int, timeout time.Duration) *RealBattleMetrics {
	// 简化的NATS配置
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("real-nats-%s", pressure),
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
				Name:      fmt.Sprintf("REAL_%s_STREAM", pressure), // 使用大写和下划线
				Subjects:  []string{fmt.Sprintf("real.nats.%s.*", pressure)},
				Retention: "limits",
				Storage:   "memory",
				Replicas:  1,
				MaxAge:    10 * time.Minute,
				MaxBytes:  64 * 1024 * 1024, // 64MB
				MaxMsgs:   10000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("real_%s_consumer", pressure),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 100,
				MaxWaiting:    50,
				MaxDeliver:    3,
			},
		},
	}

	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Logf("❌ NATS failed: %v", err)
		return &RealBattleMetrics{
			System:      "NATS",
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
			Errors:      int64(messageCount),
		}
	}
	defer eventBus.Close()

	testTopic := fmt.Sprintf("real.nats.%s.test", pressure)
	return runRealTest(t, eventBus, "NATS", pressure, messageCount, timeout, testTopic)
}

// runRealTest 运行真实测试
func runRealTest(t *testing.T, eventBus EventBus, system, pressure string, messageCount int, timeout time.Duration, topic string) *RealBattleMetrics {
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
		t.Logf("❌ %s subscribe failed: %v", system, err)
		return &RealBattleMetrics{
			System:      system,
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
			Errors:      int64(messageCount),
		}
	}

	// 预热
	time.Sleep(3 * time.Second)

	t.Logf("⚡ %s sending %d messages...", system, messageCount)

	// 发送消息
	sendStart := time.Now()
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":      fmt.Sprintf("%s-%s-%d", system, pressure, i),
			"content": fmt.Sprintf("%s %s test message %d", system, pressure, i),
			"time":    time.Now().UnixNano(),
		}

		messageBytes, _ := json.Marshal(message)
		err := eventBus.Publish(ctx, topic, messageBytes)
		if err != nil {
			atomic.AddInt64(&errors, 1)
		}
	}
	sendDuration := time.Since(sendStart)
	sendRate := float64(messageCount) / sendDuration.Seconds()

	t.Logf("📤 %s sent %d messages in %.2fs (%.1f msg/s)", 
		system, messageCount, sendDuration.Seconds(), sendRate)

	// 等待处理
	waitTime := 15 * time.Second
	t.Logf("⏳ %s waiting %.0fs...", system, waitTime.Seconds())
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

	metrics := &RealBattleMetrics{
		System:       system,
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

	t.Logf("📊 %s Results: %d/%d (%.1f%%), %.1f msg/s, %v latency", 
		system, finalReceived, messageCount, successRate, throughput, firstLatency)

	return metrics
}

// compareRound 比较回合结果
func compareRound(kafka, nats *RealBattleMetrics) string {
	kafkaScore := 0
	natsScore := 0

	// 成功率比较 (50%)
	if kafka.SuccessRate > nats.SuccessRate {
		kafkaScore += 50
	} else if nats.SuccessRate > kafka.SuccessRate {
		natsScore += 50
	} else {
		kafkaScore += 25
		natsScore += 25
	}

	// 吞吐量比较 (30%)
	if kafka.Throughput > nats.Throughput {
		kafkaScore += 30
	} else if nats.Throughput > kafka.Throughput {
		natsScore += 30
	} else {
		kafkaScore += 15
		natsScore += 15
	}

	// 延迟比较 (20%)
	if kafka.FirstLatency < nats.FirstLatency && kafka.FirstLatency > 0 {
		kafkaScore += 20
	} else if nats.FirstLatency < kafka.FirstLatency && nats.FirstLatency > 0 {
		natsScore += 20
	} else {
		kafkaScore += 10
		natsScore += 10
	}

	if kafkaScore > natsScore {
		return fmt.Sprintf("Kafka (%d vs %d)", kafkaScore, natsScore)
	} else if natsScore > kafkaScore {
		return fmt.Sprintf("NATS (%d vs %d)", natsScore, kafkaScore)
	} else {
		return fmt.Sprintf("Tie (%d vs %d)", kafkaScore, natsScore)
	}
}

// declareRealWinner 宣布真实胜者
func declareRealWinner(t *testing.T, kafkaResults, natsResults []*RealBattleMetrics) {
	t.Logf("\n🏆 ===== REAL BATTLE FINAL RESULTS =====")
	
	// 详细对比表
	t.Logf("📊 Detailed Performance Comparison:")
	t.Logf("%-12s | %-6s | %-8s | %-10s | %-10s | %-8s", 
		"Pressure", "System", "Messages", "Success", "Throughput", "Latency")
	t.Logf("%-12s-+-%-6s-+-%-8s-+-%-10s-+-%-10s-+-%-8s", 
		"------------", "------", "--------", "----------", "----------", "--------")
	
	pressures := []string{"Low", "Medium", "High"}
	
	for i, pressure := range pressures {
		if i < len(kafkaResults) && i < len(natsResults) {
			kafka := kafkaResults[i]
			nats := natsResults[i]
			
			t.Logf("%-12s | %-6s | %8d | %9.1f%% | %8.1f/s | %6.0fms", 
				pressure, "Kafka", kafka.Messages, kafka.SuccessRate, 
				kafka.Throughput, float64(kafka.FirstLatency.Nanoseconds())/1000000)
			t.Logf("%-12s | %-6s | %8d | %9.1f%% | %8.1f/s | %6.0fms", 
				"", "NATS", nats.Messages, nats.SuccessRate, 
				nats.Throughput, float64(nats.FirstLatency.Nanoseconds())/1000000)
		}
	}
	
	// 计算胜负
	kafkaWins := 0
	natsWins := 0
	ties := 0
	
	for i := 0; i < len(kafkaResults) && i < len(natsResults); i++ {
		winner := compareRound(kafkaResults[i], natsResults[i])
		if winner[0] == 'K' {
			kafkaWins++
		} else if winner[0] == 'N' {
			natsWins++
		} else {
			ties++
		}
	}
	
	t.Logf("\n🥊 Battle Score:")
	t.Logf("   🔴 Kafka Wins: %d", kafkaWins)
	t.Logf("   🔵 NATS Wins: %d", natsWins)
	t.Logf("   🤝 Ties: %d", ties)
	
	// 计算平均性能
	var kafkaAvgSuccess, kafkaAvgThroughput float64
	var natsAvgSuccess, natsAvgThroughput float64
	
	for _, kafka := range kafkaResults {
		kafkaAvgSuccess += kafka.SuccessRate
		kafkaAvgThroughput += kafka.Throughput
	}
	for _, nats := range natsResults {
		natsAvgSuccess += nats.SuccessRate
		natsAvgThroughput += nats.Throughput
	}
	
	if len(kafkaResults) > 0 {
		kafkaAvgSuccess /= float64(len(kafkaResults))
		kafkaAvgThroughput /= float64(len(kafkaResults))
	}
	if len(natsResults) > 0 {
		natsAvgSuccess /= float64(len(natsResults))
		natsAvgThroughput /= float64(len(natsResults))
	}
	
	t.Logf("\n📈 Average Performance:")
	t.Logf("   🔴 Kafka: %.1f%% success, %.1f msg/s throughput", 
		kafkaAvgSuccess, kafkaAvgThroughput)
	t.Logf("   🔵 NATS: %.1f%% success, %.1f msg/s throughput", 
		natsAvgSuccess, natsAvgThroughput)
	
	// 宣布最终胜者
	if kafkaWins > natsWins {
		t.Logf("\n🏆 ULTIMATE CHAMPION: 🔴 KAFKA (RedPanda)")
		t.Logf("🎉 Kafka dominates with %d victories!", kafkaWins)
	} else if natsWins > kafkaWins {
		t.Logf("\n🏆 ULTIMATE CHAMPION: 🔵 NATS JETSTREAM")
		t.Logf("🎉 NATS conquers with %d victories!", natsWins)
	} else {
		t.Logf("\n🤝 ULTIMATE RESULT: TIE")
		t.Logf("🎉 Both systems are equally matched!")
	}
	
	// 技术建议
	t.Logf("\n💡 Technical Recommendations:")
	if kafkaAvgSuccess > natsAvgSuccess {
		t.Logf("   ✅ Choose Kafka for: Reliability and stability")
	}
	if natsAvgThroughput > kafkaAvgThroughput {
		t.Logf("   ✅ Choose NATS for: Higher throughput")
	}
	if kafkaAvgThroughput > natsAvgThroughput {
		t.Logf("   ✅ Choose Kafka for: Consistent performance")
	}
	
	t.Logf("\n⚔️ Real battle completed!")
}
