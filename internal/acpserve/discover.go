// Post-25-06 kit wiring helpers (the composition root's app-side discovery
// home): the startup step that replaces the kit's LoadCommandRegistry —
// .claude/ discovery + precedence merging (internal/ecosys), the OpenSpec
// command-mutability table, and the catalog adapter the kit consumes. The
// graceful-degradation arms relocate VERBATIM: a discovery failure installs
// an EMPTY catalog (expansion off — turns proceed on plain text, never a
// stale registry), and a mutability load failure leaves boundaries off.
// Everything here lives at the WIRING SITE (the loadEngineSetup precedent,
// 25-05) — the kit packages stay free of app imports (D-17/D-19).

package acpserve

import (
	"log"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/openspec"
)

// loadCommandCatalog is the app-side discovery startup step (25-06 Task 2,
// D-17 — the relocated LoadCommandRegistry body): run .claude/ discovery,
// load the OpenSpec mutability table, and return the catalog adapter over
// the results. The composition calls this WHERE LoadCommandRegistry once ran
// and injects the result via runner.SetCatalog.
//
// Degradation arms (VERBATIM from the kit body this replaces):
//
//   - a discovery failure logs "command registry load failed (continuing
//     without slash expansion)" and returns an adapter over an EMPTY
//     registry — the kit's expansion no-ops and every turn proceeds on
//     plain text (T-8-16: an expansion problem NEVER becomes a turn failure
//     or an ACP error; never serve a stale registry);
//   - an openspec config failure logs "openspec config load failed
//     (continuing without command boundaries)" and leaves the mutability
//     table empty (no D-11 boundaries — the mutating-command context
//     boundary stays off).
func loadCommandCatalog(workDir string) *catalogAdapter {
	reg, servers, err := ecosys.Discover(workDir)
	if err != nil {
		// Drop to zero (do NOT serve a stale registry): the registry mirrors
		// the on-disk command tree, and an unreadable tree means expansion is
		// OFF — turns proceed on plain text (T-8-16).
		log.Printf("ass-guard: command registry load failed (continuing without slash expansion): %v", err)

		return newCatalogAdapter(workDir, ecosys.Registry{}, nil, nil)
	}

	mutability := map[string]string(nil)

	oscfg, cerr := openspec.DefaultConfig()
	if cerr != nil {
		log.Printf("ass-guard: openspec config load failed (continuing without command boundaries): %v", cerr)
	} else {
		mutability = oscfg.CommandMutability
	}

	return newCatalogAdapter(workDir, reg, servers, mutability)
}
