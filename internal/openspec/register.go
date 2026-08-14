package openspec

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

var (
	errRegisterToolsNilCatalog = errors.New("openspec: RegisterTools requires a non-nil catalog")
	errRegisterToolsNilConfig  = errors.New("openspec: RegisterTools requires a non-nil config")
)

// namespacedName returns the catalog registration name for an OpenSpec command:
// "openspec:" + command. OpenSpec tools are namespaced so they never collide
// with built-in tools (e.g. an OpenSpec "archive" is registered as
// "openspec:archive", distinct from any built-in "archive"). The prefix also
// lets IsBoundary + the engine attribute OpenSpec tool calls unambiguously.
func namespacedName(command string) string {
	return "openspec:" + command
}

// argsInputSchema is the minimal model-facing call shape: {"args": [...]}
// forwarded verbatim after the entry's argv prefix.
const argsInputSchema = `{"type":"object","properties":{"args":{"type":"array","items":{"type":"string"}}}}`

// RegisterTools registers each declared OpenSpec command into the catalog with
// its declared mutability (D-15 / OPEN-03 — a mutating OpenSpec command is a
// boundary via the EXISTING toolcat.IsBoundary floor, no boundary-engine
// change) AND an Adapter-backed Execute closure (Phase-8 CMD-03): the model
// invoking openspec:* runs the real subprocess under the non-interactive
// guards and receives the structured result {stdout, stderr, exit_code,
// classification} — the "no implementation yet" path is dead for openspec
// tools, and command-level failures are structured results, never Go errors
// (D-10).
func RegisterTools(catalog *toolcat.Catalog, cfg *OpenSpecConfig) error {
	if catalog == nil {
		return errRegisterToolsNilCatalog
	}

	if cfg == nil {
		return errRegisterToolsNilConfig
	}

	adapter := &Adapter{}

	for name, shape := range cfg.Commands {
		mut := toolcat.MutabilityReadOnly
		if shape.Mutability == MutabilityMutating {
			mut = toolcat.MutabilityMutating
		}

		catalog.Register(toolcat.Tool{
			Name:        namespacedName(name),
			Description: "OpenSpec " + name + " command (hosted as a subprocess)",
			InputSchema: json.RawMessage(argsInputSchema),
			Mutability:  mut,
			Execute:     executeClosure(adapter, shape),
		})
	}

	return nil
}

// executeClosure builds the per-entry Execute closure: parse {"args":[…]}
// input (best-effort; empty input → no args), run the subprocess under the
// entry's timeout + guards, marshal the structured result. ALL outcome classes
// return (json.RawMessage, nil) — structure over error (D-10).
func executeClosure(adapter *Adapter, shape CommandShape) toolcat.Stub {
	return func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		var in struct {
			Args []string `json:"args"`
		}

		if len(input) > 0 {
			_ = json.Unmarshal(input, &in) //nolint:err113-best-effort // best-effort parse; empty args on garbage
		}

		res := adapter.RunGuarded(ctx, shape.TimeoutSecs, shape.ExitClass, shape.Argv, in.Args...)

		out, err := json.Marshal(res)
		if err != nil {
			return nil, err //nolint:wrapcheck // marshaling a plain struct
		}

		return out, nil
	}
}
