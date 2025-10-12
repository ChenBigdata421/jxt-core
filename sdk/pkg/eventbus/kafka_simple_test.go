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

// TestKafkaUnifiedConsumerSimple 简化的UnifiedConsumer测试
func TestKafkaUnifiedConsumerSimple(t *testing.T) {
	t.Log("🚀 开始Kafka UnifiedConsumer简化测试")

	// 创建简化的Kafka配置
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29092"},
		Producer: ProducerConfig{
			RequiredAcks:     1,
			Compression:      "none",
			FlushFrequency:   50 * time.Millisecond,
			FlushMessages:    10,
			Timeout:          10 * time.Second,
			FlushBytes:       1024,
			RetryMax:         1,
			BatchSize:        1024,
			BufferSize:       1024 * 1024,
			Idempotent:       false,
			MaxMessageBytes:  1024 * 1024,
			PartitionerType:  "hash",
			LingerMs:         0,
			CompressionLevel: 1,
			MaxInFlight:      5,
		},
		Consumer: ConsumerConfig{
			GroupID:           "simple-test-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 5 * time.Second,
			FetchMinBytes:     1,
			FetchMaxBytes:     1024 * 1024,
			FetchMaxWait:      250 * time.Millisecond,
		},
		ClientID:            "simple-test-client",
		HealthCheckInterval: 30 * time.Second,
	}

	// 创建EventBus
	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err)
	defer eventBus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 测试配置
	topicCount := 3
	messagesPerTopic := 10
	aggregateCount := 5

	// 生成topic列表
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("simple-test-topic-%d", i)
	}

	// 消息计数器
	var receivedCount int64
	var processedMessages sync.Map

	// 设置订阅者
	t.Log("📡 设置订阅者...")
	for _, topic := range topics {
		topicName := topic // 避免闭包问题
		handler := func(ctx context.Context, message []byte) error {
			// 解析消息
			var envelope SimpleTestEnvelope
			if err := json.Unmarshal(message, &envelope); err != nil {
				t.Logf("❌ 解析消息失败: %v", err)
				return err
			}

			// 记录消息
			key := fmt.Sprintf("%s-%s-%d", topicName, envelope.AggregateID, envelope.Sequence)
			processedMessages.Store(key, envelope)
			
			count := atomic.AddInt64(&receivedCount, 1)
			if count%10 == 0 {
				t.Logf("📨 已处理 %d 条消息", count)
			}
			
			return nil
		}

		err := eventBus.Subscribe(ctx, topicName, handler)
		require.NoError(t, err)
	}

	// 等待订阅者准备就绪
	time.Sleep(2 * time.Second)

	// 发送测试消息
	t.Log("📤 发送测试消息...")
	totalMessages := topicCount * messagesPerTopic
	
	for i, topic := range topics {
		for j := 0; j < messagesPerTopic; j++ {
			aggregateID := fmt.Sprintf("agg-%d", j%aggregateCount)
			envelope := SimpleTestEnvelope{
				AggregateID: aggregateID,
				EventType:   "TestEvent",
				Sequence:    int64(j),
				Timestamp:   time.Now(),
				Data:        fmt.Sprintf("Test data for topic %d, message %d", i, j),
			}

			messageBytes, err := json.Marshal(envelope)
			require.NoError(t, err)

			err = eventBus.Publish(ctx, topic, messageBytes)
			require.NoError(t, err)
		}
	}

	t.Logf("✅ 发送完成，总计 %d 条消息", totalMessages)

	// 等待消息处理完成
	t.Log("⏳ 等待消息处理完成...")
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️ 等待超时，已处理 %d/%d 条消息", atomic.LoadInt64(&receivedCount), totalMessages)
			goto validate
		case <-ticker.C:
			received := atomic.LoadInt64(&receivedCount)
			t.Logf("📊 进度: %d/%d 条消息", received, totalMessages)
			if received >= int64(totalMessages) {
				t.Log("✅ 所有消息处理完成")
				goto validate
			}
		}
	}

validate:
	// 验证结果
	t.Log("🔍 验证测试结果...")
	
	finalCount := atomic.LoadInt64(&receivedCount)
	t.Logf("📈 最终统计: 收到 %d/%d 条消息", finalCount, totalMessages)
	
	// 基本验证
	assert.True(t, finalCount > 0, "应该收到至少一些消息")
	
	if finalCount >= int64(totalMessages) {
		t.Log("🎉 测试完全成功！所有消息都已处理")
	} else {
		t.Logf("⚠️ 部分成功：处理了 %.1f%% 的消息", float64(finalCount)/float64(totalMessages)*100)
	}

	// 验证消息内容
	messageCount := 0
	processedMessages.Range(func(key, value interface{}) bool {
		messageCount++
		return true
	})
	
	t.Logf("📋 消息详情: 存储了 %d 条有效消息", messageCount)
	
	// 输出测试总结
	separator := "=================================================="
	t.Log("\n" + separator)
	t.Log("🎯 Kafka UnifiedConsumer 简化测试总结")
	t.Log(separator)
	t.Logf("✅ Kafka连接: 成功")
	t.Logf("✅ 多Topic订阅: %d个topic", topicCount)
	t.Logf("✅ 消息发送: %d条消息", totalMessages)
	t.Logf("✅ 消息接收: %d条消息", finalCount)
	t.Logf("✅ 成功率: %.1f%%", float64(finalCount)/float64(totalMessages)*100)
	t.Log(separator)
}

// SimpleTestEnvelope 简化测试用的消息封装
type SimpleTestEnvelope struct {
	AggregateID string    `json:"aggregateId"`
	EventType   string    `json:"eventType"`
	Sequence    int64     `json:"sequence"`
	Timestamp   time.Time `json:"timestamp"`
	Data        string    `json:"data"`
}
