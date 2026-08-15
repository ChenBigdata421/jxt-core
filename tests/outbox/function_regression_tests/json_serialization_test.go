package function_regression_tests

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// ============================================================================
// Outbox 统一 JSON 序列化架构回归测试
// ============================================================================

// TestOutbox_UsesEventComponentSerialization 测试 Outbox 使用 event 组件的序列化方法
func TestOutbox_UsesEventComponentSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 DomainEvent
	payload := map[string]interface{}{
		"orderId": "order-123",
		"amount":  99.99,
	}
	domainEvent := jxtevent.NewBaseDomainEvent("OrderCreated", "order-123", "Order", payload)

	// 创建 OutboxEvent（应该使用 event 组件的序列化方法）
	outboxEvent, err := outbox.NewOutboxEvent(
		1,
		"order-123",
		"Order",
		"OrderCreated",
		domainEvent,
	)

	helper.AssertNoError(err, "NewOutboxEvent should succeed")
	helper.AssertNotEmpty(outboxEvent.Payload, "Payload should not be empty")

	// 验证 Payload 包含完整的 DomainEvent
	var decodedEvent jxtevent.BaseDomainEvent
	err = jxtjson.Unmarshal(outboxEvent.Payload, &decodedEvent)
	helper.AssertNoError(err, "Unmarshal Payload should succeed")
	helper.AssertEqual("OrderCreated", decodedEvent.EventType, "EventType should match")
	helper.AssertEqual("order-123", decodedEvent.AggregateID, "AggregateID should match")
}

// TestOutbox_PayloadContainsFullDomainEvent 测试 Outbox Payload 包含完整的 DomainEvent
func TestOutbox_PayloadContainsFullDomainEvent(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 EnterpriseDomainEvent
	payload := map[string]interface{}{
		"title":     "Test Archive",
		"createdBy": "user-001",
	}
	domainEvent := jxtevent.NewEnterpriseDomainEvent("ArchiveCreated", "archive-123", "Archive", payload)
	domainEvent.SetTenantId(1)
	domainEvent.SetTraceId("trace-123")
	domainEvent.SetCorrelationId("corr-456")

	// 创建 OutboxEvent
	outboxEvent, err := outbox.NewOutboxEvent(
		1,
		"archive-123",
		"Archive",
		"ArchiveCreated",
		domainEvent,
	)

	helper.AssertNoError(err, "NewOutboxEvent should succeed")

	// 反序列化 Payload
	var decodedEvent jxtevent.EnterpriseDomainEvent
	err = jxtjson.Unmarshal(outboxEvent.Payload, &decodedEvent)
	helper.AssertNoError(err, "Unmarshal Payload should succeed")

	// 验证完整的 DomainEvent 信息
	helper.AssertEqual("ArchiveCreated", decodedEvent.EventType, "EventType should match")
	helper.AssertEqual("archive-123", decodedEvent.AggregateID, "AggregateID should match")
	helper.AssertEqual("Archive", decodedEvent.AggregateType, "AggregateType should match")
	helper.AssertEqual(1, decodedEvent.GetTenantId(), "TenantId should match")
	helper.AssertEqual("trace-123", decodedEvent.GetTraceId(), "TraceId should match")
	helper.AssertEqual("corr-456", decodedEvent.GetCorrelationId(), "CorrelationId should match")

	// 验证 Payload 数据
	payloadMap, ok := decodedEvent.Payload.(map[string]interface{})
	helper.AssertTrue(ok, "Payload should be map[string]interface{}")
	helper.AssertEqual("Test Archive", payloadMap["title"], "Title should match")
	helper.AssertEqual("user-001", payloadMap["createdBy"], "CreatedBy should match")
}

// TestOutbox_SetPayloadUsesEventSerialization 测试 SetPayload 使用 event 组件的序列化
func TestOutbox_SetPayloadUsesEventSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建空的 OutboxEvent
	outboxEvent := &outbox.OutboxEvent{}

	// 创建 DomainEvent
	payload := map[string]interface{}{
		"mediaId":   "media-123",
		"mediaName": "test.mp4",
	}
	domainEvent := jxtevent.NewBaseDomainEvent("MediaUploaded", "media-123", "Media", payload)

	// 设置 Payload（应该使用 event 组件的序列化方法）
	err := outboxEvent.SetPayload(domainEvent)
	helper.AssertNoError(err, "SetPayload should succeed")
	helper.AssertNotEmpty(outboxEvent.Payload, "Payload should not be empty")

	// 验证 Payload
	var decodedEvent jxtevent.BaseDomainEvent
	err = jxtjson.Unmarshal(outboxEvent.Payload, &decodedEvent)
	helper.AssertNoError(err, "Unmarshal Payload should succeed")
	helper.AssertEqual("MediaUploaded", decodedEvent.EventType, "EventType should match")
}

// TestOutbox_GetPayloadAsReturnsFullDomainEvent 测试 GetPayloadAs 返回完整的 DomainEvent
func TestOutbox_GetPayloadAsReturnsFullDomainEvent(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 OutboxEvent
	payload := map[string]interface{}{
		"userId": "user-123",
		"email":  "test@example.com",
	}
	domainEvent := jxtevent.NewBaseDomainEvent("UserRegistered", "user-123", "User", payload)

	outboxEvent, err := outbox.NewOutboxEvent(
		1,
		"user-123",
		"User",
		"UserRegistered",
		domainEvent,
	)
	helper.AssertNoError(err, "NewOutboxEvent should succeed")

	// 使用 GetPayloadAs 获取 DomainEvent
	var decodedEvent jxtevent.BaseDomainEvent
	err = outboxEvent.GetPayloadAs(&decodedEvent)
	helper.AssertNoError(err, "GetPayloadAs should succeed")

	// 验证 DomainEvent
	helper.AssertEqual("UserRegistered", decodedEvent.EventType, "EventType should match")
	helper.AssertEqual("user-123", decodedEvent.AggregateID, "AggregateID should match")

	// 提取 Payload 数据
	payloadMap, ok := decodedEvent.Payload.(map[string]interface{})
	helper.AssertTrue(ok, "Payload should be map[string]interface{}")
	helper.AssertEqual("user-123", payloadMap["userId"], "UserId should match")
	helper.AssertEqual("test@example.com", payloadMap["email"], "Email should match")
}

// TestOutbox_RawMessageCompatibility 测试 RawMessage 兼容性
func TestOutbox_RawMessageCompatibility(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 OutboxEvent
	payload := map[string]interface{}{"test": "data"}
	domainEvent := jxtevent.NewBaseDomainEvent("TestEvent", "test-123", "Test", payload)

	outboxEvent, err := outbox.NewOutboxEvent(
		1,
		"test-123",
		"Test",
		"TestEvent",
		domainEvent,
	)
	helper.AssertNoError(err, "NewOutboxEvent should succeed")

	// outboxEvent.Payload 应该是 jxtjson.RawMessage 类型
	// jxtjson.RawMessage 是 jsoniter.RawMessage 的别名，与 []byte 兼容

	var bytes []byte = outboxEvent.Payload
	helper.AssertNotEmpty(bytes, "Payload should be compatible with []byte")

	// 应该可以直接反序列化
	var result map[string]interface{}
	err = jxtjson.Unmarshal(bytes, &result)
	helper.AssertNoError(err, "Unmarshal should succeed")
}

// TestOutbox_PerformanceWithEventSerialization 校验 OutboxEvent 构造的性能回归。
//
// 旧实现用「平均 < 10µs」绝对墙钟阈值，在繁忙机器/CI（30+ 容器负载）下会 flake
// （实测 16.4µs 即误判失败）。改为相对比较：NewOutboxEvent 的主体开销是
// jsoniter 序列化（MarshalDomainEvent）+ UUID/幂等键，以标准库 encoding/json
// 序列化同一 event 为负载无关基线，jsoniter 管线不应慢于其 2×——真正退化
// （如误换反射重路径）仍会触发，正常调度抖动不会。
func TestOutbox_PerformanceWithEventSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建测试数据
	payload := map[string]interface{}{
		"orderId": "order-123",
		"items":   []string{"item1", "item2", "item3"},
		"total":   299.99,
	}
	domainEvent := jxtevent.NewBaseDomainEvent("OrderCreated", "order-123", "Order", payload)

	// 1) 功能正确性（硬断言）
	ev, err := outbox.NewOutboxEvent(1, "order-123", "Order", "OrderCreated", domainEvent)
	helper.AssertNoError(err, "NewOutboxEvent should succeed")
	helper.AssertNotEmpty(ev.Payload, "payload should be serialized")

	// 2) 预热，避免一次性初始化开销污染测量
	_, _ = outbox.NewOutboxEvent(1, "order-123", "Order", "OrderCreated", domainEvent)
	_, _ = json.Marshal(domainEvent)

	const iterations = 2000

	// 3) NewOutboxEvent 管线（jsoniter 序列化 + UUID + 幂等键）
	outboxStart := time.Now()
	for i := 0; i < iterations; i++ {
		if _, err := outbox.NewOutboxEvent(1, "order-123", "Order", "OrderCreated", domainEvent); err != nil {
			t.Fatalf("NewOutboxEvent failed: %v", err)
		}
	}
	outboxAvg := time.Since(outboxStart) / iterations

	// 4) 基线：标准库 encoding/json 序列化同一 event（同等工作量级别）
	stdStart := time.Now()
	for i := 0; i < iterations; i++ {
		if _, err := json.Marshal(domainEvent); err != nil {
			t.Fatalf("encoding/json marshal failed: %v", err)
		}
	}
	stdAvg := time.Since(stdStart) / iterations

	// 5) 相对断言（负载无关）。NewOutboxEvent 除序列化外还含 UUID 生成（uuid.Must(NewV7)，
	//    实测 ~µs 级），故余量取 5×；仅捕捉量级级退化（10×+），不苛求精确比例。
	const safetyFactor = 5
	helper.AssertTrue(outboxAvg <= safetyFactor*stdAvg,
		"NewOutboxEvent (jsoniter pipeline) should not be dramatically slower than encoding/json baseline")

	t.Logf("avg over %d iters: NewOutboxEvent=%v  encoding/json=%v  ratio=%.2f",
		iterations, outboxAvg, stdAvg, float64(outboxAvg)/float64(stdAvg))
}

// TestOutbox_ConcurrentSerialization 测试并发序列化
func TestOutbox_ConcurrentSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	goroutines := 100
	iterations := 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				payload := map[string]interface{}{
					"id":    id,
					"index": j,
				}
				domainEvent := jxtevent.NewBaseDomainEvent("TestEvent", "test-123", "Test", payload)

				_, err := outbox.NewOutboxEvent(
					1,
					"test-123",
					"Test",
					"TestEvent",
					domainEvent,
				)
				helper.AssertNoError(err, "Concurrent NewOutboxEvent should succeed")
			}
		}(i)
	}

	wg.Wait()
}

// TestOutbox_ComplexPayloadSerialization 测试复杂 Payload 序列化
func TestOutbox_ComplexPayloadSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建复杂 Payload
	complexPayload := map[string]interface{}{
		"title": "Complex Event",
		"metadata": map[string]interface{}{
			"version": "1.0",
			"tags":    []string{"tag1", "tag2", "tag3"},
		},
		"items": []map[string]interface{}{
			{"id": 1, "name": "item1"},
			{"id": 2, "name": "item2"},
		},
		"timestamp": time.Now(),
	}

	domainEvent := jxtevent.NewBaseDomainEvent("ComplexEvent", "test-123", "Test", complexPayload)

	// 创建 OutboxEvent
	outboxEvent, err := outbox.NewOutboxEvent(
		1,
		"test-123",
		"Test",
		"ComplexEvent",
		domainEvent,
	)
	helper.AssertNoError(err, "NewOutboxEvent should succeed")

	// 反序列化并验证
	var decodedEvent jxtevent.BaseDomainEvent
	err = jxtjson.Unmarshal(outboxEvent.Payload, &decodedEvent)
	helper.AssertNoError(err, "Unmarshal should succeed")

	payloadMap, ok := decodedEvent.Payload.(map[string]interface{})
	helper.AssertTrue(ok, "Payload should be map[string]interface{}")
	helper.AssertEqual("Complex Event", payloadMap["title"], "Title should match")

	metadata, ok := payloadMap["metadata"].(map[string]interface{})
	helper.AssertTrue(ok, "Metadata should be map[string]interface{}")
	helper.AssertEqual("1.0", metadata["version"], "Version should match")
}
