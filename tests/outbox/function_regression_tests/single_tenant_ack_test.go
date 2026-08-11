//go:build integration
// +build integration

package function_regression_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	outboxadapters "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters"
	"github.com/stretchr/testify/require"
)

// TestSingleTenantACKChannel_NATS 测试非多租户模式（单租户）使用默认租户ID "*" - NATS
// 注意：Memory EventBus 不支持异步 ACK，因此不适合测试 ACK Channel 功能
func TestSingleTenantACKChannel_NATS(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	t.Log("========================================")
	t.Log("  单租户 ACK Channel 测试 (NATS JetStream)")
	t.Log("  默认租户ID: 1")
	t.Log("========================================")

	// 创建 NATS EventBus
	clientID := fmt.Sprintf("single-tenant-nats-%d", time.Now().UnixNano())
	streamName := fmt.Sprintf("TEST_STREAM_%s", clientID)
	subjectPrefix := fmt.Sprintf("%s.>", clientID)

	cfg := &eventbus.NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: clientID,
		JetStream: eventbus.JetStreamConfig{
			Enabled: true,
			Stream: eventbus.StreamConfig{
				Name:     streamName,
				Subjects: []string{subjectPrefix},
			},
		},
	}

	eventBus, err := eventbus.NewNATSEventBus(cfg)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer eventBus.Close()

	// 创建 EventBusAdapter
	adapter := outboxadapters.NewEventBusAdapter(eventBus)
	defer adapter.Close()

	// 注册默认租户 1（非多租户模式）
	defaultTenantID := 1
	err = adapter.RegisterTenant(defaultTenantID, 10000)
	require.NoError(t, err, "Failed to register default tenant")
	t.Logf("✅ Registered default tenant: %d", defaultTenantID)

	// 配置主题
	topic := fmt.Sprintf("%s.default-events", clientID)
	t.Logf("✅ Will use topic: %s", topic)

	// 创建 TopicMapper（使用静态 Topic）
	topicMapper := outbox.NewStaticTopicMapper(topic)

	// 创建 Repository 和 Publisher
	repo := NewMockRepository()
	publisherConfig := GetDefaultPublisherConfig()
	publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, publisherConfig)

	// 获取默认租户的 ACK Channel
	ackChan := adapter.GetTenantPublishResultChannel(defaultTenantID)
	require.NotNil(t, ackChan, "ACK channel should not be nil for default tenant")
	t.Logf("✅ Got ACK channel for default tenant: %d", defaultTenantID)

	// 启动 ACK 监听器
	ctx := context.Background()
	publisher.StartACKListenerWithChannel(ctx, ackChan)
	defer publisher.StopACKListener()
	t.Logf("✅ Started ACK listener for default tenant")

	// 创建测试事件（使用默认租户ID "*"）
	eventsCount := 500
	for i := 0; i < eventsCount; i++ {
		event := &outbox.OutboxEvent{
			ID:            fmt.Sprintf("event-nats-default-%d", i),
			AggregateID:   fmt.Sprintf("agg-nats-default-%d", i),
			AggregateType: "TestAggregate",
			EventType:     "TestEvent",
			Payload:       []byte(fmt.Sprintf(`{"tenant":"*","index":%d}`, i)),
			Status:        outbox.EventStatusPending,
			TenantID:      defaultTenantID, // 使用默认租户ID "*"
		}
		err := repo.Save(context.Background(), event)
		require.NoError(t, err, "Failed to save event")
	}
	t.Logf("Created %d events for tenant: %d", eventsCount, defaultTenantID)

	// 发布所有事件
	events, err := repo.FindPendingEvents(ctx, 1000, defaultTenantID)
	require.NoError(t, err)
	require.Equal(t, eventsCount, len(events), "Should have %d pending events", eventsCount)

	for _, event := range events {
		err := publisher.PublishEvent(ctx, event)
		require.NoError(t, err, "Failed to publish event %s", event.ID)
	}
	t.Logf("✅ Published %d events for default tenant", eventsCount)

	// 等待所有 ACK 被接收
	t.Log("⏳ Waiting for all events to be marked as Published...")
	t.Logf("📊 Total events to publish: %d (default tenant '*')", eventsCount)
	maxWaitTime := 30 * time.Second
	checkInterval := 1 * time.Second
	startTime := time.Now()

	allPublished := false
	lastLogTime := startTime
	for time.Since(startTime) < maxWaitTime {
		publishedCount := 0
		for i := 0; i < eventsCount; i++ {
			eventID := fmt.Sprintf("event-nats-default-%d", i)
			event, err := repo.FindByID(ctx, eventID)
			if err == nil && event != nil && event.IsPublished() {
				publishedCount++
			}
		}

		// 每5秒打印一次进度
		if time.Since(lastLogTime) >= 5*time.Second {
			percentage := float64(publishedCount) / float64(eventsCount) * 100
			rate := float64(publishedCount) / time.Since(startTime).Seconds()
			t.Logf("⏳ Progress: %d/%d events published (%.1f%%), rate: %.0f events/s, elapsed: %v",
				publishedCount, eventsCount,
				percentage, rate,
				time.Since(startTime))
			lastLogTime = time.Now()
		}

		if publishedCount == eventsCount {
			allPublished = true
			t.Logf("✅ All %d events published in %v", eventsCount, time.Since(startTime))
			break
		}

		time.Sleep(checkInterval)
	}

	require.True(t, allPublished, "Not all events were published within timeout")

	// 验证所有事件都已发布
	for i := 0; i < eventsCount; i++ {
		eventID := fmt.Sprintf("event-nats-default-%d", i)
		event, err := repo.FindByID(ctx, eventID)
		require.NoError(t, err, "Failed to find event %s", eventID)
		require.NotNil(t, event, "Event %s should exist", eventID)
		require.True(t, event.IsPublished(), "Event %s should be published", eventID)
	}

	t.Logf("%d", defaultTenantID)

	// 注销默认租户
	err = adapter.UnregisterTenant(defaultTenantID)
	require.NoError(t, err, "Failed to unregister default tenant")
	t.Logf("Unregistered tenant: %d", defaultTenantID)

	t.Log("✅ Single-tenant ACK Channel test with NATS JetStream passed!")
}

// TestSingleTenantACKChannel_Kafka 测试非多租户模式（单租户）使用默认租户ID "*" - Kafka
func TestSingleTenantACKChannel_Kafka(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	t.Log("========================================")
	t.Log("  单租户 ACK Channel 测试 (Kafka/RedPanda)")
	t.Log("  默认租户ID: *")
	t.Log("========================================")

	// 创建 Kafka EventBus
	clientID := fmt.Sprintf("single-tenant-kafka-%d", time.Now().UnixNano())

	cfg := &eventbus.KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: eventbus.ProducerConfig{
			RequiredAcks:    1,
			FlushFrequency:  10 * time.Millisecond,
			FlushMessages:   100,
			Timeout:         5 * time.Second,
			FlushBytes:      1048576,
			RetryMax:        3,
			BatchSize:       16384,
			BufferSize:      8388608,
			MaxMessageBytes: 1048576,
		},
		Consumer: eventbus.ConsumerConfig{
			GroupID:            fmt.Sprintf("test-group-%s", clientID),
			SessionTimeout:     10 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  5 * time.Second,
			AutoOffsetReset:    "earliest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
			FetchMaxWait:       100 * time.Millisecond,
			MaxPollRecords:     500,
		},
	}

	eventBus, err := eventbus.NewKafkaEventBus(cfg)
	require.NoError(t, err, "Failed to create Kafka EventBus")
	defer eventBus.Close()

	// 创建 EventBusAdapter
	adapter := outboxadapters.NewEventBusAdapter(eventBus)
	defer adapter.Close()

	// 注册默认租户 1（非多租户模式）
	defaultTenantID := 1
	err = adapter.RegisterTenant(defaultTenantID, 10000)
	require.NoError(t, err, "Failed to register default tenant")
	t.Logf("✅ Registered default tenant: %d", defaultTenantID)

	// 配置主题
	topic := fmt.Sprintf("tenant-default-events-%d", time.Now().UnixNano())
	t.Logf("✅ Will use topic: %s", topic)

	// 创建 TopicMapper（使用静态 Topic）
	topicMapper := outbox.NewStaticTopicMapper(topic)

	// 创建 Repository 和 Publisher
	repo := NewMockRepository()
	publisherConfig := GetDefaultPublisherConfig()
	publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, publisherConfig)

	// 获取默认租户的 ACK Channel
	ackChan := adapter.GetTenantPublishResultChannel(defaultTenantID)
	require.NotNil(t, ackChan, "ACK channel should not be nil for default tenant")
	t.Logf("✅ Got ACK channel for default tenant: %d", defaultTenantID)

	// 启动 ACK 监听器
	ctx := context.Background()
	publisher.StartACKListenerWithChannel(ctx, ackChan)
	defer publisher.StopACKListener()
	t.Logf("✅ Started ACK listener for default tenant")

	// 创建测试事件（使用默认租户ID "*"）
	eventsCount := 500
	for i := 0; i < eventsCount; i++ {
		event := &outbox.OutboxEvent{
			ID:            fmt.Sprintf("event-kafka-default-%d", i),
			AggregateID:   fmt.Sprintf("agg-kafka-default-%d", i),
			AggregateType: "TestAggregate",
			EventType:     "TestEvent",
			Payload:       []byte(fmt.Sprintf(`{"tenant":"*","index":%d}`, i)),
			Status:        outbox.EventStatusPending,
			TenantID:      defaultTenantID, // 使用默认租户ID "*"
		}
		err := repo.Save(context.Background(), event)
		require.NoError(t, err, "Failed to save event")
	}
	t.Logf("Created %d events for tenant: %d", eventsCount, defaultTenantID)

	// 发布所有事件
	events, err := repo.FindPendingEvents(ctx, 1000, defaultTenantID)
	require.NoError(t, err)
	require.Equal(t, eventsCount, len(events), "Should have %d pending events", eventsCount)

	for _, event := range events {
		err := publisher.PublishEvent(ctx, event)
		require.NoError(t, err, "Failed to publish event %s", event.ID)
	}
	t.Logf("✅ Published %d events for default tenant", eventsCount)

	// 等待所有 ACK 被接收
	t.Log("⏳ Waiting for all events to be marked as Published...")
	t.Logf("📊 Total events to publish: %d (default tenant '*')", eventsCount)
	maxWaitTime := 30 * time.Second
	checkInterval := 1 * time.Second
	startTime := time.Now()

	allPublished := false
	lastLogTime := startTime
	for time.Since(startTime) < maxWaitTime {
		publishedCount := 0
		for i := 0; i < eventsCount; i++ {
			eventID := fmt.Sprintf("event-kafka-default-%d", i)
			event, err := repo.FindByID(ctx, eventID)
			if err == nil && event != nil && event.IsPublished() {
				publishedCount++
			}
		}

		// 每5秒打印一次进度
		if time.Since(lastLogTime) >= 5*time.Second {
			percentage := float64(publishedCount) / float64(eventsCount) * 100
			rate := float64(publishedCount) / time.Since(startTime).Seconds()
			t.Logf("⏳ Progress: %d/%d events published (%.1f%%), rate: %.0f events/s, elapsed: %v",
				publishedCount, eventsCount,
				percentage, rate,
				time.Since(startTime))
			lastLogTime = time.Now()
		}

		if publishedCount == eventsCount {
			allPublished = true
			t.Logf("✅ All %d events published in %v", eventsCount, time.Since(startTime))
			break
		}

		time.Sleep(checkInterval)
	}

	require.True(t, allPublished, "Not all events were published within timeout")

	// 验证所有事件都已发布
	for i := 0; i < eventsCount; i++ {
		eventID := fmt.Sprintf("event-kafka-default-%d", i)
		event, err := repo.FindByID(ctx, eventID)
		require.NoError(t, err, "Failed to find event %s", eventID)
		require.NotNil(t, event, "Event %s should exist", eventID)
		require.True(t, event.IsPublished(), "Event %s should be published", eventID)
	}

	t.Logf("%d", defaultTenantID)

	// 注销默认租户
	err = adapter.UnregisterTenant(defaultTenantID)
	require.NoError(t, err, "Failed to unregister default tenant")
	t.Logf("Unregistered tenant: %d", defaultTenantID)

	t.Log("✅ Single-tenant ACK Channel test with Kafka passed!")
}
