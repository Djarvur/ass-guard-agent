---
phase: 23
slug: seed-gaps-close-out
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-08-28
---

# Phase 23 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` with `-race` (go 1.26 via mise); testify available in go.mod |
| **Config file** | none — convention `*_test.go` beside subjects; gate tasks in `.mise.toml` |
| **Quick run command** | `go test -race -count=1 ./internal/session/ ./internal/runtime/ ./internal/checkpoint/ ./internal/acpserve/` |
| **Full suite command** | `mise ci` (vet + lint + build + test) |
| **Estimated runtime** | ~30 seconds per-task `-run` battery; full suite per RESEARCH Validation Architecture |

---

## Sampling Rate

- **After every task commit:** Run the task's `-run` battery (each < 30s)
- **After every plan wave:** Run `go test -race -count=1 ./internal/session/ ./internal/runtime/ ./internal/checkpoint/ ./internal/acpserve/`
- **Before `/gsd-verify-work`:** `mise ci` must be green (RESEARCH phase gate)
- **Max feedback latency:** ~30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 23-01-T1 | 01 | 1 | SEEDG-01 | T-23-01/03 | Steering delivers only at the boundary as marker-wrapped user speech; REDACTED append path; pairs intact | integration (tracer) | `go test -race -count=1 ./internal/session/ -run 'TestSteeringDelivery' -v` | ❌ W0 | ⬜ pending |
| 23-01-T2 | 01 | 1 | SEEDG-01 | T-23-02 | Ticket conservation under cancel races; queue import-block free of wire packages | unit (-race) | `go test -race -count=1 ./internal/session/ -run 'TestSteerQueue' -v` | ❌ W0 | ⬜ pending |
| 23-01-T3 | 01 | 1 | SEEDG-01 | T-23-03 | Anchor safety + live/replay parity for steering_delivery folds | unit | `go test -race -count=1 ./internal/session/ -run 'TestSteeringProject|TestSteeringReplay' -v` | ❌ W0 | ⬜ pending |
| 23-02-T1 | 02 | 2 | SEEDG-01 | T-23-04 | Pre-mutex classification; steered prompt returns while turn blocked (anti-queue-behind) | integration (-race) | `go test -race -count=1 ./internal/runtime/ -run 'TestSteerIngress' -v` | ❌ W0 | ⬜ pending |
| 23-02-T2 | 02 | 2 | SEEDG-01 | T-23-05 | Parked ask visible + cancelable; cancel never kills the running turn | integration (-race) | `go test -race -count=1 ./internal/runtime/ ./internal/session/ -run 'TestParkedAsk' -v` | ❌ W0 | ⬜ pending |
| 23-02-T3 | 02 | 2 | SEEDG-01 | T-23-06 | Combined steer+ask+cancel scenario; engine-chain steered E2E | integration (-race) | `go test -race -count=1 ./internal/runtime/ -run 'TestCombinedSteerScenario|TestEngineChainSteered' -v` | ❌ W0 | ⬜ pending |
| 23-03-T1 | 03 | 1 | SEEDG-02 | T-23-07 | Pre-restore id family accepted at all three grammar sites; fail-closed error surface | unit | `go test -count=1 ./internal/checkpoint/ -run 'TestPreRestore|TestIDGrammar' -v` | ❌ W0 | ⬜ pending |
| 23-03-T2 | 03 | 1 | SEEDG-02 | T-23-08 | Nested-repo refusal incl. the .git-FILE worktree variant; clean never descends | unit | `go test -count=1 ./internal/checkpoint/ -run 'TestNestedRepo' -v` | ❌ W0 | ⬜ pending |
| 23-03-T3 | 03 | 1 | SEEDG-02 | T-23-09/10 | Dual-axis GC + object expiry; idempotent exclude append; check-ignore fires | unit | `go test -count=1 ./internal/checkpoint/ -run 'TestCheckpointGC|TestUserRepoExclude' -v` | ❌ W0 | ⬜ pending |
| 23-04-T1 | 04 | 3 | SEEDG-02 | T-23-12 | Guard refuses active turn/parked chain (non-blocking check); session-start sweep; nil-store loud | integration (-race) | `go test -race -count=1 ./internal/runtime/ -run 'TestRestoreGuard|TestSessionStartSweep' -v` | ❌ W0 | ⬜ pending |
| 23-04-T2 | 04 | 3 | SEEDG-02 | T-23-13 | checkpoint.* menu selects; membership-validated Set; persist + generic-map read-back | integration (-race) | `go test -race -count=1 ./internal/acpserve/ -run 'TestCheckpointOptions' -v` | ❌ W0 | ⬜ pending |
| 23-04-T3 | 04 | 3 | SEEDG-02 | T-23-12 | Refusal matrix (8 cells); config-to-GC bounds; wave gate | integration (-race) | `go test -race -count=1 ./internal/session/ ./internal/runtime/ ./internal/checkpoint/ ./internal/acpserve/` | ✅ (suite) | ⬜ pending |
| 23-05-T1 | 05 | 4 | SEEDG-03 | T-23-15/18 | /undo zero model turns; D-05 shape; D-11 walk; local_command verbatim | integration (-race) | `go test -race -count=1 ./internal/runtime/ -run 'TestClassB.*Undo|TestUndoWalk' -v` | ❌ W0 | ⬜ pending |
| 23-05-T2 | 05 | 4 | SEEDG-03 | T-23-16/17 | Auto-cancel-then-restore ordering (snapshot first); fail-closed; nested refusal outranks cancel | integration (-race) | `go test -race -count=1 ./internal/runtime/ -run 'TestUndoAutoCancel|TestUndoFailClosed|TestUndoNestedRefusal' -v` | ❌ W0 | ⬜ pending |
| 23-05-T3 | 05 | 4 | SEEDG-01/03 | — | Live-Zed UAT: steering mid-turn + /undo incl. auto-cancel path | manual (operator) | checkpoint:human-verify (23-05 Task 3) | n/a | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/session/steerqueue_test.go` — SteerQueue battery incl. -race producer/cancel hammer + import-block gate (SEEDG-01)
- [ ] `internal/session/steering_test.go` — boundary-drain E2E, Projector anchor/replay tests, parked-ask tests (SEEDG-01)
- [ ] `internal/runtime/steering_ingress_test.go` — pre-mutex classifier, combined scenario, engine-chain E2E (SEEDG-01)
- [ ] `internal/checkpoint/store_test.go` extensions — pre-restore family, grammar table, nested-repo fixture (dir + .git FILE), GC, exclude (SEEDG-02)
- [ ] `internal/runtime/restore_guard_test.go` — guard matrix, session-start sweep, config-to-GC (SEEDG-02)
- [ ] `internal/runtime/commands_test.go` — /undo battery (joins the TestClassB family; existence governed by the 23-05 Task 1 precondition — HALT if Phase 20 machinery is absent, do not scaffold it here) (SEEDG-03)
- [ ] No framework install needed — existing `go test` infrastructure covers all phase requirements

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live-Zed steering delivery + /undo (incl. mid-turn auto-cancel) | SEEDG-01, SEEDG-03 | Requires a real Zed ACP session (client behavior under concurrent prompts is RESEARCH assumption A2 — may queue client-side; the record itself is the deliverable) | 23-05 Task 3 checkpoint:human-verify — launch `ass-guard acp` in Zed on a scratch git repo, steer mid-turn, /undo idle and mid-turn, check ACP logs for malformed frames |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 30s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-08-28 (planner — populated from 23-RESEARCH.md Validation Architecture test map; wave_0_complete flips at execution)
