---
phase: 14-adoption-readiness-analysis-dispositions
fixed_at: 2026-08-19T21:10:48Z
review_path: .planning/phases/14-adoption-readiness-analysis-dispositions/14-REVIEW.md
iteration: 1
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 14: Code Review Fix Report

**Fixed at:** 2026-08-19T21:10:48Z
**Source review:** .planning/phases/14-adoption-readiness-analysis-dispositions/14-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 2 (Critical only — CR-01, CR-02)
- Fixed: 2
- Skipped: 0

## Fixed Issues

### CR-01: Restore of a non-latest checkpoint does not remove files created in later turns

**Files modified:** `internal/checkpoint/store.go`, `internal/checkpoint/store_test.go`
**Commits:** `483321a` (RED test), `0640fac` (fix)
**Applied fix:** `Restore` now runs `git checkout --no-overlay -f <ref> -- .` instead of the overlay
checkout. Overlay checkout never deletes index entries absent from the target tree, and files added
in later turns were staged into the shadow index by the next snapshot's `add -A`, so `clean -fd`
(untracked-only) could never remove them. `--no-overlay` deletes index+worktree entries the target
tree lacks; the follow-up `clean -fd` keeps sweeping genuinely untracked files, unchanged.

**TDD evidence:** `TestRestoreNonLatest_RemovesLaterTurnFiles` (snapshot A → create
`later.txt` + `laterdir/nested.txt` → snapshot B → restore A → assert workspace byte-identical to
A) failed pre-fix with exactly the two "created in a LATER turn survived restoring turn-001" errors;
green post-fix. Full `internal/checkpoint` package green both before and after (existing restore
tests unaffected by `--no-overlay`).

### CR-02: Subagent Task results with newlines/quotes bypass the truncation cap and produce invalid JSON

**Files modified:** `internal/session/session.go`, `internal/session/truncate_test.go`
**Commits:** `529b469` (RED test), `cd1c576` (fix)
**Applied fix:** The subagent result payload is now built by `json.Marshal(result)` (new helper
`subagentResultPayload`), replacing the raw `` `"`+result+`"` `` concat. Multi-line results
previously produced invalid JSON, which (a) failed `boundedToolResult`'s Unmarshal and passed
through unbounded, and (b) — observed through the real session path, worse than the review stated —
failed `appendLine`'s own `json.Marshal`, so the tool_result line was DROPPED from the transcript
entirely (a tool_call with no result). The fix keeps the review's defensive Marshal-error branch
(degrades to the error-payload form with `isError=true`): it is unreachable for a Go string, but
`errchkjson` requires the error checked. The helper is extracted from `runTurn` to keep that
function above the `maintidx` threshold.

**TDD evidence:** `TestSubagentMultilineOverCapResult_Bounded` (over-cap multi-line result with
newlines/quotes/backslashes must land as VALID JSON carrying the marker+tail bounded form) and
`TestSubagentMultilineUnderCapResult_RoundTripByteIdentical` (under-cap multi-line result reaches
the transcript byte-identical to its `json.Marshal` encoding) both failed pre-fix with
"no tool_result line for the Task call"; green post-fix.
`TestJSONMarshalEscapeFree_EqualsNaiveConcat` pins the equivalence claim (for escape-free strings —
every previously-valid payload — `json.Marshal` output is byte-identical to the old concat), so the
encoding switch changes nothing for previously-valid transcripts. Full `internal/session` package
green post-fix, including the pre-existing escape-free truncation fixtures.

## Verification

- `gofmt -l` on all touched packages: clean.
- `go test ./internal/checkpoint/... ./internal/session/... -count=1 -race`: ok (both packages).
- `mise run ci` (vet + golangci-lint + build + `go test -race -count=1 ./...`): all green.
- Verification ran in the MAIN CHECKOUT (`workflow.use_worktrees=false`; committed directly to
  `gsd/v1.1-acp-early-adoption` per orchestrator instruction) — results are reproducible from the
  branch as committed.

---

_Fixed: 2026-08-19T21:10:48Z_
_Fixer: ZCode (gsd-code-fixer)_
_Iteration: 1_
