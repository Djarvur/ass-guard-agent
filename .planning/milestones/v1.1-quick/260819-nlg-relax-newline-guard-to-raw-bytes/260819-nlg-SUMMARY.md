---
quick_id: 260819-nlg
slug: relax-newline-guard-to-raw-bytes
date: 2026-08-19
status: complete
commits:
  - f5f5b52
  - d6fa490
---

# Quick Task 260819-nlg — Summary

**Task:** Operator disposition on WINDOWS.md #1/#2 (12-01 live witness `d9f98023`): the ACP
Writer's decoded-newline transport guard silently dropped the model's own newline-carrying text
chunks from the live wire. Relax the guard to the raw-byte check.

**Result:** COMPLETE — the guard is relaxed; model text chunks with line breaks now reach the
editor; full `mise run ci` green (vet + lint 0 + build + race, exit 0).

## Detailed разбор

### What was wrong

The ACP transport guard in `internal/acp/framer.go` (`writeFrame`, line 34, since v1.0) refused to
emit ANY frame whose **decoded** JSON text contained a literal newline (`\n`). The ACP spec quote
behind it — *"Messages MUST NOT contain embedded newlines"* (transports.md) — was read as applying
to the decoded message content.

That reading is wrong. The rule is a **wire-bytes framing rule**: the transport is
newline-delimited, so each frame must be exactly one line of bytes. `json.Marshal` already escapes
`\n` inside strings to the two-character sequence `\\n`, so a decoded newline can **never** split
the frame on the wire. A decoded newline is legal, ordinary JSON string content — and it is exactly
what the model's streamed chunks carry (paragraphs, code, lists).

### Why it was silent

The Writer's drain goroutine swallows write errors: `_ = writeFrame(w.w, msg)`. Every rejected
frame vanished between the event bus and the client with no log line and no error. The 12-01 live
witness measured the effect: **450 chunks in the transcript vs 440 on the wire** — 10 chunks with
newlines dropped (9 of the model's own text chunks + the multi-line ask surface, pre-fix). The
editor displayed missing fragments of text and code.

### What changed

| File | Change |
|------|--------|
| `internal/acp/framer.go` | Removed `containsDecodedNewline`, `walkDecodedNewline`, `errFrameEmbeddedNewline`. The pre-marshal decoded-newline rejection is gone; the post-marshal raw-byte check (`bytes.ContainsRune(raw, '\n')`) stays — that is the actual wire invariant. Doc comment rewritten to the corrected spec reading. |
| `internal/acp/framer_test.go` | `TestWriteFrameRejectsDecodedNewline` → `TestWriteFrameAllowsDecodedNewline`: decoded newlines (map field, nested slice, RawMessage param, and the production `agent_message_chunk` drop class) must emit ONE single-line frame; exactly one trailing `\n`; no raw newline in the body; round-trip preserves the newline. Added `TestWriteFrameRejectsRawNewlineBytes`: a hostile `Marshaler` emitting a raw 0x0A still errors and writes nothing. |
| `internal/coreexec/ask.go` + `ask_test.go` | Stale rationale comments corrected — the single-line `RenderAskSurface` convention is retained as the documented default (corpus-absent UI form), no longer citing a guard that drops frames. |
| `cmd/ass-guard/ask_wiring_test.go` | Verification collateral: widened the 5s deadline of `TestAskWiring_ServerLevelSurface` to 30s (see below). |

### Evidence

- TDD: the rewritten tests failed red against the old guard, green after the fix.
- The spec reading was verified empirically before committing: a custom `Marshaler` emitting a raw
  `0x0A` inside a string literal is rejected by the stdlib scanner on compact — confirming the
  raw-byte check is the correct (and only reachable) error surface.
- Full gate: `mise run ci` exit 0 — vet, golangci-lint (0 issues), `CGO_ENABLED=0 go build`,
  `go test -race -count=1 ./...` (all packages ok).

### Pre-existing flake fixed (verification collateral)

`TestAskWiring_ServerLevelSurface` (the 12-01 witness pin) repeatedly blew its 5s response deadline
under full-suite `-race` load (6.4–8.6s observed). Proven pre-existing: reproduced on baseline
WITHOUT this task's changes via `git stash` (8.56s fail), then green in isolation (4.66s) and green
after the widen. Assertion logic untouched (ordering-based, not timing-based) — the deadline only
bounds a hang. Committed separately (`d6fa490`) so the guard fix and the flake fix stay atomic.

### Related (operator request: add ruvnet/open-claude-code)

A detailed deep-dive of **Open Claude Code** (`ruvnet/open-claude-code`, `occ`) — plus the sibling
project **OpenClaude** (`Gitlawb/openclaude`) — is ALREADY present in
`.planning/research/ECOSYSTEM-AUDIT.md` (§1.1 deep dives, amended 2026-08-19, externally authored
during this session). This task verified the key claims against the live GitHub API and the README
— stars 477, MIT, JS/ESM, 25 tools / 40 slash commands / 6 permission modes / 4 MCP transports,
nightly upstream-tracking releases, ruDevolution decompilation provenance, clean-room disclaimers —
all match. No duplication was written; the amendment was left uncommitted for its author. The
audit's highest-value borrow for this project is the **NIGHTLY-PARITY** (ADR-001) pattern: a
re-run gate that fires whenever the tracked target ships — the missing operational half of the
parity harness.

**Files:** `internal/acp/framer.go`, `internal/acp/framer_test.go`, `internal/coreexec/ask.go`,
`internal/coreexec/ask_test.go`, `cmd/ass-guard/ask_wiring_test.go`, this task dir (PLAN + SUMMARY),
`.planning/WINDOWS.md`, `.planning/phases/12-product-functional-completeness/deferred-items.md`,
`.planning/STATE.md`.

**Verification:** `mise run ci` green (exit 0); WINDOWS.md frontmatter counts consistent
(open 1, fixed 2, total 3); deferred-items resolution note appended.