package evalsuite_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/evalharness"
	"github.com/Djarvur/ass-guard-agent/internal/evalsuite"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// fakeTB records the loud-skip calls (never a silent pass — T-12-08-02).
type fakeTB struct {
	skips []string
	fatal string
}

func (f *fakeTB) Helper()                      {}
func (f *fakeTB) Logf(format string, a ...any) {}

func (f *fakeTB) Fatalf(format string, a ...any) {
	f.fatal = strings.ReplaceAll(format, "%s", "") // recorded, not panicking
}

func (f *fakeTB) Skip(_ ...any) {}
func (f *fakeTB) Skipf(format string, a ...any) {
	f.skips = append(f.skips, fmt.Sprintf(format, a...))
}
func (f *fakeTB) TempDir() string {
	d, _ := os.MkdirTemp("", "evalsuite-fake-*")

	return d
}

// fakeSeam drives scripted transcript lines without any model or binary.
type fakeSeam struct {
	lines []session.Line
}

func (s fakeSeam) RunPrompt(_ context.Context, _, _ string) error { return nil }

func (s fakeSeam) TranscriptLines(string) ([]session.Line, error) { return s.lines, nil }

// passingLines builds the transcript a PASSING zero_continue pass yields.
func passingLines(t *testing.T, change string) []session.Line {
	t.Helper()

	dir := t.TempDir()
	archive := filepath.Join(dir, "openspec", "changes", "archive")

	err := os.MkdirAll(filepath.Join(archive, change), 0o750)
	if err != nil {
		t.Fatal(err)
	}

	return []session.Line{
		{Type: session.TypeAssistantMessage, Text: "explored"},
		{Type: session.TypeEngineDecision, Name: evalharness.ActionContinue},
		{Type: session.TypeCommandProvenance, Name: "opsx:propose"},
		{Type: session.TypeEngineDecision, Name: evalharness.ActionContinue},
		{Type: session.TypeCommandProvenance, Name: "opsx:apply"},
		{Type: session.TypeEngineDecision, Name: evalharness.ActionContinue},
		{Type: session.TypeCommandProvenance, Name: "opsx:archive"},
	}
}

func fakeScenario() evalsuite.Scenario {
	return evalsuite.Scenario{
		ID: "fake", Description: "offline fake", ChangeName: evalharness.ChangeName,
		Stages: []string{"/opsx:explore " + evalharness.ChangeName}, Asserts: []string{"zero_continue"},
	}
}

// TestRunSuite_KSemantics (T2 Test 1): pass@k arithmetic — 1/1, 3/3, and the
// deliberate 2/3 case (pass@3=false, pass@1=true).
func TestRunSuite_KSemantics(t *testing.T) { //nolint:funlen // flat arithmetic battery
	t.Parallel()

	newDriver := func(lines []session.Line) evalsuite.Driver {
		return func(_ evalsuite.GateTB, _ string) (evalharness.RunnerSeam, func() error) {
			return fakeSeam{lines: lines}, func() error { return nil }
		}
	}

	// 1/1 at k=1.
	scenario := fakeScenario()

	allPass := newDriver(passingLines(t, evalharness.ChangeName))

	res, err := evalsuite.RunForTest(t, []evalsuite.Scenario{scenario}, 1, allPass)
	if err != nil {
		t.Fatal(err)
	}

	if !res.PassAtK || !res.Scenarios[0].PassAt1 || res.Scenarios[0].Passes != 1 {
		t.Errorf("k=1 all-pass: %+v", res.Scenarios[0])
	}

	// 3/3 at k=3.
	threePass := newDriver(passingLines(t, evalharness.ChangeName))

	res, err = evalsuite.RunForTest(t, []evalsuite.Scenario{scenario}, 3, threePass)
	if err != nil {
		t.Fatal(err)
	}

	if !res.PassAtK || res.Scenarios[0].Passes != 3 {
		t.Errorf("k=3 all-pass: %+v", res.Scenarios[0])
	}

	// The deliberate 2/3: passes 1-2 pass, pass 3 fails (no continues).
	weak := []session.Line{{Type: session.TypeAssistantMessage, Text: "flat"}}

	var call int

	flaky := func(_ evalsuite.GateTB, _ string) (evalharness.RunnerSeam, func() error) {
		call++

		if call <= 2 { // the deliberate 2-of-3
			return fakeSeam{lines: passingLines(t, evalharness.ChangeName)}, func() error { return nil }
		}

		return fakeSeam{lines: weak}, func() error { return nil }
	}

	res, err = evalsuite.RunForTest(t, []evalsuite.Scenario{scenario}, 3, flaky)
	if err != nil {
		t.Fatal(err)
	}

	sc := res.Scenarios[0]
	if sc.PassAtK {
		t.Error("2/3 pass: pass@3 = true, want false")
	}

	if !sc.PassAt1 {
		t.Error("2/3 pass: pass@1 = false, want true (the first pass passed)")
	}

	if sc.Passes != 2 { // the deliberate 2-of-3
		t.Errorf("2/3 pass: passes = %d, want 2", sc.Passes)
	}
}

// TestLoadScenarios_Schema (T2 Test 2a): unknown keys rejected; required
// fields enforced; unknown assertion keys rejected.
func TestLoadScenarios_Schema(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cases := map[string]string{
		//nolint:lll // raw JSON fixtures read best on one line
		"unknown-key.json":    `{"id":"x","description":"d","changeName":"c","stages":["a"],"asserts":["zero_continue"],"evil":"rm -rf"}`,
		"missing-field.json":  `{"id":"x","description":"d","stages":["a"],"asserts":["zero_continue"]}`,
		"unknown-assert.json": `{"id":"x","description":"d","changeName":"c","stages":["a"],"asserts":["always_pass"]}`,
	}

	for name, body := range cases {
		werr := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
		if werr != nil {
			t.Fatal(werr)
		}

		if _, lerr := evalsuite.LoadScenarios(dir); lerr == nil { //nolint:noinlineerr // the negative-acceptance check
			t.Errorf("%s: LoadScenarios accepted an invalid scenario", name)
		}
	}

	// The committed flagship loads clean through the embedded set.
	def, err := evalsuite.DefaultScenarios()
	if err != nil {
		t.Fatal(err)
	}

	if len(def) != 1 || def[0].ID != "opsx-flagship" || len(def[0].Asserts) != 1 {
		t.Fatalf("DefaultScenarios = %+v", def)
	}
}

// TestGatesEvalGate_LoudSkip (T2 Test 2b): unset gates skip LOUDLY carrying
// the missing gate names — never a silent pass.
func TestGatesEvalGate_LoudSkip(t *testing.T) {
	// NOT parallel: t.Setenv (the gate-flag environment is global).
	t.Setenv(evalsuite.EnvEvalGate, "")
	t.Setenv(evalharness.EnvOpenspecBin, "")
	t.Setenv(evalharness.EnvE2ELLM, "")

	fake := &fakeTB{}
	evalsuite.GatesEvalGate(fake)

	if len(fake.skips) == 0 {
		t.Fatal("no skip recorded — the gate would run ungated")
	}

	if !strings.Contains(fake.skips[0], evalsuite.EnvEvalGate) {
		t.Errorf("skip message %q does not name the gate flag", fake.skips[0])
	}

	t.Setenv(evalsuite.EnvEvalGate, "1")

	fake = &fakeTB{}
	evalsuite.GatesEvalGate(fake)

	if len(fake.skips) == 0 || !strings.Contains(fake.skips[0], evalharness.EnvOpenspecBin) {
		t.Errorf("standing-gate skip missing gate names: %v", fake.skips)
	}
}

// TestRunSuite_ArtifactEmit (T2 Test 3): every run emits the JSON artifact
// carrying scenario ids, k, per-pass outcomes, duration.
func TestRunSuite_ArtifactEmit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	res, err := evalsuite.RunForTestWithArtifact(t, []evalsuite.Scenario{fakeScenario()}, 1,
		func(_ evalsuite.GateTB, _ string) (evalharness.RunnerSeam, func() error) {
			return fakeSeam{lines: passingLines(t, evalharness.ChangeName)}, func() error { return nil }
		}, dir)
	if err != nil {
		t.Fatal(err)
	}

	if res.Artifact == "" {
		t.Fatal("artifact path not recorded in the result")
	}

	raw, err := os.ReadFile(res.Artifact)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}

	var doc map[string]any

	jerr := json.Unmarshal(raw, &doc)
	if jerr != nil {
		t.Fatalf("artifact json: %v", jerr)
	}

	if doc["k"].(float64) != 1 { //nolint:forcetypeassert // pinned shape
		t.Errorf("artifact k = %v, want 1", doc["k"])
	}

	scs, _ := doc["scenarios"].([]any)
	if len(scs) != 1 {
		t.Fatalf("artifact scenarios = %v, want 1", scs)
	}

	sc := sccsMap(t, scs[0])
	if sc["id"] != "fake" {
		t.Errorf("artifact scenario id = %v", sc["id"])
	}

	if _, ok := sc["passes_detail"]; !ok {
		t.Error("artifact missing per-pass outcomes")
	}

	if _, ok := doc["timestamp"]; !ok {
		t.Error("artifact missing timestamp")
	}
}

func sccsMap(t *testing.T, v any) map[string]any {
	t.Helper()

	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("scenario entry shape: %T", v)
	}

	return m
}

// TestDeterministicLayer_Completeness (T2 Test 4): the phase's new executor
// set each carries a *_test.go battery (EVAL-01's completeness for this
// phase's additions — ask, plan-mode pair, messaging pair, background trio,
// cron quartet + the sched store).
func TestDeterministicLayer_Completeness(t *testing.T) {
	t.Parallel()

	batteries := []string{
		"internal/coreexec/ask_test.go",
		"internal/coreexec/planmode_test.go",
		"internal/coreexec/messaging_test.go",
		"internal/coreexec/background_test.go",
		"internal/coreexec/cron_test.go",
		"internal/sched/sched_test.go",
	}

	for _, b := range batteries {
		_, serr := os.Stat(filepath.Join("..", "..", b))
		if serr != nil {
			t.Errorf("deterministic-layer battery missing: %s (%v)", b, serr)
		}
	}
}
