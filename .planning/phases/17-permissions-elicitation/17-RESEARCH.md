# Phase 17: Permissions + Elicitation - Research

**Researched:** 2026-08-27
**Domain:** ACP permission requests + elicitation forms on the AskBroker suspension pattern; CC-parity permission rule grammar; the ONE gate chokepoint
**Confidence:** HIGH (wire shapes parsed from canonical schema; in-repo seams read this session; CC model from official docs)

## Project Constraints (from CLAUDE.md)

No `CLAUDE.md` exists at the repo root or `.claude/CLAUDE.md` (verified this session — both absent). No `.claude/skills/` or `.agents/skills/` directories exist. Project constraints come from `.planning/config.json` (tdd_mode, nyquist_validation on) and the GSD planning docs only.

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions

Verbatim from 17-CONTEXT.md. D-01..D-13 are LOCKED; deviations are not the planner's to make.

#### Permission Scope Taxonomy
- **D-01:** One "always allow" click persists tool×project into the project's permissions.yaml. Richer rules (arg patterns, path prefixes) exist in the FILE for hand-editing only — the dialog never writes them. Other projects re-ask.
- **D-02:** permissions.yaml carries the Claude-Code-parity rule grammar: deny → ask → allow, first match wins, deny beats any allow; exact + wildcard patterns; MCP tools namespaced `mcp__server__tool`. The dialog writes simple entries; the file supports the full grammar. — **Reversibility:** costly.
- **D-03:** Reject-also-always writes deny rules (same tool×project scope) — both directions from the dialog (CC "No, and don't ask again" precedent).
- **D-04:** Hooks are checked BEFORE permission rules (criterion 5's ordering; CC parity) — the chokepoint runs hook verdict first, then permission evaluation.

#### Gate Chokepoint & Tool Set
- **D-05:** ONE pipeline for both modes: hook verdict → permission ask → execute. Ungated (default) = the ask step evaluates rules and returns allow without ever opening a dialog — deny rules still deny. Mode-independent shape; both modes tested through the same chokepoint; criterion 4 (zero new dialogs on default config) holds by construction.
- **D-06:** The dialog set is DERIVED by the rule grammar: deny rules → deny; allow rules → allow; ask rules (DEFAULT = today's mutating/serialized tool class) → dialog; everything else (read-only concurrent class) → allow. The file tunes ask subjects per pattern; dialog fatigue has a relief valve short of always-allow.
- **D-07:** Automation/cron turns (no human present): deny + allow rules still enforced; ask-class tools DECLINE with a client-visible note — fail-safe: never a dialog nobody answers, never a silent allow. The declined-execution audit trail is visible in the next foreground session.

#### Elicitation Form Mapping
- **D-08:** Typed mapping table: single-choice → select (enum) field; multiSelect AskUserQuestion → array-of-enum field; free-text → text field; boolean → boolean field ONLY when the client advertised boolean support (ACP constraint — else yes/no string select); headers/labels → field titles. Every existing ask kind has a defined form rendering.
- **D-09:** The WHOLE ask family converts: model-initiated AskUserQuestion joins learning-store and engine asks in elicitation forms (same suspension, consistent editor UX). The v1.1 captured-shape plain-text route becomes the FALLBACK, not the primary. — extends ACP-02's letter by operator endorsement.
- **D-10:** Invalid elicitation response: the agent re-validates against the requested schema, re-asks ONCE with the violation noted; second failure routes exactly like a non-answer (12-D-01 corpus form). Bounded loop; the user always lands somewhere.

#### Dialog Serialization
- **D-11:** One ask outstanding at a time (never stacked modals). Queue order: FIFO within priority classes mirroring the emitter's fg/bg — the foreground turn's asks fire before subagent/engine asks; within a class, ask order preserved.
- **D-12:** The queue is visible: each enqueue emits a session/update note ("ask queued — N pending") and increments a counter (same counter family as the stall metrics). Criterion 2's "session stays alive and responsive" gets user-visible evidence.
- **D-13:** Queue entries carry their turn id: turn death resolves the OPEN dialog as cancelled (16-D-19 synthetic-cancel) AND drains that turn's queued-but-unfired asks as cancelled-normal immediately — no orphaned dialogs, no zombie asks firing for a dead turn.

### Claude's Discretion
- permissions.yaml concrete syntax (YAML lists vs rule strings) within the D-02 grammar.
- Pattern-matching semantics details (glob vs prefix) — CC-compatible prefix wildcards preferred.
- The pending-count note's exact wording and dedupe rate (avoid spam on rapid enqueues).
- Probe elicitation from 16-D-13's discreet replacement with a real form once mapping exists.

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description (from REQUIREMENTS.md) | Research Support |
|----|-------------------------------------|------------------|
| ACP-01 | User sees clickable permission asks via `session/request_permission` (allow/reject × once/always options; cancelled handled as a normal response when turn dies mid-ask); new `permissions.mode: ungated\|gated` config switchable via editor configOptions — available, not default (safety-model amendment) | Wire shapes verified verbatim from canonical v1 schema (§Wire Shapes); PermissionOptionKind enum is exactly allow_once/allow_always/reject_once/reject_always; cancel contract + synthetic-cancel cascade pinned; Zed renders the agent-supplied option list as native dialog buttons (§Zed Client Facts); chokepoint landing zone + suspension mechanics mapped (§Architecture Patterns) |
| ACP-02 | Learning-store and engine asks surface as structured forms via `elicitation/create` (form mode) with plain-text AskBroker fallback and -32601 probe-and-degrade on older clients; url mode deferred | ElicitationSchema/ElicitationPropertySchema/accept-decline-cancel verified verbatim (§Wire Shapes); RFD form-mode semantics fetched (flat primitive properties, oneOf titled selects); D-08 mapping table grounded against both the schema variants and the in-repo AskQuestion shape; fallback = today's onSurface bus-publish path (runtime.go:984-989) |

</phase_requirements>

## Summary

Phase 17 converts the agent's human-facing asks into editor-native surfaces, riding two Phase-16 primitives that **exist today only as plans** — the outbound request registry (16-03: `internal/acp/request_registry.go`, Call/Deliver/ResolveCancelled, HUMAN-ASK timeout class) and the config surface (16-05: `internal/acpserve/config_surface.go`, advertising `permissions.mode` as a select ungated|gated pending-handler no-op). STATE.md records Phase 16 as planned-but-not-executed (7 completed plans, all Phase 15); zero request_permission/elicitation/registry/permissions-mode code exists in the tree (verified by grep this session). Phase 17's plans must therefore code against the 16-plan contracts and re-verify anchor paths at execution time.

The load-bearing architecture fact: **the two ask families have different mechanics**. Permission asks are PRE-execution gates at a chokepoint that must sit in `session.runTurn`'s per-call tool loop — the same site as the existing plan-mode gate (`s.planModeBlocks(tc.Name)`, internal/session/session.go:469) — NOT inside the executor path, because `toolexec.DispatchBatch` wraps every call in a per-tool deadline (`executeBounded`, internal/toolexec/batch.go:210-232; default 120000ms, internal/toolcat/types.go:69) that would deadline-kill any human-scale wait, and because subagent Task calls bypass DispatchBatch entirely (session.go:423-461). Per the locked phase goal, the permission wait rides the suspension pattern so it never holds the per-session turn mutex (`sessionTurnMu`, internal/runtime/cron_wiring.go:28-37 — held by client Run, ask resumes, and automation firings for their whole turn). Elicitation asks keep today's AskBroker suspension verbatim and convert only the SURFACE: the plain-text `AgentMessageChunk` bus publish (runtime.go:984-989) becomes an `elicitation/create` form via the ask queue + registry, with today's path as fallback. A NEW ask QUEUE (D-11..D-13) must be built — today's AskBroker is single-pending (`pending *PendingAsk`, internal/session/ask.go:98), with no queue, no priority classes, and no turn-death drain.

The rule grammar's CC parity target is verified against the official docs: "Rules are evaluated in order: deny, then ask, then allow. The first match in that order determines the outcome, and rule specificity doesn't change the order" [CITED: code.claude.com/docs/en/permissions]. One nuance for the chokepoint documentation (criterion 5): CC's real hook ordering is two-layer — a blocking PreToolUse hook stops the call BEFORE permission rules are evaluated, while a hook's "allow" decision does NOT bypass deny/ask rules [CITED: same]. Our D-04 (hook verdict first) matches the blocking-hook precedent, and PAR-03's deny-only hook authority is deliberately stricter than CC (a project safety decision, not drift).

**Primary recommendation:** build the chokepoint as a gate seam invoked per-call inside `session.runTurn`'s tool-call loop (beside `planModeBlocks`), holding hook-verdict-then-permission-evaluation for BOTH modes; extend the ask machinery with a queue manager wrapping the AskBroker family (one outstanding, priority classes, turn-id drain); map the dialog/elicitation surfaces onto 16-03's registry with HUMAN-ASK class; persist permissions.yaml as CC-parity rule strings under `.ass-guard/` with the 0600 atomic-write discipline; and register the real `permissions.mode` handler behind 16-05's locked advertisement shape.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Gate chokepoint (hook verdict → permission ask → execute) | `internal/session` runTurn per-call loop | toolexec (unchanged dispatch) | The only site with turnID + callID + input + catalog class that sits ABOVE the per-call deadline wrap and sees subagent bypasses; D-05 forbids a second path |
| Permission rule grammar + permissions.yaml | new `internal/perm` (or session-owned file) | toolcat (Mutability class) | Pure evaluation + persistence; D-06 derives the default ask set from toolcat's mutating/serialized class, so the evaluator consumes toolcat |
| Ask queue (one-outstanding, priority, drain) | `internal/session` (AskBroker family extension) | runtime (wiring) | The broker is session-scoped today; subagent/engine asks are session-scoped emitters; the queue serializes dialog FIRING, not turn execution |
| Outbound permission/elicitation requests | `internal/acp` registry (16-03) | emitter foreground lane | Id'd requests, HUMAN-ASK timeout class, $/cancel_request cascade, sticky degradation — all locked by 16-03's plan |
| Elicitation form mapping (D-08 table) | new mapping code beside `RenderAskSurface` | coreexec (AskUserQuestion executor unchanged) | The executor's suspension contract (ErrSuspended + questions in Output) stays; only the surface renders differently |
| Structured reply → tool result | `internal/session` (ResolveAsk widening) | — | Today's reply is a plain string (ask.go:350); elicitation accept carries a content map that must render into the captured answered form |
| permissions.mode live flip | `internal/acpserve` ConfigSurface (16-05 shape) | session/chokepoint (consumer) | 16-05 advertises the pending option; Phase 17 registers the real handler and applies the mode to the running session's chokepoint |
| Engine/learning ask conversion | runtime advisory-note path | learning store (answer persistence) | Today ActionAsk → ErrAskPending → advisory note (runtime.go:546-548); the note surface becomes an elicitation form whose answer writes the learning entry |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib (`sync`, `context`, `time`, `encoding/json`, `os`, `path/filepath`) | go.mod toolchain | Queue mutex/condvar, ctx-aware waits, atomic YAML writes | Repo hand-rolls by precedent (framer, registry); zero new deps expected |
| `gopkg.in/yaml.v3` | v3.0.1 (go.mod:14, existing) | permissions.yaml read/write | Already the config-family parser (modelrouting); no new parsing stack |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| none new | — | — | — |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled JSON-Schema-subset validation for elicitation replies (D-10) | a JSON Schema library | The elicitation subset is tiny (type + enum/oneOf/anyOf + required + length bounds); the RFD's subset is closed; a dep buys nothing and the repo precedent is zero-dep |
| glob-matching library for rule patterns | stdlib `path.Match` + prefix logic | CC semantics are prefix-wildcard (`:*` == trailing space-star), not full glob; hand-rolling ~40 LOC matches CC parity exactly |

**Installation:** none — `go mod` unchanged.

**Version verification:** no registry lookups needed (zero new packages; yaml.v3 already in go.mod, verified this session).

## Package Legitimacy Audit

No external packages are installed by this phase (stdlib + existing go.mod only).

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| — (none) | — | — | — | — | — | N/A |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Wire Shapes (verified verbatim — the CONTEXT's canonical_refs checklist)

All quotes below are verbatim extractions from the canonical ACP v1 schema parsed THIS session (`/tmp/acp-schema/v1.json`, downloaded from `github.com/zed-industries/agent-client-protocol` @ main 2026-08-26 by Phase-16 research; re-parsed locally today). Tag: `[VERIFIED: /tmp/acp-schema/v1.json]`.

### session/request_permission (agent → client)

- Method marker: `x-method: "session/request_permission"`, `x-side: "client"` (client-handled).
- Request required fields: `"required": ["sessionId", "toolCall", "options"]` where `toolCall` is a **ToolCallUpdate** (the same wire shape as the streaming update — carry `toolCallId`, `title`, `kind`, `content` etc. so the dialog shows what it is authorizing).
- PermissionOption: `"required": ["optionId", "name", "kind"]`.
- PermissionOptionKind (the complete enum — exactly ACP-01's allow/reject × once/always):
  - `"allow_once"` — "Allow this operation only this time."
  - `"allow_always"` — "Allow this operation and remember the choice."
  - `"reject_once"` — "Reject this operation only this time."
  - `"reject_always"` — "Reject this operation and remember the choice."
- Response required: `["outcome"]`; either `{"outcome": "cancelled"}` or `{"outcome": "selected", "optionId": "…"}` (SelectedPermissionOutcome required: `["optionId"]`).
- Cancelled-outcome normative text (schema description, verbatim): "When a client sends a `session/cancel` notification to cancel an ongoing prompt turn, it MUST respond to all pending `session/request_permission` requests with this `Cancelled` outcome."

### elicitation/create (agent → client)

- CreateElicitationRequest: `message` (string, human-readable) + mode discriminator. Form mode: `"mode": "form"` + `requestedSchema` (**required**). Session scope: `sessionId` (+ optional `toolCallId` — "useful when an agent receives an elicitation from an MCP server during a tool call and needs to redirect it to the user"); request scope: `requestId`.
- ElicitationSchema: `type: "object"` (discriminator, default "object"), optional `title`/`description`, `properties` (map name → ElicitationPropertySchema), `required` (array of property names).
- ElicitationPropertySchema variants (schema description verbatim): "Single-select enums use the `String` variant with `enum` or `oneOf` set. Multi-select enums use the `Array` variant." Variants: `string` (+minLength/maxLength/pattern/format/default), `number`, `integer`, `boolean` (+default), `array` (multi-select: minItems/maxItems/items), `other` (`_`-prefixed, preserved not rendered).
- CreateElicitationResponse: `action` ∈ `"accept"` (carries `content`: object matching requestedSchema; values are ElicitationContentValue = `string | integer | number | boolean | array-of-string`), `"decline"`, `"cancel"`, `other`.
- RFD semantics [CITED: agentclientprotocol.com/rfds/elicitation]: flat primitive properties only; "use `oneOf` for titled single-select choices and `anyOf` for titled multi-select choices"; `enumNames` NOT supported; "clients SHOULD pre-populate form fields" from defaults; "Agents SHOULD also validate received data matches the requested schema, as defense-in-depth"; "Agents MUST NOT assume an elicitation succeeds"; `pattern` evaluation "MUST NOT be allowed to block Client UI or unboundedly consume resources" (bounded matcher); session-state binding "MUST NOT be associated with session IDs alone".

### Capability advertisement (verified in schema + Zed source)

- ElicitationCapabilities: `form: {}` / `url: {}` — "Supplying `{}` explicitly advertises form support"; omission = not advertised.
- Boolean CONFIG-OPTION gate [CITED: agentclientprotocol.com/protocol/v1/session-config-options]: "Agents MUST NOT include `type: "boolean"` options in `configOptions` payloads unless the Client advertised support" via `session.configOptions.boolean: {}`. **Scope note for D-08:** this gate applies to *config options*, not to elicitation form fields — the schema's BooleanPropertySchema variant carries no capability gate of its own. D-08's conservative reading (fall back to a yes/no string select when unadvertised) remains implementable and safe; Zed advertises `session.configOptions.boolean:{}` anyway (16-RESEARCH Zed fact #2). Implement D-08 as locked; know the wire does not demand the fallback.

## Zed Client Facts

1. **Zed renders request_permission natively, driven by OUR option list.** `handle_request_permission` (crates/agent_servers/src/acp.rs:4582-4625, fetched from main this session) routes through `thread.request_tool_call_authorization(args.tool_call, acp_thread::PermissionOptions::Flat(args.options), acp_thread::AuthorizationKind::PermissionGrant, cx)` — the dialog buttons come from the agent-supplied `options` array, so our four kinds (allow_once/allow_always/reject_once/reject_always) define Zed's dialog. Registered via `on_request!(handle_request_permission)` at acp.rs:711. [VERIFIED: raw.githubusercontent.com/zed-industries/zed/main/crates/agent_servers/src/acp.rs:4582-4625, fetched this session]
2. **Zed handles elicitation/create for both scopes**: Session scope → `thread.request_elicitation_with_id`; Request scope → a `request_elicitations` store; both cancellation-aware via `responder.cancellation()` (same fetch). [VERIFIED: same source, handle_create_elicitation]
3. **Stable-channel caveat**: main is verified; stable Zed may lag (16-RESEARCH assumption A1 carries forward). The 16-D-13/16-D-14 probe-and-degrade machinery covers older clients for elicitation; the permission dialog has existed since ~v0.219 (secondary source — zed-industries/zed discussion #48784 notes confirmation dialogs for tool permissions limited to allow/reject at that vintage; current main passes full option sets through). Live-Zed verification stays an operator-ledger item.
4. **Zed's own dialog cancellation**: on `ErrorCode::RequestCancelled` Zed calls `cancel_tool_call_authorization(&tool_call_id)` — the dialog closes when the request is cancelled (the client side of the $/cancel_request cascade). [VERIFIED: same source]

## Architecture Patterns

### System Architecture Diagram

```
                                session.runTurn tool-call loop (internal/session/session.go:407-577)
                                ┌────────────────────────────────────────────────────────────────┐
 model tool_calls ──────────────┤ per call:                                                         │
   [Read, Write, Task, …]       │  1. subagent? ── yes ── DispatchSubagent (bypasses batch)        │
                                │  2. s.planModeBlocks(name)? ── yes ── captured refusal result    │
                                │  3. ★ CHOKEPOINT (NEW, D-04/D-05):                               │
                                │       hook verdict (Phase 21 joins here; no-op head today)       │
                                │         │ deny ──► deny result, skip execute                     │
                                │         ▼                                                         │
                                │       permission evaluate (rule grammar, D-02/D-06):             │
                                │         deny rule ──► deny result, skip execute                   │
                                │         allow rule ──► execute (both modes)                      │
                                │         ungated + ask-class ──► execute (NO dialog, criterion 4)  │
                                │         gated + ask-class + human turn ──► SUSPEND:              │
                                │              PendingAsk{kind:"permission", tool, input, ids}     │
                                │              turn ends (stopAsk family) — sessionTurnMu RELEASED │
                                │         gated + ask-class + automation turn ──► DECLINE note (D-07)│
                                │         ▼                                                         │
                                │  4. survivors → toolexec.DispatchBatch (deadline-wrapped, unchanged)│
                                └────────────────────────────────────────────────────────────────┘
        suspend path                                        registry path (16-03, HUMAN-ASK class)
   ┌───────────────────────────┐                     ┌──────────────────────────────────────────┐
   │ ASK QUEUE (NEW, D-11..13) │── fire head ───────►│ Registry.Call(ctx, method, params,       │
   │ one outstanding           │                     │   HUMAN-ASK)                             │
   │ priority: fg turn >       │                     │ uuid id ─► frame ─► client ─► select{ch,  │
   │   subagent/engine, FIFO   │                     │   ctx, timer} ─► resolution              │
   │ entries carry turnID      │                     │ turn death: ResolveCancelled + emit       │
   │ enqueue ─► session/update │                     │   $/cancel_request{id} (16-D-19 cascade) │
   │   note + counter (D-12)   │                     └──────────────────────────────────────────┘
   │ turn death: drain open +  │
   │   queued as cancelled (D-13)│                   resolution fan-in (per ask type):
   └───────────┬───────────────┘                    ┌──────────────────────────────────────────┐
               │                                    │ permission: selected{optionId} ──►       │
   ask producers│                                    │   allow_always: persist rule THEN execute │
   ┌───────────┴───────────────┐                    │   allow_once: execute gated tool          │
   │ (a) permission gate       │                    │   reject_*: denial result (+rule on always)│
   │ (b) AskUserQuestion       │                    │   cancelled: cancelled-normal result      │
   │     (ErrSuspended, today) │                    │   ──► resume turn (runTurn, new variant   │
   │ (c) engine/learning asks  │                    │     that executes the gated call)         │
   │     (advisory, post-turn) │                    │ elicitation: accept{content} ──► validate │
   └───────────────────────────┘                    │   (D-10 re-ask once) ──► structured reply │
                                                    │   ──► ResolveAsk ──► answered form + resume│
   surface dispatch (per ask type):                 │   decline/cancel ──► non-answer (12-D-01) │
   permission  ──► session/request_permission       │ engine ask: accept ──► learning-store write│
   asks (b)(c) ──► elicitation/create (form mode)   └──────────────────────────────────────────┘
        └ degraded (older client / -32601 probe) ──► today's plain-text path:
              onSurface bus publish → AgentMessageChunk (runtime.go:984-989) / advisory note
```

Trace criterion 1: gated mode, model calls Write → chokepoint step 3 → ask-class (mutating) → suspend → queue fires `session/request_permission` with 4 options → Zed native dialog (agent options = dialog buttons) → user picks allow_always → rule persisted to `.ass-guard/permissions.yaml` (0600, atomic) → gated tool executes → result appended → turn resumes → next Write call hits the allow rule → no dialog.

### Recommended Project Structure

```
internal/perm/            (NEW — or session-owned; planner picks the home)
├── rules.go              # rule grammar: parse Tool / Tool(specifier), deny→ask→allow first-match,
│                         #   :* trailing wildcard, mcp__server__tool namespace, compound split
├── rules_test.go         # grammar table tests (CC-parity cases from §CC Rule Model below)
├── store.go              # permissions.yaml load/save (0600 atomic, tool×project scope)
└── store_test.go
internal/session/
├── gate.go               (NEW) # the chokepoint seam: hook-verdict head (no-op until Phase 21)
│                               #   + permission evaluation + suspend/decline/allow decisions
├── askqueue.go           (NEW) # D-11..D-13 queue: one-outstanding, priority classes,
│                               #   turn-id drain, enqueue notes + counter
├── ask.go                # + permission-kind PendingAsk, structured-reply seam widening
└── gate_test.go / askqueue_test.go
internal/acpserve/
├── ask_surface.go        (NEW) # surface dispatch: permission frame build (ToolCallUpdate),
│                               #   elicitation build (D-08 mapping), fallback routing
└── config_surface.go     # permissions.mode handler registration (16-05's pending slot → real)
internal/acp/
├── types.go              # + RequestPermissionFrame/ElicitationFrame shapes (v1 verbatim)
└── request_registry.go   # (16-03 delivers; Phase 17 only consumes)
```

(Exact split is planner's; the seams above are load-bearing.)

### Pattern 1: The chokepoint as a per-call gate seam (D-04/D-05)

**What:** one function invoked per tool call inside runTurn's loop, returning a gate verdict (execute / deny-with-result / suspend-for-permission / decline-with-note).
**When to use:** every batch-eligible call AND the subagent dispatch branch (both call sites funnel through it — subagents bypass DispatchBatch but must not bypass the gate).

```go
// Sketch — sits exactly where s.planModeBlocks sits today (session.go:469):
type gateVerdict struct {
    action  gateAction // gateExecute | gateDeny | gateSuspend | gateDeclineAutomation
    payload json.RawMessage // deny/decline result form when not executing
}
func (s *Session) gateCall(ctx context.Context, turnID, callID, tool string, input json.RawMessage) gateVerdict {
    // 1. hook verdict head (Phase 21 joins; today: implicit allow) — D-04
    // 2. rule evaluation (perm.Evaluate) — D-02/D-06
    // 3. mode + class + human-present decisions — D-05/D-07
}
```

**Why here and not lower:** (a) `executeBounded` (toolexec/batch.go:210-232) wraps EVERY dispatched call in a `timeout_ms` deadline (default `DefaultToolTimeoutMS int64 = 120000`, toolcat/types.go:69) — a human-scale wait inside the executor is deadline-killed; (b) the gate needs turnID + callID which the Stub seam deliberately does not carry (ask.go:17-19 comment); (c) subagent Task calls never reach DispatchBatch (session.go:423-461) so an executor-level gate would miss them.

### Pattern 2: Permission suspension + execute-on-resume (the new resume variant)

**What:** today's `resumeAskClaimed` (session/ask.go:368-416) only renders a result and re-enters runTurn. The permission variant must, on allow, EXECUTE the gated call first (through `s.toolExecOrStub()` with the call's own deadline wrap), append its result, then resume.

```go
// Sketch — mirror of resumeAskClaimed's shape for kind == PendingAskKindPermission:
// outcome selected{allow_*}: (persist rule if always) → out, isErr := exec gated call
//                            → appendToolResultLoud(turnID, callID, tool, boundedToolResult(out), isErr)
// outcome selected{reject_*}: (persist deny rule if always) → append denial form (isErr=true)
// outcome cancelled / turn death: append cancelled-normal form (NO error result — criterion 2)
// then: stop := s.runTurn(resumeCtx, turnID)  // the SAME turn's loop, like the ask resume
```

Persist-before-execute ordering for allow_always (recommended, mirrors 16-D-07 persist-then-apply): write the rule first; a write failure downgrades to allow_once semantics for THIS click (loud structured log) — the user's "always" intent must never silently widen if the file can't record it.

### Pattern 3: The ask queue (D-11..D-13) wrapping the broker family

**What:** a session-scoped queue manager in front of the ask SURFACE (not the suspension): producers enqueue ask entries (permission dialogs, AskUserQuestion surfaces, engine/learning asks); one fires at a time; entries carry turn id + priority class.

```go
type askEntry struct {
    turnID string
    class  askClass // foreground-turn | subagent/engine (D-11)
    seq    uint64   // FIFO within class
    fire   func(resolver askResolution) // sends via registry (HUMAN-ASK) or plain-text fallback
}
// Enqueue: append + session/update note ("ask queued — N pending") + counter (D-12, dedup rate = discretion)
// Turn death (cancel hook from handleSessionCancel / runner): resolve OPEN entry cancelled (16-D-19
//   registry ResolveCancelled + $/cancel_request cascade) + drain that turnID's queued entries
//   cancelled-normal immediately (D-13)
```

**Key boundary:** the queue serializes DIALOG FIRING (one modal at a time), never turn execution — a suspended turn holds no lock while its ask sits queued (criterion 2).

### Pattern 4: Elicitation surface conversion (D-08/D-09) with fallback

**What:** the surface dispatcher at the acpserve layer chooses, per ask and per sticky session capability (16-D-13/D-18): elicitation/create (form) when supported, else today's path verbatim.

D-08 mapping table, grounded in the verified schema variants and the in-repo `AskQuestion{Question, Header, Options[]{Label, Description}, MultiSelect}` (session/ask.go:56-63, read this session):

| Ask shape (in-repo) | Elicitation property (wire) | Schema basis [VERIFIED: /tmp/acp-schema/v1.json] |
|---|---|---|
| single-choice (Options, !MultiSelect) | `type:"string"` + `oneOf:[{const:Label, title:Label, description:Description}]` | StringPropertySchema: "When `enum` or `oneOf` is set, this represents a single-select enum" |
| MultiSelect | `type:"array"` + `items` (enum values or anyOf titled) | "Multi-select enums use the `Array` variant"; RFD: "anyOf for titled multi-select" |
| free-text | `type:"string"` (minLength/maxLength optional) | StringPropertySchema |
| boolean ask | `type:"boolean"` when boolean support advertised; else `type:"string"` + two-value oneOf (D-08 as locked) | BooleanPropertySchema variant; config-boolean gate is config-options-scoped (§Wire Shapes note) |
| Header / question text | property `title` / schema-level `title`+`message` | ElicitationSchema title; CreateElicitationRequest message |
| N questions | N properties (one per question), all in `required` | ElicitationSchema properties/required |

**Accept mapping:** `accept.content` (map field→value) → the structured reply seam → rendered into the captured answered form (today `RenderAskAnswered` pairs `"question"="answer"` per question with a single string, ask.go:281-297 — the widening must preserve the captured form for string-valued replies; multi-field replies serialize deterministically, e.g. field-ordered join — planner pins the exact rendering). `decline`/`cancel` → route exactly like the 12-D-01 non-answer (NOT D-10's re-ask — decline is an answer-shaped refusal, not a schema violation).

### Pattern 5: permissions.yaml (discretion item — recommended shape)

CC-parity rule STRINGS in three ordered lists, closest to the verified CC grammar:

```yaml
# .ass-guard/permissions.yaml (project-scoped, 0600, atomic temp+rename)
deny:
  - "Bash(rm -rf *)"        # Tool(specifier); :* == trailing " *" (CC parity)
  - "mcp__puppeteer"        # whole-server deny
ask:
  - "Write"                 # bare tool = all uses
allow:
  - "Bash(git *)"
  - "mcp__github__get_*"    # allow globs only after literal mcp__server__ prefix
```

Evaluation (D-02 + CC parity): first match across deny→ask→allow, specificity never reorders; a deny anywhere beats any allow; unmatched ask-class (mutating/serialized per `isAloneInSlot`, toolexec/batch.go:174-185) falls to the mode decision (gated→dialog, ungated→execute); unmatched read-only/concurrent class → allow (D-06).

### Anti-Patterns to Avoid

- **Gating inside the executor or DispatchBatch** — deadline-kill (120s default) + missing call identity + subagent bypass. The chokepoint is per-call in runTurn.
- **A second permission path for subagents or engine turns** — D-05's ONE pipeline is mode-independent AND call-source-independent; subagent dispatch and automation turns route through the same gate seam with different human-present/mode inputs.
- **Blocking on the registry while holding `sessionTurnMu`** — the suspension releases it; the resume re-acquires (the AskBroker/claim discipline already models this).
- **Treating elicitation decline/cancel as D-10 schema violations** — re-ask is for INVALID accept payloads only; decline/cancel are terminal routes to the non-answer form.
- **Writing the allow_always rule after executing** — a crash between click and write loses the trust decision the user believes they made; persist first (16-D-07 discipline).
- **Unbounded `pattern` matching in rule specifiers** — the RFD's ReDoS bound applies to elicitation schemas; apply the same bounded-matcher discipline to rule patterns.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Id'd outbound requests, timeouts, retry, synthetic-cancel | New pending-map machinery | 16-03's Registry (`Call`/`Deliver`/`ResolveCancelled`, HUMAN-ASK class) | The wire phase ships it complete; Phase 17 consumes (16-D-19's own instruction) |
| Capability probe + sticky degradation | Per-ask probing | 16-03's initialize-time probe + 16-D-18 sticky state | Locked; no mid-session re-probe |
| configOptions advertisement + set handler shape | New option plumbing | 16-05's ConfigSurface + the advertised `permissions.mode` slot | The shape is locked; Phase 17 registers the real handler behind it |
| Atomic 0600 config writes | New file-write code | 16-04's WriteLayerOption primitive / the transcript `filePermOwner = 0o600` discipline (internal/session/transcript.go:11, per 16-RESEARCH) | Repo artifact-family convention |
| UUID request/message ids | Fresh schemes | The newSessionID crypto/rand v4 pattern (handlers.go:227-239 per 16-RESEARCH) | Collision-by-construction across restarts (Phase 18's resume needs it) |

**Key insight:** every wire shape this phase emits is already decided by the canonical schema — copying field names verbatim (`toolCallId`, `optionId`, `requestedSchema`, camelCase throughout, `//nolint:tagliatelle` markers) is correctness, not style.

## Common Pitfalls

### Pitfall 1: The deadline wrap kills human-scale waits
**What goes wrong:** a permission gate implemented inside `RealExecutor.Execute` or DispatchBatch is wrapped by `executeBounded`'s per-call `context.WithTimeout(ctx, timeout_ms)` (batch.go:215) — default 120s — the dialog dies mid-thought with a timeout-classified error.
**Why it happens:** the deadline backstop is invisible at the executor interface.
**How to avoid:** the gate runs in runTurn's per-call loop BEFORE the batch (Pattern 1); the gated tool's EXECUTION (post-allow) enters the batch normally and keeps its deadline.
**Warning signs:** permission tests that pass with instant fake replies and flake with delayed ones.

### Pitfall 2: Holding the turn mutex through the wait
**What goes wrong:** the gate blocks the turn goroutine on the registry resolution while `Run` holds `sessionTurnMu` (cron_wiring.go:28-37) — every subsequent prompt, ask-resume, and automation firing queues behind a human; criterion 2's "nothing blocks holding the turn mutex" fails.
**Why it happens:** blocking wait is the intuitive implementation of "wait for the dialog".
**How to avoid:** suspend (Pattern 2) — the turn ends with the ask marker, the mutex releases, the resolution drives a fresh resume under the serve-lifetime ctx (the D-01 timer path already proves this shape works, ask.go:315-322).
**Warning signs:** a test where a second prompt cannot start while a (fake) dialog is open.

### Pitfall 3: Stacked modals / cross-type interleaving
**What goes wrong:** a permission dialog and an elicitation form (or two engine asks) are outstanding simultaneously — D-11's "never stacked modals" violated; Zed may bury one dialog behind another.
**Why it happens:** the AskBroker (single-pending, questions only) and the permission gate are separate machines firing independently.
**How to avoid:** ONE queue owns every ask's firing (Pattern 3); producers only enqueue.
**Warning signs:** any registry.Call for an ask method issued outside the queue's fire step.

### Pitfall 4: Orphaned dialogs on turn/session death
**What goes wrong:** turn dies (cancel, session close, process shutdown) with an open dialog or queued asks — the client dialog never closes (no $/cancel_request), the registry entry leaks until HUMAN-ASK timeout, and a zombie ask later fires for a dead turn.
**Why it happens:** suspension decouples the ask's lifetime from the turn's ctx.
**How to avoid:** D-13's turn-id-carrying drain wired to the cancel hooks that already exist (handleSessionCancel, handlers.go:153-182; Serve ctx drain from 16-03 Pitfall 8).
**Warning signs:** goroutine-leak reports at shutdown; dialogs surviving a killed turn in the live-Zed check.

### Pitfall 5: Ungated default silently widening
**What goes wrong:** criterion 4 reads as "do nothing when ungated" and deny rules or the ask-class evaluation never run — an ungated session with a hand-edited deny rule executes the denied tool.
**Why it happens:** conflating "no dialogs" with "no evaluation".
**How to avoid:** D-05 verbatim — ungated = the ask step evaluates rules and returns allow without a dialog; deny rules still deny; the decline-automation path (D-07) also runs in BOTH modes.
**Warning signs:** any gate test that only exercises gated mode.

### Pitfall 6: Elicitation reply losing the captured answered form
**What goes wrong:** the structured `accept.content` map renders into the transcript as raw JSON, breaking the capture-pinned `RenderAskAnswered` pairing (the mimicry discipline: the model's view of an answered question is the captured form).
**Why it happens:** ResolveAsk's reply is a plain string (ask.go:350) and the renderers quote it verbatim.
**How to avoid:** widen the reply seam with a deterministic string rendering for structured replies (planner pins the exact serialization); string-valued single-field replies must render byte-identically to today.
**Warning signs:** golden-test drift on the answered-form fixture (testdata/zcode-interactive-results.json).

### Pitfall 7: MCP tool names not matching the rule namespace
**What goes wrong:** rules written `mcp__server__tool` never match because the per-session catalog registers MCP tools under their advertised names (mcpHost.Register(sCatalog), runtime.go:950) — the namespace mapping is unspecified.
**Why it happens:** D-02 imports CC's namespace convention; the repo's MCP registration shape is independent.
**How to avoid:** the rule evaluator maps catalog names to/from the `mcp__<server>__<tool>` namespace via the session's MCP host knowledge; pin the mapping in a table test.
**Warning signs:** an mcp__ rule that matches nothing in tests with a registered fake MCP tool.

### Pitfall 8: Mode flip not reaching the running session
**What goes wrong:** `permissions.mode` set from the editor persists but the chokepoint keeps the boot-time mode until restart — criterion 4's "mode flips take effect on the running session" fails.
**Why it happens:** the gate reads a snapshot captured at session construction.
**How to avoid:** the gate reads the mode through a live accessor (atomic/mutex-guarded) owned by the same surface that handles set_config_option (16-05's apply-hook seam, its "live apply through the runner seam" artifact).
**Warning signs:** flip-then-gate test that passes only after session recreation.

### Pitfall 9: Automation decline invisible
**What goes wrong:** D-07's decline note goes to stderr only, or lands in no transcript — "the declined-execution audit trail is visible in the next foreground session" fails.
**Why it happens:** notes emitted off-turn have no bus subscriber (the 13-03 PATTERNS timing hazard, runtime.go:528-534).
**How to avoid:** the decline writes a transcript line (loud structured family) + the queue-note counter family (D-12), not just a log.
**Warning signs:** a cron-fired gated ask-class call whose decline appears nowhere in the next session's view.

## Code Examples

### Permission request frame (v1 verbatim field names)

```go
// Source: /tmp/acp-schema/v1.json defs RequestPermissionRequest, PermissionOption, ToolCallUpdate
type RequestPermissionFrame struct {
	SessionID string              `json:"sessionId"` // required
	ToolCall  ToolCallUpdateFrame `json:"toolCall"`  // required — the SAME shape as the streaming update
	Options   []PermissionOption  `json:"options"`   // required
}
type PermissionOption struct {
	OptionID string `json:"optionId"` // required
	Name     string `json:"name"`     // required — human label
	Kind     string `json:"kind"`     // required — "allow_once"|"allow_always"|"reject_once"|"reject_always"
}
// Response: {"outcome":"cancelled"} | {"outcome":"selected","optionId":"…"}
```

### Elicitation form (single-select + multi-select + text, from the D-08 mapping)

```go
// Source: /tmp/acp-schema/v1.json defs CreateElicitationRequest, ElicitationSchema, ElicitationPropertySchema
type ElicitationFormFrame struct {
	Message        string           `json:"message"`                    // required (human-readable)
	Mode           string           `json:"mode"`                       // "form"
	RequestedSchema ElicitationSchema `json:"requestedSchema"`          // required
	SessionID      string           `json:"sessionId,omitempty"`        // session scope (with optional ToolCallID)
	ToolCallID     string           `json:"toolCallId,omitempty"`
}
type ElicitationSchema struct {
	Type       string                            `json:"type"`       // "object"
	Title      string                            `json:"title,omitempty"`
	Properties map[string]json.RawMessage        `json:"properties"` // variant-tagged property schemas
	Required   []string                          `json:"required,omitempty"`
}
// A single-select property: {"type":"string","title":"Approach","oneOf":[
//   {"const":"ship it","title":"ship it","description":"…"}, …]}
// A multi-select property: {"type":"array","title":"Areas","items":{"type":"string","enum":[…]}}
```

### CC-parity rule evaluation (the verified order)

```text
// Source: code.claude.com/docs/en/permissions (verbatim semantics)
// "Rules are evaluated in order: deny, then ask, then allow. The first match
//  in that order determines the outcome, and rule specificity doesn't change
//  the order."
//
// Evaluate(toolName, input):
//   for r in deny  { if r.matches(toolName, input) { return DENY  } }
//   for r in ask   { if r.matches(toolName, input) { return ASK   } }
//   for r in allow { if r.matches(toolName, input) { return ALLOW } }
//   return UNMATCHED   // → mode decision (D-05/D-06)
// Cross-layer: a deny in ANY layer beats an allow in ANY layer (evaluate all
// denies first regardless of layer — CC: "deny rules from any scope are
// evaluated before allow rules").
```

## CC Rule Model (D-02's parity target — verified against official docs)

All [CITED: code.claude.com/docs/en/permissions, fetched 2026-08-27]:

- **Order:** "Rules are evaluated in order: deny, then ask, then allow. The first match in that order determines the outcome, and rule specificity doesn't change the order." A broad deny (`Bash(aws *)`) blocks even when a narrower allow (`Bash(aws s3 ls)`) also matches — "a deny rule can't carry allowlist exceptions". A matching ask rule prompts even when a more specific allow also matches.
- **Layers + precedence:** deny at any level cannot be overridden by any other level ("If a tool is denied at any level, no other level can allow it"); deny rules from any scope evaluate before allow rules from any scope.
- **Syntax:** `Tool` or `Tool(specifier)`; `Bash(*)` is equivalent to `Bash`; the `:*` suffix equals a trailing `" *"` wildcard (`Bash(ls:*)` == `Bash(ls *)`) and "is only recognized at the end of a pattern".
- **Compound commands:** CC splits on `&&`, `||`, `;`, `|`, `|&`, `&`, and newlines; "A rule must match each subcommand independently."
- **Tool-name globs:** deny/ask accept full-name globs (`mcp__*` matches every MCP tool); allow globs only after a literal `mcp__<server>__` prefix ("An unanchored allow glob such as `*`, `B*`, or `mcp__*` is skipped with a warning").
- **Bare-name deny:** removes the tool from the model's context entirely (schema-level); a scoped deny (`Bash(rm *)`) leaves the tool visible and blocks matching calls at execution.
- **Persistence target:** "Yes, and don't ask again" writes `.claude/settings.local.json` at the repo root (Bash: permanent per repository and command; file-modification approvals: session-only). OUR equivalent per D-01/D-03: `.ass-guard/permissions.yaml`, tool×project, both directions.
- **Hooks × permissions (criterion 5's documentation input):** "A blocking hook also takes precedence over allow rules. A hook that exits with code 2 stops the tool call before permission rules are evaluated" AND "Hook decisions don't bypass permission rules… a matching deny rule blocks the call, and a matching ask rule still prompts even when the hook returned `allow` or `ask`". Our D-04 (hook verdict first at the chokepoint) matches the blocking-hook precedent; PAR-03's deny-only hook authority (hooks can never grant allow) is intentionally stricter than CC.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Plain-text ask surfaces (v1.1 AskBroker → AgentMessageChunk) | elicitation/create form mode + session/request_permission (stabilized ACP v1) | ACP stabilization era | Phase 17's whole point; fallback keeps v1.1 behavior on older clients |
| Modes (`set_mode`) | configOptions supersede modes ("Modes will be removed in a future version") | stabilization | permissions.mode rides 16-05's SessionConfigOption select (v1 field `id`, not v2's `configId`) |
| No request cancellation | `$/cancel_request` + `-32800` + the cancelled outcome (stabilized; 16-RESEARCH State of the Art) | stabilized | D-13/16-D-19's cascade is spec-backed |
| CC file-edit approvals session-scoped | ass-guard D-01: one always-click persists tool×project both directions | this phase (deliberate divergence) | Simpler, blast-radius-scoped trust model; document as intentional non-parity |
| Elicitation enumNames (MCP legacy) | oneOf/anyOf titled choices; "ACP does not support MCP's deprecated `enumNames`" [CITED: RFD] | RFD | D-08's mapping uses oneOf/anyOf, never enumNames |

**Deprecated/outdated:** elicitation `url` mode is wire-defined but explicitly deferred (ACP-02); do not advertise `url` capability expectations.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Zed stable renders request_permission with agent-supplied options and elicitation forms as main does (main verified this session; stable may lag ~v0.219-era allow/reject-only dialogs) | Zed Client Facts | Probe/degrade covers elicitation; permission dialog with 4 options needs the live-Zed operator check (criterion 1) — flag, do not code around |
| A2 | The permission-suspension resume executes the gated tool via a new resume variant beside `resumeAskClaimed` (design recommendation — no code exists) | Pattern 2 | Planner may place it elsewhere; the seam facts (turnID/callID available, deadline wrap below, mutex released) hold regardless |
| A3 | permissions.yaml shape = CC-parity rule strings in deny/ask/allow lists (CONTEXT discretion item — recommended, not locked) | Pattern 5 | Operator may prefer structured YAML; grammar semantics unchanged either way |
| A4 | Phase 16 lands as planned — 16-03's Registry API (Call/Deliver/ResolveCancelled/HUMAN-ASK) and 16-05's ConfigSurface (permissions.mode pending slot, `_global/` twins) hold as written | throughout | Phase 17 plans must re-verify anchor paths at execution; any 16 deviation ripples into every section here |
| A5 | allow_always persists the rule BEFORE executing (persist-then-execute; recommendation mirroring 16-D-07) | Pattern 2 | A click-then-crash loses the recorded trust decision; ordering is cheap to flip if planner disagrees |
| A6 | The ask queue is session-scoped (subagent/engine asks are session-scoped emitters today) | Pattern 3 | If serve-scoped asks emerge (none found), the queue home moves to the serve layer — interface unchanged |
| A7 | Catalog MCP tool names map to `mcp__<server>__<tool>` via the session's MCP host knowledge (mapping table needed; repo registration shape not yet verified against CC's namespace) | Pitfall 7 | Rules written in CC namespace silently match nothing until the mapping exists |
| A8 | D-08's boolean-field capability gate is conservative over-spec (the wire schema's BooleanPropertySchema carries no gate; the verified gate is config-options-scoped) — implemented as locked regardless | Wire Shapes note | None — belt-and-suspenders; Zed advertises the capability anyway |
| A9 | Engine-ask elicitation replies persist to the learning store (answers persist as today — CONTEXT's Reusable Assets line) | Architecture Diagram | If engine asks need a different sink, only the resolution fan-in changes |
| A10 | Structured multi-field replies render deterministically into the captured answered form (exact serialization is planner's to pin; string-valued replies must stay byte-identical) | Pattern 4 | Golden-test drift on the answered-form fixture catches any deviation |

## Open Questions

1. **Where the "turn death" signal originates for a SUSPENDED permission ask**
   - What we know: 16-D-19 ships registry.ResolveCancelled + the $/cancel_request cascade; D-13 requires draining the open dialog + queued asks on turn death; `handleSessionCancel` (handlers.go:153-182) is the cancel hook that exists today; the suspended turn's own ctx is already gone when the prompt response was sent.
   - What's unclear: the precise event that marks a turn "dead" for queue-drain purposes once the turn goroutine has returned (cancel notification vs session close vs serve shutdown — likely all three, wired to the same drain).
   - Recommendation: implement the drain as one function called from all three teardown paths; test each.
2. **Does the subagent dispatch branch gate?**
   - What we know: subagent Task calls bypass DispatchBatch; the chokepoint site sees them; D-11 names "subagent asks" as a queue priority class (implying subagent-ORIGINATED asks exist), but nothing locks whether a subagent's TOOL calls pass the permission gate.
   - What's unclear: whether a Task dispatch is itself ask-class (it spawns arbitrary nested work).
   - Recommendation: gate it (ONE pipeline, D-05's letter) with the Task tool's own specifier; planner confirms against D-06's class derivation (`isAloneInSlot`).
3. **permissions.yaml vs the 16-05 pending-handler contract**
   - What we know: 16-05's plan says pending ids "log the pending-handler line, do not persist, return the set unchanged"; Phase 17's real handler persists (the mode is a real config choice).
   - What's unclear: nothing structural — but the handler must follow 16-D-07 persist-then-apply and the D-10 idempotence guard already coded for model/tier.
   - Recommendation: register the handler as a first-class sibling of tier/model in the ConfigSurface, reusing WriteLayerOption.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | everything | ✓ | go.mod (repo builds green) | — |
| golangci-lint v2 | mise ci lint | ✓ | existing gate | — |
| Zed (live) | criteria 1+3 native-dialog/form verification | operator-side | per operator setup | deterministic Zed-client simulator (16-06 ships one) + operator ledger (15-07 pattern) |
| ACP v1 schema (reference) | wire-shape goldens | ✓ | /tmp/acp-schema/v1.json (re-parsed this session) | re-fetch from zed-industries/agent-client-protocol |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (+ `-race`), project-standard table tests |
| Config file | none needed (per-package _test.go, repo convention) |
| Quick run command | `go test -race -count=1 ./internal/perm/... ./internal/session/... ./internal/acpserve/... ./internal/acp/...` |
| Full suite command | `mise ci` (vet + lint + build + `go test -race -count=1 ./...`) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ACP-01/D-02 | Rule grammar: deny→ask→allow first-match, specificity no-reorder, cross-layer deny-wins, `:*` wildcard, mcp__ namespace, compound split | unit (table) | `go test ./internal/perm/ -run TestRules -count=1` | ❌ Wave 0 |
| ACP-01/D-01/D-03 | permissions.yaml: load/persist round-trip, 0600, atomic, dialog-written entries (tool×project, both directions), restart survival | unit | `go test ./internal/perm/ -run TestStore -count=1` | ❌ Wave 0 |
| ACP-01/D-05 | Chokepoint: ungated+no-rules ⇒ execute, zero dialogs; ungated+deny-rule ⇒ deny; both modes through ONE path | unit | `go test ./internal/session/ -run TestGateChokepoint -count=1` | ❌ Wave 0 |
| ACP-01/crit 1 | Gated ask-class ⇒ request_permission frame with 4 options; allow_always persists then executes; next call no dialog | unit (fake registry) | `go test ./internal/session/ -run TestGatePermissionSuspend -count=1` | ❌ Wave 0 |
| ACP-01/crit 2 | Suspend releases sessionTurnMu (second prompt runs while dialog open); cancel mid-ask ⇒ cancelled-normal, dialog resolved via cascade; -race | unit (-race) | `go test -race ./internal/session/ -run TestGateTurnDeath -count=1` | ❌ Wave 0 |
| ACP-01/D-07 | Automation turn + ask-class ⇒ decline note + transcript line, never a dialog, never silent allow; deny/allow still enforced | unit | `go test ./internal/session/ -run TestGateAutomationDecline -count=1` | ❌ Wave 0 |
| ACP-01/crit 4 | permissions.mode flip via set_config_option reaches the running session's gate live | unit | `go test ./internal/acpserve/ -run TestPermissionsModeFlip -count=1` | ❌ Wave 0 |
| ACP-02/D-08 | Mapping table: every AskQuestion shape → correct ElicitationPropertySchema variant (oneOf/array/boolean/string) | unit (golden) | `go test ./internal/acpserve/ -run TestElicitationMapping -count=1` | ❌ Wave 0 |
| ACP-02/D-09 | AskUserQuestion + engine/learning asks route to elicitation on capable client; -32601/probe-absent ⇒ plain-text fallback (today's path) verbatim | unit | `go test ./internal/acpserve/ -run TestAskSurfaceDispatch -count=1` | ❌ Wave 0 |
| ACP-02/D-10 | Invalid accept payload ⇒ re-validate + ONE re-ask with violation noted; second failure ⇒ non-answer form; decline/cancel ⇒ non-answer directly (no re-ask) | unit | `go test ./internal/session/ -run TestElicitationRevalidation -count=1` | ❌ Wave 0 |
| D-11..D-13 | Queue: one outstanding, fg-before-bg, FIFO within class, enqueue note + counter, turn-death drain (open + queued) | unit (-race) | `go test -race ./internal/session/ -run TestAskQueue -count=1` | ❌ Wave 0 |
| crit 5 | Chokepoint doc exists; grep-level check that no second gate path exists (one call site family) | docs/grep | `grep -rn "planModeBlocks\|gateCall" internal/session/` review | n/a — plan task |
| crit 1+3 live | Native dialog + form rendering in real Zed | manual-only (operator ledger, 15-07 pattern) | live Zed session | n/a — WINDOWS ledger |

### Sampling Rate
- **Per task commit:** the quick command above for touched packages
- **Per wave merge:** `mise ci`
- **Phase gate:** full suite green before `/gsd-verify-work`; live-Zed criteria via operator confirmation ledger

### Wave 0 Gaps
- [ ] `internal/perm/rules_test.go` + `store_test.go` — grammar + persistence (REQ ACP-01)
- [ ] `internal/session/gate_test.go` — chokepoint both modes, suspend, automation decline, turn death (REQ ACP-01)
- [ ] `internal/session/askqueue_test.go` — serialization, priority, drain (-race) (D-11..D-13)
- [ ] `internal/session/elicitation_reply_test.go` — structured reply render, re-validation, decline/cancel routing (REQ ACP-02)
- [ ] `internal/acpserve/ask_surface_test.go` — D-08 mapping goldens, dispatch + fallback (REQ ACP-02)

## Security Domain

`security_enforcement` is absent from `.planning/config.json` — treated as enabled.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | No auth surfaces added |
| V3 Session Management | yes (ask/registry ids) | uuidV4 ids (existing pattern); queue entries bound to turn ids; elicitation state bound to connection + turn, not session id alone (RFD: "State MUST NOT be associated with session IDs alone") |
| V4 Access Control | **yes — the phase's core** | The permission gate IS an access-control enforcement point: deny-first ordering, cross-layer deny-wins, fail-safe unmatched-ask behavior, deny-only hook authority reserved for Phase 21 |
| V5 Input Validation | yes | D-10 re-validation of elicitation accept payloads against requestedSchema (RFD's defense-in-depth MUST-adjacent); D-09-typed reject (16-05) on invalid mode values; bounded pattern matching (ReDoS) |
| V6 Cryptography | no | No new crypto |
| V14 Config | yes | permissions.yaml 0600 atomic writes; persist-then-execute for trust decisions; API keys never touched |

### Known Threat Patterns for permission/elicitation surfaces (Go, ACP stdio)

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Permission bypass via rule-order bug | Elevation of Privilege | First-match deny→ask→allow with cross-layer deny-first; table tests pin the order; a matching ask always prompts even when allow also matches |
| Trust-state tampering (permissions.yaml) | Tampering | 0600 perms, atomic temp+rename, dialog writes only tool×project simple entries; richer grammar is hand-edit-only (D-01) — the file is operator-owned like the config layers |
| Forged elicitation accept payloads | Tampering | Agent-side schema re-validation (D-10, RFD defense-in-depth); violations re-ask once then non-answer — never executed |
| Dialog spoofing via id collision | Spoofing | Registry keyed by agent-generated uuidV4 ids (16-D-15); unknown-id responses logged + dropped (16-03) |
| Orphaned-dialog DoS on turn death | DoS | D-13 drain + 16-D-19 cascade + Serve ctx drain; HUMAN-ASK timeout as backstop, never the primary |
| ReDoS via rule/elicitation patterns | DoS | Bounded matchers (RFD's MUST on pattern evaluation); no user-supplied regex reaches unbounded engines |
| Automation silent-allow | Elevation of Privilege | D-07 fail-safe: ask-class declines loudly in no-human turns; deny/allow still enforced; decline is auditable |

## Sources

### Primary (HIGH confidence)
- `/tmp/acp-schema/v1.json` — canonical ACP v1 schema (source: `github.com/zed-industries/agent-client-protocol` @ main, downloaded 2026-08-26 by Phase-16 research), parsed THIS session with a local script: RequestPermissionRequest/Response, PermissionOption(+Id/Kind), RequestPermissionOutcome, SelectedPermissionOutcome, CreateElicitationRequest/Response, ElicitationSchema/SchemaType, ElicitationPropertySchema, StringPropertySchema, NumberPropertySchema, IntegerPropertySchema, BooleanPropertySchema, MultiSelectPropertySchema, MultiSelectItems, ElicitationSessionScope/RequestScope, ElicitationCapabilities(+Form/Url), ElicitationContentValue, ElicitationAcceptAction, CancelRequestNotification — every verbatim quote in §Wire Shapes traces here
- zed-industries/zed `crates/agent_servers/src/acp.rs` @ main (raw.githubusercontent fetch this session): `handle_request_permission` :4582-4625, `handle_create_elicitation` (both scopes), `on_request!` registration :711
- In-repo (Read/grep this session): internal/session/{ask,session}.go (AskBroker, PendingAsk, runTurn tool loop, planModeBlocks :469, DispatchSubagent bypass :423-461, stop vocabulary goconst_constants.go:7-8); internal/coreexec/ask.go (executor suspension, RenderAskSurface); internal/toolexec/{real,batch}.go (RealExecutor.Execute, DispatchBatch, executeBounded deadline wrap :210-232, isAloneInSlot :174-185); internal/toolcat/{types,catalog,mutability}.go (Tool struct, Mutability classes, DefaultToolTimeoutMS :69); internal/runtime/{runtime,cron_wiring}.go (askBroker wiring :984-990, advisory note :528-548/:1340-1344, mapAskStop :553-564, sessionTurnMu :28-37, runAutomationTurn :123, mcpHost.Register :950, RegisterCore site :972); internal/runtime/enginebridge/enginebridge.go (ACPDispatcher.Ask :418-427, ErrAskPending); internal/engine/{decide,observe}.go (ActionAsk, askSuspendedDecision); internal/learning/{types,store}.go (Entry, Lookup); internal/ecosys/hooks.go (HookRunner.PreToolUse :232-243); internal/acp/{handlers,server,types,framer}.go (handleSessionPrompt :102-147, handleSessionCancel :153-182, s.mu discipline); .planning/phases/16-acp-wire-foundation/{16-CONTEXT.md, 16-RESEARCH.md, 16-03-PLAN.md, 16-05-PLAN.md} (registry + config-surface contracts Phase 17 consumes); .mise.toml (ci/eval gates); go.mod (yaml.v3 :14)
- Code.claude.com/docs/en/permissions (official CC docs, fetched 2026-08-27, full text locally cached): rule order, layers, syntax, globs, compound split, persistence target, hooks×permissions — §CC Rule Model quotes

### Secondary (MEDIUM confidence)
- [Agent Client Protocol — elicitation RFD](https://agentclientprotocol.com/rfds/elicitation) — form-mode semantics, oneOf/anyOf, validation duties, security binding
- [ACP v1 session-config-options](https://agentclientprotocol.com/protocol/v1/session-config-options) — boolean capability gate (config-options scope)
- [Configure permissions — Claude Code docs](https://code.claude.com/docs/en/permissions) — CC parity target

### Tertiary (LOW confidence)
- WebSearch (Zed dialog rendering): [zed.dev/acp](https://zed.dev/acp), [Zed External Agents docs](https://zed.dev/docs/ai/external-agents), [zed-industries/zed discussion #48784 (ACP elicitation spec)](https://github.com/zed-industries/zed/discussions/48784) — orientation only; all load-bearing claims re-verified in Zed source or the canonical schema

## Metadata

**Confidence breakdown:**
- Wire shapes (request_permission, elicitation, cancel contract): HIGH — verbatim from the canonical schema parsed locally; Zed handling verified in Zed source
- Chokepoint architecture: HIGH — grounded in read source (deadline wrap, turn mutex, planModeBlocks precedent, subagent bypass); the placement recommendation is prescriptive but the seams are verified
- Suspension/queue design: MEDIUM — the AskBroker facts are verified; the queue and permission-resume variant are NEW design (no code exists) following locked decisions
- CC rule grammar: HIGH for the quoted semantics (official docs); the permissions.yaml shape is a discretion recommendation (A3)
- Phase-16 contracts: MEDIUM — plans are detailed but unexecuted (A4); re-verify at execution

**Research date:** 2026-08-27
**Valid until:** 2026-09-26 (re-check Zed stable's permission/elicitation rendering and Phase-16 landing state if planning slips past then)
