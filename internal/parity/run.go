package parity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/djarvur/ass-guard-agent/internal/loop"
	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/provider"
)

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
	calls, err := loop.Run(ctx, a.Profile, a.Provider, prompt)
	if err != nil {
		return nil, err
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
}

// RunResult is the parity run's outcome + reproducibility config.
type RunResult struct {
	Summary Summary        `json:"summary"`
	Config  RunConfig      `json:"config"`
	Turns   []turnEvidence `json:"turns"`
}

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
func Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	if opts.Provider == nil {
		return RunResult{}, fmt.Errorf("parity run: nil provider")
	}
	arm := &LiveArm{Profile: opts.Profile, Provider: opts.Provider}
	h := NewHarness()
	results, err := h.Run(ctx, opts.Suite, arm)
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
		if err := writeResults(opts.ResultsPath, res); err != nil {
			fmt.Fprintf(os.Stderr, "parity: could not write results: %v\n", err)
		}
	}
	return res, nil
}

func writeResults(path string, res RunResult) error {
	raw, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
