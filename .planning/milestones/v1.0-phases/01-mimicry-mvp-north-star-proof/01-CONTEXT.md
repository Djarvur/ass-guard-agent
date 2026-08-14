# Phase 1: Mimicry MVP (north-star proof) - Context

**Gathered:** 2026-08-09
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 1 is the **thesis-proof phase**. It empirically validates the project's Core Value (PROJECT.md): that ass-guard, with the zcode profile loaded, produces outgoing model requests structurally indistinguishable from live zcode — measured by a behavioral A/B parity test on tool-call sequences. **If the A/B parity test fails, the project stops and re-plans** (ROADMAP.md Phase 1 Goal; PROJECT.md Anti-Pattern 5). This is the literal gate; everything downstream (Phases 2–6) is conditional on it passing.

**In scope (16 REQ-IDs):**
- **Mimicry chokepoint (MIMC-01..04):** a Profile Shaper component sits between the Turn Loop and the provider adapter, applying the active profile's {system prompt, tool catalog declaration, message shape, identity fields} to every outgoing request. The A/B parity test validates statistical indistinguishability; byte-identical parity is explicitly out of scope (MIMC-04).
- **Profile artifact (PROF-01..05):** a profile is a configurable bundle selected by name; zcode is profile #1; the architecture supports N from day one (no zcode-specific code paths — PROF-02). The zcode profile is extracted from real on-disk JSONL transcripts (Phase 0's verified path), carries a `target_capture_ref` and coverage manifest, and a drift detector (`ass-guard profile check zcode`) flags target divergence.
- **Tool catalog substrate (TOOL-01..03):** a built-in catalog matching Claude Code's set; the profile-declared tool schema is authoritative at runtime via a schema-adapter layer (TOOL-02); a CI catalog-consistency check rejects profiles the catalog can't satisfy (TOOL-03).
- **Provider adapters (PROV-01..03):** both Anthropic-shape and OpenAI-shape protocols via a common `Provider` interface with `TranslateToInternal`/`TranslateFromInternal` tool-call translation. Anthropic-shape uses `anthropics/anthropic-sdk-go` (swappable base URL for Z.ai/GLM); OpenAI-shape uses `sashabaranov/go-openai` (Chat Completions, NOT Responses API — Phase 0 #2).
- **Audit log foundation (LOG-01):** records the verbatim shaped outgoing request for every turn — the mimicry evidence source, from day one. (Phase 2 completes LOG-02..04.)

**Out of scope (Phase 2+ owns these — PROJECT.md mandates serialized deltas, not thin-slicing):**
- Session Core (SESS-01..06): two-layer context model, boundary semantics, Session Manager as sole transcript owner.
- ACP interface (ACP-01..05): the stdio JSON-RPC server, streaming, session/load replay.
- Concurrency bounds (PARA-01..04): subagent dispatch, semaphore-bounded outbound concurrency.
- Real tool execution (TOOL-04..05): concurrent read tools, serialized mutating tools, swappable WebSearch/WebFetch backends. Phase 1 stubs tool execution; the parity test only measures the model's tool-SELECTION behavior.
- Everything in Phases 3–6.

**Mode:** mvp (ROADMAP.md Phase 1). The phase builds *just enough* loop to prove the thesis, with a clean seam where Phase 2's Session Core slots in. The load-bearing parts (Shaper + adapters + profile loading) survive into Phase 2; the test-harness loop is replaced.

</domain>

<decisions>
## Implementation Decisions

### A/B parity test design (the gate — MIMC-03/MIMC-04)

- **D-01:** The parity prompt suite is a **curated divergence set** of 5–15 hand-authored prompts, each deliberately targeting a tool-divergence-prone path (multi-tool sequencing, ambiguous tool selection, edge cases where a weaker shaper would pick a different tool) and carrying an expected tool-call pattern. The team curates the suite; research documents what makes a prompt "divergence-prone" and justifies each inclusion. (Rejected: replay-from-transcripts risks overfitting; synthetic sweep may not reflect real workload.)
- **D-02:** The parity metric is **two-layer**: (1) **tool-name sequence equality** (the ordered list of tool names each arm produces must match — e.g. `[Read,Grep,Edit]` vs `[Read,Grep,Edit]` = match; any reorder/substitution/insertion = mismatch) AND (2) **per-tool argument structural equality** (same JSON keys present, same value shapes — NOT exact string match). This is stricter than pure sequence equality (catches a shaper that picks the right tool but passes malformed args) but stops short of byte-diff (respects MIMC-04). Research must define where argument-structural comparison draws the line (e.g. does it normalize file paths? sort arrays?).
- **D-03:** The pass/fail threshold is **100%** — ALL curated prompts must match on BOTH sequence AND argument structure. The phase is a literal gate; anything softer turns "gate" into "checkpoint." This is achievable because the team controls what's in the curated suite. The discipline shifts to: *the curated set must genuinely probe divergence* — a suite of trivial prompts that always match is a self-deception risk. Research/planning must document the divergence-probe rationale per prompt.
- **D-04:** Run-to-run model variance is handled via **deterministic temp=0, single run per arm**. Every mismatch is a real signal pointing at the shaper — no "it might be variance" escape hatch. The test config (model, temp, seed if the provider exposes one) is recorded alongside results for reproducibility.
- **D-05:** The "zcode arm" of the A/B test is **replay of zcode's captured tool-call output** from the rollout JSONL transcripts (Phase 0's verified path `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl`). The test asks: "does ass-guard, given the same prompt, reproduce the tool-call sequence zcode actually produced?" Fully offline, reproducible, no live zcode install or live API calls at test time. (Rejected: live zcode is stochastic + operationally heavy; frozen golden set adds a capture step without clear benefit over direct replay.)
- **RESEARCH-FLAG-01 (overfitting risk — NOT a decision, a mandatory research item):** the rollout transcripts feed BOTH profile extraction (request-shape layer: system blocks, tools, headers) AND the parity reference (tool-call sequence layer). These are different layers of the same data, but the coupling is real. The researcher MUST evaluate whether a **held-out transcript split** is needed (extract the profile from sessions A–N; test parity against sessions N+1–M) and recommend a split strategy if so. The profile is extracted from request *shape*; parity compares tool-call *sequences* — the layers differ, but a clean split removes the concern entirely.

### Profile fidelity & coverage boundary (PROF-03/04/05)

- **D-06:** Profile fields are tiered into **three explicit fidelity bands**. Research assigns each captured field to a tier based on how it participates in the mimicry thesis:
  - **TIER-1 (byte-faithful):** system-prompt text, tool `name`+`input_schema`, `tool_choice` — model-visible content the mimicry thesis directly depends on. Drift here is a hard failure.
  - **TIER-2 (structural):** presence and shape matter, exact values don't — notably the 12 identity header NAMES must be present (their VALUES like `x-request-id` vary per session), and the message-block shape (system array structure, tools array structure). Drift in structure/presence is flagged; value variance is expected.
  - **TIER-3 (informational):** timestamps, token counts, request IDs, duration — logged for audit (LOG-01) but NEVER drift-flagged. These are per-request ephemera.
  - The drift detector (PROF-04) only flags TIER-1/2 drift. TIER-3 is audit-only. This mapping makes the drift detector signal, not noise.
- **D-07:** The coverage manifest (PROF-05) is a **machine-readable artifact shipped with the profile** (e.g. YAML/JSON sidecar or embedded block). It lists every captured field, its tier assignment, and its source location in the capture. "Incomplete capture" (PROF-05's core concern) becomes a **CI-style hard failure**: if a known field from a fresh capture is MISSING from the manifest, the check fails loud. The same manifest feeds TOOL-03 catalog-consistency CI — one artifact, two gates.
- **D-08:** `ass-guard profile check zcode` (PROF-04, north-star killer-feature N1 engineered out from day one) performs a **fresh live capture**: it runs live zcode (spawned, with operator-provided keys), captures the resulting outgoing request, diffs it field-by-field against the tiered profile manifest, and reports the specific fields + tiers that drifted. This is an **operator command, not a CI step** — it needs a live zcode install + keys at check time. (Rejected: diff-against-recent-transcripts has a fuzzy freshness window; two-mode adds complexity without clear benefit for v1.)

### Profile Shaper architecture (MIMC-01, PROF-02)

- **D-09:** The Profile Shaper drives the provider SDK via a **hybrid: native SDK types for the bulk + a data-driven escape hatch for the long tail**. For Anthropic-shape profiles, the Shaper populates `anthropic-sdk-go`'s native types (`Message`, `System` blocks, `ToolDef` with `InputSchema`, `Thinking` config) — leveraging compile-time type safety, SDK-managed streaming, and retries. For fields the SDK types don't model — notably the 12 identity headers, where SDK request-option layers are inconsistent across providers — a per-profile **escape hatch** (a `map[string]json.RawMessage` merged into the final serialized body) carries them. The escape hatch is **part of the profile artifact, not code** — it preserves PROF-02 (no zcode-specific code paths; the data drives the difference). [Claude's discretion — user declined to specify; reversible at planning. Chosen because it gets SDK benefits where possible and raw fidelity where needed, with the coupling staying data-driven.]
- **D-10:** Tool schema resolution follows **TOOL-02 verbatim — the profile-declared schema is authoritative at runtime**. A schema-adapter layer sits between the catalog and the Shaper: when the Shaper builds the outgoing request, it pulls each tool's model-facing schema from the **profile** (what the model sees); the built-in catalog provides **execution behavior** (what the tool does when called). When a `tool_call` returns, it resolves/validates against the profile schema too. Drift between profile-declared and catalog-implemented schemas is caught by TOOL-03 CI. [Claude's discretion — user declined to specify; directly follows from TOOL-02's wording. The mechanism keeps "what the model sees" and "what the tool does" cleanly separable.]
- **D-11:** PROF-02 ("no zcode-specific code paths in the Shaper") is enforced by a **synthetic-profile conformance test**: a CI test loads a non-zcode fixture profile (a minimal synthetic profile with different system blocks, different tool names, different headers) and runs it through the same Shaper code path. Any zcode-specific branch (hardcoded zcode strings, zcode-only defaults, `if profile.Name=="zcode"`) breaks the synthetic profile. A grep-based lint rule (`grep -r zcode internal/shaper/` returns nothing outside test fixtures) is belt-and-suspenders. This makes PROF-02 a **structural guarantee, not a code-review hope**. [Claude's discretion — user declined to specify; chosen as the strongest lightweight enforcement that doesn't double the profile-extraction work.]

### Turn Loop MVP scope vs Phase 2 (serialized-deltas discipline)

- **D-12:** The Phase-1 Turn Loop is a **test-harness loop with a narrow interface**, NOT a session engine. Interface: `Run(ctx, profile, prompt, toolStubs) -> []ToolCall`. It loads the profile, shapes the request via the Shaper, sends to the provider, parses `tool_calls`, returns them. **NO session management, NO context-window projection, NO ACP streaming, NO session/load replay** — those are SESS-01..06, Phase-2 REQs, and PROJECT.md mandates serialized deltas (not thin-sliced). The load-bearing parts (Shaper + adapters + profile loading) survive into Phase 2; the test-harness loop is replaced by the real Session Core. [Claude's discretion — user declined to specify; chosen to respect the serialized-deltas discipline while still giving the Shaper a real integration seam.]
- **D-13:** LOG-01 (audit log of the verbatim shaped request) hooks in via an **event-bus subscriber**: the Turn Loop emits a `RequestShaped` event carrying the verbatim outgoing request; the audit log subscribes asynchronously and writes it. This satisfies LOG-02 (async, never in critical path) from day one, rather than building a synchronous coupling that Phase 2 must decouple. The Phase-1 event bus is a **minimal interface** (just the `RequestShaped` event type for now); Phase 2 expands it. This also gives PARA-02 (subagent results tagged with parent-turn-id via the bus) a seam to plug into later. [Claude's discretion — user declined to specify; chosen to satisfy LOG-02's async requirement from the start.]
- **D-14:** The built-in tool catalog (TOOL-01) declares **all 77 captured tools faithfully** in the zcode profile so the model sees the complete catalog and selection behavior matches zcode. The built-in catalog provides schemas satisfying all 77 profile declarations (TOOL-02/TOOL-03). Because tool **execution** is stubbed across the board in the parity test (per D-15 below), the full-vs-schema-stub execution distinction is moot for the test — the work is faithful **schema declarations** for 77 tools + a dispatch skeleton. [Claude's discretion — user declined to specify; collapses with the stub-all-tools decision. The model's tool SELECTION is exercised across all 77 even though only the schema matters for the parity test.]

### Claude's Discretion

The following are explicitly Claude's-discretion (marked inline above) because the user declined to specify them during discussion. Each is reversible at planning:
- **D-09** SDK-driving mechanism (hybrid native + escape hatch) — research may propose an alternative if the escape-hatch shape proves awkward for a specific SDK.
- **D-10** Schema-resolution mechanism (profile-authoritative adapter layer) — directly follows from TOOL-02; research validates the adapter sits at the right boundary.
- **D-11** PROF-02 enforcement (synthetic-profile conformance test + lint) — research may propose a stronger mechanism (e.g. a second real profile) if the synthetic fixture proves too weak.
- **D-12** Turn Loop shape (test-harness, narrow interface) — research validates the interface is rich enough for the parity test and clean enough to replace in Phase 2.
- **D-13** Audit hook mechanism (event-bus subscriber) — research validates the Phase-1 minimal bus is the right seed for Phase 2's expansion.
- **D-14** Tool catalog scope (all 77 declared faithfully) — research confirms 77 is the right number and identifies any tools needing special schema handling.
- **D-15** Tool execution in the parity test (stub all tools) — see A/B parity decisions; reversible if research finds multi-turn reactions to real tool output are needed for the curated prompts.

Also at Claude's discretion (the researcher/planner resolves):
- The exact composition of the 5–15 curated divergence prompts (D-01) — the divergence-probe rationale must be documented, but specific prompts are at the researcher's discretion informed by the captured zcode transcripts.
- The exact tier assignment of each captured field (D-06) — the three-tier *framework* is locked; which field goes in which tier is the researcher's call based on the captured evidence.
- The exact format of the machine-readable coverage manifest (D-07) — YAML vs JSON vs embedded block is the planner's call.

### Data-source strategy (D-16 — blocker resolution, added 2026-08-11)

- **D-16:** The profile extractor and parity test read from zcode's **continuously-written rollout logs at execution time** — they do NOT hardcode specific session IDs or tool counts. zcode writes rollout JSONL continuously as the user works (`~/.zcode/cli/rollout/model-io-sess_<id>.jsonl`); the extractor scans the directory, picks the session(s) with the richest request-body capture (full system blocks + tools + headers), and extracts from those. The tool count is **whatever the source session declares** (observed: 97–103 depending on session; the stable built-in core is ~20, the rest are MCP/plugin tools that vary per session — this variability is itself PROF-04 drift evidence). Acceptance criteria must NOT hardcode a specific count (the original plans' `== 77` was wrong — it pinned a snapshot that drifted within hours). **RESEARCH-FLAG-01 resolved:** use DIFFERENT sessions for extraction vs parity reference (natural held-out split — e.g. extract the profile from the richest main session; draw parity-reference tool-call sequences from a different session with divergence-prone multi-tool turns). The coupling concern (same data for extraction + parity) is removed by using disjoint sessions.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 0 output (the ground truth Phase 1 builds on — MIMC-02/PROF-03/PROF-05 source)
- `.planning/research/VERIFIED-FACTS.md` — **the post-spike source of truth.** Item #1 (zcode JSONL path + per-line schema) is the load-bearing input for MIMC-02/PROF-03/PROF-05: corrected path `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl`, full wire-level request/response schema, the 12 identity header names, the 77-tool catalog shape, the 3-block `system[]` structure, `thinking` + `tool_choice` presence, Anthropic-shape body with `providerId: "builtin:zai-coding-plan"`. Item #2 (go-openai tool-calling schema) grounds PROV-03's OpenAI-shape adapter: Chat Completions shape (NOT Responses API), `tools[].function` wrapper, string-typed `arguments`, `role:"tool"` result message. Item #3 (ACP v1) is Phase 2's input but confirms transport discipline. Items #4/#5 are out of Phase 1's direct scope but confirm constraints.

### The 16 Phase-1 requirements (what this phase delivers)
- `.planning/REQUIREMENTS.md` §"Mimicry (north star — Phase 1)" (MIMC-01..04), §"Profiles (Phase 1)" (PROF-01..05), §"Tool catalog (Phase 1 substrate + Phase 2)" — TOOL-01/02/03 are Phase 1; TOOL-04/05 are Phase 4, §"Providers (Phase 1 substrate + Phase 3)" — PROV-01/02/03 are Phase 1; SCHED-* are Phase 3, §"Audit log (Phase 1 + Phase 2)" — LOG-01 is Phase 1; LOG-02..04 are Phase 2.

### Phase goal + success criteria (the gate definition)
- `.planning/ROADMAP.md` §"Phase 1: Mimicry MVP (north-star proof)" — the 5 success criteria, especially #4 (the A/B parity test gate) and #3 (`profile check` drift detector). Mode: `mvp`.

### Project-level constraints that shape every Phase-1 decision
- `.planning/PROJECT.md` — **Core Value** (structural indistinguishability is the bar), **Constraints** (Go single static binary; stdout = ACP only — load-bearing for the event-bus/audit-log decision; Claude-Code config drop-in compat; static binary via goreleaser; macOS+Linux amd64+arm64; no tool-execution confirmation tier), **v1 Cut-Line** (zcode profile only; OpenSpec in v1; ACP-only interface — Telegram v2), **Anti-Pattern 5** (north-star mimicry thesis gates everything; stop-and-replan on Phase-1 failure). The "Constraints" and "v1 Cut-Line" sections are load-bearing for D-09/D-12/D-13.

### Phase 0 decisions that propagate into Phase 1 (do not relitigate)
- `.planning/phases/00-spike-re-verification/00-CONTEXT.md` — D-06 (STT always external — not Phase-1 relevant but the architectural-rule pattern matters), D-07 (negative-finding protocol — the tiered-response pattern informs the three-tier fidelity model in D-06 above). The MIMC-02 path correction (Tier-B resolution) is recorded in VERIFIED-FACTS.md item #1 Notes.

### Research/stack references
- `.planning/research/STACK.md` §Focus 1 (Mimicry / Profile Mechanism) — the profile-as-config-bundle pattern, the capture→expression flow. §"Inherited Core Technologies" rows for `anthropic-sdk-go` (PROV-03 Anthropic adapter), `sashabaranov/go-openai` (PROV-03 OpenAI adapter), `log/slog` (stderr logging discipline). §"What NOT to Use" (hand-written mimicry profiles — Phase 1 MUST extract, not guess).

### Codebase (minimal — greenfield + throwaway spikes)
- `spikes/` — the Phase-0 throwaway spikes (isolated module `github.com/djarvur/ass-guard-spikes`, gitignored build artifacts per Phase-0 D-05). `spikes/02-openai-toolschema/` is the most relevant precedent for the OpenAI-shape adapter (PROV-03); `spikes/03-acp-handshake/` for the ACP framing Phase 2 needs (but confirms the newline-delimited JSON-RPC discipline). The spikes are NOT the start of the real codebase — the repo-root `go.mod` starts in Phase 1.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **None in the production sense.** This is a greenfield repository — the repo root has no `go.mod` (Phase 1 starts it), no `.go` files outside `spikes/`, no `src/`. The only existing Go code is the Phase-0 throwaway spikes under `spikes/` (isolated module, not the codebase).
- **Spike reference value:** `spikes/02-openai-toolschema/` is a working reference for `go-openai@v1.42.0` Chat Completions tool-calling serialization (the schema the OpenAI-shape adapter conforms to). `spikes/03-acp-handshake/` demonstrates the newline-delimited JSON-RPC framing (Phase 2's direct input, but confirms the transport discipline Phase 1's event-bus/audit-log must respect). These are *references to study*, not code to import — the spikes module is deliberately decoupled from the real codebase (Phase-0 D-05).

### Established Patterns
- **Transport discipline (load-bearing):** stdout is reserved exclusively for ACP JSON-RPC frames; all logging/diagnostics go to stderr (PROJECT.md "Transport discipline"; AGENTS.md). Phase 1's event-bus + audit-log (D-13) and any diagnostic output MUST write to stderr, never stdout. This is non-negotiable and inherited from the project's LSP-style discipline.
- **Serialized deltas (PROJECT.md):** Phase 1 does NOT thin-slice the six deltas. It proves ONE delta (mimicry) before anything else builds. D-12 (test-harness loop, not Session Core) is the direct application.
- **Stop-and-replan culture (PROJECT.md Anti-Pattern 5):** if the A/B parity test fails (D-03), the project stops and re-plans. This is not a "fix and continue" failure — it's a "the thesis may be wrong" failure.

### Integration Points
- **Profile artifact → Turn Loop → Shaper → Provider adapter → Provider:** the Phase-1 data flow. The profile artifact (extracted from zcode JSONL per MIMC-02) is loaded by the test-harness Turn Loop (D-12), which drives the Profile Shaper (D-09/D-10), which shapes the request for the provider adapter (PROV-01..03), which talks to Z.ai's GLM via the Anthropic protocol (Phase 0 #1).
- **Audit log subscription (D-13):** the Turn Loop emits `RequestShaped`; the audit log subscribes via the minimal event bus. This is the LOG-01 hook and the seed for Phase 2's fuller bus.
- **`ass-guard profile check zcode` (D-08):** a cobra subcommand (the CLI surface is the ACP-registry `acp` subcommand per STACK; `profile check` is a sibling subcommand). Spawns live zcode, captures, diffs against the tiered manifest. This is the operator-facing drift detector.

</code_context>

<specifics>
## Specific Ideas

- The user's framing throughout was that **the A/B parity test IS the project's reason to exist** — every other decision (profile tiers, Shaper architecture, loop scope) is in service of making that test a meaningful, falsifiable gate. D-03 (100% threshold) and D-04 (temp=0 determinism) both come from making the test falsifiable: every failure must point at the shaper, not at variance or a soft threshold.
- The two-layer metric (D-02: sequence equality AND argument structural equality) is deliberately stricter than the bare MIMC-03 reading. The user chose the stricter bar — catching a shaper that picks the right tool but passes malformed args — which means research must define the argument-structural comparison carefully (where it normalizes, where it's strict).
- The user chose **fresh live capture** for `profile check` (D-08) over diff-against-recent-transcripts, accepting the operational weight (live zcode install + keys at check time) because PROF-04 is the "killer feature engineered out from day one" and deserves the truest semantics.
- Several decisions are marked Claude's-discretion because the user declined to specify them across multiple prompts. Each was chosen to follow from an existing locked decision or requirement (D-10 from TOOL-02; D-12 from serialized-deltas; D-14 from D-15 + the 77-tool capture) — they are the defaults that make the locked decisions coherent, not new commitments. All are reversible at planning.

</specifics>

<deferred>
## Deferred Ideas

None raised during discussion — discussion stayed tightly within the Phase-1 mimicry scope. The following adjacent topics were identified as out-of-scope and belong in their own phases (recorded so they're not lost):
- **OpenAI-shape profile fields translation (PROV-02):** how the Anthropic-shape zcode profile fields translate when routed through the OpenAI-shape adapter is a Phase-1-research item but NOT a Phase-1-user-decision — it's implementation detail the researcher/planner resolves via the `TranslateToInternal`/`TranslateFromInternal` interface.
- **Profile versioning / pinning mechanics (PROF-03 `target_capture_ref`):** the *concept* of `target_capture_ref` is locked (= sessionId + corrected path per VERIFIED-FACTS.md item #1 Notes); the exact versioning scheme is a planner detail.
- **Phase-1 CLI surface shape:** `profile check` is a cobra subcommand (D-08); the exact subcommand tree is a planner detail. The ACP `acp` subcommand is Phase 2's concern.
- **Second real profile (claude-code):** would prove PROF-02 harder than the synthetic fixture (D-11), but doubles profile-extraction work and claude-code transcripts have a different schema (`queue-operation` per VERIFIED-FACTS.md item #1). Deferred to v2 (PROF-06).
- **Held-out transcript split (RESEARCH-FLAG-01):** not a deferral — a mandatory research item. The researcher decides whether profile-extraction and parity-test transcripts need to be disjoint sessions.

</deferred>

---

*Phase: 1-Mimicry MVP (north-star proof)*
*Context gathered: 2026-08-09*
