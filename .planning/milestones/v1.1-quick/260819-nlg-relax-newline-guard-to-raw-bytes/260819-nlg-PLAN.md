---
quick_id: 260819-nlg
slug: relax-newline-guard-to-raw-bytes
date: 2026-08-19
status: complete
---

# Quick Task 260819-nlg: Relax the ACP decoded-newline transport guard to the raw-byte check

## Task

Operator disposition (WINDOWS.md #1/#2, 12-01 live witness session `d9f98023`): the ACP Writer's
decoded-newline transport guard (`internal/acp/framer.go` `containsDecodedNewline`, v1.0) silently
DROPS any frame whose decoded text carries a literal `\n` — the drain goroutine swallows the write
error (`_ = writeFrame`). Live witness: 450 transcript chunks vs 440 wire frames; the model's own
newline-carrying text chunks vanish between the bus and the editor (missing text/code fragments in
the client).

**Disposition: relax the guard to the raw-byte check.** The spec's "MUST NOT contain embedded
newlines" (transports.md, VERIFIED-FACTS #3) is a WIRE-BYTES framing rule: one frame = one line.
`json.Marshal` escapes `\n` inside strings to the two-byte `"\\n"` sequence, so a decoded newline can
never split the frame on the wire. Decoded text with `\n` is legal JSON content and is exactly what
the model's streamed chunks carry. The raw-byte check (`bytes.ContainsRune(raw, '\n')`) after marshal
is the actual invariant and stays.

## Tasks

1. **Rewrite the framer tests (TDD red first)** — replace `TestWriteFrameRejectsDecodedNewline`
   with `TestWriteFrameAllowsDecodedNewline`: decoded newlines (map field, nested slice,
   RawMessage param, and the production `agent_message_chunk` shape) must be emitted as ONE
   single-line frame that round-trips with the newline intact; the raw-newline byte check keeps its
   rejection (a custom `Marshaler` emitting a raw 0x0A must error and write nothing).
   - files: `internal/acp/framer_test.go`
   - action: rewrite the one test + add the raw-byte rejection test
   - verify: new tests fail against current code (red), pass after task 2 (green)
   - done: commit

2. **Relax the guard in `internal/acp/framer.go`** — remove `errFrameEmbeddedNewline`,
   `containsDecodedNewline`, `walkDecodedNewline` and the pre-marshal decoded-newline check; keep
   the post-marshal raw-byte check; rewrite the `writeFrame` doc comment to state the corrected
   spec reading (decoded newlines legal; raw wire bytes must stay line-clean). Drop now-unused
   imports (`slices`, `strings`).
   - files: `internal/acp/framer.go`
   - action: remove the decoded-newline machinery, keep the belt-and-suspenders raw-byte check
   - verify: `mise run ci` green (vet + lint + build + race)
   - done: commit

3. **Update stale rationale comments** — `internal/coreexec/ask.go` `RenderAskSurface` header cites
   the guard as the reason for the single-line render; the single-line convention STAYS (documented
   default for the corpus-absent UI form) but must not cite a guard that no longer drops frames.
   Same for the pin comment in `internal/coreexec/ask_test.go`.
   - files: `internal/coreexec/ask.go`, `internal/coreexec/ask_test.go`
   - action: comment-only edit
   - verify: `mise run ci` green
   - done: commit

4. **Close the ledger + record the outcome** — mark WINDOWS.md #1 and #2 `fixed` (both duplicate
   this finding) with `resolved_at`; flip the deferred-items.md 12-01 entry to resolved; add the
   quick-task row to STATE.md "Quick Tasks Completed"; write SUMMARY.md.
   - files: `.planning/WINDOWS.md`, `.planning/phases/12-product-functional-completeness/deferred-items.md`, `.planning/STATE.md`, this dir's SUMMARY.md
   - action: ledger close-out + state update
   - verify: frontmatter counts match table rows (`open_count` 3 → 1, `fixed_count` 0 → 2)
   - done: commit

## Deviations / notes

- `gsd-tools windows` subcommand is absent in this install (gsd-sdk exposes run/auto/init/query
  only) — the ledger is edited directly, keeping the frontmatter counts and embedded JSON in sync.
- Spikes `03-acp-handshake` / `05-stdout-collision` carry their own historical copies of the guard —
  throwaway archaeology, untouched.
- The single-line `RenderAskSurface` is deliberately KEPT (harmless once the guard is relaxed;
  reverting to multi-line is a separate cosmetic call with no corpus evidence for the client-side form).
- Committed on `gsd/v1.1-acp-early-adoption` (the active milestone branch — the original 12-01 fix
  `1b38e3d` and all phase work live there; origin/master is behind), matching the 12-01 close-out
  line. Executed in-process (no planner/executor agents) per the house precedent for this runtime.
- **Executed as planned with two execution deviations:**
  1. **Pre-existing CI flake fixed as verification collateral** — `TestAskWiring_ServerLevelSurface`
     (the 12-01 witness pin) repeatedly blew its 5s deadline under full-suite race load (6.4–8.6s
     observed, reproduced on baseline WITHOUT this task's changes via `git stash`). Deadline widened
     to 30s in a dedicated atomic test commit; assertion logic untouched (ordering-based, not
     timing-based). Recorded in the SUMMARY for operator visibility.
  2. **The `ruvnet/open-claude-code` deep-dive requested by the operator was ALREADY present** in
     `.planning/research/ECOSYSTEM-AUDIT.md` (an external 2026-08-19 amendment covering both
     `ruvnet/open-claude-code` and `Gitlawb/openclaude`). This task's research verified it against
     live GitHub API + README (stars 477, MIT, JS, 25 tools / 6 permission modes / 4 MCP transports /
     nightly upstream tracking all match) — no duplication written; the amendment was left
     uncommitted for its author.