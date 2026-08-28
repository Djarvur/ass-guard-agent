package acpserve //nolint:testpackage // internal package test (reuses config_test fixtures)

// 16-REVIEW CR-02 regression: the initialize _meta blob channel must reach the
// WIRE, not just the chip. 16-09 gap 4b pinned chip==wire for the LAYER path
// (TestConfigAdvertisement_ResolverTruth here + internal/runtime's
// TestDefaultTurnModel_FollowsTierResolution) but the blob path stayed open:
// the advertisement resolved currentValues through the in-memory blob overlay
// while the wire-side default (Runner.defaultTurnModel) reads only schedCfg —
// chip showed the fill, the provider request carried the tier-resolved floor.
// The fix stamps the runner through the blob hook (the acpserve.Run wiring
// under test here), exactly as the production composition wires it.

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/runtime"
)

// Wire/chunk literals local to this fixture (goconst hygiene).
const (
	testChunkText    = "text"
	testChunkDone    = "done"
	testStopEndTurn  = "end_turn"
	testBlobWireText = "hi"
)

// wireModelRecorder is a provider.Provider recording the profile model of every
// Stream call — the value the Shaper stamps on the real outgoing request (the
// wire side of chip==wire).
type wireModelRecorder struct {
	mu     sync.Mutex
	models []string
	seen   chan string
}

func newWireModelRecorder() *wireModelRecorder {
	return &wireModelRecorder{seen: make(chan string, 8)}
}

func (r *wireModelRecorder) Send(
	context.Context, *profile.Profile, []provider.Message,
) (provider.Response, error) {
	return provider.Response{FinishReason: testStopEndTurn}, nil
}

func (r *wireModelRecorder) Stream(
	_ context.Context, prof *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	r.mu.Lock()
	r.models = append(r.models, prof.Model)
	r.mu.Unlock()

	r.seen <- prof.Model

	ch := make(chan provider.StreamChunk, 1)

	ch <- provider.StreamChunk{Type: testChunkDone, FinishReason: testStopEndTurn}

	close(ch)

	return ch, nil
}

func (r *wireModelRecorder) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// nopChunkEmitter satisfies acp.ChunkEmitter for turns that stream no chunks.
type nopChunkEmitter struct{}

func (nopChunkEmitter) AgentMessageChunk(string, string) error { return nil }

// blobChipWireSchedCfg loads a temp scheduling config whose heavy tier binds
// GLM-5.3 on the anthropic provider — the tier-resolved default the wire
// (wrongly) carried before the fix, proving the fixture can produce the
// divergence the fix closes.
func blobChipWireSchedCfg(t *testing.T) *modelrouting.Config {
	t.Helper()

	path := filepath.Join(t.TempDir(), "scheduling.yaml")
	content := "models:\n  " + testModelPrimary + ":\n    provider: anthropic\n" +
		"tiers:\n  " + testTierHeavy + ":\n    model: " + testModelPrimary + "\n"

	writeLayer(t, path, content)

	cfg, err := modelrouting.Load(path)
	if err != nil {
		t.Fatalf("load scheduling config: %v", err)
	}

	return cfg
}

// newBlobWireRunner builds a real runtime Runner over the recording provider
// (the same composition acpserve.Run builds, minus the engine/cron extras the
// invariant does not touch).
func newBlobWireRunner(t *testing.T, rec *wireModelRecorder, schedCfg *modelrouting.Config) *runtime.Runner {
	t.Helper()

	return runtime.NewRunner(&runtime.RunnerConfig{
		Bus:     event.NewBus(),
		Profile: profile.Profile{Name: "test", Model: testModelPrimary},
		WorkDir: t.TempDir(),
		MaxConc: 2,
		MakeProvider: func(provider.RequestCapturer) provider.Provider {
			return rec
		},
		SchedCfg:     schedCfg,
		ProviderName: "anthropic",
		Stderr:       io.Discard,
	})
}

// wireModelOf drives one turn through the runner and returns the model the
// provider request carried.
func wireModelOf(t *testing.T, runner *runtime.Runner, rec *wireModelRecorder, sessionID string) string {
	t.Helper()

	_, err := runner.Run(context.Background(), sessionID, nopChunkEmitter{},
		[]acp.ContentBlock{{Type: testChunkText, Text: testBlobWireText}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case model := <-rec.seen:
		return model
	case <-time.After(5 * time.Second):
		t.Fatal("the turn's provider request never opened")

		return ""
	}
}

// TestBlobFillReachesTheWire pins the CR-02 fix end-to-end: an initialize _meta
// model fill over an unset slot shows on the chip AND rides the next provider
// request — with the tier-resolved config default (GLM-5.3) provably available
// as the pre-fix divergence value.
func TestBlobFillReachesTheWire(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// No layer files written: the surface's layer view is the embedded floor
	// and the blob fill owns the model slot (fills-unset, D-10).
	surface := NewConfigSurface(
		filepath.Join(root, "global.yaml"), filepath.Join(root, "project.yaml"), "anthropic", nil)

	rec := newWireModelRecorder()
	runner := newBlobWireRunner(t, rec, blobChipWireSchedCfg(t))

	// The production wiring under test (acpserve.Run).
	surface.SetBlobDefaultHook(runner.SetDefaultTurnModel)

	changed, aerr := surface.ApplyBlobDefaults(map[string]json.RawMessage{
		optModel: json.RawMessage(`"` + testModelFallback + `"`),
	})
	if aerr != nil {
		t.Fatalf("ApplyBlobDefaults: %v", aerr)
	}

	if !changed {
		t.Fatal("blob model fill over the unset slot did not report changed")
	}

	// CHIP: the advertisement carries the fill.
	chip := optionByID(t, surface.Options(), optModel).CurrentValue
	if chip != testModelFallback {
		t.Fatalf("advertisement model currentValue = %q; want the blob fill %q", chip, testModelFallback)
	}

	// WIRE: the very next request carries the SAME value. Pre-fix it carried
	// the tier-resolved GLM-5.3 — chip==wire broken on the blob path.
	wire := wireModelOf(t, runner, rec, "s-blob")
	if wire != chip {
		t.Errorf("chip=%q wire=%q — the _meta blob path broke chip==wire", chip, wire)
	}
}

// TestBlobFillYieldsToExplicitLayer pins the D-12 side of the seam: an
// explicit operator layer setting makes the blob fill INERT — nothing moves,
// the hook never fires, and the wire keeps the layer-backed value.
func TestBlobFillYieldsToExplicitLayer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	globalPath := filepath.Join(root, "global.yaml")
	projectPath := filepath.Join(root, "project.yaml")

	// The operator's project layer EXPLICITLY sets the model (D-10's
	// explicitness boundary: the blob never overrides operator config).
	writeLayer(t, projectPath, "tiers:\n  "+testTierHeavy+":\n    model: "+testModelPrimary+"\n")

	surface := NewConfigSurface(globalPath, projectPath, "anthropic", nil)

	rec := newWireModelRecorder()
	// schedCfg nil: the no-stamp wire default is the profile slug —
	// deliberately the same GLM-5.3 the layer resolves, so a wrongly-fired
	// hook (glm-5.2) shows up as a wire divergence.
	runner := newBlobWireRunner(t, rec, nil)

	surface.SetBlobDefaultHook(runner.SetDefaultTurnModel)

	changed, aerr := surface.ApplyBlobDefaults(map[string]json.RawMessage{
		optModel: json.RawMessage(`"` + testModelFallback + `"`),
	})
	if aerr != nil {
		t.Fatalf("ApplyBlobDefaults: %v", aerr)
	}

	if changed {
		t.Fatal("a blob fill over an explicitly-set layer slot reported changed (fills-unset broken)")
	}

	chip := optionByID(t, surface.Options(), optModel).CurrentValue
	if chip != testModelPrimary {
		t.Fatalf("advertisement model currentValue = %q; want the explicit layer's %q", chip, testModelPrimary)
	}

	wire := wireModelOf(t, runner, rec, "s-explicit")
	if wire != chip {
		t.Errorf("chip=%q wire=%q — an inert blob fill moved the wire (hook fired over an explicit layer)",
			chip, wire)
	}
}
