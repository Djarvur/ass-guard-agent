---
phase: 25
slug: seed-001-kit-extraction-strictly-last
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-08-28
---

# Phase 25 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Populated 2026-08-28 from 25-RESEARCH.md §Validation Architecture + the nine phase plans.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard testing + `-race`; testify in some suites; orchestrated via mise |
| **Config file** | `.mise.toml` (`[tasks.ci]` = vet → lint → build → test; `eval-check-changed` → `eval-gate`; `kit-boundary` joins ci in plan 25-09) |
| **Quick run command** | `go build ./... && go vet ./... && go test ./kit/... ./internal/acpserve/... ./cmd/ass-guard/ -count=1` (touched-package focus per task) |
| **Full suite command** | `mise ci` (+ `mise eval-check-changed` → `mise eval-gate` when the detector fires, ~8-10 min) |
| **Estimated runtime** | quick ~40-60s; `mise ci` ~2-4 min; eval-gate ~8-10 min |

---

## Sampling Rate

- **After every task commit:** Run the quick command scoped to touched packages
- **After every plan wave:** Run `mise ci` — the D-20 equivalence proof at each boundary
- **Before `/gsd-verify-work`:** Full battery green (mise ci + ledger sum + CLI golden + zeroconfig + eval-gate) — plan 25-09 Task 2 records it
- **Max feedback latency:** ~60s (quick command); eval-gate is gated behind the change-class detector, not every commit

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 25-01-01 | 01 | 1 | KIT-01 | T-25-04 | one-way boundary confirmed before first path | checkpoint | (blocking decision — no automated) | — | ⬜ pending |
| 25-01-02 | 01 | 1 | KIT-03 (D-20) | T-25-01 | eval detector recognizes kit paths, legacy retained | script selftest | `./scripts/eval-change-class.sh --selftest` | ✅ | ⬜ pending |
| 25-01-03 | 01 | 1 | KIT-01 | T-25-02/03 | verbatim rank-0 move, gate green | gate suite + color-moved review | `mise ci && go test ./kit/... -count=1` | ✅ | ⬜ pending |
| 25-02-01 | 02 | 2 | KIT-01 | T-25-05 | embed payloads travel (hookdag/toolcat) | gate suite + package tests | `mise ci && go test ./kit/hookdag/ ./kit/toolcat/ ./kit/audit/ ./kit/shaper/ -count=1` | ✅ | ⬜ pending |
| 25-02-02 | 02 | 2 | KIT-01 | T-25-05/07/08 | embeds + edge re-audit (provider/mcp/toolexec/modelrouting) | gate suite + package tests | `mise ci && go test ./kit/modelrouting/ ./kit/provider/ ./kit/mcp/ ./kit/toolexec/ -count=1` | ✅ | ⬜ pending |
| 25-03-01 | 03 | 3 | KIT-01 | T-25-11 | session/engine move verbatim (ecosys edge expected) | gate suite | `mise ci && go test ./kit/session/ ./kit/engine/ -count=1` | ✅ | ⬜ pending |
| 25-03-02 | 03 | 3 | KIT-01 | T-25-09 | kit core move (runtime + enginebridge→kit/internal) | gate suite | `mise ci && go test ./kit/runtime/... -count=1` | ✅ | ⬜ pending |
| 25-03-03 | 03 | 3 | KIT-03 (D-20/D-06) | T-25-12 | ledger sum == baseline; CLI golden; detector fires | battery | `mise ci && go test ./cmd/ass-guard/ -run 'TestCLIBinaryContract\|TestZeroConfigFirstRun' -count=1 && mise eval-check-changed` (exit 3) | ✅ | ⬜ pending |
| 25-04-01 | 04 | 4 | KIT-02 | T-25-14 | kit/runtime zero acp vocabulary | import-graph + relocated suites | `go list -deps ./kit/runtime \| grep -c "ass-guard-agent/internal/acp$"` == 0 + `go test ./kit/runtime/ ./kit/session/` | ✅ (relocated) | ⬜ pending |
| 25-04-02 | 04 | 4 | KIT-02 | T-25-15/16/18 | frozen TurnRunner; order preserved; frames ordered | relocated e2e + serve tests | `mise ci && go test ./internal/acpserve/... ./kit/runtime/ ./cmd/ass-guard/ -count=1` | ✅ (relocated) | ⬜ pending |
| 25-04-03 | 04 | 4 | KIT-02 | T-25-17 | single ask-timeout owner; capture-shaped non-answer | relocated ask batteries | `go test ./kit/session/ ./internal/acpserve/... -run 'Ask\|ask' -count=1` | ✅ (relocated) | ⬜ pending |
| 25-05-01 | 05 | 5 | KIT-01 (D-16) | T-25-20/21 | port == 4 call sites; json tags verbatim | cron + YAML round-trip suites | `go list -deps ./kit/runtime \| grep -c "internal/sched$"` == 0 + `go test ./kit/runtime/ ./internal/sched/` | ✅ (relocated) | ⬜ pending |
| 25-05-02 | 05 | 5 | KIT-01 | T-25-22 | LearnedStore port, (string, bool), nil→ErrAskPending | relocated enginebridge tests | `go list -deps ./kit/internal/enginebridge \| grep -c "internal/learning$"` == 0 + package tests | ✅ (relocated) | ⬜ pending |
| 25-05-03 | 05 | 5 | KIT-01/03 (OQ4) | T-25-23 | SetupEngine inputs; degradation arms identical | relocated engine wiring tests | `go list -deps ./kit/runtime \| grep -c "internal/openspec$"` + `mise ci` | ✅ (relocated) | ⬜ pending |
| 25-06-01 | 06 | 6 | KIT-02/03 (D-17) | T-25-27 | kit/session zero ecosys vocabulary | relocated subagent/hooks batteries | `go list -deps ./kit/session \| grep -c "internal/ecosys$"` == 0 + `go test ./kit/session/` | ✅ (relocated) | ⬜ pending |
| 25-06-02 | 06 | 6 | KIT-01/03 (D-17) | T-25-25/26/28 | catalog injection; degradation arm preserved | relocated wiring batteries | `go list -deps ./kit/runtime \| grep -cE "internal/(ecosys\|openspec)$"` == 0 + `mise ci` | ✅ (relocated) | ⬜ pending |
| 25-07-01 | 07 | 7 | KIT-01 (OQ1) | T-25-31/33 | coarse Attach + Reaper; nil→stub-exec | relocated wiring tests | `go test ./kit/runtime/ -count=1` | ✅ (relocated) | ⬜ pending |
| 25-07-02 | 07 | 7 | KIT-01/03 | T-25-30/32 | last edge severed; kit/ non-test import-clean | gate suite + import sweep | `go list -deps ./kit/runtime \| grep -c "internal/coreexec$"` == 0 + `mise ci` | ✅ | ⬜ pending |
| 25-08-01 | 08 | 8 | KIT-01/03 | T-25-35 | subject-split moves conserve functions | moved test suites | `go test ./internal/acpserve/... -count=1` | ❌ W0 (25-08 creates homes) | ⬜ pending |
| 25-08-02 | 08 | 8 | KIT-01 | T-25-36/38 | kit fakes replace app fixtures; assertions preserved | in-kit suites | `go test ./kit/runtime/ -count=1 && go vet ./kit/runtime/` | ❌ W0 | ⬜ pending |
| 25-08-03 | 08 | 8 | KIT-01/03 | T-25-35/37 | final sweep; ledger sum == baseline | full kit suite + ledger | `go test ./kit/... -count=1` + sum check | ❌ W0 | ⬜ pending |
| 25-09-01 | 09 | 9 | KIT-01/02 (D-19/D-08) | T-25-40/41 | gates red-on-violation proven; hostproof green | gate + hostproof test | `mise kit-boundary && go test ./kit/runtime/ -run TestKitHostsNonACPFrontend -count=1` | ❌ W0 (25-09 creates) | ⬜ pending |
| 25-09-02 | 09 | 9 | KIT-03 (D-20) | T-25-39/42 | full equivalence battery incl. eval-gate | battery | `mise ci && MERGE_BASE=... mise eval-check-changed` (exit 3) `&& mise eval-gate` | ✅ (task exists) | ⬜ pending |
| 25-09-03 | 09 | 9 | KIT-03 | T-25-43 | live-Zed equivalence (criterion #3 manual leg) | manual checkpoint | (checkpoint:human-verify — blocking) | — | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `scripts/eval-change-class.sh` CLASSES extended with kit/ equivalents + selftest fixtures — plan 25-01 Task 2 (Pitfall 1: the detector must recognize kit paths BEFORE the risky moves land)
- [x] `.planning/phases/25-.../test-ledger.txt` pre-move baseline — plan 25-01 Task 2 (D-20 sum-equality instrument)
- [ ] `kit/runtime/hostproof_test.go` — plan 25-09 Task 1 (deliberately NOT wave 1: it requires the 25-04..25-07 seams and the nil-degradation paths to exist; a wave-1 skeleton could not compile — the gate-after-seams ordering, D-03)
- [ ] `.mise.toml` kit-boundary task + `.golangci.yml` KitBoundary rule — plan 25-09 Task 1 (deliberately last: enabling during pass 1 fails the build by construction — kit still imports internal until 25-07 severs the last edge)

*Deviation note: the research's Wave-0 gap list places the hostproof skeleton and gate wiring early; this plan set orders them after seam completion per D-03 (equivalence and design never mix; the gate enabled early fails by construction). The eval extension + ledger ARE wave 1 because they must precede the risky moves.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live Zed session unchanged (streams, tools, diffs, asks, replay) | KIT-03 (criterion #3) | Requires editor + live credentials; the 15-07/16-06 precedent | Plan 25-09 Task 3 checkpoint: operator drives their daily-use session against the freshly built binary and compares against prior close criteria |
| Pass-1 pure-move review | KIT-01 (D-03) | Reviewer judgment on color-moved output | `git diff --color-moved=dimmed-zebra master...HEAD` — relocated blocks, no body edits |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or are checkpoint tasks (2 of 26: 25-01-01 decision, 25-09-03 human-verify)
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (hostproof + gates scheduled at their compilable position, deviation noted)
- [x] No watch-mode flags
- [x] Feedback latency: quick command ~40-60s (documented; the repo's race-enabled full gate is the per-wave, not per-task, instrument)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-08-28 (planner — populated from 25-RESEARCH.md §Validation Architecture test map)
