---
status: diagnosed
trigger: "UAT G-23-1: Zed rejects /undo client-side — '/undo is not a recognized command in ass-guard … Available commands for ass-guard: none'. available_commands_update advertisement empty/absent in fresh binary (built 22:11, tested 22:17). Steering works, tools work — only command surface dead."
created: 2026-09-11T22:30:00+03:00
updated: 2026-09-11T23:05:00+03:00
audit_acknowledged:
  milestone: v1.2
  at: 2026-09-20
  status: diagnosed
---

## Current Focus

hypothesis: CONFIRMED — ass-guard's handleSessionNew sends available_commands_update BEFORE the session/new response (the 16-01 "updates-before-response" Barrier order mirrored in 20-01); Zed's session/new client path cannot pre-register the session (it learns the sessionId only from the response) and handle_session_notification DROPS every session/update for an unregistered session ("unknown session" warn). The well-formed, non-empty advertisement is therefore discarded client-side -> command list empty -> "/undo is not a recognized command … Available commands for ass-guard: none". Known upstream: zed-industries/zed#60199 (open).
test: DONE — realistic fake-editor drive of the shipped binary (probe answered, initialize completed, then session/new): full advertisement frame (14 commands incl. undo, correct sessionUpdate kind) arrives BEFORE the id:2 response on the wire. Zed source (main, crates/agent_servers/src/acp.rs): new_session awaits response -> only then constructs AcpThread/registers; handle_session_notification drops unknown-session updates. session/load path pre-registers (acp.rs:1258-1267) which is why the load path's pre-response order works — mirroring it onto session/new was the invalid generalization.
expecting: (met) Advertisement non-empty on the wire; drop happens client-side.
next_action: none — diagnosis complete (goal: find_root_cause_only). Fix direction (NON-BINDING): emit the session/new advertisement AFTER the response frame is written (defer the NotifyAvailableCommands past handler return), or a per-request post-response hook; the rescan re-fire (NotifyAllAvailableCommands) then covers later discovery changes as today.

## Symptoms

expected: In a live editor session, /undo (mid-turn and idle) reaches the agent's class-B command resolver — Zed offers it via autocomplete and executes cancel-then-restore / instant restore. Same for cross-session restore (Test 2).
actual: Zed client-side rejects /undo with 'An Error Happened: /undo is not a recognized command in ass-guard … Available commands for ass-guard: none'. Test 2 (cross-session) hit the same wall («то же»). Steering works in the same session; tools/turns work; ONLY the command advertisement surface is dead.
errors: "An Error Happened: /undo is not a recognized command in ass-guard … Available commands for ass-guard: none" (Zed UI, screenshots 2026-09-11 22-17/22-18)
reproduction: Live Zed UAT per 23-UAT.md test 1. Stand-in USED: fake editor over ACP stdio (initialize -> session/new), Zed's exact launch args — deterministic reproduction achieved.
started: Shipped broken at commit 89bcc6f (Phase 20-01, 2026-09-07); first LIVE detection 2026-09-11 22:17 (+0300) because Phase 20's operator autocomplete test (20-HUMAN-UAT.md test 1) has been [pending] since 2026-09-08.

## Eliminated

- hypothesis: Stale binary (G-19-2 class repeat)
  evidence: Binary fresh: ~/go/bin/ass-guard built 2026-09-11 22:11:36 +0300, test ran 22:17 (operator evidence, pre-gathered; plus my own wire reproduction against that same binary).
  timestamp: 2026-09-11T22:30:00+03:00

- hypothesis: Advertisement emitted EMPTY (nil commandSrc / broken chain) or silenced by the Phase 25 kit extraction
  evidence: Realistic fake-editor drive of the SHIPPED binary (Zed's exact args, elicitation probe answered like Zed, initialize completed, then session/new): available_commands_update arrives with the FULL set — 14 commands including undo, correct frame shape (update.availableCommands + update.sessionUpdate="available_commands_update") — BEFORE the session/new response. Not empty, not absent, not malformed. Static path at HEAD also intact (WithCommandSource at acp_serve.go:404; buildChain pre-seeds builtins; commandChainRef degrades to builtin-only chain).
  timestamp: 2026-09-11T22:35:00+03:00

- hypothesis: Zed requires specific clientCapabilities to honor the advertisement
  evidence: Irrelevant to the drop — Zed's dispatch never reaches capability-specific handling; the drop is at session registration lookup (handle_session_notification).
  timestamp: 2026-09-11T22:50:00+03:00

- hypothesis: Phase 25 (kit extraction) regression window broke the emitter arming
  evidence: Emitter armed and firing in the shipped binary (wire reproduction); introducing commit for the pre-response order is 89bcc6f (2026-09-07, Phase 20-01) — long before Phase 25.
  timestamp: 2026-09-11T22:55:00+03:00

## Evidence

- timestamp: 2026-09-11T22:30:00+03:00
  checked: Error phrase in binary
  found: "not a recognized command" = 0 hits in binary strings — phrase is Zed's, not ass-guard's. Binary DOES contain "available_commands_update", reserved builtin strings, and the "available_commands_update enqueue failed (continuing)" degrade paths.
  implication: Zed blocks /commands absent from the agent's advertisement; the advertisement surface was not being registered client-side.

- timestamp: 2026-09-11T22:31:00+03:00
  checked: Static wiring path at HEAD (internal/acp/handlers.go:335-369, internal/acp/server.go:538-560, internal/acpserve/acp_serve.go:404, internal/acpserve/command_source.go, kit/runtime/commands.go:253-334,481-487)
  found: Wiring INTACT. handleSessionNew fires NotifyAvailableCommands BEFORE returning the response (deliberate "updates-before-response" Barrier order, handlers.go:353-366). /undo IS a live builtin (kit/runtime/commands.go:181).
  implication: Source at HEAD cannot advertise an empty set through this path; suspicion shifts to delivery/registration.

- timestamp: 2026-09-11T22:31:30+03:00
  checked: Artificial-run wire capture (session/new sent while initialize probe unanswered)
  found: Frames: [elicitation/create probe, available_commands_update (full set), session/new response, $/cancel_request, initialize response]. The notification precedes the response on the wire by construction (Barrier).
  implication: Ordering is deliberate and observable; set is non-empty.

- timestamp: 2026-09-11T22:35:00+03:00
  checked: Realistic wire capture (probe answered, initialize completed, THEN session/new — Zed's real sequencing) + Zed upstream issue research
  found: Same pre-response ordering with the full 14-command set. Upstream match: zed-industries/zed#60199 (OPEN since 2026-07-01, needs triage): "ACP available_commands_update never shown because notification arrives before session/new response" — identical mechanism, identical symptom phrasing ("The /init command is not supported by CodeBuddy Code. Available commands: none"); Zed logs "Received session notification for unknown session" for each pre-response session/update.
  implication: Known Zed client behavior; the agent's pre-response emission is the triggering condition.

- timestamp: 2026-09-11T22:45:00+03:00
  checked: Zed source, current main — crates/agent_servers/src/acp.rs, crates/acp_thread/src/acp_thread.rs
  found: handle_session_notification (acp.rs ~4772-4784): sessions.get(session_id) -> None => warn "Received session notification for unknown session" + return (DROP). new_session (acp.rs:1606+): AWAITS the session/new response, only THEN constructs AcpThread/registers — pre-registration impossible on session/new (id unknown). Contrast session/load (acp.rs:1258-1267): pre-registers BEFORE awaiting, with an explicit comment saying so ("so that any session/update notifications that arrive during the call ... can find the thread"). acp_thread.rs:2669-2674 applies AvailableCommandsUpdate once delivered.
  implication: ass-guard's pre-response emission is structurally undeliverable on session/new in Zed; the same order on session/load works because Zed pre-registers there. The 20-01 comment "the barrier keeps the updates-before-response order the load path established" is exactly the invalid generalization.

- timestamp: 2026-09-11T22:55:00+03:00
  checked: git archaeology — internal/acp/handlers.go session/new advertisement; 20-HUMAN-UAT.md
  found: Introduced in 89bcc6f "feat(20-01): ..." (2026-09-07 21:18 +0300). Phase 20 operator UAT tests 1-3 (autocomplete, /status live, mid-session pickup) ALL still [pending] since 2026-09-08 — the advertisement surface was NEVER live-verified before today.
  implication: Broken-on-arrival against Zed since 20-01; not a Phase 25 regression.

- timestamp: 2026-09-11T23:00:00+03:00
  checked: Post-registration re-fire possibility in a quiescent session
  found: The ONLY per-session advertisement is the session/new one. Startup SetCatalog fires commandsNotify before any session exists (NotifyAllAvailableCommands over 0 sessions = no-op); later re-fires only on .claude/ discovery changes (fsnotify, 20-05) which did not occur during UAT. Confirmed in the stdio drive: no further available_commands_update after the response.
  implication: Zed's command registry for the thread stays empty for the whole session — explains both UAT failures (mid-turn AND idle /undo; both test-2 sessions).

## Resolution

root_cause: >
  AND-gated interoperability mismatch (both conditions necessary):
  (1) agent-side [code]: internal/acp/handlers.go handleSessionNew emits the available_commands_update
  notification BEFORE writing the session/new response (TurnEmitter Barrier order mirrored from the
  16-01/18-05 load path; introduced 89bcc6f / Phase 20-01, 2026-09-07, never operator-verified —
  20-HUMAN-UAT tests 1-3 pending);
  (2) client-side [environment]: Zed (all versions through current main; upstream issue
  zed-industries/zed#60199, open) drops every session/update whose session is not yet registered, and on
  session/new it CANNOT pre-register because the sessionId is learned only from the response
  (crates/agent_servers/src/acp.rs new_session vs the pre-registering session/load path).
  The full, well-formed advertisement (14 commands incl. /undo — verified on the wire against the shipped
  fresh binary) is discarded client-side; Zed's per-thread command list stays empty and every slash
  command is client-side rejected ("/undo is not a recognized command … Available commands for
  ass-guard: none"). NOT a stale binary, NOT an empty/absent advertisement, NOT a Phase-25 kit-extraction
  regression.
fix: (diagnose-only mode — not applied) NON-BINDING direction: move the session/new advertisement to AFTER the session/new response frame is written (e.g. post-response emission hook / deferred enqueue ordered behind the response write); the 20-05 rescan re-fire (NotifyAllAvailableCommands on discovery change) already covers later updates, and the session/load pre-response order is correct as-is (Zed pre-registers there — do not "fix" that one).
verification: Wire-level reproduction with the shipped binary (fresh, Zed's exact launch args) + Zed main source reading + upstream issue #60199 symptom match + git archaeology (89bcc6f, pending 20-UAT). Live-Zed confirmation of a post-response fix belongs to the fix leg.
files_changed: []
