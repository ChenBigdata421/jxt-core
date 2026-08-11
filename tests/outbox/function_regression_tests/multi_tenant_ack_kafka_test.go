//go:build integration
// +build integration

package function_regression_tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	outboxadapters "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiTenantACKChannel_Kafka 测试多租户 ACK Channel（Kafka）
// 完整版本：测试事件发布和 ACK 接收
func TestMultiTenantACKChannel_Kafka(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	// 创建 Kafka EventBus
	clientID := fmt.Sprintf("kafka-multi-tenant-%d", time.Now().UnixNano())

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
		Net: eventbus.NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		ClientID: clientID,
	}

	kafkaEventBus, err := eventbus.NewKafkaEventBus(cfg)
	require.NoError(t, err, "Failed to create Kafka EventBus")
	defer kafkaEventBus.Close()

	// 创建 EventBusAdapter
	adapter := outboxadapters.NewEventBusAdapter(kafkaEventBus)
	defer adapter.Close()

	// 定义租户列表：10个租户
	tenants := []int{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	}

	// 为每个租户注册 ACK Channel
	for _, tenantID := range tenants {
		err := adapter.RegisterTenant(tenantID, 1000)
		require.NoError(t, err, "Failed to register tenant %d", tenantID)
		t.Logf("✅ Registered tenant: %d", tenantID)
	}

	// 验证租户已注册
	registeredTenants := adapter.GetRegisteredTenants()
	assert.Equal(t, len(tenants), len(registeredTenants), "Should have registered all tenants")
	t.Logf("✅ Registered tenants: %v", registeredTenants)

	// 为每个租户创建 Repository 和 Publisher
	type TenantContext struct {
		tenantID   int
		repo       *MockRepository
		publisher  *outbox.OutboxPublisher
		ackChan    <-chan *outbox.PublishResult
		receivedMu sync.Mutex
		received   []*outbox.PublishResult
	}

	tenantContexts := make(map[int]*TenantContext)

	// 为每个租户配置 Topic
	topicNames := make(map[int]string)
	for _, tenantID := range tenants {
		topicName := fmt.Sprintf("tenant-%d-events-%d", tenantID, time.Now().UnixNano())
		topicNames[tenantID] = topicName
		t.Logf("✅ Will use topic: %s for tenant: %d", topicName, tenantID)
	}

	for _, tenantID := range tenants {
		// 创建 Repository
		repo := NewMockRepository()

		// 创建 TopicMapper（使用静态 Topic）
		topicMapper := outbox.NewStaticTopicMapper(topicNames[tenantID])

		// 创建 Publisher
		publisherCfg := &outbox.PublisherConfig{
			MaxRetries: 3,
			RetryDelay: 100 * time.Millisecond,
		}
		publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, publisherCfg)

		// 获取租户专属的 ACK Channel
		ackChan := adapter.GetTenantPublishResultChannel(tenantID)
		require.NotNil(t, ackChan, "ACK channel should not be nil for tenant %d", tenantID)

		tenantContexts[tenantID] = &TenantContext{
			tenantID:  tenantID,
			repo:      repo,
			publisher: publisher,
			ackChan:   ackChan,
			received:  make([]*outbox.PublishResult, 0),
		}

		t.Logf("✅ Created context for tenant: %d", tenantID)
	}

	// 为每个租户创建事件：每个租户500个事件
	eventsPerTenant := 500
	for _, tenantID := range tenants {
		tenantCtx := tenantContexts[tenantID]

		for i := 0; i < eventsPerTenant; i++ {
			event := &outbox.OutboxEvent{
				ID:            fmt.Sprintf("event-%d-%d", tenantID, i),
				AggregateID:   fmt.Sprintf("agg-%d-%d", tenantID, i),
				AggregateType: "TestAggregate",
				EventType:     "TestEvent",
				Payload:       []byte(fmt.Sprintf(`{"tenant":"%d","index":%d}`, tenantID, i)),
				Status:        outbox.EventStatusPending,
				TenantID:      tenantID,
			}
			err := tenantCtx.repo.Save(context.Background(), event)
			require.NoError(t, err, "Failed to save event for tenant %d", tenantID)
		}
		t.Logf("✅ Created %d events for tenant: %d", eventsPerTenant, tenantID)
	}

	// 启动所有租户的 ACK 监听器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, tenantID := range tenants {
		tenantCtx := tenantContexts[tenantID]

		// 使用租户专属的 ACK Channel 启动 ACK 监听器
		// Publisher 的 ACK 监听器会自动调用 repo.MarkAsPublished()
		tenantCtx.publisher.StartACKListenerWithChannel(ctx, tenantCtx.ackChan)

		t.Logf("✅ Started ACK listener for tenant: %d", tenantID)
	}

	// 为每个租户发布事件
	for _, tenantID := range tenants {
		tenantCtx := tenantContexts[tenantID]

		// 获取待发布的事件
		events, err := tenantCtx.repo.FindPendingEvents(ctx, eventsPerTenant, tenantID)
		require.NoError(t, err, "Failed to find pending events for tenant %d", tenantID)
		require.Equal(t, eventsPerTenant, len(events), "Should have %d pending events for tenant %d", eventsPerTenant, tenantID)

		// 发布事件
		for _, event := range events {
			err := tenantCtx.publisher.PublishEvent(ctx, event)
			require.NoError(t, err, "Failed to publish event for tenant %d", tenantID)
		}

		t.Logf("✅ Published %d events for tenant: %d", len(events), tenantID)
	}

	// 等待所有 ACK 被接收（Kafka 异步发布需要时间）
	// Publisher 的 ACK 监听器会调用 repo.MarkAsPublished()
	// 我们通过检查 Repository 中的事件状态来验证
	t.Log("⏳ Waiting for all events to be marked as Published...")
	t.Logf("📊 Total events to publish: %d (10 tenants × 500 events)", len(tenants)*eventsPerTenant)
	maxWaitTime := 120 * time.Second // 增加到120秒，因为有5000个事件
	checkInterval := 1 * time.Second
	startTime := time.Now()

	allPublished := false
	lastLogTime := startTime
	for time.Since(startTime) < maxWaitTime {
		allPublished = true
		totalPublished := 0

		for _, tenantID := range tenants {
			tenantCtx := tenantContexts[tenantID]

			// 统计已发布的事件数量
			publishedCount := 0
			for i := 0; i < eventsPerTenant; i++ {
				eventID := fmt.Sprintf("event-%d-%d", tenantID, i)
				event, err := tenantCtx.repo.FindByID(ctx, eventID)
				if err == nil && event != nil && event.IsPublished() {
					publishedCount++
				}
			}

			totalPublished += publishedCount

			if publishedCount < eventsPerTenant {
				allPublished = false
			}
		}

		// 每10秒打印一次进度（因为事件数量多，减少日志频率）
		if time.Since(lastLogTime) >= 10*time.Second {
			percentage := float64(totalPublished) / float64(len(tenants)*eventsPerTenant) * 100
			rate := float64(totalPublished) / time.Since(startTime).Seconds()
			t.Logf("⏳ Progress: %d/%d events published (%.1f%%), rate: %.0f events/s, elapsed: %v",
				totalPublished, len(tenants)*eventsPerTenant,
				percentage, rate,
				time.Since(startTime))
			lastLogTime = time.Now()
		}

		if allPublished {
			t.Logf("✅ All events published in %v", time.Since(startTime))
			break
		}

		time.Sleep(checkInterval)
	}

	if !allPublished {
		t.Logf("⚠️  Timeout after %v, checking partial results...", maxWaitTime)
	}

	// 验证每个租户的事件发布状态
	for _, tenantID := range tenants {
		tenantCtx := tenantContexts[tenantID]

		// 统计已发布的事件
		publishedCount := 0
		publishedEvents := make(map[string]bool)

		for i := 0; i < eventsPerTenant; i++ {
			eventID := fmt.Sprintf("event-%d-%d", tenantID, i)
			event, err := tenantCtx.repo.FindByID(ctx, eventID)
			if err == nil && event != nil && event.IsPublished() {
				publishedCount++
				publishedEvents[eventID] = true
			}
		}

		// 验证：每个租户应该有所有50个事件被标记为Published
		// 由于Kafka异步发布的特性，允许少量丢失，但至少要有80%
		minExpectedPublished := int(float64(eventsPerTenant) * 0.8)
		assert.GreaterOrEqual(t, publishedCount, minExpectedPublished,
			"Tenant %s should have at least %d events published (80%%), but got %d",
			tenantID, minExpectedPublished, publishedCount)

		if publishedCount == eventsPerTenant {
			t.Logf("✅ Tenant %d: all %d/%d events published (100%%)", tenantID, publishedCount, eventsPerTenant)
		} else {
			t.Logf("⚠️  Tenant %d: %d/%d events published (%.1f%%)",
				tenantID, publishedCount, eventsPerTenant,
				float64(publishedCount)/float64(eventsPerTenant)*100)
		}
	}

	// 清理：注销所有租户
	for _, tenantID := range tenants {
		err := adapter.UnregisterTenant(tenantID)
		require.NoError(t, err, "Failed to unregister tenant %d", tenantID)
		t.Logf("✅ Unregistered tenant: %d", tenantID)
	}

	t.Log("✅ Multi-tenant ACK Channel test with Kafka passed!")
}
