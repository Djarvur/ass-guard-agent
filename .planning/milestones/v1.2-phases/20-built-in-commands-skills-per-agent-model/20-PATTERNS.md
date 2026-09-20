# Phase 20: Built-in Commands + Skills + Per-Agent Model - Pattern Map

**Mapped:** 2026-08-28
**Files analyzed:** 12 (4 new source, 4 modified source, 3 new test, 1 wiring mod)
**Analogs found:** 10 / 12 with exact-or-role matches; 2 genuinely new surfaces (fsnotify watcher, atomic chain swap) flagged under "No Analog Found"

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/runtime/commands.go` (NEW) | controller (command resolution) | request-response | `internal/runtime/runtime.go` expandUserBlocks/invocationFor + `internal/ecosys` types/expand | exact |
| `internal/runtime/rescan.go` (NEW) | service (watcher lifecycle) | event-driven | `internal/runtime/runtime.go` LoadCommandRegistry + `internal/acpserve/acp_serve.go` serve-lifetime goroutine wiring | role-match |
| `internal/runtime/runtime.go` (MOD — class-B intercept in `Run`) | controller | request-response | `internal/runtime/runtime.go` Run (479–566) + routeAskReply intercept | exact |
| `internal/runtime/runtime.go` (MOD — D-13/D-15 dispatch routing) | controller | request-response | `resolveSubagentModel` (1311–1331) + `internal/session/subagent.go` subagentProfile + `internal/modelrouting` resolver/factory | exact |
| `internal/session/transcript.go` (MOD — ResolvedModel field) | model | file-I/O | Line struct Phase-16 additive-kinds family (144–156) | exact |
| `internal/session/manager.go` (MOD — AppendSubagentDispatch param) | service (transcript API) | file-I/O | AppendLocalCommand (362–367) / AppendSubagentDispatch (274–280) | exact |
| `internal/acp/types.go` (MOD — AvailableCommands frames) | config (wire types) | request-response | ConfigOptionFrame + KindConfigOptionUpdate (203–234), ThoughtChunkFrame (176–179) | exact |
| `internal/acp/server.go` (MOD — NotifyAvailableCommands) | service (wire emitter) | request-response | NotifyConfigOptions (299–317) | exact |
| `internal/acpserve/acp_serve.go` (MOD — watcher/advertisement wiring) | provider (composition) | event-driven | Run's SetNotify/SetEmitter closure wiring (219–239) | exact |
| `internal/runtime/commands_test.go` (NEW) | test | — | `internal/runtime/runner_battery_test.go` + `internal/session/subagent_test.go` fakes | role-match |
| `internal/runtime/rescan_test.go` (NEW) | test | — | `internal/ecosys/loader_test.go` temp-dir fixtures | role-match |
| `internal/acp/available_commands_test.go` (NEW) | test | — | `internal/acp/server_test.go` / `emitter_test.go` frame-shape tests | role-match |

## Pattern Assignments

### `internal/runtime/commands.go` (NEW — chain resolver + builtin table)

**Analog:** `internal/runtime/runtime.go` expandUserBlocks + invocationFor — the chain GENERALIZES the single `r.reg.Commands[key]` lookup at their heart (CMDS-01). Keep their single-parse discipline.

**Single-parse resolution shape** (runtime.go:458–476 — the exact function whose `reg.Commands[key]` lookup the chain replaces; enginebridge consumes it via `cfg.Invoke`, runtime.go:808–817):

```go
func (r *Runner) invocationFor( //nolint:funcorder,nonamedreturns // sibling of expandUserBlocks
	blocks []session.ContentBlock,
) (key, args string, ok bool) {
	idx := firstTextBlockIndex(blocks)
	if idx < 0 {
		return "", "", false
	}

	key, args, parsed := ecosys.ParseInvocation(blocks[idx].Text)
	if !parsed {
		return "", "", false
	}

	if _, found := r.reg.Commands[key]; !found { // ← THIS lookup becomes chain.resolve(key)
		return "", "", false
	}

	return key, args, true
}
```

**Registry-miss = plain text, never an error** (runtime.go:395–438, the expansion seam; expandUserBlocks itself stays the class-A/skill path — /init enters as a synthesized `ecosys.Command`):

```go
cmd, found := r.reg.Commands[key]
if !found {
	return blocks
}

out := append([]session.ContentBlock(nil), blocks...)
out[idx] = session.ContentBlock{Type: blockText, Text: cmd.Expand(args)}
```

**Invocation grammar the builtin names must fit** (ecosys/expand.go:14, 24–33 — Pitfall 7: all twelve class-B names + `init` are lowercase ASCII and fit):

```go
var invocationRe = regexp.MustCompile(`^/([a-z0-9][a-z0-9_:-]{0,63})(\s+(.*))?$`)
```

**Skill slash-expansion = synthesize a Command, reuse Expand** (ecosys/skills.go:111–125 — `ResolveSkill` re-reads the SKILL.md body by registry key; SKLS-01 rides `Command.Expand` verbatim):

```go
func ResolveSkill(reg Registry, name string) (string, bool) {
	sk, ok := reg.Skills[name]
	if !ok {
		return "", false
	}

	data, err := os.ReadFile(sk.Path)
	if err != nil {
		return "", false
	}

	_, body := splitFrontmatter(string(data))

	return body, true
}
```

**Expand semantics to reuse, never re-implement** (ecosys/expand.go:51–70 — $ARGUMENTS, $1..$9, $0→empty, append-under-heading rule; locked zcode parity):

```go
func (c Command) Expand(args string) string {
	fields := strings.Fields(args)

	body := strings.ReplaceAll(c.Body, "$ARGUMENTS", args)

	for n := maxPositional; n >= 1; n-- {
		body = strings.ReplaceAll(body, "$"+strconv.Itoa(n), fieldAt(fields, n))
	}

	body = strings.ReplaceAll(body, "$0", "")

	if args != "" && !hasPlaceholder(c.Body) {
		body += "\n\nUser arguments:\n" + args
	}

	return body
}
```

**Registry + Agent types the chain view reads** (ecosys/types.go:60–83 — `Agent.Model` already carries the frontmatter slug; deterministic accessors `AllSkills/AllCommands/AllAgents` at loader.go:1178–1222 give the advertisement its sorted order):

```go
type Agent struct {
	Name        string
	Description string
	Tools       []string
	Model       string
	Prompt      string
	Path        string
}

type Registry struct {
	Skills   map[string]Skill
	Commands map[string]Command
	Plugins  map[string]Plugin
	Agents   map[string]Agent
	Hooks    []HookConfig
}
```

---

### `internal/runtime/runtime.go` — class-B intercept in `Run` (MOD)

**Analog:** `internal/runtime/runtime.go` Run (479–566). The class-B intercept mirrors `routeAskReply`'s placement exactly: AFTER turnMu + forwarder subscriptions + routeAskReply, BEFORE the engine branch (539). Placing it after the engine branch is the research's Anti-Pattern #3 (divergent engine-on/off behavior).

**Intercept placement + ordering** (runtime.go:483–541 — copy the ordering; the new block slots in at the marked line):

```go
sess := r.sessionFor(ctx, sessionID)

turnMu := r.sessionTurnMu(sessionID)
turnMu.Lock()
defer turnMu.Unlock()

r.markClientTurn(sessionID, true)
defer r.markClientTurn(sessionID, false)

ch := r.bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
// ...toolCh/toolUpdCh subscriptions + startChunkForwarder...

blocks := toContentBlocks(prompt)

if stop, handled := r.routeAskReply(ctx, sess, blocks); handled { /* existing early return */ }

// >>> NEW class-B intercept goes HERE (ParseInvocation once → chain.resolve →
// builtin class-B handler → AppendLocalCommand → echo/output chunks → end_turn;
// class-A/skill/file fall through unchanged) <<<

if !r.engineEnabled || r.eng == nil || r.patternTable == nil {
	blocks = r.expandUserBlocks(sess, blocks)
}
```

**Chunk emission through the in-hand emitter** (runtime.go:550–563 — the advisory-note machinery D-16's live note reuses; note it fires AFTER the forwarder drains, directly through `emit`):

```go
if adv := <-advDone; adv != nil && r.advisoryNoteDue(sessionID, adv.class) {
	_ = emit.AgentMessageChunk(adv.turnID, adv.text)
}
```

**Per-session dedupe state on the Runner** (runtime.go:202–208 + 1584–1603 — the pattern for D-15's "exactly ONE loud warning" if warnings must be once-per-session client-visible):

```go
func (r *Runner) advisoryNoteDue(sessionID, class string) bool {
	r.advisoryMu.Lock()
	defer r.advisoryMu.Unlock()
	if r.advisorySeen == nil { r.advisorySeen = make(map[string]map[string]bool) }
	if r.advisorySeen[sessionID] == nil { r.advisorySeen[sessionID] = make(map[string]bool) }
	if r.advisorySeen[sessionID][class] { return false }
	r.advisorySeen[sessionID][class] = true
	return true
}
```

---

### `internal/runtime/runtime.go` — D-13/D-15 per-agent model routing (MOD)

**Analog:** `resolveSubagentModel` (runtime.go:1311–1331) is the 14-05 machinery D-13 REVERSES; `subagentProfile` (subagent.go:273–288) is the stamp site that stays; modelrouting supplies target + second provider.

**The resolution being modified** (runtime.go:1311–1331 — D-13 changes the PRECEDENCE here: frontmatter > dispatch > session > tier; tier consulted only when no session model exists):

```go
func resolveSubagentModel(cfg *modelrouting.Config, sessionProvider string, now time.Time, stderr io.Writer) string {
	if cfg == nil {
		return ""
	}

	primary, _, err := modelrouting.NewResolver(cfg).Resolve(tierLight, "", now, modelrouting.CapabilityReq{})
	if err != nil {
		return "" // no tiers.light binding — the documented default
	}

	if primary.Provider != sessionProvider {
		_, _ = fmt.Fprintf(stderr,
			"ass-guard: tiers.light is bound to provider %q but the session provider is %q — "+
				"subagent model override SKIPPED (parent model kept; cross-provider light-tier "+
				"routing is routed post-adoption)\n", primary.Provider, sessionProvider)

		return ""
	}

	return primary.Model
}
```

**The stamp site (unchanged in shape)** (subagent.go:273–288 — value-copy semantics: `prof` is a struct copy; D-13's resolved slug lands in the same `prof.Model` assignment):

```go
func subagentProfile(s *Session, agentDef *ecosys.Agent) profile.Profile {
	prof := s.Profile

	if s.SubagentModel != "" {
		prof.Model = s.SubagentModel
	}

	if agentDef == nil || agentDef.Prompt == "" {
		return prof
	}

	prof.System = append(append([]profile.TextBlock(nil), s.Profile.System...),
		profile.TextBlock{Type: blockText, Text: agentDef.Prompt})

	return prof
}
```

**Dispatch call site gaining the resolved model** (subagent.go:43–52 — D-16: extend `AppendSubagentDispatch` with the resolved slug; the turn ID + restricted-set flow stays):

```go
restricted := subagentRestrictedDefault
if agentDef != nil && len(agentDef.Tools) > 0 {
	restricted = agentDef.Tools
}

subagentTurnID := s.nextTurnID()
_ = s.Manager.AppendSubagentDispatch(parentTurnID, subagentTurnID, toolCallID, restricted)
```

**Slug → Target lookup (unknown-slug tripwire)** (modelrouting/resolver.go:101–122 — `buildTarget` errors on a slug absent from `cfg.Models`; that error IS the D-15 degrade trigger, never a turn failure):

```go
func (r *Resolver) buildTarget(modelSlug string) (Target, error) {
	m, ok := r.cfg.Models[modelSlug]
	if !ok {
		return Target{}, fmt.Errorf("model %q is not declared in models", modelSlug)
	}

	p, ok := r.cfg.Providers[m.Provider]
	if !ok {
		return Target{}, fmt.Errorf("provider %q (for model %q) is not declared in providers", m.Provider, modelSlug)
	}

	return Target{
		Provider: m.Provider, Model: modelSlug, BaseURL: p.BaseURL, Shape: p.Shape,
		Capabilities: m.Capabilities, Pricing: m.Pricing,
	}, nil
}
```

**Second-provider construction — the SINGLE seam** (modelrouting/factory.go:142–171 — D-15's cross-provider instance comes from `BuildWithCapturer`; note the uncredentialed arm returns the lazy-failing wrapper, which is exactly the "missing credentials" tripwire to detect via `Endpoint` or a test Stream):

```go
func (f *ProviderFactory) BuildWithCapturer(
	providerName string, sh *shaper.Shaper, capturer provider.RequestCapturer,
) (provider.Provider, error) {
	prov, ok := f.cfg.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q: not declared in providers", providerName)
	}

	cred := ResolveCredential(prov, providerName, f.flagKey)
	if cred.Key == "" {
		return noCredentialProvider{name: providerName, envName: credentialEnvName(prov, providerName)}, nil
	}
	// ... shape switch → NewAnthropicProvider / NewOpenAIProvider with capturer ...
}
```

**Session-side wiring points** (runtime.go:1185–1190 — `SubagentModel` stamps at sessionFor construction and `SubagentTypes` ALIASES `r.reg.Agents`; the alias is the stale-reference hazard the chain-view fix must route around):

```go
SubagentModel: resolveSubagentModel(r.schedCfg, r.providerName, time.Now(), r.stderrOrDefault()),

SubagentTypes: r.reg.Agents, // ← alias; after a rescan swap this is the OLD map
```

---

### `internal/runtime/rescan.go` (NEW — fsnotify watch + debounce + swap)

**Analog (discovery re-run target):** `LoadCommandRegistry` (runtime.go:357–383) — the one-call `ecosys.Discover` + graceful-degradation logging shape; rescan re-runs exactly this discovery and builds the chain from the fresh Registry.

```go
func (r *Runner) LoadCommandRegistry() {
	reg, servers, err := ecosys.Discover(r.workDirOrDefault())
	if err != nil {
		log.Printf("ass-guard: command registry load failed (continuing without slash expansion): %v", err)
		r.reg = ecosys.Registry{}
		r.cmdMutability = nil
		r.mcpServers = nil
		return
	}

	r.reg = reg
	r.mcpServers = servers
	// ...cmdMutability load...
}
```

**Analog (discovery internals):** `ecosys.Discover` (loader.go:1275–1296) and `discoverAgentsDir` (loader.go:194–224, D-11's malformed-file skip + `logPluginSkipf` one-structured-warning seam at loader.go:801–803).

**Analog (serve-lifetime goroutine + ctx wiring):** `acp_serve.go` Run (164–249) — watcher lifecycle derives from the serve ctx exactly like the scheduler and the ctx-done reap:

```go
runner.StartScheduler(ctx)

go func() {
	<-ctx.Done()
	runner.CloseAllSessions() //nolint:contextcheck // the reap is ctx-driven by design
}()
```

**No-analog core (use RESEARCH skeleton):** the fsnotify watch loop, debounce timer (100–500ms, Chmod-skip, dynamic `Add` on new leaf dirs), atomic chain-pointer swap, and D-12 watcher-failure degrade have no codebase precedent — copy RESEARCH.md §Code Examples "fsnotify watch + debounce skeleton" and §Pattern 1's concurrency contract verbatim. Add `github.com/fsnotify/fsnotify@v1.10.1` (verified absent from go.mod).

**Concurrency contract for the swap** (modeled on existing guarded-state fields, runtime.go:134–141 `modelMu/effectiveModel` and runtime.go:181–190 `sync.Map` turnMus): the chain is immutable once built; readers load through one access point; NEVER mutate `r.reg` maps in place (Pitfall 1 — `expandUserBlocks` reads them on every turn goroutine).

---

### `internal/session/transcript.go` + `internal/session/manager.go` (MOD — D-16 ResolvedModel)

**Analog:** the Phase-16 additive-kinds family on `Line` (transcript.go:144–156) — D-20 tolerance means old lines simply omit the new field; add `ResolvedModel string` to this exact block:

```go
// Phase-16 additive kinds (D-20..D-23). ... Readers must tolerate these kinds
// AND unknown future kinds/fields.
Args        string   `json:"args,omitempty"`
SourceChain []string `json:"sourceChain,omitempty"` //nolint:tagliatelle // on-disk format
Expansion   string   `json:"expansion,omitempty"`
```

**Writer API to extend** (manager.go:274–280 — grow the signature additively; every caller is in `internal/session/subagent.go` + tests):

```go
func (m *Manager) AppendSubagentDispatch(parentTurnID, subagentTurnID, toolCallID string, restricted []string) error {
	return m.appendLine(&Line{
		Type: TypeSubagentDispatch, TurnID: subagentTurnID, Timestamp: now(),
		ParentTurnID: parentTurnID, SubagentTurnID: subagentTurnID,
		ToolCallID: toolCallID, RestrictedTools: restricted,
	})
}
```

**The append+redact discipline class-B output and local_command already ride** (manager.go:362–367 — `AppendLocalCommand` exists and routes through the REDACTED `appendLine`; class-B handlers just call it, per Pitfall 10):

```go
func (m *Manager) AppendLocalCommand(turnID, key, args, expansion string, sourceChain []string) error {
	return m.appendLine(&Line{
		Type: TypeLocalCommand, TurnID: turnID, Timestamp: now(),
		Name: key, Args: args, Expansion: expansion, SourceChain: sourceChain,
	})
}
```

---

### `internal/acp/types.go` + `internal/acp/server.go` (MOD — available_commands_update)

**Analog (frame types + kind constant):** `types.go:203–234` — `KindConfigOptionUpdate` and `ConfigOptionFrame` are the template for `KindAvailableCommandsUpdate = "available_commands_update"`, `AvailableCommandFrame{Name, Description, Input *AvailableCommandInputFrame}`, with the same `//nolint:tagliatelle` discipline and "field names pinned VERBATIM against schema/v1" header comment style:

```go
const KindConfigOptionUpdate = "config_option_update"

type ConfigOptionFrame struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	...
	CurrentValue string              `json:"currentValue"` //nolint:tagliatelle // ACP wire field (D-11)
}
```

**ContentChunk shape for the D-05 echo (user_message_chunk)** (types.go:176–179 — `ThoughtChunkFrame` is the existing ContentChunk; the echo needs `messageId` distinct from the following agent chunks'):

```go
type ThoughtChunkFrame struct {
	MessageID string       `json:"messageId"` //nolint:tagliatelle // ACP wire field
	Content   ContentBlock `json:"content"`   // required
}
```

**kind-constant family to extend** (emitter.go:17–24 — add `updKindAvailableCommandsUpdate = "available_commands_update"` beside these):

```go
const (
	keySessionUpdate         = "sessionUpdate"
	updKindAgentMessageChunk = "agent_message_chunk"
	updKindToolCall          = "tool_call"
	...
)
```

**The Notify template to mirror** (server.go:299–317 — `NotifyAvailableCommands` is a line-for-line copy with a different payload map and kind; FULL set every send, FOREGROUND lane, best-effort error):

```go
func (s *Server) NotifyConfigOptions(sessionID string, opts []ConfigOptionFrame) error {
	if opts == nil {
		opts = []ConfigOptionFrame{}
	}

	raw, err := json.Marshal(map[string]any{
		keySessionID: sessionID,
		"update": map[string]any{
			keySessionUpdate: KindConfigOptionUpdate,
			"configOptions":  opts,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal config_option_update: %w", err)
	}

	return s.emitter.newHandle(sessionID, classForeground).Notify(
		&Message{JSONRPC: protocolVersion20, Method: methodSessionUpdate, Params: raw})
}
```

**Emission points** (handlers.go:253–271 `handleSessionNew` + handlers.go:152–158 `configOptionsFor` — the session-start advertisement builder pattern; `handleSessionLoad` (handlers.go:457–462) is currently a -32601 no-op, so the Phase-18 resume re-advertise seam must be exposed for it):

```go
func (s *Server) configOptionsFor() []ConfigOptionFrame {
	if s.configSurf == nil {
		return nil
	}

	return s.configSurf.Options()
}
```

---

### `internal/acpserve/acp_serve.go` (MOD — watcher → advertisement wiring)

**Analog:** the surface notify-closure wiring (acp_serve.go:219–225) — the rescan-complete callback binds to `srv.NotifyAvailableCommands` exactly the way `surface.SetNotify` binds to `NotifyConfigOptions`:

```go
surface.SetNotify(func(sessionID string, opts []acp.ConfigOptionFrame) {
	nerr := srv.NotifyConfigOptions(sessionID, opts)
	if nerr != nil {
		log.Printf("ass-guard: config_option_update enqueue failed (continuing): %v", nerr)
	}
})
```

Also mirror: `runner.LoadCommandRegistry()` call site (acp_serve.go:189, becomes load + initial chain build), `runner.SetEmitter(srv.Emitter)` + `StartScheduler(ctx)` ordering (239–240), and the ctx-done goroutine shape (245–249) for watcher teardown.

---

### Test files (NEW — Wave 0)

**Analog (dispatch/chain tests):** `internal/session/subagent_test.go` — helper-builder + transcript-line assertion style (`newSubagentSession` wires fake provider responses + a real bus; tests walk `linesOf(s)` asserting line types/fields). Copy for `TestClassB` (assert zero provider Stream calls + a `local_command` line + `end_turn`) and `TestDispatchModel` (assert `ResolvedModel` on the dispatch line).

**Analog (runner-level tests):** `internal/runtime/runner_battery_test.go` + `e2e_opsx_test.go` (scratch-dir fixtures; gated live-model tests use `//nolint:paralleltest // real scratch + live model`). Copy fixture style for `TestCommandChain` / `TestSkillSlash` / `TestAgentSlash` / `TestRescan`.

**Analog (frame tests):** `internal/acp/server_test.go` / `emitter_test.go` — golden-JSON frame assertions against schema-pinned spellings. Copy for `available_commands_test.go` (Pitfall 5: pin `user_message_chunk`, never `user_message`).

**Analog (discovery fixtures):** `internal/ecosys/loader_test.go` + `precedence_test.go` — temp-dir `.claude/` tree fixtures; copy for `rescan_test.go` (create/remove files under a watched temp root; `-race` concurrent rescan × turn).

---

## Shared Patterns

### Graceful degradation + ONE loud stderr warning
**Source:** `internal/runtime/runtime.go:363` (registry load), `internal/runtime/runtime.go:1321–1326` (cross-provider skip — the wording D-15's degrade replaces), `internal/ecosys/loader.go:801–803` (`logPluginSkipf` — D-11's malformed-file skip), `internal/acpserve/acp_serve.go:231–235` (schedule store disable)
**Apply to:** D-01 shadow-check warning, D-11 malformed discovered files, D-12 watcher failure, D-15 uncredentialed/unknown-slug degrade, D-07 /cost source note
```go
log.Printf("ass-guard: command registry load failed (continuing without slash expansion): %v", err)
```
Transport discipline: stderr only (use `r.stderrOrDefault()`, runtime.go:1289–1295) — stdout stays ACP frames.

### Transcript append via the REDACTED path
**Source:** `internal/session/manager.go:77–104` (appendLine) + `362–367` (AppendLocalCommand)
**Apply to:** all class-B records and the D-16 ResolvedModel field. Never route through `appendLineUnredacted` (type-scoped to raw_thinking, manager.go:106–134).

### Wire frames through the emitter FOREGROUND lane
**Source:** `internal/acp/server.go:299–317`
**Apply to:** `NotifyAvailableCommands` (session start, resume, and every rescan swap — full winner set per D-04/full-replacement semantics) and the D-05 echo/output chunks (via the turn's `emit` handle).

### Turn-mutex + client-turn marking discipline
**Source:** `internal/runtime/runtime.go:489–498`
**Apply to:** the class-B intercept — it runs while holding the per-session turn mutex; keep pure-local handlers off the network and bound /cost's live fetch to the FAST-CONTROL window (registry class at `internal/acp/request_registry.go:105`, used at handlers.go:207) so the queue never wedges (Pitfall 8).

### Deterministic ordering from the registry
**Source:** `internal/ecosys/loader.go:1178–1222` (`AllAgents/AllSkills/AllCommands` sort-by-name)
**Apply to:** the chain advertisement list and `/help` inventory (`/help` is generated FROM the chain — D-08).

## No Analog Found

| File / Concern | Role | Data Flow | Reason |
|----------------|------|-----------|--------|
| fsnotify watch loop (inside `internal/runtime/rescan.go`) | service | event-driven | No filesystem watcher exists anywhere in the codebase (fsnotify absent from go.mod — grep-verified). Use RESEARCH.md "fsnotify watch + debounce skeleton" + §Pitfalls 2–4 verbatim. |
| Immutable chain + atomic-pointer swap | concurrency pattern | — | `r.reg` today is a startup one-shot field (runtime.go:351–357, "loads … ONCE at startup"); no mid-lifecycle swap exists. Nearest partial models: `modelMu/effectiveModel` (runtime.go:134–141) for guarded state and the sync.Map turnMus family (181–190). The swap discipline itself comes from RESEARCH §Pattern 1 + §Pitfall 1. |
| `available_commands_update` frame | wire type | request-response | Absent from the entire codebase (grep-verified). Build on the NotifyConfigOptions template above; wire shape per RESEARCH §Wire Vocabulary. |
| Class-B command handlers (/status /cost /doctor /mcp …) | controller | request-response | No local-command execution surface exists. /compact and /resume DELEGATE to Phase 19/18 machinery; /cost's math rides `CostCeilingTracker` (`internal/modelrouting/cost.go:61–89` Account pricing formula, `:179–184` Spent) + `Target.Pricing` (`config.go:56–59`); /model rides `ApplyTurnModel` (runtime.go:1470–1498) — persist-then-apply via the existing `surface.SetApplyHook` (acp_serve.go:225). |

## Known Locked Divergences (do not "fix" toward CC — Pitfall 9)

- `/clear` = same-session context-reset boundary (D-06), not a new session.
- `/model` = session-scope ephemeral override on top (16-D-12), not a saved default.
- `/memory` = read-only view (D-09); `/doctor` = class-B fixed logic, not a prompt skill.
- `$0` expands to empty (1-based positionals, expand.go:62–63) — zcode parity, not CC's 0-based.
- Echo kind is `user_message_chunk` with a distinct `messageId` — `user_message` does not exist in the v1 SessionUpdate union.

## Metadata

**Analog search scope:** `internal/runtime`, `internal/session`, `internal/ecosys`, `internal/modelrouting`, `internal/acp`, `internal/acpserve`
**Files scanned:** ~20 source files read (full or targeted); 2 absence greps (fsnotify dep, available_commands)
**Pattern extraction date:** 2026-08-28
**All line numbers verified against working tree this session.**
