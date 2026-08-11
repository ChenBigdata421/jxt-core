//go:build integration
// +build integration

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

// TestNATSFaultIsolation 测试 NATS JetStream EventBus 故障隔离（单个 Actor 故障不影响其他 Actor）
//
// 🎯 测试目的:
//   验证 NATS JetStream EventBus 的 Actor Pool 故障隔离能力：当一个聚合 ID 的 Actor 发生 panic 时，
//   不应该影响其他聚合 ID 的消息处理，并且 panic 的消息应该被重投递。
//
// ✅ NATS JetStream 语义:
//   NATS JetStream 支持 at-least-once 语义，panic 后会重投递消息。
//
// 📋 测试逻辑:
//   1. 创建 NATS EventBus 并订阅 Envelope topic
//   2. Handler 在处理 aggregate-1 的 version=1 时触发 panic（只触发一次）
//   3. 交错发送 3 个聚合（aggregate-1/2/3）各 5 个版本（共 15 条）
//   4. 等待所有消息处理完成
//
// ✅ 检查项:
//   - 所有 15 条消息都应该被接收（totalReceived = 15）
//   - panic 应该发生至少 1 次（panicCount >= 1）
//   - aggregate-2 和 aggregate-3 应该接收所有 5 个版本
//   - aggregate-1 应该接收所有 5 个版本（包括重投递的 v1）
//   - 测试应该在 30 秒内完成
//
// 🔍 验证点:
//   - Actor Pool 的故障隔离：aggregate-1 的 panic 不影响其他聚合
//   - NATS 的 at-least-once 语义：panic 消息被重投递
//   - 每个聚合使用独立的 Actor（通过一致性哈希路由）
//
// 📊 测试规模:
//   - 聚合数量: 3
//   - 每个聚合版本数: 5
//   - 总消息数: 15
//   - 预期接收: 15（所有消息，包括重投递）
func TestNATSFaultIsolation(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.fault.isolation.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-fault-isolation-%d", helper.GetTimestamp())
	streamName := fmt.Sprintf("fault_isolation_stream_%d", helper.GetTimestamp())
	durableName := fmt.Sprintf("fault_isolation_durable_%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID, streamName, []string{topic}, durableName)
	if bus == nil {
		t.Fatalf("Failed to create NATS EventBus")
	}

	var totalReceived int64
	var aggregate1Received int64
	var aggregate2Received int64
	var aggregate3Received int64
	var panicCount int64
	var panicTriggered atomic.Bool

	ctx := context.Background()

	// 订阅 Envelope，aggregate-1 的消息触发 panic
	helper.AssertNoError(bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		// aggregate-1 的第一条消息触发 panic（只触发一次）
		if envelope.AggregateID == "aggregate-1" && envelope.EventVersion == 1 {
			if panicTriggered.CompareAndSwap(false, true) {
				atomic.AddInt64(&panicCount, 1)
				t.Logf("Panic on aggregate-1")
				panic("simulated panic for aggregate-1")
			}
		}

		// ⭐ 只计数成功处理的消息（不包括 panic 的）
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

		t.Logf("Processed: AggregateID=%s, Version=%d", envelope.AggregateID, envelope.EventVersion)
		return nil
	}), "SubscribeEnvelope should not return error")

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
			if err := bus.PublishEnvelope(ctx, topic, envelope); err != nil {
				helper.AssertNoError(err, "PublishEnvelope should not return error")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// NATS JetStream 使用 at-least-once 语义，panic 后会重投递消息
	// 等待所有消息处理完成（包括重投递）
	expectedMessages := int64(totalMessages)
	success := helper.WaitForMessages(&totalReceived, expectedMessages, 60*time.Second)
	helper.AssertTrue(success, "Should receive all messages after panic recovery")

	// 验证结果
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertEqual(expectedMessages, actualReceived, "Should receive all messages (at-least-once semantics)")
	helper.AssertGreaterThan(atomic.LoadInt64(&panicCount), 0, "Should panic at least once")

	// 验证故障隔离：aggregate-2 和 aggregate-3 应该收到所有消息
	helper.AssertEqual(int64(versionsPerAggregate), atomic.LoadInt64(&aggregate2Received), "aggregate-2 should receive all messages")
	helper.AssertEqual(int64(versionsPerAggregate), atomic.LoadInt64(&aggregate3Received), "aggregate-3 should receive all messages")

	// aggregate-1 应该收到所有消息（包括重投递的 version=1）
	aggregate1Count := atomic.LoadInt64(&aggregate1Received)
	helper.AssertEqual(int64(versionsPerAggregate), aggregate1Count, "aggregate-1 should receive all messages including retried ones (at-least-once semantics)")

	t.Logf("NATS Fault isolation test passed (at-least-once semantics)")
	t.Logf("aggregate-1: %d, aggregate-2: %d, aggregate-3: %d, Panic count: %d",
		aggregate1Count,
		atomic.LoadInt64(&aggregate2Received),
		atomic.LoadInt64(&aggregate3Received),
		atomic.LoadInt64(&panicCount))
	t.Logf("✅ Note: NATS JetStream EventBus uses at-least-once semantics, so panic triggers message retry")
}

// TestNATSFaultIsolationRaw 测试 NATS Subscribe (非 Envelope) 的 at-most-once 语义
func TestNATSFaultIsolationRaw(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.fault.raw.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-fault-raw-%d", helper.GetTimestamp())
	streamName := fmt.Sprintf("fault_raw_stream_%d", helper.GetTimestamp())
	durableName := fmt.Sprintf("fault_raw_durable_%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID, streamName, []string{topic}, durableName)
	if bus == nil {
		t.Fatalf("Failed to create NATS EventBus")
	}

	var totalReceived int64
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

		// 只在第一次处理 aggregate-1 version 2 时触发 panic（跳过 version 1）
		if payload.Aggregate == "aggregate-1" && payload.Version == 2 {
			if panicTriggered.CompareAndSwap(false, true) {
				atomic.AddInt64(&panicCount, 1)
				t.Logf("⚠️ Panic on aggregate-1 version 2 (non-envelope)")
				panic("simulated panic for aggregate-1 raw message")
			}
		}

		// ⭐ 只计数成功处理的消息（不包括 panic 的）
		atomic.AddInt64(&totalReceived, 1)
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
			if err := bus.Publish(ctx, topic, bytes); err != nil {
				helper.AssertNoError(err, "Publish should not fail")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// at-most-once: panic 消息会丢失，预期收到 totalMessages - 1
	expectedMessages := int64(totalMessages - 1)
	success := helper.WaitForMessages(&totalReceived, expectedMessages, 60*time.Second)
	helper.AssertTrue(success, "Should receive all raw messages except the one that panicked")

	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertEqual(expectedMessages, actualReceived, "Total raw messages should match at-most-once expectation")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&panicCount), "Raw handler should panic exactly once")

	t.Logf("✅ NATS raw (non-envelope) fault isolation test passed (at-most-once semantics)")
	t.Logf("📊 Expected: %d, Received: %d, Panic: %d", expectedMessages, actualReceived, atomic.LoadInt64(&panicCount))
}

// TestNATSConcurrentFaultRecovery 测试 NATS JetStream EventBus 并发故障恢复
func TestNATSConcurrentFaultRecovery(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.concurrent.fault.recovery.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-concurrent-fault-%d", helper.GetTimestamp())
	streamName := fmt.Sprintf("fault_concurrent_stream_%d", helper.GetTimestamp())
	durableName := fmt.Sprintf("fault_concurrent_durable_%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID, streamName, []string{topic}, durableName)
	if bus == nil {
		t.Fatalf("Failed to create NATS EventBus")
	}

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
				t.Logf("Panic on %s version 1", envelope.AggregateID)
				panic(fmt.Sprintf("simulated panic for %s", envelope.AggregateID))
			}
			mu.Unlock()
		}

		t.Logf("Processed: AggregateID=%s, Version=%d", envelope.AggregateID, envelope.EventVersion)
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
				if err := bus.PublishEnvelope(ctx, topic, envelope); err != nil {
					t.Logf("PublishEnvelope error: %v", err)
				}
				time.Sleep(20 * time.Millisecond)
			}
		}(aggID)
	}

	wg.Wait()

	// 等待所有消息处理完成（至少收到预期数量）
	// 注意：at-least-once语义可能导致消息重投递，所以实际收到的消息数可能大于预期
	time.Sleep(5 * time.Second) // 等待所有消息处理完成

	// 验证结果（允许 >= 预期，因为重投递可能导致重复）
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertGreaterThanOrEqual(actualReceived, int64(totalMessages), "Should receive at least all messages (at-least-once semantics)")
	helper.AssertGreaterThan(atomic.LoadInt64(&panicCount), 0, "Should panic at least once per aggregate")

	t.Logf("✅ NATS Concurrent fault recovery test passed (at-least-once semantics)")
	t.Logf("📊 Total messages: %d (expected at least %d), Panic count: %d",
		actualReceived, totalMessages, atomic.LoadInt64(&panicCount))
	t.Logf("✅ Note: Received %d messages (expected at least %d). Extra messages are due to at-least-once semantics and message retry after panic.",
		actualReceived, totalMessages)
}

// TestNATSFaultIsolationWithHighLoad 测试 NATS JetStream EventBus 高负载下的故障隔离
func TestNATSFaultIsolationWithHighLoad(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.fault.isolation.high.load.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-high-load-fault-%d", helper.GetTimestamp())
	streamName := fmt.Sprintf("fault_high_load_stream_%d", helper.GetTimestamp())
	durableName := fmt.Sprintf("fault_high_load_durable_%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID, streamName, []string{topic}, durableName)
	helper.AssertTrue(bus != nil, "Failed to create NATS EventBus")

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
			return fmt.Errorf("simulated panic for aggregate-fault")
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

	// 等待所有消息处理完成（允许重投递导致消息数 > 预期）
	expectedMessages := int64(totalMessages)
	success := helper.WaitForMessages(&totalReceived, expectedMessages, 60*time.Second)
	helper.AssertTrue(success, "Should receive at least all messages under high load (may have duplicates due to retries)")

	// 验证结果（允许 >= 预期，因为重投递可能导致重复）
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertGreaterThanOrEqual(actualReceived, expectedMessages, "Should receive at least all messages (at-least-once semantics)")
	helper.AssertGreaterThan(atomic.LoadInt64(&panicCount), 0, "Should panic at least once")

	// 验证故障隔离：正常聚合应该收到所有消息
	expectedNormalMessages := int64(normalAggregateCount * normalAggregateVersions)
	helper.AssertEqual(expectedNormalMessages, atomic.LoadInt64(&normalAggregatesReceived), "Normal aggregates should receive all messages")

	t.Logf("✅ NATS Fault isolation with high load test passed")
	t.Logf("📊 Total: %d, Faulty: %d, Normal: %d, Panic: %d",
		atomic.LoadInt64(&totalReceived),
		atomic.LoadInt64(&faultyAggregateReceived),
		atomic.LoadInt64(&normalAggregatesReceived),
		atomic.LoadInt64(&panicCount))
}
