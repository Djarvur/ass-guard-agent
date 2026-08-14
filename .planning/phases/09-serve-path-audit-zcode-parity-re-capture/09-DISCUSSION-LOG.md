# Phase 9: Serve-Path Audit + zcode Parity Re-capture - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-14
**Phase:** 9-serve-path-audit-zcode-parity-re-capture
**Areas discussed:** Audit verbosity, Audit file layout, Engine-event detail, Re-capture workload

---

## Audit verbosity

| Option | Description | Selected |
|--------|-------------|----------|
| Metadata + body store | Events carry IDs/shapes/hashes; full redacted bodies in the capped store, retrievable by hash | ✓ |
| Full bodies inline | Easiest debugging; ~80 KB/turn × hands-off multiplication = hundreds of MB/day | |
| Config-toggled | Switch between modes, inline default | |

**User's choice:** Metadata + body store

---

## Audit file layout

| Option | Description | Selected |
|--------|-------------|----------|
| Per-session files | One JSONL per session under `.ass-guard/audit/`; aligns with per-session transcripts; natural retention boundaries | ✓ |
| Single rolling file | Simple tailing; interleaved sessions | |
| Operator path + per-session default | `--audit-log` override with per-session fallback | |

**User's choice:** Per-session files (operator override noted in CONTEXT D-02)

---

## Engine-event detail

| Option | Description | Selected |
|--------|-------------|----------|
| Full provenance | Action + matched signal + matched text span + source turn + config source — log alone answers "why did it continue" | ✓ |
| Action + signal only | Minimal; investigating needs the transcript too | |

**User's choice:** Full provenance

---

## Re-capture workload

| Option | Description | Selected |
|--------|-------------|----------|
| Scripted workload | Divergence-prone (subagents, MCP attach/detach, tool variety) run once in real zcode; session pins the test | ✓ |
| Natural usage + richest | Research flags as selection-biased (PickRichestMain vacuous-proof risk) | |
| Both | Scripted pins, natural validates informally | |

**User's choice:** Scripted workload

---

## Claude's Discretion

- CurrentTurnID() accessor vs empty-TurnID fallback (acceptable v1.1 fallback identified)
- Captured-provider helper shape; body-store cap size and eviction policy
- Audit sink rotation mechanics beyond per-session boundaries
- The scripted re-capture runbook's exact prompt sequence

## Deferred Ideas

- OTLP export, raw-body opt-in mode — Future Requirements
- Audit-driven parity fingerprints in production — v1.1.x candidate
- Retention automation beyond per-session file boundaries — in-plan discretion
