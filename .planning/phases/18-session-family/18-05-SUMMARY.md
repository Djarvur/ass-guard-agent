---
phase: 18-session-family
plan: 05
subsystem: acp-sessions
tags: [acp, session-load, reconciliation, resume, kill9, sigkill, modes, available-commands, d01, d02, d03]

# Dependency graph
requires:
  - "18-01 load/replay spine — Runner.ResumeSession, ReplayTranscript, the D-03 loading/ready gate"
  - "18-02 reconciliation engine — Reconcile/Seed/InterruptedCause + Manager.AppendSynthetic"
  - "16-01 ordered TurnEmitter + Server.Emitter — the path replay rides"
  - "16-02 registry — the dead-ask late-response resolution surface"
  - "17-02/17-03/17-04 ask machinery — ask_suspended lines + the gated suspension family"
provides:
  - "Runner.ResumeSession as the FULL transcript-side resume (D-01): ReadAll -> session.Reconcile -> AppendSynthetic per provenance-marked closure -> SeedResume(maxTurns, planMode)"
  - "Session.SeedResume(maxTurns, planMode) — the row-6/7 state seed; Session.ModesState() building the v1 SessionModeState shape"
  - "The completed load core: typed already-active double-registration guard, modes from the ModeStateProvider capability, available_commands_update re-advertisement after the last replayed frame (acp.CommandSource seam + the acpserve adapter)"
  - "acp.KindAvailableCommandsUpdate + AvailableCommandFrame (the first available_commands_update emission in the codebase — Phase 20 reuses the seam)"
  - "internal/acpserve/kill9_test.go — TestKill9Resume: the re-exec child-process SIGKILL matrix (classes 1-5, 8-9, ask-interplay, kill-during-replay)"
affects: [18-06-resume-cli, 20-commands, ACP-06]

# Actuals (#2632) — chars/4 over the realized diff (109,902 diff chars on internal/), same scale as the plan's estimate
actuals:
  tokens: 27475
  tasks: 2
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Optional-capability seams stay the runner->server contract language: ModeStateProvider (seeded modes) joins SessionLoader/SessionCloser/AskDrainer; CommandSource joins SessionStore/ConfigSurface (composition-root adapters under 25-D-13)"
    - "The registration guard is ONE critical section: the live-check and the loading-begin share s.mu so no window exists where a concurrent load slips past both; the closure append makes re-load observably destructive, so already-live refuses (typed already-active)"
    - "Determinism in process-kill tests: suspension waits are stub-hold channels, wire frames, or bounded polls of the transcript FILE for the last synchronous line before the dangler — never a blind sleep; the only deliberate wait is the SIGKILL round-trip"
    - "The re-exec child pattern: TestMain branches on an env var into the real Run composition over process stdio; the parent's stub provider survives child death and its call counter spans children (the zero-provider-calls pin)"

key-files:
  created:
    - internal/acpserve/kill9_test.go
    - internal/acpserve/command_source.go
    - internal/runtime/resume_session_test.go
  modified:
    - internal/runtime/runtime.go
    - internal/acp/handlers.go
    - internal/acp/server.go
    - internal/acp/types.go
    - internal/acp/emitter.go
    - internal/session/session.go
    - internal/session/planmode.go
    - internal/session/reconcile.go
    - internal/acpserve/acp_serve.go
    - internal/acp/replay_test.go
    - internal/acp/session_family_test.go

key-decisions:
  - "The double-registration contract changed from 18-01's idempotent-success to the plan-pinned typed already-active error: with closures now APPENDED by ResumeSession, a second load of a live id would re-replay frames onto a live session — refusal is the observable-safe side; the busy error (one load at a time) stays for concurrent second loads"
  - "modes semantics: non-null exactly when the transcript carries a plan_mode line (Seed.PlanModePresent); currentModeId from the LAST line's cause (enter -> \"plan\", exit -> \"default\"); the v1 shape is availableModes [default, plan] + currentModeId (schema-verified this session: SessionModeState = availableModes[]+currentModeId — NOT a modeId/state pair)"
  - "available_commands_update lands as the FIRST emission of that kind in the codebase via a new optional CommandSource seam: the plan assumed session/new already advertised commands, but ACP-04 is Phase 20 — the seam + acpserve adapter (runner's ecosys registry) gives Phase 20 the same source; with no source wired the frame still fires with the empty set (full-replacement semantics keep the client coherent)"
  - "PlanModeState gains used-tracking: a session whose LAST transition was an exit still reports the v1 shape with default current (it USED modes); only never-touched sessions report null"
  - "The ask-interplay row drives the PERMISSION family (gated Write + Bash in one response -> two ask_suspended, one dialog open one queued) — the queue's one-outstanding discipline gives the 'with queued asks' premise deterministically"
  - "The dead ask's late response resolves through Registry.Deliver's unknown-id path (logged + dropped): the dying process's cascade died with it, the fresh registry never minted that id — the harness proves no-wedge (the follow-up prompt works) rather than asserting a specific cascade frame"

patterns-established:
  - "Pattern: resume order is Runner-first, wire-second — ResumeSession appends closures + seeds BEFORE ReplayTranscript reads the file, so closures replay as ordinary terminal frames (D-02 subtle-in-UI, unambiguous-on-disk)"
  - "Pattern: exactly one turn-suffix scanner — Reconcile delegates to seed.go's MaxTurnCounter (the 18-02 same-wave duplication retired)"
  - "Pattern: process-kill harness bounds are load-tolerant (30s frame waits, 15s transcript polls) — under the full -race suite's parallel load a healthy child takes seconds per phase; the bound fails a wedge, it never races a healthy flow"

requirements-completed: [ACP-06]

# Coverage metadata (#1602)
coverage:
  - id: D1
    description: "Runner.ResumeSession is the full transcript-side resume: closures appended on disk with interrupted provenance, second classification clean (idempotent), turn counter + plan-mode state seeded from the transcript alone"
    requirement: ACP-06
    verification:
      - kind: unit
        ref: "internal/runtime/resume_session_test.go#TestResumeSessionReconcilesAndSeeds"
        status: pass
      - kind: unit
        ref: "internal/runtime/resume_session_test.go#TestResumeSessionSeedsPlanMode"
        status: pass
    human_judgment: false
  - id: D2
    description: "The load core completes the v1 response contract: replay INCLUDING closure frames, then available_commands_update, then the response whose modes carries the seeded plan-mode state (null without a plan_mode line); prompt during parked replay typed-rejected, prompt after load accepted with the sequence continued; double registration typed-rejected; interrupted-then-completed load succeeds"
    requirement: ACP-06
    verification:
      - kind: unit
        ref: "internal/acp/replay_test.go#TestLoadFullOrdering"
        status: pass
      - kind: unit
        ref: "internal/acp/replay_test.go#TestLoadModesNullWithoutPlanMode"
        status: pass
      - kind: unit
        ref: "internal/acp/replay_test.go#TestLoadDoubleRegistration"
        status: pass
    human_judgment: false
  - id: D3
    description: "Zero provider/LLM invocations on the resume path (D-01 transcript-local) — the counting provider pins the whole load flow"
    requirement: ACP-06
    verification:
      - kind: unit
        ref: "internal/runtime/resume_session_test.go#TestResumeNoProviderCalls"
        status: pass
      - kind: other
        ref: "internal/acpserve/kill9_test.go#TestKill9Resume (stub call-count snapshot unchanged from kill to follow-up prompt in every row)"
        status: pass
    human_judgment: false
  - id: D4
    description: "The kill -9 matrix against REAL serve processes: classes 1-5 and 8-9 each SIGKILL a child at the class's suspension point and assert the four checks (ordering, on-disk+in-replay interrupted provenance, turn-id/mode continuation, no ghost state); ask-interplay (queued asks, no parked ask post-resume, dead-ask late response no-wedge) and kill-during-replay (typed D-03 rejection mid-replay, second resume idempotent) rows included"
    requirement: ACP-06
    verification:
      - kind: integration
        ref: "internal/acpserve/kill9_test.go#TestKill9Resume"
        status: pass
      - kind: other
        ref: "go test -race -count=1 -timeout 180s ./internal/acpserve/ -run TestKill9Resume (4 consecutive green runs; ~15s parallel)"
        status: pass
      - kind: other
        ref: "go test -race -count=1 ./internal/acpserve/ (full suite green with the matrix in it)"
        status: pass
    human_judgment: false
  - id: D5
    description: "The standing gate: mise ci (vet + golangci-lint v2 + CGO_ENABLED=0 build + full -race suite) green with the matrix in the suite"
    requirement: ACP-06
    verification:
      - kind: other
        ref: "mise ci (exit 0, 0 lint issues, zero FAIL lines; log at /tmp/mise-ci-1805b.log)"
        status: pass
    human_judgment: false

# Metrics
duration: 72min
completed: 2026-09-03T15:25:00Z
status: complete
---

# Phase 18 Plan 05: Reconciliation in the Live Load Path + kill -9 Matrix Summary

**One-liner:** Runner.ResumeSession became the full transcript-side resume (reconcile -> append provenance-marked closures -> seed turn counter + plan mode), the load core replays the post-closure transcript with commands re-advertised and the seeded v1 modes carried, and TestKill9Resume SIGKILLs real serve children across the ten-row matrix proving resume leaves no ghost state.

## What Was Built

### Task 1 — Reconciliation wired into the load path (tdd)

- **Runner.ResumeSession (internal/runtime/runtime.go)** now performs the complete D-01 ordering: `Manager.ReadAll` (loud degrade on error) -> `session.Reconcile` -> `Manager.AppendSynthetic` per closure (append failures loud + non-fatal — the next resume re-appends only what is missing) -> `SeedResume(seed.MaxTurns, planTarget)`. The acp load core calls it BEFORE `ReplayTranscript`, so replay streams the post-closure file and the closures render as ordinary terminal frames (the class-1 failed result as a terminal failed tool_call_update, D-02).
- **Session.SeedResume widened** to `(maxTurns int64, planMode string)`: the row-6 seed applies the plan-mode target (`plan_mode_enter` -> ON, `plan_mode_exit` -> OFF; empty leaves the fresh default). `PlanModeState` gained `used`-tracking; `Session.ModesState()` builds the v1 `SessionModeState` shape (`availableModes` [default, plan] + `currentModeId`), nil when the session never recorded a transition. The schema shape was verified this session against agentclientprotocol.com/protocol/v1/schema.md (SessionModeState = availableModes[]+currentModeId — not a modeId/state pair).
- **The load core (internal/acp/handlers.go)**: the registration guard became ONE critical section under `s.mu` — an already-registered live id answers the typed `already active` error (18-01's idempotent-success retired: with closures appended, a re-load would duplicate replayed frames onto a live session), a concurrent second load keeps the busy error; `modes` is read from the new `ModeStateProvider` optional capability AFTER ResumeSession; `NotifyAvailableCommands` fires the `available_commands_update` frame AFTER the last replayed frame and BEFORE the Barrier/response.
- **The commands seam**: `acp.CommandSource` + `WithCommandSource` + `acp.AvailableCommandFrame` + `acp.KindAvailableCommandsUpdate`; the acpserve adapter (command_source.go) projects the runner's discovered ecosys registry. With no source wired the frame still fires with the empty set — full-replacement semantics keep the client's autocomplete coherent.
- **Scanner unification**: `Reconcile`'s seed scan delegates to `seed.go`'s `MaxTurnCounter`; the 18-02-era local copy is deleted (exactly one suffix scanner; the 18-02 suite stayed green through the change).

### Task 2 — the kill -9 matrix harness

- **internal/acpserve/kill9_test.go** — `TestKill9Resume`: table-driven over classes 1-5, 8-9, ask-interplay, kill-during-replay. Each row spawns the test binary re-exec'd via `TestMain` under `ASS_GUARD_KILL9_CHILD` into the REAL `Run` composition (EngineEnabled:true — the 17-05 lesson), drives initialize/session/new/session/prompt over stdio pipes, parks the row's stub provider at the suspension point (hold/partial SSE entries signal the parent by channel; transcript polls gate on the last synchronous line), SIGKILLs the child (proven via `WaitStatus`), respawns over the same workdir, resumes via session/load, and asserts (a) replay-then-commands-then-response ordering with no parked ask firing, (b) interrupted provenance on disk AND in replay, (c) the follow-up turn id continues the sequence (and the plan-mode row resumes `modes.currentModeId: plan`), (d) the follow-up completes a full turn with the whole post-reconciliation transcript pairing cleanly. The stub's cross-child counter pins zero provider calls between kill and the deliberate follow-up.
- **The ask-interplay row** flips permissions.mode gated through the real set_config_option, drives ONE response carrying Write + Bash (two gated suspensions: one dialog open, one queued), kills while pending, and proves post-resume: both close cancelled-normal, no parked ask fires (bounded quiet drain), and a late client response to the DEAD ask's id resolves through `Registry.Deliver`'s unknown-id path without wedging (the follow-up prompt works).
- **The kill-during-replay row**: a 2000-chunk transcript; the RESUMING child is killed mid-replay after the parent observes the typed D-03 prompt rejection (the unread pipe parks the replay deterministically); the second resume completes with the closures exactly-once (idempotent) and the full 2001-chunk replay.
- Rows run parallel: ~15s standalone, ~96s inside the full -race acpserve suite; no env-flag gating; bounds are load-tolerant (30s/15s) so healthy phases never race the guards.

## Phase-17 shape record (Task 1 precondition)

Phase 17 ships `ask_suspended` transcript lines for BOTH ask families — the question family (`suspendForAsk` -> `AppendAskSuspended`, internal/session/session.go:711) and the permission family (`suspendForPermission` -> `AppendAskSuspended`, internal/session/gate.go:376). Class 10 therefore needs no separate machinery: class-3 treatment covers it (unresolved `ask_suspended` -> cancelled-normal tool_result), and in-memory ask registries are empty after process death by construction — no parked ask can fire post-resume (exactly the 18-RESEARCH Open-Question-2 resolution the precondition anticipated).

## Deviations from Plan

### Plan-vs-reality adjustments

**1. [Rule 3 - Blocker] The commands source the plan referenced does not exist yet**
- **Found during:** Task 1 GREEN
- **Issue:** The plan said to emit available_commands_update "from the same registry source the new-session path advertises from" — but session/new advertises no commands anywhere in the codebase (ACP-04/ACP-06's session-start advertisement is Phase 20 work, per ROADMAP 20-01).
- **Fix:** Added the `acp.CommandSource` optional seam + `WithCommandSource` + the acpserve adapter over `Runner.CommandRegistry()` (the ecosys discovery). Phase 20's session/new emission reuses the same seam; an unwired server still re-advertises the empty set (the frame always fires).
- **Files modified:** internal/acp/server.go, internal/acp/types.go, internal/acpserve/command_source.go, internal/acpserve/acp_serve.go, internal/runtime/runtime.go
- **Commit:** 79ec851

**2. [Plan-directed] 18-01's idempotent re-load retired**
- The plan's must_haves/actions explicitly pin the typed already-active error for double registration; this supersedes 18-01's "already-live READY answers the v1 response" decision (with reconciliation appending closures, a re-load is now observably destructive). The one 18-01 test that pinned the old frame sequence was extended with the commands frame. Not a defect — recorded because it reverses a prior plan's documented decision.
- **Commit:** 79ec851

**3. [Reading] Matrix check (a) in the harness**
- "No prompt is accepted before the load response is readable" cannot mean client-read time (a server-side-completed load legitimately accepts a prompt while the response sits in the pipe buffer). The harness asserts the deterministic core: during load, no new-turn frame and no ask frame appears before the response, `available_commands_update` is the last pre-response frame, and the follow-up prompt is accepted only after the response is consumed; the STRICT typed mid-replay rejection is asserted where the replay is provably parked (the kill-during-replay row on the wire; TestLoadFullOrdering + 18-01's TestLoadGateRejectsPromptDuringReplay in-process with a shrunken lane).

**4. [Reading] modes "null otherwise"**
- Implemented as: non-null exactly when the transcript carries a plan_mode line (PlanModePresent); currentModeId from the LAST line's cause (enter -> "plan", exit -> "default"). The behavior bullet's "(and null otherwise)" is read as the no-plan-mode case; a session whose last transition was an exit still reports the shape with default current (the acceptance bullet's "ends in a plan-mode line" reading). Both arms are test-pinned.

**5. [Placement] TestResumeNoProviderCalls lives in internal/runtime**
- The plan placed the four RED tests in replay_test.go with a package-boundary carve-out for the ResumeSession one; the provider-counting test equally demands the runtime side (acp cannot construct a provider-bearing runner — runtime imports acp). Both landed in internal/runtime/resume_session_test.go.

**6. [TDD note] TestResumeNoProviderCalls passed from RED**
- The resume path was already transcript-local; the test pins the invariant against this plan's changes rather than driving new behavior. The other three RED tests failed genuinely (closures absent, modes null, double-registration succeeding, commands frame absent).

### Out-of-scope discovery (recorded, not fixed)

**session TestGateOutcomeMatrix flake** — `allow_once` subtest fails ~1-in-10 under -race ("allow_once result = []; want exactly one non-error result"). Reproduced at the PRE-18-05 commit 628f7cd (worktree loop, 1/10 FAIL) — pre-existing, none of 18-05's changes on its path. Logged in `.planning/phases/16-acp-wire-foundation/deferred-items.md`. One mise ci run caught it; the re-run (and every targeted run) is green.

**Total deviations:** 6 documented (1 auto-fixed seam addition, 1 plan-directed contract change, 2 documented readings, 1 test-placement carve-out, 1 TDD pass-with-note) + 1 out-of-scope flake logged. **Impact:** none — all plan verify commands and the standing gate green.

## Verification Results

- `go test -race -count=1 ./internal/acp/ -run 'TestLoad|TestResume|TestSessionLoad'` — ok
- `go test -race -count=1 ./internal/runtime/ -run 'TestResumeSession'` — ok
- `go test -race -count=1 -timeout 180s ./internal/acpserve/ -run TestKill9Resume` — ok (4 consecutive runs: 15.0s / 14.9s / 14.3s / 19.1s; well under the 60s target)
- `go test -race -count=1 ./internal/acpserve/` — ok (96-113s, matrix included)
- `go test -race -count=1 ./internal/acp/ ./internal/runtime/ ./internal/session/` — ok
- `mise ci` — exit 0, lint 0 issues, zero FAIL lines (one earlier run caught the pre-existing session flake documented above; log at /tmp/mise-ci-1805b.log)

## TDD Gate Compliance

- RED commit `test(18-05)` (5baf0b7) precedes GREEN `feat(18-05)` (79ec851): three of the four Task-1 behaviors failed genuinely at RED (closure append, modes, registration guard, commands frame); the fourth is the documented invariant pin. Task 2 is test-only (test(18-05), c7dce90) per the plan.

## Self-Check: PASSED

- internal/acpserve/kill9_test.go, internal/acpserve/command_source.go, internal/runtime/resume_session_test.go exist on disk.
- Commits 5baf0b7 / 79ec851 / c7dce90 present on gsd/v1.2-claude-code-parity.
- Every task acceptance criterion re-verified green before the docs commit.

## Next

Ready for 18-06 (the CLI --resume/--continue trio funnels into the completed load engine — one engine, two entrypoints).
