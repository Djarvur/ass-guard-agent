# Phase 16: ACP Wire Foundation - Research

**Researched:** 2026-08-26
**Domain:** ACP v1 wire protocol (outbound JSON-RPC, ordered TurnEmitter, transcript schema) on the Phase-15 carved runtime
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

Every decision D-01..D-23 is locked. Deviations are not the planner's to make. Verbatim from 16-CONTEXT.md:

#### Backpressure & Interleaving
- **D-01:** Overflow behavior: block producers, priority bypass — bounded lanes, nothing ever dropped (criterion 2 verbatim). The foreground lane bypasses queued background frames so a chatty subagent/engine stream cannot head-of-line-block the editor's active turn.
- **D-02:** Ordering: single priority queue inside the TurnEmitter; foreground class preempts the queue head, background is FIFO within itself. Single-writer total order. The interleaving policy is "foreground preempts, background FIFO" — documented and tested as an invariant (criterion 2's multi-emitter -race stress).
- **D-03:** Writer wedge: producers block with context awareness; a stall detector logs + increments a structured metric when any lane is full beyond a ~5s threshold — degrade loudly, never auto-disconnect, never block silently.
- **D-04:** Gate placement: bounded multi-emitter -race stress (~seconds, fake slow writer, asserts order + no-drop) runs in every `mise ci`; a deeper adversarial soak (minutes) lives in the eval suite.

#### Config Advertisement & Scope
- **D-05:** Full v1.2 menu advertised from day 1. Apply-as-landed reconciliation: initialize/set applies keys whose handlers exist (tier/model day 1); advertised-but-unhandled keys apply as logged pending-handler no-ops — never an error, never silently discarded state. Later phases register real handlers behind the shape locked here.
- **D-06:** Menu membership: the planner enumerates the full v1.2 menu from REQUIREMENTS.md's editor-facing options at plan time; this CONTEXT locks the rule (full menu, apply-as-landed), the plan lists the enumerated membership for review.
- **D-07:** Mutation semantics: persist-then-apply — the config write (existing config-write path, 0600 discipline) must succeed before live application; write failure returns a typed JSON-RPC error with session state untouched. No half-applied state. API keys are never options (env/file only, per ACP-08).
- **D-08:** Two config layers, both editor-editable, project is the default target; the global layer is addressed via a wire-level scope parameter in the option schema. No dependency on Phase 20's command family for global access.
- **D-09:** Invalid option values: typed reject (option key + violation detail returned to the client); never accept-and-ignore.
- **D-10:** Initialize-time Zed settings blob: parsed schema-tolerantly (unknown keys survive round-trip) and every recognized key is applied eagerly, with blob-fills-unset precedence — explicit config (files, editor-set options) always wins; the blob is the default-of-last-resort.
- **D-11:** Advertised options carry their current EFFECTIVE value (resolved through the precedence chain), not static defaults — the editor UI can never display stale truth.
- **D-12:** Precedence stays one chain: session override (/model, Phase 20) > time-window > project > global. Editor config writes are simply another writer into the project/global layers; /model stays the ephemeral top of the chain.

#### Client Probing & Degradation
- **D-13:** Capability probed eagerly at initialize (e.g. a probe elicitation), result cached for the session lifetime — no per-ask probing, no mid-session re-probe. Older clients answer -32601 once and every surface knows to degrade.
- **D-14:** Unanswered id'd request: timeout → ONE retry → fall back to the plain-text/stdout path + structured log (request id + elapsed). Mirrors the v1.1 ask-timeout-then-non-answer philosophy (12-D-01): the user always gets something; the connection never wedges.
- **D-15:** Request ids: UUID strings BOTH direction. Registry keyed by normalized id string.
- **D-16:** Telemetry: structured stderr logs + in-process counters (probes, timeouts, fallbacks, writer stalls — same counter family as D-03's stall metric). No OTel dependency; counters surface via /status (Phase 20) later.
- **D-17:** Two timeout classes in the registry: FAST-CONTROL (probe/config requests, ~10s default) vs HUMAN-ASK (elicitation/permission, human timescale — minutes, mirroring 12-D-01's 10-min default). A permission form never times out on a thinking user.
- **D-18:** Degradation is sticky per session (a capability that degraded stays degraded; no flapping, no re-probe backoff). The next session probes fresh.
- **D-19:** The registry supports synthetic-cancel resolution NOW (turn dies → pending ask resolves cancelled-normal, the contract ACP-01 names) — Phase 17 only calls the primitive; the wire phase ships it complete. — **Reversibility:** costly.

#### Transcript Schema Stability
- **D-20:** Additive-only weak schema: the existing line envelope keeps its kind-discriminated shape; new kinds (raw-thinking, local_command, compaction marker) append; readers tolerate unknown kinds and unknown fields; payloads carry `json.RawMessage` until their owning phase parses them. No global schema_version field. — **Reversibility:** one-way.
- **D-21:** Compaction marker = rich boundary record: boundary id + timestamp + token-usage snapshot + pre/post pointers into the transcript.
- **D-22:** local_command = full invocation record: command key + verbatim args + resolution-source chain (builtin→skill→agent→file, per CMDS-01) + expansion outcome.
- **D-23:** Thinking-bytes redaction exclusion is type-level: raw-thinking lines take a distinct code path the redactor never sees; payload stays untouched `json.RawMessage` (no re-serialization → no accidental mutation). Test asserts the redactor is called zero times for thinking bytes.

### Claude's Discretion
- Lane capacity bounds, exact stall threshold (D-03's ~5s is directional).
- Probe elicitation's concrete payload (any cheap no-op-shaped ask; Phase 17 replaces with real forms).
- Counter names + log field names (consistency with existing audit/tracer field conventions).
- Whether raw-thinking lines carry provider attribution metadata (model, phase) — planner decides from replay needs.

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description (from REQUIREMENTS.md) | Research Support |
|----|-------------------------------------|------------------|
| ACP-03 | Zed renders live turn activity: `tool_call`/`tool_call_update` streaming (kind/status/diff/locations), `plan` updates mirroring TodoWrite, `agent_thought_chunk` — all through one ordered inline TurnEmitter with explicit backpressure policy | v1 sessionUpdate vocabulary, ToolCall/ToolCallUpdate/Diff/ToolCallLocation/Plan wire shapes verified verbatim from the canonical schema (§Wire Vocabulary); Zed-side rendering paths confirmed in Zed source; emitter/bus/Writer seams mapped (§Architecture Patterns) |
| ACP-08 | Editor drives configuration: `configOptions[]` advertised at initialize/new/load/resume responses, Zed settings payload read at initialize, `session/set_config_option` handled (tier/model defaults switchable from editor UI); API keys stay env/file, never editor settings | SessionConfigOption/set_config_option/config_option_update shapes verified verbatim; Zed's ACTUAL settings mechanism verified (no initialize blob — set_config_option push; §D-10 Reconciliation); config layer paths + precedence chain mapped; missing config-write path identified as build work |
</phase_requirements>

## Research Flag Verdicts (the gate — read first)

The three ROADMAP-flagged LOW-confidence claims were verified against the canonical source (schema + docs downloaded from `github.com/zed-industries/agent-client-protocol` @ main, 2026-08-26, and grepped locally).

### Verdict 1: sequenceNumber — DOES NOT EXIST in ACP

`grep -c sequenceNumber` over `schema/v1/schema.json` (170 defs) and `schema/v2/schema.json` (175 defs) = **0 and 0**; zero occurrences in `docs/protocol/v1/*.mdx` (prompt-turn, cancellation, session-config-options, slash-commands, schema). [VERIFIED: zed-industries/agent-client-protocol schema/v1/schema.json + schema/v2/schema.json + docs/protocol/v1/ — 0 matches each]

**Consequence for planning:** the "continuity / reset-on-load legality" question is void — there is no wire-level sequence number whose reset rules Phase 16/18 must honor. Frame ordering on ACP stdio is simply newline-frame order (our mutex-guarded `Writer` already guarantees it — internal/acp/framer.go:78-131). What DOES exist and must stay unique within a session: `messageId` (all chunks of one message share it; "A change in messageId indicates a new message has started" [VERIFIED: schema v1 ContentChunk def]) and `toolCallId`. ACP-06's "continued id sequences from transcript maxima" is therefore a **transcript-side** freshness concern (new ids must not collide with pre-restart ids), not a wire-counter concern — uuidV4-style generation (the existing `newSessionID` pattern, internal/acp/handlers.go:227-239) satisfies it by construction.

### Verdict 2: available_commands_update — full replacement

Schema: `AvailableCommandsUpdate.availableCommands` = "Commands the agent can execute" (array of `{name, description, input{hint}}`); no add/remove/patch discriminators exist anywhere in the def. Docs: "The Agent can update the list of available commands at any time during a session by sending another `available_commands_update` notification. This allows commands to be **added** based on context, **removed** when no longer relevant, or **modified** with updated descriptions." [VERIFIED: zed-industries/agent-client-protocol docs/protocol/v1/slash-commands.mdx §Dynamic updates + schema/v1 AvailableCommandsUpdate/AvailableCommand defs]

**Consequence:** add/remove/modify are only expressible as "send the new complete array" — the client holds no merge state. Phase 20 (ACP-04) must always send the complete current set; Phase 16 only locks this vocabulary into the emitter's frame surface.

### Verdict 3: the cancel contract — two layers, both now fully pinned

**Protocol layer, `$/cancel_request` (notification, params `{requestId}`)** — on receipt a supporting implementation:
- "**MAY** cancel the corresponding request activity and all nested activities related to that request"
- "**MUST** send one of these responses for the original request: A valid response with appropriate data (such as partial results or cancellation marker) / An error response with code `-32800` (Request Cancelled)"
- Internal cancellation (agent-side timeout, resource limits) "**SHOULD** send the same `-32800`" [VERIFIED: zed-industries/agent-client-protocol docs/protocol/v1/cancellation.mdx]

**Feature layer, `session/cancel` (notification, params `{sessionId}`)** — verbatim duties:
- Client "**SHOULD** preemptively mark all non-finished tool calls pertaining to the current turn as `cancelled` as soon as it sends the notification"
- Client "**MUST** respond to all pending `session/request_permission` requests with the `cancelled` outcome"
- Agent "**SHOULD** stop all language model requests and all tool call invocations as soon as possible"
- "After all ongoing operations have been successfully aborted and pending updates have been sent, the Agent **MUST** respond to the original `session/prompt` request with the `cancelled` stop reason" — and "Agents **MUST** catch these [abort] errors and return the semantically meaningful `cancelled` stop reason" (not an error response)
- Agent "**MAY** send `session/update` notifications ... after receiving the notification, but it **MUST** ensure that it does so before responding to the `session/prompt` request" [VERIFIED: zed-industries/agent-client-protocol docs/protocol/v1/prompt-turn.mdx §Cancellation]

**Cascade (the doc's own mermaid):** client sends `session/cancel` → agent sends `$/cancel_request` for each of ITS in-flight outbound requests (example shows `terminal/create` id=2 and `session/request_permission` id=3) → client answers each with `-32800` → agent answers the original `session/prompt` with `stopReason: "cancelled"`. [VERIFIED: same file, "Cascading Cancellation Flow"]

**Consequence for D-19:** "turn dies → pending ask resolves cancelled-normal" maps exactly onto the cascade: on turn death the registry resolves the pending entry as cancelled AND (wire-visibly) emits `$/cancel_request(requestId)` for it; the client's `-32800` (or D-14 timeout/fallback if the client never answers) closes the loop. Note v1 `ToolCallStatus` has NO "cancelled" value (v1: pending/in_progress/completed/failed — v2 adds cancelled) [VERIFIED: schema v1 ToolCallStatus def], so agent-side tool-call closure after cancel is agent-internal state, not a v1 wire status.

### stopReason enum (verified verbatim, needed by the runner contract)

`end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled`, plus an open "other" variant ("Values beginning with `_` are reserved for implementation-specific extensions"). [VERIFIED: schema v1 StopReason def — anyOf of consts]

## Zed Client Facts (verified in zed-industries/zed @ main)

1. **Zed speaks v1.** `acp::InitializeRequest::new(ProtocolVersion::V1)` at crates/agent_servers/src/acp.rs:993; `MINIMUM_SUPPORTED_VERSION = ProtocolVersion::V1` at :663. Our `protocolVersion: 1` advertisement (internal/acp/handlers.go:48) is correct for a real Zed handshake; the criterion-1 vocabulary (`tool_call`/`tool_call_update`/`plan`/`agent_thought_chunk`) is exactly the **v1** sessionUpdate kind set. [VERIFIED: zed-industries/zed crates/agent_servers/src/acp.rs:993,:663]
2. **Zed advertises elicitation AND boolean config options in clientCapabilities.** `client_capabilities_for_agent` (acp.rs:767-795): `fs{readTextFile:true,writeTextFile:true}`, `terminal:true`, `auth{terminal:true}`, `session.configOptions.boolean:{}`, `elicitation{form:{},url:{}}`, `_meta{terminal_output:true,"terminal-auth":true}` (+`parameterizedModelPicker` for Cursor only). [VERIFIED: zed-industries/zed crates/agent_servers/src/acp.rs:767-795]
   - **D-13 reconciliation input:** elicitation-form capability is available in the initialize handshake itself (v1 ClientCapabilities.elicitation). The probe stays valuable for clients that DON'T advertise (older/other editors) and as belt-and-suspenders per the locked decision — but the planner should read the advertisement FIRST and probe only when it is absent/ambiguous, keeping D-13's sticky-degradation machinery for both paths. Boolean options may be advertised to Zed without fallback (capability present).
3. **There is NO initialize-time settings blob.** Zed reads its own `AllAgentServersSettings` (per-agent `default_config_options: HashMap<String, AgentConfigOptionValue>`) and, when the agent's `configOptions` advertisement arrives, calls `apply_default_config_options` (acp.rs:1303-1391): for each advertised option where Zed's settings hold a default whose value exists in the advertised options list (ungrouped or grouped), Zed sends a `session/set_config_option` request; settings changes refresh defaults live. [VERIFIED: zed-industries/zed crates/agent_servers/src/acp.rs:1303-1391 + AcpConnectionDefaults::refresh_from_settings :452-477] → see §D-10 Reconciliation.
4. **Zed renders plan updates natively:** `acp::SessionUpdate::Plan(plan) => self.update_plan(plan, cx)` (crates/acp_thread/src/acp_thread.rs:2610) maintaining entries with per-entry status. [VERIFIED: zed-industries/zed crates/acp_thread/src/acp_thread.rs:2610]
5. **Zed sends UUID string request ids** (the v1.0 endless-loading incident — internal/acp/types.go:26-32 documents the string-id fix). D-15's UUID-strings-both-directions is legal: JSON-RPC ids are `string | integer | null` and "The Server MUST reply with the same value"; agent-originated ids for outbound calls are unconstrained by ACP beyond JSON-RPC shape. [VERIFIED: schema v1 RequestId def + internal/acp/types.go:26-32]

## D-10 Reconciliation (locked intent, corrected mechanism)

D-10 locks: settings parsed schema-tolerantly, recognized keys applied eagerly, blob-fills-unset precedence. The mechanism premise ("initialize-time Zed settings blob") does not exist in current Zed — Zed pushes per-option defaults via `session/set_config_option` right after our advertisement arrives, and re-pushes on its own settings changes. What exists on our inbound initialize is `_meta` (free-form extension object, "Implementations MUST NOT make assumptions about values at these keys" [VERIFIED: schema v1 _meta annotation]).

Planning shape that honors the locked intent without the falsified premise:
- Keep a schema-tolerant `_meta` reader on initialize (any client MAY carry defaults there; unknown keys survive round-trip) — this IS D-10's blob channel, generic.
- Treat inbound `session/set_config_option` as the second editor-defaults channel (Zed's actual one): values arrive AFTER our advertisement, exactly when Zed knows our option ids.
- Both channels write with the same precedence discipline (blob/editor-set never overrides explicit file config per D-10/D-12's chain) and both route through the D-07 persist-then-apply write path.
- This needs no CONTEXT change: D-10's semantics (eager apply, fills-unset) hold; only the transport observation is corrected.

## Summary

Phase 16 attaches three wire primitives to the Phase-15 carved seam. (1) **TurnEmitter**: today every frame goes through the acp `Writer` — a 256-slot FIFO channel with one drain goroutine (internal/acp/framer.go:90-131) — which serializes but gives no priority; D-01/D-02's foreground-preempts/background-FIFO policy needs a new single-owner priority queue sitting between all frame producers and that Writer, constructed in acpserve.Run (internal/acpserve/acp_serve.go:200-215, where `srv := acp.NewServer(...)` and `runner.SetEmitter(srv.Emitter)` already meet). (2) **Outbound id'd requests + pending-response registry**: the server currently has no outbound request support at all, and its dispatch loop would mis-route an inbound response frame (id set, method empty) into the handler map and emit a spurious `-32601` — the registry must intercept response-shaped frames before handler lookup (internal/acp/server.go:180-196 + handlers.go:215-220). (3) **Transcript extension**: the `Line` envelope is a flat kind-discriminated struct with 16 types (internal/session/transcript.go:18-49) that already tolerates unknown fields (Go stdlib unmarshal) — three new kinds append per D-20..D-23, with the raw-thinking append taking a redaction-free code path distinct from `Manager.appendLine` (internal/session/manager.go:76-101, which always redacts before write).

On the protocol side, all three research flags are now settled against the canonical zed-industries source (§Research Flag Verdicts): sequenceNumber does not exist; available_commands_update is full-replacement; the cancel contract is a two-layer MUST-set with a documented cascade that D-19's synthetic-cancel implements verbatim. Zed speaks v1 and advertises both elicitation and boolean-config capabilities in the handshake. One locked-decision premise needs planner reconciliation (D-10's settings blob → Zed actually pushes set_config_option; §D-10 Reconciliation), and one "existing asset" in the CONTEXT does not exist yet: there is **no config-write path** — `internal/modelrouting/load.go` only Loads; `internal/providerfactory` only exposes `GlobalConfigPath()`/`ProjectConfigPath(workDir)` and a 0600 permission warning. D-07's typed persist-then-apply writer must be built in this phase, following the `.ass-guard` artifact-family 0600 discipline (e.g. internal/session/transcript.go:11 `filePermOwner = 0o600`).

**Primary recommendation:** build the TurnEmitter as the sole notification producer into the existing `Writer` (so the Writer channel never accumulates a reorderable backlog), extend `Server` with a response-routing shim + UUID-keyed registry with two timeout classes, append the three transcript kinds through a redaction-free thinking path, and implement D-05..D-12's config surface as v1 `SessionConfigOption` shapes with effective-value `currentValue` — verifying against a real Zed handshake since Zed is confirmed v1.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Frame serialization + slow-client backpressure | internal/acp `Writer` (transport) | — | Mutex + bounded channel + drain goroutine already owns no-interleaved-lines; keep as the final chokepoint (framer.go:78-131) |
| Priority interleaving (fg preempts / bg FIFO) | internal/acp TurnEmitter (new) | acpserve composition | D-02 puts the single priority queue inside the TurnEmitter; acpserve.Run constructs it at the existing srv/emitFor junction |
| Bus-kind → wire-frame forwarding | internal/runtime Runner (emitFor seam) | internal/acp (interface owner) | Runner subscribes bus kinds and calls the emitter interface (runtime.go:492-601 pattern); ACP words stay out of runtime per 15-D-20 |
| Outbound requests + pending registry | internal/acp Server (new registry type) | — | Wire concern: id generation, response interception, timeout classes, synthetic-cancel — all live with the frame codec |
| Transcript line schema + append paths | internal/session Manager/TranscriptWriter | runtime/session core as producers | Line envelope, tolerance, and the redaction seam are session-owned (transcript.go, manager.go) |
| Config advertisement + set handler | internal/acpserve (composition) + modelrouting/providerfactory (layers) | internal/acp (wire shapes) | Layer paths + precedence are modelrouting/providerfactory-owned (provider_factory.go:20/:34); wire shape is acp-owned; the composition root binds them |
| Capability probing + sticky degradation | internal/acpserve session state | registry | Session-lifetime cache per D-13/D-18 |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib (`sync`, `context`, `time`, `encoding/json`, `crypto/rand`) | go.mod toolchain | Priority lanes, ctx-aware blocking, timers, RawMessage passthrough, UUID v4 generation | The whole phase is stdlib-shaped; the repo hand-rolls JSON-RPC by locked precedent (framer.go D-14 note: "~150 LOC, zero deps") |
| `golang.org/go-yaml` (already dep, via internal/modelrouting) | existing | Config layer read/merge; the NEW writer serializes the same format | modelrouting/load.go already Load()s YAML layers; no new parsing stack |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| none new | — | — | No new packages are introduced by this phase |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled priority emitter | A queue library / disruptor pattern | Rejected by project convention (zero-dep framer precedent); two-lane priority is ~100 LOC with nested select |
| Custom UUID gen | github.com/google/uuid | newSessionID (handlers.go:227-239) + shaper uuidV4 already exist in-repo; a dep buys nothing |

**Installation:** none — `go mod` unchanged.

**Version verification:** no registry lookups needed (zero new packages; this section satisfied by inspection).

## Package Legitimacy Audit

No external packages are installed by this phase (stdlib + existing go.mod only).

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| — (none) | — | — | — | — | — | N/A |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
                     ┌──────────────────────────── acpserve.Run (composition root) ────────────────────────────┐
                     │                                                                                       │
 bus events ──┐      │  runtime.Runner                                                                       │
 (ToolCall,    │      │  ┌──────────────┐   emitFor(sessionID) ──> acp.ChunkEmitter (extended iface)           │
 ToolCallUpdate│      │  │ turn loop    │───────────────────────────────────┐                                  │
 AgentMsgChunk,│──────┼─>│ subagents    │                                    │                                  │
 Usage, ...)   │      │  │ engine/cron  │                        ┌───────────▼──────────────┐                   │
               │      │  └──────────────┘                        │ TurnEmitter (NEW, acp)  │                   │
               │      │             background emitters ────────>│ lane: FOREGROUND (preempt at head)          │
               │      │                                           │ lane: BACKGROUND  (FIFO)                   │
               │      │                                           │ stall detector (D-03, ~5s → log+counter)   │
               │      │                                           │ single drain goroutine ──► acp.Writer       │
               │      │                                           └──────────────────────────┬────────────────┘                   │
               ▼      │                                                                      │                                   │
        TranscriptWriter                                                                      │                                   │
        (Manager.Append*)                                                          ┌──────────▼────────────────┐        │
        + NEW kinds:                                                              │ Writer (existing)        │        │
        raw_thinking / local_command /                                            │ bounded ch(256)+drain    │        │
        compaction (D-20..23)                                                     └──────────┬────────────────┘        │
        raw path skips Redact                                                     │                                   │
                                                                                   ▼ stdout (one frame/line)          │
   stdin ──> Server.Serve reader loop                                                                                 │
     ├─ id==nil  ──> inline notification dispatch (session/cancel, $/cancel_request)                                 │
     ├─ id!=nil && method!="" ──> per-request goroutine ──> handlers ──> writeResult/writeError ──> Writer            │
     └─ id!=nil && method=="" (RESPONSE to our outbound request) ──> [NEW] registry.Resolve(id) ──chan──────────┐      │
                                                                                                                │      │
   Outbound request path (NEW): registry.Call(ctx, method, params, class)                                        │      │
     uuid v4 id ──> map[id]chan entry ──> frame via TurnEmitter/Writer ──> select{response, ctx, timer(FAST-CONTROL|HUMAN-ASK)}
     timeout ──> ONE retry (D-14) ──> fallback + structured log ──> degrade sticky (D-18)
     turn dies ──> synthetic-cancel: resolve pending as cancelled + emit $/cancel_request(id) (D-19, cascade)
```

Trace the primary use case: a foreground turn streams a tool_call → bus ToolCall → runner forwarder → emitter foreground lane → drain → Writer → stdout → Zed renders the native card. Concurrently a subagent streams thought chunks → background lane → FIFO behind any foreground preemption — criterion 2's invariant.

### Recommended Project Structure

```
internal/acp/
├── framer.go            # unchanged Writer (final serialization chokepoint)
├── server.go            # + response-frame interception in Serve dispatch
├── emitter.go           # NEW: TurnEmitter (lanes, stall detector, drain) + extended ChunkEmitter methods
├── request_registry.go  # NEW: pending-response registry, Call(), timeout classes, synthetic-cancel
├── handlers.go          # initialize/new/load/resume capability enrichment; set_config_option handler
└── types.go             # + wire shapes: ToolCallFrame, PlanFrame, ConfigOptionFrame, ...
internal/acpserve/
└── acp_serve.go         # TurnEmitter construction + wiring (srv.Emitter → emitter-backed view)
internal/session/
├── transcript.go        # + TypeRawThinking / TypeLocalCommand / TypeCompaction constants + fields
├── manager.go           # + AppendRawThinking (redaction-free path), AppendLocalCommand, AppendCompaction
└── projector.go         # tolerance: skip unknown/new kinds on replay
internal/modelrouting or providerfactory/
└── config_write.go      # NEW: typed persist-then-apply layer writer (0600)
```

(Exact file split is planner's; the seams above are the load-bearing part.)

### Pattern 1: Two-lane priority emitter (nested select)

**What:** one goroutine owns total order; foreground preempts at the head, background is FIFO.
**When to use:** the TurnEmitter (and only it — everything else keeps using plain channels).

```go
// Sketch (illustrative — lane capacities are Claude's-discretion per CONTEXT):
for {
	select {
	case f := <-fgCh: // foreground first, always
		write(f)
	default:
		select { // both, equal chance only when fg empty
		case f := <-fgCh:
			write(f)
		case f := <-bgCh:
			write(f)
		case <-ctx.Done():
			return
		}
	}
}
```

Producers block on their lane's buffered channel (D-01: bounded, nothing dropped) with ctx-awareness (D-03); a `time.Ticker`-sampled "lane full since" watermark feeds the stall detector (log + counter, never disconnect).

**Critical ordering rule:** the emitter's drain goroutine must be the ONLY producer of session/update frames into `acp.Writer` (direct `adapter` writes are retired). Then the Writer's channel can never hold a backlog of emitter frames (the drain blocks per write, keeping ≤1 in flight), so foreground preemption is never defeated by frames already sitting in the Writer buffer.

### Pattern 2: Response interception before handler dispatch

`Serve` currently dispatches by id-presence alone (server.go:180-196); a client response to our outbound request carries `id` + `result|error` and NO `method`, and `handleRequest` would look up `s.handlers[""]`, miss, and write a spurious `-32601` (handlers.go:216-220). The registry shim:

```go
if msg.ID != nil && msg.Method == "" && (msg.Result != nil || msg.Error != nil) {
	s.registry.Deliver(msg.ID, msg.Result, msg.Error) // chan send, never blocks long
	return // a response gets NO response
}
```

`Deliver` resolves the map entry (registry's own mutex — never the writer mutex, criterion 3) or logs an unknown-id stderr line. `Message` (internal/acp/types.go:33-40) already models this: `Method` has `omitempty`, `ID` is `json.RawMessage`.

### Pattern 3: Registry entry (timeout classes, retry, synthetic-cancel)

```go
type pending struct {
	ch     chan resolution // buffered(1): Deliver never blocks on a waiter
	class  timeoutClass    // FAST-CONTROL (~10s) | HUMAN-ASK (mirrors session.DefaultAskTimeout = 10*time.Minute, ask.go:36)
	method string
	sent   time.Time
}
// Call: uuid v4 id -> map[string]*pending (own mutex) -> enqueue frame -> select{ch, ctx.Done, time.After(class)}
// timeout: ONE retry (fresh elapsed window) -> fallback error + structured log{id, method, elapsed} (D-14)
// turn death: ResolveCancelled(id) -> deliver cancelled + emit $/cancel_request{id} (D-19 cascade)
// -32800 response: immediate cancelled resolution — NOT a timeout, no retry
```

### Pattern 4: Additive transcript kinds + redaction-free thinking path

`Line` is flat and kind-discriminated (transcript.go:61-115); append three constants in the existing block (verbatim neighbors at transcript.go:18-49: `TypeSessionStart = "session_start"` … `TypeAskSuspended = "ask_suspended"`):

```go
TypeRawThinking  = "raw_thinking"   // D-20/PAR-05: payload json.RawMessage, provider verbatim
TypeLocalCommand = "local_command"  // D-22/CMDS-02: command key + args + resolution chain + outcome
TypeCompaction   = "compaction"     // D-21/PAR-01: boundary id + ts + usage snapshot + pre/post pointers
```

`Manager.appendLine` ALWAYS redacts before write (manager.go:76-101: `red, err := m.redactor.Redact(raw)`). D-23 requires a distinct method (e.g. `appendLineUnredacted`) used ONLY by `AppendRawThinking`; `json.Marshal` of a `json.RawMessage` field emits the original bytes verbatim, so byte-identity survives marshaling — the redactor is the only mutation risk, hence the type-level exclusion + zero-call-count test. Reader tolerance already holds: `readTranscriptFile` unmarshals into `[]Line` and Go ignores unknown fields; unknown `Type` values parse with the discriminator intact. `json.RawMessage` payloads satisfy D-20's "no parse until owning phase".

### Anti-Patterns to Avoid

- **Writing notifications around the TurnEmitter** (any direct `s.out.Write` of session/update frames from runner/subagent/engine paths) — reintroduces cross-kind reordering and defeats criterion 2's invariant.
- **Resolving pending entries while holding the writer mutex** — the registry mutex is separate; Deliver is a buffered-chan send (criterion 3 verbatim: "without blocking the writer mutex").
- **Responding to a response** — after registry interception, never write a frame for a method-less inbound message.
- **Dropping frames on overflow** — D-01: block producers; the stall detector is the loud signal, dropping is the bug.
- **Routing the prompt response past the queue while turn frames are still queued** — the cancel contract's "updates MUST be sent before responding to session/prompt" (see Pitfall 4).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Frame serialization / no-interleave guarantee | New locking around stdout | Existing `acp.Writer` (framer.go) | Already mutex+channel+drain, already the D-05 backpressure boundary; the emitter composes it |
| UUID generation | Fresh random-string scheme | The `newSessionID` crypto/rand v4 pattern (handlers.go:227-239) / shaper uuidV4 | Collision-by-construction for message/toolCall/request ids across restarts (ACP-06's real continuity need) |
| Config layer resolution | New path logic | `providerfactory.GlobalConfigPath()` / `ProjectConfigPath(workDir)` + `modelrouting.Load` layering (provider_factory.go:20/:34; load.go:43) | Precedence chain D-12 rides already exists read-side; only the writer is new |
| YAML round-trip for the config writer | Bespoke string surgery | The same YAML marshal/load the readers use | Layer merge semantics (deepMerge, load.go:100) must be invertible with what Load reads back |

**Key insight:** every wire-shape decision this phase locks is already decided by the canonical schema — copying the schema's verbatim field names (camelCase: `toolCallId`, `currentValue`, `configOptions`) is correctness, not style; the repo's `//nolint:tagliatelle` convention marks each one.

## Common Pitfalls

### Pitfall 1: Inbound responses mis-dispatched as requests
**What goes wrong:** a client response (id + result/error, no method) hits the handler map under key `""`, misses, and the server writes a spurious `-32601` error frame — protocol garbage the client may treat as a failure.
**Why:** `Serve` branches only on `msg.ID == nil` (server.go:180-187).
**How to avoid:** the Pattern-2 interception, ordered BEFORE handler lookup, with a test that feeds a response frame and asserts zero outbound frames + registry delivery.
**Warning signs:** -32601 frames in Zed's ACP debug log (Zed exposes `subscribe_debug_messages`) right after our outbound probe fires.

### Pitfall 2: Priority inversion through the shared Writer buffer
**What goes wrong:** foreground frames bypass the emitter queue but 200 background frames already sit in the Writer's 256-slot channel — foreground still queues behind them; criterion 2's invariant silently fails only under backpressure.
**Why:** two queueing layers with different policies.
**How to avoid:** TurnEmitter drain is the ONLY notification producer into Writer (≤1 frame in flight); responses (writeResult/writeError) may still use Writer directly — response-vs-notification ordering is not spec-constrained except the turn-end rule (Pitfall 4).
**Warning signs:** stress test that passes without the fake-slow-writer but fails with it.

### Pitfall 3: Deadlock via lock-held enqueue
**What goes wrong:** a producer blocks on a full lane while holding a lock the drain path (or Deliver) needs — whole serve wedges silently, the anti-D-03.
**Why:** lanes block by design; locks composition is the bug.
**How to avoid:** no lock acquisition spans an Enqueue; registry Deliver uses buffered(1) channels; the -race stress with fake slow writer exercises exactly this.
**Warning signs:** stress test hang (not failure); stall-counter climbing while frames stop.

### Pitfall 4: Prompt response overtaking queued turn frames
**What goes wrong:** session/prompt handler returns and `writeResult` hits the Writer while the emitter queue still holds turn frames — violates "agent MAY send updates after cancel but MUST send them BEFORE responding to session/prompt" (verified cancel contract) and generally reorders the turn's epilogue.
**Why:** the response path and the emitter queue are independent.
**How to avoid:** turn-end flush — the runner/emit seam drains (or barrier-joins) the session's emitter lane before the handler returns; alternatively route the prompt response through the emitter as a foreground-class frame.
**Warning signs:** Zed showing a completed turn card with missing last chunks; cancel-then-missing-final-update in tests.

### Pitfall 5: Redactor touching thinking bytes
**What goes wrong:** raw-thinking lines routed through `appendLine` get walked/redacted — Anthropic signature bytes or provider JSON mutated; PAR-05's byte-identical round-trip dies here.
**Why:** `appendLine` unconditionally redacts (manager.go:83-89).
**How to avoid:** distinct `appendLineUnredacted` used only by the thinking path; test asserts the redactor is called ZERO times for thinking bytes (D-23 verbatim).
**Warning signs:** any shared helper both paths call "just for the newline".

### Pitfall 6: Config apply half-state on write failure
**What goes wrong:** live values changed, then the 0600 persist fails — D-07's forbidden half-applied state.
**Why:** natural ordering is apply-then-save.
**How to avoid:** persist first (typed write, atomic temp+rename per repo artifact conventions), apply only on success; failure returns the typed JSON-RPC error with session state untouched.
**Warning signs:** any code path that mutates the in-memory chain before the write returns nil.

### Pitfall 7: v1/v2 vocabulary drift
**What goes wrong:** implementing v2 names (`plan_update`, `configId`, bare-tool_call removal, `state_update`) while Zed speaks v1 — frames Zed ignores or errors on.
**Why:** v2 is the visible "current" schema on the repo main branch.
**How to avoid:** pin every wire name against `schema/v1/schema.json` (see §State of the Art table); Zed is verified V1.
**Warning signs:** `configId` vs `id` in SessionConfigOption; `plan_update` vs `plan`.

### Pitfall 8: Writer Close/Write after shutdown
**What goes wrong:** `Writer.Write` after `Close` blocks forever (documented at framer.go:133-136) — a late synthetic-cancel or retry wedges a goroutine at exit.
**Why:** Serve's defer closes the Writer after handlerWG waits.
**How to avoid:** registry/telemetry paths take ctx; TurnEmitter stops before server close; D-14's fallback path never needs the wire after Serve exits.
**Warning signs:** goroutine leak reports in the -race suite at shutdown.

## Code Examples

### Extended emitter surface (v1 wire shapes, field names verbatim from schema)

```go
// Source: schema/v1/schema.json defs ToolCall, ToolCallUpdate, Plan, PlanEntry, ContentChunk, Diff, ToolCallLocation
type ToolCallFrame struct {
	ToolCallID string             `json:"toolCallId"`           // required
	Title      string             `json:"title,omitempty"`
	Kind       string             `json:"kind,omitempty"`        // read|edit|delete|move|search|execute|think|fetch|switch_mode|other
	Status     string             `json:"status,omitempty"`      // pending|in_progress|completed|failed (v1 — NO cancelled)
	Content    []ToolCallContent  `json:"content,omitempty"`     // {type: content|diff|terminal, ...}
	Locations  []ToolCallLocation `json:"locations,omitempty"`   // {path (required, absolute), line?}
}
type PlanFrame struct {
	Entries []PlanEntry `json:"entries"` // REQUIRED, full replacement: "The client replaces the entire plan with each update"
}
type PlanEntry struct {
	Content  string `json:"content"`            // required
	Priority string `json:"priority"`           // required (PlanEntryPriority)
	Status   string `json:"status"`             // required: pending|in_progress|completed
}
type ThoughtChunkFrame struct { // agent_thought_chunk = ContentChunk
	MessageID string        `json:"messageId"`  // required
	Content   ContentBlock  `json:"content"`    // required
}
type DiffContent struct { // ToolCallContent variant "diff"
	Type    string `json:"type"` // "diff"
	Path    string `json:"path"` // required
	OldText string `json:"oldText,omitempty"` // null for new files
	NewText string `json:"newText"`           // required
}
```

### Config advertisement (v1 SessionConfigOption, verbatim field names)

```go
// Source: schema/v1 defs SessionConfigOption, SessionConfigSelect, SessionConfigBoolean; docs/protocol/v1/session-config-options.mdx
type ConfigOptionFrame struct {
	ID          string   `json:"id"`                    // v1 field name is `id` (v2 renames to configId — Pitfall 7)
	Name        string   `json:"name"`                  // required
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`    // mode|model|model_config|thought_level | _custom
	Type        string   `json:"type"`                  // "select" | "boolean" (boolean only after client advertises session.configOptions.boolean)
	// select variant:
	CurrentValue string             `json:"currentValue"` // REQUIRED — D-11: the EFFECTIVE value through the precedence chain
	Options      []ConfigOptionValue `json:"options"`     // required for select, omitted for boolean
}
// set_config_option response + config_option_update notification BOTH carry
// "the full set of configuration options and their current values" (docs verbatim).
```

### Outbound request + registry resolution

```go
// Source: docs/protocol/v1/cancellation.mdx (cascade) + JSON-RPC 2.0 id rules (schema v1 RequestId)
func (r *Registry) Call(ctx context.Context, method string, params json.RawMessage, class TimeoutClass) (*Message, error) {
	id := uuidV4() // D-15: UUID strings BOTH directions; JSON-RPC allows string|integer|null ids
	p := r.register(id, method, class)
	defer r.unregister(id)
	if err := r.emitRequest(id, method, params); err != nil { return nil, err }
	select {
	case m := <-p.ch:  return m, nil        // includes -32800 → sentinel cancelled resolution
	case <-ctx.Done(): return nil, ctx.Err()
	case <-time.After(class.limit):
		// D-14: ONE retry, then fallback + structured log{id, method, elapsed}; sticky degrade per D-18
	}
	...
}
// Turn death (D-19): ResolveCancelled(id) delivers a cancelled resolution to the waiter AND
// emits {"jsonrpc":"2.0","method":"$/cancel_request","params":{"requestId":id}} — the doc's cascade.
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| ACP single schema | Versioned schemas: `schema/v1` + `schema/v2` (v2 announced as draft — docs/announcements/acp-v2-draft.mdx) | v2 draft era, 2026 | Zed is V1 today (verified); implement v1 names; keep v2 drift table below |
| `modes` on session responses | `configOptions` supersede modes ("Modes will be removed in a future version") — session-config-options.mdx | stabilization era | Advertise configOptions; we already no-op set_mode — no dual-send obligation for a mode-like option we don't have |
| v1 `plan` kind | v2 renames to `plan_update` + PlanUpdateContent | v2 | v1 target — use `plan` |
| v1 `tool_call` + `tool_call_update` | v2 replaces with upsert-only `tool_call_update` + `tool_call_content_chunk`/`terminal_*` kinds | v2 | v1 target — send both kinds as today's criterion says |
| SessionConfigOption `id` | v2 renames to `configId` | v2 | v1 target — use `id` |
| ToolCallStatus 4 values | v2 adds `cancelled` | v2 | v1 has no cancelled status — agent-side closure is internal |
| No request cancellation | `$/cancel_request` + `-32800` stabilized (docs/announcements/request-cancellation-stabilized.mdx) | stabilized | D-19's primitive is spec-backed |
| configOptions select-only | boolean stabilized behind client capability (docs/announcements/boolean-config-option-stabilized.mdx) | stabilized | Zed advertises it (verified) — booleans usable with Zed |

**Deprecated/outdated:**
- `modes` field: transitional dual-send recommended by docs only for agents that HAVE mode-like options; we have none beyond plan-mode internal state.
- v1 `current_mode_update`: still valid v1, not needed by this phase's scope.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Zed @ main (fetched 2026-08-26) represents the Zed build this project targets (V1 protocol, set_config_option defaults push, elicitation/boolean capabilities) | Zed Client Facts | A stable Zed lagging main may lack boolean/elicitation advertisement — probe/degrade paths (D-13/D-14) already cover it; live-Zed criterion check will confirm |
| A2 | v2 remains draft/non-final for this milestone's window | State of the Art | If v2 finalizes and Zed switches before Phase 18, session vocabulary needs a v1/v2 negotiation layer — flag, do not build now |
| A3 | Probe elicitation payload = minimal no-op-shaped ask (CONTEXT discretion) | D-13 | None material — Phase 17 replaces it |
| A4 | Stall threshold ~5s, lane capacities (CONTEXT discretion) | Pattern 1 | Tuning only; invariant tests are threshold-independent |
| A5 | Raw-thinking lines carry provider attribution (model, phase) — recommended YES for Phase 19 replay needs (discretion item) | Pattern 4 | If omitted, replay cannot attribute thinking provenance; additive-only schema allows adding later but historical lines stay bare |
| A6 | The v1.2 full menu (D-06) input set = tier + model (day-1 handlers) + permissions.mode (ACP-01, Phase 17) + compaction threshold (PAR-01, Phase 19) — derived from REQUIREMENTS' editor-facing options | ACP-08 | Planner reviews enumeration per D-06; a missed option = later schema append (allowed by additive rule, but advertisement churn) |
| A7 | D-08's "wire-level scope parameter" has NO ACP-native field (verified absent from SessionConfigOption) — implemented as id-namespace (e.g. `model` vs `_global/model`) or `_meta`-carried scope; planner picks | ACP-08 / D-08 | Either is extension-legal (`_`-prefix reserved for implementation-specific use); choice affects editor UI grouping |
| A8 | Inbound `$/cancel_request` from client (cancelling ITS requests) needs only fast no-op/ack handling this phase — our outbound-request cancellation by the client is Phase 17 territory | Pattern 2/3 | If Zed sends it for long-running inbound requests (e.g. a future slow session/prompt), behavior degrades to natural completion — acceptable |

## Open Questions (RESOLVED)

All three open questions were resolved at plan time; the resolutions are pinned in 16-01-PLAN.md and inlined below.

1. **ChunkEmitter interface growth vs KIT-02 minimal pair** — (RESOLVED in 16-01 Task 1)
   - What we know: 25-CONTEXT D-13/D-14 want Emitter/Requester as kit-neutral session-side interfaces with an acp adapter translating; today runtime holds `emitFor func(sessionID string) acp.ChunkEmitter` (runtime.go:182) — runtime already speaks the acp interface.
   - What's unclear: whether Phase 16 should extend `acp.ChunkEmitter` in place (fast, acp-coupled) or introduce a neutral event interface in runtime with translation at acpserve.
   - Recommendation: extend `acp.ChunkEmitter` now (Phase 16 ships acp-side; 15-D-20's "no ACP words in runtime API" is about naming, and the existing seam already crosses the boundary); Phase 25's KIT-02 does the neutral-pair redesign with the emitter as prior art. Keeps this phase's diff reviewable.
   - Resolution: do NOT widen `acp.ChunkEmitter` in place. A new `ActivityEmitter` interface in internal/acp/emitter.go embeds ChunkEmitter and adds ToolCall/ToolCallUpdate/PlanUpdate/ThoughtChunk; the existing interface keeps declaring exactly AgentMessageChunk so test fakes across the repo keep compiling, and runtime forwarders type-assert emit to ActivityEmitter. The acp-side-now guidance holds — the growth is additive-via-embedding, not in-place mutation; Phase 25's KIT-02 still owns the neutral-pair redesign.
2. **Where the turn-end flush barrier lives** (Pitfall 4) — (RESOLVED in 16-01 Task 2)
   - What we know: the cancel contract requires updates-before-response; the prompt handler owns the response write.
   - What's unclear: flush-in-emitFor-wrap vs route-response-through-emitter.
   - Recommendation: planner decides; route-response-through-emitter is the stronger invariant but touches handleSessionPrompt's return path.
   - Resolution: barrier at the handler. handleSessionPrompt calls the emitter barrier after turnRunner.Run returns and before the handler returns; responses keep using writeResult directly, which stays legal because only notification-vs-response order at turn end is spec-constrained — the barrier's enqueue/written counter pair orders the response after all queued turn frames with at most one in-flight frame preceding it.
3. **Session-scoped vs serve-scoped emitter ownership for background emitters** (engine firings before any session/prompt — WINDOWS #3's server-driven turns) — (RESOLVED in 16-01 Task 1)
   - What we know: srv.Emitter(sessionID) exists for exactly this; class (fg/bg) must be selectable per handle.
   - Recommendation: two constructors (`Emitter` foreground, `BackgroundEmitter`) or a class parameter — planner picks; test both classes in the stress.
   - Resolution: both handle classes over ONE TurnEmitter — Server.Emitter(sessionID) returns the foreground-class handle; a background-class constructor routes to the bg lane (class selected per handle; each handle's methods enqueue into its own lane). Both classes are exercised in the -race stress per the recommendation.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | everything | ✓ | go.mod (existing repo builds green in Phase 15) | — |
| golangci-lint v2 | mise ci lint | ✓ | existing gate | — |
| Zed (live) | criterion 1 + criterion 4 live handshake verification | operator-side | per operator setup (Phase 15's 15-07 live check pattern: PENDING-OPERATOR-CONFIRMATION ledger) | deterministic Zed-client simulator test in-repo |
| ACP schema (reference artifact) | wire-shape goldens | ✓ fetched | zed-industries/agent-client-protocol @ main 2026-08-26 (/tmp/acp-schema/v1.json) | re-fetch; optionally vendor golden snippets into testdata |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (+ `-race`), project-standard table tests |
| Config file | none needed (per-package _test.go, repo convention) |
| Quick run command | `go test -race -count=1 ./internal/acp/... ./internal/session/... ./internal/acpserve/...` |
| Full suite command | `mise ci` (vet + lint + build + `go test -race -count=1 ./...` — .mise.toml:25-27) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ACP-03 | Emitter total order: fg preempts, bg FIFO, no cross-kind reorder, no drop under fake slow writer | unit (-race stress, D-04's mise ci lane) | `go test -race ./internal/acp/ -run TestTurnEmitterPriority -count=1` | ❌ Wave 0 (internal/acp/emitter_test.go) |
| ACP-03 | Stall detector fires loud (log + counter) when lane full > threshold | unit | `go test -race ./internal/acp/ -run TestTurnEmitterStall -count=1` | ❌ Wave 0 |
| ACP-03 | tool_call/update/plan/thought frames render in live Zed | manual-only (operator live check, 15-07 pattern) | live Zed session | n/a — WINDOWS ledger |
| ACP-03/registry | Response matched by id while notifications stream; no writer-mutex blocking | unit (-race) | `go test -race ./internal/acp/ -run TestRegistryConcurrentResolve -count=1` | ❌ Wave 0 |
| ACP-08 | Inbound response frame intercepted, zero spurious -32601 | unit | `go test ./internal/acp/ -run TestServeResponseRouting -count=1` | ❌ Wave 0 |
| registry/D-14 | timeout → one retry → fallback + sticky degrade; -32800 = immediate cancel | unit | `go test ./internal/acp/ -run TestRegistryTimeoutFallback -count=1` | ❌ Wave 0 |
| registry/D-19 | turn-death synthetic-cancel resolves pending + emits $/cancel_request | unit | `go test ./internal/acp/ -run TestRegistrySyntheticCancel -count=1` | ❌ Wave 0 |
| ACP-08 | initialize/new/load/resume advertise richer caps; golden vs v1 schema shapes | unit (golden) | `go test ./internal/acp/ -run TestInitializeCapabilities -count=1` | ❌ Wave 0 |
| ACP-08 | set_config_option: persist-then-apply, typed reject, full-list response, effective currentValue | unit | `go test ./internal/acpserve/ -run TestSetConfigOption -count=1` | ❌ Wave 0 |
| ACP-08/D-07 | config write 0600, atomic, failure leaves state untouched | unit | `go test ./internal/modelrouting/ -run TestConfigWrite -count=1` (or providerfactory home) | ❌ Wave 0 |
| ACP-03/D-20..23 | transcript: 3 new kinds append; unknown-kind tolerance on replay; redactor zero-calls for thinking | unit | `go test ./internal/session/ -run TestTranscriptNewKinds -count=1` | ❌ Wave 0 |
| soak (D-04) | adversarial multi-emitter soak (minutes) | eval-suite gated | env-flag pattern per .mise.toml eval lanes | ❌ later wave |

### Sampling Rate
- **Per task commit:** `go test -race -count=1 ./internal/acp/... ./internal/session/... ./internal/acpserve/...`
- **Per wave merge:** `mise ci`
- **Phase gate:** full suite green before `/gsd-verify-work`; live-Zed criteria via operator confirmation ledger.

### Wave 0 Gaps
- [ ] `internal/acp/emitter_test.go` — priority/order/no-drop/stall (REQ ACP-03)
- [ ] `internal/acp/request_registry_test.go` — concurrent resolve, timeout/retry/fallback, synthetic-cancel (REQ ACP-03 + D-14/D-19)
- [ ] `internal/acp/server_test.go` additions — response-frame interception, capability goldens (REQ ACP-08)
- [ ] `internal/session/transcript_newkinds_test.go` — additive kinds, tolerance, redaction-exclusion zero-call assertion (D-20..D-23)
- [ ] config-write tests at the writer's package home (D-07)

## Security Domain

`security_enforcement` is absent from `.planning/config.json` — treated as enabled.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Auth methods surface not in scope (empty `authMethods` remains) |
| V3 Session Management | yes (session ids, registry ids) | uuidV4 CSPRNG ids (newSessionID pattern, handlers.go:227-239); no sequential ids to guess |
| V4 Access Control | no | No new authority surfaces; config mutation is operator-scoped by design |
| V5 Input Validation | yes | D-09 typed reject of invalid option values; D-10 schema-tolerant `_meta`/blob parse (unknown keys SURVIVE but are never executed); inbound frames remain length-bounded by line framing |
| V6 Cryptography | no | No new crypto; CSPRNG reuse only |
| V14 Config | yes | D-07: 0600 config writes, persist-then-apply, no half-state; API keys NEVER options (ACP-08 — env/file only) |
| V7 Errors/Logging | yes | D-16 structured stderr logs + counters; stdout remains frames-only (transport discipline); scrubbed error responses unchanged (server.go:238) |

### Known Threat Patterns for ACP stdio server (Go)

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Credential leak via new transcript kinds | Information Disclosure | Redaction stays on every non-thinking path; thinking bytes exclusion is D-23's type-level distinct path — assert zero redactor calls, and never extend that path to other kinds |
| Config injection via editor settings | Tampering | Persist-then-apply with typed validation (D-07/D-09); blob values are defaults-of-last-resort (D-10) — never silent authority over file config |
| Writer stall → silent wedge | DoS | D-03: loud stall detector, ctx-aware producers, never auto-disconnect |
| Unbounded memory under chatty emitters | DoS | D-01 bounded lanes (block, never buffer-grow) + existing 256-slot Writer boundary |
| Response spoofing by id collision | Spoofing | Registry keyed by OUR generated uuidV4 ids (D-15); unknown-id responses logged, dropped |

## Sources

### Primary (HIGH confidence)
- zed-industries/agent-client-protocol `schema/v1/schema.json` @ main (downloaded + parsed 2026-08-26): SessionUpdate kinds, StopReason, ToolCall/ToolCallUpdate/ToolKind/ToolCallStatus, Plan/PlanEntry(+Priority/Status), ContentChunk, Diff, ToolCallLocation, AvailableCommandsUpdate/AvailableCommand, AgentCapabilities, SessionCapabilities, SessionConfigOption(+Select/Boolean/Category), NewSession/SetSessionConfigOption(+Response), CancelNotification, CancelRequestNotification, RequestId, ProtocolVersion, InitializeRequest/Response, Load/ResumeSession — every verbatim quote in this document traces here
- zed-industries/agent-client-protocol `schema/v2/schema.json` @ main: v2 vocabulary diff (plan_update, configId, tool_call_content_chunk, state_update, ToolCallStatus+cancelled)
- zed-industries/agent-client-protocol `docs/protocol/v1/cancellation.mdx`, `prompt-turn.mdx` (§Cancellation), `slash-commands.mdx`, `session-config-options.mdx`, `schema.mdx` — cancel contract, full-replacement semantics, config option rules, -32800
- zed-industries/zed `crates/agent_servers/src/acp.rs` (:663, :767-795, :993, :1303-1391) and `crates/acp_thread/src/acp_thread.rs` (:2610) — Zed V1, clientCapabilities, defaults application, plan rendering
- In-repo (Read this session): internal/acp/{server,handlers,types,framer}.go; internal/acpserve/acp_serve.go; internal/runtime/runtime.go (emitFor seam); internal/session/{transcript,manager,ask,transcript_writer}.go; internal/event/events.go; internal/provider/provider.go; internal/providerfactory/provider_factory.go; internal/modelrouting/{load.go,dispatch.go,defaults/config.yaml}; .mise.toml

### Secondary (MEDIUM confidence)
- zed.dev/docs/ai/external-agents + zed.dev/acp (orientation only; all load-bearing claims re-verified in source)

### Tertiary (LOW confidence)
- None used — no claim in this document rests on unverified web content

## Metadata

**Confidence breakdown:**
- Wire vocabulary/shapes: HIGH — verbatim from canonical schema, locally parsed; v1 confirmed as Zed's version in Zed source
- Cancel contract: HIGH — verbatim MUST/SHOULD text from official docs, cascade diagram included
- Emitter/registry architecture: HIGH — grounded in read source seams (framer.go, server.go dispatch, emitFor junction); pattern choice is prescriptive but the seams are verified
- Config write path: MEDIUM — the writer must be BUILT (verified absent); its shape follows repo conventions but is new design
- Zed stable-channel behavior: MEDIUM (A1) — main-branch verified; stable may lag

**Research date:** 2026-08-26
**Valid until:** 2026-09-25 (stable domain; re-check Zed's protocol version if Phase 18 slips past then — A2)
