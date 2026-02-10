package event

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 测试用的Payload结构
type TestPayload struct {
	Title     string    `json:"title"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	Count     int       `json:"count"`
}

func TestUnmarshalPayload_Success(t *testing.T) {
	// 准备测试数据
	now := time.Now()
	originalPayload := TestPayload{
		Title:     "Test Title",
		CreatedBy: "user-001",
		CreatedAt: now,
		Count:     42,
	}

	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", originalPayload)

	// 执行
	result, err := UnmarshalPayload[TestPayload](event)

	// 验证
	assert.NoError(t, err)
	assert.Equal(t, originalPayload.Title, result.Title)
	assert.Equal(t, originalPayload.CreatedBy, result.CreatedBy)
	assert.Equal(t, originalPayload.Count, result.Count)
	// 时间字段可能因为序列化/反序列化有微小差异，使用WithinDuration
	assert.WithinDuration(t, originalPayload.CreatedAt, result.CreatedAt, time.Second)
}

func TestUnmarshalPayload_DirectTypeMatch(t *testing.T) {
	// 测试：如果payload已经是目标类型，直接返回
	originalPayload := TestPayload{
		Title:     "Direct Match",
		CreatedBy: "user-001",
		Count:     42,
	}

	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", originalPayload)

	// 执行
	result, err := UnmarshalPayload[TestPayload](event)

	// 验证
	assert.NoError(t, err)
	assert.Equal(t, originalPayload.Title, result.Title)
	assert.Equal(t, originalPayload.CreatedBy, result.CreatedBy)
	assert.Equal(t, originalPayload.Count, result.Count)
}

func TestUnmarshalPayload_NilPayload(t *testing.T) {
	// 准备测试数据 - nil payload
	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	// 执行
	result, err := UnmarshalPayload[TestPayload](event)

	// 验证
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "payload is nil")
	assert.Equal(t, TestPayload{}, result)
}

func TestMarshalPayload_Success(t *testing.T) {
	// 准备测试数据
	payload := TestPayload{
		Title:     "Test Title",
		CreatedBy: "user-001",
		Count:     42,
	}

	// 执行
	result, err := MarshalPayload(payload)

	// 验证
	assert.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Contains(t, string(result), "Test Title")
	assert.Contains(t, string(result), "user-001")
}

func TestMarshalPayload_NilPayload(t *testing.T) {
	// 执行
	result, err := MarshalPayload(nil)

	// 验证
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "payload is nil")
	assert.Nil(t, result)
}

func TestUnmarshalPayload_WithEnterpriseDomainEvent(t *testing.T) {
	// 测试与EnterpriseDomainEvent的兼容性
	originalPayload := TestPayload{
		Title:     "Enterprise Test",
		CreatedBy: "user-001",
		Count:     100,
	}

	event := NewEnterpriseDomainEvent("TestEvent", "test-123", "TestAggregate", originalPayload)
	event.SetTenantId(1)

	// 执行
	result, err := UnmarshalPayload[TestPayload](event)

	// 验证
	assert.NoError(t, err)
	assert.Equal(t, originalPayload.Title, result.Title)
	assert.Equal(t, originalPayload.CreatedBy, result.CreatedBy)
	assert.Equal(t, originalPayload.Count, result.Count)
}

func TestUnmarshalPayload_ComplexNestedStructure(t *testing.T) {
	// 测试复杂嵌套结构
	type NestedPayload struct {
		ID   string `json:"id"`
		Data struct {
			Name  string   `json:"name"`
			Tags  []string `json:"tags"`
			Count int      `json:"count"`
		} `json:"data"`
	}

	originalPayload := NestedPayload{
		ID: "nested-123",
	}
	originalPayload.Data.Name = "Test Name"
	originalPayload.Data.Tags = []string{"tag1", "tag2", "tag3"}
	originalPayload.Data.Count = 42

	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", originalPayload)

	// 执行
	result, err := UnmarshalPayload[NestedPayload](event)

	// 验证
	assert.NoError(t, err)
	assert.Equal(t, originalPayload.ID, result.ID)
	assert.Equal(t, originalPayload.Data.Name, result.Data.Name)
	assert.Equal(t, originalPayload.Data.Tags, result.Data.Tags)
	assert.Equal(t, originalPayload.Data.Count, result.Data.Count)
}
