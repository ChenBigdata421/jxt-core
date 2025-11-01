package reliability_regression_tests

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// TestActorPanicRecovery 测试 Actor panic 后自动恢复
//
// 🎯 测试目的:
//   验证 Hollywood Actor Pool 的 Supervisor 机制能够在 Actor 发生 panic 后自动重启，
//   并继续处理后续消息，确保系统的容错能力和可用性。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 topic
//   2. Handler 在处理第 1 条消息时触发 panic
//   3. 发送 3 条消息（msg1-panic, msg2-normal, msg3-normal）
//   4. 等待所有消息处理完成
//
// ✅ 检查项:
//   - 所有 3 条消息都应该被接收和计数（received = 3）
//   - panic 应该只发生 1 次（panicCount = 1）
//   - Actor 应该在 panic 后自动重启并继续处理后续消息
//   - 测试应该在 5 秒内完成
//
// 🔍 验证点:
//   - Memory EventBus 的 at-most-once 语义：panic 消息被计数但不重投
//   - Supervisor 的自动重启机制正常工作
//   - 后续消息不受 panic 影响
func TestActorPanicRecovery(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.actor.panic.recovery.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var received int64
	var panicCount int64
	ctx := context.Background()

	// 订阅消息，第一条消息触发 panic
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, msg []byte) error {
		count := atomic.AddInt64(&received, 1)

		// 第一条消息触发 panic（模拟故障）
		if count == 1 {
			atomic.AddInt64(&panicCount, 1)
			panic("simulated panic for testing")
		}

		// 后续消息正常处理
		t.Logf("📨 Processed message %d: %s", count, string(msg))
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送多条消息
	messages := []string{"msg1-panic", "msg2-normal", "msg3-normal"}
	for i, msg := range messages {
		err = bus.Publish(ctx, topic, []byte(msg))
		helper.AssertNoError(err, fmt.Sprintf("Publish message %d should not return error", i+1))
	}

	// 等待所有消息处理完成
	success := helper.WaitForMessages(&received, int64(len(messages)), 5*time.Second)
	helper.AssertTrue(success, "Should receive all messages after panic recovery")

	// 验证结果
	helper.AssertEqual(int64(len(messages)), atomic.LoadInt64(&received), "Should receive all messages")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Should panic exactly once")

	t.Logf("✅ Actor panic recovery test passed")
}

// TestMultiplePanicRestarts 测试多次 panic 重启（不超过 MaxRestarts）
//
// 🎯 测试目的:
//   验证 Supervisor 能够处理连续多次 panic，在不超过 MaxRestarts（默认 3 次）的情况下，
//   Actor 能够持续重启并恢复正常工作。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 topic
//   2. Handler 在处理前 3 条消息时都触发 panic
//   3. 发送 5 条消息，每次发送间隔 50ms（给 Actor 恢复时间）
//   4. 等待所有消息处理完成
//
// ✅ 检查项:
//   - 所有 5 条消息都应该被接收和计数（received = 5）
//   - panic 应该发生 3 次（panicCount = 3）
//   - Actor 应该在每次 panic 后重启
//   - 第 4、5 条消息应该正常处理（不再 panic）
//   - 测试应该在 10 秒内完成
//
// 🔍 验证点:
//   - Supervisor 的多次重启能力（MaxRestarts = 3）
//   - 重启计数器在成功处理消息后不会重置
//   - Actor 在达到 MaxRestarts 前能够恢复正常
func TestMultiplePanicRestarts(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.multiple.panic.restarts.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var received int64
	var panicCount int64
	ctx := context.Background()

	// 订阅消息，前 3 条消息触发 panic（MaxRestarts = 3）
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, msg []byte) error {
		count := atomic.AddInt64(&received, 1)

		// 前 3 条消息触发 panic
		if count <= 3 {
			atomic.AddInt64(&panicCount, 1)
			t.Logf("⚠️ Panic on message %d", count)
			panic(fmt.Sprintf("simulated panic %d", count))
		}

		// 后续消息正常处理
		t.Logf("📨 Processed message %d: %s", count, string(msg))
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送 5 条消息
	messageCount := 5
	for i := 1; i <= messageCount; i++ {
		msg := fmt.Sprintf("msg%d", i)
		err = bus.Publish(ctx, topic, []byte(msg))
		helper.AssertNoError(err, fmt.Sprintf("Publish message %d should not return error", i))
		time.Sleep(50 * time.Millisecond) // 给 Actor 时间恢复
	}

	// 等待所有消息处理完成
	success := helper.WaitForMessages(&received, int64(messageCount), 10*time.Second)
	helper.AssertTrue(success, "Should receive all messages after multiple restarts")

	// 验证结果
	helper.AssertEqual(int64(messageCount), atomic.LoadInt64(&received), "Should receive all messages")
	helper.AssertEqual(int64(3), atomic.LoadInt64(&panicCount), "Should panic exactly 3 times")

	t.Logf("✅ Multiple panic restarts test passed")
}

// TestMaxRestartsExceeded 测试达到最大重启次数
//
// 🎯 测试目的:
//   验证当 Actor 的 panic 次数超过 MaxRestarts 限制时，Supervisor 的行为。
//
// ⚠️ 当前状态: SKIPPED
//   由于 Hollywood Actor Pool 使用业务错误处理策略而不是 panic，
//   这个测试需要重新设计来测试真正的系统级 panic（如 nil pointer）。
//
// 📋 原计划测试逻辑:
//   1. 创建 Memory EventBus 并订阅 topic
//   2. Handler 持续触发 panic（超过 MaxRestarts 次数）
//   3. 验证 Actor 停止重启或进入错误状态
//
// ✅ 原计划检查项:
//   - Actor 在超过 MaxRestarts 后停止处理消息
//   - 系统应该记录错误日志
//   - 后续消息不应该被处理
//
// 🔄 需要重新设计:
//   - 使用系统级 panic（如访问 nil 指针）而非业务错误
//   - 或者测试其他资源耗尽场景
func TestMaxRestartsExceeded(t *testing.T) {
	t.Skip("⚠️ Skipping: Hollywood Actor Pool uses business error handling instead of panic")

	// 注意：由于我们修改了错误处理策略，业务错误不再触发 panic
	// 这个测试需要重新设计，测试真正的系统级 panic（如 nil pointer）
	// 而不是业务错误
}

// TestPanicWithDifferentAggregates 测试不同聚合 ID 的 panic 恢复
//
// 🎯 测试目的:
//   验证 Actor Pool 的故障隔离能力：一个聚合 ID 的 Actor 发生 panic 后，
//   不应该影响其他聚合 ID 的消息处理。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 Envelope topic
//   2. Handler 在处理 aggregate-1 的 version=1 时触发 panic
//   3. 发送 3 个聚合（aggregate-1/2/3）各 3 个版本的消息（共 9 条）
//   4. 等待所有消息处理完成
//
// ✅ 检查项:
//   - 所有 9 条消息都应该被接收（received = 9）
//   - panic 应该只发生 1 次（panicCount = 1）
//   - aggregate-2 和 aggregate-3 的消息不受影响
//   - aggregate-1 的后续版本（v2, v3）应该正常处理
//   - 测试应该在 10 秒内完成
//
// 🔍 验证点:
//   - Actor Pool 的故障隔离：不同聚合使用不同 Actor
//   - 一个 Actor 的 panic 不影响其他 Actor
//   - 同一聚合的 Actor 在 panic 后能够恢复并处理后续消息
func TestPanicWithDifferentAggregates(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.panic.different.aggregates.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var received int64
	var panicCount int64
	ctx := context.Background()

	// 订阅 Envelope，aggregate-1 的第一条消息触发 panic
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		atomic.AddInt64(&received, 1)

		// aggregate-1 的第一条消息触发 panic
		if envelope.AggregateID == "aggregate-1" && envelope.EventVersion == 1 {
			atomic.AddInt64(&panicCount, 1)
			t.Logf("⚠️ Panic on aggregate-1 version 1")
			panic("simulated panic for aggregate-1")
		}

		// 其他消息正常处理
		t.Logf("📨 Processed: AggregateID=%s, Version=%d", envelope.AggregateID, envelope.EventVersion)
		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送多个聚合的消息
	aggregates := []string{"aggregate-1", "aggregate-2", "aggregate-3"}
	versionsPerAggregate := 3
	totalMessages := len(aggregates) * versionsPerAggregate

	for _, aggID := range aggregates {
		for version := 1; version <= versionsPerAggregate; version++ {
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
			time.Sleep(20 * time.Millisecond)
		}
	}

	// 等待所有消息处理完成
	success := helper.WaitForMessages(&received, int64(totalMessages), 10*time.Second)
	helper.AssertTrue(success, "Should receive all messages after panic recovery")

	// 验证结果
	helper.AssertEqual(int64(totalMessages), atomic.LoadInt64(&received), "Should receive all messages")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Should panic exactly once")

	t.Logf("✅ Panic with different aggregates test passed")
}

// TestRecoveryLatency 测试恢复延迟
//
// 🎯 测试目的:
//   测量 Actor 从 panic 到恢复并处理下一条消息的时间延迟，
//   验证 Supervisor 的重启速度是否满足性能要求。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 topic
//   2. Handler 在处理第 1 条消息时记录时间并触发 panic
//   3. Handler 在处理第 2 条消息时记录恢复时间
//   4. 发送 2 条消息，间隔 50ms
//   5. 计算恢复延迟（recoveryTime - panicTime）
//
// ✅ 检查项:
//   - 两条消息都应该被接收（received = 2）
//   - 恢复延迟应该 < 1 秒（Memory EventBus 应该很快）
//   - panicTime 和 recoveryTime 都应该被正确记录
//   - 测试应该在 5 秒内完成
//
// 🔍 验证点:
//   - Supervisor 的重启速度
//   - Actor 恢复后能够立即处理消息
//   - Memory EventBus 的低延迟特性
//
// 📊 性能基准:
//   - 预期恢复延迟 < 1s（通常在毫秒级）
func TestRecoveryLatency(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.recovery.latency.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var received int64
	var panicTime time.Time
	var recoveryTime time.Time
	ctx := context.Background()

	// 订阅消息，第一条消息触发 panic
	err := bus.Subscribe(ctx, topic, func(ctx context.Context, msg []byte) error {
		count := atomic.AddInt64(&received, 1)

		if count == 1 {
			panicTime = time.Now()
			t.Logf("⚠️ Panic at %v", panicTime)
			panic("simulated panic for latency test")
		}

		if count == 2 {
			recoveryTime = time.Now()
			t.Logf("✅ Recovered at %v", recoveryTime)
		}

		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送 2 条消息
	for i := 1; i <= 2; i++ {
		err = bus.Publish(ctx, topic, []byte(fmt.Sprintf("msg%d", i)))
		helper.AssertNoError(err, "Publish should not return error")
		time.Sleep(50 * time.Millisecond)
	}

	// 等待消息处理完成
	success := helper.WaitForMessages(&received, 2, 5*time.Second)
	helper.AssertTrue(success, "Should receive both messages")

	// 计算恢复延迟
	if !panicTime.IsZero() && !recoveryTime.IsZero() {
		latency := recoveryTime.Sub(panicTime)
		t.Logf("📊 Recovery latency: %v", latency)

		// 验证恢复延迟 < 1 秒（Memory EventBus 应该很快）
		helper.AssertTrue(latency < 1*time.Second, "Recovery latency should be < 1s")
	}

	t.Logf("✅ Recovery latency test passed")
}
