package providerfactory //nolint:testpackage // same-package test — renameFunc seam injection

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
)

// The TestConfigWrite family proves the D-07 write primitive
// (WriteLayerOption): atomic replace, hard 0600, failure transparency, and
// inverse-compatibility with the REAL loader (modelrouting.Load), never a
// local re-parse. The tests stay SERIAL (no t.Parallel) — the rename-failure
// case swaps the package-global renameFunc seam, which would race under
// parallel execution.

// roundTripWrittenValue is the slug the round-trip cases write into
// tiers.heavy.model. It must be DECLARED in the embedded default's models map
// or modelrouting.Load's validation rejects the written layer — and it must
// DIFFER from the floor's heavy primary (GLM-5.3) so a passing round-trip
// proves the written value won, not that the default echoed back.
const roundTripWrittenValue = "glm-5.2"

// testKeyTimezone is the key path leaf used by the failure-transparency cases.
const testKeyTimezone = "timezone"

// errInjectedRename is the static error the seam injects in place of os.Rename.
var errInjectedRename = errors.New("injected rename failure")

// tempLeftovers globs the writer's temp-file pattern prefix inside dir — the
// no-half-written-state assertion (T-16-10).
func tempLeftovers(dir string) []string {
	prefix := strings.TrimSuffix(tempFilePattern, "*.tmp")

	matches, _ := filepath.Glob(filepath.Join(dir, prefix+"*"))

	return matches
}

// TestConfigWrite_CreatesLayerAndRoundTripsThroughLoad: writing a nested key
// into an empty project dir creates the file at mode 0600 and the REAL loader
// resolves the written value (global layer absent, written file as the only
// overlay over the embedded default).
//
//nolint:paralleltest // renameFunc seam is package-global — serial family
func TestConfigWrite_CreatesLayerAndRoundTripsThroughLoad(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "config.yaml")

	require.NoError(t, WriteLayerOption(layer, []string{"tiers", "heavy", "model"}, roundTripWrittenValue))

	info, err := os.Stat(layer)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "written layer must be exactly 0600")

	// Round-trip through the REAL loader — not a local re-parse of the bytes.
	cfg, err := modelrouting.Load(layer)
	require.NoError(t, err)
	require.Equal(t, roundTripWrittenValue, cfg.Tiers["heavy"].Model,
		"Load must resolve the written value at the written key path")

	require.Empty(t, tempLeftovers(dir), "happy-path write leaves no temp leftovers")
}

// TestConfigWrite_PreservesUnrelatedKeys: writing onto a layer that already
// carries the full defaults fixture re-Loads equal to the original config plus
// the one changed value — compared via the LOADED STRUCTS (YAML re-marshal
// reflows, so raw bytes never compare).
//
//nolint:paralleltest // renameFunc seam is package-global — serial family
func TestConfigWrite_PreservesUnrelatedKeys(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "config.yaml")

	require.NoError(t, os.WriteFile(layer, modelrouting.EmbeddedDefaultScheduling(), 0o600))

	before, err := modelrouting.Load(layer)
	require.NoError(t, err)

	require.NoError(t, WriteLayerOption(layer, []string{"tiers", "heavy", "model"}, roundTripWrittenValue))

	after, err := modelrouting.Load(layer)
	require.NoError(t, err)

	require.Equal(t, roundTripWrittenValue, after.Tiers["heavy"].Model,
		"the written value must survive the merge into the existing layer")

	// Neutralize the ONE intentionally-changed value; everything else must be
	// struct-identical (deep-merge preserved unrelated keys, Load-for-Load).
	bBinding := before.Tiers["heavy"]
	bBinding.Model = ""
	before.Tiers["heavy"] = bBinding

	aBinding := after.Tiers["heavy"]
	aBinding.Model = ""
	after.Tiers["heavy"] = aBinding

	require.Equal(t, before, after, "every unrelated key must survive byte-for-byte in structure")
}

// TestConfigWrite_UnwritableDirectory: a target directory that refuses writes
// yields a typed *LayerWriteError, the existing layer file stays untouched,
// and no temp leftovers remain in the directory.
//
//nolint:paralleltest // renameFunc seam is package-global — serial family
func TestConfigWrite_UnwritableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory chmod cannot block a root test runner")
	}

	dir := t.TempDir()
	layer := filepath.Join(dir, "config.yaml")

	original := []byte("timezone: UTC\n")
	require.NoError(t, os.WriteFile(layer, original, 0o600))

	require.NoError(t, os.Chmod(dir, 0o500), "strip the directory's write bit")
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // let TempDir cleanup remove contents

	err := WriteLayerOption(layer, []string{testKeyTimezone}, "Asia/Tokyo")
	require.Error(t, err)

	var werr *LayerWriteError
	require.ErrorAs(t, err, &werr, "write-side failure must be a *LayerWriteError")

	got, rerr := os.ReadFile(layer)
	require.NoError(t, rerr)
	require.Equal(t, original, got, "existing layer file must be untouched")

	require.Empty(t, tempLeftovers(dir), "no temp leftovers in the failed directory")
}

// TestConfigWrite_CorruptLayerNeverClobbered: a layer file carrying invalid
// YAML fails with a typed *LayerReadError naming the layer path, and the
// corrupt file is left BYTE-IDENTICAL (a failed merge never destroys the
// operator's file).
//
//nolint:paralleltest // renameFunc seam is package-global — serial family
func TestConfigWrite_CorruptLayerNeverClobbered(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "config.yaml")

	corrupt := []byte("tiers: [unclosed\n  bad: :: ::\n\tx: \"unterminated")
	require.NoError(t, os.WriteFile(layer, corrupt, 0o600))

	err := WriteLayerOption(layer, []string{testKeyTimezone}, "UTC")
	require.Error(t, err)

	var rerr *LayerReadError
	require.ErrorAs(t, err, &rerr, "read/parse failure must be a *LayerReadError")
	require.Contains(t, err.Error(), layer, "the typed error must name the layer path")

	got, gerr := os.ReadFile(layer)
	require.NoError(t, gerr)
	require.Equal(t, corrupt, got, "corrupt layer must stay byte-identical — never clobbered")
}

// testKeySessionTier is the additive tier option key (16-05's menu exposes it).
const testKeySessionTier = "session_tier"

// testTierLight is the tier value the session_tier round-trip cases write.
const testTierLight = "light"

// TestConfigWrite_SessionTierRoundTrip: the FULL persist→read path for the
// tier option — a session_tier write into a layer file resolves through the
// REAL loader, and a project-layer write never touches the global layer file
// (byte comparison, D-08 two-layer groundwork).
//
//nolint:paralleltest // renameFunc seam is package-global — serial family
func TestConfigWrite_SessionTierRoundTrip(t *testing.T) {
	t.Run("write global session_tier then Load resolves light", func(t *testing.T) {
		global := filepath.Join(t.TempDir(), "config.yaml")

		require.NoError(t, WriteLayerOption(global, []string{testKeySessionTier}, testTierLight))

		cfg, err := modelrouting.Load(global)
		require.NoError(t, err)
		require.Equal(t, testTierLight, cfg.SessionTier,
			"the full persist→read path must resolve the written tier option")
	})

	t.Run("project write leaves the global layer byte-identical", func(t *testing.T) {
		globalDir := t.TempDir()
		globalPath := filepath.Join(globalDir, "config.yaml")

		globalBytes := []byte("session_tier: heavy\n")
		require.NoError(t, os.WriteFile(globalPath, globalBytes, 0o600))

		projectPath := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, WriteLayerOption(projectPath, []string{testKeySessionTier}, testTierLight))

		got, err := os.ReadFile(globalPath)
		require.NoError(t, err)
		require.Equal(t, globalBytes, got,
			"a project-layer write must never modify the global layer file")
	})
}

// The compaction key path (19-05/D-03): both leaves under the compaction map.
const (
	testKeyCompaction = "compaction"

	testKeyCompactionThreshold = "threshold_pct"
	testKeyCompactionEnabled   = "enabled"
)

// TestConfigWrite_CompactionRoundTrip: the FULL persist→read path for the
// compaction policy keys (19-05) — the written value resolves through the
// REAL loader, the other leaf keeps the floor default, and a project-layer
// write never touches the global layer file (byte comparison, the session_tier
// precedent's layer isolation).
//
//nolint:paralleltest // renameFunc seam is package-global — serial family
func TestConfigWrite_CompactionRoundTrip(t *testing.T) {
	t.Run("global threshold_pct write then Load resolves 60", func(t *testing.T) {
		global := filepath.Join(t.TempDir(), "config.yaml")

		require.NoError(t, WriteLayerOption(global,
			[]string{testKeyCompaction, testKeyCompactionThreshold}, 60))

		cfg, err := modelrouting.Load(global)
		require.NoError(t, err)
		require.Equal(t, 60, cfg.Compaction.ThresholdPct,
			"the full persist→read path must resolve the written threshold")
		require.True(t, cfg.Compaction.Enabled,
			"the untouched leaf keeps the floor's enabled: true default")
	})

	t.Run("project enabled write then Load resolves false", func(t *testing.T) {
		project := filepath.Join(t.TempDir(), "config.yaml")

		require.NoError(t, WriteLayerOption(project,
			[]string{testKeyCompaction, testKeyCompactionEnabled}, false))

		cfg, err := modelrouting.Load(project)
		require.NoError(t, err)
		require.False(t, cfg.Compaction.Enabled,
			"the full persist→read path must resolve the written enabled flag")
		require.Equal(t, 80, cfg.Compaction.ThresholdPct,
			"the untouched leaf keeps the floor's threshold_pct: 80 default")
	})

	t.Run("project write leaves the global layer byte-identical", func(t *testing.T) {
		globalDir := t.TempDir()
		globalPath := filepath.Join(globalDir, "config.yaml")

		globalBytes := []byte("compaction:\n  threshold_pct: 95\n")
		require.NoError(t, os.WriteFile(globalPath, globalBytes, 0o600))

		projectPath := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, WriteLayerOption(projectPath,
			[]string{testKeyCompaction, testKeyCompactionThreshold}, 60))

		got, err := os.ReadFile(globalPath)
		require.NoError(t, err)
		require.Equal(t, globalBytes, got,
			"a project-layer compaction write must never modify the global layer file")
	})
}

// TestConfigWrite_RenameFailureKeepsOriginal: injecting a rename failure (the
// crash-between-marshal-and-rename window) must surface a typed write error,
// leave the ORIGINAL layer file byte-identical (no half-written state), and
// clean up the staged temp file.
//
//nolint:paralleltest // swaps the package-global renameFunc seam
func TestConfigWrite_RenameFailureKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "config.yaml")

	original := []byte("timezone: UTC\n")
	require.NoError(t, os.WriteFile(layer, original, 0o600))

	restore := renameFunc
	renameFunc = func(_, _ string) error { return errInjectedRename }

	t.Cleanup(func() { renameFunc = restore })

	err := WriteLayerOption(layer, []string{testKeyTimezone}, "Asia/Tokyo")
	require.Error(t, err)

	var werr *LayerWriteError
	require.ErrorAs(t, err, &werr, "rename failure must be a *LayerWriteError")

	got, rerr := os.ReadFile(layer)
	require.NoError(t, rerr)
	require.Equal(t, original, got, "original must survive the crash-between-marshal-and-rename window")

	require.Empty(t, tempLeftovers(dir), "the staged temp file must be cleaned up after a failed rename")
}
