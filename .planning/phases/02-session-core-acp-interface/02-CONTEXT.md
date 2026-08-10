# Phase 2: Session Core + ACP Interface - Context

**Gathered:** 2026-08-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2 wraps the Phase-1 mimicry machinery (Shaper + adapters + profile loading) in real session management and the IDE-native ACP interface. The goal (ROADMAP): a developer can spawn ass-guard from Zed via ACP, send a prompt, watch the model's output stream token-by-token, restart Zed (session persisted as a human-investigation artifact), all while the model sees a clean, lean context window at each command boundary. ass-guard becomes usable as a daily coding agent inside the IDE.

**In scope (18 REQ-IDs):**
- **Session Core (SESS-01..06):** two-layer context model (durable transcript + lean projected window), mutating commands as immutable boundaries, the effective-mutability formula, boundary-reset lean seed, Session Manager as sole transcript owner.
- **ACP interface (ACP-01, ACP-02, ACP-04, ACP-05):** stdio JSON-RPC server, lifecycle methods (initialize/session/new/session/prompt/session/cancel), end-to-end streaming, framing choice resolved.
- **Audit log completion (LOG-02, LOG-03, LOG-04):** async bus consumer, secrets redaction, rotation + reconstruction-sufficiency. (LOG-01 seeded in Phase 1.)
- **Parallelism (PARA-01..04):** Task/Agent subagent dispatch as isolated goroutine turn-loops, restricted tool subset, event-bus result tagging, panic recovery, provider-layer concurrency semaphore.

**Out of scope (v1 cut-line + deferred):**
- **ACP-03 (session/load replay) — DROPPED from v1.** The transcript's primary purpose is human investigation, not editor replay. See D-09 below. (v1 cut-line decision, like "Telegram deferred to v2.")
- Unified engine + hooks + autocontinue (ENG-01..05, HOOK-01..05 — Phase 4). Phase 2 provides the `session/cancel` *mechanism* but the engine that decides *when* to call it is Phase 4.
- OpenSpec hosting (OPEN-01..03 — Phase 4). Phase 2 builds the mutability-declaration interface forward-compatible with Phase 4's OpenSpec registration (D-18), but does not host OpenSpec.
- Model scheduling (SCHED-01..06 — Phase 3).
- Ecosystem compatibility (ECOS-01..05 — Phase 5).

**Mode:** mvp (ROADMAP.md Phase 2). Phase 1's test-harness loop (D-12) is *replaced* by the real Session Core; the Shaper + adapters + profile loading survive.

</domain>

<decisions>
## Implementation Decisions

### Context projection & lean seed (SESS-01/04/06)

- **D-01:** The lean seed (what the model sees after a boundary reset, SESS-04) contains: **system prompt (profile TIER-1 faithful) + one-paragraph task summary + active workspace context the new command references + ZERO carry-forward of prior turn messages.** The boundary is a hard cut; the summary is the bridge. The model knows WHAT it's doing and WHERE, but not the full conversation history.
- **D-02:** The task summary is produced by **mechanical extraction from the durable transcript**, NOT a model call. Extraction rule: last user message (current intent) + files touched (from tool calls) + last assistant message truncated to N chars. No model call → no latency, no mimicry divergence (a model-generated summary would be a behavioral divergence from zcode if zcode doesn't summarize at boundaries). Research defines the exact extraction rules and the N-char truncation policy.
- **D-03:** The durable transcript is **append-only JSONL** — every turn (user message, assistant response, tool call, tool result, boundary marker) appended as a structured JSON line. Single source of truth; the projector reads it to build the lean window. **PRIMARY PURPOSE: human investigation** — the transcript exists so the user can open/grep/review the conversation later. It is NOT primarily a replay datasource for the editor.

### Streaming path & backpressure (ACP-04)

- **D-04:** The event bus expands from Phase-1's single-event seed into **typed Go channels** — one channel per event type (`RequestShaped`, `AgentMessageChunk`, `ToolCall`, `ToolCallUpdate`, `UsageUpdate`, `SubagentResult`, etc.). Producers send; consumers (ACP adapter, audit/transcript writer) select on channels they care about. Idiomatic Go, type-safe, zero-dep. This is the spine of the streaming path AND the audit/transcript writer AND future subagent results.
- **D-05:** Backpressure is handled via **bounded buffer + block**. Each streaming channel is buffered to N (e.g. 64–128); if the buffer fills (consumer too slow), the producer blocks on send, which naturally slows the provider SSE read (Go's `io.Reader` blocks until consumed). Backpressure propagates end-to-end. A stuck client stalls the turn rather than dropping user-visible tokens or growing memory unbounded. Research picks the exact N per channel type.

### Session rehydration & boundaries (SESS-05, transcript-as-investigation-artifact)

- **D-06:** The transcript lives **per-project under `.ass-guard/`** in the working directory. Named by session-id + timestamp. Part of the workspace the developer investigates.
- **D-07:** `.ass-guard/` is kept out of git via a **self-gitignoring directory**: ass-guard creates `.ass-guard/.gitignore` (containing `*` + `!.gitignore`) on first run, so the dir exists in the repo but its contents are never committed. Matches the `.claude/` convention. LOG-03 secrets redaction still applies to transcript content as belt-and-suspenders.
- **D-08:** Boundary markers are **explicit JSONL lines** in the transcript: `{"type": "boundary", "cause": "mutating-command:Bash", "timestamp": ..., "command_ref": ...}`. First-class transcript entries written by the Session Manager when a mutating command completes. Inspectable, unambiguous, no re-derivation logic to drift. Needed for LIVE projection (boundary resets) even though replay is dropped (D-09).
- **D-09 (v1 cut-line — MAJOR SCOPE REDUCTION):** **NO REPLAY IN v1.** ACP-03 (`session/load` replays the durable transcript as `session/update` notifications) is **DROPPED from Phase 2 / v1 scope.** The transcript's primary purpose is human investigation (D-03), not editor replay. `session/load` is either not implemented or a no-op/new-session fallback. The transcript file is the history artifact regardless. SESS-05's *rehydration-on-session/load* part is MOOT; the *boundary-marker durability* part (D-08) is still needed for live projection. This removes the most complex part of the session-core (replay rehydration) from v1. *(User's explicit reframing: "the main idea behind the session log is not to replay it, but to provide user the history so it would be able to investigate the conversation later" + "we do not need replay at all in v1".)*

### Subagent isolation & concurrency (PARA-01..04)

- **D-10:** The profile declares the **full tool catalog** to the model (parent-side mimicry preserved); the subagent turn loop **restricts the execution subset at runtime.** If the subagent's model invokes a restricted tool, it gets a "tool not available" error and adapts. Restriction is a runtime constraint, not a catalog-shape difference — the model sees all tools, the subagent process enforces which can actually run.
- **D-11:** Subagent results return via **streamed intermediate progress**: the subagent streams tool calls + partial outputs to the event bus tagged with parent-turn-id (PARA-02) as it works; the parent observes and can forward to ACP `session/update` for user visibility. **RESEARCH NOTE — PARA-02 tension:** "only results, not accumulated context" still applies to the parent's **context window** (lean seed) — events stream for visibility, but the parent's window receives only the final result. Research must define the event-vs-context-window boundary precisely so the parent's lean window doesn't bloat from streamed intermediates.
- **D-12:** Outbound concurrency is bounded by a **single provider-layer semaphore, combined across parent + subagents** (PARA-04 verbatim). Configurable max (default conservative, e.g. 4–8). When full, new dispatches wait. Protects against fan-out explosions. Research defines the default + config surface.
- **D-13:** A panicking subagent is handled via **goroutine `recover()` → tool-error result** (PARA-03). Each subagent goroutine wraps its turn loop in `recover()`; any panic is converted to a structured tool-error result published to the event bus (parent-turn-id tagged); the parent receives the error as the subagent's result and adapts. Process NEVER crashes. The recover is at the goroutine boundary, not inside the turn loop.

### ACP server skeleton & framing (ACP-01/02/05)

- **D-14 (ACP-05 resolved):** The JSON-RPC framing is **hand-rolled** (~150 LOC per STACK). `bufio.Scanner` over stdin reads newline-delimited frames; a mutex-guarded writer writes frames to stdout; a method→handler map dispatches. Full control over ACP's notification semantics (notifications carry no `id`, no response). Zero deps. Matches the LSP-in-Go lineage. (Rejected: `kwo/jsonrpc2` fork adds a dep for a small protocol with unverified fit; `sourcegraph/jsonrpc2` is superseded, not for new builds.)
- **D-15:** The ACP server dispatch is a **single reader goroutine + per-prompt turn goroutine.** One goroutine reads frames from stdin and dispatches via the method→handler map. Each `session/prompt` spawns a concurrent turn goroutine (so `session/cancel` can be processed mid-turn). Requests carry `id` (get responses); notifications carry no `id` (no response). Method set: `initialize`, `session/new`, `session/prompt`, `session/cancel`, `session/load` (no-op per D-09), `logout`, `session/set_mode`.
- **D-16:** `session/cancel` works via **context cancellation + drain queued events.** Cancelling the active turn's `context.Context`: (1) aborts the in-flight provider HTTP request (context propagation through the SDK), (2) drains queued bus events for that turn (discarded + a "canceled" marker emitted), (3) the turn goroutine exits cleanly, (4) the transcript records a "canceled" boundary marker. Phase 2 provides the *mechanism*; Phase 4's unified engine decides *when* to trigger it. This is the user's only off-switch mid-turn (PROJECT.md: "manual cancellation is the only safety mechanism").

### Turn loop → Session Core integration (SESS-06)

- **D-17:** The Session Core **owns the transcript + projector + turn loop** (one component, one package — e.g. `internal/session`). The Session Manager is the sole transcript owner (SESS-06, append/read API). The projector builds the lean window from the transcript at boundaries. The turn loop is a method on the Session Core (not a standalone component). The Shaper + adapters from Phase 1 are dependencies, passed in or constructed once. Phase 1's test-harness loop (Phase-1 D-12) is replaced by this.
- **D-18:** The turn cycle (standard tool-loop): (1) Session Core projects lean window from transcript, (2) shapes request via Shaper (emits `RequestShaped` to bus), (3) sends to provider, (4) as chunks stream in, emits `AgentMessageChunk`/`ToolCall`/`UsageUpdate` to bus, (5) if response contains tool_calls, executes them (stubbed in Phase 1; real execution in Phase 4), appends results to transcript, loops to step 1 with updated window, (6) on `end_turn`, appends assistant response to transcript and returns. Session Manager writes every step to the append-only transcript.

### Mutability declaration interface (SESS-02/03, forward-design for OPEN-03)

- **D-19:** Mutability is a **per-tool field in the tool catalog** (`Mutability: 'mutating' | 'read-only'`), resolved by the SESS-03 formula at runtime: `mutating if (adapter-class(command) == mutating OR tool.Mutability == mutating) else read-only` — the more-mutating wins. Built-in catalog seeds defaults (Bash/Write/Edit = mutating; Read/Glob/Grep = read-only). Phase 4's OpenSpec adapter registers commands with their own Mutability field — **no boundary-engine change** (OPEN-03 single-source-of-truth satisfied). Config can ADD boundaries but cannot flip a declared-mutating tool to read-only (SESS-02 enforced structurally).

### Audit log = transcript (LOG-01..04)

- **D-20:** The audit log and the transcript are **ONE artifact.** The per-session JSONL under `.ass-guard/` captures everything (user input, verbatim shaped outgoing requests, tool calls, tool results, engine decisions, boundary markers) with LOG-03 secrets redaction applied per-line as written. The verbatim-request capture (LOG-01 mimicry evidence) is just another line type in the same file. Rotation = per-session files (a new session = a new file; natural boundary). Reconstruction-sufficiency (LOG-04) = the transcript + the profile artifact together reconstruct what happened. LOG-02 (async bus consumer) is satisfied because the transcript writer subscribes to the event bus async (Phase-1 D-13 seed expanded in D-04). One writer, one schema, one place to look — no two-writer drift.

### Claude's Discretion
None — every Phase-2 decision was explicitly user-selected or user-answered. (Contrast with Phase 1, where 7 decisions were Claude's-discretion due to user declining to answer.) The user was highly engaged across all eight Phase-2 areas and made every call.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 0 output (ground truth for ACP + transport discipline)
- `.planning/research/VERIFIED-FACTS.md` — Item #3 (ACP v1 method names + wire shape) is the load-bearing input for ACP-01/02/05: the full lifecycle `initialize → session/new → session/prompt`, the field name `agentCapabilities` (NOT `capabilities`/`serverInfo`), integer `protocolVersion: 1`, `session/update` as a notification (no `id`) with `sessionUpdate` discriminator, **newline-delimited JSON-RPC (NO Content-Length)**, and the framing rule ("MUST NOT write anything to stdout that is not a valid ACP message"). Item #1 (zcode JSONL schema) informs the transcript format. Item #5 (transport discipline) confirms stdout/stderr separation.

### Phase 1 output (the machinery Phase 2 wraps)
- `.planning/phases/01-mimicry-mvp-north-star-proof/01-CONTEXT.md` — **the primary input.** D-09 (Shaper hybrid SDK driving), D-12 (test-harness loop interface — Phase 2 REPLACES this per D-17), D-13 (event-bus seed with `RequestShaped` — Phase 2 EXPANDS this per D-04), D-14 (full tool catalog declared), D-15 (stubbed tool execution — Phase 2 keeps stubs; real execution is Phase 4).
- `.planning/phases/01-mimicry-mvp-north-star-proof/01-RESEARCH.md` + `01-PATTERNS.md` + the 6 plans — the researcher's analysis of the profile/Shaper/adapter structure that survives into Phase 2. The plan structure (tracer-first, wave-based) is the precedent.

### The 18 Phase-2 requirements (what this phase delivers)
- `.planning/REQUIREMENTS.md` §"Session & context (Phase 2)" (SESS-01..06), §"ACP interface (Phase 2)" (ACP-01..05 — note ACP-03 DROPPED per D-09), §"Audit log (Phase 1 + Phase 2)" (LOG-02..04 — LOG-01 is Phase 1), §"Parallelism (Phase 2 + Phase 4)" (PARA-01..04).

### Phase goal + success criteria
- `.planning/ROADMAP.md` §"Phase 2: Session Core + ACP Interface" — the 4 success criteria. Note: criterion #2 references ACP-03 replay; per D-09, the replay portion is out of v1 scope (the transcript persistence for human investigation remains). Mode: `mvp`.

### Project-level constraints (load-bearing)
- `.planning/PROJECT.md` — **Constraints** (single static binary; stdout = ACP only, stderr = logs — load-bearing for D-04/D-14/D-15; macOS+Linux), **Anti-Pattern on safety** ("manual cancellation is the only safety mechanism" — load-bearing for D-16), **v1 Cut-Line** (ACP-only interface; serialized deltas — Phase 2 does NOT build Phase 3/4 machinery).

### Research/stack references
- `.planning/research/STACK.md` §"Inherited Core Technologies" row for JSON-RPC framing (hand-rolled recommendation, ~150 LOC; `kwo/jsonrpc2` fork as fallback only). §Focus on the ACP transport + the event-bus/channel pattern.

### Codebase (greenfield + Phase-1 plans)
- The Phase-1 plans (`.planning/phases/01-*/01-{01..06}-PLAN.md`) define the package structure (`internal/session`, `internal/profile`, `internal/provider`, `internal/shaper`, etc.) that Phase 2 builds on. Phase 2 adds `internal/acp` (the ACP server), expands `internal/session` (the Session Core replaces the test-harness loop), and expands the event bus.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Phase-1 Shaper + adapters + profile loading** (planned in Phase 1, surviving into Phase 2): the Profile Shaper (Phase-1 D-09/D-10), the Anthropic-shape adapter (`anthropic-sdk-go`), the OpenAI-shape adapter (`go-openai`), and the profile artifact loader. These are the DEPENDENCIES the Session Core (D-17) takes in. They do not get rewritten in Phase 2.
- **Phase-1 event bus seed** (Phase-1 D-13): the minimal `RequestShaped` event type + async audit subscriber. Phase 2 EXPANDS this into the typed-channels bus (D-04) — the seed grows, not replaced.
- **spikes/03-acp-handshake/** — the Phase-0 throwaway spike that empirically validated the ACP v1 wire shape (initialize/session/new/session/prompt round-trip, `agentCapabilities` field name, newline-delimited framing). A reference for the hand-rolled framer (D-14), NOT code to import.

### Established Patterns
- **Transport discipline (load-bearing):** stdout = ACP frames ONLY; all logs to stderr (PROJECT.md, Phase 0 #5). Phase 2's event bus (D-04), transcript writer (D-20), and ACP adapter (D-15) MUST write to stderr for diagnostics, never stdout. Non-negotiable.
- **Serialized deltas (PROJECT.md):** Phase 2 does NOT build Phase 3 (scheduling) or Phase 4 (engine/hooks/OpenSpec) machinery. D-16 (cancel mechanism) is provided but the engine that triggers it is Phase 4. D-19 (mutability interface) is forward-designed for Phase 4's OpenSpec registration but Phase 2 doesn't host OpenSpec.
- **Append-only JSONL as the data format:** both the zcode rollout transcripts (Phase 0 #1) and the Claude-Code session files use JSONL. Phase 2's transcript (D-03/D-20) follows the same pattern — one line per event, append-only, human-greppable.

### Integration Points
- **ACP stdin/stdout ↔ Session Core:** the ACP server's reader goroutine (D-15) dispatches `session/prompt` to the Session Core's turn loop (D-17/D-18). The Session Core streams to the event bus; the ACP adapter subscribes and writes `session/update` notifications to stdout.
- **Event bus ↔ transcript writer:** the transcript writer (the audit log, unified per D-20) subscribes to the bus async and appends every event to the per-session JSONL under `.ass-guard/`.
- **Provider semaphore ↔ subagent dispatch:** when the `Task`/`Agent` tool dispatches a subagent (D-10/D-11), the subagent's provider calls go through the same semaphore (D-12) as the parent.

</code_context>

<specifics>
## Specific Ideas

- The user's pivotal reframing of the transcript's purpose — **"the main idea behind the session log is not to replay it, but to provide user the history so it would be able to investigate the conversation later"** followed by **"we do not need replay at all in v1"** — drove the single largest scope decision in Phase 2 (D-09, ACP-03 dropped). This reframing cascaded: it made SESS-05's rehydration part moot, simplified the transcript format (human-investigation-optimized, not replay-optimized), and removed the most complex session-core component from v1. This is the kind of scope reduction that makes the MVP achievable.
- The user chose **per-project `.ass-guard/`** storage (D-06) over central `~/.ass-guard/` — preferring the transcript to be part of the workspace the developer investigates, even though it requires gitignore discipline (solved by D-07's self-gitignoring dir).
- The user chose **streamed intermediate progress** for subagent results (D-11) over run-to-completion — accepting the PARA-02 tension (events stream for visibility, but the parent's lean window gets only the final result) in exchange for richer user visibility into subagent work. This is a deliberate enrichment over the minimal PARA-02 reading.
- The user chose **one artifact** (D-20, transcript = audit log) — refusing the two-artifact split that would create two-writer drift. Every line in the transcript IS an audit record; LOG-01..04 are satisfied by the one file.
- Phase 1's tool-catalog finding (catalog is NOT fixed 77 — varies per session 77/97/103; stable built-in core ~20 tools) propagates into Phase 2's D-10 (full catalog declared to the model; subagent restricts at runtime) and D-19 (mutability field on the per-tool declaration).

</specifics>

<deferred>
## Deferred Ideas

None raised as new capabilities during discussion. The following are noted as explicit v1 cut-lines (recorded so they're not lost):
- **ACP-03 session/load replay** (D-09) — dropped from v1; the transcript is the history artifact for human investigation. May be revisited in v2 if editor-replay becomes a requirement.
- **Real tool execution** (TOOL-04 concurrent reads / TOOL-05 swappable backends) — Phase 4. Phase 2 keeps tools stubbed (inheriting Phase-1 D-15) for the session-core mechanics; the engine + real execution land together in Phase 4.
- **Unified engine / hooks / autocontinue** (ENG/HOOK/LRN — Phase 4). Phase 2 provides the `session/cancel` mechanism (D-16) but not the engine that triggers it.
- **OpenSpec hosting** (OPEN-01..03 — Phase 4). Phase 2 forward-designs the mutability interface (D-19) so Phase 4 can register OpenSpec commands without engine changes.

</deferred>

---

*Phase: 2-Session Core + ACP Interface*
*Context gathered: 2026-08-10*
