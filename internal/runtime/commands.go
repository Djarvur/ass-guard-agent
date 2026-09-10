package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// 20-01 (CMDS-01): the slash-command resolver chain. One immutable view over
// the discovered registry: builtins → skills → agents → file commands,
// winners-only per name (D-02), with the thirteen builtin names RESERVED and
// unoverridable (D-01) and the advertisement generated FROM the same map the
// resolver resolves through (D-04 — autocomplete cannot lie).
//
// Concurrency contract (RESEARCH Pattern 1): a commandChain is immutable once
// built; the Runner holds it behind an atomic pointer. Turn goroutines and
// the advertisement builder LOAD the pointer; a rescan builds a fresh chain
// off-thread and swaps (20-05). The chain is a VIEW — it never mutates the
// underlying ecosys.Registry, and a shadowed entry stays reachable through
// its native surface (agent dispatch via the Agent tool, skills via the
// Skill tool); only the slash name is exclusive.

// Chain-entry kinds (the D-02 order is build-time overlay order, not a
// runtime walk: first writer wins and the chain is flat by name).
const (
	chainKindBuiltin = "builtin"
	chainKindSkill   = "skill"
	chainKindAgent   = "agent"
	chainKindFile    = "file"
)

// statusUnsetLabel marks an unresolvable /status field (never an empty line);
// tierHeavyDefault is the implicit session tier when config leaves it unset.
const statusUnsetLabel = "(unset)"

// builtinHandler executes one class-B command against live session state and
// returns the output text plus an OUTCOME override for the local_command
// record ("" = the default "ok"; the delegation family records its
// "unavailable: ..." degrades here — CMDS-02). Pure-local, control-plane
// fast, ZERO provider calls. Handlers must never touch the network (the
// FAST-CONTROL budget discipline for the network-bound family lands in 20-02).
type builtinHandler func(ctx context.Context, r *Runner, sess *session.Session, turnID, args string) (string, string)

// chainEntry is one name's winner in the resolver chain.
type chainEntry struct {
	name string
	kind string // chainKind* vocabulary
	key  string // the registry key (skills/agents/file); "" for builtins
	desc string
	hint string // argument hint for the wire input object; "" omits Input

	// builtin class-B: the live handler. nil = a reserved name whose command
	// is not live yet (falls through as plain text; its plan-20-02 task wires
	// the handler) — the NAME is still reserved against discovery (D-01).
	handler builtinHandler

	// builtin class-A (/init, CMDS-03): a synthesized ecosys.Command the
	// EXISTING expandUserBlocks seam expands. Registered in the chain as a
	// reservation; its prompt body is plan 20-04's deliverable — until then
	// the expansion path misses in r.reg.Commands and the text stays plain.
	classA    ecosys.Command
	hasClassA bool

	// Discovered winners (kind-scoped): the registry value the entry shadows
	// for slash purposes. The loser's native surface keeps the registry maps
	// untouched (the chain is a view).
	skill *ecosys.Skill
	agent *ecosys.Agent
	file  *ecosys.Command
}

// builtinClassB describes one reserved class-B name's live surface.
type builtinClassB struct {
	name    string
	desc    string
	hint    string
	handler builtinHandler
}

// reservedNames is the D-01 published contract: the thirteen class-B names
// (CMDS-02 + undo, 23-05/SEEDG-03) plus init (class-A, CMDS-03). A
// discovered file, skill, or agent sharing one of these names NEVER fires via
// slash — dropped at chain build with exactly one structured warning naming
// the file and the reserved name. Growth of this list is costly by design (a
// later builtin silently shadows an existing discovered command from the
// user's view) — the shadow-check warning is the mitigation this set ships
// with.
var reservedNames = map[string]struct{}{ //nolint:gochecknoglobals // the D-01 published contract table
	"help": {}, "status": {}, "cost": {}, "mcp": {}, "memory": {},
	"permissions": {}, "doctor": {}, "config": {}, "model": {},
	"clear": {}, "resume": {}, "compact": {}, "init": {},
	"undo": {},
}

// builtinTable is the LIVE builtin set (a subset of reservedNames — the
// remaining names become live entries in their own plan-20-02/20-04 tasks;
// they are reserved against discovery from this commit onward regardless).
// 20-01 ships exactly two live builtins: /status (the tracer) and /init
// (class-A reservation).
//
//nolint:funlen // the thirteen-command table is the deliverable
func builtinTable() []builtinClassB {
	return []builtinClassB{
		{
			name:    "status",
			desc:    "Show the live session snapshot (model, provider, turns, context usage)",
			handler: builtinStatus,
		},
		{
			name:    "help",
			desc:    "List every command this agent resolves (generated from the live chain)",
			handler: builtinHelp,
		},
		{
			name:    "memory",
			desc:    "List loaded memory sources (read-only)",
			handler: builtinMemory,
		},
		{
			name:    "permissions",
			desc:    "Show the permission-gate mode",
			handler: builtinPermissions,
		},
		{
			name:    "mcp",
			desc:    "List configured MCP servers",
			handler: builtinMcp,
		},
		{
			name:    "doctor",
			desc:    "Run the built-in health checks (workdir, config, credentials, transcript dir, registry)",
			handler: builtinDoctor,
		},
		{
			name:    "config",
			desc:    "Show the resolved scheduling config (tiers, session tier, compaction)",
			handler: builtinConfig,
		},
		{
			name:    "model",
			desc:    "Switch the session model (session-scope; nothing persisted)",
			hint:    "<slug>",
			handler: builtinModel,
		},
		{
			name:    "clear",
			desc:    "Reset the conversation context (same session, history retained)",
			handler: builtinClear,
		},
		{
			name:    "resume",
			desc:    "List resumable sessions and the resume surfaces",
			handler: builtinResume,
		},
		{
			name:    "compact",
			desc:    "Compact the conversation context now (Phase 19 machinery)",
			hint:    "[focus instructions]",
			handler: builtinCompact,
		},
		{
			name:    "cost",
			desc:    "Show session cost (provider usage endpoint when declared, transcript-derived otherwise)",
			handler: builtinCost,
		},
		{
			name:    "undo",
			desc:    "Restore the last workspace checkpoint (auto-cancels an active turn first; repeat to walk back)",
			hint:    "[N]",
			handler: builtinUndo,
		},
	}
}

// initPromptBody is the /init class-A builtin's authored prompt (20-04
// Task 3 — the one piece of authored content this phase adds): CC-parity in
// intent, $ARGUMENTS-capable for focus hints.
const initPromptBody = `Analyze this codebase and create (or update) a CLAUDE.md project guide
that helps AI coding agents work effectively here.

Inspect the repository, then write the guide with these sections:

1. **Commands** — the exact build, test, and lint commands (check Makefile,
   package scripts, CI config; prefer the shortest verified forms).
2. **Architecture** — a short overview of the directory layout and how the
   major pieces connect (entry points, core packages, data flow).
3. **Conventions** — naming, error-handling, testing, and style rules you
   can OBSERVE in the existing code (cite what you saw; do not invent).

Rules: derive everything from the actual repository — never guess a command
you did not verify exists. Keep it under 80 lines. If a CLAUDE.md already
exists, update it in place and preserve anything still accurate.

$ARGUMENTS`

// initClassAReservation is the /init class-A entry (CMDS-03): a synthesized
// ecosys.Command carrying the authored prompt body — the expansion seam
// expands it like any file command (provenance names builtin:init).
func initClassAReservation() ecosys.Command {
	return ecosys.Command{
		Name:        "init",
		Description: "Analyze the codebase and create a CLAUDE.md guide",
		Body:        initPromptBody,
		Path:        "builtin:init",
	}
}

// commandChain is the immutable winners map (one entry per resolvable name).
type commandChain struct {
	entries map[string]chainEntry
}

// resolve returns the winner entry for name (single resolution — callers
// parse the invocation exactly once and consult the chain exactly once; the
// invocationFor discipline generalized).
func (c *commandChain) resolve(name string) (chainEntry, bool) {
	e, ok := c.entries[name]

	return e, ok
}

// advertisement projects the winners onto the v1 AvailableCommand frames
// (D-04): sorted by name, each winning name exactly once, shadowed entries
// absent (not annotated), the Input object omitted entirely when the entry
// carries no hint. A FRESH slice every call — the caller may swap the chain
// between calls and the frames must never alias chain internals.
func (c *commandChain) advertisement() []acp.AvailableCommandFrame {
	names := slices.Sorted(maps.Keys(c.entries))

	out := make([]acp.AvailableCommandFrame, 0, len(names))

	for _, name := range names {
		e := c.entries[name]

		frame := acp.AvailableCommandFrame{Name: e.name, Description: e.desc}
		if e.hint != "" {
			frame.Input = &acp.AvailableCommandInputFrame{Hint: e.hint}
		}

		out = append(out, frame)
	}

	return out
}

// buildChain derives the resolver chain from reg (CMDS-01): live builtins
// pre-seated FIRST (D-01), then skills, agents, and file commands overlaid in
// chain order with first-writer-wins per name (D-02) — silent and
// deterministic among discovered kinds. A discovered entry colliding with a
// RESERVED name is dropped with exactly one structured warning per shadowed
// FILE per build (deduped by path — the logPluginSkipf pattern), naming the
// file and the reserved name. reg is never mutated (the chain is a view; the
// registry maps keep every discovered entry reachable on its native
// surface). warn may be nil (warnings skipped — tests that only exercise
// ordering).
func buildChain(reg ecosys.Registry, warn io.Writer) *commandChain {
	c := &commandChain{entries: make(map[string]chainEntry)}

	// D-01: builtins pre-seated first.
	for _, b := range builtinTable() {
		c.entries[b.name] = chainEntry{
			name: b.name, kind: chainKindBuiltin, desc: b.desc, hint: b.hint,
			handler: b.handler,
		}
	}

	initCmd := initClassAReservation()
	c.entries[initCmd.Name] = chainEntry{
		name: initCmd.Name, kind: chainKindBuiltin, desc: initCmd.Description,
		classA: initCmd, hasClassA: true,
	}

	warned := make(map[string]struct{})

	claim := func(e *chainEntry, srcPath string) {
		if _, reserved := reservedNames[e.name]; reserved {
			warnReserved(warn, warned, e, srcPath)

			return
		}

		if _, taken := c.entries[e.name]; taken {
			return // D-02: first writer wins, silently
		}

		c.entries[e.name] = *e
	}

	c.overlayDiscovered(reg, claim)

	return c
}

// warnReserved emits the D-01 shadow-check: exactly ONE structured warning
// per shadowed FILE per build, naming the discovered source and the reserved
// name it lost to.
func warnReserved(warn io.Writer, warned map[string]struct{}, e *chainEntry, srcPath string) {
	if warn == nil {
		return
	}

	if _, seen := warned[srcPath]; seen {
		return
	}

	warned[srcPath] = struct{}{}

	_, _ = fmt.Fprintf(warn,
		"ass-guard: discovered %s %q shadows reserved builtin name %q — "+
			"the builtin wins /%s (reachable only on its native surface)\n",
		e.kind, srcPath, e.name, e.name)
}

// overlayDiscovered walks the registry in D-02 chain order (skills → agents
// → file commands), claiming each name through claim (first-writer-wins;
// reserved names rejected). AllSkills/AllAgents/AllCommands are the loader's
// name-sorted deterministic accessors.
func (c *commandChain) overlayDiscovered(reg ecosys.Registry, claim func(e *chainEntry, srcPath string)) {
	// 20-04 (D-04): an explicit user-invocable: false skill NEVER enters the
	// chain (it cannot fire via slash); the registry map keeps it for the
	// model-invocation path — only the slash surface excludes it.
	for _, sk := range reg.AllSkills() {
		if sk.UserInvocable != nil && !*sk.UserInvocable {
			continue
		}

		claim(&chainEntry{
			name: sk.Name, kind: chainKindSkill, key: sk.Name,
			desc: sk.Description, skill: &sk,
		}, sk.Path)
	}

	for _, ag := range reg.AllAgents() {
		claim(&chainEntry{
			name: ag.Name, kind: chainKindAgent, key: ag.Name,
			desc: ag.Description, hint: agentDispatchHint, agent: &ag,
		}, ag.Path)
	}

	for _, cmd := range reg.AllCommands() {
		claim(&chainEntry{
			name: cmd.Name, kind: chainKindFile, key: cmd.Name,
			desc: cmd.Description, hint: cmd.ArgumentHint, file: &cmd,
		}, cmd.Path)
	}
}

// agentDispatchHint is the /<agent-name> wire hint (SKLS-02 dispatch: the
// typed args become the subagent's prompt).
const agentDispatchHint = "(prompt for the agent)"

// builtinStatus is the /status handler (D-08): a live session snapshot read
// from in-process state only — resolved model + tier, provider, session id,
// turn count, context-usage estimate, degraded-capability flags. Every line
// derives from live state, zero network, zero credential values (T-20-03).
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinStatus(_ context.Context, r *Runner, sess *session.Session, _, _ string) (string, string) {
	model := statusModel(r, sess)
	provider := statusProvider(r)
	tier := statusTier(r)
	turns, usage, ctxPct := r.statusTurnUsage(sess)

	var out string
	if ctxPct >= 0 {
		out = fmt.Sprintf(
			"session: %s\nprovider: %s (tier %s)\nmodel: %s\nturns: %d\ncontext: ~%d tokens in use (~%d%% of window)\n",
			sess.SessionID, provider, tier, model, turns, usage, ctxPct)
	} else {
		out = fmt.Sprintf(
			"session: %s\nprovider: %s (tier %s)\nmodel: %s\nturns: %d\ncontext: ~%d tokens in use\n",
			sess.SessionID, provider, tier, model, turns, usage)
	}

	if flags := r.degradedCapabilityFlags(sess.SessionID); flags != "" {
		out += "degraded: " + flags + "\n"
	}

	return out, ""
}

// statusModel resolves the session's EFFECTIVE model through the same ladder
// sessionFor stamps profiles with (editor stamp → tier-resolved config
// default → profile's own model).
func statusModel(r *Runner, sess *session.Session) string {
	m := r.effectiveModelFor()
	if m == "" {
		m = r.defaultTurnModel()
	}

	if m == "" {
		m = sess.Profile.Model
	}

	if m == "" {
		return statusUnsetLabel
	}

	return m
}

// statusProvider names the session provider ("" → unset label).
func statusProvider(r *Runner) string {
	if r.providerName == "" {
		return statusUnsetLabel
	}

	return r.providerName
}

// statusTier names the effective session tier (the defaultTurnModel ladder's
// tier; "" → unset label).
func statusTier(r *Runner) string {
	if r.schedCfg == nil {
		return statusUnsetLabel
	}

	tier := r.schedCfg.SessionTier
	if tier == "" {
		tier = tierHeavy
	}

	return tier
}

// statusTurnUsage derives the turn count and the context-usage estimate from
// the transcript (transcript-as-truth): turns = user_message line count;
// usage = the most recent line carrying a token snapshot (input+cache);
// pct = usage against the resolved context window (-1 when the limit is
// unresolvable — the raw number still reports).
//
//nolint:nonamedreturns // the triple reads best named at the signature
func (r *Runner) statusTurnUsage(sess *session.Session) (turns int, usage int64, ctxPct int) {
	lines, err := sess.Manager.ReadAll()
	if err != nil {
		return 0, 0, -1
	}

	for i := range lines {
		if lines[i].Type == session.TypeUserMessage {
			turns++
		}
	}

	for i := len(lines) - 1; i >= 0; i-- { //nolint:modernize // hot scan; index form avoids 608-byte copies
		if lines[i].InputTokens > 0 || lines[i].CacheTokens > 0 {
			usage = lines[i].InputTokens + lines[i].CacheTokens

			break
		}
	}

	limit := r.compactionContextLimit()
	if limit <= 0 {
		return turns, usage, -1
	}

	return turns, usage, int(usage * 100 / limit)
}

// degradedCapabilityFlags renders the session's degraded-capability flags
// from the advisory family's per-session dedupe state (the classes already
// seen ARE the degrades the operator was warned about).
func (r *Runner) degradedCapabilityFlags(sessionID string) string {
	r.advisoryMu.Lock()
	defer r.advisoryMu.Unlock()

	return strings.Join(slices.Sorted(maps.Keys(r.advisorySeen[sessionID])), ", ")
}

// commandChainRef loads the live chain (the single read point — RESEARCH
// Pattern 1's one-access rule). A nil pointer (never built) degrades to a
// builtin-only chain so resolution semantics hold from construction.
func (r *Runner) commandChainRef() *commandChain {
	if c := r.chainPtr.Load(); c != nil {
		return c
	}

	return buildChain(ecosys.Registry{}, nil)
}

// installRegistry installs a freshly discovered registry + MCP set
// wholesale (the 20-05 swap discipline — reg maps are REPLACED, never
// mutated in place), then rebuilds the chain over it and stamps the
// invoke-time probe's root signature.
func (r *Runner) installRegistry(reg ecosys.Registry, servers []ecosys.ServerConfig) {
	r.regMu.Lock()
	r.reg = reg
	r.mcpServers = servers
	r.regMu.Unlock()

	r.rebuildCommandChain()
	r.noteChainSignature()
}

// rebuildCommandChain derives a fresh chain from r.reg and swaps it in
// atomically (the 20-05 rescan re-runs this after Discover). Fires the
// commands-notify seam AFTER the swap so listeners re-advertise from the
// chain that is already live.
func (r *Runner) rebuildCommandChain() {
	c := buildChain(r.registrySnapshot(), r.stderrOrDefault())
	r.chainPtr.Store(c)

	r.commandsNotifyMu.RLock()
	cb := r.commandsNotify
	r.commandsNotifyMu.RUnlock()

	if cb != nil {
		cb()
	}
}

// SetCommandsNotify installs the chain-change callback (20-01's re-fire
// seam): the acpserve composition binds it to the server's
// NotifyAllAvailableCommands so every swap re-fires the advertisement for
// live sessions (ACP-04 "on discovery change"). Set once at composition,
// before Serve.
func (r *Runner) SetCommandsNotify(cb func()) {
	r.commandsNotifyMu.Lock()
	defer r.commandsNotifyMu.Unlock()

	r.commandsNotify = cb
}

// CommandAdvertisement returns the current winner set as v1 wire frames —
// the SAME chain the resolver resolves through (D-04 one truth; the
// commandSourceAdapter consumes this).
func (r *Runner) CommandAdvertisement() []acp.AvailableCommandFrame {
	return r.commandChainRef().advertisement()
}

// chainResolveCount reports how many chain resolutions were consulted (test
// observability for the single-parse discipline: one resolution per Run).
func (r *Runner) chainResolveCount() uint64 { return r.chainResolves.Load() }

// --- 20-02: the class-B family (CMDS-02) ---

// builtinHelp renders its inventory FROM the live chain (D-08:
// self-describing — never a hand-maintained list; a discovered skill or
// command file changes the output at the next chain build).
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinHelp(_ context.Context, r *Runner, _ *session.Session, _, _ string) (string, string) {
	chain := r.commandChainRef()

	names := slices.Sorted(maps.Keys(chain.entries))

	var sb strings.Builder

	sb.WriteString("Commands (chain winners — what you see is what runs):\n")

	for _, name := range names {
		e := chain.entries[name]

		line := fmt.Sprintf("  /%s [%s]", e.name, e.kind)
		if e.desc != "" {
			line += " — " + e.desc
		}

		if e.hint != "" {
			line += " " + e.hint
		}

		sb.WriteString(line + "\n")
	}

	return sb.String(), ""
}

// builtinMemory lists the loaded memory sources read-only (D-09): the
// agent-md discovery tree + the learning store's entries. No edit affordance
// (ACP has no editor-open mechanism); zero file writes.
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinMemory(_ context.Context, r *Runner, _ *session.Session, _, _ string) (string, string) {
	var sb strings.Builder

	sb.WriteString("memory sources (read-only):\n")

	files := ecosys.DiscoverMemoryFiles(r.workDirOrDefault())
	if len(files) == 0 {
		sb.WriteString("  (no memory files discovered)\n")
	}

	for _, f := range files {
		note := ""
		if f.SkipNote != "" {
			note = " (" + f.SkipNote + ")"
		}

		fmt.Fprintf(&sb, "  %s — %d bytes%s\n", f.Path, f.OrigBytes, note)
	}

	sb.WriteString("learning store:\n")

	if r.learned != nil {
		entries := r.learned.List()

		suffix := "ies"
		if len(entries) == 1 {
			suffix = "y"
		}

		fmt.Fprintf(&sb, "  %d learned entr%s\n", len(entries), suffix)
	} else {
		sb.WriteString("  (learning store not loaded)\n")
	}

	return sb.String(), ""
}

// builtinPermissions reports the current permission-gate mode (the
// safety-model amendment: ungated is the default; the Phase 17 gate machinery
// rides the runner's perm store either way).
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinPermissions(_ context.Context, r *Runner, _ *session.Session, _, _ string) (string, string) {
	mode := r.PermMode()
	if mode == "" {
		mode = "ungated"
	}

	return fmt.Sprintf("permissions mode: %s\n"+
		"(tool execution is ungated by default; gated mode asks before mutating tools — "+
		"see the editor's permissions config option)\n", mode), ""
}

// builtinMCP lists the discovered MCP server configs (D-11/CONTEXT
// discretion): name + launch command. Connection state is not probed — a
// control-plane command never launches servers (Pitfall 8).
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinMcp(_ context.Context, r *Runner, _ *session.Session, _, _ string) (string, string) {
	var sb strings.Builder

	sb.WriteString("mcp servers (configured):\n")

	if len(r.mcpServers) == 0 {
		sb.WriteString("  (none configured)\n")

		return sb.String(), ""
	}

	for _, cfg := range r.mcpServers {
		fmt.Fprintf(&sb, "  %s — %s %s (connection state unknown at command time)\n",
			cfg.Name, cfg.Command, strings.Join(cfg.Args, " "))
	}

	return sb.String(), ""
}

// builtinDoctor runs the fixed check list (CONTEXT discretion): workdir,
// scheduling config, credential env var PRESENCE (names — never values,
// T-20-05), transcript directory writability, registry load status.
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinDoctor(_ context.Context, r *Runner, _ *session.Session, _, _ string) (string, string) {
	var sb strings.Builder

	sb.WriteString("doctor:\n")

	// 1. workdir.
	if wd := r.workDirOrDefault(); wd != "" {
		_, err := os.Stat(wd)
		if err == nil {
			sb.WriteString("  workdir: ok (" + wd + ")\n")
		} else {
			sb.WriteString("  workdir: UNRESOLVABLE (" + wd + ")\n")
		}
	}

	// 2. scheduling config.
	if r.schedCfg != nil {
		sb.WriteString("  scheduling config: loaded\n")
	} else {
		sb.WriteString("  scheduling config: NOT LOADED (tier resolution off)\n")
	}

	// 3. credential presence for the session provider — env var NAMES only.
	sb.WriteString("  credentials: " + doctorCredentialState(r) + "\n")

	// 4. transcript directory writability.
	sb.WriteString("  transcript dir: " + doctorTranscriptState(r.workDirOrDefault()) + "\n")

	// 5. registry load status (D-11 skip count comes from the loader's own
	// warnings; here we report the loaded surface sizes).
	fmt.Fprintf(&sb, "  command registry: %d command(s), %d skill(s), %d agent(s)\n",
		len(r.reg.Commands), len(r.reg.Skills), len(r.reg.Agents))

	return sb.String(), ""
}

// builtinConfig renders the current resolved scheduling view: session tier,
// tier→model bindings, compaction keys (16-05/19-05 surfaces).
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinConfig(_ context.Context, r *Runner, _ *session.Session, _, _ string) (string, string) {
	var sb strings.Builder

	if r.schedCfg == nil {
		return "config: no scheduling config loaded (defaults in effect)\n", ""
	}

	sb.WriteString("config:\n")

	tier := r.schedCfg.SessionTier
	if tier == "" {
		tier = tierHeavy
	}

	sb.WriteString("  session tier: " + tier + "\n")

	for _, name := range slices.Sorted(maps.Keys(r.schedCfg.Tiers)) {
		b := r.schedCfg.Tiers[name]

		line := fmt.Sprintf("  tier %s -> %s", name, b.Model)
		if len(b.Fallback) > 0 {
			line += " (fallback: " + strings.Join(b.Fallback, ", ") + ")"
		}

		sb.WriteString(line + "\n")
	}

	fmt.Fprintf(&sb, "  compaction: enabled=%v threshold=%d%%\n",
		r.schedCfg.Compaction.Enabled, r.schedCfg.Compaction.ThresholdPct)

	return sb.String(), ""
}

// doctorCredentialState reports the session provider's credential env var
// NAME and whether it is set — never the value (T-20-05).
func doctorCredentialState(r *Runner) string {
	if r.schedCfg == nil || r.providerName == "" {
		return "no provider declared"
	}

	prov, ok := r.schedCfg.Providers[r.providerName]
	if !ok {
		return "provider " + r.providerName + " not declared in config"
	}

	envName := prov.APIKeyEnv
	if envName == "" {
		envName = strings.ToUpper(r.providerName) + "_API_KEY"
	}

	if os.Getenv(envName) != "" {
		return envName + " is set"
	}

	return envName + " is NOT set"
}

// doctorTranscriptState probes the transcript directory's writability with a
// create-write-remove round trip (fixed-form outcomes only). dirPerm750 and
// filePerm600 mirror the session append-path convention.
const (
	dirPerm750  = 0o750
	filePerm600 = 0o600
)

func doctorTranscriptState(workDir string) string {
	tDir := filepath.Join(workDir, ".ass-guard")

	werr := os.MkdirAll(tDir, dirPerm750)
	if werr != nil {
		return "NOT creatable"
	}

	probe := filepath.Join(tDir, ".doctor-probe")

	perr := os.WriteFile(probe, []byte("x"), filePerm600)
	if perr != nil {
		return "NOT writable"
	}

	_ = os.Remove(probe)

	return "writable"
}

// localClearCause is the /clear boundary's cause (D-06): the session-side
// BoundaryCauseContextReset — the Projector's lean window starts empty on
// the next turn; transcript and session id survive.
const localClearCause = session.BoundaryCauseContextReset

// builtinModel applies a session-scope model switch (16-D-12): a DECLARED
// slug stamps this session's turn model through the 16-05 live-apply seam's
// per-session leg (SetTurnModel) — never a config-layer write (the
// second-write-path Anti-Pattern). No args prints the current model + tier;
// an unknown slug degrades loudly with the model UNCHANGED.
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinModel(_ context.Context, r *Runner, sess *session.Session, _, args string) (string, string) {
	slug := strings.TrimSpace(args)
	if slug == "" {
		model := sess.Profile.Model
		if model == "" {
			model = statusModel(r, sess)
		}

		return fmt.Sprintf("session model: %s\ntier: %s\nusage: /model <slug> (session-scope; no config is written)\n",
			model, statusTier(r)), ""
	}

	if r.schedCfg != nil {
		if _, ok := r.schedCfg.Models[slug]; !ok {
			return fmt.Sprintf(
				"unknown model %q — not declared in the scheduling config; session model UNCHANGED (%s)\n",
				slug, statusModel(r, sess)), "failed: unknown model"
		}
	}

	sess.SetTurnModel(slug)

	return fmt.Sprintf(
		"session model -> %s (applies to the next request; session-scope, nothing persisted)\n", slug), ""
}

// builtinClear writes the full context-reset boundary (D-06): same session,
// same transcript, the Projector's lean window starts empty for the next
// turn. Resume replays the full history including the boundary.
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinClear(_ context.Context, _ *Runner, sess *session.Session, turnID, _ string) (string, string) {
	if sess.Manager == nil {
		return "clear failed: no transcript manager\n", "failed: no manager"
	}

	err := sess.Manager.AppendBoundary(localClearCause, "builtin:clear", turnID)
	if err != nil {
		return fmt.Sprintf("clear failed: boundary write error: %v\n", err), "failed: boundary write"
	}

	return "context cleared (same session; history retained on disk; new window starts empty)\n", ""
}

// builtinResume surfaces the resumable-session pointer (18-D-10 delegation):
// the CLIENT owns list UX (the agent-side TUI anti-pattern), so /resume lists
// resumable sessions through the 18-03 listing engine and points at the
// operator's resume surfaces. Degrades loudly when the listing seam is
// unregistered.
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinResume(_ context.Context, r *Runner, _ *session.Session, _, _ string) (string, string) {
	if r.resumeListHook == nil {
		out, outcome := unavailableMachinery("resume picker", "Phase 18 session listing")

		return out, outcome
	}

	out, err := r.resumeListHook()
	if err != nil {
		return fmt.Sprintf("resume listing failed: %v (retry or use the editor's session picker)\n",
			err), "failed: listing"
	}

	return out, ""
}

// builtinCompact delegates to the Phase 19 immediate-trigger machinery
// (19-D-11: CompactNow — same machinery, manual intent bypasses the
// threshold/enabled gates). Typed args are recorded verbatim in the
// local_command line (16-D-22); CompactNow's signature admits no focus
// instructions, so present args get a fixed-form note. Degrades loudly when
// the seam is unregistered.
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinCompact(ctx context.Context, r *Runner, sess *session.Session, _, args string) (string, string) {
	if r.compactNowHook == nil {
		out, outcome := unavailableMachinery("compaction", "Phase 19 CompactNow")

		return out, outcome
	}

	out, err := r.compactNowHook(ctx, sess, args)
	if err != nil {
		return fmt.Sprintf("compaction failed: %v\n", err), "failed: compaction"
	}

	if strings.TrimSpace(args) != "" {
		out += "note: focus instructions are recorded but not yet passed to the summarizer\n"
	}

	return out, ""
}

// unavailableMachinery renders the delegation seams' loud degrade: the named
// machinery is absent from THIS build and the invocation was recorded as an
// unavailable outcome — never silent, never a wedged turn.
//
//nolint:gocritic // the pair mirrors the handler shape
func unavailableMachinery(what, phase string) (string, string) {
	return fmt.Sprintf("/%s unavailable: %s not registered in this build (%s machinery absent) — "+
			"the invocation is recorded; nothing was executed\n", what, what, phase),
		fmt.Sprintf("unavailable: %s not registered in this build", what)
}

// resumeListLimit bounds /resume's one-page listing (pointer text, not a
// session browser — the client owns list UX).
const resumeListLimit = 20

// realResumeListing is the production resume-listing entry (18-D-03/18-04):
// one page through the session listing engine rendered as pointer text.
func realResumeListing(workDir string) func() (string, error) {
	return func() (string, error) {
		headers, _, err := session.ListSessions(workDir, "", resumeListLimit)
		if err != nil {
			return "", err //nolint:wrapcheck // delegation seam — the handler wraps
		}

		var sb strings.Builder

		sb.WriteString("resumable sessions (most recent first):\n")

		if len(headers) == 0 {
			sb.WriteString("  (none found)\n")
		}

		for _, h := range headers {
			title := h.Title
			if !h.TitlePresent || title == "" {
				title = "(untitled)"
			}

			fmt.Fprintf(&sb, "  %s — %s\n", h.SessionID, title)
		}

		sb.WriteString("resume via the editor's session picker or `ass-guard --resume <session-id>`\n")

		return sb.String(), nil
	}
}

// realCompactNow is the production compaction entry (19-D-11): the SAME
// machinery as the threshold path, invoked immediately. The caller (the
// class-B intercept) already holds the session's turn mutex — exactly
// CompactNow's serialization contract.
func realCompactNow(ctx context.Context, sess *session.Session, _ string) (string, error) {
	err := sess.CompactNow(ctx)
	if err != nil {
		return "", err //nolint:wrapcheck // delegation seam — the handler wraps
	}

	return "compaction complete — the next request projects from the fresh window\n", nil
}

// costFetchBudgetDefault is the /cost live leg's FAST-CONTROL budget
// (16-D-17's ~10s class): a slow or hung usage endpoint falls back to the
// transcript derivation instead of wedging the turn mutex (Pitfall 8).
const costFetchBudgetDefault = 10 * time.Second

// builtinCost implements D-07 (operator directive): the provider's declared
// usage endpoint FIRST, on-demand under the FAST-CONTROL budget; on
// absence/timeout/error the transcript usage × modelrouting Pricing fallback
// — ALWAYS with a source note naming which producer made the number
// (P-20-02). Output carries amounts/currencies/model names only — never
// credential material (T-20-05).
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinCost(ctx context.Context, r *Runner, sess *session.Session, _, _ string) (string, string) {
	model := statusModel(r, sess)

	inTok, outTok := costTranscriptUsage(sess)

	// Live leg: only when the provider declares a usage endpoint.
	if provider, ok := r.costProviderConfig(model); ok && provider.UsageEndpoint != "" {
		body, err := r.fetchUsageEndpoint(ctx, provider.UsageEndpoint, provider)
		if err == nil {
			return fmt.Sprintf("cost (source: provider usage endpoint %s — live):\n%s",
				provider.UsageEndpoint, costRenderLive(body)), ""
		}

		_, _ = fmt.Fprintf(r.stderrOrDefault(), "ass-guard: /cost live endpoint failed (falling back): %v\n", err)
	}

	// Fallback: transcript usage × Pricing (the CostCeilingTracker.Account
	// formula, cost.go:61–89 — reused, never re-implemented).
	var perMIn, perMOut float64

	if mc, ok := r.schedCfg.Models[model]; ok {
		perMIn = mc.Pricing.InputPerMToken
		perMOut = mc.Pricing.OutputPerMToken
	}

	amount := (float64(inTok)*perMIn + float64(outTok)*perMOut) / costTokenDivisor

	return fmt.Sprintf(
		"cost (source: transcript usage × modelrouting cost table — derived, not provider-live):\n"+
			"  model: %s\n  usage: %d in + %d out tokens\n  estimated: $%.2f\n",
		model, inTok, outTok, amount), ""
}

// costProviderConfig resolves the effective model's ProviderConfig when the
// scheduling config declares both.
func (r *Runner) costProviderConfig(model string) (*modelrouting.ProviderConfig, bool) {
	if r.schedCfg == nil {
		return nil, false
	}

	mc, ok := r.schedCfg.Models[model]
	if !ok {
		return nil, false
	}

	prov, ok := r.schedCfg.Providers[mc.Provider]

	return &prov, ok
}

// fetchUsageEndpoint performs the ONE bounded live fetch (FAST-CONTROL): GET
// the declared URL with the provider's credential in the Authorization
// header (the credential NEVER enters output — T-20-05). transport/budget
// seams are test-injectable (nil = the default client + 10s budget).
func (r *Runner) fetchUsageEndpoint(
	ctx context.Context, url string, provider *modelrouting.ProviderConfig,
) ([]byte, error) {
	budget := r.costFetchBudget
	if budget <= 0 {
		budget = costFetchBudgetDefault
	}

	fctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	req, err := http.NewRequestWithContext(fctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	if key := r.costCredential(provider); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{}
	if r.costTransport != nil {
		client.Transport = r.costTransport
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", errCostEndpoint, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, costBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return body, nil
}

// costBodyLimit bounds the live endpoint's response read (the read-cap
// discipline; a pathological endpoint cannot stream the turn wedged).
const costBodyLimit = 64 * 1024

// errCostEndpoint is the live leg's static wrap target (non-OK status).
var errCostEndpoint = errors.New("usage endpoint")

// costCredential resolves the provider's env-var credential for the live
// fetch (value used ONLY in the Authorization header; never rendered).
func (r *Runner) costCredential(provider *modelrouting.ProviderConfig) string {
	envName := provider.APIKeyEnv
	if envName == "" {
		envName = strings.ToUpper(r.providerName) + "_API_KEY"
	}

	return os.Getenv(envName)
}

// costRenderLive renders the live endpoint's response: a provider-specific
// JSON payload summarized best-effort (the raw pretty-printed object) — the
// SOURCE NOTE in the surrounding output is the contract, not this shape.
func costRenderLive(body []byte) string {
	var pretty map[string]any

	jerr := json.Unmarshal(body, &pretty)
	if jerr == nil {
		raw, merr := json.MarshalIndent(pretty, "  ", "  ")
		if merr == nil {
			return "  " + string(raw) + "\n"
		}
	}

	return "  " + string(body) + "\n"
}

// costTokenDivisor is the Pricing per-1M-token normalization (cost.go's
// Account formula divisor, reused verbatim).
const costTokenDivisor = 1e6

// costTranscriptUsage aggregates the transcript's usage lines
// (transcript-as-truth, 18-D-01: the derivation survives resume — recomputed
// from lines, never in-memory state).
//
//nolint:nonamedreturns // the pair reads best named at the signature
func costTranscriptUsage(sess *session.Session) (inTok, outTok int64) {
	lines, err := sess.Manager.ReadAll()
	if err != nil {
		return 0, 0
	}

	for i := range lines {
		if lines[i].Type == session.TypeUsage {
			inTok += lines[i].InputTokens
			outTok += lines[i].OutputTokens
		}
	}

	return inTok, outTok
}

// --- 23-05 (SEEDG-03): /undo — the D-11 stack walk over the checkpoint store ---

// nameUndo is the /undo builtin's reserved name (the fourteenth RESERVED
// name, 23-05).
const nameUndo = "undo"

// undoRefPrefix mirrors the store's ref namespace (refs/checkpoints/ — the
// id grammar's prefix, pinned by the store's own battery). The runtime trims
// it off Entry.Ref to hand Store.Restore the bare checkpoint id.
const undoRefPrefix = "refs/checkpoints/"

// parseUndoDepth parses /undo's depth argument (CONTEXT discretion — the
// documented edge table): empty -> 1; zero and negative normalize to 1;
// non-numeric is a typed error the caller renders as the D-05 error text
// (never a model turn, still a recorded invocation).
func parseUndoDepth(args string) (int, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return 1, nil
	}

	n, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid depth %q: expected /undo [N] with N a positive integer", trimmed)
	}

	if n < 1 {
		return 1, nil
	}

	return n, nil
}

// undoWalkTarget selects the D-11 walk target from the store's entries:
// THIS session's entries ordered newest-first, N steps from the newest;
// beyond-depth clamps to the oldest available entry.
//
// The walk order is RECENCY (CommittedAt), not List()'s grammar order —
// that is load-bearing: every restore mints its pre-restore snapshot FIRST
// (D-09), that snapshot is the newest entry of the walk, and the NEXT /undo
// therefore targets it — the walk itself is reversible (undo-of-undo). On
// CommittedAt ties (same-second snapshots are the common case in tests and
// fast operator sequences) the tie-breaks reproduce mint order: pre-family
// above turn-family (a snapshot minted by an undo that restored a turn of
// the same second is the later mint), then higher sequence numbers above
// lower, then Ref — the GC count-axis ordering discipline applied to the
// walk.
func undoWalkTarget(entries []checkpoint.Entry, sessionID string, depth int) (checkpoint.Entry, bool) {
	mine := make([]checkpoint.Entry, 0, len(entries))

	for _, e := range entries {
		if e.SessionID == sessionID {
			mine = append(mine, e)
		}
	}

	if len(mine) == 0 {
		return checkpoint.Entry{}, false
	}

	slices.SortFunc(mine, func(a, b checkpoint.Entry) int {
		if !a.CommittedAt.Equal(b.CommittedAt) {
			return b.CommittedAt.Compare(a.CommittedAt) // newest first
		}

		if a.Kind != b.Kind {
			return strings.Compare(a.Kind, b.Kind) // "pre" above "turn" on ties
		}

		if a.TurnNum != b.TurnNum {
			return b.TurnNum - a.TurnNum
		}

		return strings.Compare(b.Ref, a.Ref)
	})

	idx := depth - 1
	if idx >= len(mine) {
		idx = len(mine) - 1 // beyond-depth clamps to the oldest
	}

	return mine[idx], true
}

// undoPlan is one fully-validated /undo invocation (the mutation steps still
// ahead: pre-restore snapshot + restore).
type undoPlan struct {
	targetID string
	depth    int
}

// undoSnapshotPreRestore is the SnapshotPreRestore seam (the gitRun
// same-package-injection precedent): Task 2's fail-closed test injects a
// failure here to pin that a pre-restore snapshot failure aborts the undo
// without cancelling anything. nil is never observed — restoreUndo and the
// active path both fall back to realSnapshotPreRestore.
var undoSnapshotPreRestore func(ctx context.Context, st *checkpoint.Store, sessionID string) (string, error) //nolint:gochecknoglobals // injectable test seam

// realSnapshotPreRestore is the seam's default: the store's own mint.
func realSnapshotPreRestore(ctx context.Context, st *checkpoint.Store, sessionID string) (string, error) {
	return st.SnapshotPreRestore(ctx, sessionID)
}

// prepareUndo runs every pre-mutation step of /undo — the head the idle path
// (Task 1) and the active-turn path (Task 2, D-12) share: nil-store loud
// degrade (Pitfall 9), depth parsing, the nested-repo refusal (23-03
// RestoreGuard — outright, never an auto path), and the D-11 walk target
// selection. A non-ready plan carries the D-05 output + outcome the caller
// renders verbatim.
func (r *Runner) prepareUndo(sess *session.Session, args string) (plan undoPlan, out, outcome string) {
	st := r.checkpointStore()
	if st == nil {
		return undoPlan{},
			"/undo unavailable: checkpoint store disabled for this workspace — " +
				"turns ran without undo snapshots (see stderr; the invocation is recorded)\n",
			"unavailable: checkpoint store disabled"
	}

	depth, perr := parseUndoDepth(args)
	if perr != nil {
		return undoPlan{}, fmt.Sprintf("/undo failed: %v\n", perr), "failed: invalid depth"
	}

	if gerr := st.RestoreGuard(r.workDirOrDefault()); gerr != nil {
		return undoPlan{}, fmt.Sprintf("undo refused: %v\n", gerr), "refused: nested repositories"
	}

	entries, lerr := st.List()
	if lerr != nil {
		return undoPlan{},
			fmt.Sprintf("/undo failed: checkpoint listing failed: %v\n", lerr),
			"failed: checkpoint list"
	}

	target, ok := undoWalkTarget(entries, sess.SessionID, depth)
	if !ok {
		return undoPlan{},
			"nothing to restore — this session has no checkpoints yet " +
				"(snapshots land at every turn start)\n", ""
	}

	return undoPlan{targetID: strings.TrimPrefix(target.Ref, undoRefPrefix), depth: depth}, "", ""
}

// restoreUndo performs the mutation half of the IDLE path (Task 1): the
// D-09 fail-closed sequence — the pre-restore snapshot ALWAYS precedes the
// restore, and its failure aborts the undo with nothing mutated. Store ops
// ride the serve ctx (the parked-chain precedent): a restore must not die
// with the requesting request halfway through its git plumbing.
func (r *Runner) restoreUndo(sess *session.Session, plan undoPlan) (string, string) {
	st := r.checkpointStore()
	ctx := r.serveCtxOrBackground()

	preID, serr := st.SnapshotPreRestore(ctx, sess.SessionID)
	if serr != nil {
		return fmt.Sprintf(
			"undo aborted: pre-restore snapshot failed: %v (nothing was cancelled, nothing was restored)\n",
			serr), "failed: pre-restore snapshot"
	}

	if rerr := st.Restore(ctx, plan.targetID); rerr != nil {
		return fmt.Sprintf(
			"undo failed: restore of %s failed: %v (pre-restore snapshot %s is available)\n",
			plan.targetID, rerr, preID), "failed: restore"
	}

	return fmt.Sprintf(
		"undo complete — restored checkpoint %s (pre-restore snapshot: %s); "+
			"run /undo again to walk back further\n",
		plan.targetID, preID), ""
}

// builtinUndo is the /undo handler — the idle-path half (23-05 Task 1,
// SEEDG-03). The tryLocalCommand intercept already holds the session turn
// mutex (the session is idle by construction), so the sequence is prepare ->
// snapshot -> restore, zero provider calls. The ACTIVE-turn half (D-12
// auto-cancel-then-restore) composes the same prepare/restore steps from the
// pre-mutex classifier slot (routeUndoActive, runtime.go) — it never runs
// here, because a mid-turn /undo is classified BEFORE the mutex.
//
//nolint:gocritic // (output, outcome) pair — the handler-table shape
func builtinUndo(_ context.Context, r *Runner, sess *session.Session, _, args string) (string, string) {
	plan, out, outcome := r.prepareUndo(sess, args)
	if plan.targetID == "" {
		return out, outcome
	}

	return r.restoreUndo(sess, plan)
}

// resolveSlashCommand returns the Command value the EXPANSION seam expands
// for a parsed invocation key (20-04 — the chain-aware generalization of the
// old r.reg.Commands[key] lookup): file commands verbatim, skills as
// synthesized Commands (the SKILL.md body re-read from disk — the rescan
// path serves fresh bodies by design), class-A builtins as their authored
// Command. ok=false for unknown names, class-B builtins, and agents (the
// caller's own surfaces own those) — plain text, never an error.
func (r *Runner) resolveSlashCommand(key string) (ecosys.Command, bool) {
	r.maybeFreshRescan() // D-10 backstop: never resolve against a stale chain

	e, found := r.commandChainRef().resolve(key)
	if !found {
		return ecosys.Command{}, false
	}

	switch e.kind {
	case chainKindFile:
		if e.file != nil {
			return *e.file, true
		}
	case chainKindSkill:
		if e.skill != nil {
			if body, ok := ecosys.ResolveSkill(r.registrySnapshot(), key); ok {
				return ecosys.Command{
					Name: e.skill.Name, Description: e.skill.Description,
					Body: body, Path: e.skill.Path,
				}, true
			}
		}
	case chainKindBuiltin:
		if e.hasClassA {
			return e.classA, true
		}
	}

	return ecosys.Command{}, false
}

// skillBodyEmpty reports whether a skill winner's on-disk body is empty or
// whitespace (the loud-reject case: an empty prompt must never reach the
// model).
func (r *Runner) skillBodyEmpty(key string) bool {
	e, found := r.commandChainRef().resolve(key)
	if !found || e.kind != chainKindSkill || e.skill == nil {
		return false
	}

	body, ok := ecosys.ResolveSkill(r.registrySnapshot(), key)

	return ok && strings.TrimSpace(body) == ""
}
