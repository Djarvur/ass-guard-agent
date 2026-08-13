package toolcat

import (
	"encoding/json"
	"fmt"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// Adapter is the schema-adapter layer (TOOL-02). It enforces the invariant that
// the profile-declared schema is AUTHORITATIVE at runtime: the Shaper pulls
// model-facing schemas through this adapter (from the profile), NOT from the
// catalog. The catalog provides execution behavior; the profile provides what
// the model sees.
type Adapter struct{}

// NewAdapter returns an Adapter.
func NewAdapter() *Adapter { return &Adapter{} }

// ModelFacingSchemas returns the profile-declared schemas as model-facing Decls.
// The profile is authoritative — these are exactly what the Shaper passes to the
// SDK. The catalog is not consulted here.
func (a *Adapter) ModelFacingSchemas(profileDecls []profile.Decl) []Decl {
	out := make([]Decl, 0, len(profileDecls))
	for _, d := range profileDecls {
		out = append(out, Decl{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema})
	}

	return out
}

// ResolveCall validates a returned tool-call's input against the profile-declared
// schema for the named tool (Phase 1: parse-only — execution is stubbed, D-15).
// On successful parse it returns the input unchanged. A missing declaration or a
// malformed JSON input returns an error; structural schema validation fires in
// Phase 2/4.
func (a *Adapter) ResolveCall(
	name string, input json.RawMessage, profileDecls []profile.Decl,
) (json.RawMessage, error) {
	decl := findDecl(profileDecls, name)
	if decl == nil {
		return nil, fmt.Errorf("adapter: tool %q not declared in the profile", name) //nolint:err113 // dynamic error message
	}
	// Phase 1: parse-only check. The input must be valid JSON against the schema's
	// declared type (object). Full JSON-schema validation is Phase 2/4.
	var probe any

	err := json.Unmarshal(input, &probe)
	if err != nil {
		return nil, fmt.Errorf("adapter: tool %q input is not valid JSON: %w", name, err)
	}

	_ = decl

	return input, nil
}

func findDecl(decls []profile.Decl, name string) *profile.Decl {
	for i := range decls {
		if decls[i].Name == name {
			return &decls[i]
		}
	}

	return nil
}
