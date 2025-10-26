package function_regression_tests

import (
	"testing"
	"time"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelper 测试辅助工具
type TestHelper struct {
	t *testing.T
}

// NewTestHelper 创建测试辅助工具
func NewTestHelper(t *testing.T) *TestHelper {
	return &TestHelper{t: t}
}

// AssertNoError 断言无错误
func (h *TestHelper) AssertNoError(err error, msgAndArgs ...interface{}) {
	assert.NoError(h.t, err, msgAndArgs...)
}

// RequireNoError 要求无错误
func (h *TestHelper) RequireNoError(err error, msgAndArgs ...interface{}) {
	require.NoError(h.t, err, msgAndArgs...)
}

// AssertEqual 断言相等
func (h *TestHelper) AssertEqual(expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.Equal(h.t, expected, actual, msgAndArgs...)
}

// AssertNotEmpty 断言非空
func (h *TestHelper) AssertNotEmpty(obj interface{}, msgAndArgs ...interface{}) {
	assert.NotEmpty(h.t, obj, msgAndArgs...)
}

// AssertNil 断言为 nil
func (h *TestHelper) AssertNil(obj interface{}, msgAndArgs ...interface{}) {
	assert.Nil(h.t, obj, msgAndArgs...)
}

// AssertNotNil 断言不为 nil
func (h *TestHelper) AssertNotNil(obj interface{}, msgAndArgs ...interface{}) {
	assert.NotNil(h.t, obj, msgAndArgs...)
}

// AssertTrue 断言为 true
func (h *TestHelper) AssertTrue(value bool, msgAndArgs ...interface{}) {
	assert.True(h.t, value, msgAndArgs...)
}

// AssertFalse 断言为 false
func (h *TestHelper) AssertFalse(value bool, msgAndArgs ...interface{}) {
	assert.False(h.t, value, msgAndArgs...)
}

// AssertGreater 断言大于
func (h *TestHelper) AssertGreater(e1, e2 interface{}, msgAndArgs ...interface{}) {
	assert.Greater(h.t, e1, e2, msgAndArgs...)
}

// AssertContains 断言包含
func (h *TestHelper) AssertContains(s, contains interface{}, msgAndArgs ...interface{}) {
	assert.Contains(h.t, s, contains, msgAndArgs...)
}

// AssertNotEqual 断言两个值不相等
func (h *TestHelper) AssertNotEqual(expected, actual interface{}, msgAndArgs ...interface{}) {
	assert.NotEqual(h.t, expected, actual, msgAndArgs...)
}

// AssertRegex 断言匹配正则表达式
func (h *TestHelper) AssertRegex(pattern, str string, msgAndArgs ...interface{}) {
	assert.Regexp(h.t, pattern, str, msgAndArgs...)
}

// AssertWithinDuration 断言时间在范围内
func (h *TestHelper) AssertWithinDuration(expected, actual time.Time, delta time.Duration, msgAndArgs ...interface{}) {
	assert.WithinDuration(h.t, expected, actual, delta, msgAndArgs...)
}

// AssertError 断言有错误
func (h *TestHelper) AssertError(err error, msgAndArgs ...interface{}) {
	assert.Error(h.t, err, msgAndArgs...)
}

// AssertErrorContains 断言错误包含指定文本
func (h *TestHelper) AssertErrorContains(err error, contains string, msgAndArgs ...interface{}) {
	h.AssertError(err, msgAndArgs...)
	if err != nil {
		h.AssertContains(err.Error(), contains, msgAndArgs...)
	}
}

// CreateBaseDomainEvent 创建测试用的基础领域事件
func (h *TestHelper) CreateBaseDomainEvent(eventType, aggregateID, aggregateType string, payload interface{}) *jxtevent.BaseDomainEvent {
	return jxtevent.NewBaseDomainEvent(eventType, aggregateID, aggregateType, payload)
}

// CreateEnterpriseDomainEvent 创建测试用的企业级领域事件
func (h *TestHelper) CreateEnterpriseDomainEvent(eventType, aggregateID, aggregateType string, payload interface{}) *jxtevent.EnterpriseDomainEvent {
	return jxtevent.NewEnterpriseDomainEvent(eventType, aggregateID, aggregateType, payload)
}

// TestPayload 测试用的 Payload 结构
type TestPayload struct {
	Title     string    `json:"title"`
	CreatedBy string    `json:"createdBy"`
	Count     int       `json:"count"`
	CreatedAt time.Time `json:"createdAt"`
}

// ComplexPayload 复杂的 Payload 结构（用于嵌套测试）
type ComplexPayload struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Metadata MetadataPayload `json:"metadata"`
	Tags     []string        `json:"tags"`
	Nested   *NestedPayload  `json:"nested"`
}

// MetadataPayload 元数据 Payload（避免使用 map[string]interface{}）
type MetadataPayload struct {
	Key1 string `json:"key1"`
	Key2 int    `json:"key2"`
}

// NestedPayload 嵌套的 Payload 结构
type NestedPayload struct {
	Field1 string `json:"field1"`
	Field2 int    `json:"field2"`
}

// CreateTestPayload 创建测试用的 Payload
func (h *TestHelper) CreateTestPayload() TestPayload {
	return TestPayload{
		Title:     "Test Title",
		CreatedBy: "user-001",
		Count:     42,
		CreatedAt: time.Now(),
	}
}

// CreateComplexPayload 创建复杂的测试 Payload
func (h *TestHelper) CreateComplexPayload() ComplexPayload {
	return ComplexPayload{
		ID:   "complex-001",
		Name: "Complex Test",
		Metadata: MetadataPayload{
			Key1: "value1",
			Key2: 123,
		},
		Tags: []string{"tag1", "tag2", "tag3"},
		Nested: &NestedPayload{
			Field1: "nested-value",
			Field2: 999,
		},
	}
}

// CreateEnvelope 创建测试用的 Envelope
func (h *TestHelper) CreateEnvelope(eventType, aggregateID, tenantID string, payload []byte) *jxtevent.Envelope {
	return &jxtevent.Envelope{
		EventType:   eventType,
		AggregateID: aggregateID,
		TenantID:    tenantID,
		Payload:     payload,
	}
}
