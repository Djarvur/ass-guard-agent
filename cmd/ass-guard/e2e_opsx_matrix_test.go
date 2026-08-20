package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/evalharness"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// The expanded-matrix E2E (13-01, D-01/D-03/D-08): the 6 opt-in commands
// (new/continue/ff/verify/bulk-archive/onboard) driven through ass-guard
// against the real openspec binary + the real model, in scratches whose
// GLOBAL config carries the expanded profile — guarded byte-exactly around
// every run (GuardOpenSpecGlobalConfig + the in-runner pre/post compare).
//
// The bootstrap is the ONE truth: newOpsxMatrixRunner clones newOpsxRunner
// (same provider/engine/expansion wiring) EXCEPT it applies the profile guard
// BEFORE `openspec init` and FAILS LOUD when the 6 expanded command files did
// not install.

// matrixExpandedCommands are the 6 opt-in commands the expanded profile must
// install (the core 5 always install).
//
//nolint:gochecknoglobals // a fixed test fixture list
var matrixExpandedCommands = []string{
	"new", "continue", "ff", "verify", "bulk-archive", "onboard",
}

// newOpsxMatrixRunner bootstraps an EXPANDED-PROFILE scratch: the operator's
// global openspec config is snapshotted (bytes, or recorded absent) with a
// t.Cleanup byte-compare registered BEFORE the guard's restore (LIFO → the
// compare runs AFTER the restore, proving post-run identity on the operator's
// REAL config inside every gated leg), then GuardOpenSpecGlobalConfig writes
// the 11-workflow switch, and only then does `openspec init` run — so init
// installs all 11 commands + skills (D-01's precondition).
//
//nolint:gocritic // unnamed result matches the newOpsxRunner house shape
func newOpsxMatrixRunner(t *testing.T) (*sessionTurnRunner, string) {
	t.Helper()

	scratch := t.TempDir()

	return newOpsxMatrixRunnerAt(t, scratch), scratch
}

// newOpsxMatrixRunnerAt is newOpsxMatrixRunner over an existing scratch dir.
//
//nolint:funlen,cyclop // bootstrap reads as one flow
func newOpsxMatrixRunnerAt(t *testing.T, scratch string) *sessionTurnRunner {
	t.Helper()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve HOME: %v", err)
	}

	cfgPath := filepath.Join(home, ".config", "openspec", "config.json")

	preBytes, preErr := os.ReadFile(cfgPath)
	absent := os.IsNotExist(preErr)

	if preErr != nil && !absent {
		t.Fatalf("read the operator's global openspec config: %v", preErr)
	}

	// LIFO: registered FIRST → runs AFTER the guard's restore. The post-run
	// byte-identity is MEASURED, never assumed (T-13-01-01).
	t.Cleanup(func() {
		postBytes, postErr := os.ReadFile(cfgPath)
		switch {
		case absent:
			if !os.IsNotExist(postErr) {
				t.Errorf("the operator's global openspec config LEAKED into a previously-absent HOME")
			}
		case postErr != nil:
			t.Errorf("the operator's global openspec config vanished: %v", postErr)
		case !bytes.Equal(preBytes, postBytes):
			t.Errorf("the operator's global openspec config did not restore byte-identically")
		}
	})

	evalharness.GuardOpenSpecGlobalConfig(t, evalharness.ExpandedMatrixProfileJSON)

	repo := findRepoRoot(t)

	factory, providerName, ferr := setupProviderFactory(repo, os.Stderr)
	if ferr != nil {
		t.Fatalf("BLOCKER: provider factory (real model creds): %v", ferr)
	}

	if _, _, ok := factory.Endpoint(providerName); !ok {
		t.Fatalf("BLOCKER: provider %q has no credentialed endpoint — the E2E needs the real model", providerName)
	}

	initCmd := exec.CommandContext(context.Background(), "openspec", "init", "--tools", "claude", "--force")
	initCmd.Dir = scratch

	out, ierr := initCmd.CombinedOutput()
	if ierr != nil {
		t.Fatalf("BLOCKER: expanded-profile openspec init failed in scratch: %v\n%s", ierr, out)
	}

	// The wiring proof IS the assertion: the expanded profile installs the 6
	// opt-in commands + their skills (FAIL LOUD — a silently-core scratch
	// would dead-end every leg at a missing command).
	for _, cmd := range matrixExpandedCommands {
		p := filepath.Join(scratch, ".claude", "commands", "opsx", cmd+".md")
		if !fileExists(p) {
			t.Fatalf("expanded-profile init did NOT install /opsx:%s (missing %s) — "+
				"the profile switch/bootstrap failed", cmd, p)
		}
	}

	skillBase := filepath.Join(scratch, ".claude", "skills")

	for _, cmd := range matrixExpandedCommands {
		matches, _ := filepath.Glob(filepath.Join(skillBase, "openspec-*"+cmd+"*"))
		if len(matches) == 0 {
			t.Fatalf("expanded-profile init installed /opsx:%s without its skill "+
				"(no openspec-*%s* under %s)", cmd, cmd, skillBase)
		}
	}

	seedScratchCodebase(t, scratch)

	prof, perr := profile.NewLoader(filepath.Join(repo, "profiles")).Load(profileZcode)
	if perr != nil {
		t.Fatalf("load real zcode profile: %v", perr)
	}

	r := &sessionTurnRunner{
		bus:     event.NewBus(),
		profile: prof,
		workDir: scratch,
		maxConc: 6,
		// D-01's documented hands-off mode (the 12-08 finding): asks are real
		// since 12-01; a bounded ask timeout keeps the matrix legs hands-off
		// (and 13-00's engine-visible resume carries the chain through them).
		makeProvider: func(rc provider.RequestCapturer) provider.Provider {
			p, _ := factory.Build(providerName, shaper.New())

			return p
		},
		askTimeout: 45 * time.Second,
	}

	err = r.setupEngine()
	if err != nil {
		t.Fatalf("setupEngine: %v", err)
	}

	r.loadCommandRegistry()

	return r
}

// runMatrixStage sends one typed matrix prompt and waits for the turn (+
// any parked chain) to finish — the shared leg driver.
func runMatrixStage(t *testing.T, r *sessionTurnRunner, sessionID, text string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), e2eOverallWait)
	defer cancel()

	_, err := r.Run(ctx, sessionID, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: text}})
	if err != nil {
		t.Fatalf("matrix prompt %q: %v", text, err)
	}

	if !r.WaitChainIdle(ctx, sessionID) {
		t.Fatalf("matrix prompt %q: the engine chain did not go idle within the overall wait", text)
	}
}

// captureMatrixClosing appends one leg's closing output to
// testdata/opsx-e2e-matrix/<name> with the provenance header (D-12: capture
// files carry date + binary version + model — the cross-plan corpus contract).
func captureMatrixClosing(t *testing.T, name, text string) {
	t.Helper()

	dir := filepath.Join("testdata", "opsx-e2e-matrix")

	err := os.MkdirAll(dir, 0o750)
	if err != nil {
		t.Fatalf("mkdir testdata matrix: %v", err)
	}

	ver, verr := exec.CommandContext(context.Background(), "openspec", "--version").Output()
	if verr != nil {
		t.Fatalf("openspec --version: %v", verr)
	}

	body := fmt.Sprintf("# opsx matrix %s (captured %s, openspec %s, real binary + real model)\n\n%s\n",
		name, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(string(ver)), text)

	err = os.WriteFile(filepath.Join(dir, name+".txt"), []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write capture %s: %v", name, err)
	}

	t.Logf("captured %s to testdata/opsx-e2e-matrix/", name)
}

// assertNoNotImplementedResults scans the session transcript for the
// no-implementation dead-end string (zero tolerance — the Phase-8 convention).
func assertNoNotImplementedResults(t *testing.T, r *sessionTurnRunner, sessionID string) {
	t.Helper()

	lines, err := r.sessions[sessionID].Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	for i := range lines {
		isResult := lines[i].Type == session.TypeToolResult
		if isResult && strings.Contains(string(lines[i].Output), "no implementation yet") {
			t.Errorf("not-implemented tool result at line %d (call %s): %s",
				i, lines[i].ToolCallID, string(lines[i].Output))
		}
	}
}

// lastAssistantText returns the session's final assistant message text.
func lastAssistantText(t *testing.T, r *sessionTurnRunner, sessionID string) string {
	t.Helper()

	texts := stageAssistantTexts(t, r, sessionID)
	if len(texts) == 0 {
		t.Fatal("no assistant message in the transcript")
	}

	return texts[len(texts)-1]
}

// TestOpsxMatrixNew_Gated (13-01 Task 2, the TRACER leg): ONE typed
// /opsx:new invocation drives the real command end-to-end — the change
// scaffold (incl. the workflow's sidecar) lands on disk, zero
// not-implemented results, and the closing output is captured to testdata as
// D-03 pass-1 raw material.
func TestOpsxMatrixNew_Gated(t *testing.T) { //nolint:paralleltest // gated, HOME-pinned leg
	e2eGates(t)

	const (
		sid     = "sess-matrix-new"
		subject = "matrix-tracer-subject"
	)

	r, scratch := newOpsxMatrixRunner(t)

	runMatrixStage(t, r, sid, "/opsx:new "+subject)

	// The command's own reported artifacts exist on disk: minimally the new
	// change directory (the scaffold the closing output reports).
	changeDir := filepath.Join(scratch, "openspec", "changes", subject)
	if !fileExists(changeDir) {
		entries, _ := os.ReadDir(filepath.Join(scratch, "openspec", "changes"))

		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}

		t.Fatalf("no change directory %s after /opsx:new (changes/: %v)",
			changeDir, names)
	}

	assertNoNotImplementedResults(t, r, sid)

	captureMatrixClosing(t, "new-happy-capture", lastAssistantText(t, r, sid))
}

// seedMatrixChange runs a real `openspec new change <name>` in the scratch
// and returns the change dir (the scaffold: .openspec.yaml sidecar).
func seedMatrixChange(t *testing.T, scratch, name string) string {
	t.Helper()

	newCmd := exec.CommandContext(context.Background(), "openspec", "new", "change", name)
	newCmd.Dir = scratch

	out, oerr := newCmd.CombinedOutput()
	if oerr != nil {
		t.Fatalf("openspec new change %s: %v\n%s", name, oerr, out)
	}

	dir := filepath.Join(scratch, "openspec", "changes", name)
	if !fileExists(dir) {
		t.Fatalf("new change %s produced no %s", name, dir)
	}

	return dir
}

// writeMatrixArtifact plants a minimal hand-authored artifact in a seeded
// change (the fixture state continue/ff/verify need — the REAL artifacts are
// model-authored during the legs themselves).
func writeMatrixArtifact(t *testing.T, changeDir, rel, body string) {
	t.Helper()

	p := filepath.Join(changeDir, rel)

	err := os.MkdirAll(filepath.Dir(p), 0o750)
	if err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}

	err = os.WriteFile(p, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// matrixProposal is the minimal proposal fixture (ff/verify's precondition).
const matrixProposal = `## Why

The scratch app needs the sum feature verified end-to-end.

## What Changes

- Add the add function and exercise it from main.

## Impact

Minimal: one function, one call site.
`

// TestOpsxMatrixContinue_Gated (13-01 Task 3): /opsx:continue on a mid-flight
// change (the new-change scaffold, 0/4 artifacts) creates the NEXT artifact
// (the proposal) — the command's own closing reports it.
func TestOpsxMatrixContinue_Gated(t *testing.T) { //nolint:paralleltest // HOME-pinned leg
	e2eGates(t)

	const (
		sid     = "sess-matrix-continue"
		subject = "matrix-continue-subj"
	)

	r, scratch := newOpsxMatrixRunner(t)

	changeDir := seedMatrixChange(t, scratch, subject)

	runMatrixStage(t, r, sid, "/opsx:continue "+subject)

	if !fileExists(filepath.Join(changeDir, "proposal.md")) {
		t.Errorf("no proposal.md in %s after /opsx:continue (the next artifact)", changeDir)
	}

	assertNoNotImplementedResults(t, r, sid)

	captureMatrixClosing(t, "continue-happy-capture", lastAssistantText(t, r, sid))
}

// TestOpsxMatrixFF_Gated (13-01 Task 3): /opsx:ff on a change carrying a
// proposal completes ALL remaining artifacts in dependency order (design +
// specs + tasks at minimum, per the spec-driven schema).
func TestOpsxMatrixFF_Gated(t *testing.T) { //nolint:paralleltest // HOME-pinned leg
	e2eGates(t)

	const (
		sid     = "sess-matrix-ff"
		subject = "matrix-ff-subj"
	)

	r, scratch := newOpsxMatrixRunner(t)

	changeDir := seedMatrixChange(t, scratch, subject)
	writeMatrixArtifact(t, changeDir, "proposal.md", matrixProposal)

	runMatrixStage(t, r, sid, "/opsx:ff "+subject)

	// The terminal artifact: tasks.md (the dependency-order end of the
	// spec-driven sequence). CAPTURE-WINS: the observed ff flow completes the
	// artifacts AND archives the change in one pass (ff-happy-capture
	// 2026-08-20: "All artifacts complete... Archived to changes/archive/") —
	// so tasks.md lives in the live change dir OR the dated archive dir.
	if !fileExists(filepath.Join(changeDir, "tasks.md")) {
		found := false

		matches, _ := filepath.Glob(filepath.Join(scratch, "openspec", "changes", "archive", "*"+subject))
		for _, m := range matches {
			if fileExists(filepath.Join(m, "tasks.md")) {
				found = true
			}
		}

		if !found {
			t.Errorf("no tasks.md in %s (or its archive) after /opsx:ff (the dependency-order terminus)", changeDir)
		}
	}

	assertNoNotImplementedResults(t, r, sid)

	captureMatrixClosing(t, "ff-happy-capture", lastAssistantText(t, r, sid))
}

// TestOpsxMatrixVerify_Gated (13-01 Task 3): /opsx:verify on a change with
// artifacts present reports the validation verdict (CRITICAL/WARNING/
// SUGGESTION shape per FEATURES.md — the CAPTURE wins on divergence).
func TestOpsxMatrixVerify_Gated(t *testing.T) { //nolint:paralleltest // HOME-pinned leg
	e2eGates(t)

	const (
		sid     = "sess-matrix-verify"
		subject = "matrix-verify-subj"
	)

	r, scratch := newOpsxMatrixRunner(t)

	changeDir := seedMatrixChange(t, scratch, subject)
	writeMatrixArtifact(t, changeDir, "proposal.md", matrixProposal)
	writeMatrixArtifact(t, changeDir, "specs/spec.md",
		"# matrix-verify-subj spec\n\n## ADDED Requirements\n\n### Requirement: summation\n\nThe app SHALL sum two numbers.\n")

	runMatrixStage(t, r, sid, "/opsx:verify "+subject)

	// Read-only row discipline: verify writes no archive artifacts; the
	// assertion is the captured verdict itself (the closing output).
	assertNoNotImplementedResults(t, r, sid)

	captureMatrixClosing(t, "verify-happy-capture", lastAssistantText(t, r, sid))
}

// TestOpsxMatrixBulkArchive_Gated (13-01 Task 3): /opsx:bulk-archive over
// multiple completed changes (N=2 with completed tasks) archives the batch —
// archive dirs land under openspec/changes/archive/.
func TestOpsxMatrixBulkArchive_Gated(t *testing.T) { //nolint:funlen,paralleltest // two-fixture batch leg
	e2eGates(t)

	const sid = "sess-matrix-bulk"

	r, scratch := newOpsxMatrixRunner(t)

	for _, subject := range []string{"matrix-bulk-one", "matrix-bulk-two"} {
		changeDir := seedMatrixChange(t, scratch, subject)
		writeMatrixArtifact(t, changeDir, "proposal.md", matrixProposal)
		writeMatrixArtifact(t, changeDir, "specs/spec.md",
			"# "+subject+" spec\n\n## ADDED Requirements\n\n### Requirement: batch\n\nThe app SHALL batch.\n")
		writeMatrixArtifact(t, changeDir, "tasks.md", "- [x] 1. Seed task (complete)\n")
	}

	runMatrixStage(t, r, sid, "/opsx:bulk-archive")

	archiveBase := filepath.Join(scratch, "openspec", "changes", "archive")

	archived := 0

	entries, _ := os.ReadDir(archiveBase)
	for _, e := range entries {
		if e.IsDir() && strings.Contains(e.Name(), "matrix-bulk") {
			archived++
		}
	}

	if archived < 2 {
		t.Errorf("archived matrix-bulk changes = %d; want both (archive/: %+v)", archived, entries)
	}

	assertNoNotImplementedResults(t, r, sid)

	captureMatrixClosing(t, "bulk-archive-happy-capture", lastAssistantText(t, r, sid))
}

// TestOpsxMatrixOnboard_Gated (13-01 Task 3, D-08 full-tilt): /opsx:onboard
// in its own FRESH seeded scratch runs the guided tutorial with REAL project
// writes (a real change is created and worked) — asks time out per the D-01
// hands-off mode and 13-00's engine-visible resume carries the turn through
// them. Assertions follow the command's own closing report (capture-wins).
func TestOpsxMatrixOnboard_Gated(t *testing.T) { //nolint:funlen,paralleltest // tutorial leg
	e2eGates(t)

	const sid = "sess-matrix-onboard"

	r, scratch := newOpsxMatrixRunner(t)

	runMatrixStage(t, r, sid, "/opsx:onboard")

	// Full-tilt per D-08: the tutorial creates a real change in the scratch
	// (the onboarding cycle's core write). Assert what the cycle must leave:
	// at least one change dir beyond the sidecar-only state is NOT required
	// (the tutorial may archive its own change at the end) — the load-bearing
	// assertion is the scratch-local project state: openspec/ config exists
	// (init) AND the transcript shows real openspec CLI activity.
	if !fileExists(filepath.Join(scratch, "openspec", "config.yaml")) {
		t.Errorf("no openspec/config.yaml in the onboard scratch — the project wiring is missing")
	}

	assertNoNotImplementedResults(t, r, sid)

	captureMatrixClosing(t, "onboard-happy-capture", lastAssistantText(t, r, sid))
}
