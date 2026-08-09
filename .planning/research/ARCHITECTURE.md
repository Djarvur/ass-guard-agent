# Architecture Research

**Domain:** Go-based SDD-hosting AI coding agent with mimicry, multi-tier model scheduling, configurable tool backends, learning-mode unified engine, hook-DAGs, and dual ACP+Telegram interfaces
**Researched:** 2026-08-09
**Confidence:** HIGH (inherited spine high-confidence from predecessor research; new deltas medium-high — interfaces fixed, internals to be validated during build)

> This document extends — and does not re-derive — the predecessor's architecture (`sdd-acp-agent`, technical research dated 2026-07-02). The predecessor's 6-component model, event bus, two-layer context, provider-shape isolation, and stdio discipline are treated as given and load-bearing. Read sections "Inherited Architecture" and "Where the Deltas Attach" first; everything after that is the ass-guard-specific layering on top.

---

## Inherited Architecture (carried forward, not re-derived)

The predecessor established a 6-component runtime that is the foundation for ass-guard. These are **unchanged in responsibility**; only some get new collaborators (see the deltas below).

| # | Component (inherited) | Owns | Boundary contract |
|---|---|---|---|
| 1 | **ACP Frontend** | stdin/stdout JSON-RPC framing; ACP method handlers | Inbound frames → Session Manager; outbound `session/update` from Event Bus |
| 2 | **Session Manager** | Durable replayable **transcript** (ACP-visible); **context-window projection** (model-visible); context-drop at boundaries | Transcript is source of truth; window is reproducible projection |
| 3 | **Turn Loop** | model↔tool execution for one turn; provider call, tool exec, streaming | Spawns subagents via Task; emits assistant stream to bus |
| 4 | **Tool Registry** | Built-in tool catalog (`Bash`, `Read`, `Write`, `Edit`, `Glob`, `Grep`, `WebSearch`, `WebFetch`, `Task`, `TodoWrite`, …); each tool `func(ctx, args) (result, error)` | Read-only tools parallelize; mutating tools serialize |
| 5 | **Subagent Manager** | Implements the `Task` tool; each subagent = goroutine running its own Turn Loop with isolated context + restricted tool subset | Fan-out via goroutines, fan-in via channels; panic-recover isolates failures |
| 6 | **Autocontinue Engine** | Tap on Event Bus; pattern-match → inject next turn | Observer only — never in a turn's critical path; failure = graceful degrade to manual-continue |

**Cross-cutting spine (also inherited):**
- **Internal Event Bus** — integration spine. Producers: Turn Loop, Subagent Manager, Tool Registry. Consumers: ACP Frontend (outbound), Autocontinue Engine, and now (new) the Unified Engine, Audit Log.
- **Two provider-shape adapters** (Anthropic-shape, OpenAI-shape) behind **one internal tool-call representation**. Tool-call translation is owned per-adapter.
- **Single-process, multi-goroutine**; stdout reserved for protocol, all logging to stderr.

These six are the floor. The ass-guard deltas build **above and beside** them without rewriting any.

---

## Where the Deltas Attach (executive summary)

The eight new architecture questions resolve to **five new components** plus **one internal rename/promotion**:

| Delta | Mechanism | New/changed component |
|---|---|---|
| (1) Mimicry/profiles | Shape outgoing requests *before* the provider adapter | **NEW: Profile Shaper** |
| (2) Multi-tier scheduling | Tier→model resolution with time-windows, per-project override, fallback chains | **NEW: Model Scheduler** |
| (3) Unified engine | Autocontinue is the upper mechanism; hooks are a special case; learning mode | **PROMOTED: Autocontinue Engine → Unified Engine** |
| (4) Hook-DAG executor | DAG of run-command/send-prompt/fresh-context/wait steps | **NEW: Hook-DAG Executor** |
| (5) Multi-interface (ACP + Telegram) | Interface Adapter above Session Manager | **NEW: Interface Adapter Layer** (with ACP Adapter + Telegram Adapter impls) |
| (6) Learning-mode persistence | Remember decisions; propose new hooks | **NEW: Learned Config (Memory)** |
| — | Reconstruct-exact-sequence audit | **NEW: Audit Log** |

**Total component count: 11** (6 inherited + 5 new + 1 promoted-to-unified counted within the 6). The Autocontinue Engine's responsibility expands into the Unified Engine — it is not a separate 12th component, it is the same component with a broader decision mandate.

The rest of this document specifies each new component's boundary, the end-to-end data flow with all new tap-in points, the dependency-ordered build sequence, and explicit interface boundaries.

---

## Standard Architecture

### System Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                       INTERFACE LAYER (peers)                             │
│   ┌──────────────────┐        ┌────────────────────────┐                  │
│   │   ACP Adapter    │        │   Telegram Adapter     │  (text + voice;  │
│   │ (stdio JSON-RPC) │        │  (HTTP long-poll/bot)  │   STT at edge)   │
│   └────────┬─────────┘        └───────────┬────────────┘                  │
│            │            Interface Adapter │                               │
└────────────┼──────────────────────────────────────────────────────────────┘
             │  (normalized Input/Output events, serialized per session)
             ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                         SESSION CORE                                      │
│   ┌──────────────────────────────────────────────────────────────────┐    │
│   │                     Session Manager                              │    │
│   │   (transcript + projection; owns per-session lock → input order) │    │
│   └──────────────────────────────┬───────────────────────────────────┘    │
│                                  │ active context window                  │
│                                  ▼                                        │
│   ┌──────────────────────────────────────────────────────────────────┐    │
│   │                          Turn Loop                                │    │
│   │   (model↔tool loop; spawns Subagent Manager for Task)             │    │
│   └────┬───────────────────────┬───────────────────────────┬──────────┘    │
│        │                       │                           │               │
│        ▼                       ▼                           ▼               │
│  ┌──────────┐         ┌─────────────────┐         ┌──────────────────┐     │
│  │ Profile  │         │ Model Scheduler │◀────────│  Tool Registry   │     │
│  │ Shaper   │         │  (tier→model,   │         │ (catalog +       │     │
│  │(system   │         │   time-windows, │         │  backends)       │     │
│  │ prompt,  │         │   per-project,  │         └──────────────────┘     │
│  │ catalog, │         │   fallback)     │                                  │
│  │ shape)   │         └────────┬────────┘                                  │
│  └────┬─────┘                  │ model spec                                 │
│       │ shaped request         ▼                                           │
│       │            ┌─────────────────────────┐                            │
│       └───────────▶│ Provider Adapters       │                            │
│                    │ (Anthropic / OpenAI)    │                            │
│                    └────────────┬────────────┘                            │
│                                 │ SSE stream                              │
└─────────────────────────────────┼─────────────────────────────────────────┘
                                  │
                                  ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                       EVENT BUS (spine)                                    │
│  Producers: Turn Loop, Subagent Manager, Tool Registry, Hook-DAG Exec     │
│  Consumers: Interface Adapters (outbound), Unified Engine, Audit Log      │
└───┬───────────────────┬─────────────────────────┬─────────────────────────┘
    │                   │                         │
    ▼                   ▼                         ▼
┌────────────┐  ┌──────────────────┐    ┌──────────────────┐
│  Unified   │  │  Hook-DAG        │    │   Audit Log      │
│  Engine    │─▶│  Executor        │    │  (reconstruct    │
│ (decision: │  │  (DAG of steps;  │    │   exact sequence)│
│  continue/ │  │  fresh-context)  │    └──────────────────┘
│  hook/ask/ │  └────────┬─────────┘
│  wait)     │           │
└─────┬──────┘           │ proposed-new-hooks / learned settings
      │ feedback          │
      ▼                   ▼
┌──────────────────────────────────────────┐
│   Learned Config (Memory)                │
│   (settings store + proposed-hook queue) │
└──────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Inherited or new |
|-----------|----------------|------------------|
| **ACP Adapter** | ACP stdio JSON-RPC frontend | Inherited (was "ACP Frontend", now an Interface Adapter impl) |
| **Telegram Adapter** | Telegram Bot API frontend; STT at the edge (voice → text) | **NEW** |
| **Session Manager** | Durable transcript + context-window projection; per-session serialization | Inherited (gains per-session lock for multi-interface inputs) |
| **Turn Loop** | One model↔tool turn; provider call, tool execution, streaming | Inherited (now receives shaped request + model spec) |
| **Tool Registry** | Built-in tool catalog; **configurable backends** per tool | Inherited (gains backend-config indirection) |
| **Subagent Manager** | `Task` tool → goroutine Turn Loops with isolated context | Inherited, unchanged |
| **Profile Shaper** | Apply active profile (system prompt, tool catalog projection, message shape, identity) to outgoing provider request, before the adapter | **NEW** |
| **Model Scheduler** | Resolve tier→model at request time; time-windows, per-project override, fallback chains | **NEW** |
| **Unified Engine** | Post-turn-complete decision: continue / trigger hook-DAG / ask user / wait; also drives learning mode. (Autocontinue is the upper mechanism; hooks are a special action type) | **PROMOTED** from Autocontinue Engine |
| **Hook-DAG Executor** | Runs a configurable DAG of steps (run-command / send-prompt / fresh-context / wait); owns its own context windows | **NEW** |
| **Learned Config (Memory)** | Persistent store of learned launch decisions and a queue of proposed-new-hooks awaiting user confirmation | **NEW** |
| **Audit Log** | Tap on Event Bus; full reconstruction-grade record of inputs, requests, tool calls, decisions | **NEW** |

---

## Where Each Delta Sits — Concrete Answers to the Eight Questions

### Q1 — Where does the mimicry/profile layer sit?

**Sits between the Turn Loop and the Provider Adapter, as a synchronous transform.** The Turn Loop prepares an internal "request intent" (messages, requested tier, intended tool subset) and hands it to the **Profile Shaper**, which produces the wire-shaped request the chosen provider adapter expects.

**New component: Profile Shaper.** Boundary:

```go
// internal/profile
type Shaper interface {
    // Apply takes the canonical internal request and produces a provider-shaped
    // request that is structurally indistinguishable from the active profile's
    // target agent. Called synchronously per turn; failure is a turn error.
    Apply(ctx context.Context, in InternalRequest, profile Profile) (ProviderRequest, error)
}

type Profile struct {
    ID            string
    SystemPrompts []PromptFragment   // identity + role
    ToolCatalog   []ToolSpec         // names + JSON schemas the model should see
    MessageShape  MessageMolder      // role/field ordering, wrapper conventions
    Identity      IdentityFields     // significant headers / client tags
}
```

- **Location:** between Turn Loop and Provider Adapters. Not between Profile and bus — shaping is request-direction only; response tokens flow back unmodified through the adapter and onto the bus.
- **Tool catalog projection:** the Shaper narrows the Tool Registry's full catalog to the profile's `ToolSpec` set. The Registry still owns execution; the Shaper only decides which tools the *model* sees.
- **Profile source:** extracted from the target agent's on-disk request logs (the PROJECT.md north-star). Profiles are **config**, not code; zcode is profile #1, N supported by design.
- **Mimicry verification hook:** the Audit Log records the *shaped* request verbatim, enabling a "diff against a recorded zcode request" assertion in tests. The Profile Shaper is the single place mimicry correctness is asserted.

### Q2 — Where does model scheduling sit?

**Separate component, not folded into the provider adapter.** New component **Model Scheduler** resolves tier→concrete-model at request time and owns fallback chains.

```go
// internal/scheduler
type Scheduler interface {
    // Resolve picks a concrete model+provider+baseURL for the requested tier,
    // honoring time-windows, per-project override, and walking the fallback
    // chain if the preferred entry is unavailable.
    Resolve(ctx context.Context, in ResolveInput) (Resolved, error)
    // Report lets the Turn Loop record a provider error so the Scheduler can
    // advance its fallback-chain cursor for the next attempt.
    Report(ctx context.Context, key ResolveKey, err ProviderError)
}

type ResolveInput struct {
    Tier        Tier          // heavy / good / light
    ProjectID   string        // for per-project override
    RequestID   string        // correlation
}

type Resolved struct {
    Provider   string         // "anthropic" | "openai"
    BaseURL    string
    ModelID    string
    Fallback   []FallbackStep // the chain the Turn Loop may walk on its own
}
```

- **Resolution inputs:** tier (chosen by command/skill/subagent), project id (per-project override table), wall-clock (time-windows). The default tier→model table + override + window + chain are all config.
- **Fallback interaction with the Turn Loop:** the Scheduler returns a *primary* resolution plus a *fallback chain*. The Turn Loop attempts the primary; on a provider error class the Scheduler flagged as fallback-eligible (rate limit, 5xx, no-key), the Turn Loop walks one step down the chain, *re-shapes* via the Profile Shaper (the chain may cross providers, so the request shape can change), and retries. The Turn Loop owns the retry loop body; the Scheduler owns *which model is next*.
- **Why separate from the adapter:** the adapter is shape-mechanical (Anthropic vs OpenAI wire format). Scheduling is policy (which model, when, on what failure). Mixing them couples policy to wire format and makes time-windows/fallback untestable in isolation. Separate = each testable, each swappable.

### Q3 — How does the unified engine extend the predecessor's Autocontinue Engine?

**The Autocontinue Engine is promoted to the Unified Engine.** Same siting (observer on the Event Bus), same safety property (no match → nothing runs), but the decision after turn-complete is broader than "inject next turn."

**Decision-point structure: a small rule engine, not a state machine.** The input is a turn-complete event carrying the accumulated assistant text, the active toolkit, the project, and the last handoff-tool-call (if any). The output is one of:

1. `ContinueNextTurn(text)` — the predecessor's behavior: synthesize the next user-turn and re-enter the Turn Loop. Dual-signal: text-pattern match OR a known handoff tool-call.
2. `TriggerHookDAG(hookID, vars)` — invoke the Hook-DAG Executor with a named hook and its variables (e.g. `post-implement` after an implement stage).
3. `AskUser(prompt)` — request user input via the active Interface Adapter (terminal answer to a question, or a Telegram reply).
4. `Wait(duration | condition)` — park the session, resumable by timeout or by an external wake event.
5. `Learn(unknown)` — only in learning mode: the engine has no rule for the situation; ask the user how to handle it, persist the answer, and act on it now.
6. `Stop` — no continuation; the scenario is complete or unmatched.

```go
// internal/engine
type UnifiedEngine interface {
    // OnTurnComplete is invoked by the Event Bus after every turn. It is the
    // single post-turn decision point. Hooks are a special case of Action.
    OnTurnComplete(ctx context.Context, ev TurnComplete) (Action, error)
}

type Action interface{ kind() string } // ContinueNextTurn | TriggerHookDAG | AskUser | Wait | Learn | Stop
```

- **Hooks as a special case:** `TriggerHookDAG` is just another Action kind. The engine does not treat hooks as a separate mechanism; the PROJECT.md delta (autocontinue is the upper mechanism; hooks are a special case) is enforced structurally.
- **Structural safety property preserved:** unmatched output still triggers nothing. The only off-switch is manual cancellation, which drains queued injections — inherited unchanged.
- **Learning mode plumbing:** when the engine emits `Learn`, it consults Learned Config first (maybe the answer exists). If not, it surfaces an AskUser via the Interface Adapter, persists the result, then converts the answer into the equivalent `ContinueNextTurn`/`Wait`/`TriggerHookDAG` action.

### Q4 — Hook-DAG Executor

**New component running a configurable DAG of steps.** Each "send-prompt" step **is** a turn (it re-enters the Turn Loop). The executor owns its own context windows via the "fresh-context" step.

```go
// internal/hookdag
type Executor interface {
    // Run executes a named DAG against the current session. Each send-prompt
    // node re-enters the Turn Loop; each fresh-context node resets the window;
    // each run-command node shells out; each wait node parks.
    Run(ctx context.Context, in RunInput) (RunResult, error)
}

type Step interface{ kind() string } // RunCommand | SendPrompt | FreshContext | Wait
```

- **Relation to the Turn Loop:** `SendPrompt` step = one turn. The executor does not bypass the Turn Loop; it calls into it. A DAG of three `SendPrompt` nodes is three sequential turns (or parallel branches → three goroutine Turn Loops, one per node).
- **Owns its own context windows:** the `FreshContext` step is explicit, not implicit. The executor asks the Session Manager to start a new projected window (transcript stays durable; only the window resets). This is the same two-layer mechanism the predecessor used for command boundaries; the executor reuses it rather than inventing a second path.
- **Concurrency model for parallel branches:** a DAG node with multiple out-edges fans out into goroutines (one per edge), each running its sub-DAG to completion; the join node waits on all. Same fan-out/fan-in shape as the Subagent Manager, scoped to DAG execution. Mutating side effects (run-command mutating the filesystem, mutating tool calls) must declare ordering dependencies in the DAG itself — the executor will not infer them.
- **Seeded hook set out of the box:** post-implement (test + lint + review + memory), post-phase (improvement proposals). These are config files shipped with the binary; learning-mode-proposed hooks land in the same store pending user confirmation.
- **Boundary:** the executor emits step/progress events onto the Event Bus (so the Audit Log records them and the Interface Adapter surfaces progress to the user). It never writes to the protocol directly.

### Q5 — Multi-interface: ACP + Telegram as peers

**New "Interface Adapter" abstraction above the Session Manager.** Both ACP and Telegram are full peers — both can drive an entire SDD scenario.

```go
// internal/interface
type Adapter interface {
    // In returns a stream of normalized user-input events (text turns,
    // cancellations). STT happens before this — the Adapter emits text only.
    In(ctx context.Context) <-chan InputEvent
    // Out consumes normalized outbound events (assistant stream, tool events,
    // AskUser prompts, progress) and renders them to the surface.
    Out(ctx context.Context) chan<- OutputEvent
}
```

- **ACP Adapter** is the renamed inherited ACP Frontend, now an Interface Adapter implementation. Transport: stdio JSON-RPC. Same ACP method set, same stdout=protocol/stderr=logs discipline.
- **Telegram Adapter** is the new peer. Transport: Telegram Bot API (HTTP long-poll or webhook). Renders assistant tokens as message edits; renders tool-call progress as status messages; surfaces `AskUser` actions as inline reply keyboards.
- **STT sits at the Telegram Adapter boundary, not in the core.** Voice messages are transcribed to text via a configurable STT backend *before* entering `In()`. The core never sees audio — it sees ordinary text input, indistinguishable from a typed message. This keeps STT a surface concern and lets the core remain interface-agnostic.
- **Concurrent input serialization:** each session has a single serialized input queue inside the Session Manager. Whether an input arrives from ACP or Telegram, it enters the same queue; turns run one-at-a-time per session. If Telegram sends a second input mid-turn, it queues (or is rejected with a "busy" reply — config). This avoids dual-interface races without a distributed lock; the Session Manager's per-session mutex is the single arbiter.

### Q6 — Learning-mode persistence

**New component: Learned Config (Memory).** Two distinct stores, one component:

```go
// internal/memory
type Learned interface {
    // LaunchDecision returns a remembered answer to a "how do I launch this?"
    // question (fresh-context? wait? how long?), or NotKnown.
    LaunchDecision(ctx context.Context, key LaunchKey) (LaunchDecision, error)
    RecordLaunchDecision(ctx context.Context, key LaunchKey, d LaunchDecision) error

    // ProposedHooks returns hooks the engine has proposed but the user hasn't
    // confirmed yet; ConfirmHook promotes one into the live hook table.
    ProposedHooks(ctx context.Context) ([]ProposedHook, error)
    RecordProposedHook(ctx context.Context, h ProposedHook) error
    ConfirmHook(ctx context.Context, id string) error
}
```

- **Launch decisions** are (situation-signature → action) entries the Unified Engine's `Learn` action writes after asking the user. The engine consults this store first; a hit short-circuits the ask.
- **Proposed hooks** are derived from the work log (the Audit Log is the source of "what just happened repeatedly"). The engine proposes; the user confirms via either Interface Adapter. Unconfirmed hooks never run — preserves the "no match → nothing runs" safety property.
- **Feedback into the Unified Engine:** at decision time the engine queries Learned Config first (launch decision) and writes to it on a confirmed learning. The hook table the executor runs is the union of seeded + user-confirmed hooks; proposed-but-unconfirmed hooks are *visible* to the user (so they can confirm them) but never executed.
- **Storage:** on-disk under the project's config namespace (Claude-Code-compat: `.claude/` layout, ass-guard additions namespaced cleanly). No external service; this is a local file store.

### Q7 — Audit log

**Dedicated component, fed as a tap on the Event Bus** (same shape as the Unified Engine — observer, not in the critical path).

```go
// internal/audit
type Logger interface {
    // Record appends a reconstruction-grade event. Never blocks producers;
    // internally buffered + flushed to disk.
    Record(ctx context.Context, ev Event)
}
```

- **Siting:** the Audit Log subscribes to the Event Bus like any other consumer. It is not a tap *on the bus implementation* (no special bus plumbing) — it is a regular consumer that happens to record everything.
- **Log schema for "reconstruct the exact sequence of actions":** each event carries:
  - `timestamp` (monotonic + wall-clock pair)
  - `session_id`, `turn_id`, `parent_turn_id` (subagents)
  - `kind` (input | shaped_request | provider_response_token | tool_call | tool_result | engine_decision | dag_step | dag_complete | output_rendered | cancel)
  - `payload` (the verbatim content; for `shaped_request`, the exact bytes the Profile Shaper produced — this is the mimicry-correctness evidence)
  - `source` (which component produced it)
- **Reconstruction property:** given the audit log for a session, an external reader can replay the exact input→request→response→tool→decision sequence without re-running the model. The `shaped_request` payloads are diff-able against recorded target-agent requests — this is how mimicry correctness is verified continuously.
- **Failure mode:** the Audit Log never blocks producers. If the disk fill or error, it logs to stderr and drops (or buffers, per config) — never breaks the agent.

### Q8 — Component count and build order

**Total component count: 11.** Six inherited (ACP Adapter [renamed from ACP Frontend], Session Manager, Turn Loop, Tool Registry, Subagent Manager, Unified Engine [promoted from Autocontinue Engine]) plus five new (Profile Shaper, Model Scheduler, Hook-DAG Executor, Interface Adapter Layer [which contains ACP + Telegram impls], Learned Config, Audit Log). Note: "Interface Adapter Layer" is one new abstraction; the ACP impl is the renamed inherited frontend, and the Telegram impl is new. Counted as one new layer-component plus its new Telegram impl, which keeps the new-component total at five.

**Dependency-ordered build sequence (mimicry thesis proven first):**

| # | Build item | Validates | Depends on |
|---|---|---|---|
| 1 | **Profile Shaper + Audit Log + thinnest possible Turn Loop (one provider adapter, one tier)** | The north star: outgoing requests structurally indistinguishable from zcode's. Audit Log enables the diff-against-recorded-zcode-request assertion. | nothing (this *is* the foundation) |
| 2 | **Session Manager (transcript + projection) wired to the Turn Loop** | Two-layer context; replay-on-load contract holds while window is resettable | #1 |
| 3 | **ACP Adapter (renamed frontend) + Interface Adapter abstraction** | ACP surface works end-to-end with mimicry-faithful requests; abstraction validates cleanly with one impl | #2 |
| 4 | **Model Scheduler (tiers, time-windows, per-project override, fallback chain)** | Multi-tier substitution; fallback retries flow back through the Profile Shaper correctly when crossing providers | #1, #3 |
| 5 | **Tool Registry with configurable backends + Subagent Manager** | Built-in catalog faithful to the profile; `Task` subagents isolated; WebSearch-style tools run via configurable backend | #3 |
| 6 | **Unified Engine (continue-action only first)** | Predecessor's autocontinue behavior reproduced inside the new decision-point shape | #3 |
| 7 | **Hook-DAG Executor + seeded hooks (post-implement, post-phase)** | The "forgotten routine" runs hands-off; send-prompt step re-enters Turn Loop correctly | #5, #6 |
| 8 | **Learned Config + learning mode in the Unified Engine** | Engine asks and remembers; proposed-hook flow works | #6, #7 |
| 9 | **Telegram Adapter (text first, then voice via STT at the edge)** | Second interface as a true peer; serialization of concurrent inputs is correct | #3, #6 |
| 10 | **Polish + distribution (goreleaser, ACP registry manifest, full audit-log replay tooling)** | Ship readiness | all |

The mimicry thesis (#1) is built before anything else. If it fails, the project stops and re-plans — this is the explicit, deliberate de-risking of the north star. Every later item assumes #1 is proven.

---

## Data Flow

### Request Flow (end-to-end with all new tap-in points)

```
   [User input via ACP or Telegram]
        │
        ▼
   Interface Adapter ──(STT at Telegram edge: voice → text)──▶ normalized InputEvent
        │
        ▼
   Session Manager ──(per-session lock serializes concurrent inputs)──▶ active context window
        │                                                       │
        │                                                       ▼
        │                                          (record input) ──▶ Audit Log
        ▼
   Turn Loop
        │
        ├─▶ 1. Build canonical InternalRequest (messages, requested Tier, tool subset)
        │
        ├─▶ 2. ★ NEW: Model Scheduler.Resolve(tier, projectID) → Resolved{provider, modelID, fallback}
        │
        ├─▶ 3. ★ NEW: Profile Shaper.Apply(InternalRequest, Profile) → ProviderRequest
        │             (system prompt + tool catalog projection + message shape + identity)
        │
        ├─▶ 4. ★ Audit Log records the verbatim ProviderRequest (mimicry evidence)
        │
        ├─▶ 5. Provider Adapter sends ProviderRequest → model
        │
        ├─▶ 6. Streamed tokens + tool_use requests arrive
        │             │
        │             ▼ published onto Event Bus
        │       ┌─────────────────────────────────────────────────┐
        │       │  Consumers (parallel):                          │
        │       │   • Interface Adapter.Out  (user sees tokens)   │
        │       │   • Audit Log              (records each)        │
        │       │   • Unified Engine         (accumulates text)    │
        │       └─────────────────────────────────────────────────┘
        │
        ├─▶ 7. For each tool_use: Tool Registry executes
        │             │
        │             ▼ (read-only tools parallelize; mutating serialize)
        │       backend resolution (e.g. WebSearch → configured backend)
        │             │
        │             ▼ tool result published onto Event Bus
        │
        └─▶ 8. Model stops requesting tools → turn completes
                  │
                  ▼ TurnComplete event on Event Bus
        ╔═══════════════════════════════════════════════════════╗
        ║   ★ NEW: Unified Engine decision (post-turn-complete) ║
        ║                                                       ║
        ║   consult Learned Config ──┐                          ║
        ║                            ▼                          ║
        ║   match text-pattern  ──▶ ContinueNextTurn           ║
        ║   known handoff call  ──▶ ContinueNextTurn           ║
        ║   hook-DAG trigger    ──▶ TriggerHookDAG             ║
        ║   needs user input    ──▶ AskUser (via Adapter)      ║
        ║   should wait         ──▶ Wait                       ║
        ║   unknown + learning  ──▶ Learn (ask + persist)      ║
        ║   nothing matched     ──▶ Stop                       ║
        ╚═══════════════════════════════════════════════════════╝
                  │
                  ├── ContinueNextTurn ──▶ re-enter Session Manager (synthesized input) ──▶ Turn Loop
                  │
                  ├── TriggerHookDAG ──▶ ★ NEW: Hook-DAG Executor.Run(hookID, vars)
                  │        │
                  │        ├── RunCommand step ──▶ shell out; stdout into context
                  │        ├── SendPrompt step ──▶ re-enter Turn Loop (one turn per node)
                  │        ├── FreshContext step ──▶ Session Manager resets window
                  │        └── Wait step ──▶ park; resumable
                  │        │
                  │        └── progress events onto Event Bus (→ Audit Log + Interface)
                  │
                  ├── AskUser ──▶ Interface Adapter.Out renders the question; awaits In
                  │
                  └── Wait ──▶ timer / external wake ──▶ resume decision
```

### State Management

```
              ┌──────────────────────────────────────────────┐
              │           Session Manager (per session)       │
              │  ┌────────────────────────────────────────┐  │
              │  │  Transcript (durable, ACP-visible)     │  │
              │  │  append-only; replayable on load       │  │
              │  └────────────────────────────────────────┘  │
              │  ┌────────────────────────────────────────┐  │
              │  │  Active Context Window (model-visible) │  │
              │  │  projection of transcript; reset at:   │  │
              │  │   • mutating-toolkit-command boundary  │  │
              │  │   • DAG FreshContext step              │  │
              │  └────────────────────────────────────────┘  │
              └──────────────────────────────────────────────┘
                              │
                              ▼ (mutating-toolkit boundaries are non-removable; config may only add)
        ┌─────────────────────────────────────────────────────────┐
        │  Unified Engine decision state                          │
        │   per-session: accumulated assistant text, last         │
        │   handoff tool-call, active toolkit, learning flag      │
        └─────────────────────────────────────────────────────────┘
                              │
                              ▼
        ┌─────────────────────────────────────────────────────────┐
        │  Learned Config (cross-session, persistent)             │
        │   launch decisions; proposed hooks (pending confirm)    │
        └─────────────────────────────────────────────────────────┘
                              │
                              ▼
        ┌─────────────────────────────────────────────────────────┐
        │  Audit Log (append-only, per session)                   │
        │   every event, verbatim shaped requests, full replay    │
        └─────────────────────────────────────────────────────────┘
```

### Key Data Flows

1. **Mimicry flow (request direction):** Turn Loop → Model Scheduler (resolve tier) → Profile Shaper (apply profile) → Audit Log (record verbatim) → Provider Adapter → model. The Shaper is the single chokepoint where mimicry correctness is asserted; the Audit Log is the single source of mimicry evidence.
2. **Stream flow (response direction):** provider SSE → Event Bus → fan-out to Interface Adapter (user-visible), Audit Log (record), Unified Engine (accumulate). Same fan-out shape as the predecessor — only the consumer set grew.
3. **Decision flow (post-turn):** TurnComplete → Unified Engine → Action. Each Action kind has a single, distinct sink (Session Manager, Hook-DAG Executor, Interface Adapter, or timer). Hooks are not a separate decision path; they are one Action kind.
4. **Learning flow:** Unified Engine emits `Learn` → consults Learned Config → on miss, asks via Interface Adapter → user answer writes Learned Config → answer translates to a concrete Action.
5. **Multi-interface serialization:** any input from any Adapter → Session Manager's per-session input queue → one turn at a time. The Adapter abstraction means the core is unaware whether input came from ACP or Telegram; serialization is centralized.
6. **Fallback flow:** Turn Loop hits a fallback-eligible provider error → reports to Model Scheduler → Scheduler advances fallback cursor → Turn Loop receives next resolution → re-enters Profile Shaper (the chain may cross providers, so shaping repeats) → retries.

---

## Recommended Project Structure

```
ass-guard-agent/
├── cmd/
│   └── ass-guard/              # main package; wires components, starts Interface Adapters
├── internal/
│   ├── interface/              # ★ NEW layer
│   │   ├── adapter.go          # Adapter interface (In/Out channels)
│   │   ├── acp/                # ACP Adapter (renamed inherited frontend)
│   │   └── telegram/           # ★ NEW Telegram Adapter + STT at edge
│   ├── session/                # Session Manager (transcript + projection; per-session lock)
│   ├── turnloop/               # Turn Loop (model↔tool; calls Shaper, Scheduler)
│   ├── profile/                # ★ NEW Profile Shaper + Profile definitions
│   ├── scheduler/              # ★ NEW Model Scheduler (tiers, windows, fallback)
│   ├── provider/               # Provider Adapters (Anthropic-shape, OpenAI-shape)
│   ├── tools/                  # Tool Registry + tool implementations
│   │   └── backends/           # ★ NEW configurable backends per complex tool
│   ├── subagent/               # Subagent Manager (Task tool → goroutine Turn Loops)
│   ├── engine/                 # ★ PROMOTED Unified Engine (was autocontinue)
│   ├── hookdag/                # ★ NEW Hook-DAG Executor + step types
│   ├── memory/                 # ★ NEW Learned Config (launch decisions + proposed hooks)
│   ├── audit/                  # ★ NEW Audit Log (Event Bus tap)
│   ├── bus/                    # Internal Event Bus (inherited spine)
│   └── config/                 # Config loading (Claude-Code-compat layout, ass-guard namespace)
├── config/
│   ├── profiles/               # zcode.toml (first profile), future profiles
│   ├── hooks/                  # seeded hook DAGs (post-implement, post-phase)
│   └── models.toml             # tier→model table, time-windows, per-project overrides
├── testdata/                   # recorded zcode requests (mimicry fixtures); ACP replay fixtures
├── .planning/                  # PROJECT.md, research (this file), specs
└── go.mod
```

### Structure Rationale

- **`internal/`:** everything is implementation-private; only `cmd/ass-guard` is the entry point. This is idiomatic Go and matches the predecessor.
- **`interface/` as a layer package (not `acp/` at the root):** signals that ACP is one of N peers. Adding a third interface later (web, CLI) lands here without restructuring.
- **`profile/`, `scheduler/`, `hookdag/`, `memory/`, `audit/` as peers:** each new delta is one cohesive package with a narrow interface — same shape as the inherited components. No god-package.
- **`tools/backends/` subpackage:** isolates the configurable-backend indirection. A tool that wants a pluggable backend depends on a backend interface defined next to the tool, with implementations registered in `backends/`.
- **`config/profiles/` and `config/hooks/`:** profiles and hook-DAGs are *data*, not code. Keeping them as config files makes team-sharing and learning-mode hook proposals first-class (proposed hooks land as files pending confirmation).
- **`testdata/` carries recorded zcode requests:** these are the mimicry-correctness fixtures. They are load-bearing test assets, not examples.

---

## Architectural Patterns

### Pattern 1: Observer-not-controller (inherited, broadened)

**What:** the dangerous/complex thing is an observer/projection/adapter. The canonical path stays simple. The predecessor applied this to the Autocontinue Engine (event-bus tap, never in the critical path). ass-guard broadens it: the Unified Engine, the Audit Log, and the Profile Shaper's tool-catalog projection are all observers/projections.

**When to use:** whenever a cross-cutting concern (continuation, audit, mimicry projection) could be woven into the Turn Loop. Don't. Tap the bus or transform at the boundary.

**Trade-offs:** pros — graceful degradation (any observer can fail without breaking turns), independently testable, swappable. Cons — ordering across observers must be specified (the bus guarantees per-turn order; cross-observer causality is by design not enforced — observers must be independent).

### Pattern 2: Boundary-transform isolation (new — for mimicry)

**What:** structural indistinguishability is enforced at exactly one boundary (Profile Shaper → Provider Adapter), with one source of evidence (Audit Log's verbatim `shaped_request` records). The Turn Loop speaks a canonical internal representation; mimicry is a transform *on the way out*.

**When to use:** any "X must look like Y to an external observer" requirement. Isolate the shaping in one place, record the output, diff against a reference. Never let mimicry leak into the Turn Loop's internals.

**Trade-offs:** pros — mimicry correctness is locally verifiable, profile-swappable by design (N profiles), Turn Loop stays provider-agnostic. Cons — every profile must be authored/maintained; the boundary interface must be stable enough to survive profile additions.

### Pattern 3: Policy/mechanism separation (new — for scheduling)

**What:** the Provider Adapter is mechanism (wire format); the Model Scheduler is policy (which model, when, on what failure). They are separate components with a narrow contract.

**When to use:** any time a wire-format concern and a routing/scheduling concern could be conflated. Keep them separate so each is testable in isolation.

**Trade-offs:** pros — fallback chains, time-windows, and per-project overrides are testable without HTTP; adapters are testable without policy. Cons — one extra hop on the request path; the Scheduler must return enough context (the fallback chain) for the Turn Loop to retry without re-consulting.

### Pattern 4: Hooks-as-action-kind (new — for the unified engine)

**What:** the post-turn decision is a single rule engine emitting typed Actions. `TriggerHookDAG` is one Action kind among five. The Unified Engine does not have a separate "hook trigger path" — it has a decision point that can choose to trigger a hook.

**When to use:** when collapsing two predecessor mechanisms (autocontinue + hooks) into one. A single decision point with typed outputs beats two parallel mechanisms with overlapping concerns.

**Trade-offs:** pros — one place to reason about post-turn behavior; learning mode plugs into the same decision point; safety property ("no match → nothing") is preserved uniformly. Cons — the Action union must stay disciplined; allowing ad-hoc Action kinds re-creates the two-mechanism problem.

### Pattern 5: Interface Adapter above Session Manager (new — for multi-surface)

**What:** a normalized Input/Output channel abstraction sits between any surface (ACP, Telegram, future) and the Session Manager. The core is surface-agnostic.

**When to use:** any time more than one surface must drive the same session core, especially with mixed modalities (text + voice).

**Trade-offs:** pros — surfaces are independently developable; STT and rendering stay at the edge. Cons — concurrent-input serialization must be centralized (Session Manager's per-session queue), or surfaces race.

---

## Scaling Considerations

"Scale" here is local-concurrency and per-project complexity, not horizontal distribution. The agent is a single binary subprocess; it never multiplies across machines.

| Scale dimension | Architecture adjustment |
|-----------------|------------------------|
| Sessions per process | Bounded by model-API concurrency, not by the runtime. A semaphore on outbound provider calls (inherited) prevents self-DDoS. Per-session state is small (transcript on disk, window in memory). |
| Tool fan-out per turn | Read-only tools parallelize freely (inherited). Mutating tools serialize. The Tool Registry enforces this; no per-tool tuning needed. |
| Subagent fan-out | One goroutine per subagent; bounded by a configurable concurrency limit. Fan-in via channels (inherited). |
| Hook-DAG parallelism | One goroutine per parallel DAG branch; join nodes wait. Mutating side-effect ordering must be expressed as DAG dependencies. |
| Audit Log volume | Buffered + batched flush. On overflow, configurable: drop or back-pressure. Never blocks producers. |
| Profile count | Profiles are config; N supported by design. Only one active per session. |

### Scaling Priorities

1. **First bottleneck: model-API concurrency.** Hits long before CPU or memory. Mitigation: the outbound semaphore (inherited) plus tier-aware scheduling (heavy models cost more, light models cheap) — the Model Scheduler naturally biases toward cheaper tiers for fan-out work.
2. **Second bottleneck: context-window growth within a long scenario.** Mitigated structurally by the two-layer model (window is a projection; transcript on disk). The Hook-DAG's FreshContext step is the explicit reset trigger for "forgotten routine" sub-workflows that would otherwise bloat the window.

---

## Anti-Patterns

### Anti-Pattern 1: Weaving mimicry into the Turn Loop

**What people do:** inline system-prompt assembly and tool-catalog filtering inside the Turn Loop's request-building code, because "it's just a few fields."
**Why it's wrong:** mimicry correctness becomes implicit, untestable in isolation, and impossible to diff against a reference. Profile-swapping requires editing the Turn Loop.
**Do this instead:** Turn Loop produces a canonical InternalRequest; the Profile Shaper applies the profile at the boundary. The Audit Log records the verbatim result for diffing.

### Anti-Pattern 2: Folding scheduling into the provider adapter

**What people do:** put tier→model resolution and fallback logic inside the Anthropic/OpenAI adapter, because "the adapter knows the provider."
**Why it's wrong:** time-windows, per-project overrides, and fallback chains are policy, not wire format. Mixing them couples policy to adapter internals and makes both untestable.
**Do this instead:** separate Model Scheduler (policy) from Provider Adapter (mechanism). The Scheduler resolves; the Adapter transmits.

### Anti-Pattern 3: Two parallel post-turn mechanisms (autocontinue + hooks)

**What people do:** keep the predecessor's Autocontinue Engine as-is and add a separate "hook runner" that also fires after turn-complete.
**Why it's wrong:** two mechanisms with overlapping triggers, undefined precedence, and double the surface area for the safety property. Learning mode has nowhere clean to plug in.
**Do this instead:** one Unified Engine, one decision point, typed Actions. `TriggerHookDAG` is an Action kind.

### Anti-Pattern 4: Letting STT leak into the core

**What people do:** pass audio or a "voice input" flag through the Session Manager and Turn Loop because "the model should know."
**Why it's wrong:** the core becomes surface-aware; every new modality requires core changes; voice and text diverge in behavior.
**Do this instead:** STT at the Telegram Adapter boundary. The core sees text only. Voice is just a way the user typed.

### Anti-Pattern 5: Skipping the mimicry-first build order

**What people do:** build all of ACP + tools + autocontinue first (because it feels like progress), then bolt mimicry on at the end.
**Why it's wrong:** if mimicry fails, everything built on top is compromised. The north star is unvalidated exactly when the most code depends on it.
**Do this instead:** build the Profile Shaper + Audit Log + thinnest Turn Loop first. Prove the thesis. Then build downstream.

### Anti-Pattern 6: Audit log as an afterthought

**What people do:** add logging "later," scattered across components, each choosing what to record.
**Why it's wrong:** the "reconstruct the exact sequence" property is unachievable retroactively; mimicry evidence is incomplete.
**Do this instead:** Audit Log as a first-class Event Bus consumer, built alongside the Profile Shaper (item #1 in the build order).

---

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Model provider (Anthropic-shape: GLM via Z.ai, etc.) | HTTP/SSE via `anthropic-sdk-go`, base URL configurable | Fallback chain may cross to OpenAI-shape; re-shaping required on switch |
| Model provider (OpenAI-shape: MiniMax M3, etc.) | HTTP/SSE via `go-openai`, base URL configurable | Tool-call translation owned by this adapter |
| Telegram Bot API | HTTP long-poll or webhook; Telegram Adapter owns transport | STT backend invoked at this boundary for voice messages |
| STT backend | Pluggable interface, configurable (specific providers out of scope for v1 per PROJECT.md) | Voice → text before entering the Adapter's In stream |
| SDD toolkit (OpenSpec) | Shell subprocess via toolkit adapter; stdout into context | v1 = OpenSpec only; GSD/spec-kit/BMad interface-accommodated, deferred |
| Web search backend | Configurable backend interface (generalizing ddg-search) | No hardcoded backend; no paid integrations committed in v1 |
| ACP Registry | `agent.json` manifest, goreleaser packaging | Static binary; stdio subprocess; no daemon |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| Interface Adapter ↔ Session Manager | Normalized Input/Output event channels | Per-session lock inside Session Manager serializes concurrent inputs from any adapter |
| Session Manager ↔ Turn Loop | Active context window (in-memory projection) | Transcript stays durable; window is reproducible from transcript |
| Turn Loop ↔ Model Scheduler | `Resolve(input) → Resolved` (sync call) | Scheduler returns primary + fallback chain; Turn Loop owns retry loop body |
| Turn Loop ↔ Profile Shaper | `Apply(InternalRequest, Profile) → ProviderRequest` (sync) | Single mimicry boundary; Profile is read-only config |
| Turn Loop ↔ Provider Adapter | `Send(ProviderRequest) → Stream` | Adapter owns tool-call translation; one internal representation on return |
| Turn Loop / Subagent / Tool Registry ↔ Event Bus | Publish events | Ordered per turn; subagent events tagged with parent turn id |
| Event Bus ↔ {Interface Adapter, Unified Engine, Audit Log} | Subscribe (fan-out) | Consumers are independent; ordering guaranteed per producer, not across consumers |
| Unified Engine ↔ Hook-DAG Executor | `Run(hookID, vars) → RunResult` | Executor emits step events onto bus; engine awaits completion |
| Unified Engine ↔ Learned Config | Query launch decision; write on learn; list/confirm proposed hooks | Cross-session persistent store |
| Hook-DAG Executor ↔ Session Manager | FreshContext step requests window reset | Reuses two-layer mechanism; no second reset path |
| Hook-DAG Executor ↔ Turn Loop | SendPrompt step re-enters Turn Loop | One turn per node; same loop, no shortcut |
| Profile Shaper ↔ Audit Log | Audit Log records verbatim ProviderRequest | Mimicry-correctness evidence; diff-able against recorded zcode requests |

---

## Composability With the Predecessor's 6-Component Model

The ass-guard design **composes cleanly** with the predecessor's six components:

- **No inherited component is removed.** ACP Frontend is renamed to ACP Adapter and becomes one Interface Adapter impl — same responsibility, broader abstraction. Autocontinue Engine is promoted to Unified Engine — same siting (observer on bus), same safety property, broader decision mandate.
- **No inherited boundary is violated.** The Turn Loop still produces/consumes the same internal tool-call representation; the Session Manager still owns transcript + projection; the Subagent Manager still maps `Task` to goroutine Turn Loops; the Tool Registry still owns execution with read-only-parallelize / mutating-serialize.
- **The Event Bus remains the spine.** All new consumers (Unified Engine was already there as Autocontinue; Audit Log is new) subscribe like any other consumer. The bus contract is unchanged: ordered per turn, fan-out to independent consumers.
- **The two-layer context model is reused, not duplicated.** The Hook-DAG Executor's FreshContext step asks the Session Manager for a window reset — the same mechanism the predecessor used for command boundaries. There is exactly one context-reset path.
- **The safety property is preserved uniformly.** "No match → nothing runs" applies to the Unified Engine's decision (unmatched → Stop, inert), to proposed-but-unconfirmed hooks (never executed), and to the inherited manual-cancel-drains-queue behavior. No new mechanism introduces an off-path trigger.

The net effect: the predecessor's six components are the floor; ass-guard adds five new components and broadens one. No contradictions, no rewrites of inherited internals.

---

## Sources

**Project scope (primary):**
- `/Users/nil/DiskD/W/Djarvur/ass-guard-agent/.planning/PROJECT.md` — six deltas, requirements, key decisions, constraints

**Predecessor architecture (inherited spine, primary):**
- `/Users/nil/DiskD/W/Djarvur/sdd-acp-agent/_bmad-output/planning-artifacts/research/technical-sdd-acp-agent-research-2026-07-02.md` — 6-component decomposition, event bus, two-layer context, provider-shape isolation, ACP wire protocol, integration patterns, phased risk-ordered build

**External references (from predecessor research, still load-bearing):**
- [ACP spec](https://agentclientprotocol.com/get-started/introduction), [ACP transports](https://agentclientprotocol.com/protocol/v1/transports), [prompt-turn](https://agentclientprotocol.com/protocol/v1/prompt-turn), [tool-calls](https://agentclientprotocol.com/protocol/v1/tool-calls), [session-setup](https://agentclientprotocol.com/protocol/v1/session-setup)
- [ACP Agent Registry RFD](https://agentclientprotocol.com/rfds/acp-agent-registry), [agent.schema.json](https://github.com/agentclientprotocol/registry/blob/main/agent.schema.json)
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go), [go-openai](https://github.com/sashabaranov/go-openai)
- [gsd-pi auto-mode](https://www.opengsd.net/docs/v2/auto-mode) (autonomous-loop reference)

---
*Architecture research for: ass-guard-agent — Go-based SDD-hosting AI coding agent with mimicry, scheduling, unified engine, hook-DAGs, and dual ACP+Telegram interfaces*
*Researched: 2026-08-09*
