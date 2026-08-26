package modelroutingcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
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
