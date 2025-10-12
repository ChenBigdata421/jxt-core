package eventbus

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestKafkaSimpleDebug 最简单的调试测试
func TestKafkaSimpleDebug(t *testing.T) {
	t.Log("🔍 Kafka简单调试测试")

	// 创建配置
	config := &KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: "simple-debug-test",
		Producer: ProducerConfig{
			RequiredAcks:    1,
			Compression:     "lz4",
			FlushFrequency:  10 * time.Millisecond,
			FlushMessages:   100,
			FlushBytes:      100000,
			RetryMax:        3,
			Timeout:         10 * time.Second,
			MaxMessageBytes: 1024 * 1024,
			MaxInFlight:     100,
		},
		Consumer: ConsumerConfig{
			GroupID:           fmt.Sprintf("simple-debug-group-%d", time.Now().UnixNano()),
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 1 * time.Second,
			FetchMinBytes:     1,                // 最小1字节，确保能读取
			FetchMaxBytes:     10 * 1024 * 1024, // 10MB
			FetchMaxWait:      500 * time.Millisecond,
			AutoOffsetReset:   "earliest", // 使用earliest
			RebalanceStrategy: "range",
			IsolationLevel:    "read_uncommitted",
		},
		Net: NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			KeepAlive:    30 * time.Second,
		},
		MetadataRefreshFreq:  10 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 250 * time.Millisecond,
		HealthCheckInterval:  30 * time.Second,
	}

	// 创建EventBus
	bus, err := NewKafkaEventBus(config)
	if err != nil {
		t.Fatalf("❌ Failed to create Kafka EventBus: %v", err)
	}
	defer bus.Close()

	// 创建topic
	topic := fmt.Sprintf("simple-debug-%d", time.Now().UnixNano())
	ctx := context.Background()

	t.Logf("📝 使用topic: %s", topic)
	t.Logf("📝 Consumer Group: %s", config.Consumer.GroupID)

	// 配置预订阅
	if kafkaBus, ok := bus.(*kafkaEventBus); ok {
		kafkaBus.allPossibleTopics = []string{topic}
		t.Logf("✅ 配置预订阅: %s", topic)
	}

	// 接收计数器
	var receivedCount atomic.Int64
	receivedChan := make(chan struct{}, 10)

	// 订阅
	handler := func(ctx context.Context, message []byte) error {
		count := receivedCount.Add(1)
		t.Logf("📥 收到消息 #%d: %s", count, string(message))
		receivedChan <- struct{}{}
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	if err != nil {
		t.Fatalf("❌ Failed to subscribe: %v", err)
	}
	t.Logf("✅ 订阅成功: %s", topic)

	// 等待订阅就绪
	t.Logf("⏳ 等待订阅就绪...")
	time.Sleep(5 * time.Second)

	// 发送10条简单消息
	messageCount := 10
	t.Logf("📤 开始发送 %d 条消息...", messageCount)

	for i := 0; i < messageCount; i++ {
		message := []byte(fmt.Sprintf("Message #%d", i))
		err := bus.Publish(ctx, topic, message)
		if err != nil {
			t.Errorf("❌ Failed to publish message #%d: %v", i, err)
		} else {
			t.Logf("✅ 发送消息 #%d", i)
		}
	}

	t.Logf("✅ 发送完成")

	// 等待AsyncProducer批量发送完成
	t.Logf("⏳ 等待AsyncProducer批量发送完成...")
	time.Sleep(5 * time.Second)

	// 等待接收
	t.Logf("📥 等待接收消息...")
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	receivedInTime := 0
receiveLoop:
	for receivedInTime < messageCount {
		select {
		case <-receivedChan:
			receivedInTime++
			t.Logf("   进度: %d/%d", receivedInTime, messageCount)
		case <-timeout.C:
			t.Logf("⏱️  接收超时")
			break receiveLoop
		}
	}

	// 输出结果
	t.Logf("\n📊 测试结果:")
	t.Logf("   发送: %d", messageCount)
	t.Logf("   接收: %d", receivedInTime)
	t.Logf("   成功率: %.2f%%", float64(receivedInTime)/float64(messageCount)*100)

	if receivedInTime < messageCount {
		t.Errorf("❌ 未收到所有消息: %d/%d", receivedInTime, messageCount)
	} else {
		t.Logf("✅ 所有消息都已接收！")
	}
}

