package profile_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// rolloutDir resolves the zcode rollout directory; empty if absent.
func rolloutDir(t *testing.T) string {
	t.Helper()

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	dir := filepath.Join(home, ".zcode", "cli", "rollout")

	_, err = os.Stat(dir)
	if err != nil {
		return ""
	}

	return dir
}

// TestStability_WithinSessionExtractionSource is the HARD within-session
// stability assertion on the profile's extraction source: every full-request
// line in the session must agree on system block count, the tool-NAME set, and
// the 12 identity header names (RESEARCH §1.3). This is the load-bearing check
// — the profile is extracted from this session, so its shape must be stable
// across the session's turns.
//
// 09-03 (AUD-05 / Pitfall 17): the subject is the session PINNED in the
// shipped coverage manifest — NEVER richest-session selection (richness-biased
// picking yields a vacuous pass: a homogeneous session proves nothing) and
// NEVER a wholesale dir scan (mixed zcode builds make sessions
// incomparable). The test skips cleanly while the pinned capture is absent;
// docs/recapture-runbook.md is the re-grounding procedure.
func TestStability_WithinSessionExtractionSource(t *testing.T) {
	t.Parallel()

	manifest, lerr := profile.LoadCoverage(filepath.Join("..", "..", "profiles", "zcode", "coverage.yaml"))
	if lerr != nil {
		t.Skipf("shipped coverage manifest unavailable (repo layout?): %v", lerr)
	}

	if len(manifest.TargetCaptureRef.Sessions) == 0 {
		t.Skip("coverage manifest pins no capture session (run docs/recapture-runbook.md)")
	}

	pinned := manifest.TargetCaptureRef.Sessions[0]

	dir := rolloutDir(t)
	if dir == "" {
		t.Skipf("rollout dir unavailable — pinned %s unchecked (see docs/recapture-runbook.md)", pinned.ID)
	}

	path := filepath.Join(dir, "model-io-sess_"+pinned.ID+".jsonl")

	_, statErr := os.Stat(path)
	if statErr != nil {
		t.Skipf("pinned session %s absent — re-ground per docs/recapture-runbook.md", pinned.ID)
	}

	_, xerr := profile.ExtractFromRollout(path)
	if xerr != nil {
		t.Fatalf("within-session stability failed for the pinned session %s: %v", pinned.ID, xerr)
	}
}

// TestStability_CrossSessionHeaderNames is the cross-session TIER-2 stability
// check: the 12 identity header NAMES must be identical across all sessions
// (they are the zcode identity fingerprint — VERIFIED-FACTS.md item #1). Main
// and subagent sessions both carry the same 12 names.
//
// NOTE (D-16 finding): the system-block COUNT and the built-in tool SET differ
// across ROLES (main: 3 blocks + 19 core tools; subagent: 4 blocks + 13 core
// tools incl. RespondToCoordinator). Those differences are EXPECTED role-driven
// variance, NOT drift — the cross-session test asserts ONLY the header-name
// invariant (the part that is stable across roles).
//
// 09-03: this test KEEPS its directory scan deliberately — its invariant (the
// 12 header names across roles) is version-insensitive, and Pitfall 17's
// selection-bias critique targets the extraction-SOURCE selection (the
// within-session test above), not this cross-role fingerprint check.
func TestStability_CrossSessionHeaderNames(t *testing.T) {
	t.Parallel()

	dir := rolloutDir(t)
	if dir == "" {
		t.Skip("rollout dir unavailable")
	}

	stats, err := profile.ScanRolloutDir(dir)
	if err != nil {
		t.Skipf("scan: %v", err)
	}

	if len(stats) < 2 {
		t.Skip("need >=2 sessions for cross-session comparison")
	}

	want := map[string]bool{}

	for _, s := range stats {
		if s.FullRequestLines == 0 {
			continue
		}

		got, err := profile.ExtractFromRollout(s.Path)
		if err != nil {
			continue
		}

		names := make([]string, 0, len(got.Headers))
		for _, h := range got.Headers {
			names = append(names, h.Name)
		}

		sort.Strings(names)

		if len(want) == 0 {
			// first comparable session sets the expected set
			for _, n := range names {
				want[n] = true
			}
		}

		for _, n := range names {
			if !want[n] {
				t.Errorf("session %s: header %q not in the shared 12-name set", s.ID, n)
			}
		}

		if len(names) != len(want) {
			t.Errorf("session %s: %d header names, expected %d (role-driven variance is "+
				"informational, but the identity set must be the 12)",
				s.ID, len(names), len(want))
		}
	}
}

// --- 09-03 T1: pinned-session consumption + the divergence canary (AUD-05) ---

// TestStability_CanaryDetectsDivergence (Pitfall 17): a synthetic session
// whose tool catalog CHANGES mid-session MUST fail ExtractFromRollout — the
// canary proving the stability assertion can fail, so a green run on the real
// pinned session means something (non-vacuity).
func TestStability_CanaryDetectsDivergence(t *testing.T) {
	t.Parallel()

	path := filepath.Join(".", "testdata", "divergent-session", "model-io-sess_testdivergent.jsonl")

	_, err := profile.ExtractFromRollout(path)
	if err == nil {
		t.Fatal("ExtractFromRollout accepted a mid-session catalog change — the canary is broken (vacuous stability)")
	}
}

// TestStability_HomogeneousPasses: a two-line session with the SAME catalog
// extracts cleanly — the canary fires ONLY on real divergence.
func TestStability_HomogeneousPasses(t *testing.T) {
	t.Parallel()

	path := filepath.Join(".", "testdata", "divergent-session", "model-io-sess_testhomogeneous.jsonl")

	_, err := profile.ExtractFromRollout(path)
	if err != nil {
		t.Fatalf("ExtractFromRollout rejected a homogeneous session: %v", err)
	}
}
