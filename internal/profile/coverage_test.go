package profile_test

import (
	"testing"

	"github.com/djarvur/ass-guard-agent/internal/profile"
)

// TestCheckCoverage_Tier1Drift confirms a TIER-1 count mismatch surfaces as a
// diff with ok=false (PROF-05 incomplete-capture gate).
func TestCheckCoverage_Tier1Drift(t *testing.T) {
	manifest := profile.CoverageManifest{
		Fields: []profile.CoverageEntry{
			{Path: "request.body.tools", Tier: profile.Tier1ByteFaithful, ObservedCount: 103},
			{Path: "request.body.system", Tier: profile.Tier1ByteFaithful, ObservedCount: 3},
		},
	}
	fresh := map[string]int{
		"request.body.tools":  102, // drifted
		"request.body.system": 3,
	}
	diffs, ok := profile.CheckCoverage(manifest, fresh)
	if ok {
		t.Fatal("ok = true; want false (tools drifted)")
	}
	if len(diffs) != 1 {
		t.Fatalf("len(diffs) = %d, want 1", len(diffs))
	}
	if diffs[0].Path != "request.body.tools" {
		t.Errorf("diff path = %q, want request.body.tools", diffs[0].Path)
	}
}

// TestCheckCoverage_Tier3Ignored confirms TIER-3 variance does NOT surface as a
// drift (timestamps/token counts are audit-only — D-06).
func TestCheckCoverage_Tier3Ignored(t *testing.T) {
	manifest := profile.CoverageManifest{
		Fields: []profile.CoverageEntry{
			{Path: "request.body.tools", Tier: profile.Tier1ByteFaithful, ObservedCount: 103},
			{Path: "response.usage.totalTokens", Tier: profile.Tier3Informational, ObservedCount: 5000},
		},
	}
	fresh := map[string]int{
		"request.body.tools":         103,
		"response.usage.totalTokens": 9999, // TIER-3 variance — must NOT count
	}
	diffs, ok := profile.CheckCoverage(manifest, fresh)
	if !ok {
		t.Fatalf("ok = false; want true (only TIER-3 differs): diffs=%+v", diffs)
	}
	if len(diffs) != 0 {
		t.Errorf("len(diffs) = %d, want 0 (TIER-3 ignored)", len(diffs))
	}
}

// TestCheckCoverage_MissingField confirms a manifest field absent from the
// capture surfaces as a diff with Missing=true.
func TestCheckCoverage_MissingField(t *testing.T) {
	manifest := profile.CoverageManifest{
		Fields: []profile.CoverageEntry{
			{Path: "request.headers", Tier: profile.Tier2Structural, ObservedCount: 12},
		},
	}
	diffs, ok := profile.CheckCoverage(manifest, map[string]int{})
	if ok {
		t.Fatal("ok = true; want false (headers missing)")
	}
	if !diffs[0].Missing {
		t.Error("diff.Missing = false; want true")
	}
}

// TestTier_DriftFlagged pins the D-06 tier-to-flag mapping.
func TestTier_DriftFlagged(t *testing.T) {
	if !profile.Tier1ByteFaithful.DriftFlagged() {
		t.Error("Tier1 should be drift-flagged")
	}
	if !profile.Tier2Structural.DriftFlagged() {
		t.Error("Tier2 should be drift-flagged")
	}
	if profile.Tier3Informational.DriftFlagged() {
		t.Error("Tier3 should NOT be drift-flagged")
	}
}

// TestCoverageManifest_Validate is the PROF-05 gate used by the extractor.
func TestCoverageManifest_Validate(t *testing.T) {
	m := profile.CoverageManifest{
		Fields: []profile.CoverageEntry{
			{Path: "request.body.tools", Tier: profile.Tier1ByteFaithful, ObservedCount: 103},
		},
	}
	if err := m.Validate(map[string]int{"request.body.tools": 103}); err != nil {
		t.Errorf("matching capture failed Validate: %v", err)
	}
	if err := m.Validate(map[string]int{"request.body.tools": 50}); err == nil {
		t.Error("drifted capture passed Validate; want error")
	}
}
