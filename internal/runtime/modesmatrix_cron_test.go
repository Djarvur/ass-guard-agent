package runtime //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/modesmatrix"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// The ECOS-04 (TAIL-03, 24-05) automation/cron leg: a same-package Runner
// with the engine ON, the modes-matrix fixture mounted in the host project,
// and a REAL sched.ScheduleStore automation driven through
// runAutomationTurn (the live firing path — cron semantics come from
// internal/sched only, never re-implemented). Per cell, D-12/D-13's bar:
//
//   - commands/skills: the automation prompt IS a slash invocation; the
//     adapter's expansion seam (Expand = expandUserBlocks) expands it, so
//     the provider request and the transcript's user message carry the
//     fixture body — the functional outcome, observed not assumed.
//   - hooks: the automation turn's scripted Read tool call consults the
//     gate head (21-06), which executes the fixture's PreToolUse hook —
//     the marker file receives the stdin JSON.
//
// Determinism: HOME pinned (no operator hooks), the store clock pinned
// around creation (the overdue-creation discipline from the 12-07 battery),
// transcript-as-truth + provider-capture lenses, poll deadlines instead of
// open-ended sleeps.

// cronRegistry is this test binary's automation-row registry (the wake row
// joins it from modesmatrix_wake_test.go).
var cronRegistry = modesmatrix.NewRegistry() //nolint:gochecknoglobals // per-test-binary registry

// newMatrixRunner builds an engine-on Runner over project (the fixture's
// host) with a scripted provider — the newExpansionRunner shape with the
// workDir fixed at construction so registry discovery loads the fixture.
func newMatrixRunner(t *testing.T, project string, script ...scriptedResp) (*Runner, *scriptedACPProvider) {
	t.Helper()

	bus := event.NewBus()
	prov := &scriptedACPProvider{}
	prov.queue(script...)

	r := &Runner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      project,
		maxConc:      4,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}

	if err := r.SetupEngine(); err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	r.LoadCommandRegistry()

	return r, prov
}

// fireMatrixAutomation creates one overdue hourly automation pinned to
// prompt, makes the session active, and drives the REAL due-walk →
// runAutomationTurn path once.
func fireMatrixAutomation(t *testing.T, r *Runner, project, sessID, prompt string) {
	t.Helper()

	store, err := sched.Open(project)
	if err != nil {
		t.Fatalf("sched.Open: %v", err)
	}

	r.schedule = store

	store.SetNow(func() time.Time { return time.Now().Add(-2 * time.Hour) })

	if _, err := store.Create(&sched.Automation{
		Title: "modes-matrix " + prompt, Prompt: prompt, Cron: cronExprHourly, Recurring: true,
	}); err != nil {
		t.Fatalf("store.Create: %v", err)
	}

	store.SetNow(time.Now)

	_ = r.sessionFor(context.Background(), sessID)
	r.fireDueAutomations(context.Background())
}

// matrixCronUserText waits (bounded) for the automation turn's user message
// carrying needle, then returns it ("" on deadline — the caller fails).
func matrixCronUserText(t *testing.T, r *Runner, sessID, needle string) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		lines, err := r.sessions[sessID].Manager.ReadAll()
		if err == nil {
			for i := range lines {
				if lines[i].Type != session.TypeUserMessage || !strings.Contains(string(lines[i].Content), needle) {
					continue
				}

				var blocks []session.ContentBlock
				if json.Unmarshal(lines[i].Content, &blocks) == nil {
					var sb strings.Builder
					for j := range blocks {
						sb.WriteString(blocks[j].Text)
					}

					return sb.String()
				}
			}
		}

		time.Sleep(20 * time.Millisecond)
	}

	return ""
}

// provSawText reports whether any captured provider request carried needle
// (the scriptedACPProvider's received-prompt lens).
func provSawText(p *scriptedACPProvider, needle string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, has := range p.streamedHasText {
		if has(needle) {
			return true
		}
	}

	return false
}

// TestModesMatrixCronCommands (automation x commands): the automation's
// prompt is /matrix-echo with args; the firing turn expands it through the
// adapter's seam and BOTH lenses (transcript user message + captured
// provider request) carry the fixture command's body with the args
// substituted — the D-13 functional outcome.
func TestModesMatrixCronCommands(t *testing.T) { // HOME-pinned leg
	t.Setenv("HOME", t.TempDir())

	project := t.TempDir()

	modesmatrix.MountFixture(t, project)

	r, prov := newMatrixRunner(t, project, scriptedResp{text: "automation commands turn done"})

	fireMatrixAutomation(t, r, project, "sess-matrix-cron-c", "/matrix-echo cron-args")

	const marker = "MATRIX-ECHO-EXPANSION invoked: cron-args"

	if got := matrixCronUserText(t, r, "sess-matrix-cron-c", marker); got == "" {
		t.Fatal("the fired turn's user message never carried the expanded command body")
	}

	if !provSawText(prov, marker) {
		t.Error("no captured provider request carried the expanded command body (the model never saw the expansion)")
	}

	cronRegistry.Record(
		modesmatrix.ModeAutomation, modesmatrix.SurfaceCommands, modesmatrix.StatusPass,
		"runAutomationTurn over a sched.Create'd automation; /matrix-echo expanded "+
			"(transcript + provider lenses agree)",
	)

	cronRegistry.ReportT(t)
}

// TestModesMatrixCronSkills (automation x skills): the same firing path
// over /matrix-skill — the SKILL.md body expands with the args substituted.
func TestModesMatrixCronSkills(t *testing.T) { // HOME-pinned leg
	t.Setenv("HOME", t.TempDir())

	project := t.TempDir()

	modesmatrix.MountFixture(t, project)

	r, prov := newMatrixRunner(t, project, scriptedResp{text: "automation skills turn done"})

	fireMatrixAutomation(t, r, project, "sess-matrix-cron-s", "/matrix-skill cron-skill-args")

	const marker = "MATRIX-SKILL-BODY invoked: cron-skill-args"

	if got := matrixCronUserText(t, r, "sess-matrix-cron-s", marker); got == "" {
		t.Fatal("the fired turn's user message never carried the expanded skill body")
	}

	if !provSawText(prov, marker) {
		t.Error("no captured provider request carried the expanded skill body (the model never saw the expansion)")
	}

	cronRegistry.Record(
		modesmatrix.ModeAutomation, modesmatrix.SurfaceSkills, modesmatrix.StatusPass,
		"same firing path over /matrix-skill; SKILL.md body expanded with args substituted",
	)

	cronRegistry.ReportT(t)
}

// TestModesMatrixCronHooks (automation x hooks): the automation turn's
// scripted Read tool call consults the gate head, which executes the
// fixture's PreToolUse hook — the marker file inside the temp project
// receives the hook's stdin JSON (tool_name Read, the automation session's
// id).
func TestModesMatrixCronHooks(t *testing.T) { // HOME-pinned leg
	t.Setenv("HOME", t.TempDir())

	project := t.TempDir()

	modesmatrix.MountFixture(t, project)

	notes := filepath.Join(project, "notes.md")
	if err := os.WriteFile(notes, []byte("the automation turn reads this\n"), 0o600); err != nil {
		t.Fatalf("write notes.md: %v", err)
	}

	readInput, merr := json.Marshal(map[string]string{"file_path": notes})
	if merr != nil {
		t.Fatalf("marshal read input: %v", merr)
	}

	r, _ := newMatrixRunner(t, project,
		scriptedResp{
			toolCalls: []provider.ToolCall{{Name: toolNameRead, Input: readInput}},
			finish:    chunkToolUse,
		},
		scriptedResp{text: "automation hooks turn done"},
	)

	fireMatrixAutomation(t, r, project, "sess-matrix-cron-h", "run the matrix automation hook turn")

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) && len(matrixMarkerEvents(t, project, "PreToolUse")) == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	pre := matrixMarkerEvents(t, project, "PreToolUse")
	if len(pre) == 0 {
		t.Fatal("no PreToolUse payload in the marker file — the fixture hook never fired inside the automation turn")
	}

	if got := pre[0]["tool_name"]; got != toolNameRead {
		t.Errorf("PreToolUse tool_name = %v; want %s", got, toolNameRead)
	}

	cronRegistry.Record(
		modesmatrix.ModeAutomation, modesmatrix.SurfaceHooks, modesmatrix.StatusPass,
		"automation turn's Read consulted the gate head; fixture PreToolUse executed "+
			"and the marker file carries the stdin JSON",
	)

	cronRegistry.ReportT(t)
}

// TestModesMatrixCronEmpty (the empty-input row, automation leg): with NO
// fixture mounted, the same automation prompt falls through UNEXPANDED (the
// typed invocation is the user message verbatim) and the marker file stays
// ABSENT — absence is the pass condition, never an error.
func TestModesMatrixCronEmpty(t *testing.T) { // HOME-pinned leg
	t.Setenv("HOME", t.TempDir())

	project := t.TempDir() // deliberately NO fixture

	r, _ := newMatrixRunner(t, project, scriptedResp{text: "empty fixture turn done"})

	fireMatrixAutomation(t, r, project, "sess-matrix-cron-e", "/matrix-echo cron-args")

	const emptyRowInvocation = "/matrix-echo cron-args"

	if got := matrixCronUserText(t, r, "sess-matrix-cron-e", emptyRowInvocation); got != emptyRowInvocation {
		t.Fatalf("empty row: user message = %q; want the typed invocation verbatim (fallthrough)", got)
	}

	if _, err := os.Stat(modesmatrix.MarkerPath(project)); !os.IsNotExist(err) {
		t.Error("empty row: marker file exists; want ABSENT (no fixture hooks anywhere)")
	}

	t.Log("MODES-MATRIX empty-row automation: invocation fell through verbatim; marker file ABSENT")
}

// matrixMarkerEvents parses the fixture marker file inside project and
// returns the parsed payloads whose hook_event_name equals eventName.
func matrixMarkerEvents(t *testing.T, project, eventName string) []map[string]any {
	t.Helper()

	lines, err := modesmatrix.MarkerLines(project)
	if err != nil {
		t.Fatalf("marker read: %v", err)
	}

	var out []map[string]any

	for _, line := range lines {
		var payload map[string]any
		if json.Unmarshal([]byte(line), &payload) != nil {
			continue
		}

		if payload["hook_event_name"] == eventName {
			out = append(out, payload)
		}
	}

	return out
}
