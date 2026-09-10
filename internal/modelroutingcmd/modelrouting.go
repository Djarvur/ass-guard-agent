package modelroutingcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Djarvur/ass-guard-agent/kit/modelrouting"
)

// EmitResolveHuman writes the human-readable resolution to the STDERR writer
// (transport discipline — stdout stays byte-clean unless --json). In production
// cobra wires this to os.Stderr; tests redirect via SetErr.
func EmitResolveHuman(
	w io.Writer, tier, project string, primary *modelrouting.Target, fallbacks []modelrouting.Target,
) {
	proj := project
	if proj == "" {
		proj = "(global)"
	}

	_, _ = fmt.Fprintf(w, "%s [%s] -> %s/%s\n", tier, proj, primary.Provider, primary.Model)
	_, _ = fmt.Fprintf(w, "  base_url: %s\n", primary.BaseURL)
	_, _ = fmt.Fprintf(w, "  shape: %s\n", primary.Shape)
	_, _ = fmt.Fprintf(w, "  capabilities: %s\n", DescribeCapabilities(primary.Capabilities))
	_, _ = fmt.Fprintf(w, "  pricing: $%.4f/Mtok in, $%.4f/Mtok out\n",
		primary.Pricing.InputPerMToken, primary.Pricing.OutputPerMToken)

	if len(fallbacks) > 0 {
		names := make([]string, 0, len(fallbacks))
		for i := range fallbacks {
			names = append(names, fallbacks[i].Provider+"/"+fallbacks[i].Model)
		}

		_, _ = fmt.Fprintf(w, "  fallback: %s\n", strings.Join(names, ", "))
	} else {
		_, _ = fmt.Fprintf(w, "  fallback: (none)\n")
	}
}

// EmitResolveJSON writes the machine-readable resolution to the STDOUT writer —
// the ONLY stdout path in the scheduling CLI, only when --json is explicitly
// requested. In production cobra wires this to os.Stdout; tests redirect via
// SetOut.
func EmitResolveJSON(
	w io.Writer, tier, project string, primary *modelrouting.Target, fallbacks []modelrouting.Target,
) error {
	type fb struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}

	out := struct {
		Tier         string                         `json:"tier"`
		Project      string                         `json:"project"`
		Provider     string                         `json:"provider"`
		Model        string                         `json:"model"`
		BaseURL      string                         `json:"base_url"`
		Shape        string                         `json:"shape"`
		Capabilities modelrouting.CapabilityProfile `json:"capabilities"`
		Pricing      modelrouting.Pricing           `json:"pricing"`
		Fallback     []fb                           `json:"fallback"`
	}{
		Tier: tier, Project: project,
		Provider: primary.Provider, Model: primary.Model,
		BaseURL: primary.BaseURL, Shape: primary.Shape,
		Capabilities: primary.Capabilities, Pricing: primary.Pricing,
	}
	for i := range fallbacks {
		out.Fallback = append(out.Fallback, fb{Provider: fallbacks[i].Provider, Model: fallbacks[i].Model})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out) //nolint:wrapcheck // json encoder
}

// DescribeCapabilities renders a compact human form of the capability profile.
func DescribeCapabilities(c modelrouting.CapabilityProfile) string {
	parts := []string{}
	if c.ToolCalling {
		parts = append(parts, "tool_calling")
	}

	if c.Streaming {
		parts = append(parts, "streaming")
	}

	if c.ExtendedThinking {
		parts = append(parts, "extended_thinking")
	}

	parts = append(parts, fmt.Sprintf("ctx=%d", c.ContextWindow))

	return strings.Join(parts, " ")
}

// StatsRow is one aggregated (provider, model) row of the stats dump (24-02,
// D-07 CLI half), enriched with the tier binding(s) whose primary model is the
// row's model (operator context; "" when no tier binds it).
type StatsRow struct {
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
}

// LoadAndAggregate reads the outcome store at storePath tolerantly and
// aggregates it over every recorded outcome (OutcomeWindowAll). A missing
// store file is the EMPTY aggregate — no store yet is a normal state, not an
// error. The second return is the count of skipped (malformed/unknown-shape)
// lines, surfaced in the dump so a torn tail is visible to the operator.
func LoadAndAggregate(
	storePath string,
) (map[modelrouting.ProviderModelKey]modelrouting.AggregateStats, int, error) {
	records, skipped, err := modelrouting.ReadOutcomes(storePath)
	if err != nil {
		return nil, 0, fmt.Errorf("read outcome store: %w", err)
	}

	return modelrouting.Aggregate(records, modelrouting.OutcomeWindowAll), skipped, nil
}

// BuildStatsRows flattens the aggregate into deterministic rows (sorted
// provider, then model), enriching each with cfg's tier bindings: every tier
// whose PRIMARY model is the row's model, sorted and comma-joined ("" when no
// tier binds it). Operator context only — the enrichment never filters rows.
func BuildStatsRows(
	agg map[modelrouting.ProviderModelKey]modelrouting.AggregateStats, cfg *modelrouting.Config,
) []StatsRow {
	rows := make([]StatsRow, 0, len(agg))
	for key, stats := range agg {
		rows = append(rows, StatsRow{
			Provider: key.Provider, Model: key.Model,
			OK: stats.OK, Transient: stats.Transient,
			Structural: stats.Structural, Exhausted: stats.Exhausted,
			InTokens: stats.InTokens, OutTokens: stats.OutTokens, CostUSD: stats.CostUSD,
			Tier: tierBindingFor(cfg, key.Model),
		})
	}

	slices.SortFunc(rows, func(a, b StatsRow) int {
		if c := strings.Compare(a.Provider, b.Provider); c != 0 {
			return c
		}

		return strings.Compare(a.Model, b.Model)
	})

	return rows
}

// tierBindingFor returns the sorted, comma-joined tier names whose primary
// binding is model ("" when none or cfg is nil).
func tierBindingFor(cfg *modelrouting.Config, model string) string {
	if cfg == nil {
		return ""
	}

	var tiers []string
	for tier, binding := range cfg.Tiers {
		if binding.Model == model {
			tiers = append(tiers, tier)
		}
	}

	slices.Sort(tiers)

	return strings.Join(tiers, ",")
}

// EmitStatsHuman writes the human dump to w (the STDERR discipline — write
// errors are ignored, exactly like EmitResolveHuman). One line per
// (provider, model) with the outcome-class counts and token/cost totals; an
// empty aggregate prints the no-store-yet note (a normal state, not an error).
func EmitStatsHuman(w io.Writer, rows []StatsRow, storePath string, skipped int) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintf(w, "no outcomes recorded yet (store: %s)\n", storePath)

		return
	}

	_, _ = fmt.Fprintf(w, "outcomes store: %s (skipped %d lines)\n", storePath, skipped)

	for i := range rows {
		row := &rows[i]

		tier := ""
		if row.Tier != "" {
			tier = fmt.Sprintf(" (tier: %s)", row.Tier)
		}

		_, _ = fmt.Fprintf(w, "%s/%s%s: ok=%d transient=%d structural=%d exhausted=%d in=%d out=%d cost=$%.4f\n",
			row.Provider, row.Model, tier,
			row.OK, row.Transient, row.Structural, row.Exhausted,
			row.InTokens, row.OutTokens, row.CostUSD)
	}
}

// EmitStatsJSON writes the machine dump to w — the ONLY stdout path in the
// scheduling CLI, only when --json is explicitly requested — and returns the
// encode error (the EmitResolveJSON discipline). Rows are already sorted by
// BuildStatsRows, so the output is byte-deterministic.
func EmitStatsJSON(w io.Writer, rows []StatsRow, storePath string, skipped int) error {
	out := struct {
		Store   string     `json:"store"`
		Skipped int        `json:"skipped"`
		Targets []StatsRow `json:"targets"`
	}{Store: storePath, Skipped: skipped, Targets: rows}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out) //nolint:wrapcheck // json encoder
}
