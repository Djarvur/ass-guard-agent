package session

import (
	"context"
	"encoding/json"
	"strings"
)

// The kit-neutral session vocabulary (25-06, D-05 per-case mirrors): the
// agent-definition, hook, and permission-rule types the Session struct and
// its seams name. The kit never imports the app packages that produce these
// values (internal/ecosys discovery, internal/perm's rule store); the
// composition's adapter maps app values onto these types exactly ONCE
// (internal/acpserve/catalog_adapter.go — Pitfall 3).
//
// Mirror-or-promote choice, per type:
//   - AgentDef: MIRROR. The full discovered agent record is ecosystem
//     vocabulary (loader-fed, app-side per D-17); the kit carries only the
//     fields dispatch actually reads — subagent.go's Tools/Prompt and the
//     runtime planner's Model (20-03 D-13 frontmatter arm). Name and
//     Description ride along for chain/advisory surfaces. Field minimality
//     limits what a repo-shipped definition can influence (T-25-27).
//   - Hooks: MIRROR (an interface, not a promoted type). Hook execution is
//     behavioral machinery that stays app-side (D-17); the kit sees only the
//     three-method surface its turn loop and gate head consume. The
//     app-side adapter wraps the ecosystem hook runner in this interface.
//   - HookOutcome / HookVerdict: MIRROR (pure data, minimal fields). The
//     captured-stdout payload and the four-valued verdict are consumed
//     contracts; mirroring keeps the enum's authority app-side.
//   - RuleSet / RuleVerdict: MIRROR (the 25-03-assigned session→perm
//     severance). The rule store, its grammar, and persistence stay
//     app-side (internal/perm); the gate consumes an Evaluate-only view
//     returning a kit enum. The composition adapts the app rule set.

// AgentDef is one spawnable subagent type definition as the kit consumes it
// (the mirror of the discovered agent record's dispatch-relevant fields):
// Prompt applies as the per-dispatch system block, Tools as the restricted
// set, Model as the 20-03 D-13 frontmatter routing arm. An unknown or absent
// type falls back to the default restricted subagent (advisory listing).
type AgentDef struct {
	Name        string
	Description string
	Tools       []string
	Model       string
	Prompt      string
}

// Hooks is the lifecycle-hook surface the session consumes (the mirror of
// the ecosystem hook runner's kit-relevant method set): Fire drives the
// SessionStart/UserPromptSubmit/Stop/SubagentStop/SessionEnd lifecycle
// seams, PreToolUseVerdict is the permission-gate pipeline's HEAD (21-06
// D-04 — hooks before rules), and PostToolUse is the coreexec.ToolHooks
// observation seam at the tool-exec chokepoint. The interface is
// deny/lifecycle-shaped exactly as the locked Phase-17/21 pipeline defines:
// no allow-granting method exists kit-side, and verdict authority (only
// USER scope may allow) is resolved app-side BEFORE the verdict crosses
// this seam (T-25-25).
//
// Implemented app-side over the discovered hook table; nil = no hooks wired
// (every seam no-ops — the documented degraded state, never a panic).
type Hooks interface {
	// Fire runs the mapped lifecycle event and returns its outcome
	// (Proceed=false ONLY for a blocking refusal; Message carries the
	// refusal text or the context-bearing captured stdout).
	Fire(ctx context.Context, event string, fields map[string]any) HookOutcome
	// PreToolUseVerdict resolves the combined PreToolUse verdict for one
	// call (deny-wins over the scope order; allow is USER-scope only,
	// already demoted app-side). VerdictNone is the no-decision fall-through.
	PreToolUseVerdict(ctx context.Context, toolName string, input json.RawMessage) (HookVerdict, string)
	// PostToolUse observes a completed tool result (never blocks).
	PostToolUse(ctx context.Context, toolName string, input, output json.RawMessage)
}

// HookOutcome is one Fire result (the mirror of the ecosystem runner's
// outcome): Proceed=false ONLY for a blocking refusal; Message carries the
// refusal text or the captured stdout (the context-bearing events'
// injection payload).
type HookOutcome struct {
	Proceed bool
	Message string
}

// HookVerdict is the four-valued PreToolUse verdict (the mirror of the
// ecosystem verdict enum): the gate head maps each value onto exactly one
// gate action — deny blocks, ask escalates through the fail-safes, allow
// executes, none falls through to the permission rules (T-21-20's
// one-target-per-value rule).
type HookVerdict int

const (
	// HookVerdictNone is no decision — silence, schema-invalid, or
	// execution failure. Never an approval (Pitfall 4).
	HookVerdictNone HookVerdict = iota
	// HookVerdictDeny — a denying hook refused the call.
	HookVerdictDeny
	// HookVerdictAsk — the operator's escalation lever: route through the
	// ask path's fail-safes, then suspend EVEN UNGATED.
	HookVerdictAsk
	// HookVerdictAllow — a USER-scope allow executes (hook-scoped, never
	// persisted; non-user allows are demoted app-side before crossing).
	HookVerdictAllow
)

// RuleVerdict is the permission-rule outcome for one (tool, argument)
// subject (the mirror of the app rule engine's verdict): the mode+class
// decision at the gate chokepoint remains the CALLER's — Unmatched never
// silently allows, it defers to the ask-class/mode decision (D-05/D-06).
type RuleVerdict int

const (
	// RuleUnmatched — no rule matched; the gate's class/mode arms decide.
	RuleUnmatched RuleVerdict = iota
	// RuleAllow — an allow rule matched (execute).
	RuleAllow
	// RuleAsk — an ask rule matched (permission surface).
	RuleAsk
	// RuleDeny — a deny rule matched (never execute; deny beats allow).
	RuleDeny
)

// RuleSet is the permission-rule evaluation view the gate consumes (the
// 25-03-assigned session→perm severance): the app-side store supplies the
// live snapshot through GateDeps.Rules; the grammar, compound splitting,
// and persistence stay app-side. A nil Rules dependency degrades to an
// empty rule set (nothing matches — Unmatched everywhere).
type RuleSet interface {
	// Evaluate returns the verdict for one toolName/primaryArg subject.
	Evaluate(toolName, primaryArg string) RuleVerdict
}

// The MCP tool-name canonicalization (the mirror of the app-side namespace
// helpers, Pitfall 7): the catalog registers MCP tools under their full
// mcp__<server>__<tool> names, so the mapping is a split-and-rebuild plus
// identity for non-MCP tools. The convention is wire vocabulary the kit
// already speaks (kit/mcp names tools the same way); mirroring the two
// helpers kit-side keeps the session package free of the app rule package.
const (
	mcpNamePrefix = "mcp__"
	mcpNameSep    = "__"
)

// canonicalToolName normalizes one tool name for rule matching: MCP names
// split and rebuild through the canonical joiner (identity for already-
// canonical and non-MCP names alike — the rebuild IS the canonical form).
func canonicalToolName(tool string) string {
	rest, found := strings.CutPrefix(tool, mcpNamePrefix)
	if !found {
		return tool
	}

	server, tl, found := strings.Cut(rest, mcpNameSep)
	if !found || server == "" || tl == "" {
		return tool
	}

	return mcpNamePrefix + server + mcpNameSep + tl
}
