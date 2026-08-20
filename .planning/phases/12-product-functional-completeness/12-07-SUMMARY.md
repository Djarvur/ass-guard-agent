---
phase: 12-product-functional-completeness
plan: "07"
subsystem: acp
tags: [acp-04, d-02, cron, schedule-store, engine-driven-turns, catch-up, windows-3, completeness-gate]

requires:
  - phase: 12-product-functional-completeness
    provides: "12-05's fixture (the cron corpus_absent routing: zero successful cron calls captured — plan-mode-blocked; CronList's only capture is zcode's own zod defect, explicitly NOT mimicked)"
provides:
  - "The cron quartet (CronCreate/CronList/CronUpdate/CronDelete) executes against the REAL persisted per-project store (.ass-guard/schedule/schedules.json, 0600, atomic temp+rename, corrupt-quarantine) with the schema's local-tz + delayMinutes semantics honored and every form a documented corpus_absent default"
  - "internal/sched: the store + hand-rolled 5-field parser (lists/ranges/steps, local wall clock per the do-not-convert rule) + Due walker + ClaimForFire exactly-once claim + fire-once CatchUp with missed-window notes"
  - "The firing engine at the real wiring site (cmd/ass-guard): per-session turn mutex (queue-behind-active-turn), the serve-lifetime scheduler goroutine (30s tick, no daemon/no port), catch-up on the first active session, EngineDecision audit lines with automation provenance, the automation StartedBy vocabulary extension"
  - "WINDOWS #3 FIXED: session-lifetime chunk forwarder (acp.Server.Emitter + turnID-prefix filter + client-turn mute) — server-driven turns (timer resumes AND automation firings) mirror to the connected client"
  - "TestBackgroundWiring_CoreCompleteness extended to the FULL set — the cron exception dropped; zero non-mcp catalog dead ends remain (the permanent phase-gate regression test)"
affects: [12-08 (the eval net gates turn behavior this plan settles), Phase 13 (chaining rides the engine-driven-turn machinery)]

actuals:
  tokens: 61000   # chars/4 over the plan's production commits (estimate was 82000)
  tasks: 2
  commits: 5

tech-stack:
  added: []
  patterns:
    - "ClaimForFire: the exactly-once claim advances lastFired UNDER the store mutex BEFORE the turn runs — the due-walk, the tick, and the catch-up pass race safely (exactly one fires); a crash mid-turn consumes the window (D-02's fire-ONCE priority: at-most-once, never duplicate)"
    - "sessMu spans the WHOLE sessionFor construction: the async catch-up goroutine racing the first Run could double-construct a session (pre-existing unsynchronized sessions-map access, surfaced by 12-07 — Rule 1 fix)"
    - "Server-driven-turn client mirror: ONE session-lifetime forwarder filtered by turnID prefix (session-scoped), muted while a client Run is active (Run's own forwarder owns those chunks — no duplicates, client turns byte-identical)"
    - "Automation provenance is runner-side exclusive (the command-provenance discipline extended): model-visible content can never set StartedBy (T-12-07-03)"

key-files:
  created:
    - internal/sched/sched.go
    - internal/sched/sched_test.go
    - internal/coreexec/cron.go
    - internal/coreexec/cron_test.go
    - cmd/ass-guard/cron_wiring.go
    - cmd/ass-guard/cron_wiring_test.go
  modified:
    - internal/coreexec/planmode.go (InteractiveConfig.Schedule + the quartet registration)
    - internal/toolcat/coretools.json (CronUpdate joins the embedded core — see key-decisions)
    - internal/toolcat/catalog_test.go (core count pin 19→20, dated)
    - cmd/ass-guard/acp_serve.go (runner fields, Run serialization, sessionFor wiring, serve startup)
    - cmd/ass-guard/background_wiring_test.go (the FULL completeness gate)
    - internal/acp/server.go (Server.Emitter)
    - internal/session/manager.go (untouched; AppendEngineDecision reused as-is)

key-decisions:
  - "CronUpdate was MISSING from the embedded coretools.json (the captured catalog always carried the quartet; the embedded copy had three of four) — it joins verbatim from profiles/zcode/tools.json with sibling classification; the D-16 core-count pin moves 19→20 with a dated comment (the plan's '21 after the Phase-9 re-pin' anticipated growth; today's true count with the quartet is 20)"
  - "Every cron result form is a DOCUMENTED corpus_absent default (plain-string acks like TaskStop's, a JSON-array listing like TodoRead's) — the 12-05 harvest caught zero successful cron calls, and zcode's CronList zod error is deliberately NOT mimicked per the fixture note"
  - "MarkFired-before-turn (via ClaimForFire) over mark-after: D-02's exactly-once wins over at-least-once — a crash mid-fire consumes the window instead of duplicating it on recovery"
  - "The wiring lives at cmd/ass-guard (acp_serve.go + cron_wiring.go), NOT the plan's named internal/runtime/runtime.go — the plan itself said 'grep to locate'; sessionFor/Runner live in cmd/ass-guard (documented deviation)"

metrics:
  duration: ~130min
  completed: 2026-08-20

status: complete
---

# Phase 12 Plan 07: Cron — the persisted schedule + queue/fire-once engine-driven turns Summary

**One-liner:** The cron quartet executes against a real atomic per-project store, and due prompts fire as engine-driven serialized turns — queue-behind-active-turn, exactly-once catch-up with missed-window notes, full audit provenance, no daemon — plus the WINDOWS #3 client-mirror fix and the FULL catalog-completeness gate (zero non-mcp dead ends).

## What Was Built

### Task 1 — internal/sched + the quartet executors (tracer, TDD)

RED commits (8c8c747 sched battery, 343aec3 executor battery) → GREEN (f31e493, fb2f908):

- **internal/sched**: `ScheduleStore` (per-project JSON, 0600, temp+rename atomic, corrupt-quarantine + fresh-store persist, injectable clock); the hand-rolled 5-field parser (lists/ranges/steps, dow 0|7 Sunday, LOCAL wall-clock Next per the schema's "Do not convert to UTC", 366-day horizon); delayMinutes one-shots from the creation instant; `Due` (unfired slots at-or-before now); `CatchUp` (fire-once per missed window, lastFired persisted under the lock before events return).
- **coreexec/cron.go**: the four executors with the schema contract enforced (prompt+title required, cron XOR delayMinutes with the schema's bounds, recurring/maxRuns semantics); CronUpdate patches omit-to-preserve and returns the synchronized-title GUIDANCE note (advice, never a block — the no-confirmation-tier model); all forms documented corpus_absent defaults.
- **CronUpdate added to the embedded coretools.json** (see key-decisions) — the quartet now registers through RegisterInteractive's new `Schedule` config.

### Task 2 — the firing engine + the FULL gate (TDD)

GREEN (2b05848 + the race-hardened amend 3df577e):

- **Queue semantics**: the per-session turn mutex at the Run path — client turns, ask resumes, and firings serialize; a due firing waits for the active turn (proven via transcript ordering under a held lock).
- **Exactly-once**: `ClaimForFire` advances lastFired under the store mutex BEFORE the turn — the due-walk, the 30s tick, and the first-session catch-up race safely (the battery caught a real double-fire race during development; the claim kills it).
- **Catch-up**: first active session of a serve lifetime fires each missed automation once with the missed-window note; a fresh runner over the same workDir never re-fires (pinned).
- **Provenance**: EngineDecision audit lines (action names automation fire/catch-up/queued; the id in signal + reason) and the `automation:<id>` StartedBy vocabulary extension (runner-side exclusive, T-12-07-03).
- **No daemon/no port**: the scheduler is a serve-ctx goroutine, goroutine-exit on cancellation pinned; no net.Listen anywhere in the touched packages (structural check).
- **WINDOWS #3 FIXED**: `acp.Server.Emitter` + the session-lifetime forwarder — server-driven turns (D-01 timer resumes AND automation firings) now mirror to the client; client turns byte-identical (Run's forwarder owns them; the mute flag prevents duplicates). `TestCronWiring_ServerDrivenMirror` pins it.
- **The FULL completeness gate**: every non-mcp catalog tool carries Execute through the real per-session registration path — the cron exception dropped (the phase-gate dead-end check as a permanent regression test).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] unsynchronized sessions-map access (pre-existing, surfaced by 12-07)**
- **Found during:** Task 2 — the wiring battery intermittently read an empty transcript.
- **Issue:** `sessionTurnRunner.sessions` was read/written without a lock; the new async catch-up goroutine racing the first Run could double-construct a session (two transcripts, one silently overwritten).
- **Fix:** sessMu now spans the whole sessionFor construction; closeAllSessions/CloseSession snapshot under the lock.
- **Files modified:** cmd/ass-guard/acp_serve.go
- **Commit:** 2b05848

**2. [Rule 1 - Bug] fire-path wall-clock dependence + a double-fire race**
- **Found during:** Task 2 — the race battery (the mission's "only trustworthy signal").
- **Issue:** fireDueAutomations/runCatchUpOnce used real time.Now (tests became time-of-day dependent), and the claim-after-turn window let the catch-up pass and the due-walk both fire the same window.
- **Fix:** the fire path uses the store's injectable clock; ClaimForFire claims atomically before the turn.
- **Files modified:** cmd/ass-guard/cron_wiring.go, internal/sched/sched.go
- **Commit:** 2b05848

**3. [Rule 3 - Blocking] plan named internal/runtime/runtime.go as the wiring site**
- **Issue:** the package does not exist; sessionFor/Runner live in cmd/ass-guard/acp_serve.go (the plan itself instructs "grep to locate").
- **Fix:** wiring at the real site (acp_serve.go + the new cron_wiring.go sibling); test at cmd/ass-guard/cron_wiring_test.go.
- **Commits:** 2b05848

## Verification Evidence

- `mise ci` green (exit 0 — full race battery + whole-repo lint; log at /tmp/eval-net-evidence/12-07-mise-ci.log).
- go test ./internal/sched/ ./internal/coreexec/ ./cmd/ass-guard/ green including the queue/catch-up/provenance/mirror/lifecycle batteries and the FULL completeness gate.
- WINDOWS #3 marked fixed in the ledger with the wiring test as evidence.

## Threat-Model Mitigations Landed

- **T-12-07-01 (store injection):** 0600 + atomic writes + quarantine; every firing audited with automation provenance (attributable tampering).
- **T-12-07-02 (catch-up storm):** fire-once per window + serialized firings + the 1000-slot pathological bound in missedSlots.
- **T-12-07-03 (provenance spoofing):** StartedBy automation values are runner-side exclusive — transcript content can never set them (the adapter reads only the runner-owned field).
- **T-12-07-04 (goroutine leak):** serve-ctx parenting, exit-on-cancel pinned by test.
- **T-12-07-05 (firing evidence):** an EngineDecision line for every queue/fire/catch-up decision.

## Self-Check: PASSED

- internal/sched/sched.go, internal/coreexec/cron.go, cmd/ass-guard/cron_wiring.go + tests — FOUND
- Commits 8c8c747 / f31e493 / 343aec3 / fb2f908 / 2b05848 (amended to 3df577e) — FOUND in git log
