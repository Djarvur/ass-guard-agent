# Phase 2: Session Core + ACP Interface - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-10
**Phase:** 2-Session Core + ACP Interface
**Areas discussed:** Context projection & lean seed, Streaming path & backpressure, Session rehydration & boundaries, Subagent isolation & concurrency, ACP server skeleton & framing, Turn loop → Session Core integration, Mutability declaration interface, Audit log vs transcript relationship

---

## Area selection (first round)

| Option | Description | Selected |
|--------|-------------|----------|
| Context projection & lean seed | SESS-01/04/06 — what goes in the lean seed, how projector decides, transcript structure | ✓ |
| Streaming path & backpressure | ACP-04 + event bus expansion, where backpressure lives | ✓ |
| Session rehydration & boundaries | SESS-05/ACP-03 — what editor sees on replay, boundary marker persistence | ✓ |
| Subagent isolation & concurrency | PARA-01..04 — restricted tools, result return, panic recovery, semaphore | ✓ |

**User's choice:** All four selected.

---

## Context projection & lean seed

### Lean seed contents

| Option | Description | Selected |
|--------|-------------|----------|
| System + task summary + workspace ctx (rec.) | System prompt + one-paragraph summary + active workspace + zero carry-forward | ✓ |
| System + new-command intent only | Maximally lean; model re-derives state | |
| System + summary + sliding N turns | Preserves recent continuity but risks drift back to bloat | |

**User's choice:** System + task summary + workspace ctx.

### Summary source

| Option | Description | Selected |
|--------|-------------|----------|
| Model-generated at boundary | Highest quality but adds model call (latency + mimicry divergence) | |
| Mechanical extraction (rec.) | Derive from transcript deterministically; no model call, no divergence | ✓ |
| Hybrid: mechanical + threshold-triggered | Mechanical default, model summary when transcript large | |

**User's choice:** Mechanical extraction.

### Transcript structure

| Option | Description | Selected |
|--------|-------------|----------|
| Append-only JSONL (rec.) | One structured JSON line per turn; matches zcode rollout familiarity | ✓ |
| SQLite database | Indexed queries, atomic transactions; infrastructure cost | |
| In-memory + snapshots | Fast but snapshot/rehydration complex; memory/disk divergence risk | |

**User's choice:** Append-only JSONL.

---

## Streaming path & backpressure

### Bus shape

| Option | Description | Selected |
|--------|-------------|----------|
| Typed Go channels (rec.) | One channel per event type; type-safe; zero-dep; idiomatic | ✓ |
| Generic pub/sub bus | Flexible but loses type safety; topic strings = coupling surface | |
| Direct write, no bus | Simplest but couples adapter to ACP framer; blocks PARA-02 later | |

**User's choice:** Typed Go channels.

### Backpressure

| Option | Description | Selected |
|--------|-------------|----------|
| Bounded buffer + block (rec.) | Buffer fills → producer blocks → slows provider read; backpressure end-to-end | ✓ |
| Bounded buffer + drop oldest | Never blocks but drops user-visible tokens | |
| Unbounded adapter queue | Never blocks but memory grows on slow reader; OOM risk | |

**User's choice:** Bounded buffer + block.

---

## Session rehydration & boundaries

### Replay scope — INITIAL decision (later overridden)

| Option | Description | Selected (initially) |
|--------|-------------|----------|
| Full replay, then project for model (rec.) | Editor reconstructs everything; model gets boundary-reset lean window | ✓ (initially) |
| Replay from boundary only | Lean view; loses pre-boundary history | |
| Full replay, capped at N turns | Pragmatic for long sessions | |

**User's choice (initially):** Full replay, then project for model.

### User's pivotal reframing

The user then provided a critical reframe: **"the main idea behind the session log is not to replay it, but to provide user the history so it would be able to investigate the conversation later."** This reframed the transcript's primary purpose as human investigation, not editor replay.

When asked whether this changes the replay-scope decision, the user escalated: **"I would say we do not need replay at all in v1."**

### Replay scope — FINAL decision (D-09)

| Option | Description | Selected (final) |
|--------|-------------|----------|
| Replay from boundary forward (rec. given reframe) | Aligns replay with transcript's primary purpose; restart lightweight | |
| Keep full replay | ACP-03 says "replays the durable transcript" | |
| Full replay, but human-file is canonical | Optimize transcript for humans; replay is secondary | |
| **NO REPLAY IN v1** (user's escalation) | ACP-03 dropped entirely; transcript is history artifact only | ✓ (final) |

**User's final choice:** NO REPLAY IN v1. ACP-03 dropped from Phase 2 / v1 scope.

**Notes:** This is a v1 cut-line decision (like "Telegram deferred to v2"). Removed the most complex session-core component from v1. SESS-05 rehydration-on-session/load part moot; boundary-marker durability part still needed for live projection.

### Boundary persistence

| Option | Description | Selected |
|--------|-------------|----------|
| Explicit boundary lines in JSONL (rec.) | First-class entries; inspectable; unambiguous; no re-derivation drift | ✓ |
| Implicit (derived from mutating calls) | Simpler write path but re-derivation must be identical on every read | |
| Sidecar index file | Two files to sync; complexity for single-owner structure | |

**User's choice:** Explicit boundary lines in JSONL.

### Transcript location

| Option | Description | Selected |
|--------|-------------|----------|
| Per-project under .ass-guard/ | Part of workspace; gitignore discipline needed | ✓ |
| Central under ~/.ass-guard/ (rec.) | Mirrors zcode layout; git-clean; consistent across repos | |
| Configurable (default central) | Most flexible; config surface for rarely-changed path | |

**User's choice:** Per-project under `.ass-guard/`.

### Git safety

| Option | Description | Selected |
|--------|-------------|----------|
| Self-gitignoring dir (rec.) | `.ass-guard/.gitignore` with `*` + `!.gitignore`; matches `.claude/` convention | ✓ |
| Add to root .gitignore | Modifies user's gitignore; some dislike | |
| Document only | Relies on human discipline for safety-critical default | |

**User's choice:** Self-gitignoring dir.

---

## Subagent isolation & concurrency

### Tool restriction

| Option | Description | Selected |
|--------|-------------|----------|
| Full catalog declared, subagent runtime restricts (rec.) | Parent mimicry preserved; subagent enforces execution subset; model adapts to error | ✓ |
| Per-subagent restricted catalog | Faithful to restriction concept but catalog shape differs from zcode | |
| No restriction in v1 (defer PARA-01) | Simplest but removes safety property | |

**User's choice:** Full catalog declared, subagent runtime restricts.

### Result return

| Option | Description | Selected |
|--------|-------------|----------|
| Run-to-completion, publish result (rec.) | Cleanest PARA-02 reading; parent blocks, gets final result | |
| Stream intermediate progress | Richer visibility; parent can forward to ACP; muddies "only results" | ✓ |
| Result to parent + own transcript for humans | Parent gets results, humans get full history | |

**User's choice:** Stream intermediate progress.

**Notes:** Deliberate enrichment over minimal PARA-02. RESEARCH FLAG: PARA-02 "only results not accumulated context" still applies to the parent's CONTEXT WINDOW — events stream for visibility, but the parent's lean window receives only the final result. Research must define the event-vs-context-window boundary.

### Concurrency bound

| Option | Description | Selected |
|--------|-------------|----------|
| Provider-layer semaphore, combined bound (rec.) | Single semaphore, parent + subagents; PARA-04 verbatim | ✓ |
| Separate parent/subagent bounds | More granular but PARA-04 says "across parent + subagents" | |
| No bound in v1 (defer PARA-04) | Simplest but fan-out hits rate limits, cascades | |

**User's choice:** Provider-layer semaphore, combined bound.

### Panic recovery

| Option | Description | Selected |
|--------|-------------|----------|
| Goroutine recover() → tool-error result (rec.) | PARA-03 verbatim; recover at goroutine boundary; process never crashes | ✓ |
| Propagate to parent recover | Simpler but subagent can corrupt shared state before parent catches | |
| Recover + log to transcript for diagnosis | Adds diagnostic value aligned with transcript purpose | |

**User's choice:** Goroutine recover() → tool-error result.

---

## Area selection (second round — additional areas)

| Option | Description | Selected |
|--------|-------------|----------|
| ACP server skeleton & framing | ACP-01/02/05 lifecycle, hand-rolled framing, session/cancel | ✓ |
| Turn loop → Session Core integration | How test-harness loop gets replaced; turn cycle structure | ✓ |
| Mutability declaration interface | SESS-02/03 forward-design for Phase 4 OpenSpec registration | ✓ |
| Audit log vs transcript relationship | One artifact or two; LOG-01..04 resolution | ✓ |

**User's choice:** All four selected.

---

## ACP server skeleton & framing

### Framing (ACP-05)

| Option | Description | Selected |
|--------|-------------|----------|
| Hand-rolled (rec.) | ~150 LOC; full control over notification semantics; zero deps | ✓ |
| kwo/jsonrpc2 fork | Saves ~150 LOC but dep for small protocol; unverified fit | |
| sourcegraph/jsonrpc2 | Superseded; not for new builds | |

**User's choice:** Hand-rolled.

### Dispatch structure

| Option | Description | Selected |
|--------|-------------|----------|
| Single reader goroutine + per-prompt turn goroutine (rec.) | One stdin reader; session/prompt spawns turn goroutine; cancel works mid-turn | ✓ |
| Goroutine-per-connection | Collapses to single-reader for stdio anyway | |
| Synchronous read loop | Blocks cancel during long turns; unsuitable | |

**User's choice:** Single reader goroutine + per-prompt turn goroutine.

### session/cancel

| Option | Description | Selected |
|--------|-------------|----------|
| Context cancellation + drain queued events (rec.) | Abort provider, discard queued, record boundary, exit clean | ✓ |
| Abort provider only, flush queued | Simpler but may send trailing tokens after cancel | |
| Defer cancel to Phase 4 | Removes user's only off-switch mid-turn | |

**User's choice:** Context cancellation + drain queued events.

---

## Turn loop → Session Core integration

### Ownership

| Option | Description | Selected |
|--------|-------------|----------|
| Session Core owns transcript+projector+loop (rec.) | One component, one package; SESS-06 sole owner; Shaper+adapters as deps | ✓ |
| Separate components wired together | More decoupled but SESS-06 must be enforced across boundaries | |
| Monolithic engine | Simplest conceptually but violates Phase-1 survival separation | |

**User's choice:** Session Core owns transcript+projector+loop.

### Turn cycle

| Option | Description | Selected |
|--------|-------------|----------|
| Standard tool-loop cycle (rec.) | Project → shape → send → stream → tool calls → loop until end_turn | ✓ |
| Single-shot per prompt | Simpler but doesn't match how coding agents chain tool calls | |
| Tool-loop, all tools serialize (Phase 2 conservative) | Conservative but TOOL-04 concurrent reads is Phase 4 | |

**User's choice:** Standard tool-loop cycle.

---

## Mutability declaration interface

| Option | Description | Selected |
|--------|-------------|----------|
| Tool-catalog field + runtime formula (rec.) | Per-tool Mutability field; SESS-03 formula at runtime; Phase 4 registers via field | ✓ |
| Separate mutability registry | Decoupled but risks drifting from catalog; violates OPEN-03 single source | |
| Hardcode now, refactor in Phase 4 | Simplest but OPEN-03 not designed forward | |

**User's choice:** Tool-catalog field + runtime formula.

---

## Audit log vs transcript relationship

| Option | Description | Selected |
|--------|-------------|----------|
| One artifact: transcript = audit log (rec.) | Per-session JSONL captures everything; redaction per-line; natural rotation | ✓ |
| Two artifacts: transcript + audit log | Separation of concerns but two writers, two schemas, drift risk | |
| Transcript primary, audit log derived | One source of truth; audit log is generated index | |

**User's choice:** One artifact: transcript = audit log.

---

## Claude's Discretion

None — every Phase-2 decision was explicitly user-selected or user-answered. The user was highly engaged across all eight areas. (Contrast with Phase 1, where 7 decisions were Claude's-discretion.)

## Deferred Ideas

None raised as new capabilities. Explicit v1 cut-lines recorded:
- ACP-03 session/load replay (D-09) — dropped from v1
- Real tool execution (TOOL-04/05) — Phase 4
- Unified engine/hooks/autocontinue (ENG/HOOK/LRN) — Phase 4
- OpenSpec hosting (OPEN-01..03) — Phase 4

---

*Phase: 2-Session Core + ACP Interface*
*Discussion date: 2026-08-10*
