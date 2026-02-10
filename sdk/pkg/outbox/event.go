package outbox

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
)

// EventStatus 事件状态
type EventStatus string

const (
	// EventStatusPending 待发布
	EventStatusPending EventStatus = "pending"
	// EventStatusPublished 已发布
	EventStatusPublished EventStatus = "published"
	// EventStatusFailed 发布失败
	EventStatusFailed EventStatus = "failed"
	// EventStatusMaxRetry 超过最大重试次数
	EventStatusMaxRetry EventStatus = "max_retry"
)

// OutboxEvent Outbox 事件领域模型
// 完全数据库无关，不包含任何数据库标签
type OutboxEvent struct {
	// ID 事件唯一标识（UUID）
	ID string

	// TenantID 租户 ID（多租户支持）
	TenantID int

	// AggregateID 聚合根 ID
	AggregateID string

	// AggregateType 聚合根类型（用于 Topic 映射）
	// 例如："Archive", "Media", "User"
	AggregateType string

	// EventType 事件类型
	// 例如："ArchiveCreated", "MediaUploaded", "UserRegistered"
	EventType string

	// Payload 事件负载（JSON 格式）
	Payload jxtjson.RawMessage

	// Status 事件状态
	Status EventStatus

	// RetryCount 重试次数
	RetryCount int

	// MaxRetries 最大重试次数
	MaxRetries int

	// LastError 最后一次错误信息
	LastError string

	// CreatedAt 创建时间
	CreatedAt time.Time

	// UpdatedAt 更新时间
	UpdatedAt time.Time

	// PublishedAt 发布时间
	PublishedAt *time.Time

	// ScheduledAt 计划发布时间（延迟发布）
	ScheduledAt *time.Time

	// LastRetryAt 最后重试时间
	LastRetryAt *time.Time

	// Version 事件版本（用于事件演化）
	Version int64

	// TraceID 链路追踪ID（用于分布式追踪）
	// 用于追踪事件在多个微服务之间的流转
	TraceID string

	// CorrelationID 关联ID（用于关联相关事件）
	// 用于关联同一业务流程中的多个事件（例如：Saga 模式）
	CorrelationID string

	// IdempotencyKey 幂等性键（用于防止重复发布）
	// 格式建议：{TenantID}:{AggregateType}:{AggregateID}:{EventType}:{Timestamp}
	// 或者使用业务自定义的唯一标识
	// 用于在发布前检查是否已经发布过相同的事件
	IdempotencyKey string
}

// NewOutboxEvent 创建新的 Outbox 事件
// payload 必须是 jxtevent.BaseEvent 类型（BaseDomainEvent 或 EnterpriseDomainEvent）
// 使用 event 组件的序列化方法确保统一性和性能
func NewOutboxEvent(
	tenantID int,
	aggregateID string,
	aggregateType string,
	eventType string,
	payload jxtevent.BaseEvent,
) (*OutboxEvent, error) {
	// 使用 event 组件的序列化方法
	payloadBytes, err := jxtevent.MarshalDomainEvent(payload)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	eventID := generateID()

	// 生成默认的幂等性键
	// 格式：{TenantID}:{AggregateType}:{AggregateID}:{EventType}:{EventID}
	// 使用 EventID 确保唯一性，同时包含业务语义
	idempotencyKey := generateIdempotencyKey(tenantID, aggregateType, aggregateID, eventType, eventID)

	return &OutboxEvent{
		ID:             eventID,
		TenantID:       tenantID,
		AggregateID:    aggregateID,
		AggregateType:  aggregateType,
		EventType:      eventType,
		Payload:        payloadBytes,
		Status:         EventStatusPending,
		RetryCount:     0,
		MaxRetries:     3, // 默认最大重试 3 次
		CreatedAt:      now,
		UpdatedAt:      now,
		Version:        1,  // 默认版本为 1
		TraceID:        "", // 默认为空，由业务代码设置
		CorrelationID:  "", // 默认为空，由业务代码设置
		IdempotencyKey: idempotencyKey,
	}, nil
}

// CanRetry 判断是否可以重试
func (e *OutboxEvent) CanRetry() bool {
	return e.RetryCount < e.MaxRetries
}

// MarkAsPublished 标记为已发布
func (e *OutboxEvent) MarkAsPublished() {
	now := time.Now()
	e.Status = EventStatusPublished
	e.PublishedAt = &now
	e.UpdatedAt = now
}

// MarkAsFailed 标记为失败
func (e *OutboxEvent) MarkAsFailed(err error) {
	e.Status = EventStatusFailed
	e.RetryCount++
	if err != nil {
		e.LastError = err.Error()
	}
	now := time.Now()
	e.LastRetryAt = &now
	e.UpdatedAt = now
}

// IncrementRetry 增加重试次数
func (e *OutboxEvent) IncrementRetry(errorMessage string) {
	e.RetryCount++
	e.LastError = errorMessage
	now := time.Now()
	e.LastRetryAt = &now
	e.UpdatedAt = now

	// 如果超过最大重试次数，标记为 max_retry
	if e.RetryCount >= e.MaxRetries {
		e.Status = EventStatusMaxRetry
	} else {
		e.Status = EventStatusFailed
	}
}

// MarkAsMaxRetry 标记为超过最大重试次数
func (e *OutboxEvent) MarkAsMaxRetry(errorMessage string) {
	e.Status = EventStatusMaxRetry
	e.LastError = errorMessage
	e.UpdatedAt = time.Now()
}

// ResetForRetry 重置为待发布状态（用于重试）
func (e *OutboxEvent) ResetForRetry() {
	e.Status = EventStatusPending
	e.UpdatedAt = time.Now()
}

// IsExpired 判断事件是否过期（超过指定时间未发布）
func (e *OutboxEvent) IsExpired(maxAge time.Duration) bool {
	return time.Since(e.CreatedAt) > maxAge
}

// IsPending 判断是否为待发布状态
func (e *OutboxEvent) IsPending() bool {
	return e.Status == EventStatusPending
}

// IsPublished 判断是否已发布
func (e *OutboxEvent) IsPublished() bool {
	return e.Status == EventStatusPublished
}

// IsFailed 判断是否失败
func (e *OutboxEvent) IsFailed() bool {
	return e.Status == EventStatusFailed
}

// IsMaxRetry 判断是否超过最大重试次数
func (e *OutboxEvent) IsMaxRetry() bool {
	return e.Status == EventStatusMaxRetry
}

// ShouldPublishNow 判断是否应该立即发布
func (e *OutboxEvent) ShouldPublishNow() bool {
	// 如果没有设置计划发布时间，立即发布
	if e.ScheduledAt == nil {
		return true
	}
	// 如果计划发布时间已到，立即发布
	return time.Now().After(*e.ScheduledAt)
}

// GetPayloadAs 将 Payload 反序列化为指定类型
// 推荐使用 event 组件的 UnmarshalDomainEvent 和 UnmarshalPayload 方法
func (e *OutboxEvent) GetPayloadAs(v interface{}) error {
	return jxtjson.Unmarshal(e.Payload, v)
}

// SetPayload 设置 Payload
// payload 应该是 jxtevent.BaseEvent 类型
func (e *OutboxEvent) SetPayload(payload jxtevent.BaseEvent) error {
	payloadBytes, err := jxtevent.MarshalDomainEvent(payload)
	if err != nil {
		return err
	}
	e.Payload = payloadBytes
	e.UpdatedAt = time.Now()
	return nil
}

// Clone 克隆事件（用于测试）
func (e *OutboxEvent) Clone() *OutboxEvent {
	clone := *e
	// 深拷贝 Payload
	if e.Payload != nil {
		clone.Payload = make(jxtjson.RawMessage, len(e.Payload))
		copy(clone.Payload, e.Payload)
	}
	// 深拷贝指针字段
	if e.PublishedAt != nil {
		publishedAt := *e.PublishedAt
		clone.PublishedAt = &publishedAt
	}
	if e.ScheduledAt != nil {
		scheduledAt := *e.ScheduledAt
		clone.ScheduledAt = &scheduledAt
	}
	if e.LastRetryAt != nil {
		lastRetryAt := *e.LastRetryAt
		clone.LastRetryAt = &lastRetryAt
	}
	return &clone
}

// WithTraceID 设置链路追踪ID（支持链式调用）
func (e *OutboxEvent) WithTraceID(traceID string) *OutboxEvent {
	e.TraceID = traceID
	return e
}

// WithCorrelationID 设置关联ID（支持链式调用）
func (e *OutboxEvent) WithCorrelationID(correlationID string) *OutboxEvent {
	e.CorrelationID = correlationID
	return e
}

// ToEnvelope 转换为 EventBus Envelope
// 用于将 OutboxEvent 转换为 EventBus 的统一消息包络格式
func (e *OutboxEvent) ToEnvelope() interface{} {
	// 注意：这里返回 interface{} 是为了避免循环依赖
	// 实际使用时，业务代码需要将返回值转换为 *eventbus.Envelope
	//
	// 示例：
	//   envelope := event.ToEnvelope().(*eventbus.Envelope)
	//
	// 或者在 OutboxPublisher 中直接构造 Envelope：
	//   envelope := &eventbus.Envelope{
	//       EventID:       event.ID,
	//       AggregateID:   event.AggregateID,
	//       EventType:     event.EventType,
	//       EventVersion:  event.Version,
	//       Timestamp:     event.CreatedAt,
	//       TraceID:       event.TraceID,
	//       CorrelationID: event.CorrelationID,
	//       Payload:       eventbus.RawMessage(event.Payload),
	//   }

	// 返回一个 map，避免循环依赖
	return map[string]interface{}{
		"event_id":       e.ID,
		"aggregate_id":   e.AggregateID,
		"event_type":     e.EventType,
		"event_version":  e.Version,
		"timestamp":      e.CreatedAt,
		"trace_id":       e.TraceID,
		"correlation_id": e.CorrelationID,
		"payload":        e.Payload,
	}
}

// generateID 生成唯一 ID（UUID）
// 使用 UUIDv7（RFC 9562）：基于时间戳的 UUID，具有以下优势：
// 1. 时间排序：按创建时间自然排序，适合事件溯源和数据库索引
// 2. 数据库友好：作为主键时索引性能更好（相比 UUID v4）
// 3. 分布式友好：包含时间戳和随机性，避免冲突
// 4. 全局唯一：在分布式系统中保证唯一性
//
// 降级策略：如果 UUIDv7 生成失败（理论上不会发生，除非系统时钟异常），
// 则降级使用 UUIDv4（完全随机）
func generateID() string {
	// 优先使用 UUIDv7（时间排序）
	id, err := uuid.NewV7()
	if err != nil {
		// 降级到 UUIDv4（完全随机）
		// 注意：uuid.New() 内部使用 uuid.NewRandom()，即 UUIDv4
		id = uuid.New()
	}
	return id.String()
}

// generateIdempotencyKey 生成幂等性键
// 格式：{TenantID}:{AggregateType}:{AggregateID}:{EventType}:{EventID}
// 这个键用于防止重复发布相同的事件
//
// 参数：
//   - tenantID: 租户 ID
//   - aggregateType: 聚合类型
//   - aggregateID: 聚合根 ID
//   - eventType: 事件类型
//   - eventID: 事件 ID
//
// 返回：
//   - string: 幂等性键
//
// 示例：
//
//	1:Archive:archive-123:ArchiveCreated:019a04f8-cf89-78d0-b643-beda84a98a59
func generateIdempotencyKey(tenantID int, aggregateType, aggregateID, eventType, eventID string) string {
	// 使用冒号分隔各个部分，便于解析和调试
	// 注意：如果业务需要自定义幂等性键，可以通过 WithIdempotencyKey 方法设置
	tenantIDStr := fmt.Sprintf("%d", tenantID)
	return tenantIDStr + ":" + aggregateType + ":" + aggregateID + ":" + eventType + ":" + eventID
}

// WithIdempotencyKey 设置自定义幂等性键（支持链式调用）
// 用于业务代码自定义幂等性键的生成逻辑
//
// 参数：
//   - idempotencyKey: 自定义的幂等性键
//
// 返回：
//   - *OutboxEvent: 事件对象（支持链式调用）
//
// 示例：
//
//	event.WithIdempotencyKey("custom-key-123")
func (e *OutboxEvent) WithIdempotencyKey(idempotencyKey string) *OutboxEvent {
	e.IdempotencyKey = idempotencyKey
	return e
}

// GetIdempotencyKey 获取幂等性键
func (e *OutboxEvent) GetIdempotencyKey() string {
	return e.IdempotencyKey
}
