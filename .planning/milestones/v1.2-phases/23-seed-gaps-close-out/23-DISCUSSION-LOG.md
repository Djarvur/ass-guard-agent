# Phase 23: SEED Gaps Close-out - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-27
**Phase:** 23 - SEED Gaps Close-out
**Areas discussed:** Steering injection form, Parked-ask handling, Checkpoint GC + retention, /undo depth

**Context:** Discussed inline while Phase 16 plans in background. Research-before-questions enabled.

---

## Steering injection form

Research basis: converged agent pattern — steering delivers at the next break-point as a user message, often reminder-wrapped; queued inputs batch in arrival order ([PicoClaw](https://docs.picoclaw.io/docs/steering/), [OpenClaw](https://docs.openclaw.ai/concepts/queue-steering), [ACP discussion #1220](https://github.com/agentclientprotocol/agent-client-protocol/discussions/1220)).

| Option | Description | Selected |
|--------|-------------|----------|
| User msg + marker | Reminder-wrapped user-role message; steering distinguishable | ✓ |
| Plain user message | Indistinguishable from turns in replay | |

**User's choice:** User msg + marker (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Coalesce, arrival order | One delivery per boundary; bounded growth | ✓ |
| One per boundary | Per-input timing; more requests touched | |

**User's choice:** Coalesce, arrival order (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Note + transcript record | Visible live; reconstructable | ✓ |
| Record only | Durable but invisible | |

**User's choice:** Note + transcript record (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Steer ≠ cancel | Absorb and continue; cancel stays separate | ✓ |
| Heuristic preempt | Correction-looking text preempts — surprising | |

**User's choice:** Steer ≠ cancel (Recommended)

---

## Parked-ask handling

| Option | Description | Selected |
|--------|-------------|----------|
| Note now, fire later | Parked note immediately; full ask at queue position | ✓ |
| Silent until fired | Discovered only at turn end | |

**User's choice:** Note now, fire later (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Cancelable while parked | Resolves cancelled-normal; turn survives | ✓ |
| No early cancel | Wait, then answer or decline | |

**User's choice:** Cancelable while parked (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Parking never blocks | Delivery order only; active turn untouched | ✓ |
| Origin turn waits | Suspension blocks the turn | |

**User's choice:** Parking never blocks (Recommended)

---

## Checkpoint GC + retention

| Option | Description | Selected |
|--------|-------------|----------|
| Age-based, 7d | Grace-family precedent | |
| Count-based | Bounded per session; loses old points | |
| Age + count | Whichever binds first | ✓ (operator choice) |

**User's choice:** Age + count

| Option | Description | Selected |
|--------|-------------|----------|
| Snapshots = checkpoints | Enter same GC; /undo-able restores | ✓ |
| Single undo slot | One level of restore-undo | |

**User's choice:** Snapshots = checkpoints (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| configOptions | expiry_days + max_per_session tunable | ✓ |
| Constants | Compile-time | |

**User's choice:** configOptions (Recommended)

---

## /undo depth

| Option | Description | Selected |
|--------|-------------|----------|
| Stack walk + /undo N | Repeated undo walks back; N jumps | ✓ |
| Single-step | Only the last checkpoint | |

**User's choice:** Stack walk + /undo N (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Guards = message | Refuse while active; guard is the UX | |
| Auto-cancel then undo | Cancel, drain, snapshot, restore | ✓ (operator override) |

**User's choice:** Auto-cancel then undo — NOT the recommendation; nested-repo refusal still refuses.

---

## Deferred Ideas

None — discussion stayed within phase scope.
