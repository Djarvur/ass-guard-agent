package runtime //nolint:testpackage // internal package test

// 25-05 Task 3 test twin (the 25-04 acp_adapter_shim_test.go precedent):
// the production engine-setup loader lives app-side (acpserve's
// loadEngineSetup — OQ4/D-17); this in-package twin mirrors its openspec
// loads so the relocated engine batteries keep calling SetupEngine with the
// same catalog + pattern table their assertions pin. Zero Test functions —
// the D-20 ledger holds. Kit test files may import app packages under the
// current gate scope (the 25-03 ride-along set; the D-19 gate-scope
// decision is 25-08/25-09's).

import (
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/learning"
	"github.com/Djarvur/ass-guard-agent/internal/openspec"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// testEngineSetup mirrors acpserve's loadEngineSetup: openspec defaults →
// the shared catalog with tools registered + the FromConfig-derived pattern
// table. Learned stays nil (tests that pin learning build their own store
// and inject it through the same EngineSetup field — ask_wiring's rig, the
// e2e Criterion4 battery).
func testEngineSetup(t *testing.T) EngineSetup {
	t.Helper()

	catalog := toolcat.NewCatalog()

	oscfg, err := openspec.DefaultConfig()
	if err != nil {
		t.Fatalf("openspec default config: %v", err)
	}

	err = openspec.RegisterTools(catalog, oscfg)
	if err != nil {
		t.Fatalf("openspec register tools: %v", err)
	}

	pt, err := openspec.FromConfig(oscfg)
	if err != nil {
		t.Fatalf("openspec pattern table: %v", err)
	}

	return EngineSetup{Catalog: catalog, PatternTable: pt}
}

// testLearned is the in-package twin of acpserve's kitLearned (the 25-04
// adapter-twin precedent): wraps the real store for the Learned port so
// batteries that pin learning behavior inject the same store the production
// adapter wraps — assertions keep hitting the concrete store.
type testLearned struct{ store *learning.Store }

func (a testLearned) Lookup(situation string) (string, bool) {
	e, ok := a.store.Lookup(situation)

	return e.Answer, ok
}

func (a testLearned) RecordCandidate(situation, answer, sourceTurnID string) error {
	err := a.store.RecordCandidate(situation, answer, sourceTurnID)

	return err //nolint:wrapcheck // thin delegation across the seam (the twin of kitLearned)
}

func (a testLearned) EntryCount() int {
	return len(a.store.List())
}
