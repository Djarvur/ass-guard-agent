package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// testdata paths (tests run with cwd = cmd/ass-guard/).
const (
	validSchedulingCfg  = "../../internal/scheduler/testdata/valid.yaml"
	invalidSchedulingCfg = "../../internal/scheduler/testdata/invalid_cap_mismatch.yaml"
)

// runSchedulingCmd executes the scheduling command tree with the given args,
// capturing stdout + stderr. Returns the cobra error (nil on success).
func runSchedulingCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := &cobra.Command{Use: "ass-guard", SilenceUsage: true}
	root.AddCommand(newSchedulingCmd())
	root.SetArgs(append([]string{"scheduling"}, args...))
	var out, errs bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errs)
	err = root.Execute()
	return out.String(), errs.String(), err
}

// TestSchedulingValidateValid: validate on a valid config exits 0 + prints
// "scheduling config valid" to STDERR.
func TestSchedulingValidateValid(t *testing.T) {
	stdout, stderr, err := runSchedulingCmd(t, "validate", "--config", validSchedulingCfg)
	require.NoError(t, err)
	require.Empty(t, stdout, "validate must not write to stdout (transport discipline)")
	require.Contains(t, stderr, "scheduling config valid")
}

// TestSchedulingValidateInvalid: validate on an inconsistent config returns a
// non-nil error + the ConfigError report on STDERR (D-10 operator surface).
func TestSchedulingValidateInvalid(t *testing.T) {
	stdout, stderr, err := runSchedulingCmd(t, "validate", "--config", invalidSchedulingCfg)
	require.Error(t, err, "inconsistent config must exit non-zero")
	require.Empty(t, stdout, "validate must not write to stdout even on failure")
	require.Contains(t, stderr, "scheduling config invalid")
	require.Contains(t, stderr, "tool_calling", "the ConfigError report names the unmet capability")
}

// TestSchedulingResolveJSON: resolve --json writes a JSON object to STDOUT with
// the resolved model.
func TestSchedulingResolveJSON(t *testing.T) {
	stdout, stderr, err := runSchedulingCmd(t, "resolve", "--tier", "heavy",
		"--config", validSchedulingCfg,
		"--at", "2026-08-16T12:00:00-04:00", // Sunday noon NY → global table → glm-5.2
		"--json")
	require.NoError(t, err)
	require.NotEmpty(t, stdout, "--json must write to stdout")
	require.Empty(t, stderr, "--json must not write to stderr on success")

	var got struct {
		Tier     string `json:"tier"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Shape    string `json:"shape"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, "heavy", got.Tier)
	require.Equal(t, "glm-5.2", got.Model, "Sunday noon resolves the global heavy primary")
	require.Equal(t, "anthropic", got.Provider)
	require.Equal(t, "anthropic", got.Shape)
}

// TestSchedulingResolveHumanGoesToStderr: resolve WITHOUT --json writes NOTHING
// to stdout (the human form goes to stderr — transport discipline, pitfall 9).
func TestSchedulingResolveHumanGoesToStderr(t *testing.T) {
	stdout, stderr, err := runSchedulingCmd(t, "resolve", "--tier", "heavy",
		"--config", validSchedulingCfg,
		"--at", "2026-08-16T12:00:00-04:00")
	require.NoError(t, err)
	require.Empty(t, stdout, "human-readable resolve must NOT write to stdout (transport discipline)")
	require.NotEmpty(t, stderr, "human-readable form goes to stderr")
	require.True(t, strings.Contains(stderr, "heavy"), "stderr shows the tier")
	require.True(t, strings.Contains(stderr, "glm-5.2"), "stderr shows the resolved model")
}

// TestSchedulingResolvePeakWindow: resolve at peak time (Monday 10:00 NY)
// returns the window's heavy pick (minimax-m3), proving the D-02 precedence is
// honored through the CLI.
func TestSchedulingResolvePeakWindow(t *testing.T) {
	stdout, _, err := runSchedulingCmd(t, "resolve", "--tier", "heavy",
		"--config", validSchedulingCfg,
		"--at", "2026-08-10T10:00:00-04:00", // Monday 10:00 NY → peak → minimax-m3
		"--json")
	require.NoError(t, err)
	var got struct {
		Model string `json:"model"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, "minimax-m3", got.Model, "peak window heavy pick wins (D-02)")
}
