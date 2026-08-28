// Package enginebridge holds the engine's adapter types (D-12/D-14): the
// seven small adapters that bridge the Phase-4 engine + hook-DAG seams to the
// Session Core. The types live here; their CONSTRUCTION stays in package
// runtime (setupEngine, runOneTurn, sessionFor), which feeds runner-derived
// behavior across as func values through BridgeConfig — behavior crosses the
// package boundary, unexported names never do.
package enginebridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync"

	"github.com/Djarvur/ass-guard-agent/internal/engine"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/hookdag"
	"github.com/Djarvur/ass-guard-agent/internal/learning"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// BridgeConfig is the mirror-config seam (D-14): the runner members the
// bridge adapters need, delivered as plain values + func values by the
// runtime-core construction sites. Filled from ONE source per serve
// (acpserve.Run → runtime core); divergence risk is accepted deliberately
// per D-14 (operator choice).
type BridgeConfig struct {
	// Hooks/HookCfg/Learned/Bus mirror the runner's engine-wiring fields
	// (setupEngine's products).
	Hooks   *hookdag.Executor
	HookCfg []hookdag.Hook
	Learned *learning.Store
	Bus     *event.Bus

	// NextPromptFor answers "what does the engine inject when this pattern
	// matches" (08-06 chaining) — the pattern-table capability delivered as a
	// func value so the table stays the single source of truth (the runtime
	// core resolves it dynamically; the dispatcher follows table swaps).
	NextPromptFor func(patternID string) string

	// Expand applies slash-command expansion to a prompt (expandUserBlocks
	// delivered as a func value; nil-safe in the adapter).
	Expand func(sess *session.Session, blocks []session.ContentBlock) []session.ContentBlock

	// Invoke resolves a prompt's first text block as a registry command
	// invocation (invocationFor delivered as a func value; the adapter's
	// expansion path is disabled without it).
	Invoke func(blocks []session.ContentBlock) (key, args string, ok bool)

	// AutomationProvenance reads the sessMu-guarded automation provenance
	// (the firing path's exclusive vocabulary — T-12-07-03).
	AutomationProvenance func() string
}

// stubExecResult is the canned tool result for the non-engine path (mirrors
// session.stubResultMsg). Used by StubCatalogExec so MCPExecutor has a working
// inner executor when the engine is disabled (--no-engine).
const stubExecResult = `{"output":"stubbed (engine disabled)"}`

// StubCatalogExec is the ToolExecutor for the non-engine path: it returns the
// canned stub result for every non-mcp tool (mirrors session.stubExecutor). MCP
// calls never reach it (MCPExecutor intercepts them first).
type StubCatalogExec struct{}

// NewStubCatalogExec constructs the non-engine-path ToolExecutor.
func NewStubCatalogExec() StubCatalogExec { return StubCatalogExec{} }

// Execute returns the canned stub result for every non-mcp tool.
func (StubCatalogExec) Execute(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(stubExecResult), nil
}

// stopAskACP mirrors session's ask stop marker (kept local: internal/session
// owns the vocabulary; the bridge layer only needs the one mapping — the
// runtime core keeps its own copy, per the D-03 duplication discipline).
const stopAskACP = "ask"

// EngineTurnAdapter adapts the Session Core to the engine.TurnRunner seam
// (Plan 04-05 D-01). Run delegates to sess.Prompt (a real turn) after applying
// slash-command expansion (08-04: the engine's continue-injections re-enter
// here, so an injected "/opsx:propose …" gets identical expansion + provenance
// + boundary treatment as a user-typed command — and the USER prompt also
// arrives raw here, because Runner.Run defers engine-path expansion to this
// adapter); LastTurnOutput reads the transcript via Manager.ReadAll to
// extract the most-recent assistant_message text + the turn's tool-call names.
type EngineTurnAdapter struct {
	sess *session.Session
	mgr  *session.Manager
	cfg  BridgeConfig // the runner-derived func values (expansion owner)

	// startedBy is the registry command key whose invocation the LAST Run call
	// expanded (hybrid chaining, findings-6 disposition — TurnOutput.StartedBy).
	// Set from the RAW prompt inside Run, before expansion; a plain-text turn
	// leaves it "". Single-threaded by construction: the engine's Observe loop
	// calls Run and LastTurnOutput sequentially.
	startedBy string

	// subject is the FIRST turn's typed invocation arguments ("add-login" for
	// "/opsx:explore add-login") — the scenario subject. A later BARE injected
	// command ("/opsx:propose" with no args) inherits it, mirroring the
	// captured operator behavior (every stage invocation named the change; a
	// bare propose would ask for a subject and the chain would die — live
	// evidence: the first hybrid-chained E2E run). Sourced from the USER-side
	// typed prompt only; empty when the first prompt was not an invocation
	// with arguments.
	subject   string
	seenFirst bool

	// 13-00 park plumbing (all mutated ONLY on the engine's Observe
	// goroutine — single-threaded by construction, see above):
	//
	// lastStop records the stop the LAST sess.Prompt returned; OnSuspended
	// fires once when the engine first consults AskSettle after an ask stop
	// (i.e. AFTER the first-turn ask decision has been emitted — the park
	// signal therefore implies the audit line is already on disk); ParkMu,
	// armed by that signal, is the session's turn mutex — every post-park
	// Run (the settled chain's injections) holds it around sess.Prompt so
	// server-driven injections serialize with client turns exactly like the
	// cron/automation precedent.
	lastStop     string
	suspSignaled bool

	// OnSuspended is armed by the runtime core's runOneTurn after
	// construction (the park signal).
	OnSuspended func()

	// ParkMu is set by the OnSuspended signal (the session's turn mutex).
	ParkMu *sync.Mutex
}

// NewEngineTurnAdapter constructs the adapter over the active session, fed
// the runner-derived func values from cfg (the construction site stays in
// package runtime — runOneTurn).
func NewEngineTurnAdapter(sess *session.Session, cfg *BridgeConfig) *EngineTurnAdapter {
	return &EngineTurnAdapter{sess: sess, mgr: sess.Manager, cfg: *cfg}
}

// Run drives one turn through the Session Core.
func (a *EngineTurnAdapter) Run(ctx context.Context, prompt []session.ContentBlock) (string, error) {
	a.startedBy = ""
	if a.cfg.Invoke != nil {
		key, args, ok := a.cfg.Invoke(prompt)

		// Resolve the starting command from the RAW prompt (the same lookup
		// expandUserBlocks acts on) BEFORE expansion — an already-expanded body
		// carries no invocation. Prompt-side only: assistant/tool content can
		// never set this.
		a.startedBy = key

		// 12-07 vocabulary extension: an automation-fired turn carries the
		// automation's provenance INSTEAD of a command key (set exclusively by
		// the runner-side firing path — model content can never reach it,
		// T-12-07-03; no behavior change for user/command turns).
		if a.cfg.AutomationProvenance != nil {
			if prov := a.cfg.AutomationProvenance(); prov != "" {
				key = prov
			}
		}

		if !a.seenFirst {
			a.seenFirst = true
			a.subject = args
		} else if ok && key != "" && args == "" && a.subject != "" {
			// Scenario-subject forwarding: a BARE injected command inherits the
			// first invocation's arguments (an injection carrying its own args
			// is left alone). Rebuild the invocation text with the subject and
			// re-resolve so the expansion below sees it.
			idx := firstTextBlockIndex(prompt)
			withSubject := append([]session.ContentBlock(nil), prompt...)
			withSubject[idx] = session.ContentBlock{
				Type: blockText, Text: "/" + key + " " + a.subject,
			}
			prompt = withSubject
		}

		prompt = a.cfg.Expand(a.sess, prompt)
	}

	if a.ParkMu != nil {
		// 13-00: a post-park injection — a server-driven turn on the parked
		// chain. Hold the session's turn mutex for the WHOLE turn (the cron
		// firing discipline): client turns and replies queue against it,
		// never overlap.
		a.ParkMu.Lock()
		defer a.ParkMu.Unlock()
	}

	stop, err := a.sess.Prompt(ctx, prompt)
	a.lastStop = stop

	return stop, err //nolint:wrapcheck // session delegation
}

// AskSettle implements engine.AskSettler (13-00): the engine's ask-wait
// consumes the session's per-suspension settle channel (closed after the
// resumed turn completes — whichever driver resumed it). The FIRST
// consultation after an ask stop also fires the park signal — at that point
// the first-turn ask decision has already been emitted, so the caller
// returning the prompt response at the suspension cannot race the audit
// line.
func (a *EngineTurnAdapter) AskSettle() <-chan struct{} {
	if a.lastStop == stopAskACP && !a.suspSignaled && a.OnSuspended != nil {
		a.suspSignaled = true
		a.OnSuspended()
	}

	return a.sess.AskSettleChan()
}

// LastTurnOutput reads the transcript to build the engine's view of the most
// recent turn: the last assistant_message's TurnID + Text + the tool-call names
// recorded under that TurnID (D-02 second signal). A missing assistant_message
// yields an empty TurnOutput (the engine treats it as unmatched ⇒ nothing).
//
// 12-01 (ACP-01): the turn-TERMINAL line is the last of assistant_message |
// ask_suspended — a suspended turn has NO assistant_message (it ended at the
// tool loop), so scanning only assistant lines would attribute the suspension
// to the PREVIOUS turn. An ask_suspended terminal line yields
// TurnOutput with AskSuspended set (Decide → ActionAsk, never Continue).
//
//nolint:cyclop,funlen // the backward scan is one cohesive walk
func (a *EngineTurnAdapter) LastTurnOutput() engine.TurnOutput {
	if a.mgr == nil {
		return engine.TurnOutput{}
	}

	lines, err := a.mgr.ReadAll()
	if err != nil {
		return engine.TurnOutput{}
	}

	var lastAssistant *session.Line

	var askSuspended *session.Line

	// 12-04 (ACP-02): the plan-mode state at turn end — the LAST plan_mode
	// marker's cause (enter/exit) is the state the turn ENDED in; it rides
	// TurnOutput as engine-decision provenance (signal context only).
	planModeOn := false

	// 12-09 (G-12-3): the terminal line is found FIRST scanning backward; its
	// turn's plan_mode markers sit EARLIER in the transcript, so the scan must
	// continue to the terminal line's own boundary before honoring the break.
	// The marker is always written BEFORE its turn's terminal line (the
	// tool-result site vs Step 6), so once the scan crosses INTO earlier turns
	// (a user_message of a different turn) the newest-marker read is final.
	var terminalTurn string

	// 12-09: per-turn "a tool_result landed BELOW this point" flags — a
	// suspension whose result already landed is RESOLVED and no longer
	// terminal (the reply or the D-01 timer completed it; the resumed turn's
	// own assistant_message is the real terminal).
	resolvedBelow := map[string]bool{}

	for i := len(lines) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
		switch lines[i].Type {
		case session.TypeAssistantMessage:
			if lastAssistant == nil {
				lastAssistant = &lines[i]

				terminalTurn = lines[i].TurnID
			}
		case session.TypeToolResult:
			resolvedBelow[lines[i].TurnID] = true
		case session.TypeAskSuspended:
			// Only an UNRESOLVED suspension is terminal: one whose tool_result
			// already landed below it was completed by the reply/timer resume.
			if !resolvedBelow[lines[i].TurnID] && askSuspended == nil {
				askSuspended = &lines[i]

				terminalTurn = lines[i].TurnID
			}
		case session.TypePlanMode:
			// Newest marker within/at-or-before the terminal turn wins; keep
			// reading until we cross out of that turn so the marker written
			// mid-turn is seen.
			planModeOn = lines[i].Cause == session.PlanModeCauseEnter
		}

		if terminalTurn != "" && lines[i].Type == session.TypeUserMessage && lines[i].TurnID == terminalTurn {
			break
		}
	}

	if askSuspended != nil {
		return engine.TurnOutput{
			TurnID: askSuspended.TurnID, StartedBy: a.startedBy,
			AskSuspended: true,
			ToolCalls:    toolCallNamesUnder(lines, askSuspended.TurnID),
			PlanMode:     planModeOn,
		}
	}

	if lastAssistant == nil {
		return engine.TurnOutput{}
	}

	return engine.TurnOutput{
		TurnID: lastAssistant.TurnID, Text: lastAssistant.Text,
		StartedBy: a.startedBy, ToolCalls: toolCallNamesUnder(lines, lastAssistant.TurnID),
		PlanMode: planModeOn,
	}
}

// toolCallNamesUnder collects the tool-call NAMES recorded under turnID (D-02
// second signal; shared by the assistant-terminal + ask-suspended paths).
func toolCallNamesUnder(lines []session.Line, turnID string) []string {
	var names []string

	for i := range lines {
		if lines[i].TurnID == turnID && lines[i].Type == session.TypeToolCall {
			names = append(names, lines[i].Name)
		}
	}

	return names
}

// firstTextBlockIndex returns the index of the first text-typed block, or -1.
// (D-03 duplication: the runtime core keeps its own copy beside the expansion
// seam — the two packages never share test/scan helpers.)
func firstTextBlockIndex(blocks []session.ContentBlock) int {
	for i, b := range blocks {
		if b.Type == blockText {
			return i
		}
	}

	return -1
}

// ACPDispatcher implements engine.ActionDispatcher, routing the engine's
// hook/ask actions to the real seams (Plan 04-05 T1).
//
//   - Hook maps the matched signal to a hook in the loaded config + launches the
//     hook-DAG via hookdag.Executor (the real CommandRunner/TurnRunner/
//     BoundaryOpener seams are wired lazily per session — see hookSeamsFor).
//   - Ask consults the learning store (Lookup → use the stored answer; else
//     ErrAskPending so the engine surfaces a session/update asking the user).
type ACPDispatcher struct {
	hooks   *hookdag.Executor
	hookCfg []hookdag.Hook
	learned *learning.Store
	bus     *event.Bus

	// nextPromptFor answers "what does the engine inject when this pattern
	// matches" (08-06 chaining — the pattern table's next-command field).
	nextPromptFor func(patternID string) string
}

// NewACPDispatcher constructs the dispatcher from the bridge config (the
// construction site stays in package runtime — setupEngine).
func NewACPDispatcher(cfg *BridgeConfig) *ACPDispatcher {
	return &ACPDispatcher{
		hooks:         cfg.Hooks,
		hookCfg:       cfg.HookCfg,
		learned:       cfg.Learned,
		bus:           cfg.Bus,
		nextPromptFor: cfg.NextPromptFor,
	}
}

// PopulateContinue satisfies engine.ContinuePopulator (08-06): a continue
// decision with an empty NextPrompt gets the matched pattern's next /opsx:*
// command — the injection then flows through the 08-04 expansion seam
// (expand + provenance + boundary) like a typed command. Handles all three
// signal vocabularies: text:/tool: (the dual signals) and command: (hybrid
// chaining — the provenance rows share the same next field).
func (d *ACPDispatcher) PopulateContinue(dec *engine.Decision) {
	if d.nextPromptFor == nil {
		return
	}

	id := dec.Signal
	for _, p := range []string{"text:", "tool:", "command:"} {
		if len(id) > len(p) && id[:len(p)] == p {
			id = id[len(p):]
		}
	}

	if next := d.nextPromptFor(id); next != "" {
		dec.NextPrompt = []session.ContentBlock{{Type: blockText, Text: next}}
	}
}

// Hook launches the hook-DAG matching the trigger derived from the signal. The
// dispatcher carries the PER-TURN hook executor whose seams
// (CommandRunner/TurnRunner/BoundaryOpener) are bound to the active session at
// construction (runOneTurn — 16-REVIEW WR-01: per-invocation wiring, the shared
// runner templates are never rebound), so Hook just selects the matching hook +
// executes it. The signal shape is "hook:<id>"; v1 maps
// proposal-ready/changes-proposed to post-phase, everything else to
// post-implement.
func (d *ACPDispatcher) Hook(ctx context.Context, hookSignal, sourceTurnID string) string {
	if d.hooks == nil || len(d.hookCfg) == 0 {
		return "no-hooks-configured"
	}

	trigger := triggerFromSignal(hookSignal)
	for _, h := range d.hookCfg {
		if h.Trigger != trigger {
			continue
		}

		prov := hookdag.Provenance{HookName: h.Name, TriggerStage: trigger, SourceTurnID: sourceTurnID}
		res := d.hooks.Execute(ctx, &h, prov)

		return res.Status
	}

	return "no-matching-hook:" + trigger
}

// Ask consults the learning store. A stored answer (active/candidate) is
// returned; otherwise ErrAskPending surfaces (the engine emits an ask
// EngineDecision + the ACP adapter surfaces it as a session/update).
func (d *ACPDispatcher) Ask(_ context.Context, situation string) (string, error) {
	if d.learned == nil {
		return "", engine.ErrAskPending
	}

	if e, ok := d.learned.Lookup(situation); ok {
		return e.Answer, nil
	}

	return "", engine.ErrAskPending
}

// Hook-stage vocabulary (08-06): stage-bearing pattern ids select their
// own hook stage; un-staged ids keep the v1.0 default.
const (
	stagePostImplement = "post-implement"
	stagePostExplore   = "post-explore"
	stagePostPropose   = "post-propose"
)

// triggerFromSignal extracts the trigger stage from an engine signal string.
// The signal shape is "hook:<id>" (the OpenSpec pattern id), "text:<id>", or
// "command:<id>" (hybrid chaining — the provenance rows carry the same
// stage-bearing ids); v1 defaults to "post-implement" unless the id carries an
// explicit stage.
func triggerFromSignal(hookSignal string) string {
	// Strip a "hook:" / "text:" / "command:" prefix.
	id := hookSignal
	for _, p := range []string{"hook:", "text:", "command:"} {
		if len(id) > len(p) && id[:len(p)] == p {
			id = id[len(p):]
		}
	}

	if id == "changes-proposed" || id == "proposal-ready" {
		return "post-phase"
	}

	// 08-06 stage vocabulary: stage-bearing pattern ids ("post-<stage>-...")
	// select their own hook stage; un-staged ids keep the v1.0 default.
	for _, stage := range []string{stagePostExplore, stagePostPropose, "post-apply", "post-archive"} {
		if strings.HasPrefix(id, stage) {
			return stage
		}
	}

	return stagePostImplement
}

// HookSessionTurnRunner adapts the active Session to the hookdag.TurnRunner seam
// (send-prompt IS a turn — HOOK-05). It converts hookdag.ContentBlock to
// session.ContentBlock + delegates to sess.Prompt.
type HookSessionTurnRunner struct {
	sess *session.Session
}

// NewHookSessionTurnRunner constructs the hookdag.TurnRunner adapter over the
// active session (the construction site stays in package runtime —
// runOneTurn).
func NewHookSessionTurnRunner(sess *session.Session) *HookSessionTurnRunner {
	return &HookSessionTurnRunner{sess: sess}
}

// Run drives one hook-context turn through sess.Prompt (content-block
// conversion only — no expansion, no engine).
func (h *HookSessionTurnRunner) Run(ctx context.Context, prompt []hookdag.ContentBlock) (string, error) {
	blocks := make([]session.ContentBlock, len(prompt))
	for i, b := range prompt {
		blocks[i] = session.ContentBlock{Type: b.Type, Text: b.Text}
	}

	return h.sess.Prompt(ctx, blocks) //nolint:wrapcheck // session delegation
}

// HookSessionBoundaryOpener adapts the active Manager to the hookdag
// BoundaryOpener seam (fresh-context IS a boundary — HOOK-05).
type HookSessionBoundaryOpener struct {
	mgr    *session.Manager
	turnID string
}

// NewHookSessionBoundaryOpener constructs the hookdag.BoundaryOpener adapter
// over the active session's Manager (the construction site stays in package
// runtime — runOneTurn).
func NewHookSessionBoundaryOpener(mgr *session.Manager) *HookSessionBoundaryOpener {
	return &HookSessionBoundaryOpener{mgr: mgr}
}

// OpenBoundary records the fresh-context boundary on the transcript
// (HOOK-05).
func (h *HookSessionBoundaryOpener) OpenBoundary(_ context.Context, cause string) error {
	if h.mgr == nil {
		return nil
	}

	return h.mgr.AppendBoundary(cause, "", h.turnID) //nolint:wrapcheck // manager delegation
}

// RealCommandRunner is the hookdag.CommandRunner seam: exec a shell command,
// capture stdout/stderr, return the exit code (D-08 exit-code contract).
type RealCommandRunner struct{}

// NewRealCommandRunner constructs the hookdag.CommandRunner seam.
func NewRealCommandRunner() RealCommandRunner { return RealCommandRunner{} }

// Run execs the command and returns stdout, stderr, and the exit code
// (D-08 exit-code contract).
//
//nolint:gocritic // conflicts w/ nonamedreturns
func (RealCommandRunner) Run(ctx context.Context, command string, args []string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0

	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			return stdout.String(), stderr.String(), 0, err
		}
	}

	return stdout.String(), stderr.String(), code, nil
}
