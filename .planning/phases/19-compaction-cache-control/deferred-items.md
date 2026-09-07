# Phase 19 Deferred Items (out-of-scope discoveries)

## [2026-09-06, 19-03] mise lint gate broken repo-wide by golangci-lint version drift

The `mise run lint` gate fails on every package (including ones untouched since
Phase 15) in the current environment:

- mise resolves `golangci-lint = "2"` to **2.13.2** (built with go1.27.0);
  `.golangci.yml` was tuned for **2.12.x** (STATE's research note pins 2.12.2).
- 2.13.2 deprecates `exhaustruct` in favor of `exhaustruct_v5`, so the config's
  wildcard exclusion (`- linters: [exhaustruct] text: ".*"`) no longer matches —
  the new linter flags every partial struct literal in the repo (100+ findings).
- New/renamed linters (`noinlineerr`, `stringsseq`, `slicesbackward`, `wsl_v5`,
  `tparallel`) fire on large amounts of pre-existing code that passed 2.12.x.
- The stale `~/go/bin/golangci-lint` (2.12.2, built go1.26) cannot typecheck
  against the installed go1.27.1 toolchain at all (panics: "file requires newer
  Go version").

Repro: `mise exec -- golangci-lint run ./internal/shaper/...` (untouched by
phases 19-21) shows the same failures.

Suggested fix (config/tooling reconciliation, one commit): pin the linter minor
in `.mise.toml` (e.g. `golangci-lint = "2.12"` or the latest 2.13.x) and update
`.golangci.yml` exclusions for the renamed linters, or migrate the config
forward to 2.13.x deliberately.

## [2026-09-06, 19-03] internal/runtime full-package flakes under machine load

`go test -race ./internal/runtime/ -count=1` intermittently fails (reproduced
at the pre-19-03 commit 00c9da6, so NOT caused by 19-03):

- `TestIntegration_RealStreamingThroughACP` (4.3s fail / 0.4s pass in isolation)
- `TestAskPark_PromptResponsePrecedesResolution` (13-15s, deadline-style)

Both pass in isolation. The host was under sustained load during 19-03's
repeated full-suite verification runs. Worth a dedicated timing-deadline review
next time `internal/runtime` is touched.

## [2026-09-06, 19-03] gate_test.go allow_once race — FIXED in-plan

`TestGateOutcomeMatrix/allow_once_executes_without_persisting` read
`toolResultsFor` without the `gateWaitFor` its sibling subtests use; 19-03's
added parallel batteries widened the scheduling window to ~50% full-suite
flake. Fixed in a5a46a1 (documented as a deviation in 19-03-SUMMARY.md).

## [2026-09-06, 19-04] Compacting note has no session/update frame (degrade taken)

Task 2's compacting note degraded to the warning-counter family (one structured
stderr line + one counter per compaction start): the landed ACP session/update
vocabulary (16-01) has exactly five kinds (agent_message_chunk, tool_call,
tool_call_update, plan, agent_thought_chunk) — no status/info frame — and the
only text-bearing frame (agent_message_chunk) is exactly what PAR-01's
bus-isolation prohibition bars from the compaction path (Pitfall 4). The plan
pre-authorized this degrade. A proper user-visible note needs a deliberate wire
decision (a new v1 frame kind does not exist; candidates: a dedicated
session/update kind if the spec grows one, or a client-side note surfaced by
Phase 20's /compact handler at the command surface).

## [2026-09-06, 19-04] Producing turn's own window is not re-seeded by its loop-head marker

D-07's "summarize → write marker → build request → send" sequence lands with a
one-turn delay under 19-03's pinned reset-point position rule (the marker
resets turns that START after it — test-enforced by
TestProjector_CompactionResetPoint). The compacting turn's own requests keep
their pre-phase window; the NEXT turn projects the summary seed + budget-filled
tail. Consequences: (a) criterion 3's overflow retry-once recovers the SESSION
(future turns) rather than guaranteed-shrinking the producing turn's outgoing
window — a fat mid-turn tail overflows again after the single retry and fails
loudly through the existing error path; (b) the threshold check's blocking
pause benefits the next turn, not the current one. Candidate fix (needs
architect sign-off — 19-03's pin would need a carve-out): a same-turn marker
(TurnID == projected turn, no pre-user-marker present) serving as that turn's
reset point; subagent-safe by the TurnID key (a parent marker never matches a
subagent turn), and inert in every 19-03 fixture except the never-compacted
case the pins do not cover.

## [2026-09-06, 19-04] internal/runtime flake family wider than the two documented

`TestCronWiring_AutomationTurnDeclinesGatedAsk` also failed once under
full-suite machine load (17.7s runtime) alongside the documented
TestAskPark_PromptResponsePrecedesResolution; both passed in isolation and the
full package passed on rerun. 19-04's changes are structurally inert on the
disabled path (maybeCompact returns before any work when settings are unset),
so this is the same load-sensitive family — the timing-deadline review noted at
19-03 should cover it.

## [2026-09-06, phase-19 close] flake family member: TestCronWiring_QueueBehindActiveTurn

Failed once in the phase-close full-repo `go test -race ./...` gate (14.1s,
deadline-style) while every other package was green; passed 2/2 in isolation
immediately after. Same load-sensitive `internal/runtime` deadline family as
TestIntegration_RealStreamingThroughACP / TestAskPark_PromptResponsePrecedesResolution
/ TestCronWiring_AutomationTurnDeclinesGatedAsk — the family, not the individual
test, is the unit that needs a timing-deadline review.

## 19-07 (2026-09-07): stale battery doc comment on TestProjector_SameTurnCarveOut

The battery-level doc comment (internal/session/projector_test.go ~:2177, above `func TestProjector_SameTurnCarveOut`) still describes the OLD precedence: "reshapes that turn's projection ONLY when the engine armed the per-turn override (SetRetryCompactedTurn) AND no pre-user marker exists" and "the pre-user marker keeps 19-03's winning scan". After 19-07 the armed override takes precedence over the pre-user scan (with the no-same-turn-marker fallback). Editing it inside 19-07 would have introduced deletions outside the inverted subtest and violated the pinned zero-deletions audit vs 06d92d1, so it was left as-is. Fix opportunistically in the next plan that legitimately touches that file (a pure comment fix produces a deletions-bearing hunk vs 06d92d1, which is fine once 19-07's audit is no longer the active gate).
