---
phase: 18
slug: session-family
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: true
wave_0_complete: true
created: 2026-08-27
---

# Phase 18 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (Go 1.25, module-local); kill -9 rows use the re-exec child pattern (os.Args[0] + env var) |
| **Config file** | .mise.toml (ci task = the standing gate: full test run with -race) |
| **Quick run command** | `go test -race -count=1 <touched package(s)>` |
| **Full suite command** | `mise ci` |
| **Estimated runtime** | ~25 s quick (kill -9 matrix alone <= 60 s with -timeout 180s), ~3 min full |

---

## Sampling Rate

- **After every task commit:** Run the task's `<automated>` command (each PLAN task carries one — package-scoped, seconds)
- **After every plan wave:** Run `mise ci`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 60 s (kill -9 matrix is the ceiling; unit tests run in single-digit seconds)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 18-01-01 | 01 | 1 | ACP-06 | T-18-01/02/03 | replay gate: no prompt accepted mid-replay; zero provider calls on resume | unit (TDD RED/GREEN) | `go test -race -count=1 ./internal/acp/ -run 'TestSessionLoad\|TestLoadGate'` | ✅ (plan creates) | ⬜ pending |
| 18-01-02 | 01 | 1 | ACP-06 | T-18-02 | replay mapping: agent-visible kinds only, closures render as terminal frames | unit | `go test -race -count=1 ./internal/acp/ -run TestReplayLine` | ✅ (plan creates) | ⬜ pending |
| 18-02-01 | 02 | 1 | ACP-06 | T-18-04 | classification: every inventory row → its exact closure + seed; idempotent | unit (TDD) | `go test -race -count=1 ./internal/session/ -run TestReconcile` | ✅ (plan creates) | ⬜ pending |
| 18-02-02 | 02 | 1 | ACP-06 | T-18-04/05 | closures append-only through redaction seam; no sidecar | unit | `go test -race -count=1 ./internal/session/ && go vet ./internal/session/` | ✅ (plan creates) | ⬜ pending |
| 18-03-01 | 03 | 1 | ACP-05 | T-18-06 | ordering/pagination/cursor contracts encoded RED | unit (TDD) | `go test -race -count=1 ./internal/session/ -run TestSessionList` | ✅ (plan creates) | ⬜ pending |
| 18-03-02 | 03 | 1 | ACP-05 | T-18-06/07/08 | bounded scan, composite cursor, stat-only tombstone filter | unit | `go test -race -count=1 ./internal/session/ && go vet ./internal/session/` | ✅ (plan creates) | ⬜ pending |
| 18-04-01 | 04 | 2 | ACP-05 | T-18-02 | list RPC + capability advertisement shape | unit | `go test -race -count=1 ./internal/acp/ -run TestSessionList` | ✅ (plan creates) | ⬜ pending |
| 18-04-02 | 04 | 2 | ACP-07 | T-18-09 | close = cancel-and-drain, idempotent; delete tombstones + sweeps checkpoints | unit | `go test -race -count=1 ./internal/acp/ -run 'TestSessionClose\|TestSessionDelete\|TestTombstone'` | ✅ (plan creates) | ⬜ pending |
| 18-04-03 | 04 | 2 | ACP-07 | T-18-03/10 | grace-before-purge sweep, config key, startup wiring | unit | `go test -race -count=1 ./internal/acpserve/ && go test -race -count=1 ./internal/session/ -run TestSweep` | ✅ (plan creates) | ⬜ pending |
| 18-05-01 | 05 | 2 | ACP-06 | T-18-04/11 | reconcile-append-seed before replay; modes; double-registration | unit (TDD) | `go test -race -count=1 ./internal/acp/ -run 'TestLoad\|TestResume\|TestSessionLoad' && go test -race -count=1 ./internal/runtime/ -run TestResumeSession` | ✅ (plan creates) | ⬜ pending |
| 18-05-02 | 05 | 2 | ACP-06 | T-18-04/11/12 | kill -9 matrix E2E: real SIGKILL, resume, four checks/row | E2E (real process) | `go test -race -count=1 -timeout 180s ./internal/acpserve/ -run TestKill9Resume` | ✅ (plan creates) | ⬜ pending |
| 18-06-01 | 06 | 3 | ACP-06 | T-18-13 | flag trio parsing + cwd-scoped resolution + combination errors | unit (TDD) | `go test -race -count=1 ./cmd/ass-guard/ -run 'TestResumeFlag\|TestContinueResolves\|TestResumeTarget'` | ✅ (plan creates) | ⬜ pending |
| 18-06-02 | 06 | 3 | ACP-06 | T-18-14 | picker: pipe-safe, retry-once, truncation, relative time | unit (TDD) | `go test -race -count=1 ./cmd/ass-guard/ -run TestPicker` | ✅ (plan creates) | ⬜ pending |
| 18-06-03 | 06 | 3 | ACP-06 | T-18-13 | ResumeTarget loads before Serve; loud failure; noop when empty | unit (TDD) | `go test -race -count=1 ./internal/acpserve/ -run TestResumeTarget && mise build` | ✅ (plan creates) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements — go test + -race + .mise.toml are the standing Phase 15/16 gate; every test file is created by its own plan (TDD RED step). No Wave 0 needed.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Session picker in Zed shows listed sessions, cursor pagination works live, full conversation replays through ordered frames | ACP-05/06 | Real-editor surface; the 16-06 simulator covers protocol shape but not the human eye | Open Zed with the ass-guard ACP agent configured; open the session picker; page through; open one session; watch the replay |
| CLI trio in a real terminal (and over ssh/pipe) | ACP-06 | TTY/pipe ergonomics judged by hand; unit tests prove pipe-safety mechanically | Run `ass-guard --resume` (picker), `ass-guard --resume <id>`, `ass-guard -c`; repeat the picker over `ssh localhost` |
| Deleted session remains investigable on disk | ACP-07 | Audit-invariant observation across process boundaries | Delete a session via session/delete; confirm it vanished from list; inspect the tombstoned transcript + audit log on disk |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies (all 14 tasks carry package-scoped automated commands; all test files created by their plans)
- [x] Sampling continuity: no 3 consecutive tasks without automated verify (every task has one)
- [x] Wave 0 covers all MISSING references (none MISSING)
- [x] No watch-mode flags (plain `go test -race -count=1` everywhere)
- [x] Feedback latency < 60 s (kill -9 matrix ceiling 60 s; all other tasks < 15 s)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending (validate-phase sets validated)
