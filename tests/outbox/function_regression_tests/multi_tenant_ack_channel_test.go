package function_regression_tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	outboxadapters "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiTenantACKChannel_MemoryEventBus 测试多租户 ACK Channel（Memory EventBus）
func TestMultiTenantACKChannel_MemoryEventBus(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	helper := NewTestHelper(t)

	// 创建 Memory EventBus
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}
	eventBus, err := eventbus.NewEventBus(cfg)
	require.NoError(t, err, "Failed to create Memory EventBus")
	defer eventBus.Close()

	// 创建 EventBusAdapter
	adapter := outboxadapters.NewEventBusAdapter(eventBus)
	defer adapter.Close()

	// 定义租户列表
	tenants := []string{"tenant-a", "tenant-b", "tenant-c"}

	// 为每个租户注册 ACK Channel
	for _, tenantID := range tenants {
		err := adapter.RegisterTenant(tenantID, 1000)
		require.NoError(t, err, "Failed to register tenant %s", tenantID)
		t.Logf("✅ Registered tenant: %s", tenantID)
	}

	// 验证租户已注册
	registeredTenants := adapter.GetRegisteredTenants()
	assert.Equal(t, len(tenants), len(registeredTenants), "Should have registered all tenants")

	// 为每个租户创建 Outbox Publisher 和 Scheduler
	type TenantContext struct {
		tenantID  string
		repo      *MockRepository
		publisher *outbox.OutboxPublisher
		scheduler *outbox.OutboxScheduler
		ackChan   <-chan *outbox.PublishResult
	}

	tenantContexts := make([]*TenantContext, 0, len(tenants))
	topicMapper := NewMockTopicMapper()
	publisherConfig := GetDefaultPublisherConfig()

	for _, tenantID := range tenants {
		// 创建租户专属的 Repository
		repo := NewMockRepository()

		// 创建 Outbox Publisher
		publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, publisherConfig)

		// 获取租户专属的 ACK Channel
		ackChan := adapter.GetTenantPublishResultChannel(tenantID)
		require.NotNil(t, ackChan, "ACK channel should not be nil for tenant %s", tenantID)

		// 启动 ACK 监听器（使用租户专属 Channel）
		ctx := context.Background()
		publisher.StartACKListenerWithChannel(ctx, ackChan)

		tenantContexts = append(tenantContexts, &TenantContext{
			tenantID:  tenantID,
			repo:      repo,
			publisher: publisher,
			scheduler: nil, // 不使用 Scheduler，直接手动发布
			ackChan:   ackChan,
		})

		t.Logf("✅ Created Outbox Publisher for tenant: %s", tenantID)
	}

	// 为每个租户创建并发布测试事件
	ctx := context.Background()
	eventsPerTenant := 5
	for _, tc := range tenantContexts {
		for i := 0; i < eventsPerTenant; i++ {
			event := helper.CreateTestEvent(
				tc.tenantID,
				"Order",
				fmt.Sprintf("order-%s-%d", tc.tenantID, i),
				"OrderCreated",
			)
			err := tc.repo.Save(ctx, event)
			require.NoError(t, err, "Failed to save event for tenant %s", tc.tenantID)

			// 直接发布事件
			err = tc.publisher.PublishEvent(ctx, event)
			require.NoError(t, err, "Failed to publish event for tenant %s", tc.tenantID)
		}
		t.Logf("✅ Created and published %d events for tenant: %s", eventsPerTenant, tc.tenantID)
	}

	// 等待所有 ACK 处理完成
	time.Sleep(2 * time.Second)

	// 验证每个租户的事件都被标记为 Published
	for _, tc := range tenantContexts {
		publishedCount, err := tc.repo.Count(ctx, outbox.EventStatusPublished, tc.tenantID)
		require.NoError(t, err)

		t.Logf("📊 Tenant %s: %d/%d events published", tc.tenantID, publishedCount, eventsPerTenant)

		// 如果有未发布的事件，打印详细信息
		if publishedCount < int64(eventsPerTenant) {
			allEvents, _ := tc.repo.FindPendingEvents(ctx, 100, tc.tenantID)
			t.Logf("  ⚠️  Tenant %s has %d pending events", tc.tenantID, len(allEvents))
		}

		assert.Equal(t, int64(eventsPerTenant), publishedCount,
			"Tenant %s should have %d published events", tc.tenantID, eventsPerTenant)
	}

	// 停止所有 ACK Listener
	for _, tc := range tenantContexts {
		tc.publisher.StopACKListener()
	}

	// 注销所有租户
	for _, tenantID := range tenants {
		err := adapter.UnregisterTenant(tenantID)
		require.NoError(t, err, "Failed to unregister tenant %s", tenantID)
		t.Logf("✅ Unregistered tenant: %s", tenantID)
	}

	t.Log("✅ Multi-tenant ACK Channel test passed (Memory EventBus)")
}

// TestMultiTenantACKChannel_Isolation 测试租户隔离性
func TestMultiTenantACKChannel_Isolation(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	helper := NewTestHelper(t)

	// 创建 Memory EventBus
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}
	eventBus, err := eventbus.NewEventBus(cfg)
	require.NoError(t, err, "Failed to create Memory EventBus")
	defer eventBus.Close()

	// 创建 EventBusAdapter
	adapter := outboxadapters.NewEventBusAdapter(eventBus)
	defer adapter.Close()

	// 注册两个租户
	tenantA := "tenant-isolation-a"
	tenantB := "tenant-isolation-b"

	err = adapter.RegisterTenant(tenantA, 1000)
	require.NoError(t, err)
	err = adapter.RegisterTenant(tenantB, 1000)
	require.NoError(t, err)

	// 为租户 A 创建 Publisher
	repoA := NewMockRepository()
	publisherA := outbox.NewOutboxPublisher(repoA, adapter, NewMockTopicMapper(), GetDefaultPublisherConfig())
	ackChanA := adapter.GetTenantPublishResultChannel(tenantA)
	publisherA.StartACKListenerWithChannel(context.Background(), ackChanA)
	defer publisherA.StopACKListener()

	// 为租户 B 创建 Publisher
	repoB := NewMockRepository()
	publisherB := outbox.NewOutboxPublisher(repoB, adapter, NewMockTopicMapper(), GetDefaultPublisherConfig())
	ackChanB := adapter.GetTenantPublishResultChannel(tenantB)
	publisherB.StartACKListenerWithChannel(context.Background(), ackChanB)
	defer publisherB.StopACKListener()

	// 为租户 A 创建事件
	eventA := helper.CreateTestEvent(tenantA, "Order", "order-a-1", "OrderCreated")
	err = repoA.Save(context.Background(), eventA)
	require.NoError(t, err)

	// 为租户 B 创建事件
	eventB := helper.CreateTestEvent(tenantB, "Order", "order-b-1", "OrderCreated")
	err = repoB.Save(context.Background(), eventB)
	require.NoError(t, err)

	// 发布租户 A 的事件
	err = publisherA.PublishEvent(context.Background(), eventA)
	require.NoError(t, err)

	// 发布租户 B 的事件
	err = publisherB.PublishEvent(context.Background(), eventB)
	require.NoError(t, err)

	// 等待 ACK 处理
	time.Sleep(500 * time.Millisecond)

	// 验证租户 A 的事件状态
	updatedEventA, err := repoA.FindByID(context.Background(), eventA.ID)
	require.NoError(t, err)
	assert.Equal(t, outbox.EventStatusPublished, updatedEventA.Status,
		"Tenant A's event should be published")

	// 验证租户 B 的事件状态
	updatedEventB, err := repoB.FindByID(context.Background(), eventB.ID)
	require.NoError(t, err)
	assert.Equal(t, outbox.EventStatusPublished, updatedEventB.Status,
		"Tenant B's event should be published")

	// 验证租户 A 的 Repository 中没有租户 B 的事件
	eventBInRepoA, _ := repoA.FindByID(context.Background(), eventB.ID)
	assert.Nil(t, eventBInRepoA, "Tenant A's repo should not have Tenant B's event")

	// 验证租户 B 的 Repository 中没有租户 A 的事件
	eventAInRepoB, _ := repoB.FindByID(context.Background(), eventA.ID)
	assert.Nil(t, eventAInRepoB, "Tenant B's repo should not have Tenant A's event")

	// 注销租户
	adapter.UnregisterTenant(tenantA)
	adapter.UnregisterTenant(tenantB)

	t.Log("✅ Tenant isolation test passed")
}

// TestMultiTenantACKChannel_ConcurrentPublish 测试多租户并发发布
func TestMultiTenantACKChannel_ConcurrentPublish(t *testing.T) {
	// 初始化 logger
	logger.Setup()

	helper := NewTestHelper(t)

	// 创建 Memory EventBus
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}
	eventBus, err := eventbus.NewEventBus(cfg)
	require.NoError(t, err, "Failed to create Memory EventBus")
	defer eventBus.Close()

	// 创建 EventBusAdapter
	adapter := outboxadapters.NewEventBusAdapter(eventBus)
	defer adapter.Close()

	// 定义租户数量和每个租户的事件数量
	numTenants := 5
	eventsPerTenant := 20

	// 注册所有租户
	tenants := make([]string, numTenants)
	for i := 0; i < numTenants; i++ {
		tenants[i] = fmt.Sprintf("tenant-concurrent-%d", i)
		err := adapter.RegisterTenant(tenants[i], 1000)
		require.NoError(t, err)
	}

	// 为每个租户创建 Publisher
	type TenantPublisher struct {
		tenantID  string
		repo      *MockRepository
		publisher *outbox.OutboxPublisher
	}

	publishers := make([]*TenantPublisher, numTenants)
	for i, tenantID := range tenants {
		repo := NewMockRepository()
		publisher := outbox.NewOutboxPublisher(repo, adapter, NewMockTopicMapper(), GetDefaultPublisherConfig())
		ackChan := adapter.GetTenantPublishResultChannel(tenantID)
		publisher.StartACKListenerWithChannel(context.Background(), ackChan)

		publishers[i] = &TenantPublisher{
			tenantID:  tenantID,
			repo:      repo,
			publisher: publisher,
		}
	}

	// 并发发布事件
	var wg sync.WaitGroup
	var totalPublished int64

	for _, tp := range publishers {
		wg.Add(1)
		go func(tp *TenantPublisher) {
			defer wg.Done()
			for j := 0; j < eventsPerTenant; j++ {
				event := helper.CreateTestEvent(
					tp.tenantID,
					"Order",
					fmt.Sprintf("order-%s-%d", tp.tenantID, j),
					"OrderCreated",
				)
				err := tp.repo.Save(context.Background(), event)
				if err != nil {
					t.Errorf("Failed to save event: %v", err)
					return
				}

				err = tp.publisher.PublishEvent(context.Background(), event)
				if err != nil {
					t.Errorf("Failed to publish event: %v", err)
					return
				}

				atomic.AddInt64(&totalPublished, 1)
			}
		}(tp)
	}

	// 等待所有 goroutine 完成
	wg.Wait()

	// 等待 ACK 处理
	time.Sleep(1 * time.Second)

	// 验证所有事件都被发布
	expectedTotal := int64(numTenants * eventsPerTenant)
	assert.Equal(t, expectedTotal, atomic.LoadInt64(&totalPublished),
		"Should have published all events")

	// 验证每个租户的事件状态
	for _, tp := range publishers {
		publishedCount, err := tp.repo.Count(context.Background(), outbox.EventStatusPublished, tp.tenantID)
		require.NoError(t, err)
		assert.Equal(t, int64(eventsPerTenant), publishedCount,
			"Tenant %s should have %d published events", tp.tenantID, eventsPerTenant)
	}

	// 清理
	for _, tp := range publishers {
		tp.publisher.StopACKListener()
	}
	for _, tenantID := range tenants {
		adapter.UnregisterTenant(tenantID)
	}

	t.Logf("✅ Concurrent publish test passed: %d tenants × %d events = %d total",
		numTenants, eventsPerTenant, expectedTotal)
}
