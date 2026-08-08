package gormshared

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"unicode/utf8"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
)

const maxErrorMessageBytes = 2048 // §10：2KB 上限

// redactorPatterns 是 spec §10 的 day-1 脱敏地板（D11）：DSN / 凭证 / token / **人员 PII** / 栈路径。
// over-redact 是安全失败模式——命中即替换为 [REDACTED]；服务侧可在 PR-3 装饰器扩展 org-specific PII。
// 本地板必须覆盖 spec §10 line 1038「SQL 参数、DSN、凭证、token 与人员 PII」全部类别（本轮评审 S1/S2）。
//
// S2：原稿硬编码 `(root|test|admin|app)` 用户名，租户命名 DSN（如 tenant_42:pwd@tcp）全部漏脱敏，
// 且 fingerprint 测试只用 root: 输入 → 假绿（与 C5 sentinel 同型）。现改为用户名无关、锚定 @tcp(/://...@ 结构。
// bare "name" JSON key 故意外排（过宽，会误吞列名）；人类姓名走 full_name/fullname，服务侧可按需扩展。
var redactorPatterns = []*regexp.Regexp{
	// DSN：Go MySQL user:pass@tcp(host) —— 用户名无关（S2）
	regexp.MustCompile(`(?i)[A-Za-z0-9_.$]+:[^@\s]+@tcp\(`),
	// DSN：URI 形 postgres://user:pass@ / mysql:// / mongodb:// / redis:// / amqp://
	regexp.MustCompile(`(?i)\b(postgres(?:ql)?|mysql|mongodb|redis|amqp)://[^@\s]+:[^@\s]+@`),
	// R2：URL 编码 DSN——@ 被 %40 编码（跨服务 HTTP 回显常见），锚定字面 @ 的规则漏掉
	regexp.MustCompile(`(?i)[A-Za-z0-9_.$]+:[A-Za-z0-9._\-+%]+%40`),
	// DSN / 配置 key=value：password= / passwd= / pwd= / secret= / _auth=
	// 值可为无引号/双引号/单引号（与根 reliable.SanitizeForStorage 同源——INI/YAML 回显的
	// password="x"/'x' 也要脱敏；原 [^;&\s"']+ 排除引号会漏配。两处 sanitize 必须同步此分支）。
	regexp.MustCompile(`(?i)(password|passwd|pwd|secret|_auth)=([^;&\s"']+|"[^"]*"|'[^']*')`),

	// 凭证头 / token（含空格分隔的 `Bearer <jwt>`）
	regexp.MustCompile(`(?i)(authorization|bearer|token)[\s:=]+[A-Za-z0-9._+/=-]+(\s+[A-Za-z0-9._+/=-]+)?`),
	regexp.MustCompile(`(?i)(x-api-key|x-auth-token|cookie|set-cookie)[:=]\s?[A-Za-z0-9._+/=-]+`),

	// JSON 字段：凭证（S1 补 PII）
	regexp.MustCompile(`(?i)"(password|secret|token|api[_-]?key)"\s*:\s*"[^"]*"`),
	// 人员 PII（spec §10）：user/phone/id_card/email/full_name/case_id/evidence_id 等
	regexp.MustCompile(`(?i)"(user|username|phone|mobile|id_?card|citizen_id|email|full_?name|case_id|evidence_id)"\s*:\s*"[^"]*"`),
	// R2：数组/集合形态 PII（"phones":[...]、"id_cards":[...]），批量 handler（M11）错误聚合热点
	regexp.MustCompile(`(?i)"(phones?|mobiles?|id_?cards?|citizen_ids?|emails?|user_?ids?)"\s*:\s*\[[^\]]*\]`),

	// 栈路径 / 配置文件泄漏：/xx/yy.yml|.env|.conf|.key|.pem（Unix）
	regexp.MustCompile(`(?i)(/[A-Za-z0-9_\-./]+)+\.(ya?ml|env|conf|key|pem)\b`),
	// R2：Windows 路径——本项目跑在 D:\JXT\...，round-1 的 Unix-only 规则漏掉
	regexp.MustCompile(`(?i)[A-Za-z]:\\[^\s"<>|]+\.(ya?ml|env|conf|key|pem)\b`),
	// R3：Windows 正斜杠形式（Go 在 Windows 常以 D:/JXT/... 输出路径），backslash 规则漏掉
	regexp.MustCompile(`(?i)[A-Za-z]:/[^\s"<>|]+\.(ya?ml|env|conf|key|pem)\b`),
}

// sanitizeMsg 先清洗 secret 再截断到 2KB（§10）。
// C5（本轮评审）：截断按 rune 边界回退，避免把多字节字符切半——错误消息含中文时 s[:2048]
// 会产出非法 UTF-8，MySQL utf8mb4 列会报 1366 或静默替换为 U+FFFD。
func sanitizeMsg(s string) string {
	for _, p := range redactorPatterns {
		s = p.ReplaceAllString(s, "[REDACTED]")
	}
	if len(s) > maxErrorMessageBytes {
		s = s[:maxErrorMessageBytes]
		for len(s) > 0 && !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// fingerprint = sha256(class + ":" + sanitized) → 64 hex（D10）。稳定、契合 CHAR(64)、可聚合。
func fingerprint(class reliable.ErrorClass, msg string) string {
	sum := sha256.Sum256([]byte(string(class) + ":" + sanitizeMsg(msg)))
	return hex.EncodeToString(sum[:])
}

// stableErrorCode 提取驱动原生错误码（如 MySQL "1213"、PostgreSQL "23505"），
// 回落 classifier 第 2 级分类名，再回落 "UNKNOWN"。保证 error_code 列始终有值（≤64 chars）。
func stableErrorCode(err error, classifier reliable.ErrorClassifier) string {
	if classifier != nil {
		if code, ok := classifier.ErrorCode(err); ok {
			return code
		}
	}
	if err == nil {
		return "UNKNOWN"
	}
	// 回落：用 Classify 的 class 名（RETRYABLE/POISON/...）——不如驱动码精确，但优于空字符串。
	code := string(reliable.Classify(err, classifier))
	if len(code) > 64 {
		code = code[:64]
	}
	return code
}
