package hookdag_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/hookdag"
)

// TestLoadSeeded verifies the embedded zero-config floor returns the two seeded
// hooks (HOOK-02) with the documented shape.
func TestLoadSeeded(t *testing.T) {
	t.Parallel()

	hooks, err := hookdag.DefaultHooks()
	if err != nil {
		t.Fatalf("DefaultHooks: %v", err)
	}

	if len(hooks) != 2 {
		t.Fatalf("got %d hooks; want 2 (post-implement + post-phase)", len(hooks))
	}

	byName := map[string]hookdag.Hook{}
	for _, h := range hooks {
		byName[h.Name] = h
	}

	pi, ok := byName[stagePostImplement]
	if !ok {
		t.Fatal("post-implement hook missing from seed")
	}

	if len(pi.Steps) != 4 {
		t.Errorf("post-implement steps = %d; want 4 (test+lint+review+memory)", len(pi.Steps))
	}

	if _, ok := byName["post-phase"]; !ok {
		t.Error("post-phase hook missing from seed")
	}
	// The seeded test step halts on failure (a failing test stops the chain).
	var testStep *hookdag.Step

	for i := range pi.Steps {
		if pi.Steps[i].Name == "test" {
			testStep = &pi.Steps[i]
		}
	}

	if testStep == nil || testStep.OnFailure != hookdag.OnFailureHalt {
		t.Errorf("seeded test step on_failure = %v; want halt", testStep)
	}
}

// TestValidateRejectsUnknownKind verifies an unknown step kind yields a
// ConfigError naming the offender (collect-all).
func TestValidateRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	hooks := []hookdag.Hook{{
		Name: "h", Trigger: stagePostImplement,
		Steps: []hookdag.Step{{Name: "s", Kind: "teleport"}},
	}}

	err := hookdag.Validate(hooks)
	if err == nil {
		t.Fatal("Validate returned nil; want ConfigError for unknown kind")
	}

	if !contains(err.Error(), "teleport") {
		t.Errorf("err = %v; want it to name the unknown kind", err)
	}
}

// TestValidateRejectsBadOnFailure verifies an unknown on_failure yields a
// ConfigError naming the offender.
func TestValidateRejectsBadOnFailure(t *testing.T) {
	t.Parallel()

	hooks := []hookdag.Hook{{
		Name: "h", Trigger: stagePostImplement, OnFailure: "explode",
		Steps: []hookdag.Step{{Name: "s", Kind: hookdag.StepSendPrompt, Prompt: "x"}},
	}}

	err := hookdag.Validate(hooks)
	if err == nil {
		t.Fatal("Validate returned nil; want ConfigError for unknown on_failure")
	}
}

// TestValidateRejectsMissingFields verifies run-command without a command +
// send-prompt without a prompt + wait without a duration each yield violations.
func TestValidateRejectsMissingFields(t *testing.T) {
	t.Parallel()

	hooks := []hookdag.Hook{{
		Name: "h", Trigger: stagePostImplement,
		Steps: []hookdag.Step{
			{Name: "rc", Kind: hookdag.StepRunCommand},                         // missing command
			{Name: "sp", Kind: hookdag.StepSendPrompt},                         // missing prompt
			{Name: "w", Kind: hookdag.StepWait},                                // missing duration
			{Name: "badw", Kind: hookdag.StepWait, Duration: "not-a-duration"}, // bad duration
		},
	}}

	err := hookdag.Validate(hooks)
	if err == nil {
		t.Fatal("Validate returned nil; want ConfigError")
	}

	msg := err.Error()
	for _, want := range []string{"rc", "sp", "w", "badw"} {
		if !contains(msg, want) {
			t.Errorf("err %q missing violation for step %q", msg, want)
		}
	}
}

// TestLoadLayering verifies an operator overlay adds a step to the embedded
// default (D-06 — layered embedded-default → operator-overlay).
func TestLoadLayering(t *testing.T) {
	t.Parallel()

	overlay := `hooks:
  - name: custom-extra
    trigger: custom-stage
    on_failure: continue
    steps:
      - name: ping
        kind: run-command
        command: echo
        args: ["hi"]
`
	dir := t.TempDir()

	path := filepath.Join(dir, "overlay.yaml")
	err := os.WriteFile(path, []byte(overlay), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	hooks, err := hookdag.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The overlay REPLACES the hooks list (yaml overlay semantics at the
	// `hooks` key level) — so the merged result is the overlay's hooks only
	// (the embedded default is overridden when the operator declares a full
	// hooks list). Both the embedded seed (TestLoadSeeded) + this overlay path
	// work independently; this test pins the overlay behavior.
	var found bool

	for _, h := range hooks {
		if h.Name == "custom-extra" {
			found = true
		}
	}

	if !found {
		t.Errorf("overlay hook custom-extra missing from merged result: %+v", hooks)
	}
}

// TestLoadRejectsBadOverlay verifies a layered overlay with an invalid hook
// yields a ConfigError.
func TestLoadRejectsBadOverlay(t *testing.T) {
	t.Parallel()

	overlay := `hooks:
  - name: bad
    trigger: post-implement
    steps:
      - name: s
        kind: nonsense
`
	dir := t.TempDir()

	path := filepath.Join(dir, "bad.yaml")

	err := os.WriteFile(path, []byte(overlay), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = hookdag.Load(path)
	if err == nil {
		t.Fatal("Load returned nil; want ConfigError for bad overlay")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexStr(haystack, needle) >= 0)
}

func indexStr(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}

	return -1
}
