package outbox

import (
	"strings"
	"testing"
)

// TestSanitizeLastError documents and enforces the credential/PII scrubbing contract for the
// shared outbox DLQ scrubber (spec §10). It asserts real input->output behavior: each
// forbidden credential fragment must be absent from the sanitized result, and the 2 KB
// truncation invariant must hold for every input. The plain-error case additionally proves
// the scrubber does not inject <redacted> into benign strings.
func TestSanitizeLastError(t *testing.T) {
	// Direct constant assertion (§8.5 name home): the canonical dead-letter counter name
	// lives in the outbox package and equals the reliable-side string.
	if MetricOutboxDeadLetteredTotal != "outbox_dead_lettered_total" {
		t.Fatalf("MetricOutboxDeadLetteredTotal = %q, want %q",
			MetricOutboxDeadLetteredTotal, "outbox_dead_lettered_total")
	}

	cases := []struct {
		name      string
		input     string
		forbidden string // credential/PII fragment that must NOT survive scrubbing
	}{
		{"dsn password clause", "host=localhost password=hunter2 other=x", "hunter2"},
		{"token", "token=abc.def.ghi", "abc.def.ghi"},
		{"api_key", "api_key=AKIAEXAMPLE", "AKIAEXAMPLE"},
		{"apikey", "apikey=AKIAEXAMPLE", "AKIAEXAMPLE"},
		{"secret", "secret=topsecret", "topsecret"},
		{"access_token", "access_token=ya29.x", "ya29.x"},
		{"bearer token", "Authorization: Bearer mF_9.B5f-4", "mF_9.B5f-4"},
		{"dsn user password", "postgres://user:p%40ss@host:5432/db", "p%40ss"},
		{"bare jwt body", "jwt=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIx.SflKxw", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIx.SflKxw"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeLastError(tc.input)
			if strings.Contains(got, tc.forbidden) {
				t.Errorf("SanitizeLastError(%q) = %q; must not contain forbidden %q",
					tc.input, got, tc.forbidden)
			}
			if len(got) > 2000 {
				t.Errorf("SanitizeLastError(%q) length %d exceeds 2000", tc.input, len(got))
			}
		})
	}

	// Benign error: content must survive verbatim with NO spurious <redacted>.
	t.Run("plain error verbatim", func(t *testing.T) {
		const in = "plain error: connection refused"
		got := SanitizeLastError(in)
		if got != in {
			t.Errorf("SanitizeLastError(%q) = %q; want verbatim input", in, got)
		}
		if strings.Contains(got, "<redacted>") {
			t.Errorf("SanitizeLastError(%q) = %q; spurious <redacted> injected", in, got)
		}
	})

	// Oversized input ending in a secret beyond the 2 KB cut: the discarded tail AND the
	// secret must both be absent from the result.
	t.Run("truncation discards tail and secret", func(t *testing.T) {
		suffix := "password=tail"
		in := strings.Repeat("a", 3000-len(suffix)) + suffix
		if len(in) != 3000 {
			t.Fatalf("setup: input length %d, want 3000", len(in))
		}
		got := SanitizeLastError(in)
		if len(got) > 2000 {
			t.Errorf("result length %d exceeds 2000 (truncation failed)", len(got))
		}
		if strings.Contains(got, "password=tail") {
			t.Errorf("result still contains tail secret: %q...", got[:min(120, len(got))])
		}
		if strings.Contains(got, "tail") {
			t.Errorf("result still contains tail fragment")
		}
	})

	// Truncation cuts at a key= value boundary: the surviving prefix still matches the
	// credential alternation and is redacted (SanitizeLastError docstring contract).
	t.Run("boundary cut at key value redacts", func(t *testing.T) {
		// 1990 filler bytes, then "password=" (indices 1990-1998), then a secret value
		// starting at index 1999 that extends past the 2000-byte truncation point.
		in := strings.Repeat("a", 1990) + "password=" + strings.Repeat("Z", 20)
		if len(in) <= 2000 {
			t.Fatalf("setup: input length %d must exceed 2000", len(in))
		}
		got := SanitizeLastError(in)
		if len(got) > 2000 {
			t.Errorf("result length %d exceeds 2000", len(got))
		}
		if strings.Contains(got, "password=") {
			t.Errorf("result still contains key marker %q after boundary cut", "password=")
		}
		if strings.Contains(got, "Z") {
			t.Errorf("result still contains secret value char after boundary cut")
		}
	})
}
