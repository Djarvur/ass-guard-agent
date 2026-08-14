# Phase 0: Spike + Re-verification - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-09
**Phase:** 0-Spike + Re-verification
**Areas discussed:** Verification registry, Proof strength, whisper.cpp #4 scope, Negative-finding protocol

---

## Verification registry

Where the `{fact, source, verified_date, verified_against_version}` records (success criterion #4) live and what shape.

| Option | Description | Selected |
|--------|-------------|----------|
| VERIFIED-FACTS.md (new) | New `.planning/research/VERIFIED-FACTS.md`; prose + dated table per fact. STACK.md stays as the pre-spike recommendation. | ✓ |
| Append to STACK.md | New "Phase-0 Re-verification Results" section appended to STACK.md, updating items in place. | |
| Structured YAML/JSON | Machine-readable file (e.g. `verified-facts.yaml`) with each fact as a keyed record. | |
| You decide | Pick the best fit given the "dated evidence is load-bearing" stance. | |

**User's choice:** VERIFIED-FACTS.md (new)
**Notes:** A dedicated artifact signals "this is the source of truth" and keeps the pre-spike recommendation (STACK.md) cleanly separated from the post-spike verified record. Phase 1+ reads VERIFIED-FACTS.md without pulling all 52KB of STACK.md.

**Follow-up — file structure:**

| Option | Description | Selected |
|--------|-------------|----------|
| One section per fact | `## Fact N` with Fact/Source/Verified/Verified against/Status/Evidence/Notes fields. Room for evidence refs and notes. | ✓ |
| Single summary table | One markdown table; compact, scannable; loses space for evidence/notes. | |
| Table + per-fact detail | Summary table at top + per-fact sections below. | |
| You decide | Choose structure serving Phase 1's need for schema specifics. | |

**User's choice:** One section per fact
**Notes:** Phase 1's researcher needs JSONL schema specifics and spike observations, not just a status check — per-fact sections give that room.

**Follow-up — where captured samples live:**

| Option | Description | Selected |
|--------|-------------|----------|
| Inline in VERIFIED-FACTS.md | Small samples as fenced code blocks; self-contained; risk of bloat/sensitive content. | ✓ |
| External files under research/captures/ | Raw captures as separate files, referenced by path; lean doc; must sanitize. | |
| Hybrid | Snippet inline + full raw capture as external referenced file. | |
| You decide | Keep VERIFIED-FACTS.md trustworthy without leaking sensitive content. | |

**User's choice:** Inline in VERIFIED-FACTS.md
**Notes:** With the caveat that inline samples must be representative excerpts, sanitized/redacted of prompts/tokens/PII before commit, with redaction noted. Captured as D-03.

---

## Proof strength

Does Phase 0 produce any Go code at all, or stay docs-only? Items #1 (sample) and #5 (integration test) resist doc-only treatment; #2 (schema fidelity) is empirical; #3 (method names) is a lookup.

| Option | Description | Selected |
|--------|-------------|----------|
| Docs-only | VERIFIED-FACTS.md captures all; #1/#5 become manual observations/recipes. | |
| Throwaway spikes/ dir | Disposable Go programs under `spikes/`; NOT the start of the codebase. | ✓ |
| Real scaffolding | Start the real module structure (root go.mod, internal/, ACP skeleton). | |
| You decide | Assign proof strength by whether the claim is empirical vs. a lookup. | |

**User's choice:** Throwaway spikes/ dir
**Notes:** #5 is an integration test per the roadmap — only a running process proves the stdout discipline holds. #2 schema fidelity is empirical. Spikes stay isolated from the real codebase.

**Follow-up — spikes/ module layout:**

| Option | Description | Selected |
|--------|-------------|----------|
| Own go.mod, gitignored | `spikes/` has its own module; source + README committed, go.sum/binaries ignored; repo root clean until Phase 1. | ✓ |
| Own go.mod, committed | `spikes/` go.mod + go.sum both committed; reproducible. | |
| Repo-root go.mod shared | Single root go.mod, spikes/ a subpackage; Phase 1 inherits. | |
| You decide | Honor "throwaway proof, not the codebase" while keeping evidence reproducible. | |

**User's choice:** Own go.mod, gitignored
**Notes:** Strongest isolation. The real module starts fresh in Phase 1 with clean dependency decisions, uncoupled from what the spikes pinned.

**Follow-up — per-item proof assignment:**

| Option | Description | Selected |
|--------|-------------|----------|
| #2, #3, #5 spike; #1 capture, #4 | #1 = filesystem capture; #2 = spike; #3 = doc-pin; #4 = per whisper scope; #5 = spike. | |
| All 5 get spikes/captures | Maximum rigor; even #3 gets a handshake spike. | |
| Only #2 and #5 spike | Minimal; #2 + #5 spikes, #1 capture, #3 doc-pin, #4 per whisper scope. | |
| You decide | Assign proof per item by empirical vs. lookup. | |

**User's choice:** "#2, #3, #5 spike; #1 capture, #4" (the recommended option's label) — but in the follow-up the user effectively chose to spike #3 as well. Recorded as: #1 capture, #2 spike, #3 spike, #5 spike, #4 pending whisper scope.
**Notes:** The user wants #3 (ACP method names) verified with a running handshake, not just doc-pinned. Cost is low; ACP wire shape is load-bearing for Phase 2.

---

## whisper.cpp #4 scope

STACK #4 is conditional ("if local STT is in scope"). The v1 cut-line defaults STT to OpenAI Whisper API and defers Telegram (the sole STT consumer) to v2 — so STT is doubly off the v1 critical path.

| Option | Description | Selected |
|--------|-------------|----------|
| Defer entirely out of Phase 0 | Record #4 as DEFERRED; closes 4/5 items. | |
| Shallow desk-check, no spike | 30-min documented check; note deferral of real test. | |
| Full spike now | Build whisper-cli for the 4-platform matrix; verify goreleaser. | |
| You decide | Weigh load-bearing-for-v1 vs. honest deferral. | |

**User's choice:** (free-text) "I would say we can use external utility for STT, so no crosscompile needed"
**Notes:** This is stronger than any offered option — it's an architectural rule, not a deferral. STT is ALWAYS an external utility (OpenAI API = network call; whisper.cpp = local subprocess). ass-guard never bundles STT via cgo. Therefore the binary stays pure-Go and goreleaser cross-compile is unaffected. Captured as D-06.

**Follow-up — how #4 is recorded:**

| Option | Description | Selected |
|--------|-------------|----------|
| Record as STRUCTURALLY-MOOT | #4 status = STRUCTURALLY-MOOT; closed by the external-utility rule. Closes 5/5. | ✓ |
| Record as DEFERRED-to-v2 | Defer whisper-cli integration test to whenever local STT lands. | |
| You decide | Whether "STT is always external" fully closes #4 or defers its integration test. | |

**User's choice:** Record as STRUCTURALLY-MOOT
**Notes:** Phase 0 closes 5/5 items. The cross-compile premise does not arise under the external-utility architecture.

---

## Negative-finding protocol

What happens when a Phase 0 fact FAILS re-verification? Shaped by the project's stop-and-replan culture (Phase 1 gates everything; PITFALLS N18; PROJECT.md Anti-Pattern 5).

| Option | Description | Selected |
|--------|-------------|----------|
| Severity-tiered | (A) doc/minor → record + correct + continue; (B) architectural → record + stop + ask user. | ✓ |
| Always document-and-continue | Record every failure, never halt; risks silent propagation of architectural failures. | |
| Always halt-and-replan | Any failure stops the phase; overreactive for cosmetic drift. | |
| You decide | Fit the project's risk profile (north-star thesis, serialized deltas). | |

**User's choice:** Severity-tiered
**Notes:** Two tiers — doc/minor failures are just updates (continue), architectural failures are human-decision events (stop + ask). Avoids both silent propagation and overreaction.

**Follow-up — architectural-tier escalation action:**

| Option | Description | Selected |
|--------|-------------|----------|
| Stop + ask user | Write FAILED finding, stop, present revise-and-continue / halt-and-replan / accept-and-document. | ✓ |
| Stop + auto-create replan task | Write finding, stop, create replan backlog entry, wait. | |
| Stop + halt cold | Write finding, stop, report "manual intervention required", exit. | |
| You decide | Respect "human decides on architectural surprises" while keeping evidence. | |

**User's choice:** Stop + ask user
**Notes:** Three explicit options presented to the user at the halt point. No silent propagation; no presumption of the replan path before the user weighs in. Captured as D-07.

---

## Claude's Discretion

- VERIFIED-FACTS.md section wording beyond the D-02 field schema.
- The specific prompt(s) the #2 tool-schema spike sends to MiniMax M3 / Groq (any minimal tool-calling request exercising the schema).
- The precise structure of the #5 stdout-collision test (goroutine layout, assertion mechanism).
- Whether the #3 ACP handshake also doc-pins the spec version (welcome, not required).

## Deferred Ideas

None — discussion stayed within Phase 0 scope. The whisper.cpp end-to-end integration test is obviated for v1 by D-06 (STT always external); should local STT land in v2+, that test lands then, but the cross-compile concern it addressed stays closed.
