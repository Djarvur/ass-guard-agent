// Package evalharness is the EXTRACTED, importable E2E harness (12-08,
// ACP-08/D-03): the Phase-8-proven gated-E2E machinery — scratch bootstrap
// with the real openspec binary, the zero-continue assertion set, capture
// mode, and the fixable-recovery probe — moved out of test-file scope so the
// runtime E2E tests (cmd/ass-guard/e2e_opsx_test.go, re-pointed here) and
// the evalsuite scenario runner consume ONE implementation.
//
// The turn-runner construction stays at its owner (package main): the
// harness takes it as the RunnerSeam — the same machinery, injected where it
// is built.
package evalharness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// Env gates (the standing house pattern — loud skip, never silent):
// ASSGUARD_OPENSPEC_BIN=1 (real openspec binary on PATH) +
// ASSGUARD_E2E_LLM=1 (live model; creds from the repo config).
const (
	EnvOpenspecBin = "ASSGUARD_OPENSPEC_BIN"
	EnvE2ELLM      = "ASSGUARD_E2E_LLM"
	EnvCapture     = "ASSGUARD_E2E_CAPTURE"
)

// The flagship flow's pinned vocabulary (the 08-06/08-09 proven constants).
const (
	StageCount      = 4                // explore, propose, apply, archive
	OverallWait     = 20 * time.Minute // the Phase-8 measured budget (474-505s/leg)
	ActionContinue  = "continue"       // the engine-decision action name
	ChangeName      = "add-a-tiny-feature"
	archiveDirParts = "openspec/changes/archive"
)

// Scratch-tree permissions (owner-only files; owner-exec dirs).
const (
	filePermSeed = 0o600
	dirPerm      = 0o750
)

// ErrNoOpenSpec is the loud gate failure when the real binary is missing.
var ErrNoOpenSpec = errors.New("evalharness: ASSGUARD_OPENSPEC_BIN=1 but no openspec binary on PATH")

// TB is the harness's testing surface (the subset of testing.TB the helpers
// use — the suite's GateTB satisfies it without dragging the full TB).
type TB interface {
	Helper()
	Logf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// RunnerSeam is the turn-runner surface the scenario passes drive: one typed
// prompt through a REAL turn (model + engine + expansion) and the session's
// transcript. Implemented by the runtime runner adapter at package main.
type RunnerSeam interface {
	RunPrompt(ctx context.Context, sessionID, text string) error
	TranscriptLines(sessionID string) ([]session.Line, error)
}

// Scratch is a bootstrapped scratch project: the real openspec tree (the
// binary's own init) plus the seeded tiny codebase.
type Scratch struct {
	Dir string
}

// seedCodebase is the tiny planted codebase so /opsx:explore has something
// real to read (the Phase-8 seed, verbatim).
var seedCodebase = map[string]string{ //nolint:gochecknoglobals // a constant-shaped fixture table
	"README.md": "# scratch-app\n\nA tiny app. Features: add two numbers.\n\nRun: go run .\n",
	"main.go": "package main\n\nimport \"fmt\"\n\n// add returns the sum of a and b.\n" +
		"func add(a, b int) int { return a + b }\n\n" +
		"func main() { fmt.Println(add(2, 3)) }\n",
}

// SkipTB adds the skip + scratch surfaces (the gate's loud skip; the
// scratch bootstrap's temp dir).
type SkipTB interface {
	TB
	Skip(args ...any)
	Skipf(format string, args ...any)
	TempDir() string
}

// Gates is the double env gate with the loud skip (T-8-25) + the
// binary-on-PATH check.
func Gates(t SkipTB) {
	t.Helper()

	if os.Getenv(EnvOpenspecBin) != "1" || os.Getenv(EnvE2ELLM) != "1" {
		t.Skip("set " + EnvOpenspecBin + "=1 (real openspec binary on PATH) AND " +
			EnvE2ELLM + "=1 (live model; creds from the repo's .ass-guard/config.yaml) to run this gate")
	}

	_, lerr := exec.LookPath("openspec")
	if lerr != nil {
		t.Fatalf("BLOCKER: %v", ErrNoOpenSpec)
	}
}

// GateEnv reports the missing gate NAMES (nil when both standing gates set)
// — the suite's loud-skip diagnostic.
func GateEnv() []string {
	var missing []string
	if os.Getenv(EnvOpenspecBin) != "1" {
		missing = append(missing, EnvOpenspecBin+"=1")
	}

	if os.Getenv(EnvE2ELLM) != "1" {
		missing = append(missing, EnvE2ELLM+"=1")
	}

	return missing
}

// BootstrapScratch initializes a scratch project with the REAL openspec
// binary and seeds the tiny codebase.
func BootstrapScratch(t SkipTB) *Scratch {
	t.Helper()

	scratch := t.TempDir()

	initCmd := exec.CommandContext(context.Background(), "openspec", "init", "--tools", "claude", "--force")
	initCmd.Dir = scratch

	out, err := initCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("BLOCKER: real openspec init failed in scratch: %v\n%s", err, out)
	}

	SeedScratch(t, scratch)

	return &Scratch{Dir: scratch}
}

// SeedScratch plants the tiny codebase into dir (the fixture table is
// package-private; SeedScratch is the public seeding surface).
func SeedScratch(t TB, dir string) {
	t.Helper()

	for name, body := range seedCodebase {
		p := filepath.Join(dir, name)

		err := os.WriteFile(p, []byte(body), filePermSeed)
		if err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
}

// AssistantTexts returns each turn's final assistant text, in order.
func AssistantTexts(lines []session.Line) []string {
	var out []string

	for i := range lines {
		if lines[i].Type == session.TypeAssistantMessage {
			out = append(out, lines[i].Text)
		}
	}

	return out
}

// CaptureStageOutputs writes each stage's assistant closing output to dir
// (stage-<n>-output<suffix>.txt) — D-12's raw material, committed as evidence.
func CaptureStageOutputs(t TB, texts []string, dir, suffix string) {
	t.Helper()

	err := os.MkdirAll(dir, dirPerm)
	if err != nil {
		t.Fatalf("mkdir capture dir: %v", err)
	}

	for i, text := range texts {
		name := fmt.Sprintf("stage-%d-output%s.txt", i+1, suffix)

		body := fmt.Sprintf("# opsx E2E stage %d closing output (captured %s, real binary + real model)\n\n%s\n",
			i+1, time.Now().UTC().Format(time.RFC3339), text)

		err = os.WriteFile(filepath.Join(dir, name), []byte(body), filePermSeed)
		if err != nil {
			t.Fatalf("write capture %s: %v", name, err)
		}
	}

	t.Logf("captured %d stage outputs to %s", len(texts), dir)
}

// RunOutcome is one scenario pass's evidence (the suite's per-pass record).
type RunOutcome struct {
	StopReason   string
	Transcripts  []string
	Continues    int
	Provenance   []string
	Pass         bool
	Failures     []string
	AssistantMsg int
}

// AssertZeroContinue is the product-proof assertion set over one pass's
// transcript: the engine chained >= minContinues stages, every chained stage
// arrived as an EXPANDED injected turn (command_provenance), and the archive
// directory carries the change.
//
//nolint:cyclop,funlen // the assertion battery is flat by design
func AssertZeroContinue(t TB, lines []session.Line, scratchDir, changeName string, minContinues int) RunOutcome {
	t.Helper()

	out := RunOutcome{Transcripts: transcriptsOf(lines)}

	var actions []string

	for i := range lines {
		if lines[i].Type == session.TypeEngineDecision {
			actions = append(actions, lines[i].Name)
		}

		if lines[i].Type == session.TypeCommandProvenance {
			out.Provenance = append(out.Provenance, lines[i].Name)
		}

		if lines[i].Type == session.TypeAssistantMessage {
			out.AssistantMsg++
		}
	}

	for _, a := range actions {
		if a == ActionContinue {
			out.Continues++
		}
	}

	if out.Continues < minContinues {
		out.Failures = append(out.Failures, fmt.Sprintf(
			"engine continue decisions = %d; want >= %d (explore→propose→apply→archive). decisions=%v",
			out.Continues, minContinues, actions))
	}

	for _, key := range []string{"opsx:propose", "opsx:apply", "opsx:archive"} {
		found := false

		for _, p := range out.Provenance {
			if p == key {
				found = true
			}
		}

		if !found {
			out.Failures = append(out.Failures, fmt.Sprintf(
				"no %s provenance — the stage did not arrive as an expanded injected turn (provenance=%v)",
				key, out.Provenance))
		}
	}

	archiveDir := filepath.Join(scratchDir, filepath.FromSlash(archiveDirParts))

	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		out.Failures = append(out.Failures,
			fmt.Sprintf("archive directory missing after the run (%s): %v", archiveDir, err))
	} else {
		found := false

		for _, e := range entries {
			if strings.Contains(e.Name(), changeName) {
				found = true
			}
		}

		if !found {
			out.Failures = append(out.Failures,
				fmt.Sprintf("no archived %q directory under %s (entries: %v)", changeName, archiveDir, entries))
		}
	}

	out.Pass = len(out.Failures) == 0

	return out
}

// ScanFixableRecovery locates the fixable-failure → recovery pair in a
// transcript (the model ADAPTS from the observed CLI error: retry with --yes
// or complete the task — no halt, no silent swallow; D-10).
//
//nolint:gocritic // unnamedResult: house config forbids named returns
func ScanFixableRecovery(lines []session.Line) (int, int) {
	failIdx, recoverIdx := -1, -1

	for i := range lines {
		if lines[i].Type != session.TypeToolResult {
			continue
		}

		out := string(lines[i].Output)

		isFixableFailure := strings.Contains(out, "force closed the prompt") ||
			strings.Contains(out, "archive_tasks_incomplete")

		if isFixableFailure && failIdx == -1 {
			failIdx = i
		}

		if failIdx != -1 && recoverIdx == -1 && strings.Contains(out, "archived") {
			recoverIdx = i
		}
	}

	return failIdx, recoverIdx
}

// StagePrompts renders the four typed stage commands for changeName.
func StagePrompts(changeName string) []string {
	return []string{
		"/opsx:explore " + changeName,
		"/opsx:propose " + changeName,
		"/opsx:apply " + changeName,
		"/opsx:archive " + changeName,
	}
}

func transcriptsOf(lines []session.Line) []string {
	out := make([]string, 0, len(lines))
	for i := range lines {
		out = append(out, string(lines[i].Content))
	}

	return out
}
