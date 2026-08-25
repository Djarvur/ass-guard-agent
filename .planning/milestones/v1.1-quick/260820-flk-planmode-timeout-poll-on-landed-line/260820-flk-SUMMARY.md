---
quick_id: 260820-flk
slug: planmode-timeout-poll-on-landed-line
date: 2026-08-20
status: complete
commits:
  - ec45a47
---

# Quick Task 260820-flk — Summary

**Task:** `TestPlanMode_TimeoutStaysOn` (internal/session/planmode_test.go, 12-04 battery,
landed today) is flaky under full-battery parallel load: `go test -race -count=1 ./...` failed
twice on it while the isolated package run passes.

**Result:** COMPLETE — test-only fix; the poll now waits for the actual asserted condition
(the landed non-answer tool_result line), not a racy proxy. ask.go untouched (production
ordering — first-claimant-wins, async resume — is correct by design).

## What was wrong

The test polled `s.HasPendingAsk()` until false, then immediately read the transcript. But in
`internal/session/ask.go` `Surface()` the D-01 `time.AfterFunc` callback does `Claim()` FIRST
(clearing pending → `HasPendingAsk()` false) and THEN calls `fire(claimed)` →
`resumeAskClaimed(...)`, which asynchronously appends the tool_result line
(`AppendToolResult`) and resumes the turn. The line lands strictly after the poll condition
clears — a racy proxy that loses only under parallel package load.

## The fix

In `TestPlanMode_TimeoutStaysOn` only: the poll loop keeps its 5s deadline but now breaks only
when BOTH `!s.HasPendingAsk()` AND the transcript contains the `TypeToolResult` line with
`ToolCallID == pmCallID2` whose unmarshalled output is non-empty (a `pmNonAnswer` closure
mirrors the test's existing final-scan shape). The existing assertions are unchanged
(non-answer prefix check, gate-stays-on check). A comment in the loop states the ordering
constraint.

Note: the closure lives INSIDE the test function — a file-scope helper with the same loop
tripped gocritic `rangeValCopy` (480-byte `Line` copies), which the in-test loops are exempt
from; the inline shape keeps lint at 0 without a nolint.

## Evidence

- `go test -race -count=1 ./internal/session/` — ok.
- `go test -race -count=1 ./...` — run TWICE consecutively on the final code: 26/26 packages
  ok, 0 failures both runs.
- `golangci-lint run` — 0 issues (baseline also 0, verified via `git stash`). `go vet ./...` —
  clean.

**Files:** `internal/session/planmode_test.go`, this task dir, `.planning/STATE.md`.
