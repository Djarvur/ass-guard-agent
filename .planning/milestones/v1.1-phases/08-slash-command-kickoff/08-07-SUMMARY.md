---
phase: 08-slash-command-kickoff
plan: 07
subsystem: within-turn-conversation-assembly
tags: [gap-closure, tool-call-ids, mid-turn, projector, shaper, capture-fixture, convergence, e2e]
status: complete   # T1-T3 green; T4 E2E run autonomously per the overnight delegation — carry proven live, a DISTINCT downstream gap (core tool execution) blocks full scenario completion (recorded in STATE.md)

# Dependency graph
requires:
  - phase: 08-01..08-06
    provides: command discovery/expansion, real openspec tools, skills, web tools, engine chaining, the gated E2E harness
provides:
  - "provider.ToolCall carries the REAL provider id end-to-end (alias of shaper.ToolCall{ID,Name,Input})"
  - "shaper.Message structured mid-turn fields (ToolCalls/ToolCallID/ToolName/IsError) rendered to Anthropic tool_use/tool_result blocks (grouped, ordered) + OpenAI tool_calls/role:tool"
  - "Projector within-turn accumulation after the lean seed: batch-grouped assistant tool_use + tool-role results, pair-safe, boundary-safe, MidTurnWindowMessages=64 capture-pinned"
  - "committed capture-grounded golden fixture + pairing invariant + engine assistant-role-only regression + redaction-carry test"
affects: [08-06 T3/gate (needs converging real turns), Phase 9 AUD-05 (wire-render verification), every agentic turn's request shape]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "shaper-owned structured types with provider aliases (Message pattern extended to ToolCall) — one-directional edge preserved"
    - "capture-pinned magic numbers (MidTurnWindowMessages=64 ← observed zcode tail window) with provenance comments"
    - "pair-safe window tailing: drop only COMPLETE exchange groups (never separate tool_use from its results)"

key-files:
  created:
    - internal/shaper/midturn_test.go
    - internal/shaper/testdata/zcode-midturn-messages.json
  modified:
    - internal/shaper/shaper.go
    - internal/provider/provider.go
    - internal/provider/streaming.go
    - internal/provider/openai.go
    - internal/provider/anthropic_test.go
    - internal/provider/openai_test.go
    - internal/provider/conformance_test.go
    - internal/session/projector.go
    - internal/session/session.go
    - internal/session/session_test.go
    - cmd/ass-guard/acp_serve_test.go
    - cmd/ass-guard/e2e_opsx_test.go (evidence wiring only — see Deviations)

key-decisions:
  - "mid-conversation system messages (present in the captured fixture) render as user-role text blocks on the wire — the Anthropic Messages API has no mid-conversation system role; claude-code-compat convention rides the content through the user role"
  - "fixture json keys stay camelCase verbatim (D-03: the capture's keys are the ground truth) — tagliatelle nolint with rationale instead of renaming"
  - "orphaned tool_results are DROPPED from the projection (pair-safety) — a result without its call never reaches the model mispaired"

patterns-established:
  - "golden fixture from live rollout records: redact values, preserve keys/roles/shapes/array lengths, header names source session + harvest date + corpus limitations"

requirements-completed: [CMD-04-partial]   # the convergence infrastructure this plan owed; the scenario completion itself remains 08-06's (now blocked downstream — see STATE.md)

# Metrics
duration: 195min   # two executor sessions (the first died mid-T3 on a provider usage limit; this run completed T3 + T4)
completed: 2026-08-15
---

# Phase 8 Plan 07: Within-turn tool-result carry Summary

**The 08-06 blocker's four root causes are closed at unit scale and the carry is PROVEN LIVE against the real model — the model now sees its own tool calls and results mid-turn and demonstrably adapts to them. The full /opsx scenario still cannot complete: a DISTINCT, previously-masked product gap (core tool execution — Bash/Read/Write/Edit were never implemented; v1.0 shipped schema-only catalog entries) now surfaces as the turn's actual blocker. Recorded in STATE.md; NOT this plan's scope.**

## Performance

- **Duration:** ~195 min across two executor sessions (T1+T2 by the first; T3 remainder + T4 by this one)
- **Tasks:** T1 complete (RED `ad4cf62` → GREEN `1d93c4d`), T2 complete (RED `682671c` → GREEN `96a73ae`), T3 complete (RED/GREEN `57bd9c4` → `d4d39d6`), T4 run autonomously per the overnight delegation (evidence below; full scenario completion blocked downstream)

## Accomplishments

- **T1 — the seam** (`ad4cf62` RED → `1d93c4d` GREEN): `shaper.ToolCall{ID,Name,Input}` with `provider.ToolCall` as an alias (the Message pattern extended; provider imports shaper, never inverted); streaming + OpenAI parsing carry the real id (`call_abc123`/`call_oai_1` asserted); `shaper.Message` gains ToolCalls/ToolCallID/ToolName/IsError; Anthropic rendering via NewToolUseBlock/NewToolResultBlock with consecutive tool-role messages GROUPED into one user param; OpenAI rendering (tool_calls + role:tool); conformance pins `ToolResultMessage` ≡ the Shaper rendering (single-source); text-only shaping byte-identical (existing tests unmodified)
- **T2 — the loop** (`682671c` RED → `96a73ae` GREEN): `session.Prompt` records `AppendToolCall(turnID, tc.ID, …)` — the REAL provider id (the name-as-id bug fixed); Projector accumulates the current turn's exchanges after the UNCHANGED lean seed (anchor = later of {last boundary, the turn's user_message}; consecutive tool_calls fold into ONE assistant batch message; tool_results become tool-role messages with the name resolved from the paired call; orphaned results dropped); `MidTurnWindowMessages = 64` capture-pinned tail with complete-group-only drops; **the convergence test**: the fake model that needs its own tool result now returns end_turn within 3 iterations (RED reproduced the 08-06 finding at unit scale: max-iterations exhaustion + name-as-id); every existing projector/session test green unmodified
- **T3 — the guard** (`57bd9c4` RED → `d4d39d6` GREEN): committed fixture `internal/shaper/testdata/zcode-midturn-messages.json` (redacted excerpt of rollout session fb066d52's request.messages, harvest 2026-08-15, provenance header per D-03; corpus limitation — rollout records carry NO request.body.messages so wire rendering is protocol-canonical per PROV-02, wire verification rides Phase 9 AUD-05; tool-result CONTENT-format divergence flagged for Phase 9); golden shape pin (roles/batch order/grouping/tool_use_id==toolCallId/is_error, mid-conversation system role); pairing invariant over BOTH the fixture and a Projector-produced window; engine assistant-role-only regression (poisoned tool_result content triggers NOTHING); redaction-carry (secret in a tool result projects as [REDACTED]); full guard green: parity/profile/shaper/provider/session `-race` + `mise run ci` clean
- **T4 — the gated E2E, run autonomously** per the overnight delegation (operator entries 5f27c6e/7d13f0f): see below

## T4 Evidence (gated run 2026-08-15, real binary openspec 1.5.0 + real model)

Command: `ASSGUARD_OPENSPEC_BIN=1 ASSGUARD_E2E_LLM=1 go test ./cmd/ass-guard/ -run 'TestOpsxEndToEnd_Gated|TestOpsxFixableRecovery_Gated' -v -count=1 -timeout 40m`

**Result: TestOpsxEndToEnd_Gated FAILED after 570.93s with `tool loop exceeded max iterations` — but the failure MODE is new, and the within-turn carry is PROVEN LIVE:**

1. **The model now SEES its tool results and adapts.** The explore turn made 89 tool calls (88 Bash + 1 Skill) with **29 unique command inputs and a longest identical-input run of 2** — the model varied its attempts, switched tools (Bash → Skill), and returned to Skill after Bash errors. The pre-fix signature (08-06 diagnosis: identical fresh-window re-exploration forever) is gone. Every Bash result was the structured error `{"error":"tool Bash has no implementation yet (catalog schema authoritative)"}` — and the model's behavior changed in response to those errors, which is only possible if the results reach it on iteration N+1.
2. **Unit-scale convergence proof**: T2's convergence test (committed, green, `-race`) asserts exactly the acceptance property — iteration 2's outgoing messages include the assistant tool_use AND the tool result; end_turn within 3 iterations.
3. **The remaining failure is a DISTINCT product gap**: the zcode profile declares 103 tools (Bash, Read, Write, Edit, Grep, Glob, …) but ONLY openspec_* / Skill / WebSearch / WebFetch have Execute implementations (08-02/03/05). The /opsx stages need file reads and the openspec CLI via Bash — all unimplemented → the model cannot do the stage's real work → it exhausts 64 iterations trying alternatives. This was masked before 08-07 (the turn never converged long enough to matter) and is NOT in any approved plan. Recorded in STATE.md as the new blocking finding; full scenario completion (and 08-06's capture → re-seed → gate chain) awaits the operator's disposition (recommended: a gap-closure plan for core tool execution, capture-grounded result formats per the zcode transcripts — same discipline as this plan).
4. The fixable-recovery probe FAILED identically (`tool loop exceeded max iterations` after 700.37s; full run: both tests FAIL, 1272s total): Skill works (real openspec-archive-change skill content returned), Bash does not; the model interleaved 12 Skill + 15+ Bash attempts — again adaptation, not blindness. Same root cause, same disposition.

**Transcript evidence preserved:** polled copies of the scratch session transcripts (t.TempDir is ephemeral) at `/tmp/e2e-evidence/` (TestOpsxEndToEnd_Gated*/001/.ass-guard/transcript_sess-opsx-e2e.jsonl, 295 lines; TestOpsxFixableRecovery_Gated*/001/.ass-guard/transcript_sess-opsx-fixable.jsonl). NOTE: these runs predated the harness TranscriptWriter wiring (commit `5701d4d`), so they carry tool_call/tool_result/boundary lines (synchronous Manager writes) but no request_shaped lines; the NEXT gated run will carry request_shaped (the wiring exists now).

## Task Commits

1. **T1 RED** `ad4cf62` → **T1 GREEN** `1d93c4d` (tool-call identity + structured shapes through the seam)
2. **T2 RED** `682671c` → **T2 GREEN** `96a73ae` (projector accumulation + real ids in the loop)
3. **T3 RED/GREEN** `57bd9c4` (fixture + pairing invariant + engine regression, RED on the captured system role) → `d4d39d6` (system-role GREEN + lint clean)
4. **Evidence wiring** `5701d4d` (TranscriptWriter in the gated harness — deviation, below)

**Plan metadata:** this commit

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2] Harness evidence wiring (commit `5701d4d`)**
- **Issue:** the plan's T4 how-to-verify step 4 inspects `request_shaped` lines in the scratch transcript, but the runner never subscribes the async TranscriptWriter (LOG-02) — bus events with no subscriber DROP (production serve-path wiring is Phase 9 AUD-01..04 scope, plan 09-01 T2 wires it in sessionFor)
- **Fix:** test-file-only `startTranscriptWriter` (one writer per test; Publish fan-outs to every subscriber, so a second writer would duplicate lines); no assertion or product change
- **Committed in:** `5701d4d`

**2. [Rule 2] First executor died mid-T3 on a provider usage limit; this run completed the remainder**
- **Issue:** uncommitted lint cleanup + the shaper system-role GREEN fix were left in the working tree
- **Fix:** verified RED honestly (stash → `unsupported message role "system"` → restore → green), fixed 17 lint findings, committed as `d4d39d6`
- **Committed in:** `d4d39d6`

### Blocking (recorded, not worked around)

**3. Core tool execution missing — full E2E scenario completion blocked (distinct from this plan's closed gap)**
- **Found during:** T4's gated run — the model sees its results (carry works) but Bash/Read/Write/Edit have no Execute; the /opsx stages' real work is impossible
- **Status:** STATE.md Blockers (new entry); the recommended path (gap-closure plan for capture-grounded core tool execution) mirrors this plan's own discipline; NOT worked around — a fake Bash executor would be exactly the hollow green the gates exist to prevent
- **Consequence:** 08-06 T3 (capture → pattern re-seed needs COMPLETED stage outputs) and the phase gate remain blocked; the 08-06 blocker entry's carry half is RESOLVED, the scenario half is superseded by this new finding

## Issues Encountered

- The `internal/profile` stability test flaked once under `mise run ci` while this live zcode session wrote rollout logs (the pre-existing, STATE.md-documented flake); re-run green — no action

## TDD Gate Compliance

RED→GREEN per task: T1 `ad4cf62`→`1d93c4d`, T2 `682671c`→`96a73ae`, T3 `57bd9c4`→`d4d39d6` (RED verified by stash-replay this session). Guard: `go test ./internal/parity/ ./internal/profile/ ./internal/shaper/ ./internal/provider/ ./internal/session/ -race -count=1` green; `mise run ci` green (exit 0).

## Checkpoint: DELEGATED (overnight delegation 5f27c6e)

T4's operator witnessing was delegated for tonight: the gated E2E ran autonomously with evidence preserved (above + /tmp/e2e-evidence/). The morning disposition needed: (1) witness the carry evidence (this SUMMARY + the unit convergence test + the transcript copies); (2) disposition the NEW core-tool-execution gap (recommended: gap-closure plan); (3) 08-06's resume decision rides on it.

---
*Phase: 08-slash-command-kickoff*
*Completed: 2026-08-15 (overnight delegated run)*
