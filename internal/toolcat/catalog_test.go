package toolcat_test

import (
	"testing"

	"github.com/djarvur/ass-guard-agent/internal/toolcat"
)

// TestCatalog_GetCoreTools confirms the built-in catalog carries the core tools
// with faithful schemas + correct mutability classification.
func TestCatalog_GetCoreTools(t *testing.T) {
	c := toolcat.NewCatalog()
	for _, name := range []string{"Read", "Bash", "Write", "Edit", "TodoWrite", "WebSearch"} {
		tl, ok := c.Get(name)
		if !ok {
			t.Errorf("Get(%q) returned ok=false; want a built-in entry", name)
			continue
		}
		if len(tl.InputSchema) == 0 {
			t.Errorf("tool %q has empty InputSchema", name)
		}
	}
}

// TestCatalog_Mutability pins the mutability classification (Phase-2/4 record).
func TestCatalog_Mutability(t *testing.T) {
	c := toolcat.NewCatalog()
	readTool, _ := c.Get("Read")
	if readTool.IsMutating() {
		t.Error("Read classified mutating; want read-only")
	}
	for _, name := range []string{"Bash", "Write", "Edit"} {
		tl, _ := c.Get(name)
		if !tl.IsMutating() {
			t.Errorf("%s classified read-only; want mutating", name)
		}
	}
}

// TestCatalog_Nonexistent returns ok=false for an unknown tool.
func TestCatalog_Nonexistent(t *testing.T) {
	c := toolcat.NewCatalog()
	if _, ok := c.Get("DoesNotExist"); ok {
		t.Error("Get(DoesNotExist) returned ok=true; want false")
	}
}

// TestCatalog_Names covers the full built-in core set.
func TestCatalog_Names(t *testing.T) {
	c := toolcat.NewCatalog()
	names := c.Names()
	want := map[string]bool{
		"Read": false, "Bash": false, "Edit": false, "Write": false,
		"TodoWrite": false, "TodoRead": false, "WebSearch": false, "WebFetch": false,
		"Skill": false, "Agent": false, "AskUserQuestion": false,
	}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("core tool %q missing from catalog Names()", name)
		}
	}
	// D-16: the core set is ~19-20 tools (stable built-in core). MCP/plugin tools
	// are NOT in the built-in catalog.
	if len(names) < 15 {
		t.Errorf("catalog has %d names, want >= 15 core tools", len(names))
	}
}
