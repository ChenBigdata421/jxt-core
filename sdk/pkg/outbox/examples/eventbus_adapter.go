package examples

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// EventBus 接口（简化版本，与 jxt-core/sdk/pkg/eventbus.EventBus 兼容）
// 这里只定义 Outbox 需要的方法
type EventBus interface {
	// PublishEnvelope 发布 Envelope 消息（领域事件）
	// ✅ 支持 Outbox 模式：通过 GetPublishResultChannel() 获取 ACK 结果
	// ✅ 可靠投递：不容许丢失的领域事件必须使用此方法
	PublishEnvelope(ctx context.Context, topic string, envelope *EventBusEnvelope) error

	// GetPublishResultChannel 获取异步发布结果通道
	// ⚠️ 仅 PublishEnvelope() 发送 ACK 结果到此通道
	// ⚠️ Publish() 不发送 ACK 结果（不支持 Outbox 模式）
	// 用于 Outbox Processor 监听发布结果并更新 Outbox 状态
	GetPublishResultChannel() <-chan *EventBusPublishResult
}

// EventBusEnvelope EventBus Envelope 结构（与 eventbus.Envelope 兼容）
type EventBusEnvelope struct {
	EventID       string `json:"event_id"`
	AggregateID   string `json:"aggregate_id"`
	EventType     string `json:"event_type"`
	EventVersion  int64  `json:"event_version"`
	Payload       []byte `json:"payload"`
	Timestamp     string `json:"timestamp"`
	TraceID       string `json:"trace_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// EventBusPublishResult EventBus 发布结果（与 eventbus.PublishResult 兼容）
type EventBusPublishResult struct {
	EventID     string
	Topic       string
	Success     bool
	Error       error
	Timestamp   string
	AggregateID string
	EventType   string
}

// EventBusAdapter EventBus 适配器
// 将 EventBus 接口适配为 Outbox 的 EventPublisher 接口
//
// 使用示例：
//
//	// 1. 创建 EventBus 实例
//	eventBus, err := eventbus.NewKafkaEventBus(kafkaConfig)
//	if err != nil {
//	    panic(err)
//	}
//
//	// 2. 创建适配器
//	adapter := NewEventBusAdapter(eventBus)
//
//	// 3. 创建 Outbox Publisher
//	publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, config)
//
//	// 4. 启动 ACK 监听器
//	publisher.StartACKListener(ctx)
//
//	// 5. 发布事件
//	event := outbox.NewOutboxEvent(...)
//	publisher.PublishEvent(ctx, event)
type EventBusAdapter struct {
	eventBus EventBus
}

// NewEventBusAdapter 创建 EventBus 适配器
func NewEventBusAdapter(eventBus EventBus) *EventBusAdapter {
	return &EventBusAdapter{
		eventBus: eventBus,
	}
}

// PublishEnvelope 实现 outbox.EventPublisher 接口
func (a *EventBusAdapter) PublishEnvelope(ctx context.Context, topic string, envelope *outbox.Envelope) error {
	// 转换 Outbox Envelope 为 EventBus Envelope
	eventBusEnvelope := &EventBusEnvelope{
		EventID:       envelope.EventID,
		AggregateID:   envelope.AggregateID,
		EventType:     envelope.EventType,
		EventVersion:  envelope.EventVersion,
		Payload:       envelope.Payload,
		Timestamp:     envelope.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		TraceID:       envelope.TraceID,
		CorrelationID: envelope.CorrelationID,
	}

	// 调用 EventBus 的 PublishEnvelope 方法
	return a.eventBus.PublishEnvelope(ctx, topic, eventBusEnvelope)
}

// GetPublishResultChannel 实现 outbox.EventPublisher 接口
func (a *EventBusAdapter) GetPublishResultChannel() <-chan *outbox.PublishResult {
	// 获取 EventBus 的发布结果通道
	eventBusResultChan := a.eventBus.GetPublishResultChannel()

	// 创建 Outbox 的发布结果通道
	outboxResultChan := make(chan *outbox.PublishResult, 100)

	// 启动转换 goroutine
	go func() {
		defer close(outboxResultChan)

		for eventBusResult := range eventBusResultChan {
			// 转换 EventBus PublishResult 为 Outbox PublishResult
			outboxResult := &outbox.PublishResult{
				EventID:     eventBusResult.EventID,
				Topic:       eventBusResult.Topic,
				Success:     eventBusResult.Success,
				Error:       eventBusResult.Error,
				AggregateID: eventBusResult.AggregateID,
				EventType:   eventBusResult.EventType,
			}

			// 发送到 Outbox 结果通道
			outboxResultChan <- outboxResult
		}
	}()

	return outboxResultChan
}

// ========== 完整使用示例 ==========

// OutboxService Outbox 服务示例
type OutboxService struct {
	publisher *outbox.OutboxPublisher
	repo      outbox.OutboxRepository
}

// NewOutboxService 创建 Outbox 服务
func NewOutboxService(
	eventBus EventBus,
	repo outbox.OutboxRepository,
	topicMapper outbox.TopicMapper,
) *OutboxService {
	// 1. 创建 EventBus 适配器
	adapter := NewEventBusAdapter(eventBus)

	// 2. 创建 Publisher 配置
	config := &outbox.PublisherConfig{
		MaxRetries:         3,
		PublishTimeout:     30000000000, // 30 seconds in nanoseconds
		EnableMetrics:      true,
		ConcurrentPublish:  true,
		PublishConcurrency: 10,
	}

	// 3. 创建 Outbox Publisher
	publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, config)

	return &OutboxService{
		publisher: publisher,
		repo:      repo,
	}
}

// Start 启动 Outbox 服务
func (s *OutboxService) Start(ctx context.Context) error {
	// 启动 ACK 监听器
	// ✅ 监听 EventBus 的异步发布结果
	// ✅ ACK 成功时标记事件为 Published
	// ✅ ACK 失败时保持 Pending 状态，等待重试
	s.publisher.StartACKListener(ctx)

	fmt.Println("✅ Outbox ACK Listener started")
	return nil
}

// Stop 停止 Outbox 服务
func (s *OutboxService) Stop() {
	// 停止 ACK 监听器
	s.publisher.StopACKListener()

	fmt.Println("✅ Outbox ACK Listener stopped")
}

// PublishEvent 发布事件
func (s *OutboxService) PublishEvent(ctx context.Context, event *outbox.OutboxEvent) error {
	// 发布事件
	// ✅ 使用 PublishEnvelope() 异步发布
	// ✅ 立即返回，不等待 ACK
	// ✅ ACK 结果通过 ACK 监听器异步处理
	return s.publisher.PublishEvent(ctx, event)
}

// PublishPendingEvents 发布待发布的事件
func (s *OutboxService) PublishPendingEvents(ctx context.Context, limit int, tenantID string) (int, error) {
	// 批量发布待发布的事件
	// ✅ 查询待发布的事件
	// ✅ 批量发布到 EventBus
	// ✅ ACK 结果通过 ACK 监听器异步处理
	return s.publisher.PublishPendingEvents(ctx, limit, tenantID)
}

// ========== 使用示例 ==========

/*
func main() {
	// 1. 创建 EventBus 实例（Kafka 或 NATS）
	eventBus, err := eventbus.NewKafkaEventBus(kafkaConfig)
	if err != nil {
		panic(err)
	}

	// 2. 创建 Outbox Repository
	repo := outbox.NewGormOutboxRepository(db)

	// 3. 创建 Topic Mapper
	topicMapper := outbox.NewSimpleTopicMapper(map[string]string{
		"Order":   "business.orders",
		"Payment": "business.payments",
	})

	// 4. 创建 Outbox 服务
	service := NewOutboxService(eventBus, repo, topicMapper)

	// 5. 启动 ACK 监听器
	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		panic(err)
	}
	defer service.Stop()

	// 6. 发布事件
	event := outbox.NewOutboxEvent(
		"order-123",
		"Order",
		"OrderCreated",
		[]byte(`{"orderId":"order-123","amount":99.99}`),
	)

	if err := service.PublishEvent(ctx, event); err != nil {
		panic(err)
	}

	// 7. 定时发布待发布的事件（可选）
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			count, err := service.PublishPendingEvents(ctx, 100, "")
			if err != nil {
				log.Printf("Failed to publish pending events: %v", err)
			} else if count > 0 {
				log.Printf("Published %d pending events", count)
			}

		case <-ctx.Done():
			return
		}
	}
}
*/
