# Phase 19: Compaction + cache_control - Context

**Gathered:** 2026-08-27
**Status:** Ready for planning

<domain>
## Phase Boundary

Long sessions stop silently losing earlier turns: threshold-triggered light-tier compaction appends an additive typed `compaction` transcript marker the Projector treats as a durable reset-point class (summary = seed immune to mutating-boundary resets), retry-once recovery on provider overflow, and `cache_control {"type":"ephemeral"}` ships on every system block. Compaction is an ass-guard-wanted CC-parity feature — the zcode corpus provably has none (docs/compaction-decision.md); cache_control is the corpus-proven parity lever.

</domain>

<decisions>
## Implementation Decisions

### Trigger & Measurement
- **D-01:** The ~80% threshold measures REAL provider-reported `input_tokens` from the last request (carried in the transcript boundary record) plus a conservative estimate for content added since, against `context_limit × threshold`. No tokenizer dependency — provider usage is ground truth.
- **D-02:** Pre-request check on every turn AND every tool-loop iteration: over threshold → compact first, then send. PAR-01's overflow retry-once stays as the backstop for estimation drift.
- **D-03:** `compaction.threshold_pct` (80 default) and `compaction.enabled` join the configOptions menu under Phase 16's rule (advertise all, apply-as-landed, persist-then-apply, effective values advertised).

### Reset-Point Context Shape
- **D-04:** Post-compaction context = `[summary] + [recent tail kept verbatim]`. Older history is replaced by the summary; the tail preserves current-task grip (CC behavior, canonical pattern). Tool_use/result pairs never split at the cut; thinking chains never rewritten.
- **D-05:** Tail size = BUDGET-FILL (operator override of fixed-N): after placing the summary, fill the remaining context budget with as much recent tail as fits, cut at pair boundaries. Real usage numbers anchor the fill math. — **Reversibility:** reversible — the fill policy is internal to the Projector's compaction cut; switching to fixed-N later changes only the cut computation.
- **D-06:** The durable seed (immune to mutating-boundary resets, PAR-01 letter) = the SUMMARY ONLY. The kept tail follows existing lean-seed/boundary discipline — a later mutating boundary may drop it; a later compaction re-summarizes it.

### Summarizer Execution
- **D-07:** Compaction runs BLOCKING inside the pre-request check: summarize → write marker → build request → send. No Projector races, no half-compacted states; cost is one visible pause when it fires (rare by design).
- **D-08:** The summarizer reads the transcript span since the PREVIOUS compaction marker (or session start): summary_N = f(summary_N-1 + span since). Everything about to be dropped is absorbed; chaining is well-defined across repeated compactions.
- **D-09:** Summarizer failure (error/timeout): skip compaction this turn, proceed un-compacted, ONE loud warning + counter, retry at the next pre-request check. The turn never fails over compaction; overflow retry remains the last line.
- **D-10:** The summarizer request rides the SAME provider pipeline as any turn — breaker, cost accounting, transcript local record with its own usage. No shadow path; visible in /cost and audit.

### Manual /compact + Cache Scope
- **D-11:** Manual `/compact` (Phase 20's class-B routing) uses THE SAME machinery: same summarizer, same marker, same seed semantics, runs immediately regardless of threshold. One implementation, two triggers.
- **D-12:** `cache_control` placement = system blocks ONLY, every block, `{"type":"ephemeral"}` — corpus-proven (910/910 placements), docs/compaction-decision.md disposition #1's routed scope (profile TextBlock gains the captured value; shaper maps it onto the wire). The `ass-guard parity` cache probe's expected-placement baseline stays unchanged. No speculative tools/messages placements.

### Claude's Discretion
- The conservative added-content estimate's formula (chars/4 class heuristic vs provider-quoted sizes).
- Summary prompt wording and maximum summary length.
- The compaction session/update note's exact shape (user-visible "compacting…" indication).
- Marker line placement relative to mutating-boundary lines in the same turn window.
- context_limit source per model (provider capability table vs config override).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §CC Parity Closures — PAR-01 (threshold compaction, marker class, pair atomicity, retry-once) and PAR-02 (cache_control on every system block) verbatim
- `.planning/ROADMAP.md` §Phase 19 — goal, 4 success criteria; note the phase is pulled ahead of its nominal slot because /compact (Phase 20) requires it and it touches the Projector Phase 18 disturbed

### Decision Artifacts (hard dependencies)
- `docs/compaction-decision.md` — THE settling artifact: zcode corpus has no auto-compaction/eviction (compaction is wanted-feature, not mimicry); cache_control corpus-proven system-only; disposition #1's routed scope (profile TextBlock value → shaper mapping); disposition #2/#3 already-delivered window facts
- `.planning/phases/16-acp-wire-foundation/16-CONTEXT.md` — D-21 (rich compaction marker: id, timestamp, usage, pre/post pointers), D-20 (additive-only transcript kinds — compaction is the new kind's first concrete instance), D-05 (configOptions menu rule D-03 joins)
- `.planning/phases/18-session-family/18-CONTEXT.md` — D-01 (transcript-as-truth: the marker and summary reconstruct on resume), replay-order interplay with the emitter

### External References
- `platform.claude.com/docs/en/build-with-claude/compaction` — summary+recent-tail canonical shape (D-04)
- `learn.microsoft.com/en-us/agent-framework/concepts/agents/conversations/compaction` — separate smaller model for summarization; logical-boundary cuts (D-05/D-07 family)

### Code Anchors
- `internal/session/projector.go` — MidTurnWindowMessages=64, boundMidTurn, lean-seed discipline (the machinery D-04..D-06 extend with a reset-point class)
- `internal/shaper/shaper.go` — where cache_control maps onto request system blocks (D-12's landing)
- `internal/profile` — captured corpus values (cache_control form) D-12 reads
- Boundary/usage transcript records (16-D-21) — D-01's real-usage source

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Projector's two-layer machinery (mid-turn window + lean seed) — compaction adds a third class (durable reset point), not a bypass; boundMidTurn's pair-safety bounding is the cut discipline D-05 reuses.
- Light-tier resolution (resolveSubagentModel's tier machinery) — the summarizer's model comes from the same tiers table.
- Provider pipeline (breaker, cost, local records) — D-10 rides it unchanged.
- Transcript additive-kind discipline — the compaction marker is a new kind under 16-D-20's additive-only weak schema.

### Established Patterns
- Graceful degradation + loud counters — D-09's skip-and-retry joins the family.
- Real-usage-as-truth (16-D-21 records) — D-01's measurement foundation.
- Verify-first corpus evidence (compaction-decision.md) — D-12 stays inside proven placements.

### Integration Points
- Pre-request check sits at the request-build seam (where the Projector output is assembled) — planner places it.
- configOptions registration (Phase 16's apply-as-landed registry) gains the compaction keys.
- Phase 20's class-B /compact handler calls this phase's public compaction entry point (D-11).
- `ass-guard parity` cache probe flips green when D-12 lands (criterion 4).

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- Budget-fill tail over fixed-N: retain as much verbatim recent context as the window allows rather than a fixed message count — maximizes current-task grip at the cost of cut-math complexity.
- Everything else endorsed as recommended: real-usage trigger, blocking execution, same-pipeline summarizer, single-machinery /compact, system-only cache placement.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 19-compaction-cache-control*
*Context gathered: 2026-08-27*
