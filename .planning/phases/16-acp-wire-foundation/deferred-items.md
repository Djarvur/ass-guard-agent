# Phase 16 Deferred Items

| Item | Source | Status | Deferred At | Owner |
|------|--------|--------|-------------|-------|
| `TestAskPark_PromptResponsePrecedesResolution` (internal/runtime, Phase 12) tripped its 10s "near-instant" bound at 11.6s under one fully parallel `mise ci` run (2026-08-27, 16-01 execution) — scheduling delay under load, not semantics (the resolution path it guards against is a 1-hour timer). Passes standalone and in subsequent full-CI runs. Pre-existing test, out of 16-01 scope per the deviation scope boundary. | 16-01 execution (SUMMARY §Issues Encountered) | Open — verifier to consider a bound raise (10s→30s keeps the assertion's power) | 2026-08-27 | Phase 16 verifier / test-robustness pass |
| `TestServeMirror_Override` (internal/acpserve, Phase 09) failed TempDir RemoveAll cleanup (`.ass-guard: directory not empty`) in one full-parallel `mise ci` after 16-05 (2026-08-27) — leaked background writer racing test cleanup under load. Full `go test -race ./...` re-run immediately after: zero failures. Pre-existing test, untouched by Phase 16 plans; same leaked-goroutine family as the 16-01 ask-park flake. | Wave 3 post-merge gate (orchestrator) | Open — verifier to consider waiting for serve goroutines to exit before test return | 2026-08-27 | Phase 16 verifier / test-robustness pass |
| Pre-stamp Model chip untruthful (16-06 operator checkpoint FINDING, recorded not fixed): the runner's wire model defaults to the profile's own slug (`internal/runtime/runtime.go:131` — `effectiveModel ""` = profile model, mimicry parity), but the ACP-08 Model option advertises the tier-resolved value (`internal/acpserve/config_surface.go:320` `resolveModelLocked`; `effectiveFor` :398 shares it) — turn-001 went out GLM-5.3 while the chip showed glm-5.2 (live evidence: `.ass-guard/transcript_b7fd0737-*.jsonl` turn-001 vs turn-002, where the operator's UI switch converged both on GLM-5.3). Divergence appears only when the project tier model ≠ profile slug; post-switch the two converge. Operator kept GLM-5.3 as the project value. | 16-06 operator checkpoint | Open — verifier decides gap-closure vs defer (severity against 16-05 must_haves and ROADMAP criterion 4) | 2026-08-27 | Phase 16 verifier |

## 2026-09-03 (found during 18-05): pre-existing flake in session TestGateOutcomeMatrix

`TestGateOutcomeMatrix/allow_once...` (internal/session/gate_test.go:~1084,
"allow_once result = []; want exactly one non-error result") fails ~1-in-10
runs under `-race` — REPRODUCED at the pre-18-05 commit 628f7cd (worktree
loop, 1/10 FAIL), so it predates this phase's changes (18-05 touched
SeedResume/PlanModeState/reconcile delegation only — none on this path).
Looks like a resume-timing race in the test's wait for the tool result after
the dialog answer. Out of scope for 18-05 (scope boundary); recorded for a
dedicated fix pass.
