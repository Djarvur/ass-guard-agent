package runtime //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
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
	"github.com/Djarvur/ass-guard-agent/internal/providerfactory"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// The Phase-8 product proof (CMD-04): a REAL /opsx scenario —
// explore → propose → apply → archive — through ass-guard in a scratch
// project, against the REAL openspec binary and the REAL model, with ZERO
// manual continues.
//
// Double env gate (T-8-25):
//   - ASSGUARD_OPENSPEC_BIN=1 — the real openspec binary must be on PATH
//     (the adapter + init/new/archive run for real);
//   - ASSGUARD_E2E_LLM=1 — the live model dependency (provider creds come
//     from the repo's .ass-guard/config.yaml — located by walking up
//     from the test working directory; missing creds FAIL LOUD, never skip).
//
// Modes:
//   - default: the zero-continue product proof (ONE typed prompt; the engine
//     chains the rest via the seeded stage patterns).
//   - ASSGUARD_E2E_CAPTURE=1: the capture drive (D-12's raw material) — the
//     four stages are typed MANUALLY so each stage's real assistant output
//     lands in testdata BEFORE the chaining patterns exist. The re-seed
//     procedure (plan 08-06 T3) runs this mode, folds the captured texts
//     into seeded.toml, then the default mode proves the chain.
//
// Both modes write stage outputs to testdata/opsx-e2e/stage-<n>-output.txt.

const (
	e2eChangeName   = "add-a-tiny-feature"
	e2eStageCount   = 4 // explore, propose, apply, archive
	e2eOverallWait  = 20 * time.Minute
	e2eTestDataDir  = "testdata/opsx-e2e"
	e2eCaptureStage = "ASSGUARD_E2E_CAPTURE"
)

// e2eGates gate the live scenario.
func e2eGates(t *testing.T) {
	t.Helper()

	if os.Getenv("ASSGUARD_OPENSPEC_BIN") != "1" || os.Getenv("ASSGUARD_E2E_LLM") != "1" {
		t.Skip("set ASSGUARD_OPENSPEC_BIN=1 (real openspec binary on PATH) AND " +
			"ASSGUARD_E2E_LLM=1 (live model; creds from the repo's " +
			".ass-guard/config.yaml) to run the real /opsx E2E")
	}

	_, lerr := exec.LookPath("openspec")
	if lerr != nil {
		t.Fatalf("BLOCKER: ASSGUARD_OPENSPEC_BIN=1 but no openspec binary on PATH: %v", lerr)
	}
}

// findRepoRoot walks up from the test cwd until a directory carrying both
// .ass-guard/config.yaml (provider creds) and profiles/ (the zcode
// bundle) is found.
func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for range 16 {
		if fileExists(filepath.Join(dir, ".ass-guard", "config.yaml")) &&
			fileExists(filepath.Join(dir, "profiles")) {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	t.Fatal("BLOCKER: cannot locate the repo root (provider creds .ass-guard/config.yaml + profiles/) — " +
		"the E2E needs the real model; run from a checkout that has them")

	return ""
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// newOpsxRunner bootstraps a scratch project with the REAL binary and returns
// a fully wired Runner (real profile + real provider + engine +
// expansion) over it.
//
//nolint:gocritic // unnamed result vs nonamedreturns (house) conflict
func newOpsxRunner(t *testing.T) (*Runner, string) {
	t.Helper()

	scratch := t.TempDir()

	return newOpsxRunnerAt(t, scratch), scratch
}

// newOpsxRunnerAt bootstraps the runner over an EXISTING scratch dir (the
// evalsuite bridge drives passes over the harness's fresh scratches).
func newOpsxRunnerAt(t *testing.T, scratch string) *Runner {
	t.Helper()

	repo := findRepoRoot(t)

	factory, providerName, err := providerfactory.SetupProviderFactory(repo, os.Stderr)
	if err != nil {
		t.Fatalf("BLOCKER: provider factory (real model creds): %v", err)
	}

	if _, _, ok := factory.Endpoint(providerName); !ok {
		t.Fatalf("BLOCKER: provider %q has no credentialed endpoint — the E2E needs the real model", providerName)
	}

	// REAL binary: install the opsx command + skill files into scratch.
	initCmd := exec.CommandContext(context.Background(), "openspec", "init", "--tools", "claude", "--force")
	initCmd.Dir = scratch

	out, err := initCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("BLOCKER: real openspec init failed in scratch: %v\n%s", err, out)
	}

	seedScratchCodebase(t, scratch)

	prof, err := profile.NewLoader(filepath.Join(repo, "profiles")).Load(profileZcode)
	if err != nil {
		t.Fatalf("load real zcode profile: %v", err)
	}

	r := &Runner{
		bus:     event.NewBus(),
		profile: prof,
		workDir: scratch,
		maxConc: 6,
		// 12-08 finding: with asks REAL (12-01) the model occasionally asks
		// mid-chain (observed: the archive stage) — the suspension would kill
		// the hands-off chain at the no-chain-suspension pin. D-01's documented
		// hands-off mode: a bounded ask timeout returns the capture-shaped
		// non-answer and the model proceeds. 45s bounds the gate's budget.
		askTimeout: 45 * time.Second,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider {
			p, _ := factory.Build(providerName, shaper.New())

			return p
		},
	}

	err = r.SetupEngine()
	if err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	r.LoadCommandRegistry()

	return r
}

// seedScratchCodebase plants a tiny codebase so /opsx:explore has something
// real to read.
func seedScratchCodebase(t *testing.T, scratch string) {
	t.Helper()

	files := map[string]string{
		"README.md": "# scratch-app\n\nA tiny app. Features: add two numbers.\n\nRun: go run .\n",
		"main.go": "package main\n\nimport \"fmt\"\n\n// add returns the sum of a and b.\n" +
			"func add(a, b int) int { return a + b }\n\n" +
			"func main() { fmt.Println(add(2, 3)) }\n",
	}

	for name, body := range files {
		p := filepath.Join(scratch, name)

		err := os.WriteFile(p, []byte(body), 0o600)
		if err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
}

// stageAssistantTexts returns each Prompt turn's final assistant text, in
// order (one per stage in the scenario).
func stageAssistantTexts(t *testing.T, r *Runner, sessionID string) []string {
	t.Helper()

	lines, err := r.sessions[sessionID].Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var out []string

	for i := range lines {
		if lines[i].Type == session.TypeAssistantMessage {
			out = append(out, lines[i].Text)
		}
	}

	return out
}

// captureStageOutputs writes each stage's assistant closing output to
// testdata/opsx-e2e/stage-<n>-output.txt (D-12's raw material — the re-seed
// source). Committed as evidence; the provenance header names the run.
func captureStageOutputs(t *testing.T, texts []string, suffix string) {
	t.Helper()

	dir := e2eTestDataDir

	err := os.MkdirAll(dir, 0o750)
	if err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}

	for i, text := range texts {
		name := fmt.Sprintf("stage-%d-output%s.txt", i+1, suffix)

		body := fmt.Sprintf("# opsx E2E stage %d closing output (captured %s, real binary + real model)\n\n%s\n",
			i+1, time.Now().UTC().Format(time.RFC3339), text)

		err = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
		if err != nil {
			t.Fatalf("write capture %s: %v", name, err)
		}
	}

	t.Logf("captured %d stage outputs to %s", len(texts), dir)
}

// runStageTyped sends one opsx command as a typed prompt and waits for the
// turn to finish.
func runStageTyped(t *testing.T, r *Runner, sessionID, text string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), e2eOverallWait)
	defer cancel()

	_, err := r.Run(ctx, sessionID, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: text}})
	if err != nil {
		t.Fatalf("stage prompt %q: %v", text, err)
	}

	// 13-00: the harness mirrors the serve loop's wait-through-suspension —
	// WaitChainIdle after Run returns (bounded by the same overall wait), so a
	// mid-stage ask's parked chain completes before the harness asserts.
	if !r.WaitChainIdle(ctx, sessionID) {
		t.Fatalf("stage prompt %q: the engine chain did not go idle within the overall wait "+
			"(a parked ask never resolved, or a chain hung)", text)
	}
}

// opsxRunnerSeam adapts Runner to evalharness.RunnerSeam (the
// extraction's seam — the suite's passes drive the SAME machinery).
type opsxRunnerSeam struct {
	r *Runner
}

func (o opsxRunnerSeam) RunPrompt(ctx context.Context, sessionID, text string) error {
	_, err := o.r.Run(ctx, sessionID, &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: text}})
	if err != nil {
		return err
	}

	// 13-00: wait through suspension exactly as the serve turn loop's parked
	// chain does — the flagship scenario's expected chain completes through
	// asks; the seam never re-drives stages (that would fake zero-continue).
	if !o.r.WaitChainIdle(ctx, sessionID) {
		//nolint:err113 // test-seam diagnostic
		return fmt.Errorf("eval seam: the engine chain did not go idle for %s within the ctx bound",
			sessionID)
	}

	return nil
}

func (o opsxRunnerSeam) TranscriptLines(sessionID string) ([]session.Line, error) {
	lines, err := o.r.sessions[sessionID].Manager.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("e2e transcript: %w", err)
	}

	return lines, nil
}

// TestOpsxEndToEnd_Gated is the milestone's product proof: the real
// explore→propose→apply→archive scenario, zero manual continues, archive
// directory present, every stage output captured.
func TestOpsxEndToEnd_Gated(t *testing.T) { //nolint:paralleltest // real scratch + live model
	e2eGates(t)

	r, scratch := newOpsxRunner(t)

	const sessionID = "sess-opsx-e2e"

	if os.Getenv(e2eCaptureStage) == "1" {
		// CAPTURE MODE (pre-re-seed): drive the four stages manually so each
		// stage's real output lands in testdata; the chaining patterns are
		// seeded FROM THIS capture (D-12), then the default mode proves the
		// zero-continue chain.
		for _, cmd := range []string{
			"/opsx:explore " + e2eChangeName,
			"/opsx:propose " + e2eChangeName,
			"/opsx:apply " + e2eChangeName,
			"/opsx:archive " + e2eChangeName,
		} {
			runStageTyped(t, r, sessionID, cmd)
		}

		texts := stageAssistantTexts(t, r, sessionID)
		if len(texts) < e2eStageCount {
			t.Fatalf("capture run produced %d assistant turns; want >= %d", len(texts), e2eStageCount)
		}

		captureStageOutputs(t, texts[:e2eStageCount], "-capture")

		return
	}

	// PRODUCT-PROOF MODE: ONE typed prompt; the engine chains the rest.
	runStageTyped(t, r, sessionID, "/opsx:explore "+e2eChangeName)

	texts := evalharness.AssistantTexts(mustLines(t, r, sessionID))
	evalharness.CaptureStageOutputs(t, texts, e2eTestDataDir, "")

	// 12-08: the zero-continue assertion set lives in the extracted harness —
	// the runtime test and the evalsuite assert identically.
	outcome := evalharness.AssertZeroContinue(t, mustLines(t, r, sessionID), scratch, e2eChangeName, e2eStageCount-1)
	for _, f := range outcome.Failures {
		t.Error(f)
	}
}

// mustLines reads the session's transcript (the assertion lens).
func mustLines(t *testing.T, r *Runner, sessionID string) []session.Line {
	t.Helper()

	lines, err := r.sessions[sessionID].Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	return lines
}

// seedIncompleteChange creates a real change whose tasks.md is deliberately
// INCOMPLETE — the deterministic fixable trigger: `openspec archive <name>`
// without --yes then hits the incomplete-tasks confirmation ("Continue?
// (y/N)"), which on a non-TTY/nil-stdin reads EOF and fails with a non-zero
// exit (verified against the installed 1.5.0 binary: plain and --json routes
// BOTH fail; only --yes or completing the tasks recovers).
func seedIncompleteChange(t *testing.T, scratch, name string) {
	t.Helper()

	newCmd := exec.CommandContext(context.Background(), "openspec", "new", "change", name)
	newCmd.Dir = scratch

	out, oerr := newCmd.CombinedOutput()
	if oerr != nil {
		t.Fatalf("openspec new change %s: %v\n%s", name, oerr, out)
	}

	tasks := filepath.Join(scratch, "openspec", "changes", name, "tasks.md")

	err := os.WriteFile(tasks, []byte("- [ ] 1. Probe task (deliberately incomplete — the fixable trigger)\n"), 0o600)
	if err != nil {
		t.Fatalf("seed tasks.md for %s: %v", name, err)
	}
}

// scanFixableRecovery walks the transcript's tool results and returns the
// line indexes of the FIRST fixable failure + the first recovery AFTER it
// (-1 when absent). Fixable signatures cover both routes the model might take
// on a no---yes first attempt: the interactive force-close (plain route) and
// the structured archive_tasks_incomplete (--json route). Recovery = a result
// reporting the archive completed.

// TestOpsxFixableRecovery_Gated proves D-10 with the real model + real
// binary, CAPTURE-FAITHFUL (the findings-5 disposition, 2026-08-15): the
// model meets openspec via Skill+Bash — the request catalog carries the
// profile's 103 zcode tools and NO openspec:* entries (exposing them would
// diverge from the capture), so the fixable-failure criterion asserts the
// behavior where it actually happens:
//
//   - MODEL-VISIBLE leg: the probe drives a fixable failure through the
//     model's own route (the first archive attempt WITHOUT --yes hits the
//     incomplete-tasks confirmation → the CLI's force-close/incomplete error
//     reaches the model as tool output) and asserts the model ADAPTS from
//     what it observed (retry with --yes, or complete the task) so the
//     archive actually completes — no halt, no silent swallow.
//   - EXECUTION-LAYER leg: the openspec:* tools stay EXECUTION-ONLY (no
//     catalog change) — the structured classification is asserted where it
//     lives, by invoking the registered openspec:archive tool DIRECTLY (no
//     model) on a second incomplete change and requiring the structured
//     result {classification: "fixable", exit_code: 1}.
func TestOpsxFixableRecovery_Gated(t *testing.T) { //nolint:paralleltest,funlen // real scratch + live model
	e2eGates(t)

	r, scratch := newOpsxRunner(t)

	// A deliberately incomplete change: the deterministic fixable trigger.
	seedIncompleteChange(t, scratch, "fixable-probe")

	const sessionID = "sess-opsx-fixable"

	// The probe asks for a natural first attempt (no tool-pinning — the model
	// cannot see openspec:* tools; pinning them was the failed premise). Any
	// no---yes first attempt fails fixably (plain AND --json routes both hit
	// the incomplete-tasks guard against nil stdin).
	prompt := "Archive the change named fixable-probe. " +
		"On your FIRST attempt do NOT pass --yes and do NOT edit tasks.md — " +
		"attempt the archive the way you normally would, and observe exactly what the command " +
		"reports in this non-interactive environment. " +
		"Then adapt based on what you observed so the archive actually completes " +
		"(you may choose the route for the retry). Report both attempts' outcomes."

	runStageTyped(t, r, sessionID, prompt)

	lines, err := r.sessions[sessionID].Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	// The model-visible behavior, asserted where it happens: a FAILED first
	// attempt, then a recovery attempt that archives.
	failIdx, recoverIdx := evalharness.ScanFixableRecovery(lines)

	if failIdx == -1 {
		t.Error("no fixable-failure result observed — the model's first archive attempt did not " +
			"hit the incomplete-tasks failure (D-10's model-facing criterion)")
	}

	if recoverIdx == -1 || recoverIdx < failIdx {
		t.Errorf("no recovery observed after the fixable failure (fail@%d recover@%d) — the model did not adapt (D-10)",
			failIdx, recoverIdx)
	}

	// The recovery is real: the archived change directory exists.
	archiveDir := filepath.Join(scratch, "openspec", "changes", "archive")

	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("archive directory missing after the run (%s): %v", archiveDir, err)
	}

	found := false

	for _, e := range entries {
		if strings.Contains(e.Name(), "fixable-probe") {
			found = true
		}
	}

	if !found {
		t.Errorf("no archived fixable-probe directory under %s (entries: %v)", archiveDir, entries)
	}

	// The EXECUTION-LAYER classification, asserted where it lives: the
	// registered openspec:archive tool (execution-only — never in the request
	// catalog) invoked DIRECTLY on a second incomplete change classifies the
	// same failure as fixable. t.Chdir keeps the adapter's subprocess inside
	// the scratch (the adapter runs in the process cwd).
	seedIncompleteChange(t, scratch, "exec-layer-probe")

	t.Chdir(scratch)

	sess := r.sessions[sessionID]

	tool, ok := sess.Catalog.Get("openspec:archive")
	if !ok {
		t.Fatal("openspec:archive tool not registered in the session catalog")
	}

	out, terr := tool.Execute(context.Background(), json.RawMessage(`{"args":["exec-layer-probe"]}`))
	if terr != nil {
		t.Fatalf("direct openspec:archive execution errored (structure over error violated): %v", terr)
	}

	var res struct {
		ExitCode       int    `json:"exit_code"`
		Classification string `json:"classification"`
		Stderr         string `json:"stderr"`
	}

	jerr := json.Unmarshal(out, &res)
	if jerr != nil {
		t.Fatalf("openspec:archive result not structured JSON: %v (%s)", jerr, out)
	}

	if res.Classification != "fixable" {
		t.Errorf("direct openspec:archive classification = %q (exit %d); want fixable — "+
			"the execution layer must still classify the failure (D-10)", res.Classification, res.ExitCode)
	}

	if res.ExitCode == 0 {
		t.Errorf("direct openspec:archive exit_code = 0; want non-zero (the incomplete-tasks failure)")
	}
}
