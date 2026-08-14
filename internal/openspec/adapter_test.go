package openspec_test

import (
	"context"
	"encoding/json"
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
	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
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
// "openspec") runs the stub as a real subprocess. Returns the stub's absolute
// path (for orphan process-list checks); the env is scoped to the test.
func putStubOnPATH(t *testing.T) string {
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

	return target
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

// TestRunGuarded_TimeoutKillsHanger (Test 6) verifies a per-command timeout
// kills a hanging subprocess — the WHOLE process group, not just the direct
// child: the stub sh spawns `sleep 10`, and a lone SIGKILL of the shell leaves
// the grandchild holding the output pipes (Wait stalls for the child's full
// runtime). Budget 1s; RunGuarded must return within ~2s classified "timeout"
// with no orphaned stub process left.
func TestRunGuarded_TimeoutKillsHanger(t *testing.T) { //nolint:paralleltest // putStubOnPATH mutates PATH
	stub := putStubOnPATH(t)
	t.Setenv("ASSGUARD_STUB_SLEEP", "10")

	a := &openspec.Adapter{}

	start := time.Now()

	res := a.RunGuarded(context.Background(), 1, "", "sleep")
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("RunGuarded took %v; want < 2s (timeout must kill the hanger)", elapsed)
	}

	if res.Classification != openspec.ClassTimeout {
		t.Errorf("classification = %q; want timeout", res.Classification)
	}

	// No orphan: the group kill must have taken the grandchild sleep too.
	// Best-effort pgrep scoped to the stub's absolute path (if ps tooling is
	// unavailable this check is a no-op — same convention as the ctx-cancel
	// test).
	if _, perr := exec.LookPath("pgrep"); perr == nil {
		time.Sleep(100 * time.Millisecond) // give the OS a moment to reap

		out, _ := exec.CommandContext(context.Background(), "pgrep", "-f", stub).Output()
		if strings.TrimSpace(string(out)) != "" {
			t.Errorf("orphaned stub process after timeout kill: %s", out)
		}
	}
}

// TestRunGuarded_InteractiveEnvAndNilStdin (Test 7) verifies the child env
// carries OPEN_SPEC_INTERACTIVE=0 and stdin is nil (prompt reads hit immediate
// EOF) — both observed by the stub.
func TestRunGuarded_InteractiveEnvAndNilStdin(t *testing.T) { //nolint:paralleltest // putStubOnPATH mutates PATH
	putStubOnPATH(t)

	a := &openspec.Adapter{}

	envRes := a.RunGuarded(context.Background(), 60, "", "env")
	if envRes.Classification != openspec.ClassOK {
		t.Fatalf("env probe classification = %q; want ok (%s)", envRes.Classification, envRes.Stderr)
	}

	if !strings.Contains(envRes.Stdout, "OPEN_SPEC_INTERACTIVE=0") {
		t.Errorf("stdout = %q; want OPEN_SPEC_INTERACTIVE=0 in the child env", envRes.Stdout)
	}

	stdinRes := a.RunGuarded(context.Background(), 60, "", "stdin")
	if !strings.Contains(stdinRes.Stdout, "stdin=eof") {
		t.Errorf("stdout = %q; want immediate EOF on stdin (nil stdin guard)", stdinRes.Stdout)
	}
}

// TestRunGuarded_ClassificationTable (Test 8) verifies exit-code
// classification: exit 0 → ok; non-zero default → hard-error; non-zero with a
// fixable entry → fixable (table-driven).
func TestRunGuarded_ClassificationTable(t *testing.T) { //nolint:paralleltest // stub env vars
	putStubOnPATH(t)

	a := &openspec.Adapter{}

	cases := []struct {
		name      string
		exitCode  string
		exitClass string
		want      string
	}{
		{"exit0 ok", "", "", openspec.ClassOK},
		{"exit2 default hard", "2", "", openspec.ClassHardErr},
		{"exit2 fixable", "2", openspec.ExitClassFixable, openspec.ClassFixable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ASSGUARD_STUB_EXIT", tc.exitCode)
			t.Setenv("ASSGUARD_STUB_STDERR", "synthetic failure")

			res := a.RunGuarded(context.Background(), 60, tc.exitClass, cmdList)
			if res.Classification != tc.want {
				t.Errorf("classification = %q; want %q", res.Classification, tc.want)
			}
		})
	}
}

// bootstrapScratchProject initializes a scratch OpenSpec project with one
// change carrying an incomplete task (the three-path gate's shared fixture).
func bootstrapScratchProject(t *testing.T) string {
	t.Helper()

	scratch := t.TempDir()

	a := &openspec.Adapter{}

	res := a.RunGuarded(context.Background(), 120, "", "init", "--tools", "claude", "--force", scratch)
	if res.Classification != openspec.ClassOK {
		t.Fatalf("scratch init failed: %+v", res)
	}

	res = a.RunGuarded(context.Background(), 60, "", "new change", "gate-change")
	if res.Classification != openspec.ClassOK {
		t.Fatalf("scratch new change failed: %+v", res)
	}

	tasks := filepath.Join(scratch, "openspec", "changes", "gate-change", "tasks.md")
	if err := os.WriteFile(tasks, []byte("- [ ] 1. Incomplete task one\n"), 0o600); err != nil {
		t.Fatalf("seed tasks.md: %v", err)
	}

	return scratch
}

// TestRunGuarded_ThreePathRealBinaryGate (Tests 9-11) is the plan's tracer:
// the exact evidence class the phase gate re-runs — REAL binary, REAL scratch
// project, three outcomes (happy / fixable-failure / missing-binary). Gated
// behind ASSGUARD_OPENSPEC_BIN=1 like the other real-binary tests.
func TestRunGuarded_ThreePathRealBinaryGate(t *testing.T) { //nolint:paralleltest // t.Chdir + t.Setenv
	if os.Getenv("ASSGUARD_OPENSPEC_BIN") != "1" {
		t.Skip("set ASSGUARD_OPENSPEC_BIN=1 to run against the real openspec binary")
	}

	if _, err := exec.LookPath("openspec"); err != nil { //nolint:noinlineerr // skip-path
		t.Skip("openspec binary not on PATH even though ASSGUARD_OPENSPEC_BIN=1")
	}

	scratch := bootstrapScratchProject(t)

	t.Chdir(scratch)

	cfg, err := openspec.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	cat := toolcat.NewCatalog()

	err = openspec.RegisterTools(cat, cfg)
	if err != nil {
		t.Fatalf("RegisterTools: %v", err)
	}

	t.Run("happy path", func(t *testing.T) {
		list, ok := cat.Get("openspec:" + cmdList)
		if !ok {
			t.Fatal("openspec:list not registered")
		}

		out, err := list.Execute(context.Background(), json.RawMessage(`{"args":["--json"]}`))
		if err != nil {
			t.Fatalf("list Execute err = %v", err)
		}

		var res execResult

		err = json.Unmarshal(out, &res)
		if err != nil {
			t.Fatalf("list result not JSON: %v", err)
		}

		if res.Classification != openspec.ClassOK {
			t.Fatalf("list classification = %q; want ok (stderr: %s)", res.Classification, res.Stderr)
		}

		if !json.Valid([]byte(strings.TrimSpace(res.Stdout))) {
			t.Errorf("list --json stdout not valid JSON: %q", res.Stdout)
		}

		status, _ := cat.Get("openspec:status")

		out, err = status.Execute(context.Background(), json.RawMessage(`{"args":["--change","gate-change","--json"]}`))
		if err != nil {
			t.Fatalf("status Execute err = %v", err)
		}

		_ = json.Unmarshal(out, &res)

		if res.Classification != openspec.ClassOK || !json.Valid([]byte(strings.TrimSpace(res.Stdout))) {
			t.Errorf("status --json = %+v; want ok with valid JSON stdout", res)
		}
	})

	t.Run("fixable failure path", func(t *testing.T) {
		archive, ok := cat.Get("openspec:archive")
		if !ok {
			t.Fatal("openspec:archive not registered")
		}

		// No --yes: the interactive confirmation prompt reads nil stdin →
		// immediate EOF → non-zero exit → fixable (actionable stderr), NOT a
		// hard error and NOT a hang.
		out, err := archive.Execute(context.Background(), json.RawMessage(`{"args":["gate-change"]}`))
		if err != nil {
			t.Fatalf("archive Execute err = %v; want nil (fixable is structured, D-10)", err)
		}

		var res execResult

		_ = json.Unmarshal(out, &res)

		if res.Classification != openspec.ClassFixable {
			t.Errorf("archive classification = %q; want fixable (result %+v)", res.Classification, res)
		}

		if !strings.Contains(res.Stderr, "incomplete task") {
			t.Errorf("stderr = %q; want actionable incomplete-task detail", res.Stderr)
		}
	})

	t.Run("missing binary path", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir()) // no openspec anywhere

		list, _ := cat.Get("openspec:" + cmdList)

		out, err := list.Execute(context.Background(), json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("Execute err = %v; want nil (not-found is structured)", err)
		}

		var res execResult

		_ = json.Unmarshal(out, &res)

		if res.Classification != openspec.ClassNotFound {
			t.Errorf("classification = %q; want not-found", res.Classification)
		}
	})
}
