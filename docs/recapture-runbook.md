# zcode Parity Re-capture Runbook (AUD-05, D-04)

The single operator procedure that re-grounds the within-session stability
test on a newly pinned, divergence-prone capture session. Written verbatim
from research Pitfalls 17/18 — do not paraphrase the thresholds or reorder the
drift-before-update step.

## 1. Purpose

Produce a newly pinned, DIVERGENCE-PRONE capture session re-grounding the
within-session stability test (AUD-05). NOT richest-session selection: the
pinned session must be able to FAIL the test (Pitfall 17 — richness-biased
picking optimizes for richness; a homogeneous session passes vacuously and
proves nothing).

## 2. Preconditions

(a) **FRESH ROLLOUT DIR** — archive existing sessions:

```
mkdir -p ~/.zcode/cli/rollout-archive && mv ~/.zcode/cli/rollout/model-io-sess_*.jsonl ~/.zcode/cli/rollout-archive/
```

or, if archiving is unacceptable, record the current newest filename and
consider ONLY sessions newer than it — NEVER scan the whole dir wholesale
(mixed zcode builds make sessions incomparable).

(b) **RECORD THE ZCODE VERSION FIRST**: run `zcode --version`, save the
verbatim output (feeds `-zcode-version`).

(c) Note the ass-guard git commit the extractor runs from (the drift report
records it).

## 3. The scripted divergence-prone workload — run ONCE, in real zcode, ONE session (D-04)

**REJECTION THRESHOLDS** — the capture is REJECTED unless the session shows:

- at least **5 user turns**,
- at least **10 distinct tools**,
- at least **1 subagent dispatch** (a subagent session file appears on disk),
- at least **1 mid-session tool-catalog change** (MCP attach AND detach).

Below-threshold captures are re-run, never relaxed.

- **Turn 1 (read/search variety)**: explore a small codebase question
  requiring Read + Grep or Glob + WebSearch or WebFetch.
- **Turn 2 (mutation variety)**: a code edit requiring Edit or Write + Bash
  (build/test) + Read to verify.
- **Turn 3 (subagent fan-out)**: dispatch a subagent task that itself uses
  2-3 tools; follow up referencing its result.
- **Turn 4 (MCP attach MID-SESSION — the catalog-change trigger)**: add an
  MCP server to the workspace config, then a turn that lists and invokes one
  of its tools.
- **Turn 5 (MCP detach + mixed finale)**: remove the MCP server, then a mixed
  turn (text + a couple of tool classes) so the session ends with a second
  catalog change.
- **Optional Turn 6**: a real SDD scenario via the toolkit's slash-commands
  when available (nice-to-have; not required for thresholds).

## 4. Pin + verify

Identify the new main session file (`model-io-sess_<uuid>.jsonl`, largest
main-role file created after step 2a). Verify the thresholds (count turns =
full-request lines, distinct tool names, subagent files, distinct tool-sets
across the session) and record the counts in the drift report. Sanity-run the
synthetic canary test to confirm the assertion CAN fail:

```
go test ./internal/profile/ -run TestStability_CanaryDetectsDivergence -v
```

## 5. Extraction — NO API key needed (do NOT gate this on ZAI_API_KEY)

(a) **Classify drift BEFORE any profile update** (Pitfall 18): with the OLD
profile still in place, run

```
go run ./cmd/ass-guard profile check zcode --capture-file <pinned session file>
```

and save the output as `profiles/zcode/drift-reports/<YYYY-MM-DD>-recapture.md`;
COMMIT the drift report BEFORE the profile-update commit (a profile update PR
without a drift-report artifact is a Pitfall 18 warning sign).

(b) Re-extract with the pinned session:

```
go run ./cmd/extract-profile -sessions <sessionID> -zcode-version "<verbatim version output>" -out profiles/zcode
```

The `-sessions` flag pins explicitly — the flag-less richest-main default is
FORBIDDEN for re-capture.

## 6. Re-baseline — the parity leg DOES need ZAI_API_KEY (separate gate from step 5)

(a) `go test ./internal/profile/ -run TestStability_WithinSessionExtractionSource -v`
MUST be green on the newly pinned session.

(b) Re-run A/B parity and record the FRESH numbers in the drift report —
thresholds are EXPLICITLY re-baselined, the Phase-1 numeric baseline is
SUPERSEDED, and parity numbers are never compared across baselines silently.

(c) Commit the profile update referencing the drift-report commit; append
session ID + both versions + threshold counts to the **Run record** section
below.

## 7. Run record

(filled per capture; newest first)

| date | session ID | zcode version | extractor version | turns / tools / subagents / catalog-changes | drift-report path |
|------|-----------|---------------|-------------------|---------------------------------------------|-------------------|
| 2026-08-16 | `sess_3cee56ae-cc6a-43a6-8f00-a08eb266e1aa` | 0.16.3 | extract-profile/01-02 + divergence-class fixes (`d3fcfa1`) | 13 recs / 81 tools / 1 subagent / 2 transitions (79→80 `mcp__recapture_probe`→79) | `profiles/zcode/drift-reports/2026-08-16-recapture.md` |

Run note (2026-08-16): the workload was driven autonomously via
`zcode.cjs app-server --stdio` (the internal line-delimited JSON-RPC; driver +
mechanism findings in the drift report) under the operator's explicit
direction — the §2–3 procedure's "operator runs the scripted workload" step
executed programmatically, and the mid-session attach/detach came from
config-change + `session/resume` boundaries (the only headless-reachable
mechanism; `/mcp` connect/disconnect are TUI-client-side).
