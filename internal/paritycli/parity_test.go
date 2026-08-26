package paritycli //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
)

// pinVersion is the coverage manifest's pinned zcode_version in the parity
// fixtures; the drift-warning tests fake the INSTALLED side around it.
const pinVersion = "0.16.3"

// driftWarningMarker is the warning line's stable substring every drift test
// keys on (Task 1's "a warning line matching `zcode version drift`").
const driftWarningMarker = "zcode version drift"

// probeToolRead names the captured Read tool in the wiring-test compositions
// (goconst discipline: no bare tool-name literals).
const probeToolRead = "Read"

// writeParityProfileFixture builds a minimal loadable profile bundle under
// <root>/zcode/ (profile.yaml, system/block-0.txt, tools.json, identity.yaml,
// thinking.json, tool_choice.json) whose coverage.yaml pins zcodeVersion as
// the capture provenance, and returns the profiles root.
func writeParityProfileFixture(t *testing.T, zcodeVersion string) string {
	t.Helper()

	root := t.TempDir()
	pdir := filepath.Join(root, profileZcode)

	err := os.MkdirAll(filepath.Join(pdir, "system"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	coverage := "profile: " + profileZcode + "\ntarget_capture_ref:\n" +
		"  zcode_version: \"" + zcodeVersion + "\"\nfields: []\n"

	files := map[string]string{
		"profile.yaml":       "name: " + profileZcode + "\nmodel: ph-model\nmax_tokens: 128\n",
		"system/block-0.txt": "ph-sys-0",
		"tools.json":         "[]",
		"identity.yaml":      "headers: []\n",
		"thinking.json":      "{}",
		"tool_choice.json":   "{}",
		"coverage.yaml":      coverage,
	}

	for name, body := range files {
		err := os.WriteFile(filepath.Join(pdir, name), []byte(body), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root
}

// writeParitySuiteFixture writes a one-turn replay-suite JSON and returns its
// path (the drift tests fake the A/B run itself — the suite only needs to load).
func writeParitySuiteFixture(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "suite.json")
	body := `[{"turn_id":"ph-01","prompt":"ph","expected_tool_calls":[]}]`

	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return path
}

// captureParityStderr runs fn with os.Stderr captured (the house pipe idiom)
// and returns everything fn wrote plus its error.
func captureParityStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	var buf bytes.Buffer

	old := os.Stderr

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}

	os.Stderr = w

	done := make(chan struct{})

	go func() { _, _ = buf.ReadFrom(r); close(done) }()

	fnErr := fn()

	_ = w.Close()
	os.Stderr = old

	<-done

	return buf.String(), fnErr
}

// fakeZcodeVersion swaps the zcodeInstalledVersion seam: version+resolveErr
// are what the fake "installed zcode" reports.
func fakeZcodeVersion(t *testing.T, version string, resolveErr error) {
	t.Helper()

	old := zcodeInstalledVersion

	zcodeInstalledVersion = func() (string, error) { return version, resolveErr }

	t.Cleanup(func() { zcodeInstalledVersion = old })
}

// fakeParityRun swaps the parityRun seam so runParity is offline-testable (no
// live provider): the A/B arms return a passing single-run result.
func fakeParityRun(t *testing.T) {
	t.Helper()

	old := parityRun

	parityRun = func(_ context.Context, opts *parity.RunOptions) (parity.RunResult, error) {
		return parity.RunResult{
			Summary: parity.Summary{SuiteSize: len(opts.Suite), OverallPass: true},
			Config:  parity.RunConfig{Model: opts.Model, Temp: 0, SuiteSize: len(opts.Suite)},
		}, nil
	}

	t.Cleanup(func() { parityRun = old })
}

// fakeCacheComposition swaps the composeCacheProbeInput seam: the probe sees
// exactly comp regardless of the loaded profile ("via the test's merge
// labeling" — the footer test seeds pass and violation states this way).
func fakeCacheComposition(t *testing.T, comp parity.CacheComposition) {
	t.Helper()

	old := composeCacheProbeInput

	composeCacheProbeInput = func(_ *profile.Profile) parity.CacheComposition { return comp }

	t.Cleanup(func() { composeCacheProbeInput = old })
}

// corpusPinFixturePath points at 14-02's committed cache-control fixture (the
// placement pin source) from the cmd/ass-guard test working directory.
func corpusPinFixturePath() string {
	return filepath.Join("..", "..", "internal", "profile", "testdata", "context-behavior", "cache-control.jsonl")
}

// summaryLines extracts the footer's summary lines (the run's verdict shape)
// for the non-blocking equality assertion.
func summaryLines(out string) []string {
	prefixes := []string{"suite_size:", "layer1_pass_rate:", "layer2_pass_rate:", "overall_status:"}

	var lines []string

	for ln := range strings.SplitSeq(out, "\n") {
		for _, p := range prefixes {
			if strings.HasPrefix(ln, p) {
				lines = append(lines, ln)
			}
		}
	}

	return lines
}

// TestParityDriftWarning_Mismatch (Task 1, Test 1): installed zcode differs
// from the manifest pin -> a LOUD stderr warning naming BOTH versions, while
// the run's returned error and footer summary stay IDENTICAL to a matching
// run (the non-blocking lock is test-pinned, not aspirational).
func TestParityDriftWarning_Mismatch(t *testing.T) { //nolint:paralleltest // swaps process-global seams + os.Stderr
	fakeParityRun(t)

	profilesDir := writeParityProfileFixture(t, pinVersion)
	suite := writeParitySuiteFixture(t)

	fakeZcodeVersion(t, "0.17.0", nil)

	mismatchOut, mismatchErr := captureParityStderr(t, func() error {
		return RunParity(suite, "", profileZcode, profilesDir, "", "", "")
	})

	fakeZcodeVersion(t, pinVersion, nil)

	matchOut, matchErr := captureParityStderr(t, func() error {
		return RunParity(suite, "", profileZcode, profilesDir, "", "", "")
	})
	if mismatchErr != nil || matchErr != nil {
		t.Fatalf("drift warning must never affect the run: mismatch err=%v, match err=%v",
			mismatchErr, matchErr)
	}

	for _, want := range []string{driftWarningMarker, "0.17.0", pinVersion} {
		if !strings.Contains(mismatchOut, want) {
			t.Errorf("mismatch run missing %q in stderr:\n%s", want, mismatchOut)
		}
	}

	if strings.Contains(matchOut, driftWarningMarker) {
		t.Errorf("matching run must not warn:\n%s", matchOut)
	}

	want := strings.Join(summaryLines(matchOut), "|")

	got := strings.Join(summaryLines(mismatchOut), "|")
	if got != want {
		t.Errorf("footer summary changed between match and mismatch runs:\nmatch:    %s\nmismatch: %s", got, want)
	}
}

// TestParityDriftWarning_Match (Task 1, Test 2): matching versions print the
// explicit provenance line and NO drift warning.
func TestParityDriftWarning_Match(t *testing.T) { //nolint:paralleltest // swaps process-global seams + os.Stderr
	fakeParityRun(t)

	profilesDir := writeParityProfileFixture(t, pinVersion)
	suite := writeParitySuiteFixture(t)

	fakeZcodeVersion(t, pinVersion, nil)

	out, err := captureParityStderr(t, func() error {
		return RunParity(suite, "", profileZcode, profilesDir, "", "", "")
	})
	if err != nil {
		t.Fatalf("run errored: %v", err)
	}

	if !strings.Contains(out, "capture provenance: zcode "+pinVersion+" matches installed") {
		t.Errorf("stderr missing the provenance line:\n%s", out)
	}

	if strings.Contains(out, driftWarningMarker) {
		t.Errorf("matching run must not warn:\n%s", out)
	}
}

// TestParityDriftWarning_Unresolvable (Task 1, Test 3): the seam errors
// (binary absent) or the manifest is missing -> an explicit skip note naming
// the pinned version, no warning, run unaffected.
func TestParityDriftWarning_Unresolvable(t *testing.T) { //nolint:paralleltest // swaps process-global seams + os.Stderr
	fakeParityRun(t)

	profilesDir := writeParityProfileFixture(t, pinVersion)
	suite := writeParitySuiteFixture(t)

	fakeZcodeVersion(t, "", os.ErrNotExist)

	out, err := captureParityStderr(t, func() error {
		return RunParity(suite, "", profileZcode, profilesDir, "", "", "")
	})
	if err != nil {
		t.Fatalf("unresolvable version must never fail the run: %v", err)
	}

	if !strings.Contains(out, "skipped") || !strings.Contains(out, pinVersion) {
		t.Errorf("stderr missing the skip note naming the pinned version:\n%s", out)
	}

	if strings.Contains(out, driftWarningMarker) {
		t.Errorf("unresolvable run must not warn:\n%s", out)
	}

	// Missing manifest (same unresolvable path): the pin side cannot be read.
	err = os.Remove(filepath.Join(profilesDir, profileZcode, "coverage.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	out2, err2 := captureParityStderr(t, func() error {
		return RunParity(suite, "", profileZcode, profilesDir, "", "", "")
	})
	if err2 != nil {
		t.Fatalf("missing manifest must never fail the run: %v", err2)
	}

	if !strings.Contains(out2, "skipped") {
		t.Errorf("stderr missing the skip note for a missing manifest:\n%s", out2)
	}

	if strings.Contains(out2, driftWarningMarker) {
		t.Errorf("missing-manifest run must not warn:\n%s", out2)
	}
}

// TestParityRun_CacheProbeWired (Task 3, Test 9): runParity (offline, faked
// A/B arms) executes the cache probe and the footer carries a `cache probe:`
// line with pass/fail; a seeded violation (via the test's merge labeling)
// flips the line to fail WITHOUT changing the A/B summary semantics.
func TestParityRun_CacheProbeWired(t *testing.T) { //nolint:paralleltest // swaps process-global seams + os.Stderr
	fakeParityRun(t)

	profilesDir := writeParityProfileFixture(t, pinVersion)
	suite := writeParitySuiteFixture(t)
	pinPath := corpusPinFixturePath()

	// PASS state: a composition matching the corpus pin (the post-emission-fix
	// shape — captured system blocks carry cache_control, dynamic tails
	// appended after the stable prefix).
	good := parity.CacheComposition{
		System: []parity.ProbeSystemBlock{
			{Block: profile.TextBlock{Type: blockText, Text: "captured-0"}, CacheControl: true},
			{Block: profile.TextBlock{Type: blockText, Text: "skills listing"}, Dynamic: true},
		},
		Tools: []parity.ProbeToolDecl{{Decl: profile.Decl{Name: probeToolRead}}},
	}

	fakeCacheComposition(t, good)

	passOut, passErr := captureParityStderr(t, func() error {
		return RunParity(suite, "", profileZcode, profilesDir, "", "", pinPath)
	})
	if passErr != nil {
		t.Fatalf("pass-state run errored: %v", passErr)
	}

	if !strings.Contains(passOut, "cache probe: PASS") {
		t.Errorf("footer missing the passing cache probe line:\n%s", passOut)
	}

	// FAIL state: a seeded merge-ordering violation (volatile block spliced
	// mid-stable-prefix).
	violation := parity.CacheComposition{
		System: []parity.ProbeSystemBlock{
			{Block: profile.TextBlock{Type: blockText, Text: "captured-0"}, CacheControl: true},
			{Block: profile.TextBlock{Type: blockText, Text: "skills listing"}, Dynamic: true},
			{Block: profile.TextBlock{Type: blockText, Text: "captured-2"}, CacheControl: true},
		},
		Tools: []parity.ProbeToolDecl{{Decl: profile.Decl{Name: probeToolRead}}},
	}

	fakeCacheComposition(t, violation)

	failOut, failErr := captureParityStderr(t, func() error {
		return RunParity(suite, "", profileZcode, profilesDir, "", "", pinPath)
	})
	if failErr != nil {
		t.Fatalf("probe verdict must never fail the run: %v", failErr)
	}

	if !strings.Contains(failOut, "cache probe: FAIL") {
		t.Errorf("footer missing the failing cache probe line:\n%s", failOut)
	}

	if got, want := strings.Join(summaryLines(failOut), "|"), strings.Join(summaryLines(passOut), "|"); got != want {
		t.Errorf("probe verdict changed the A/B summary semantics:\npass: %s\nfail: %s", want, got)
	}
}

// TestParityRun_CacheProbeDefaultGap (Task 3, Test 9's default leg): the
// DEFAULT composition reports today's wiring-time verdict — the shaper emits
// no cache_control while the corpus pin carries it on system blocks; the
// routed emission gap (14-03 CC-1, divergence-routed post-adoption) IS the
// probe line's fact, with the system delta named.
func TestParityRun_CacheProbeDefaultGap(t *testing.T) { //nolint:paralleltest // swaps process-global seams + os.Stderr
	fakeParityRun(t)

	profilesDir := writeParityProfileFixture(t, pinVersion)
	suite := writeParitySuiteFixture(t)
	pinPath := corpusPinFixturePath()

	defaultOut, defaultErr := captureParityStderr(t, func() error {
		return RunParity(suite, "", profileZcode, profilesDir, "", "", pinPath)
	})
	if defaultErr != nil {
		t.Fatalf("default-composition run errored: %v", defaultErr)
	}

	if !strings.Contains(defaultOut, "cache probe: FAIL") ||
		!strings.Contains(defaultOut, "pin-has-composed-lacks [system]") {
		t.Errorf("default run must report the routed emission gap with the system delta named:\n%s", defaultOut)
	}
}
