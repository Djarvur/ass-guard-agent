package profile_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// TestExtractFromRollout_GoodSession parses a synthetic JSONL fixture with two
// stable full-request lines and confirms the extracted shape + within-session
// stability check.
func TestExtractFromRollout_GoodSession(t *testing.T) {
	t.Parallel()

	path := filepath.Join(".", "testdata", "sample-sessions", "good.jsonl")

	res, err := profile.ExtractFromRollout(path)
	if err != nil {
		t.Fatalf("ExtractFromRollout: %v", err)
	}

	if len(res.System) != 2 {
		t.Errorf("len(System) = %d, want 2", len(res.System))
	}

	if len(res.Tools) != 2 {
		t.Errorf("len(Tools) = %d, want 2 (null-named filtered)", len(res.Tools))
	}

	for _, d := range res.Tools {
		if d.Name == "" {
			t.Error("a null/empty-named tool was not filtered")
		}
	}

	if len(res.Headers) != 2 {
		t.Errorf("len(Headers) = %d, want 2", len(res.Headers))
	}

	if res.Model != "synth-model" {
		t.Errorf("Model = %q, want synth-model", res.Model)
	}

	if len(res.Thinking) == 0 || len(res.ToolChoice) == 0 {
		t.Error("Thinking/ToolChoice not extracted")
	}
}

// TestExtractFromRollout_AuxQuerySourceSkipped confirms that auxiliary subrequest
// records (querySource=web_search_tool — zcode's server-side web-search arm, one
// mini system block + one tool) are excluded from extraction and the stability
// check: they are a different request class, not drift (observed live in the
// 2026-08-16 re-capture, zcode 0.16.3).
func TestExtractFromRollout_AuxQuerySourceSkipped(t *testing.T) {
	t.Parallel()

	path := filepath.Join(".", "testdata", "sample-sessions", "aux-query.jsonl")

	res, err := profile.ExtractFromRollout(path)
	if err != nil {
		t.Fatalf("ExtractFromRollout: %v (aux query-source records must not trip stability)", err)
	}

	if len(res.System) != 2 {
		t.Errorf("len(System) = %d, want 2 (main_turn line)", len(res.System))
	}

	if len(res.Tools) != 2 {
		t.Errorf("len(Tools) = %d, want 2 (aux web_search tool excluded)", len(res.Tools))
	}
}

// TestExtractFromRollout_MCPCatalogTransitionTolerated confirms that mid-session
// MCP attach/detach — an mcp__-prefixed-only tool-set delta — is treated as
// runtime catalog state, not drift: extraction succeeds and keeps the BASE
// catalog (the profile models the session's main-turn shape; connected MCP
// servers are per-session runtime additions). Observed live in the 2026-08-16
// re-capture (79 -> 80 with mcp__recapture_probe__recapture_probe -> 79).
func TestExtractFromRollout_MCPCatalogTransitionTolerated(t *testing.T) {
	t.Parallel()

	path := filepath.Join(".", "testdata", "sample-sessions", "mcp-transition.jsonl")

	res, err := profile.ExtractFromRollout(path)
	if err != nil {
		t.Fatalf("ExtractFromRollout: %v (mcp__-only catalog transitions must not trip stability)", err)
	}

	if len(res.Tools) != 2 {
		t.Errorf("len(Tools) = %d, want 2 (base catalog; the mcp__ tool is runtime state)", len(res.Tools))
	}

	for _, d := range res.Tools {
		if strings.HasPrefix(d.Name, "mcp__") {
			t.Errorf("mcp__ tool %q leaked into the extracted base catalog", d.Name)
		}
	}
}

// TestExtractFromRollout_NonMCPToolDriftStillFails confirms the tolerance is
// narrow: a NON-mcp__ tool appearing mid-session is still fatal drift.
func TestExtractFromRollout_NonMCPToolDriftStillFails(t *testing.T) {
	t.Parallel()

	path := filepath.Join(".", "testdata", "sample-sessions", "mcp-drift-nonmcp.jsonl")

	_, err := profile.ExtractFromRollout(path)
	if err == nil {
		t.Fatal("ExtractFromRollout returned nil for a non-mcp tool-set drift; want a stability error")
	}
}

// TestExtractFromRollout_DriftFails confirms the within-session stability check
// fires when two full-request lines disagree on the tool count.
func TestExtractFromRollout_DriftFails(t *testing.T) {
	t.Parallel()

	path := filepath.Join(".", "testdata", "sample-sessions", "drift.jsonl")

	_, err := profile.ExtractFromRollout(path)
	if err == nil {
		t.Fatal("ExtractFromRollout returned nil for a drift fixture; want a stability error")
	}
}

// TestExtractFromRollout_NoFullRequests confirms a session with no full-request
// lines surfaces a clear error.
func TestExtractFromRollout_NoFullRequests(t *testing.T) {
	t.Parallel()

	path := filepath.Join(".", "testdata", "sample-sessions", "no-full.jsonl")

	_, err := profile.ExtractFromRollout(path)
	if err == nil {
		t.Fatal("ExtractFromRollout returned nil for an empty/no-full fixture; want error")
	}
}
