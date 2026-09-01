// Package perm implements the permission rule grammar (D-02) and the
// permissions.yaml persistence layer (D-01/D-03): a pure, dependency-free
// evaluation core the Phase-17 gate chokepoint (17-02) consults per tool
// call. ACP-01 prohibition honored by construction — nothing in this
// package imports or gains a network path; permission rules and decisions
// never leave the machine.
package perm

import (
	"errors"
	"fmt"
	"strings"
)

// Effect is the action a rule carries. ParseRule leaves it EffectNone; the
// list a rule lives in assigns the effect (NewRuleSet).
type Effect uint8

const (
	// EffectNone marks a parsed-but-not-yet-placed rule (ParseRule output
	// before the owning list assigns the effect).
	EffectNone Effect = iota
	// EffectDeny blocks the matching call. Evaluated FIRST — a deny
	// anywhere beats any allow (D-02).
	EffectDeny
	// EffectAsk routes the matching call to the gate's permission surface
	// (D-06).
	EffectAsk
	// EffectAllow executes the matching call without a dialog. Evaluated
	// LAST (D-02).
	EffectAllow
)

// Verdict is the rule-set outcome for one (toolName, primaryArg) subject.
// Unmatched is the zero value: an absent rule set matches nothing.
type Verdict uint8

const (
	// Unmatched means no rule matched. The mode+class decision at the gate
	// chokepoint (ungated execute / gated dialog / automation decline) is
	// the CALLER's (D-05/D-06) — never this package's.
	Unmatched Verdict = iota
	// VerdictAllow — an allow rule matched (execute).
	VerdictAllow
	// VerdictAsk — an ask rule matched (permission surface).
	VerdictAsk
	// VerdictDeny — a deny rule matched (never execute).
	VerdictDeny
)

// mcpPrefix and mcpSep name the CC MCP namespace: mcp__<server>__<tool>.
const (
	mcpPrefix = "mcp__"
	mcpSep    = "__"
)

// compoundTool is the tool whose primary argument is compound-split into
// subcommands before matching (CC parity: the compound-command discipline
// is a Bash-rule concept).
const compoundTool = "Bash"

// reasonUnanchoredAllow is the warning reason for an inert allow glob.
const reasonUnanchoredAllow = "unanchored allow glob skipped (allow globs require a literal mcp__<server>__ prefix)"

// reasonMidStarTool is the warning reason for a tool selector carrying a star
// that is NOT trailing (17-REVIEW WR-03): the matcher honors only a TRAILING
// star — every other star is literal — so a mid-star selector ("Bash*git",
// "mcp__github__issue*c") is unmatchable by construction. The rule is skipped
// LOUDLY instead of being accepted as a silently dead line (for a deny rule
// that is a false sense of coverage). Trailing-star globs and bare stars are
// untouched.
const reasonMidStarTool = "mid-pattern star in tool selector never matches (rule skipped)"

// Parse-rule validation sentinels — every malformed shape degrades to an
// error (the caller skips + warns), never a panic.
var (
	errEmptyRule        = errors.New("empty rule")
	errUnbalancedParens = errors.New("unbalanced parentheses")
	errEmptySpecifier   = errors.New("empty specifier")
	errBadToolName      = errors.New("invalid tool name characters")
	errParensSpecifier  = errors.New("parenthesis inside specifier")
)

// Rule is one parsed permission rule: a tool-name selector (glob-capable,
// mcp__-aware) plus an optional specifier pattern for the tool's primary
// argument.
type Rule struct {
	// Tool is the rule's tool selector: a bare tool name ("Write"), a
	// trailing-star tool-name glob ("mcp__github__get_*"), or the tool side
	// of Tool(specifier). A bare mcp__<server> name selects the whole
	// server namespace.
	Tool string
	// Pattern is the Tool(specifier) pattern; empty for bare rules (any
	// input). A trailing star is a prefix wildcard; a trailing ":*" was
	// normalized to " *" at parse (D-02 equivalence, end-anchored only).
	Pattern string
	// Effect is assigned by the owning list (deny/ask/allow), never by
	// ParseRule.
	Effect Effect
}

// Warning names one rule line skipped during NewRuleSet construction — the
// package's structured warning sink (returned diagnostics; no logging).
type Warning struct {
	// Rule is the raw rule string as written.
	Rule string
	// Reason says why the line was skipped (parse failure or inert allow
	// glob).
	Reason string
}

// RuleSet is the parsed, evaluated form of a permissions rule file: three
// ordered rule lists consulted in the FIXED order deny → ask → allow.
//
// Evaluation contract (D-02, CC parity — the verified CC rule model,
// semantics quoted from code.claude.com/docs/en/permissions): "Rules are
// evaluated in order: deny, then ask, then allow. The first match in that
// order determines the outcome, and rule specificity doesn't change the
// order." A deny anywhere beats any allow, and a matching ask prompts even
// when a more specific allow also matches. Bash primary arguments are
// compound-split (&&, ||, ;, |, |&, &, newlines) and every subcommand is
// matched independently: any deny subcommand denies, any explicit ask
// subcommand asks, and an unmatched subcommand fails safe to Unmatched (an
// allow must cover EVERY subcommand before the compound executes). When no
// rule matches, Evaluate returns Unmatched — the mode+class decision
// (ungated execute / gated dialog / automation decline) is the CALLER's at
// the gate chokepoint (D-05/D-06), never this package's.
type RuleSet struct {
	Deny  []Rule
	Ask   []Rule
	Allow []Rule
}

// ParseRule parses one rule string in the D-02 grammar: a bare tool name
// ("Write" — matches the tool with any input), a Tool(specifier) form
// ("Bash(git *)" — matches the tool with a primary argument matching the
// specifier), or a trailing-star tool-name glob. A trailing ":*" specifier
// suffix is equivalent to a trailing " *" and recognized ONLY at pattern
// end (a mid-pattern colon-star is literal). Malformed strings (empty,
// unbalanced parentheses, invalid tool-name characters) return an error —
// the caller skips + warns. Parsing never panics.
func ParseRule(s string) (Rule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Rule{}, fmt.Errorf("%w: %q", errEmptyRule, s)
	}

	tool, pattern := s, ""

	if open := strings.Index(s, "("); open >= 0 {
		if !strings.HasSuffix(s, ")") {
			return Rule{}, fmt.Errorf("%w: %q", errUnbalancedParens, s)
		}

		tool, pattern = s[:open], s[open+1:len(s)-1]

		switch {
		case strings.ContainsAny(tool, "()"):
			return Rule{}, fmt.Errorf("%w: %q", errUnbalancedParens, s)
		case strings.ContainsAny(pattern, "()"):
			return Rule{}, fmt.Errorf("%w: %q", errParensSpecifier, s)
		case strings.TrimSpace(pattern) == "":
			return Rule{}, fmt.Errorf("%w: %q", errEmptySpecifier, s)
		}
	} else if strings.Contains(s, ")") {
		return Rule{}, fmt.Errorf("%w: %q", errUnbalancedParens, s)
	}

	if !validToolName(tool) {
		return Rule{}, fmt.Errorf("%w: %q", errBadToolName, tool)
	}

	if base, found := strings.CutSuffix(pattern, ":*"); found {
		pattern = base + " *"
	}

	return Rule{Tool: tool, Pattern: pattern}, nil
}

// NewRuleSet parses the three ordered rule lists (deny, ask, allow) into a
// RuleSet. Malformed lines are skipped with one Warning each; an unanchored
// allow glob — a tool-name glob without a literal mcp__<server>__ prefix
// ("*", "B*", "mcp__*") — is skipped as inert with a Warning (CC asymmetry:
// the same glob in the deny or ask list DOES match). Parsing never panics.
func NewRuleSet(deny, ask, allow []string) (RuleSet, []Warning) {
	var (
		rs RuleSet
		ws []Warning
	)

	rs.Deny, ws = parseList(deny, EffectDeny, ws)
	rs.Ask, ws = parseList(ask, EffectAsk, ws)
	rs.Allow, ws = parseList(allow, EffectAllow, ws)

	return rs, ws
}

// Evaluate returns the rule-set verdict for (toolName, primaryArg) — see
// the RuleSet evaluation contract above.
func (rs RuleSet) Evaluate(toolName, primaryArg string) Verdict {
	if toolName == compoundTool {
		if subs := SplitCompound(primaryArg); len(subs) > 1 {
			return rs.evaluateCompound(toolName, subs)
		}
	}

	return rs.evaluateSingle(toolName, primaryArg)
}

// MCPName builds the namespaced mcp__<server>__<tool> tool name (the CC
// namespace D-02 imports; the session's MCP-host mapping consumes these —
// RESEARCH Pitfall 7).
func MCPName(server, tool string) string {
	return mcpPrefix + server + mcpSep + tool
}

// SplitMCPName splits a fully-qualified mcp__<server>__<tool> name.
// ok is false for non-MCP names and server-only names like "mcp__puppeteer"
// (no tool part).
func SplitMCPName(name string) (server, tool string, ok bool) { //nolint:nonamedreturns // pinned artifact signature
	rest, found := strings.CutPrefix(name, mcpPrefix)
	if !found {
		return "", "", false
	}

	server, tool, found = strings.Cut(rest, mcpSep)
	if !found || server == "" || tool == "" {
		return "", "", false
	}

	return server, tool, true
}

// SplitCompound splits a shell command line into its subcommands on the CC
// compound separators (&&, ||, ;, |, |&, and newlines), trimming whitespace
// and dropping empty segments — every returned subcommand is matched
// independently against the rule set. The scan is QUOTE-AWARE (17-REVIEW
// WR-02): separators inside single or double quotes split nothing ("git
// commit -m \"fix a && b\"" is ONE subcommand, not two false compounds).
func SplitCompound(cmd string) []string {
	var subs []string

	start := 0

	var quote byte // 0 = unquoted; else the active quote character (' or ")

	for i := 0; i < len(cmd); {
		if quote == 0 {
			switch c := cmd[i]; c {
			case '\'', '"':
				quote = c
			default:
				if n := separatorLen(cmd, i); n > 0 {
					if sub := strings.TrimSpace(cmd[start:i]); sub != "" {
						subs = append(subs, sub)
					}

					i += n
					start = i

					continue
				}
			}
		} else if cmd[i] == quote {
			quote = 0
		}

		i++
	}

	if sub := strings.TrimSpace(cmd[start:]); sub != "" {
		subs = append(subs, sub)
	}

	return subs
}

// hasSubstitution reports whether the command carries shell command
// substitution — `$(...)` or backticks. Shell semantics honored to the extent
// the guard needs (WR-02): substitution is LIVE inside double quotes and only
// dead inside single quotes; an unquoted backslash escapes the next byte. The
// guard is intentionally one-sided: a deny or ask rule still matches a
// substitution-bearing command (deny stays strong), but an ALLOW never can —
// the substitution's payload executes without appearing as a separate
// subcommand, so allowing it would defeat the compound discipline ("an allow
// must cover EVERY subcommand") with no separator anywhere in the visible
// text.
func hasSubstitution(cmd string) bool {
	var quote byte // 0 = unquoted; '\'' = literal span; '"' = double-quote span

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]

		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case quote == '"':
			switch c {
			case '"':
				quote = 0
			case '\\':
				i++ // skip the escaped byte ("\" / "\$" are literal)
			case '`':
				return true
			case '$':
				if i+1 < len(cmd) && cmd[i+1] == '(' {
					return true
				}
			}
		case c == '\'':
			quote = '\''
		case c == '"':
			quote = '"'
		case c == '\\':
			i++ // skip the escaped byte (a quoted separator, or an escaped \$)
		case c == '`':
			return true
		case c == '$':
			if i+1 < len(cmd) && cmd[i+1] == '(' {
				return true
			}
		}
	}

	return false
}

// evaluateSingle scans the three lists in the fixed order; the first match
// decides (D-02). The ALLOW scan is substitution-guarded (WR-02): an allow
// never matches a command substitution — its payload would execute under the
// allow without ever appearing as a split subcommand. Deny and ask scan the
// raw subject (a deny that matches `rm $(x)` still denies).
func (rs RuleSet) evaluateSingle(toolName, primaryArg string) Verdict {
	for i := range rs.Deny {
		if rs.Deny[i].matches(toolName, primaryArg) {
			return VerdictDeny
		}
	}

	for i := range rs.Ask {
		if rs.Ask[i].matches(toolName, primaryArg) {
			return VerdictAsk
		}
	}

	if hasSubstitution(primaryArg) {
		return Unmatched // fail safe: the dialog (or the mode decision) owns it
	}

	for i := range rs.Allow {
		if rs.Allow[i].matches(toolName, primaryArg) {
			return VerdictAllow
		}
	}

	return Unmatched
}

// evaluateCompound applies the compound discipline: every subcommand is
// matched independently — any deny wins, any explicit ask wins, an
// unmatched subcommand fails safe to Unmatched, and only an all-allow
// compound is allowed.
func (rs RuleSet) evaluateCompound(toolName string, subs []string) Verdict {
	var sawAsk, sawUnmatched, sawAllow bool

	for _, sub := range subs {
		switch rs.evaluateSingle(toolName, sub) {
		case VerdictDeny:
			return VerdictDeny
		case VerdictAsk:
			sawAsk = true
		case VerdictAllow:
			sawAllow = true
		case Unmatched:
			sawUnmatched = true
		}
	}

	switch {
	case sawAsk:
		return VerdictAsk
	case sawUnmatched:
		return Unmatched
	case sawAllow:
		return VerdictAllow
	default:
		return Unmatched
	}
}

// matches reports whether the rule selects (toolName, primaryArg).
func (r Rule) matches(toolName, primaryArg string) bool {
	if !r.matchesTool(toolName) {
		return false
	}

	if r.Pattern == "" {
		return true // bare rule — any input
	}

	return matchGlob(r.Pattern, primaryArg)
}

// matchesTool matches the rule's tool selector: trailing-star glob, exact
// name, or a bare mcp__<server> selecting the whole server namespace.
func (r Rule) matchesTool(name string) bool {
	if base, found := strings.CutSuffix(r.Tool, "*"); found {
		return strings.HasPrefix(name, base)
	}

	if r.Tool == name {
		return true
	}

	return strings.HasPrefix(r.Tool, mcpPrefix) && strings.HasPrefix(name, r.Tool+mcpSep)
}

// parseList parses one effect's lines, appending warnings for skipped lines.
func parseList(lines []string, eff Effect, ws []Warning) ([]Rule, []Warning) {
	rules := make([]Rule, 0, len(lines))

	for _, line := range lines {
		r, err := ParseRule(line)
		if err != nil {
			ws = append(ws, Warning{Rule: line, Reason: err.Error()})

			continue
		}

		// WR-03: a tool selector whose star is not trailing can never match —
		// warn-and-skip in EVERY list (the inert-allow-glob precedent), never
		// a silently dead rule.
		if i := strings.Index(r.Tool, "*"); i >= 0 && i != len(r.Tool)-1 {
			ws = append(ws, Warning{Rule: line, Reason: reasonMidStarTool})

			continue
		}

		if eff == EffectAllow && unanchoredAllowGlob(r.Tool) {
			ws = append(ws, Warning{Rule: line, Reason: reasonUnanchoredAllow})

			continue
		}

		r.Effect = eff
		rules = append(rules, r)
	}

	return rules, ws
}

// unanchoredAllowGlob reports whether a tool-name glob lacks the literal
// mcp__<server>__ prefix CC requires before an allow glob is honored.
// Literal tool names (no star) are always anchored.
func unanchoredAllowGlob(tool string) bool {
	prefix, _, found := strings.Cut(tool, "*")
	if !found {
		return false // literal tool name — always anchored
	}

	return !strings.HasPrefix(prefix, mcpPrefix) ||
		!strings.Contains(prefix[len(mcpPrefix):], mcpSep)
}

// matchGlob is the bounded prefix-wildcard matcher (T-17-03): a trailing
// star is a prefix match; every other star is LITERAL; no star is exact
// equality. No regex engine is compiled anywhere in the package.
func matchGlob(pattern, subject string) bool {
	if base, found := strings.CutSuffix(pattern, "*"); found {
		return strings.HasPrefix(subject, base)
	}

	return pattern == subject
}

// validToolName enforces the tool-name charset: letters, digits, underscore
// (the mcp__ namespace is underscores), and the wildcard star.
func validToolName(tool string) bool {
	if tool == "" {
		return false
	}

	for _, r := range tool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '*':
		default:
			return false
		}
	}

	return true
}

// separatorLen returns the length of the compound separator at cmd[i], or
// zero. Two-character separators are matched first (|& before |, && and ||
// before &).
func separatorLen(cmd string, i int) int {
	if i+1 < len(cmd) {
		switch pair := cmd[i : i+2]; pair {
		case "|&", "&&", "||":
			return len(pair)
		}
	}

	switch cmd[i] {
	case ';', '|', '&', '\n':
		return 1
	default:
		return 0
	}
}
