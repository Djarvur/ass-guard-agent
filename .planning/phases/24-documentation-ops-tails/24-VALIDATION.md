---
phase: 24
slug: documentation-ops-tails
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-08-28
---

# Phase 24 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Populated from 24-RESEARCH.md "Validation Architecture" test map (2026-08-28).

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + race detector (repo-wide convention; no testify) |
| **Config file** | `.mise.toml` (tasks); no test-framework config file |
| **Quick run command** | `go test -race -count=1 ./internal/modelrouting/ ./internal/session/ ./internal/acpserve/ ./internal/runtime/ ./internal/profilecheckcmd/` |
| **Full suite command** | `mise test` (= `go test -race -count=1 ./...`) |
| **Estimated runtime** | ~24 seconds (per-task package-scoped runs; full suite minutes) |

---

## Sampling Rate

- **After every task commit:** Run the quick run command (package-scoped)
- **After every plan wave:** Run `mise test`
- **Before `/gsd-verify-work`:** `mise ci` must be green (vet + lint + build + test)
- **Max feedback latency:** ~24 seconds (package-scoped)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 24-01-01 | 01 | 1 | TAIL-01 | T-24-01-01 | Record schema carries mechanical fields only (D-04) | unit (RED) | `go test -race -count=1 ./internal/modelrouting/ -run 'TestOutcome\|TestFirstAllowed'` (must fail) | ❌ W0 | ⬜ pending |
| 24-01-02 | 01 | 1 | TAIL-01 | T-24-01-02/03 | Tolerant read, perm 0600/0750, seam replay (D-07) | unit (GREEN) | `go test -race -count=1 ./internal/modelrouting/` | ❌ W0 | ⬜ pending |
| 24-01-03 | 01 | 1 | TAIL-01 | — | Standing gate | gate | `mise ci` | ✅ | ⬜ pending |
| 24-02-01 | 02 | 2 | TAIL-01 | T-24-02-01/02 | Live record path; store failure never fails a turn | unit (tracer) | `go test -race -count=1 ./internal/session/ -run TestSessionOutcomeRecording` | ❌ W0 | ⬜ pending |
| 24-02-02 | 02 | 2 | TAIL-01 | T-24-02-04 | Stats CLI transport discipline (stderr default, --json stdout) | unit | `go test -count=1 ./cmd/ass-guard/ -run TestSchedulingStats` | ❌ W0 | ⬜ pending |
| 24-02-03 | 02 | 2 | TAIL-01 | T-24-02-03 | Seeded evidence demotes live resolution; empty store = no change | unit | `go test -race -count=1 ./internal/acpserve/ ./internal/runtime/ -run 'TestOutcome\|TestResolveSubagentModel\|TestConfigSurface'` | ❌ W0 | ⬜ pending |
| 24-03-01 | 03 | 1 | DOC-01 | T-24-03-01 | Doc structure: three-leg surface, out-of-scope statement | static grep | `grep -q 'context_servers' docs/lsp-setup.md && grep -q 'mcpServers' docs/lsp-setup.md && grep -q '52449' docs/lsp-setup.md` | ❌ W0 (doc created in-task) | ⬜ pending |
| 24-03-02 | 03 | 1 | DOC-01 | T-24-03-02 | README link + dry-run build sanity | static + build | `grep -q 'docs/lsp-setup.md' README.md && go build -o /tmp/ass-guard-dryrun ./cmd/ass-guard/` | ✅ (README exists) | ⬜ pending |
| 24-04-01 | 04 | 1 | TAIL-02 | T-24-04-04 | Drift core offline: probe seam + hash compare + report | unit (tracer) | `go test -race -count=1 ./internal/profilecheckcmd/ -run TestNightlyCheck` + built-binary run vs real pin | ❌ W0 | ⬜ pending |
| 24-04-02 | 04 | 1 | TAIL-02 | T-24-04-02/03/05 | Workflow: off-peak cron, least-priv perms, file-based issue body, no eval-gate touch | config parse | `ruby -ryaml -e "YAML.load_file('.github/workflows/nightly-parity.yml')"` + greps | ❌ W0 (file created in-task) | ⬜ pending |
| 24-04-03 | 04 | 1 | TAIL-02 | T-24-04-01 | Dispatch smoke: build-test job green on hosted runner | CI (workflow_dispatch) | `gh workflow run nightly-parity.yml && gh run list --workflow nightly-parity.yml` | ❌ W0 (needs 24-04-02) | ⬜ pending |
| 24-05-01 | 05 | 3 | TAIL-03 | T-24-05-01 | Fixture hook fires functionally in real ACP session | E2E (tracer) | `go test -race -count=1 ./internal/acpserve/ -run TestModesMatrixInteractive` | ❌ W0 | ⬜ pending |
| 24-05-02 | 05 | 3 | TAIL-03 | T-24-05-03 | 12-cell matrix: 9 exercised cells + loud precondition marks + empty row | E2E | `go test -race -count=1 ./internal/acpserve/ ./internal/session/ ./internal/runtime/ -run TestModesMatrix` | ❌ W0 | ⬜ pending |
| 24-05-03 | 05 | 3 | TAIL-03 | T-24-05-03 | Wake cells loud-skip count exactly 3 (PRECONDITION-UNMET(22)) | E2E | `go test -race -count=1 ./internal/runtime/ -run TestModesMatrixWake -v \| grep -c 'PRECONDITION-UNMET(22'` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/modelrouting/outcomes_test.go` — store + D-07 feedback suite (plan 24-01 Task 1, RED)
- [ ] `internal/session/session_outcomes_test.go` — live record path (plan 24-02)
- [ ] `cmd/ass-guard/modelrouting_test.go` additions — `TestSchedulingStats` wire discipline (plan 24-02)
- [ ] `internal/acpserve/config_surface_outcomes_test.go` — demotion tests (plan 24-02)
- [ ] `internal/profilecheckcmd/nightly_check_test.go` — drift core offline suite (plan 24-04)
- [ ] `internal/modesmatrix/matrix.go` + per-package `TestModesMatrix*` files (plan 24-05)
- [ ] `internal/ecosys/testdata/modes-matrix/` fixture (plan 24-05)

*Framework present (go test + mise); no framework install needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| D-03 dry-run: author follows docs/lsp-setup.md top-to-bottom to working LSP tools in a real session | DOC-01 | D-03 explicitly demands author execution ("review-only accuracy does not satisfy"); involves real session + installed LSP MCP server | Record install command, scratch .mcp.json, and observed mcp__-prefixed tool list in 24-03-SUMMARY.md |
| Nightly schedule fires unattended from default branch | TAIL-02 | Schedule semantics only observable in GitHub; fires only once the file exists on `master` (milestone merge — Pitfall 2) | Post-merge: GitHub → Actions → nightly-parity shows a scheduled run; until then workflow_dispatch is the backstop |
| Drift job on self-hosted runner (zcode corpus local) | TAIL-02 | Self-hosted runner is operator infrastructure; zcode binary absent on the dev machine by design | Confirm runner online in repo Settings → Actions → Runners; drift job artifact present after a dispatch run |
| D-14 real installed Claude Code plugin spot-check | TAIL-03 | Requires the operator's real plugin environment | Mount one real plugin read-only into the harness's temp project; record per-surface observations in 24-05-SUMMARY.md; confirmed at the plan's human-verify checkpoint |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 24s (package-scoped runs)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending (planner-seeded 2026-08-28; validate-phase §6 sets status)
