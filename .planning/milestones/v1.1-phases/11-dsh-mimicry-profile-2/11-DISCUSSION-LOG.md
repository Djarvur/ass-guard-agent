# Phase 11: dsh Mimicry Profile #2 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-14
**Phase:** 11-dsh-mimicry-profile-2
**Areas discussed:** Endpoint target, Profile lifecycle, Parity bar, Selection UX

---

## Endpoint target

| Option | Description | Selected |
|--------|-------------|----------|
| Official API | DeepSeek's official API with operator key as canonical probe target; gateway as config alternative | |
| opencode gateway | The operator's actual access path (subscription gateway); requires OpenAI-compatible endpoint exposure | ✓ |
| Both | Both endpoints configured and probed | |

**User's choice:** opencode gateway
**Notes:** Official API stays config-swappable via base URL — nothing hardcodes the gateway.

---

## Profile lifecycle

| Option | Description | Selected |
|--------|-------------|----------|
| Pin + drift guard | One capture against a pinned dsh commit; drift check guards; re-capture is a documented manual procedure | ✓ |
| Scheduled re-capture | Documented ritual every N dsh releases | |
| Capture + alarm only | No documented re-capture procedure | |

**User's choice:** Pin + drift guard

---

## Parity bar

| Option | Description | Selected |
|--------|-------------|----------|
| Full behavioral bar | Same statistical A/B standard as zcode — tool-call sequences indistinguishable from live dsh | ✓ |
| Structural-only bar | Request-shape match suffices for v1.1; behavioral A/B deferred | |
| Staged gates | Structural gate closes the phase; behavioral as follow-up gate | |

**User's choice:** Full behavioral bar

---

## Selection UX

| Option | Description | Selected |
|--------|-------------|----------|
| Flag-only | Explicit --profile dsh only for v1.1 | |
| Flag + project default | Per-project default profile in .ass-guard/ config | |
| Tier-driven | Scheduler tiers map to profiles | |
| **Per-model in config** (freeform) | Model entries in scheduling.yaml declare their profile | ✓ |

**User's choice (freeform):** "profile is cpecified per-model in config"
**Notes:** Refines DSH-04 — amended in REQUIREMENTS.md: per-model `profile` field in scheduling.yaml (DeepSeek → dsh); `--profile` flag remains as override; per-turn switching stays v1.2.

---

## Claude's Discretion

- Recording-proxy implementation shape; zstd harvest tooling details
- dsh capture workload prompt sequence (divergence-prone, mirrors Phase-9 runbook discipline)
- Redaction genericization mechanics; dialect handling keyed by capability profile

## Deferred Ideas

- Per-turn profile switching — v1.2 non-goal
- dsh Responses-API shape — only if verification demands
- Scheduled re-capture automation — revisit if dsh stabilizes
- dsh UI/plugin-runtime mimicry — permanent non-goal (model-wire surface only)
