---
phase: 17-permissions-elicitation
plan: 06
subsystem: permissions
tags: [acp, permissions-gate, wire-shape, gap-closure, G-17-1, zed-1-18-0, cc-parity, tdd]

# Dependency graph
requires:
  - phase: 17-permissions-elicitation (17-01..17-05)
    provides: the permission-ask surface, gate pipeline, ask queue, elicitation surfaces, and the simulator E2E battery this plan repairs to the canonical wire shape
  - phase: 17-permissions-elicitation (17-UAT)
    provides: gap G-17-1's root-cause diagnosis (flat-string decode vs the canonical nested outcome object; live Zed 1.18.0 transcript evidence)
provides:
  - "internal/acp/types.go — PermissionOutcomeFrame decoded from the CANONICAL ACP v1 nested outcome object (new inner PermissionOutcome struct: outcome discriminator + optionId, v1 spellings verbatim)"
  - "internal/acpserve/ask_surface.go — Fire's outcome switch reads the INNER discriminator; every fail-safe sentinel preserved verbatim (errPermissionUnsupported/-Bad/-Unknown, -32601 sticky degrade, D-14 terminal fallback, log wording)"
  - "Fail-safe pins: flat one-level outcome → errPermissionOutcomeBad (never an allow); unknown inner discriminator → errPermissionOutcomeUnknown — the forged/nonconformant-answer tampering class can now only produce a decline or cancelled"
  - "The E2E simulator answers the canonical nested shape, so TestPermissionsE2E models the real client (Zed 1.18.0) — this decode-regression class can never again hide behind a green battery"
affects: [phase-17-verifier, verify-work, ACP-01, phase-21-hooks]

# Actuals (#2632) — chars/4 over the realized diff (~8.7k chars), same scale as the plan's estimate
actuals:
  tokens: 2200
  tasks: 2
  commits: 2

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Wire truth re-verified at the canonical spec page DURING execution (Pitfall-7): the tool-calls example page pins selected = {\"outcome\":{\"outcome\":\"selected\",\"optionId\":\"…\"}} / cancelled = {\"outcome\":{\"outcome\":\"cancelled\"}} while CreateElicitationResponse stays FLAT (action + content) — two union encodings in one protocol; each surface's shape is pinned to its own schema def, never inferred from its sibling"
    - "Nonconformance fails safe by construction: the wrong nesting (flat string) fails json.Unmarshal → errPermissionOutcomeBad; an unknown inner discriminator string decodes but hits the switch default → errPermissionOutcomeUnknown; an empty optionId on selected falls through to the session gate's unknown-option decline — three distinct malformed classes, three distinct sentinels, one outcome: decline, never allow"

key-files:
  created: []
  modified:
    - internal/acp/types.go
    - internal/acpserve/ask_surface.go
    - internal/acpserve/ask_surface_test.go
    - internal/acpserve/permissions_e2e_test.go

key-decisions:
  - "The frame keeps exactly ONE wire field (outcome) typed as the new inner PermissionOutcome struct — the flat shape the old type declared is not merely remapped, it now fails the decode, so the pre-17-06 wrong shape is a hard nonconformance instead of a silently-accepted dialect"
  - "A selected outcome with an empty optionId gets NO new guard in Fire: it falls through to the session gate's default unknown-option fail-safe branch (resolvePermissionOutcome in internal/session/ask.go), which is the correct decline — the guard already exists at the trust boundary that owns option semantics"
  - "permAnswerAccept / ElicitationOutcomeFrame deliberately untouched: the elicitation response IS a flat top-level action discriminator with optional content (schema CreateElicitationResponse, re-verified during 17-06) — a comment on permAnswerAccept records the verification so nobody 'fixes' it to nested"
  - "Discriminator constants PermissionOutcomeSelected/Cancelled unchanged — the fix reshapes the nesting, not the vocabulary"

patterns-established:
  - "Pattern: when one protocol carries two union encodings (nested object vs flat discriminator), pin each decode site to its own canonical example page and add a nonconformance pin per WRONG nesting — the flat-rejection subtest is the regression proof that the battery once codified the bug it now rejects"

requirements-completed: [ACP-01]

# Coverage metadata (#1602) — one entry per shipped deliverable
coverage:
  - id: D1
    description: "Canonical nested outcome decode in production: a selected nested answer (inner discriminator selected + optionId allow_always) maps to AskOutcome.Selected so the gated call executes and persists; a cancelled nested answer maps to Cancelled (cancelled-NORMAL, never an error); the flat one-level shape fails safe via errPermissionOutcomeBad; an unknown inner discriminator fails safe via errPermissionOutcomeUnknown"
    requirement: ACP-01
    verification:
      - kind: unit
        ref: "internal/acpserve/ask_surface_test.go#TestPermissionAskDispatch (7/7 subtests green: selected, cancelled_outcome, flat_outcome_nonconformance, unknown_inner_discriminator, client_cancelled_-32800, method_not_found_-32601, timeout_fallback)"
        status: pass
      - kind: other
        ref: "RED proof: the same test failed 4/4 new/changed subtests against the flat parse pre-fix ('malformed permission outcome: json: cannot unmarshal object into Go struct field PermissionOutcomeFrame.outcome of type string' — the live-Zed failure string)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Simulator fidelity: the E2E simulator answers the canonical nested shape, so TestPermissionsE2E exercises the same decode live Zed 1.18.0 exercises — the full five-scenario battery (dialog round-trip with on-disk allow_always persistence, elicitation capable, degraded fallback, turn-death cascade, ungated default) green under -race with the canonical-answer client"
    requirement: ACP-01
    verification:
      - kind: e2e
        ref: "internal/acpserve/permissions_e2e_test.go#TestPermissionsE2E (PASS, 5/5 scenarios, -race, canonical nested permAnswerSelected)"
        status: pass
      - kind: other
        ref: "go test -race ./internal/acp/ ./internal/acpserve/ ./internal/session/ -count=1 — ok/ok/ok (all pre-existing pins hold)"
        status: pass
      - kind: other
        ref: "mise ci green: vet + golangci-lint 0 issues + CGO_ENABLED=0 build + go test -race ./... (zero FAIL/ERROR lines; log /tmp/17-06-mise-ci.log)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Live-client contract confirmation: UAT Test 1 re-runnable against live Zed (dialog answers execute/persist instead of fail-safe-declining) and UAT Test 2 (native elicitation form) un-blocked"
    requirement: ACP-01
    verification: []
    human_judgment: true
    rationale: "Requires the operator's real Zed 1.18.0 session (the plan's verification item 6 assigns it to the verify-work re-run; the deterministic battery bounds the decode but cannot replace the live client). WINDOWS ledger #15 already carries the live-Zed checklist and 17-UAT.md carries gap G-17-1 for re-test."

# Metrics
duration: 12min
completed: 2026-09-02
status: complete
---

# Phase 17 Plan 06: Canonical Nested request_permission Outcome (G-17-1) Summary

**PermissionOutcomeFrame reshaped to the canonical ACP v1 nested outcome object (inner discriminator + optionId) with RED-proven pins and a canonical-answer simulator, so live Zed 1.18.0 dialog answers decode, execute, and persist instead of fail-safe-declining on every click**

## Performance

- **Duration:** 12 min
- **Started:** 2026-09-02T21:20:59Z
- **Completed:** 2026-09-02T21:33:00Z
- **Tasks:** 2 (both tdd — RED then GREEN)
- **Files modified:** 4

## Accomplishments
- Gap G-17-1 (blocker) closed at the wire: `internal/acp/types.go` declares the response as the schema's nested union object (new inner `PermissionOutcome` struct; frame's single `outcome` field typed as it), and `internal/acpserve/ask_surface.go`'s Fire switch reads the inner discriminator + optionId — the exact shape the canonical example page and the live Zed 1.18.0 transcript pin
- Fail-safe surface hardened and pinned: the old flat one-level shape now FAILS the decode (`errPermissionOutcomeBad` — decline, never an allow), an unknown inner discriminator lands on `errPermissionOutcomeUnknown`, and an empty optionId falls to the session gate's unknown-option decline — every malformed/forged answer class can only produce decline or cancelled (T-17-06-01 mitigated)
- The E2E simulator (`permAnswerSelected`) answers canonical nested, so TestPermissionsE2E's five scenarios now exercise the same decode the real client exercises — a green battery can no longer codify this bug class
- Full battery green: dispatch 7/7 subtests, TestPermissionsE2E 5/5 scenarios under `-race`, the three-package sweep, and `mise ci` end-to-end (vet + golangci-lint 0 issues + CGO_ENABLED=0 build + `go test -race ./...`)

## Task Commits

1. **Task 1: RED — pin the canonical nested outcome in both test clients** - `1538673` (test)
2. **Task 2: GREEN — decode the canonical nested outcome in production** - `f372db4` (fix; includes lint-only adjustments to the test file made during the GREEN run)

**Plan metadata:** (docs commit follows this SUMMARY)

## Files Created/Modified
- `internal/acp/types.go` - PermissionOutcomeFrame reshaped: new inner PermissionOutcome struct (outcome + optionId, v1 spellings verbatim, tagliatelle nolint per file convention); discriminator constants unchanged; doc comment cites the schema def, the canonical example page, and Zed 1.18.0
- `internal/acpserve/ask_surface.go` - Fire's outcome switch subject changed to the inner discriminator (`of.Outcome.Outcome`), Selected mapped from the inner optionId; json.Unmarshal + errPermissionOutcomeBad wrap, ErrRequestCancelled branch, -32601 sticky degrade, D-14 terminal fallback, and all log wording preserved verbatim
- `internal/acpserve/ask_surface_test.go` - TestPermissionAskDispatch selected/cancelled payloads corrected to canonical nested; NEW flat-outcome-nonconformance and unknown-inner-discriminator subtests (errors.Is against the sentinel family); -32800/-32601/timeout subtests untouched; lint vocabulary aligned with the sibling-battery convention
- `internal/acpserve/permissions_e2e_test.go` - permAnswerSelected sends the canonical nested object (doc comment cites the example page + Zed 1.18.0); permAnswerAccept payload untouched with a comment recording the canonical-flat verification

## Decisions Made
- **One wire field, hard nonconformance:** the frame keeps exactly one `outcome` field typed as the nested struct — the flat shape fails the decode rather than being remapped, making the pre-17-06 dialect a rejected nonconformance instead of an accepted one
- **No new empty-optionId guard:** a selected outcome with an empty optionId falls through to the session gate's default unknown-option fail-safe branch (`resolvePermissionOutcome`) — the guard already lives at the boundary that owns option semantics
- **Elicitation side deliberately untouched:** CreateElicitationResponse is a flat action + optional content (re-verified against the v1 schema page during execution); permAnswerAccept carries a comment citing the 17-06 verification so the two union encodings are never "unified" by mistake
- **Wire truth re-verified during execution (Pitfall-7):** the tool-calls example page and the schema page were fetched and read before any edit — selected/cancelled examples match the plan's shapes byte-for-byte

## Deviations from Plan

None - plan executed exactly as written (both tasks, both commits, all sentinels preserved; the elicitation side untouched as specified).

## Issues Encountered
- Two lint findings surfaced by `mise ci` on the grown TestPermissionAskDispatch (gocognit 31 > 30 from the two new subtests; one 125-char payload line): fixed lint-only (nolint vocabulary per the sibling golden-battery convention, payload line wrapped) and folded into the Task 2 commit — no behavior change
- Environment note: the bare `golangci-lint` binary on PATH panics ("file requires newer Go version go1.27, application built with go1.26"); mise's pinned v2.12.2 tool is the working one — `mise ci` (the actual gate) runs it green. No repo change needed

## TDD Gate Compliance
- Both tasks are tdd="true". RED gate ran genuinely: Task 1's dispatch test failed 4/4 new/changed subtests against the flat parse (output below) before any production change. GREEN: Task 2's production commit turned them green
- Commit sequence per the plan's own instructions: `test(17-06):` (1538673) precedes `fix(17-06):` (f372db4) — the plan specifies `fix` (not `feat`) as the GREEN commit type for this gap-closure bug fix

### RED proof (Task 1 verify, verbatim)

```
--- FAIL: TestPermissionAskDispatch (0.00s)
    --- FAIL: TestPermissionAskDispatch/selected (0.00s)
        ask_surface_test.go:214: outcome = {Selected: Cancelled:false Err:malformed permission outcome: json: cannot unmarshal object into Go struct field PermissionOutcomeFrame.outcome of type string Unsupported:false Elicit: Content:map[] Violation: Fallback:false}; want selected allow_always
    --- FAIL: TestPermissionAskDispatch/unknown_inner_discriminator (0.00s)
        ask_surface_test.go:260: outcome Err = malformed permission outcome: json: cannot unmarshal object into Go struct field PermissionOutcomeFrame.outcome of type string; want the errPermissionOutcomeUnknown family
    --- FAIL: TestPermissionAskDispatch/cancelled_outcome (0.00s)
        ask_surface_test.go:226: outcome = {Selected: Cancelled:false Err:malformed permission outcome: json: cannot unmarshal object into Go struct field PermissionOutcomeFrame.outcome of type string Unsupported:false Elicit: Content:map[] Violation: Fallback:false}; want cancelled
    --- FAIL: TestPermissionAskDispatch/flat_outcome_nonconformance (0.00s)
        ask_surface_test.go:240: outcome = {Selected:allow_always Cancelled:false Err:<nil> Unsupported:false Elicit: Content:map[] Violation: Fallback:false}; want a fail-safe Err (never an allow)
        ask_surface_test.go:244: outcome Err = <nil>; want the errPermissionOutcomeBad family
FAIL
FAIL	github.com/Djarvur/ass-guard-agent/internal/acpserve	0.692s
FAIL
exit=1
```

The selected/cancelled failures reproduce the live-Zed error string verbatim (G-17-1's reported failure); the flat-nonconformance failure shows the old type ACCEPTING the wrong shape as an allow — exactly the never-an-allow direction the new pin forbids.

## Verification Results

1. RED proof — recorded above (4/4 subtests failing pre-fix; exit=1)
2. `go test -race ./internal/acpserve/ -run 'TestPermissionAskDispatch|TestPermissionsE2E' -count=1` — ok (dispatch 7/7 subtests; TestPermissionsE2E PASS 11.65s)
3. `go test -race ./internal/acp/ ./internal/acpserve/ ./internal/session/ -count=1` — ok / ok (51.0s) / ok (3.5s)
4. `mise ci` — green (vet + golangci-lint 0 issues + CGO_ENABLED=0 build + `go test -race ./...`; zero FAIL/ERROR lines; log /tmp/17-06-mise-ci.log)
5. Positive shape gates: `grep -c '"outcome":{"outcome"'` = 3 on ask_surface_test.go (≥3) and 2 on permissions_e2e_test.go (≥1); `grep -c 'PermissionOutcome\b' internal/acp/types.go` = 6 (≥2); sentinels errPermissionOutcomeBad (ask_surface.go:100,152,161) and errPermissionOutcomeUnknown (:101,175) referenced by live code paths and exercised by the new subtests; permAnswerAccept payload byte-identical to pre-plan
6. Live-client confirmation — assigned to the verify-work re-run (coverage D3, human_judgment), per the plan's own scope

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Gap G-17-1 is closable at verify-work: UAT Test 1 (live-Zed four-option dialog + always-persistence) is re-runnable and expected to pass end-to-end; UAT Test 2 (native elicitation form) is un-blocked — the AskUserQuestion gate dialog answer now decodes, so elicitation/create can actually be sent
- The deterministic half is fully proven: the same wire story runs green over pipes in TestPermissionsE2E with the client answering the canonical shape
- No new WINDOWS ledger entry: the live-Zed checklist already lives in WINDOWS #15 and the re-test in 17-UAT.md — coverage D3 routes the human leg deterministically at verify-work

---
*Phase: 17-permissions-elicitation*
*Completed: 2026-09-02*

## Self-Check: PASSED

- Modified files verified on disk: internal/acp/types.go, internal/acpserve/ask_surface.go, internal/acpserve/ask_surface_test.go, internal/acpserve/permissions_e2e_test.go
- Task commits verified in git log: 1538673 (test, RED), f372db4 (fix, GREEN)
- All plan verification commands re-run green (items 1-5 above); no failing test reported as passing
- Post-commit deletion check: zero deletions across both task commits; no untracked files in touched paths
