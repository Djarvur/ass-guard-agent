package ecosys

import (
	"regexp"
	"strconv"
	"strings"
)

// invocationRe is the zcode command-token regex: a leading `/` followed by a
// key matching ^[a-z0-9][a-z0-9_:-]{0,63}$, then optional whitespace + the
// verbatim argument remainder. A leading newline is tolerated (the invocation
// must simply be the first non-newline content of the text — anything else is
// ordinary prose, never an invocation).
var invocationRe = regexp.MustCompile(`^/([a-z0-9][a-z0-9_:-]{0,63})(\s+(.*))?$`)

// maxPositional is the highest positional token Expand substitutes ($1..$9 —
// zcode's positional set; higher tokens stay literal).
const maxPositional = 9

// ParseInvocation reports whether text opens with a slash-command invocation
// and splits it into (key, args). ok=false means ordinary text (the caller
// leaves it untouched — unknown /foo is normal input, not an error). The
// registry-miss decision belongs to the CALLER: /foo parses fine here.
func ParseInvocation(text string) (key, args string, ok bool) { //nolint:nonamedreturns // named shape
	trimmed := strings.TrimLeft(text, "\r\n")

	m := invocationRe.FindStringSubmatch(trimmed)
	if m == nil {
		return "", "", false
	}

	return m[1], m[3], true
}

// Expand substitutes args into the command body with EXACT zcode semantics
// (PITFALLS Pitfall 3 — the mimicry target wins over Claude Code wherever they
// differ). Pure text substitution; it NEVER executes anything (“ !`cmd` “
// and ${ARGUMENTS} stay literal — zcode rejects both) and never re-parses its
// own output (single pass per token; a substituted literal does not re-expand).
//
// The contract:
//
//   - $ARGUMENTS → the full argument string, verbatim (no field splitting);
//   - $1..$9 → whitespace-split positionals; out-of-range → empty ($0 →
//     empty too — zcode positionals are 1-based);
//   - args supplied but NO placeholder exists in the body (no $ARGUMENTS and
//     no $0..$9 token) → args appended under a "User arguments:" heading;
//   - substitution applies inside code fences (no fence skipping).
//
//nolint:gocritic // value receiver: Expand reads naturally on registry values
func (c Command) Expand(args string) string {
	fields := strings.Fields(args)

	body := strings.ReplaceAll(c.Body, "$ARGUMENTS", args)

	// Replace $9 down to $1 so a lower token never rewrites inside a higher
	// one's digits. One ReplaceAll per token — no loops over the output.
	for n := maxPositional; n >= 1; n-- {
		body = strings.ReplaceAll(body, "$"+strconv.Itoa(n), fieldAt(fields, n))
	}

	// $0 is not a positional placeholder (1-based); it expands to empty.
	body = strings.ReplaceAll(body, "$0", "")

	if args != "" && !hasPlaceholder(c.Body) {
		body += "\n\nUser arguments:\n" + args
	}

	return body
}

// hasPlaceholder reports whether the body carries any substitution token the
// args could land in ($ARGUMENTS or any $0..$9 literal) — the append-suppression
// rule (FEATURES §a: args are appended only when NO placeholder exists).
func hasPlaceholder(body string) bool {
	if strings.Contains(body, "$ARGUMENTS") {
		return true
	}

	for n := 0; n <= maxPositional; n++ {
		if strings.Contains(body, "$"+strconv.Itoa(n)) {
			return true
		}
	}

	return false
}

// fieldAt returns the n-th (1-based) whitespace-split field, or "" when
// out of range (the zcode out-of-range rule).
func fieldAt(fields []string, n int) string {
	if n < 1 || n > len(fields) {
		return ""
	}

	return fields[n-1]
}
