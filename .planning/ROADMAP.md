# Roadmap: ass-guard-agent (working name)

**Project mode:** mvp (vertical slices — each phase delivers an end-to-end user capability)
**Milestone:** v1.1 Kickoff & Peers (Phases 8–11; numbering continues from v1.0's Phase 7 — never reset)
**Requirements mapped:** 23/23 v1.1 ✓

v1.1 makes the hands-off OpenSpec promise real end-to-end. The milestone's reason to exist is Phase 8: closing the kickoff loop (`/opsx:explore → propose → apply → archive` chained by the engine with zero manual continues, verified against the real openspec binary) is the product's proof — v1.0 shipped with this as its major known gap. Phases 9–11 then close the v1.0 operational gaps (audit on `acp serve`, parity re-capture) and add the two peer surfaces (Telegram, dsh profile #2), strictly in the operator's priority chain. Scope discipline: two new deps only (`go-telegram/bot` v1.23.0, `klauspost/compress/zstd` v1.19.2), one structural refactor (`internal/runtime` extraction, Phase 10), and the generalized v1.0 lesson as a cross-phase invariant — **no feature closes with stub-only evidence; every external surface carries a real-binary/live-service gate**.

## Milestones

- ✅ **v1.0 MVP** — Phases 0–7 (shipped 2026-08-14; full detail: `.planning/milestones/v1.0-ROADMAP.md`, artifacts in `.planning/milestones/v1.0-phases/`, record in `.planning/MILESTONES.md`)
- 🚧 **v1.1 Kickoff & Peers** — Phases 8–11 (1/4 complete)

<details>
<summary>✅ v1.0 MVP (Phases 0–7) — SHIPPED 2026-08-14</summary>

- [x] Phase 0: Spike + Re-verification (5/5 plans) — completed 2026-08-09
- [x] Phase 1: Mimicry MVP (north-star proof) (6/6 plans) — completed 2026-08-13
- [x] Phase 2: Session Core + ACP Interface (7/7 plans) — completed 2026-08-11
- [x] Phase 3: Model Scheduling (4/4 plans) — completed 2026-08-11
- [x] Phase 4: Unified Engine + Hook-DAG + OpenSpec + Learning (7/7 plans) — completed 2026-08-12
- [x] Phase 5: Ecosystem Compatibility (3/3 plans) — completed 2026-08-13
- [x] Phase 6: Distribution + Polish (2/2 plans) — completed 2026-08-13
- [x] Phase 7: Multi-Provider Config & Credentials (2/2 plans) — completed 2026-08-14

</details>

## Phases

- [x] **Phase 8: Slash-Command Kickoff** - `/namespace:name` discovery + zcode-semantics expansion + OpenSpec adapter reconciled to the real binary; the zero-continue product proof (completed 2026-08-16, operator witness accepted; execution + evidence 2026-08-15)
- [ ] **Phase 9: Serve-Path Audit + zcode Parity Re-capture** - redacted, bounded, decision-explaining audit on `acp serve`; stability test re-grounded on a pinned capture session
- [ ] **Phase 10: Telegram Peer (Text + Voice)** - the same engine drivable from a Telegram chat, voice STT, context-first shutdown beside ACP stdio
- [ ] **Phase 11: dsh Mimicry Profile #2** - DeepSeek-model turns structurally indistinguishable from deepseek-harness; N profiles, no target-specific code paths

## Phase Overview

| # | Phase | Goal | Requirements | Success Criteria |
|---|-------|------|--------------|------------------|
| 8 | Slash-Command Kickoff | 8/9 | Completed 2026-08-16 — operator witness accepted; findings 5+6 dispositions confirmed (hybrid provenance chaining + D-10 capture-faithful reshape); full guard green; gated E2E: fixable green + zero-continue 2/3 (one stage-4 model-variance fail recorded as known residual, same class as UAT check 3); UAT 10 pass/1 partial/0 blocked | 2026-08-16 |
| 9 | Serve-Path Audit + zcode Parity Re-capture | Every serve-path session leaves a redacted, bounded audit trail that explains the engine's decisions; the parity stability test runs green on a newly pinned session | AUD-01, AUD-02, AUD-03, AUD-04, AUD-05 | 5 |
| 10 | Telegram Peer (Text + Voice) | A user drives the same engine from a Telegram chat — full SDD scenarios, voice input, disciplined shutdown | TG-01, TG-02, TG-03, TG-04, TG-05, TG-06 | 5 |
| 11 | dsh Mimicry Profile #2 | DeepSeek-model turns are structurally indistinguishable from deepseek-harness, captured not hand-written | DSH-01, DSH-02, DSH-03, DSH-04, DSH-05 | 5 |

## Phase Details

### Phase 8: Slash-Command Kickoff

**Goal:** As a developer practicing SDD, I want to kick off and drive an entire OpenSpec change (`/opsx:explore → propose → apply → archive`) from ass-guard by typing the toolkit's own slash-commands, so that the unmodified workflow runs hands-off — the milestone's product proof and the closure of v1.0's major known gap.
**Mode:** mvp
**Depends on:** Nothing (first v1.1 phase; builds on shipped v1.0 — `internal/ecosys`, `internal/openspec`, `internal/session`, the engine). Carries the milestone's only structural blocker as its **first task**: `internal/ecosys.discoverCommands` scans `commands/*.md` flat and skips directories, so the `openspec init --tools claude` layout (`.claude/commands/opsx/*.md`) is invisible today — without fixing this first, `/opsx:*` cannot work at all. Expansion hooks at the turn runner (session layer), NOT the ACP handler — that surface-agnosticity is why Telegram later gets `/opsx:*` for free.
**Requirements:** CMD-01, CMD-02, CMD-03, CMD-04, CMD-05, CMD-06, CMD-07
**Success Criteria** (what must be TRUE):

  1. A user invoking `/opsx:explore` has the command discovered: `commands/<ns>/<name>.md` layouts (like `openspec init --tools claude` installs) are found via one-level subdirectory scan with colon-joined keys, under the existing precedence (project `.claude/` > user `.claude/` > `.ass-guard/`), proven by a real-fixture test generated from actual `openspec init` output (CMD-01)
  2. A user typing `/namespace:name args` gets the command's markdown body expanded with exact zcode substitution semantics — `$ARGUMENTS` and `$1..$N` (out-of-range → empty), args without placeholders appended under a "User arguments:" heading, `${ARGUMENTS}` brace form and `` !`cmd` `` dynamic shell NOT recognized — fed to the turn as the user message, with the contract pinned by a table-driven edge-case test written before implementation; unknown `/foo` falls through as plain text (CMD-02)
  3. The model invoking any `openspec:*` tool gets a real executed subprocess result — Adapter-backed `Execute` on registered tools, command surface pinned to the installed binary's probe (phantom `apply`/`implement` removed; read-only vs mutating classified), non-interactive guards (nil stdin, per-command timeout, exit-code classification) (CMD-03)
  4. A developer runs a real `/opsx:explore → propose → apply → archive` OpenSpec scenario end-to-end through ass-guard in a scratch project with zero manual continues — happy / fixable-failure / missing-binary paths — and the 11 deferred v1.0 Phase-4 UAT checks pass (CMD-04)
  5. Expanded turns record provenance (which command file drove the turn), engine pattern-matching remains assistant-role-only (regression test — the prompt-injection guard for repo-shipped command markdown), and same-key shadowing across discovery scopes emits a warning (CMD-05)
  6. Skills work claude-code-compatibly — the model invokes the `Skill` tool; skill name + description reach context via the captured profile's shape with dynamically discovered skills merged in (the v1.0 dynamic-MCP pattern); SKILL.md loads into the turn; all discovered skills exposed — `/opsx` prompts naturally trigger the matching `openspec-*` skills (CMD-06, added at Phase-8 discussion)
  7. WebSearch ships a real DDG-HTML default backend (zero key) on the swappable seam and WebFetch returns fetch + html→markdown — at minimum sufficient for `/opsx:explore` workflows; zero-config first run unaffected (CMD-07, added at Phase-8 discussion)

**Phase gate:** `mise ci` clean AND the operator-gated real-binary test (`ASSGUARD_OPENSPEC_BIN=1` against real openspec v1.5.0, all three paths) AND a real `/opsx` E2E in a scratch project AND the 11 deferred UAT checks green. No stub-only evidence closes this phase.
**Plans:** 9/9 plans complete (verified 2026-08-16)
Plans:
**Wave 1**

- [x] 08-01-PLAN.md — ecosys namespaced discovery: one-level `commands/<ns>/<name>.md` scan with colon-join keys, zcode name/frontmatter rules, flat-parser fallback, shadow warnings (THE structural blocker, first) [CMD-01, CMD-05]
- [x] 08-02-PLAN.md — real web-tool backends: DDG-HTML WebSearch default (zero key) + WebFetch fetch+html→markdown on the swappable seam [CMD-07]
- [x] 08-03-PLAN.md — openspec adapter reconciliation: probe-pinned surface (phantom apply/implement removed), Adapter-backed Execute closures, non-interactive guards + three-path real-binary gate [CMD-03]

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 08-04-PLAN.md — Expand engine + turn wiring: zcode substitution contract (test-first), expansion at the turn runner on all paths + injections, transcript provenance, D-11 command boundaries, injection regression [CMD-02, CMD-05]

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 08-05-PLAN.md — skills invocation: model-invoked Skill tool with SKILL.md loading into the turn, captured-shape listing dynamic merge, all skills exposed [CMD-06]

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 08-06-PLAN.md — the gate: real /opsx E2E in a scratch project, pattern re-seed from real output, 11 UAT checks, operator witness [CMD-04] — **CHECKPOINT RESOLVED — COMPLETE**: the three stacked gaps (within-turn carry → core tool execution → the D-11 boundary reset) closed via 08-07/08-08/08-09; T3 + the gated E2E green at the witness; phase accepted 2026-08-16.

**Wave 5** *(blocked on Wave 4 completion — gap closure for the 08-06 blocking finding)*

- [x] 08-07-PLAN.md — gap closure: within-turn tool-result carry — tool-call ids through the provider seam (`shaper.ToolCall{ID,Name,Input}`), structured mid-turn message shapes (assistant tool_use / tool-role results, capture-grounded), Projector within-turn accumulation (boundary-safe, pair-safe, capture-bounded), parity/fidelity/stability guard, operator-witnessed gated E2E re-run proving convergence, handback to 08-06 T3 [CMD-04] — COMPLETE 2026-08-15 (carry proven live); the E2E surfaced the SECOND blocking gap (core tool execution never implemented — STATE.md Blockers).

**Wave 6** *(blocked on Wave 5 completion — gap closure for the core-tool-execution blocker)*

- [x] 08-08-PLAN.md — gap closure: capture-grounded core tool execution for the /opsx working set — real Bash (workdir, model ms timeouts, process-group kill, captured `Exit code <N>`/sentinel forms), Read/Write/Edit/TodoWrite/TodoRead executors in captured result forms (line-numbered Reads, captured success texts, camelCase todo echo), committed result-shape fixture + plain-text rendering rule in the Projector, single RegisterCore wiring site, operator-witnessed gated E2E re-run proving ≥1 full stage of real work, handback to 08-06 T3 [CMD-04] — COMPLETE 2026-08-15 (core-tool gap closed live: zero `no implementation yet`, real archive artifacts); the E2E surfaced the THIRD blocking gap (the D-11 per-mutating-call boundary reset makes turns non-convergent — STATE.md Blockers).

**Wave 7** *(blocked on Wave 6 completion — gap closure for the D-11 boundary-reset finding)*

- [x] 08-09-PLAN.md — gap closure: boundary resets move BETWEEN turns — the Projector's reset point becomes the last boundary recorded BEFORE the projected turn's user message, mid-turn accumulation survives to the turn's end (rolling 64-tail + pair-safety, capture-pinned: 46/46 tail records, 579 same-turn persistences, zero tool-result resets), boundary writers + D-11 expansion-time semantics + between-turn lean reset + the no-confirmation-tier safety model all preserved, SESS-02-era tests re-pinned (both halves) not deleted, STATE.md revision record, operator-witnessed gated E2E re-run proving convergence + ≥1 full stage of real work, handback to 08-06 T3 [CMD-04] — EXECUTED 2026-08-15 (SESS-04 revision recorded in STATE.md under the 08-09 execution authority; handed back to 08-06 T3; witness-accepted 2026-08-16).

### Phase 9: Serve-Path Audit + zcode Parity Re-capture

**Goal:** As an operator of a hands-off agent, I want every `acp serve` session to leave a redacted, bounded audit trail that also records the engine's decisions, and the zcode parity stability test re-grounded on a newly pinned capture session, so that I can answer "what did the agent do, and why did it continue" from the log alone and trust that the mimicry hasn't drifted.
**Mode:** mvp
**Depends on:** Phase 8 (operator priority chain; also the engine decisions recorded by AUD-04 observe Phase-8-hardened turns). AUD (audit) and AUD-05 (re-capture) are adjacent in the operator's chain and merged here by design: both are small, both land before Phase 10's `internal/runtime` extraction moves the wiring — the capturer factory seam reaches its final home here, written once, not re-wired. AUD-05 additionally unblocks the v1.0 Phase-1 within-session stability test (`eea3dc48` is absent on disk).
**Requirements:** AUD-01, AUD-02, AUD-03, AUD-04, AUD-05
**Success Criteria** (what must be TRUE):

  1. A provider request made during `acp serve` is captured through a single-sourced factory-seam capturer covering both protocol shapes — the existing tracer path refactored onto the same seam, with no divergent provider-construction copies (the copy-`tracerProvider` shortcut is banned) (AUD-01)
  2. Every serve-path session writes a redacted audit trail — per-session transcript events with correlation IDs (session/turn/request) including `RequestShaped`, plus an optional `--audit-log` mirror (0600, append-only JSONL, rejects stdout as a target) (AUD-02)
  3. Audit volume is bounded via the body_ref pattern (hash in the event, full body in a capped store), and audit write failures are loud but never fatal to a turn (AUD-03)
  4. The audit trail records engine decisions (continue / hook / ask / wait, with the matched signal) — the "why did the agent continue" question is answerable from the log alone (AUD-04)
  5. The zcode parity stability test runs green against a newly pinned divergence-prone capture session produced via the operator runbook (scripted subagent/MCP-attach/tool-variety workload — not richest-session selection), with the pinned session ID consumed by the test, zcode + extractor versions recorded, thresholds explicitly re-baselined, and the drift report committed before any profile update (AUD-05)

**Phase gate:** `mise ci` clean AND live-serve redacted-audit verification (a redacted `RequestShaped` line observed on a real serve) AND the stability test green against the newly pinned session AND token/secret canary greps clean. No stub-only evidence closes this phase.
**Plans:** 6 plans
Plans:
**Wave 1**

- [ ] 09-01-PLAN.md — factory capturer seam: `BuildWithCapturer` both shapes (anthropic + openai), tracer refactored onto the seam (`tracerProvider` deleted), serve-path RequestShaped + CurrentTurnID + per-session TranscriptWriter, redacted-line integration test [AUD-01, AUD-02]
- [ ] 09-02-PLAN.md — engine-decision full provenance: matched text span + config source threaded PatternTable → Decide → EngineDecision event + engine_decision transcript line (D-03) [AUD-04]
- [ ] 09-03-PLAN.md — re-capture machinery: stability test consumes the pinned session (never PickRichestMain), divergence canary fixture, zcode_version provenance, the operator runbook in-repo (verbatim from Pitfalls 17/18) [AUD-05]

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 09-04-PLAN.md — the re-capture run: operator scripted divergence-prone workload (checkpoint), threshold verification, drift report committed BEFORE the profile update, re-extract + re-pin, stability green, parity re-baseline (ZAI_API_KEY-gated leg) [AUD-05]
- [ ] 09-05-PLAN.md — bounded audit volume: the body_ref pattern — metadata-only request events (correlation triple + shape fingerprint), capped redacted body store retrievable by hash (D-01) [AUD-02, AUD-03]

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 09-06-PLAN.md — the per-session audit mirror under `.ass-guard/audit/` (default ON), `--audit-log` override on serve, shared stdout-rejecting sink opener, header-NAME discipline, automated secret canary (D-02) [AUD-02]

### Phase 10: Telegram Peer (Text + Voice)

**Goal:** As a developer away from my IDE, I want to drive ass-guard from a Telegram chat — full SDD scenarios over text and voice, on the same engine ACP uses — so that the agent keeps working wherever I am as a peer surface, not a stripped-down remote.
**Mode:** mvp
**Depends on:** Phase 8 (session-layer command expansion — Telegram gets `/opsx:*` for free) and Phase 9 (capturer seam landed in its final home before this phase's `internal/runtime` extraction moves the code). This phase performs the milestone's one structural refactor: extracting the turn core (runner + engine/hook/MCP wiring) from `cmd/ass-guard/acp_serve.go` into `internal/runtime` as the Telegram prerequisite.
**Requirements:** TG-01, TG-02, TG-03, TG-04, TG-05, TG-06
**Success Criteria** (what must be TRUE):

  1. The turn core (runner + engine/hook/MCP wiring) lives in `internal/runtime` shared by the ACP and Telegram frontends with no copied construction paths, and the operator can launch Telegram standalone (`ass-guard telegram`) or beside ACP (`acp serve --telegram`), both reusing the shared core builder (TG-01, TG-06)
  2. A user chatting with the bot drives the same engine as ACP — stable `tg-<chatID>` session IDs with resume across restarts, long-poll `Start(ctx)`, `deleteWebhook` at startup, no daemon and no network port; shutdown is one drain path, SIGTERM and stdin-EOF identical, draining in < 5s (TG-02)
  3. A user runs a full SDD scenario from Telegram text — `/opsx:*` commands work from chat, output arrives as fence-aware ≤4096-char chunks with MarkdownV2 escaping, `/stop` cancels the in-flight turn, an allowlist gates access, and engine-ask surfaces as a chat message (TG-03)
  4. A user sending a voice message gets it transcribed and executed as ordinary user input — OGG/Opus download → `Transcriber` interface → text — with the OpenAI default via the existing go-openai client, Groq via base-URL swap, whisper.cpp via config-gated subprocess, async acknowledgement while transcribing, and both `voice` and `audio` updates handled (TG-04)
  5. The bot token never appears in any log, audit event, or error string — a dedicated redactor pattern with a canary test landing **before the first Telegram HTTP call** (TG-05)

**Phase gate:** `mise ci` clean AND a live Telegram round-trip — a full text SDD scenario driven from a real chat plus a voice message transcribed and driving a turn — AND the SIGTERM/stdin-EOF drain test AND stdout byte-clean in both launch modes. No stub-only evidence closes this phase.
**Plans:** 7 plans
Plans:
**Wave 1**

- [ ] 10-01-PLAN.md — `internal/runtime` extraction: the turn core becomes the shared engine home (THE structural refactor, first work)
- [ ] 10-02-PLAN.md — Telegram bot-token redactor + canary BEFORE the first Telegram HTTP call
- [ ] 10-03-PLAN.md — chat output pipeline: fence-aware 4096 chunking, MarkdownV2 escaping, throttled edit-and-spill streaming
- [ ] 10-05-PLAN.md — `internal/stt`: the Transcriber seam — OpenAI default (existing client), Groq by URL swap, whisper.cpp subprocess

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 10-04-PLAN.md — Telegram bot loop: chat↔session binding, allowlist, text turns on the shared engine, /stop + /new, long-poll lifecycle

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 10-06-PLAN.md — voice flow: download → transcribe → the transcript IS the user message (ack + auto-sent echo, no buttons)

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 10-07-PLAN.md — launch modes (`ass-guard telegram` + `acp serve --telegram`), the one drain path, and the LIVE phase gate

**UI hint**: yes

### Phase 11: dsh Mimicry Profile #2

**Goal:** As an operator routing DeepSeek-model turns, I want a second mimicry profile whose content is captured from deepseek-harness's real wire traffic, so that ass-guard's requests are structurally indistinguishable from dsh's — proving the N-profile thesis with no target-specific code paths.
**Mode:** mvp
**Depends on:** Phase 9 (capture-provenance pattern — recording-proxy wire truth, pinned-session discipline — made profile-agnostic) and follows Phase 10 per the operator priority chain. DSH-02's OpenAI `profile.System` mapping fix is generic and may land earlier if convenient, but the profile itself closes last. Verification-gated: wire truth comes from the capture, never from source-reading alone (dsh session JSONL stores events, not outgoing HTTP requests).
**Requirements:** DSH-01, DSH-02, DSH-03, DSH-04, DSH-05
**Success Criteria** (what must be TRUE):

  1. Shared mimicry code contains no zcode-specific paths — a zcode-ism audit genericizes or parameterizes them (e.g. redaction's preserved-header list becomes profile-supplied) so "N profiles, no target-specific code paths" holds for two profiles (DSH-01)
  2. The OpenAI-shape provider maps `profile.System` onto the wire exactly as captured dsh traffic does (form decided from the capture, not assumption), and the profile loader tolerates profiles without `thinking.json` / `tool_choice.json` (DSH-02)
  3. The dsh profile's content is captured, not hand-written — recording-proxy wire-truth capture at the configured baseURL plus zstd harvest of dsh session event logs (decode spike against a real `session.jsonl.zstd` first), with the pinned dsh commit recorded in profile meta (DSH-03)
  4. An operator selects `--profile dsh`, `scheduling.yaml` carries a credentialed DeepSeek provider entry, `profile check dsh` detects drift against the per-profile capture, and a sanitized dsh seed ships in the embedded defaults (DSH-04)
  5. The A/B parity harness is green for dsh against a DeepSeek endpoint, one live DeepSeek tool-calling round-trip succeeds per routed model, and every profile entry is traceable to a captured request (DSH-05)

**Phase gate:** `mise ci` clean AND the zstd decode spike passed against a real `session.jsonl.zstd` AND the A/B parity harness (`internal/parity`) green against a DeepSeek endpoint AND one live DeepSeek tool-calling round-trip per routed model AND every profile entry traceable to a captured request. Per-turn profile switching stays a v1.2 non-goal — `--profile` remains explicit. No stub-only evidence closes this phase.
**Plans:** 7 plans
Plans:
**Wave 1**

- [ ] 11-01-PLAN.md — zcode-ism audit + shared-code generalization: committed census, the grep gate (CI-wired, provider-name-switch ban), redaction identity semantics profile-supplied [DSH-01]
- [ ] 11-02-PLAN.md — OpenAI adapter wire mapping (TDD): profile.System emission (1..N captured form), omit-when-empty tools, strict threading, message-shape fields, loader tolerance, real Stream (SSE + include_usage) + profile headers [DSH-02]
- [ ] 11-03-PLAN.md — the recording proxy (cmd/dsh-proxy): positioned plain-HTTP reverse proxy at the configured baseURL, redact-at-record, SSE capture, the recording-contract conformance suite [DSH-03]

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 11-04-PLAN.md — per-model profile selection: ModelConfig.Profile schema + resolution (--profile stays the override), the DeepSeek provider entry (opencode gateway, base-URL-swappable) with dialect Limitations, the gateway compatibility probe [DSH-04]
- [ ] 11-05-PLAN.md — zstd spike + dsh extractor + runbook: klauspost v1.19.2 exact pin (retraction guard), extract-profile -mode dsh (wire-truth extraction, system-form decision procedure, capture-line provenance), docs/dsh-recapture-runbook.md (manual, D-02) [DSH-03]

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 11-06-PLAN.md — the capture run (autonomous: false): operator workload at the pinned dsh commit (checkpoint), mechanical threshold verification, real-artifact zstd spike green, capture census BEFORE the bundle, profiles/dsh shipped as pure extractor output, sanitized seed embedded [DSH-03, DSH-04]

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 11-07-PLAN.md — the proof: per-profile capture loaders for `profile check dsh`, the dsh parity suite extractor, A/B parity run (fresh baseline; credential-gated loud-skip), live probe per routed model, traceability audit, the full phase-gate close-out [DSH-04, DSH-05]

## Dependency Chains

- **Phase 8 gates the milestone.** It closes v1.0's major known gap (the Phase-4 UAT kickoff diagnosis) and carries the only structural blocker (`discoverCommands` flat-scan) as its first task. Its expansion must land at the turn runner, not the ACP handler — surface-agnosticity is why Phase 10 inherits `/opsx:*` without rework.
- **Phase 9 follows Phase 8** per the operator chain, with audit + re-capture merged (adjacent, both small). The deliberate sequencing judgment: landing the capturer factory seam in its final home BEFORE Phase 10's runtime extraction moves the wiring — written once, not re-wired. AUD-05 is technically independent of Phase 8 but rides this phase's operator-runbook design.
- **Phase 10 needs Phases 8–9.** Telegram needs the session-layer command expansion (Phase 8) and the landed capturer seam (Phase 9), and itself performs the `internal/runtime` extraction both frontends then share. The bot-token redactor + canary must land before the first Telegram HTTP call.
- **Phase 11 needs Phase 9's genericized capture provenance** and follows Phase 10 per the operator chain. It is the only phase with genuine unknowns left (wire capture runbook, zstd decode, system-message mapping form) — expect a research/capture spike at phase start.
- **Cross-phase invariants:** `mise ci` clean at every phase gate; stdout = ACP frames only; no daemon, no network port, single static binary (CGO_ENABLED=0); `.claude/` strictly read-only; no feature closes with stub-only evidence.

## Research Flags by Phase

- **Phase 8:** ground truth already in-repo (installed-binary surface table in STACK.md, zcode substitution semantics from the shipped diagnostics skill, real `openspec init` fixture layout) — plan directly, no research phase.
- **Phase 9:** not library research but operator-procedure design — the divergence-prone workload spec, session pinning, and re-baselining steps should be written into the phase plan verbatim from research Pitfalls 17/18. Also decide the `CurrentTurnID()` accessor vs empty-TurnID fallback.
- **Phase 10:** standard patterns (go-telegram/bot usage, Bot API limits, STT endpoints documented; pitfalls enumerated) — skip research phase. Resolve the telegram-only-mode vs "no standalone CLI surface" PROJECT.md tension deliberately at planning, not via a hack.
- **Phase 11 (highest research depth):** the recording-proxy capture runbook (baseURL swap against a real dsh install), the zstd decode spike, the exact system-message mapping form (answerable only from the capture), and DeepSeek dialect quirks against the captured wire. Recommend `/gsd:plan-phase --research-phase 11`.

## Progress

**Execution order:** 8 → 9 → 10 → 11 (strict operator priority chain; no parallelism across phases).

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 8. Slash-Command Kickoff | v1.1 | 0/TBD | Not started | - |
| 9. Serve-Path Audit + zcode Parity Re-capture | v1.1 | 0/TBD | Not started | - |
| 10. Telegram Peer (Text + Voice) | v1.1 | 0/TBD | Not started | - |
| 11. dsh Mimicry Profile #2 | v1.1 | 0/TBD | Not started | - |

---
*Roadmap created: 2026-08-14*
*Derived from: PROJECT.md (v1.1 milestone), REQUIREMENTS.md (21 v1.1 REQ-IDs), research/SUMMARY.md (4-phase proposal matching the operator chain)*
