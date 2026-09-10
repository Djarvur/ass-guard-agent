package acpserve

// The 18-05 CommandSource adapter: the bridge from the acp seam to the
// runtime's discovered slash-command registry (08-04). internal/acp stays
// free of internal/runtime and internal/ecosys (the session_store.go 25-D-13
// composition-root convention); the adapter lives here, injected via
// acp.WithCommandSource.

import (
	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/kit/runtime"
)

// commandSourceAdapter adapts the runner's resolver CHAIN (20-01/CMDS-01 —
// builtins → skills → agents → file, winners-only) to the acp.CommandSource
// seam behind available_commands_update (18-05/ACP-06 "commands
// re-advertised"; 20-01's session-start advertisement + 20-05's rescan
// re-fire read the same surface). The advertisement IS the chain's winner
// projection — D-04's one-truth rule (what autocomplete shows is exactly
// what runs), sorted deterministically by the chain builder.
type commandSourceAdapter struct {
	runner *runtime.Runner
}

// AvailableCommands projects the chain winners onto the v1 AvailableCommand
// frames (name + description + optional input hint — the three
// consumer-facing fields a winner carries; 25-04: the runner hands back the
// kit-neutral runtime.CommandAd rows, this adapter builds the wire frames).
func (a commandSourceAdapter) AvailableCommands() []acp.AvailableCommandFrame {
	rows := a.runner.CommandAdvertisement()

	out := make([]acp.AvailableCommandFrame, 0, len(rows))

	for _, row := range rows {
		frame := acp.AvailableCommandFrame{Name: row.Name, Description: row.Description}
		if row.InputHint != "" {
			frame.Input = &acp.AvailableCommandInputFrame{Hint: row.InputHint}
		}

		out = append(out, frame)
	}

	return out
}
