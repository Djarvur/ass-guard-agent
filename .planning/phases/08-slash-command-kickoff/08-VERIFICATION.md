---
phase: 08-slash-command-kickoff
verified: 2026-08-16T00:55:00Z
status: passed
score: 7/7 must-haves verified
behavior_unverified: 0
overrides_applied: 1
overrides:
  - must_have: "the 11 deferred v1.0 Phase-4 UAT checks pass (CMD-04) — strict 11/11 pass"
    reason: >-
      10 of 11 checks pass outright (mechanism suites re-run green in this
      verification); UAT check 3's live implement-stage hook leg is un-exercised
      BY CONSTRUCTION (the seeded /opsx scenario's actions are continue/wait —
      no hook rows fire) while the hookdag mechanism is fully green. The
      stage-4 model-variance fail in the witness zero-continue run (the real
      model asked the operator a sync-or-archive question; engine chained all 4
      stages, unmatched=>nothing held, no code changed; immediate re-run PASSED
      309.35s with archive on disk — evidence committed as 7677051) is the same
      residual class. Operator witness ACCEPTED the gate with both residuals on
      2026-08-16 (STATE.md: "operator witness ACCEPTED 2026-08-16").
    accepted_by: "operator (Phase-8 gate witness)"
    accepted_at: "2026-08-16T08:30:00Z"
deferred:
  - truth: "Skills listing placed mid-conversation exactly as the capture (currently a trailing system block — documented in-code divergence)"
    addressed_in: "Phase 9"
    evidence: "ROADMAP Phase 9: AUD-05 parity re-capture on a newly pinned session (08-05 key-decisions flag the placement + Skill-result shape for the re-capture)"
  - truth: "Edit/Write read-tracking enforcement forms ('File has not been read yet' / 'modified since read') — corpus_absent in the capture fixture"
    addressed_in: "Phase 9"
    evidence: "08-08 fixture corpus_absent flags routed to the Phase-9 re-capture; ROADMAP Phase 9 AUD-05 re-pins capture-grounded forms"
---

# Phase 8: Slash-Command Kickoff Verification Report

**Phase Goal:** As a developer practicing SDD, I want to kick off and drive an entire OpenSpec change (`/opsx:explore → propose → apply → archive`) from ass-guard by typing the toolkit's own slash-commands, so that the unmodified workflow runs hands-off — the milestone's product proof and the closure of v1.0's major known gap.
**Verified:** 2026-08-16T00:55:00Z
**Status:** passed (7/7 success criteria; 1 operator-accepted residual recorded as override)
**Re-verification:** No — initial verification
**Mode:** mvp (user story validated via `user-story.validate` → true)

**Verification method:** Goal-backward, codebase-first. Every SUMMARY claim below was re-derived from the actual code and tests: file/implementation inspection at every named site, fresh test runs in this session (real-binary gated suites included), witness-log and committed-evidence inspection, and git-history cross-checks. The gated LLM E2E was NOT re-run (fresh operator-witnessed evidence from 2026-08-15 exists on disk and in commit 7677051); its assertion set, harness, and committed outputs were inspected instead.

## Goal Achievement

**Verdict: GOAL_ACHIEVED.**

### User Flow Coverage (MVP mode)

| Step | Expected | Evidence in codebase | Status |
|------|----------|----------------------|--------|
| Developer types `/opsx:explore <subject>` | Command discovered + expanded invisibly; typed command rides as provenance | `internal/ecosys/loader.go:166-222` (one-level `commands/<ns>/<name>.md` scan, colon-join `ns+":"+stem`); `internal/ecosys/expand.go` (zcode semantics); `cmd/ass-guard/acp_serve.go:555-598` `expandUserBlocks` (parse → registry hit → expand → mutating boundary → provenance); `internal/session/manager.go:187` `AppendCommandProvenance` | VERIFIED |
| Model works the stage (reads code, runs openspec, uses skills, searches web) | Real tool execution, not stubs | `internal/openspec/register.go:38-90` (Adapter-backed Execute closures); `internal/coreexec/` (Bash/Read/Write/Edit/Todo real executors); `internal/ecosys/skills.go:106-143` `SkillExecute`; `internal/toolexec/ddg.go` (DDG default backend) | VERIFIED |
| Stage ends → engine chains the next stage, zero manual continues | >= 3 continue decisions chain propose→apply→archive from ONE typed prompt | `internal/openspec/seeded.toml` (`post-archive-terminal` shield, `post-propose-handoff`, `post-apply-handoff` text rows, `post-explore-handoff` command-provenance row); `cmd/ass-guard/acp_serve.go:1027-1058` (StartedBy + scenario-subject forwarding); E2E assertions `cmd/ass-guard/e2e_opsx_test.go:292-360`; witness re-run PASS 309.35s (`/tmp/e2e-witness-evidence/gated-e2e-rerun.log`) | VERIFIED (witnessed; stage-4 variance accepted — see override) |
| Change archived, real artifacts on disk | Archive directory exists in the scratch project | E2E archive assertion (`e2e_opsx_test.go:342-360`); committed stage-4 closing `cmd/ass-guard/testdata/opsx-e2e/stage-4-output.txt` ("Archive Complete", archived-to path); commit `7677051` = the passing evidence | VERIFIED |

### Observable Truths (7 roadmap success criteria)

| # | Truth (SC / REQ) | Status | Evidence |
|---|------------------|--------|----------|
| 1 | `commands/<ns>/<name>.md` layouts found via one-level subdirectory scan with colon-joined keys, existing precedence, real-fixture test from actual `openspec init` output (CMD-01) | VERIFIED | `loader.go:170-222` `discoverCommands`/`discoverNamespacedCommands` (`ns+":"+stem`, one level only); real fixture `internal/ecosys/testdata/opsx-real/.claude/commands/opsx/{explore,propose,apply,archive,sync}.md` committed from live `openspec init --tools claude` v1.5.0; gated `TestOpsxFixtureMatchesRealInit` re-diffs against the live binary; precedence battery (`precedence_test.go`). **My run:** `ASSGUARD_OPENSPEC_BIN=1 go test ./internal/ecosys/ -count=1` GREEN |
| 2 | zcode substitution semantics — `$ARGUMENTS`, `$1..$N` (out-of-range → empty), "User arguments:" heading, `${ARGUMENTS}` and `` !`cmd` `` NOT recognized, unknown `/foo` falls through; table-driven contract test (CMD-02) | VERIFIED | `expand.go:51-97` implements the exact contract ($ARGUMENTS verbatim, $1..$9 with `fieldAt` out-of-range→empty, $0→empty, single-pass, no fence skip, append only when `hasPlaceholder` is false); `${ARGUMENTS}`/backtick stay literal structurally (no matching token). 14-row table `expand_test.go` written RED-first; wiring `expandUserBlocks` on both turn paths + engine adapter; `TestExpansion_UnknownCommandFallsThrough` (`acp_serve_test.go:440`). **My runs:** ecosys + cmd/ass-guard suites GREEN |
| 3 | `openspec:*` tools return real executed subprocess results — Adapter-backed Execute, probe-pinned surface (phantoms removed), read-only/mutating classified, non-interactive guards (CMD-03) | VERIFIED | `register.go` sets `Execute: executeClosure(adapter, shape)` on every entry; `seeded.toml` [commands] = 27 probe-pinned v1.5.0 entries, phantom `apply`/`implement` ABSENT (grep-verified), read-only/mutating mutability on every entry; `adapter.go` `RunGuarded` (per-command `context.WithTimeout`, `OPEN_SPEC_INTERACTIVE=0`, `cmd.Stdin = nil`, `killGroupOnCtx` process-group kill, 5-way exit-code classification). **My run:** `ASSGUARD_OPENSPEC_BIN=1 go test ./internal/openspec/ -count=1` GREEN — includes `TestSurfaceMatchesInstalledBinary` + `TestRunGuarded_ThreePathRealBinaryGate` against the installed binary at /usr/local/bin/openspec |
| 4 | Real `/opsx:explore → propose → apply → archive` E2E, zero manual continues, happy/fixable/missing-binary paths, 11 deferred UAT checks (CMD-04) | VERIFIED (operator-accepted residual — see override) | Harness `e2e_opsx_test.go`: double env gate FAIL-LOUD, real-binary scratch bootstrap, real profile+provider, zero-continue assertions (>= 3 continues, per-stage provenance for propose/apply/archive, archive dir on disk), capture mode (`ASSGUARD_E2E_CAPTURE`), `TestOpsxFixableRecovery_Gated` (failed-attempt → recovery → archive dir). **Witness evidence:** fixable-recovery PASS 95.86s + zero-continue re-run PASS 309.35s (`/tmp/e2e-witness-evidence/`); 4 stage closings committed (7677051, timestamps corroborate the log). 11 UAT checks: 10 pass (mechanism tests re-run green in this verification) + check 3 partial, ACCEPTED by operator 2026-08-16 |
| 5 | Expanded turns record provenance; engine pattern-matching remains assistant-role-only (regression); same-key shadowing warns (CMD-05) | VERIFIED | `TypeCommandProvenance`/`AppendCommandProvenance` (`transcript.go:40`, `manager.go:187`); `StartedBy` sourced exclusively from the prompt-side invocation (`acp_serve.go:1007-1036`). **My runs — all PASS:** `TestEngine_ToolResultContentIgnored`, `TestEngine_CommandProvenanceNotInjectable` (cmd), `TestDecide_NonCommandTurnTriggersNothing`, `TestDecide_UnmatchedIsNothing` (engine); shadow warnings `merge{Skills,Commands}WithShadowWarnings` (`loader.go:541-570`) + `TestShadowWarning*` battery |
| 6 | Skills claude-code-compatible — model-invoked Skill tool, captured-shape listing with dynamic merge, SKILL.md loads, all discovered skills exposed (CMD-06) | VERIFIED | `skills.go`: `SkillListing` (captured header/entry/249-char cut, pinned from rollout `model-io-sess_fb066d52…jsonl`), `ResolveSkill` by registry key only, `SkillExecute` structured results for all outcomes; wiring `acp_serve.go:810` (listing merged into the profile copy) + `:829-830` (Skill tool Execute override at the per-session clone); `AllSkills` = no filtering (D-04). Live leg: 1 Skill call in the passing E2E run's 44 tool calls (08-06 4th addendum). Placement divergence documented in-code, routed to Phase 9 (see deferred) |
| 7 | WebSearch real DDG-HTML default backend (zero key) on the seam; WebFetch fetch + html→markdown; zero-config unaffected (CMD-07) | VERIFIED | `ddg.go` `DefaultBackend` (scrapes `https://html.duckduckgo.com/html/?q=`, parses `result__a`/`result__snippet` via x/net/html, exact-host uddg unwrap); `real.go:28` `defaultWebBackend` fallback + `selectBackend("")`/`("ddg")` → DefaultBackend — zero-config acp_serve construction works with no wiring change; html→markdown via `JohannesKaufmann/html-to-markdown` v1.6.0 (go.mod). **My run:** `go test ./internal/toolexec/ -count=1` GREEN. Note: live DDG egress is bot-walled from this environment — parse behavior is pinned by the committed structure fixture (corroborated across 4 independent scrapers) + the REAL anomaly page as the degradation fixture (documented 08-02); keyed APIs remain config-swappable |

**Score:** 7/7 truths verified (0 present-but-behavior-unverified)

### Behavioral Spot-Checks (run in this verification)

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Real-binary adapter + surface + fixture suites | `ASSGUARD_OPENSPEC_BIN=1 go test ./internal/openspec/ ./internal/ecosys/ -count=1` | ok / ok (20.5s, 4.6s) | PASS |
| Injection-guard regressions (CMD-05) | `go test ./cmd/ass-guard/ ./internal/engine/ -run 'TestEngine_ToolResultContentIgnored\|TestEngine_CommandProvenanceNotInjectable\|TestDecide_NonCommandTurnTriggersNothing\|TestDecide_UnmatchedIsNothing'` | all PASS | PASS |
| UAT mechanism legs (checks 4/8/9/10/11) | `go test ./internal/engine/ ./internal/learning/ ./internal/toolexec/ -run 'TestDispatch_Ask*\|TestObserve_CancelDrain\|TestObserve_ReFireBudget\|TestDispatchBatch_*\|TestStore_Revert*'` | all PASS | PASS |
| Mechanism suites (session/engine/cmd/coreexec/shaper) | `go test ./internal/session/ ./internal/engine/ ./cmd/ass-guard/ ./internal/coreexec/ ./internal/shaper/ -count=1` | all ok | PASS |
| Web backends | `go test ./internal/toolexec/ -count=1` | ok | PASS |
| Build + vet | `go build ./...` + `go vet` on ecosys/openspec/cmd | clean | PASS |
| Gated LLM E2E | NOT re-run per contract (fresh 2026-08-15 witness evidence) | PASS 309.35s (witness log) + committed evidence 7677051 | PASS (witnessed) |

### Key Link Verification

| From | To | Via | Status |
|------|----|----|--------|
| `sessionTurnRunner.Run` (both paths) + `engineTurnRunnerAdapter.Run` | `ecosys.ParseInvocation` → `Command.Expand` | `expandUserBlocks` (acp_serve.go:555, :696, :1054) | WIRED |
| `expandUserBlocks` | `session.Manager.AppendCommandProvenance` / `AppendBoundary` | mutating-command cause `mutating-command:<key>` (D-11) | WIRED |
| `engineTurnRunnerAdapter` | `engine.TurnOutput.StartedBy` → `command_patterns` row | `invocationFor` from RAW prompt only; `PopulateContinue` learns `command:` prefix | WIRED |
| `openspec.RegisterTools` | `Adapter.RunGuarded` subprocess | per-entry Execute closure (register.go:60-81) | WIRED |
| `sessionFor` | `ecosys.SkillListing` merge + `SkillExecute` override | acp_serve.go:810, :829-830 | WIRED |
| `RealExecutor` | `DefaultBackend` ("ddg") | `defaultWebBackend` fallback + `selectBackend` (backend.go:182-184) | WIRED |
| E2E assertions | transcript engine decisions + archive dir on disk | `TypeEngineDecision` / `TypeCommandProvenance` scans (e2e_opsx_test.go:292-360) | WIRED |

### Requirements Coverage

| Requirement | Source Plan | Status | Evidence |
|-------------|------------|--------|----------|
| CMD-01 | 08-01 | SATISFIED | Truth 1 above |
| CMD-02 | 08-04 | SATISFIED | Truth 2 above |
| CMD-03 | 08-03 | SATISFIED | Truth 3 above |
| CMD-04 | 08-06..08-09 | SATISFIED (residual accepted) | Truth 4 above |
| CMD-05 | 08-01, 08-04 | SATISFIED | Truth 5 above |
| CMD-06 | 08-05 | SATISFIED | Truth 6 above |
| CMD-07 | 08-02 | SATISFIED | Truth 7 above |

No orphaned requirements: ROADMAP maps exactly CMD-01..07 to Phase 8 and every ID is claimed by a plan and verified. (Informational: REQUIREMENTS.md's status column still shows CMD-02/03/04/06 as "Pending" — stale relative to the accepted gate; the manager's state update closes this. Likewise ROADMAP's Wave-7 detail checkbox for 08-09 remains unchecked while the top-level Phase-8 line, STATE.md, and 08-09-SUMMARY all record completion + acceptance.)

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/openspec/register.go | 35 | comment mentions "no implementation yet" — describing the path this phase ELIMINATED, not a stub | Info | none |

No TBD/FIXME/XXX markers, no placeholder returns, no empty handlers in any phase-modified file (scanned: ecosys loader/expand/skills, openspec adapter/register/seeded, toolexec ddg/real/backend, coreexec bash/files/todo/register, e2e_opsx_test, projector, engine). Zero debt-marker blockers.

### Human Verification Required

None pending. The inherently human-gated legs — the real-model zero-continue E2E, the fixable-recovery run, and the residuals (UAT check 3 live-leg; stage-4 model variance) — were witnessed and ACCEPTED by the operator on 2026-08-16 (STATE.md frontmatter: "PHASE 8 COMPLETE (operator witness ACCEPTED 2026-08-16)"), recorded here as the single override. No new human-verification items were raised by this verification.

### Gaps Summary

No gaps. All 7 success criteria are delivered by real, wired, tested code; the two known residuals were adjudicated and accepted by the operator at the gate (recorded as the override above); two minor capture-grounding follow-ups are deferred to Phase 9's re-capture by design (deferred section). Verification combined fresh test runs in this session (including the real-binary gated adapter/ecosys suites) with inspection of the committed witness evidence (logs, stage transcripts, commit 7677051) — no SUMMARY claim was accepted without independent codebase derivation.

---

_Verified: 2026-08-16T00:55:00Z_
_Verifier: Claude (gsd-verifier)_
