package session

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// DefaultToolResultCapBytes bounds a tool-result Output payload before it
// enters the transcript (14-05, EARLY-05 — the token-economics lever): 128
// KiB, deliberately ABOVE the 08-08 corpus harvest's largest observed result
// (~70KB, untruncated, with NO truncation markers anywhere in the corpus), so
// no corpus-typical result is ever modified while the pathological case is
// bounded before it can flood the model's context window (T-14-14).
const DefaultToolResultCapBytes = 131072

// truncateToolResult bounds an over-cap tool-output string: the TAIL is kept
// (the most recent output — error summaries live at the tail) and prefixed by
// a one-line marker stating the original and kept byte sizes. Results at or
// under the cap are returned byte-unmodified (capture fidelity — the cap sits
// above the corpus maximum by design). The byte-boundary slice advances past
// a partial UTF-8 rune so the marker's size math survives JSON re-encoding.
//
// CORPUS-ABSENT FORM: the 08-08 harvest found NO truncation marker in the
// corpus (largest observed result ~70KB, untruncated) — the marker text below
// is a documented ass-guard form, NOT a captured zcode form. It must never be
// added to any capture-pinning fixture as if observed; the captured marker
// shape is routed to the 12-05 re-record for the post-adoption capture.
func truncateToolResult(s string) string {
	if len(s) <= DefaultToolResultCapBytes {
		return s
	}

	tail := s[len(s)-DefaultToolResultCapBytes:]

	// Rune-align the slice start: json.Marshal replaces invalid UTF-8 with
	// U+FFFD, which would silently change the kept size the marker reports.
	for i := 0; i < utf8.UTFMax-1 && tail != "" && !utf8.RuneStart(tail[0]); i++ {
		tail = tail[1:]
	}

	marker := fmt.Sprintf("[ass-guard: tool output truncated; original %d bytes, kept tail %d bytes]",
		len(s), len(tail))

	return marker + "\n" + tail
}

// boundedToolResult is the ONE append-boundary chokepoint (14-05, EARLY-05):
// every tool Output payload passes through here on its way into the
// transcript — before the line exists, hence before BOTH the mid-turn model
// view and the Projector's projected window fold it in. It decodes only the
// JSON-STRING payload form (the captured plain-text result form — Bash
// output, Read's numbered text, Write/Edit success texts render as JSON
// strings): an over-cap string is bounded + re-encoded; an under-cap string
// is returned BYTE-IDENTICAL (no decode/re-encode round trip). Every other
// shape (the structured {"error":…} convention, the openspec
// {stdout,stderr,exit_code,classification} objects, TodoWrite's JSON echo,
// subagent dispatch-error forms) passes through untouched — only a tool
// Output payload is ever bounded, never transcript metadata or the
// AskUserQuestion surfaces (which do not flow through this seam).
func boundedToolResult(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || raw[0] != '"' {
		return raw
	}

	var s string
	if json.Unmarshal(raw, &s) != nil {
		return raw
	}

	if len(s) <= DefaultToolResultCapBytes {
		return raw
	}

	out, err := json.Marshal(truncateToolResult(s))
	if err != nil {
		return raw
	}

	return out
}
