package openspec_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/openspec"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// executeInput is the model-facing call shape for openspec:* tools.
type executeInput struct {
	Args []string `json:"args"`
}

// execResult is the structured model-facing result shape (D-10).
type execResult struct {
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	ExitCode       int    `json:"exit_code"`
	Classification string `json:"classification"`
}

// registeredTool builds a catalog with the seeded surface registered.
func registeredTool(t *testing.T) (*toolcat.Catalog, *openspec.OpenSpecConfig) {
	t.Helper()

	cfg, err := openspec.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	cat := toolcat.NewCatalog()
	if err := openspec.RegisterTools(cat, cfg); err != nil {
		t.Fatalf("RegisterTools: %v", err)
	}

	return cat, cfg
}

// TestExecute_RealSubprocessViaStub (Test 1) verifies RegisterTools installs
// Adapter-backed Execute closures: openspec:list runs a REAL subprocess (the
// stub on PATH) and returns its stdout in the structured result.
func TestExecute_RealSubprocessViaStub(t *testing.T) { //nolint:paralleltest // putStubOnPATH mutates PATH
	putStubOnPATH(t)

	cat, _ := registeredTool(t)

	tool, ok := cat.Get("openspec:list")
	if !ok {
		t.Fatal("openspec:list not registered")
	}

	if tool.Execute == nil {
		t.Fatal("openspec:list Execute is nil — the no-Execute gap is not closed")
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"args":["--json"]}`))
	if err != nil {
		t.Fatalf("Execute err = %v; want nil (structure over error, D-10)", err)
	}

	var res execResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("result not JSON: %v (%s)", err, out)
	}

	if res.ExitCode != 0 || res.Classification != "ok" {
		t.Errorf("result = %+v; want exit 0 / ok", res)
	}

	if res.Stdout == "" {
		t.Errorf("stdout empty; want the stub's output")
	}
}

// TestExecute_ArgsForwarded (Test 2) verifies the model's args reach the child
// argv verbatim.
func TestExecute_ArgsForwarded(t *testing.T) { //nolint:paralleltest // putStubOnPATH mutates PATH
	putStubOnPATH(t)

	cat, _ := registeredTool(t)

	tool, _ := cat.Get("openspec:list")

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"args":["--changes","--json"]}`))
	if err != nil {
		t.Fatalf("Execute err = %v", err)
	}

	var res execResult
	_ = json.Unmarshal(out, &res)

	for _, want := range []string{"--changes", "--json"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("stdout = %q; want forwarded arg %q", res.Stdout, want)
		}
	}
}

// TestExecute_FailureIsStructured (Test 3, D-10) verifies a non-zero exit with
// a fixable entry yields {exit_code, stderr, classification:"fixable"} and a
// NIL Go error — the model adapts, nothing halts.
func TestExecute_FailureIsStructured(t *testing.T) { //nolint:paralleltest // stub env vars via t.Setenv
	putStubOnPATH(t)
	t.Setenv("ASSGUARD_STUB_EXIT", "3")
	t.Setenv("ASSGUARD_STUB_STDERR", "blocked: tasks incomplete")

	cat, _ := registeredTool(t)

	tool, _ := cat.Get("openspec:archive") // seeded exit_class = fixable

	out, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute err = %v; want nil for fixable outcome (D-10)", err)
	}

	var res execResult
	_ = json.Unmarshal(out, &res)

	if res.ExitCode != 3 {
		t.Errorf("exit_code = %d; want 3", res.ExitCode)
	}

	if !strings.Contains(res.Stderr, "blocked: tasks incomplete") {
		t.Errorf("stderr = %q; want the stub's stderr surfaced", res.Stderr)
	}

	if res.Classification != "fixable" {
		t.Errorf("classification = %q; want fixable", res.Classification)
	}
}

// TestExecute_HardErrorDefault (Test 8 default-class half) verifies a non-zero
// exit WITHOUT the fixable hint classifies hard-error.
func TestExecute_HardErrorDefault(t *testing.T) { //nolint:paralleltest // stub env vars
	putStubOnPATH(t)
	t.Setenv("ASSGUARD_STUB_EXIT", "2")
	t.Setenv("ASSGUARD_STUB_STDERR", "boom")

	cat, _ := registeredTool(t)

	tool, _ := cat.Get("openspec:list") // read-only, no exit_class

	out, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute err = %v; want nil (structured result, not error)", err)
	}

	var res execResult
	_ = json.Unmarshal(out, &res)

	if res.Classification != "hard-error" {
		t.Errorf("classification = %q; want hard-error", res.Classification)
	}
}

// TestExecute_MissingBinaryStructured (Test 4) verifies a missing binary yields
// classification "not-found" with a NIL Go error (model-facing structure).
func TestExecute_MissingBinaryStructured(t *testing.T) { //nolint:paralleltest // isolates PATH
	empty := t.TempDir()
	t.Setenv("PATH", empty) // no openspec anywhere

	cat, _ := registeredTool(t)

	tool, _ := cat.Get("openspec:list")

	out, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute err = %v; want nil (not-found is structured, D-10)", err)
	}

	var res execResult
	_ = json.Unmarshal(out, &res)

	if res.Classification != "not-found" {
		t.Errorf("classification = %q; want not-found", res.Classification)
	}
}

// TestExecute_AllMutatingStayBoundaries (Test 5) verifies every seeded mutating
// entry registers toolcat.MutabilityMutating (boundary floor intact) AND has an
// Execute closure.
func TestExecute_AllMutatingStayBoundaries(t *testing.T) {
	t.Parallel()

	cfg, err := openspec.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	cat := toolcat.NewCatalog()
	if err := openspec.RegisterTools(cat, cfg); err != nil {
		t.Fatalf("RegisterTools: %v", err)
	}

	mutating := 0

	for name, shape := range cfg.Commands {
		tool, ok := cat.Get("openspec:" + name)
		if !ok {
			t.Errorf("openspec:%s not registered", name)

			continue
		}

		if tool.Execute == nil {
			t.Errorf("openspec:%s Execute nil — every registered tool must be real", name)
		}

		if shape.Mutability == "mutating" {
			mutating++

			if !tool.IsMutating() {
				t.Errorf("openspec:%s should be mutating", name)
			}

			if !toolcat.IsBoundary("openspec:"+name, cat, nil) {
				t.Errorf("IsBoundary(openspec:%s) = false; want true (D-15 floor)", name)
			}
		}
	}

	if mutating < 10 {
		t.Errorf("mutating entries = %d; want >= 10 (probe surface)", mutating)
	}
}

// TestExecute_MultiWordArgvPrefix verifies the argv prefix lands before the
// model's args (key new-change → subprocess "new change <args...>").
func TestExecute_MultiWordArgvPrefix(t *testing.T) { //nolint:paralleltest // putStubOnPATH mutates PATH
	putStubOnPATH(t)

	cat, _ := registeredTool(t)

	tool, ok := cat.Get("openspec:new-change")
	if !ok {
		t.Fatal("openspec:new-change not registered")
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"args":["fix-login"]}`))
	if err != nil {
		t.Fatalf("Execute err = %v", err)
	}

	var res execResult
	_ = json.Unmarshal(out, &res)

	if !strings.Contains(res.Stdout, "new") || !strings.Contains(res.Stdout, "change") || !strings.Contains(res.Stdout, "fix-login") {
		t.Errorf("stdout = %q; want argv prefix [new change] then model arg fix-login", res.Stdout)
	}
}

// TestExecute_InputSchemaPresent verifies each tool carries the args schema so
// the model knows the call shape.
func TestExecute_InputSchemaPresent(t *testing.T) {
	t.Parallel()

	cat, _ := registeredTool(t)

	tool, _ := cat.Get("openspec:list")
	if len(tool.InputSchema) == 0 {
		t.Fatal("openspec:list InputSchema empty; want the args-array schema")
	}

	if !strings.Contains(string(tool.InputSchema), "args") {
		t.Errorf("InputSchema = %s; want an args property", tool.InputSchema)
	}
}
