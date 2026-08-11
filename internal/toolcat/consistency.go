package toolcat

import (
	"encoding/json"
	"strings"

	"github.com/djarvur/ass-guard-agent/internal/profile"
)

// ConsistencyResult is the TOOL-03 catalog-consistency outcome. Built-in tools
// whose schema drifted are Unsatisfied (a HARD failure — the catalog cannot
// satisfy what the profile declares). MCP/plugin tools absent from the built-in
// catalog are MCPOrPlugin (INFORMATIONAL in Phase 1 — their execution lands via
// Phase-5 ECOS-01; they are satisfied by definition of the catalog's scope).
type ConsistencyResult struct {
	// Unsatisfied: built-in tools whose profile-declared required-field set
	// differs from the catalog's (a drift the CI gate rejects).
	Unsatisfied []string
	// MCPOrPlugin: profile-declared tools that are not built-in (mcp__*/plugin);
	// expected to be unsatisfied by the built-in catalog — informational.
	MCPOrPlugin []string
	// Satisfied: built-in tools the catalog schema-structural-matches.
	Satisfied []string
}

// CheckConsistency compares the profile-declared tool schemas against the
// built-in catalog (TOOL-03). For each built-in tool, the profile-declared
// schema's required-field set must structurally match the catalog's; a mismatch
// is a hard failure (Unsatisfied). MCP/plugin tools are reported in
// MCPOrPlugin (informational). ok is true iff Unsatisfied is empty.
func CheckConsistency(catalog *Catalog, profileDecls []profile.Decl) (result ConsistencyResult, err error) {
	for _, d := range profileDecls {
		if isMCPOrPlugin(d.Name) {
			result.MCPOrPlugin = append(result.MCPOrPlugin, d.Name)
			continue
		}
		cat, ok := catalog.Get(d.Name)
		if !ok {
			// A non-MCP/plugin tool not in the catalog: report as unsatisfied
			// (could be a newly-added built-in the catalog hasn't picked up).
			result.Unsatisfied = append(result.Unsatisfied, d.Name)
			continue
		}
		if !structurallyCompatible(d.InputSchema, cat.InputSchema) {
			result.Unsatisfied = append(result.Unsatisfied, d.Name)
			continue
		}
		result.Satisfied = append(result.Satisfied, d.Name)
	}
	return result, nil
}

// Ok reports whether the consistency gate passes (no built-in schema drift).
func (r ConsistencyResult) Ok() bool { return len(r.Unsatisfied) == 0 }

// isMCPOrPlugin classifies a tool name as external (MCP/plugin). These are
// expected to be absent from the built-in catalog — they are INFORMATIONAL, not
// hard failures (Phase-5 ECOS-01 hosts them).
func isMCPOrPlugin(name string) bool {
	return strings.HasPrefix(name, "mcp__") || strings.Contains(name, "__")
}

// structurallyCompatible checks the required-field set of two JSON schemas for
// equality. MIMC-04: structural, not byte-equal — the catalog and profile may
// differ in description text or property ordering, but the required fields
// (which constrain what the model emits) must agree.
func structurallyCompatible(a, b json.RawMessage) bool {
	ra := requiredFields(a)
	rb := requiredFields(b)
	if len(ra) != len(rb) {
		return false
	}
	set := make(map[string]struct{}, len(ra))
	for _, f := range ra {
		set[f] = struct{}{}
	}
	for _, f := range rb {
		if _, ok := set[f]; !ok {
			return false
		}
	}
	return true
}

func requiredFields(raw json.RawMessage) []string {
	var s struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	return s.Required
}
