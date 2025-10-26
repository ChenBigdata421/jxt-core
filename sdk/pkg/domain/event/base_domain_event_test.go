package event

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewBaseDomainEvent(t *testing.T) {
	// 准备测试数据
	eventType := "TestEvent"
	aggregateID := "test-123"
	aggregateType := "TestAggregate"
	payload := map[string]interface{}{
		"key": "value",
	}

	// 执行
	event := NewBaseDomainEvent(eventType, aggregateID, aggregateType, payload)

	// 验证
	assert.NotEmpty(t, event.EventID, "EventID should not be empty")
	assert.Equal(t, eventType, event.EventType)
	assert.Equal(t, aggregateID, event.AggregateID)
	assert.Equal(t, aggregateType, event.AggregateType)
	assert.Equal(t, 1, event.Version)
	assert.Equal(t, payload, event.Payload)
	assert.WithinDuration(t, time.Now(), event.OccurredAt, time.Second)
}

func TestConvertAggregateIDToString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "string input",
			input:    "test-123",
			expected: "test-123",
		},
		{
			name:     "int64 input",
			input:    int64(12345),
			expected: "12345",
		},
		{
			name:     "int input",
			input:    123,
			expected: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertAggregateIDToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseDomainEvent_Getters(t *testing.T) {
	// 准备测试数据
	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", map[string]string{"key": "value"})

	// 验证所有getter方法
	assert.NotEmpty(t, event.GetEventID())
	assert.Equal(t, "TestEvent", event.GetEventType())
	assert.NotZero(t, event.GetOccurredAt())
	assert.Equal(t, 1, event.GetVersion())
	assert.Equal(t, "test-123", event.GetAggregateID())
	assert.Equal(t, "TestAggregate", event.GetAggregateType())
	assert.NotNil(t, event.GetPayload())
}

func TestBaseDomainEvent_ImplementsBaseEvent(t *testing.T) {
	// 验证BaseDomainEvent实现了BaseEvent接口
	var _ BaseEvent = (*BaseDomainEvent)(nil)
}

