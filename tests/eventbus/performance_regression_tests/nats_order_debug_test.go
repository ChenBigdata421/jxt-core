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

// TestNATSOrderDebug 调试 NATS 顺序违反问题
func TestNATSOrderDebug(t *testing.T) {
	// 初始化 logger
	if logger.DefaultLogger == nil {
		zapLogger, err := zap.NewDevelopment()
		if err != nil {
			t.Fatalf("Failed to initialize logger: %v", err)
		}
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	t.Log("🔍 ===== NATS 顺序违反调试测试 =====")

	// 创建 NATS EventBus
	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("DEBUG_%d", timestamp)
	subjectPattern := fmt.Sprintf("debug.%d.>", timestamp)

	natsConfig := &eventbus.NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-debug-%d", timestamp),
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
				DurableName:   fmt.Sprintf("debug_%d", timestamp),
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

	// 测试参数 - 模拟性能测试的低压场景
	aggregateCount := 100     // 100 个聚合
	messagesPerAggregate := 5 // 每个聚合 5 条消息（总共 500 条）
	totalMessages := aggregateCount * messagesPerAggregate

	// 🔑 使用 5 个 topics（与性能测试一致）
	topicCount := 5
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("debug.%d.topic%d", timestamp, i+1)
	}

	t.Logf("📊 测试参数:")
	t.Logf("   聚合数量: %d", aggregateCount)
	t.Logf("   每个聚合消息数: %d", messagesPerAggregate)
	t.Logf("   总消息数: %d", totalMessages)
	t.Logf("   Topic 数量: %d", topicCount)
	t.Logf("   Topics: %v", topics)

	// 接收消息统计
	type ReceivedMessage struct {
		AggregateID  string
		EventVersion int64
		ReceivedAt   time.Time
		ReceivedSeq  int64
	}

	var receivedMessages []ReceivedMessage
	var receivedMu sync.Mutex
	var receivedCount int64
	var orderViolations int64

	// 每个聚合的最后接收版本
	lastVersions := make(map[string]int64)
	var lastVersionsMu sync.Mutex

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

		// 检查顺序
		lastVersionsMu.Lock()
		lastVersion, exists := lastVersions[env.AggregateID]
		if exists && env.EventVersion <= lastVersion {
			atomic.AddInt64(&orderViolations, 1)
			t.Logf("❌ 顺序违反: AggregateID=%s, LastVersion=%d, CurrentVersion=%d, ReceivedSeq=%d",
				env.AggregateID, lastVersion, env.EventVersion, seq)
		}
		lastVersions[env.AggregateID] = env.EventVersion
		lastVersionsMu.Unlock()

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
	// 🔑 关键：同一个聚合ID的所有消息都发送到同一个 topic
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
				// 使用 NewEnvelopeWithAutoID 自动生成 EventID
				// Payload 必须是有效的 JSON
				payload := fmt.Sprintf(`{"aggregate":"%s","message":%d}`, aggregateID, version)
				envelope := eventbus.NewEnvelopeWithAutoID(
					aggregateID,
					"TestEvent",
					version,
					[]byte(payload),
				)

				err := eb.PublishEnvelope(ctx, topic, envelope)
				if err != nil {
					t.Logf("❌ 发送失败: AggregateID=%s, Version=%d, Topic=%s, Error=%v", aggregateID, version, topic, err)
				}
			}

			t.Logf("✅ 聚合 %s 的所有消息已发送到 topic %s", aggregateID, topic)
		}(aggIndex)
	}

	sendWg.Wait()
	sendDuration := time.Since(sendStart)
	t.Logf("✅ 发送完成: %d 条消息, 耗时: %v", totalMessages, sendDuration)

	// 等待消息处理完成
	t.Log("⏳ 等待消息处理完成...")
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️  超时: 接收到 %d/%d 条消息", atomic.LoadInt64(&receivedCount), totalMessages)
			goto analysis
		case <-ticker.C:
			received := atomic.LoadInt64(&receivedCount)
			violations := atomic.LoadInt64(&orderViolations)
			t.Logf("📊 进度: 接收 %d/%d, 顺序违反 %d", received, totalMessages, violations)
			if received >= int64(totalMessages) {
				t.Log("✅ 所有消息已接收")
				goto analysis
			}
		}
	}

analysis:
	// 分析结果
	t.Log("\n" + strings.Repeat("=", 80))
	t.Log("📊 测试结果分析")
	t.Log(strings.Repeat("=", 80))

	finalReceived := atomic.LoadInt64(&receivedCount)
	finalViolations := atomic.LoadInt64(&orderViolations)

	t.Logf("发送消息数: %d", totalMessages)
	t.Logf("接收消息数: %d", finalReceived)
	t.Logf("成功率: %.2f%%", float64(finalReceived)/float64(totalMessages)*100)
	t.Logf("顺序违反: %d", finalViolations)
	t.Logf("顺序违反率: %.2f%%", float64(finalViolations)/float64(finalReceived)*100)

	// 分析每个聚合的接收情况
	t.Log("\n📊 每个聚合的接收情况:")
	aggregateStats := make(map[string][]int64)
	receivedMu.Lock()
	for _, msg := range receivedMessages {
		aggregateStats[msg.AggregateID] = append(aggregateStats[msg.AggregateID], msg.EventVersion)
	}
	receivedMu.Unlock()

	for aggID, versions := range aggregateStats {
		// 检查是否有顺序违反
		violations := 0
		for i := 1; i < len(versions); i++ {
			if versions[i] <= versions[i-1] {
				violations++
			}
		}

		// 检查是否有缺失
		missing := []int64{}
		versionSet := make(map[int64]bool)
		for _, v := range versions {
			versionSet[v] = true
		}
		for v := int64(1); v <= int64(messagesPerAggregate); v++ {
			if !versionSet[v] {
				missing = append(missing, v)
			}
		}

		// 检查是否有重复
		duplicates := []int64{}
		versionCount := make(map[int64]int)
		for _, v := range versions {
			versionCount[v]++
		}
		for v, count := range versionCount {
			if count > 1 {
				duplicates = append(duplicates, v)
			}
		}

		status := "✅"
		if violations > 0 || len(missing) > 0 || len(duplicates) > 0 {
			status = "❌"
		}

		t.Logf("%s %s: 接收=%d/%d, 顺序违反=%d, 缺失=%d, 重复=%d",
			status, aggID, len(versions), messagesPerAggregate, violations, len(missing), len(duplicates))

		if len(missing) > 0 && len(missing) <= 10 {
			t.Logf("   缺失版本: %v", missing)
		}
		if len(duplicates) > 0 && len(duplicates) <= 10 {
			t.Logf("   重复版本: %v", duplicates)
		}
	}

	// 分析接收时间分布
	t.Log("\n📊 接收时间分析:")
	if len(receivedMessages) > 0 {
		receivedMu.Lock()
		firstReceived := receivedMessages[0].ReceivedAt
		lastReceived := receivedMessages[len(receivedMessages)-1].ReceivedAt
		receivedMu.Unlock()

		receiveDuration := lastReceived.Sub(firstReceived)
		t.Logf("首条消息接收时间: %v", firstReceived.Format("15:04:05.000"))
		t.Logf("末条消息接收时间: %v", lastReceived.Format("15:04:05.000"))
		t.Logf("接收耗时: %v", receiveDuration)
		if receiveDuration.Seconds() > 0 {
			t.Logf("接收吞吐量: %.2f msg/s", float64(finalReceived)/receiveDuration.Seconds())
		}
	}

	t.Log(strings.Repeat("=", 80))

	// 断言
	require.Equal(t, int64(totalMessages), finalReceived, "应该接收到所有消息")
	require.Equal(t, int64(0), finalViolations, "不应该有顺序违反")
}
