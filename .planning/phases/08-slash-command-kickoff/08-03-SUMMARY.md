---
phase: 08-slash-command-kickoff
plan: 03
subsystem: openspec-adapter
tags: [openspec, subprocess, adapter, process-group-kill, exit-classification, non-interactive-guards, toolcat]

# Dependency graph
requires:
  - phase: 04 tool-executor (v1.0)
    provides: toolexec RealExecutor dispatch + toolcat registration/boundary floor
  - phase: 08-01
    provides: ecosys flat frontmatter view + seeded.toml config/loader base
provides:
  - openspec:* tools execute the REAL binary through the catalog (no-Execute gap closed — "no implementation yet" path dead)
  - probe-pinned v1.5.0 command surface in seeded.toml (phantom apply/implement gone; human-only commands never registered)
  - RunGuarded: per-command timeout, OPEN_SPEC_INTERACTIVE=0 + nil stdin, exit-code classification (ok/fixable/hard-error/timeout/not-found) — structure over error (D-10)
  - process-group kill on timeout/cancel (whole child tree reaped, no pipe-holding orphans)
  - gated three-path REAL-binary suite (happy / fixable / missing-binary) — the phase-gate tracer
affects: [08-04 (expand engine invokes openspec tools), 08-06 (E2E gate re-runs the three-path suite), openspec version upgrades (drift gate = re-probe procedure)]

# Tech tracking
tech-stack:
  added: []  # stdlib only (os/exec ctx kill + syscall Setpgid/group SIGKILL)
  patterns:
    - "killGroupOnCtx: Setpgid + cmd.Cancel group SIGKILL — a lone child kill leaves wrapper-shell grandchildren holding output pipes, stalling Wait for the child's full runtime"
    - "RunGuarded returns GuardedResult for ALL outcome classes — never a Go error for command-level failures (D-10)"
    - "gated real-binary tests: ASSGUARD_OPENSPEC_BIN=1 + LookPath skip — CI never depends on the binary, the phase gate re-runs them on the operator machine"

key-files:
  created:
    - internal/openspec/execute_test.go
    - internal/openspec/testdata/openspec-stub.sh
  modified:
    - internal/openspec/seeded.toml
    - internal/openspec/config.go
    - internal/openspec/config_test.go
    - internal/openspec/adapter.go
    - internal/openspec/adapter_test.go
    - internal/openspec/register.go
    - internal/openspec/register_test.go

key-decisions:
  - "RunGuarded implementation landed with T2's GREEN (T2's action text requires running under the entry's timeout); T3's RED→GREEN pair covers the genuinely failing slice — the process-group kill (RED: 1s-budget hang took 10s+)"
  - "Fixable-failure path pinned to the LIVE v1.5.0 behavior: archive --yes succeeds despite incomplete tasks (warn + archive), so the reproducible fixable failure is the no---yes prompt-EOF (exit 1, 'force closed the prompt' stderr) — the probe table's '--yes required non-TTY'; plan example text did not match the binary"
  - "Group kill applied to BOTH run paths (Run + runGuardedProcess): the pre-existing ctx-cancel test was passing only because its 50ms cancel usually lands before the shell forks the grandchild — a latent race, now structural"
  - "Adapter classification map: exit 0 → ok; non-zero + entry ExitClass fixable → fixable; other non-zero → hard-error; deadline → timeout; ErrOpenSpecNotFound → not-found (ESRCH on group kill treated as success)"

patterns-established:
  - "per-entry CommandShape{Argv, TimeoutSecs, ExitClass} drives the closure: argv prefix (new change), timeout budget (default 60; init/update/context/store-setup 120), classification hint"
  - "three-path gate bootstraps ONE scratch project (init --tools claude --force + new change) and chdirs INTO it before any mutation — never touches the repo"

requirements-completed: [CMD-03]

# Metrics
duration: 55min
completed: 2026-08-14
---

# Phase 8 Plan 03: OpenSpec adapter reconciliation Summary

**Every registered openspec:* tool now executes the real binary behind non-interactive guards (per-command timeout, interactive-killswitch env, nil stdin, exit-code classification) with a probe-pinned v1.5.0 surface — failures reach the model as structured results, and timeout/cancel reaps the whole child process group.**

## Performance

- **Duration:** ~55 min (interrupted by a transient network drop mid-T3; resumed from disk — in-flight test edit inspected, completed, committed)
- **Tasks:** 3
- **Files modified:** 9

## Accomplishments
- seeded.toml `[commands]` rebuilt from the installed v1.5.0 probe: read-only list/show/validate/status/instructions/context/doctor/templates/schemas/spec/change; mutating archive/new-change/init/update/config-*/schema-*/store-*/workset-create|remove; phantom `apply`/`implement` deleted; human-only view/workset open/config edit/feedback never registered; gated `TestSurfaceMatchesInstalledBinary` re-probes the binary and diffs stale entries (the upgrade re-probe procedure, Pitfall 7.2)
- `RegisterTools` sets Adapter-backed `Execute` closures on every entry: parse `{"args":[...]}`, RunGuarded under the entry's timeout/exit_class/argv prefix, marshal `{stdout, stderr, exit_code, classification}` — nil Go error for ALL classes; minimal args-array InputSchema on every tool
- `RunGuarded` guards: `context.WithTimeout` per command (distinct from turn ctx), `OPEN_SPEC_INTERACTIVE=0` prepended to child env, nil stdin (prompt reads EOF), classification per entry hint; diagnostics via stderr slog only
- `killGroupOnCtx` on both run paths: Setpgid + group SIGKILL via `cmd.Cancel` — timeout/cancel no longer strands wrapper-shell grandchildren holding output pipes (the RED test hung 10s+ under a 1s budget; now ~1.2s with a pgrep no-orphan assertion)
- Gated three-path REAL-binary suite green on this machine (openspec 1.5.0): happy (`list`/`status --json` valid JSON on a scratch project), fixable (archive prompt-EOF → fixable + actionable stderr, nil error), missing binary (PATH stripped → not-found)

## Task Commits

1. **Task 1: probe-pinned surface + drift gate** — `243904a` (feat; offline config tests included in-commit)
2. **Task 2: Execute closures (RED→GREEN)** — `b4e3121` (test: stub-binary tests 1-5, RED on no-Execute) → `4d17047` (feat: closures + RunGuarded + classification)
3. **Task 3: guards + three-path gate (RED→GREEN)** — `5af0df9` (test: tests 6-11, timeout test RED — 10s hang under 1s budget) → `66787c4` (feat: process-group kill + scratch-chdir bootstrap fix + lint extraction)

**Plan metadata:** (this commit)

## Files Created/Modified
- `internal/openspec/seeded.toml` - probe-pinned v1.5.0 [commands] table (Argv/TimeoutSecs/ExitClass per entry)
- `internal/openspec/config.go` - CommandShape extensions + loader defaults
- `internal/openspec/adapter.go` - RunGuarded + killGroupOnCtx + classification
- `internal/openspec/register.go` - Adapter-backed Execute closures, InputSchema
- `internal/openspec/execute_test.go` - tests 1-5 over the stub binary
- `internal/openspec/adapter_test.go` - tests 6-11 + drift gate + bootstrap
- `internal/openspec/testdata/openspec-stub.sh` - env-controlled stub (exit/stderr/env/stdin/sleep modes)
- `internal/openspec/config_test.go`, `register_test.go` - extended for the new fields

## Decisions Made
- See key-decisions: bundled RunGuarded landing, live-binary fixable-path pin, both-paths group kill, ESRCH-as-success
- Timeout defaults: 60s standard; 120s for init/update/context/store-setup (probe showed long runs)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixable-failure path example didn't match the installed binary**
- **Found during:** Task 3 (three-path gate run)
- **Issue:** plan expected `archive` on incomplete tasks to exit fixable with "incomplete task" stderr; live v1.5.0 probe: `--yes` WARNS and archives (exit 0) — no failure. The no---yes path prompts → EOF → exit 1 "User force closed the prompt"
- **Fix:** fixable subtest pinned to the no---yes prompt-EOF path (exit 1 + actionable stderr + fixable classification + nil Go error — D-10 exactly; the model's adaptation is retry with --yes). Deviation documented in the test comment
- **Verification:** `TestRunGuarded_ThreePathRealBinaryGate/fixable_failure_path` green under the gate
- **Committed in:** `66787c4`

**2. [Rule 2] Scratch bootstrap mutated the repo before chdir**
- **Found during:** Task 3 resume (interrupted in-flight edit)
- **Issue:** bootstrapScratchProject ran init/new before t.Chdir(scratch) — `new change` scaffolded `internal/openspec/openspec/` in the package dir
- **Fix:** chdir moved to the top of the bootstrap; pollution removed; all gated runs verified clean since
- **Committed in:** `66787c4`

**3. [Rule 2] TDD sequencing across the interruption**
- **Issue:** T3's implementation (RunGuarded) had already landed in T2's GREEN before the crash; the resumed RED commit therefore pins the genuinely-failing slice (process-group kill) rather than the whole task
- **Fix:** `5af0df9` (RED: timeout test 10s hang) → `66787c4` (GREEN) — the RED→GREEN gate evidence is real and minimal

---

**Total deviations:** 3 auto-fixed (1 blocking, 2 sequencing)
**Impact on plan:** No scope creep; surface semantics pinned to live-binary ground truth rather than plan prose.

## Issues Encountered
- None beyond the deviations above. Full-package `-race` green offline; real-binary gate green; `go vet ./...` + `golangci-lint run internal/openspec/...` clean (0 issues); `go build ./...` green.

## TDD Gate Compliance
RED commits: `b4e3121` (T2, no-Execute), `5af0df9` (T3, group-kill hang). GREEN commits: `4d17047`, `66787c4`. Full package `-race` green after each GREEN.

## Next Phase Readiness
- 08-04's expand engine can invoke openspec:* tools expecting structured `{stdout,stderr,exit_code,classification}` results
- 08-06's E2E gate re-runs `ASSGUARD_OPENSPEC_BIN=1 go test ./internal/openspec/ -run 'Real|ThreePath|Gated'` as its evidence class
- Drift gate documents the re-probe procedure for future binary upgrades

## Self-Check: PASSED

- FOUND: internal/openspec/adapter.go (RunGuarded + killGroupOnCtx + OPEN_SPEC_INTERACTIVE + WithTimeout)
- FOUND: internal/openspec/register.go (Execute closures + classification mapping)
- FOUND: internal/openspec/seeded.toml (probe table; `grep "commands.apply\|commands.implement"` empty)
- Commits 243904a/b4e3121/4d17047/5af0df9/66787c4 present on master

---
*Phase: 08-slash-command-kickoff*
*Completed: 2026-08-14*
