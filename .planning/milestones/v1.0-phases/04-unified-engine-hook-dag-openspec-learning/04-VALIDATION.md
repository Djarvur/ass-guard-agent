---
phase: 4
slug: unified-engine-hook-dag-openspec-learning
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-08-09
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Derived from `04-RESEARCH.md` §7 (Validation Architecture). Phase 4 is concurrent + integration-dense (the engine wraps the Session Core; the hook-DAG runs goroutine-isolated; tool dispatch batches read-only calls; the learning store is single-writer). The `-race` flag is load-bearing throughout — a green run without `-race` does not prove the concurrency invariants (provenance map, tool batch, learning mutex, cancel-drain). The structural-safety property (unmatched ⇒ nothing) is sampled as a `testing/quick` property at 2× the rate of any single pattern match.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (stdlib `testing` + `stretchr/testify` + `testing/quick` for the structural-safety property; already in the project stack) |
| **Config file** | none — `go test` reads `go.mod`; per-package `_test.go` files + embedded testdata fixtures (`*.yaml`, `*.toml`, `openspec-stub.sh`) |
| **Quick run command** | `go test ./internal/engine/ ./internal/hookdag/ ./internal/toolexec/ -race` |
| **Full suite command** | `go test ./... -race` |
| **Estimated runtime** | ~25-40 seconds (concurrent unit tests with forced sleeps for overlap assertion + table tests + one full e2e; no network — providers + openspec are fakes/stubs) |

> The forced-sleep tests (tool-concurrency parallelism, provenance re-entrant overlap, cancel-drain timing) are deterministic via recorded start-monotonic-times + tight budgets (30ms sleeps, 80ms budgets). They run fast but NOT instant — the sleep is the signal.

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/engine/ ./internal/hookdag/ ./internal/toolexec/ -race` (the concurrent cores) OR the specific package(s) the task touched
- **After every plan wave:** Run `go test ./... -race`
- **Before `/gsd:verify-work`:** Full suite green AND `go build ./...` + `go vet ./...` clean AND `grep -rn "TODO\|FIXME" internal/engine internal/hookdag internal/openspec internal/learning internal/toolexec cmd/ass-guard | wc -l` == 0
- **Max feedback latency:** 40 seconds (full suite); 8 seconds (quick run)

---

## Per-Task Verification Map

> Task IDs map 1:1 to the `<task id="...">` in each PLAN.md. REQ = requirement covered. Threat Ref = the invariants below. "File Exists ❌ W0" = the task itself creates the file (Phase 4 has no pre-existing Wave-0 stubs — it's greenfield packages + additive edits).

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 04-01-T1 | 01 | 1 | ENG-02 | — | EngineDecision event on every turn (audit log proves safety) | unit | `go test ./internal/event ./internal/session -run 'TestEngineDecision\|TestAppendEngine' -race -v` | ❌ W0 | ⬜ pending |
| 04-01-T2 | 01 | 1 | ENG-01, ENG-02, ENG-03 | — | unmatched ⇒ ActionNothing (structural safety); `testing/quick` property | unit+property | `go test ./internal/engine -run TestDecide -race -v && go test ./internal/engine -run Quick -race -v` | ❌ W0 | ⬜ pending |
| 04-01-T3 | 01 | 1 | ENG-01, ENG-04, ENG-05 | — | engine panic ⇒ original (stop,err) returned (graceful degradation); ctx cancel ⇒ no further injections | unit | `go test ./internal/engine -run 'TestObserve\|TestObserve_Degradation\|TestObserve_Budget\|TestObserve_Cancel' -race -v` | ❌ W0 | ⬜ pending |
| 04-01-T4 | 01 | 1 | ENG-01, ENG-05 | — | tracer: handoff text ⇒ 1 continue injection; unmatched ⇒ 0; dual-signal (text+tool) | integration | `go test ./internal/engine -run TestIntegration -race -v` | ❌ W0 | ⬜ pending |
| 04-02-T1 | 02 | 2 | OPEN-02 | — | TOML config + seeded patterns/handoff/commands; invalid regex/action/mutability ⇒ ConfigError | unit | `go test ./internal/openspec -run 'TestLoad\|TestInvalid' -race -v` | ❌ W0 | ⬜ pending |
| 04-02-T2 | 02 | 2 | OPEN-01 | T-04-06 | subprocess: exit 0 stdout surfaced; non-zero stderr+err; ctx-cancel kills; not-on-PATH ⇒ ErrOpenSpecNotFound | unit | `go test ./internal/openspec -run TestAdapter -race -v` | ❌ W0 | ⬜ pending |
| 04-02-T3 | 02 | 2 | OPEN-02, OPEN-03 | — | registered mutability drives IsBoundary unchanged (D-15); PatternTable bridges to engine | unit | `go test ./internal/openspec -run 'TestRegister\|TestPatternTable' -race -v` | ❌ W0 | ⬜ pending |
| 04-03-T1 | 03 | 1 | HOOK-01 | — | YAML hook config (yaml.v3 NOT viper); layered seeded.yaml default | unit | `go test ./internal/hookdag -run 'TestLoad\|TestValidate' -race -v` | ❌ W0 | ⬜ pending |
| 04-03-T2 | 03 | 1 | HOOK-01, HOOK-03 | — | on-failure halt/continue/ask correctly applied; HookProgress per step | unit | `go test ./internal/hookdag -run TestExecute -race -v` | ❌ W0 | ⬜ pending |
| 04-03-T3 | 03 | 1 | HOOK-04, HOOK-05 | T-04-02 | provenance: concurrent same-hook ⇒ exactly one skipped-reentrant; send-prompt IS a turn; fresh-context IS a boundary | unit | `go test ./internal/hookdag -run 'TestReentrant\|TestStep' -race -v` | ❌ W0 | ⬜ pending |
| 04-04-T1 | 04 | 1 | TOOL-04 | T-04-03 | read-only concurrent (4 calls < 80ms); mutating serialized (no overlap); arrival-order results | unit | `go test ./internal/toolexec -run TestDispatchBatch -race -v` | ❌ W0 | ⬜ pending |
| 04-04-T2 | 04 | 1 | TOOL-05 | — | swappable Backend; swap by config, no code change; firecrawl is a stub (no dep) | unit | `go test ./internal/toolexec -run 'TestHTTPBackend\|TestBackendsFromConfig' -race -v` | ❌ W0 | ⬜ pending |
| 04-04-T3 | 04 | 1 | TOOL-04, TOOL-05 | — | RealExecutor catalog-backed; Session.SetToolExecutor + loop uses DispatchBatch; nil ⇒ stub backward-compat | unit+integration | `go test ./internal/toolexec ./internal/toolcat ./internal/session -race -v` | ❌ W0 | ⬜ pending |
| 04-05-T1 | 05 | 3 | ENG-02, HOOK-05, LRN-01 | — | ActionDispatcher: hook→executor, ask→store; nil dispatcher ⇒ continue-only (04-01 tests still pass) | unit | `go test ./internal/engine -race -v` | ❌ W0 | ⬜ pending |
| 04-05-T2 | 05 | 3 | ENG-05, OPEN-03 | — | acp_serve wiring: engine wraps sess.Prompt; SetToolExecutor; RegisterTools; disabled-engine fallback | integration | `go test ./cmd/ass-guard -run TestEndToEnd -race -v` | ✅ (edits existing) | ⬜ pending |
| 04-05-T3 | 05 | 3 | ENG-05, HOOK-02, OPEN-02 | — | 3-stage zero-continue (3 calls, 0 manual taps); structural safety; forgotten routine fires; full seeding | integration | `go test ./cmd/ass-guard ./internal/engine ./internal/openspec -run 'TestEndToEnd\|TestZeroContinue\|TestForgottenRoutine\|TestSeeded' -race -v` | ❌ W0 | ⬜ pending |
| 04-06-T1 | 06 | 2 | LRN-01, LRN-03 | — | learned.yaml store; single-writer mutex; atomic save (temp+rename) | unit | `go test ./internal/learning -run TestStore -race -v` | ❌ W0 | ⬜ pending |
| 04-06-T2 | 06 | 2 | LRN-01, LRN-03 | — | candidate(conf 0) → ≥3 confirms ⇒ active; different answer ⇒ conflict (not incremented); expiry ignored-not-deleted | unit | `go test ./internal/learning -run 'TestCandidate\|TestConfirm\|TestConflict\|TestExpiry' -race -v` | ❌ W0 | ⬜ pending |
| 04-06-T3 | 06 | 2 | LRN-02, LRN-04 | — | ProposeHooks (repeated seq ≥3 ⇒ Proposal); Revert atomic + no-op; List deterministic | unit | `go test ./internal/learning -run 'TestPropose\|TestRevert\|TestList' -race -v` | ❌ W0 | ⬜ pending |
| 04-06-T4 | 06 | 2 | LRN-04 | — | `ass-guard learning list` + `revert <id>` CLI; transport discipline (stdout table, stderr diag) | unit | `go test ./cmd/ass-guard -run TestLearning -race -v` | ❌ W0 | ⬜ pending |
| 04-07-T1 | 07 | 4 | ENG-03 | T-04-04 | cancel-drain: session/cancel after 1st turn ⇒ sess.Prompt called ONCE (pending injections abandoned) | unit+integration | `go test ./internal/engine ./cmd/ass-guard -run 'TestCancel\|TestDrain' -race -v` | ❌ W0 | ⬜ pending |
| 04-07-T2 | 07 | 4 | (all 5 criteria) | — | full e2e: 5 ROADMAP success criteria (zero-continue + hooks + on-failure/provenance + learning + concurrency/backend) | integration | `go test ./internal/engine -run TestE2E -race -v` | ❌ W0 | ⬜ pending |
| 04-07-T3 | 07 | 4 | (all) | — | 04-VERIFICATION-PREP.md: 5-criterion→test map + phase gate (full suite green + vet clean + 0 TODOs) | doc+gate | `go test ./... -race && go build ./... && go vet ./...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Threat References (invariants sampled above their operating rate)

| Ref | Invariant | Nyquist sampling |
|-----|-----------|------------------|
| T-04-01 | An engine failure NEVER prevents a turn from completing (the engine is an observer, not in the critical path) | injected-panic test returns the original `(stop, err)` — sampled per engine task; re-asserted at integration (04-05-T3, 04-07-T2) |
| T-04-02 | No infinite continue/hook loop (engine budget + hook-DAG provenance) | always-match fake ⇒ budget-capped call count; concurrent same-hook ⇒ one skipped-reentrant — sampled under `-race` at 2× the launch rate |
| T-04-03 | A mutating tool NEVER overlaps another tool call (concurrency safety) | recorded-start-time overlap assertion under `-race` — sampled per batch |
| T-04-04 | session/cancel drains EVERY queued injection (the structural off-switch — PROJECT.md's only safety mechanism) | queue-3-cancel-after-1 ⇒ call-count == 1 assertion — sampled at 2× the cancel rate |
| T-04-05 | Unmatched output NEVER triggers an action (structural safety) | `testing/quick` property: random unmatched ⇒ ActionNothing — sampled far above the match rate |
| T-04-06 | OpenSpec subprocess never orphans on ctx cancel (the kill reaches the child) | exit + timing assertion (cancel within 200ms) — sampled per test, well above the rare-cancel rate |
| T-04-07 | A conflicting learned entry is NEVER silently applied | Confirm-different-answer ⇒ Status=conflict + Lookup skips it — sampled per Confirm |
| T-04-08 | A hook failure is NEVER silent (on-failure always declared + emitted) | every on-failure cell (halt/continue/ask) emits HookProgress{error} — sampled per Execute |
| T-04-09 | The learned store is NEVER torn by concurrent read/write | single-writer mutex + atomic save — sampled under `-race` across all store tests |

---

## Wave 0 Requirements

*Existing infrastructure covers all phase requirements.* Phase 4 uses the in-repo Go toolchain (`go test`, testify, testing/quick — all already in go.mod). The only new test-only artifacts are:
- `internal/openspec/testdata/openspec-stub.sh` — a portable stub script for the subprocess tests (created in 04-02-T2)
- `testdata/*.yaml` / `*.toml` fixtures per package (created in the tasks that consume them)

No framework install, no conftest, no shared fixtures beyond the per-package testdata.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Real OpenSpec CLI round-trip (live `openspec` binary on PATH) | OPEN-01, ENG-05 | CI/first-run cannot assume the binary; tests gate behind `ASSGUARD_OPENSPEC_BIN=1` | Install OpenSpec, set `ASSGUARD_OPENSPEC_BIN=1`, run `go test ./internal/openspec -run TestAdapter/Real -race -v` + run a real scenario through `ass-guard` |
| Live provider zero-continue (ZAI_API_KEY set) | ENG-05 | Autonomous plans use mock providers; the live round-trip needs operator keys (the Phase-1 carry-forward) | Provision `ZAI_API_KEY`, run the 04-05-T3 scenario against the real provider, confirm zero manual taps |

---

*Phase: 4 · Validation derived from 04-RESEARCH.md §7 + the 7 PLAN.md task IDs.*
