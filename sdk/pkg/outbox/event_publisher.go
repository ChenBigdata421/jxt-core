package outbox

import (
	"context"
	"time"
)

// EventPublisher 事件发布器接口
// Outbox 组件通过这个接口发布事件，不依赖具体的 EventBus 实现
// 这是依赖注入的核心，符合依赖倒置原则（DIP）
//
// 设计原则：
// 1. 最小化接口：只包含 Outbox 需要的方法
// 2. 零外部依赖：不依赖任何其他包
// 3. 清晰语义：方法签名简单明了
//
// 使用方式：
// 业务微服务需要创建一个适配器，将具体的 EventBus 实现适配为 EventPublisher 接口
//
// 示例（推荐使用 PublishEnvelope）：
//
//	type EventBusAdapter struct {
//	    eventBus eventbus.EventBus
//	}
//
//	func (a *EventBusAdapter) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
//	    return a.eventBus.PublishEnvelope(ctx, topic, envelope)
//	}
//
//	func (a *EventBusAdapter) GetPublishResultChannel() <-chan *PublishResult {
//	    return a.eventBus.GetPublishResultChannel()
//	}
type EventPublisher interface {
	// PublishEnvelope 发布 Envelope 消息（推荐）
	// ✅ 支持 Outbox 模式：通过 GetPublishResultChannel() 获取 ACK 结果
	// ✅ 可靠投递：不容许丢失的领域事件必须使用此方法
	//
	// 参数：
	//   ctx: 上下文（可能包含租户信息、追踪信息等）
	//   topic: 目标 topic（由 TopicMapper 提供）
	//   envelope: Envelope 消息（包含 EventID、AggregateID、EventType 等元数据）
	//
	// 返回：
	//   error: 提交失败时返回错误（注意：立即返回，不等待 ACK）
	//
	// 注意：
	//   - 此方法是异步的，立即返回，不等待 ACK
	//   - ACK 结果通过 GetPublishResultChannel() 异步通知
	//   - 实现者应该记录日志
	//   - 实现者应该处理超时
	PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error

	// GetPublishResultChannel 获取异步发布结果通道
	// ⚠️ 仅 PublishEnvelope() 发送 ACK 结果到此通道
	// 用于 Outbox Processor 监听发布结果并更新 Outbox 状态
	//
	// 返回：
	//   <-chan *PublishResult: 只读的发布结果通道
	GetPublishResultChannel() <-chan *PublishResult
}

// Envelope Envelope 消息结构（与 eventbus.Envelope 兼容）
// 为了避免 Outbox 包依赖 EventBus 包，这里定义一个简化的 Envelope 结构
type Envelope struct {
	// EventID 事件唯一ID（必填，用于 Outbox 模式）
	EventID string `json:"event_id"`
	// AggregateID 聚合根ID
	AggregateID string `json:"aggregate_id"`
	// EventType 事件类型
	EventType string `json:"event_type"`
	// EventVersion 事件版本
	EventVersion int64 `json:"event_version"`
	// Payload 事件负载
	Payload []byte `json:"payload"`
	// Timestamp 事件时间戳
	Timestamp time.Time `json:"timestamp"`
	// TraceID 链路追踪ID（可选）
	TraceID string `json:"trace_id,omitempty"`
	// CorrelationID 关联ID（可选）
	CorrelationID string `json:"correlation_id,omitempty"`
	// TenantID 租户ID（多租户支持，用于Outbox ACK路由）
	TenantID int `json:"tenant_id,omitempty"`
}

// PublishResult 异步发布结果
type PublishResult struct {
	// EventID 事件ID（来自 Envelope.EventID）
	EventID string
	// Topic 主题
	Topic string
	// Success 是否成功
	Success bool
	// Error 错误信息（失败时）
	Error error
	// Timestamp 发布时间戳
	Timestamp time.Time
	// AggregateID 聚合ID（可选，来自 Envelope）
	AggregateID string
	// EventType 事件类型（可选，来自 Envelope）
	EventType string
	// TenantID 租户ID（多租户支持，用于Outbox ACK路由）
	TenantID int
}

// NoOpEventPublisher 空操作 EventPublisher（用于测试）
type NoOpEventPublisher struct {
	resultChan chan *PublishResult
}

// PublishEnvelope 实现 EventPublisher 接口（什么都不做，立即返回成功）
func (n *NoOpEventPublisher) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
	// 模拟异步 ACK 成功
	if n.resultChan != nil {
		go func() {
			n.resultChan <- &PublishResult{
				EventID:     envelope.EventID,
				Topic:       topic,
				Success:     true,
				Timestamp:   time.Now(),
				AggregateID: envelope.AggregateID,
				EventType:   envelope.EventType,
			}
		}()
	}
	return nil
}

// GetPublishResultChannel 实现 EventPublisher 接口
func (n *NoOpEventPublisher) GetPublishResultChannel() <-chan *PublishResult {
	if n.resultChan == nil {
		n.resultChan = make(chan *PublishResult, 100)
	}
	return n.resultChan
}

// NewNoOpEventPublisher 创建空操作 EventPublisher
func NewNoOpEventPublisher() EventPublisher {
	return &NoOpEventPublisher{
		resultChan: make(chan *PublishResult, 100),
	}
}
