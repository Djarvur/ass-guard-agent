---
phase: 19-compaction-cache-control
verified: 2026-09-06T23:45:00Z
status: human_needed
score: 7/9 must-haves verified
behavior_unverified: 2 # truths present + wired whose asserted outcome no test can prove: the SC-3 producing-turn retry rescue (structurally masked, architect sign-off pending) and the SC-1 live-LLM coherence leg (operator-gated by design)
overrides_applied: 0
human_verification:
  - test: "Architect decision — same-turn marker carve-out vs next-turn semantics (CR-01 / deferred-items.md)"
    expected: "One of: (a) accept next-turn recovery semantics (criterion 3's retry rescues the session, not the producing turn) and re-pin the criterion wording; or (b) rule the producing turn must be rescuable and schedule the same-turn carve-out (project the retry turn as post-marker via a per-turn override, per 19-REVIEW's fix sketch) with a content-sensitive regression test"
    why_human: "A design decision against a test-pinned cross-plan contract (19-03's position rule), deliberately deferred by the 19-04 executor and independently flagged critical by the code reviewer — not machine-resolvable without choosing between two pinned contracts"
  - test: "Operator live-LLM compaction check — run a real long session past the 80% threshold (or force a low threshold via set_config_option), let it compact, continue working"
    expected: "Conversation continues coherently with earlier-turn context preserved via the summary (where v1.1 silently lost turns); note that compaction is currently INVISIBLE to the user (the compacting note degraded to stderr+counter — no session/update status frame exists in the landed v1 vocabulary), which itself may need a UX decision"
    why_human: "Summary quality and user-visible coherence under a real model are the research validation table's explicitly operator-gated leg; offline tests prove the machinery (marker, seed, tail, headroom) with a scripted summarizer, not summary fidelity"
behavior_unverified_items:
  - truth: "SC-3: on a provider overflow error, recovery retries once post-compaction instead of failing the turn"
    test: "Trigger a deterministic mid-turn overflow (payload over the provider limit) and observe the retry"
    expected: "For the criterion's letter the turn should complete after the single retry; as implemented the retry re-sends a byte-identical window (the marker lands after the producing turn's user message and projector.go:314 accepts markers only strictly before it), so a deterministic overflow fails the turn after exactly one retry — recovery reaches the NEXT turn"
    why_human: "The retry mechanism, per-turn guard, and fail-twice bound are all test-pinned green, but the green 'recovery' test's fake provider is content-blind (script succeeds regardless of payload), so no test distinguishes a shrunk resend from an identical one; the producing-turn rescue is structurally impossible under the pinned position rule — an architect must choose which contract gives"
  - truth: "SC-1c: the user observes the conversation continuing coherently where v1.1 would have lost earlier turns"
    test: "Drive a real over-threshold session with the live provider and continue the conversation post-compaction"
    expected: "Turns after compaction reference earlier-turn facts preserved in the summary seed; no silent context loss"
    why_human: "Live-model summary fidelity is the research validation table's operator-gated leg; the offline E2E (TestCompaction_EndToEnd) proves marker/seed/tail/headroom with a scripted summarizer, which cannot speak to coherence"
---

# Phase 19: Compaction + cache_control Verification Report

**Phase Goal:** Long sessions stop silently losing earlier turns: threshold-triggered light-tier compaction appends an additive typed marker the Projector treats as a durable reset-point class, and cache_control ephemeral breakpoints ship on every system block — the corpus-proven parity-faithful lever (zcode has no auto-compact; docs/compaction-decision.md settles design).
**Verified:** 2026-09-06T23:45:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

Machine-verified against the codebase and named tests the verifier executed itself (all runs `-race -count=1`): `internal/shaper`, `internal/profile`, `internal/paritycli`, `internal/provider`, `internal/session`, `internal/modelrouting`, `internal/providerfactory`, `internal/acpserve` (88.8s full package) — all green. `go vet` clean on all nine touched packages. SUMMARY claims were cross-checked against code, not trusted.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC-1a: over-threshold session compacts automatically — blocking compact at the loop head, summary-bearing marker on disk, next turn seeded, headroom restored, disabled = zero delta | ✓ VERIFIED | `compaction.go` read: inclusive `overThreshold` (:111), `estimateLinesSinceLastRequest` (:141, folded line bytes/4 since last `request_shaped` — never the request's own Bytes), blocking `compact` (:215, private stream consumer, D-09 degrade :328, chaining, 2048 cap, 60s timeout); `session.go:514` loop-head `maybeCompact` (disabled short-circuit first), `session.go:975` `lastInputTokens` read model. Tests run green by verifier: `TestCompaction_Threshold`, `TestCompaction_Chaining` (incl. zero-bus-publishes + profile-copy + A4 default subtests), `TestCompaction_SummarizerFailure` (4 degrade modes), `TestCompaction_LoopHead`, `TestCompactNow`, `TestCompaction_EndToEnd` |
| 2 | SC-1b: threshold ~80% default and configurable end-to-end (config keys, menu advertise + effective values, persist-then-apply, live apply, clamps, no context-limit key) | ✓ VERIFIED | `modelrouting/config.go:34` Compaction block + `defaults/config.yaml:44-46` (threshold_pct 80 / enabled true); `config_surface.go` both ids handled (pending machinery deleted — grep 0), `SetCompactionHook` (:227); `acp_serve.go:331` binds `runner.ApplyCompactionSettings` → `runtime.go:2438` per-session `SetCompactionSettings` under turn mutex + construction seam `:1759`; `clampCompactionPct` 1..100. Tests green: `TestCompaction` (modelrouting), `TestConfigWrite_CompactionRoundTrip`, `TestCompaction_SettingsFromConfig`, `TestCompactionOptions`, `TestCompactionLive_BootDefaults`, `TestCompactionLive_MenuThresholdRoundTrip` |
| 3 | SC-1c: the user observes the conversation continuing coherently where v1.1 lost earlier turns | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Machinery fully proven offline (truth 1); live-LLM summary fidelity is the research validation table's explicitly operator-gated leg — see Human Verification 2 |
| 4 | SC-2: tool_use/result pairs atomic, thinking never rewritten mid-chain, pinned Projector tests prove boundary-survival and pair-atomicity | ✓ VERIFIED | `projector.go` read: `compactionMarkerIdx`/`projectCompacted` (:123-194, D-06 durable seed, summary-only after later boundary), `boundCompactionTailByBudget` (:236, newest-first group walk, stop-before-exceed, group-head cut — orphaned tool result impossible), `messageCost` = folded JSON/4. Tests green (10 battery fns, 0 failures): `TestProjector_CompactionResetPoint` (5 subtests: marker-wins seed, D-06 durability, no-marker byte-identity, subagent in-flight safety, most-recent-marker), `TestProjector_CompactionTailCut` (pair-atomic multi-turn fill, 64-message zero-budget fallback, thinking-untouched) |
| 5 | SC-3: on a provider overflow error, recovery retries once post-compaction instead of failing the turn | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Mechanism verified: `session.go:548-553` intercept (`provider.IsOverflow` + per-turn `overflowRetried` guard, `continue` re-projects, no loop), `TestCompaction_OverflowRetryOnce` both subtests green (exactly 3 stream calls on fail-twice). BUT the producing turn's rescue is structurally impossible: the marker lands during turn T (after T's user message) and `projector.go:314` accepts markers only strictly BEFORE the projected turn's user message, so the retried projection is byte-identical to the overflowing one — a deterministic overflow always reaches the fail-twice path. The green "recovery" subtest's fake provider is content-blind (script #3 succeeds regardless of payload). Confirmed independently from code (CR-01), from the 19-04 executor's deferred-items.md carve-out entry, and from the test. Architect sign-off pending — see Human Verification 1 |
| 6 | SC-4a: every outgoing request carries cache_control {"type":"ephemeral"} on each system block (keep-last-4 at the API cap), visible in the marshaled body/request log | ✓ VERIFIED | `profiles/zcode/profile.yaml:5` `system_cache_control: true` → `loader.go:57` applies to every system block → `shaper.go:167-183` gated emission via `anthropic.NewCacheControlEphemeralParam()`; degrade arithmetic verified by verifier (`flagged-seenFlagged <= 4` keeps exactly the LAST four, order preserved); CacheControl appears ONLY in the systemBlocks loop (never tools/messages — prohibition grep clean). Production serve path loads through `profile.NewLoader().Load()` (`acp_serve.go:129`). Tests green: `TestShape_CacheControlEmission`, `TestShape_CacheControlCapDegrade`, golden pin in `TestMidTurnCapture_GoldenShape` |
| 7 | SC-4b: the parity cache-discipline probe flips green on placement against the committed pin fixture; baseline untouched | ✓ VERIFIED | `TestCacheProbe_PlacementFlip` green (composition derives from `profile.TextBlock.CacheControl` — `composeCacheProbeInput`); `git diff --name-only 73440bd -- internal/parity/cacheprobe.go` EMPTY (baseline byte-stable, prohibition held); WINDOWS ledger item 5 → fixed with the 19-01 reason; extractor preserves the declaration on re-capture (`TestExtractFromRollout_CacheControlDeclaration` green). The live `ass-guard parity` A/B gate stays operator-gated by design (live model calls) |
| 8 | PAR-01 substrate: non-2xx rejections surface as ClassifyHTTP-typed error chunks (never a defaulted end_turn done chunk); IsOverflow is a message-class matcher with no new ErrorKind; happy SSE path unchanged | ✓ VERIFIED | `streaming.go:244-246` status check before any drain-path construction, `rejectStreamError` (:286-307, bounded read + synchronous close + ClassifyHTTP); `errors.go:158` IsOverflow (case-insensitive contains over `errors.As` *ProviderError); ErrorKind value set unchanged (Transient/Structural/Exhausted only). Tests green: `TestStream_Non2xxError`, `TestStream_Non2xxError_MalformedBody`, `TestIsOverflow`, full provider package |
| 9 | 19-03: summary payload additive on the marker line (redacted path, round-trip, field-tolerant); transcripts without markers project byte-identically to pre-phase | ✓ VERIFIED | `transcript.go:198` Summary field (`summary`, omitempty); `manager.go:390` AppendCompaction extended; `TestTranscriptNewKinds` green incl. the compaction-summary-payload battery; no-marker byte-identity pinned inside `TestProjector_CompactionResetPoint` (boundary-only hand-derived pin) and the pre-phase replay identity subtest of `TestCompaction_EndToEnd` |

**Score:** 7/9 truths verified (2 present, behavior-unverified — both routed to human verification)

### Prohibition Checks

All plan prohibitions verified with machine-checkable evidence:

| Prohibition | Verdict | Evidence |
|-------------|---------|----------|
| cache_control never on tools/message blocks (19-01) | VERIFIED | grep: CacheControl only in the systemBlocks loop of shaper.go; `TestShape_CacheControlEmission` decodes the marshaled system array only — placement classes guarded by the untouched probe baseline |
| probe baseline never edited to manufacture green (19-01) | VERIFIED | `git diff 73440bd -- internal/parity/cacheprobe.go` empty across the whole phase |
| emitted value carries no field beyond type ephemeral (19-01) | VERIFIED | `NewCacheControlEphemeralParam` only; TTL omitzero; golden pin asserts exactly `{"type":"ephemeral"}` per block |
| no new ErrorKind; no generic-400 misclassification (19-02) | VERIFIED | Kind constants unchanged; `TestIsOverflow` false-table covers generic 400/429/network |
| error-envelope parsing never panics (19-02) | VERIFIED | `TestStream_Non2xxError_MalformedBody` green; bounded 8 KiB read |
| transcript never mutated/rewritten by compaction (19-03) | VERIFIED | compaction.go only calls AppendUsage/AppendCompaction; projector folds mechanically, drops complete leading groups only |
| durable seed never displaced by a later boundary (19-03) | VERIFIED | `projectCompacted` runs whenever a marker precedes the user message regardless of later boundaries; D-06 subtest green |
| tail never starts with an orphaned tool result (19-03) | VERIFIED | group-head cut in `boundCompactionTailByBudget`; pair-atomic subtest green |
| summarizer never publishes to the client bus (19-04) | VERIFIED | zero `Bus` references in compaction.go; zero-client-publishes subtest green |
| estimate never uses the request's total body size (19-04) | VERIFIED | `estimateLinesSinceLastRequest` sums post-anchor transcript lines only; "never derived from the request's total bytes" subtest green |
| overflow never retries more than once per turn (19-04) | VERIFIED | per-turn local guard never reset; fail-twice subtest: exactly 3 stream calls then existing error path |
| no context-limit config key / menu entry (19-05) | VERIFIED | resolved inside `runner.compactionContextLimit` from the capability table; no key in config.go/defaults.yaml; twelve-entry menu pinned |
| out-of-range threshold never persisted (19-05) | VERIFIED | validation-first typed rejection precedes the layer write; `TestCompactionOptions` |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/session/compaction.go` | compaction engine (threshold, estimate, blocking compact, D-09 degrade, CompactNow, settings) | ✓ VERIFIED | 437 lines, all symbols present and substantive |
| `internal/session/compaction_test.go` | the 6-function offline battery | ✓ VERIFIED | 1241 lines; 10 top-level fns incl. 19-05's SettingsFromConfig — run green by verifier |
| `internal/session/projector.go` | compaction reset-point class + budget-fill tail + CompactionTailBudget/SetCompactionTailBudget | ✓ VERIFIED | 832 lines; exact pinned seam names landed |
| `internal/session/session.go` | lastInputTokens read model, loop-head check, overflow intercept | ✓ VERIFIED | :975, :514, :548-553 |
| `internal/session/transcript.go` / `manager.go` | Summary field / AppendCompaction(summary) | ✓ VERIFIED | :198 / :390, redacted path |
| `internal/shaper/shaper.go` | gated emission + keep-last-4 | ✓ VERIFIED | :164-187 with the cap rationale at the site |
| `internal/profile/types.go` / `loader.go` / `extract.go` | CacheControl field / system_cache_control application / extractor capture | ✓ VERIFIED | :58 / :57 / systemCacheDeclared + extract-profile writer |
| `internal/provider/streaming.go` / `errors.go` | non-2xx check / IsOverflow | ✓ VERIFIED | :244 / :158 |
| `internal/modelrouting/config.go` + `defaults/config.yaml` | Compaction keys with 80/true floor defaults | ✓ VERIFIED | :34 + :44-46; seed copy synced (drift guard green) |
| `internal/acpserve/config_surface.go` | both menu ids handled, persist-then-apply, live apply | ✓ VERIFIED | pending machinery deleted (grep = 0) |
| `internal/runtime/runtime.go` | construction init + ApplyCompactionSettings relay | ✓ VERIFIED | :1759, :2412-2444, :2674 |

All artifacts: exists + substantive + wired (Level 3) + real data sources (Level 4: profile yaml → loader → shaper → marshaled body; real usage chunks → lastInputTokens; capability table → context limit; layered config → live-apply). No static/hollow data paths found.

### Key Link Verification

| From | To | Via | Status |
|------|----|----|--------|
| profiles/zcode/profile.yaml | shaper.go | system_cache_control → loader flag fan-out → NewCacheControlEphemeralParam | ✓ WIRED |
| shaper emission | paritycli probe | composeCacheProbeInput mirrors the same profile field | ✓ WIRED |
| provider/errors.go IsOverflow | session.go retry branch | intercept before appendError | ✓ WIRED |
| compaction.go compact | manager.go | AppendCompaction(summary) + AppendUsage | ✓ WIRED |
| session.go | projector.go | SetCompactionTailBudget from the same context window | ✓ WIRED |
| config_surface.go | session | SetCompactionHook → runner.ApplyCompactionSettings → SetCompactionSettings (acp_serve.go:331) | ✓ WIRED |
| modelrouting floor config | session construction | effective values + capability-table limit (runtime.go:1759) | ✓ WIRED |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| 19-01 packages full | `go test -race -count=1 ./internal/shaper/ ./internal/profile/ ./internal/paritycli/` | ok 2.4s / 24.0s / 4.8s | ✓ PASS |
| 19-02 package full | `go test -race -count=1 ./internal/provider/` | ok | ✓ PASS |
| 19-03/19-04 package full | `go test -race -count=1 ./internal/session/` | ok 4.0s | ✓ PASS |
| Named battery (10 fns) | `go test -race -count=1 -run 'TestProjector_Compaction…|TestCompaction…|TestCompactNow' -v ./internal/session/` | 10 PASS, 0 FAIL | ✓ PASS |
| 19-05 packages | `go test -race -count=1 ./internal/modelrouting/ ./internal/providerfactory/` + full `./internal/acpserve/` | ok 3.1s / 2.2s / 88.8s | ✓ PASS |
| Baseline untouched | `git diff --name-only 73440bd -- internal/parity/cacheprobe.go` | empty | ✓ PASS |
| Vet, nine touched pkgs | `go vet …` | exit 0 | ✓ PASS |
| runtime package | `go test -race -count=1 ./internal/runtime/` | FAIL `TestAskPark_PromptResponsePrecedesResolution` (11.3s) under verifier's own parallel load → PASS in isolation (3.3s) | ? KNOWN-FLAKE (pre-existing family, reproduced at pre-phase commit 00c9da6 per deferred-items.md — not a phase-19 regression) |

Step 7b note: no runnable probe scripts declared by this phase; `mise ci`'s lint leg is red from the documented pre-existing repo-wide golangci-lint 2.12↔2.13 config drift (deferred-items.md) — vet/build/test legs are the gate and are green.

### Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| PAR-01 | 19-02, 19-03, 19-04, 19-05 | Compaction on context overflow risk: threshold-triggered configurable light-tier summarizer, additive typed marker, durable reset-point class, pair atomicity, retry-once recovery, thinking never rewritten | ✓ SATISFIED (machine-verified; 2 flagged human legs) | Truths 1, 2, 4, 8, 9; retry MECHANISM verified (truth 5) — the retry's producing-turn rescue efficacy and live-LLM coherence are the flagged items |
| PAR-02 | 19-01 | cache_control {"type":"ephemeral"} on every system block via the Shaper | ✓ SATISFIED | Truths 6, 7 |

No orphaned requirements: REQUIREMENTS.md maps exactly PAR-01 and PAR-02 to Phase 19; all five plans declare them (19-01: PAR-02; 19-02..05: PAR-01).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER in any of the 16 phase-modified production files; no stdout writes (fmt.Print/os.Stdout) — ACP frame discipline holds | - | - |

Review findings carried (19-REVIEW.md, none block a pinned SC; all open as engineering debt):
- 🛑 CR-01 — mapped to truth 5 / Human Verification 1 (the one review finding that touches a roadmap criterion).
- ⚠️ WR-01 blob-fill advertisement can show a value the engine ignores (documented deliberate: set_config_option is the lever); ⚠️ WR-02 estimate anchor is async-appended → uncontrolled early-firing drift (never late — inside the pinned conservative direction); ⚠️ WR-03 summarizer input unbounded AND the degrade retries at EVERY loop-head iteration (up to 64 summarizer calls per turn on persistent failure — deviates from D-09's "exactly ONE warning… for this turn" letter on multi-iteration turns; compounds CR-01 on the overflow path); ⚠️ WR-05 marker pre/post refs race the async writer (write-only fields today); ⚠️ WR-06 stale context limit after per-session model switch. ℹ️ IN-01..06 cosmetic.

Documented, pre-authorized deviations (NOT gaps):
- Compacting note degraded to stderr + counter — the landed v1 session/update vocabulary has no status frame and `agent_message_chunk` is barred by the bus-isolation prohibition; the plan pre-authorized exactly this degrade and deferred-items.md records it. Consequence folded into Human Verification 2: compaction is currently invisible in the editor UI.

### Human Verification Required

### 1. Architect decision — producing-turn retry semantics (CR-01 / deferred-items.md carve-out)

**Test:** Decide between (a) accepting next-turn recovery semantics — criterion 3's retry rescues the session (future turns) while the producing turn fails loudly after its single retry — and re-pin the criterion wording, or (b) requiring the producing turn be rescuable via the analyzed same-turn carve-out (a TurnID-matched marker serving as its own turn's reset point when no pre-user marker exists; subagent-safe by the TurnID key), with the content-sensitive regression test 19-REVIEW specifies (a fake that rejects requests over a byte bound, which the current content-blind fake cannot do).
**Expected:** A recorded decision. Under (a) this phase closes as-is; under (b) a follow-up plan lands the carve-out plus the regression test.
**Why human:** Two test-pinned contracts conflict — 19-03's position rule (markers reset turns that START after them, enforced by `TestProjector_CompactionResetPoint`) versus SC-3's "instead of failing the turn" letter. The verifier confirmed from code that the conflict is real, not narrative: `projector.go:314` (`i < userIdx`) makes the producing turn's rescue structurally impossible, and the passing "recovery" test cannot detect a byte-identical resend. Choosing which contract gives is an architecture decision the 19-04 executor correctly declined to take unilaterally (Rule 4). WR-03's per-iteration degrade re-fire rides the same decision (a per-turn compaction-attempt cap fixes both).

### 2. Operator live-LLM compaction check (SC-1's user-observable leg)

**Test:** Run a real long session past the threshold (or push `compaction-threshold` low via `set_config_option` — the live-apply path is proven), let it compact, keep working across several more turns.
**Expected:** Earlier-turn facts survive in the summary seed; the conversation stays coherent where v1.1 silently lost turns; subsequent turns stay under the threshold (headroom). Note: no visible compaction indicator will appear (the degraded note) — if the operator wants one, that is a Phase-20-era wire/UX decision, not a bug.
**Why human:** Live-model summary fidelity is the research validation table's operator-gated leg; the offline battery proves the machinery with a scripted summarizer, which says nothing about coherence.

### Gaps Summary

No failed truths. All artifacts exist, are substantive, are wired, and flow real data. All thirteen plan prohibitions hold with concrete evidence. PAR-02 is fully closed (probe green, baseline untouched, WINDOWS #5 fixed). PAR-01's machinery — threshold engine, additive marker, durable reset-point class, pair-safe budget tail, config keys with live apply, overflow retry mechanism — is machine-verified by named tests the verifier ran itself. The phase's one genuine unresolved tension (SC-3's producing-turn rescue, CR-01) is present, wired, and test-pinned at the mechanism level but structurally cannot deliver the criterion's recovery outcome for the producing turn under 19-03's pinned position rule; it was deliberately deferred for architect sign-off and is routed to Human Verification 1 rather than silently passed or failed. The runtime-package failure observed during verification is the documented pre-existing load-sensitive flake family (passes in isolation, reproduced at the pre-phase commit), not a phase-19 regression. Machine verification is complete and green; awaiting the architect/operator legs above.

---

_Verified: 2026-09-06T23:45:00Z_
_Verifier: ZCode (gsd-verifier)_
