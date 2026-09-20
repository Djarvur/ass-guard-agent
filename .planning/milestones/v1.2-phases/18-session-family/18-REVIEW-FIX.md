---
phase: 18-session-family
fixed_at: 2026-09-03T17:15:37Z
review_path: .planning/phases/18-session-family/18-REVIEW.md
iteration: 1
findings_in_scope: 5
fixed: 5
skipped: 0
status: all_fixed
---

# Phase 18: Code Review Fix Report

**Fixed at:** 2026-09-03T17:15:37Z
**Source review:** .planning/phases/18-session-family/18-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope (critical_warning): 5 (CR-01, WR-01..WR-04)
- Fixed: 5
- Skipped: 0
- Info findings IN-01..04: out of default scope, left open in 18-REVIEW.md

**Verification (ran in the MAIN checkout — `workflow.use_worktrees` is `false`):**
`go test -race -count=1 ./internal/acp/ ./internal/runtime/ ./cmd/ass-guard/` — ok
(3.1s / 62.8s / 52.3s); `go test -race ./internal/acpserve/ ./internal/session/` — ok;
`go vet ./...` clean; `gofmt -l` empty on every touched package. `mise ci`'s lint
step remains red from the pre-existing golangci-lint version drift (WINDOWS
ledger) — lint-cleanliness of the touched files covered by gofmt + go vet.

## Fixed Issues

### CR-01: session/delete racing an in-flight session/load resurrects a deleted session

**Files modified:** `internal/acp/handlers.go`, `internal/acp/session_family_test.go`
**Commit:** 4c67190
**Applied fix:** `LoadSession` re-stats the `<id>.deleted` marker under `s.mu` in
ONE critical section with the `s.sessions` insert — the insert is the last step
of the load, so it is also the last tombstone check; a delete that raced the
replay fails the load with the typed `deleted during replay` error.
`handleSessionDelete` additionally writes the marker BEFORE
`closeSessionSequence`'s close lookup: with the marker second, a load could
insert between the (missing) lookup and the marker write, leaving a live session
over a tombstone no close ever reaps. Marker-first + the insert-side gate make
both interleavings safe. Beyond the review sketch (which left that second
window open).
**Pin:** `TestSessionDeleteDuringLoadNeverResurrects` — RED-first: the parked-
replay construction (unread pipe + capacity-1 foreground lane, from the D-03
gate test) deterministically reproduces delete-during-load; pre-fix the load
succeeded (resurrected), post-fix it fails typed and no sessionState exists.

### WR-01: Runner.ResumeSession skips the per-session turn mutex

**Files modified:** `internal/runtime/runtime.go`, `internal/runtime/resume_session_test.go`
**Commit:** a48435e
**Applied fix:** the ReadAll → Reconcile → AppendSynthetic → SeedResume block in
`ResumeSession` now holds `r.sessionTurnMu(sessionID)` for its whole extent
(after the `sessionFor` nil-guard, mirroring `Run`'s own ordering) — the same
mutex `Run` and every injected async resume driver serialize through
(17-REVIEW CR-02 discipline extended to the load path).
**Pin:** `TestResumeSessionSerializesWithTurnMutex` — RED-first 3/3 runs: a
warmed session cache isolates the mutex from construction cost; pre-fix the
resume block completed under a held turn mutex, post-fix it queues and
completes on release.

### WR-02: session/close racing session/prompt can drain past a turn that registers afterwards

**Files modified:** `internal/acp/handlers.go`, `internal/acp/cancel_session_test.go`
**Commit:** a6dd607
**Applied fix:** the inline `turnWG.Add(1)` became `registerTurn`: the Add lands
FIRST, then liveness is re-confirmed under `s.mu` by IDENTITY (a same-id re-load
constructs a new sessionState; only the state the gate vetted may run — stricter
than the review's presence check). Close ahead of the Add → the typed
`closed while the prompt was arriving` error; close after it → the drain waits.
**Pin:** `TestPromptRegistrationAbortsAfterCloseReaped` — RED-first against the
extracted-but-unguarded registration: the gate→close→Add interleaving sequenced
at the method level (the wire window is microseconds); pre-fix the Add was
accepted, leaking a ghost turnWG count.

### WR-03: In-process re-load after session/close reuses a half-reaped runner session

**Files modified:** `internal/runtime/runtime.go`, `internal/runtime/resume_session_test.go`, `internal/runtime/ask_wiring_test.go`
**Commit:** 14b9d70
**Applied fix (design: evict on close):** `Runner.CloseSession` deletes the
entry from `r.sessions` under `sessMu` BEFORE `s.Close()` — eviction-first keeps
a concurrent `sessionFor` from joining a session mid-reap, and the next
`sessionFor` of the id reconstructs a full session (new writer ctx, MCP host,
forwarder, SessionEnd chain). `countEngineDecisions` (test helper) re-read
`r.sessions[sid]` after close and nil-derefed under eviction; it now takes the
`*session.Session` captured before the close.
**Pin:** `TestInProcessReloadAfterCloseRebuildsSession` — RED-first: pre-fix
`CloseSession` left the entry and the re-load adopted the closed pointer;
post-fix the entry is gone and the rebuild is a new, seeded, functional session
(`CurrentTurnID` continues the on-disk sequence).

### WR-04: --continue + --resume both typed — code contradicted the documented precedence

**Files modified:** `cmd/ass-guard/acp_serve.go`, `cmd/ass-guard/resume_flags_test.go`
**Commit:** ed5261d
**Applied fix (decision: the doc is right, --resume wins):** `resolveResumeTarget`'s
`--continue` fast path now requires an absent `--resume`; bare
`--resume --continue` (target-less combination) resolves as --continue's newest
row — the picker never fires inside the combination, so an unattended serve
start cannot stall on terminal input.
**Pin:** `TestResumeWinsOverContinue` — RED-first on both explicit-target forms
(`--continue --resume=<id>` and the space form resolved to the newest row
pre-fix); the bare-combination case pins the newest-row fallback as a
regression guard.

## Skipped Issues

None — all five in-scope findings fixed.

---

_Fixed: 2026-09-03T17:15:37Z_
_Fixer: ZCode (gsd-code-fixer)_
_Iteration: 1_
