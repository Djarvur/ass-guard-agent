---
phase: 22-background-execution-sandbox-reality
plan: "06"
subsystem: sandbox
tags: [sandbox, landlock, re-exec-sentinel, cobra-flag, process-groups, coreexec, ptty, background-tasks, golang]

requires:
  - phase: 22-04
    provides: the PTYManager seam (PTYOpts.NoteFn, ensureShell, Drain) this plan's third exec site wraps
  - phase: 22-05
    provides: internal/sandbox (WrapCmd, Handle, Availability, ApplySandboxChildHook, DefaultPolicy) — this plan is pure wiring against it
  - phase: 22-02
    provides: the TaskRegistry launch funnel (Start + FIFO queued-start) and the Run-pipeline sweep position the probe step joins
provides:
  - The --sandbox flag made real (SAND-01): default OFF, {off,on} validated at startup, startup probe-and-degrade loudly before the scheduler, Handle stored once for the process
  - Confinement at ALL THREE Bash-class exec sites (foreground Bash, TaskRegistry.launch for both start routes, PTYManager.ensureShell) through 22-05's ONE WrapCmd entry
  - The honored dangerouslyDisableSandbox contract (OQ2): unconfined + one numbered stderr note + counter at every site; off-arm stays the documented no-op; the persistent arm falls through to the foreground machinery so a per-call escape never weakens the shared shell
  - The re-exec sentinel main hook (linux): sandbox.ApplySandboxChildHook() as main()'s first statement — the sentinel child never reaches cobra
affects: [verify-phase SAND-01 row, coreexec, acpserve, runtime sessionFor, future exec sites (the three-site review gate)]

actuals:
  tokens: 26284   # chars/4 over the realized diff (105136 chars) — plan estimated 50000
  tasks: 3
  commits: 8      # measured: git rev-list --count 72779533..HEAD
plan_head_before: 7277953347c1b6fb6a7f47924cc99e6f05e0bd39

tech-stack:
  added: []   # no new dependencies — pure wiring over 22-05's go-landlock/seatbelt package
  patterns:
    - "one entry three callers: every Bash-class exec site calls the same wrapSandboxCmd seam -> sandbox.WrapCmd; a fourth exec site without it is a named review-gate violation"
    - "test-binary-as-loader: TestMain (linux-tagged) doubles the coreexec test binary as the landlock re-exec loader, so live batteries run the REAL production wrap path (apply ruleset -> exec target) without building the ass-guard binary"
    - "never-silent fail-open taxonomy: ONE startup warning (composition) + per-run numbered notes + process-wide counter (exec sites) — a green result can never imply confinement that did not happen"

key-files:
  created:
    - internal/coreexec/sandboxchild_test.go
  modified:
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/main.go
    - internal/acpserve/options.go
    - internal/acpserve/acp_serve.go
    - internal/acpserve/serve_test.go
    - internal/coreexec/bash.go
    - internal/coreexec/bash_test.go
    - internal/coreexec/register.go
    - internal/coreexec/background.go
    - internal/coreexec/background_test.go
    - internal/coreexec/ptty.go
    - internal/coreexec/ptty_test.go
    - internal/runtime/runtime.go
    - internal/sandbox/landlock_linux.go
    - internal/sandbox/child_linux.go
    - internal/sandbox/landlock_linux_test.go
    - cmd/ass-guard/background_wiring_test.go

key-decisions:
  - "The flag-path policy grants the WHOLE os.TempDir() (D-04's triple reads tmp as the system tmp) — the live batteries' outside-policy differential therefore uses /var/tmp, not a t.TempDir() sibling (which is INSIDE the rw set)"
  - "The persistent OQ2 arm: a per-call disable cannot unconfine the SHARED shell, so with the sandbox ON the call falls through to the foreground machinery (one-shot unconfined + note) while the shell keeps its confinement — persistence is never bought with another call's boundary"
  - "Zero-value Availability (Mode \"\") reads as OFF at every exec-site gate: confined only when the operator asked, and asking always resolves a backend mode (landlock|seatbelt); bare test configs and off Handles leave argv byte-identical with zero notes"
  - "Runner.SetSandboxHandle late injection (the SetBackgroundCaps precedent): the probe step sits beside the 22-02 sweep before StartScheduler, and sessionFor reads the stored Handle into the registry, PTYOpts, and coreexec.Config — one composition, three consumers"
  - "The linux live arms ran on THIS host (kernel 7.2.4, landlock ABI 10) through the real re-exec path; the darwin legs are compile-gated (GOOS=darwin build+vet green) per the plan's platform-tolerance notes"

patterns-established:
  - "Per-run unconfined notes share ONE process-wide counter (unconfinedRuns) across all three sites — each note individually numbered in the log stream"
  - "The RED evidence pipeline for Go: go test -v projected faithfully into Node-TAP (ok/not ok + summary) so gsd-tools' check tdd-red-evidence validates Go runs"

requirements-completed: [SAND-01]

coverage:
  - id: D1
    description: "--sandbox flag plumbing: default OFF with zero probing, {off,on} validated at startup (never mid-serve), the startup probe step in Run's pipeline before the scheduler with exactly one loud warning on enabled-but-unavailable, the D-04 policy triple, and the re-exec sentinel main hook before root dispatch"
    requirement: SAND-01
    verification:
      - kind: unit
        ref: internal/acpserve/serve_test.go#TestSandboxFlag_Validation
        status: pass
      - kind: unit
        ref: internal/acpserve/serve_test.go#TestSandboxFlag_DefaultOffZeroProbing
        status: pass
      - kind: unit
        ref: internal/acpserve/serve_test.go#TestSandboxFlag_OnProbesOnce
        status: pass
      - kind: unit
        ref: internal/acpserve/serve_test.go#TestSandboxFlag_UnavailableDegradesLoudlyOnce
        status: pass
      - kind: unit
        ref: internal/acpserve/serve_test.go#TestSandboxFlag_ProbeBeforeScheduler
        status: pass
      - kind: unit
        ref: internal/acpserve/serve_test.go#TestSandboxFlag_MainHookFirstCall
        status: pass
      - kind: unit
        ref: internal/acpserve/serve_test.go#TestSandboxFlag_PolicyTriple
        status: pass
    human_judgment: false
  - id: D2
    description: "Foreground Bash confinement through the full flag path: default OFF byte-identical argv (seam-pinned), live landlock confinement (curl connect denied, outside write EPERM, inside-tmp write allowed) through the executor, --sandbox=off always escapes, dangerouslyDisableSandbox honored loudly (OQ2 both arms), availability-false noted per run, group-kill reaches the wrapped child"
    requirement: SAND-01
    verification:
      - kind: unit
        ref: internal/coreexec/bash_test.go#TestBashSandbox_LiveConfinementDeniesNetworkWriteOutside
        status: pass
      - kind: unit
        ref: internal/coreexec/bash_test.go#TestBashSandbox_DefaultOffArgvIdentity
        status: pass
      - kind: unit
        ref: internal/coreexec/bash_test.go#TestBashSandbox_OffAlwaysEscapes
        status: pass
      - kind: unit
        ref: internal/coreexec/bash_test.go#TestBashSandbox_DisableFlagOnRunsUnconfinedWithNote
        status: pass
      - kind: unit
        ref: internal/coreexec/bash_test.go#TestBashSandbox_DisableFlagOffIsNoop
        status: pass
      - kind: unit
        ref: internal/coreexec/bash_test.go#TestBashSandbox_UnavailableNotedPerRun
        status: pass
      - kind: unit
        ref: internal/coreexec/bash_test.go#TestBashSandbox_GroupKillReachesWrappedChild
        status: pass
    human_judgment: false
  - id: D3
    description: "Background + persistent confinement: a backgrounded curl via TaskRegistry.Start (and the FIFO queued-start route) fails its connect under --sandbox=on; the persistent shell is confined at spawn (outside touch fails, inside tmp succeeds, sentinel cycle completes); dead-shell restart re-wraps with the D-08 note once; unavailable noted per run at both sites; Stop's ladder completes on a confined child; the persistent disable escape runs unconfined while the shared shell stays confined; runtime sessionFor threads the Handle to all three consumers"
    requirement: SAND-01
    verification:
      - kind: unit
        ref: internal/coreexec/background_test.go#TestBackgroundSandbox_LiveDeniesNetwork
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestBackgroundSandbox_QueuedStartWrapsIdentically
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestBackgroundSandbox_StopOnConfinedTaskCompletes
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestBackgroundSandbox_DisableAndUnavailableNotes
        status: pass
      - kind: unit
        ref: internal/coreexec/background_test.go#TestBackgroundSandbox_DefaultOffArgvIdentity
        status: pass
      - kind: unit
        ref: internal/coreexec/ptty_test.go#TestPTYSandbox_LiveShellConfined
        status: pass
      - kind: unit
        ref: internal/coreexec/ptty_test.go#TestPTYSandbox_RestartRewrapsWithNoteOnce
        status: pass
      - kind: unit
        ref: internal/coreexec/ptty_test.go#TestPTYSandbox_UnavailableNotedPerRun
        status: pass
      - kind: unit
        ref: internal/coreexec/ptty_test.go#TestPTYSandbox_DisableEscapeRunsUnconfinedShellStaysConfined
        status: pass
      - kind: unit
        ref: internal/coreexec/ptty_test.go#TestPTYSandbox_DefaultOffByteIdentical
        status: pass
    human_judgment: false
  - id: D4
    description: "Cross-platform gates: the linux compile gate (GOOS=linux CGO_ENABLED=0 build + vet over sandbox/coreexec) and the darwin compile gate (GOOS=darwin build + vet) both green — the darwin seatbelt legs are compile-gated per the plan's platform notes while the linux landlock legs ran LIVE on this host (ABI 10)"
    requirement: SAND-01
    verification:
      - kind: other
        ref: "GOOS=linux CGO_ENABLED=0 go build ./... && GOOS=linux CGO_ENABLED=0 go vet ./internal/sandbox/ ./internal/coreexec/"
        status: pass
      - kind: other
        ref: "GOOS=darwin CGO_ENABLED=0 go build ./... && GOOS=darwin CGO_ENABLED=0 go vet ./internal/sandbox/ ./internal/coreexec/ ./internal/acpserve/ ./cmd/ass-guard/"
        status: pass
    human_judgment: false

duration: 32 min
completed: 2026-09-10
status: complete
---

# Phase 22 Plan 6: Sandbox flag made real Summary

**--sandbox plumbing (default OFF, probed loudly before the scheduler) driving ONE WrapCmd entry at all THREE Bash-class exec sites — foreground, TaskRegistry's both launch routes, and the PTY persistent shell — with live landlock confinement proven through the real re-exec path on this host (ABI 10) and the OQ2 per-call escape honored loudly everywhere (SAND-01)**

## Performance

- **Duration:** 32 min
- **Started:** 2026-09-10T00:34:33Z
- **Completed:** 2026-09-10T01:06:47Z
- **Tasks:** 3/3
- **Files modified:** 18 (16 plan-declared + background_wiring_test.go stale-arity fix + sandboxchild_test.go loader)

## Accomplishments

- Task 1: `--sandbox` cobra flag (default "off", `{off,on}` validated at startup), `resolveSandboxAvailability` probe step in Run's pipeline beside the 22-02 sweep and before StartScheduler (off short-circuits with ZERO probing; enabled-but-unavailable warns exactly once), `Runner.SetSandboxHandle` stores the resolved Handle, and main() calls `sandbox.ApplySandboxChildHook()` as its first statement (the sentinel child never reaches cobra)
- Task 2: the foreground site wraps through `confineForeground` BEFORE the group discipline (killGroupOnCtx/reapGroup own the wrapped child); `dangerouslyDisableSandbox` honored (OQ2): on → unconfined + one numbered note + process-wide counter; off → the documented no-op; availability-false → per-run notes, never a silent fail-open; the 12-06 no-op comment retired
- Task 3: `TaskRegistry.launch` wraps EVERY launch (immediate + FIFO queued pop — one funnel), the PTY shell is confined at ensureShell with lazy-restart re-wrap (D-09: persistence and confinement compose), the persistent disable arm falls through to the foreground machinery (the shared shell keeps its confinement), and sessionFor threads the Handle + note sink into the registry, PTYOpts, and Config
- LIVE linux proof through the production re-exec path (the test binary doubles as the landlock loader via a linux-tagged TestMain): curl connect denied, outside write EPERM, inside-tmp write allowed — at all three sites

## Task Commits

Each task was committed atomically (TDD: RED test commit → GREEN feat commit):

1. **Prereq (deviation, Rule 1+3):** `abe3173` (fix) — the 22-05 linux leg made live-runnable
2. **Task 1: flag + probe + main hook** — `e6b3cb3` (test, RED) + `7f94c91` (feat, GREEN)
3. **Task 2: foreground wrap + OQ2** — `7e99639` (test, RED) + `bd9ac5c` (feat, GREEN)
4. **Task 3: background + persistent confinement** — `9998a2c` (test, RED) + `3f8ea2d` (feat, GREEN)
5. **style:** `5637a05` (gofmt the touched files)

**Plan metadata:** this commit (docs: complete plan)

## TDD Gate Compliance

| Gate | Task 1 | Task 2 | Task 3 |
|------|--------|--------|--------|
| RED (test commit, target failing on assertions) | ✓ `e6b3cb3` (6 failing: validation/off-path/probe-once/one-warning/probe-order/main-hook) | ✓ `7e99639` (4 failing: live triple, disable note, per-run notes, wrap-seam) | ✓ `9998a2c` (7 failing: background live+queued, notes ×2, PTY live+restart+unavailable+escape) |
| GREEN (feat commit, tests pass) | ✓ `7f94c91` | ✓ `bd9ac5c` | ✓ `3f8ea2d` |

All three RED records validated `RED_EVIDENCE_OK` by `gsd-tools check tdd-red-evidence` (Go `-v` output projected faithfully into Node-TAP — the checker's parser — by a mechanical converter; records committed as task{1,2,3}-red-evidence.json). Unexpected-green guard rows are enumerated in each record (off-arm semantics trivially hold pre-wrap).

## Files Created/Modified

- `cmd/ass-guard/acp_serve.go` — the `--sandbox` flag, startup validation, Options threading
- `cmd/ass-guard/main.go` — the unconditional child hook as main()'s first statement
- `internal/acpserve/options.go` — `Options.SandboxMode`
- `internal/acpserve/acp_serve.go` — `ValidateSandboxMode`, `sandboxProbe` seam, `sandboxPolicyFor` (D-04 triple), `resolveSandboxAvailability`, the pipeline step + `SetSandboxHandle` before StartScheduler
- `internal/coreexec/bash.go` — `confineForeground`, shared `sandboxHandleEnabled`/`noteUnconfinedRun`/`unconfinedRuns`, the wrap seam, the persistent disable fall-through, background escape plumbing
- `internal/coreexec/register.go` — `Config.Sandbox` + `Config.SandboxNote`
- `internal/coreexec/background.go` — `TaskRegistry.Sandbox/SandboxNote`, `StartOpts`/`StartWithOpts`, `confineLaunch` inside the one launch funnel
- `internal/coreexec/ptty.go` — `PTYOpts.Sandbox`, the ensureShell wrap (both entries), Run's per-run unavailable note via the NoteFn family
- `internal/runtime/runtime.go` — `sandboxHandle` + `SetSandboxHandle`; sessionFor threads Handle+sink into the three consumers
- `internal/sandbox/landlock_linux.go`, `child_linux.go`, `landlock_linux_test.go` — the deviation fixes (below)
- `cmd/ass-guard/background_wiring_test.go` — stale 2-value `reg.Start` calls updated to 22-02's 3-value signature (the plan's vet gate required it)
- Tests: `serve_test.go` (TestSandboxFlag_* ×7), `bash_test.go` (TestBashSandbox_* ×7 + the old no-op test replaced), `background_test.go` (TestBackgroundSandbox_* ×5), `ptty_test.go` (TestPTYSandbox_* ×5), `sandboxchild_test.go` (the linux loader TestMain)

## Decisions Made

See key-decisions. The two load-bearing semantic decisions: the whole-os.TempDir() rw grant forced the /var/tmp differential (a t.TempDir() sibling is inside the rw set — discovered live when the RED-then-GREEN battery kept passing), and the persistent OQ2 fall-through (a per-call escape can never weaken the shared shell's confinement).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] 22-05's linux landlock leg could not confine anything: missing-path ruleset failure**
- **Found during:** pre-Task-1 platform probing (the live batteries' feasibility check)
- **Issue:** `ApplyChildRuleset` opens every path at restrict time — `DefaultPolicy`'s darwin-only `/System`+`/Library` rows fail the WHOLE linux ruleset, so every confined run exits 1 (fail-closed). Invisible in 22-05 because the linux leg was compile-gated on the darwin host.
- **Fix:** `rulesetPaths` skips rows whose path is absent on this host (the A8 common-denominator realized per-boot; conservative in both directions); the 22-05 pin test updated to the filtered contract
- **Files modified:** internal/sandbox/landlock_linux.go, landlock_linux_test.go
- **Verification:** live probe (inside ok / outside EPERM / curl exit 7) then the full Task 2/3 batteries
- **Committed in:** abe3173

**2. [Rule 1 - Bug] The re-exec child exec'd a bare argv[0] — syscall.Exec does no PATH lookup**
- **Found during:** the same probe
- **Issue:** the wrapped tail carries the original `sh` (bare name); `syscall.Exec("sh", …)` dies ENOENT on every confined run — same compile-gated invisibility
- **Fix:** `sandboxChildExec` resolves a non-absolute target via `exec.LookPath` before the exec (the already-applied ro ruleset still grants the lookup+exec)
- **Files modified:** internal/sandbox/child_linux.go
- **Verification:** the live probe + batteries
- **Committed in:** abe3173

**3. [Rule 3 - Blocking] 22-05's linux-tagged test battery had never executed: Setenv-in-Parallel panic**
- **Found during:** the pre-task sandbox package run
- **Fix:** de-parallelized `TestLandlock_ChildEntryContract` (Setenv panics in parallel tests)
- **Files modified:** internal/sandbox/landlock_linux_test.go
- **Committed in:** abe3173

**4. [Rule 3 - Blocking] `go vet ./cmd/ass-guard/` (Task 1's acceptance gate) failed on a stale 12-11 test**
- **Issue:** `background_wiring_test.go` still called 22-02's pre-queue `reg.Start` with 2 values — missed when 22-02 changed the signature
- **Fix:** four call sites updated to the 3-value form (discard queued)
- **Files modified:** cmd/ass-guard/background_wiring_test.go
- **Verification:** go vet clean; TestBackgroundWiring green
- **Committed in:** 7f94c91

**5. [Rule 2 - Missing Critical] Battery engineering: /var/tmp differential + relative counter**
- **Issue:** the flag-path policy grants the whole os.TempDir(), so t.TempDir() "outside" dirs are INSIDE the rw set (the escape hatch/differential would never fire); the absolute `#2` counter assertion was order-dependent (process-wide counter)
- **Fix:** `dirOutsidePolicy` (/var/tmp — FHS-separated from /tmp) + the increment-based counter pin
- **Files modified:** internal/coreexec/bash_test.go
- **Committed in:** bd9ac5c

---

**Total deviations:** 5 auto-fixed (2 Rule 1/3 blocking bugs in 22-05's compile-gated linux leg surfaced by running live, 1 Rule 3 test defect, 1 Rule 3 stale-test vet gate, 1 Rule 2 battery correctness). **Impact on plan:** no scope creep — the linux leg is now both correct and LIVE-proven (the plan's precondition expected the linux arms to wait for the verify phase; this host ran them now). The darwin arms remain compile-gated per the plan's own platform notes.

## Issues Encountered

- `go test -race ./internal/acpserve/` fails on **TestPermissionsE2E** — the DOCUMENTED pre-existing cross-workstream regression (STATE.md blockers: Phase 23 commit 40b2bbc, operator-bisected with zero Phase-22 acpserve involvement). Skipped via `-skip` for the phase-quick verify; appended to WINDOWS.md as unrun-verify.
- `go test -race ./internal/runtime/` fails on **TestRescanConcurrency** — the documented pre-existing race (22-04's deferred-items entry + WINDOWS). Skipped via `-skip`; the test passes without `-race`.
- Nil-map panics during Task 3 RED: batteries built `&TaskRegistry{…}` literals (no tasks map) — fixed to `NewTaskRegistry()`; the panics also orphaned `sleep` processes that polluted one batch run (cleaned; Stop's battery passes in isolation and in the batch afterward).

## Authentication Gates

None — no external services.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- SAND-01 complete: the sandbox is real end-to-end — flag → probe → Handle → three confined exec sites, loud degradation everywhere, OQ2 honored; both declaring plans (22-05, 22-06) now have summaries, so the shared-ID gate unlocks SAND-01
- The darwin seatbelt arms remain compile-gated (build+vet green); live darwin proof and any operator/CI linux-host re-proof land in the verify phase (the plan's flagged assumption disposition)
- Future exec sites: bash.go's wrap comment names the three-site contract — a fourth Bash-class exec site without the wrap is a review-gate violation

## Self-Check: PASSED

All 16 plan-declared files exist on disk (plus the two deviation files); all 8 commits exist in history (abe3173, e6b3cb3, 7f94c91, 7e99639, bd9ac5c, 9998a2c, 3f8ea2d, 5637a05); every acceptance criterion re-run green (the logs above); the phase-quick subset green with the two documented pre-existing skips; both compile gates green; no 22-06 file left untracked (the pre-existing untracked .bg-shell/, .claude/, .gsd/, .planning/research/ trees are not this plan's and were left untouched).
