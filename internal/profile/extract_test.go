package profile_test

import (
	"path/filepath"
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
