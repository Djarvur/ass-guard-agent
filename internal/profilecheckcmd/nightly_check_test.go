package profilecheckcmd //nolint:testpackage // internal package test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The nightly upstream-parity check battery (24-04 TAIL-02, D-09 light path).
// Fully OFFLINE by construction: the version probe is the injected
// VersionFunc seam (never a live zcode exec) and the profile bundle is a
// t.TempDir seven-file fixture — no network, no zcode binary, race-safe.

const (
	testZcodeVersion = "0.16.3"
	pinFileName      = "structure-pin.json"
	reportFileName   = "nightly-report.json"
)

// nightlyBundleFileNames is the pinned bundle's fixed file set as the tests
// declare it (independent of the implementation's own list — a set mismatch
// must fail here, not silently agree).
var nightlyBundleFileNames = []string{ //nolint:gochecknoglobals // fixed fixture set
	"profile.yaml",
	"tools.json",
	"thinking.json",
	"tool_choice.json",
	"coverage.yaml",
	"identity.yaml",
	"meta.yaml",
}

// nightlyBundleContents is the deterministic per-file fixture content.
var nightlyBundleContents = map[string]string{ //nolint:gochecknoglobals // fixed fixture set
	"profile.yaml":     "profile: zcode\n",
	"tools.json":       `{"tools": [{"name": "Bash"}]}`,
	"thinking.json":    "",
	"tool_choice.json": `{"type": "auto"}`,
	"coverage.yaml":    "profile: zcode\nfields: []\n",
	"identity.yaml":    "identity: zcode\n",
	"meta.yaml":        "zcode_version: " + testZcodeVersion + "\n",
}

// writeNightlyBundle writes the seven pinned bundle files into a temp dir
// shaped like profiles/zcode and returns the dir path.
func writeNightlyBundle(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	for _, name := range nightlyBundleFileNames {
		err := os.WriteFile(filepath.Join(dir, name), []byte(nightlyBundleContents[name]), 0o600)
		if err != nil {
			t.Fatalf("write bundle file %s: %v", name, err)
		}
	}

	return dir
}

// nightlyTestPaths returns (bundleDir, pinPath, reportPath) with the bundle
// written into its own temp dir and pin/report paths in a fresh output dir.
func nightlyTestPaths(t *testing.T) (string, string, string) {
	t.Helper()

	out := t.TempDir()

	return writeNightlyBundle(t), filepath.Join(out, pinFileName), filepath.Join(out, reportFileName)
}

// fixedProbe returns a VersionFunc reporting the given installed version.
func fixedProbe(version string) func() (string, error) {
	return func() (string, error) { return version, nil }
}

// readNightlyReport loads and decodes the report JSON written to path.
func readNightlyReport(t *testing.T, path string) NightlyReport {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report %s: %v", path, err)
	}

	var rep NightlyReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("decode report %s: %v", path, err)
	}

	return rep
}

// readPin loads and decodes the structure pin at path.
func readPin(t *testing.T, path string) structurePin {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pin %s: %v", path, err)
	}

	var pin structurePin
	if err := json.Unmarshal(raw, &pin); err != nil {
		t.Fatalf("decode pin %s: %v", path, err)
	}

	return pin
}

// requireParityFiles asserts every report row matches (the bundle-content
// side of parity).
func requireParityFiles(t *testing.T, rep NightlyReport) {
	t.Helper()

	if len(rep.Files) != len(nightlyBundleFileNames) {
		t.Fatalf("report lists %d files, want %d", len(rep.Files), len(nightlyBundleFileNames))
	}

	for _, f := range rep.Files {
		if !f.Match {
			t.Errorf("file %s reported mismatched under content parity", f.Name)
		}
	}
}

// TestNightlyCheckWritePin: --write-pin freezes the current bundle — the pin
// carries a digest for each of the seven files, the version source is the
// bundle's own meta.yaml record, and the command exits clean.
func TestNightlyCheckWritePin(t *testing.T) {
	t.Parallel()

	dir, pin, _ := nightlyTestPaths(t)

	err := RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir,
		PinPath:     pin,
		WritePin:    true,
		VersionFunc: fixedProbe(testZcodeVersion),
	})
	if err != nil {
		t.Fatalf("write-pin: %v", err)
	}

	pinDoc := readPin(t, pin)
	if pinDoc.ZcodeVersion != testZcodeVersion {
		t.Errorf("pin zcode_version = %q, want %q (meta.yaml is the pin's version source)",
			pinDoc.ZcodeVersion, testZcodeVersion)
	}

	if len(pinDoc.Files) != len(nightlyBundleFileNames) {
		t.Errorf("pin lists %d files, want exactly the seven pinned files", len(pinDoc.Files))
	}

	if pinDoc.GeneratedAt == "" {
		t.Error("pin generated_at is empty")
	}

	for _, name := range nightlyBundleFileNames {
		if pinDoc.Files[name] == "" {
			t.Errorf("pin has no digest for %s", name)
		}
	}
}

// TestNightlyCheckParityRoundTrip: write-pin then check against the unchanged
// bundle → nil error, drift=false, every row matches, report JSON written.
func TestNightlyCheckParityRoundTrip(t *testing.T) {
	t.Parallel()

	dir, pin, report := nightlyTestPaths(t)

	err := RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, WritePin: true, VersionFunc: fixedProbe(testZcodeVersion),
	})
	if err != nil {
		t.Fatalf("write-pin: %v", err)
	}

	err = RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, ReportPath: report, VersionFunc: fixedProbe(testZcodeVersion),
	})
	if err != nil {
		t.Fatalf("check after write-pin: %v", err)
	}

	rep := readNightlyReport(t, report)
	if rep.Drift {
		t.Errorf("parity run reports drift: %s", rep.Summary)
	}

	if !rep.ZcodeVersion.Match {
		t.Errorf("version match = false: %+v", rep.ZcodeVersion)
	}

	if rep.ZcodeVersion.Expected != testZcodeVersion || rep.ZcodeVersion.Found != testZcodeVersion {
		t.Errorf("version = expected %q found %q, want %q both sides",
			rep.ZcodeVersion.Expected, rep.ZcodeVersion.Found, testZcodeVersion)
	}

	requireParityFiles(t, rep)

	if rep.GeneratedAt == "" {
		t.Error("report generated_at is empty")
	}
}

// TestNightlyCheckVersionDrift: the probe reports a version the pin does not
// carry → typed drift error, report drift=true with both versions named, and
// the bundle files themselves still match (version-only drift).
func TestNightlyCheckVersionDrift(t *testing.T) {
	t.Parallel()

	dir, pin, report := nightlyTestPaths(t)

	err := RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, WritePin: true, VersionFunc: fixedProbe(testZcodeVersion),
	})
	if err != nil {
		t.Fatalf("write-pin: %v", err)
	}

	const movedVersion = "0.17.0"

	err = RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, ReportPath: report, VersionFunc: fixedProbe(movedVersion),
	})
	if !errors.Is(err, ErrNightlyDrift) {
		t.Fatalf("want ErrNightlyDrift, got %v", err)
	}

	rep := readNightlyReport(t, report)
	if !rep.Drift {
		t.Errorf("version drift not flagged: %s", rep.Summary)
	}

	if rep.ZcodeVersion.Match {
		t.Error("version row matches despite drift")
	}

	if rep.ZcodeVersion.Expected != testZcodeVersion || rep.ZcodeVersion.Found != movedVersion {
		t.Errorf("version = expected %q found %q, want %q/%q",
			rep.ZcodeVersion.Expected, rep.ZcodeVersion.Found, testZcodeVersion, movedVersion)
	}

	requireParityFiles(t, rep)
}

// TestNightlyCheckContentDrift: one bundle file's bytes change after pinning
// → typed drift error, that file's row mismatches while every other row and
// the version stay green.
func TestNightlyCheckContentDrift(t *testing.T) {
	t.Parallel()

	dir, pin, report := nightlyTestPaths(t)

	err := RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, WritePin: true, VersionFunc: fixedProbe(testZcodeVersion),
	})
	if err != nil {
		t.Fatalf("write-pin: %v", err)
	}

	const drifted = `{"tools": [{"name": "WebFetch"}]}`

	err = os.WriteFile(filepath.Join(dir, "tools.json"), []byte(drifted), 0o600)
	if err != nil {
		t.Fatalf("mutate tools.json: %v", err)
	}

	err = RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, ReportPath: report, VersionFunc: fixedProbe(testZcodeVersion),
	})
	if !errors.Is(err, ErrNightlyDrift) {
		t.Fatalf("want ErrNightlyDrift, got %v", err)
	}

	rep := readNightlyReport(t, report)
	if !rep.Drift {
		t.Errorf("content drift not flagged: %s", rep.Summary)
	}

	if !rep.ZcodeVersion.Match {
		t.Errorf("version row drifted on a content-only change: %+v", rep.ZcodeVersion)
	}

	toolsMismatched := false

	for _, f := range rep.Files {
		wantMismatch := f.Name == "tools.json"
		if f.Match == wantMismatch {
			t.Errorf("file %s match=%v, want match=%v", f.Name, f.Match, !wantMismatch)
		}

		if wantMismatch {
			toolsMismatched = true
			if f.Expected == f.Actual {
				t.Error("mismatched tools.json row carries identical digests")
			}
		}
	}

	if !toolsMismatched {
		t.Error("report has no tools.json row")
	}
}

// TestNightlyCheckProbeFailure: the version probe errors → DRIFT with the
// probe-failed reason (an unprovable environment must never read as parity),
// the report is still written (artifact-always), and the bundle rows match.
func TestNightlyCheckProbeFailure(t *testing.T) {
	t.Parallel()

	dir, pin, report := nightlyTestPaths(t)

	err := RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, WritePin: true, VersionFunc: fixedProbe(testZcodeVersion),
	})
	if err != nil {
		t.Fatalf("write-pin: %v", err)
	}

	err = RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir,
		PinPath:     pin,
		ReportPath:  report,
		VersionFunc: func() (string, error) { return "", errors.New("exec zcode --version: not found") },
	})
	if !errors.Is(err, ErrNightlyDrift) {
		t.Fatalf("probe failure must read as drift, got %v", err)
	}

	rep := readNightlyReport(t, report)
	if !rep.Drift {
		t.Errorf("probe failure not flagged as drift: %s", rep.Summary)
	}

	if rep.ZcodeVersion.Reason != "probe-failed" {
		t.Errorf("version reason = %q, want probe-failed", rep.ZcodeVersion.Reason)
	}

	if rep.ZcodeVersion.Match {
		t.Error("version row matches despite the failed probe")
	}

	requireParityFiles(t, rep)
}

// TestNightlyCheckMissingPin: a check with no pin file on disk is an
// OPERATIONAL error (exit 1 class) — never drift, never parity.
func TestNightlyCheckMissingPin(t *testing.T) {
	t.Parallel()

	dir, _, report := nightlyTestPaths(t)
	pin := filepath.Join(t.TempDir(), "absent-pin.json")

	err := RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, ReportPath: report, VersionFunc: fixedProbe(testZcodeVersion),
	})
	if err == nil {
		t.Fatal("missing pin must be an operational error, got nil")
	}

	if errors.Is(err, ErrNightlyDrift) {
		t.Fatalf("missing pin is operational (exit 1), not drift: %v", err)
	}
}

// TestNightlyCheckMissingBundleFile: a pinned bundle file missing from disk is
// an OPERATIONAL error (exit 1 class) — the bundle is incomplete, which is not
// a comparable state.
func TestNightlyCheckMissingBundleFile(t *testing.T) {
	t.Parallel()

	dir, pin, report := nightlyTestPaths(t)

	err := RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, WritePin: true, VersionFunc: fixedProbe(testZcodeVersion),
	})
	if err != nil {
		t.Fatalf("write-pin: %v", err)
	}

	err = os.Remove(filepath.Join(dir, "meta.yaml"))
	if err != nil {
		t.Fatalf("remove meta.yaml: %v", err)
	}

	err = RunNightlyCheck(NightlyCheckOptions{
		ProfilesDir: dir, PinPath: pin, ReportPath: report, VersionFunc: fixedProbe(testZcodeVersion),
	})
	if err == nil {
		t.Fatal("missing bundle file must be an operational error, got nil")
	}

	if errors.Is(err, ErrNightlyDrift) {
		t.Fatalf("missing bundle file is operational (exit 1), not drift: %v", err)
	}
}
