---
phase: 21-context-policy-parity-closures
plan: "01"
subsystem: policy
tags: [hooks, settings-json, pretooluse, verdicts, deny-wins, claude-code-parity, go]

requires:
  - phase: 12-built-in-tools-infra
    provides: the ecosys.HookRunner (stdin-JSON contract, matcher regex, per-hook timeout, exit-2 refusal, sanitized env) this plan extends in place
provides:
  - HookScope-tagged settings.json hook loading (project .claude/settings.json + user ~/.claude/settings.json) with loud-soft degradation
  - CC's two-path matcher dialect for settings hooks (exact/alternatives vs unanchored regex) with plugin hooks kept legacy
  - parseHookVerdict — both verdict channels (hookSpecificOutput JSON + legacy exit-2) with the exit-2-overrides-JSON rule
  - ResolveVerdict — the pure deny-wins resolver over D-03 scope order (project → user → plugin), project allows demoted loud
  - PreToolUseVerdict — the structured (Verdict, reason) seam 21-06's gate head consumes; scope partition at NewHookRunner construction
affects: [21-06 (the gate-join tracer), Phase 17 gateCall, PAR-03 verification]

actuals:
  tokens: 14105   # chars/4 over the realized internal/ecosys diff (estimate was 48000)
  tasks: 3
  commits: 7

tech-stack:
  added: []   # stdlib only — encoding/json, os/exec, regexp, slices
  patterns:
    - "Pure resolver separated from the Fire loop (RESEARCH Pattern 2) — the 21-06 gate join is a one-line delegation"
    - "Scope partition at runner construction, never in the loader merge (Pitfall 1)"
    - "Four-valued per-hook outcome: silence/schema-invalid/execution-failure all yield NO-DECISION (Pitfall 4)"

key-files:
  created:
    - internal/ecosys/hookverdict.go
    - internal/ecosys/hookverdict_test.go
    - internal/ecosys/testdata/settings-project.json
    - internal/ecosys/testdata/settings-user.json
  modified:
    - internal/ecosys/hooks.go
    - internal/ecosys/loader.go
    - internal/ecosys/hooks_test.go

key-decisions:
  - "Plugin-scope allow verdicts are demoted alongside project-scope ones (the prohibition's letter: ONLY user scope widens trust) — one loud warning per demoted result"
  - "The exit-2 reason in the composed path comes from classifyHookRun's stderr-first extraction; parseHookVerdict's direct-call fallback is the capped stdout (its only channel)"
  - "Alternatives split on pipes, commas, AND spaces with == comparison (space is in CC's exact-set; tool names never contain spaces)"
  - "hookExecResult carries the raw runErr so parseHookVerdict classifies timeout/ctx-cancel kills as execution failure → fail-open"

patterns-established:
  - "Pattern: enum with zero-value compatibility — HookScope's zero value is scopePlugin so every pre-existing constructor is unchanged; firing rank comes from scopeRank, not enum order"
  - "Pattern: shared parser parameterized by scope — parseHooksFile keeps the plugin parse byte-identical while settings scopes reuse the exact shape"

requirements-completed: [PAR-03]

coverage:
  - id: D1
    description: "Settings-scope hook loading: project + user settings.json parse into the scope-tagged registry with CC's matcher dialect; malformed/oversized degrade loudly-but-softly; plugin hooks keep legacy matching"
    requirement: PAR-03
    verification:
      - kind: unit
        ref: internal/ecosys/hooks_test.go#TestSettingsHooksProjectScope
        status: pass
      - kind: unit
        ref: internal/ecosys/hooks_test.go#TestSettingsHooksUserScope
        status: pass
      - kind: unit
        ref: internal/ecosys/hooks_test.go#TestSettingsHooksMalformed
        status: pass
      - kind: unit
        ref: internal/ecosys/hooks_test.go#TestSettingsHooksOversized
        status: pass
      - kind: unit
        ref: internal/ecosys/hooks_test.go#TestSettingsHooksMissing
        status: pass
      - kind: unit
        ref: internal/ecosys/hooks_test.go#TestHookMatcherDialect
        status: pass
    human_judgment: false
  - id: D2
    description: "Verdict channels + pure deny-wins resolver: hookSpecificOutput allow/deny/ask with reasons, exit-2 overriding JSON allow, NO-DECISION for silence/schema-invalid/execution failure; deny-wins with first-deny reason, project allow demoted loud, ask survives"
    requirement: PAR-03
    verification:
      - kind: unit
        ref: internal/ecosys/hookverdict_test.go#TestHookVerdictParse
        status: pass
      - kind: unit
        ref: internal/ecosys/hookverdict_test.go#TestHookVerdictResolve
        status: pass
    human_judgment: false
  - id: D3
    description: "Runner integration: D-03 scope partition at construction, PreToolUseVerdict sequential seam with fail-open timeout/cancel, executor boolean contract preserved via delegation (coreexec untouched and green)"
    requirement: PAR-03
    verification:
      - kind: integration
        ref: internal/ecosys/hooks_test.go#TestHookScopeOrder
        status: pass
      - kind: integration
        ref: internal/ecosys/hooks_test.go#TestPreToolUseVerdict
        status: pass
      - kind: integration
        ref: internal/ecosys/hooks_test.go#TestPreToolUseDelegation
        status: pass
      - kind: integration
        ref: command:go test ./internal/coreexec/ -count=1 (zero edits, TestRegisterCoreHook* green)
        status: pass
    human_judgment: false

duration: 46min
completed: 2026-09-03
status: complete
---

# Phase 21 Plan 01: Settings-Scope Hooks + Verdict Substrate Summary

**PAR-03's hook substrate inside internal/ecosys: both settings.json scopes load scope-tagged with CC's two-path matcher dialect, both verdict channels (hookSpecificOutput JSON + legacy exit-2) parse with exit-2 precedence, and the pure deny-wins ResolveVerdict resolves over D-03 scope order — 17-independent, ready for 21-06's one-line gate delegation.**

## Performance

- **Duration:** 46 min
- **Started:** 2026-09-03T17:27:06Z
- **Completed:** 2026-09-03T18:13:05Z
- **Tasks:** 3 (each TDD: RED + GREEN; plus one lint-cleanup refactor commit)
- **Files modified:** 7 (3 source, 2 test, 2 fixtures)

## Accomplishments

- Settings hooks load from BOTH scopes (project `.claude/settings.json`, user `~/.claude/settings.json`) with the same hooks→event→matcher-group shape the plugin parser accepts; Scope tagged at parse time; malformed/oversized files degrade to a stderr warning + skip, never an error through Load (Pitfall 8 pinned: a repo cannot brick session construction)
- CC's two-path matcher dialect for settings hooks (exact / pipe+comma+space alternatives vs unanchored regex) while plugin hooks keep the legacy compile-everything regex byte-for-byte — the dialect split is table-pinned (settings "Edit" does not match "NotebookEdit"; plugin "Edit" still does)
- parseHookVerdict: the hookSpecificOutput channel (first-non-whitespace-`{` gate, permissionDecision allow/deny/ask + reason, schema-invalid → no-decision) with the exit-2 check FIRST — exit 2 blocks even over a valid JSON allow (T-21-02 pinned at both the unit and runner level); every execution failure (timeout, cancel, spawn error, non-2 exit) fails open to NO-DECISION (PAR-03 letter)
- ResolveVerdict: pure deny-wins fold over D-03 order — ANY deny wins carrying the FIRST denying hook's reason, user-scope allow applies only absent a deny, project (and plugin) allows demoted with one loud ignored-allow warning each (D-01: repo-shipped files never widen trust), ask survives unless denied (D-04)
- PreToolUseVerdict: matching hooks run SEQUENTIALLY in partitioned order under the existing runOne bounds, verdicts assembled via classifyHookRun + parseHookVerdict (exit-2 reason upgraded to the stderr-first extraction), then delegation to ResolveVerdict — the seam 21-06's gate head consumes; PreToolUse keeps the executor's boolean contract by thin delegation with zero coreexec edits

## Task Commits

Each task was committed atomically (TDD: RED before GREEN):

1. **Task 1: Settings-scope hook loading** — `ab36672` (test/RED) + `7528ed8` (feat/GREEN)
2. **Task 2: Verdict channels + ResolveVerdict** — `db6d312` (test/RED) + `b4ecc8f` (feat/GREEN)
3. **Task 3: Scope partition + PreToolUseVerdict seam** — `8b5a162` (test/RED) + `e5720b2` (feat/GREEN)
4. **Lint cleanup (no behavior change)** — `90a5165` (refactor)

**Plan metadata:** this SUMMARY commit (docs).

## TDD Gate Compliance

All three tdd="true" tasks produced a `test(21-01)` commit whose tests failed (compile-level: the API under test did not exist) BEFORE the corresponding `feat(21-01)` commit — gate sequence satisfied for every task (verified via `git log --oneline --grep`).

## Files Created/Modified

- `internal/ecosys/hookverdict.go` (NEW) — Verdict enum, ScopedResult, parseHookVerdict, ResolveVerdict; doc comment cites D-01..D-04, the Phase-17 gate consumer (21-06), and the A2 60s-vs-600s timeout divergence
- `internal/ecosys/hooks.go` — HookScope enum + HookConfig.Scope; parseHooksFile shared parser (plugin parse byte-identical); matchSettingsHook two-path dialect; NewHookRunner stable scope partition (scopeRank) on a copy; PreToolUseVerdict; PreToolUse as thin delegation; hookExecResult.runErr plumbing
- `internal/ecosys/loader.go` — settingsJSONName, loadSettingsHooks (project from claudeDir, user from $HOME), wired into loadAll after plugin hooks (merge order untouched, Pitfall 1)
- `internal/ecosys/hooks_test.go` — settings/dialect/partition/verdict-integration batteries; the 12-02 stdin-payload observation moved to Fire
- `internal/ecosys/hookverdict_test.go` (NEW) — parse + resolve batteries named after the D-facts
- `internal/ecosys/testdata/settings-project.json`, `settings-user.json` (NEW) — the RESEARCH Wave-0 fixtures

## Decisions Made

- Plugin-scope allow verdicts demote alongside project-scope ones — the prohibition's letter ("the ONLY trust-widening verdict is an explicit allow from user scope") leaves no room for plugin allows; one loud warning per demoted result either way
- The exit-2 direct-call reason in parseHookVerdict is the capped stdout (the signature's only channel); the composed PreToolUseVerdict path upgrades it to classifyHookRun's stderr-first message — both D-02 clauses hold where each applies
- Alternatives split on pipes, commas, and spaces (space is inside CC's exact-set; "Edit, Write" matches both tools); `Bash(git *)`-style matchers take the regex path where Go reads parens as a capture group — pinned by the dialect table
- The runtime.go NewHookRunner call site (line drifted to ~1284) needed zero changes — the partition is construction-internal

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Test adaptation] 12-02 stdin-payload test observation moved to Fire**
- **Found during:** Task 3 (GREEN)
- **Issue:** The planned thin delegation makes PreToolUse return (true, "") on proceed — it can no longer echo hook stdout, which TestPreToolUseStdinAndExitRouting asserted to prove the stdin payload contract
- **Fix:** The payload observation now rides `r.Fire(ctx, PreToolUse, fields)` (identical payload construction); the exit-2 refusal row keeps using PreToolUse
- **Files modified:** internal/ecosys/hooks_test.go
- **Verification:** `go test -race ./internal/ecosys/ -count=1` green; coreexec wiring green untouched
- **Committed in:** e5720b2 (Task 3 GREEN)

**2. [Rule 1 - RED-test corrections] three rows fixed against reality during GREEN**
- **Found during:** Tasks 1–2 (GREEN)
- **Issue:** (a) the parens dialect row assumed `Bash(git *)` matches the literal tool "Bash(git )" — Go regex treats the parens as a capture group; (b) the spawn-error row used a two-value form of exec.Command().Run(); (c) the exit-2 row's expected reason was unpinned
- **Fix:** (a) the row now proves the regex path with tool "Bashgit" (an exact == can never fire there) plus a literal-miss counter-row; (b) single-value form; (c) the stdout-fallback reason pinned explicitly
- **Files modified:** internal/ecosys/hooks_test.go, internal/ecosys/hookverdict_test.go
- **Verification:** dialect + verdict batteries green under -race
- **Committed in:** 7528ed8, b4ecc8f

**3. [Rule 3 - Lint hygiene] new lint categories introduced by the new code cleaned up**
- **Found during:** plan-level verification (mise lint)
- **Issue:** exhaustive (default-only HookScope switches), mnd (magic rank 2), modernize (hand-rolled contains loop), noinlineerr, wsl_v5, tagliatelle (CC's camelCase wire fields), lll, and a nonamedreturns nolint misplaced one line off its finding
- **Fix:** enumerate all enum cases, scopeRankPlugin const, slices.Contains, plain Stat assignment, per-field tagliatelle nolints (existing precedent), correctly-placed nolints — no behavior change
- **Files modified:** internal/ecosys/hooks.go, internal/ecosys/hookverdict.go, internal/ecosys/loader.go
- **Verification:** whole-package -race green; the only remaining findings in touched files are the two pre-existing resolveInstalledPlugin drift findings (WINDOWS 16-18, untouched code)
- **Committed in:** 90a5165

---

**Total deviations:** 3 auto-fixed (1 test adaptation, 1 RED-test correction, 1 lint hygiene)
**Impact on plan:** All fixes were consequences of the planned design decisions (thin delegation, Go regex semantics, lint config), not scope changes. No behavioral drift from the plan's must_haves.

## Issues Encountered

- The first full-suite run (`go test -race ./...`, default 600s timeout) failed in `internal/acp` at 602.9s under machine load — a timeout flake, not an assertion failure: acp passes standalone in 3.8s, does not import internal/ecosys (`go list -deps` verified), and the full tree passes with `-timeout 20m` (exit 0). No code change warranted.

## Verification

- `go test -race ./internal/ecosys/ -run 'TestSettingsHooks|TestHookMatcherDialect' -count=1` — PASS
- `go test -race ./internal/ecosys/ -run TestHookVerdict -count=1` — PASS
- `go test -race ./internal/ecosys/ -run 'TestHookScopeOrder|TestPreToolUseVerdict' -count=1` — PASS
- `go test -race ./internal/ecosys/ -count=1` (whole package) — PASS
- `go test ./internal/coreexec/ -count=1` — PASS (zero edits; TestRegisterCoreHook* green)
- `go test ./internal/runtime/ -count=1` — PASS (the NewHookRunner consumer)
- `go vet ./...` (mise vet) — clean; `gofmt` — clean; `CGO_ENABLED=0 go build ./...` (mise build) — clean
- `go test -race -count=1 -timeout 20m ./...` — PASS (full tree; see Issues Encountered for the first-run flake)
- `mise run lint` — RED on the pre-existing golangci-lint version drift (WINDOWS 16-18: the config's exhaustruct exclusion targets the v1 linter name, so exhaustruct_v5 flags 2286 pre-existing issues). The 21-01 files are clean of every other lint category; the two remaining findings in loader.go predate this plan (verified at commit 625dd2d).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- PAR-03's substrate is complete and 17-independent: every test runs green without internal/perm or internal/session/gate.go
- 21-06's gate join consumes `(*HookRunner).PreToolUseVerdict` → maps deny→gateDeny (first denying hook's reason), ask→gateSuspend even ungated (D-04), user-scope allow→past the hook head into rule evaluation, and then disposes the executor PreToolUse leg (Pitfall 2 single-consultation grep audit)
- The Verdict constants are deliberately unexported per the plan's artifact spec; 21-06 will export-by-necessity (D-19 precedent) when the gate needs to name them
- PAR-03 stays shared-open until 21-06 summarizes (shared-ID gate #2388)

## Self-Check: PASSED

- All 7 created/modified files exist on disk (verified via git diff --stat 625dd2d..HEAD, 7 files changed)
- All 7 commits present in git log (ab36672, 7528ed8, db6d312, b4ecc8f, 8b5a162, e5720b2, 90a5165)
- All task acceptance criteria re-verified: dialect rows pinned both directions, malformed → nil Load error + warning, `grep -c 'Scope' internal/ecosys/hooks.go` = 13 (≥3), vet clean, coreexec green untouched, whole-package -race green

---
*Phase: 21-context-policy-parity-closures*
*Completed: 2026-09-03*
