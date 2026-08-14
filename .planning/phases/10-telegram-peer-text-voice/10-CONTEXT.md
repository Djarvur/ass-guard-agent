# Phase 10: Telegram Peer (Text + Voice) - Context

**Gathered:** 2026-08-14
**Status:** Ready for planning

<domain>
## Phase Boundary

A user drives the same engine from a Telegram chat — full SDD scenarios over text and voice, on the turn core ACP uses (`internal/runtime` extraction is this phase's prerequisite refactor), with disciplined shutdown. The in-process peer beside ACP stdio (one engine, same sessions, no daemon) is the differentiator no comparable offers.

</domain>

<decisions>
## Implementation Decisions

### Launch mode (the flagged tension, resolved)
- **D-01:** **Both modes ship**: `ass-guard telegram` (standalone subcommand) AND `acp serve --telegram` (sidecar beside the editor), both reusing the shared core builder — never a copied `runACPServe`. The subcommand is positioned as a **launch MODE for the Telegram interface**, not a terminal REPL; PROJECT.md's Out-of-Scope line is amended accordingly ("no standalone terminal REPL" — launch modes for the two interfaces are fine). *[User-selected — resolves the STACK-flagged tension deliberately, not via a hack.]*

### Session lifecycle
- **D-02:** Engine context boundaries handle resets naturally (mutating commands open boundaries per Phase-8 D-11); a **`/new` chat command forces a fresh session** when wanted. Stable `tg-<chatID>` session IDs with resume otherwise (locked by TG-02). *[User-selected.]*

### Output streaming
- **D-03:** **Live throttled streaming** — the bot streams the turn's output in throttled in-place message edits (rate-limit aware with `retry_after` handling, 429-respecting), spilling to new messages at the 4096 fence-aware boundary. Chat-like liveness during multi-minute turns; the comparables' pattern. *[User-selected.]*

### Voice UX
- **D-04:** **Ack + auto-sent transcript**: immediate ack message while transcribing, then the transcript is sent AS the user message and the turn proceeds immediately — the transcript doubles as confirmation (you see what was heard), with `/stop` available if it's wrong. No Confirm/Edit buttons (rejected anti-pattern — confirmation-tier violation). *[User-selected.]*

### Claude's Discretion
- Throttle interval/strategy details, edit-coalescing heuristics, MarkdownV2 escaping edge cases
- Allowlist mechanics (config source and format) within TG-03's gate requirement
- `/stop` and `/new` command parsing details; engine-ask message rendering format
- `internal/runtime` extraction mechanics (mechanical move — planner sequences it as the phase's first work)
- STT backend config surface (within the Transcriber interface contract)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### v1.1 research (ground truth, 2026-08-14)
- `.planning/research/SUMMARY.md` — Phase-10 section (runtime extraction prerequisite, capturer-seam inheritance from Phase 9)
- `.planning/research/STACK.md` — go-telegram/bot v1.23.0 (long-poll, `GetFile`→`FileDownloadLink`→download, Start(ctx) blocking), STT backends (gpt-transcribe via existing client, Groq swap, whisper.cpp subprocess + ffmpeg transcode caveat)
- `.planning/research/PITFALLS.md` — Pitfall 9 (bot-token redaction BEFORE first HTTP call), 11 (lifecycle coupling), 12 (limits: 4096-after-entities, 429, voice OGG/Opus), 13 (stdout discipline)

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` §"Telegram Peer" — TG-01..06
- `.planning/ROADMAP.md` §"Phase 10" — goal, 5 success criteria, phase gate (mise ci + live Telegram text+voice round-trip + drain test + stdout byte-clean)

### Prior decisions that bind
- v1.0 Phase 0 VERIFIED-FACTS item #5 — go-telegram/bot stdout-silence + context-first shutdown (2s drain), handler signatures (`ErrorsHandler func(err error)`, `DebugHandler`), mitigation recipe (WithErrorsHandler→slog→stderr, NO WithDebug)
- Phase 8 context D-01/D-11 — session-layer command expansion and command boundaries (Telegram inherits `/opsx:*` from the session layer)
- Phase 9 context — capturer seam landed in `internal/runtime`'s future home (this phase moves the code onto it)

### Code ground truth
- `cmd/ass-guard/acp_serve.go` — `sessionTurnRunner` (the code `internal/runtime` extracts)
- `internal/session` — session IDs, two-layer context (tg-<chatID> joins the family)
- `internal/toolexec` — the Backend seam pattern the Transcriber interface mirrors

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `go-telegram/bot` v1.23.0 — new dep, zero-dep, context-first (Phase-0 verified)
- The existing go-openai client — STT default needs no new dep
- Phase 8's session-layer expansion — Telegram gets `/opsx:*` free
- Phase 9's capturer seam + audit trail — engine-decision events record Telegram-driven turns identically

### Established Patterns
- One engine, N frontends (ACP now, Telegram joins) — shared core builder, no copied construction
- stdout = ACP frames only (Telegram's logging → slog → stderr, per the Phase-0 recipe)
- Loud-but-never-fatal auxiliary behavior; context-first shutdown everywhere

### Integration Points
- `internal/runtime` (NEW) — extracted turn core; both frontends consume it
- `internal/telegram` + `internal/stt` (NEW) — frontend + Transcriber interface
- cobra command surface — `telegram` subcommand + `--telegram` sidecar flag

</code_context>

<specifics>
## Specific Ideas

- User's streaming preference: chat-like liveness during long turns — throttled in-place edits, not silent waits.
- Voice flow is "ordinary user input" end to end: the transcript IS the user message (mirrors Phase-8's expanded-body-as-user-message pattern — what you see is what the model saw).

</specifics>

<deferred>
## Deferred Ideas

- Forum-topic threading — Future Requirements
- TTS replies — rejected anti-feature (Future Requirements notes it as rejected)
- STT fallback chain beyond config — Future Requirements
- Telegram-only mode's process-supervision story (launchd/systemd units) — operator docs, not code

</deferred>

---

*Phase: 10-Telegram Peer (Text + Voice)*
*Context gathered: 2026-08-14*
