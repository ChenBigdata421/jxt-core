package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestConsumerWarmupAnalysis 分析Consumer预热问题
func TestConsumerWarmupAnalysis(t *testing.T) {
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: "warmup-analysis-test",
		Producer: ProducerConfig{
			MaxMessageBytes: 1024 * 1024,
			RequiredAcks:    1,
			Timeout:         5 * time.Second,
			Compression:     "none",
			MaxInFlight:     10,
		},
		Consumer: ConsumerConfig{
			GroupID:            "warmup-analysis-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     6 * time.Second,
			HeartbeatInterval:  2 * time.Second,
			MaxProcessingTime:  3 * time.Second,
			FetchMinBytes:      1024 * 1024,
			FetchMaxBytes:      50 * 1024 * 1024,
			FetchMaxWait:       50 * time.Millisecond,
			MaxPollRecords:     1000,
			EnableAutoCommit:   true,
			AutoCommitInterval: 500 * time.Millisecond,
		},
	}

	eventBus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("Failed to create EventBus: %v", err)
	}
	defer eventBus.Close()

	kafkaBus := eventBus.(*kafkaEventBus)
	testTopic := "warmup.analysis.test"
	kafkaBus.allPossibleTopics = []string{testTopic}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 记录时间点
	var (
		subscribeTime    time.Time
		firstMessageTime time.Time
		receivedCount    int64
	)

	handler := func(ctx context.Context, message []byte) error {
		if atomic.AddInt64(&receivedCount, 1) == 1 {
			firstMessageTime = time.Now()
		}
		return nil
	}

	t.Logf("🔍 Starting Consumer Warmup Analysis...")

	// 1. 记录订阅时间
	subscribeStart := time.Now()
	err = eventBus.Subscribe(ctx, testTopic, handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	subscribeTime = time.Now()
	subscribeLatency := subscribeTime.Sub(subscribeStart)

	t.Logf("📊 Subscribe completed in: %v", subscribeLatency)

	// 2. 测试不同预热时间的效果
	warmupTimes := []time.Duration{
		0 * time.Second,    // 无预热
		1 * time.Second,    // 1秒预热
		3 * time.Second,    // 3秒预热
		5 * time.Second,    // 5秒预热
		10 * time.Second,   // 10秒预热
	}

	for _, warmupTime := range warmupTimes {
		t.Logf("\n🧪 Testing with %v warmup time...", warmupTime)
		
		// 重置计数器
		atomic.StoreInt64(&receivedCount, 0)
		firstMessageTime = time.Time{}

		// 预热等待
		if warmupTime > 0 {
			t.Logf("⏳ Warming up for %v...", warmupTime)
			time.Sleep(warmupTime)
		}

		// 发送测试消息
		messageCount := 50
		sendStart := time.Now()
		
		for i := 0; i < messageCount; i++ {
			message := map[string]interface{}{
				"id":       fmt.Sprintf("warmup-msg-%d", i),
				"content":  fmt.Sprintf("Warmup test message %d", i),
				"sendTime": time.Now().Format(time.RFC3339Nano),
				"index":    i,
			}

			messageBytes, _ := json.Marshal(message)
			err := eventBus.Publish(ctx, testTopic, messageBytes)
			if err != nil {
				t.Logf("Failed to publish message %d: %v", i, err)
			}
		}
		sendEnd := time.Now()
		sendDuration := sendEnd.Sub(sendStart)

		t.Logf("📤 Sent %d messages in %v (%.2f msg/s)", 
			messageCount, sendDuration, float64(messageCount)/sendDuration.Seconds())

		// 等待消息处理
		waitTime := 15 * time.Second
		time.Sleep(waitTime)

		// 分析结果
		finalCount := atomic.LoadInt64(&receivedCount)
		successRate := float64(finalCount) / float64(messageCount) * 100

		var firstMessageLatency time.Duration
		if !firstMessageTime.IsZero() {
			firstMessageLatency = firstMessageTime.Sub(sendStart)
		}

		t.Logf("📊 Results for %v warmup:", warmupTime)
		t.Logf("   📥 Received: %d/%d (%.2f%%)", finalCount, messageCount, successRate)
		t.Logf("   ⏱️  First message latency: %v", firstMessageLatency)
		t.Logf("   🚀 Effective throughput: %.2f msg/s", 
			float64(finalCount)/waitTime.Seconds())
	}
}

// TestProducerBottleneckAnalysis 分析Producer瓶颈
func TestProducerBottleneckAnalysis(t *testing.T) {
	configs := []struct {
		name        string
		maxInFlight int
		compression string
	}{
		{"Default", 10, "none"},
		{"HighConcurrency", 50, "none"},
		{"WithCompression", 10, "snappy"},
		{"Optimized", 50, "snappy"},
	}

	for _, cfg := range configs {
		t.Logf("\n🧪 Testing Producer config: %s", cfg.name)
		
		config := &KafkaConfig{
			Brokers:  []string{"localhost:29094"},
			ClientID: fmt.Sprintf("producer-analysis-%s", cfg.name),
			Producer: ProducerConfig{
				MaxMessageBytes: 1024 * 1024,
				RequiredAcks:    1,
				Timeout:         5 * time.Second,
				Compression:     cfg.compression,
				MaxInFlight:     cfg.maxInFlight,
			},
			Consumer: ConsumerConfig{
				GroupID:            fmt.Sprintf("producer-analysis-%s-group", cfg.name),
				AutoOffsetReset:    "earliest",
				SessionTimeout:     6 * time.Second,
				HeartbeatInterval:  2 * time.Second,
				MaxProcessingTime:  3 * time.Second,
			},
		}

		eventBus, err := NewKafkaEventBus(config)
		if err != nil {
			t.Fatalf("Failed to create EventBus: %v", err)
		}

		testTopic := fmt.Sprintf("producer.analysis.%s", cfg.name)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		// 测试发送性能
		messageCount := 200
		sendStart := time.Now()
		errors := 0

		for i := 0; i < messageCount; i++ {
			message := map[string]interface{}{
				"id":      fmt.Sprintf("producer-msg-%d", i),
				"content": fmt.Sprintf("Producer test message %d", i),
				"config":  cfg.name,
			}

			messageBytes, _ := json.Marshal(message)
			err := eventBus.Publish(ctx, testTopic, messageBytes)
			if err != nil {
				errors++
				if errors <= 5 {
					t.Logf("Publish error %d: %v", errors, err)
				}
			}
		}

		sendEnd := time.Now()
		sendDuration := sendEnd.Sub(sendStart)
		sendRate := float64(messageCount-errors) / sendDuration.Seconds()

		t.Logf("📊 Producer Results for %s:", cfg.name)
		t.Logf("   📤 Sent: %d/%d (%.2f%% success)", messageCount-errors, messageCount, 
			float64(messageCount-errors)/float64(messageCount)*100)
		t.Logf("   ⏱️  Duration: %v", sendDuration)
		t.Logf("   🚀 Send rate: %.2f msg/s", sendRate)
		t.Logf("   ⚠️ Errors: %d", errors)

		eventBus.Close()
		cancel()
	}
}

// TestBatchProcessingAnalysis 分析批量处理效应
func TestBatchProcessingAnalysis(t *testing.T) {
	batchSizes := []int{100, 500, 1000, 2000}

	for _, batchSize := range batchSizes {
		t.Logf("\n🧪 Testing batch size: %d", batchSize)

		config := &KafkaConfig{
			Brokers:  []string{"localhost:29094"},
			ClientID: fmt.Sprintf("batch-analysis-%d", batchSize),
			Producer: ProducerConfig{
				MaxMessageBytes: 1024 * 1024,
				RequiredAcks:    1,
				Timeout:         5 * time.Second,
				Compression:     "none",
				MaxInFlight:     10,
			},
			Consumer: ConsumerConfig{
				GroupID:            fmt.Sprintf("batch-analysis-%d-group", batchSize),
				AutoOffsetReset:    "earliest",
				SessionTimeout:     6 * time.Second,
				HeartbeatInterval:  2 * time.Second,
				MaxProcessingTime:  3 * time.Second,
				FetchMinBytes:      1024 * 1024,
				FetchMaxBytes:      50 * 1024 * 1024,
				FetchMaxWait:       50 * time.Millisecond,
				MaxPollRecords:     batchSize,
				EnableAutoCommit:   true,
				AutoCommitInterval: 500 * time.Millisecond,
			},
		}

		eventBus, err := NewKafkaEventBus(config)
		if err != nil {
			t.Fatalf("Failed to create EventBus: %v", err)
		}

		kafkaBus := eventBus.(*kafkaEventBus)
		testTopic := fmt.Sprintf("batch.analysis.%d", batchSize)
		kafkaBus.allPossibleTopics = []string{testTopic}

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)

		var receivedCount int64
		handler := func(ctx context.Context, message []byte) error {
			atomic.AddInt64(&receivedCount, 1)
			return nil
		}

		err = eventBus.Subscribe(ctx, testTopic, handler)
		if err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}

		// 预热
		time.Sleep(5 * time.Second)

		// 发送消息
		messageCount := 500
		sendStart := time.Now()

		for i := 0; i < messageCount; i++ {
			message := map[string]interface{}{
				"id":        fmt.Sprintf("batch-msg-%d", i),
				"content":   fmt.Sprintf("Batch test message %d", i),
				"batchSize": batchSize,
			}

			messageBytes, _ := json.Marshal(message)
			err := eventBus.Publish(ctx, testTopic, messageBytes)
			if err != nil {
				t.Logf("Failed to publish message %d: %v", i, err)
			}
		}

		sendEnd := time.Now()
		sendDuration := sendEnd.Sub(sendStart)

		// 等待处理
		time.Sleep(20 * time.Second)

		finalCount := atomic.LoadInt64(&receivedCount)
		successRate := float64(finalCount) / float64(messageCount) * 100
		throughput := float64(finalCount) / (time.Since(sendStart).Seconds())

		t.Logf("📊 Batch Results for size %d:", batchSize)
		t.Logf("   📤 Send rate: %.2f msg/s", float64(messageCount)/sendDuration.Seconds())
		t.Logf("   📥 Received: %d/%d (%.2f%%)", finalCount, messageCount, successRate)
		t.Logf("   🚀 Throughput: %.2f msg/s", throughput)

		eventBus.Close()
		cancel()
	}
}
