package coreexec //nolint:testpackage // asserts the unexported keyError convention

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// probeHooks is a fake ToolHooks recording its calls; refusal is configurable
// (the pre-join PreToolUse refusal shape — post-join the leg is disposed, so
// refuse only proves the executor NEVER consults it).
type probeHooks struct {
	refuse     bool
	preCalled  []string
	postCalled []string
	lastOutput json.RawMessage
}

func (p *probeHooks) PreToolUse( //nolint:nonamedreturns // mirrors the pre-join seam pair
	_ context.Context, toolName string, _ json.RawMessage,
) (proceed bool, message string) {
	p.preCalled = append(p.preCalled, toolName)
	if p.refuse {
		return false, "operator policy forbids this"
	}

	return true, ""
}

func (p *probeHooks) PostToolUse(_ context.Context, toolName string, _, output json.RawMessage) {
	p.postCalled = append(p.postCalled, toolName)
	p.lastOutput = output
}

// TestRegisterCorePreToolUseDisposal (21-06 Task 2, the reduced contract):
// after the gate join the executor wrap consults NO PreToolUse — a hook that
// would deny does NOT produce the legacy refusal result form at the
// executor; the tool runs and denials arrive ONLY as gate results (one
// result form per decision, 21-RESEARCH Pitfall 2).
func TestRegisterCorePreToolUseDisposal(t *testing.T) {
	t.Parallel()

	cat := toolcat.NewCatalog()
	cat.Register(toolcat.Tool{Name: toolNameBash})

	hooks := &probeHooks{refuse: true}
	RegisterCore(cat, Config{WorkDir: t.TempDir(), Hooks: hooks})

	tool, ok := cat.Get(toolNameBash)
	if !ok {
		t.Fatal("Bash not registered")
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo still-runs"}`))
	if err != nil {
		t.Fatalf("post-join the executor runs what the gate admitted: %v", err)
	}

	if !contains(string(out), "still-runs") {
		t.Errorf("a would-deny hook blocked at the executor (denials are gate results now), got %q", string(out))
	}

	if len(hooks.preCalled) != 0 {
		t.Errorf("PreToolUse consulted at the executor: %v — the 21-06 join disposed this leg", hooks.preCalled)
	}

	if len(hooks.postCalled) != 1 || hooks.postCalled[0] != toolNameBash {
		t.Errorf("PostToolUse calls = %v; want exactly [Bash] (observation survives the join)", hooks.postCalled)
	}
}

// TestRegisterCoreHookPassthrough verifies a proceeding hook wraps without
// changing behavior: the tool runs for real and PostToolUse observes the
// output.
func TestRegisterCoreHookPassthrough(t *testing.T) {
	t.Parallel()

	cat := toolcat.NewCatalog()
	cat.Register(toolcat.Tool{Name: toolNameBash})

	hooks := &probeHooks{}
	RegisterCore(cat, Config{WorkDir: t.TempDir(), Hooks: hooks})

	tool, _ := cat.Get(toolNameBash)

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo real-run"}`))
	if err != nil {
		t.Fatalf("proceeding hook must not change execution: %v", err)
	}

	if !contains(string(out), "real-run") {
		t.Errorf("Bash must have actually run, got %q", string(out))
	}

	if len(hooks.postCalled) != 1 || hooks.postCalled[0] != toolNameBash {
		t.Errorf("PostToolUse calls = %v, want [Bash]", hooks.postCalled)
	}

	if len(hooks.lastOutput) == 0 {
		t.Error("PostToolUse must observe the tool output")
	}
}

// TestRegisterCoreNoHooksUnchanged verifies nil Hooks leaves the executors
// unwrapped (the pre-12-02 behavior).
func TestRegisterCoreNoHooksUnchanged(t *testing.T) {
	t.Parallel()

	cat := toolcat.NewCatalog()
	cat.Register(toolcat.Tool{Name: toolNameBash})
	RegisterCore(cat, Config{WorkDir: t.TempDir()})

	tool, _ := cat.Get(toolNameBash)

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo plain"}`))
	if err != nil {
		t.Fatalf("plain execution: %v", err)
	}

	if !contains(string(out), "plain") {
		t.Errorf("plain Bash result = %q", string(out))
	}
}

// markerCount reads the marker file and counts appended lines (0 when
// absent).
func markerCount(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	return strings.Count(string(data), "\n")
}

// TestHooksOnceOnly (21-06 Task 2, Pitfall 2's double-fire regression pin):
// a REAL script hook with an observable side effect (marker-file append)
// fires EXACTLY ONCE per tool call post-join. The marker hook is registered
// for BOTH PreToolUse and PostToolUse, so before the leg disposal each call
// appended TWICE (the executor consultation + the observation, plus the gate
// head's own consultation in the full composition); after the join the
// PostToolUse observation is the executor's only hook leg.
func TestHooksOnceOnly(t *testing.T) {
	t.Parallel()

	t.Run("side effect exactly once per call", func(t *testing.T) {
		t.Parallel()

		work := t.TempDir()
		marker := filepath.Join(work, "marker.log")

		appendCmd := "printf 'x\\n' >> '" + marker + "'"

		runner := ecosys.NewHookRunner([]ecosys.HookConfig{
			{Event: "PreToolUse", Matcher: toolNameBash, Command: appendCmd},
			{Event: "PostToolUse", Matcher: toolNameBash, Command: appendCmd},
		}, "sess-once", work, "")

		cat := toolcat.NewCatalog()
		cat.Register(toolcat.Tool{Name: toolNameBash})
		RegisterCore(cat, Config{WorkDir: work, Hooks: runner})

		tool, ok := cat.Get(toolNameBash)
		if !ok {
			t.Fatal("Bash not registered")
		}

		const calls = 2

		for i := 0; i < calls; i++ {
			out, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo once"}`))
			if err != nil || !contains(string(out), "once") {
				t.Fatalf("call %d did not run for real (err=%v out=%s)", i, err, string(out))
			}
		}

		if got := markerCount(t, marker); got != calls {
			t.Errorf("hook side effect fired %d times for %d calls; want exactly %d (ONCE per call — "+
				"the executor PreToolUse leg is disposed, Pitfall 2)", got, calls, calls)
		}
	})

	t.Run("PostToolUse still observes the tool output", func(t *testing.T) {
		t.Parallel()

		work := t.TempDir()
		payloadPath := filepath.Join(work, "payload.log")

		// `cat` appends the hook's stdin payload (the documented PostToolUse
		// JSON) to the file — the observation leg still carries the tool
		// output after the join.
		runner := ecosys.NewHookRunner([]ecosys.HookConfig{
			{Event: "PostToolUse", Matcher: toolNameBash, Command: "cat >> '" + payloadPath + "'"},
		}, "sess-post", work, "")

		cat := toolcat.NewCatalog()
		cat.Register(toolcat.Tool{Name: toolNameBash})
		RegisterCore(cat, Config{WorkDir: work, Hooks: runner})

		tool, _ := cat.Get(toolNameBash)

		out, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo observed-output"}`))
		if err != nil {
			t.Fatalf("execution: %v", err)
		}

		if !contains(string(out), "observed-output") {
			t.Fatalf("Bash did not run: %s", string(out))
		}

		data, rerr := os.ReadFile(payloadPath)
		if rerr != nil {
			t.Fatalf("PostToolUse hook never fired (no payload file): %v", rerr)
		}

		if !contains(string(data), "tool_response") || !contains(string(data), "observed-output") {
			t.Errorf("PostToolUse payload = %s; want the tool output as tool_response", string(data))
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}

const toolNameBash = "Bash"
