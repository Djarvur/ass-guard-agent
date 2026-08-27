# Phase 20: Built-in Commands + Skills + Per-Agent Model - Research

**Researched:** 2026-08-28
**Domain:** Slash-command resolver chain (builtins → skills → agents → file commands), ACP `available_commands_update` wire surface, fsnotify live rescan, per-agent `model:` routing into `internal/modelrouting`
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Resolver Chain & Collisions
- **D-01:** Builtin command names (CMDS-02's class-B list + /init) are RESERVED and unoverridable — a discovered file, skill, or agent sharing a builtin name never fires via slash. Matches CC (builtins are fixed logic). Autocomplete cannot lie. — **Reversibility:** costly — the reserved-name list becomes a published contract; adding a builtin later can silently (from the user's view) shadow an existing discovered command, so growth of the list needs a shadow-check warning.
- **D-02:** Collisions among skills/agents/file-commands break by chain order (CMDS-01 letter: builtins → skills → agents → file commands), silently and deterministically. The loser remains reachable through its native surface (agent dispatch via the Agent tool, skills via the Skill tool) — only the slash name is exclusive.
- **D-03:** `/agent-name` (SKLS-02) DISPATCHES the subagent: the invocation body becomes the subagent's prompt, the discovered agent's Prompt/Tools apply (existing PARA machinery), and the result returns into the conversation as a subagent turn — not a prompt expansion.
- **D-04:** available_commands_update lists WINNERS ONLY per name — what autocomplete shows is exactly what runs. One name, one truth; shadowed entries are absent, not annotated.

#### Class-B Command Surface
- **D-05:** Class-B output shape: session/update user_message echoing the typed /command, then agent_message chunks carrying the output, then stopReason end_turn. Zero model turns; the local_command transcript line (16-D-22 full record: key, args, source, outcome) is the durable record. ACP has no special local-command wire semantics — commands arrive as ordinary prompt text and answer per normal turn shape.
- **D-06:** `/clear` writes a full context-reset boundary in the SAME session — the Projector's lean window starts empty; transcript and session id survive; resume is unaffected (CC parity: clears context, not the session).
- **D-07:** `/cost` numbers: use the provider's usage/billing endpoint live when one exists (operator directive) — on-demand fetch at invocation under the FAST-CONTROL timeout class (~10s, 16-D-17); on timeout or error fall back to transcript usage × modelrouting cost table WITH a source note naming which produced the number. Transcript-derived numbers survive resume (18-D-01 transcript-as-truth).
- **D-08:** `/status` = live session snapshot (resolved model + tier, provider, session id, turn count, context-usage estimate, degraded-capability flags from the counters family). `/help` = inventory generated FROM the resolver chain itself — self-describing, never drifts from reality.
- **D-09:** `/memory` = read-only view of loaded memory sources (agent-md files, learning-store summary); grows richer when PAR-04 lands in Phase 21. No inline editing — ACP has no editor-open mechanism.

#### Live Rescan
- **D-10:** Live rescan (CMDS-04) = fsnotify watches on the discovery directories (debounced) driving rescan + available_commands_update re-fire, AND an invoke-time freshness re-check before every slash resolution. Watch gives autocomplete immediacy; the invoke-time backstop guarantees resolution never runs against a stale chain when the watch misses (editor quirks, races).
- **D-11:** Malformed discovered files (bad frontmatter, unreadable) are skipped with ONE structured warning naming file + reason; the rest of the registry loads — graceful-degradation family, MCP-host precedent.
- **D-12:** Watcher failure (unsupported filesystem, resource limits): ONE loud degrade warning + fallback to invoke-time-only rescan. Commands still resolve correctly; autocomplete just updates later. A session never dies over discovery.

#### Per-Agent Model Routing
- **D-13:** Strict SKLS-03 precedence: frontmatter `model:` > dispatch-time model > session (parent) model > tier. An agent file with NO model frontmatter runs on the PARENT model — tiers.light is consulted only when no session model exists at all (degraded session). — **Reversibility:** costly — this REVERSES 14-05's unconditional light-tier subagent default; operators relying on light-tier economics must now write `model:` into agent frontmatter explicitly. Token spend changes silently for existing unset agents.
- **D-14:** `model: inherit` parses as unset → parent model (CC keyword parity). Any other slug routes to that model subject to D-15.
- **D-15:** Cross-provider frontmatter models ROUTE (operator override of the existing skip precedent): the dispatch builds/reaches the second provider through the modelrouting provider factory. If that provider's credentials/config are missing, the dispatch DEGRADES to the parent model + exactly ONE loud warning naming the intended model — a turn never fails over routing. — **Reversibility:** costly — a second live provider instance in one process is a new lifecycle surface (key loading, breaker state) that Phase 22's background work builds on.
- **D-16:** resolvedModel is reported BOTH ways: a field on the subagent_dispatch transcript line (durable — replay and audit see what actually ran) AND a session/update note at dispatch time (live editor visibility). Criterion 6's "reported back" holds for both live and post-hoc inspection.

### Claude's Discretion
- Debounce window for watch events (100–500ms per research; pick and document).
- Which directories get watches (project + user discovery roots; plugin caches as forced).
- /doctor's concrete check list; /mcp's output shape.
- Invalid (non-inherit, unknown) model slug in frontmatter: degrade-to-parent + loud warning follows the D-15 family — exact wording and counter name.
- Echo user_message formatting (command + args verbatim vs redacted).

### Deferred Ideas (OUT OF SCOPE)
- Namespaced autocomplete (/foo@agent suffixes for colliding names) — rejected for CC divergence; revisit only if silent shadowing proves painful in practice.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description (from REQUIREMENTS.md) | Research Support |
|----|-------------------------------------|------------------|
| ACP-04 | Editor autocompletes `/` commands: `available_commands_update` sent on session start and on discovery change | Wire shape verified verbatim from canonical v1 schema (§Wire Vocabulary); frame does NOT exist in code yet — Phase 20 builds it on the Phase-16 TurnEmitter `Notify` seam (§Architecture Patterns); full-replacement semantics pinned |
| CMDS-01 | Slash-invocation resolves via one chain: builtins → skills → agents → file-discovered commands (current behavior last) | The chain generalizes the single `r.reg.Commands[key]` lookup at the heart of `expandUserBlocks`/`invocationFor` (runtime.go:395–476, read this session); registry maps + precedence machinery mapped (§Architecture Patterns) |
| CMDS-02 | Class-B control-plane commands execute at the runner seam with NO model turn and write `local_command` transcript lines (12 commands listed) | `AppendLocalCommand(turnID, key, args, expansion, sourceChain)` already exists (manager.go:362); class-B intercept point in `Runner.Run` mapped (§Architecture Patterns); CC fixed-logic parity verified |
| CMDS-03 | Class-A prompt-expanding commands (/init) ride the existing `expandUserBlocks` seam and are recorded as user turns with provenance | The seam is intact and engine-path-aware (runtime.go:539–541; enginebridge.go:142–155); /init is a builtin-synthesized Command value — no code change to the seam itself |
| CMDS-04 | Live rescan — newly created/installed commands, skills, agents (and removals) discovered without restart; available_commands_update re-fires | fsnotify v1.10.1 verified (watch dirs not files, non-recursive, no polling watcher); D-10's two-layer design (watch + invoke-time re-check) maps onto verified fsnotify limitations; registry hot-swap race hazard identified (§Common Pitfalls 1) |
| SKLS-01 | Typing `/<skill-name>` resolves skill keys in invocationFor; SKILL.md body expands as the prompt with args appended | `ecosys.ResolveSkill` re-reads the SKILL.md body by registry key (skills.go:111–125); `Command.Expand` substitution semantics ($ARGUMENTS, $1..$9, append rule) reuse path mapped (§Architecture Patterns) |
| SKLS-02 | Discovered AGENTS addressable as slash commands (BMad-style `.claude/agents/*.md`) | `discoverAgents` already walks project+user+plugin `.claude/agents/` (loader.go:185–224); CC docs confirm `.claude/agents/*.md` frontmatter shape + `inherit` semantics; D-03 dispatch rides `DispatchSubagent` (subagent.go:43–123) |
| SKLS-03 | Per-agent `model:` frontmatter wired into subagent dispatch — precedence frontmatter > dispatch > session default > tier — with resolvedModel reported back | `Agent.Model` already parsed (loader.go:269–272); `subagentProfile` (subagent.go:273–288) is the stamp site D-13 modifies; `resolveSubagentModel` (runtime.go:1311–1331) is the 14-05 machinery D-13 reverses; modelrouting `buildTarget`/`ProviderFactory.Build` mapped for D-15 cross-provider routing |
</phase_requirements>

## Summary

Phase 20 unifies slash invocation into one resolver chain and adds three subsystems around it: (1) the ACP-04 command-advertisement wire surface, (2) fsnotify-based live rescan, (3) per-agent model routing. The codebase groundwork is unusually strong: the expansion seam (`expandUserBlocks`/`invocationFor`, runtime.go:395–476) already does single-parse resolution with provenance + boundary discipline; the `local_command` transcript kind and `AppendLocalCommand` writer already exist (Phase 16, transcript.go:66 + manager.go:362); the `Agent` struct already carries `Model` (types.go:60–67); and `DispatchSubagent` already applies per-type Prompt/Tools through the per-dispatch profile copy. What does NOT exist yet: any `available_commands_update` frame (grep-verified absent from the entire codebase — Phase 16 built the TurnEmitter seam but no command frames), the resolver chain itself, the builtin command table, the watcher, and the D-13/D-15 routing precedence.

Two wire-level findings materially shape planning. **First**, the v1 `SessionUpdate` union (fetched from the canonical zed-industries schema @ main) has **no `user_message` kind** — D-05's echo maps to **`user_message_chunk`** (a `ContentChunk`: `content` + shared `messageId`). This is the same locked-intent-holds/transport-corrected pattern as 16-RESEARCH's D-10 reconciliation; the echo, the output chunks, and `end_turn` are all expressible. **Second**, the same union exposes `usage_update` (`used`, `size`, `cost{amount, currency, …}`) and `session_info_update` — v1 kinds the editor natively renders — which are natural (optional) carriers for /status//cost context data; using them is planner discretion, NOT a locked requirement (D-05's locked shape is chunk-echo + chunks + end_turn).

The riskiest interaction is **registry hot-swap under concurrency**: `LoadCommandRegistry` replaces `r.reg` wholesale, but live sessions hold aliased references (`SubagentTypes: r.reg.Agents` at sessionFor, runtime.go:1190) and every turn reads `r.reg.Commands` on the turn goroutine — a rescan that mutates in place is a data race, and a wholesale replace strands existing sessions on stale maps (breaking D-10's "picked up without restarting the session" for agent dispatch). The research recommends an immutable-registry pointer swap (atomic/RWMutex-guarded) with sessions re-resolving through the runner, not through their construction-time snapshot.

**Primary recommendation:** build one `internal/ecosys`-level resolved-command view (the chain: reserved builtins → skills → agents → file commands, winners-only per D-02/D-04) held behind an atomic pointer on the Runner; intercept class-B at the top of `Runner.Run` (before the engine branch, after `routeAskReply`); emit `available_commands_update` through a `NotifyAvailableCommands` mirroring `NotifyConfigOptions` (server.go:299–317); rescan via fsnotify directory watches + debounced atomic swap + invoke-time re-check; implement D-13/D-15 inside `subagentProfile` + a new dispatch-time provider selector over `Resolver.buildTarget`/`ProviderFactory.BuildWithCapturer`.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Resolver chain (builtins → skills → agents → file) | internal/runtime Runner (chain view over `ecosys.Registry`) | internal/ecosys (registry types, Expand, ResolveSkill) | 15-D-20: ACP/command vocabulary stays out of session core; the chain consumes the registry the runtime already owns (r.reg) |
| Reserved builtin table + class-B handlers | internal/runtime (runner seam) | internal/modelrouting (cost table), internal/session (boundaries, compaction call-through) | CMDS-02 locks "at the runner seam"; handlers call existing machinery, never duplicate it |
| Class-A expansion (/init) | internal/runtime `expandUserBlocks` (unchanged) | internal/ecosys Expand | CMDS-03: seam rides unchanged; /init enters as a synthesized registry entry |
| `available_commands_update` frame + emission | internal/acp (frame type, `NotifyAvailableCommands`) | internal/acpserve (composition: session-start + rescan firing) | Wire concern lives with the codec (16-RESEARCH responsibility split); acpserve owns when |
| Live rescan (fsnotify watch + debounce + swap) | internal/acpserve or internal/runtime (planner picks owner) | internal/ecosys `Discover` (re-run target) | Watch lifecycle ties to serve ctx (CONTEXT integration point); Discover is already the one-call rescan primitive |
| Per-agent model precedence + cross-provider dispatch | internal/session `subagentProfile` + dispatch site | internal/modelrouting (Resolver.buildTarget, ProviderFactory) | D-13 modifies the existing stamp seam; D-15's second provider comes from the existing factory — no new construction seam |
| resolvedModel reporting | internal/session (transcript line field) + internal/runtime (advisory-note emit) | — | D-16's both-ways reporting: durable field on subagent_dispatch line + live note via the established advisory machinery |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib (`os`, `sync/atomic`, `time`, `regexp`) | go 1.26 (go.mod pin; toolchain 1.26.5 verified installed) | Registry swap (atomic.Pointer), debounce timer, chain matching | Zero-dep discipline; everything needed exists [VERIFIED: go.mod:3 + `go version` 1.26.5] |
| `github.com/fsnotify/fsnotify` | v1.10.1 | D-10 filesystem watches on discovery directories | THE standard Go fs-notification library; event-driven, cross-platform (inotify/FSEvents/kqueue/ReadDirectoryChangesW); requires Go 1.23+ [VERIFIED: `go list -m -versions` shows v1.10.1 latest; github.com/fsnotify/fsnotify README] |
| `gopkg.in/yaml.v3` | v3.0.1 (already required) | Frontmatter parse on rescan (existing loader paths) | Already the loader's parser — no new dep [VERIFIED: go.mod:14] |
| internal/ecosys, internal/runtime, internal/session, internal/modelrouting, internal/acp, internal/acpserve | (project packages) | Every subsystem this phase touches exists | Mapped in §Architecture Patterns [VERIFIED: file reads this session] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `stretchr/testify` | v1.11.1 (already required) | Assertions in new tests | Optional, repo-standard for table tests [VERIFIED: go.mod:13] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| fsnotify directory watches | Invoke-time re-discovery only | D-10 LOCKS both layers — watch gives autocomplete immediacy, invoke-time is the backstop. Watch-less would violate D-10. |
| fsnotify | Hand-rolled polling watcher | fsnotify README: a polling watcher is "not yet implemented" upstream (#9); hand-rolling polling for NFS/SMB is deferred — D-12's locked fallback is invoke-time-only, not polling |
| atomic.Pointer registry swap | RWMutex around r.reg | Pointer swap is lock-free on the read-hot path (every turn); RWMutex adds contention. Either is correct; pick one and document |
| Reuse `Command.Expand` for skills | New skill-expansion code | None — synthesize `Command{Body: skillBody}` and call `Expand`; the $ARGUMENTS/$1–9/append semantics are already locked zcode parity |

**Installation:**
```bash
go get github.com/fsnotify/fsnotify@v1.10.1
```

## Package Legitimacy Audit

> One new external package this phase. The gsd-tools legitimacy seam supports npm|pypi|crates only — **no Go ecosystem check exists**, so verification used the Go module proxy + the official upstream repo directly.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| github.com/fsnotify/fsnotify | Go module proxy (module go list) | 10+ yrs (project since 2014) | de-facto standard (used by Hugo, Prometheus, air, …) | github.com/fsnotify/fsnotify (official README fetched this session) | OK | Approved — v1.10.1 pinned |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

*fsnotify provenance: version v1.10.1 confirmed via `go list -m -versions github.com/fsnotify/fsnotify` (module proxy) AND the official repo README fetched this session (Go 1.23+ requirement, watch-guidance, platform table). The package name was additionally known from the locked decision D-10 itself ("fsnotify watches"), i.e. operator-supplied, not search-discovered.*

## Wire Vocabulary (verified from canonical sources)

### available_commands_update (the ACP-04 frame)

Session/update notification, params `{sessionId, update}`:

```json
{
  "jsonrpc": "2.0", "method": "session/update",
  "params": {
    "sessionId": "…",
    "update": {
      "sessionUpdate": "available_commands_update",
      "availableCommands": [
        { "name": "init", "description": "…", "input": { "hint": "…" } }
      ]
    }
  }
}
```

- `AvailableCommand`: `name` (required), `description` (required), `input` (optional; v1 has exactly one variant, `unstructured`, whose `hint` is "A hint to display when the input hasn't been provided yet"). [VERIFIED: zed-industries/agent-client-protocol schema/v1/schema.json $defs/AvailableCommand + AvailableCommandInput, fetched 2026-08-28]
- **Full replacement**: "The Agent can update the list of available commands at any time during a session by sending another available_commands_update notification" — the client holds no merge state; every send is the COMPLETE current set. [VERIFIED: agentclientprotocol.com/protocol/v1/slash-commands + schema def — matches 16-RESEARCH Verdict 2]
- Sending after session creation is MAY, not MUST; invocation is ordinary `session/prompt` text ("/web agent client protocol") and "Commands may be accompanied by any other user message content types". [VERIFIED: agentclientprotocol.com/protocol/v1/slash-commands]

### SessionUpdate union — the complete v1 kind set

From the canonical schema (`$defs/SessionUpdate.oneOf`), all eleven kinds: `user_message_chunk`, `agent_message_chunk`, `agent_thought_chunk`, `tool_call`, `tool_call_update`, `plan`, `available_commands_update`, `current_mode_update`, `config_option_update`, `session_info_update`, `usage_update`. **There is NO `user_message` kind** — D-05's "session/update user_message echoing the typed /command" maps on the wire to **`user_message_chunk`** (a `ContentChunk`: `content` ContentBlock + shared `messageId`; "A chunk of the user's message being streamed"). [VERIFIED: schema/v1/schema.json fetched 2026-08-28]

**D-05 reconciliation (locked intent holds, transport observation corrected — same pattern as 16-RESEARCH §D-10 Reconciliation):** the class-B echo is a `user_message_chunk` carrying one text ContentBlock with the typed command; `messageId` must differ from the following agent chunks' messageId so the client renders a new message ("A change in messageId indicates a new message has started"). The output then streams as `agent_message_chunk`s, and the turn returns `stopReason: "end_turn"`. Zero model turns.

### Opportunistic v1 kinds (available, NOT required by any locked decision)

- `usage_update`: `{used: uint64 (required), size: uint64 (required), cost: {amount, currency, …} | null}` — "Context window and cost update for a session." A natural live carrier for /status context-usage and /cost numbers into Zed's native UI. [VERIFIED: $defs/UsageUpdate]
- `session_info_update`: `{title?, updatedAt?}` — dynamic session titles. Out of phase scope; noted for completeness. [VERIFIED: $defs/SessionInfoUpdate]

Planner discretion: D-05's locked output shape (chunks + end_turn + local_command line) is the contract; emitting `usage_update` alongside is an optional enhancement, not a requirement. Recommend deferring unless trivial — do not let it grow scope.

## Architecture Patterns

### System Architecture Diagram

```
                        Zed (ACP client)
                             │ session/prompt "/status extra"      ▲ available_commands_update
                             │ session/prompt "/agent-x do thing"  │ session/update (user_message_chunk echo,
                             ▼                                      │  agent_message_chunk output)
                ┌─────────────────────────── internal/acp Server ───────────────────────────┐
                │ handleSessionPrompt ──► TurnRunner.Run ─────────────────────────────────── │
                │        (NotifyAvailableCommands via emitter classForeground .Notify)       │
                └───────────────────────────────────┬───────────────────────────────────────┘
                                                    ▼
                     internal/runtime Runner.Run (turn mutex held)
                              │
                              ├─ routeAskReply? ── yes → ask-answer path (unchanged)
                              │
                              ├─ NEW class-B intercept: chain.Resolve(firstTextBlock)
                              │        │
                              │        ├─ builtin class-B (/help /status /cost /mcp /memory
                              │        │   /permissions /doctor /config /model /clear /resume
                              │        │   /compact) → handler → AppendLocalCommand → echo +
                              │        │   output chunks → end_turn   [NO model turn]
                              │        │
                              │        ├─ builtin class-A (/init) → fall THROUGH to expansion
              resolver chain  │        │    (expandUserBlocks seam, unchanged behavior)
       (atomic pointer swap)  │        │
                              │        ├─ skill → synthesize Command{Body: SKILL.md body},
                              │        │    Expand(args) → expansion path + provenance
                              │        │
                              │        ├─ agent → DispatchSubagent(prompt=args/body,
                              │        │    agentDef) → subagent turn streams into this
                              │        │    turn's forwarder → end_turn  [NO parent model turn]
                              │        │
                              │        └─ file command (or nothing) → existing behavior
                              ▼
                 engine branch (if engine on: adapter expands via cfg.Invoke)
                              ▼
                        runOneTurn → provider (skipped entirely for class-B)

  ── rescan side-channel ────────────────────────────────────────────────
  fsnotify watches (project+user discovery roots, debounced 100–500ms)
        │ events                     every slash resolution: invoke-time
        ▼                            freshness re-check (D-10 backstop)
  ecosys.Discover(workDir) ──► build chain view ──► atomic swap ──► NotifyAvailableCommands (re-fire)
```

### Recommended Project Structure

```
internal/ecosys/            # chain view builder? NO — keep ecosys discovery-only (loader is pure)
internal/runtime/
├── commands.go             # NEW: builtin table (name → class-B handler / class-A Command),
│                           #   chain resolver (builtins→skills→agents→file, winners-only),
│                           #   reserved-name set + shadow-check warning (D-01)
├── rescan.go               # NEW: fsnotify watch set, debounce, Discover re-run, atomic swap,
│                           #   watcher-failure degrade (D-12)   [planner may prefer acpserve]
└── runtime.go              # class-B intercept in Run; subagent model precedence (D-13/D-15)
internal/session/
├── transcript.go           # +ResolvedModel field on Line (additive, D-16)
└── manager.go              # AppendSubagentDispatch gains resolvedModel param
internal/acp/
├── types.go                # AvailableCommandsFrame type + method/kind constants
└── server.go               # NotifyAvailableCommands (mirror of NotifyConfigOptions :299–317)
internal/acpserve/
└── acp_serve.go            # wire watcher → runner; fire initial advertisement per session
```

### Pattern 1: One chain view, winners-only, behind an atomic pointer

**What:** The chain is a derived view: `map[name]entry` where entry = `{kind: builtin|skill|agent|file, …}`. Builtins pre-fill the map (D-01 reserved — discovered entries that collide are dropped at build time with the D-01 shadow-check warning); then skills, then agents, then file commands overlay in order, first-writer-wins per D-02. The advertisement list is generated FROM this map (D-04: winners only) and `/help` is generated from it too (D-08: self-describing).

**When to use:** everywhere slash resolution happens — `expandUserBlocks`, `invocationFor` (which the engine adapter consumes via `cfg.Invoke`), the class-B intercept, and autocomplete.

**Example:**

```go
// Source: synthesis over runtime.go:395–476 (read this session) + D-01..D-04
type chainEntry struct {
    kind string // "builtin" | "skill" | "agent" | "file"
    // builtin: the class handler or class-A Command; skill/agent/file: registry key
    key    string
    desc   string
    hint   string // argument-hint / "(arguments)" for the wire input.hint
}

// resolve returns the winner for name — single parse, multiple consumers,
// the invocationFor shape (runtime.go:458–476).
func (c *commandChain) resolve(name string) (chainEntry, bool)
```

**Concurrency:** the whole `commandChain` is immutable once built; the Runner holds it via `atomic.Pointer[commandChain]` (or behind the existing session mutex family). Readers (turn goroutines, advertisement builder) load the pointer; the rescan builds a fresh chain off-thread and swaps. NEVER mutate a live chain, and NEVER let sessions keep construction-time sub-slices of it (see Pitfall 1).

### Pattern 2: Class-B intercept at the top of Runner.Run

**What:** A class-B invocation short-circuits before the engine branch — `routeAskReply` has already had its chance (runtime.go:525), the turn mutex is held, the chunk forwarder is subscribed, so the handler just emits the echo + output and returns `end_turn`. This placement is load-bearing: the engine path (runtime.go:539) defers expansion to the adapter, so an intercept placed after that branch would behave differently with the engine on/off. CMDS-02 says "at the runner seam" — this is the seam.

```go
// Source: runtime.go Run structure (read this session); ordering per CONTEXT integration points
sess := r.sessionFor(ctx, sessionID)
turnMu := r.sessionTurnMu(sessionID); turnMu.Lock(); defer turnMu.Unlock()
r.markClientTurn(sessionID, true); defer r.markClientTurn(sessionID, false)
// ...forwarder subscriptions (existing)...
blocks := toContentBlocks(prompt)
if stop, handled := r.routeAskReply(ctx, sess, blocks); handled { /* existing */ }

if key, args, ok := ecosys.ParseInvocation(firstText(blocks)); ok {
    if handled, stop := r.tryBuiltinCommand(ctx, sess, emit, key, args); handled {
        return stop, nil // end_turn; AppendLocalCommand already written; zero provider calls
    }
}
// existing engine/expansion flow unchanged
```

D-03's `/agent-name` dispatch rides the same intercept (a `kind: agent` winner calls `DispatchSubagent` and streams its result through the already-subscribed forwarder; the subagent's chunks are bus events — PARA-02 — so they reach the client inside this turn's forwarder, then `end_turn`). Class-A (`/init`) and skills fall through to the expansion path.

### Pattern 3: available_commands_update emission — mirror NotifyConfigOptions

**What:** Phase 16 already established the exact template: build the params JSON, enqueue through `emitter.newHandle(sessionID, classForeground).Notify(&Message{...})` (internal/acp/server.go:299–317). Add `NotifyAvailableCommands(sessionID string, cmds []AvailableCommandFrame) error` beside it, plus the `updKindAvailableCommandsUpdate = "available_commands_update"` constant beside the existing `updKind*` family (emitter.go:19–23). Emission points: (a) after session/new (and per ACP-06's Phase-18 contract, after session/load resume — "commands re-advertised" is already in ACP-06's letter, so expose a re-advertise entry Phase 18 can call); (b) after every rescan swap. Full list every time (D-04 + full-replacement semantics).

### Pattern 4: D-13/D-15 routing inside the existing per-dispatch profile seam

**What:** `subagentProfile` (session/subagent.go:273–288) is where `SubagentModel` stamps `prof.Model` today (14-05). D-13 changes the RESOLUTION, not the stamp site: precedence becomes frontmatter `Model` (parsed since 12-02, loader.go:269–272) → dispatch-time model (the subagent_type call's parameter, today effectively unused) → session/parent model (`prof.Model` as-is) → tier (only when no session model exists at all — the degraded case 14-05's machinery still covers). `model: inherit` normalizes to "" at chain-view build time (D-14; CC parity: omitted ALSO means inherit [CITED: code.claude.com/docs/en/sub-agents]).

**Cross-provider (D-15):** when the winning slug's `buildTarget` lands on a provider != the session's provider (resolver.go:101–122 — the slug must be DECLARED in `cfg.Models`, else it's the unknown-slug degrade path), the dispatch obtains a second `provider.Provider` through `ProviderFactory.BuildWithCapturer` (factory.go:142–171 — the SINGLE construction seam; uncredentialed returns the lazy-failing `noCredentialProvider`, which is exactly the D-15 "missing credentials → degrade + ONE loud warning" tripwire). Cache the built provider per (provider, session) to avoid per-dispatch construction; its lifecycle rides the session (D-15 flags it as a new lifecycle surface Phase 22 builds on).

**resolvedModel (D-16):** extend `AppendSubagentDispatch(parentTurnID, subagentTurnID, toolCallID, restricted)` with the resolved model (additive `ResolvedModel` field on `Line`, transcript.go:149–155 family — D-20 tolerance covers old lines missing it). The live note reuses the advisory-note machinery (collectAdvisory → `emit.AgentMessageChunk` post-turn, runtime.go:550–563) or a direct emitter chunk at dispatch time.

### Pattern 5: /cost per D-07 — provider endpoint first, transcript fallback second

The modelrouting side of the fallback is ready: `Target.Pricing` (config.go:131–138) + `CostCeilingTracker.Account/Spent` (cost.go:66/179) give transcript-usage × price-list math. The provider-live leg needs a per-provider usage/billing query tried under the FAST-CONTROL timeout class (~10s, 16-D-17's registry classes); on absence/timeout/error, fall back and label the source (D-07). Which providers actually expose queryable usage endpoints is an open verification item (A1) — build the seam so a provider without an endpoint is a config-declared "none", not a runtime surprise.

### Anti-Patterns to Avoid

- **Mutating `r.reg` in place on rescan** — data race with turn goroutines reading `reg.Commands` (expandUserBlocks runs every turn). Build-new-then-swap.
- **Letting sessions hold construction-time registry sub-references** — `SubagentTypes: r.reg.Agents` (runtime.go:1190) aliases the OLD map after a swap; a new agent file would never be dispatchable from a pre-existing session, violating criterion 2. Route agent lookup through the runner's live chain.
- **Putting the class-B intercept after the engine branch** — divergent behavior engine-on/off, and the engine would see (and chain on) commands that must never reach a model.
- **A second config-write path for /model** — D-12/16-D-12: /model is the ephemeral TOP of the precedence chain; it rides `ApplyTurnModel` (runtime.go:1470–1492, the 16-05 live-apply seam) + `SetTurnModel`; it must NOT persist into project/global layers the way editor sets do.
- **An agent-side TUI/picker for /resume** — REQUIREMENTS.md Out-of-Scope anti-feature ("clients own list UX"); /resume surfaces pointer text + relies on Phase 18's session/list surface.
- **Re-parsing invocation text per consumer** — keep the single-parse discipline (invocationFor pattern); ParseInvocation once, resolve once.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Filesystem watching | inotify/FSEvents syscalls or a polling looper | `github.com/fsnotify/fsnotify` v1.10.1 | Cross-platform event plumbing is subtle (atomic-rename storms, spotlight dupes, kqueue fd limits); fsnotify IS the ecosystem standard [VERIFIED: README] |
| YAML frontmatter parsing | a second parser for skills/agents | existing `splitFrontmatter` + `yaml.v3` loader paths | Loader already handles the tolerant forms (toolsList scalar/list/comma, flat command frontmatter) [VERIFIED: loader.go:805–1057] |
| Arg substitution semantics | a skill-specific expander | `ecosys.Command.Expand` (synthesize a Command value) | $ARGUMENTS/$1–9/append rule is locked zcode parity, tested since Phase 8 [VERIFIED: expand.go:35–87] |
| Model routing/fallback/provider construction | per-dispatch provider logic | `modelrouting.Resolver.buildTarget` + `ProviderFactory.BuildWithCapturer` | The single construction seam (09-01 AUD-01); duplicating it breaks capture/tracer parity [VERIFIED: factory.go:128–171] |
| Cost math | ad-hoc price lookup | `Target.Pricing` + `CostCeilingTracker` accounting | Already feeds /cost's fallback and breaker ceilings [VERIFIED: cost.go:24–187] |
| Compaction | a /compact-specific summarizer | Phase 19's single machinery, immediate trigger (19-D-11) | "One implementation, two triggers" is the locked 19-D-11 contract |

**Key insight:** this phase is almost entirely *wiring* — the expensive machinery (discovery, expansion, dispatch, routing, compaction, config mutation, transcript kinds) all exists and is tested. The new code is the chain view, the builtin table, the watcher, the wire frame, and the routing precedence.

## Common Pitfalls

### Pitfall 1: Registry hot-swap races and stale aliases (the big one)
**What goes wrong:** rescan mutates `r.reg` maps while a turn goroutine reads them (race — `-race` CI will catch it late), or replaces `r.reg` wholesale while live sessions hold `SubagentTypes: r.reg.Agents` / stale chain entries (silently stale dispatch targets — violates criterion 2 for agents created after session start).
**Why it happens:** `LoadCommandRegistry` was designed as a startup one-shot (runtime.go:351–383: "loads … ONCE at startup"); nothing mid-lifecycle ever replaced it.
**How to avoid:** immutable chain built off-thread + atomic pointer swap; sessions resolve agents/skills through the runner's live chain, not construction-time references. Add a `-race` test that rescans concurrently with turns.
**Warning signs:** flaky `-race` failures in runner tests; "new agent not dispatchable until new session" bug reports.

### Pitfall 2: fsnotify watch roots must exist at watch time; watches are non-recursive
**What goes wrong:** watching `<project>/.claude/skills` fails or silently no-ops when the dir doesn't exist yet; a `skills/` subdir created later fires no events; new nested dirs aren't covered.
**Why it happens:** fsnotify requires `watcher.Add` on existing paths and does not recurse ("you must add watches for any directory you want to watch"). CC documents the SAME limitation: it picks up file changes in existing watched dirs mid-session, but "If you create a top-level skills directory that didn't exist when the session started, restart Claude Code". [VERIFIED: fsnotify README; code.claude.com/docs/en/skills §Troubleshooting]
**How to avoid:** watch the stable PARENTS (`.claude/`, `.ass-guard/`, and the project root for `.claude/` creation — D-10 discretion covers the watch set) plus each existing discovery leaf; rely on the invoke-time re-check (D-10) for anything the watch set can't see; on watch-root creation, `Add` it dynamically.
**Warning signs:** "command added but autocomplete didn't update" reports concentrated on first-ever installs.

### Pitfall 3: Editor atomic writes fan out multiple events per save
**What goes wrong:** editors write temp+rename, so one save produces CREATE/REMOVE/WRITE bursts; an undebounced rescan per event hammers `Discover` and re-fires `available_commands_update` repeatedly (autocomplete flicker).
**How to avoid:** debounce 100–500ms (D-10 discretion; CONTEXT research note recommends this range), coalesce pending paths into one rescan, ignore Chmod.
**Warning signs:** multiple advertisement frames per single save in the simulator.

### Pitfall 4: fsnotify has NO polling watcher — network filesystems just degrade
**What goes wrong:** assuming D-12's fallback includes polling; NFS/SMB send no events at all (README: NFS/SMB "does not provide network level support"; upstream polling watcher #9 "not yet implemented").
**How to avoid:** D-12's locked fallback is invoke-time-only re-check — on watcher construction failure or a degenerate event stream, log ONE loud warning and rely on the backstop. Do not promise watch immediacy on network mounts.
**Warning signs:** plans that mention a polling ticker as the D-12 mechanism.

### Pitfall 5: The echo kind is `user_message_chunk`, not `user_message`
**What goes wrong:** emitting `"sessionUpdate": "user_message"` — not in the v1 union; Zed drops or errors the frame and the class-B echo disappears.
**How to avoid:** use `user_message_chunk` with a distinct `messageId` per §Wire Vocabulary; add a schema-derived frame test.
**Warning signs:** no echo bubble in Zed on /status.

### Pitfall 6: Frontmatter model slugs that aren't declared in modelrouting config
**What goes wrong:** real-world agent files carry CC aliases (`sonnet`, `opus`, `haiku`) or full IDs absent from `cfg.Models`; `buildTarget` errors (resolver.go:101–106) and, handled carelessly, turns fail.
**How to avoid:** D-15's degrade path is the contract: unknown/non-inherit slug → parent model + exactly ONE loud warning naming the intended model + counter (discretion covers wording). Never fail the dispatch. Note `inherit` normalizes to unset (D-14) BEFORE lookup.
**Warning signs:** dispatched subagents silently all on parent model; one stderr line per dispatch.

### Pitfall 7: Builtin names must fit the invocation regex and the chain must reserve them
**What goes wrong:** a builtin name outside `^[a-z0-9][a-z0-9_:-]{0,63}$` can never fire (ParseInvocation, expand.go:14); a discovered file named `help.md` would shadow /help if builtins aren't pre-seated in the chain.
**How to avoid:** all twelve class-B names + `init` are lowercase ASCII (fit fine); pre-seat builtins at chain build (D-01) and emit the D-01 shadow-check warning when a discovered entry loses to a builtin name.
**Warning signs:** a user's `commands/help.md` that "doesn't work".

### Pitfall 8: Class-B handlers blocking the turn mutex
**What goes wrong:** /cost's live provider fetch (D-07) or /mcp's server probes running long while holding the per-session turn mutex wedges the next prompt and the automation queue (12-07 semantics).
**How to avoid:** keep handlers under the FAST-CONTROL ~10s budget; timeout → fallback path (D-07's own rule); pure-local handlers (/help /status /memory /clear /model /config) must never touch the network.
**Warning signs:** queued-turn warnings after a /cost on a slow provider.

### Pitfall 9: Known, deliberate CC divergences — do not "fix" them
- `/clear`: CC starts a NEW conversation; D-06 locks a same-session context-reset boundary. Locked.
- `/model`: CC saves as default; D-12/16-D-12 lock session-scope ephemeral override on top.
- `/memory`: CC opens editors; D-09 locks read-only (ACP has no editor-open mechanism).
- `/doctor`: CC's is a prompt-based bundled Skill; here it is class-B fixed logic (locked in CMDS-02).
- `$N` positionals: CC docs are 0-based ($0 = first arg); ass-guard's Expand is locked zcode parity (1-based, `$0` → empty, expand.go:62). Skill bodies written for CC with `$0` shift by one — documented divergence, existing locked behavior.
- Scope precedence inside the loader is project-over-user (v1.0 D-06); CC skills docs say personal-over-project. Existing locked loader decision — out of scope here.
**Warning signs:** an executor "aligning with CC docs" and breaking a locked decision.

### Pitfall 10: local_command lines ride the REDACTED path; the echo is user content
**What goes wrong:** writing class-B output via the unredacted seam (raw_thinking's path) leaks content that should be redacted; conversely redacting the typed command when the operator wants verbatim (D-22: args verbatim).
**How to avoid:** AppendLocalCommand already routes through the standard redacted append (manager.go:362); the echo's verbatim-vs-redacted formatting is planner discretion — pick one, document.
**Warning signs:** redactor tests failing on class-B paths.

## Code Examples

### Builtin table entry + class-A /init

```go
// Source: synthesis over expandUserBlocks (runtime.go:395–438) + CMDS-02/03 + ecosys.Command
// /init is a class-A builtin: it enters the chain as a Command value so the
// EXISTING expansion seam expands it — zero seam changes (CMDS-03).
initCmd := ecosys.Command{
    Name:        "init",
    Description: "Analyze the codebase and create a CLAUDE.md guide",
    Body:        initPromptBody, // $ARGUMENTS-capable, per Expand semantics
    Path:        "builtin:init",
}
```

### Chain-winner advertisement (D-04 winners-only)

```go
// Source: schema $defs/AvailableCommand (fetched 2026-08-28) + acp NotifyConfigOptions pattern (server.go:299–317)
cmds := make([]acp.AvailableCommandFrame, 0, chain.len())
for _, e := range chain.sorted() { // deterministic order
    cmds = append(cmds, acp.AvailableCommandFrame{
        Name: e.name, Description: e.desc,
        Input: &acp.AvailableCommandInputFrame{Hint: e.hint}, // omit when empty
    })
}
_ = srv.NotifyAvailableCommands(sessionID, cmds) // FULL set every time
```

### D-13 precedence + D-15 cross-provider dispatch

```go
// Source: subagent.go:273–288 (subagentProfile) + runtime.go:1311–1331 (resolveSubagentModel)
// + resolver.go:101–122 (buildTarget) + factory.go:142–171 (BuildWithCapturer) — all read this session
func (r *Runner) resolveDispatchModel(agentDef *ecosys.Agent, sess *session.Session, now time.Time) (string, provider.Provider) {
    slug := ""
    if agentDef != nil && agentDef.Model != "" && agentDef.Model != "inherit" { // D-14
        slug = agentDef.Model // D-13: frontmatter FIRST
    }
    if slug == "" {
        return "", nil // dispatch/session model — subagentProfile keeps parent prof.Model as-is
    }
    target, err := modelrouting.NewResolver(r.schedCfg).ResolveBySlug(slug, now) // buildTarget-based
    if err != nil { // unknown slug → D-15 degrade + ONE loud warning + counter
        warnOnce(sess, "agent model %q not declared — parent model used", slug)
        return "", nil
    }
    if target.Provider == r.providerName {
        return target.Model, nil // same wire: subagentProfile stamps prof.Model
    }
    p, err := r.providerFor(target.Provider) // ProviderFactory.BuildWithCapturer, cached per session
    if err != nil { // uncredentialed (lazy noCredentialProvider resolves here) → degrade loudly
        warnOnce(sess, "provider %q unreachable for agent model %q — parent model used", target.Provider, slug)
        return "", nil
    }
    return target.Model, p // D-15: cross-provider ROUTES — a turn never fails over routing
}
```

### fsnotify watch + debounce skeleton

```go
// Source: fsnotify README guidance (fetched 2026-08-28) + D-10/D-12
w, err := fsnotify.NewWatcher()
if err != nil { degradeToInvokeTimeOnly("watcher create: %v", err); return }
for _, dir := range watchSet(workDir) { // existing roots + leaves; D-10 discretion
    if err := w.Add(dir); err != nil { degradeOne(dir, err) } // D-12: one loud warning, continue
}
go func() {
    defer w.Close()
    t := time.NewTimer(0); var pending bool // 100–500ms debounce (discretion)
    for {
        select {
        case ev, ok := <-w.Events:
            if !ok { return }
            if ev.Has(fsnotify.Chmod) { continue }      // README: ignore Chmod
            if ev.Has(fsnotify.Create) { maybeAddNewLeaf(w, ev.Name) } // Pitfall 2
            if !pending { pending = true; t.Reset(debounceWindow) }
        case err, ok := <-w.Errors:
            if !ok { return }
            degradeLoud(err) // D-12
        case <-t.C:
            if pending { pending = false; rescanAndSwap(); fireAdvertisement() }
        case <-serveCtx.Done(): return
        }
    }
}()
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| CC: `.claude/commands/*.md` custom commands as their own feature | Commands MERGED into skills — a command file and `skills/<name>/SKILL.md` both create `/<name>`; "if a skill and a command share the same name, the skill takes precedence" | Current CC docs (fetched 2026-08-28) | Ass-guard's chain order (skills before file-commands, D-02) MATCHES CC's precedence — no change needed; keep both discovery paths |
| CC builtins as an undifferentiated command list | Docs now split "built-in commands (execute fixed logic directly)" from "bundled skills (prompt-based)" | Current CC docs | Validates the class-B/class-A split; /doctor is prompt-based in CC but locked class-B here (Pitfall 9) |
| 14-05: subagents unconditionally on tiers.light | D-13: frontmatter > dispatch > session > tier; unset = PARENT model | This phase (locked) | Token economics now explicit via frontmatter; `resolveSubagentModel`'s light-tier default is reversed |
| 14-05/16-05: cross-provider model targets skip with a warning | D-15: cross-provider frontmatter models actually ROUTE via a second factory-built provider | This phase (locked) | New second-provider lifecycle surface; Phase 22 builds on it |
| v1.1: no command advertisement at all | `available_commands_update` full-replacement advertisement + live re-fire | ACP v1 (current) | Zed's slash menu becomes ass-guard-driven |

**Deprecated/outdated (do not build on):** `user_message` as an update kind (doesn't exist — `user_message_chunk`); CC's "commands vs skills" as separate features (merged upstream; ass-guard keeps both discovery paths per CC compat, chain decides); polling-based fsnotify fallback (upstream #9 unimplemented).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | At least some configured providers (Z.ai/GLM, Anthropic, MiniMax, OpenRouter) expose a queryable usage/billing API usable for D-07's live /cost leg | Pattern 5, Open Questions 1 | /cost always uses the transcript fallback — acceptable per D-07 (it mandates the fallback), but the operator's "must be used when available" directive goes unmet until per-provider endpoints are verified at execution time |
| A2 | Zed renders `available_commands_update` entries as slash autocomplete with `input.hint` shown pre-argument | Wire Vocabulary | If Zed's rendering differs, criterion 1's "typing / autocompletes" still holds (protocol-conformant), but UX details differ — covered by a live-Zed operator checkpoint like Phase 16's |
| A3 | fsnotify v1.10.1 API (`NewWatcher/Add/Events/Errors/Has/Chmod`) as used in the skeleton | Code Examples | API drift would be a compile-time (not runtime) failure — trivially caught |
| A4 | BMad-style installers write `.claude/agents/*.md` (SKLS-02's own letter) and need no BMad-specific parsing beyond what `discoverAgents` already does | SKLS-02 row | None — the loader already walks exactly that layout [VERIFIED: loader.go:185–224] |
| A5 | Phase 16–19 plans land before Phase 20 executes (Phase 16 plans 07–09 and all of 17–19 are planned/pending per STATE.md) so `/compact` (19-D-11), the picker (18-D-10), and gate machinery (17) exist to delegate to | Summary | If ordering shifts, the delegating handlers stub behind seams — no structural risk, but criterion 3's /compact leg depends on Phase 19 |

## Open Questions

1. **Which providers expose live usage/billing endpoints for /cost (D-07)?**
   - What we know: D-07 locks the ORDER (endpoint live when one exists → transcript × cost table fallback) and the timeout class; the fallback is fully implementable today from `Target.Pricing` + transcript usage lines.
   - What's unclear: per-provider endpoint availability/shape (Z.ai, Anthropic, MiniMax, OpenRouter) — not verifiable without operator credentials.
   - Recommendation: plan the endpoint fetch as a per-provider capability seam (config-declared "none" default); executor verifies against the operator's actual providers behind a checkpoint if none is found. Never blocks the class-B budget (fallback + source note).
2. **Watcher owner: acpserve (composition) or runtime?**
   - What we know: the watch lifecycle ties to the serve ctx (CONTEXT integration point); the chain + rescan consumer is the Runner; the advertisement emitter lives on the acp server.
   - What's unclear: which package owns the goroutine.
   - Recommendation: runtime owns the chain + rescan (it owns `Discover` and resolution); acpserve owns the wire (watches the runner's rescan-complete callback → `NotifyAvailableCommands`). Keeps 15-D-20 layering.
3. **Does `/compact` accept focus-instruction args (CC parity) or ignore them?**
   - What we know: CC's /compact takes optional focus instructions; 19-D-11 locks same-machinery-immediate but says nothing about args.
   - Recommendation: pass typed args through to the Phase 19 summarizer entry if its signature admits them; otherwise record args in local_command (verbatim, D-22) and note "focus instructions not yet supported" in the output — planner picks with Phase 19's plan in hand.
4. **Should `user-invocable: false` skills be hidden from the advertisement (CC hides them from the `/` menu)?**
   - What we know: CC: `user-invocable: false` → "hides it from the `/` menu and doesn't run it when you type /name" [CITED: code.claude.com/docs/en/skills]; ass-guard's `Skill` type does not parse that field today.
   - Recommendation: parse the two relevant fields (`user-invocable`, `disable-model-invocation`) in the chain build and exclude `user-invocable: false` from the chain (it can never fire via slash — D-04 winners-only says absent); treat as a small, CC-parity-faithful addition. Planner confirms.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | everything | ✓ | go1.26.5 darwin/amd64 (go.mod pins 1.26) | — |
| golangci-lint | `mise ci` gate | ✓ (mise-managed v2) | v2 per .mise.toml | — |
| fsnotify | D-10 watches | ✓ via `go get` (module proxy reachable) | v1.10.1 | invoke-time-only rescan (D-12 locked fallback) |
| mise | CI gate | ✓ (repo-standard) | — | raw `go vet && go test -race ./...` |
| Live Zed | criterion 1/2 UX confirmation | ✓ (operator environment, per Phases 15/16 precedent) | — | internal/acpserve simulator E2E (16-06 pattern) |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none (fsnotify is a `go get` away; its own absence degrades per D-12 by design).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `stretchr/testify` v1.11.1 (repo standard) |
| Config file | none (convention: `_test.go` beside subject; gate via `.mise.toml`) |
| Quick run command | `go test -race -count=1 ./internal/runtime/ ./internal/acp/ ./internal/session/ ./internal/ecosys/` |
| Full suite command | `mise ci` (vet + lint + build + `go test -race -count=1 ./...`) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ACP-04 | `available_commands_update` frame carries full winner set; re-fires after rescan | unit (frame build) + E2E (simulator, 16-06 pattern) | `go test ./internal/acp/ -run TestNotifyAvailableCommands -x` ; `go test ./internal/acpserve/ -run TestSimulatorCommandAdvert -x` | ❌ Wave 0 (with tasks) |
| CMDS-01 | Chain precedence builtin > skill > agent > file; silent deterministic winners | unit | `go test ./internal/runtime/ -run TestCommandChain -x` | ❌ Wave 0 |
| CMDS-02 | Each class-B command: zero provider calls, `local_command` line written, echo + chunks + end_turn | unit (fake provider asserts 0 Streams) | `go test ./internal/runtime/ -run TestClassB -x` | ❌ Wave 0 |
| CMDS-03 | `/init` expands via existing seam; provenance line; engine-on parity | unit + existing e2e harness | `go test ./internal/runtime/ -run TestInitExpansion -x` | ❌ Wave 0 |
| CMDS-04 | File created/removed under discovery root → chain updated + advertisement re-fired; concurrent rescan × turn is `-race`-clean | unit + race test | `go test -race ./internal/runtime/ -run TestRescan -x` | ❌ Wave 0 |
| SKLS-01 | `/<skill-name>` expands SKILL.md body with args appended (Expand semantics) | unit | `go test ./internal/runtime/ -run TestSkillSlash -x` | ❌ Wave 0 |
| SKLS-02 | `/<agent-name>` dispatches subagent (prompt = args; Prompt/Tools apply; result streams) | unit (fake subagentRunner) | `go test ./internal/runtime/ -run TestAgentSlash -x` | ❌ Wave 0 |
| SKLS-03 | Precedence frontmatter > dispatch > session > tier; `inherit`/unset → parent; unknown slug → parent + ONE warning; cross-provider routes; resolvedModel on line + live note | unit | `go test ./internal/runtime/ ./internal/session/ -run TestDispatchModel -x` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -race -count=1 ./internal/runtime/ ./internal/acp/ ./internal/session/ ./internal/ecosys/`
- **Per wave merge:** `mise ci`
- **Phase gate:** full `mise ci` green before `/gsd:verify-work`; live-Zed operator checkpoint for autocomplete UX (A2), mirroring the Phase 15/16 WINDOWS pattern

### Wave 0 Gaps
- [ ] `internal/runtime/commands_test.go` — chain + class-B table fixtures (CMDS-01/02)
- [ ] `internal/runtime/rescan_test.go` — temp-dir watch fixtures + concurrent-swap race test (CMDS-04)
- [ ] `internal/acp/available_commands_test.go` — frame-shape test against schema-derived golden JSON (ACP-04, Pitfall 5)
- [ ] Existing suites cover the rest: `internal/runtime/e2e_opsx_test.go` (expansion seam), `internal/session/subagent_test.go` (dispatch), `internal/ecosys` loader tests (discovery)
- [ ] Framework install: none needed (stdlib + testify already required)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V5 Input Validation | yes | Discovered files are UNTRUSTED repo/user content: existing tolerant parsers + `commandNameRe` key whitelist (loader.go:392) + `pluginArtifactMaxBytes` read-cap precedent; expansion is pure text substitution — `Expand` never executes anything (expand.go:38) |
| V4 Access Control | yes | Class-B mutations (/model /config) ride 16-D-07 persist-then-apply with typed failures; /clear's boundary rides Projector discipline; no new authority surface |
| V7 Error Handling & Logging | yes | D-11/D-12: one structured warning per degradation, stderr only (stdout stays ACP frames); no stack traces to the wire |
| V8 Data Protection | yes | local_command lines ride the REDACTED append path (D-23 type-scoping keeps only raw_thinking exempt); /status//cost output must never include API keys (ACP-08: credentials env/file, never surfaced) |
| V2/V3/V6 | no | No auth/session-credential/crypto surface in this phase |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Malicious command/skill/agent file (prompt injection via body) | Tampering/Elevation | Accepted CC-parity surface: body enters conversation as prompt text; expansion never executes; AllowedTools/Tools frontmatter stays advisory (existing D-09 posture); plugin-artifact containment (`resolveInstallPath` symlink check) already guards cache paths |
| Watcher event storm / pathological registry (DoS) | Denial of Service | Debounce (100–500ms) + read-cap on file reads + rescan off the turn goroutine; D-12 degrade keeps sessions alive |
| Secret leakage through /status //cost//config output | Information Disclosure | Keys resolved only via `ResolveCredential` (never logged — WarnUncredentialed names the env var, not the key); command output builders must not interpolate credential values |
| Symlinked discovery dirs escaping watch roots | Tampering | Follow the `resolveInstallPath`/`samePath` EvalSymlinks precedent if watch-set resolution follows links; otherwise watch literal paths only |

## Sources

### Primary (HIGH confidence)
- zed-industries/agent-client-protocol `schema/v1/schema.json` (fetched 2026-08-28) — SessionUpdate union (11 kinds, verbatim), AvailableCommandsUpdate/AvailableCommand/AvailableCommandInput, UsageUpdate, SessionInfoUpdate, ContentChunk
- agentclientprotocol.com/protocol/v1/slash-commands — available_commands_update lifecycle, full replacement, invocation-as-prompt
- code.claude.com/docs/en/skills (fetched 2026-08-28) — commands-merged-into-skills, /skill-name invocation, builtins-fixed-logic, reserved names, watch/restart limitation, user-invocable/disable-model-invocation, $ARGUMENTS forms
- code.claude.com/docs/en/commands (fetched 2026-08-28) — every class-B command's CC behavior verbatim
- code.claude.com/docs/en/sub-agents (fetched 2026-08-28) — model frontmatter values, `inherit` + omitted-defaults-to-inherit, CC's own resolution order, `.claude/agents/` scopes and name rules
- github.com/fsnotify/fsnotify README (fetched 2026-08-28) — watch-parent guidance, non-recursion, platform table (NFS/SMB unsupported, polling "not yet implemented"), editor temp+rename, Chmod advice, Go 1.23+
- In-repo (all `Read` this session): internal/runtime/runtime.go (351–476, 539–566, 1046–1195, 1297–1331, 1463–1518), internal/session/subagent.go (17–341), internal/session/transcript.go (15–192), internal/session/manager.go (append surface), internal/ecosys/types.go + loader.go + expand.go + skills.go, internal/modelrouting/{config,resolver,factory,cost}.go, internal/acp/{server,handlers,emitter,types}.go, internal/runtime/enginebridge/enginebridge.go, go.mod, .mise.toml, .planning/{16,18,19}-CONTEXT.md, 16-RESEARCH.md, 20-CONTEXT.md, REQUIREMENTS.md, ROADMAP.md

### Secondary (MEDIUM confidence)
- gsd-tools classify-confidence seam rated the `webfetch` provider class LOW (static conservative rating) — superseded per-claim by the canonical-source fetches above, which meet the [VERIFIED] definition (tool-confirmed + authoritative origin)

### Tertiary (LOW confidence)
- None — no claim in this document rests on uncorroborated search results

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — one new dep (fsnotify), verified at proxy + official repo; everything else is stdlib/existing project packages read this session
- Architecture: HIGH — all seams verified by reading source; wire shapes verified from the canonical schema; the two corrections (no `user_message` kind; no existing command frame) are pinned, not assumed
- Pitfalls: HIGH — each is grounded in read code (aliasing at runtime.go:1190, one-shot Load contract at :351) or verified upstream behavior (fsnotify README, CC docs)

**Research date:** 2026-08-28
**Valid until:** 2026-09-27 (30 days — ACP v1 and CC docs are stable targets; re-verify fsnotify minor version if execution slips past a Go release cycle)
