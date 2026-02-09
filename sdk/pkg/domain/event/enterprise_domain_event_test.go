package event

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewEnterpriseDomainEvent(t *testing.T) {
	// 准备测试数据
	eventType := "TestEvent"
	aggregateID := "test-123"
	aggregateType := "TestAggregate"
	payload := map[string]interface{}{
		"key": "value",
	}

	// 执行
	event := NewEnterpriseDomainEvent(eventType, aggregateID, aggregateType, payload)

	// 验证基础字段
	assert.NotEmpty(t, event.EventID, "EventID should not be empty")
	assert.Equal(t, eventType, event.EventType)
	assert.Equal(t, aggregateID, event.AggregateID)
	assert.Equal(t, aggregateType, event.AggregateType)
	assert.Equal(t, 1, event.Version)
	assert.Equal(t, payload, event.Payload)
	assert.WithinDuration(t, time.Now(), event.OccurredAt, time.Second)

	// 验证企业级字段
	assert.Equal(t, 0, event.TenantId, "Default TenantId should be 0")
	assert.Empty(t, event.CorrelationId)
	assert.Empty(t, event.CausationId)
	assert.Empty(t, event.TraceId)
}

func TestEnterpriseDomainEvent_TenantId(t *testing.T) {
	event := NewEnterpriseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	// 测试默认值
	assert.Equal(t, 0, event.GetTenantId())

	// 测试设置租户ID
	event.SetTenantId(1)
	assert.Equal(t, int(1), event.GetTenantId())
}

func TestEnterpriseDomainEvent_ObservabilityFields(t *testing.T) {
	event := NewEnterpriseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	// 测试CorrelationId
	assert.Empty(t, event.GetCorrelationId())
	event.SetCorrelationId("correlation-123")
	assert.Equal(t, "correlation-123", event.GetCorrelationId())

	// 测试CausationId
	assert.Empty(t, event.GetCausationId())
	event.SetCausationId("causation-456")
	assert.Equal(t, "causation-456", event.GetCausationId())

	// 测试TraceId
	assert.Empty(t, event.GetTraceId())
	event.SetTraceId("trace-789")
	assert.Equal(t, "trace-789", event.GetTraceId())
}

func TestEnterpriseDomainEvent_ImplementsEnterpriseEvent(t *testing.T) {
	// 验证EnterpriseDomainEvent实现了EnterpriseEvent接口
	var _ EnterpriseEvent = (*EnterpriseDomainEvent)(nil)
	// 同时也实现了BaseEvent接口
	var _ BaseEvent = (*EnterpriseDomainEvent)(nil)
}

func TestEnterpriseDomainEvent_CompleteWorkflow(t *testing.T) {
	// 模拟完整的企业级事件工作流
	event := NewEnterpriseDomainEvent(
		"Archive.Created",
		"archive-123",
		"Archive",
		map[string]interface{}{
			"title":     "Test Archive",
			"createdBy": "user-001",
		},
	)

	// 设置租户ID
	event.SetTenantId(1)

	// 设置可观测性字段
	event.SetCorrelationId("workflow-123")
	event.SetCausationId("trigger-event-456")
	event.SetTraceId("trace-789")

	// 验证所有字段
	assert.Equal(t, "Archive.Created", event.GetEventType())
	assert.Equal(t, "archive-123", event.GetAggregateID())
	assert.Equal(t, "Archive", event.GetAggregateType())
	assert.Equal(t, 1, event.GetTenantId())
	assert.Equal(t, "workflow-123", event.GetCorrelationId())
	assert.Equal(t, "trigger-event-456", event.GetCausationId())
	assert.Equal(t, "trace-789", event.GetTraceId())
}

