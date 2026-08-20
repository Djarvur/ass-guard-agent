package evalharness_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/evalharness"
)

// The GuardOpenSpecGlobalConfig invariant battery (13-01 Task 2, T-13-01-01):
// the guard writes OUTSIDE the repo (the operator's global openspec config —
// HOME-resolved ~/.config/openspec/config.json), so its restore contract is
// pinned byte-exactly, OFFLINE, under a fake HOME:
//
//   - absent-file case: restore = remove (the file must not appear for the
//     operator just because a gated run happened);
//   - existing-arbitrary-bytes case: restore = the EXACT original bytes
//     (never a synthesized "core" profile, never key-drift);
//   - the expanded write actually lands while the guard is active
//     (read-back equals ExpandedMatrixProfileJSON).
//
// The gated matrix legs add a SECOND, in-runner pre/post byte-compare on the
// operator's REAL config (registered before the guard's restore so LIFO runs
// it after) plus the verify command's pre/post shasum equality — measurement,
// never goodwill.
//
// The guard runs inside a subtest so its t.Cleanup (the restore) fires at the
// subtest's end — the parent then asserts the POST-RESTORE state.

// TestGuardOpenSpecGlobalConfig_AbsentFileRestoresAbsent: no pre-existing
// global config => after the guard's cleanup the file is ABSENT again.
func TestGuardOpenSpecGlobalConfig_AbsentFileRestoresAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath := filepath.Join(home, ".config", "openspec", "config.json")

	_, err := os.Stat(cfgPath)
	if !os.IsNotExist(err) {
		t.Fatalf("precondition: fake HOME must start without a global config (%v)", err)
	}

	t.Run("guard", func(t *testing.T) {
		evalharness.GuardOpenSpecGlobalConfig(t, evalharness.ExpandedMatrixProfileJSON)
	})

	_, err = os.Stat(cfgPath)
	if !os.IsNotExist(err) {
		t.Errorf("global config leaked into a previously-absent HOME: stat err = %v; want not-exist", err)
	}
}

// TestGuardOpenSpecGlobalConfig_ExistingBytesRestoreIdentical: arbitrary
// pre-existing bytes (incl. telemetry keys, no profile key) restore EXACTLY —
// byte-compare, never a synthesized "core".
func TestGuardOpenSpecGlobalConfig_ExistingBytesRestoreIdentical(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgDir := filepath.Join(home, ".config", "openspec")

	err := os.MkdirAll(cfgDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir fake config dir: %v", err)
	}

	// Deliberately NOT the expanded profile and NOT a "core" synthesis: a
	// config with foreign keys (telemetry etc.) and no profile key at all.
	original := []byte("{\n  \"featureFlags\": {},\n  \"telemetry\": {\"anonymousId\": \"probe\"}\n}\n")

	cfgPath := filepath.Join(cfgDir, "config.json")

	err = os.WriteFile(cfgPath, original, 0o600)
	if err != nil {
		t.Fatalf("seed original config: %v", err)
	}

	t.Run("guard", func(t *testing.T) {
		evalharness.GuardOpenSpecGlobalConfig(t, evalharness.ExpandedMatrixProfileJSON)
	})

	restored, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read restored config: %v", err)
	}

	if !bytes.Equal(restored, original) {
		t.Errorf("restore not byte-identical:\n got: %q\nwant: %q", restored, original)
	}
}

// TestGuardOpenSpecGlobalConfig_ExpandedWriteLands: while the guard is active
// the global config IS the expanded matrix profile (the switch the matrix
// bootstrap needs before `openspec init`).
func TestGuardOpenSpecGlobalConfig_ExpandedWriteLands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("guard", func(t *testing.T) {
		evalharness.GuardOpenSpecGlobalConfig(t, evalharness.ExpandedMatrixProfileJSON)

		cfgPath := filepath.Join(home, ".config", "openspec", "config.json")

		got, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("read active config: %v", err)
		}

		if !bytes.Equal(got, evalharness.ExpandedMatrixProfileJSON) {
			t.Errorf("active config = %q; want ExpandedMatrixProfileJSON verbatim", got)
		}
	})
}

// TestExpandedMatrixProfileJSONShape: the pinned profile object — profile
// "custom" (the preset enum is core|custom ONLY on 1.5.0; the workflows array
// is the switch) carrying ALL 11 workflows, so init installs the 6 expanded
// commands. Planning probe 2026-08-19 (/tmp/opsx-custom2; dist/core/profiles.js
// + config-schema.js).
func TestExpandedMatrixProfileJSONShape(t *testing.T) { //nolint:paralleltest // same file as Setenv siblings
	want := `{
  "profile": "custom",
  "workflows": [
    "explore",
    "propose",
    "apply",
    "sync",
    "archive",
    "new",
    "continue",
    "ff",
    "verify",
    "bulk-archive",
    "onboard"
  ]
}`

	if !bytes.Equal(evalharness.ExpandedMatrixProfileJSON, []byte(want)) {
		t.Errorf("ExpandedMatrixProfileJSON =\n%s\nwant\n%s", evalharness.ExpandedMatrixProfileJSON, want)
	}
}
