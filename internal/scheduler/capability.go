package scheduler

import (
	"fmt"
	"strings"
)

// CapabilityError is returned when no candidate for a tier satisfies the turn's
// capability requirement (D-09 request-time gate). It names the unmet
// requirement and the candidates that were checked — investigate-and-fix-ready.
type CapabilityError struct {
	Requirement string   // e.g. "tool_calling+streaming"
	Candidates  []string // the model slugs that were checked
}

// Error formats the mismatch investigate-and-fix-ready (C5).
func (e *CapabilityError) Error() string {
	return fmt.Sprintf("scheduler: no candidate for tier satisfies %s: checked %s",
		e.Requirement, strings.Join(e.Candidates, ", "))
}

// applyCapabilityGate filters [primary, ...fallback] by the capability
// requirement, returning the first capable candidate plus the remaining chain
// (the candidates after it, so Dispatch's walker still has a fallback chain).
// A zero-valued capReq (no specific needs) disables the gate — primary and the
// full chain are returned unchanged (Plan 03-01 behavior preserved).
//
// This is the request-time half of SCHED-06 (D-09), pairing with the load-time
// D-10 validation in Validate: together they make tier mismatch explicit at
// both authoring time and request time.
func applyCapabilityGate(primary Target, fallback []Target, capReq CapabilityReq) (Target, []Target, error) {
	if capReq == (CapabilityReq{}) {
		return primary, fallback, nil
	}

	candidates := append([]Target{primary}, fallback...)
	for i, cand := range candidates {
		if satisfies(cand.Capabilities, capReq) {
			return cand, candidates[i+1:], nil
		}
	}

	slugs := make([]string, len(candidates))
	for i, c := range candidates {
		slugs[i] = c.Model
	}

	return Target{}, nil, &CapabilityError{Requirement: describeReq(capReq), Candidates: slugs}
}

// describeReq renders a CapabilityReq as a human-readable requirement string
// for CapabilityError (C5).
func describeReq(req CapabilityReq) string {
	var parts []string
	if req.NeedsTools {
		parts = append(parts, "tool_calling")
	}

	if req.NeedsStreaming {
		parts = append(parts, "streaming")
	}

	if req.NeedsThinking {
		parts = append(parts, "extended_thinking")
	}

	if len(parts) == 0 {
		return "(none)"
	}

	return strings.Join(parts, "+")
}

// satisfies reports whether a capability profile meets a capability requirement
// (D-09). A zero-valued CapabilityReq (no specific needs) is always satisfied.
// Relocated from resolver.go (Plan 03-04) so all capability logic lives here.
func satisfies(caps CapabilityProfile, req CapabilityReq) bool {
	if req.NeedsTools && !caps.ToolCalling {
		return false
	}

	if req.NeedsStreaming && !caps.Streaming {
		return false
	}

	if req.NeedsThinking && !caps.ExtendedThinking {
		return false
	}

	return true
}
