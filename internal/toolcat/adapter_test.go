package toolcat_test

import (
	"encoding/json"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// TestAdapter_ProfileAuthoritative proves the adapter returns the PROFILE's
// schemas, not the catalog's. A synthetic profile with a custom Read schema must
// yield the synthetic schema (TOOL-02 — profile is authoritative).
func TestAdapter_ProfileAuthoritative(t *testing.T) {
	a := toolcat.NewAdapter()
	customSchema := json.RawMessage(`{"type":"object","properties":{"custom":{"type":"string"}},"required":["custom"]}`)
	decls := []profile.Decl{
		{Name: "Read", Description: "synthetic read", InputSchema: customSchema},
	}

	out := a.ModelFacingSchemas(decls)
	if len(out) != 1 || out[0].Name != "Read" {
		t.Fatalf("got %+v", out)
	}
	// The returned schema must be the PROFILE's (synthetic), byte-identical.
	if string(out[0].InputSchema) != string(customSchema) {
		t.Errorf("adapter returned a different schema than the profile declared (TOOL-02 violation)")
	}
}

// TestAdapter_ResolveCallParsesInput confirms ResolveCall accepts valid JSON
// input for a declared tool and rejects undeclared/invalid inputs.
func TestAdapter_ResolveCallParsesInput(t *testing.T) {
	a := toolcat.NewAdapter()
	decls := []profile.Decl{
		{Name: "Read", InputSchema: json.RawMessage(`{"type":"object","required":["file_path"]}`)},
	}
	in := json.RawMessage(`{"file_path":"go.mod"}`)

	out, err := a.ResolveCall("Read", in, decls)
	if err != nil {
		t.Fatalf("ResolveCall Read: %v", err)
	}

	if string(out) != string(in) {
		t.Errorf("ResolveCall returned %s, want the input unchanged", out)
	}

	if _, err := a.ResolveCall("Write", in, decls); err == nil {
		t.Error("ResolveCall Write (undeclared) returned nil error; want non-nil")
	}

	if _, err := a.ResolveCall("Read", json.RawMessage(`{bad`), decls); err == nil {
		t.Error("ResolveCall accepted malformed JSON; want non-nil error")
	}
}
