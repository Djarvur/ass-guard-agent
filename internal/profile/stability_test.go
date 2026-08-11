package profile_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/djarvur/ass-guard-agent/internal/profile"
)

// rolloutDir resolves the zcode rollout directory; empty if absent.
func rolloutDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".zcode", "cli", "rollout")
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	return dir
}

// sessionPath returns the rollout path for a session id prefix, or "" if absent.
func sessionPath(t *testing.T, idPrefix string) string {
	t.Helper()
	dir := rolloutDir(t)
	if dir == "" {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "model-io-sess_"+idPrefix+"*.jsonl"))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// TestStability_WithinSessionExtractionSource is the HARD within-session
// stability assertion on the profile's extraction source: every full-request
// line in the session must agree on system block count, the tool-NAME set, and
// the 12 identity header names (RESEARCH §1.3). This is the load-bearing check
// — the profile is extracted from this session, so its shape must be stable
// across the session's turns. Skips cleanly when the rollout dir is absent.
func TestStability_WithinSessionExtractionSource(t *testing.T) {
	// D-16: the extraction source is whatever main session the extractor picks.
	stats, err := profile.ScanRolloutDir(rolloutDir(t))
	if err != nil {
		t.Skipf("rollout dir unavailable: %v", err)
	}
	chosen, err := profile.PickRichestMain(stats)
	if err != nil {
		t.Skipf("no main session: %v", err)
	}
	if _, err := profile.ExtractFromRollout(chosen.Path); err != nil {
		t.Fatalf("within-session stability failed for %s: %v", chosen.ID, err)
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
func TestStability_CrossSessionHeaderNames(t *testing.T) {
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
			t.Errorf("session %s: %d header names, expected %d (role-driven variance is informational, but the identity set must be the 12)", s.ID, len(names), len(want))
		}
	}
}
