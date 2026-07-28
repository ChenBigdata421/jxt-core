package reliable

import (
	"errors"
	"fmt"
)

// —— sentinels（C5：全仓单一定义，禁止 persistence/服务层另立）——

// ErrDuplicateKey 表示某次写入撞了唯一索引。它只声明事实，不声明该冲突是否等于幂等命中
// —— handler 必须自行核实约束名属本次事件的幂等键、读回已有行、比对内容一致后，才能包成 ErrSkip（§5）。
// 取代 evidence-management 与 process-management 各自的 service.ErrDuplicateKey 分裂副本（PR1_SCOPE C5）。
var ErrDuplicateKey = errors.New("reliable: duplicate key violation")

// ErrSkip 由 handler 在第 1 级显式声明：本次事件已被认定为幂等命中（如核实后的唯一冲突），
// 装饰器按成功终结（§4 Phase B 的 MarkSucceeded 分支）。第 2 级 driver classifier 不再兜底判 Skip。
var ErrSkip = errors.New("reliable: idempotent skip")

// ErrRetryLater 由 TryClaim 在 AlreadyProcessing 分支返回：他人持有有效租约，本次不落库、不 ACK，
// 交还 broker 稍后重投（§3.1）。
var ErrRetryLater = errors.New("reliable: retry later")

// ErrNotPermitted 重放时 CanAutoReplay=false 命中（纵深防御，正常不应触发；触发即 §6.1 矩阵有漏洞）。
var ErrNotPermitted = errors.New("reliable: replay not permitted")

// ErrNotSelfReplayable 行的 payload IS NULL，靠 broker 重投；若该行已 ACK 则 broker 不再投，需人工介入。
var ErrNotSelfReplayable = errors.New("reliable: row not self-replayable")

// ErrConflict 终态 CAS 版本不符（运维 API expected_row_version 不匹配）或 RecordTerminal 遇现存 PROCESSING，
// 或 ScheduleReplay 的 requester==approver（双人确认违规）。
var ErrConflict = errors.New("reliable: conflicting state")

// —— typed wrappers（第 1 级领域显式声明，权威）——

// PermanentError 包装一个「永不可恢复」的根因：消息结构损坏或触发了永不会变的业务规则。
// 两者终点相同（DEAD_LETTER），排查思路不同——以 error_message 区分（§5 v2.6）。
type PermanentError struct{ Cause error }

func (e *PermanentError) Error() string { return fmt.Sprintf("reliable: permanent: %v", e.Cause) }
func (e *PermanentError) Unwrap() error { return e.Cause }

// Permanent 把 err 标记为 POISON（终态 DEAD_LETTER）。handler 对「外键关系不会最终一致」等
// 明确判断也可用它覆盖第 2 级 1452→Retryable 的默认（§5）。
func Permanent(err error) error { return &PermanentError{Cause: err} }

// IsPermanent 报告 err 链上是否含 PermanentError。
func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

// RetryableError 包装一个「可重试」的根因（瞬态 DB 超时/死锁/锁等待等）。
type RetryableError struct{ Cause error }

func (e *RetryableError) Error() string { return fmt.Sprintf("reliable: retryable: %v", e.Cause) }
func (e *RetryableError) Unwrap() error { return e.Cause }

// Retryable 把 err 标记为 RETRYABLE（终点由 §6.1 矩阵按 ReplaySafety 决定）。
func Retryable(err error) error { return &RetryableError{Cause: err} }

// IsRetryable 报告 err 链上是否含 RetryableError。
func IsRetryable(err error) bool {
	var r *RetryableError
	return errors.As(err, &r)
}
