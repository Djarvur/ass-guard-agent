---
phase: 16
slug: acp-wire-foundation
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: 2026-08-27
---

# Phase 16 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `-race`; per-package `_test.go`, table tests, fake/writer-injection seams; orchestrated via mise |
| **Config file** | none needed — `go.mod` (go 1.26) + `.mise.toml` (`[tasks.ci]` = vet → lint → build → test); 155 existing `_test.go` files establish conventions |
| **Quick run command** | `go test -race -count=1 ./internal/acp/... ./internal/acpserve/... ./internal/session/... ./internal/runtime/... ./internal/providerfactory/... ./internal/modelrouting/...` (<60s target; every touched package) |
| **Full suite command** | `mise ci` (vet + `golangci-lint run` + build + `go test -race -count=1 ./...`) |
| **Estimated runtime** | ~5–10 seconds per targeted `-run` verify; ~60 seconds quick slice; ~3–5 minutes full `mise ci`; soak (env-gated, NOT in ci) ~2–10 minutes |

---

## Sampling Rate

- **After every task commit:** Run that task's own `<automated>` verify command from the Per-Task Map (targeted `-race` `-run` filters — seconds-scale)
- **After every plan wave:** Run `mise ci` (Wave 1 = plans 01/02/04, Wave 2 = 03, Wave 3 = 05, Wave 4 = 06)
- **Before `/gsd-verify-work`:** Full suite must be green (`mise ci` + `go test -race ./... -count=1`)
- **Max feedback latency:** 60 seconds (targeted task verify), ~5 minutes (wave gate)

---

## Per-Task Verification Map

All 16 tasks across 6 plans carry `<automated>` verify blocks (verified against each PLAN.md this audit). Test files are the RED (failing-test) commit of their owning TDD task — File Exists flips to ✅ as each plan lands.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 16-01-01 | 01 | 1 | ACP-03 | T-16-03 | Emitter drain goroutine is the sole producer of notification frames into the Writer (≤1 in flight — priority-inversion guard); no fabricated frames (prohibition) | tracer e2e (-race) | `go test -race ./internal/runtime/ -run TestTurnEmitterEndToEnd -count=1 && go test ./internal/acp/ -count=1 && go build ./...` | ❌ W0 (emitter_test.go) | ⬜ pending |
| 16-01-02 | 01 | 1 | ACP-03 | T-16-01, T-16-02 | Bounded lanes block-never-drop (D-01); no lock spans an enqueue; stall detector logs loudly, never disconnects (D-03); barrier orders updates-before-response | unit (-race stress, mise ci lane) | `go test -race ./internal/acp/ -run 'TestTurnEmitterPriority\|TestTurnEmitterStall\|TestTurnEmitterBarrier\|TestTurnEmitterEmptyTurn' -count=1 && mise ci` | ❌ W0 | ⬜ pending |
| 16-01-03 | 01 | 1 | ACP-03 | — | v1 vocabulary pinned (no `plan_update`/`configId` drift, Pitfall 7); TodoWrite→plan mapping stays acp-side | unit | `go test ./internal/acp/ -run 'TestPlanFrameFromTodoWrite\|TestThoughtChunkFrame\|TestToolCallUpdateFrame' -count=1 && go test -race ./internal/acp/ ./internal/runtime/ -count=1` | ❌ W0 | ⬜ pending |
| 16-02-01 | 02 | 1 | ACP-03 | T-16-04, T-16-05 | Redaction exclusion type-scoped to raw_thinking (counting-fake zero-calls proof, D-23); thinking bytes byte-identical round-trip; unknown-kind replay tolerance (D-20) | unit (-race) | `go test -race ./internal/session/ -run TestTranscriptNewKinds -count=1 && go test ./internal/session/ -count=1` | ❌ W0 (transcript_newkinds_test.go) | ⬜ pending |
| 16-02-02 | 02 | 1 | ACP-03 | T-16-05 | Projector inert to the new kinds until Phase 19/21 activate them (additive-only guarantee) | unit | `go test ./internal/session/ -run TestProjectorToleratesNewKinds -count=1 && go test ./internal/session/ ./internal/runtime/ -count=1` | ❌ W0 | ⬜ pending |
| 16-03-01 | 03 | 2 | ACP-03 | T-16-06, T-16-07 | UUID v4 ids both directions (D-15); timeout → one retry → fallback ladder (D-14); -32800 immediate cancel; shutdown drain, no write-after-close (Pitfall 8) | unit (-race) | `go test -race ./internal/acp/ -run 'TestRegistryConcurrentResolve\|TestRegistryTimeoutFallback\|TestRegistrySyntheticCancel\|TestRegistryShutdown' -count=1` | ❌ W0 (request_registry_test.go) | ⬜ pending |
| 16-03-02 | 03 | 2 | ACP-03 | T-16-06 | Response frames intercepted before dispatch — zero spurious method-not-found; unknown-id responses logged once and dropped (Pitfall 1) | unit (-race) | `go test -race ./internal/acp/ -run 'TestServeResponseRouting\|TestServeInboundCancelNoOp' -count=1 && go test ./internal/acp/ -count=1` | ❌ W0 (server_test.go) | ⬜ pending |
| 16-03-03 | 03 | 2 | ACP-03 | T-16-08, T-16-09 | Probe payload minimal static (no session content); capability degradation sticky + logged (D-13/D-18); counters visible (D-16) | unit (-race) | `go test -race ./internal/acp/ -run 'TestInitializeProbe\|TestCapabilityStickiness\|TestProbeTimeoutFallback' -count=1 && go test ./internal/acp/ ./internal/acpserve/ -count=1` | ❌ W0 | ⬜ pending |
| 16-04-01 | 04 | 1 | ACP-08 | T-16-10, T-16-11 | Atomic temp+rename write at 0600; failure leaves existing file byte-identical, no temp leftovers (D-07); 0750-max dirs | unit | `go test ./internal/providerfactory/ -run TestConfigWrite -count=1 && go test ./internal/providerfactory/ ./internal/modelrouting/ -count=1` | ❌ W0 (config_write_test.go) | ⬜ pending |
| 16-04-02 | 04 | 1 | ACP-08 | T-16-12 | session_tier additive + defaulted; layer isolation (project write leaves global untouched, D-08 groundwork) | unit | `go test ./internal/modelrouting/ -run 'TestSessionTier\|TestLoad' -count=1 && go test ./internal/providerfactory/ -run TestConfigWrite -count=1` | ❌ W0 | ⬜ pending |
| 16-05-01 | 05 | 3 | ACP-08 | T-16-13 | Menu-whitelist validation before any write (D-09 typed reject); persist-before-apply ordering; no-surface degrade never crashes | unit (-race) | `go test -race ./internal/acp/ -run 'TestSetConfigOption\|TestConfigAdvertise' -count=1 && go test ./internal/acp/ -count=1` | ❌ W0 (handlers_test.go) | ⬜ pending |
| 16-05-02 | 05 | 3 | ACP-08 | T-16-14, T-16-15 | _meta blob fills-unset in-memory only (never persisted — operator files authoritative); no credential option ids; serialized mutation, no torn YAML | unit (-race) | `go test -race ./internal/acpserve/ -run 'TestConfigSurface\|TestMetaBlob\|TestScopeRouting\|TestSetIdempotent' -count=1` | ❌ W0 (config_test.go) | ⬜ pending |
| 16-05-03 | 05 | 3 | ACP-08 | T-16-16 | Persist-then-apply live seam: mid-turn Set lands between turns (no torn stamp); cross-provider tier switch degrades loudly | unit (-race) + integration | `go test -race ./internal/acpserve/ -run 'TestLiveModelApply\|TestTierSwitch' -count=1 && go test -race ./internal/runtime/ -run TestApplyTurnModel -count=1 && go test ./internal/session/ -count=1` | ❌ W0 | ⬜ pending |
| 16-06-01 | 06 | 4 | ACP-03 + ACP-08 | T-16-17 | Whole-surface deterministic story in v1 vocabulary only (highest-level Pitfall-7 drift guard); updates-before-response + cascade-before-response | e2e (-race) | `go test -race ./internal/acpserve/ -run TestZedSimulatorE2E -count=1 && go test ./internal/acpserve/ -count=1` | ❌ W0 (simulator_e2e_test.go) | ⬜ pending |
| 16-06-02 | 06 | 4 | ACP-03 | T-16-17 | Adversarial soak closing invariants: no drop, bg FIFO, no leak, clean close, stall-detector fired (D-04) | soak (env-gated, OUT of ci) | `ASSGUARD_EMITTER_SOAK=1 go test ./internal/acp/ -run TestTurnEmitterSoak -count=1 -timeout 10m && mise run emitter-soak && mise ci` | ❌ W0 (emitter_soak_test.go) | ⬜ pending |
| 16-06-03 | 06 | 4 | ACP-03 + ACP-08 | T-16-18 | Checkpoint disposition honesty: exactly one line-initial marker in the SUMMARY — an unconfirmed claim cannot pass silently | checkpoint guard (grep) + manual | `[ "$(grep -cE '^(PENDING-OPERATOR-CONFIRMATION\|OPERATOR-CONFIRMED)' .planning/phases/16-acp-wire-foundation/16-06-SUMMARY.md)" -eq 1 ]` | ❌ W0 (16-06-SUMMARY.md) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements — no standalone Wave 0.

- Framework, config, and runner: already in place (Go stdlib `testing` + `-race`, `mise ci` gate, 155 existing test files, pipe/fake-writer/`t.TempDir` fixture conventions per 16-PATTERNS.md).
- Test-file creation is inlined into each plan's RED-first TDD task (every task commits its failing test before its implementation); the plans pin exact file paths and test names (the ❌ W0 cells above). No stubbing layer is needed between the plans and the infrastructure.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live Zed native rendering: tool cards w/ diff, TodoWrite plan panel, streaming tokens (ROADMAP criterion 1) + settings-UI options with truthful current values and editor-driven model switch (criterion 4's human half) | ACP-03, ACP-08 | Human visual judgment is the criterion; the deterministic simulator (16-06-01) proves the wire-correct substrate and bounds the risk, but only the operator can confirm native rendering | 16-06-PLAN Task 3 `how-to-verify` steps (a)–(e), including the thought-chunk exclusion note; disposition recorded as line-initial `OPERATOR-CONFIRMED` or `PENDING-OPERATOR-CONFIRMATION` in 16-06-SUMMARY.md → WINDOWS ledger (15-07 pattern) |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies — all 16 tasks carry automated verify blocks (16-06-03's grep guard + the manual checkpoint row above)
- [x] Sampling continuity: no 3 consecutive tasks without automated verify — zero tasks lack one
- [x] Wave 0 covers all MISSING references — no standalone Wave 0 required (TDD-inline; see above)
- [x] No watch-mode flags — every command is `-count=1` (soak uses `-timeout 10m`, a bound not a watch; the `-count=3` flakiness check in 16-06-01 acceptance prose is not a verify command)
- [x] Feedback latency < 60s targeted / ~5 min wave gate
- [x] `nyquist_compliant: true` set in frontmatter — every task has automated verification; the single manual-only row is the strategy-sanctioned operator checkpoint (Phase 15 precedent: manual-only rows do not flip compliance when automated coverage bounds the behavior)

**Approval:** approved 2026-08-27

---

## Validation Audit 2026-08-27

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

**Run mode:** State A on the seeded draft — VALIDATION.md existed (status: draft), no SUMMARY files (phase planned, not yet executed). Classification basis is therefore each PLAN.md's designed verification (verify blocks, acceptance criteria, test-name pins), not executed test runs. No gaps → workflow §3 no-gap path: §4 (gap-plan gate) and §5 (nyquist-auditor spawn) not reached; an auditor pre-execution would have no implementation to test — test generation is each plan's own RED task.

**Coverage cross-reference:** RESEARCH.md §Validation Architecture test map fully accounted for in plans — TestTurnEmitterPriority/Stall (16-01-02), registry trio (16-03-01), TestServeResponseRouting (16-03-02), TestSetConfigOption (16-05-01), TestConfigWrite (16-04-01, homed in providerfactory), TestTranscriptNewKinds (16-02-01), env-gated soak (16-06-02), live-Zed manual row (16-06-03).

**Plan findings (recorded, not fixed — validation-only run):**

1. **F-1 (minor, 16-05):** Task 3's action prose says "inject via `WithConfigServer` option" while the frontmatter artifacts, key_links, and must_haves all say `WithConfigSurface`. One inconsistent alias; executor ambiguity risk only.
2. **F-2 (trivial, 16-01):** Task 1's `read_first` contains the typo "internal/acp/frimer.go is framer.go" — self-correcting in the same line.
3. **F-3 (drift, RESEARCH → plans):** RESEARCH's test map names `TestInitializeCapabilities` (golden vs v1 schema); the plans realize that coverage as `TestInitializeProbe` (16-03-03) + `TestConfigAdvertise` (16-05-01) + the simulator's initialize assertions (16-06-01) — equivalent coverage under different names. Likewise RESEARCH suggested `TestConfigWrite` in modelrouting; the plan homes it in providerfactory (decided, fine). RESEARCH's quick-run command omits `./internal/runtime/...`, `./internal/providerfactory/...`, `./internal/modelrouting/...` which this phase touches — the Sampling Rate section above uses the corrected package set.
4. **F-4 (scope note, criterion 4):** ROADMAP criterion 4 asks for initialize/new/load/resume capability advertisement "verified against the ACP schema in a real Zed handshake." Phase 16 delivers initialize/new (16-05, with `agentCapabilities.loadSession: false` advertised truthfully) and defers load/resume responses to Phase 18 per CONTEXT D-05's apply-as-landed split; the live-Zed handshake verification rides the 16-06-03 operator checkpoint items (d)/(e). Deliberate, not a miss — recorded so milestone audit reads criterion 4 as satisfied-at-Phase-16-scope.

*Phase 16 validation strategy — validated pre-execution against plan verify design, 2026-08-27.*
