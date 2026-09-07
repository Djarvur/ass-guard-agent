---
phase: 22-background-execution-sandbox-reality
plan: 05
subsystem: sandbox
tags: [landlock, sandbox-exec, seatbelt, os-sandbox, confinement, darwin, linux]

requires:
  - phase: 22 CONTEXT/RESEARCH
    provides: D-04/D-05/D-06 locked decisions, live-verified seatbelt profile text, go-landlock v0.10.0 pin
provides:
  - internal/sandbox package — ONE Policy struct → both backends (landlock V4 linux / sandbox-exec darwin), portable Wrap/WrapCmd entry, strict probe taxonomy (Availability), re-exec child sentinel protocol, D-05 honesty doc
  - go-landlock v0.10.0 dependency (linux-tagged imports only; cgo-free verified)
affects: [22-06, sandbox, coreexec]

tech-stack:
  added: ["github.com/landlock-lsm/go-landlock v0.10.0 (linux-tagged only)"]
  patterns:
    - "one policy two backends: Policy renders LandlockRules rows AND SeatbeltProfile text from the same source — drift impossible by construction (D-06)"
    - "self re-exec sentinel: ass-guard's own binary is the linux sandbox loader (os/exec has no pre-exec child hook)"

key-files:
  created:
    - internal/sandbox/policy.go
    - internal/sandbox/policy_test.go
    - internal/sandbox/seatbelt_darwin.go
    - internal/sandbox/seatbelt_darwin_test.go
    - internal/sandbox/landlock_linux.go
    - internal/sandbox/landlock_linux_test.go
    - internal/sandbox/child_linux.go
    - internal/sandbox/doc.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "Seatbelt subpath filters match RESOLVED paths — /tmp is a symlink to /private/tmp on macOS; SeatbeltProfile EvalSymlinks-resolves every rw path at render (best-effort with ancestor-resolution fallback for not-yet-created dirs) — live-probed discovery, pinned by the live battery"
  - "ProbeABI uses the raw Syscall6(SYS_LANDLOCK_CREATE_RULESET, NULL, 0, VERSION) query — x/sys v0.47.0 ships the syscall number + flags but no wrapper; the version query restricts nothing (docs.kernel.org)"
  - "Darwin WrapCmd does NOT touch Env (no sentinel needed — sandbox-exec IS the loader); linux WrapCmd materializes cmd.Env from os.Environ() when nil so the sentinel + JSON policy vars ride the child"
  - "Child confinement failure exits 1 (fail-closed) vs probe-unavailable degrade (fail-open loudly): the distinction is whether confinement was ever promised"
  - "Availability probe cached per process (sync.Once) — real child probes never run per-exec"

patterns-established:
  - "Targeted denies never deny-default: (allow default) + (deny network*) + (deny file-write*) re-allowed for exactly the D-04 rw triple"
  - "Comment-level grep gate: 'BestEffort' token kept out of the package entirely so the prohibition grep (grep -rc BestEffort internal/sandbox/ == 0) is enforceable"

requirements-completed: [SAND-01]

duration: 55 min
completed: 2026-09-07T22:05:00Z
---

# Phase 22 Plan 05: Sandbox Package Core Summary

**One Policy struct now drives symmetric landlock (linux, strict-probed, child-applied via self re-exec) and sandbox-exec (darwin, in-memory profile) backends behind one portable WrapCmd — with the deny set proven LIVE on this host and the linux leg compile-gated cgo-free.**

## Performance

- **Duration:** 55 min
- **Started:** 2026-09-07T21:10:00Z
- **Completed:** 2026-09-07T22:05:00Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments

- policy.go (portable): Policy{RWPaths, ROSysPaths, DenyNetwork}, DefaultPolicy (D-04 triple + documented ro system set), LandlockRules() rows, SeatbeltProfile() (embedded template, in-memory render, symlink-resolved paths), Availability taxonomy, Handle, Probe/Resolve, Wrap/WrapCmd (substitution-only contract), sentinel constants, IsSandboxChild, ApplySandboxChildHook.
- seatbelt_darwin.go: first-use probe (LookPath + real -p child) with distinct reasons, WrapCommand argv form (`sandbox-exec -p <rendered> <original argv...>`), loud degrade to unconfined.
- LIVE darwin battery: curl connect fails under the profile (network deny), touch outside the rw triple fails EPERM, touch INSIDE tmp succeeds, render creates zero artifacts, PATH-scrubbed probe degrades with a reason.
- landlock_linux.go: strict ProbeABI (raw version query), ENOSYS/EOPNOTSUPP/ABI<4 distinct reasons, ApplyChildRuleset (V4 RestrictPaths RWDirs/RODirs + RestrictNet deny-all, WithRefer OFF).
- child_linux.go: RunSandboxChild (sentinel + JSON policy env → apply → syscall.Exec with sentinels stripped), fail-closed on confinement failure, swapSandboxExec test seam.
- doc.go: the D-05 honesty note (what is/isn't confined, clone-inheritance, MPTCP caveat).

## Verification Evidence

- `go test -race -count=1 ./internal/sandbox/` — PASS (7 batteries incl. live seatbelt).
- `GOOS=linux CGO_ENABLED=0 go build ./...` — PASS (go-landlock v0.10.0 in go.mod, cgo-free).
- `GOOS=linux CGO_ENABLED=0 go vet ./internal/sandbox/` — PASS (linux-tagged tests compile).
- `go build ./...` + `go vet ./internal/sandbox/` on darwin — PASS; zero go-landlock references in darwin/portable files.
- `grep -rc 'BestEffort' internal/sandbox/` — 0 matches.

## Deviations from Plan

- **[Rule 3 — environment reality] Seatbelt path resolution.** Found during: Task 1 GREEN live battery (inside-tmp write failed EPERM). Issue: seatbelt subpath filters match RESOLVED paths; /tmp → /private/tmp symlink made the re-allow never match. Fix: resolveForSeatbelt (EvalSymlinks best-effort + ancestor fallback) inside SeatbeltProfile; live battery now proves inside-writes succeed. Verified: full live battery green.
- **[Rule 3 — dependency reality] ProbeABI via raw Syscall6.** x/sys v0.47.0 ships SYS_LANDLOCK_CREATE_RULESET + LANDLOCK_CREATE_RULESET_VERSION but no LandlockCreateRuleset wrapper; the raw call implements the same docs.kernel.org probe.

**Total deviations:** 2 auto-fixed (Rule 3). **Impact:** none — the portable API surface matches the plan exactly; 22-06 consumes Wrap/Probe unchanged.

## Issues Encountered

None.

## Self-Check: PASSED

## Next Phase Readiness

Ready for 22-06 (the flag + three exec-site wraps consume Wrap/Probe/Availability; nothing in coreexec/acpserve/cmd was touched by this plan).
