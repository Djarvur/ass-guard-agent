---
phase: 5
slug: ecosystem-compatibility
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-08-09
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (stdlib `testing`) + `stretchr/testify` v1.11.1 (already in go.mod) |
| **Config file** | none (Go convention: `*_test.go` colocated; `go.mod` pins module) |
| **Quick run command** | `go test ./internal/mcp/... ./internal/toolcat/... ./internal/ecosys/...` |
| **Full suite command** | `go test ./... && go build ./... && go vet ./...` |
| **Estimated runtime** | ~25 seconds (MCP self-exec subprocess tests add ~5s; rest is sub-second unit tests) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/mcp/... ./internal/toolcat/... ./internal/ecosys/...` (+ the package(s) a task touched)
- **After every plan wave:** Run `go test ./... && go build ./... && go vet ./...`
- **Before `/gsd:verify-work`:** Full suite must be green AND the phase-level end-to-end test (ACP → session/new → MCP host spawn → model tool-call → result) must pass
- **Max feedback latency:** 30 seconds (subprocess spawn/grace tests are the long tail; tagged `testing.Short()`-skippable for fast loops)

---

## Per-Task Verification Map

Map convention: `05-PP-TT` = plan `PP`, task `T#` (the `<task id="TT">` in that plan). 12 tasks across 3 plans (4 each).

| Task ID | Plan.Task | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|-----------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 05-01-T1 | 01.T1 | 1 | ECOS-01 | T-5-01 | go-sdk pinned + builds; verified API names recorded in doc.go | unit | `go build ./... && go vet ./... && go doc ...CommandTransport` | ✅ (doc.go created in-task) | ⬜ pending |
| 05-01-T2 | 01.T2 | 1 | ECOS-01, ECOS-03 | — | Host spawns via CommandTransport; ListTools every Start (no cache); mcp__ naming; CallTool routes | unit+integration | `go test ./internal/mcp/ -race -run 'TestHostStart|TestHostNaming|TestHostNoCache|TestHostCallTool|TestHostClose'` | ❌ W0 | ⬜ pending |
| 05-01-T3 | 01.T3 | 1 | ECOS-01 | — | Catalog.Register; MCPExecutor routes mcp__* to host, falls through; no import cycle | unit | `go test ./internal/toolcat/ -race -run 'TestRegister|TestMCPExecutor'` | ❌ W0 | ⬜ pending |
| 05-01-T4 | 01.T4 | 1 | ECOS-01, ECOS-03 | T-5-02 | per-session profile copy (no leak); executor wrap; TRACER e2e tool-call+result in transcript; shutdown wired | integration | `go test ./cmd/ass-guard/ -race -run 'TestSessionForMCP|TestTracerMCPEndToEnd|TestSessionShutdownWired'` | ❌ W0 | ⬜ pending |
| 05-02-T1 | 02.T1 | 2 | ECOS-02 | — | Process-group spawn (Setpgid); grandchild PGID==server PID; build tags darwin\|\|linux | integration | `go test ./internal/mcp/ -race -run 'TestProcessGroupIsolation|TestPGIDCapture'` | ❌ W0 | ⬜ pending |
| 05-02-T2 | 02.T2 | 2 | ECOS-02 | T-5-06 | SIGTERM→3s grace→SIGKILL; group signal reaches grandchild; idempotent Close | integration | `go test ./internal/mcp/ -race -run 'TestShutdownGraceful|TestShutdownForceSIGKILL|TestShutdownGrandchild|TestCloseIdempotent'` | ❌ W0 | ⬜ pending |
| 05-02-T3 | 02.T3 | 2 | ECOS-02 | T-5-04b | Reaper Wait4(-pgid) loop; no zombie after Close; mid-session reaper goroutine | integration | `go test ./internal/mcp/ -race -run 'TestReaperNoZombie|TestReapDrains|TestReapNonBlocking'` | ❌ W0 | ⬜ pending |
| 05-02-T4 | 02.T4 | 2 | ECOS-02 | T-5-07 | Session.Close() via OnClose seam (no import cycle); cancel/logout/ctx-done all reach host.Close(); panic keeps session | integration | `go test ./internal/session/ ./cmd/ass-guard/ -race -run 'TestSessionCloseOnCancel|TestSessionCloseOnLogout|TestSessionCloseOnCtxDone|TestPromptPanicKeepsSession'` | ❌ W0 | ⬜ pending |
| 05-03-T1 | 03.T1 | 1 | ECOS-04 | — | Loader discovers SKILL.md / command.md / manifest from one tree; yaml.v3+json (no viper) | unit | `go test ./internal/ecosys/ -race -run 'TestDiscoverSkill|TestDiscoverCommand|TestDiscoverPlugin|TestEmptyDir'` | ❌ W0 | ⬜ pending |
| 05-03-T2 | 03.T2 | 1 | ECOS-05 | T-5-10 | .claude/ wins over .ass-guard/; project wins over user (two-phase merge); no-clobber invariant | unit | `go test ./internal/ecosys/ -race -run 'TestPrecedenceClaudeWins|TestPrecedenceProjectWins|TestAssguardAdditionsSurface|TestNoClobber'` | ❌ W0 | ⬜ pending |
| 05-03-T3 | 03.T3 | 1 | ECOS-05 | T-5-08 | .claude/ read-only structural (os.Open/os.ReadFile only); EnsureGitignore rejects .claude/; D-07 gitignore covers new subdirs | unit | `go test ./internal/ecosys/ -race -run 'TestReadOnlyEnforced|TestGitignoreCoverage|TestEnsureGitignoreFirstRun'` | ❌ W0 | ⬜ pending |
| 05-03-T4 | 03.T4 | 1 | ECOS-04, ECOS-05 | — | Discover() ties together; LoadUserMCPConfig reads ~/.claude.json; deterministic accessors; no import cycle | unit | `go test ./internal/ecosys/ -race -run 'TestDiscover|TestLoadUserMCPConfig|TestRegistryAccessors'` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/mcp/host.go` + `internal/mcp/host_test.go` + `internal/mcp/echoserver.go` — Host + self-exec echo MCP server (helper shared by 05-01-T2 and the 05-01-T4 tracer) [plan 05-01]
- [ ] `internal/mcp/lifecycle.go` + `internal/mcp/lifecycle_test.go` — grandchild-spawn fixtures for ECOS-02 [plan 05-02]
- [ ] `internal/ecosys/loader.go` + `internal/ecosys/loader_test.go` — fixture-tree builder for ECOS-04/05 [plan 05-03]
- [ ] `internal/mcp/doc.go` — created in 05-01-T1 so `go build ./...` stays green after the dep task (records the verified go-sdk API names)

*Framework + testify already present — no install needed. Each plan's first task creates the package skeleton so `go build ./...` never breaks between tasks.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Real `.mcp.json` from a developer's Claude Code setup loads unchanged | ECOS-01 | Requires an external real MCP server binary (e.g. `@modelcontextprotocol/server-filesystem`) not safe to pin in unit tests | Copy a known-good `.mcp.json` into a temp project, run `ass-guard acp`, confirm the server's tools appear in the shaped request (audit transcript `request_shaped` line) and a tool-call round-trips |

*All other phase behaviors have automated verification.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 30s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
