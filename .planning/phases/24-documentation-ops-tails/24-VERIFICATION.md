---
phase: 24-documentation-ops-tails
verified: 2026-09-10T19:12:04Z
status: human_needed
score: 13/14 must-haves verified
covered_files:
  - .planning/phases/24-documentation-ops-tails/24-01-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-02-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-03-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-04-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-05-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-06-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-01-SUMMARY.md
  - .planning/phases/24-documentation-ops-tails/24-02-SUMMARY.md
  - .planning/phases/24-documentation-ops-tails/24-03-SUMMARY.md
  - .planning/phases/24-documentation-ops-tails/24-04-SUMMARY.md
  - .planning/phases/24-documentation-ops-tails/24-05-SUMMARY.md
  - .planning/phases/24-documentation-ops-tails/24-06-SUMMARY.md
  - .planning/REQUIREMENTS.md
  - .planning/WINDOWS.md
  - .planning/phases/24-documentation-ops-tails/24-REVIEW.md
  - .planning/phases/24-documentation-ops-tails/24-USER-SETUP.md
  - .planning/phases/24-documentation-ops-tails/deferred-items.md
  - internal/modelrouting/outcomes.go
  - internal/modelrouting/outcomes_agg.go
  - internal/modelrouting/outcomes_test.go
  - internal/modelrouting/dispatch.go
  - internal/session/session.go
  - internal/session/subagent.go
  - internal/session/session_outcomes_test.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/acp_serve.go
  - internal/acpserve/config_surface_outcomes_test.go
  - internal/runtime/runtime.go
  - internal/runtime/apply_model_test.go
  - internal/modelroutingcmd/modelrouting.go
  - cmd/ass-guard/modelrouting.go
  - cmd/ass-guard/modelrouting_test.go
  - docs/lsp-setup.md
  - README.md
  - internal/profilecheckcmd/nightly_check.go
  - internal/profilecheckcmd/nightly_check_test.go
  - cmd/ass-guard/nightly_check.go
  - profiles/zcode/structure-pin.json
  - .github/workflows/nightly-parity.yml
  - .mise.toml
  - internal/modesmatrix/matrix.go
  - internal/modesmatrix/matrix_test.go
  - internal/acpserve/modesmatrix_interactive_test.go
  - internal/session/modesmatrix_subagent_test.go
  - internal/runtime/modesmatrix_cron_test.go
  - internal/runtime/modesmatrix_wake_test.go
  - internal/ecosys/testdata/modes-matrix/plugin.json
  - internal/ecosys/testdata/modes-matrix/hooks/hooks.json
  - internal/ecosys/testdata/modes-matrix/commands/matrix-echo.md
  - internal/ecosys/testdata/modes-matrix/skills/matrix-skill/SKILL.md
  - .golangci.yml
covered_digest: "v1:sha256:095c32af008e71874f4b5a8d9b35c1d40a2b1effaf358e6f26d5be18e8c1fa47"
behavior_unverified: 1 # truths present + wired but the runtime behavior is not exercisable pre-merge
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 11/14
  gaps_closed:
    - "G-24-1: SC-4 wake mode row — three real drivers through the live phase-22 wake chain, zero PRECONDITION-UNMET; false premise corrected in test file, 24-05-SUMMARY, WINDOWS #29 (commits cba127a, cde0b80, d3623dc)"
    - "G-24-2: CR-01/WR-01 cross-provider wrong-wire demotion — provider-filtered walks at BOTH resolve sites with the cross-provider-PRIMARY guard, pinned red-then-green at both sites (commits 3d95dc5, f926747)"
  gaps_remaining: []
  regressions: []
behavior_unverified_items:
  - truth: "Nightly CI runs the upstream-parity gate unattended on a schedule and reports drift without human triggering (SC-3 live half)"
    test: "After the v1.2 milestone merges to master: confirm the Actions tab shows nightly-parity scheduled runs firing at cron 41 3 * * *; run the workflow_dispatch smoke and confirm build-test green + drift-job report artifact; register the [self-hosted, zcode] runner; confirm Workflow permissions allow GITHUB_TOKEN issue creation"
    expected: "Scheduled runs appear unattended; dispatch run shows build-test green and a report artifact on every run; drift job starts once a matching runner is online; a drift opens an issue"
    why_human: "The schedule fires only when the workflow file exists on the DEFAULT branch (GitHub behavior — the plan's documented activation point); dispatch was 404-rejected pre-merge (both gh workflow run --ref and the REST dispatches API, recorded in 24-04-SUMMARY) per the plan's own Task 3 contingency (DEFERRED-TO-MERGE with local stand-in evidence); zero self-hosted runners are registered"
human_verification:
  - test: "Open Zed against the scratch project from docs/lsp-setup.md, prompt the agent to call an mcp__gopls__* tool (24-03 step 7 live editor leg)"
    expected: "The gopls MCP tool executes inside the editor-hosted session (context server configured IDE-side per the guide)"
    why_human: "GUI editor behavior; the scripted catalog dry-run (six mcp__gopls__* tools observed against the real binary, 24-03-SUMMARY) covered everything programmatically checkable"
  - test: "After merge to master: confirm nightly-parity scheduled runs fire unattended; run the dispatch smoke; register the [self-hosted, zcode] runner; confirm issue-on-drift permissions (the behavior_unverified_items entry; 24-USER-SETUP.md tracks all four)"
    expected: "Scheduled runs appear without human triggering; dispatch run green with report artifact; drift job runs once a runner is online; a drift opens an issue"
    why_human: "Schedule activation is structurally a default-branch property; pre-merge dispatch is 404-rejected by GitHub (recorded twice in 24-04-SUMMARY) — not locally exercisable"
  - test: "D-14 real-plugin spot-check fidelity: compare the superpowers 6.1.1 observations recorded in 24-05-SUMMARY against your own Claude Code setup"
    expected: "Plugins/skills behave unchanged vs the operator's own Claude Code environment (the plan's own human-checkpoint item)"
    why_human: "The 'unchanged' judgment is the operator's call against their live environment; grep cannot see behavioral fidelity"
---

# Phase 24: Documentation & Ops Tails Verification Report

**Phase Goal:** Small independent tails close out: LSP documented as an IDE-side MCP configuration requirement (no agent-side implementation), the scheduler gains its deterministic outcome store + feedback loop, nightly upstream-parity CI automation runs unattended, and plugins/skills prove working unchanged in every interaction mode (ECOS-04 end-to-end).
**Verified:** 2026-09-10T19:12:04Z
**Status:** human_needed
**Re-verification:** Yes — after gap closure (24-06, commits cba127a..d3623dc)

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC-1/DOC-01: guide exists, honest three-leg Zed surface, README-linked, dry-run executed with recorded evidence | ✓ VERIFIED | docs/lsp-setup.md (399 lines, regression: present, untouched by gap closure); README link; dry-run evidence in 24-03-SUMMARY (initial verification, re-confirmed) |
| 2 | 24-03 prohibitions: no forwarded-mcpServers consumption claim; no agent-side LSP promise | ✓ VERIFIED | Initial verification grep evidence stands; no phase file touched the doc since |
| 3 | SC-2a/TAIL-01: outcomes stored deterministically, zero LLM calls, mechanical schema only | ✓ VERIFIED (regression) | `go test -race ./internal/modelrouting/ -run 'TestOutcome|TestFirstAllowed'` ok (re-run this verification) |
| 4 | 24-01 seam replay: seeded transients open breakers, ok recovers, structural/exhausted never feed, cost degrade fires, FirstAllowed demotes | ✓ VERIFIED (regression) | Same battery, ok under -race (re-run) |
| 5 | 24-02 live recording: one record per completed attempt (turn + subagent), store failure never fails a turn | ✓ VERIFIED (regression) | `go test -race ./internal/session/ -run TestSessionOutcomeRecording` ok (re-run) |
| 6 | 24-02 stats CLI: human→stderr (stdout byte-clean), --json→stdout only, missing store exit 0 | ✓ VERIFIED (regression) | `go test ./cmd/ass-guard/ -run TestSchedulingStats` ok (re-run); initial live-binary run stands |
| 7 | SC-2b/D-06: replayed breaker evidence bends BOTH live Resolve sites — and the demoted target is usable on the session provider's wire (CR-01 closed) | ✓ VERIFIED | resolveSubagentModel walk provider-filtered (runtime.go:3185-3209: chain = primary + only `fb.Provider == sessionProvider` fallbacks; :3161 primary guard intact; fully-denied same-provider chain keeps primary + provider-constraint note); demoteIfDeniedLocked same-filtered with the cross-provider-PRIMARY guard (config_surface.go:787 returns byte-identical before any walk) — re-run green this verification: TestResolveSubagentModelCrossProviderDemotion (2 rows), TestOutcomeBreakersCrossProviderDemotion (2 rows), TestOutcomeBreakersExcludedPrimaryCorner (pin), plus the pre-existing same-provider rows (TestResolveSubagentModelBreakerDemotion / TestOutcomeBreakersDemotesSessionModel / EmptyMapKeepsPrimary) all ok under -race |
| 8 | 24-02: with an empty/absent store, resolution output is byte-identical to pre-change behavior | ✓ VERIFIED (regression) | TestOutcomeBreakersEmptyMapKeepsPrimary ok (re-run); empty-map early return at config_surface.go:780 |
| 9 | SC-3 core/TAIL-02: offline drift core (probe seam + sha256 vs committed pin), exit 0/7/1 contract, report always written | ✓ VERIFIED (regression) | `go test -race ./internal/profilecheckcmd/ -run TestNightlyCheck` ok (re-run); initial live exit-7 run vs real bundle stands |
| 10 | 24-04 workflow contract: off-peak cron + workflow_dispatch, least-privilege perms, artifact-always, issue body from report FILE, eval-gate untouched | ✓ VERIFIED (regression) | .github/workflows/nightly-parity.yml untouched since 11e9cfe (24-04) — the initial static verification stands unchanged |
| 11 | SC-3 live half: nightly runs unattended on a schedule without human triggering | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Core + workflow + pin live-proven locally; the unattended run is structurally post-merge (schedule fires only from the default branch; dispatch 404-rejected twice, recorded per the plan's own Task 3 DEFERRED-TO-MERGE contingency; zero runners registered) — see behavior_unverified_items and Human Verification |
| 12 | SC-4 exercised modes/TAIL-03: plugins/skills FUNCTIONAL (not merely discovered) in interactive, subagent, automation x commands/skills/hooks | ✓ VERIFIED (regression) | TestModesMatrixInteractive, TestModesMatrixSubagent, TestModesMatrixCron* all ok under -race (re-run this verification) |
| 13 | SC-4 wake mode/TAIL-03: the wake row functionally proven through the REAL phase-22 wake chain | ✓ VERIFIED (G-24-1 closed) | modesmatrix_wake_test.go now carries three real drivers — TestModesMatrixWakeCommands/Skills/Hooks all PASS under -race (0.85s/0.82s/0.87s, re-run this verification) with ZERO PRECONDITION-UNMET lines (grep count 0 over the -v output). Each drives client turn → Task run_in_background (backgroundLaunch seam) → nested completion → Tracker.Complete → scheduleWakeDrain → wakeDrainChain → drainWakeNotifications → runOneTurn, and asserts: exactly ONE wake turn naming the completing dispatch (kind subagent + minted task id joined to the transcript's async_launched payload), one EngineDecision with wake provenance (bounded poll), first user message stays the typed prompt, the invocation rides the dispatch verbatim (commands/skills), the woken model's captured request carries the notification, and PreToolUse fires INSIDE the wake turn (hooks, marker stdin JSON tool_name Read). The two assertion-shape substitutions were verified against the code, not just the summary: renderWakeBlocks (cron_wiring.go:531) emits only machine-composed notification blocks (in-wake slash expansion structurally impossible), and SubagentStop's single fire-site is subagent.go:150 inside the foreground DispatchSubagent wrapper (the background leg calls the nested Runner.Run directly) — the substituted observables are the strongest TRUE outcomes, not weakened coverage |
| 14 | 24-05: matrix totality + honest loud-skip vocabulary (never a fabricated green) | ✓ VERIFIED (G-24-1 honesty closed) | False premise corrected everywhere it lived: corrected test-file comment (premise named FALSE, cites 24-VERIFICATION gap 1); zero SkipPrecondition / PLANNED-BUT-UNEXECUTED tokens under internal/; 24-05-SUMMARY carries the dated correction paragraph ("## Correction (2026-09-10, plan 24-06 — G-24-1)") restating BOTH substitutions, with the wake table row, artifact bullet, honesty bullet, readiness bullets, and WINDOWS note amended; WINDOWS entry #29 status=fixed with a resolution note naming 24-06 |

**Score:** 13/14 truths verified (1 present, behavior-unverified)

### Advisory (New Scope, Unevidenced)

New-scope findings from Step 7 with no deterministic evidence — reported, not blocking.

| # | Finding | Category | Why Advisory |
|---|---------|----------|--------------|
| — | None | — | No new-scope, unevidenced findings this round; go vet clean on both modified packages, zero debt markers (TBD/FIXME/XXX) in any gap-closure-modified file |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| internal/modelrouting/outcomes.go | Store + record schema | ✓ VERIFIED | 208 lines; append-only O_APPEND 0600, self-gitignore, tolerant ReadOutcomes |
| internal/modelrouting/outcomes_agg.go | Aggregate/Replay*/FirstAllowed | ✓ VERIFIED | 187 lines; ReplayCostTracker still ORPHANED live (WR-04, unchanged review debt) |
| internal/modelrouting/outcomes_test.go | 9-test RED→GREEN suite | ✓ VERIFIED | 484 lines, green under -race (re-run) |
| internal/session/session_outcomes_test.go | Live record battery | ✓ VERIFIED | 357 lines, green under -race (re-run) |
| internal/acpserve/config_surface_outcomes_test.go | Surface demotion tests | ✓ VERIFIED | Now 267 lines: prior battery + crossProviderHeavyLayer/excludedPrimaryLayer fixtures, crossProviderBreakers seeder, TestOutcomeBreakersCrossProviderDemotion + TestOutcomeBreakersExcludedPrimaryCorner — all green under -race (re-run) |
| internal/modelroutingcmd/modelrouting.go | Stats renderers | ✓ VERIFIED | LoadAndAggregate/BuildStatsRows/EmitStatsHuman/EmitStatsJSON; live CLI discipline verified initially |
| cmd/ass-guard/modelrouting.go | stats subcommand | ✓ VERIFIED | Registered in the model-routing group (:25) |
| docs/lsp-setup.md | Three-leg guide | ✓ VERIFIED | 399 lines, all structural keys, citations, honest status; untouched by gap closure |
| README.md | Docs index link | ✓ VERIFIED | Mid-sentence style, adjacent to install |
| internal/profilecheckcmd/nightly_check.go + test | Drift core | ✓ VERIFIED | Exit contract live-verified initially (0/7/1); 7-case battery green (re-run) |
| cmd/ass-guard/nightly_check.go | Cobra shell | ✓ VERIFIED | Dual-group registration (parity + profile) |
| profiles/zcode/structure-pin.json | Committed baseline | ✓ VERIFIED | 7 digests, zcode_version 0.16.3; 7/7 match live (initial verification) |
| .github/workflows/nightly-parity.yml | First Actions workflow | ✓ VERIFIED | Parses; full contract verified initially; file untouched since 11e9cfe |
| .mise.toml | [tasks.parity-nightly] | ✓ VERIFIED | Wraps the same build+check invocation |
| internal/modesmatrix/matrix.go (+test) | Cell registry/report/loud-skip | ✓ VERIFIED | 573+139 lines; totality gate unit-pinned |
| modesmatrix_{interactive,subagent,cron,wake} test files | 4 mode legs — ALL REAL | ✓ VERIFIED | Interactive/subagent/cron green (re-run); wake = three real drivers green under -race with zero precondition marks (G-24-1 closed) |
| internal/ecosys/testdata/modes-matrix/ | Fixture plugin | ✓ VERIFIED | plugin.json + hooks + command + skill; hermetic (no network/absolute paths) |
| 24-06-task1/task2-red-evidence.json | Gap-closure TDD RED records | ✓ VERIFIED | Present in the phase dir; task2's 4 red rows document the wrong-wire at HEAD pre-fix |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| session/subagent record sites | OutcomeStore.Append | recordDispatchOutcome | ✓ WIRED | session.go:700, subagent.go:215 (initial verification, unchanged) |
| acp_serve composition | ConfigSurface.SetOutcomeBreakers | ReplayBreakers over WorkDir store | ✓ WIRED | acp_serve.go:314, exactly once (initial verification, unchanged) |
| runtime SubagentModel population | resolveSubagentModel + replayed breakers | outcomeBreakersFor() | ✓ WIRED | runtime.go single production caller (initial verification, unchanged) |
| stats shell | modelroutingcmd renderers | AddCommand | ✓ WIRED | cmd/ass-guard/modelrouting.go:25 |
| nightly_check.go | paritycli version seam + pinned bundle | ZcodeInstalledVersion + sha256 | ✓ WIRED | Initial live run: 7/7 match |
| workflow drift job | report JSON | artifact + github-script fs.readFileSync | ✓ WIRED | if: always() + JS-composed issue body (initial verification, unchanged) |
| docs/lsp-setup.md | README docs index | relative link | ✓ WIRED | Present, existing style |
| ReplayCostTracker | CostTracker live seam | production consumer | ⚠️ ORPHANED | WR-04 unchanged review debt (out of 24-06 scope by design; documented, not gating) |
| wake cells | wake-turn machinery | real chain: backgroundLaunch → Tracker.Complete → scheduleWakeDrain → wakeDrainChain → drainWakeNotifications → runOneTurn | ✓ WIRED | G-24-1 closed: the three drivers execute the live chain end to end (green under -race) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| stats CLI | aggregate rows | .ass-guard/routing/outcomes.jsonl (ReadOutcomes) | Yes — live run printed seeded row exactly (initial) | ✓ FLOWING |
| demotion sites | breakers map | ReadOutcomes + ReplayBreakers over WorkDir store | Yes — seeded stores demote in tests; cross-provider rows prove the provider-filtered pick | ✓ FLOWING |
| session recording | DispatchOutcome | live streamErr/usage counters/latency | Yes — TestSessionOutcomeRecording asserts file contents | ✓ FLOWING |
| nightly-check | report JSON | bundle sha256 + meta.yaml + probe | Yes — live report written with real digests (initial) | ✓ FLOWING |
| wake cells | wake turn input | renderWakeBlocks over the tracker's real notification batch | Yes — the drivers assert the notification block's kind/task-id against the dispatch's minted id | ✓ FLOWING |
| LSP doc dry-run | tool catalog | real binary + mcp-language-server/gopls | Yes — summary evidence (six tools; initial) | ✓ FLOWING (author-run; editor leg = human) |

### Behavioral Spot-Checks

All commands re-run in THIS verification process (not from SUMMARY claims).

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Wake matrix trio (G-24-1) | `go test -race ./internal/runtime/ -run 'TestModesMatrixWake' -v` | 3/3 PASS (0.85s/0.82s/0.87s); `grep -c PRECONDITION-UNMET` over the -v output = **0** | ✓ PASS |
| Runtime cross-provider demotion (G-24-2) | `go test -race ./internal/runtime/ -run TestResolveSubagentModelCrossProviderDemotion -v` | PASS (both subtests: open primary skips allowed cross-provider candidate → glm-4.6; fully-denied same-provider chain keeps primary + note) | ✓ PASS |
| Surface cross-provider demotion + corner pin (G-24-2) | `go test -race ./internal/acpserve/ -run 'TestOutcomeBreakersCrossProviderDemotion|TestOutcomeBreakersExcludedPrimaryCorner' -v` | PASS (both subtests + the excluded-primary corner: byte-identical, zero breaker notes) | ✓ PASS |
| Same-provider demotion + wake substrate + cron row (regression) | `go test -race ./internal/runtime/ -run 'TestResolveSubagentModelBreakerDemotion|TestWakeTurn_BackgroundBashCompletion|TestModesMatrixCron'` | ok 2.720s | ✓ PASS |
| Surface demotion + interactive matrix (regression) | `go test -race ./internal/acpserve/ -run 'TestOutcomeBreakersDemotesSessionModel|TestOutcomeBreakersEmptyMapKeepsPrimary|TestModesMatrixInteractive'` | ok 2.532s | ✓ PASS |
| Outcome store + replay suite (regression) | `go test -race ./internal/modelrouting/ -run 'TestOutcome|TestFirstAllowed'` | ok 1.027s | ✓ PASS |
| Live recording + subagent matrix (regression) | `go test -race ./internal/session/ -run 'TestSessionOutcomeRecording|TestModesMatrixSubagent'` | ok 1.052s | ✓ PASS |
| Nightly drift core (regression) | `go test -race ./internal/profilecheckcmd/ -run TestNightlyCheck` | ok 1.016s | ✓ PASS |
| Stats CLI (regression) | `go test ./cmd/ass-guard/ -run TestSchedulingStats` | ok | ✓ PASS |
| Static analysis | `go vet ./internal/runtime/ ./internal/acpserve/` | clean | ✓ PASS |

### Probe Execution

Not applicable — this phase's plans declare no `scripts/*/tests/probe-*.sh` probes; verification evidence is the Go test batteries above plus the recorded dry-run/live-run artifacts in the SUMMARYs.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| DOC-01 | 24-03 | LSP documented as IDE-side MCP config requirement (no agent-side implementation) | ✓ SATISFIED | Truths 1-2; editor leg routed to Human Verification |
| TAIL-01 | 24-01 + 24-02 (+24-06) | Scheduler outcome store + feedback loop (deterministic, zero LLM) | ✓ SATISFIED | Truths 3-8; CR-01 defect FIXED and pinned by cross-provider rows at both sites (truth 7) |
| TAIL-02 | 24-04 | Nightly upstream-parity CI automation | ✓ SATISFIED at plan letter; unattended half = behavior-unverified (post-merge activation per the plan's own contingency) | Truths 9-11 |
| TAIL-03 | 24-05 (+24-06) | ECOS-04 — plugins/skills working unchanged in every interaction mode | ✓ SATISFIED | Truths 12-14: 12/12 cells functional through real drivers (wake row over the live phase-22 chain), zero precondition marks |

Orphaned requirements: none — REQUIREMENTS.md maps exactly DOC-01/TAIL-01/02/03 to Phase 24, all claimed by plans (now including the 24-06 gap-closure plan).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/modelrouting/outcomes_agg.go | 113 | ⚠️ WR-04: ReplayCostTracker has zero production callers — cost half of the feedback loop unwired (D-06 names cost-degrade as a seam) | ⚠️ Warning | Unchanged review debt, deliberately out of 24-06 scope; windowed spend does not survive restart |
| internal/session/session.go | 700, 1321-1340 | ⚠️ WR-02: context.Canceled (user Stop) classifies OutcomeTransient — feeds breaker evidence | ⚠️ Warning | Unchanged review debt; breaker/stats pollution by non-provider failures |
| internal/session/session.go | 1300 | ⚠️ WR-03: D-15 cross-provider subagent attempts recorded under the session provider — wrong breaker key + stats misattribution | ⚠️ Warning | Unchanged review debt |
| ~15 phase-24 files | — | ⚠️ WR-05: accidental 100755 exec bit | ⚠️ Warning | Unchanged review debt; diff noise |
| cmd/ass-guard/profile_check.go | 30 | ℹ️ WR-06: --zcode-bin parsed but never used (pre-existing) | ℹ️ Info | Silent flag no-op |
| internal/modelrouting/breaker.go | 94-114 | ℹ️ WR-07: half_open_probes knob never enforced (pre-existing) | ℹ️ Info | Unlimited concurrent half-open probes |
| various | — | ℹ️ IN-01..06 (see 24-REVIEW.md) | ℹ️ Info | Unchanged review debt |

Debt markers (TBD/FIXME/XXX): none in any phase-modified file (re-scanned this round over all five gap-closure-modified code files — zero matches). The two prior 🛑 Blockers (CR-01 unfiltered demotion walk; false-precondition wake skips) are both RESOLVED — verified in code and by the batteries above. Known pre-existing failures (TestPermissionsE2E, TestRescanConcurrency, evalsuite openspec env, coreexec load-flake) remain documented out-of-scope and reproduced in none of this verification's scoped runs.

### Human Verification Required

1. **LSP guide live editor leg (24-03 step 7)**
   **Test:** Open Zed against the scratch project from docs/lsp-setup.md; prompt the agent to call an `mcp__gopls__*` tool.
   **Expected:** The tool executes in the editor-hosted session (context server configured IDE-side per the guide).
   **Why human:** GUI editor behavior; the scripted catalog proof (six tools against the real binary) covered everything else.

2. **Nightly unattended activation (post-merge) — the behavior-unverified truth**
   **Test:** After the v1.2 milestone merges to master: confirm the Actions tab shows nightly-parity scheduled runs firing at cron 41 3 * * *; run the workflow_dispatch smoke; register the [self-hosted, zcode] runner; confirm Workflow permissions allow GITHUB_TOKEN issue creation (24-USER-SETUP.md tracks all four).
   **Expected:** Scheduled runs appear unattended; dispatch run shows build-test green + a report artifact; drift job starts once a runner is online; a drift opens an issue.
   **Why human:** Schedule activation is structurally a default-branch property; pre-merge dispatch is 404-rejected by GitHub (recorded twice in 24-04-SUMMARY per the plan's own DEFERRED-TO-MERGE contingency).

3. **D-14 real-plugin spot-check fidelity**
   **Test:** Compare the superpowers 6.1.1 observations recorded in 24-05-SUMMARY against your own Claude Code setup.
   **Expected:** Plugins/skills behave unchanged vs the operator's own environment.
   **Why human:** The "unchanged" judgment is the operator's call against their live environment.

### Gaps Summary

No gaps remain. Both prior gaps are closed with verifier-executed evidence:

1. **G-24-1 (wake row, TAIL-03) — CLOSED.** The three wake cells are real drivers over the live phase-22 chain (green under -race, zero PRECONDITION-UNMET lines — the prior count was 3). The two assertion-shape substitutions were verified against the code itself (renderWakeBlocks emits notification-only machine-composed blocks; SubagentStop's only fire-site is the foreground DispatchSubagent wrapper), so they are the strongest true functional outcomes rather than weakened coverage. The false premise was corrected in every artifact that carried it (test-file comment, 24-05-SUMMARY dated correction paragraph + amended claims, WINDOWS #29 fixed).
2. **G-24-2 (CR-01/WR-01 cross-provider wrong-wire, TAIL-01) — CLOSED.** Both demotion walks are provider-filtered (runtime: sessionProvider; surface: s.providerName behind a cross-provider-PRIMARY guard that never claims an unconsulted breaker), and the behavior is pinned red-then-green at both sites by rows that demonstrably fail against the unfiltered walk (the wrong-wire minimax-m3 pick) and by the excluded-primary corner pin.

Status is human_needed (Step 9 rule 2): every automated check passes and both gaps are closed, but three items require human verification — the nightly workflow's post-merge live dispatch (the one behavior-unverified truth, standing on complete local stand-in evidence per the plan's own contingency), the LSP guide's live-editor leg, and the D-14 operator judgment. None is a code gap.

Warnings worth scheduling (not gating, unchanged from the initial verification): ReplayCostTracker dead code (WR-04), cancellation-as-transient pollution (WR-02), cross-provider attribution (WR-03), exec-bit cleanup (WR-05).

---

_Verified: 2026-09-10T19:12:04Z_
_Verifier: Claude (gsd-verifier) — re-verification after 24-06 gap closure (cba127a..d3623dc)_
