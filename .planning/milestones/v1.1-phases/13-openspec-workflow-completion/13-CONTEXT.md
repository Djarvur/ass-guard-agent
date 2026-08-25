# Phase 13: OpenSpec Workflow Completion - Context

**Gathered:** 2026-08-17
**Status:** Ready for planning

<domain>
## Phase Boundary

The OpenSpec command matrix beyond the proven flagship loop — `new / continue / ff /
verify / bulk-archive / onboard` — drives E2E through ass-guard against the real
`openspec` binary with zero-continue chaining, question-shaped interactive dead-ends
routed through AskUserQuestion (consuming Phase 12's ACP-01), and per-command eval
suites extending Phase 12's regression net. The workflow half of the 2026-08-16
re-scope + split; absorbs ex-ACP-09.

</domain>

<decisions>
## Implementation Decisions

### E2E coverage (OS-01)
- **D-01:** **Full 2-path matrix** — every one of the 6 commands carries BOTH a happy-path and a fixable-failure E2E leg (12 legs total) in the gate. Evidence completeness is prioritized over gate time; fixable legs on lower-risk commands re-proving the recovery machinery is accepted cost. *[User-selected.]*

### Interactive dead-end routing (OS-02)
- **D-02:** **Model-initiated + engine advisory.** Skill/prompt seeds make the model aware AskUserQuestion exists (the model chooses it when it wants an answer). Additionally, on an unmatched question-shaped ending, the engine emits an **advisory** — audit-logged as an EngineDecision and client-visible (see D-05) — suggesting the model re-ask via AskUserQuestion. The advisory NEVER holds continuation; unmatched ⇒ nothing stays absolute; the Phase-8 rejection of auto-continue on question endings is untouched. *[User-selected.]*

### Seeding procedure (OS-02, rides Phase-8 D-12)
- **D-03:** **Harvest pass, then verify.** Pass 1 runs the full matrix unseeded (the gate-needed E2E anyway) and records actual stage-end outputs; chaining seeds are derived from those transcripts; pass 2 re-runs the gate and asserts zero-continue. Seeds provably come from real output (D-12 provenance), and the zero-continue claim is tested against fresh runs. *[User-selected.]*

### Eval suites (OS-03)
- **D-04:** **Per-command suites (6)** — one suite per command, each carrying its happy + fixable cases, feeding the Phase-12 D-03 change-class gate. Exact failure attribution: a gate failure names the failing command immediately. *[User-selected.]*

### Advisory surface & repeat policy (D-02 detail)
- **D-05:** The client-visible advisory is a **session/update note** after the turn ends (agent-message-style) — never an ACP user-message (transcript/session-load replay fidelity per 08 D-02 stays intact), never audit-log-only. **Dedupe per session + pattern-class**: the first unmatched question-shaped ending per class per session is client-visible; repeats increment the audit-log count only — no spam in long hands-off runs. *[User-selected.]*

### Gate flake policy
- **D-06:** **One re-run with no-delta proof.** A failed gated E2E leg may re-run ONCE, and only if zero source/config commits landed since the failure (the Phase-8 evidence class: FAIL run 1 → PASS re-run at a docs-only HEAD). Both runs are recorded; two failures = hard gate failure. No best-of-N (that would approximate pass-any-of-k and weaken what a green gate claims). *[User-selected.]*

### Verify semantics
- **D-07:** `/opsx:verify` **chains into the fix workflow** — a seeded handoff (e.g. verify → `/opsx:continue` or a new change, per harvest evidence) fires when the report identifies unimplemented specs. The mechanism stays the standard seeded-pattern machinery: real-output seeds, assistant-role-only matching, unmatched ⇒ nothing; the engine never freehand-interprets report semantics. *[User-selected — diverges from the terminus recommendation; the operator chose the ambitious route.]*
- Verify's own report-ending is a legitimate turn end, not a dead-end (the D-02 advisory does not fire on it when the chain's seed matched).

### Onboard boundaries
- **D-08:** `/opsx:onboard` runs **full-tilt in the E2E scratch projects** — real `project.json` writes and agent-tool wiring; its idempotent re-run (already-onboarded → clean reconciliation) is the fixable class. Scratch dirs are disposable; no blast-radius assertion beyond the existing scratch isolation (considered, not taken — v1.1 candidate if evidence demands it). *[User-selected.]*

### Claude's Discretion
- The D-11 mutability-table classification of each new command (onboard likely mutating — boundary at expansion; planner confirms per captured semantics)
- Exact advisory wording and its session/update field shape (capture-informed)
- The verify→fix handoff's concrete target command (driven by D-03 harvest evidence)
- Scratch-project fixture shape and matrix leg ordering (dependency-driven, planner territory)
- Seed-row syntax details (existing `[[command_patterns]]`/`[[patterns]]` forms in `internal/openspec/seeded.toml`)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` §"OpenSpec Workflow Completion" — OS-01..03 (locked scope)
- `.planning/ROADMAP.md` §"Phase 13" — goal, 3 success criteria, phase gate (incl. chaining-decisions-in-audit evidence requirement)

### Prior decisions that bind
- `.planning/phases/08-slash-command-kickoff/08-CONTEXT.md` — D-12 (seeds from real output), D-11 (mutating-command boundaries), the structural-safety invariants; STATE.md's findings-6 disposition (`TurnOutput.StartedBy` + `CommandMatcher` + seeded rows, consulted only after dual-signal miss) and the question-ending auto-continue REJECTION
- `.planning/phases/12-product-functional-completeness/12-CONTEXT.md` — D-03 (the eval gate these suites extend), D-01 (the AskUserQuestion surface this phase consumes); 12-01-PLAN (ACP-01 implementation)
- `.planning/phases/09-serve-path-audit-zcode-parity-re-capture/09-CONTEXT.md` — D-03 (engine-decision provenance — the advisory is a new EngineDecision class and must carry full provenance)

### Research ground truth
- `.planning/research/ECOSYSTEM-AUDIT.md` §4.4 — EVAL-01..03 (the net being extended)
- `docs/recapture-runbook.md` — capture discipline the harvest pass's transcripts follow

### Code ground truth
- `internal/engine/decide.go` + `observe.go` — where the advisory's new decision class lands
- `internal/openspec/seeded.toml` — existing `[[patterns]]` / `[[command_patterns]]` row forms
- `.planning/phases/08-slash-command-kickoff/08-06-*` + `08-08-*` — the E2E harness + gated-test pattern (`ASSGUARD_OPENSPEC_BIN=1`) this matrix reuses
- The installed `openspec` v1.5.0 binary surface (Phase-8 adapter probe pinned it — see `internal/openspec` probe records)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- The proven flagship-loop E2E harness (08-06 pattern): scratch project, real binary, gated env flags — the matrix legs clone it
- The chaining machinery: `TurnOutput.StartedBy` + `engine.CommandMatcher` + seeded rows — zero new mechanism needed for most handoffs
- Engine Decision audit lines (Phase 9) — the advisory's observability rail exists
- The openspec adapter + probe-pinned command surface

### Established Patterns
- Seeds from real run output, never command-file input text (D-12)
- Structural safety: assistant-role-only matching, unmatched ⇒ nothing, provenance unspoofable
- Gated real-binary tests via env flags; phase gate = `mise ci` + real-binary evidence
- Pass@k=1 in gate (Phase-12 D-03); pass^k consistency concern shapes the flake policy (D-06)

### Integration Points
- Engine decide path — one new advisory action class (audit + session/update note, deduped)
- The eval gate's change-class detector (Phase 12's 12-08) — the six new suites register into it
- The AskUserQuestion executor (Phase 12's 12-01) — consumed, not modified

</code_context>

<specifics>
## Specific Ideas

- The operator's evidence appetite: the full 2-path matrix and the ambitious verify→fix chaining were both chosen over cheaper/conservative recommendations — this phase prefers proof coverage where it costs gate time
- The advisory must read as the engine being helpful about routing, never as the chain being held open — the distinction is the safety story

</specifics>

<deferred>
## Deferred Ideas

- Engine-forced conversion of question endings into AskUserQuestion calls — rejected on safety grounds (Phase-8 class); do not revisit without an operator-authored invariant revision
- Best-of-N gate re-runs — rejected (pass-any-of-k weakens the gate's claim)
- ACP user-message advisory surface — rejected (transcript replay fidelity)
- Blast-radius assertion for onboard writes (path-escape check in E2E) — considered, not taken; v1.1 candidate if evidence demands
- Cross-command mega-suite — rejected (attribution)

</deferred>

---

*Phase: 13-OpenSpec Workflow Completion*
*Context gathered: 2026-08-17*
