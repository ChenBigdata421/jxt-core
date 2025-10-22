package adapters

import (
	"context"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// EventBusAdapter EventBus 适配器
// 将 jxt-core/sdk/pkg/eventbus.EventBus 适配为 outbox.EventPublisher 接口
//
// 功能：
// 1. ✅ 转换 Outbox Envelope 为 EventBus Envelope
// 2. ✅ 转换 EventBus PublishResult 为 Outbox PublishResult
// 3. ✅ 自动启动 ACK 结果转换 goroutine
// 4. ✅ 线程安全，支持并发调用
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
//	defer publisher.StopACKListener()
//
//	// 5. 发布事件
//	event := outbox.NewOutboxEvent(...)
//	publisher.PublishEvent(ctx, event)
type EventBusAdapter struct {
	// eventBus EventBus 实例
	eventBus eventbus.EventBus

	// outboxResultChan Outbox 发布结果通道
	outboxResultChan chan *outbox.PublishResult

	// stopChan 停止信号通道
	stopChan chan struct{}

	// started 是否已启动
	started bool

	// mu 互斥锁
	mu sync.Mutex
}

// NewEventBusAdapter 创建 EventBus 适配器
//
// 参数：
//
//	eventBus: EventBus 实例（Kafka 或 NATS）
//
// 返回：
//
//	*EventBusAdapter: 适配器实例
//
// 注意：
//   - 适配器会自动启动 ACK 结果转换 goroutine
//   - 使用完毕后应该调用 Close() 释放资源
func NewEventBusAdapter(eventBus eventbus.EventBus) *EventBusAdapter {
	adapter := &EventBusAdapter{
		eventBus:         eventBus,
		outboxResultChan: make(chan *outbox.PublishResult, 1000), // 缓冲区 1000
		stopChan:         make(chan struct{}),
		started:          false,
	}

	// 启动 ACK 结果转换 goroutine
	adapter.start()

	return adapter
}

// PublishEnvelope 实现 outbox.EventPublisher 接口
// 发布 Envelope 消息到 EventBus
//
// 参数：
//
//	ctx: 上下文
//	topic: 目标 topic
//	envelope: Outbox Envelope
//
// 返回：
//
//	error: 发布失败时返回错误（注意：立即返回，不等待 ACK）
func (a *EventBusAdapter) PublishEnvelope(ctx context.Context, topic string, envelope *outbox.Envelope) error {
	// 转换 Outbox Envelope 为 EventBus Envelope
	eventBusEnvelope := a.toEventBusEnvelope(envelope)

	// 调用 EventBus 的 PublishEnvelope 方法
	// ✅ 异步发布，立即返回
	// ✅ ACK 结果通过 GetPublishResultChannel() 异步通知
	return a.eventBus.PublishEnvelope(ctx, topic, eventBusEnvelope)
}

// GetPublishResultChannel 实现 outbox.EventPublisher 接口
// 获取异步发布结果通道
//
// 返回：
//
//	<-chan *outbox.PublishResult: 只读的发布结果通道
func (a *EventBusAdapter) GetPublishResultChannel() <-chan *outbox.PublishResult {
	return a.outboxResultChan
}

// Close 关闭适配器，释放资源
// 应该在应用关闭时调用
func (a *EventBusAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.started {
		return nil
	}

	// 发送停止信号
	close(a.stopChan)

	// 等待 goroutine 退出
	time.Sleep(100 * time.Millisecond)

	// 关闭 Outbox 结果通道
	close(a.outboxResultChan)

	a.started = false

	return nil
}

// start 启动 ACK 结果转换 goroutine
func (a *EventBusAdapter) start() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return
	}

	a.started = true

	// 启动 ACK 结果转换 goroutine
	go a.resultConversionLoop()
}

// resultConversionLoop ACK 结果转换循环
// 从 EventBus 的 PublishResultChannel 读取结果，转换后发送到 Outbox 的 PublishResultChannel
func (a *EventBusAdapter) resultConversionLoop() {
	// 获取 EventBus 的发布结果通道
	eventBusResultChan := a.eventBus.GetPublishResultChannel()

	for {
		select {
		case eventBusResult, ok := <-eventBusResultChan:
			if !ok {
				// EventBus 结果通道已关闭
				return
			}

			// 转换 EventBus PublishResult 为 Outbox PublishResult
			outboxResult := a.toOutboxPublishResult(eventBusResult)

			// 发送到 Outbox 结果通道
			select {
			case a.outboxResultChan <- outboxResult:
				// 成功发送
			case <-a.stopChan:
				// 收到停止信号
				return
			default:
				// 通道满，丢弃结果（避免阻塞）
				// 注意：这种情况很少发生，因为缓冲区足够大
			}

		case <-a.stopChan:
			// 收到停止信号
			return
		}
	}
}

// toEventBusEnvelope 转换 Outbox Envelope 为 EventBus Envelope
func (a *EventBusAdapter) toEventBusEnvelope(envelope *outbox.Envelope) *eventbus.Envelope {
	return &eventbus.Envelope{
		EventID:       envelope.EventID,
		AggregateID:   envelope.AggregateID,
		EventType:     envelope.EventType,
		EventVersion:  envelope.EventVersion,
		Timestamp:     envelope.Timestamp,
		TraceID:       envelope.TraceID,
		CorrelationID: envelope.CorrelationID,
		Payload:       eventbus.RawMessage(envelope.Payload),
	}
}

// toOutboxPublishResult 转换 EventBus PublishResult 为 Outbox PublishResult
func (a *EventBusAdapter) toOutboxPublishResult(result *eventbus.PublishResult) *outbox.PublishResult {
	return &outbox.PublishResult{
		EventID:     result.EventID,
		Topic:       result.Topic,
		Success:     result.Success,
		Error:       result.Error,
		Timestamp:   result.Timestamp,
		AggregateID: result.AggregateID,
		EventType:   result.EventType,
	}
}

// ========== 辅助方法 ==========

// IsStarted 检查适配器是否已启动
func (a *EventBusAdapter) IsStarted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.started
}

// GetEventBus 获取底层 EventBus 实例
// 用于需要直接访问 EventBus 的场景
func (a *EventBusAdapter) GetEventBus() eventbus.EventBus {
	return a.eventBus
}

// ========== 工厂方法 ==========

// NewKafkaEventBusAdapter 创建 Kafka EventBus 适配器
//
// 参数：
//
//	kafkaConfig: Kafka 配置
//
// 返回：
//
//	*EventBusAdapter: 适配器实例
//	error: 创建失败时返回错误
//
// 使用示例：
//
//	adapter, err := NewKafkaEventBusAdapter(kafkaConfig)
//	if err != nil {
//	    panic(err)
//	}
//	defer adapter.Close()
func NewKafkaEventBusAdapter(kafkaConfig *eventbus.KafkaConfig) (*EventBusAdapter, error) {
	// 创建 Kafka EventBus
	eventBus, err := eventbus.NewKafkaEventBus(kafkaConfig)
	if err != nil {
		return nil, err
	}

	// 创建适配器
	return NewEventBusAdapter(eventBus), nil
}

// NewNATSEventBusAdapter 创建 NATS EventBus 适配器
//
// 参数：
//
//	natsConfig: NATS 配置
//
// 返回：
//
//	*EventBusAdapter: 适配器实例
//	error: 创建失败时返回错误
//
// 使用示例：
//
//	adapter, err := NewNATSEventBusAdapter(natsConfig)
//	if err != nil {
//	    panic(err)
//	}
//	defer adapter.Close()
func NewNATSEventBusAdapter(natsConfig *eventbus.NATSConfig) (*EventBusAdapter, error) {
	// 创建 NATS EventBus
	eventBus, err := eventbus.NewNATSEventBus(natsConfig)
	if err != nil {
		return nil, err
	}

	// 创建适配器
	return NewEventBusAdapter(eventBus), nil
}

