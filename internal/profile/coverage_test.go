package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// TestCheckCoverage_Tier1Drift confirms a TIER-1 count mismatch surfaces as a
// diff with ok=false (PROF-05 incomplete-capture gate).
func TestCheckCoverage_Tier1Drift(t *testing.T) {
	t.Parallel()

	manifest := profile.CoverageManifest{
		Fields: []profile.CoverageEntry{
			{Path: covRequestBodyTools, Tier: profile.Tier1ByteFaithful, ObservedCount: 103},
			{Path: "request.body.system", Tier: profile.Tier1ByteFaithful, ObservedCount: 3},
		},
	}
	fresh := map[string]int{
		covRequestBodyTools:   102, // drifted
		"request.body.system": 3,
	}

	diffs, ok := profile.CheckCoverage(&manifest, fresh)
	if ok {
		t.Fatal("ok = true; want false (tools drifted)")
	}

	if len(diffs) != 1 {
		t.Fatalf("len(diffs) = %d, want 1", len(diffs))
	}

	if diffs[0].Path != covRequestBodyTools {
		t.Errorf("diff path = %q, want request.body.tools", diffs[0].Path)
	}
}

// TestCheckCoverage_Tier3Ignored confirms TIER-3 variance does NOT surface as a
// drift (timestamps/token counts are audit-only — D-06).
func TestCheckCoverage_Tier3Ignored(t *testing.T) {
	t.Parallel()

	manifest := profile.CoverageManifest{
		Fields: []profile.CoverageEntry{
			{Path: covRequestBodyTools, Tier: profile.Tier1ByteFaithful, ObservedCount: 103},
			{Path: "response.usage.totalTokens", Tier: profile.Tier3Informational, ObservedCount: 5000},
		},
	}
	fresh := map[string]int{
		covRequestBodyTools:          103,
		"response.usage.totalTokens": 9999, // TIER-3 variance — must NOT count
	}

	diffs, ok := profile.CheckCoverage(&manifest, fresh)
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
	t.Parallel()

	manifest := profile.CoverageManifest{
		Fields: []profile.CoverageEntry{
			{Path: "request.headers", Tier: profile.Tier2Structural, ObservedCount: 12},
		},
	}

	diffs, ok := profile.CheckCoverage(&manifest, map[string]int{})
	if ok {
		t.Fatal("ok = true; want false (headers missing)")
	}

	if !diffs[0].Missing {
		t.Error("diff.Missing = false; want true")
	}
}

// TestTier_DriftFlagged pins the D-06 tier-to-flag mapping.
func TestTier_DriftFlagged(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	m := profile.CoverageManifest{
		Fields: []profile.CoverageEntry{
			{Path: covRequestBodyTools, Tier: profile.Tier1ByteFaithful, ObservedCount: 103},
		},
	}

	err := m.Validate(map[string]int{covRequestBodyTools: 103})
	if err != nil {
		t.Errorf("matching capture failed Validate: %v", err)
	}

	err = m.Validate(map[string]int{covRequestBodyTools: 50})
	if err == nil {
		t.Error("drifted capture passed Validate; want error")
	}
}

// TestCoverageZcodeVersionRoundTrip (09-03 T2, Pitfall 18): the capture-time
// zcode version round-trips through the manifest yaml.
func TestCoverageZcodeVersionRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "coverage.yaml")

	yamlSrc := "profile: zcode\n" +
		"target_capture_ref:\n" +
		"  sessions:\n    - id: sess-x\n      role: main\n" +
		"  zcode_version: zcode 1.2.3\n" +
		"  extractor_version: extract-profile/01-02\n"

	err := os.WriteFile(path, []byte(yamlSrc), 0o600)
	if err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	m, err := profile.LoadCoverage(path)
	if err != nil {
		t.Fatalf("LoadCoverage: %v", err)
	}

	if m.TargetCaptureRef.ZcodeVersion != "zcode 1.2.3" {
		t.Errorf("ZcodeVersion = %q; want zcode 1.2.3 (round-trip)", m.TargetCaptureRef.ZcodeVersion)
	}
}
