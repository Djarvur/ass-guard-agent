package runtime //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// The 16-05 live-apply seam tests (ACP-08): ApplyTurnModel changes the model
// of the very next provider request, a mid-turn Set waits for the in-flight
// turn (no torn stamp), and sessions created after the change stamp the
// effective model at construction.

// Repeated literals (goconst).
const (
	testProfileName = "test"
	testModelBefore = "model-a"
	testModelAfter  = "model-b"
)

// nopEmitter satisfies acp.ChunkEmitter for turns that stream no chunks.
type nopEmitter struct{}

func (nopEmitter) AgentMessageChunk(string, string) error { return nil }

// gatedStreamProvider records the profile model of every Stream (what the
// Shaper would stamp onto the real request) and holds the FIRST request open
// until the test releases it — the controllable in-flight request.
type gatedStreamProvider struct {
	mu           sync.Mutex
	models       []string
	seen         chan string   // one entry per Stream call, in order
	releaseFirst chan struct{} // closed by the test to free the held request
	heldOnce     sync.Once
}

func newGatedStreamProvider() *gatedStreamProvider {
	return &gatedStreamProvider{
		seen:         make(chan string, 8),
		releaseFirst: make(chan struct{}),
	}
}

func (p *gatedStreamProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{FinishReason: stopEndTurn}, nil
}

func (p *gatedStreamProvider) Stream(
	ctx context.Context, prof *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.mu.Lock()
	first := len(p.models) == 0
	p.models = append(p.models, prof.Model)
	p.mu.Unlock()

	p.seen <- prof.Model

	if first {
		p.heldOnce.Do(func() {})

		select {
		case <-p.releaseFirst:
		case <-ctx.Done():
			return nil, ctx.Err() //nolint:wrapcheck // test fake passthrough
		}
	}

	ch := make(chan provider.StreamChunk, 1)

	ch <- provider.StreamChunk{Type: chunkDone, FinishReason: stopEndTurn}

	close(ch)

	return ch, nil
}

func (p *gatedStreamProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// SupportsImages: the fake is text-only (21-05 D-11 seam stub).
func (p *gatedStreamProvider) SupportsImages() bool { return false }

func TestApplyTurnModel(t *testing.T) { //nolint:paralleltest // drives a background turn with real timing
	gated := newGatedStreamProvider()
	runner := newModelTestRunner(t, gated, testModelBefore)

	ctx := context.Background()

	// Sessions stamp the profile's model at construction before any change.
	sess1 := runner.sessionFor(ctx, "s1")
	if sess1.Profile.Model != testModelBefore {
		t.Fatalf("initial session model = %q; want the profile's model", sess1.Profile.Model)
	}

	turnDone := startTestTurn(t, runner, "s1", ctx)

	// Wait until the in-flight request is open (its model recorded).
	awaitSeenModel(t, gated, testModelBefore)

	// A Set arriving MID-TURN must wait: the in-flight request keeps its model.
	applyDone := make(chan error, 1)

	go func() { applyDone <- runner.ApplyTurnModel(testModelAfter) }()

	time.Sleep(150 * time.Millisecond)

	if got := sess1.Profile.Model; got != testModelBefore {
		t.Errorf("mid-turn Set tore the live model: %q (want %q until the turn ends)", got, testModelBefore)
	}

	select {
	case err := <-applyDone:
		t.Fatalf("ApplyTurnModel returned mid-turn (err=%v); want it waiting on the turn mutex", err)
	default:
	}

	// The turn ends; the serialized apply lands BETWEEN turns.
	close(gated.releaseFirst)
	<-turnDone

	select {
	case err := <-applyDone:
		if err != nil {
			t.Fatalf("ApplyTurnModel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ApplyTurnModel never returned after the turn ended")
	}

	if got := sess1.Profile.Model; got != testModelAfter {
		t.Errorf("session model after apply = %q; want %q", got, testModelAfter)
	}

	// The very next request on the SAME session carries the new model.
	_, _ = runner.Run(ctx, "s1", nopEmitter{}, []acp.ContentBlock{{Type: blockText, Text: "again"}})

	awaitSeenModel(t, gated, testModelAfter)
}

// newModelTestRunner builds a Runner wired to the gated provider.
func newModelTestRunner(t *testing.T, gated *gatedStreamProvider, model string) *Runner {
	t.Helper()

	return &Runner{
		bus:     event.NewBus(),
		profile: profile.Profile{Name: testProfileName, Model: model},
		workDir: t.TempDir(),
		maxConc: 2,
		makeProvider: func(provider.RequestCapturer) provider.Provider {
			return gated
		},
	}
}

// startTestTurn drives one background prompt turn and returns its done signal.
//
//nolint:revive // test helper keeps the caller's arg order
func startTestTurn(t *testing.T, runner *Runner, sessionID string, ctx context.Context) chan struct{} {
	t.Helper()

	turnDone := make(chan struct{})

	go func() {
		defer close(turnDone)

		_, _ = runner.Run(ctx, sessionID, nopEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "hi"}})
	}()

	return turnDone
}

// awaitSeenModel waits for the provider's next recorded request model and
// asserts it carries the live-applied value.
func awaitSeenModel(t *testing.T, gated *gatedStreamProvider, want string) {
	t.Helper()

	select {
	case got := <-gated.seen:
		if got != want {
			t.Errorf("next request model = %q; want %q (the live apply reached the wire)", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second turn's request never opened")
	}
}

// awaitSeenModelWhy is awaitSeenModel with a contract-specific failure note
// (kept separate so the 16-05 tests above stay byte-identical).
func awaitSeenModelWhy(t *testing.T, gated *gatedStreamProvider, want, why string) {
	t.Helper()

	select {
	case got := <-gated.seen:
		if got != want {
			t.Errorf("next request model = %q; want %q (%s)", got, want, why)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the request never opened")
	}
}

// 16-09 gap-4b fixtures (chip==wire): the profile slug the wire would carry
// pre-fix, the config-resolved tier default, the explicit editor stamp, and
// the static binding a resolver decline must fall back to.
const (
	testProfileSlug     = "model-profile-slug"
	testModelFromConfig = "model-from-config"
	testModelExplicit   = "model-explicit"
	testModelStaticBind = "model-static-binding"
	testTierHeavy       = "heavy"
)

// tierDefaultTestConfig loads a temp scheduling config whose heavy tier binds
// testModelFromConfig (declared on the embedded floor's anthropic provider so
// the loader's validation passes) — the config side of the chip==wire
// invariant.
func tierDefaultTestConfig(t *testing.T) *modelrouting.Config {
	t.Helper()

	path := filepath.Join(t.TempDir(), "scheduling.yaml")
	content := "models:\n  " + testModelFromConfig + ":\n    provider: anthropic\n" +
		"tiers:\n  " + testTierHeavy + ":\n    model: " + testModelFromConfig + "\n"

	werr := os.WriteFile(path, []byte(content), 0o600)
	if werr != nil {
		t.Fatalf("write scheduling config: %v", werr)
	}

	cfg, err := modelrouting.Load(path)
	if err != nil {
		t.Fatalf("load scheduling config: %v", err)
	}

	return cfg
}

// TestDefaultTurnModel_FollowsTierResolution pins the wire side of the 16-09
// chip==wire invariant (gap 4b): with NO editor stamp the model on the wire is
// the tier-resolved config model — the same value the ACP-08 advertisement
// displays — while an explicit editor stamp keeps absolute precedence (D-12),
// a nil schedCfg keeps the documented profile-slug default, and a resolver
// decline falls back to the static binding exactly like the advertisement's
// resolveModelLocked.
func TestDefaultTurnModel_FollowsTierResolution(t *testing.T) {
	t.Parallel()

	t.Run("default-follows-tier-resolution", func(t *testing.T) {
		t.Parallel()

		defaultTurnTierResolutionSubtest(t)
	})

	t.Run("explicit-stamp-wins", func(t *testing.T) {
		t.Parallel()

		defaultTurnExplicitStampSubtest(t)
	})

	t.Run("nil-config-keeps-profile-slug", func(t *testing.T) {
		t.Parallel()

		defaultTurnNilConfigSubtest(t)
	})

	t.Run("resolver-decline-falls-back-to-static-binding", func(t *testing.T) {
		t.Parallel()

		defaultTurnResolverDeclineSubtest(t)
	})
}

// defaultTurnTierResolutionSubtest: with no editor stamp and a loaded
// schedCfg, the tier-resolved config model rides out — not the profile slug
// (the operator-observed turn-001 divergence).
func defaultTurnTierResolutionSubtest(t *testing.T) {
	t.Helper()

	gated := newGatedStreamProvider()

	runner := newModelTestRunner(t, gated, testProfileSlug)
	runner.schedCfg = tierDefaultTestConfig(t)

	ctx := context.Background()

	turnDone := startTestTurn(t, runner, "s-default", ctx)

	awaitSeenModelWhy(t, gated, testModelFromConfig,
		"the tier-resolved default is chip==wire with no editor stamp")

	close(gated.releaseFirst)
	<-turnDone
}

// defaultTurnExplicitStampSubtest: D-12 — the editor write tops the chain;
// the tier default never overrides an explicit stamp.
func defaultTurnExplicitStampSubtest(t *testing.T) {
	t.Helper()

	gated := newGatedStreamProvider()

	runner := newModelTestRunner(t, gated, testProfileSlug)
	runner.schedCfg = tierDefaultTestConfig(t)

	aerr := runner.ApplyTurnModel(testModelExplicit)
	if aerr != nil {
		t.Fatalf("ApplyTurnModel: %v", aerr)
	}

	ctx := context.Background()

	turnDone := startTestTurn(t, runner, "s-explicit", ctx)

	awaitSeenModelWhy(t, gated, testModelExplicit, "the editor write tops the chain (D-12)")

	close(gated.releaseFirst)
	<-turnDone
}

// defaultTurnNilConfigSubtest: a Runner with schedCfg == nil keeps the
// documented profile-slug default (test runners).
func defaultTurnNilConfigSubtest(t *testing.T) {
	t.Helper()

	gated := newGatedStreamProvider()

	// schedCfg stays nil — the documented test-runner default.
	runner := newModelTestRunner(t, gated, testProfileSlug)

	ctx := context.Background()

	turnDone := startTestTurn(t, runner, "s-nil", ctx)

	awaitSeenModelWhy(t, gated, testProfileSlug,
		"nil schedCfg keeps the documented profile-slug default")

	close(gated.releaseFirst)
	<-turnDone
}

// defaultTurnResolverDeclineSubtest: hand-built schedCfg whose heavy binding's
// model is NOT declared in models (Resolve declines) and whose fixture
// window's schedule is malformed, so the window is never active (the
// T-16-09-03 malformed-window degrade). The static binding's slug must be
// stamped — exactly the advertisement's resolveModelLocked fallback (chip
// parity).
func defaultTurnResolverDeclineSubtest(t *testing.T) {
	t.Helper()

	gated := newGatedStreamProvider()

	runner := newModelTestRunner(t, gated, testProfileSlug)
	runner.schedCfg = &modelrouting.Config{
		Timezone:    "UTC",
		SessionTier: testTierHeavy,
		Providers: map[string]modelrouting.ProviderConfig{
			"anthropic": {BaseURL: "https://fallback.invalid", Shape: "anthropic"},
		},
		Models: map[string]modelrouting.ModelConfig{},
		Tiers: map[string]modelrouting.TierBinding{
			testTierHeavy: {Model: testModelStaticBind},
		},
		TimeWindows: []modelrouting.TimeWindow{{
			Name:     "never-active",
			Zone:     "UTC",
			Schedule: modelrouting.Schedule{From: "99:99", To: "00:00"},
			Tiers: map[string]modelrouting.TierBinding{
				testTierHeavy: {Model: testModelFromConfig},
			},
		}},
	}

	ctx := context.Background()

	turnDone := startTestTurn(t, runner, "s-fallback", ctx)

	awaitSeenModelWhy(t, gated, testModelStaticBind,
		"a resolver decline falls back to the static binding (chip parity)")

	close(gated.releaseFirst)
	<-turnDone
}

// TestApplyTurnModel_StampsFutureSessions pins the future-sessions leg:
// sessions created after the change stamp the effective model at construction.
func TestApplyTurnModel_StampsFutureSessions(t *testing.T) {
	t.Parallel()

	runner := &Runner{
		bus:     event.NewBus(),
		profile: profile.Profile{Name: testProfileName, Model: testModelBefore},
		workDir: t.TempDir(),
		maxConc: 2,
		makeProvider: func(provider.RequestCapturer) provider.Provider {
			return newGatedStreamProvider()
		},
	}

	ctx := context.Background()

	_ = runner.sessionFor(ctx, "s1")

	aerr := runner.ApplyTurnModel(testModelAfter)
	if aerr != nil {
		t.Fatalf("ApplyTurnModel: %v", aerr)
	}

	sess2 := runner.sessionFor(ctx, "s2")
	if got := sess2.Profile.Model; got != testModelAfter {
		t.Errorf("new session model = %q; want the effective %q stamped at construction", got, testModelAfter)
	}
}

// --- 24-02 (TAIL-01, D-06 observable): replayed breakers bend the light tier ---

// lightTierProvider is the battery's session-provider slug (goconst).
const lightTierProvider = "anthropic"

// lightTierTestConfig loads a temp scheduling config whose LIGHT tier binds
// light-primary with light-fallback (both declared on the embedded floor's
// anthropic provider so loader validation passes) — the fixture for the
// subagent demotion battery.
func lightTierTestConfig(t *testing.T) *modelrouting.Config {
	t.Helper()

	path := filepath.Join(t.TempDir(), "light.yaml")
	content := "models:\n  light-primary:\n    provider: anthropic\n  light-fallback:\n    provider: anthropic\n" +
		"tiers:\n  light:\n    model: light-primary\n    fallback: [light-fallback]\n"

	werr := os.WriteFile(path, []byte(content), 0o600)
	if werr != nil {
		t.Fatalf("write light-tier config: %v", werr)
	}

	cfg, err := modelrouting.Load(path)
	if err != nil {
		t.Fatalf("load light-tier config: %v", err)
	}

	return cfg
}

// seedLightTierBreakers seeds an outcome store with enough consecutive
// transients to OPEN the named models' replayed breakers (the floor's
// thresholds), then replays it — the same ReadOutcomes+ReplayBreakers the
// Runner's lazy load performs. Records are stamped within the cooldown window
// of resolveAt (a breaker denies only while its cooldown has NOT elapsed since
// the last transient — older stamps would admit the half-open probe).
func seedLightTierBreakers(
	t *testing.T, cfg *modelrouting.Config, resolveAt time.Time, models ...string,
) map[modelrouting.ProviderModelKey]modelrouting.Breaker {
	t.Helper()

	store, err := modelrouting.NewOutcomeStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewOutcomeStore: %v", err)
	}

	base := resolveAt.Add(-time.Duration(cfg.CircuitBreaker.ConsecutiveFailures+2) * time.Second)
	n := cfg.CircuitBreaker.ConsecutiveFailures

	for _, model := range models {
		for i := range n {
			aerr := store.Append(modelrouting.DispatchOutcome{
				At:       base.Add(time.Duration(i) * time.Second),
				Provider: lightTierProvider,
				Model:    model,
				Outcome:  modelrouting.OutcomeTransient,
				Origin:   modelrouting.OutcomeOriginSubagent,
			})
			if aerr != nil {
				t.Fatalf("Append seed record %s/%d: %v", model, i, aerr)
			}
		}
	}

	records, _, rerr := modelrouting.ReadOutcomes(store.Path())
	if rerr != nil {
		t.Fatalf("ReadOutcomes: %v", rerr)
	}

	return modelrouting.ReplayBreakers(records, cfg.CircuitBreaker, nil)
}

// TestResolveSubagentModelBreakerDemotion: with the light-tier primary's
// replayed breaker OPEN, resolveSubagentModel returns the first allowed
// FALLBACK slug; with empty breakers it returns the primary; with every
// candidate denied it keeps the primary (a resolution never fails over
// evidence) — each demotion fires exactly one loud stderr note.
func TestResolveSubagentModelBreakerDemotion(t *testing.T) {
	t.Parallel()

	cfg := lightTierTestConfig(t)
	now := time.Date(2026, 9, 10, 13, 0, 0, 0, time.UTC)

	t.Run("open primary demotes to the first allowed fallback", func(t *testing.T) {
		t.Parallel()

		breakers := seedLightTierBreakers(t, cfg, now, "light-primary")
		var stderr strings.Builder

		got := resolveSubagentModel(cfg, lightTierProvider, breakers, now, &stderr)
		if got != "light-fallback" {
			t.Fatalf("resolveSubagentModel = %q; want the demoted light-fallback (primary breaker-open)", got)
		}

		if !strings.Contains(stderr.String(), "light-primary") || !strings.Contains(stderr.String(), "light-fallback") {
			t.Errorf("stderr does not name the demoted primary + replacement: %q", stderr.String())
		}
	})

	t.Run("empty breakers keep the primary (no evidence, no demotion)", func(t *testing.T) {
		t.Parallel()

		var stderr strings.Builder

		got := resolveSubagentModel(cfg, lightTierProvider, nil, now, &stderr)
		if got != "light-primary" {
			t.Fatalf("resolveSubagentModel = %q; want the primary light-primary (empty breakers never demote)", got)
		}

		if strings.Contains(stderr.String(), "breaker") {
			t.Errorf("demotion note fired with empty breakers: %q", stderr.String())
		}
	})

	t.Run("all candidates denied keeps the primary", func(t *testing.T) {
		t.Parallel()

		breakers := seedLightTierBreakers(t, cfg, now, "light-primary", "light-fallback")
		var stderr strings.Builder

		got := resolveSubagentModel(cfg, lightTierProvider, breakers, now, &stderr)
		if got != "light-primary" {
			t.Fatalf("resolveSubagentModel = %q; want the primary kept when every candidate is denied", got)
		}
	})
}

// --- 24-06 (G-24-2, CR-01): cross-provider demotion never wires the wrong slug ---

// crossProviderLightConfig loads a temp scheduling config whose LIGHT tier
// rides a CROSS-PROVIDER chain (the valid.yaml vocabulary): primary
// light-xtra on the session provider (anthropic), fallbacks [minimax-m3 on
// openai, glm-4.6 on anthropic] — the wrong-wire fixture: an unfiltered
// demotion walk admits minimax-m3 and returns a slug the session provider
// does not host (checkBinding checks capability compatibility, never
// provider equality — the configuration class is supported).
func crossProviderLightConfig(t *testing.T) *modelrouting.Config {
	t.Helper()

	path := filepath.Join(t.TempDir(), "light-xprovider.yaml")
	content := "providers:\n" +
		"  anthropic:\n    base_url: \"https://api.z.ai/api/anthropic\"\n    shape: anthropic\n" +
		"  openai:\n    base_url: \"https://api.openai.com/v1\"\n    shape: openai\n" +
		"models:\n" +
		"  light-xtra:\n    provider: anthropic\n" +
		"    capabilities: { context_window: 128000, max_output_tokens: 16000, tool_calling: true, streaming: true, extended_thinking: true }\n" +
		"  minimax-m3:\n    provider: openai\n" +
		"    capabilities: { context_window: 200000, max_output_tokens: 32000, tool_calling: true, streaming: true, extended_thinking: true }\n" +
		"  glm-4.6:\n    provider: anthropic\n" +
		"    capabilities: { context_window: 128000, max_output_tokens: 16000, tool_calling: true, streaming: true, extended_thinking: true }\n" +
		"tiers:\n  light:\n    model: light-xtra\n    fallback: [minimax-m3, glm-4.6]\n"

	werr := os.WriteFile(path, []byte(content), 0o600)
	if werr != nil {
		t.Fatalf("write cross-provider light config: %v", werr)
	}

	cfg, err := modelrouting.Load(path)
	if err != nil {
		t.Fatalf("load cross-provider light config: %v", err)
	}

	return cfg
}

// TestResolveSubagentModelCrossProviderDemotion (24-06, G-24-2/CR-01): the
// demotion walk is provider-FILTERED — a breaker-open light primary demotes
// ONLY onto a fallback the session provider hosts, and a fully-denied
// same-provider chain keeps the primary with exactly ONE truthful note
// naming the provider constraint (the returned slug is stamped onto a
// profile riding the session provider's wire — the no-silent-wrong-wire
// contract, runtime.go's own doc).
func TestResolveSubagentModelCrossProviderDemotion(t *testing.T) {
	t.Parallel()

	cfg := crossProviderLightConfig(t)
	now := time.Date(2026, 9, 10, 19, 0, 0, 0, time.UTC)

	t.Run("open primary skips the allowed cross-provider candidate", func(t *testing.T) {
		t.Parallel()

		breakers := seedLightTierBreakers(t, cfg, now, "light-xtra")
		var stderr strings.Builder

		got := resolveSubagentModel(cfg, lightTierProvider, breakers, now, &stderr)
		if got != "glm-4.6" {
			t.Fatalf("resolveSubagentModel = %q; want glm-4.6 (the same-provider fallback — never the cross-provider minimax-m3)", got)
		}

		if !strings.Contains(stderr.String(), "light-xtra") || !strings.Contains(stderr.String(), "glm-4.6") {
			t.Errorf("stderr does not name the demoted primary + replacement: %q", stderr.String())
		}
	})

	t.Run("same-provider chain fully denied keeps the primary with the provider-constraint note", func(t *testing.T) {
		t.Parallel()

		breakers := seedLightTierBreakers(t, cfg, now, "light-xtra", "glm-4.6")
		var stderr strings.Builder

		got := resolveSubagentModel(cfg, lightTierProvider, breakers, now, &stderr)
		if got != "light-xtra" {
			t.Fatalf("resolveSubagentModel = %q; want the primary light-xtra kept (the only admitted fallback is cross-provider)", got)
		}

		if !strings.Contains(stderr.String(), lightTierProvider) {
			t.Errorf("the keep-primary note must name the session provider constraint %q: %q", lightTierProvider, stderr.String())
		}
	})
}
