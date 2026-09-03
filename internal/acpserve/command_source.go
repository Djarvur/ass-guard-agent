package acpserve

// The 18-05 CommandSource adapter: the bridge from the acp seam to the
// runtime's discovered slash-command registry (08-04). internal/acp stays
// free of internal/runtime and internal/ecosys (the session_store.go 25-D-13
// composition-root convention); the adapter lives here, injected via
// acp.WithCommandSource.

import (
	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/runtime"
)

// commandSourceAdapter adapts the runner's ecosys command registry
// (Runner.CommandRegistry — LoadCommandRegistry's startup discovery) to the
// acp.CommandSource seam behind available_commands_update (18-05/ACP-06
// "commands re-advertised"; Phase 20's session-start advertisement reuses
// the same source). AllCommands is already name-sorted, so the wire order is
// deterministic.
type commandSourceAdapter struct {
	runner *runtime.Runner
}

// AvailableCommands projects the discovered commands onto the v1
// AvailableCommand frames (name + description — the two consumer-facing
// fields a discovered command carries).
func (a commandSourceAdapter) AvailableCommands() []acp.AvailableCommandFrame {
	cmds := a.runner.CommandRegistry().AllCommands()

	out := make([]acp.AvailableCommandFrame, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, acp.AvailableCommandFrame{Name: c.Name, Description: c.Description})
	}

	return out
}
