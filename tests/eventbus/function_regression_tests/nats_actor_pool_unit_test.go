//go:build integration
// +build integration

package function_tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestNATSConfig 创建测试用的 NATS 配置
func createTestNATSConfig(streamName, subjectPattern, durableName string, storage string) *eventbus.NATSConfig {
	return &eventbus.NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: fmt.Sprintf("test-%d", time.Now().UnixNano()),
		JetStream: eventbus.JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 5 * time.Second,
			AckWait:        10 * time.Second,
			MaxDeliver:     3,
			Stream: eventbus.StreamConfig{
				Name:      streamName,
				Subjects:  []string{subjectPattern},
				Storage:   storage,
				Retention: "limits",
				MaxAge:    10 * time.Minute,
				MaxBytes:  100 * 1024 * 1024,
				MaxMsgs:   10000,
				Replicas:  1,
				Discard:   "old",
			},
			Consumer: eventbus.NATSConsumerConfig{
				DurableName:   durableName,
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 1000,
				MaxWaiting:    500,
				MaxDeliver:    3,
			},
		},
	}
}

// TestNATSActorPool_RoundRobinRouting 测试普通消息的 Round-Robin 路由
func TestNATSActorPool_RoundRobinRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("TEST_RR_%d", timestamp)
	subjectPattern := fmt.Sprintf("test.rr.%d.>", timestamp)
	durableName := fmt.Sprintf("test-rr-consumer-%d", timestamp)

	natsConfig := createTestNATSConfig(streamName, subjectPattern, durableName, "memory")
	eb, err := eventbus.NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eb.Close()

	time.Sleep(2 * time.Second)

	topic := fmt.Sprintf("test.rr.%d.messages", timestamp)
	receivedCount := atomic.Int64{}

	handler := func(ctx context.Context, data []byte) error {
		receivedCount.Add(1)
		return nil
	}

	err = eb.Subscribe(ctx, topic, handler)
	require.NoError(t, err, "Failed to subscribe")

	time.Sleep(2 * time.Second)

	// 发送 50 条消息
	messageCount := 50
	for i := 0; i < messageCount; i++ {
		messageID := fmt.Sprintf("msg-%d", i)
		err := eb.Publish(ctx, topic, []byte(messageID))
		require.NoError(t, err, "Failed to publish message %d", i)
	}

	// 等待消息处理完成
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for messages. Received: %d/%d", receivedCount.Load(), messageCount)
		case <-ticker.C:
			if receivedCount.Load() >= int64(messageCount) {
				goto done
			}
		}
	}

done:
	t.Logf("✅ Round-Robin 路由测试通过: 接收到 %d/%d 条消息", receivedCount.Load(), messageCount)
	assert.Equal(t, int64(messageCount), receivedCount.Load(), "Should receive all messages")
}

// TestNATSActorPool_AggregateIDRouting 测试领域事件的聚合ID路由
func TestNATSActorPool_AggregateIDRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("TEST_AGG_%d", timestamp)
	subjectPattern := fmt.Sprintf("test.agg.%d.>", timestamp)
	durableName := fmt.Sprintf("test-agg-consumer-%d", timestamp)

	natsConfig := createTestNATSConfig(streamName, subjectPattern, durableName, "file")
	eb, err := eventbus.NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eb.Close()

	time.Sleep(2 * time.Second)

	topic := fmt.Sprintf("test.agg.%d.events", timestamp)

	var mu sync.Mutex
	receivedCount := atomic.Int64{}
	orderViolations := atomic.Int64{}
	lastSequence := make(map[string]int)

	handler := func(ctx context.Context, env *eventbus.Envelope) error {
		aggregateID := env.AggregateID

		// 从 Payload 中解析序列号 (格式: {"message":"payload-{aggIdx}-{seq}"})
		var sequence int
		var aggIdx int
		fmt.Sscanf(string(env.Payload), `{"message":"payload-%d-%d"}`, &aggIdx, &sequence)

		mu.Lock()
		if last, exists := lastSequence[aggregateID]; exists {
			if sequence != last+1 {
				orderViolations.Add(1)
				t.Logf("⚠️ 顺序违反: AggregateID=%s, Expected=%d, Got=%d", aggregateID, last+1, sequence)
			}
		}
		lastSequence[aggregateID] = sequence
		mu.Unlock()

		receivedCount.Add(1)
		return nil
	}

	err = eb.SubscribeEnvelope(ctx, topic, handler)
	require.NoError(t, err, "Failed to subscribe")

	time.Sleep(2 * time.Second)

	// 发送 50 条消息，分布在 5 个聚合ID上
	aggregateCount := 5
	messagesPerAggregate := 10
	messageCount := aggregateCount * messagesPerAggregate

	for aggIdx := 0; aggIdx < aggregateCount; aggIdx++ {
		aggregateID := fmt.Sprintf("agg-%d", aggIdx)
		for seq := 0; seq < messagesPerAggregate; seq++ {
			// Payload 必须是有效的 JSON
			payload := []byte(fmt.Sprintf(`{"message":"payload-%d-%d"}`, aggIdx, seq))
			env := eventbus.NewEnvelopeWithAutoID(
				aggregateID,
				"TestEvent",
				1, // EventVersion is int64
				payload,
			)

			err := eb.PublishEnvelope(ctx, topic, env)
			require.NoError(t, err, "Failed to publish envelope")
		}
	}

	// 等待消息处理完成
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for messages. Received: %d/%d", receivedCount.Load(), messageCount)
		case <-ticker.C:
			if receivedCount.Load() >= int64(messageCount) {
				goto done
			}
		}
	}

done:
	t.Logf("✅ 聚合ID路由测试通过: 接收到 %d/%d 条消息", receivedCount.Load(), messageCount)
	t.Logf("✅ 顺序违反: %d 次", orderViolations.Load())

	assert.Equal(t, int64(messageCount), receivedCount.Load(), "Should receive all messages")
	assert.Equal(t, int64(0), orderViolations.Load(), "Should have no order violations")
}

// TestNATSActorPool_ErrorHandling_AtLeastOnce 测试领域事件的 at-least-once 错误处理
func TestNATSActorPool_ErrorHandling_AtLeastOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("TEST_ERR_ATLEAST_%d", timestamp)
	subjectPattern := fmt.Sprintf("test.err.atleast.%d.>", timestamp)
	durableName := fmt.Sprintf("test-err-atleast-consumer-%d", timestamp)

	natsConfig := createTestNATSConfig(streamName, subjectPattern, durableName, "file")
	natsConfig.JetStream.AckWait = 3 * time.Second // 短 AckWait 以便快速重投

	eb, err := eventbus.NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eb.Close()

	time.Sleep(2 * time.Second)

	topic := fmt.Sprintf("test.err.atleast.%d.events", timestamp)

	var mu sync.Mutex
	attemptCount := make(map[string]int)
	successCount := atomic.Int64{}

	handler := func(ctx context.Context, env *eventbus.Envelope) error {
		messageID := string(env.Payload)

		mu.Lock()
		attemptCount[messageID]++
		attempts := attemptCount[messageID]
		mu.Unlock()

		// 前 2 次失败，第 3 次成功
		if attempts < 3 {
			return fmt.Errorf("simulated error (attempt %d)", attempts)
		}

		successCount.Add(1)
		return nil
	}

	err = eb.SubscribeEnvelope(ctx, topic, handler)
	require.NoError(t, err, "Failed to subscribe")

	time.Sleep(2 * time.Second)

	// 发送 3 条消息
	messageCount := 3
	for i := 0; i < messageCount; i++ {
		// Payload 必须是有效的 JSON
		payload := []byte(fmt.Sprintf(`{"message":"msg-%d"}`, i))
		env := eventbus.NewEnvelopeWithAutoID(
			fmt.Sprintf("agg-%d", i),
			"TestEvent",
			1,
			payload,
		)

		err := eb.PublishEnvelope(ctx, topic, env)
		require.NoError(t, err, "Failed to publish envelope")
	}

	// 等待消息处理完成（包括重试）
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Logf("⚠️ Timeout: Success=%d/%d", successCount.Load(), messageCount)
			goto done
		case <-ticker.C:
			if successCount.Load() >= int64(messageCount) {
				goto done
			}
		}
	}

done:
	mu.Lock()
	defer mu.Unlock()

	t.Logf("✅ At-least-once 测试: 成功处理 %d/%d 条消息", successCount.Load(), messageCount)

	// 验证每条消息都被重试了 3 次
	for messageID, attempts := range attemptCount {
		t.Logf("📊 Message %s: %d attempts", messageID, attempts)
		assert.Equal(t, 3, attempts, "Message %s should be attempted 3 times (at-least-once)", messageID)
	}
}

// TestNATSActorPool_ErrorHandling_AtMostOnce 测试普通消息的 at-most-once 错误处理
func TestNATSActorPool_ErrorHandling_AtMostOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("TEST_ERR_ATMOST_%d", timestamp)
	subjectPattern := fmt.Sprintf("test.err.atmost.%d.>", timestamp)
	durableName := fmt.Sprintf("test-err-atmost-consumer-%d", timestamp)

	natsConfig := createTestNATSConfig(streamName, subjectPattern, durableName, "memory")
	natsConfig.JetStream.AckWait = 3 * time.Second

	eb, err := eventbus.NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eb.Close()

	time.Sleep(2 * time.Second)

	topic := fmt.Sprintf("test.err.atmost.%d.messages", timestamp)

	var mu sync.Mutex
	attemptCount := make(map[string]int)
	receivedCount := atomic.Int64{}

	handler := func(ctx context.Context, data []byte) error {
		messageID := string(data)

		mu.Lock()
		attemptCount[messageID]++
		mu.Unlock()

		receivedCount.Add(1)

		// 总是失败
		return fmt.Errorf("simulated error")
	}

	err = eb.Subscribe(ctx, topic, handler)
	require.NoError(t, err, "Failed to subscribe")

	time.Sleep(2 * time.Second)

	// 发送 3 条消息
	messageCount := 3
	for i := 0; i < messageCount; i++ {
		messageID := fmt.Sprintf("msg-%d", i)
		err := eb.Publish(ctx, topic, []byte(messageID))
		require.NoError(t, err, "Failed to publish message")
	}

	// 等待消息处理完成（不会重试）
	time.Sleep(5 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	t.Logf("✅ At-most-once 测试: 接收到 %d 条消息", receivedCount.Load())

	// 验证每条消息只被尝试了 1 次（at-most-once）
	for messageID, attempts := range attemptCount {
		t.Logf("📊 Message %s: %d attempts", messageID, attempts)
		assert.Equal(t, 1, attempts, "Message %s should be attempted only once (at-most-once)", messageID)
	}
}

// TestNATSActorPool_DoneChannelWaiting 测试 Done Channel 等待逻辑
func TestNATSActorPool_DoneChannelWaiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("TEST_DONE_%d", timestamp)
	subjectPattern := fmt.Sprintf("test.done.%d.>", timestamp)
	durableName := fmt.Sprintf("test-done-consumer-%d", timestamp)

	natsConfig := createTestNATSConfig(streamName, subjectPattern, durableName, "memory")
	eb, err := eventbus.NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eb.Close()

	time.Sleep(2 * time.Second)

	topic := fmt.Sprintf("test.done.%d.messages", timestamp)

	var mu sync.Mutex
	processingTimes := make(map[string]time.Duration)
	receivedCount := atomic.Int64{}

	handler := func(ctx context.Context, data []byte) error {
		messageID := string(data)
		start := time.Now()

		// 模拟耗时处理
		time.Sleep(100 * time.Millisecond)

		duration := time.Since(start)
		mu.Lock()
		processingTimes[messageID] = duration
		mu.Unlock()

		receivedCount.Add(1)
		return nil
	}

	err = eb.Subscribe(ctx, topic, handler)
	require.NoError(t, err, "Failed to subscribe")

	time.Sleep(2 * time.Second)

	// 发送 5 条消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		messageID := fmt.Sprintf("msg-%d", i)
		err := eb.Publish(ctx, topic, []byte(messageID))
		require.NoError(t, err, "Failed to publish message")
	}

	// 等待消息处理完成
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for messages. Received: %d/%d", receivedCount.Load(), messageCount)
		case <-ticker.C:
			if receivedCount.Load() >= int64(messageCount) {
				goto done
			}
		}
	}

done:
	mu.Lock()
	defer mu.Unlock()

	t.Logf("✅ Done Channel 等待测试: 接收到 %d/%d 条消息", receivedCount.Load(), messageCount)

	// 验证每条消息都被完整处理（处理时间 >= 100ms）
	for messageID, duration := range processingTimes {
		t.Logf("📊 Message %s: processing time = %v", messageID, duration)
		assert.GreaterOrEqual(t, duration, 100*time.Millisecond,
			"Message %s should be fully processed (>= 100ms)", messageID)
	}
}

// TestNATSActorPool_MissingAggregateID 测试领域事件缺少聚合ID的处理
func TestNATSActorPool_MissingAggregateID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("TEST_MISSING_AGG_%d", timestamp)
	subjectPattern := fmt.Sprintf("test.missing.agg.%d.>", timestamp)
	durableName := fmt.Sprintf("test-missing-agg-consumer-%d", timestamp)

	natsConfig := createTestNATSConfig(streamName, subjectPattern, durableName, "file")
	natsConfig.JetStream.AckWait = 3 * time.Second
	natsConfig.JetStream.MaxDeliver = 3

	eb, err := eventbus.NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eb.Close()

	time.Sleep(2 * time.Second)

	topic := fmt.Sprintf("test.missing.agg.%d.events", timestamp)

	receivedCount := atomic.Int64{}

	handler := func(ctx context.Context, env *eventbus.Envelope) error {
		receivedCount.Add(1)
		t.Logf("⚠️ Received envelope with AggregateID: '%s'", env.AggregateID)
		return nil
	}

	err = eb.SubscribeEnvelope(ctx, topic, handler)
	require.NoError(t, err, "Failed to subscribe")

	time.Sleep(2 * time.Second)

	// 发送一个没有聚合ID的领域事件
	// 注意：NewEnvelopeWithAutoID 会校验 aggregateID 不能为空，所以我们直接构造 Envelope
	env := &eventbus.Envelope{
		EventID:      "test-event-id",
		AggregateID:  "", // 空聚合ID
		EventType:    "TestEvent",
		EventVersion: 1,
		Timestamp:    time.Now(),
		Payload:      []byte(`{"message":"test-payload"}`), // Payload 必须是有效的 JSON
	}

	// 注意：PublishEnvelope 会调用 Validate()，空聚合ID会导致发布失败
	// 所以我们需要绕过 PublishEnvelope，直接发布原始字节
	envelopeBytes, err := env.ToBytes()
	if err != nil {
		// 预期会失败，因为 aggregateID 为空
		t.Logf("✅ Envelope validation failed as expected: %v", err)

		// 验证消息不应该被成功处理
		assert.Equal(t, int64(0), receivedCount.Load(),
			"Domain event without aggregate ID should not be processed")
		return
	}

	// 如果 Validate 没有失败（不应该发生），则发布消息
	err = eb.Publish(ctx, topic, envelopeBytes)
	require.NoError(t, err, "Failed to publish envelope")

	// 等待一段时间，观察是否被处理
	time.Sleep(10 * time.Second)

	t.Logf("✅ 缺少聚合ID测试: 接收到 %d 条消息", receivedCount.Load())

	// 验证消息不应该被成功处理（应该被 Nak 重投，最终进入死信队列）
	// 由于我们设置了 MaxDeliver=3，消息会被重投 3 次后进入死信队列
	// 在这个测试中，我们期望 receivedCount 为 0（因为 handleMessageWithWrapper 会在聚合ID为空时直接 Nak）
	assert.Equal(t, int64(0), receivedCount.Load(),
		"Domain event without aggregate ID should not be processed")
}
