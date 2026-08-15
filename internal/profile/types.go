package profile

import (
	"encoding/json"
	"fmt"
	"time"
)

// Profile is a configurable bundle of system prompts, tool catalog, message
// shape, and identity that ass-guard shapes every outgoing model-provider
// request from (PROF-01). A profile is selected by name; zcode is profile #1,
// and the architecture supports N profiles from day one — the Shaper and loader
// contain no profile-specific code paths (PROF-02).
//
// Field fidelity follows D-06's three-tier model:
//   - TIER-1 byte-faithful: System text, Tools (name+InputSchema), Thinking,
//     ToolChoice — model-visible content the mimicry thesis depends on.
//   - TIER-2 structural: Header NAMES must be present; VALUES are per-session
//     and templated (never stored verbatim — T-01-02).
//   - TIER-3 informational: Model/MaxTokens/Name are metadata; logged for audit
//     but not drift-flagged on value variance.
type Profile struct {
	Name       string          `json:"name"        yaml:"name"`
	Model      string          `json:"model"       yaml:"model"`
	MaxTokens  int             `json:"max_tokens"  yaml:"max_tokens"`
	System     []TextBlock     `json:"system"      yaml:"system"`
	Tools      []Decl          `json:"tools"       yaml:"tools"`
	Headers    []Header        `json:"headers"     yaml:"headers"`
	Thinking   json.RawMessage `json:"thinking"    yaml:"thinking"`
	ToolChoice json.RawMessage `json:"tool_choice" yaml:"tool_choice"`
	// CaptureWorkDir is the CAPTURED session's absolute working directory —
	// the value the runtime-composed env/system fields carried at capture time
	// (e.g. the "Primary working directory" line of the zcode env block). The
	// mimicry target composes those fields from the RUNTIME cwd per session;
	// ass-guard's Shaper substitutes the session's actual workDir for this
	// value at composition time (form identical to the capture, value =
	// current session cwd). Empty = the profile declares no cwd-bearing
	// runtime-composed content (no substitution happens).
	CaptureWorkDir string `json:"capture_work_dir" yaml:"capture_work_dir"`
}

// TextBlock is one entry of the Anthropic-shape system[] array. The Text is
// stored byte-faithful from the capture (TIER-1).
type TextBlock struct {
	Type string `json:"type" yaml:"type"`
	Text string `json:"text" yaml:"text"`
}

// Decl is a model-facing tool declaration. InputSchema is the verbatim captured
// JSON schema; the profile-declared schema is authoritative at runtime (TOOL-02).
type Decl struct {
	Name        string          `json:"name"         yaml:"name"`
	Description string          `json:"description"  yaml:"description"`
	InputSchema json.RawMessage `json:"input_schema" yaml:"input_schema"`
}

// Header is one identity header. Name is byte-faithful (TIER-2 structural);
// ValueTemplate carries a placeholder rendered per-request (real values are
// per-session ephemera and never stored verbatim — T-01-02).
type Header struct {
	Name          string `json:"name"           yaml:"name"`
	ValueTemplate string `json:"value_template" yaml:"value_template"`
}

// Tier labels a captured field's fidelity band (D-06):
//   - Tier1ByteFaithful: model-visible content the mimicry thesis depends on
//     (system-prompt text, tool name+input_schema, tool_choice). Drift = hard fail.
//   - Tier2Structural: presence + shape matter, exact values don't (the 12
//     identity header NAMES; the system/tools array shape). Structural drift is
//     flagged; per-session value variance (e.g. an x-request-id value) is not.
//   - Tier3Informational: timestamps, token counts, request ids. Audit-logged
//     only; NEVER drift-flagged.
type Tier int

const (
	// Tier1ByteFaithful is the byte-exact band.
	Tier1ByteFaithful Tier = 1
	// Tier2Structural is the presence-and-shape band.
	Tier2Structural Tier = 2
	// Tier3Informational is the audit-only band.
	Tier3Informational Tier = 3
)

// String returns the canonical tier name for logging/manifests.
func (t Tier) String() string {
	switch t {
	case Tier1ByteFaithful:
		return "TIER-1"
	case Tier2Structural:
		return "TIER-2"
	case Tier3Informational:
		return "TIER-3"
	default:
		return fmt.Sprintf("TIER-?%d", int(t))
	}
}

// DriftFlagged reports whether the drift detector should flag a change in this
// field's value. TIER-1 and TIER-2 are flagged; TIER-3 (per-request ephemera) is
// audit-only and never flagged (D-06).
func (t Tier) DriftFlagged() bool { return t == Tier1ByteFaithful || t == Tier2Structural }

// CoverageEntry is one row of the machine-readable coverage manifest (PROF-05,
// D-07). It ties a captured field to its tier, type, observed count, and the
// source location the value was extracted from.
type CoverageEntry struct {
	Path          string `json:"path"           yaml:"path"`
	Tier          Tier   `json:"tier"           yaml:"tier"`
	Type          string `json:"type"           yaml:"type"`
	ObservedCount int    `json:"observed_count" yaml:"observed_count"`
	Source        string `json:"source"         yaml:"source"`
}

// SessionRef is one extraction-source session inside a TargetCaptureRef.
type SessionRef struct {
	ID   string `json:"id"   yaml:"id"`
	Path string `json:"path" yaml:"path"`
	Role string `json:"role" yaml:"role"`
}

// TargetCaptureRef records the provenance of a profile extraction (PROF-03):
// the session(s) and corrected on-disk path the profile fields were read from.
type TargetCaptureRef struct {
	Sessions []SessionRef `json:"sessions" yaml:"sessions"`
	// ZcodeVersion is the verbatim `zcode --version` output at capture time
	// (09-03, Pitfall 18 "record BOTH versions" — extraction comparability
	// needs the capture-side version alongside ExtractorVersion). Empty means
	// captured before the field existed.
	ZcodeVersion     string    `json:"zcode_version"     yaml:"zcode_version"`
	ExtractedAt      time.Time `json:"extracted_at"      yaml:"extracted_at"`
	ExtractorVersion string    `json:"extractor_version" yaml:"extractor_version"`
}

// CoverageManifest is the machine-readable coverage manifest shipped with the
// profile (PROF-05, D-07). It makes "incomplete capture" a loud failure and
// feeds the TOOL-03 catalog-consistency check.
type CoverageManifest struct {
	Profile          string           `json:"profile"            yaml:"profile"`
	TargetCaptureRef TargetCaptureRef `json:"target_capture_ref" yaml:"target_capture_ref"`
	ExtractedAt      time.Time        `json:"extracted_at"       yaml:"extracted_at"`
	ExtractorVersion string           `json:"extractor_version"  yaml:"extractor_version"`
	Fields           []CoverageEntry  `json:"fields"             yaml:"fields"`

	// RequiredToolsSatisfiedByCatalog has dual json/yaml tags whose key name
	// matches the serialized manifest format and cannot be shortened.
	//nolint:lll // long field name + required json/yaml keys
	RequiredToolsSatisfiedByCatalog bool `json:"required_tools_satisfied_by_catalog" yaml:"required_tools_satisfied_by_catalog"`
}

// Validate returns an error naming any TIER-1/2 manifest field whose observed
// count in a fresh capture differs from the manifest's declared ObservedCount
// (the PROF-05 incomplete-capture gate). TIER-3 fields are not validated.
func (m *CoverageManifest) Validate(captured map[string]int) error {
	var mismatches []string

	for _, f := range m.Fields {
		if !f.Tier.DriftFlagged() {
			continue
		}

		got, ok := captured[f.Path]
		if !ok {
			mismatches = append(mismatches, f.Path+": missing from capture")

			continue
		}

		if got != f.ObservedCount {
			mismatches = append(mismatches,
				fmt.Sprintf("%s: observed %d, manifest declares %d (%s)",
					f.Path, got, f.ObservedCount, f.Tier))
		}
	}

	if len(mismatches) > 0 {
		return fmt.Errorf("coverage check failed: %v", mismatches) //nolint:err113 // dynamic error message
	}

	return nil
}
