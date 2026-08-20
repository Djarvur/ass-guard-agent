---
phase: 12-product-functional-completeness
plan: "03"
subsystem: parity
tags: [acp-07, d-04, extractor, delta-records, messageoffset, workspace-isolation, replay-harness]

requires:
  - phase: 12-product-functional-completeness
    provides: "12-05's BEFORE decomposition (36 turns / 12 empty-expectation with the then-current extractor) — the acceptance baseline this plan's fix is measured against"
provides:
  - "Delta-record-aware ExtractTurnsFromRollout: messagesKind=delta windows assemble by messageOffset arithmetic (full re-anchors, same-turn partial-block merge, truncate-and-place on occupied tails) into complete tool_use expectations — ZERO empty-expectation turns on delta-streamed corpora"
  - "ExtractTurnsFromRolloutDetailed: the diagnostics surface (turns/requests/delta/skips/emptyExpectation) — the WINDOWS #6 before/after decomposition instrument"
  - "Per-turn workspace isolation in the replay run path (RunSuite + WorkspaceArm + CapturedTurn.FixtureSnapshot): each turn replays into a fresh scratch seeded from its snapshot > the suite base > fresh empty — never a sibling's residue"
  - "The committed synthetic delta-records.jsonl fixture (deterministic offline ground truth; real rollouts rotate off disk — D-04)"
affects: [12-05 (re-pin evidence leg), 12-08 (eval machinery reuse), 14-03 CC-1 placement probe consumers]

actuals:
  tokens: 38000   # chars/4 over the plan's production commits (estimate was 48000)
  tasks: 2
  commits: 4

tech-stack:
  added: []
  patterns:
    - "Per-turn aggregation keyed by turnId: per-record emission was the duplicate/empty artifact carrier — a turn's trailing text-only response adds nothing instead of emitting an empty-expectation sibling"
    - "Expectations = assembled-assistant-tool_use backbone merged multiset-aware with response toolCalls (response views are per-request subsets; the merge never double-counts, and the final response's calls — which never enter any later window — append in order)"
    - "Prompt capture from the turn's OWN windows only (first record carrying the user message): a prompt reconstructed from the history tail alone could be a previous turn's — the fabricated-pairing class T-12-03-01 forbids"
    - "Interface-upgrade arm pattern: WorkspaceArm is an optional capability check on the arm; plain arms run unchanged (LiveArm = call-sequence comparison, no tool execution — isolation is structural, never a behavior change)"

key-files:
  created:
    - internal/parity/testdata/delta-records.jsonl
    - internal/parity/run_test.go
  modified:
    - internal/parity/replay.go
    - internal/parity/replay_test.go
    - internal/parity/run.go
    - internal/parity/harness.go (Harness.BaseWorkspace field — the one file beyond the plan's list; the seeding seam lives with the harness's run loop)

key-decisions:
  - "The empty-expectation mechanism decomposed as TWO carriers, both fixed at the extractor level: (1) delta records whose windows omit the user prompt (turn lost or mispaired) — fixed by window assembly + own-window prompt capture; (2) per-record emission duplicating a turn once per agentic-loop request, the final text-only response yielding an empty sibling — fixed by per-turn aggregation"
  - "Tool-use reconstruction prefers the assembled history backbone because the final request's response.toolCalls never appear in any later window — response-only union would drop them; history-only would drop nothing but requires the merge for dedup"
  - "Offset contradictions (out of bounds, role flip on a merging index, >4MiB per-message accumulation) skip with a counted warning (DeltaSkips in the report) — a corrupted capture degrades to fewer turns, never fabricated expectations (T-12-03-01/02)"

metrics:
  duration: ~50min
  completed: 2026-08-20

status: complete
---

# Phase 12 Plan 03: Parity extractor fixes — delta-records + per-turn workspace fixtures Summary

**One-liner:** The from-rollout A/B's two documented harness-artifact classes are dead: delta windows assemble by messageOffset into complete tool_use expectations (zero empty-expectation turns — verified over live current captures), and replays run in per-turn isolated scratches seeded from fixture snapshots.

## What Was Built

### Task 1 — delta-aware ExtractTurnsFromRollout (tracer, TDD)

RED first (`test(12-03)` 5eba609): the committed synthetic fixture reproduces the artifact class against the then-current extractor — every delta-streamed turn extracted EMPTY. GREEN (`feat(12-03)` d1aa739):

- **Window assembly** (`turnAccumulator`): `kind=full` records re-anchor the effective history; `kind=delta` records place their window at `messageOffset` — a window message landing on an index the same turn's stream already wrote MERGES by content-block append (partial-block streaming); any other occupied index truncates the tail first. Contradictions skip with a counted warning.
- **Per-turn aggregation**: one CapturedTurn per turnId; expectations = the turn's assembled assistant tool_use blocks (backbone, conversation order) merged multiset-aware with the response toolCalls across the turn's records.
- **`ExtractTurnsFromRolloutDetailed`**: turns + requests/delta/skips/emptyExpectation diagnostics; the exported `ExtractTurnsFromRollout` signature is unchanged.

**WINDOWS #6 acceptance (the zero-empty re-run):** the fixed extractor over the three live current captures in `~/.zcode/cli/rollout/` (incl. this executor's own live session): **7 turns / 0 empty-expectation / 1 counted skip** (evidence: `/tmp/eval-net-evidence/12-03-extract-decomp.txt`). The BEFORE records: the old algorithm simulated over the same files = 53 per-record turns / 6 empty; 12-05's fresh-capture record = 36 turns / 12 empty. The artifact class is dead at the extractor level.

### Task 2 — per-turn workspace isolation in the replay run path (TDD)

RED (`test(12-03)` 1a2b088) with pollution-detecting arms (outputs depend on scratch state). GREEN (`feat(12-03)` 6bc5844): `RunSuite` materializes a fresh scratch per CapturedTurn — seeded from the turn's `FixtureSnapshot` when recorded, else the suite's `BaseWorkspace`, else fresh-empty — hands it to workspace-aware arms via the optional `WorkspaceArm` interface, and tears it down after the turn. Seed sources are never mutated; the legacy no-config path is byte-identical behavior (the curated suite — Pitfall 18 honored: no threshold, baseline, or curated-path movement).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] harness.go modified beyond the plan's file list**
- **Found during:** Task 2
- **Issue:** The plan's `files_modified` lists run.go/run_test.go/replay(_test).go + the fixture, but the workspace-seam (which run needs to consult per turn) naturally lives on the Harness the run path drives.
- **Fix:** `Harness.BaseWorkspace` field + all isolation logic kept in run.go; harness.go gained the one field + doc comment. Existing harness tests green unmodified.
- **Files modified:** internal/parity/harness.go
- **Commit:** 6bc5844

**2. [Rule 1 - Bug] turn-reset gap in the accumulator refactor**
- **Found during:** Task 1 GREEN (caught by the test battery immediately)
- **Issue:** The closure-based materialize reset per-turn state; the first method-based refactor dropped the reset, cross-turn call leakage (14 turns from a 5-turn fixture).
- **Fix:** `maybeOpenTurn` resets prompt/calls after materializing a turn boundary.
- **Files modified:** internal/parity/replay.go
- **Commit:** d1aa739 (amended pre-push; the failing intermediate never left a green gate)

## Verification Evidence

- `mise ci` green (exit 0, all 26 packages — log preserved at `/tmp/eval-net-evidence/12-03-mise-ci.log`).
- `go test ./internal/parity/ -count=1` green twice consecutively (the determinism check).
- Existing curated-suite + harness tests green unmodified; `git diff --name-only HEAD~3` shows only the plan's files + harness.go.
- Live-capture decomposition evidence: `/tmp/eval-net-evidence/12-03-extract-decomp.txt` (WINDOWS #6's re-run, above).

## Threat-Model Mitigations Landed

- **T-12-03-01 (tampered delta records):** offset-bounds + role-flip validation skips records with a counted warning; prompt capture restricted to the turn's own windows (no stale-tail fabrication).
- **T-12-03-02 (adversarial captures):** 4 MiB per-message accumulation cap (counted skip); scanner buffer unchanged.
- **T-12-03-03 (silent threshold drift):** NO thresholds/baselines touched — the curated suite, drift-report record, and metric layers byte-identical (instrument-only scope, asserted by the untouched tests).
- **T-12-03-04 (committed fixture):** structure-only payloads — synthetic prompts/tool args, no session ids, no real capture text.

## Self-Check: PASSED

- internal/parity/testdata/delta-records.jsonl — FOUND
- internal/parity/run_test.go — FOUND
- Commits 5eba609 / d1aa739 / 1a2b088 / 6bc5844 — FOUND in git log
