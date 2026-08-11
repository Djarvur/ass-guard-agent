package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCapabilityZeroReqNoFiltering: a zero capReq leaves the primary unchanged
// regardless of its capabilities (Plan 03-01 behavior, no regression).
func TestCapabilityZeroReqNoFiltering(t *testing.T) {
	r := NewResolver(loadValid(t))
	now := ny(2026, time.August, 16, 12, 0) // Sunday noon → global table
	primary, fallbacks, err := r.Resolve("heavy", "myproj", now, CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, "glm-5.2", primary.Model, "zero capReq returns the primary")
	require.Len(t, fallbacks, 2)
}

// TestCapabilityNeedsToolsSkipsToolLessPrimary: a tier whose primary lacks
// tool_calling but whose first fallback has it → Resolve with NeedsTools returns
// the fallback (the tool-capable one).
func TestCapabilityNeedsToolsSkipsToolLessPrimary(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{"p": {BaseURL: "x", Shape: "anthropic"}},
		Models: map[string]ModelConfig{
			"tool-less": {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, Streaming: true, ToolCalling: false}},
			"tool-full": {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, Streaming: true, ToolCalling: true}},
		},
		Tiers: map[string]TierBinding{"heavy": {Model: "tool-less", Fallback: []string{"tool-full"}}},
	}
	r := NewResolver(cfg)
	primary, _, err := r.Resolve("heavy", "", time.Now(), CapabilityReq{NeedsTools: true})
	require.NoError(t, err)
	require.Equal(t, "tool-full", primary.Model, "tool-less primary skipped → tool-full fallback chosen")
}

// TestCapabilityNeedsStreamingAndThinking: analogous skips for streaming + thinking.
func TestCapabilityNeedsStreamingAndThinking(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{"p": {BaseURL: "x", Shape: "anthropic"}},
		Models: map[string]ModelConfig{
			"no-stream": {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: true, Streaming: false, ExtendedThinking: true}},
			"stream":    {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: true, Streaming: true, ExtendedThinking: true}},
			"no-think":  {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: true, Streaming: true, ExtendedThinking: false}},
			"think":     {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: true, Streaming: true, ExtendedThinking: true}},
		},
		Tiers: map[string]TierBinding{
			"a": {Model: "no-stream", Fallback: []string{"stream"}},
			"b": {Model: "no-think", Fallback: []string{"think"}},
		},
	}
	r := NewResolver(cfg)
	p1, _, err := r.Resolve("a", "", time.Now(), CapabilityReq{NeedsStreaming: true})
	require.NoError(t, err)
	require.Equal(t, "stream", p1.Model, "no-stream primary skipped when NeedsStreaming")

	p2, _, err := r.Resolve("b", "", time.Now(), CapabilityReq{NeedsThinking: true})
	require.NoError(t, err)
	require.Equal(t, "think", p2.Model, "no-think primary skipped when NeedsThinking")
}

// TestCapabilityNoCandidateReturnsError: a tier whose primary AND all fallbacks
// lack the required capability → *CapabilityError naming the requirement + the
// candidates checked.
func TestCapabilityNoCandidateReturnsError(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{"p": {BaseURL: "x", Shape: "anthropic"}},
		Models: map[string]ModelConfig{
			"a": {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: false}},
			"b": {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: false}},
		},
		Tiers: map[string]TierBinding{"heavy": {Model: "a", Fallback: []string{"b"}}},
	}
	r := NewResolver(cfg)
	_, _, err := r.Resolve("heavy", "", time.Now(), CapabilityReq{NeedsTools: true})
	require.Error(t, err)
	var cerr *CapabilityError
	require.ErrorAs(t, err, &cerr)
	require.Contains(t, cerr.Requirement, "tool_calling")
	require.ElementsMatch(t, []string{"a", "b"}, cerr.Candidates)
	require.Contains(t, err.Error(), "no candidate")
}

// TestCapabilityPreservesFallbackOrder: when the primary is skipped, the gate
// returns the FIRST capable fallback in declared order (D-05 ordered semantics
// carry through the filter), plus the remaining chain after it.
func TestCapabilityPreservesFallbackOrder(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{"p": {BaseURL: "x", Shape: "anthropic"}},
		Models: map[string]ModelConfig{
			"nope1":    {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: false}},
			"yes-first": {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: true}},
			"yes-second": {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: true}},
		},
		Tiers: map[string]TierBinding{"heavy": {Model: "nope1", Fallback: []string{"yes-first", "yes-second"}}},
	}
	r := NewResolver(cfg)
	primary, remaining, err := r.Resolve("heavy", "", time.Now(), CapabilityReq{NeedsTools: true})
	require.NoError(t, err)
	require.Equal(t, "yes-first", primary.Model, "first capable fallback wins (ordered)")
	require.Equal(t, []string{"yes-second"}, []string{remaining[0].Model}, "remaining chain is the candidates after the chosen one")
}

// TestCapabilityDescribeReq covers the requirement-string helper.
func TestCapabilityDescribeReq(t *testing.T) {
	require.Equal(t, "(none)", describeReq(CapabilityReq{}))
	require.Equal(t, "tool_calling", describeReq(CapabilityReq{NeedsTools: true}))
	require.Equal(t, "tool_calling+streaming", describeReq(CapabilityReq{NeedsTools: true, NeedsStreaming: true}))
	require.Equal(t, "tool_calling+streaming+extended_thinking", describeReq(CapabilityReq{NeedsTools: true, NeedsStreaming: true, NeedsThinking: true}))
}
