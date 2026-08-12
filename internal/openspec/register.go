package openspec

import (
	"errors"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// namespacedName returns the catalog registration name for an OpenSpec command:
// "openspec:" + command. OpenSpec tools are namespaced so they never collide
// with built-in tools (e.g. an OpenSpec "apply" is registered as
// "openspec:apply", distinct from any built-in "apply"). The prefix also lets
// IsBoundary + the engine attribute OpenSpec tool calls unambiguously.
func namespacedName(command string) string {
	return "openspec:" + command
}

// RegisterTools registers each declared OpenSpec command into the catalog with
// its declared mutability (D-15 / OPEN-03). A mutating OpenSpec command is a
// boundary via the EXISTING toolcat.IsBoundary floor — no boundary-engine
// change. The catalog entries carry no Execute impl (OpenSpec commands are
// invoked via the subprocess Adapter by the engine in 04-05, not the
// catalog-driven tool executor); the registration is solely for mutability +
// boundary classification.
func RegisterTools(catalog *toolcat.Catalog, cfg *OpenSpecConfig) error {
	if catalog == nil {
		return errors.New("openspec: RegisterTools requires a non-nil catalog")
	}

	if cfg == nil {
		return errors.New("openspec: RegisterTools requires a non-nil config")
	}

	for name, shape := range cfg.Commands {
		mut := toolcat.MutabilityReadOnly
		if shape.Mutability == "mutating" {
			mut = toolcat.MutabilityMutating
		}

		catalog.Register(toolcat.Tool{
			Name:        namespacedName(name),
			Description: "OpenSpec " + name + " command (hosted as a subprocess)",
			Mutability:  mut,
		})
	}

	return nil
}
