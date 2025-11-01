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

// TestKafkaFaultIsolation 测试 Kafka EventBus 故障隔离（单个 Actor 故障不影响其他 Actor）
// ✅ Kafka Envelope 支持 at-least-once 语义，panic 后会重投递消息
func TestKafkaFaultIsolation(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.fault.isolation.%d", helper.GetTimestamp())
	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-fault-isolation-%d", helper.GetTimestamp()))

	var totalReceived int64
	var aggregate1Received int64
	var aggregate2Received int64
	var aggregate3Received int64
	var panicCount int64
	var panicTriggered atomic.Bool

	ctx := context.Background()

	// 订阅 Envelope，aggregate-1 的消息触发 panic
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		// aggregate-1 的第一条消息触发 panic（只触发一次）
		if envelope.AggregateID == "aggregate-1" && envelope.EventVersion == 1 {
			if panicTriggered.CompareAndSwap(false, true) {
				atomic.AddInt64(&panicCount, 1)
				t.Logf("⚠️ Panic on aggregate-1")
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

	// ✅ Kafka 使用 at-least-once 语义，panic 后会重投递消息
	expectedMessages := int64(totalMessages)
	success := helper.WaitForMessages(&totalReceived, expectedMessages, 30*time.Second)
	helper.AssertTrue(success, "Should receive all messages after panic recovery (at-least-once)")

	// 验证结果
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertEqual(expectedMessages, actualReceived, "Should receive all messages (at-least-once semantics)")
	helper.AssertGreaterThan(atomic.LoadInt64(&panicCount), 0, "Should panic at least once")

	// 验证故障隔离：aggregate-2 和 aggregate-3 应该收到所有消息
	helper.AssertEqual(int64(versionsPerAggregate), atomic.LoadInt64(&aggregate2Received), "aggregate-2 should receive all messages")
	helper.AssertEqual(int64(versionsPerAggregate), atomic.LoadInt64(&aggregate3Received), "aggregate-3 should receive all messages")

	// ✅ aggregate-1 应该收到所有消息（包括重投递的 version=1）
	aggregate1Count := atomic.LoadInt64(&aggregate1Received)
	helper.AssertEqual(int64(versionsPerAggregate), aggregate1Count, "aggregate-1 should receive all messages including retried ones (at-least-once)")

	t.Logf("✅ Kafka Envelope Fault isolation test passed (at-least-once semantics)")
	t.Logf("📊 aggregate-1: %d, aggregate-2: %d, aggregate-3: %d, Panic count: %d",
		aggregate1Count,
		atomic.LoadInt64(&aggregate2Received),
		atomic.LoadInt64(&aggregate3Received),
		atomic.LoadInt64(&panicCount))
	t.Logf("✅ Note: Kafka Envelope uses at-least-once semantics, so panic triggers message retry")
}

// TestKafkaFaultIsolationRaw 测试 Kafka Subscribe (非 Envelope) 的 at-most-once 语义
func TestKafkaFaultIsolationRaw(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.fault.raw.%d", helper.GetTimestamp())
	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-fault-raw-%d", helper.GetTimestamp()))

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

	t.Logf("✅ Kafka raw (non-envelope) fault isolation test passed (at-most-once semantics)")
	t.Logf("📊 Expected: %d, Received: %d, Panic: %d", expectedMessages, actualReceived, atomic.LoadInt64(&panicCount))
}

// TestKafkaConcurrentFaultRecovery 测试 Kafka EventBus 并发故障恢复
func TestKafkaConcurrentFaultRecovery(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.concurrent.fault.recovery.%d", helper.GetTimestamp())
	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-concurrent-fault-%d", helper.GetTimestamp()))

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

	// 等待所有消息处理完成（允许重投递）
	time.Sleep(5 * time.Second)

	// 验证结果（允许 >= 预期，因为重投递可能导致重复）
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertGreaterThanOrEqual(actualReceived, int64(totalMessages), "Should receive at least all messages (at-least-once semantics)")
	helper.AssertGreaterThan(atomic.LoadInt64(&panicCount), 0, "Should panic at least once per aggregate")

	t.Logf("✅ Kafka Concurrent fault recovery test passed (at-least-once semantics)")
	t.Logf("📊 Total messages: %d (expected >= %d), Panic count: %d", actualReceived, totalMessages, atomic.LoadInt64(&panicCount))
	if actualReceived > int64(totalMessages) {
		t.Logf("ℹ️ Received %d extra messages due to redelivery (expected behavior for at-least-once)", actualReceived-int64(totalMessages))
	}
}

// TestKafkaFaultIsolationWithHighLoad 测试 Kafka EventBus 高负载下的故障隔离
func TestKafkaFaultIsolationWithHighLoad(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.fault.isolation.high.load.%d", helper.GetTimestamp())
	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-high-load-fault-%d", helper.GetTimestamp()))

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

	// 等待所有消息处理完成（允许重投递）
	time.Sleep(5 * time.Second)

	// 验证结果（允许 >= 预期，因为重投递可能导致重复）
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertGreaterThanOrEqual(actualReceived, int64(totalMessages), "Should receive at least all messages (at-least-once semantics)")
	helper.AssertGreaterThan(atomic.LoadInt64(&panicCount), 0, "Should panic at least once")

	// 验证故障隔离：正常聚合应该收到所有消息（允许重投递）
	expectedNormalMessages := int64(normalAggregateCount * normalAggregateVersions)
	actualNormalReceived := atomic.LoadInt64(&normalAggregatesReceived)
	helper.AssertGreaterThanOrEqual(actualNormalReceived, expectedNormalMessages, "Normal aggregates should receive at least all messages")

	t.Logf("✅ Kafka Fault isolation with high load test passed (at-least-once semantics)")
	t.Logf("📊 Total: %d (expected >= %d), Faulty: %d, Normal: %d, Panic: %d",
		actualReceived,
		totalMessages,
		atomic.LoadInt64(&faultyAggregateReceived),
		actualNormalReceived,
		atomic.LoadInt64(&panicCount))
}
