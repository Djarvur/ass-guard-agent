# Curated Divergence Suite — Rationale

**Phase 1 north-star gate substrate (MIMC-03/MIMC-04, D-01/D-03).**

This document records, per prompt, *why* it is divergence-prone and what a weaker
shaper would do differently. The gate (Plan 01-06 T6) is a 100% pass on BOTH
layers (sequence equality AND per-tool argument structural equality). A suite of
trivial prompts that always match would be a self-deception risk (D-03); this
suite deliberately probes the divergence-prone paths.

## Source strategy (D-16)

Per D-16, the parity-reference session is **disjoint** from the profile-extraction
source. The profile (request shape: system blocks, tools, headers) is extracted
from the main session `016eee8a` (103 tools). The parity-reference multi-tool
sequences below are informed by the captured tool-selection behavior of the
disjoint subagent session `subagent_agent_5b30d6eb` (97 tools; turns observed
included `[Skill,Read]`, `[Read,Read,Read]`, `[Read,Read,Bash]`,
`[TodoWrite,Read,Read,Bash]`, `[Bash,Read,Read]`).

Per D-01, the prompts themselves are **hand-authored** (clean single-turn
instructions) rather than replayed verbatim — the captured subagent turns were
tool-result-driven multi-turn follow-ups (last message role=`tool`, shared
turnId), which are not reproducible by the Phase-1 single-turn test-harness loop
(D-12). The expected tool-call *sequences* are the researcher's curated
expectation, shaped after the real captured multi-tool patterns; the LIVE gate
(T6) validates that ass-guard reproduces them at temp=0 against Z.ai GLM.

## Per-prompt rationale

| turn_id | Pattern probed | A weaker shaper would... |
|---|---|---|
| div-01-multi-read-parallel | multi-tool parallel reads | batch into one Read, or pick a different read order than the three-file parallel fan-out |
| div-02-read-then-bash | mixed read + mutating sequencing | collapse/ misorder the Read→Read→Bash sequence; skip the inspection before executing |
| div-03-plan-plus-reads-plus-build | long 4-tool mixed sequence | drop TodoWrite, or reorder the reads/build, or skip the planning step entirely |
| div-04-ambiguous-read-vs-grep | ambiguous tool selection | pick Read-with-guessing instead of Grep for a search-for-definition task |
| div-05-single-bash-mutating | single mutating tool (baseline) | pick a non-Bash tool or wrap in a needless read; this baseline MUST pass before complex sequences are meaningful |
| div-06-skill-dispatch | Skill-tool dispatch + read | omit the Skill call (declares Skill tool wrongly) and jump to a plain Read |
| div-07-zero-tool-direct-answer | zero-tool direct answer | spuriously call Read/Grep on an "answer from memory" prompt (over-declaration failure) |
| div-08-bash-reads-inspection | Bash-then-parallel-Reads | inline the discovery (skip Bash) or read before discovering the file set |

## Coverage

The suite covers 8 of the 6 divergence-probe patterns enumerated in RESEARCH §3.3
(multi-tool sequencing, ambiguous selection, MCP/plugin selection is not directly
probed here — it is exercised by the catalog's MCP tail in the profile itself,
not by the parity arm; tool-arg edge cases are exercised via the metric's
normalization rules; plan-mode transitions are covered by TodoWrite in div-03;
zero-tool is div-07). ≥4 patterns are covered (the D-01/D-03 requirement).

## What the gate proves

PASS (100% on BOTH layers across all 8 prompts) ⇒ the mimicry thesis is
empirically supported at temp=0 for the curated divergence set: ass-guard, with
the zcode profile loaded, reproduces zcode's tool-SELECTION behavior for these
divergence-prone prompts. FAIL ⇒ stop-and-replan (PROJECT.md Anti-Pattern 5); a
specific failing turn + its Layer-1/Layer-2 mismatch is the re-plan input.
