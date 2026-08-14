# Project Research Summary

**Project:** ass-guard — milestone v1.1 "Kickoff & Peers" (on the shipped v1.0 Go ACP agent)
**Domain:** AI coding agent with model-request mimicry; SDD toolkit hosting; multi-surface (ACP stdio + Telegram)
**Researched:** 2026-08-14
**Confidence:** HIGH overall — integration points verified against this repo's source and a live `openspec v1.5.0` probe; the one MEDIUM area (dsh zstd/log capture against real artifacts) carries a single named spike.

## Executive Summary

v1.1 adds five feature areas to a working shipped agent, in the operator's strict priority order: (1) slash-command kickoff — ecosys wiring + `/namespace:name` expansion + OpenSpec adapter reconciliation + the real-binary gate + 11 deferred UAT checks; (2) LOG-01 audit log on `acp serve`; (3) zcode parity re-capture; (4) Telegram peer (text + voice STT); (5) deepseek-harness ("dsh") mimicry profile #2. The milestone's reason to exist is area 1: closing the OpenSpec loop (`/opsx:explore → propose → apply → archive` chained by the engine with zero manual continues) is the product's proof. The recommended approach is conservative: exactly **two new Go dependencies** (`github.com/go-telegram/bot` v1.23.0 for the Telegram frontend, `github.com/klauspost/compress/zstd` v1.19.2 for dsh log harvest), one structural refactor (extract the turn core from `cmd/ass-guard/acp_serve.go` into `internal/runtime` as the Telegram prerequisite), and otherwise wiring on existing seams — the tracer/redactor for audit, the N-profile loader for dsh, the existing go-openai client for STT.

The four researchers independently converged on findings that must drive roadmap shape. **The structural blocker:** `internal/ecosys.discoverCommands` scans `commands/*.md` flat and skips directories, so `openspec init --tools claude`'s `.claude/commands/opsx/*.md` layout is invisible today — without fixing this first, `/opsx:*` cannot work at all, and the "wiring" phase becomes a mid-phase loader rewrite. **Dead tools:** `openspec.RegisterTools` registers catalog entries with no `Execute` (the Adapter is constructed nowhere outside tests) — model-invoked `openspec:*` calls return "no implementation yet". **Silent prompt drop:** the OpenAI-shape provider never reads `profile.System` — a dsh turn would carry no system prompt until this is fixed. **Capture correction:** dsh session JSONL stores session *events* (assistant chunks, tool calls), not outgoing HTTP requests — wire truth for profile #2 must come from a recording proxy at the configured `baseURL` (plus zstd harvest of events), never from source-reading alone. **Security hole:** the Telegram bot token (`123456789:AAH…`) matches no existing redactor pattern and is embedded in file-download URLs — the scrubber must land before the first Telegram HTTP call. **The generalized v1.0 lesson:** no feature closes with stub-only evidence; every phase touching an external surface carries a real-binary/live-service gate (`ASSGUARD_OPENSPEC_BIN=1` in P1, real-log re-capture in P2, live Telegram round-trip in P3, live DeepSeek tool-call probe in P4).

Key risks and mitigations: prompt injection via repo-shipped command markdown riding the autocontinue engine (keep pattern-matching scoped to assistant-role text, record provenance on expanded turns, hooks stay config-authored never markdown-authorable); substitution semantics diverging from zcode — the target wins over Claude Code wherever they differ (`` !`cmd` `` dynamic shell and `${ARGUMENTS}` braces are zcode-rejected non-goals; pin the contract with a table-driven edge-case test before implementing); audit wiring forked into divergent copies (single-source the capturer through the provider factory seam — the copy-`tracerProvider` shortcut is explicitly banned); Telegram lifecycle coupling (shared core builder, one drain path for SIGTERM and stdin-EOF, chat-scoped session IDs so one Session never serves two frontends). The phase structure all research supports: **kickoff → (audit + re-capture) → Telegram → dsh**.

## Key Findings

### Recommended Stack

From STACK.md: v1.1 is stack-light — two pinned new deps, zero new runtime-mandatory externals; features 2 and 3 need no stack at all (internal wiring + operator action). Static-binary / CGO_ENABLED=0 / no-daemon / no-port constraints survive everywhere (klauspost zstd is pure Go; whisper.cpp stays an out-of-process subprocess).

**Core additions:**
- `go-telegram/bot` **v1.23.0** — Telegram peer; zero-dep, idiomatic `context.Context`, `Start(ctx)` long-poll blocks until cancel (context-first drain); stdout-silence verified in Phase 0. Webhook mode banned (opens a port).
- `klauspost/compress/zstd` **v1.19.2** — decode dsh `session.jsonl.zstd` (concatenated checksummed frames by default — exactly dsh's per-append-batch layout). Avoid v1.18.1 (retracted) and early v1.19.x (arm64 bug). One spike against a real dsh file before building on it.
- STT as an internal interface — `Transcribe(ctx, oggBytes)`; OpenAI default via the **existing** go-openai client (`CreateTranscription`; `gpt-transcribe`/`whisper-1`, 25 MB cap), Groq via base-URL swap, whisper.cpp via `ffmpeg`-transcode + `whisper-cli` subprocess (both config-gated externals, never cgo).

**Confirmed sufficient / corrected:**
- `gopkg.in/yaml.v3` handles all real command frontmatter (zcode flat ⊂ YAML; opsx flow arrays parse) — gaps are in our code, not the library; add a flat-parser fallback on unmarshal error for zcode parity.
- OpenSpec v1.5.0 surface captured **from the installed binary**: `apply`/`implement` are phantom commands ("apply" is the `instructions apply` artifact surface); the workflow is command-file-driven with the binary as supporting tooling; `--json` on most read commands; `archive` needs `--yes` non-TTY. npm main is already 1.9.0 — pin the adapter to the installed binary's probe, never to docs.

### Expected Features

From FEATURES.md:

**Must have (table stakes):**
- `/namespace:name` expansion with **zcode semantics** — `$ARGUMENTS`/`$1..$N` (out-of-range → empty), args-without-placeholder appended under "User arguments:", flat single-line frontmatter (6 recognized keys), `:` namespacing, zcode discovery precedence, dynamic shell rejected; unknown `/foo` falls through as plain text.
- OpenSpec core profile E2E driven with zero manual continues; adapter pinned to the installed binary; `ASSGUARD_OPENSPEC_BIN=1` gate green; 11 deferred UAT checks closed.
- Audit on every path including `acp serve`: correlation IDs, secrets always redacted, append-only JSONL with caps — plus engine-decision events (why the agent continued/ran a hook/waited), which no comparable records.
- Telegram: chat↔session binding with resume (stable `tg-<chatID>` session IDs), long-poll with drain, allowlist access control, 4096 chunking + MarkdownV2 escaping, voice → STT → ordinary user input, `/opsx:*` drivable from chat.
- dsh profile #2: captured not hand-written (recording-proxy wire truth + event-log harvest), pinned dsh commit, model→profile availability via config, drift check extended.

**Should have (differentiators):**
- Zero-continue `/opsx:*` stage chaining — the headline value no comparable has.
- In-process Telegram peer beside ACP stdio (one engine, same sessions, no daemon) — every comparable runs a separate daemon supervising CLI processes.
- Audit doubling as parity evidence (per-turn shape fingerprints make drift visible in production).
- Cross-harness mimicry as a platform thesis (2 profiles, one shaper) — the acceptance test is "no zcode-specific paths".

**Defer (v1.1.x / v2+):** forum-topic threading; STT fallback chain beyond config; OTLP export; raw-body opt-in audit mode; expanded OpenSpec profile E2E; dsh Responses-API shape (only if verification demands it); Claude-Code command≡skill merge semantics (`context: fork`, stacking); TTS replies; approval buttons — **rejected**, they violate the no-confirmation-tier safety model and surface-independence invariant; webhook mode — **rejected**, violates no-port.

### Architecture Approach

From ARCHITECTURE.md: nothing rewrites a v1.0 component; every feature is a new leaf package or wiring inside existing seams. Expansion hooks at the **turn runner** (`sessionTurnRunner.Run` → `internal/runtime`), NOT the ACP handler — keeps `internal/acp` protocol-pure and gives Telegram slash-commands for free. The transcript remains the primary audit artifact (D-20 one-writer); the flat `--audit-log` file is an optional operator-facing mirror. Profile #2 is data, not code — a `profiles/dsh/` bundle under the profile-agnostic loader.

**Major components:**
1. `internal/runtime` (NEW) — extracted turn core (runner, engine/hook/MCP wiring) shared by both frontends; the one structural refactor.
2. `internal/ecosys` (MODIFIED) — one-level subdirectory discovery (`commands/<ns>/<name>.md` → `ns:name`), richer frontmatter, pure `Expand`; its first non-test importer.
3. `internal/openspec` (MODIFIED) — `seeded.toml` reconciled to the probed binary surface; Adapter-backed `Execute` closures on registered tools; pattern table re-seeded from real handoff texts.
4. `internal/telegram` + `internal/stt` (NEW) — bot loop, per-chat binding, chunk sink (rate-limit aware), `/stop` cancellation, voice download + Transcriber interface.
5. `internal/provider` / `internal/profile` (MODIFIED) — OpenAI `buildRequest` maps `profile.System` → system message(s); loader tolerates absent `thinking.json`/`tool_choice.json`; `cmd/extract-profile` gains a dsh mode.
6. Audit wiring (MODIFIED) — shared captured-provider helper in the factory (both shapes), per-session TranscriptWriter on the serve path, optional AuditLogger sink, `CurrentTurnID()` accessor.

### Critical Pitfalls

Top pitfalls from PITFALLS.md (18 total, all phase-mapped):

1. **Validating against a stand-in** (the v1.0 stub lesson, generalized) — make real-dependency runs phase gates, not optional extras: real openspec binary (P1), real-log re-capture (P2), live Telegram round-trip (P3), live DeepSeek probe (P4). Rule: no feature closes with only stub-path evidence.
2. **The ecosys namespace discovery gap** — `discoverCommands` skips directories; `/opsx:*` is structurally invisible today. Fix is the FIRST task of Phase 1, with a real-fixture test from actual `openspec init` output; colon-join keys or `opsx/explore.md` collides silently with top-level files.
3. **Prompt injection via command markdown riding autocontinue** — repo-shipped `.claude/commands/*.md` is attacker-controllable prompt content in a no-confirmation agent. Keep pattern-matching assistant-role only (regression test), record provenance on expanded turns, never sanitize/fence bodies (mimicry), hooks stay config-authored.
4. **Substitution semantics divergence** — implement the zcode contract exactly (append-heading, empty out-of-range positionals, brace form NOT recognized, `` !`cmd` `` REJECTED, single-pass `strings.ReplaceAll`, substitution inside code fences); table-driven edge-case test written before the implementation.
5. **Divergent audit wiring + unbounded growth** — copy-pasting `tracerProvider` into `acp_serve.go` creates drifting provider-construction sites; single-source the capturer through the factory seam. Verbatim ~80 KB requests per turn × hands-off multiplication = hundreds of MB/day; adopt the body_ref pattern (hash in the event, full body in a capped store), loud-but-never-fatal write failures.
6. **Telegram token redaction hole + lifecycle coupling** — the bot-token shape matches no redactor pattern and rides download URLs; add the regex + canary test before the first HTTP call. Editor close kills the combined-mode bot mid-turn (defined behavior, documented); telegram-only mode must reuse the shared core builder, not a copied `runACPServe`.
7. **dsh extraction traps** — source-reading is a guess wearing a costume (logs win); the wire protocol comes from the capture, not docs; `strict: true` and DeepSeek dialect quirks keyed by capability profile, never `if provider == "deepseek"`; and zcode-isms in shared code (the "12 identity headers" rule in `internal/redact` etc.) must be audited and genericized first.

## Implications for Roadmap

Suggested phase structure — matches the operator priority chain and the dependency analysis; all four research files agree.

### Phase 1: Slash-command kickoff (ecosys discovery + expansion + OpenSpec reconciliation)
**Rationale:** The milestone's product proof; everything else is downstream. Also carries the milestone's only structural blocker.
**Delivers:** ecosys namespaced discovery (first task — Pitfall 4) + real-fixture test; `Expand` with zcode substitution semantics (edge-case test first); expansion wired at the turn runner so the transcript records the expanded body; `seeded.toml` rebuilt from the installed binary's probe table (read-only vs mutating; `apply`/`implement` removed; `view`/`workset open`/`config edit` never called); Adapter-backed `Execute` on `openspec:*` tools; `triggerFromSignal` stage vocabulary (post-explore/post-propose/post-apply/post-archive); interactive-subprocess guards (`OPEN_SPEC_INTERACTIVE=0`, nil stdin, per-command timeout, exit-code classification); shadow warnings + provenance.
**Addresses:** Area 1 table stakes + the zero-continue-chaining differentiator.
**Avoids:** Pitfalls 1–7 (stub-vs-real gate enforced here; injection provenance; substitution contract; namespace gap; silent shadowing; flat-parser trap; rename-not-remodel).
**Gate:** `mise ci`; operator-gated `ASSGUARD_OPENSPEC_BIN=1` against real openspec v1.5.0 (happy/fixable-failure/missing-binary paths); 11 deferred UAT checks; a real `/opsx:explore → propose → apply → archive` E2E in a scratch project.

### Phase 2: Operational gaps — audit on `acp serve` + zcode parity re-capture (merged; adjacent)
**Rationale:** Both are small; the operator chain allows them to share a phase. Sequencing BEFORE Telegram is the one judgment call: the shared capturer seam lands in its final home before Phase 3's runtime extraction moves the code — written once, not re-wired. (Both orderings satisfy the operator's stated constraints; the roadmapper should phase this deliberately.)
**Delivers:** Factory-seam capturer (`BuildWithCapturer`-style; both shapes) with the tracer path refactored onto it (deleting `tracerProvider`'s reconstruction); per-session TranscriptWriter on the serve path; optional `--audit-log` mirror via `openAuditSink` verbatim (0600, stdout-rejecting); `CurrentTurnID()` accessor; serve-path audit integration test asserting a redacted `RequestShaped` line; audit growth policy (body_ref + caps). Re-capture: operator runbook (fresh rollout dir, scripted divergence-prone workload — subagents, MCP attach/detach, tool variety; NOT richest-session), pinned session ID consumed by the stability test, zcode + extractor versions in meta, drift report committed before the profile update, parity thresholds re-baselined explicitly. Header-capture redaction discipline.
**Addresses:** Area 2 table stakes + engine-decision events + parity-fingerprint differentiator; Area 3 stability-test unblock.
**Avoids:** Pitfalls 8, 9 (headers), 10, 17, 18.
**Gate:** `mise ci`; redacted audit file verified on a live serve; stability test green against the newly pinned session; token/secret canary greps clean.

### Phase 3: Telegram peer (text + voice STT)
**Rationale:** Needs the Phase 2 seam and the runtime extraction; gets slash-commands for free because expansion lives in the session layer. Telegram before dsh per the operator chain.
**Delivers:** `internal/runtime` extraction (mechanical move — prerequisite, done here); `internal/telegram` frontend (long-poll `Start(ctx)`, per-chat stable session IDs, rate-limit-aware chunk sink with fence-aware 4096 splitting + MarkdownV2 escaping, `/stop` cancellation); `internal/stt` Transcriber (OpenAI default via existing client, Groq base-URL swap, whisper.cpp subprocess config-gated); `telegram` cobra subcommand + `acp serve --telegram` sidecar; `deleteWebhook` at startup; one-drain-path shutdown (SIGTERM and stdin-EOF identical; drain < 5s); **bot-token redactor + canary before the first HTTP call**; allowlist; async STT ack; `voice` AND `audio` update handling; engine-`ask` rendered as a Telegram message.
**Addresses:** Area 4 table stakes + the in-process-peer differentiator + full SDD from Telegram.
**Avoids:** Pitfalls 9 (token shape), 11, 12, 13; anti-features (no approval buttons, no webhook, no TTS).
**Gate:** `mise ci`; a full SDD scenario driven from a Telegram chat (text); a voice message transcribed and driving a turn (live round-trip — Pitfall 1's rule); SIGTERM/stdin-EOF drain test; stdout byte-clean in both modes.

### Phase 4: deepseek-harness mimicry profile #2
**Rationale:** Last per the operator chain; benefits from Area 3's capture-provenance work made generic; needs the OpenAI system-mapping fix anyway.
**Delivers:** First task — zcode-ism audit (`grep -ri zcode internal/ cmd/`; genericize/parameterize, e.g. redaction's preserved-header list becomes profile-supplied); OpenAI adapter `profile.System` mapping (form follows the captured dsh wire, not assumption); loader tolerance for absent Anthropic-ism files; dsh capture via **recording proxy at the configured baseURL** (wire truth) + zstd event-log harvest (klauspost spike first); `profiles/dsh/` bundle + sanitized seed; extract-profile dsh mode; `profile check dsh` per-profile capture loader; scheduling.yaml DeepSeek provider entry; explicit `--profile dsh` selection (per-turn profile switching stays a v1.2 extension — do not introduce it silently here).
**Addresses:** Area 5 table stakes + the cross-harness-mimicry thesis validation.
**Avoids:** Pitfalls 14, 15, 16; anti-features (no HEAD tracking — pin the commit; no UI/plugin-runtime mimicry; never assume Anthropic-shape).
**Gate:** `mise ci`; zstd spike passed against a real `session.jsonl.zstd`; A/B parity harness (`internal/parity`) green against a DeepSeek endpoint; `profile check dsh` green; one live DeepSeek tool-calling round-trip per routed model; every profile entry traceable to a captured request.

### Phase Ordering Rationale
- **Kickoff first:** the UAT-closed OpenSpec loop is the product's proof; Areas 4–5 depend on its wiring surface-agnosticity (Telegram gets `/opsx:*` only because expansion lives in the session layer).
- **Audit + re-capture merged second:** both small, both operator-adjacent; landing the capturer factory seam before the Phase-3 runtime move avoids re-homing the wiring (write-once). Flag: PROJECT.md's compressed chain (`1→4→3→5→2`) reads Telegram immediately after kickoff — if the operator prefers that, Phase 2's audit work must be re-homed in Phase 3's extraction with zero functional change; the recommended order is 1→2→3→4.
- **Telegram third:** runtime extraction is its mechanical prerequisite; shares the capturer seam and the drain path; the token redactor lands here at the latest.
- **dsh last:** verification-gated; reuses the genericized capture provenance from Area 3; the OpenAI system-mapping fix is generic and could land earlier if convenient, but the profile itself closes last.
- **Cross-phase invariants:** `mise ci` clean; stdout = ACP frames only; no daemon, no port, static binary; `.claude/` strictly read-only; real-binary/live-service evidence in every gate.

### Research Flags
Phases likely needing deeper research during planning:
- **Phase 4 (dsh):** the only phase with genuine unknowns left — the recording-proxy capture runbook (baseURL swap against a real dsh install), the zstd decode spike, the exact system-message mapping form (one joined message vs multiple — a ground-truth question answerable only from the capture), and DeepSeek dialect quirks (`strict: true`, param rejection) against the captured wire. Recommend `/gsd:plan-phase --research-phase 4` or an explicit capture spike task at phase start.
- **Phase 2 (re-capture runbook):** not library research but operator-procedure design — the divergence-prone workload spec, session pinning, and re-baselining steps should be written into the phase plan verbatim from Pitfalls 17/18.

Phases with standard patterns (skip research-phase):
- **Phase 1:** ground truth is already captured in-repo (installed-binary surface table in STACK.md, zcode semantics from the shipped diagnostics skill, real `openspec init` fixture layout) — plan directly against it.
- **Phase 3:** library and API behaviors are fully documented (go-telegram/bot usage, Bot API limits, STT endpoints); the pitfalls file enumerates the minefield with mitigations — no additional research needed.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Both new deps verified against releases + Go module proxy (v1.23.0 / v1.19.2); OpenSpec surface captured from the installed binary itself. One MEDIUM: zstd against real dsh files (named spike). |
| Features | HIGH | Slash-command semantics triple-sourced (Claude Code docs, zcode local ground truth, real opsx files); audit norms from official OTel docs. MEDIUM: Telegram comparables (community projects) and dsh user-surface (developer preview). |
| Architecture | HIGH | Every integration point names the real package/type/function, verified against source; the namespace gap and no-Execute gap verified live. MEDIUM only for Telegram/dsh internals to be validated during build. |
| Pitfalls | HIGH | Grounded in this repo's actual v1.0 code and the Phase-4 UAT root cause; external facts (DeepSeek `strict`, Telegram limits, OpenSpec interactive mode) source-verified. |

**Overall confidence:** HIGH — this is incremental work on a shipped, well-instrumented codebase, with the two lowest-confidence items (dsh wire capture, zstd decode) reduced to named spikes rather than open questions.

### Gaps to Address
- **dsh wire-truth capture:** no recording-proxy run has happened yet; the capture runbook and profile content depend on it. Handle as Phase 4's opening task/spike, not as more desk research.
- **zstd against real dsh artifacts:** one decode spike before building the harvest tool (MEDIUM → HIGH).
- **OpenAI system-message mapping form:** mimic however dsh itself sends it — decide from the capture, not assumption.
- **openspec version drift:** installed v1.5.0 vs npm 1.9.0 — the adapter pins the installed binary's probe and records the version; note the re-probe procedure for upgrades.
- **telegram-only mode vs "no standalone CLI surface"** (PROJECT.md Out of Scope): explicit tension flagged in STACK.md — resolve deliberately at Phase 3 planning, not via a hack.
- **TurnID on `RequestCapturer`:** cheapest fix is the `CurrentTurnID()` accessor; acceptable v1.1 fallback is empty TurnID (ordering preserved by append) — decide in Phase 2 planning.
- **Per-turn profile switching:** deferred to v1.2 by design; Phase 4 keeps `--profile` explicit — record as a non-goal so it isn't half-built.

## Sources

### Primary (HIGH confidence)
- This repo's source (all packages read: `internal/{acp,session,engine,openspec,ecosys,audit,redact,profile,provider,…}`, `cmd/ass-guard/*`, `go.mod`, `profiles/zcode/`) — integration points, verified gaps.
- Live probe 2026-08-14: installed `openspec` v1.5.0 (`--help` per command; `openspec init --tools claude` scratch run → real opsx command/skill layout; `openspec/config.yaml` contents).
- Local `zcode-guide/diagnosing-commands` skill (shipped by the mimicry target) — discovery order, name regex, flat frontmatter, substitution semantics.
- dsh sources (raw.githubusercontent.com, master): `llm-deepseek/adapter.ts` + `serialize.ts` (wire shape quote-level), `core/system-prompt` (runtime-composed — why source-only fails), `session-persistence-jsonl` (zstd framing, events-not-requests).
- GitHub releases + Go module proxy: `go-telegram/bot` v1.23.0, `klauspost/compress` v1.19.2; `go doc` against pinned `sashabaranov/go-openai` v1.42.0.
- Claude Code official docs: slash-commands/skills semantics; OTel monitoring (event names, correlation IDs, redaction norms, 60 KB caps, body_ref).

### Secondary (MEDIUM confidence)
- Telegram Bot API behavior (4096-after-entities, 409 webhook conflict, 429 `retry_after`, voice = OGG/Opus ≤50 MB vs STT 25 MB, `getFile` URL mechanics); OpenAI + Groq speech-to-text docs.
- DeepSeek API docs (`strict: true` tool schema validation, JSON-mode truncation, reasoner param quirks); OpenSpec CHANGELOG/CLI docs (interactive-mode env, exit-code semantics, 1.5.0→1.9.0 surface movement).
- Comparables: `RichardAtCT/claude-code-telegram` (session binding, throttled streaming, whitelist, audit-to-SQLite); Claude Code Channels plugin (voice fallback chain, forum topics, pairing, daemon supervisor — the contrast that defines ass-guard's in-process differentiator).

### Tertiary (LOW confidence)
- Community issue trackers for Telegram limit behaviors (node-telegram-bot-api issues, StackOverflow 429 threads) — consistent but anecdotal; validated by comparables' resolved patterns.
- VentureBeat/x-cmd pages on dsh — positioning and mode facts, superseded by direct source reading.

---
*Research completed: 2026-08-14*
*Ready for roadmap: yes*
