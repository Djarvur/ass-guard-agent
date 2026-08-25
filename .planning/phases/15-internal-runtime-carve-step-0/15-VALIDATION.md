---
phase: 15
slug: internal-runtime-carve-step-0
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-08-26
---

# Phase 15 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard testing + `-race`; testify in some suites; orchestrated via mise |
| **Config file** | `.mise.toml` (`[tasks.ci]` = vet → lint → build → test); no test-framework config file beyond go.mod |
| **Quick run command** | `go build ./... && go vet ./... && go test ./internal/runtime/... ./internal/acpserve/... ./cmd/ass-guard/ -count=1` (<60s target) |
| **Full suite command** | `mise ci` (vet + `golangci-lint run` + `CGO_ENABLED=0 go build ./...` + `go test -race -count=1 ./...`) |
| **Estimated runtime** | ~60 seconds quick; ~3–5 minutes full race suite |

---

## Sampling Rate

- **After every task commit:** Run quick command (build + vet + touched-package tests)
- **After every plan wave:** Run `mise ci` — the RUNT-01 equivalence proof at each boundary
- **Before `/gsd-verify-work`:** Full suite must be green + test-count ledger match
- **Max feedback latency:** 60 seconds (quick), ~5 minutes (full)

---

## Per-Task Verification Map

RUNT-01's proof is *equivalence*: the existing suite passes unchanged from new homes. No new feature tests; two cheap guards (Wave 0).

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RUNT-01 | All 130 existing test functions still pass, relocated | regression (existing suite) | `mise ci` | ✅ (baseline: 130 `^func Test` in cmd today — ledger captured pre-move) |
| RUNT-01 | Relocated tests execute in their new packages | structural | `go test ./internal/runtime/... ./internal/acpserve/... -count=1 -v \| grep -c '^=== RUN'` compared to ledger | ❌ Wave 0 ledger artifact |
| RUNT-01 | CLI contract unchanged (commands + flags byte-identical) | contract smoke | golden-list test asserting root/acp/checkpoint/learning/model-routing/profile/parity command+flag sets | ❌ Wave 0 recommended addition (~80 lines; protects success criterion #2 mechanically) |
| RUNT-01 | Binary handshake still clean (stdout = ACP frames only) | e2e smoke (exists) | `go test ./cmd/ass-guard/ -run TestZeroConfigFirstRun -count=1` (builds binary, spawns `acp serve`, no keys needed) | ✅ zeroconfig_test.go |
| RUNT-01 | Diff is pure relocation | review aid (manual) | `git diff --color-moved=plain master` + moved-line ratio in PR description | manual-only — justified: reviewer judgment is the criterion |
| RUNT-01 | Live Zed session identical (criterion #2) | manual-only | operator's daily-use session | manual-only — justified: requires editor + live credentials; automated coverage above (handshake smoke + full race suite) bounds the risk |

---

## Wave 0 Gaps

- [ ] Test-count ledger (pre-move): `grep -h "^func Test" cmd/ass-guard/*_test.go \| wc -l` (=130 today) recorded in the plan; post-move sum across cmd + new packages must equal it.
- [ ] CLI-contract golden test (recommended above) — the only suggested *new* test; everything else reuses the standing suite.
- [ ] Framework install: none needed.

---

## Security Domain

`security_enforcement` is not disabled in `.planning/config.json` (key absent → enabled). This is a behavior-preserving refactor, so the security posture is inherited; the phase's duty is *not weakening* it.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | provider credentials handled at internal/provider (untouched); API keys env/file only (project decision) |
| V3 Session Management | no | ACP session lifecycle untouched in internal/acp |
| V4 Access Control | no | permission/gate pipeline arrives in Phase 17; nothing added here |
| V5 Input Validation | inherits | flag parsing stays in cmd (D-09); config loading paths move verbatim |
| V6 Cryptography | no | none in scope |
| V14 Config Hygiene | yes | `warnLooseConfigPerm` (0600 advice) moves verbatim — must not be dropped in the providerfactorycmd split |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| CLI contract drift breaking the editor spawn (or widening it) | Tampering / Elevation | D-09 (cobra stays in cmd) + flag-surface golden test |
| Transport-discipline breach (diagnostics leaking to stdout → corrupt ACP stream) | Information Disclosure | existing stderr discipline preserved verbatim; stdout-clean tests relocate with acpserve and keep guarding |
| Credential-on-disk exposure regression | Information Disclosure | `warnLooseConfigPerm` verbatim move + its existing tests following it |
| Audit-trail degradation during split (mirror/bodyStore dropped) | Repudiation | `startAuditMirror`/bodyStore construction order pinned in acpserve.Run; serve audit/mirror tests cover it |

---

## Security Gate

**ASVS Level:** L1 · **Block on:** high

Each PLAN.md carries a `<threat_model>` block. For this phase the threat model is inheritance-only: no new inputs, no new surfaces, no new authority. The blocking duty is *preservation* — the four patterns above name what must survive the move verbatim.

---

*Phase 15 validation strategy — seeded from RESEARCH.md §Validation Architecture, 2026-08-26*
