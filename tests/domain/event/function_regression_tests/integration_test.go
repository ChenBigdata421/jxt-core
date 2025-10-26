package function_regression_tests

import (
	"sync"
	"testing"
	"time"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
)

// TestIntegration_CompleteEventLifecycle 测试完整的事件生命周期
func TestIntegration_CompleteEventLifecycle(t *testing.T) {
	helper := NewTestHelper(t)

	// 1. 创建事件
	payload := helper.CreateTestPayload()
	event := helper.CreateEnterpriseDomainEvent("Order.Created", "order-12345", "Order", payload)
	event.SetTenantId("tenant-001")
	event.SetCorrelationId("workflow-001")
	event.SetCausationId("cart-checkout-123")
	event.SetTraceId("trace-xyz-789")

	// 2. 验证事件创建
	helper.AssertNotEmpty(event.GetEventID(), "EventID should be generated")
	helper.AssertEqual("Order.Created", event.GetEventType(), "EventType should match")

	// 3. 序列化 Payload
	payloadBytes, err := jxtevent.MarshalPayload(event.GetPayload())
	helper.AssertNoError(err, "MarshalPayload should succeed")

	// 4. 创建 Envelope
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		event.GetTenantId(),
		payloadBytes,
	)

	// 5. 验证一致性
	err = jxtevent.ValidateConsistency(envelope, event)
	helper.AssertNoError(err, "ValidateConsistency should succeed")

	// 6. 反序列化 Payload
	result, err := jxtevent.UnmarshalPayload[TestPayload](event)
	helper.AssertNoError(err, "UnmarshalPayload should succeed")
	helper.AssertEqual(payload.Title, result.Title, "Payload should match")
}

// TestIntegration_EventCausationChain 测试事件因果链集成
func TestIntegration_EventCausationChain(t *testing.T) {
	helper := NewTestHelper(t)

	correlationID := "workflow-order-processing-001"

	// 事件1: 订单创建
	event1 := helper.CreateEnterpriseDomainEvent("Order.Created", "order-123", "Order", TestPayload{
		Title:     "Order Created",
		CreatedBy: "customer",
		Count:     1,
	})
	event1.SetTenantId("tenant-001")
	event1.SetCorrelationId(correlationID)
	event1.SetTraceId("trace-001")

	// 事件2: 支付处理（由订单创建触发）
	event2 := helper.CreateEnterpriseDomainEvent("Payment.Processed", "payment-456", "Payment", TestPayload{
		Title:     "Payment Processed",
		CreatedBy: "payment-gateway",
		Count:     1,
	})
	event2.SetTenantId("tenant-001")
	event2.SetCorrelationId(correlationID)
	event2.SetCausationId(event1.GetEventID())
	event2.SetTraceId("trace-001")

	// 事件3: 发货创建（由支付处理触发）
	event3 := helper.CreateEnterpriseDomainEvent("Shipment.Created", "shipment-789", "Shipment", TestPayload{
		Title:     "Shipment Created",
		CreatedBy: "warehouse",
		Count:     1,
	})
	event3.SetTenantId("tenant-001")
	event3.SetCorrelationId(correlationID)
	event3.SetCausationId(event2.GetEventID())
	event3.SetTraceId("trace-001")

	// 验证因果链
	helper.AssertEqual(correlationID, event1.GetCorrelationId(), "All events should have same CorrelationId")
	helper.AssertEqual(correlationID, event2.GetCorrelationId(), "All events should have same CorrelationId")
	helper.AssertEqual(correlationID, event3.GetCorrelationId(), "All events should have same CorrelationId")

	helper.AssertEqual(event1.GetEventID(), event2.GetCausationId(), "Event2 caused by Event1")
	helper.AssertEqual(event2.GetEventID(), event3.GetCausationId(), "Event3 caused by Event2")

	// 验证租户隔离
	helper.AssertEqual("tenant-001", event1.GetTenantId(), "All events should belong to same tenant")
	helper.AssertEqual("tenant-001", event2.GetTenantId(), "All events should belong to same tenant")
	helper.AssertEqual("tenant-001", event3.GetTenantId(), "All events should belong to same tenant")
}

// TestIntegration_MultiTenantEventProcessing 测试多租户事件处理
func TestIntegration_MultiTenantEventProcessing(t *testing.T) {
	helper := NewTestHelper(t)

	tenants := []string{"tenant-001", "tenant-002", "tenant-003"}
	eventsPerTenant := 10

	var allEvents []*jxtevent.EnterpriseDomainEvent

	// 为每个租户创建事件
	for _, tenantID := range tenants {
		for i := 0; i < eventsPerTenant; i++ {
			event := helper.CreateEnterpriseDomainEvent(
				"Order.Created",
				"order-"+tenantID+"-"+string(rune(i)),
				"Order",
				nil,
			)
			event.SetTenantId(tenantID)
			allEvents = append(allEvents, event)
		}
	}

	// 验证事件总数
	helper.AssertEqual(len(tenants)*eventsPerTenant, len(allEvents), "Should have correct number of events")

	// 验证每个租户的事件
	for _, tenantID := range tenants {
		count := 0
		for _, event := range allEvents {
			if event.GetTenantId() == tenantID {
				count++
			}
		}
		helper.AssertEqual(eventsPerTenant, count, "Each tenant should have correct number of events")
	}
}

// TestIntegration_ConcurrentEventCreationAndValidation 测试并发事件创建和验证
func TestIntegration_ConcurrentEventCreationAndValidation(t *testing.T) {
	helper := NewTestHelper(t)

	goroutines := 50
	eventsPerGoroutine := 20

	var wg sync.WaitGroup
	var mu sync.Mutex
	var allEvents []*jxtevent.EnterpriseDomainEvent
	var errors []error

	// 并发创建和验证事件
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			for j := 0; j < eventsPerGoroutine; j++ {
				// 创建事件（使用结构体而非 map，避免 Go 1.24 map 并发问题）
				payload := TestPayload{
					Title:     "Test Event",
					CreatedBy: "user-" + string(rune(idx)),
					Count:     idx*eventsPerGoroutine + j,
				}
				event := helper.CreateEnterpriseDomainEvent(
					"Test.Event",
					"test-"+string(rune(idx))+"-"+string(rune(j)),
					"Test",
					payload,
				)
				event.SetTenantId("tenant-" + string(rune(idx)))

				// 序列化 Payload
				payloadBytes, err := jxtevent.MarshalPayload(payload)
				if err != nil {
					mu.Lock()
					errors = append(errors, err)
					mu.Unlock()
					return
				}

				// 创建 Envelope
				envelope := helper.CreateEnvelope(
					event.GetEventType(),
					event.GetAggregateID(),
					event.GetTenantId(),
					payloadBytes,
				)

				// 验证一致性
				err = jxtevent.ValidateConsistency(envelope, event)
				if err != nil {
					mu.Lock()
					errors = append(errors, err)
					mu.Unlock()
					return
				}

				mu.Lock()
				allEvents = append(allEvents, event)
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 验证没有错误
	helper.AssertEqual(0, len(errors), "Should have no errors")

	// 验证事件总数
	expectedTotal := goroutines * eventsPerGoroutine
	helper.AssertEqual(expectedTotal, len(allEvents), "Should have correct number of events")

	// 验证所有事件的 UUID 唯一性
	uuids := make(map[string]bool)
	for _, event := range allEvents {
		uuids[event.GetEventID()] = true
	}
	helper.AssertEqual(expectedTotal, len(uuids), "All event IDs should be unique")
}

// TestIntegration_PayloadSerializationRoundTrip 测试 Payload 序列化往返集成
func TestIntegration_PayloadSerializationRoundTrip(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建复杂 Payload
	originalPayload := helper.CreateComplexPayload()

	// 创建事件
	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", originalPayload)

	// 序列化
	payloadBytes, err := jxtevent.MarshalPayload(event.GetPayload())
	helper.AssertNoError(err, "MarshalPayload should succeed")

	// 反序列化
	result, err := jxtevent.UnmarshalPayload[ComplexPayload](event)
	helper.AssertNoError(err, "UnmarshalPayload should succeed")

	// 验证数据完整性
	helper.AssertEqual(originalPayload.ID, result.ID, "ID should match")
	helper.AssertEqual(originalPayload.Name, result.Name, "Name should match")
	helper.AssertEqual(len(originalPayload.Tags), len(result.Tags), "Tags length should match")
	helper.AssertNotNil(result.Nested, "Nested should not be nil")
	helper.AssertEqual(originalPayload.Nested.Field1, result.Nested.Field1, "Nested.Field1 should match")
	helper.AssertEqual(originalPayload.Nested.Field2, result.Nested.Field2, "Nested.Field2 should match")

	// 验证 Envelope 一致性
	envelope := helper.CreateEnvelope(
		event.GetEventType(),
		event.GetAggregateID(),
		event.GetTenantId(),
		payloadBytes,
	)
	err = jxtevent.ValidateConsistency(envelope, event)
	helper.AssertNoError(err, "ValidateConsistency should succeed")
}

// TestIntegration_EventVersioning 测试事件版本控制
func TestIntegration_EventVersioning(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建不同版本的事件
	eventV1 := helper.CreateBaseDomainEvent("Order.Created", "order-123", "Order", TestPayload{
		Title:     "Order Created",
		CreatedBy: "customer",
		Count:     1,
	})

	// 验证默认版本
	helper.AssertEqual(1, eventV1.GetVersion(), "Default version should be 1")

	// 模拟事件演化（未来可能支持版本升级）
	// 这里只是验证版本字段的存在和正确性
	helper.AssertNotNil(eventV1.Version, "Version field should exist")
}

// TestIntegration_EventTimeOrdering 测试事件时序性
func TestIntegration_EventTimeOrdering(t *testing.T) {
	helper := NewTestHelper(t)

	var events []*jxtevent.BaseDomainEvent

	// 创建一系列事件
	for i := 0; i < 100; i++ {
		event := helper.CreateBaseDomainEvent("Test.Event", "test-"+string(rune(i)), "Test", nil)
		events = append(events, event)
		time.Sleep(1 * time.Millisecond) // 确保时间差异
	}

	// 验证 OccurredAt 时间递增
	for i := 1; i < len(events); i++ {
		helper.AssertTrue(
			events[i].GetOccurredAt().After(events[i-1].GetOccurredAt()) ||
				events[i].GetOccurredAt().Equal(events[i-1].GetOccurredAt()),
			"Events should be time-ordered",
		)
	}

	// 验证 UUIDv7 递增（字符串比较）
	for i := 1; i < len(events); i++ {
		helper.AssertTrue(
			events[i].GetEventID() >= events[i-1].GetEventID(),
			"Event IDs should be ordered",
		)
	}
}

// TestIntegration_ErrorHandling 测试错误处理集成
func TestIntegration_ErrorHandling(t *testing.T) {
	helper := NewTestHelper(t)

	// 测试 nil payload 的错误处理
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)
	_, err := jxtevent.UnmarshalPayload[TestPayload](event)
	helper.AssertError(err, "Should handle nil payload error")

	// 测试 nil envelope 的错误处理
	err = jxtevent.ValidateConsistency(nil, event)
	helper.AssertError(err, "Should handle nil envelope error")

	// 测试 nil event 的错误处理
	envelope := helper.CreateEnvelope("Test.Event", "test-123", "", []byte("{}"))
	err = jxtevent.ValidateConsistency(envelope, nil)
	helper.AssertError(err, "Should handle nil event error")

	// 测试不匹配的错误处理
	event2 := helper.CreateBaseDomainEvent("Different.Event", "test-123", "Test", nil)
	err = jxtevent.ValidateConsistency(envelope, event2)
	helper.AssertError(err, "Should handle mismatch error")
}

// TestIntegration_RealWorldScenario_OrderProcessing 测试真实场景：订单处理
func TestIntegration_RealWorldScenario_OrderProcessing(t *testing.T) {
	helper := NewTestHelper(t)

	tenantID := "tenant-acme-corp"
	correlationID := "order-processing-workflow-12345"
	traceID := "trace-abc-def-123"

	// 1. 订单创建事件（使用简化的 payload）
	orderCreatedPayload := TestPayload{
		Title:     "Order Created",
		CreatedBy: "customer-67890",
		Count:     2,
	}
	orderCreated := helper.CreateEnterpriseDomainEvent("Order.Created", "order-12345", "Order", orderCreatedPayload)
	orderCreated.SetTenantId(tenantID)
	orderCreated.SetCorrelationId(correlationID)
	orderCreated.SetTraceId(traceID)

	// 2. 库存预留事件
	inventoryReservedPayload := TestPayload{
		Title:     "Inventory Reserved",
		CreatedBy: "system",
		Count:     2,
	}
	inventoryReserved := helper.CreateEnterpriseDomainEvent("Inventory.Reserved", "reservation-001", "Inventory", inventoryReservedPayload)
	inventoryReserved.SetTenantId(tenantID)
	inventoryReserved.SetCorrelationId(correlationID)
	inventoryReserved.SetCausationId(orderCreated.GetEventID())
	inventoryReserved.SetTraceId(traceID)

	// 3. 支付处理事件
	paymentProcessedPayload := TestPayload{
		Title:     "Payment Processed",
		CreatedBy: "payment-gateway",
		Count:     1,
	}
	paymentProcessed := helper.CreateEnterpriseDomainEvent("Payment.Processed", "payment-456", "Payment", paymentProcessedPayload)
	paymentProcessed.SetTenantId(tenantID)
	paymentProcessed.SetCorrelationId(correlationID)
	paymentProcessed.SetCausationId(inventoryReserved.GetEventID())
	paymentProcessed.SetTraceId(traceID)

	// 4. 发货创建事件
	shipmentCreatedPayload := TestPayload{
		Title:     "Shipment Created",
		CreatedBy: "warehouse",
		Count:     1,
	}
	shipmentCreated := helper.CreateEnterpriseDomainEvent("Shipment.Created", "shipment-789", "Shipment", shipmentCreatedPayload)
	shipmentCreated.SetTenantId(tenantID)
	shipmentCreated.SetCorrelationId(correlationID)
	shipmentCreated.SetCausationId(paymentProcessed.GetEventID())
	shipmentCreated.SetTraceId(traceID)

	// 验证整个流程
	events := []*jxtevent.EnterpriseDomainEvent{
		orderCreated,
		inventoryReserved,
		paymentProcessed,
		shipmentCreated,
	}

	// 验证所有事件的租户一致性
	for _, event := range events {
		helper.AssertEqual(tenantID, event.GetTenantId(), "All events should belong to same tenant")
		helper.AssertEqual(correlationID, event.GetCorrelationId(), "All events should have same correlation ID")
		helper.AssertEqual(traceID, event.GetTraceId(), "All events should have same trace ID")
	}

	// 验证因果链
	helper.AssertEqual(orderCreated.GetEventID(), inventoryReserved.GetCausationId(), "Inventory caused by Order")
	helper.AssertEqual(inventoryReserved.GetEventID(), paymentProcessed.GetCausationId(), "Payment caused by Inventory")
	helper.AssertEqual(paymentProcessed.GetEventID(), shipmentCreated.GetCausationId(), "Shipment caused by Payment")

	// 验证每个事件的 Payload 可以正确反序列化
	for _, event := range events {
		payloadBytes, err := jxtevent.MarshalPayload(event.GetPayload())
		helper.AssertNoError(err, "MarshalPayload should succeed for all events")

		envelope := helper.CreateEnvelope(
			event.GetEventType(),
			event.GetAggregateID(),
			event.GetTenantId(),
			payloadBytes,
		)

		err = jxtevent.ValidateConsistency(envelope, event)
		helper.AssertNoError(err, "ValidateConsistency should succeed for all events")
	}
}
