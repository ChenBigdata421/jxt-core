package reliable

import (
	"errors"
	"time"
)

// Status 是 event_consumption.status 的五态枚举（§2.1/§3）。
type Status string

const (
	StatusProcessing     Status = "PROCESSING"
	StatusSucceeded      Status = "SUCCEEDED"
	StatusRetryScheduled Status = "RETRY_SCHEDULED"
	StatusDeadLetter     Status = "DEAD_LETTER"
	StatusDiscarded      Status = "DISCARDED"
)

// IsTerminal 报告该状态是否已终结。
func IsTerminal(s Status) bool {
	switch s {
	case StatusSucceeded, StatusDeadLetter, StatusDiscarded:
		return true
	}
	return false
}

// legalTransitions 是 §3 状态图的合法转移表（唯一真相源）。
var legalTransitions = map[Status]map[Status]bool{
	StatusProcessing: {
		StatusSucceeded:      true,
		StatusRetryScheduled: true,
		StatusDeadLetter:     true,
	},
	StatusRetryScheduled: {
		StatusProcessing: true,
		StatusDeadLetter: true,
	},
	StatusDeadLetter: {
		StatusRetryScheduled: true,
		StatusDiscarded:      true,
		StatusSucceeded:      true,
	},
	StatusSucceeded: {},
	StatusDiscarded: {},
}

// CanTransition 报告 from→to 是否合法。
func CanTransition(from, to Status) bool {
	if dest, ok := legalTransitions[from]; ok {
		return dest[to]
	}
	return false
}

// ErrIllegalTransition 由 store 在检测到非法转移时返回（纵深防御）。
var ErrIllegalTransition = errors.New("reliable: illegal status transition")

// —— attempt / backoff oracle（纯函数，§6.2）——

// AdvanceAttempt 把 attempt 推进到下一次业务执行（RETRY_SCHEDULED→PROCESSING 时调用）。
func AdvanceAttempt(current int) int {
	if current < 1 {
		return 1
	}
	return current + 1
}

// ShouldDeadLetter 报告「已开始的业务执行次数」是否已达上限。
func ShouldDeadLetter(attempt, maxAttempts int) bool {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return attempt >= maxAttempts
}

// Backoff 计算下次重试的退避时长（§6.2）。纯函数：jitterFraction ∈ [0,1) 由调用方提供。
// 公式：base × 2^(attempt-1)，封顶 cap；jitter ±20%。
func Backoff(attempt int, base, cap time.Duration, jitterFraction float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if base <= 0 {
		base = time.Second
	}
	if cap <= 0 {
		cap = time.Hour
	}
	d := base
	for i := 1; i < attempt; i++ {
		if d >= cap {
			d = cap
			break
		}
		d *= 2
		if d > cap {
			d = cap
			break
		}
	}
	scale := 0.8 + 0.4*jitterFraction
	if scale < 0.8 {
		scale = 0.8
	} else if scale > 1.2 {
		scale = 1.2
	}
	r := time.Duration(float64(d) * scale)
	// 硬封顶：jitter 不得让结果超过 cap（§6.2「封顶 cap」+ property 不变量）。
	if r > cap {
		r = cap
	}
	return r
}

// DefaultBackoffBase / DefaultBackoffCap（§6.2 起点 1s / 上限 1h）。
const (
	DefaultBackoffBase = time.Second
	DefaultBackoffCap  = time.Hour
)
