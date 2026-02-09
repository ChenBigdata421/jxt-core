package event

import (
	"time"

	"github.com/google/uuid"
)

// EnterpriseDomainEvent 企业级领域事件结构
// 在BaseDomainEvent基础上增加企业级通用字段
// 适用于：多租户SaaS系统、企业级应用
type EnterpriseDomainEvent struct {
	BaseDomainEvent

	// ========== 企业级通用字段 ==========
	// 租户隔离：多租户系统的核心字段
	// 多租户系统：使用实际的租户ID（如 1, 2, 3）
	// 单租户系统：使用 0 表示全局/无租户
	TenantId int `json:"tenantId" gorm:"type:int;index;column:tenant_id;comment:租户ID"`

	// ========== 可观测性字段 ==========
	// 用于分布式追踪和因果链路分析
	CorrelationId string `json:"correlationId,omitempty" gorm:"type:varchar(255);index;column:correlation_id;comment:业务关联ID"`
	CausationId   string `json:"causationId,omitempty" gorm:"type:varchar(255);index;column:causation_id;comment:因果事件ID"`
	TraceId       string `json:"traceId,omitempty" gorm:"type:varchar(255);index;column:trace_id;comment:分布式追踪ID"`

	// 可扩展的企业级字段（根据需要启用）
	// OrganizationId string `json:"organizationId,omitempty" gorm:"type:varchar(255);index;column:organization_id;comment:组织ID"`
}

// NewEnterpriseDomainEvent 创建企业级领域事件
func NewEnterpriseDomainEvent(eventType string, aggregateID interface{}, aggregateType string, payload interface{}) *EnterpriseDomainEvent {
	return &EnterpriseDomainEvent{
		BaseDomainEvent: BaseDomainEvent{
			EventID:       uuid.Must(uuid.NewV7()).String(),
			EventType:     eventType,
			OccurredAt:    time.Now(),
			Version:       1,
			AggregateID:   convertAggregateIDToString(aggregateID),
			AggregateType: aggregateType,
			Payload:       payload,
		},
		TenantId: 0, // 默认为 0，由业务层根据需要设置实际租户ID
	}
}

// 企业级特定方法
func (e *EnterpriseDomainEvent) GetTenantId() int   { return e.TenantId }
func (e *EnterpriseDomainEvent) SetTenantId(id int) { e.TenantId = id }

// 可观测性方法
func (e *EnterpriseDomainEvent) GetCorrelationId() string { return e.CorrelationId }
func (e *EnterpriseDomainEvent) SetCorrelationId(id string) {
	e.CorrelationId = id
}
func (e *EnterpriseDomainEvent) GetCausationId() string { return e.CausationId }
func (e *EnterpriseDomainEvent) SetCausationId(id string) {
	e.CausationId = id
}
func (e *EnterpriseDomainEvent) GetTraceId() string { return e.TraceId }
func (e *EnterpriseDomainEvent) SetTraceId(id string) {
	e.TraceId = id
}

