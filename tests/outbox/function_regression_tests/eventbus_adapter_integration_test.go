package function_regression_tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters"
)

// TestEventBusAdapter_Integration_PublishSuccess 测试 EventBus 适配器集成 - 发布成功
func TestEventBusAdapter_Integration_PublishSuccess(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock EventBus
	mockEventBus := NewMockEventBusForAdapter()

	// 创建 EventBus 适配器
	adapter := adapters.NewEventBusAdapter(mockEventBus)
	defer adapter.Close()

	// 验证适配器已启动
	helper.AssertTrue(adapter.IsStarted(), "Adapter should be started")

	// 创建 Outbox 组件
	repo := NewMockRepository()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, config)

	// 启动 ACK 监听器
	ctx := context.Background()
	publisher.StartACKListener(ctx)
	defer publisher.StopACKListener()

	// 创建测试事件
	event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
	err := repo.Save(ctx, event)
	helper.RequireNoError(err)

	// 发布事件
	err = publisher.PublishEvent(ctx, event)
	helper.AssertNoError(err, "PublishEvent should succeed")

	// 模拟 EventBus 发送 ACK 成功
	mockEventBus.SendACKSuccess(event.ID, "Order-events", "order-123", "OrderCreated")

	// 等待 ACK 处理
	time.Sleep(200 * time.Millisecond)

	// 验证事件状态已更新为 Published
	updatedEvent, err := repo.FindByID(ctx, event.ID)
	helper.RequireNoError(err)
	helper.AssertNotNil(updatedEvent, "Event should exist")
	helper.AssertEqual(outbox.EventStatusPublished, updatedEvent.Status, "Event should be marked as Published")

	// 验证 EventBus 收到了正确的 Envelope
	helper.AssertEqual(1, mockEventBus.GetPublishedEnvelopeCount(), "Should have 1 published envelope")

	envelopes := mockEventBus.GetPublishedEnvelopes()
	envelope := envelopes[0]
	helper.AssertEqual(event.ID, envelope.EventID, "EventID should match")
	helper.AssertEqual(event.AggregateID, envelope.AggregateID, "AggregateID should match")
	helper.AssertEqual(event.EventType, envelope.EventType, "EventType should match")
}

// TestEventBusAdapter_Integration_PublishFailure 测试 EventBus 适配器集成 - 发布失败
func TestEventBusAdapter_Integration_PublishFailure(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock EventBus
	mockEventBus := NewMockEventBusForAdapter()

	// 创建 EventBus 适配器
	adapter := adapters.NewEventBusAdapter(mockEventBus)
	defer adapter.Close()

	// 创建 Outbox 组件
	repo := NewMockRepository()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, config)

	// 启动 ACK 监听器
	ctx := context.Background()
	publisher.StartACKListener(ctx)
	defer publisher.StopACKListener()

	// 创建测试事件
	event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
	err := repo.Save(ctx, event)
	helper.RequireNoError(err)

	// 发布事件
	err = publisher.PublishEvent(ctx, event)
	helper.AssertNoError(err, "PublishEvent should succeed")

	// 模拟 EventBus 发送 ACK 失败
	mockEventBus.SendACKFailure(event.ID, "Order-events", "order-123", "OrderCreated", errors.New("kafka broker unavailable"))

	// 等待 ACK 处理
	time.Sleep(200 * time.Millisecond)

	// 验证事件状态保持 Pending
	updatedEvent, err := repo.FindByID(ctx, event.ID)
	helper.RequireNoError(err)
	helper.AssertNotNil(updatedEvent, "Event should exist")
	helper.AssertEqual(outbox.EventStatusPending, updatedEvent.Status, "Event should remain Pending for retry")
}

// TestEventBusAdapter_Integration_BatchPublish 测试 EventBus 适配器集成 - 批量发布
func TestEventBusAdapter_Integration_BatchPublish(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock EventBus
	mockEventBus := NewMockEventBusForAdapter()

	// 创建 EventBus 适配器
	adapter := adapters.NewEventBusAdapter(mockEventBus)
	defer adapter.Close()

	// 创建 Outbox 组件
	repo := NewMockRepository()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, config)

	// 启动 ACK 监听器
	ctx := context.Background()
	publisher.StartACKListener(ctx)
	defer publisher.StopACKListener()

	// 创建多个测试事件
	events := []*outbox.OutboxEvent{
		helper.CreateTestEvent(1, "Order", "order-1", "OrderCreated"),
		helper.CreateTestEvent(1, "Order", "order-2", "OrderCreated"),
		helper.CreateTestEvent(1, "Order", "order-3", "OrderCreated"),
	}

	for _, event := range events {
		err := repo.Save(ctx, event)
		helper.RequireNoError(err)
	}

	// 批量发布事件
	successCount, err := publisher.PublishBatch(ctx, events)
	helper.AssertNoError(err, "PublishBatch should succeed")
	helper.AssertEqual(len(events), successCount, "All events should be published")

	// 模拟 EventBus 发送 ACK
	mockEventBus.SendACKSuccess(events[0].ID, "Order-events", "order-1", "OrderCreated")
	mockEventBus.SendACKSuccess(events[1].ID, "Order-events", "order-2", "OrderCreated")
	mockEventBus.SendACKSuccess(events[2].ID, "Order-events", "order-3", "OrderCreated")

	// 等待 ACK 处理
	time.Sleep(300 * time.Millisecond)

	// 验证所有事件都已发布
	for i, event := range events {
		updatedEvent, err := repo.FindByID(ctx, event.ID)
		helper.RequireNoError(err)
		helper.AssertEqual(outbox.EventStatusPublished, updatedEvent.Status, "Event %d should be Published", i+1)
	}

	// 验证 EventBus 收到了所有 Envelope
	helper.AssertEqual(3, mockEventBus.GetPublishedEnvelopeCount(), "Should have 3 published envelopes")
}

// TestEventBusAdapter_Integration_AdapterClose 测试 EventBus 适配器关闭
func TestEventBusAdapter_Integration_AdapterClose(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock EventBus
	mockEventBus := NewMockEventBusForAdapter()

	// 创建 EventBus 适配器
	adapter := adapters.NewEventBusAdapter(mockEventBus)

	// 验证适配器已启动
	helper.AssertTrue(adapter.IsStarted(), "Adapter should be started")

	// 关闭适配器
	err := adapter.Close()
	helper.AssertNoError(err, "Close should succeed")

	// 验证适配器已停止
	helper.AssertFalse(adapter.IsStarted(), "Adapter should be stopped")

	// 重复关闭应该是安全的
	err = adapter.Close()
	helper.AssertNoError(err, "Second close should succeed")
}

// TestEventBusAdapter_Integration_GetEventBus 测试获取底层 EventBus
func TestEventBusAdapter_Integration_GetEventBus(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock EventBus
	mockEventBus := NewMockEventBusForAdapter()

	// 创建 EventBus 适配器
	adapter := adapters.NewEventBusAdapter(mockEventBus)
	defer adapter.Close()

	// 获取底层 EventBus
	eventBus := adapter.GetEventBus()
	helper.AssertNotNil(eventBus, "EventBus should not be nil")
}

// TestEventBusAdapter_Integration_EnvelopeConversion 测试 Envelope 转换
func TestEventBusAdapter_Integration_EnvelopeConversion(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock EventBus
	mockEventBus := NewMockEventBusForAdapter()

	// 创建 EventBus 适配器
	adapter := adapters.NewEventBusAdapter(mockEventBus)
	defer adapter.Close()

	// 创建 Outbox 组件
	repo := NewMockRepository()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, config)

	// 启动 ACK 监听器
	ctx := context.Background()
	publisher.StartACKListener(ctx)
	defer publisher.StopACKListener()

	// 创建测试事件（带完整字段）
	event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
	err := repo.Save(ctx, event)
	helper.RequireNoError(err)

	// 发布事件
	err = publisher.PublishEvent(ctx, event)
	helper.AssertNoError(err)

	// 等待发布完成
	time.Sleep(100 * time.Millisecond)

	// 验证 Envelope 转换
	envelopes := mockEventBus.GetPublishedEnvelopes()
	helper.AssertEqual(1, len(envelopes), "Should have 1 envelope")

	envelope := envelopes[0]

	// 验证所有字段都正确转换
	helper.AssertEqual(event.ID, envelope.EventID, "EventID should match")
	helper.AssertEqual(event.AggregateID, envelope.AggregateID, "AggregateID should match")
	helper.AssertEqual(event.EventType, envelope.EventType, "EventType should match")
	helper.AssertNotNil(envelope.Payload, "Payload should not be nil")
	helper.AssertNotEmpty(envelope.Timestamp, "Timestamp should not be empty")

	// 验证 Payload 内容
	helper.AssertGreater(len(envelope.Payload), 0, "Payload should have content")
}
