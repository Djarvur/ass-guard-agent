# Phase 23: SEED Gaps Close-out - Research

**Researched:** 2026-08-28
**Domain:** Go agent runtime — mid-turn steering queue (transport-neutral), checkpoint restore hardening + GC, /undo as class-B command
**Confidence:** HIGH (in-repo code seams — all read this session) / MEDIUM (external steering-semantics survey — official docs verified, CC behavior version-drift flagged)

## Summary

Phase 23 closes the three remaining SEED-004 gaps on top of machinery that already exists and was read end-to-end this session. The steering queue (SEEDG-01) has a precise, already-carved seam: the model-request boundary is the top of the tool-loop iteration in `Session.runTurn` (internal/session/session.go:349-583) — a point reached between fully-appended tool results and the next projection + stream, which makes "never mid-in-flight-request, never splitting tool_use/result pairs" a structural property of the drain point rather than a discipline the drain must enforce. Because engine chains drive their turns through the same `sess.Prompt` → `runTurn` path (internal/runtime/enginebridge/enginebridge.go:191), a drain placed inside `runTurn` covers client turns, parked-chain injection turns, and hook turns uniformly — one seam, every path.

The single sharpest hazard found: steering **must not** be appended to the transcript as a plain `user_message` line. The Projector's anchor search takes the LAST `user_message` under the turn id as the window anchor and resets accumulation from it (internal/session/projector.go:197-203) — a mid-turn user message would move the anchor and wipe the turn's accumulated exchanges from the model window. D-03's typed `steering_delivery` record is therefore not just replay bookkeeping; it is what keeps the Projector correct. `accumulateMidTurn` must gain a case that folds `steering_delivery` lines as marker-wrapped user-role messages (it currently folds only tool_call/tool_result/assistant_message and silently skips everything else).

For SEEDG-02/SEEDG-03, the checkpoint store (internal/checkpoint/store.go, read in full) is solid but has five exact extension points the planner must plan against: (1) the Runner never retains the store — it is constructed inside `sessionFor` and only the adapter lands on the session (runtime.go:1004-1011, 1201-1203), so /undo, the restore guard, and the GC sweep need a Runner-level handle; (2) the snapshot-id grammar `^[A-Za-z0-9_-]+-turn-(\d{3,})$` (store.go:114) is enforced in three places (validate :374, parse :483, restore :253) and gates pre-restore snapshots as a second id family; (3) today's prune is a GLOBAL keep-50-ref cap (store.go:51, 494-511), which D-08's per-session count + age GC extends and partially supersedes; (4) nested-repo refusal has no detection today, and a local experiment this session confirmed the failure shape (gitlink 160000 recorded, contents never snapshotted, `clean -fd` never descends); (5) `.git/info/exclude` (the USER repo's) is untouched today — only the shadow store's own info/exclude self-exclusion exists (store.go:312-314). A verified experiment confirmed appending `.ass-guard/` to the user repo's `.git/info/exclude` works and `git check-ignore -v` fires.

**Primary recommendation:** Build a small transport-neutral SteerQueue (stdlib sync, monotonic tickets, cutoff-based cancel) consumed at the existing `runTurn` iteration boundary; carry steering as a `steering_delivery` transcript kind folded into the window by a Projector extension; promote the checkpoint store to a Runner-level service for the guard/snapshot/GC/exclude work and register /undo as the fourteenth RESERVED class-B name with D-12's auto-cancel composed from the existing cancel contract.

## Project Constraints (from AGENTS.md — no CLAUDE.md exists)

- **Transport discipline:** stdout reserved exclusively for ACP JSON-RPC frames; all logging/diagnostics to stderr. GC sweep logs and checkpoint warnings go to stderr/slog.
- **Go single static binary, no daemon, no network port** — the GC and steering queue are in-process; git remains the only subprocess (checkpoint store precedent, store.go:53-56).
- **Go floor:** go.mod pins `go 1.26` [VERIFIED: go.mod:3]; mise toolchain go 1.26 [VERIFIED: .mise.toml:2].
- **No new runtime dependencies required by this phase** — everything is stdlib + the existing isolated git-subprocess pattern.
- **GSD workflow:** edits flow through GSD commands (repo AGENTS.md workflow section).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Implementation Decisions

### Steering Injection
- **D-01:** Steering text enters the NEXT request as a user-role message wrapped in a steering marker (system-reminder-shaped — the captured wire convention): the model sees the user speaking mid-turn, distinguishable from turn-start prompts; replay can tell steering from prompts.
- **D-02:** All steering queued since the last boundary COALESCES into one delivery at the next boundary — arrival order preserved, one marker block. Bounded request growth.
- **D-03:** Each boundary drain emits a session/update note ("steering applied: N inputs") + a typed `steering_delivery` transcript record — visible live, reconstructable on replay (18-D-01).
- **D-04:** Steering NEVER cancels or interrupts: the running turn absorbs it at the boundary and continues. Cancellation stays the separate explicit ticket/cutoff protocol (SEEDG-01 letter). One knob each.
- **D-05:** A parked ask is VISIBLE immediately: one session/update note when parked ("ask waiting behind running turn: \<summary\>"), then fires in full when its queue position is reached.
- **D-06:** A parked ask is CANCELABLE while waiting (cancel surface on the parked note; a subsequent input can parse as cancel): resolves cancelled-normal WITHOUT killing the turn — 17-D-13's drain semantics applied proactively.
- **D-07:** Parking never blocks: parked asks belong to their own (or origin) turns; parking affects delivery order only. The active turn's progress is untouched.
- **D-08:** Checkpoint expiry = AGE + COUNT, whichever binds first: default 7d age AND a max-per-session count (count default Claude's discretion); GC sweeps at session start (18-D-09's grace-family precedent).
- **D-09:** Pre-restore snapshots ARE checkpoint objects: they enter the same age+count GC and are restorable via /undo — a botched restore is undoable by the same machinery. One store, one lifecycle.
- **D-10:** `checkpoint.expiry_days` (7) and `checkpoint.max_per_session` join the configOptions menu (Phase 16's rule — advertise all, apply-as-landed).
- **D-11:** Stack walk: /undo restores the last checkpoint; repeated /undo keeps walking back (each restore snapshots current state first — D-09 makes the walk reversible); `/undo N` jumps N steps. Depth bounded naturally by the age+count GC.
- **D-12:** /undo with an ACTIVE turn AUTO-CANCELS then restores (operator override of the refuse option): cancel via the existing cancel contract (clean drain — 18-D-12 family), then pre-restore snapshot, then restore. Nested-repo refusal still refuses outright (no auto path — gitlink contents would be silently unprotected). — **Reversibility:** costly — auto-cancel-then-restore is a compound destructive action on one keystroke; the pre-restore snapshot is the only undo, so the snapshot MUST precede the cancel's state changes where ordering permits.

### Claude's Discretion
- The steering marker's exact wrapper syntax (existing system-reminder shape family).
- Parked-note wording and the cancel-input grammar (what parses as "cancel the parked ask").
- max_per_session default value; GC sweep logging shape.
- /undo N argument parsing edge cases (0, negative, beyond-depth).
- The transport-neutral steering API's concrete interface shape (Go API consumable by a non-ACP frontend — TG-02's contract).

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SEEDG-01 | Steering queue during a running turn: drained at model-request boundaries only (never mid-in-flight-request, never splitting tool_use/result pairs), ticket/cutoff cancel protocol, parked-ask disambiguation, transport-neutral API (Telegram prerequisite — do not descope to queue-behind silently) | Drain seam located at `runTurn` loop top (session.go:365); ticket/cutoff protocol defined (PITFALLS.md Pitfall 13); ingress dispatch order mapped (routeAskReply precedent runtime.go:1611); boundary kinds + Projector fold design; convergent external semantics verified (PicoClaw/OpenClaw/pi); ACP v1 has no inject surface — agent-side over ordinary session/prompt (discussion #1220) |
| SEEDG-02 | Checkpoint restore guard: refuse restore with active turn/engine chains; pre-restore snapshot before overwriting; nested-repo refusal (gitlink contents silently unprotected otherwise); checkpoint object-expiry GC; `.ass-guard/` in `.git/info/exclude` | Store read in full — all five guards mapped to exact extension points (store handle promotion, id-grammar constraint, global-prune vs per-session count, nested-repo experiment results, user-repo exclude append verified) |
| SEEDG-03 | `/undo` command (class-B) restoring last checkpoint | 20-01/20-02 class-B contracts read (intercept placement, RESERVED 13-name set, D-05 output shape, local_command record); /undo joins as 14th reserved name; D-11 stack walk over `Store.List()`; D-12 auto-cancel composed from existing cancel contract |
</phase_requirements>

## Architectural Responsibility Map

Tier vocabulary adapted to this process's internal seams (ACP frontend / runtime core / session core / storage).

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Steering ingress + classification (ask-reply / parked-cancel / class-B / steer / new turn) | Runtime core (Runner.Run entry, pre-turnMu) | ACP layer (transport only) | The transport-neutral queue API must be ACP-type-free (15-D-20 layering; TG-02 consumer); classification is engine logic, not wire logic |
| SteerQueue (tickets, coalesce, cutoff cancel) | New session-scoped component (stdlib) | — | No mature Go library for this; ~100 lines; must be transport-neutral by construction |
| Boundary drain → steering delivery | Session core (`runTurn` iteration top) | — | Only place where no request is in flight and pairs are closed; shared by engine + non-engine paths |
| `steering_delivery` / `parked_ask` transcript records | Session core (Manager append) | Projector (window fold + replay tolerance) | Transcript-as-truth (18-D-01); additive weak schema (16-D-20) |
| Steering/parked notes to the client | ACP emitter (existing note machinery) | Bus forwarders | Notes ride `agent_message_chunk` through the in-hand emitter (runtime.go:561-563 precedent) |
| Restore guard + pre-restore snapshot + nested detection | Checkpoint package + Runner wiring | — | Store owns git isolation; Runner owns active-turn/chain state (turnActive, chainCount) |
| Age+count GC sweep | Checkpoint package (git subprocess) | Session-start wiring | Shadow-repo ref deletion + `reflog expire`/`gc` stay inside the store's withLock discipline |
| `.git/info/exclude` append | Checkpoint package (store init) | — | Physical sibling of the existing shadow-store self-exclusion (store.go:312-314) |
| /undo class-B command | Runtime core (commands.go chain table) | Checkpoint service | 20-01 class-B contract; zero model turns; local_command record |
| `checkpoint.expiry_days` / `max_per_session` menu | ACP configOptions surface (acpserve) | Config read-back | 16-05 ConfigSurface precedent; select-type wire constraint |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib (`sync`, `context`, `time`, `os`, `path/filepath`, `encoding/json`, `log/slog`) | go 1.26 [VERIFIED: go.mod:3] | SteerQueue, tickets, GC wiring, exclude append | Zero-dep invariant; the queue is ~100 lines of channel/mutex code |
| `git` subprocess (isolated env) | 2.50.1 on this machine [VERIFIED: local probe] | All checkpoint operations incl. new GC (`update-ref -d`, `reflog expire`, `gc --prune=now`) | The store already funnels every git call through `gitRun`/`s.git` with neutralized config (store.go:131-148, 331-359) — extend, never fork |
| existing deps (anthropic-sdk-go, cobra, testify, yaml.v3) | pinned in go.mod | unchanged | No new module requirements this phase |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golangci-lint` v2 (mise) | [VERIFIED: .mise.toml:3] | lint gate incl. new code | every task |
| `mise ci` | [VERIFIED: .mise.toml:24-27] | vet + lint + build + test gate | per-wave / phase gate |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled SteerQueue | channel-only queue | Channels cannot implement cutoff-cancel + inspection (List for parked-ask notes) cleanly; a mutex+slice with tickets is simpler to test under `-race` |
| Steering as system-role message | (rejected shape) | D-01 locks user-role; system-role mid-turn would diverge from the captured convention (corpus census counts `<system-reminder>` as message text — internal/profile/corpus_scan.go:29-31) |
| Filesystem watch for GC trigger | session-start sweep | D-08 locks session-start sweep (18-D-09 grace-family precedent) |

**Installation:** none — no new packages. Verify no accidental dep drift with `mise tidy` at phase close.

## Package Legitimacy Audit

> No external packages are installed by this phase (stdlib + existing go.mod deps + the platform `git` binary, which store.go:53-56 already documents as a pre-existing platform dependency, NOT a module dependency).

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none — no new installs) | — | — | — | — | — | — |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
                        ┌──────────────────────────────────────────────────┐
                        │                 INPUT INGRESS                     │
                        │  (ACP session/prompt today; Telegram goroutine    │
                        │   tomorrow — SAME entry, no ACP types below this  │
                        │   point, TG-02 consumer contract)                 │
                        └───────────────────┬──────────────────────────────┘
                                            v
                              ┌──────────────────────────┐
                              │ Ingress classifier (pre-  │
                              │ turnMu), in priority order│
                              └──┬───────┬───────┬───────┘
              pending ask reply  │       │       │  turn/chain active?
              (routeAskReply     │       │       │
               unchanged,        v       │       v
               runtime.go:1611) ┌──────────────┐  ┌──────────────────┐
                        ┌──────►│ parked-ask    │  │ class-B /undo?   │
                        │       │ cancel grammar│  │ → D-12 auto-cancel│
                        │       └───────┬───────┘  │ → snapshot→restore│
                        │               v          └──────────────────┘
                        │       ┌──────────────┐
                        │       │ STEER ENQUEUE │  ticket N assigned,
                        │       │ (SteerQueue)  │  note "queued behind
                        │       └───────┬───────┘   running turn"
                        │               v          else: ordinary new turn
                        │   ┌───────────────────────────────┐
                        │   │  RUNNING TURN (runTurn loop)  │
                        │   │  ┌─────────────────────────┐  │
                        │   │  │ provider request (SSE)  │  │
                        │   │  └───────────┬─────────────┘  │
                        │   │              v                │
                        │   │  tool_use batch + results     │
                        │   │  appended (pairs closed)      │
                        │   └───────────┬───────────────────┘
                        │               v  MODEL-REQUEST BOUNDARY
                        │   ┌───────────────────────────────┐
                        └──►│ DRAIN: coalesce queue → one    │
                            │ marker-wrapped user-role text; │
                            │ append steering_delivery line; │
                            │ note "steering applied: N";    │
                            │ cutoff = last drained ticket   │
                            └───────────┬───────────────────┘
                                        v
                        ┌───────────────────────────────┐
                        │ Project(turnID) folds window: │
                        │ ... tool pairs ... + steering │
                        │ delivery as user-role message │
                        └───────────┬───────────────────┘
                                    v
                        ┌───────────────────────────────┐
                        │ next provider request (steering│
                        │ visible; prefix up to the new  │
                        │ tail unchanged → cache-friendly)│
                        └───────────────────────────────┘

  Checkpoint side (SEEDG-02/03):  turn entry ──► Snapshot(pre-turn)
    /undo ──► [active turn? auto-cancel via cancel contract] ──►
    guard (nested repo? refuse) ──► pre-restore Snapshot ──► Restore
    session start ──► GC sweep (age 7d ∥ per-session count) ──►
    ref deletion + reflog expire + gc in shadow store
    store init ──► append ".ass-guard/" to USER repo .git/info/exclude
```

### Recommended Project Structure

```
internal/
├── session/
│   ├── steerqueue.go        # NEW: transport-neutral SteerQueue (tickets, coalesce, cutoff)
│   ├── steerqueue_test.go   # -race producer/cancel hammer tests
│   ├── session.go           # MOD: drain at runTurn iteration top; SetSteerQueue wiring
│   ├── projector.go         # MOD: fold steering_delivery lines into mid-turn window
│   ├── transcript.go        # MOD: TypeSteeringDelivery, TypeParkedAsk kinds (16-D-20 additive)
│   └── manager.go           # MOD: AppendSteeringDelivery / AppendParkedAsk (AppendLocalCommand template)
├── runtime/
│   ├── runtime.go           # MOD: ingress classifier pre-turnMu; checkpoint service handle; GC-at-session-start wiring
│   └── commands.go          # MOD: /undo class-B handler (14th RESERVED name)
├── checkpoint/
│   ├── store.go             # MOD: RestoreGuard, Snapshot family (pre-restore ids), GC, user-repo exclude
│   └── store_test.go        # MOD: nested-repo fixture, GC, guard tests
└── acpserve/
    └── config_surface.go    # MOD: checkpoint.expiry_days + checkpoint.max_per_session menu entries
```

### Pattern 1: Boundary drain at the tool-loop iteration top

**What:** drain the SteerQueue at the one point where the previous provider response is fully processed (all tool results appended — pairs closed) and no new request has started.
**When to use:** every turn path — client turns, engine-chain injections, hook turns all funnel through `runTurn`.
**Example** (drain point in the real loop; source internal/session/session.go:349-394, verified this session):

```go
func (s *Session) runTurn(ctx context.Context, turnID string) (stop string, err error) {
    const maxIterations = 64
    for range maxIterations {                     // <- each iteration = one model request
        // *** SEEDG-01 DRAIN POINT: before Project/streamAndEmit ***
        // if delivered := s.steer.DrainCutoff(&cutoffTicket); delivered > 0 {
        //     s.appendSteeringDelivery(turnID, batch)   // steering_delivery kind
        //     emit note "steering applied: N"           // existing note machinery
        // }
        messages, err := s.Projector.Project(turnID)   // session.go:373
        // ... Semaphore.Acquire ...
        resp, textBuf, streamErr := s.streamAndEmit(ctx, turnID, messages) // session.go:394
        // tool calls recorded (:421-423), results appended (:529-536) —
        // pairs are CLOSED before the next iteration reaches the drain point
    }
}
```

Never split a pair: appending steering BEFORE `Project` and AFTER the previous iteration's result appends makes pair-safety structural. The Projector's own pair-safety precedent (orphan-drop + `boundMidTurn` complete-group truncation, projector.go:207-260, 262+) shows the invariant's shape.

### Pattern 2: Ticket/cutoff cancel protocol

**What:** every enqueued steering input gets a monotonically increasing ticket; cancellation marks a cutoff; items with ticket <= cutoff are drained-with-acknowledgment, items > cutoff survive to the next turn.
**When to use:** session/cancel, D-12's /undo auto-cancel, and turn death — anywhere queued inputs must be resolved deterministically against an in-flight producer.
**Source:** milestone research, quoted verbatim [CITED: .planning/research/PITFALLS.md:329]: "assign monotonically increasing queue tickets; cancel marks a cutoff ticket — items with ticket ≤ cutoff are drained with acknowledgment (synthetic 'cancelled' closure in transcript), items > cutoff survive to the next turn. Test the race with a producer hammering enqueue during cancels under `-race`."

```go
type SteerQueue struct {
    mu      sync.Mutex
    items   []SteerItem // {Ticket uint64, Text string, At time.Time}
    next    uint64
    cutoff  uint64 // cancels resolve items with Ticket <= cutoff as cancelled-normal
}
// Enqueue -> ticket; Drain -> coalesced batch (D-02, arrival order) + advances
// the watermark; Cancel(cutoff) -> marks/drops covered items with transcript
// acknowledgment; producer enqueues never block the drain (single mutex, no chans).
```

The existing cancel contract it composes with: `session/cancel` → `st.cancelTurn()` → turn ctx cancelled → `stopReason "cancelled"` with updates-before-response barrier (internal/acp/handlers.go:382-452, verified). Turn-death drains of queued asks are the 17-D-13 precedent; D-06 extends the same resolve-cancelled-normal semantics proactively to parked asks.

### Pattern 3: Additive transcript kinds (steering_delivery, parked_ask)

**What:** new line kinds under 16-D-20's additive-only weak schema — readers tolerate unknown kinds; payloads parse in their owning phase.
**Verified kind family today** (internal/session/transcript.go:19-77, quoted verbatim — the values are the source of truth):

```go
TypeSessionStart      = "session_start"
TypeUserMessage       = userMessageType
TypeRequestShaped     = "request_shaped"
TypeAgentMessageChunk = "agent_message_chunk"
TypeAssistantMessage  = "assistant_message"
TypeToolCall          = "tool_call"
TypeToolResult        = "tool_result"
TypeBoundary          = kindBoundary
TypeSubagentDispatch  = "subagent_dispatch"
TypeSubagentResult    = "subagent_result"
TypeUsage             = "usage"
TypeCanceled          = "canceled"
TypeEngineDecision    = "engine_decision"
TypeError             = "error"
TypeSessionEnd        = "session_end"
TypeCommandProvenance = "command_provenance"
TypeAskSuspended      = "ask_suspended"
TypeRawThinking       = "raw_thinking"
TypeLocalCommand      = "local_command"
TypeCompaction        = "compaction"
```

There is **no checkpoint marker kind today** — /undo's durable record is the `local_command` line (16-D-22 full record: key, args, source chain, outcome; append template internal/session/manager.go:362, REDACTED path per 20-01). `steering_delivery` and `parked_ask` join the same family; the Line struct already carries reusable fields (Text/Content/TurnID; Args/SourceChain/Expansion for command-shaped records — transcript.go:89-156).

### Pattern 4: Class-B /undo registration (Phase 20 contract)

**Verified contracts from 20-01-PLAN/20-02-PLAN (read this session):**
- Class-B intercept placement: in `Runner.Run` AFTER `routeAskReply` and AFTER `turnMu` + forwarder subscriptions, BEFORE the engine branch (20-01 key_links; the engine branch is runtime.go:539, routeAskReply call runtime.go:525).
- RESERVED builtin set is exactly thirteen names — "help status cost mcp memory permissions doctor config model clear resume compact init" (20-01 must_haves, quoted verbatim). **/undo joins as the fourteenth**; 20-D-01's shadow-check warning (already shipped by 20-01 Task 1) covers discovered-file shadowing when the set grows.
- Output shape (20-D-05): echo `user_message_chunk` → `agent_message_chunk` output → stopReason end_turn; zero provider calls; durable `local_command` line via the REDACTED path.
- `/undo N` parses args verbatim into the local_command record (16-D-22); 0/negative/beyond-depth handling is CONTEXT discretion.

### Pattern 5: Restore guard + pre-restore snapshot + nested detection + GC

**What:** the five SEEDG-02 guards as store-level operations composed by the Runner.
**Verified store facts this session (internal/checkpoint/store.go):**

| Fact | Value | Provenance |
|------|-------|-----------|
| Store path | `<workDir>/.ass-guard/checkpoints/shadow.git` | store.go:58-63, 183-184 |
| Ref namespace | `refs/checkpoints/<sessionID>-turn-<NNN>`; convenience tip `refs/checkpoints/last` | store.go:66-71 |
| Id grammar | `^[A-Za-z0-9_-]+-turn-(\d{3,})$` | store.go:114 (also enforced :253, :369-379; parse derives SessionID via TrimSuffix :483) |
| Retention today | `DefaultKeep = 50`, GLOBAL prune of oldest refs after each snapshot, ties by (sessionID, turn) | store.go:51, 494-511 |
| Restore op | `checkout --no-overlay -f refs/checkpoints/<id> -- .` then `clean -fd -e .ass-guard/` | store.go:267-272 |
| Locking | in-process mutex + cross-process O_EXCL lock file, 30s stale theft | store.go:518-563 |
| Snapshot | retry-safe (same turn id updates same ref), commit-then-update-ref | store.go:214-237 |
| Self-exclusion | shadow store's OWN info/exclude carries `.ass-guard/` — the USER repo's exclude is untouched today | store.go:77-79, 312-314 |
| Turn id source | `<sessionID>-turn-%03d` from `nextTurnID` | session.go:140-144 |

Design consequences:
1. **Pre-restore snapshots need an id family.** `validateTurnID` requires the `<sessionID>-turn-` prefix (store.go:374) and `idPattern` the literal `-turn-` — either extend the grammar (e.g. a `-pre-NNN`/`-undo-NNN` family) in all three enforcement sites (store.go:114, :374, :483) or mint turn-family ids; either is plan-decidable, but the three-site coupling must be planned, not discovered.
2. **GC supersedes/extends prune.** D-08's per-session count binds per session; today's `prune` is global keep-50. GC also must expire OBJECTS: deleting refs leaves loose objects — `reflog expire --expire=now --all` + `gc --prune=now` inside the shadow store (milestone PITFALLS wall 2), all under `withLock`.
3. **Nested-repo detection must cover `.git` FILES too** (linked worktrees and submodules use a `.git` file, not a directory).
4. **The exclude append targets the USER repo** (`<workDir>/.git/info/exclude`) — a different file from the shadow store's own exclude; append idempotently, never clobber, degrade loudly when `<workDir>` is not a git repo.
5. **The Runner must hold the store.** Today the store exists only as `session.Checkpointer` (runtime.go:1201-1203); /undo, the guard, and the session-start GC sweep need a Runner-level `*checkpoint.Store` (or a small service wrapping it) built once per workspace.

### Pattern 6: Ingress classification order (the disambiguation core)

**What:** one classifier at Run entry decides what an input IS, before any blocking. Verified current ordering to preserve/extend (runtime.go:489-541): turnMu.Lock → markClientTurn → subscribe forwarders → `routeAskReply` (runtime.go:525) → engine branch (runtime.go:539) → runOneTurn (runtime.go:556).

Recommended order (preserves every existing behavior):
1. **Pending ask** → `routeAskReply` unchanged (17-D-11: one ask outstanding; reply routing is today's contract). A parked-ask CANCEL grammar parses here (D-06) before ordinary reply interpretation.
2. **Class-B invocation** (chain resolve on the first text block — single-parse discipline) — never becomes steering. `/undo` with active turn/chain takes the D-12 auto-cancel path; other class-B commands may still queue-behind on turnMu as today.
3. **Steering enqueue** when a turn is active for the session (`turnActive` map, runtime.go:181-182, or a queue-owned active flag) — assign ticket, emit the queued note, return.
4. **Ordinary new turn** otherwise (today's path verbatim).

D-12's mutex trap: the 20-01 class-B intercept runs while HOLDING `turnMu` (20-01 key_links). A handler holding `turnMu` can never wait for an active turn to release it — auto-cancel must classify BEFORE `turnMu.Lock()` (ingress level), cancel via the existing contract, acquire the mutex for the restore, snapshot, restore. Note the parked-chain case: a parked engine chain holds NO mutex (Run returned at suspension, runtime.go:849-855) while `chainCount > 0` (runtime.go:946-951) — that is the in-process state the "refuse with active chains" guard and D-12's cancel compose over. `WaitChainIdle` (runtime.go:957-969) is the existing wait-through-suspension seam.

### Anti-Patterns to Avoid

- **Queue-behind masquerading as steering:** relying on `turnMu` blocking (today's 12-07 semantics) is exactly the descope SEEDG-01 forbids — the correction must reach the model while the turn can still act on it.
- **Appending steering as `user_message`:** moves the Projector anchor (projector.go:197-203) and wipes the turn's window; also makes replay indistinguishable from turn prompts (D-01's letter).
- **Injecting into an in-flight request** or between a `tool_call` append and its `tool_result` append — both break provider pairing and the Shaper's message alternation (`toMessageParams` folds tool messages, shaper.go:165+).
- **Importing PicoClaw/OpenClaw's skip-remaining-tools behavior:** those agents skip not-yet-run tools with synthetic results on steering; D-04 locks absorb-and-continue (steering never cancels or interrupts; already-dispatched batches complete).
- **Importing strands' interrupt semantics:** strands steering handlers HALT the loop for approve/deny (interrupt-and-pause family) — a different feature.
- **`git clean -ff`:** descends into nested repos — never enter the store's vocabulary.
- **Writing the user's tracked `.gitignore`:** `.git/info/exclude` is the sanctioned channel (repo-local, untracked) — consistent with the strictly-read-only-on-user-config spirit.
- **Holding `turnMu` while waiting for the turn** (see Pattern 6) — self-deadlock.
- **Post-turn note emission for steering:** a bus publish after the subscriber drains is LOST (the documented PATTERNS timing hazard, runtime.go:543-549) — steering/parked notes fire at the boundary through the live emitter path, not deferred like the deduped advisory collector.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Snapshot/restore/git plumbing | a second git integration | `internal/checkpoint` Store (extend) | isolation env (GIT_INDEX_FILE, neutralized config, disabled hooks), commit-then-ref ordering, O_EXCL locking are already test-pinned (store.go docblock invariants) |
| Locking for store ops | ad-hoc mutexes at call sites | `withLock` (store.go:518-563) | in-process + cross-process + stale-theft already handled |
| Ask suspension/resume | a parallel park mechanism | AskBroker Surface/Claim/Settle (ask.go:97-263) + ArmTimeout | parked asks ARE queued asks with delivery positions (CONTEXT); the settle channel is the wait seam |
| Turn-active detection | new flags | `turnActive` map (runtime.go:181) + `chainCount`/`WaitChainIdle` (runtime.go:946-969) | both exist and are race-tested |
| Cancel + drain | a new cancel channel | `st.cancelTurn` contract + 17-D-13 drain semantics (18-D-12 family) | D-12 explicitly rides the existing contract |
| Note emission | a new wire kind for notes | `agent_message_chunk` note family (runtime.go:561-563 precedent) | D-05's "session/update note" maps to the established shape |
| Config menu entries | bespoke wire handling | ConfigSurface option registration (config_surface.go:138-207, 479-531) | D-09 typed reject, D-10 idempotent re-push guard, scope namespace all free |

**Key insight:** this phase is almost entirely composition over test-pinned machinery. The genuinely new code is the SteerQueue, the drain seam, the Projector fold, the store guards/GC, and the /undo handler — everything else is wiring.

## Common Pitfalls

### Pitfall 1: The Projector anchor-move hazard (steering as user_message)
**What goes wrong:** `accumulateMidTurn`'s anchor loop takes the LAST `user_message` with the running turn's id and starts accumulation AFTER it (projector.go:197-203). A steering line of kind `user_message` resets the anchor mid-turn — every prior exchange of the turn vanishes from the model window mid-conversation.
**Why it happens:** the anchor rule assumes user messages only START turns.
**How to avoid:** `steering_delivery` is its own kind; `accumulateMidTurn` gains an explicit case folding it as a user-role message (marker text included) in arrival position; `findCurrentIntent`/seed extraction never sees it.
**Warning signs:** mid-turn requests suddenly carry only the steering text; task summary disappears after steering.

### Pitfall 2: Splitting pairs / injecting mid-flight
**What goes wrong:** a drain that fires during `streamAndEmit` or between `AppendToolCall` and `AppendToolResult` produces a user message interleaved in an assistant tool_use batch — provider 400s or silent pairing corruption (the Projector drops orphaned results, projector.go:247-249, silently losing content instead).
**How to avoid:** drain only at the iteration top (Pattern 1); append all steering before `Project`.
**Warning signs:** flaky provider errors only on steered turns; missing tool results in projected windows.

### Pitfall 3: The last-boundary cutoff race
**What goes wrong:** inputs enqueued after the turn's FINAL model request has begun are never drained by this turn — if the turn just drops them they are acknowledged-but-lost; if it drains late they arrive after the turn ended (zombie delivery).
**Why it happens:** enqueue and turn-completion race.
**How to avoid:** ticket/cutoff (Pattern 2): the turn stamps its final drained ticket; at completion, items above the watermark are released to become the next turn's input (or resolved cancelled on cancel). Milestone test prescription: producer hammering enqueue during cancels under `-race` [CITED: .planning/research/PITFALLS.md:329].
**Warning signs:** lost messages reported once per N steered turns; duplicated delivery after resume.

### Pitfall 4: /undo auto-cancel deadlock (mutex ordering)
**What goes wrong:** implementing D-12 inside the 20-01 class-B intercept (which holds `turnMu`, per 20-01 key_links) and then waiting for the active turn to finish — the turn needs the mutex the handler holds.
**How to avoid:** classify /undo BEFORE `turnMu.Lock()` (ingress level); cancel first (existing contract), then acquire, snapshot, restore (D-12 ordering: snapshot precedes the cancel's state changes where ordering permits).
**Warning signs:** a /undo during a long turn hangs the session; -race does NOT catch this — it needs a behavioral test.

### Pitfall 5: GC semantics drift (global prune vs per-session count; orphaned objects)
**What goes wrong:** layering D-08's per-session count over the existing GLOBAL `DefaultKeep=50` prune (store.go:51, 494-511) produces surprising eviction (a chatty session evicts a quiet session's checkpoints); deleting refs without expiring objects leaves the store growing forever.
**How to avoid:** make the age+count sweep the single GC authority at session start; keep (or retire) prune deliberately, documented; run `reflog expire --expire=now --all` + `gc --prune=now` inside the shadow store under `withLock` after ref deletion.
**Warning signs:** shadow.git object size grows monotonically in a soak; checkpoints of idle sessions vanish.

### Pitfall 6: Pre-restore snapshot id grammar
**What goes wrong:** minting a pre-restore snapshot id that fails `idPattern` (store.go:114) or `validateTurnID`'s session-prefix check (store.go:374) — Snapshot errors and the restore proceeds WITHOUT its safety snapshot (D-12's worst case).
**How to avoid:** decide the id family up front; update all three grammar sites (store.go:114, :374, :483) together; test that snapshot failure ABORTS the restore (fail-closed), never restores unsnapshotted.
**Warning signs:** "invalid turn id" errors in stderr during /undo.

### Pitfall 7: Nested repos — detection and the true failure shape
**What goes wrong (verified by local experiment this session):** `git add -A` records a nested repo as GITLINK mode 160000 and warns "adding embedded git repository… Clones… will not contain the contents"; restoring to a pre-nested checkpoint warns `unable to rmdir 'nested': Directory not empty` and `clean -fd` never descends — nested contents are neither snapshotted NOR destroyed, they are silently unprotected (a turn that modified files inside a nested repo cannot be undone, and /undo's success message lies). Detection must catch `.git` as dir OR file (worktrees/submodules).
**How to avoid:** walk the workspace for `.git` entries (skip `.ass-guard/`); refuse the restore outright (D-12: no auto path); optionally warn at snapshot time (the `git add` embedded-repo warning already surfaces on stderr — capture it or walk at snapshot too).
**Warning signs:** E2E fixtures are flat text-only trees (milestone PITFALLS prescription: add a nested-repo + binary fixture).

### Pitfall 8: The wrong exclude file
**What goes wrong:** "append `.ass-guard/` to `.git/info/exclude`" implemented against the SHADOW store's info/exclude (which already carries it, store.go:312-314) and the user repo stays polluted — `git status` shows `.ass-guard/` untracked.
**How to avoid:** target `<workDir>/.git/info/exclude` specifically; append (never truncate; preserve `existing-exclude-entry` lines — verified by experiment with `git check-ignore -v` firing on the appended rule); idempotent (check-before-append); no `.git` dir → structured stderr note, never a failure.
**Warning signs:** `git status` in the user's worktree showing `.ass-guard/` (milestone warning sign, verbatim).

### Pitfall 9: The Runner doesn't own the store
**What goes wrong:** /undo handler, restore guard, or GC reaching for a store that only the session holds (runtime.go:1201-1203) — nil Checkpointer on degraded sessions silently disables /undo with no loud note.
**How to avoid:** promote the store to a Runner-level field built once per workspace; degrade loudly (the AUD-03 discipline — session without checkpoints runs, /undo reports "unavailable: checkpoint store disabled") exactly like today's open-failure log (runtime.go:1010).
**Warning signs:** /undo works in fresh sessions but not resumed/degraded ones.

### Pitfall 10: configOptions shape mismatch
**What goes wrong:** assuming numeric config options exist. The only pinned wire type is `ConfigOptionTypeSelect = "select"` (types.go:208-210); `checkpoint.expiry_days`/`max_per_session` must ship as enumerated selects (the compaction-threshold numeric-as-select precedent — values off/50/65/80/95, config_surface.go:624) or extend the type deliberately.
**Also:** persisting `checkpoint:` keys into a modelrouting layer file is LOAD-SAFE — `Load` decodes via plain yaml into `map[string]any` with no KnownFields restriction and the final typed decode silently ignores unknown top-level keys (load.go:43-79, verified) — but the value is INVISIBLE to `modelrouting.Config`; read-back must use the generic layer-map pattern (`readLayerMap`/`mapHasPath`, config_surface.go:629-668).
**Warning signs:** advertised option rejected with "unknown option id" (`isMenuOption`, config_surface.go:608-610, must learn the new ids).

### Pitfall 11: Steering vs parked-ask vs class-B ambiguity
**What goes wrong:** text arriving while an ask is parked AND a turn runs — is it an answer, a cancel, a class-B command, or steering? Guessing wrong either kills the turn (the thing D-04 forbids) or swallows a command.
**How to avoid:** the fixed classifier order (Pattern 6); the parked-ask cancel grammar is CONTEXT discretion but must be parsed BEFORE steering interpretation; class-B resolution precedes steering. Milestone prescription: "Tests never combining steering + parked ask + cancel in one scenario" is the warning sign [CITED: .planning/research/PITFALLS.md:339-341].
**Warning signs:** a /undo typed mid-turn reaching the model as steering text.

### Pitfall 12: Replay divergence
**What goes wrong:** steering deliveries visible live but reconstructed wrong (or over-eagerly injected) on session/load replay (18-D-01 transcript-as-truth; 18-D-01 is why D-03 demands the typed record).
**How to avoid:** `steering_delivery` lines carry everything replay needs (coalesced text, ticket/count, turnID); the Projector folds them the same way live and on replay; readers tolerate the kinds until their owning phase parses (16-D-20).
**Warning signs:** replayed sessions differ from live ones in request shape (the parity harness would catch it).

## Code Examples

### Steering delivery append + note (session-core, at the boundary)
```go
// Source: composed from verified repo patterns — Manager.AppendLocalCommand
// (internal/session/manager.go:362) template + note machinery (runtime.go:561-563).
func (s *Session) appendSteeringDelivery(turnID string, batch []SteerItem) error {
    text := renderSteeringMarker(batch) // D-01: user-role text wrapped in the
    // system-reminder-shaped marker; exact wrapper is CONTEXT discretion
    // (captured convention: "<system-reminder>…" — corpus_scan.go:29-31)
    return s.Manager.AppendSteeringDelivery(turnID, text, len(batch))
}
// note, through the live emitter path (never a post-turn bus publish):
// emit.AgentMessageChunk(turnID, fmt.Sprintf("steering applied: %d inputs", len(batch)))
```

### /undo class-B handler skeleton (runtime-core)
```go
// Source: 20-01/20-02 class-B contract (zero provider calls; D-05 shape;
// local_command record) + D-11/D-12 semantics.
func undoCommand(r *Runner, sess *session.Session, args string) (string, error) {
    n := parseUndoDepth(args) // discretion: 0/negative/beyond-depth
    entries, err := r.ckpt.List()          // refs ascending (sessionID, turn)
    // pick the target: last checkpoint of THIS session, n steps back (D-11)
    if err := r.ckpt.EnsureExclude(); err != nil { /* loud stderr note, continue */ }
    if nested := findNestedRepos(r.workDir); len(nested) > 0 {
        return "refused: nested repositories (" + strings.Join(nested, ", ") +
            ") are silently unprotected by checkpoints", nil // D-12 refusal
    }
    if err := r.ckpt.Snapshot(preRestoreID(sess)); err != nil {
        return "", err // FAIL-CLOSED: no restore without its snapshot (D-09)
    }
    if err := r.ckpt.Restore(ctx, targetID); err != nil { return "", err }
    return "restored " + targetID + " (pre-restore snapshot: " + preID + ")", nil
}
```

### Age+count GC sweep (checkpoint package)
```go
// Source: extends store.go primitives (listRefs :423-449, update-ref -d :504,
// withLock :518-563) under one lock; git args are the store's own vocabulary.
func (s *Store) Sweep(ctx context.Context, maxAge time.Duration, perSession int) error {
    return s.withLock(ctx, func() error {
        entries, _ := s.listRefs(ctx) // ascending; CommittedAt per entry
        victims := expireByAge(entries, maxAge)         // D-08 axis 1 (7d default)
        victims = append(victims, expireByCount(entries, perSession)...) // axis 2
        for _, v := range victims { _, err := s.git(ctx, "update-ref", "-d", v.Ref); ... }
        // expire objects so the store stops growing (refs alone are not enough):
        _, err := s.git(ctx, "reflog", "expire", "--expire=now", "--all")
        _, err2 := s.git(ctx, "gc", "--prune=now")
        return errors.Join(err, err2)
    })
}
```

### User-repo exclude append (store init)
```go
// Source: verified by experiment — append + git check-ignore -v fires
// (.git/info/exclude:2:.ass-guard/ .ass-guard/x); existing lines preserved.
func ensureUserRepoExclude(workDir string) error {
    gitDir := filepath.Join(workDir, ".git")
    if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
        return nil // not a repo (or .git is a FILE = worktree: skip + note)
    }
    path := filepath.Join(gitDir, "info", "exclude")
    _ = os.MkdirAll(filepath.Dir(path), 0o700)
    data, _ := os.ReadFile(path)
    if slices.Contains(strings.Split(string(data), "\n"), storeRootDir+"/") {
        return nil // idempotent
    }
    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
    if err != nil { return err }
    defer f.Close()
    _, err = f.WriteString(storeRootDir + "/\n")
    return err
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Queue-behind on turnMu (12-07 D-02) | Boundary-drain steering (this phase) | Phase 23 | The correction reaches the model mid-turn; turnMu remains the turn-completion serializer |
| No mid-turn wire primitive | ACP `session/inject` PROPOSED (discussion #1220), not in v1 | 2026 (v2 draft era) | Steering stays agent-side over ordinary session/prompt in v1; revisit if ACP v2 lands it |
| CC queued-until-turn-end | CC delivers queued messages "as soon as possible" (next boundary) per maintainer | evolving, version-dependent | ass-guard locks boundary-drain by decision (D-01..D-04) — CC drift is not a spec risk |
| strands-style interrupt steering | PicoClaw/OpenClaw/pi-style inject steering | current | ass-guard implements the inject family; strands' interrupt-and-pause is a different feature |
| age-only or count-only retention | age + count dual expiry (D-08) | this phase | Disk hygiene on both axes; supersedes the global keep-50 prune |

**Deprecated/outdated:**
- pi/strands as UNVERIFIED references (ROADMAP research flag): now verified — pi's rpc.md documents boundary delivery "after the current assistant turn finishes executing its tool calls, before the next LLM call"; strands' steering handlers are the interrupt family, NOT a boundary-injection reference. The flag is resolved.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Claude Code's mid-turn delivery timing is version-dependent (community evidence only; no official spec page) — ass-guard's D-01..D-04 lock the semantics regardless | State of the Art | Low: design is decision-locked; CC parity claim in docs should be softened to "convergent" |
| A2 | Zed sends a second `session/prompt` mid-turn (it may instead queue client-side or send session/cancel first — unverified this session) | Open Questions | If Zed never delivers a mid-turn prompt, the ACP ingress path is exercised only by tests + the simulator; the transport-neutral core still lands for TG-02. Planner may add a live-Zed UAT checkpoint |
| A3 | `max_per_session` default proposed as 50 (aligns with the existing `DefaultKeep = 50`) | Standard Stack / GC | Wrong default is a one-line config change (CONTEXT discretion) |
| A4 | A second snapshot-id family (e.g. `<sessionID>-pre-<NNN>`) is the cleanest D-09 realization vs minting turn-family ids | Pattern 5 / Pitfall 6 | Grammar choice affects List ordering and /undo walk logic; either works, but re-planning mid-execution is costly |
| A5 | `parked_ask` transcript kind name (16-D-20 names "steering_delivery, parked_ask records" verbatim in CONTEXT canonical refs) | Pattern 3 | Cosmetic; replay tolerance makes the exact name non-load-bearing |
| A6 | Steering/parked notes ride `agent_message_chunk` (the existing note family) rather than a new session/update kind | Pattern 6 / Don't-Hand-Roll | If the planner prefers a dedicated kind, the emitter gains one const + handle method (16-01 embedding precedent) |

## Open Questions (RESOLVED)

1. **Response semantics for a steering `session/prompt` request**
   - What we know: ACP v1 says nothing about concurrent prompts; each `session/prompt` MUST eventually return a stopReason (handlers.go:393-417 pattern). The turnMu currently makes the second prompt block until the turn ends.
   - What's unclear: should a steered prompt return immediately (stopReason end_turn after the queued note) or block until its steering is delivered (an ack)?
   - Recommendation: return promptly with the queued note + end_turn (keeps the editor responsive; the running turn's stream continues via the session-lifetime forwarder — WINDOWS #3 split). The transport-neutral API returns the ticket either way; the ACP adapter chooses.
   - **Resolution:** 23-02 Task 1 implements return-after-enqueue (queued note + end_turn-mapped stop, no turn-mutex acquisition) per this recommendation.
2. **Persistence home for `checkpoint.expiry_days` / `max_per_session`**
   - What we know: layer YAML tolerates a `checkpoint:` key (load.go verified); the typed Config drops it, so read-back uses the generic layer-map pattern.
   - Recommendation: persist via `WriteLayerOption` under a `checkpoint:` key path; read at GC/config time via `readLayerMap`; embedded floor holds the 7d + count defaults.
   - **Resolution:** 23-04 Task 2 implements exactly this — WriteLayerOption under `checkpoint:`, read-back via `checkpointGCBounds` over readLayerMap; the typed modelrouting.Config is deliberately NOT extended.
3. **Restore-guard scope for the v1.1 CLI (`ass-guard checkpoint restore`)**
   - What we know: the CLI is a separate process; in-process turn/chain state is invisible cross-process (the store lock serializes git ops only, store.go:158-166).
   - Recommendation: guards + D-12 apply to the agent's in-process restore path; the CLI keeps today's behavior and documents the caveat. Do not build cross-process turn detection.
   - **Resolution:** 23-03 flagged_assumptions + 23-04 flagged_assumptions record the in-process-only scope as a documented constraint; no cross-process turn detection is built.
4. **Engine-chain interaction with steering lines**
   - What we know: `LastTurnOutput` scans only assistant/ask_suspended terminal lines (enginebridge.go:225-306), so steering lines are invisible to engine Decide signals — chains won't misfire on them. Post-park injection turns hold `ParkMu` around `sess.Prompt` (enginebridge.go:182-189) so the boundary drain works unchanged.
   - Recommendation: no engine changes; add one E2E asserting a steered engine-chain turn applies steering and the chain completes.
   - **Resolution:** 23-02 Task 3 implements the engine-chain E2E (steering applies at the boundary AND the chain completes) with zero engine changes, pinning the LastTurnOutput kind-filter guarantee.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| git (subprocess) | All checkpoint work | ✓ | 2.50.1 (Apple Git-155) | none needed — platform git is the documented dependency (store.go:53-56) |
| Go toolchain | build/test | ✓ | go1.26.5 via mise | .mise.toml pins go = "1.26" |
| golangci-lint v2 | lint gate | ✓ (mise-managed) | v2 | mise ci blocks without it |
| a git-repo workspace | exclude-append feature | n/a (runtime condition) | — | skip + structured stderr note when `<workDir>/.git` absent (graceful) |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** not a git repo workdir — checkpointing still runs (store is workspace-local); only the user-repo exclude append degrades to a note.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | `go test` with `-race` (Go 1.26.5); testify present in go.mod |
| Config file | none (convention: `*_test.go` beside subjects); gate tasks in `.mise.toml` |
| Quick run command | `go test -race -count=1 ./internal/session/ ./internal/runtime/ ./internal/checkpoint/` |
| Full suite command | `mise ci` (vet + lint + build + test) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SEEDG-01 | Enqueue/drain/coalesce/ticket-cutoff incl. producer-during-cancel | unit (-race) | `go test -race -count=1 ./internal/session/ -run TestSteerQueue -x` | ❌ Wave 0 |
| SEEDG-01 | Drain applies at boundary; never mid-request; pairs intact (fake provider counts requests, captures shaped requests) | integration | `go test -race -count=1 ./internal/runtime/ -run TestBoundarySteering -x` | ❌ Wave 0 |
| SEEDG-01 | Parked-ask: visible note, cancelable, never blocks; steering+ask+cancel combined scenario | integration | `go test -race -count=1 ./internal/runtime/ -run TestParkedAskSteering -x` | ❌ Wave 0 |
| SEEDG-01 | Transport-neutrality: queue package imports no ACP package (compile-level assertion) | unit | `go test -count=1 ./internal/session/ -run TestSteerQueueNoACP -x` (or go vet/import check) | ❌ Wave 0 |
| SEEDG-02 | Restore refuses with active turn/chain; /undo auto-cancels (mutex-ordering behavioral test) | integration | `go test -race -count=1 ./internal/runtime/ -run TestRestoreGuard -x` | ❌ Wave 0 |
| SEEDG-02 | Pre-restore snapshot precedes restore; failed snapshot aborts restore (fail-closed) | unit | `go test -count=1 ./internal/checkpoint/ -run TestPreRestoreSnapshot -x` | ❌ Wave 0 |
| SEEDG-02 | Nested-repo refusal (fixture with nested repo + `.git` FILE variant) | unit (fs fixture) | `go test -count=1 ./internal/checkpoint/ -run TestNestedRepoRefusal -x` | ❌ Wave 0 |
| SEEDG-02 | Age+count GC (expireByAge, expireByCount, object expiry incl. reflog/gc invocation, session-start sweep) | unit | `go test -count=1 ./internal/checkpoint/ -run TestCheckpointGC -x` | ❌ Wave 0 |
| SEEDG-02 | `.ass-guard/` appended to USER `.git/info/exclude` (idempotent, existing lines preserved, non-repo degrade) | unit | `go test -count=1 ./internal/checkpoint/ -run TestUserRepoExclude -x` | ❌ Wave 0 |
| SEEDG-03 | /undo class-B: zero provider calls, D-05 shape, local_command line, byte-identical restore, /undo N walk (extends TestClassB battery) | integration | `go test -race -count=1 ./internal/runtime/ -run 'TestClassB.*Undo' -x` | ❌ Wave 0 (extends existing commands_test.go battery from 20-01/20-02) |

### Sampling Rate
- **Per task commit:** the task's `-run` test above (each < 30s)
- **Per wave merge:** `go test -race -count=1 ./internal/session/ ./internal/runtime/ ./internal/checkpoint/ ./internal/acpserve/`
- **Phase gate:** `mise ci` green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/session/steerqueue_test.go` — SteerQueue unit battery incl. -race producer/cancel hammer (SEEDG-01)
- [ ] `internal/checkpoint/store_test.go` extensions — GC, guards, nested-repo fixture (dir + `.git` file), exclude, pre-restore snapshot family (SEEDG-02)
- [ ] `internal/runtime/commands_test.go` extension — /undo battery cases (SEEDG-03)
- [ ] No framework install needed — existing go test infrastructure covers all phase requirements

## Security Domain

`security_enforcement` is not disabled in .planning/config.json — section required. This phase's security surface is workspace-destructiveness and data-integrity, not auth.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V2 Authentication | no | n/a (local stdio process) |
| V3 Session Management | no | n/a (session model unchanged) |
| V4 Access Control | yes (operator-authority actions) | /undo and restore are operator-typed, same trust as prompt (20-01 T-20-08 posture); restore guard refuses unsafe states; no new authority granted to model content — steering text reaches the model as user content only |
| V5 Input Validation | yes | Steering text is untrusted input: routed through the REDACTED transcript path (thinking-path exemption stays type-scoped — T-16-04 must never widen); marker wrapper must not be spoofable by model/tool output replay (steering_delivery lines are agent-appended only); /undo N args parsed + bounded (discretion covers 0/negative/beyond-depth) |
| V6 Cryptography | no | n/a (no new crypto; checkpoint ids remain regex-validated before any git refspec — T-14-01 discipline extends to the new id family) |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Destructive restore destroying concurrent human edits | Tampering | Pre-restore snapshot (D-09/D-12 ordering: snapshot FIRST), guard on active turns/chains |
| Nested-repo silent data loss (gitlink contents unprotected) | Tampering (data loss) | Refusal (D-12/SEEDG-02), detection incl. `.git` files; verified experiment documents the exact failure shape |
| Shadow store leaking into user repo (secrets in snapshots ingested by `git add -A`) | Information Disclosure | `.git/info/exclude` append (SEEDG-02) + existing shadow self-exclusion + 0700 store perms (store.go:90-94) |
| Steering text smuggling instructions that look like harness notices | Spoofing/Elevation | Marker is a RENDERING convention on agent-appended lines only; the model treats it as user speech (D-01); no tool/permission authority rides steering |
| Queue growth DoS (unbounded enqueues mid-turn) | Denial of Service | Coalesce (D-02) bounds request growth; bounded queue with drop+loud warning is the PicoClaw max-10 precedent if the planner wants a cap |
| GC destroying restorable state prematurely | Tampering | Age+count with conservative defaults; pre-restore snapshots share the same lifecycle (D-09) so undo-of-undo survives within the window; audit NEVER purged (18-D-09 family discipline) |

## Sources

### Primary (HIGH confidence)
- In-repo (all Read this session): internal/runtime/runtime.go; internal/session/{session,ask,projector,transcript,boundary,manager}.go; internal/checkpoint/store.go; internal/runtime/enginebridge/enginebridge.go; internal/acp/{handlers,types,emitter}.go; internal/acpserve/config_surface.go; internal/modelrouting/load.go; internal/checkpointcmd/checkpoint.go; internal/profile/corpus_scan.go; go.mod; .mise.toml
- In-repo planning: 20-01-PLAN.md / 20-02-PLAN.md (class-B contracts); 20-CONTEXT.md (D-05/D-06); 17-CONTEXT.md (D-11/D-12/D-13); 18-CONTEXT.md (D-01/D-09/D-12); 16-CONTEXT.md (D-20/D-22); .planning/research/PITFALLS.md Pitfall 13 (ticket/cutoff) + checkpoint-pitfalls section; ROADMAP §Phase 23
- Local experiment (2026-08-28, /tmp fixtures): gitlink 160000 + embedded-repo warning on `add -A`; checkout `unable to rmdir` + `clean -fd` non-descent on restore; `.git/info/exclude` append + `git check-ignore -v` confirmation
- https://agentclientprotocol.com/protocol/v1/prompt-turn — concurrent-prompt silence, session/cancel contract, updates-before-response (fetched verbatim)
- https://docs.picoclaw.io/docs/steering/ — boundary queue, user-message injection, coalesce modes, queue bound, skip-tail behavior (the deliberate divergence)
- https://docs.openclaw.ai/concepts/queue-steering — drain points, never-interrupt-in-flight-tool, append-only structurally-paired transcript
- https://github.com/agentclientprotocol/agent-client-protocol/discussions/1220 — session/inject is a v2 proposal; v1 has no mid-turn surface

### Secondary (MEDIUM confidence)
- Claude Code steering behavior: github.com/anthropics/claude-code issues #50246, #64624, #71726, #49373; Boris Cherny Threads post; r/ClaudeAI threads — queued mid-turn inputs; delivery timing version-dependent; ESC interrupts
- pi (badlogic/pi-mono) packages/coding-agent/docs/rpc.md + npm @earendil-works/pi-coding-agent — steering delivered after current tool executions, before next LLM call; Enter vs Alt+Enter (steer vs follow-up)
- strands-agents docs (strandsagents.com Interrupts + Steering plugin pages) — steering handlers = interrupt-and-pause family

### Tertiary (LOW confidence)
- None — no load-bearing claim rests on unverified sources; A2 (Zed client behavior) is explicitly logged as an assumption instead.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — stdlib + existing deps; no installs; versions probed locally
- Architecture: HIGH — every seam (Run ordering, runTurn loop, Projector, store, AskBroker, cancel contract, config surface) read in full this session with line-level citations
- Pitfalls: HIGH — code-level hazards derived from read source; external-semantics divergences verified against official docs; git behaviors verified by experiment
- Steering survey: MEDIUM — official docs for PicoClaw/OpenClaw/pi/strands; CC behavior is community-evidence only (logged A1)

**Research date:** 2026-08-28
**Valid until:** 2026-09-27 (in-repo seams stable within the milestone; external steering survey re-check only if ACP v2 inject lands)
