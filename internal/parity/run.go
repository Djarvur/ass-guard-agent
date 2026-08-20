package parity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/loop"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

const filePermOwner = 0o600

var errParityNilProvider = errors.New("parity run: nil provider")

// LiveArm is the ass-guard arm: it runs one prompt through the test-harness Turn
// Loop with the zcode profile + a Provider (live Anthropic by default), and
// converts the returned provider.ToolCalls to parity.ToolCalls. It implements
// AssGuardArm.
type LiveArm struct {
	Profile  profile.Profile
	Provider provider.Provider
}

// RunTurn runs one prompt and returns the parity-normalized tool-calls.
func (a *LiveArm) RunTurn(ctx context.Context, prompt string) ([]ToolCall, error) {
	calls, err := loop.Run(ctx, &a.Profile, a.Provider, prompt)
	if err != nil {
		return nil, fmt.Errorf("call: %w", err)
	}

	out := make([]ToolCall, 0, len(calls))
	for _, c := range calls {
		out = append(out, ToolCall{Name: c.Name, Input: c.Input})
	}

	return out, nil
}

// RunOptions configures a parity run.
type RunOptions struct {
	Suite       []CapturedTurn
	Profile     profile.Profile
	Provider    provider.Provider
	ResultsPath string // JSON results file ("" = skip the file)
	Model       string // recorded in results (reproducibility, D-04)
	// BaseWorkspace seeds each turn's isolated scratch when set (12-03 Task 2:
	// per-turn workspace fixtures). Empty + no per-turn FixtureSnapshot = the
	// legacy shared-cwd behavior (the curated path — unchanged, Pitfall 18).
	BaseWorkspace string
}

// WorkspaceArm is the optional arm upgrade for workspace-isolated replays:
// when the run path materializes a per-turn scratch (BaseWorkspace set or a
// turn carries FixtureSnapshot), the arm receives the scratch dir instead of
// the bare prompt. Arms that execute file-touching tools implement it; the
// live parity arm (call-sequence comparison, no tool execution) does not.
type WorkspaceArm interface {
	RunTurnInWorkspace(ctx context.Context, prompt, workspaceDir string) ([]ToolCall, error)
}

// RunResult is the parity run's outcome + reproducibility config.
type RunResult struct {
	Summary Summary        `json:"summary"`
	Config  RunConfig      `json:"config"`
	Turns   []turnEvidence `json:"turns"`
	// CacheProbe (14-04, EARLY-03) is the cache-discipline probe's additive
	// verdict — ordering + placement-vs-pin, set by the wiring site beside the
	// A/B arms. It NEVER feeds Summary or the gate's exit semantics (additive
	// report only); nil = the probe did not run.
	CacheProbe *CacheProbeReport `json:"cache_probe,omitempty"`
}

// RunConfig is the parity run configuration recorded alongside results.
type RunConfig struct {
	Model     string    `json:"model"`
	Temp      float64   `json:"temp"`
	Timestamp time.Time `json:"timestamp"`
	SuiteSize int       `json:"suite_size"`
}

type turnEvidence struct {
	TurnID         string     `json:"turn_id"`
	AssGuardCalls  []ToolCall `json:"ass_guard_calls"`
	ZcodeCalls     []ToolCall `json:"zcode_calls"`
	Layer1Match    bool       `json:"layer1_match"`
	Layer2Mismatch int        `json:"layer2_mismatches"`
}

// Run executes the parity A/B comparison and writes the structured footer +
// results file. Returns the Summary; the caller decides exit-code semantics.
// Temp is fixed at 0 (D-04 determinism).
func Run(ctx context.Context, opts *RunOptions) (RunResult, error) {
	if opts.Provider == nil {
		return RunResult{}, errParityNilProvider
	}

	arm := &LiveArm{Profile: opts.Profile, Provider: opts.Provider}
	h := NewHarness()
	h.BaseWorkspace = opts.BaseWorkspace

	results, err := RunSuite(ctx, h, opts.Suite, arm)
	if err != nil {
		return RunResult{}, err
	}

	summary := Summarize(results)

	res := RunResult{
		Summary: summary,
		Config: RunConfig{
			Model: opts.Model, Temp: 0, Timestamp: time.Now().UTC(),
			SuiteSize: summary.SuiteSize,
		},
	}
	for _, r := range results {
		res.Turns = append(res.Turns, turnEvidence{
			TurnID: r.TurnID, AssGuardCalls: r.AssGuardCalls, ZcodeCalls: r.ZcodeCalls,
			Layer1Match: r.Comparison.Layer1SequenceMatch, Layer2Mismatch: len(r.Comparison.Layer2ArgMismatches),
		})
	}

	if opts.ResultsPath != "" {
		err := writeResults(opts.ResultsPath, &res)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parity: could not write results: %v\n", err)
		}
	}

	return res, nil
}

func writeResults(path string, res *RunResult) error {
	raw, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	err = os.WriteFile(path, raw, filePermOwner)
	if err != nil {
		return fmt.Errorf("write results: %w", err)
	}

	return nil
}

// RunSuite drives the suite through the harness: the legacy shared path when
// no workspace source is configured, else the per-turn isolated path (12-03
// Task 2 — each CapturedTurn replays into a freshly seeded scratch derived
// from the turn's fixture snapshot or the suite's base snapshot, torn down
// after the turn; file-mutating turns cannot contaminate successors — the
// 2026-08-16 drift report's 4/13 state-pollution class).
func RunSuite(ctx context.Context, h *Harness, suite []CapturedTurn, arm AssGuardArm) ([]TurnResult, error) {
	if h.BaseWorkspace == "" && !anyFixtureSnapshot(suite) {
		return h.Run(ctx, suite, arm)
	}

	all := make([]TurnResult, 0, len(suite))

	for _, turn := range suite {
		ws, err := seedTurnWorkspace(&turn, h.BaseWorkspace)
		if err != nil {
			return nil, fmt.Errorf("turn %q: seed workspace: %w", turn.TurnID, err)
		}

		res, runErr := h.Run(ctx, []CapturedTurn{turn}, dirArm{inner: arm, dir: ws.dir})

		ws.cleanup()

		if runErr != nil {
			return nil, runErr
		}

		all = append(all, res...)
	}

	return all, nil
}

// dirArm routes a turn's replay into its isolated scratch: workspace-aware
// arms receive the dir; plain arms run unchanged (their model flow touches no
// files, so isolation is a structural no-op for them — never a behavior
// change, Pitfall 18).
type dirArm struct {
	inner AssGuardArm
	dir   string
}

func (d dirArm) RunTurn(ctx context.Context, prompt string) ([]ToolCall, error) {
	if wa, ok := d.inner.(WorkspaceArm); ok {
		calls, err := wa.RunTurnInWorkspace(ctx, prompt, d.dir)
		if err != nil {
			return nil, fmt.Errorf("workspace turn: %w", err)
		}

		return calls, nil
	}

	calls, err := d.inner.RunTurn(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("turn: %w", err)
	}

	return calls, nil
}

func anyFixtureSnapshot(suite []CapturedTurn) bool {
	for _, turn := range suite {
		if turn.FixtureSnapshot != "" {
			return true
		}
	}

	return false
}

// turnWorkspace is one turn's isolated scratch + its teardown.
type turnWorkspace struct {
	dir     string
	cleanup func()
}

// seedTurnWorkspace materializes a fresh scratch dir for one turn, seeded
// from the turn's recorded fixture snapshot when present, else the suite's
// base snapshot, else left empty (a FRESH dir — never a sibling's residue).
// The cleanup removes the scratch; the seed source is never mutated.
func seedTurnWorkspace(turn *CapturedTurn, base string) (turnWorkspace, error) {
	scratch, err := os.MkdirTemp("", "parity-turn-*")
	if err != nil {
		return turnWorkspace{}, fmt.Errorf("scratch: %w", err)
	}

	ws := turnWorkspace{dir: scratch, cleanup: func() { _ = os.RemoveAll(scratch) }}

	src := turn.FixtureSnapshot
	if src == "" {
		src = base
	}

	if src == "" {
		return ws, nil
	}

	err = copyTree(src, scratch)
	if err != nil {
		ws.cleanup()

		return turnWorkspace{}, fmt.Errorf("seed from %s: %w", src, err)
	}

	return ws, nil
}

// dirPermOwnerExec is the scratch-tree directory permission (owner rwx).
const dirPermOwnerExec = 0o750

// copyTree recursively copies the src tree into dst (an existing empty dir).
func copyTree(src, dst string) error {
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk: %w", walkErr)
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("rel: %w", err)
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, dirPermOwnerExec)
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("info: %w", err)
		}

		// G122/G703: paths derive from the walk over the developer-supplied
		// snapshot dir (suite config, not model-controlled content) — the
		// seed source trust boundary is the run configuration (T-12-03-02).
		raw, err := os.ReadFile(path) //nolint:gosec // see comment above
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		return os.WriteFile(target, raw, info.Mode().Perm()) //nolint:gosec // see comment above
	})
	if err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}

	return nil
}
