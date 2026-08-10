---
phase: 2
slug: session-core-acp-interface
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-08-09
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Derived from `02-RESEARCH.md` §"Validation Architecture".

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib `testing`) + `github.com/stretchr/testify` (Phase-1 dep) |
| **Config file** | none (Go convention; per-package `_test.go` files) |
| **Quick run command** | `go test ./internal/...` |
| **Full suite command** | `go test ./... -race` (`-race` is load-bearing — channel/bus/concurrency code) |
| **Estimated runtime** | ~15-30 seconds (unit + integration; no live network — provider calls mocked via httptest) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/...` (affected packages)
- **After every plan wave:** Run `go test ./... -race`
- **Before `/gsd:verify-work`:** Full suite must be green AND `internal/session/reconstruction_test.go::TestTranscriptReconstructsSession` must pass (LOG-04 gate)
- **Max feedback latency:** ~30 seconds (the `-race` flag adds overhead but is non-negotiable for the goroutine-heavy Session Core)

---

## Per-Task Verification Map

> Filled progressively as plans define tasks. Task IDs follow `{phase}-{plan}-{task}` (e.g., 02-01-T1). The planner assigns final IDs; this table is the target coverage map from RESEARCH §"Validation Architecture".

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 02-01-* | 01 | 1 | ACP-05 | T-02-01 | Hand-rolled framer rejects malformed frames; transport discipline (stdout clean) | unit | `go test ./internal/acp -run TestFramer -race` | ❌ W0 | ⬜ pending |
| 02-01-* | 01 | 1 | ACP-01 | T-02-02 | Stdout carries ONLY valid ACP frames; all logs to stderr | integration | `go test ./internal/acp -run TestServerStdoutClean -race` | ❌ W0 | ⬜ pending |
| 02-02-* | 02 | 2 | SESS-06 | T-02-04 | Manager sole owner; mutex-guarded append (no corruption) | unit | `go test ./internal/session -run TestManagerAppendRead -race` | ❌ W0 | ⬜ pending |
| 02-02-* | 02 | 2 | LOG-03 | T-02-08 | Per-line redaction; no `sk-`/`Bearer` in transcript | unit | `go test ./internal/session -run TestTranscriptRedaction` | ❌ W0 | ⬜ pending |
| 02-02-* | 02 | 2 | SESS-01 | — | Projector builds lean window from transcript | unit | `go test ./internal/session -run TestProjector -race` | ❌ W0 | ⬜ pending |
| 02-02-* | 02 | 2 | SESS-04 | — | Boundary resets to lean seed; zero carry-forward | unit | `go test ./internal/session -run TestLeanSeedAfterBoundary` | ❌ W0 | ⬜ pending |
| 02-03-* | 03 | 2 | SESS-03 | — | More-mutating-wins formula (TDD) | unit | `go test ./internal/toolcat -run TestEffectiveMutability` | ❌ W0 | ⬜ pending |
| 02-03-* | 03 | 2 | SESS-02 | T-02-03 | Config can ADD boundaries, NEVER flip mutating→read-only | unit | `go test ./internal/session -run TestBoundaryEnforcement -race` | ❌ W0 | ⬜ pending |
| 02-04-* | 04 | 3 | ACP-02 | — | Lifecycle methods round-trip (initialize/session/new/session/prompt) | integration | `go test ./internal/acp -run TestLifecycle -race` | ❌ W0 | ⬜ pending |
| 02-04-* | 04 | 3 | ACP-03 | — | session/load is no-op (D-09 DROPPED) | unit | `go test ./internal/acp -run TestSessionLoadNoOp` | ❌ W0 | ⬜ pending |
| 02-04-* | 04 | 3 | ACP-04 | — | End-to-end streaming: SSE → bus → session/update | integration | `go test ./internal/acp -run TestStreamingEndToEnd -race` | ❌ W0 | ⬜ pending |
| 02-05-* | 05 | 3 | PARA-01 | T-02-05 | Subagent isolated goroutine, restricted tool subset | unit | `go test ./internal/session -run TestSubagentDispatch -race` | ❌ W0 | ⬜ pending |
| 02-05-* | 05 | 3 | PARA-02 | — | Results via bus tagged parent-turn-id | unit | `go test ./internal/session -run TestSubagentResultBusTagged` | ❌ W0 | ⬜ pending |
| 02-05-* | 05 | 3 | PARA-03 | T-02-06 | Panic recovered → tool-error, never crash | unit | `go test ./internal/session -run TestSubagentPanicRecovery -race` | ❌ W0 | ⬜ pending |
| 02-05-* | 05 | 3 | PARA-04 | — | Provider semaphore bounds parent+subagent concurrency | unit | `go test ./internal/provider -run TestSemaphore -race` | ❌ W0 | ⬜ pending |
| 02-06-* | 06 | 4 | LOG-04 | — | Reconstruction sufficiency (transcript + profile reconstructs session) | integration | `go test ./internal/session -run TestTranscriptReconstructsSession` | ❌ W0 | ⬜ pending |
| 02-06-* | 06 | 4 | LOG-02 | — | Transcript writer async bus consumer (never in critical path) | unit | `go test ./internal/session -run TestTranscriptWriterAsync` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

> Test scaffolds that MUST exist before implementation tasks can claim their `<automated>` verify commands. Created in the first task of each plan (or a dedicated Wave 0 setup task).

- [ ] `internal/acp/server_test.go` — in-process mock ACP client over `io.Pipe` (pattern from `spikes/03-acp-handshake/`); covers ACP-01/02/04/05
- [ ] `internal/acp/framer_test.go` — framing round-trip + notification-vs-request distinction; covers ACP-05
- [ ] `internal/session/manager_test.go` — transcript append/read + concurrency (`-race`); covers SESS-06
- [ ] `internal/session/projector_test.go` — lean window extraction rules (D-02 N-char policy); covers SESS-01/04
- [ ] `internal/session/reconstruction_test.go` — LOG-04 end-to-end; covers LOG-04
- [ ] `internal/session/transcript_test.go` — per-line redaction (LOG-03); covers LOG-03
- [ ] `internal/session/subagent_test.go` — PARA-01/02/03 with `-race`; covers PARA-01/02/03
- [ ] `internal/session/boundary_test.go` — boundary enforcement + SESS-02 config-adds-only; covers SESS-02/05
- [ ] `internal/toolcat/mutability_test.go` — TDD for EffectiveMutability formula; covers SESS-03
- [ ] `internal/provider/semaphore_test.go` — PARA-04 semaphore under contention
- [ ] `internal/provider/streaming_test.go` — ACP-04 provider.Stream with httptest mock SSE
- [ ] `internal/event/bus_test.go` — EXPANDED from Phase 1: typed channels + backpressure (D-04/D-05)

*Framework install: none — Go testing + testify are Phase-1 deps (go.mod already declares them).*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Zed spawns ass-guard via ACP registry; live `session/prompt` streams token-by-token in the editor | ACP-01/02/04 (end-to-end in the real editor) | Requires Zed + operator's ZAI_API_KEY + the ACP registry manifest; cannot be automated in CI | 1. Install ass-guard via ACP registry. 2. Open Zed in a project. 3. Send a prompt. 4. Observe streamed tokens. 5. Confirm transcript appears under `.ass-guard/`. |
| Live Z.ai streaming round-trip | ACP-04 (real provider SSE) | Requires ZAI_API_KEY; the unit/integration tests use httptest mock SSE | `ASSGUARD_LIVE=1 go test ./internal/provider -run TestStreamingLive -race` (operator-gated, skipped by default) |

---

## Dimension 8 Gate Summary

- **Check 8a (Automated verify presence):** every implementation task has a `<verify><automated>` command mapping to a Wave 0 test file above.
- **Check 8b (Feedback latency):** all commands < 30s; no E2E-only suites; no watch mode.
- **Check 8c (Sampling continuity):** every wave has ≥2/3 tasks with automated verify (all tasks do — Go test is the default).
- **Check 8d (Wave 0 completeness):** the Wave 0 list above covers every test file referenced by the per-task map.
