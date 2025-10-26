package function_regression_tests

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
)

// ============================================================================
// EnterpriseDomainEvent 序列化/反序列化专项测试
// ============================================================================

// TestEnterpriseDomainEvent_BasicSerialization 测试基本序列化
func TestEnterpriseDomainEvent_BasicSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件
	payload := helper.CreateTestPayload()
	event := helper.CreateEnterpriseDomainEvent("Media.Uploaded", "media-123", "Media", payload)
	event.SetTenantId("tenant-001")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")
	helper.AssertNotEmpty(bytes, "Serialized bytes should not be empty")

	// 验证 JSON 格式
	var jsonMap map[string]interface{}
	err = json.Unmarshal(bytes, &jsonMap)
	helper.AssertNoError(err, "Should be valid JSON")
	helper.AssertEqual("Media.Uploaded", jsonMap["eventType"], "EventType should be serialized")
	helper.AssertEqual("media-123", jsonMap["aggregateId"], "AggregateId should be serialized")
	helper.AssertEqual("tenant-001", jsonMap["tenantId"], "TenantId should be serialized")
}

// TestEnterpriseDomainEvent_BasicDeserialization 测试基本反序列化
func TestEnterpriseDomainEvent_BasicDeserialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建并序列化事件
	payload := helper.CreateTestPayload()
	originalEvent := helper.CreateEnterpriseDomainEvent("Media.Uploaded", "media-123", "Media", payload)
	originalEvent.SetTenantId("tenant-001")

	bytes, err := jxtevent.MarshalDomainEvent(originalEvent)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 验证基本字段
	helper.AssertEqual(originalEvent.GetEventID(), deserializedEvent.GetEventID(), "EventID should match")
	helper.AssertEqual(originalEvent.GetEventType(), deserializedEvent.GetEventType(), "EventType should match")
	helper.AssertEqual(originalEvent.GetAggregateID(), deserializedEvent.GetAggregateID(), "AggregateID should match")
	helper.AssertEqual(originalEvent.GetAggregateType(), deserializedEvent.GetAggregateType(), "AggregateType should match")
	helper.AssertEqual(originalEvent.GetTenantId(), deserializedEvent.GetTenantId(), "TenantId should match")
}

// TestEnterpriseDomainEvent_AllEnterpriseFieldsSerialization 测试所有企业级字段序列化
func TestEnterpriseDomainEvent_AllEnterpriseFieldsSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件并设置所有企业级字段
	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-123", "Order", nil)
	event.SetTenantId("tenant-acme")
	event.SetCorrelationId("correlation-xyz")
	event.SetCausationId("causation-abc")
	event.SetTraceId("trace-def")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 验证所有企业级字段
	helper.AssertEqual("tenant-acme", deserializedEvent.GetTenantId(), "TenantId should match")
	helper.AssertEqual("correlation-xyz", deserializedEvent.GetCorrelationId(), "CorrelationId should match")
	helper.AssertEqual("causation-abc", deserializedEvent.GetCausationId(), "CausationId should match")
	helper.AssertEqual("trace-def", deserializedEvent.GetTraceId(), "TraceId should match")
}

// TestEnterpriseDomainEvent_PayloadSerialization 测试 Payload 序列化
func TestEnterpriseDomainEvent_PayloadSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建带复杂 Payload 的事件
	payload := helper.CreateComplexPayload()
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", payload)
	event.SetTenantId("tenant-001")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 提取并验证 Payload
	extractedPayload, err := jxtevent.UnmarshalPayload[ComplexPayload](deserializedEvent)
	helper.AssertNoError(err, "UnmarshalPayload should succeed")
	helper.AssertEqual(payload.ID, extractedPayload.ID, "Payload ID should match")
	helper.AssertEqual(payload.Name, extractedPayload.Name, "Payload Name should match")
	helper.AssertEqual(len(payload.Tags), len(extractedPayload.Tags), "Payload Tags length should match")
}

// TestEnterpriseDomainEvent_JSONFieldNames 测试 JSON 字段名称
func TestEnterpriseDomainEvent_JSONFieldNames(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId("tenant-001")
	event.SetCorrelationId("corr-123")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	jsonStr := string(bytes)

	// 验证 JSON 字段名称使用驼峰命名（camelCase）
	helper.AssertContains(jsonStr, `"eventId"`, "Should use camelCase for eventId")
	helper.AssertContains(jsonStr, `"eventType"`, "Should use camelCase for eventType")
	helper.AssertContains(jsonStr, `"aggregateId"`, "Should use camelCase for aggregateId")
	helper.AssertContains(jsonStr, `"aggregateType"`, "Should use camelCase for aggregateType")
	helper.AssertContains(jsonStr, `"tenantId"`, "Should use camelCase for tenantId")
	helper.AssertContains(jsonStr, `"correlationId"`, "Should use camelCase for correlationId")
	helper.AssertContains(jsonStr, `"occurredAt"`, "Should use camelCase for occurredAt")
}

// TestEnterpriseDomainEvent_EmptyOptionalFields 测试空的可选字段序列化
func TestEnterpriseDomainEvent_EmptyOptionalFields(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件，不设置可选字段
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId("tenant-001")
	// CorrelationId, CausationId, TraceId 保持为空

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 验证空字段
	helper.AssertEqual("", deserializedEvent.GetCorrelationId(), "CorrelationId should be empty")
	helper.AssertEqual("", deserializedEvent.GetCausationId(), "CausationId should be empty")
	helper.AssertEqual("", deserializedEvent.GetTraceId(), "TraceId should be empty")
}

// TestEnterpriseDomainEvent_OmitEmptyFields 测试 omitempty 标签
func TestEnterpriseDomainEvent_OmitEmptyFields(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件，不设置可选字段
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId("tenant-001")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	jsonStr := string(bytes)

	// 验证空的可选字段不出现在 JSON 中（因为有 omitempty 标签）
	helper.AssertFalse(strings.Contains(jsonStr, `"correlationId":`), "Empty correlationId should be omitted")
	helper.AssertFalse(strings.Contains(jsonStr, `"causationId":`), "Empty causationId should be omitted")
	helper.AssertFalse(strings.Contains(jsonStr, `"traceId":`), "Empty traceId should be omitted")
}

// TestEnterpriseDomainEvent_RoundTripSerialization 测试往返序列化
func TestEnterpriseDomainEvent_RoundTripSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建原始事件
	payload := helper.CreateTestPayload()
	originalEvent := helper.CreateEnterpriseDomainEvent("Archive.Created", "archive-123", "Archive", payload)
	originalEvent.SetTenantId("tenant-001")
	originalEvent.SetCorrelationId("workflow-123")
	originalEvent.SetCausationId("trigger-456")
	originalEvent.SetTraceId("trace-789")

	// 第一次序列化
	bytes1, err := jxtevent.MarshalDomainEvent(originalEvent)
	helper.AssertNoError(err, "First serialization should succeed")

	// 第一次反序列化
	event1, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes1)
	helper.AssertNoError(err, "First deserialization should succeed")

	// 第二次序列化
	bytes2, err := jxtevent.MarshalDomainEvent(event1)
	helper.AssertNoError(err, "Second serialization should succeed")

	// 第二次反序列化
	event2, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes2)
	helper.AssertNoError(err, "Second deserialization should succeed")

	// 验证所有字段在往返后保持一致
	helper.AssertEqual(originalEvent.GetEventID(), event2.GetEventID(), "EventID should survive round-trip")
	helper.AssertEqual(originalEvent.GetEventType(), event2.GetEventType(), "EventType should survive round-trip")
	helper.AssertEqual(originalEvent.GetTenantId(), event2.GetTenantId(), "TenantId should survive round-trip")
	helper.AssertEqual(originalEvent.GetCorrelationId(), event2.GetCorrelationId(), "CorrelationId should survive round-trip")
	helper.AssertEqual(originalEvent.GetCausationId(), event2.GetCausationId(), "CausationId should survive round-trip")
	helper.AssertEqual(originalEvent.GetTraceId(), event2.GetTraceId(), "TraceId should survive round-trip")
}

// TestEnterpriseDomainEvent_StringSerialization 测试字符串序列化
func TestEnterpriseDomainEvent_StringSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId("tenant-001")

	// 序列化为字符串
	jsonStr, err := jxtevent.MarshalDomainEventToString(event)
	helper.AssertNoError(err, "MarshalDomainEventToString should succeed")
	helper.AssertNotEmpty(jsonStr, "JSON string should not be empty")

	// 从字符串反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEventFromString[*jxtevent.EnterpriseDomainEvent](jsonStr)
	helper.AssertNoError(err, "UnmarshalDomainEventFromString should succeed")

	// 验证
	helper.AssertEqual(event.GetEventID(), deserializedEvent.GetEventID(), "EventID should match")
	helper.AssertEqual(event.GetTenantId(), deserializedEvent.GetTenantId(), "TenantId should match")
}

// TestEnterpriseDomainEvent_TimestampSerialization 测试时间戳序列化
func TestEnterpriseDomainEvent_TimestampSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	originalTime := event.GetOccurredAt()

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 验证时间戳（允许微秒级误差）
	helper.AssertWithinDuration(originalTime, deserializedEvent.GetOccurredAt(), time.Millisecond, "OccurredAt should match")
}

// TestEnterpriseDomainEvent_NilPayloadSerialization 测试 nil Payload 序列化
func TestEnterpriseDomainEvent_NilPayloadSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建没有 Payload 的事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId("tenant-001")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed with nil payload")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed with nil payload")

	// 验证 Payload 为 nil
	helper.AssertNil(deserializedEvent.GetPayload(), "Payload should be nil")
}

// TestEnterpriseDomainEvent_ConcurrentSerialization 测试并发序列化
func TestEnterpriseDomainEvent_ConcurrentSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	goroutines := 100
	iterations := 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				// 创建事件
				event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
				event.SetTenantId("tenant-001")

				// 序列化
				bytes, err := jxtevent.MarshalDomainEvent(event)
				helper.AssertNoError(err, "Concurrent serialization should succeed")

				// 反序列化
				_, err = jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
				helper.AssertNoError(err, "Concurrent deserialization should succeed")
			}
		}(i)
	}

	wg.Wait()
}

// TestEnterpriseDomainEvent_PerformanceBenchmark 测试序列化性能
func TestEnterpriseDomainEvent_PerformanceBenchmark(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件
	payload := helper.CreateTestPayload()
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", payload)
	event.SetTenantId("tenant-001")
	event.SetCorrelationId("correlation-123")
	event.SetTraceId("trace-456")

	iterations := 10000
	start := time.Now()

	// 测试序列化性能
	for i := 0; i < iterations; i++ {
		_, err := jxtevent.MarshalDomainEvent(event)
		helper.AssertNoError(err, "Serialization should succeed")
	}

	serializationDuration := time.Since(start)
	avgSerializationTime := serializationDuration / time.Duration(iterations)

	t.Logf("Serialization: %d iterations in %v (avg: %v per operation)", iterations, serializationDuration, avgSerializationTime)

	// 序列化一次用于反序列化测试
	bytes, _ := jxtevent.MarshalDomainEvent(event)

	start = time.Now()

	// 测试反序列化性能
	for i := 0; i < iterations; i++ {
		_, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
		helper.AssertNoError(err, "Deserialization should succeed")
	}

	deserializationDuration := time.Since(start)
	avgDeserializationTime := deserializationDuration / time.Duration(iterations)

	t.Logf("Deserialization: %d iterations in %v (avg: %v per operation)", iterations, deserializationDuration, avgDeserializationTime)

	// 验证性能（平均每次应该小于 10 微秒）
	helper.AssertTrue(avgSerializationTime < 10*time.Microsecond, "Average serialization should be fast")
	helper.AssertTrue(avgDeserializationTime < 10*time.Microsecond, "Average deserialization should be fast")
}

// TestEnterpriseDomainEvent_LargePayloadSerialization 测试大 Payload 序列化
func TestEnterpriseDomainEvent_LargePayloadSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建大 Payload
	largePayload := make(map[string]interface{})
	for i := 0; i < 1000; i++ {
		largePayload["field_"+string(rune(i))] = map[string]interface{}{
			"id":    i,
			"name":  "item-" + string(rune(i)),
			"value": i * 2,
		}
	}

	// 创建事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", largePayload)
	event.SetTenantId("tenant-001")

	// 序列化
	start := time.Now()
	bytes, err := jxtevent.MarshalDomainEvent(event)
	serializationTime := time.Since(start)

	helper.AssertNoError(err, "Large payload serialization should succeed")
	helper.AssertNotEmpty(bytes, "Serialized bytes should not be empty")

	t.Logf("Large payload serialization time: %v (size: %d bytes)", serializationTime, len(bytes))

	// 反序列化
	start = time.Now()
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	deserializationTime := time.Since(start)

	helper.AssertNoError(err, "Large payload deserialization should succeed")
	helper.AssertNotNil(deserializedEvent.GetPayload(), "Payload should not be nil")

	t.Logf("Large payload deserialization time: %v", deserializationTime)
}

// TestEnterpriseDomainEvent_SpecialCharactersSerialization 测试特殊字符序列化
func TestEnterpriseDomainEvent_SpecialCharactersSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建包含特殊字符的 Payload
	payload := map[string]interface{}{
		"chinese":   "中文测试",
		"emoji":     "😀🎉🚀",
		"quotes":    `"quoted" and 'single'`,
		"newlines":  "line1\nline2\nline3",
		"tabs":      "col1\tcol2\tcol3",
		"backslash": "path\\to\\file",
		"unicode":   "Unicode: \u4e2d\u6587",
	}

	// 创建事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", payload)
	event.SetTenantId("tenant-中文")
	event.SetCorrelationId("correlation-😀")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "Special characters serialization should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "Special characters deserialization should succeed")

	// 验证特殊字符保持不变
	helper.AssertEqual("tenant-中文", deserializedEvent.GetTenantId(), "Chinese characters should be preserved")
	helper.AssertEqual("correlation-😀", deserializedEvent.GetCorrelationId(), "Emoji should be preserved")
}

// TestEnterpriseDomainEvent_NestedStructureSerialization 测试嵌套结构序列化
func TestEnterpriseDomainEvent_NestedStructureSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建深度嵌套的 Payload
	nestedPayload := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": map[string]interface{}{
					"level4": map[string]interface{}{
						"value": "deep nested value",
					},
				},
			},
		},
	}

	// 创建事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nestedPayload)
	event.SetTenantId("tenant-001")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "Nested structure serialization should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "Nested structure deserialization should succeed")

	// 验证嵌套结构
	helper.AssertNotNil(deserializedEvent.GetPayload(), "Payload should not be nil")
}

// TestEnterpriseDomainEvent_ArrayPayloadSerialization 测试数组 Payload 序列化
func TestEnterpriseDomainEvent_ArrayPayloadSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建数组 Payload
	arrayPayload := []interface{}{
		map[string]interface{}{"id": 1, "name": "item1"},
		map[string]interface{}{"id": 2, "name": "item2"},
		map[string]interface{}{"id": 3, "name": "item3"},
	}

	// 创建事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", arrayPayload)
	event.SetTenantId("tenant-001")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "Array payload serialization should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "Array payload deserialization should succeed")

	// 验证数组 Payload
	helper.AssertNotNil(deserializedEvent.GetPayload(), "Payload should not be nil")
}

// TestEnterpriseDomainEvent_JSONCompatibility 测试与标准 JSON 库的兼容性
func TestEnterpriseDomainEvent_JSONCompatibility(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件
	payload := helper.CreateTestPayload()
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", payload)
	event.SetTenantId("tenant-001")

	// 使用 jxtevent 序列化
	jxtBytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "jxtevent serialization should succeed")

	// 使用标准 JSON 库反序列化
	var stdEvent jxtevent.EnterpriseDomainEvent
	err = json.Unmarshal(jxtBytes, &stdEvent)
	helper.AssertNoError(err, "Standard JSON deserialization should succeed")

	// 验证字段
	helper.AssertEqual(event.GetEventID(), stdEvent.GetEventID(), "EventID should match")
	helper.AssertEqual(event.GetTenantId(), stdEvent.GetTenantId(), "TenantId should match")

	// 使用标准 JSON 库序列化
	stdBytes, err := json.Marshal(event)
	helper.AssertNoError(err, "Standard JSON serialization should succeed")

	// 使用 jxtevent 反序列化
	jxtEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](stdBytes)
	helper.AssertNoError(err, "jxtevent deserialization should succeed")

	// 验证字段
	helper.AssertEqual(event.GetEventID(), jxtEvent.GetEventID(), "EventID should match")
	helper.AssertEqual(event.GetTenantId(), jxtEvent.GetTenantId(), "TenantId should match")
}

// TestEnterpriseDomainEvent_UnifiedJSONUsage 测试使用统一的 JSON 包
func TestEnterpriseDomainEvent_UnifiedJSONUsage(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId("tenant-001")

	// 使用 jxtjson 序列化
	bytes, err := jxtjson.Marshal(event)
	helper.AssertNoError(err, "jxtjson.Marshal should succeed")

	// 使用 jxtjson 反序列化
	var deserializedEvent jxtevent.EnterpriseDomainEvent
	err = jxtjson.Unmarshal(bytes, &deserializedEvent)
	helper.AssertNoError(err, "jxtjson.Unmarshal should succeed")

	// 验证
	helper.AssertEqual(event.GetEventID(), deserializedEvent.GetEventID(), "EventID should match")
	helper.AssertEqual(event.GetTenantId(), deserializedEvent.GetTenantId(), "TenantId should match")
}

// TestEnterpriseDomainEvent_ErrorHandling 测试错误处理
func TestEnterpriseDomainEvent_ErrorHandling(t *testing.T) {
	helper := NewTestHelper(t)

	// 测试反序列化空数据
	_, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent]([]byte{})
	helper.AssertError(err, "Deserializing empty data should fail")

	// 测试反序列化无效 JSON
	_, err = jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent]([]byte("invalid json"))
	helper.AssertError(err, "Deserializing invalid JSON should fail")

	// 测试序列化 nil 事件
	_, err = jxtevent.MarshalDomainEvent(nil)
	helper.AssertError(err, "Serializing nil event should fail")
}

// TestEnterpriseDomainEvent_MultiTenantSerialization 测试多租户序列化
func TestEnterpriseDomainEvent_MultiTenantSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建多个租户的事件
	events := make([]*jxtevent.EnterpriseDomainEvent, 0, 10)
	for i := 0; i < 10; i++ {
		event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
		event.SetTenantId("tenant-" + string(rune('0'+i)))
		events = append(events, event)
	}

	// 序列化所有事件
	serializedEvents := make([][]byte, 0, 10)
	for _, event := range events {
		bytes, err := jxtevent.MarshalDomainEvent(event)
		helper.AssertNoError(err, "Serialization should succeed")
		serializedEvents = append(serializedEvents, bytes)
	}

	// 反序列化并验证租户隔离
	for i, bytes := range serializedEvents {
		deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
		helper.AssertNoError(err, "Deserialization should succeed")
		expectedTenantId := "tenant-" + string(rune('0'+i))
		helper.AssertEqual(expectedTenantId, deserializedEvent.GetTenantId(), "TenantId should match")
	}
}
