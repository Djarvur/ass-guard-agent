---
phase: 3
slug: model-scheduling
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-08-09
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Derived from `03-RESEARCH.md` §13 (Validation Architecture). Phase 3 is logic-dense with mostly pure functions (resolver, breaker state machine, cost window, config-validation graph) — the state spaces are small and fully enumerable, so the sampling risk is low; the mitigation is exhaustive table tests, not more samples.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (stdlib `testing` + `stretchr/testify` for assertions, already in the project stack) |
| **Config file** | none — `go test` reads `go.mod`; per-package `_test.go` files |
| **Quick run command** | `go test ./internal/scheduler/ ./internal/provider/ -race` |
| **Full suite command** | `go test ./... -race` |
| **Estimated runtime** | ~15-25 seconds (pure-logic unit tests + table tests; no network — provider is a fake) |

> The `-race` flag is **load-bearing**: the breaker, cost tracker, and event bus are all concurrent (parent + subagents hit the same `(provider, model)` breaker; the bus is multi-subscriber). A green run without `-race` does not prove the concurrency invariants.

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/scheduler/ ./internal/provider/ -race`
- **After every plan wave:** Run `go test ./... -race`
- **Before `/gsd:verify-work`:** Full suite must be green AND `go build ./...` + `go vet ./...` clean
- **Max feedback latency:** 25 seconds

---

## Per-Task Verification Map

> Task IDs map 1:1 to the `<task id="...">` in each PLAN.md. REQ = requirement covered. Threat Ref = the `T-03-NN` from each plan's `<threat_model>`. "File Exists ❌ W0" means Wave 0 / the task itself creates the file.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 03-01-T1 | 01 | 1 | SCHED-02 | T-03-01 | `time/tzdata` bundled → no host zoneinfo dep (single-binary) | unit | `go test ./internal/scheduler -run TestTzdataBundled -race` | ❌ W0 | ⬜ pending |
| 03-01-T2 | 01 | 1 | SCHED-06 | T-03-02 | D-10 reject-on-load: inconsistent configs NEVER start the server | unit | `go test ./internal/scheduler -run TestValidate -race -v` | ❌ W0 | ⬜ pending |
| 03-01-T3 | 01 | 1 | SCHED-01, SCHED-02, SCHED-03 | T-03-03 | D-02 window-wins-outright enforced (project cannot override structural window) | unit | `go test ./internal/scheduler -run TestResolve -race -v` | ❌ W0 | ⬜ pending |
| 03-02-T1 | 02 | 2 | SCHED-04 | T-03-05 | Exhausted kind NEVER produced by `ClassifyHTTP` (only cost tracker emits it) | unit | `go test ./internal/provider -run TestClassify -race -v` | ❌ W0 | ⬜ pending |
| 03-02-T2 | 02 | 2 | SCHED-04, SCHED-05 | T-03-06 | fallback/cost events published to stderr-routed bus, never stdout | unit | `go test ./internal/scheduler -run TestEvents -race -v` | ❌ W0 | ⬜ pending |
| 03-02-T3 | 02 | 2 | SCHED-04 | T-03-07 | Structural error stops the walk (no silent retry); Transient walks the chain | unit | `go test ./internal/scheduler -run TestDispatch -race -v` | ❌ W0 | ⬜ pending |
| 03-03-T1 | 03 | 3 | SCHED-05 | T-03-09 | breaker mutex held only across state R/W, not the provider call (no concurrency bottleneck); Structural errors don't feed the breaker | unit | `go test ./internal/scheduler -run 'TestBreaker' -race -v` | ❌ W0 | ⬜ pending |
| 03-03-T2 | 03 | 3 | SCHED-05 | T-03-10 | cost ceiling degrade-then-stop (not hard-stop on first breach); window rollover resets budget | unit | `go test ./internal/scheduler -run TestCost -race -v` | ❌ W0 | ⬜ pending |
| 03-03-T3 | 03 | 3 | SCHED-05 | T-03-11 | breaker+cost wired into Dispatch; HardStop returns `KindExhausted` | integration | `go test ./internal/scheduler -run TestDispatchSafety -race -v` | ❌ W0 | ⬜ pending |
| 03-04-T1 | 04 | 3 | SCHED-06 | T-03-13 | runtime capability gate never routes a tool-using turn to a tool-less model | unit+integration | `go test ./internal/scheduler -run TestCapabilityGate -race -v` | ❌ W0 | ⬜ pending |
| 03-04-T2 | 04 | 3 | SCHED-01, SCHED-06 | T-03-14 | CLI diagnostic output to stderr only; `--json` machine output to stdout only on explicit request (transport discipline) | unit | `go test ./cmd/ass-guard -run TestSchedulingCmds -race -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `go.mod` gains `require` lines for `github.com/spf13/viper`, `gopkg.in/yaml.v3`, `github.com/stretchr/testify` (created in 03-01-T1; `go mod tidy` finalizes `go.sum`)
- [ ] `internal/scheduler/` package directory + `init.go` (`_ "time/tzdata"` blank import) — 03-01-T1
- [ ] `internal/scheduler/testdata/` fixtures directory (valid + invalid configs) — 03-01-T2
- [ ] `internal/provider/errors.go` (ProviderError type) — 03-02-T1

*No external test framework beyond `testify` (already in stack). The fake Provider for dispatch tests lives in `internal/scheduler`'s `_test.go` (test-only, not shipped).*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live provider transient fallback (real 429 from a real provider) | SCHED-04 | needs operator API keys + a rate-limited live endpoint (ZAI_API_KEY / MINIMAX_API_KEY gates, per STATE.md Phase-0 carry-forward) | After keys provisioned: point a `scheduling.yaml` fixture at the live endpoint, force a 429 (rapid requests), observe the `ProviderFallback` session/update in the ACP stream + the stderr log line |
| Live cost-ceiling breach | SCHED-05 | needs a live provider to accumulate real token costs | Provision a low `cost_ceiling.amount_usd` (e.g. 0.01) against a live endpoint; run turns until breach; observe the CostCeilingWarn + degrade, then HardStop |

*All other phase behaviors have automated verification (pure-logic table tests + fake-provider dispatch scenarios).*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies (every task above has a `go test -run` command)
- [x] Sampling continuity: no 3 consecutive tasks without automated verify (every task has one)
- [ ] Wave 0 covers all MISSING references (pending 03-01-T1 execution)
- [x] No watch-mode flags (`go test` one-shot; no `--watch`)
- [x] Feedback latency < 25s (pure-logic unit tests; no network in the loop)
- [ ] `nyquist_compliant: true` set in frontmatter (set after plans pass the checker + Wave 0 confirmed)

**Approval:** pending
