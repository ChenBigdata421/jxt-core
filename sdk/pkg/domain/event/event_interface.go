package event

import "time"

// BaseEvent 基础事件接口
// 所有事件都必须实现此接口
type BaseEvent interface {
	GetEventID() string
	GetEventType() string
	GetOccurredAt() time.Time
	GetVersion() int
	GetAggregateID() string
	GetAggregateType() string
	GetPayload() interface{}
}

// EnterpriseEvent 企业级事件接口
// 多租户系统和需要可观测性支持的系统应实现此接口
type EnterpriseEvent interface {
	BaseEvent

	// 租户隔离
	GetTenantId() int
	SetTenantId(int)

	// 可观测性方法
	GetCorrelationId() string
	SetCorrelationId(string)
	GetCausationId() string
	SetCausationId(string)
	GetTraceId() string
	SetTraceId(string)
}

