// Package shaper test: runtime cwd composition of system blocks (the 08-09
// profile-fidelity finding — the E2E's explore leg read the REAL repo because
// the composed system prompt statically embedded the CAPTURED cwd).
package shaper_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// shapedSystemTexts shapes one trivial turn and returns the serialized system
// block texts of the outgoing request.
func shapedSystemTexts(t *testing.T, prof *profile.Profile) []string {
	t.Helper()

	params, _, err := shaper.New().Shape(prof, []shaper.Message{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatalf("shape: %v", err)
	}

	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got struct {
		System []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"system"`
	}

	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	out := make([]string, 0, len(got.System))
	for _, b := range got.System {
		out = append(out, b.Text)
	}

	return out
}

// TestComposeRuntimeWorkDir_SessionCwdInComposedBlock pins the capture-faithful
// form: zcode composes its env block with the RUNTIME cwd, so the composed
// system block must carry the SESSION's working directory — never the captured
// repo's (the 08-09 E2E explore leg read the real repo instead of the scratch
// because the static capture said it was standing in the real repo).
func TestComposeRuntimeWorkDir_SessionCwdInComposedBlock(t *testing.T) {
	t.Parallel()

	prof := loadProfileFromRoot(t, profilesRoot(t), profileZcode)

	if prof.CaptureWorkDir == "" {
		t.Fatal("zcode profile does not declare capture_work_dir (the composition seam is unseeded)")
	}

	capturedBefore := 0
	for _, b := range shapedSystemTexts(t, &prof) {
		if strings.Contains(b, prof.CaptureWorkDir) {
			capturedBefore++
		}
	}
	if capturedBefore == 0 {
		t.Fatal("precondition: no system block carries the captured cwd — the test would be vacuous")
	}

	const sessionDir = "/tmp/ass-guard-scratch-session"

	before := shapedSystemTexts(t, &prof)

	shaper.ComposeRuntimeWorkDir(&prof, sessionDir)

	after := shapedSystemTexts(t, &prof)

	if len(after) != len(before) {
		t.Fatalf("composition changed the block count: %d → %d", len(before), len(after))
	}

	sawSession, sawCaptured := false, false
	for i, b := range after {
		if strings.Contains(b, sessionDir) {
			sawSession = true
		}

		if strings.Contains(b, prof.CaptureWorkDir) {
			sawCaptured = true
		}

		// Form identity: the ONLY difference vs. the uncomposed block is the
		// cwd value itself.
		want := strings.ReplaceAll(before[i], prof.CaptureWorkDir, sessionDir)
		if b != want {
			t.Errorf("block %d differs beyond the cwd substitution (form not identical to the capture)", i)
		}
	}

	if !sawSession {
		t.Error("no composed system block carries the session cwd — the model still does not know where it stands")
	}

	if sawCaptured {
		t.Error("a composed system block still carries the CAPTURED cwd (the static-cwd bug)")
	}
}

// TestComposeRuntimeWorkDir_NoopWithoutDeclaration pins the guard behavior: a
// profile without capture_work_dir (or with a session cwd equal to the capture)
// composes byte-identically — the TIER-1 byte-fidelity default stands when no
// runtime composition is declared or needed.
func TestComposeRuntimeWorkDir_NoopWithoutDeclaration(t *testing.T) {
	t.Parallel()

	prof := loadProfileFromRoot(t, profilesRoot(t), "synthetic")

	prof.CaptureWorkDir = "" // undeclared

	before := shapedSystemTexts(t, &prof)

	shaper.ComposeRuntimeWorkDir(&prof, "/tmp/elsewhere")

	after := shapedSystemTexts(t, &prof)

	for i := range before {
		if before[i] != after[i] {
			t.Errorf("block %d changed without a capture_work_dir declaration (TIER-1 byte fidelity violated)", i)
		}
	}

	// Same-dir session: nothing to substitute.
	real := loadProfileFromRoot(t, profilesRoot(t), profileZcode)

	same := shapedSystemTexts(t, &real)

	shaper.ComposeRuntimeWorkDir(&real, real.CaptureWorkDir)

	for i, b := range shapedSystemTexts(t, &real) {
		if same[i] != b {
			t.Errorf("block %d changed when session cwd == captured cwd (no-op case violated)", i)
		}
	}
}
