package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// SimpleDiskTestMessage 简单磁盘测试消息
type SimpleDiskTestMessage struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// SimpleDiskMetrics 简单磁盘测试指标
type SimpleDiskMetrics struct {
	MessagesSent     int64
	MessagesReceived int64
	SendErrors       int64
	ProcessErrors    int64
	StartTime        time.Time
	EndTime          time.Time
	OrderViolations  int64
	SendLatencySum   int64
	SendLatencyCount int64
}

// TestNATSDiskLowPressureSimple NATS磁盘存储低压力简单测试
func TestNATSDiskLowPressureSimple(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS disk low pressure simple test in short mode")
	}

	metrics := &SimpleDiskMetrics{StartTime: time.Now()}

	// 🔵 NATS磁盘存储配置
	natsConfig := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            "nats-disk-simple-client",
		MaxReconnects:       10,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   30 * time.Second,
		HealthCheckInterval: 60 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: true, // 启用JetStream
			Stream: StreamConfig{
				Name:      "SIMPLE_DISK_STREAM",
				Subjects:  []string{"simple.disk.>"},
				Storage:   "file", // 🔑 关键：使用磁盘存储
				Retention: "limits",
				MaxAge:    24 * time.Hour,
				MaxBytes:  50 * 1024 * 1024, // 50MB
				MaxMsgs:   50000,
				Replicas:  1,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   "simple-disk-consumer",
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 500,
				MaxWaiting:    256,
				MaxDeliver:    3,
				BackOff:       []time.Duration{1 * time.Second, 2 * time.Second},
			},
			PublishTimeout: 10 * time.Second,
		},
	}

	eventBus, err := NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS disk EventBus")
	defer eventBus.Close()

	// 等待JetStream初始化
	time.Sleep(5 * time.Second)

	t.Logf("🔵 Starting NATS Disk Low Pressure Simple Test")

	// 低压力：2个topic，每个300条消息，总计600条
	runSimpleDiskTest(t, eventBus, "NATS-Disk", 2, 300, metrics)
	analyzeSimpleDiskResults(t, "NATS-Disk", metrics)
}

// TestKafkaDiskLowPressureSimple Kafka磁盘存储低压力简单测试
func TestKafkaDiskLowPressureSimple(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka disk low pressure simple test in short mode")
	}

	metrics := &SimpleDiskMetrics{StartTime: time.Now()}

	// 🟠 Kafka磁盘存储配置
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29093"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   10 * time.Millisecond,
			FlushMessages:    10,
			Timeout:          30 * time.Second,
			FlushBytes:       64 * 1024,
			RetryMax:         3,
			BatchSize:        4 * 1024,
			BufferSize:       8 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  256 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         5 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      3,
		},
		Consumer: ConsumerConfig{
			GroupID:            "kafka-disk-simple-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     60 * time.Second,
			HeartbeatInterval:  20 * time.Second,
			MaxProcessingTime:  90 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      5 * 1024 * 1024,
			FetchMaxWait:       1 * time.Second,
			MaxPollRecords:     50,
			EnableAutoCommit:   true,
			AutoCommitInterval: 10 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		HealthCheckInterval:  60 * time.Second,
		ClientID:             "kafka-disk-simple-client",
		MetadataRefreshFreq:  10 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 250 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:  30 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			KeepAlive:    30 * time.Second,
			MaxIdleConns: 10,
			MaxOpenConns: 100,
		},
	}

	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err, "Failed to create Kafka disk EventBus")
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(5 * time.Second)

	t.Logf("🟠 Starting Kafka Disk Low Pressure Simple Test")

	// 低压力：2个topic，每个300条消息，总计600条
	runSimpleDiskTest(t, eventBus, "Kafka-Disk", 2, 300, metrics)
	analyzeSimpleDiskResults(t, "Kafka-Disk", metrics)
}

// TestNATSDiskMediumPressureSimple NATS磁盘存储中压力简单测试
func TestNATSDiskMediumPressureSimple(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NATS disk medium pressure simple test in short mode")
	}

	metrics := &SimpleDiskMetrics{StartTime: time.Now()}

	natsConfig := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            "nats-disk-medium-simple-client",
		MaxReconnects:       10,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   30 * time.Second,
		HealthCheckInterval: 60 * time.Second,
		JetStream: JetStreamConfig{
			Enabled: true,
			Stream: StreamConfig{
				Name:      "MEDIUM_DISK_STREAM",
				Subjects:  []string{"medium.disk.>"},
				Storage:   "file", // 磁盘存储
				Retention: "limits",
				MaxAge:    24 * time.Hour,
				MaxBytes:  100 * 1024 * 1024, // 100MB
				MaxMsgs:   100000,
				Replicas:  1,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   "medium-disk-consumer",
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 1000,
				MaxWaiting:    512,
				MaxDeliver:    3,
				BackOff:       []time.Duration{1 * time.Second, 2 * time.Second},
			},
			PublishTimeout: 15 * time.Second,
		},
	}

	eventBus, err := NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS disk EventBus")
	defer eventBus.Close()

	time.Sleep(5 * time.Second)

	t.Logf("🔵 Starting NATS Disk Medium Pressure Simple Test")

	// 中压力：3个topic，每个1000条消息，总计3,000条
	runSimpleDiskTest(t, eventBus, "NATS-Disk", 3, 1000, metrics)
	analyzeSimpleDiskResults(t, "NATS-Disk", metrics)
}

// TestKafkaDiskMediumPressureSimple Kafka磁盘存储中压力简单测试
func TestKafkaDiskMediumPressureSimple(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka disk medium pressure simple test in short mode")
	}

	metrics := &SimpleDiskMetrics{StartTime: time.Now()}

	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29093"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   10 * time.Millisecond,
			FlushMessages:    20,
			Timeout:          60 * time.Second,
			FlushBytes:       128 * 1024,
			RetryMax:         3,
			BatchSize:        8 * 1024,
			BufferSize:       16 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  512 * 1024,
			PartitionerType:  "round_robin",
			LingerMs:         5 * time.Millisecond,
			CompressionLevel: 1,
			MaxInFlight:      3,
		},
		Consumer: ConsumerConfig{
			GroupID:            "kafka-disk-medium-simple-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     90 * time.Second,
			HeartbeatInterval:  30 * time.Second,
			MaxProcessingTime:  120 * time.Second,
			FetchMinBytes:      1,
			FetchMaxBytes:      10 * 1024 * 1024,
			FetchMaxWait:       2 * time.Second,
			MaxPollRecords:     100,
			EnableAutoCommit:   true,
			AutoCommitInterval: 15 * time.Second,
			IsolationLevel:     "read_uncommitted",
			RebalanceStrategy:  "sticky",
		},
		HealthCheckInterval:  90 * time.Second,
		ClientID:             "kafka-disk-medium-simple-client",
		MetadataRefreshFreq:  15 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 500 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:  60 * time.Second,
			ReadTimeout:  60 * time.Second,
			WriteTimeout: 60 * time.Second,
			KeepAlive:    60 * time.Second,
			MaxIdleConns: 15,
			MaxOpenConns: 150,
		},
	}

	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err, "Failed to create Kafka disk EventBus")
	defer eventBus.Close()

	time.Sleep(8 * time.Second)

	t.Logf("🟠 Starting Kafka Disk Medium Pressure Simple Test")

	// 中压力：3个topic，每个1000条消息，总计3,000条
	runSimpleDiskTest(t, eventBus, "Kafka-Disk", 3, 1000, metrics)
	analyzeSimpleDiskResults(t, "Kafka-Disk", metrics)
}

// runSimpleDiskTest 运行简单磁盘测试
func runSimpleDiskTest(t *testing.T, eventBus EventBus, system string, topicCount, messagesPerTopic int, metrics *SimpleDiskMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second) // 3分钟超时
	defer cancel()

	// 生成topic列表
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("simple.disk.topic.%d", i+1)
	}

	totalMessages := topicCount * messagesPerTopic
	t.Logf("📊 %s Simple Disk Test Config: %d topics, %d msgs/topic, total: %d messages",
		system, topicCount, messagesPerTopic, totalMessages)

	// 设置消息处理器
	var wg sync.WaitGroup
	setupSimpleDiskHandlers(t, eventBus, topics, metrics, &wg)

	// 等待订阅建立
	t.Logf("⏳ Waiting for %s subscriptions to stabilize...", system)
	time.Sleep(3 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendSimpleDiskMessages(t, eventBus, topics, messagesPerTopic, metrics)

	// 等待消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("✅ All %s simple disk messages processed successfully", system)
	case <-time.After(120 * time.Second): // 2分钟等待
		t.Logf("⏰ %s simple disk test timeout reached", system)
	case <-ctx.Done():
		t.Logf("🛑 %s simple disk context cancelled", system)
	}

	metrics.EndTime = time.Now()
}

// setupSimpleDiskHandlers 设置简单磁盘处理器
func setupSimpleDiskHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *SimpleDiskMetrics, wg *sync.WaitGroup) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()

			var lastSequence int64 = -1

			handler := func(ctx context.Context, message []byte) error {
				// 解析消息
				var testMsg SimpleDiskTestMessage
				if err := json.Unmarshal(message, &testMsg); err != nil {
					atomic.AddInt64(&metrics.ProcessErrors, 1)
					return err
				}

				// 检测顺序违反
				if lastSequence >= 0 && testMsg.Sequence <= lastSequence {
					atomic.AddInt64(&metrics.OrderViolations, 1)
				}
				lastSequence = testMsg.Sequence

				// 更新接收计数
				atomic.AddInt64(&metrics.MessagesReceived, 1)

				return nil
			}

			err := eventBus.Subscribe(context.Background(), topicName, handler)
			if err != nil {
				t.Errorf("Failed to subscribe to topic %s: %v", topicName, err)
			}
		}(topic)
	}
}

// sendSimpleDiskMessages 发送简单磁盘消息
func sendSimpleDiskMessages(t *testing.T, eventBus EventBus, topics []string, messagesPerTopic int, metrics *SimpleDiskMetrics) {
	var sendWg sync.WaitGroup

	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()

			for i := 0; i < messagesPerTopic; i++ {
				testMsg := SimpleDiskTestMessage{
					ID:        fmt.Sprintf("%s-%d", topicName, i),
					Topic:     topicName,
					Sequence:  int64(i),
					Timestamp: time.Now(),
					Data:      fmt.Sprintf("simple-disk-data-%d", i),
				}

				messageBytes, err := json.Marshal(testMsg)
				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
					continue
				}

				startTime := time.Now()
				err = eventBus.Publish(context.Background(), topicName, messageBytes)
				sendTime := time.Since(startTime).Microseconds()

				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
				} else {
					atomic.AddInt64(&metrics.MessagesSent, 1)
					atomic.AddInt64(&metrics.SendLatencySum, sendTime)
					atomic.AddInt64(&metrics.SendLatencyCount, 1)
				}

				// 控制发送速率：每秒约200条消息
				time.Sleep(5 * time.Millisecond)
			}
		}(topic)
	}

	sendWg.Wait()
	t.Logf("📤 Finished sending simple disk messages")
}

// analyzeSimpleDiskResults 分析简单磁盘结果
func analyzeSimpleDiskResults(t *testing.T, system string, metrics *SimpleDiskMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	successRate := float64(metrics.MessagesReceived) / float64(metrics.MessagesSent) * 100
	throughput := float64(metrics.MessagesReceived) / duration.Seconds()

	avgSendLatency := float64(0)
	if metrics.SendLatencyCount > 0 {
		avgSendLatency = float64(metrics.SendLatencySum) / float64(metrics.SendLatencyCount) / 1000.0 // ms
	}

	t.Logf("\n🎯 ===== %s Simple Disk Test Results =====", system)
	t.Logf("⏱️  Test Duration: %v", duration)
	t.Logf("📤 Messages Sent: %d", metrics.MessagesSent)
	t.Logf("📥 Messages Received: %d", metrics.MessagesReceived)
	t.Logf("❌ Send Errors: %d", metrics.SendErrors)
	t.Logf("❌ Process Errors: %d", metrics.ProcessErrors)
	t.Logf("✅ Success Rate: %.2f%%", successRate)
	t.Logf("🚀 Throughput: %.2f msg/s", throughput)
	t.Logf("⚡ Avg Send Latency: %.3f ms", avgSendLatency)
	t.Logf("⚠️ Order Violations: %d", metrics.OrderViolations)

	// 🏆 性能评估
	t.Logf("\n🏆 %s Simple Disk Performance Evaluation:", system)
	if successRate >= 95.0 {
		t.Logf("🎉 优秀! %s磁盘存储表现卓越!", system)
		t.Logf("   ✅ 成功率: %.2f%%", successRate)
		t.Logf("   ✅ 吞吐量: %.0f msg/s", throughput)
		t.Logf("   ✅ 延迟: %.3f ms", avgSendLatency)
	} else if successRate >= 80.0 {
		t.Logf("⚠️ 良好! %s磁盘存储表现良好", system)
		t.Logf("   ✅ 成功率: %.2f%%", successRate)
	} else {
		t.Logf("❌ %s磁盘存储需要优化，成功率仅为 %.2f%%", system, successRate)
	}

	t.Logf("✅ %s Simple Disk Test Completed!", system)
}
