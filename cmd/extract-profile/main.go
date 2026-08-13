// Command extract-profile is the dev tool that regenerates a profile artifact
// from zcode's on-disk rollout logs (MIMC-02, D-16). It scans the rollout
// directory at execution time, picks the richest main session, extracts the
// TIER-1/2 request shape, asserts within-session stability, and writes the
// profile bundle + coverage manifest (PROF-05) + target_capture_ref (PROF-03).
//
// Per D-16: it does NOT hardcode session IDs or tool counts. The session is
// picked live; the tool count is whatever the source declares.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

const mnd60 = 60
const dirPerm = 0o750
const filePermDefault = 0o600

const extractorVersion = "extract-profile/01-02"

func main() {
	sessions := flag.String(
		"sessions", "",
		"comma-separated session IDs to extract from (default: auto-pick the richest main session)",
	)
	rolloutDir := flag.String("rollout-dir", defaultRolloutDir(),
		"directory containing model-io-sess_*.jsonl files")
	out := flag.String("out", "profiles/zcode", "output profile directory")
	paritySession := flag.String(
		"parity-session", "",
		"optional disjoint session id recorded as the held-out parity reference (D-16)",
	)
	profileName := flag.String("name", "zcode", "profile name")

	flag.Parse()

	err := run(*sessions, *rolloutDir, *out, *profileName, *paritySession)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract-profile: %v\n", err)
		os.Exit(1)
	}
}

func defaultRolloutDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".zcode", "cli", "rollout")
	}

	return ".zcode/cli/rollout"
}

func run(sessions, rolloutDir, out, name, paritySession string) error {
	stats, err := profile.ScanRolloutDir(rolloutDir)
	if err != nil {
		return fmt.Errorf("scan %q: %w", rolloutDir, err)
	}

	if len(stats) == 0 {
		return fmt.Errorf("no model-io-sess_*.jsonl files in %q", rolloutDir) //nolint:err113 // dynamic error message
	}

	printStats(stats)

	chosen, err := chooseSession(stats, sessions)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "extract-profile: extracting from %s session %s (%s)\n",
		chosen.Role, chosen.ID, short(chosen.Path))

	res, err := profile.ExtractFromRollout(chosen.Path)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	err = writeArtifact(out, name, &res, paritySession)
	if err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}

	fmt.Fprintf(os.Stderr,
		"extract-profile: wrote %s — %d system blocks, %d tools (%d after null-filter), "+
			"%d identity headers, model %s\n",
		out, len(res.System), res.ToolCount, res.ToolCount, len(res.Headers), res.Model)
	fmt.Fprintf(os.Stderr,
		"extract-profile: coverage.yaml + meta.yaml written (PROF-03 target_capture_ref, PROF-05 manifest)\n")

	return nil
}

func chooseSession(stats []profile.SessionStat, sessions string) (profile.SessionStat, error) {
	if sessions == "" {
		return profile.PickRichestMain(stats) //nolint:wrapcheck // profile selection
	}

	want := strings.Split(sessions, ",")
	for _, s := range stats {
		for _, w := range want {
			if s.ID == strings.TrimSpace(w) {
				return s, nil
			}
		}
	}

	//nolint:err113 // dynamic error message
	return profile.SessionStat{}, fmt.Errorf("requested session(s) %q not found in rollout dir", sessions)
}

func writeArtifact(out, name string, res *profile.ExtractResult, paritySession string) error {
	err := os.MkdirAll(filepath.Join(out, "system"), dirPerm)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}
	// system blocks
	for i, b := range res.System {
		blockPath := filepath.Join(out, "system", fmt.Sprintf("block-%d.txt", i))

		err := os.WriteFile(blockPath, []byte(b.Text), filePermDefault)
		if err != nil {
			return fmt.Errorf("call: %w", err)
		}
	}
	// tools.json
	toolsJSON, err := json.MarshalIndent(res.Tools, "", "  ")
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	err = os.WriteFile(filepath.Join(out, "tools.json"), toolsJSON, filePermDefault)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}
	// thinking + tool_choice
	err = os.WriteFile(filepath.Join(out, "thinking.json"), res.Thinking, filePermDefault)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	err = os.WriteFile(filepath.Join(out, "tool_choice.json"), res.ToolChoice, filePermDefault)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}
	// profile.yaml
	profileYAML, err := yaml.Marshal(map[string]any{
		"name":       name,
		"model":      res.Model,
		"max_tokens": res.MaxTokens,
	})
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	err = os.WriteFile(filepath.Join(out, "profile.yaml"), profileYAML, filePermDefault)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}
	// identity.yaml (header names byte-faithful, values templated)
	idMap := map[string]any{"headers": headersToYAML(res.Headers)}

	idYAML, err := yaml.Marshal(idMap)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	err = os.WriteFile(filepath.Join(out, "identity.yaml"), idYAML, filePermDefault)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}
	// coverage.yaml (PROF-05 manifest)
	manifest := buildManifest(name, res)

	covYAML, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	err = os.WriteFile(filepath.Join(out, "coverage.yaml"), covYAML, filePermDefault)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}
	// meta.yaml (PROF-03 target_capture_ref)
	meta, err := yaml.Marshal(buildMeta(name, res, paritySession))
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	err = os.WriteFile(filepath.Join(out, "meta.yaml"), meta, filePermDefault)
	if err != nil {
		return fmt.Errorf("write meta.yaml: %w", err)
	}

	return nil
}

func headersToYAML(hs []profile.Header) []map[string]string {
	out := make([]map[string]string, 0, len(hs))
	for _, h := range hs {
		out = append(out, map[string]string{"name": h.Name, "value_template": h.ValueTemplate})
	}

	return out
}

func buildManifest(name string, res *profile.ExtractResult) profile.CoverageManifest {
	src := res.SourcePath
	t1 := profile.Tier1ByteFaithful
	fields := []profile.CoverageEntry{
		{Path: "request.body.system", Tier: t1, Type: "[]TextBlock", ObservedCount: len(res.System), Source: src},
		{Path: "request.body.tools", Tier: t1, Type: "[]Decl", ObservedCount: res.ToolCount, Source: src},
		{Path: "request.body.thinking", Tier: t1, Type: "json", ObservedCount: 1, Source: src},
		{Path: "request.body.tool_choice", Tier: t1, Type: "json", ObservedCount: 1, Source: src},
		{
			Path: "request.headers", Tier: profile.Tier2Structural,
			Type: "[]Header (names)", ObservedCount: len(res.Headers), Source: src,
		},
		{Path: "request.body.model", Tier: t1, Type: "string", ObservedCount: 1, Source: src},
	}

	return profile.CoverageManifest{
		Profile:          name,
		TargetCaptureRef: buildTargetCaptureRef(res),
		ExtractedAt:      time.Now().UTC(),
		ExtractorVersion: extractorVersion,
		Fields:           fields,
		// RequiredToolsSatisfiedByCatalog is set by Plan 01-03's TOOL-03 check.
		RequiredToolsSatisfiedByCatalog: false,
	}
}

func buildMeta(name string, res *profile.ExtractResult, paritySession string) map[string]any {
	m := map[string]any{
		"profile":            name,
		"extractor_version":  extractorVersion,
		"extracted_at":       time.Now().UTC().Format(time.RFC3339),
		"target_capture_ref": buildTargetCaptureRef(res),
		"data_source_strategy": "D-16: scan rollout dir at extraction time; " +
			"tool count = source-declared (not hardcoded)",
	}
	if paritySession != "" {
		m["parity_reference_session_id"] = paritySession
		m["parity_note"] = "disjoint session supplying divergence-prone multi-tool turns " +
			"(RESEARCH-FLAG-01 held-out split)"
	}

	return m
}

func buildTargetCaptureRef(res *profile.ExtractResult) profile.TargetCaptureRef {
	ref := profile.TargetCaptureRef{
		ExtractedAt:      time.Now().UTC(),
		ExtractorVersion: extractorVersion,
		Sessions: []profile.SessionRef{{
			ID:   res.SessionID,
			Path: res.SourcePath,
			Role: res.SessionRole,
		}},
	}

	return ref
}

func printStats(stats []profile.SessionStat) {
	fmt.Fprintf(os.Stderr, "extract-profile: rollout sessions found (D-16 live scan):\n")

	for _, s := range stats {
		fmt.Fprintf(os.Stderr, "  %s  role=%s fullReqLines=%d firstToolCount=%d\n",
			s.ID, s.Role, s.FullRequestLines, s.FirstToolCount)
	}
}

func short(p string) string {
	if len(p) > mnd60 {
		return "..." + p[len(p)-57:]
	}

	return p
}
