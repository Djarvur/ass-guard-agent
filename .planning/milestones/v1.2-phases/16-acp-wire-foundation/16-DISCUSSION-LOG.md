# Phase 16: ACP Wire Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-26
**Phase:** 16 - ACP Wire Foundation
**Areas discussed:** Backpressure & interleaving, Config advertisement & scope, Client probing & degradation, Transcript schema stability

**Context:** Discussed inline while Phase 15 executes in the background (manager parallel-work pattern). Research-before-questions enabled.

---

## Backpressure & interleaving

Research basis: bounded-queue overwrite guidance ([Paritytech DoS-resilience](https://paritytech.github.io/json-rpc-interface-spec/dos-attacks-resilience.html)) — rejected because criterion 2 demands no dropped frames; LSP jsonrpc2 single-writer ordered queue precedent.

| Option | Description | Selected |
|--------|-------------|----------|
| Block, priority bypass | Bounded lanes; producers block; fg bypasses queued bg | ✓ |
| Drop-droppable kinds | Overwrite-on-full for thought chunks | — contradicts criterion 2 |
| Unbounded + alarm | Memory as backpressure | |

**User's choice:** Block, priority bypass (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Single pq, fg preempts | One priority queue in TurnEmitter | ✓ |
| Strict-preference queues | bg only drains when fg empty | — starvation |
| Timestamp merge | Per-source FIFO merged by timestamp | — untestable total order |

**User's choice:** Single pq, fg preempts (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Block + loud stall metric | Detector logs + metric past ~5s | ✓ |
| Block silently | Natural slowdown | |
| Deadline disconnect | Fail-fast close | |

**User's choice:** Block + loud stall metric (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Ci stress + eval soak | Seconds in mise ci; minutes in eval suite | ✓ |
| Eval-suite only | Fast units only in ci | |

**User's choice:** Ci stress + eval soak (Recommended)

---

## Config advertisement & scope

Research basis: Zed settings surface documented ([Agent Settings](https://zed.dev/docs/ai/agent-settings)); configOptions/set_config_option specifics only in ACP repo source → research-phase verifies.

| Option | Description | Selected |
|--------|-------------|----------|
| Mechanism + tier/model | One option family day 1 | |
| Full menu upfront | Every v1.2 option advertised day 1 | ✓ (operator override) |

**User's choice:** Full menu upfront — NOT the recommendation; consequences reconciled below.

| Option | Description | Selected |
|--------|-------------|----------|
| Persist + apply live | .ass-guard write then live apply | ✓ |
| Session-only override | Dies with session | |
| Per-call persist flag | Two behaviors per option | |

**User's choice:** Persist + apply live (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Typed reject | Key + violation detail | ✓ |
| Accept + ignore | Silent no-op | |

**User's choice:** Typed reject (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Parse, store, act minimal | Schema-tolerant store, act on documented | |
| Apply everything recognized | Eager application at initialize | ✓ (operator override) |

**User's choice:** Apply everything recognized — tension with full-menu/upfront resolved next.

### Reconciliation

| Option | Description | Selected |
|--------|-------------|----------|
| Advertise all, apply as-landed | Full menu; keys without handlers = logged pending no-ops | ✓ |
| Implement all handlers now | Absorbs Phases 17–19 scope | |
| Revert to minimal menu | Discard full-menu preference | |

**User's choice:** Advertise all, apply as-landed (Recommended)

### Follow-ups

| Option | Description | Selected |
|--------|-------------|----------|
| Planner from reqs | Membership enumerated at plan time from REQUIREMENTS | ✓ |
| Hand-enumerate now | Lock list in CONTEXT before planning | |

**User's choice:** Planner from reqs (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Project layer | .ass-guard project config | |
| Global layer | ~/.ass-guard | |
| Free-text | "2 configs: global and project. we can edit both, default project" | ✓ |

**User's choice:** free-text — both layers editable, project default.

| Option | Description | Selected |
|--------|-------------|----------|
| Wire-level scope param | Global via option-schema scope | ✓ |
| Separate command later | /config global (Phase 20) | |
| Planner decides | After ACP schema research | |

**User's choice:** Wire-level scope param (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Persist-then-apply | Write succeeds before live apply | ✓ |
| Apply-then-persist | Best-effort background write | |

**User's choice:** Persist-then-apply (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Advertise effective value | Current resolved value in configOptions | ✓ |
| Static defaults only | Leaner schema | |

**User's choice:** Advertise effective value (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Blob fills unset | Explicit config wins | ✓ |
| Blob overrides | Editor wins every initialize | |

**User's choice:** Blob fills unset (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| One chain, /model on top | session > time-window > project > global | ✓ |
| Editor over /model | Most-recent-editor-intent wins | |

**User's choice:** One chain, /model on top (Recommended)

---

## Client probing & degradation

Research basis: probe→error→fallback canonical in [MCP versioning](https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning); -32601 per [JSON-RPC 2.0](https://www.jsonrpc.org/specification).

| Option | Description | Selected |
|--------|-------------|----------|
| Eager at initialize | One probe, cached session-lifetime | ✓ |
| Lazy per-capability | First use pays discovery | |
| No probe, reactive | Degrade on -32601 per call | |

**User's choice:** Eager at initialize (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Timeout → retry → fallback | One retry then plain-text + log | ✓ |
| Timeout → fallback, no retry | Fast recovery, flap-prone | |
| Error + no auto-fallback | Operator decides | |

**User's choice:** Timeout → retry → fallback (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Monotonic ints out, tolerant in | JSON-RPC canonical | |
| UUIDs both ways | Matches Zed inbound style | ✓ (operator override) |

**User's choice:** UUIDs both ways

| Option | Description | Selected |
|--------|-------------|----------|
| Logs + counters | stderr structured + in-process counters | ✓ |
| OTel semconv now | Telemetry stack import | |

**User's choice:** Logs + counters (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Two classes | FAST-CONTROL ~10s vs HUMAN-ASK minutes | ✓ |
| Single uniform timeout | One value | |

**User's choice:** Two classes (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Sticky per session | No re-probe backoff | ✓ |
| Re-probe with backoff | Recovers transient confusion | |

**User's choice:** Sticky per session (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Support now | Registry ships cancel resolution; 17 calls it | ✓ |
| Defer to Phase 17 | Resolve-by-response only now | |

**User's choice:** Support now (Recommended)

---

## Transcript schema stability

Research basis: additive-only + weak schemas over version fields ([Event-Driven.io](https://event-driven.io/en/simple_events_versioning_patterns/), [Burning Monk critique](https://www.linkedin.com/posts/theburningmonk_most-event-schema-versioning-strategies-don-activity-7385625451555168256-AB5m)).

| Option | Description | Selected |
|--------|-------------|----------|
| Additive-only weak schema | Kinds append; unknown tolerated; no version field | ✓ |
| Versioned envelope | schema_version + upcasting | |
| Strict per-kind freeze | Any change = new kind | |

**User's choice:** Additive-only weak schema (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Rich boundary record | id + timestamp + usage + pre/post pointers | ✓ |
| Minimal marker | kind + timestamp | |

**User's choice:** Rich boundary record (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Full invocation record | key + args + source chain + outcome | ✓ |
| Key + args only | Smaller lines | |

**User's choice:** Full invocation record (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Type-level bypass | Distinct code path; redactor never sees thinking | ✓ |
| Redactor allowlist | Config-driven exemption | |

**User's choice:** Type-level bypass (Recommended)

---

## Deferred Ideas

None — discussion stayed within phase scope.
