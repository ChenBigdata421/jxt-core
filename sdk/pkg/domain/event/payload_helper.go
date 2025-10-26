package event

import (
	"fmt"

	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
)

// UnmarshalPayload 标准化的Payload反序列化助手
// 解决各服务自行处理序列化导致的不一致问题
//
// 使用示例：
//
//	payload, err := event.UnmarshalPayload[MediaUploadedPayload](domainEvent)
//	if err != nil {
//	    return fmt.Errorf("failed to unmarshal payload: %w", err)
//	}
//
// 优势：
// 1. 统一使用jsoniter.ConfigCompatibleWithStandardLibrary
// 2. 类型安全（泛型）
// 3. 自动处理 interface{} → map[string]interface{} → 结构体 的转换
// 4. 支持 []byte 和 RawMessage 类型的 Payload
// 5. 统一的错误处理
// 6. 避免各服务重复实现
//
// 注意：
// - 当 Payload 是 interface{} 类型且从 JSON 反序列化时，实际类型是 map[string]interface{}
// - 本方法会自动处理这种情况，将 map 转换为目标结构体
func UnmarshalPayload[T any](ev BaseEvent) (T, error) {
	var result T

	// 获取原始Payload
	payload := ev.GetPayload()
	if payload == nil {
		return result, fmt.Errorf("payload is nil")
	}

	// 如果payload已经是目标类型，直接返回
	if typedPayload, ok := payload.(T); ok {
		return typedPayload, nil
	}

	// 如果payload是 []byte 或 RawMessage，直接反序列化
	switch p := payload.(type) {
	case []byte:
		if err := jxtjson.Unmarshal(p, &result); err != nil {
			return result, fmt.Errorf("failed to unmarshal payload from bytes: %w", err)
		}
		return result, nil
	case jxtjson.RawMessage:
		if err := jxtjson.Unmarshal([]byte(p), &result); err != nil {
			return result, fmt.Errorf("failed to unmarshal payload from RawMessage: %w", err)
		}
		return result, nil
	}

	// 否则，先序列化再反序列化（处理 map[string]interface{} 的情况）
	payloadBytes, err := jxtjson.Marshal(payload)
	if err != nil {
		return result, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 反序列化为目标类型
	if err := jxtjson.Unmarshal(payloadBytes, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal payload to %T: %w", result, err)
	}

	return result, nil
}

// MarshalPayload 标准化的Payload序列化助手（用于特殊场景）
// 一般不需要直接使用，UnmarshalPayload 内部会自动处理
//
// 使用示例：
//
//	payloadBytes, err := event.MarshalPayload(payload)
//	if err != nil {
//	    return fmt.Errorf("failed to marshal payload: %w", err)
//	}
//
// 优势：
// 1. 统一使用 jxt-core/sdk/pkg/json 的 jsoniter 配置
// 2. 统一的错误处理
func MarshalPayload(payload interface{}) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}

	payloadBytes, err := jxtjson.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return payloadBytes, nil
}
