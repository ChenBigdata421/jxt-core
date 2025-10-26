package function_regression_tests

import (
	"sync"
	"testing"
	"time"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
)

// TestEnterpriseDomainEvent_Creation 测试企业级领域事件创建
func TestEnterpriseDomainEvent_Creation(t *testing.T) {
	helper := NewTestHelper(t)

	payload := helper.CreateTestPayload()
	event := helper.CreateEnterpriseDomainEvent("Archive.Created", "archive-123", "Archive", payload)

	// 验证基本字段（继承自 BaseDomainEvent）
	helper.AssertNotEmpty(event.EventID, "EventID should be generated")
	helper.AssertEqual("Archive.Created", event.EventType, "EventType should match")
	helper.AssertEqual("archive-123", event.AggregateID, "AggregateID should match")
	helper.AssertEqual("Archive", event.AggregateType, "AggregateType should match")
	helper.AssertEqual(1, event.Version, "Version should be 1")
	helper.AssertNotNil(event.Payload, "Payload should not be nil")

	// 验证企业级字段
	helper.AssertEqual("*", event.TenantId, "Default TenantId should be '*'")
	helper.AssertEqual("", event.CorrelationId, "Default CorrelationId should be empty")
	helper.AssertEqual("", event.CausationId, "Default CausationId should be empty")
	helper.AssertEqual("", event.TraceId, "Default TraceId should be empty")
}

// TestEnterpriseDomainEvent_DefaultTenantId 测试默认租户ID
func TestEnterpriseDomainEvent_DefaultTenantId(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证默认租户ID是 "*"（全局租户）
	helper.AssertEqual("*", event.TenantId, "Default TenantId should be '*'")
	helper.AssertEqual("*", event.GetTenantId(), "GetTenantId should return '*'")
}

// TestEnterpriseDomainEvent_SetTenantId 测试设置租户ID
func TestEnterpriseDomainEvent_SetTenantId(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 设置租户ID
	event.SetTenantId("tenant-001")

	// 验证
	helper.AssertEqual("tenant-001", event.TenantId, "TenantId should be set")
	helper.AssertEqual("tenant-001", event.GetTenantId(), "GetTenantId should return set value")
}

// TestEnterpriseDomainEvent_MultiTenantScenario 测试多租户场景
func TestEnterpriseDomainEvent_MultiTenantScenario(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建不同租户的事件
	event1 := helper.CreateEnterpriseDomainEvent("Order.Created", "order-1", "Order", nil)
	event1.SetTenantId("tenant-001")

	event2 := helper.CreateEnterpriseDomainEvent("Order.Created", "order-2", "Order", nil)
	event2.SetTenantId("tenant-002")

	event3 := helper.CreateEnterpriseDomainEvent("Order.Created", "order-3", "Order", nil)
	// event3 使用默认租户 "*"

	// 验证租户隔离
	helper.AssertEqual("tenant-001", event1.GetTenantId(), "Event1 should belong to tenant-001")
	helper.AssertEqual("tenant-002", event2.GetTenantId(), "Event2 should belong to tenant-002")
	helper.AssertEqual("*", event3.GetTenantId(), "Event3 should belong to global tenant")

	helper.AssertNotEqual(event1.GetTenantId(), event2.GetTenantId(), "Different tenants should have different IDs")
}

// TestEnterpriseDomainEvent_CorrelationId 测试业务关联ID
func TestEnterpriseDomainEvent_CorrelationId(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证默认值
	helper.AssertEqual("", event.GetCorrelationId(), "Default CorrelationId should be empty")

	// 设置 CorrelationId
	event.SetCorrelationId("workflow-12345")

	// 验证
	helper.AssertEqual("workflow-12345", event.CorrelationId, "CorrelationId should be set")
	helper.AssertEqual("workflow-12345", event.GetCorrelationId(), "GetCorrelationId should return set value")
}

// TestEnterpriseDomainEvent_CausationId 测试因果事件ID
func TestEnterpriseDomainEvent_CausationId(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证默认值
	helper.AssertEqual("", event.GetCausationId(), "Default CausationId should be empty")

	// 设置 CausationId
	event.SetCausationId("trigger-event-67890")

	// 验证
	helper.AssertEqual("trigger-event-67890", event.CausationId, "CausationId should be set")
	helper.AssertEqual("trigger-event-67890", event.GetCausationId(), "GetCausationId should return set value")
}

// TestEnterpriseDomainEvent_TraceId 测试分布式追踪ID
func TestEnterpriseDomainEvent_TraceId(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证默认值
	helper.AssertEqual("", event.GetTraceId(), "Default TraceId should be empty")

	// 设置 TraceId
	event.SetTraceId("trace-abc-def-123")

	// 验证
	helper.AssertEqual("trace-abc-def-123", event.TraceId, "TraceId should be set")
	helper.AssertEqual("trace-abc-def-123", event.GetTraceId(), "GetTraceId should return set value")
}

// TestEnterpriseDomainEvent_ObservabilityFields 测试所有可观测性字段
func TestEnterpriseDomainEvent_ObservabilityFields(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-123", "Order", nil)

	// 设置所有可观测性字段
	event.SetTenantId("tenant-001")
	event.SetCorrelationId("workflow-12345")
	event.SetCausationId("trigger-event-67890")
	event.SetTraceId("trace-abc-def-123")

	// 验证所有字段
	helper.AssertEqual("tenant-001", event.GetTenantId(), "TenantId should be set")
	helper.AssertEqual("workflow-12345", event.GetCorrelationId(), "CorrelationId should be set")
	helper.AssertEqual("trigger-event-67890", event.GetCausationId(), "CausationId should be set")
	helper.AssertEqual("trace-abc-def-123", event.GetTraceId(), "TraceId should be set")
}

// TestEnterpriseDomainEvent_EventCausationChain 测试事件因果链
func TestEnterpriseDomainEvent_EventCausationChain(t *testing.T) {
	helper := NewTestHelper(t)

	// 模拟事件因果链：Event1 -> Event2 -> Event3
	event1 := helper.CreateEnterpriseDomainEvent("Order.Created", "order-123", "Order", nil)
	event1.SetCorrelationId("workflow-001")

	event2 := helper.CreateEnterpriseDomainEvent("Payment.Processed", "payment-456", "Payment", nil)
	event2.SetCorrelationId("workflow-001")           // 同一个业务流程
	event2.SetCausationId(event1.GetEventID())        // 由 event1 触发

	event3 := helper.CreateEnterpriseDomainEvent("Shipment.Created", "shipment-789", "Shipment", nil)
	event3.SetCorrelationId("workflow-001")           // 同一个业务流程
	event3.SetCausationId(event2.GetEventID())        // 由 event2 触发

	// 验证因果链
	helper.AssertEqual("workflow-001", event1.GetCorrelationId(), "All events should have same CorrelationId")
	helper.AssertEqual("workflow-001", event2.GetCorrelationId(), "All events should have same CorrelationId")
	helper.AssertEqual("workflow-001", event3.GetCorrelationId(), "All events should have same CorrelationId")

	helper.AssertEqual(event1.GetEventID(), event2.GetCausationId(), "Event2 should be caused by Event1")
	helper.AssertEqual(event2.GetEventID(), event3.GetCausationId(), "Event3 should be caused by Event2")
}

// TestEnterpriseDomainEvent_InterfaceCompliance 测试接口实现
func TestEnterpriseDomainEvent_InterfaceCompliance(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证实现了 BaseEvent 接口
	var _ jxtevent.BaseEvent = event
	// 验证实现了 EnterpriseEvent 接口
	var _ jxtevent.EnterpriseEvent = event

	helper.AssertTrue(true, "EnterpriseDomainEvent should implement both BaseEvent and EnterpriseEvent interfaces")
}

// TestEnterpriseDomainEvent_InheritedMethods 测试继承的方法
func TestEnterpriseDomainEvent_InheritedMethods(t *testing.T) {
	helper := NewTestHelper(t)

	payload := helper.CreateTestPayload()
	event := helper.CreateEnterpriseDomainEvent("Archive.Created", "archive-123", "Archive", payload)

	// 测试继承自 BaseDomainEvent 的所有方法
	helper.AssertNotEmpty(event.GetEventID(), "GetEventID should work")
	helper.AssertEqual("Archive.Created", event.GetEventType(), "GetEventType should work")
	helper.AssertEqual("archive-123", event.GetAggregateID(), "GetAggregateID should work")
	helper.AssertEqual("Archive", event.GetAggregateType(), "GetAggregateType should work")
	helper.AssertEqual(1, event.GetVersion(), "GetVersion should work")
	helper.AssertNotNil(event.GetPayload(), "GetPayload should work")
	helper.AssertWithinDuration(time.Now(), event.GetOccurredAt(), 5*time.Second, "GetOccurredAt should work")
}

// TestEnterpriseDomainEvent_ConcurrentFieldAccess 测试并发字段访问
func TestEnterpriseDomainEvent_ConcurrentFieldAccess(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)

	var wg sync.WaitGroup
	iterations := 100

	// 并发设置和读取字段
	for i := 0; i < iterations; i++ {
		wg.Add(4)

		go func(idx int) {
			defer wg.Done()
			event.SetTenantId("tenant-" + string(rune(idx)))
		}(i)

		go func(idx int) {
			defer wg.Done()
			event.SetCorrelationId("correlation-" + string(rune(idx)))
		}(i)

		go func(idx int) {
			defer wg.Done()
			event.SetCausationId("causation-" + string(rune(idx)))
		}(i)

		go func(idx int) {
			defer wg.Done()
			event.SetTraceId("trace-" + string(rune(idx)))
		}(i)
	}

	wg.Wait()

	// 验证字段已被设置（具体值不确定，因为并发）
	helper.AssertNotEmpty(event.GetTenantId(), "TenantId should be set")
	helper.AssertNotEmpty(event.GetCorrelationId(), "CorrelationId should be set")
	helper.AssertNotEmpty(event.GetCausationId(), "CausationId should be set")
	helper.AssertNotEmpty(event.GetTraceId(), "TraceId should be set")
}

// TestEnterpriseDomainEvent_EmptyObservabilityFields 测试空的可观测性字段
func TestEnterpriseDomainEvent_EmptyObservabilityFields(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 设置空字符串
	event.SetTenantId("")
	event.SetCorrelationId("")
	event.SetCausationId("")
	event.SetTraceId("")

	// 验证可以设置空字符串
	helper.AssertEqual("", event.GetTenantId(), "TenantId can be empty")
	helper.AssertEqual("", event.GetCorrelationId(), "CorrelationId can be empty")
	helper.AssertEqual("", event.GetCausationId(), "CausationId can be empty")
	helper.AssertEqual("", event.GetTraceId(), "TraceId can be empty")
}

// TestEnterpriseDomainEvent_CompleteWorkflow 测试完整工作流
func TestEnterpriseDomainEvent_CompleteWorkflow(t *testing.T) {
	helper := NewTestHelper(t)

	// 模拟完整的业务流程
	payload := map[string]interface{}{
		"orderId":    "order-12345",
		"customerId": "customer-67890",
		"amount":     99.99,
	}

	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-12345", "Order", payload)

	// 设置租户信息
	event.SetTenantId("tenant-acme-corp")

	// 设置可观测性信息
	event.SetCorrelationId("checkout-workflow-001")
	event.SetCausationId("cart-checkout-event-123")
	event.SetTraceId("trace-xyz-789")

	// 验证所有字段
	helper.AssertNotEmpty(event.GetEventID(), "EventID should be generated")
	helper.AssertEqual("Order.Created", event.GetEventType(), "EventType should match")
	helper.AssertEqual("order-12345", event.GetAggregateID(), "AggregateID should match")
	helper.AssertEqual("Order", event.GetAggregateType(), "AggregateType should match")
	helper.AssertEqual("tenant-acme-corp", event.GetTenantId(), "TenantId should match")
	helper.AssertEqual("checkout-workflow-001", event.GetCorrelationId(), "CorrelationId should match")
	helper.AssertEqual("cart-checkout-event-123", event.GetCausationId(), "CausationId should match")
	helper.AssertEqual("trace-xyz-789", event.GetTraceId(), "TraceId should match")
	helper.AssertNotNil(event.GetPayload(), "Payload should not be nil")
}

// TestEnterpriseDomainEvent_FieldIndependence 测试字段独立性
func TestEnterpriseDomainEvent_FieldIndependence(t *testing.T) {
	helper := NewTestHelper(t)

	event1 := helper.CreateEnterpriseDomainEvent("Test.Event", "test-1", "Test", nil)
	event2 := helper.CreateEnterpriseDomainEvent("Test.Event", "test-2", "Test", nil)

	// 设置 event1 的字段
	event1.SetTenantId("tenant-001")
	event1.SetCorrelationId("correlation-001")
	event1.SetCausationId("causation-001")
	event1.SetTraceId("trace-001")

	// 设置 event2 的字段
	event2.SetTenantId("tenant-002")
	event2.SetCorrelationId("correlation-002")
	event2.SetCausationId("causation-002")
	event2.SetTraceId("trace-002")

	// 验证两个事件的字段完全独立
	helper.AssertNotEqual(event1.GetTenantId(), event2.GetTenantId(), "TenantIds should be independent")
	helper.AssertNotEqual(event1.GetCorrelationId(), event2.GetCorrelationId(), "CorrelationIds should be independent")
	helper.AssertNotEqual(event1.GetCausationId(), event2.GetCausationId(), "CausationIds should be independent")
	helper.AssertNotEqual(event1.GetTraceId(), event2.GetTraceId(), "TraceIds should be independent")
}

