package coreexec

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// probeHooks is a fake ToolHooks recording its calls; refusal is configurable.
type probeHooks struct {
	refuse     bool
	preCalled  []string
	postCalled []string
	lastOutput json.RawMessage
}

func (p *probeHooks) PreToolUse(_ context.Context, toolName string, _ json.RawMessage) (bool, string) {
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

// TestRegisterCoreHookRefusal (12-02 Task 4, Test 2 wiring) verifies the
// PreToolUse exit-2-equivalent refusal at the RegisterCore chokepoint: the
// tool call is REFUSED with the hook's message as the structured tool result
// (is_error), and the underlying tool never runs.
func TestRegisterCoreHookRefusal(t *testing.T) {
	t.Parallel()

	cat := toolcat.NewCatalog()
	cat.Register(toolcat.Tool{Name: toolNameBash})

	hooks := &probeHooks{refuse: true}
	RegisterCore(cat, Config{WorkDir: t.TempDir(), Hooks: hooks})

	tool, ok := cat.Get(toolNameBash)
	if !ok {
		t.Fatal("Bash not registered")
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo must-not-run"}`))
	if err == nil {
		t.Error("a refused call must carry the error (IsError)")
	}

	if string(out) == "" || !json.Valid(out) {
		t.Fatalf("refusal result must be structured JSON, got %q", string(out))
	}

	var parsed map[string]any
	if uerr := json.Unmarshal(out, &parsed); uerr != nil {
		t.Fatalf("parse refusal: %v", uerr)
	}

	if msg, _ := parsed[keyError].(string); msg == "" || !contains(msg, "operator policy") {
		t.Errorf("refusal result must carry the hook's message, got %q", parsed[keyError])
	}

	if len(hooks.preCalled) != 1 || hooks.preCalled[0] != toolNameBash {
		t.Errorf("PreToolUse calls = %v, want exactly [Bash]", hooks.preCalled)
	}

	if len(hooks.postCalled) != 0 {
		t.Errorf("PostToolUse must not fire for a refused call, got %v", hooks.postCalled)
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

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}

const toolNameBash = "Bash"
