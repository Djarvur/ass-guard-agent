---
phase: 17-permissions-elicitation
plan: 05
subsystem: permissions
tags: [acp, permissions-gate, documentation, e2e, zed-simulator, operator-checkpoint, criterion-5, cc-parity]

# Dependency graph
requires:
  - phase: 17-permissions-elicitation (17-01..17-04)
    provides: the perm grammar + store, the gateCall chokepoint, the ask queue, and the elicitation surfaces this plan documents and proves
  - phase: 16-acp-wire-foundation (16-06)
    provides: the deterministic Zed-client simulator fixtures (simClient/simStub over pipes) the E2E battery consumes
provides:
  - "docs/permissions-gate.md — the ONE-chokepoint contract document: the hook-verdict → permission-ask → execute precedence at internal/session gateCall, the Phase-21 deny-only hook-join contract, both-modes semantics, the permissions.yaml grammar, the deliberate CC divergences, and the degradation family"
  - "internal/acpserve/permissions_e2e_test.go — TestPermissionsE2E: the five-scenario simulator battery (permission happy path with on-disk persistence, elicitation capable, elicitation degraded with byte-parity fallback, turn-death cascade, ungated default), non-vacuity-proven by a scratch mutation check"
  - "The operator checkpoint disposition: the seven-step live-Zed checklist recorded as pending in the WINDOWS ledger (#15), criteria 1+3's manual-only legs"
affects: [21-hooks, phase-17-verifier, ACP-01, ACP-02]

# Actuals (#2632) — chars/4 over the realized diff (~45k chars), same scale as the plan's estimate
actuals:
  tokens: 11300
  tasks: 3
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Pass-at-write with harness-only fixes: a proof battery over shipped surfaces fails first on harness bugs (id quoting, wrapper decoding, engine option), never on the surfaces — production untouched, proven by git diff + a scratch mutation check (unwire the fire seams → the wire scenarios fail)"
    - "EngineEnabled: true is the E2E requirement that makes a gated call REALLY execute — without it every non-mcp call lands the canned engine-disabled stub result (a trap for any future whole-Run battery asserting on-disk effects)"
    - "Best-effort temp-project cleanup: engine-enabled runs flush files just after serve exit, and t.TempDir's strict removal fails the TEST on macOS (unlinkat race) — the dir is not an assertion"

key-files:
  created:
    - docs/permissions-gate.md
    - internal/acpserve/permissions_e2e_test.go
  modified: []

key-decisions:
  - "The doc names gateCall because gate.go does: the chokepoint document pins the FIVE-step shipped verdict chain (hook head → rules in both modes → automation decline → mode decision → degraded guard) and states the Phase-21 join contract verbatim (deny-only authority from project scope; repo-shipped files never grant allow; a deny stops the call before rule evaluation, D-04)"
  - "The no-second-gate property is enforced as a region check, not a review claim: perm.RuleSet.Evaluate call sites across ALL internal/session non-test sources appear in gate.go only (the plan's widened package-wide grep), re-runnable in one command"
  - "Scenarios each boot their OWN serve + temp project: permissions.mode persists to the project layer, so a shared instance would leak gated state into the ungated-default scenario — isolation keeps the five stories independent"
  - "The degraded-fallback byte-parity assertion compares the chunk against coreexec.RenderAskSurface computed in-test — the fallback cannot drift from the v1.1 path without failing the battery"
  - "The operator checkpoint is recorded PENDING per the plan's own design (the auto-advance run cannot answer a blocking-human gate): the WINDOWS ledger carries the seven-step checklist, exactly the 15-07/16-06 precedent"

patterns-established:
  - "Pattern: criterion documents live in docs/ with a drift guard — the doc names the code's function, the code's property is greppable, and the E2E battery binds the wire story"

requirements-completed: [ACP-01, ACP-02]

# Coverage metadata (#1602) — one entry per shipped deliverable
coverage:
  - id: D1
    description: "Criterion-5 chokepoint documentation: docs/permissions-gate.md names gateCall identically to gate.go, documents the hook-verdict → permission-ask → execute precedence (D-04) with the Phase-21 deny-only join contract, both-modes semantics (D-05/D-06/D-07), the permissions.yaml grammar + dialog write surface (D-01/D-03), the deliberate CC divergences, and the degradation family"
    requirement: ACP-01
    verification:
      - kind: other
        ref: "test -f docs/permissions-gate.md && grep -c gateCall docs/permissions-gate.md (7 >= 1)"
        status: pass
      - kind: other
        ref: "grep -c 'hook verdict' docs/permissions-gate.md (3) — the precedence chain appears in order with the Phase-21 contract"
        status: pass
    human_judgment: false
  - id: D2
    description: "No second gate path: zero rule-evaluation call sites across ALL internal/session non-test sources outside gate.go (the widened package-wide grep), against compiling sources"
    requirement: ACP-01
    verification:
      - kind: other
        ref: "! grep -rn 'Evaluate(' --include='*.go' --exclude='*_test.go' --exclude='gate.go' internal/session/ — clean; go build ./internal/session/ — clean"
        status: pass
    human_judgment: false
  - id: D3
    description: "TestPermissionsE2E — the five-scenario simulator battery green under -race (3 consecutive full runs): happy path asserts the on-disk allow entry + the no-second-dialog frame count; degraded asserts the RenderAskSurface byte-parity chunk; turn-death asserts exactly one $/cancel_request, the cancelled-normal transcript result, no execution, and a quiet drain window; ungated asserts zero permission frames and zero trust writes"
    requirement: ACP-01
    verification:
      - kind: e2e
        ref: "tests/internal/acpserve/permissions_e2e_test.go#TestPermissionsE2E"
        status: pass
      - kind: other
        ref: "mutation check: unwiring SetPermissionAskFire/SetAskFire (scratch, restored) fails happy-path + capable — the assertions bind to real wire frames"
        status: pass
    human_judgment: false
  - id: D4
    description: "Live-Zed native permission-dialog rendering (criterion 1's operator leg): the four agent-supplied options render as Zed's dialog buttons, always-allow persists, cancel closes cleanly (the seven-step checklist)"
    requirement: ACP-01
    verification: []
    human_judgment: true
    rationale: "Requires a human in a real editor session (RESEARCH's validation map: manual-only leg); the deterministic simulator battery bounds but cannot replace it — recorded pending in the WINDOWS ledger (#15) per the 15-07/16-06 precedent"
  - id: D5
    description: "Live-Zed native elicitation-form rendering (criterion 3's operator leg): a learning/engine/model ask renders as an answerable native form"
    requirement: ACP-02
    verification: []
    human_judgment: true
    rationale: "Requires a human in a real editor session (manual-only leg); probe/degrade plus the wire-level battery cover older clients deterministically — recorded pending in the WINDOWS ledger (#15)"

# Metrics
duration: 29min
completed: 2026-09-01
status: complete
---

# Phase 17 Plan 05: Chokepoint Documentation + Simulator E2E + Operator Checkpoint Summary

**The ONE gate pipeline locked in docs/permissions-gate.md at the gateCall chokepoint with a greppable no-second-gate proof, the whole ask surface proven end-to-end by a five-scenario Zed-simulator battery, and the two live-rendering criteria handed to the operator via the WINDOWS ledger**

## Performance

- **Duration:** 29 min
- **Started:** 2026-09-01T03:11Z
- **Completed:** 2026-09-01T03:40Z
- **Tasks:** 3 (2 executed, 1 blocking-human checkpoint dispositioned pending)
- **Files modified:** 2 created (+ the WINDOWS ledger entry)

## Accomplishments
- Criterion 5 closed: docs/permissions-gate.md documents the ONE pipeline where Phase 21's planner will find it — the five-step gateCall verdict chain, the deny-only hook-join contract, both-modes semantics, the full permissions.yaml grammar with 17-01-table examples, the deliberate CC divergences, and the degradation family — and the codebase provably has no second rule-evaluation path (package-wide grep over compiling sources)
- The phase's wire-level story is proven end-to-end on the deterministic client: dialog → allow_always → on-disk rule → execution → no second dialog; form → accept → captured answered form; probe -32601 → byte-parity plain-text fallback; cancel mid-dialog → one cascade + cancelled-normal + no orphans; ungated default → zero dialogs
- The battery is non-vacuous by demonstration: a scratch mutation unwiring the ask-fire seams fails the wire scenarios (restored; production diff clean)
- `mise ci` green end-to-end carrying the new battery (vet + golangci-lint v2 0 issues + CGO_ENABLED=0 build + go test -race ./... — zero FAIL lines)

## Task Commits

1. **Task 1: Criterion-5 chokepoint documentation + no-second-gate check** - `10dc0dc` (feat)
2. **Task 2: Simulator E2E battery — five scenarios on the 16-06 fake client** - `0fd827c` (test; see TDD Gate Compliance)
3. **Task 3: Live-Zed operator checkpoint** — dispositioned pending (no code; the record rides this SUMMARY + WINDOWS #15)

**Plan metadata:** (docs commit follows this SUMMARY)

## Files Created/Modified
- `docs/permissions-gate.md` - the ONE-chokepoint contract: precedence chain, Phase-21 join contract, mode semantics, permissions.yaml grammar, CC divergences, degradation
- `internal/acpserve/permissions_e2e_test.go` - TestPermissionsE2E: the five-scenario battery consuming the 16-06 simulator fixtures against the real Run composition
- `.planning/WINDOWS.md` - entry #15: the pending operator checkpoint (criteria 1+3, seven-step checklist)

## Decisions Made
- **Scenario isolation:** each of the five scenarios boots its own serve + temp project — permissions.mode persists to the project layer, so a shared instance would leak gated state into the ungated-default story
- **EngineEnabled for real execution:** the happy path asserts the gated Write landed on disk, which requires the engine-backed executor — the default Options zero value leaves the canned stub executor in place (a trap documented in the battery's comments)
- **Byte-parity by computation:** the degraded-fallback chunk is compared against coreexec.RenderAskSurface computed in-test from the same scripted questions, so the v1.1 fallback cannot drift silently
- **Sequential stages (16-06 shape):** the battery runs its scenarios sequentially over per-scenario serves — parallelism would multiply serve load for no coverage gain (the flake-family discipline)

## Deviations from Plan

None - plan executed exactly as written (no production code changed; the two task commits are doc + test only).

## Issues Encountered
- The battery's first executions failed on harness-side bugs, each fixed within Task 2 before its commit: an unquoted JSON-RPC id in the prompt frame (the serve's parse error named it), the transcript tool_result output compared as raw JSON instead of its inner text, the missing EngineEnabled option (every non-mcp call landed the canned engine-disabled stub — the gated call never really executed), and a macOS TempDir cleanup race with late async flushes (made best-effort — the leftover dir is not an assertion). All three post-fix full runs were green; the surfaces under test (17-02/03/04) needed zero changes.

## TDD Gate Compliance
- Task 2 is tdd="true". The RED gate ran genuinely: the first battery execution failed (5/5 scenarios). Investigation per the unexpected-green/fail-first rules showed every failure was harness-side (above) — the surfaces under proof landed in 17-01..17-04, so a failing RED meant harness bugs, not missing code. No separate GREEN production commit exists because production needed no change (git diff on production files clean); the battery landed as one test(17-05) commit. Non-vacuity is proven by the scratch mutation check (unwiring the fire seams fails the wire scenarios). Pass-with-note per the 16-06 simulator precedent.

## Operator Checkpoint (Task 3 — blocking-human)

PENDING-OPERATOR-CONFIRMATION

The seven-step live-Zed checklist (from the plan's checkpoint task; WINDOWS ledger #15 carries the same record — record PASS/FAIL per step plus the Zed version there, then flip this marker to OPERATOR-CONFIRMED):

1. In a scratch project, set `permissions.mode: gated` via Zed's agent settings (or configOptions) and start a fresh ass-guard session.
2. Prompt a mutating tool call (e.g. ask the agent to write a file). EXPECT: Zed's native permission dialog with FOUR options — Allow once / Always allow / Reject once / Always reject (A1 caveat: if only two appear, the stable channel lags main — record the Zed version in the ledger).
3. Click Always allow. EXPECT: the tool runs, `.ass-guard/permissions.yaml` gains the allow entry (0600), and a second identical call runs with NO dialog.
4. Start an ask (a question-shaped prompt or a learning-store ask). EXPECT: a native form, answerable; on decline the agent proceeds with the non-answer.
5. Cancel a turn while a dialog is open. EXPECT: the dialog closes, no hang, the session stays responsive.
6. Flip `permissions.mode` back to ungated mid-session. EXPECT: no further dialogs on the running session.
7. Record PASS/FAIL per step in the WINDOWS ledger. A FAIL on step 2's option count is a ledger note (A1 caveat), not a phase blocker (probe/degrade covers it).

Deterministic bounds already proven: the same five stories run green over pipes in TestPermissionsE2E (stage for stage: 1↔steps 2-3, 2↔step 4, 4↔step 5, 5↔step 6).

## User Setup Required

No new external service configuration. The live checkpoint above needs only the operator's existing Zed install (any version; record it in the ledger — stable-channel lag on step 2 is a note, not a blocker).

## Next Phase Readiness
- Phase 17 is CLOSED pending the operator leg: criterion 5's document + grep proof and the deterministic half of criteria 1/3 are landed; ACP-01 and ACP-02 are now markable complete (17-05 is the last declaring sibling for both)
- Phase 21's hooks join at the documented seam: docs/permissions-gate.md §1.3 is the contract (deny-only authority, no second gate); ROADMAP criterion 2 audits against this doc + the internal/session grep
- Verifier note: flip WINDOWS #15 to resolved (and this SUMMARY's marker) when the operator runs the seven steps; a two-option-only dialog on stable Zed is an A1 ledger note, not a divergence
- Watchlist: the battery's per-scenario serves are load-light, but the engine-enabled Run makes each stage ~2s — if the suite grows, consider a shared-serve variant for elicitation-only stages

---
*Phase: 17-permissions-elicitation*
*Completed: 2026-09-01*

## Self-Check: PASSED

- Created files verified on disk: docs/permissions-gate.md, internal/acpserve/permissions_e2e_test.go
- Task commits verified in git log: 10dc0dc (docs), 0fd827c (battery)
- Task 1 acceptance criteria re-run green: gateCall grep count 7; no-second-gate grep clean; go build ./internal/session/ clean
- Task 2 acceptance criteria re-run green: TestPermissionsE2E 5/5 stages under -race, 3 consecutive full runs; on-disk rule + no-second-dialog frame count asserted (happy path), byte-parity fixture asserted (degraded), cascade + cancelled-normal asserted (turn death); mise ci green end-to-end (zero FAIL lines, log at /tmp/17-05-mise-ci.log)
- Task 3 disposition recorded: the pending marker is the first token of its own line above (count 1); WINDOWS ledger entry #15 exists with the seven-step checklist
