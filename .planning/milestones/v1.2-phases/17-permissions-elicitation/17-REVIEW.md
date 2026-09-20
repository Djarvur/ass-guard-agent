---
phase: 17-permissions-elicitation
reviewed: 2026-09-02T21:41:36Z
depth: deep
files_reviewed: 4
files_reviewed_list:
  - internal/acp/types.go
  - internal/acpserve/ask_surface.go
  - internal/acpserve/ask_surface_test.go
  - internal/acpserve/permissions_e2e_test.go
findings:
  critical: 0
  warning: 1
  info: 0
  total: 1
status: fixed
---

# Phase 17: Code Review Report — Gap-Closure 17-06 Re-Review

**Reviewed:** 2026-09-02T21:41:36Z
**Depth:** deep (cross-file: acp types → acpserve parse site → session gate; canonical-schema diff)
**Scope:** 388d517..HEAD — commits 1538673 (RED tests), f372db4 (GREEN fix), d445ab3 (docs). Supersedes the phase's first-round review (history in git).
**Files Reviewed:** 4
**Status:** issues_found (0 critical, 1 warning — a test gap, not a behavior defect)

## Summary

The 17-06 gap-closure is correct. It closes UAT gap G-17-1 (live Zed 1.18.0 answers
`session/request_permission` with the canonical nested outcome object) with a minimal,
fail-safe-preserving type reshuffle, and the RED→GREEN→docs commit discipline is genuine.

What was verified, and how:

**Wire fidelity (canonical sources fetched live during this review).** The canonical
docs source (`docs/protocol/v1/tool-calls.mdx` in agentclientprotocol/agent-client-protocol)
pins the exact example:

```json
{"result": {"outcome": {"outcome": "selected", "optionId": "allow-once"}}}
...
{"result": {"outcome": {"outcome": "cancelled"}}}
```

and the canonical schema crate (`agent-client-protocol-schema/src/v1/client.rs`) defines
`RequestPermissionResponse { outcome: RequestPermissionOutcome }` — an internally tagged
union (`tag = "outcome"`, snake_case) with `Selected(SelectedPermissionOutcome)` whose
`option_id` serializes camelCase to `optionId`. The new Go types
(`internal/acp/types.go:342-356`) model this verbatim: no invented fields, v1 spellings
exact, and every schema def name cited in the new comments
(`RequestPermissionResponse`, `RequestPermissionOutcome`, `SelectedPermissionOutcome`)
is a genuine canonical def name. The optional `_meta` the canonical response carries is
tolerated by Go's default unknown-field ignoring — correct. The elicitation response
(`ElicitationOutcomeFrame`, flat `{"action","content"}`) is untouched and remains
canonical, and `permissions_e2e_test.go:389-399` documents the deliberate asymmetry.

**Fail-safe preservation (check 2) — all intact.** Sentinels unchanged
(`internal/acpserve/ask_surface.go:98-102`); every malformed path declines, none allows:

- flat one-level outcome → `json.Unmarshal` type error → `errPermissionOutcomeBad`
  (ask_surface.go:157-162) — newly pinned by the "flat outcome nonconformance" subtest;
- unknown inner discriminator, `{"outcome":{}}`, or `{"outcome":null}` (zero value) →
  `errPermissionOutcomeUnknown` (ask_surface.go:172-176) — pinned by the
  "unknown inner discriminator" subtest;
- `selected` without `optionId` → `AskOutcome{Selected: ""}` → the session gate's
  default unknown-option decline (`internal/session/ask.go:960-965`) — verified by code
  reading, but unpinned (WR-01 below);
- `-32800` → Cancelled (ask_surface.go:122-127), `-32601` → `errPermissionUnsupported` +
  `Unsupported: true` → sticky degrade (ask_surface.go:138-149; ask.go:918-920),
  D-14 terminal fallback and transport failures → Err decline (ask_surface.go:129-134)
  — all byte-identical to pre-diff behavior.

**Concurrency (check 3) — no exposure.** The parse site is pure local computation on
values already received (`json.Unmarshal` + a three-case switch on a string) inside
`Fire`, which runs on the ask-queue pump goroutine. The diff adds no locks, changes no
lock ordering, and introduces nothing blocking under the registry mutex. The only mutex
in the file (`ElicitationAsk.mu`) is pre-existing and untouched.

**Test quality (check 4) — genuine RED, no weakening.** The RED commit (1538673)
precedes the fix and is genuinely RED under the old flat decoder: the nested payload's
`outcome` object cannot unmarshal into the old string field, so the old code fail-safed
every answer — the "selected" subtest (`want Selected == allow_always`) fails, and the
e2e happy path times out at `permWaitFile` because the gated call never executes. The
GREEN commit's only test-file changes are cosmetic (nolint vocabulary matching the
sibling battery convention at ask_surface_test.go:392, plus one line wrap) — zero
assertions weakened. The two new fail-safe subtests assert the sentinel families via
`errors.Is`, strengthening the battery. The e2e simulator (`permAnswerSelected`,
permissions_e2e_test.go:382-387) now answers the canonical shape, so the full
wire→decode→gate→execute→persist stack exercises the same decode live Zed exercises.

**Verification evidence (run during this review).** `go vet` clean on
internal/acp, internal/acpserve, internal/session; `gofmt -l` empty on all four files;
`go test -race -count=1 ./internal/acp/ ./internal/acpserve/` green (1.8s / 53.0s,
e2e included).

## Critical Issues

None.

## Warnings

### WR-01: The selected-without-optionId fail-safe path is load-bearing, documented, and unpinned by any test

**Fixed:** commit `2f56969` — gate battery rows `TestGateOutcomeMatrix/unknown_option_id_declines_fail-safe/{empty_optionId_(selected_without_optionId), non-canonical_optionId}` (decline form + zero executions/rule writes) and dispatch subtest `TestPermissionAskDispatch/selected_without_optionId_passes_through_empty` (pass-through shape `Selected == ""`, no Err, not cancelled). All pass against current code under `-race`.

**File:** `internal/acpserve/ask_surface.go:167-171` (the claim), `internal/session/ask.go:960-965` (the relied-on branch), `internal/acpserve/ask_surface_test.go:206-263` (the battery that stops one case short)

**Issue:** The new comment asserts that `{"outcome":{"outcome":"selected"}}` (missing
`optionId`) "falls through to the session gate's default unknown-option fail-safe branch
… the correct decline." That claim is true today — `resolvePermissionOutcome`'s `default:`
branch declines any `Selected` value that is not one of the four canonical option ids,
with a loud `slog.Warn` — but nothing pins it. The dispatch battery covers
selected/cancelled/flat/unknown-discriminator and stops exactly one case short; a grep
across all of `internal/` finds no test constructing a `Selected` outcome with an empty
or non-canonical option id, and `internal/session/gate_test.go` has no unknown-option
row. This is precisely the wire-shape→fail-safe seam 17-06 exists to tighten, and the
cross-package reliance lives in a comment only — a future refactor of the gate's switch
(e.g. treating an empty selection specially, or reordering cases) could silently change
the decline into something else with no test failing.

**Fix:** Add one row to the session gate battery (the decline is the gate's behavior,
so that is where the pin belongs):

```go
t.Run("unknown option id declines fail-safe", func(t *testing.T) {
    // answers: []AskOutcome{{Selected: ""}}            // selected with no optionId
    // and/or  []AskOutcome{{Selected: "banana_opt"}}   // a non-canonical id
    // assert: one ERROR tool result carrying the decline form
    //         ("the dialog returned an unknown option"), zero executions.
})
```

Optionally also a dispatch subtest answering `{"outcome":{"outcome":"selected"}}` and
asserting the pass-through shape (`Selected == ""`, `Err == nil`, `Cancelled == false`)
so the surface/gate division of labor is explicit at both sites.

## Info

None.

---

_Reviewed: 2026-09-02T21:41:36Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
