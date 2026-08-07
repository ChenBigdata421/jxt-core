package reliable

import (
	"regexp"
	"unicode/utf8"
)

// maxErrorMessageBytes is the spec §10 2KB ceiling on stored error_message.
// The reliable root is the single canonical home for this constant; the
// gormshared kernel copy (store/gormshared/fingerprint.go) keeps its own for
// the fingerprint path. The two MUST stay numerically equal.
const maxErrorMessageBytes = 2048

// redactorPatterns is the spec §10 day-1 redaction floor (D11): DSN /
// credentials / token / staff PII / stack paths. Over-redact is the safe
// failure mode — a hit is replaced with [REDACTED]; services may extend with
// org-specific PII via a decorating wrapper.
//
// IMPORTANT — linear vs nested quantifier. The Unix path pattern below is the
// LINEAR single-layer form `/[A-Za-z0-9_\-./]*\.ext`. The older kernel copy in
// store/gormshared/fingerprint.go uses the nested-quantifier shape
// `(/[A-Za-z0-9_\-./]+)+\.ext`, which is `(a+)+` over a class that contains
// both `/` and `.`: under a backtracking engine it is exponential on a
// `/a/a/a/…` input (no .ext terminator) on the DLQ write path. Go's regexp is
// RE2 (linear by construction), so the linear form is defense-in-depth for the
// day someone routes this scrubber through a backtracking engine — and it
// matches the same path set as the nested form. Do NOT regress this pattern to
// the nested shape, and do NOT copy the kernel pattern back over it. The set
// MUST include the Windows-backslash (R2), Windows-forward-slash (R3), and
// URL-encoded %40 (R2) DSN/path variants — an implementer on one OS easily
// misses the others.
//
// S2: the patterns are username-agnostic and anchor on the @tcp( / ://...@
// structures, so tenant-named DSNs (e.g. tenant_42:pwd@tcp) are also redacted.
// A bare "name" JSON key is intentionally excluded (too broad — would swallow
// column names); human names go through full_name/fullname.
var redactorPatterns = []*regexp.Regexp{
	// DSN：Go MySQL user:pass@tcp(host) —— 用户名无关（S2）
	regexp.MustCompile(`(?i)[A-Za-z0-9_.$]+:[^@\s]+@tcp\(`),
	// DSN：URI 形 postgres://user:pass@ / mysql:// / mongodb:// / redis:// / amqp://
	regexp.MustCompile(`(?i)\b(postgres(?:ql)?|mysql|mongodb|redis|amqp)://[^@\s]+:[^@\s]+@`),
	// R2：URL 编码 DSN——@ 被 %40 编码（跨服务 HTTP 回显常见），锚定字面 @ 的规则漏掉
	regexp.MustCompile(`(?i)[A-Za-z0-9_.$]+:[A-Za-z0-9._\-+%]+%40`),
	// DSN / 配置 key=value：password= / passwd= / pwd= / secret= / _auth=
	regexp.MustCompile(`(?i)(password|passwd|pwd|secret|_auth)=[^;&\s"']+`),

	// 凭证头 / token（含空格分隔的 `Bearer <jwt>`）
	regexp.MustCompile(`(?i)(authorization|bearer|token)[\s:=]+[A-Za-z0-9._+/=-]+(\s+[A-Za-z0-9._+/=-]+)?`),
	regexp.MustCompile(`(?i)(x-api-key|x-auth-token|cookie|set-cookie)[:=]\s?[A-Za-z0-9._+/=-]+`),

	// JSON 字段：凭证（S1 补 PII）
	regexp.MustCompile(`(?i)"(password|secret|token|api[_-]?key)"\s*:\s*"[^"]*"`),
	// 人员 PII（spec §10）：user/phone/id_card/email/full_name/case_id/evidence_id 等
	regexp.MustCompile(`(?i)"(user|username|phone|mobile|id_?card|citizen_id|email|full_?name|case_id|evidence_id)"\s*:\s*"[^"]*"`),
	// R2：数组/集合形态 PII（"phones":[...]、"id_cards":[...]），批量 handler（M11）错误聚合热点
	regexp.MustCompile(`(?i)"(phones?|mobiles?|id_?cards?|citizen_ids?|emails?|user_?ids?)"\s*:\s*\[[^\]]*\]`),

	// 栈路径 / 配置文件泄漏：/xx/yy.yml|.env|.conf|.key|.pem（Unix）。
	// 线性形式（单层 *[A-Za-z0-9_\-./]*\.）：与内核 nested-quantifier 副本
	// (store/gormshared/fingerprint.go) 匹配同样的路径集，但在线性时间完成。
	// 内核副本尚未同步此修复——这是根包的规范化版本，供 DLQ adapter 与服务共享。
	regexp.MustCompile(`(?i)/[A-Za-z0-9_\-./]*\.(ya?ml|env|conf|key|pem)\b`),
	// R2：Windows 路径——本项目跑在 D:\JXT\...，round-1 的 Unix-only 规则漏掉
	regexp.MustCompile(`(?i)[A-Za-z]:\\[^\s"<>|]+\.(ya?ml|env|conf|key|pem)\b`),
	// R3：Windows 正斜杠形式（Go 在 Windows 常以 D:/JXT/... 输出路径），backslash 规则漏掉
	regexp.MustCompile(`(?i)[A-Za-z]:/[^\s"<>|]+\.(ya?ml|env|conf|key|pem)\b`),
}

// SanitizeForStorage scrubs secrets and truncates to 2KB on a rune boundary
// (spec §10). The ordering is truncate → regex → truncate:
//   - The PRE-truncate bounds the regex input. The DLQ cause is untrusted error
//     text (a GORM/driver message can itself echo attacker bytes); capping it
//     first keeps the regex engine linear even if a future pattern reintroduces
//     backtracking.
//   - The POST-truncate re-enforces the 2KB storage ceiling because redaction
//     can GROW the string ([REDACTED] is longer than a short match).
//
// The rune-boundary backoff (C5) is preserved at both truncation sites: a naive
// s[:2048] splits a multi-byte character in half and produces invalid UTF-8,
// which the utf8mb4 column then rejects (MySQL error 1366) or silently replaces
// with U+FFFD. CJK error messages routinely hit this.
//
// This is the canonical root scrubber. The DLQ adapter (adapters/eventbus) and
// the service-side EventBusDLQAdapter both route quarantine error text through
// it; the kernel's own MarkFailed/RecordTerminal paths still call the internal
// store/gormshared.sanitizeMsg (unchanged — additive PR, no delegation wired).
func SanitizeForStorage(s string) string {
	s = truncateUTF8(s) // ReDoS bound: cap input before the regex pass
	for _, p := range redactorPatterns {
		s = p.ReplaceAllString(s, "[REDACTED]")
	}
	return truncateUTF8(s) // honor §10 2KB storage ceiling (redaction can grow)
}

// truncateUTF8 caps s to maxErrorMessageBytes on a UTF-8 rune boundary (C5).
// Exported only to the package: callers reach the ceiling via SanitizeForStorage
// / SanitizeForLog; nothing outside the package needs the raw truncator.
func truncateUTF8(s string) string {
	if len(s) <= maxErrorMessageBytes {
		return s
	}
	s = s[:maxErrorMessageBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// SanitizeForLog is the log-line redaction entry point (F4-extend). The DLQ
// cause is scrubbed before it lands in a DB row, but the SAME cause — which can
// echo the tenant DSN — was otherwise rendered to logs verbatim (the sdk logger
// has no built-in redaction). Route every cause/err rendered into a reliable-
// path log line through this. It shares the storage scrubber so the log and
// storage paths can never diverge below the D11 floor; the two entry points are
// distinct only so callers express intent and so a future lighter-touch log
// form can be introduced without touching storage callers.
func SanitizeForLog(s string) string { return SanitizeForStorage(s) }
