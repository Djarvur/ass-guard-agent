---
status: resolved
trigger: "G-15-1 (zed-session-resume-unsupported): After restarting Zed mid-session, attempting to resume the previous session shows 'Failed to Launch — Loading or resuming sessions is not supported by this agent.' instead of the v1.1-close behavior."
created: 2026-08-27T00:00:00Z
updated: 2026-08-27T13:25:59Z
---

## Current Focus

hypothesis: "CONFIRMED — ass-guard's ACP initialize response advertises `agentCapabilities.loadSession:false` (original v1.0 Phase-02 D-09 'NO REPLAY IN v1'); Zed's `resume_session` refuses client-side exactly when that resume capability is absent, producing 'Loading/Resuming sessions is not supported by this agent.' This behavior is unchanged since v1.1 (not a Phase 15 carve regression); the D-09 reversal ('session resume = must-have', ROADMAP.md:38) is scheduled as v1.2 Phase 18 Session Family, which is READY TO EXECUTE but not implemented."
test: "git diff v1.1..HEAD -- internal/acp/ (empty); git show v1.1:internal/acp/handlers.go vs working tree (identical); live-wire drive of freshly built HEAD binary"
expecting: "initialize reply carries loadSession:false and session/load returns -32601 — observed exactly"
next_action: "None — root cause delivered; fix owned by Phase 18 (18-01 loadSession:true + reconcile-then-replay, 18-04 sessionCapabilities). Disposition recorded in 15-UAT.md Deferred Follow-Ups and 15-07-SUMMARY.md."

bug_class: Bohrbug (deterministic — reproduces identically on every attempt)
known_pattern_candidate: none (no knowledge base existed)

## Symptoms

expected: Restart of Zed mid-session replays matching v1.1-close behavior (session/load no-op, D-09 — a fresh session starts; transcripts remain on disk under .ass-guard/)
actual: Zed shows "Failed to Launch — Loading or resuming sessions is not supported by this agent." when the operator attempts to open/resume the previous session from Zed's history.
errors: "Failed to Launch: Loading or resuming sessions is not supported by this agent." (Zed UI error dialog)
reproduction: Test 1 in UAT (.planning/phases/15-internal-runtime-carve-step-0/15-UAT.md) — spawn `ass-guard acp serve` from Zed as in daily use, restart Zed mid-session, attempt to resume the session from Zed's session history.
started: Discovered during UAT — Phase 15 "internal/runtime Carve (Step 0)" operator live-Zed checklist (phase-review.md, ROADMAP criterion 2). The carve moved the turn runner from cmd/acpserve into internal/runtime + internal/runtime/enginebridge (verbatim-move discipline).

## Eliminated

- hypothesis: "The Phase 15 carve regressed/dropped the loadSession capability advertisement or session/load handling"
  evidence: "`git show v1.1:internal/acp/handlers.go` is byte-identical to working tree for both regions: line 50 advertises `\"loadSession\": false` and lines 187-192 return RPCError CodeMethodNotFound (-32601). Also `git log --oneline v1.1..HEAD -- internal/acp/` returns NO commits; `git diff v1.1..HEAD --stat -- internal/acp/handlers.go internal/acp/server.go` is empty."
  timestamp: 2026-08-27

- hypothesis: "Zed sends session/load and ass-guard's -32601 error response produces the dialog (server-side failure)"
  evidence: "Zed source crates/agent_servers/src/acp.rs `resume_session` (~line 1771-1783, current master): `if self.agent_capabilities.session_capabilities.resume.is_none() { return Task::ready(Err(anyhow!(LoadError::Other(\"Resuming sessions is not supported by this agent.\")))) }` — a pure client-side capability check that fires BEFORE any JSON-RPC request is sent; sibling message 'Loading sessions is not supported by this agent.' at line 1739 in the load path. ass-guard never receives session/load on this path."
  timestamp: 2026-08-27

- hypothesis: "The enginebridge ACPDispatcher or another post-carve component changed initialize handling"
  evidence: "internal/runtime/enginebridge/enginebridge.go:356 NewACPDispatcher implements continue/hook/ask bridging (autocontinue), not JSON-RPC method routing. Production serve path composes the unchanged internal/acp server: internal/acpserve/acp_serve.go:200 `srv := acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner))`."
  timestamp: 2026-08-27

## Evidence

- timestamp: 2026-08-27
  checked: "grep for session/load + loadSession across cmd/ and internal/"
  found: "internal/acp/handlers.go:50 advertises `\"loadSession\": false`; handlers.go:28 registers `s.handlers[\"session/load\"] = s.handleSessionLoad`; handlers.go:187-192 returns RPCError{Code: CodeMethodNotFound(-32601), Message: 'session/load not supported (loadSession is false; replay is out of v1 scope — D-09)'}. internal/runtime/integration_test.go:260 TestIntegration_SessionLoadNoOp asserts -32601 end-to-end POST-carve. cmd/ass-guard/acp_serve.go:49 CLI help text documents 'session/load is a no-op (D-09 — NO replay)'."
  implication: "Current wire behavior: initialize says loadSession:false AND session/load errors -32601. Both surfaced intentionally per original decision D-09."

- timestamp: 2026-08-27
  checked: ".planning/ROADMAP.md:38"
  found: "'Operator decisions at close (2026-08-23…25): D-09 REVERSED (session resume = must-have); … mimicry abandoned as the product bar in favor of client-native ACP surfaces'"
  implication: "D-09 was REVERSED at v1.1 close — session resume declared must-have — but its implementation is scheduled for v1.2 Phase 18 'Session Family' (STATE.md: current_phase 18 READY TO EXECUTE). The shipped binary still carries pre-reversal D-09 behavior."

- timestamp: 2026-08-27
  checked: "git show v1.1:internal/acp/handlers.go"
  found: "At tag v1.1, handleInitialize advertises loadSession:false (line 50) and handleSessionLoad returns RPCError CodeMethodNotFound (-32601) (lines 187-190) — identical to current working tree."
  implication: "No regression introduced by the Phase 15 carve: initialize capabilities and session/load handling are unchanged since v1.1."

- timestamp: 2026-08-27
  checked: "ROADMAP.md criterion 2 (line 70) and phase-review.md baseline (lines 92, 100)"
  found: "Criterion 2: 'a live Zed-spawned session streams tokens, executes tools, replays on restart exactly as at v1.1 close (the operator's daily-use surface unchanged)'. phase-review.md:92: 'Baseline: daily-use memory of the v1.1-close editor session surface'; :100 step 4: 'Restart Zed mid-session; confirm replay-on-restart matches v1.1 close'."
  implication: "The UAT criterion is an equivalence criterion. At the protocol level the binary DOES behave exactly as at v1.1 close. What the UAT newly exercised is the explicit resume-from-history click, which daily use never relied on (a plain Zed restart opens a fresh thread via session/new)."

- timestamp: 2026-08-27
  checked: "Live-wire drive of freshly built HEAD binary (`go build -o /tmp/ass-guard-g15 ./cmd/ass-guard`; scripted stdin frames; stdout captured)"
  found: "initialize -> `{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":1,\"agentCapabilities\":{\"loadSession\":false},\"agentInfo\":{\"name\":\"ass-guard\",\"version\":\"0\"},\"authMethods\":[]}}`; session/new -> fresh sessionId; session/load -> `{\"code\":-32601,\"message\":\"session/load not supported (loadSession is false; replay is out of v1 scope — D-09)\"}`"
  implication: "Deterministic reproduction of the proximate cause against HEAD as-built. Direct observation, not inference."

- timestamp: 2026-08-27
  checked: ".planning/milestones/v1.0-phases/02-session-core-acp-interface/02-RESEARCH.md (lines 31, 41, 58, 79)"
  found: "Original D-09 definition: 'MAJOR SCOPE REDUCTION: NO REPLAY IN v1. ACP-03 (session/load replay) DROPPED… `session/load` is no-op or new-session fallback'; ACP-03 listed DROPPED per D-09 ('no-op/fallback; NOT implemented')."
  implication: "The UAT truth statement ('v1.1-close behavior: session/load no-op — a fresh session starts') reflects the DECISION language, not the shipped IMPLEMENTATION. v1 shipped -32601 + loadSession:false, so an explicit resume attempt would have failed identically at v1.1 close."

- timestamp: 2026-08-27
  checked: ".planning/phases/18-session-family/18-01-PLAN.md (lines 29, 113, 139) and 18-04-PLAN.md (lines 32, 119)"
  found: "18-01 plans 'initialize advertises loadSession: true' plus full replay of cleanly-closed sessions; 18-04 plans the nested sessionCapabilities {list:{}, close:{}, delete:{}} advertisement. 18-05 wires reconciliation into load."
  implication: "The fix direction already exists as an approved, planned Phase 18 implementation (the reversed-D-09 realization). G-15-1 is the concrete manifestation of Phase 18 being not-yet-executed."

## Resolution

root_cause: "PRE-EXISTING BY-DESIGN GAP, NOT A PHASE-15 CARVE REGRESSION. ass-guard's ACP initialize response advertises `agentCapabilities.loadSession:false` (internal/acp/handlers.go:50) and its session/load handler answers -32601 (handlers.go:187-192) — the literal embodiment of the original v1.0 Phase-02 D-09 'NO REPLAY IN v1'. When the operator attempts to open/resume a persisted thread, Zed performs a CLIENT-SIDE capability check first (Zed crates/agent_servers/src/acp.rs, resume_session ~line 1771: resume capability absent -> Err('Resuming sessions is not supported by this agent.'); load path line 1739 equivalent) and aborts launch WITHOUT sending session/load. Because the advertisement has been identical since v1.1 (`git diff v1.1..HEAD -- internal/acp/` is empty; live-wire test of HEAD build confirms loadSession:false), the behavior today IS the v1.1-close wire behavior — it merely became visible because the UAT checklist asked the operator to exercise explicit resume-from-history for the first time. The deeper cause: D-09 was REVERSED at v1.1 close ('session resume = must-have', .planning/ROADMAP.md:38), and its implementation was scheduled as v1.2 Phase 18 'Session Family' (18-01 flips loadSession:true + replay; 18-04 adds sessionCapabilities), which STATE.md shows as READY TO EXECUTE — i.e., a known, planned gap sequenced after Phase 15, surfacing during the Phase 15 equivalence checkpoint."
fix: "(none applied — find_root_cause_only) Fix direction: execute planned Phase 18 — flip initialize advertisement to loadSession:true (18-01) with reconcile-then-replay session/load, then add sessionCapabilities list/close/delete (18-04). No code change belongs in Phase 15; the carve preserved v1.1 behavior exactly as its criterion demanded."
verification: "n/a (diagnosis only). Reproduced deterministically at HEAD via live-wire drive; carve-regression alternative falsified by empty v1.1..HEAD diff on internal/acp/ + byte-identical handler at tag v1.1 + production composition proven to be acp.NewServer (internal/acpserve/acp_serve.go:200)."
files_changed: []

### RCA branching record

candidate_causes:
  - "code: handlers advertise loadSession:false / session/load -32601 per original D-09 (CONFIRMED — proximate cause)"
  - "process/schedule: reversed-D-09 implementation deferred to v1.2 Phase 18, not yet executed (CONFIRMED — why the gap persists past the v1.1-close reversal)"
  - "environment: Zed version drift changing capability schema (RULED OUT as cause-of-failure — any variant of Zed's check keys off the same absent advertised capability)"
  - "data: transcript/state corruption under .ass-guard/ preventing load (RULED OUT — dialog fires client-side before any load attempt; no RPC sent)"
and_gate: "no — single contributing condition (absent/false resume capability advertisement) deterministically produces the refusal; the phase-sequence fact explains persistence, not a second simultaneous failure condition."
