package session //nolint:testpackage // internal package test (the kit/session battery family)

import (
	"context"
	"encoding/json"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/perm"
)

// 25-06 test twins (the 25-05 engine_setup_test precedent): the batteries
// below plant REAL ecosystem fixtures (script hooks, permission rule sets)
// and adapt them onto the kit-neutral session types at the wiring site,
// exactly as the composition's app-side adapter does in production. Test
// files may import the app packages (the documented D-19 ride-along
// exemption); PRODUCTION kit/session code may not — pinned by the dep-list
// checks in the plan's verify clauses.

// testHooks adapts a real ecosystem hook runner onto the kit Hooks
// interface for seam batteries (the gate tests plant genuine script hooks
// and assert their verdicts end-to-end).
type testHooks struct{ runner *ecosys.HookRunner }

func (h testHooks) Fire(ctx context.Context, event string, fields map[string]any) HookOutcome {
	out := h.runner.Fire(ctx, event, fields)

	return HookOutcome{Proceed: out.Proceed, Message: out.Message}
}

func (h testHooks) PreToolUseVerdict(
	ctx context.Context, toolName string, input json.RawMessage,
) (verdict HookVerdict, reason string) { //nolint:nonamedreturns // mirrors the seam's named pair
	v, r := h.runner.PreToolUseVerdict(ctx, toolName, input)

	switch v {
	case ecosys.VerdictNone:
		return HookVerdictNone, ""
	case ecosys.VerdictDeny:
		return HookVerdictDeny, r
	case ecosys.VerdictAsk:
		return HookVerdictAsk, r
	case ecosys.VerdictAllow:
		return HookVerdictAllow, r
	default:
		return HookVerdictNone, "" // an unknown app verdict stays no-decision
	}
}

func (h testHooks) PostToolUse(ctx context.Context, toolName string, input, output json.RawMessage) {
	h.runner.PostToolUse(ctx, toolName, input, output)
}

// testRuleSet adapts a real app permission rule set onto the kit RuleSet
// view (the fakePermStore composition): verdict values map through an
// explicit switch, mirroring the production adapter.
type testRuleSet struct{ rs perm.RuleSet }

func (t testRuleSet) Evaluate(toolName, primaryArg string) RuleVerdict {
	switch t.rs.Evaluate(toolName, primaryArg) {
	case perm.Unmatched:
		return RuleUnmatched
	case perm.VerdictAllow:
		return RuleAllow
	case perm.VerdictAsk:
		return RuleAsk
	case perm.VerdictDeny:
		return RuleDeny
	default:
		return RuleUnmatched // an unknown app verdict stays unmatched
	}
}
