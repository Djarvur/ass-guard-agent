---
phase: 20
slug: built-in-commands-skills-per-agent-model
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-08-28
---

# Phase 20 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib `testing` + `stretchr/testify` v1.11.1, already required) |
| **Config file** | none — convention: `_test.go` beside subject; gate via `.mise.toml` |
| **Quick run command** | `go test -race -count=1 ./internal/runtime/ ./internal/acp/ ./internal/session/ ./internal/acpserve/` |
| **Full suite command** | `mise ci` (vet + golangci-lint + CGO_ENABLED=0 build + `go test -race -count=1 ./...`) |
| **Estimated runtime** | ~25s quick / ~3min full |

---

## Sampling Rate

- **After every task commit:** Run `go test -race -count=1 ./internal/runtime/ ./internal/acp/ ./internal/session/ ./internal/acpserve/`
- **After every plan wave:** Run `mise ci`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 25 seconds (quick run)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 20-01-01 | 01 | 1 | ACP-04, CMDS-01, CMDS-02 | T-20-01/02 | class-B /status: zero provider calls, echo kind correct, durable local_command record | tracer e2e + unit | `go test -race -count=1 ./internal/runtime/ -run TestTracerStatusClassB -x && go test -count=1 ./internal/acp/ -run 'TestAvailableCommandsFrame\|TestUserMessageChunkEcho' -x` | ❌ W0 (commands_test.go, available_commands_test.go) | ⬜ pending |
| 20-01-02 | 01 | 1 | CMDS-01 | T-20-01 | D-01 reservation + D-02 collision order + D-04 winners-only; discovered files confer no authority | unit (table) | `go test -race -count=1 ./internal/runtime/ -run TestCommandChain -x` | ❌ W0 | ⬜ pending |
| 20-01-03 | 01 | 1 | ACP-04 | T-20-01 | schema-pinned frame goldens; full-replacement re-fire seam | unit (golden) | `go test -count=1 ./internal/acp/ -run 'TestAvailableCommands\|TestUserMessageChunk' -x && go test -race -count=1 ./internal/acpserve/ -run TestCommandsNotifySeam -x` | ❌ W0 | ⬜ pending |
| 20-02-01 | 02 | 2 | CMDS-02 | T-20-05 | six pure-local handlers; credential values never in output; /help self-describing | unit | `go test -race -count=1 ./internal/runtime/ -run TestClassB -x` | ❌ W0 (extends) | ⬜ pending |
| 20-02-02 | 02 | 2 | CMDS-02 | T-20-07 | /model session-scope only (layers byte-identical); /clear same-session boundary; /resume + /compact loud degrade | unit | `go test -race -count=1 ./internal/runtime/ -run 'TestClassBModel\|TestClassBClear\|TestClassBDelegate' -x` | ❌ W0 (extends) | ⬜ pending |
| 20-02-03 | 02 | 2 | CMDS-02 | T-20-05/06 | /cost endpoint-first, bounded fetch, source-noted fallback, no credentials in output | unit | `go test -race -count=1 ./internal/runtime/ -run TestClassBCost -x` | ❌ W0 (extends) | ⬜ pending |
| 20-03-01 | 03 | 2 | SKLS-03 | T-20-10 | D-13/D-14 precedence; unset agents NOT on tiers.light; live-chain agent lookup | unit | `go test -race -count=1 ./internal/runtime/ ./internal/session/ -run 'TestDispatchModel\|TestSubagentTier' -x` | ✅ (subagent_tier_wiring_test.go rewritten) | ⬜ pending |
| 20-03-02 | 03 | 2 | SKLS-03 | T-20-11/12 | cross-provider routes via factory seam; one-warning degrade; turn never fails | unit | `go test -race -count=1 ./internal/runtime/ ./internal/session/ -run TestDispatchModel -x` | ✅ (extends) | ⬜ pending |
| 20-03-03 | 03 | 2 | SKLS-03 | T-20-13 | ResolvedModel durable field + D-20 old-line tolerance + deduped live note | unit | `go test -race -count=1 ./internal/session/ -run 'TestSubagentDispatch\|TestTranscript' -x` | ✅ (extends) | ⬜ pending |
| 20-04-01 | 04 | 3 | SKLS-01 | T-20-15/16 | skill expansion per locked Expand semantics; empty/edge args; user-invocable filter | unit | `go test -race -count=1 ./internal/runtime/ -run TestSkillSlash -x` | ❌ W0 (extends) | ⬜ pending |
| 20-04-02 | 04 | 3 | SKLS-02 | T-20-14 | agent slash dispatch: Prompt/Tools applied, advisory-only authority, no parent-model turn | unit | `go test -race -count=1 ./internal/runtime/ -run TestAgentSlash -x` | ❌ W0 (extends) | ⬜ pending |
| 20-04-03 | 04 | 3 | CMDS-03 | T-20-17 | /init expansion via untouched seam, provenance, engine-on parity | unit | `go test -race -count=1 ./internal/runtime/ -run 'TestInitExpansion\|TestClassB' -x` | ❌ W0 (extends) | ⬜ pending |
| 20-05-01 | 05 | 4 | CMDS-04 | T-20-18/19, T-20-SC | watcher + debounce + swap + D-11 one-warning skip + D-12 degrade | unit (temp-dir) | `go test -race -count=1 ./internal/runtime/ -run TestRescan -x && go vet ./internal/runtime/` | ❌ W0 (rescan_test.go) | ⬜ pending |
| 20-05-02 | 05 | 4 | CMDS-04 | T-20-19 | invoke-time freshness backstop catches watch misses; bounded probe | unit | `go test -race -count=1 ./internal/runtime/ -run TestFreshnessBackstop -x` | ❌ W0 (extends) | ⬜ pending |
| 20-05-03 | 05 | 4 | CMDS-04, ACP-04 | T-20-18 | re-fire full set; rescan × turns -race clean; clean shutdown; stderr-only discipline | unit + race | `go test -race -count=1 ./internal/runtime/ ./internal/acpserve/ -run 'TestRescanConcurrency\|TestRescanRefire\|TestRescanShutdown' -x` | ❌ W0 (extends) | ⬜ pending |
| 20-06-01 | 06 | 5 | all eight | T-20-22 | wire-level E2E: advertisement, class-B round-trip, rescan re-fire, agent dispatch | e2e (simulator) | `go test -race -count=1 ./internal/acpserve/ -run TestSimulatorCommandSurface -x` | ✅ (simulator_e2e_test.go extended) | ⬜ pending |
| 20-06-02 | 06 | 5 | all eight | T-20-22 | full gate + criteria-to-evidence matrix | gate | `mise ci` | n/a | ⬜ pending |
| 20-06-03 | 06 | 5 | ACP-04, CMDS-01..04, SKLS-01..02 | — | live-Zed UX: autocomplete, instant class-B, live pickup, resolvedModel note | manual (operator) | see 20-06 Task 3 checkpoint | n/a | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/runtime/commands_test.go` — created by 20-01 Task 1 (RED first), extended by 20-02/20-04
- [ ] `internal/acp/available_commands_test.go` — created by 20-01 Task 1/3 (schema goldens)
- [ ] `internal/runtime/rescan_test.go` — created by 20-05 Task 1, extended by Tasks 2–3
- [ ] Framework install: none — stdlib `testing` + testify v1.11.1 already in go.mod; fsnotify v1.10.1 added by 20-05 (RESEARCH legitimacy audit: Approved)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live-Zed autocomplete + instant class-B + mid-session pickup + resolvedModel visibility (RESEARCH A2) | ACP-04, CMDS-01..04, SKLS-01..02 | Editor-native UX rendering (Zed's autocomplete cards, echo bubbles) is not observable from the agent side; Phases 15/16 WINDOWS precedent | 20-06 Task 3 checkpoint: 7-step script (launch in Zed with fixture skill/agent/command; "/" autocomplete; /status + /help; live-create commands/extra.md; /demo-agent dispatch; /cost source note; /compact loud absence) |

*One manual behavior only; all other phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 20s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending — set at plan close by /gsd-plan-phase; per-task statuses flip ⬜→✅ during execution
