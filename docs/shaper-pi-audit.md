# pi↔shaper cross-validation audit (EARLY-04)

Cross-validation of `internal/shaper` (ass-guard's mimicry wire chokepoint) against
pi's Anthropic-Messages wire layer, read from source at a pinned commit. pi is the
**behavioral spec to DIFF against** — never a content source: every disposition is
resolved against the capture-authority hierarchy **zcode corpus > in-repo
justification > pi behavior** (redline 8; a pi behavior zcode never exhibits is a
JUSTIFIED divergence — ass-guard matches the target, not pi).

## Provenance

- **Pin:** `496185f6e4267b979e3663c45f7eb70b0c6a97b4` (resolved HEAD of the shallow
  clone; `git -C tools/pi-audit rev-parse HEAD`).
- **Clone command:** `git clone --depth 1 https://github.com/earendil-works/pi tools/pi-audit`
  (planning-time verified reachable; re-cloning at the default branch may move HEAD —
  the recorded pin is the authority, and re-derivation at this pin is deterministic:
  `git -C tools/pi-audit fetch --depth 1 origin 496185f6e4267b979e3663c45f7eb70b0c6a97b4 && git -C tools/pi-audit checkout 496185f`).
- **Clone dir:** `tools/pi-audit/` — gitignored, never vendored, never committed.
  Read/diff only: no code from the clone is built, imported, or executed (T-14-SC).
- **Audit date:** 2026-08-19.
- **pi sources read (at the pin):**
  - `packages/ai/src/api/anthropic-messages.ts` (1,391 lines) — the wire layer.
  - `packages/ai/src/api/transform-messages.ts` (223 lines) — message normalization.
  - `packages/ai/test/cache-retention.test.ts`, plus the thinking/compat/oauth test
    files cited per row — pi's own tests are the spec oracle (T-14-10).
- **License:** pi is MIT (`tools/pi-audit/LICENSE`, © 2025 Mario Zechner). Idea-level
  study and behavioral diffing are unambiguous; the no-mechanical-porting discipline
  (citations are file:line references, never pasted code) is a project redline, not a
  license constraint.
- **ass-guard side cited at:** the shaper file:line as of each task's commit
  (Task 1 commit for the cache_control rows; later tasks extend the table).
- **Corpus grounding:** 14-02's committed census (`docs/compaction-decision.md`,
  228 records, live 2026-08-19 sessions): `cache_control {"type":"ephemeral"}` on
  EVERY `request.body.system` block (910/910 placements; other bucket empty), zero
  on tools/message content. **Corpus wins on any conflict with pi** (stated here and
  consumed by 14-04's cache probe).

## Classification vocabulary (locked)

`match` · `divergence-fixed` · `divergence-justified` (in-repo rationale) ·
`divergence-routed` (with destination) · `absent-at-pin` — exactly one per row, no
unclassified rows, no silent skips (rows whose pi symbol is absent/renamed at the pin
get `absent-at-pin` with the searched locations noted).

## Audit table

Format: `Row ID | Dimension | pi behavior (file:line at pin) | ass-guard behavior (file:line) | Classification | Disposition/Action`.

### Dimension 1 — cache_control placement (Task 1)

| Row ID | Dimension | pi behavior (file:line) | ass-guard behavior (file:line) | Classification | Disposition/Action |
|---|---|---|---|---|---|
| CC-1 | cache_control on system[] blocks | EVERY system block carries `cache_control` when retention ≠ none: OAuth path stamps the identity block AND the systemPrompt block; non-OAuth stamps the single systemPrompt block (`getCacheControl` packages/ai/src/api/anthropic-messages.ts:60-74; stamping at :1009-1034). Oracle: default `{type:"ephemeral"}` on `system[0]` (packages/ai/test/cache-retention.test.ts:73); `cacheRetention:"none"` omits it (:188). | Shaper emits NO cache_control anywhere: system blocks are built as `anthropic.TextBlockParam{Text: b.Text}` only (internal/shaper/shaper.go:99-102), and `profile.TextBlock` carries no captured cache value (internal/profile/types.go:43-47). 14-02 census: the TARGET emits `{"type":"ephemeral"}` on every system block (910/910). | divergence-routed | Known divergence (14-02 disposition #1). Fix is NOT shaper-local — it requires the profile-bundle format change (TextBlock gains the captured value, TIER-1) before the shaper can map it onto `anthropic.TextBlockParam.CacheControl` (field exists in SDK v1.63.0, message.go:342). Routed to the **post-adoption queue** per 14-02's committed disposition; 14-04 consumes this row's placement facts only (probe-only, emission explicitly out of its scope). No in-14-03 fix. |
| CC-2 | cache_control on tools declarations | `cache_control` on the LAST immediate tool only, gated by compat `supportsCacheControlOnTools` (default true); deferred tools never carry it (`convertTools` packages/ai/src/api/anthropic-messages.ts:1326-1363, cache at :1360; gate at :1041-1059). | `toToolUnions` emits `anthropic.ToolParam` with Name/Description/InputSchema only — no CacheControl (internal/shaper/shaper.go:279-300). 14-02 census: the TARGET puts ZERO cache_control on tools declarations (0 across 228 records). | divergence-justified | ass-guard matches the TARGET, not pi: the corpus never exhibits pi's last-tool breakpoint. Corpus wins (capture hierarchy); no action. |
| CC-3 | cache_control on the last user message (history breakpoint) | After conversion, the LAST block of the last message gets `cache_control` when that message is role=user and the block is text/image/tool_result; string content is upgraded to a block array to host it (`convertMessages` packages/ai/src/api/anthropic-messages.ts:1295-1317). Oracle: last user message's last block carries `{type:"ephemeral"}` (packages/ai/test/cache-retention.test.ts:211-214). | `toMessageParams` renders text/tool blocks with no cache_control attachment (internal/shaper/shaper.go:165-221; text blocks at :199-203). 14-02 census: the TARGET puts ZERO cache_control on message content blocks (other bucket empty, 228 records). | divergence-justified | ass-guard matches the TARGET: the corpus never exhibits a conversation-history breakpoint. Corpus wins; no action. |
| CC-4 | cache_control value form + retention knobs | Value is always `{type:"ephemeral"}`, optionally `ttl:"1h"` when retention=long AND compat `supportsLongCacheRetention`; retention "none" disables entirely; `PI_CACHE_RETENTION` env is the legacy switch (`resolveCacheRetention`/`getCacheControl` packages/ai/src/api/anthropic-messages.ts:46-74). Oracles: ttl 1h (:96), compat-false omits ttl (:164), none omits cache_control (:188). | No value form exists — ass-guard emits nothing (zero cache_control matches in internal/shaper). The captured form the eventual fix must emit is `{"type":"ephemeral"}` exactly (14-02: all 910 placements, no ttl observed anywhere in the corpus). | divergence-routed | Rides CC-1: the routed emission fix is corpus-pinned to the bare ephemeral value; pi's ttl/retention/env knobs are pi-isms with no target counterpart (corpus shows only the bare value) and must NOT ride the fix. |

_Row CC-1 is the EARLY-03/14-04 input: the probe's expected placement baseline is
"system blocks only, every block, value `{"type":"ephemeral"}`" — this audit's rows
cross-checked with 14-02's corpus evidence, corpus wins on conflict (stated in both
docs)._

### Dimensions 2-5 (Task 2 — pending)

thinking-config mapping, header merge order, compat-flag catalog, and
transform-messages message-shape mapping: rows appended by Task 2 in this same
table format with both-side citations and final classifications.
