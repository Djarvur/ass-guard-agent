package parity

import (
	"context"
	"fmt"
)

// AssGuardArm runs one ass-guard turn for a prompt, returning the tool-calls the
// ass-guard arm produced. The live arm wraps internal/loop.Run with the zcode
// profile + Anthropic adapter; the parity harness depends only on this interface.
type AssGuardArm interface {
	RunTurn(ctx context.Context, prompt string) ([]ToolCall, error)
}

// TurnResult is one turn's A/B outcome: the ass-guard arm's observed calls, the
// zcode arm's expected calls, and the two-layer comparison.
type TurnResult struct {
	TurnID        string
	Prompt        string
	AssGuardCalls []ToolCall
	ZcodeCalls    []ToolCall
	Comparison    Comparison
}

// Summary is the aggregate gate result across a suite.
type Summary struct {
	SuiteSize      int     `json:"suite_size"`
	Layer1PassRate float64 `json:"layer1_pass_rate"`
	Layer2PassRate float64 `json:"layer2_pass_rate"`
	// OverallPass is true iff every turn matched on BOTH layers (D-03 100% gate).
	OverallPass bool `json:"overall_pass"`
}

// Harness runs the parity A/B comparison.
type Harness struct{}

// NewHarness returns a Harness.
func NewHarness() *Harness { return &Harness{} }

// Run executes each suite turn: calls the ass-guard arm for the prompt, compares
// the observed calls to the suite entry's ExpectedToolCalls (the zcode arm =
// replay), and records the result.
func (h *Harness) Run(ctx context.Context, suite []CapturedTurn, arm AssGuardArm) ([]TurnResult, error) {
	results := make([]TurnResult, 0, len(suite))

	for _, turn := range suite {
		observed, err := arm.RunTurn(ctx, turn.Prompt)
		if err != nil {
			return nil, fmt.Errorf("turn %q: ass-guard arm: %w", turn.TurnID, err)
		}

		results = append(results, TurnResult{
			TurnID:        turn.TurnID,
			Prompt:        turn.Prompt,
			AssGuardCalls: observed,
			ZcodeCalls:    turn.ExpectedToolCalls,
			Comparison:    Compare(turn.ExpectedToolCalls, observed),
		})
	}

	return results, nil
}

// Summarize computes the gate summary. OverallPass is true iff every turn
// matched on BOTH Layer 1 (sequence) AND Layer 2 (arg structure) — the D-03
// 100% gate.
func Summarize(results []TurnResult) Summary {
	s := Summary{SuiteSize: len(results)}
	if len(results) == 0 {
		s.OverallPass = true

		return s
	}

	layer1Pass, layer2Pass := 0, 0

	for _, r := range results {
		if r.Comparison.Layer1SequenceMatch {
			layer1Pass++
		}
		// Layer-2 pass per turn: Layer-1 matched AND no arg mismatches.
		if r.Comparison.Layer1SequenceMatch && len(r.Comparison.Layer2ArgMismatches) == 0 {
			layer2Pass++
		}
	}

	s.Layer1PassRate = float64(layer1Pass) / float64(len(results))
	s.Layer2PassRate = float64(layer2Pass) / float64(len(results))
	s.OverallPass = layer1Pass == len(results) && layer2Pass == len(results)

	return s
}
