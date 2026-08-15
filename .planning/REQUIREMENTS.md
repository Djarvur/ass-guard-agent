# Requirements: ass-guard-agent (working name) — v1.1 ACP Completion

**Defined:** 2026-08-14 · **Re-scoped:** 2026-08-16 (operator)
**Core Value:** Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first) — if the model can tell the requests apart, everything built on top is compromised.

**v1.1 scope discipline (after the 2026-08-16 re-scope):** The milestone is the ACP story — kickoff gap (Phase 8, done), audit + parity re-capture (Phase 9), then ACP Functional Completeness (Phase 12): every catalog tool the model can see executes for real, the result forms are capture-pinned, a behavioral-eval regression net exists, and the OpenSpec command matrix is covered E2E. **Telegram (TG-01..06) and the dsh profile (DSH-01..05) are lowered to the v1.2 planning pool** (operator priority: the agent fully functional on ACP as soon as possible) — their two deps (go-telegram/bot v1.23.0, klauspost/compress/zstd v1.19.2) and the `internal/runtime` structural refactor move with them; v1.1 adds zero new dependencies. No feature closes with stub-only evidence — every external surface carries a real-binary/live-service gate. Research: `.planning/research/SUMMARY.md` (2026-08-14), `.planning/research/ECOSYSTEM-AUDIT.md` (Phase 12: EVAL §4.4, PLUG-05 §4.1/§5).

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

### ACP Functional Completeness (priority 3 — the re-scope's product goal; added 2026-08-16)

*(Operator re-scope 2026-08-16: "make the agent fully functional on ACP as soon as possible." Ground truth: 9 of the 19 built-in catalog tools still return "no implementation yet" — the 08-08 deferred-tools table never dispositioned; Bash background flags unimplemented; corpus-absent result forms routed to Phase 9's re-capture; zero behavioral-eval regression net — ECOSYSTEM-AUDIT §3.3 calls this the biggest methodological hole.)*

- [ ] **ACP-01**: The model invoking `AskUserQuestion` gets a real question surface on ACP — question + options surface to the client in the captured shape, the turn suspends on the engine's ask path, and the operator's reply lands as the tool result; capture-grounded result form; the no-confirmation-tier safety model is untouched (a model-initiated question, not a tool-execution gate). Directly addresses the Phase-8 residual class where the model ends a terminal stage by asking a question in plain text and the engine correctly does nothing
- [ ] **ACP-02**: `EnterPlanMode` / `ExitPlanMode` execute for real with captured result forms; plan-mode state is visible in the transcript and respected by the turn loop (scope per capture — design at plan-phase)
- [ ] **ACP-03**: `SendMessage` and `ReadSessionContext` execute for real (cross-agent messaging + prior-session context reads), capture-grounded result forms
- [ ] **ACP-04**: `CronCreate` / `CronList` / `CronDelete` execute against a real persisted schedule, and a due scheduled prompt fires as an engine-driven turn while the agent runs (no daemon, no network port — firing semantics designed at plan-phase within the editor-owned lifecycle)
- [ ] **ACP-05**: `TaskStop` executes for real — cancels the targeted in-flight task/background work with the captured result form
- [ ] **ACP-06**: Bash `run_in_background` and `dangerouslyDisableSandbox` execute faithfully per captured semantics, with background-shell output retrieval working in the captured form
- [ ] **ACP-07**: The corpus-absent result forms — truncation markers, Bash timeout form, Bash default timeout, file-tool failure forms — are re-pinned against the newly pinned Phase-9 capture session (depends on AUD-05's pin) and implemented where the corpus shows them
- [ ] **ACP-08**: A behavioral-eval regression net exists (ECOSYSTEM-AUDIT §4.4 EVAL-01..03): deterministic tool-unit tests, `/opsx` scenario suites (pass@k against a scratch project with the real binary), and a re-run gate wired so profile / model / turn-behavior changes cannot land without the suites green
- [ ] **ACP-09**: The expanded OpenSpec command matrix — `new / continue / ff / verify / bulk-archive / onboard` — drives E2E through ass-guard against the real openspec binary (the `/opsx` beyond explore/propose/apply/archive)
- [ ] **ACP-10**: Command + skill discovery reads Claude-Code-compatible plugin installs (`installed_plugins.json` + `<root>/plugins/cache/<marketplace>/<plugin>/<version>/` layout, PLUG-05 carve-out) and merges plugin skills/commands into the existing discovery with documented precedence; reads span `~/.claude/plugins/` and `~/.zcode/cli/plugins/`, writes stay under the ass-guard root only

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

Which phases cover which requirements. Filled during roadmap creation (2026-08-14 — 21/21 mapped, no orphans); re-scoped 2026-08-16 (22/22 mapped in v1.1: CMD→8, AUD→9, ACP→12; TG/DSH moved to the v1.2 pool).

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
| ACP-01 | Phase 12 | Pending |
| ACP-02 | Phase 12 | Pending |
| ACP-03 | Phase 12 | Pending |
| ACP-04 | Phase 12 | Pending |
| ACP-05 | Phase 12 | Pending |
| ACP-06 | Phase 12 | Pending |
| ACP-07 | Phase 12 | Pending (depends on AUD-05's pinned session) |
| ACP-08 | Phase 12 | Pending |
| ACP-09 | Phase 12 | Pending |
| ACP-10 | Phase 12 | Pending |
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
*Last updated: 2026-08-16 — operator re-scope: Phases 10–11 lowered to the v1.2 pool; ACP-01..10 added as Phase 12 (ACP Functional Completeness); milestone renamed v1.1 ACP Completion*
