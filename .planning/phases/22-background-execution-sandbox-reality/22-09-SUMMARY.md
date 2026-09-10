---
phase: 22-background-execution-sandbox-reality
plan: 09
subsystem: coreexec
tags: [background-execution, taskstop, taskoutput, tracker, subagent, cancellation, tdd, gap-closure]

# Dependency graph
requires:
  - phase: 22-background-execution-sandbox-reality (22-07)
    provides: releaseSubagentSlot + cancel-entry retirement in Complete (TestTrackerComplete_RetiresCancelEntry) — the finished-vs-running disambiguation substrate this plan's classifier consumes
  - phase: 22-background-execution-sandbox-reality (22-08)
    provides: CancelRunning + the non-constructing-lookup close semantics — the close-time cancellation contract the wiring rows assume (running subagents die with the session)
  - phase: 22-background-execution-sandbox-reality (22-01..22-06)
    provides: the tasks Tracker, the sessionFor construction holding both taskRegistry and tracker, the runner/session harness in wake_wiring_test.go
provides:
  - G-22-5 (WR-01, PAR-07 cancellation) closed — TaskStop/TaskOutput address background-subagent task ids: ids unknown to the TaskRegistry fall through to the tracker seam, Tracker.CancelTask gains production callers, and the coretools.json promise "Works with all task types" is no longer false for async agents
  - The coreexec primitive-arg seam pair (InteractiveConfig.TaskStopFallback/TaskOutputFallback) — no coreexec dependency on the tasks package (the CompletionHook precedent)
  - Tracker.SubagentState — the truthful queued/running classifier whose finished row is truthful BECAUSE 22-07 retired cancel entries
affects: [internal/coreexec consumers, internal/runtime sessionFor, internal/tasks consumers, Phase 23+ steering (stop/read surface)]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
# Same estimateTokens scale (chars/4 over the realized diff), never a harness token count.
actuals:
  tokens: 9080   # chars/4 over the realized diff (36,320 chars across the 8 source files)
  tasks: 2
  commits: 4     # MEASURED: git rev-list --count 87a83ab..HEAD

# Tech tracking
tech-stack:
  added: []      # zero new deps; stdlib only (io/os/path/filepath already imported)
  patterns:
    - "primitive-arg fallback seams: constructors take func(id string) shapes, never package types — the dependency arrow stays runtime→tasks, coreexec stays tasks-free (the CompletionHook precedent, extended to the retrieval/stop pair)"
    - "one envelope renderer for both id families: renderTaskOutput serves the registry path AND the fallback path, so the fallback forms are byte-identical to the registry's by construction (one form, never two)"
    - "signature-first RED for API-extension tasks: introduce the new params/method with a zero-value body so the package compiles and the target rows fail on assertions (a compile failure is INVALID_RED per #3770)"
    - "classifier-over-map-presence: SubagentState reads waiters + subagentCancels under one lock; finished-vs-running is decided by an ABSENCE that 22-07's delete created — the classifier is only as truthful as the retirement it consumes"

key-files:
  created: []
  modified:
    - internal/coreexec/background.go
    - internal/coreexec/planmode.go
    - internal/coreexec/background_test.go
    - cmd/ass-guard/background_wiring_test.go
    - internal/tasks/tracker.go
    - internal/tasks/tracker_test.go
    - internal/runtime/runtime.go
    - internal/runtime/wake_wiring_test.go

key-decisions:
  - "Signature-first RED (the plan's authorized order) for both tasks: a test battery calling not-yet-existing signatures cannot compile, and a compile failure classifies as INVALID_RED — so the params/method are introduced with zero-value bodies and the rows fail on assertions (RED_EVIDENCE_OK for both tasks)"
  - "The fallback contract maps running=true to 'renders not_ready': queued carries (\"\", \"queued\", true, true) — mirroring the registry's own Output semantics where a queued task also renders not_ready via the state arm; finished carries (content, \"finished\", false, true)"
  - "A stat miss on the output file reports not-handled → the structured unknown-task error: a never-registered id (or a finished id whose file is gone) is nobody's, and the truthful error beats a fabricated envelope"
  - "block/timeout documented as IGNORED for fallback ids (stub comment): the tracker seam renders the CURRENT state immediately — the schema's promise is the id is ADDRESSABLE; the pre-22-09 lie was 'unknown task'"
  - "readTail is a bounded seek+LimitReader tail (last 64 KiB), never a whole-file load — the D-02 tail discipline's TaskOutput analogue; the full file stays a Read away exactly as the notification's OutputFile pointer promises"

patterns-established:
  - "Acceptance-grep-clean comments: when a plan pins a grep (e.g. no 'internal/tasks' string in coreexec), doc comments must avoid the literal too — word the invariant, not the path"
  - "Wiring rows neutralize the tracker drain (SetDrain(noop)) to pin the STOP/READ path at zero provider calls — the wake path has its own battery; a completion in a stop/read test must not conflate the two"

requirements-completed: [PAR-07]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "G-22-5 seam (Task 1): TaskStop/TaskOutput fall through to primitive-arg fallback seams on registry-unknown ids — the captured ack on a claiming stop fallback; the captured not_ready (queued/running) or ready (finished) envelope shapes with the fallback's output; declined/nil keep the structured unknown-task error unchanged; registry-owned ids never consult the seam (existing rows byte-stable; the four cmd wiring sites compile on the two-arg nil form)"
    requirement: PAR-07
    verification:
      - kind: unit
        ref: internal/coreexec/background_test.go#TestTaskStop_FallbackAck
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestTaskStop_FallbackDeclined
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestTaskStop_NilFallback
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestTaskStop_RegistryOwnedSkipsFallback
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestTaskOutput_FallbackNotReadyStates
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestTaskOutput_FallbackFinishedOutput
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestTaskOutput_FallbackDeclined
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestTaskOutput_NilFallback
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestTaskOutput_RegistryOwnedSkipsFallback
        status: pass
      - kind: command
        ref: "go test -race -count=1 ./internal/coreexec/ (full package — existing TaskStop/TaskOutput rows green byte-identically)"
        status: pass
      - kind: command
        ref: "go test -race -count=1 ./cmd/ass-guard/ -run 'TestBackgroundWiring' (four nil-fallback sites, CrossSessionIsolation's structured error preserved)"
        status: pass
    human_judgment: false
  - id: D2
    description: "G-22-5 wiring (Task 2): Tracker.SubagentState truthfully classifies queued/running/finished (the finished row is the disambiguation 22-07's cancel-entry retirement enables), and sessionFor binds the seams to the session's tracker — the model stops a live background subagent by its exec_ id (cancel fires, captured ack, zero provider calls), a finished id's TaskStop renders the truthful structured error, and a finished subagent's output file surfaces through the ready envelope (the classifier-fix regression row)"
    requirement: PAR-07
    verification:
      - kind: unit
        ref: internal/tasks/tracker_test.go#TestTrackerSubagentState
        status: pass
      - kind: unit
        ref: internal/runtime/wake_wiring_test.go#TestTaskStopFallbackWiring
        status: pass
      - kind: unit
        ref: internal/runtime/wake_wiring_test.go#TestTaskOutputFallbackWiring
        status: pass
      - kind: command
        ref: "go test -race -count=1 -skip 'TestRescanConcurrency' ./internal/coreexec/ ./internal/tasks/ ./internal/runtime/ (the closure round's FINAL three-package gate)"
        status: pass
      - kind: command
        ref: "go vet ./internal/coreexec/ ./internal/tasks/ ./internal/runtime/ + GOOS=linux CGO_ENABLED=0 go build ./... (no new deps)"
        status: pass
    human_judgment: false

# Metrics
duration: 22 min
completed: 2026-09-10
status: complete
---

# Phase 22 Plan 09: Gap closure — coreexec seam cluster (G-22-5: TaskStop/TaskOutput reach subagent task ids) Summary

**TaskStop/TaskOutput now address background-subagent exec_ ids through two primitive-arg fallback seams (coreexec fallthrough → tracker CancelTask / SubagentState + a bounded 64 KiB output-file tail) — Tracker.CancelTask gains its first production callers, finished reads finished, and PAR-07's model-facing cancellation letter is reachable end to end**

## Performance

- **Duration:** 22 min
- **Started:** 2026-09-10T03:36:02Z
- **Completed:** 2026-09-10T03:58:14Z
- **Tasks:** 2
- **Files modified:** 8 (+2 RED-evidence records in .planning)

## Accomplishments

- **G-22-5 (WR-01, PAR-07) closed — the coreexec seam (Task 1):** `TaskStopExecute`/`TaskOutputExecute` arm a fallthrough ONLY on the registry's own unknown-task error (`errors.Is(errUnknownTask)`): a claiming `stopFallback` renders the same captured ack; a handled `outputFallback` renders through `renderTaskOutput` — the ONE envelope pair now shared with the registry path, so the fallback forms are byte-identical by construction. Declined or nil seams keep the structured unknown-task error byte-stable (the pre-22-09 behavior every existing row and the cmd wiring battery pin). `InteractiveConfig` gains `TaskStopFallback`/`TaskOutputFallback` (primitive-arg seam — no coreexec dependency on the tasks package, the :2017 CompletionHook precedent) and binds them at the stubs map.
- **G-22-5 — the truthful classifier + the wiring (Task 2):** `Tracker.SubagentState(id)` scans waiters and `subagentCancels` under one lock — and its finished row is truthful BECAUSE 22-07's `releaseSubagentSlot` deleted completed cancel entries (the disambiguation the original closure sketch could not express). `sessionFor` binds `TaskStopFallback: tracker.CancelTask` (first production callers) and `TaskOutputFallback: subagentOutputFallback(dir, tracker)` — classify via SubagentState, else a bounded 64 KiB tail read of `.ass-guard/outputs/<id>.log`; a stat miss reports not-handled (the id is nobody's).
- **End-to-end rows:** the session catalog's TaskStop with a live subagent's exec_ id fires the registered cancel func and renders the captured ack; TaskStop on a finished id renders the truthful structured error (no fake ack — nothing is left to stop); TaskOutput on a finished id surfaces the output-file content through the ready envelope — the classifier-fix regression row (the pre-fix surface rendered the unknown-task error, and the original sketch would have rendered not_ready forever). Zero provider calls in both wiring rows: a stop or a read never wakes turns.
- **Strictly additive:** registry-owned TaskStop/TaskOutput rows byte-stable (full coreexec package green), the full internal/tasks battery green, and the closure round's FINAL three-package gate (`-skip 'TestRescanConcurrency'` — the deferred pre-existing rescan race, deferred-items.md) green across coreexec/tasks/runtime with every 22-01..22-08 row.

## Task Commits

Each task was committed atomically (TDD: RED test commit → GREEN implementation commit):

1. **Task 1 RED: coreexec fallback seam battery** - `6c579da` (test)
2. **Task 1 GREEN: TaskStop/TaskOutput unknown-id fallthrough** - `65625b3` (feat)
3. **Task 2 RED: classifier battery + session wiring rows** - `da31f03` (test)
4. **Task 2 GREEN: SubagentState classifier + sessionFor seam binding** - `05a18ec` (feat)

**Plan metadata:** see the docs(22-09) commit following this summary.

## TDD Gate Compliance

Both tasks executed full RED→GREEN cycles under `tdd_mode: true`:

| Task | RED commit | RED evidence | GREEN commit | Gate |
|------|-----------|--------------|--------------|------|
| 1 (G-22-5 seam) | `6c579da` test(22-09) | `RED_EVIDENCE_OK` (target_test_failed; exit 1, 3 tests / 4 rows failing on assertions: the structured unknown-task error instead of the ack and the not_ready/ready envelopes; 6 preserved-behavior witnesses green) | `65625b3` feat(22-09) | PASS |
| 2 (G-22-5 wiring) | `da31f03` test(22-09) | `RED_EVIDENCE_OK` (target_test_failed; exit 1, 3 tests failing on assertions: queued/admitted classifier rows + both wiring rows on the structured unknown-task error; precondition delete-count = 1 verified first) | `05a18ec` feat(22-09) | PASS |

REFACTOR: not needed — both GREEN implementations are minimal and the batteries pass as written (Task 1's envelope-renderer extraction was part of its GREEN prescription, not a separate refactor step).

## Files Created/Modified

- `internal/coreexec/background.go` — both constructors gain the fallback params + the unknown-task-error fallthrough; `renderTaskOutput` extracted as the ONE envelope renderer for both id families; block-ignored choice documented in the stub comment
- `internal/coreexec/planmode.go` — `InteractiveConfig.TaskStopFallback`/`TaskOutputFallback` fields + stubs-map binding; nil = the fallthrough never fires
- `internal/coreexec/background_test.go` — the 9 seam rows (ack / declined / nil / registry-owned × both tools; not_ready queued+running byte-exact; finished ready envelope byte-exact) + the 4 existing sites on the two-arg nil form
- `cmd/ass-guard/background_wiring_test.go` — the four constructor sites (:46/:64/:93/:138) on the two-arg nil form — CrossSessionIsolation's structured unknown-id error preserved exactly
- `internal/tasks/tracker.go` — `SubagentState(id)` (waiters scan + subagentCancels lookup under t.mu; doc comment cites TestTrackerComplete_RetiresCancelEntry as the disambiguation substrate)
- `internal/tasks/tracker_test.go` — `TestTrackerSubagentState` four rows (queued waiter / admitted running / completed NEITHER / never-registered neither)
- `internal/runtime/runtime.go` — sessionFor binds the seams to this session's tracker; `subagentOutputFallback` + `readTail` (bounded 64 KiB tail; stat-miss = not-handled)
- `internal/runtime/wake_wiring_test.go` — `TestTaskStopFallbackWiring` (live cancel fires + finished-id truthful error; zero calls) + `TestTaskOutputFallbackWiring` (the classifier-fix regression row; zero calls), serial per the 22-08 rule

## Decisions Made

- **Signature-first RED (the plan's authorized order) for both tasks:** the new rows must call the TARGET signatures, but a not-yet-existing signature cannot compile — and a compile failure classifies as INVALID_RED (#3770). Introducing the params/method with zero-value bodies keeps the package compiling so the rows fail on assertions (both records verified RED_EVIDENCE_OK before GREEN).
- **The fallback contract's `running` bool means "renders not_ready":** queued carries `("", "queued", true, true)` — mirroring the registry's own semantics where a queued task also renders not_ready (via the `state == bgQueued` arm); finished carries `(content, "finished", false, true)` and hits the ready arm.
- **Stat-miss = not-handled:** a never-registered id, or a finished id whose output file is gone, keeps the structured unknown-task error — the truthful error beats a fabricated envelope, and no id-guessing surface is added (ids stay session-local crypto/rand values).
- **One envelope renderer:** extracting `renderTaskOutput` (rather than duplicating the envelope strings in the fallthrough) guarantees the fallback forms cannot drift from the registry's — the mimicry discipline applied to internal forms.
- **Comment wording follows the acceptance greps:** the plans pin `grep 'internal/tasks' finds nothing` in coreexec and the exact `TaskStopFallback: tracker.CancelTask` binding — doc comments state the invariant without the literal path, and the binding's alignment group is split so the single-space grep form survives gofmt.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None. (Pre-existing, out of scope, not touched: the `gofmt -l` flags in internal/tasks/{notify.go,notify_test.go,tracker.go} are the pre-existing TrackerOpts/comment-alignment regions — verified via `gofmt -d` that this plan's hunks are clean; the known TestRescanConcurrency race (deferred-items.md, status open — excluded by the plan's -skip) and TestPermissionsE2E (Phase 23 commit) remain tracked elsewhere.)

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- G-22-5 is closed with named regression batteries that were red against the pre-fix code (re-verification mapping: G-22-5 → the 9 coreexec seam rows + TestTrackerSubagentState (four rows, finished disambiguated) + TestTaskStopFallbackWiring + TestTaskOutputFallbackWiring).
- The gap-closure round (22-07/22-08/22-09) is complete: G-22-1..G-22-5 all closed; the FINAL three-package gate green with the `-skip 'TestRescanConcurrency'` scoping (only the deferred pre-existing rescan race excluded).
- PAR-07: this was the last declaring plan — its SUMMARY landing makes PAR-07 ready to mark complete via the requirements.ready-ids shared-ID gate.
- Scope guard unchanged: WR-02..WR-08, IN-01/IN-02/IN-04 (and IN-03's remaining half) stay deferred review debt per 22-07's objective.
- Phase 22 is now 9/9 plans summarized — ready for phase verification.

## Self-Check: PASSED

All 8 modified source/test files exist on disk; all 4 task commits (6c579da, 65625b3, da31f03, 05a18ec) exist in git log; TDD gate sequence verified (test(22-09) × 2 each precede their feat(22-09)); every acceptance criterion re-run green in the final battery (both scoped runs, the full coreexec package, the cmd leg, the three-package gate with the -skip scoping, vet on all three packages, CGO_ENABLED=0 static build, grep counts 3/3 for the planmode fields, 1/1 for SubagentState and the runtime binding, internal/tasks grep clean in coreexec).

---
*Phase: 22-background-execution-sandbox-reality*
*Completed: 2026-09-10*
