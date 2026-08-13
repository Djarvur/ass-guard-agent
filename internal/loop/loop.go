package loop

import (
	"context"
	"fmt"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// Run executes one turn: it wraps the prompt as a single user message, sends it
// through the provider, and returns the parsed tool-calls. SINGLE-TURN (D-15):
// no session state, no context-window projection, no ACP streaming — those are
// Phase-2 SESS/ACP concerns.
func Run(ctx context.Context, prof *profile.Profile, p provider.Provider, prompt string) ([]provider.ToolCall, error) {
	msgs := []provider.Message{{Role: "user", Content: prompt}}

	resp, err := p.Send(ctx, prof, msgs)
	if err != nil {
		return nil, fmt.Errorf("loop turn: %w", err)
	}

	return resp.ToolCalls, nil
}
