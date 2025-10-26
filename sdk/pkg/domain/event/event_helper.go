package event

import (
	"fmt"

	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
)

// ========== DomainEvent 序列化/反序列化 ==========
// 注意：使用 jxt-core/sdk/pkg/json 的统一 jsoniter 配置

// UnmarshalDomainEvent 反序列化完整的 DomainEvent
// 用于 Query Side 从消息队列接收事件
//
// 使用示例：
//
//	// 使用 BaseDomainEvent
//	domainEvent, err := event.UnmarshalDomainEvent[*event.BaseDomainEvent](msg.Payload)
//	if err != nil {
//	    return fmt.Errorf("failed to unmarshal domain event: %w", err)
//	}
//
//	// 使用 EnterpriseDomainEvent
//	enterpriseEvent, err := event.UnmarshalDomainEvent[*event.EnterpriseDomainEvent](msg.Payload)
//	if err != nil {
//	    return fmt.Errorf("failed to unmarshal enterprise event: %w", err)
//	}
//
// 优势：
// 1. 统一使用 jsoniter.ConfigCompatibleWithStandardLibrary
// 2. 类型安全（泛型）
// 3. 统一的错误处理
// 4. 避免各服务重复实现
//
// 注意：
// - 反序列化后，Payload 字段的类型是 map[string]interface{}（而不是原始结构体）
// - 需要使用 UnmarshalPayload 进一步提取具体的 Payload 结构体
func UnmarshalDomainEvent[T BaseEvent](data []byte) (T, error) {
	var result T

	// 验证输入
	if len(data) == 0 {
		return result, fmt.Errorf("data is empty")
	}

	// 反序列化
	if err := jxtjson.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal domain event: %w", err)
	}

	return result, nil
}

// MarshalDomainEvent 序列化完整的 DomainEvent
// 用于 Command Side 保存到 Outbox 或发布到消息队列
//
// 使用示例：
//
//	eventBytes, err := event.MarshalDomainEvent(domainEvent)
//	if err != nil {
//	    return fmt.Errorf("failed to marshal domain event: %w", err)
//	}
//
// 优势：
// 1. 统一使用 jsoniter.ConfigCompatibleWithStandardLibrary
// 2. 统一的错误处理
// 3. 避免各服务重复实现
//
// 注意：
// - 序列化时会包含所有字段（EventID, EventType, Payload 等）
// - Payload 字段会被序列化为 JSON 对象（嵌套在 DomainEvent JSON 中）
func MarshalDomainEvent(event BaseEvent) ([]byte, error) {
	// 验证输入
	if event == nil {
		return nil, fmt.Errorf("event is nil")
	}

	// 序列化
	data, err := jxtjson.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal domain event: %w", err)
	}

	return data, nil
}

// UnmarshalDomainEventFromString 从 JSON 字符串反序列化 DomainEvent
// 用于测试或特殊场景
//
// 使用示例：
//
//	domainEvent, err := event.UnmarshalDomainEventFromString[*event.BaseDomainEvent](jsonString)
//	if err != nil {
//	    return fmt.Errorf("failed to unmarshal domain event: %w", err)
//	}
func UnmarshalDomainEventFromString[T BaseEvent](jsonString string) (T, error) {
	var result T

	// 验证输入
	if jsonString == "" {
		return result, fmt.Errorf("json string is empty")
	}

	// 反序列化
	if err := jxtjson.Unmarshal([]byte(jsonString), &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal domain event from string: %w", err)
	}

	return result, nil
}

// MarshalDomainEventToString 序列化 DomainEvent 为 JSON 字符串
// 用于测试或特殊场景
//
// 使用示例：
//
//	jsonString, err := event.MarshalDomainEventToString(domainEvent)
//	if err != nil {
//	    return fmt.Errorf("failed to marshal domain event: %w", err)
//	}
func MarshalDomainEventToString(event BaseEvent) (string, error) {
	// 验证输入
	if event == nil {
		return "", fmt.Errorf("event is nil")
	}

	// 序列化
	data, err := jxtjson.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("failed to marshal domain event to string: %w", err)
	}

	return string(data), nil
}
