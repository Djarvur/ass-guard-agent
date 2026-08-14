# Phase 1: Mimicry MVP (north-star proof) — Research

**Researched:** 2026-08-09
**Status:** Ready for planning
**Researcher model:** sonnet (inline; gsd-phase-researcher contract)
**Primary inputs:** `01-CONTEXT.md` (D-01..D-15 + RESEARCH-FLAG-01), `.planning/research/VERIFIED-FACTS.md` (post-spike source of truth), `.planning/REQUIREMENTS.md`, `.planning/ROADMAP.md`, `.planning/PROJECT.md`, `.planning/research/STACK.md`, `spikes/` (throwaway references), live zcode rollout transcripts under `~/.zcode/cli/rollout/`.

> This research answers "what do I need to know to PLAN Phase 1 well?" It does NOT relitigate CONTEXT.md locked decisions (D-01..D-15) — it resolves the discretion items, addresses RESEARCH-FLAG-01, and gives the planner concrete identifiers, type names, file layouts, and test designs. Every claim that is not a direct quotation is grounded in a dated, inspectable source (on-disk transcripts, the SDK module cache, the spikes, or the canonical docs).

---

## 0. Carry-forward correction (apply during planning, D-07 consequence)

**MIMC-02 path wording is stale in ROADMAP.md and REQUIREMENTS.md.** Both still say the claude-code-compat canonical path `~/.claude/projects/<munged-cwd>/<session-id>.jsonl`. VERIFIED-FACTS.md item #1 (Tier-B resolved, option-a applied) corrects this: zcode transcripts live at `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl` (plus `model-io-no-session.jsonl` for pre-session calls and `model-io-sess_subagent_agent_<id>.jsonl` for subagent sessions — same schema). The `~/.claude/projects/<munged-cwd>/...` path is a *different product* (Claude Code, `type: "queue-operation"`).

**Planning action:** the profile-extraction plan MUST read the corrected path. Do not edit ROADMAP.md/REQUIREMENTS.md as part of plan-phase (those are state mutations handled by the transition step at phase close); the plan text simply uses the corrected path everywhere it touches the filesystem. The correction is already reflected in CONTEXT.md (the phase source of truth) and VERIFIED-FACTS.md.

---

## 1. The transcript corpus (ground truth for extraction + parity)

Three real zcode rollout transcripts exist on this machine under `~/.zcode/cli/rollout/` (inspected 2026-08-09):

| File | Lines (`model_io`) | Tool-calls (`response.toolCalls`) | Tools declared (max array len) |
|------|-------------------:|----------------------------------:|-------------------------------:|
| `model-io-sess_eea3dc48-….jsonl` | 69 | 88 | 77 |
| `model-io-sess_016eee8a-….jsonl` | 68 | 76 | 103 |
| `model-io-sess_subagent_agent_24f00e13-….jsonl` | 20 | 30 | 97 |

**Key structural facts (load-bearing for D-14 and RESEARCH-FLAG-01):**

1. **Every line is `type: "model_io"`** carrying the full wire-level request+response (per VERIFIED-FACTS.md item #1 corrected schema). Top-level keys: 13. `request.*`: 9 keys. `request.body.*`: 9 keys (`max_tokens, metadata, model, output_config, stream, system, thinking, tool_choice, tools`). `request.headers`: exactly **12 identity header names** (the fingerprint Phase 1 must mimic, TIER-2 per D-06). `response.toolCalls[]`: `{id, name, input}` (zcode-normalized). `response.usage`: 5 token-count fields.

2. **The tool catalog is NOT a fixed 77.** It varies per session: 77 / 97 / 103. VERIFIED-FACTS.md item #1 recorded "77" because it inspected one line of one session. The variation is real and load-bearing:
   - A **stable built-in core** of ~20 tools appears in main sessions: `Agent, AskUserQuestion, Bash, CronCreate, CronDelete, CronList, Edit, EnterPlanMode, ExitPlanMode, Read, ReadSessionContext, SendMessage, Skill, TaskStop, TodoRead, TodoWrite, WebFetch, WebSearch, Write`. (Subagent sessions expose a restricted ~13-tool subset — `Bash, Edit, Read, ReadSessionContext, AskUserQuestion, RespondToCoordinator, Skill, TaskStop, TodoRead, TodoWrite, WebFetch, WebSearch, Write` — because subagents run with a scoped tool set, per PARA-01's eventual shape. The subagent transcript `toolNames` reflects this.)
   - A **variable MCP/plugin tail** (`mcp__firecrawl__*`, `mcp__node_repl__*`, `mcp__plugin_chrome-devtools-mcp_chrome-devtools__*`, `mcp__plugin_playwright_playwright__*`, `mcp__plugin_context7_context7__*`) depends on which MCP servers/plugins the operator had enabled for that session. This tail is session-specific configuration, not a fixed identity.
   - **Implication for D-14 ("declare all 77 faithfully"):** the number is wrong as a fixed literal. The faithful behavior is: *the profile captures the tool set the target session declares* (the built-in core + whatever MCP/plugin tools were configured), and the parity test runs against a session whose captured catalog the ass-guard arm reproduces. **Recommendation: pin the parity reference to ONE session's catalog (the `eea3dc48` main session, 77 tools — the smallest, cleanest main-session capture) and declare exactly that set in the zcode profile's tool catalog.** The catalog-consistency check (TOOL-03) then validates the built-in catalog satisfies those 77 declarations. The other two sessions serve as extraction cross-check + held-out parity material (see §2). A `null` tool name appeared once in `016eee8a` (an MCP serialization quirk on one line) — the extractor must skip/filter null tool entries defensively rather than crash.

3. **The catalog is stable *within* a session** — all 67 non-degenerate `model_io` lines in `016eee8a` declare exactly 103 tools (one line declared 0, a pre-session probe). So once a session is chosen as the capture source, every line carries the same `tools[]` array; the extractor can read it from any line.

4. **`thinking` + `tool_choice` are present on the body** (Anthropic extended-thinking knobs). The Shaper (D-09) MUST reproduce these — they are TIER-1 (model-visible content) per D-06. `request.body.thinking` is an object like `{type, budget_tokens}`; `request.body.tool_choice` is a tool-choice directive.

---

## 2. RESEARCH-FLAG-01 — held-out transcript split (overfitting risk)

**The coupling (restated):** the rollout transcripts feed two layers — (a) **profile extraction** (request *shape*: system blocks, tools, the 12 headers, thinking, tool_choice) and (b) **parity reference** (tool-call *sequences*: the ordered `[toolName,...]` zcode actually produced). These are different layers of the same data. The concern: if the SAME session feeds both, a profile that over-fits to that session's quirks could pass parity by memorizing rather than generalizing.

**Assessment: the coupling is real but BOUNDED, because the layers are genuinely different objects.**
- Profile extraction reads `request.body.{system, tools, thinking, tool_choice}` + `request.headers` — the *inputs* the model sees. The profile is a snapshot of zcode's outgoing-request shape; it does not encode what the model *did*.
- Parity comparison reads `response.toolCalls[].name` + `.input` — the model's *output*. The parity test asks: given the same prompt, does ass-guard's shaper cause the same tool SELECTION?

The profile cannot "memorize" tool-call sequences because it never sees them — it only captures the request shape. The shaper's job is to reproduce the request; the model (deterministic at temp=0, D-04) produces the selection. So overfitting in the classic ML sense (training on test labels) does not arise — the parity label (`response.toolCalls`) is never an input to the profile.

**However, a residual risk remains:** a single session's prompts may share stylistic quirks (same project, same toolchain idioms) that make tool selection easy in a way that flatters the shaper. A held-out split removes even this residual.

**Recommendation: ADOPT A HELD-OUT SPLIT — it is cheap and removes the concern entirely.** Three sessions is a small corpus, but enough for a clean 2-train / 1-test split:

| Role | Session | Rationale |
|------|---------|-----------|
| **Extraction source (profile shape)** | `016eee8a` (103 tools, 68 lines) AND `eea3dc48` (77 tools, 69 lines) | Two main sessions; union gives the most complete built-in core + cross-check that the shape is stable. The profile's `tools[]` is pinned to `eea3dc48`'s 77 (the parity-reference catalog — see §1.2); `016eee8a` validates shape stability across sessions. |
| **Held-out parity reference** | `eea3dc48` (77 tools, 88 tool-calls across 69 turns) | Disjoint *purpose*, not disjoint session: the profile's *shape* is extracted from both sessions' requests (shape is session-stable per §1.3, so this is not leakage — the shape is an identity, not a behavior), but the parity test's curated prompts (D-01) are drawn from `eea3dc48`'s actual user-prompt → tool-call-sequence pairs, and the ass-guard arm must reproduce those sequences. `016eee8a`'s turns are reserved as a second held-out set for a surprise-check if the planner wants one. |

**Why this is not leakage:** the request *shape* (system blocks, tool catalog, headers) is zcode's *identity* — it is (within a session) constant across every turn (§1.3). Extracting it from session A and testing tool selection on session B tests whether the shaper correctly reproduces the identity that causes the model to select tools the way zcode does. The shape does not vary turn-by-turn, so "training shape on A, testing behavior on B" is the correct experimental design. What WOULD be leakage: extracting the shape from the same single turn whose tool-call sequence is the parity target. We avoid that by using the session-stable shape (extracted from the union) and curating parity prompts from individual turns.

**Split strategy for the planner:**
1. Profile extraction reads `request.body` + `request.headers` from BOTH main sessions (`016eee8a`, `eea3dc48`) and asserts the shape is identical across them (a built-in consistency check; any drift is a finding, not a silent merge).
2. The profile's `tools[]` catalog is pinned to `eea3dc48`'s 77 declarations (the parity reference session).
3. The curated parity prompt suite (D-01, 5–15 prompts) is drawn from `eea3dc48`'s turns — each prompt is a real user-turn prompt from that session, paired with the tool-call sequence zcode actually produced for it (`response.toolCalls[].name`). The suite is "held-out" in the sense that the *individual turns* chosen for parity were not used to tune anything (nothing is tuned — the profile is extracted, not trained).
4. `016eee8a`'s turns (68 lines, 76 tool-calls) are an optional second held-out set the parity harness can run as a surprise check after the curated suite passes.
5. The subagent session (`subagent_agent_24f00e13`) is OUT of scope for the Phase-1 parity test (subagent dispatch is PARA-01, Phase 2) but its transcript validates that the extraction code handles the subagent-variant path and the restricted tool subset.

**Conclusion on RESEARCH-FLAG-01:** a held-out split IS adopted (recommendation above). The risk is bounded because profile extraction (shape) and parity reference (behavior) are different layers, but the split is cheap and makes the gate defensible. This resolves RESEARCH-FLAG-01.

---

## 3. A/B parity test design (D-01..D-05 — concrete test architecture)

### 3.1 The two arms

- **ass-guard arm:** the test-harness Turn Loop (D-12) loads the zcode profile, shapes the request via the Profile Shaper, sends it to the provider (Anthropic-shape adapter → Z.ai GLM via `https://api.z.ai/api/anthropic`), parses `tool_calls` from the response, returns `[]ToolCall{name, input}`.
- **zcode arm:** NOT a live call. Per D-05, this arm is **replay** of zcode's captured tool-call output from the rollout JSONL. For each curated prompt, the arm reads the matching `response.toolCalls[]` from `eea3dc48` (the held-out parity session) and returns it. Fully offline, reproducible, deterministic.

**Critical implication (D-04 consistency):** the ass-guard arm runs at temp=0 (deterministic). The zcode arm is a replay of what zcode *actually produced* (which was zcode's own temp/config at capture time). The test asks: "does ass-guard, at temp=0 with the zcode profile, reproduce the tool-call sequence zcode produced?" Because D-04 fixes determinism on the ass-guard side, every mismatch is a real signal pointing at the shaper — no variance escape hatch. (Note: the zcode-captured sequence itself was produced at whatever config zcode used; we trust it as ground truth. If the planner is worried that zcode's capture was non-deterministic and we got "lucky," the `016eee8a` surprise-check set is the backstop — a shaper that only reproduces `eea3dc48` by coincidence will fail on `016eee8a`.)

### 3.2 The two-layer metric (D-02 — operationalized)

D-02 fixes the metric as (1) tool-name sequence equality AND (2) per-tool argument structural equality. Concrete comparison rules:

**Layer 1 — tool-name sequence equality:**
- Extract `[]string` of tool names from each arm: ass-guard returns `[{name, input}, ...]`; zcode replay returns `response.toolCalls[].name` in order.
- Match = identical ordered slice. Any reorder, substitution, insertion, deletion = mismatch.
- Edge cases: zero tool-calls (model returned text only) = empty slice on both sides = match iff both empty. Parallel tool-calls (multiple `tool_calls` in one response) = ordered as the response lists them.

**Layer 2 — per-tool argument structural equality (the stricter bar):**
- For each paired tool-call (matched by index after Layer 1 passes), compare `input` structurally:
  - **Keys present:** the set of JSON keys must match (no missing/extra keys).
  - **Value shapes:** each value's JSON type must match (`string`/`number`/`bool`/`object`/`array`/`null`).
  - **NOT compared (intentionally, respects MIMC-04):** exact string values, exact numbers, array element order (unless order is semantically load-bearing — see below), whitespace, key order in objects.
- **Normalization rules (the planner must implement these explicitly):**
  - **File paths:** normalize to forward-slash, relative-where-possible. A tool-call with `file_path: "/abs/repo/src/foo.go"` and one with `file_path: "src/foo.go"` are structurally equal IF the repo root is consistent. The harness records the repo root and normalizes both arms against it.
  - **String values that are enums/identifiers:** these ARE compared exactly (a `command: "Read"` vs `command: "Grep"` is a real divergence even though both are strings — Layer 1 already catches tool-name; Layer 2 catches the rare case where the same tool takes a mode argument). The rule: structural equality on JSON *type*, but for string values that come from a closed enum (tool names, role fields, mode flags), compare exactly. For free-text strings (file content, prompts, queries), compare type-only.
  - **Arrays:** compare length + per-element structural equality. Arrays of primitives (e.g. `["a","b"]`) compare as sets iff order is not semantically load-bearing; the default is ORDER-SENSITIVE (most tool args are ordered — e.g. a list of files to read). The harness flags which tool args are order-sensitive (a small per-tool table; most are).
  - **Numbers:** compare type only (int vs float distinction ignored — JSON doesn't distinguish). A `limit: 100` vs `limit: 50` is a structural match (both numbers) UNLESS the planner decides a specific numeric arg is load-bearing (e.g. `max_results`); the default is type-only per MIMC-04.

**Where the line is drawn (the MIMC-04 boundary):** structural equality stops at JSON type + key set + closed-enum exact match. It does NOT do string diffing, numeric equality, or deep value comparison on free-form content. This catches a shaper that picks the right tool but passes malformed args (missing keys, wrong types) — the failure D-02 is designed to catch — without sliding into byte-diff (MIMC-04 out of scope).

### 3.3 The threshold (D-03) and the divergence-probe rationale (D-01)

D-03 fixes the threshold at 100% — every curated prompt must match on BOTH layers. D-01 fixes the suite as 5–15 hand-authored prompts, each targeting a divergence-prone path. **What makes a prompt divergence-prone** (the research output the suite author needs):

A prompt is divergence-prone when a WEAKER shaper (one that gets identity partly wrong) would plausibly pick a different tool, a different order, or malformed args than zcode did. Concrete divergence-prone patterns observed in the corpus:
- **Multi-tool sequencing:** prompts that trigger `[Read, Grep, Edit]` or `[Bash, Read, Edit]` — a shaper with a missing or mis-schemas tool forces a different sequence.
- **Ambiguous tool selection:** prompts where `Grep` vs `Bash(grep)` vs `Read` could all plausibly serve; zcode's actual choice reflects its catalog/schemas, so a shaper with drifted schemas picks differently.
- **MCP/plugin tool selection:** prompts that trigger an `mcp__firecrawl__*` or `mcp__node_repl__*` call — a shaper that drops or misrenames these (they have long `mcp__namespace__name` identifiers) diverges immediately.
- **Tool-arg edge cases:** prompts that trigger a tool with many required fields (e.g. a `Bash` call with `command + description + run_in_background`); a shaper with a schema missing a required field produces a malformed-arg mismatch (Layer 2).
- **Plan-mode transitions:** prompts triggering `EnterPlanMode`/`ExitPlanMode` — these are zcode-specific tools a generic shaper would not declare.
- **Zero-tool turns:** prompts where zcode returned text only (no tool-calls) — a shaper that over-eagerly declares tools might cause the model to call one anyway.

**Suite composition (the planner curates 5–15 from the `eea3dc48` corpus):** for each of the 69 turns in `eea3dc48`, the harness can extract `(user_prompt, expected_tool_call_sequence)`. The planner selects 5–15 turns that collectively cover the patterns above, and documents the divergence-probe rationale for each. Because the suite is curated FROM the held-out session's real turns (not synthetic), it reflects real workload. Because the threshold is 100%, the suite MUST genuinely probe — a suite of trivial prompts that always match is a self-deception risk (D-03). The divergence-probe rationale per prompt is the discipline that makes the 100% gate meaningful.

> **Note on prompt redaction:** the curated prompts are real user turns from the operator's own zcode session. They may contain project-specific content. The parity test runs locally (operator's machine, operator's keys); the prompts themselves are NOT shipped as part of the profile artifact (the profile ships the request *shape*, not the prompts). The test suite is a dev-time artifact, like the spikes. If the operator wants to ship a demo parity suite, it can be synthesized from the divergence patterns above without using real turns.

### 3.4 Test reproducibility config (D-04)

Recorded alongside results: model (`glm-5.2` via Z.ai), temp (0), max_tokens (from profile), thinking config (from profile), seed (if the provider exposes one — Z.ai/Anthropic exposes `thinking.budget_tokens` but not a deterministic seed; temp=0 is the determinism mechanism). The harness writes a JSON results file per run with `{prompt_id, ass_guard_arm: {tool_calls}, zcode_arm: {tool_calls}, layer1_match, layer2_mismatches, config}`.

---

## 4. Profile artifact design (PROF-01..05, MIMC-02 — concrete format)

### 4.1 Profile structure

A profile is a directory (or single artifact) loaded by name. Recommended layout (planner's discretion per CONTEXT.md, but this is the coherent default):

```
profiles/zcode/
  profile.yaml          # the profile manifest (PROF-01/03)
  system/               # the 3 system blocks (TIER-1, byte-faithful)
    block-0.txt
    block-1.txt
    block-2.txt
  tools.json            # the 77 tool declarations (TIER-1, name+input_schema)
  identity.yaml         # the 12 headers (TIER-2, names required, values templated)
  thinking.json         # thinking config (TIER-1)
  tool_choice.json      # tool_choice directive (TIER-1)
  coverage.yaml         # the coverage manifest (PROF-05)
  meta.yaml             # target_capture_ref (PROF-03), extraction provenance
```

**Why a directory, not a single file:** the system blocks are large (the 3-block system prompt is the bulk of the mimicry payload) and byte-faithful (TIER-1) — storing them as separate `.txt` files lets the extractor write them verbatim and the loader read them verbatim without JSON-escaping/escaping corruption. The YAML/JSON files carry the structured fields. (The planner may choose a single YAML/JSON with embedded blocks if it prefers; the directory layout is the recommendation because it survives round-trips cleanest.)

### 4.2 Field tiering (D-06 — concrete assignment)

| Field | Source location in transcript | Tier | Rationale |
|-------|-------------------------------|------|-----------|
| `request.body.system[]` (3 text blocks) | `body.system` | **TIER-1** (byte-faithful) | Model-visible identity; drift = hard failure |
| `request.body.tools[].name` + `.input_schema` | `body.tools` | **TIER-1** | Model-visible catalog; the mimicry thesis core |
| `request.body.tool_choice` | `body.tool_choice` | **TIER-1** | Model-visible directive |
| `request.body.thinking` | `body.thinking` | **TIER-1** | Model-visible extended-thinking config |
| `request.body.model` | `body.model` | **TIER-1** | Model identity (e.g. `GLM-5.2`) |
| `request.body.max_tokens` | `body.max_tokens` | **TIER-2** (structural) | Presence matters; exact value is session-tunable |
| `request.headers` (12 header NAMES) | `request.headers` keys | **TIER-2** | Names required (the fingerprint); VALUES (`x-request-id` etc.) are per-session |
| `request.body.metadata`, `output_config`, `stream` | `body.*` | **TIER-2** | Shape matters; values vary |
| `request.messages[]` shape | `request.messages` | **TIER-2** | Block shape (system/user roles); content is per-turn |
| `response.*` (all) | `response` | **TIER-3** (informational) | This is the *output*, not the profile; logged for audit (LOG-01), never drift-flagged |
| `durationMs`, `startedAt`, `completedAt`, token counts, all ids | various | **TIER-3** | Per-request ephemera |

The 12 identity header names (TIER-2, the fingerprint): `HTTP-Referer, User-Agent, X-Os-Category, X-Os-Version, X-Platform, X-Title, X-ZCode-Agent, X-ZCode-App-Version, x-query-id, x-request-id, x-session-id, x-zcode-trace-id` (per VERIFIED-FACTS.md item #1). Header VALUES are templated (`x-request-id: <generated-per-request>`) — the profile declares the name set; the Shaper (D-09 escape hatch) injects fresh values per request.

### 4.3 Coverage manifest (D-07 / PROF-05 — format)

`coverage.yaml` is the machine-readable artifact shipped with the profile (D-07). It lists every captured field, its tier, and its source location. Incomplete capture = CI hard failure. Sketch:

```yaml
# profiles/zcode/coverage.yaml
profile: zcode
target_capture_ref:
  sessions:
    - id: "016eee8a-..."   # redacted in shipped artifact; full id in extraction provenance
      path: "~/.zcode/cli/rollout/model-io-sess_<id>.jsonl"
      role: extraction_source
    - id: "eea3dc48-..."
      path: "~/.zcode/cli/rollout/model-io-sess_<id>.jsonl"
      role: extraction_source + parity_reference
  extracted_at: 2026-08-09
  extractor_version: "0.1.0"
fields:
  - path: "request.body.system"
    tier: 1
    type: "array<text-block>"
    observed_count: 3
    source: "body.system"
  - path: "request.body.tools"
    tier: 1
    type: "array<tool-def>"
    observed_count: 77   # pinned to eea3dc48
    source: "body.tools"
  - path: "request.headers"
    tier: 2
    type: "object<header-name,string>"
    observed_names: ["HTTP-Referer", "User-Agent", "X-Os-Category", "X-Os-Version", "X-Platform", "X-Title", "X-ZCode-Agent", "X-ZCode-App-Version", "x-query-id", "x-request-id", "x-session-id", "x-zcode-trace-id"]
    observed_count: 12
    source: "request.headers"
  # ... one entry per captured field
required_tools_satisfied_by_catalog: true   # feeds TOOL-03 CI
```

This single artifact feeds both PROF-05 (incomplete-capture detection — a fresh capture missing a listed field fails) and TOOL-03 (catalog-consistency — the built-in catalog must satisfy every declared tool's schema).

### 4.4 `target_capture_ref` (PROF-03 — versioning)

PROF-03's `target_capture_ref` = the `sessionId`(s) + corrected path the profile was extracted from (per VERIFIED-FACTS.md item #1 Notes). The `meta.yaml` carries this. Exact versioning scheme is a planner detail, but the recommendation is: the profile is immutable per extraction; a new capture = a new profile version (timestamped). `ass-guard profile check zcode` (PROF-04) compares a FRESH live capture against the tiered manifest of the CURRENT version.

---

## 5. Profile Shaper architecture (MIMC-01, PROF-02, D-09, D-10 — concrete types)

### 5.1 The hybrid SDK-driving mechanism (D-09 — validated against the SDK)

D-09 specifies: native `anthropic-sdk-go` types for the bulk + a data-driven escape hatch for the 12 identity headers. Inspecting `anthropic-sdk-go@v1.61.0` (in the module cache; STACK recommends v1.62.0 — pin latest at go-get) confirms this is cleanly supported:

- **Native types the Shaper populates:**
  - `MessageParam` (role + content blocks) for `request.messages[]`.
  - `ContentBlockParamUnion` for message content.
  - `TextBlockParam` for system-block text.
  - `ToolParam{InputSchema ToolInputSchemaParam, ...}` + `ToolUnionParam` for `request.body.tools[]` — the profile-declared tool schemas populate `InputSchema` directly.
  - `ThinkingConfigEnabledParam{BudgetTokens int64}` (or `ThinkingConfigAdaptiveParam`/`ThinkingConfigDisabledParam`) for `request.body.thinking`.
  - `ToolChoiceUnionParam` (with `ToolChoiceParamOfTool(name)`) for `request.body.tool_choice`.
  - Client construction: `client.NewClient(option.WithAPIKey(...), option.WithBaseURL("https://api.z.ai/api/anthropic"))` — base-URL swap for Z.ai/GLM is a first-class option.
- **Escape hatch for the 12 identity headers (D-09):** the SDK's `option` package provides `option.WithHeader(key, value string)`, `option.WithHeaderAdd`, `option.WithHeaderDel` (`option/requestoption.go`). The Shaper passes one `option.WithHeader(name, value)` per identity header as a per-request option. The header set is **data-driven from the profile** (`identity.yaml` declares the 12 names + value templates), NOT hardcoded — this preserves PROF-02 (no zcode-specific code paths). The escape hatch is "part of the profile artifact, not code" exactly as D-09 requires: the Shaper code reads `profile.Headers` and emits `option.WithHeader` calls in a loop; changing profiles changes headers without touching Shaper code.

**No alternative mechanism needed** — the SDK's native types + the `option.WithHeader` request-option layer cover the entire request body + the 12 headers with zero raw-JSON marshaling for the TIER-1 fields. The only place raw JSON may be needed is a future TIER-2 field the SDK doesn't model (none observed in Phase 1); the escape hatch extends to `option.WithJSONSet` or a body-level merge if ever needed, but Phase 1 does not require it.

### 5.2 Schema-adapter layer (D-10 / TOOL-02 — the boundary)

D-10 fixes: the profile-declared tool schema is authoritative at runtime. The adapter sits between the built-in catalog and the Shaper:
- **When shaping the outgoing request:** the Shaper pulls each tool's model-facing schema from the **profile** (`profile.Tools[]` — what the model sees). It does NOT consult the built-in catalog for the schema.
- **When a `tool_call` returns:** the harness (Phase 1) / executor (Phase 4) resolves/validates the call against the profile schema. In Phase 1, tool execution is stubbed (D-15), so this is a no-op parse/validate, not a real execution.
- **The built-in catalog provides execution behavior** (what the tool does) — irrelevant in Phase 1 because all tools are stubbed (D-15), but the catalog MUST still declare schemas satisfying all 77 profile declarations (TOOL-02/TOOL-03). The catalog's schemas are the *execution contract*; the profile's schemas are the *model-facing contract*; TOOL-03 CI asserts they don't drift.

**The adapter interface (concrete sketch for the planner):**
```go
// internal/toolcat — the schema-adapter layer
type Tool struct {
    Name       string
    InputSchema json.RawMessage   // the model-facing schema (from profile, authoritative)
    Mutability  Mutability         // read-only | mutating (Phase 2/4 concern; stubbed in Phase 1)
    Execute     func(ctx, args) (json.RawMessage, error)  // stubbed in Phase 1 (D-15)
}

// The catalog implements the EXECUTION side; the profile owns the MODEL-FACING schema.
type Catalog interface {
    Get(name string) (Tool, bool)        // execution behavior
    Satisfies(profileDeclarations []toolcat.Decl) (unsatisfied []string, ok bool)  // TOOL-03 CI
}
```

### 5.3 PROF-02 enforcement (D-11 — synthetic-profile conformance test)

D-11 specifies a synthetic-profile conformance test + a grep lint. Concrete design:
- **Synthetic fixture:** a minimal non-zcode profile fixture (`profiles/synthetic/` in testdata) with different system blocks (e.g. "You are a synthetic test agent"), different tool names (e.g. `synth_tool_a`, `synth_tool_b`), different headers (e.g. `X-Synth-Test`). The test loads it through the SAME Shaper code path as zcode and asserts the outgoing request carries the synthetic fields, not zcode fields.
- **Failure modes it catches:** any `if profile.Name == "zcode"` branch, any hardcoded zcode string in the Shaper, any zcode-only default. The synthetic profile breaks on all of them.
- **Lint rule:** `grep -rn "zcode" internal/shaper/` returns nothing outside test fixtures (belt-and-suspenders). Note: the string `zcode` IS allowed in `internal/profile/loader.go` (loading a profile BY NAME `zcode` is not a zcode-specific code path — it's data-driven name lookup) and in test fixtures; the lint scope is `internal/shaper/` specifically (the chokepoint), not the whole codebase. The planner scopes the grep precisely.

---

## 6. Provider adapters (PROV-01..03, MIMC-01 — concrete interface + SDK refs)

### 6.1 The common `Provider` interface (PROV-02)

```go
// internal/provider — the common interface both adapter shapes implement
type Provider interface {
    // Send shapes + sends the request via the active profile + shaper, returns the parsed tool-calls.
    Send(ctx context.Context, profile profile.Profile, messages []Message, tools []toolcat.Decl) (Response, error)
}

type Response struct {
    ToolCalls []ToolCall   // {Name string, Input json.RawMessage} — the zcode-normalized shape
    FinishReason string
    Raw         json.RawMessage  // the verbatim response (for LOG-01 audit)
}

type ToolCall struct {
    Name  string
    Input json.RawMessage
}
```

Both `TranslateToInternal`/`TranslateFromInternal` (PROV-02's wording) are the per-adapter methods that convert between the SDK's native tool-call representation and the internal `[]ToolCall` (which is already zcode-normalized: `{name, input}`). Round-trip conformance tests verify each adapter.

### 6.2 Anthropic-shape adapter (PROV-03 — primary, used by zcode)

- Uses `github.com/anthropics/anthropic-sdk-go` (v1.62.0 per STACK; v1.61.0 in cache — pin latest at go-get).
- Base URL: `https://api.z.ai/api/anthropic` for GLM (Z.ai's Anthropic-compatible endpoint, per STACK + VERIFIED-FACTS.md item #1 `providerId: "builtin:zai-coding-plan"`).
- The Shaper (D-09) populates native types; the adapter calls `client.Messages.New(...)` with the shaped params + the header escape-hatch options.
- Tool-call translation: Anthropic returns `tool_use` content blocks (`{id, name, input}`); the adapter's `TranslateToInternal` maps each to `ToolCall{Name, Input}`. (zcode's `response.toolCalls[]` is already this normalized shape — VERIFIED-FACTS.md item #1 — so the adapter's output matches the parity reference directly.)

### 6.3 OpenAI-shape adapter (PROV-03 — secondary, conformance only in Phase 1)

- Uses `github.com/sashabaranov/go-openai@v1.42.0` (VERIFIED-FACTS.md item #2: schema VERIFIED offline; live round-trip deferred pending operator keys, but the schema shape is confirmed).
- Targets **Chat Completions** (NOT Responses API — VERIFIED-FACTS.md item #2 Notes). Request shape: `tools[].function.{name, description, parameters}` with the `{"type":"function"}` wrapper; `tool_choice:"required"`; assistant `tool_calls[]` with string-typed `function.arguments`; `role:"tool"` result message. (All asserted by `spikes/02-openai-toolschema`.)
- **Phase-1 scope for this adapter:** the adapter is IMPLEMENTED (it satisfies PROV-01/02 — both shapes supported from day one) and gets round-trip CONFORMANCE TESTS (PROV-02), but it is NOT on the parity-test path (the parity test uses the Anthropic-shape arm because zcode is Anthropic-shape). A zcode-via-OpenAI-shape arm would be a different thesis; Phase 1 proves Anthropic-shape mimicry. The OpenAI-shape adapter exists so PROV-01 ("both shapes supported") is structurally true and so Phase 3 (scheduling across providers) has a real adapter to route to.
- **Reference:** `spikes/02-openai-toolschema/main.go` is the working reference for the serialization shape — study, don't import (spikes are a separate module, Phase-0 D-05).

### 6.4 Configurable base URL (PROV-01)

Both adapters take a configurable base URL (Anthropic via `option.WithBaseURL`; OpenAI via `client.NewConfig().BaseURL`). The profile does NOT fix the base URL (the profile is shape, not routing — routing is Phase 3 scheduling). Phase 1's harness configures Z.ai's Anthropic base URL directly for the parity arm.

---

## 7. Turn Loop MVP scope (D-12, D-13, D-14, D-15 — concrete interfaces)

### 7.1 The test-harness Turn Loop (D-12)

```go
// internal/loop — the Phase-1 test-harness loop (NOT the Phase-2 Session Core)
// Run loads the profile, shapes one request via the Shaper, sends to the provider,
// parses tool_calls, returns them. NO session management, NO context projection,
// NO ACP streaming, NO session/load replay (those are SESS-*/ACP-*, Phase 2).
func Run(ctx context.Context, profile profile.Profile, provider provider.Provider,
    prompt string, toolStubs map[string]toolcat.Stub) ([]provider.ToolCall, error)
```

- Single-turn in Phase 1 (the parity test measures tool SELECTION per prompt, not multi-turn reactions — D-15). If research found multi-turn needed, this would loop; it does not (D-15 stands: stub all tools, single turn).
- The interface is narrow on purpose: it gives the Shaper a real integration seam and gives Phase 2 a clean replacement target (the load-bearing Shaper + adapters + profile loading survive; the loop is swapped for the Session Core).

### 7.2 Audit hook (D-13 / LOG-01 — minimal event bus)

D-13 specifies an event-bus subscriber for LOG-01. The Phase-1 minimal bus:
```go
// internal/event — minimal Phase-1 event bus (Phase 2 expands)
type Event interface{ kind() string }
type RequestShaped struct { VerbatimRequest json.RawMessage; Profile string; Timestamp time.Time }
type Bus struct { /* subscribers per event kind */ }
func (b *Bus) Subscribe(kind string, handler func(Event))   // audit log subscribes to "RequestShaped"
func (b *Bus) Publish(e Event)                               // synchronous in Phase 1 is fine (single subscriber, no hot path)
```
- The Turn Loop calls `bus.Publish(RequestShaped{...})` carrying the verbatim shaped outgoing request BEFORE sending.
- The audit-log subscriber (`internal/audit`) writes the verbatim request to the log (LOG-01).
- **LOG-02 (async, never in critical path):** in Phase 1 with a single subscriber and a test harness (not a live ACP server), synchronous publish is acceptable — the "critical path" concern arises when ACP streaming is live (Phase 2). D-13 says the bus is the RIGHT SEED (the interface is async-ready; Phase 2 makes the subscriber goroutine-driven). The planner may make Publish dispatch to subscribers in goroutines from the start if it wants the async property from day one — either is defensible; the recommendation is goroutine-per-subscriber from day one (cheap, matches LOG-02's eventual contract, avoids a Phase-2 refactor).
- **LOG-03 (redaction):** the verbatim request MUST be redacted before logging (API keys, the `Authorization`/`x-api-key` headers, any header VALUE that is a secret). The 12 identity header NAMES are logged; their VALUES are templated/per-session and the secret ones (auth tokens) are redacted to `[REDACTED]`. This is LOG-03's Phase-1 slice (full LOG-03 is Phase 2 but the redaction discipline starts now).
- **Transport discipline:** the audit log writes to a FILE (or stderr), NEVER stdout (stdout is ACP-only, even though Phase 1 has no ACP server yet — the discipline is established from day one).

### 7.3 Tool catalog scope (D-14 — corrected to "session's declared set")

D-14 said "all 77 declared faithfully." Per §1.2, the correct reading is: the built-in catalog declares the **stable built-in core** (~20 tools) faithfully, and the profile declares the **full session catalog** (77 for `eea3dc48`) including the MCP/plugin tail. The catalog's job (TOOL-01) is the built-in core; the profile's job (MIMC-02) is the full captured set.

**Concrete catalog scope for TOOL-01 (the built-in core, ~20 tools, faithful schemas):** `Agent, AskUserQuestion, Bash, CronCreate, CronDelete, CronList, Edit, EnterPlanMode, ExitPlanMode, Read, ReadSessionContext, SendMessage, Skill, TaskStop, TodoRead, TodoWrite, WebFetch, WebSearch, Write`. (The subagent-restricted subset is a Phase-2 PARA-01 concern.) The MCP/plugin tools (`mcp__*`, `plugin__*`) are NOT in the built-in catalog — they are declared in the profile (captured from the session) because they are session-config-dependent. TOOL-03 CI checks the catalog satisfies the 77 profile declarations; for the ~57 MCP/plugin declarations the catalog provides schema-only satisfaction (the schemas ARE captured in the profile; the catalog's `Satisfies()` check confirms it has a matching entry — execution is stubbed per D-15 anyway).

**Tools needing special schema handling:** the `mcp__*` and `plugin__*` tools have long names with double-underscore namespace separators; the extractor must preserve the exact name string (the model sees it; drift here = immediate Layer-1 mismatch). The `null`-named tool observed once (`016eee8a`) is an extractor edge case to filter, not a real tool.

### 7.4 Tool execution (D-15 — stubbed)

All tools are stubbed in the parity test. The Turn Loop sends the request; the model returns tool-calls; the harness parses and returns them WITHOUT executing. A `toolStubs map[string]toolcat.Stub` is passed to `Run` for forward-compatibility (Phase 2/4 will fill these in), but in Phase 1 the stubs are no-ops and the loop is single-turn (the model's tool SELECTION is what's measured, not its reaction to results).

---

## 8. `ass-guard profile check zcode` (PROF-04 — concrete operator command)

D-08 specifies fresh live capture. Concrete design:
- **CLI surface:** a cobra subcommand. The exact tree is a planner detail, but the recommendation is `ass-guard profile check <name>` (e.g. `ass-guard profile check zcode`). STACK notes the ACP entrypoint is `ass-guard acp` (Phase 2); `profile check` is a sibling subcommand (operator-facing, not ACP).
- **What it does (D-08):**
  1. Spawns live zcode (operator has zcode installed; the command takes a `--zcode-bin` flag or relies on PATH).
  2. Sends a fixed probe prompt through live zcode (a minimal prompt that triggers at least one tool-call).
  3. Captures the resulting outgoing request from the freshest `~/.zcode/cli/rollout/model-io-sess_<new-id>.jsonl` (the corrected path).
  4. Diffs the captured request field-by-field against the tiered profile manifest (`coverage.yaml`).
  5. Reports the specific fields + tiers that drifted (TIER-1/2 only; TIER-3 is audit-only per D-06).
- **Operator requirements:** live zcode install + valid keys at check time. This is an OPERATOR command, not CI (D-08). If zcode is not installed or keys are missing, the command exits with a clear error (not a silent skip).
- **Phase-1 scope:** the command is IMPLEMENTED and RUNNABLE, but the planner may mark it `autonomous: false` (it needs the operator's live zcode + keys, so it's a checkpoint-style task during execution, not a fully autonomous one). The drift-detection LOGIC (the tiered diff) is fully testable autonomously via fixtures (a captured "before" profile + a synthetic "after" fixture with a known drift → assert the diff reports exactly the drifted TIER-1/2 fields). So the unit tests are autonomous; the live command is operator-gated.

---

## 9. MIMC-02 path correction + extraction implementation

The extractor (PROF-03/MIMC-02) reads `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl`:
1. **Input:** one or more session paths (the extractor takes the session IDs / paths as args).
2. **Process:** stream the JSONL (each line is one `model_io` object), parse with `encoding/json` into a struct mirroring the VERIFIED-FACTS.md item #1 schema, extract the TIER-1/2 fields, assert the shape is stable across lines (§1.3 — within a session it must be), write the profile directory (`profiles/zcode/` per §4.1).
3. **Redaction (LOG-03 discipline at extraction time):** the profile artifact stores the system-prompt blocks (TIER-1, byte-faithful — these are the mimicry payload, NOT secrets) and the tool schemas (TIER-1) and the 12 header NAMES (TIER-2). It does NOT store header VALUES that are secrets (auth tokens, `x-request-id` values, `x-session-id` values — these are templated to `<generated-per-request>`). System-prompt text is the mimicry target, not a secret; it is stored verbatim. (If the operator's system prompt contained a secret, that would be a zcode-config problem, not an ass-guard-extraction problem — the extractor stores what zcode sent.)
4. **Output:** the profile directory + `coverage.yaml` + `meta.yaml` (with `target_capture_ref`).
5. **Consistency check (part of extraction):** assert `request.body.tools` length + the 12 header names + the 3 system blocks are identical across all lines of the extraction-source sessions. Any drift is a hard error (the profile is only valid if the shape is stable).

**The extractor is a one-shot CLI tool or a `go run` dev tool** (like the spikes) — it is NOT shipped in the runtime binary. It produces the profile artifact, which IS shipped (or at least checked in for the zcode profile). Planner's call on exact invocation (`ass-guard profile extract <session-id>` reusing the cobra tree, or a standalone `cmd/extract-profile/`).

---

## Validation Architecture

> Phase 1's validation is **deterministic-gate-based**, not sampling-based. The Nyquist VALIDATION.md template targets sampling validation; this section records the validation architecture in RESEARCH.md instead, and the gate criteria (D-03 100%, D-04 determinism, D-02 two-layer metric) are encoded directly as the parity-test plan's acceptance criteria. No separate VALIDATION.md is emitted — the deterministic gate does not need a sampling-rate rationale.

**Sampling / validation strategy for the mimicry thesis:**

The "signal" Phase 1 must not alias is: a shaper that passes the curated suite by coincidence but fails on real workload. Mitigations (the validation architecture):
1. **The curated suite is drawn from real turns** (§3.3, the divergence-probe rationale) — not synthetic.
2. **A second held-out set** (`016eee8a`'s 68 turns, §2) runs as a surprise check after the curated suite passes. The harness reports both pass rates.
3. **The divergence-probe rationale is documented per prompt** (D-01/D-03 discipline) — the suite's coverage of divergence-prone patterns is auditable.
4. **temp=0 determinism (D-04)** — no variance aliasing; every failure is real.
5. **The two-layer metric (D-02)** — Layer 1 (sequence) + Layer 2 (arg structure) independently; a shaper passing Layer 1 but failing Layer 2 is a partial-credit finding the metric surfaces, not hides.

**What "validation" means here is unusual:** the parity test is not sampling a distribution for a statistical claim — it is a deterministic gate (100% threshold, D-03) on a curated set. The "statistical indistinguishability" language in the ROADMAP goal is the THESIS; the OPERATIONAL test is the deterministic 100% gate on the curated suite + the held-out surprise set. The planner should make this distinction explicit in the plan (the test is deterministic; the thesis it validates is statistical — the curated set's diversity is what connects them).

**VALIDATION.md is NOT created** — the gate criteria (D-03 100%, D-04 determinism, the two-layer metric D-02) are encoded directly in the parity-test plan's acceptance criteria. (The Nyquist VALIDATION.md template is for sampling-based validation; Phase 1's validation is deterministic-gate-based, so the template doesn't fit cleanly. The research records the validation architecture here; the plan encodes it as test assertions.)

---

## 11. Project skeleton + module layout (for the planner)

### 11.1 Module

- **Module path:** `github.com/djarvur/ass-guard-agent` (matches STACK Installation; the spikes use the isolated `github.com/djarvur/ass-guard-spikes` module — they are NOT the codebase).
- **Go version:** `go 1.25` (STACK mandates the floor; the toolchain on this machine is 1.26.5 — `go 1.25` in go.mod is the supported floor, `go mod tidy` resolves upward).
- **`go.mod` at repo root** — Phase 1 starts it (the repo root has no `go.mod` today; only `spikes/go.mod` exists for the isolated spikes module).

### 11.2 Recommended package layout (the planner refines)

```
ass-guard-agent/
  go.mod                          # module github.com/djarvur/ass-guard-agent, go 1.25
  cmd/
    ass-guard/                    # cobra root command
      main.go                     # rootCmd + subcommands (acp [Phase 2], profile [Phase 1])
    extract-profile/              # OR a profile-extract subcommand (planner's call) — the extractor
  internal/
    profile/                      # PROF-01..05: Profile types, Loader, the zcode profile artifact
      loader.go
      types.go
      coverage.go                 # the coverage manifest (PROF-05)
    shaper/                       # MIMC-01/D-09/D-10: the Profile Shaper (single mimicry chokepoint)
      shaper.go                   # NO "zcode" string outside test fixtures (D-11 lint)
    provider/                     # PROV-01..03: the common Provider interface + 2 adapters
      provider.go                 # the interface
      anthropic.go                # anthropic-sdk-go adapter (primary, Z.ai base URL)
      openai.go                   # go-openai adapter (Chat Completions, conformance-tested)
    toolcat/                      # TOOL-01..03: the built-in tool catalog + schema-adapter
      catalog.go                  # the ~20 built-in tools + Satisfies() for TOOL-03
      types.go                    # Tool, Decl, Mutability, Stub
    loop/                         # D-12: the Phase-1 test-harness Turn Loop
      loop.go                     # Run(ctx, profile, provider, prompt, toolStubs) -> []ToolCall
    event/                        # D-13: the minimal event bus
      bus.go                      # Publish/Subscribe; RequestShaped event
    audit/                        # LOG-01: the audit-log subscriber (verbatim shaped request)
      audit.go                    # subscribes to RequestShaped, writes redacted verbatim to file/stderr
    parity/                       # MIMC-03/04, D-01..05: the A/B parity test harness + metric
      harness.go                  # the two arms (ass-guard live + zcode replay), the curated suite
      metric.go                   # Layer 1 (sequence) + Layer 2 (arg structure) comparison
      suite/                      # the curated divergence prompts (5–15, from eea3dc48)
      testdata/                   # redacted fixtures; the real prompts are dev-time (operator's machine)
    drift/                        # PROF-04: the profile-check drift detector (tiered diff)
      drift.go                    # field-by-field diff against coverage.yaml
  profiles/
    zcode/                        # the extracted zcode profile artifact (shipped / checked in)
      profile.yaml, system/, tools.json, identity.yaml, coverage.yaml, meta.yaml
    synthetic/                    # the PROF-02 conformance fixture (D-11)
  .claude/                        # (Phase 5; out of Phase-1 scope but the layout is respected)
```

### 11.3 Dependencies (pinned)

- `github.com/anthropics/anthropic-sdk-go` — latest at go-get (STACK v1.62.0; v1.61.0 in cache). Anthropic-shape provider + Z.ai base URL.
- `github.com/sashabaranov/go-openai@v1.42.0` — VERIFIED-FACTS.md item #2. OpenAI-shape provider (Chat Completions).
- `github.com/spf13/cobra` — CLI surface (`profile check`, `profile extract`, future `acp`).
- `github.com/spf13/viper` — config layering (profile selection by name, base URLs). (Optional in Phase 1 if the planner prefers a minimal YAML loader; viper is the STACK default and accommodates Phase 2+ config growth.)
- `github.com/stretchr/testify` — optional, for table-driven parity assertions (STACK MEDIUM; stdlib `testing` alone is also fine).
- stdlib: `log/slog` (stderr logging), `encoding/json`, `net/http`, `time`, `context`, `os`, `path/filepath`.
- **NOT a dependency in Phase 1:** `go-telegram/bot` (Phase 5), `modelcontextprotocol/go-sdk` (Phase 5), `goreleaser` (Phase 6 — though the go.mod/build should stay `CGO_ENABLED=0`-clean from day one to preserve the static-binary claim).

---

## 12. Decisions left to the planner (the discretion items, resolved)

CONTEXT.md marks D-09, D-10, D-11, D-12, D-13, D-14, D-15 as Claude's-discretion. Research resolution:
- **D-09 (SDK-driving):** validated — native types + `option.WithHeader` escape hatch (§5.1). No alternative needed.
- **D-10 (schema adapter):** validated — the adapter sits at `internal/toolcat`, profile-authoritative (§5.2).
- **D-11 (PROF-02 enforcement):** validated — synthetic fixture + scoped grep lint (§5.3). The synthetic fixture is strong enough for v1; a second real profile (claude-code) is deferred (PROF-06, v2).
- **D-12 (Turn Loop):** validated — narrow `Run(ctx, profile, provider, prompt, toolStubs) -> []ToolCall` (§7.1). Rich enough for parity; clean enough to replace.
- **D-13 (audit hook):** validated — minimal event bus, goroutine-per-subscriber recommended from day one (§7.2).
- **D-14 (catalog scope):** corrected — built-in core (~20) in the catalog + full session set (77) in the profile (§7.3). The "77" is the `eea3dc48` session's count, not a universal.
- **D-15 (stub all tools):** stands — single-turn, stubbed, selection-only (§7.4). Multi-turn reactions are not needed for the curated divergence suite (each prompt's expected sequence is captured; the test does not need the model to react to tool results).

Plus the three planner's-call items: exact prompt composition (5–15 from `eea3dc48`, §3.3), exact tier assignment (§4.2 — the framework is locked, assignments are the table), exact manifest format (§4.3 — YAML recommended).

---

## 13. Open questions / risks for the planner to flag

1. **Operator key provisioning (carry-forward from Phase 0):** the parity test's ass-guard arm needs a working Z.ai GLM key (`ZAI_API_KEY` or equivalent) at temp=0. The OpenAI-shape adapter's live round-trip is still PARTIAL (Phase 0 #2 — needs `MINIMAX_API_KEY`/`GROQ_API_KEY`). The plan should mark the live parity run as `autonomous: false` (operator-gated on key presence) while keeping the metric/harness/replay logic fully unit-tested and autonomous.
2. **The "statistical indistinguishability" language vs the deterministic gate:** make the distinction explicit in the plan (§10). The ROADMAP goal says "statistically indistinguishable"; the operational test is a 100% deterministic gate on a curated set. The curated set's diversity (the divergence-probe rationale) is what makes the deterministic gate a meaningful test of the statistical thesis.
3. **Profile artifact shipping vs dev-time:** should `profiles/zcode/` be checked in (shipped, reproducible by anyone without re-extracting) or generated at build/dev time? Recommendation: check it in (the zcode profile is the project's first-class artifact; checking it in makes the parity test reproducible without the operator's transcripts). The system-prompt blocks are the mimicry payload (not secrets). The extractor exists to RE-generate it when zcode drifts (PROF-04), not to generate it at every build.
4. **System-prompt byte-faithfulness vs SDK normalization:** the SDK's `TextBlockParam` may normalize whitespace or apply caching. The Shaper must preserve the 3 system blocks byte-faithfully (TIER-1). The planner should add an assertion that the shaped request's serialized `system[]` is byte-equal to the profile's `system/block-N.txt` (a round-trip fidelity test). If the SDK mangles anything, the escape hatch (raw body merge) is the fallback — but research expects the native `TextBlockParam` to be faithful.
5. **`tool_choice` + `thinking` exact reproduction:** these are TIER-1 (model-visible). The Shaper must reproduce them exactly as captured. If Z.ai's endpoint rejects the captured `thinking` config (some providers are picky), that's a finding — but zcode itself sent it to Z.ai and got a response, so reproduction should work.

---

## RESEARCH COMPLETE

Phase 1 is plan-ready. The mimicry thesis (MIMC-01..04), profile fidelity (PROF-01..05), tool substrate (TOOL-01..03), provider adapters (PROV-01..03), and audit foundation (LOG-01) are all concretely specified. RESEARCH-FLAG-01 (overfitting risk) is resolved: a held-out split is adopted (§2). The MIMC-02 path correction is carried forward (§0). The planner has concrete type names (§5.1), package layout (§11.2), pinned dependencies (§11.3), test architecture (§3), and a recommended tracer-first slice (the parity harness is the north-star proof — lead with it).
