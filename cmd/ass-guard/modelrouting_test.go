package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
)

// testdata paths (tests run with cwd = cmd/ass-guard/).
const (
	validSchedulingCfg   = "../../internal/modelrouting/testdata/valid.yaml"
	invalidSchedulingCfg = "../../internal/modelrouting/testdata/invalid_cap_mismatch.yaml"
)

// runSchedulingCmd executes the scheduling command tree with the given args,
// capturing stdout + stderr. Returns the cobra error (nil on success).
//
//nolint:gocritic // conflicts w/ nonamedreturns
func runSchedulingCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	root := &cobra.Command{Use: "ass-guard", SilenceUsage: true}
	root.AddCommand(newModelRoutingCmd())
	root.SetArgs(append([]string{"model-routing"}, args...))

	var out, errs bytes.Buffer

	root.SetOut(&out)
	root.SetErr(&errs)
	err := root.Execute()

	return out.String(), errs.String(), err
}

// TestModelRoutingValidateValid: validate on a valid config exits 0 + prints
// "scheduling config valid" to STDERR.
func TestModelRoutingValidateValid(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runSchedulingCmd(t, "validate", "--config", validSchedulingCfg)
	require.NoError(t, err)
	require.Empty(t, stdout, "validate must not write to stdout (transport discipline)")
	require.Contains(t, stderr, "scheduling config valid")
}

// TestModelRoutingValidateInvalid: validate on an inconsistent config returns a
// non-nil error + the ConfigError report on STDERR (D-10 operator surface).
func TestModelRoutingValidateInvalid(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runSchedulingCmd(t, "validate", "--config", invalidSchedulingCfg)
	require.Error(t, err, "inconsistent config must exit non-zero")
	require.Empty(t, stdout, "validate must not write to stdout even on failure")
	require.Contains(t, stderr, "scheduling config invalid")
	require.Contains(t, stderr, "tool_calling", "the ConfigError report names the unmet capability")
}

// TestModelRoutingResolveJSON: resolve --json writes a JSON object to STDOUT with
// the resolved model.
func TestModelRoutingResolveJSON(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runSchedulingCmd(t, "resolve", "--tier", tierHeavy,
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
	require.Equal(t, tierHeavy, got.Tier)
	require.Equal(t, "glm-5.2", got.Model, "Sunday noon resolves the global heavy primary")
	require.Equal(t, "anthropic", got.Provider)
	require.Equal(t, "anthropic", got.Shape)
}

// TestModelRoutingResolveHumanGoesToStderr: resolve WITHOUT --json writes NOTHING
// to stdout (the human form goes to stderr — transport discipline, pitfall 9).
func TestModelRoutingResolveHumanGoesToStderr(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runSchedulingCmd(t, "resolve", "--tier", tierHeavy,
		"--config", validSchedulingCfg,
		"--at", "2026-08-16T12:00:00-04:00")
	require.NoError(t, err)
	require.Empty(t, stdout, "human-readable resolve must NOT write to stdout (transport discipline)")
	require.NotEmpty(t, stderr, "human-readable form goes to stderr")
	require.Contains(t, stderr, tierHeavy, "stderr shows the tier")
	require.Contains(t, stderr, "glm-5.2", "stderr shows the resolved model")
}

// TestModelRoutingResolvePeakWindow: resolve at peak time (Monday 10:00 NY)
// returns the window's heavy pick (minimax-m3), proving the D-02 precedence is
// honored through the CLI.
func TestModelRoutingResolvePeakWindow(t *testing.T) {
	t.Parallel()
	stdout, _, err := runSchedulingCmd(t, "resolve", "--tier", tierHeavy,
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

// --- 24-02 (TAIL-01, D-07 CLI half): ass-guard model-routing stats ---

// seedStatsStore appends a fixed four-record evidence set to a fresh store
// under a temp root: 2x ok + 1x transient on (anthropic, GLM-5.3) with
// token/cost-bearing records, 1x structural on (anthropic, glm-5.2). Returns
// the store.
func seedStatsStore(t *testing.T) *modelrouting.OutcomeStore {
	t.Helper()

	store, err := modelrouting.NewOutcomeStore(t.TempDir())
	require.NoError(t, err, "NewOutcomeStore")

	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

	seeded := []modelrouting.DispatchOutcome{
		{At: base, Provider: "anthropic", Model: "GLM-5.3", Tier: "heavy",
			Outcome: modelrouting.OutcomeOK, InTokens: 100, OutTokens: 50, CostUSD: 0.06, Origin: modelrouting.OutcomeOriginTurn},
		{At: base.Add(time.Second), Provider: "anthropic", Model: "GLM-5.3", Tier: "heavy",
			Outcome: modelrouting.OutcomeOK, InTokens: 200, OutTokens: 100, CostUSD: 0.12, Origin: modelrouting.OutcomeOriginTurn},
		{At: base.Add(2 * time.Second), Provider: "anthropic", Model: "GLM-5.3", Tier: "heavy",
			Outcome: modelrouting.OutcomeTransient, Origin: modelrouting.OutcomeOriginSubagent},
		{At: base.Add(3 * time.Second), Provider: "anthropic", Model: "glm-5.2", Tier: "light",
			Outcome: modelrouting.OutcomeStructural, InTokens: 10, OutTokens: 5, Origin: modelrouting.OutcomeOriginTurn},
	}

	for i := range seeded {
		require.NoError(t, store.Append(seeded[i]), "Append seed record")
	}

	return store
}

// TestSchedulingStatsHuman: stats over a seeded temp store prints one line per
// (provider, model) with the outcome-class counts and token/cost totals to
// STDERR; STDOUT stays byte-empty (the standing C1 discipline).
func TestSchedulingStatsHuman(t *testing.T) {
	t.Parallel()

	store := seedStatsStore(t)

	stdout, stderr, err := runSchedulingCmd(t, "stats", "--store", store.Path())
	require.NoError(t, err)
	require.Empty(t, stdout, "human stats must NOT write to stdout (transport discipline)")
	require.Contains(t, stderr, "anthropic/GLM-5.3", "one line per (provider, model)")
	require.Contains(t, stderr, "anthropic/glm-5.2", "the second key's line")
	require.Contains(t, stderr, "ok=2", "GLM-5.3 ok count")
	require.Contains(t, stderr, "transient=1", "GLM-5.3 transient count")
	require.Contains(t, stderr, "structural=1", "glm-5.2 structural count")
	require.Contains(t, stderr, "exhausted=0", "the exhausted class is present even at zero")
	require.Contains(t, stderr, "in=300", "GLM-5.3 summed input tokens")
	require.Contains(t, stderr, "out=150", "GLM-5.3 summed output tokens")
	require.Contains(t, stderr, "$0.1800", "GLM-5.3 summed cost")
	require.Contains(t, stderr, "tier: heavy", "the configured tier binding rides the row")
}

// TestSchedulingStatsJSON: stats --json prints the aggregate JSON to STDOUT
// and writes NOTHING to stderr; the JSON parses and round-trips the seeded
// counts exactly.
func TestSchedulingStatsJSON(t *testing.T) {
	t.Parallel()

	store := seedStatsStore(t)

	stdout, stderr, err := runSchedulingCmd(t, "stats", "--store", store.Path(), "--json")
	require.NoError(t, err)
	require.Empty(t, stderr, "--json must not write to stderr on success")

	var got struct {
		Store   string `json:"store"`
		Skipped int    `json:"skipped"`
		Targets []struct {
			Provider   string  `json:"provider"`
			Model      string  `json:"model"`
			Tier       string  `json:"tier"`
			OK         int     `json:"ok"`
			Transient  int     `json:"transient"`
			Structural int     `json:"structural"`
			Exhausted  int     `json:"exhausted"`
			InTokens   int64   `json:"in_tokens"`
			OutTokens  int64   `json:"out_tokens"`
			CostUSD    float64 `json:"cost_usd"`
		} `json:"targets"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, store.Path(), got.Store)
	require.Equal(t, 0, got.Skipped)
	require.Len(t, got.Targets, 2, "one row per (provider, model)")

	byModel := map[string]int{}
	for i, row := range got.Targets {
		byModel[row.Model] = i
	}

	primary := got.Targets[byModel["GLM-5.3"]]
	require.Equal(t, "anthropic", primary.Provider)
	require.Equal(t, "heavy", primary.Tier, "the floor config binds GLM-5.3 as heavy")
	require.Equal(t, 2, primary.OK)
	require.Equal(t, 1, primary.Transient)
	require.Equal(t, 0, primary.Structural)
	require.Equal(t, 0, primary.Exhausted)
	require.Equal(t, int64(300), primary.InTokens)
	require.Equal(t, int64(150), primary.OutTokens)
	require.InDelta(t, 0.18, primary.CostUSD, 1e-9)

	secondary := got.Targets[byModel["glm-5.2"]]
	require.Equal(t, 1, secondary.Structural)
	require.Equal(t, int64(10), secondary.InTokens)
	require.Equal(t, int64(5), secondary.OutTokens)
}

// TestSchedulingStatsEmptyStore: stats against a nonexistent store path exits
// 0 with an empty-aggregate human note — no store yet is a normal state, not
// an error.
func TestSchedulingStatsEmptyStore(t *testing.T) {
	t.Parallel()

	missing := t.TempDir() + "/nope/routing/outcomes.jsonl"

	stdout, stderr, err := runSchedulingCmd(t, "stats", "--store", missing)
	require.NoError(t, err, "a missing store is a normal state (exit 0)")
	require.Empty(t, stdout, "human stats must NOT write to stdout")
	require.Contains(t, stderr, "no outcomes", "the empty-aggregate note")
	require.Contains(t, stderr, missing, "the note names the store path")
}

// TestSchedulingStatsStoreFlagRoutes: --store routes the dump at the flagged
// location — a store seeded at a NON-default path is the one aggregated (the
// flag is a real override, not a decoration).
func TestSchedulingStatsStoreFlagRoutes(t *testing.T) {
	t.Parallel()

	store := seedStatsStore(t)

	stdout, _, err := runSchedulingCmd(t, "stats", "--store", store.Path(), "--json")
	require.NoError(t, err)

	var got struct {
		Targets []struct {
			Model string `json:"model"`
		} `json:"targets"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got.Targets, 2, "the flagged store's keys — not some default location's")
}
