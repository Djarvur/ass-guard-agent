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
	"sort"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// Probe check names (footer/report-stable identifiers).
const (
	checkSystemOrdering = "system-ordering"
	checkToolsOrdering  = "tools-ordering"
	// CheckPlacementVsPin is exported: the wiring site (runParity) appends the
	// placement check — or its explicit skip note — under this stable name.
	CheckPlacementVsPin = "placement-vs-pin"
)

// Placement site classes — the class-level reduction of 14-02's census keys
// ("system:index=N" → system, "tools:index=N" → tools,
// "message:role=R:block=T" → message).
const (
	classSystem  = "system"
	classTools   = "tools"
	classMessage = "message"
)

// ProbeSystemBlock is the probe's view of one composed system[] entry: the
// profile's TextBlock (captured or runtime-merged) labeled by WHO produced
// it. Dynamic=true marks a runtime merge (skills/agents listing, hook
// context) — the caller labels what it merged. CacheControl states whether
// the composed request carries cache_control on this entry.
type ProbeSystemBlock struct {
	Block        profile.TextBlock
	Dynamic      bool
	CacheControl bool
}

// ProbeToolDecl is the probe's view of one composed tools[] declaration:
// the profile's Decl labeled captured vs runtime-appended
// (mcp__<server>__<tool>). CacheControl states whether the composed
// request carries cache_control on this declaration.
type ProbeToolDecl struct {
	Decl         profile.Decl
	Dynamic      bool
	CacheControl bool
}

// CacheComposition is the parity package's view of one composed request's
// cache-discipline-relevant shape — the composed system blocks and tool
// declarations in request order, each labeled captured (stable prefix) or
// dynamic (volatile tail). MessageCacheSites counts cache_control sites on
// request.messages content blocks (any role) — the pin's message class.
type CacheComposition struct {
	System            []ProbeSystemBlock
	Tools             []ProbeToolDecl
	MessageCacheSites int
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
	census := censusList(len(blocks), func(i int) bool { return blocks[i].Dynamic })

	pair, violated := orderingPair(census.labels)
	if violated {
		return ProbeCheck{
			Name: checkSystemOrdering,
			OK:   false,
			Detail: fmt.Sprintf("dynamic system block at index %d precedes captured block at index %d — "+
				"stable→volatile ordering broken (cache prefix busted)", pair.dynamic, pair.captured),
		}
	}

	return ProbeCheck{
		Name:   checkSystemOrdering,
		OK:     true,
		Detail: fmt.Sprintf("stable prefix %d, dynamic %d, ordering ok", census.stable, census.dynamic),
	}
}

// toolsOrderingCheck asserts the tools[] stable prefix: the captured catalog
// in captured order, runtime-bridged mcp__* decls appended after.
func toolsOrderingCheck(tools []ProbeToolDecl) ProbeCheck {
	census := censusList(len(tools), func(i int) bool { return tools[i].Dynamic })

	pair, violated := orderingPair(census.labels)
	if violated {
		return ProbeCheck{
			Name: checkToolsOrdering,
			OK:   false,
			Detail: fmt.Sprintf("dynamic tool at index %d (%q) precedes captured tool at index %d (%q) — "+
				"stable→volatile ordering broken (cache prefix busted)",
				pair.dynamic, tools[pair.dynamic].Decl.Name, pair.captured, tools[pair.captured].Decl.Name),
		}
	}

	return ProbeCheck{
		Name:   checkToolsOrdering,
		OK:     true,
		Detail: fmt.Sprintf("stable prefix %d, dynamic %d, ordering ok", census.stable, census.dynamic),
	}
}

// listCensus is one composed list's dynamic-label slice plus its stable and
// dynamic entry counts.
type listCensus struct {
	labels  []bool
	stable  int
	dynamic int
}

// censusList builds the census off the caller's dynamic predicate.
func censusList(n int, dynamicAt func(i int) bool) listCensus {
	census := listCensus{labels: make([]bool, n)}

	for i := range n {
		census.labels[i] = dynamicAt(i)

		if census.labels[i] {
			census.dynamic++
		} else {
			census.stable++
		}
	}

	return census
}

// offendingPair names the (dynamic, captured) index pair of an ordering
// violation.
type offendingPair struct {
	dynamic  int
	captured int
}

// orderingPair finds the offending index pair for one list: the FIRST dynamic
// entry and the LAST captured entry. The ordering discipline is violated iff
// some dynamic entry precedes some captured entry, which is exactly
// firstDynamic < lastCaptured; the returned pair names the worst offender.
func orderingPair(labels []bool) (offendingPair, bool) {
	firstDynamic, lastCaptured := -1, -1

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
		return offendingPair{dynamic: firstDynamic, captured: lastCaptured}, true
	}

	return offendingPair{}, false
}

// PinClasses reduces a 14-02 context-behavior census to the class-level
// placement pin: the site classes where the captured target carries
// cache_control. A class is pinned iff its census placements are at least the
// scanned-record count — the target exhibits the class in EVERY request,
// matching the corpus fact "every system block (910/910)". Classes below
// that bar are corpus-absent for pin purposes: the committed fixture's
// self-documented SYNTHETIC classifier probes (single-occurrence tools/message
// placements that exist to exercise the scanner, not to represent the corpus)
// never clear it — corpus wins on conflict (locked prohibition).
func PinClasses(rep profile.ContextBehaviorReport) map[string]bool {
	pin := map[string]bool{}
	if rep.ScannedLines <= 0 {
		return pin
	}

	counts := map[string]int{}

	for key, n := range rep.CacheControlPlacements {
		class, _, _ := strings.Cut(key, ":")
		counts[class] += n
	}

	for class, n := range counts {
		if n >= rep.ScannedLines {
			pin[class] = true
		}
	}

	return pin
}

// placementClasses reduces the composition to the site classes its composed
// request carries cache_control on.
func (comp *CacheComposition) placementClasses() map[string]bool {
	classes := map[string]bool{}

	for _, b := range comp.System {
		if b.CacheControl {
			classes[classSystem] = true
		}
	}

	for _, t := range comp.Tools {
		if t.CacheControl {
			classes[classTools] = true
		}
	}

	if comp.MessageCacheSites > 0 {
		classes[classMessage] = true
	}

	return classes
}

// AssertPlacementAgainstPin compares the composed request's cache_control
// site classes against the corpus pin — a BIDIRECTIONAL set difference: a
// class the pin has that the composition lacks (e.g. today's routed emission
// gap: system) and a class the composition has that the pin lacks (pi-style
// tool/history breakpoints the corpus never exhibits) are BOTH reported with
// the delta named. The pin's authority is the corpus (see the file doc
// comment); an empty pin asserts the composition carries no cache_control
// anywhere.
func AssertPlacementAgainstPin(comp *CacheComposition, pin map[string]bool) ProbeCheck {
	if comp == nil {
		comp = &CacheComposition{}
	}

	composed := comp.placementClasses()

	var missing, extra []string

	for class := range pin {
		if !composed[class] {
			missing = append(missing, class)
		}
	}

	for class := range composed {
		if !pin[class] {
			extra = append(extra, class)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) == 0 && len(extra) == 0 {
		detail := "cache_control sites match the corpus pin"
		if len(pin) == 0 {
			detail = "corpus pin carries no class; composition agrees (no cache_control anywhere)"
		}

		return ProbeCheck{Name: CheckPlacementVsPin, OK: true, Detail: detail}
	}

	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "pin-has-composed-lacks ["+strings.Join(missing, ", ")+"]")
	}

	if len(extra) > 0 {
		parts = append(parts, "composed-has-pin-lacks ["+strings.Join(extra, ", ")+"]")
	}

	return ProbeCheck{
		Name:   CheckPlacementVsPin,
		OK:     false,
		Detail: strings.Join(parts, "; ") + " (corpus pin: 14-02 fixture)",
	}
}

// Fact renders the report's one-line footer fact: every check's name and
// detail joined — a FAIL status plus these deltas names the regression and
// its offending indices/classes from the verdict line alone.
func (r CacheProbeReport) Fact() string {
	parts := make([]string, 0, len(r.Checks))

	for _, c := range r.Checks {
		parts = append(parts, c.Name+": "+c.Detail)
	}

	return strings.Join(parts, "; ")
}
