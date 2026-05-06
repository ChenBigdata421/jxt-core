package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// EventHandler 进程内事件处理器
type EventHandler func(ctx context.Context, envelope *outbox.Envelope) error

// InProcessOption 进程内发布器配置选项
type InProcessOption func(*InProcessEventPublisher)

// WithBufferSize 设置 ACK result channel 缓冲区大小，默认 1000
func WithBufferSize(n int) InProcessOption {
	return func(p *InProcessEventPublisher) {
		if n > 0 {
			p.bufferSize = n
		}
	}
}

// InProcessEventPublisher 进程内事件发布器，实现 outbox.EventPublisher 接口。
// 与 EventBusAdapter（Kafka/NATS）对称，将事件派发到进程内注册的 Go handler 而非外部消息总线。
//
// 使用场景：微服务内部领域事件派发（如 IAM 设备管理），事件不跨服务、不经过 broker。
//
// 使用示例见 examples/evidence_management_adapter.go 底部的 InProcessEventPublisher 示例。
type InProcessEventPublisher struct {
	handlers   map[string][]EventHandler
	mu         sync.RWMutex
	resultChan chan *outbox.PublishResult
	bufferSize int
	closeOnce  sync.Once
}

// 确保 InProcessEventPublisher 实现 outbox.EventPublisher 接口（编译时检查）
var _ outbox.EventPublisher = (*InProcessEventPublisher)(nil)

func NewInProcessEventPublisher(opts ...InProcessOption) *InProcessEventPublisher {
	p := &InProcessEventPublisher{
		handlers:   make(map[string][]EventHandler),
		bufferSize: 1000,
	}

	for _, opt := range opts {
		opt(p)
	}

	p.resultChan = make(chan *outbox.PublishResult, p.bufferSize)

	return p
}

// RegisterHandler 注册事件处理器（线程安全）。同一 topic 可注册多个 handler，按注册顺序依次调用。
// 必须在 Scheduler 启动前调用。
func (p *InProcessEventPublisher) RegisterHandler(topic string, handler EventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[topic] = append(p.handlers[topic], handler)
}

// PublishEnvelope 实现 outbox.EventPublisher 接口。
// 同步调用该 topic 下的所有 handler，全部完成后发送 ACK result。
// 所有 handler 都会被调用（不因中途失败而短路），返回第一个遇到的 error。
func (p *InProcessEventPublisher) PublishEnvelope(ctx context.Context, topic string, envelope *outbox.Envelope) error {
	if envelope == nil {
		return fmt.Errorf("envelope must not be nil")
	}

	p.mu.RLock()
	handlers := p.handlers[topic]
	handlersCopy := make([]EventHandler, len(handlers))
	copy(handlersCopy, handlers)
	p.mu.RUnlock()

	if len(handlersCopy) == 0 {
		p.sendResult(&outbox.PublishResult{
			EventID:     envelope.EventID,
			Topic:       topic,
			Success:     true,
			Timestamp:   time.Now(),
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
			TenantID:    envelope.TenantID,
		})
		return nil
	}

	var firstErr error
	for _, handler := range handlersCopy {
		if err := handler(ctx, envelope); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	result := &outbox.PublishResult{
		EventID:     envelope.EventID,
		Topic:       topic,
		Success:     firstErr == nil,
		Error:       firstErr,
		Timestamp:   time.Now(),
		AggregateID: envelope.AggregateID,
		EventType:   envelope.EventType,
		TenantID:    envelope.TenantID,
	}
	p.sendResult(result)

	return firstErr
}

// GetPublishResultChannel 实现 outbox.EventPublisher 接口
func (p *InProcessEventPublisher) GetPublishResultChannel() <-chan *outbox.PublishResult {
	return p.resultChan
}

// Close 关闭发布器。可安全多次调用（sync.Once）。
func (p *InProcessEventPublisher) Close() error {
	p.closeOnce.Do(func() {
		close(p.resultChan)
	})
	return nil
}

// sendResult 非阻塞发送 ACK result。recover() 防止 Close() 后 send on closed channel panic。
func (p *InProcessEventPublisher) sendResult(result *outbox.PublishResult) {
	defer func() {
		recover() // send on closed channel — safe to ignore during shutdown
	}()
	select {
	case p.resultChan <- result:
	default:
	}
}
