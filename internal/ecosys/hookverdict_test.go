package ecosys //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exitCodeErr runs `sh -c "exit N"` and returns the genuine *exec.ExitError —
// the portable way to hand parseHookVerdict a real exit-code error (an
// ExitError cannot be constructed with a meaningful ProcessState by hand).
func exitCodeErr(t *testing.T, code int) error {
	t.Helper()

	err := exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run() //nolint:gosec // test helper, fixed code
	require.Error(t, err)

	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr), "sh exit must yield *exec.ExitError")
	require.Equal(t, code, exitErr.ExitCode())

	return err
}

// verdictJSON renders one hookSpecificOutput verdict body.
func verdictJSON(decision, reason string) string {
	if reason == "" {
		return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"` + decision + `"}}`
	}

	return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"` + decision +
		`","permissionDecisionReason":"` + reason + `"}}`
}

// TestHookVerdictParse (21-01 Task 2, D-02/Pitfall 4) pins the per-hook
// verdict channels: the hookSpecificOutput JSON body (permissionDecision
// allow/deny/ask + permissionDecisionReason), the legacy exit-2 refusal —
// which BLOCKS even over a valid JSON allow on stdout (the CC spoofing rule,
// T-21-02) — and the NO-DECISION fail-open for silence, schema-invalid
// bodies, and execution failures (PAR-03 letter).
func TestHookVerdictParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		stdout     string
		runErr     error
		want       Verdict
		wantReason string
	}{
		{
			name:       "deny decision with reason",
			stdout:     verdictJSON("deny", "Destructive command blocked by hook"),
			want:       verdictDeny,
			wantReason: "Destructive command blocked by hook",
		},
		{
			name:       "ask decision with reason",
			stdout:     verdictJSON("ask", "operator review required"),
			want:       verdictAsk,
			wantReason: "operator review required",
		},
		{
			name:       "allow decision with reason",
			stdout:     verdictJSON("allow", "trusted command"),
			want:       verdictAllow,
			wantReason: "trusted command",
		},
		{
			name:   "deny with empty reason",
			stdout: verdictJSON("deny", ""),
			want:   verdictDeny,
		},
		{
			name:   "leading whitespace before JSON still parses",
			stdout: "\n  \t" + verdictJSON("deny", "ws-gate"),
			want:   verdictDeny, wantReason: "ws-gate",
		},
		{
			// Pitfall 4: schema-invalid output is NEVER honored.
			name:   "missing permissionDecision",
			stdout: `{"hookSpecificOutput":{"hookEventName":"PreToolUse"}}`,
			want:   verdictNone,
		},
		{
			name:   "unknown permissionDecision value",
			stdout: verdictJSON("maybe", "x"),
			want:   verdictNone,
		},
		{
			name:   "wrong-typed permissionDecision",
			stdout: `{"hookSpecificOutput":{"permissionDecision":5}}`,
			want:   verdictNone,
		},
		{
			name:   "wrong-typed reason",
			stdout: `{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":7}}`,
			want:   verdictNone,
		},
		{
			name:   "json without hookSpecificOutput",
			stdout: `{"decision":"deny","reason":"nope"}`,
			want:   verdictNone,
		},
		{
			name:   "truncated json object",
			stdout: `{"hookSpecificOutput":`,
			want:   verdictNone,
		},
		{
			name:   "plain-text stdout",
			stdout: "hook ran fine, no verdict",
			want:   verdictNone,
		},
		{
			name:   "empty stdout",
			stdout: "",
			want:   verdictNone,
		},
		{
			// T-21-02 / D-02: exit 2 is checked BEFORE the JSON channel and
			// overrides it — a valid allow verdict on stdout cannot spoof a
			// pass past the refusal code. The direct-call reason is the
			// capped stdout (the only channel this signature sees); the
			// composed runner path upgrades it to stderr-first.
			name:       "exit2-overrides-json",
			stdout:     verdictJSON("allow", "trust me"),
			runErr:     exitCodeErr(t, 2),
			want:       verdictDeny,
			wantReason: verdictJSON("allow", "trust me"),
		},
		{
			name:   "exit2 empty stdout",
			stdout: "",
			runErr: exitCodeErr(t, 2),
			want:   verdictDeny,
		},
		{
			// PAR-03 letter: hook EXECUTION failure is fail-open — the hook
			// contributes NO verdict.
			name:   "exit1 non-refusal fail-open",
			runErr: exitCodeErr(t, 1),
			want:   verdictNone,
		},
		{
			name:   "timeout error fail-open",
			runErr: context.DeadlineExceeded,
			want:   verdictNone,
		},
		{
			name:   "spawn error fail-open",
			runErr: exec.Command("definitely-not-a-real-binary-xyz").Run(),
			want:   verdictNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, reason := parseHookVerdict(tc.stdout, tc.runErr)

			assert.Equal(t, tc.want, got, "verdict for %q (runErr %v)", tc.stdout, tc.runErr)
			assert.Equal(t, tc.wantReason, reason, "reason")
		})
	}
}

// TestHookVerdictResolve (21-01 Task 2, D-01/D-03/D-04) pins the pure
// deny-wins resolver over the deterministic scope order project → user →
// plugin: ANY deny wins carrying the FIRST denying hook's reason; a
// USER-scope allow applies only when no deny exists; a project-scope allow
// is demoted to a loud ignored-allow warning contributing no decision
// (repo-shipped files never widen trust); ask survives unless a deny exists.
func TestHookVerdictResolve(t *testing.T) { //nolint:paralleltest // mutates the package logger seam
	cases := []struct {
		name       string
		results    []ScopedResult
		want       Verdict
		wantReason string
		wantWarn   int // expected ignored-allow warnings
	}{
		{
			name: "deny-wins over user allow",
			results: []ScopedResult{
				{Scope: scopeProject, Verdict: verdictDeny, Reason: "project says no"},
				{Scope: scopeUser, Verdict: verdictAllow},
			},
			want:       verdictDeny,
			wantReason: "project says no",
		},
		{
			name: "first-deny-reason in scope order",
			results: []ScopedResult{
				{Scope: scopeProject, Verdict: verdictDeny, Reason: "first"},
				{Scope: scopeUser, Verdict: verdictDeny, Reason: "second"},
			},
			want:       verdictDeny,
			wantReason: "first",
		},
		{
			name: "user deny carries its reason",
			results: []ScopedResult{
				{Scope: scopeProject, Verdict: verdictNone},
				{Scope: scopeUser, Verdict: verdictDeny, Reason: "user blocks"},
			},
			want:       verdictDeny,
			wantReason: "user blocks",
		},
		{
			name: "plugin deny beats user allow",
			results: []ScopedResult{
				{Scope: scopeUser, Verdict: verdictAllow},
				{Scope: scopePlugin, Verdict: verdictDeny, Reason: "plugin blocks"},
			},
			want:       verdictDeny,
			wantReason: "plugin blocks",
		},
		{
			name: "project-allow-demoted to no-decision",
			results: []ScopedResult{
				{Scope: scopeProject, Verdict: verdictAllow, Reason: "repo grants itself"},
			},
			want:     verdictNone,
			wantWarn: 1,
		},
		{
			name: "project-allow-demoted beside user allow",
			results: []ScopedResult{
				{Scope: scopeProject, Verdict: verdictAllow, Reason: "repo grants itself"},
				{Scope: scopeUser, Verdict: verdictAllow, Reason: "operator grants"},
			},
			want:       verdictAllow,
			wantReason: "operator grants",
			wantWarn:   1,
		},
		{
			name: "plugin-allow-demoted to no-decision",
			results: []ScopedResult{
				{Scope: scopePlugin, Verdict: verdictAllow},
			},
			want:     verdictNone,
			wantWarn: 1,
		},
		{
			name: "ask-survives user allow",
			results: []ScopedResult{
				{Scope: scopeProject, Verdict: verdictAsk, Reason: "check this"},
				{Scope: scopeUser, Verdict: verdictAllow},
			},
			want:       verdictAsk,
			wantReason: "check this",
		},
		{
			name: "ask suppressed by deny",
			results: []ScopedResult{
				{Scope: scopeProject, Verdict: verdictAsk, Reason: "check this"},
				{Scope: scopeUser, Verdict: verdictDeny, Reason: "no"},
			},
			want:       verdictDeny,
			wantReason: "no",
		},
		{
			name: "silent-is-nothing",
			results: []ScopedResult{
				{Scope: scopeProject, Verdict: verdictNone},
				{Scope: scopeUser, Verdict: verdictNone},
				{Scope: scopePlugin, Verdict: verdictNone},
			},
			want: verdictNone,
		},
		{
			name: "empty input is no-decision",
			want: verdictNone,
		},
		{
			name: "user allow alone applies",
			results: []ScopedResult{
				{Scope: scopeUser, Verdict: verdictAllow, Reason: "granted"},
			},
			want:       verdictAllow,
			wantReason: "granted",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureShadowLogger(t)

			got, reason := ResolveVerdict(tc.results)

			assert.Equal(t, tc.want, got, "resolved verdict")
			assert.Equal(t, tc.wantReason, reason, "resolved reason")

			warns := strings.Count(buf.String(), "ignored allow")
			assert.Equal(t, tc.wantWarn, warns,
				"demoted-allow warnings: got log %q", buf.String())
		})
	}
}
