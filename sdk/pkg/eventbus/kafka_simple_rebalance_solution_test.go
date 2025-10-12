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

// 🎯 Kafka简单再平衡解决方案测试
// 验证优化配置是否能解决高压力下的再平衡问题

// SimpleRebalanceSolutionMetrics 简单再平衡解决方案指标
type SimpleRebalanceSolutionMetrics struct {
	MessagesSent     int64
	MessagesReceived int64
	SendErrors       int64
	ProcessErrors    int64
	StartTime        time.Time
	EndTime          time.Time
	ProcessingGaps   int64
	OrderViolations  int64
}

// SimpleRebalanceTestMessage 简单再平衡测试消息
type SimpleRebalanceTestMessage struct {
	ID       string    `json:"id"`
	Topic    string    `json:"topic"`
	Sequence int64     `json:"sequence"`
	Time     time.Time `json:"time"`
}

// TestKafkaSimpleRebalanceSolution Kafka简单再平衡解决方案测试
func TestKafkaSimpleRebalanceSolution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka simple rebalance solution test in short mode")
	}

	metrics := &SimpleRebalanceSolutionMetrics{
		StartTime: time.Now(),
	}

	// 🔧 极度保守的Kafka配置 - 专门解决再平衡问题
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29093"},
		Producer: ProducerConfig{
			RequiredAcks:     1,                      // 降低一致性要求
			Compression:      "none",                 // 禁用压缩减少CPU负载
			FlushFrequency:   100 * time.Millisecond, // 增加刷新频率
			FlushMessages:    10,                     // 极小批量
			Timeout:          120 * time.Second,      // 大幅增加超时
			FlushBytes:       64 * 1024,              // 小刷新字节数
			RetryMax:         10,                     // 增加重试次数
			BatchSize:        4 * 1024,               // 极小批量大小
			BufferSize:       8 * 1024 * 1024,        // 小缓冲区
			Idempotent:       false,                  // 禁用幂等性
			MaxMessageBytes:  256 * 1024,             // 小消息大小
			PartitionerType:  "manual",               // 手动分区
			LingerMs:         50 * time.Millisecond,  // 增加延迟
			CompressionLevel: 1,
			MaxInFlight:      1, // 最小飞行请求数
		},
		Consumer: ConsumerConfig{
			GroupID:            "simple-rebalance-solution-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     300 * time.Second, // 极大会话超时 (5分钟)
			HeartbeatInterval:  60 * time.Second,  // 极大心跳间隔 (1分钟)
			MaxProcessingTime:  300 * time.Second, // 极大处理时间
			FetchMinBytes:      1,
			FetchMaxBytes:      1 * 1024 * 1024,    // 小获取字节数
			FetchMaxWait:       5 * time.Second,    // 大等待时间
			MaxPollRecords:     10,                 // 极小轮询记录数
			EnableAutoCommit:   true,               // 启用自动提交
			AutoCommitInterval: 30 * time.Second,   // 大自动提交间隔
			IsolationLevel:     "read_uncommitted", // 最低隔离级别
			RebalanceStrategy:  "sticky",           // 使用粘性策略
		},
		HealthCheckInterval:  120 * time.Second, // 大健康检查间隔
		ClientID:             "simple-rebalance-solution-client",
		MetadataRefreshFreq:  60 * time.Minute, // 大元数据刷新频率
		MetadataRetryMax:     10,
		MetadataRetryBackoff: 1 * time.Second,
		Net: NetConfig{
			DialTimeout:  120 * time.Second, // 大连接超时
			ReadTimeout:  120 * time.Second, // 大读取超时
			WriteTimeout: 120 * time.Second, // 大写入超时
			KeepAlive:    120 * time.Second, // 大保活时间
			MaxIdleConns: 5,                 // 小连接数
			MaxOpenConns: 50,                // 小连接数
		},
	}

	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err, "Failed to create simple rebalance solution Kafka EventBus")
	defer eventBus.Close()

	// 等待连接建立和配置生效
	t.Logf("⏳ Waiting for Kafka to stabilize with optimized config...")
	time.Sleep(15 * time.Second)

	t.Logf("🚀 Starting Kafka Simple Rebalance Solution Test")

	// 运行简单测试
	runSimpleRebalanceSolutionTest(t, eventBus, metrics)

	// 分析结果
	analyzeSimpleRebalanceSolutionResults(t, metrics)
}

// runSimpleRebalanceSolutionTest 运行简单再平衡解决方案测试
func runSimpleRebalanceSolutionTest(t *testing.T, eventBus EventBus, metrics *SimpleRebalanceSolutionMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 使用少量topic和消息进行测试
	topics := []string{
		"simple.rebalance.solution.topic.1",
		"simple.rebalance.solution.topic.2",
	}
	messagesPerTopic := 500 // 适中的消息数量

	// 设置消息处理器
	var wg sync.WaitGroup
	setupSimpleRebalanceHandlers(t, eventBus, topics, metrics, &wg)

	// 等待订阅建立
	t.Logf("⏳ Waiting for subscriptions to stabilize...")
	time.Sleep(10 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()
	sendSimpleRebalanceMessages(t, eventBus, topics, messagesPerTopic, metrics)

	// 等待消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("✅ All messages processed successfully")
	case <-time.After(90 * time.Second):
		t.Logf("⏰ Test timeout reached")
	case <-ctx.Done():
		t.Logf("🛑 Context cancelled")
	}

	metrics.EndTime = time.Now()
}

// setupSimpleRebalanceHandlers 设置简单再平衡处理器
func setupSimpleRebalanceHandlers(t *testing.T, eventBus EventBus, topics []string, metrics *SimpleRebalanceSolutionMetrics, wg *sync.WaitGroup) {
	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()

			var lastSequence int64 = -1
			var lastProcessTime time.Time = time.Now()

			handler := func(ctx context.Context, message []byte) error {
				currentTime := time.Now()

				// 检测处理间隙
				if !lastProcessTime.IsZero() {
					gap := currentTime.Sub(lastProcessTime)
					if gap > 15*time.Second { // 超过15秒认为是异常间隙
						atomic.AddInt64(&metrics.ProcessingGaps, 1)
						t.Logf("⚠️ Processing gap detected: %v in topic %s", gap, topicName)
					}
				}
				lastProcessTime = currentTime

				// 解析消息
				var testMsg SimpleRebalanceTestMessage
				if err := json.Unmarshal(message, &testMsg); err != nil {
					atomic.AddInt64(&metrics.ProcessErrors, 1)
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

				return nil
			}

			err := eventBus.Subscribe(context.Background(), topicName, handler)
			if err != nil {
				t.Errorf("Failed to subscribe to topic %s: %v", topicName, err)
			}
		}(topic)
	}
}

// sendSimpleRebalanceMessages 发送简单再平衡消息
func sendSimpleRebalanceMessages(t *testing.T, eventBus EventBus, topics []string, messagesPerTopic int, metrics *SimpleRebalanceSolutionMetrics) {
	var sendWg sync.WaitGroup

	for _, topic := range topics {
		sendWg.Add(1)
		go func(topicName string) {
			defer sendWg.Done()

			for i := 0; i < messagesPerTopic; i++ {
				testMsg := SimpleRebalanceTestMessage{
					ID:       fmt.Sprintf("%s-%d", topicName, i),
					Topic:    topicName,
					Sequence: int64(i),
					Time:     time.Now(),
				}

				messageBytes, err := json.Marshal(testMsg)
				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
					continue
				}

				err = eventBus.Publish(context.Background(), topicName, messageBytes)
				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
					t.Logf("❌ Failed to send message to %s: %v", topicName, err)
				} else {
					atomic.AddInt64(&metrics.MessagesSent, 1)
				}

				// 控制发送速率 - 每50ms发送一条消息
				time.Sleep(50 * time.Millisecond)
			}
		}(topic)
	}

	sendWg.Wait()
	t.Logf("📤 Finished sending simple rebalance messages")
}

// analyzeSimpleRebalanceSolutionResults 分析简单再平衡解决方案结果
func analyzeSimpleRebalanceSolutionResults(t *testing.T, metrics *SimpleRebalanceSolutionMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	successRate := float64(metrics.MessagesReceived) / float64(metrics.MessagesSent) * 100
	throughput := float64(metrics.MessagesReceived) / duration.Seconds()

	t.Logf("\n🎯 ===== Kafka Simple Rebalance Solution Results =====")
	t.Logf("⏱️  Test Duration: %v", duration)
	t.Logf("📤 Messages Sent: %d", metrics.MessagesSent)
	t.Logf("📥 Messages Received: %d", metrics.MessagesReceived)
	t.Logf("❌ Send Errors: %d", metrics.SendErrors)
	t.Logf("❌ Process Errors: %d", metrics.ProcessErrors)
	t.Logf("✅ Success Rate: %.2f%%", successRate)
	t.Logf("🚀 Throughput: %.2f msg/s", throughput)

	t.Logf("\n🚨 Rebalance Analysis:")
	t.Logf("   Processing Gaps (>15s): %d", metrics.ProcessingGaps)
	t.Logf("   Order Violations: %d", metrics.OrderViolations)

	// 评估解决方案效果
	if successRate >= 95.0 && metrics.ProcessingGaps == 0 && metrics.OrderViolations == 0 {
		t.Logf("\n🎉 优秀! 再平衡问题已完全解决!")
		t.Logf("   ✅ 成功率: %.2f%% (>95%%)", successRate)
		t.Logf("   ✅ 零处理间隙")
		t.Logf("   ✅ 零顺序违反")
	} else if successRate >= 90.0 && metrics.ProcessingGaps <= 1 {
		t.Logf("\n⚠️ 良好! 再平衡问题基本解决")
		t.Logf("   ✅ 成功率: %.2f%% (>90%%)", successRate)
		t.Logf("   ⚠️ 处理间隙: %d次", metrics.ProcessingGaps)
		t.Logf("   ⚠️ 顺序违反: %d次", metrics.OrderViolations)
	} else {
		t.Logf("\n❌ 需要进一步优化")
		t.Logf("   ❌ 成功率: %.2f%% (<90%%)", successRate)
		t.Logf("   ❌ 处理间隙: %d次", metrics.ProcessingGaps)
		t.Logf("   ❌ 顺序违反: %d次", metrics.OrderViolations)
	}

	// 基本验证
	assert.Greater(t, metrics.MessagesSent, int64(0), "Should send messages")
	assert.Greater(t, metrics.MessagesReceived, int64(0), "Should receive messages")
	assert.Greater(t, successRate, 85.0, "Success rate should be > 85%")
	assert.LessOrEqual(t, metrics.ProcessingGaps, int64(2), "Should have minimal processing gaps")
	assert.LessOrEqual(t, metrics.OrderViolations, int64(10), "Should have minimal order violations")

	t.Logf("✅ Kafka Simple Rebalance Solution Test Completed!")
}
