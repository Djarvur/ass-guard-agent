// Package modesmatrix is the ECOS-04 (TAIL-03, plan 24-05) mode/surface
// matrix: the fixed 4x3 vocabulary of interaction modes (D-11: turn ORIGIN —
// interactive ACP prompt, subagent dispatch, background wake-turn,
// automation/cron turn) times plugin surfaces (D-12: commands, skills,
// hooks), a per-cell result registry, a 4x3 report renderer, and the LOUD
// precondition-skip helper that makes "substrate phase not shipped" a
// grep-able test status instead of a silent pass or a fabricated green
// (T-24-05-03). Steering delivery and parked asks are mechanics INSIDE
// interactive turns, not rows (D-11).
//
// The registry is deliberately per-test-binary: each harness package
// (acpserve/session/runtime) records the cells it exercises and prints the
// full grid with its cells resolved and the rest marked not-exercised. The
// phase's aggregated evidence table lives in 24-05-SUMMARY.md; Validate
// enforces totality so a registry can never silently omit a cell from the
// grid it claims to report.
package modesmatrix

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Mode is one interaction mode (D-11 — turn ORIGIN, exactly four).
type Mode string

// The four modes (D-11). The enum is closed by decision; a fifth mode is a
// planning-level change, not a string to append here.
const (
	ModeInteractive Mode = "interactive"
	ModeSubagent    Mode = "subagent"
	ModeWake        Mode = "wake"
	ModeAutomation  Mode = "automation"
)

// Surface is one plugin surface (D-12 — all three must work per mode).
type Surface string

// The three surfaces (D-12).
const (
	SurfaceCommands Surface = "commands"
	SurfaceSkills   Surface = "skills"
	SurfaceHooks    Surface = "hooks"
)

// Status is one cell's matrix status. The vocabulary is closed: pass/fail
// are exercised outcomes, precondition-unmet is the LOUD phase-contract cite
// (T-24-05-03), skipped-empty is the empty-input row's absence-pass, and
// not-exercised marks cells this test binary did not touch (the report is
// total; a cell is never silently missing).
type Status string

// The cell statuses.
const (
	StatusPass              Status = "pass"
	StatusFail              Status = "fail"
	StatusPreconditionUnmet Status = "precondition-unmet"
	StatusSkippedEmpty      Status = "skipped-empty"
	StatusNotExercised      Status = "not-exercised"
)

// reportReasonMax is the report's reason-column truncation width.
const reportReasonMax = 72

// matrixDirPerm/matrixFilePerm are the mount's permission constants (the
// .ass-guard/.claude house convention).
const (
	matrixDirPerm  = 0o750
	matrixFilePerm = 0o600
)

// Modes and Surfaces are the fixed axis vocabularies (report iteration order).
//
//nolint:gochecknoglobals // immutable axis tables
var (
	Modes    = []Mode{ModeInteractive, ModeSubagent, ModeWake, ModeAutomation}
	Surfaces = []Surface{SurfaceCommands, SurfaceSkills, SurfaceHooks}
)

// Cell is one matrix coordinate.
type Cell struct {
	Mode    Mode
	Surface Surface
}

// AllCells is the total 4x3 grid (twelve cells).
//
//nolint:gochecknoglobals // derived immutable table
var AllCells = buildAllCells()

func buildAllCells() []Cell {
	out := make([]Cell, 0, len(Modes)*len(Surfaces))
	for _, m := range Modes {
		for _, s := range Surfaces {
			out = append(out, Cell{Mode: m, Surface: s})
		}
	}

	return out
}

// String renders the cell coordinate ("interactive x hooks").
func (c Cell) String() string { return string(c.Mode) + " x " + string(c.Surface) }

// Result is one cell's outcome plus the one-line reason the report prints.
type Result struct {
	Cell   Cell
	Status Status
	Reason string
}

// Registry accumulates cell results for one test binary. Safe for
// sequential test use; mutexed so non-parallel siblings stay correct.
type Registry struct {
	mu      sync.Mutex
	results map[Cell]Result
}

// errGridIncomplete is Validate's static sentinel (err113 discipline).
var errGridIncomplete = errors.New("modesmatrix: unresolved cells")

// errFixtureSource is the fixture-source location failure's static sentinel.
var errFixtureSource = errors.New("modesmatrix: cannot locate fixture source path")

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{results: map[Cell]Result{}} }

// Record sets one cell's result (last write wins — a cell re-exercised by a
// later subtest reports its latest observation).
func (r *Registry) Record(mode Mode, surface Surface, status Status, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.results[Cell{Mode: mode, Surface: surface}] = Result{
		Cell: Cell{Mode: mode, Surface: surface}, Status: status, Reason: reason,
	}
}

// RecordFail registers a failing cell (the explicit counterpart of
// RecordPass — a fail must never be phrased as a pass with a sad reason).
func (r *Registry) RecordFail(mode Mode, surface Surface, reason string) {
	r.Record(mode, surface, StatusFail, reason)
}

// Snapshot returns the recorded results keyed by cell (defensive copy).
func (r *Registry) Snapshot() map[Cell]Result {
	r.mu.Lock()
	defer r.mu.Unlock()

	return maps.Clone(r.results)
}

// Missing returns the grid cells with no recorded result — the totality
// lens: the 12-cell grid is total, a report claiming completeness while
// Missing() is non-empty is a fabrication (T-24-05-03).
func (r *Registry) Missing() []Cell {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []Cell
	for _, c := range AllCells {
		if _, ok := r.results[c]; !ok {
			out = append(out, c)
		}
	}

	return out
}

// Validate enforces totality: every one of the twelve cells carries a
// recorded result. The error names every missing cell — a registry that
// exercised fewer cells must not present itself as complete.
func (r *Registry) Validate() error {
	if missing := r.Missing(); len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for _, c := range missing {
			names = append(names, c.String())
		}

		return fmt.Errorf("%w: %d cell(s) unresolved: %v", errGridIncomplete, len(missing), names)
	}

	return nil
}

// Report renders the full 4x3 grid (every cell, recorded or not) plus a
// totals line, and returns Validate's totality error for the caller to act
// on (tests t.Error it; the grid still prints so the state stays readable).
func (r *Registry) Report(w io.Writer) error {
	r.mu.Lock()
	results := maps.Clone(r.results)
	r.mu.Unlock()

	for _, m := range Modes {
		for _, s := range Surfaces {
			c := Cell{Mode: m, Surface: s}
			res, ok := results[c]
			if !ok {
				res = Result{Cell: c, Status: StatusNotExercised, Reason: "not exercised by this test binary"}
			}

			reason := res.Reason
			if len(reason) > reportReasonMax {
				reason = reason[:reportReasonMax] + "…"
			}

			_, _ = fmt.Fprintf(w, "MODES-MATRIX %-13s x %-8s %-20s %s\n",
				string(c.Mode), string(c.Surface), string(res.Status), reason)
		}
	}

	tally := map[Status]int{}
	for _, res := range results {
		tally[res.Status]++
	}

	_, _ = fmt.Fprintf(w,
		"MODES-MATRIX totals: %d/%d cells recorded (pass=%d fail=%d precondition-unmet=%d skipped-empty=%d)\n",
		len(results), len(AllCells),
		tally[StatusPass], tally[StatusFail],
		tally[StatusPreconditionUnmet], tally[StatusSkippedEmpty])

	return r.Validate()
}

// ReportT logs the grid through t (the per-test print at "test end" the
// plan's tasks name). Totality is NOT enforced here: a per-test-binary
// registry is intentionally partial (acpserve exercises the interactive
// row; runtime the automation/wake rows) — AssertComplete is the failing
// gate for a registry that claims completeness.
func (r *Registry) ReportT(t *testing.T) {
	t.Helper()

	var sb strings.Builder

	_ = r.Report(&sb)

	t.Log("\n" + strings.TrimRight(sb.String(), "\n"))
}

// AssertComplete fails t when any of the twelve cells is unresolved — the
// TOTALITY gate (24-05 Task 3): a registry that claims to present the
// ECOS-04 state cannot silently omit a cell; the grid is total and every
// cell carries an explicit status. Per-test-binary reports stay partial by
// design (ReportT); a caller presenting a COMPLETE phase claim binds this.
func (r *Registry) AssertComplete(t *testing.T) {
	t.Helper()

	if err := r.Validate(); err != nil {
		t.Errorf("%v", err)
	}
}

// PreconditionMessage builds the loud-skip message: it BEGINS with
// "PRECONDITION-UNMET(<phase>, <contract-ref>)" so the skip is grep-able
// and can never masquerade as a pass (T-24-05-03; the wake cells' count
// assertion depends on the exact prefix).
func PreconditionMessage(phase, contractRef string, mode Mode, surface Surface, detail string) string {
	return fmt.Sprintf("PRECONDITION-UNMET(%s, %s) %s x %s: %s", phase, contractRef, mode, surface, detail)
}

// SkipPrecondition registers one cell as precondition-unmet — the substrate
// phase named by contractRef has not executed, so the cell carries NO
// functional evidence — and skips the test LOUDLY with PreconditionMessage.
// detail states exactly what functional assertion will replace the skip
// once the phase executes — no fabricated exercise bodies against unbuilt
// APIs (Pitfall 9).
func SkipPrecondition(
	tb testing.TB, r *Registry, mode Mode, surface Surface, phase, contractRef, detail string,
) {
	tb.Helper()

	r.Record(mode, surface, StatusPreconditionUnmet, phase+" "+contractRef+": "+detail)

	tb.Skipf("%s", PreconditionMessage(phase, contractRef, mode, surface, detail))
}

// --- fixture mount (D-14 synthetic half) --------------------------------------
//
// The synthetic fixture lives at internal/ecosys/testdata/modes-matrix/ (a
// plain source tree: plugin.json, commands/, skills/, hooks/) and is COPIED
// into each harness's t.TempDir project as an INSTALLED plugin (the
// .claude/plugins/cache + installed_plugins.json layout the 12-02 loader
// discovers) — never executed in place from testdata (T-24-05-01).

// FixtureDir returns the fixture source tree (repo-relative, located via
// this file's own path — the repo is present whenever tests run).
func FixtureDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errFixtureSource
	}

	dir := filepath.Join(filepath.Dir(thisFile), "..", "ecosys", "testdata", "modes-matrix")

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("modesmatrix: fixture tree missing at %s: %w", dir, err)
	}

	return filepath.Clean(dir), nil
}

// fixturePluginName/marketplace/version identify the mounted fixture in the
// temp project's installed_plugins.json (v1 array shape).
const (
	fixtureInstallRoot  = ".claude/plugins"
	fixtureMarketplace  = "matrix-local"
	fixtureInstallRel   = "cache/matrix-local/modes-matrix/1.0.0"
	fixturePluginKey    = "modes-matrix@" + fixtureMarketplace
	fixtureScopeProject = "project"
	fixtureVersion      = "1.0.0"
)

// MountFixture copies the fixture tree into projectDir's project-scoped
// installed-plugins cache and writes the installed_plugins.json entry that
// makes ecosys.Discover(projectDir) find it: the plugin manifest lands at
// .claude-plugin/plugin.json (the layout resolveInstalledPlugin validates)
// and commands/, skills/, hooks/ ride beside it. The fixture is hermetic by
// construction — no network, no absolute paths, writes land only inside
// projectDir (always a t.TempDir in the harnesses).
func MountFixture(tb testing.TB, projectDir string) {
	tb.Helper()

	src, err := FixtureDir()
	if err != nil {
		tb.Fatalf("modesmatrix: %v", err)
	}

	install := filepath.Join(projectDir, fixtureInstallRoot, fixtureInstallRel)

	// The manifest moves to the .claude-plugin/ name the loader validates;
	// every other file keeps its fixture-relative path.
	manifestSrc := filepath.Join(src, "plugin.json")
	manifestDst := filepath.Join(install, ".claude-plugin", "plugin.json")

	copyFile(tb, manifestSrc, manifestDst)

	for _, sub := range []string{"commands", "skills", "hooks"} {
		copyTree(tb, filepath.Join(src, sub), filepath.Join(install, sub))
	}

	registry := fmt.Sprintf(`[{"name":%q,"installPath":%q,"scope":%q,"version":%q}]`,
		fixturePluginKey, fixtureInstallRel, fixtureScopeProject, fixtureVersion)

	writeFile(tb, filepath.Join(projectDir, fixtureInstallRoot, "installed_plugins.json"), registry)
}

// MountFixtureMarkerName is the fixture hooks' marker file (appended by the
// hook commands in the session's host project dir — the harness's temp dir).
const MountFixtureMarkerName = "matrix-hook.log"

// MarkerPath returns the marker file's path inside the host project dir.
func MarkerPath(projectDir string) string { return filepath.Join(projectDir, MountFixtureMarkerName) }

// MarkerLines reads the marker file's non-empty lines (one JSON object per
// hook firing); a missing file yields nil (the absence lens for the
// empty-input row).
func MarkerLines(projectDir string) ([]string, error) {
	data, err := os.ReadFile(MarkerPath(projectDir))
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("modesmatrix: read marker: %w", err)
	}

	return splitMarkerLines(string(data)), nil
}

func splitMarkerLines(raw string) []string {
	var out []string

	start := -1

	for i := range len(raw) {
		switch raw[i] {
		case '\n':
			if start >= 0 && i > start {
				out = append(out, raw[start:i])
			}

			start = -1
		default:
			if start < 0 {
				start = i
			}
		}
	}

	if start >= 0 && start < len(raw) {
		out = append(out, raw[start:])
	}

	return out
}

// copyFile copies one regular file (creating parent dirs, 0600 — fixture
// content is static text).
func copyFile(tb testing.TB, src, dst string) {
	tb.Helper()

	data, err := os.ReadFile(src)
	if err != nil {
		tb.Fatalf("modesmatrix: read fixture file %s: %v", src, err)
	}

	writeFile(tb, dst, string(data))
}

// copyTree recursively copies a fixture subdirectory.
func copyTree(tb testing.TB, src, dst string) {
	tb.Helper()

	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return // fixture subdirectory not authored yet — nothing to mount
		}

		tb.Fatalf("modesmatrix: read fixture dir %s: %v", src, err)
	}

	if err := os.MkdirAll(dst, matrixDirPerm); err != nil {
		tb.Fatalf("modesmatrix: mkdir %s: %v", dst, err)
	}

	for _, e := range entries {
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())

		switch {
		case e.IsDir():
			copyTree(tb, s, d)
		default:
			copyFile(tb, s, d)
		}
	}
}

// copyTreePreservingMode copies src to dst keeping every file's permission
// bits (the real-plugin mount's +x-fidelity route).
func copyTreePreservingMode(tb testing.TB, src, dst string) {
	tb.Helper()

	entries, err := os.ReadDir(src)
	if err != nil {
		tb.Fatalf("modesmatrix: read real plugin dir %s: %v", src, err)
	}

	if err := os.MkdirAll(dst, matrixDirPerm); err != nil {
		tb.Fatalf("modesmatrix: mkdir %s: %v", dst, err)
	}

	for _, e := range entries {
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())

		switch {
		case e.IsDir():
			copyTreePreservingMode(tb, s, d)
		default:
			info, serr := e.Info()
			if serr != nil {
				tb.Fatalf("modesmatrix: stat %s: %v", s, serr)
			}

			data, rerr := os.ReadFile(s)
			if rerr != nil {
				tb.Fatalf("modesmatrix: read %s: %v", s, rerr)
			}

			if werr := os.MkdirAll(filepath.Dir(d), matrixDirPerm); werr != nil {
				tb.Fatalf("modesmatrix: mkdir %s: %v", filepath.Dir(d), werr)
			}

			//nolint:gosec // mode comes from the operator's own installed plugin tree
			if werr := os.WriteFile(d, data, info.Mode().Perm()); werr != nil {
				tb.Fatalf("modesmatrix: write %s: %v", d, werr)
			}
		}
	}
}

// writeFile writes path's parent dirs into existence then the content at
// 0600 (owner-only — the .ass-guard/.claude house convention).
func writeFile(tb testing.TB, path, content string) {
	tb.Helper()

	if err := os.MkdirAll(filepath.Dir(path), matrixDirPerm); err != nil {
		tb.Fatalf("modesmatrix: mkdir %s: %v", filepath.Dir(path), err)
	}

	//nolint:gosec // test-owned temp paths (the mount discipline: t.TempDir projects only)
	if err := os.WriteFile(path, []byte(content), matrixFilePerm); err != nil {
		tb.Fatalf("modesmatrix: write %s: %v", path, err)
	}
}

// MountRealPlugin mounts ONE REAL installed Claude Code plugin (the D-14
// spot-check's operator-environment half) into projectDir's project-scoped
// installed-plugins cache, READ-ONLY in effect: the source tree is COPIED,
// never written, never executed in place (T-24-05-02 — no test writes under
// any real .claude/ path). The plugin's name comes from its own
// .claude-plugin/plugin.json manifest; the copy layout mirrors MountFixture.
// It returns the plugin's declared name (the evidence record's identity).
func MountRealPlugin(t *testing.T, projectDir, pluginSrcDir string) string {
	t.Helper()

	manifestSrc := filepath.Join(pluginSrcDir, ".claude-plugin", "plugin.json")

	data, err := os.ReadFile(manifestSrc)
	if err != nil {
		t.Fatalf("modesmatrix: real plugin manifest %s: %v", manifestSrc, err)
	}

	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Name == "" {
		t.Fatalf("modesmatrix: real plugin manifest unreadable at %s: %v", manifestSrc, err)
	}

	version := manifest.Version
	if version == "" {
		version = "0.0.0"
	}

	installRel := filepath.ToSlash(filepath.Join("cache", fixtureMarketplace, manifest.Name, version))
	install := filepath.Join(projectDir, fixtureInstallRoot, installRel)

	// Copy the WHOLE plugin tree (read-only discipline: source untouched),
	// PRESERVING each file's mode — real plugins ship directly-executed
	// hook scripts whose +x bit is load-bearing (the spot-check's polyglot
	// wrapper is invoked as a command, not via an interpreter).
	copyTreePreservingMode(t, pluginSrcDir, install)

	// The installed-plugins registry gains the entry beside any previously
	// mounted fixture (append-preserving: read-modify-write the v1 array).
	regPath := filepath.Join(projectDir, fixtureInstallRoot, "installed_plugins.json")

	entries := make([]map[string]string, 0, 1)

	if raw, rerr := os.ReadFile(regPath); rerr == nil {
		_ = json.Unmarshal(raw, &entries) // tolerate and replace on failure
	}

	entries = append(entries, map[string]string{
		"name":        manifest.Name + "@" + fixtureMarketplace,
		"installPath": installRel,
		"scope":       fixtureScopeProject,
		"version":     version,
	})

	encoded, merr := json.Marshal(entries)
	if merr != nil {
		t.Fatalf("modesmatrix: encode installed_plugins.json: %v", merr)
	}

	writeFile(t, regPath, string(encoded))

	return manifest.Name
}
