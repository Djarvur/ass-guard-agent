---
phase: 09-serve-path-audit-zcode-parity-re-capture
verified: 2026-08-16T07:26:11Z
status: human_needed
score: 6/7 must-haves verified
behavior_unverified: 1 # truths present + wired but whose acceptance is a recorded operator disposition, not provable by grep/test
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 5/7
  prior_report: verified 2026-08-15T23:09:17Z at HEAD 25146f8 (committed by cfababa)
  trigger: cfababa ("test(profile): refresh re-pin-stale expectations...") claims to close the single prior gap and claims "Full mise ci green"
  gaps_closed:
    - "Phase gate: `mise ci` clean (test leg `go test -race -count=1 ./...` green at HEAD) — the four stale GLM-5.2/thinking expectations were refreshed by cfababa; all four previously-failing tests re-run PASS by this verifier; full race suite green on an unloaded machine (25/25 pkgs, exit 0); vet+build+lint legs also PASS"
  gaps_remaining: []
  regressions: [] # no verified truth regressed; two non-regression observations recorded in the report body: (a) Phase-6 TestZeroConfigFirstRun is load-sensitive (15s stdin-EOF deadline) — failed once under verifier-induced concurrent CPU load, passes isolated AND in the unloaded full run; (b) the pinned rollout file rotated off ~/.zcode/cli/rollout, so the stability test now SKIPs locally (designed degradation; machinery unchanged, cfababa's extract.go delta is comment-whitespace only)
behavior_unverified_items:
  - truth: "AUD-05 parity clause: thresholds explicitly re-baselined"
    test: "Operator disposes the recorded re-baseline finding in profiles/zcode/drift-reports/2026-08-16-recapture.md"
    expected: >-
      An explicit operator decision: accept the fresh numbers as the new recorded reference
      (curated 1/8 on BOTH old and new bundles -- control-proven profile-independent stale
      expectations; from-rollout 1/13 with documented harness artifacts; Phase-1 baseline
      marked SUPERSEDED, no silent threshold movement) and schedule the two follow-ups
      (re-record curated expectations from current live zcode; ExtractTurnsFromRollout
      delta-record handling + per-turn workspace fixtures), OR reject and re-open the leg.
    why_human: "The plan itself closed as complete-with-finding pending this disposition; no grep or test can make an operator acceptance decision"
human_verification:
  - test: "Read profiles/zcode/drift-reports/2026-08-16-recapture.md §'Parity re-baseline' and decide: accept the fresh numbers as the new recorded reference + schedule the two follow-ups, or reject and re-open the leg"
    expected: "Explicit operator acceptance or rejection of the re-baseline (curated FAIL 1/8 identically on OLD and NEW bundles -- control-proven stale v1.0-era expectations, profile-independent; from-rollout 1/13 with documented artifacts; Phase-1 baseline SUPERSEDED)"
    why_human: "An acceptance decision on a control-experiment finding; no automated check can make it. Phase 12 SC-8 is adjacent but does not explicitly cover the curated-suite re-recording"
  - test: "Run a real `ass-guard acp serve` against the live provider (needs ZAI_API_KEY); observe a redacted request_shaped line + an engine_decision line in .ass-guard/audit/<session>.jsonl"
    expected: "Redacted lines present; no credential material anywhere in the artifact tree"
    why_human: "The in-process e2e (TestServeAudit_RequestShapedThroughRealSeam) proves the machinery through the REAL factory — and now PASSES at HEAD — but 09-06-SUMMARY records the operator witness as pending and no witness record exists in STATE.md"
  - test: "Confirm the operator-directed app-server stdio driver capture (drift report §'Capture method') stands in place of the runbook's interactive procedure, or formalize via override"
    expected: "Acceptance as-is (drift report + STATE.md already record the operator direction) or a formal overrides: entry"
    why_human: "Documented deviation from the runbook's literal procedure; substance preserved (scripted, divergence-prone, thresholds applied, not richest-session selection)"
  - test: "(informational) User-story format: goal reads 'As an operator' / 'I want every ... session to leave' — user-story.validate regex wants literal 'As a ' / ', I want '"
    expected: "Operator reformats via /gsd mvp-phase 9 or accepts as-is (role/capability/outcome all present substantively)"
    why_human: "Format discrepancy only; verification proceeded against the obvious slots"
---

# Phase 9: Serve-Path Audit + zcode Parity Re-capture Verification Report

**Phase Goal:** As an operator of a hands-off agent, I want every `acp serve` session to leave a redacted, bounded audit trail that also records the engine's decisions, and the zcode parity stability test re-grounded on a newly pinned capture session, so that I can answer "what did the agent do, and why did it continue" from the log alone and trust that the mimicry hasn't drifted.
**Verified:** 2026-08-16T07:26:11Z (re-verification at HEAD cfababa, clean tree)
**Status:** human_needed
**Re-verification:** Yes — after gap closure. Prior report: 2026-08-15T23:09:17Z @ 25146f8, gaps_found 5/7. This run re-verified the single failed truth (the phase gate / four stale-expectation tests) with real test runs, spot-re-confirmed the stability leg, and regression-checked the audit-chain wiring. Diff scope `25146f8..HEAD` is exactly one commit (cfababa): the four claimed test files + `internal/profile/extract.go` (comment-whitespace-only nolint alignment — zero functional delta; the commit message's "four tests" claim is accurate in substance) + two planning docs.

**Verdict: the prior gap is CLOSED — all automated truths now verified at HEAD. What remains is exactly what cannot be closed by code or tests: the operator disposition of the parity re-baseline, the live-serve witness, and two acceptance/format notes. One new environmental observation (pinned rollout file rotated off disk) is recorded for the operator — it invalidates nothing verified, but Phase 12's ACP-07 depends on that pin.**

## User Flow Coverage (MVP mode)

User story: «As an operator of a hands-off agent, I want every `acp serve` session to leave a redacted, bounded audit trail that also records the engine's decisions, and the zcode parity stability test re-grounded on a newly pinned capture session, so that I can answer "what did the agent do, and why did it continue" from the log alone and trust that the mimicry hasn't drifted.»

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Serve a session | A turn on `acp serve` leaves per-session artifacts: transcript + audit mirror under `.ass-guard/audit/` (default ON) | `cmd/ass-guard/acp_serve.go:222-249` (startAuditMirror), `:330-331`; e2e `TestServeAudit_RequestShapedThroughRealSeam` drives the REAL `runACPServe` over pipes through the REAL factory — **PASS re-run by this verifier at cfababa (0.12s)**; prior run's stale GLM-5.2 substring fixed → whole test now green incl. body-store roundtrip | ✓ |
| Ask "what did it do" | Per-event transcript lines with the correlation triple (session/turn/request) incl. `request_shaped` | `internal/session/transcript.go:63-77`, `manager.go:145`; e2e assertions TurnID/Profile/Ref/Model/Bytes pass (part of the green e2e above) | ✓ |
| Ask "why did it continue" | One `engine_decision` line: action + signal + matched span + config source | `internal/event/events.go:123-138`, `engine/decide.go:41-48`, `engine/observe.go:319` → `manager.go:248`; engine + session packages green in the full race run | ✓ |
| Debug a body on demand | Full redacted body retrievable by hash from a capped store | `internal/audit/bodystore.go`; `ok internal/audit 4.436s` in the full race run; e2e body-store roundtrip assertion green | ✓ |
| Trust the mimicry | Stability test green on the pinned divergence-prone session; drift reported before the profile moved | Machinery intact and previously behaviorally PROVEN (verifier-run PASS at 25146f8; cfababa touched only comment whitespace on this path). NOTE: a fresh run now SKIPs by design — the pinned rollout file rotated off `~/.zcode/cli/rollout` (dir now holds only Aug-16-02:21+ sessions); test source documents this exact degradation path ("skips cleanly while the pinned capture is absent; runbook is the re-grounding procedure"). Parity numbers recorded; disposition pending operator | ◐ |

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | AUD-01: single-sourced factory-seam capturer, both protocol shapes; tracer refactored onto the seam; no divergent copies (`tracerProvider` banned) | ✓ VERIFIED (regression-checked) | `grep -rn tracerProvider --include="*.go"` → 0 matches at cfababa; `BuildWithCapturer` still the single seam — `main.go:133` and `acp_serve.go:348` both construct through it, `Build` delegates with nil (`factory.go:125`); untouched by cfababa |
| 2 | AUD-02: every serve-path session writes a redacted audit trail — correlation IDs incl. `RequestShaped` + optional `--audit-log` mirror (0600, append-only JSONL, rejects stdout) | ✓ VERIFIED (regression-checked) | `OpenFileSink` stdout-rejection intact (`audit.go:32,46`); transcript correlation triple unchanged; `ok internal/audit` + `ok internal/session` in the full race run; the previously-red e2e now fully GREEN |
| 3 | AUD-03: audit volume bounded via body_ref (hash in event, full body in capped store); write failures loud, never fatal | ✓ VERIFIED (regression-checked) | `bodystore.go` unchanged since prior verification (sha256 of redacted bytes, 64 MiB cap, oldest-eviction, dedup); `ok internal/audit 4.436s` |
| 4 | AUD-04: audit records engine decisions (continue/hook/ask/wait + matched signal) — "why did it continue" answerable from the log alone | ✓ VERIFIED (regression-checked) | MatchDetail → Decision → EngineDecision event → `Manager.AppendEngineDecision` (`manager.go:248`) all present at cfababa; `ok internal/engine 16.249s` in the full race run |
| 5 | AUD-05 (machinery legs): stability test green on the newly pinned divergence-prone session; pinned session ID consumed; zcode + extractor versions recorded; drift report committed before any profile update | ✓ VERIFIED | Machinery + ordering verified at 25146f8 (verifier-run PASS 0.04s on `sess_3cee56ae` from `coverage.yaml`; commit order 5a2c7f5 drift → d3fcfa1 extractor → 3a250b5 re-pin; canary PASS = non-vacuity). cfababa's only change on this path is comment whitespace in `extract.go`. CAVEAT (environment, not code): the pinned rollout file has since rotated off `~/.zcode/cli/rollout/`, so a fresh run now SKIPs with the designed message ("pinned session ... absent — re-ground per docs/recapture-runbook.md"); the PASS was real when the capture existed and nothing that produced it changed. See Info note below re Phase-12 ACP-07 |
| 6 | AUD-05 (parity clause): thresholds explicitly re-baselined | ⚠ PRESENT_BEHAVIOR_UNVERIFIED | Fresh numbers recorded in `profiles/zcode/drift-reports/2026-08-16-recapture.md` §"Parity re-baseline" (curated FAIL 1/8 IDENTICALLY on old AND new bundles — control run proves profile-independent stale v1.0-era expectations; from-rollout 1/13 with documented artifacts: 6 empty-expected delta-records, 4 workspace-pollution, 1 by-design-absent runtime MCP tool; Phase-1 baseline SUPERSEDED; follow-up items listed §55-57). The plan closed `complete-with-finding` — acceptance is an OPERATOR DECISION (Human Verification item 1) |
| 7 | Phase gate: `mise ci` clean (test leg `go test -race -count=1 ./...` green at HEAD) | ✓ VERIFIED | All four legs re-run by this verifier at cfababa: `go vet ./...` exit 0; `CGO_ENABLED=0 go build ./...` exit 0; `golangci-lint run` 0 issues; `go test -race -count=1 ./...` **exit 0, 25/25 packages ok** (unloaded run). First full-suite attempt showed one failure — `TestZeroConfigFirstRun` (Phase-6 test, zeroconfig_test.go:32 "did not exit within 15s after stdin EOF", 18.34s elapsed) — caused by this verifier's own concurrent golangci-lint run starving the timing deadline; it passes isolated under `-race` (3.95s) and in the unloaded full run. Not a Phase-9 artifact; recorded as a load-sensitivity note |

**Score:** 6/7 truths verified (1 present/behavior-unverified — the operator-disposition item)

### Gap Closure (re-verification focus)

The prior report's only failed truth was the phase gate: commit 3a250b5 (GLM-5.3 / no-thinking re-pin) landed with four tests still pinning GLM-5.2 / thinking-budget 32000 expectations. Commit cfababa claims to fix exactly this. Verified:

1. **Commit scope** (`git show cfababa --stat`): the four claimed test files changed — `cmd/ass-guard/acp_serve_test.go` (GLM-5.2→GLM-5.3 substring), `internal/profile/loader_test.go` (model expectation), `internal/shaper/shaper_test.go` + `internal/shaper/fidelity_test.go` (model + thinking assertions inverted: "must emit budget 32000" → "must NOT emit thinking"). It also touched `internal/profile/extract.go` — **comment-whitespace only** (nolint directive alignment), zero functional delta, unmentioned in the commit message but harmless.
2. **Not test-weakening:** the inverted thinking assertions match the actual pinned bundle — `profiles/zcode/profile.yaml` is `model: GLM-5.3` and carries NO thinking key (grep: 0 matches); "thinking" appears only in coverage/drift docs and prompt prose (system/block-2.txt). The `len(opts) != 12` identity-header assertions were retained in both shaper tests.
3. **Re-run of the four previously-failing tests** (`-count=1`): all PASS — see Behavioral Spot-Checks.

### Required Artifacts

All previously verified; cfababa touched none of them functionally. Regression checks: tracerProvider 0 matches, seam call-sites intact (main.go:133, acp_serve.go:348, factory.go:125), EngineDecision chain intact (events.go:117, manager.go:248), stdout-rejection guard intact (audit.go:32), bodystore/mirror/transcript unchanged by the diff scope.

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/scheduler/factory.go` | BuildWithCapturer seam, both shapes | ✓ VERIFIED | unchanged since prior report |
| `internal/audit/{audit,bodystore,mirror}.go` | sink/bounded-store/mirror | ✓ VERIFIED | unchanged; `ok internal/audit` in race run |
| `internal/event/events.go` | RequestShaped + EngineDecision | ✓ VERIFIED | unchanged |
| `internal/engine/{types,decide,observe}.go` | MatchDetail → Decision → writer | ✓ VERIFIED | unchanged; `ok internal/engine` |
| `internal/session/{transcript,manager,transcript_writer}.go` | correlation triple + body_ref | ✓ VERIFIED | unchanged; `ok internal/session` |
| `cmd/ass-guard/{main,acp_serve}.go` | seam wiring; --audit-log | ✓ VERIFIED | unchanged |
| `internal/profile/stability_test.go` | consumes the pinned session | ✓ VERIFIED | unchanged; SKIPs by design now that the rollout file rotated (see Info) |
| `docs/recapture-runbook.md` | operator procedure | ✓ VERIFIED | unchanged |
| `profiles/zcode/coverage.yaml` | pinned session + versions | ✓ VERIFIED | unchanged (sess_3cee56ae, zcode 0.16.3) |
| `profiles/zcode/drift-reports/2026-08-16-recapture.md` | drift + parity re-baseline record | ✓ VERIFIED | unchanged; disposition pending |

### Key Link Verification

Regression-checked via grep at cfababa (all intact; none touched by the diff scope):

| From | To | Via | Status |
|------|----|----|--------|
| `cmd/ass-guard/acp_serve.go` makeProvider | `scheduler.BuildWithCapturer` | acp_serve.go:348 | ✓ WIRED |
| `cmd/ass-guard/main.go` runTrace | `scheduler.BuildWithCapturer` | main.go:133 | ✓ WIRED |
| `internal/engine/observe.go` | `Manager.AppendEngineDecision` | observe.go:319 → manager.go:248 | ✓ WIRED |
| `internal/profile/stability_test.go` | coverage.yaml pin → rollout file | stability_test.go:48-77 | ✓ WIRED (SKIPs cleanly while the pinned file is absent — designed) |

### Behavioral Spot-Checks (this re-verification)

| Behavior | Command | Result | Status |
|--------|---------|--------|--------|
| Previously-failing e2e (serve audit through REAL seam) | `go test ./cmd/ass-guard/ -run '^TestServeAudit_RequestShapedThroughRealSeam$' -count=1 -v` | `--- PASS (0.12s)` | ✓ PASS |
| Previously-failing loader test | `go test ./internal/profile/ -run '^TestLoader_ZcodeProfile$' -count=1 -v` | `--- PASS (0.00s)` | ✓ PASS |
| Previously-failing shaper tests | `go test ./internal/shaper/ -run '^TestShape_ZcodeProfile$\|^TestFidelity_ToolCountAndThinking$' -count=1 -v` | both `--- PASS (0.01s)` | ✓ PASS |
| Stability test on the pinned session | `go test ./internal/profile/ -run '^TestStability_WithinSessionExtractionSource$' -count=1 -v` | `--- SKIP (0.00s)`: "pinned session sess_3cee56ae... absent — re-ground per docs/recapture-runbook.md" | ◐ SKIP-by-design (rollout file rotated off disk; PASS was verifier-proven at 25146f8; code path unchanged) |
| Full race suite (gate test leg), unloaded | `go test -race -count=1 ./...` | exit 0, 25/25 `ok` (profile 114.6s, openspec 85.8s heaviest) | ✓ PASS |
| Full race suite, verifier-loaded machine (first attempt) | same | 1 FAIL: `TestZeroConfigFirstRun` (Phase-6; "did not exit within 15s after stdin EOF" at 18.34s) while golangci-lint ran concurrently | ◐ load flake (see below) |
| TestZeroConfigFirstRun isolated | `go test ./cmd/ass-guard/ -run '^TestZeroConfigFirstRun$' -race -count=1 -v` | `--- PASS (3.95s)` | ✓ PASS |
| vet / build / lint legs | `go vet ./...` · `CGO_ENABLED=0 go build ./...` · `golangci-lint run` | exit 0 · exit 0 · 0 issues | ✓ PASS |

### Probe Execution

No `scripts/*/tests/probe-*.sh` probes declared by this phase's plans; not a migration/tooling phase. Step 7c: SKIPPED (no probes declared).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|---------------------|----------|----------|
| AUD-01 | 09-01 | single-sourced factory-seam capturer both shapes | ✓ SATISFIED | regression-checked at cfababa (truth 1) |
| AUD-02 | 09-01, 09-05, 09-06 | redacted serve-path audit trail + `--audit-log` mirror | ✓ SATISFIED | regression-checked; e2e now fully green |
| AUD-03 | 09-05 | body_ref bounding + loud-never-fatal | ✓ SATISFIED | unchanged; audit pkg green in race run |
| AUD-04 | 09-02 | engine decisions with matched signal in the log | ✓ SATISFIED | unchanged; engine pkg green in race run |
| AUD-05 | 09-03, 09-04 | stability green on newly pinned runbook-produced session, pin consumed, versions recorded, thresholds re-baselined, drift-before-update | ◐ PARTIAL | machinery/capture/pin/stability/ordering VERIFIED (behaviorally proven at prior HEAD; code unchanged since; fresh runs now SKIP because the rollout file rotated — designed degradation); parity re-baseline recorded-with-finding pending operator disposition; capture via operator-directed app-server stdio driver (documented deviation, acceptance item 3) |

No orphaned requirements: REQUIREMENTS.md maps exactly AUD-01..05 to Phase 9, all claimed by plans.

### Anti-Patterns Found

The prior report's four blocker rows (stale GLM-5.2/thinking expectations) are RESOLVED by cfababa — verified against the actual bundle (GLM-5.3, no thinking key in profile.yaml) and by passing tests. No TBD/FIXME/XXX markers in any file cfababa touched (grep: 0 matches).

| Item | Pattern | Severity | Impact / Disposition |
|------|---------|----------|---------------------|
| Pinned rollout source rotated off `~/.zcode/cli/rollout/` | stability test input no longer on disk; test SKIPs (designed) | ℹ️ Info (operator action recommended) | Derivable artifacts ARE committed (profile bundle, coverage.yaml, drift report); only the raw 56-MB-class source is gone. RECOMMEND archiving the rollout file (or a distilled fixture) into durable storage before Phase 12 — ACP-07 "re-pinned against the newly pinned Phase-9 capture session (depends on AUD-05's pin)" needs that session; otherwise re-ground via docs/recapture-runbook.md |
| `TestZeroConfigFirstRun` (Phase-6) exceeded its 15s stdin-EOF deadline under concurrent CPU load | load-sensitive timing test | ℹ️ Info | Not a Phase-9 artifact (added 591fa7a, plan 06-01); passes isolated (3.95s) and in the unloaded full suite. `mise ci` runs legs sequentially so the claim "Full mise ci green" reproduces; worth knowing for parallel-CI futures |

### Human Verification Required

**1. Parity re-baseline disposition (AUD-05 clause — the plan's own recorded finding)**
**Test:** Read `profiles/zcode/drift-reports/2026-08-16-recapture.md` §"Parity re-baseline" and decide.
**Expected:** Explicit operator acceptance (fresh numbers = new recorded reference; schedule follow-ups: re-record curated expectations from current live zcode; `ExtractTurnsFromRollout` delta-record handling + per-turn workspace fixtures) or rejection with a re-opened leg.
**Why human:** An acceptance decision on a control-experiment finding; no automated check can make it. Phase 12 SC-8 (behavioral-eval regression net) is adjacent but does not explicitly cover the curated-suite re-recording.

**2. Live-serve redacted-audit witness (phase-gate leg)**
**Test:** Run a real `ass-guard acp serve` against the live provider; observe a redacted `request_shaped` line + an `engine_decision` line in `.ass-guard/audit/<session>.jsonl`.
**Expected:** Redacted lines present; no credential material anywhere in the artifact tree.
**Why human:** The in-process e2e proves the machinery through the REAL factory and now PASSES at HEAD — but 09-06-SUMMARY still records the operator witness as "pending morning" and no witness record was found in STATE.md. Requires ZAI_API_KEY.

**3. Capture-method deviation acceptance (optional, to make the record airtight)**
**Test:** Confirm the operator-directed app-server stdio driver capture (drift report §"Capture method") stands in place of the runbook's interactive procedure.
**Expected:** Accept as-is (drift report + STATE.md already record the operator direction) or add a formal `overrides:` entry (suggested text in the prior report, carried forward unchanged).

**4. User-story format discrepancy (informational).** `user-story.validate` wants literal "As a " / ", I want to "; the goal reads "As an operator" / "I want every … session to leave". Substantively a user story; operator may reformat via `/gsd mvp-phase 9` or accept as-is.

### Gaps Summary

No gaps remain. The single prior gap (phase gate red from four stale post-re-pin expectations) is closed by cfababa and independently confirmed: the four tests re-run PASS, the expectations match the actual pinned bundle (not weakened — the no-thinking assertions mirror the bundle's lack of a thinking config), and the full gate reproduces green (vet, lint, build, race suite 25/25 on an unloaded machine; the one first-attempt failure was this verifier's own concurrent lint run starving a Phase-6 timing test, which passes isolated and unloaded).

Everything the goal promised is verified in code and by tests: the single factory seam, the redacted default-ON audit mirror with correlation triples, body_ref bounding with a capped retrievable store, the engine-decision provenance line, and the parity machinery with drift-before-update ordering. Two operator-facing items remain open by nature, not by defect: the parity re-baseline disposition (recorded finding) and the live-serve witness. One environmental note for the operator: the pinned capture session's rollout file has rotated off disk — archive it or plan a re-ground before Phase 12 consumes the pin (ACP-07).

---

_Verified: 2026-08-16T07:26:11Z (re-verification)_
_Previous: 2026-08-15T23:09:17Z @ 25146f8 (gaps_found 5/7)_
_Verifier: Claude (gsd-verifier)_
