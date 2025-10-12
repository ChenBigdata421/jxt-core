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

// NATSTestMetrics NATS测试指标
type NATSTestMetrics struct {
	Pressure      string
	Messages      int
	Received      int64
	Errors        int64
	Duration      time.Duration
	SendRate      float64
	SuccessRate   float64
	Throughput    float64
	FirstLatency  time.Duration
	AvgLatency    time.Duration
	Goroutines    int
	MemoryMB      float64
}

// TestNATSJetStreamPressure NATS JetStream压力测试
func TestNATSJetStreamPressure(t *testing.T) {
	t.Logf("🔵 NATS JetStream Pressure Test")
	t.Logf("📋 Prerequisites: NATS server should be running on localhost:4222")
	t.Logf("🚀 Start NATS server with: nats-server -js")

	// 测试场景
	scenarios := []struct {
		name     string
		messages int
		timeout  time.Duration
	}{
		{"Low", 500, 60 * time.Second},
		{"Medium", 1500, 90 * time.Second},
		{"High", 3000, 120 * time.Second},
	}

	results := make([]*NATSTestMetrics, 0)

	for _, scenario := range scenarios {
		t.Logf("\n🔵 ===== NATS %s Pressure Test (%d messages) =====", 
			scenario.name, scenario.messages)
		
		metrics := runNATSPressureTest(t, scenario.name, scenario.messages, scenario.timeout)
		results = append(results, metrics)
		
		// 休息一下
		time.Sleep(5 * time.Second)
	}

	// 生成NATS性能报告
	generateNATSReport(t, results)
}

// runNATSPressureTest 运行NATS压力测试
func runNATSPressureTest(t *testing.T, pressure string, messageCount int, timeout time.Duration) *NATSTestMetrics {
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4222"},
		ClientID:            fmt.Sprintf("nats-pressure-%s", pressure),
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
				Name:      fmt.Sprintf("pressure-%s-stream", pressure),
				Subjects:  []string{fmt.Sprintf("pressure.%s.>", pressure)},
				Retention: "limits",
				Storage:   "memory", // 使用内存存储获得最佳性能
				Replicas:  1,
				MaxAge:    1 * time.Hour,
				MaxBytes:  256 * 1024 * 1024, // 256MB
				MaxMsgs:   100000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("pressure-%s-consumer", pressure),
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
		t.Logf("❌ NATS server not available: %v", err)
		t.Logf("💡 To run this test:")
		t.Logf("   1. Install NATS server: go install github.com/nats-io/nats-server/v2@latest")
		t.Logf("   2. Start server: nats-server -js")
		t.Logf("   3. Run test again")
		
		// 返回失败指标
		return &NATSTestMetrics{
			Pressure:    pressure,
			Messages:    messageCount,
			SuccessRate: 0,
			Errors:      int64(messageCount),
		}
	}
	defer eventBus.Close()

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

	// 消息处理器
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
	testTopic := fmt.Sprintf("pressure.%s.test", pressure)
	err = eventBus.Subscribe(ctx, testTopic, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 预热
	t.Logf("🔥 NATS warming up for 3 seconds...")
	time.Sleep(3 * time.Second)

	t.Logf("⚡ NATS attacking with %d messages...", messageCount)

	// 发送消息
	sendStart := time.Now()
	for i := 0; i < messageCount; i++ {
		message := map[string]interface{}{
			"id":      fmt.Sprintf("nats-%s-msg-%d", pressure, i),
			"content": fmt.Sprintf("NATS %s pressure test message %d", pressure, i),
			"time":    time.Now().UnixNano(),
			"index":   i,
		}

		messageBytes, _ := json.Marshal(message)
		err := eventBus.Publish(ctx, testTopic, messageBytes)
		if err != nil {
			atomic.AddInt64(&errors, 1)
		}
	}
	sendDuration := time.Since(sendStart)
	sendRate := float64(messageCount) / sendDuration.Seconds()

	t.Logf("📤 NATS sent %d messages in %.2fs (%.1f msg/s)", 
		messageCount, sendDuration.Seconds(), sendRate)

	// 等待处理
	waitTime := 20 * time.Second
	t.Logf("⏳ NATS waiting %.0fs for processing...", waitTime.Seconds())
	time.Sleep(waitTime)

	// 计算结果
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

	metrics := &NATSTestMetrics{
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

	t.Logf("🔵 NATS %s Results:", pressure)
	t.Logf("   📊 Success: %d/%d (%.1f%%)", finalReceived, messageCount, successRate)
	t.Logf("   🚀 Throughput: %.1f msg/s", throughput)
	t.Logf("   ⏱️  First Latency: %v", firstLatency)
	t.Logf("   🔧 Resources: +%d goroutines, +%.2f MB", 
		peakGoroutines-initialGoroutines, float64(peakMemory-initialMemory)/1024/1024)

	return metrics
}

// generateNATSReport 生成NATS性能报告
func generateNATSReport(t *testing.T, results []*NATSTestMetrics) {
	t.Logf("\n🔵 ===== NATS JETSTREAM PERFORMANCE REPORT =====")
	
	// 检查是否有有效结果
	hasValidResults := false
	for _, result := range results {
		if result.SuccessRate > 0 {
			hasValidResults = true
			break
		}
	}
	
	if !hasValidResults {
		t.Logf("❌ No valid NATS test results - server not available")
		t.Logf("💡 To get NATS performance data:")
		t.Logf("   1. Install: go install github.com/nats-io/nats-server/v2@latest")
		t.Logf("   2. Start: nats-server -js")
		t.Logf("   3. Rerun: go test -v -run TestNATSJetStreamPressure")
		return
	}
	
	// 性能表格
	t.Logf("📊 NATS Performance Table:")
	t.Logf("%-12s | %-8s | %-10s | %-10s | %-10s | %-10s", 
		"Pressure", "Messages", "Success", "Send Rate", "Throughput", "Latency")
	t.Logf("%-12s-+-%-8s-+-%-10s-+-%-10s-+-%-10s-+-%-10s", 
		"------------", "--------", "----------", "----------", "----------", "----------")
	
	for _, result := range results {
		if result.SuccessRate > 0 {
			t.Logf("%-12s | %8d | %9.1f%% | %8.1f/s | %8.1f/s | %8.0fms", 
				result.Pressure,
				result.Messages,
				result.SuccessRate,
				result.SendRate,
				result.Throughput,
				float64(result.FirstLatency.Nanoseconds())/1000000)
		}
	}
	
	// 性能分析
	t.Logf("\n📈 NATS Performance Analysis:")
	
	var totalSuccess, totalThroughput float64
	validCount := 0
	
	for _, result := range results {
		if result.SuccessRate > 0 {
			totalSuccess += result.SuccessRate
			totalThroughput += result.Throughput
			validCount++
			
			// 单项分析
			if result.SuccessRate >= 95 {
				t.Logf("   ✅ %s: Excellent (%.1f%% success, %.1f msg/s)", 
					result.Pressure, result.SuccessRate, result.Throughput)
			} else if result.SuccessRate >= 80 {
				t.Logf("   ⚠️ %s: Good (%.1f%% success, %.1f msg/s)", 
					result.Pressure, result.SuccessRate, result.Throughput)
			} else {
				t.Logf("   ❌ %s: Poor (%.1f%% success, %.1f msg/s)", 
					result.Pressure, result.SuccessRate, result.Throughput)
			}
		}
	}
	
	if validCount > 0 {
		avgSuccess := totalSuccess / float64(validCount)
		avgThroughput := totalThroughput / float64(validCount)
		
		t.Logf("\n📊 NATS Overall Performance:")
		t.Logf("   🔵 Average Success Rate: %.1f%%", avgSuccess)
		t.Logf("   🔵 Average Throughput: %.1f msg/s", avgThroughput)
		
		// 与Kafka对比
		t.Logf("\n⚔️ NATS vs Kafka Comparison:")
		t.Logf("   🔵 NATS Average: %.1f%% success, %.1f msg/s", avgSuccess, avgThroughput)
		t.Logf("   🔴 Kafka Average: 94.3%% success, 11.1 msg/s (from previous tests)")
		
		if avgSuccess > 94.3 && avgThroughput > 11.1 {
			t.Logf("   🏆 Winner: NATS JetStream (better in both metrics)")
		} else if avgSuccess > 94.3 {
			t.Logf("   🏆 NATS wins in reliability, Kafka wins in throughput")
		} else if avgThroughput > 11.1 {
			t.Logf("   🏆 NATS wins in throughput, Kafka wins in reliability")
		} else {
			t.Logf("   🏆 Winner: Kafka (better overall performance)")
		}
	}
	
	t.Logf("\n🔵 NATS JetStream test completed!")
}

// TestNATSQuickCheck NATS快速检查
func TestNATSQuickCheck(t *testing.T) {
	t.Logf("🔵 NATS Quick Connection Check...")
	
	config := &NATSConfig{
		URLs:              []string{"nats://localhost:4222"},
		ClientID:          "nats-quick-check",
		ConnectionTimeout: 5 * time.Second,
	}
	
	eventBus, err := NewNATSEventBus(config)
	if err != nil {
		t.Logf("❌ NATS server not available: %v", err)
		t.Logf("💡 To start NATS server:")
		t.Logf("   # Install NATS server")
		t.Logf("   go install github.com/nats-io/nats-server/v2@latest")
		t.Logf("   ")
		t.Logf("   # Start with JetStream enabled")
		t.Logf("   nats-server -js")
		t.Logf("   ")
		t.Logf("   # Or with config file")
		t.Logf("   nats-server -c nats.conf")
		return
	}
	defer eventBus.Close()
	
	t.Logf("✅ NATS server is available!")
	t.Logf("🚀 Ready to run performance tests")
}
