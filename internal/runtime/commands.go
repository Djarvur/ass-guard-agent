package runtime

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
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

// reservedNames is the D-01 published contract: the twelve class-B names
// (CMDS-02) plus init (class-A, CMDS-03). A discovered file, skill, or agent
// sharing one of these names NEVER fires via slash — dropped at chain build
// with exactly one structured warning naming the file and the reserved name.
// Growth of this list is costly by design (a later builtin silently shadows
// an existing discovered command from the user's view) — the shadow-check
// warning is the mitigation this set ships with.
var reservedNames = map[string]struct{}{ //nolint:gochecknoglobals // the D-01 published contract table
	"help": {}, "status": {}, "cost": {}, "mcp": {}, "memory": {},
	"permissions": {}, "doctor": {}, "config": {}, "model": {},
	"clear": {}, "resume": {}, "compact": {}, "init": {},
}

// builtinTable is the LIVE builtin set (a subset of reservedNames — the
// remaining names become live entries in their own plan-20-02/20-04 tasks;
// they are reserved against discovery from this commit onward regardless).
// 20-01 ships exactly two live builtins: /status (the tracer) and /init
// (class-A reservation).
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
	}
}

// initClassAReservation is the /init class-A entry (CMDS-03): a synthesized
// ecosys.Command carrying only identity — its prompt BODY is plan 20-04's
// deliverable. Until that plan injects the body into the registry view, a
// typed /init parses, resolves to this reservation at the intercept, falls
// through the class-A path, misses in r.reg.Commands, and stays plain text —
// exactly today's behavior (the reservation only blocks discovery).
func initClassAReservation() ecosys.Command {
	return ecosys.Command{
		Name:        "init",
		Description: "Analyze the codebase and scaffold a CLAUDE.md project guide",
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
	for _, sk := range reg.AllSkills() {
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

// rebuildCommandChain derives a fresh chain from r.reg and swaps it in
// atomically (the 20-05 rescan re-runs this after Discover). Fires the
// commands-notify seam AFTER the swap so listeners re-advertise from the
// chain that is already live.
func (r *Runner) rebuildCommandChain() {
	c := buildChain(r.reg, r.stderrOrDefault())
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
