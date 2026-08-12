// Package redact is the shared secret-redaction chokepoint (PATTERNS C2,
// LOG-03 foundation). It provides a JSON-tree walker that replaces
// secret-carrier VALUES while preserving field NAMES, plus regex
// belt-and-suspenders for non-JSON bodies and error strings.
//
// The canonical secret-key allowlist is case-insensitive:
//
//	authorization, api_key, api-key, apikey, key, bearer, token, x-api-key
//
// Field NAMES that carry identity (the 12 zcode identity header names —
// HTTP-Referer, User-Agent, x-session-id, etc.) are NOT secret keys and are
// preserved verbatim; only their VALUES are redacted when the value itself
// is secret-like (e.g. an Authorization header). This preserves the mimicry
// fingerprint (VERIFIED-FACTS.md item #1) while never leaking a credential.
//
// Redact is a shared package: Phase 1 has >=3 callers (the audit log, the
// parity harness's results writer, the extractor's provenance output). Do
// not copy-paste the spike's redact logic — import this.
package redact

import (
	"encoding/json"
	"regexp"
	"strings"
)

// redacted is the canonical placeholder for a scrubbed secret value.
const redacted = "[REDACTED]"

// secretKeys is the case-insensitive set of field names whose VALUES are
// always secret. Mirrors spikes/02-openai-toolschema/main.go isSecretKey.
var secretKeys = map[string]struct{}{ //nolint:gochecknoglobals // immutable table
	"authorization": {},
	"api_key":       {},
	"api-key":       {},
	"apikey":        {},
	"key":           {},
	"bearer":        {},
	"token":         {},
	"x-api-key":     {},
}

// IsSecretKey reports whether k names a secret-carrying field
// (case-insensitive). Identity header names (User-Agent, x-session-id, ...)
// return false — they are the mimicry fingerprint, not secrets.
func IsSecretKey(k string) bool {
	_, ok := secretKeys[strings.ToLower(strings.TrimSpace(k))]

	return ok
}

// bearerRe matches "Bearer <token>" with a trailing-word-boundary token.
// Token is permissive (word chars, dots, dashes, underscores) to cover
// "Bearer sk-...", "Bearer eyJ...", "Bearer deadbeef-1234".
var bearerRe = regexp.MustCompile(`(?i)\bBearer\b\s+[A-Za-z0-9._\-=]+`)

// skRe matches a bare "sk-<token>" (the Anthropic/OpenAI key prefix) so
// leaked keys in non-JSON bodies and error strings are scrubbed.
var skRe = regexp.MustCompile(`sk-[A-Za-z0-9_\-]{6,}`)

// Redact decodes raw as JSON and returns a re-encoded copy with every
// secret-carrier value replaced by "[REDACTED]", field names preserved. If
// raw is not valid JSON, it falls back to regex scrubbing of Bearer/sk-
// tokens (belt-and-suspenders for HTML error pages and other non-JSON
// bodies). The JSON path preserves the mimicry fingerprint (identity header
// names + structural keys); the regex path preserves the non-JSON body
// structure minus any embedded tokens.
func Redact(raw []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) == 0 {
		return raw, nil
	}
	// Fast path: valid JSON. Walk the tree, replace secret values, re-encode.
	var node any

	err := json.Unmarshal(raw, &node)
	if err == nil {
		walkRedact(node)

		out, err := json.Marshal(node)
		if err != nil {
			return nil, err
		}

		return out, nil
	}
	// Fallback: non-JSON (HTML error pages, plain text). Regex-scrub tokens
	// in place; preserve the surrounding body structure.
	return scrubBytes(raw), nil
}

// walkRedact recurses through the decoded JSON tree, replacing secret
// values in place. Field names are preserved (the mimicry fingerprint).
func walkRedact(node any) {
	switch v := node.(type) {
	case map[string]any:
		for k, val := range v {
			if IsSecretKey(k) {
				v[k] = redacted

				continue
			}

			walkRedact(val)
		}
	case []any:
		for i := range v {
			walkRedact(v[i])
		}
	}
}

// scrubBytes applies the Bearer/sk- regex scrubbers to a non-JSON body.
func scrubBytes(raw []byte) []byte {
	s := string(raw)
	s = bearerRe.ReplaceAllString(s, "Bearer "+redacted)
	s = skRe.ReplaceAllString(s, redacted)

	return []byte(s)
}

// ScrubError returns a redacted string for err. Tokens (Bearer, sk-) in the
// error message are replaced with "[REDACTED]"; a nil error returns "".
// Use this whenever an error may carry a credential (provider errors, HTTP
// transport errors) before it reaches stderr or a log.
func ScrubError(err error) string {
	if err == nil {
		return ""
	}

	return string(scrubBytes([]byte(err.Error())))
}
