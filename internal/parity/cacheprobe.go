// cacheprobe.go (14-04, EARLY-03) is the parity harness's cache-discipline
// probe: it observes one COMPOSED request shape (the same profile data the
// Shaper composes from, plus the dynamic merges the serve path applies) and
// asserts the ordering discipline that keeps the captured prefix cache-stable.
//
// AUTHORITY RULE (locked prohibition): cache_control placement authority is
// the zcode CORPUS pin (14-02's committed fixture and its census), not pi's
// rules — on conflict, the corpus wins. The captured target places
// cache_control {"type":"ephemeral"} on EVERY system block (910/910 census
// placements) and nowhere else; pi-derived expectations (last-tool
// breakpoints, conversation-history breakpoints, ttl knobs) are pi-isms with
// no target counterpart and never enter this probe's expectations.
//
// Ordering discipline ("stable→volatile", EARLY-03 / ECOSYSTEM-AUDIT §4.4
// CACHE): the captured profile's system blocks and tools array are the STABLE
// prefix; everything dynamically merged at runtime (skills/agents listing
// blocks from CMD-06/12-02, hook-context blocks from 12-01, mcp__* tools from
// the MCP host) is VOLATILE and must append AFTER the stable prefix — never
// splice into it. The dynamic-merge-into-captured-shape pattern (08-05
// skills, 12-02 agents, 12-01 hook context, the MCP tool bridge) already
// appends; this probe PINS that property so a future merge site cannot
// silently regress it (a merge can be structurally perfect and still bust
// the cache prefix).
package parity

import (
	"fmt"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// Probe check names (footer/report-stable identifiers).
const (
	checkSystemOrdering = "system-ordering"
	checkToolsOrdering  = "tools-ordering"
)

// ProbeSystemBlock is the probe's view of one composed system[] entry: the
// profile's TextBlock (captured or runtime-merged) labeled by WHO produced
// it. Dynamic=true marks a runtime merge (skills/agents listing, hook
// context) — the caller labels what it merged.
type ProbeSystemBlock struct {
	Block   profile.TextBlock
	Dynamic bool
}

// ProbeToolDecl is the probe's view of one composed tools[] declaration:
// the profile's Decl labeled captured vs runtime-appended (mcp__<server>__<tool>).
type ProbeToolDecl struct {
	Decl    profile.Decl
	Dynamic bool
}

// CacheComposition is the parity package's view of one composed request's
// cache-discipline-relevant shape — the composed system blocks and tool
// declarations in request order, each labeled captured (stable prefix) or
// dynamic (volatile tail).
type CacheComposition struct {
	System []ProbeSystemBlock
	Tools  []ProbeToolDecl
}

// ProbeCheck is one named probe assertion with a human-readable verdict.
type ProbeCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// CacheProbeReport is the probe's outcome: every check run plus the AND of
// their OK flags.
type CacheProbeReport struct {
	Checks []ProbeCheck `json:"checks"`
	OK     bool         `json:"ok"`
}

// RunCacheProbe asserts the stable→volatile ordering discipline over one
// composed shape: within each list (system blocks, tools), every dynamic
// entry's index must be >= every captured entry's index. A violation is
// reported with the offending index pair NAMED — never a bare boolean — so
// the merge site that regressed is identifiable from the verdict alone.
// Empty or pure-capture compositions pass trivially (the empty-input edge).
func RunCacheProbe(comp *CacheComposition) CacheProbeReport {
	if comp == nil {
		comp = &CacheComposition{}
	}

	rep := CacheProbeReport{Checks: []ProbeCheck{
		systemOrderingCheck(comp.System),
		toolsOrderingCheck(comp.Tools),
	}}

	rep.OK = true

	for _, c := range rep.Checks {
		if !c.OK {
			rep.OK = false
		}
	}

	return rep
}

// systemOrderingCheck asserts the system[] stable prefix: captured blocks
// first, dynamically merged blocks (skills/agents listings, hook context)
// appended after.
func systemOrderingCheck(blocks []ProbeSystemBlock) ProbeCheck {
	labels, stable, dynamic := systemLabels(blocks)

	if dyn, cap, violated := orderingPair(labels); violated {
		return ProbeCheck{
			Name: checkSystemOrdering,
			OK:   false,
			Detail: fmt.Sprintf("dynamic system block at index %d precedes captured block at index %d — "+
				"stable→volatile ordering broken (cache prefix busted)", dyn, cap),
		}
	}

	return ProbeCheck{
		Name:   checkSystemOrdering,
		OK:     true,
		Detail: fmt.Sprintf("stable prefix %d, dynamic %d, ordering ok", stable, dynamic),
	}
}

// toolsOrderingCheck asserts the tools[] stable prefix: the captured catalog
// in captured order, runtime-bridged mcp__* decls appended after.
func toolsOrderingCheck(tools []ProbeToolDecl) ProbeCheck {
	labels := make([]bool, len(tools))
	stable, dynamic := 0, 0

	for i, t := range tools {
		labels[i] = t.Dynamic

		if t.Dynamic {
			dynamic++
		} else {
			stable++
		}
	}

	if dyn, cap, violated := orderingPair(labels); violated {
		return ProbeCheck{
			Name: checkToolsOrdering,
			OK:   false,
			Detail: fmt.Sprintf("dynamic tool at index %d (%q) precedes captured tool at index %d (%q) — "+
				"stable→volatile ordering broken (cache prefix busted)", dyn, tools[dyn].Decl.Name, cap, tools[cap].Decl.Name),
		}
	}

	return ProbeCheck{
		Name:   checkToolsOrdering,
		OK:     true,
		Detail: fmt.Sprintf("stable prefix %d, dynamic %d, ordering ok", stable, dynamic),
	}
}

// systemLabels extracts the dynamic-label slice and the stable/dynamic counts.
func systemLabels(blocks []ProbeSystemBlock) (labels []bool, stable, dynamic int) {
	labels = make([]bool, len(blocks))

	for i, b := range blocks {
		labels[i] = b.Dynamic

		if b.Dynamic {
			dynamic++
		} else {
			stable++
		}
	}

	return labels, stable, dynamic
}

// orderingPair finds the offending index pair for one list: the FIRST dynamic
// entry and the LAST captured entry. The ordering discipline is violated iff
// some dynamic entry precedes some captured entry, which is exactly
// firstDynamic < lastCaptured; the returned pair names the worst offender.
func orderingPair(labels []bool) (firstDynamic, lastCaptured int, violated bool) {
	firstDynamic, lastCaptured = -1, -1

	for i, d := range labels {
		if d {
			if firstDynamic < 0 {
				firstDynamic = i
			}
		} else {
			lastCaptured = i
		}
	}

	if firstDynamic >= 0 && lastCaptured >= 0 && firstDynamic < lastCaptured {
		return firstDynamic, lastCaptured, true
	}

	return 0, 0, false
}
