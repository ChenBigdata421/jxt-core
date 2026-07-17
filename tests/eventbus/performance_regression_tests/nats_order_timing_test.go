package performance_tests

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNATSOrderTiming 测试 NATS 消息发送和接收的时序
func TestNATSOrderTiming(t *testing.T) {
	// 初始化 logger
	if logger.DefaultLogger == nil {
		zapLogger, err := zap.NewDevelopment()
		if err != nil {
			t.Fatalf("Failed to initialize logger: %v", err)
		}
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	t.Log("🔍 ===== NATS 消息时序调试测试 =====")

	// 创建 NATS EventBus
	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("TIMING_%d", timestamp)
	subjectPattern := fmt.Sprintf("timing.%d.>", timestamp)

	natsConfig := &eventbus.NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-timing-%d", timestamp),
		MaxReconnects:       10,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   30 * time.Second,
		HealthCheckInterval: 60 * time.Second,
		JetStream: eventbus.JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 10 * time.Second,
			AckWait:        15 * time.Second,
			MaxDeliver:     3,
			Stream: eventbus.StreamConfig{
				Name:      streamName,
				Subjects:  []string{subjectPattern},
				Storage:   "file",
				Retention: "limits",
				MaxAge:    1 * time.Hour,
				MaxBytes:  1024 * 1024 * 1024,
				MaxMsgs:   100000,
				Replicas:  1,
				Discard:   "old",
			},
			Consumer: eventbus.NATSConsumerConfig{
				DurableName:   fmt.Sprintf("timing_%d", timestamp),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 1000,
				MaxWaiting:    500,
				MaxDeliver:    3,
			},
		},
	}

	eb, err := eventbus.NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eb.Close()
	// Clean up the JetStream stream so it doesn't accumulate on the shared broker
	// (leftover streams cause "subjects overlap" failures on subsequent runs).
	defer func() {
		if nc, err := nats.Connect(natsConfig.URLs[0]); err == nil {
			defer nc.Close()
			if js, err := nc.JetStream(); err == nil {
				_ = js.DeleteStream(streamName)
			}
		}
	}()

	// 等待连接建立
	time.Sleep(2 * time.Second)

	// 测试参数 - 使用少量消息便于观察
	aggregateCount := 3        // 3 个聚合
	messagesPerAggregate := 10 // 每个聚合 10 条消息
	totalMessages := aggregateCount * messagesPerAggregate
	topicCount := 5
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("timing.%d.topic%d", timestamp, i+1)
	}

	t.Logf("📊 测试参数:")
	t.Logf("   聚合数量: %d", aggregateCount)
	t.Logf("   每个聚合消息数: %d", messagesPerAggregate)
	t.Logf("   总消息数: %d", totalMessages)
	t.Logf("   Topic 数量: %d", topicCount)

	// 记录发送时间
	type SentMessage struct {
		AggregateID  string
		EventVersion int64
		Topic        string
		SentAt       time.Time
	}
	var sentMessages []SentMessage
	var sentMu sync.Mutex

	// 记录接收时间
	type ReceivedMessage struct {
		AggregateID  string
		EventVersion int64
		ReceivedAt   time.Time
		ReceivedSeq  int64
	}
	var receivedMessages []ReceivedMessage
	var receivedMu sync.Mutex
	var receivedCount int64

	// 订阅消息
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	handler := func(ctx context.Context, env *eventbus.Envelope) error {
		seq := atomic.AddInt64(&receivedCount, 1)

		receivedMu.Lock()
		receivedMessages = append(receivedMessages, ReceivedMessage{
			AggregateID:  env.AggregateID,
			EventVersion: env.EventVersion,
			ReceivedAt:   time.Now(),
			ReceivedSeq:  seq,
		})
		receivedMu.Unlock()

		return nil
	}

	// 订阅所有 topics
	for _, topic := range topics {
		err = eb.SubscribeEnvelope(ctx, topic, handler)
		require.NoError(t, err, "Failed to subscribe to topic: "+topic)
	}

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发送消息
	t.Log("📤 开始发送消息...")
	sendStart := time.Now()

	aggregateIDs := make([]string, aggregateCount)
	for i := 0; i < aggregateCount; i++ {
		aggregateIDs[i] = fmt.Sprintf("agg-%d", i)
	}

	// 为每个聚合ID创建一个 goroutine 串行发送消息
	var sendWg sync.WaitGroup
	for aggIndex := 0; aggIndex < aggregateCount; aggIndex++ {
		sendWg.Add(1)
		go func(aggregateIndex int) {
			defer sendWg.Done()
			aggregateID := aggregateIDs[aggregateIndex]

			// 选择 topic（同一个聚合ID始终使用同一个 topic）
			topicIndex := aggregateIndex % topicCount
			topic := topics[topicIndex]

			// 串行发送该聚合ID的所有消息到同一个 topic
			for version := int64(1); version <= int64(messagesPerAggregate); version++ {
				envelope := eventbus.NewEnvelopeWithAutoID(
					aggregateID,
					"TestEvent",
					version,
					[]byte(fmt.Sprintf("aggregate %s message %d", aggregateID, version)),
				)

				sentAt := time.Now()
				err := eb.PublishEnvelope(ctx, topic, envelope)
				if err != nil {
					t.Logf("❌ 发送失败: AggregateID=%s, Version=%d, Topic=%s, Error=%v", aggregateID, version, topic, err)
				} else {
					sentMu.Lock()
					sentMessages = append(sentMessages, SentMessage{
						AggregateID:  aggregateID,
						EventVersion: version,
						Topic:        topic,
						SentAt:       sentAt,
					})
					sentMu.Unlock()
				}
			}
		}(aggIndex)
	}

	sendWg.Wait()
	sendDuration := time.Since(sendStart)
	t.Logf("✅ 发送完成: %d 条消息, 耗时: %v", totalMessages, sendDuration)

	// 等待消息处理完成
	t.Log("⏳ 等待消息处理完成...")
	time.Sleep(10 * time.Second)

	// 分析时序
	t.Log("\n" + strings.Repeat("=", 80))
	t.Log("📊 时序分析")
	t.Log(strings.Repeat("=", 80))

	finalReceived := atomic.LoadInt64(&receivedCount)
	t.Logf("发送消息数: %d", totalMessages)
	t.Logf("接收消息数: %d", finalReceived)

	// 为每个聚合ID分析时序
	for _, aggID := range aggregateIDs {
		t.Logf("\n📊 聚合 %s 的时序分析:", aggID)

		// 获取发送记录
		sentMu.Lock()
		var aggSent []SentMessage
		for _, msg := range sentMessages {
			if msg.AggregateID == aggID {
				aggSent = append(aggSent, msg)
			}
		}
		sentMu.Unlock()

		// 获取接收记录
		receivedMu.Lock()
		var aggReceived []ReceivedMessage
		for _, msg := range receivedMessages {
			if msg.AggregateID == aggID {
				aggReceived = append(aggReceived, msg)
			}
		}
		receivedMu.Unlock()

		if len(aggSent) == 0 || len(aggReceived) == 0 {
			t.Logf("   ⚠️  发送=%d, 接收=%d", len(aggSent), len(aggReceived))
			continue
		}

		// 检查顺序
		violations := 0
		for i := 1; i < len(aggReceived); i++ {
			if aggReceived[i].EventVersion <= aggReceived[i-1].EventVersion {
				violations++
				t.Logf("   ❌ 顺序违反: 接收顺序 %d -> %d (接收序号 %d -> %d)",
					aggReceived[i-1].EventVersion, aggReceived[i].EventVersion,
					aggReceived[i-1].ReceivedSeq, aggReceived[i].ReceivedSeq)
			}
		}

		if violations == 0 {
			t.Logf("   ✅ 无顺序违反")
		} else {
			t.Logf("   ❌ 顺序违反: %d 次", violations)
		}

		// 显示前5条消息的时序
		t.Logf("   前5条消息时序:")
		for i := 0; i < 5 && i < len(aggSent) && i < len(aggReceived); i++ {
			sent := aggSent[i]
			received := aggReceived[i]
			delay := received.ReceivedAt.Sub(sent.SentAt)
			t.Logf("      v%d: 发送=%s, 接收=%s, 延迟=%v, 接收序号=%d",
				sent.EventVersion,
				sent.SentAt.Format("15:04:05.000"),
				received.ReceivedAt.Format("15:04:05.000"),
				delay,
				received.ReceivedSeq)
		}
	}

	t.Log(strings.Repeat("=", 80))
}
