---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
current_phase: 0
current_phase_name: Spike + Re-verification
status: executing
last_updated: "2026-08-09T17:52:26.854Z"
progress:
  total_phases: 1
  completed_phases: 0
  total_plans: 5
  completed_plans: 2
---

# State: ass-guard-agent (working name)

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-09)
**Core value:** Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first)
**Current focus:** Phase 0 (in progress — Plan 00-01 + 00-02 complete; 2/5 plans)

## Current Phase

**Phase:** 0 — Spike + Re-verification
**Status:** Executing (Wave 1 done; Wave 2: Plan 00-02 done, Plans 00-03/00-04 pending)
**Next action:** Execute remaining Wave 2 plans 00-03/00-04 (parallel) once dispatched
**Last session:** Plan 00-02 complete — #2 go-openai tool-calling schema spike: schema fidelity (Chat Completions tools[].function wrapper, string-typed arguments, tool-role result message) VERIFIED offline via RED→GREEN test suite; live provider round-trip recorded PARTIAL (MiniMax/Groq keys both unset at exec — Tier-A fallback). See `.planning/phases/00-spike-re-verification/00-02-SUMMARY.md`

## Phase Status

| Phase | Status | Notes |
|-------|--------|-------|
| 0 — Spike + Re-verification | Executing (2/5 plans) | Plans 00-01 + 00-02 done: spikes module + #1 JSONL capture + #4 STT closure + #2 go-openai tool-schema spike (schema VERIFIED offline, live round-trip PARTIAL pending keys). Next: Wave 2 Plans 00-03/00-04, then Wave 3 (Plan 05 authors VERIFIED-FACTS.md). |
| 1 — Mimicry MVP (north-star proof) | Not started | Gates everything; A/B parity test must pass before Phase 2. 16 REQ-IDs. Highest research depth. |
| 2 — Session Core + ACP Interface | Not started | 18 REQ-IDs. Needs Phase 1. |
| 3 — Model Scheduling | Not started | 6 REQ-IDs. Needs Phases 1-2. |
| 4 — Unified Engine + Hook-DAG + OpenSpec + Learning | Not started | 19 REQ-IDs. Needs Phases 1-3. Project's reason to exist. |
| 5 — Ecosystem Compatibility | Not started | 5 REQ-IDs. Needs stable tool registry (Phase 2). |
| 6 — Distribution + Polish | Not started | 3 REQ-IDs. Ship readiness after all deltas validated. |

## Decisions Log

- **Plan 00-01 (2026-08-09):** STACK item #1 (zcode JSONL path) is Tier-B-favorable → D-07 (a) revise-and-continue, confirmed on disk. Corrected path: `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl` (STACK's `~/.claude/projects/<munged-cwd>/...` is a different product, Claude Code, `queue-operation` schema). Schema richer than STACK claimed (full wire-level capture; no MITM needed for request body). Explicit user sign-off on MIMC-02 wording correction lands in Plan 00-05.
- **Plan 00-01 (2026-08-09):** STACK item #4 (whisper.cpp cross-compile) closed STRUCTURALLY-MOOT per D-06 — STT always external (HTTPS or out-of-process `whisper-cli` subprocess), zero cgo, goreleaser matrix unaffected. No spike produced.

## Blockers

(none)

---

*State initialized: 2026-08-09 after roadmap creation*

## Performance Metrics

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 00 P01 | 9min | 3 tasks | 5 files |
| Phase 00 P02 | 6min | 2 tasks | 3 files |

## Decisions

- [Phase ?]: Plan 00-02 (2026-08-09): STACK item #2 (go-openai tool-calling schema) — schema fidelity VERIFIED offline (Chat Completions tools[].function wrapper, string-typed arguments, tool-role result message, distinct from newer Responses API). Live round-trip recorded PARTIAL: MINIMAX_API_KEY/GROQ_API_KEY both unset at exec (Tier-A fallback, D-07). VERIFIED requires operator to export a key + re-run spikes/02-openai-toolschema. Plan 00-05 folds RESULT.md into VERIFIED-FACTS.md item #2.
- [Phase ?]: Plan 00-02 (2026-08-09): go-openai@v1.42.0 targets Chat Completions (tools[].function, tool_calls, role:tool) — NOT the newer Responses API shape (output[].function_call). Phase 1's OpenAI-shape provider adapter conforms to the Chat Completions schema; must not drift to Responses API. pkg.go.dev cadence lag confirmed real but not stagnation (GitHub active: v1.40→v1.42 over May-Aug 2026).
