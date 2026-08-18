# Roadmap: ass-guard-agent (working name)

**Project mode:** mvp (vertical slices — each phase delivers an end-to-end user capability)
**Milestone:** v1.1 Product Completion (Phases 8, 9, 12, 13; numbering continues from v1.0's Phase 7 — never reset)
**Requirements mapped:** 24/24 v1.1 ✓

v1.1 completes the product. Phase 8 closed the kickoff loop (`/opsx:explore → propose → apply → archive` chained by the engine with zero manual continues, verified against the real openspec binary) — v1.0's major known gap. **Operator re-scope 2026-08-16 + phase split:** Telegram (ex-Phase 10) and the dsh profile (ex-Phase 11) are lowered to the v1.2 planning pool in favor of two new phases — **Phase 12: Product Functional Completeness** (every built-in catalog tool executes for real — 9 still return "no implementation yet" — result forms capture-pinned, the behavioral-eval regression net, plugin-install discovery) and **Phase 13: OpenSpec Workflow Completion** (the expanded command matrix runs E2E with zero-continue chaining and eval coverage). Phase 9 stays next (it *is* product-surface work: serve-path audit + parity re-capture). Scope discipline after the re-scope: **zero new dependencies in v1.1** (go-telegram/bot, zstd, and the `internal/runtime` refactor move to v1.2 with their phases), and the generalized v1.0 lesson holds as a cross-phase invariant — **no feature closes with stub-only evidence; every external surface carries a real-binary/live-service gate**.

## Milestones

- ✅ **v1.0 MVP** — Phases 0–7 (shipped 2026-08-14; full detail: `.planning/milestones/v1.0-ROADMAP.md`, artifacts in `.planning/milestones/v1.0-phases/`, record in `.planning/MILESTONES.md`)
- 🚧 **v1.1 Product Completion** — Phases 8, 9, 12, 13 (1/4 complete; renamed "Kickoff & Peers" → "ACP Completion" → "Product Completion" at the 2026-08-16 re-scope + split)
- 📋 **v1.2 pool (not yet a milestone)** — ex-Phases 10–11 (Telegram peer; dsh profile #2 — replan against source-analysis), SEED-001 agent-creation kit, ECOSYSTEM-AUDIT clusters PLUG (full lifecycle) / LSP / MEM

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
- [x] **Phase 9: Serve-Path Audit + zcode Parity Re-capture** - redacted, bounded, decision-explaining audit on `acp serve`; stability test re-grounded on a pinned capture session (completed 2026-08-18)
- [ ] **Phase 12: Product Functional Completeness** - every built-in catalog tool executes for real (the 9 deferred tools + Bash background flags), capture-pinned result forms, the behavioral-eval regression net, plugin-install discovery *(added at the 2026-08-16 re-scope; split same day — machinery half; executes directly after Phase 9)*
- [ ] **Phase 13: OpenSpec Workflow Completion** - the expanded OpenSpec command matrix (`new / continue / ff / verify / bulk-archive / onboard`) runs E2E with zero-continue chaining and eval coverage *(the 2026-08-16 split's workflow half; executes after Phase 12)*
- [ ] ~~**Phase 10: Telegram Peer (Text + Voice)**~~ - → **moved to the v1.2 pool** (operator 2026-08-16; plan preserved in `.planning/phases/10-telegram-peer-text-voice/`)
- [ ] ~~**Phase 11: dsh Mimicry Profile #2**~~ - → **moved to the v1.2 pool** (operator 2026-08-16; needs replanning against source-analysis — plan preserved in `.planning/phases/11-dsh-mimicry-profile-2/`)

## Phase Overview

| # | Phase | Goal | Requirements | Success Criteria |
|---|-------|------|--------------|------------------|
| 8 | Slash-Command Kickoff | 8/9 | Completed 2026-08-16 — operator witness accepted; findings 5+6 dispositions confirmed (hybrid provenance chaining + D-10 capture-faithful reshape); full guard green; gated E2E: fixable green + zero-continue 2/3 (one stage-4 model-variance fail recorded as known residual, same class as UAT check 3); UAT 10 pass/1 partial/0 blocked | 2026-08-16 |
| 9 | Serve-Path Audit + zcode Parity Re-capture | 6/6 | Complete   | 2026-08-18 |
| 12 | Product Functional Completeness | Every catalog tool the model can see executes for real, result forms are capture-pinned, a behavioral-eval regression net guards turn behavior, and plugin installs widen discovery | ACP-01..08, ACP-10 | 9 |
| 13 | OpenSpec Workflow Completion | The expanded OpenSpec command matrix runs E2E through ass-guard against the real binary, zero-continue chained, eval-covered | OS-01, OS-02, OS-03 | 3 |
| 10 | ~~Telegram Peer (Text + Voice)~~ | *Moved to v1.2 pool (2026-08-16)* | TG-01..06 | - |
| 11 | ~~dsh Mimicry Profile #2~~ | *Moved to v1.2 pool (2026-08-16); replan against source-analysis* | DSH-01..05 | - |

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
**Depends on:** Phase 8 (operator priority chain; also the engine decisions recorded by AUD-04 observe Phase-8-hardened turns). AUD (audit) and AUD-05 (re-capture) are adjacent in the operator's chain and merged here by design. AUD-05 additionally unblocks the v1.0 Phase-1 within-session stability test (`eea3dc48` is absent on disk), and its newly pinned session is the ground truth ACP-07's corpus-absent result forms re-pin against in Phase 12.
**Requirements:** AUD-01, AUD-02, AUD-03, AUD-04, AUD-05
**Success Criteria** (what must be TRUE):

  1. A provider request made during `acp serve` is captured through a single-sourced factory-seam capturer covering both protocol shapes — the existing tracer path refactored onto the same seam, with no divergent provider-construction copies (the copy-`tracerProvider` shortcut is banned) (AUD-01)
  2. Every serve-path session writes a redacted audit trail — per-session transcript events with correlation IDs (session/turn/request) including `RequestShaped`, plus an optional `--audit-log` mirror (0600, append-only JSONL, rejects stdout as a target) (AUD-02)
  3. Audit volume is bounded via the body_ref pattern (hash in the event, full body in a capped store), and audit write failures are loud but never fatal to a turn (AUD-03)
  4. The audit trail records engine decisions (continue / hook / ask / wait, with the matched signal) — the "why did the agent continue" question is answerable from the log alone (AUD-04)
  5. The zcode parity stability test runs green against a newly pinned divergence-prone capture session produced via the operator runbook (scripted subagent/MCP-attach/tool-variety workload — not richest-session selection), with the pinned session ID consumed by the test, zcode + extractor versions recorded, thresholds explicitly re-baselined, and the drift report committed before any profile update (AUD-05)

**Phase gate:** `mise ci` clean AND live-serve redacted-audit verification (a redacted `RequestShaped` line observed on a real serve) AND the stability test green against the newly pinned session AND token/secret canary greps clean. No stub-only evidence closes this phase.
**Plans:** 6/6 plans complete
Plans:
**Wave 1**

- [x] 09-01-PLAN.md — factory capturer seam: `BuildWithCapturer` both shapes (anthropic + openai), tracer refactored onto the seam (`tracerProvider` deleted), serve-path RequestShaped + CurrentTurnID + per-session TranscriptWriter, redacted-line integration test [AUD-01, AUD-02]
- [x] 09-02-PLAN.md — engine-decision full provenance: matched text span + config source threaded PatternTable → Decide → EngineDecision event + engine_decision transcript line (D-03) [AUD-04]
- [x] 09-03-PLAN.md — re-capture machinery: stability test consumes the pinned session (never PickRichestMain), divergence canary fixture, zcode_version provenance, the operator runbook in-repo (verbatim from Pitfalls 17/18) [AUD-05]

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 09-04-PLAN.md — the re-capture run: operator scripted divergence-prone workload (checkpoint), threshold verification, drift report committed BEFORE the profile update, re-extract + re-pin, stability green, parity re-baseline (ZAI_API_KEY-gated leg) [AUD-05]
- [x] 09-05-PLAN.md — bounded audit volume: the body_ref pattern — metadata-only request events (correlation triple + shape fingerprint), capped redacted body store retrievable by hash (D-01) [AUD-02, AUD-03]

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 09-06-PLAN.md — the per-session audit mirror under `.ass-guard/audit/` (default ON), `--audit-log` override on serve, shared stdout-rejecting sink opener, header-NAME discipline, automated secret canary (D-02) [AUD-02]

### Phase 10: Telegram Peer (Text + Voice) — MOVED TO v1.2 (operator 2026-08-16)

> **Status: lowered to the v1.2 planning pool.** The operator re-prioritized ACP functional completeness (Phase 12) ahead of the peer surfaces. This phase's approved plans are preserved verbatim in `.planning/phases/10-telegram-peer-text-voice/` for v1.2 re-ingestion; requirements TG-01..06 moved with it. When re-planned, re-verify the "Depends on" lines below against whatever has landed in the meantime (Phase 12 adds no `internal/runtime` extraction — that refactor now belongs to this phase again whenever it executes).

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

### Phase 11: dsh Mimicry Profile #2 — MOVED TO v1.2 (operator 2026-08-16; replan required)

> **Status: lowered to the v1.2 planning pool.** Two independent reasons: the operator's 2026-08-16 priority call (ACP first), and the standing 2026-08-15 operator constraint — dsh wire capture is impossible, so ground truth must come from **source analysis of the pinned dsh commit**. As written below, this phase's method (recording-proxy wire capture, DSH-03) and the DSH-05 live-A/B bar are dead as designed; plans 11-03, 11-06, and the capture-dependent parts of 11-02/11-05/11-07 must be replanned before this phase can execute in v1.2. The text below is preserved as the last approved shape, not the method going forward.

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

### Phase 12: Product Functional Completeness

**Goal:** As a developer driving ass-guard from an ACP editor, I want every tool in the captured catalog to execute for real with capture-pinned result forms, and turn behavior guarded by a behavioral-eval regression net, so that the product machinery is complete — the model never hits a `no implementation yet` dead end mid-task.
**Mode:** mvp
**Depends on:** Phase 9 (AUD-05's newly pinned capture session is the ground truth ACP-07 re-pins the corpus-absent result forms against; the eval scenarios ride the Phase-8-proven real-binary gate pattern). Builds on shipped v1.0 + Phases 8–9: the 08-08 core-executor pattern (RegisterCore, captured result forms, catalog-schema-never-rewritten discipline) is the template every new executor follows. **Added at the 2026-08-16 operator re-scope, split same day** — this is the machinery half; the OpenSpec-workflow half is Phase 13 (which extends this phase's eval suites and consumes ACP-01's AskUserQuestion route).
**Requirements:** ACP-01, ACP-02, ACP-03, ACP-04, ACP-05, ACP-06, ACP-07, ACP-08, ACP-10
**Success Criteria** (what must be TRUE):

  1. The model invoking `AskUserQuestion` gets a real question surface on ACP — question + options reach the client in the captured shape, the turn suspends on the engine's ask path, the operator's reply lands as the tool result; capture-grounded result form; the no-confirmation-tier safety model untouched (a model-initiated question, not a tool-execution gate) (ACP-01)
  2. `EnterPlanMode` / `ExitPlanMode` execute with captured result forms, plan-mode state visible in the transcript and respected by the turn loop, scope per capture (ACP-02)
  3. `SendMessage` and `ReadSessionContext` execute for real — cross-agent messaging and prior-session context reads in capture-grounded forms (ACP-03)
  4. `CronCreate` / `CronList` / `CronDelete` execute against a real persisted schedule, and a due scheduled prompt fires as an engine-driven turn while the agent runs — no daemon, no network port, firing semantics within the editor-owned lifecycle (ACP-04)
  5. `TaskStop` executes for real — cancels the targeted in-flight task/background work with the captured result form (ACP-05)
  6. Bash `run_in_background` and `dangerouslyDisableSandbox` execute faithfully per captured semantics, with background-shell output retrieval working in the captured form (ACP-06)
  7. The corpus-absent result forms — truncation markers, Bash timeout form, Bash default timeout, file-tool failure forms — are re-pinned against the Phase-9 pinned session and implemented where the corpus shows them (ACP-07, depends on AUD-05)
  8. A behavioral-eval regression net exists and gates: deterministic tool-unit tests, scenario suites (pass@k, real binary, scratch project — initial suite = the Phase-8-proven `explore → propose → apply → archive` scenario), re-run gate on profile / model / turn-behavior changes (ACP-08, ECOSYSTEM-AUDIT §4.4 EVAL-01..03; Phase 13 extends the suites)
  9. Command + skill discovery reads Claude-Code-compatible plugin installs (`installed_plugins.json` + cache layout, PLUG-05 carve-out), merging plugin skills/commands with documented precedence; reads span `~/.claude/plugins/` and `~/.zcode/cli/plugins/`, writes stay under the ass-guard root (ACP-10)

**Phase gate:** `mise ci` clean AND a live-serve session exercising `AskUserQuestion` end-to-end (operator answers a real model question) AND zero `no implementation yet` strings reachable from the 19-tool built-in catalog on any turn (grep-gated) AND the eval suites green in CI. No stub-only evidence closes this phase.
**Plans:** 8 plans
Plans:
**Wave 1**

- [ ] 12-01-PLAN.md — AskUserQuestion end-to-end: the question surface, turn suspension on the engine ask path, reply-as-tool-result, the D-01 timeout policy + the live-serve operator witness (THE phase tracer) [ACP-01]
- [ ] 12-02-PLAN.md — plugin-install discovery: `installed_plugins.json` + cache layout from `~/.claude/plugins/` AND `~/.zcode/cli/plugins/`, precedence merge, read-only boundary [ACP-10]
- [ ] 12-03-PLAN.md — parity extractor fixes: `ExtractTurnsFromRollout` delta-record reconstruction + per-turn workspace isolation (the from-rollout A/B artifact classes, D-04 #2) [ACP-07]

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 12-04-PLAN.md — plan mode (EnterPlanMode/ExitPlanMode over the ask seam, no-gating design pinned) + SendMessage (agent mailbox) + ReadSessionContext (persisted-session reader) [ACP-02, ACP-03]
- [ ] 12-05-PLAN.md — re-record from live zcode via the archived driver kit: the deferred-tools tour, the forms harvest, the committed re-pinned fixture, the ACP-07 families implemented (D-04 primary route)

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 12-06-PLAN.md — background work: Bash `run_in_background` + the TaskRegistry, TaskOutput retrieval, TaskStop group-kill, session-close reaping, `dangerouslyDisableSandbox` by-design [ACP-05, ACP-06]

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 12-07-PLAN.md — cron: the persisted `.ass-guard/schedule/` store, the four executors, queue/fire-once engine-driven turns with missed-window notes, the FULL catalog-completeness test [ACP-04]

**Wave 5** *(blocked on Wave 4 completion)*

- [ ] 12-08-PLAN.md — the behavioral-eval regression net: extracted E2E harness, the scenario runner (pass@k, flagship suite), the `ASSGUARD_EVAL_GATE` gate + change-class detector + k=3 manual task (D-03) [ACP-08]

### Phase 13: OpenSpec Workflow Completion

**Goal:** As a developer practicing SDD with OpenSpec, I want the toolkit's full command matrix — beyond the proven `explore → propose → apply → archive` loop — to run end-to-end through ass-guard with zero-continue chaining, so that the unmodified toolkit works hands-off, not just its flagship workflow.
**Mode:** mvp
**Depends on:** Phase 12 (the eval net its scenarios extend, OS-03; ACP-01's AskUserQuestion as the interactive-dead-end route, OS-02) and Phase 8 (the proven loop, the chaining machinery — `TurnOutput.StartedBy` + `CommandMatcher` + `[[command_patterns]]` seeds — and the probe-pinned adapter surface). **Added at the 2026-08-16 operator split** ("make it working with openspec"); absorbs ex-ACP-09.
**Requirements:** OS-01, OS-02, OS-03
**Success Criteria** (what must be TRUE):

  1. The expanded OpenSpec command matrix — `new / continue / ff / verify / bulk-archive / onboard` — drives E2E through ass-guard against the real openspec binary, happy and fixable-failure paths (OS-01)
  2. Zero-continue chaining covers the expanded matrix — engine pattern seeds / dual-signal rows for the new command handoffs, structural-safety discipline unchanged (assistant-role-only matching, unmatched ⇒ nothing); interactive dead-ends surface via `AskUserQuestion` instead of stalling the chain — the Phase-8 stage-4 residual class, now with a tool-shaped route (OS-02)
  3. The Phase-12 eval suites are extended with per-command scenario suites for the expanded matrix (pass@k, real binary, scratch project), running in the same re-run gate (OS-03)

**Phase gate:** `mise ci` clean AND the expanded-matrix E2E green against the real binary (`ASSGUARD_OPENSPEC_BIN=1`, happy + fixable per command) AND chaining decisions evidenced in the audit trail (Phase-9 EngineDecision lines) AND the extended eval suites green in CI. No stub-only evidence closes this phase.
**Plans:** TBD — dispatch `/gsd:plan-phase 13` after Phase 12 closes (context: this section, the Phase-8 chaining/provenance decisions in STATE.md, the openspec v1.5.0 installed-binary surface the Phase-8 adapter probe already pinned, and the 08-06 E2E harness as the template).

## Dependency Chains

- **Phase 8 gated the milestone** and is complete (2026-08-16). Its session-layer command expansion is what any future peer surface (v1.2 Telegram) inherits for free, and its chaining machinery (`StartedBy` + `CommandMatcher` + pattern seeds) is what Phase 13 extends to the full command matrix.
- **Phase 9 follows Phase 8** per the operator chain, with audit + re-capture merged (adjacent, both small). Its operator blocker stands: the scripted 5-turn capture workload (`docs/recapture-runbook.md` §3) must be run once by the operator; the automated 09-04 legs + phase verification follow. AUD-05's pinned session is also Phase 12's ACP-07 ground truth.
- **Phase 12 needs Phase 9.** The corpus-absent result forms re-pin against AUD-05's newly pinned session, and the eval scenarios reuse the Phase-8/9 real-binary gate pattern. Everything else in Phase 12 builds directly on shipped 08-08 machinery (RegisterCore + captured result forms).
- **Phase 13 needs Phases 8 and 12.** The command matrix rides Phase 8's proven loop, chaining machinery, and probe-pinned adapter surface; its eval scenarios extend Phase 12's suites (OS-03) and its interactive dead-ends route through Phase 12's AskUserQuestion (OS-02).
- **Phases 10–11 moved to the v1.2 pool (2026-08-16).** Their former ordering constraints are recorded for the v1.2 replan: Telegram wants Phase 8's session-layer expansion (already shipped) and performs the `internal/runtime` extraction itself; dsh needs full replanning against source-analysis ground truth. Nothing in v1.1 depends on either.
- **Cross-phase invariants:** `mise ci` clean at every phase gate; stdout = ACP frames only; no daemon, no network port, single static binary (CGO_ENABLED=0); `.claude/` strictly read-only; no feature closes with stub-only evidence.

## Research Flags by Phase

- **Phase 8:** ground truth already in-repo (installed-binary surface table in STACK.md, zcode substitution semantics from the shipped diagnostics skill, real `openspec init` fixture layout) — plan directly, no research phase. *(Complete.)*
- **Phase 9:** not library research but operator-procedure design — the divergence-prone workload spec, session pinning, and re-baselining steps should be written into the phase plan verbatim from research Pitfalls 17/18. Also decide the `CurrentTurnID()` accessor vs empty-TurnID fallback. *(Plans done; executing.)*
- **Phase 12 (new):** mostly in-repo ground truth — the 08-08 deferred-tools table (08-08-SUMMARY), the rollout corpus for captured AskUserQuestion/plan-mode/cron/SendMessage forms, ECOSYSTEM-AUDIT §4.1 (PLUG plugin layout ground truth: `installed_plugins.json` + cache layout measured live), §4.4 (EVAL pyramid), §5 (near-term folds). One design unknown to resolve at planning: ACP-01's ask-path suspension semantics (how a model-initiated AskUserQuestion suspends and resumes within ACP's turn model without becoming a confirmation tier) and ACP-04's cron firing semantics under the editor-owned lifecycle. Recommend `/gsd:plan-phase 12` with a light research pass over the corpus forms only if the harvested shapes prove insufficient.
- **Phase 13 (new):** ground truth already in-repo — the openspec v1.5.0 installed-binary surface the Phase-8 adapter probe pinned, the Phase-8 chaining/provenance decision record (STATE.md), and the 08-06 E2E harness as template. Plan directly; no research phase expected.
- **Phase 11 (moved to v1.2, replan required):** the original research flags (recording-proxy runbook, zstd decode, system-message form from capture) are superseded by the source-analysis method — the v1.2 replan re-scopes them (derive runtime-composed content from dsh's source composition logic; re-scope the A/B bar per the 2026-08-15 STATE.md constraint entry).

## Progress

**Execution order:** 8 → 9 → 12 → 13 (v1.1; strict operator priority chain; no parallelism across phases). Phases 10–11 moved to the v1.2 pool 2026-08-16.

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 8. Slash-Command Kickoff | v1.1 | 9/9 | Complete (operator witness accepted; 2 known residuals documented) | 2026-08-16 |
| 9. Serve-Path Audit + zcode Parity Re-capture | v1.1 | 6/6 | Complete (verifier PASS; UAT 3/4 — parity re-baseline disposition accepted, live-serve audit witnessed, capture-method deviation stands; test 4 informational) | 2026-08-18 |
| 12. Product Functional Completeness | v1.1 | 8 plans approved (checker-passed 2026-08-17; awaiting execution dispatch after Phase 9 closes) | Planning complete | - |
| 13. OpenSpec Workflow Completion | v1.1 | 0 (planning dispatches when Phase 12 closes) | Not started | - |
| 10. Telegram Peer (Text + Voice) | v1.2 pool | 0/7 (plans preserved) | Moved to v1.2 (2026-08-16) | - |
| 11. dsh Mimicry Profile #2 | v1.2 pool | 0/7 (plans stale — replan required) | Moved to v1.2 (2026-08-16) | - |

---
*Roadmap created: 2026-08-14 · Re-scoped: 2026-08-16 (operator — phases 10/11 lowered to the v1.2 pool; Phase 12 + Phase 13 added via re-scope + split; milestone renamed v1.1 Product Completion)*
*Derived from: PROJECT.md (v1.1 milestone), REQUIREMENTS.md (24 v1.1 REQ-IDs after re-scope + split), research/SUMMARY.md, research/ECOSYSTEM-AUDIT.md (Phase 12)*
