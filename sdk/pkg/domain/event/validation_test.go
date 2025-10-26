package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateConsistency_Success_BaseEvent(t *testing.T) {
	// 准备测试数据
	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	envelope := &Envelope{
		EventType:   event.GetEventType(),
		AggregateID: event.GetAggregateID(),
		TenantID:    "",
		Payload:     []byte("{}"),
	}

	// 执行
	err := ValidateConsistency(envelope, event)

	// 验证
	assert.NoError(t, err)
}

func TestValidateConsistency_Success_EnterpriseEvent(t *testing.T) {
	// 准备测试数据
	event := NewEnterpriseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)
	event.SetTenantId("tenant-001")

	envelope := &Envelope{
		EventType:   event.GetEventType(),
		AggregateID: event.GetAggregateID(),
		TenantID:    event.GetTenantId(),
		Payload:     []byte("{}"),
	}

	// 执行
	err := ValidateConsistency(envelope, event)

	// 验证
	assert.NoError(t, err)
}

func TestValidateConsistency_NilEnvelope(t *testing.T) {
	// 准备测试数据
	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	// 执行
	err := ValidateConsistency(nil, event)

	// 验证
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "envelope is nil")
}

func TestValidateConsistency_NilEvent(t *testing.T) {
	// 准备测试数据
	envelope := &Envelope{
		EventType:   "TestEvent",
		AggregateID: "test-123",
		TenantID:    "",
		Payload:     []byte("{}"),
	}

	// 执行
	err := ValidateConsistency(envelope, nil)

	// 验证
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event is nil")
}

func TestValidateConsistency_EventTypeMismatch(t *testing.T) {
	// 准备测试数据
	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	envelope := &Envelope{
		EventType:   "DifferentEvent", // 不匹配
		AggregateID: event.GetAggregateID(),
		TenantID:    "",
		Payload:     []byte("{}"),
	}

	// 执行
	err := ValidateConsistency(envelope, event)

	// 验证
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventType mismatch")
	assert.Contains(t, err.Error(), "DifferentEvent")
	assert.Contains(t, err.Error(), "TestEvent")
}

func TestValidateConsistency_AggregateIDMismatch(t *testing.T) {
	// 准备测试数据
	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	envelope := &Envelope{
		EventType:   event.GetEventType(),
		AggregateID: "different-id", // 不匹配
		TenantID:    "",
		Payload:     []byte("{}"),
	}

	// 执行
	err := ValidateConsistency(envelope, event)

	// 验证
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aggregateID mismatch")
	assert.Contains(t, err.Error(), "different-id")
	assert.Contains(t, err.Error(), "test-123")
}

func TestValidateConsistency_TenantIDMismatch(t *testing.T) {
	// 准备测试数据
	event := NewEnterpriseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)
	event.SetTenantId("tenant-001")

	envelope := &Envelope{
		EventType:   event.GetEventType(),
		AggregateID: event.GetAggregateID(),
		TenantID:    "tenant-002", // 不匹配
		Payload:     []byte("{}"),
	}

	// 执行
	err := ValidateConsistency(envelope, event)

	// 验证
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tenantID mismatch")
	assert.Contains(t, err.Error(), "tenant-002")
	assert.Contains(t, err.Error(), "tenant-001")
}

func TestValidateConsistency_CompleteWorkflow(t *testing.T) {
	// 模拟完整的一致性校验工作流

	// 1. 创建企业级事件
	event := NewEnterpriseDomainEvent(
		"Archive.Created",
		"archive-123",
		"Archive",
		map[string]interface{}{
			"title": "Test Archive",
		},
	)
	event.SetTenantId("tenant-001")

	// 2. 创建匹配的Envelope
	envelope := &Envelope{
		EventType:   "Archive.Created",
		AggregateID: "archive-123",
		TenantID:    "tenant-001",
		Payload:     []byte(`{"title":"Test Archive"}`),
	}

	// 3. 验证一致性
	err := ValidateConsistency(envelope, event)
	assert.NoError(t, err)

	// 4. 修改Envelope的TenantID，应该失败
	envelope.TenantID = "tenant-002"
	err = ValidateConsistency(envelope, event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tenantID mismatch")
}

func TestValidateConsistency_BaseEventWithTenantIDInEnvelope(t *testing.T) {
	// 测试：BaseEvent不检查TenantID
	event := NewBaseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	envelope := &Envelope{
		EventType:   event.GetEventType(),
		AggregateID: event.GetAggregateID(),
		TenantID:    "tenant-001", // BaseEvent不会检查这个字段
		Payload:     []byte("{}"),
	}

	// 执行
	err := ValidateConsistency(envelope, event)

	// 验证 - 应该成功，因为BaseEvent不检查TenantID
	assert.NoError(t, err)
}

