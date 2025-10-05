package eventbus

import (
	"encoding/json"

	jsoniter "github.com/json-iterator/go"
)

// JSON 统一的 jsoniter 配置实例
// 使用 ConfigCompatibleWithStandardLibrary 确保与标准库完全兼容
// 同时获得更高的性能
var JSON = jsoniter.ConfigCompatibleWithStandardLibrary

// JSONFast 高性能 jsoniter 配置实例
// 使用 ConfigFastest 获得最高性能，但可能在某些边缘情况下与标准库不完全兼容
// 适用于性能敏感的场景
var JSONFast = jsoniter.ConfigFastest

// JSONDefault 默认 jsoniter 配置实例
// 使用 ConfigDefault 平衡性能和兼容性
var JSONDefault = jsoniter.ConfigDefault

// 性能优化说明：
//
// 1. ConfigCompatibleWithStandardLibrary:
//    - 与标准库 encoding/json 100% 兼容
//    - 性能提升约 2-3 倍
//    - 推荐用于需要完全兼容性的场景
//
// 2. ConfigFastest:
//    - 最高性能，提升约 5-10 倍
//    - 可能在某些边缘情况下行为略有不同
//    - 推荐用于性能敏感且数据格式可控的场景
//
// 3. ConfigDefault:
//    - 平衡性能和兼容性
//    - 性能提升约 3-5 倍
//    - 推荐用于大多数场景

// MarshalToString 将对象序列化为 JSON 字符串
// 使用 jsoniter 的高性能 API
func MarshalToString(v interface{}) (string, error) {
	return JSON.MarshalToString(v)
}

// UnmarshalFromString 从 JSON 字符串反序列化对象
// 使用 jsoniter 的高性能 API
func UnmarshalFromString(str string, v interface{}) error {
	return JSON.UnmarshalFromString(str, v)
}

// Marshal 序列化对象为 JSON 字节数组
// 兼容标准库 json.Marshal 接口
// 暂时使用标准库避免jsoniter的map序列化问题
func Marshal(v interface{}) ([]byte, error) {
	// 使用标准库JSON避免jsoniter在某些Go版本中的map序列化问题
	return json.Marshal(v)
}

// Unmarshal 从 JSON 字节数组反序列化对象
// 兼容标准库 json.Unmarshal 接口
func Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// MarshalFast 使用最高性能配置序列化对象
// 适用于性能敏感的场景
func MarshalFast(v interface{}) ([]byte, error) {
	return JSONFast.Marshal(v)
}

// UnmarshalFast 使用最高性能配置反序列化对象
// 适用于性能敏感的场景
func UnmarshalFast(data []byte, v interface{}) error {
	return JSONFast.Unmarshal(data, v)
}

// RawMessage jsoniter 兼容的 RawMessage 类型
// 与标准库 json.RawMessage 完全兼容
type RawMessage = jsoniter.RawMessage

// 使用示例：
//
// 1. 基本使用（兼容标准库）：
//    data, err := eventbus.Marshal(obj)
//    err = eventbus.Unmarshal(data, &obj)
//
// 2. 高性能使用：
//    data, err := eventbus.MarshalFast(obj)
//    err = eventbus.UnmarshalFast(data, &obj)
//
// 3. 字符串操作（避免字节数组转换）：
//    str, err := eventbus.MarshalToString(obj)
//    err = eventbus.UnmarshalFromString(str, &obj)
//
// 4. RawMessage 使用：
//    type Message struct {
//        Data eventbus.RawMessage `json:"data"`
//    }
