package coreexec

import (
	"context"
	"encoding/json"

	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// Plan-mode execution (12-04, ACP-02): the plan pair rides the interactive-tool
// executor class. EnterPlanMode is a pure form render (the CAPTURED guidance
// text — 43 observations, 12-05 re-record); the session tool loop owns the
// state flip + the plan_mode_enter marker (the identity-binding rule from
// 12-01: the loop is the one place that knows turnID + callID). ExitPlanMode
// surfaces the plan as an approval question on the 12-01 AskBroker seam and
// suspends; the reply (or the D-01 timeout) lands the approved / captured-
// denial / non-answer form as the tool result and resumes the SAME turn (the
// session resume path also flips the state + writes the exit marker).
//
// SCOPE NOTE (resolved by the 12-05 capture): the TARGET enforces plan mode as
// a runtime-level mutating-tool gate — internal/session/planmode.go mirrors it
// with the captured refusal form; this file only renders the tool surfaces.

// approveOptionIdx is the approve option's position in the surfaced approval
// question (0 — the captured options render approve first).
const approveOptionIdx = 0

// enterPlanModeToolName / exitPlanModeToolName are the captured catalog entries.
const (
	enterPlanModeToolName = "EnterPlanMode"
	exitPlanModeToolName  = "ExitPlanMode"
)

// EnterPlanModeExecute returns the EnterPlanMode catalog Stub: the CAPTURED
// entered form (session.PlanModeEnteredForm — the single definition; the
// fixture test pins it to the 12-05 re-record). The state flip is the session
// loop's (see planmode.go there).
func EnterPlanModeExecute() toolcat.Stub {
	return func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(session.PlanModeEnteredForm)
	}
}

// ExitPlanModeExecute returns the ExitPlanMode catalog Stub over the plan-mode
// state: NOT in plan mode → the structured not-in-plan error (the target
// throws InvalidState; corpus-informed convention); ON → the plan rides the
// approval question payload and the call SUSPENDS (session.ErrSuspended — the
// same resume path AskUserQuestion proved; the captured ExitPlanMode success
// form was never observed — the 12-05 hunt — so the approved form is the
// documented source-informed default and the denial is the CAPTURED form).
func ExitPlanModeExecute(state *session.PlanModeState) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if state == nil || !state.IsOn() {
			return marshalStructured("exit plan mode: not in plan mode", errExitNotInPlan)
		}

		var a struct {
			Plan           string `json:"plan"`
			AllowedPrompts []struct {
				Tool   string `json:"tool"`
				Prompt string `json:"prompt"`
			} `json:"allowedPrompts"` //nolint:tagliatelle // captured input key
		}

		_ = json.Unmarshal(args, &a) // best-effort: the plan is required by schema

		q := session.AskQuestion{
			Header:   "Plan approval",
			Question: "Approve this implementation plan? Reply with an option.\n\n" + a.Plan,
			Options: []session.AskOption{
				{Label: "approve", Description: "approve the plan and exit plan mode"},
				{Label: "decline", Description: "keep planning"},
			},
		}
		for _, ap := range a.AllowedPrompts {
			q.Options[approveOptionIdx].Description += "; " + ap.Tool + ": " + ap.Prompt
		}

		out, err := json.Marshal([]session.AskQuestion{q})
		if err != nil {
			return marshalStructured("exit plan mode: marshal approval failed", err)
		}

		return out, session.ErrSuspended
	}
}

// errExitNotInPlan is the structured-error carrier for the not-in-plan call.
var errExitNotInPlan = &notInPlanError{}

type notInPlanError struct{}

func (*notInPlanError) Error() string { return "coreexec: ExitPlanMode outside plan mode" }

// InteractiveConfig carries the interactive-tool family's per-session state
// (12-04): the AskBroker (plan approvals ride the same suspension seam) and
// the PlanModeState. 12-06 extends this with the background TaskRegistry.
type InteractiveConfig struct {
	Ask      *session.AskBroker
	PlanMode *session.PlanModeState
	// Mailbox delivers SendMessage (nil = the executor returns the structured
	// no-mailbox error); Sessions reads ReadSessionContext (nil likewise).
	Mailbox  *AgentMailbox
	Sessions *SessionReader
}

// RegisterInteractive sets Execute on the interactive-family catalog entries
// (12-04: the plan pair; the messaging pair lands through the SAME site) —
// the RegisterCore discipline: ONLY the Execute field is overridden; Name,
// Description, InputSchema, and Mutability stay byte-identical to the captured
// entry. A missing catalog entry is skipped, not fatal (forward-compat).
func RegisterInteractive(catalog *toolcat.Catalog, cfg InteractiveConfig) {
	if catalog == nil {
		return
	}

	stubs := map[string]toolcat.Stub{
		enterPlanModeToolName: EnterPlanModeExecute(),
		exitPlanModeToolName:  ExitPlanModeExecute(cfg.PlanMode),
		"SendMessage":         SendMessageExecute(cfg.Mailbox),
		"ReadSessionContext":  ReadSessionContextExecute(cfg.Sessions),
	}

	for name, exec := range stubs {
		if tool, ok := catalog.Get(name); ok {
			tool.Execute = exec
			catalog.Register(tool)
		}
	}
}
