package logger

import "context"

type loggerKey struct{}

// FromContext 从 context 中获取 logger
// 如果不存在，返回默认 logger（保证永远不返回 nil）
func FromContext(ctx context.Context) *Helper {
	if ctx == nil {
		return NewHelper(DefaultLogger)
	}

	if l, ok := ctx.Value(&loggerKey{}).(*Helper); ok {
		return l
	}

	return NewHelper(DefaultLogger)
}

// NewContext 将 logger 放入 context（中间件使用）
func NewContext(ctx context.Context, l *Helper) context.Context {
	return context.WithValue(ctx, &loggerKey{}, l)
}
