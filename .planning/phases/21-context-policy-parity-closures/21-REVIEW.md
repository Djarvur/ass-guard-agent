---
phase: 21-context-policy-parity-closures
reviewed: 2026-09-03T23:09:34Z
depth: deep
files_reviewed: 24
files_reviewed_list:
  - internal/session/gate.go
  - internal/session/session.go
  - internal/session/projector.go
  - internal/session/transcript.go
  - internal/session/manager.go
  - internal/ecosys/hookverdict.go
  - internal/ecosys/hooks.go
  - internal/ecosys/loader.go
  - internal/ecosys/memory.go
  - internal/ecosys/mentions.go
  - internal/runtime/runtime.go
  - internal/runtime/imgscale.go
  - internal/runtime/cron_wiring.go
  - internal/coreexec/register.go
  - internal/coreexec/bash.go
  - internal/shaper/shaper.go
  - internal/provider/provider.go
  - internal/provider/anthropic.go
  - internal/provider/openai.go
  - internal/provider/streaming.go
  - internal/event/events.go
  - internal/acp/types.go
  - internal/modelrouting/factory.go
  - go.mod
findings:
  critical: 0
  warning: 8
  info: 5
  total: 13
status: fixed
---

# Phase 21: Code Review Report

**Reviewed:** 2026-09-03T23:09:34Z
**Depth:** deep (standard per-file + cross-file call-chain tracing on the gate join, mention seam, and thinking/image pipelines)
**Files Reviewed:** 24 production files (of 68 changed; the rest are tests/goldens/testdata)
**Status:** fixed (all 8 warnings closed — see per-finding Fixed lines and 21-REVIEW-FIX.md; the 5 info findings stay open as documented)

## Summary

Reviewed commit range `625dd2d..HEAD` (plans 21-01..21-06). The Phase-17 locked gate contract **holds verbatim**: exactly two `gateCall` sites (both dispatch branches in `runTurn`), one verdict resolution site (`HookRunner.PreToolUseVerdict`), one consumption site (the gate head), verdict-before-rules (D-04), deny-only project authority (D-01 demotion is loud), exit-2-beats-JSON (T-21-02 spoofing rule), and the executor PreToolUse leg is genuinely disposed. `ResolveVerdict` is pure and table-testable as claimed. The image ingress is careful (DecodeConfig-first, decode-safety ceiling before any pixel allocation, sha-keyed atomic writes). `go build`, `go vet`, and `go test -race` pass on every touched package.

The findings below are robustness/consistency gaps, not contract breaks. The two clusters that matter most: (1) the hook-ask head short-circuits two documented fail-safes in the same function (D-07 automation decline, 16-D-18 degraded-client stickiness) — an untested interaction; (2) the 21-06 "ONE rule authority" wiring for @-mentions is neither race-free nor live — it pins the first-wired session's perm store with an unsynchronized once-guard.

Known non-findings honored: pre-existing golangci-lint go1.26/1.27 drift (WINDOWS), corpus-absent thinking goldens (WINDOWS #17).

## Critical Issues

None found.

## Warnings

### WR-01: Hook-ask verdict bypasses the D-07 automation decline and the degraded-client guard

**Fixed:** commit `17d91fe` — hook ask maps to a distinct `gateAskHook` verdict that `gateCall` funnels through the Step 3/Step 5 fail-safes before suspending (deny/allow mappings unchanged; rule evaluation and the Step 4 mode check stay skipped for hook asks); pinned RED-first by `TestGateHookAskFailSafes` (both interactions + the human control).

**File:** `internal/session/gate.go:187-189` (head), `internal/session/gate.go:284-285` (ask mapping), vs `internal/session/gate.go:222-250` (the bypassed guards)
**Issue:** `gateHookVerdict` maps `VerdictAsk` to `gateSuspend` "UNCONDITIONALLY" and returns `decided=true`, so `gateCall` returns before Step 3 (automation-turn fail-safe decline, D-07: "An automation turn NEVER opens a dialog nobody would answer — in BOTH modes") and Step 5 (the sticky -32601 degraded-client decline, 16-D-18). A PreToolUse hook returning `permissionDecision:"ask"` on an automation turn now suspends the turn and enqueues a background dialog the D-07 contract says must fail-safe decline; on a degraded client it re-fires a surface round-trip the sticky guard exists to suppress. The plan locks "unconditionally" only with respect to *mode* ("ask → gateSuspend unconditionally (D-04: even ungated...)") — neither the plan nor any test addresses the automation/degraded interactions (`TestGateAutomationDecline` predates the join; the 21-06 battery covers only ungated mode). Worst case degrades to a decline via the unwired-surface fail-safe, so this is a contract inconsistency rather than a wedge — but it contradicts gate.go's own package doc in the same file.
**Fix:** Route a hook ask through the same Step 3/Step 5 guards the rule-ask path takes — e.g. in `gateHookVerdict`, map `VerdictAsk` to a distinct `gateAskHook` verdict that `gateCall` funnels into Step 3/5 before falling through to `gateSuspend` — or record the bypass as a deliberate divergence in the D-07/D-04 docs and pin it with an automation×hook-ask test.

### WR-02: `readRuleEvaluator` once-guard is not race-free — unsynchronized write vs lock-free turn-time reads

**Fixed:** commit `583ae18` — the WR-03 hoist removed the production write entirely (the store publishes through `r.permStore`, an `atomic.Pointer`); pinned by `TestHookJoin_MentionSeamRaceFree` (concurrent `sessionFor` hammered against live `readAllows` consults, clean under `-race`).

**File:** `internal/runtime/runtime.go:1759-1763` (write under `sessMu`), `internal/runtime/runtime.go:606-612` (`readAllows` reads the field with no lock)
**Issue:** The comment claims "the once-guard keeps concurrent sessionFor construction race-free against turn-time reads", but the guard only serializes writes against other `sessionFor` calls. Turn-time reads in `readAllows`/`resolveMention` (via `expandUserBlocks` → `expandMentions`) take no lock, so a write can race a read with no happens-before edge. Reachable sequence: session A is constructed when `perm.OpenRepaired` fails (the comment itself says residual errors are "environmental (mkdir/stat/create)"), so the seam stays nil; session A runs turns reading `r.readRuleEvaluator`; session B is later constructed with a healthy store and executes the nil-check-and-write — a data race per the Go memory model (func-value field read while written). Not exercised by the current suite, so `-race` stays green.
**Fix:** Assign the seam before any session can run (Runner construction / `NewRunner`), or store it in an `atomic.Value`/`sync.RWMutex`-guarded field.

### WR-03: @-mention rule evaluation is pinned to the first-wired session's perm store — diverges from the live gate authority

**Fixed:** commit `f8707dc` — the store hoisted to the Runner (`r.permStore`): one live instance shared by every session's `GateDeps` AND the @-mention consult (`readRuleEval` resolves the injected test seam first, then the shared store); pinned RED-first by `TestHookJoin_OneLiveRuleAuthority` (session B's `reject_always` click denies session A's mention).

**File:** `internal/runtime/runtime.go:1741-1763`
**Issue:** Each `sessionFor` opens its own `perm.Store` over the same `permissions.yaml`; the gate consumes the *live per-session* store, but `readRuleEvaluator` closes over the FIRST successfully-wired session's store instance forever. In-memory rule mutations do not propagate across store instances: an `allow_always`/`reject_always` dialog click in session B updates B's memory (and the file) but the mention seam keeps consulting session A's snapshot — a Read path rejected in B can still expand via `@mention` in B. The same comment's claim that "mentions and tool calls answer to ONE rule authority" holds only at file-construction time, not live.
**Fix:** Hoist the store to the Runner (open once at construction; all sessions share the live instance — they already share the workDir/file), which also eliminates WR-02. Alternatively have the evaluator consult the *current* session's `GateDeps.Rules` rather than a captured store.

### WR-04: Thinking-stash capture before `flushBatch` can duplicate the same thinking block onto two messages

**Fixed:** commit `3f7b633` — `flushBatch` reports whether it emitted; the assistant-message case drops its captured stash when a batch consumed it; pinned RED-first by `TestProjector_ThinkingNeverDuplicatedAcrossBatchAndText` (the block appeared twice; now exactly once, on the batch).

**File:** `internal/session/projector.go:298-311`
**Issue:** In the `TypeAssistantMessage` case, `th := pendingThinking` captures the stash, then `flushBatch()` runs — and `flushBatch` *attaches* `pendingThinking` to any assistant batch it emits (projector.go:256-266) before nil-ing it. If an `assistant_message` line ever follows unflushed tool calls while the stash is non-empty, the same `ThinkingBlocks` slice rides BOTH the flushed batch message and the assistant text message — duplicated signed thinking on replay (a provider 400 risk, and exactly the shape Pitfall 5 guards against). The comment ("captured BEFORE flushBatch (which resets the stash)") is misleading: flushBatch does not just reset, it consumes. Today's writer makes the ordering unreachable (every non-execute gate outcome appends a `tool_result` line, which flushes the batch first), so tests pass — but the invariant is implicit, undocumented, and unguarded.
**Fix:** In the `TypeAssistantMessage` case, flush first and only then read the stash — e.g. give `flushBatch` a variant that returns whether it emitted (and consumed) the thinking, or set `pendingThinking = nil` inside the emitted-batch branch and re-read it after the call:
```go
case TypeAssistantMessage:
    flushBatch()
    th := pendingThinking // non-nil only if no batch consumed it
    out = append(out, provider.Message{Role: roleAssistant, Content: l.Text, ThinkingBlocks: th})
    pendingThinking = nil
```

### WR-05: `AppendRawThinking` write error silently discarded — violates the package's loudness discipline

**Fixed:** commit `8143014` — the error now lands as one structured `slog.Warn` (turnID, model, error), mirroring the ask_suspended/CR-05 pattern; loud, never blocking the display publish.

**File:** `internal/session/session.go:865`
**Issue:** `_ = s.Manager.AppendRawThinking(turnID, s.Profile.Model, chunk.Raw)`. The package's G-12-3b discipline routes every transcript write through loud failure handling (`appendToolResultLoud`, the CR-05/IN-03 patterns elsewhere in this phase), and `suspendForPermission` itself was just fixed (gate.go:430-434) to log `AppendAskSuspended` failures. A silently dropped thinking line corrupts replay: the projector folds thinking from the transcript (projector.go:282-284), so the next request in an extended-thinking session replays the assistant batch without its signed thinking block — a provider 400 with no diagnostic trail pointing at the lost write.
**Fix:** Mirror the ask_suspended pattern: on error, `slog.Warn("raw_thinking transcript write failed", "turnID", turnID, "error", err.Error())`.

### WR-06: `golang.org/x/image` is marked `// indirect` despite being a direct import

**Fixed:** commit `d3079bd` — `go mod tidy` moved `golang.org/x/image v0.45.0` into the direct require block (go.sum already converged, no sum changes); verified with a clean `CGO_ENABLED=0 go build` and a no-op second tidy.

**File:** `go.mod:39`
**Issue:** `internal/runtime/imgscale.go` imports `golang.org/x/image/draw` and `.../webp` directly, but the module sits in the indirect require block. Verified: `go mod tidy -diff` produces a diff moving it to the direct block. Any CI tidy/generate check will flag this; it also misstates the dependency graph to readers.
**Fix:** Run `go mod tidy` and commit the result (moves `golang.org/x/image v0.45.0` into the first require block, drops the `// indirect` comment).

### WR-07: An image-only prompt whose image is dropped degrades to zero content blocks — empty user message on the wire

**Fixed:** commit `a6afec0` — `ensureTextBearingBlock` appends one fixed-form in-band placeholder (`[image content could not be delivered: <class>]`) whenever a drop leaves no non-empty text block (both legs: D-11 provider-unsupported, D-10 ingress); pinned RED-first end to end (the provider-seed body was EMPTY) and at the ingress unit level (zero-length block list).

**File:** `internal/runtime/runtime.go:1892-1925` (`dropUnsupportedImages`), `internal/runtime/runtime.go:1944-2030` (`ingressImages`), `internal/shaper/shaper.go:264-266`
**Issue:** Both drop legs only emit a *stderr* note (the D-10/D-11 letter) and append nothing in-band. When the prompt's ONLY block was the dropped image (corrupt/undecodable payload on an image-capable provider, or any image on an OpenAI-shape provider), `blocks` becomes empty, the seed is `Message{Content: ""}`, and the shaper's byte-compat path appends `anthropic.NewTextBlock("")` — the Anthropic API rejects empty text content blocks ("text content blocks must be non-empty"), so the turn dies with a provider 400. "The turn proceeds with the text — never a dead turn" holds only when text exists; the no-text corner is unhandled and untested.
**Fix:** When a drop leaves the block list with no text-bearing block, insert a fixed-form in-band placeholder (e.g. `[image content could not be delivered: <class>]`) so the outgoing user message is never empty.

### WR-08: `.claude/settings.local.json` is never read — hooks defined there silently never fire (CC parity gap)

**Fixed:** commit `ff6ccb0` — `loadSettingsLocalHooks` reads `<claudeDir>/settings.local.json` at the project scope rank after `settings.json` (shared `parseSettingsHooks` degradation discipline); pinned RED-first with the committed `testdata/settings-local.json` fixture (`TestSettingsHooksLocalProjectScope`: the deny hook loads, ScopeProject, provenance names the file).

**File:** `internal/ecosys/loader.go:119-120, 139-184`
**Issue:** CC reads project hooks from BOTH `.claude/settings.json` and `.claude/settings.local.json` (the gitignored personal-override scope — the standard place operators put hooks they do not ship). `loadSettingsHooks` reads only `settings.json` at both scopes, yet its comment claims "the SAME files CC reads". A repo-parity operator who configures PreToolUse policy in `settings.local.json` gets silent non-enforcement — for a *deny* hook that is a silent security-policy gap on the phase's own CC-parity terms. No plan doc mentions `settings.local.json` (grep across the phase directory is empty), so this looks like a blind spot rather than a recorded scope cut.
**Fix:** Add a third scope read (`<claudeDir>/settings.local.json`, project scope rank, warn-skip on failure), or record the divergence explicitly in 21-CONTEXT and the loader comment.

## Info

### IN-01: `ValidateAndScaleImage`'s `mediaType` parameter is unused

**File:** `internal/runtime/imgscale.go:136-138`
**Issue:** The declared media type is never consulted (the decoded format wins, T-21-16) — the dead parameter misleads callers into thinking the declared type participates in validation.
**Fix:** Drop the parameter (the one caller passes `b.MediaType`), or use it only in a debug/audit note.

### IN-02: `@dir` listings expand with no Read-rule consult

**File:** `internal/runtime/runtime.go:580-589`
**Issue:** `resolveMention` gates `@file` content through `readAllows` but the `IsDir` branch lists names+sizes ungated. Plan-letter sanctioned (21-04 pins the gate on "@file" only), but file names and sizes of a Read-denied directory still reach the prompt.
**Fix:** Consider consulting `readAllows(abs)` before the listing for symmetry; at minimum document the exemption next to the branch.

### IN-03: URI-form image blocks are silently ignored

**File:** `internal/runtime/runtime.go:1959-1963`, `internal/acp/types.go:73-75`
**Issue:** A block with `Type == "image"` and a `uri` but no `data` passes through ingress unchanged (no `DataRef`, no note), never reaches the provider (the shaper's `imageBlocksOf` skips `DataRef == ""`), and emits no loud note — violating the phase's own never-silent D-10 discipline for an ACP-legal input shape.
**Fix:** Emit one fixed-form drop note ("uri-form image not fetched") at ingress, mirroring the base64-decode-failure leg.

### IN-04: `@file` expansion reads whole files uncapped

**File:** `internal/runtime/runtime.go:595-601`
**Issue:** `mentionFileSection` uses `os.ReadFile` with no size cap — a `@huge.log` mention loads the entire file into the prompt. Consistent with the Read tool's own unbounded read (coreexec/files.go:92), so not a new inconsistency, but asymmetric with D-08's hard caps on the memory injection in the same phase.
**Fix:** Apply a per-mention cap (the `memoryPerFileCapBytes` idiom) with a loud truncation note.

### IN-05: Package-global memory cache never evicted

**File:** `internal/ecosys/memory.go:331-335`
**Issue:** `memCache` accumulates one entry per distinct memory-file path for the process lifetime with no eviction. Bounded by distinct paths seen; benign for a session process, worth noting for long-lived single-process deployments.
**Fix:** Cap the map size or clear it per `DiscoverMemoryFiles` run; acceptable to leave with a comment.

---

_Reviewed: 2026-09-03T23:09:34Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
_Fixed: 2026-09-03T23:34:26Z — all 8 warnings (17d91fe, 583ae18, f8707dc, 3f7b633, 8143014, d3079bd, a6afec0, ff6ccb0); see 21-REVIEW-FIX.md_
