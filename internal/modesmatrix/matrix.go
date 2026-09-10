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

		return fmt.Errorf("modesmatrix: %d cell(s) unresolved: %v", len(missing), names)
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
			if len(reason) > 72 {
				reason = reason[:72] + "…"
			}

			fmt.Fprintf(w, "MODES-MATRIX %-13s x %-8s %-20s %s\n",
				string(c.Mode), string(c.Surface), string(res.Status), reason)
		}
	}

	tally := map[Status]int{}
	for _, res := range results {
		tally[res.Status]++
	}

	fmt.Fprintf(w, "MODES-MATRIX totals: %d/%d cells recorded (pass=%d fail=%d precondition-unmet=%d skipped-empty=%d)\n",
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

// SkipPrecondition registers one cell as precondition-unmet — the substrate
// phase named by contractRef has not executed, so the cell carries NO
// functional evidence — and skips the test LOUDLY with a message that
// begins with "PRECONDITION-UNMET(<phase>, <contract-ref>)" so the skip is
// grep-able and can never masquerade as a pass (T-24-05-03; the count
// assertion in the wake cells proves loudness). detail states exactly what
// functional assertion will replace the skip once the phase executes — no
// fabricated exercise bodies against unbuilt APIs (Pitfall 9).
func SkipPrecondition(
	t testing.TB, r *Registry, mode Mode, surface Surface, phase, contractRef, detail string,
) {
	t.Helper()

	r.Record(mode, surface, StatusPreconditionUnmet, phase+" "+contractRef+": "+detail)

	t.Skipf("PRECONDITION-UNMET(%s, %s) %s x %s: %s", phase, contractRef, mode, surface, detail)
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
		return "", fmt.Errorf("modesmatrix: cannot locate fixture source path")
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
func MountFixture(t testing.TB, projectDir string) {
	t.Helper()

	src, err := FixtureDir()
	if err != nil {
		t.Fatalf("modesmatrix: %v", err)
	}

	install := filepath.Join(projectDir, fixtureInstallRoot, fixtureInstallRel)

	// The manifest moves to the .claude-plugin/ name the loader validates;
	// every other file keeps its fixture-relative path.
	manifestSrc := filepath.Join(src, "plugin.json")
	manifestDst := filepath.Join(install, ".claude-plugin", "plugin.json")

	copyFile(t, manifestSrc, manifestDst)

	for _, sub := range []string{"commands", "skills", "hooks"} {
		copyTree(t, filepath.Join(src, sub), filepath.Join(install, sub))
	}

	registry := fmt.Sprintf(`[{"name":%q,"installPath":%q,"scope":%q,"version":%q}]`,
		fixturePluginKey, fixtureInstallRel, fixtureScopeProject, fixtureVersion)

	writeFile(t, filepath.Join(projectDir, fixtureInstallRoot, "installed_plugins.json"), registry)
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

	for i := 0; i < len(raw); i++ {
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
func copyFile(t testing.TB, src, dst string) {
	t.Helper()

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("modesmatrix: read fixture file %s: %v", src, err)
	}

	writeFile(t, dst, string(data))
}

// copyTree recursively copies a fixture subdirectory.
func copyTree(t testing.TB, src, dst string) {
	t.Helper()

	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return // fixture subdirectory not authored yet — nothing to mount
		}

		t.Fatalf("modesmatrix: read fixture dir %s: %v", src, err)
	}

	if err := os.MkdirAll(dst, 0o750); err != nil {
		t.Fatalf("modesmatrix: mkdir %s: %v", dst, err)
	}

	for _, e := range entries {
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())

		switch {
		case e.IsDir():
			copyTree(t, s, d)
		default:
			copyFile(t, s, d)
		}
	}
}

// writeFile writes path's parent dirs into existence then the content at
// 0600 (owner-only — the .ass-guard/.claude house convention).
func writeFile(t testing.TB, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("modesmatrix: mkdir %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("modesmatrix: write %s: %v", path, err)
	}
}
