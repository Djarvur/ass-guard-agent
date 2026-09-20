---
phase: 17-permissions-elicitation
plan: 04
subsystem: permissions
tags: [acp, elicitation, d08-mapping, d10-revalidation, structured-reply, ask-family, capability-gating, fallback-parity]

# Dependency graph
requires:
  - phase: 17-permissions-elicitation (17-02/17-03)
    provides: the ask queue (one-outstanding firing path, priority classes, D-12 notes, D-13 drain), the gate chokepoint's enqueue discipline, and the PermissionAsk surface family (Registry-backed HUMAN-ASK fire)
  - phase: 16-acp-wire-foundation (16-03)
    provides: the outbound Registry (Call/Deliver/ResolveCancelled, HUMAN-ASK class) and the sticky elicitation-form capability cache (D-13/D-18 advertisement-or-probe)
provides:
  - "internal/acp/types.go — ElicitationFormFrame/ElicitationSchema (per-property raw variants)/ElicitationOutcomeFrame v1-verbatim + MethodElicitationCreate exported + the form/accept/decline/cancel vocabulary + CapElicitationForm/CapBooleanConfigOption sticky capability keys (boolean advertisement parsed at initialize)"
  - "internal/acpserve/ask_surface.go — BuildElicitationForm (THE D-08 mapping: oneOf titled consts / array enum-or-anyOf / free-text / boolean both capability branches, q1..qN keys all required, enumNames never), ElicitationAsk (capability-gated elicitation/create under HUMAN-ASK; degraded/-32601-sticky/malformed → today's plain-text path verbatim), ValidateElicitationContent (the closed D-10 subset: required/type/oneOf/enum by exact equality, item-wise arrays, rune-counted bounds, no pattern engine, no Unicode rewriting)"
  - "internal/session/ask.go — RenderStructuredReply (single string field → RenderAskAnswered byte-identically; multi-field → property-order title=value lines, arrays comma-joined, booleans true/false), ResolveAskStructured (the widened seam), resumeAskForm (the shared resume tail), EnqueueElicitationAsk + the D-10 bounded loop, EnqueueEngineAsk (background class, learning-store landing)"
  - "internal/runtime — the AskBroker onSurface conversion (question asks enqueue; plain-text publish demoted to the non-primary degrade family), SetAskFire/PublishAskChunk, the engine ask-pending → background elicitation entry conversion (accept persists via RecordCandidate; decline lands the 12-D-05 advisory note)"
affects: [17-05-permission-docs, 21-hooks, ACP-02]

# Actuals (#2632) — chars/4 over the realized diff (118,320 chars), same scale as the plan's estimate
actuals:
  tokens: 29500
  tasks: 3
  commits: 5

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Byte-identity anchoring: the structured-reply seam's single string-valued field renders THROUGH the legacy renderer (RenderAskAnswered), so the capture-pinned answered form is preserved by construction — the golden suite cannot drift"
    - "Conservative capability gating: elicitation/create fires only when the sticky elicitation-form capability is ok; every degraded/malformed case lands on today's plain-text path verbatim — and only -32601 is sticky (16-D-18 family), transient failures re-probe next ask"
    - "Closed-subset validation: the D-10 validator models only the fields it enforces (type/oneOf/enum/items/min-max), matches by exact string equality, counts runes — the bounded-matcher MUST is satisfied by the pattern engine's absence"
    - "Family-uniform landing: question asks resume through the captured form; engine asks persist through the learning store's existing RecordCandidate; both ride the SAME D-10 verdict function — one loop, two landings"

key-files:
  created:
    - internal/session/elicitation_reply_test.go
  modified:
    - internal/acp/types.go
    - internal/acp/handlers.go
    - internal/acp/server.go
    - internal/acp/server_test.go
    - internal/acpserve/ask_surface.go
    - internal/acpserve/ask_surface_test.go
    - internal/acpserve/acp_serve.go
    - internal/session/ask.go
    - internal/session/askqueue.go
    - internal/session/gate.go
    - internal/runtime/runtime.go
    - internal/runtime/ask_wiring_test.go

key-decisions:
  - "Byte-identity anchor: a single property carrying a STRING value renders through RenderAskAnswered verbatim — the zcode-interactive-results.json answered form is byte-preserved and the coreexec golden suite stays green untouched; multi-field renders property-order title=value lines (arrays comma-joined, booleans true/false) inside the captured wrapper"
  - "Boolean-ask convention: the captured AskQuestion schema carries no boolean flag, so a yes/no two-option single-choice question IS the boolean ask; the boolean property renders only when the client advertised the boolean config-option capability (the conservative D-08 gate, now wired to the real session.configOptions.boolean advertisement)"
  - "Engine asks are not reply-answerable: their degraded surface is the caller's advisory note (PlainTextFallback=false — no dead-end question text is published), while question-family fallback keeps the broker's reply routing + D-01 timer alive exactly as v1.1"
  - "Elicitation surface failures degrade, never dead-end: transport failures/-32601/malformed outcomes all fall back to the plain-text publish (D-01 semantics intact); only -32601 marks the sticky degradation so later asks skip the round-trip"
  - "Unknown accept content keys are IGNORED by the validator (they can neither execute nor render — the renderer walks the schema's own property order), so defense-in-depth stays strict on required/type/membership without false-rejecting real clients"
  - "Plan-approval asks stay on the captured string-reply path: the plan's conversion scope is question-shaped asks ('the whole ask family' = model questions, learning-store, engine); ExitPlanMode's approval/denial rendering + gate flip are a distinct resume contract"

patterns-established:
  - "Pattern: seam-widening by byte-identity — a structured input renders into the legacy string form through the legacy renderer itself, so capture-pinned goldens hold by construction"
  - "Pattern: replyAnswerable surfaces — an ask entry declares whether its degraded plain-text surface can still be answered; non-answerable families skip the dead-end publish"

requirements-completed: [ACP-02]

# Coverage metadata (#1602) — one entry per shipped deliverable
coverage:
  - id: D1
    description: "The D-08 mapping is exhaustively pinned: single-choice → string oneOf titled consts, multiSelect → array items (enum, anyOf-titled when descriptions exist), free-text/empty-Options → string, boolean → boolean ONLY when advertised else two-value string oneOf, N questions → N required q1..qN properties, enumNames never, question text in the message, re-ask note rides the message"
    requirement: ACP-02
    verification:
      - kind: unit
        ref: "internal/acpserve/ask_surface_test.go#TestElicitationMapping"
        status: pass
      - kind: other
        ref: "go vet ./internal/acp/ ./internal/acpserve/ clean"
        status: pass
    human_judgment: false
  - id: D2
    description: "Capability-gated dispatch: sticky capability ok → one elicitation/create round-trip under the registry (accept/decline/cancel mapped to the session-native outcomes); probe-degraded → the plain-text fallback with byte-parity RenderAskSurface content and ZERO registry writes; -32601 → fallback + sticky (the second ask never re-round-trips)"
    requirement: ACP-02
    verification:
      - kind: unit
        ref: "internal/acpserve/ask_surface_test.go#TestAskSurfaceDispatchElicitation"
        status: pass
    human_judgment: false
  - id: D3
    description: "The structured-reply seam preserves the captured answered form byte-for-byte: single string field → RenderAskAnswered verbatim (the fixture template shape asserted literally); multi-field → property-order title=value lines, arrays comma-joined, booleans true/false, missing values empty"
    requirement: ACP-02
    verification:
      - kind: unit
        ref: "internal/session/elicitation_reply_test.go#TestStructuredReplyRender"
        status: pass
      - kind: unit
        ref: "internal/coreexec golden suite (go test ./internal/coreexec/ -count=1) — green untouched"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-10 lands as a bounded loop end-to-end through the real suspension + queue: an invalid accept re-asks EXACTLY once (a NEW queue entry carrying the violation note) then the 12-D-01 non-answer; decline/cancel route directly (one ask total); the validator's closed subset (required/type/membership-by-exact-equality/item-wise arrays/rune-counted bounds, null-content violation, no normalization) is table-pinned"
    requirement: ACP-02
    verification:
      - kind: unit
        ref: "internal/session/elicitation_reply_test.go#TestElicitationRevalidation"
        status: pass
      - kind: unit
        ref: "internal/acpserve/ask_surface_test.go#TestValidateElicitationContent"
        status: pass
      - kind: other
        ref: "grep -c 'regexp|Norm' internal/acpserve/ask_surface.go == 0"
        status: pass
    human_judgment: false
  - id: D5
    description: "The whole ask family rides one queue to one dispatcher: the Run-level AskUserQuestion round-trip (suspend → one foreground entry → held dialog → accept → captured-form tool result → resumed turn, provider streamed twice, ErrSuspended contract intact) and the engine-ask round-trip (background class, accept writes the learning entry through the existing RecordCandidate API, decline lands the 12-D-05 advisory note subscriber-backed, invalid accept re-asks once)"
    requirement: ACP-02
    verification:
      - kind: unit
        ref: "internal/runtime/ask_wiring_test.go#TestAskWiring_ElicitationQueueRoundTrip"
        status: pass
      - kind: unit
        ref: "internal/runtime/ask_wiring_test.go#TestAskWiring_EngineAskConversion"
        status: pass
      - kind: unit
        ref: "internal/acpserve/ask_surface_test.go#TestFamilyConversionEngineAsk"
        status: pass
      - kind: other
        ref: "mise ci green (vet + golangci-lint v2 0 issues + CGO_ENABLED=0 build + go test -race ./...)"
        status: pass
    human_judgment: false
  - id: D6
    description: "Live-Zed structured form rendering (criterion 3's operator leg): the D-08-mapped form renders as a clickable native form in a real editor session"
    requirement: ACP-02
    verification: []
    human_judgment: true
    rationale: "Live rendering is 17-05's operator checkpoint per the plan's verification section (wire/test-level coverage shipped here); needs a human in a real editor session"

# Metrics
duration: 51 min
completed: 2026-09-01
status: complete
---

# Phase 17 Plan 04: Elicitation Forms — Ask Family Conversion Summary

**The whole ask family (model questions, learning-store, engine asks) surfaces as D-08-mapped elicitation/create forms with probe-and-degrade fallback to today's plain-text path verbatim, a structured-reply seam that preserves the captured answered form byte-for-byte, and D-10's bounded re-validation loop**

## Performance

- **Duration:** 51 min
- **Started:** 2026-09-01T01:55:37Z
- **Completed:** 2026-09-01T02:47Z
- **Tasks:** 3
- **Files modified:** 12 (1 created, 11 modified)

## Accomplishments
- D-08 complete: every existing ask kind has a wire-exact form rendering — single-choice → string property with oneOf titled consts (the enum names ARE the option labels, enumNames never), multiSelect → array property (enum, anyOf-titled when descriptions exist), free-text/empty-Options → string property, boolean ask → boolean property only when the client advertised boolean support else a two-value string oneOf; N questions → N required properties on stable q1..qN keys; the question text lands in the request message
- Criterion 3's wire half works end-to-end: on a capable client every ask is a clickable elicitation/create form dispatched by the sticky capability under HUMAN-ASK; on older clients the -32601/probe-degraded surface falls back to today's plain-text AskBroker path VERBATIM (byte-parity pinned, zero registry writes, sticky degradation with no re-probe)
- The captured answered form survives the structured upgrade byte-for-byte: a single string-valued field renders through RenderAskAnswered itself, multi-field replies render deterministically in schema property order — the zcode-interactive-results.json golden stays green untouched
- D-10 lands as a bounded loop: an invalid accept re-validates against the requested schema (the closed validator subset — no pattern engine, no normalization, exact equality, rune-counted bounds) and re-asks EXACTLY once with the violation named in the new message; a second failure, decline, and cancel all route to the 12-D-01 non-answer
- D-09 complete: model questions, learning-store asks, and engine asks all ride the ONE queue to ONE dispatcher — consistent editor UX, the executor suspension contract (ErrSuspended + questions in Output) untouched, answer persistence unchanged (RecordCandidate)

## Task Commits

Each task was committed atomically (TDD RED→GREEN for Tasks 1-2):

1. **Task 1: D-08 mapping + elicitation frames + capability-gated dispatch**
   - `e4056a2` test(17-04): add failing elicitation mapping + dispatch battery (RED)
   - `bc1f1a8` feat(17-04): implement the D-08 elicitation mapping + capability-gated dispatch (GREEN)
2. **Task 2: Structured-reply seam + D-10 re-validation loop**
   - `612f6ed` test(17-04): add failing structured-reply + D-10 revalidation battery (RED)
   - `e1238d7` feat(17-04): implement the structured-reply seam + D-10 bounded re-validation loop (GREEN)
3. **Task 3: Family conversion wiring — AskUserQuestion, learning-store, engine asks**
   - `996008b` feat(17-04): convert the whole ask family through the queue + dispatcher (D-09)

## Files Created/Modified
- `internal/acp/types.go` — ElicitationFormFrame/ElicitationSchema (raw per-property variants)/ElicitationOutcomeFrame v1-verbatim; MethodElicitationCreate exported; form/accept/decline/cancel vocabulary; CapElicitationForm + CapBooleanConfigOption
- `internal/acp/handlers.go` — the initialize negotiation now also caches the boolean config-option advertisement (D-08's gate input); 16-03's probe payload untouched
- `internal/acpserve/ask_surface.go` — BuildElicitationForm, the ElicitationAsk dispatcher (capability-gated, sticky-degrading, plain-text fallback), ValidateElicitationContent
- `internal/acpserve/acp_serve.go` — the ElicitationAsk composition (capability readers + the runner's fallback publish)
- `internal/session/ask.go` — StructuredPropertyKey, RenderStructuredReply, ResolveAskStructured, resumeAskForm, EnqueueElicitationAsk + the D-10 loop, EnqueueEngineAsk
- `internal/session/askqueue.go` — AskOutcome elicitation fields (Elicit/Content/Violation/Fallback); AskEntry.Note/PlainTextFallback
- `internal/runtime/runtime.go` — the AskBroker onSurface conversion, SetAskFire/askFire, PublishAskChunk, the engine ask-pending conversion (advisoryFromDecision + enqueueEngineAsk)
- Tests: `internal/session/elicitation_reply_test.go` (new), `internal/acpserve/ask_surface_test.go`, `internal/runtime/ask_wiring_test.go`, `internal/acp/server_test.go`

## Decisions Made
- **Byte-identity anchoring:** the structured seam's single string-valued field renders THROUGH the legacy renderer — the model's view of an answered ask cannot drift, and the golden suite enforces it by construction (Pitfall 6 dead by design)
- **Boolean-ask convention:** the captured AskQuestion schema has no boolean flag, so an authored yes/no two-option pair IS the boolean ask; the boolean property is gated on the real session.configOptions.boolean advertisement (D-08's conservative reading, now fully wired)
- **Fail-safe degrade family:** every degraded or malformed elicitation case lands on the plain-text fallback — never a dead ask, never a fabricated answer; only -32601 is sticky (mirroring the permission surface's 16-D-18 discipline)
- **Engine asks are not reply-answerable:** their degraded surface is the advisory note (AskEntry.PlainTextFallback=false suppresses the dead-end question publish), and their accept persists through the learning store's existing RecordCandidate — persistence behavior unchanged, only the surface differs (A9)
- **Lenient on unknown content keys:** the validator ignores extra accept keys (they can neither execute nor render); strict on required/type/membership — defense-in-depth without false-rejecting real clients
- **Plan approval stays captured-shape:** ExitPlanMode's approval/denial forms + gate flip are a distinct string-reply resume contract outside the plan's question-shaped scope — left on today's path verbatim

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Composition + wiring seams beyond the plan's file list**
- **Found during:** Tasks 1-3 (GREEN)
- **Issue:** the dispatcher's input/outcome contract lives on session.AskEntry/AskOutcome (Note, PlainTextFallback; Elicit/Content/Violation/Fallback), the serve composition that binds ElicitationAsk + SetAskFire lives in acp_serve.go, and the real AskUserQuestion round-trip harness exists only in runtime/ask_wiring_test.go — none were in the plan's file lists, yet all three are named by the plan's own key_links/artifacts
- **Fix:** minimal touches — six fields + one composition block + one wiring test file; internal/session still never imports internal/acp in production code
- **Files modified:** internal/session/askqueue.go, internal/acpserve/acp_serve.go, internal/runtime/ask_wiring_test.go (plus internal/acp/server.go/server_test.go for the capability-key export)
- **Verification:** full battery + mise ci green
- **Committed in:** bc1f1a8, e1238d7, 996008b

**2. [Rule 1 - Bug] Test-only encoding drift in the normalization golden**
- **Found during:** Task 2 (GREEN)
- **Issue:** the no-normalization test's NFC "café" literal was written to the file as a decomposed sequence, making the exact-const case fail spuriously
- **Fix:** the subtest now builds both variants from explicit `\u00e9` / `\u0301` escapes — byte-explicit, no encoding ambiguity
- **Files modified:** internal/acpserve/ask_surface_test.go
- **Verification:** TestValidateElicitationContent green (composed passes, decomposed violates)
- **Committed in:** e1238d7

---

**Total deviations:** 2 auto-fixed (1 wiring-seam family, 1 test encoding). **Impact on plan:** all within the plan's architecture; the file-list gaps are the seams the plan's own key_links name.

## Issues Encountered
None new — the window-flake family from 17-01/17-02/17-03 did NOT recur (results-based waits were used throughout the new batteries; the final `mise ci` was green on the first full run with zero FAIL lines).

## TDD Gate Compliance
- Tasks 1 and 2 followed RED→GREEN with `test(17-04)` commits preceding `feat(17-04)` commits (e4056a2→bc1f1a8, 612f6ed→e1238d7); Task 3 is `type="auto"` (no `<behavior>` block) and landed as a single feat commit (996008b); no REFACTOR commits needed (lint-clean at each GREEN).

## Known Stubs

None — every deliverable is wired end-to-end through the real queue, registry, and store APIs; no placeholder paths.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- 17-05 owns the phase's documentation + operator legs: the chokepoint/queue/surface doc task can pin the elicitation dispatcher contract (sticky capability → form; degraded → plain text; D-10 bounded loop) alongside the gate audit; the live-Zed form-rendering checkpoint (D6 above) joins criterion 1's native-dialog leg
- ACP-02's REQUIREMENTS.md checkbox is intentionally UNMARKED: 17-05 also declares ACP-02 (the shared-ID gate — the last declaring sibling marks it once its SUMMARY exists)
- Phase 21's hooks note: the question-family queue entries already carry turn IDs, so hook-driven asks (if any) inherit the same firing monopoly, notes/counter, and drain without new machinery

---
*Phase: 17-permissions-elicitation*
*Completed: 2026-09-01*

## Self-Check: PASSED

- Created files verified on disk: internal/session/elicitation_reply_test.go (+ all 5 must-have artifact paths present)
- All 5 task commits verified in git log: e4056a2, bc1f1a8, 612f6ed, e1238d7, 996008b
- Plan-level verification re-run green: `go test -race ./internal/acpserve/ ./internal/session/ -count=1`, `go test ./internal/coreexec/ ./internal/runtime/ -count=1`, the validator grep (0 hits), `go vet ./internal/acp/ ./internal/acpserve/` clean, and final `mise ci` green (vet + golangci-lint v2 0 issues + CGO_ENABLED=0 build + go test -race ./... — zero FAIL lines)
- All task acceptance criteria individually re-verified (D-08 row goldens, dispatch ok/degraded/sticky, byte-identity + golden suite, exactly-two-asks re-validation, decline/cancel single-ask, grep-based fallback-path check, mise ci)
