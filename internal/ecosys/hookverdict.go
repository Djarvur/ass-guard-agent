// The PreToolUse verdict substrate (21-01 PAR-03): the per-hook verdict
// channels (CC's hookSpecificOutput JSON + the legacy exit-2 refusal) and the
// pure deny-wins resolver over the D-03 scope order. This file is
// 17-independent by design — the runner RESOLVES verdicts here; the Phase-17
// gate head (internal/session/gate.go, joined in 21-06) is the one CONSUMPTION
// site, and ResolveVerdict's purity is what makes that join a one-line
// delegation (21-RESEARCH Pattern 2).
//
// Locked rules (21-CONTEXT):
//
//   - D-01: hook ALLOW authority is scope-split — USER-scope settings hooks
//     may return allow; PROJECT-scope (repo-shipped) hooks are deny-only; a
//     non-user allow is demoted to a loud ignored-allow warning and
//     contributes no decision. ass-guard has no trust dialog, so the
//     demotion IS the repo-trust mitigation.
//   - D-02: both verdict channels parse — hookSpecificOutput JSON
//     (permissionDecision allow/deny/ask + permissionDecisionReason) AND the
//     legacy exit-2 refusal; exit 2 BLOCKS regardless of any JSON verdict on
//     stdout (the CC spoofing rule, T-21-02).
//   - D-03: deny-wins over the deterministic scope order project → user →
//     plugin; the deny carries the FIRST denying hook's reason. Fail-open
//     applies to hook EXECUTION failure only, never to conflicting verdicts.
//   - D-04: ask survives unless a deny exists — the operator's escalation
//     lever, routed to the ask step by the gate join (21-06).
//
// Timeout divergence (flagged assumption A2): hooks keep the existing 60s
// hookDefaultTimeoutSec bound rather than CC's documented 600s command-hook
// default, to bound tool-loop latency; see loadSettingsHooks.

package ecosys

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

// Verdict is the four-valued per-hook outcome (D-02/Pitfall 4): deny / ask /
// allow from an explicit permissionDecision (or the exit-2 legacy deny), and
// NO-DECISION for everything else. Silence NEVER approves: exit-0 without
// valid decision JSON and schema-invalid JSON both yield VerdictNone
// (fail-open — the call proceeds through the normal flow).
type Verdict int

// The verdict constants are exported by necessity at the 21-06 gate join:
// the session gate head (internal/session/gate.go) maps this four-valued
// enum onto the 17 gateVerdict actions, so the enum is a consumed contract,
// not a package-internal detail.
const (
	VerdictNone  Verdict = iota // no decision — silence, schema-invalid, or execution failure
	VerdictDeny                 // explicit deny (JSON permissionDecision "deny" or legacy exit 2)
	VerdictAsk                  // explicit ask — the operator escalation lever (D-04)
	VerdictAllow                // explicit allow — honored from USER scope only (D-01)
)

// ScopedResult is one hook's parsed verdict paired with its discovery scope.
// Callers hand ResolveVerdict the results in D-03 order (project → user →
// plugin); the resolver is a pure fold over that ordered slice.
type ScopedResult struct {
	Scope   HookScope
	Verdict Verdict
	Reason  string
}

// permissionDecision values (CC's documented vocabulary).
const (
	decisionDeny  = "deny"
	decisionAsk   = "ask"
	decisionAllow = "allow"
)

// parseHookVerdict classifies ONE hook execution into a Verdict. The exit-2
// check comes FIRST and overrides everything (D-02/T-21-02: a refusal code
// blocks even when stdout carries a valid JSON allow); any other execution
// failure is fail-open (VerdictNone, PAR-03 letter); a clean run takes the
// JSON channel, which admits a verdict ONLY from a stdout whose first
// non-whitespace character is `{` and whose hookSpecificOutput body is
// schema-valid (permissionDecision allow/deny/ask + string reason). The
// exit-2 reason here is the capped stdout (the only channel this signature
// sees); the composed runner path upgrades it to classifyHookRun's
// stderr-first extraction.
func parseHookVerdict(stdout string, runErr error) (verdict Verdict, reason string) { //nolint:nonamedreturns // D-02
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitCode() == hookRefusalExitCode {
			return VerdictDeny, capHookOutput(stdout)
		}

		return VerdictNone, ""
	}

	body := strings.TrimLeft(stdout, " \t\r\n")
	if !strings.HasPrefix(body, "{") {
		return VerdictNone, "" // non-JSON stdout — no decision
	}

	var raw struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`       //nolint:tagliatelle // CC wire field
			PermissionDecisionReason string `json:"permissionDecisionReason"` //nolint:tagliatelle // CC wire field
		} `json:"hookSpecificOutput"` //nolint:tagliatelle // CC wire field
	}

	// Schema-invalid JSON (truncated body, wrong-typed fields) unmarshals to
	// an error → no decision — never honored (Pitfall 4).
	if json.Unmarshal([]byte(body), &raw) != nil {
		return VerdictNone, ""
	}

	switch raw.HookSpecificOutput.PermissionDecision {
	case decisionDeny:
		return VerdictDeny, raw.HookSpecificOutput.PermissionDecisionReason
	case decisionAsk:
		return VerdictAsk, raw.HookSpecificOutput.PermissionDecisionReason
	case decisionAllow:
		return VerdictAllow, raw.HookSpecificOutput.PermissionDecisionReason
	default:
		return VerdictNone, "" // missing or unknown decision value
	}
}

// ResolveVerdict folds ordered ScopedResults (D-03: project → user →
// plugin) into the combined verdict — a PURE function with no exec/context
// dependency so it stays table-testable and the 21-06 gate join is a
// delegation:
//
//   - ANY deny wins, carrying the FIRST denying result's reason (not the
//     last, not the most severe) — deny-wins mirrors 17-D-02.
//   - A USER-scope allow applies only when no deny exists (D-01).
//   - A non-user allow (project or plugin scope) is DEMOTED: it contributes
//     no decision and emits one loud ignored-allow warning through the
//     package warning sink per demoted result — repo-shipped files can never
//     widen trust (D-01/T-21-01).
//   - ask survives unless a deny exists (D-04).
//   - All no-decision (or empty input) → VerdictNone.
func ResolveVerdict(results []ScopedResult) (verdict Verdict, reason string) { //nolint:nonamedreturns // D-01..D-04
	var (
		firstDeny *ScopedResult
		firstAsk  *ScopedResult
		userAllow *ScopedResult
	)

	for i := range results {
		res := &results[i]

		switch res.Verdict {
		case VerdictDeny:
			if firstDeny == nil {
				firstDeny = res
			}
		case VerdictAsk:
			if firstAsk == nil {
				firstAsk = res
			}
		case VerdictAllow:
			if res.Scope == ScopeUser {
				if userAllow == nil {
					userAllow = res
				}

				continue
			}

			// D-01 demotion: only USER scope may widen trust. One loud
			// warning per demoted result — never silent, never a decision.
			logPluginSkipf("ignored allow from %s-scope hook (D-01: deny-only authority): %q",
				scopeName(res.Scope), res.Reason)
		case VerdictNone:
			// Contributes nothing.
		}
	}

	switch {
	case firstDeny != nil:
		return VerdictDeny, firstDeny.Reason
	case firstAsk != nil:
		return VerdictAsk, firstAsk.Reason
	case userAllow != nil:
		return VerdictAllow, userAllow.Reason
	default:
		return VerdictNone, ""
	}
}

// scopeName renders a scope for warnings (the loud-demotion wording).
func scopeName(s HookScope) string {
	switch s {
	case ScopeProject:
		return "project"
	case ScopeUser:
		return "user"
	case ScopePlugin:
		return "plugin"
	default:
		return "plugin"
	}
}
