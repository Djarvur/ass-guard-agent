---
phase: 18-session-family
plan: 06
subsystem: acp-sessions
tags: [cli, resume, continue, picker, d10, d11, acp-06, cobra, pipe-safe, load-engine]

# Dependency graph
requires:
  - "18-01 load/replay spine — Server.LoadSession, ReplayTranscript, the D-03 loading/ready gate"
  - "18-03 list engine — session.ListSessions (tombstone-filtered, recency-ordered headers, SessionHeader.Title)"
  - "18-05 full resume — Runner.ResumeSession (reconcile + closures + SeedResume), commands re-advertisement"
  - "15-05/15-06 — the de-cobra'd cmd shell conventions and the Run composition statement order"
provides:
  - "The CC-parity CLI trio (D-10): root-persistent --resume (NoOptDefVal picker sentinel; bare/= /space forms), --continue/-c, resolvable at BOTH entrypoints (root RunE delegation + the serve child's inherited cmd.Flags())"
  - "Shared resolveResumeTarget: pattern-valid id pass-through (T-18-13 pre-access validation), case-insensitive title-prefix names with ambiguity/miss errors naming the directory, --continue = newest non-tombstoned cwd row, bare --resume = picker mode"
  - "cmd/ass-guard/picker.go — SelectSession (io.Reader/io.Writer, numbered rows `N) <title-60-runes>  (<rel>)`, one re-prompt, typed error on second invalid/EOF) + FormatRelativeTime buckets (just now / Nm / Nh / Nd / Nw, rounded)"
  - "acpserve.Options.ResumeTarget + the pre-Serve injection: Run calls srv.LoadSession (the SAME engine session/load uses) before Serve; failure is a loud fatal naming the target"
  - "Regenerated 15-01 CLI-contract goldens covering the new flag surfaces"
affects: [20-built-in-commands, ACP-06, phase-verification]

# Actuals (#2632) — chars/4 over the realized diff (94,570 diff chars on cmd/ + internal/), same scale as the plan's estimate
actuals:
  tokens: 23642
  tasks: 3
  commits: 8

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "pflag NoOptDefVal as the bare-flag/=/space-form discriminator: bare --resume parses to a sentinel, the space form leaves the target as a positional arg the RunE consumes — no custom parser"
    - "Conditional cobra Args validator: the root takes a positional arg ONLY when a resume flag is typed (legacy unknown-subcommand behavior otherwise)"
    - "Function-valued package vars as testable CLI seams (pickResumeSession, runServeWithResumeTarget) — the runACPServeCmd extraction precedent; os.Stdin/os.Stderr enter ONLY at the composition edge, the logic stays io.Reader/io.Writer (D-11 pipe safety)"
    - "Pre-Serve serve injection: Options.ResumeTarget loads through Server.LoadSession after the wiring block and before Serve, so replayed frames reach the client with zero client input — the load-before-serve proof is structural, not timing-based"

key-files:
  created:
    - cmd/ass-guard/picker.go
    - cmd/ass-guard/resume_flags_test.go
    - internal/acpserve/resume_test.go
  modified:
    - cmd/ass-guard/main.go
    - cmd/ass-guard/acp_serve.go
    - cmd/ass-guard/cli_contract_test.go
    - internal/acpserve/options.go
    - internal/acpserve/acp_serve.go

key-decisions:
  - "One resolver, two entrypoints (research OQ3): resolveResumeTarget lives in cmd/ass-guard/acp_serve.go; the root RunE resolves against the cwd and delegates through the runServeWithResumeTarget seam, the serve RunE resolves against the resolved --work-dir — identical semantics, no duplication"
  - "Id-form targets pass resolution WITHOUT an existence check (existence is the load engine's typed unknown-session error — one checker, not two); name-form targets never join a path (matched against ListSessions titles; traversal-shaped or separator-carrying targets reject BEFORE any directory scan, T-18-13)"
  - "The picker's interim Task 1 state (a var defaulting to a typed 'picker unavailable' error) was plan-sequenced: Task 2's SelectSession replaced it within the same plan; no stub ships"
  - "CLI-contract goldens regenerate FROM the binary's actual output (the 15-01 transcription rule): the root-persistent flags shift every help surface's flag alignment (--resume string[=\"@picker\"] widens the usage column), so hand-editing was rejected in favor of deterministic regeneration"

patterns-established:
  - "Pipe-safe terminal interaction: rows on stderr, line-wise number from stdin via bufio with bounded retries (T-18-14) — no termios anywhere, proven by an io.Pipe-driven test"
  - "Fixture-transcript tests in acpserve: hand-written CLEAN transcripts (session_start + complete turn + session_end) so reconciliation appends nothing and the replay assertions are exact"

requirements-completed: [ACP-06]

coverage:
  - id: D1
    description: "The CC trio parses and resolves at both CLI entrypoints with cwd-scoped semantics and clean typed errors (D-10)"
    requirement: ACP-06
    verification:
      - kind: unit
        ref: cmd/ass-guard/resume_flags_test.go#TestResumeFlagParsing
        status: pass
      - kind: unit
        ref: cmd/ass-guard/resume_flags_test.go#TestContinueResolvesMostRecentCwd
        status: pass
      - kind: unit
        ref: cmd/ass-guard/resume_flags_test.go#TestResumeTargetResolution
        status: pass
      - kind: unit
        ref: cmd/ass-guard/resume_flags_test.go#TestRootResumeDelegation
        status: pass
      - kind: unit
        ref: cmd/ass-guard/cli_contract_test.go#TestCLIBinaryContractRootAndACP
        status: pass
    human_judgment: false
  - id: D2
    description: "The D-11 numbered picker: pipe/ssh-safe terminal I/O, one retry, clean EOF failure, 60-rune truncation, bucketed relative time"
    requirement: ACP-06
    verification:
      - kind: unit
        ref: cmd/ass-guard/resume_flags_test.go#TestPickerSelectsByNumber
        status: pass
      - kind: unit
        ref: cmd/ass-guard/resume_flags_test.go#TestPickerRetriesOnceThenFails
        status: pass
      - kind: unit
        ref: cmd/ass-guard/resume_flags_test.go#TestPickerRelativeTime
        status: pass
      - kind: unit
        ref: cmd/ass-guard/resume_flags_test.go#TestPickerPipeSafe
        status: pass
      - kind: unit
        ref: cmd/ass-guard/resume_flags_test.go#TestPickerTruncatesTitles
        status: pass
    human_judgment: false
  - id: D3
    description: "Serve-side target injection through the ONE load engine: pre-Serve LoadSession, replay frames before any client input, follow-up prompt continues the turn sequence, loud failure on a missing target"
    requirement: ACP-06
    verification:
      - kind: integration
        ref: internal/acpserve/resume_test.go#TestResumeTargetLoadsBeforeServe
        status: pass
      - kind: integration
        ref: internal/acpserve/resume_test.go#TestResumeTargetAbsentIsNoop
        status: pass
      - kind: integration
        ref: internal/acpserve/resume_test.go#TestResumeTargetFailureIsLoud
        status: pass
    human_judgment: false
  - id: D4
    description: "The real trio over an actual pipe: picker rows render and select, name-form --resume resolves at the serve entrypoint and replays before the handshake, error surfaces match the contract"
    verification:
      - kind: manual_procedural
        ref: "binary smoke (executed this session): echo 1 | ass-guard --resume rendered `1) fix the login flow  (just now)` + prompt; ass-guard acp serve --resume 'fix the log' replayed the fixture chunk as the FIRST wire frame; -c/--resume <missing>/traversal/--prompt-conflict errors verified"
        status: pass
    human_judgment: true
    rationale: "The plan's manual verification (real TTY + ssh session with past sessions) belongs to phase verification; the pipe-driven smoke covers the D-11 mechanics but not an interactive ssh terminal"

# Metrics
duration: 60min
completed: 2026-09-03
status: complete
---

# Phase 18 Plan 06: CC-Parity CLI Resume Surface Summary

**The full `--resume`/`--resume <id|name>`/`-c` trio on the root command, a pipe-safe numbered picker (D-11), and serve-side injection through the same Server.LoadSession engine session/load uses (D-10).**

## Performance

- **Duration:** ~60 min (15:24Z–16:24Z, 2026-09-03)
- **Tasks:** 3/3 (TDD: 3 RED commits, 3 GREEN commits, 2 lint-polish commits)
- **Files modified:** 8 (3 created, 5 modified)

## Accomplishments

- The trio is typed at the ROOT and inherited everywhere: `ass-guard --resume` opens the picker, `--resume <id|name>` (all three spellings) resumes directly, `-c`/`--continue` resumes the newest non-tombstoned session of the current directory, and any resume flag plus `--prompt` is the typed combination error (ROADMAP criterion 3 — "works anywhere (CLI flag), not only from the editor").
- The picker is exactly D-11: numbered rows on stderr with 60-rune-safe titles and bucketed relative times, one line-wise number read from stdin, one re-prompt, a typed error on the second invalid entry or EOF — io.Reader/io.Writer throughout, verified over io.Pipe.
- `acpserve.Run` consumes `Options.ResumeTarget` BEFORE Serve via `srv.LoadSession` — the identical 18-01/18-05 engine the editor's session/load uses; the test proves load-before-Serve structurally (replayed frames arrive with zero client input) and a follow-up prompt continues the transcript's turn sequence (turn-002 after the fixture's turn-001).
- Real-binary smoke: name-form `--resume` through the serve entrypoint replayed the fixture's assistant chunk as the first wire frame and exited cleanly on EOF; all error surfaces verified against the actual CLI.

## Task Commits

Each task was committed atomically (TDD):

1. **Task 1: Root + serve flag trio with resolution semantics** — `60de68a` (test RED), `d160834` (feat GREEN)
2. **Task 2: The numbered picker (D-11)** — `d41b509` (test RED), `ff6820e` (feat GREEN)
3. **Task 3: Serve-side target injection through the one load engine** — `5dcba43` (test RED), `083d733` (feat GREEN)
4. **Lint polish to the project gate** — `bfac301`, `d465521`

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `cmd/ass-guard/main.go` — root-persistent `--resume` (NoOptDefVal sentinel) + `--continue`/`-c` registration, the conditional positional-arg validator, the RunE resume branch
- `cmd/ass-guard/acp_serve.go` — the shared resolution home: pickSentinel, sessIDPattern copy, typed errors, resumeFlags/readResumeFlags/resolveResumeTarget (+continueMostRecent/pickerChoice/resolveDirectTarget), the pickResumeSession/runServeWithResumeTarget seams, the serve RunE inherited-flag resolution
- `cmd/ass-guard/picker.go` — SelectSession + FormatRelativeTime (D-11)
- `cmd/ass-guard/cli_contract_test.go` — goldens regenerated from binary output for the new flag surfaces
- `internal/acpserve/options.go` — Options.ResumeTarget (resolved pre-serve by cmd; empty = none)
- `internal/acpserve/acp_serve.go` — the pre-Serve LoadSession injection (loud fatal on failure)
- `cmd/ass-guard/resume_flags_test.go`, `internal/acpserve/resume_test.go` — the 12-test battery

## Decisions Made

- pflag's NoOptDefVal is the bare/= /space-form discriminator; the space form's target is consumed from positional args by the RunE (with a conditional root Args validator so legacy `ass-guard <typo>` still errors as an unknown command).
- `--resume` wins over `--continue` when both are typed (documented precedence, no invented conflict error — the plan is silent).
- Name matching considers only rows with TitlePresent (the "(no prompt)" fallback literal is never matchable input).
- Externally-observed behavior note for the smoke: in a bare directory with no seeded profiles, the resume path fails exactly as plain `acp serve` does (the pre-existing DIST-03 zero-config profile resolution) — verified identical, no regression.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocker] Options.ResumeTarget field landed in Task 1, not Task 3**
- **Found during:** Task 1 GREEN
- **Issue:** Task 1's action wires `Options.ResumeTarget` from cmd, but the plan assigned the options.go field to Task 3 — Task 1 cannot compile without it.
- **Fix:** The field (with its full doc comment) was added in Task 1's commit; Task 3 then added only the acpserve.Run consumption. No contract change — both tasks' artifacts are exactly as specified.
- **Files modified:** internal/acpserve/options.go
- **Commit:** d160834

**2. [Rule 1 - Bug] CLI-contract goldens regenerated (all 15 constants)**
- **Found during:** Task 1 GREEN (full cmd suite)
- **Issue:** The root-persistent flags change EVERY --help surface (new rows + a shifted usage column: `--resume string[="@picker"]` widens the longest flag-name in every section).
- **Fix:** Regenerated all goldens deterministically from the built binary's actual output (the file's own transcription rule), with a generator script; `TestCLIBinaryContract*` green.
- **Files modified:** cmd/ass-guard/cli_contract_test.go
- **Commit:** d160834

**3. [Rule 3 - Blocker] Root positional-arg validator + constructor extractions**
- **Found during:** Task 1 GREEN
- **Issue:** cobra rejects positional args at a root with subcommands (`unknown command "<id>"`), breaking the `--resume <id>` space form; the added registration/wiring also pushed newRootCmd/newACPServeCmd past funlen-60.
- **Fix:** Conditional Args validator (positional allowed only when a resume flag is typed), registerResumeFlags + rootArgsValidator + resolveServeResumeTarget extractions.
- **Files modified:** cmd/ass-guard/main.go, cmd/ass-guard/acp_serve.go
- **Commits:** d160834, d465521 (polish)

**4. [Rule 1 - Bug] Two lint-polish commits (test files)**
- **Found during:** post-GREEN lint pass
- **Issue:** RED-committed test files escaped the --new-from-rev lint check; funlen/gocognit/noinlineerr/paralleltest/modernize/goconst issues in the new batteries.
- **Fix:** Table-driving + helper extraction + plain error assignments + strings.Cut + errors.AsType; 18-06 files are clean under the project's rule set (verified via golangci-lint 2.13.2; only the repo-wide exhaustruct_v5 version-drift class remains, shared with 2222 pre-existing instances).
- **Commits:** bfac301, d465521

**Total deviations:** 4 auto-fixed (1 bug-class goldens + polish, 2 blockers, 1 sequencing). **Impact:** none on contracts; all plan artifacts landed as specified.

## OPERATOR SIGN-OFF PENDING (inherited from the plan's <deviation_note>)

The cwd-scoped `--resume <id|name>` deviation is carried VERBATIM from 18-06-PLAN.md for acknowledgment at phase verification — this run was unattended and did not decide it:

> CC's `--resume <session-id>` resolves across ALL projects via a central registry (`~/.claude/projects/`). ass-guard's store is per-directory with no registry, and ROADMAP criterion 3 glosses "works anywhere" as CLI-flag availability ("not only from the editor"), which this plan delivers. Resolution is therefore cwd-scoped with a clear not-found error naming the directory searched. Cross-project unique-match search would need a new project-directory registry — a store design decision outside this phase's sources; it is surfaced here as a documented deviation for the operator, not silently descoped. The Deferred Ideas list is untouched (un-delete surface remains the only deferral).

Escalate to a blocking change ONLY if the operator rules that cross-project resolution is required (that would need the registry store design, a new scope decision).

## Issues Encountered

- **Pre-existing (out of scope, deferred):** `golangci-lint` 2.12.2 (the `.mise.toml` "2" pin at last install) panics with `file requires newer Go version go1.27 (application built with go1.26)` on untouched packages — a dependency in the module cache now declares go 1.27. 2.13.2 runs but its exhaustruct_v5 behavior reports 2222 pre-existing issues repo-wide. 18-06's gate therefore ran as: `mise vet` green, `mise build` green, full `go test -race ./...` green, and golangci-lint 2.13.2 with 18-06 files clean (only the shared version-drift class remains). Logged in `deferred-items.md`; suggested fix is a standalone tooling task (pin bump + exhaustruct config migration).
- `mise ci` as a single task is consequently NOT green solely because of the lint step's pre-existing environment drift; every non-lint gate is green and no 18-06 test fails.

## Verification

- `go test -race -count=1 ./cmd/ass-guard/ ./internal/acpserve/` — PASS (final state).
- `go test -race -count=1 ./...` (mise test's recipe) — PASS, full repo.
- `go vet ./...` and `CGO_ENABLED=0 go build ./...` (mise vet/build) — PASS.
- Plan <verify> per task: Task 1/2/3 automated commands — PASS (run at each GREEN commit).
- Real-binary smoke over pipes — PASS (picker render+select, name-form serve-side replay-before-handshake, all error surfaces; details in coverage D4).

## Self-Check: PASSED

- Created files exist: cmd/ass-guard/picker.go, cmd/ass-guard/resume_flags_test.go, internal/acpserve/resume_test.go — FOUND
- Commits 60de68a, d160834, d41b509, ff6820e, 5dcba43, 083d733, bfac301, d465521 — FOUND in git log
- All 12 plan test functions + contract tests green under -race (final full-suite run)

## Next

Phase 18 plan 6 of 6 — **Phase complete, ready for phase verification** (`/gsd-verify-work 18`): the operator sign-off item above (cwd-scoped `--resume <id|name>`) and the manual ssh/TTY picker check are the two human-judgment surfaces this plan leaves open.
