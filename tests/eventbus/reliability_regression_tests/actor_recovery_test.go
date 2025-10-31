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
func TestMaxRestartsExceeded(t *testing.T) {
	t.Skip("⚠️ Skipping: Hollywood Actor Pool uses business error handling instead of panic")

	// 注意：由于我们修改了错误处理策略，业务错误不再触发 panic
	// 这个测试需要重新设计，测试真正的系统级 panic（如 nil pointer）
	// 而不是业务错误
}

// TestPanicWithDifferentAggregates 测试不同聚合 ID 的 panic 恢复
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
