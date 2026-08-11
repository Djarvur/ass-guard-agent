package toolcat

import (
	_ "embed"
	"encoding/json"
	"fmt"
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
	if err := json.Unmarshal(coreToolsJSON, &entries); err != nil {
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
