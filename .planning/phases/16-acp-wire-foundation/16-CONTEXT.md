# Phase 16: ACP Wire Foundation - Context

**Gathered:** 2026-08-26
**Status:** Ready for planning

<domain>
## Phase Boundary

The three wire primitives every interactive phase (17–23) rides: (1) outbound id'd JSON-RPC requests with a concurrent pending-response registry, (2) one ordered inline TurnEmitter owning all client frames with an explicit, raced backpressure policy, (3) extended transcript line types (raw-thinking passthrough, local_command, compaction marker) that later phases' schemas lock against — plus richer initialize/new/load/resume capabilities (loadSession, sessionCapabilities, configOptions advertisement). Attaches to the Phase-15 carved runner's emitFor seam. Real-time Zed rendering of tool_call/plan/thought activity (ACP-03) and editor-driven configuration (ACP-08) prove the primitives live.

</domain>

<decisions>
## Implementation Decisions

### Backpressure & Interleaving
- **D-01:** Overflow behavior: block producers, priority bypass — bounded lanes, nothing ever dropped (criterion 2 verbatim). The foreground lane bypasses queued background frames so a chatty subagent/engine stream cannot head-of-line-block the editor's active turn.
- **D-02:** Ordering: single priority queue inside the TurnEmitter; foreground class preempts the queue head, background is FIFO within itself. Single-writer total order. The interleaving policy is "foreground preempts, background FIFO" — documented and tested as an invariant (criterion 2's multi-emitter -race stress).
- **D-03:** Writer wedge: producers block with context awareness; a stall detector logs + increments a structured metric when any lane is full beyond a ~5s threshold — degrade loudly, never auto-disconnect, never block silently.
- **D-04:** Gate placement: bounded multi-emitter -race stress (~seconds, fake slow writer, asserts order + no-drop) runs in every `mise ci`; a deeper adversarial soak (minutes) lives in the eval suite.

### Config Advertisement & Scope
- **D-05:** Full v1.2 menu advertised from day 1 (operator overrode the minimal-tier/model recommendation). Apply-as-landed reconciliation: initialize/set applies keys whose handlers exist (tier/model day 1); advertised-but-unhandled keys apply as logged pending-handler no-ops — never an error, never silently discarded state. Later phases register real handlers behind the shape locked here.
- **D-06:** Menu membership: the planner enumerates the full v1.2 menu from REQUIREMENTS.md's editor-facing options at plan time; this CONTEXT locks the rule (full menu, apply-as-landed), the plan lists the enumerated membership for review.
- **D-07:** Mutation semantics: persist-then-apply — the config write (existing config-write path, 0600 discipline) must succeed before live application; write failure returns a typed JSON-RPC error with session state untouched. No half-applied state. API keys are never options (env/file only, per ACP-08).
- **D-08:** Two config layers, both editor-editable, project is the default target; the global layer is addressed via a wire-level scope parameter in the option schema (operator free-text: "there actually 2 configs: global and project. we can edit both, default project"). No dependency on Phase 20's command family for global access.
- **D-09:** Invalid option values: typed reject (option key + violation detail returned to the client); never accept-and-ignore.
- **D-10:** Initialize-time Zed settings blob: parsed schema-tolerantly (unknown keys survive round-trip) and every recognized key is applied eagerly (operator choice), with blob-fills-unset precedence — explicit config (files, editor-set options) always wins; the blob is the default-of-last-resort.
- **D-11:** Advertised options carry their current EFFECTIVE value (resolved through the precedence chain), not static defaults — the editor UI can never display stale truth.
- **D-12:** Precedence stays one chain: session override (/model, Phase 20) > time-window > project > global. Editor config writes are simply another writer into the project/global layers; /model stays the ephemeral top of the chain.

### Client Probing & Degradation
- **D-13:** Capability probed eagerly at initialize (e.g. a probe elicitation), result cached for the session lifetime — no per-ask probing, no mid-session re-probe. Older clients answer -32601 once and every surface knows to degrade.
- **D-14:** Unanswered id'd request: timeout → ONE retry → fall back to the plain-text/stdout path + structured log (request id + elapsed). Mirrors the v1.1 ask-timeout-then-non-answer philosophy (12-D-01): the user always gets something; the connection never wedges.
- **D-15:** Request ids: UUID strings BOTH directions (operator overrode the monotonic-int recommendation — matches Zed's inbound UUID style). Registry keyed by normalized id string.
- **D-16:** Telemetry: structured stderr logs + in-process counters (probes, timeouts, fallbacks, writer stalls — same counter family as D-03's stall metric). No OTel dependency; counters surface via /status (Phase 20) later.
- **D-17:** Two timeout classes in the registry: FAST-CONTROL (probe/config requests, ~10s default) vs HUMAN-ASK (elicitation/permission, human timescale — minutes, mirroring 12-D-01's 10-min default). A permission form never times out on a thinking user.
- **D-18:** Degradation is sticky per session (a capability that degraded stays degraded; no flapping, no re-probe backoff). The next session probes fresh.
- **D-19:** The registry supports synthetic-cancel resolution NOW (turn dies → pending ask resolves cancelled-normal, the contract ACP-01 names) — Phase 17 only calls the primitive; the wire phase ships it complete. — **Reversibility:** costly — reshaping the registry API after Phases 17–18 build on it re-touches every ask path.

### Transcript Schema Stability
- **D-20:** Additive-only weak schema: the existing line envelope keeps its kind-discriminated shape; new kinds (raw-thinking, local_command, compaction marker) append; readers tolerate unknown kinds and unknown fields; payloads carry `json.RawMessage` until their owning phase parses them. No global schema_version field. Later phases lock against this additive-only contract. — **Reversibility:** one-way — once lines are written, their shape is historical record; the tolerance rule is the only migration mechanism.
- **D-21:** Compaction marker = rich boundary record: boundary id + timestamp + token-usage snapshot + pre/post pointers into the transcript (what survived, where the projected window resets). Phase 19's replay reconstructs the reset from the marker itself — compaction logic need not stay deterministic forever.
- **D-22:** local_command = full invocation record: command key + verbatim args + resolution-source chain (builtin→skill→agent→file, per CMDS-01) + expansion outcome. The transcript can answer "what actually ran and why" for any past command.
- **D-23:** Thinking-bytes redaction exclusion is type-level: raw-thinking lines take a distinct code path the redactor never sees; payload stays untouched `json.RawMessage` (no re-serialization → no accidental mutation). Test asserts the redactor is called zero times for thinking bytes.

### Claude's Discretion
- Lane capacity bounds, exact stall threshold (D-03's ~5s is directional).
- Probe elicitation's concrete payload (any cheap no-op-shaped ask; Phase 17 replaces with real forms).
- Counter names + log field names (consistency with existing audit/tracer field conventions).
- Whether raw-thinking lines carry provider attribution metadata (model, phase) — planner decides from replay needs.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §ACP completeness — ACP-03 (live turn activity, one ordered TurnEmitter) and ACP-08 (editor-driven configuration) verbatim
- `.planning/ROADMAP.md` §Phase 16 — goal, 5 success criteria (live Zed cards, raced ordering, id'd-request matching, richer capabilities, extended transcript types)
- `.planning/ROADMAP.md` §Research Flags — Phase 16 flag: sequenceNumber continuity/reset-on-load legality, available_commands_update full-replacement semantics, cancel contract are web-derived LOW confidence; MUST verify against `zed-industries/agent-client-protocol` source during research BEFORE coding

### Prior Phase Contracts
- `.planning/phases/15-internal-runtime-carve-step-0/15-CONTEXT.md` — D-16 (emitFor func-field seam this phase attaches to), D-18/D-19 (Runner/export shape), D-20 (no ACP words in runtime API — the emitter stays acp-side)
- `.planning/phases/25-seed-001-kit-extraction-strictly-last/25-CONTEXT.md` — D-13/D-14 (KIT-02 Emitter/Requester minimal pair; kit-neutral event vocabulary, acp adapter translates): Phase 16's emitter design must not foreclose it
- `.planning/milestones/v1.1-phases/12-product-functional-completeness/12-CONTEXT.md` D-01 — ask-timeout-then-non-answer semantics D-14/D-17 mirror

### External Protocol Source (research-phase)
- `github.com/zed-industries/agent-client-protocol` — canonical ACP schema; configOptions/set_config_option shapes, capability negotiation, sequenceNumber rules all verified here, not from memory

### Verification Gate
- `.mise.toml` [tasks.ci] — D-04's -race stress joins the standing gate

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/acp` server + TurnRunner/SessionCloser/ChunkEmitter interfaces — the frame surface this phase extends; JSON-RPC string-id tolerance already fixed post-v1.0 (Zed UUIDs).
- Phase-15 carved `internal/runtime` Runner + `emitFor` RunnerConfig func-field — the attachment point; acpserve injects srv.Emitter post-construction today.
- Existing transcript (durable + lean projected window, session replay on restart) — D-20 extends its envelope additively.
- Existing config-write path (0600 discipline, precedence chain) — D-07/D-08 ride it; model-routing rename (732a8a3) established current naming.
- askBroker suspended-turn machinery — the human-ask class (D-17) grounds on it.

### Established Patterns
- Graceful degradation with loud structured errors (Transient/Structural provider classification) — D-14's degrade-loudly mirrors it.
- stdout discipline (ACP frames only) — the fallback paths (D-14) must keep diagnostics on stderr.
- Deterministic tests + env-flag-gated eval suites — D-04's stress/soak split follows the 12-D-03 change-class gate pattern.

### Integration Points
- TurnEmitter construction in acpserve.Run (composition root) — foreground/background lanes fed by runner emitFor + subagent/engine wiring.
- initialize/new/load/resume response builders — capability set extension (criterion 4) verified against a real Zed handshake.
- Transcript append path — three new kinds; replay reader gains tolerance rule.

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- "there actually 2 configs: global and project. we can edit both, default project" — both layers editor-editable, project default, wire-level scope for global.
- Full-menu-upfront + apply-everything-recognized chosen against minimal recommendations — then reconciled as advertise-all/apply-as-landed so Phase 16 doesn't absorb Phases 17–19 scope.
- UUID ids both directions — matching the client's own style over JSON-RPC-canonical ints.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 16-acp-wire-foundation*
*Context gathered: 2026-08-26*
