package drift_test

import (
	"encoding/json"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/drift"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// manifest builds a small CoverageManifest for the test cases.
func manifest(fields ...profile.CoverageEntry) profile.CoverageManifest {
	return profile.CoverageManifest{Fields: fields}
}

// TestDetect_NoDrift: a captured request matching the manifest → no drifts.
func TestDetect_NoDrift(t *testing.T) {
	t.Parallel()

	m := manifest(
		profile.CoverageEntry{Path: covRequestBodySystem, Tier: profile.Tier1ByteFaithful, ObservedCount: 3},
		profile.CoverageEntry{Path: covRequestHeaders, Tier: profile.Tier2Structural, ObservedCount: 12},
	)
	captured := map[string]any{
		covRequestBodySystem: []any{map[string]any{"type": "text"}, map[string]any{}, map[string]any{}}, // length 3
		covRequestHeaders: map[string]any{ // 12 names
			"h1": "x", "h2": "x", "h3": "x", "h4": "x", "h5": "x", "h6": "x",
			"h7": "x", "h8": "x", "h9": "x", "h10": "x", "h11": "x", "h12": "x",
		},
	}

	got := drift.Detect(&m, captured)
	if len(got) != 0 {
		t.Errorf("expected no drift, got %+v", got)
	}
}

// TestDetect_MissingTier2Header: a captured request missing a declared header
// set → one TIER-2 drift.
func TestDetect_MissingTier2Header(t *testing.T) {
	t.Parallel()

	m := manifest(
		profile.CoverageEntry{Path: covRequestHeaders, Tier: profile.Tier2Structural, ObservedCount: 12},
	)
	captured := map[string]any{
		covRequestHeaders: map[string]any{"User-Agent": "x"}, // 1, not 12
	}

	got := drift.Detect(&m, captured)
	if len(got) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(got))
	}

	if got[0].Tier != profile.Tier2Structural {
		t.Errorf("drift tier = %v, want TIER-2", got[0].Tier)
	}
}

// TestDetect_Tier3Ignored: token-count variance must NOT surface (D-06).
func TestDetect_Tier3Ignored(t *testing.T) {
	t.Parallel()

	m := manifest(
		profile.CoverageEntry{
			Path: "response.usage.totalTokens",
			Tier: profile.Tier3Informational, ObservedCount: 5000,
		},
	)
	captured := map[string]any{
		"response.usage.totalTokens": 9999,
	}

	got := drift.Detect(&m, captured)
	if len(got) != 0 {
		t.Errorf("TIER-3 variance surfaced as drift: %+v (must be audit-only)", got)
	}
}

// TestDetect_Tier2ValueVarianceNotFlagged: a different x-request-id VALUE (same
// header name present) must NOT be a drift — per-session value variance is
// expected (TIER-2 structural, not byte-value).
func TestDetect_Tier2ValueVarianceNotFlagged(t *testing.T) {
	t.Parallel()

	m := manifest(
		profile.CoverageEntry{Path: covRequestHeaders, Tier: profile.Tier2Structural, ObservedCount: 12},
	)
	// 12 header NAMES present (values differ from the manifest's snapshot — fine).
	headers := map[string]any{}
	for _, n := range []string{"HTTP-Referer", "User-Agent", "X-Os-Category", "X-Os-Version",
		"X-Platform", "X-Title", "X-ZCode-Agent", "X-ZCode-App-Version",
		"x-query-id", "x-request-id", "x-session-id", "x-zcode-trace-id"} {
		headers[n] = "different-value-per-session"
	}

	captured := map[string]any{covRequestHeaders: headers}

	got := drift.Detect(&m, captured)
	if len(got) != 0 {
		t.Errorf("TIER-2 value variance flagged as drift (only structure/presence matters): %+v", got)
	}
}

// TestDetect_ChangedTier1SystemBlock: a different system-block COUNT is a TIER-1 drift.
func TestDetect_ChangedTier1SystemBlock(t *testing.T) {
	t.Parallel()

	m := manifest(
		profile.CoverageEntry{Path: covRequestBodySystem, Tier: profile.Tier1ByteFaithful, ObservedCount: 3},
	)
	captured := map[string]any{
		covRequestBodySystem: []any{map[string]any{"type": "text"}}, // 1, not 3
	}

	got := drift.Detect(&m, captured)
	if len(got) != 1 || got[0].Tier != profile.Tier1ByteFaithful {
		t.Errorf("expected 1 TIER-1 drift, got %+v", got)
	}
}

// sanity: json import is used by future rich-capture cases (kept for clarity).
var _ = json.RawMessage{}
