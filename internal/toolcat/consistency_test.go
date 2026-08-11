package toolcat_test

import (
	"encoding/json"
	"testing"

	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/toolcat"
)

// TestCheckConsistency_OnlyBuiltinsSatisfied confirms a profile declaring only
// built-in core tools passes with empty Unsatisfied.
func TestCheckConsistency_OnlyBuiltinsSatisfied(t *testing.T) {
	c := toolcat.NewCatalog()
	// Pull the catalog's own Read schema so the required-field set matches exactly.
	readTool, _ := c.Get("Read")
	bashTool, _ := c.Get("Bash")
	decls := []profile.Decl{
		{Name: "Read", InputSchema: readTool.InputSchema},
		{Name: "Bash", InputSchema: bashTool.InputSchema},
	}
	res, err := toolcat.CheckConsistency(c, decls)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Ok() {
		t.Errorf("expected Ok, got Unsatisfied=%v", res.Unsatisfied)
	}
}

// TestCheckConsistency_DriftedSchemaIsHardFailure confirms a built-in tool whose
// profile-declared required-field set differs is Unsatisfied (TOOL-03 hard fail).
func TestCheckConsistency_DriftedSchemaIsHardFailure(t *testing.T) {
	c := toolcat.NewCatalog()
	readTool, _ := c.Get("Read")
	// Mutate the required-field set to force a structural mismatch.
	drifted := mutateRequired(t, readTool.InputSchema, []string{"totally_different_field"})
	decls := []profile.Decl{{Name: "Read", InputSchema: drifted}}
	res, _ := toolcat.CheckConsistency(c, decls)
	if res.Ok() {
		t.Fatal("expected Read with drifted required fields to be Unsatisfied; got Ok")
	}
	if len(res.Unsatisfied) != 1 || res.Unsatisfied[0] != "Read" {
		t.Errorf("Unsatisfied = %v, want [Read]", res.Unsatisfied)
	}
}

// TestCheckConsistency_MCPIsInformational confirms an MCP tool is reported in
// MCPOrPlugin (informational), NOT Unsatisfied.
func TestCheckConsistency_MCPIsInformational(t *testing.T) {
	c := toolcat.NewCatalog()
	decls := []profile.Decl{
		{Name: "mcp__firecrawl__search", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	res, _ := toolcat.CheckConsistency(c, decls)
	if len(res.MCPOrPlugin) != 1 {
		t.Errorf("MCPOrPlugin = %v, want [mcp__firecrawl__search]", res.MCPOrPlugin)
	}
	if !res.Ok() {
		t.Errorf("MCP-only profile should be Ok (informational), got Unsatisfied=%v", res.Unsatisfied)
	}
}

// mutateRequired rewrites the required-field set of a schema (helper for the
// drift test).
func mutateRequired(t *testing.T, schema json.RawMessage, required []string) json.RawMessage {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(schema, &m); err != nil {
		t.Fatal(err)
	}
	m["required"] = required
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
