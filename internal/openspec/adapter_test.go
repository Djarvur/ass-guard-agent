package openspec_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/openspec"
)

// stubPath returns the absolute path to the testdata stub script.
func stubPath(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	return filepath.Join(filepath.Dir(file), "testdata", "openspec-stub.sh")
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

	if err := os.WriteFile(target, data, 0o755); err != nil {
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
func TestAdapter_CtxCancelKillsProcess(t *testing.T) { //nolint:paralleltest // putStubOnPATH mutates PATH; pgrep could match sibling stubs
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
	if _, perr := exec.LookPath("pgrep"); perr == nil {
		// Give the OS a moment to reap.
		time.Sleep(100 * time.Millisecond)

		out, _ := exec.Command("pgrep", "-f", "openspec-stub").Output()
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

	if _, err := exec.LookPath("openspec"); err != nil {
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
