---
phase: 12-product-functional-completeness
plan: "05"
subsystem: capture
tags: [acp-07, d04-re-record, deferred-tools, forms-harvest, driver-kit, corpus-pinning, truncation, bash-timeout, ask-forms]

requires:
  - phase: 09-serve-path-audit-zcode-parity-re-capture
    provides: "the archived driver kit (quick task 260817-uv3) + the accepted UAT disposition's follow-up #1 (re-record curated expectations from current live zcode)"
provides:
  - "internal/coreexec/testdata/zcode-recaptured-2026-08.json — the committed re-record fixture: every deferred-tool family capture-pinned or honestly corpus_absent with the full hunt record (the D-04 primary route executed)"
  - "tools/zcode-recapture — the proven re-capture mechanism, hardened: kit-relative paths, config backup/restore with exit-handler diff-verify, workspace seeding, --tour/--only/--session modes, the forms harvester with D-03 redaction at extraction"
  - "Bash timeout + <persisted-output> truncation forms implemented from the capture (ACP-07's two flagship corpus-absent families resolved)"
  - "The captured answered-ask pairing form (RenderAskAnswered) — 12-01's interim form upgraded in place"
  - "ACP-02 capture evidence for 12-04: zcode's plan mode enforces a runtime-level mutating-tool gate (captured refusal forms) + the Agent tool result carries the agent_<uuid> SendMessage address"
affects: [12-03, 12-04, 12-06, 12-07, 13-openspec-workflow-completion]

actuals:
  tokens: 66000   # chars/4 over the plan's production commits (estimate was 84000)
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "v4 interaction answer shape: user-input-class interactions require {action:'accept',content:{answer|{}}} — anything else normalizes to decline/'Permission request failed'"
    - "Held-pending interaction = no self-timeout: the runtime's ask timeout does not fire while a client holds the request; auto-resolution is persisted-settings-gated (not headless-reachable)"
    - "Snapshot the rollout file IMMEDIATELY after each live pass — the rotation window is minutes-scale, not days (the D-04 class repeating live)"

key-files:
  created:
    - internal/coreexec/testdata/zcode-recaptured-2026-08.json
    - tools/zcode-recapture/harvest-deferred-forms.mjs
    - tools/zcode-recapture/harvest-deferred-forms.test.mjs
    - tools/zcode-recapture/scripted-workload.test.mjs
    - tools/zcode-recapture/testdata/synthetic-rollout.jsonl
    - tools/zcode-recapture/task1-freshness-2026-08-20.txt
    - tools/zcode-recapture/task3-tour-2026-08-20.txt
    - tools/zcode-recapture/task3-retry2-2026-08-20.txt
    - tools/zcode-recapture/task3-retry5-2026-08-20.txt
  modified:
    - tools/zcode-recapture/zcode-driver.mjs
    - tools/zcode-recapture/scripted-workload.mjs
    - tools/zcode-recapture/README.md
    - internal/coreexec/bash.go
    - internal/coreexec/bash_test.go
    - internal/coreexec/ask_test.go
    - internal/coreexec/testdata/zcode-interactive-results.json
    - internal/session/ask.go
    - internal/session/ask_test.go
    - cmd/ass-guard/ask_wiring_test.go

key-decisions:
  - "The re-record is PRIMARY and it ran LIVE: four capture passes against zcode 0.16.3 under the operator's logged-in plan; the qualifying session (38 recs / 81 tools / subagent / 79→80→79 probe timeline) doubles as the forms source"
  - "Honest corpus-absent dispositions with full hunts: cron quartet (plan-mode gate + the zcode 0.16.3 automation zod bug — CronList fails its own schema validation on EVERY headless call, 3 sessions), SendMessage/TaskStop acks (plan-mode refusals captured instead), ExitPlanMode approved form, ask non-answer (600s held-pending produced no self-timeout)"
  - "The truncation threshold (15000 chars) and preview (2000) are target-source-verified constants consistent with the capture (135.6KB persisted) — the exact boundary byte is not observation-bracketed and is flagged in the fixture"
  - "The primary rollout rotated off mid-harvest (D-04 class, live repeat) — the committed fixture is the pin; WINDOWS #7 records the immediate-snapshot rule for future harvests"

requirements-completed: [ACP-07]

duration: 214min
completed: 2026-08-20
status: complete
---

# Phase 12 Plan 05: Re-record from live zcode — the deferred-tool forms + ACP-07 families Summary

**The D-04 re-record route executed live: the archived driver kit re-proven against current zcode 0.16.3 (freshness run QUALIFIES), extended with a deferred-tools tour + a D-03-redacting forms harvester, then four capture passes produced the committed re-pinned fixture — every ACP-07 family either capture-implemented (Bash timeout form, the `<persisted-output>` truncation envelope, the answered-ask pairing form) or honestly corpus_absent with the hunt documented (cron quartet behind zcode's own runtime plan-mode gate + a reproducible automation zod bug).**

## Performance

- **Duration:** 214 min (live capture passes dominate: 5 runs × 4-12 min + harvest curation)
- **Tasks:** 3 (tracer + 2 TDD)
- **Commits:** 8ae2b2f (tracer), c03b461 (RED), 740003d (GREEN), 7165804 (mechanism fix), 1613295 (fixture + executor upgrades)
- **Files:** 20 (10 created, 10 modified)

## What the capture produced

**Capture metadata:** zcode 0.16.3 (CLI undrifted since the kit's record); primary session `sess_6e4b5cc7` (qualifying: 38 request records, 81 distinct tools, 1 subagent, 79→80→79 `mcp__recapture_probe` timeline); supplementary `sess_bc155bc5` (fresh-session retry). Config restore diff-verified EMPTY on every pass; ZERO automations ever created in the operator env (verified — no marker hits outside this agent's own artifacts).

**Family hits/misses table** (fixture: `internal/coreexec/testdata/zcode-recaptured-2026-08.json`):

| Family | Status | Disposition |
|---|---|---|
| Bash timeout form | CAPTURED (18 obs) | Implemented — `Command timed out after <d>\n<error>Command was aborted before completion</error>` |
| Bash truncation marker | CAPTURED (19 obs) | Implemented — `<persisted-output>` envelope, full output saved under `.ass-guard/outputs/`, 2KB preview |
| Bash background start | CAPTURED (32 obs) | Pinned for 12-06 (`Command running in background with ID: exec_<uuid>…`) |
| Bash default timeout | still corpus_absent | Only explicit timeouts observed; schema-declared 120000ms stands |
| Ask answered | CAPTURED (15 obs) | Implemented — pairing form `"<question>"="<reply>"… You can now continue…` |
| Ask non-answer | corpus_absent (hunt) | 600s held-pending: no self-timeout with a connected client; auto-resolution is persisted-settings-gated |
| Ask failure form | CAPTURED (39 obs) | `Permission request failed` (wrong client-answer shape) |
| EnterPlanMode | CAPTURED (43 obs) | Full entry guidance text |
| ExitPlanMode approved | corpus_absent (hunt) | Only error forms observed (`Permission denied` / `Permission request failed`) |
| SendMessage ack | corpus_absent (hunt) | Plan-mode refusal captured instead (43 obs) — ACP-02 evidence |
| TaskStop ack | corpus_absent (hunt) | Same refusal story (28 obs) |
| TaskOutput not_ready | CAPTURED (30 obs) | XML-tagged `<retrieval_status>` form — 12-06 pin |
| TaskOutput ready | corpus_absent | The blocking leg was plan-mode-blocked |
| ReadSessionContext lite | CAPTURED (17 obs) | `ReadSessionContext returned lite context for sess_…` |
| CronList | CAPTURED (45 obs) | The zod-error family (zcode 0.16.3 headless bug — NOT mimicked by ass-guard) |
| CronCreate/Update/Delete success | corpus_absent (hunt) | 4 attempts: plan-mode gate + automation backend bug |
| Read unchanged-dedup | CAPTURED (37 obs) | `Wasted call — file unchanged since your last Read…` — ROUTED with read-tracking (stateful) |
| run_in_background input | CAPTURED (first corpus usage) | The 08-08 "zero corpus usage" note resolves for this flag |

**Side-dirs salvage check (Task 1):** `~/.zcode/cli/{agents,artifacts,exec}/sess_3cee56ae*` hold only the old pin's ordinary machinery — 2 Edit-diff JSONs, 1 subagent transcript (Bash/find only), empty exec/. ZERO deferred-tool content; nothing folded (the fresh capture is the sole ground truth). Recorded in the kit README.

**Extractor decomposition (Behavior 4, BEFORE numbers):** the CURRENT `ExtractTurnsFromRollout` over the fresh primary capture: **36 turns / 12 empty-expectation** (vs the 2026-08-16 drift report's 6/13 empty + 4/13 pollution). The zero-empty acceptance is 12-03's (post-adoption — the adoption-line re-order inverted this plan's `depends_on: ["12-03"]`); recorded in WINDOWS #6 for 12-03 to re-run over the committed capture.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Kit freshness beyond protocol drift**
- **Found during:** Task 1 — `/tmp/zcode-recapture/` was already reaped: the probe-server path and the config-restore path inside scripted-workload.mjs were dead, and the restore lived in a `catch {}` that would SILENTLY leave the probe entry in the operator's config (a live T-12-05-01 hazard).
- **Fix:** kit-relative probe path; fresh config backup before any touch + exit-handler restore with diff-verify (non-zero exit on diff); workspace seeding; `--selftest`. The archived mechanism probes (hotreload/kill/resume) left verbatim (documented dead paths).
- **Commit:** 8ae2b2f

**2. [Rule 1 - Bug] The tour's interaction-answer shape was wrong (live discovery)**
- **Found during:** Task 3 — every ask/plan-approval interaction failed with `Permission request failed`: user-input-class interactions require the v4 answer shape `{action:'accept', content:{answer}}` (bundle-verified + live-proven); the generic auto-allow shape is invalid for them.
- **Fix:** driver `onUserInput` + per-leg prefs; corrected shape landed the answered-ask form on the retry. Related discoveries: the non-answer leg must HOLD the request pending (auto-allow defeated the timeout capture) — and even held, no self-timeout fires in 600s (honest corpus-absent).
- **Commit:** 7165804

**3. [Rule 3 - Blocking] Plan-mode poisoning of the capture session**
- **Found during:** Task 3 — the model re-enters plan mode habitually and zcode ENFORCES it at the tool-result level (`Plan mode only allows read-only, non-destructive tools` — CronCreate/SendMessage/TaskStop all refused). One failed ExitPlanMode approval leaves the session stuck.
- **Fix:** retries with hardened no-plan-mode prompts + a fresh session; the still-missing success forms dispositioned corpus_absent with the hunt documented. The capture itself is the finding: ACP-02's "plan mode is model self-restraint" premise is DISPROVEN by the target — 12-04 implements the captured gate.
- **Commits:** 7165804, 1613295

**4. [Scope note - documented] Files beyond files_modified**
- `zcode-driver.mjs` (prefs/env/onUserInput — the tour cannot be driven without it), the two `.test.mjs` self-test files + synthetic fixture (the plan's own verify runs `node --test ./`), the kit `.txt` run logs (evidence preservation), `internal/session/ask.go` + tests (the answered renderer lives in session — the single-definition rule from 12-01), `cmd/ass-guard/ask_wiring_test.go` (the re-pinned form's wiring assertion).

**5. [Deviation - dependency inversion] 12-05 ran BEFORE 12-03**
- The adoption-line re-order (2026-08-18, operator) puts 12-03 post-adoption, but this plan's Task 3 Behavior 4 consumes "12-03's FIXED extractor". The decomposition ran with the CURRENT extractor (before-numbers recorded); the zero-empty acceptance re-runs when 12-03 executes. WINDOWS #6.

**6. [Deviation - data loss] The primary rollout rotated off mid-harvest**
- `sess_6e4b5cc7`'s file disappeared from `~/.zcode/cli/rollout/` between harvests (minutes-scale rotation; the D-04 class repeating live). Its unique families are pinned VERBATIM in the committed fixture from the in-flight extraction records; the supplementary session was snapshotted to /tmp immediately and harvested mechanically. WINDOWS #7 records the immediate-snapshot rule.

## Auth gates

None — the driver authenticates via the operator's logged-in `~/.zcode/v2/config.json` provider (read by the driver itself; no ZAI_API_KEY needed for this plan's legs).

## Known Stubs

None — every shipped form is capture-pinned or a documented corpus-informed default with the hunt recorded (the fixture's corpus_absent list).

## Self-Check: PASSED

- internal/coreexec/testdata/zcode-recaptured-2026-08.json — FOUND
- tools/zcode-recapture/harvest-deferred-forms.mjs — FOUND
- Commits 8ae2b2f / c03b461 / 740003d / 7165804 / 1613295 — FOUND
- `mise ci` green (vet + lint 0 issues + build + full test battery incl. race)
