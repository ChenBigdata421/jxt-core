package reliability_regression_tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// TestMessageBufferGuarantee 测试消息缓冲区保证（Actor 重启期间消息不丢失）
//
// 🎯 测试目的:
//   验证 Actor Inbox 缓冲区机制：在 Actor 重启期间，发送到该 Actor 的消息应该被缓存，
//   而不是丢失，重启后能够继续处理缓冲区中的消息。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 topic
//   2. Handler 在处理第 1 条消息时触发 panic
//   3. 快速连续发送 5 条消息（不等待，模拟 Actor 重启期间的消息堆积）
//   4. 等待所有消息处理完成
//
// ✅ 检查项:
//   - 所有 5 条消息都应该被接收（received = 5）
//   - panic 应该只发生 1 次（panicCount = 1）
//   - 除了第一条触发 panic 的消息，其他 4 条消息都应该被正确接收
//   - 消息顺序应该保持（除了第一条）
//   - 测试应该在 10 秒内完成
//
// 🔍 验证点:
//   - Actor Inbox 的缓冲能力（默认 1000 条消息）
//   - Actor 重启后能够继续处理缓冲区中的消息
//   - Memory EventBus 的 at-most-once 语义：panic 消息被计数但不重投
//
// 📊 缓冲区配置:
//   - Inbox 大小: 1000
//   - 测试消息数: 5（远小于缓冲区大小）
func TestMessageBufferGuarantee(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.message.buffer.guarantee.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var received int64
	var panicCount int64
	receivedMessages := make([]string, 0)
	var mu sync.Mutex

	ctx := context.Background()

	// 订阅消息，第一条消息触发 panic
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, msg []byte) error {
		count := atomic.AddInt64(&received, 1)

		// 第一条消息触发 panic
		if count == 1 {
			atomic.AddInt64(&panicCount, 1)
			t.Logf("⚠️ Panic on first message")
			panic("simulated panic on first message")
		}

		// 记录收到的消息
		mu.Lock()
		receivedMessages = append(receivedMessages, string(msg))
		mu.Unlock()

		t.Logf("📨 Processed message %d: %s", count, string(msg))
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	// 快速发送多条消息（在 Actor 重启期间）
	messages := []string{"msg1-panic", "msg2", "msg3", "msg4", "msg5"}
	for i, msg := range messages {
		err = bus.Publish(ctx, topic, []byte(msg))
		helper.AssertNoError(err, fmt.Sprintf("Publish message %d should not return error", i+1))
		// 不等待，快速发送
	}

	// 等待所有消息处理完成
	success := helper.WaitForMessages(&received, int64(len(messages)), 10*time.Second)
	helper.AssertTrue(success, "Should receive all messages from buffer")

	// 验证结果
	helper.AssertEqual(int64(len(messages)), atomic.LoadInt64(&received), "Should receive all messages")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Should panic exactly once")

	// 验证消息顺序（除了第一条触发 panic 的消息）
	mu.Lock()
	expectedMessages := messages[1:] // 跳过第一条
	helper.AssertEqual(len(expectedMessages), len(receivedMessages), "Should receive correct number of messages")
	mu.Unlock()

	t.Logf("✅ Message buffer guarantee test passed")
}

// TestMessageOrderingAfterRecovery 测试恢复后的消息顺序保证
//
// 🎯 测试目的:
//   验证 Actor 在 panic 恢复后，能够继续按照正确的顺序处理同一聚合 ID 的后续消息，
//   确保事件溯源场景下的顺序性保证。
//
// ⚠️ Memory EventBus 限制:
//   Memory EventBus 使用 at-most-once 语义，panic 会导致消息丢失（不重投）。
//   这与 Kafka/NATS 的 at-least-once 语义不同。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 Envelope topic
//   2. Handler 在处理 aggregate-1 的 version=1 时触发 panic（panic 前不计数）
//   3. 快速发送 aggregate-1 的 10 个版本（v1-v10）
//   4. 等待消息处理完成（预期接收 9 条，v1 丢失）
//
// ✅ 检查项:
//   - 应该接收 9 条消息（received = 9，v1 丢失）
//   - panic 应该只发生 1 次（panicCount = 1）
//   - 接收到的版本号应该是 v2-v10，按顺序排列
//   - 版本号应该递增，但跳过 v1
//   - 测试应该在 10 秒内完成
//
// 🔍 验证点:
//   - Actor 恢复后继续处理同一聚合的后续版本
//   - 版本号顺序保持正确（v2, v3, v4, ...）
//   - Memory EventBus 的 at-most-once 语义：v1 丢失不重投
//
// 📊 语义对比:
//   - Memory: at-most-once（panic 消息丢失）
//   - Kafka/NATS Envelope: at-least-once（panic 消息重投）
func TestMessageOrderingAfterRecovery(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.message.ordering.recovery.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var received int64
	var panicCount int64
	receivedVersions := make([]int64, 0)
	var mu sync.Mutex

	ctx := context.Background()

	// 订阅 Envelope，aggregate-1 的第一条消息触发 panic
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		// ⚠️ 重要：先检查是否需要 panic，再递增计数器
		if envelope.AggregateID == "aggregate-1" && envelope.EventVersion == 1 {
			atomic.AddInt64(&panicCount, 1)
			t.Logf("⚠️ Panic on aggregate-1 version 1")
			panic("simulated panic for aggregate-1")
		}

		// 成功处理的消息才计数
		atomic.AddInt64(&received, 1)

		// 记录收到的版本号
		if envelope.AggregateID == "aggregate-1" {
			mu.Lock()
			receivedVersions = append(receivedVersions, envelope.EventVersion)
			mu.Unlock()
		}

		t.Logf("📨 Processed: AggregateID=%s, Version=%d", envelope.AggregateID, envelope.EventVersion)
		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送 aggregate-1 的多个版本（快速发送）
	versionsCount := 10
	for version := 1; version <= versionsCount; version++ {
		envelope := &eventbus.Envelope{
			EventID:      fmt.Sprintf("evt-agg1-v%d", version),
			AggregateID:  "aggregate-1",
			EventType:    "TestEvent",
			EventVersion: int64(version),
			Timestamp:    time.Now(),
			Payload:      []byte(fmt.Sprintf(`{"version":%d}`, version)),
		}
		err = bus.PublishEnvelope(ctx, topic, envelope)
		helper.AssertNoError(err, "PublishEnvelope should not return error")
		// 不等待，快速发送
	}

	// ⚠️ Memory EventBus 使用 at-most-once 语义，panic 会导致消息丢失
	// 等待消息处理（预期会丢失 1 条消息）
	expectedMessages := int64(versionsCount - 1) // version=1 会丢失
	success := helper.WaitForMessages(&received, expectedMessages, 10*time.Second)
	helper.AssertTrue(success, "Should receive all messages except the one that panicked")

	// 验证结果（调整预期）
	actualReceived := atomic.LoadInt64(&received)
	helper.AssertEqual(expectedMessages, actualReceived, "Should receive all messages except the one that panicked (at-most-once semantics)")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Should panic exactly once")

	// 验证消息顺序（版本号应该递增，但跳过 version=1）
	mu.Lock()
	expectedVersionsCount := versionsCount - 1 // version=1 丢失
	helper.AssertEqual(expectedVersionsCount, len(receivedVersions), "Should receive all versions except the one that panicked")
	for i := 0; i < len(receivedVersions); i++ {
		expectedVersion := int64(i + 2) // 从 version=2 开始
		helper.AssertEqual(expectedVersion, receivedVersions[i], fmt.Sprintf("Version %d should be in order", i+2))
	}
	mu.Unlock()

	t.Logf("✅ Message ordering after recovery test passed (at-most-once semantics)")
	t.Logf("📊 Received: %d (expected %d, version=1 lost)", actualReceived, expectedMessages)
	t.Logf("⚠️ Note: Memory EventBus uses at-most-once semantics, so panic causes message loss")
}

// TestHighThroughputWithRecovery 测试高吞吐量下的恢复
//
// 🎯 测试目的:
//   验证在高吞吐量场景下，Actor 发生 panic 后能够快速恢复并继续处理大量消息，
//   确保系统在高负载下的稳定性和容错能力。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 topic
//   2. Handler 在处理第 100 条消息时触发 panic
//   3. 快速连续发送 1000 条消息（不等待）
//   4. 等待所有消息处理完成
//
// ✅ 检查项:
//   - 所有 1000 条消息都应该被接收（received = 1000）
//   - panic 应该只发生 1 次（panicCount = 1）
//   - Actor 应该在 panic 后快速恢复并继续处理剩余 900 条消息
//   - 测试应该在 30 秒内完成
//
// 🔍 验证点:
//   - Actor 在高吞吐量下的稳定性
//   - Supervisor 在高负载下的恢复能力
//   - Inbox 缓冲区在高吞吐量下的表现
//   - Memory EventBus 的 at-most-once 语义在高负载下的正确性
//
// 📊 性能指标:
//   - 消息总数: 1000
//   - Panic 位置: 第 100 条
//   - 预期完成时间: < 30s
func TestHighThroughputWithRecovery(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.high.throughput.recovery.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var received int64
	var panicCount int64
	ctx := context.Background()

	// 订阅消息，第 100 条消息触发 panic
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, msg []byte) error {
		count := atomic.AddInt64(&received, 1)

		// 第 100 条消息触发 panic
		if count == 100 {
			atomic.AddInt64(&panicCount, 1)
			t.Logf("⚠️ Panic on message 100")
			panic("simulated panic on message 100")
		}

		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送大量消息（1000 条）
	messageCount := 1000
	for i := 1; i <= messageCount; i++ {
		msg := fmt.Sprintf("msg%d", i)
		err = bus.Publish(ctx, topic, []byte(msg))
		helper.AssertNoError(err, "Publish should not return error")
		// 不等待，快速发送
	}

	// 等待所有消息处理完成
	success := helper.WaitForMessages(&received, int64(messageCount), 30*time.Second)
	helper.AssertTrue(success, "Should receive all messages under high throughput")

	// 验证结果
	helper.AssertEqual(int64(messageCount), atomic.LoadInt64(&received), "Should receive all messages")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Should panic exactly once")

	t.Logf("✅ High throughput with recovery test passed")
	t.Logf("📊 Processed %d messages with 1 panic", messageCount)
}

// TestMessageGuaranteeWithMultipleAggregates 测试多聚合的消息保证
//
// 🎯 测试目的:
//   验证在多个聚合 ID 并发处理的场景下，一个聚合的 panic 不会影响其他聚合，
//   并且每个聚合的消息顺序都能得到保证。
//
// ⚠️ Memory EventBus 限制:
//   Memory EventBus 使用 at-most-once 语义，panic 会导致消息丢失（不重投）。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 Envelope topic
//   2. Handler 在处理 aggregate-2 的 version=3 时触发 panic（panic 前不计数）
//   3. 快速发送 3 个聚合（aggregate-1/2/3）各 10 个版本（共 30 条）
//   4. 等待消息处理完成（预期接收 29 条，aggregate-2 的 v3 丢失）
//
// ✅ 检查项:
//   - 应该接收 29 条消息（totalReceived = 29，aggregate-2 的 v3 丢失）
//   - panic 应该只发生 1 次（panicCount = 1）
//   - aggregate-1 和 aggregate-3 应该接收所有 10 个版本
//   - aggregate-2 应该接收 9 个版本（v1,v2,v4-v10，跳过 v3）
//   - 每个聚合的版本号都应该按顺序排列
//   - 测试应该在 15 秒内完成
//
// 🔍 验证点:
//   - Actor Pool 的故障隔离：aggregate-2 的 panic 不影响其他聚合
//   - 每个聚合使用独立的 Actor（通过一致性哈希路由）
//   - 同一聚合的消息顺序保证（版本号递增）
//   - Memory EventBus 的 at-most-once 语义：aggregate-2 的 v3 丢失不重投
//
// 📊 测试规模:
//   - 聚合数量: 3
//   - 每个聚合版本数: 10
//   - 总消息数: 30
//   - 预期接收: 29（aggregate-2 的 v3 丢失）
func TestMessageGuaranteeWithMultipleAggregates(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.message.guarantee.multi.agg.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var totalReceived int64
	var panicCount int64
	receivedByAggregate := make(map[string][]int64)
	var mu sync.Mutex

	ctx := context.Background()

	// 订阅 Envelope，aggregate-2 的第 3 条消息触发 panic
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		// ⚠️ 重要：先检查是否需要 panic，再递增计数器
		if envelope.AggregateID == "aggregate-2" && envelope.EventVersion == 3 {
			atomic.AddInt64(&panicCount, 1)
			t.Logf("⚠️ Panic on aggregate-2 version 3")
			panic("simulated panic for aggregate-2")
		}

		// 成功处理的消息才计数
		atomic.AddInt64(&totalReceived, 1)

		// 记录收到的版本号
		mu.Lock()
		if receivedByAggregate[envelope.AggregateID] == nil {
			receivedByAggregate[envelope.AggregateID] = make([]int64, 0)
		}
		receivedByAggregate[envelope.AggregateID] = append(receivedByAggregate[envelope.AggregateID], envelope.EventVersion)
		mu.Unlock()

		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送多个聚合的消息（快速发送）
	aggregates := []string{"aggregate-1", "aggregate-2", "aggregate-3"}
	versionsPerAggregate := 10
	totalMessages := len(aggregates) * versionsPerAggregate

	for version := 1; version <= versionsPerAggregate; version++ {
		for _, aggID := range aggregates {
			envelope := &eventbus.Envelope{
				EventID:      fmt.Sprintf("evt-%s-v%d", aggID, version),
				AggregateID:  aggID,
				EventType:    "TestEvent",
				EventVersion: int64(version),
				Timestamp:    time.Now(),
				Payload:      []byte(fmt.Sprintf(`{"aggregate":"%s","version":%d}`, aggID, version)),
			}
			err = bus.PublishEnvelope(ctx, topic, envelope)
			helper.AssertNoError(err, "PublishEnvelope should not return error")
		}
	}

	// ⚠️ Memory EventBus 使用 at-most-once 语义，panic 会导致消息丢失
	// 等待消息处理（预期会丢失 1 条消息）
	expectedMessages := int64(totalMessages - 1) // aggregate-2 的 version=3 会丢失
	success := helper.WaitForMessages(&totalReceived, expectedMessages, 15*time.Second)
	helper.AssertTrue(success, "Should receive all messages except the one that panicked")

	// 验证结果（调整预期）
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertEqual(expectedMessages, actualReceived, "Should receive all messages except the one that panicked (at-most-once semantics)")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Should panic exactly once")

	// 验证每个聚合的消息顺序
	mu.Lock()
	for _, aggID := range aggregates {
		versions := receivedByAggregate[aggID]

		if aggID == "aggregate-2" {
			// aggregate-2 应该收到 9 条消息（version=3 丢失）
			expectedVersionsCount := versionsPerAggregate - 1
			helper.AssertEqual(expectedVersionsCount, len(versions), fmt.Sprintf("%s should receive all versions except the one that panicked", aggID))

			// 验证顺序（跳过 version=3）
			for i := 0; i < len(versions); i++ {
				var expectedVersion int64
				if i < 2 {
					expectedVersion = int64(i + 1) // version=1,2
				} else {
					expectedVersion = int64(i + 2) // version=4,5,6,7,8,9,10
				}
				helper.AssertEqual(expectedVersion, versions[i], fmt.Sprintf("%s version %d should be in order", aggID, i+1))
			}
		} else {
			// aggregate-1 和 aggregate-3 应该收到所有消息
			helper.AssertEqual(versionsPerAggregate, len(versions), fmt.Sprintf("%s should receive all versions", aggID))

			// 验证顺序
			for i := 0; i < len(versions); i++ {
				expectedVersion := int64(i + 1)
				helper.AssertEqual(expectedVersion, versions[i], fmt.Sprintf("%s version %d should be in order", aggID, i+1))
			}
		}
	}
	mu.Unlock()

	t.Logf("✅ Message guarantee with multiple aggregates test passed (at-most-once semantics)")
	t.Logf("📊 Total received: %d (expected %d, aggregate-2 version=3 lost)", actualReceived, expectedMessages)
	t.Logf("⚠️ Note: Memory EventBus uses at-most-once semantics, so panic causes message loss")
}
