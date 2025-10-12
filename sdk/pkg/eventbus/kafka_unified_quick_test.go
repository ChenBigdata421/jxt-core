package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 🚀 Kafka UnifiedConsumer 快速验证测试
// 验证基本功能：10个topic，envelope消息，聚合ID顺序处理

// QuickTestConfig 快速测试配置
type QuickTestConfig struct {
	TopicCount     int
	MessagesPerTopic int
	AggregateCount int
	WorkerPoolSize int
}

// QuickTestMetrics 快速测试指标
type QuickTestMetrics struct {
	MessagesSent     int64
	MessagesReceived int64
	OrderViolations  int64
	ProcessingErrors int64
	AggregateStats   sync.Map // map[string]*QuickAggregateStats
}

// QuickAggregateStats 快速聚合统计
type QuickAggregateStats struct {
	MessageCount    int64
	LastSequence    int64
	OrderViolations int64
}

// TestKafkaUnifiedConsumerQuickValidation 快速验证测试
func TestKafkaUnifiedConsumerQuickValidation(t *testing.T) {
	config := QuickTestConfig{
		TopicCount:       10,
		MessagesPerTopic: 100,
		AggregateCount:   20,
		WorkerPoolSize:   10,
	}

	metrics := &QuickTestMetrics{}

	t.Logf("🚀 开始Kafka UnifiedConsumer快速验证测试")
	t.Logf("📊 配置: %d topics, %d messages/topic, %d aggregates, %d workers",
		config.TopicCount, config.MessagesPerTopic, config.AggregateCount, config.WorkerPoolSize)

	// 创建Kafka EventBus
	kafkaConfig := createQuickTestKafkaConfig()
	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err)
	defer eventBus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 生成topic列表
	topics := make([]string, config.TopicCount)
	for i := 0; i < config.TopicCount; i++ {
		topics[i] = fmt.Sprintf("quick-test-topic-%d", i)
	}

	// 设置订阅者
	t.Log("📡 设置UnifiedConsumer订阅者...")
	err = setupQuickTestSubscribers(ctx, eventBus, topics, config, metrics)
	require.NoError(t, err)

	// 等待订阅者准备就绪
	time.Sleep(2 * time.Second)

	// 发送测试消息
	t.Log("📤 发送测试消息...")
	err = sendQuickTestMessages(ctx, eventBus, topics, config, metrics)
	require.NoError(t, err)

	// 等待消息处理完成
	t.Log("⏳ 等待消息处理完成...")
	waitForQuickTestCompletion(config, metrics, t)

	// 验证结果
	t.Log("✅ 验证测试结果...")
	validateQuickTestResults(config, metrics, t)
}

// createQuickTestKafkaConfig 创建快速测试Kafka配置
func createQuickTestKafkaConfig() *KafkaConfig {
	return &KafkaConfig{
		Brokers: []string{"localhost:29092"},
		Producer: ProducerConfig{
			RequiredAcks:     1, // 简单确认，非幂等性
			Compression:      "snappy",
			FlushFrequency:   10 * time.Millisecond,
			FlushMessages:    50,
			Timeout:          30 * time.Second,
			FlushBytes:       1024 * 1024,
			RetryMax:         3,
			BatchSize:        16 * 1024,
			BufferSize:       32 * 1024 * 1024,
			Idempotent:       false, // 禁用幂等性以简化测试
			MaxMessageBytes:  1024 * 1024,
			PartitionerType:  "hash",
			LingerMs:         5 * time.Millisecond,
			CompressionLevel: 6,
			MaxInFlight:      5, // 非幂等性可以使用>1
		},
		Consumer: ConsumerConfig{
			GroupID:            "quick-test-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  30 * time.Second,
			FetchMinBytes:      1024,
			FetchMaxBytes:      50 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     500,
			EnableAutoCommit:   false,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_committed",
			RebalanceStrategy:  "range",
		},
		HealthCheckInterval:  30 * time.Second,
		ClientID:            "quick-test-client",
		MetadataRefreshFreq: 10 * time.Minute,
		MetadataRetryMax:    3,
		MetadataRetryBackoff: 250 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:   30 * time.Second,
			ReadTimeout:   30 * time.Second,
			WriteTimeout:  30 * time.Second,
			KeepAlive:     30 * time.Second,
			MaxIdleConns:  10,
			MaxOpenConns:  100,
		},
		Security: SecurityConfig{Enabled: false},
	}
}

// setupQuickTestSubscribers 设置快速测试订阅者
func setupQuickTestSubscribers(ctx context.Context, eventBus EventBus, topics []string, config QuickTestConfig, metrics *QuickTestMetrics) error {
	var wg sync.WaitGroup

	for _, topic := range topics {
		wg.Add(1)
		go func(topicName string) {
			defer wg.Done()

			handler := createQuickTestHandler(topicName, metrics)
			err := eventBus.Subscribe(ctx, topicName, handler)
			if err != nil {
				panic(fmt.Sprintf("Failed to subscribe to topic %s: %v", topicName, err))
			}
		}(topic)
	}

	wg.Wait()
	return nil
}

// createQuickTestHandler 创建快速测试处理器
func createQuickTestHandler(topic string, metrics *QuickTestMetrics) MessageHandler {
	return func(ctx context.Context, message []byte) error {
		// 解析Envelope消息
		var envelope TestEnvelope
		if err := json.Unmarshal(message, &envelope); err != nil {
			atomic.AddInt64(&metrics.ProcessingErrors, 1)
			return fmt.Errorf("failed to unmarshal envelope: %w", err)
		}

		// 更新接收计数
		atomic.AddInt64(&metrics.MessagesReceived, 1)

		// 检查消息顺序
		checkQuickTestOrder(envelope, metrics)

		// 模拟简单处理
		time.Sleep(1 * time.Millisecond)

		return nil
	}
}

// checkQuickTestOrder 检查快速测试消息顺序
func checkQuickTestOrder(envelope TestEnvelope, metrics *QuickTestMetrics) {
	aggregateID := envelope.AggregateID

	statsInterface, _ := metrics.AggregateStats.LoadOrStore(aggregateID, &QuickAggregateStats{
		LastSequence: -1,
	})

	stats := statsInterface.(*QuickAggregateStats)

	// 检查序列号顺序
	if envelope.Sequence != stats.LastSequence+1 {
		atomic.AddInt64(&stats.OrderViolations, 1)
		atomic.AddInt64(&metrics.OrderViolations, 1)
	}

	// 更新统计
	atomic.AddInt64(&stats.MessageCount, 1)
	stats.LastSequence = envelope.Sequence
}

// sendQuickTestMessages 发送快速测试消息
func sendQuickTestMessages(ctx context.Context, eventBus EventBus, topics []string, config QuickTestConfig, metrics *QuickTestMetrics) error {
	var wg sync.WaitGroup

	for topicIndex, topic := range topics {
		wg.Add(1)
		go func(topicName string, tIndex int) {
			defer wg.Done()

			for msgIndex := 0; msgIndex < config.MessagesPerTopic; msgIndex++ {
				// 选择聚合ID（确保分布均匀）
				aggregateID := fmt.Sprintf("quick-agg-%d", (tIndex*config.MessagesPerTopic+msgIndex)%config.AggregateCount)

				// 创建Envelope消息
				envelope := TestEnvelope{
					AggregateID: aggregateID,
					EventType:   fmt.Sprintf("QuickTestEvent-%d", msgIndex%3),
					Sequence:    int64(msgIndex),
					Timestamp:   time.Now(),
					Payload: map[string]interface{}{
						"topicIndex":   tIndex,
						"messageIndex": msgIndex,
						"testData":     fmt.Sprintf("quick-test-%d-%d", tIndex, msgIndex),
					},
					Metadata: map[string]string{
						"source":    "quick-test",
						"topic":     topicName,
						"aggregate": aggregateID,
					},
				}

				// 序列化并发送
				messageBytes, err := json.Marshal(envelope)
				if err != nil {
					continue
				}

				err = eventBus.Publish(ctx, topicName, messageBytes)
				if err == nil {
					atomic.AddInt64(&metrics.MessagesSent, 1)
				}

				// 小延迟避免过载
				if msgIndex%10 == 0 {
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(topic, topicIndex)
	}

	wg.Wait()
	return nil
}

// waitForQuickTestCompletion 等待快速测试完成
func waitForQuickTestCompletion(config QuickTestConfig, metrics *QuickTestMetrics, t *testing.T) {
	expectedMessages := int64(config.TopicCount * config.MessagesPerTopic)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️  等待超时，已处理 %d/%d 消息",
				atomic.LoadInt64(&metrics.MessagesReceived), expectedMessages)
			return
		case <-ticker.C:
			received := atomic.LoadInt64(&metrics.MessagesReceived)
			t.Logf("📊 处理进度: %d/%d (%.1f%%)",
				received, expectedMessages, float64(received)/float64(expectedMessages)*100)

			if received >= expectedMessages {
				t.Log("✅ 所有消息处理完成")
				return
			}
		}
	}
}

// validateQuickTestResults 验证快速测试结果
func validateQuickTestResults(config QuickTestConfig, metrics *QuickTestMetrics, t *testing.T) {
	sent := atomic.LoadInt64(&metrics.MessagesSent)
	received := atomic.LoadInt64(&metrics.MessagesReceived)
	orderViolations := atomic.LoadInt64(&metrics.OrderViolations)
	processingErrors := atomic.LoadInt64(&metrics.ProcessingErrors)

	t.Log("\n" + strings.Repeat("=", 60))
	t.Log("🚀 Kafka UnifiedConsumer 快速验证结果")
	t.Log(strings.Repeat("=", 60))

	t.Logf("📊 消息统计:")
	t.Logf("   发送消息: %d", sent)
	t.Logf("   接收消息: %d", received)
	t.Logf("   成功率: %.2f%%", float64(received)/float64(sent)*100)
	t.Logf("   处理错误: %d", processingErrors)

	t.Logf("\n🔍 顺序性验证:")
	t.Logf("   顺序违反: %d", orderViolations)

	// 聚合统计
	var aggregateCount int64
	metrics.AggregateStats.Range(func(key, value interface{}) bool {
		aggregateCount++
		return true
	})
	t.Logf("   聚合ID数量: %d", aggregateCount)

	// 断言验证
	assert.Equal(t, int64(config.TopicCount*config.MessagesPerTopic), sent, "发送消息数应该正确")
	assert.True(t, float64(received)/float64(sent) > 0.95, "消息成功率应该 > 95%")
	assert.Equal(t, int64(0), orderViolations, "不应该有顺序违反")
	assert.Equal(t, int64(0), processingErrors, "不应该有处理错误")

	t.Log(strings.Repeat("=", 60))
	t.Log("✅ 快速验证测试通过！")
}
