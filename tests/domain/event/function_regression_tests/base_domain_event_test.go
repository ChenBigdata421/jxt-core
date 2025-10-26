package function_regression_tests

import (
	"fmt"
	"sync"
	"testing"
	"time"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
)

// TestBaseDomainEvent_Creation 测试基础领域事件创建
func TestBaseDomainEvent_Creation(t *testing.T) {
	helper := NewTestHelper(t)

	payload := helper.CreateTestPayload()
	event := helper.CreateBaseDomainEvent("Archive.Created", "archive-123", "Archive", payload)

	// 验证基本字段
	helper.AssertNotEmpty(event.EventID, "EventID should be generated")
	helper.AssertEqual("Archive.Created", event.EventType, "EventType should match")
	helper.AssertEqual("archive-123", event.AggregateID, "AggregateID should match")
	helper.AssertEqual("Archive", event.AggregateType, "AggregateType should match")
	helper.AssertEqual(1, event.Version, "Version should be 1")
	helper.AssertNotNil(event.Payload, "Payload should not be nil")

	// 验证时间字段
	helper.AssertWithinDuration(time.Now(), event.OccurredAt, 5*time.Second, "OccurredAt should be recent")
}

// TestBaseDomainEvent_UUIDv7Format 测试 UUIDv7 格式
func TestBaseDomainEvent_UUIDv7Format(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证 UUID 格式（8-4-4-4-12）
	helper.AssertRegex(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, event.EventID, "EventID should be valid UUID")

	// UUIDv7 的版本号应该是 7（第15个字符应该是 '7'）
	helper.AssertEqual(byte('7'), event.EventID[14], "Should be UUIDv7")
}

// TestBaseDomainEvent_UUIDUniqueness 测试 UUID 唯一性
func TestBaseDomainEvent_UUIDUniqueness(t *testing.T) {
	helper := NewTestHelper(t)

	count := 1000
	uuids := make(map[string]bool, count)

	for i := 0; i < count; i++ {
		event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)
		uuids[event.EventID] = true
	}

	// 验证所有 UUID 都是唯一的
	helper.AssertEqual(count, len(uuids), "All UUIDs should be unique")
}

// TestBaseDomainEvent_UUIDTimeOrdering 测试 UUIDv7 时序性
func TestBaseDomainEvent_UUIDTimeOrdering(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建多个事件，验证 UUID 是递增的
	var eventIDs []string
	for i := 0; i < 10; i++ {
		event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)
		eventIDs = append(eventIDs, event.EventID)
		time.Sleep(1 * time.Millisecond) // 确保时间差异
	}

	// 验证 UUID 是递增的（字符串比较）
	for i := 1; i < len(eventIDs); i++ {
		helper.AssertTrue(eventIDs[i] > eventIDs[i-1], fmt.Sprintf("UUID should be ordered: %s > %s", eventIDs[i], eventIDs[i-1]))
	}
}

// TestBaseDomainEvent_ConcurrentCreation 测试并发创建
func TestBaseDomainEvent_ConcurrentCreation(t *testing.T) {
	helper := NewTestHelper(t)

	goroutines := 100
	eventsPerGoroutine := 100
	totalEvents := goroutines * eventsPerGoroutine

	var wg sync.WaitGroup
	var mu sync.Mutex
	uuids := make(map[string]bool, totalEvents)

	// 并发创建事件
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)
				mu.Lock()
				uuids[event.EventID] = true
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// 验证所有 UUID 都是唯一的
	helper.AssertEqual(totalEvents, len(uuids), "All concurrent UUIDs should be unique")
}

// TestBaseDomainEvent_AggregateIDTypes 测试不同类型的聚合根ID
func TestBaseDomainEvent_AggregateIDTypes(t *testing.T) {
	helper := NewTestHelper(t)

	// 测试 string 类型
	event1 := jxtevent.NewBaseDomainEvent("Test.Event", "string-id", "Test", nil)
	helper.AssertEqual("string-id", event1.AggregateID, "String ID should work")

	// 测试 int64 类型
	event2 := jxtevent.NewBaseDomainEvent("Test.Event", int64(12345), "Test", nil)
	helper.AssertEqual("12345", event2.AggregateID, "Int64 ID should be converted to string")

	// 测试 int 类型
	event3 := jxtevent.NewBaseDomainEvent("Test.Event", 67890, "Test", nil)
	helper.AssertEqual("67890", event3.AggregateID, "Int ID should be converted to string")
}

// TestBaseDomainEvent_GetterMethods 测试 Getter 方法
func TestBaseDomainEvent_GetterMethods(t *testing.T) {
	helper := NewTestHelper(t)

	payload := helper.CreateTestPayload()
	event := helper.CreateBaseDomainEvent("Archive.Created", "archive-123", "Archive", payload)

	// 测试所有 Getter 方法
	helper.AssertNotEmpty(event.GetEventID(), "GetEventID should return value")
	helper.AssertEqual("Archive.Created", event.GetEventType(), "GetEventType should return value")
	helper.AssertEqual("archive-123", event.GetAggregateID(), "GetAggregateID should return value")
	helper.AssertEqual("Archive", event.GetAggregateType(), "GetAggregateType should return value")
	helper.AssertEqual(1, event.GetVersion(), "GetVersion should return value")
	helper.AssertNotNil(event.GetPayload(), "GetPayload should return value")
	helper.AssertWithinDuration(time.Now(), event.GetOccurredAt(), 5*time.Second, "GetOccurredAt should return recent time")
}

// TestBaseDomainEvent_InterfaceCompliance 测试接口实现
func TestBaseDomainEvent_InterfaceCompliance(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证实现了 BaseEvent 接口
	var _ jxtevent.BaseEvent = event
	helper.AssertTrue(true, "BaseDomainEvent should implement BaseEvent interface")
}

// TestBaseDomainEvent_NilPayload 测试 nil Payload
func TestBaseDomainEvent_NilPayload(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证可以创建 nil payload 的事件
	helper.AssertNotEmpty(event.EventID, "Event should be created with nil payload")
	helper.AssertNil(event.Payload, "Payload should be nil")
	helper.AssertNil(event.GetPayload(), "GetPayload should return nil")
}

// TestBaseDomainEvent_EmptyStrings 测试空字符串
func TestBaseDomainEvent_EmptyStrings(t *testing.T) {
	helper := NewTestHelper(t)

	// 测试空字符串参数
	event := jxtevent.NewBaseDomainEvent("", "", "", nil)

	// 验证事件仍然可以创建（业务层应该做验证）
	helper.AssertNotEmpty(event.EventID, "Event should be created even with empty strings")
	helper.AssertEqual("", event.EventType, "EventType should be empty")
	helper.AssertEqual("", event.AggregateID, "AggregateID should be empty")
	helper.AssertEqual("", event.AggregateType, "AggregateType should be empty")
}

// TestBaseDomainEvent_PayloadTypes 测试不同类型的 Payload
func TestBaseDomainEvent_PayloadTypes(t *testing.T) {
	helper := NewTestHelper(t)

	// 测试结构体 Payload
	structPayload := helper.CreateTestPayload()
	event1 := helper.CreateBaseDomainEvent("Test.Event", "test-1", "Test", structPayload)
	helper.AssertNotNil(event1.Payload, "Struct payload should work")

	// 测试 map Payload
	mapPayload := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}
	event2 := helper.CreateBaseDomainEvent("Test.Event", "test-2", "Test", mapPayload)
	helper.AssertNotNil(event2.Payload, "Map payload should work")

	// 测试字符串 Payload
	stringPayload := "simple string payload"
	event3 := helper.CreateBaseDomainEvent("Test.Event", "test-3", "Test", stringPayload)
	helper.AssertNotNil(event3.Payload, "String payload should work")

	// 测试数字 Payload
	numberPayload := 12345
	event4 := helper.CreateBaseDomainEvent("Test.Event", "test-4", "Test", numberPayload)
	helper.AssertNotNil(event4.Payload, "Number payload should work")
}

// TestBaseDomainEvent_VersionField 测试版本字段
func TestBaseDomainEvent_VersionField(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)

	// 验证默认版本是 1
	helper.AssertEqual(1, event.Version, "Default version should be 1")
	helper.AssertEqual(1, event.GetVersion(), "GetVersion should return 1")
}

// TestBaseDomainEvent_OccurredAtPrecision 测试时间精度
func TestBaseDomainEvent_OccurredAtPrecision(t *testing.T) {
	helper := NewTestHelper(t)

	before := time.Now()
	event := helper.CreateBaseDomainEvent("Test.Event", "test-123", "Test", nil)
	after := time.Now()

	// 验证 OccurredAt 在创建前后的时间范围内
	helper.AssertTrue(event.OccurredAt.After(before) || event.OccurredAt.Equal(before), "OccurredAt should be after or equal to before time")
	helper.AssertTrue(event.OccurredAt.Before(after) || event.OccurredAt.Equal(after), "OccurredAt should be before or equal to after time")
}

// TestBaseDomainEvent_MultipleEventsIndependence 测试多个事件的独立性
func TestBaseDomainEvent_MultipleEventsIndependence(t *testing.T) {
	helper := NewTestHelper(t)

	event1 := helper.CreateBaseDomainEvent("Event.Type1", "id-1", "Type1", "payload1")
	event2 := helper.CreateBaseDomainEvent("Event.Type2", "id-2", "Type2", "payload2")

	// 验证两个事件完全独立
	helper.AssertNotEqual(event1.EventID, event2.EventID, "EventIDs should be different")
	helper.AssertNotEqual(event1.EventType, event2.EventType, "EventTypes should be different")
	helper.AssertNotEqual(event1.AggregateID, event2.AggregateID, "AggregateIDs should be different")
	helper.AssertNotEqual(event1.AggregateType, event2.AggregateType, "AggregateTypes should be different")
	helper.AssertNotEqual(event1.Payload, event2.Payload, "Payloads should be different")
}

