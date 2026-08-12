// Package toolcat implements the built-in tool catalog (TOOL-01), the
// schema-adapter layer (TOOL-02 — the profile-declared schema is authoritative
// at runtime), and the catalog-consistency CI check (TOOL-03).
//
// "What the model sees" (profile-declared schema) and "what the tool does"
// (catalog execution behavior) are cleanly separable. In Phase 1 tool execution
// is stubbed across the board (D-15); only schemas + the dispatch skeleton ship.
package toolcat

import (
	"context"
	"encoding/json"
	"fmt"
)

// Mutability classifies a tool's execution side-effect (a Phase-2/4 concern).
// Recorded in Phase 1 even though execution is stubbed.
type Mutability int

const (
	// MutabilityReadOnly is the default for tools with no side effects.
	MutabilityReadOnly Mutability = iota
	// MutabilityMutating marks tools that change state (Bash, Write, Edit).
	MutabilityMutating
)

// String returns the catalog's canonical mutability label.
func (m Mutability) String() string {
	switch m {
	case MutabilityMutating:
		return classMutating
	default:
		return "read-only"
	}
}

// UnmarshalJSON accepts the catalog's string form ("mutating"/"read-only") so
// the embedded coretools.json (which stores mutability as a human-readable
// string) deserializes into the typed Mutability enum.
func (m *Mutability) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case `"mutating"`:
		*m = MutabilityMutating
	case `"read-only"`, `""`, `null`:
		*m = MutabilityReadOnly
	default:
		return fmt.Errorf("toolcat: unknown mutability %s", b)
	}

	return nil
}

// Decl is a model-facing tool declaration (mirrors profile.Decl). The adapter
// returns these to the Shaper — the profile-declared schema is authoritative
// (TOOL-02), not the catalog's.
type Decl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Stub is the tool execution signature (forward-compatible to Phase 2/4).
// Phase-1 catalog entries leave Execute nil; the dispatch skeleton returns a
// canned "stubbed" result when called.
type Stub func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// Tool is one catalog entry: the model-facing schema + the execution behavior.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Mutability  Mutability      `json:"mutability"`
	// Execute is the tool's runtime behavior (nil/stubbed in Phase 1 — D-15).
	Execute Stub `json:"-"`
}

// IsMutating reports whether the tool has side effects.
func (t Tool) IsMutating() bool { return t.Mutability == MutabilityMutating }
