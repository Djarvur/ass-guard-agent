package ecosys //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	require.NotEmpty(t, byEvent[hookEventPreToolUse], "PreToolUse hooks must parse")
	pre := byEvent["PreToolUse"][0]
	assert.Equal(t, "Bash", pre.Matcher)
	assert.Equal(t, "echo fixture-pretool", pre.Command)
	assert.Equal(t, 5, pre.TimeoutSec)
	assert.Contains(t, pre.Path, "hooks", "provenance names the hooks.json")

	require.NotEmpty(t, byEvent[hookEventPostToolUse], "PostToolUse hooks must parse")
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
		{Event: hookEventPreToolUse, Matcher: toolBash + "|^Edit", Command: "echo matched"},
		{Event: hookEventPreToolUse, Matcher: "", Command: "echo catchall"},
	}, "s", t.TempDir(), "")

	cases := map[string]int{ // tool → matching hook count
		toolBash: 2, // the regex AND the catchall
		"Edit":   2,
		toolRead: 1, // catchall only
		"Fetch":  1,
	}

	for tool, want := range cases {
		assert.Len(t, r.matchingHooks(hookEventPreToolUse, tool), want, "tool %s", tool)
	}

	assert.Empty(t, r.matchingHooks(hookEventUserPromptSubmit, ""),
		"hooks bound to tool events do not fire on non-tool events")
}

// TestPreToolUseStdinAndExitRouting (Task 4, Test 2) verifies the documented
// stdin payload and exit-code routing: `cat` as the hook command proves the
// payload carries session_id, hook_event_name, tool_name, tool_input, cwd,
// transcript_path; exit 0 proceeds; exit 2 REFUSES with the hook's stderr as
// the refusal message. (21-01: the payload observation rides Fire. 21-06:
// the boolean PreToolUse pair was deleted at the gate join — the refusal
// assertion rides PreToolUseVerdict, the surface the gate head consumes.)
func TestPreToolUseStdinAndExitRouting(t *testing.T) {
	t.Parallel()

	work := t.TempDir()

	r := NewHookRunner([]HookConfig{
		{Event: hookEventPreToolUse, Matcher: toolBash, Command: "cat"},
	}, "sess-1", work, "/transcript/audit.jsonl")

	payload := r.Fire(context.Background(), hookEventPreToolUse, map[string]any{
		keyToolName:  toolBash,
		keyToolInput: json.RawMessage(`{"command":"ls"}`),
	})

	for _, field := range []string{
		`"session_id":"sess-1"`,
		`"hook_event_name":"` + hookEventPreToolUse + `"`,
		`"` + keyToolName + `":"` + toolBash + `"`,
		`"command":"ls"`,
		`"cwd":"` + work + `"`,
		`"transcript_path":"/transcript/audit.jsonl"`,
	} {
		assert.Contains(t, payload.Message, field, "stdin payload must carry %s", field)
	}

	// Exit 2 → deny verdict with the hook's stderr as the reason (the
	// legacy channel, D-02 — stderr-first extraction in the composed path).
	r2 := NewHookRunner([]HookConfig{
		{Event: hookEventPreToolUse, Matcher: toolBash, Command: `echo refuse-reason >&2; exit 2`},
	}, "sess-1", work, "")

	v, reason := r2.PreToolUseVerdict(context.Background(), toolBash, json.RawMessage(`{}`))
	assert.Equal(t, VerdictDeny, v, "exit 2 on PreToolUse must DENY the call")
	assert.Contains(t, reason, "refuse-reason", "the denial reason is the hook's stderr")
}

// TestPostToolUsePayload verifies PostToolUse receives tool_output in the
// payload (observed via `cat`) and never blocks.
func TestPostToolUsePayload(t *testing.T) {
	t.Parallel()

	r := NewHookRunner([]HookConfig{
		{Event: hookEventPostToolUse, Matcher: toolBash, Command: "cat"},
	}, "s", t.TempDir(), "")

	out := r.Fire(context.Background(), hookEventPostToolUse, map[string]any{
		keyToolName:     toolBash,
		keyToolInput:    json.RawMessage(`{"command":"ls"}`),
		keyToolResponse: json.RawMessage(`"file.txt"`),
	})

	assert.True(t, out.Proceed, "PostToolUse never blocks")
	assert.Contains(t, out.Message, `"`+keyToolName+`":"`+toolBash+`"`)
	assert.Contains(t, out.Message, `"`+keyToolResponse+`":"file.txt"`,
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
		{Event: hookEventUserPromptSubmit, Command: "env", PluginRoot: "/plug/root"},
	}, "s", work, "")

	out := r.Fire(context.Background(), hookEventUserPromptSubmit, map[string]any{"prompt": "hi"})
	assert.NotContains(t, out.Message, "HOOKTEST_SECRET", "sanitized env must not leak parent vars")
	assert.Contains(t, out.Message, "CLAUDE_PLUGIN_ROOT=/plug/root",
		"CLAUDE_PLUGIN_ROOT must be exported to the hook env")

	// cwd is the session workdir.
	r2 := NewHookRunner([]HookConfig{{Event: hookEventUserPromptSubmit, Command: "pwd"}}, "s", work, "")
	assert.Contains(t, r2.Fire(context.Background(), hookEventUserPromptSubmit, nil).Message, work,
		"the hook runs with cwd = the session workdir")

	// Output size cap.
	r3 := NewHookRunner(
		[]HookConfig{{Event: hookEventUserPromptSubmit, Command: "head -c 200000 /dev/zero"}}, "s", work, "")
	capped := r3.Fire(context.Background(), hookEventUserPromptSubmit, nil)
	assert.LessOrEqual(t, len(capped.Message), hookOutputCap+2, "output must be size-capped")

	// Timeout: a hanging hook is skipped with a warning, never fatal.
	buf.Reset()

	r4 := NewHookRunner(
		[]HookConfig{{Event: hookEventUserPromptSubmit, Command: "sleep 30", TimeoutSec: 1}}, "s", work, "")
	timed := r4.Fire(context.Background(), hookEventUserPromptSubmit, nil)

	assert.True(t, timed.Proceed, "a timed-out hook must never block the turn")
	assert.Contains(t, buf.String(), "timeout", "the timeout skip warns (audit-loud)")

	// A failing hook (exit 1) also proceeds with a warning.
	buf.Reset()

	r5 := NewHookRunner([]HookConfig{{Event: hookEventUserPromptSubmit, Command: "exit 1"}}, "s", work, "")
	failed := r5.Fire(context.Background(), hookEventUserPromptSubmit, nil)

	assert.True(t, failed.Proceed, "a failed hook must never block the turn")
	assert.Contains(t, buf.String(), "hook", "the failure skip warns")
}

// TestUnmappedEventObserveOnly verifies firing an unmapped event is a warned
// no-op (parse + log + degrade — never silently ignored).
func TestUnmappedEventObserveOnly(t *testing.T) { //nolint:paralleltest // mutates the package logger seam
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
func TestFireNeverPanics(t *testing.T) { //nolint:paralleltest // mutates the package logger seam
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

// --- 21-01 Task 1: settings-scope hook loading (PAR-03) ---

// plantSettings writes content as one tree's settings.json (creating the
// parent dir) and returns the path.
func plantSettings(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), dirPerms))
	require.NoError(t, os.WriteFile(path, []byte(content), filePerms))
}

// plantSettingsFixture copies a committed settings fixture into path.
func plantSettingsFixture(t *testing.T, fixture, path string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", fixture))
	require.NoError(t, err, "fixture %s must be committed", fixture)

	plantSettings(t, path, string(data))
}

// TestSettingsHooksProjectScope (21-01 Task 1) verifies a project
// .claude/settings.json's hooks entries — the SAME hooks → event →
// matcher-group → {type,command,timeout} shape the plugin parser accepts —
// load into the registry tagged Scope=project, with the 60s default applied
// when a command entry omits its timeout (D-02: CC-compatible layout).
func TestSettingsHooksProjectScope(t *testing.T) {
	buf := captureShadowLogger(t)

	t.Setenv("HOME", t.TempDir()) // no user settings — project scope only

	proj := t.TempDir()
	plantSettingsFixture(t, "settings-project.json",
		filepath.Join(proj, claudeDirName, settingsJSONName))

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	byEvent := map[string][]HookConfig{}
	for _, h := range reg.Hooks {
		assert.Equal(t, ScopeProject, h.Scope, "settings hooks carry the project scope tag")
		byEvent[h.Event] = append(byEvent[h.Event], h)
	}

	pre := byEvent[hookEventPreToolUse]
	require.Len(t, pre, 2, "both PreToolUse matcher groups must parse")

	assert.Equal(t, "Edit|Write", pre[0].Matcher)
	assert.Equal(t, "./scripts/deny-edits.sh", pre[0].Command)
	assert.Equal(t, 5, pre[0].TimeoutSec, "explicit timeout preserved")
	assert.Equal(t, filepath.Join(proj, claudeDirName, settingsJSONName), pre[0].Path,
		"provenance names the settings.json")

	assert.Equal(t, "Bash", pre[1].Matcher)
	assert.Equal(t, hookDefaultTimeoutSec, pre[1].TimeoutSec,
		"absent timeout defaults to the 60s bound at parse time")

	post := byEvent[hookEventPostToolUse]
	require.Len(t, post, 1)
	assert.Equal(t, ".*", post[0].Matcher, "regex matcher preserved verbatim")

	assert.Empty(t, buf.String(), "a well-formed settings file contributes zero warnings")
}

// TestSettingsHooksUserScope verifies a user ~/.claude/settings.json (HOME
// redirected) loads with Scope=user even when no project settings exist.
func TestSettingsHooksUserScope(t *testing.T) {
	buf := captureShadowLogger(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	plantSettingsFixture(t, "settings-user.json",
		filepath.Join(home, claudeDirName, settingsJSONName))

	proj := t.TempDir() // project .claude/ exists but holds no settings.json
	require.NoError(t, os.MkdirAll(filepath.Join(proj, claudeDirName), dirPerms))

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))
	require.NoError(t, err)

	byEvent := map[string][]HookConfig{}
	for _, h := range reg.Hooks {
		assert.Equal(t, ScopeUser, h.Scope, "user settings hooks carry the user scope tag")
		byEvent[h.Event] = append(byEvent[h.Event], h)
	}

	pre := byEvent[hookEventPreToolUse]
	require.Len(t, pre, 1)
	assert.Empty(t, pre[0].Matcher, "empty matcher (catch-all) preserved verbatim")
	assert.Equal(t, "$HOME/.claude/hooks/user-guard.sh", pre[0].Command)
	assert.Equal(t, hookDefaultTimeoutSec, pre[0].TimeoutSec)

	sess := byEvent[hookEventSessionStart]
	require.Len(t, sess, 1)
	assert.Equal(t, 10, sess[0].TimeoutSec)

	assert.Empty(t, buf.String())
}

// TestSettingsHooksMalformed verifies every malformed settings shape degrades
// loudly-but-softly: zero hooks contributed, ONE stderr warning naming the
// file path, and Load still returns a nil error (Pitfall 8 — a repo must not
// be able to brick session construction).
func TestSettingsHooksMalformed(t *testing.T) { //nolint:paralleltest // mutates the package logger seam
	cases := map[string]string{
		"truncated bytes":     `{"hooks": {`,
		"hooks wrong type":    `{"hooks": 3}`,
		"group wrong type":    `{"hooks": {"PreToolUse": "not-a-list"}}`,
		"empty hooks map":     `{"hooks": {}}`,
		"no hooks key at all": `{"model": "opus"}`,
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			buf := captureShadowLogger(t)

			t.Setenv("HOME", t.TempDir()) // no user settings

			proj := t.TempDir()
			settingsPath := filepath.Join(proj, claudeDirName, settingsJSONName)
			plantSettings(t, settingsPath, content)

			reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))

			require.NoError(t, err, "malformed settings must never fail Load (Pitfall 8)")
			assert.Empty(t, reg.Hooks, "a malformed settings file contributes zero hooks")
			assert.Contains(t, buf.String(), settingsPath,
				"the skip warning names the offending file path")
		})
	}
}

// TestSettingsHooksOversized verifies a settings.json beyond readCapped's
// plugin-artifact cap is skipped with a warning — the same discipline as
// hooks.json (an untrusted file cannot pin the loader).
func TestSettingsHooksOversized(t *testing.T) { //nolint:paralleltest // mutates the package logger seam
	buf := captureShadowLogger(t)

	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	settingsPath := filepath.Join(proj, claudeDirName, settingsJSONName)
	plantSettings(t, settingsPath,
		`{"junk": "`+strings.Repeat("x", pluginArtifactMaxBytes)+`"}`)

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))

	require.NoError(t, err)
	assert.Empty(t, reg.Hooks, "an oversized settings file contributes zero hooks")
	assert.Contains(t, buf.String(), settingsPath, "the oversize skip warns")
}

// TestSettingsHooksMissing verifies absent settings files are the normal
// case: zero contributions and ZERO warnings.
func TestSettingsHooksMissing(t *testing.T) { //nolint:paralleltest // mutates the package logger seam
	buf := captureShadowLogger(t)

	t.Setenv("HOME", t.TempDir()) // no user file

	proj := t.TempDir() // no project .claude/ at all

	reg, err := Load(filepath.Join(proj, claudeDirName), filepath.Join(proj, assguardDirName))

	require.NoError(t, err)
	assert.Empty(t, reg.Hooks)
	assert.Empty(t, buf.String(), "absent settings files must not warn")
}

// --- 21-01 Task 3: scope partition + the PreToolUseVerdict seam ---

// TestHookScopeOrder (21-01 Task 3, D-03) verifies NewHookRunner partitions a
// deliberately scrambled input slice into the deterministic firing order
// project → user → plugin (stable within scope) — resolved at CONSTRUCTION,
// so the runtime.go call site keeps its signature — and that the caller's
// slice is never mutated.
func TestHookScopeOrder(t *testing.T) {
	t.Parallel()

	scrambled := []HookConfig{
		{Event: hookEventPreToolUse, Command: "u1", Scope: ScopeUser},
		{Event: hookEventPreToolUse, Command: "p1", Scope: ScopePlugin},
		{Event: hookEventPreToolUse, Command: "pr1", Scope: ScopeProject},
		{Event: hookEventPreToolUse, Command: "u2", Scope: ScopeUser},
		{Event: hookEventPreToolUse, Command: "p2", Scope: ScopePlugin},
	}

	r := NewHookRunner(scrambled, "s", t.TempDir(), "")

	matches := r.matchingHooks(hookEventPreToolUse, toolBash)

	got := make([]string, 0, len(matches))
	for _, h := range matches {
		got = append(got, h.Command)
	}

	assert.Equal(t, []string{"pr1", "u1", "u2", "p1", "p2"}, got,
		"D-03 firing order: project → user → plugin, stable within scope")

	assert.Equal(t, []string{"u1", "p1", "pr1", "u2", "p2"},
		commandsOf(scrambled), "the caller's slice must not be mutated")
}

// commandsOf extracts the commands for an order assertion.
func commandsOf(hooks []HookConfig) []string {
	out := make([]string, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, h.Command)
	}

	return out
}

// TestPreToolUseVerdict (21-01 Task 3) is the script-driven integration
// battery: matching hooks run SEQUENTIALLY in partitioned D-03 order under
// the existing runOne bounds, verdicts resolve through ResolveVerdict, and
// every failure mode fails open with NO-DECISION (PAR-03 letter — including
// the ctx-cancel row that pins the flagged PAR-03 concurrency assumption:
// sequential execution, cancel → no-decision).
func TestPreToolUseVerdict(t *testing.T) { //nolint:paralleltest // mutates the package logger seam
	buf := captureShadowLogger(t)

	work := t.TempDir()
	toolInput := json.RawMessage(`{"command":"ls"}`)

	denyJSONCmd := "printf '%s' '" + verdictJSON("deny", "no edits from hook") + "'"
	allowJSONCmd := "printf '%s' '" + verdictJSON("allow", "operator approved") + "'"

	t.Run("project deny json", func(t *testing.T) {
		r := NewHookRunner([]HookConfig{
			{Event: hookEventPreToolUse, Command: denyJSONCmd, Scope: ScopeProject},
		}, "s", work, "")

		v, reason := r.PreToolUseVerdict(context.Background(), toolBash, toolInput)

		assert.Equal(t, VerdictDeny, v)
		assert.Equal(t, "no edits from hook", reason)
	})

	t.Run("user allow json applies", func(t *testing.T) {
		r := NewHookRunner([]HookConfig{
			{Event: hookEventPreToolUse, Command: allowJSONCmd, Scope: ScopeUser},
		}, "s", work, "")

		v, reason := r.PreToolUseVerdict(context.Background(), toolBash, toolInput)

		assert.Equal(t, VerdictAllow, v)
		assert.Equal(t, "operator approved", reason)
	})

	t.Run("project allow demoted loud", func(t *testing.T) {
		buf.Reset()

		r := NewHookRunner([]HookConfig{
			{Event: hookEventPreToolUse, Command: allowJSONCmd, Scope: ScopeProject},
		}, "s", work, "")

		v, reason := r.PreToolUseVerdict(context.Background(), toolBash, toolInput)

		assert.Equal(t, VerdictNone, v, "repo-shipped allow must never widen trust (D-01)")
		assert.Empty(t, reason)
		assert.Equal(t, 1, strings.Count(buf.String(), "ignored allow"),
			"exactly one loud ignored-allow warning: %q", buf.String())
	})

	t.Run("timeout fails open", func(t *testing.T) {
		r := NewHookRunner([]HookConfig{
			{Event: hookEventPreToolUse, Command: "sleep 30", TimeoutSec: 1, Scope: ScopeUser},
		}, "s", work, "")

		v, reason := r.PreToolUseVerdict(context.Background(), toolBash, toolInput)

		assert.Equal(t, VerdictNone, v, "a timed-out hook contributes no verdict (PAR-03)")
		assert.Empty(t, reason)
	})

	t.Run("ctx cancel fails open", func(t *testing.T) {
		r := NewHookRunner([]HookConfig{
			{Event: hookEventPreToolUse, Command: "sleep 30", TimeoutSec: 60, Scope: ScopeUser},
		}, "s", work, "")

		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(200 * time.Millisecond)
			cancel()
		}()

		v, reason := r.PreToolUseVerdict(ctx, toolBash, toolInput)

		assert.Equal(t, VerdictNone, v,
			"the flagged PAR-03 concurrency pin: cancellation mid-hook yields no-decision")
		assert.Empty(t, reason)
	})

	t.Run("exit2 legacy deny reason from stderr", func(t *testing.T) {
		r := NewHookRunner([]HookConfig{
			{Event: hookEventPreToolUse, Command: "echo legacy-refusal >&2; exit 2", Scope: ScopeProject},
		}, "s", work, "")

		v, reason := r.PreToolUseVerdict(context.Background(), toolBash, toolInput)

		assert.Equal(t, VerdictDeny, v)
		assert.Contains(t, reason, "legacy-refusal",
			"the exit-2 reason is classifyHookRun's stderr-first extraction")
	})

	t.Run("exit2 overrides json allow at runner", func(t *testing.T) {
		r := NewHookRunner([]HookConfig{
			{Event: hookEventPreToolUse, Command: allowJSONCmd + "; exit 2", Scope: ScopeProject},
		}, "s", work, "")

		v, _ := r.PreToolUseVerdict(context.Background(), toolBash, toolInput)

		assert.Equal(t, VerdictDeny, v,
			"exit 2 blocks even when stdout carries a valid allow verdict (T-21-02)")
	})

	t.Run("sequential d-03 firing order", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "marker")

		r := NewHookRunner([]HookConfig{
			{Event: hookEventPreToolUse, Command: "echo g >> " + marker, Scope: ScopePlugin},
			{Event: hookEventPreToolUse, Command: "echo u >> " + marker, Scope: ScopeUser},
			{Event: hookEventPreToolUse, Command: "echo p >> " + marker, Scope: ScopeProject},
		}, "s", work, "")

		v, _ := r.PreToolUseVerdict(context.Background(), toolBash, toolInput)

		assert.Equal(t, VerdictNone, v, "silent hooks contribute no verdict")

		data, err := os.ReadFile(marker)
		require.NoError(t, err)
		assert.Equal(t, "p\nu\ng\n", string(data),
			"hooks fire SEQUENTIALLY in D-03 order — never concurrently")
	})

	t.Run("no matching hooks is no decision", func(t *testing.T) {
		r := NewHookRunner([]HookConfig{
			{Event: hookEventPreToolUse, Matcher: "Edit", Command: denyJSONCmd, Scope: ScopeProject},
		}, "s", work, "")

		v, reason := r.PreToolUseVerdict(context.Background(), toolBash, toolInput)

		assert.Equal(t, VerdictNone, v)
		assert.Empty(t, reason)
	})
}

// (TestPreToolUseDelegation, 21-01 Task 3, was deleted at the 21-06 gate
// join: it pinned the executor leg's boolean refusal pair — HookRunner's
// PreToolUse method and coreexec's consultation — both disposed so hook
// verdicts consume at internal/session's gateCall head ONLY. The verdict
// semantics it covered (deny reason, user-scope allow) live in
// TestPreToolUseVerdict and the session TestGateHookVerdict battery.)

// TestHookMatcherDialect pins CC's two-path matcher dialect for settings
// hooks (21-01, Pitfall 3) against the legacy compile-everything regex
// behavior kept for plugin hooks (D-02: existing plugin hooks unchanged).
func TestHookMatcherDialect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		scope   HookScope
		matcher string
		tool    string
		want    bool
	}{
		// Settings exact path: matcher "Edit" matches ONLY the tool named
		// Edit — the divergence from legacy substring/regex matching.
		{"settings exact fires on exact", ScopeProject, "Edit", "Edit", true},
		{"settings exact not substring", ScopeProject, "Edit", "NotebookEdit", false},
		{"settings exact not substring user scope", ScopeUser, "Edit", "NotebookEdit", false},
		{"settings exact full-name only", ScopeUser, "NotebookEdit", "NotebookEdit", true},

		// Alternatives (pipes and commas both split).
		{"settings pipe alternatives Edit", ScopeProject, "Edit|Write", "Edit", true},
		{"settings pipe alternatives Write", ScopeProject, "Edit|Write", "Write", true},
		{"settings pipe alternatives miss", ScopeProject, "Edit|Write", "Read", false},
		{"settings comma alternatives", ScopeProject, "Edit,Write", "Write", true},
		{"settings comma with space", ScopeProject, "Edit, Write", "Write", true},

		// Any char outside the exact-set → unanchored regex path. The parens
		// row proves REGEX semantics: Go reads "Bash(git *)" as "Bash" + a
		// capture group "git *", so it matches "Bashgit" — an exact-string
		// comparison ("Bash(git *)" == tool) could never fire there.
		{"settings regex dot-star", ScopeProject, "mcp__github__.*", "mcp__github__get_issue", true},
		{"settings regex dot-star miss", ScopeProject, "mcp__github__.*", "mcp__fs__read", false},
		{"settings regex unanchored", ScopeProject, "Edit.*", "NotebookEdit", true},
		{"settings regex parens group", ScopeProject, "Bash(git *)", "Bashgit", true},
		{"settings regex parens literal miss", ScopeProject, "Bash(git *)", "Bash(git )", false},

		// Catch-alls keep their meaning in both scopes.
		{"settings empty matcher catch-all", ScopeProject, "", "Anything", true},
		{"settings star catch-all", ScopeUser, "*", "Anything", true},

		// Legacy plugin scope: EVERY non-empty matcher compiles as an
		// unanchored regex — "Edit" still matches "NotebookEdit" (D-02).
		{"plugin legacy substring match", ScopePlugin, "Edit", "NotebookEdit", true},
		{"plugin legacy exact match", ScopePlugin, "Edit", "Edit", true},
		{"plugin legacy regex", ScopePlugin, "mcp__github__.*", "mcp__github__get_issue", true},
		{"plugin legacy miss", ScopePlugin, "Edit", "Write", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := NewHookRunner([]HookConfig{{
				Event: hookEventPreToolUse, Matcher: tc.matcher,
				Command: "true", Scope: tc.scope,
			}}, "s", "", "")

			matches := r.matchingHooks(hookEventPreToolUse, tc.tool)

			if tc.want {
				assert.Len(t, matches, 1, "matcher %q must fire on tool %q (scope %d)",
					tc.matcher, tc.tool, tc.scope)
			} else {
				assert.Empty(t, matches, "matcher %q must NOT fire on tool %q (scope %d)",
					tc.matcher, tc.tool, tc.scope)
			}
		})
	}
}
