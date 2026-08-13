package toolcat

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
)

//go:embed coretools.json
var coreToolsJSON []byte

// Catalog is the built-in tool catalog (TOOL-01). It carries the stable built-in
// core tools with their faithful captured schemas and (stubbed) execution
// behavior. A zero Catalog is empty; use NewCatalog to load the built-in core.
type Catalog struct {
	tools map[string]Tool
}

// NewCatalog returns a Catalog populated with the built-in core tools (embedded
// in the binary, sourced faithfully from the captured zcode catalog).
func NewCatalog() *Catalog {
	c := &Catalog{tools: map[string]Tool{}}

	var entries []Tool

	err := json.Unmarshal(coreToolsJSON, &entries)
	if err != nil {
		panic(fmt.Sprintf("toolcat: embedded coretools.json failed to parse: %v", err))
	}

	for _, e := range entries {
		c.tools[e.Name] = e
	}

	return c
}

// Get returns the catalog entry for name and ok=false if not present.
func (c *Catalog) Get(name string) (Tool, bool) {
	t, ok := c.tools[name]

	return t, ok
}

// Register adds (or overwrites) a tool entry in the catalog. Phase-4 uses this
// to register dynamically-discovered tools — OpenSpec commands (Plan 04-02
// D-15), swappable backends, and learning-mode proposed tools — without
// rebuilding the embedded core. A registered mutating tool is reflected by
// IsBoundary via the existing mutability floor (no boundary-engine change).
func (c *Catalog) Register(t Tool) {
	if c.tools == nil {
		c.tools = map[string]Tool{}
	}

	c.tools[t.Name] = t
}

// Clone returns a shallow copy of the catalog. Phase 5 uses this to build a
// per-session catalog from the shared engine catalog so per-session MCP tools
// (D-16) never leak across sessions or back into the shared catalog.
func (c *Catalog) Clone() *Catalog {
	out := &Catalog{tools: make(map[string]Tool, len(c.tools))}
	maps.Copy(out.tools, c.tools)

	return out
}

// Names returns the sorted catalog tool names.
func (c *Catalog) Names() []string {
	out := make([]string, 0, len(c.tools))
	for n := range c.tools {
		out = append(out, n)
	}

	sort.Strings(out)

	return out
}

// Decls returns the catalog entries as model-facing Decl values.
func (c *Catalog) Decls() []Decl {
	out := make([]Decl, 0, len(c.tools))

	for _, n := range c.Names() {
		t := c.tools[n]
		out = append(out, Decl{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}

	return out
}
