# Phase 17: Permissions + Elicitation - Context

**Gathered:** 2026-08-26
**Status:** Ready for planning

<domain>
## Phase Boundary

The agent's asks become clickable editor surfaces: permission asks via session/request_permission (allow/reject × once/always), learning/engine/model-initiated asks via elicitation/create structured forms — all riding the AskBroker suspension pattern through Phase 16's registry and probe/degrade machinery. The ONE gate pipeline (hook verdict → permission ask → execute) is implemented at a single mode-independent chokepoint and documented, so Phase 21's hooks join rather than bolt on. permissions.yaml persists choices across restarts. Default stays ungated ("available, not default").

</domain>

<decisions>
## Implementation Decisions

### Permission Scope Taxonomy
- **D-01:** One "always allow" click persists tool×project into the project's permissions.yaml. Richer rules (arg patterns, path prefixes) exist in the FILE for hand-editing only — the dialog never writes them. Other projects re-ask.
- **D-02:** permissions.yaml carries the Claude-Code-parity rule grammar: deny → ask → allow, first match wins, deny beats any allow; exact + wildcard patterns; MCP tools namespaced `mcp__server__tool`. The dialog writes simple entries; the file supports the full grammar. — **Reversibility:** costly — permissions.yaml entries accumulate as user trust decisions; reshaping the grammar later means migrating trust state.
- **D-03:** Reject-also-always writes deny rules (same tool×project scope) — both directions from the dialog (CC "No, and don't ask again" precedent).
- **D-04:** Hooks are checked BEFORE permission rules (criterion 5's ordering; CC parity) — the chokepoint runs hook verdict first, then permission evaluation.

### Gate Chokepoint & Tool Set
- **D-05:** ONE pipeline for both modes: hook verdict → permission ask → execute. Ungated (default) = the ask step evaluates rules and returns allow without ever opening a dialog — deny rules still deny. Mode-independent shape; both modes tested through the same chokepoint; criterion 4 (zero new dialogs on default config) holds by construction.
- **D-06:** The dialog set is DERIVED by the rule grammar: deny rules → deny; allow rules → allow; ask rules (DEFAULT = today's mutating/serialized tool class) → dialog; everything else (read-only concurrent class) → allow. The file tunes ask subjects per pattern; dialog fatigue has a relief valve short of always-allow.
- **D-07:** Automation/cron turns (no human present): deny + allow rules still enforced; ask-class tools DECLINE with a client-visible note — fail-safe: never a dialog nobody answers, never a silent allow. The declined-execution audit trail is visible in the next foreground session.

### Elicitation Form Mapping
- **D-08:** Typed mapping table: single-choice → select (enum) field; multiSelect AskUserQuestion → array-of-enum field; free-text → text field; boolean → boolean field ONLY when the client advertised boolean support (ACP constraint — else yes/no string select); headers/labels → field titles. Every existing ask kind has a defined form rendering.
- **D-09:** The WHOLE ask family converts: model-initiated AskUserQuestion joins learning-store and engine asks in elicitation forms (same suspension, consistent editor UX). The v1.1 captured-shape plain-text route becomes the FALLBACK, not the primary. — extends ACP-02's letter by operator endorsement.
- **D-10:** Invalid elicitation response: the agent re-validates against the requested schema, re-asks ONCE with the violation noted; second failure routes exactly like a non-answer (12-D-01 corpus form). Bounded loop; the user always lands somewhere.

### Dialog Serialization
- **D-11:** One ask outstanding at a time (never stacked modals). Queue order: FIFO within priority classes mirroring the emitter's fg/bg — the foreground turn's asks fire before subagent/engine asks; within a class, ask order preserved.
- **D-12:** The queue is visible: each enqueue emits a session/update note ("ask queued — N pending") and increments a counter (same counter family as the stall metrics). Criterion 2's "session stays alive and responsive" gets user-visible evidence.
- **D-13:** Queue entries carry their turn id: turn death resolves the OPEN dialog as cancelled (16-D-19 synthetic-cancel) AND drains that turn's queued-but-unfired asks as cancelled-normal immediately — no orphaned dialogs, no zombie asks firing for a dead turn.

### Claude's Discretion
- permissions.yaml concrete syntax (YAML lists vs rule strings) within the D-02 grammar.
- Pattern-matching semantics details (glob vs prefix) — CC-compatible prefix wildcards preferred.
- The pending-count note's exact wording and dedupe rate (avoid spam on rapid enqueues).
- Probe elicitation from 16-D-13's discreet replacement with a real form once mapping exists.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §ACP completeness — ACP-01 (request_permission, ungated|gated switch) and ACP-02 (elicitation forms + probe-and-degrade) verbatim
- `.planning/ROADMAP.md` §Phase 17 — goal, 5 success criteria (native dialog + persistence, alive-while-asking, form/degrade duality, ungated default, ONE chokepoint)

### Prior Phase Contracts (hard dependencies)
- `.planning/phases/16-acp-wire-foundation/16-CONTEXT.md` — D-13 (eager probe, sticky), D-14 (timeout→retry→fallback), D-17 (HUMAN-ASK class), D-19 (registry synthetic-cancel — this phase calls it), D-05/D-12 (permissions.mode in advertised menu; one precedence chain)
- `.planning/milestones/v1.1-phases/12-product-functional-completeness/12-CONTEXT.md` — D-01 (ask timeout → non-answer), D-02 (model-initiated asks + engine advisory), D-05 (advisory note shape) — the ask semantics being upgraded

### Protocol Sources (research-phase verifies shapes)
- `agentclientprotocol.com/protocol/v1/schema` — session/request_permission options shape, elicitation/create requestedSchema
- `agentclientprotocol.com/rfds/elicitation` — form-mode multi-field schema semantics
- `agentclientprotocol.com/protocol/v1/session-config-options` — boolean-type advertisement constraint (D-08)
- `code.claude.com/docs/en/permissions` — CC rule model reference (D-02's parity target: deny→ask→allow, layers, wildcards, hooks-before)

### Code Anchors
- `internal/runtime` askBroker wiring (post-Phase-15 home) — the suspension pattern these asks ride
- toolexec/coreexec dispatch boundary — the chokepoint's landing zone (planner places it; D-05 forbids a second path)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- AskBroker suspension pattern (sessionFor callback wiring) — human-timescale waits never hold locks; both ask types ride it.
- Read-only-concurrent vs mutating-serialized tool classes — D-06's default ask set IS today's mutating class; no new classification needed.
- Phase-16 registry (once landed): id'd outbound requests, HUMAN-ASK timeout class, synthetic-cancel — the transport D-11..D-13 build on.
- Engine advisory note machinery (12-D-05) — D-12's queued-notes reuse the session/update note shape + dedupe discipline.
- Learning store's ask-once-remember — its asks become elicitation forms; answers persist as today.

### Established Patterns
- Graceful degradation (probe → plain-text fallback) — 16-D-13/D-14 machinery, reused per-ask-type.
- Loud structured counters — D-07's decline notes and D-12's queue counters join the family.
- Fail-safe defaults (unmatched ⇒ nothing) — D-07's decline-not-allow mirrors the discipline.

### Integration Points
- Gate chokepoint sits at the tool-execution dispatch boundary (hook verdict → permission ask → execute) — the ONE place; Phase 21's hooks join at its head.
- permissions.yaml: new file in the project's .ass-guard family (0600 discipline), loaded at session start, consulted at the chokepoint, written by dialog clicks.
- configOptions permissions.mode flip (16-D-05/D-12) toggles D-05's ask-step behavior live.

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- All-recommended sweep across all four areas — the operator endorsed the CC-parity rule model, the mode-independent chokepoint, the fail-safe automation policy, whole-family form conversion, and turn-scoped queue drain.
- Blast-radius thinking dominated scope taxonomy: tool×project as the dialog's write scope keeps one click from unleashing a tool everywhere.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 17-permissions-elicitation*
*Context gathered: 2026-08-26*
