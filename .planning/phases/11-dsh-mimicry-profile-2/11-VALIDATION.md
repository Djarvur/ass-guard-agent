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
| 11-01-T1 | 01 | 1 | DSH-01 | T-11-02 | identity-preserved names never override canonical secretKeys | unit (tdd) | `go test ./internal/redact/ -race` | ✅ extends | ⬜ pending |
| 11-01-T2 | 01 | 1 | DSH-01 | T-11-01 | grep gate: unlabeled zcode refs + provider-name switches fail | unit+script | `./scripts/zcodeism-gate.sh` | ❌ W0 | ⬜ pending |
| 11-01-T3 | 01 | 1 | DSH-01 | T-11-03 | census keeps exclusions reviewable | script+docs | `./scripts/zcodeism-gate.sh && go test ./... ` | ✅ | ⬜ pending |
| 11-02-T1 | 02 | 1 | DSH-02 | — | wire fidelity from profile data only | unit (tdd) | `go test ./internal/provider/ -race -run TestOpenAI` | ✅ extends | ⬜ pending |
| 11-02-T2 | 02 | 1 | DSH-02 | — | tolerant loading both directions | unit (tdd) | `go test ./internal/profile/ -race -run TestLoader` | ✅ extends | ⬜ pending |
| 11-02-T3 | 02 | 1 | DSH-02 | T-11-04, T-11-06 | defensive SSE parse; ctx-cancellable stream; header values templated | unit (tdd) | `go test ./internal/provider/ -race -run TestOpenAI` | ✅ extends | ⬜ pending |
| 11-03-T1 | 03 | 1 | DSH-03 | T-11-07, T-11-09, T-11-10 | redact-at-record; 0600; 127.0.0.1 bind; body bounds | unit (tdd) | `go test ./cmd/dsh-proxy/ -race` | ❌ W0 | ⬜ pending |
| 11-03-T2 | 03 | 1 | DSH-03 | T-11-08 | recording-contract conformance pinned by test | unit (tdd) | `go test ./cmd/dsh-proxy/ -race -run Contract` | ❌ W0 | ⬜ pending |
| 11-04-T1 | 04 | 2 | DSH-04 | T-11-11 | load-time profile-name validation | unit (tdd) | `go test ./internal/scheduler/ -race -run 'TestProfile\|TestModelConfig'` | ✅ extends | ⬜ pending |
| 11-04-T2 | 04 | 2 | DSH-04 | — | swappability assertion; dialect as Limitations data | unit | `go test ./internal/scheduler/ -race -run 'TestDefault\|TestLoad'` | ✅ extends | ⬜ pending |
| 11-04-T3 | 04 | 2 | DSH-04 | T-11-12, T-11-13 | credential never printed; transport/dialect FAIL classified | unit | `go test ./cmd/ass-guard/ -race -run TestProbe` | ❌ W0 | ⬜ pending |
| 11-05-T1 | 05 | 2 | DSH-03 | T-11-15, T-11-17 | exact v1.19.2 pin enforced; checksums ON; MaxMemory bound | unit (tdd) | `go test ./internal/profile/ -race -run TestZstd` | ❌ W0 | ⬜ pending |
| 11-05-T2 | 05 | 2 | DSH-03 | T-11-14, T-11-16 | intra-session inconsistency STOPs; provenance cited per entry | unit (tdd) | `go test ./internal/profile/ -race -run 'TestDsh\|TestExtractDsh'` | ❌ W0 | ⬜ pending |
| 11-05-T3 | 05 | 2 | DSH-03 | — | manual-only re-capture (D-02); unkeyed extraction | docs | `test -f docs/dsh-recapture-runbook.md && grep -c ...` | ❌ W0 | ⬜ pending |
| 11-06-T1 | 06 | 3 | DSH-03 | — | operator capture checkpoint (no automated leg by design) | checkpoint | resume-signal + file sanity | — | ⬜ pending |
| 11-06-T2 | 06 | 3 | DSH-03 | T-11-18 | thresholds mechanical; real-artifact spike PASS; census before bundle | script+test | `ASSGUARD_DSH_SESSION_ZST=<p> go test ./internal/profile/ -run TestZstdReal -v` | ❌ W0 | ⬜ pending |
| 11-06-T3 | 06 | 3 | DSH-03 | T-11-21 | bundle = extractor output only | integration | `ls profiles/dsh/ && go test ./... -race && mise run ci` | ❌ (produced by run) | ⬜ pending |
| 11-06-T4 | 06 | 3 | DSH-04 | T-11-19 | seed sanitized; leak-guard green; values spot-checked | unit+script | `go test ./internal/defaults/ -race` | ✅ extends | ⬜ pending |
| 11-07-T1 | 07 | 4 | DSH-04 | — | per-profile loaders; drift canary proves detection | unit (tdd) | `go test ./cmd/ass-guard/ -race -run TestProfileCheck` | ✅ extends | ⬜ pending |
| 11-07-T2 | 07 | 4 | DSH-05 | T-11-25 | malformed pairs error (no silent suite shortening) | unit (tdd) | `go test ./internal/parity/ -race -run TestDsh` | ❌ W0 | ⬜ pending |
| 11-07-T3 | 07 | 4 | DSH-05 | T-11-22, T-11-23, T-11-24 | fresh baseline only; outcomes recorded; keys never printed | integration | `mise run ci && go test ./... -race` | ✅ extends | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

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
