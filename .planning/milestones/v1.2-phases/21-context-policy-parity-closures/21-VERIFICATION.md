---
phase: 21-context-policy-parity-closures
verified: 2026-09-06T20:05:00Z
status: passed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
human_verification: [] # all four items closed by operator UAT 2026-09-06 — see "Human Verification Closure"
---

# Phase 21: Context & Policy Parity Closures Verification Report

**Phase Goal:** The content-path parity closures land as one coherent wave sharing two vehicles already built: hooks PreToolUse deny joins Phase 17's single gate pipeline (deny-only authority from project scope), AGENTS.md/CLAUDE.md auto-inject via the profile-copy merge, thinking streams end-to-end byte-identical, and rich prompt content (images, @-mentions) enters with ingress validation.
**Verified:** 2026-09-06T20:05:00Z (canonicalized — machine verification 2026-09-03, human items closed 2026-09-06)
**Status:** passed
**Re-verification:** Human legs closed 2026-09-06 via operator UAT (incl. the corrected-dialect hook-deny retest)

## Goal Achievement

### Observable Truths

Merged must-haves: the 5 ROADMAP success criteria (the contract) carry the six plans' truths (21-01: 7, 21-02: 6, 21-03: 6, 21-04: 4, 21-05: 4, 21-06: 6). Every plan truth was checked against its named test/artifact; the table below reports at SC granularity with the plan-truth evidence folded in.

| # | Truth | Status | Evidence |
| - | ----- | ------ | -------- |
| 1 | SC1 (PAR-03): settings.json hook deny blocks at the chokepoint — bounded sync execution, hard timeout, fail-open, structured verdicts, repo-shipped files NEVER grant allow | ✓ VERIFIED | Locked-contract greps: exactly 2 `gateCall` sites (internal/session/session.go:530 batch branch, :601 subagent branch — both carry the "hook-verdict head runs FIRST" comment), 1 `PreToolUseVerdict` consumption (internal/session/gate.go:311 `gateHookVerdict`), 0 `hooks.PreToolUse(` in internal/coreexec (executor leg deleted, `errHookRefused` gone). Resolver: internal/ecosys/hookverdict.go — exit-2 checked FIRST, JSON schema gate, deny-wins with FIRST-deny reason, non-user allows demoted loud (D-01). Batteries PASS under `-race`: `TestSettingsHooks*` (6 incl. WR-08 local scope), `TestHookMatcherDialect`, `TestHookVerdictParse/Resolve`, `TestHookScopeOrder`, `TestPreToolUseVerdict`, `TestGateHookVerdict` (both branches), `TestHookJoin_ProjectDenyThroughComposition`, `TestHooksOnceOnly`, `TestRegisterCorePreToolUseDisposal` |
| 2 | SC2 (PAR-03): hook verdicts flow through the SAME pipeline as permission asks, documented precedence, no second gate | ✓ VERIFIED | Precedence verbatim in gateCall's doc comment + package doc: "hook verdict → permission ask → execute". `rules.Evaluate` in internal/session non-test sources exists ONLY in gate.go:220 (no second gate). Head mapping is total (deny/ask/allow/none each one target; NO-DECISION the only fall-through); WR-01 fix verified in code: hook ask funnels through Step 3 (D-07 automation decline) + Step 5 (degraded-client guard) while skipping the Step 4 mode check (suspends even ungated, D-04) — pinned by `TestGateHookAskFailSafes` (PASS) |
| 3 | SC3 (PAR-04): fresh session in a memory-bearing repo shows AGENTS.md/CLAUDE.md content automatically, mtime-cached, edits picked up | ✓ VERIFIED | internal/ecosys/memory.go: `DiscoverMemoryFiles` (D-05 collision, D-06 git-root stop incl. `.git` file, D-07 user-global precedence), `MemoryInjection` (24 KB/file rune-safe cap, 64 KB budget, notes), path+mtime+size cache. Exactly 1 `MemoryInjection` call site (internal/runtime/runtime.go:1547, fourth trailing-TextBlock merge after AgentListing). Batteries PASS under `-race`: 12 `TestMemoryDiscovery_*` rows + 4 `TestMemoryInjection_*` wiring rows (block-last, empty-adds-nothing, shared-profile-untouched, truncation-flows). Live-Zed operator half → Human Verification |
| 4 | SC4 (PAR-05): thinking renders live as thought chunks AND round-trips byte/field-value identical incl. the Anthropic signature | ✓ VERIFIED | Chain wired end-to-end: SSE accumulator (internal/provider/streaming.go, `chunkTypeThinking`, redacted emits immediately) → `streamAndEmit` case → `AppendRawThinking` production caller (internal/session/session.go:873, verbatim RawMessage, WR-05 error logged) → `AgentThoughtChunk` bus event → both forwarder subscribe sites + `routeBusEvent` case + `forwardThoughtChunk` (internal/runtime/runtime.go:772/954/963) → projector fold (`TypeRawThinking`, WR-04 stash-capture fix visible) → shaper both SDK param types. Batteries PASS under `-race`: 6 `TestStreamThinking_*`, 3 `TestStreamAndEmit_Thinking*` (zero-redactor pin PASS, verbose), `TestThinkingGolden` (field-value identity every hop + D-13 replay pin at hop 2), 4 `TestProjector_Thinking*` incl. `TestProjector_ThinkingNeverDuplicatedAcrossBatchAndText` (WR-04), 2 `TestShaperThinking_*`, `TestThoughtForward`. Golden fixture carries the loud corpus_absent provenance header (WINDOWS #17 — known non-gap). Live Zed rendering → Human Verification |
| 5 | SC5 (PAR-06): image + @-mention produce corresponding content blocks in the outgoing request, Read-rule gated with provenance, ingress validated per provider shape, loud degrade | ✓ VERIFIED | Mentions: `ParseMentions` (internal/ecosys/mentions.go) + runtime expansion in `expandUserBlocks` via `readRuleEval` (workspace-root bound, admission-before-stat, fixed-form notes) + `TypeMentionProvenance`/`AppendMentionProvenance`; 13 `TestMentionExpand_*` + `TestParseMentions` PASS. Images: internal/runtime/imgscale.go `ValidateAndScaleImage` (DecodeConfig FIRST, `draw.CatmullRom`, 100 Mpx decode-safety ceiling) + sha-keyed atomic persistence + Ref-form transcript lines (`json:"-"` Data carrier) + `Provider.SupportsImages()` (anthropic true / openai false) + shaper `imageBlockParam` + projector seed fold; 5 `TestValidateAndScale_*` (pixel-bomb refused pre-decode), 6 `TestImageIngress_*` (lean no-base64, idempotent, WR-07 text-bearing), 4 `TestImageCapability_*` (D-11 one-note drop, image reaches outgoing request via real Anthropic adapter) PASS. WR-02/WR-03 pins `TestHookJoin_MentionSeamRaceFree` + `TestHookJoin_OneLiveRuleAuthority` PASS (Runner-scoped `permStore atomic.Pointer`, one rule authority). Build contracts: `CGO_ENABLED=0 go build ./...` OK, `golang.org/x/image v0.45.0` direct, `go mod tidy -diff` clean. Editor paste round-trip → Human Verification |

**Score:** 5/5 truths verified (0 present, behavior-unverified — every behavior-dependent invariant has a passing behavioral test; the four live-editor legs are external-service checks routed to humans, not unexercised code)

### Prohibitions

| Prohibition (plan) | Tier | Status | Evidence |
| ------------------ | ---- | ------ | -------- |
| 21-01 PAR-03 safety: silent/schema-invalid/project-scope verdicts never honored as allow | judgment | ✓ NOT-VIOLATED | `parseHookVerdict` (exit-2 first, `{`-gate, schema-valid only) + `ResolveVerdict` demotes every non-user allow with a loud warning; pinned by TestHookVerdictParse/Resolve and the PreToolUseVerdict rows |
| 21-03 PAR-05 safety: thinking/signature bytes never parsed-and-reserialized, edited, reordered, or filtered | judgment | ✓ NOT-VIOLATED | RawMessage verbatim into the transcript; field values extracted only at the projector; shaper maps BOTH block types in original order; goldens + zero-redactor pin |
| 21-04 PAR-06 privacy: ingress is no filesystem-existence oracle | judgment | ✓ NOT-VIOLATED | Admission-before-stat; fixed-form notes name token + outcome class only; `TestMentionExpand_OutsideAdmittedRoots`/`_UnresolvableNote` PASS |
| 21-05 PAR-06 privacy: image bytes never logged/inlined in the transcript | judgment | ✓ NOT-VIOLATED | `json:"-"` Data carrier makes the leak type-impossible; `TestImageIngress_TranscriptLeanNoBase64` PASS |
| 21-06 PAR-03 safety: no second PreToolUse consumption path alive | judgment | ✓ NOT-VIOLATED | Grep audit: 0 consultation sites in coreexec; `HookRunner.PreToolUse` + `errHookRefused` deleted; `TestHooksOnceOnly` pins the once-only side effect |

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/ecosys/hookverdict.go` | parseHookVerdict + ResolveVerdict (pure) | ✓ VERIFIED | 199 lines; D-01..D-04 documented; no exec/context in the resolver |
| `internal/ecosys/hooks.go` | Scope tagging, two-path matcher, scope partition, PreToolUseVerdict | ✓ VERIFIED | `scopeRank` + `sort.SliceStable`; sequential run loop; `PreToolUse` deleted at the join |
| `internal/ecosys/loader.go` | loadSettingsHooks (project+user) + settings.local.json (WR-08) | ✓ VERIFIED | `parseSettingsHooks` shared; degrade-softly discipline |
| `internal/ecosys/memory.go` | walker + cache + capped injection | ✓ VERIFIED | All D-05..D-08 symbols present |
| `internal/ecosys/mentions.go` | ParseMentions (parse-only) | ✓ VERIFIED | No I/O, no resolution |
| `internal/runtime/imgscale.go` | ValidateAndScaleImage (DecodeConfig-first, CatmullRom) | ✓ VERIFIED | Limits + typed ImageError + atomic persistence |
| `internal/session/gate.go` | live hook-verdict head + precedence doc | ✓ VERIFIED | gateHookVerdict total mapping; WR-01 funnel |
| `internal/coreexec/register.go` | withHooks reduced to PostToolUse | ✓ VERIFIED | Disposal recorded in seam comments |
| `internal/session/testdata/thinking-golden/sse-thinking.jsonl` | fixtures with provenance | ✓ VERIFIED | corpus_absent header (LOUD, WINDOWS #17) |
| All other key-files (21-02..21-05: session.go, projector.go, shaper.go, events.go, streaming.go, provider files, transcript.go, manager.go, runtime.go) | as declared | ✓ VERIFIED | Symbols + wiring grepped; every file substantive |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| loader.go loadAll | hooks registry | Scope-tagged merge after plugin hooks | ✓ WIRED | runtime.go:124-126 (3 append sites) |
| gate.go gateCall head | ecosys PreToolUseVerdict | one delegation + total mapping | ✓ WIRED | gate.go:311; runtime.go:1745 injects `hookRunner.PreToolUseVerdict` |
| runtime sessionFor | perm rule set | `permStore` hoisted to Runner, one authority | ✓ WIRED | runtime.go:1781-1799; WR-02/WR-03 pins |
| memory.go MemoryInjection | sessionFor | fourth trailing-TextBlock merge | ✓ WIRED | Exactly 1 call site (runtime.go:1547) |
| streaming.go thinking | session.go streamAndEmit | AppendRawThinking + AgentThoughtChunk | ✓ WIRED | session.go:873/:880; forwarder :772/:954 |
| mentions.go ParseMentions | expandUserBlocks | runtime expansion + readRuleEval | ✓ WIRED | runtime.go:567/:622-638 |
| acp image blocks | outgoing request | ingress → Ref → shaper → projector seed fold | ✓ WIRED | TestImageCapability_ImageReachesOutgoingRequest PASS (real adapter) |
| Provider interface | SupportsImages | compile-level completeness | ✓ WIRED | anthropic true / openai false; conformance + all fakes updated |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Hook + memory + mention substrate | `go test -race -count=1 ./internal/ecosys/ -run 'TestSettings\|TestHookVerdict\|TestHookScopeOrder\|TestPreToolUse\|TestParseMentions\|TestMemoryDiscovery'` | ok 3.059s | ✓ PASS |
| Gate join + thinking + projector | `go test -race -count=1 ./internal/session/ -run 'TestGateHook\|TestThinking\|TestProjector_Thinking\|TestContentBlockRoundTrip\|TestProjectorToleratesNewKinds\|TestGatePermissionSuspend'` | ok 2.831s | ✓ PASS |
| Zero-redactor thinking pin (verbose) | `go test -race -count=1 ./internal/session/ -run 'TestStreamAndEmit_Thinking' -v` | 3/3 --- PASS | ✓ PASS |
| Wiring batteries (memory/mention/join/image) | `go test -race -count=1 ./internal/runtime/ -run 'TestMemoryInjection\|TestMentionExpand\|TestHookJoin\|TestImageCapability\|TestImageIngress\|TestValidateAndScale\|TestThoughtForward'` | ok 9.969s | ✓ PASS |
| Disposal + shaper + provider + event | `go test -race -count=1 ./internal/coreexec/ ./internal/shaper/ ./internal/provider/ ./internal/event/ -run '...'` | all 4 ok | ✓ PASS |
| D-09 build contract | `CGO_ENABLED=0 go build ./...` + `go list -m golang.org/x/image` | OK / v0.45.0 | ✓ PASS |
| Module hygiene (WR-06) | `go mod tidy -diff` | empty | ✓ PASS |
| Single-consumption grep audit | gateCall/PreToolUseVerdict/rules.Evaluate/hooks.PreToolUse greps | 2 sites / 1 head / gate.go-only / 0 in coreexec | ✓ PASS |

### Probe Execution

Not applicable — phase declares no `scripts/*/tests/probe-*.sh`; verification rides the Go test batteries above.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| PAR-03 | 21-01, 21-06 | Hooks: settings.json parsing (project+user), PreToolUse deny at the chokepoint, fail-open, deny-only from project scope, joins the ONE gate | ✓ SATISFIED | Truths 1-2 |
| PAR-04 | 21-02 | AGENTS.md/CLAUDE.md auto-injected every session, mtime-cached | ✓ SATISFIED | Truth 3 |
| PAR-05 | 21-03 | Thinking streamed to client, raw passthrough end-to-end, signatures round-trip | ✓ SATISFIED | Truth 4 |
| PAR-06 | 21-04, 21-05 | Rich prompt content: images + @-mentions with Read-rule gating and provenance, per-provider ingress validation | ✓ SATISFIED | Truth 5 |

No orphaned requirements: REQUIREMENTS.md maps exactly PAR-03..PAR-06 to Phase 21; all four are claimed by plans and satisfied. REQUIREMENTS.md checkboxes for PAR-03..06 are `[x]` (consistent with the evidence).

### Anti-Patterns Found

None. Debt-marker scan (TBD/FIXME/XXX) across all 19 phase production files: zero hits. The only "placeholder" text match is `internal/runtime/runtime.go:1986` — `[image content could not be delivered: <class>]` — which IS the WR-07 fix's intentional in-band degrade note (real functionality, pinned by two RED-first tests), not a stub. No empty-return/console.log-only implementations in the new files.

### Review Fix Verification (b170a06 range)

All 8 warnings verified fixed at the code level, each with its named RED-first pin test present and passing: WR-01 (`TestGateHookAskFailSafes`, gateAskHook funnel visible in gate.go Steps 3/5), WR-02/WR-03 (`permStore atomic.Pointer` + `TestHookJoin_MentionSeamRaceFree`/`_OneLiveRuleAuthority`), WR-04 (`flushBatch` return + `TestProjector_ThinkingNeverDuplicatedAcrossBatchAndText`), WR-05 (session.go:873 `tErr` slog.Warn), WR-06 (tidy -diff clean), WR-07 (`ensureTextBearingBlock` + 2 pins), WR-08 (`loadSettingsLocalHooks` + `TestSettingsHooksLocalProjectScope`).

### Human Verification Required

Four live-editor legs (detailed in frontmatter `human_verification`):

1. **Live-Zed AGENTS.md auto-injection** — fresh session in a memory-bearing repo shows the content automatically; edits picked up on change. The wiring is proven by `memory_wiring_test.go`; the operator-visible half of criterion 3 needs a real editor.
2. **Thinking stream rendering in Zed** — thought chunks render live during a turn. Frame emission proven by `TestThoughtForward`; UI rendering is unobservable to tests.
3. **@-mention / image paste round-trip in the editor** — outgoing request carries the expanded blocks; transcript stays lean. Code path proven by the mention/image batteries; the clipboard/paste UX needs a human.
4. **Live hook-deny demonstration** — a deny hook in `.claude/settings.json` blocks a tool call with the reason surfaced. Gate path proven by `TestGateHookVerdict` + composition test; the live editor demo needs a human.

### Gaps Summary

No gaps. All five roadmap success criteria are verified in the codebase with behavioral test evidence; every prohibition holds with code + test proof; the locked-contract greps (2 gateCall sites, 1 verdict consumption, no second gate, zero executor consultation) all pass; build contracts (CGO_ENABLED=0, x/image v0.45.0, tidy clean) hold. Known non-gaps honored per the execution brief: `mise ci` lint drift (golangci go1.26/go1.27, WINDOWS ledger) and corpus_absent thinking goldens (WINDOWS #17, loud synthetic provenance header, field-value identity fully test-proven). The status is `human_needed` solely for the four live-editor legs above — automated checks passed on every touched package.

## Human Verification Closure (2026-09-06)

All four human legs closed by operator UAT on 2026-09-06 (recorded in `21-UAT.md`, status: complete — 4/4 passed, 0 issues):

1. **AGENTS.md/CLAUDE.md auto-injection, live editor** — PASS (UAT test 1, Б1): rule from AGENTS.md followed unprompted in a fresh session; edits picked up on the next session (mtime re-read).
2. **Thinking stream rendering in Zed** — PASS (UAT test 2, Б2): thought blocks render live during turns.
3. **Image paste + @-mention round-trip** — PASS (UAT test 3, Б3/Б4): @-mention expanded and read (provenance on disk); image block carried with the Anthropic adapter (SupportsImages=true) and the text-only glm-5.2 scratch config produced the designed D-11 loud model-side outcome — pipeline correct; vision understanding is an operator config choice, not a defect.
4. **Live hook-deny demonstration** — PASS (UAT test 4, retest): the first attempt (Б5) was inconclusive — the test instruction itself used the wrong output dialect (`{"decision":"block"}` is schema-invalid → VerdictNone → fail-open BY DESIGN). Retest with the corrected fixture (~/tmp/hook-deny-uat: PreToolUse matcher Bash, hookSpecificOutput permissionDecision deny) blocked the Bash call with the hook's reason in a live session. The project-scope-allow-never-widens-trust half is machine-proven (TestHookJoin_ProjectDenyThroughComposition, TestGateHookVerdict).

---

_Verified: 2026-09-06T20:05:00Z (canonicalized)_
_Machine verification: 2026-09-03T23:40:26Z — ZCode (gsd-verifier)_
_Human legs closed: 2026-09-06 — operator UAT, see 21-UAT.md_
