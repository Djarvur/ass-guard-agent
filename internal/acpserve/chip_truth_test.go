package acpserve //nolint:testpackage // internal package test

// The 16-09 chip==wire invariant (gap 4b), ADVERTISEMENT side: the bare
// `model` entry's currentValue is resolver-true by construction — it equals
// an INDEPENDENT modelrouting resolver evaluation over the same layer files,
// computed in this test. Together with the wire-side pin (internal/runtime's
// TestDefaultTurnModel_FollowsTierResolution, same phase) the pre-stamp chip
// divergence — operator-observed turn-001: GLM-5.3 on the wire while the chip
// showed glm-5.2 — cannot recur without failing a test.
//
// Deliberately a NEW file (no files_modified overlap with 16-08's
// config_surface.go/config_test.go); the layer-file fixture style is copied
// from config_test.go, whose helpers (writeLayer/optionByID/assertEight and
// the floor-slug constants) are reused in place.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
)

// TestConfigAdvertisement_ResolverTruth pins the advertisement to resolver
// truth: with temp project + global layer files, the bare model entry's
// CurrentValue equals an independent resolver evaluation over the same files
// (time-window-free fixture — no flaky boundary crossing).
func TestConfigAdvertisement_ResolverTruth(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	globalPath := filepath.Join(root, "home", ".config", "ass-guard-agent", "config.yaml")
	projectPath := filepath.Join(root, "proj", ".ass-guard", "config.yaml")

	// Both layers carry an explicit heavy binding: the project value wins the
	// combined view (glm-5.2 — the operator-observed chip value); the global
	// layer carries the floor's primary.
	writeLayer(t, globalPath, "tiers:\n  "+testTierHeavy+":\n    model: "+testModelPrimary+"\n")
	writeLayer(t, projectPath, "tiers:\n  "+testTierHeavy+":\n    model: "+testModelFallback+"\n")

	surface := NewConfigSurface(globalPath, projectPath, "anthropic", nil)

	opts := surface.Options()
	assertEight(t, opts, "resolver-truth advertisement")

	model := optionByID(t, opts, optModel)

	// The independent evaluation: the same files through the loader (global
	// under project — the surface's layerPaths order), then the resolver —
	// computed HERE, never by the surface under test.
	loaded, lerr := modelrouting.Load(globalPath, projectPath)
	if lerr != nil {
		t.Fatalf("load layer files for the independent evaluation: %v", lerr)
	}

	tgt, _, rerr := modelrouting.NewResolver(loaded).
		Resolve(loaded.SessionTier, "", time.Now(), modelrouting.CapabilityReq{})
	if rerr != nil {
		t.Fatalf("independent resolver evaluation: %v", rerr)
	}

	if model.CurrentValue != tgt.Model {
		t.Errorf("advertisement model currentValue = %q; want the independent resolver evaluation %q (chip==wire)",
			model.CurrentValue, tgt.Model)
	}

	// Non-vacuity anchor: the fixture's combined value is the project layer's
	// explicit binding — if both sides ever collapsed to "" the equality
	// above would pass for the wrong reason.
	if model.CurrentValue != testModelFallback {
		t.Errorf("advertisement model currentValue = %q; want the project layer's %q (fixture anchor)",
			model.CurrentValue, testModelFallback)
	}
}
