package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/drift"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// newProfileCheckCmd builds the `ass-guard profile check <name>` subcommand
// (PROF-04, D-08). The fixture path (--capture-file) is fully unit-tested; the
// live path reads the freshest main rollout line and diffs against the manifest.
func newProfileCheckCmd() *cobra.Command {
	var (
		captureFile string
		zcodeBin    string
		profilesDir string
	)

	cmd := &cobra.Command{
		Use:          "check <name>",
		Short:        "diff a fresh capture against the profile's tiered manifest (PROF-04 drift detector)",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileCheck(args[0], profilesDir, captureFile, zcodeBin)
		},
	}
	cmd.Flags().StringVar(&captureFile, "capture-file", "", "fixture JSON capture (a model_io line) — bypasses the live path")
	cmd.Flags().StringVar(&zcodeBin, "zcode-bin", profileZcode, "zcode binary (live path; operator-gated)")
	cmd.Flags().StringVar(&profilesDir, "profiles-dir", defaultProfilesDir(), "directory containing profile bundles")

	return cmd
}

// newProfileCmd builds the `ass-guard profile` command group.
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "profile operations (drift detection, inspection)",
	}
	cmd.AddCommand(newProfileCheckCmd())

	return cmd
}

// runProfileCheck loads the profile's coverage manifest, builds a captured
// request snapshot (from --capture-file or the freshest main rollout line), and
// runs drift.Detect, reporting TIER-1/2 drifts + a structured footer to stderr.
func runProfileCheck(name, profilesDir, captureFile, zcodeBin string) error {
	manifestPath := filepath.Join(profilesDir, name, "coverage.yaml")

	manifest, err := profile.LoadCoverage(manifestPath)
	if err != nil {
		return fmt.Errorf("load coverage manifest %q: %w", manifestPath, err)
	}

	raw, err := loadCaptureLine(captureFile)
	if err != nil {
		return err
	}

	captured := extractCaptureCounts(raw)

	drifts := drift.Detect(manifest, captured)
	reportProfileCheck(name, drifts, len(manifest.Fields))

	if len(drifts) > 0 {
		return fmt.Errorf("drift detected: %d TIER-1/2 field(s) changed", len(drifts))
	}

	return nil
}

// loadCaptureLine returns the raw bytes of a model_io capture line: from
// --capture-file if set, otherwise the freshest main rollout line found on disk.
func loadCaptureLine(captureFile string) (json.RawMessage, error) {
	if captureFile != "" {
		raw, err := os.ReadFile(captureFile)
		if err != nil {
			return nil, fmt.Errorf("read capture file: %w", err)
		}

		return json.RawMessage(raw), nil
	}
	// Live path (D-08): a full implementation spawns the zcode binary to produce
	// a fresh capture. Phase-1 simplification: read the freshest main rollout
	// line on disk. The full live-spawn-via-exec needs operator testing.
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home for rollout dir: %w", err)
	}

	dir := filepath.Join(home, ".zcode", "cli", "rollout")

	stats, err := profile.ScanRolloutDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scan rollout dir (live path): %w", err)
	}

	chosen, err := profile.PickRichestMain(stats)
	if err != nil {
		return nil, fmt.Errorf("no fresh capture available; pass --capture-file or run zcode to produce a rollout: %w", err)
	}

	return readFirstLine(chosen.Path)
}

// extractCaptureCounts builds the captured map the drift detector consumes:
// field-path keys → their structural values (slices for arrays, maps for
// header-name sets) pulled out of a model_io JSON line.
func extractCaptureCounts(raw json.RawMessage) map[string]any {
	var mio profile.ModelIO

	err := json.Unmarshal(raw, &mio)
	if err != nil {
		return map[string]any{}
	}

	out := map[string]any{}

	if len(mio.Request.Body.System) > 0 {
		s := make([]any, len(mio.Request.Body.System))
		for i, b := range mio.Request.Body.System {
			s[i] = b
		}

		out["request.body.system"] = s
	}

	if len(mio.Request.Body.Tools) > 0 {
		t := make([]any, len(mio.Request.Body.Tools))
		for i, d := range mio.Request.Body.Tools {
			t[i] = d
		}

		out["request.body.tools"] = t
	}

	if len(mio.Request.Body.Thinking) > 0 {
		out["request.body.thinking"] = "present"
	}

	if len(mio.Request.Body.ToolChoice) > 0 {
		out["request.body.tool_choice"] = "present"
	}

	if len(mio.Request.Headers) > 0 {
		hdrs := map[string]any{}
		for k, v := range mio.Request.Headers {
			hdrs[k] = v
		}

		out["request.headers"] = hdrs
	}

	if len(mio.Request.Body.Model) > 0 {
		out["request.body.model"] = string(mio.Request.Body.Model)
	}

	return out
}

// reportProfileCheck writes the structured footer to stderr (PATTERNS C6).
func reportProfileCheck(profileName string, drifts []drift.Drift, manifestFields int) {
	fmt.Fprintf(os.Stderr, "=== PROFILE CHECK (PROF-04) ===\n")
	fmt.Fprintf(os.Stderr, "profile: %s\n", profileName)
	fmt.Fprintf(os.Stderr, "manifest_fields: %d\n", manifestFields)
	fmt.Fprintf(os.Stderr, "drifts: %d\n", len(drifts))

	for _, d := range drifts {
		fmt.Fprintf(os.Stderr, "  [%s] %s — %s\n", d.Tier, d.FieldPath, d.Reason)
	}

	status := "no drift"
	if len(drifts) > 0 {
		status = "DRIFT"
	}

	fmt.Fprintf(os.Stderr, "overall_status: %s\n", status)
}

// readFirstLine returns the first non-empty line of a file.
func readFirstLine(path string) (json.RawMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Read the whole first line (rollout lines can be large).
	buf := make([]byte, 0, 4096)

	chunk := make([]byte, 4096)

	for {
		n, err := f.Read(chunk)
		for i := range n {
			if chunk[i] == '\n' {
				return json.RawMessage(buf), nil
			}

			buf = append(buf, chunk[i])
		}

		if err != nil {
			break
		}
	}

	return json.RawMessage(buf), nil
}
