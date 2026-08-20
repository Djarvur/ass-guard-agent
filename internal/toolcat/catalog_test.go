package toolcat_test

import (
	"encoding/json"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// coreToolCount is the stable built-in core (D-16): 19 through Phase-11;
// 20 from 12-07 (CronUpdate joins the embedded core — the captured catalog
// always carried the quartet; the embedded copy had only three of the four).
const coreToolCount = 20

// 14-06 spot-check names (goconst: repeated literals).
const (
	nameTodoWrite = "TodoWrite"
	nameTodoRead  = "TodoRead"
	nameWebSearch = "WebSearch"
)

// TestCatalog_GetCoreTools confirms the built-in catalog carries the core tools
// with faithful schemas + correct mutability classification.
func TestCatalog_GetCoreTools(t *testing.T) {
	t.Parallel()

	c := toolcat.NewCatalog()
	for _, name := range []string{toolRead, toolBash, toolWrite, toolEdit, nameTodoWrite, nameWebSearch} {
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
		nameTodoWrite: false, nameTodoRead: false, nameWebSearch: false, "WebFetch": false,
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

// TestCatalogParsesContractAnnotations (14-06 test 3): every one of the 19
// coretools entries deserializes the contract annotations (timeout_ms,
// concurrency_safe, destructive) and the accessors return the declared values
// with the documented defaults (IsConcurrencySafe defaults to
// mutability==read-only when undeclared; IsDestructive defaults false).
func TestCatalogParsesContractAnnotations(t *testing.T) {
	t.Parallel()

	c := toolcat.NewCatalog()
	names := c.Names()

	if len(names) != coreToolCount {
		t.Fatalf("catalog carries %d tools; want the %d coretools", len(names), coreToolCount)
	}

	for _, n := range names {
		tl, ok := c.Get(n)
		if !ok {
			t.Fatalf("Get(%q) ok=false", n)
		}

		if tl.EffectiveTimeoutMS() <= 0 {
			t.Errorf("%s: EffectiveTimeoutMS() = %d; want a declared timeout_ms > 0", n, tl.EffectiveTimeoutMS())
		}

		// Undeclared concurrency_safe derives from mutability (read-only ⇒ true).
		want := tl.Mutability == toolcat.MutabilityReadOnly
		if tl.IsConcurrencySafe() != want && !declaredConcurrencySafe(n) {
			t.Errorf("%s: IsConcurrencySafe() = %v; want mutability default %v", n, tl.IsConcurrencySafe(), want)
		}
	}

	// Declared-value spot checks (concurrency_safe overrides the mutability
	// default where declared).
	if tl, _ := c.Get(nameTodoWrite); tl.IsConcurrencySafe() {
		t.Errorf("%s: IsConcurrencySafe() = true; want declared false (serialized session-state writer)", nameTodoWrite)
	}

	for _, n := range []string{toolRead, nameWebSearch, "WebFetch", nameTodoRead, "Skill", "CronList"} {
		if tl, _ := c.Get(n); !tl.IsConcurrencySafe() {
			t.Errorf("%s: IsConcurrencySafe() = false; want declared true", n)
		}
	}

	// The mutating trio stays alone-in-slot regardless of any declaration.
	for _, n := range []string{toolBash, toolWrite, toolEdit} {
		if tl, _ := c.Get(n); tl.IsConcurrencySafe() {
			t.Errorf("%s: IsConcurrencySafe() = true; want false (mutating)", n)
		}
	}
}

// TestCatalogDestructiveOnlyBash pins the destructive annotation: declared
// true ONLY on Bash (irreversible-by-nature mutation — 14-06's flags map).
func TestCatalogDestructiveOnlyBash(t *testing.T) {
	t.Parallel()

	c := toolcat.NewCatalog()

	for _, n := range c.Names() {
		tl, _ := c.Get(n)

		if n == toolBash {
			if !tl.IsDestructive() {
				t.Errorf("Bash: IsDestructive() = false; want true")
			}

			continue
		}

		if tl.IsDestructive() {
			t.Errorf("%s: IsDestructive() = true; want false (Bash only)", n)
		}
	}
}

// declaredConcurrencySafe lists the coretools carrying an EXPLICIT
// concurrency_safe declaration that intentionally overrides the mutability
// default (the read-only-but-serialized set — 14-06's flags consumption map).
func declaredConcurrencySafe(name string) bool {
	switch name {
	case nameTodoWrite, nameTodoRead, "CronCreate", "CronUpdate", "CronDelete", "CronList",
		"TaskStop", "SendMessage", "ExitPlanMode", "ReadSessionContext", "AskUserQuestion", "Agent":
		return true
	}

	return false
}

// TestCatalogRawCoretoolsAnnotations pins the RAW embedded JSON: every entry
// carries a timeout_ms annotation (the acceptance grep's programmatic twin).
func TestCatalogRawCoretoolsAnnotations(t *testing.T) {
	t.Parallel()

	var entries []toolcat.Tool

	err := json.Unmarshal(toolcat.CoreToolsJSON(), &entries)
	if err != nil {
		t.Fatalf("unmarshal coretools.json: %v", err)
	}

	if len(entries) != coreToolCount {
		t.Fatalf("coretools.json carries %d entries; want %d", len(entries), coreToolCount)
	}

	for _, e := range entries {
		if e.TimeoutMS <= 0 {
			t.Errorf("%s: coretools.json entry has no timeout_ms annotation", e.Name)
		}
	}
}
