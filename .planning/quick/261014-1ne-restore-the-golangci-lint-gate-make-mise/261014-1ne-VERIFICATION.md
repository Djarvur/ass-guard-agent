---
phase: quick-261014-1ne
verified: 2026-10-13T23:51:34Z
status: passed
score: 5/5 must-haves verified
covered_digest: "v1:sha256:9460c2b24e06739b603a7a3794d57aa473de03678773a26fd64e7afefdf7c191"
behavior_unverified: 0
overrides_applied: 0
covered_files:
  - .golangci.yml
  - .planning/quick/261014-1ne-restore-the-golangci-lint-gate-make-mise/261014-1ne-PLAN.md
  - .planning/quick/261014-1ne-restore-the-golangci-lint-gate-make-mise/261014-1ne-SUMMARY.md
  - cmd/ass-guard/resume_flags_test.go
  - cmd/extract-profile/main.go
  - internal/acp/available_commands_test.go
  - internal/acp/cancel_session_test.go
  - internal/acp/emitter_soak_test.go
  - internal/acp/emitter_test.go
  - internal/acp/handlers_test.go
  - internal/acp/replay_test.go
  - internal/acp/request_registry_test.go
  - internal/acp/server_test.go
  - internal/acp/session_family_test.go
  - internal/acpserve/acp_serve.go
  - internal/acpserve/ask_drain_test.go
  - internal/acpserve/ask_surface_test.go
  - internal/acpserve/checkpoint_options_test.go
  - internal/acpserve/commands_seam_test.go
  - internal/acpserve/config_surface.go
  - internal/acpserve/config_test.go
  - internal/acpserve/kill9_test.go
  - internal/acpserve/kit_advisory_wire_test.go
  - internal/acpserve/kit_ask_wire_test.go
  - internal/acpserve/kit_checkpoint_live_test.go
  - internal/acpserve/kit_e2e_opsx_matrix_test.go
  - internal/acpserve/kit_e2e_opsx_test.go
  - internal/acpserve/kit_emitter_e2e_test.go
  - internal/acpserve/kit_integration_test.go
  - internal/acpserve/kit_planmode_wiring_test.go
  - internal/acpserve/kit_wire_helpers_test.go
  - internal/acpserve/modesmatrix_interactive_test.go
  - internal/acpserve/permissions_e2e_test.go
  - internal/acpserve/resume_test.go
  - internal/acpserve/serve_test.go
  - internal/acpserve/simulator_e2e_test.go
  - internal/acpserve/tombstone_grace_test.go
  - internal/acpserve/toolkit.go
  - internal/coreexec/ansistrip.go
  - internal/coreexec/ask_test.go
  - internal/coreexec/background.go
  - internal/coreexec/background_test.go
  - internal/coreexec/bash.go
  - internal/coreexec/bash_test.go
  - internal/coreexec/fixtures_test.go
  - internal/coreexec/hooks_wiring_test.go
  - internal/coreexec/planmode_test.go
  - internal/coreexec/ptty.go
  - internal/coreexec/ptty_test.go
  - internal/coreexec/todo_test.go
  - internal/ecosys/expand.go
  - internal/ecosys/hooks_test.go
  - internal/ecosys/hookverdict_test.go
  - internal/ecosys/memory_test.go
  - internal/evalharness/harness.go
  - internal/evalsuite/suite_test.go
  - internal/modelroutingcmd/modelrouting.go
  - internal/openspec/patterntable_test.go
  - internal/openspec/register.go
  - internal/paritycli/parity.go
  - internal/perm/rules.go
  - internal/perm/rules_internal_test.go
  - internal/perm/rules_test.go
  - internal/sandbox/child_linux.go
  - internal/sandbox/landlock_linux.go
  - internal/sandbox/policy.go
  - internal/sandbox/policy_test.go
  - internal/tasks/notify.go
  - internal/tasks/notify_test.go
  - internal/tasks/subagent.go
  - internal/tasks/subagent_test.go
  - internal/tasks/tracker.go
  - internal/tasks/tracker_test.go
  - kit/checkpoint/store.go
  - kit/checkpoint/store_test.go
  - kit/engine/decide.go
  - kit/engine/observe.go
  - kit/engine/observe_test.go
  - kit/hookdag/config.go
  - kit/internal/enginebridge/enginebridge.go
  - kit/mcp/host_test.go
  - kit/mcp/lifecycle_test.go
  - kit/profile/loader_test.go
  - kit/provider/anthropic_test.go
  - kit/provider/conformance_test.go
  - kit/provider/streaming.go
  - kit/runtime/acp_adapter_shim_test.go
  - kit/runtime/acp_engine_e2e_test.go
  - kit/runtime/advisory_wiring_test.go
  - kit/runtime/ask_wiring_test.go
  - kit/runtime/commands.go
  - kit/runtime/commands_test.go
  - kit/runtime/coreexec_wiring_test.go
  - kit/runtime/cron_wiring.go
  - kit/runtime/cron_wiring_test.go
  - kit/runtime/engine_setup_test.go
  - kit/runtime/fakes_test.go
  - kit/runtime/hooks_join_wiring_test.go
  - kit/runtime/hostproof_test.go
  - kit/runtime/img_capability_test.go
  - kit/runtime/imgscale_test.go
  - kit/runtime/mcp_tracer_test.go
  - kit/runtime/mention_expand_test.go
  - kit/runtime/modesmatrix_wake_test.go
  - kit/runtime/restore_guard_test.go
  - kit/runtime/runtime.go
  - kit/runtime/seams.go
  - kit/runtime/session_opener_test.go
  - kit/runtime/steering_ingress_test.go
  - kit/runtime/subagent_tier_wiring_test.go
  - kit/runtime/toolkit_twin_test.go
  - kit/runtime/wake_wiring_test.go
  - kit/session/ask.go
  - kit/session/ask_test.go
  - kit/session/askqueue_test.go
  - kit/session/compaction.go
  - kit/session/compaction_test.go
  - kit/session/elicitation_reply_test.go
  - kit/session/end_to_end_test.go
  - kit/session/gate_test.go
  - kit/session/list_test.go
  - kit/session/modesmatrix_subagent_test.go
  - kit/session/projector_test.go
  - kit/session/reconstruction_test.go
  - kit/session/session.go
  - kit/session/session_test.go
  - kit/session/steering_test.go
  - kit/session/steerqueue_test.go
  - kit/session/subagent.go
  - kit/session/subagent_test.go
  - kit/session/thinking_test.go
  - kit/shaper/midturn_test.go
  - kit/shaper/shaper.go
  - kit/shaper/shaper_test.go
  - kit/toolcat/catalog.go
  - kit/toolcat/mutability.go
  - kit/toolcat/types.go
  - kit/toolexec/batch.go
  - kit/toolexec/iserror_corpus_test.go
---

# Quick Task 261014-1ne: Restore the golangci-lint gate — Verification Report

**Task Goal:** Restore the golangci-lint gate: make `mise run lint` exit 0 (655 findings at master) via a config+code hybrid — disable/tune 2.13-added opinionated linters with rationale comments in .golangci.yml (house precedent: the 24-01 baseline repair), add test-file exclusions for fixture-noisy classes, and mechanically fix genuine code findings; inspect-and-dispose the real-signal classes (nilerr, contextcheck, noctx, staticcheck, unused, forcetypeassert) without weakening correctness gates.

**Verified:** 2026-10-13T23:51:34Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Cache-clean `mise run lint` exits 0 on the full repo — backlog disposed, not suppressed wholesale | ✓ VERIFIED | Verifier re-ran the gate at HEAD (72932fb): `mise exec -- golangci-lint cache clean && mise run lint` → exit 0, "0 issues." (only the documented exhaustruct deprecation warning). Not wholesale suppression: total nolint directives went DOWN 1269 → 1089 across the range (115 added, ~295 stale deleted); config disables are exactly the 7 opinionated classes; all real-signal linters stay enabled |
| 2 | Every disable/tune/exclusion in .golangci.yml carries a dated (2026-10-14) rationale citing the repo invariant | ✓ VERIFIED | Read .golangci.yml in full: all 7 disables (funcorder, paralleltest, nonamedreturns, wrapcheck, err113, nlreturn, testpackage), cyclop 20 (measured-complexity comment), funlen lines 80 (gateCall 63-line comment), 4 test exclusions (lll/goconst/tagliatelle/noctx), gocritic sizeThreshold 129 (>= semantics comment) — every entry dated "2026-10-14 gate restoration" citing its invariant (parity-pinned layout, sequential tests, 22-07 defer-recovery, 19-02 seams, ClassifyHTTP, Phase 15 injection) in the 24-01 style |
| 3 | Every nolint directive genuinely used and carries a same-line reason — nolintlint zero unused | ✓ VERIFIED | nolintlint is enabled (default:all, not in disable list) and the gate is green — unused directives would be findings, and there are 0. Reason scan: 0 real directives without a same-line `//` reason (only 2 prose mentions inside doc comments). Zero `nolint:unused`, `nolint:staticcheck`, `nolint:forcetypeassert`, `nolint:embeddedstructfieldcheck` added — those classes were fixed, not silenced |
| 4 | go build / go vet exit 0; -race green on touched packages; 4 documented pre-existing -race flakes untouched and unskipped | ✓ VERIFIED | `mise run build` → 0, `mise run vet` → 0 (verifier-run). -race -count=1 green on 4 sampled touched packages: kit/checkpoint (2.2s), kit/runtime (46.7s), internal/sandbox (1.0s), internal/coreexec (12.7s — the flake-family package, green). Diff adds zero t.Skip. Full-repo -race not re-run per the verification contract (sample scope); the 4 documented pre-existing failures were not exercised, not fixed, not skipped |
| 5 | Real-signal classes site-inspected: genuine bugs fixed with test, deliberate patterns suppressed site-by-site with reasons, byte-pinned layouts never reordered | ✓ VERIFIED | nilerr ×2: nolints at kit/checkpoint/store.go:642 and kit/provider/streaming.go:320 match the plan's pre-analysis; adjacent comments read by verifier and they match the code (WalkDir skip-unreadable; envelope degrade). contextcheck: nolints at every census site (commands.go:1322/1327, runtime.go:1168/1170/2186/2434, cron_wiring.go:105/429) citing 23-04/22-08 doctrines. noctx: ptty.go:204 `exec.Command("sh")` nolint cites 22-04 persistent-shell doctrine. staticcheck ×5: De Morgan rewrites visible in diff, no nolints added. unused ×6: deleted (0 nolint:unused in tree). forcetypeassert ×2: comma-ok fixes (`addr, aok := ln.Addr().(*net.TCPAddr)` ×2). errchkjson ×4: json.Marshal errors now captured (lerr/rerr/qerr in diff). embeddedstructfieldcheck ×4: blank-line separations in diff, zero field reorders (diff scan of struct declarations found none). No genuine bug surfaced, so the red/green clause is vacuously satisfied. All these linters remain enabled, so the green gate proves zero surviving findings in them |

**Score:** 5/5 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `.golangci.yml` | 7 dated disables, cyclop 20/funlen 80 tuning, 4 test exclusions, gocritic 129 thresholds | ✓ VERIFIED | Diff 0f06e1b..HEAD touches exactly the planned entries and nothing else; `golangci-lint config verify` passes on 2.13.2 (verifier-run) |
| `261014-1ne-deferred-items.md` | Intentional non-fixes + observations recorded | ✓ VERIFIED | Exists in the task directory: mise floating-pin root cause, exhaustruct deprecation, stale-directive purge, the 4 documented -race failures, per-site nolint rationale table, 5 mechanism observations |
| Modified .go files disposing every finding class | 655 findings disposed | ✓ VERIFIED | 137 files changed (625+/621-); gate reports 0 issues; classes from the census individually confirmed disposed (fix or reasoned site nolint) |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| .golangci.yml disable names | golangci-lint 2.13.2 linter set | config schema | ✓ VERIFIED | `config verify` exit 0 on 2.13.2; disabled classes produce zero findings on the verifier's cache-clean run |
| New nolint directives | suppressed findings | nolintlint unused check | ✓ VERIFIED | nolintlint green at 0; directive count fell 1269 → 1089 (stale sweep deleted more than the sweep added) |
| embeddedstructfieldcheck sites | golden/parity pins | blank-line remedy (order-neutral) | ✓ VERIFIED | Diff shows blank-line insertions only; no field reordering anywhere; no nolint needed |
| Acceptance gate command | CI lint task | `.mise.toml` lint task | ✓ VERIFIED | .mise.toml untouched in the range (empty diff); `lint` = `golangci-lint run`; verifier ran exactly the documented command |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Lint gate (acceptance) | `mise exec -- golangci-lint cache clean && mise run lint` | exit 0, "0 issues." | ✓ PASS |
| Config validity | `mise exec -- golangci-lint config verify` | exit 0 (2.13.2) | ✓ PASS |
| Build | `mise run build` | exit 0 | ✓ PASS |
| Vet | `mise run vet` | exit 0 | ✓ PASS |
| Race: kit/checkpoint | `go test -race -count=1 ./kit/checkpoint/` | ok 2.2s | ✓ PASS |
| Race: kit/runtime | `go test -race -count=1 ./kit/runtime/` | ok 46.7s | ✓ PASS |
| Race: internal/sandbox | `go test -race -count=1 ./internal/sandbox/` | ok 1.0s | ✓ PASS |
| Race: internal/coreexec (flake family) | `go test -race -count=1 ./internal/coreexec/` | ok 12.7s | ✓ PASS |

### Requirements Coverage

N/A — quick task; no REQUIREMENTS.md IDs are mapped to this item. Plan-scope constraint verified instead: zero edits to .planning/WINDOWS.md, STATE.md, or ROADMAP.md in 0f06e1b..HEAD (checked).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | Debt markers (TBD/FIXME/XXX) across all 137 changed files: none; added test skips: none; empty implementations: none applicable (lint/tooling task, no runtime features added) | — | — |

### Test Quality Audit

No requirement-linked test files (tooling task). Disabled-test scan on the diff: zero added skips. The 4 documented pre-existing -race failures remain active and unskipped (verified by absence of added t.Skip and by green -race on the flake-family package).

### Human Verification Required

N/A — infrastructure/tooling task with no user-facing elements. Every must-have truth has direct command evidence (gate run, config verify, build/vet, -race sample, diff inspections). No state-transition or cancellation invariant is asserted by this task's truths.

### Gaps Summary

None. The acceptance gate was independently re-run cache-clean at HEAD and exits 0; the config repair is scoped and auditable (exactly the 7 planned disables + tunings + exclusions, all dated with invariant citations); suppression discipline holds (nolintlint green, directive count net-decreased, no correctness class disabled); real-signal classes were disposed by fix (staticcheck, unused, forcetypeassert, errchkjson, embeddedstructfieldcheck) or by site-specific reasoned nolint (nilerr, contextcheck, noctx) with no field reorders; build, vet, and the -race sample are green with the documented pre-existing failures untouched. The gocritic 129-vs-128 deviation is documented with measured >= comparison semantics and honors the plan's stated intent.

---

_Verified: 2026-10-13T23:51:34Z_
_Verifier: Claude (gsd-verifier)_
