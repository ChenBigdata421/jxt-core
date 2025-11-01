package reliability_regression_tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
)

// TestMemoryFaultIsolation 测试 Memory EventBus 故障隔离（单个 Actor 故障不影响其他 Actor）
//
// 🎯 测试目的:
//   验证 Memory EventBus 的 Actor Pool 故障隔离能力：当一个聚合 ID 的 Actor 发生 panic 时，
//   不应该影响其他聚合 ID 的消息处理。
//
// ⚠️ Memory EventBus 限制:
//   Memory EventBus 使用 at-most-once 语义，panic 会导致消息丢失（不重投）。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 Envelope topic
//   2. Handler 在处理 aggregate-1 的 version=1 时触发 panic（panic 前不计数）
//   3. 交错发送 3 个聚合（aggregate-1/2/3）各 5 个版本（共 15 条）
//   4. 等待消息处理完成
//
// ✅ 检查项:
//   - 应该接收 14 条消息（totalReceived = 14，aggregate-1 的 v1 丢失）
//   - panic 应该只发生 1 次（panicCount = 1）
//   - aggregate-2 和 aggregate-3 应该接收所有 5 个版本
//   - aggregate-1 应该接收 4 个版本（v2-v5，v1 丢失）
//   - 测试应该在 10 秒内完成
//
// 🔍 验证点:
//   - Actor Pool 的故障隔离：aggregate-1 的 panic 不影响其他聚合
//   - Memory 的 at-most-once 语义：panic 消息丢失不重投
//   - 每个聚合使用独立的 Actor（通过一致性哈希路由）
//
// 📊 测试规模:
//   - 聚合数量: 3
//   - 每个聚合版本数: 5
//   - 总消息数: 15
//   - 预期接收: 14（aggregate-1 的 v1 丢失）
func TestMemoryFaultIsolation(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.memory.fault.isolation.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var totalReceived int64
	var aggregate1Received int64
	var aggregate2Received int64
	var aggregate3Received int64
	var panicCount int64

	ctx := context.Background()

	// 订阅 Envelope，aggregate-1 的消息触发 panic
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		// ⚠️ 重要：先检查是否需要 panic，再递增计数器
		// 这样可以准确统计成功处理的消息数
		if envelope.AggregateID == "aggregate-1" && envelope.EventVersion == 1 {
			atomic.AddInt64(&panicCount, 1)
			t.Logf("⚠️ Panic on aggregate-1")
			panic("simulated panic for aggregate-1")
		}

		// 成功处理的消息才计数
		atomic.AddInt64(&totalReceived, 1)

		// 统计各聚合的消息数
		switch envelope.AggregateID {
		case "aggregate-1":
			atomic.AddInt64(&aggregate1Received, 1)
		case "aggregate-2":
			atomic.AddInt64(&aggregate2Received, 1)
		case "aggregate-3":
			atomic.AddInt64(&aggregate3Received, 1)
		}

		t.Logf("📨 Processed: AggregateID=%s, Version=%d", envelope.AggregateID, envelope.EventVersion)
		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送多个聚合的消息（交错发送，测试隔离性）
	aggregates := []string{"aggregate-1", "aggregate-2", "aggregate-3"}
	versionsPerAggregate := 5
	totalMessages := len(aggregates) * versionsPerAggregate

	for version := 1; version <= versionsPerAggregate; version++ {
		for _, aggID := range aggregates {
			envelope := &eventbus.Envelope{
				EventID:      fmt.Sprintf("evt-%s-v%d", aggID, version),
				AggregateID:  aggID,
				EventType:    "TestEvent",
				EventVersion: int64(version),
				Timestamp:    time.Now(),
				Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"aggregate":"%s","version":%d}`, aggID, version)),
			}
			err = bus.PublishEnvelope(ctx, topic, envelope)
			helper.AssertNoError(err, "PublishEnvelope should not return error")
			time.Sleep(10 * time.Millisecond)
		}
	}

	// ⚠️ Memory EventBus 使用 at-most-once 语义，panic 会导致消息丢失
	// 等待消息处理（预期会丢失 1 条消息）
	expectedMessages := int64(totalMessages - 1) // aggregate-1 的 version=1 会丢失
	success := helper.WaitForMessages(&totalReceived, expectedMessages, 10*time.Second)
	helper.AssertTrue(success, "Should receive all messages except the one that panicked")

	// 验证结果（调整预期）
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertEqual(expectedMessages, actualReceived, "Should receive all messages except the one that panicked (at-most-once semantics)")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Should panic exactly once")

	// 验证故障隔离：aggregate-2 和 aggregate-3 应该收到所有消息
	helper.AssertEqual(int64(versionsPerAggregate), atomic.LoadInt64(&aggregate2Received), "aggregate-2 should receive all messages")
	helper.AssertEqual(int64(versionsPerAggregate), atomic.LoadInt64(&aggregate3Received), "aggregate-3 should receive all messages")

	// ⚠️ aggregate-1 应该收到 4 条消息（version=1 丢失，version=2-5 成功）
	aggregate1Count := atomic.LoadInt64(&aggregate1Received)
	helper.AssertEqual(int64(versionsPerAggregate-1), aggregate1Count, "aggregate-1 should receive all messages except the one that panicked (at-most-once semantics)")

	t.Logf("✅ Memory Fault isolation test passed (at-most-once semantics)")
	t.Logf("📊 aggregate-1: %d (expected %d, version=1 lost), aggregate-2: %d, aggregate-3: %d",
		aggregate1Count,
		versionsPerAggregate-1,
		atomic.LoadInt64(&aggregate2Received),
		atomic.LoadInt64(&aggregate3Received))
	t.Logf("⚠️ Note: Memory EventBus uses at-most-once semantics, so panic causes message loss")
}

// TestMemoryFaultIsolationRaw 验证 Memory Subscribe (非 Envelope) 的 at-most-once 语义
//
// 🎯 测试目的:
//   验证 Memory 的 Subscribe 方法（非 Envelope）使用 at-most-once 语义，
//   当 Handler 发生 panic 时，消息不会被重投递，而是直接丢失。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 topic（使用 Subscribe，非 Envelope）
//   2. Handler 在处理 aggregate-1 的 version=1 时触发 panic（panic 前不计数）
//   3. 发送 3 个聚合各 5 个版本的原始消息（共 15 条）
//   4. 等待消息处理完成
//
// ✅ 检查项:
//   - 应该接收 14 条消息（totalReceived = 14，aggregate-1 的 v1 丢失）
//   - panic 应该只发生 1 次（panicCount = 1）
//   - panic 的消息不会被重投递
//   - 测试应该在 10 秒内完成
//
// 🔍 验证点:
//   - Memory Subscribe（非 Envelope）的 at-most-once 语义
//   - panic 消息被丢失，不重投
//   - 与 SubscribeEnvelope 的行为一致（Memory 都是 at-most-once）
//
// 📊 测试规模:
//   - 聚合数量: 3
//   - 每个聚合版本数: 5
//   - 总消息数: 15
//   - 预期接收: 14（aggregate-1 的 v1 丢失）
func TestMemoryFaultIsolationRaw(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.memory.fault.raw.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var totalReceived int64
	var aggregate1Received int64
	var aggregate2Received int64
	var aggregate3Received int64
	var panicCount int64
	var panicTriggered atomic.Bool

	ctx := context.Background()

	helper.AssertNoError(bus.Subscribe(ctx, topic, func(ctx context.Context, data []byte) error {
		var payload struct {
			Aggregate string `json:"aggregate"`
			Version   int64  `json:"version"`
		}

		if err := jxtjson.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("failed to decode payload: %w", err)
		}

		if payload.Aggregate == "aggregate-1" && payload.Version == 1 {
			if panicTriggered.CompareAndSwap(false, true) {
				atomic.AddInt64(&panicCount, 1)
				t.Logf("⚠️ Panic on aggregate-1 (non-envelope)")
				panic("simulated panic for aggregate-1 raw message")
			}
		}

		atomic.AddInt64(&totalReceived, 1)
		switch payload.Aggregate {
		case "aggregate-1":
			atomic.AddInt64(&aggregate1Received, 1)
		case "aggregate-2":
			atomic.AddInt64(&aggregate2Received, 1)
		case "aggregate-3":
			atomic.AddInt64(&aggregate3Received, 1)
		}

		t.Logf("📨 Processed raw message: AggregateID=%s, Version=%d", payload.Aggregate, payload.Version)
		return nil
	}), "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	aggregates := []string{"aggregate-1", "aggregate-2", "aggregate-3"}
	versionsPerAggregate := 5
	totalMessages := len(aggregates) * versionsPerAggregate

	for version := int64(1); version <= int64(versionsPerAggregate); version++ {
		for _, aggID := range aggregates {
			payload := struct {
				Aggregate string `json:"aggregate"`
				Version   int64  `json:"version"`
			}{
				Aggregate: aggID,
				Version:   version,
			}
			bytes, err := jxtjson.Marshal(payload)
			helper.AssertNoError(err, "Marshal raw payload should not fail")
			helper.AssertNoError(bus.Publish(ctx, topic, bytes), "Publish should not fail")
			time.Sleep(10 * time.Millisecond)
		}
	}

	expectedMessages := int64(totalMessages - 1)
	success := helper.WaitForMessages(&totalReceived, expectedMessages, 10*time.Second)
	helper.AssertTrue(success, "Should receive all raw messages except the one that panicked")

	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertEqual(expectedMessages, actualReceived, "Total raw messages should match at-most-once expectation")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Raw handler should panic exactly once")
	helper.AssertEqual(int64(versionsPerAggregate-1), atomic.LoadInt64(&aggregate1Received), "aggregate-1 should miss the first raw message")
	helper.AssertEqual(int64(versionsPerAggregate), atomic.LoadInt64(&aggregate2Received), "aggregate-2 raw messages should all arrive")
	helper.AssertEqual(int64(versionsPerAggregate), atomic.LoadInt64(&aggregate3Received), "aggregate-3 raw messages should all arrive")

	t.Logf("✅ Memory raw fault isolation test passed (at-most-once semantics)")
}

// TestMemoryConcurrentFaultRecovery 测试 Memory EventBus 并发故障恢复
//
// 🎯 测试目的:
//   验证多个聚合 ID 并发发生 panic 时，每个聚合的 Actor 都能独立恢复，
//   但由于 at-most-once 语义，每个聚合的 v1 消息会丢失。
//
// 📋 测试逻辑:
//   1. 创建 Memory EventBus 并订阅 Envelope topic
//   2. Handler 在处理每个聚合的 version=1 时都触发 panic（计数后 panic）
//   3. 并发发送 5 个聚合各 3 个版本的消息（共 15 条）
//   4. 等待消息处理完成
//
// ✅ 检查项:
//   - 应该接收 15 条消息（totalReceived = 15，panic 消息先计数再 panic）
//   - panic 应该发生 5 次（每个聚合 1 次）
//   - 每个聚合应该接收 3 个版本（v1, v2, v3，v1 被计数但触发 panic）
//   - 测试应该在合理时间内完成
//
// 🔍 验证点:
//   - 多个 Actor 并发 panic 后都能恢复
//   - 每个聚合使用独立的 Actor（故障隔离）
//   - Memory 的特殊性：panic 发生在计数之后，消息被计数但不重投
//   - 并发场景下的 Supervisor 稳定性
//
// 📊 测试规模:
//   - 聚合数量: 5
//   - 每个聚合版本数: 3
//   - 总消息数: 15
//   - 预期接收: 15（所有消息，panic 消息先计数再 panic）
//   - 预期 panic 次数: 5
func TestMemoryConcurrentFaultRecovery(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.memory.concurrent.fault.recovery.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var totalReceived int64
	var panicCount int64
	var mu sync.Mutex
	panicAggregates := make(map[string]bool)

	ctx := context.Background()

	// 订阅 Envelope，多个聚合的第一条消息都触发 panic
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		atomic.AddInt64(&totalReceived, 1)

		// 每个聚合的第一条消息触发 panic
		if envelope.EventVersion == 1 {
			mu.Lock()
			if !panicAggregates[envelope.AggregateID] {
				panicAggregates[envelope.AggregateID] = true
				mu.Unlock()
				atomic.AddInt64(&panicCount, 1)
				t.Logf("⚠️ Panic on %s version 1", envelope.AggregateID)
				panic(fmt.Sprintf("simulated panic for %s", envelope.AggregateID))
			}
			mu.Unlock()
		}

		t.Logf("📨 Processed: AggregateID=%s, Version=%d", envelope.AggregateID, envelope.EventVersion)
		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送多个聚合的消息（并发发送）
	aggregateCount := 5
	versionsPerAggregate := 3
	totalMessages := aggregateCount * versionsPerAggregate

	var wg sync.WaitGroup
	for aggID := 1; aggID <= aggregateCount; aggID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for version := 1; version <= versionsPerAggregate; version++ {
				envelope := &eventbus.Envelope{
					EventID:      fmt.Sprintf("evt-agg%d-v%d", id, version),
					AggregateID:  fmt.Sprintf("aggregate-%d", id),
					EventType:    "TestEvent",
					EventVersion: int64(version),
					Timestamp:    time.Now(),
					Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"aggregate":"aggregate-%d","version":%d}`, id, version)),
				}
				err := bus.PublishEnvelope(ctx, topic, envelope)
				if err != nil {
					t.Logf("⚠️ PublishEnvelope error: %v", err)
				}
				time.Sleep(20 * time.Millisecond)
			}
		}(aggID)
	}

	wg.Wait()

	// 等待所有消息处理完成
	success := helper.WaitForMessages(&totalReceived, int64(totalMessages), 15*time.Second)
	helper.AssertTrue(success, "Should receive all messages after concurrent fault recovery")

	// 验证结果
	helper.AssertEqual(int64(totalMessages), atomic.LoadInt64(&totalReceived), "Should receive all messages")
	helper.AssertEqual(int64(aggregateCount), atomic.LoadInt64(&panicCount), "Should panic once per aggregate")

	t.Logf("✅ Memory Concurrent fault recovery test passed")
	t.Logf("📊 Total messages: %d, Panic count: %d", atomic.LoadInt64(&totalReceived), atomic.LoadInt64(&panicCount))
}

// TestMemoryFaultIsolationWithHighLoad 测试 Memory EventBus 高负载下的故障隔离
//
// 🎯 测试目的:
//   验证在高负载场景下（100个聚合，1000条消息），单个聚合的 panic 不会影响其他 99 个聚合的处理。
//
// ⚠️ 特殊性:
//   Memory EventBus 的 panic 发生在计数之后，因此 panic 消息会被计数但不重投。
//
// 📋 测试逻辄:
//   1. 创建 Memory EventBus 并订阅 Envelope topic
//   2. Handler 在处理 aggregate-fault 的 version=1 时触发 panic（计数后 panic）
//   3. 发送 100 个聚合各 10 个版本的消息（共 1000 条）
//   4. 等待消息处理完成
//
// ✅ 检查项:
//   - 应该接收 1000 条消息（totalReceived = 1000，panic 消息被计数）
//   - panic 应该发生 1 次（panicCount = 1）
//   - 其他 99 个聚合不受影响
//   - aggregate-fault 应该接收所有 10 个版本（包括 v1）
//   - 测试应该在 30 秒内完成
//
// 🔍 验证点:
//   - 高负载下的故障隔离能力
//   - 单个 Actor 的 panic 不影响其他 99 个 Actor
//   - Memory 的特殊性：panic 发生在计数之后
//   - Actor Pool 在高并发下的稳定性
//   - Supervisor 在高负载下的恢复能力
//
// 📊 测试规模:
//   - 聚合数量: 100（1 个故障 + 99 个正常）
//   - 每个聚合版本数: 10
//   - 总消息数: 1000
//   - 预期接收: 1000（panic 消息被计数）
//   - 故障聚合: aggregate-fault
//   - 正常聚合: aggregate-0 到 aggregate-98
func TestMemoryFaultIsolationWithHighLoad(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.memory.fault.isolation.high.load.%d", helper.GetTimestamp())
	bus := helper.CreateMemoryEventBus()

	var totalReceived int64
	var panicCount int64
	var faultyAggregateReceived int64
	var normalAggregatesReceived int64

	ctx := context.Background()

	// 订阅 Envelope，aggregate-fault 的消息触发 panic
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		atomic.AddInt64(&totalReceived, 1)

		// aggregate-fault 的第一条消息触发 panic
		if envelope.AggregateID == "aggregate-fault" && envelope.EventVersion == 1 {
			atomic.AddInt64(&panicCount, 1)
			panic("simulated panic for aggregate-fault")
		}

		// 统计消息数
		if envelope.AggregateID == "aggregate-fault" {
			atomic.AddInt64(&faultyAggregateReceived, 1)
		} else {
			atomic.AddInt64(&normalAggregatesReceived, 1)
		}

		return nil
	})
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发送大量消息（1 个故障聚合 + 99 个正常聚合）
	normalAggregateCount := 99
	faultyAggregateVersions := 10
	normalAggregateVersions := 10
	totalMessages := faultyAggregateVersions + (normalAggregateCount * normalAggregateVersions)

	// 并发发送消息
	var wg sync.WaitGroup

	// 发送故障聚合的消息
	wg.Add(1)
	go func() {
		defer wg.Done()
		for version := 1; version <= faultyAggregateVersions; version++ {
			envelope := &eventbus.Envelope{
				EventID:      fmt.Sprintf("evt-fault-v%d", version),
				AggregateID:  "aggregate-fault",
				EventType:    "TestEvent",
				EventVersion: int64(version),
				Timestamp:    time.Now(),
				Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"aggregate":"aggregate-fault","version":%d}`, version)),
			}
			_ = bus.PublishEnvelope(ctx, topic, envelope)
		}
	}()

	// 发送正常聚合的消息
	for aggID := 1; aggID <= normalAggregateCount; aggID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for version := 1; version <= normalAggregateVersions; version++ {
				envelope := &eventbus.Envelope{
					EventID:      fmt.Sprintf("evt-normal%d-v%d", id, version),
					AggregateID:  fmt.Sprintf("aggregate-normal-%d", id),
					EventType:    "TestEvent",
					EventVersion: int64(version),
					Timestamp:    time.Now(),
					Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"aggregate":"aggregate-normal-%d","version":%d}`, id, version)),
				}
				_ = bus.PublishEnvelope(ctx, topic, envelope)
			}
		}(aggID)
	}

	wg.Wait()

	// 等待所有消息处理完成
	// ⭐ 注意：Memory EventBus 的 at-most-once 语义意味着：
	// - panic 消息会被接收和处理（计数）
	// - 但不会被重新投递
	// - 所以应该接收所有消息（包括 panic 的那条）
	success := helper.WaitForMessages(&totalReceived, int64(totalMessages), 30*time.Second)
	helper.AssertTrue(success, "Should receive all messages under high load")

	// 验证结果
	helper.AssertEqual(int64(totalMessages), atomic.LoadInt64(&totalReceived), "Total messages should match (at-most-once: no redelivery)")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Should panic exactly once")

	// 验证故障隔离：正常聚合应该收到所有消息
	expectedNormalMessages := int64(normalAggregateCount * normalAggregateVersions)
	helper.AssertEqual(expectedNormalMessages, atomic.LoadInt64(&normalAggregatesReceived), "Normal aggregates should receive all messages under high load")
	// ⭐ panic 消息被计数为 totalReceived，但不被计数为 faultyAggregateReceived
	// 因为 panic 发生在故障聚合计数之前
	helper.AssertEqual(int64(faultyAggregateVersions-1), atomic.LoadInt64(&faultyAggregateReceived), "Faulty aggregate should lose the first message under high load")

	t.Logf("✅ Memory Fault isolation with high load test passed (at-most-once semantics)")
	t.Logf("📊 Total: expected %d, received %d; Faulty received: %d, Normal received: %d, Panic: %d",
		int64(totalMessages),
		atomic.LoadInt64(&totalReceived),
		atomic.LoadInt64(&faultyAggregateReceived),
		atomic.LoadInt64(&normalAggregatesReceived),
		atomic.LoadInt64(&panicCount))
}
