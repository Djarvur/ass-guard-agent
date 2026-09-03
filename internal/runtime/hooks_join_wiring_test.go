package runtime //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// 21-06 join wiring tests: hooks PreToolUse verdicts join Phase 17's ONE
// gate pipeline at the sessionFor composition — a project settings.json deny
// hook blocks a REAL turn through the real discovery + gate wiring, and the
// 21-04 Read-rule consult seam is backed by the SAME perm rule set the gate
// consumes (mentions and tool calls answer to one rule authority).

// hookJoinDenyReason is the project-scope deny hook's pinned reason.
const hookJoinDenyReason = "denied by project policy"

// plantProjectDenySettings plants <dir>/.claude/settings.json carrying one
// project-scope PreToolUse deny hook for Bash (the real settings tree the
// loader discovers — the same file CC reads).
func plantProjectDenySettings(t *testing.T, dir string) {
	t.Helper()

	body := map[string]any{
		"hooks": map[string][]map[string]any{
			"PreToolUse": {
				{
					"matcher": "Bash",
					"hooks": []map[string]any{
						{
							"type": "command",
							"command": "printf '%s' '" + `{"hookSpecificOutput":{"permissionDecision":"deny",` +
								`"permissionDecisionReason":"` + hookJoinDenyReason + `"}}'`,
						},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}

	p := filepath.Join(dir, ".claude", "settings.json")

	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
}

// newHookJoinRunner builds a Runner over a temp workDir with the planted
// tree, the command registry loaded (the real ecosys.Discover picks up the
// planted settings.json hooks), HOME pinned hermetic.
func newHookJoinRunner(
	t *testing.T, plant func(dir string), script ...scriptedResp,
) (*Runner, *scriptedACPProvider) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())

	bus := event.NewBus()
	prov := &scriptedACPProvider{}
	prov.queue(script...)

	dir := t.TempDir()

	if plant != nil {
		plant(dir)
	}

	r := &Runner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      4,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}

	if err := r.SetupEngine(); err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	r.LoadCommandRegistry()

	return r, prov
}

// TestHookJoin_ProjectDenyThroughComposition (criterion 1's spine through
// the REAL settings tree + the REAL composition): a project-scope
// settings.json PreToolUse deny hook, discovered by the real loader and
// wired into the gate deps at sessionFor, blocks the tool call at the
// chokepoint — the transcript carries the STRUCTURED gate denial with the
// hook's reason (not the executor's legacy "hook:" form), and the command
// never runs.
func TestHookJoin_ProjectDenyThroughComposition(t *testing.T) { //nolint:paralleltest // HOME pin
	const (
		sessID  = "sess-hookjoin-1"
		callID  = "call_hookjoin_1"
		marker  = "hookjoin-must-not-run"
		cmdText = "echo " + marker
	)

	r, _ := newHookJoinRunner(t, func(dir string) { plantProjectDenySettings(t, dir) },
		scriptedResp{toolCalls: []provider.ToolCall{{
			ID: callID, Name: "Bash",
			Input: json.RawMessage(`{"command":"` + cmdText + `","description":"deny me"}`),
		}}},
		scriptedResp{text: "finished"},
	)

	prompt := []acp.ContentBlock{{Type: blockText, Text: "run the command"}}

	//nolint:contextcheck // test-scoped background ctx
	if _, err := r.Run(context.Background(), sessID, &noopEmitter{}, prompt); err != nil {
		t.Fatalf("Run: %v", err)
	}

	sess := r.sessions[sessID]
	if sess == nil {
		t.Fatal("session missing after Run")
	}

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var result *session.Line

	for i := range lines {
		if lines[i].Type == session.TypeToolResult && lines[i].ToolCallID == callID {
			result = &lines[i]

			break
		}
	}

	if result == nil {
		t.Fatal("no tool_result for the hook-denied Bash call")
	}

	if !result.IsError {
		t.Fatalf("hook-deny result must be an error result, got output=%s", result.Output)
	}

	if !strings.Contains(string(result.Output), "Permission denied") ||
		!strings.Contains(string(result.Output), hookJoinDenyReason) {
		t.Errorf("hook-deny form = %s; want the structured gate denial carrying the hook's reason", result.Output)
	}

	if strings.Contains(string(result.Output), "Permission denied by the project permission rules") {
		t.Errorf("hook-deny form = %s; must be the HOOK denial form, not the rule-deny form", result.Output)
	}

	// The command never ran: scan every transcript line for the marker.
	for i := range lines {
		if strings.Contains(string(lines[i].Output), marker) {
			t.Errorf("the hook-denied command executed (marker found in the transcript)")
		}
	}
}

// TestHookJoin_ReadRuleEvaluatorBackedByPerm (the PAR-06 ↔ PAR-03 join):
// after sessionFor wires the gate, the 21-04 Read-rule consult seam is
// backed by the SAME perm rule set the gate consumes — a permissions.yaml
// deny on Read turns an @file mention into the loud denied note, and an ask
// rule does NOT (only VerdictDeny denies a mention).
func TestHookJoin_ReadRuleEvaluatorBackedByPerm(t *testing.T) { //nolint:paralleltest // HOME pin
	plant := func(rules string) func(dir string) {
		return func(dir string) {
			mentionFixture(t, dir)

			p := filepath.Join(dir, ".ass-guard", "permissions.yaml")

			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatalf("mkdir .ass-guard: %v", err)
			}

			if err := os.WriteFile(p, []byte(rules), 0o600); err != nil {
				t.Fatalf("write permissions.yaml: %v", err)
			}
		}
	}

	t.Run("deny rule denies the mention", func(t *testing.T) {
		r, _ := memRunner(t, plant("deny:\n  - Read\n"))

		sess := r.sessionFor(context.Background(), "sess-readrule-deny")
		if sess == nil {
			t.Fatal("sessionFor returned nil")
		}

		if r.permStore.Load() == nil {
			t.Fatal("the Runner-scoped perm store was not opened (mention seam still implicit-allow)")
		}

		got := r.expandUserBlocks(sess, []session.ContentBlock{
			{Type: blockText, Text: "review @notes.md please"},
		})

		text := got[0].Text
		if !strings.Contains(text, "[@notes.md] could not be expanded:") {
			t.Errorf("perm-denied Read rule did not deny the mention:\n%q", text)
		}

		if strings.Contains(text, mentionFileBody) {
			t.Errorf("denied @file content leaked into the prompt:\n%q", text)
		}

		prov := mentionProvenance(t, sess)
		if len(prov) != 1 || prov[0].Text != "denied" {
			t.Errorf("provenance = %+v; want one denied-form line", prov)
		}
	})

	t.Run("ask rule does not deny the mention (only VerdictDeny denies)", func(t *testing.T) {
		r, _ := memRunner(t, plant("ask:\n  - Read\n"))

		sess := r.sessionFor(context.Background(), "sess-readrule-ask")
		if sess == nil {
			t.Fatal("sessionFor returned nil")
		}

		if r.permStore.Load() == nil {
			t.Fatal("the Runner-scoped perm store was not opened")
		}

		got := r.expandUserBlocks(sess, []session.ContentBlock{
			{Type: blockText, Text: "review @notes.md please"},
		})

		if !strings.Contains(got[0].Text, mentionFileBody) {
			t.Errorf("ask rule must not deny a mention (only VerdictDeny does):\n%q", got[0].Text)
		}
	})
}

// TestHookJoin_OneLiveRuleAuthority (21-REVIEW WR-03): the perm store is
// RUNNER-scoped — session B's reject_always dialog click (Store.ForbidTool,
// the gate's Forbid seam) lands in the SAME live instance session A's gate
// and the @-mention Read-rule consult answer to. The old wiring pinned the
// FIRST-wired session's store into the mention seam forever, so the click
// governed B's gate while A's mention gating stayed on the stale snapshot —
// a Read path rejected in B could still expand via @mention in B.
func TestHookJoin_OneLiveRuleAuthority(t *testing.T) { //nolint:paralleltest // HOME pin
	r, _ := memRunner(t, func(dir string) { mentionFixture(t, dir) })

	sessA := r.sessionFor(context.Background(), "sess-authority-a")
	if sessA == nil {
		t.Fatal("sessionFor A returned nil")
	}

	// Before any rule exists: the mention expands (implicit allow).
	before := r.expandUserBlocks(sessA, []session.ContentBlock{
		{Type: blockText, Text: "review @notes.md"},
	})
	if !strings.Contains(before[0].Text, mentionFileBody) {
		t.Fatalf("pre-rule mention did not expand:\n%q", before[0].Text)
	}

	// Session B is constructed AFTER A wired the seam — the divergence case.
	if sessB := r.sessionFor(context.Background(), "sess-authority-b"); sessB == nil {
		t.Fatal("sessionFor B returned nil")
	}

	st := r.permStore.Load()
	if st == nil {
		t.Fatal("the Runner-scoped perm store was never opened by sessionFor")
	}

	// The reject_always click: the dialog write the gate's Forbid seam makes.
	if err := st.ForbidTool("Read"); err != nil {
		t.Fatalf("ForbidTool (the reject_always click): %v", err)
	}

	after := r.expandUserBlocks(sessA, []session.ContentBlock{
		{Type: blockText, Text: "review @notes.md"},
	})
	if !strings.Contains(after[0].Text, "[@notes.md] could not be expanded:") {
		t.Fatalf("session B's reject_always click did not govern session A's mention gating "+
			"(divergent rule authorities):\n%q", after[0].Text)
	}

	if strings.Contains(after[0].Text, mentionFileBody) {
		t.Fatalf("denied @file content leaked into the prompt:\n%q", after[0].Text)
	}
}

// TestHookJoin_MentionSeamRaceFree (21-REVIEW WR-02): the old wiring wrote
// readRuleEvaluator inside sessionFor (under sessMu) while turn-time mention
// consults read the field with NO lock — a session B construction could race
// session A's in-flight readAllows with no happens-before edge. The hoisted
// store is published through r.permStore (atomic) and the evaluator field is
// production-read-only, so this hammer — concurrent sessionFor construction
// against live mention resolution — must stay clean under -race.
func TestHookJoin_MentionSeamRaceFree(t *testing.T) { //nolint:paralleltest // HOME pin
	r, dir := memRunner(t, func(dir string) { mentionFixture(t, dir) })

	target := filepath.Join(dir, "notes.md")

	var wg sync.WaitGroup

	// The writer side: concurrent session construction (the async catch-up
	// firing racing the first Run — 12-07's reachable sequence).
	for i := range 4 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if s := r.sessionFor(context.Background(), fmt.Sprintf("sess-race-%d", i)); s == nil {
				t.Error("sessionFor returned nil under concurrency")
			}
		}()
	}

	// The reader side: lock-free turn-time consults while construction runs.
	wg.Add(1)

	go func() {
		defer wg.Done()

		for range 200 {
			_ = r.readAllows(target)
		}
	}()

	wg.Wait()

	if r.permStore.Load() == nil {
		t.Fatal("the shared perm store never opened under concurrent sessionFor")
	}
}
