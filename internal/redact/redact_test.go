package redact //nolint:testpackage // internal package test

import (
	"errors"
	"strings"
	"testing"
)

var errRequestBearer = errors.New("request failed: Authorization: Bearer sk-leak-1234")
var errInvalid = errors.New("invalid api_key sk-deadbeef99")
var errConnectionRefused = errors.New("connection refused")

// TestRedact covers the canonical secret-redaction contract (Plan 01-01 T2,
// PATTERNS C2): decode JSON, walk the tree replacing secret-carrier VALUES
// while preserving field NAMES, re-encode.
func TestRedact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "authorization value redacted key preserved",
			in:   `{"authorization": "Bearer sk-abc"}`,
			want: `{"authorization":"[REDACTED]"}`,
		},
		{
			name: "nested x-api-key value redacted",
			in:   `{"headers": {"x-api-key": "sk-xyz"}}`,
			want: `{"headers":{"x-api-key":"[REDACTED]"}}`,
		},
		{
			name: "api_key snake case redacted",
			in:   `{"api_key": "sk-1"}`,
			want: `{"api_key":"[REDACTED]"}`,
		},
		{
			name: "token field redacted",
			in:   `{"token": "t-12345"}`,
			want: `{"token":"[REDACTED]"}`,
		},
		{
			name: "non-secret field preserved (model name, content)",
			in:   `{"model": "GLM-5.2", "max_tokens": 128}`,
			want: `{"max_tokens":128,"model":"GLM-5.2"}`,
		},
		{
			name: "secret key holding an array is replaced wholesale",
			in:   `{"token": ["sk-a", "sk-b"]}`,
			want: `{"token":"[REDACTED]"}`,
		},
		{
			name: "deeply nested secret in array of objects",
			in:   `{"items": [{"authorization": "Bearer z"}, {"ok": 1}]}`,
			want: `{"items":[{"authorization":"[REDACTED]"},{"ok":1}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Redact([]byte(tt.in))
			if err != nil {
				t.Fatalf("Redact(%q) unexpected error: %v", tt.in, err)
			}

			if string(got) != tt.want {
				t.Errorf("Redact(%q)\n got  %s\n want %s", tt.in, got, tt.want)
			}
		})
	}
}

// TestRedact_PreservesIdentityHeaderNames is the load-bearing mimicry check
// (VERIFIED-FACTS.md item #1): the 12 zcode identity header NAMES are the
// fingerprint Phase 1 must mimic. They MUST NOT be redacted; only their
// VALUES are redacted when secret-like. User-Agent is explicitly NOT a secret
// key (T2 acceptance: IsSecretKey("User-Agent") == false).
func TestRedact_PreservesIdentityHeaderNames(t *testing.T) {
	t.Parallel()
	// The 12 identity header names from VERIFIED-FACTS.md item #1 (lowercased
	// for JSON-key matching). None of these are in the secret-key allowlist.
	identityNames := []string{
		"http-referer", "user-agent", "x-os-category", "x-os-version",
		"x-platform", "x-title", "x-zcode-agent", "x-zcode-app-version",
		"x-query-id", "x-request-id", "x-session-id", "x-zcode-trace-id",
	}
	for _, name := range identityNames {
		if IsSecretKey(name) {
			t.Errorf("IsSecretKey(%q) = true; identity header names must NOT be secret keys", name)
		}
	}
	// A request-headers object carrying all 12 names with non-secret values:
	// every value is preserved verbatim, every key preserved.
	in := `{"http-referer":"ref","user-agent":"ua/1.0","x-os-category":"darwin",` +
		`"x-os-version":"25.5.0","x-platform":"x64","x-title":"agent",` +
		`"x-zcode-agent":"1","x-zcode-app-version":"0.1","x-query-id":"q1",` +
		`"x-request-id":"r1","x-session-id":"s1","x-zcode-trace-id":"t1"}`

	got, err := Redact([]byte(in))
	if err != nil {
		t.Fatalf("Redact unexpected error: %v", err)
	}
	// None of the values should have been replaced with [REDACTED].
	if strings.Contains(string(got), "[REDACTED]") {
		t.Errorf("Redact redacted a non-secret identity header value:\n got %s", got)
	}
	// Every header name still present.
	for _, name := range identityNames {
		if !strings.Contains(string(got), name) {
			t.Errorf("Redact dropped header name %q from output", name)
		}
	}
}

// TestRedact_NonJSONBearerRegexScrubbed: non-JSON input (e.g. an HTML error
// page) with a Bearer token must be regex-scrubbed (the belt-and-suspenders
// fallback, PATTERNS C2).
func TestRedact_NonJSONBearerRegexScrubbed(t *testing.T) {
	t.Parallel()

	in := `<html><body>Unauthorized: Bearer sk-deadbeef-1234</body></html>`

	got, err := Redact([]byte(in))
	if err != nil {
		t.Fatalf("Redact non-JSON unexpected error: %v", err)
	}

	if strings.Contains(string(got), "sk-deadbeef-1234") {
		t.Errorf("Redact leaked Bearer token in non-JSON input:\n got %s", got)
	}

	if !strings.Contains(string(got), "<html>") {
		t.Errorf("Redact mangled non-JSON body structure:\n got %s", got)
	}
}

// TestRedact_NonJSONSkTokenScrubbed: a bare sk- token outside JSON is scrubbed.
func TestRedact_NonJSONSkTokenScrubbed(t *testing.T) {
	t.Parallel()

	in := `error: invalid key sk-abcdef1234567890`

	got, err := Redact([]byte(in))
	if err != nil {
		t.Fatalf("Redact unexpected error: %v", err)
	}

	if strings.Contains(string(got), "sk-abcdef1234567890") {
		t.Errorf("Redact leaked sk- token:\n got %s", got)
	}
}

// TestIsSecretKey: the canonical allowlist (case-insensitive).
func TestIsSecretKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{"authorization", true},
		{"Authorization", true}, // case-insensitive
		{"AUTHORIZATION", true},
		{"api_key", true},
		{"api-key", true},
		{"apikey", true},
		{"key", true},
		{"bearer", true},
		{"token", true},
		{"x-api-key", true},
		// Explicit non-secret (the 12 identity header names + others).
		{"User-Agent", false},
		{"user-agent", false},
		{"http-referer", false},
		{"x-session-id", false},
		{"model", false},
		{"max_tokens", false},
		{"content", false},
		{"", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()

			if got := IsSecretKey(c.in); got != c.want {
				t.Errorf("IsSecretKey(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestScrubError: token-bearing error strings are scrubbed.
func TestScrubError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string // substring that must NOT appear in the scrubbed output
	}{
		{
			name: "bearer token in error",
			err:  errRequestBearer,
			want: "sk-leak-1234",
		},
		{
			name: "bare sk token in error",
			err:  errInvalid,
			want: "sk-deadbeef99",
		},
		{
			name: "clean error unchanged in substance",
			err:  errConnectionRefused,
			want: "__never_present__",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := ScrubError(c.err)
			if strings.Contains(got, c.want) {
				t.Errorf("ScrubError leaked %q:\n got %s", c.want, got)
			}
		})
	}
}

// TestScrubError_NilReturnsEmpty: a nil error scrubs to an empty string
// (avoids "<nil>" artifacts in logs).
func TestScrubError_NilReturnsEmpty(t *testing.T) {
	t.Parallel()

	if got := ScrubError(nil); got != "" {
		t.Errorf("ScrubError(nil) = %q, want empty", got)
	}
}

// TestRedact_StringValueTokenScrubbed (08-08 T1 Test 5 support): a token
// embedded in a PLAIN STRING value under a non-secret key (the tool-result
// output shape — e.g. Bash stdout "KEY=sk-livesecret…") is scrubbed by the
// JSON walker's belt-and-suspenders pass, while surrounding text and the key
// name are preserved.
func TestRedact_StringValueTokenScrubbed(t *testing.T) {
	t.Parallel()

	in := `{"type":"tool_result","output":"env shows KEY=sk-livesecretvalue123` +
		` plus Bearer sk-abc123def456 and plain words"}`

	got, err := Redact([]byte(in))
	if err != nil {
		t.Fatalf("Redact unexpected error: %v", err)
	}

	s := string(got)
	if strings.Contains(s, "sk-livesecretvalue123") || strings.Contains(s, "sk-abc123def456") {
		t.Errorf("Redact leaked a token inside a string value:\n got %s", s)
	}

	if !strings.Contains(s, "[REDACTED]") {
		t.Errorf("Redact did not scrub the embedded tokens:\n got %s", s)
	}

	for _, want := range []string{"env shows", "plus", `"output"`} {
		if !strings.Contains(s, want) {
			t.Errorf("Redact mangled surrounding text or key names (missing %q):\n got %s", want, s)
		}
	}

	// A string with no token shape passes through verbatim (structure and
	// non-secret content are the transcript's fidelity contract).
	plain := `{"text":"the task-123 notes say hi"}`

	gotPlain, err := Redact([]byte(plain))
	if err != nil {
		t.Fatalf("Redact plain unexpected error: %v", err)
	}

	if string(gotPlain) != `{"text":"the task-123 notes say hi"}` {
		t.Errorf("Redact altered a token-free string: %s", gotPlain)
	}
}
