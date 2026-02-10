package function_regression_tests

import (
	"sync"
	"testing"
	"time"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
)

// ============================================================================
// 统一 JSON 序列化架构回归测试
// ============================================================================

// TestJSON_UnifiedPackage 测试统一的 JSON 包
func TestJSON_UnifiedPackage(t *testing.T) {
	helper := NewTestHelper(t)

	// 测试数据
	data := map[string]interface{}{
		"name":  "test",
		"count": 42,
	}

	// 测试 Marshal
	bytes, err := jxtjson.Marshal(data)
	helper.AssertNoError(err, "Marshal should succeed")
	helper.AssertNotEmpty(bytes, "Marshal should return non-empty bytes")

	// 测试 Unmarshal
	var result map[string]interface{}
	err = jxtjson.Unmarshal(bytes, &result)
	helper.AssertNoError(err, "Unmarshal should succeed")
	helper.AssertEqual("test", result["name"], "Name should match")
	helper.AssertEqual(float64(42), result["count"], "Count should match")
}

// TestJSON_MarshalToString 测试字符串序列化
func TestJSON_MarshalToString(t *testing.T) {
	helper := NewTestHelper(t)

	data := map[string]string{
		"key": "value",
	}

	// 测试 MarshalToString
	str, err := jxtjson.MarshalToString(data)
	helper.AssertNoError(err, "MarshalToString should succeed")
	helper.AssertNotEmpty(str, "MarshalToString should return non-empty string")

	// 测试 UnmarshalFromString
	var result map[string]string
	err = jxtjson.UnmarshalFromString(str, &result)
	helper.AssertNoError(err, "UnmarshalFromString should succeed")
	helper.AssertEqual("value", result["key"], "Value should match")
}

// TestJSON_FastSerialization 测试高性能序列化
func TestJSON_FastSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	data := []int{1, 2, 3, 4, 5}

	// 测试 MarshalFast
	bytes, err := jxtjson.MarshalFast(data)
	helper.AssertNoError(err, "MarshalFast should succeed")
	helper.AssertNotEmpty(bytes, "MarshalFast should return non-empty bytes")

	// 测试 UnmarshalFast
	var result []int
	err = jxtjson.UnmarshalFast(bytes, &result)
	helper.AssertNoError(err, "UnmarshalFast should succeed")
	helper.AssertEqual(5, len(result), "Length should match")
}

// TestJSON_RawMessage 测试 RawMessage 类型
func TestJSON_RawMessage(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 RawMessage
	raw := jxtjson.RawMessage(`{"test":"data"}`)
	helper.AssertNotEmpty(raw, "RawMessage should not be empty")

	// 反序列化 RawMessage
	var result map[string]string
	err := jxtjson.Unmarshal(raw, &result)
	helper.AssertNoError(err, "Unmarshal RawMessage should succeed")
	helper.AssertEqual("data", result["test"], "Value should match")
}

// TestDomainEvent_UsesUnifiedJSON 测试 DomainEvent 使用统一的 JSON 包
func TestDomainEvent_UsesUnifiedJSON(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 DomainEvent
	payload := helper.CreateTestPayload()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", payload)

	// 序列化 DomainEvent（应该使用 jxtjson）
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")
	helper.AssertNotEmpty(bytes, "MarshalDomainEvent should return non-empty bytes")

	// 反序列化 DomainEvent
	result, err := jxtevent.UnmarshalDomainEvent[*jxtevent.BaseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")
	helper.AssertEqual(event.EventType, result.EventType, "EventType should match")
	helper.AssertEqual(event.AggregateID, result.AggregateID, "AggregateID should match")
}

// TestDomainEvent_PayloadSerialization 测试 Payload 序列化
func TestDomainEvent_PayloadSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建复杂 Payload
	payload := helper.CreateComplexPayload()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", payload)

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	result, err := jxtevent.UnmarshalDomainEvent[*jxtevent.BaseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 提取 Payload
	extractedPayload, err := jxtevent.UnmarshalPayload[ComplexPayload](result)
	helper.AssertNoError(err, "UnmarshalPayload should succeed")
	helper.AssertEqual(payload.Name, extractedPayload.Name, "Name should match")
	helper.AssertEqual(len(payload.Tags), len(extractedPayload.Tags), "Tags length should match")
}

// TestDomainEvent_PerformanceComparison 测试性能对比
func TestDomainEvent_PerformanceComparison(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建测试数据
	payload := helper.CreateTestPayload()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", payload)

	// 测试序列化性能（应该使用 jsoniter，比 encoding/json 快 2-3 倍）
	iterations := 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		_, err := jxtevent.MarshalDomainEvent(event)
		helper.AssertNoError(err, "MarshalDomainEvent should succeed")
	}

	duration := time.Since(start)
	avgDuration := duration / time.Duration(iterations)

	// 验证性能（平均每次应该小于 1 微秒）
	helper.AssertTrue(avgDuration < time.Microsecond, "Average serialization should be fast")

	t.Logf("Average serialization time: %v", avgDuration)
}

// TestEnterpriseDomainEvent_UsesUnifiedJSON 测试 EnterpriseDomainEvent 使用统一的 JSON 包
func TestEnterpriseDomainEvent_UsesUnifiedJSON(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 EnterpriseDomainEvent
	payload := helper.CreateTestPayload()
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", payload)
	event.SetTenantId(1)
	event.SetTraceId("trace-123")
	event.SetCorrelationId("corr-456")

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	result, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 验证企业级字段
	helper.AssertEqual(1, result.GetTenantId(), "TenantId should match")
	helper.AssertEqual("trace-123", result.GetTraceId(), "TraceId should match")
	helper.AssertEqual("corr-456", result.GetCorrelationId(), "CorrelationId should match")
}

// TestJSON_Compatibility 测试 JSON 兼容性
func TestJSON_Compatibility(t *testing.T) {
	helper := NewTestHelper(t)

	// 测试 jxtjson.RawMessage 与 encoding/json 的兼容性
	// jxtjson.RawMessage 是 jsoniter.RawMessage 的别名，应该与 []byte 兼容

	raw := jxtjson.RawMessage(`{"test":"data"}`)

	// 应该可以直接赋值给 []byte
	var bytes []byte = raw
	helper.AssertNotEmpty(bytes, "RawMessage should be compatible with []byte")

	// 应该可以从 []byte 创建
	raw2 := jxtjson.RawMessage([]byte(`{"test":"data"}`))
	helper.AssertNotEmpty(raw2, "RawMessage should be creatable from []byte")
}

// TestJSON_EdgeCases 测试边界情况
func TestJSON_EdgeCases(t *testing.T) {
	helper := NewTestHelper(t)

	// 测试空对象
	emptyMap := map[string]interface{}{}
	bytes, err := jxtjson.Marshal(emptyMap)
	helper.AssertNoError(err, "Marshal empty map should succeed")
	helper.AssertEqual("{}", string(bytes), "Empty map should serialize to {}")

	// 测试 nil
	var nilMap map[string]interface{}
	bytes, err = jxtjson.Marshal(nilMap)
	helper.AssertNoError(err, "Marshal nil map should succeed")
	helper.AssertEqual("null", string(bytes), "Nil map should serialize to null")

	// 测试空数组
	emptyArray := []interface{}{}
	bytes, err = jxtjson.Marshal(emptyArray)
	helper.AssertNoError(err, "Marshal empty array should succeed")
	helper.AssertEqual("[]", string(bytes), "Empty array should serialize to []")
}

// TestJSON_ConcurrentSerialization 测试并发序列化
func TestJSON_ConcurrentSerialization(t *testing.T) {
	helper := NewTestHelper(t)

	goroutines := 100
	iterations := 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				data := map[string]interface{}{
					"id":    id,
					"index": j,
				}

				bytes, err := jxtjson.Marshal(data)
				helper.AssertNoError(err, "Concurrent Marshal should succeed")
				helper.AssertNotEmpty(bytes, "Concurrent Marshal should return non-empty bytes")

				var result map[string]interface{}
				err = jxtjson.Unmarshal(bytes, &result)
				helper.AssertNoError(err, "Concurrent Unmarshal should succeed")
			}
		}(i)
	}

	wg.Wait()
}

// TestJSON_LargePayload 测试大 Payload 序列化
func TestJSON_LargePayload(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建大 Payload（10000 个元素）
	largeArray := make([]map[string]interface{}, 10000)
	for i := 0; i < 10000; i++ {
		largeArray[i] = map[string]interface{}{
			"id":    i,
			"name":  "item-" + string(rune(i)),
			"value": i * 2,
		}
	}

	// 序列化
	start := time.Now()
	bytes, err := jxtjson.Marshal(largeArray)
	duration := time.Since(start)

	helper.AssertNoError(err, "Marshal large payload should succeed")
	helper.AssertNotEmpty(bytes, "Marshal large payload should return non-empty bytes")

	t.Logf("Large payload serialization time: %v", duration)

	// 反序列化
	start = time.Now()
	var result []map[string]interface{}
	err = jxtjson.Unmarshal(bytes, &result)
	duration = time.Since(start)

	helper.AssertNoError(err, "Unmarshal large payload should succeed")
	helper.AssertEqual(10000, len(result), "Large payload length should match")

	t.Logf("Large payload deserialization time: %v", duration)
}
