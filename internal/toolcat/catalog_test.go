package toolcat_test

import (
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// TestCatalog_GetCoreTools confirms the built-in catalog carries the core tools
// with faithful schemas + correct mutability classification.
func TestCatalog_GetCoreTools(t *testing.T) {
	t.Parallel()

	c := toolcat.NewCatalog()
	for _, name := range []string{toolRead, toolBash, toolWrite, toolEdit, "TodoWrite", "WebSearch"} {
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
	t.Parallel()

	c := toolcat.NewCatalog()

	readTool, _ := c.Get(toolRead)
	if readTool.IsMutating() {
		t.Error("Read classified mutating; want read-only")
	}

	for _, name := range []string{toolBash, toolWrite, toolEdit} {
		tl, _ := c.Get(name)
		if !tl.IsMutating() {
			t.Errorf("%s classified read-only; want mutating", name)
		}
	}
}

// TestCatalog_Nonexistent returns ok=false for an unknown tool.
func TestCatalog_Nonexistent(t *testing.T) {
	t.Parallel()

	c := toolcat.NewCatalog()
	if _, ok := c.Get("DoesNotExist"); ok {
		t.Error("Get(DoesNotExist) returned ok=true; want false")
	}
}

// TestCatalog_Register verifies Register adds (or overwrites) a tool entry, a
// registered mutating tool is reflected by IsBoundary via the existing
// mutability floor (no boundary-engine change — OPEN-03/Plan 04-02 relies on
// this), and a read-only registration stays read-only.
func TestCatalog_Register(t *testing.T) {
	t.Parallel()

	c := toolcat.NewCatalog()
	// Register a mutating OpenSpec command.
	c.Register(toolcat.Tool{Name: "openspec:apply", Mutability: toolcat.MutabilityMutating})
	// Register a read-only command.
	c.Register(toolcat.Tool{Name: "openspec:list", Mutability: toolcat.MutabilityReadOnly})

	apply, ok := c.Get("openspec:apply")
	if !ok {
		t.Fatal("Get(openspec:apply) returned ok=false after Register")
	}

	if !apply.IsMutating() {
		t.Error("openspec:apply classified read-only; want mutating")
	}
	// IsBoundary reflects the registered mutability with NO boundary-engine
	// change (the SESS-02 structural floor consults the catalog field).
	if !toolcat.IsBoundary("openspec:apply", c, nil) {
		t.Error("IsBoundary(openspec:apply) = false; want true (mutating ⇒ boundary)")
	}

	if toolcat.IsBoundary("openspec:list", c, nil) {
		t.Error("IsBoundary(openspec:list) = true; want false (read-only)")
	}

	// Overwrite: re-registering list as mutating flips it (operator override).
	c.Register(toolcat.Tool{Name: "openspec:list", Mutability: toolcat.MutabilityMutating})

	if !toolcat.IsBoundary("openspec:list", c, nil) {
		t.Error("IsBoundary(openspec:list) = false after overwrite; want true")
	}
}

// TestCatalog_Names covers the full built-in core set.
func TestCatalog_Names(t *testing.T) {
	t.Parallel()

	c := toolcat.NewCatalog()
	names := c.Names()

	want := map[string]bool{
		toolRead: false, toolBash: false, toolEdit: false, toolWrite: false,
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
