package function_tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
)

// ============================================================================
// EventBus 统一 JSON 序列化架构回归测试
// ============================================================================

// TestEventBus_EnvelopeUsesUnifiedJSON 测试 Envelope 使用统一的 JSON 包
func TestEventBus_EnvelopeUsesUnifiedJSON(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Envelope
	payload := map[string]interface{}{
		"orderId": "order-123",
		"amount":  99.99,
	}
	payloadBytes, err := jxtjson.Marshal(payload)
	helper.AssertNoError(err, "Marshal payload should succeed")

	envelope := &eventbus.Envelope{
		EventID:      "event-123",
		AggregateID:  "order-123",
		EventType:    "OrderCreated",
		EventVersion: 1,
		Payload:      jxtjson.RawMessage(payloadBytes),
		Timestamp:    time.Now(),
	}

	// 序列化 Envelope（应该使用 jxtjson）
	bytes, err := envelope.ToBytes()
	helper.AssertNoError(err, "ToBytes should succeed")
	helper.AssertNotEmpty(bytes, "ToBytes should return non-empty bytes")

	// 反序列化 Envelope
	result, err := eventbus.FromBytes(bytes)
	helper.AssertNoError(err, "FromBytes should succeed")
	helper.AssertEqual("event-123", result.EventID, "EventID should match")
	helper.AssertEqual("order-123", result.AggregateID, "AggregateID should match")
}

// TestEventBus_RawMessageCompatibility 测试 RawMessage 兼容性
func TestEventBus_RawMessageCompatibility(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 RawMessage
	raw := jxtjson.RawMessage(`{"test":"data"}`)

	// 应该可以用于 Envelope.Payload
	envelope := &eventbus.Envelope{
		EventID:      "event-123",
		AggregateID:  "test-123",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      raw,
		Timestamp:    time.Now(),
	}

	helper.AssertNotEmpty(envelope.Payload, "Payload should not be empty")

	// 序列化和反序列化
	bytes, err := envelope.ToBytes()
	helper.AssertNoError(err, "ToBytes should succeed")

	result, err := eventbus.FromBytes(bytes)
	helper.AssertNoError(err, "FromBytes should succeed")

	// 验证 Payload
	var payloadMap map[string]string
	err = jxtjson.Unmarshal(result.Payload, &payloadMap)
	helper.AssertNoError(err, "Unmarshal Payload should succeed")
	helper.AssertEqual("data", payloadMap["test"], "Payload data should match")
}

// TestEventBus_EnvelopeValidation 测试 Envelope 验证
func TestEventBus_EnvelopeValidation(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建有效的 Envelope
	payload := jxtjson.RawMessage(`{"test":"data"}`)
	envelope := &eventbus.Envelope{
		EventID:      "event-123",
		AggregateID:  "test-123",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      payload,
		Timestamp:    time.Now(),
	}

	// 验证
	err := envelope.Validate()
	helper.AssertNoError(err, "Valid envelope should pass validation")

	// 测试无效的 Envelope（缺少 AggregateID）
	invalidEnvelope := &eventbus.Envelope{
		EventID:      "event-123",
		AggregateID:  "",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      payload,
		Timestamp:    time.Now(),
	}

	err = invalidEnvelope.Validate()
	helper.AssertError(err, "Invalid envelope should fail validation")
}

// TestEventBus_PublishEnvelopeWithJSON 测试使用 JSON 发布 Envelope
func TestEventBus_PublishEnvelopeWithJSON(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateMemoryEventBus()
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	topic := "test.events"

	// 订阅
	var receivedEnvelope *eventbus.Envelope
	handler := func(ctx context.Context, env *eventbus.Envelope) error {
		receivedEnvelope = env
		return nil
	}

	err := bus.SubscribeEnvelope(ctx, topic, handler)
	helper.AssertNoError(err, "SubscribeEnvelope should succeed")

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 创建 Envelope
	payload := map[string]interface{}{
		"message": "Hello, World!",
		"count":   42,
	}
	payloadBytes, err := jxtjson.Marshal(payload)
	helper.AssertNoError(err, "Marshal payload should succeed")

	envelope := &eventbus.Envelope{
		EventID:      "event-123",
		AggregateID:  "test-123",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      jxtjson.RawMessage(payloadBytes),
		Timestamp:    time.Now(),
	}

	// 发布
	err = bus.PublishEnvelope(ctx, topic, envelope)
	helper.AssertNoError(err, "PublishEnvelope should succeed")

	// 等待接收
	time.Sleep(200 * time.Millisecond)

	// 验证接收到的 Envelope
	helper.AssertNotNil(receivedEnvelope, "Should receive envelope")
	helper.AssertEqual("event-123", receivedEnvelope.EventID, "EventID should match")

	// 验证 Payload
	var receivedPayload map[string]interface{}
	err = jxtjson.Unmarshal(receivedEnvelope.Payload, &receivedPayload)
	helper.AssertNoError(err, "Unmarshal received payload should succeed")
	helper.AssertEqual("Hello, World!", receivedPayload["message"], "Message should match")
	helper.AssertEqual(float64(42), receivedPayload["count"], "Count should match")
}

// TestEventBus_HealthCheckMessageUsesJSON 测试 HealthCheckMessage 使用 JSON
func TestEventBus_HealthCheckMessageUsesJSON(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 HealthCheckMessage
	msg := eventbus.CreateHealthCheckMessage("test-service", "memory")

	// 序列化（应该使用 jxtjson）
	bytes, err := msg.ToBytes()
	helper.AssertNoError(err, "ToBytes should succeed")
	helper.AssertNotEmpty(bytes, "ToBytes should return non-empty bytes")

	// 反序列化
	var result eventbus.HealthCheckMessage
	err = result.FromBytes(bytes)
	helper.AssertNoError(err, "FromBytes should succeed")
	helper.AssertEqual("test-service", result.Source, "Source should match")
	helper.AssertEqual("memory", result.EventBusType, "EventBusType should match")
}

// TestEventBus_PerformanceWithJSON 测试使用 JSON 的性能
func TestEventBus_PerformanceWithJSON(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建测试数据
	payload := map[string]interface{}{
		"orderId": "order-123",
		"items":   []string{"item1", "item2", "item3"},
		"total":   299.99,
	}
	payloadBytes, err := jxtjson.Marshal(payload)
	helper.AssertNoError(err, "Marshal payload should succeed")

	envelope := &eventbus.Envelope{
		EventID:      "event-123",
		AggregateID:  "order-123",
		EventType:    "OrderCreated",
		EventVersion: 1,
		Payload:      jxtjson.RawMessage(payloadBytes),
		Timestamp:    time.Now(),
	}

	// 测试序列化性能
	iterations := 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		_, err := envelope.ToBytes()
		helper.AssertNoError(err, "ToBytes should succeed")
	}

	duration := time.Since(start)
	avgDuration := duration / time.Duration(iterations)

	// 验证性能（平均每次应该小于 10 微秒）
	helper.AssertTrue(avgDuration < 10*time.Microsecond, "Average serialization should be fast")

	t.Logf("Average Envelope serialization time: %v", avgDuration)
}

// TestEventBus_ConcurrentEnvelopeSerialization 测试并发 Envelope 序列化
func TestEventBus_ConcurrentEnvelopeSerialization(t *testing.T) {
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
				payloadBytes, err := jxtjson.Marshal(payload)
				helper.AssertNoError(err, "Marshal payload should succeed")

				envelope := &eventbus.Envelope{
					EventID:      "event-123",
					AggregateID:  "test-123",
					EventType:    "TestEvent",
					EventVersion: 1,
					Payload:      jxtjson.RawMessage(payloadBytes),
					Timestamp:    time.Now(),
				}

				bytes, err := envelope.ToBytes()
				helper.AssertNoError(err, "Concurrent ToBytes should succeed")
				helper.AssertNotEmpty(bytes, "Concurrent ToBytes should return non-empty bytes")

				result, err := eventbus.FromBytes(bytes)
				helper.AssertNoError(err, "Concurrent FromBytes should succeed")
				helper.AssertEqual("event-123", result.EventID, "EventID should match")
			}
		}(i)
	}

	wg.Wait()
}

// TestEventBus_ComplexPayloadSerialization 测试复杂 Payload 序列化
func TestEventBus_ComplexPayloadSerialization(t *testing.T) {
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

	payloadBytes, err := jxtjson.Marshal(complexPayload)
	helper.AssertNoError(err, "Marshal complex payload should succeed")

	envelope := &eventbus.Envelope{
		EventID:      "event-123",
		AggregateID:  "test-123",
		EventType:    "ComplexEvent",
		EventVersion: 1,
		Payload:      jxtjson.RawMessage(payloadBytes),
		Timestamp:    time.Now(),
	}

	// 序列化和反序列化
	bytes, err := envelope.ToBytes()
	helper.AssertNoError(err, "ToBytes should succeed")

	result, err := eventbus.FromBytes(bytes)
	helper.AssertNoError(err, "FromBytes should succeed")

	// 验证复杂 Payload
	var resultPayload map[string]interface{}
	err = jxtjson.Unmarshal(result.Payload, &resultPayload)
	helper.AssertNoError(err, "Unmarshal complex payload should succeed")
	helper.AssertEqual("Complex Event", resultPayload["title"], "Title should match")

	metadata, ok := resultPayload["metadata"].(map[string]interface{})
	helper.AssertTrue(ok, "Metadata should be map[string]interface{}")
	helper.AssertEqual("1.0", metadata["version"], "Version should match")
}
