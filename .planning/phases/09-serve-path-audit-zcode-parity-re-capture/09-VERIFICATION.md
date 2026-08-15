---
phase: 09-serve-path-audit-zcode-parity-re-capture
verified: 2026-08-15T23:09:17Z
status: gaps_found
score: 5/7 must-haves verified
behavior_unverified: 1 # truths present + wired but whose acceptance is a recorded operator disposition, not provable by grep/test
overrides_applied: 0
gaps:
  - truth: "Phase gate: `mise ci` clean (test leg `go test -race -count=1 ./...` green at HEAD)"
    status: failed
    reason: >-
      The AUD-05 re-pin (3a250b5: profile model GLM-5.2 -> GLM-5.3, thinking field dropped per
      the zcode-0.16.3 drift classification) landed WITHOUT updating four tests that load the
      repo profile bundle and pin the OLD expectations. Four deterministic failures at HEAD
      (verified 2026-08-15T23Z, working tree clean at 25146f8): cmd/ass-guard
      TestServeAudit_RequestShapedThroughRealSeam ("stored body lost non-secret content" --
      asserts substring "GLM-5.2", body now carries GLM-5.3), internal/profile
      TestLoader_ZcodeProfile ("Model = GLM-5.3, want GLM-5.2"), internal/shaper
      TestShape_ZcodeProfile + TestFidelity_ToolCountAndThinking (thinking-budget assertions
      + GLM-5.2). Root cause is one stale-expectation class, not a functional defect: every
      audit-trail assertion in the e2e test around the failure (TurnID, Profile, Ref,
      fingerprint, transcript canary, mirror, stdout-only-frames) passed.
    artifacts:
      - path: cmd/ass-guard/acp_serve_test.go
        issue: "line ~1806 assertBodyStoreRoundTrip hardcodes \"GLM-5.2\"; re-pinned bundle is GLM-5.3"
      - path: internal/profile/loader_test.go
        issue: "line ~110 TestLoader_ZcodeProfile asserts p.Model == \"GLM-5.2\""
      - path: internal/shaper/shaper_test.go
        issue: "TestShape_ZcodeProfile asserts thinking-budget reproduction + GLM-5.2 against the repo bundle"
      - path: internal/shaper/fidelity_test.go
        issue: "TestFidelity_ToolCountAndThinking asserts thinking budget byte-faithful + GLM-5.2"
    missing:
      - "Re-pin the four model-slug/thinking expectations to the GLM-5.3 bundle (or make them read the bundle's model instead of a literal)"
      - "Re-run `go test ./...` (then `mise ci`) after any profile re-pin -- the 09-04 legs reported green, but these suites were evidently not re-run post 3a250b5"
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
---

# Phase 9: Serve-Path Audit + zcode Parity Re-capture Verification Report

**Phase Goal:** As an operator of a hands-off agent, I want every `acp serve` session to leave a redacted, bounded audit trail that also records the engine's decisions, and the zcode parity stability test re-grounded on a newly pinned capture session, so that I can answer "what did the agent do, and why did it continue" from the log alone and trust that the mimicry hasn't drifted.
**Verified:** 2026-08-15T23:09:17Z
**Status:** gaps_found
**Re-verification:** No — initial verification
**Method:** Goal-backward against the codebase (not SUMMARY claims). Static wiring traces (grep/call-site) + targeted `go test` runs in fresh processes. NOT run per instructions: live parity A/B (needs ZAI_API_KEY; fresh numbers already recorded in the drift report), full race suite, `golangci-lint`. One full-package `go test ./cmd/ass-guard/` run performed to size the CI-red gap.

**Verdict: GOAL largely achieved in substance; one narrow, mechanically fixable gap (red CI from stale post-re-pin test expectations) blocks the phase gate, and one recorded finding (parity re-baseline) awaits operator disposition.**

## User Flow Coverage (MVP mode)

User story: «As an operator of a hands-off agent, I want every `acp serve` session to leave a redacted, bounded audit trail that also records the engine's decisions, and the zcode parity stability test re-grounded on a newly pinned capture session, so that I can answer "what did the agent do, and why did it continue" from the log alone and trust that the mimicry hasn't drifted.»

| Step | Expected | Evidence | Status |
|------|----------|----------|--------|
| Serve a session | A turn on `acp serve` leaves per-session artifacts: transcript + audit mirror under `.ass-guard/audit/` (default ON) | `cmd/ass-guard/acp_serve.go:222-249` (startAuditMirror), `:330-331` (bodyStore + mirror startup); e2e test `TestServeAudit_RequestShapedThroughRealSeam` drives the REAL `runACPServe` over pipes through the REAL factory — mirror-presence + headerNames + canary assertions all passed | ✓ |
| Ask "what did it do" | The transcript carries per-event lines with the correlation triple (session/turn/request) incl. `request_shaped` | `internal/session/transcript.go:63-77` (metadata-only request_shaped: Ref + fingerprint), `internal/session/manager.go:145` (AppendRequestShaped); e2e assertions TurnID/Profile/Ref/Model/Bytes passed | ✓ |
| Ask "why did it continue" | One `engine_decision` line answers it: action + signal + matched span + config source | `internal/event/events.go:123-138` (EngineDecision{Action,Signal,MatchedSpan,ConfigSource,Reason}), `internal/engine/decide.go:41-48`, `internal/engine/observe.go:319` → `internal/session/manager.go:248`; `internal/engine/integration_test.go:96-102` asserts 2 REAL engine_decision transcript lines | ✓ |
| Debug a body on demand | Full redacted body retrievable by hash from a capped store | `internal/audit/bodystore.go` (sha256 content-addressed, redact-inside-Put, 64 MiB cap, oldest-mtime eviction, dedup); `TestBodyStore_PutRetrieveRedacted/Dedup/CapEvictionOldest/UnwritableNeverFatal` PASS | ✓ |
| Trust the mimicry | Stability test green on the newly pinned divergence-prone session; drift reported before the profile moved | `TestStability_WithinSessionExtractionSource` **PASS (0.04s, run by verifier)** consuming `sess_3cee56ae` from `profiles/zcode/coverage.yaml`; commit order 5a2c7f5 (drift) → d3fcfa1 → 3a250b5 (re-pin); parity numbers recorded but FAIL 1/8 (stale expectations, control-proven) — disposition pending | ◐ |

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | AUD-01: single-sourced factory-seam capturer, both protocol shapes; tracer refactored onto the seam; no divergent copies (`tracerProvider` banned) | ✓ VERIFIED | `internal/scheduler/factory.go:142` `BuildWithCapturer` — anthropic attaches `WithAnthropicRequestCapture` (per Stream), openai `WithOpenAIRequestCapture` (per Send); `Build` delegates with nil (`:125`); `grep -rn tracerProvider --include="*.go"` → ZERO matches (deleted everywhere incl. tests); tracer path `cmd/ass-guard/main.go:133` and serve path `cmd/ass-guard/acp_serve.go:348` BOTH construct through the seam |
| 2 | AUD-02: every serve-path session writes a redacted audit trail — correlation IDs (session/turn/request) incl. `RequestShaped` + optional `--audit-log` mirror (0600, append-only JSONL, rejects stdout) | ✓ VERIFIED | request_shaped line carries session-from-filename + TurnID + request=Ref (`internal/session/transcript.go:63`); `audit.OpenFileSink` (`internal/audit/audit.go:46-61`): 0600, O_APPEND, literal `stdout`/`/dev/stdout` targets REJECTED; `NewAuditLogger`/`NewMirrorFile` panic on os.Stdout; `--audit-log` read on serve (`acp_serve.go:209`), default = per-session mirror under `.ass-guard/audit` DEFAULT ON; `TestOpenFileSink`, `TestMirror_StdoutRejection`, `TestMirror_PerSessionRouting`, `TestServeMirror_Override` all PASS; e2e-through-real-seam audit assertions passed (see Gap 1 for the one stale-substring failure in that test) |
| 3 | AUD-03: audit volume bounded via body_ref (hash in event, full body in capped store); write failures loud, never fatal | ✓ VERIFIED | `internal/audit/bodystore.go`: `bodyRef` = sha256 of REDACTED bytes, one helper for line ref AND store key (cannot diverge); 64 MiB default cap, oldest-mtime eviction, content dedup; `Mirror.fail` counts + logs once per distinct error, never blocks (`mirror.go:211-223`); `TestBodyStore_*` (incl. `UnwritableNeverFatal`) + `TestMirror_DropWithCounter` PASS |
| 4 | AUD-04: audit records engine decisions (continue/hook/ask/wait + matched signal) — "why did it continue" answerable from the log alone | ✓ VERIFIED | Full provenance chain: `engine.MatchDetail{ID,Action,Span,ConfigSource}` (`internal/engine/types.go:45`) → `Decide` threads MatchedSpan+ConfigSource into `Decision` (`decide.go:45-46`) → `EngineDecision` event (`events.go:123`) → `Manager.AppendEngineDecision` transcript line (`manager.go:248`) + mirror formats the same fields; `internal/engine/integration_test.go:96-102` proves 2 REAL engine_decision lines in a real transcript; engine + session packages PASS |
| 5 | AUD-05 (machinery legs): stability test green on the newly pinned divergence-prone session; pinned session ID consumed by the test; zcode + extractor versions recorded; drift report committed before any profile update | ✓ VERIFIED | `TestStability_WithinSessionExtractionSource` **PASS — run by this verifier** (0.04s) on `sess_3cee56ae-cc6a-43a6-8f00-a08eb266e1aa` read from `profiles/zcode/coverage.yaml` (`zcode_version: 0.16.3`, `extractor_version: extract-profile/01-02`); the double-prefix filename fix (`stability_test.go:64-68`) proves genuine pinned-file consumption; git order verified: 5a2c7f5 (drift report) → d3fcfa1 (extractor) → 3a250b5 (profile re-pin); `TestStability_CanaryDetectsDivergence` PASS (non-vacuity); runbook `docs/recapture-runbook.md` in-repo; threshold scorecard all-PASS in the drift report (13 records, 81 tools, 1 subagent, catalog 79→80→79 with attach AND detach) |
| 6 | AUD-05 (parity clause): thresholds explicitly re-baselined | ⚠ PRESENT_BEHAVIOR_UNVERIFIED | Fresh numbers recorded in `profiles/zcode/drift-reports/2026-08-16-recapture.md` §"Parity re-baseline": curated FAIL 1/8 IDENTICALLY on old AND new bundles (control run proves profile-independent stale v1.0-era expectations), from-rollout 1/13 with documented harness artifacts (6 empty-expected delta-records, 4 workspace-pollution, 1 by-design-absent runtime MCP tool); Phase-1 baseline marked SUPERSEDED; no silent threshold movement. The plan closed `complete-with-finding` — the acceptance of this re-baseline is an OPERATOR DECISION (see Human Verification) |
| 7 | Phase gate: `mise ci` clean | ✗ FAILED | `.mise.toml` ci = vet+lint+build+`go test -race -count=1 ./...`; 4 deterministic test failures at HEAD 25146f8 (clean tree), all one root cause: stale GLM-5.2/thinking expectations vs the re-pinned GLM-5.3 bundle. See Gaps Summary |

**Score:** 5/7 truths verified (1 present/behavior-unverified, 1 failed)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/scheduler/factory.go` | BuildWithCapturer seam, both shapes | ✓ VERIFIED | Single seam; Build delegates with nil; both capture options attached per shape |
| `internal/audit/audit.go` | shared sink opener, stdout-reject | ✓ VERIFIED | OpenFileSink: 0600/append-only; `stdout`+`/dev/stdout` rejected; NewAuditLogger panics on os.Stdout |
| `internal/audit/bodystore.go` | capped redacted body store | ✓ VERIFIED | sha256-keyed, redact-inside-Put, 64 MiB cap, oldest-eviction, dedup; 5 tests PASS |
| `internal/audit/mirror.go` | per-session JSONL mirror | ✓ VERIFIED | Default-ON dir mode + `--audit-log` single-file mode; drop-with-counter failures; redaction chokepoint |
| `internal/event/events.go` | RequestShaped + EngineDecision events | ✓ VERIFIED | HeaderNames (names only — Pitfall 9); EngineDecision full provenance fields |
| `internal/engine/{types,decide,observe}.go` | MatchDetail → Decision → writer | ✓ VERIFIED | Span + ConfigSource threaded end-to-end; wired to Manager.AppendEngineDecision |
| `internal/session/{transcript,manager,transcript_writer}.go` | metadata-only request_shaped + engine_decision lines; body_ref | ✓ VERIFIED | Correlation triple + fingerprint + Ref; SummarizeRequest → Put on the writer path |
| `cmd/ass-guard/{main,acp_serve}.go` | serve + tracer wiring through the seam; --audit-log | ✓ VERIFIED | runTrace + makeProvider both via BuildWithCapturer; startAuditMirror default ON |
| `internal/profile/stability_test.go` | consumes the pinned session, never PickRichestMain | ✓ VERIFIED | Reads coverage.yaml pin; skips clean when absent; double-prefix bug fixed; PASS on current pin |
| `docs/recapture-runbook.md` | operator procedure (Pitfalls 17/18 verbatim) | ✓ VERIFIED | 5.4 KB; workload, thresholds, pinning procedure, drift-before-update ordering |
| `profiles/zcode/coverage.yaml` | pinned session + versions | ✓ VERIFIED | sess_3cee56ae, zcode_version 0.16.3, extractor extract-profile/01-02 |
| `profiles/zcode/drift-reports/2026-08-16-recapture.md` | drift report + parity re-baseline record | ✓ VERIFIED | 6.6 KB; threshold table, drift classification, parity fresh numbers, follow-ups; committed BEFORE the profile update |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `cmd/ass-guard/acp_serve.go` makeProvider | `internal/scheduler.BuildWithCapturer` | closure at acp_serve.go:348 | ✓ WIRED | serve path constructs through the seam |
| `cmd/ass-guard/main.go` runTrace | `internal/scheduler.BuildWithCapturer` | main.go:133 | ✓ WIRED | tracer path on the same seam (divergent copy gone) |
| `internal/session/transcript_writer.go` | `audit.SummarizeRequest` + `BodyStore.Put` | writer path (transcript_writer.go:41-47) | ✓ WIRED | request_shaped metadata + best-effort store (failure degrades loudly) |
| `cmd/ass-guard/acp_serve.go` startAuditMirror | `audit.NewMirror` / `OpenFileSink` | acp_serve.go:222-249 | ✓ WIRED | default per-session dir; `--audit-log` override; close on ctx done |
| `internal/engine/observe.go` | `Manager.AppendEngineDecision` | observe.go:319 | ✓ WIRED | engine verdict → transcript engine_decision line (integration test asserts real lines) |
| `internal/profile/stability_test.go` | `profiles/zcode/coverage.yaml` pin → rollout file | stability_test.go:48-77 | ✓ WIRED | pinned ID consumed; PASS proves the file was read and extracted |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Within-session stability on the new pin | `go test ./internal/profile/ -run TestStability_WithinSessionExtractionSource -v` | `--- PASS (0.04s)` | ✓ PASS |
| Divergence canary (non-vacuity) | `go test ./internal/profile/ -run TestStability_CanaryDetectsDivergence -v` | `--- PASS` | ✓ PASS |
| Audit package (sink/mirror/bodystore) | `go test ./internal/audit/` | `ok ... 0.798s` | ✓ PASS |
| Serve audit e2e through the REAL seam | `go test ./cmd/ass-guard/ -run TestServeAudit_RequestShapedThroughRealSeam -v` | `--- FAIL` at acp_serve_test.go:1613 | ✗ FAIL (stale "GLM-5.2" substring; ALL other audit assertions in the same test passed — TurnID/Profile/Ref/fingerprint/transcript-canary/mirror/stdout-frames) |
| Mirror override | `go test ./cmd/ass-guard/ -run TestServeMirror_Override -v` | `--- PASS` | ✓ PASS |
| Engine + session + openspec packages | `go test ./internal/session/ ./internal/engine/ ./internal/openspec/` | all `ok` | ✓ PASS |
| Profile loader against repo bundle | `go test ./internal/profile/` | `FAIL: TestLoader_ZcodeProfile ("Model = GLM-5.3, want GLM-5.2")` | ✗ FAIL |
| Shaper against repo bundle | `go test ./internal/shaper/` | `FAIL: TestShape_ZcodeProfile, TestFidelity_ToolCountAndThinking` | ✗ FAIL |
| Full cmd package (one run) | `go test ./cmd/ass-guard/` | single failure: TestServeAudit_RequestShapedThroughRealSeam | ✗ FAIL |
| Secret canary grep over committed artifacts | `grep -rn "sk-…|Bearer …" profiles/ docs/ phase dir` | zero matches | ✓ PASS |

### Probe Execution

No `scripts/*/tests/probe-*.sh` probes declared by this phase's plans; not a migration/tooling phase. Step 7c: SKIPPED (no probes declared).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|---------------------|----------|
| AUD-01 | 09-01 | single-sourced factory-seam capturer both shapes | ✓ SATISFIED | factory.go:142; tracerProvider zero matches; both paths on the seam |
| AUD-02 | 09-01, 09-05, 09-06 | redacted serve-path audit trail + `--audit-log` mirror | ✓ SATISFIED | transcript/mirror/bodystore wiring + passing unit tests + e2e audit assertions (one stale-substring failure cross-listed under the CI gap) |
| AUD-03 | 09-05 | body_ref bounding + loud-never-fatal | ✓ SATISFIED | bodystore.go + TestBodyStore_UnwritableNeverFatal + Mirror.drop-with-counter |
| AUD-04 | 09-02 | engine decisions with matched signal in the log | ✓ SATISFIED | MatchDetail→Decision→event→transcript chain; engine integration test asserts real lines |
| AUD-05 | 09-03, 09-04 | stability green on newly pinned runbook-produced session, pin consumed, versions recorded, thresholds re-baselined, drift-before-update | ◐ PARTIAL | machinery/capture/pin/stability/ordering VERIFIED (verifier-run test PASS); parity re-baseline recorded-with-finding pending operator disposition; capture produced via operator-directed app-server stdio driver (documented deviation from the runbook's interactive procedure — substance preserved: scripted, divergence-prone, threshold scorecard applied, NOT richest-session selection) |

No orphaned requirements: REQUIREMENTS.md maps exactly AUD-01..05 to Phase 9, all claimed by plans.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| cmd/ass-guard/acp_serve_test.go | ~1806 | stale pinned expectation "GLM-5.2" vs re-pinned GLM-5.3 bundle | 🛑 Blocker (breaks `mise ci` test leg) | e2e audit proof red at HEAD |
| internal/profile/loader_test.go | ~110 | stale pinned expectation "GLM-5.2" | 🛑 Blocker (same class) | loader test red at HEAD |
| internal/shaper/shaper_test.go | 139,147 | stale thinking-budget + GLM-5.2 expectations | 🛑 Blocker (same class) | shaper tests red at HEAD |
| internal/shaper/fidelity_test.go | 100,107 | stale thinking-budget + model expectations | 🛑 Blocker (same class) | fidelity test red at HEAD |

No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER debt markers in any phase-modified file scanned (internal/audit/*, internal/session/{transcript,manager,transcript_writer}.go, internal/scheduler/factory.go, internal/engine/{types,decide,observe}.go, internal/event/events.go, internal/profile/stability_test.go, cmd/ass-guard/{main,acp_serve}.go, docs/recapture-runbook.md).

### Human Verification Required

**1. Parity re-baseline disposition (AUD-05 clause — the plan's own recorded finding)**
**Test:** Read `profiles/zcode/drift-reports/2026-08-16-recapture.md` §"Parity re-baseline" and decide.
**Expected:** Explicit operator acceptance (fresh numbers = new recorded reference; schedule follow-ups: re-record curated expectations from current live zcode; `ExtractTurnsFromRollout` delta-record handling + per-turn workspace fixtures) or rejection with a re-opened leg.
**Why human:** An acceptance decision on a control-experiment finding; no automated check can make it. Note: Phase 12 SC-8 (behavioral-eval regression net, re-run gate on profile/model changes) is adjacent but does NOT explicitly cover the zcode-parity curated-suite re-recording — conservatively NOT deferred.

**2. Live-serve redacted-audit witness (phase-gate leg)**
**Test:** Run a real `ass-guard acp serve` against the live provider; observe a redacted `request_shaped` line + an `engine_decision` line in `.ass-guard/audit/<session>.jsonl`.
**Expected:** Redacted lines present; no credential material anywhere in the artifact tree.
**Why human:** The in-process e2e (`TestServeAudit_RequestShapedThroughRealSeam`) proves the machinery through the REAL factory against an httptest SSE stub — all its audit assertions passed — but 09-06-SUMMARY records the operator witness as "pending morning" and no witness record was found in STATE.md. Requires ZAI_API_KEY; excluded from this verification per instructions.

**3. Capture-method deviation acceptance (optional, to make the record airtight)**
**Test:** Confirm the operator-directed app-server stdio driver capture (documented in the drift report §"Capture method") stands in place of the runbook's interactive procedure.
**Expected:** Either accept as-is (drift report + STATE.md already record the operator direction) or add a formal override. Suggested override if formalized:

```yaml
overrides:
  - must_have: "capture session produced via the operator runbook"
    reason: "Operator directed the app-server stdio driver on 2026-08-16 (premise 'no CLI/API path' disproven); workload stayed scripted + divergence-prone, runbook §3 thresholds applied, not richest-session selection"
    accepted_by: "operator"
    accepted_at: "2026-08-16T00:00:00Z"
```

**4. User-story format discrepancy (informational).** `gsd-tools query user-story.validate` returns `valid: false` for the ROADMAP goal — the canonical regex requires literal "As a " / ", I want to " but the goal reads "As **an** operator" and "I want **every … session to** leave". Substantively a user story (role/capability/outcome all present); verification proceeded against the obvious slots. Operator may reformat via `/gsd mvp-phase 9` or accept as-is.

### Gaps Summary

One gap, one root cause, four symptoms: **the AUD-05 profile re-pin (3a250b5) landed without refreshing the pinned test expectations that consume the repo profile bundle.** `mise ci`'s test leg is red at HEAD (25146f8, clean tree) via four deterministic failures — `cmd/ass-guard TestServeAudit_RequestShapedThroughRealSeam`, `internal/profile TestLoader_ZcodeProfile`, `internal/shaper TestShape_ZcodeProfile` + `TestFidelity_ToolCountAndThinking` — all asserting the OLD bundle's GLM-5.2 model slug / thinking budget against the NEW GLM-5.3 / no-thinking bundle the drift report itself classified as zcode-changed. This falsifies the phase-gate leg "`mise ci` clean" and shows the 09-04 "stability green" claim was made without re-running the affected suites post-re-pin. The fix is mechanical (update four expectations, or derive them from the bundle); no functional audit/parity defect is implicated — every audit-behavior assertion inside the failing e2e test passed around the stale substring.

Everything else the goal promised is verified in code and by verifier-run tests: the single factory seam (tracer copy deleted), the redacted default-ON audit mirror with correlation triples, the body_ref bounding with capped retrievable store, the engine-decision provenance line ("why did it continue" answerable from the log), and the stability test green on the newly pinned divergence-prone session with drift-before-update ordering intact. The parity re-baseline remains a recorded finding awaiting the operator's disposition, exactly as the plan closed it.

---

_Verified: 2026-08-15T23:09:17Z_
_Verifier: Claude (gsd-verifier)_
