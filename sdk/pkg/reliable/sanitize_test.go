package reliable

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// redactsCases mirrors the kernel gormshared/fingerprint_test.go PII table
// (spec §10 / D11 day-1 redaction floor): DSN / credentials / token / staff
// PII / stack paths. Each entry must lose `leak` and gain "[REDACTED]" after
// sanitization. Kept in lock-step with the kernel cases so the two sanitize
// copies cannot silently drift on the PII surface.
var redactsCases = []struct {
	name string
	in   string
	leak string // must NOT appear in the sanitized output
}{
	{"mysql dsn", `dial error: root:s3cr3t@tcp(10.0.0.1:3306)/db`, "s3cr3t"},
	// S2: tenant-named username (not root/test/admin/app) must also redact.
	{"tenant dsn (S2)", `conn fail: tenant_42:S3cret@tcp(db:3306)/evidence`, "S3cret"},
	{"pg uri dsn", `dial postgres://app:hunter2@10.0.0.1:5432/db`, "hunter2"},
	{"pg password=", `pq: host=10.0.0.1 user=app password=hunter2 dbname=x`, "hunter2"},
	// quoted key=value（INI/YAML 回显）：双引号 / 单引号形态也要脱敏——原 [^;&\s"']+ 排除引号会漏配。
	{"quoted password double", `conn fail password="hunter2" retry`, "hunter2"},
	{"quoted password single", `cfg password='s3cr3t' ok`, "s3cr3t"},
	{"bearer token", `401 Unauthorized: Authorization=Bearer eyJhbGciOi.abc-123`, "eyJhbGciOi.abc-123"},
	{"api key header", `500 upstream: X-API-Key: AKIAEXAMPLE123`, "AKIAEXAMPLE123"},
	{"json secret", `body {"api_key":"AKIA0000","id":1}`, "AKIA0000"},
	// S1: staff PII (spec §10) — id_card / phone must never reach error_message.
	{"pii id_card (S1)", `upsert failed for {"id_card":"110101199001011234","status":1}`, "110101199001011234"},
	{"pii phone (S1)", `notify failed: {"phone":"13800138000","ok":false}`, "13800138000"},
	// R2: array-form PII ("phones":[...]) — batch handler error aggregation hotspot.
	{"pii phone array (R2)", `batch fail: {"phones":["13800138000","13900139000"]}`, "13800138000"},
	// R2: Windows config path (the project runs under D:\JXT\...).
	{"windows config path (R2)", `open D:\JXT\jxt-core\config\settings.yml failed`, "settings.yml"},
	// R2: URL-encoded DSN (%40 encodes @ — cross-service HTTP echo).
	{"url-encoded dsn (R2)", `dial root:pwd%40tcp(10.0.0.1:3306)/db`, "pwd"},
	// R3: Windows forward-slash path (Go on Windows often emits D:/JXT/...).
	{"windows fwd-slash path (R3)", `open D:/JXT/jxt-evidence-system/jxt-core/config/settings.yml failed`, "settings.yml"},
}

func TestSanitizeForStorage_RedactsPII(t *testing.T) {
	for _, tc := range redactsCases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeForStorage(tc.in)
			if strings.Contains(got, tc.leak) {
				t.Fatalf("D11 leak: %q still present in sanitized output %q", tc.leak, got)
			}
			if !strings.Contains(got, "[REDACTED]") {
				t.Fatalf("expected [REDACTED] marker in output, got %q", got)
			}
		})
	}
}

// SanitizeForLog shares the storage scrubber (same D11 floor on the log path —
// F4-extend: the DLQ cause can echo the tenant DSN and the sdk logger has no
// built-in redaction). Assert the contract here so a future "lighter form"
// divergence cannot silently drop the floor.
func TestSanitizeForLog_SameFloorAsStorage(t *testing.T) {
	for _, tc := range redactsCases {
		got := SanitizeForLog(tc.in)
		if strings.Contains(got, tc.leak) {
			t.Fatalf("SanitizeForLog leaked %q: %q", tc.leak, got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Fatalf("SanitizeForLog expected [REDACTED] marker, got %q", got)
		}
	}
}

// TestSanitizeForStorage_TruncatesAt2KBRuneBoundary (C5): the 2KB cut must not
// split a multi-byte rune. The utf8mb4 column rejects invalid UTF-8 (MySQL
// error 1366) or silently replaces it with U+FFFD; CJK error messages hit this
// routinely. 3 bytes/CJK char × 1000 + ASCII comfortably straddles 2048.
func TestSanitizeForStorage_TruncatesAt2KBRuneBoundary(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("世界ab") // 2×3-byte CJK + 2 ASCII = 8 bytes/iter → 8000 bytes
	}
	out := SanitizeForStorage(sb.String())
	if len(out) > maxErrorMessageBytes {
		t.Fatalf("output exceeds 2KB ceiling: %d bytes", len(out))
	}
	if !utf8.ValidString(out) {
		t.Fatalf("output is not valid UTF-8 (rune split)")
	}
}

// TestSanitizeForStorage_SecondTruncateCapsRedactionGrowth: redaction can GROW
// the string ([REDACTED] is longer than a short match). The post-regex truncate
// must re-enforce the 2KB ceiling. Feed many short pwd= matches that each grow.
func TestSanitizeForStorage_SecondTruncateCapsRedactionGrowth(t *testing.T) {
	// "pwd=x " is 6 bytes → "[REDACTED] " is 11 bytes (growth). 600 iters =
	// 3600 bytes in; the post-redaction intermediate is ~6600, well past 2KB.
	var sb strings.Builder
	for i := 0; i < 600; i++ {
		sb.WriteString("pwd=x ")
	}
	out := SanitizeForStorage(sb.String())
	if len(out) > maxErrorMessageBytes {
		t.Fatalf("redaction growth escaped the second truncate: %d bytes", len(out))
	}
	if !utf8.ValidString(out) {
		t.Fatalf("output is not valid UTF-8")
	}
	if !strings.Contains(out, "[REDACTED]") {
		preview := out
		if len(preview) > 64 {
			preview = preview[:64]
		}
		t.Fatalf("expected [REDACTED] in output, got %q", preview)
	}
}

// TestSanitizeForStorage_NearCapBoundaryMatch: a secret that begins inside the
// first 2KB and straddles the cut is still redacted (pre-truncate keeps the
// lead-in, the regex matches, the second truncate re-caps). Output stays ≤2KB
// and the secret never leaks. This exercises the truncate→regex→truncate order
// at the boundary.
func TestSanitizeForStorage_NearCapBoundaryMatch(t *testing.T) {
	fill := strings.Repeat("x", 2030) // 2030 filler bytes
	in := fill + "password=hunter2 trailing tail beyond the cap that must not leak"
	out := SanitizeForStorage(in)
	if len(out) > maxErrorMessageBytes {
		t.Fatalf("near-cap output exceeds 2KB: %d bytes", len(out))
	}
	if strings.Contains(out, "hunter2") {
		t.Fatalf("near-cap secret leaked: %q", out)
	}
	if !utf8.ValidString(out) {
		t.Fatalf("near-cap output is not valid UTF-8")
	}
}

// TestSanitizeForStorage_UsesLinearRegexNotNestedQuantifier: the Unix path
// pattern MUST be the linear single-layer form `/[A-Za-z0-9_\-./]*\.ext`, not
// the kernel's nested-quantifier `(/[A-Za-z0-9_\-./]+)+\.ext` shape. The
// nested shape is `(a+)+` over a class that contains `/` and `.` — exponential
// on `/a/a/a/…` (no .ext terminator) under a backtracking engine. Go's regexp
// is RE2 (linear by construction), so this is a defense-in-depth SOURCE
// assertion, not a timing claim. The discriminator is the pattern source: the
// nested form contains the 4-char group-quantifier signature `)+\.` (close-
// paren, plus, backslash, dot); the linear form never does. A character-class
// quantifier `[...]+\.` is linear and does NOT match the signature.
func TestSanitizeForStorage_UsesLinearRegexNotNestedQuantifier(t *testing.T) {
	const nestedSig = ")+\\." // Go string literal → 4 chars: ) + \ .
	for _, p := range redactorPatterns {
		if strings.Contains(p.String(), nestedSig) {
			t.Fatalf("nested-quantifier signature %q in pattern %q — must be linear", nestedSig, p.String())
		}
	}
	// Pin the Unix-path pattern source to the linear literal so a paste of the
	// kernel pattern is caught at review time.
	want := `(?i)/[A-Za-z0-9_\-./]*\.(ya?ml|env|conf|key|pem)\b`
	found := false
	for _, p := range redactorPatterns {
		if p.String() == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("linear Unix-path pattern not present; expected %q in redactorPatterns", want)
	}
}

// TestSanitizeForStorage_LongPathLikeInputIsBounded: a pathological `/a/a/a/…`
// run with no .ext terminator is the input shape the nested-quantifier pattern
// is vulnerable to under backtracking. Under the linear pattern it is bounded
// in both time and output size.
func TestSanitizeForStorage_LongPathLikeInputIsBounded(t *testing.T) {
	in := "/" + strings.Repeat("a/", 5000) // 10001 bytes, no .yml terminator
	out := SanitizeForStorage(in)
	if len(out) > maxErrorMessageBytes {
		t.Fatalf("pathological path output exceeds 2KB: %d bytes", len(out))
	}
	if !utf8.ValidString(out) {
		t.Fatalf("pathological path output is not valid UTF-8")
	}
}

// TestSanitizeForStorage_LongInputDoesNotLeakBeyondCap: a secret located
// entirely AFTER the 2KB cut is truncated away before the regex pass and never
// reaches the stored output. Guards against a future reorder that feeds the
// full unbounded string to the regex engine.
func TestSanitizeForStorage_LongInputDoesNotLeakBeyondCap(t *testing.T) {
	in := strings.Repeat("x", 3000) + "password=after-cap-secret"
	out := SanitizeForStorage(in)
	if len(out) > maxErrorMessageBytes {
		t.Fatalf("output exceeds 2KB: %d bytes", len(out))
	}
	if strings.Contains(out, "after-cap-secret") {
		t.Fatalf("post-cap secret leaked into output: %q", out)
	}
}
