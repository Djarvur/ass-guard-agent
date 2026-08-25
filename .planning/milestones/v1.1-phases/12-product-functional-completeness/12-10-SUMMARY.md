---
phase: 12-product-functional-completeness
plan: "10"
subsystem: tool-execution
tags: [webfetch, transcript-integrity, g-12-3b, tdd, gap-closure]

requires:
  - phase: 08-slash-command-kickoff
    provides: "the session tool loop + Manager.appendLine Marshal path the fix routes through"
  - phase: 14-adoption-readiness
    provides: "14-05 CR-02's subagent-payload JSON-encode precedent (same defect class; the web passthrough was its missed sibling)"
provides:
  - "DefaultBackend.Fetch non-HTML passthrough returns VALID JSON ({\"content\": string}) — a text/plain fetch can no longer poison the transcript write path"
  - "appendToolResultLoud chokepoint in internal/session — every AppendToolResult site degrades loud (stderr warn + fallback {\"error\":…} line keyed by the call id) instead of vanishing"
  - "Regression battery: text/plain fetch → transcript-visible tool_result through the REAL session loop (TestSession_TextPlainFetchLandsInTranscript)"
affects: []

gap_ids: [G-12-3b]

actuals:
  tokens: 4985   # chars/4 over the realized diff (19938 chars); estimate was 60000 — planner over-estimated ~12x on this small surgical fix
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Loud-append helper: one package-level wrapper (appendToolResultLoud) every AppendToolResult call site routes through; nil-error behavior byte-identical to today, error behavior = stderr slog.Warn + structured fallback payload keyed by the same call id"

key-files:
  created: []
  modified:
    - internal/toolexec/ddg.go
    - internal/toolexec/ddg_test.go
    - internal/session/session.go
    - internal/session/ask.go
    - internal/session/subagent.go
    - internal/session/session_test.go

key-decisions:
  - "Capture-fidelity disposition for the wrapped shape: WebFetch result forms are corpus-absent in ALL THREE committed fixtures (zcode-recaptured-2026-08.json, zcode-core-results.json, zcode-interactive-results.json — grep-verified during planning), so there is NO captured form to diverge from; {\"content\": …} mirrors the already-shipped HTML branch, which IS the established ass-guard fetched-content convention"
  - "The loudness gate lives in ONE session-package helper (appendToolResultLoud), NOT inside Manager.appendLine — redaction ordering and the appendLine contract stay byte-untouched; all EIGHT bare '_ =' sites converted (session.go x5 incl. subagent-dispatch-error/plan-mode-refusal/no-broker-ask-degrade, subagent.go x2, ask.go x1 — the plan's 'seven sites' undercounted by one)"
  - "Task 3 (repeat-detect hint) SKIPPED honestly per the plan's own condition: no clean injection seam exists in the session tool loop (the only injections are engine-level continue-injections and turn-entry hook system blocks; fabricating a synthetic tool_result would corrupt 08-07 pairing discipline). The storm PRECONDITIONS are eliminated anyway at Task 1+2 — blind retries are no longer individually invisible"

requirements-completed: [ACP-07]

coverage:
  - id: G-12-3b-validity
    description: "Fetch on non-HTML bodies returns valid JSON wrapped as content; over-cap truncates before wrap; JSON content-types travel as original TEXT inside content"
    requirement: ACP-07
    verification:
      - kind: unit
        ref: "internal/toolexec/ddg_test.go#TestFetchNonHTMLPassthrough"
        status: pass
  - id: G-12-3b-loudness
    description: "An AppendToolResult Marshal failure logs stderr naming turn/call-id/tool and appends a fallback {\"error\":…} tool_result keyed by the failing call id"
    requirement: ACP-07
    verification:
      - kind: unit
        ref: "internal/session/session_test.go#TestSession_ToolResultAppendFailureSurfaces"
        status: pass
  - id: G-12-3b-e2e
    description: "Real RealExecutor + text/plain backend through the real session loop leaves a transcript-visible, valid-JSON, non-error tool_result"
    requirement: ACP-07
    verification:
      - kind: integration
        ref: "internal/session/session_test.go#TestSession_TextPlainFetchLandsInTranscript"
        status: pass

duration: 25min
completed: 2026-08-23
status: complete
---

# Phase 12 Plan 10: WebFetch silent-loss closure Summary

**G-12-3b closed at both layers: the non-HTML Fetch passthrough now wraps as valid JSON like the HTML branch (the invalid-JSON source of the 33-call retry storm is structurally gone), and every AppendToolResult site in the session package degrades LOUD — stderr warning plus a model-visible fallback `{"error":…}` line keyed by the call id — so silent transcript loss is mechanically impossible.**

## Performance

- **Duration:** 25 min
- **Completed:** 2026-08-23
- **Tasks:** 2 executed + 1 skipped-with-rationale (optional hardening)
- **Files modified:** 6

## Accomplishments

- The UAT reproduction is now a mechanical regression battery: RED commit `5d1a8d8` failed on the current tree with exactly the diagnosed signatures (`invalid character 'a' looking for beginning of value` from the bare-bytes passthrough; `NO tool_result line` for the poisoned call).
- Layer 1 (source): `DefaultBackend.Fetch`'s non-HTML branch truncates at fetchRawCap THEN wraps via `json.Marshal(map[string]string{"content": string(body)})` with the HTML branch's marshal-error convention.
- Layer 2 (chokepoint): `appendToolResultLoud` covers ALL EIGHT bare `_ = s.Manager.AppendToolResult(...)` sites — session.go x5 (subagent dispatch error, subagent result, plan-mode refusal, the batch result loop [the G-12-3b storm site], no-broker ask degrade), subagent.go x2, ask.go x1. Nil-error behavior unchanged; on failure: slog.Warn naming turn/call-id/tool/cause + a fallback `{"error": …}` line keyed by the SAME call id, hardcoded literal if even that fails to marshal. `Manager.appendLine` untouched (redaction ordering intact).
- End-to-end pin: `TestSession_TextPlainFetchLandsInTranscript` drives the REAL session loop (fake provider + catalog-backed RealExecutor + text/plain-serving Backend) and asserts a transcript-visible, valid-JSON, non-error tool_result carrying the body verbatim.
- The existing `TestFetchNonHTMLPassthrough` was re-pinned IN PLACE (never deleted — SESS-04 discipline), gaining three legs: text/plain wraps, over-cap truncates-before-wrap, JSON content-types travel as original TEXT.

## Task Commits

1. **Task 1 RED** — `5d1a8d8` (test): re-pinned passthrough expectations + the session surfacing test; both FAIL with the UAT signatures
2. **Task 2 GREEN** — `d5e3719` (feat): wrapped passthrough + the eight-site loud fallback; full three-package batteries + `mise ci` green
3. **Regression battery** — `0ec69fc` (test): the end-to-end text/plain→transcript-visible leg (the verification section's regression requirement)

_TDD discipline held: RED commit precedes the behavior-adding GREEN commit._

## Capture-Fidelity Disposition (per plan)

WebFetch result forms are CORPUS-ABSENT in all three committed fixtures — verified by grep during planning and unchanged by execution. There is no captured zcode shape to diverge from, so the `{"content": string}` wrap introduces no fidelity drift; it mirrors the already-shipped HTML branch convention. No fixture update needed; no conformance test pinned the old unwrapped shape outside `TestFetchNonHTMLPassthrough` (re-pinned by Task 1).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Site count corrected: seven named sites were actually EIGHT bare `_ =` calls**
- **Found during:** Task 2
- **Issue:** the plan enumerates five session.go sites + ask.go :403 + subagent.go :181/:183 = seven, but session.go itself carries FIVE (:441 dispatch error, :452 subagent result, :470 plan-mode refusal, :520 batch loop, :551 no-broker ask degrade) — total eight
- **Fix:** all eight routed through the helper; the plan's own truth ("every swallowed AppendToolResult error site") governs over its arithmetic
- **Files modified:** internal/session/session.go, subagent.go, ask.go
- **Committed in:** d5e3719

### Honest Skip

**Task 3 (optional repeated-identical-call hint) — SKIPPED per the plan's own condition.** No clean injection seam exists in the session tool loop: the only hint channels are engine-level continue-injections (chaining machinery, wrong layer) and turn-entry hook system blocks (wrong timing — before the turn, not mid-loop). The plan explicitly forbids fabricating a synthetic tool_result (pairing discipline, 08-07), which is the only remaining channel that would reach the model mid-turn. Rationale recorded here per the plan's done-criterion ("EITHER … OR the SUMMARY records an honest skip"); the UAT marks this hardening OPTIONAL and the two mandatory layers are landed. Note the storm PRECONDITIONS are eliminated regardless: with Layers 1+2, a retried call can no longer be silently invisible — each result lands or screams.

## Verification Results

- RED gate: `/tmp/12-10-red.txt` — both tests FAILED pre-GREEN with the diagnosed signatures (11 FAIL markers).
- GREEN gate: `go test ./internal/toolexec/ ./internal/session/ ./cmd/ass-guard/ -count=1` — all ok.
- Full gate: `mise ci` green (vet + golangci-lint 0 issues + build + race tests across all packages).
- Validity/loudness/regression gates: see coverage table above — all PASS.

## Known Stubs / Residuals

None new. The optional dedup-hint residual is accepted-and-recorded above (T-12-10-03 disposition: mitigated-by-precondition-elimination).

## Self-Check: PASS

- Commits exist: `5d1a8d8`, `d5e3719`, `0ec69fc` (git log verified).
- Files exist on disk: ddg.go, ddg_test.go, session.go, ask.go, subagent.go, session_test.go (all committed, working tree clean of task changes).
- Tests re-run post-commit: three-package battery + lint green.

---
*Phase: 12-product-functional-completeness*
*Completed: 2026-08-23*
