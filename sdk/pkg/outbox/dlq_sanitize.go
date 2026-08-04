package outbox

import "regexp"

// MetricOutboxDeadLetteredTotal is the canonical Prometheus counter name for publish-side outbox
// events that entered the dead_lettered terminal state (opus5-RCC-v2 §8.4③). It lives here, in the
// outbox package, so publish-only services can reference it WITHOUT importing sdk/pkg/reliable
// (§8.5 dependency matrix: security/process import nothing from reliable). sdk/pkg/reliable defines
// the same string for the consumption side; both intentionally equal "outbox_dead_lettered_total".
const MetricOutboxDeadLetteredTotal = "outbox_dead_lettered_total"

// dlqSecretRe scrubs credential / PII-bearing patterns from publisher LastError strings before
// they reach any log (spec §10). Covers DSN passwords (:pass@), key=value secrets (password=,
// token=, access_token=, refresh_token=, api_key=, apikey=, secret=, client_secret=), Bearer
// tokens, bare JWTs (eyJ…), and single-quoted literals. Case-insensitive.
var dlqSecretRe = regexp.MustCompile(`(?i)(:[^@/:]+@` +
	`|(?:password|passwd|pwd|token|access_token|refresh_token|api_key|apikey|secret|client_secret)=[^\s&;,]*` +
	`|bearer\s+[^\s]+` +
	`|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*` +
	`|'[^']*')`)

// SanitizeLastError truncates a publisher error string to 2 KB and scrubs credential/PII patterns
// before it reaches any log (spec §10). Truncation runs BEFORE scrubbing: the discarded tail never
// reaches the log, and a value cut at a key= boundary still matches its own prefix and is redacted.
// This is the single shared scrubber used by every service's outbox DLQ handler.
func SanitizeLastError(s string) string {
	if len(s) > 2000 {
		s = s[:2000]
	}
	return dlqSecretRe.ReplaceAllString(s, "<redacted>")
}
