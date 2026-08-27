# Phase 18: Session Family - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-26
**Phase:** 18 - Session Family
**Areas discussed:** Reconciliation architecture, List & pagination, Delete & tombstones, Resume CLI + close

**Context:** Discussed inline while Phase 15 executes and Phase 16 plans in background. Research-before-questions enabled.

---

## Reconciliation architecture

Research basis: event log as sole truth ([Fowler](https://martinfowler.com/eaaDev/EventSourcing.html)); snapshots as perf hybrid not correctness ([microservices.io](https://microservices.io/patterns/data/event-sourcing.html), [CodeOpinion](https://codeopinion.com/snapshots-in-event-sourcing-for-rehydrating-aggregates/)).

| Option | Description | Selected |
|--------|-------------|----------|
| Transcript-as-truth | Reconstruct from log + synthetic closures; no sidecar | ✓ |
| Hybrid sidecar | + live-state snapshot at boundaries | |
| Snapshot-primary | Restore IS the state | |

**User's choice:** Transcript-as-truth (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Marked synthetic | Interrupted marker, subtle UI, unambiguous disk | ✓ |
| Seamless | Looks like ordinary failure | |

**User's choice:** Marked synthetic (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Reconcile-then-accept | Linearizable resume | ✓ |
| Concurrent accept | Prompt mid-replay races closures | |

**User's choice:** Reconcile-then-accept (Recommended)

---

## List & pagination

Research basis: composite (key, id) cursors, stable sort ([keyset non-unique](https://medium.com/@george_16060/cursor-based-pagination-with-arbitrary-ordering-b4af6d5e22db), [tiebreaker checklist](https://mfaani.com/posts/interviewing/systems-design/pagination/)).

| Option | Description | Selected |
|--------|-------------|----------|
| On-demand header scan | mtime-ordered walk, first-line read | ✓ |
| Maintained index | id→header file | |

**User's choice:** On-demand header scan (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| lastActivity desc | (lastActivity, id) cursor, best-effort | ✓ |
| createdAt desc | Immutable, stale-first picker | |

**User's choice:** lastActivity desc (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Lean header | id, title, timestamps, flags | ✓ |
| Rich header | + excerpt, model, tokens, plan | |

**User's choice:** Lean header (Recommended)

---

## Delete & tombstones

Research basis: tombstone hides-not-erases; append-only uses markers ([soft delete](https://dev.to/sampseiol1/what-is-soft-delete-e3p), [crypto-shred](https://www.conduktor.io/glossary/crypto-shredding-for-kafka)); audit under separate policy ([analysis](https://medium.com/@annxsa/soft-deletes-in-php-the-pattern-that-quietly-corrupts-your-data-0c66d90812d3)); grace-then-purge lifecycle ([pattern](https://cadence.withremote.ai/blog/data-deletion-gdpr)).

| Option | Description | Selected |
|--------|-------------|----------|
| Marker file | Zero-byte .deleted; stat-filtered | ✓ |
| Tombstone line | Appended; scan can't see first-line | |
| Trash subdir | Path-based hiding | |

**User's choice:** Marker file (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Session artifacts, audit survives | Transcript+checkpoints+derived | ✓ |
| Everything incl. audit | Weakens D-20 | |

**User's choice:** Session artifacts, audit survives (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Reversible, no GC | Operator purges manually | |
| GC after grace | Auto-purge; follow-up pinned params | ✓ (operator chose GC) |
| One-way | Delete final | |

**User's choice:** GC after grace — NOT the recommendation; parameters resolved next.

| Option | Description | Selected |
|--------|-------------|----------|
| 30d default, audit exempt | Configurable grace; audit never purges | ✓ |
| 7d grace | Tighter, less undo window | |
| Opt-in GC | No default purge | |

**User's choice:** 30d default, audit exempt (Recommended)

---

## Resume CLI + close

Research basis: CC's documented trio — picker/id/continue ([official](https://code.claude.com/docs/en/sessions), [guide](https://pasqualepillitteri.it/en/news/366/claude-continue-resume-guide), [latest-shorthand request](https://github.com/anthropics/claude-code/issues/35599)).

| Option | Description | Selected |
|--------|-------------|----------|
| Full CC trio | picker + id + --continue/-c | ✓ |
| Resume only | picker + id | |
| Id required | Script-friendly, human-hostile | |

**User's choice:** Full CC trio (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Numbered list | stdin/stderr numbers, pipe-safe | ✓ |
| Full TUI | Raw-mode arrow keys | |

**User's choice:** Numbered list (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Cancel-and-drain | Cancel turns, drain asks, flush, close; idempotent | ✓ |
| Refuse while active | Busy + retry choreography | |

**User's choice:** Cancel-and-drain (Recommended)

---

## Deferred Ideas

- Dedicated session-restore (un-delete) surface beyond marker removal — post-v1.2 if wanted.
