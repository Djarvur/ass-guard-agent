package perm_test

import (
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/perm"
)

// TestRules is the CC-parity grammar battery (D-02): every quoted semantic
// from 17-RESEARCH §CC Rule Model is a pinned table row named after the
// parity fact it locks. Evaluation order bugs here are silent privilege
// escalation (T-17-01), so the order rows assert the verdict even when the
// competing rule is strictly more specific.
func TestRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		deny  []string
		ask   []string
		allow []string
		tool  string
		arg   string
		want  perm.Verdict
	}{
		// --- order-beats-specificity: deny → ask → allow, first match wins ---
		{
			name:  "order-beats-specificity/broad-deny-beats-narrower-allow",
			deny:  []string{"Bash(aws *)"},
			allow: []string{"Bash(aws s3 ls)"},
			tool:  "Bash",
			arg:   "aws s3 rm bucket/insecure",
			want:  perm.VerdictDeny,
		},
		{
			name:  "order-beats-specificity/exact-allow-still-deny",
			deny:  []string{"Bash(aws *)"},
			allow: []string{"Bash(aws s3 ls)"},
			tool:  "Bash",
			arg:   "aws s3 ls",
			want:  perm.VerdictDeny,
		},

		// --- ask-beats-allow: a matching ask prompts even when a more specific allow also matches ---
		{
			name:  "ask-beats-allow/specific-allow-does-not-bypass",
			ask:   []string{"Write"},
			allow: []string{"Write(*.go)"},
			tool:  "Write",
			arg:   "main.go",
			want:  perm.VerdictAsk,
		},
		{
			name:  "ask-beats-allow/any-write-input",
			ask:   []string{"Write"},
			allow: []string{"Write(*.go)"},
			tool:  "Write",
			arg:   "notes.txt",
			want:  perm.VerdictAsk,
		},

		// --- deny-beats-allow: a deny anywhere beats any allow ---
		{
			name:  "deny-beats-allow/bare-deny-over-allow-specifier",
			deny:  []string{"Write"},
			allow: []string{"Write(*)"},
			tool:  "Write",
			arg:   "anything",
			want:  perm.VerdictDeny,
		},
		{
			name:  "deny-beats-allow/specific-deny-over-exact-allow",
			deny:  []string{"Bash(rm *)"},
			allow: []string{"Bash(rm -rf /tmp/x)"},
			tool:  "Bash",
			arg:   "rm -rf /tmp/x",
			want:  perm.VerdictDeny,
		},

		// --- compound-worst-match: split on && || ; | |& & and newlines; every subcommand matched independently ---
		{
			name:  "compound-worst-match/deny-subcommand-worst",
			deny:  []string{"Bash(rm *)"},
			allow: []string{"Bash(git *)"},
			tool:  "Bash",
			arg:   "git status && rm -rf /tmp/x",
			want:  perm.VerdictDeny,
		},
		{
			name:  "compound-worst-match/all-allow-subcommands",
			allow: []string{"Bash(git *)"},
			tool:  "Bash",
			arg:   "git status && git log",
			want:  perm.VerdictAllow,
		},
		{
			name:  "compound-worst-match/ask-subcommand-beats-allow",
			ask:   []string{"Bash(npm *)"},
			allow: []string{"Bash(git *)"},
			tool:  "Bash",
			arg:   "git status && npm publish",
			want:  perm.VerdictAsk,
		},
		{
			name:  "compound-worst-match/unmatched-subcommand-failsafe",
			allow: []string{"Bash(git *)"},
			tool:  "Bash",
			arg:   "git status && curl example.com",
			want:  perm.Unmatched,
		},
		{
			name: "compound-worst-match/all-separators-split",
			deny: []string{"Bash(rm *)"},
			tool: "Bash",
			arg:  "echo hi; ls | rm -rf x & sleep 1 || true\nsafe",
			want: perm.VerdictDeny,
		},
		{
			name:  "compound-worst-match/single-segment-no-split",
			allow: []string{"Bash(git *)"},
			tool:  "Bash",
			arg:   "git status",
			want:  perm.VerdictAllow,
		},

		// --- mcp-namespace: mcp__server__tool rules, bare server = whole server ---
		{
			name:  "mcp-namespace/tool-glob-allow",
			allow: []string{"mcp__github__get_*"},
			tool:  "mcp__github__get_issue",
			want:  perm.VerdictAllow,
		},
		{
			name: "mcp-namespace/whole-server-deny",
			deny: []string{"mcp__puppeteer"},
			tool: "mcp__puppeteer__click",
			want: perm.VerdictDeny,
		},
		{
			name: "mcp-namespace/whole-server-deny-exact-name",
			deny: []string{"mcp__puppeteer"},
			tool: "mcp__puppeteer",
			want: perm.VerdictDeny,
		},
		{
			name: "mcp-namespace/no-cross-server-match",
			deny: []string{"mcp__puppeteer"},
			tool: "mcp__github__get_issue",
			want: perm.Unmatched,
		},
		{
			name: "mcp-namespace/deny-name-glob-matches",
			deny: []string{"mcp__github__get_*"},
			tool: "mcp__github__get_issue",
			want: perm.VerdictDeny,
		},

		// --- unanchored-allow-skip: "*", "B*", "mcp__*" are inert in the allow list (CC asymmetry) ---
		{
			name:  "unanchored-allow-skip/star-inert",
			allow: []string{"*"},
			tool:  "Bash",
			arg:   "anything",
			want:  perm.Unmatched,
		},
		{
			name:  "unanchored-allow-skip/prefix-glob-inert",
			allow: []string{"B*"},
			tool:  "Bash",
			arg:   "ls",
			want:  perm.Unmatched,
		},
		{
			name:  "unanchored-allow-skip/mcp-star-inert",
			allow: []string{"mcp__*"},
			tool:  "mcp__github__get_issue",
			want:  perm.Unmatched,
		},
		{
			name:  "unanchored-allow-skip/anchored-server-glob-kept",
			allow: []string{"mcp__puppeteer__*"},
			tool:  "mcp__puppeteer__click",
			want:  perm.VerdictAllow,
		},
		{
			name: "unanchored-allow-skip/same-glob-deny-matches",
			deny: []string{"mcp__*"},
			tool: "mcp__github__get_issue",
			want: perm.VerdictDeny,
		},

		// --- malformed-skip: bad lines are skipped with warnings; valid lines still apply; never panics ---
		{
			name: "malformed-skip/valid-line-still-applies",
			deny: []string{"", "Bash(git", "Wri te", "Write"},
			tool: "Write",
			arg:  "x",
			want: perm.VerdictDeny,
		},

		// --- grammar details pinned by D-02's syntax ---
		{
			name: "bare-tool-matches-any-input",
			ask:  []string{"Write"},
			tool: "Write",
			want: perm.VerdictAsk,
		},
		{
			name: "no-rules-no-match",
			tool: "Read",
			arg:  "f.go",
			want: perm.Unmatched,
		},
		{
			name: "colon-star-suffix-equals-trailing-star",
			deny: []string{"Bash(ls:*)"},
			tool: "Bash",
			arg:  "ls -la",
			want: perm.VerdictDeny,
		},
		{
			name: "colon-star-suffix-space-star-identical",
			deny: []string{"Bash(ls *)"},
			tool: "Bash",
			arg:  "ls -la",
			want: perm.VerdictDeny,
		},
		{
			name: "colon-star-mid-pattern-is-literal",
			deny: []string{"Bash(echo a:b:*c)"},
			tool: "Bash",
			arg:  "echo a:b:*c",
			want: perm.VerdictDeny,
		},
		{
			name: "colon-star-mid-pattern-literal-no-wildcard",
			deny: []string{"Bash(echo a:b:*c)"},
			tool: "Bash",
			arg:  "echo a:bXc",
			want: perm.Unmatched,
		},
		{
			name: "exact-specifier-no-prefix-creep",
			deny: []string{"Bash(git status)"},
			tool: "Bash",
			arg:  "git statusx",
			want: perm.Unmatched,
		},
		{
			name:  "bare-tool-does-not-match-suffix-extended-name",
			deny:  []string{"Write"},
			tool:  "WriteExtra",
			want:  perm.Unmatched,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rs, _ := perm.NewRuleSet(tt.deny, tt.ask, tt.allow)

			if got := rs.Evaluate(tt.tool, tt.arg); got != tt.want {
				t.Errorf("Evaluate(%q, %q) = %v; want %v", tt.tool, tt.arg, got, tt.want)
			}
		})
	}
}

// TestRulesParse pins the rule-string grammar: bare names, Tool(specifier),
// the end-anchored ":*" equivalence, and the malformed shapes that must
// degrade to errors (never panic).
func TestRulesParse(t *testing.T) {
	t.Parallel()

	ok := []struct {
		in   string
		want perm.Rule
	}{
		{in: "Write", want: perm.Rule{Tool: "Write"}},
		{in: "  Write  ", want: perm.Rule{Tool: "Write"}},
		{in: "Bash(git *)", want: perm.Rule{Tool: "Bash", Pattern: "git *"}},
		{in: "Bash(ls:*)", want: perm.Rule{Tool: "Bash", Pattern: "ls *"}},
		{in: "Bash(git status)", want: perm.Rule{Tool: "Bash", Pattern: "git status"}},
		{in: "mcp__github__get_*", want: perm.Rule{Tool: "mcp__github__get_*"}},
		{in: "Write(*)", want: perm.Rule{Tool: "Write", Pattern: "*"}},
	}

	for _, tt := range ok {
		r, err := perm.ParseRule(tt.in)
		if err != nil {
			t.Errorf("ParseRule(%q) unexpected error: %v", tt.in, err)

			continue
		}

		if r.Tool != tt.want.Tool || r.Pattern != tt.want.Pattern {
			t.Errorf("ParseRule(%q) = %+v; want %+v", tt.in, r, tt.want)
		}

		if r.Effect != perm.EffectNone {
			t.Errorf("ParseRule(%q) Effect = %v; want EffectNone (the owning list assigns the effect)", tt.in, r.Effect)
		}
	}

	bad := []string{
		"",
		"   ",
		"Bash(git",
		"Bash git)",
		")Write",
		"Bash()",
		"Wri te",
		"Wri te(git)",
		"Bash(a(b)",
		"Bash(a)b)",
		"Write!",
		"Bash(x)extra",
	}

	for _, in := range bad {
		r, err := perm.ParseRule(in)
		if err == nil {
			t.Errorf("ParseRule(%q) = %+v; want error", in, r)
		}
	}
}

// TestRulesWarnings pins the structured warning sink: one warning per
// malformed line and one per unanchored allow glob, with valid lines kept.
func TestRulesWarnings(t *testing.T) {
	t.Parallel()

	// Unanchored allow globs: exactly one warning each, all skipped as inert.
	_, warns := perm.NewRuleSet(nil, nil, []string{"*", "B*", "mcp__*", "mcp__puppeteer__*", "Write"})
	if len(warns) != 3 {
		t.Fatalf("unanchored allow warnings = %d (%+v); want 3", len(warns), warns)
	}

	for _, w := range warns {
		switch w.Rule {
		case "*", "B*", "mcp__*":
		default:
			t.Errorf("unexpected unanchored-allow warning for rule %q", w.Rule)
		}

		if w.Reason == "" {
			t.Errorf("warning for rule %q has empty reason", w.Rule)
		}
	}

	// Malformed lines: one warning each, parse failures named as the reason.
	_, warns = perm.NewRuleSet([]string{"", "Bash(git", "Write"}, nil, nil)
	if len(warns) != 2 {
		t.Fatalf("malformed warnings = %d (%+v); want 2", len(warns), warns)
	}

	for _, w := range warns {
		switch w.Rule {
		case "", "Bash(git":
		default:
			t.Errorf("unexpected malformed warning for rule %q", w.Rule)
		}

		if w.Reason == "" {
			t.Errorf("warning for rule %q has empty reason", w.Rule)
		}
	}

	// A clean rule set carries no warnings.
	_, warns = perm.NewRuleSet([]string{"Bash(rm *)"}, []string{"Write"}, []string{"Bash(git *)"})
	if len(warns) != 0 {
		t.Errorf("clean rule set warnings = %+v; want none", warns)
	}
}

// TestRulesMCPHelpers pins the mcp__<server>__<tool> namespace helpers the
// session mapping consumes (RESEARCH Pitfall 7 / A7).
func TestRulesMCPHelpers(t *testing.T) {
	t.Parallel()

	if got := perm.MCPName("github", "get_issue"); got != "mcp__github__get_issue" {
		t.Errorf("MCPName = %q; want mcp__github__get_issue", got)
	}

	server, tool, ok := perm.SplitMCPName("mcp__github__get_issue")
	if !ok || server != "github" || tool != "get_issue" {
		t.Errorf("SplitMCPName = (%q, %q, %v); want (github, get_issue, true)", server, tool, ok)
	}

	if _, _, ok := perm.SplitMCPName("Write"); ok {
		t.Error("SplitMCPName(Write) ok = true; want false (not an MCP name)")
	}

	if _, _, ok := perm.SplitMCPName("mcp__puppeteer"); ok {
		t.Error("SplitMCPName(mcp__puppeteer) ok = true; want false (server-only, no tool part)")
	}

	if _, _, ok := perm.SplitMCPName("mcp__"); ok {
		t.Error("SplitMCPName(mcp__) ok = true; want false (empty server)")
	}

	for _, server := range []string{"github", "puppeteer"} {
		for _, tool := range []string{"get_issue", "click", "get_*"} {
			name := perm.MCPName(server, tool)

			gotServer, gotTool, ok := perm.SplitMCPName(name)
			if !ok || gotServer != server || gotTool != tool {
				t.Errorf("round-trip %q = (%q, %q, %v); want (%q, %q, true)", name, gotServer, gotTool, ok, server, tool)
			}
		}
	}
}
