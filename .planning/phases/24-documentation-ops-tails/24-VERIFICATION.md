---
phase: 24-documentation-ops-tails
verified: 2026-09-10T18:07:08Z
status: gaps_found
score: 11/14 must-haves verified
covered_files:
  - .planning/phases/24-documentation-ops-tails/24-01-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-02-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-03-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-04-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-05-PLAN.md
  - .planning/phases/24-documentation-ops-tails/24-01-SUMMARY.md
  - .planning/phases/24-documentation-ops-tails/24-02-SUMMARY.md
  - .planning/phases/24-documentation-ops-tails/24-03-SUMMARY.md
  - .planning/phases/24-documentation-ops-tails/24-04-SUMMARY.md
  - .planning/phases/24-documentation-ops-tails/24-05-SUMMARY.md
  - .planning/REQUIREMENTS.md
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
covered_digest: "v1:sha256:5a2df1531ff18ed86ebb99ef6672515154b4241d83c571ae7fbe21a529a8e501"
behavior_unverified: 1 # truths present + wired but the runtime behavior is not exercisable pre-merge
overrides_applied: 0
gaps:
  - truth: "Plugins/skills prove working unchanged in EVERY interaction mode (ECOS-04 end-to-end) — the wake mode is functionally proven"
    status: failed
    reason: >-
      The three wake cells (wake x commands/skills/hooks) are skipped with PRECONDITION-UNMET(22,
      background wake-turn D-01) citing "Phase 22 is PLANNED-BUT-UNEXECUTED on the roadmap this
      harness ships against" — but Phase 22 EXECUTED before Phase 24: the wake-turn machinery
      (22-CONTEXT D-01: agent-initiated wake turn through the automation-turn machinery) shipped in
      feat(22-01) 43a8378 on 2026-09-08 and was gap-closed by 22-07/22-08 on 2026-09-10 morning
      (commits 5ad02a0, a59baf9), ALL ancestors of Phase 24's first commit 5552942 (2026-09-10
      16:56). The substrate is live and green today: wakeDrainChain (internal/runtime/cron_wiring.go:403)
      and TestWakeTurn_BackgroundBashCompletion PASS under -race (re-run during verification).
      The skip premise is factually false, so 3 of 12 matrix cells carry no functional evidence
      while their substrate exists and is drivable in-package exactly as the automation row is.
      The 24-05 summary's "honest ECOS-04 state" claim is built on the same false premise
      (likely a stale ROADMAP.md phase-22 checkbox), and REQUIREMENTS.md marks TAIL-03 [x] Complete.
    artifacts:
      - path: internal/runtime/modesmatrix_wake_test.go
        issue: "Comment asserts Phase 22 is PLANNED-BUT-UNEXECUTED; all three cells are SkipPrecondition stubs despite the live substrate"
      - path: .planning/phases/24-documentation-ops-tails/24-05-SUMMARY.md
        issue: "Asserts the wake marks are the honest state; the precondition citation is false"
    missing:
      - "Implement the three in-file replacement assertions (already spelled out in modesmatrix_wake_test.go): wake x commands (expanded MATRIX-ECHO-EXPANSION body in the wake turn's user message), wake x skills (MATRIX-SKILL-BODY expansion), wake x hooks (PreToolUse marker in the wake turn + SubagentStop at the completing dispatch), driven through the real wake machinery the wake_wiring_test.go battery already exercises"
      - "Correct the false PREMISE text in modesmatrix_wake_test.go and 24-05-SUMMARY.md; update the WINDOWS ledger deviation entry (it resolves now, not 'when Phase 22 executes')"
  - truth: "Replayed breaker evidence bends live routing decisions CORRECTLY (D-06) — the demoted target is usable on the session provider's wire"
    status: partial
    reason: >-
      CR-01 (code review Critical, independently confirmed in code): resolveSubagentModel's
      demotion block (internal/runtime/runtime.go:3170-3180) and ConfigSurface.demoteIfDeniedLocked
      (internal/acpserve/config_surface.go:767-792) walk [primary, fallbacks...] through
      modelrouting.FirstAllowed with NO same-provider filter. Config validation permits
      cross-provider fallback chains (checkBinding checks capability compatibility, never provider
      equality), so an open light primary can demote a subagent to a model slug hosted by a
      DIFFERENT provider — stamped onto a profile that rides the session provider's wire. This
      violates resolveSubagentModel's own documented contract (runtime.go:3130-3136: a different
      provider means "" + loud warning, "never a silent wrong-wire"); on the surface side
      (WR-01) it advertises a model the live session cannot apply. Tests cover same-provider
      chains only (lightTierTestConfig: primary and fallback both on anthropic), so the suite
      is green while the defect exists for a supported configuration class.
    artifacts:
      - path: internal/runtime/runtime.go
        issue: "Demotion walk at :3170-3180 not provider-filtered; returns cross-provider pick.Model against the function's own contract"
      - path: internal/acpserve/config_surface.go
        issue: "demoteIfDeniedLocked at :767-792 same unfiltered walk (WR-01)"
      - path: internal/runtime/apply_model_test.go
        issue: "Demotion battery uses same-provider fixtures only; no cross-provider case"
    missing:
      - "Filter the demotion chain to the session provider before FirstAllowed (or keep the primary when only cross-provider candidates are allowed) in BOTH resolveSubagentModel and demoteIfDeniedLocked"
      - "Add cross-provider demotion test cases at both sites pinning the guarded behavior"
behavior_unverified_items:
  - truth: "Nightly CI runs the upstream-parity gate unattended on a schedule and reports drift without human triggering (SC-3 live half)"
    test: "After the v1.2 milestone merges to master: confirm the Actions tab shows nightly-parity scheduled runs firing at cron 41 3 * * *; run the workflow_dispatch smoke and confirm build-test green + drift-job report artifact; register the [self-hosted, zcode] runner; confirm Workflow permissions allow GITHUB_TOKEN issue creation"
    expected: "Scheduled runs appear unattended; dispatch run shows build-test green and a report artifact on every run; drift job starts once a matching runner is online; a drift opens an issue"
    why_human: "The schedule fires only when the workflow file exists on the DEFAULT branch (GitHub behavior — the plan's documented activation point); dispatch was 404-rejected pre-merge (both gh workflow run --ref and the REST dispatches API, recorded in 24-04-SUMMARY) per the plan's own Task 3 contingency (DEFERRED-TO-MERGE with local stand-in evidence); zero self-hosted runners are registered"
---

# Phase 24: Documentation & Ops Tails Verification Report

**Phase Goal:** Small independent tails close out: LSP documented as an IDE-side MCP configuration requirement (no agent-side implementation), the scheduler gains its deterministic outcome store + feedback loop, nightly upstream-parity CI automation runs unattended, and plugins/skills prove working unchanged in every interaction mode (ECOS-04 end-to-end).
**Verified:** 2026-09-10T18:07:08Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC-1/DOC-01: guide exists, honest three-leg Zed surface, README-linked, dry-run executed with recorded evidence | ✓ VERIFIED | docs/lsp-setup.md (399 lines) carries context_servers (6x), mcpServers, .mcp.json, diagnostics, 52449, explicit out-of-scope statement; README link `lsp setup → docs/lsp-setup.md`; dry-run evidence in 24-03-SUMMARY (six `mcp__gopls__*` tools observed via scripted catalog check against the real binary; zed#52449 re-checked 2026-09-10) |
| 2 | 24-03 prohibitions: no forwarded-mcpServers consumption claim; no agent-side LSP promise | ✓ VERIFIED | Grep for affirmative consumption phrasing returns nothing; out-of-scope statement present; dry-run fixes applied (silent MCP-skip correction) |
| 3 | SC-2a/TAIL-01: outcomes stored deterministically, zero LLM calls, mechanical schema only | ✓ VERIFIED | outcomes.go: exactly 11 mechanical json-tagged fields (at/provider/model/tier/outcome/fallback_used/latency_ms/in_tokens/out_tokens/cost_usd/origin); no prompt/content/quality field; `go test -race ./internal/modelrouting/ -run 'TestOutcome|TestFirstAllowed'` ok (tolerant read, interrupted tail, concurrency, perms+gitignore, no-dedup all pinned) |
| 4 | 24-01 seam replay: seeded transients open breakers, ok recovers, structural/exhausted never feed, cost degrade fires, FirstAllowed demotes | ✓ VERIFIED | outcomes_test.go battery green under -race (TestOutcomeFeedbackBreakerOpens / RecoversAndSkipsStructural / CostDegrade, TestFirstAllowedDemotes); ReplayBreakers mirrors dispatch.go outcome learning |
| 5 | 24-02 live recording: one record per completed attempt (turn + subagent), store failure never fails a turn | ✓ VERIFIED | recordDispatchOutcome at session.go:700 (turn) and subagent.go:215 (subagent); `go test -race ./internal/session/ -run TestSessionOutcomeRecording` ok; nil-store = byte-identical |
| 6 | 24-02 stats CLI: human→stderr (stdout byte-clean), --json→stdout only, missing store exit 0 | ✓ VERIFIED | Live run of the built binary over a seeded store: human path stdout 0 bytes / aggregate on stderr; --json parses to stdout; nonexistent store → "no outcomes recorded yet", exit 0; TestSchedulingStats* green |
| 7 | SC-2b/D-06: replayed breaker evidence bends BOTH live Resolve sites; empty store changes nothing | ✓ VERIFIED | SetOutcomeBreakers called exactly once (acp_serve.go:314); resolveSubagentModel's single production caller passes r.outcomeBreakersFor() (runtime.go:4194); TestOutcomeBreakersDemoteSessionModel / EmptyMapKeepsPrimary / TestResolveSubagentModelBreakerDemotion green under -race — **BUT see CR-01 blocker under Anti-Patterns and gap 2: the demotion is not provider-guarded** |
| 8 | 24-02: with an empty/absent store, resolution output is byte-identical to pre-change behavior | ✓ VERIFIED | Empty-map guard pinned by TestOutcomeBreakersEmptyMapKeepsPrimary (code: `len(breakers)==0 → return primary.Model`) |
| 9 | SC-3 core/TAIL-02: offline drift core (probe seam + sha256 vs committed pin), exit 0/7/1 contract, report always written | ✓ VERIFIED | Live run during verification: built binary against the real bundle → exit 7 with truthful version drift (pinned 0.16.3 vs installed desktop build), 7/7 file digests match, report JSON non-empty (drift/files/generated_at/summary/zcode_version); TestNightlyCheck battery green under -race |
| 10 | 24-04 workflow contract: off-peak cron + workflow_dispatch, least-privilege perms, artifact-always, issue body from report FILE, eval-gate untouched | ✓ VERIFIED | cron "41 3 * * *", workflow_dispatch present, top-level contents: read, drift job [self-hosted, zcode] with issues: write, upload-artifact if: always(), fs.readFileSync + github.rest.issues.create (no shell interpolation), 0 pull_request_target, 0 ASSGUARD_EVAL_GATE references; ruby YAML parse OK; [tasks.parity-nightly] in .mise.toml invokes the same cobra path |
| 11 | SC-3 live half: nightly runs unattended on a schedule without human triggering | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Core + workflow + pin all live-proven locally; the unattended run is structurally post-merge (schedule fires only from the default branch; dispatch 404-rejected twice, recorded per the plan's own Task 3 DEFERRED-TO-MERGE contingency; zero runners registered) — see behavior_unverified_items and Human Verification |
| 12 | SC-4 exercised modes/TAIL-03: plugins/skills FUNCTIONAL (not merely discovered) in interactive, subagent, automation x commands/skills/hooks | ✓ VERIFIED | TestModesMatrixInteractive(+Surfaces/+Empty), TestModesMatrixSubagent, TestModesMatrixCronCommands/Skills/Hooks/Empty all green under -race; marker-file + received-prompt/transcript lenses assert functional outcomes; fixture hermetic |
| 13 | SC-4 wake mode/TAIL-03: the wake row functionally proven (or honestly precondition-marked) | ✗ FAILED | modesmatrix_wake_test.go skips all 3 cells citing "Phase 22 PLANNED-BUT-UNEXECUTED" — FALSE: wake machinery shipped 2026-09-08 (43a8378) and was gap-closed 2026-09-10 morning, all ancestors of phase 24's base 5552942; TestWakeTurn_BackgroundBashCompletion passes at HEAD; the substrate is live and drivable — gap 1 |
| 14 | 24-05: matrix totality + honest loud-skip vocabulary (never a fabricated green) | ✗ FAILED | The 12-cell grid IS total and the loudness machinery works (wake count exactly 3, AssertComplete, prefix pinned) — but the wake skip's PREMISE is factually false, so the "honest ECOS-04 state" claim is itself wrong (same root cause as gap 1; counted separately because honesty is TAIL-03's core deliverable) |

**Score:** 11/14 truths verified (1 present, behavior-unverified; 2 failed)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| internal/modelrouting/outcomes.go | Store + record schema | ✓ VERIFIED | 208 lines; append-only O_APPEND 0600, self-gitignore, tolerant ReadOutcomes |
| internal/modelrouting/outcomes_agg.go | Aggregate/Replay*/FirstAllowed | ✓ VERIFIED | 187 lines; ReplayCostTracker present but ORPHANED live (WR-04) |
| internal/modelrouting/outcomes_test.go | 9-test RED→GREEN suite | ✓ VERIFIED | 484 lines, green under -race |
| internal/session/session_outcomes_test.go | Live record battery | ✓ VERIFIED | 357 lines, green under -race |
| internal/acpserve/config_surface_outcomes_test.go | Surface demotion tests | ✓ VERIFIED | 113 lines, green under -race |
| internal/modelroutingcmd/modelrouting.go | Stats renderers | ✓ VERIFIED | LoadAndAggregate/BuildStatsRows/EmitStatsHuman/EmitStatsJSON; live CLI discipline verified |
| cmd/ass-guard/modelrouting.go | stats subcommand | ✓ VERIFIED | Registered in the model-routing group (:25) |
| docs/lsp-setup.md | Three-leg guide | ✓ VERIFIED | 399 lines, all structural keys, citations, honest status |
| README.md | Docs index link | ✓ VERIFIED | Mid-sentence style, adjacent to install |
| internal/profilecheckcmd/nightly_check.go + test | Drift core | ✓ VERIFIED | Exit contract live-verified (0/7/1); 7-case battery green |
| cmd/ass-guard/nightly_check.go | Cobra shell | ✓ VERIFIED | Dual-group registration (parity + profile) |
| profiles/zcode/structure-pin.json | Committed baseline | ✓ VERIFIED | 7 digests, zcode_version 0.16.3; 7/7 match live |
| .github/workflows/nightly-parity.yml | First Actions workflow | ✓ VERIFIED | Parses; full contract verified (truth 10) |
| .mise.toml | [tasks.parity-nightly] | ✓ VERIFIED | Wraps the same build+check invocation |
| internal/modesmatrix/matrix.go (+test) | Cell registry/report/loud-skip | ✓ VERIFIED | 573+139 lines; totality gate unit-pinned |
| modesmatrix_{interactive,subagent,cron,wake} test files | 4 mode legs | ✓ VERIFIED (wake = precondition stubs — see gap 1) | Interactive/subagent/cron green; wake cells are skips on a false premise |
| internal/ecosys/testdata/modes-matrix/ | Fixture plugin | ✓ VERIFIED | plugin.json + hooks + command + skill; hermetic (no network/absolute paths) |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| session/subagent record sites | OutcomeStore.Append | recordDispatchOutcome | ✓ WIRED | session.go:700, subagent.go:215 → session.go:1289-1310 |
| acp_serve composition | ConfigSurface.SetOutcomeBreakers | ReplayBreakers over WorkDir store | ✓ WIRED | acp_serve.go:314, exactly once |
| runtime SubagentModel population | resolveSubagentModel + replayed breakers | outcomeBreakersFor() | ✓ WIRED | runtime.go:4194 (single production caller) |
| stats shell | modelroutingcmd renderers | AddCommand | ✓ WIRED | cmd/ass-guard/modelrouting.go:25 |
| nightly_check.go | paritycli version seam | ZcodeInstalledVersion | ✓ WIRED | Reused seam, injected in tests |
| nightly_check.go | pinned bundle + structure-pin.json | sha256 comparison | ✓ WIRED | Live run: 7/7 match |
| workflow drift job | report JSON | artifact + github-script fs.readFileSync | ✓ WIRED | if: always() + JS-composed issue body |
| docs/lsp-setup.md | README docs index | relative link | ✓ WIRED | Present, existing style |
| ReplayCostTracker | CostTracker live seam | production consumer | ⚠️ ORPHANED | Zero production callers (WR-04) — tested only; the cost half of D-06's feedback is unwired live |
| wake cells | wake-turn machinery | (should drive the real wake path) | ✗ NOT WIRED | Cells are SkipPrecondition stubs although the machinery exists (gap 1) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| stats CLI | aggregate rows | .ass-guard/routing/outcomes.jsonl (ReadOutcomes) | Yes — live run printed seeded row exactly | ✓ FLOWING |
| demotion sites | breakers map | ReadOutcomes + ReplayBreakers over WorkDir store | Yes — seeded stores demote in tests | ✓ FLOWING |
| session recording | DispatchOutcome | live streamErr/usage counters/latency | Yes — TestSessionOutcomeRecording asserts file contents | ✓ FLOWING |
| nightly-check | report JSON | bundle sha256 + meta.yaml + probe | Yes — live report written with real digests | ✓ FLOWING |
| LSP doc dry-run | tool catalog | real binary + mcp-language-server/gopls | Yes — summary evidence (catalog check script exit 0, six tools) | ✓ FLOWING (author-run; editor leg = human) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Outcome store + replay suite | `go test -race ./internal/modelrouting/ -run 'TestOutcome|TestFirstAllowed'` | ok | ✓ PASS |
| Live turn recording | `go test -race ./internal/session/ -run TestSessionOutcomeRecording` | ok | ✓ PASS |
| Surface + interactive matrix | `go test -race ./internal/acpserve/ -run 'TestOutcomeBreakers|TestModesMatrixInteractive'` | ok | ✓ PASS |
| Subagent demotion + cron matrix | `go test -race ./internal/runtime/ -run 'TestResolveSubagentModelBreakerDemotion|TestModesMatrixCron'` | ok | ✓ PASS |
| Stats CLI | `go test ./cmd/ass-guard/ -run TestSchedulingStats` + live binary run | ok; stdout 0 bytes human / JSON stdout / empty-store exit 0 | ✓ PASS |
| Nightly drift core | `go test -race ./internal/profilecheckcmd/ -run TestNightlyCheck` + live binary vs real bundle | ok; exit 7 truthful drift, 7/7 digests match, report written | ✓ PASS |
| Modesmatrix package + subagent row | `go test -race ./internal/modesmatrix/` + `-run TestModesMatrixSubagent` | ok | ✓ PASS |
| Wake substrate (verification of the skip premise) | `go test -race ./internal/runtime/ -run TestWakeTurn_BackgroundBashCompletion` | PASS (1.06s) | ✓ PASS — proves the substrate the wake cells claim is missing |
| Wake-cell loudness count | `-run TestModesMatrixWake -v \| grep -c 'PRECONDITION-UNMET(22'` | 3 | ✓ PASS (count) — but see gap 1 (false premise) |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| DOC-01 | 24-03 | LSP documented as IDE-side MCP config requirement (no agent-side implementation) | ✓ SATISFIED | Truths 1-2; editor leg noted for UAT |
| TAIL-01 | 24-01 + 24-02 | Scheduler outcome store + feedback loop (deterministic, zero LLM) | ✓ SATISFIED (with CR-01 defect → gap 2) | Truths 3-8 |
| TAIL-02 | 24-04 | Nightly upstream-parity CI automation | ✓ SATISFIED at plan letter; unattended half = behavior-unverified (post-merge activation per the plan's own contingency) | Truths 9-11 |
| TAIL-03 | 24-05 | ECOS-04 — plugins/skills working unchanged in every interaction mode | ✗ BLOCKED (partial) | 9/12 cells proven; wake row skipped on a false precondition — gap 1 |

Orphaned requirements: none — REQUIREMENTS.md maps exactly DOC-01/TAIL-01/02/03 to Phase 24, all claimed by plans. Note: REQUIREMENTS.md marks TAIL-03 `[x]` Complete; that checkbox is premature pending gap 1 closure.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/runtime/runtime.go | 3170-3180 | 🛑 CR-01 (review Critical, confirmed): demotion walk not provider-filtered — cross-provider fallback slug returned on the session provider's wire, violating the function's own contract (:3130-3136 "never a silent wrong-wire") | 🛑 Blocker | Wrong-wire model requests for supported cross-provider chains; untested (fixtures same-provider only) — gap 2 |
| internal/runtime/modesmatrix_wake_test.go | 9-16 | 🛑 False precondition claim ("Phase 22 is PLANNED-BUT-UNEXECUTED") justifying 3 skipped cells whose substrate is live | 🛑 Blocker | TAIL-03's every-mode claim unproven; honesty claim itself false — gap 1 |
| internal/modelrouting/outcomes_agg.go | 113 | ⚠️ WR-04: ReplayCostTracker has zero production callers — cost half of the feedback loop unwired (D-06 names cost-degrade as a seam) | ⚠️ Warning | Windowed spend does not survive restart; dead exported API implies enforcement that does not happen |
| internal/session/session.go | 700, 1321-1340 | ⚠️ WR-02: context.Canceled (user Stop) classifies OutcomeTransient — feeds breaker evidence (plan-directed "conservative class", but consequence real: rapid cancels + restart can demote a healthy provider) | ⚠️ Warning | Breaker/stats pollution by non-provider failures |
| internal/session/session.go | 1300 | ⚠️ WR-03: D-15 cross-provider subagent attempts recorded under the session provider — wrong breaker key + stats misattribution | ⚠️ Warning | Evidence rows name the wrong provider |
| internal/acpserve/config_surface.go | 767-792 | ⚠️ WR-01: surface variant of the unfiltered demotion walk (advertises a model applyTargetLocked refuses to apply) | ⚠️ Warning | Folded into gap 2's fix scope |
| ~15 phase-24 files | — | ⚠️ WR-05: accidental 100755 exec bit (38 executable files repo-wide, most .go sources) | ⚠️ Warning | Diff noise; leaks into release metadata |
| cmd/ass-guard/profile_check.go | 30 | ℹ️ WR-06: --zcode-bin parsed but never used (pre-existing) | ℹ️ Info | Silent flag no-op |
| internal/modelrouting/breaker.go | 94-114 | ℹ️ WR-07: half_open_probes knob never enforced (pre-existing) | ℹ️ Info | Unlimited concurrent half-open probes |
| various | — | ℹ️ IN-01..06 (dead import-keeper; cwd-relative stats default; replay/live asymmetry undocumented; symlinked mount copies; duplicate replay plumbing; exact-string version match) | ℹ️ Info | See 24-REVIEW.md |

Debt markers (TBD/FIXME/XXX): none in any phase-modified file. Known pre-existing failures (TestPermissionsE2E, TestRescanConcurrency, evalsuite openspec env, coreexec load-flake) reproduced none of the phase-24 scoped runs and are documented as out of scope.

### Human Verification Required

Status is gaps_found (Step 9 rule 1), so these are recorded for the follow-up/UAT path rather than gating human_needed:

1. **LSP guide live editor leg (24-03 step 7)** — open Zed against the scratch project, prompt the agent to call an `mcp__gopls__*` tool. Expected: the tool executes in the editor-hosted session. Why human: GUI editor behavior; the scripted catalog proof covered everything else.
2. **Nightly unattended activation (post-merge)** — the behavior_unverified_items entry: schedule fires from master, dispatch smoke green, [self-hosted, zcode] runner online, issue-on-drift permission confirmed (24-USER-SETUP.md tracks all four).
3. **D-14 real-plugin spot-check fidelity** — superpowers 6.1.1 observations recorded in 24-05-SUMMARY; the "unchanged vs the operator's own Claude Code setup" judgment is the operator's (the plan's own human-checkpoint item).

### Gaps Summary

Two gaps block the phase goal, both narrow and fixable:

1. **Wake row unproven on a false premise (TAIL-03).** The harness's design (loud precondition marks) was applied to cells whose substrate actually shipped two days earlier in the same tree. The fix is small: implement the three already-written replacement assertions through the live wake machinery, and correct the premise text in the test file and summary. Until then, "every interaction mode" is 3/4 modes proven, and the summary's honesty claim is itself inaccurate.
2. **Cross-provider demotion wrong-wire (CR-01, review Critical).** The D-06 feedback observables hold as tested, but the demotion walk lacks the same-provider guard the surrounding code contract mandates; a cross-provider fallback chain (supported config) gets a wrong-wire slug at both Resolve sites. Fix: provider-filter the walk (or keep the primary) + cross-provider test cases.

The DEFERRED-TO-MERGE nightly dispatch is NOT counted as a gap: the plan's own Task 3 acceptance criteria explicitly authorize that path when GitHub rejects pre-merge dispatch (which it did, twice, with evidence), the local stand-in evidence is complete and was re-verified live here, and schedule activation is structurally a post-merge property of any first workflow. It remains the behavior-unverified item above. If the maintainer prefers it recorded as an accepted deviation rather than a verification item, an override can be added per Step 3b.

Warnings worth scheduling (not gating): ReplayCostTracker dead code (WR-04 — wire it or doc-mark it reserved), cancellation-as-transient pollution (WR-02), cross-provider attribution (WR-03), exec-bit cleanup (WR-05).

---

_Verified: 2026-09-10T18:07:08Z_
_Verifier: Claude (gsd-verifier)_
