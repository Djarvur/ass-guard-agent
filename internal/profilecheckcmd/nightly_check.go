package profilecheckcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Djarvur/ass-guard-agent/internal/paritycli"
)

// ErrNightlyDrift is the typed drift verdict: the pinned bundle or the zcode
// version moved (or the probe failed — an unprovable environment never reads
// as parity). The cobra shell maps it to exit code 7 so CI can distinguish
// drift from operational failure (exit 1).
var ErrNightlyDrift = errors.New("nightly parity drift")

// errNightlyMissingPaths: both path flags are required for any mode.
var errNightlyMissingPaths = errors.New("nightly-check: --profiles-dir and --pin are required")

// errPinIncomplete: the pin lacks a digest for a file in the fixed set.
var errPinIncomplete = errors.New("structure pin incomplete")

// errMetaNoVersion: the bundle's meta.yaml carries no zcode_version.
var errMetaNoVersion = errors.New("meta.yaml records no zcode_version — the pin has no version source")

// filePermDefault follows the extract-profile writer convention (0o600;
// gosec G306 + mnd both satisfied by the named constant).
const filePermDefault = 0o600

// nightlyPinnedFiles is the pinned bundle's FIXED file set (24-04 A4: the
// exact set is chosen here and pinned). Subdirectories (system/,
// drift-reports/) are deliberately NOT part of the structure pin — the seven
// files are the profile's wire-facing surface; regenerating the pin after a
// set change here requires a fresh --write-pin by design.
var nightlyPinnedFiles = []string{ //nolint:gochecknoglobals // fixed by design (A4)
	"profile.yaml",
	"tools.json",
	"thinking.json",
	"tool_choice.json",
	"coverage.yaml",
	"identity.yaml",
	"meta.yaml",
}

// NightlyCheckOptions configures the nightly upstream-parity check (TAIL-02,
// D-09 light path: zcode version probe + bundle structure hash vs the pinned
// capture — zero API spend, OQ3). VersionFunc defaults to the paritycli exec
// seam; tests inject a fake probe.
type NightlyCheckOptions struct {
	ProfilesDir string
	PinPath     string
	ReportPath  string
	WritePin    bool
	VersionFunc func() (string, error)
}

// structurePin is the committed structure-pin.json shape: the pinned
// capture's version source and one sha256 digest per pinned bundle file.
type structurePin struct {
	GeneratedAt  string            `json:"generated_at"`
	ZcodeVersion string            `json:"zcode_version"`
	Files        map[string]string `json:"files"`
}

// nightlyVersionState is the report's version row: pin expectation vs probe.
type nightlyVersionState struct {
	Expected string `json:"expected"`
	Found    string `json:"found"`
	Match    bool   `json:"match"`
	Reason   string `json:"reason,omitempty"`
}

// nightlyFileState is the report's per-file row: pin digest vs bundle digest.
type nightlyFileState struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Match    bool   `json:"match"`
}

// NightlyReport is the drift report JSON written on every check run (D-10:
// the artifact is the belt — it exists whether or not drift was found). File
// names, sha256 digests, and version strings only — never capture bytes
// (T-24-04-04).
type NightlyReport struct {
	GeneratedAt  string              `json:"generated_at"`
	ZcodeVersion nightlyVersionState `json:"zcode_version"`
	Files        []nightlyFileState  `json:"files"`
	Drift        bool                `json:"drift"`
	Summary      string              `json:"summary"`
}

// profileMeta carries the single meta.yaml field the pin sources its version
// from — the bundle's own recorded capture version, so pinning is
// deterministic (the same bundle pins to the same pin on any machine).
type profileMeta struct {
	ZcodeVersion string `yaml:"zcode_version"`
}

// RunNightlyCheck runs the nightly upstream-parity drift core (TAIL-02 light
// path). --write-pin regenerates the pin from the current bundle and exits
// clean; check mode probes the installed zcode version, hashes the pinned
// bundle files, compares against the pin, always writes the report JSON to
// ReportPath, prints a one-line human summary to stderr, and returns
// ErrNightlyDrift on drift (exit 7 via the shell) or an operational error
// (exit 1: missing pin, missing bundle file). A failed probe is DRIFT with
// the probe-failed reason — an unprovable environment must never read as
// parity.
func RunNightlyCheck(opts NightlyCheckOptions) error {
	if opts.VersionFunc == nil {
		opts.VersionFunc = paritycli.ZcodeInstalledVersion
	}

	if opts.ProfilesDir == "" || opts.PinPath == "" {
		return errNightlyMissingPaths
	}

	if opts.WritePin {
		return writeNightlyPin(opts)
	}

	return runNightlyComparison(opts)
}

// writeNightlyPin freezes the current bundle as the comparison baseline. The
// version source is the bundle's own meta.yaml record (the pinned capture's
// provenance); the installed zcode is probed only for an operator note —
// never blocking, never recorded. Pin updates go exclusively through the
// recapture runbook.
func writeNightlyPin(opts NightlyCheckOptions) error {
	digests, err := hashPinnedBundle(opts.ProfilesDir)
	if err != nil {
		return err
	}

	version, err := pinnedVersionFromMeta(opts.ProfilesDir)
	if err != nil {
		return err
	}

	pin := structurePin{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		ZcodeVersion: version,
		Files:        digests,
	}

	raw, err := json.MarshalIndent(pin, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal structure pin: %w", err)
	}

	if err := os.WriteFile(opts.PinPath, append(raw, '\n'), filePermDefault); err != nil {
		return fmt.Errorf("write structure pin %q: %w", opts.PinPath, err)
	}

	fmt.Fprintf(os.Stderr, "nightly-check: structure pin written — zcode %s, %d files (%s)\n",
		version, len(digests), opts.PinPath)

	if installed, ierr := opts.VersionFunc(); ierr != nil {
		fmt.Fprintf(os.Stderr,
			"nightly-check: installed zcode unresolvable (%v) — pin records the bundle's meta.yaml version\n", ierr)
	} else if installed != version {
		fmt.Fprintf(os.Stderr,
			"nightly-check: installed zcode %s differs from pinned %s — pin updates go through the recapture runbook\n",
			installed, version)
	}

	return nil
}

// runNightlyComparison executes the check leg: pin vs bundle digests + pin
// version vs probe, report always, one-line stderr summary, typed drift
// verdict.
func runNightlyComparison(opts NightlyCheckOptions) error {
	pin, err := loadStructurePin(opts.PinPath)
	if err != nil {
		return err
	}

	digests, err := hashPinnedBundle(opts.ProfilesDir)
	if err != nil {
		return err
	}

	found, probeErr := opts.VersionFunc()

	version := nightlyVersionState{Expected: pin.ZcodeVersion, Found: found}
	if probeErr != nil {
		version.Reason = "probe-failed"
	} else if found == pin.ZcodeVersion {
		version.Match = true
	}

	files, fileDrift := comparePinnedFiles(pin, digests)

	rep := NightlyReport{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		ZcodeVersion: version,
		Files:        files,
		Drift:        fileDrift || !version.Match,
	}

	rep.Summary = summarizeNightly(&rep, fileDrift)

	if werr := writeNightlyReport(opts.ReportPath, &rep); werr != nil {
		return werr
	}

	fmt.Fprintln(os.Stderr, "nightly-check:", rep.Summary)

	if rep.Drift {
		return fmt.Errorf("%w: %s", ErrNightlyDrift, rep.Summary)
	}

	return nil
}

// comparePinnedFiles builds the per-file rows against the FIXED file set.
// loadStructurePin has already validated that the pin carries a digest for
// every pinned file (a pin/file-set disagreement is an operational error
// there, resolvable only by a fresh --write-pin).
func comparePinnedFiles(pin structurePin, digests map[string]string) ([]nightlyFileState, bool) {
	files := make([]nightlyFileState, 0, len(nightlyPinnedFiles))
	drift := false

	for _, name := range nightlyPinnedFiles {
		expected := pin.Files[name]
		actual := digests[name]
		match := expected == actual
		drift = drift || !match

		files = append(files, nightlyFileState{Name: name, Expected: expected, Actual: actual, Match: match})
	}

	return files, drift
}

// summarizeNightly renders the one-line human summary (stderr + report).
func summarizeNightly(rep *NightlyReport, fileDrift bool) string {
	matched := 0

	for _, f := range rep.Files {
		if f.Match {
			matched++
		}
	}

	total := len(rep.Files)

	switch {
	case rep.ZcodeVersion.Reason == "probe-failed":
		return fmt.Sprintf("DRIFT (probe-failed): zcode version probe failed — pinned %s; %d/%d bundle files match",
			rep.ZcodeVersion.Expected, matched, total)
	case !rep.ZcodeVersion.Match:
		return fmt.Sprintf("DRIFT: zcode version moved — pinned %s, found %s; %d/%d bundle files match",
			rep.ZcodeVersion.Expected, rep.ZcodeVersion.Found, matched, total)
	case fileDrift:
		return fmt.Sprintf("DRIFT: profile bundle content drift — %d/%d files match the pinned capture", matched, total)
	default:
		return fmt.Sprintf("parity: zcode %s, %d/%d bundle files match the pinned capture",
			rep.ZcodeVersion.Expected, matched, total)
	}
}

// writeNightlyReport persists the report JSON (D-10: on EVERY check run,
// drift or not). An empty path skips the file — the stderr summary remains.
func writeNightlyReport(path string, rep *NightlyReport) error {
	if path == "" {
		return nil
	}

	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal nightly report: %w", err)
	}

	if err := os.WriteFile(path, append(raw, '\n'), filePermDefault); err != nil {
		return fmt.Errorf("write nightly report %q: %w", path, err)
	}

	return nil
}

// loadStructurePin reads and validates the committed pin.
func loadStructurePin(path string) (structurePin, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return structurePin{}, fmt.Errorf("read structure pin %q: %w", path, err)
	}

	var pin structurePin
	if err := json.Unmarshal(raw, &pin); err != nil {
		return structurePin{}, fmt.Errorf("decode structure pin %q: %w", path, err)
	}

	for _, name := range nightlyPinnedFiles {
		if pin.Files[name] == "" {
			return structurePin{}, fmt.Errorf(
				"%w: %q has no digest for pinned file %q — regenerate with --write-pin",
				errPinIncomplete, path, name)
		}
	}

	return pin, nil
}

// hashPinnedBundle computes the sha256 digest of each pinned bundle file. A
// missing file is an operational error: the bundle is incomplete, which is
// not a comparable state.
func hashPinnedBundle(profilesDir string) (map[string]string, error) {
	digests := make(map[string]string, len(nightlyPinnedFiles))

	for _, name := range nightlyPinnedFiles {
		raw, err := os.ReadFile(filepath.Join(profilesDir, name))
		if err != nil {
			return nil, fmt.Errorf("read pinned bundle file %q: %w", name, err)
		}

		sum := sha256.Sum256(raw)
		digests[name] = hex.EncodeToString(sum[:])
	}

	return digests, nil
}

// pinnedVersionFromMeta resolves the pin's version source: the bundle's own
// meta.yaml zcode_version record (deterministic pinning — the same bundle
// pins identically on any machine, probe or no probe).
func pinnedVersionFromMeta(profilesDir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(profilesDir, "meta.yaml"))
	if err != nil {
		return "", fmt.Errorf("read meta.yaml: %w", err)
	}

	var meta profileMeta
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return "", fmt.Errorf("parse meta.yaml: %w", err)
	}

	if meta.ZcodeVersion == "" {
		return "", errMetaNoVersion
	}

	return meta.ZcodeVersion, nil
}
