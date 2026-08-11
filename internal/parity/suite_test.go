package parity_test

import (
	"path/filepath"
	"testing"

	"github.com/djarvur/ass-guard-agent/internal/parity"
)

// TestSuite_CuratedLoads confirms the shipped curated_suite.json parses and
// meets D-01's 5-15 bound with at least one zero-tool entry.
func TestSuite_CuratedLoads(t *testing.T) {
	path := filepath.Join(".", "suite", "curated_suite.json")
	turns, err := parity.LoadReplaySession(path)
	if err != nil {
		t.Fatalf("curated suite did not parse: %v", err)
	}
	if len(turns) < 5 || len(turns) > 15 {
		t.Errorf("curated suite has %d entries, want 5-15 (D-01)", len(turns))
	}
	hasZeroTool := false
	patterns := map[string]bool{}
	for _, trn := range turns {
		if trn.Prompt == "" || trn.DivergenceReason == "" {
			t.Errorf("turn %q missing prompt or divergence rationale", trn.TurnID)
		}
		if len(trn.ExpectedToolCalls) == 0 {
			hasZeroTool = true
		}
		if len(trn.ExpectedToolCalls) > 1 {
			patterns["multi-tool"] = true
		}
	}
	if !hasZeroTool {
		t.Error("no zero-tool entry; want at least one (zero-tool is a divergence pattern)")
	}
	if !patterns["multi-tool"] {
		t.Error("no multi-tool entry; want at least one")
	}
}
