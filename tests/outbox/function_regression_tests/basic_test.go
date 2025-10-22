package function_regression_tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// TestBasic_UUIDGeneration 测试基本 UUID 生成
func TestBasic_UUIDGeneration(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 验证 UUID 已生成
	helper.AssertNotEmpty(event.ID, "UUID should be generated")

	// 验证 UUID 格式（8-4-4-4-12）
	helper.AssertRegex(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, event.ID, "UUID should match format")
}

// TestBasic_UUIDUniqueness 测试 UUID 唯一性
func TestBasic_UUIDUniqueness(t *testing.T) {
	helper := NewTestHelper(t)

	count := 1000
	uuids := make(map[string]bool, count)

	for i := 0; i < count; i++ {
		event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
		uuids[event.ID] = true
	}

	// 验证所有 UUID 都是唯一的
	helper.AssertEqual(count, len(uuids), "All UUIDs should be unique")
}

// TestBasic_UUIDConcurrent 测试并发 UUID 生成
func TestBasic_UUIDConcurrent(t *testing.T) {
	helper := NewTestHelper(t)

	goroutines := 100
	eventsPerGoroutine := 100
	totalEvents := goroutines * eventsPerGoroutine

	var wg sync.WaitGroup
	var mu sync.Mutex
	uuids := make(map[string]bool, totalEvents)

	// 并发生成 UUID
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
				mu.Lock()
				uuids[event.ID] = true
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// 验证所有 UUID 都是唯一的
	helper.AssertEqual(totalEvents, len(uuids), "All concurrent UUIDs should be unique")
}

// TestBasic_IdempotencyKeyGeneration 测试幂等性键自动生成
func TestBasic_IdempotencyKeyGeneration(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 验证幂等性键已生成
	helper.AssertNotEmpty(event.IdempotencyKey, "IdempotencyKey should be generated")

	// 验证幂等性键包含所有必要字段
	helper.AssertContains(event.IdempotencyKey, "tenant1", "Should contain tenant ID")
	helper.AssertContains(event.IdempotencyKey, "Order", "Should contain aggregate type")
	helper.AssertContains(event.IdempotencyKey, "order-123", "Should contain aggregate ID")
	helper.AssertContains(event.IdempotencyKey, "OrderCreated", "Should contain event type")
	helper.AssertContains(event.IdempotencyKey, event.ID, "Should contain event ID")
}

// TestBasic_IdempotencyKeyUniqueness 测试幂等性键唯一性
func TestBasic_IdempotencyKeyUniqueness(t *testing.T) {
	helper := NewTestHelper(t)

	event1 := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
	event2 := helper.CreateTestEvent("tenant1", "Order", "order-456", "OrderCreated")
	event3 := helper.CreateTestEvent("tenant2", "Order", "order-123", "OrderCreated")

	// 验证不同事件的幂等性键不同
	helper.AssertNotEqual(event1.IdempotencyKey, event2.IdempotencyKey, "Different aggregate IDs should have different keys")
	helper.AssertNotEqual(event1.IdempotencyKey, event3.IdempotencyKey, "Different tenants should have different keys")
}

// TestBasic_EventInitialState 测试事件初始状态
func TestBasic_EventInitialState(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 验证初始状态
	helper.AssertEqual(outbox.EventStatusPending, event.Status, "Initial status should be Pending")
	helper.AssertEqual(0, event.RetryCount, "Initial retry count should be 0")
	helper.AssertEqual(3, event.MaxRetries, "Default max retries should be 3")
	helper.AssertEqual("", event.LastError, "Initial last error should be empty")
	helper.AssertNil(event.PublishedAt, "Initial published at should be nil")
	helper.AssertNil(event.LastRetryAt, "Initial last retry at should be nil")
}

// TestBasic_EventStatusTransitions 测试事件状态转换
func TestBasic_EventStatusTransitions(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
	now := time.Now()

	// 初始状态：Pending
	helper.AssertTrue(event.IsPending(), "Should be pending")
	helper.AssertFalse(event.IsPublished(), "Should not be published")
	helper.AssertFalse(event.IsFailed(), "Should not be failed")
	helper.AssertFalse(event.IsMaxRetry(), "Should not be max retry")

	// 标记为已发布
	event.Status = outbox.EventStatusPublished
	event.PublishedAt = &now

	helper.AssertFalse(event.IsPending(), "Should not be pending")
	helper.AssertTrue(event.IsPublished(), "Should be published")
	helper.AssertFalse(event.IsFailed(), "Should not be failed")
	helper.AssertFalse(event.IsMaxRetry(), "Should not be max retry")

	// 标记为失败
	event.Status = outbox.EventStatusFailed
	event.RetryCount = 1

	helper.AssertFalse(event.IsPending(), "Should not be pending")
	helper.AssertFalse(event.IsPublished(), "Should not be published")
	helper.AssertTrue(event.IsFailed(), "Should be failed")
	helper.AssertFalse(event.IsMaxRetry(), "Should not be max retry")

	// 标记为超过最大重试次数
	event.Status = outbox.EventStatusMaxRetry
	event.RetryCount = 4

	helper.AssertFalse(event.IsPending(), "Should not be pending")
	helper.AssertFalse(event.IsPublished(), "Should not be published")
	helper.AssertFalse(event.IsFailed(), "Should not be failed")
	helper.AssertTrue(event.IsMaxRetry(), "Should be max retry")
}

// TestBasic_EventTimestamps 测试事件时间戳
func TestBasic_EventTimestamps(t *testing.T) {
	helper := NewTestHelper(t)

	before := time.Now()
	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
	after := time.Now()

	// 验证创建时间
	helper.AssertNotNil(event.CreatedAt, "CreatedAt should be set")
	helper.AssertTrue(event.CreatedAt.After(before) || event.CreatedAt.Equal(before), "CreatedAt should be after or equal to before")
	helper.AssertTrue(event.CreatedAt.Before(after) || event.CreatedAt.Equal(after), "CreatedAt should be before or equal to after")

	// 验证更新时间
	helper.AssertNotNil(event.UpdatedAt, "UpdatedAt should be set")
	helper.AssertTrue(event.UpdatedAt.After(before) || event.UpdatedAt.Equal(before), "UpdatedAt should be after or equal to before")
	helper.AssertTrue(event.UpdatedAt.Before(after) || event.UpdatedAt.Equal(after), "UpdatedAt should be before or equal to after")
}

// TestBasic_EventScheduledPublish 测试延迟发布
func TestBasic_EventScheduledPublish(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 设置延迟发布时间
	scheduledTime := time.Now().Add(1 * time.Hour)
	event.ScheduledAt = &scheduledTime

	// 验证延迟发布时间
	helper.AssertNotNil(event.ScheduledAt, "ScheduledAt should be set")
	helper.AssertEqual(scheduledTime.Unix(), event.ScheduledAt.Unix(), "ScheduledAt should match")
}

// TestBasic_EventErrorTracking 测试错误跟踪
func TestBasic_EventErrorTracking(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 模拟发布失败
	testErr := context.DeadlineExceeded
	event.MarkAsFailed(testErr)

	// 验证错误信息
	helper.AssertEqual(outbox.EventStatusFailed, event.Status, "Status should be Failed")
	helper.AssertEqual(1, event.RetryCount, "Retry count should be 1")
	helper.AssertNotEmpty(event.LastError, "Last error should be set")
	helper.AssertNotNil(event.LastRetryAt, "LastRetryAt should be set")
}

// TestBasic_EventVersionTracking 测试版本跟踪
func TestBasic_EventVersionTracking(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 验证版本字段（Version 是 int64 类型）
	helper.AssertEqual(int64(1), event.Version, "Default version should be 1")
}

// TestBasic_EventTraceAndCorrelation 测试追踪和关联 ID
func TestBasic_EventTraceAndCorrelation(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 设置追踪和关联 ID
	event.TraceID = "trace-123"
	event.CorrelationID = "correlation-456"

	// 验证追踪和关联 ID
	helper.AssertEqual("trace-123", event.TraceID, "TraceID should match")
	helper.AssertEqual("correlation-456", event.CorrelationID, "CorrelationID should match")
}

// TestBasic_PublishSuccess 测试发布成功
func TestBasic_PublishSuccess(t *testing.T) {
	helper := NewTestHelper(t)
	ctx := context.Background()

	// 创建模拟对象
	repo := NewMockRepository()
	publisher := NewMockEventPublisher()
	topicMapper := NewMockTopicMapper()
	topicMapper.SetTopicMapping("Order", "order-events")

	// 创建 Outbox 发布器
	config := GetDefaultPublisherConfig()
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	// 创建事件
	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 发布事件
	err := outboxPublisher.PublishEvent(ctx, event)
	helper.AssertNoError(err, "Publish should succeed")

	// 验证事件状态
	helper.AssertEqual(outbox.EventStatusPublished, event.Status, "Status should be Published")
	helper.AssertNotNil(event.PublishedAt, "PublishedAt should be set")
	helper.AssertEqual(0, event.RetryCount, "Retry count should still be 0")
	helper.AssertEqual("", event.LastError, "Last error should be empty")
}
