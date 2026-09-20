---
phase: 21-context-policy-parity-closures
fixed_at: 2026-09-03T23:34:26Z
review_path: .planning/phases/21-context-policy-parity-closures/21-REVIEW.md
iteration: 1
findings_in_scope: 8
fixed: 8
skipped: 0
status: all_fixed
---

# Phase 21: Code Review Fix Report

**Fixed at:** 2026-09-03T23:34:26Z
**Source review:** .planning/phases/21-context-policy-parity-closures/21-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 8 (all warnings; the 5 info findings stay open as documented)
- Fixed: 8
- Skipped: 0

## Fixed Issues

### WR-01: Hook-ask verdict bypasses the D-07 automation decline and the degraded-client guard

**Files modified:** `internal/session/gate.go`, `internal/session/gate_test.go`
**Commit:** `17d91fe`
**Applied fix:** `gateHookVerdict` maps `VerdictAsk` to a distinct `gateAskHook` action instead of returning `gateSuspend` decided; `gateCall` funnels it into the ask path so Step 3 (D-07 automation decline, both modes) and Step 5 (16-D-18 degraded-client decline) apply, while rule evaluation (verdict-before-rules, D-04) and the Step 4 mode check (a hook ask suspends even ungated) stay skipped for hook asks. Deny/allow mappings unchanged. Pinned RED-first by `TestGateHookAskFailSafes`: the automation and degraded subtests failed with `stop = "ask"` pre-fix; both now decline with the correct note forms and zero surface fires, and the human+capable control still suspends.

### WR-02: `readRuleEvaluator` once-guard is not race-free

**Files modified:** `internal/runtime/hooks_join_wiring_test.go`
**Commit:** `583ae18`
**Applied fix:** The WR-03 hoist removed the racing production write entirely — the rule authority publishes through `r.permStore` (an `atomic.Pointer[perm.Store]`) and the `readRuleEvaluator` field is production-read-only (test seam only). Pinned by `TestHookJoin_MentionSeamRaceFree`: concurrent `sessionFor` construction hammered against live `readAllows` consults stays clean under `-race` and the shared store opens.

### WR-03: @-mention rule evaluation pinned to the first-wired session's perm store

**Files modified:** `internal/runtime/runtime.go`, `internal/runtime/hooks_join_wiring_test.go`
**Commit:** `f8707dc`
**Applied fix:** The perm store is hoisted to the Runner (`r.permStore`): opened lazily by the first healthy `sessionFor` (under `sessMu`, published atomically) and shared by every session's `GateDeps` AND the @-mention Read-rule consult — `readRuleEval` resolves the injected test seam first, then the shared store; a failed open still degrades that session rule-less with the loud log and the next `sessionFor` retries. Pinned RED-first by `TestHookJoin_OneLiveRuleAuthority`: session B's `reject_always` click (`Store.ForbidTool`, the gate's Forbid seam) now denies session A's `@notes.md` expansion (pre-fix the store was never Runner-scoped and the click never propagated).

### WR-04: Thinking-stash capture before `flushBatch` can duplicate the thinking block

**Files modified:** `internal/session/projector.go`, `internal/session/projector_test.go`
**Commit:** `3f7b633`
**Applied fix:** `flushBatch` reports whether it emitted a batch; the `TypeAssistantMessage` case drops its captured stash when a batch consumed it (the flush-first discipline — the stash rides the text message only when no batch took it). Pinned RED-first by `TestProjector_ThinkingNeverDuplicatedAcrossBatchAndText`: pre-fix the thinking appeared twice (carriers [1 2]); now exactly once, on the assistant batch.

### WR-05: `AppendRawThinking` write error silently discarded

**Files modified:** `internal/session/session.go`
**Commit:** `8143014`
**Applied fix:** The error lands as one structured `slog.Warn` (`turnID`, `model`, `error`), mirroring the ask_suspended/CR-05 pattern — loud, never blocking the display publish. (Trivial per the rules; no RED pin required.)

### WR-06: `golang.org/x/image` mis-marked `// indirect`

**Files modified:** `go.mod`
**Commit:** `d3079bd`
**Applied fix:** `go mod tidy` moved `golang.org/x/image v0.45.0` into the direct require block; `go.sum` was already converged (no sum changes). Verified with a clean `CGO_ENABLED=0 go build ./...` and a no-op second tidy (`go mod tidy -diff` clean).

### WR-07: Image-only prompt with the image dropped leaves zero content blocks

**Files modified:** `internal/runtime/runtime.go`, `internal/runtime/img_capability_test.go`
**Commit:** `a6afec0`
**Applied fix:** `ensureTextBearingBlock` appends one fixed-form in-band placeholder text block (`[image content could not be delivered: <class>]`) whenever a drop leaves no non-empty text block — applied on both legs (`dropUnsupportedImages` D-11 with class `provider does not support images`; `ingressImages` D-10 with class `image failed ingress validation`). The stderr note discipline is unchanged (still exactly one D-11 note). Pinned RED-first end to end (`TestImageCapability_ImageOnlyDropKeepsNonEmptyBody`: the provider-seed body was EMPTY pre-fix) and at the ingress unit level (`TestImageIngress_ImageOnlyDropKeepsTextBearingBlock`: zero-length block list pre-fix).

### WR-08: `.claude/settings.local.json` never read

**Files modified:** `internal/ecosys/loader.go`, `internal/ecosys/hooks_test.go`, `internal/ecosys/testdata/settings-local.json` (new fixture), `internal/runtime/runtime.go` (comment)
**Commit:** `ff6ccb0`
**Applied fix:** `loadSettingsLocalHooks` reads `<claudeDir>/settings.local.json` at the project scope rank, after `settings.json` (CC's local-overrides precedence direction); the per-file logic is extracted into `parseSettingsHooks` carrying the unchanged degradation discipline (oversized/malformed warn-skip, absent is silent, 60s timeout default). Pinned RED-first with the committed fixture via `TestSettingsHooksLocalProjectScope`: the PreToolUse deny hook loads with `ScopeProject` (so the D-03 firing order and deny-wins resolver treat it exactly like settings.json hooks) and provenance names the local file.

## Verification

All gates ran in the MAIN CHECKOUT (`workflow.use_worktrees=false` — no isolation worktree), so the numbers are reproducible from the tree as committed:

- `go vet ./...` — clean.
- `go test -race -count=1 ./internal/session/ ./internal/ecosys/ ./internal/shaper/ ./internal/runtime/` — all ok (runtime 59.8s).
- `go test -race -count=1 ./...` (full repo) — all ok, zero failures.
- `CGO_ENABLED=0 go build ./...` — ok.
- `go mod tidy` converges (second run and `go mod tidy -diff` produce no changes).
- Normal git commits with hooks on (no `--no-verify`); every commit message prefixed `fix(21):`.
- `mise ci lint` was NOT chased: the golangci go1.26/go1.27 drift is the known pre-existing WINDOWS ledger item.

## Skipped Issues

None — all 8 in-scope findings fixed. The 5 info findings (IN-01..IN-05) remain open by design (documented in the review; not in the critical+warning fix scope).

---

_Fixed: 2026-09-03T23:34:26Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
