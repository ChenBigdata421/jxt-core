package logger

import (
	"time"
)

// FieldType 字段类型（零分配设计）
type FieldType uint8

const (
	FieldTypeAny FieldType = iota
	FieldTypeString
	FieldTypeInt
	FieldTypeBool
	FieldTypeDuration
	FieldTypeTime
	FieldTypeError
)

// Field 结构化日志字段（零分配）
type Field struct {
	Key    string
	Type   FieldType
	Int64  int64       // 存储 int、bool、duration、time
	String string      // 存储 string
	Any    interface{} // 存储 error、复杂类型
}

// 字段构造函数（模仿 zap.Field）

// FieldString 字符串字段（避免与 logger.String() 冲突）
func FieldString(key string, val string) Field {
	return Field{Key: key, Type: FieldTypeString, String: val}
}

// Int 整数字段
func Int(key string, val int) Field {
	return Field{Key: key, Type: FieldTypeInt, Int64: int64(val)}
}

// Int64 64位整数字段
func Int64(key string, val int64) Field {
	return Field{Key: key, Type: FieldTypeInt, Int64: val}
}

// Bool 布尔字段
func Bool(key string, val bool) Field {
	if val {
		return Field{Key: key, Type: FieldTypeBool, Int64: 1}
	}
	return Field{Key: key, Type: FieldTypeBool, Int64: 0}
}

// Duration 时长字段
func Duration(key string, val time.Duration) Field {
	return Field{Key: key, Type: FieldTypeDuration, Int64: int64(val)}
}

// Time 时间字段
func Time(key string, val time.Time) Field {
	return Field{Key: key, Type: FieldTypeTime, Int64: val.UnixNano()}
}

// FieldError 错误字段（避免与 level.Error() 冲突）
func FieldError(err error) Field {
	return Field{Key: "error", Type: FieldTypeError, Any: err}
}

// Any 任意类型字段（性能较低，尽量少用）
func Any(key string, val interface{}) Field {
	return Field{Key: key, Type: FieldTypeAny, Any: val}
}

// 常用字段快捷方式

// RequestID 请求 ID 字段
func RequestID(val string) Field {
	return FieldString("request_id", val)
}

// TraceID 链路追踪 ID 字段
func TraceID(val string) Field {
	return FieldString("trace_id", val)
}

// UserID 用户 ID 字段
func UserID(val int64) Field {
	return Int64("user_id", val)
}

// Method HTTP 方法字段
func Method(val string) Field {
	return FieldString("method", val)
}

// URI 请求路径字段
func URI(val string) Field {
	return FieldString("uri", val)
}

// StatusCode HTTP 状态码字段
func StatusCode(val int) Field {
	return Int("status_code", val)
}

// Latency 请求延迟字段
func Latency(val time.Duration) Field {
	return Duration("latency", val)
}

// ClientIP 客户端 IP 字段
func ClientIP(val string) Field {
	return FieldString("client_ip", val)
}
