package runtime

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Djarvur/ass-guard-agent/kit/mcp"
	"github.com/Djarvur/ass-guard-agent/kit/session"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// CommandCatalog is the D-17 injection seam: everything the kit consumes
// from the command/skill/plugin world — discovery results, invocation
// expansion, mutability boundaries, listings, agent definitions, hook
// runners, the Skill tool's executor, MCP lower layers, and the rescan
// lifecycle — behind ONE kit-defined interface. The app composition
// (internal/acpserve) implements it over its ecosystem packages
// (internal/ecosys discovery + precedence, internal/openspec mutability);
// `.claude/` discovery and OpenSpec hosting stay app-side, and the kit is
// ecosystem-blind by construction.
//
// Contract (who implements / what degrades): the reference app installs the
// adapter via SetCatalog at the composition step where LoadCommandRegistry
// once ran; a nil (never-installed) catalog degrades to the emptyCatalog
// documented below — expansion misses turn into plain text, listings and
// merges no-op, hooks and agents stay unwired, builtins still resolve
// (the builtin chain is kit machinery) — never a panic (T-25-28).
//
// Method-set note (the promoted-parsing split): ParseInvocation and
// ParseMentions are NOT catalog methods — they are pure text parsers with
// zero ecosystem knowledge, promoted kit-side (below) so invocation
// detection and mention parsing work identically with or without a
// catalog (the nil-degradation invariant: /undo routing and class-B
// detection must never depend on discovery having succeeded).
type CommandCatalog interface { //nolint:interfacebloat // D-17 seam: every method is a live site (D-12)
	// Entries returns the slash-eligible discovered entries in D-02 chain
	// order (skills, then agents, then file commands) for chain building.
	// The adapter applies the user-invocable filter (20-04 D-04) and every
	// precedence/validation rule app-side — the kit receives post-
	// validation rows only. Nil/empty = discovery found nothing.
	Entries() []CatalogEntry

	// Rediscover re-runs discovery app-side (the 20-05 rescan body): the
	// adapter swaps its internal registry wholesale and subsequent calls
	// reflect the fresh state. An error keeps the PREVIOUS state (the
	// "keeping the current chain" degrade) — never a partial swap.
	Rediscover() error

	// LookupCommand resolves one chain winner's expandable body: kind is
	// the winner's EntryKind (the chain decided WHO won; the catalog
	// fetches the body). File commands return verbatim; skill commands
	// re-read the SKILL.md body fresh (the rescan-path freshness
	// guarantee). ok=false for unknown names — plain text, never an error.
	LookupCommand(kind, key string) (Command, bool)

	// SkillBody returns one skill's fresh on-disk body (the empty-body
	// loud-reject check). ok=false when the skill is unknown.
	SkillBody(key string) (string, bool)

	// Mutating reports whether one command key is boundary-opening (the
	// D-11 mutability table, resolved app-side from OpenSpec's vocabulary —
	// no mutability literal exists kit-side, T-25-29).
	Mutating(key string) bool

	// Agents returns the discovered agent definitions as kit AgentDef
	// mirrors (the ONE mapping site lives app-side — Pitfall 3). Nil = no
	// agents discovered (the advisory listing degrades).
	Agents() map[string]session.AgentDef

	// Hooks builds the per-session lifecycle-hook surface (the kit
	// session.Hooks interface) for one session's identity/paths. Nil
	// return = no hooks wired (every hook seam no-ops).
	Hooks(sessionID, dir, transcriptPath string) session.Hooks

	// SkillExecute returns the Skill tool's per-session Execute override
	// (resolution by registry key). Nil = the Skill tool keeps its
	// captured stub behavior.
	SkillExecute() toolcat.Stub

	// SkillListing renders the skills listing block (the captured zcode
	// system-message shape). Empty = no merge.
	SkillListing() string

	// AgentListing renders the agent-type listing block. Empty = no merge.
	AgentListing() string

	// MemoryInjection renders the AGENTS.md/CLAUDE.md memory body for dir
	// (mtime-cached, capped — the app-side discovery contract). Empty = no
	// merge.
	MemoryInjection(dir string) string

	// MCPLayers returns the NON-project MCP server set (user + plugin
	// layers) merged BELOW the project .mcp.json. Empty = project-only.
	MCPLayers() []mcp.ServerConfig

	// MemoryFiles lists the discovered memory sources for the /memory
	// builtin (read-only). Empty = none discovered.
	MemoryFiles(dir string) []MemoryFile

	// Counts returns the discovered registry surface sizes for the /doctor
	// builtin. Zero values = nothing loaded.
	Counts() CatalogCounts
}

// CatalogEntry is one discovered, slash-eligible chain row (the kit-neutral
// projection of a discovered skill/agent/command): the chain overlays these
// in order with first-writer-wins per name (D-02) and reserved-name
// rejection (D-01). Agent rows carry the dispatch definition (the kit
// AgentDef mirror); the Path feeds the reserved-name shadow warning.
type CatalogEntry struct {
	Kind        EntryKind
	Name        string
	Description string
	// InputHint is the optional wire-input placeholder hint ("" = none).
	InputHint string
	Path      string
	// Agent is the dispatch definition for EntryKindAgent rows (nil for
	// the other kinds).
	Agent *session.AgentDef
}

// EntryKind is the discovered-entry vocabulary (the chain-winner kinds the
// catalog resolves and LookupCommand switches on).
type EntryKind string

const (
	// EntryKindSkill — a discovered `<name>/SKILL.md` skill.
	EntryKindSkill EntryKind = "skill"
	// EntryKindAgent — a discovered agent definition.
	EntryKindAgent EntryKind = "agent"
	// EntryKindFile — a discovered commands/<name>.md file command.
	EntryKindFile EntryKind = "file"
)

// Command is one expandable slash-command body as the kit consumes it (the
// mirror of the discovered command record's expansion-relevant fields):
// Expand substitutes args with the captured zcode semantics (promoted
// below), Path names the provenance source. Class-A builtins synthesize
// kit Commands directly.
type Command struct {
	Name        string
	Description string
	Body        string
	Path        string
}

// MemoryFile is one discovered memory source row for the /memory listing
// (the mirror of the app discovery record's display fields).
type MemoryFile struct {
	Path      string
	OrigBytes int
	SkipNote  string
}

// CatalogCounts is the discovered-surface size triple for /doctor.
type CatalogCounts struct {
	Commands int
	Skills   int
	Agents   int
}

// emptyCatalog is the nil-catalog degradation (T-25-28): every method
// returns its documented empty value — expansion misses (plain text), no
// listings/merges, no hooks/agents, no MCP layers, zero counts. Builtins
// keep resolving: the builtin chain half is kit machinery, not discovery.
// Rediscover is a no-op success (there is nothing to rediscover; the chain
// rebuilds to the same builtin-only view).
type emptyCatalog struct{}

func (emptyCatalog) Entries() []CatalogEntry { return nil }
func (emptyCatalog) Rediscover() error       { return nil }
func (emptyCatalog) LookupCommand(string, string) (Command, bool) {
	return Command{}, false
}
func (emptyCatalog) SkillBody(string) (string, bool)     { return "", false }
func (emptyCatalog) Mutating(string) bool                { return false }
func (emptyCatalog) Agents() map[string]session.AgentDef { return nil }
func (emptyCatalog) Hooks(string, string, string) session.Hooks { //nolint:ireturn // seam shape
	return nil
}
func (emptyCatalog) SkillExecute() toolcat.Stub      { return nil }
func (emptyCatalog) SkillListing() string            { return "" }
func (emptyCatalog) AgentListing() string            { return "" }
func (emptyCatalog) MemoryInjection(string) string   { return "" }
func (emptyCatalog) MCPLayers() []mcp.ServerConfig   { return nil }
func (emptyCatalog) MemoryFiles(string) []MemoryFile { return nil }
func (emptyCatalog) Counts() CatalogCounts           { return CatalogCounts{} }

// liveCatalog returns the injected catalog or the empty degradation.
func (r *Runner) liveCatalog() CommandCatalog { //nolint:ireturn // the catalog interface is the seam shape
	if r.commandCatalog != nil {
		return r.commandCatalog
	}

	return emptyCatalog{}
}

// --- promoted pure parsing (verbatim from the ecosystem originals) -------
//
// The three parsers below are PROMOTED (mirror-over-promote: pure
// functions may promote): they are pure text transformations with zero
// ecosystem knowledge — no registry, no filesystem, no `.claude/` layout.
// They are promoted rather than catalog-method'd so invocation detection,
// mention parsing, and argument substitution behave IDENTICALLY with or
// without a catalog (the nil-degradation invariant). The app-side
// originals remain for the discovery package's own consumers; these kit
// copies are the runtime path, pinned byte-for-byte by the relocated
// expansion/mention batteries.

// invocationRe is the zcode command-token regex: a leading `/` followed by a
// key matching ^[a-z0-9][a-z0-9_:-]{0,63}$, then optional whitespace + the
// verbatim argument remainder. A leading newline is tolerated (the invocation
// must simply be the first non-newline content of the text — anything else is
// ordinary prose, never an invocation).
var invocationRe = regexp.MustCompile(`^/([a-z0-9][a-z0-9_:-]{0,63})(\s+(.*))?$`)

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

// maxPositional is the highest positional token Expand substitutes ($1..$9 —
// zcode's positional set; higher tokens stay literal).
const maxPositional = 9

// Expand substitutes args into the command body with EXACT zcode semantics
// (PITFALLS Pitfall 3 — the mimicry target wins over Claude Code wherever
// they differ). Pure text substitution; it NEVER executes anything
// (" !`cmd` " and ${ARGUMENTS} stay literal — zcode rejects both) and never
// re-parses its own output (single pass per token; a substituted literal
// does not re-expand).
//
// The contract:
//
//   - $ARGUMENTS → the full argument string, verbatim (no field splitting);
//   - $1..$9 → whitespace-split positionals; out-of-range → empty ($0 →
//     empty too — zcode positionals are 1-based);
//   - args supplied but NO placeholder exists in the body (no $ARGUMENTS and
//     no $0..$9 token) → args appended under a "User arguments:" heading;
//   - substitution applies inside code fences (no fence skipping).
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
// args could land in ($ARGUMENTS or any $0..$9 literal) — the append-
// suppression rule (args are appended only when NO placeholder exists).
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

// mentionTrailingPunct is the trailing-punctuation set never part of a
// mention's path: a mention closed by sentence punctuation keeps a clean
// path — @docs/readme.md. → path docs/readme.md.
const mentionTrailingPunct = ".,:"

// Mention is one parsed @-mention: Token is the raw mention as typed (@path,
// INCLUDING its quote characters, EXCLUDING the trailing punctuation trimmed
// from the path); Path is the unquoted path the token carries. IsDir is
// UNKNOWN at parse time — the parser is deliberately blind to the
// filesystem; the resolution ladder stats the path and fills IsDir there.
type Mention struct {
	Token string
	Path  string
	IsDir bool
}

// ParseMentions extracts every @-mention token from text, in order. The
// contract:
//
//   - whitespace-delimited, token-INITIAL @ only — a mid-word @ (email-like
//     foo@bar) is never a mention;
//   - a quote directly after the @ (@"path with spaces" / @'path with
//     spaces') captures spaces into ONE token; a path tail may follow the
//     closing quote (@"my docs"/file.md);
//   - trailing punctuation (. , :) never joins the path — quotes protect
//     their interior, the unquoted tail is trimmed;
//   - an unterminated quote, a bare @, or @ followed only by punctuation is
//     not a mention;
//   - zero mentions → nil (no error — absence is ordinary prose).
//
// Pure text extraction: no I/O, no resolution, no errors.
func ParseMentions(text string) []Mention {
	var out []Mention

	i, n := 0, len(text)

	for i < n {
		if isMentionSpace(text[i]) {
			i++

			continue
		}

		if text[i] == '@' {
			if m, next, ok := scanMention(text, i); ok {
				out = append(out, m)
				i = next

				continue
			}
		}

		// Ordinary token (or a failed mention shape): skip to whitespace.
		for i < n && !isMentionSpace(text[i]) {
			i++
		}
	}

	return out
}

// scanMention attempts to parse one mention starting at text[start] == '@'.
// It returns the mention, the scan resume offset (the token's end, including
// any trailing punctuation skipped), and ok=false when the shape is not a
// mention (bare @, only-punctuation path, unterminated quote).
//
//nolint:nonamedreturns // the (value, offset, ok) triple reads best named
func scanMention(text string, start int) (m Mention, next int, ok bool) {
	n := len(text)
	i := start + 1

	var path strings.Builder

	// A quote directly after the @ opens a space-capturing segment; the
	// closing quote may be followed by an ordinary path tail.
	if i < n && (text[i] == '"' || text[i] == '\'') {
		quote := text[i]
		i++

		segStart := i
		for i < n && text[i] != quote {
			i++
		}

		if i >= n {
			return Mention{}, 0, false // unterminated quote — not a mention
		}

		path.WriteString(text[segStart:i])
		i++ // closing quote
	}

	// Ordinary tail: everything up to whitespace.
	tailStart := i
	for i < n && !isMentionSpace(text[i]) {
		i++
	}

	tail := strings.TrimRight(text[tailStart:i], mentionTrailingPunct)
	path.WriteString(tail)

	if path.Len() == 0 {
		return Mention{}, 0, false
	}

	// The token ends before the trimmed punctuation; the scan itself resumes
	// at the whitespace boundary (next = i).
	tokenEnd := i - (len(text[tailStart:i]) - len(tail))

	return Mention{Token: text[start:tokenEnd], Path: path.String()}, i, true
}

// isMentionSpace reports whether c is a mention-token delimiter (ASCII
// whitespace — the parse table's "whitespace-delimited" set).
func isMentionSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}
