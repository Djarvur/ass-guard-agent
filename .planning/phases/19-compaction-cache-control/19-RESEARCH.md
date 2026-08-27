# Phase 19: Compaction + cache_control - Research

**Researched:** 2026-08-27
**Domain:** Context-window compaction (client-side, transcript-grounded) + Anthropic cache_control emission in a Go agent runtime
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Trigger & Measurement**
- **D-01:** The ~80% threshold measures REAL provider-reported `input_tokens` from the last request (carried in the transcript boundary record) plus a conservative estimate for content added since, against `context_limit × threshold`. No tokenizer dependency — provider usage is ground truth.
- **D-02:** Pre-request check on every turn AND every tool-loop iteration: over threshold → compact first, then send. PAR-01's overflow retry-once stays as the backstop for estimation drift.
- **D-03:** `compaction.threshold_pct` (80 default) and `compaction.enabled` join the configOptions menu under Phase 16's rule (advertise all, apply-as-landed, persist-then-apply, effective values advertised).

**Reset-Point Context Shape**
- **D-04:** Post-compaction context = `[summary] + [recent tail kept verbatim]`. Older history is replaced by the summary; the tail preserves current-task grip (CC behavior, canonical pattern). Tool_use/result pairs never split at the cut; thinking chains never rewritten.
- **D-05:** Tail size = BUDGET-FILL (operator override of fixed-N): after placing the summary, fill the remaining context budget with as much recent tail as fits, cut at pair boundaries. Real usage numbers anchor the fill math. — **Reversibility:** reversible — the fill policy is internal to the Projector's compaction cut; switching to fixed-N later changes only the cut computation.
- **D-06:** The durable seed (immune to mutating-boundary resets, PAR-01 letter) = the SUMMARY ONLY. The kept tail follows existing lean-seed/boundary discipline — a later mutating boundary may drop it; a later compaction re-summarizes it.

**Summarizer Execution**
- **D-07:** Compaction runs BLOCKING inside the pre-request check: summarize → write marker → build request → send. No Projector races, no half-compacted states; cost is one visible pause when it fires (rare by design).
- **D-08:** The summarizer reads the transcript span since the PREVIOUS compaction marker (or session start): summary_N = f(summary_N-1 + span since). Everything about to be dropped is absorbed; chaining is well-defined across repeated compactions.
- **D-09:** Summarizer failure (error/timeout): skip compaction this turn, proceed un-compacted, ONE loud warning + counter, retry at the next pre-request check. The turn never fails over compaction; overflow retry remains the last line.
- **D-10:** The summarizer request rides the SAME provider pipeline as any turn — breaker, cost accounting, transcript local record with its own usage. No shadow path; visible in /cost and audit.

**Manual /compact + Cache Scope**
- **D-11:** Manual `/compact` (Phase 20's class-B routing) uses THE SAME machinery: same summarizer, same marker, same seed semantics, runs immediately regardless of threshold. One implementation, two triggers.
- **D-12:** `cache_control` placement = system blocks ONLY, every block, `{"type":"ephemeral"}` — corpus-proven (910/910 placements), docs/compaction-decision.md disposition #1's routed scope (profile TextBlock gains the captured value; shaper maps it onto the wire). The `ass-guard parity` cache probe's expected-placement baseline stays unchanged. No speculative tools/messages placements.

### Claude's Discretion
- The conservative added-content estimate's formula (chars/4 class heuristic vs provider-quoted sizes).
- Summary prompt wording and maximum summary length.
- The compaction session/update note's exact shape (user-visible "compacting…" indication).
- Marker line placement relative to mutating-boundary lines in the same turn window.
- context_limit source per model (provider capability table vs config override).

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description (REQUIREMENTS.md verbatim) | Research Support |
|----|---------------------------------------|------------------|
| PAR-01 | "Compaction on context overflow risk: threshold-triggered (~80% default, configurable) light-tier summarizer appending an additive typed `compaction` transcript line; Projector treats it as a hard reset-point class (summary = durable seed, immune to mutating-boundary resets); tool_use/result pairs atomic; retry-once recovery on provider overflow error; thinking blocks never rewritten mid-chain" | Trigger math (§Code Examples), reset-point projection semantics (§Pattern 1), pair-atomic cut discipline (§Pattern 2 + existing `boundMidTurn`), overflow surfacing + retry-once (§Pattern 4 + Pitfall 1), marker schema from Phase 16 (§Existing Code Facts) |
| PAR-02 | "`cache_control {"type":"ephemeral"}` emitted on every system block via the Shaper (parity-faithful lever; zcode corpus has no auto-compact — verified in docs/compaction-decision.md)" | Shaper landing (§Pattern 5), SDK constructor verification, probe flip path in `paritycli` (§Existing Code Facts), 4-breakpoint cap constraint (§Pitfall 2) |
</phase_requirements>

## Summary

This phase is greenfield feature work inside an already-mature two-layer context machinery. The Projector (internal/session/projector.go) builds every request window from the transcript; compaction adds a THIRD projection class — a durable reset point whose summary survives later mutating boundaries — without bypassing the existing mid-turn window or lean-seed discipline. The single chokepoint where every provider request originates is `Session.runTurn` (internal/session/session.go:340): every parent turn, every tool-loop iteration, every ask-resume re-entry, and (via the engine bridge, which delegates to `sess.Prompt` [VERIFIED: internal/runtime/enginebridge/enginebridge.go:191 — `stop, err := a.sess.Prompt(ctx, prompt)`]) every engine chain passes through it. Placing the blocking pre-request check at the top of its `for range maxIterations` loop satisfies D-02 completely.

Three verified gaps make this phase larger than "write the marker": (1) the provider's streaming path NEVER checks `resp.StatusCode` after `httpClient.Do` — a 400 "prompt is too long" response body is silently drained as non-SSE lines and surfaces as an empty `end_turn` (Pitfall 1), so overflow detection requires provider work first; (2) `paritycli.composeCacheProbeInput` hard-codes `CacheControl: false` with a comment explicitly routing the flag derivation to this phase [VERIFIED: internal/paritycli/parity.go:53-56]; (3) the Anthropic API caps cache_control at 4 breakpoints per request — a 5th returns 400 — while ass-guard's composed system array can exceed 4 blocks once dynamic merges (skills/agents listings, hook context) stack on the 3 captured blocks (Pitfall 2).

The default tiers table has NO `light` binding, so by default the summarizer runs on the parent model (GLM-5.3) — the light tier is config-conditional, exactly like subagent routing. `context_window` already exists per model in the modelrouting capability table (200000 for GLM-5.3), resolving the context_limit discretion item in favor of the existing table.

**Primary recommendation:** Land in four waves — (1) provider overflow surfacing (`StatusCode` check in `Stream` + an `IsOverflow` predicate), (2) the cache_control chain (profile field → shaper → probe derivation, closing WINDOWS #5), (3) the compaction engine (threshold check → blocking summarizer → marker → Projector reset-point class), (4) retry-once + config keys + the public `CompactNow()` entry point Phase 20 rides.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Threshold measurement (usage + estimate) | Session Core (internal/session) | Transcript Manager (reads usage/request_shaped lines) | Only the Session sees both the transcript and the request cycle; no tokenizer dependency per D-01 |
| Compaction orchestration (blocking check) | Session Core (`runTurn` loop head) | — | The single chokepoint every request already passes through; D-02's "every turn AND every tool-loop iteration" |
| Summarizer LLM call | Provider (via `Session.Provider`) | modelrouting (light-tier slug) | D-10: same pipeline as any turn; model override rides the per-call profile copy (subagentProfile precedent) |
| Compaction marker writing | Transcript Manager (`AppendCompaction` from Phase 16) | — | Sole-owner mutex append discipline; additive-only kind |
| Reset-point projection semantics | Projector (internal/session/projector.go) | — | The Projector is the sole producer of the model-visible window; the marker becomes a reset-point class there |
| Pair-atomic tail cut | Projector (`boundMidTurn`-style group advance) | — | The existing pair-safety bounding is the cut discipline D-05 reuses |
| cache_control emission | Shaper (internal/shaper/shaper.go) | profile (captured value) | Disposition #1's routed scope: profile TextBlock gains the value; shaper maps it onto the wire |
| cache_control capture/storage | internal/profile (TextBlock field + extractor/loader) | — | TIER-1 fidelity discipline; 910/910 corpus-uniform value |
| Probe flip (criterion 4) | internal/paritycli (composeCacheProbeInput) | internal/parity (class comparison, unchanged) | The probe's expectations stay untouched; only the composition's flags start reflecting the profile |
| Compaction config keys | modelrouting layered config + Phase 16 configOptions registry | — | D-03 joins the Phase 16 rule; persistence rides the existing config-write path (0600 discipline) |
| Manual /compact entry | Session public method (this phase) | Phase 20 class-B handler | D-11: one implementation, two triggers — the public entry point is exported now, Phase 20 calls it |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| anthropic-sdk-go | v1.63.0 (go.mod:8) | `anthropic.TextBlockParam.CacheControl` — the D-12 wire landing | Already the pinned SDK the Shaper shapes through; `NewCacheControlEphemeralParam()` marshals to exactly `{"type":"ephemeral"}` |
| Go standard library | go1.26.5 | Everything else (transcript I/O, thresholds, JSON) | The codebase's standing zero-new-dependency discipline |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| internal/modelrouting resolver | in-repo | Light-tier model slug + `CapabilityProfile.ContextWindow` | Threshold math and summarizer model; both resolved through the EXISTING tiers/capability tables |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Client-side compaction (locked D-04..D-06) | Anthropic server-side compaction beta (`compact-2026-01-12`) | Server-side drops everything before the compaction block and requires the pause-and-reappend dance; also not available on the Z.ai GLM endpoint. Client-side is the corpus-honest and endpoint-portable choice — matches the locked decision |
| Provider-usage ground truth (locked D-01) | Local tokenizer (tiktoken-class) | A tokenizer dependency contradicts D-01's letter and drifts across models; rejected by the decision itself |
| MS-style pipeline of strategies | Single summarization strategy + truncation backstop | ass-guard's overflow retry IS the backstop; a strategy pipeline is over-engineering for one compaction class |

**Installation:**
```bash
# No packages to install. Zero new dependencies.
```

**Version verification:** `go.mod:8` pins `github.com/anthropics/anthropic-sdk-go v1.63.0` [VERIFIED: go.mod:8, read this session]; module cache present at `~/go/pkg/mod/github.com/anthropics/anthropic-sdk-go@v1.63.0`; `go version` → go1.26.5 darwin/amd64; `golangci-lint version` → 2.12.2. The SDK type surface was read directly from the module cache (see Code Examples).

## Package Legitimacy Audit

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | — | — | — | — | — | No external packages are installed by this phase |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

*This phase adds zero dependencies: the SDK is already pinned, and all compaction machinery is in-repo Go.*

## Architecture Patterns

### System Architecture Diagram

```
                      ┌────────────────────────────────────────────────────────┐
                      │                    Session.runTurn                      │
                      │        (internal/session/session.go:340 — the           │
                      │   single chokepoint: parent turns, tool-loop iters,     │
                      │   ask-resumes, engine chains via sess.Prompt)           │
                      └────────────────────────────────────────────────────────┘
        entry: top of each maxIterations loop iteration
                          │
                          ▼
              ┌───────────────────────────┐   enabled?  └── no ──> Project → Shape → Stream (unchanged path)
              │ PRE-REQUEST COMPACTION    │
              │ CHECK (D-02, blocking)    │
              │ lastUsage (transcript     │
              │  usage line)              │
              │  + estimate(since last    │
              │  request_shaped) (D-01)   │
              │  vs contextWindow × pct   │
              └───────────┬───────────────┘
                          │ over threshold
                          ▼
              ┌───────────────────────────┐    failure (D-09): ONE warning + counter,
              │ SUMMARIZER (light tier    │◄────────────────────────── skip this turn,
              │  via s.Provider, same     │                             retry next check
              │  pipeline, D-10)          │
              │ summary_N = f(prev        │
              │  summary + span since     │
              │  prev marker, D-08)       │
              └───────────┬───────────────┘
                          │ summary text
                          ▼
              ┌───────────────────────────┐
              │ AppendCompaction marker   │  (Phase 16's kind: id + ts + usage
              │ (additive transcript      │   snapshot + pre/post pointers +
              │  line, D-20/16-D-21)      │   summary payload)
              └───────────┬───────────────┘
                          ▼
              ┌───────────────────────────┐      ┌──────────────────────────────┐
              │ PROJECTOR reset-point     │─────►│ request window =             │
              │ class (durable seed)      │      │ [summary seed] +             │
              │ - marker joins the        │      │ [budget-fill tail, pair-safe]│
              │   reset-point scan        │      └──────────────────────────────┘
              │ - seed = marker summary,  │
              │   immune to later         │
              │   TypeBoundary resets     │
              │   (D-06)                  │
              └───────────┬───────────────┘
                          ▼
              Provider.Stream ──► [overflow 400 "prompt is too long"?] ──yes──► force-compact
                          │              (Pitfall 1: must FIRST be surfaced      (bypass
                          │               as an error chunk — today it is          threshold),
                          │               silently drained)                        retry ONCE
                          ▼                                                    (criterion 3)
              Shaper.Shape ──► every system block carries
                              cache_control {"type":"ephemeral"}  (D-12, PAR-02)
                          │              [probe: paritycli derives flags from
                          ▼               profile → placement-vs-pin flips green]
              request log (capturer → request_shaped + body store)
```

### Recommended Project Structure
```
internal/session/
├── compaction.go          # NEW: threshold check, summarizer call, marker write,
│                          #      public CompactNow() (Phase 20 rides it, D-11)
├── compaction_test.go     # NEW: trigger math, blocking semantics, D-09 degrade,
│                          #      chaining (summary_N), marker write, tail cut
├── projector.go           # EXTEND: reset-point class (marker joins the boundary
│                          #      scan; durable seed; budget-fill tail cut)
└── projector_test.go      # EXTEND: boundary-survival + pair-atomicity pins
internal/provider/
├── streaming.go           # EXTEND: non-2xx status check → error chunk (Pitfall 1)
└── errors.go              # EXTEND: overflow predicate (message-class matcher)
internal/shaper/
└── shaper.go              # EXTEND: TextBlock.CacheControl → anthropic param (D-12)
internal/profile/
├── types.go               # EXTEND: TextBlock gains the captured value
└── extract.go             # EXTEND: capture cache_control from rollouts
internal/paritycli/
└── parity.go              # EXTEND: composeCacheProbeInput derives flags (probe flip)
```

### Pattern 1: The durable reset-point class (Projector)
**What:** The compaction marker participates in the between-turn reset-point scan alongside `TypeBoundary`, but with different seed semantics: when the last reset point before the projected turn is a compaction marker, the seed = the marker's SUMMARY (carried forward across any LATER mutating boundaries — the D-06 durability), and the tail is drawn from the marker's post-pointer budget-fill. When no marker exists, projection is byte-identical to today.
**When to use:** Always — this is the only mechanism by which compaction affects the window. There is no bypass.
**Why this shape:** `splitAtResetBoundary` currently resets "projections of turns that START after it" [VERIFIED: internal/session/projector.go:114-120 — "a boundary resets projections of turns that START after it, never the producing turn's own mid-turn window"]. A naive "marker = just another boundary" would lose the summary at the next mutating boundary — exactly what PAR-01's "immune to mutating-boundary resets" forbids. The Projector must explicitly prefer the most recent compaction marker's summary for the seed, regardless of later `TypeBoundary` lines, while the kept TAIL follows existing boundary discipline (a later boundary may drop the tail; only the summary is durable).

### Pattern 2: Pair-atomic budget-fill cut (D-05)
**What:** Fill the remaining context budget with the most recent tail, cutting only at group boundaries: the cut advances past tool-role messages so the tail never STARTS with an orphaned tool result.
**When to use:** At the compaction cut, and nowhere else.
**Why this shape:** It is the existing `boundMidTurn` discipline generalized from message-count to token-budget: "the cut advances past tool-role messages so the window never STARTS with an orphaned tool result (its assistant batch would be missing — pair-safety)" [VERIFIED: internal/session/projector.go:261-276]. Budget-fill replaces the fixed `MidTurnWindowMessages` count with a token computation anchored on real usage (D-05); the group-advance loop is reused. Thinking chains: PAR-01's "never rewritten mid-chain" is satisfied by construction — the Projector folds transcript lines mechanically and never rewrites assistant content; the cut only ever drops complete leading groups (and Phase 21's raw_thinking lines are inert to projection per Phase 16's tolerance tests).

### Pattern 3: The summarizer seam (D-08/D-10)
**What:** A `summarize()` helper that calls `s.Provider.Stream` with a light-tier profile COPY (model override via the `subagentProfile` precedent: "assigning prof.Model never writes back to s.Profile" [VERIFIED: internal/session/subagent.go:265-277]), consumes chunks WITHOUT publishing bus events, and appends its own usage line.
**When to use:** Both triggers (threshold check and manual `/compact`) — one implementation (D-11).
**Why this shape:** Reusing `streamAndEmit` would leak summarizer text deltas into the client-visible turn stream (`s.Bus.Publish(event.AgentMessageChunk{...})` [VERIFIED: internal/session/session.go:659-661]) — Pitfall 4. The summarizer input = previous marker's summary + transcript span since (D-08); everything about to be dropped must be absorbed. D-10's "same pipeline" concretely means: same `Session.Provider` adapter (same classification), same capturer (its request lands as a `request_shaped` line attributed to the current turn via `sess.CurrentTurnID()`), its own usage transcript line (visible to Phase 20's /cost). NOTE for the planner: the turn path does NOT ride the scheduler's breaker/cost wrapper today — `BuildWithCapturer` returns the raw adapter [VERIFIED: internal/modelrouting/factory.go:142-166] — so "breaker, cost accounting" means parity with whatever the TURN path does, which is capturer + usage records; do not invent a scheduler dispatch for the summarizer.

### Pattern 4: Overflow surfacing + retry-once (criterion 3)
**What:** (a) In `Stream`, after `httpClient.Do`, check `resp.StatusCode`; non-2xx → read the JSON error body, classify via `ClassifyHTTP` (400 → `KindStructural`), emit an "error" chunk, never drain it as SSE. (b) An overflow predicate matching the error message class "prompt is too long" (community-documented format: `prompt is too long: 200936 tokens > 199999 maximum`, riding 400 `invalid_request_error` [CITED: platform.claude.com/docs/en/api/errors — 400 = invalid_request_error for request-content issues; exact message text from community sources, MEDIUM]). (c) In `runTurn`, on an overflow-classified error: force compaction (threshold bypass) → re-project → retry ONCE; a second failure fails the turn.
**When to use:** Overflow only — NOT a general 400 retry (that would violate the typed-Kind discipline that engineered out retry storms).
**Why this shape:** Today a 400 never reaches the turn loop as an error at all (Pitfall 1). `ClassifyHTTP` maps 400 to `KindStructural` [VERIFIED: internal/provider/errors.go:148-151 — `structuralStatuses = map[int]struct{}{400: {}, 401: {}, 403: {}, 404: {}, 405: {}, 411: {}, 413: {}, 422: {}}`], so the overflow predicate must be a separate matcher over the ProviderError's cause/message, not a new Kind.

### Pattern 5: The cache_control chain (D-12, PAR-02)
**What:** `profile.TextBlock` gains the captured value (disposition #1's TIER-1 route) → the loader populates it → `Shaper.Shape` maps it onto `anthropic.TextBlockParam.CacheControl` → `paritycli.composeCacheProbeInput` derives its `CacheControl` flags from the profile instead of the hard-coded `false`.
**When to use:** Every system block, every request.
**Why this shape:** The corpus value is 910/910 uniform `{"type":"ephemeral"}` [VERIFIED: docs/compaction-decision.md §2 census rows — "`cache_control` value form | all `{\"type\":\"ephemeral\"}` | no other value observed"]. The SDK constructor marshals to exactly that form (see Code Examples). Storage shape is planner territory: since the value is corpus-uniform, a single profile.yaml-level declaration applying to every block carries the same information as per-block files; the loader currently reads blocks from `system/block-*.txt` with no per-block metadata channel [VERIFIED: internal/profile/loader.go:86-126 — blocks load as `TextBlock{Type: "text", Text: string(raw)}`].

### Anti-Patterns to Avoid
- **Compacting by mutating or rewriting transcript lines** — the transcript is append-only (D-20's additive-only weak schema); compaction APPENDS a marker and changes only FUTURE projections. Never edit history.
- **A second gate/decision pipeline for the summarizer** — it is a provider call like any turn; no separate retry policy, no separate cost ledger.
- **Cache breakpoints on tools or messages** — D-12 is explicit: system blocks ONLY. The probe reports any tools/message placement as "composed-has-pin-lacks" and FAILS.
- **Emitting summarizer chunks to the client bus** — the user's turn stream must not interleave summary-generation text (Pitfall 4).
- **Hand-rolling SSE error parsing from the 400 body** — read the JSON error envelope, classify via `ClassifyHTTP`, match the overflow class; do not pattern-match the whole body.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Pair-safe window bounding | New cut logic | `boundMidTurn`'s group-advance discipline (projector.go:261-276) | It already encodes the invariant; budget-fill generalizes the bound, not the mechanism |
| Token estimation | A tokenizer dependency | Provider usage + chars/4-class estimate (D-01's letter) | Tokenizer drift across models; provider usage is ground truth; MS Framework's own default is the 4-chars/token heuristic [CITED: learn.microsoft.com/.../compaction — CharacterEstimatorTokenizer] |
| cache_control wire form | Manual JSON injection | `anthropic.NewCacheControlEphemeralParam()` | SDK-native, marshals to exactly `{"type":"ephemeral"}`, `omitzero` TTL |
| Config persistence | New config file format | The modelrouting layered config + Phase 16's configOptions registry + existing config-write path (0600) | D-03 joins an existing menu rule; no new surface |
| Light-tier model resolution | New tier lookup | `resolveSubagentModel`'s resolver call shape (runtime.go:1175) | Same tiers table, same cross-provider degrade warning pattern |
| Marker id generation | New id scheme | The crypto/rand UUID pattern Phase 16's AppendCompaction already imports (16-02-PLAN Task 1) | Collision-by-construction; already decided upstream |

**Key insight:** every subsystem this phase touches (reset-point scanning, pair-safe bounding, tier resolution, config layers, probe comparison) already exists with the exact shape needed — the phase's genuine new design acts are exactly two: the durable-seed projection semantics and the budget-fill cut computation.

## Runtime State Inventory

> Greenfield feature phase (new transcript kind activation + new emission) — not a rename/refactor/migration. Step 2.5 SKIPPED by rule.
>
> One backward-compat consideration recorded for the planner: sessions created BEFORE this phase have no compaction markers; the Projector must treat "no marker present" as byte-identical-to-today projection (the additive-only guarantee Phase 16's tolerance tests already pin). Resume/replay of old transcripts must never require a marker.

## Common Pitfalls

### Pitfall 1: Provider 400s are currently SILENTLY SWALLOWED as empty end_turns
**What goes wrong:** `Stream` never inspects `resp.StatusCode` after `httpClient.Do` [VERIFIED: internal/provider/streaming.go:230-233 — the response goes straight to the drain goroutine]. A 400 "prompt is too long" body is plain JSON, not SSE; `drainSSE` skips every non-`data:` line, hits EOF, and `sendDone` defaults the empty finish reason to `"end_turn"` [VERIFIED: internal/provider/streaming.go:497-501 — `if finishReason == "" { finishReason = "end_turn" }`]. The overflow error criterion 3 must detect NEVER REACHES the turn loop today.
**Why it happens:** The streaming path was built for the happy SSE path; the watchdog covers mid-stream stalls, not request rejection.
**How to avoid:** Add the status check as the phase's FIRST provider task (the retry-once and the pre-request check both depend on errors being real). Test: httptest server returning 400 + the JSON error envelope → expect an error chunk, not a done chunk.
**Warning signs:** An E2E "overflow" test that passes while producing an empty assistant message.

### Pitfall 2: The 4-breakpoint API cap vs "every system block"
**What goes wrong:** The Anthropic API allows at most 4 cache_control breakpoints per request — a 5th returns 400 [CITED: platform.claude.com/docs/en/build-with-claude/prompt-caching]. The captured corpus is 3 blocks (main-role) / 4 (subagent-role) — every-block placement sits exactly at the cap. ass-guard's composed system array = 3 captured blocks + dynamic merges (skills/agents listings, hook context, Phase 21's AGENTS.md) — placing cache_control on EVERY block once merges stack exceeds 4 and 400s the request.
**Why it happens:** D-12's "every block" is corpus-proven at the corpus's own block count; ass-guard's dynamic merges are an ass-guard composition the corpus never shaped.
**How to avoid:** Place on every block while total breakpoints ≤ 4; beyond the cap, degrade deliberately (keep the LAST breakpoints — the deepest cache prefixes) and document the policy at the shaper site. The probe compares placement CLASSES, not per-block counts [VERIFIED: internal/parity/cacheprobe.go:261-281 — `placementClasses` sets `classes[classSystem] = true` if ANY system block carries it], so the probe stays green under either policy; the planner should still pin the degradation rule in a test.
**Warning signs:** Any test composing 5+ system blocks with unconditional per-block placement.

### Pitfall 3: The async usage-line race in threshold measurement
**What goes wrong:** Usage transcript lines are written ASYNCHRONOUSLY by the TranscriptWriter consuming the bus [VERIFIED: internal/session/transcript_writer.go:109-116 — the `usage` case calls `w.manager.AppendUsage(u.TurnID, u.InputTokens, u.OutputTokens)`]. A pre-request check reading `ReadAll()` immediately after a stream completes may race the writer and read a STALE last-usage value.
**Why it happens:** The one-writer/one-artifact discipline routes all streaming audit lines through the bus by design.
**How to avoid:** Either (a) read usage from the in-memory chunk at `streamAndEmit` time (the Session can record `lastInputTokens` when the "usage" chunk arrives — it already switches on it [VERIFIED: internal/session/session.go:675-681]) and let the transcript line remain the audit record; or (b) accept the race window knowing the conservative added-since estimate covers it (the missed delta is bounded by one request's growth). Option (a) is the clean fix; D-01's "carried in the transcript boundary record" is satisfied by the audit line, with the in-memory value as the check's read model.
**Warning signs:** A threshold test that intermittently under-fires under `-race` timing.

### Pitfall 4: Summarizer text leaking into the client turn stream
**What goes wrong:** Reusing `streamAndEmit` for the summarizer publishes `AgentMessageChunk` events — the user sees summary-generation text interleave with their turn.
**Why it happens:** `streamAndEmit` is the turn path's emitter by design.
**How to avoid:** The summarizer consumes its stream privately (Pattern 3): no bus publishes except its usage record; the user-visible signal is the D-09/D-11 "compacting…" note (a session/update the planner shapes — Claude's discretion item).
**Warning signs:** Any call to `streamAndEmit` or `s.Bus.Publish` inside the summarizer path.

### Pitfall 5: Compaction mid-subagent corrupting the subagent's window
**What goes wrong:** Subagent turns run their own `runTurn` on the SAME transcript with their own TurnIDs; `accumulateMidTurn` filters by TurnID, but a compaction marker appended during a subagent's turn becomes a reset point for turns starting after it. If the marker's post-pointer/tail math assumes the PARENT turn's lines, a subagent's projection could see a wrong seed.
**Why it happens:** The transcript is shared; the reset-point scan is global (by position), the mid-turn fold is per-TurnID.
**How to avoid:** Follow the boundary precedent exactly: the marker resets turns that START after it, never the producing turn's own mid-turn window; the durable-seed lookup is "most recent marker BEFORE the projected turn's user message." Pin a test: parent + concurrent subagent lines around a marker — the subagent window is unchanged, the next parent turn gets the summary seed.
**Warning signs:** A Projector test suite that only exercises single-turn transcripts.

### Pitfall 6: Golden/fidelity tests churn when emission lands
**What goes wrong:** `TestMidTurnCapture_GoldenShape` and any fixture pinning the full marshaled request body will see `cache_control` appear on system blocks. The text-level fidelity test is safe (it decodes only `text`+`type` fields) [VERIFIED: internal/shaper/fidelity_test.go:57-78 — its decode struct carries only Type/Text].
**Why it happens:** Emission changes the wire body by design.
**How to avoid:** Regenerate goldens deliberately in the emission task with the expected-diff discipline (goldens are supposed to change once, visibly); add a NEW pin asserting every system block in the golden carries `{"type":"ephemeral"}`.
**Warning signs:** A golden regenerated without an accompanying assertion of the new field.

### Pitfall 7: Estimating with the request's TOTAL size instead of the delta
**What goes wrong:** `request_shaped` lines carry `Bytes` — the FULL shaped body size [VERIFIED: internal/session/transcript.go:82 — `Bytes int `json:"bytes,omitempty"``]. Using the last request's Bytes as "added since" double-counts the whole conversation.
**Why it happens:** Field name suggests size, not growth.
**How to avoid:** The estimate is over TRANSCRIPT CONTENT appended since the last request (sum of line bytes / 4-class divisor, or the Bytes DELTA between consecutive request_shaped lines); the anchor is the last usage line's `InputTokens` [VERIFIED: internal/session/transcript.go:108-109 — `InputTokens int64 `json:"inputTokens,omitempty"`` / `OutputTokens int64 `json:"outputTokens,omitempty"``]. JSON envelope overhead makes a line-bytes estimate slightly conservative — which is the safe direction (fires early, never late).
**Warning signs:** A threshold check that fires only after the overflow already happened.

### Pitfall 8: Retry-once looping
**What goes wrong:** The overflow retry re-sends, gets the same 400 (compaction didn't shrink enough, or the overflow matcher misfired on a different 400), and loops.
**Why it happens:** Recovery paths without a bounded attempt count.
**How to avoid:** Exactly ONE retry per turn (criterion 3's letter); a second overflow fails the turn with the investigate-and-fix-ready error line. The retry counter resets per turn, not per iteration — pin it.
**Warning signs:** Any `for` around the retry; anything but a single boolean/attempt guard.

## Code Examples

### cache_control landing in the Shaper (verified against the pinned SDK)
```go
// Source: anthropic-sdk-go v1.63.0 module cache, message.go:481-485:
//   func NewCacheControlEphemeralParam() CacheControlEphemeralParam {
//       return CacheControlEphemeralParam{
//           Type: "ephemeral",
//       }
//   }
// and message.go:5646-5654 (TextBlockParam):
//   type TextBlockParam struct {
//       Text      string                   `json:"text" api:"required"`
//       Citations []TextCitationParamUnion `json:"citations,omitzero"`
//       CacheControl CacheControlEphemeralParam `json:"cache_control,omitzero"`
//       Type constant.Text `json:"type" default:"text"`
//   }
// TTL is `json:"ttl,omitzero"` — the zero value marshals to exactly {"type":"ephemeral"}.

// The Shape site today (shaper.go:99-102, verbatim):
//   systemBlocks := make([]anthropic.TextBlockParam, 0, len(p.System))
//   for _, b := range p.System {
//       systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: b.Text})
//   }
// The D-12 change at this site:
systemBlocks := make([]anthropic.TextBlockParam, 0, len(p.System))
for _, b := range p.System {
    tb := anthropic.TextBlockParam{Text: b.Text}
    if b.CacheControl { // profile-gated: absent value = today's byte-identical output
        tb.CacheControl = anthropic.NewCacheControlEphemeralParam()
    }
    systemBlocks = append(systemBlocks, tb)
}
// (bool-vs-raw-value field shape is the planner's call; the corpus value is
// 910/910 uniform {"type":"ephemeral"} so a presence flag carries full fidelity)
```

### The probe flip (paritycli, the routed seam's own comment)
```go
// Source: internal/paritycli/parity.go:53-56 (verbatim comment read this session):
//   The cache_control flags state TODAY'S truth — the shaper emits none
//   anywhere (14-03 CC-1, divergence-routed post-adoption); when the routed
//   emission fix lands, this seam derives the flags from the profile's
//   captured values instead.
//
// This phase IS that fix: composeCacheProbeInput's ProbeSystemBlock.CacheControl
// starts mirroring the profile field, and the standing placement-vs-pin FAIL
// (WINDOWS.md item #5) flips green. AssertPlacementAgainstPin and PinClasses
// stay untouched — the baseline is unchanged (D-12).
```

### Threshold check (D-01/D-02, placement at the runTurn loop head)
```go
// Anchor values, all read this session:
//   MidTurnWindowMessages = 64          [VERIFIED: internal/session/projector.go:32]
//   maxIterations = 64                  [VERIFIED: internal/session/session.go:355]
//   CapabilityProfile.ContextWindow    [VERIFIED: internal/modelrouting/config.go:58]
//   GLM-5.3: context_window: 200000     [VERIFIED: internal/modelrouting/defaults/config.yaml:27]
//   tiers: heavy: GLM-5.3, fallback glm-5.2 — NO light binding in defaults
//                                     [VERIFIED: internal/modelrouting/defaults/config.yaml:33-36]
//   tierLight = "light"                 [VERIFIED: internal/modelrouting/goconst_constants.go:13]

// inside runTurn's loop, before s.Projector.Project(turnID):
if s.compactionEnabled() {
    lastInput := s.lastReportedInputTokens()  // from the last usage chunk (Pitfall 3)
    est := s.estimateSinceLastRequest()       // chars/4-class over transcript lines since last request_shaped (D-01 discretion)
    if limit := s.contextLimit(); limit > 0 &&
        lastInput+est >= limit*s.thresholdPct()/100 {
        if cerr := s.compact(ctx, turnID); cerr != nil {
            // D-09: ONE loud warning + counter; the turn proceeds un-compacted.
            // The overflow retry (criterion 3) remains the last line.
        }
    }
}
```

### Marker consumption in the Projector (reset-point scan extension)
```go
// Today's scan (projector.go:141-147, verbatim):
//   boundaryIdx := -1
//   for i := range lines {
//       if lines[i].Type == TypeBoundary && (turnUserIdx < 0 || i < turnUserIdx) {
//           boundaryIdx = i
//       }
//   }
// Extension: the scan ALSO accepts the Phase-16 compaction kind (TypeCompaction
// = "compaction" per 16-02-PLAN/16-RESEARCH:302) as a reset point; when the
// winning reset point is a compaction marker:
//   - seed  = marker's summary payload (DURABLE — later TypeBoundary lines do
//     not displace it; D-06)
//   - tail  = budget-fill from the marker's post-pointer, cut at pair
//     boundaries (D-05), still subject to the lean-seed/boundary discipline
//   - when the winning reset point is a plain TypeBoundary AND a compaction
//     marker exists earlier: seed = that marker's summary + mechanically
//     extracted summary of post-marker pre-boundary lines (planner finalizes
//     the merge shape); tail from post-boundary lines as today
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Client-side-only compaction (CC's classic /compact + auto-compact) | Anthropic server-side compaction beta (`compact-2026-01-12`): API-side summary, drops pre-compaction content, `pause_after_compaction` for tail re-append | Beta current as of research date | Reference pattern only — Z.ai GLM endpoint parity and D-04's client-side lock make the server-side API irrelevant to this phase; its summary+recent-tail shape CONFIRMS D-04 |
| Fixed-N retained tail | Budget-fill tail (D-05, operator override) | This phase's decision | Retains maximal current-task grip; cut math anchored on real usage |
| Tokenizer-based triggers | Provider-usage-as-truth (D-01) | This phase's decision | No tokenizer dependency; estimation drift covered by retry-once backstop |
| Static per-request cache_control | Same (system-only, every block) — unchanged since the 14-02 corpus census | — | The corpus pin (910/910) remains the authority; the probe baseline stays byte-stable (D-12) |

**Deprecated/outdated:**
- None in-repo. The one SUPERSEDED expectation: `composeCacheProbeInput`'s hard-coded `CacheControl: false` — its own comment routes the change to this phase.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The exact overflow message text matches `"prompt is too long: N tokens > M maximum"` on the Z.ai GLM endpoint (community-documented for the direct Anthropic API; the 400/`invalid_request_error` class is officially documented, the exact wording is not) | Pattern 4, Pitfall 1 | Matcher misses → retry-once never fires; mitigate by matching the "prompt is too long" prefix case-insensitively and testing against the real endpoint once |
| A2 | The Z.ai GLM endpoint enforces (or at least accepts) the 4-breakpoint cap the same way the direct API does | Pitfall 2 | If Z.ai ignores breakpoints, over-placement is harmless but still a parity divergence; the cap-at-4 degrade is safe either way |
| A3 | Phase 16's `AppendCompaction` lands with fields sufficient for the summary payload (id, timestamp, usage snapshot, pre/post pointers) — the SUMMARY text's field/line home may need an additive extension from this phase | Pattern 1, Existing Code Facts | If the summary needs a new field, it is an additive Line extension under D-20's tolerance rule (cheap, but the planner should confirm what landed) |
| A4 | The summarizer defaulting to the parent model (no `tiers.light` binding in the shipped defaults) is acceptable operator-visible behavior (matching the subagent override's documented default) | Standard Stack | If the operator expects light-tier-by-default, a defaults/config.yaml `tiers.light` entry is a one-line follow-up |
| A5 | `compaction.threshold_pct` / `compaction.enabled` persist in the modelrouting layered config.yaml (global + project) behind the Phase 16 configOptions registry | Architectural Responsibility Map | If Phase 16's registry landed a different persistence home, the keys move there instead — planner verifies against the executed Phase 16 code |

## Open Questions

1. **Breakpoint degradation policy beyond 4 blocks**
   - What we know: the API cap is 4; the corpus is 3-4 blocks; dynamic merges can exceed 4 [CITED: platform.claude.com prompt-caching docs].
   - What's unclear: whether to keep the LAST 4 breakpoints (deepest prefixes) or only the final block when over cap; whether Z.ai enforces the cap at all.
   - Recommendation: keep the last 4 (deepest cache utility), document at the shaper site, pin with a test. This is an API-constraint accommodation of D-12, not a scope change — but the planner should surface it for operator visibility.
2. **Summary text's transcript home (marker payload vs sibling line)**
   - What we know: 16-D-21 names id/timestamp/usage/pre-post pointers; 18-D-01 requires marker+summary to reconstruct on resume.
   - What's unclear: whether Phase 16's executed `AppendCompaction` signature carries the summary.
   - Recommendation: read the landed Phase 16 code at plan time; extend additively if the field is missing (A3).
3. **Durable-seed merge shape when a mutating boundary follows a compaction marker**
   - What we know: D-06 — summary is immune; the tail follows existing discipline.
   - What's unclear: whether the seed after a later boundary is summary-ONLY or summary + mechanical post-marker summary.
   - Recommendation: summary-only (simplest reading of D-06's letter); the mechanical summary remains the no-marker path. Pin whichever the planner picks in the boundary-survival test.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | all tasks | ✓ | go1.26.5 darwin/amd64 | — |
| golangci-lint | `mise ci` lint gate | ✓ | 2.12.2 | — |
| mise | CI gate runner | ✓ | .mise.toml present (vet+lint+build+test) | `go test -race -count=1 ./...` directly |
| anthropic-sdk-go | D-12 emission | ✓ | v1.63.0 (go.mod:8, module cache present) | — |
| ZAI_API_KEY | live summarizer/turn E2E (operator-gated) | env-dependent | — | deterministic unit tests with fake providers (the established eval-suite gate-flag pattern) |

**Missing dependencies with no fallback:** none
**Missing dependencies with fallback:** none — all toolchain pieces verified present

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + `-race` (repo standard; testify NOT used — plain stdlib assertions throughout) |
| Config file | none needed — package-scoped tests; `.mise.toml [tasks.ci]` is the gate |
| Quick run command | `go test ./internal/session/ ./internal/shaper/ ./internal/provider/ ./internal/paritycli/ -count=1` |
| Full suite command | `mise ci` (= `go vet` + `golangci-lint run` + `CGO_ENABLED=0 go build ./...` + `go test -race -count=1 ./...`) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PAR-01 | Threshold math: lastUsage + estimate ≥ limit×pct fires; below does not | unit | `go test ./internal/session/ -run TestCompaction_Threshold -count=1` | ❌ Wave 0 (compaction_test.go) |
| PAR-01 | Marker = durable reset point: seed survives later TypeBoundary; no-marker transcripts project byte-identically | unit | `go test ./internal/session/ -run TestProjector_CompactionResetPoint -count=1` | ❌ Wave 0 (projector_test.go extension) |
| PAR-01 | Pair atomicity at the budget-fill cut: tail never starts with orphaned tool result | unit | `go test ./internal/session/ -run TestProjector_CompactionTailCut -count=1` | ❌ Wave 0 |
| PAR-01 | Summarizer failure: skip + ONE warning + counter; turn proceeds (D-09) | unit | `go test ./internal/session/ -run TestCompaction_SummarizerFailure -count=1` | ❌ Wave 0 |
| PAR-01 | Overflow surfaced: 400 JSON body → error chunk (not empty end_turn) | unit | `go test ./internal/provider/ -run TestStream_Non2xxError -count=1` | ❌ Wave 0 (httptest) |
| PAR-01 | Retry-once post-compaction on overflow; second overflow fails turn | unit | `go test ./internal/session/ -run TestCompaction_OverflowRetryOnce -count=1` | ❌ Wave 0 (fake provider) |
| PAR-02 | Every system block carries `{"type":"ephemeral"}` in the marshaled request | unit | `go test ./internal/shaper/ -run TestShape_CacheControlEmission -count=1` | ❌ Wave 0 |
| PAR-02 | Probe derives flags from profile; placement-vs-pin green on the pin fixture | unit | `go test ./internal/paritycli/ -run TestCacheProbe_PlacementFlip -count=1` | existing paritycli tests extend (cacheprobe_test.go exists) |
| PAR-01+02 | Long-session coherence (criterion 1) — model-in-loop | eval (gated) | `ASSGUARD_E2E_LLM=1` suite per `.mise.toml` eval-gate pattern | ❌ operator-gated, not in ci |

### Sampling Rate
- **Per task commit:** the task's package-scoped `-run` command (above) + `go vet ./...`
- **Per wave merge:** `go test -race -count=1 ./internal/... `
- **Phase gate:** full `mise ci` green before `/gsd-verify-work`; the parity probe flip additionally verified once against the committed pin fixture (`internal/profile/testdata/context-behavior/cache-control.jsonl` [VERIFIED: cmd/ass-guard/parity.go:60 — `filepath.Join("internal", "profile", "testdata", "context-behavior", "cache-control.jsonl")`])

### Wave 0 Gaps
- [ ] `internal/session/compaction_test.go` — covers PAR-01 trigger/degrade/chaining/marker-write
- [ ] `internal/session/projector_test.go` extension — reset-point + tail-cut + boundary-survival pins (criterion 2's "pinned Projector tests")
- [ ] `internal/provider/streaming_test.go` extension — non-2xx surfacing (httptest)
- [ ] `internal/shaper/shaper_test.go` extension — emission + over-cap degrade
- [ ] `internal/paritycli` probe-derivation test — the criterion-4 flip
- [ ] Windows ledger: close WINDOWS.md item #5 (standing cache-probe FAIL) when the flip lands

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | No credential surface change; API keys stay env/file (project decision) |
| V3 Session Management | no | Session lifecycle unchanged |
| V4 Access Control | no | No new authority paths |
| V5 Input Validation | yes | The summary is MODEL OUTPUT over UNTRUSTED tool-result content; it is appended through the Manager's per-line redaction chokepoint (LOG-03) like every other line — the marker's metadata rides the redacted append path per Phase 16's D-23 scoping (only raw_thinking is exempt). The summarizer prompt should demand extractive summarization (defense against tool-output-driven summary manipulation) |
| V6 Cryptography | no | No new crypto; marker ids reuse the crypto/rand UUID pattern |
| V12 File Handling | marginal | Transcript appends stay behind the 0600 sole-owner Manager discipline |

### Known Threat Patterns for {Go agent + LLM provider}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Prompt injection via tool output steering the summary (a malicious repo file's content reshapes the durable seed) | Tampering | Extractive summary prompt; the summary is data inside a user-role seed message, never executed; redaction chokepoint applies to the appended line |
| Summary content persisting secrets from earlier turns | Information disclosure | The Manager redacts every appended line pre-write [VERIFIED: internal/session/manager.go:76-101 — redaction happens before the write]; the summarizer's INPUT is already-redacted transcript content |
| Oversized summary bloating every future request (compaction that doesn't compact) | DoS | Maximum summary length is a locked discretion item — enforce a hard cap; marker usage snapshot makes regressions auditable |
| Overflow-error matcher abused by attacker-controlled error text (fake "prompt too long" from a tampered endpoint forcing compaction loops) | Tampering | Retry-once is bounded per turn (Pitfall 8); compaction is idempotent-ish (re-summarizes, never destroys — transcript is append-only) |

## Sources

### Primary (HIGH confidence)
- docs/compaction-decision.md (in-repo, read this session) — corpus census, 8 dispositions, the routed scope D-12 implements
- Internal code read this session: internal/session/{projector,session,manager,transcript,boundary,transcript_writer,subagent}.go, internal/shaper/shaper.go, internal/provider/{provider,errors,anthropic,streaming}.go, internal/profile/{types,loader,extract}.go, internal/parity/cacheprobe.go, internal/paritycli/parity.go, internal/modelrouting/{config,factory,defaults/config.yaml}.go, internal/runtime/runtime.go, internal/providerfactory/provider_factory.go, cmd/ass-guard/parity.go, profiles/zcode/ (profile.yaml + system/block-*.txt ×3)
- anthropic-sdk-go v1.63.0 module cache (message.go read directly) — TextBlockParam/CacheControlEphemeralParam shapes
- platform.claude.com/docs/en/build-with-claude/prompt-caching — 4-breakpoint cap, TTL options, placement surfaces
- platform.claude.com/docs/en/build-with-claude/compaction — server-side beta; summary+recent-tail canonical shape; cache_control-on-compaction-block guidance
- platform.claude.com/docs/en/api/errors — 400 = invalid_request_error envelope shape
- learn.microsoft.com/en-us/agent-framework/concepts/agents/conversations/compaction — atomic tool-call groups, separate smaller summarizer model, MinimumPreserved floor, 4-chars/token estimator
- .planning/phases/16-acp-wire-foundation/{16-CONTEXT.md, 16-02-PLAN.md, 16-RESEARCH.md} — the marker kind contract this phase consumes

### Secondary (MEDIUM confidence)
- dev.to "Best practices to handle prompts too long" + GitHub issues (hermes-agent #813, claude-code #57296) — the exact "prompt is too long: N tokens > M maximum" message wording (community sources; the 400/invalid_request_error class is officially documented, the wording is not)

### Tertiary (LOW confidence)
- None — no claim in this document rests on unverified training memory alone; the two community-sourced items are tagged in the Assumptions Log (A1, A2)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — zero new dependencies; SDK surface read from the pinned module cache
- Architecture: HIGH — every seam named was opened and read this session; the two genuine design acts are isolated and specified
- Pitfalls: HIGH — Pitfall 1 (400 swallowing) and Pitfall 2 (4-breakpoint cap) are load-bearing discoveries verified against source, not speculation

**Research date:** 2026-08-27
**Valid until:** 2026-09-27 (stable domain; re-verify only the two community-sourced items A1/A2 against the live Z.ai endpoint at execution time)
