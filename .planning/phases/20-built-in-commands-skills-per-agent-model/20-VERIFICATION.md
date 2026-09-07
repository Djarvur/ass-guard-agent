---
status: passed
verified_at: 2026-09-08T01:10:00Z
verifier: inline-execute-phase (agent-runtime unavailable; verification executed directly)
phase: 20-built-in-commands-skills-per-agent-model
---

# Phase 20 Verification

Goal under test: *"Slash-invocation becomes one coherent resolver chain — builtins → skills → agents → file-discovered commands — with CC's useful internal commands executing control-plane-fast (no model turn) or prompt-expanding as appropriate, skills addressable as slash commands, AGENTS addressable likewise, and per-agent `model:` frontmatter actually routing subagent dispatch. Everything autocompletes in the editor via available_commands_update."*

## Method

Every claim below names an executed command. The full `go test -race -count=1 ./...` ran twice (before and after the review fix) — all 38 packages green both times.

## Must-have verification (per plan)

### 20-01 (chain skeleton + advertisement)
- Typing /status returns the live snapshot with ZERO provider calls, end_turn: `go test ./internal/runtime/ -run TestTracerStatusClassB` — PASS (Stream count asserted 0).
- Echo as user_message_chunk with DISTINCT messageId: same test + `go test ./internal/acp/ -run TestUserMessageChunkEcho` — PASS.
- Thirteen reserved names pre-seated; shadowing blocked + one warning: `go test ./internal/runtime/ -run 'TestCommandChainReservedShadowing|TestReservedNamesFitInvocationGrammar'` — PASS.
- Chain collision order silent/deterministic: `TestCommandChainSemantics` — PASS.
- available_commands_update full sorted winner set: `TestAvailableCommandsUpdateGolden` + `TestAvailableCommandsFullReplacement` + `TestCommandsNotifySeam` — PASS.
- local_command 16-D-22 shape via the REDACTED path: `TestTracerStatusClassB` (Name/Args/SourceChain asserted) — PASS.

### 20-02 (class-B family)
- All twelve commands, zero Stream calls each + local_command lines: `go test ./internal/runtime/ -run TestClassB` — PASS.
- /help from the live chain (self-describing): `TestClassBHelpSelfDescribing` — PASS.
- /clear same-session reset (D-06): `TestClassBClear` — boundary line + empty post-clear projection + session id unchanged — PASS.
- /cost three-case matrix + source note (P-20-02) + no credential leak: `TestClassBCost` — PASS.
- /model session-scope, no layer writes: `TestClassBModel` — PASS.
- /compact + /resume delegation: seams ARMED with the real Phase 18/19 machinery (both executed); degrade posture pinned by nil-seam tests: `TestClassBDelegate` — PASS.

### 20-03 (per-agent model)
- All precedence arms incl. inherit→parent and unset→PARENT (14-05 reversed): `go test ./internal/runtime/ -run TestDispatchModel` — PASS.
- Cross-provider ROUTE + per-(provider,session) cache; unknown/uncredentialed one-warning degrades; note dedupe: `TestDispatchModel_Precedence` — PASS.
- resolvedModel both ways (durable field + deduped live note; D-20 tolerance): `go test ./internal/session/ -run TestSubagentDispatch_ResolvedModel` — PASS.
- Live-chain agent lookup (post-construction dispatch): `TestDispatchModel_LiveAgentLookup` — PASS.

### 20-04 (skills/agents/init)
- /<skill-name> locked Expand semantics + provenance; empty-body reject; user-invocable:false excluded from slash AND advertisement: `go test ./internal/runtime/ -run TestSkillSlash` — PASS.
- /<agent-name> dispatch (args→prompt, Tools, resolvedModel, streaming, zero parent turns; D-02 collision): `TestAgentSlash` — PASS.
- /init through the seam, provenance builtin:init, engine-on parity, D-01 shadow: `TestInitExpansion` — PASS.

### 20-05 (live discovery)
- Watched create/remove lands in the chain without restart; debounce single-fire; D-12 degrade; freshness backstop; -race concurrency + mid-session agent; shutdown: `go test ./internal/runtime/ -run 'TestRescan|TestFreshness'` — PASS.
- Wire-level re-fire over the composition: `TestSimulatorCommandSurface` scenario 3 — PASS.

### 20-06 (E2E + gate)
- Four wire scenarios: `go test ./internal/acpserve/ -run TestSimulatorCommandSurface` — PASS.
- Gate: vet ✓, CGO_ENABLED=0 build ✓, `go test -race ./...` ✓ (38/38). Lint: pre-existing tooling drift (golangci 2.13.2 renames vs .golangci.yml exclusion names) — verified at baseline before any Phase 20 commit; every phase-touched file passes the still-matching linters; recorded in STATE.md with the recommended config fix.

## Requirement traceability

| ID | Plans claiming | Evidence above | Accounted |
|----|----------------|----------------|-----------|
| ACP-04 | 20-01, 20-05, 20-06 | goldens + re-fire + E2E scenarios 1/3 | ✓ |
| CMDS-01 | 20-01, 20-06 | chain battery | ✓ |
| CMDS-02 | 20-02, 20-06 | twelve-command battery; /compact real machinery registered (PAR-01 landed in Phase 19 — the "requires PAR-01" note is satisfied by execution order) | ✓ |
| CMDS-03 | 20-04, 20-06 | TestInitExpansion | ✓ |
| CMDS-04 | 20-05, 20-06 | rescan battery + E2E scenario 3 | ✓ |
| SKLS-01 | 20-04, 20-06 | TestSkillSlash | ✓ |
| SKLS-02 | 20-04, 20-06 | TestAgentSlash + E2E scenario 4 | ✓ |
| SKLS-03 | 20-03, 20-06 | TestDispatchModel + dispatch-line tests | ✓ |

## Human verification (pending, tracked)

Seven live-Zed UX items persisted in `20-HUMAN-UAT.md` (autocomplete completeness, instant /status, mid-session pickup, dispatch note, /cost source note, /compact output, reserved-name shadowing). Automated wire-level equivalents all pass; the operator confirms UX at the next session (Phase 15/16 WINDOWS precedent).

## Prohibition + probe status

- P-20-01/P-20-02 enforced by the batteries named above (review verdict: clean, one Warning fixed in-phase — CR-01).
- The nine PROBE-UNRESOLVED rows remain flagged per the fallback protocol (never auto-resolved).

## Verdict

**passed** — the phase goal holds against the codebase; the six ROADMAP criteria map to executed commands (matrix in 20-06-SUMMARY.md); the operator UX confirmation is tracked as pending UAT.
