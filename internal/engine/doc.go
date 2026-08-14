// Package engine implements the unified decision engine (Phase-4 ENG-01..05).
//
// The engine is a POST-TURN OBSERVER (D-01): after the Session Core's turn loop
// reaches end_turn and the response is fully streamed + transcripted, the engine
// inspects the just-finished turn's output and decides what to do next —
// continue the SDD scenario (inject the next-stage prompt), trigger a hook-DAG,
// ask the user, wait, or do nothing. It NEVER sits inside the turn loop's
// critical path (D-04): if it panics or misconfigures, the turn has already
// completed normally, so the engine's failure is invisible to the caller (the
// original stop reason + error are returned unchanged — graceful degradation).
//
// The continue decision uses DUAL-SIGNAL detection (D-02): (1) the assistant's
// output text matches a known handoff pattern from the PatternTable, OR (2) the
// model invoked a known handoff tool-call. Either signal ⇒ the engine launches
// the next stage; NEITHER ⇒ ActionNothing (D-03 structural safety — unmatched
// output triggers nothing; the only off-switch is manual cancellation, which
// drains the engine's queued continue-injections).
//
// Every Decision carries the source turn id + matched signal (D-05 provenance);
// the wrapper emits one EngineDecision event + writes one engine_decision
// transcript line per turn (including ActionNothing, so the audit log proves the
// structural-safety property).
//
// Plan 04-01 ships the TRACER: the pure Decide function + the Observe wrapper
// + the TurnRunner/PatternTable seams. Plan 04-02 supplies the OpenSpec-backed
// PatternTable; 04-03 ships the hook-DAG; 04-04 ships real tool execution;
// 04-05 wires the real seams into the ACP serve layer; 04-06 adds learning;
// 04-07 completes the cancel-drain.
package engine
