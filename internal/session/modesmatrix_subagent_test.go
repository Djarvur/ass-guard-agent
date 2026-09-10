package session //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/modesmatrix"
	"github.com/Djarvur/ass-guard-agent/kit/provider"
)

// The ECOS-04 (TAIL-03, 24-05) subagent-mode leg: a REAL DispatchSubagent
// (the Task tool_call through Session.Prompt — the live nested-turn rails)
// with the modes-matrix fixture plugin mounted in the host project. Per
// cell, D-12/D-13's bar:
//
//   - hooks: the fixture's SubagentStop hook FIRES when the dispatch
//     completes — the marker file receives the hook's stdin JSON (the
//     observed effect, not discovery).
//   - commands/skills: the functional probe observes what the nested turn
//     actually carries — the current live path pins the invocation VERBATIM
//     in the subagent's user message (expansion is parent-side by design;
//     the nested loop does not run expandUserBlocks), while the fixture's
//     command/skill are DISCOVERED in the registry the dispatch rides.
//     Classified by observation (the plan's probe rule), never guessed.
//
// Determinism: HOME pinned to a temp dir (no operator user-scope hooks can
// fire); scripted provider responses; transcript-as-truth assertions.

// subagentRegistry is this test binary's cell registry.
var subagentRegistry = modesmatrix.NewRegistry() //nolint:gochecknoglobals // per-test-binary registry

// matrixUserTexts returns every user_message line's assembled text — the
// transcript lens over what turns (parent and nested subagent alike) sent.
func matrixUserTexts(t *testing.T, s *Session) []string {
	t.Helper()

	lines, err := s.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var out []string

	for i := range lines {
		if lines[i].Type != TypeUserMessage {
			continue
		}

		var blocks []ContentBlock
		if json.Unmarshal(lines[i].Content, &blocks) != nil {
			continue
		}

		var sb strings.Builder
		for j := range blocks {
			sb.WriteString(blocks[j].Text)
		}

		out = append(out, sb.String())
	}

	return out
}

// matrixSubagentMarkerHooks parses the fixture marker file inside project
// and returns the parsed payloads for event.
func matrixSubagentMarkerHooks(t *testing.T, project, event string) []map[string]any {
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

		if payload["hook_event_name"] == event {
			out = append(out, payload)
		}
	}

	return out
}

// TestModesMatrixSubagent (24-05 Task 2) drives TWO real subagent dispatches
// (one per invocation surface) plus the SubagentStop hook observation, all
// through the live nested-turn rails with the fixture mounted.
func TestModesMatrixSubagent(t *testing.T) { //nolint:funlen,cyclop // HOME-pinned leg; three cells, one story
	t.Setenv("HOME", t.TempDir()) // hermetic user-scope discovery

	project := t.TempDir()

	modesmatrix.MountFixture(t, project)

	// Discovery lens (the registry the dispatch rides): the fixture's
	// command and skill are installed-plugin contributions.
	reg, _, err := ecosys.Discover(project)
	if err != nil {
		t.Fatalf("ecosys.Discover: %v", err)
	}

	cmd, cmdFound := reg.Commands["matrix-echo"]
	if !cmdFound {
		t.Fatal("fixture command matrix-echo not discovered (commands surface absent from the mounted registry)")
	}

	if !strings.Contains(cmd.Path, "modes-matrix") {
		t.Errorf("matrix-echo resolved from %q; want the mounted fixture plugin path", cmd.Path)
	}

	skill, skillFound := reg.Skills["matrix-skill"]
	if !skillFound {
		t.Fatal("fixture skill matrix-skill not discovered (skills surface absent from the mounted registry)")
	}

	if !strings.Contains(skill.Path, "modes-matrix") {
		t.Errorf("matrix-skill resolved from %q; want the mounted fixture plugin path", skill.Path)
	}

	// The live dispatch: the parent model calls Task twice (one per
	// surface); each subagent answers with plain text and completes.
	s, bus, _ := newSubagentSession(t, []provider.Response{
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{Name: toolTask, Input: json.RawMessage(`{"prompt":"/matrix-echo sub-args"}`)},
		}},
		{FinishReason: stopEndTurn}, // subagent 1's answer
		{FinishReason: blockToolUse, ToolCalls: []provider.ToolCall{
			{Name: toolTask, Input: json.RawMessage(`{"prompt":"/matrix-skill sub-skill-args"}`)},
		}},
		{FinishReason: stopEndTurn}, // subagent 2's answer
		{FinishReason: stopEndTurn}, // parent continuation ends the turn
	})

	s.WorkDir = project
	s.Hooks = ecosys.NewHookRunner(reg.Hooks, s.SessionID, project, s.Manager.Path())

	_ = bus // chunks flow via the bus (PARA-02); the cells' lenses are transcript + marker

	if _, perr := s.Prompt(context.Background(), []ContentBlock{
		{Type: blockText, Text: "dispatch the matrix subagent probes"},
	}); perr != nil {
		t.Fatalf("Prompt: %v", perr)
	}

	texts := matrixUserTexts(t, s)

	// Commands cell — the OBSERVED live behavior: the nested turn carries
	// the invocation VERBATIM (parent-side expansion; pinned, not guessed).
	cmdVerbatim := false

	for i := range texts {
		if texts[i] == "/matrix-echo sub-args" {
			cmdVerbatim = true
		}
	}

	if !cmdVerbatim {
		t.Error("the subagent's user message does not carry '/matrix-echo sub-args' verbatim " +
			"(nested-path carry broken)")
	}

	subagentRegistry.Record(
		modesmatrix.ModeSubagent, modesmatrix.SurfaceCommands, modesmatrix.StatusPass,
		"dispatch through real DispatchSubagent; nested turn carries /matrix-echo verbatim "+
			"(expansion parent-side — pinned live behavior); fixture command discovered from the plugin",
	)

	// Skills cell — same probe, the skill surface.
	skillVerbatim := false

	for i := range texts {
		if texts[i] == "/matrix-skill sub-skill-args" {
			skillVerbatim = true
		}
	}

	if !skillVerbatim {
		t.Error("the subagent's user message does not carry '/matrix-skill sub-skill-args' verbatim " +
			"(nested-path carry broken)")
	}

	subagentRegistry.Record(
		modesmatrix.ModeSubagent, modesmatrix.SurfaceSkills, modesmatrix.StatusPass,
		"second dispatch carries /matrix-skill verbatim (same pinned nested-path behavior); "+
			"fixture skill discovered from the plugin",
	)

	// Hooks cell — D-13's functional bar: SubagentStop FIRED on dispatch
	// completion and the marker file received the hook's stdin JSON.
	deadline := time.Now().Add(5 * time.Second)

	subStops := matrixSubagentMarkerHooks(t, project, "SubagentStop")

	for time.Now().Before(deadline) && len(subStops) == 0 {
		time.Sleep(20 * time.Millisecond)

		subStops = matrixSubagentMarkerHooks(t, project, "SubagentStop")
	}

	if len(subStops) == 0 {
		t.Fatal("no SubagentStop payload in the marker file — the fixture hook never fired on dispatch completion")
	}

	if got := subStops[0]["session_id"]; got != s.SessionID {
		t.Errorf("SubagentStop session_id = %v; want %q", got, s.SessionID)
	}

	subagentRegistry.Record(
		modesmatrix.ModeSubagent, modesmatrix.SurfaceHooks, modesmatrix.StatusPass,
		"SubagentStop fired per dispatch completion through the real rails; "+
			"marker file carries the hook's stdin JSON",
	)

	subagentRegistry.ReportT(t)
}
