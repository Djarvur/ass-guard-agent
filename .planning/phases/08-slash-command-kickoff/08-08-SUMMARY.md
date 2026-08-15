---
phase: 08-slash-command-kickoff
plan: 08
subsystem: core-tool-execution
tags: [gap-closure, core-tools, bash, files, todo, capture-fixture, plain-text-rendering, e2e]
status: complete   # T1-T3 green (executors real + capture-grounded, guard green); T4 E2E run per the delegated approval — core-tool gap CLOSED (zero "no implementation" results, real artifacts on disk) but BOTH gated turns exhausted maxIterations on a NEW structural finding (D-11 per-mutating-call boundary reset defeats within-turn carry — recorded in STATE.md)

# Dependency graph
requires:
  - phase: 08-01..08-07
    provides: command discovery/expansion, real openspec tools, skills, web tools, engine chaining, mid-turn accumulation + tool-call identity, the gated E2E harness
provides:
  - "REAL execution for the six-tool /opsx working set (Bash, Read, Write, Edit, TodoWrite, TodoRead) with capture-grounded result forms — the 'no implementation yet' wall is dead for the E2E's flow"
  - "internal/coreexec: BashExecute (workdir, schema-declared 120000ms default + 600000 clamp, process-group kill + straggler reap, captured output/sentinel/'Exit code N' forms), ReadExecute (1-based N<TAB> rendering, offset/limit, captured missing-file error), WriteExecute (captured created/updated texts, parent mkdir), EditExecute (captured not-found form, structured not-unique, replace_all), TodoWriteExecute/TodoReadExecute (camelCase echo over the per-session store)"
  - "RegisterCore(catalog, Config{WorkDir, Todos}) — the single per-session registration site beside the Skill override (schema byte-identical, Execute-only override)"
  - "Projector plainContent: JSON-string tool-result Outputs render UNQUOTED (objects verbatim, IsError preserved, invalid-JSON falls back raw) — the captured plain-text forms actually reach the model"
  - "redact.walkRedact scrubs Bearer/sk- tokens embedded in string VALUES (LOG-03 at the new plain-text surface)"
  - "committed capture fixture internal/coreexec/testdata/zcode-core-results.json (sessions 4440f5a7 + 8a003655, harvest 2026-08-15, corpus_absent flags, late-harvest additions marked)"
affects: [08-06 T3/gate (still blocked — now on the D-11 finding), Phase 9 re-capture (corpus-absent forms to pin), every agentic turn's tool-result content]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "capture-grounded executors: every shipped form pinned by the committed fixture + conformance tests; every corpus-absent form an explicit structured-{'error':…} convention or documented default, flagged for the Phase-9 re-capture (never invented)"
    - "Execute-only catalog override at the per-session clone (the 08-05 Skill pattern generalized to six tools)"
    - "process-group kill + bounded kill-until-ESRCH reap loop (the fork-vs-kill straggler race, observed live as pid 68114)"

key-files:
  created:
    - internal/coreexec/bash.go
    - internal/coreexec/bash_test.go
    - internal/coreexec/files.go
    - internal/coreexec/files_test.go
    - internal/coreexec/todo.go
    - internal/coreexec/todo_test.go
    - internal/coreexec/fixtures_test.go
    - internal/coreexec/register.go
    - internal/coreexec/testdata/zcode-core-results.json
    - cmd/ass-guard/coreexec_wiring_test.go
  modified:
    - internal/session/projector.go (plainContent at the TypeToolResult mapping)
    - internal/session/projector_test.go (the plainContent battery beside 08-07's)
    - internal/redact/redact.go (string-value token scrub)
    - internal/redact/redact_test.go
    - cmd/ass-guard/acp_serve.go (RegisterCore beside the Skill override)

key-decisions:
  - "corpus grew between the plan's morning harvest and the fixture commit (the live session kept writing): forms the plan marked corpus-absent — Read missing-file, Edit not-found + read-tracking errors, Write-overwrite updated-form — were OBSERVED and are pinned as captured (lateHarvest-marked) instead of the structured convention"
  - "trailing-whitespace TRIM on Bash output is capture-pinned (0/107 observed results end in any whitespace) — 'echo hi' renders 'hi'"
  - "Edit/Write read-tracking enforcement (the observed 'File has not been read yet' / 'modified since read' forms) is deliberately NOT implemented (stateful cross-tool session state; out of gap-closure scope) — flagged in the fixture corpus_absent for the operator's disposition"
  - "TodoWrite echo ships as a JSON object output (wire-equivalent to the captured plain-text JSON: the model sees the same characters); plainContent passes objects verbatim"

patterns-established:
  - "fixture conformance tests string-compare executor outputs against the committed capture templates (literal + placeholder substitution)"

requirements-completed: [CMD-04-partial]   # the core-tool half of the blocker; convergence/scenario completion remains blocked on the NEW D-11 finding

# Metrics
duration: 150min
completed: 2026-08-15
---

# Phase 8 Plan 08: Capture-grounded core tool execution Summary

## What Was Built

The second blocking architectural gap's product half, closed with the same discipline as 08-07: real executors for the /opsx working set (Bash, Read, Write, Edit, TodoWrite, TodoRead), every result form pinned to the committed capture fixture, slotted into the existing catalog/DispatchBatch/RealExecutor seam with zero behavior change to shipped tools, plus the rendering rule that makes the captured plain-text forms actually reach the model.

## Task Log

- **T1 — the result contract** (RED `42c9f96` → GREEN `a3f95ff`): the capture fixture `internal/coreexec/testdata/zcode-core-results.json` (_provenance: sessions 4440f5a7 main + 8a003655 subagent, harvest 2026-08-15, D-03 redaction note, observed_counts, corpus_absent families) + `plainContent` in the Projector (JSON-string Output → unquoted text; objects verbatim via deep-equal — the redactor normalizes key order, noted; IsError preserved; invalid-JSON → raw fallback, unit-tested directly because a non-JSON RawMessage cannot pass Manager marshal).
- **T2 — the Bash executor** (RED `164098d` → GREEN `c021386`): real `sh -c` under the session workdir; model-supplied ms timeout (schema-declared default 120000, clamp 600000, fractional rounded); stdout+stderr captured (stdout before stderr, trailing-trimmed per capture); the sentinel and `Exit code <N>` forms with non-nil error (IsError); corpus-absent structured timeout form; `killGroupOnCtx` (08-03 idiom mirrored, not imported) + wiring `coreexec.RegisterCore` beside the Skill override. RED evidence: the live `no implementation yet` wall in the wiring test's transcript assertion.
- **T3 — the file + todo executors** (RED `0b78620` → GREEN `b4e4b2d`): Read/Write/Edit/TodoWrite/TodoRead in captured forms (details in key-decisions); RegisterCore completes the six-tool set; `reapGroup` added after a live fork-vs-kill straggler survived (pid 68114 — a same-group fork completing microseconds after the SIGKILL delivery never receives it; bounded kill-until-ESRCH loop closes it).
- **T4 — the gated E2E re-run** (per the operator's recorded approval, run autonomously with evidence preserved): see below — the core-tool gap is CLOSED, but BOTH gated turns exhausted maxIterations on a NEW structural finding (recorded in STATE.md; 08-06's remaining scope stays blocked).

## T4 Evidence (gated run 2026-08-15, real binary openspec 1.5.0 + real model)

Command: `ASSGUARD_OPENSPEC_BIN=1 ASSGUARD_E2E_LLM=1 go test ./cmd/ass-guard/ -run 'TestOpsxEndToEnd_Gated|TestOpsxFixableRecovery_Gated' -v -count=1 -timeout 40m` (total 1387s; log + transcripts preserved under /tmp/e2e-08-08-evidence/).

**The 08-08 acceptance bar's product half — MET, with live proof:**
1. **Zero `no implementation yet` results in both transcripts** (0 across 88+ Bash calls in the fixable probe; the string was grepped over every tool_result).
2. **Real work completed**: `openspec list --json` returned real JSON; the model drove a REAL archive through Bash mid-turn — `openspec archive fixable-probe` → `Change 'fixable-probe' archived as '2026-08-15-fixable-probe'` (EXIT 0) — and `openspec/changes/archive/2026-08-15-fixable-probe/` EXISTS in the preserved scratch.
3. Captured forms flowed: plain-text Bash outputs, `Exit code` errors with IsError, Skill objects — the plainContent seam worked live.

**Convergence — FAILED, both legs, same signature:** `session: tool loop exceeded max iterations` (EndToEnd explore turn, 640.9s; FixableRecovery probe turn, 744.7s; neither turn emitted a final assistant message — assistant_message count 0 in the fixable transcript).

**Diagnosis (evidence-backed, the NEW blocker in STATE.md):** every Bash result is immediately followed by a D-11 context boundary (42/42 in the fixable transcript; 75 boundaries for 77 results in the explore turn's live poll). The boundary resets the mid-turn window to the lean seed, so the model never sees its own prior exchanges within the turn — it re-starts from scratch every iteration (Skill `openspec-archive-change` re-read with the SAME input 60 consecutive times; 64 Skill calls / 2 unique; Bash 88 calls / 41 unique with longest identical run 4). The model COMPLETED real work mid-turn but could not see that it had, so it never ended its turn. 08-07's "the model sees its tool results and adapts" inference is corrected by this run: input variety was equally explainable by sampling temperature; with real execution the window-reset is proven load-bearing against convergence. Notable: the captured zcode corpus carries the last 64 messages of the conversation (the MidTurnWindowMessages source) — zcode does NOT reset on tool results, so D-11's per-mutating-call reset is itself a request-shape divergence from the mimicry target. Disposition needed from the operator (recommended: a gap-closure plan re-scoping D-11 to between-turn resets / capture-faithful within-turn carry — it changes a LOCKED safety invariant, so it is NOT done here).

**Handback state:** 08-06 T3 (pattern re-seed), the remaining UAT legs, and the phase gate remain BLOCKED on the new finding — not executed (the approval's chain was conditional on a genuine gate pass; the 08-06/08-07 no-hollow-greens precedent applies).

## Deviations

1. **internal/redact touched (1 file beyond the plan's list):** T1 Test 5 (redaction carry on string outputs) is unsatisfiable without string-value token scrubbing — the JSON walker only redacted secret-KEYED values, so `KEY=sk-…` in a plain-text tool result reached the model. Fixed at the chokepoint (completing the doc-comment's own belt-and-suspenders promise); token-free strings verbatim; focused test added.
2. **Late-harvest upgrades:** corpus-absent-planned forms observed and pinned as captured (Read missing-file, Edit not-found + read-tracking, Write-overwrite) — more grounded than the plan expected, each marked `lateHarvest` in the fixture.
3. **T2/T3 test-shape adjustments (in-plan intent, documented in-test):** Test 10's projection assertion became the D-11 boundary-discipline assertion (Bash is mutating → the post-boundary window carries no tool message — pinned by 08-07's own battery); the projected plainContent proof rides T3's read-only mixed batch. TodoStore landed in T2 (the plan's wiring site calls NewTodoStore there).
4. **Evidence preservation miss (EndToEnd leg):** the explore-turn scratch was reaped by t.TempDir before copying (the preserver loop was started after the fact, too late); its live-poll stats (154 calls: 150 Bash + 4 Skill; zero no-implementation; command samples incl. real `openspec list` output) are recorded above, and the failure line + duration are in the preserved run log. The FixableRecovery leg IS fully preserved (transcript + scratch incl. the real archive artifacts).

## Deferred Remainder (the plan's honest table, unchanged)

AskUserQuestion, EnterPlanMode/ExitPlanMode, Cron*, TaskStop, SendMessage/ReadSessionContext, Bash run_in_background + dangerouslyDisableSandbox — all schema-only, rationale in the plan's <deferred_scope>; plus read-tracking enforcement for Write/Edit (observed forms pinned in the fixture, deliberately unimplemented). None is required by CMD-04's gate; all recorded for the operator's next disposition.
