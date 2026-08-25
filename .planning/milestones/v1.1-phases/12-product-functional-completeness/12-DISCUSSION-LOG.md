# Phase 12: Product Functional Completeness - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-17
**Phase:** 12-Product Functional Completeness
**Areas discussed:** AskUserQuestion hands-off policy, Cron firing semantics, Eval-net cost & gate triggers, Phase-9 follow-ups & /tmp archival

**Mode note:** Launched via the GSD manager (SDK-recommended inline discuss). The gray-area
selection AskUserQuestion received no operator response; per the harness directive and the
project's standing overnight-delegation autonomy pattern (2026-08-15), all areas were
auto-selected and resolved from locked prior decisions, tagged for retroactive confirmation
in CONTEXT.md. The auto-mode single-pass cap was honored (one pass, no re-analysis loops).

---

## AskUserQuestion hands-off policy (ACP-01)

| Option | Description | Selected |
|--------|-------------|----------|
| Configurable wait-then-shaped-non-answer | Wait N minutes (default 10, 0 = block forever); on timeout return the corpus's non-answer form (or documented corpus-informed default routed like ACP-07) so the model proceeds/declines | ✓ |
| Block-until-answered | Pure interactive semantics; hands-off runs dead-end on any model question | |
| Engine ask-state suspension | Unanswered question parks the session in an engine ask-state surfaced on resume | |

**User's choice:** (no response — auto-selected "Configurable wait-then-shaped-non-answer")
**Notes:** Block-forever recreates the Phase-8 stage-4 question-shaped-ending residual the requirement exists to fix. Surface route itself was already locked by ROADMAP (captured shape); only the non-answer policy was open.

## Cron firing semantics (ACP-04)

| Option | Description | Selected |
|--------|-------------|----------|
| Queue + fire-once catch-up | Active turn → queue behind it; agent not running → persist + fire once on next active session (lastFired dedupe, missed-window note) | ✓ |
| Skip-if-missed | Missed windows are dropped; only fires that come due while running execute | |
| Catch-up-all | Every missed occurrence fires (duplicate risk) | |

**User's choice:** (no response — auto-selected "Queue + fire-once catch-up")
**Notes:** No-daemon/no-port constraint preserved by construction; serialization mirrors the mutating-alone-in-slot discipline. Persistence: `.ass-guard/schedule/` JSON store, 0600.

## Eval-net cost & gate triggers (ACP-08)

| Option | Description | Selected |
|--------|-------------|----------|
| Change-class gate, k=1 | Scenario suites gate only on profile/model/turn-behavior diffs at pass@k=1 (~8–10 min, env-flag pattern); deterministic units in every mise ci; deeper k=3 local-only | ✓ |
| Every-commit full gate | Scenario suites on all commits (adds ~8+ min to every CI run) | |
| Units-only in CI | Scenario suites never gate, manual only (violates ACP-08's re-run gate requirement) | |

**User's choice:** (no response — auto-selected "Change-class gate, k=1")
**Notes:** Trigger set is fixed by the requirement text; the open question was cost placement. Phase-8 flagship E2E measured 474–505s — k=1 keeps the gate inside a tolerable budget.

## Phase-9 follow-ups & /tmp archival

| Option | Description | Selected |
|--------|-------------|----------|
| Archival quick task now; follow-ups as Phase-12 plan items | Mechanical copy of the driver kit immediately (reaping clock); re-record + extractor fixes become Phase-12 plans | ✓ |
| All as Phase-12 plan items | Everything waits for Phase-12 planning (risk: /tmp reaps first) | |
| Archival + re-record as one quick task | Also re-record expectations now (scope creep for a quick task; needs the eval-suite context) | |

**User's choice:** (no response — auto-selected "Archival quick task now; follow-ups as Phase-12 plan items")
**Notes:** Environment facts established during this area (verified, not derived): `/tmp/zcode-recapture/` alive (17 files, full request-record dumps); the pinned rollout file `model-io-sess_3cee56ae….jsonl` gone from all live locations (rollout dir rotated; only artifacts/agents/exec side-dirs remain) — so ACP-07's re-pinning primary path becomes the re-record route. 09-VERIFICATION.md already carries the rotation caveat.

---

## Claude's Discretion

Executor grouping for the 9 tools; plugin-precedence details (ACP-10); D-01 timeout default fine-tuning; schedule-store naming; salvage-check of the sess_3cee56ae side-dirs.

## Deferred Ideas

Native permission-surface re-shape (rejected for v1.1); pass@k>3 depth; advanced cron semantics; Time Machine salvage of the lost rollout file (operator-side, optional).
