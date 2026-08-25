# Architecture Research

**Domain:** v1.2 Claude Code Parity — integration of ten feature clusters onto the shipped ass-guard v1.1 architecture (Go binary, ~24 internal packages)
**Researched:** 2026-08-26
**Confidence:** HIGH for all codebase integration points (every named package/type/function verified against source in this repo); MEDIUM for exact ACP wire shapes (taken from the official spec pages at agentclientprotocol.com — primary-source fetched and cross-checked, but not yet validated against a live Zed handshake)

> This document covers ONLY how the v1.2 features integrate with the shipped architecture. The v1.1 architecture itself (event-bus spine, two-layer context, unified engine, stdio discipline, hook-DAG, scheduler) is treated as given. Every integration point names the real package, type, and function it attaches to, and marks **NEW** vs **MODIFIED** explicitly. A suggested build order that respects the dependencies is at the end.

---

## System Overview

All ten feature clusters attach to four layers. No v1.1 component is rewritten; the largest single concentration of change is `cmd/ass-guard/acp_serve.go`'s `sessionTurnRunner` (2,116 lines) — which is itself the argument for the early `internal/runtime` carve recommended in the build order.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ACP FRONTEND (internal/acp + cmd/ass-guard/acp_serve.go)                     │
│                                                                              │
│  ┌────────────────────┐  ┌──────────────────────────────────────────────┐    │
│  │ Server / handlers  │  │ EMITTERS + REQUESTERS [NEW capabilities]     │    │
│  │ [MODIFIED: session │  │ TurnEmitter: agent_message_chunk (today)     │    │
│  │ family, caps,      │  │   + tool_call / tool_call_update / plan      │    │
│  │ configOptions]     │  │   + agent_thought_chunk                      │    │
│  └────────────────────┘  │ Requester: request_permission, elicitation/  │    │
│                          │   create  (agent→client REQUESTS w/ id)      │    │
│                          └──────────────────────────────────────────────┘    │
├──────────────────────────────────────────────────────────────────────────────┤
│ RUNNER LAYER (cmd/ass-guard sessionTurnRunner → internal/runtime [CARVE])    │
│  ┌────────────────────┐  ┌───────────────────┐  ┌────────────────────────┐   │
│  │ invocation resolver│  │ session family ops│  │ editor-config override │   │
│  │ [MODIFIED: builtins│  │ [NEW: list/resume/│  │ [NEW: session-scope    │   │
│  │ → skills → agents] │  │  close/delete]    │  │  tier/model routing]   │   │
│  └────────────────────┘  └───────────────────┘  └────────────────────────┘   │
├──────────────────────────────────────────────────────────────────────────────┤
│ SESSION CORE (internal/session)                                              │
│  ┌───────────────┐ ┌────────────┐ ┌──────────────────────────────────────┐   │
│  │ turn loop     │ │ Projector  │ │ SEAMS [MODIFIED/NEW]:                │   │
│  │ [MODIFIED:    │ │ [MODIFIED: │ │ PermissionGate · SteerQueue ·        │   │
│  │ gates, steer, │ │ compaction │ │ Compactor · BackgroundSubagents ·    │   │
│  │ bg subagents] │ │  reset pt] │ │ TurnEmitter · AGENTS.md merge        │   │
│  └───────────────┘ └────────────┘ └──────────────────────────────────────┘   │
├──────────────────────────────────────────────────────────────────────────────┤
│ OBSERVER + ECOSYSTEM + PROVIDER                                              │
│  engine (post-turn) · ecosys discovery [MODIFIED: settings.json hooks,       │
│  memory files] · shaper [MODIFIED: cache_control, images, thinking] ·        │
│  provider [MODIFIED: thinking StreamChunk]                                   │
├──────────────────────────────────────────────────────────────────────────────┤
│ STORES: transcript_*.jsonl · shadow.git checkpoints · learned.yaml ·         │
│  schedule store · permissions.yaml [NEW]                                     │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities (v1.2 delta view)

| Component | Status | v1.2 Responsibility |
|-----------|--------|---------------------|
| `internal/acp` Server | MODIFIED | New handlers (`session/list`, `session/load`, `session/resume`, `session/close`, `session/delete`, `session/set_config_option`, `elicitation/create` responses); richer `initialize` capabilities (`loadSession:true`, `sessionCapabilities`, `configOptions`); generalized emitter beyond `AgentMessageChunk`; outbound request capability (agent→client JSON-RPC *requests* with an id — the server has only ever sent notifications) |
| `cmd/ass-guard` `sessionTurnRunner` | MODIFIED (+ carve) | Invocation resolver chain; session-family operations; permission/elicitation brokering; editor-config override plumbing; everything below that today lives in `Run`/`sessionFor` |
| `internal/session` Session | MODIFIED | Permission gate seam beside the plan-mode gate; steering queue drain in `runTurn`; compaction marker handling; background-subagent registry; turn-emitter publication of tool_call/plan/thought frames |
| `internal/session` Projector | MODIFIED | Treat a `compaction` transcript line as a second reset-point class (summary becomes the seed); otherwise untouched |
| `internal/ecosys` | MODIFIED | Parse `settings.json` hooks (project+user) in the loader; AGENTS.md/CLAUDE.md readers; skills/agents surfaced as invocable keys |
| `internal/shaper` / `internal/provider` | MODIFIED | `cache_control {"type":"ephemeral"}` on every system block (routed gap, docs/compaction-decision.md §3 row 1); `"thinking"` StreamChunk type; image content blocks |
| `internal/coreexec` | MODIFIED | Sandbox wrapper for Bash; background-task completion callbacks; persistent-shell option; uniform hook wrap coverage |
| `internal/checkpoint` | GIVEN (shipped v1.1 EARLY-01) | Shadow-git store, refs, CLI already exist; v1.2 adds only the surface/guard rails (see §9) |

---

## Recommended Project Structure (v1.2 deltas)

```
cmd/ass-guard/
├── acp_serve.go              # MODIFIED — thins over time as runtime carves out
├── acp_sessions.go           # NEW  — session list/resume/close/delete runner ops
├── acp_commands.go           # NEW  — builtin command table + local-render handlers
├── acp_config.go             # NEW  — editor configOptions ↔ routing override
├── checkpoint.go             # GIVEN — CLI surface (v1.1)
internal/runtime/             # NEW (carve) — sessionTurnRunner + engine/MCP wiring,
│                              #   moved mechanically from acp_serve.go; frontends
│                              #   (acp today, telegram v1.3) become clients
internal/acp/
├── server.go                 # MODIFIED — outbound-request writer path (id'd frames)
├── emitters.go               # NEW  — TurnEmitter (chunk/thought/tool_call/plan)
├── requesters.go             # NEW  — RequestPermission / ElicitationCreate flows
├── handlers_sessions.go      # NEW  — session family handlers
├── handlers_config.go        # NEW  — session/set_config_option
internal/session/
├── permission.go             # NEW  — PermissionGate (broker pattern, mirror of ask.go)
├── steering.go               # NEW  — SteerQueue + runTurn drain point
├── compaction.go             # NEW  — Compactor (summarize call + marker line)
├── bgsubagent.go             # NEW  — background dispatch registry + completion cb
internal/event/events.go      # MODIFIED — + AgentThoughtChunk, BackgroundTaskDone,
│                              #   + payload fields on existing ToolCall/ToolCallUpdate
internal/ecosys/
├── settings_hooks.go         # NEW  — settings.json hooks parsing (both scopes)
├── memory.go                 # NEW  — AGENTS.md/CLAUDE.md discovery + concat
```

### Structure rationale

- **New files, not new packages, inside `internal/session`:** every new seam follows the established file-per-seam convention (`ask.go`, `planmode.go`, `boundary.go`) — the Session struct gains fields wired via `Set*` accessors, exactly like `SetAskBroker`/`SetPlanMode`.
- **`internal/runtime` carve is mechanical, not architectural:** move `sessionTurnRunner`, `engineTurnRunnerAdapter`, `acpDispatcher`, the cron wiring helpers, and their constructors verbatim; zero behavior change; `cmd/ass-guard` composes it. This is the same extraction ROADMAP Phase-10 already sketched and SEED-001 names as the kit's down-payment.

---

## Architectural Patterns (the five that carry v1.2)

### Pattern 1: The broker/suspension pattern (reuse for permissions, elicitation, steering acks)

**What:** a per-session broker holds a pending interaction, fires a client-visible surface callback, and exposes a settle channel; the requesting seam suspends; a later prompt resolves it.
**When:** any agent→client round trip that must suspend model progress.
**Proven in-repo:** `internal/session/ask.go` (`AskBroker`: `Surface` → bus-published render → `ResolveAsk` resumes the suspended turn; `routeAskReply` in acp_serve.go routes the reply prompt; the D-01 timer bounds the wait; `AskSettleChan` lets the parked engine chain wait through suspension).

```go
// PermissionGate = AskBroker's twin, different payload + resolution channel.
type PermissionGate struct {
    mu      sync.Mutex
    pending map[string]chan PermissionOutcome // toolCallID -> resolution
    onAsk   func(PermissionRequest)            // publishes request_permission via acp
}
func (g *PermissionGate) Await(ctx context.Context, req PermissionRequest) (PermissionOutcome, error) {
    ch := make(chan PermissionOutcome, 1)
    g.register(req.ToolCallID, ch)
    g.onAsk(req) // acp layer writes the id'd request; response resolves via Resolve()
    select {
    case o := <-ch: return o, nil
    case <-ctx.Done(): return PermissionReject, ctx.Err() // cancel ⇒ reject_once semantics
    }
}
```

**Trade-offs:** identical suspension semantics to asks (turn stops consuming the loop, client controls latency); the known hazard — reply-vs-new-prompt ambiguity — is already solved by `HasPendingAsk` routing and must be extended: while a permission is pending, the *response frame* (not a prompt) resolves it, so no routing conflict arises.

### Pattern 2: Ordered inline emission, not multi-kind bus fan-out, for client-visible frames

**What:** client-visible session/update frames for a turn (text chunks, thought chunks, tool_call lifecycle, plan) are emitted **inline by the turn goroutine through one emitter object**, in causal order. The bus remains the async/audit spine (TranscriptWriter, engine, audit mirror).
**When:** any feature that adds a second client-visible frame type during streaming turns.
**Why not the bus for these:** the bus demultiplexes by kind into separate bounded channels (`BufAgentMessageChunk=128`, `BufToolCall=16`, …). One subscriber goroutine selecting over N channels gives **no cross-kind ordering guarantee** — a `tool_call_update` can overtake the `tool_call` that created it, or a chunk can interleave between them, and ACP clients build UI state off these transitions. The existing single-kind forwarding worked precisely because there was only one kind.

```go
// internal/acp/emitters.go — replaces/augments the ChunkEmitter seam.
type TurnEmitter interface {
    AgentMessageChunk(messageID, text string) error          // exists today
    AgentThoughtChunk(messageID, text string) error          // NEW (thinking)
    ToolCallCreated(tc ToolCallFrame) error                  // NEW
    ToolCallUpdated(update ToolCallUpdateFrame) error        // NEW
    Plan(entries []PlanEntry) error                          // NEW (TodoWrite mirror)
}
// session.runTurn calls these at the SAME sites where it appends transcript
// lines (record-tool_call site → ToolCallCreated; result-append site →
// ToolCallUpdated completed/failed) — transcript and wire stay causally aligned.
```

Server-driven turns (cron firings, parked-chain injections) obtain the same interface from `srv.Emitter(sessionID)` — the `startSessionForwarder` mute-flag dance (`turnActive`) extends to the new frame kinds unchanged.

### Pattern 3: Dynamic merge into the profile copy (the injection vehicle)

**What:** anything the model must see that is not part of the static capture (runtime cwd, skills listing, agent listing, hook stdout, and now AGENTS.md/CLAUDE.md) is appended as trailing `System` TextBlocks on the **per-session profile copy** assembled in `sessionFor` — never mutating the shared `r.profile`.
**When:** every context-injection feature.
**Existing sites:** `ComposeRuntimeWorkDir`, `ecosys.SkillListing`, `ecosys.AgentListing`, `injectHookContext` — all copy-on-append.
**Known documented nuisance:** the capture places some of this as system-role messages *inside* `request.messages`, while the Shaper supports only leading system-param blocks; the trailing-System-block form is the accepted divergence pending re-capture evidence. v1.2 additions (memory files) reuse the same vehicle rather than inventing a second one.

### Pattern 4: Optional-capability interface type-assertion at the server boundary

**What:** the ACP server discovers runner capabilities by type-asserting the injected `TurnRunner` to optional interfaces (`SessionCloser` exists today).
**When:** every new session-family operation.
**Extension:** `SessionLister`, `SessionLoader`, `SessionEraser`, `ConfigOptionsProvider` — each a small interface in `internal/acp`, asserted in the corresponding handler; a stub runner simply doesn't implement them and the handler degrades (mirroring `closeSessionIfPossible`). This keeps `internal/acp` free of `internal/session` imports.

### Pattern 5: Executor-only override on the captured catalog

**What:** captured tool decls (schema/description) are never rewritten; behavior attaches by overriding only `Execute` on the per-session catalog clone.
**When:** new interactive/control tools.
**Existing sites:** `Skill` override (`ecosys.SkillExecute`), `RegisterAsk`, `RegisterInteractive`. v1.2's `/commands` do **not** become tools (see Anti-Pattern 2); but the *Undo* ACP command and any future checkpoint tool would ride this pattern.

---

## Data Flow (the six flows v1.2 adds or reroutes)

### Flow 1: request_permission (the amended safety model)

```
model returns tool_use batch (session.runTurn)
    ↓ pre-batch classification loop (where planModeBlocks sits today)
PermissionGate.Allowed?(name, input)                    ← policy: ungated default;
    ↓ gated?                                               opt-in via config/editor
    ├─ publish ToolCallCreated{status: pending}            (TurnEmitter, inline)
    ├─ Gate.Await → acp.Requesters.RequestPermission       (id'd agent→client request:
    │     {sessionId, toolCall:{toolCallId}, options:[allow_once|allow_always|
    │      reject_once|reject_always]})
    ├─ client responds outcome selected(optionId)|cancelled
    ├─ allow_always → persist to .ass-guard/permissions.yaml (project scope,
    │  session-scope memory checked first) — the Learning-store precedent
    ├─ reject / cancelled → structured isError tool_result (errHookRefused form),
    │  no execution                                        ← same shape the
    └─ allow → proceed into DispatchBatch as today          PreToolUse denial uses
```

**Landing sites.**
- **Intercept point:** `internal/session/session.go` `runTurn`, the pre-batch loop that already special-cases subagent tools and the plan-mode gate — a third branch calling the gate. This is *above* `DispatchBatch` so read-only concurrent calls are classified before batching (a blocked call never enters the batch), and *beside* `PreToolUse` hooks, which continue to run at the executor chokepoint (`coreexec.withHooks`) — permission is consent, hooks are policy; both can refuse.
- **Wire mechanics (NEW in `internal/acp`):** the server today only writes notifications and responses. `request_permission` and `elicitation/create` are **outbound requests** needing an id, a pending-response registry, and correlation of the inbound response frame. The framer already delivers arbitrary inbound requests to `handlers` — add a `pendingRequests map[json.RawMessage]chan json.RawMessage` guarded by the writer mutex's sibling.
- **Capability advertisement:** `initialize` gains `agentCapabilities.requestPermission` (and meta toggles); Zed decides clickable UX from this.
- **Safety-model amendment, made explicit:** PROJECT.md Out-of-Scope still says "no confirmation/permission tier." v1.2 amends this per the milestone's own feature list. Architecture stance: the tier is **available but not default** — `permissions.mode: ungated` (v1.1 behavior, structural default) | `gated` (policy-table-driven asks) — switchable per session via editor configOptions. Nothing forces a prompt on the hands-off OpenSpec workflow.

### Flow 2: tool_call / plan / thought streaming during a turn

```
streamAndEmit chunk loop ─ text → AgentMessageChunk (existing) ─────────────┐
                        ├─ thinking delta → AgentThoughtChunk [NEW event]   ├── inline
                        └─ tool_use  → (existing ToolCall event for audit)   │   TurnEmitter
runTurn tool-call sites                                                     │
  ├─ AppendToolCall site      → ToolCallCreated{id,title,kind,status:pending}┤
  ├─ result-append site       → ToolCallUpdated{status:completed|failed,     │
  │                             content:[output excerpt], locations:[paths]} │
  └─ TodoWrite executor site  → Plan(snapshot of TodoStore)  ────────────────┘
```

- **Title/kind derivation** is a pure function over `(toolName, input)` — reuse `projector.extractFilePaths` for `locations`; kind table: Read/Glob/Grep→`read`, Write/Edit→`edit`, Bash→`execute`, WebFetch/WebSearch→`fetch`, Task/Agent→`other`, AskUserQuestion→`think`. Lives in `internal/coreexec` or `internal/toolcat` (catalog metadata is the natural home).
- **Event vocabulary:** add `event.AgentThoughtChunk` (mirror of `AgentMessageChunk`, buffer 128) so TranscriptWriter persists thinking for replay; extend `event.ToolCall`/`ToolCallUpdate` payloads with the ACP-facing fields rather than adding a third kind.
- **Transcript:** thinking text must land in the durable transcript (replay-on-load re-streams it); either a new `agent_thought_chunk` line type or a marked segment of `assistant_message`. Prefer the dedicated line type — `LastTurnOutput`'s backward scan ignores unknown-to-it line types safely.
- **Plan variant:** the TodoStore snapshot is already centralized (`TodoWriteExecute` → `Swap`); publishing the plan frame at that one site gives CC-parity todo panels with no projector changes.

### Flow 3: compaction (summarize-then-marker; window semantics preserved)

```
trigger: cumulative UsageUpdate tokens ≥ threshold (context limit from
         modelrouting capability metadata) OR manual /compact
    ↓
Compactor.Run: provider call (light tier) over Manager.ReadAll() lines
    ↓
Manager.AppendCompaction(summary, coversThroughTurn)   ← NEW line type "compaction"
    ↓
Projector.Project: latest compaction line is a HARD reset point —
    seed := summary-message + post-compaction history (existing between-turn
    rules apply within that span); mid-turn accumulation untouched (64-tail,
    pair-safe cut unchanged)
    ↓
replay-on-load: unaffected — the summary lives IN the durable transcript;
    session/load re-streams full history from lines regardless of what the
    Projector sends to the model (two independent consumers of one artifact)
```

**Why this shape.** The two-layer design already separates "what the model sees" (Projector) from "what happened" (Manager). Compaction is purely a Projector-seed policy plus one new durable line type — it does **not** truncate or rewrite the transcript (audit-artifact invariant D-20 survives intact), and resume works because reconstruction reads lines, not projections. The verify-first mandate from SEED-004 is already discharged: `docs/compaction-decision.md` proves the zcode corpus contains **no auto-compact** — the target manages context via the rolling 64-tail (which we already replicate) plus `cache_control` on every system block (the routed gap). Consequence for v1.2: ship **cache_control emission alongside** compaction (Shaper maps a per-TextBlock flag onto `anthropic.TextBlockParam.CacheControl`; profile TextBlock gains the captured value) — it is the parity-faithful lever, cheap, and TIER-1-scoped per the decision doc. Auto-compact threshold defaults conservative (e.g. 80% of the model's context window; `/compact [instructions]` always available); the summarizer rides the light tier like subagents.

### Flow 4: session list / resume (load) / close / delete

```
session/list  → SessionLister.List(cursor,cwd): scan <workdir>/.ass-guard/
                transcript_*.jsonl headers (session_start + first user_message
                for title, last line ts for updatedAt) → SessionInfo[]
session/load  → SessionLoader.Load(sessionId, cwd, mcpServers):
                1. Manager opens the EXISTING transcript (openTranscript already
                   O_APPEND — resume is append-native)
                2. sessionFor builds the Session normally (Projector replays from
                   lines; no model call)
                3. REPLAY: walk transcript lines → emit user_message_chunk /
                   agent_thought_chunk / agent_message_chunk / tool_call /
                   tool_call_update frames via TurnEmitter (reconstruction_test.go
                   is the existing proof this data suffices)
                4. respond with configOptions/modes
session/close → cancel parked chains + cancelTurn + s.Close() (all exist via
                CloseSession/cancelParkedChains) + drop sessionState
session/delete→ close + archive (rename transcript to .deleted/) — NEVER rm
                (transcript = the investigate-and-fix artifact, D-20)
```

- **Identity:** Zed issues its own sessionId strings; the runner keys sessions by whatever id the client presents (`sessions map[string]*session.Session` already does). Resume therefore needs **an id-alias map** (presented id ↔ stored transcript id) or, simpler, `session/list` returns the *stored* ids and `session/load` accepts them — the transcript filename is `transcript_<sessionID>.jsonl`, so the presented id must equal the stored one; alias map handles the general case.
- **Capabilities:** `initialize` flips to `loadSession:true` + `sessionCapabilities{list,resume,close,delete}`; D-09's `-32601` stub is replaced by the real handler (the reversal is already logged as an operator decision).
- **Engine state on resume:** parked chains are process-local; a resumed session starts clean (chains ended when the process died). The transcript's `engine_decision` lines preserve the audit trail; `WaitChainIdle` simply finds zero chains.

### Flow 5: invocation resolution chain + available_commands_update

```
prompt "/key args"
    ↓ invocationFor [MODIFIED — resolver chain, first match wins]
    1. builtins table (acp_commands.go): /model /config /compact /clear /cost
       /resume /memory /mcp /permissions /doctor /status /help /init
    2. reg.Skills  → SKILL.md body = prompt, args appended (Expand semantics)
    3. reg.Agents  → BMad-style: agent body = prompt (per-agent model: rides
       dispatch, see Flow 6)
    4. reg.Commands (today's only lookup — unchanged last position)
    ↓ class A: prompt-expanding (skills, agents, /init, /memory)
        expandUserBlocks path UNCHANGED: body replaces block text,
        AppendCommandProvenance(key,path,args) written, mutating-boundary rule
        applies ⇒ YES, recorded as ordinary user turns (the model sees exactly
        what it sees for /opsx:* today; replay shows the expanded body +
        provenance metadata)
    ↓ class B: control-plane (/model /cost /status /help /doctor /mcp
       /permissions /config /compact /clear /resume)
        NO model turn. Handler executes at the runner seam and returns;
        response rendered locally via TurnEmitter.AgentMessageChunk.
        Audit: NEW transcript line "local_command" {key, args} — the typed
        command IS metadata (provenance precedent), the rendered output is a
        client-visible note, never an assistant_message (the model never said it).
available_commands_update [NEW emitter call]
    fired: after session/new response, after session/load, after any registry
    reload; payload = builtins ∪ reg.Commands ∪ skills ∪ agents
    ({name, description}); Zed autocompletes from this single list.
```

- **Registry freshness:** `loadCommandRegistry` loads once at startup; v1.2 keeps that (a per-turn rediscovery race is not worth it) but emits the update once per session so late-started sessions still see the list.
- **`/model` and `/config`** mutate the same session-scope routing override as editor configOptions (one seam): precedence **editor/UI override > time-window > project > global** — an explicit human choice outranks automation, consistent with D-02's spirit (window narrows project narrows global; the UI is narrower still).
- **`/cost`** aggregates `TypeUsage` lines already written by TranscriptWriter. **`/mcp`** reads live host state — `sessionFor` closes over `mcpHost`; expose an accessor on the runner. **`/clear`** = `AppendBoundary("command:/clear")` (next projection seeds empty) + client-visible note. **`/compact`** invokes the Compactor (Flow 3). **`/resume`** returns the session list through the same machinery as `session/list`.

### Flow 6: full subagents / background agents / per-agent model

```
Task/Agent call with run_in_background=true        [MODIFIED session/subagent.go]
    ├─ BackgroundDispatch: register in per-session SubagentRegistry
    │    {subagentTurnID, cancel, toolCallID}  (mirror of coreexec.TaskRegistry)
    ├─ return immediately: tool_result = task id (TaskOutput-pollable — the
    │    registry + TaskOutput tool already exist for Bash tasks)
    ├─ completion → event.SubagentResult (EXISTS) + NEW event.BackgroundTaskDone
    │    → adapter emits ToolCallUpdated{toolCallId: <dispatch call>,
    │      status:completed, content:[result]} — the notification path rides the
    │    SAME tool_call frame the client already renders, no new UI concept
    └─ cancellation: registry cancels feed CloseSession/cancelParkedChains
       (foreground path unchanged: DispatchSubagent select on resCh/ctx)

per-agent model:  subagentProfile [MODIFIED]
    def.Model != "" → resolve BEFORE the light-tier default:
      tier-name match ("heavy"/"good"/"light") → modelrouting resolver for that tier
      else treat as literal slug on the session's provider (buildTarget);
      unresolvable → loud stderr warn + fall back to today's light-tier rule.
    Precedence: agent frontmatter > Session.SubagentModel (light tier) > parent.
```

Foreground subagent chunk leakage note: `defaultSubagentRunner` publishes chunks tagged with the *subagent* turn id; the client forwarder's prefix filter admits them (same session prefix). With tool_call streaming this becomes visible noise — filter subagent-tagged chunks out of the parent's stream (they are interior work; CC does not stream subagent internals to the main transcript either) while keeping `SubagentResult` notifications.

---

## Integration Points (feature-by-feature index)

| # | Feature (milestone wording) | Primary landing | NEW / MODIFIED | Depends on |
|---|---|---|---|---|
| 1 | session/request_permission | `session.runTurn` pre-batch loop; `internal/acp` outbound-request writer; `PermissionGate` (new, `session/permission.go`); `permissions.yaml` store | NEW seam + MODIFIED loop | outbound-request mechanics |
| 2 | elicitation/create | `internal/acp/requesters.go`; upgrade paths: engine `ActionAsk` note → structured form; `AskUserQuestion` renders via elicitation when client advertises support, AskBroker text fallback otherwise | NEW | outbound-request mechanics |
| 3 | tool_call + plan streaming | `TurnEmitter` (new `internal/acp/emitters.go`); `runTurn` record/result sites; `TodoWriteExecute` plan publish; `event.ToolCall(+Update)` payload growth | NEW emitter + MODIFIED loop/sites | — |
| 4 | available_commands_update | emitter + resolver chain (Flow 5) | NEW | #3 emitters |
| 5 | session list/resume/close/delete | `internal/acp/handlers_sessions.go`; `SessionLister/Loader` capability interfaces; alias map in runner; `handlers.go` capabilities flip | NEW handlers + MODIFIED initialize | #3 (replay frames) |
| 6 | editor configOptions | `initialize`/`session/new` response fields; `session/set_config_option` handler; session-scope routing override consulted in `sessionFor`/`resolveSubagentModel` | NEW + MODIFIED resolver precedence | — |
| 7 | built-in chat commands | `cmd/ass-guard/acp_commands.go` table + class-B handlers; `invocationFor` chain | NEW + MODIFIED seam | #4, /compact→#8, /resume→#5, /model→#6 |
| 8 | compaction (+ cache_control) | `session/compaction.go`; `Manager.AppendCompaction`; `Projector` reset-point class; `shaper` cache_control mapping; profile TextBlock field | NEW + MODIFIED projector/shaper | usage accounting (exists) |
| 9 | slash-invocable skills + AGENTS-as-commands + per-agent model | resolver chain positions 2–3; `subagentProfile` model resolution | MODIFIED `invocationFor`, `subagentProfile` | #4 (autocomplete) |
| 10 | hooks PreToolUse deny (full lifecycle) | `ecosys` loader parses `settings.json` hooks (project + user scopes — today plugin-bundle `hooks/hooks.json` only); widen `withHooks` wrap to the `MCPExecutor` boundary so MCP tools are gated too; deny already returns structured refusal | MODIFIED loader + wrap site | — |
| 11 | AGENTS.md/CLAUDE.md injection | `sessionFor` profile-copy assembly (Pattern 3), fed by `ecosys/memory.go` (project root + `.claude/CLAUDE.md` + user `~/.claude/CLAUDE.md`, project wins; mtime-cached per serve) | NEW reader + one merge site | — |
| 12 | thinking blocks | `provider` `"thinking"` StreamChunk (anthropic `thinking_delta`; OpenAI-shape `reasoning_content` when present, silent absence otherwise); `event.AgentThoughtChunk`; emitter + TranscriptWriter line | MODIFIED provider/streaming + NEW event | #3 |
| 13 | rich prompt content (@-mentions, images) | `acp.ContentBlock`/`session.ContentBlock` gain image/resource variants; `expandUserBlocks` grows an @-mention expander (inline-read + cap, riding `boundedToolResult` limits); shaper maps images (anthropic native; OpenAI `image_url`) | MODIFIED types + expander + shaper | — |
| 14 | background Bash + persistent shell + sandbox | `coreexec/background.go` completion callback → `BackgroundTaskDone` event → emitter; persistent shell = per-session shell process owned by bash executor config; sandbox = `sandbox-exec`(seatbelt)/`bwrap` wrapper around `exec.CommandContext` honoring the existing `dangerouslyDisableSandbox` input for real | MODIFIED coreexec/bash+background | #3 (notifications) |
| 15 | steering queue | `session/steering.go`: per-session buffered chan; `session/prompt` handler enqueues when `turnActive`; `runTurn` drains at each iteration top → steered blocks appended as turn-tagged user_messages (projector picks them up naturally); parked-chain resumes drain identically | NEW | — |
| 16 | shadow-git checkpoints | store + CLI **exist** (EARLY-01 shipped: `internal/checkpoint`, `SnapshotTurn` at parent-turn entry, refs/checkpoints/<sid>-turn-NNN, keep-50, 0700). v1.2 remainder: restore guard (refuse restore into a session with active turn/chains — `chainCount`+`turnActive` checks), optional `/undo` class-B command surfacing the CLI, eval coverage | GIVEN + small NEW surface | — |
| 17 | SEED-001 kit extraction | see Build Order; library surface over `profile/shaper/provider/sched/toolcat/toolexec/engine/hookdag/event/session/redact/checkpoint/audit/mcp`; app-retained: `ecosys`, `openspec`, `coreexec`, `learning`, `firstrun`, `evalsuite`, `parity/drift`, `cmd/`, `acp` (frontend) | NEW `pkg/` (or `kit/`) API layer | runtime carve |

---

## Anti-Patterns

### Anti-Pattern 1: Fanning client frames out through multiple bus subscriptions
**What people do:** subscribe the forwarder to `AgentMessageChunk` + `ToolCall` + new kinds, select over channels.
**Why wrong:** per-kind channels destroy cross-kind ordering; clients render `tool_call_update` for a call they never saw created.
**Instead:** Pattern 2 — inline ordered `TurnEmitter` from the turn goroutine; bus stays the audit/async spine.

### Anti-Pattern 2: Making built-in commands model turns
**What people do:** implement `/cost` by expanding a prompt that instructs the model to compute cost.
**Why wrong:** burns tokens, adds latency and hallucination surface for deterministic answers, pollutes the transcript with fake Q/A.
**Instead:** class-B handlers execute at the runner seam; results are client-visible notes, never `assistant_message` lines.

### Anti-Pattern 3: Truncating or rewriting the transcript for compaction
**What people do:** delete/replace old transcript lines when compacting (the CC mental model imported literally).
**Why wrong:** breaks the D-20 one-audit-artifact invariant, breaks replay-on-load, loses investigate-and-fix material.
**Instead:** compaction is additive (marker line) and lives entirely in the Projector's seed policy.

### Anti-Pattern 4: Putting permission logic inside tool executors
**What people do:** teach each core executor to consult the permission gate.
**Why wrong:** executors are per-tool; MCP and future tools leak past the gate; the batch classifier is the single chokepoint that already exists.
**Instead:** one gate check in the pre-batch classification loop; hooks remain the executor-level policy layer; the two refusals compose.

### Anti-Pattern 5: `rm`ing transcripts on session/delete
**What people do:** delete means delete.
**Why wrong:** the transcript is the primary diagnostic surface (PROJECT.md constraint); deletion is unrecoverable forensics loss.
**Instead:** rename to `.deleted/` (or tombstone line + exclude from `session/list`).

### Anti-Pattern 6: Building v1.2 features deeper into `cmd/ass-guard/acp_serve.go`
**What people do:** keep appending to the 2,100-line composition file because "the kit extraction comes last."
**Why wrong:** seven of ten clusters modify exactly this code; deferring the carve means every feature lands twice (write in `cmd/`, migrate in the kit phase).
**Instead:** front-load the mechanical `internal/runtime` carve (build order step 0); features then land in their final home.

---

## Integration Points — External Services & Internal Boundaries

| Boundary | Communication | v1.2 notes |
|----------|---------------|------------|
| Zed ⇄ agent (stdio JSON-RPC) | ACP v1 frames | New: outbound id'd requests (`request_permission`, `elicitation/create`) require a pending-response registry beside the mutex-guarded Writer; string ids already handled (RawMessage verbatim echo) |
| agent ⇄ provider APIs | SSE streams | `"thinking"` chunk type; image blocks; `cache_control` on system blocks (Anthropic-shape only — OpenAI-shape silently omits, matching today's shape-isolation) |
| session core ⇄ runner | direct calls + `Set*` seams | All new seams follow the ask-broker/accessor convention; `internal/session` must not import `internal/acp` (emitter/requester interfaces defined session-side, implemented acp-side — the `OnClose` func-seam precedent) |
| runner ⇄ stores | files under `.ass-guard/` | `permissions.yaml` joins `learned.yaml`/schedule store; transcripts remain sole-owner Manager writes |
| kit ⇄ app (post-extraction) | exported constructors + interfaces | Public API = composition root options; invariants (stdout discipline, `.claude/` read-only, static binary) enforced app-side, exposed as kit choices |

## Scaling Considerations

| Concern | Now (single user, one editor) | v1.2 stress | Adjustment |
|---------|------------------------------|-------------|------------|
| `session/list` cost | trivial (few transcripts) | hundreds of long sessions | header-scan only (first/last lines), cursor pagination per spec; never `ReadAll` per listing |
| Replay size on resume | short histories | 18–40 MB transcripts exist in the wild (corpus evidence) | replay streams from lines with backpressure via the Writer; cap initial replay (spec permits truncated replay; full fidelity lives on disk) |
| Compaction summarizer | rare | every long session | light-tier route (subagent precedent); summarizer output capped; compaction is O(transcript) once per threshold crossing, not per turn |
| Frame volume with tool_call streaming | one kind | 4–5 kinds per busy turn | inline emission shares the single Writer mutex (safe interleaving, no new locks); buffers unchanged since bus no longer carries client traffic |
| Permissions store contention | n/a | concurrent sessions allow_always | whole-file rewrite under mutex (Learning-store pattern); project file is small |

---

## Build Order (dependency-respecting)

**Step 0 — `internal/runtime` carve (recommended phase 0, mechanical).** Move `sessionTurnRunner` + adapters + cron wiring out of `cmd/ass-guard/acp_serve.go` verbatim. Everything in waves 1–3 modifies this code; carving first means every feature lands in its final home and the SEED-001 phase inherits a clean seam instead of performing archaeology. Cost: a move, not a redesign (zero behavior change, `mise ci` proves it). If the operator rejects the refactor-up-front discipline, fall back to feature-first and accept the double migration — but then budget the kit phase accordingly.

**Wave 1 — ACP foundation (unblocks nearly everything):**
1. Outbound-request mechanics in `internal/acp` (writer + pending-response registry) — prerequisite for permissions *and* elicitation.
2. `TurnEmitter` (tool_call/tool_call_update/plan/thought) + event payload growth + TranscriptWriter line types.
3. `initialize` capabilities overhaul + `session/new`/`load`/`resume` response fields.
4. `request_permission` end-to-end (gate seam, policy config, permissions.yaml) — the safety-model amendment lands here, default ungated.
5. `elicitation/create` (+ AskUserQuestion upgrade path with AskBroker fallback).

**Wave 2 — session family:** list → close/delete (small, capability-interface assertions) → load/resume with replay (largest; depends on Wave-1 emitters for replay frames; alias-map identity decision up front).

**Wave 3 — commands, skills, compaction:**
1. Compaction + cache_control first (`/compact` depends on it; parity-audit item too).
2. Resolver chain (builtins → skills → agents → commands) + `available_commands_update`.
3. Class-B builtins in dependency order: /help /status /cost /mcp /doctor /permissions (cheap) → /model /config (routing override seam shared with editor configOptions) → /clear /resume (ride Wave 2) → /compact → /init /memory (expansion class).

**Wave 4 — parity closures:** hooks settings.json parsing + MCP-boundary wrap → AGENTS.md injection → thinking streaming (provider chunk type) → rich content (@-mentions/images) → background-subagent registry + completion notifications → background-Bash completion callback + persistent shell + sandbox reality.

**Wave 5 — SEED gaps:** steering queue (mid-turn drain; Telegram prerequisite — do not descope to queue-until-next-turn silently; if forced, make it a flagged fallback) → checkpoint restore guard + `/undo`.

**Wave 6 — SEED-001 kit extraction.** With Waves 1–5 settled, the library boundary is drawn around stabilized machinery: export `profile/shaper/provider/sched(modelrouting)/toolcat/toolexec/engine/hookdag/event/session/redact/checkpoint/audit/mcp` behind a composition-root API in `pkg/` (single module first; separate module only if an external consumer appears); `ecosys/openspec/coreexec/learning/firstrun/evalsuite/parity` stay app-side as the zcode-flavored composition; `cmd/ass-guard` becomes the reference app. The per-agent-model and permission seams must be expressed as kit interfaces (Frontend seam: emitter + requester), not concrete ACP types — that inversion is the one genuinely new design act in this wave; everything else is re-homing.

**Cross-wave invariants to hold:** `mise ci` gate per phase (non-negotiable); stdout reserved for frames; `.claude/` strictly read-only; transcript = sole audit artifact; graceful degradation everywhere (a failed permission store or registry load degrades to v1.1 behavior, never a serve refusal).

---

## Sources

**Codebase (HIGH — read directly for this research):**
- `cmd/ass-guard/acp_serve.go` — sessionTurnRunner, invocationFor/expandUserBlocks, sessionFor, runOneTurn park machinery, engineTurnRunnerAdapter
- `cmd/ass-guard/cron_wiring.go` — startSessionForwarder, turnActive mute discipline, server-driven turn pattern
- `internal/acp/server.go`, `handlers.go`, `types.go` — handler map, ChunkEmitter, SessionCloser assertion, D-09 load stub, RawMessage id handling
- `internal/session/session.go`, `projector.go`, `subagent.go`, `ask.go`, `manager.go`, `truncate.go`, `transcript*.go` — turn loop, gate sites, suspension broker, transcript line vocabulary, truncation chokepoint, reconstruction prior art
- `internal/event/events.go` — existing ToolCall/ToolCallUpdate kinds and buffer sizing
- `internal/provider/provider.go`, `internal/shaper/shaper.go` — StreamChunk vocabulary, Message shape, toThinking mapping
- `internal/ecosys/types.go`, `expand.go`, `skills.go`, `hooks.go`, `loader.go` — registry shape, Expand semantics, listings, HookRunner deny path, discovery scopes
- `internal/coreexec/register.go`, `bash.go`, `background.go`, `todo.go`, `messaging.go` — ToolHooks chokepoint, TaskRegistry, TodoStore, mailbox
- `internal/checkpoint/store.go` — shipped shadow-git design (EARLY-01)
- `docs/compaction-decision.md` + `internal/profile/corpus_scan.go` — corpus-proven compaction/cache_control facts and routed gaps
- `.planning/seeds/SEED-001…md`, `SEED-004…md` — kit scope, four-gap dispositions

**External (MEDIUM — primary-source fetched, not yet live-handshake-validated):**
- [ACP Schema](https://agentclientprotocol.com/protocol/schema) — session/update variant union; session family methods + capability gates; configOptions placement (ClientCapabilities.session.configOptions → new/load/resume responses → session/set_config_option)
- [ACP Tool Calls](https://agentclientprotocol.com/protocol/tool-calls) — tool_call/tool_call_update fields, statuses, content/locations; request_permission options/outcome contract; pending-status-while-awaiting semantics
- [ACP Elicitation](https://agentclientprotocol.com/protocol/elicitation) — elicitation/create union (form/url/other), accept/decline/cancel outcomes, elicitation/complete
- Claude Code compaction behavior (community-documented: ~92–95% auto-compact threshold, /compact instructions, known lossiness) — treated as directional context only; ass-guard's own corpus scan overrides it for design decisions

---
*Architecture research for: ass-guard v1.2 Claude Code Parity*
*Researched: 2026-08-26*
