package reliable

import (
	"context"
	"errors"
	"net"
)

// ErrorClass 是错误分类的结果（§5）。
type ErrorClass string

const (
	// ClassRetryable 瞬态：DB 死锁/锁等待/超时/连接限/关停中/1452 缺父（默认）。
	ClassRetryable ErrorClass = "RETRYABLE"
	// ClassPoison 永不可恢复（消息结构损坏或永不变的业务规则）。终点 DEAD_LETTER（§5 v2.6）。
	ClassPoison ErrorClass = "POISON"
	// ClassUnrecoverable 兜底 + schema 漂移。
	ClassUnrecoverable ErrorClass = "UNRECOVERABLE"
	// ClassConflict 未被第 1 级认领的唯一冲突（1062/23505 未核实为幂等命中）。终点 DEAD_LETTER。
	ClassConflict ErrorClass = "CONFLICT"
	// ClassSkip 仅由 handler 第 1 级显式声明（ErrSkip）；第 2 级不兜底判 Skip。终点按成功终结。
	ClassSkip ErrorClass = "SKIP"
)

// ErrorClassifier 是 driver classifier 的统一接口（§5 第 2 级）。
// 实现放 store/mysql（识别 *mysql.MySQLError.Number）与 store/postgres（识别 *pgconn.PgError.Code）。
// kernel 不 import 任何数据库驱动——这就是为什么 classifier 是注入而非硬编码。
// dup 检测（TryClaim 的 Create 竞态）也复用此接口，避免字符串匹配（D3）。
type ErrorClassifier interface {
	// ClassifyDriver 只做保守默认；返回 (class, true) 表示命中已知驱动错误码，(0, false) 表示不认识。
	ClassifyDriver(err error) (ErrorClass, bool)
	// IsDuplicateKey 报告 err 是否是该驱动的唯一冲突错误（TryClaim dup 检测用，D3）。
	IsDuplicateKey(err error) bool
	// ErrorCode 提取驱动原生错误码字符串（如 MySQL "1213"、PostgreSQL "23505"），
	// 用于填充 error_code 列（有界、可聚合的稳定代码，§5）。返回 (code, true) 表示已提取，
	// ("", false) 表示不认识的错误——调用方回落 Classify() 的 class 名作为 code。
	ErrorCode(err error) (string, bool)
}

// Classify 是两级分类的纯函数（§5）：
//  1. 领域显式声明（权威）：PermanentError→Poison；RetryableError→Retryable；ErrSkip→Skip。
//  2. 基础设施错误码（精确，保守默认）：driver 识别 + context 超时/取消 + net 超时。
//  3. 兜底 Unrecoverable（v2.5：未知不再当 Retryable）。
func Classify(err error, driver ErrorClassifier) ErrorClass {
	if err == nil {
		return ClassSkip
	}
	if IsPermanent(err) {
		return ClassPoison
	}
	if IsRetryable(err) {
		return ClassRetryable
	}
	if errors.Is(err, ErrSkip) {
		return ClassSkip
	}
	if driver != nil {
		if c, ok := driver.ClassifyDriver(err); ok {
			return c
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ClassRetryable
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ClassRetryable
	}
	return ClassUnrecoverable
}

// TerminalOutcome 是 MarkFailed 按 §6.1 矩阵推导的终点。
type TerminalOutcome struct{ DeadLetter bool }

// OutcomeFor 实现 §6.1 的 ErrorClass × ReplaySafety 矩阵（纯函数，双方言 store 共用）。
func OutcomeFor(class ErrorClass, safety ReplaySafety) TerminalOutcome {
	if class == ClassSkip {
		return TerminalOutcome{DeadLetter: false}
	}
	if class == ClassRetryable && safety != ReplayUnsafe {
		return TerminalOutcome{DeadLetter: false} // RETRY_SCHEDULED
	}
	return TerminalOutcome{DeadLetter: true}
}
