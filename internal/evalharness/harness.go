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
	"io/fs"
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
	Cleanup(f func())
	Errorf(format string, args ...any)
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

// ExpandedMatrixProfileJSON is the expanded-profile switch the Phase-13
// matrix bootstraps write into the operator's global openspec config before
// `openspec init`: profile "custom" (the 1.5.0 preset enum is core|custom
// ONLY — there is no named "expanded" preset) + the workflows array naming
// ALL 11 workflows, so init installs the 6 expanded commands
// (new/continue/ff/verify/bulk-archive/onboard) alongside the core 5.
// Planning probe 2026-08-19 (isolated-HOME /tmp/opsx-custom2; the installed
// package's dist/core/profiles.js CORE_WORKFLOWS vs ALL_WORKFLOWS +
// dist/core/config-schema.js).
//
//nolint:gochecknoglobals // a constant-shaped fixture payload
var ExpandedMatrixProfileJSON = []byte(`{
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
}`)

// GuardOpenSpecGlobalConfig writes cfgJSON to the operator's GLOBAL openspec
// config and restores the EXACT original state via t.Cleanup (13-01, the ONE
// bootstrap primitive every Phase-13 E2E and eval leg shares; T-13-01-01).
//
// Path resolution mirrors the binary: HOME-based
// ~/.config/openspec/config.json (verified with `openspec config path` on
// darwin + openspec 1.5.0 — the config is global-scope ONLY on the pinned
// binary, so the guard is the only way an expanded-profile scratch can exist
// without permanently switching the operator's installs).
//
// Restore semantics (byte-exact, the invariant battery pins them offline):
//   - file existed  ⇒ the original BYTES (and mode) return verbatim — never
//     a synthesized "core", never key drift from anything the binary wrote
//     mid-run (telemetry etc.);
//   - file absent   ⇒ the file is REMOVED again — a gated run must not leave
//     a global config behind in a HOME that had none.
//
// Consumers wanting a post-run byte-identity MEASUREMENT (the matrix runner
// does) must register their compare cleanup BEFORE calling this guard:
// t.Cleanup is LIFO, so a compare registered first runs AFTER the restore.
func GuardOpenSpecGlobalConfig(tb TB, cfgJSON []byte) {
	tb.Helper()

	home, err := os.UserHomeDir()
	if err != nil {
		tb.Fatalf("evalharness: resolve HOME for the global openspec config: %v", err)
	}

	cfgPath := filepath.Join(home, ".config", "openspec", "config.json")

	orig, origMode, readErr := readConfigWithMode(cfgPath)
	absent := errors.Is(readErr, fs.ErrNotExist)

	if readErr != nil && !absent {
		tb.Fatalf("evalharness: read global openspec config %s: %v", cfgPath, readErr)
	}

	err = os.MkdirAll(filepath.Dir(cfgPath), dirPerm)
	if err != nil {
		tb.Fatalf("evalharness: mkdir global openspec config dir: %v", err)
	}

	err = os.WriteFile(cfgPath, cfgJSON, filePermSeed)
	if err != nil {
		tb.Fatalf("evalharness: write expanded profile to %s: %v", cfgPath, err)
	}

	tb.Cleanup(func() {
		restoreGlobalConfig(tb, cfgPath, orig, origMode, absent)
	})
}

// restoreGlobalConfig is the guard's t.Cleanup body: byte-exact restore of the
// pre-guard state (T-13-01-01).
func restoreGlobalConfig(tb TB, cfgPath string, orig []byte, origMode fs.FileMode, absent bool) {
	tb.Helper()

	if absent {
		err := os.Remove(cfgPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			tb.Errorf("evalharness: restore absent global config (remove %s): %v", cfgPath, err)
		}

		return
	}

	err := os.WriteFile(cfgPath, orig, origMode)
	if err != nil {
		tb.Errorf("evalharness: restore global config bytes %s: %v", cfgPath, err)
	}
}

// readConfigWithMode reads path's bytes + mode (the byte-exact restore needs
// both). A missing file returns (nil, 0, fs.ErrNotExist).
func readConfigWithMode(path string) ([]byte, fs.FileMode, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("evalharness: stat global config: %w", err)
	}

	orig, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("evalharness: read global config: %w", err)
	}

	return orig, st.Mode().Perm(), nil
}

// BootstrapExpandedScratch initializes a scratch under the EXPANDED profile
// (13-04): the operator's global openspec config is guarded byte-exactly
// (GuardOpenSpecGlobalConfig) BEFORE init, so `openspec init` installs all
// 11 commands; the 6 expanded command files are asserted present (FAIL
// LOUD — a silently-core scratch would dead-end every matrix scenario).
func BootstrapExpandedScratch(t SkipTB) *Scratch {
	t.Helper()

	GuardOpenSpecGlobalConfig(t, ExpandedMatrixProfileJSON)

	scratch := t.TempDir()

	initCmd := exec.CommandContext(context.Background(), "openspec", "init", "--tools", "claude", "--force")
	initCmd.Dir = scratch

	out, err := initCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("BLOCKER: expanded-profile openspec init failed in scratch: %v\n%s", err, out)
	}

	expandedCmds := []string{"new", "continue", "ff", "verify", "bulk-archive", "onboard"}

	for _, cmd := range expandedCmds {
		p := filepath.Join(scratch, ".claude", "commands", "opsx", cmd+".md")

		_, serr := os.Stat(p)
		if serr != nil {
			t.Fatalf("expanded-profile init did NOT install /opsx:%s (missing %s)", cmd, p)
		}
	}

	SeedScratch(t, scratch)

	return &Scratch{Dir: scratch}
}

// dirPermFixture is the fixture writer's directory mode.
const dirPermFixture = 0o750

// The fixture delta bodies (scenario-less / batch / done).
const fixtureScenariolessDelta = "## ADDED Requirements\n\n" +
	"### Requirement: scenarioless\n\nThe DELIBERATELY scenario-less requirement.\n"
const fixtureBatchDelta = "## ADDED Requirements\n\n### Requirement: batch\n\n" +
	"The app SHALL batch.\n\n#### Scenario: batching\n\n- **WHEN** bulk archiving\n- **THEN** both archive\n"
const fixtureDoneDelta = "## ADDED Requirements\n\n### Requirement: done\n\n" +
	"The app SHALL done.\n\n#### Scenario: done\n\n- **WHEN** complete\n- **THEN** archived\n"

// SeedFixture plants a deterministic fixable-trigger fixture into the
// scratch (13-04; the probed classes from the 13-01 matrix legs):
//   - existing_change: `openspec new change <name>` (the already-exists
//     trigger for /opsx:new);
//   - bogus_schema: the change + a sidecar declaring an unknown schema (the
//     "Unknown schema" trigger for /opsx:continue + /opsx:ff);
//   - scenarioless: the change + proposal + a scenario-less spec delta (the
//     verify gap);
//     (the bulk-archive batch trigger);
//   - completed_change: the archivable state (CHECKED tasks — the bulk
//     happy + the onboard re-run approximation).
func SeedFixture(t TB, dir, fixture, changeName string) {
	t.Helper()

	newChange := func() string {
		newCmd := exec.CommandContext(context.Background(), "openspec", "new", "change", changeName)
		newCmd.Dir = dir

		out, err := newCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture %s: openspec new change %s: %v\n%s", fixture, changeName, err, out)
		}

		return filepath.Join(dir, "openspec", "changes", changeName)
	}

	write := func(changeDir, rel, body string) {
		t.Helper()

		p := filepath.Join(changeDir, rel)

		merr := os.MkdirAll(filepath.Dir(p), dirPermFixture)
		if merr != nil {
			t.Fatalf("fixture %s: mkdir: %v", fixture, merr)
		}

		werr := os.WriteFile(p, []byte(body), filePermSeed)
		if werr != nil {
			t.Fatalf("fixture %s: write %s: %v", fixture, p, werr)
		}
	}

	switch fixture {
	case "existing_change":
		newChange()
	case "bogus_schema":
		write(newChange(), ".openspec.yaml", "schema: bogus-schema\ncreated: 2026-08-20\n")
	case "scenarioless":
		changeDir := newChange()
		write(changeDir, "proposal.md", "## Why\n\nFix the gap.\n")
		write(changeDir, "specs/cap/spec.md", fixtureScenariolessDelta)
	case "incomplete_tasks":
		changeDir := newChange()
		write(changeDir, "proposal.md", "## Why\n\nBatch.\n")
		write(changeDir, "specs/cap/spec.md", fixtureBatchDelta)
		write(changeDir, "tasks.md", "- [ ] 1. Deliberately incomplete task (the fixable trigger)\n")
	case "completed_change":
		// The bulk-archive happy + onboard re-run approximation: a change in
		// the archivable state (proposal + valid delta + CHECKED tasks).
		changeDir := newChange()
		write(changeDir, "proposal.md", "## Why\n\nDone work.\n")
		write(changeDir, "specs/cap/spec.md", fixtureDoneDelta)
		write(changeDir, "tasks.md", "- [x] 1. Complete task\n")
	}
}

// AssertChangeDir asserts the change directory exists live or archived
// (13-04's named key for the expanded commands whose artifact is the change
// itself).
func AssertChangeDir(t TB, scratchDir, changeName string) []string {
	t.Helper()

	_, err := os.Stat(filepath.Join(scratchDir, "openspec", "changes", changeName))
	if err == nil {
		return nil // the live change directory
	}

	matches, _ := filepath.Glob(filepath.Join(scratchDir, "openspec", "changes", "archive", "*"+changeName))
	if len(matches) > 0 {
		return nil
	}

	return []string{fmt.Sprintf("change directory %s not found (live or archived) under %s", changeName, scratchDir)}
}

// AssertFixableRecovery asserts the D-01 fixable contract from the
// transcript + disk (13-04's named key): a fixable failure reached the model
// (a tool result carrying a probed failure signature), a recovery followed
// it, and the goal artifact (the change directory) landed.
//
//nolint:cyclop,funlen // the two-phase scan reads flat
func AssertFixableRecovery(t TB, lines []session.Line, scratchDir, changeName string) []string {
	t.Helper()

	failSigs := []string{
		"already exists", "Unknown schema", "archive_tasks_incomplete",
		"force closed the prompt", "No items found", "Unknown item",
		"must have at least one delta", "incomplete",
	}

	// The failure may surface in a TOOL RESULT (the CLI error classes) OR in
	// the ASSISTANT text (the report-driven commands — the 13-01 verify
	// finding: the model authors the report, no CLI validate step exists).
	bodyAt := func(i int) (string, bool) {
		switch lines[i].Type {
		case session.TypeToolResult:
			return string(lines[i].Output), true
		case session.TypeAssistantMessage:
			return lines[i].Text, true
		default:
			return "", false
		}
	}

	failIdx := -1

	for i := range lines {
		out, ok := bodyAt(i)
		if !ok {
			continue
		}

		for _, sig := range failSigs {
			if strings.Contains(out, sig) {
				failIdx = i

				break
			}
		}

		if failIdx != -1 {
			break
		}
	}

	if failIdx == -1 {
		return []string{
			"no fixable failure reached the model (no probed signature in any tool result or closing)",
		}
	}

	recoverIdx := -1

	for i := range lines {
		if i <= failIdx {
			continue
		}

		out, ok := bodyAt(i)
		if !ok {
			continue
		}

		out = strings.ToLower(out)
		recoverSigs := []string{"created", "archived", "proposal", "complete", "spec", "tasks", "verdict", "report"}

		for _, sig := range recoverSigs {
			if strings.Contains(out, sig) {
				recoverIdx = i

				break
			}
		}

		if recoverIdx != -1 {
			break
		}
	}

	if recoverIdx == -1 {
		return []string{fmt.Sprintf("no recovery after the failure (failIdx=%d)", failIdx)}
	}

	return AssertChangeDir(t, scratchDir, changeName)
}
