package logger

import (
	"os"
)

// Helper 日志辅助器（API 层、Service 层、Job 层通用）
// 提供简洁的日志 API，支持结构化日志和链式调用
//
// 使用示例：
//   e.Log.Infow("用户登录成功",
//       "user_id", 123,
//       "ip", "192.168.1.1",
//   )
//
//   e.Log.WithError(err).Error("数据库查询失败")
//
// 性能优化：
//   - 每个方法都先检查日志级别，避免无效调用
//   - fields 采用 map 存储，支持高效合并
type Helper struct {
	Logger
	fields map[string]interface{} // 预设字段（链式调用时累加）
}

func NewHelper(log Logger) *Helper {
	return &Helper{Logger: log}
}

func (h *Helper) Info(args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(InfoLevel) {
		return
	}
	h.Logger.Fields(h.fields).Log(InfoLevel, args...)
}

func (h *Helper) Infof(template string, args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(InfoLevel) {
		return
	}
	h.Logger.Fields(h.fields).Logf(InfoLevel, template, args...)
}

// Infow 使用结构化字段记录 INFO 日志
// 性能优化：先检查级别，再构建字段 map
// 使用示例：e.Log.Infow("用户登录", "user_id", 123, "ip", "192.168.1.1")
func (h *Helper) Infow(msg string, keysAndValues ...interface{}) {
	if !h.Logger.Options().Level.Enabled(InfoLevel) {
		return
	}
	fields := h.kvToMap(keysAndValues...)
	h.Logger.Fields(fields).Log(InfoLevel, msg)
}

func (h *Helper) Trace(args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(TraceLevel) {
		return
	}
	h.Logger.Fields(h.fields).Log(TraceLevel, args...)
}

func (h *Helper) Tracef(template string, args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(TraceLevel) {
		return
	}
	h.Logger.Fields(h.fields).Logf(TraceLevel, template, args...)
}

func (h *Helper) Debug(args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(DebugLevel) {
		return
	}
	h.Logger.Fields(h.fields).Log(DebugLevel, args...)
}

func (h *Helper) Debugf(template string, args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(DebugLevel) {
		return
	}
	h.Logger.Fields(h.fields).Logf(DebugLevel, template, args...)
}

func (h *Helper) Warn(args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(WarnLevel) {
		return
	}
	h.Logger.Fields(h.fields).Log(WarnLevel, args...)
}

func (h *Helper) Warnf(template string, args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(WarnLevel) {
		return
	}
	h.Logger.Fields(h.fields).Logf(WarnLevel, template, args...)
}

// Warnw 使用结构化字段记录 WARN 日志
func (h *Helper) Warnw(msg string, keysAndValues ...interface{}) {
	if !h.Logger.Options().Level.Enabled(WarnLevel) {
		return
	}
	fields := h.kvToMap(keysAndValues...)
	h.Logger.Fields(fields).Log(WarnLevel, msg)
}

func (h *Helper) Error(args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(ErrorLevel) {
		return
	}
	h.Logger.Fields(h.fields).Log(ErrorLevel, args...)
}

func (h *Helper) Errorf(template string, args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(ErrorLevel) {
		return
	}
	h.Logger.Fields(h.fields).Logf(ErrorLevel, template, args...)
}

// Errorw 使用结构化字段记录 ERROR 日志
func (h *Helper) Errorw(msg string, keysAndValues ...interface{}) {
	if !h.Logger.Options().Level.Enabled(ErrorLevel) {
		return
	}
	fields := h.kvToMap(keysAndValues...)
	h.Logger.Fields(fields).Log(ErrorLevel, msg)
}

// Debugw 使用结构化字段记录 DEBUG 日志
func (h *Helper) Debugw(msg string, keysAndValues ...interface{}) {
	if !h.Logger.Options().Level.Enabled(DebugLevel) {
		return
	}
	fields := h.kvToMap(keysAndValues...)
	h.Logger.Fields(fields).Log(DebugLevel, msg)
}

// kvToMap 将 key-value 数组转换为 map
// 性能优化：复用现有 fields，避免重复创建
// 输入示例："user_id", 123, "ip", "192.168.1.1"
func (h *Helper) kvToMap(keysAndValues ...interface{}) map[string]interface{} {
	fields := copyFields(h.fields) // 复用现有字段
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := keysAndValues[i]
			val := keysAndValues[i+1]
			if keyStr, ok := key.(string); ok {
				fields[keyStr] = val
			}
		}
	}
	return fields
}

func (h *Helper) Fatal(args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(FatalLevel) {
		return
	}
	h.Logger.Fields(h.fields).Log(FatalLevel, args...)
	os.Exit(1)
}

func (h *Helper) Fatalf(template string, args ...interface{}) {
	if !h.Logger.Options().Level.Enabled(FatalLevel) {
		return
	}
	h.Logger.Fields(h.fields).Logf(FatalLevel, template, args...)
	os.Exit(1)
}

func (h *Helper) WithError(err error) *Helper {
	fields := copyFields(h.fields)
	fields["error"] = err
	return &Helper{Logger: h.Logger, fields: fields}
}

func (h *Helper) WithFields(fields map[string]interface{}) *Helper {
	nfields := copyFields(fields)
	for k, v := range h.fields {
		nfields[k] = v
	}
	return &Helper{Logger: h.Logger, fields: nfields}
}
