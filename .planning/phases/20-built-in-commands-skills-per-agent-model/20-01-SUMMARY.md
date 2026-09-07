---
phase: 20-built-in-commands-skills-per-agent-model
plan: 01
subsystem: commands
tags: [slash-commands, resolver-chain, acp-wire, available-commands-update, class-b]

requires:
  - phase: 16-acp-wire-foundation
    provides: TurnEmitter + EmitterHandle (foreground lane), AppendLocalCommand + local_command transcript kind (16-D-22), Notify template
  - phase: 18-session-family
    provides: CommandSource seam + NotifyAvailableCommands + commandSourceAdapter (18-05/ACP-06 load re-advertise)
  - phase: 15-runtime-carve
    provides: internal/runtime Runner seam (expandUserBlocks/invocationFor, Run ordering)
provides:
  - Immutable command chain (builtins → skills → agents → file, winners-only, D-01 reserved thirteen) behind atomic.Pointer on the Runner + rebuildCommandChain swap
  - Class-B intercept in Runner.Run (after routeAskReply, before the engine branch) with panic-recovery envelope
  - /status live snapshot builtin (zero provider calls) — the tracer
  - EmitterHandle.UserMessageChunk + updKindUserMessageChunk (D-05 echo frame; ActivityEmitter widened)
  - AvailableCommandInputFrame + Input field on AvailableCommandFrame (v1 unstructured hint variant)
  - Session-start available_commands_update (updates-before-response via Barrier) in handleSessionNew
  - Server.NotifyAllAvailableCommands fan-out + Runner.SetCommandsNotify re-fire seam wired in acpserve
  - Chain-backed commandSourceAdapter (advertisement == resolver's winners, D-04 one truth)
affects: [20-02 class-B family, 20-03 per-agent model, 20-04 skills/agents/init, 20-05 rescan, 20-06 E2E]

tech-stack:
  added: []
  patterns:
    - "Immutable-chain-behind-atomic-pointer (RESEARCH Pattern 1): lock-free read-hot path, wholesale swap on rebuild; chain is a VIEW over ecosys.Registry (never mutates it)"
    - "Class-B intercept placement: AFTER routeAskReply, BEFORE the engine branch (Anti-Pattern 3 fenced by construction)"
    - "Echo/output chunks ride the turn's in-hand emitter handle via the ActivityEmitter assertion (plain ChunkEmitter fakes silently skip the echo)"

key-files:
  created:
    - internal/runtime/commands.go
    - internal/runtime/commands_test.go
    - internal/acp/available_commands_test.go
    - internal/acpserve/commands_seam_test.go
  modified:
    - internal/runtime/runtime.go
    - internal/acp/types.go
    - internal/acp/emitter.go
    - internal/acp/server.go
    - internal/acp/handlers.go
    - internal/acpserve/acp_serve.go
    - internal/acpserve/command_source.go
    - internal/session/session.go
    - test harnesses (acp server_test/emitter_test/replay_test/handlers_test/cancel_session_test; runtime integration/emitter_e2e/ask_wiring/advisory_wiring/planmode_wiring) — readResultFrame/readResultFramesCounting skip notification-before-response

key-decisions:
  - "Reserved-set vs live-entry split: all THIRTEEN names block discovery (D-01 warning), but only status+init are chain entries in 20-01 — dormant reserved names fall through as plain text and never appear in the advertisement (D-04: what is shown is what runs)"
  - "ActivityEmitter widened with UserMessageChunk (one test fake updated) rather than a third narrow interface — user_message_chunk is v1 live-turn vocabulary, one emitter surface"
  - "Session-start advertisement fires inside handleSessionNew through the EXISTING NotifyAvailableCommands/CommandSource path with a Barrier (updates-before-response, the load-path contract generalized); the SetCommandsNotify seam carries swap-driven re-fires (20-05)"
  - "Session.MintLocalCommandTurnID exported — class-B turns mint from the same monotonic sequence (SeedResume collision-freedom)"
  - "Advertisement golden pins byte order Name,Description,Input (struct field order) with HTML-safe hint values (json.Marshal escapes < >)"

patterns-established:
  - "Chain resolution counter (chainResolves) as behavioral single-parse proof — one resolve per Run, asserted not grepped"
  - "readResultFrame/readResultFramesCounting test helpers: response readers that skip (and count) session/update notifications ahead of responses"

requirements-completed: [ACP-04, CMDS-01, CMDS-02]

duration: 65 min
completed: 2026-09-07T18:43:44Z
---

# Phase 20 Plan 01: Command Chain Skeleton + /status Tracer Summary

**One typed /status now resolves through an immutable builtins→skills→agents→file winners-only chain and answers control-plane-fast (echo + output chunks + end_turn, zero provider calls) while session start advertises the exact winner set over a schema-pinned available_commands_update frame.**

## Performance

- **Duration:** 65 min (2026-09-07T17:38Z → 18:43Z)
- **Tasks:** 3/3 (tracer + 2 TDD batteries)
- **Files:** 4 created, 15 modified (8 source + 7 test-harness)

## Accomplishments

- Task 1 (tracer, 89bcc6f): TestTracerStatusClassB drives Run("/status deep-check") end-to-end — fake provider Stream count == 0, exactly one user_message_chunk echo with a distinct messageId, output agent_message_chunks, end_turn, one 16-D-22 local_command line (Name/Args/SourceChain [builtin]), chainResolveCount == 1, advertisement carries the winners.
- Task 2 (6142301): chain semantics battery — three-way skill>agent>file, agent-vs-file, builtin-vs-discovered (D-01), unknown miss, empty registry; shadow warning exactly once per file per build; advertisement sorted/unique/hint-omission; registry view-immutability; thirteen-name invocation-grammar pin.
- Task 3 (2d0820d): wire contract battery — golden JSON for available_commands_update (byte-for-byte), empty set as [], user_message_chunk echo pinned against the literal schema spelling with a manual mutation check (flip goes red, revert green, flip NOT committed), full-replacement twice-fire proof, composition-level TestCommandsNotifySeam over Run pipes.

## Self-Check: PASSED

- go test -race -count=1 ./internal/runtime/ ./internal/acp/ ./internal/acpserve/ green (one pre-existing load flake: TestAskPark_PromptResponsePrecedesResolution fails at BASELINE too under full-suite parallel load, passes in isolation — unrelated to this plan, recorded below).
- golangci-lint on new/modified files clean modulo the repo-wide exhaustruct_v5/wsl_v5 storm from the golangci 2.13.2 rename drift (baseline red; .golangci.yml exclusions reference pre-rename linter names — flagged for the operator).

## Deviations from Plan

- **[Rule 2 - missing detail] tracer advertisement assertion scoped to membership+sortedness**: user-scope discovery (the operator's real ~/.claude tree) makes the runner-level winner set environment-dependent, so the tracer asserts greet/init/status membership + sorted + unique instead of exact equality. Exact-set coverage lives in the pure buildChain battery (registry built in code), which is the stronger pin anyway.
- **[Rule 1 - bug in plan fixture] "plan" is not a reserved name**: the Task 2 battery's first draft used /plan as a reserved collision case; the CMDS-02 thirteen (help status cost mcp memory permissions doctor config model clear resume compact init) do NOT include plan. Fixture corrected to /compact; no production impact (RED caught it before any code change).
- **[Rule 3 - test-side breakage] notification-before-response at session/new**: 9 acp harness sites + 5 runtime e2e sites read exactly the response frame after session/new; they now use readResultFrame/readResultFramesCounting (skip + count notifications). TestTurnEmitterEmptyTurn's no-notification invariant baselined after the session/new response (the advertisement is a real emission).

## Issues Encountered

- Pre-existing (verified at baseline via stash): TestAskPark_PromptResponsePrecedesResolution is load-flaky in the full runtime suite ("response took 12.9s; want near-instant") — passes in isolation. Not touched (scope boundary).
- Pre-existing: `mise run lint` red at baseline (golangci-lint 2.13.2 renamed exhaustruct→exhaustruct_v5 etc.; config exclusions match old names → ~1.6k false positives repo-wide). My files pass every linter still matching the config's exclusion set.

## Next Phase Readiness

Ready for 20-02 (class-B family handlers land in builtinTable + commands.go), 20-03, 20-04 (class-A body injection point is initClassAReservation), 20-05 (rebuildCommandChain + SetCommandsNotify are the swap hook).
