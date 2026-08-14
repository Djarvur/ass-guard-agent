package openspec_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/openspec"
)

// stubPath returns the absolute path to the testdata stub script. `go test` runs
// with CWD = the package source directory, so a relative path resolves correctly
// under -trimpath (where runtime.Caller returns module-relative paths that are
// not real filesystem paths).
func stubPath(t *testing.T) string {
	t.Helper()

	abs, err := filepath.Abs(filepath.Join("testdata", "openspec-stub.sh"))
	if err != nil {
		t.Fatal(err)
	}

	return abs
}

// putStubOnPATH copies/symlinks the stub script into a temp dir named "openspec"
// and prepends that dir to PATH via t.Setenv, so the Adapter (default binary
// "openspec") runs the stub as a real subprocess. Returns nothing — the env is
// scoped to the test.
func putStubOnPATH(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "openspec")
	src := stubPath(t)
	// Copy (don't symlink) so the named binary "openspec" has the exec bit +
	// the shell shebang works regardless of the source's extension.
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read stub: %v", err)
	}

	err = os.WriteFile(target, data, 0o755)
	if err != nil {
		t.Fatalf("write stub copy: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestAdapter_Exit0StdoutSurfaced verifies exit 0 returns the stdout as the tool
// result with no stderr + no error.
func TestAdapter_Exit0StdoutSurfaced(t *testing.T) { //nolint:paralleltest // putStubOnPATH mutates PATH via t.Setenv
	putStubOnPATH(t)

	a := &openspec.Adapter{}

	stdout, stderr, err := a.Run(context.Background(), "echo", "result line 1", "result line 2")
	if err != nil {
		t.Fatalf("Run err = %v; want nil", err)
	}

	if !strings.Contains(stdout, "result line 1") || !strings.Contains(stdout, "result line 2") {
		t.Errorf("stdout = %q; want both lines surfaced", stdout)
	}

	if stderr != "" {
		t.Errorf("stderr = %q; want empty on exit 0", stderr)
	}
}

// TestAdapter_NonZeroStderrCaptured verifies a non-zero exit captures stderr +
// the err carries the exit code.
func TestAdapter_NonZeroStderrCaptured(t *testing.T) {
	putStubOnPATH(t)
	t.Setenv("ASSGUARD_STUB_STDERR", "the boom")

	a := &openspec.Adapter{}

	stdout, stderr, err := a.Run(context.Background(), "fail")
	if err == nil {
		t.Fatal("Run err = nil; want non-nil for non-zero exit")
	}

	if !strings.Contains(stderr, "the boom") {
		t.Errorf("stderr = %q; want it to carry the boom", stderr)
	}

	if !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("err = %v; want it to mention exit code 1", err)
	}

	_ = stdout
}

// TestAdapter_CtxCancelKillsProcess verifies ctx cancellation kills the child
// within ~200ms (no orphan — T-04-06).
func TestAdapter_CtxCancelKillsProcess(t *testing.T) { //nolint:paralleltest // putStubOnPATH mutates PATH
	putStubOnPATH(t)

	a := &openspec.Adapter{}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, _, err := a.Run(ctx, "sleep")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run err = nil; want ctx error after cancel")
	}

	if elapsed > 500*time.Millisecond {
		t.Errorf("Run took %v after cancel; want < 500ms (no orphan)", elapsed)
	}
	// Confirm no orphaned stub process lingers. Best-effort: pgrep for the
	// stub sleep; if ps is unavailable this check is a no-op.
	_, perr := exec.LookPath("pgrep")
	if perr == nil {
		// Give the OS a moment to reap.
		time.Sleep(100 * time.Millisecond)

		out, _ := exec.CommandContext(context.Background(), "pgrep", "-f", "openspec-stub").Output()
		if strings.TrimSpace(string(out)) != "" {
			t.Errorf("orphaned openspec-stub process detected after cancel: %s", out)
		}
	}
}

// TestAdapter_BinaryNotOnPATH verifies a missing binary yields ErrOpenSpecNotFound
// (typed — the engine routes it to learning ask).
func TestAdapter_BinaryNotOnPATH(t *testing.T) {
	t.Parallel()

	a := &openspec.Adapter{Binary: "definitely-not-a-real-binary-xyz-12345"}

	_, _, err := a.Run(context.Background(), "list")
	if !errors.Is(err, openspec.ErrOpenSpecNotFound) {
		t.Errorf("Run err = %v; want ErrOpenSpecNotFound (errors.Is)", err)
	}
}

// TestAdapter_ArgsForwarded verifies the args are forwarded verbatim.
func TestAdapter_ArgsForwarded(t *testing.T) { //nolint:paralleltest // putStubOnPATH mutates PATH via t.Setenv
	putStubOnPATH(t)

	a := &openspec.Adapter{}
	// The stub's "argv" mode prints the remaining argv after the mode token.
	stdout, _, err := a.Run(context.Background(), "argv", "show", "--format", "json")
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}

	for _, want := range []string{"show", "--format", "json"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q; want it to contain forwarded arg %q", stdout, want)
		}
	}
}

// TestAdapter_RealOpenspecGated verifies the real openspec binary runs when
// ASSGUARD_OPENSPEC_BIN=1 (skipped otherwise — CI never depends on the binary).
func TestAdapter_RealOpenspecGated(t *testing.T) {
	t.Parallel()

	if os.Getenv("ASSGUARD_OPENSPEC_BIN") != "1" {
		t.Skip("set ASSGUARD_OPENSPEC_BIN=1 to run against the real openspec binary")
	}

	_, err := exec.LookPath("openspec")
	if err != nil {
		t.Skip("openspec binary not on PATH even though ASSGUARD_OPENSPEC_BIN=1")
	}

	a := &openspec.Adapter{}

	stdout, _, err := a.Run(context.Background(), "--version")
	if err != nil {
		t.Fatalf("real openspec --version: %v", err)
	}

	if strings.TrimSpace(stdout) == "" {
		t.Errorf("real openspec --version stdout empty")
	}
}

// installedSubcommands runs `openspec --help` and returns the top-level
// subcommand names (lines indented exactly two spaces in the Commands section).
func installedSubcommands(t *testing.T) map[string]bool {
	t.Helper()

	a := &openspec.Adapter{}

	help, _, err := a.Run(context.Background(), "--help")
	if err != nil {
		t.Fatalf("real openspec --help: %v", err)
	}

	cmds := map[string]bool{}

	cmdLine := regexp.MustCompile(`^ {2}([a-zA-Z-]+)\b`)

	for line := range strings.SplitSeq(help, "\n") {
		if m := cmdLine.FindStringSubmatch(line); m != nil {
			cmds[m[1]] = true
		}
	}

	if len(cmds) == 0 {
		t.Fatal("parsed zero subcommands from --help — parser drift?")
	}

	return cmds
}

// TestSurfaceMatchesInstalledBinary is the surface-drift gate (Pitfall 7.2):
// every seeded [commands] entry must exist on the INSTALLED binary's probe,
// and the phantom apply/implement must never come back. Gated like
// TestAdapter_RealOpenspecGated — this is the re-probe procedure for upgrades.
func TestSurfaceMatchesInstalledBinary(t *testing.T) {
	t.Parallel()

	if os.Getenv("ASSGUARD_OPENSPEC_BIN") != "1" {
		t.Skip("set ASSGUARD_OPENSPEC_BIN=1 to run against the real openspec binary")
	}

	if _, err := exec.LookPath("openspec"); err != nil { //nolint:noinlineerr // skip-path
		t.Skip("openspec binary not on PATH even though ASSGUARD_OPENSPEC_BIN=1")
	}

	cfg, err := openspec.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	if _, exists := cfg.Commands["apply"]; exists {
		t.Error("phantom [commands.apply] present — deleted in Phase 8, must stay out")
	}

	if _, exists := cfg.Commands["implement"]; exists {
		t.Error("phantom [commands.implement] present — deleted in Phase 8, must stay out")
	}

	installed := installedSubcommands(t)

	var stale []string

	for name, shape := range cfg.Commands {
		argv0 := strings.Fields(shape.Argv)
		if len(argv0) == 0 {
			stale = append(stale, name+" (empty argv)")

			continue
		}

		if !installed[argv0[0]] {
			stale = append(stale, fmt.Sprintf("%s (argv %q: %q not on installed binary)",
				name, shape.Argv, argv0[0]))
		}
	}

	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("seeded surface drifted from installed binary — stale entries:\n  %s\n"+
			"Re-probe the binary and update seeded.toml (see its header).",
			strings.Join(stale, "\n  "))
	}
}
