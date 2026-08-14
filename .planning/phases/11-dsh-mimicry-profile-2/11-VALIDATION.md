---
phase: 11
slug: dsh-mimicry-profile-2
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-08-14
---

# Phase 11 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|-------|-------|
| **Framework** | go test (stdlib testing; race detector on) |
| **Config file** | none — go.mod (new dep: github.com/klauspost/compress v1.19.2, exact pin) |
| **Quick run command** | `go test ./internal/profile/ ./internal/provider/ -race` |
| **Full suite command** | `go test ./... -race && mise run vet && mise run lint` |
| **Estimated runtime** | ~60–120 seconds (full suite; real-artifact tests skip-clean when captures absent) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/profile/ ./internal/provider/ -race` (plus the touched package)
- **After every plan wave:** Run `go test ./... -race && mise run vet && mise run lint`
- **Before `/gsd:verify-work`:** Full suite green (`mise run ci`) AND the phase gate legs: zstd spike on the real artifact, A/B parity green for dsh, one live DeepSeek tool-calling round-trip per routed model, full profile traceability
- **Max feedback latency:** 120 seconds (automated legs); the operator-gated legs (live probe, real-artifact decode) report skip/pending loudly, never silently

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 11-01-01 | 01 | 1 | DSH-03 | T-11-03 | zstd decode bounded (WithDecoderMaxMemory), checksums ON | unit | `go test ./internal/profile/ -race -run TestZstd` | ❌ W0 | ⬜ pending |
| 11-01-02 | 01 | 1 | DSH-01 | T-11-01 | grep gate: no zcode mimicry paths, no provider-name switches | unit+script | `go test ./internal/redact/ -race && ./scripts/zcodeism-gate.sh` | ❌ W0 | ⬜ pending |
| 11-02-01 | 02 | 1 | DSH-02 | T-11-02 | profile.System emitted per captured form; loader tolerates absent thinking/tool_choice | unit (TDD) | `go test ./internal/provider/ -race -run TestOpenAI` | ✅ extends | ⬜ pending |
| 11-02-02 | 02 | 1 | DSH-02 | — | always-stream + include_usage; headers sent; tools omitted when empty; strict per capture | unit (TDD) | `go test ./internal/provider/ -race -run TestOpenAI` | ✅ extends | ⬜ pending |
| 11-03-01 | 03 | 2 | DSH-04 | — | per-model profile field resolves; --profile override intact | unit | `go test ./internal/scheduler/ -race -run TestProfile` | ✅ extends | ⬜ pending |
| 11-03-02 | 03 | 2 | DSH-04 | T-11-04 | profile check dsh drift path; seed sanitized (leak-guard) | unit | `go test ./cmd/ass-guard/ -race -run TestProfileCheck` | ✅ extends | ⬜ pending |
| 11-04-01 | 04 | 2 | DSH-03 | T-11-05 | proxy records + redacts at record time (Authorization → [REDACTED]); 0600 | unit | `go test ./cmd/dsh-proxy/ -race` (or tool pkg) | ❌ W0 | ⬜ pending |
| 11-05-01 | 05 | 3 | DSH-03 | — | dsh extraction: every entry carries capture-line source | unit | `go test ./cmd/extract-profile/ -race -run TestDsh` | ❌ W0 | ⬜ pending |
| 11-06-01 | 06 | 3 | DSH-05 | — | parity suite extraction from dsh capture; arm provider-agnostic | unit | `go test ./internal/parity/ -race -run TestDsh` | ✅ extends | ⬜ pending |
| 11-06-02 | 06 | 3 | DSH-05 | T-11-06 | live round-trip loud-skip-with-note when credential absent | integration | `go test ./internal/parity/ -race` + parity run | ✅ extends | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*
*(Task IDs provisional at strategy-creation time — finalized against the written plans; the planner keeps every task's `<verify><automated>` aligned with this map.)*

---

## Wave 0 Requirements

- [ ] `internal/profile/zstd_test.go` — synthetic round-trip fixture (multi-frame EncodeAll→DecodeAll) + real-artifact skip-clean test — covers DSH-03
- [ ] zcode-ism grep gate (script or Go test) over `internal/ cmd/` excluding `internal/profile`, `internal/ecosys` — covers DSH-01
- [ ] dsh synthetic capture-line fixture (one chat-completions request JSON) for extractor/check tests — covers DSH-03/04

*Framework install: none — go test already established.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| dsh capture workload (≥5 turns, ≥8 tools, ≥1 subagent, ≥1 empty-output turn) run ONCE in real dsh at the pinned commit | DSH-03 | Usage of the mimicked agent's UI; no CLI/API path (Phase-9 09-04 checkpoint precedent) | Follow `docs/dsh-recapture-runbook.md` (built this phase); resume signal carries the pinned commit + capture path |
| One live DeepSeek tool-calling round-trip per routed model | DSH-05 | Consumes the operator-held gateway credential | Export `OPENCODE_GATEWAY_KEY` (per D-01), run the probe command from the plan; absent key → loud skip + pending note |
