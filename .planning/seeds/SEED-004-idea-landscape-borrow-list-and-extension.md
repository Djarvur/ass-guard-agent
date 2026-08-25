---
id: SEED-004
status: dormant
planted: 2026-08-17
planted_during: v1.1 Kickoff & Peers / Phase 9 (legs complete, awaiting verification)
trigger_when: (a) Phase 12 planning (sandbox-flag honesty, evalset format, behavioral-eval references, Claudecourse checklist); (b) Phase 13 planning (per-command eval suites); (c) v1.2 pool planning (steering queue for Telegram, worktree isolation for hookdag fresh-context, MEM cluster study of letta/qwen, checkpoints decision); (d) every /gsd:new-milestone scan (landscape refresh); (e) before any "how do others solve X" research spike — check IDEA-LANDSCAPE.md first
scope: medium — four gap items (each small–medium, none violates an invariant) + a living doc to keep extended
audit_acknowledged:
  milestone: v1.1
  at: 2026-08-25
  status: dormant
---

# SEED-004: Act on the IDEA-LANDSCAPE borrow list — four gaps + the extension protocol for the 34-source reference doc

## Why This Matters

The 2026-08-17 investigation (`.planning/research/IDEA-LANDSCAPE.md`) compared ass-guard against 34 sources and concluded the plan is differentiated-and-ahead on its core (structural mimicry/parity/drift — no peer in the field) with exactly **four genuine gaps**, plus a ranked tail of refinements. None of the four violates a single project invariant — they are pre-authorized-compatible improvements waiting for the right planning moment. This seed exists so those moments find them automatically, and so the landscape doc keeps growing instead of ossifying (the operator explicitly intends to extend it).

## The four gaps (actionable summary — details + evidence in IDEA-LANDSCAPE.md §borrow-list)

1. **Checkpoints / undo (shadow-git)** — highest value. Snapshot workspace at turn boundaries; rollback surface (`ass-guard checkpoint` CLI + optional ACP/Telegram command). The no-confirmation-tier safety backstop the field universally has (cline, opencode `/undo`, gemini-cli, LangGraph checkpointer, ms time-travel). External git, no daemon, no port — invariant-clean. Decision point: any v1.x planning after Phase 13.
2. **Compaction policy — verify before designing.** Everyone has one; we have two-layer context but no summarization story. FIRST STEP: check whether the zcode profile already captures zcode's auto-compact behavior and `cache_control` breakpoint placement — if yes, mimicry delivers compaction for free and pi's extension-point shape is how profile-specific compaction plugs in. Tag onto the Phase-9 parity disposition or Phase-12 planning.
3. **Make the sandbox flag real.** Phase 12 already passes Bash `dangerouslyDisableSandbox` through for surface parity — the default case shouldn't be a fake. codex proves sandbox≠approval (macOS Seatbelt profile, Linux bwrap+seccomp). Parity-driven sandboxing: implement what zcode's tool semantics imply. Tag onto Phase 12's Bash background/sandbox work.
4. **Steering / input queue during a running turn.** Engine has cancel, no queue. Prerequisite-quality for the v1.2 Telegram peer (messages arrive mid-turn); pi + strands have the reference semantics. Tag onto ex-Phase-10 Telegram replan (v1.2 pool).

Ranked tail (7 more, in the doc): buffer-then-apply hookdag steps (gh-aw/plandex), worktree isolation for fresh-context steps, eval corpus+format (pi HF sessions, ADK `.evalset.json`, gemini-cli behavioral-evals), event/transcript schema versioning + conformance (pi-telemetry, ADK 2.0), architect-mode scheduler recipe (aider), supply-chain min-release-age (pi), Claudecourse-as-profile-coverage-checklist.

**2026-08-19 extension (ECOSYSTEM-AUDIT §3.3 items 12–14 ↔ IDEA-LANDSCAPE borrows #13–15):** three new borrows entered the landscape — **nightly upstream-parity gate** (occ ADR-001; re-run the EVAL/parity suite whenever the tracked target ships — the operational half of our parity harness), **scheduler outcome store + feedback loop** (occ `optimize/` cost-cascade; deterministic, zero LLM calls, invariant-clean; ECON extension), and the **Claude-Code surface census** (occ `env.mjs` ~100 env vars + 25 tool names + 6 permission-mode names + 4 MCP transports; free toolcat/profile/config coverage manifest). Triggers: Phase 8/12 planning (EVAL gate), scheduler work (feedback loop), any config-surface coverage pass. **Dispositioned same day against the adoption line (operator priority ruling: agent usable ASAP / important things before use / functionality+autotests):** #15 census → ADOPTED as a free verification input (Phase 14 EARLY-06 inventory + 12-06/12-07 catalog-completeness manifests, zero scope growth); #13 nightly gate → NOT pre-adoption (cheap zcode-version drift-warning slice → EARLY-03 planning input; full gate → 12-08 post-adoption; CI automation → v1.2); #14 outcome store → v1.2 pool. Recorded in ROADMAP Phase 14/12-08/v1.2-pool + REQUIREMENTS traceability note; the adoption line does not move.

## When to Surface

**Triggers:** Phase 8/12 planning (EVAL gate run on target release; parity-harness operationalization) · Phase 12 planning (items 2a/3 + tail 7/11) · Phase 13 planning (tail 7 eval suites) · scheduler work (outcome-store feedback loop) · v1.2 pool planning (item 4, tail 5/6 + MEM→letta/qwen study) · new-milestone scans (refresh pass) · any comparative research spike (read the doc before re-searching).

## Scope Estimate

**Medium** spread thin: each gap is a small–medium phase-plan-sized item; the doc extension is an ongoing light discipline (see protocol). No milestone-sized work hides in here.

## Breadcrumbs

- `.planning/research/IDEA-LANDSCAPE.md` — the investigation itself: 34-source tables (✅/📋/🧲/🚫), ranked borrow list (#13–15 added 2026-08-19), rejections-validated section, verdict; now carries the **Extension protocol** section
- `.planning/research/ECOSYSTEM-AUDIT.md` — §1.1 deep dives for OpenClaude + Open Claude Code (source-read 2026-08-19; redline 8: profile content derives only from the target's own wire traffic — fidelity rule; studying third-party code allowed per operator ruling 2026-08-19, with one guard: no mechanical porting from OpenClaude's unauthorized-redistribution tree)
- `.planning/seeds/SEED-002-…fantasy…` and `.planning/seeds/SEED-003-…landscape…` — companion seeds (fantasy depth; Go landscape)
- Phase 12/13 + v1.2 pool: `.planning/ROADMAP.md`; current position: `.planning/STATE.md`
- Gap-2 verification target: `internal/profile/` coverage manifest vs zcode compaction/cache_control behavior; `internal/parity/` curated suite
- Gap-3 target: Phase 12 Bash sandbox flag + `internal/coreexec/bash.go`; codex sandbox docs as reference
- Gap-4 target: `internal/engine/` (Decide/observe) + `internal/acp/handlers.go` cancel path; ex-Phase-10 plans in `.planning/phases/10-telegram-peer-text-voice/`

## Notes

Captured 2026-08-17 from the operator: "save this investigation for the future use and probably extending."

Disposition model: the four gaps are NOT roadmap commits — they are pre-analyzed proposals with triggers. At each trigger, the planner should decide adopt-now vs keep-dormant (e.g., checkpoints may wait for post-v1.1; steering should NOT miss the Telegram replan). The landscape doc is the single source of truth for the comparative detail; this seed is only the surfacing mechanism. Stats in the doc are point-in-time 2026-08-17 — re-verify before citing externally.
