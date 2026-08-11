package profile

import "encoding/json"

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
	Name       string          `yaml:"name" json:"name"`
	Model      string          `yaml:"model" json:"model"`
	MaxTokens  int             `yaml:"max_tokens" json:"max_tokens"`
	System     []TextBlock     `yaml:"system" json:"system"`
	Tools      []Decl          `yaml:"tools" json:"tools"`
	Headers    []Header        `yaml:"headers" json:"headers"`
	Thinking   json.RawMessage `yaml:"thinking" json:"thinking"`
	ToolChoice json.RawMessage `yaml:"tool_choice" json:"tool_choice"`
}

// TextBlock is one entry of the Anthropic-shape system[] array. The Text is
// stored byte-faithful from the capture (TIER-1).
type TextBlock struct {
	Type string `yaml:"type" json:"type"`
	Text string `yaml:"text" json:"text"`
}

// Decl is a model-facing tool declaration. InputSchema is the verbatim captured
// JSON schema; the profile-declared schema is authoritative at runtime (TOOL-02).
type Decl struct {
	Name        string          `yaml:"name" json:"name"`
	Description string          `yaml:"description" json:"description"`
	InputSchema json.RawMessage `yaml:"input_schema" json:"input_schema"`
}

// Header is one identity header. Name is byte-faithful (TIER-2 structural);
// ValueTemplate carries a placeholder rendered per-request (real values are
// per-session ephemera and never stored verbatim — T-01-02).
type Header struct {
	Name          string `yaml:"name" json:"name"`
	ValueTemplate string `yaml:"value_template" json:"value_template"`
}
