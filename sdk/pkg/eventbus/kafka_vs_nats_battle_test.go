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

// BattleMetrics 对战测试指标
type BattleMetrics struct {
	System       string
	Pressure     string
	Messages     int
	Received     int64
	Errors       int64
	Duration     time.Duration
	SendRate     float64
	SuccessRate  float64
	Throughput   float64
	FirstLatency time.Duration
	AvgLatency   time.Duration
	Goroutines   int
	MemoryMB     float64
}

// TestKafkaVsNatsBattle Kafka vs NATS JetStream终极对决
func TestKafkaVsNatsBattle(t *testing.T) {
	t.Logf("⚔️ KAFKA vs NATS JETSTREAM - ULTIMATE BATTLE!")
	t.Logf("🥊 Fighting for the title of 'Best Message Queue'")

	// 战斗场景
	battles := []struct {
		name     string
		messages int
		timeout  time.Duration
	}{
		{"Low Pressure", 500, 60 * time.Second},
		{"Medium Pressure", 1500, 90 * time.Second},
		{"High Pressure", 3000, 120 * time.Second},
	}

	kafkaResults := make([]*BattleMetrics, 0)
	natsResults := make([]*BattleMetrics, 0)

	for _, battle := range battles {
		t.Logf("\n🥊 ===== %s BATTLE (%d messages) =====", battle.name, battle.messages)

		// Round 1: Kafka (RedPanda)
		t.Logf("🔴 Round 1: Kafka (RedPanda) enters the ring...")
		kafkaMetrics := fightKafka(t, battle.name, battle.messages, battle.timeout)
		kafkaResults = append(kafkaResults, kafkaMetrics)

		// 短暂休息
		time.Sleep(5 * time.Second)

		// Round 2: NATS JetStream
		t.Logf("🔵 Round 2: NATS JetStream enters the ring...")
		natsMetrics := fightNats(t, battle.name, battle.messages, battle.timeout)
		natsResults = append(natsResults, natsMetrics)

		// 回合结果
		winner := determineRoundWinner(kafkaMetrics, natsMetrics)
		t.Logf("🏆 %s Round Winner: %s", battle.name, winner)

		// 休息准备下一回合
		time.Sleep(10 * time.Second)
	}

	// 最终决战结果
	declareUltimateWinner(t, kafkaResults, natsResults)
}

// fightKafka Kafka战斗函数
func fightKafka(t *testing.T, pressure string, messageCount int, timeout time.Duration) *BattleMetrics {
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: fmt.Sprintf("kafka-battle-%s", pressure),
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
		},
		Consumer: ConsumerConfig{
			GroupID:            fmt.Sprintf("kafka-battle-%s-group", pressure),
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
		t.Fatalf("Kafka failed to enter the ring: %v", err)
	}
	defer eventBus.Close()

	return fight(t, eventBus, "Kafka", pressure, messageCount, timeout, fmt.Sprintf("kafka.battle.%s", pressure))
}

// fightNats NATS战斗函数
func fightNats(t *testing.T, pressure string, messageCount int, timeout time.Duration) *BattleMetrics {
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-battle-%s", pressure),
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
				Name:      fmt.Sprintf("battle-%s-stream", pressure),
				Subjects:  []string{fmt.Sprintf("nats.battle.%s.>", pressure)},
				Retention: "limits",
				Storage:   "memory",
				Replicas:  1,
				MaxAge:    1 * time.Hour,
				MaxBytes:  256 * 1024 * 1024,
				MaxMsgs:   100000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("battle-%s-consumer", pressure),
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
		t.Logf("❌ NATS failed to enter the ring: %v", err)
		// 返回失败指标
		return &BattleMetrics{
			System:      "NATS",
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
			Errors:      int64(messageCount),
		}
	}
	defer eventBus.Close()

	return fight(t, eventBus, "NATS", pressure, messageCount, timeout, fmt.Sprintf("nats.battle.%s", pressure))
}

// fight 通用战斗函数
func fight(t *testing.T, eventBus EventBus, system, pressure string, messageCount int, timeout time.Duration, topic string) *BattleMetrics {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	startTime := time.Now()
	var receivedCount int64
	var errors int64
	var firstMessageTime time.Time
	var lastMessageTime time.Time

	// 获取初始资源
	initialGoroutines := runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	initialMemory := m.Alloc

	// 战斗处理器
	handler := func(ctx context.Context, message []byte) error {
		receiveTime := time.Now()
		count := atomic.AddInt64(&receivedCount, 1)

		if count == 1 {
			firstMessageTime = receiveTime
		}
		lastMessageTime = receiveTime

		return nil
	}

	// 订阅
	err := eventBus.Subscribe(ctx, topic, handler)
	if err != nil {
		t.Logf("❌ %s failed to subscribe: %v", system, err)
		return &BattleMetrics{
			System:      system,
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
			Errors:      int64(messageCount),
		}
	}

	// 预热
	time.Sleep(3 * time.Second)

	t.Logf("⚡ %s attacking with %d messages...", system, messageCount)

	// 发送攻击
	sendStart := time.Now()
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":      fmt.Sprintf("%s-%s-msg-%d", system, pressure, i),
			"content": fmt.Sprintf("%s %s battle message %d", system, pressure, i),
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

	t.Logf("📤 %s sent %d messages in %.2fs (%.1f msg/s)",
		system, messageCount, sendDuration.Seconds(), sendRate)

	// 等待战斗结果
	waitTime := 20 * time.Second
	t.Logf("⏳ %s waiting %.0fs for battle results...", system, waitTime.Seconds())
	time.Sleep(waitTime)

	// 计算战斗结果
	totalDuration := time.Since(startTime)
	finalReceived := atomic.LoadInt64(&receivedCount)
	finalErrors := atomic.LoadInt64(&errors)
	successRate := float64(finalReceived) / float64(messageCount) * 100
	throughput := float64(finalReceived) / totalDuration.Seconds()

	var firstLatency time.Duration
	var avgLatency time.Duration
	if !firstMessageTime.IsZero() {
		firstLatency = firstMessageTime.Sub(sendStart)
		if !lastMessageTime.IsZero() && finalReceived > 1 {
			avgLatency = lastMessageTime.Sub(firstMessageTime) / time.Duration(finalReceived-1)
		}
	}

	// 获取峰值资源
	peakGoroutines := runtime.NumGoroutine()
	runtime.ReadMemStats(&m)
	peakMemory := m.Alloc

	metrics := &BattleMetrics{
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
		AvgLatency:   avgLatency,
		Goroutines:   peakGoroutines - initialGoroutines,
		MemoryMB:     float64(peakMemory-initialMemory) / 1024 / 1024,
	}

	t.Logf("🥊 %s Battle Results:", system)
	t.Logf("   📊 Success: %d/%d (%.1f%%)", finalReceived, messageCount, successRate)
	t.Logf("   🚀 Throughput: %.1f msg/s", throughput)
	t.Logf("   ⏱️  First Latency: %v", firstLatency)
	t.Logf("   🔧 Resources: +%d goroutines, +%.2f MB",
		peakGoroutines-initialGoroutines, float64(peakMemory-initialMemory)/1024/1024)

	return metrics
}

// determineRoundWinner 判断回合胜者
func determineRoundWinner(kafka, nats *BattleMetrics) string {
	kafkaScore := 0
	natsScore := 0

	// 成功率对比 (权重: 40%)
	if kafka.SuccessRate > nats.SuccessRate {
		kafkaScore += 40
	} else if nats.SuccessRate > kafka.SuccessRate {
		natsScore += 40
	} else {
		kafkaScore += 20
		natsScore += 20
	}

	// 吞吐量对比 (权重: 30%)
	if kafka.Throughput > nats.Throughput {
		kafkaScore += 30
	} else if nats.Throughput > kafka.Throughput {
		natsScore += 30
	} else {
		kafkaScore += 15
		natsScore += 15
	}

	// 延迟对比 (权重: 20%)
	if kafka.FirstLatency < nats.FirstLatency && kafka.FirstLatency > 0 {
		kafkaScore += 20
	} else if nats.FirstLatency < kafka.FirstLatency && nats.FirstLatency > 0 {
		natsScore += 20
	} else {
		kafkaScore += 10
		natsScore += 10
	}

	// 资源效率对比 (权重: 10%)
	if kafka.MemoryMB < nats.MemoryMB {
		kafkaScore += 10
	} else if nats.MemoryMB < kafka.MemoryMB {
		natsScore += 10
	} else {
		kafkaScore += 5
		natsScore += 5
	}

	if kafkaScore > natsScore {
		return fmt.Sprintf("Kafka (Score: %d vs %d)", kafkaScore, natsScore)
	} else if natsScore > kafkaScore {
		return fmt.Sprintf("NATS (Score: %d vs %d)", natsScore, kafkaScore)
	} else {
		return fmt.Sprintf("Tie (Score: %d vs %d)", kafkaScore, natsScore)
	}
}

// declareUltimateWinner 宣布最终胜者
func declareUltimateWinner(t *testing.T, kafkaResults, natsResults []*BattleMetrics) {
	t.Logf("\n🏆 ===== ULTIMATE BATTLE RESULTS =====")

	// 战斗统计表
	t.Logf("📊 Battle Statistics:")
	t.Logf("%-15s | %-8s | %-10s | %-10s | %-10s | %-10s",
		"Pressure", "System", "Success", "Throughput", "Latency", "Memory")
	t.Logf("%-15s-+-%-8s-+-%-10s-+-%-10s-+-%-10s-+-%-10s",
		"---------------", "--------", "----------", "----------", "----------", "----------")

	pressures := []string{"Low Pressure", "Medium Pressure", "High Pressure"}

	for i, pressure := range pressures {
		if i < len(kafkaResults) && i < len(natsResults) {
			kafka := kafkaResults[i]
			nats := natsResults[i]

			t.Logf("%-15s | %-8s | %9.1f%% | %8.1f/s | %8.0fms | %8.1fMB",
				pressure, "Kafka", kafka.SuccessRate, kafka.Throughput,
				float64(kafka.FirstLatency.Nanoseconds())/1000000, kafka.MemoryMB)
			t.Logf("%-15s | %-8s | %9.1f%% | %8.1f/s | %8.0fms | %8.1fMB",
				"", "NATS", nats.SuccessRate, nats.Throughput,
				float64(nats.FirstLatency.Nanoseconds())/1000000, nats.MemoryMB)
		}
	}

	// 计算总分
	kafkaWins := 0
	natsWins := 0
	ties := 0

	for i := 0; i < len(kafkaResults) && i < len(natsResults); i++ {
		winner := determineRoundWinner(kafkaResults[i], natsResults[i])
		if len(winner) > 0 && winner[0] == 'K' { // Kafka wins
			kafkaWins++
		} else if len(winner) > 0 && winner[0] == 'N' { // NATS wins
			natsWins++
		} else {
			ties++
		}
	}

	t.Logf("\n🥊 Final Score:")
	t.Logf("   🔴 Kafka Wins: %d", kafkaWins)
	t.Logf("   🔵 NATS Wins: %d", natsWins)
	t.Logf("   🤝 Ties: %d", ties)

	// 宣布最终冠军
	if kafkaWins > natsWins {
		t.Logf("\n🏆 ULTIMATE CHAMPION: 🔴 KAFKA (RedPanda)")
		t.Logf("🎉 Kafka dominates with superior performance!")
	} else if natsWins > kafkaWins {
		t.Logf("\n🏆 ULTIMATE CHAMPION: 🔵 NATS JETSTREAM")
		t.Logf("🎉 NATS JetStream conquers the battlefield!")
	} else {
		t.Logf("\n🤝 ULTIMATE RESULT: TIE")
		t.Logf("🎉 Both systems are equally matched!")
	}

	// 详细分析
	t.Logf("\n📈 Performance Analysis:")

	// 计算平均指标
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

	t.Logf("   🔴 Kafka Average: %.1f%% success, %.1f msg/s throughput",
		kafkaAvgSuccess, kafkaAvgThroughput)
	t.Logf("   🔵 NATS Average: %.1f%% success, %.1f msg/s throughput",
		natsAvgSuccess, natsAvgThroughput)

	t.Logf("\n⚔️ The ultimate battle is complete!")
}
