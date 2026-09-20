---
phase: 19
slug: compaction-cache-control
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-08-27
---

# Phase 19 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing go.mod |
| **Quick run command** | `go test ./internal/... -count=1 -race` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/... -count=1 -race`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

Regenerated (revision 1) from the 5 plans' actual `<verify><automated>` commands — 13 tasks total.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 19-01-01 | 01 | 1 | PAR-02 | T-19-01 | Flag-gated emission; flag-off output byte-identical (tamper surface unchanged) | unit (tracer) | `go test ./internal/shaper/ ./internal/profile/ -run 'CacheControl' -count=1 && go test ./internal/shaper/ ./internal/profile/ -count=1 && go vet ./internal/shaper/ ./internal/profile/` | ✅ extend `internal/shaper/shaper_test.go` | ⬜ pending |
| 19-01-02 | 01 | 1 | PAR-02 | T-19-02 (probe-integrity leg) | Baseline byte-stable vs pre-phase ref 73440bd — green comes from real emission, never a moved baseline | unit | `go test ./internal/paritycli/ -run 'TestCacheProbe' -count=1 && go test ./internal/paritycli/ ./internal/parity/ -count=1 && test -z "$(git diff --name-only 73440bd -- internal/parity/cacheprobe.go)"` | ✅ extend `internal/paritycli/parity_test.go` | ⬜ pending |
| 19-01-03 | 01 | 1 | PAR-02 | T-19-02 | Keep-last-4 degrade pins the 4-breakpoint API cap (400-DoS avoidance) | unit | `go test ./internal/shaper/ -run 'CacheControl|Golden|Fidelity' -count=1 && go test ./internal/profile/ -count=1 && go test -race ./internal/shaper/ ./internal/profile/ ./internal/paritycli/ -count=1` | ✅ extend `shaper_test.go`, `midturn_test.go`, `fidelity_test.go` | ⬜ pending |
| 19-02-01 | 02 | 1 | PAR-01 | T-19-03 | Bounded error-body read; malformed body degrades to typed error without panic | unit | `go test -race ./internal/provider/ -run TestStream_Non2xxError -count=1 && go test ./internal/provider/ -count=1 && go vet ./internal/provider/` | ✅ extend `internal/provider/streaming_test.go` | ⬜ pending |
| 19-02-02 | 02 | 1 | PAR-01 | T-19-04 | Message-class-only matcher, no new ErrorKind (retry bounded downstream in 19-04) | unit | `go test ./internal/provider/ -run TestIsOverflow -count=1 && go test ./internal/provider/ -count=1` | ✅ extend `internal/provider/errors_test.go` | ⬜ pending |
| 19-03-01 | 03 | 1 | PAR-01 | T-19-05, T-19-06 | Summary rides the redacted append path (counting-Redactor proof); field-tolerant readers | unit | `go test -race ./internal/session/ -run 'TestTranscriptNewKinds' -count=1 && go test ./internal/session/ -count=1 && go vet ./internal/session/` | ⚠ 16-gated: `internal/session/transcript_newkinds_test.go` arrives with 16-02 | ⬜ pending |
| 19-03-02 | 03 | 1 | PAR-01 | T-19-05 | Durable seed is data in a user-role message; survives later boundaries; no-marker byte-identity | unit | `go test -race ./internal/session/ -run 'TestProjector_CompactionResetPoint|TestProjectorToleratesNewKinds' -count=1 && go test ./internal/session/ -count=1` | ✅ extend `internal/session/projector_test.go` | ⬜ pending |
| 19-03-03 | 03 | 1 | PAR-01 | T-19-07 | Summary estimate accounted against fill target; pair-atomic cut; thinking untouched | unit | `go test -race ./internal/session/ -run 'TestProjector_CompactionTailCut' -count=1 && go test -race ./internal/session/ ./internal/runtime/ -count=1` | ✅ extend `internal/session/projector_test.go` | ⬜ pending |
| 19-04-01 | 04 | 2 | PAR-01 | T-19-08, T-19-09, T-19-10 | Extractive prompt, redacted input, 2048 hard cap, zero client-visible bus publishes | unit | `go test -race ./internal/session/ -run 'TestCompaction' -count=1 && go test ./internal/session/ -count=1 && go vet ./internal/session/` | ❌ RED creates `internal/session/compaction_test.go` | ⬜ pending |
| 19-04-02 | 04 | 2 | PAR-01 | T-19-11 | Per-turn single retry guard (fail-twice ends in the normal error path) | unit | `go test -race ./internal/session/ -run 'TestCompaction_OverflowRetryOnce|TestCompaction_LoopHead|TestCompactNow' -count=1 && go test -race ./internal/session/ ./internal/runtime/ -count=1` | ❌ RED creates | ⬜ pending |
| 19-04-03 | 04 | 2 | PAR-01 | T-19-08…T-19-11 (composed) | Engine composition: marker + seed + pair-safe tail + headroom + replay identity | integration (offline E2E, fake provider) | `go test -race ./internal/session/ -run 'TestCompaction' -count=1 && mise ci` | ❌ RED creates | ⬜ pending |
| 19-05-01 | 05 | 3 | PAR-01 | T-19-12, T-19-13 | Typed validation-first (1..100) precedes any write; whitelisted key set; layer isolation | unit + cross-package round-trip | `go test ./internal/modelrouting/ ./internal/providerfactory/ -run 'Compaction|ConfigWrite' -count=1 && go test -race ./internal/session/ -run 'TestCompaction' -count=1 && go vet ./internal/modelrouting/` | ✅ extend `internal/modelrouting/config_test.go`; ⚠ 16-gated writer from 16-04 | ⬜ pending |
| 19-05-02 | 05 | 3 | PAR-01 | T-19-12, T-19-14 | Persist-then-apply; idempotent no-op on equal value; effective-value round trip | unit | `go test -race ./internal/acpserve/ -run 'Compaction' -count=1 && go test ./internal/acp/ ./internal/acpserve/ ./internal/modelrouting/ -count=1 && mise ci` | ⚠ 16-gated: `internal/acpserve/config_surface.go` + harness arrive with 16-05 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/shaper/shaper_test.go`, `midturn_test.go`, `fidelity_test.go` — extend targets for 19-01 RED steps (files exist)
- [ ] `internal/paritycli/parity_test.go` — extend target for 19-01 Task 2 RED (file exists)
- [ ] `internal/provider/streaming_test.go`, `errors_test.go` — extend targets for 19-02 RED steps (files exist)
- [ ] `internal/session/projector_test.go` — extend target for 19-03 Tasks 2-3 RED (file exists)
- [ ] `internal/session/transcript_newkinds_test.go` — extends 16-02's file; gated on Phase 16 execution (see 19-03 `<execution_precondition>`)
- [ ] `internal/session/compaction_test.go` — NEW; created by 19-04 Task 1 RED (no scaffold needed)
- [ ] `internal/modelrouting/config_test.go` — extend target for 19-05 Task 1 RED (file exists)
- [ ] `internal/acpserve/config_test.go` — extends 16-05's harness; gated on Phase 16 execution (see 19-05 `<execution_precondition>`)

*Every task is tdd="true" with a RED-first step, so test scaffolding is created by the tasks themselves; Wave 0 exists only to confirm the extend targets are present and Phase 16 has executed for the two gated files.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Coherent continuation after compaction | PAR-01 | Long-session end-to-end observation | Run a session past threshold with compaction.enabled; observe continuation |

*If none: "All phase behaviors have automated verification."*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
