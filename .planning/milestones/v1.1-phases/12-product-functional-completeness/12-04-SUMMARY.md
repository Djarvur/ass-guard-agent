---
phase: 12-product-functional-completeness
plan: "04"
subsystem: acp
tags: [acp-02, acp-03, plan-mode, sendmessage, readsessioncontext, ask-seam, capture-grounded]

requires:
  - phase: 12-product-functional-completeness
    provides: "12-01's AskBroker suspension/resume seam + 12-05's re-record fixture (the plan-mode capture evidence)"
provides:
  - "EnterPlanMode/ExitPlanMode execute for real: the captured entered form, per-session PlanModeState with transcript plan_mode markers (audit lines, never boundaries), the approval on the 12-01 ask seam, and the CAPTURED runtime-level mutating-tool gate"
  - "SendMessage executes over the per-session AgentMailbox (agent_<uuid> space): byte-faithful delivery, honest queued-acks, structured unknown-recipient errors"
  - "ReadSessionContext executes over the .ass-guard/ transcript family: deterministic relevant/handoff excerpting bounded by maxTokens"
  - "RegisterInteractive — the interactive-family registration site (Execute-only) beside RegisterCore; engine TurnOutput.PlanMode as decision provenance"
affects: [12-06, 12-07, 12-08, 13-openspec-workflow-completion]

actuals:
  tokens: 58000   # chars/4 over the plan's production commits (estimate was 78000)
  tasks: 2
  commits: 5

tech-stack:
  added: []
  patterns:
    - "Kind-tagged PendingAsk: the same broker/seam serves questions AND plan approvals (the resume renders by Kind; an approval also flips state + writes the exit marker)"
    - "Audit-marker line types (ask_suspended, plan_mode) — schema-stable, Projector-skipped, NEVER boundary lines (no projection resets)"
    - "The plan-mode gate at the tool-loop batch-build site: catalog mutability + the captured extra-refused set"

key-files:
  created:
    - internal/session/planmode.go
    - internal/session/planmode_test.go
    - internal/coreexec/planmode.go
    - internal/coreexec/planmode_test.go
    - internal/coreexec/messaging.go
    - internal/coreexec/messaging_test.go
    - internal/engine/planmode_test.go
    - cmd/ass-guard/interactive_wiring_test.go
  modified:
    - internal/session/session.go
    - internal/session/ask.go
    - internal/session/manager.go
    - internal/coreexec/planmode.go (RegisterInteractive)
    - internal/engine/types.go
    - internal/engine/decide.go
    - cmd/ass-guard/acp_serve.go
    - internal/coreexec/testdata/zcode-interactive-results.json
    - internal/coreexec/testdata/zcode-recaptured-2026-08.json

key-decisions:
  - "ACP-02's scope question RESOLVED BY CAPTURE (the headline deviation): the 12-05 re-record PROVED zcode enforces plan mode at the tool-result level (mutating calls refused: 'Plan mode only allows read-only, non-destructive tools' / 'changes persistent state') — the plan's pre-capture 'model self-restraint, no gate' premise (incl. its Test 4) was superseded; ass-guard MIRRORS the target (mutating + SendMessage/TaskStop/cron set refused, isError, never executed) and Test 4 pins the CAPTURED behavior"
  - "The approval rides the 12-01 seam with PendingAsk.Kind=plan_approval: approve → the source-informed approved form (corpus_absent — the 12-05 hunt never observed it) + state OFF + exit marker; decline → the CAPTURED denial; D-01 timeout → the non-answer with the gate STAYING ON (an unapproved plan never ungates)"
  - "plan_mode markers are their OWN line type (the ask_suspended precedent) — NOT boundary lines: entering plan mode is not a mutating call and must never reset a projection window"
  - "SendMessage's ack is the honest queued form (source-informed corpus-absent); the mailbox's live-delivery fn is the dispatch-surface seam, completed agents record-only"

requirements-completed: [ACP-02, ACP-03]

duration: 92min
completed: 2026-08-20
status: complete
---

# Phase 12 Plan 04: Plan mode + agent messaging + session-context reads Summary

**The interactive-tool family executes for real: plan mode with the CAPTURED runtime-level mutating-tool gate (the 12-05 capture resolved ACP-02's scope question against the plan's pre-capture premise) and the approval on the proven 12-01 suspension seam; SendMessage over a per-session agent mailbox; ReadSessionContext over the product's own persisted transcripts — all four registered at one Execute-only site, fixture-pinned, mise ci green.**

## Performance

- **Duration:** 92 min
- **Tasks:** 2 (both TDD: RED 323ad8e/9ed71b8 → GREEN ae3e345/80e4870)
- **Files:** 17 (8 created, 9 modified)

## Accomplishments

- **Plan mode (ACP-02):** EnterPlanMode returns the captured guidance form (43 obs) + flips the session state with a `plan_mode_enter` marker; ExitPlanMode surfaces the plan as an approval question (Kind=plan_approval) and suspends — the reply or D-01 timeout resumes the SAME turn. The GATE: while ON, mutating calls + the captured refusal set (SendMessage/TaskStop/CronCreate/Update/Delete) return `Plan mode only allows read-only, non-destructive tools` (isError, never executed) — exactly the target's runtime enforcement, capture-pinned.
- **Engine provenance:** TurnOutput.PlanMode decorates every Decide path's Reason (`plan-mode ON; …`) — signal context only; the pinned test proves action/signal are unchanged by the flag.
- **Messaging (ACP-03):** AgentMailbox (per-session, agent_<uuid> space) with live deliver fns + record-only completed agents; SendMessage enforces the schema bounds, delivers byte-faithful, acks honestly (`Message msg_N was queued for local agent …` — source-informed corpus_absent), and errors structurally on unknown ids (never best-effort broadcast).
- **Session context (ACP-03):** SessionReader over `<workDir>/.ass-guard/transcript_<sess>.jsonl` — relevant (query-term matches with turn context) + handoff (bounded tail) strategies, maxTokens chars/4 with the documented `[truncated at maxTokens=N]` tail, the captured `ReadSessionContext returned lite context for <sess>.` head; deterministic, no LLM inside the executor.
- **Wiring:** RegisterInteractive (Execute-only) at sessionFor beside RegisterCore; the wiring tests prove all four Execute + an e2e delivery through the real catalog path.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1/capture authority - Design premise superseded] The plan-mode gate**
- **Found during:** 12-05's capture (immediately before this plan) — zcode REFUSES mutating tools in plan mode at the runtime level. The 12-04 plan's truth ("plan mode is model-facing state with NO tool-execution gating") and its Test 4 (pinning no-gating) were written pre-capture.
- **Fix:** the capture is the authority (the mimicry contract): implement the gate with the captured refusal form + the captured refusal set; Test 4 re-pinned to assert the CAPTURED behavior (a mutating call refused while ON, executing after an approval). This is NOT a confirmation tier (model-toggled state + deterministic policy; no human in the gating loop). Documented in planmode.go's scope note.
- **Commits:** ae3e345

**2. [Rule 3 - Blocking] Wiring-site location**
- The plan's files_modified name `internal/runtime/runtime.go` + `internal/runtime/interactive_wiring_test.go`; the actual sessionFor lives in `cmd/ass-guard/acp_serve.go` (the plan anticipated this: "grep to locate"). Wiring + tests landed at the real site.
- **Commits:** ae3e345, 80e4870

**3. [Scope note] Marker mechanism**
- The plan suggested markers "follow the boundary-line writer shape"; a REAL boundary line would reset the next turn's projection window (a request-shape change with no capture grounding). Markers use their OWN line type (`plan_mode`, the ask_suspended precedent — schema-stable, cause-named, Projector-skipped).

**4. [Scope note] Files beyond files_modified**
- `internal/session/{planmode.go,ask.go,session.go,manager.go}` + `internal/engine/{types,decide}.go`: the state/gate/markers/approval-rendering live at the session layer (the identity-binding rule — the loop is the one place that knows turnID+callID); the executor files render forms only. The interactive fixture + the re-record fixture carry the new families.

## Known Stubs

None. Corpus-absent forms ship as documented defaults with flags + hunts (the approved form, the SendMessage ack) — recorded in both fixtures.

## Self-Check: PASSED

- internal/session/planmode.go, internal/coreexec/{planmode,messaging}.go, cmd/ass-guard/interactive_wiring_test.go — FOUND
- Commits 323ad8e / 9ed71b8 / ae3e345 / 80e4870 — FOUND
- `mise ci` green (lint 0 issues + full battery)
