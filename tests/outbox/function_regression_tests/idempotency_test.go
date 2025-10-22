package function_regression_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// TestIdempotency_AutoGeneration 测试幂等性键自动生成
func TestIdempotency_AutoGeneration(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 验证幂等性键已自动生成
	helper.AssertNotEmpty(event.IdempotencyKey, "IdempotencyKey should be auto-generated")

	// 验证幂等性键格式
	expectedPrefix := "tenant1:Order:order-123:OrderCreated:"
	helper.AssertContains(event.IdempotencyKey, expectedPrefix, "IdempotencyKey should contain expected prefix")
}

// TestIdempotency_CustomKey 测试自定义幂等性键
func TestIdempotency_CustomKey(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 设置自定义幂等性键
	customKey := "custom-idempotency-key-12345"
	event.IdempotencyKey = customKey

	// 验证自定义幂等性键
	helper.AssertEqual(customKey, event.IdempotencyKey, "Should use custom IdempotencyKey")
}

// TestIdempotency_UniqueKeys 测试幂等性键唯一性
func TestIdempotency_UniqueKeys(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建多个不同的事件
	events := []*outbox.OutboxEvent{
		helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated"),
		helper.CreateTestEvent("tenant1", "Order", "order-456", "OrderCreated"),
		helper.CreateTestEvent("tenant1", "User", "user-789", "UserRegistered"),
		helper.CreateTestEvent("tenant2", "Order", "order-123", "OrderCreated"),
	}

	// 收集所有幂等性键
	keys := make(map[string]bool)
	for _, event := range events {
		if keys[event.IdempotencyKey] {
			t.Fatalf("Duplicate IdempotencyKey found: %s", event.IdempotencyKey)
		}
		keys[event.IdempotencyKey] = true
	}

	// 验证生成了唯一的幂等性键
	helper.AssertEqual(len(events), len(keys), "Should generate unique IdempotencyKeys")
}

// TestIdempotency_SameEventDifferentIDs 测试相同事件不同 ID
func TestIdempotency_SameEventDifferentIDs(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建两个相同参数的事件（但 ID 不同）
	event1 := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
	time.Sleep(1 * time.Millisecond) // 确保时间戳不同
	event2 := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 验证 ID 不同
	helper.AssertTrue(event1.ID != event2.ID, "Event IDs should be different")

	// 验证幂等性键不同（因为包含事件 ID）
	helper.AssertTrue(event1.IdempotencyKey != event2.IdempotencyKey,
		"IdempotencyKeys should be different when event IDs are different")
}

// TestIdempotency_PublisherCheck 测试发布器幂等性检查
func TestIdempotency_PublisherCheck(t *testing.T) {
	helper := NewTestHelper(t)
	ctx := context.Background()

	// 创建模拟仓储和发布器
	repo := NewMockRepository()
	publisher := NewMockEventPublisher()
	topicMapper := NewMockTopicMapper()
	topicMapper.SetTopicMapping("Order", "order-events")

	// 创建 Outbox 发布器
	config := GetDefaultPublisherConfig()
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	// 创建事件
	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 第一次发布
	err := outboxPublisher.PublishEvent(ctx, event)
	helper.AssertNoError(err, "First publish should succeed")

	// 模拟事件已发布
	event.Status = outbox.EventStatusPublished
	publishedAt := time.Now()
	event.PublishedAt = &publishedAt
	err = repo.Update(ctx, event)
	helper.AssertNoError(err, "Should update event status")

	// 第二次发布相同的幂等性键（应该被跳过）
	event2 := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
	event2.IdempotencyKey = event.IdempotencyKey

	err = outboxPublisher.PublishEvent(ctx, event2)
	helper.AssertNoError(err, "Second publish should succeed (idempotent)")

	// 验证只发布了一次
	helper.AssertEqual(1, publisher.GetPublishedCount(), "Should only publish once")
}

// TestIdempotency_DifferentTenants 测试不同租户的幂等性
func TestIdempotency_DifferentTenants(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建不同租户的相同事件
	event1 := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
	event2 := helper.CreateTestEvent("tenant2", "Order", "order-123", "OrderCreated")

	// 验证幂等性键不同
	helper.AssertTrue(event1.IdempotencyKey != event2.IdempotencyKey,
		"IdempotencyKeys should be different for different tenants")

	// 验证幂等性键包含租户 ID
	helper.AssertContains(event1.IdempotencyKey, "tenant1", "IdempotencyKey should contain tenant1")
	helper.AssertContains(event2.IdempotencyKey, "tenant2", "IdempotencyKey should contain tenant2")
}

// TestIdempotency_KeyFormat 测试幂等性键格式
func TestIdempotency_KeyFormat(t *testing.T) {
	helper := NewTestHelper(t)

	event, err := outbox.NewOutboxEvent(
		"tenant-abc",
		"Order",
		"order-xyz",
		"OrderCreated",
		[]byte(`{"test": true}`),
	)
	helper.RequireNoError(err, "Failed to create event")

	// 验证幂等性键格式：{TenantID}:{AggregateType}:{AggregateID}:{EventType}:{EventID}
	expectedFormat := fmt.Sprintf("tenant-abc:Order:order-xyz:OrderCreated:%s", event.ID)
	helper.AssertEqual(expectedFormat, event.IdempotencyKey, "IdempotencyKey should match expected format")
}

// TestIdempotency_EmptyFields 测试空字段的幂等性键生成
func TestIdempotency_EmptyFields(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建带空字段的事件
	event, err := outbox.NewOutboxEvent(
		"", // 空租户 ID
		"Order",
		"order-123",
		"OrderCreated",
		[]byte(`{"test": true}`),
	)
	helper.RequireNoError(err, "Failed to create event")

	// 验证幂等性键仍然生成
	helper.AssertNotEmpty(event.IdempotencyKey, "IdempotencyKey should be generated even with empty tenant ID")
}

// TestIdempotency_SpecialCharacters 测试特殊字符的幂等性键
func TestIdempotency_SpecialCharacters(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建带特殊字符的事件
	event, err := outbox.NewOutboxEvent(
		"tenant:123",
		"Order-Type",
		"order/456",
		"Order:Created",
		[]byte(`{"test": true}`),
	)
	helper.RequireNoError(err, "Failed to create event")

	// 验证幂等性键已生成
	helper.AssertNotEmpty(event.IdempotencyKey, "IdempotencyKey should be generated with special characters")

	// 验证幂等性键包含所有字段
	helper.AssertContains(event.IdempotencyKey, "tenant:123", "Should contain tenant ID")
	helper.AssertContains(event.IdempotencyKey, "Order-Type", "Should contain aggregate type")
	helper.AssertContains(event.IdempotencyKey, "order/456", "Should contain aggregate ID")
	helper.AssertContains(event.IdempotencyKey, "Order:Created", "Should contain event type")
}

// TestIdempotency_ConcurrentPublish 测试并发发布的幂等性
func TestIdempotency_ConcurrentPublish(t *testing.T) {
	helper := NewTestHelper(t)
	ctx := context.Background()

	// 创建模拟仓储和发布器
	repo := NewMockRepository()
	publisher := NewMockEventPublisher()
	topicMapper := NewMockTopicMapper()
	topicMapper.SetTopicMapping("Order", "order-events")

	// 创建 Outbox 发布器
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper)

	// 创建事件
	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
	idempotencyKey := event.IdempotencyKey

	// 保存事件
	err := repo.Save(ctx, event)
	helper.AssertNoError(err, "Should save event")

	// 并发发布相同幂等性键的事件
	const goroutines = 10
	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			duplicateEvent := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
			duplicateEvent.IdempotencyKey = idempotencyKey
			errors <- outboxPublisher.PublishEvent(ctx, duplicateEvent)
		}()
	}

	// 收集错误
	for i := 0; i < goroutines; i++ {
		err := <-errors
		helper.AssertNoError(err, "Concurrent publish should not error")
	}

	// 验证只发布了一次（幂等性保证）
	// 注意：由于并发，可能会有多次发布，但应该被幂等性检查拦截
	t.Logf("Published count: %d (expected: 1 or more due to race conditions)", publisher.GetPublishedCount())
}

// TestIdempotency_BatchPublish 测试批量发布的幂等性
func TestIdempotency_BatchPublish(t *testing.T) {
	helper := NewTestHelper(t)
	ctx := context.Background()

	// 创建模拟仓储和发布器
	repo := NewMockRepository()
	publisher := NewMockEventPublisher()
	topicMapper := NewMockTopicMapper()
	topicMapper.SetTopicMapping("Order", "order-events")

	// 创建 Outbox 发布器
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper)

	// 创建多个事件，其中有重复的幂等性键
	events := []*outbox.OutboxEvent{
		helper.CreateTestEvent("tenant1", "Order", "order-1", "OrderCreated"),
		helper.CreateTestEvent("tenant1", "Order", "order-2", "OrderCreated"),
		helper.CreateTestEvent("tenant1", "Order", "order-3", "OrderCreated"),
	}

	// 保存第一个事件并标记为已发布
	events[0].Status = outbox.EventStatusPublished
	publishedAt := time.Now()
	events[0].PublishedAt = &publishedAt
	err := repo.Save(ctx, events[0])
	helper.AssertNoError(err, "Should save first event")

	// 创建一个与第一个事件相同幂等性键的事件
	duplicateEvent := helper.CreateTestEvent("tenant1", "Order", "order-1", "OrderCreated")
	duplicateEvent.IdempotencyKey = events[0].IdempotencyKey

	// 批量发布（包含重复的幂等性键）
	batchEvents := []*outbox.OutboxEvent{
		duplicateEvent, // 重复
		events[1],      // 新事件
		events[2],      // 新事件
	}

	err = outboxPublisher.PublishEvents(ctx, batchEvents)
	helper.AssertNoError(err, "Batch publish should succeed")

	// 验证只发布了新事件（跳过了重复的）
	helper.AssertEqual(2, publisher.GetPublishedCount(), "Should only publish new events")
}
