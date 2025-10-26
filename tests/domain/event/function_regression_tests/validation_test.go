package function_regression_tests

import (
	"testing"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
)

// TestValidateConsistency_BaseEvent_Success 测试基础事件一致性校验成功
func TestValidateConsistency_BaseEvent_Success(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建基础事件
	event := helper.CreateBaseDomainEvent("Archive.Created", "archive-123", "Archive", nil)

	// 创建匹配的 Envelope
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		"",
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该成功
	helper.AssertNoError(err, "Validation should succeed for matching envelope and event")
}

// TestValidateConsistency_EnterpriseEvent_Success 测试企业级事件一致性校验成功
func TestValidateConsistency_EnterpriseEvent_Success(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建企业级事件
	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-123", "Order", nil)
	event.SetTenantId("tenant-001")

	// 创建匹配的 Envelope
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		event.GetTenantId(),
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该成功
	helper.AssertNoError(err, "Validation should succeed for matching envelope and enterprise event")
}

// TestValidateConsistency_NilEnvelope 测试 nil Envelope
func TestValidateConsistency_NilEnvelope(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证一致性
	err := jxtevent.ValidateConsistency(nil, event)

	// 应该返回错误
	helper.AssertError(err, "Should return error for nil envelope")
	helper.AssertErrorContains(err, "envelope is nil", "Error should mention nil envelope")
}

// TestValidateConsistency_NilEvent 测试 nil Event
func TestValidateConsistency_NilEvent(t *testing.T) {
	helper := NewTestHelper(t)

	envelope := helper.CreateEnvelope("Test.Event", "test-123", "", []byte("{}"))

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, nil)

	// 应该返回错误
	helper.AssertError(err, "Should return error for nil event")
	helper.AssertErrorContains(err, "event is nil", "Error should mention nil event")
}

// TestValidateConsistency_EventTypeMismatch 测试 EventType 不匹配
func TestValidateConsistency_EventTypeMismatch(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Archive.Created", "archive-123", "Archive", nil)

	// 创建不匹配的 Envelope（EventType 不同）
	envelope := helper.CreateEnvelope(
		"Archive.Updated", // 不同的 EventType
		event.GetAggregateID(),
		"",
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该返回错误
	helper.AssertError(err, "Should return error for eventType mismatch")
	helper.AssertErrorContains(err, "eventType mismatch", "Error should mention eventType mismatch")
	helper.AssertErrorContains(err, "Archive.Updated", "Error should contain envelope EventType")
	helper.AssertErrorContains(err, "Archive.Created", "Error should contain event EventType")
}

// TestValidateConsistency_AggregateIDMismatch 测试 AggregateID 不匹配
func TestValidateConsistency_AggregateIDMismatch(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Archive.Created", "archive-123", "Archive", nil)

	// 创建不匹配的 Envelope（AggregateID 不同）
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		"archive-456", // 不同的 AggregateID
		"",
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该返回错误
	helper.AssertError(err, "Should return error for aggregateID mismatch")
	helper.AssertErrorContains(err, "aggregateID mismatch", "Error should mention aggregateID mismatch")
	helper.AssertErrorContains(err, "archive-456", "Error should contain envelope AggregateID")
	helper.AssertErrorContains(err, "archive-123", "Error should contain event AggregateID")
}

// TestValidateConsistency_TenantIDMismatch 测试 TenantID 不匹配
func TestValidateConsistency_TenantIDMismatch(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建企业级事件
	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-123", "Order", nil)
	event.SetTenantId("tenant-001")

	// 创建不匹配的 Envelope（TenantID 不同）
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		"tenant-002", // 不同的 TenantID
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该返回错误
	helper.AssertError(err, "Should return error for tenantID mismatch")
	helper.AssertErrorContains(err, "tenantID mismatch", "Error should mention tenantID mismatch")
	helper.AssertErrorContains(err, "tenant-002", "Error should contain envelope TenantID")
	helper.AssertErrorContains(err, "tenant-001", "Error should contain event TenantID")
}

// TestValidateConsistency_BaseEventWithTenantIDInEnvelope 测试基础事件但 Envelope 有 TenantID
func TestValidateConsistency_BaseEventWithTenantIDInEnvelope(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建基础事件（不是企业级事件）
	event := helper.CreateBaseDomainEvent("Archive.Created", "archive-123", "Archive", nil)

	// 创建带 TenantID 的 Envelope
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		"tenant-001", // Envelope 有 TenantID
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该成功（基础事件不校验 TenantID）
	helper.AssertNoError(err, "Validation should succeed for BaseEvent even if envelope has TenantID")
}

// TestValidateConsistency_EnterpriseEventWithEmptyTenantID 测试企业级事件但 TenantID 为空
func TestValidateConsistency_EnterpriseEventWithEmptyTenantID(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建企业级事件（TenantID 设置为空）
	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-123", "Order", nil)
	event.SetTenantId("")

	// 创建匹配的 Envelope
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		"", // 空 TenantID
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该成功
	helper.AssertNoError(err, "Validation should succeed for empty TenantID")
}

// TestValidateConsistency_EnterpriseEventWithDefaultTenantID 测试企业级事件默认 TenantID
func TestValidateConsistency_EnterpriseEventWithDefaultTenantID(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建企业级事件（使用默认 TenantID "*"）
	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-123", "Order", nil)

	// 创建匹配的 Envelope
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		"*", // 默认 TenantID
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该成功
	helper.AssertNoError(err, "Validation should succeed for default TenantID '*'")
}

// TestValidateConsistency_MultipleFieldsMismatch 测试多个字段不匹配
func TestValidateConsistency_MultipleFieldsMismatch(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-123", "Order", nil)
	event.SetTenantId("tenant-001")

	// 创建完全不匹配的 Envelope
	envelope := helper.CreateEnvelope(
		"Order.Updated", // 不同的 EventType
		"order-456",     // 不同的 AggregateID
		"tenant-002",    // 不同的 TenantID
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该返回错误（第一个不匹配的字段）
	helper.AssertError(err, "Should return error for multiple mismatches")
	// 应该返回第一个不匹配的错误（eventType）
	helper.AssertErrorContains(err, "eventType mismatch", "Should report first mismatch")
}

// TestValidateConsistency_CompleteWorkflow 测试完整工作流
func TestValidateConsistency_CompleteWorkflow(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建完整的企业级事件
	payload := TestPayload{
		Title:     "Order Created",
		CreatedBy: "customer",
		Count:     1,
	}
	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-12345", "Order", payload)
	event.SetTenantId("tenant-acme-corp")
	event.SetCorrelationId("workflow-001")
	event.SetCausationId("cart-checkout-123")
	event.SetTraceId("trace-xyz-789")

	// 序列化 Payload
	payloadBytes, err := jxtevent.MarshalPayload(payload)
	helper.AssertNoError(err, "MarshalPayload should succeed")

	// 创建匹配的 Envelope
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		event.GetTenantId(),
		payloadBytes,
	)

	// 验证一致性
	err = jxtevent.ValidateConsistency(envelope, event)

	// 应该成功
	helper.AssertNoError(err, "Complete workflow validation should succeed")
}

// TestValidateConsistency_CaseSensitivity 测试大小写敏感性
func TestValidateConsistency_CaseSensitivity(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Archive.Created", "archive-123", "Archive", nil)

	// 创建大小写不同的 Envelope
	envelope := helper.CreateEnvelope(
		"archive.created", // 小写
		event.GetAggregateID(),
		"",
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该返回错误（大小写敏感）
	helper.AssertError(err, "Should return error for case mismatch")
	helper.AssertErrorContains(err, "eventType mismatch", "Error should mention eventType mismatch")
}

// TestValidateConsistency_WhitespaceDifference 测试空格差异
func TestValidateConsistency_WhitespaceDifference(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Archive.Created", "archive-123", "Archive", nil)

	// 创建带空格的 Envelope
	envelope := helper.CreateEnvelope(
		" Archive.Created ", // 前后有空格
		event.GetAggregateID(),
		"",
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该返回错误（不会自动 trim）
	helper.AssertError(err, "Should return error for whitespace difference")
	helper.AssertErrorContains(err, "eventType mismatch", "Error should mention eventType mismatch")
}

// TestValidateConsistency_EmptyStrings 测试空字符串
func TestValidateConsistency_EmptyStrings(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建空字符串的事件
	event := jxtevent.NewBaseDomainEvent("", "", "", nil)

	// 创建匹配的 Envelope
	envelope := helper.CreateEnvelope("", "", "", []byte("{}"))

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该成功（空字符串也是有效的匹配）
	helper.AssertNoError(err, "Validation should succeed for empty strings")
}

// TestValidateConsistency_SpecialCharacters 测试特殊字符
func TestValidateConsistency_SpecialCharacters(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建包含特殊字符的事件
	event := helper.CreateBaseDomainEvent(
		"Archive.Created@v2",
		"archive-123-中文-🎉",
		"Archive",
		nil,
	)

	// 创建匹配的 Envelope
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		"",
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该成功
	helper.AssertNoError(err, "Validation should succeed for special characters")
}

// TestValidateConsistency_LongStrings 测试长字符串
func TestValidateConsistency_LongStrings(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建很长的字符串
	longEventType := "Archive.Created.With.Very.Long.Name.That.Contains.Many.Dots.And.Words"
	longAggregateID := "archive-123456789012345678901234567890123456789012345678901234567890"

	event := helper.CreateBaseDomainEvent(longEventType, longAggregateID, "Archive", nil)

	// 创建匹配的 Envelope
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		"",
		[]byte("{}"),
	)

	// 验证一致性
	err := jxtevent.ValidateConsistency(envelope, event)

	// 应该成功
	helper.AssertNoError(err, "Validation should succeed for long strings")
}
