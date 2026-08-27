package providerfactory //nolint:testpackage // same-package test — renameFunc seam injection

import (
	"errors"
	"os"
	"path/filepath"
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

// TestConfigWrite_CreatesLayerAndRoundTripsThroughLoad: writing a nested key
// into an empty project dir creates the file at mode 0600 and the REAL loader
// resolves the written value (global layer absent, written file as the only
// overlay over the embedded default).
func TestConfigWrite_CreatesLayerAndRoundTripsThroughLoad(t *testing.T) { //nolint:paralleltest // renameFunc seam is package-global — serial family
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

	leftovers, gerr := filepath.Glob(filepath.Join(dir, tempFilePattern[:len(tempFilePattern)-len("*.tmp")])+"*")
	require.NoError(t, gerr)
	require.Empty(t, leftovers, "happy-path write leaves no temp leftovers")
}

// TestConfigWrite_PreservesUnrelatedKeys: writing onto a layer that already
// carries the full defaults fixture re-Loads equal to the original config plus
// the one changed value — compared via the LOADED STRUCTS (YAML re-marshal
// reflows, so raw bytes never compare).
func TestConfigWrite_PreservesUnrelatedKeys(t *testing.T) { //nolint:paralleltest // renameFunc seam is package-global — serial family
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
	before.Tiers["heavy"].Model = ""
	after.Tiers["heavy"].Model = ""
	require.Equal(t, before, after, "every unrelated key must survive byte-for-byte in structure")
}

// TestConfigWrite_UnwritableDirectory: a target directory that refuses writes
// yields a typed *LayerWriteError, the existing layer file stays untouched,
// and no temp leftovers remain in the directory.
func TestConfigWrite_UnwritableDirectory(t *testing.T) { //nolint:paralleltest // renameFunc seam is package-global — serial family
	if os.Geteuid() == 0 {
		t.Skip("directory chmod cannot block a root test runner")
	}

	dir := t.TempDir()
	layer := filepath.Join(dir, "config.yaml")

	original := []byte("timezone: UTC\n")
	require.NoError(t, os.WriteFile(layer, original, 0o600))

	require.NoError(t, os.Chmod(dir, 0o500), "strip the directory's write bit")
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // let TempDir cleanup remove contents

	err := WriteLayerOption(layer, []string{"timezone"}, "Asia/Tokyo")
	require.Error(t, err)

	var werr *LayerWriteError
	require.ErrorAs(t, err, &werr, "write-side failure must be a *LayerWriteError")

	got, rerr := os.ReadFile(layer)
	require.NoError(t, rerr)
	require.Equal(t, original, got, "existing layer file must be untouched")

	leftovers, gerr := filepath.Glob(filepath.Join(dir, tempFilePattern[:len(tempFilePattern)-len("*.tmp")])+"*")
	require.NoError(t, gerr)
	require.Empty(t, leftovers, "no temp leftovers in the failed directory")
}

// TestConfigWrite_CorruptLayerNeverClobbered: a layer file carrying invalid
// YAML fails with a typed *LayerReadError naming the layer path, and the
// corrupt file is left BYTE-IDENTICAL (a failed merge never destroys the
// operator's file).
func TestConfigWrite_CorruptLayerNeverClobbered(t *testing.T) { //nolint:paralleltest // renameFunc seam is package-global — serial family
	dir := t.TempDir()
	layer := filepath.Join(dir, "config.yaml")

	corrupt := []byte("tiers: [unclosed\n  bad: :: ::\n\tx: \"unterminated")
	require.NoError(t, os.WriteFile(layer, corrupt, 0o600))

	err := WriteLayerOption(layer, []string{"timezone"}, "UTC")
	require.Error(t, err)

	var rerr *LayerReadError
	require.ErrorAs(t, err, &rerr, "read/parse failure must be a *LayerReadError")
	require.Contains(t, err.Error(), layer, "the typed error must name the layer path")

	got, rerr := os.ReadFile(layer)
	require.NoError(t, rerr)
	require.Equal(t, corrupt, got, "corrupt layer must stay byte-identical — never clobbered")
}

// TestConfigWrite_RenameFailureKeepsOriginal: injecting a rename failure (the
// crash-between-marshal-and-rename window) must surface a typed write error,
// leave the ORIGINAL layer file byte-identical (no half-written state), and
// clean up the staged temp file.
func TestConfigWrite_RenameFailureKeepsOriginal(t *testing.T) { //nolint:paralleltest // swaps the package-global renameFunc seam
	dir := t.TempDir()
	layer := filepath.Join(dir, "config.yaml")

	original := []byte("timezone: UTC\n")
	require.NoError(t, os.WriteFile(layer, original, 0o600))

	restore := renameFunc
	renameFunc = func(_, _ string) error { return errors.New("injected rename failure") }
	t.Cleanup(func() { renameFunc = restore })

	err := WriteLayerOption(layer, []string{"timezone"}, "Asia/Tokyo")
	require.Error(t, err)

	var werr *LayerWriteError
	require.ErrorAs(t, err, &werr, "rename failure must be a *LayerWriteError")

	got, rerr := os.ReadFile(layer)
	require.NoError(t, rerr)
	require.Equal(t, original, got, "original must survive the crash-between-marshal-and-rename window")

	leftovers, gerr := filepath.Glob(filepath.Join(dir, tempFilePattern[:len(tempFilePattern)-len("*.tmp")])+"*")
	require.NoError(t, gerr)
	require.Empty(t, leftovers, "the staged temp file must be cleaned up after a failed rename")
}
