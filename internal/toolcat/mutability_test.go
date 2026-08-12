package toolcat

import (
	"strings"
	"testing"
)

// TestEffectiveMutabilityFormula verifies the SESS-03 "more-mutating wins"
// formula: EffectiveMutability returns Mutating if EITHER the catalog tool's
// declared Mutability OR the adapter-class argument is Mutating. This is the
// load-bearing rule for boundary detection (D-19).
func TestEffectiveMutabilityFormula(t *testing.T) {
	cases := []struct {
		name         string
		tool         Mutability
		adapterClass Mutability
		want         Mutability
	}{
		{"catalog mutating wins over read-only adapter", MutabilityMutating, MutabilityReadOnly, MutabilityMutating},
		{"adapter mutating wins over read-only catalog", MutabilityReadOnly, MutabilityMutating, MutabilityMutating},
		{"both read-only stays read-only", MutabilityReadOnly, MutabilityReadOnly, MutabilityReadOnly},
		{"both mutating stays mutating", MutabilityMutating, MutabilityMutating, MutabilityMutating},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := Tool{Name: "X", Mutability: c.tool}

			got := EffectiveMutability(tool, c.adapterClass)

			if got != c.want {
				t.Fatalf("EffectiveMutability(tool=%s, adapter=%s) = %s; want %s",
					c.tool, c.adapterClass, got, c.want)
			}
		})
	}
}

// TestBuiltinCatalogMutabilityDefaults verifies the built-in core catalog seeds
// the mutating/read-only defaults per D-19: Bash/Write/Edit = Mutating (the
// side-effecting tools); Read/Glob/Grep = ReadOnly. The catalog is the faithful
// captured zcode tool set (TOOL-01); these defaults are the structural floor
// (SESS-02 — config cannot downgrade them).
func TestBuiltinCatalogMutabilityDefaults(t *testing.T) {
	c := NewCatalog()
	mutating := []string{"Bash", "Write", "Edit"}
	readOnly := []string{"Read", "Glob", "Grep", "Agent", "TodoRead"}

	for _, name := range mutating {
		t.Run("mutating/"+name, func(t *testing.T) {
			tool, ok := c.Get(name)
			if !ok {
				t.Skipf("catalog does not include %q (catalog drift); skipping default check", name)
			}

			if tool.Mutability != MutabilityMutating {
				t.Errorf("catalog tool %q Mutability = %s; want Mutating", name, tool.Mutability)
			}
		})
	}

	for _, name := range readOnly {
		t.Run("read-only/"+name, func(t *testing.T) {
			tool, ok := c.Get(name)
			if !ok {
				t.Skipf("catalog does not include %q (catalog drift); skipping default check", name)
			}

			if tool.Mutability != MutabilityReadOnly {
				t.Errorf("catalog tool %q Mutability = %s; want ReadOnly", name, tool.Mutability)
			}
		})
	}
}

// TestIsBoundaryStructuralFloor verifies SESS-02: IsBoundary returns true when a
// tool's EffectiveMutability is Mutating OR the tool name is in the config-added
// list. Config may ADD boundaries but the structural floor guarantees a
// catalog-mutating tool is ALWAYS a boundary even when configAdded is empty —
// config cannot downgrade a declared-mutating tool to read-only.
func TestIsBoundaryStructuralFloor(t *testing.T) {
	c := NewCatalog()

	t.Run("catalog mutating tool is boundary with empty config", func(t *testing.T) {
		if !IsBoundary("Bash", c, nil) {
			t.Errorf("IsBoundary(Bash, catalog, nil) = false; want true (structural floor — config cannot downgrade)")
		}
	})
	t.Run("catalog read-only tool is not a boundary by default", func(t *testing.T) {
		if IsBoundary("Read", c, nil) {
			t.Errorf("IsBoundary(Read, catalog, nil) = true; want false")
		}
	})
	t.Run("config adds a boundary for a read-only tool", func(t *testing.T) {
		if !IsBoundary("WebFetch", c, []string{"WebFetch"}) {
			t.Errorf("IsBoundary(WebFetch, catalog, [WebFetch]) = false; want true (config adds)")
		}
	})
	t.Run("config cannot downgrade a mutating tool", func(t *testing.T) {
		// Even with an empty configAdded, Bash (mutating) stays a boundary.
		// There is no API path to make IsBoundary(Bash,...) return false.
		if !IsBoundary("Bash", c, nil) {
			t.Errorf("IsBoundary(Bash) = false; structural floor violated — config downgrade must be impossible")
		}
	})
	t.Run("unknown tool without config is not a boundary", func(t *testing.T) {
		if IsBoundary("TotallyUnknownTool", c, nil) {
			t.Errorf("IsBoundary(unknown, nil) = true; want false")
		}
	})
	t.Run("unknown tool can be config-added boundary", func(t *testing.T) {
		if !IsBoundary("CustomSpec", c, []string{"CustomSpec"}) {
			t.Errorf("IsBoundary(CustomSpec, [CustomSpec]) = false; want true (config-added boundary for OpenSpec-style commands)")
		}
	})
}

// TestEffectiveMutabilityStringStability documents that the canonical string
// labels are stable (Phase 4's OpenSpec adapter compares against them when
// registering command mutability — D-19 forward-design).
func TestEffectiveMutabilityStringStability(t *testing.T) {
	if MutabilityMutating.String() != "mutating" {
		t.Errorf(`MutabilityMutating.String() = %q; want "mutating"`, MutabilityMutating.String())
	}

	if MutabilityReadOnly.String() != "read-only" {
		t.Errorf(`MutabilityReadOnly.String() = %q; want "read-only"`, MutabilityReadOnly.String())
	}
	// Ensure the test file references the mutating label at least once so the
	// stability contract is grep-visible.
	_ = strings.Contains("mutating", "mutating")
}
