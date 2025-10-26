package function_regression_tests

import (
	"testing"
	"time"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
)

// TestUnmarshalPayload_StructToStruct 测试结构体到结构体的反序列化
func TestUnmarshalPayload_StructToStruct(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建原始 Payload
	originalPayload := helper.CreateTestPayload()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", originalPayload)

	// 反序列化
	result, err := jxtevent.UnmarshalPayload[TestPayload](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should succeed")
	helper.AssertEqual(originalPayload.Title, result.Title, "Title should match")
	helper.AssertEqual(originalPayload.CreatedBy, result.CreatedBy, "CreatedBy should match")
	helper.AssertEqual(originalPayload.Count, result.Count, "Count should match")
	helper.AssertWithinDuration(originalPayload.CreatedAt, result.CreatedAt, time.Second, "CreatedAt should match")
}

// TestUnmarshalPayload_DirectTypeMatch 测试直接类型匹配优化
func TestUnmarshalPayload_DirectTypeMatch(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Payload（已经是目标类型）
	originalPayload := helper.CreateTestPayload()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", originalPayload)

	// 反序列化（应该直接返回，不需要序列化/反序列化）
	result, err := jxtevent.UnmarshalPayload[TestPayload](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should succeed")
	helper.AssertEqual(originalPayload.Title, result.Title, "Should match original")
}

// TestUnmarshalPayload_ComplexNestedStructure 测试复杂嵌套结构
func TestUnmarshalPayload_ComplexNestedStructure(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建复杂 Payload
	originalPayload := helper.CreateComplexPayload()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", originalPayload)

	// 反序列化
	result, err := jxtevent.UnmarshalPayload[ComplexPayload](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should succeed")
	helper.AssertEqual(originalPayload.ID, result.ID, "ID should match")
	helper.AssertEqual(originalPayload.Name, result.Name, "Name should match")
	helper.AssertEqual(len(originalPayload.Tags), len(result.Tags), "Tags length should match")
	helper.AssertNotNil(result.Nested, "Nested should not be nil")
	helper.AssertEqual(originalPayload.Nested.Field1, result.Nested.Field1, "Nested.Field1 should match")
	helper.AssertEqual(originalPayload.Nested.Field2, result.Nested.Field2, "Nested.Field2 should match")
}

// TestUnmarshalPayload_NilPayload 测试 nil Payload
func TestUnmarshalPayload_NilPayload(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 反序列化
	result, err := jxtevent.UnmarshalPayload[TestPayload](event)

	// 验证应该返回错误
	helper.AssertError(err, "Should return error for nil payload")
	helper.AssertErrorContains(err, "payload is nil", "Error should mention nil payload")
	helper.AssertEqual(TestPayload{}, result, "Result should be zero value")
}

// TestUnmarshalPayload_WithEnterpriseDomainEvent 测试企业级事件的 Payload
func TestUnmarshalPayload_WithEnterpriseDomainEvent(t *testing.T) {
	helper := NewTestHelper(t)

	payload := helper.CreateTestPayload()
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", payload)

	// 反序列化
	result, err := jxtevent.UnmarshalPayload[TestPayload](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should work with EnterpriseDomainEvent")
	helper.AssertEqual(payload.Title, result.Title, "Title should match")
}

// TestUnmarshalPayload_DifferentTypes 测试不同类型的转换
func TestUnmarshalPayload_DifferentTypes(t *testing.T) {
	helper := NewTestHelper(t)

	// 定义另一个结构体
	type AnotherPayload struct {
		Title string `json:"title"`
		Count int    `json:"count"`
	}

	// 创建 TestPayload
	originalPayload := TestPayload{
		Title:     "Test Title",
		CreatedBy: "user-001",
		Count:     42,
	}
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", originalPayload)

	// 反序列化为 AnotherPayload（部分字段匹配）
	result, err := jxtevent.UnmarshalPayload[AnotherPayload](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should succeed for partial match")
	helper.AssertEqual(originalPayload.Title, result.Title, "Title should match")
	helper.AssertEqual(originalPayload.Count, result.Count, "Count should match")
}

// TestUnmarshalPayload_StringPayload 测试字符串 Payload
func TestUnmarshalPayload_StringPayload(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", "simple string")

	// 反序列化为字符串
	result, err := jxtevent.UnmarshalPayload[string](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should work with string")
	helper.AssertEqual("simple string", result, "String should match")
}

// TestUnmarshalPayload_NumberPayload 测试数字 Payload
func TestUnmarshalPayload_NumberPayload(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", 12345)

	// 反序列化为 int
	result, err := jxtevent.UnmarshalPayload[int](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should work with int")
	helper.AssertEqual(12345, result, "Number should match")
}

// TestUnmarshalPayload_ArrayPayload 测试数组 Payload
func TestUnmarshalPayload_ArrayPayload(t *testing.T) {
	helper := NewTestHelper(t)

	originalArray := []string{"item1", "item2", "item3"}
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", originalArray)

	// 反序列化为数组
	result, err := jxtevent.UnmarshalPayload[[]string](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should work with array")
	helper.AssertEqual(len(originalArray), len(result), "Array length should match")
	helper.AssertEqual(originalArray[0], result[0], "Array elements should match")
}

// TestUnmarshalPayload_PointerPayload 测试指针 Payload
func TestUnmarshalPayload_PointerPayload(t *testing.T) {
	helper := NewTestHelper(t)

	originalPayload := helper.CreateTestPayload()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", &originalPayload)

	// 反序列化为指针
	result, err := jxtevent.UnmarshalPayload[*TestPayload](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should work with pointer")
	helper.AssertNotNil(result, "Result should not be nil")
	helper.AssertEqual(originalPayload.Title, result.Title, "Title should match")
}

// TestMarshalPayload_Success 测试 Payload 序列化
func TestMarshalPayload_Success(t *testing.T) {
	helper := NewTestHelper(t)

	payload := helper.CreateTestPayload()

	// 序列化
	bytes, err := jxtevent.MarshalPayload(payload)

	// 验证
	helper.AssertNoError(err, "MarshalPayload should succeed")
	helper.AssertNotEmpty(bytes, "Bytes should not be empty")
	helper.AssertContains(string(bytes), payload.Title, "Should contain title")
	helper.AssertContains(string(bytes), payload.CreatedBy, "Should contain createdBy")
}

// TestMarshalPayload_NilPayload 测试序列化 nil Payload
func TestMarshalPayload_NilPayload(t *testing.T) {
	helper := NewTestHelper(t)

	// 序列化 nil
	_, err := jxtevent.MarshalPayload(nil)

	// 验证
	helper.AssertError(err, "MarshalPayload should return error for nil")
	helper.AssertErrorContains(err, "payload is nil", "Error should mention nil payload")
}

// TestMarshalPayload_ComplexPayload 测试序列化复杂 Payload
func TestMarshalPayload_ComplexPayload(t *testing.T) {
	helper := NewTestHelper(t)

	payload := helper.CreateComplexPayload()

	// 序列化
	bytes, err := jxtevent.MarshalPayload(payload)

	// 验证
	helper.AssertNoError(err, "MarshalPayload should succeed")
	helper.AssertNotEmpty(bytes, "Bytes should not be empty")
	helper.AssertContains(string(bytes), payload.ID, "Should contain ID")
	helper.AssertContains(string(bytes), payload.Name, "Should contain Name")
}

// TestPayloadRoundTrip 测试 Payload 序列化/反序列化往返
func TestPayloadRoundTrip(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建原始 Payload
	originalPayload := helper.CreateTestPayload()

	// 序列化
	_, err := jxtevent.MarshalPayload(originalPayload)
	helper.AssertNoError(err, "MarshalPayload should succeed")

	// 创建事件（使用序列化后的字节）
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", originalPayload)

	// 反序列化
	result, err := jxtevent.UnmarshalPayload[TestPayload](event)
	helper.AssertNoError(err, "UnmarshalPayload should succeed")

	// 验证往返后数据一致
	helper.AssertEqual(originalPayload.Title, result.Title, "Title should match after round trip")
	helper.AssertEqual(originalPayload.CreatedBy, result.CreatedBy, "CreatedBy should match after round trip")
	helper.AssertEqual(originalPayload.Count, result.Count, "Count should match after round trip")
}

// TestUnmarshalPayload_MultipleUnmarshal 测试多次反序列化
func TestUnmarshalPayload_MultipleUnmarshal(t *testing.T) {
	helper := NewTestHelper(t)

	payload := helper.CreateTestPayload()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", payload)

	// 多次反序列化同一个事件
	result1, err1 := jxtevent.UnmarshalPayload[TestPayload](event)
	result2, err2 := jxtevent.UnmarshalPayload[TestPayload](event)
	result3, err3 := jxtevent.UnmarshalPayload[TestPayload](event)

	// 验证所有反序列化都成功
	helper.AssertNoError(err1, "First unmarshal should succeed")
	helper.AssertNoError(err2, "Second unmarshal should succeed")
	helper.AssertNoError(err3, "Third unmarshal should succeed")

	// 验证结果一致
	helper.AssertEqual(result1.Title, result2.Title, "Results should be consistent")
	helper.AssertEqual(result2.Title, result3.Title, "Results should be consistent")
}

// TestUnmarshalPayload_EmptyStruct 测试空结构体
func TestUnmarshalPayload_EmptyStruct(t *testing.T) {
	helper := NewTestHelper(t)

	type EmptyPayload struct{}

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", EmptyPayload{})

	// 反序列化
	result, err := jxtevent.UnmarshalPayload[EmptyPayload](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should work with empty struct")
	helper.AssertEqual(EmptyPayload{}, result, "Result should be empty struct")
}

// TestUnmarshalPayload_WithTags 测试带 JSON 标签的结构体
func TestUnmarshalPayload_WithTags(t *testing.T) {
	helper := NewTestHelper(t)

	type TaggedPayload struct {
		FieldOne   string `json:"field_one"`
		FieldTwo   int    `json:"field_two"`
		FieldThree bool   `json:"field_three,omitempty"`
	}

	originalPayload := TaggedPayload{
		FieldOne:   "value1",
		FieldTwo:   123,
		FieldThree: true,
	}

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", originalPayload)

	// 反序列化
	result, err := jxtevent.UnmarshalPayload[TaggedPayload](event)

	// 验证
	helper.AssertNoError(err, "UnmarshalPayload should work with JSON tags")
	helper.AssertEqual(originalPayload.FieldOne, result.FieldOne, "FieldOne should match")
	helper.AssertEqual(originalPayload.FieldTwo, result.FieldTwo, "FieldTwo should match")
	helper.AssertEqual(originalPayload.FieldThree, result.FieldThree, "FieldThree should match")
}

// TestUnmarshalPayload_TypeSafety 测试类型安全
func TestUnmarshalPayload_TypeSafety(t *testing.T) {
	helper := NewTestHelper(t)

	// 这个测试主要验证编译期类型安全
	// 如果类型不匹配，编译器会报错

	payload := helper.CreateTestPayload()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", payload)

	// 正确的类型
	_, err := jxtevent.UnmarshalPayload[TestPayload](event)
	helper.AssertNoError(err, "Correct type should work")

	// 不同的类型（但字段兼容）
	type CompatiblePayload struct {
		Title string `json:"title"`
	}
	_, err = jxtevent.UnmarshalPayload[CompatiblePayload](event)
	helper.AssertNoError(err, "Compatible type should work")
}
