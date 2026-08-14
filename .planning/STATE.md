---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: executing
last_updated: "2026-08-13T22:15:38.617Z"
progress:
  total_phases: 8
  completed_phases: 7
  total_plans: 36
  completed_plans: 34
  percent: 88
---

# State: ass-guard-agent (working name)

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-09)
**Core value:** Outgoing requests to the model provider must be structurally indistinguishable from the mimicked agent's (zcode first)
**Current focus:** Phase 6 COMPLETE — distribution + polish (go:embed zero-config defaults, goreleaser 4-target static build, canonical ACP registry agent.json). v1.0 milestone is SHIP-READY: full `mise ci` gate green (vet + golangci-lint v2 all-linters 0 issues + CGO_ENABLED=0 build + go test -race, all 24 packages).

## Current Phase

**Phase:** 6
**Status:** Ready to execute
**Next action:** v1.0 milestone ready to ship. Suggested: `/gsd:complete-milestone` + `/gsd:verify-work 06`. The full test suite passes under -race (the earlier Phase-1 data-source blocker `internal/profile.TestStability_WithinSessionExtractionSource` now PASSES — the rollout session is available again).
**Last session:** 2026-08-13T09:15:00.000Z

## Phase Status

| Phase | Status | Notes |
|-------|--------|-------|
| 0 — Spike + Re-verification | **COMPLETE (5/5 plans)** | All 5 plans done. VERIFIED-FACTS.md authored (post-spike source of truth): #1 zcode JSONL FAILED→revise-and-continue [Tier-B resolved], #2 go-openai PARTIAL [schema VERIFIED offline; live round-trip deferred pending operator keys], #3 ACP v1 VERIFIED [Tier-A session/new correction], #4 whisper.cpp STRUCTURALLY-MOOT [D-06], #5 go-telegram stdout VERIFIED [Tier-A handler-signature refinement]. STACK.md unchanged (D-01). Gate exits 0 (PHASE0_GATE_PASS). D-03 sanitized. Phase 0 closes 5/5 STACK items. Next: Phase 1 planning (carries the MIMC-02 path correction + the operator-key unblock for #2). |
| 1 — Mimicry MVP (north-star proof) | Not started | Gates everything; A/B parity test must pass before Phase 2. 16 REQ-IDs. Highest research depth. |
| 2 — Session Core + ACP Interface | Not started | 18 REQ-IDs. Needs Phase 1. |
| 3 — Model Scheduling | Not started | 6 REQ-IDs. Needs Phases 1-2. |
| 4 — Unified Engine + Hook-DAG + OpenSpec + Learning | **COMPLETE (7/7 plans)** | All 7 plans done across 4 waves (04-01/03/04 W1, 04-02/06 W2, 04-05 W3, 04-07 W4). 19 REQ-IDs implemented (ENG-01..05, HOOK-01..05, LRN-01..04, OPEN-01..03, TOOL-04/05). 5 new packages: internal/engine, internal/hookdag, internal/openspec, internal/learning, internal/toolexec + toolcat.Register + session.SetToolExecutor + acp_serve engine wiring + `ass-guard learning` CLI. End-to-end zero-continue OpenSpec scenario green (`cmd/ass-guard.TestEndToEnd_ZeroContinue`); structural safety (unmatched ⇒ nothing) proven end-to-end + via testing/quick property; cancel-drain (ENG-03) proven at the ACP level; hand-rolled hook-DAG ≤300 LOC core; swappable WebSearch/WebFetch backends. Phase gate GREEN: build + vet clean, 9 Phase-4 packages -race green, 0 TODO/FIXME. 04-VERIFICATION-PREP.md maps the 5 success criteria → tests + commands. Real-openspec tests gated behind ASSGUARD_OPENSPEC_BIN=1. Carry-forward: `internal/profile.TestStability_WithinSessionExtractionSource` still FAILS (pre-existing Phase-1 data-source blocker — Phase 4 did not touch internal/profile). Next: Phase 5 (Ecosystem Compatibility). |
| 5 — Ecosystem Compatibility | Not started | 5 REQ-IDs. Needs stable tool registry (Phase 2). |
| 6 — Distribution + Polish | **COMPLETE (2/2 plans)** | All 3 REQ-IDs (DIST-01/02/03) implemented. 06-01: `internal/defaults` (go:embed seed: sanitized zcode profile + openspec.toml + scheduling.yaml) + `internal/firstrun` (Ensure → non-clobbering first-run) + `acp serve` zero-config profiles-dir wiring + zeroconfig_test.go TRACER proof (build binary, seed, ACP handshake, idempotent). 06-02: `internal/version` (--version flag, ldflags-injected) + `.goreleaser.yml` (4 targets darwin/linux × amd64/arm64, CGO_ENABLED=0, `goreleaser check` exits 0) + canonical `agent.json` (REAL ACP schema: cmd/args, NOT command/command_args/cwd — Tier-A correction) + `scripts/gen-agent-json.sh` + `docs/install.md`. Tier-A corrections: ldflags target MUST be capital-D `github.com/Djarvur/...` (lowercase silently fails); archives use v2.17 `formats`/`files` (plan's `format`/`extra_files` deprecated). Snapshot build proof: linux_amd64 statically linked, `--version` prints 0.0.1-dev, zero-config first-run works inside the goreleaser binary. Phase gate GREEN: `mise ci` exits 0 (vet + golangci-lint v2 all-linters 0 issues + CGO build + -race, 24 packages). Note: catalog drift — tools.json carries 103 tools (PROF-04 drift, STATE.md blocker), not the plans' stale 77; seed mirrors source byte-for-byte. |

## Decisions Log

- **Phase 7 added (2026-08-13):** New phase "Multi-Provider Config & Credentials" added to v1.0 (`07-multi-provider-config-credentials`). Rationale: live testing (this session) confirmed the mimicry path works end-to-end with a real `ZAI_API_KEY`, but surfaced that v1's multi-provider promise is incomplete — `internal/scheduler/defaults/scheduling.yaml` declares a `providers`/`models`/`tiers` schema but (a) no `api_key` field, (b) only 1 provider (Z.ai anthropic) + 1 model (glm-5.2) wired, (c) provider keys read env-only (`$ZAI_API_KEY`) so editor-spawned processes (Zed → `ass-guard acp serve`) can't authenticate. PROV-01 ("any compatible provider via configurable base URL") + SCHED-02 ("heavy=glm-5.2, minimax-m3 in peak") are v1 REQ-IDs that presume multi-provider; Phase 7 closes that gap. Depends on Phase 3 (scheduler) + Phase 6 (`.ass-guard/` + embed + redactor). REQ-IDs PCFG-01.. assigned in discuss. Milestone status flipped completed→in_progress (outstanding phase). Next: `/gsd-discuss-phase 7`.
- **Phase 6 (2026-08-13):** Phase 6 COMPLETE (2/2 plans, autonomous execution). All 3 REQ-IDs (DIST-01/02/03) implemented + tested. 06-01 (embed + first-run, D-01/D-04): `internal/defaults` embeds the seed tree via go:embed (sanitized zcode profile + openspec.toml + scheduling.yaml) — `sync.sh` is the T-06-01 (HIGH) build-operator leak barrier (`/Users/nil/...` → `<working_directory>` / tilde-relative; coverage.yaml excluded). `internal/firstrun.Ensure` is the non-clobbering first-run (D-07 self-gitignore, byte-identical to internal/session). `acp serve` RunE seeds `.ass-guard/` + resolves profilesDir to `.ass-guard/profiles` (zero-config). 06-02 (goreleaser + agent.json, D-02/D-03): `internal/version` + `--version` flag; `.goreleaser.yml` 4 targets CGO_ENABLED=0; canonical `agent.json` (cmd/args, NOT command/command_args/cwd). Three Tier-A corrections: (1) ldflags -X target MUST be capital-D `github.com/Djarvur/...` (lowercase silently fails — verified); (2) D-03 field names corrected to canonical ACP schema (agent.schema.json fetched 2026-08-09); (3) goreleaser v2.17 `formats`/`files` (plan's `format`/`extra_files` deprecated). Two `-trimpath` bugs found + fixed at the gate (mise sets GOFLAGS=-trimpath): the drift guard + the E2E binary build used `runtime.Caller` (returns module-relative paths under -trimpath) — replaced with embed-to-embed comparison (`scheduler.EmbeddedDefaultScheduling()` accessor) and a go build by import path respectively. Catalog-drift deviation: tools.json carries 103 tools (PROF-04, STATE.md blocker), not the plans' stale 77 — seed mirrors source byte-for-byte. Phase gate GREEN: `mise ci` exits 0 (vet + golangci-lint v2 all-linters 0 issues + CGO_ENABLED=0 build + go test -race, 24 packages). Snapshot build proof: linux_amd64 statically linked, version injection confirmed, zero-config first-run works in the goreleaser binary (embed survived cross-compile). v1.0 milestone is SHIP-READY.
- **Phase 4 (2026-08-12):** Phase 4 COMPLETE (7/7 plans, 4 waves, autonomous execution). All 19 REQ-IDs (ENG-01..05, HOOK-01..05, LRN-01..04, OPEN-01..03, TOOL-04/05) implemented + tested under `-race`. Wave structure: W1 = 04-01 (engine decision core + pure Decide + Observe wrapper with graceful degradation + re-fire budget + cancel-drain) + 04-03 (hookdag: declarative YAML config + ≤300 LOC executor + provenance loop prevention + 4 step kinds) + 04-04 (toolexec: DispatchBatch concurrent-reads/serialized-mutations + swappable Backend + RealExecutor + toolcat.Register + session.SetToolExecutor loop replacement); W2 = 04-02 (openspec: TOML config + subprocess Adapter + RegisterTools + OpenSpecPatternTable bridge, real-openspec gated behind ASSGUARD_OPENSPEC_BIN=1) + 04-06 (learning: single-writer yaml Store + ProposeHooks + `ass-guard learning list/revert` CLI); W3 = 04-05 (integration: acp_serve engine wiring + ActionDispatcher hook→hookdag/ask→store + engineTurnRunnerAdapter + end-to-end zero-continue); W4 = 04-07 (cancel-drain ACP-level proof + consolidated 5-criteria e2e + 04-VERIFICATION-PREP.md). Key invariants honored: D-01 post-turn observer (never in the turn critical path), D-03 structural safety (unmatched ⇒ nothing — proven by testing/quick + end-to-end), D-04 graceful degradation (engine panic recovered, original stop returned), D-07 hand-rolled DAG executor (executor.go = 187 LOC, ≤300 budget), D-14 TOML isolated to internal/openspec (yaml.v3 for hookdag + learning), D-22 swappable backends (no firecrawl dep — stub proves the seam). One deviation recorded inline: case-insensitive import-path casing (`Djarvur` per go.mod) — followed the canonical module path. Phase gate GREEN (build + vet clean; internal/engine, hookdag, openspec, learning, toolexec, toolcat, event, session, cmd/ass-guard all `-race` green; 0 TODO/FIXME in Phase-4 production files). The single `go test ./... -race` failure is the PRE-EXISTING Phase-1 data-source blocker (internal/profile TestStability_WithinSessionExtractionSource — globs for the absent zcode rollout session eea3dc48, documented above); no Phase-4 file was touched in internal/profile. Commits: 7943c62 (W1), 994542f (W2), edaeaf4 (W3), 5b5331c (W4).
- **Plan 00-05 (2026-08-09):** D-07 Tier-B resolution for item #1 (zcode JSONL path) — research-predicted default **option-a (revise-and-continue) APPLIED BY DEFAULT** after the user declined to override the checkpoint across multiple prompts (NOT 'user chose option-a' — honest audit-trail attribution). Confirmed by on-disk evidence from plans 00-01..00-04 and documented as the prediction in 00-05-PLAN.md Task 2. Consequence (option-a, verbatim): Phase 1 MIMC-02 path wording to be corrected to `~/.zcode/cli/rollout/model-io-sess_<id>.jsonl` during Phase 1 planning; munged-cwd convention obsolete for zcode. Reversible at Phase 1 planning. Recorded in VERIFIED-FACTS.md item #1 Notes.
- **Plan 00-05 (2026-08-09):** Phase 0 COMPLETE (5/5 plans, 5/5 STACK items closed). VERIFIED-FACTS.md is the post-spike source of truth (STACK.md unchanged, D-01 — md5 `5e4eecc8418f9ff61272702044da0f54` preserved). The completeness gate `check-verified-facts.sh` exits 0 (6/6 checks); the plan `<verify>` chain prints PHASE0_GATE_PASS. D-03 sanitization is the phase's final secret-leak barrier (threat T-00-10, high) — zero sk-/Bearer/*_API_KEY=/raw UUID//Users/ matches across the merged file. Item #2 carries forward as PARTIAL: schema VERIFIED offline, live round-trip deferred pending operator key provisioning (single carry-forward operator action). Phase 1 (Mimicry MVP) is ready to plan.
- **Plan 00-01 (2026-08-09):** STACK item #1 (zcode JSONL path) is Tier-B-favorable → D-07 (a) revise-and-continue, confirmed on disk. Corrected path: `~/.zcode/cli/rollout/model-io-sess_<session-id>.jsonl` (STACK's `~/.claude/projects/<munged-cwd>/...` is a different product, Claude Code, `queue-operation` schema). Schema richer than STACK claimed (full wire-level capture; no MITM needed for request body). Explicit user sign-off on MIMC-02 wording correction lands in Plan 00-05.
- **Plan 00-01 (2026-08-09):** STACK item #4 (whisper.cpp cross-compile) closed STRUCTURALLY-MOOT per D-06 — STT always external (HTTPS or out-of-process `whisper-cli` subprocess), zero cgo, goreleaser matrix unaffected. No spike produced.
- **Plan 00-04 (2026-08-09):** STACK item #5 (go-telegram/bot + ACP stdout collision) VERIFIED — go-telegram/bot@v1.23.0 is silent-by-default (zero os.Stdout writes in package source; all output via 3 log.Printf→os.Stderr handlers; debug path gated behind WithDebug; no WithLogger/io.Writer option), confirmed by direct source inspection + the integration spike's captured-os.Stdout byte-equality assertion (415==415 bytes, 0 extra) + deterministic handler-routing probe. Transport discipline (stdout = ACP only) holds for the multi-frontend case. Tier-A refinement: actual callback signatures (ErrorsHandler func(err error), DebugHandler func(format, args...any)) are simpler than RESEARCH.md §5 documented — Phase 5 must use the real signatures. Mitigation recipe for v2 Telegram frontend recorded (WithErrorsHandler→slog→stderr, NO WithDebug, log.SetOutput(os.Stderr), go b.Start(ctx) goroutine).
- **Plan 00-04 (2026-08-09):** context-first shutdown contract confirmed for go-telegram/bot@v1.23.0 — bot goroutine exits within 2s of telegramCtx cancel (STACK's load-bearing reason for picking it over gotgbot/telebot). Wave 2 complete (4/4 spike plans); Plan 00-05 (Wave 3) can now fold all four evidence files into VERIFIED-FACTS.md.

## Blockers

()

---

*State initialized: 2026-08-09 after roadmap creation*

== What was being executed ==
Plan 01-01 T3: extract the zcode profile (3 system blocks, 77 tools, 12 identity headers, thinking, tool_choice) from the pinned rollout session `eea3dc48-9c8b-4162-a466-cee58181b741` into profiles/zcode/, and build internal/profile loader+types. Acceptance criterion: `jq 'length' profiles/zcode/tools.json` == 77.

== Root cause (investigate-and-fix-ready) ==
(1) The pinned model-io capture file does NOT exist:
    `~/.zcode/cli/rollout/model-io-sess_eea3dc48-9c8b-4162-a466-cee58181b741.jsonl` is ABSENT.
    `find ~/.zcode -name "*eea3dc48*"` returns only:

      - ~/.zcode/cli/artifacts/sess_eea3dc48.../  (tool-result JSON fragments, NOT wire-level requests)
      - ~/.zcode/cli/exec/sess_eea3dc48.../       (EMPTY directory)
      - ~/.zcode/cli/exec/bash-startup/sess_eea3dc48.../  (one shell script)
    of these carry request.body.{system,tools,thinking,tool_choice} or request.headers.
    So `eea3dc48` is UNRECOVERABLE as a model-io extraction source.

(2) The ONLY main session now in rollout/ is `016eee8a-f2f0-44c3-abf9-9d57a2cf04a1.jsonl` (120 lines, 32MB). Its schema matches VERIFIED-FACTS.md item #1 (model_io lines, the exact 12 identity header names, GLM-5.2 / builtin:zai-coding-plan) — BUT:

    - It carries 103 tool declarations per full line, NOT 77 (catalog drifted +26 tools since the plans were authored ~Aug 10 14:31; this is the PROF-04 drift scenario materializing during capture).
    - It has only 1 response with a non-empty toolCalls array (a single `AskUserQuestion` call). It is therefore NOT a viable parity-reference session: it cannot supply the 5-15 divergence-prone tool-calling turns Plan 01-06 T3 requires.
    - `thinking` = {type:"enabled", budget_tokens:32000} and `tool_choice` = {type:"auto"} are present on full lines (good — those fields extract fine).

(3) RESEARCH-FLAG-01's held-out split (Plan 01-02/01-06 design: `eea3dc48` = parity-reference, `016eee8a` = surprise-check) is BROKEN: the parity-reference session is gone and the surprise-check session has no tool-call turns.

== Downstream blast radius (all UNBLOCKED-on-replan only) ==

- Plan 01-01 T3 (profile extract): BLOCKED now — 77-tool criterion fails (103 on disk) and pinned session gone.
- Plan 01-01 T7 (tracer live round-trip): the ZAI_API_KEY gate the runtime predicted — still applies, but moot until T3 resolves (no profile to load).
- Plan 01-02 (formal extractor): verify command hardcodes `-sessions eea3dc48-...,016eee8a-...` and `jq 'length' == 77`; the cross-session stability test asserts eea3dc48 + 016eee8a both have 3 system blocks + identical built-in tool-name sets — eea3dc48 absent makes this unrunnable.
- Plan 01-05 (drift detector PROF-04): the tiered coverage.yaml baseline must be re-grounded on the 103-tool catalog; the drift thresholds likely need recalibration.
- Plan 01-06 (the parity GATE — project make-or-break): T2 replay reader reads `model-io-sess_eea3dc48...jsonl` ("69 lines, 88 tool-calls" per the plan — GONE); T3 curates 5-15 divergence turns from eea3dc48 (no tool-call turns available in either session). T6 is autonomous:false (ZAI_API_KEY) AND now also data-blocked.

== What CAN run autonomously (already done; preserved) ==

- 01-01 T1 (module init): DONE, committed (adaf8af, 7544ad8 chain). go.mod `module github.com/djarvur/ass-guard-agent` + `go 1.25`; deps pinned (anthropic-sdk-go@v1.62.0, go-openai@v1.42.0, cobra, viper, testify, yaml.v3); package skeleton; CGO_ENABLED=0 build clean; no go-telegram/go-sdk. Unaffected by this blocker.
- 01-01 T2 (internal/redact): DONE RED+GREEN, committed (test commit + feat commit). Redact/IsSecretKey/ScrubError; all tests pass -race; the 12 identity header NAMES preserved (fingerprint). Unaffected by this blocker — it has no profile dependency.

== Decisions the operator/planner must make (blocking) ==
A. DATA SOURCE: Do we (i) re-capture a fresh zcode session that produces divergence-prone tool-call turns (the operator runs a short coding task in zcode now to generate a new rollout with 5-15+ tool-calling turns), OR (ii) accept 016eee8a as the sole source and re-scope the parity gate to single-session (drops the held-out surprise-check), OR (iii) something else?
B. CATALOG DRIFT: Accept 103 tools as the new ground truth and update all `== 77` criteria to `== 103` (or to a range / "built-in core set" per RESEARCH §1.2 — the core ~20 stable, MCP/plugin tail variable)? This is itself PROF-04 drift evidence and may belong in the profile's meta.
C. RESEARCH-FLAG-01 HELD-OUT SPLIT: With eea3dc48 gone, is the split still viable, or does the parity design collapse to single-session + synthetic-perturbation? This changes Plan 01-06's structure.

== Why I stopped instead of auto-fixing ==
Per the critical rules + PROJECT.md investigate-and-fix-ready principle: silently extracting 103 tools and fudging the `== 77` criterion would violate Plan 01-01 T3 acceptance, corrupt Plan 01-06's parity reference (it would replay against a different session than it was designed for), and mask a real PROF-04 drift event. The ZAI_API_KEY gate (01-01 T7, 01-06 T6) was predicted; this data-source gate was not, and it is the operator's call how to re-ground the mimicry source of truth.

== Diagnostic evidence (re-runnable) ==

- `find ~/.zcode -name "*eea3dc48*"` → no model-io file (proves (1))
- `ls ~/.zcode/cli/rollout/` → only 016eee8a + 2 subagent files (proves session set changed)
- `jq 'select((.request.body.system|length)==3 and (.request.body.tools|length)>0) | (.request.body.tools|length)' 016eee8a...jsonl | sort -n | uniq -c` → 118 lines of 103, 0 lines of 77 (proves (2) catalog drift)
- `jq 'select((.response.toolCalls//[])|length>0)' 016eee8a...jsonl | wc -l` → 1 (proves (2) no parity turns)
- Schema of 016eee8a confirmed identical to VERIFIED-FACTS #1 (12 header names, GLM-5.2, builtin:zai-coding-plan) — so the extractor design is sound; only the input session inventory changed.
- Phase-1 live gates (01-01 T7 tracer round-trip; 01-06 T6 the parity gate) need ZAI_API_KEY, which is UNSET here. All autonomous substrate is built + race-clean (11 packages, plans 01-02..01-05 complete; 01-01/01-06 autonomous parts done). To unblock: export ZAI_API_KEY and run 01-01 T7 ('go run ./cmd/ass-guard --profile zcode --prompt "<tool-triggering prompt>"') then 01-06 T6 ('go run ./cmd/ass-guard parity --results /tmp/parity-results.json'). FAIL on 01-06 T6 = stop-and-replan (PROJECT.md Anti-Pattern 5).

## Performance Metrics

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 00 P01 | 9min | 3 tasks | 5 files |
| Phase 00 P02 | 6min | 2 tasks | 3 files |
| Phase 00 P03 | 8min | 2 tasks | 3 files |
| Phase 00 P04 | 10min | 2 tasks | 3 files |
| Phase 00 P05 | 8min | 1 task | 4 files |
| Phase 04 P01 (engine) | — | 4 tasks | 7 files |
| Phase 04 P02 (openspec) | — | 3 tasks | 9 files |
| Phase 04 P03 (hookdag) | — | 3 tasks | 8 files |
| Phase 04 P04 (toolexec) | — | 3 tasks | 8 files |
| Phase 04 P05 (integration) | — | 3 tasks | 5 files |
| Phase 04 P06 (learning) | — | 4 tasks | 9 files |
| Phase 04 P07 (cancel+verify) | — | 3 tasks | 4 files |

Phase-4 totals: 7 plans, 23 tasks, 5 new packages (engine/hookdag/openspec/learning/toolexec), 2 expanded packages (toolcat/session), 1 CLI subcommand (`ass-guard learning`), 1 wiring point (cmd/ass-guard/acp_serve.go). All 9 Phase-4 packages green under `-race`.

## Decisions

- [Phase ?]: Plan 00-02 (2026-08-09): STACK item #2 (go-openai tool-calling schema) — schema fidelity VERIFIED offline (Chat Completions tools[].function wrapper, string-typed arguments, tool-role result message, distinct from newer Responses API). Live round-trip recorded PARTIAL: MINIMAX_API_KEY/GROQ_API_KEY both unset at exec (Tier-A fallback, D-07). VERIFIED requires operator to export a key + re-run spikes/02-openai-toolschema. Plan 00-05 folds RESULT.md into VERIFIED-FACTS.md item #2.
- [Phase ?]: Plan 00-02 (2026-08-09): go-openai@v1.42.0 targets Chat Completions (tools[].function, tool_calls, role:tool) — NOT the newer Responses API shape (output[].function_call). Phase 1's OpenAI-shape provider adapter conforms to the Chat Completions schema; must not drift to Responses API. pkg.go.dev cadence lag confirmed real but not stagnation (GitHub active: v1.40→v1.42 over May-Aug 2026).
- [Phase ?]: Plan 00-03 (2026-08-09): STACK item #3 (ACP v1 method names + wire shape) — VERIFIED against the canonical spec (agentclientprotocol.com/protocol/v1/, fetched 2026-08-09). Framing is newline-delimited JSON-RPC (NOT LSP-style Content-Length headers; no embedded newlines; stdout = ACP only). Lifecycle is initialize → session/new → session/prompt (+ observe session/update) — Tier-A correction: STACK omitted the mandatory session/new step (you cannot send session/prompt without a sessionId from session/new). Result field is agentCapabilities (NOT capabilities/serverInfo). protocolVersion is integer 1 (not string). No discrepancy vs STACK on framing; the only correction is the omitted session/new step.
- [Phase ?]: Plan 00-03 (2026-08-09): ACP v1 wire shape (hand-rolled newline-delimited JSON-RPC, spec-pinned envelope builders) is copy-paste-safe starting point for Phase 2's ACP adapter. The in-process mock peer proved the wire shape deterministically without an external ACP server or model call (D-04 throwaway; critical_constraint #3/#7).
- [Phase ?]: Plan 00-04 (2026-08-09): STACK item #5 (go-telegram/bot + ACP stdout collision) — VERIFIED via the integration spike at spikes/05-stdout-collision/. go-telegram/bot@v1.23.0 is silent-by-default (zero os.Stdout writes in package source — only the examples/ samples write to stdout, not the library; all library output via 3 default handlers in bot.go all log.Printf→os.Stderr; debug path gated behind WithDebug; no WithLogger/io.Writer option). Captured real os.Stdout (os.Pipe) byte-equal to 3 canned ACP frames (415==415, 0 extra) while the bot goroutine ran concurrently; deterministic handler-routing probe (synthetic marker in stderr sink, absent from stdout). Transport discipline (stdout = ACP only) holds for the multi-frontend case.
- [Phase ?]: Plan 00-04 (2026-08-09): Tier-A refinement vs RESEARCH.md §5 — go-telegram/bot@v1.23.0 callback signatures are ErrorsHandler func(err error) and DebugHandler func(format string, args ...any) (SIMPLER than RESEARCH.md documented; no context.Context params). The load-bearing "callbacks, no stdout, no io.Writer" claim is unchanged. Phase 5's v2 Telegram frontend must use the real signatures + the 4-step mitigation recipe (WithErrorsHandler→slog→stderr, NO WithDebug, log.SetOutput(os.Stderr) belt-and-suspenders, go b.Start(ctx) goroutine; cancel telegramCtx on editor-initiated shutdown to drain the loop).
- [Phase ?]: Phase 1 re-dispatch: autonomous substrate complete (01-02..01-05 + 01-01/01-06 T1-T5); D-16 session assignment documented; 2 live gates blocked on ZAI_API_KEY
