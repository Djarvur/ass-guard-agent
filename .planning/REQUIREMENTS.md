# Requirements: ass-guard-agent (working name) — v1.1 ACP Early Adoption

**Defined:** 2026-08-14 · **Re-scoped:** 2026-08-16 (operator) · **Re-ordered:** 2026-08-18 (operator — early adoption)
**Core Value:** Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first) — if the model can tell the requests apart, everything built on top is compromised.

**v1.1 scope discipline (after the 2026-08-16 re-scope + phase split):** The milestone completes the product — Phase 8 closed the kickoff loop (done); Phase 9 closes audit + parity re-capture; **Phase 12 (Product Functional Completeness)** makes the machinery whole: every catalog tool executes for real, result forms capture-pinned, the behavioral-eval regression net, plugin-install discovery; **Phase 13 (OpenSpec Workflow Completion)** makes the flagship toolkit whole: the expanded OpenSpec command matrix runs E2E with zero-continue chaining and eval coverage. **Telegram (TG-01..06) and the dsh profile (DSH-01..05) are lowered to the v1.2 planning pool** (operator priority: the product fully functional as soon as possible) — their two deps (go-telegram/bot v1.23.0, klauspost/compress/zstd v1.19.2) and the `internal/runtime` structural refactor move with them; v1.1 adds zero new dependencies. No feature closes with stub-only evidence — every external surface carries a real-binary/live-service gate. Research: `.planning/research/SUMMARY.md` (2026-08-14), `.planning/research/ECOSYSTEM-AUDIT.md` (Phase 12: EVAL §4.4, PLUG-05 §4.1/§5; Phase 14: §3.3 + §4.4).

**2026-08-18 re-order (early adoption):** the operator prioritizes starting daily ACP use ASAP — an explicit **adoption line** splits Phase 12's waves (12-01/02/05/04/06 pre-adoption; 12-03/07/08 post), Phase 13 continues post-adoption, and the dispositioned YES items of the other-agents analysis land as **Phase 14: Adoption Readiness** (EARLY-01..06 below — the operator confirmed the full six-item set 2026-08-18: "делаем все 1-6"). The milestone completes when daily use begins.

## v1.1 Requirements

### Slash-Command Kickoff (priority 1 — the milestone's product proof)

- [x] **CMD-01**: A user invoking `/opsx:explore` has the command discovered: `commands/<ns>/<name>.md` layouts (like `openspec init --tools claude` installs) are found via one-level subdirectory scan with colon-joined keys, under the existing precedence (project `.claude/` > user `.claude/` > `.ass-guard/`), proven by a real-fixture test generated from actual `openspec init` output
- [x] **CMD-02**: A user typing `/namespace:name args` has the command's markdown body expanded with **zcode substitution semantics** — `$ARGUMENTS` and `$1..$N` (out-of-range → empty), args without placeholders appended under a "User arguments:" heading, `${ARGUMENTS}` brace form and `` !`cmd` `` dynamic shell NOT recognized — fed to the turn as the user message with the contract pinned by a table-driven edge-case test written before implementation; unknown `/foo` falls through as plain text
- [x] **CMD-03**: The model invoking any `openspec:*` tool gets a real executed subprocess result — Adapter-backed `Execute` on registered tools, command surface pinned to the installed binary's probe (phantom `apply`/`implement` removed; read-only vs mutating classified), non-interactive guards (nil stdin, per-command timeout, exit-code classification)
- [x] **CMD-04**: A developer runs a real `/opsx:explore → propose → apply → archive` OpenSpec scenario end-to-end through ass-guard in a scratch project with zero manual continues, gated by `ASSGUARD_OPENSPEC_BIN=1` against the real binary (happy / fixable-failure / missing-binary paths) — and the 11 deferred v1.0 Phase-4 UAT checks pass
- [x] **CMD-05**: Expanded turns record provenance (which command file drove the turn), engine pattern-matching remains assistant-role-only (regression test), and same-key shadowing across discovery scopes emits a warning
- [x] **CMD-06**: Skills work claude-code-compatibly: the model invokes the `Skill` tool, skill name + description reach the model's context via the captured profile's shape with dynamically discovered skills merged in (the v1.0 dynamic-MCP-tools pattern), the skill's SKILL.md loads into the turn, and all discovered skills (`.claude/skills/` + `.ass-guard/skills/`, existing precedence) are exposed — the `/opsx` prompts naturally trigger the matching `openspec-*` skills *(added at Phase-8 discussion 2026-08-14 — user: "to make the commands working we also need skills working, again in claude code compatible way")*
- [x] **CMD-07**: WebSearch ships a real DDG-HTML default backend (zero API key) on the existing swappable-Backend seam, and WebFetch returns fetch + html→markdown conversion — at minimum sufficient for `/opsx:explore` workflows; keyed APIs remain config-swappable; zero-config first run unaffected *(added at Phase-8 discussion 2026-08-14 — user: "we need websearch and webfetch tools at least for /opsx:explore")*

### Audit & Parity Re-capture (priority 2-3, merged — adjacent in the operator chain)

- [ ] **AUD-01**: A provider request made during `acp serve` is captured through a single-sourced factory-seam capturer covering both protocol shapes (the existing tracer path refactored onto the same seam — no divergent provider-construction copies)
- [ ] **AUD-02**: Every serve-path session writes a redacted audit trail — per-session transcript events with correlation IDs (session/turn/request) including `RequestShaped`, plus an optional `--audit-log` mirror (0600, append-only JSONL, rejects stdout as a target)
- [ ] **AUD-03**: Audit volume is bounded via the body_ref pattern (hash in the event, full body in a capped store) and audit write failures are loud but never fatal to a turn
- [ ] **AUD-04**: The audit trail records engine decisions (continue / hook / ask / wait, with the matched signal) — the "why did the agent continue" question is answerable from the log alone
- [ ] **AUD-05**: The zcode parity stability test runs green against a newly pinned divergence-prone capture session, produced via an operator runbook (scripted subagent/MCP-attach/tool-variety workload — not richest-session selection), with the pinned session ID consumed by the test, zcode + extractor versions recorded, thresholds explicitly re-baselined, and the drift report committed before any profile update

### Adoption Readiness (priority 3 — added at the 2026-08-18 early-adoption re-order; Phase 14)

*(Operator 2026-08-18: "as the first step I would like the things we've got from the other agents analysis must be processed: are there anything useful worth to be implemented before I start to use the agent." Disposition of `research/ECOSYSTEM-AUDIT.md` §3.3/§4.4 + `research/IDEA-LANDSCAPE.md` §borrow-list (SEED-004): six YES items — the operator took the full set ("делаем все 1-6"). The remaining NO items are recorded in ROADMAP Phase 14's added-at note and stay routed — nothing analyzed is dropped silently. 2026-08-19 new-data disposition (operator priorities: agent usable ASAP / important things before use / functionality+autotests): the research amendment `cb3ab50` borrows #13-15 add NO new requirement — #15 surface census enters as EARLY-06's free verification input (+ the 12-06/12-07 completeness-test coverage manifest), #13's cheap drift-warning slice is an EARLY-03 planning input with the full nightly gate landing in 12-08, #14 outcome store routes to the v1.2 pool; the adoption line and the 30/30 mapping are unchanged.)*

- [x] **EARLY-01**: A bad mutating turn is recoverable — ass-guard snapshots workspace file state at turn boundaries into a shadow-git store under the ass-guard root (external git; never touches the user's repo index/remotes/HEAD; no daemon, no port — invariant-clean per ECOSYSTEM-AUDIT §4.4 SNAP and IDEA-LANDSCAPE 🧲-1), and the operator can list and restore checkpoints from the terminal (`ass-guard checkpoint list|restore`) — the no-confirmation-tier safety model gains its reversibility backstop, both analyses' #1 borrow
- [x] **EARLY-02**: The compaction question is answered from evidence, not assumption — the Phase-9 pinned capture is analyzed for zcode auto-compact / context-eviction / `cache_control` breakpoint behavior; the finding is committed as a decision artifact, and either "the profile already delivers compaction via mimicry" is documented and closed or a scoped implementation requirement is issued to the post-adoption queue (IDEA-LANDSCAPE gap 2, verify-first)
- [x] **EARLY-03**: Cache-hit behavior is observable — the parity harness carries a probe asserting dynamic merges (skills, MCP tools) respect stable→volatile ordering and captured `cache_control` placement against the pin, wired into the existing parity run, so daily-use cache/cost drift is detectable rather than only structural divergence (ECOSYSTEM-AUDIT §4.4 CACHE)
- [x] **EARLY-04**: The shaper is cross-validated against pi's wire layer — a committed audit diffs `internal/shaper` behaviors against pi's `packages/ai/src/api/anthropic-messages.ts` + `transform-messages.ts` (cache_control placement, thinking-config mapping, header merge order, compat-flag catalog), using pi's 43k-line test suite as the behavioral spec; every divergence found is dispositioned — fixed, justified in-repo, or routed with rationale (ECOSYSTEM-AUDIT §5-1; hardens exactly the north-star component)
- [ ] **EARLY-05**: Token economics are explicit — subagent turns route through the scheduler `light` tier by default (configurable), and a tool-output truncation policy (tail/bounded extraction in the captured result form) bounds oversized tool results before they enter the projected window (Claudecourse #27; ECOSYSTEM-AUDIT §4.4 ECON)
- [ ] **EARLY-06**: The uniform tool contract holds across the catalog, capture-grounded — `is_error` in tool_result exactly where the zcode corpus shows it (mimicry discipline: forms stay capture-faithful), per-tool timeouts, retry-only-transient (429/5xx/timeout) at the provider/tool seam, and `isConcurrencySafe`/`isDestructive` flags feeding the engine + parallel dispatch; existing partial implementations (openspec per-command timeouts, SSE-layer transient classification) are inventoried and the gaps closed (Claudecourse #5/#30; ECOSYSTEM-AUDIT §4.4 TOOLCON)

### Product Functional Completeness (priority 3 — the re-scope's machinery half, Phase 12; split 2026-08-16)

*(Operator re-scope 2026-08-16: "make the product fully functional as soon as possible"; split same day into the machinery half (this, Phase 12) and the OpenSpec-workflow half (Phase 13). Ground truth: 9 of the 19 built-in catalog tools still return "no implementation yet" — the 08-08 deferred-tools table never dispositioned; Bash background flags unimplemented; corpus-absent result forms routed to Phase 9's re-capture; zero behavioral-eval regression net — ECOSYSTEM-AUDIT §3.3 calls this the biggest methodological hole.)*

- [x] **ACP-01**: The model invoking `AskUserQuestion` gets a real question surface on ACP — question + options surface to the client in the captured shape, the turn suspends on the engine's ask path, and the operator's reply lands as the tool result; capture-grounded result form; the no-confirmation-tier safety model is untouched (a model-initiated question, not a tool-execution gate). Directly addresses the Phase-8 residual class where the model ends a terminal stage by asking a question in plain text and the engine correctly does nothing
- [ ] **ACP-02**: `EnterPlanMode` / `ExitPlanMode` execute for real with captured result forms; plan-mode state is visible in the transcript and respected by the turn loop (scope per capture — design at plan-phase)
- [ ] **ACP-03**: `SendMessage` and `ReadSessionContext` execute for real (cross-agent messaging + prior-session context reads), capture-grounded result forms
- [ ] **ACP-04**: `CronCreate` / `CronList` / `CronDelete` execute against a real persisted schedule, and a due scheduled prompt fires as an engine-driven turn while the agent runs (no daemon, no network port — firing semantics designed at plan-phase within the editor-owned lifecycle)
- [ ] **ACP-05**: `TaskStop` executes for real — cancels the targeted in-flight task/background work with the captured result form
- [ ] **ACP-06**: Bash `run_in_background` and `dangerouslyDisableSandbox` execute faithfully per captured semantics, with background-shell output retrieval working in the captured form
- [ ] **ACP-07**: The corpus-absent result forms — truncation markers, Bash timeout form, Bash default timeout, file-tool failure forms — are re-pinned against the newly pinned Phase-9 capture session (depends on AUD-05's pin) and implemented where the corpus shows them
- [ ] **ACP-08**: A behavioral-eval regression net exists (ECOSYSTEM-AUDIT §4.4 EVAL-01..03): deterministic tool-unit tests, scenario suites (pass@k against a scratch project with the real binary — the initial suite is the Phase-8-proven `explore → propose → apply → archive` scenario), and a re-run gate wired so profile / model / turn-behavior changes cannot land without the suites green. Phase 13 extends the suites across the expanded command matrix (OS-03)
- [x] **ACP-10**: Command + skill discovery reads Claude-Code-compatible plugin installs as native (`installed_plugins.json` + `<root>/plugins/cache/<marketplace>/<plugin>/<version>/` layout, PLUG-05 carve-out) — plugin-bundled `skills/`, `commands/`, `agents/`, `hooks/hooks.json`, and `.mcp.json` all merge into the existing surfaces with documented precedence (agents register as spawnable subagent types; hooks fire at the mapped lifecycle points with documented stdin/stdout/exit semantics; MCP servers register through the existing host as `mcp__<server>__<tool>`); roots are the PROJECT `.claude/plugins/` and USER `~/.claude/plugins/` (the `~/.zcode/cli/plugins/` root dropped by the operator 2026-08-19 — no kit ships an ass-guard target, so plugins are installed for Claude Code and consumed as native; the dynamic listing is content, not structure — mimicry unaffected); user + project `.claude/skills/`, `.claude/commands/`, `.claude/agents/` remain first-class chain entries; writes stay under the ass-guard root only *(revised by the operator 2026-08-19: "…поддержки ass-guard нет ни в одном популярном ките…, поддержать agents/, hooks/hooks.json, .mcp.json тоже")*

### OpenSpec Workflow Completion (priority 4 — the re-scope's workflow half, Phase 13; split 2026-08-16)

*(The operator split 2026-08-16: "make it working with openspec." Phase 8 proved the flagship loop — `explore → propose → apply → archive` — zero-continue against the real binary; the rest of the toolkit's command matrix is unproven. Absorbs ex-ACP-09.)*

- [ ] **OS-01**: The expanded OpenSpec command matrix — `new / continue / ff / verify / bulk-archive / onboard` — drives E2E through ass-guard against the real openspec binary, happy and fixable-failure paths (the `/opsx` beyond the proven loop)
- [ ] **OS-02**: Zero-continue chaining covers the expanded matrix — engine pattern seeds / dual-signal rows for the new command handoffs, with the structural-safety discipline unchanged (assistant-role-only matching, unmatched ⇒ nothing); interactive dead-ends in the matrix surface via `AskUserQuestion` (ACP-01) instead of stalling the chain — the Phase-8 stage-4 residual class, now with a tool-shaped route
- [ ] **OS-03**: The Phase-12 eval suites are extended with per-command scenario suites for the expanded matrix (pass@k, real binary, scratch project), running in the same re-run gate

## Moved to v1.2 (operator re-scope 2026-08-16)

*Phases 10–11 lowered from v1.1 by the operator (priority: ACP functionality first). Requirements preserved verbatim for the v1.2 planning pool; approved phase plans remain in `.planning/phases/10-…` / `11-…` for re-ingestion. Phase 11 additionally requires replanning against source-analysis ground truth (operator constraint 2026-08-15 — dsh wire capture impossible).*

### Telegram Peer (moved to v1.2)

- [ ] **TG-01**: The turn core (runner + engine/hook/MCP wiring) lives in `internal/runtime` shared by ACP and Telegram frontends with no copied construction paths
- [ ] **TG-02**: A user chatting with the bot drives the same engine as ACP — stable `tg-<chatID>` session IDs with resume across restarts, long-poll `Start(ctx)`, one-drain-path shutdown (SIGTERM and stdin-EOF identical, < 5s), `deleteWebhook` at startup, no daemon and no network port
- [ ] **TG-03**: A user runs a full SDD scenario from Telegram text — `/opsx:*` commands work from chat, output arrives as fence-aware ≤4096-char chunks with MarkdownV2 escaping, `/stop` cancels the in-flight turn, an allowlist gates access, and engine-ask surfaces as a chat message
- [ ] **TG-04**: A user sending a voice message gets it transcribed and executed as ordinary user input — OGG/Opus download → `Transcriber` interface → text, with OpenAI default via the existing go-openai client, Groq via base-URL swap, whisper.cpp via config-gated subprocess; async acknowledgement while transcribing; `voice` and `audio` updates both handled
- [ ] **TG-05**: The Telegram bot token never appears in any log, audit event, or error string — dedicated redactor pattern with a canary test landing before the first Telegram HTTP call
- [ ] **TG-06**: The operator can launch Telegram standalone (`ass-guard telegram`) or beside ACP (`acp serve --telegram`), both reusing the shared core builder

### dsh Mimicry Profile #2 (moved to v1.2 — needs replan against source-analysis)

- [ ] **DSH-01**: Shared mimicry code contains no zcode-specific paths — a zcode-ism audit genericizes or parameterizes them (e.g. redaction's preserved-header list becomes profile-supplied) so "N profiles, no target-specific code paths" holds for two profiles
- [ ] **DSH-02**: The OpenAI-shape provider maps `profile.System` onto the wire exactly as captured dsh traffic does (form decided from the capture, not assumption), and the profile loader tolerates profiles without `thinking.json` / `tool_choice.json`
- [ ] **DSH-03**: The dsh profile's content is captured, not hand-written — recording-proxy wire-truth capture at the configured baseURL plus zstd harvest of dsh session event logs (decode spike against a real `session.jsonl.zstd` first), with the pinned dsh commit recorded in profile meta
- [ ] **DSH-04**: Profile selection is declared **per-model in config** — model entries in `scheduling.yaml` carry a `profile` field (DeepSeek models → `dsh`, GLM → `zcode`; the explicit `--profile` flag remains as an override) — and `scheduling.yaml` carries a credentialed DeepSeek provider entry (the opencode gateway's OpenAI-compatible endpoint per Phase-11 D-01, base-URL-swappable), `profile check dsh` detects drift against the per-profile capture, and a sanitized dsh seed ships in the embedded defaults *(amended at Phase-11 discussion 2026-08-14 — user: "profile is specified per-model in config")*
- [ ] **DSH-05**: The A/B parity harness is green for dsh against a DeepSeek endpoint, one live DeepSeek tool-calling round-trip succeeds per routed model, and every profile entry is traceable to a captured request

## Future Requirements (deferred)

- Forum-topic threading per Telegram chat
- STT fallback chain beyond config-declared backends
- OTLP export of audit events
- Raw-body opt-in audit mode
- ~~Expanded OpenSpec profile E2E (new/continue/ff/verify/bulk-archive/onboard commands)~~ — consumed by ACP-09 (Phase 12, re-scope 2026-08-16)
- dsh Responses-API wire shape (only if verification demands it)
- Claude-Code command≡skill merge semantics (`context: fork`, stacking)
- TTS replies on Telegram
- Per-turn profile switching (v1.2)

## Out of Scope

- Approval buttons on Telegram — violate the no-confirmation-tier safety model and surface independence
- Webhook mode — opens a network port (violates the no-port constraint); long-poll only
- opencode as a mimicry target — operator-excluded (2026-08-14); landscape context only
- whisper.cpp via cgo binding — subprocess only; static binary preserved
- A standalone terminal REPL — `acp serve` and `telegram` are launch modes of the same binary, not a CLI surface
- Per-turn profile switching in v1.1 — `--profile` stays explicit; switching is v1.2

## Traceability

Which phases cover which requirements. Filled during roadmap creation (2026-08-14 — 21/21 mapped, no orphans); re-scoped 2026-08-16 (24/24 mapped in v1.1: CMD→8, AUD→9, ACP→12, OS→13; TG/DSH moved to the v1.2 pool; ex-ACP-09 absorbed into OS-01); re-ordered 2026-08-18 (30/30 mapped: EARLY→14).

| Requirement | Phase | Status |
|-------------|-------|--------|
| CMD-01 | Phase 8 | Complete |
| CMD-02 | Phase 8 | Complete |
| CMD-03 | Phase 8 | Complete |
| CMD-04 | Phase 8 | Complete (known residuals documented at gate) |
| CMD-05 | Phase 8 | Complete |
| CMD-06 | Phase 8 | Complete |
| CMD-07 | Phase 8 | Complete |
| AUD-01 | Phase 9 | Pending (code-complete per 09-01..03/05/06 summaries; phase not verified) |
| AUD-02 | Phase 9 | Pending (code-complete; phase not verified) |
| AUD-03 | Phase 9 | Pending (code-complete; phase not verified) |
| AUD-04 | Phase 9 | Pending (code-complete; phase not verified) |
| AUD-05 | Phase 9 | Pending (blocked on operator capture workload) |
| ACP-01 | Phase 12 | Complete |
| ACP-02 | Phase 12 | Pending |
| ACP-03 | Phase 12 | Pending |
| ACP-04 | Phase 12 | Pending |
| ACP-05 | Phase 12 | Pending |
| ACP-06 | Phase 12 | Pending |
| ACP-07 | Phase 12 | Pending (depends on AUD-05's pinned session) |
| ACP-08 | Phase 12 | Pending |
| ACP-10 | Phase 12 | Complete |
| EARLY-01 | Phase 14 | Complete |
| EARLY-02 | Phase 14 | Complete |
| EARLY-03 | Phase 14 | Complete |
| EARLY-04 | Phase 14 | Complete |
| EARLY-05 | Phase 14 | Pending |
| EARLY-06 | Phase 14 | Pending |
| OS-01 | Phase 13 | Pending (absorbs ex-ACP-09; post-adoption continuation) |
| OS-02 | Phase 13 | Pending |
| OS-03 | Phase 13 | Pending |
| TG-01 | v1.2 pool (ex-Phase 10) | Deferred |
| TG-02 | v1.2 pool (ex-Phase 10) | Deferred |
| TG-03 | v1.2 pool (ex-Phase 10) | Deferred |
| TG-04 | v1.2 pool (ex-Phase 10) | Deferred |
| TG-05 | v1.2 pool (ex-Phase 10) | Deferred |
| TG-06 | v1.2 pool (ex-Phase 10) | Deferred |
| DSH-01 | v1.2 pool (ex-Phase 11) | Deferred (replan needed) |
| DSH-02 | v1.2 pool (ex-Phase 11) | Deferred (replan needed) |
| DSH-03 | v1.2 pool (ex-Phase 11) | Deferred (replan needed — source-analysis method) |
| DSH-04 | v1.2 pool (ex-Phase 11) | Deferred (replan needed) |
| DSH-05 | v1.2 pool (ex-Phase 11) | Deferred (replan needed — A/B bar re-scope) |

---
*Requirements defined: 2026-08-14*
*Updated: 2026-08-18 — early-adoption re-order: Phase 14 Adoption Readiness added (EARLY-01..06, the full other-agents-analysis disposition — operator confirmed all six); Phase 12's waves split around the adoption line (12-01/02/05/04/06 pre-adoption; 12-03/07/08 post); Phase 13 post-adoption; 30/30 mapped; milestone renamed v1.1 ACP Early Adoption*
*Updated: 2026-08-19 — new-analysis disposition (borrows #13-15 from `cb3ab50`): no new requirements; #15 census → EARLY-06 input, #13 drift-warning → EARLY-03 input with the full gate in 12-08, #14 outcome store → v1.2 pool; adoption line unchanged (30/30 mapped)*
