---
phase: 12-product-functional-completeness
verified: 2026-08-20T17:05:00Z
status: human_needed
score: 8/9 must-haves verified
behavior_unverified: 0
overrides_applied: 0
deferred:
  - truth: "The eval gate's FIRST GREEN — the flagship suite (opsx-flagship) has never passed live; the opening runs caught pattern-table drift (3 runs, 3 chain shapes)"
    addressed_in: "Phase 13 (13-01 opener per the manager ruling 2026-08-20, folded into 13-01's scope; exit evidence = Phase 13's own gate sentence 'the extended eval suites green')"
    evidence: "Phase 13 SC-3 'The Phase-12 eval suites are extended … running in the same re-run gate' + Phase-13 phase gate 'AND the extended eval suites green in CI' cannot close without the flagship green; STATE.md Blockers entry names the fix route; WINDOWS #8 carries it open; evidence /tmp/eval-net-evidence/12-08-first-runs/ independently corroborated this verification"
human_verification:
  - test: "Confirm (retroactively) the manager ruling of 2026-08-20: the eval gate's first green is Phase 13's EXIT evidence, not Phase 12's — the capture-informed pattern re-tuning is routed to 13-01"
    expected: "An explicit operator confirmation recorded in STATE.md/ROADMAP Phase 13, OR a reversal routing the re-tuning back as a Phase-12 residual (small fix in-phase before close)"
    why_human: "The ruling was made under standing operator autonomy and is FLAGGED for retroactive confirmation; the Phase-12 written phase gate ('eval suites green in CI') is materially amended by it — an operator-scope decision, not a verifier call"
  - test: "Before Phase 13 executes: amend 13-01-PLAN.md (or 13-02) to name the 08-06 flagship stage-transition row re-tuning as in-scope"
    expected: "The checker-passed 13-01/13-02 plans (written 2026-08-19, BEFORE the 2026-08-20 drift finding) explicitly carry the flagship-row re-tuning + the eval re-baseline as opener work; otherwise the fix has no named plan owner and falls between phases"
    why_human: "Plan re-scoping is planner/manager territory; this verification may only record that the routing is currently absent from the Phase-13 plan artifacts on disk"
---

# Phase 12: Product Functional Completeness Verification Report

**Phase Goal:** As a developer driving ass-guard from an ACP editor, I want every tool in the captured catalog to execute for real with capture-pinned result forms, and turn behavior guarded by a behavioral-eval regression net, so that the product machinery is complete — the model never hits a `no implementation yet` dead end mid-task.
**Verified:** 2026-08-20T17:05:00Z
**Status:** human_needed — 8/9 criteria PASS; ACP-08 PARTIAL (machinery complete + real catches; first green deferred to Phase 13 under a manager ruling awaiting retroactive operator confirmation)
**Re-verification:** No — initial verification

## MVP Mode Note

ROADMAP marks this phase `Mode: mvp`. The goal is a well-formed user story (role/capability/outcome all present; "As a developer…" satisfies the canonical format). Verification proceeded on the nine ROADMAP Success Criteria as the contract, per-plan evidence beneath each.

## User Flow Coverage

User story: «As a developer driving ass-guard from an ACP editor, I want every tool in the captured catalog to execute for real with capture-pinned result forms, and turn behavior guarded by a behavioral-eval regression net, so that the product machinery is complete — the model never hits a `no implementation yet` dead end mid-task.»

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Model invokes any catalog tool | Every non-MCP catalog tool executes for real; the `no implementation yet` fallback (toolexec/real.go:85) is unreachable except via documented by-design routes (WebSearch/WebFetch backend routing, Agent pre-batch dispatch, Skill wiring-site closure) | TestBackgroundWiring_CoreCompleteness green this run (cmd/ass-guard/background_wiring_test.go:147 — iterates catalog.Names(), asserts Execute != nil for all non-MCP non-exception tools); grep: the string exists only as real.go forward-compat + a register.go comment calling it dead for openspec | ✓ |
| Model asks a question mid-task | Question surfaces to client in captured shape, turn suspends, reply lands as tool result | AskBroker (internal/session/ask.go, 388 lines) + RegisterAsk wired at acp_serve.go:1040-1046/1128; live operator witness APPROVED both legs (STATE.md RESOLVED 2026-08-19, sessions d9f98023/e253bfbd); TestAskWiring* green this run | ✓ |
| Scheduled prompt comes due | Fires as engine-driven turn while agent runs; no daemon/port | internal/sched (741 lines: store/parser/Due/ClaimForFire/CatchUp) + cron_wiring.go (startScheduler goroutine, queue-behind-active-turn, catch-up); no net.Listen in touched path; TestCronWiring* green this run | ✓ |
| Turn behavior changes land | A regression net gates profile/model/turn-behavior changes | evalharness (336 lines) + evalsuite (431 lines + tests + scenario) + mise eval-gate/eval-check-changed + detector selftest green this run; net RAN live 3x and CAUGHT real drift (evidence independently corroborated) | ✓ machinery / ✗ first green deferred (see ACP-08 verdict) |
| Outcome: no dead ends mid-task | Zero `no implementation yet` reachable from the built-in catalog | The FULL completeness gate is a permanent regression test; the cron exception dropped in 12-07; core pin 19→20 (CronUpdate joined, catalog_test.go:13) | ✓ |

## Goal Achievement

### Observable Truths (the 9 ROADMAP Success Criteria)

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | SC-1 AskUserQuestion end-to-end (ACP-01) | ✓ PASS | Executor (coreexec/ask.go 133 lines) + AskBroker suspension/resume (session/ask.go) + engine ActionAsk no-chain pin (no table lookup, learning-store bypass) + `--ask-timeout` D-01 flag (acp_serve.go:187, default 10m/0=block). Fixture zcode-interactive-results.json conformance + TestAskForms_ConformToFixture + TestDecide_AskSuspendedNeverChains + server-level TestAskWiring_ServerLevelSurface — all packages green this run. LIVE gate leg: operator witness APPROVED (STATE.md RESOLVED entry quotes both session transcripts verbatim; the witness's one finding — newline-frame drop — fixed 1b38e3d + WINDOWS #1/#2 closed by quick task 260819-nlg) |
| 2 | SC-2 EnterPlanMode/ExitPlanMode (ACP-02) | ✓ PASS | session/planmode.go (182 lines): PlanModeState + the CAPTURED runtime gate (`Plan mode only allows read-only, non-destructive tools`, const at :105) + captured extra-refused set; approval rides the ask seam (PendingAsk.Kind=plan_approval); plan_mode markers are own line type (never boundary lines); engine TurnOutput.PlanMode provenance. The 12-04 no-gating premise was superseded BY CAPTURE (12-05 proved the target enforces at tool-result level) — deviation documented in planmode.go scope note; Test 4 re-pinned to the captured behavior. RegisterInteractive wired (acp_serve.go:1053-1060) |
| 3 | SC-3 SendMessage + ReadSessionContext (ACP-03) | ✓ PASS | coreexec/messaging.go (419 lines): AgentMailbox (agent_<uuid> space, live deliver fns, byte-faithful delivery, honest queued-ack [documented corpus_absent], structured unknown-recipient errors) + SessionReader over .ass-guard/transcript_<sess>.jsonl (relevant/handoff strategies, maxTokens bound, captured lite-context head). Wired via RegisterInteractive (Mailbox + Sessions). Tests green this run |
| 4 | SC-4 Cron quartet + engine-driven firing (ACP-04) | ✓ PASS | internal/sched (741 lines: 0600 atomic store with corrupt-quarantine, hand-rolled 5-field local-wall-clock parser, Due, ClaimForFire exactly-once, CatchUp fire-once with missed-window notes) + coreexec/cron.go (278 lines, quartet; CronUpdate added to the embedded core, D-16 pin 20 with dated comment) + cron_wiring.go (258 lines: per-session turn mutex queue-behind-active-turn, serve-ctx scheduler goroutine 30s tick, catch-up on first active session, EngineDecision automation provenance, StartedBy runner-side exclusive). No daemon/no port (structural: no net.Listen; goroutine-exit-on-cancel pinned). TestCronWiring_QueueBehindActiveTurn/AuditProvenance/FireOnceCatchUp/SchedulerGoroutineLifecycle green this run. WINDOWS #3 closed (acp.Server.Emitter + session-lifetime forwarder; TestCronWiring_ServerDrivenMirror) |
| 5 | SC-5 TaskStop (ACP-05) | ✓ PASS | coreexec/background.go (445 lines): TaskRegistry (16-task cap, 1MB bounds, own process group — no ctx Cancel, registry owns lifecycle), Stop = group SIGKILL + reap loop, ReapAll chained into OnClose, deprecated shell_id alias, per-session scoping (TestBackgroundWiring_CrossSessionIsolation green). Stop-ack form = documented corpus_absent default with the hunt recorded in the fixture |
| 6 | SC-6 Bash run_in_background + dangerouslyDisableSandbox (ACP-06) | ✓ PASS | bash.go run_in_background branch renders the CAPTURED start form (32 obs — exec_<uuid> + live progressive log under .ass-guard/outputs/ + Read-guidance); TaskOutput renders the CAPTURED `<retrieval_status>not_ready</retrieval_status>` XML form (30 obs); dangerouslyDisableSandbox parsed deliberately as the documented no-op (bash.go:58-70). Foreground path byte-unchanged. Tests green this run |
| 7 | SC-7 corpus-absent forms re-pinned (ACP-07) | ✓ PASS | The D-04 re-record executed LIVE (4 capture passes, zcode 0.16.3, qualifying session sess_6e4b5cc7) → committed fixture zcode-recaptured-2026-08.json (215 lines; _provenance + 13 tool families; honest corpus_absent HUNT records for cron success forms / SendMessage ack / TaskStop ack / ExitPlanMode approved). IMPLEMENTED from the capture: Bash timeout form (bash.go:357), `<persisted-output>` truncation envelope (:188-210) with target-source-verified constants 15000/2000, answered-ask pairing form. 12-03's delta-aware ExtractTurnsFromRollout closed WINDOWS #6 with live evidence (7 turns / 0 empty-expectation over live current captures). Bash default timeout stays honestly corpus_absent (schema 120000ms stands). The re-record-primary route correctly replaced the rotated-off Phase-9 pin (D-04 fact) |
| 8 | SC-8 behavioral-eval regression net (ACP-08) | ⚠ PARTIAL — machinery PASS, first green DEFERRED to Phase 13 | The three-layer net EXISTS end-to-end and is wired: internal/evalharness (extracted Phase-8 machinery — AssertZeroContinue, Gates, BootstrapScratch, ScanFixableRecovery; the runtime gated tests re-pointed through it), internal/evalsuite (harness-owned assertion keys with schema unknown-key rejection [T-12-08-01], pass@k all-k + pass@1, JSON artifacts, loud gate skip), opsx-flagship.json scenario, bridge test, mise eval-gate (k=1, env triple, 15m) / eval-deep (k=3) / eval-check-changed, ci UNTOUCHED (D-03). Detector selftest green this run. The net RAN LIVE 3x and CAUGHT REAL drift — independently corroborated from /tmp/eval-net-evidence/12-08-first-runs/ (both artifacts: pass_at_k=false with distinct failure shapes; run-3 transcript shows the explore→propose provenance chain, the →apply text-pattern match, and the budget-cap stop at exactly 8 injections; safety pins held). Assertions untouched (Pitfall 18). THE FIRST GREEN IS BLOCKED: the 08-06 stage-transition patterns drifted vs the current model. Manager ruling 2026-08-20 routes the re-tuning to Phase 13 (see the dedicated assessment below) — FLAGGED for retroactive operator confirmation |
| 9 | SC-9 plugin-install discovery (ACP-10) | ✓ PASS | parseInstalledPlugins (v1 array + live v2 object, projectPath gating) + discoverInstalledPlugins (symlink-resolved containment, 1 MiB caps) in loader.go; doc.go documents the chain incl. the dropped ~/.zcode/cli/plugins/ root with the operator rationale; ALL FIVE contribution kinds merged (skills/, commands/, agents/ → SubagentTypes spawnable types wired acp_serve.go:1110; hooks/hooks.json → HookRunner at five lifecycle seams; .mcp.json → lowest MCP layer); write-boundary proof (content + mtimes); ecosys package green this run incl. the presence-gated live probe (8 real plugins on this machine). Landed pre-adoption 2026-08-18 |

**Score:** 8/9 criteria PASS (1 PARTIAL — machinery verified, first green deferred with explicit routing)

### The ACP-08 Disposition Assessment (the judgment item)

**Question:** does the net (machinery + live-run evidence + the fact that its opening runs CAUGHT real drift) satisfy "a behavioral-eval regression net exists and gates" with the first green routed to Phase 13 — or is this a gap that must block Phase 12's close?

**Verdict: the phase MAY close under the stated disposition, with two conditions.** Reasoning:

1. **The requirement's letter is met.** ACP-08 asks for a net that EXISTS and GATES — deterministic tool-unit tests (in every mise ci), scenario suites (exist; pass@k, real binary, scratch), and a re-run gate on profile/model/turn-behavior changes (detector + eval-gate + CI hook protocol). All three layers exist, are wired, tested offline, and selftest-green. Nothing in ACP-08's text requires the initial suite to be green at Phase-12 close.
2. **The phase-gate sentence IS materially unmet — and honestly so.** The ROADMAP phase gate reads "AND the eval suites green in CI." At close, the flagship suite has never passed (three live runs, three failure shapes). This is not papered over: WINDOWS #8 (unrun-verify) + #9 (deviation) open, STATE.md Blockers entry with the full diagnosis and fix route, evidence preserved. Assertions were NOT weakened (the askTimeout=45s config is D-01's documented hands-off mode, not an assertion change — verified at e2e_opsx_test.go:163).
3. **The deferral is structurally sound, not convenience.** Phase 13's own gate cannot close without the flagship green: OS-03 extends "the Phase-12 eval suites … in the same re-run gate," and the phase gate sentence requires "the extended eval suites green." The eval-gate mise task runs TestEvalSuite_Flagship_*. 13-02 edits the SAME pattern table (internal/openspec/seeded.toml) for the expanded matrix — one table-touch, one re-baseline is the right engineering shape. The net caught a real product defect and held its safety pins; a regression net whose first live firings are red-on-real-drift is a WORKING net, not a failed one.
4. **CONDITION 1 (the routing is not yet on paper).** The Phase-13 plans were checker-passed 2026-08-19 — BEFORE the drift finding (2026-08-20T13:29). 13-01-PLAN.md does not name the 08-06 flagship-row re-tuning; 13-02 derives NEW rows for the expanded matrix from NEW captures without re-tuning the existing flagship rows. STATE.md's blocker names the route as "a small Phase-12 residual OR the Phase-13 opener (operator decision)." The manager ruling picks the opener, but the disposition must be written into 13-01's scope before Phase 13 executes, or the fix has no named owner. Recorded as human-verification item 2.
5. **CONDITION 2 (retroactive operator confirmation).** The ruling amends a written phase gate under standing autonomy and is already FLAGGED for confirmation. Recorded as human-verification item 1. A reversal costs little: the fix is scoped as small (pattern-row re-tuning), and Phase 13 has not started.

**Not moved:** no thresholds, no assertion weakening, no gate bypass — the verifier confirms the failure is in the product's pattern table, not the net, and that the net reported it faithfully.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| internal/session/ask.go (388 lines) | AskBroker + D-01 timer + answered/non-answer renderers | ✓ VERIFIED | Wired (acp_serve.go:1040, :1128); tests green |
| internal/coreexec/ask.go (133 lines) | AskUserQuestion executor + single-line surface | ✓ VERIFIED | Fixture conformance test pins forms |
| internal/session/planmode.go (182 lines) | PlanModeState + captured runtime gate | ✓ VERIFIED | Captured refusal const :105; markers own line type |
| internal/coreexec/messaging.go (419 lines) | AgentMailbox + SessionReader | ✓ VERIFIED | Wired via RegisterInteractive |
| internal/coreexec/background.go (445 lines) | TaskRegistry + trio executors | ✓ VERIFIED | Registry-owned lifecycle; OnClose reaping |
| internal/coreexec/cron.go (278 lines) | Cron quartet | ✓ VERIFIED | CronUpdate in embedded core (pin 20) |
| internal/sched/sched.go (741 lines) | Store + parser + Due/ClaimForFire/CatchUp | ✓ VERIFIED | 0600 atomic; injectable clock; tests green |
| cmd/ass-guard/cron_wiring.go (258 lines) | Firing engine + scheduler goroutine + forwarder | ✓ VERIFIED | Queue/provenance/catch-up/lifecycle batteries green |
| internal/coreexec/testdata/zcode-recaptured-2026-08.json (215 lines) | The re-pinned fixture (13 families + provenance + hunts) | ✓ VERIFIED | Committed; corpus_absent hunts documented |
| internal/evalharness/harness.go (336 lines) | Extracted Phase-8 E2E machinery | ✓ VERIFIED | AssertZeroContinue + Gates + Bootstrap; runtime tests re-pointed |
| internal/evalsuite/suite.go (431 lines) + suite_test.go + scenarios/opsx-flagship.json | Scenario runner, pass@k, artifacts, schema rejection | ✓ VERIFIED | Offline battery green this run |
| cmd/ass-guard/evalsuite_bridge_test.go (100 lines) | The gated suite bridge | ✓ VERIFIED | Loud-skips ungated (named flags) |
| scripts/eval-change-class.sh (114 lines) | Change-class detector, exit-3 protocol | ✓ VERIFIED | --selftest green this run |
| internal/ecosys/loader.go + hooks.go + doc.go | Plugin discovery (5 kinds) + hook runner + documented chain | ✓ VERIFIED | Package green incl. live probe |
| .mise.toml eval tasks | eval-gate / eval-deep / eval-check-changed; ci untouched | ✓ VERIFIED | [tasks.ci] depends = vet/lint/build/test only |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| acp_serve.go sessionFor | RegisterCore/RegisterAsk/RegisterInteractive | Execute-only registration at the wiring site | ✓ WIRED | acp_serve.go:1028/1046/1056 — verified in source |
| sessionFor | sched.ScheduleStore | r.schedule (per-project store) | ✓ WIRED | :1059; nil-in-tests → structured no-store errors |
| AskBroker surface | ACP client | bus AgentMessageChunk → Run forwarder | ✓ WIRED | :1040-1045; server-level test through real acp.Server |
| Cron firing | engine-driven turn | ClaimForFire → runAutomationTurn under sessMu | ✓ WIRED | TestCronWiring_QueueBehindActiveTurn green |
| Server-driven turns | client | acp.Server.Emitter + session-lifetime forwarder (WINDOWS #3) | ✓ WIRED | TestCronWiring_ServerDrivenMirror green |
| Plugin agents | subagent dispatch | Session.SubagentTypes | ✓ WIRED | acp_serve.go:1110 |
| Plugin hooks | tool chokepoint | coreexec.ToolHooks + HookRunner | ✓ WIRED | hookRunner wraps every core executor (:1026-1029) |
| evalsuite scenarios | harness assertions | named keys only; schema rejects unknown | ✓ WIRED | T-12-08-01 test green |
| change-class detector | mise eval-gate | exit-3 protocol + MERGE_BASE CI hook | ✓ WIRED | Selftest green; hook documented in header |
| e2e runner D-01 config | askTimeout 45s | WINDOWS #9 documented config | ✓ WIRED | e2e_opsx_test.go:163 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| Cron executors | schedules | .ass-guard/schedule/schedules.json (real persisted store) | Yes | ✓ FLOWING |
| ReadSessionContext | excerpt | .ass-guard/transcript_<sess>.jsonl (real transcripts) | Yes | ✓ FLOWING |
| Background start form | exec_<uuid> + log path | real os.Process group + tee'd log | Yes | ✓ FLOWING |
| Result forms fixture | captured forms | live zcode 0.16.3 re-record (4 passes, qualifying session recorded) | Yes | ✓ FLOWING |
| Eval artifacts | pass/fail records | real suite runs → .ass-guard/eval/*.json (2 artifacts on disk) | Yes | ✓ FLOWING |
| Plugin discovery | registry entries | real installed_plugins.json v2 + cache layout (live probe: 8 plugins) | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| FULL catalog-completeness gate (zero dead ends) | go test ./cmd/ass-guard/ -run TestBackgroundWiring_CoreCompleteness | ok (PASS, 0.00s) | ✓ PASS |
| Cron queue/provenance/catch-up/lifecycle + ask/interactive/background wiring | go test ./cmd/ass-guard/ -run 'TestCronWiring\|TestAskWiring\|TestInteractiveWiring\|TestBackgroundWiring' | ok 5.551s | ✓ PASS |
| sched battery (store/parser/due/catch-up) | go test ./internal/sched/ -count=1 | ok 0.487s | ✓ PASS |
| evalsuite offline battery (pass@k arithmetic, schema rejection, loud skip, artifacts) | go test ./internal/evalsuite/ -count=1 | ok 7.867s | ✓ PASS |
| Executor/session/engine/parity batteries | go test ./internal/coreexec/ ./internal/session/ ./internal/engine/ ./internal/parity/ -count=1 | all ok | ✓ PASS |
| Plugin discovery + hooks | go test ./internal/ecosys/ -count=1 | ok 2.017s (incl. live probe) | ✓ PASS |
| Change-class detector selftest | ./scripts/eval-change-class.sh --selftest | selftest ok, exit 0 | ✓ PASS |
| Run-3 drift evidence (budget cap + chain provenance) | read /tmp/eval-net-evidence/12-08-first-runs/ transcript + artifacts | corroborated: explore→propose provenance decision, →apply text match, "re-fire budget cap reached (8 continue-injections)" at turn-009; both artifacts pass_at_k=false with distinct failure sets | ✓ PASS (evidence genuine) |
| Zero new dependencies | git diff 041a250..HEAD -- go.mod go.sum | empty | ✓ PASS |
| Full mise ci | (not re-run — independently certified green 2026-08-20, 28/28 packages, race battery) | certified | ✓ PASS (external certification) |

### Probe Execution

No `scripts/*/tests/probe-*.sh` probes declared; this phase's probe equivalents are the Go batteries above + the detector selftest. Step 7c: N/A.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| ACP-01 | 12-01 | AskUserQuestion end-to-end + live witness | ✓ SATISFIED | Witness APPROVED (STATE.md RESOLVED); all batteries green |
| ACP-02 | 12-04 | Plan pair, captured forms, state respected | ✓ SATISFIED | Captured runtime gate (capture-resolved scope deviation documented) |
| ACP-03 | 12-04 | SendMessage + ReadSessionContext | ✓ SATISFIED | messaging.go wired + tested |
| ACP-04 | 12-07 | Cron quartet + engine-driven firing | ✓ SATISFIED | sched + cron_wiring batteries green |
| ACP-05 | 12-06 | TaskStop real cancellation | ✓ SATISFIED | Group kill + reap + isolation tests green |
| ACP-06 | 12-06 | Bash background flags faithful | ✓ SATISFIED | Captured start/not_ready forms; no-op flag documented |
| ACP-07 | 12-05 + 12-03 | Corpus-absent forms re-pinned + implemented | ✓ SATISFIED | Committed fixture + implemented families + extractor fix (WINDOWS #6 closed) |
| ACP-08 | 12-08 | Behavioral-eval regression net exists and gates | ⚠ PARTIAL | Machinery complete + live catches; first green deferred to Phase 13 (manager ruling, flagged) |
| ACP-10 | 12-02 | Plugin-install discovery, five kinds, both roots | ✓ SATISFIED | Live probe (8 plugins); all five kinds wired |

Orphaned requirements: none — REQUIREMENTS maps exactly ACP-01..08 + ACP-10 to Phase 12; each claimed by exactly one plan (ACP-07 by 12-05+12-03 jointly, per the D-04 split). Note: REQUIREMENTS.md status cells for ACP-02..08 still read "Pending" and ROADMAP still shows "Plans: 2/8" with 12-03..12-08 unchecked — stale close-bookkeeping for the manager (all eight summaries exist and the work verified on disk).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (all 16 phase production/test files scanned) | — | TBD/FIXME/XXX/TODO/placeholder | — | NONE FOUND (clean) |
| internal/toolexec/real.go | 85 | The `no implementation yet` fallback string remains | ℹ️ Info | Forward-compat branch for future schema-only tools; the completeness gate proves no current catalog tool reaches it — this IS the grep-gate's target, neutralized by Execute coverage |
| .planning bookkeeping | — | ROADMAP "2/8 plans" + unchecked 12-03..08 boxes; REQUIREMENTS ACP-02..08 "Pending" | ℹ️ Info | Stale, not missing — manager close bookkeeping (out of verifier scope by instruction) |
| WINDOWS ledger | — | open_count=4 (#5 cache probe, #7 rollout-snapshot rule, #8 eval first green, #9 askTimeout deviation) | ⚠️ Info | All four correctly recorded with routing; #5/#7 pre-existing by prior disposition; /gsd-ship blocks while open — expected behavior of the ledger |

### Human Verification Required

### 1. Retroactive confirmation of the first-green disposition (closes the ACP-08 PARTIAL)

**Test:** Confirm or reverse the manager ruling of 2026-08-20: the eval gate's first green is Phase 13's EXIT evidence; the capture-informed 08-06 pattern re-tuning is 13-01 opener work.
**Expected:** Explicit operator confirmation recorded (STATE.md / ROADMAP Phase 13 note), or a reversal executing the small re-tuning as a Phase-12 residual before close.
**Why human:** The ruling amends the written Phase-12 phase gate ("eval suites green in CI") under standing autonomy and is already FLAGGED for confirmation. The verifier judges the routing structurally sound (Phase 13's gate cannot close without the flagship green) but the amendment is an operator-scope decision.

### 2. Write the routing into Phase 13's plan scope before it executes

**Test:** Amend 13-01-PLAN.md (or 13-02) to name the flagship stage-transition row re-tuning + eval re-baseline as opener work.
**Expected:** The checker-passed Phase-13 plans (written pre-drift) carry the disposition explicitly; the fix has a named plan owner.
**Why human:** Plan re-scoping is planner/manager territory; this verification records only that the routing is currently absent from the Phase-13 plan artifacts on disk (verified by grep — 13-01/13-02 cover NEW matrix rows, not the 08-06 flagship rows).

### Gaps Summary

No missing artifacts, no stubs, no unwired links, no orphaned requirements, no debt markers, zero new dependencies. Eight of nine success criteria fully verified on disk with green behavioral evidence re-run by this verifier (not trusted from SUMMARYs). The completeness gate — the phase goal's core promise ("the model never hits a `no implementation yet` dead end") — is a permanent regression test and passes.

The single open item is the eval net's FIRST GREEN (ACP-08): the machinery is complete, wired, selftest-green, and demonstrably working (its opening live runs caught real pattern-table drift; safety pins held; assertions untouched; evidence independently corroborated from /tmp/eval-net-evidence/12-08-first-runs/). Under the manager ruling the re-tuning rides Phase 13's same-table work with the first green as Phase 13's exit evidence. This verifier judges the phase goal — product machinery complete, turn behavior guarded by the net — ACHIEVED, with the first-green deferral sound but conditional on (1) the operator's retroactive confirmation and (2) writing the routing into 13-01's scope before Phase 13 executes. Status is therefore human_needed, not passed.

**Overall verdict: PHASE 12 MAY CLOSE under the stated disposition** — 8/9 criteria PASS, ACP-08 PARTIAL (deferred with evidence), pending the two recorded human items.

---

_Verified: 2026-08-20T17:05:00Z_
_Verifier: ZCode (gsd-verifier)_
