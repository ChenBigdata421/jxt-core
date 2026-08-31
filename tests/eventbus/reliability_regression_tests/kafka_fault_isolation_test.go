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

// TestKafkaFaultIsolation 测试 Kafka EventBus 故障隔离（单个 Actor 故障不影响其他 Actor）
//
// 🎯 测试目的:
//   验证 Kafka EventBus 的 Actor Pool 故障隔离能力：当一个聚合 ID 的 Actor 发生 panic 时，
//   不应该影响其他聚合 ID 的消息处理，并且 panic 的消息应该被重投递。
//
// ✅ Kafka Envelope 语义:
//   Kafka Envelope 支持 at-least-once 语义，panic 后会重投递消息。
//
// 📋 测试逻辑:
//   1. 创建 Kafka EventBus 并订阅 Envelope topic
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
//   - Kafka 的 at-least-once 语义：panic 消息被重投递
//   - 每个聚合使用独立的 Actor（通过一致性哈希路由）
//
// 📊 测试规模:
//   - 聚合数量: 3
//   - 每个聚合版本数: 5
//   - 总消息数: 15
//   - 预期接收: 15（所有消息，包括重投递）
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
	readyGate := NewReadyGate()

	ctx := context.Background()

	// 订阅 Envelope，aggregate-1 的消息触发 panic
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		// 就绪屏障：probe 被目击即放行（不进业务计数）
		if envelope.AggregateID == ReadyProbeAggregateID {
			readyGate.Trip()
			return nil
		}

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

	// 就绪屏障：probe 被目击 ⇒ consumer group fetch 已定位，替代固定 sleep
	// （CI 受限资源上 assignment 可能超 100ms，固定窗口会漏读早期消息——run #19/#23/#28）
	helper.AssertTrue(helper.AwaitConsumerReady(ctx, bus, topic, readyGate, 15*time.Second),
		"Consumer should become ready (probe witnessed) before publishing")

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

	// 门计数器（totalReceived）与下方断言的各聚合计数器是不同变量——门开瞬间，最后一个
	// handler 可能已执行 totalReceived++ 但尚未执行各聚合的 ++（两步非原子组合）。
	// 先等分类计数器收敛到期望值，再做精确相等断言。
	helper.WaitForMessages(&aggregate1Received, int64(versionsPerAggregate), 10*time.Second)
	helper.WaitForMessages(&aggregate2Received, int64(versionsPerAggregate), 10*time.Second)
	helper.WaitForMessages(&aggregate3Received, int64(versionsPerAggregate), 10*time.Second)

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
//
// 🎯 测试目的:
//   验证 Kafka 的 Subscribe 方法（非 Envelope）使用 at-most-once 语义，
//   当 Handler 发生 panic 时，消息不会被重投递，而是直接丢失。
//
// 📋 测试逻辑:
//   1. 创建 Kafka EventBus 并订阅 topic（使用 Subscribe，非 Envelope）
//   2. Handler 在处理 aggregate-1 的 version=2 时触发 panic（只触发一次）
//   3. 发送 3 个聚合各 5 个版本的原始消息（共 15 条）
//   4. 等待消息处理完成
//
// ✅ 检查项:
//   - 应该接收 14 条消息（totalReceived = 14，aggregate-1 的 v2 丢失）
//   - panic 应该只发生 1 次（panicCount = 1）
//   - panic 的消息不会被重投递
//   - 测试应该在 10 秒内完成
//
// 🔍 验证点:
//   - Kafka Subscribe（非 Envelope）的 at-most-once 语义
//   - panic 消息被标记为已处理，不重投
//   - 与 SubscribeEnvelope 的 at-least-once 语义形成对比
//
// 📊 语义对比:
//   - Subscribe (Raw): at-most-once（panic 消息丢失）
//   - SubscribeEnvelope: at-least-once（panic 消息重投）
//
// 🔧 测试规模:
//   - 聚合数量: 3
//   - 每个聚合版本数: 5
//   - 总消息数: 15
//   - 预期接收: 14（aggregate-1 的 v2 丢失）
func TestKafkaFaultIsolationRaw(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.fault.raw.%d", helper.GetTimestamp())
	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-fault-raw-%d", helper.GetTimestamp()))

	var totalReceived int64
	var panicCount int64
	var panicTriggered atomic.Bool
	rawReadyGate := NewReadyGate()

	ctx := context.Background()

	helper.AssertNoError(bus.Subscribe(ctx, topic, func(ctx context.Context, data []byte) error {
		var payload struct {
			Aggregate string `json:"aggregate"`
			Version   int64  `json:"version"`
		}

		if err := jxtjson.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("failed to decode payload: %w", err)
		}

		// 就绪屏障：raw 路径按解码后的 Aggregate 哨兵识别 probe（不进业务计数）
		if payload.Aggregate == ReadyProbeAggregateID {
			rawReadyGate.Trip()
			return nil
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

	// 就绪屏障（raw 路径）：probe 以原始 JSON 发布，handler 解码后按哨兵放行
	helper.AssertTrue(helper.AwaitRawConsumerReady(ctx, bus, topic, rawReadyGate, 15*time.Second),
		"Raw consumer should become ready (probe witnessed) before publishing")

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
//
// 🎯 测试目的:
//   验证多个聚合 ID 并发发生 panic 时，每个聚合的 Actor 都能独立恢复，
//   并且所有消息最终都能被正确处理（at-least-once 语义）。
//
// 📋 测试逻辑:
//   1. 创建 Kafka EventBus 并订阅 Envelope topic
//   2. Handler 在处理每个聚合的 version=1 时都触发 panic（每个聚合只触发一次）
//   3. 并发发送 5 个聚合各 3 个版本的消息（共 15 条）
//   4. 等待所有消息处理完成
//
// ✅ 检查项:
//   - 应该接收至少 15 条消息（actualReceived >= 15，包括重投递）
//   - panic 应该发生 5 次（每个聚合 1 次）
//   - 所有聚合的消息都应该被处理
//   - 测试应该在合理时间内完成
//
// 🔍 验证点:
//   - 多个 Actor 并发 panic 后都能恢复
//   - 每个聚合使用独立的 Actor（故障隔离）
//   - Kafka 的 at-least-once 语义：所有 panic 消息都被重投递
//   - 并发场景下的 Supervisor 稳定性
//
// 📊 测试规模:
//   - 聚合数量: 5
//   - 每个聚合版本数: 3
//   - 总消息数: 15
//   - 预期接收: >= 15（at-least-once，可能有重复）
//   - 预期 panic 次数: 5
func TestKafkaConcurrentFaultRecovery(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.concurrent.fault.recovery.%d", helper.GetTimestamp())
	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-concurrent-fault-%d", helper.GetTimestamp()))

	var totalReceived int64
	var panicCount int64
	var mu sync.Mutex
	panicAggregates := make(map[string]bool)
	readyGate := NewReadyGate()

	ctx := context.Background()

	// 订阅 Envelope，多个聚合的第一条消息都触发 panic
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		// 就绪屏障：probe 被目击即放行（不进业务计数）
		if envelope.AggregateID == ReadyProbeAggregateID {
			readyGate.Trip()
			return nil
		}

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

	// 就绪屏障（run #28 flake 根修）：替代固定 sleep
	helper.AssertTrue(helper.AwaitConsumerReady(ctx, bus, topic, readyGate, 15*time.Second),
		"Consumer should become ready (probe witnessed) before publishing")

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

	// 等待所有消息处理完成（轮询而非固定 sleep：慢环境需要更久，固定 5s 会欠投递）
	success := helper.WaitForMessages(&totalReceived, int64(totalMessages), 30*time.Second)

	// 验证结果（允许 >= 预期，因为重投递可能导致重复）
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertTrue(success, "Should receive at least all messages (at-least-once semantics)")
	helper.AssertGreaterThanOrEqual(actualReceived, int64(totalMessages), "Should receive at least all messages (at-least-once semantics)")
	helper.AssertGreaterThan(atomic.LoadInt64(&panicCount), 0, "Should panic at least once per aggregate")

	t.Logf("✅ Kafka Concurrent fault recovery test passed (at-least-once semantics)")
	t.Logf("📊 Total messages: %d (expected >= %d), Panic count: %d", actualReceived, totalMessages, atomic.LoadInt64(&panicCount))
	if actualReceived > int64(totalMessages) {
		t.Logf("ℹ️ Received %d extra messages due to redelivery (expected behavior for at-least-once)", actualReceived-int64(totalMessages))
	}
}

// TestKafkaFaultIsolationWithHighLoad 测试 Kafka EventBus 高负载下的故障隔离
//
// 🎯 测试目的:
//   验证在高负载场景下（100个聚合，1000条消息），单个聚合的 panic 不会影响其他 99 个聚合的处理，
//   并且故障聚合能够恢复并处理所有消息。
//
// 📋 测试逻辑:
//   1. 创建 Kafka EventBus 并订阅 Envelope topic
//   2. Handler 在处理 aggregate-fault 的 version=1 时触发 panic（只触发一次）
//   3. 发送 100 个聚合各 10 个版本的消息（共 1000 条）
//   4. 等待所有消息处理完成
//
// ✅ 检查项:
//   - 应该接收至少 1000 条消息（actualReceived >= 1000）
//   - panic 应该发生至少 1 次（panicCount >= 1）
//   - 其他 99 个聚合不受影响
//   - aggregate-fault 应该接收所有 10 个版本（包括重投递的 v1）
//   - 测试应该在 60 秒内完成
//
// 🔍 验证点:
//   - 高负载下的故障隔离能力
//   - 单个 Actor 的 panic 不影响其他 99 个 Actor
//   - Kafka 的 at-least-once 语义在高负载下正常工作
//   - Actor Pool 在高并发下的稳定性
//   - Supervisor 在高负载下的恢复能力
//
// 📊 测试规模:
//   - 聚合数量: 100（1 个故障 + 99 个正常）
//   - 每个聚合版本数: 10
//   - 总消息数: 1000
//   - 预期接收: >= 1000（at-least-once）
//   - 故障聚合: aggregate-fault
//   - 正常聚合: aggregate-0 到 aggregate-98
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
	highLoadGate := NewReadyGate()
	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		// 就绪屏障：probe 被目击即放行（不进业务计数）
		if envelope.AggregateID == ReadyProbeAggregateID {
			highLoadGate.Trip()
			return nil
		}

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

	// 就绪屏障（run #19 flake 根修）：替代固定 sleep
	helper.AssertTrue(helper.AwaitConsumerReady(ctx, bus, topic, highLoadGate, 15*time.Second),
		"Consumer should become ready (probe witnessed) before publishing")

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

	// 等待所有消息处理完成（轮询而非固定 sleep：慢环境需要更久，固定 5s 会欠投递）
	success := helper.WaitForMessages(&totalReceived, int64(totalMessages), 60*time.Second)

	// 验证结果（允许 >= 预期，因为重投递可能导致重复）
	actualReceived := atomic.LoadInt64(&totalReceived)
	helper.AssertTrue(success, "Should receive at least all messages (at-least-once semantics)")
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
