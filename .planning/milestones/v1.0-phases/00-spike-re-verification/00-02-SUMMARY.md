---
phase: 00-spike-re-verification
plan: 02
subsystem: infra
tags: [go, go-openai, openai, tool-calling, chat-completions, function-calling, minimax, groq, spikes, throwaway, redaction, sanitization]

# Dependency graph
requires:
  - phase: 00-spike-re-verification
    provides: "spikes module skeleton (github.com/djarvur/ass-guard-spikes, go 1.25, go-openai@v1.42.0 + go-telegram/bot@v1.23.0 pins) — Plan 00-01's output this spike `go run`s against"
provides:
  - "spikes/02-openai-toolschema/main.go — throwaway Go spike proving sashabaranov/go-openai@v1.42.0 serializes + round-trips the OpenAI Chat Completions tool-call schema (NOT the Responses API)"
  - "spikes/02-openai-toolschema/main_test.go — RED-then-GREEN offline test suite asserting the Chat Completions schema fidelity (tools[].function wrapper, string-typed arguments, tool-role result message)"
  - "spikes/02-openai-toolschema/RESULT.md — D-02-shaped item-#2 draft (Status: PARTIAL) for Plan 00-05 to fold into VERIFIED-FACTS.md; schema verified offline, live round-trip deferred pending operator key provisioning"
affects: [00-05, phase-1-mimicry-mvp]

# Actuals (#2632) — pairs with the plan's estimate (38000 tokens, 2 tasks, low confidence)
actuals:
  tokens: 9638    # chars/4 over the realized diff (38555 chars across 3 files)
  tasks: 2
  commits: 3       # RED test + GREEN impl + RESULT.md

# Tech tracking
tech-stack:
  added: []  # go-openai@v1.42.0 + go-telegram/bot@v1.23.0 were pinned by Plan 00-01; this spike only consumes them
  patterns: ["Chat Completions tool-call schema (tools[].function wrapper, string-typed arguments, tool-role result message) — distinct from the newer Responses API shape", "D-03 JSON-walk redaction preserving header NAMES + JSON keys + enum values while replacing secret-carrier values", "transport discipline enforced in a spike: stdout byte-empty (T-00-05 mitigated), all diagnostics to stderr", "capturingTransport (http.RoundTripper wrapper) to tee provider response bodies for redacted printing without breaking library decode"]

key-files:
  created:
    - spikes/02-openai-toolschema/main.go
    - spikes/02-openai-toolschema/main_test.go
    - spikes/02-openai-toolschema/RESULT.md
  modified: []

key-decisions:
  - "Offline schema-fidelity assertions run UNCONDITIONALLY and count toward the footer regardless of provider-key availability — the schema shape (the load-bearing #2 question) is verifiable by serializing go-openai's own Tool/FunctionDefinition/jsonschema.Definition types, no network needed. Only the live provider round-trip + quirk observations require a key. This means PARTIAL-with-no-key still records the schema finding."
  - "Status PARTIAL (not VERIFIED, not FAILED) is the correct Tier-A outcome when MINIMAX_API_KEY/GROQ_API_KEY/OPENAI_API_KEY are all unset. Per D-07 a missing key is a doc/minor condition — record + continue, no halt-and-replan. The three Tier-B options are NOT triggered because the failure is an operator-key gap, not a schema divergence go-openai cannot represent (the library serializes the correct Chat Completions shape, proven offline)."
  - "Three-provider target design honored faithfully: MiniMax (M3 slug + base URL), Groq (chat slug + api.groq.com/openai/v1), generic OPENAI_* triple. Each provider's key is probed once via os.Getenv; unset → SKIP that provider (not fatal). If NO key at all → exit non-zero with a clear stderr message + PARTIAL footer. This is exactly the design in critical_constraints #2/#3."
  - "Captured raw request via json.Marshal(req) directly (this is exactly what the library serializes before sending) — lets the spike assert + print the schema shape offline. Captured raw response via a custom http.RoundTripper (teeTransport) that reads + restores the body so the library decodes it normally. Both bodies redacted before any print."

patterns-established:
  - "TDD RED→GREEN for a throwaway spike: the offline-verifiable slice (schema fidelity) is the testable behavior; the live network round-trip is asserted by the binary itself via a structured stderr footer. RED = undefined helpers (compile fail); GREEN = main.go provides the helpers + full spike."
  - "D-03 redaction as a commit gate for any artifact carrying provider traffic: walkRequest JSON tree replacing Authorization/api_key/key/token VALUES with [REDACTED] while preserving the field NAMES + all JSON keys + enum values (the schema IS the verification target). Verified clean against a synthetic sk-... key run."
  - "Transport discipline asserted in the spike itself: the plan's <verify> one-liner pipes stdout to a file and asserts it is byte-empty (`[ ! -s ... ]`) — the spike models the project's stdout=ACP-only rule from day one (T-00-05 mitigated)."

requirements-completed: [STACK-Phase0-#2]

# Coverage metadata (#1602) — the deliverable is the #2 finding, which is auto-proven
# for schema fidelity (passing offline test suite) but PARTIAL overall because the live
# provider round-trip requires an operator action (key provisioning). Routes to a human
# so the verifier confirms the PARTIAL→VERIFIED unblock path is real.
coverage:
  - id: D1
    description: "STACK item #2 finding in spikes/02-openai-toolschema/RESULT.md — go-openai@v1.42.0 Chat Completions tool-call schema fidelity recorded (D-02 seven fields, Status PARTIAL)"
    requirement: "STACK-Phase0-#2"
    verification:
      - kind: unit
        ref: "spikes/02-openai-toolschema/main_test.go#TestRequestToolSerializesToChatCompletionsShape"
        status: pass
      - kind: unit
        ref: "spikes/02-openai-toolschema/main_test.go#TestToolCallArgumentsIsStringType"
        status: pass
      - kind: unit
        ref: "spikes/02-openai-toolschema/main_test.go#TestToolResultMessageSerializesChatCompletionsShape"
        status: pass
      - kind: integration
        ref: "spikes/02-openai-toolschema/ — `go run ./02-openai-toolschema/` structured footer: offline.schema_request_shape/arguments_is_string_type/tool_result_message_shape all PASS; stdout byte-empty (transport discipline)"
        status: pass
    human_judgment: true
    rationale: "Schema fidelity is auto-proven (4 passing verifications above), but the overall item Status is PARTIAL — the live provider round-trip (MiniMax M3 / Groq) is deferred pending operator key provisioning (MINIMAX_API_KEY / GROQ_API_KEY both unset at execution). VERIFIED requires a human to provision a key and re-run; the verifier must confirm the PARTIAL→VERIFIED unblock path is the intended operator action, not a silent gap."

# Metrics
duration: 6min
completed: 2026-08-09
status: complete
---

# Phase 0 Plan 02: #2 go-openai Tool-Calling Schema Spike Summary

**Throwaway Go spike proves sashabaranov/go-openai@v1.42.0 serializes the OpenAI Chat Completions tool-call schema (tools[].function wrapper, string-typed arguments, tool-role result message — distinct from the newer Responses API); schema verified offline via a RED→GREEN test suite, live provider round-trip deferred to PARTIAL pending operator key provisioning (MiniMax/Groq keys both unset).**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-08-09T17:43:47Z
- **Completed:** 2026-08-09T17:50:04Z
- **Tasks:** 2
- **Files modified:** 3 (all created; 0 modified)

## Accomplishments

- Built the throwaway spike `spikes/02-openai-toolschema/main.go` (package main, module `github.com/djarvur/ass-guard-spikes`) that performs a real OpenAI Chat Completions tool-call round-trip via `sashabaranov/go-openai@v1.42.0` against three env-driven provider targets — MiniMax M3 (`MINIMAX_API_KEY`), Groq (`GROQ_API_KEY` + `api.groq.com/openai/v1`), and a generic `OPENAI_*` triple — skipping any provider whose key is unset (Tier-A fallback) and exiting non-zero with a clear stderr message when NO key is set.
- Verified the **Chat Completions schema fidelity offline** by serializing go-openai's own `Tool`/`FunctionDefinition`/`jsonschema.Definition` types and asserting the resulting JSON carries the `tools[].function` wrapper (`{"type":"function","function":{"name":...,"parameters":{...}}}`), the string-typed `arguments` field (`ToolCall.Function.Arguments` is `string`, parses into the declared `{location:string}` schema), and the `tool`-role result message (`role:"tool"` + `tool_call_id` + JSON-string `content`). This is distinct from the newer Responses API shape — confirmed against the v1.42.0 source (`chatCompletionsSuffix = "/chat/completions"`, `Tool.MarshalJSON` emits the `{"type","function"}` wrapper).
- Honored the security contract (T-00-03/T-00-04 mitigate, T-00-05 mitigate): a JSON-walking `redact()` replaces `Authorization`/`api_key`/`key`/`token` VALUES with `[REDACTED]` while preserving HTTP header NAMES + all JSON keys + enum values (the schema IS the verification target, per D-03 §7); `scrubError()` belt-and-suspenders on error messages; **stdout is byte-empty** (the plan's `<verify>` asserts `[ ! -s ... ]` — transport discipline modeled from day one). Verified clean against a synthetic `sk-...` key run.
- Wrote `spikes/02-openai-toolschema/RESULT.md` as the D-02 seven-field draft for Plan 00-05 to fold into VERIFIED-FACTS.md item #2: **Status: PARTIAL** — schema verified offline (3 PASS lines), live round-trip deferred pending operator key provisioning; Evidence embeds the redacted canonical request body + per-assertion footer + reference response/tool-result shapes; Notes record the Chat Completions-vs-Responses clarification, the pkg.go.dev cadence nuance, the three open provider-quirk questions, the Tier-A disposition, and the single operator action that unblocks VERIFIED.
- Did NOT create or modify `.planning/research/VERIFIED-FACTS.md` (Plan 00-05 is the single writer — zero `files_modified` overlap with sibling plans).

## Task Commits

Each task was committed atomically. Task 1 is `tdd="true"` under MVP+TDD (ROADMAP Phase 0 mode=mvp + config `tdd_mode: true`), so it follows the RED→GREEN gate sequence:

1. **Task 1 (RED): Write failing test for go-openai Chat Completions tool-schema shape** - `1ded07e` (test) — undefined `buildWeatherToolRequest`/`buildToolResultMessage` helpers → compile failure (RED bar)
2. **Task 1 (GREEN): go-openai tool-schema spike (Chat Completions round-trip)** - `31913bb` (feat) — `main.go` provides the helpers + full spike; `go test -race` passes; module `go vet` clean
3. **Task 2: Record STACK #2 finding in RESULT.md (D-02 shape, PARTIAL by design)** - `fbcb104` (feat)

**TDD Gate Compliance:** RED commit `1ded07e` (`test(00-02):`) precedes GREEN commit `31913bb` (`feat(00-02):`) for the same plan — the RED→GREEN gate sequence is satisfied. The MVP+TDD runtime gate (`task.is-behavior-adding` returned `true` for Task 1) did NOT trip because the RED commit was written before the implementation step, exactly as the gate requires.

## Files Created/Modified

- `spikes/02-openai-toolschema/main_test.go` - RED-then-GREEN offline test suite (4 tests): `TestRequestToolSerializesToChatCompletionsShape` (tools[].function wrapper), `TestToolCallArgumentsIsStringType` (string-typed arguments), `TestToolResultMessageSerializesChatCompletionsShape` (tool-role result message), `TestJsonSchemaDefinitionProducesLocationSchema` (guard that the schema helper produces real library output).
- `spikes/02-openai-toolschema/main.go` - Throwaway spike (package main): three-provider env-driven target resolution, `buildWeatherToolRequest`/`buildToolResultMessage` helpers, `teeTransport` (http.RoundTripper) for raw response capture, `runRoundTrip` asserting (a) request shape (b) tool_calls string arguments (c) follow-up references result, `redact()`/`scrubError()` D-03 sanitization, structured `=== SPIKE 02 RESULT ===` footer on stderr, stdout byte-empty.
- `spikes/02-openai-toolschema/RESULT.md` - D-02 seven-field item-#2 draft for Plan 00-05: Fact (STACK quote), Source, Verified (2026-08-09), Verified against (`go-openai@v1.42.0`), Status (PARTIAL), Evidence (redacted request + per-assertion footer + reference response/tool-result shapes), Notes (Chat Completions-vs-Responses, pkg.go.dev cadence, open provider quirks, Tier-A disposition, unblock action).

## Decisions Made

- **Offline schema-fidelity assertions always run.** The schema shape (the load-bearing #2 question) is verifiable by serializing go-openai's own types — no network needed. So PARTIAL-with-no-key still records the schema finding for Plan 00-05, and the spike remains useful evidence even when no provider key is provisioned. Only the live provider round-trip + the three provider-quirk observations require a key.
- **PARTIAL is the correct Tier-A outcome for a missing key (not FAILED, not VERIFIED).** Per D-07 a missing key is a doc/minor condition. NOT Tier B because the failure is not a schema divergence go-openai cannot represent (the library serializes the correct Chat Completions shape, proven offline). No halt-and-replan; the three Tier-B options are not triggered.
- **Raw request captured via direct `json.Marshal(req)`.** This is exactly what the library serializes before sending, so asserting it offline proves the schema without network. Raw response captured via a custom `http.RoundTripper` (`teeTransport`) that reads + restores the body — lets the library decode normally while the spike keeps a redacted copy.
- **RESULT.md `**Status: PARTIAL**` annotation placed directly under the `# #2.` heading** (mirroring Plan 00-01's FINDING.md pattern) so the plan's `RESULT2_OK` verify heuristic — which greps for VERIFIED|PARTIAL — passes regardless of the canonical D-02 field ordering. Both the annotation and the `- **Status:** \`PARTIAL\`` field agree; no information drift.

## Deviations from Plan

None - plan executed exactly as written. The TDD RED→GREEN gate sequence (required because Task 1 is `tdd="true"` under active MVP+TDD mode) produced an extra `test(00-02):` commit before the `feat(00-02):` implementation commit, but this is the *specified* TDD flow, not a deviation. All `<verify>` automations passed verbatim: Task 1 (`SPIKE02_STDERR_ONLY_OK`), Task 2 (`RESULT2_OK`).

## Issues Encountered

- **`go.sum` missing on first `go test`.** Plan 00-01 gitignored `go.sum` (D-05) and committed only `spikes/go.mod`. The first `go test` failed with "missing go.sum entry" — resolved by `go mod download` + `go mod tidy` to regenerate the gitignored `go.sum` locally. NOT a deviation: this is the designed local-regeneration path (D-05). One sub-issue caught and corrected: `go mod tidy` pruned the `go-telegram/bot` pin from `go.mod` (not yet imported by any spike file), which would have broken Plan 04 later and mismatched the README's documented module layout — restored `go.mod` to Plan 00-01's both-pin state via `git checkout -- spikes/go.mod`; verified the RED test still compiled. The restored `go.mod` was not committed (no net change vs. HEAD).

## User Setup Required

**Operator key provisioning is required to flip item #2 from PARTIAL → VERIFIED.** No external service *configuration*, just env vars. To complete the live round-trip Plan 00-05 will record as VERIFIED, an operator exports at least one of:

- `MINIMAX_API_KEY` — MiniMax platform console → API keys (drives the MiniMax M3 round-trip; the higher-divergence-risk provider)
- `GROQ_API_KEY` — Groq Console → API Keys (drives the Groq round-trip; predicted close OpenAI clone)
- `OPENAI_API_KEY` + `OPENAI_BASE_URL` + `OPENAI_MODEL` — any other OpenAI-shape endpoint

Then re-run: `cd spikes && go run ./02-openai-toolschema/`. The spike auto-detects whichever key(s) are set, round-trips each, and the structured footer flips to `overall_status: VERIFIED` once at least one provider passes all three live assertions. Until then the schema fidelity (the load-bearing #2 question) stands verified offline.

(No USER-SETUP.md generated — the plan's frontmatter `user_setup` already documents these env vars; this SUMMARY records the same for the verifier's benefit.)

## Threat Flags

The spike introduces security-relevant surface fully covered by the plan's `<threat_model>` (no NEW surface beyond it):

| Flag | File | Description |
|------|------|-------------|
| (covered) T-00-03 | spikes/02-openai-toolschema/main.go | API key handling — mitigated: key read from env only, never printed; `redact()` + `scrubError()` replace Authorization/Bearer/api_key values before any stderr write. Verified clean against a synthetic `sk-...` key run. |
| (covered) T-00-04 | spikes/02-openai-toolschema/RESULT.md | Committed evidence — mitigated: D-03 redaction applied to the embedded request/response; RESULT.md self-grepped for `Bearer <token>`/`sk-` patterns before commit (CLEAN). Plan 00-05 re-runs the grep over the merged VERIFIED-FACTS.md. |
| (covered) T-00-05 | spikes/02-openai-toolschema/main.go | stdout tampering — mitigated: spike writes NOTHING to stdout; plan `<verify>` asserts stdout byte-empty (`[ ! -s ... ]`); all diagnostics to stderr. |

No threat flags beyond the plan's register — all three registered threats (T-00-03/04/05) are mitigated and verified.

## Next Phase Readiness

- **Plan 00-02 (this plan) is complete.** The throwaway spike compiles, runs, and produces the structured footer; the D-02-shaped RESULT.md is written; the schema fidelity (the load-bearing #2 question) is verified offline.
- **Plans 00-03 / 00-04 (Wave 2, parallel)** can run independently — they own their own `spikes/0N-*/` subdirs and RESULT.md files (zero `files_modified` overlap with this plan). The spikes module (`spikes/go.mod`) is shared and untouched by this plan's commits.
- **Plan 00-05 (Wave 3)** reads this plan's `spikes/02-openai-toolschema/RESULT.md` + the three other evidence files (FINDING.md + RESULT.md from Plans 01/03/04) and authors VERIFIED-FACTS.md item #2 from it. It should record item #2 as "**schema VERIFIED offline; live round-trip PARTIAL — deferred pending operator key provisioning**" and surface the single unblock action (export `MINIMAX_API_KEY` or `GROQ_API_KEY` + re-run) for the operator.
- **Phase 1** consumes the verified schema (tools[].function wrapper, string-typed arguments, tool-role result message) as the contract its OpenAI-shape provider adapter conforms to. The Chat Completions-vs-Responses-API clarification in RESULT.md Notes prevents Phase 1 from accidentally targeting the newer Responses API shape.

## Self-Check: PASSED

- **Files created (4/4 FOUND):** `spikes/02-openai-toolschema/main.go`, `spikes/02-openai-toolschema/main_test.go`, `spikes/02-openai-toolschema/RESULT.md`, `.planning/phases/00-spike-re-verification/00-02-SUMMARY.md`
- **Commits (3/3 FOUND):** `1ded07e` (test, RED), `31913bb` (feat, GREEN), `fbcb104` (feat, RESULT.md)
- **VERIFIED-FACTS.md NOT created/modified** (constraint #1 honored — Plan 00-05 owns it; `ls` confirms file does not exist)
- **No file deletions** in any of the three commits
- **Task 1 `<verify>`:** `SPIKE02_STDERR_ONLY_OK` (vet + footer + empty stdout) — PASS
- **Task 2 `<verify>`:** `RESULT2_OK` (heading + Status + VERIFIED|PARTIAL + v1.42.0 + tool_calls + no sk-/Bearer) — PASS
- **D-03 self-grep over RESULT.md:** CLEAN (no Bearer token, no sk- key, no Authorization value)

---

*Phase: 00-spike-re-verification*
*Plan: 02*
*Completed: 2026-08-09*
