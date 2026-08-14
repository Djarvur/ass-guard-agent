package ecosys_test

import (
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
)

// expandCases is the zcode substitution contract (FEATURES §(a) / PITFALLS
// Pitfall 3) encoded as one table — written BEFORE the implementation (the
// REQ mandates the contract test precede Expand). Every row pins an observable
// zcode semantic; ass-guard must match the mimicry target exactly, including
// where it differs from Claude Code (brace form + dynamic shell stay literal).
func expandCases() []struct {
	name string
	body string
	args string
	want string
} {
	return []struct {
		name string
		body string
		args string
		want string
	}{
		{
			name: "happy full-argument substitution",
			body: "Study $ARGUMENTS now",
			args: "fix-the-thing",
			want: "Study fix-the-thing now",
		},
		{
			name: "positionals split on whitespace",
			body: "$1 then $2",
			args: "a b c",
			want: "a then b",
		},
		{
			name: "out-of-range positional becomes empty",
			body: "[$3]",
			args: "a b",
			want: "[]",
		},
		{
			name: "zero positional becomes empty (1-based)",
			body: "[$0]",
			args: "a b",
			want: "[]",
		},
		{
			name: "args verbatim for $ARGUMENTS (no field split)",
			body: "Run $ARGUMENTS",
			args: "  spaced   out  ",
			want: "Run   spaced   out  ",
		},
		{
			name: "no placeholder appends under User arguments heading",
			body: "Just do the thing.",
			args: "x y",
			want: "Just do the thing.\n\nUser arguments:\nx y",
		},
		{
			name: "no args no append",
			body: "Just do the thing.",
			args: "",
			want: "Just do the thing.",
		},
		{
			name: "matched positional suppresses the append",
			body: "first is $1",
			args: "one two",
			want: "first is one",
		},
		{
			name: "unmatched-only positional still appends (no token matched)",
			body: "third was [$3]",
			args: "a b",
			want: "third was []\n\nUser arguments:\na b",
		},
		{
			name: "brace form stays literal (zcode rejects ${ARGUMENTS})",
			body: "literal ${ARGUMENTS} stays",
			args: "zzz",
			want: "literal ${ARGUMENTS} stays\n\nUser arguments:\nzzz",
		},
		{
			name: "dynamic shell backtick stays literal (never executed)",
			body: "value !`rm -rf /` end",
			args: "",
			want: "value !`rm -rf /` end",
		},
		{
			name: "single pass: substituted $ARGUMENTS does not re-expand",
			body: "echo $ARGUMENTS",
			args: "$ARGUMENTS",
			want: "echo $ARGUMENTS",
		},
		{
			name: "substitution applies inside code fences",
			body: "```\nbuild $ARGUMENTS now\n```",
			args: "the-widget",
			want: "```\nbuild the-widget now\n```",
		},
		{
			name: "empty args substitute empty for present placeholders",
			body: "Study $ARGUMENTS now",
			args: "",
			want: "Study  now",
		},
	}
}

// TestExpand_Contract runs the substitution contract table.
func TestExpand_Contract(t *testing.T) {
	t.Parallel()

	for _, tc := range expandCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := ecosys.Command{Name: "t", Body: tc.body}
			if got := c.Expand(tc.args); got != tc.want {
				t.Errorf("Expand(%q) =\n%q\nwant\n%q", tc.args, got, tc.want)
			}
		})
	}
}

// TestExpand_NeverExecutes pins the structural property behind the shell
// backtick row: expansion is pure text substitution. A body that LOOKS like
// dynamic shell must survive verbatim — and the package must not even import
// an exec surface (lint-checked separately; this row is the behavioral pin).
func TestExpand_NeverExecutes(t *testing.T) {
	t.Parallel()

	c := ecosys.Command{Body: "pre !`touch /tmp/assguard-expand-pwned` post"}

	if got, want := c.Expand(""), "pre !`touch /tmp/assguard-expand-pwned` post"; got != want {
		t.Errorf("Expand = %q; want verbatim %q", got, want)
	}
}

// invocationCases pins ParseInvocation: the zcode token regex
// ^/([a-z0-9][a-z0-9_:-]{0,63})(\s+(.*))?$ with leading-newline tolerance and
// args captured verbatim after the first whitespace run.
func invocationCases() []struct {
	name string
	text string
	key  string
	args string
	ok   bool
} {
	return []struct {
		name string
		text string
		key  string
		args string
		ok   bool
	}{
		{name: "namespaced with args", text: "/opsx:explore fix the thing", key: "opsx:explore", args: "fix the thing", ok: true},
		{name: "registry miss is still parsed (caller decides)", text: "/foo", key: "foo", args: "", ok: true},
		{name: "no args yields empty args", text: "/opsx:explore", key: "opsx:explore", args: "", ok: true},
		{name: "leading newline tolerated", text: "\n/opsx:explore x", key: "opsx:explore", args: "x", ok: true},
		{name: "uppercase rejected by regex", text: "/Bad_Name", ok: false},
		{name: "mid-text slash is not an invocation", text: "hello /opsx:explore", ok: false},
		{name: "leading space is not an invocation", text: " /opsx:explore x", ok: false},
		{name: "plain text is not an invocation", text: "just a normal message", ok: false},
		{name: "empty text", text: "", ok: false},
		{name: "bare slash", text: "/", ok: false},
	}
}

// TestParseInvocation runs the invocation-parse table.
func TestParseInvocation(t *testing.T) {
	t.Parallel()

	for _, tc := range invocationCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			key, args, ok := ecosys.ParseInvocation(tc.text)
			if ok != tc.ok {
				t.Fatalf("ParseInvocation(%q) ok = %v; want %v", tc.text, ok, tc.ok)
			}

			if !tc.ok {
				return
			}

			if key != tc.key {
				t.Errorf("key = %q; want %q", key, tc.key)
			}

			if args != tc.args {
				t.Errorf("args = %q; want %q", args, tc.args)
			}
		})
	}
}

// TestExpand_RealOpsxBodyShape sanity-checks expansion against the real opsx
// command-body shape (prose-first markdown with a $ARGUMENTS mention): the
// expanded body keeps the markdown structure and does not start with a slash
// token (the no-re-expansion property the turn wiring relies on).
func TestExpand_RealOpsxBodyShape(t *testing.T) {
	t.Parallel()

	body := "Explore the requested change context.\n\nFocus on: $ARGUMENTS\n\n- read the codebase\n- compare options\n"
	c := ecosys.Command{Name: "opsx:explore", Body: body}

	got := c.Expand("fix login flow")

	if !strings.Contains(got, "Focus on: fix login flow") {
		t.Errorf("expanded = %q; want args substituted", got)
	}

	if strings.HasPrefix(got, "/") {
		t.Errorf("expanded body starts with a slash token — re-expansion risk: %q", got[:min(20, len(got))])
	}

	if !strings.Contains(got, "- read the codebase") {
		t.Errorf("expanded = %q; markdown list lost", got)
	}
}
