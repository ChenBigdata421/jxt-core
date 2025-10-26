package function_regression_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// TestEventLifecycle_InitialState 测试事件初始状态
func TestEventLifecycle_InitialState(t *testing.T) {
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

// TestEventLifecycle_PublishSuccess 测试发布成功
func TestEventLifecycle_PublishSuccess(t *testing.T) {
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

	// 启动 ACK 监听器
	outboxPublisher.StartACKListener(ctx)
	defer outboxPublisher.StopACKListener()

	// 创建事件
	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 保存事件到仓储
	err := repo.Save(ctx, event)
	helper.AssertNoError(err, "Should save event")

	// 发布事件
	err = outboxPublisher.PublishEvent(ctx, event)
	helper.AssertNoError(err, "Publish should succeed")

	// 等待 ACK 处理
	time.Sleep(100 * time.Millisecond)

	// 从仓储重新加载事件
	updatedEvent, err := repo.FindByID(ctx, event.ID)
	helper.AssertNoError(err, "Should find event by ID")
	helper.AssertNotNil(updatedEvent, "Event should exist")

	// 验证事件状态
	helper.AssertEqual(outbox.EventStatusPublished, updatedEvent.Status, "Status should be Published")
	helper.AssertNotNil(updatedEvent.PublishedAt, "PublishedAt should be set")
	helper.AssertEqual(0, updatedEvent.RetryCount, "Retry count should still be 0")
	helper.AssertEqual("", updatedEvent.LastError, "Last error should be empty")
}

// TestEventLifecycle_PublishFailure 测试发布失败
func TestEventLifecycle_PublishFailure(t *testing.T) {
	helper := NewTestHelper(t)
	ctx := context.Background()

	// 创建模拟仓储和发布器
	repo := NewMockRepository()
	publisher := NewMockEventPublisher()
	publisher.SetPublishError(fmt.Errorf("publish failed"))
	topicMapper := NewMockTopicMapper()
	topicMapper.SetTopicMapping("Order", "order-events")

	// 创建 Outbox 发布器
	config := GetDefaultPublisherConfig()
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	// 创建事件
	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 保存事件到仓储
	err := repo.Save(ctx, event)
	helper.AssertNoError(err, "Should save event")

	// 发布事件（应该失败）
	err = outboxPublisher.PublishEvent(ctx, event)
	helper.AssertError(err, "PublishEvent should return error when publish fails")

	// 从仓储重新加载事件
	updatedEvent, err := repo.FindByID(ctx, event.ID)
	helper.AssertNoError(err, "Should find event by ID")
	helper.AssertNotNil(updatedEvent, "Event should exist")

	// 验证事件状态
	helper.AssertEqual(outbox.EventStatusFailed, updatedEvent.Status, "Status should be Failed")
	helper.AssertEqual(1, updatedEvent.RetryCount, "Retry count should be 1")
	helper.AssertNotEmpty(updatedEvent.LastError, "Last error should be set")
	helper.AssertNil(updatedEvent.PublishedAt, "PublishedAt should be nil")
	helper.AssertNotNil(updatedEvent.LastRetryAt, "LastRetryAt should be set")
}

// TestEventLifecycle_RetryMechanism 测试重试机制
func TestEventLifecycle_RetryMechanism(t *testing.T) {
	helper := NewTestHelper(t)
	ctx := context.Background()

	// 创建模拟仓储和发布器
	repo := NewMockRepository()
	publisher := NewMockEventPublisher()
	publisher.SetPublishError(fmt.Errorf("publish failed"))
	topicMapper := NewMockTopicMapper()
	topicMapper.SetTopicMapping("Order", "order-events")

	// 创建 Outbox 发布器
	config := GetDefaultPublisherConfig()
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	// 创建事件
	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
	event.MaxRetries = 3

	// 保存事件到仓储
	err := repo.Save(ctx, event)
	helper.AssertNoError(err, "Should save event")

	// 第一次发布失败
	err = outboxPublisher.PublishEvent(ctx, event)
	helper.AssertError(err, "PublishEvent should return error")

	// 从仓储重新加载事件
	updatedEvent, err := repo.FindByID(ctx, event.ID)
	helper.AssertNoError(err, "Should find event by ID")
	helper.AssertEqual(outbox.EventStatusFailed, updatedEvent.Status, "Status should be Failed")

	// 第二次重试失败
	err = outboxPublisher.PublishEvent(ctx, updatedEvent)
	helper.AssertError(err, "PublishEvent should return error")

	updatedEvent, err = repo.FindByID(ctx, event.ID)
	helper.AssertNoError(err, "Should find event by ID")
	helper.AssertEqual(outbox.EventStatusFailed, updatedEvent.Status, "Status should be Failed")

	// 第三次重试失败
	err = outboxPublisher.PublishEvent(ctx, updatedEvent)
	helper.AssertError(err, "PublishEvent should return error")

	updatedEvent, err = repo.FindByID(ctx, event.ID)
	helper.AssertNoError(err, "Should find event by ID")
	helper.AssertEqual(outbox.EventStatusFailed, updatedEvent.Status, "Status should be Failed")

	// 第四次重试（超过最大重试次数）
	err = outboxPublisher.PublishEvent(ctx, updatedEvent)
	helper.AssertError(err, "PublishEvent should return error")

	updatedEvent, err = repo.FindByID(ctx, event.ID)
	helper.AssertNoError(err, "Should find event by ID")
	helper.AssertEqual(outbox.EventStatusFailed, updatedEvent.Status, "Status should be Failed")
}

// TestEventLifecycle_StatusTransitions 测试状态转换
func TestEventLifecycle_StatusTransitions(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// Pending -> Published
	helper.AssertTrue(event.IsPending(), "Should be pending")
	helper.AssertFalse(event.IsPublished(), "Should not be published")
	helper.AssertFalse(event.IsFailed(), "Should not be failed")
	helper.AssertFalse(event.IsMaxRetry(), "Should not be max retry")

	// 标记为已发布
	event.Status = outbox.EventStatusPublished
	publishedAt := time.Now()
	event.PublishedAt = &publishedAt

	helper.AssertFalse(event.IsPending(), "Should not be pending")
	helper.AssertTrue(event.IsPublished(), "Should be published")
	helper.AssertFalse(event.IsFailed(), "Should not be failed")
	helper.AssertFalse(event.IsMaxRetry(), "Should not be max retry")

	// 标记为失败
	event.Status = outbox.EventStatusFailed
	event.RetryCount = 1
	event.LastError = "test error"

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

// TestEventLifecycle_ScheduledPublish 测试延迟发布
func TestEventLifecycle_ScheduledPublish(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 设置延迟发布时间
	scheduledAt := time.Now().Add(1 * time.Hour)
	event.ScheduledAt = &scheduledAt

	// 验证延迟发布时间
	helper.AssertNotNil(event.ScheduledAt, "ScheduledAt should be set")
	helper.AssertTrue(event.ScheduledAt.After(time.Now()), "ScheduledAt should be in the future")
}

// TestEventLifecycle_Timestamps 测试时间戳
func TestEventLifecycle_Timestamps(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 验证创建时间
	helper.AssertTrue(!event.CreatedAt.IsZero(), "CreatedAt should be set")
	helper.AssertTrue(event.CreatedAt.Before(time.Now().Add(1*time.Second)), "CreatedAt should be recent")

	// 验证更新时间
	helper.AssertTrue(!event.UpdatedAt.IsZero(), "UpdatedAt should be set")
	helper.AssertTrue(event.UpdatedAt.Before(time.Now().Add(1*time.Second)), "UpdatedAt should be recent")

	// 验证发布时间初始为 nil
	helper.AssertNil(event.PublishedAt, "PublishedAt should be nil initially")

	// 验证最后重试时间初始为 nil
	helper.AssertNil(event.LastRetryAt, "LastRetryAt should be nil initially")
}

// TestEventLifecycle_MaxRetriesExceeded 测试超过最大重试次数
func TestEventLifecycle_MaxRetriesExceeded(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
	event.MaxRetries = 3

	// 模拟多次重试
	for i := 0; i < 5; i++ {
		event.RetryCount++
		if event.RetryCount > event.MaxRetries {
			event.Status = outbox.EventStatusMaxRetry
		} else {
			event.Status = outbox.EventStatusFailed
		}
	}

	// 验证状态
	helper.AssertEqual(outbox.EventStatusMaxRetry, event.Status, "Status should be Dead")
	helper.AssertEqual(5, event.RetryCount, "Retry count should be 5")
	helper.AssertTrue(event.RetryCount > event.MaxRetries, "Retry count should exceed max retries")
}

// TestEventLifecycle_ErrorTracking 测试错误跟踪
func TestEventLifecycle_ErrorTracking(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 模拟第一次错误
	event.Status = outbox.EventStatusFailed
	event.RetryCount = 1
	event.LastError = "connection timeout"
	lastRetryAt1 := time.Now()
	event.LastRetryAt = &lastRetryAt1

	helper.AssertEqual("connection timeout", event.LastError, "Should track first error")
	helper.AssertNotNil(event.LastRetryAt, "Should track first retry time")

	// 模拟第二次错误
	time.Sleep(10 * time.Millisecond)
	event.RetryCount = 2
	event.LastError = "network unreachable"
	lastRetryAt2 := time.Now()
	event.LastRetryAt = &lastRetryAt2

	helper.AssertEqual("network unreachable", event.LastError, "Should track latest error")
	helper.AssertTrue(event.LastRetryAt.After(lastRetryAt1), "Should update retry time")
}

// TestEventLifecycle_VersionTracking 测试版本跟踪
func TestEventLifecycle_VersionTracking(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 验证初始版本
	helper.AssertEqual(int64(1), event.Version, "Initial version should be 1")

	// 模拟版本升级
	event.Version = 2
	helper.AssertEqual(int64(2), event.Version, "Version should be updated")
}

// TestEventLifecycle_TraceAndCorrelation 测试追踪和关联 ID
func TestEventLifecycle_TraceAndCorrelation(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")

	// 设置追踪 ID
	event.TraceID = "trace-12345"
	helper.AssertEqual("trace-12345", event.TraceID, "Should set trace ID")

	// 设置关联 ID
	event.CorrelationID = "correlation-67890"
	helper.AssertEqual("correlation-67890", event.CorrelationID, "Should set correlation ID")
}
