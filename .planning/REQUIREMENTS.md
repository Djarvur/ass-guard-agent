# Requirements: ass-guard-agent (working name) — v1.1 Kickoff & Peers

**Defined:** 2026-08-14
**Core Value:** Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first) — if the model can tell the requests apart, everything built on top is compromised.

**v1.1 scope discipline:** Strict priority chain — kickoff gap first; audit + parity re-capture merged (adjacent); Telegram; dsh profile #2 last. Two new deps only (go-telegram/bot v1.23.0, klauspost/compress/zstd v1.19.2); one structural refactor (`internal/runtime` extraction). No feature closes with stub-only evidence — every external surface carries a real-binary/live-service gate. Research: `.planning/research/SUMMARY.md` (2026-08-14).

## v1.1 Requirements

### Slash-Command Kickoff (priority 1 — the milestone's product proof)

- [x] **CMD-01**: A user invoking `/opsx:explore` has the command discovered: `commands/<ns>/<name>.md` layouts (like `openspec init --tools claude` installs) are found via one-level subdirectory scan with colon-joined keys, under the existing precedence (project `.claude/` > user `.claude/` > `.ass-guard/`), proven by a real-fixture test generated from actual `openspec init` output
- [ ] **CMD-02**: A user typing `/namespace:name args` has the command's markdown body expanded with **zcode substitution semantics** — `$ARGUMENTS` and `$1..$N` (out-of-range → empty), args without placeholders appended under a "User arguments:" heading, `${ARGUMENTS}` brace form and `` !`cmd` `` dynamic shell NOT recognized — fed to the turn as the user message with the contract pinned by a table-driven edge-case test written before implementation; unknown `/foo` falls through as plain text
- [ ] **CMD-03**: The model invoking any `openspec:*` tool gets a real executed subprocess result — Adapter-backed `Execute` on registered tools, command surface pinned to the installed binary's probe (phantom `apply`/`implement` removed; read-only vs mutating classified), non-interactive guards (nil stdin, per-command timeout, exit-code classification)
- [ ] **CMD-04**: A developer runs a real `/opsx:explore → propose → apply → archive` OpenSpec scenario end-to-end through ass-guard in a scratch project with zero manual continues, gated by `ASSGUARD_OPENSPEC_BIN=1` against the real binary (happy / fixable-failure / missing-binary paths) — and the 11 deferred v1.0 Phase-4 UAT checks pass
- [x] **CMD-05**: Expanded turns record provenance (which command file drove the turn), engine pattern-matching remains assistant-role-only (regression test), and same-key shadowing across discovery scopes emits a warning
- [ ] **CMD-06**: Skills work claude-code-compatibly: the model invokes the `Skill` tool, skill name + description reach the model's context via the captured profile's shape with dynamically discovered skills merged in (the v1.0 dynamic-MCP-tools pattern), the skill's SKILL.md loads into the turn, and all discovered skills (`.claude/skills/` + `.ass-guard/skills/`, existing precedence) are exposed — the `/opsx` prompts naturally trigger the matching `openspec-*` skills *(added at Phase-8 discussion 2026-08-14 — user: "to make the commands working we also need skills working, again in claude code compatible way")*
- [x] **CMD-07**: WebSearch ships a real DDG-HTML default backend (zero API key) on the existing swappable-Backend seam, and WebFetch returns fetch + html→markdown conversion — at minimum sufficient for `/opsx:explore` workflows; keyed APIs remain config-swappable; zero-config first run unaffected *(added at Phase-8 discussion 2026-08-14 — user: "we need websearch and webfetch tools at least for /opsx:explore")*

### Audit & Parity Re-capture (priority 2-3, merged — adjacent in the operator chain)

- [ ] **AUD-01**: A provider request made during `acp serve` is captured through a single-sourced factory-seam capturer covering both protocol shapes (the existing tracer path refactored onto the same seam — no divergent provider-construction copies)
- [ ] **AUD-02**: Every serve-path session writes a redacted audit trail — per-session transcript events with correlation IDs (session/turn/request) including `RequestShaped`, plus an optional `--audit-log` mirror (0600, append-only JSONL, rejects stdout as a target)
- [ ] **AUD-03**: Audit volume is bounded via the body_ref pattern (hash in the event, full body in a capped store) and audit write failures are loud but never fatal to a turn
- [ ] **AUD-04**: The audit trail records engine decisions (continue / hook / ask / wait, with the matched signal) — the "why did the agent continue" question is answerable from the log alone
- [ ] **AUD-05**: The zcode parity stability test runs green against a newly pinned divergence-prone capture session, produced via an operator runbook (scripted subagent/MCP-attach/tool-variety workload — not richest-session selection), with the pinned session ID consumed by the test, zcode + extractor versions recorded, thresholds explicitly re-baselined, and the drift report committed before any profile update

### Telegram Peer (priority 4)

- [ ] **TG-01**: The turn core (runner + engine/hook/MCP wiring) lives in `internal/runtime` shared by ACP and Telegram frontends with no copied construction paths
- [ ] **TG-02**: A user chatting with the bot drives the same engine as ACP — stable `tg-<chatID>` session IDs with resume across restarts, long-poll `Start(ctx)`, one-drain-path shutdown (SIGTERM and stdin-EOF identical, < 5s), `deleteWebhook` at startup, no daemon and no network port
- [ ] **TG-03**: A user runs a full SDD scenario from Telegram text — `/opsx:*` commands work from chat, output arrives as fence-aware ≤4096-char chunks with MarkdownV2 escaping, `/stop` cancels the in-flight turn, an allowlist gates access, and engine-ask surfaces as a chat message
- [ ] **TG-04**: A user sending a voice message gets it transcribed and executed as ordinary user input — OGG/Opus download → `Transcriber` interface → text, with OpenAI default via the existing go-openai client, Groq via base-URL swap, whisper.cpp via config-gated subprocess; async acknowledgement while transcribing; `voice` and `audio` updates both handled
- [ ] **TG-05**: The Telegram bot token never appears in any log, audit event, or error string — dedicated redactor pattern with a canary test landing before the first Telegram HTTP call
- [ ] **TG-06**: The operator can launch Telegram standalone (`ass-guard telegram`) or beside ACP (`acp serve --telegram`), both reusing the shared core builder

### dsh Mimicry Profile #2 (priority 5)

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
- Expanded OpenSpec profile E2E (new/continue/ff/verify/bulk-archive/onboard commands)
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

Which phases cover which requirements. Filled during roadmap creation (2026-08-14 — 21/21 mapped, no orphans).

| Requirement | Phase | Status |
|-------------|-------|--------|
| CMD-01 | Phase 8 | Complete |
| CMD-02 | Phase 8 | Pending |
| CMD-03 | Phase 8 | Pending |
| CMD-04 | Phase 8 | Pending |
| CMD-05 | Phase 8 | Complete |
| CMD-06 | Phase 8 | Pending |
| CMD-07 | Phase 8 | Complete |
| AUD-01 | Phase 9 | Pending |
| AUD-02 | Phase 9 | Pending |
| AUD-03 | Phase 9 | Pending |
| AUD-04 | Phase 9 | Pending |
| AUD-05 | Phase 9 | Pending |
| TG-01 | Phase 10 | Pending |
| TG-02 | Phase 10 | Pending |
| TG-03 | Phase 10 | Pending |
| TG-04 | Phase 10 | Pending |
| TG-05 | Phase 10 | Pending |
| TG-06 | Phase 10 | Pending |
| DSH-01 | Phase 11 | Pending |
| DSH-02 | Phase 11 | Pending |
| DSH-03 | Phase 11 | Pending |
| DSH-04 | Phase 11 | Pending |
| DSH-05 | Phase 11 | Pending |

---
*Requirements defined: 2026-08-14*
*Last updated: 2026-08-14 — v1.1 roadmap created: Phases 8–11 mapped (CMD→8, AUD→9, TG→10, DSH→11)*
