package gormshared

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeMsgRedactsSecrets(t *testing.T) {
	cases := []struct {
		name string
		in   string
		leak string // 不得出现在结果里的片段
	}{
		{"mysql dsn", `dial error: root:s3cr3t@tcp(10.0.0.1:3306)/db`, "s3cr3t"},
		// S2：租户命名用户名（非 root/test/admin/app）也必须脱敏——原稿硬编码列表在此漏掉。
		{"tenant dsn (S2)", `conn fail: tenant_42:S3cret@tcp(db:3306)/evidence`, "S3cret"},
		{"pg uri dsn", `dial postgres://app:hunter2@10.0.0.1:5432/db`, "hunter2"},
		{"pg dsn", `pq: host=10.0.0.1 user=app password=hunter2 dbname=x`, "hunter2"},
		{"bearer", `401 Unauthorized: Authorization=Bearer eyJhbGciOi.abc-123`, "eyJhbGciOi.abc-123"},
		{"api key header", `500 upstream: X-API-Key: AKIAEXAMPLE123`, "AKIAEXAMPLE123"},
		{"json secret", `body {"api_key":"AKIA0000","id":1}`, "AKIA0000"},
		// S1：人员 PII（spec §10）——身份证号不得进 error_message / error_fingerprint。
		{"pii id_card (S1)", `upsert failed for {"id_card":"110101199001011234","status":1}`, "110101199001011234"},
		{"pii phone (S1)", `notify failed: {"phone":"13800138000","ok":false}`, "13800138000"},
		// R2：数组形态 PII
		{"pii phone array (R2)", `batch fail: {"phones":["13800138000","13900139000"]}`, "13800138000"},
		// R2：Windows 配置路径（项目跑在 D:\JXT\...）
		{"windows config path (R2)", `open D:\JXT\jxt-core\config\settings.yml failed`, "settings.yml"},
		// R2：URL 编码 DSN（%40 代 @）
		{"url-encoded dsn (R2)", `dial root:pwd%40tcp(10.0.0.1:3306)/db`, "pwd"},
		// R3：Windows 正斜杠配置路径（Go 在 Windows 常以此形式输出路径）
		{"windows fwd-slash path (R3)", `open D:/JXT/jxt-evidence-system/jxt-core/config/settings.yml failed`, "settings.yml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeMsg(tc.in)
			assert.NotContains(t, got, tc.leak, "D11: secret must be redacted before hitting the DB")
			assert.Contains(t, got, "[REDACTED]")
		})
	}
}

func TestSanitizeMsgTruncatesOnRuneBoundary(t *testing.T) {
	// 3 字节/字符的中文，长度跨过 2048 且不能被 2048 整除 → 裸 s[:2048] 会切半。
	in := strings.Repeat("测", 1000)
	got := sanitizeMsg(in)
	assert.LessOrEqual(t, len(got), maxErrorMessageBytes)
	assert.True(t, utf8.ValidString(got), "C5: truncation must not produce invalid UTF-8")
}

func TestFingerprintIsStableSha256(t *testing.T) {
	a := fingerprint(reliable.ClassRetryable, "deadlock on tx 1")
	b := fingerprint(reliable.ClassRetryable, "deadlock on tx 1")
	assert.Equal(t, a, b, "same class+msg -> same fingerprint")
	assert.Len(t, a, 64, "D10: sha256 hex")
	assert.NotEqual(t, a, fingerprint(reliable.ClassPoison, "deadlock on tx 1"), "class participates")
}

// 同一个 secret 的不同取值不得产生不同指纹——否则 error_fingerprint 聚合会被凭证打散。
func TestFingerprintIsSecretInvariant(t *testing.T) {
	a := fingerprint(reliable.ClassRetryable, `root:pwd-A@tcp(1.2.3.4:3306)/db timeout`)
	b := fingerprint(reliable.ClassRetryable, `root:pwd-B@tcp(1.2.3.4:3306)/db timeout`)
	assert.Equal(t, a, b, "redaction happens before hashing, so rotating creds don't fragment aggregation")
}
