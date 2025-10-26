package event

import (
	"fmt"
)

// Envelope 事件信封结构
// 用于事件传输和一致性校验
type Envelope struct {
	EventType   string
	AggregateID string
	TenantID    string
	Payload     []byte
}

// ValidateConsistency 校验Envelope与DomainEvent的一致性
// 确保事件在传输过程中关键信息不被篡改或不一致
//
// 校验项：
// 1. EventType一致性
// 2. AggregateID一致性
// 3. TenantId一致性（如果是企业级事件）
//
// 使用场景：
// - Outbox适配器在保存事件前校验
// - EventHandler在处理事件前校验
// - 测试用例中验证事件完整性
func ValidateConsistency(envelope *Envelope, event BaseEvent) error {
	if envelope == nil {
		return fmt.Errorf("envelope is nil")
	}
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// 1. 校验EventType一致性
	if envelope.EventType != event.GetEventType() {
		return fmt.Errorf("eventType mismatch: envelope=%s, event=%s",
			envelope.EventType, event.GetEventType())
	}

	// 2. 校验AggregateID一致性
	if envelope.AggregateID != event.GetAggregateID() {
		return fmt.Errorf("aggregateID mismatch: envelope=%s, event=%s",
			envelope.AggregateID, event.GetAggregateID())
	}

	// 3. 如果是企业级事件，校验TenantId一致性
	if ee, ok := event.(EnterpriseEvent); ok {
		if envelope.TenantID != ee.GetTenantId() {
			return fmt.Errorf("tenantID mismatch: envelope=%s, event=%s",
				envelope.TenantID, ee.GetTenantId())
		}
	}

	return nil
}

