package replay

import (
	"errors"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
)

// HandlerInfo 是注册表条目。
type HandlerInfo struct {
	HandlerID             reliable.HandlerID
	ReplaySafety          reliable.ReplaySafety
	RequiresAggregateGate bool
	Handler               reliable.ReplayableHandler
}

// HandlerRegistry 是 scheduler 对 handler 注册表的要求。
type HandlerRegistry interface {
	Lookup(id reliable.HandlerID) (HandlerInfo, bool)
	All() []HandlerInfo // 启动时打印安全类别清单（§6.1）
}

type InvokeResult int

const (
	InvokeOK InvokeResult = iota
	InvokeRetryLater
	InvokeNotPermitted
	InvokeNotSelfReplayable
	InvokeFailed
)

func invokeResult(err error) InvokeResult {
	switch {
	case err == nil:
		return InvokeOK
	case isReliable(err, reliable.ErrRetryLater):
		return InvokeRetryLater
	case isReliable(err, reliable.ErrNotPermitted):
		return InvokeNotPermitted
	case isReliable(err, reliable.ErrNotSelfReplayable):
		return InvokeNotSelfReplayable
	default:
		return InvokeFailed
	}
}

// B6（本轮评审）：原稿在这里手写了一个 unwrap 循环。它对 errors.Join（多分支
// `Unwrap() []error`）失效，对实现了 `Is(error) bool` 的自定义错误也失效；而同包内的
// handleNonExecution 用的就是 errors.Is——同一个包里两套判等逻辑。统一用标准库。
func isReliable(err, target error) bool { return errors.Is(err, target) }
