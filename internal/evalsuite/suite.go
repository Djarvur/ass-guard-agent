// Package evalsuite is the scenario-suite layer of the behavioral-eval
// regression net (12-08, ACP-08/D-03; ECOSYSTEM-AUDIT §4.4 EVAL-02): it loads
// JSON scenario definitions, executes k passes over fresh scratches with the
// real binary + real model through the evalharness seams, scores pass@k, and
// emits a machine-readable JSON result artifact per run.
//
// EXTENSION POINT (Phase 13, OS-03): adding a scenario is adding a JSON file
// under scenarios/ — no harness change. Assertions are named KEYS resolved
// by the harness (never free-form scripts in the JSON — a scenario cannot
// add or relax an assertion the harness does not own, T-12-08-01).
//
// GATE WIRING (D-03): k=1 behind ASSGUARD_EVAL_GATE=1 as the mise eval-gate
// task (NOT part of mise ci — the suites gate on the locked change classes
// via scripts/eval-change-class.sh, never every commit); k=3 is the manual
// eval-deep task.
package evalsuite

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/evalharness"
)

// scenariosFS embeds the suite definitions — adding a scenario (the Phase-13
// extension point) is adding a JSON file to this directory; no harness change.
//
//go:embed scenarios/*.json
var scenariosFS embed.FS

// DefaultScenarios loads the embedded suite definitions.
func DefaultScenarios() ([]Scenario, error) {
	dir := "scenarios"

	entries, derr := fs.ReadDir(scenariosFS, dir)
	if derr != nil {
		return nil, fmt.Errorf("evalsuite: embedded scenarios: %w", derr)
	}

	var out []Scenario

	for _, e := range entries {
		raw, rerr := scenariosFS.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			return nil, fmt.Errorf("evalsuite: embedded read %s: %w", e.Name(), rerr)
		}

		var sc Scenario

		uerr := json.Unmarshal(raw, &sc)
		if uerr != nil {
			return nil, fmt.Errorf("evalsuite: embedded %s: %w", e.Name(), uerr)
		}

		out = append(out, sc)
	}

	return out, nil
}

// EnvEvalGate is the suite's own gate flag (D-03's mirror of the
// ASSGUARD_OPENSPEC_BIN=1 pattern).
const EnvEvalGate = "ASSGUARD_EVAL_GATE"

// ErrGated is the loud-skip sentinel carrying the missing gate names.
var ErrGated = errors.New("evalsuite: gate flags unset")

// Schema sentinels (LoadScenarios' structured rejections — T-12-08-01).
var (
	//nolint:lll // the schema enumeration is the message
	ErrUnknownScenarioKey = errors.New("evalsuite: unknown scenario key (schema: id/description/changeName/stages/asserts)")
	ErrScenarioRequired   = errors.New("evalsuite: id, description, changeName, stages, asserts are all required")
	ErrUnknownAssertKey   = errors.New("evalsuite: unknown assertion key (harness-owned: zero_continue)")
	ErrNoScenarios        = errors.New("evalsuite: no scenarios found")
	ErrBadK               = errors.New("evalsuite: k must be >= 1")
	ErrNilDriver          = errors.New("evalsuite: nil driver (the wiring site must inject the runner factory)")
)

// Scenario is one suite definition: the stage prompts + the named assertion
// keys the harness owns. The JSON lives under scenarios/ (schema: exactly
// these keys — LoadScenarios rejects unknown ones, T-12-08-01).
type Scenario struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	ChangeName  string   `json:"changeName"` //nolint:tagliatelle // camelCase mirrors the captured form vocabulary
	Stages      []string `json:"stages"`
	// Asserts names harness-owned assertion keys. Today: "zero_continue"
	// (AssertZeroContinue: >= len(stages)-1 continue decisions, stage
	// provenance, the archived change directory).
	Asserts []string `json:"asserts"`
}

// PassRecord is one scenario pass's outcome evidence.
type PassRecord struct {
	Pass          bool     `json:"pass"`
	Scratch       string   `json:"scratch"`
	DurationMS    int64    `json:"duration_ms"`
	Failures      []string `json:"failures,omitempty"`
	Continues     int      `json:"continues"`
	AssistantMsgs int      `json:"assistant_messages"`
}

// ScenarioResult is one scenario's scored outcome across its k passes.
type ScenarioResult struct {
	ID       string       `json:"id"`
	K        int          `json:"k"`
	Passes   int          `json:"passes"`
	PassAtK  bool         `json:"pass_at_k"`
	PassAt1  bool         `json:"pass_at_1"`
	Records  []PassRecord `json:"passes_detail"`
	Duration float64      `json:"duration_s"`
}

// SuiteResult is the whole run: per-scenario detail + the artifact path.
type SuiteResult struct {
	Timestamp time.Time        `json:"timestamp"`
	K         int              `json:"k"`
	PassAtK   bool             `json:"pass_at_k"`
	Scenarios []ScenarioResult `json:"scenarios"`
	Artifact  string           `json:"artifact"`
}

// LoadScenarios reads every *.json scenario file in dir (sorted by name).
// Unknown keys in a scenario file are rejected — a tampered scenario cannot
// smuggle fields the runner does not own (T-12-08-01).
func LoadScenarios(dir string) ([]Scenario, error) { //nolint:cyclop // the validation battery is flat by design
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("evalsuite: read scenarios dir: %w", err)
	}

	var out []Scenario

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}

		raw, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			return nil, fmt.Errorf("evalsuite: read %s: %w", e.Name(), rerr)
		}

		var probe map[string]json.RawMessage

		uerr := json.Unmarshal(raw, &probe)
		if uerr != nil {
			return nil, fmt.Errorf("evalsuite: %s: %w", e.Name(), uerr)
		}

		for key := range probe {
			switch key {
			case "id", "description", "changeName", "stages", "asserts":
			default:
				return nil, fmt.Errorf("%w: %s: %q", ErrUnknownScenarioKey, e.Name(), key)
			}
		}

		var sc Scenario

		serr := json.Unmarshal(raw, &sc)
		if serr != nil {
			return nil, fmt.Errorf("evalsuite: %s: %w", e.Name(), serr)
		}

		if sc.ID == "" || sc.Description == "" || sc.ChangeName == "" ||
			len(sc.Stages) == 0 || len(sc.Asserts) == 0 {
			return nil, fmt.Errorf("%w: %s", ErrScenarioRequired, e.Name())
		}

		for _, a := range sc.Asserts {
			if a != "zero_continue" {
				return nil, fmt.Errorf("%w: %s: %q", ErrUnknownAssertKey, e.Name(), a)
			}
		}

		out = append(out, sc)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoScenarios, dir)
	}

	return out, nil
}

// Driver constructs one runner over a fresh scratch (the evalharness
// RunnerSeam) and tears it down after the pass — injected by the wiring site
// (package main owns the real construction).
type Driver func(t GateTB, scratchDir string) (evalharness.RunnerSeam, func() error)

// GateTB is the subset of testing.TB the suite needs (the gated run lives in
// a TEST binary; offline tests fake it).
type GateTB interface {
	Helper()
	Logf(format string, args ...any)
	Fatalf(format string, args ...any)
	Skip(args ...any)
	Skipf(format string, args ...any)
	TempDir() string
}

// GatesEvalGate skips LOUDLY unless ASSGUARD_EVAL_GATE=1 AND the harness's
// standing binary/model gates are set (never a silent pass — T-12-08-02).
func GatesEvalGate(t GateTB) {
	t.Helper()

	if os.Getenv(EnvEvalGate) != "1" {
		t.Skipf("set %s=1 (+ the standing gates %s=1 %s=1) to run the eval gate",
			EnvEvalGate, evalharness.EnvOpenspecBin, evalharness.EnvE2ELLM)
	}

	if missing := evalharness.GateEnv(); len(missing) > 0 {
		t.Skipf("standing gates unset: %v (set them alongside %s=1)", missing, EnvEvalGate)
	}
}

// Artifact-tree permissions.
const (
	dirPermOffline   = 0o750
	filePermArtifact = 0o600
)

// SuiteOpts carries the suite's injection points (defaults: the REAL
// harness bootstrap). The offline battery injects a fake bootstrap —
// deterministic k-semantics without the binary or model.
type SuiteOpts struct {
	// Bootstrap sources each pass's scratch (nil = evalharness.BootstrapScratch).
	Bootstrap func(t GateTB) *evalharness.Scratch
}

// RunForTest drives RunSuiteWith with the OFFLINE bootstrap (fresh temp
// scratches carrying the flagship archive fixture; no real binary, no
// model) — the deterministic test surface for the runner's arithmetic,
// skip-loudness, and artifact emission (T-12-08-02/03 test support).
func RunForTest(t GateTB, scenarios []Scenario, k int, driver Driver) (SuiteResult, error) {
	return RunSuiteWith(context.Background(), t, scenarios, k, driver, "", SuiteOpts{
		Bootstrap: func(gt GateTB) *evalharness.Scratch {
			// A FRESH dir per pass carrying the flagship archive fixture.
			dir, err := os.MkdirTemp("", "evalsuite-offline-*")
			if err != nil {
				gt.Fatalf("offline scratch: %v", err)
			}

			archive := filepath.Join(dir, "openspec", "changes", "archive", evalharness.ChangeName)

			perr := os.MkdirAll(archive, dirPermOffline)
			if perr != nil {
				gt.Fatalf("offline scratch archive: %v", perr)
			}

			return &evalharness.Scratch{Dir: dir}
		},
	})
}

// RunForTestWithArtifact is RunForTest plus the artifact emission.
func RunForTestWithArtifact(
	t GateTB, scenarios []Scenario, k int, driver Driver, artifactDir string,
) (SuiteResult, error) {
	res, err := RunForTest(t, scenarios, k, driver)
	if err != nil {
		return res, err
	}

	// Re-emit under artifactDir: the artifact write is idempotent over the
	// same result (RunForTest skipped the dir).
	if artifactDir != "" {
		path, werr := writeArtifact(artifactDir, &res)
		if werr != nil {
			return res, fmt.Errorf("evalsuite: artifact: %w", werr)
		}

		res.Artifact = path
	}

	return res, nil
}

// RunSuite executes every scenario k times over fresh scratches, scores
// pass@k (a scenario passes at k when ALL k passes pass — the gate's
// determinism bar) and pass@1 (the first pass alone), and emits the JSON
// artifact under artifactDir.
func RunSuite(
	ctx context.Context, t GateTB, scenarios []Scenario, k int, driver Driver, artifactDir string,
) (SuiteResult, error) {
	return RunSuiteWith(ctx, t, scenarios, k, driver, artifactDir, SuiteOpts{})
}

// RunSuiteWith is RunSuite with explicit options (the offline battery's
// bootstrap injection).
func RunSuiteWith(
	ctx context.Context, t GateTB, scenarios []Scenario, k int, driver Driver, artifactDir string, opts SuiteOpts,
) (SuiteResult, error) {
	if k < 1 {
		return SuiteResult{}, fmt.Errorf("%w: got %d", ErrBadK, k)
	}

	if driver == nil {
		return SuiteResult{}, ErrNilDriver
	}

	res := SuiteResult{Timestamp: time.Now().UTC(), K: k, PassAtK: true}
	start := time.Now()

	for _, sc := range scenarios {
		sr := ScenarioResult{ID: sc.ID, K: k, PassAtK: true}

		for range k {
			rec := runPass(ctx, t, &sc, driver, opts)
			sr.Passes += boolToInt(rec.Pass)
			sr.Records = append(sr.Records, rec)

			if !rec.Pass {
				sr.PassAtK = false
			}
		}

		sr.PassAt1 = sr.Records[0].Pass
		sr.Duration = time.Since(start).Seconds()

		if !sr.PassAtK {
			res.PassAtK = false
		}

		res.Scenarios = append(res.Scenarios, sr)
	}

	if artifactDir != "" {
		path, err := writeArtifact(artifactDir, &res)
		if err != nil {
			return res, fmt.Errorf("evalsuite: artifact: %w", err)
		}

		res.Artifact = path
	}

	return res, nil
}

// runPass drives ONE scenario pass over a fresh scratch: bootstrap, runner
// construction, then the PRODUCT-PROOF route — ONE typed prompt (the first
// stage); the engine chains the remaining stages (zero-continue is the
// flagship scenario's definition). The full-stage typed capture drive is the
// runtime E2E test's mode (evalharness.StagePrompts), not the gate's.
func runPass(ctx context.Context, t GateTB, sc *Scenario, driver Driver, opts SuiteOpts) PassRecord {
	start := time.Now()

	//nolint:contextcheck // the harness bootstrap execs init with its own bounded ctx
	scratch := evalharness.BootstrapScratch(t)
	if opts.Bootstrap != nil {
		scratch = opts.Bootstrap(t)
	}

	runner, teardown := driver(t, scratch.Dir)

	defer func() { _ = teardown() }()

	const sessionID = "sess-eval-pass"

	perr := runner.RunPrompt(ctx, sessionID, sc.Stages[0])
	if perr != nil {
		return PassRecord{
			Pass: false, Scratch: scratch.Dir, DurationMS: time.Since(start).Milliseconds(),
			Failures: []string{fmt.Sprintf("typed prompt %q: %v", sc.Stages[0], perr)},
		}
	}

	lines, err := runner.TranscriptLines(sessionID)
	if err != nil {
		return PassRecord{
			Pass: false, Scratch: scratch.Dir, DurationMS: time.Since(start).Milliseconds(),
			Failures: []string{fmt.Sprintf("transcript: %v", err)},
		}
	}

	rec := PassRecord{Scratch: scratch.Dir, DurationMS: time.Since(start).Milliseconds()}

	for _, a := range sc.Asserts {
		if a != "zero_continue" {
			continue
		}

		out := evalharness.AssertZeroContinue(t, lines, scratch.Dir, sc.ChangeName, len(sc.Stages)-1)

		rec.Continues = out.Continues
		rec.AssistantMsgs = out.AssistantMsg
		rec.Failures = append(rec.Failures, out.Failures...)
	}

	rec.Pass = len(rec.Failures) == 0

	return rec
}

func writeArtifact(dir string, res *SuiteResult) (string, error) {
	merr := os.MkdirAll(dir, dirPermOffline)
	if merr != nil {
		return "", fmt.Errorf("evalsuite: mkdir: %w", merr)
	}

	name := fmt.Sprintf("eval-%s-k%d.json", res.Timestamp.Format("20060102-150405"), res.K)
	path := filepath.Join(dir, name)

	raw, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return "", fmt.Errorf("evalsuite: marshal: %w", err)
	}

	werr := os.WriteFile(path, raw, filePermArtifact)
	if werr != nil {
		return "", fmt.Errorf("evalsuite: write: %w", werr)
	}

	return path, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}

	return 0
}
