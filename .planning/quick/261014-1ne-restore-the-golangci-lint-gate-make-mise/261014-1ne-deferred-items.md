# 261014-1ne Deferred Items (intentional non-fixes and observations)

Recorded 2026-10-14 during the lint-gate restoration. Every row is a deliberate
disposition-(c) non-fix or an operator-decision observation — nothing here is an
oversight.

## Operator decisions (not made here)

- **.mise.toml pins `golangci-lint = "2"`** — a floating minor. This is the ROOT
  CAUSE of the 2026-09-03 breakage (mise auto-updated 2.12.x → 2.13.2, renaming
  linters and adding opinionated ones). Pinning an exact version (e.g. `"2.13.2"`)
  is an operator call: it trades auto-updates for gate stability. NOT changed in
  this task.
- **exhaustruct deprecation warning on every run** — the config's exclusion block
  intentionally lists BOTH `exhaustruct` and `exhaustruct_v5` (24-01 baseline-repair
  style: tolerate either name across golangci versions). golangci 2.13.2 warns
  "linter 'exhaustruct' is deprecated … Replaced by exhaustruct_v5" on every run.
  Cosmetic; the gate exits 0. Dropping the old name is safe once the floating pin
  question above is settled.
- **Unknown-linter nolint directives removed**: `err113-best-effort` (a
  never-standard alias; 3 sites) and `testingcontext` (a linter absent from 2.13;
  4 sites) produced a `runner/nolint_filter` warning on every run. The stale names
  were trimmed (kept siblings like `err113`/`modernize`); the full directive
  hygiene contract stays "every directive fires or it goes".

## Documented pre-existing -race failures (untouched, unskipped)

- **D-23-07-1 simulator pair** (`TestZedSimulatorE2E`,
  `TestSimulatorCommandSurface`, internal/acpserve) — reproduced at pre-edit HEAD
  (cdb10a1) in a throwaway worktree during this task: pre-existing machine
  failures, not regressions.
- **D-23-07-3** `TestLiveInstalledPluginsProbe` (internal/ecosys) —
  live-environment probe, documented pre-existing.
- **TestEscalation family** (internal/coreexec) —
  `TestEscalation_TermRespectingChildDiesByTerm` failed ONCE under full-suite load
  during Task 2 verification and passed in isolation AND on package re-run (same
  machine-load flake class as the documented
  `TestEscalation_ReapAllUsesLadder`). background.go edits in this task are
  semantic no-ops (identifier renames, named results, a named constant) — no
  timing surface touched.

## Deliberate nolint dispositions (the reasoning behind terse same-line reasons)

Same-line reasons are kept ≤120 cols (lll applies to comment lines too); the full
rationale lives here.

- **subagent_test.go:244 (intrange skip)** — the loop splits `sentence` at every
  BYTE boundary (`sentence[i : i+1]`) to exercise the partial-rune buffering.
  `for i := range sentence` yields RUNE offsets, changing the fixture's semantics.
  Classic loop kept.
- **bash_test.go dirOutsidePolicy (usetesting skip)** — `os.MkdirTemp("/var/tmp",
  …)` is load-bearing: the helper must produce a directory OUTSIDE the sandbox
  policy's granted roots. `t.TempDir()` lands under TMPDIR, which the policy
  GRANTS — the test would silently stop testing the outside-policy path.
- **TestPTYEmpty / TestPTYDrain (tparallel skip)** — the subtests call
  t.Parallel() internally, but the top levels stay serial deliberately (22-04 PTY
  lifecycle; the repo-wide paralleltest disable exists because parallelizing is a
  behavior change, not a lint fix).
- **runSubagentWithProgress `req` (gocritic hugeParam skip)** — value semantics is
  the RACE-SAFE choice: the background goroutine gets a snapshot; a pointer would
  alias the caller's mutable request across the launch boundary.
- **hostproof_test Request stub (hugeParam skip)** — the `Ask` parameter is
  interface-mandated; the stub deliberately ignores it.
- **ensureShell / launch (funlen skip)** — both are the documented ONE-ordered-
  sequence bodies (22-04 shell bootstrap; 22-06 launch funnel). Splitting them
  scatters the ordering the queue/lifecycle contracts pin. launch is 41 statements
  against a 40 default — the threshold is tight, not the code wrong.
- **Run in acp_serve.go (funlen+maintidx skip)** — the serve COMPOSITION ROOT:
  one linear wiring flow. Splitting it into helpers would scatter the
  construction order the 15-x seams document. Architectural restructuring is
  Rule-4 territory, not a lint fix.
- **StripANSI (gocognit skip)** — hand-rolled byte-exact escape parser (22-04);
  restructuring risks output-byte drift against pinned captures.
- **streamAndEmit (cyclop/gocyclo/gocognit skip)** — the streaming projection
  loop; PAR-01 bus isolation + PAR-05 thinking identity make it a flat event walk
  by design.
- **Test batteries (funlen/cyclop/gocyclo/gocognit/maintidx skips)** —
  TestCheckpointOptions, TestUndoAutoCancel, TestUndoActivePathCrossSessionRefusal,
  TestCombinedSteerScenario, TestSteeringDeliveryEndToEnd, TestPTYSerialize,
  TestGateHookAskFailSafes, runGoldenThinkingCase and the other flagged batteries:
  one labeled block per wire row / refusal class / lifecycle leg is the point;
  splitting assertion-dense batteries churns byte-stable tests for zero behavior
  gain.
- **nestif ×2 (config_surface apply ladder, runtime expansion ladder)** — both are
  guarded side-effect ladders (hook-nil-guard × error-log; copy-on-write +
  boundary/provenance writes). Extraction moves the same tree into a helper;
  guard-clause restructuring risks ordering drift in pinned contracts.

## Mechanism observations (for future gate work)

- **gocritic compares >= the size threshold.** The plan pinned 128 expecting
  exactly-128-byte parity structs to be exempt; empirically 128 flags them. The
  config therefore sets **129** — the smallest value honoring the stated intent
  ("house passes ~80-128-byte wire structs by value deliberately"). 14
  exactly-128-byte sites cleared by the correction; larger copies disposed
  site-by-site.
- **Only ONE complexity-class issue per function reports per run** — when a
  function trips gocognit AND gocyclo AND funlen (etc.), successive cache-clean
  runs surface them one at a time as the covering nolint list is extended. The
  residual sweep needed five iterations to reach zero. Expect the same cascade
  when a new function crosses multiple thresholds.
- **Preceding-line nolint directives do NOT cover func-level linter reports**
  (gocyclo/gocognit/funlen/maintidx/cyclop attach at the `func` keyword line):
  a standalone `//nolint:…` on the line above is reported UNUSED by nolintlint
  while the finding survives. Same-line (on the func line) is the only reliable
  form for function-level findings.
- **Latent-lint surfacing (25-03 phenomenon) is real and large**: findings appear
  only in files the run "touches". Every edit batch surfaced new stragglers in
  previously-clean classes (gofmt 7→22 during the nolint sweep; canonicalheader
  0→2; thelper 4→7; lll +2). Only a cache-clean run after the LAST edit is
  authoritative.
- **`//nolint:err113` / `//nolint:modernize` directives on disabled linters are
  NOT flagged unused by nolintlint** (empirically) and were left in place — they
  document intent and re-arm automatically if the linters are re-enabled.
- **The `formatters` section (gofmt/goimports) participates in the same gate** —
  gofmt findings count against `mise run lint` exit status exactly like linters.

## Class-census drift vs the plan (accepted, warning-grade)

The plan's post-config estimate was ~230 findings; the measured post-config state
was 471, dominated by 272 nolintlint findings the config change itself surfaced
(stale directives for the disabled/tuned classes) — exactly the cascade the plan's
Task 2 sweep loop anticipated, just larger. Final state: 0 findings on a
cache-clean run; 655 → 0.
