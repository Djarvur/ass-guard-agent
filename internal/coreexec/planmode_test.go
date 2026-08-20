package coreexec //nolint:testpackage // internal package test (fixture helpers shared)

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// recapturedFamily loads one family from the 12-05 re-record fixture.
func recapturedFamily(t *testing.T, tool, family string) struct {
	Template string `json:"template"`
	IsError  bool   `json:"isError"` //nolint:tagliatelle // fixture mirrors the capture
} {
	t.Helper()

	raw, err := os.ReadFile(recapturedFixturePath)
	if err != nil {
		t.Fatalf("read %s: %v", recapturedFixturePath, err)
	}

	var f struct {
		Tools map[string]struct {
			Results map[string]json.RawMessage `json:"results"`
		} `json:"tools"`
	}

	if uerr := json.Unmarshal(raw, &f); uerr != nil {
		t.Fatalf("parse fixture: %v", uerr)
	}

	var fam struct {
		Template string `json:"template"`
		IsError  bool   `json:"isError"` //nolint:tagliatelle // fixture mirrors the capture
	}

	entry, ok := f.Tools[tool]
	if !ok {
		t.Fatalf("fixture missing tool %q", tool)
	}

	famRaw, ok := entry.Results[family]
	if !ok {
		t.Fatalf("fixture missing %s.%s", tool, family)
	}

	if uerr := json.Unmarshal(famRaw, &fam); uerr != nil {
		t.Fatalf("parse %s.%s: %v", tool, family, uerr)
	}

	return fam
}

// TestPlanMode_EnteredFormCaptured: EnterPlanModeExecute returns the CAPTURED
// entered form (the 12-05 re-record, 43 observations) byte-for-byte (the
// numbered list restored from the digit-masked template).
func TestPlanMode_EnteredFormCaptured(t *testing.T) {
	t.Parallel()

	out, err := EnterPlanModeExecute()(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (plan-mode entry is not an error)", err)
	}

	var got string
	if uerr := json.Unmarshal(out, &got); uerr != nil {
		t.Fatalf("Output not a JSON string: %v (%s)", uerr, out)
	}

	if !strings.HasPrefix(got, "Entered plan mode. You should now focus on exploring") {
		t.Errorf("entered form head = %q; want the captured head", got[:60])
	}

	if !strings.Contains(got, "use ExitPlanMode to present your plan for approval") ||
		!strings.HasSuffix(got, "Remember: DO NOT write or edit any files yet. This is a read-only exploration and planning phase.") {
		t.Errorf("entered form = %q; want the captured guidance tail", got)
	}
}

// TestPlanMode_ExitSuspendsWithApprovalPayload: ExitPlanModeExecute under an
// ON state parses the plan, emits the approval question payload, and returns
// the suspension sentinel; an OFF state yields the structured not-in-plan-mode
// error (the target throws InvalidState — corpus-informed convention).
func TestPlanMode_ExitSuspendsWithApprovalPayload(t *testing.T) {
	t.Parallel()

	state := session.NewPlanModeState()
	state.Enter()

	out, err := ExitPlanModeExecute(state)(context.Background(), json.RawMessage(`{"plan":"step 1"}`))
	if err == nil || err.Error() != session.ErrSuspended.Error() {
		t.Fatalf("err = %v; want the suspension sentinel", err)
	}

	var qs []session.AskQuestion
	if uerr := json.Unmarshal(out, &qs); uerr != nil || len(qs) != 1 {
		t.Fatalf("payload = %s; want one approval question", out)
	}

	if !strings.Contains(qs[0].Question, "step 1") || len(qs[0].Options) != 2 {
		t.Errorf("approval question = %+v; want the plan + approve/decline options", qs[0])
	}

	// OFF state: the structured error, never a suspension.
	off := session.NewPlanModeState()

	_, err = ExitPlanModeExecute(off)(context.Background(), json.RawMessage(`{"plan":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "plan mode") {
		t.Fatalf("off-state err = %v; want the not-in-plan-mode error", err)
	}
}

// TestRegisterInteractive_SchemaNeverRewritten: the registration overrides
// ONLY Execute — the captured schema fields stay byte-identical (the 08-05
// discipline).
func TestRegisterInteractive_SchemaNeverRewritten(t *testing.T) {
	t.Parallel()

	catalog := toolcat.NewCatalog()

	before := map[string]toolcat.Tool{}
	for _, name := range []string{"EnterPlanMode", "ExitPlanMode"} {
		tool, ok := catalog.Get(name)
		if !ok {
			t.Fatalf("catalog missing %s (the captured catalog must carry it)", name)
		}
		before[name] = tool.Clone()
	}

	RegisterInteractive(catalog, InteractiveConfig{PlanMode: session.NewPlanModeState()})

	for _, name := range []string{"EnterPlanMode", "ExitPlanMode"} {
		after, ok := catalog.Get(name)
		if !ok {
			t.Fatalf("%s vanished", name)
		}

		b := before[name]
		if after.Name != b.Name || string(after.InputSchema) != string(b.InputSchema) ||
			after.Description != b.Description || after.Mutability != b.Mutability {
			t.Errorf("%s schema rewritten by RegisterInteractive", name)
		}

		if after.Execute == nil {
			t.Errorf("%s Execute not set", name)
		}
	}
}
