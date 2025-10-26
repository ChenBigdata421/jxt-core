package event

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPayload 测试用的 Payload 结构
type TestEventPayload struct {
	Title     string    `json:"title"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	Count     int       `json:"count"`
}

// ========== UnmarshalDomainEvent 测试 ==========

func TestUnmarshalDomainEvent_BaseDomainEvent_Success(t *testing.T) {
	// 准备测试数据
	originalPayload := TestEventPayload{
		Title:     "Test Event",
		CreatedBy: "user-001",
		CreatedAt: time.Now(),
		Count:     42,
	}

	originalEvent := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", originalPayload)

	// 序列化
	eventBytes, err := MarshalDomainEvent(originalEvent)
	assert.NoError(t, err)
	assert.NotEmpty(t, eventBytes)

	// 反序列化
	result, err := UnmarshalDomainEvent[*BaseDomainEvent](eventBytes)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// 验证基本字段
	assert.Equal(t, originalEvent.GetEventID(), result.GetEventID())
	assert.Equal(t, originalEvent.GetEventType(), result.GetEventType())
	assert.Equal(t, originalEvent.GetAggregateID(), result.GetAggregateID())
	assert.Equal(t, originalEvent.GetAggregateType(), result.GetAggregateType())
	assert.Equal(t, originalEvent.GetVersion(), result.GetVersion())

	// 验证 Payload（注意：反序列化后是 map[string]interface{}）
	assert.NotNil(t, result.GetPayload())
}

func TestUnmarshalDomainEvent_EnterpriseDomainEvent_Success(t *testing.T) {
	// 准备测试数据
	originalPayload := TestEventPayload{
		Title:     "Enterprise Event",
		CreatedBy: "user-002",
		CreatedAt: time.Now(),
		Count:     100,
	}

	originalEvent := NewEnterpriseDomainEvent("EnterpriseEvent", "test-456", "TestAggregate", originalPayload)
	originalEvent.SetTenantId("tenant-001")
	originalEvent.SetCorrelationId("correlation-123")
	originalEvent.SetTraceId("trace-456")

	// 序列化
	eventBytes, err := MarshalDomainEvent(originalEvent)
	assert.NoError(t, err)
	assert.NotEmpty(t, eventBytes)

	// 反序列化
	result, err := UnmarshalDomainEvent[*EnterpriseDomainEvent](eventBytes)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// 验证基本字段
	assert.Equal(t, originalEvent.GetEventID(), result.GetEventID())
	assert.Equal(t, originalEvent.GetEventType(), result.GetEventType())
	assert.Equal(t, originalEvent.GetAggregateID(), result.GetAggregateID())
	assert.Equal(t, originalEvent.GetAggregateType(), result.GetAggregateType())

	// 验证企业级字段
	assert.Equal(t, "tenant-001", result.GetTenantId())
	assert.Equal(t, "correlation-123", result.GetCorrelationId())
	assert.Equal(t, "trace-456", result.GetTraceId())
}

func TestUnmarshalDomainEvent_EmptyData_Error(t *testing.T) {
	result, err := UnmarshalDomainEvent[*BaseDomainEvent]([]byte{})
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "data is empty")
}

func TestUnmarshalDomainEvent_InvalidJSON_Error(t *testing.T) {
	invalidJSON := []byte(`{"invalid json`)
	result, err := UnmarshalDomainEvent[*BaseDomainEvent](invalidJSON)
	assert.Error(t, err)
	// 注意：泛型返回的是指针类型，错误时返回 nil
	if result != nil {
		// 如果不是 nil，说明是零值
		assert.Empty(t, result.GetEventID())
	}
	assert.Contains(t, err.Error(), "failed to unmarshal domain event")
}

// ========== MarshalDomainEvent 测试 ==========

func TestMarshalDomainEvent_Success(t *testing.T) {
	payload := TestEventPayload{
		Title:     "Test Event",
		CreatedBy: "user-001",
		CreatedAt: time.Now(),
		Count:     42,
	}

	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", payload)

	// 序列化
	eventBytes, err := MarshalDomainEvent(event)
	assert.NoError(t, err)
	assert.NotEmpty(t, eventBytes)

	// 验证 JSON 包含关键字段
	jsonString := string(eventBytes)
	assert.Contains(t, jsonString, "TestEvent")
	assert.Contains(t, jsonString, "test-123")
	assert.Contains(t, jsonString, "TestAggregate")
	assert.Contains(t, jsonString, "Test Event")
}

func TestMarshalDomainEvent_NilEvent_Error(t *testing.T) {
	eventBytes, err := MarshalDomainEvent(nil)
	assert.Error(t, err)
	assert.Nil(t, eventBytes)
	assert.Contains(t, err.Error(), "event is nil")
}

// ========== UnmarshalDomainEventFromString 测试 ==========

func TestUnmarshalDomainEventFromString_Success(t *testing.T) {
	payload := TestEventPayload{
		Title:     "Test Event",
		CreatedBy: "user-001",
		CreatedAt: time.Now(),
		Count:     42,
	}

	originalEvent := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", payload)

	// 序列化为字符串
	jsonString, err := MarshalDomainEventToString(originalEvent)
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonString)

	// 反序列化
	result, err := UnmarshalDomainEventFromString[*BaseDomainEvent](jsonString)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, originalEvent.GetEventID(), result.GetEventID())
}

func TestUnmarshalDomainEventFromString_EmptyString_Error(t *testing.T) {
	result, err := UnmarshalDomainEventFromString[*BaseDomainEvent]("")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "json string is empty")
}

// ========== MarshalDomainEventToString 测试 ==========

func TestMarshalDomainEventToString_Success(t *testing.T) {
	payload := TestEventPayload{
		Title:     "Test Event",
		CreatedBy: "user-001",
		CreatedAt: time.Now(),
		Count:     42,
	}

	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", payload)

	// 序列化为字符串
	jsonString, err := MarshalDomainEventToString(event)
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonString)
	assert.Contains(t, jsonString, "TestEvent")
}

func TestMarshalDomainEventToString_NilEvent_Error(t *testing.T) {
	jsonString, err := MarshalDomainEventToString(nil)
	assert.Error(t, err)
	assert.Empty(t, jsonString)
	assert.Contains(t, err.Error(), "event is nil")
}

// ========== 集成测试：完整的序列化-反序列化流程 ==========

func TestDomainEvent_RoundTrip_WithPayloadExtraction(t *testing.T) {
	// 1. 创建原始事件（Command Side）
	originalPayload := TestEventPayload{
		Title:     "Integration Test",
		CreatedBy: "user-integration",
		CreatedAt: time.Now(),
		Count:     999,
	}

	originalEvent := NewEnterpriseDomainEvent("IntegrationEvent", "test-integration", "TestAggregate", originalPayload)
	originalEvent.SetTenantId("tenant-integration")

	// 2. 序列化（模拟保存到 Outbox）
	eventBytes, err := MarshalDomainEvent(originalEvent)
	assert.NoError(t, err)

	// 3. 反序列化（模拟 Query Side 接收）
	receivedEvent, err := UnmarshalDomainEvent[*EnterpriseDomainEvent](eventBytes)
	assert.NoError(t, err)

	// 4. 验证基本字段
	assert.Equal(t, originalEvent.GetEventID(), receivedEvent.GetEventID())
	assert.Equal(t, originalEvent.GetEventType(), receivedEvent.GetEventType())
	assert.Equal(t, "tenant-integration", receivedEvent.GetTenantId())

	// 5. 提取 Payload（模拟 Query Side 处理）
	extractedPayload, err := UnmarshalPayload[TestEventPayload](receivedEvent)
	assert.NoError(t, err)
	assert.Equal(t, "Integration Test", extractedPayload.Title)
	assert.Equal(t, "user-integration", extractedPayload.CreatedBy)
	assert.Equal(t, 999, extractedPayload.Count)
}

// ========== 性能测试 ==========

func BenchmarkMarshalDomainEvent(b *testing.B) {
	payload := TestEventPayload{
		Title:     "Benchmark Event",
		CreatedBy: "user-bench",
		CreatedAt: time.Now(),
		Count:     100,
	}

	event := NewBaseDomainEvent("BenchmarkEvent", "bench-123", "TestAggregate", payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = MarshalDomainEvent(event)
	}
}

func BenchmarkUnmarshalDomainEvent(b *testing.B) {
	payload := TestEventPayload{
		Title:     "Benchmark Event",
		CreatedBy: "user-bench",
		CreatedAt: time.Now(),
		Count:     100,
	}

	event := NewBaseDomainEvent("BenchmarkEvent", "bench-123", "TestAggregate", payload)
	eventBytes, _ := MarshalDomainEvent(event)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = UnmarshalDomainEvent[*BaseDomainEvent](eventBytes)
	}
}
