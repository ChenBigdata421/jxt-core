package adapters

import (
	"context"
	"sync"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
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

	// PR2-core (Task 4, ce-doc-review #2): loopsWg joins ALL conversion goroutines
	// (the global resultConversionLoop spawned in start() AND every per-tenant
	// tenantResultConversionLoop spawned in GetTenantPublishResultChannel). Close()
	// signals exit via close(stopChan), then loopsWg.Wait() — a deterministic join
	// that replaces the old time.Sleep(100ms). Add(1) BEFORE each `go`; defer Done()
	// as the first statement inside each loop (never Add after the go — races Done).
	loopsWg sync.WaitGroup
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

// ========== 多租户 ACK 支持 ==========

// RegisterTenant 注册租户（创建租户专属的 ACK Channel）
// 委托给底层 EventBus 实现
func (a *EventBusAdapter) RegisterTenant(tenantID int, bufferSize int) error {
	return a.eventBus.RegisterTenant(tenantID, bufferSize)
}

// UnregisterTenant 注销租户（关闭并清理租户的 ACK Channel）
// 委托给底层 EventBus 实现
func (a *EventBusAdapter) UnregisterTenant(tenantID int) error {
	return a.eventBus.UnregisterTenant(tenantID)
}

// GetTenantPublishResultChannel 获取租户专属的异步发布结果通道（多租户模式）
// 返回一个转换后的 Outbox PublishResult 通道
//
// 注意：此方法返回的是 EventBus 的租户通道，需要调用者自行转换类型
// 推荐使用 CreateTenantResultChannel() 方法，它会自动创建转换通道
func (a *EventBusAdapter) GetTenantPublishResultChannel(tenantID int) <-chan *outbox.PublishResult {
	// 获取 EventBus 的租户专属通道
	eventBusResultChan := a.eventBus.GetTenantPublishResultChannel(tenantID)
	if eventBusResultChan == nil {
		return nil
	}

	// 创建 Outbox 结果通道
	outboxResultChan := make(chan *outbox.PublishResult, 1000)

	// PR2-core (Task 4): Add(1) BEFORE the go so Close().Wait() never misses a Done.
	// (Never Add after the go — that races with Done.)
	a.loopsWg.Add(1)
	go a.tenantResultConversionLoop(eventBusResultChan, outboxResultChan)

	return outboxResultChan
}

// GetRegisteredTenants 获取所有已注册的租户ID列表
// 委托给底层 EventBus 实现
func (a *EventBusAdapter) GetRegisteredTenants() []int {
	return a.eventBus.GetRegisteredTenants()
}

// tenantResultConversionLoop 租户 ACK 结果转换循环
// 从 EventBus 的租户专属 PublishResultChannel 读取结果，转换后发送到 Outbox 的 PublishResultChannel
//
// PR2-core (Task 4):
//   - defer loopsWg.Done() is the FIRST statement so Close().Wait() converges even on the
//     early-return paths (paired with the Add(1) before the `go` in GetTenantPublishResultChannel).
//   - The inner select has NO `default:` discard (ce-doc-review #2 / spec §2.2): on a full
//     outbox channel, block until either the send succeeds or stopChan fires. Silent drops
//     break the outbox ACK contract (a lost ACK → the outbox row is never marked published).
func (a *EventBusAdapter) tenantResultConversionLoop(eventBusResultChan <-chan *eventbus.PublishResult, outboxResultChan chan<- *outbox.PublishResult) {
	defer a.loopsWg.Done() // PR2-core (Task 4): paired with Add(1) before the `go`
	defer close(outboxResultChan)

	for {
		select {
		case eventBusResult, ok := <-eventBusResultChan:
			if !ok {
				// EventBus 结果通道已关闭
				return
			}

			// 转换 EventBus PublishResult 为 Outbox PublishResult
			outboxResult := a.toOutboxPublishResult(eventBusResult)

			// 发送到 Outbox 结果通道 — NO default: discard. On full, block until the
			// listener drains OR stopChan fires (lossless per spec §2.2).
			select {
			case outboxResultChan <- outboxResult:
				// 成功发送
			case <-a.stopChan:
				// 收到停止信号
				return
			}

		case <-a.stopChan:
			// 收到停止信号
			return
		}
	}
}

// Close 关闭适配器，释放资源。应该在应用关闭时调用。
//
// PR2-core (Task 4, Step 4 — D9 ownership split):
// Close returns its OWN teardown error honestly and does NOT close the injected bus
// ("don't close what you don't own"). The bus is injected via NewEventBusAdapter; the
// orchestrator (e.g. the outbox consumer) owns the bus lifecycle — it calls bus.Close()
// and surfaces the terminal error for the non-zero exit (spec §3.3). This adapter's
// terminal error lives on the bus and is surfaced by the bus owner, NOT here.
//
// Deterministic join (replaces the old time.Sleep(100ms), ce-doc-review #2): close(stopChan)
// signals ALL conversion loops (global + per-tenant) to exit via their <-stopChan cases,
// then loopsWg.Wait() joins them. A single Sleep could never deterministically join the
// per-tenant loops spawned by GetTenantPublishResultChannel.
//
// Today the join+close can't fail, so the return is nil — but the signature stays honest
// (never swallow a future join/close error to nil).
func (a *EventBusAdapter) Close() error {
	a.mu.Lock()
	if !a.started {
		a.mu.Unlock()
		return nil
	}
	a.started = false
	close(a.stopChan) // signal ALL conversion loops to exit (before releasing mu)
	a.mu.Unlock()

	// Deterministic join of ALL conversion loops (global + per-tenant) — no time.Sleep.
	// Each loop's <-a.stopChan case returns, firing its defer loopsWg.Done().
	a.loopsWg.Wait()

	// Close the Outbox result channel AFTER the loops have exited (no writer remains).
	close(a.outboxResultChan)

	// D9: do NOT call a.eventBus.Close() — the bus is injected; the orchestrator owns it.
	// This adapter's terminal error is nil today (join+close can't fail); the bus's
	// terminal error is surfaced by whoever owns and closes the bus.
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

	// PR2-core (Task 4): Add(1) BEFORE the go so Close().Wait() never misses a Done.
	a.loopsWg.Add(1)
	// 启动 ACK 结果转换 goroutine
	go a.resultConversionLoop()
}

// resultConversionLoop ACK 结果转换循环
// 从 EventBus 的 PublishResultChannel 读取结果，转换后发送到 Outbox 的 PublishResultChannel
//
// PR2-core (Task 4):
//   - defer loopsWg.Done() is the FIRST statement (paired with Add(1) in start()).
//   - NO `default:` discard in the inner select (spec §2.2 lossless): on full, block
//     until the listener drains OR stopChan fires.
func (a *EventBusAdapter) resultConversionLoop() {
	defer a.loopsWg.Done() // PR2-core (Task 4): paired with Add(1) in start()

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

			// 发送到 Outbox 结果通道 — NO default: discard. On full, block until the
			// listener drains OR stopChan fires (lossless per spec §2.2).
			select {
			case a.outboxResultChan <- outboxResult:
				// 成功发送
			case <-a.stopChan:
				// 收到停止信号
				return
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
		TenantID:      envelope.TenantID, // ✅ 传递租户ID (int)
		Payload:       jxtjson.RawMessage(envelope.Payload),
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
		TenantID:    result.TenantID, // ✅ 传递租户ID
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
