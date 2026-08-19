package parity_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// Test-literal constants (goconst discipline): block type, check names, and
// the placement class names the corpus pin vocabulary uses.
const (
	blockTypeText       = "text"
	checkSystemOrdering = "system-ordering"
	checkToolsOrdering  = "tools-ordering"
	classSystem         = "system"
	classTools          = "tools"
	classMessage        = "message"
)

// capturedBlock / dynamicBlock build probe-view system blocks: the captured
// stable prefix vs the runtime-merged volatile tail (skills/agents listings,
// hook context).
func capturedBlock(text string) parity.ProbeSystemBlock {
	return parity.ProbeSystemBlock{Block: profile.TextBlock{Type: blockTypeText, Text: text}}
}

func dynamicBlock(text string) parity.ProbeSystemBlock {
	return parity.ProbeSystemBlock{Block: profile.TextBlock{Type: blockTypeText, Text: text}, Dynamic: true}
}

// capturedTool / dynamicTool build probe-view tool declarations: the captured
// catalog vs the MCP host's appended mcp__<server>__<tool> decls.
func capturedTool(name string) parity.ProbeToolDecl {
	return parity.ProbeToolDecl{Decl: profile.Decl{Name: name}}
}

func dynamicTool(name string) parity.ProbeToolDecl {
	return parity.ProbeToolDecl{Decl: profile.Decl{Name: name}, Dynamic: true}
}

// checkByName returns the named check from a report (nil when absent).
func checkByName(rep parity.CacheProbeReport, name string) *parity.ProbeCheck {
	for i := range rep.Checks {
		if rep.Checks[i].Name == name {
			return &rep.Checks[i]
		}
	}

	return nil
}

// TestCacheProbe_OrderingViolationFails (Task 2, Test 4): a volatile block
// spliced BEFORE the last captured system block — the composition a merge
// site must never produce — fails the probe with the offending index pair
// named, not just a boolean.
func TestCacheProbe_OrderingViolationFails(t *testing.T) {
	t.Parallel()

	comp := parity.CacheComposition{
		System: []parity.ProbeSystemBlock{
			capturedBlock("captured-0"),
			dynamicBlock("skills listing (volatile)"),
			capturedBlock("captured-2"),
		},
		Tools: []parity.ProbeToolDecl{
			capturedTool("Read"),
			capturedTool("Bash"),
		},
	}

	rep := parity.RunCacheProbe(&comp)

	if rep.OK {
		t.Fatalf("mid-prefix splice must fail the probe, got OK with checks %+v", rep.Checks)
	}

	check := checkByName(rep, checkSystemOrdering)
	if check == nil {
		t.Fatalf("report missing system-ordering check: %+v", rep.Checks)
	}

	if check.OK {
		t.Errorf("system-ordering must be violated:\n%+v", check)
	}

	for _, want := range []string{"index 1", "index 2"} {
		if !strings.Contains(check.Detail, want) {
			t.Errorf("violation detail must name the offending pair (%q missing): %s", want, check.Detail)
		}
	}
}

// TestCacheProbe_ToolArraySpliceFails (Task 2, Test 5): an mcp__* tool before
// a captured tool violates the tools array's stable prefix; appended-after
// passes.
func TestCacheProbe_ToolArraySpliceFails(t *testing.T) {
	t.Parallel()

	spliced := parity.CacheComposition{
		System: []parity.ProbeSystemBlock{capturedBlock("captured-0")},
		Tools: []parity.ProbeToolDecl{
			dynamicTool("mcp__serena__read_file"),
			capturedTool("Read"),
		},
	}

	rep := parity.RunCacheProbe(&spliced)

	if rep.OK {
		t.Fatalf("mid-array mcp tool must fail the probe, got OK with checks %+v", rep.Checks)
	}

	check := checkByName(rep, checkToolsOrdering)
	if check == nil {
		t.Fatalf("report missing tools-ordering check: %+v", rep.Checks)
	}

	if check.OK {
		t.Errorf("tools-ordering must be violated:\n%+v", check)
	}

	for _, want := range []string{"mcp__serena__read_file", "Read", "index 0"} {
		if !strings.Contains(check.Detail, want) {
			t.Errorf("violation detail must name the spliced tool (%q missing): %s", want, check.Detail)
		}
	}

	appended := parity.CacheComposition{
		System: []parity.ProbeSystemBlock{capturedBlock("captured-0")},
		Tools: []parity.ProbeToolDecl{
			capturedTool("Read"),
			dynamicTool("mcp__serena__read_file"),
		},
	}

	appendedRep := parity.RunCacheProbe(&appended)
	if !appendedRep.OK {
		t.Errorf("appended-after mcp tool must pass:\n%+v", appendedRep.Checks)
	}
}

// TestCacheProbe_GreenCase (Task 2, Test 6): the shipped merge pattern —
// captured blocks first, dynamic blocks appended after; captured tools in
// captured order, mcp__* appended — passes with a report enumerating the
// checks run (stable prefix length, dynamic count, ordering ok).
func TestCacheProbe_GreenCase(t *testing.T) {
	t.Parallel()

	comp := parity.CacheComposition{
		System: []parity.ProbeSystemBlock{
			capturedBlock("captured-0"),
			capturedBlock("captured-1"),
			capturedBlock("captured-2"),
			dynamicBlock("skills listing (volatile)"),
			dynamicBlock("agents listing (volatile)"),
		},
		Tools: []parity.ProbeToolDecl{
			capturedTool("Read"),
			capturedTool("Bash"),
			capturedTool("Skill"),
			dynamicTool("mcp__serena__read_file"),
		},
	}

	rep := parity.RunCacheProbe(&comp)

	if !rep.OK {
		t.Fatalf("shipped append pattern must pass:\n%+v", rep.Checks)
	}

	for _, name := range []string{checkSystemOrdering, checkToolsOrdering} {
		check := checkByName(rep, name)
		if check == nil {
			t.Fatalf("report missing %s check: %+v", name, rep.Checks)
		}

		if !check.OK {
			t.Errorf("%s must be ok: %s", name, check.Detail)
		}
	}

	sysCheck := checkByName(rep, checkSystemOrdering)
	for _, want := range []string{"stable prefix 3", "dynamic 2", "ordering ok"} {
		if !strings.Contains(sysCheck.Detail, want) {
			t.Errorf("system-ordering detail must enumerate the facts (%q missing): %s", want, sysCheck.Detail)
		}
	}

	toolCheck := checkByName(rep, checkToolsOrdering)
	for _, want := range []string{"stable prefix 3", "dynamic 1", "ordering ok"} {
		if !strings.Contains(toolCheck.Detail, want) {
			t.Errorf("tools-ordering detail must enumerate the facts (%q missing): %s", want, toolCheck.Detail)
		}
	}
}

// TestCacheProbe_EmptyMerges (Task 2, Test 7): no dynamic merges at all — the
// empty-input edge must not fail on a pure-capture (or fully empty)
// composition.
func TestCacheProbe_EmptyMerges(t *testing.T) {
	t.Parallel()

	pureCapture := parity.CacheComposition{
		System: []parity.ProbeSystemBlock{capturedBlock("captured-0"), capturedBlock("captured-1")},
		Tools:  []parity.ProbeToolDecl{capturedTool("Read")},
	}

	if rep := parity.RunCacheProbe(&pureCapture); !rep.OK {
		t.Errorf("pure-capture composition must pass:\n%+v", rep.Checks)
	}

	bare := parity.CacheComposition{}
	if rep := parity.RunCacheProbe(&bare); !rep.OK {
		t.Errorf("empty composition must pass:\n%+v", rep.Checks)
	}
}

// scanCorpusPinFixture scans 14-02's committed cache-control fixture (the
// placement pin source — corpus-derived, REDACTED payloads with the structural
// cache_control placement intact) and returns its context-behavior census.
func scanCorpusPinFixture(t *testing.T) profile.ContextBehaviorReport {
	t.Helper()

	f, err := os.Open(filepath.Join("..", "profile", "testdata", "context-behavior", "cache-control.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	rep, err := profile.ScanContextBehavior(f)
	if err != nil {
		t.Fatal(err)
	}

	return rep
}

// TestCacheProbe_PlacementAgainstPin (Task 3, Test 8): the pin fixture's
// placement classes (14-02's ContextBehaviorReport over the committed
// fixture) are the expected set; a composition matching them passes; a
// composition carrying cache_control on a class the pin lacks, or missing one
// the pin has (14-03's routed emission outcome — the shaper emits none), fails
// with the delta named. No pi-derived expected values: the pin is DERIVED
// from the fixture inside the test; the one-class expectation below is the
// 14-02 corpus census fact (corpus wins), not a pi rule.
func TestCacheProbe_PlacementAgainstPin(t *testing.T) {
	t.Parallel()

	pin := parity.PinClasses(scanCorpusPinFixture(t))

	if len(pin) != 1 || !pin[classSystem] {
		t.Fatalf("pin must be exactly the system class (14-02 census: every system block, never elsewhere; "+
			"the fixture's synthetic classifier probes stay excluded), got %v", pin)
	}

	if pin[classTools] || pin[classMessage] {
		t.Fatalf("tools/message classes must be pin-absent (corpus shows zero placements there), got %v", pin)
	}

	// Matching composition — built FROM the derived pin, not hand-written.
	matching := parity.CacheComposition{
		System: []parity.ProbeSystemBlock{
			{Block: profile.TextBlock{Type: blockTypeText, Text: "s0"}, CacheControl: pin[classSystem]},
			{Block: profile.TextBlock{Type: blockTypeText, Text: "skills listing"}, Dynamic: true},
		},
		Tools: []parity.ProbeToolDecl{
			{Decl: profile.Decl{Name: "Read"}, CacheControl: pin[classTools]},
			{Decl: profile.Decl{Name: "mcp__serena__read_file"}, Dynamic: true},
		},
	}

	if check := parity.AssertPlacementAgainstPin(&matching, pin); !check.OK {
		t.Errorf("pin-matching composition must pass: %s", check.Detail)
	}

	assertDeltaNamed(t, pin, setToolCache(matching, true), classTools)
	assertDeltaNamed(t, pin, setMessageSites(matching, 1), classMessage)
	assertDeltaNamed(t, pin, setSystemCache(matching, false), classSystem)
}

// assertDeltaNamed asserts the placement check FAILS on a mutated composition
// with the named class in its delta.
func assertDeltaNamed(t *testing.T, pin map[string]bool, mutated parity.CacheComposition, wantClass string) {
	t.Helper()

	check := parity.AssertPlacementAgainstPin(&mutated, pin)
	if check.OK {
		t.Fatalf("mutation against the pin must fail (class %s): %+v", wantClass, mutated)
	}

	if !strings.Contains(check.Detail, wantClass) {
		t.Errorf("delta must name the %s class: %s", wantClass, check.Detail)
	}
}

func setToolCache(comp parity.CacheComposition, v bool) parity.CacheComposition {
	comp.Tools = append([]parity.ProbeToolDecl(nil), comp.Tools...)
	comp.Tools[0].CacheControl = v

	return comp
}

func setMessageSites(comp parity.CacheComposition, n int) parity.CacheComposition {
	comp.MessageCacheSites = n

	return comp
}

func setSystemCache(comp parity.CacheComposition, v bool) parity.CacheComposition {
	comp.System = append([]parity.ProbeSystemBlock(nil), comp.System...)
	comp.System[0].CacheControl = v

	return comp
}
