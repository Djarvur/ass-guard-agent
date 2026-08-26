# Phase 17: Permissions + Elicitation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-26
**Phase:** 17 - Permissions + Elicitation
**Areas discussed:** Permission scope taxonomy, Gate chokepoint & tool set, Elicitation form mapping, Dialog serialization

**Context:** Discussed inline while Phase 15 executes and Phase 16 plans in the background. Research-before-questions enabled.

---

## Permission scope taxonomy

Research basis: CC's own model — deny→ask→allow first-match, hooks-before, layered persistence, mcp__server__tool namespacing ([official](https://code.claude.com/docs/en/permissions), [patterns](https://dev.to/klement_gunndu/lock-down-claude-code-with-5-permission-patterns-4gcn)); MCP deny bypass caveat ([#28595](https://github.com/anthropics/claude-code/issues/28595)).

| Option | Description | Selected |
|--------|-------------|----------|
| tool×project from dialog | Always-click bounded to repo; richer rules file-only | ✓ |
| tool global | Never re-asked anywhere | |
| Per-click scope choice | Two always-buttons per dialog | |

**User's choice:** tool×project from dialog (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| CC rule model | deny→ask→allow, wildcards, mcp__server__tool | ✓ |
| Flat allowlist | Tool names only | |

**User's choice:** CC rule model (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Both directions | Reject-always writes deny too | ✓ |
| Allow-only from dialog | Deny hand-authored only | |

**User's choice:** Both directions (Recommended)

---

## Gate chokepoint & tool set

Research basis: PEP canon — single centralized chokepoint, decision/enforce split ([Oso](https://www.osohq.com/authorization-glossary/policy-enforcement-point-pep), [NIST](https://csrc.nist.gov/glossary/term/policy_enforcement_point)); indirect-path bypass the known failure ([ActPlane](https://arxiv.org/html/2606.25189v1)).

| Option | Description | Selected |
|--------|-------------|----------|
| Same pipeline, no-op ask | Ungated routes through chokepoint; rules evaluated, no dialog | ✓ |
| Bypass when ungated | Fast path skips chokepoint | |

**User's choice:** Same pipeline, no-op ask (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Rules derive the set | ask rules default = mutating class; file tunes | ✓ |
| Hardcoded mutating set | File only allow/deny | |

**User's choice:** Rules derive the set (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Rules + decline ask-class | Automation: deny/allow enforced, ask-class declines w/ note | ✓ |
| Full gated w/ timeout | Void dialogs, minutes of stall | |
| Automation ungated | No-human = no-policy | |

**User's choice:** Rules + decline ask-class (Recommended)

---

## Elicitation form mapping

Research basis: ACP elicitation RFD — requestedSchema multi-field forms ([RFD](https://agentclientprotocol.com/rfds/elicitation), [schema](https://agentclientprotocol.com/protocol/v1/schema)); boolean-type requires client advertisement ([config options](https://agentclientprotocol.com/protocol/v1/session-config-options)).

| Option | Description | Selected |
|--------|-------------|----------|
| Typed mapping table | select/array-of-enum/text/boolean-conditional per ask kind | ✓ |
| Generic passthrough | One free-text field per ask | |

**User's choice:** Typed mapping table (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Convert whole family | AskUserQuestion joins elicitation; text = fallback | ✓ |
| Only named asks | ACP-02 letter verbatim; two UXes coexist | |

**User's choice:** Convert whole family (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Re-validate + one re-ask | Then non-answer routing | ✓ |
| Invalid → non-answer | No re-ask | |

**User's choice:** Re-validate + one re-ask (Recommended)

---

## Dialog serialization

Research basis: never stack modals — serialize one at a time ([NNG](https://www.nngroup.com/articles/modal-nonmodal-dialog/), [r/UXDesign stacking](https://www.reddit.com/r/UXDesign/comments/1gutea2/what_is_the_reason_we_dont_like_modal_stacking/), [Premiere enforced serialization](https://developer.adobe.com/premiere-pro/uxp/plugins/tutorials/add-modal-dialogs/)).

| Option | Description | Selected |
|--------|-------------|----------|
| FIFO + fg-priority | Mirrors emitter fg/bg classes | ✓ |
| Plain FIFO | Subagent asks can delay fg | |
| Permission-first | Safety prompts jump queue | |

**User's choice:** FIFO + fg-priority (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Notes + counters | Enqueue note + pending count | ✓ |
| Silent queue | Sequential dialogs, no telemetry | |

**User's choice:** Notes + counters (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Drain by turn id | Open dialog + queued asks resolve cancelled | ✓ |
| Open-only cancel | Queued asks fire anyway | |

**User's choice:** Drain by turn id (Recommended)

---

## Deferred Ideas

None — discussion stayed within phase scope.
