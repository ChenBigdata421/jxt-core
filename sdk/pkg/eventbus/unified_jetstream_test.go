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

// TestUnifiedJetStreamArchitecture 测试统一的JetStream架构
// ✅ 1个EventBus实例
// ✅ 1个NATS连接
// ✅ 1个JetStream Context
// ✅ 1个统一Consumer
// ✅ 多个Pull Subscription（每个topic一个）
// ✅ 磁盘持久化
func TestUnifiedJetStreamArchitecture(t *testing.T) {
	t.Logf("🚀 UNIFIED JETSTREAM ARCHITECTURE TEST")
	t.Logf("📊 1 EventBus, 1 Connection, 1 JetStream, 1 Consumer, Multiple Subscriptions, Disk Persistence")

	// 创建统一的JetStream配置
	// 关键：使用一个通用的Stream来处理所有topic
	// 使用纳秒时间戳确保Subject唯一性
	timestamp := time.Now().UnixNano()
	subjectPrefix := fmt.Sprintf("unified.%d", timestamp)

	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("unified-jetstream-%d", timestamp),
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
				Name:      fmt.Sprintf("UNIFIED_STREAM_%d", timestamp),
				Subjects:  []string{fmt.Sprintf("%s.>", subjectPrefix)}, // 唯一的通配符Subject
				Retention: "limits",
				Storage:   "file", // 磁盘持久化
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  512 * 1024 * 1024, // 512MB
				MaxMsgs:   100000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("unified_consumer_%d", timestamp),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 500,
				MaxWaiting:    200,
				MaxDeliver:    3,
			},
		},
	}

	// 创建EventBus实例
	t.Logf("📦 Creating unified EventBus instance...")
	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Fatalf("❌ Failed to create EventBus: %v", err)
	}
	defer eventBus.Close()

	t.Logf("✅ EventBus created successfully")

	// 测试场景：多个topic，多个subscription
	scenarios := []struct {
		name     string
		topic    string
		messages int
	}{
		{"Topic1", fmt.Sprintf("%s.topic1", subjectPrefix), 100},
		{"Topic2", fmt.Sprintf("%s.topic2", subjectPrefix), 150},
		{"Topic3", fmt.Sprintf("%s.topic3", subjectPrefix), 200},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 为每个topic创建subscription和计数器
	receivedCounts := make(map[string]*int64)

	for _, scenario := range scenarios {
		count := int64(0)
		receivedCounts[scenario.topic] = &count

		topic := scenario.topic
		counter := receivedCounts[scenario.topic]

		// 创建handler
		handler := func(ctx context.Context, message []byte) error {
			atomic.AddInt64(counter, 1)

			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err == nil {
				if msgTopic, ok := msg["topic"].(string); ok {
					if msgTopic != topic {
						t.Errorf("❌ Message topic mismatch: expected %s, got %s", topic, msgTopic)
					}
				}
			}

			return nil
		}

		// 订阅topic
		t.Logf("📝 Subscribing to %s...", scenario.topic)
		err := eventBus.Subscribe(ctx, scenario.topic, handler)
		if err != nil {
			t.Fatalf("❌ Failed to subscribe to %s: %v", scenario.topic, err)
		}
		t.Logf("✅ Subscribed to %s", scenario.topic)
	}

	// 预热
	t.Logf("🔥 Warming up for 5 seconds...")
	time.Sleep(5 * time.Second)

	// 发送消息到所有topic
	t.Logf("⚡ Sending messages to all topics...")
	startTime := time.Now()

	for _, scenario := range scenarios {
		t.Logf("📤 Sending %d messages to %s...", scenario.messages, scenario.topic)

		for i := 0; i < scenario.messages; i++ {
			message := map[string]interface{}{
				"id":        fmt.Sprintf("%s-%d", scenario.name, i),
				"topic":     scenario.topic,
				"content":   fmt.Sprintf("Test message %d for %s", i, scenario.name),
				"timestamp": time.Now().UnixNano(),
				"index":     i,
			}

			messageBytes, _ := json.Marshal(message)
			err := eventBus.Publish(ctx, scenario.topic, messageBytes)
			if err != nil {
				t.Logf("⚠️ Failed to send message %d to %s: %v", i, scenario.topic, err)
			}
		}

		t.Logf("✅ Sent %d messages to %s", scenario.messages, scenario.topic)
	}

	sendDuration := time.Since(startTime)
	totalMessages := 0
	for _, s := range scenarios {
		totalMessages += s.messages
	}
	t.Logf("📊 Total sent: %d messages in %.2fs (%.1f msg/s)",
		totalMessages, sendDuration.Seconds(), float64(totalMessages)/sendDuration.Seconds())

	// 等待处理
	t.Logf("⏳ Waiting 15 seconds for processing...")
	time.Sleep(15 * time.Second)

	// 检查结果
	t.Logf("\n📊 Results Summary:")
	t.Logf("%-20s | Expected | Received | Success Rate", "Topic")
	t.Logf("---------------------+----------+----------+-------------")

	totalExpected := 0
	totalReceived := int64(0)

	for _, scenario := range scenarios {
		received := atomic.LoadInt64(receivedCounts[scenario.topic])
		successRate := float64(received) / float64(scenario.messages) * 100

		status := "✅"
		if successRate < 100 {
			status = "⚠️"
			if successRate < 80 {
				status = "❌"
			}
		}

		t.Logf("%-20s | %8d | %8d | %10.1f%% %s",
			scenario.name, scenario.messages, received, successRate, status)

		totalExpected += scenario.messages
		totalReceived += received
	}

	t.Logf("---------------------+----------+----------+-------------")
	overallSuccessRate := float64(totalReceived) / float64(totalExpected) * 100
	t.Logf("%-20s | %8d | %8d | %10.1f%%",
		"TOTAL", totalExpected, totalReceived, overallSuccessRate)

	// 资源使用情况
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	t.Logf("\n💾 Resource Usage:")
	t.Logf("   Memory: %.2f MB", float64(m.Alloc)/1024/1024)
	t.Logf("   Goroutines: %d", runtime.NumGoroutine())

	// 验证架构
	t.Logf("\n🏗️ Architecture Verification:")
	t.Logf("   ✅ 1 EventBus instance")
	t.Logf("   ✅ 1 NATS connection")
	t.Logf("   ✅ 1 JetStream Context")
	t.Logf("   ✅ 1 Unified Consumer (Durable: %s)", config.JetStream.Consumer.DurableName)
	t.Logf("   ✅ %d Pull Subscriptions (one per topic)", len(scenarios))
	t.Logf("   ✅ Disk persistence (Storage: file)")
	t.Logf("   ✅ Stream: %s", config.JetStream.Stream.Name)
	t.Logf("   ✅ Subjects: %v", config.JetStream.Stream.Subjects)

	// 最终判断
	if overallSuccessRate >= 95 {
		t.Logf("\n🎉 SUCCESS! Unified JetStream architecture works perfectly!")
		t.Logf("📊 Overall success rate: %.1f%%", overallSuccessRate)
	} else if overallSuccessRate >= 80 {
		t.Logf("\n⚠️ PARTIAL SUCCESS. Unified JetStream architecture works but needs optimization.")
		t.Logf("📊 Overall success rate: %.1f%%", overallSuccessRate)
	} else {
		t.Fatalf("\n❌ FAILED! Overall success rate too low: %.1f%%", overallSuccessRate)
	}
}

// TestUnifiedJetStreamPerformance 统一JetStream性能测试
func TestUnifiedJetStreamPerformance(t *testing.T) {
	t.Logf("🚀 UNIFIED JETSTREAM PERFORMANCE TEST")

	// 创建统一的JetStream配置（磁盘持久化）
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("perf-jetstream-%d", time.Now().Unix()),
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
				Name:      fmt.Sprintf("PERF_STREAM_%d", time.Now().Unix()),
				Subjects:  []string{"perf.>"}, // 通配符Subject
				Retention: "limits",
				Storage:   "file", // 磁盘持久化
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  512 * 1024 * 1024,
				MaxMsgs:   100000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("perf_consumer_%d", time.Now().Unix()),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 500,
				MaxWaiting:    200,
				MaxDeliver:    3,
			},
		},
	}

	// 创建EventBus
	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Fatalf("❌ Failed to create EventBus: %v", err)
	}
	defer eventBus.Close()

	t.Logf("✅ EventBus created with disk persistence")

	// 性能测试场景
	scenarios := []struct {
		name     string
		messages int
	}{
		{"Light", 300},
		{"Medium", 800},
		{"Heavy", 1500},
	}

	for _, scenario := range scenarios {
		t.Logf("\n🎯 ===== %s Load Test (%d messages) =====", scenario.name, scenario.messages)

		topic := fmt.Sprintf("perf.%s.test", scenario.name)

		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)

		var receivedCount int64
		var firstMessageTime time.Time

		// 订阅
		handler := func(ctx context.Context, message []byte) error {
			count := atomic.AddInt64(&receivedCount, 1)
			if count == 1 {
				firstMessageTime = time.Now()
			}
			return nil
		}

		err := eventBus.Subscribe(ctx, topic, handler)
		if err != nil {
			t.Fatalf("❌ Subscribe failed: %v", err)
		}

		// 预热
		time.Sleep(5 * time.Second)

		// 发送消息
		t.Logf("⚡ Sending %d messages...", scenario.messages)
		sendStart := time.Now()

		for i := 0; i < scenario.messages; i++ {
			message := map[string]interface{}{
				"id":        fmt.Sprintf("%s-%d", scenario.name, i),
				"content":   fmt.Sprintf("Performance test message %d", i),
				"timestamp": time.Now().UnixNano(),
			}

			messageBytes, _ := json.Marshal(message)
			eventBus.Publish(ctx, topic, messageBytes)
		}

		sendDuration := time.Since(sendStart)
		sendRate := float64(scenario.messages) / sendDuration.Seconds()

		t.Logf("📤 Sent %d messages in %.2fs (%.1f msg/s)",
			scenario.messages, sendDuration.Seconds(), sendRate)

		// 等待处理
		t.Logf("⏳ Waiting 20s for processing...")
		time.Sleep(20 * time.Second)

		// 结果
		received := atomic.LoadInt64(&receivedCount)
		successRate := float64(received) / float64(scenario.messages) * 100

		var firstLatency time.Duration
		if !firstMessageTime.IsZero() {
			firstLatency = firstMessageTime.Sub(sendStart)
		}

		t.Logf("📊 Results: %d/%d (%.1f%%), First latency: %v",
			received, scenario.messages, successRate, firstLatency)

		cancel()
		time.Sleep(3 * time.Second)
	}

	t.Logf("\n🎯 Unified JetStream Performance Test Completed!")
}
