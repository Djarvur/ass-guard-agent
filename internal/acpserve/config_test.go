package acpserve //nolint:testpackage // internal package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
)

// The 16-05 ConfigSurface tests (ACP-08 implementation half): menu from real
// layers, _meta fills-unset blob semantics (D-10), _global/ scope routing
// (D-08), persistence through the real loader (D-07 write half), serialized
// mutation, and the D-10 idempotence guard on the set channel.

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

	model := optionByID(t, opts, "model")
	if model.Category != "model" || model.Type != acp.ConfigOptionTypeSelect {
		t.Errorf("model category/type = %q/%q; want model/select", model.Category, model.Type)
	}

	wantModels := []string{"GLM-5.3", "glm-5.2"} // the embedded floor's declared set, sorted
	got := make([]string, 0, len(model.Options))
	for _, v := range model.Options {
		got = append(got, v.Value)
	}

	if strings.Join(got, ",") != strings.Join(wantModels, ",") {
		t.Errorf("model options = %v; want the embedded default models %v", got, wantModels)
	}

	if model.CurrentValue != "GLM-5.3" {
		t.Errorf("model currentValue = %q; want the resolved heavy primary GLM-5.3 (D-11)", model.CurrentValue)
	}

	tier := optionByID(t, opts, "tier")
	if tier.Category != "model_config" {
		t.Errorf("tier category = %q; want model_config", tier.Category)
	}

	if tier.CurrentValue != "heavy" {
		t.Errorf("tier currentValue = %q; want Config.SessionTier heavy", tier.CurrentValue)
	}

	if len(tier.Options) != 1 || tier.Options[0].Value != "heavy" {
		t.Errorf("tier options = %+v; want the embedded tiers [heavy]", tier.Options)
	}

	perm := optionByID(t, opts, "permissions.mode")
	if perm.Category != "mode" || perm.CurrentValue != "ungated" {
		t.Errorf("permissions.mode = %q/%q; want mode/ungated (the phase default)", perm.Category, perm.CurrentValue)
	}

	comp := optionByID(t, opts, "compaction-threshold")
	if comp.Category != "_custom" || comp.CurrentValue != "80" {
		t.Errorf("compaction-threshold = %q/%q; want _custom/80 (the pending default)", comp.Category, comp.CurrentValue)
	}

	for _, id := range []string{"_global/model", "_global/tier", "_global/permissions.mode", "_global/compaction-threshold"} {
		optionByID(t, opts, id) // must exist
	}
}

func TestConfigSurface_ExplicitLayerEffectiveValues(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, "tiers:\n  heavy:\n    model: glm-5.2\n")

	opts := f.surface.Options()

	model := optionByID(t, opts, "model")
	if model.CurrentValue != "glm-5.2" {
		t.Errorf("model currentValue = %q; want the FILE's explicit glm-5.2 (project layer wins over the floor)", model.CurrentValue)
	}
}

func TestConfigSurface_SetPersistsThroughLoader(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	_, setErr := f.surface.Set("sess-1", "model", "glm-5.2")
	if setErr != nil {
		t.Fatalf("Set(model glm-5.2): %v", setErr)
	}

	// The D-07 write half: assert by re-loading through the REAL loader, not a
	// re-parse of our own struct.
	cfg, err := modelrouting.Load(f.projectPath)
	if err != nil {
		t.Fatalf("reload written project layer: %v", err)
	}

	if cfg.Tiers["heavy"].Model != "glm-5.2" {
		t.Errorf("loaded tiers.heavy.model = %q; want the written glm-5.2", cfg.Tiers["heavy"].Model)
	}

	// The refreshed response carries the new effective value.
	opts := f.surface.Options()
	if got := optionByID(t, opts, "model").CurrentValue; got != "glm-5.2" {
		t.Errorf("post-set advertisement currentValue = %q; want glm-5.2", got)
	}
}

func TestConfigSurface_PendingNoOp(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	before := f.notifyCount()

	opts, err := f.surface.Set("sess-1", "permissions.mode", "gated")
	if err != nil {
		t.Fatalf("Set(pending permissions.mode): %v (D-05: accepted no-op, never an error)", err)
	}

	assertEight(t, opts, "pending response")

	if got := optionByID(t, opts, "permissions.mode").CurrentValue; got != "ungated" {
		t.Errorf("permissions.mode currentValue = %q; want UNCHANGED ungated", got)
	}

	if f.notifyCount() != before {
		t.Error("pending no-op emitted a config_option_update (nothing changed)")
	}

	if got := f.stderr.String(); !strings.Contains(got, "pending") {
		t.Errorf("pending set not logged (stderr=%q)", got)
	}

	// No layer file was created by a pending id.
	if _, statErr := os.Stat(f.projectPath); !os.IsNotExist(statErr) {
		t.Error("pending no-op wrote a layer file (pending ids never persist)")
	}
}

func TestConfigSurface_ConcurrentMutation(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, twoTierLayer)

	var wg sync.WaitGroup

	run := func(fn func()) {
		wg.Add(1)

		go func() {
			defer wg.Done()

			fn()
		}()
	}

	run(func() { _, _ = f.surface.ApplyBlobDefaults(map[string]json.RawMessage{"compaction-threshold": json.RawMessage(`"65"`)}) })
	run(func() { _, _ = f.surface.Set("sess-c", "model", "glm-5.2") })
	run(func() { _, _ = f.surface.Set("sess-c", "tier", "light") })

	wg.Wait()

	// No torn YAML: the real loader parses the final file.
	cfg, err := modelrouting.Load(f.projectPath)
	if err != nil {
		t.Fatalf("final layer does not parse (torn YAML?): %v", err)
	}

	// Exactly one write per key: the final on-disk values are exactly the
	// values the serialized ops wrote.
	if cfg.SessionTier != "light" {
		t.Errorf("loaded session_tier = %q; want the single tier write light", cfg.SessionTier)
	}

	if cfg.Tiers["heavy"].Model != "GLM-5.3" && cfg.Tiers["heavy"].Model != "glm-5.2" {
		t.Errorf("loaded tiers.heavy.model = %q; want the single model write's value", cfg.Tiers["heavy"].Model)
	}

	// Memory == disk (no half-applied state): the advertisement reflects what
	// the loader sees.
	opts := f.surface.Options()

	if got := optionByID(t, opts, "tier").CurrentValue; got != cfg.SessionTier {
		t.Errorf("advertised tier %q != loaded session_tier %q (half-applied state)", got, cfg.SessionTier)
	}

	if got := optionByID(t, opts, "compaction-threshold").CurrentValue; got != "65" {
		t.Errorf("advertised compaction-threshold = %q; want the blob fill 65", got)
	}
}

func TestMetaBlob_ExplicitFileWins(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, "tiers:\n  heavy:\n    model: glm-5.2\n")

	changed, err := f.surface.ApplyBlobDefaults(map[string]json.RawMessage{
		"model": json.RawMessage(`"GLM-5.3"`),
	})
	if err != nil {
		t.Fatalf("ApplyBlobDefaults: %v", err)
	}

	if changed {
		t.Error("blob application reported changed while the explicit file value won (D-10: fills-unset only)")
	}

	if got := optionByID(t, f.surface.Options(), "model").CurrentValue; got != "glm-5.2" {
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
		"model": json.RawMessage(`"glm-5.2"`),
	})
	if err != nil {
		t.Fatalf("ApplyBlobDefaults: %v", err)
	}

	if !changed {
		t.Error("blob fill over an unset slot did not report changed")
	}

	if got := optionByID(t, f.surface.Options(), "model").CurrentValue; got != "glm-5.2" {
		t.Errorf("effective model = %q; want the in-memory blob fill glm-5.2", got)
	}

	// In-memory ONLY: no layer file appeared.
	if _, statErr := os.Stat(f.projectPath); !os.IsNotExist(statErr) {
		t.Error("blob application persisted to the project layer (D-10: never persisted)")
	}

	if _, statErr := os.Stat(f.globalPath); !os.IsNotExist(statErr) {
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
		"model":    json.RawMessage(`"glm-5.2"`),
		rawKey:     rawVal,
		"weird":    json.RawMessage(`[1,{"x":null}]`),
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

	if _, err := f.surface.Set("sess-1", "_global/model", "glm-5.2"); err != nil {
		t.Fatalf("Set(_global/model): %v", err)
	}

	cfg, err := modelrouting.Load(f.globalPath)
	if err != nil {
		t.Fatalf("global layer after scoped write: %v", err)
	}

	if cfg.Tiers["heavy"].Model != "glm-5.2" {
		t.Errorf("global tiers.heavy.model = %q; want the scoped write glm-5.2", cfg.Tiers["heavy"].Model)
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
	if _, err := f.surface.Set("sess-1", "model", "glm-5.2"); err != nil {
		t.Fatalf("Set(model): %v", err)
	}

	cfg, err := modelrouting.Load(f.projectPath)
	if err != nil {
		t.Fatalf("project layer after default-scope write: %v", err)
	}

	if cfg.Tiers["heavy"].Model != "glm-5.2" {
		t.Errorf("project tiers.heavy.model = %q; want glm-5.2 (project is the default target, D-08)", cfg.Tiers["heavy"].Model)
	}

	if got := readLayerBytes(t, f.globalPath); got != unrelatedLayer {
		t.Errorf("global layer changed by a project-scoped write: %q", got)
	}
}

func TestSetIdempotent_BlobDerivedEffective(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)
	writeLayer(t, f.projectPath, unrelatedLayer)

	projectBefore := readLayerBytes(t, f.projectPath)

	// The blob fills the unset slot: the EFFECTIVE model becomes blob-derived.
	if _, err := f.surface.ApplyBlobDefaults(map[string]json.RawMessage{
		"model": json.RawMessage(`"glm-5.2"`),
	}); err != nil {
		t.Fatalf("ApplyBlobDefaults: %v", err)
	}

	// A redundant re-push of that same value (Zed re-pushing stored defaults)
	// must NOT write the layer nor promote the blob value into explicit config.
	opts, err := f.surface.Set("sess-1", "model", "glm-5.2")
	if err != nil {
		t.Fatalf("idempotent Set: %v", err)
	}

	assertEight(t, opts, "idempotent response")

	if got := readLayerBytes(t, f.projectPath); got != projectBefore {
		t.Errorf("idempotent re-push churned the layer file (D-10 guard failed):\nbefore=%q\nafter=%q", projectBefore, got)
	}

	if f.notifyCount() != 1 { // exactly the blob application's one update — the re-push added none
		t.Errorf("config_option_update count = %d; want 1 (the re-push emitted nothing — nothing changed)", f.notifyCount())
	}

	if got := strings.Count(f.stderr.String(), "idempotent"); got != 1 {
		t.Errorf("idempotent log lines = %d; want exactly 1 (stderr=%q)", got, f.stderr.String())
	}

	// The overlay fill survives (not promoted, not dropped): still effective.
	if got := optionByID(t, f.surface.Options(), "model").CurrentValue; got != "glm-5.2" {
		t.Errorf("effective model after re-push = %q; want glm-5.2 (overlay unchanged)", got)
	}
}

func TestSetIdempotent_ExplicitFileEffective(t *testing.T) {
	t.Parallel()

	f := newSurfaceFixture(t)

	if _, err := f.surface.Set("sess-1", "model", "glm-5.2"); err != nil {
		t.Fatalf("first Set: %v", err)
	}

	projectBefore := readLayerBytes(t, f.projectPath)
	notifiesBefore := f.notifyCount()

	if _, err := f.surface.Set("sess-1", "model", "glm-5.2"); err != nil {
		t.Fatalf("second Set: %v", err)
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
