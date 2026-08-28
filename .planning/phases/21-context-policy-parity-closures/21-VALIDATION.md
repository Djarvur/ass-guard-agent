---
phase: 21
slug: context-policy-parity-closures
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-08-28
---

# Phase 21 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib testing + stretchr/testify v1.11.1), `-race` always |
| **Config file** | none (Go convention; golangci-lint v2 config in repo root) |
| **Quick run command** | `go test -race -count=1 ./internal/ecosys/ ./internal/session/ ./internal/provider/ ./internal/shaper/ ./internal/runtime/` |
| **Full suite command** | `mise ci` (vet + lint + CGO_ENABLED=0 build + race test) |
| **Estimated runtime** | ~21s per package `-run` filter; `mise ci` ~2-4 min |

---

## Sampling Rate

- **After every task commit:** the owning package's `-run` filter (each < 30s; per-task commands in the plan `<verify>` blocks)
- **After every plan wave:** `go test -race -count=1 ./...`
- **Before `/gsd-verify-work`:** Full suite must be green (`mise ci`)
- **Max feedback latency:** < 30s per task commit

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 21-01-01 | 01 | 1 | PAR-03 | T-21-03/T-21-04 | malformed settings degrades soft, never errors up | unit | `go test -race ./internal/ecosys/ -run 'TestSettingsHooks\|TestHookMatcherDialect' -count=1` | ❌ W0 (extend hooks_test.go) | ⬜ pending |
| 21-01-02 | 01 | 1 | PAR-03 | T-21-01/T-21-02 | project allow demoted; exit-2 overrides JSON; silence never approves | unit | `go test -race ./internal/ecosys/ -run TestHookVerdict -count=1` | ❌ W0 (hookverdict_test.go) | ⬜ pending |
| 21-01-03 | 01 | 1 | PAR-03 | T-21-04 | deny-wins scope order; timeout/cancel fail open | unit | `go test -race ./internal/ecosys/ -run 'TestHookScopeOrder\|TestPreToolUseVerdict' -count=1` | ❌ W0 | ⬜ pending |
| 21-02-01 | 02 | 1 | PAR-04 | T-21-07/T-21-08 | git-root stop; collision rule; caps with loud notes | unit | `go test -race ./internal/ecosys/ -run TestMemoryDiscovery -count=1` | ❌ W0 (memory_test.go) | ⬜ pending |
| 21-02-02 | 02 | 1 | PAR-04 | T-21-06 | injection lands last, shared profile unmutated | integration | `go test -race ./internal/runtime/ -run TestMemoryInjection -count=1` | ❌ W0 (memory_wiring_test.go) | ⬜ pending |
| 21-03-01 | 03 | 2 | PAR-05 | T-21-09 | thinking/redacted deltas → assembled Raw chunks | unit | `go test -race ./internal/provider/ -run TestStreamThinking -count=1` | ❌ W0 (extend streaming_test.go) | ⬜ pending |
| 21-03-02 | 03 | 2 | PAR-05 | T-21-10 | zero redactor calls on thinking path; live forward | unit | `go test -race ./internal/session/ -run 'TestRawThinking\|TestStreamAndEmit' -count=1 && go test -race ./internal/runtime/ -run TestThoughtForward -count=1` | partial (manager_test.go 16-02) / ❌ W0 forwarder | ⬜ pending |
| 21-03-03 | 03 | 2 | PAR-05 | T-21-09/T-21-12 | D-14 field-value identity goldens incl. boundary + redacted | golden | `go test -race ./internal/session/ -run 'TestThinkingGolden\|TestProjector' -count=1 && go test -race ./internal/shaper/ -run TestShaperThinking -count=1` | ❌ W0 (testdata/thinking-golden) | ⬜ pending |
| 21-04-01 | 04 | 3 | PAR-06 | T-21-14/T-21-15 | @dir no recursion; Read-rule gate bites; fixed-form notes | unit | `go test -race ./internal/ecosys/ -run TestParseMentions -count=1 && go test -race ./internal/runtime/ -run TestMentionExpand -count=1` | ❌ W0 | ⬜ pending |
| 21-05-01 | 05 | 4 | PAR-06 | T-21-13/T-21-17 | bomb refused pre-decode; no base64 in transcript; CGO gate | unit+build | `go test -race ./internal/runtime/ -run 'TestImageIngress\|TestValidateAndScale' -count=1 && CGO_ENABLED=0 go build ./... && go test ./internal/session/ -run TestContentBlockRoundTrip -count=1` | ❌ W0 (imgscale_test.go, transcript_newkinds_test.go) | ⬜ pending |
| 21-05-02 | 05 | 4 | PAR-06 | T-21-16 | D-11 drop + exactly one loud note on unsupported provider | unit | `go test -race ./internal/shaper/ -run TestImageCapability -count=1 && go test ./internal/provider/ -count=1` | ❌ W0 | ⬜ pending |
| 21-06-01 | 06 | 5 | PAR-03 | T-21-18/T-21-19 | hook deny/ask/allow through the ONE gate head, both branches | integration | `go test -race ./internal/session/ -run 'TestGateHookVerdict\|TestGatePermissionSuspend' -count=1` | ❌ blocked-on-17 (extends 17-02's gate_test.go — precondition-marked) | ⬜ pending |
| 21-06-02 | 06 | 5 | PAR-03 | T-21-18 | once-only side effects; single-consultation grep audit | unit+audit | `go test -race ./internal/coreexec/ -run 'TestRegisterCore\|TestHooksOnceOnly' -count=1` + filtered greps | ⚠ extend hooks_wiring_test.go (17-gated; TestHooksOnceOnly is new) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/ecosys/testdata/settings-project.json` + `settings-user.json` — settings-hook fixtures (21-01)
- [ ] `internal/ecosys/hookverdict_test.go` — verdict + resolver battery (21-01)
- [ ] `internal/ecosys/memory_test.go` — walker tree fixtures via t.TempDir (21-02)
- [ ] `internal/runtime/memory_wiring_test.go` — sessionFor integration (21-02)
- [ ] `internal/session/testdata/thinking-golden/sse-thinking.jsonl` — captured wire-pair goldens (21-03; A4 capture contingency)
- [ ] 21-03 session-leg test files: `internal/session/thinking_test.go` (NEW), `internal/event/events_test.go` + `internal/runtime/emitter_e2e_test.go` (extend)
- [ ] Image fixtures synthesized in-test (oversized JPEG + IHDR-only pixel-bomb PNG) (21-05)
- [ ] `go get golang.org/x/image@v0.45.0` in 21-05 Task 1 (build-contract gate: CGO_ENABLED=0)
- The 21-06 gate-join test file is 17-02's `gate_test.go` — created by Phase 17, EXTENDED by 21-06 (precondition-marked, not Wave 0)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Thinking renders live in Zed as thought chunks during a real turn | PAR-05 (criterion 4 leg) | requires the live editor + a real provider stream | Run a live Zed session against a thinking-capable model; observe agent_thought_chunk cards streaming |
| Pasting an image reference into the Zed prompt produces the image block in the request | PAR-06 (criterion 5 leg) | requires editor paste + request inspection | Paste an image in a live session; confirm via stderr request log that the image block shipped (or the D-11 note fired) |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter (validate-phase §6)

**Approval:** pending
