package logger

import (
	"sync/atomic"

	"go.uber.org/zap/zapcore"
)

// Masker 对日志 Entry.Message 与 string 类型 Field 做脱敏。
// 业务侧通过 SetMasker 注入（如 pkg/pii.Mask）。返回空字符串等价不脱敏。
type Masker func(string) string

// masker 是进程级单例，atomic.Value 避免读写竞争。
// 0 开销：未 SetMasker 时 Load() == nil，maskingCore 直接委托内部 core。
var maskerRef atomic.Value // Masker

// SetMasker 注册全局 Masker，传 nil 取消。线程安全，可运行时热切换。
// 约束：必须在 logger 初始化前或启动阶段调用，后续调用会立即对所有 logger 生效。
func SetMasker(fn Masker) {
	if fn == nil {
		// atomic.Value 不支持存 nil interface；存一个 explicit zero-value 表示禁用
		maskerRef.Store(Masker(nil))
		return
	}
	maskerRef.Store(fn)
}

// GetMasker 返回当前注册的 Masker，未设置返回 nil。
func GetMasker() Masker {
	v := maskerRef.Load()
	if v == nil {
		return nil
	}
	m, _ := v.(Masker)
	return m
}

// maskingCore 包裹 zapcore.Core，在 Write 前对 Entry.Message 和 string 类型 Field 脱敏。
// 未注册 Masker 时 0 开销（直接委托）。
type maskingCore struct {
	zapcore.Core
}

// newMaskingCore 包一层 mask 装饰器。
func newMaskingCore(inner zapcore.Core) zapcore.Core {
	return &maskingCore{Core: inner}
}

func (c *maskingCore) With(fields []zapcore.Field) zapcore.Core {
	return &maskingCore{Core: c.Core.With(maskFields(fields))}
}

func (c *maskingCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *maskingCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	m := GetMasker()
	if m != nil {
		if ent.Message != "" {
			ent.Message = m(ent.Message)
		}
		fields = maskFields(fields)
	}
	return c.Core.Write(ent, fields)
}

func (c *maskingCore) Sync() error {
	return c.Core.Sync()
}

// maskFields 对 string 类型 Field 逐一脱敏（不触碰数字/对象/错误等非文本字段，保证性能）。
func maskFields(fields []zapcore.Field) []zapcore.Field {
	m := GetMasker()
	if m == nil || len(fields) == 0 {
		return fields
	}
	out := make([]zapcore.Field, len(fields))
	for i, f := range fields {
		if f.Type == zapcore.StringType && f.String != "" {
			f.String = m(f.String)
		}
		out[i] = f
	}
	return out
}
