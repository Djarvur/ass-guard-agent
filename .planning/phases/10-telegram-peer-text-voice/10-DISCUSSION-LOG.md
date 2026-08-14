# Phase 10: Telegram Peer (Text + Voice) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-14
**Phase:** 10-telegram-peer-text-voice
**Areas discussed:** Launch mode, Session lifecycle, Output streaming, Voice UX

---

## Launch mode

| Option | Description | Selected |
|--------|-------------|----------|
| Both modes | `ass-guard telegram` subcommand + `acp serve --telegram` sidecar, both on the shared core builder; subcommand positioned as a launch MODE (not a REPL); Out-of-Scope wording amended | ✓ |
| Sidecar-only | Telegram exists only beside ACP; editor owns lifecycle (Telegram dies with the editor session) | |
| Subcommand-only | No combined mode in v1.1 | |

**User's choice:** Both modes

---

## Session lifecycle

| Option | Description | Selected |
|--------|-------------|----------|
| Boundaries + /new | Engine boundaries handle resets naturally; `/new` chat command forces a fresh session when wanted | ✓ |
| Boundaries-only | One eternal session per chat; no explicit reset path | |

**User's choice:** Boundaries + /new command

---

## Output streaming

| Option | Description | Selected |
|--------|-------------|----------|
| Live throttled streaming | Throttled in-place message edits, rate-limit aware, spills to new messages at 4096 fence-aware — chat-like liveness | ✓ |
| Turn-final only | Quiet chats; long silent waits during multi-minute turns | |
| Config-toggled | Streaming default, quiet configurable | |

**User's choice:** Live throttled streaming

---

## Voice UX

| Option | Description | Selected |
|--------|-------------|----------|
| Ack + auto-sent transcript | Immediate ack; transcript sent AS the user message; turn proceeds; transcript doubles as confirmation; /stop if wrong | ✓ |
| Ack + confirm buttons | NOTE: approval buttons are a rejected anti-pattern (confirmation-tier violation) | |
| Silent | No ack or transcript display until turn output | |

**User's choice:** Ack + auto-sent transcript

---

## Claude's Discretion

- Throttle interval, edit-coalescing, MarkdownV2 escaping edge cases
- Allowlist mechanics within TG-03
- /stop and /new parsing; engine-ask rendering format
- internal/runtime extraction mechanics; STT config surface

## Deferred Ideas

- Forum-topic threading — Future Requirements
- TTS replies — rejected anti-feature
- STT fallback chain — Future Requirements
- Process-supervision units for telegram-only mode — operator docs
