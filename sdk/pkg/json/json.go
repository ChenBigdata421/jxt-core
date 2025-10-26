package json

import (
	jsoniter "github.com/json-iterator/go"
)

// JSON 统一的 jsoniter 配置实例
// 使用 ConfigCompatibleWithStandardLibrary 确保与标准库完全兼容
// 同时获得更高的性能（比标准库快 2-3 倍）
//
// 所有 jxt-core 组件都应该使用这个统一的配置，包括：
// - domain/event: DomainEvent 序列化
// - eventbus: Envelope 序列化
// - outbox: OutboxEvent 序列化
// - 其他所有需要 JSON 序列化的组件
var JSON = jsoniter.ConfigCompatibleWithStandardLibrary

// JSONFast 高性能 jsoniter 配置实例
// 使用 ConfigFastest 获得最高性能，但可能在某些边缘情况下与标准库不完全兼容
// 适用于性能敏感的场景
var JSONFast = jsoniter.ConfigFastest

// JSONDefault 默认 jsoniter 配置实例
// 使用 ConfigDefault 平衡性能和兼容性
var JSONDefault = jsoniter.ConfigDefault

// Marshal 序列化对象为 JSON 字节数组
// 兼容标准库 json.Marshal 接口
//
// 使用示例：
//
//	data, err := jxtjson.Marshal(obj)
//	if err != nil {
//	    return err
//	}
//
// 性能：
//   - 比标准库 encoding/json 快约 2-3 倍
//   - 完全兼容标准库 API
func Marshal(v interface{}) ([]byte, error) {
	return JSON.Marshal(v)
}

// Unmarshal 从 JSON 字节数组反序列化对象
// 兼容标准库 json.Unmarshal 接口
//
// 使用示例：
//
//	var obj MyStruct
//	err := jxtjson.Unmarshal(data, &obj)
//	if err != nil {
//	    return err
//	}
//
// 性能：
//   - 比标准库 encoding/json 快约 2-3 倍
//   - 完全兼容标准库 API
func Unmarshal(data []byte, v interface{}) error {
	return JSON.Unmarshal(data, v)
}

// MarshalToString 将对象序列化为 JSON 字符串
// 使用 jsoniter 的高性能 API，避免字节数组到字符串的转换
//
// 使用示例：
//
//	str, err := jxtjson.MarshalToString(obj)
//	if err != nil {
//	    return err
//	}
//
// 性能：
//   - 比 string(json.Marshal(obj)) 更快
//   - 避免了字节数组到字符串的内存拷贝
func MarshalToString(v interface{}) (string, error) {
	return JSON.MarshalToString(v)
}

// UnmarshalFromString 从 JSON 字符串反序列化对象
// 使用 jsoniter 的高性能 API，避免字符串到字节数组的转换
//
// 使用示例：
//
//	var obj MyStruct
//	err := jxtjson.UnmarshalFromString(str, &obj)
//	if err != nil {
//	    return err
//	}
//
// 性能：
//   - 比 json.Unmarshal([]byte(str), &obj) 更快
//   - 避免了字符串到字节数组的内存拷贝
func UnmarshalFromString(str string, v interface{}) error {
	return JSON.UnmarshalFromString(str, v)
}

// MarshalFast 使用最高性能配置序列化对象
// 适用于性能敏感的场景
//
// 使用示例：
//
//	data, err := jxtjson.MarshalFast(obj)
//	if err != nil {
//	    return err
//	}
//
// 性能：
//   - 比标准库 encoding/json 快约 5-10 倍
//   - 可能在某些边缘情况下与标准库行为略有不同
//   - 推荐用于性能敏感且数据格式可控的场景
func MarshalFast(v interface{}) ([]byte, error) {
	return JSONFast.Marshal(v)
}

// UnmarshalFast 使用最高性能配置反序列化对象
// 适用于性能敏感的场景
//
// 使用示例：
//
//	var obj MyStruct
//	err := jxtjson.UnmarshalFast(data, &obj)
//	if err != nil {
//	    return err
//	}
//
// 性能：
//   - 比标准库 encoding/json 快约 5-10 倍
//   - 可能在某些边缘情况下与标准库行为略有不同
//   - 推荐用于性能敏感且数据格式可控的场景
func UnmarshalFast(data []byte, v interface{}) error {
	return JSONFast.Unmarshal(data, v)
}

// RawMessage jsoniter 兼容的 RawMessage 类型
// 与标准库 json.RawMessage 完全兼容
//
// 使用示例：
//
//	type Message struct {
//	    Data jxtjson.RawMessage `json:"data"`
//	}
//
// 说明：
//   - RawMessage 是 []byte 的别名
//   - 在 JSON 序列化/反序列化时，RawMessage 会被保留为原始 JSON
//   - 适用于延迟解析或透传 JSON 数据的场景
type RawMessage = jsoniter.RawMessage

// 使用示例：
//
// 1. 基本使用（兼容标准库）：
//
//	data, err := jxtjson.Marshal(obj)
//	err = jxtjson.Unmarshal(data, &obj)
//
// 2. 字符串操作（避免字节数组转换）：
//
//	str, err := jxtjson.MarshalToString(obj)
//	err = jxtjson.UnmarshalFromString(str, &obj)
//
// 3. RawMessage 使用：
//
//	type Message struct {
//	    Data jxtjson.RawMessage `json:"data"`
//	}
//
// 4. 在 domain/event 中使用：
//
//	import jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
//
//	func MarshalDomainEvent(event BaseEvent) ([]byte, error) {
//	    return jxtjson.Marshal(event)
//	}
//
// 5. 在 eventbus 中使用：
//
//	import jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
//
//	func (e *Envelope) ToBytes() ([]byte, error) {
//	    return jxtjson.Marshal(e)
//	}
//
// 6. 在 outbox 中使用：
//
//	import jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
//
//	func NewOutboxEvent(..., payload jxtevent.BaseEvent) (*OutboxEvent, error) {
//	    payloadBytes, err := jxtevent.MarshalDomainEvent(payload)
//	    ...
//	}
//
// 性能优势：
//
// 1. ConfigCompatibleWithStandardLibrary:
//    - 与标准库 encoding/json 100% 兼容
//    - 性能提升约 2-3 倍
//    - 推荐用于需要完全兼容性的场景
//
// 2. 内存优化:
//    - MarshalToString/UnmarshalFromString 避免字节数组转换
//    - 减少内存分配和拷贝
//
// 3. Go 1.24 兼容性:
//    - jsoniter v1.1.12 完全支持 Go 1.24 的 Swiss Tables map 实现
//    - 无任何兼容性问题
//
// 注意事项：
//
// 1. 统一配置:
//    - 所有 jxt-core 组件都应该使用这个统一的配置
//    - 不要在各个组件中重复定义 jsoniter 配置
//
// 2. 升级维护:
//    - 升级 jsoniter 只需在这个包中修改
//    - 所有组件自动获得升级
//
// 3. 性能测试:
//    - 定期运行性能测试，确保性能符合预期
//    - 关注 jsoniter 的版本更新
