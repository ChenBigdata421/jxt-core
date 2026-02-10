package function_regression_tests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// TestAsyncACK_PublishEnvelopeSuccess 测试异步 ACK 成功场景
func TestAsyncACK_PublishEnvelopeSuccess(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock 组件
	repo := NewMockRepository()
	publisher := NewMockAsyncEventPublisher()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	// 启动 ACK 监听器
	ctx := context.Background()
	outboxPublisher.StartACKListener(ctx)
	defer outboxPublisher.StopACKListener()

	// 创建测试事件
	event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
	err := repo.Save(ctx, event)
	helper.RequireNoError(err)

	// 发布事件
	err = outboxPublisher.PublishEvent(ctx, event)
	helper.AssertNoError(err, "PublishEvent should succeed")

	// 模拟 ACK 成功
	publisher.SendACKSuccess(event.ID, "Order-events", "order-123", "OrderCreated")

	// 等待 ACK 处理
	time.Sleep(200 * time.Millisecond)

	// 验证事件状态已更新为 Published
	updatedEvent, err := repo.FindByID(ctx, event.ID)
	helper.RequireNoError(err)
	helper.AssertNotNil(updatedEvent, "Event should exist")
	helper.AssertEqual(outbox.EventStatusPublished, updatedEvent.Status, "Event should be marked as Published")
	helper.AssertNotNil(updatedEvent.PublishedAt, "PublishedAt should be set")
}

// TestAsyncACK_PublishEnvelopeFailure 测试异步 ACK 失败场景
func TestAsyncACK_PublishEnvelopeFailure(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock 组件
	repo := NewMockRepository()
	publisher := NewMockAsyncEventPublisher()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	// 启动 ACK 监听器
	ctx := context.Background()
	outboxPublisher.StartACKListener(ctx)
	defer outboxPublisher.StopACKListener()

	// 创建测试事件
	event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
	err := repo.Save(ctx, event)
	helper.RequireNoError(err)

	// 发布事件
	err = outboxPublisher.PublishEvent(ctx, event)
	helper.AssertNoError(err, "PublishEvent should succeed")

	// 模拟 ACK 失败
	publisher.SendACKFailure(event.ID, "Order-events", "order-123", "OrderCreated", errors.New("kafka broker unavailable"))

	// 等待 ACK 处理
	time.Sleep(200 * time.Millisecond)

	// 验证事件状态保持 Pending（等待重试）
	updatedEvent, err := repo.FindByID(ctx, event.ID)
	helper.RequireNoError(err)
	helper.AssertNotNil(updatedEvent, "Event should exist")
	helper.AssertEqual(outbox.EventStatusPending, updatedEvent.Status, "Event should remain Pending for retry")
	helper.AssertNil(updatedEvent.PublishedAt, "PublishedAt should not be set")
}

// TestAsyncACK_BatchPublish 测试批量发布的异步 ACK
func TestAsyncACK_BatchPublish(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock 组件
	repo := NewMockRepository()
	publisher := NewMockAsyncEventPublisher()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	// 启动 ACK 监听器
	ctx := context.Background()
	outboxPublisher.StartACKListener(ctx)
	defer outboxPublisher.StopACKListener()

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
	successCount, err := outboxPublisher.PublishBatch(ctx, events)
	helper.AssertNoError(err, "PublishBatch should succeed")
	helper.AssertEqual(len(events), successCount, "All events should be published")

	// 模拟部分成功、部分失败
	publisher.SendACKSuccess(events[0].ID, "Order-events", "order-1", "OrderCreated")
	publisher.SendACKFailure(events[1].ID, "Order-events", "order-2", "OrderCreated", errors.New("timeout"))
	publisher.SendACKSuccess(events[2].ID, "Order-events", "order-3", "OrderCreated")

	// 等待 ACK 处理
	time.Sleep(300 * time.Millisecond)

	// 验证事件状态
	event1, _ := repo.FindByID(ctx, events[0].ID)
	helper.AssertEqual(outbox.EventStatusPublished, event1.Status, "Event 1 should be Published")

	event2, _ := repo.FindByID(ctx, events[1].ID)
	helper.AssertEqual(outbox.EventStatusPending, event2.Status, "Event 2 should remain Pending")

	event3, _ := repo.FindByID(ctx, events[2].ID)
	helper.AssertEqual(outbox.EventStatusPublished, event3.Status, "Event 3 should be Published")
}

// TestAsyncACK_ConcurrentPublish 测试并发发布的异步 ACK
func TestAsyncACK_ConcurrentPublish(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock 组件
	repo := NewMockRepository()
	publisher := NewMockAsyncEventPublisher()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	// 启动 ACK 监听器
	ctx := context.Background()
	outboxPublisher.StartACKListener(ctx)
	defer outboxPublisher.StopACKListener()

	// 并发发布多个事件
	goroutines := 10
	eventsPerGoroutine := 10
	totalEvents := goroutines * eventsPerGoroutine

	var wg sync.WaitGroup
	eventIDs := make([]string, 0, totalEvents)
	var mu sync.Mutex

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := helper.CreateTestEvent(1, "Order", "order-"+string(rune(index*eventsPerGoroutine+j)), "OrderCreated")
				err := repo.Save(ctx, event)
				if err != nil {
					return
				}

				err = outboxPublisher.PublishEvent(ctx, event)
				if err != nil {
					return
				}

				mu.Lock()
				eventIDs = append(eventIDs, event.ID)
				mu.Unlock()

				// 模拟 ACK 成功
				publisher.SendACKSuccess(event.ID, "Order-events", event.AggregateID, "OrderCreated")
			}
		}(i)
	}

	wg.Wait()

	// 等待所有 ACK 处理完成
	time.Sleep(500 * time.Millisecond)

	// 验证所有事件都已发布
	publishedCount := 0
	for _, eventID := range eventIDs {
		event, _ := repo.FindByID(ctx, eventID)
		if event != nil && event.Status == outbox.EventStatusPublished {
			publishedCount++
		}
	}

	helper.AssertEqual(len(eventIDs), publishedCount, "All events should be published")
}

// TestAsyncACK_ListenerStartStop 测试 ACK 监听器启动和停止
func TestAsyncACK_ListenerStartStop(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock 组件
	repo := NewMockRepository()
	publisher := NewMockAsyncEventPublisher()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	ctx := context.Background()

	// 测试启动监听器
	outboxPublisher.StartACKListener(ctx)

	// 创建并发布事件
	event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
	err := repo.Save(ctx, event)
	helper.RequireNoError(err)

	err = outboxPublisher.PublishEvent(ctx, event)
	helper.AssertNoError(err)

	// 模拟 ACK 成功
	publisher.SendACKSuccess(event.ID, "Order-events", "order-123", "OrderCreated")

	// 等待 ACK 处理
	time.Sleep(200 * time.Millisecond)

	// 验证事件已发布
	updatedEvent, _ := repo.FindByID(ctx, event.ID)
	helper.AssertEqual(outbox.EventStatusPublished, updatedEvent.Status, "Event should be published")

	// 停止监听器
	outboxPublisher.StopACKListener()

	// 等待监听器停止
	time.Sleep(100 * time.Millisecond)

	// 创建新事件并发布
	event2 := helper.CreateTestEvent(1, "Order", "order-456", "OrderCreated")
	err = repo.Save(ctx, event2)
	helper.RequireNoError(err)

	err = outboxPublisher.PublishEvent(ctx, event2)
	helper.AssertNoError(err)

	// 模拟 ACK 成功
	publisher.SendACKSuccess(event2.ID, "Order-events", "order-456", "OrderCreated")

	// 等待（但监听器已停止，不应处理）
	time.Sleep(200 * time.Millisecond)

	// 验证事件状态未更新（因为监听器已停止）
	updatedEvent2, _ := repo.FindByID(ctx, event2.ID)
	helper.AssertEqual(outbox.EventStatusPending, updatedEvent2.Status, "Event should remain Pending (listener stopped)")
}

// TestAsyncACK_EnvelopeConversion 测试 Envelope 转换
func TestAsyncACK_EnvelopeConversion(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建 Mock 组件
	repo := NewMockRepository()
	publisher := NewMockAsyncEventPublisher()
	topicMapper := NewMockTopicMapper()
	config := GetDefaultPublisherConfig()

	// 创建 Outbox Publisher
	outboxPublisher := outbox.NewOutboxPublisher(repo, publisher, topicMapper, config)

	// 启动 ACK 监听器
	ctx := context.Background()
	outboxPublisher.StartACKListener(ctx)
	defer outboxPublisher.StopACKListener()

	// 创建测试事件
	event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
	err := repo.Save(ctx, event)
	helper.RequireNoError(err)

	// 发布事件
	err = outboxPublisher.PublishEvent(ctx, event)
	helper.AssertNoError(err)

	// 验证 PublishEnvelope 被调用
	helper.AssertGreater(publisher.GetPublishedEnvelopeCount(), 0, "PublishEnvelope should be called")

	// 获取发布的 Envelope
	envelopes := publisher.GetPublishedEnvelopes()
	helper.AssertEqual(1, len(envelopes), "Should have 1 envelope")

	envelope := envelopes[0]
	helper.AssertEqual(event.ID, envelope.EventID, "EventID should match")
	helper.AssertEqual(event.AggregateID, envelope.AggregateID, "AggregateID should match")
	helper.AssertEqual(event.EventType, envelope.EventType, "EventType should match")
	helper.AssertNotNil(envelope.Payload, "Payload should not be nil")
}
