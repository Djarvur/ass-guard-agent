package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Djarvur/ass-guard-agent/internal/perm"
)

// The permission-gate chokepoint (17-02, ACP-01, D-04/D-05/D-06/D-07): THE
// one per-call pipeline — hook verdict → permission rules → mode/class/human
// decision — invoked for EVERY tool call in runTurn's loop, in BOTH dispatch
// branches (batch-eligible AND subagent). One pipeline, one seam: Phase 21's
// hooks join at its HEAD (the GATE PIPELINE LOCK); there is no second
// permission path for subagents or engine turns.
//
// Why here and not lower (17-RESEARCH Pattern 1): toolexec's per-call
// deadline wrap (executeBounded, default 120s) would deadline-kill any
// human-scale dialog wait inside the executor, the Stub seam carries no call
// identity, and subagent Task calls never reach DispatchBatch. The suspension
// rides the AskBroker discipline: gateSuspend ends the turn with the ask
// marker — the per-session turn mutex is NEVER held across a human wait.

// Permission-mode vocabulary (ACP-01's ungated|gated switch). The strings are
// the wire advertisement values (16-05's ConfigSurface menu); the live
// accessor is runner-owned and injected as GateDeps.Mode.
const (
	// PermModeUngated is the DEFAULT (criterion 4 — "available, not default"):
	// rules are evaluated and deny/allow enforced, but no dialog ever opens.
	PermModeUngated = "ungated"
	// PermModeGated: ask-class calls suspend for the native permission dialog
	// (human turns) or decline fail-safe (automation turns, D-07).
	PermModeGated = "gated"
)

// Dialog option kinds — byte-identical to the acp wire enum (internal/acp
// PermOption*). internal/session never imports internal/acp in production
// code, so the parity is pinned by TestGateAskKindsParity (the
// TestHumanAskTimeoutMirrorsSessionDefault pattern).
const (
	permOptAllowOnce    = "allow_once"
	permOptAllowAlways  = "allow_always"
	permOptRejectOnce   = "reject_once"
	permOptRejectAlways = "reject_always"
)

// Dialog card tool kinds (the ToolCallUpdate shape's optional kind), derived
// from the catalog class at enqueue time.
const (
	gateKindEdit    = "edit"
	gateKindExecute = "execute"
)

// errPermissionSurfaceUnwired is the fail-safe sentinel: a gate whose ask
// surface cannot fire (no fire callback injected) treats every gated ask as a
// decline — never a silent allow (the flagged-assumption fail-safe family).
var errPermissionSurfaceUnwired = errors.New("permission ask surface not wired")

// errPermissionStoreUnwired is the persist sentinel: a gate without a perm
// store cannot record an always-click, so the click degrades to once-only
// semantics with a loud structured log (the ACP-01 prohibition).
var errPermissionStoreUnwired = errors.New("permission store not wired")

// declineNoHumanReason names the D-07 automation-decline cause in the
// client-visible note.
const declineNoHumanReason = "no human is present to approve it (automation turn)"

// gateAction is the chokepoint's verdict for one tool call.
type gateAction uint8

const (
	// gateExecute — run the call (allowed, unmatched-read-only, or the
	// ungated default with no deny).
	gateExecute gateAction = iota
	// gateDeny — a deny rule (or, from Phase 21, a blocking hook) refused the
	// call: append the denial result, never execute.
	gateDeny
	// gateSuspend — gated + ask-class + human turn: record the pending ask,
	// enqueue on the ask queue, end the turn with the ask marker.
	gateSuspend
	// gateDeclineAutomation — gated or not, an automation turn never opens a
	// dialog (D-07): decline with a client-visible note + structured log.
	gateDeclineAutomation
)

// gateVerdict is gateCall's outcome. result carries the append-ready denial/
// decline payload for everything except gateExecute/gateSuspend.
type gateVerdict struct {
	action gateAction
	result json.RawMessage
}

// pendingPermission is the loop-side carry for a suspended call: the batch's
// survivors dispatch first, then the suspension ends the turn.
type pendingPermission struct {
	callID string
	tool   string
	input  json.RawMessage
}

// GateDeps carries the gate's injected dependencies (composition-wired in
// internal/runtime; the session stays free of internal/acp imports — the
// fire callback is injected from the acpserve surface via the runner).
type GateDeps struct {
	// Rules provides the live rule-set snapshot consulted per call (17-01's
	// perm.Store.Rules). nil = an empty rule set (nothing matches).
	Rules func() perm.RuleSet
	// Mode is the LIVE permission-mode accessor read PER CALL (Pitfall 8: a
	// permissions.mode flip must reach the running session without
	// recreation). nil = ungated (the boot default).
	Mode func() string
	// Allow persists a dialog allow_always click (perm.Store.AllowTool) —
	// BEFORE the gated call executes (the ACP-01 prohibition).
	Allow func(tool string) error
	// Forbid persists a dialog reject_always click (perm.Store.ForbidTool)
	// before the denial result lands (D-03).
	Forbid func(tool string) error
	// Fire performs one ask surface round-trip (the acpserve registry-backed
	// permission ask). nil = unwirable surface → fail-safe decline.
	Fire func(e *AskEntry) AskOutcome
	// Queue is the ask queue owning every dialog firing (D-11's
	// one-outstanding discipline; the broker's single-slot discipline
	// demoted to the queue's fired-head invariant).
	Queue *AskQueue
}

// SetPermissionGate wires the chokepoint (nil-unset fields degrade per their
// contracts above). Not wiring the gate at all preserves the v1.1 behavior
// for every session that never opts in.
func (s *Session) SetPermissionGate(deps GateDeps) { s.gate = &deps }

// SetTurnOriginAutomation marks the session's CURRENT turn as automation-
// origin (D-07's human-present signal). The runtime entry (the 12-07
// automation firing path) sets it around the firing turn; client-driven
// turns never set it.
func (s *Session) SetTurnOriginAutomation(v bool) { s.automationTurn.Store(v) }

// gateCall is THE per-call permission pipeline (D-04/D-05). See the
// package-file doc for the placement rationale.
func (s *Session) gateCall(turnID, callID, tool string, input json.RawMessage) gateVerdict {
	_ = turnID
	_ = callID

	deps := s.gate
	if deps == nil {
		// The gate is unwired: v1.1 behavior, every call executes (the
		// zero-new-dialogs default preserved for unwired sessions).
		return gateVerdict{action: gateExecute}
	}

	// ── Step 1: hook verdict head (D-04 — hooks are checked BEFORE permission
	// rules; CC parity, the blocking-hook precedent). PHASE 21 JOINS HERE, at
	// this exact seam: a blocking hook deny returns gateDeny; PAR-03's
	// deny-only authority (hooks never grant allow) means every allow case
	// still falls through to the rules below. Today: implicit allow — no hook
	// surface exists at the turn loop yet.

	// ── Step 2: permission rules — evaluated in BOTH modes (D-05, Pitfall 5):
	// "ungated = no dialogs" never means "no evaluation".
	var rules perm.RuleSet
	if deps.Rules != nil {
		rules = deps.Rules()
	}

	verdict := rules.Evaluate(s.ruleSubject(tool), primaryArgOf(tool, input))

	switch verdict {
	case perm.VerdictDeny:
		// A deny anywhere beats any allow, in both modes.
		return gateVerdict{action: gateDeny, result: permissionDenyForm(tool)}
	case perm.VerdictAllow:
		return gateVerdict{action: gateExecute}
	case perm.VerdictAsk:
		// An explicit ask rule routes to the dialog set REGARDLESS of tool
		// class (D-06: the file tunes ask subjects per pattern).
	case perm.Unmatched:
		if !s.gateAskClass(tool) {
			// Read-only concurrent class → allow without a dialog (D-06's
			// "everything else").
			return gateVerdict{action: gateExecute}
		}
	}

	// ── Step 3: the mode decision for the dialog set (D-05/D-06).
	if s.gateMode() != PermModeGated {
		// Ungated (the default): the ask step evaluated the rules and returns
		// allow WITHOUT a dialog — criterion 4's zero-new-dialogs default.
		return gateVerdict{action: gateExecute}
	}

	// ── Step 4: the human-present decision (D-07). An automation turn NEVER
	// opens a dialog nobody would answer: fail-safe decline, deny/allow rules
	// already enforced above.
	if s.automationTurn.Load() {
		slog.Warn("permission gate: ask-class call declined on an automation turn (D-07 fail-safe)",
			"turnID", turnID, "callID", callID, "tool", tool)

		return gateVerdict{
			action: gateDeclineAutomation,
			result: permissionDeclineForm(tool, declineNoHumanReason),
		}
	}

	// Gated + human turn: suspend. The session site records the pending ask,
	// enqueues on the ask queue, and ends the turn with the ask marker — the
	// turn mutex is released; the dialog answer drives the resume.
	return gateVerdict{action: gateSuspend}
}

// gateMode resolves the live mode through the injected accessor (empty → the
// ungated boot default).
func (s *Session) gateMode() string {
	if s.gate != nil && s.gate.Mode != nil {
		if m := s.gate.Mode(); m != "" {
			return m
		}
	}

	return PermModeUngated
}

// gateAskClass mirrors toolexec.isAloneInSlot against the toolcat API (the
// session cannot import toolexec's private helper — 17-PATTERNS): a mutating
// tool or a declared concurrency-unsafe tool is D-06's default ask class —
// today's mutating/serialized set, no new classification.
func (s *Session) gateAskClass(tool string) bool {
	if s.Catalog == nil {
		return false
	}

	t, ok := s.Catalog.Get(tool)
	if !ok {
		return false
	}

	return t.IsMutating() || !t.IsConcurrencySafe()
}

// ruleSubject resolves the rule-matching subject for one call: the catalog
// tool name. MCP tools are cataloged under their full mcp__<server>__<tool>
// namespace (internal/mcp Host.Register), so the D-02 namespace maps
// identity-side; bare mcp__<server> rules match the whole server through the
// grammar's namespace prefix rule (17-01). The host-knowledge reverse mapping
// (RESEARCH Pitfall 7) rides the injected resolver in the namespace task.
func (s *Session) ruleSubject(tool string) string {
	return tool
}

// gateDialogKind derives the dialog card's tool kind from the catalog class
// (the ToolCallUpdate shape's optional kind): mutating → edit, else execute.
func (s *Session) gateDialogKind(tool string) string {
	if t, ok := s.Catalog.Get(tool); ok && t.IsMutating() {
		return gateKindEdit
	}

	return gateKindExecute
}

// primaryArgOf extracts the rule-matching primary argument for one call (the
// D-02 specifier surface): Bash's command; the file tools' path; anything
// else matches bare-tool rules only (empty argument — compound splitting and
// glob semantics live in the grammar).
func primaryArgOf(tool string, input json.RawMessage) string {
	switch tool {
	case toolBash:
		var p struct {
			Command string `json:"command"`
		}

		_ = json.Unmarshal(input, &p)

		return p.Command
	case toolRead, toolWrite, toolEdit, "NotebookEdit":
		var p struct {
			FilePath string `json:"file_path"`
		}

		_ = json.Unmarshal(input, &p)

		return p.FilePath
	default:
		return ""
	}
}

// suspendForPermission records the suspension in the transcript (the
// ask_suspended audit marker — projector-skipped, like the question family),
// then enqueues the permission ask on the injected queue. The turn ends with
// the ask marker right after; the queue's pump fires the surface and drives
// resumePermissionAsk — sessionTurnMu is NOT held across the human wait
// (RESEARCH Pitfall 2).
func (s *Session) suspendForPermission(turnID, callID, tool string, input json.RawMessage) {
	deps := s.gate
	if deps == nil || deps.Queue == nil {
		// No queue wired: the gate cannot ask. Fail-safe — append the decline
		// result so the model is never silently dead-ended.
		s.appendToolResultLoud(turnID, callID, tool,
			permissionDeclineForm(tool, "the ask queue is not wired"), true)

		return
	}

	p := PendingAsk{
		TurnID: turnID,
		CallID: callID,
		Kind:   PendingAskKindPermission,
		Tool:   tool,
		Input:  append(json.RawMessage(nil), input...),
	}
	p.settle = make(chan struct{})

	payload, mErr := json.Marshal(map[string]string{"tool": tool})
	if mErr != nil {
		payload = json.RawMessage(`{"tool":"unknown"}`)
	}

	_ = s.Manager.AppendAskSuspended(turnID, callID, payload)

	entry := &AskEntry{
		TurnID:    turnID,
		SessionID: s.SessionID,
		CallID:    callID,
		Tool:      tool,
		Title:     tool,
		Kind:      s.gateDialogKind(tool),
		Input:     p.Input,
		Class:     s.gateEntryClass(),
	}
	entry.fire = func() AskOutcome {
		if deps.Fire == nil {
			// The unwirable-surface fail-safe (the flagged assumption):
			// decline — never a silent allow.
			return AskOutcome{Err: errPermissionSurfaceUnwired}
		}

		return deps.Fire(entry)
	}

	deps.Queue.Enqueue(entry, func(_ *AskEntry, outcome AskOutcome) {
		s.resumePermissionAsk(s.askResumeCtx, &p, outcome)
	})
}

// gateEntryClass resolves the queue priority class (D-11): automation/engine
// asks are background; foreground (client) turns are foreground. Subagent-
// origin classification joins with the queue's priority expansion (17-03).
func (s *Session) gateEntryClass() AskClass {
	if s.automationTurn.Load() {
		return AskClassBackground
	}

	return AskClassForeground
}

// permissionPersistAllow records an allow_always click through the injected
// store writer. A failure is the CALLER's downgrade signal (once-only
// semantics + loud log) — it never blocks the execution the user approved.
func (s *Session) permissionPersistAllow(tool string) error {
	if s.gate == nil || s.gate.Allow == nil {
		return errPermissionStoreUnwired
	}

	return s.gate.Allow(tool)
}

// permissionDenyForm renders the structured denial result (isErr=true): a
// deny rule refused the call.
func permissionDenyForm(tool string) json.RawMessage {
	out, err := json.Marshal(map[string]string{
		mapKeyError: fmt.Sprintf("Permission denied: %s is denied by the project permission rules; "+
			"the call was not executed.", tool),
	})
	if err != nil {
		return json.RawMessage(`{"error":"permission denied (form render failed)"}`)
	}

	return out
}

// permissionDeclineForm renders the structured decline result (isErr=true):
// the call needed approval that could not be obtained (D-07's fail-safe —
// the audit trail is this transcript line + the structured log).
func permissionDeclineForm(tool, reason string) json.RawMessage {
	out, err := json.Marshal(map[string]string{
		mapKeyError: fmt.Sprintf("Permission declined: %s requires approval but %s. "+
			"The call was not executed; it will re-ask in a foreground session.", tool, reason),
	})
	if err != nil {
		return json.RawMessage(`{"error":"permission declined (form render failed)"}`)
	}

	return out
}

// permissionCancelledForm renders the cancelled-NORMAL result (isErr=FALSE —
// criterion 2's letter: a dismissed dialog is not an error).
func permissionCancelledForm(tool string) json.RawMessage {
	out, err := json.Marshal(map[string]string{
		"output": fmt.Sprintf("Tool call cancelled: the operator dismissed the permission dialog for %s "+
			"without granting it. Continue without this call's result.", tool),
	})
	if err != nil {
		return json.RawMessage(`{"output":"tool call cancelled (form render failed)"}`)
	}

	return out
}
