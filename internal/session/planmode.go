package session

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"
)

// PlanModeState is the model-facing per-session plan-mode state with a
// RUNTIME-LEVEL mutating-tool gate (12-04, ACP-02). SCOPE DECISION (resolved by
// the 12-05 capture, zcode 0.16.3): the TARGET enforces plan mode at the
// tool-result level — mutating calls are REFUSED ('Plan mode only allows
// read-only, non-destructive tools'; captured 2026-08-20 in
// internal/coreexec/testdata/zcode-recaptured-2026-08.json) — NOT model
// self-restraint as the 12-04 plan's pre-capture premise assumed. ass-guard
// mirrors the target: while plan mode is ON, gated calls return the CAPTURED
// refusal form without executing. This is NOT a confirmation tier (no human in
// the loop; the model toggles the state itself via EnterPlanMode/ExitPlanMode
// and the approval rides the 12-01 ask path).
type PlanModeState struct {
	mu        sync.Mutex
	on        bool
	used      bool // any transition was recorded (live or resume-seeded)
	enteredAt time.Time
}

// NewPlanModeState returns an OFF plan-mode state (one per session; the
// session tool loop is the sole writer, so no per-call construction).
func NewPlanModeState() *PlanModeState { return &PlanModeState{} }

// Enter flips the state ON (the EnterPlanMode result path).
func (p *PlanModeState) Enter() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.on {
		p.on = true
		p.enteredAt = time.Now().UTC()
	}

	p.used = true
}

// Exit flips the state OFF (an APPROVED ExitPlanMode resume — the only path
// that ungates; a timed-out or declined approval keeps the gate ON).
func (p *PlanModeState) Exit() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.on = false
	p.used = true
}

// IsOn reports the current state.
func (p *PlanModeState) IsOn() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.on
}

// Used reports whether ANY transition was ever recorded (18-05): a session
// that never touched plan mode — live or on resume — reports no mode state
// (the load response's modes field stays null; the client assumes defaults),
// while a session whose LAST transition was an exit still reports the v1
// shape with the default mode current.
func (p *PlanModeState) Used() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.used
}

// Marker causes + line type: schema-stable audit lines (the ask_suspended
// shape — the Projector skips them; they are NOT boundary lines and never
// reset any window; entering plan mode is not a mutating call).
const (
	// TypePlanMode records a plan-mode transition (12-04, ACP-02): cause
	// plan_mode_enter on the EnterPlanMode result, plan_mode_exit on the
	// approved ExitPlanMode resume. An audit marker like ask_suspended.
	TypePlanMode       = "plan_mode"
	PlanModeCauseEnter = "plan_mode_enter"
	PlanModeCauseExit  = "plan_mode_exit"
	planModeCauseEnter = PlanModeCauseEnter
	planModeCauseExit  = PlanModeCauseExit
)

// PlanModeEnteredForm is the CAPTURED EnterPlanMode result (12-05 re-record,
// zcode 0.16.3, 43 observations — the full guidance list verbatim; the single
// definition lives here like the ask renderers, coreexec's executor renders it;
// the long first line is the captured form verbatim).
const PlanModeEnteredForm = "Entered plan mode. You should now focus on exploring the codebase " +
	"and designing an implementation approach." + `

In plan mode, you should:
1. Thoroughly explore the codebase to understand existing patterns
2. Identify similar features and architectural approaches
3. Consider multiple approaches and their trade-offs
4. Use AskUserQuestion if you need to clarify the approach
5. Design a concrete implementation strategy
6. When ready, use ExitPlanMode to present your plan for approval

Remember: DO NOT write or edit any files yet. This is a read-only exploration and planning phase.`

// errExitPlanModeNotInPlan is the not-in-plan-mode error (the target throws
// InvalidState; corpus-informed structured convention — the exact target text
// was not captured).
var errExitPlanModeNotInPlan = planModeError("ExitPlanMode called outside plan mode")

// planModeError types the plan-mode executor errors (a thin wrapper keeping the
// sentinel comparable without exporting a second error vocabulary).
type planModeError string

func (e planModeError) Error() string { return "session: plan mode: " + string(e) }

// planModeRefusalForm is the CAPTURED gate refusal (12-05 re-record, 43+28
// observations across SendMessage/TaskStop/CronCreate — the runtime-level
// plan-mode enforcement text).
const planModeRefusalForm = "Plan mode only allows read-only, non-destructive tools"

// planModeExtraRefused names the tools the capture showed refused under plan
// mode BEYOND catalog mutability (read-only in ass-guard's captured catalog
// but side-effecting in the target: messaging, task control, and the
// persistent-state cron family — 12-06/12-07 register their executors against
// this same gate).
var planModeExtraRefused = map[string]bool{ //nolint:gochecknoglobals // immutable capture-pinned table
	"SendMessage": true,
	"TaskStop":    true,
	"CronCreate":  true,
	"CronUpdate":  true,
	"CronDelete":  true,
}

// SetPlanMode wires the per-session plan-mode state (nil = plan mode not
// wired: the gate never fires, EnterPlanMode degrades to its form).
func (s *Session) SetPlanMode(p *PlanModeState) { s.planMode = p }

// PlanModeOn reports whether plan mode is ON (nil-state safe).
func (s *Session) PlanModeOn() bool { return s.planMode != nil && s.planMode.IsOn() }

// planModeBlocks reports whether the named tool is gated while plan mode is
// ON: catalog-mutating tools plus the captured extra-refused set. Read-only
// tools (Read, the plan pair, AskUserQuestion, ReadSessionContext) pass —
// the capture shows exploration + questioning continue in plan mode.
func (s *Session) planModeBlocks(toolName string) bool {
	if !s.PlanModeOn() {
		return false
	}

	return planModeExtraRefused[toolName] || isCatalogMutating(s.Catalog, toolName)
}

// planModeRefusal renders the CAPTURED gate refusal as the tool result
// (isError=true — the captured families all carry isError).
func planModeRefusal() json.RawMessage {
	out, err := json.Marshal(planModeRefusalForm)
	if err != nil {
		return json.RawMessage(`"plan mode refusal render failed"`)
	}

	return out
}

// appendPlanModeMarker records a plan-mode transition line (schema-stable,
// cause-named — the boundary-line WRITER shape but a distinct type so the
// Projector's boundary readers never see it).
func (s *Session) appendPlanModeMarker(cause, toolCallID, turnID string) {
	_ = s.Manager.AppendPlanMode(cause, toolCallID, turnID)
}

// planApprovalQuestion renders the approval question text: the plan rides the
// question (the 12-01 surface renderer publishes it to the ACP client).
func planApprovalQuestion(plan string) string {
	return "Approve this implementation plan? Reply with an option.\n\n" + plan
}

// RenderPlanApprovalResolved renders the ExitPlanMode result for a resolved
// approval (12-04): an approving reply → the approved form (source-informed
// corpus-absent default — the target's own formatter text, hunt recorded in
// the 12-05 fixture); anything else → the CAPTURED denial form (48 obs).
func RenderPlanApprovalResolved(plan, reply string) (string, bool) {
	if isApprovalReply(reply) {
		return "User has approved your plan. You can now start coding. " +
			"Start with updating your todo list if applicable. ## Approved Plan: " + plan, false
	}

	return "Permission denied for ExitPlanMode", true
}

// isApprovalReply matches the surfaced options' labels + common synonyms
// (case-insensitive prefix match; the surfaced options are approve/decline).
func isApprovalReply(reply string) bool {
	r := strings.ToLower(strings.TrimSpace(reply))

	return slices.Contains([]string{"approve", "approved", "yes", "y", "ok", "go ahead", "1"}, r)
}
