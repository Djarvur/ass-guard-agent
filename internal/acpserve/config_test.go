package acpserve //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// The 16-05 ConfigSurface tests (ACP-08 implementation half): menu from real
// layers, _meta fills-unset blob semantics (D-10), _global/ scope routing
// (D-08), persistence through the real loader (D-07 write half), serialized
// mutation, and the D-10 idempotence guard on the set channel.

// Repeated literals (goconst) — the embedded floor's vocabulary.
const (
	testModelPrimary     = "GLM-5.3"
	testModelFallback    = "glm-5.2"
	testTierHeavy        = "heavy"
	testTierLight        = "light"
	testPermUngated      = "ungated"
	testPermGated        = "gated"
	testCompactionBlob   = "65"
	testCompactionDefval = "80"
)

// surfaceFixture is one ConfigSurface over temp layer paths with a recording
// notify hook.
type surfaceFixture struct {
	surface      *ConfigSurface
	stderr       *strings.Builder
	globalPath   string
	projectPath  string
	notifyMu     sync.Mutex
	notifyCalls  [][]acp.ConfigOptionFrame
	notifyScopes []string
}

func newSurfaceFixture(t *testing.T) *surfaceFixture {
	t.Helper()

	root := t.TempDir()

	global := filepath.Join(root, "home", ".config", "ass-guard-agent", "config.yaml")
	project := filepath.Join(root, "proj", ".ass-guard", "config.yaml")

	f := &surfaceFixture{
		stderr:      &strings.Builder{},
		globalPath:  global,
		projectPath: project,
	}

	f.surface = NewConfigSurface(global, project, "anthropic", f.stderr)
	f.surface.SetNotify(func(sessionID string, opts []acp.ConfigOptionFrame) {
		f.notifyMu.Lock()
		defer f.notifyMu.Unlock()

		f.notifyCalls = append(f.notifyCalls, opts)
		f.notifyScopes = append(f.notifyScopes, sessionID)
	})

	return f
}

func (f *surfaceFixture) notifyCount() int {
	f.notifyMu.Lock()
	defer f.notifyMu.Unlock()

	return len(f.notifyCalls)
}

// writeLayer plants a layer file (creating parent dirs) with the given YAML.
func writeLayer(t *testing.T, path, content string) {
	t.Helper()

	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		t.Fatalf("mkdir for layer %s: %v", path, err)
	}

	werr := os.WriteFile(path, []byte(content), 0o600)
	if werr != nil {
		t.Fatalf("write layer %s: %v", path, werr)
	}
}

// twoTierLayer declares a second tier so tier-set flows have a real
// alternative to switch to (both models declared by the embedded floor).
const twoTierLayer = `tiers:
  heavy:
    model: GLM-5.3
  light:
    model: glm-5.2
`

// unrelatedLayer is a layer byte-compare anchors stay untouched by scoped
// writes.
const unrelatedLayer = `timezone: "UTC"
`

func optionByID(t *testing.T, opts []acp.ConfigOptionFrame, id string) acp.ConfigOptionFrame {
	t.Helper()

	for _, o := range opts {
		if o.ID == id {
			return o
		}
	}

	t.Fatalf("option %q not in the advertised set (%d entries)", id, len(opts))

	return acp.ConfigOptionFrame{}
}

func assertEight(t *testing.T, opts []acp.ConfigOptionFrame, where string) {
	t.Helper()

	if len(opts) != 8 {
		t.Fatalf("%s: options = %d; want the eight-entry menu", where, len(opts))
	}
}

func TestConfigSurface_MenuDefaults(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	opts := f.surface.Options()
	assertEight(t, opts, "defaults-only")

	model := optionByID(t, opts, optModel)
	if model.Category != optModel || model.Type != acp.ConfigOptionTypeSelect {
		t.Errorf("model category/type = %q/%q; want model/select", model.Category, model.Type)
	}

	wantModels := []string{testModelPrimary, testModelFallback} // the embedded floor's declared set, sorted

	got := make([]string, 0, len(model.Options))

	for _, v := range model.Options {
		got = append(got, v.Value)
	}

	if strings.Join(got, ",") != strings.Join(wantModels, ",") {
		t.Errorf("model options = %v; want the embedded default models %v", got, wantModels)
	}

	if model.CurrentValue != testModelPrimary {
		t.Errorf("model currentValue = %q; want the resolved heavy primary GLM-5.3 (D-11)", model.CurrentValue)
	}

	tier := optionByID(t, opts, optTier)
	if tier.Category != "model_config" {
		t.Errorf("tier category = %q; want model_config", tier.Category)
	}

	if tier.CurrentValue != testTierHeavy {
		t.Errorf("tier currentValue = %q; want Config.SessionTier heavy", tier.CurrentValue)
	}

	if len(tier.Options) != 1 || tier.Options[0].Value != testTierHeavy {
		t.Errorf("tier options = %+v; want the embedded tiers [heavy]", tier.Options)
	}

	perm := optionByID(t, opts, optPermissionsMode)
	if perm.Category != "mode" || perm.CurrentValue != testPermUngated {
		t.Errorf("permissions.mode = %q/%q; want mode/ungated (the phase default)", perm.Category, perm.CurrentValue)
	}

	comp := optionByID(t, opts, optCompactionThresh)
	if comp.Category != "_custom" || comp.CurrentValue != testCompactionDefval {
		t.Errorf("compaction-threshold = %q/%q; want _custom/80 (the pending default)",
			comp.Category, comp.CurrentValue)
	}

	globalTwins := []string{
		"_global/model", "_global/tier", "_global/permissions.mode", "_global/compaction-threshold",
	}

	for _, id := range globalTwins {
		optionByID(t, opts, id) // must exist
	}
}

func TestConfigSurface_ExplicitLayerEffectiveValues(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, "tiers:\n  heavy:\n    model: glm-5.2\n")

	opts := f.surface.Options()

	model := optionByID(t, opts, optModel)
	if model.CurrentValue != testModelFallback {
		t.Errorf("model currentValue = %q; want the FILE's explicit glm-5.2 (project wins over floor)",
			model.CurrentValue)
	}
}

func TestConfigSurface_SetPersistsThroughLoader(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	_, setErr := f.surface.Set("sess-1", optModel, testModelFallback)
	if setErr != nil {
		t.Fatalf("Set(model glm-5.2): %v", setErr)
	}

	// The D-07 write half: assert by re-loading through the REAL loader, not a
	// re-parse of our own struct.
	cfg, err := modelrouting.Load(f.projectPath)
	if err != nil {
		t.Fatalf("reload written project layer: %v", err)
	}

	if cfg.Tiers[testTierHeavy].Model != testModelFallback {
		t.Errorf("loaded tiers.heavy.model = %q; want the written glm-5.2", cfg.Tiers[testTierHeavy].Model)
	}

	// The refreshed response carries the new effective value.
	opts := f.surface.Options()
	if got := optionByID(t, opts, optModel).CurrentValue; got != testModelFallback {
		t.Errorf("post-set advertisement currentValue = %q; want glm-5.2", got)
	}
}

func TestConfigSurface_PendingNoOp(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	before := f.notifyCount()

	opts, err := f.surface.Set("sess-1", optPermissionsMode, testPermGated)
	if err != nil {
		t.Fatalf("Set(pending permissions.mode): %v (D-05: accepted no-op, never an error)", err)
	}

	assertEight(t, opts, "pending response")

	if got := optionByID(t, opts, optPermissionsMode).CurrentValue; got != testPermUngated {
		t.Errorf("permissions.mode currentValue = %q; want UNCHANGED ungated", got)
	}

	if f.notifyCount() != before {
		t.Error("pending no-op emitted a config_option_update (nothing changed)")
	}

	if got := f.stderr.String(); !strings.Contains(got, "pending") {
		t.Errorf("pending set not logged (stderr=%q)", got)
	}

	// No layer file was created by a pending id.
	_, statErr := os.Stat(f.projectPath)
	if !os.IsNotExist(statErr) {
		t.Error("pending no-op wrote a layer file (pending ids never persist)")
	}
}

func TestConfigSurface_ConcurrentMutation(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, twoTierLayer)

	var wg sync.WaitGroup

	wg.Go(func() {
		blob := map[string]json.RawMessage{optCompactionThresh: json.RawMessage(`"` + testCompactionBlob + `"`)}
		_, _ = f.surface.ApplyBlobDefaults(blob)
	})
	wg.Go(func() { _, _ = f.surface.Set("sess-c", optModel, testModelFallback) })
	wg.Go(func() { _, _ = f.surface.Set("sess-c", optTier, testTierLight) })

	wg.Wait()

	// No torn YAML: the real loader parses the final file.
	cfg, err := modelrouting.Load(f.projectPath)
	if err != nil {
		t.Fatalf("final layer does not parse (torn YAML?): %v", err)
	}

	// Exactly one write per key: the final on-disk values are exactly the
	// values the serialized ops wrote.
	if cfg.SessionTier != testTierLight {
		t.Errorf("loaded session_tier = %q; want the single tier write light", cfg.SessionTier)
	}

	if cfg.Tiers[testTierHeavy].Model != testModelPrimary && cfg.Tiers[testTierHeavy].Model != testModelFallback {
		t.Errorf("loaded tiers.heavy.model = %q; want the single model write's value", cfg.Tiers[testTierHeavy].Model)
	}

	// Memory == disk (no half-applied state): the advertisement reflects what
	// the loader sees.
	opts := f.surface.Options()

	if got := optionByID(t, opts, optTier).CurrentValue; got != cfg.SessionTier {
		t.Errorf("advertised tier %q != loaded session_tier %q (half-applied state)", got, cfg.SessionTier)
	}

	if got := optionByID(t, opts, optCompactionThresh).CurrentValue; got != testCompactionBlob {
		t.Errorf("advertised compaction-threshold = %q; want the blob fill 65", got)
	}
}

func TestMetaBlob_ExplicitFileWins(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, "tiers:\n  heavy:\n    model: glm-5.2\n")

	changed, err := f.surface.ApplyBlobDefaults(map[string]json.RawMessage{
		optModel: json.RawMessage(`"` + testModelPrimary + `"`),
	})
	if err != nil {
		t.Fatalf("ApplyBlobDefaults: %v", err)
	}

	if changed {
		t.Error("blob application reported changed while the explicit file value won (D-10: fills-unset only)")
	}

	if got := optionByID(t, f.surface.Options(), optModel).CurrentValue; got != testModelFallback {
		t.Errorf("effective model = %q; want the FILE's glm-5.2 (blob never overrides explicit config)", got)
	}

	if f.notifyCount() != 0 {
		t.Error("config_option_update emitted though nothing changed")
	}
}

func TestMetaBlob_FillsUnsetInMemory(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	changed, err := f.surface.ApplyBlobDefaults(map[string]json.RawMessage{
		optModel: json.RawMessage(`"` + testModelFallback + `"`),
	})
	if err != nil {
		t.Fatalf("ApplyBlobDefaults: %v", err)
	}

	if !changed {
		t.Error("blob fill over an unset slot did not report changed")
	}

	if got := optionByID(t, f.surface.Options(), optModel).CurrentValue; got != testModelFallback {
		t.Errorf("effective model = %q; want the in-memory blob fill glm-5.2", got)
	}

	// In-memory ONLY: no layer file appeared.
	_, projStatErr := os.Stat(f.projectPath)
	if !os.IsNotExist(projStatErr) {
		t.Error("blob application persisted to the project layer (D-10: never persisted)")
	}

	_, globStatErr := os.Stat(f.globalPath)
	if !os.IsNotExist(globStatErr) {
		t.Error("blob application persisted to the global layer (D-10: never persisted)")
	}

	// Exactly one config_option_update with the FULL set followed application.
	if f.notifyCount() != 1 {
		t.Fatalf("config_option_update count = %d; want exactly 1", f.notifyCount())
	}

	assertEight(t, f.notifyCalls[0], "blob notification")
}

func TestMetaBlob_UnknownKeysRetainedVerbatim(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	const rawKey = "future.things"

	rawVal := json.RawMessage(`{"a":[1,2],"b":"keep me byte-identical"}`)

	_, err := f.surface.ApplyBlobDefaults(map[string]json.RawMessage{
		optModel: json.RawMessage(`"` + testModelFallback + `"`),
		rawKey:   rawVal,
		"weird":  json.RawMessage(`[1,{"x":null}]`),
	})
	if err != nil {
		t.Fatalf("ApplyBlobDefaults: %v", err)
	}

	// Round-trip survival: the serve state holds the unknown keys byte-identical
	// (never executed, never re-serialized through a typed shape).
	if string(f.surface.blobRaw[rawKey]) != string(rawVal) {
		t.Errorf("stored unknown key = %s; want byte-identical %s", f.surface.blobRaw[rawKey], rawVal)
	}

	if _, ok := f.surface.blobRaw["weird"]; !ok {
		t.Error("second unknown key not retained")
	}
}

func TestScopeRouting_GlobalPrefixWritesGlobalLayer(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, unrelatedLayer)

	projectBefore := readLayerBytes(t, f.projectPath)

	_, err := f.surface.Set("sess-1", "_global/model", testModelFallback)
	if err != nil {
		t.Fatalf("Set(_global/model): %v", err)
	}

	cfg, err := modelrouting.Load(f.globalPath)
	if err != nil {
		t.Fatalf("global layer after scoped write: %v", err)
	}

	if cfg.Tiers[testTierHeavy].Model != testModelFallback {
		t.Errorf("global tiers.heavy.model = %q; want the scoped write glm-5.2", cfg.Tiers[testTierHeavy].Model)
	}

	if got := readLayerBytes(t, f.projectPath); got != projectBefore {
		t.Error("project layer changed by a _global-scoped write (D-08 scope routing)")
	}
}

func TestScopeRouting_DefaultWritesProjectLayer(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.globalPath, unrelatedLayer)

	// Un-prefixed model write lands in the PROJECT layer; the global file stays
	// byte-identical.
	_, err := f.surface.Set("sess-1", optModel, testModelFallback)
	if err != nil {
		t.Fatalf("Set(model): %v", err)
	}

	cfg, err := modelrouting.Load(f.projectPath)
	if err != nil {
		t.Fatalf("project layer after default-scope write: %v", err)
	}

	if cfg.Tiers[testTierHeavy].Model != testModelFallback {
		t.Errorf("project tiers.heavy.model = %q; want glm-5.2 (project is the default target, D-08)",
			cfg.Tiers[testTierHeavy].Model)
	}

	if got := readLayerBytes(t, f.globalPath); got != unrelatedLayer {
		t.Errorf("global layer changed by a project-scoped write: %q", got)
	}
}

// TestScopeRouting_GlobalWritePersistsWhenCombinedMatches pins the scope-aware
// idempotence basis (16-08 gap closure: WR-05 gaps 3+5, one root cause): a
// _global/-scoped write whose value equals the COMBINED (project-won) effective
// value but differs from the GLOBAL layer's own value must reach the global
// layer. The scope-blind guard classified it as a redundant re-push and
// silently swallowed the operator's explicitly global mutation.
func TestScopeRouting_GlobalWritePersistsWhenCombinedMatches(t *testing.T) {
	t.Parallel()

	t.Run("differing-addressed-layer-value-persists", func(t *testing.T) {
		t.Parallel()

		f := newSurfaceFixture(t)
		// Project layer wins the COMBINED resolution with GLM-5.3; the global
		// layer's own value is glm-5.2.
		writeLayer(t, f.projectPath, "tiers:\n  heavy:\n    model: "+testModelPrimary+"\n")
		writeLayer(t, f.globalPath, "tiers:\n  heavy:\n    model: "+testModelFallback+"\n")

		opts, err := f.surface.Set("sess-1", "_global/model", testModelPrimary)
		if err != nil {
			t.Fatalf("Set(_global/model) equal to the combined effective: %v", err)
		}

		assertEight(t, opts, "global-scope set response")

		// Read back through the REAL loader: the global file must carry the
		// written value (the silent swallow kept glm-5.2 on disk).
		cfg, err := modelrouting.Load(f.globalPath)
		if err != nil {
			t.Fatalf("reload global layer: %v", err)
		}

		if cfg.Tiers[testTierHeavy].Model != testModelPrimary {
			t.Errorf("global tiers.heavy.model = %q; want the explicitly global write %q persisted",
				cfg.Tiers[testTierHeavy].Model, testModelPrimary)
		}

		if got := strings.Count(f.stderr.String(), "idempotent"); got != 0 {
			t.Errorf("legitimate global mutation classified as an idempotent re-push (%d log lines) — silent swallow",
				got)
		}
	})

	t.Run("addressed-layer-true-idempotence", func(t *testing.T) {
		t.Parallel()

		f := newSurfaceFixture(t)
		// The GLOBAL layer genuinely holds glm-5.2; no project layer — the
		// combined effective is glm-5.2 as well.
		writeLayer(t, f.globalPath, "tiers:\n  heavy:\n    model: "+testModelFallback+"\n")
		writeLayer(t, f.projectPath, unrelatedLayer)

		globalBefore := readLayerBytes(t, f.globalPath)

		opts, err := f.surface.Set("sess-1", "_global/model", testModelFallback)
		if err != nil {
			t.Fatalf("Set(_global/model) equal to the global layer's value: %v", err)
		}

		assertEight(t, opts, "global idempotent response")

		if got := readLayerBytes(t, f.globalPath); got != globalBefore {
			t.Errorf("true global idempotence churned the global file:\nbefore=%q\nafter=%q",
				globalBefore, got)
		}

		if got := strings.Count(f.stderr.String(), "idempotent"); got != 1 {
			t.Errorf("idempotent log lines = %d; want exactly 1 (the addressed layer holds the value) (stderr=%q)",
				got, f.stderr.String())
		}

		if f.notifyCount() != 0 {
			t.Error("global idempotent re-push emitted a config_option_update (nothing changed)")
		}
	})
}

func TestSetIdempotent_BlobDerivedEffective(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, unrelatedLayer)

	projectBefore := readLayerBytes(t, f.projectPath)

	// The blob fills the unset slot: the EFFECTIVE model becomes blob-derived.
	blob := map[string]json.RawMessage{optModel: json.RawMessage(`"` + testModelFallback + `"`)}

	_, err := f.surface.ApplyBlobDefaults(blob)
	if err != nil {
		t.Fatalf("ApplyBlobDefaults: %v", err)
	}

	// A redundant re-push of that same value (Zed re-pushing stored defaults)
	// must NOT write the layer nor promote the blob value into explicit config.
	opts, err := f.surface.Set("sess-1", optModel, testModelFallback)
	if err != nil {
		t.Fatalf("idempotent Set: %v", err)
	}

	assertEight(t, opts, "idempotent response")

	if got := readLayerBytes(t, f.projectPath); got != projectBefore {
		t.Errorf("idempotent re-push churned the layer file (D-10 guard failed):\nbefore=%q\nafter=%q",
			projectBefore, got)
	}

	if f.notifyCount() != 1 { // exactly the blob application's one update — the re-push added none
		t.Errorf("config_option_update count = %d; want 1 (the re-push emitted nothing — nothing changed)",
			f.notifyCount())
	}

	if got := strings.Count(f.stderr.String(), "idempotent"); got != 1 {
		t.Errorf("idempotent log lines = %d; want exactly 1 (stderr=%q)", got, f.stderr.String())
	}

	// The overlay fill survives (not promoted, not dropped): still effective.
	if got := optionByID(t, f.surface.Options(), optModel).CurrentValue; got != testModelFallback {
		t.Errorf("effective model after re-push = %q; want glm-5.2 (overlay unchanged)", got)
	}
}

func TestSetIdempotent_ExplicitFileEffective(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	_, err := f.surface.Set("sess-1", optModel, testModelFallback)
	if err != nil {
		t.Fatalf("first Set: %v", err)
	}

	projectBefore := readLayerBytes(t, f.projectPath)
	notifiesBefore := f.notifyCount()

	_, err2 := f.surface.Set("sess-1", optModel, testModelFallback)
	if err2 != nil {
		t.Fatalf("second Set: %v", err2)
	}

	if got := readLayerBytes(t, f.projectPath); got != projectBefore {
		t.Error("idempotent re-push rewrote the layer file")
	}

	if f.notifyCount() != notifiesBefore {
		t.Error("idempotent re-push emitted config_option_update")
	}

	if got := strings.Count(f.stderr.String(), "idempotent"); got != 1 {
		t.Errorf("idempotent log lines = %d; want exactly 1", got)
	}
}

func readLayerBytes(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read layer %s: %v", path, err)
	}

	return string(raw)
}

// --- 16-05 Task 3: the full serve path from wire to provider request ---
//
// TestLiveModelApply / TestTierSwitch drive acpserve.Run end-to-end: the
// Run composition binds the ConfigSurface to the runner (apply hook) and
// injects it into the Server (WithConfigSurface), so an editor's
// session/set_config_option changes the very next provider request's model —
// proven through the transcript's request_shaped fingerprints.

// sseModelStub is a recording SSE stub: it parses each request body's model
// and can hold the FIRST request open (the controllable in-flight request for
// the mid-turn case).
type sseModelStub struct {
	mu           sync.Mutex
	models       []string
	delayFirst   time.Duration
	inFlight     chan struct{}
	inFlightOnce sync.Once
	srv          *httptest.Server
}

func newSSEModelStub(t *testing.T, delayFirst time.Duration) *sseModelStub {
	t.Helper()

	st := &sseModelStub{delayFirst: delayFirst, inFlight: make(chan struct{})}

	st.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, rerr := io.ReadAll(r.Body)
		if rerr == nil {
			var req struct {
				Model string `json:"model"`
			}

			if json.Unmarshal(body, &req) == nil && req.Model != "" {
				st.mu.Lock()
				first := len(st.models) == 0
				st.models = append(st.models, req.Model)
				st.mu.Unlock()

				if first && st.delayFirst > 0 {
					st.inFlightOnce.Do(func() { close(st.inFlight) })
					time.Sleep(st.delayFirst)
				}
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")

		flusher, _ := w.(http.Flusher)

		for _, frame := range []string{
			`{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", frame)

			if flusher != nil {
				flusher.Flush()
			}
		}

		fmt.Fprint(w, "data: [DONE]\n\n")
	}))

	t.Cleanup(st.srv.Close)

	return st
}

func (s *sseModelStub) recordedModels() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.models...)
}

// waitInFlight blocks until the stub is HOLDING its first request open (only
// armed when delayFirst > 0) — the deterministic "in-flight request" point.
func (s *sseModelStub) waitInFlight(t *testing.T) {
	t.Helper()

	select {
	case <-s.inFlight:
	case <-time.After(10 * time.Second):
		t.Fatal("the stub's first request never went in flight")
	}
}

// liveApplyConfig is the project layer for the live-apply serves: the
// anthropic provider aimed at the stub + the declared extra YAML (tier
// bindings for the tier-switch cases).
func liveApplyConfig(t *testing.T, stubURL, extra string) string {
	t.Helper()

	workDir := t.TempDir()

	err := os.MkdirAll(filepath.Join(workDir, ".ass-guard"), 0o750)
	if err != nil {
		t.Fatalf("mkdir .ass-guard: %v", err)
	}

	content := "providers:\n  anthropic:\n    base_url: " + strconv.Quote(stubURL) +
		"\n    api_key: \"sk-live-apply-canary\"\n" + extra

	werr := os.WriteFile(filepath.Join(workDir, ".ass-guard", "config.yaml"), []byte(content), 0o600)
	if werr != nil {
		t.Fatalf("write config.yaml: %v", werr)
	}

	return workDir
}

// runLiveApplyServe starts one acpserve.Run over pipes against workDir and
// returns the frame writer + stdout snapshot.
func runLiveApplyServe( //nolint:nonamedreturns // names document the triple for callers
	t *testing.T, workDir string,
) (in *io.PipeWriter, stdout, stderr *syncBuffer) {
	t.Helper()

	t.Setenv("ZAI_API_KEY", "") // force the config literal (canary) to win

	stdout = &syncBuffer{}
	stderr = &syncBuffer{}

	//nolint:testingcontext // explicit cancel before the pipe close
	ctx, cancel := context.WithCancel(context.Background())

	inPipeR, inPipeW := io.Pipe()

	go func() {
		_ = Run(ctx, inPipeR, stdout, stderr, &Options{
			Profile: profileZcode, MaxConcurrent: 2,
			ProfilesDir: repoProfilesDir(t), WorkDir: workDir,
		})
	}()

	t.Cleanup(func() {
		cancel()

		_ = inPipeW.Close()
	})

	return inPipeW, stdout, stderr
}

func writeServeLine(t *testing.T, w io.Writer, line string) {
	t.Helper()

	_, err := w.Write([]byte(line + "\n"))
	if err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

// startLiveApplySession handshakes a Zed-like initialize (elicitation
// advertised — the D-13 probe-free path) + session/new and returns the
// session id.
func startLiveApplySession(t *testing.T, inPipeW *io.PipeWriter, stdout *syncBuffer) string {
	t.Helper()

	writeServeLine(t, inPipeW,
		`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1,`+
			`"clientCapabilities":{"elicitation":{"form":{}}}}}`)
	writeServeLine(t, inPipeW,
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"x"}}`)

	return pollStdoutForSessionID(t, stdout)
}

// waitRequestModels polls the session transcript until n request_shaped lines
// exist, returning their fingerprinted models in order.
func waitRequestModels(t *testing.T, workDir, sessionID string, n int) []string {
	t.Helper()

	transcriptPath := filepath.Join(workDir, ".ass-guard", "transcript_"+sessionID+".jsonl")

	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		raw, rerr := os.ReadFile(transcriptPath)
		if rerr == nil {
			var models []string

			for line := range strings.SplitSeq(string(raw), "\n") {
				if line == "" {
					continue
				}

				var l session.Line

				if json.Unmarshal([]byte(line), &l) == nil && l.Type == "request_shaped" {
					models = append(models, l.Model)
				}
			}

			if len(models) >= n {
				return models
			}
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("only saw fewer than %d request_shaped lines within 15s", n)

	return nil
}

// pollSetConfigResponse waits for the set_config_option response frame and
// asserts it is a success carrying the full option set (the composition is
// wired: no surface would answer the typed not-available error instead).
func pollSetConfigResponse(t *testing.T, stdout *syncBuffer, id string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		for line := range strings.SplitSeq(stdout.String(), "\n") {
			if !strings.Contains(line, `"id":`+id+`,`) {
				continue
			}

			if !strings.Contains(line, `"error"`) && strings.Contains(line, `"configOptions"`) {
				return
			}

			t.Fatalf("set_config_option response was an error or lacked the full set: %.400s", line)
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("set_config_option response (id=%s) never arrived; stdout tail: %.600s", id, stdout.String())
}

// tierExtraLayer declares tiers.light on the same provider (the switchable
// case: light → glm-5.2, both models the floor declares on anthropic).
const tierExtraLayer = "tiers:\n  light:\n    model: glm-5.2\n"

// liveApplyCrossProviderConfig writes the loud-degrade config as ONE document
// (tiers.light bound to a model of a DIFFERENT provider — cross-provider
// switches never rewire the live provider): providers.anthropic aimed at the
// stub + providers.other, models.other-model, tiers.light.
func liveApplyCrossProviderConfig(t *testing.T, stubURL string) string {
	t.Helper()

	workDir := t.TempDir()

	mkerr := os.MkdirAll(filepath.Join(workDir, ".ass-guard"), 0o750)
	if mkerr != nil {
		t.Fatalf("mkdir .ass-guard: %v", mkerr)
	}

	content := "providers:\n  anthropic:\n    base_url: " + strconv.Quote(stubURL) +
		"\n    api_key: \"sk-live-apply-canary\"\n" +
		"  other:\n    base_url: \"https://other.invalid\"\n    shape: openai\n" +
		"models:\n  other-model:\n    provider: other\n" +
		"    pricing: { input_per_mtoken: 0.0, output_per_mtoken: 0.0 }\n" +
		"    capabilities: { context_window: 100000, max_output_tokens: 32000,\n" +
		"      tool_calling: true, streaming: true, extended_thinking: true }\n" +
		"tiers:\n  light:\n    model: other-model\n"

	werr := os.WriteFile(filepath.Join(workDir, ".ass-guard", "config.yaml"), []byte(content), 0o600)
	if werr != nil {
		t.Fatalf("write config.yaml: %v", werr)
	}

	return workDir
}

func TestLiveModelApply(t *testing.T) { //nolint:paralleltest // runLiveApplyServe uses t.Setenv
	stub := newSSEModelStub(t, 0)
	workDir := liveApplyConfig(t, stub.srv.URL, "")
	inPipeW, stdout, _ := runLiveApplyServe(t, workDir)

	sid := startLiveApplySession(t, inPipeW, stdout)

	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"`+
		sid+`","prompt":[{"type":"text","text":"hi"}]}}`)

	models := waitRequestModels(t, workDir, sid, 1)
	if models[0] != testModelPrimary {
		t.Fatalf("first request model = %q; want the configured heavy primary %q", models[0], testModelPrimary)
	}

	// The editor switches the model; the surface persists then applies live.
	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":3,"method":"session/set_config_option","params":{"sessionId":"`+
		sid+`","configId":"`+optModel+`","value":"`+testModelFallback+`"}}`)

	pollSetConfigResponse(t, stdout, "3")

	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":4,"method":"session/prompt","params":{"sessionId":"`+
		sid+`","prompt":[{"type":"text","text":"again"}]}}`)

	models = waitRequestModels(t, workDir, sid, 2)
	if models[1] != testModelFallback {
		t.Errorf("post-set request model = %q; want %q (the very next request carries the new model)",
			models[1], testModelFallback)
	}
}

func TestLiveModelApply_MidTurn(t *testing.T) { //nolint:paralleltest // runLiveApplyServe uses t.Setenv
	stub := newSSEModelStub(t, 700*time.Millisecond) // the FIRST request is held open
	workDir := liveApplyConfig(t, stub.srv.URL, "")
	inPipeW, stdout, _ := runLiveApplyServe(t, workDir)

	sid := startLiveApplySession(t, inPipeW, stdout)

	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"`+
		sid+`","prompt":[{"type":"text","text":"hi"}]}}`)

	// Wait until the stub is HOLDING the first request open, then send the
	// Set mid-turn: it must WAIT.
	stub.waitInFlight(t)

	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":3,"method":"session/set_config_option","params":{"sessionId":"`+
		sid+`","configId":"`+optModel+`","value":"`+testModelFallback+`"}}`)

	time.Sleep(150 * time.Millisecond)

	inFlight := stub.recordedModels()
	if len(inFlight) != 1 || inFlight[0] != testModelPrimary {
		t.Fatalf("in-flight models = %v; want exactly [%s] (the held request keeps its model)",
			inFlight, testModelPrimary)
	}

	// The set response arrives only AFTER the held turn finished and the
	// serialized apply landed between turns.
	pollSetConfigResponse(t, stdout, "3")

	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":4,"method":"session/prompt","params":{"sessionId":"`+
		sid+`","prompt":[{"type":"text","text":"again"}]}}`)

	models := waitRequestModels(t, workDir, sid, 2)
	if models[0] != testModelPrimary || models[1] != testModelFallback {
		t.Errorf("request models = %v; want [%s %s] (no torn stamp, next request carries the new model)",
			models, testModelPrimary, testModelFallback)
	}
}

func TestTierSwitch(t *testing.T) { //nolint:paralleltest // runLiveApplyServe uses t.Setenv
	stub := newSSEModelStub(t, 0)
	workDir := liveApplyConfig(t, stub.srv.URL, tierExtraLayer)
	inPipeW, stdout, _ := runLiveApplyServe(t, workDir)

	sid := startLiveApplySession(t, inPipeW, stdout)

	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"`+
		sid+`","prompt":[{"type":"text","text":"hi"}]}}`)

	models := waitRequestModels(t, workDir, sid, 1)
	if models[0] != testModelPrimary {
		t.Fatalf("first request model = %q; want %q", models[0], testModelPrimary)
	}

	// tiers.light is bound to glm-5.2 on the SAME provider — the tier switch
	// rewires the model the next request carries.
	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":3,"method":"session/set_config_option","params":{"sessionId":"`+
		sid+`","configId":"`+optTier+`","value":"`+testTierLight+`"}}`)

	pollSetConfigResponse(t, stdout, "3")

	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":4,"method":"session/prompt","params":{"sessionId":"`+
		sid+`","prompt":[{"type":"text","text":"again"}]}}`)

	models = waitRequestModels(t, workDir, sid, 2)
	if models[1] != testModelFallback {
		t.Errorf("post-tier-switch request model = %q; want %q (tiers.light.model)", models[1], testModelFallback)
	}
}

func TestTierSwitch_CrossProvider(t *testing.T) { //nolint:paralleltest // runLiveApplyServe uses t.Setenv
	stub := newSSEModelStub(t, 0)
	workDir := liveApplyCrossProviderConfig(t, stub.srv.URL)
	inPipeW, stdout, stderr := runLiveApplyServe(t, workDir)

	sid := startLiveApplySession(t, inPipeW, stdout)

	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"`+
		sid+`","prompt":[{"type":"text","text":"hi"}]}}`)

	models := waitRequestModels(t, workDir, sid, 1)
	if models[0] != testModelPrimary {
		t.Fatalf("first request model = %q; want %q", models[0], testModelPrimary)
	}

	// tiers.light is bound to a DIFFERENT provider: the persist succeeds (the
	// tier is a config write) but the live apply degrades LOUDLY with the
	// model unchanged — cross-provider switches never rewire the live provider.
	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":3,"method":"session/set_config_option","params":{"sessionId":"`+
		sid+`","configId":"`+optTier+`","value":"`+testTierLight+`"}}`)

	pollSetConfigResponse(t, stdout, "3")

	writeServeLine(t, inPipeW, `{"jsonrpc":"2.0","id":4,"method":"session/prompt","params":{"sessionId":"`+
		sid+`","prompt":[{"type":"text","text":"again"}]}}`)

	models = waitRequestModels(t, workDir, sid, 2)
	if models[1] != testModelPrimary {
		t.Errorf("post-tier-switch request model = %q; want %q UNCHANGED (cross-provider degrade)",
			models[1], testModelPrimary)
	}

	if got := stderr.String(); !strings.Contains(got, "live apply SKIPPED") {
		t.Errorf("cross-provider tier switch not loudly logged (stderr=%q)", got)
	}
}
