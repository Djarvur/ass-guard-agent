// Post-25-05 kit wiring helpers (the composition root's app-side loader
// home): the startup helper that loads the app-hosted ecosystem values
// SetupEngine consumes (OQ4/D-17 — openspec config, tool registration, the
// derived pattern table, the learning store) and the adapter that bridges
// the learning store to the kit's Learned port. Everything here lives at
// the WIRING SITE (the checkpointerAdapter/OnClose precedent) — the kit
// packages stay free of app imports.

package acpserve

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/Djarvur/ass-guard-agent/internal/learning"
	"github.com/Djarvur/ass-guard-agent/internal/openspec"
	"github.com/Djarvur/ass-guard-agent/kit/runtime"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// loadEngineSetup builds SetupEngine's inputs (25-05 Task 3, OQ4): the
// shared catalog with the OpenSpec tools registered, the FromConfig-derived
// pattern table, and the learning store wrapped in the port adapter — the
// exact loads SetupEngine self-performed before 25-05, in the exact order,
// with the exact degradation semantics. The composition root calls this
// immediately BEFORE runner.SetupEngine; a returned error degrades to
// engine-off exactly as a SetupEngine failure always did (serve continues,
// D-04/15-D-04). The learning open failure stays a LOG-AND-CONTINUE inside
// the helper (Learned nil) — never a returned error, exactly as before.
func loadEngineSetup(workDir string) (runtime.EngineSetup, error) {
	catalog := toolcat.NewCatalog()

	// OpenSpec config (D-13/D-15) — embedded default; an operator overlay
	// path could be loaded here. Register each command's mutability into the
	// catalog (D-17: the app registers tools; the kit owns the catalog type).
	oscfg, err := openspec.DefaultConfig()
	if err != nil {
		return runtime.EngineSetup{}, fmt.Errorf("call: %w", err)
	}

	err = openspec.RegisterTools(catalog, oscfg)
	if err != nil {
		return runtime.EngineSetup{}, fmt.Errorf("call: %w", err)
	}

	pt, err := openspec.FromConfig(oscfg)
	if err != nil {
		return runtime.EngineSetup{}, fmt.Errorf("call: %w", err)
	}

	setup := runtime.EngineSetup{Catalog: catalog, PatternTable: pt}

	// Learning store (LRN-01..04) — versioned .ass-guard/learned.yaml.
	learnedPath := filepath.Join(workDir, ".ass-guard", "learned.yaml")

	learned, lerr := learning.Open(learnedPath)
	if lerr == nil {
		setup.Learned = kitLearned{store: learned}
	} else {
		log.Printf("ass-guard: learning store open failed (continuing without learning): %v", lerr)
	}

	return setup, nil
}

// kitLearned adapts the app learning store to the kit's Learned port
// (25-05 Tasks 2+3): Lookup forwards the stored entry's Answer VERBATIM in
// the (string, bool) shape the engine consumes — learning.Entry never
// crosses the boundary (T-25-24 accept: read-only forwarding, audit paths
// unaffected). RecordCandidate is the 17-04 A9 accepted-answer write;
// EntryCount feeds the /memory listing. A nil store is never wrapped: the
// port stays nil and the kit degrades to engine.ErrAskPending / the
// no-persist skip / "not loaded" — the documented nil states.
type kitLearned struct{ store *learning.Store }

func (a kitLearned) Lookup(situation string) (string, bool) {
	e, ok := a.store.Lookup(situation)

	return e.Answer, ok
}

func (a kitLearned) RecordCandidate(situation, answer, sourceTurnID string) error {
	err := a.store.RecordCandidate(situation, answer, sourceTurnID)

	return err //nolint:wrapcheck // thin delegation across the seam
}

func (a kitLearned) EntryCount() int {
	return len(a.store.List())
}
