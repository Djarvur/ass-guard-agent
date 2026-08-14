package drift

import (
	"fmt"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// Drift is one TIER-1/2 field whose captured value differs from the manifest's
// declared expectation in a way the drift detector flags.
type Drift struct {
	FieldPath string `json:"field_path" yaml:"field_path"`
	Tier      profile.Tier
	Reason    string `json:"reason" yaml:"reason"`
}

// Detect compares a freshly-captured request (as a map[string]any decoded from
// the live-capture JSONL line) against the manifest's declared fields, returning
// the TIER-1/2 drifts. TIER-3 fields are ignored.
//
// Drift semantics (D-06):
//   - TIER-1 (system blocks, tools, thinking, tool_choice): count-based — a
//     different observed count is a drift. (Byte-value drift of the captured
//     text is handled by the coverage manifest's byte-equality checks; this
//     function works on the structural counts the manifest records.)
//   - TIER-2 (header NAMES, message-block shape): presence/count — value
//     variance for per-session fields (e.g. an x-request-id value) is NOT a
//     drift; only the NAME set / count matters.
func Detect(manifest *profile.CoverageManifest, captured map[string]any) []Drift {
	var d []Drift

	for _, f := range manifest.Fields {
		if !f.Tier.DriftFlagged() {
			continue
		}

		got, ok := captured[f.Path]
		if !ok {
			d = append(d, Drift{FieldPath: f.Path, Tier: f.Tier, Reason: "missing from capture"})

			continue
		}

		n := countOf(got)
		if n != f.ObservedCount {
			d = append(d, Drift{
				FieldPath: f.Path,
				Tier:      f.Tier,
				Reason:    fmt.Sprintf("count drift: observed %d, manifest declares %d", n, f.ObservedCount),
			})
		}
	}

	return d
}

// countOf returns the structural count of a captured value: length of a slice,
// length of a map (for the header-name set), or 1 for a present scalar.
func countOf(v any) int {
	switch t := v.(type) {
	case []any:
		return len(t)
	case map[string]any:
		return len(t)
	default:
		return 1
	}
}
