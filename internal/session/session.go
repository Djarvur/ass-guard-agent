package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
	"github.com/Djarvur/ass-guard-agent/internal/toolexec"

	"github.com/Djarvur/ass-guard-agent/internal/event"
)

var errToolLoopExceededMax = errors.New("tool loop exceeded max iterations")
var errToolLoopExceeded = errors.New("session: tool loop exceeded max iterations")

// stubToolResult is the canned Phase-2 tool result (D-15 — execution stays
// stubbed; real execution lands in Phase 4). The transcript records it so the
// reconstruction is faithful even before tools are real.
// stubResultMsg is the canned Phase-2 tool-result message (D-15).
const stubResultMsg = `{"output":"stubbed in Phase 2 (real execution in Phase 4)"}`

var stubToolResult = json.RawMessage(stubResultMsg) //nolint:gochecknoglobals // immutable table

// Session is the Session Core (D-17): it owns the transcript Manager, the
// lean-window Projector, and the turn loop (D-18). It is the sole writer of
// session-structure lines (user/assistant/tool/boundary/canceled); streaming
// audit lines (request_shaped, chunks, usage) are written by the async
// TranscriptWriter (LOG-02).
//
// The Session takes the Phase-1 Shaper + Provider as injected dependencies.
// Phase-2 tool execution is stubbed (D-15); Plan 02-05 wires Provider.Stream +
// boundaries; Plan 02-06 wires subagent dispatch.
type Session struct {
	Manager   *Manager
	Projector *Projector
	Provider  provider.Provider
	Bus       *event.Bus
	Semaphore *provider.Semaphore
	Profile   profile.Profile
	WorkDir   string
	SessionID string

	// Catalog + configAdded drive boundary detection (wired in Plan 02-05).
	Catalog     *toolcat.Catalog
	ConfigAdded []string

	// SubagentTypes maps discovered agent definitions (plugin-bundled AND
	// first-class `.claude/agents/` project+user — 12-02) by type name. An
	// Agent/Task tool call carrying subagent_type=<name> dispatches with the
	// definition's Prompt (per-dispatch system block) and Tools (the restricted
	// set); an unknown/absent type falls back to the default restricted set
	// (the listing is advisory — graceful degradation).
	SubagentTypes map[string]ecosys.Agent

	// Hooks is the Claude-Code lifecycle hook runner (12-02 Task 4): nil =
	// no hooks wired (every seam no-ops). UserPromptSubmit fires at Prompt
	// entry (post-expansion — cmd expands BEFORE calling Prompt), SessionStart
	// lazily on the first Prompt (the session-create seam), Stop at parent
	// turn end, SubagentStop at subagent completion, SessionEnd in Close.
	Hooks *ecosys.HookRunner

	// Checkpointer snapshots the workspace at PARENT turn start (14-01,
	// EARLY-01): the snapshot taken BEFORE any turn work is what makes
	// restore a pre-turn recovery point ("undo this turn"). Nil = disabled
	// (tracer-mode compat). A snapshot failure is loud but never turn-fatal
	// (the AUD-03 discipline). Subagent turns do NOT snapshot — they run
	// inside the parent turn's recovery window.
	Checkpointer Checkpointer

	// planMode is the per-session plan-mode state (12-04, ACP-02): nil =
	// plan mode not wired (the gate never fires). The state carries the
	// runtime-level mutating-tool gate MIRRORED FROM THE CAPTURE (the 12-05
	// re-record proved the target enforces it — see planmode.go).
	planMode *PlanModeState

	// SubagentModel is the scheduler light-tier model slug for SUBAGENT
	// dispatches (14-05, EARLY-05 — the token-economics lever). Empty (the
	// default) keeps the parent model exactly as before: the routing is
	// config-conditional through the EXISTING tiers table, never a new
	// surface. The override is applied on the per-dispatch profile COPY in
	// subagentProfile — the shared session profile (and the main turn loop's
	// request shape) is never touched.
	SubagentModel string

	// sessionStartFired pins the lazy SessionStart seam to exactly once.
	sessionStartFired bool

	// toolExec is the (stubbed) tool executor; Plan 02-06 wraps it in a
	// RestrictedExecutor for subagents.
	toolExec toolcat.ToolExecutor

	// subagentRunner is the seam for Task/Agent dispatch (Plan 02-06). Nil →
	// defaultSubagentRunner (real nested loop). Tests inject a fake.
	subagentRunner subagentRunner

	// OnClose is the session-end hook (Plan 05-01 T4): when set, Session.Close
	// runs it exactly once (idempotent). cmd/ass-guard sets it to host.Close()
	// so MCP subprocesses are reaped when the session ends (logout/cancel/ctx).
	// It is a func seam (not a *mcp.Host) so internal/session has no import
	// cycle on internal/mcp.
	OnClose func() error

	// ask is the per-session AskUserQuestion suspension broker (12-01, ACP-01).
	// Nil = asks are not wired (an ErrSuspended result degrades to a structured
	// error so the model is never silently dead-ended). Wired via SetAskBroker.
	ask *AskBroker

	// askResumeCtx is the context timer-driven ask resumes run under — the
	// suspending turn's ctx dies with its prompt response, so the D-01 timer
	// must resume under the serve-lifetime ctx.
	//
	//nolint:containedctx // deliberate serve-lifetime ctx storage (see SetAskBroker)
	askResumeCtx context.Context

	// resumeSerial serializes the ASYNC resume drivers (the ask queue's pump
	// resolution, the D-01 timer, the drain resolutions) against the runtime's
	// per-session turn mutex (17-REVIEW CR-02: the 12-07 discipline — the
	// whole turn, ask-reply resumes included, holds the session mutex — was
	// violated by every asynchronous resume, letting a dialog answer race a
	// new client prompt into TWO concurrent model loops on one transcript).
	// Composition injects the mutex-guarded execution (runtime's sessionFor:
	// r.sessionTurnMu(sessionID)); the SYNC reply path (routeAskReply inside
	// Run) bypasses it — it already holds the mutex, and the func is not
	// reentrant. nil (bare sessions, tests) = no serialization.
	resumeSerial func(func())

	// gate is the permission-gate chokepoint's injected dependencies (17-02,
	// ACP-01): nil = the gate is unwired and every call executes (the v1.1
	// zero-dialog behavior — "available, not default" preserved for sessions
	// that never opt in). Wired via SetPermissionGate.
	gate *GateDeps

	// automationTurn marks the CURRENT turn as automation-origin (12-07
	// engine-driven firing; 17-02 D-07's human-present signal). Set by the
	// runtime entry around the firing turn; client-driven turns never set
	// it. The gate reads it per call.
	automationTurn atomic.Bool

	// permDegradedFlag is the sticky 16-D-18 degradation for permission asks:
	// once the client answers -32601, every later gated ask declines without
	// a new surface round-trip (never a silent allow, never a retry storm).
	permDegradedFlag atomic.Bool

	closeOnce sync.Once

	turnCounter atomic.Int64
}

// Checkpointer is the turn-boundary snapshot seam (14-01, EARLY-01): Prompt
// calls SnapshotTurn at turn entry — BEFORE any turn work — so the recorded
// snapshot is the pre-turn recovery point. The interface is defined HERE
// (not by importing internal/checkpoint) so the session package keeps no
// dependency on the store — the OnClose func-seam pattern.
type Checkpointer interface {
	SnapshotTurn(ctx context.Context, sessionID, turnID string) error
}

// nextTurnID returns a monotonically-increasing turn id for this session.
func (s *Session) nextTurnID() string { //nolint:funcorder // ordering groups related logic
	n := s.turnCounter.Add(1)

	return fmt.Sprintf("%s-turn-%03d", s.SessionID, n)
}

// SeedResume seeds the turn counter from the transcript maxima (18-01, ACP-06
// — "continued id sequences from transcript maxima"): the resume path calls it
// with MaxTurnCounter's result so the next nextTurnID() continues the on-disk
// sequence instead of restarting at 001 (18-RESEARCH Pitfall 2 — replayed and
// new frames must never collide). Exported because the resume orchestration
// lives at the runtime layer, one package up.
func (s *Session) SeedResume(maxTurns int64) {
	s.turnCounter.Store(maxTurns)
}

// CurrentTurnID returns the id of the most-recently STARTED turn, or "" when
// no turn has started. It is a non-incrementing atomic read (09-01, AUD-02):
// the serve-path capturer closure calls it when a RequestShaped event fires so
// every request line carries real turn attribution — the correlation triple
// (session-from-filename, turn, request). The rejected alternative (an
// empty-TurnID fallback relying on append ordering) is documented in the
// Phase-9 context; the accessor was chosen deliberately.
func (s *Session) CurrentTurnID() string {
	n := s.turnCounter.Load()
	if n == 0 {
		return ""
	}

	return fmt.Sprintf("%s-turn-%03d", s.SessionID, n)
}

// Prompt runs the D-18 turn cycle for one user prompt: project the lean window,
// shape + send (capturer publishes RequestShaped), emit results, loop on
// tool_calls (stubbed execution), and return the stopReason. ctx cancellation
// aborts the turn and records a canceled line (D-16). A panic is recovered at
// the turn boundary and recorded as an investigate-and-fix-ready error line
// (PROJECT.md, D-13 for the parent).
//
// An AskUserQuestion tool call SUSPENDS the turn (12-01, ACP-01/D-01): the
// stop marker is stopAsk, no tool result is appended for the pending call, and
// the pending ask lives in the session broker until the operator's reply
// (ResolveAsk — the reply IS the tool result) or the D-01 timer resumes the
// SAME turn's loop via runTurn.
//
//nolint:nonamedreturns // stop/err assigned by the panic-recovery defer
func (s *Session) Prompt(ctx context.Context, userPrompt []ContentBlock) (stop string, err error) {
	turnID := s.nextTurnID()
	// Recover at the goroutine/turn boundary (D-13 parent-side): a panic becomes
	// an investigate-and-fix-ready error line + an error return (never a crash).
	defer func() {
		if r := recover(); r != nil {
			//nolint:err113 // dynamic error message
			s.appendError(turnID, "session", fmt.Errorf("turn panic: %v", r), false)
			//nolint:err113 // dynamic error message
			err = fmt.Errorf("session turn panic recovered: %v", r)
			stop = ""
		}
	}()

	// 14-01 (EARLY-01): shadow-git snapshot at PARENT turn start — BEFORE
	// any turn work (hooks, user message, provider calls) — so the snapshot
	// is exactly the PRE-turn state restore returns to ("undo this turn").
	// Subagent turns never reach here (DispatchSubagent runs inside the
	// parent turn's recovery window). A snapshot failure is LOUD (stderr +
	// the investigate-and-fix-ready transcript line) but never turn-fatal —
	// the AUD-03 audit-write failure discipline.
	if s.Checkpointer != nil {
		cerr := s.Checkpointer.SnapshotTurn(ctx, s.SessionID, turnID)
		if cerr != nil {
			slog.Error("checkpoint: snapshot failed (turn continues without a checkpoint)",
				"turnID", turnID, "error", cerr.Error())
			s.appendError(turnID, "checkpoint", cerr, true)
		}
	}

	// 12-02 Task 4: the turn-entry hook seams. Prompt receives the
	// POST-EXPANSION blocks (cmd expands slash-commands before calling), so
	// UserPromptSubmit fires here with the expanded prompt. SessionStart
	// fires lazily on the FIRST Prompt (the session-create seam — the runner
	// is wired at sessionFor but the session's first activity is its turn).
	// Context-bearing stdout (both events' documented role) is INJECTED as a
	// SYSTEM block on the session profile — the established
	// dynamic-merge-into-captured-shape vehicle (the skills/agents listings
	// use the same trailing-System-block form): the model sees it as context,
	// the recorded user_message line stays byte-pure (corpus-absent form: no
	// captured session carries hook output; flagged for the re-capture).
	if s.Hooks != nil {
		if !s.sessionStartFired {
			s.sessionStartFired = true
			s.injectHookContext(s.Hooks.Fire(ctx, "SessionStart", nil))
		}

		s.injectHookContext(s.Hooks.Fire(ctx, "UserPromptSubmit", map[string]any{
			"prompt": firstTextOf(userPrompt),
		}))
	}

	err = s.Manager.AppendUserMessage(turnID, userPrompt)
	if err != nil {
		return "", err
	}

	return s.runTurn(ctx, turnID)
}

// firstTextOf returns the first text block's text (the UserPromptSubmit
// payload's prompt field).
func firstTextOf(blocks []ContentBlock) string {
	for _, b := range blocks {
		if b.Type == blockText {
			return b.Text
		}
	}

	return ""
}

// toolCallIDOf returns the tool call's REAL provider id (T1's seam carry),
// falling back to the tool name when the call carries no id (legacy fake
// providers and parity arms) so tool_call/tool_result pairing stays consistent
// end-to-end either way.
func toolCallIDOf(tc provider.ToolCall) string {
	if tc.ID != "" {
		return tc.ID
	}

	return tc.Name
}

// subagentResultPayload encodes a subagent's final result as PROPER JSON for
// the truncation chokepoint (CR-02). The previous naive quote-concat
// produced INVALID JSON for any result needing escapes — every multi-line
// report — which bypassed the cap AND failed appendLine's Marshal (the
// tool_result line was dropped from the transcript entirely). For
// escape-free results json.Marshal is byte-identical to the old concat
// (test-pinned); a Marshal error is defensive-only (invalid UTF-8 is
// replaced rather than errored, per truncateToolResult's contract) and
// degrades to the error-payload form.
func subagentResultPayload(result string) (json.RawMessage, bool) {
	payload, err := json.Marshal(result)
	if err == nil {
		return payload, false
	}

	errJSON, emErr := json.Marshal(map[string]string{mapKeyError: err.Error()})
	if emErr != nil {
		return []byte(`{"error":"marshal error failed"}`), true
	}

	return errJSON, true
}

// SetToolExecutor injects the real tool executor (Phase-4 TOOL-04/05 — a
// catalog-backed toolexec.RealExecutor constructed at startup in Plan 04-05).
// When not called, the session uses stubToolResult for every non-subagent tool
// (the Phase-2 backward-compatible behavior — real execution is opt-in).
func (s *Session) SetToolExecutor(tx toolcat.ToolExecutor) { s.toolExec = tx }

// SetTurnModel swaps the model the session's requests are shaped with (16-05,
// ACP-08 live apply): the Shaper reads Model from the session profile at every
// Stream, so the NEXT provider request carries the new model. Serialization
// contract: the CALLER holds the session's turn serialization (the runtime's
// per-session turn mutex), so the swap lands strictly BETWEEN turns and can
// never tear a request the provider is mid-shaping — the same discipline the
// SubagentModel per-dispatch-copy precedent established for subagents.
func (s *Session) SetTurnModel(model string) { s.Profile.Model = model }

// Close ends the session: it runs the OnClose hook exactly once (idempotent) so
// MCP subprocesses are reaped, transcript managers flushed, etc. (Plan 05-01
// T4). Safe to call multiple times; concurrent calls are serialized. A
// mid-session turn panic does NOT call Close (the session survives for the next
// turn); Close is session-end only. Any armed ask timer is disarmed first
// (12-01: no goroutine leak; a pending ask dies with its session).
func (s *Session) Close() error {
	var firstErr error

	s.closeOnce.Do(func() {
		if s.ask != nil {
			s.ask.Disarm()
		}

		// 12-02 Task 4: SessionEnd fires at session close (bounded by the
		// per-hook timeout — Close carries no caller ctx by design; the
		// per-hook timeout IS the bound).
		if s.Hooks != nil {
			_ = s.Hooks.Fire(context.Background(), "SessionEnd", nil)
		}

		if s.OnClose != nil {
			firstErr = s.OnClose()
		}
	})

	return firstErr
}

// injectHookContext appends a context-bearing hook's captured stdout to the
// SESSION profile's System blocks (12-02 Task 4 — the documented context
// role). Copy-on-append: the session's own System slice grows; a shared
// backing array is never mutated.
func (s *Session) injectHookContext(out ecosys.HookOutcome) {
	if out.Message == "" {
		return
	}

	s.Profile.System = append(s.Profile.System, profile.TextBlock{
		Type: blockText, Text: out.Message,
	})
}

// runTurn is the D-18 model/tool loop core, shared by Prompt and the ask-resume
// path (the reply or the D-01 timer re-enters here with the SUSPENDED turn's
// id — no new user message; the resumed projection carries the answered tool
// result alongside its original tool_use batch). Each entry gets its own
// maxIterations budget: a suspension ENDS a client-visible turn (the client
// controls the next prompt; T-12-01-03), so the runaway bound is per-entry.
//
//nolint:gocognit,gocyclo,cyclop,funlen,nonamedreturns,maintidx // domain complexity; err used by defer
func (s *Session) runTurn(ctx context.Context, turnID string) (stop string, err error) {
	defer func() {
		if r := recover(); r != nil {
			//nolint:err113 // dynamic error message
			s.appendError(turnID, "session", fmt.Errorf("turn panic: %v", r), false)
			//nolint:err113 // dynamic error message
			err = fmt.Errorf("session turn panic recovered: %v", r)
			stop = ""
		}
	}()

	// maxIterations bounds the inner tool loop. 64 (raised from the stub-era
	// 16 in Phase 8): a REAL model explore/apply turn routinely makes dozens of
	// tool calls — the real-/opsx E2E gate burned through 16 in ~2 minutes
	// (08-06 T2 finding). Still a runaway bound, not a license.
	const maxIterations = 64 // bound the tool loop (runaway guard)
	for range maxIterations {
		err = ctx.Err()
		if err != nil {
			s.recordCanceled(turnID, "context cancelled before turn step")

			return stopCancelled, nil
		}
		// Step 1: project the lean window (D-01/D-02).
		messages, err := s.Projector.Project(turnID)
		if err != nil {
			s.appendError(turnID, "projector", err, false)

			return "", fmt.Errorf("session turn project: %w", err)
		}
		// Step 2+3: shape + Stream. The provider's RequestCapturer publishes
		// RequestShaped to the bus (LOG-01); the async TranscriptWriter appends
		// request_shaped. The semaphore bounds outbound concurrency (PARA-04).
		// Plan 02-05 replaced Send with Stream: chunks are read from the stream
		// channel, emitted to the bus as AgentMessageChunk/ToolCall, and assembled
		// into the final Response (ACP-04 — NO full-turn buffering).
		resp, textBuf, streamErr := s.withSemaphore(ctx, func() (provider.Response, string, error) {
			return s.streamAndEmit(ctx, turnID, messages)
		})
		if streamErr != nil {
			if ctx.Err() != nil {
				s.recordCanceled(turnID, "context cancelled during stream")

				return stopCancelled, nil
			}

			s.appendError(turnID, "provider", streamErr, false)

			return "", fmt.Errorf("session turn stream: %w", streamErr)
		}
		// Step 5: tool_calls → execute + boundary + loop. Task/Agent tool calls
		// dispatch an isolated goroutine subagent (PARA-01) BEFORE the batch
		// (they are NOT routed through DispatchBatch). The remaining calls are
		// dispatched in one batch via toolexec.DispatchBatch (Phase-4 TOOL-04:
		// read-only concurrent, mutating strictly serial, arrival-order results).
		// A nil toolExec preserves the Phase-2 stub behavior (backward-compat).
		if len(resp.ToolCalls) > 0 { //nolint:nestif // tool-call processing is inherently nested
			// Record every model-selected tool_call first (the audit log shows
			// what the model asked for, independent of how it was executed).
			// The REAL provider id (T1) is the pairing key for tool_result —
			// recording the tool NAME here was the 08-06 blocker's root cause 2.
			for _, tc := range resp.ToolCalls {
				_ = s.Manager.AppendToolCall(turnID, toolCallIDOf(tc), tc.Name, tc.Input)
			}
			// Subagent dispatch (Task/Agent) runs inline + is excluded from the
			// batch — it is a nested turn, not a catalog tool execution.
			var batchCalls []provider.ToolCall

			batchIDs := make([]string, 0, len(resp.ToolCalls))

			// 17-02: gated ask-class calls suspended this iteration — the
			// suspensions are applied AFTER the batch's survivors dispatch and
			// record (the same shape as the ask suspension below). A SLICE, not
			// a single slot (17-REVIEW CR-01): a multi-call batch can suspend
			// MORE THAN ONE gated ask (e.g. Write + Bash in one response), and
			// a single pointer silently dropped every suspension but the last
			// — the dropped call kept its recorded tool_call line but never got
			// an ask marker, a queue entry, or a tool result (the unpaired
			// tool_use that breaks the next provider request).
			var gateSuspensions []pendingPermission

			for _, tc := range resp.ToolCalls {
				callID := toolCallIDOf(tc)
				if isSubagentTool(tc.Name) {
					// 17-02 THE permission chokepoint — subagent branch (D-05:
					// ONE pipeline; subagents bypass DispatchBatch but NEVER
					// the gate — no second permission path).
					if v := s.gateCall(turnID, callID, tc.Name, tc.Input); v.action != gateExecute {
						if v.action == gateSuspend {
							gateSuspensions = append(gateSuspensions,
								pendingPermission{callID: callID, tool: tc.Name, input: tc.Input})

							continue
						}

						s.appendToolResultLoud(turnID, callID, tc.Name, v.result, true)

						continue
					}

					// 12-02: a subagent_type matching a discovered agent
					// definition dispatches with the definition's Prompt (a
					// per-dispatch system block) and Tools (the restricted
					// set); an unknown type keeps the defaults (advisory
					// listing — graceful degradation, no confirmation tier).
					var agentDef *ecosys.Agent
					if def, ok := s.agentDefFor(tc.Input); ok {
						agentDef = &def
					}

					result, derr := s.DispatchSubagent(ctx, turnID, tc.Name, extractSubagentPrompt(tc.Input), agentDef)
					if derr != nil {
						errJSON, mErr := json.Marshal(map[string]string{mapKeyError: derr.Error()})
						if mErr != nil {
							errJSON = []byte(`{"error":"marshal error failed"}`)
						}

						s.appendToolResultLoud(turnID, callID, tc.Name, errJSON, true)
					} else {
						// 14-05 (EARLY-05) + CR-02: the subagent Task result
						// flows through the truncation chokepoint like every
						// other tool Output, encoded as PROPER JSON first —
						// the old naive quote-concat produced INVALID JSON for
						// any result needing escapes (every multi-line
						// report), which both bypassed the cap and failed
						// appendLine's Marshal (the tool_result line was
						// dropped entirely).
						payload, isErr := subagentResultPayload(result)
						s.appendToolResultLoud(turnID, callID, tc.Name, boundedToolResult(payload), isErr)
					}
					// SESS-02/03 boundary (subagent tools are read-only; only
					// a config-added entry would fire). Same between-turn rule
					// as the batch site: the line resets the NEXT turn's
					// projection, never this turn's mid-turn window (SESS-04
					// revised 2026-08-15; 08-09 / 08-08 T4 / corpus 4440f5a7).
					_ = s.MaybeAppendBoundary(tc.Name, callID, turnID)

					continue
				}

				// 12-04 plan-mode gate (CAPTURED, 12-05 re-record): while
				// plan mode is ON, gated calls (mutating + the captured
				// extra-refused set) return the captured refusal WITHOUT
				// executing — the target's runtime-level enforcement,
				// mirrored; read-only exploration continues.
				if s.planModeBlocks(tc.Name) {
					s.appendToolResultLoud(turnID, callID, tc.Name, planModeRefusal(), true)

					continue
				}

				// 17-02 THE permission chokepoint — beside the plan-mode gate
				// (D-04/D-05): rule evaluation runs in BOTH modes (deny still
				// denies ungated); the ask class opens the native dialog only
				// when gated + human, declines fail-safe on automation turns.
				if v := s.gateCall(turnID, callID, tc.Name, tc.Input); v.action != gateExecute {
					if v.action == gateSuspend {
						gateSuspensions = append(gateSuspensions,
							pendingPermission{callID: callID, tool: tc.Name, input: tc.Input})

						continue
					}

					s.appendToolResultLoud(turnID, callID, tc.Name, v.result, true)

					continue
				}

				batchCalls = append(batchCalls, tc)
				batchIDs = append(batchIDs, callID)
			}
			// Dispatch the remaining (non-subagent) calls in one batch. Results
			// are in arrival order so the transcript stays deterministic.
			results, batchErr := toolexec.DispatchBatch(ctx, s.toolExecOrStub(), s.Catalog, batchCalls)
			if batchErr != nil {
				// DispatchBatch surfaces per-call errors in results; a non-nil
				// top-level error is a cancelled-ctx path — record + continue so
				// the loop re-projects with whatever partial results we have.
				s.appendError(turnID, "toolexec", batchErr, true)
			}

			var (
				suspendedCallID string
				suspendedOutput json.RawMessage
				suspendedTool   string
			)

			for _, res := range results {
				// Key the result by the SAME call id the tool_call line
				// recorded (pairing is consistent end-to-end, 08-07).
				callID := res.Name
				if res.CallIndex >= 0 && res.CallIndex < len(batchIDs) {
					callID = batchIDs[res.CallIndex]
				}

				// 12-01 ask suspension: the AskUserQuestion executor returns
				// ErrSuspended after parsing — NO tool result is appended for
				// the suspended call (the reply or the D-01 timer appends it);
				// the turn ends with the ask marker after the batch's other
				// results are recorded.
				if res.Err != nil && errors.Is(res.Err, ErrSuspended) {
					suspendedCallID = callID
					suspendedOutput = res.Output
					suspendedTool = res.Name

					continue
				}

				// 14-05 (EARLY-05): the append-boundary truncation chokepoint —
				// over-cap outputs are bounded HERE, before the transcript line
				// exists (hence before both the mid-turn model view and the
				// Projector's projected window see them); under-cap payloads are
				// byte-unmodified.
				s.appendToolResultLoud(turnID, callID, res.Name, boundedToolResult(res.Output), res.IsError)
				// SESS-02/03: a mutating/config-added tool is a boundary. The
				// line is the audit marker + the reset point for the NEXT
				// turn's projection — the producing turn's mid-turn window
				// survives it (SESS-04 revised 2026-08-15, between turns;
				// capture: corpus session 4440f5a7, 46/46 rolling-64 tail
				// records, zero tool-result resets — 08-09 / 08-08 T4).
				_ = s.MaybeAppendBoundary(res.Name, callID, turnID)
				// 12-04: a successful EnterPlanMode flips the state ON and
				// records the enter marker (an audit line, never a boundary).
				if res.Name == toolNameEnterPlanMode && !res.IsError && s.planMode != nil {
					s.planMode.Enter()
					s.appendPlanModeMarker(planModeCauseEnter, callID, turnID)
				}
			}

			// 17-02: gated ask-class call(s) suspended — the batch's survivors
			// (if any) were dispatched + recorded above. EVERY suspension is
			// applied in batch order (CR-01); the queue serializes their firing
			// (one outstanding — D-11) and each dialog answer drives its own
			// resume of the SAME turn. The turn ends with the ask marker: no
			// turn lock is held across the human wait (RESEARCH Pitfall 2).
			for i := range gateSuspensions {
				s.suspendForPermission(turnID,
					gateSuspensions[i].callID, gateSuspensions[i].tool, gateSuspensions[i].input)
			}

			if len(gateSuspensions) > 0 {
				return stopAsk, nil
			}

			if suspendedCallID != "" {
				if s.ask != nil {
					s.suspendForAsk(turnID, suspendedCallID, suspendedOutput, suspendedTool)

					return stopAsk, nil
				}
				// No broker wired (a bare session): degrade to the structured
				// error convention — the model is never silently dead-ended.
				errJSON, mErr := json.Marshal(map[string]string{
					mapKeyError: "ask suspended but no broker is wired (question dropped)",
				})
				if mErr != nil {
					errJSON = []byte(`{"error":"marshal error failed"}`)
				}

				s.appendToolResultLoud(turnID, suspendedCallID, suspendedTool, errJSON, true)

				continue
			}

			continue // loop to project again with the results
		}
		// 16-REVIEW WR-04: re-check BEFORE the end_turn bookkeeping. The
		// streamAndEmit cancellation checks only fire while chunks are still
		// moving — a ctx dying between the last delivered chunk and here
		// previously fell through to end_turn: the partial assistant message was
		// appended, the Stop hook fired for a turn the user cancelled, and the
		// response claimed end_turn (the D-16 "cancelled" contract broken).
		if ctx.Err() != nil {
			s.recordCanceled(turnID, "context cancelled before turn end")

			return stopCancelled, nil
		}

		// Step 6: end_turn — append the assembled assistant message + stopReason.
		assistantText := textBuf
		_ = s.Manager.AppendAssistantMessage(turnID, assistantText)

		// 12-02 Task 4: Stop fires at PARENT turn end (after the assistant
		// message; stop_hook_active=false — ass-guard never loops Stop).
		if s.Hooks != nil {
			_ = s.Hooks.Fire(ctx, "Stop", map[string]any{"stop_hook_active": false})
		}

		return mapStopReason(resp.FinishReason), nil
	}

	s.appendError(turnID, "session", errToolLoopExceededMax, true)

	return "", errToolLoopExceeded
}

// The plan-mode tool names (12-04; the loop matches them for state/marker
// transitions — the executors render forms, the loop owns identity + state).
const (
	toolNameEnterPlanMode = "EnterPlanMode"
	toolNameExitPlanMode  = "ExitPlanMode"
)

// suspendForAsk records the suspension in the transcript + surfaces the
// question through the broker (which fires the client-visible surface callback
// and arms the D-01 timer). The executor's parsed questions ride the suspended
// result's Output (the Stub seam carries no call identity; this is the one
// place that knows both turnID and callID — T-12-01-01's keying).
func (s *Session) suspendForAsk(turnID, callID string, output json.RawMessage, toolName string) {
	var qs []AskQuestion

	_ = json.Unmarshal(output, &qs)

	kind := PendingAskKindQuestion
	if toolName == toolNameExitPlanMode {
		kind = PendingAskKindPlanApproval
	}

	qJSON, mErr := json.Marshal(qs)
	if mErr != nil {
		qJSON = output
	}

	_ = s.Manager.AppendAskSuspended(turnID, callID, qJSON)
	s.ask.Surface(PendingAsk{TurnID: turnID, CallID: callID, Questions: qs, Kind: kind})
}

// stubExecutor returns the Phase-2 canned stub for every tool (D-15 — execution
// stays stubbed until SetToolExecutor wires the RealExecutor). It is used by
// toolExecOrStub when no real executor is set.
type stubExecutor struct{}

func (stubExecutor) Execute(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return stubToolResult, nil
}

// toolExecOrStub returns the real tool executor if set, else a stub executor
// that returns stubToolResult (Phase-2 backward-compat). DispatchBatch consumes
// this; a nil never reaches it.
func (s *Session) toolExecOrStub() toolcat.ToolExecutor { //nolint:ireturn // ToolExecutor abstraction
	if s.toolExec != nil {
		return s.toolExec
	}

	return stubExecutor{}
}

// executeStub is retained for Phase-2 callers/tests that drive one tool call
// withSemaphore bounds fn with the session's outbound-concurrency semaphore and
// releases the slot on EVERY exit path — including a panic unwinding from fn
// (16-REVIEW WR-02: the recover guards live at the runTurn/Prompt deferred
// level, so a panic between Acquire and a straight-line Release previously
// leaked the slot; after maxConc leaks every future turn blocked forever in
// Acquire with no diagnostic). A cancelled Acquire consumes no slot and returns
// the wrapped ctx error, which the caller routes to the canceled-transcript /
// stopCancelled path like any other cancelled stream.
func (s *Session) withSemaphore(
	ctx context.Context, fn func() (provider.Response, string, error),
) (provider.Response, string, error) {
	if s.Semaphore == nil {
		return fn()
	}

	aerr := s.Semaphore.Acquire(ctx)
	if aerr != nil {
		return provider.Response{}, "", fmt.Errorf("semaphore acquire: %w", aerr)
	}

	defer s.Semaphore.Release()

	return fn()
}

// streamAndEmit opens Provider.Stream, reads chunks until the channel closes,
// and emits each to the bus as AgentMessageChunk / ToolCall / UsageUpdate (D-18
// step 4 — ACP-04 streaming, NO full-turn buffering). It returns the assembled
// Response (tool_calls + FinishReason) + the concatenated assistant text. ctx
// cancellation closes the stream (the provider aborts the in-flight request).
//
//nolint:cyclop,funlen // one switch over the full chunk-type vocabulary; the WR-04 ctx gate adds two checks
func (s *Session) streamAndEmit(
	ctx context.Context, turnID string, messages []provider.Message,
) (provider.Response, string, error) {
	ch, err := s.Provider.Stream(ctx, &s.Profile, messages)
	if err != nil {
		return provider.Response{}, "", fmt.Errorf("call: %w", err)
	}

	var (
		resp provider.Response
		sb   strings.Builder
	)

	for chunk := range ch {
		err := ctx.Err()
		if err != nil {
			// 16-REVIEW WR-04: return the ctx error HONESTLY — `(partial, nil)`
			// skipped runTurn's cancelled branch (end_turn + Stop hook fired for
			// a user-cancelled turn; D-16 broken). The caller maps this error to
			// recordCanceled + stopCancelled via the streamErr + ctx.Err() path.
			return resp, sb.String(), err
		}

		switch chunk.Type {
		case blockText:
			sb.WriteString(chunk.Text)

			if s.Bus != nil && chunk.Text != "" {
				s.Bus.Publish(event.AgentMessageChunk{
					TurnID: turnID, MessageID: turnID, Content: chunk.Text,
				})
			}
		case blockToolUse:
			if chunk.ToolCall != nil {
				tc := *chunk.ToolCall

				resp.ToolCalls = append(resp.ToolCalls, tc)
				if s.Bus != nil {
					s.Bus.Publish(event.ToolCall{
						TurnID: turnID, ToolCallID: chunk.ToolCallID,
						Name: tc.Name, Input: tc.Input,
					})
				}
			}
		case "usage":
			if chunk.Usage != nil && s.Bus != nil {
				s.Bus.Publish(event.UsageUpdate{
					TurnID:      turnID,
					InputTokens: chunk.Usage.InputTokens, OutputTokens: chunk.Usage.OutputTokens,
				})
			}
		case stopDone:
			resp.FinishReason = chunk.FinishReason
			resp.Raw = chunk.Raw
		case chunkErrorType:
			// Mid-stream abort (the SSE idle watchdog — 08-09 finding): surface
			// the retryable error; the turn records it and returns instead of
			// fabricating a completed response from a truncated body.
			if chunk.Error != nil {
				return resp, sb.String(), chunk.Error
			}
		}
	}
	// If ctx was cancelled mid-stream, surface that so the caller records a
	// canceled line + returns "cancelled" (D-16). The provider's goroutine has
	// already closed the channel (the HTTP request was aborted by ctx).
	err = ctx.Err()
	if err != nil {
		return resp, sb.String(), err
	}

	return resp, sb.String(), nil
}

// recordCanceled appends a canceled line (D-16).
func (s *Session) recordCanceled(turnID, reason string) {
	_ = s.Manager.AppendCanceled(turnID, now(), reason)
}

// appendToolResultLoud is the G-12-3b loudness gate (12-10): EVERY
// AppendToolResult call in the session package routes through here. On nil
// error it behaves exactly as the bare call did. On error (appendLine's
// json.Marshal(Line) failed — an invalid-JSON Output payload), the loss is
// LOUD: a stderr warning names turn/call-id/tool + the cause, and a FALLBACK
// {"error": …} payload line is appended keyed by the SAME call id, so the
// model always sees SOMETHING for every executed call instead of flying blind
// with unpaired tool_use blocks accumulating per round-trip. If even the
// fallback Marshal fails, the hardcoded literal byte form is used (the same
// guard pattern as the dispatch-error sites). Manager.appendLine itself is
// untouched — redaction ordering stays as shipped.
func (s *Session) appendToolResultLoud(turnID, callID, toolName string, output json.RawMessage, isError bool) {
	err := s.Manager.AppendToolResult(turnID, callID, output, isError)
	if err == nil {
		return
	}

	slog.Warn("session: tool result could not be recorded; appending fallback",
		"turnID", turnID, "callID", callID, "tool", toolName, "error", err.Error())

	fallback, mErr := json.Marshal(map[string]string{
		mapKeyError: "tool result could not be recorded: " + err.Error(),
	})
	if mErr != nil {
		fallback = []byte(`{"error":"tool result could not be recorded"}`)
	}

	_ = s.Manager.AppendToolResult(turnID, callID, fallback, true)
}

// appendError is the investigate-and-fix-ready error writer (PROJECT.md). The
// message is scrubbed by the Manager; the stack is captured here.
func (s *Session) appendError(turnID, component string, err error, recoverable bool) {
	if err == nil {
		return
	}

	_ = s.Manager.AppendError(turnID, component, err.Error(), nil, recoverable, string(debug.Stack()))
}

// mapStopReason maps the provider's FinishReason to an ACP stopReason. Unknown
// reasons default to "end_turn".
func mapStopReason(finish string) string {
	switch strings.ToLower(finish) {
	case stopEndTurn, "stop":
		return stopEndTurn
	case blockToolUse:
		return blockToolUse
	case "max_tokens":
		return "max_tokens"
	case "":
		return stopEndTurn
	default:
		return finish
	}
}
