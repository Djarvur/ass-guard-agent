package ecosys //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHooksParseFromFixture (Task 4, Test 1) verifies the committed fixture
// plugin's hooks/hooks.json parses into Registry.Hooks end-to-end: mapped
// events carry event/matcher/command; UNMAPPED events (Notification,
// PreCompact) parse too — present in the registry with an observe-only
// warning naming the event.
func TestHooksParseFromFixture(t *testing.T) {
	buf := captureShadowLogger(t)

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	proj := plantProjectPlugins(t)

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	byEvent := map[string][]HookConfig{}
	for _, h := range reg.Hooks {
		byEvent[h.Event] = append(byEvent[h.Event], h)
	}

	require.NotEmpty(t, byEvent["PreToolUse"], "PreToolUse hooks must parse")
	pre := byEvent["PreToolUse"][0]
	assert.Equal(t, "Bash", pre.Matcher)
	assert.Equal(t, "echo fixture-pretool", pre.Command)
	assert.Equal(t, 5, pre.TimeoutSec)
	assert.Contains(t, pre.Path, "hooks", "provenance names the hooks.json")

	require.NotEmpty(t, byEvent["PostToolUse"], "PostToolUse hooks must parse")
	assert.Equal(t, ".*", byEvent["PostToolUse"][0].Matcher, "regex matcher preserved verbatim")

	// Unmapped events parse (observe-only) and warn.
	require.NotEmpty(t, byEvent["Notification"], "Notification hooks must parse")
	require.NotEmpty(t, byEvent["PreCompact"], "PreCompact hooks must parse")

	warned := buf.String()
	assert.Contains(t, warned, "Notification", "unmapped event must warn (observe-only)")
	assert.Contains(t, warned, "PreCompact", "unmapped event must warn (observe-only)")
}

// TestHookMatcher verifies the matcher semantics: a tool-name REGEX (empty or
// "*" matches everything); hooks bound to tool events do not fire on non-tool
// events.
func TestHookMatcher(t *testing.T) {
	t.Parallel()

	r := NewHookRunner([]HookConfig{
		{Event: "PreToolUse", Matcher: "Bash|^Edit", Command: "echo matched"},
		{Event: "PreToolUse", Matcher: "", Command: "echo catchall"},
	}, "s", t.TempDir(), "")

	cases := map[string]int{ // tool → matching hook count
		"Bash":  2, // the regex AND the catchall
		"Edit":  2,
		"Read":  1, // catchall only
		"Fetch": 1,
	}

	for tool, want := range cases {
		assert.Len(t, r.matchingHooks("PreToolUse", tool), want, "tool %s", tool)
	}

	assert.Empty(t, r.matchingHooks("UserPromptSubmit", ""),
		"hooks bound to tool events do not fire on non-tool events")
}

// TestPreToolUseStdinAndExitRouting (Task 4, Test 2) verifies the documented
// stdin payload and exit-code routing: `cat` as the hook command proves the
// payload carries session_id, hook_event_name, tool_name, tool_input, cwd,
// transcript_path; exit 0 proceeds (stdout captured); exit 2 REFUSES with the
// hook's stderr as the refusal message.
func TestPreToolUseStdinAndExitRouting(t *testing.T) {
	t.Parallel()

	work := t.TempDir()

	r := NewHookRunner([]HookConfig{
		{Event: "PreToolUse", Matcher: "Bash", Command: "cat"},
	}, "sess-1", work, "/transcript/audit.jsonl")

	proceed, payload := r.PreToolUse(context.Background(), toolBash, json.RawMessage(`{"command":"ls"}`))

	assert.True(t, proceed, "exit 0 must proceed")

	for _, field := range []string{
		`"session_id":"sess-1"`,
		`"hook_event_name":"PreToolUse"`,
		`"tool_name":"Bash"`,
		`"command":"ls"`,
		`"cwd":"` + work + `"`,
		`"transcript_path":"/transcript/audit.jsonl"`,
	} {
		assert.Contains(t, payload, field, "stdin payload must carry %s", field)
	}

	// Exit 2 → refusal with the hook's stderr as the message.
	r2 := NewHookRunner([]HookConfig{
		{Event: "PreToolUse", Matcher: "Bash", Command: `echo refuse-reason >&2; exit 2`},
	}, "sess-1", work, "")

	proceed, msg := r2.PreToolUse(context.Background(), toolBash, json.RawMessage(`{}`))
	assert.False(t, proceed, "exit 2 on PreToolUse must REFUSE the call")
	assert.Contains(t, msg, "refuse-reason", "the refusal message is the hook's stderr")
}

// TestPostToolUsePayload verifies PostToolUse receives tool_output in the
// payload (observed via `cat`) and never blocks.
func TestPostToolUsePayload(t *testing.T) {
	t.Parallel()

	r := NewHookRunner([]HookConfig{
		{Event: "PostToolUse", Matcher: "Bash", Command: "cat"},
	}, "s", t.TempDir(), "")

	out := r.Fire(context.Background(), "PostToolUse", map[string]any{
		"tool_name":     toolBash,
		"tool_input":    json.RawMessage(`{"command":"ls"}`),
		"tool_response": json.RawMessage(`"file.txt"`),
	})

	assert.True(t, out.Proceed, "PostToolUse never blocks")
	assert.Contains(t, out.Message, `"tool_name":"Bash"`)
	assert.Contains(t, out.Message, `"tool_response":"file.txt"`,
		"the payload carries the tool output as tool_response")
}

// TestHookBoundaries (Task 4, Test 4) verifies the execution bounds: sanitized
// env (no arbitrary parent vars leak; CLAUDE_PLUGIN_ROOT IS exported), cwd =
// the session workdir, output size-capped, and a hanging hook hits its timeout
// and is skipped with a warning (never fatal).
func TestHookBoundaries(t *testing.T) {
	buf := captureShadowLogger(t)

	t.Setenv("HOOKTEST_SECRET", "leaky") // must NOT reach the hook env

	work := t.TempDir()

	// Env sanitized: `env` output lacks the planted var; CLAUDE_PLUGIN_ROOT set.
	r := NewHookRunner([]HookConfig{
		{Event: "UserPromptSubmit", Command: "env", PluginRoot: "/plug/root"},
	}, "s", work, "")

	out := r.Fire(context.Background(), "UserPromptSubmit", map[string]any{"prompt": "hi"})
	assert.NotContains(t, out.Message, "HOOKTEST_SECRET", "sanitized env must not leak parent vars")
	assert.Contains(t, out.Message, "CLAUDE_PLUGIN_ROOT=/plug/root",
		"CLAUDE_PLUGIN_ROOT must be exported to the hook env")

	// cwd is the session workdir.
	r2 := NewHookRunner([]HookConfig{{Event: "UserPromptSubmit", Command: "pwd"}}, "s", work, "")
	assert.Contains(t, r2.Fire(context.Background(), "UserPromptSubmit", nil).Message, work,
		"the hook runs with cwd = the session workdir")

	// Output size cap.
	r3 := NewHookRunner([]HookConfig{{Event: "UserPromptSubmit", Command: "head -c 200000 /dev/zero"}}, "s", work, "")
	capped := r3.Fire(context.Background(), "UserPromptSubmit", nil)
	assert.LessOrEqual(t, len(capped.Message), hookOutputCap+2, "output must be size-capped")

	// Timeout: a hanging hook is skipped with a warning, never fatal.
	buf.Reset()

	r4 := NewHookRunner([]HookConfig{{Event: "UserPromptSubmit", Command: "sleep 30", TimeoutSec: 1}}, "s", work, "")
	timed := r4.Fire(context.Background(), "UserPromptSubmit", nil)

	assert.True(t, timed.Proceed, "a timed-out hook must never block the turn")
	assert.Contains(t, buf.String(), "timeout", "the timeout skip warns (audit-loud)")

	// A failing hook (exit 1) also proceeds with a warning.
	buf.Reset()

	r5 := NewHookRunner([]HookConfig{{Event: "UserPromptSubmit", Command: "exit 1"}}, "s", work, "")
	failed := r5.Fire(context.Background(), "UserPromptSubmit", nil)

	assert.True(t, failed.Proceed, "a failed hook must never block the turn")
	assert.Contains(t, buf.String(), "hook", "the failure skip warns")
}

// TestUnmappedEventObserveOnly verifies firing an unmapped event is a warned
// no-op (parse + log + degrade — never silently ignored).
func TestUnmappedEventObserveOnly(t *testing.T) {
	buf := captureShadowLogger(t)

	r := NewHookRunner([]HookConfig{
		{Event: "Notification", Command: "echo should-not-run"},
	}, "s", t.TempDir(), "")

	out := r.Fire(context.Background(), "Notification", nil)

	assert.True(t, out.Proceed)
	assert.Empty(t, out.Message, "unmapped events execute nothing")
	assert.Contains(t, buf.String(), "Notification", "the unmapped firing warns")
}

// TestFireNeverPanics verifies a hook whose binary is missing degrades to a
// warning (the registry keeps loading; the turn proceeds).
func TestFireNeverPanics(t *testing.T) {
	buf := captureShadowLogger(t)

	r := NewHookRunner([]HookConfig{
		{Event: "Stop", Command: "definitely-not-a-real-binary-xyz --flag"},
	}, "s", t.TempDir(), "")

	assert.NotPanics(t, func() {
		out := r.Fire(context.Background(), "Stop", nil)
		assert.True(t, out.Proceed)
	})

	assert.Contains(t, buf.String(), "Stop", "the missing-binary skip warns")
}
