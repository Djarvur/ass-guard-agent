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
