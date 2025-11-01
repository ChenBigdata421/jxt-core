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
// ⚠️ Memory EventBus 限制：无法实现 at-least-once 语义，panic 会导致消息丢失
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
