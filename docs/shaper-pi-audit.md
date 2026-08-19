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

### Dimension 2 — thinking-config mapping (Task 2)

| Row ID | Dimension | pi behavior (file:line) | ass-guard behavior (file:line) | Classification | Disposition/Action |
|---|---|---|---|---|---|
| TH-1 | thinking param construction source | pi CONSTRUCTS the config from options + model metadata, gated on `model.reasoning`: budgeted `{type:"enabled", budget_tokens: X\|\|1024, display:"summarized"}` (packages/ai/src/api/anthropic-messages.ts:1081-1085); adaptive `{type:"adaptive", display}` + `output_config:{effort}` (:1069-1078). Oracle: anthropic-thinking-disable.test.ts:144-145 (adaptive+effort), :116 (disabled). | `toThinking` REPLAYS the captured profile JSON: decode `{type, budget_tokens}` → enabled/disabled SDK unions; anything else → empty union (internal/shaper/shaper.go:365-392). The pinned capture carries NO thinking config (profiles/zcode/thinking.json is empty) so none is emitted — pinned unmodified by TestFidelity_ToolCountAndThinking (internal/shaper/fidelity_test.go:99-103). | divergence-justified | Capture-faithful by design (D-06 TIER-1): the profile is the content source; pi constructs because it has no captured wire form to replay. Known latent edge (not capture-visible today — corpus shows zero thinking configs): `toThinking` drops fields beyond type/budget_tokens (pi's `display`) and returns an empty union for unknown types (pi's `adaptive`); a future capture carrying those forms re-opens this row through the parity machinery. No in-phase fix. |
| TH-2 | disabled form | `{type:"disabled"}` emitted only when `thinkingEnabled===false` AND `model.reasoning` AND `thinkingLevelMap.off !== null`; some models omit it entirely (Fable 5) (packages/ai/src/api/anthropic-messages.ts:1087-1089). Oracle: anthropic-thinking-disable.test.ts:123 (disabled sent), :137 (omitted for Fable 5). | `{"type":"disabled"}` → `anthropic.ThinkingConfigDisabledParam` (internal/shaper/shaper.go:385-388); emission is profile-gated — no thinking JSON, no param (the captured state). | match | The disabled mapping agrees verbatim; both sides emit the bare `{"type":"disabled"}` wire form when configured. No action. |
| TH-3 | temperature interplay + budget arithmetic | `temperature` only when `!thinkingEnabled && compat.supportsTemperature` (packages/ai/src/api/anthropic-messages.ts:1036-1039); budget clamped to `maxTokens-1024` minimum 0 via `adjustMaxTokensForThinking` + `clampMaxTokensToContext` (:856-870). | No temperature field is ever emitted (the Profile carries none — internal/profile/types.go:22-40); `MaxTokens` is the captured value passed straight through (internal/shaper/shaper.go:114-122). The captured requests carry no temperature (profile artifact has no such field). | divergence-justified | Target-driven: the captured wire form has no temperature and no client-side budget arithmetic — those are pi's construction-time concerns. Replaying them would add un-captured structure. No action. |

### Dimension 3 — header merge order (Task 2)

| Row ID | Dimension | pi behavior (file:line) | ass-guard behavior (file:line) | Classification | Disposition/Action |
|---|---|---|---|---|---|
| HD-1 | assembly precedence | 4-layer chain, later wins via `Object.assign`: pi `User-Agent` + fixed defaults (accept, anthropic-dangerous-direct-browser-access, anthropic-beta list) < session-affinity < provider config (`model.headers`) < per-call (`optionsHeaders`) (`mergeHeaders`/`mergeClientHeaders` packages/ai/src/api/anthropic-messages.ts:274-286; default chain :949-960; OAuth chain adds claude-code identity headers :925-946). | Single source: the profile's captured Headers, applied in captured order as one `option.WithHeader` each (internal/shaper/shaper.go:124-127); duplicate names would resolve last-entry-wins by SDK option application order — the captured set has 12 unique names (profiles/zcode/identity.yaml). | divergence-justified | The captured header set IS the target's complete identity-header wire form (TIER-2 names, D-09). ass-guard deliberately has no provider-config/per-call layers to merge — the mimicry target's headers are fully captured; pi's layering exists because pi serves N providers from config. No action. |
| HD-2 | header VALUE rendering | Static per-provider values; UA is `pi (<platform> <release>; <arch>)` (packages/ai/src/utils/pi-user-agent.ts:19-21); dynamic values only for special providers (copilot, packages/ai/src/api/github-copilot-headers.ts). | `RenderHeaderValue` templates (internal/shaper/shaper.go:137-156): auth-bearing → `Bearer <ZAI_API_KEY>` from env; id-bearing → fresh UUIDv4 per request; else the trimmed template body. Values are TIER-3 per-session ephemera, never stored verbatim (T-01-02). | divergence-justified | Presence (TIER-2) is the load-bearing property; the captured values are per-session ephemera. Matching pi's value rendering here would be an identity LEAK (pi's UA announces pi; ass-guard's renders the captured stand-in) — the opposite of mimicry. No action. |
| HD-3 | planning-time symbol check: `transformHeaders` | No `transformHeaders` symbol exists at the pin. Searched: packages/ai/src/api/anthropic-messages.ts (full read), `grep -r transformHeaders tools/pi-audit/packages/ai/src` → zero hits. The actual header-assembly symbols are `mergeHeaders`/`mergeClientHeaders` (:274-286) + `createClient`'s `defaultHeaders` (:877-971); the payload mutation hook is `onPayload` (:565-569). | n/a (ass-guard symbol named in planning only) | absent-at-pin | Recorded per the no-silent-skips rule: the plan's symbol guess resolved to `mergeHeaders`/`mergeClientHeaders`, audited as HD-1. No action. |

### Dimension 4 — compat-flag catalog (Task 2)

pi's wire-governing compat flags (`getAnthropicCompat` packages/ai/src/api/anthropic-messages.ts:183-196; catalog packages/ai/src/types.ts:649-706). ass-guard's equivalent knobs are the profile fields themselves — one captured wire form per profile, no N-provider matrix. Each flag with no ass-guard counterpart and no corpus-visible effect is justified (target-driven).

| Row ID | Dimension | pi behavior (file:line) | ass-guard behavior (file:line) | Classification | Disposition/Action |
|---|---|---|---|---|---|
| CF-1 | `supportsEagerToolInputStreaming` (default true) | Per-tool `eager_input_streaming: true`, else the `fine-grained-tool-streaming-2025-05-14` beta header for tool requests (types.ts:655-661; anthropic-messages.ts:1356, :1322-1324, :175). | Tool decls are the captured schema replayed verbatim — no such field exists in the capture (internal/shaper/shaper.go:279-300; profiles/zcode/tools.json). | divergence-justified | The captured tool decls carry no `eager_input_streaming`; emitting it would add un-captured wire structure. No action. |
| CF-2 | `supportsLongCacheRetention` (default true) | `cache_control.ttl:"1h"` when retention=long (types.ts:662; anthropic-messages.ts:69). | Rides CC-4: no cache_control value exists; the corpus shows only the bare ephemeral form (all 910 placements). | divergence-justified | Rides CC-1/CC-4 (routed); the ttl knob is a pi-ism with no target counterpart. No separate action. |
| CF-3 | `sendSessionAffinityHeaders` (default false) | `x-session-affinity: <sessionId>` header for replica-routed providers (Fireworks) (types.ts:663-670; anthropic-messages.ts:949-950). | The target's own session header `x-session-id` is directly captured and rendered per-session (profiles/zcode/identity.yaml; internal/shaper/shaper.go:124-127). | divergence-justified | Different header, directly captured: ass-guard needs no affinity routing knob — the captured identity set already carries the target's session header. No action. |
| CF-4 | `supportsCacheControlOnTools` (default true) | Gates the last-tool cache_control breakpoint (types.ts:671-676 preceding doc; anthropic-messages.ts:1048, :1360). | Rides CC-2: no cache_control on tools, matching the target (corpus: 0 tool placements). | divergence-justified | Rides CC-2. No action. |
| CF-5 | `supportsTemperature` (default true) | Gates the temperature request field (types.ts:671-676; anthropic-messages.ts:1037). | Rides TH-3: no temperature is ever emitted. | divergence-justified | Rides TH-3. No action. |
| CF-6 | `forceAdaptiveThinking` (default false) | Forces `{type:"adaptive"}` + `output_config.effort` regardless of model id (types.ts:677-687; anthropic-messages.ts:845-851, :1067-1078). | Rides TH-1: no thinking config is emitted for the pinned capture. | divergence-justified | Rides TH-1. No action. |
| CF-7 | `allowEmptySignature` (default false) | Replays empty-signature thinking blocks as `signature:""` instead of converting to text (types.ts:688; anthropic-messages.ts:1230-1244). | Rides TM-3: the captured message form carries no thinking blocks at all. | divergence-justified | Rides TM-3. No action. |
| CF-8 | `supportsStrictTools` (default false) | `strict: true` + strict JSON schemas on tool decls (types.ts:689; anthropic-messages.ts:1337, :1346-1351). | Captured schemas round-trip verbatim — first-class keys + `ExtraFields`, no strict flag added (internal/shaper/shaper.go:306-363; the capture's tools.json carries no `strict`). | divergence-justified | The captured decls carry no strict marker; adding one would rewrite captured schema. No action. |
| CF-9 | `allowedFallbackModels` + server-side-fallback beta | `fallbacks` request param + `server-side-fallback-2026-07-01` beta (types.ts:692-699; anthropic-messages.ts:175-177, :179-181, :1107-1110). | No counterpart — the captured requests carry no fallbacks param (profile fields, internal/profile/types.go:22-40). | divergence-justified | The target never sends fallbacks on the wire; not capture-visible. No action. |
| CF-10 | `supportsToolReferences` (model-dependent default) | Deferred tools via `tool_reference` blocks in tool results + `defer_loading:true` (types.ts:701-706; default anthropic-messages.ts:203-210; use :983-994). | No counterpart: the target's 79-tool catalog is fully immediate — every request carries the whole catalog (internal/shaper/shaper.go:109; catalog census pinned since the 09-03 re-pin). | divergence-justified | The target sends all tools every request (corpus-pinned catalog stability — 14-02/09-04 census: stable tool-set across requests); deferral is a pi context-economy feature the target does not use. No action. |
| CF-11 | OAuth/stealth identity mode | OAuth tokens switch on Claude-Code mimicry: `claude-code-20250219`+`oauth-2025-04-20` betas, `user-agent: claude-cli/2.1.75`, `x-app: cli`, identity system block "You are Claude Code…", CC-canonical tool-name casing (anthropic-messages.ts:76-113, :925-946, :1009-1024). | Identity is not a mode — it IS the product: the captured 12 identity headers + captured system blocks + captured tool names ship in the profile (profiles/zcode/identity.yaml; internal/shaper/shaper.go:99-102, :124-127). | divergence-justified | Convergent design, different mechanism: pi stealth-mimics Claude Code for OAuth endpoints exactly the way ass-guard capture-mimics zcode. ass-guard's identity is always-on and corpus-sourced, not token-triggered. Validates the approach; no action. |

### Dimension 5 — message-shape mapping, transform-messages.ts (Task 2)

| Row ID | Dimension | pi behavior (file:line) | ass-guard behavior (file:line) | Classification | Disposition/Action |
|---|---|---|---|---|---|
| TM-1 | mid-conversation system messages | No mid-conversation system role exists in pi's model — system travels only as the `systemPrompt` param; `transformMessages` passes user/assistant/toolResult through with no system case (packages/ai/src/api/transform-messages.ts:77-155). | Maps mid-conversation system-role messages to user-role text blocks — the claude-code-compat convention (internal/shaper/shaper.go:267-276) — because the CAPTURED zcode request.messages carry inline role=system entries (08-07 fixture; 14-02 census: 260 inline system messages across 228 records). | divergence-justified | Target-driven: the captured form is a message class pi's Context model cannot express. ass-guard matches the capture, not pi. No action. |
| TM-2 | tool_use/tool_result pairing + grouping | Groups consecutive toolResult messages into ONE user message `[...toolResults, ...siblingContent]` (comment names z.ai Anthropic-endpoint compat) (packages/ai/src/api/anthropic-messages.ts:1266-1291); SYNTHESIZES `"No result provided"` isError results for orphaned tool calls (packages/ai/src/api/transform-messages.ts:158-223; oracle transform-messages-copilot-openai-to-anthropic.test.ts:136, :162). | Same grouping: consecutive tool-role messages → one user param, one tool_result per message in order (internal/shaper/shaper.go:171-189; lone results get their own param, TestMidTurn_AnthropicToolResultLone). NO synthetic results — pairing is an engine-level invariant over real provider IDs (internal/shaper/shaper.go:46-56; TestPairingInvariant_CaptureFixture). | divergence-justified | The grouping half MATCHES (both emit the canonical batch form; the captured normalized granularity is preserved by block order). The synthesis half is rejected: the corpus carries zero orphans (08-09: 579 same-turn toolCallId persistences, no repair markers) and inventing tool results would fabricate content the target never sent — anti-fidelity. No action. |
| TM-3 | thinking-block replay | Same-model keep (with signature), cross-model → plain text, empty dropped, redacted dropped cross-model (packages/ai/src/api/transform-messages.ts:100-117; oracle transform-messages-copilot-openai-to-anthropic.test.ts:50). | The shaper Message carries no thinking blocks — the captured zcode-normalized message form is `{role, content, toolCalls}` only (internal/shaper/shaper.go:35-44, the 08-07 VERIFIED-FACTS #1 shape). | divergence-justified | Nothing to replay: the captured request.messages form has no thinking block class. If a future capture shows one, TM-3 re-opens with corpus grounding. No action. |
| TM-4 | image content | Base64 image blocks in user/toolResult content; non-vision models get placeholder text downgrades (packages/ai/src/api/transform-messages.ts:12-57). | Text-only: content renders as text blocks (internal/shaper/shaper.go:199-203, :224-226); the captured normalized messages are text-only. | divergence-justified | The captured corpus carries zero image blocks; no downgrade behavior to mirror. No action. |
| TM-5 | tool-call ID normalization | `normalizeToolCallId`: `[^a-zA-Z0-9_-]` → `_`, slice 64 — for CROSS-PROVIDER migrated ids (OpenAI 450+ chars) (packages/ai/src/api/anthropic-messages.ts:1115-1118, wired :136-144). | IDs pass through verbatim — the real provider ID is the join key that pairs tool_use↔tool_result (internal/shaper/shaper.go:46-56, :212). | divergence-justified | ass-guard never migrates sessions across providers; rewriting captured IDs would break the pairing invariant and diverge from the capture. No action. |
| TM-6 | errored/aborted assistant messages | Drops assistant messages with stopReason error/aborted ENTIRELY before replay (packages/ai/src/api/transform-messages.ts:189-197). | No equivalent filter; what replays is governed by the Projector's capture-pinned windowing (64-tail, D-01/D-02 lean seed) upstream of the shaper (internal/session/projector.go, per 08-09/14-02). | divergence-justified | Architecture difference: pi repairs its own session history; ass-guard replays the target's captured conversation state. The corpus shows no dropped-turn markers. No action. |
| TM-7 | assistant batch block order | Preserves stored content order (text/thinking/toolCall as stored) (packages/ai/src/api/anthropic-messages.ts:1207-1260). | Optional text block FIRST, then one tool_use block per call in batch order (internal/shaper/shaper.go:196-214) — exactly the captured zcode-normalized shape `{content, toolCalls:[…]}` (08-07 VERIFIED-FACTS #1; pinned by TestMidTurn_AnthropicToolUseRendering). | match | Both render text-before-tool_use for the captured vocabulary; ass-guard's order is corpus-pinned. No action. |
| TM-8 | empty-content message skipping | User messages with empty/whitespace content are skipped (string or block forms); all-empty assistant batches are skipped (packages/ai/src/api/anthropic-messages.ts:1170-1206, :1261). | Blocks render verbatim — a text-only message with empty Content would emit an empty text block (internal/shaper/shaper.go:197-203); no skip rule exists. | divergence-justified | Not capture-visible: the corpus shows zero empty-content messages (prompts are non-empty; the Projector never synthesizes empty user messages) and the capture exhibits no skip behavior to mirror — inventing one without corpus evidence would be a guess. No action; flagged for re-check if a future capture ever shows the target skipping/keeping empty content. |

## Fix list (Task 3 walk — executed)

**Zero in-phase fixes.** The Task-3 walk over the table found no
`divergence-fixed` rows and no `(pending Task 3)` rows: the two routed rows
(CC-1/CC-4) follow 14-02's committed disposition (profile-bundle format change →
post-adoption queue; 14-04 probe-only); every other divergence is justified by the
capture-authority hierarchy (the target's own wire form differs from pi's, or the
behavior class is absent from the corpus); the remaining rows match. An empty fix
list is the audit's legitimate result (plan 14-03 Task 3): the shaper's five audited
surfaces already agree with the CAPTURED target form — pi's divergences here are
pi's N-provider generality, which ass-guard deliberately does not replicate.

Walk verification (2026-08-19): `go test ./internal/shaper/... -count=1` green;
`mise run ci` green (exit 0); `git diff` over go.mod/go.sum,
internal/shaper/shaper.go, internal/shaper/shaper_test.go, and
internal/shaper/fidelity_test.go across the plan's commits is EMPTY — zero source
changes, zero dependencies added, the fidelity battery unmodified. No
`internal/shaper/testdata/pi-audit/` fixtures exist because no fix required
grounding.

## Reproducibility

Re-running the clone+read at the pinned commit yields the identical audit input:
`git clone --depth 1 https://github.com/earendil-works/pi tools/pi-audit && git -C
tools/pi-audit rev-parse HEAD` → must equal the Pin above (or fetch the pin
explicitly per the Provenance section); then read the four cited source paths and
the cited test files. Every claim above is a file:line citation at that commit —
no run of pi code is required or performed (T-14-SC).
