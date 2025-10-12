package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 🎯 Kafka再平衡现象检测测试
// 专门检测统一Consumer架构是否解决了再平衡问题

// RebalanceDetectionMetrics 再平衡检测指标
type RebalanceDetectionMetrics struct {
	// 消息处理指标
	MessagesSent       int64 // 发送消息总数
	MessagesReceived   int64 // 接收消息总数
	ProcessingGaps     int64 // 处理间隙次数 (可能的再平衡)
	
	// 时间指标
	StartTime          time.Time
	EndTime            time.Time
	LastMessageTime    time.Time
	MaxGapDuration     time.Duration // 最大处理间隙
	
	// 再平衡检测
	SuspectedRebalances int64 // 疑似再平衡次数
	ConnectionLosses    int64 // 连接丢失次数
	
	// 消息顺序
	OrderViolations    int64 // 顺序违反次数
	DuplicateMessages  int64 // 重复消息次数
}

// RebalanceTestMessage 再平衡测试消息
type RebalanceTestMessage struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

// TestKafkaRebalanceDetection Kafka再平衡检测测试
func TestKafkaRebalanceDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka rebalance detection test in short mode")
	}

	metrics := &RebalanceDetectionMetrics{
		StartTime: time.Now(),
	}

	// 创建Kafka EventBus配置 (使用基准测试端口)
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29093"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   10 * time.Millisecond,
			FlushMessages:    50,
			Timeout:          10 * time.Second,
			FlushBytes:       1024 * 1024,
			RetryMax:         3,
			BatchSize:        16 * 1024,
			BufferSize:       32 * 1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  1024 * 1024,
			PartitionerType:  "hash",
			LingerMs:         5 * time.Millisecond,
			CompressionLevel: 6,
			MaxInFlight:      5,
		},
		Consumer: ConsumerConfig{
			GroupID:            "rebalance-detection-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second, // 增加超时以减少再平衡
			HeartbeatInterval:  10 * time.Second, // 增加心跳间隔
			MaxProcessingTime:  60 * time.Second,
			FetchMinBytes:      1024,
			FetchMaxBytes:      50 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     100, // 减少批量大小
			EnableAutoCommit:   false,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_committed",
			RebalanceStrategy:  "range", // 使用range策略
		},
		HealthCheckInterval:  30 * time.Second,
		ClientID:             "rebalance-detection-client",
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
	require.NoError(t, err, "Failed to create Kafka EventBus")
	defer eventBus.Close()

	// 等待连接建立
	time.Sleep(5 * time.Second)

	t.Logf("🔍 Starting Kafka Rebalance Detection Test")
	
	// 运行再平衡检测测试
	runRebalanceDetectionTest(t, eventBus, metrics)

	// 分析结果
	analyzeRebalanceResults(t, metrics)
}

// runRebalanceDetectionTest 运行再平衡检测测试
func runRebalanceDetectionTest(t *testing.T, eventBus EventBus, metrics *RebalanceDetectionMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 创建测试topic
	topics := []string{
		"rebalance.detection.topic.1",
		"rebalance.detection.topic.2",
		"rebalance.detection.topic.3",
	}

	// 设置消息处理器 (检测再平衡)
	var wg sync.WaitGroup
	setupRebalanceDetectionHandlers(t, eventBus, topics, metrics, &wg)

	// 等待订阅建立
	time.Sleep(3 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendRebalanceDetectionMessages(t, eventBus, topics, metrics)

	// 等待消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("✅ All messages processed")
	case <-time.After(45 * time.Second):
		t.Logf("⏰ Test timeout reached")
	case <-ctx.Done():
		t.Logf("🛑 Context cancelled")
	}

	metrics.EndTime = time.Now()
}

// setupRebalanceDetectionHandlers 设置再平衡检测处理器
func setupRebalanceDetectionHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *RebalanceDetectionMetrics, wg *sync.WaitGroup) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()
			
			var lastSequence int64 = -1
			var lastProcessTime time.Time = time.Now()
			
			handler := func(ctx context.Context, message []byte) error {
				currentTime := time.Now()
				
				// 检测处理间隙 (可能的再平衡)
				if !lastProcessTime.IsZero() {
					gap := currentTime.Sub(lastProcessTime)
					if gap > 5*time.Second { // 超过5秒认为是异常间隙
						atomic.AddInt64(&metrics.ProcessingGaps, 1)
						if gap > 10*time.Second { // 超过10秒认为是再平衡
							atomic.AddInt64(&metrics.SuspectedRebalances, 1)
							t.Logf("🚨 Suspected rebalance detected: %v gap in topic %s", gap, topicName)
						}
						
						// 更新最大间隙
						if gap > metrics.MaxGapDuration {
							metrics.MaxGapDuration = gap
						}
					}
				}
				lastProcessTime = currentTime
				
				// 解析消息
				var testMsg RebalanceTestMessage
				if err := json.Unmarshal(message, &testMsg); err != nil {
					return err
				}
				
				// 检测顺序违反
				if lastSequence >= 0 && testMsg.Sequence <= lastSequence {
					atomic.AddInt64(&metrics.OrderViolations, 1)
					t.Logf("⚠️ Order violation in topic %s: expected > %d, got %d", 
						topicName, lastSequence, testMsg.Sequence)
				}
				lastSequence = testMsg.Sequence
				
				// 更新接收计数
				atomic.AddInt64(&metrics.MessagesReceived, 1)
				metrics.LastMessageTime = currentTime
				
				return nil
			}
			
			err := eventBus.Subscribe(context.Background(), topicName, handler)
			if err != nil {
				atomic.AddInt64(&metrics.ConnectionLosses, 1)
				t.Errorf("Failed to subscribe to topic %s: %v", topicName, err)
			}
		}(topic)
	}
}

// sendRebalanceDetectionMessages 发送再平衡检测消息
func sendRebalanceDetectionMessages(t *testing.T, eventBus EventBus, topics []string, metrics *RebalanceDetectionMetrics) {
	var sendWg sync.WaitGroup
	messagesPerTopic := 200 // 每个topic发送200条消息
	
	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()
			
			for i := 0; i < messagesPerTopic; i++ {
				testMsg := RebalanceTestMessage{
					ID:        fmt.Sprintf("%s-%d", topicName, i),
					Topic:     topicName,
					Sequence:  int64(i),
					Timestamp: time.Now(),
					Data:      fmt.Sprintf("rebalance-test-data-%d", i),
				}
				
				messageBytes, err := json.Marshal(testMsg)
				if err != nil {
					continue
				}
				
				err = eventBus.Publish(context.Background(), topicName, messageBytes)
				if err != nil {
					t.Logf("❌ Failed to send message to %s: %v", topicName, err)
				} else {
					atomic.AddInt64(&metrics.MessagesSent, 1)
				}
				
				// 控制发送速率
				if i%20 == 0 {
					time.Sleep(100 * time.Millisecond)
				}
			}
		}(topic)
	}
	
	sendWg.Wait()
	t.Logf("📤 Finished sending rebalance detection messages")
}

// analyzeRebalanceResults 分析再平衡结果
func analyzeRebalanceResults(t *testing.T, metrics *RebalanceDetectionMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	successRate := float64(metrics.MessagesReceived) / float64(metrics.MessagesSent) * 100
	
	t.Logf("\n🔍 ===== Kafka Rebalance Detection Results =====")
	t.Logf("⏱️  Test Duration: %v", duration)
	t.Logf("📤 Messages Sent: %d", metrics.MessagesSent)
	t.Logf("📥 Messages Received: %d", metrics.MessagesReceived)
	t.Logf("✅ Success Rate: %.2f%%", successRate)
	
	t.Logf("\n🚨 Rebalance Detection:")
	t.Logf("   Processing Gaps (>5s): %d", metrics.ProcessingGaps)
	t.Logf("   Suspected Rebalances (>10s): %d", metrics.SuspectedRebalances)
	t.Logf("   Max Gap Duration: %v", metrics.MaxGapDuration)
	t.Logf("   Connection Losses: %d", metrics.ConnectionLosses)
	
	t.Logf("\n⚠️ Message Order Issues:")
	t.Logf("   Order Violations: %d", metrics.OrderViolations)
	t.Logf("   Duplicate Messages: %d", metrics.DuplicateMessages)
	
	// 评估再平衡情况
	if metrics.SuspectedRebalances == 0 {
		t.Logf("\n✅ 优秀! 未检测到明显的再平衡现象")
	} else if metrics.SuspectedRebalances <= 2 {
		t.Logf("\n⚠️ 检测到少量再平衡现象 (%d次)", metrics.SuspectedRebalances)
	} else {
		t.Logf("\n❌ 检测到频繁的再平衡现象 (%d次)", metrics.SuspectedRebalances)
	}
	
	// 基本验证
	assert.Greater(t, metrics.MessagesSent, int64(0), "Should send messages")
	assert.Greater(t, metrics.MessagesReceived, int64(0), "Should receive messages")
	assert.Greater(t, successRate, 80.0, "Success rate should be > 80%")
	assert.LessOrEqual(t, metrics.SuspectedRebalances, int64(3), "Should have minimal rebalances")
	
	t.Logf("✅ Kafka Rebalance Detection Test Completed!")
}
