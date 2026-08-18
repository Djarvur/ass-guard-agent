package session //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// TestSubagentTypeUsesAgentDef (12-02 Task 3, Test 2) verifies the agent
// definition surface at the PARA wiring level: an Agent tool call carrying
// subagent_type=<discovered agent> dispatches a subagent whose restricted tool
// set is the agent's declared Tools and whose provider call carries the agent's
// Prompt as an additional system block (the dynamic-merge-into-captured-shape
// pattern applied per dispatch). No confirmation tier is introduced — the
// dispatch is the same inline goroutine path as every Task call.
func TestSubagentTypeUsesAgentDef(t *testing.T) {
	t.Parallel()

	s, _, fp := newTestSession(t, nil, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				Name:  toolAgent,
				Input: json.RawMessage(`{"subagent_type":"fixture-agent","prompt":"do the thing"}`),
			}},
		},
		{FinishReason: stopEndTurn},
	})

	s.SubagentTypes = map[string]ecosys.Agent{
		"fixture-agent": {
			Name:        "fixture-agent",
			Description: "fixture wiring agent",
			Tools:       []string{toolRead, toolGrep},
			Model:       "sonnet",
			Prompt:      "You are the fixture plugin agent. Research thoroughly.",
		},
	}

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "typed-dispatch"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// The dispatch line records the agent's restricted tool set.
	found := false

	want := []string{toolRead, toolGrep}

	for _, l := range linesOf(s) {
		if l.Type == TypeSubagentDispatch && l.RestrictedTools != nil {
			found = true

			if !slices.Equal(l.RestrictedTools, want) {
				t.Errorf("restricted tools = %v, want %v (the agent's declared Tools)", l.RestrictedTools, want)
			}
		}
	}

	if !found {
		t.Error("no subagent_dispatch line carrying the agent's restricted tools")
	}

	// The subagent's provider call carried the agent's prompt as a system block.
	promptSeen := false

	for _, prof := range fp.streamedProfiles() {
		for _, blk := range prof.System {
			if strings.Contains(blk.Text, "fixture plugin agent") {
				promptSeen = true
			}
		}
	}

	if !promptSeen {
		t.Error("the subagent's provider call must carry the agent definition's prompt as a system block")
	}
}

// TestSubagentTypeUnknownFallsBack verifies an UNKNOWN subagent_type keeps the
// default restricted set (graceful degradation — the type listing is advisory).
func TestSubagentTypeUnknownFallsBack(t *testing.T) {
	t.Parallel()

	s, _, _ := newSubagentSession(t, []provider.Response{
		{
			FinishReason: blockToolUse,
			ToolCalls: []provider.ToolCall{{
				Name:  toolAgent,
				Input: json.RawMessage(`{"subagent_type":"no-such-agent","prompt":"x"}`),
			}},
		},
		{FinishReason: stopEndTurn},
	})

	s.SubagentTypes = map[string]ecosys.Agent{}

	_, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "typed-dispatch"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	found := false

	for _, l := range linesOf(s) {
		if l.Type == TypeSubagentDispatch {
			found = true

			if !slices.Equal(l.RestrictedTools, subagentRestrictedDefault) {
				t.Errorf("restricted tools = %v, want the default %v", l.RestrictedTools, subagentRestrictedDefault)
			}
		}
	}

	if !found {
		t.Error("no subagent_dispatch line for the unknown-type dispatch")
	}
}
