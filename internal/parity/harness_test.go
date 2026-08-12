package parity_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
)

// fakeArm is reserved for tests that need a single canned response (unused here;
// tableArm handles per-prompt variation). Kept minimal to avoid an unused-import.
func turnsOf(calls ...[2]string) []parity.ToolCall {
	out := make([]parity.ToolCall, 0, len(calls))
	for _, c := range calls {
		out = append(out, parity.ToolCall{Name: c[0], Input: json.RawMessage(c[1])})
	}

	return out
}

// TestHarness_AllMatch confirms OverallPass when the arm reproduces every turn.
func TestHarness_AllMatch(t *testing.T) {
	suite := []parity.CapturedTurn{
		{TurnID: "t1", Prompt: "p1", ExpectedToolCalls: turnsOf([2]string{"Read", `{"file_path":"a"}`})},
		{TurnID: "t2", Prompt: "p2", ExpectedToolCalls: turnsOf([2]string{"Bash", `{"command":"go test"}`})},
	}
	arm := &tableArm{byPrompt: map[string][]parity.ToolCall{
		"p1": turnsOf([2]string{"Read", `{"file_path":"a"}`}),
		"p2": turnsOf([2]string{"Bash", `{"command":"go test"}`}),
	}}
	h := parity.NewHarness()

	results, err := h.Run(context.Background(), suite, arm)
	if err != nil {
		t.Fatal(err)
	}

	s := parity.Summarize(results)
	if !s.OverallPass {
		t.Errorf("OverallPass=false, want true (Layer1=%.2f Layer2=%.2f)", s.Layer1PassRate, s.Layer2PassRate)
	}
}

// TestHarness_Mismatch fails the gate when one turn diverges.
func TestHarness_Mismatch(t *testing.T) {
	suite := []parity.CapturedTurn{
		{TurnID: "t1", Prompt: "p1", ExpectedToolCalls: turnsOf([2]string{"Read", `{"file_path":"a"}`})},
		{TurnID: "t2", Prompt: "p2", ExpectedToolCalls: turnsOf([2]string{"Bash", `{"command":"go test"}`})},
	}
	arm := &tableArm{byPrompt: map[string][]parity.ToolCall{
		"p1": turnsOf([2]string{"Read", `{"file_path":"a"}`}),
		"p2": turnsOf([2]string{"Grep", `{"pattern":"x"}`}), // wrong tool
	}}
	h := parity.NewHarness()
	results, _ := h.Run(context.Background(), suite, arm)

	s := parity.Summarize(results)
	if s.OverallPass {
		t.Error("OverallPass=true; want false (turn t2 diverged)")
	}

	if s.Layer1PassRate != 0.5 {
		t.Errorf("Layer1PassRate = %.2f, want 0.50", s.Layer1PassRate)
	}
}

// tableArm returns canned tool-calls keyed by prompt.
type tableArm struct {
	byPrompt map[string][]parity.ToolCall
}

func (a *tableArm) RunTurn(ctx context.Context, prompt string) ([]parity.ToolCall, error) {
	return a.byPrompt[prompt], nil
}

var _ parity.AssGuardArm = (*tableArm)(nil)
