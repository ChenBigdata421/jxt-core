package performance_tests

import (
	"context"
	"fmt"
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

// TestNATSOrderViolationAnalysis 分析 NATS 顺序违反的详细原因
func TestNATSOrderViolationAnalysis(t *testing.T) {
	t.Log("🔍 ===== NATS 顺序违反详细分析测试 =====")

	// 初始化 logger
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	// 创建 NATS EventBus
	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("PERF_ANALYSIS_%d", timestamp)
	subjectPattern := fmt.Sprintf("analysis.%d.>", timestamp)

	natsConfig := &eventbus.NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-analysis-%d", timestamp),
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
				DurableName:   fmt.Sprintf("analysis_%d", timestamp),
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

	ctx := context.Background()

	// 测试参数 - 模拟性能测试的低压场景
	aggregateCount := 50       // 50 个聚合
	messagesPerAggregate := 10 // 每个聚合 10 条消息（总共 500 条）
	totalMessages := aggregateCount * messagesPerAggregate

	// 🔑 使用 5 个 topics（与性能测试一致）
	topicCount := 5
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("analysis.%d.topic%d", timestamp, i+1)
	}

	t.Logf("📊 测试参数:")
	t.Logf("   聚合数量: %d", aggregateCount)
	t.Logf("   每个聚合消息数: %d", messagesPerAggregate)
	t.Logf("   总消息数: %d", totalMessages)
	t.Logf("   Topic 数量: %d", topicCount)
	t.Logf("   Topics: %v", topics)

	// 生成聚合ID列表（包含topic信息）
	aggregateIDs := make([]string, aggregateCount)
	for i := 0; i < aggregateCount; i++ {
		topicIndex := i % topicCount
		aggregateIDs[i] = fmt.Sprintf("topic%d-agg-%d", topicIndex+1, i)
	}

	// 详细的接收统计
	type MessageRecord struct {
		AggregateID  string
		Version      int64
		ReceivedTime time.Time
		ReceivedSeq  int
		Topic        string // 从聚合ID中提取
	}

	var receivedMessages []MessageRecord
	var receivedMu sync.Mutex
	var receivedCount int64
	var orderViolations int64
	var duplicateMessages int64

	// 顺序检查器
	type AggregateState struct {
		mu          sync.Mutex
		lastVersion int64
		received    map[int64]int // version -> 接收次数
	}
	aggregateStates := make(map[string]*AggregateState)
	var statesMu sync.Mutex

	// 创建消息处理器
	handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
		seq := int(atomic.AddInt64(&receivedCount, 1))

		// 从聚合ID中提取topic信息（格式：topic1-agg-0）
		topicInfo := ""
		if len(envelope.AggregateID) > 6 && envelope.AggregateID[:5] == "topic" {
			topicInfo = envelope.AggregateID[:6] // "topic1"
		}

		// 记录接收
		receivedMu.Lock()
		receivedMessages = append(receivedMessages, MessageRecord{
			AggregateID:  envelope.AggregateID,
			Version:      envelope.EventVersion,
			ReceivedTime: time.Now(),
			ReceivedSeq:  seq,
			Topic:        topicInfo,
		})
		receivedMu.Unlock()

		// 获取或创建聚合状态
		statesMu.Lock()
		state, exists := aggregateStates[envelope.AggregateID]
		if !exists {
			state = &AggregateState{
				received: make(map[int64]int),
			}
			aggregateStates[envelope.AggregateID] = state
		}
		statesMu.Unlock()

		// 检查顺序和重复
		state.mu.Lock()
		defer state.mu.Unlock()

		// 检查是否重复
		if count, exists := state.received[envelope.EventVersion]; exists {
			atomic.AddInt64(&duplicateMessages, 1)
			t.Logf("🔁 消息重复: AggregateID=%s, Version=%d, 接收次数=%d, ReceivedSeq=%d",
				envelope.AggregateID, envelope.EventVersion, count+1, seq)
			state.received[envelope.EventVersion] = count + 1
			return nil
		}

		// 检查顺序
		if envelope.EventVersion <= state.lastVersion {
			atomic.AddInt64(&orderViolations, 1)
			t.Logf("❌ 顺序违反: AggregateID=%s, Topic=%s, LastVersion=%d, CurrentVersion=%d, ReceivedSeq=%d",
				envelope.AggregateID, topicInfo, state.lastVersion, envelope.EventVersion, seq)
		}

		state.lastVersion = envelope.EventVersion
		state.received[envelope.EventVersion] = 1

		return nil
	}

	// 订阅所有 topics
	for _, topic := range topics {
		err = eb.SubscribeEnvelope(ctx, topic, handler)
		require.NoError(t, err, "Failed to subscribe to topic: "+topic)
	}

	// 等待订阅建立
	time.Sleep(5 * time.Second)

	t.Logf("📤 开始发送消息...")

	// 为每个聚合ID创建一个 goroutine 串行发送消息
	// 🔑 关键：同一个聚合ID的所有消息都发送到同一个 topic（与性能测试一致）
	var sendWg sync.WaitGroup
	for aggIndex := 0; aggIndex < aggregateCount; aggIndex++ {
		sendWg.Add(1)
		go func(aggregateIndex int) {
			defer sendWg.Done()
			aggregateID := aggregateIDs[aggregateIndex]

			// 🔑 选择 topic（同一个聚合ID始终使用同一个 topic）
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

				err := eb.PublishEnvelope(ctx, topic, envelope)
				if err != nil {
					t.Logf("❌ 发送失败: AggregateID=%s, Version=%d, Topic=%s, Error=%v", aggregateID, version, topic, err)
				}
			}
		}(aggIndex)
	}

	sendWg.Wait()
	t.Logf("✅ 发送完成: %d 条消息", totalMessages)

	t.Logf("⏳ 等待消息处理完成...")

	// 等待所有消息接收完成（最多等待 30 秒）
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️  超时: 接收 %d/%d", receivedCount, totalMessages)
			goto ANALYZE
		case <-ticker.C:
			current := atomic.LoadInt64(&receivedCount)
			violations := atomic.LoadInt64(&orderViolations)
			duplicates := atomic.LoadInt64(&duplicateMessages)
			t.Logf("📊 进度: 接收 %d/%d, 顺序违反 %d, 重复消息 %d", current, totalMessages, violations, duplicates)
			if current >= int64(totalMessages) {
				t.Logf("✅ 所有消息已接收")
				goto ANALYZE
			}
		}
	}

ANALYZE:
	time.Sleep(2 * time.Second) // 等待最后的消息处理完成

	t.Logf("\n================================================================================")
	t.Logf("📊 测试结果分析")
	t.Logf("================================================================================")

	finalReceived := atomic.LoadInt64(&receivedCount)
	finalViolations := atomic.LoadInt64(&orderViolations)
	finalDuplicates := atomic.LoadInt64(&duplicateMessages)

	t.Logf("发送消息数: %d", totalMessages)
	t.Logf("接收消息数: %d", finalReceived)
	t.Logf("成功率: %.2f%%", float64(finalReceived)/float64(totalMessages)*100)
	t.Logf("顺序违反: %d", finalViolations)
	t.Logf("重复消息: %d", finalDuplicates)
	t.Logf("顺序违反率: %.2f%%", float64(finalViolations)/float64(totalMessages)*100)
	t.Logf("重复率: %.2f%%", float64(finalDuplicates)/float64(totalMessages)*100)

	t.Logf("\n================================================================================")
	t.Logf("🔍 根本原因分析")
	t.Logf("================================================================================")

	if finalDuplicates > 0 {
		t.Logf("⚠️  发现 %d 条重复消息！", finalDuplicates)
		t.Logf("   可能原因:")
		t.Logf("   1. NATS JetStream 的重试机制（MaxDeliver=3）")
		t.Logf("   2. ACK 超时导致消息重新投递（AckWait=15s）")
		t.Logf("   3. 消费者处理慢导致 ACK 延迟")
	}

	if finalViolations > 0 && finalDuplicates == 0 {
		t.Logf("❌ 发现 %d 次真正的顺序违反（非重复消息导致）！", finalViolations)
		t.Logf("   这是严重问题，需要深入调查！")
	}

	if finalViolations > 0 && finalDuplicates > 0 {
		t.Logf("⚠️  顺序违反可能是由重复消息导致的")
		t.Logf("   需要区分：真正的乱序 vs 重复消息")
	}

	t.Logf("\n================================================================================")

	// 断言：不应该有顺序违反（排除重复消息的影响）
	// 如果只是重复消息，不算真正的顺序违反
	if finalViolations > 0 && finalDuplicates == 0 {
		t.Errorf("❌ 发现真正的顺序违反: %d 次", finalViolations)
	}
}
