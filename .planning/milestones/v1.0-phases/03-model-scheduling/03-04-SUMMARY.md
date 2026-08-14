---
phase: 03-model-scheduling
plan: 04
status: complete
requirements: [SCHED-06, SCHED-01]
key-files:
  created:
    - internal/scheduler/capability.go
    - internal/scheduler/capability_test.go
    - internal/scheduler/integration_test.go
    - cmd/ass-guard/scheduling.go
    - cmd/ass-guard/scheduling_test.go
  modified:
    - internal/scheduler/resolver.go
    - cmd/ass-guard/main.go
---

# Plan 03-04 SUMMARY — Runtime capability gate + operator CLI + e2e integration

## What was built

The phase close-out: the request-time capability gate (completing SCHED-06
alongside Plan-03-01's load-time D-10) + the operator CLI + the end-to-end
integration test.

- **`internal/scheduler/capability.go`** — `CapabilityError`, `applyCapabilityGate`
  (filters [primary, ...fallback] by capReq, returns the first capable candidate
  + the remaining chain), `describeReq`, and `satisfies` (relocated from
  resolver.go so all capability logic lives in one file).
- **`internal/scheduler/resolver.go`** modified — `Resolve` now applies the gate
  after building the primary+chain (and wraps the error with the tier name). A
  zero capReq leaves behavior unchanged (Plan 03-01 preserved, no regression).
- **`cmd/ass-guard/scheduling.go`** — the `scheduling` parent + `validate` +
  `resolve` subcommands. Transport discipline (C1): human-readable →
  `cmd.ErrOrStderr()` (os.Stderr in production); `--json` → `cmd.OutOrStdout()`
  (os.Stdout) — the ONLY stdout path, only when explicitly requested (pitfall 9).
- **`cmd/ass-guard/main.go`** modified — `root.AddCommand(newSchedulingCmd())`.
- **`internal/scheduler/integration_test.go`** — `TestSchedulerEndToEnd`: load →
  InstallSafety → Dispatch (fake provider, primary Transient → fallback success)
  → asserts the response, one ProviderFallback event, and the breaker advancing.

## Decisions honored

- **D-09** request-time capability gate (the runtime half of SCHED-06): a
  tool-using turn skips an incapable primary to the first capable fallback, or
  returns a structured `*CapabilityError` if none satisfy. Pairs with D-10's
  load-time rejection.
- **SCHED-01** operator surface — `ass-guard scheduling validate` (rejects
  inconsistent configs to stderr + non-zero exit) + `scheduling resolve`
  (inspects a tier's resolution; `--json` to stdout, human form to stderr).

## Self-Check: PASSED

- 6 capability + 1 e2e + 5 CLI tests pass -race.
- The FULL project test suite is green (`go test ./... -race`): every package —
  scheduler, provider, event, session, profile, shaper, redact, audit, loop,
  parity, drift, toolcat, acp, cmd — passes. No Phase-1/2 regression.
- Transport discipline: zero direct `os.Stdout` writes in scheduling.go (all
  output via cobra writers; the `--json` branch is the only stdout path).
- `go build ./...` + `go vet ./...` clean.

## Evidence

- Capability gate: `TestCapabilityNeedsToolsSkipsToolLessPrimary` — tool-less
  primary skipped, tool-full fallback chosen.
- No capable candidate: `TestCapabilityNoCandidateReturnsError` — `*CapabilityError`
  naming the requirement + candidates.
- CLI validate: `TestSchedulingValidateInvalid` — non-zero exit + the ConfigError
  report naming `tool_calling` on stderr.
- CLI resolve transport discipline: `TestSchedulingResolveHumanGoesToStderr` —
  stdout EMPTY without `--json`.
- CLI resolve peak window (D-02): `TestSchedulingResolvePeakWindow` — Monday
  10:00 NY resolves heavy to the peak pick (minimax-m3).
- End-to-end: `TestSchedulerEndToEnd` — config → InstallSafety → Dispatch →
  ProviderFallback event observed on the bus + breaker advanced.

## Phase 3 complete

All four plans executed; the full phase goal is met (see VERIFICATION.md):
- SCHED-01 tier abstraction (heavy/good/light → concrete (provider, model)).
- SCHED-02 time-windowed substitution, IANA zones, tzdata bundled.
- SCHED-03 per-project override (D-02 precedence: window → project → global).
- SCHED-04 fallback chains + transient/structural classification (N5 out).
- SCHED-05 circuit breakers (D-07) + cost ceiling (D-08 degrade-then-stop).
- SCHED-06 capability profiles, load-time validation (D-10) + request-time gate.
