package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/parity"
)

// pinVersion is the coverage manifest's pinned zcode_version in the parity
// fixtures; the drift-warning tests fake the INSTALLED side around it.
const pinVersion = "0.16.3"

// driftWarningMarker is the warning line's stable substring every drift test
// keys on (Task 1's "a warning line matching `zcode version drift`").
const driftWarningMarker = "zcode version drift"

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

	files := map[string]string{
		"profile.yaml":       "name: " + profileZcode + "\nmodel: ph-model\nmax_tokens: 128\n",
		"system/block-0.txt": "ph-sys-0",
		"tools.json":         "[]",
		"identity.yaml":      "headers: []\n",
		"thinking.json":      "{}",
		"tool_choice.json":   "{}",
		"coverage.yaml": "profile: " + profileZcode + "\ntarget_capture_ref:\n  zcode_version: \"" +
			zcodeVersion + "\"\nfields: []\n",
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

// summaryLines extracts the footer's summary lines (the run's verdict shape)
// for the non-blocking equality assertion.
func summaryLines(out string) []string {
	prefixes := []string{"suite_size:", "layer1_pass_rate:", "layer2_pass_rate:", "overall_status:"}

	var lines []string

	for _, ln := range strings.Split(out, "\n") {
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
		return runParity(suite, "", profileZcode, profilesDir, "", "")
	})

	fakeZcodeVersion(t, pinVersion, nil)

	matchOut, matchErr := captureParityStderr(t, func() error {
		return runParity(suite, "", profileZcode, profilesDir, "", "")
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

	if got, want := strings.Join(summaryLines(matchOut), "|"), strings.Join(summaryLines(mismatchOut), "|"); got != want {
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
		return runParity(suite, "", profileZcode, profilesDir, "", "")
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

	fakeZcodeVersion(t, "", errors.New("binary absent"))

	out, err := captureParityStderr(t, func() error {
		return runParity(suite, "", profileZcode, profilesDir, "", "")
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
		return runParity(suite, "", profileZcode, profilesDir, "", "")
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
