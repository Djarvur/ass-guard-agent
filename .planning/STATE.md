---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: executing
last_updated: "2026-08-09T18:20:42.612Z"
progress:
  total_phases: 7
  completed_phases: 1
  total_plans: 5
  completed_plans: 5
  percent: 14
---

# State: ass-guard-agent (working name)

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-09)
**Core value:** Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first)
**Current focus:** Phase 0 COMPLETE (5/5 plans); Phase 1 (Mimicry MVP) ready to plan

## Current Phase

**Phase:** 0 — Spike + Re-verification (COMPLETE)
**Status:** Complete (5/5 plans; Wave 1 + Wave 2 + Wave 3 all done). VERIFIED-FACTS.md is the post-spike source of truth; STACK.md is the unchanged pre-spike recommendation (D-01); the one Tier-B finding (#1 zcode JSONL path) is resolved.
**Next action:** Plan Phase 1 (Mimicry MVP — north-star proof). Carry-forward: (1) correct MIMC-02 path wording to `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl` during Phase 1 planning (option-a consequence; munged-cwd obsolete for zcode); (2) operator provisions `MINIMAX_API_KEY`/`GROQ_API_KEY` to flip item #2 PARTIAL → VERIFIED (schema VERIFIED offline already).
**Last session:** Plan 00-05 complete — authored `.planning/research/VERIFIED-FACTS.md` (post-spike source of truth) by folding the four Wave-1 evidence files into 5 D-02 sections (#1 FAILED/Tier-B-resolved, #2 PARTIAL, #3 VERIFIED, #4 STRUCTURALLY-MOOT, #5 VERIFIED). D-03 sanitized across the whole file (zero sk-/Bearer/*_API_KEY=/raw UUID//Users/ matches — threat T-00-10 mitigated). STACK.md byte-identical to pre-phase (md5 preserved; D-01 honored). Gate `check-verified-facts.sh` exits 0; `<verify>` chain prints PHASE0_GATE_PASS. Tier-B resolution recorded HONESTLY: research-predicted default option-a (revise-and-continue) applied by default after the user declined to override the checkpoint — NOT 'user chose'. Phase 0 closes 5/5 STACK items. See `.planning/phases/00-spike-re-verification/00-05-SUMMARY.md`

## Phase Status

| Phase | Status | Notes |
|-------|--------|-------|
| 0 — Spike + Re-verification | **COMPLETE (5/5 plans)** | All 5 plans done. VERIFIED-FACTS.md authored (post-spike source of truth): #1 zcode JSONL FAILED→revise-and-continue [Tier-B resolved], #2 go-openai PARTIAL [schema VERIFIED offline; live round-trip deferred pending operator keys], #3 ACP v1 VERIFIED [Tier-A session/new correction], #4 whisper.cpp STRUCTURALLY-MOOT [D-06], #5 go-telegram stdout VERIFIED [Tier-A handler-signature refinement]. STACK.md unchanged (D-01). Gate exits 0 (PHASE0_GATE_PASS). D-03 sanitized. Phase 0 closes 5/5 STACK items. Next: Phase 1 planning (carries the MIMC-02 path correction + the operator-key unblock for #2). |
| 1 — Mimicry MVP (north-star proof) | Not started | Gates everything; A/B parity test must pass before Phase 2. 16 REQ-IDs. Highest research depth. |
| 2 — Session Core + ACP Interface | Not started | 18 REQ-IDs. Needs Phase 1. |
| 3 — Model Scheduling | Not started | 6 REQ-IDs. Needs Phases 1-2. |
| 4 — Unified Engine + Hook-DAG + OpenSpec + Learning | Not started | 19 REQ-IDs. Needs Phases 1-3. Project's reason to exist. |
| 5 — Ecosystem Compatibility | Not started | 5 REQ-IDs. Needs stable tool registry (Phase 2). |
| 6 — Distribution + Polish | Not started | 3 REQ-IDs. Ship readiness after all deltas validated. |

## Decisions Log

- **Plan 00-05 (2026-08-09):** D-07 Tier-B resolution for item #1 (zcode JSONL path) — research-predicted default **option-a (revise-and-continue) APPLIED BY DEFAULT** after the user declined to override the checkpoint across multiple prompts (NOT 'user chose option-a' — honest audit-trail attribution). Confirmed by on-disk evidence from plans 00-01..00-04 and documented as the prediction in 00-05-PLAN.md Task 2. Consequence (option-a, verbatim): Phase 1 MIMC-02 path wording to be corrected to `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl` during Phase 1 planning; munged-cwd convention obsolete for zcode. Reversible at Phase 1 planning. Recorded in VERIFIED-FACTS.md item #1 Notes.
- **Plan 00-05 (2026-08-09):** Phase 0 COMPLETE (5/5 plans, 5/5 STACK items closed). VERIFIED-FACTS.md is the post-spike source of truth (STACK.md unchanged, D-01 — md5 `5e4eecc8418f9ff61272702044da0f54` preserved). The completeness gate `check-verified-facts.sh` exits 0 (6/6 checks); the plan `<verify>` chain prints PHASE0_GATE_PASS. D-03 sanitization is the phase's final secret-leak barrier (threat T-00-10, high) — zero sk-/Bearer/*_API_KEY=/raw UUID//Users/ matches across the merged file. Item #2 carries forward as PARTIAL: schema VERIFIED offline, live round-trip deferred pending operator key provisioning (single carry-forward operator action). Phase 1 (Mimicry MVP) is ready to plan.
- **Plan 00-01 (2026-08-09):** STACK item #1 (zcode JSONL path) is Tier-B-favorable → D-07 (a) revise-and-continue, confirmed on disk. Corrected path: `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl` (STACK's `~/.claude/projects/<munged-cwd>/...` is a different product, Claude Code, `queue-operation` schema). Schema richer than STACK claimed (full wire-level capture; no MITM needed for request body). Explicit user sign-off on MIMC-02 wording correction lands in Plan 00-05.
- **Plan 00-01 (2026-08-09):** STACK item #4 (whisper.cpp cross-compile) closed STRUCTURALLY-MOOT per D-06 — STT always external (HTTPS or out-of-process `whisper-cli` subprocess), zero cgo, goreleaser matrix unaffected. No spike produced.
- **Plan 00-04 (2026-08-09):** STACK item #5 (go-telegram/bot + ACP stdout collision) VERIFIED — go-telegram/bot@v1.23.0 is silent-by-default (zero os.Stdout writes in package source; all output via 3 log.Printf→os.Stderr handlers; debug path gated behind WithDebug; no WithLogger/io.Writer option), confirmed by direct source inspection + the integration spike's captured-os.Stdout byte-equality assertion (415==415 bytes, 0 extra) + deterministic handler-routing probe. Transport discipline (stdout = ACP only) holds for the multi-frontend case. Tier-A refinement: actual callback signatures (ErrorsHandler func(err error), DebugHandler func(format, args...any)) are simpler than RESEARCH.md §5 documented — Phase 5 must use the real signatures. Mitigation recipe for v2 Telegram frontend recorded (WithErrorsHandler→slog→stderr, NO WithDebug, log.SetOutput(os.Stderr), go b.Start(ctx) goroutine).
- **Plan 00-04 (2026-08-09):** context-first shutdown contract confirmed for go-telegram/bot@v1.23.0 — bot goroutine exits within 2s of telegramCtx cancel (STACK's load-bearing reason for picking it over gotgbot/telebot). Wave 2 complete (4/4 spike plans); Plan 00-05 (Wave 3) can now fold all four evidence files into VERIFIED-FACTS.md.

## Blockers

(none)

---

*State initialized: 2026-08-09 after roadmap creation*

## Performance Metrics

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 00 P01 | 9min | 3 tasks | 5 files |
| Phase 00 P02 | 6min | 2 tasks | 3 files |
| Phase 00 P03 | 8min | 2 tasks | 3 files |
| Phase 00 P04 | 10min | 2 tasks | 3 files |
| Phase 00 P05 | 8min | 1 task | 4 files |

## Decisions

- [Phase ?]: Plan 00-02 (2026-08-09): STACK item #2 (go-openai tool-calling schema) — schema fidelity VERIFIED offline (Chat Completions tools[].function wrapper, string-typed arguments, tool-role result message, distinct from newer Responses API). Live round-trip recorded PARTIAL: MINIMAX_API_KEY/GROQ_API_KEY both unset at exec (Tier-A fallback, D-07). VERIFIED requires operator to export a key + re-run spikes/02-openai-toolschema. Plan 00-05 folds RESULT.md into VERIFIED-FACTS.md item #2.
- [Phase ?]: Plan 00-02 (2026-08-09): go-openai@v1.42.0 targets Chat Completions (tools[].function, tool_calls, role:tool) — NOT the newer Responses API shape (output[].function_call). Phase 1's OpenAI-shape provider adapter conforms to the Chat Completions schema; must not drift to Responses API. pkg.go.dev cadence lag confirmed real but not stagnation (GitHub active: v1.40→v1.42 over May-Aug 2026).
- [Phase ?]: Plan 00-03 (2026-08-09): STACK item #3 (ACP v1 method names + wire shape) — VERIFIED against the canonical spec (agentclientprotocol.com/protocol/v1/, fetched 2026-08-09). Framing is newline-delimited JSON-RPC (NOT LSP-style Content-Length headers; no embedded newlines; stdout = ACP only). Lifecycle is initialize → session/new → session/prompt (+ observe session/update) — Tier-A correction: STACK omitted the mandatory session/new step (you cannot send session/prompt without a sessionId from session/new). Result field is agentCapabilities (NOT capabilities/serverInfo). protocolVersion is integer 1 (not string). No discrepancy vs STACK on framing; the only correction is the omitted session/new step.
- [Phase ?]: Plan 00-03 (2026-08-09): ACP v1 wire shape (hand-rolled newline-delimited JSON-RPC, spec-pinned envelope builders) is copy-paste-safe starting point for Phase 2's ACP adapter. The in-process mock peer proved the wire shape deterministically without an external ACP server or model call (D-04 throwaway; critical_constraint #3/#7).
- [Phase ?]: Plan 00-04 (2026-08-09): STACK item #5 (go-telegram/bot + ACP stdout collision) — VERIFIED via the integration spike at spikes/05-stdout-collision/. go-telegram/bot@v1.23.0 is silent-by-default (zero os.Stdout writes in package source — only the examples/ samples write to stdout, not the library; all library output via 3 default handlers in bot.go all log.Printf→os.Stderr; debug path gated behind WithDebug; no WithLogger/io.Writer option). Captured real os.Stdout (os.Pipe) byte-equal to 3 canned ACP frames (415==415, 0 extra) while the bot goroutine ran concurrently; deterministic handler-routing probe (synthetic marker in stderr sink, absent from stdout). Transport discipline (stdout = ACP only) holds for the multi-frontend case.
- [Phase ?]: Plan 00-04 (2026-08-09): Tier-A refinement vs RESEARCH.md §5 — go-telegram/bot@v1.23.0 callback signatures are ErrorsHandler func(err error) and DebugHandler func(format string, args ...any) (SIMPLER than RESEARCH.md documented; no context.Context params). The load-bearing "callbacks, no stdout, no io.Writer" claim is unchanged. Phase 5's v2 Telegram frontend must use the real signatures + the 4-step mitigation recipe (WithErrorsHandler→slog→stderr, NO WithDebug, log.SetOutput(os.Stderr) belt-and-suspenders, go b.Start(ctx) goroutine; cancel telegramCtx on editor-initiated shutdown to drain the loop).
