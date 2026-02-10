package gorm

import (
	"time"

	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// OutboxEventModel GORM 数据库模型
// 包含 GORM 标签，用于数据库映射
type OutboxEventModel struct {
	// ID 事件唯一标识（UUID）
	ID string `gorm:"type:char(36);primary_key;comment:事件ID"`

	// TenantID 租户 ID
	TenantID int `gorm:"type:int;not null;index:idx_tenant_status;default:0;comment:租户ID"`

	// AggregateID 聚合根 ID
	AggregateID string `gorm:"type:varchar(255);not null;index:idx_aggregate;comment:聚合根ID"`

	// AggregateType 聚合根类型
	AggregateType string `gorm:"type:varchar(100);not null;index:idx_aggregate_type;comment:聚合根类型"`

	// EventType 事件类型
	EventType string `gorm:"type:varchar(100);not null;comment:事件类型"`

	// Payload 事件负载（JSON）
	Payload jxtjson.RawMessage `gorm:"type:json;comment:事件负载"`

	// Status 事件状态
	Status string `gorm:"type:varchar(20);not null;index:idx_tenant_status;index:idx_status;comment:事件状态"`

	// RetryCount 重试次数
	RetryCount int `gorm:"type:int;not null;default:0;comment:重试次数"`

	// MaxRetries 最大重试次数
	MaxRetries int `gorm:"type:int;not null;default:3;comment:最大重试次数"`

	// LastError 最后一次错误信息
	LastError string `gorm:"type:text;comment:最后错误"`

	// CreatedAt 创建时间
	CreatedAt time.Time `gorm:"type:datetime;not null;index:idx_created_at;comment:创建时间"`

	// UpdatedAt 更新时间
	UpdatedAt time.Time `gorm:"type:datetime;not null;comment:更新时间"`

	// PublishedAt 发布时间
	PublishedAt *time.Time `gorm:"type:datetime;comment:发布时间"`

	// ScheduledAt 计划发布时间
	ScheduledAt *time.Time `gorm:"type:datetime;index:idx_scheduled_at;comment:计划发布时间"`

	// LastRetryAt 最后重试时间
	LastRetryAt *time.Time `gorm:"type:datetime;comment:最后重试时间"`

	// Version 事件版本
	Version int64 `gorm:"type:bigint;not null;default:1;comment:事件版本"`

	// TraceID 链路追踪ID
	TraceID string `gorm:"type:varchar(64);index:idx_trace_id;comment:链路追踪ID"`

	// CorrelationID 关联ID
	CorrelationID string `gorm:"type:varchar(64);index:idx_correlation_id;comment:关联ID"`

	// IdempotencyKey 幂等性键（用于防止重复发布）
	// 唯一索引确保同一个幂等性键只能发布一次
	IdempotencyKey string `gorm:"type:varchar(512);uniqueIndex:idx_idempotency_key;comment:幂等性键"`
}

// TableName 指定表名
func (OutboxEventModel) TableName() string {
	return "outbox_events"
}

// ToEntity 转换为领域模型
func (m *OutboxEventModel) ToEntity() *outbox.OutboxEvent {
	return &outbox.OutboxEvent{
		ID:             m.ID,
		TenantID:       m.TenantID,
		AggregateID:    m.AggregateID,
		AggregateType:  m.AggregateType,
		EventType:      m.EventType,
		Payload:        m.Payload,
		Status:         outbox.EventStatus(m.Status),
		RetryCount:     m.RetryCount,
		MaxRetries:     m.MaxRetries,
		LastError:      m.LastError,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
		PublishedAt:    m.PublishedAt,
		ScheduledAt:    m.ScheduledAt,
		LastRetryAt:    m.LastRetryAt,
		Version:        m.Version,
		TraceID:        m.TraceID,
		CorrelationID:  m.CorrelationID,
		IdempotencyKey: m.IdempotencyKey,
	}
}

// FromEntity 从领域模型转换
func FromEntity(e *outbox.OutboxEvent) *OutboxEventModel {
	return &OutboxEventModel{
		ID:             e.ID,
		TenantID:       e.TenantID,
		AggregateID:    e.AggregateID,
		AggregateType:  e.AggregateType,
		EventType:      e.EventType,
		Payload:        e.Payload,
		Status:         string(e.Status),
		RetryCount:     e.RetryCount,
		MaxRetries:     e.MaxRetries,
		LastError:      e.LastError,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
		PublishedAt:    e.PublishedAt,
		ScheduledAt:    e.ScheduledAt,
		LastRetryAt:    e.LastRetryAt,
		Version:        e.Version,
		TraceID:        e.TraceID,
		CorrelationID:  e.CorrelationID,
		IdempotencyKey: e.IdempotencyKey,
	}
}

// ToEntities 批量转换为领域模型
func ToEntities(models []*OutboxEventModel) []*outbox.OutboxEvent {
	entities := make([]*outbox.OutboxEvent, len(models))
	for i, model := range models {
		entities[i] = model.ToEntity()
	}
	return entities
}

// FromEntities 批量从领域模型转换
func FromEntities(entities []*outbox.OutboxEvent) []*OutboxEventModel {
	models := make([]*OutboxEventModel, len(entities))
	for i, entity := range entities {
		models[i] = FromEntity(entity)
	}
	return models
}
