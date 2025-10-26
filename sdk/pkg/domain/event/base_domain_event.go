package event

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BaseDomainEvent 基础领域事件结构
// 包含所有事件驱动系统都需要的核心字段
// 适用于：所有使用事件驱动架构的系统
type BaseDomainEvent struct {
	// ========== 技术基础字段 ==========
	EventID    string    `json:"eventId" gorm:"type:char(36);primary_key;column:event_id;comment:事件ID"`
	EventType  string    `json:"eventType" gorm:"type:varchar(255);index;column:event_type;comment:事件类型"`
	OccurredAt time.Time `json:"occurredAt" gorm:"type:datetime;index;column:occurred_at;comment:事件发生时间"`
	Version    int       `json:"version" gorm:"type:int;column:event_version;comment:事件版本"`

	// ========== DDD核心概念 ==========
	AggregateID   string `json:"aggregateId" gorm:"type:varchar(255);index;column:aggregate_id;comment:聚合根ID"`
	AggregateType string `json:"aggregateType" gorm:"type:varchar(255);index;column:aggregate_type;comment:聚合根类型"`

	// ========== 事件载荷 ==========
	Payload interface{} `json:"payload" gorm:"-"` // 不持久化到数据库
}

// NewBaseDomainEvent 创建基础领域事件
func NewBaseDomainEvent(eventType string, aggregateID interface{}, aggregateType string, payload interface{}) *BaseDomainEvent {
	return &BaseDomainEvent{
		EventID:       uuid.Must(uuid.NewV7()).String(), // UUIDv7保证时序性
		EventType:     eventType,
		OccurredAt:    time.Now(),
		Version:       1,
		AggregateID:   convertAggregateIDToString(aggregateID),
		AggregateType: aggregateType,
		Payload:       payload,
	}
}

// convertAggregateIDToString 将任意类型的聚合根ID转换为string
func convertAggregateIDToString(aggregateID interface{}) string {
	switch v := aggregateID.(type) {
	case string:
		return v
	case int64:
		return fmt.Sprintf("%d", v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Getter方法
func (e *BaseDomainEvent) GetEventID() string       { return e.EventID }
func (e *BaseDomainEvent) GetEventType() string     { return e.EventType }
func (e *BaseDomainEvent) GetOccurredAt() time.Time { return e.OccurredAt }
func (e *BaseDomainEvent) GetVersion() int          { return e.Version }
func (e *BaseDomainEvent) GetAggregateID() string   { return e.AggregateID }
func (e *BaseDomainEvent) GetAggregateType() string { return e.AggregateType }
func (e *BaseDomainEvent) GetPayload() interface{}  { return e.Payload }

