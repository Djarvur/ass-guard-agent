package runtime //nolint:testpackage // internal package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// D-03 carve discipline: verbatim cmd-local duplicates of the two
// provider-factory test helpers that subagent_tier_wiring_test.go (still in
// cmd until plan 15-06 relocates it to internal/runtime) calls at :92
// (writeTestModelRouting) and :112/:144 (pinEmptyHome). The originals moved to
// internal/providerfactory with the suite in plan 15-02; no shared testutil
// package is created during the carve. Plan 15-06 Task 2 retires these copies
// when their last cmd consumer leaves.

// writeTestModelRouting writes content to the PROJECT config layer
// (<workDir>/.ass-guard/config.yaml) and returns the written path.
func writeTestModelRouting(t *testing.T, workDir, content string) string {
	t.Helper()

	dir := filepath.Join(workDir, ".ass-guard")
	require.NoError(t, os.MkdirAll(dir, 0o700))

	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// pinEmptyHome pins HOME to an empty temp dir so the global config layer
// contributes nothing — every loadModelRoutingFactory-reaching test stays
// hermetic against the operator's real home (t.Setenv ⇒ these tests are not
// parallel).
func pinEmptyHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	return home
}
