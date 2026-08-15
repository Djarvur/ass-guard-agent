package coreexec

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// decodeJSONString decodes a JSON-string Output (the captured plain-text form
// the executors marshal); fatals when the Output is not a JSON string.
func decodeJSONString(t *testing.T, raw json.RawMessage) string {
	t.Helper()

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("Output is not a JSON string: %v (%s)", err, raw)
	}

	return s
}

// TestBash_SuccessForm (T2 Test 1): `printf hello` returns the JSON-string
// Output `hello` with nil error (IsError=false on the ToolResult).
func TestBash_SuccessForm(t *testing.T) {
	t.Parallel()

	bashExec := BashExecute(Config{WorkDir: t.TempDir()})

	out, err := bashExec(context.Background(), json.RawMessage(`{"command":"printf hello","description":"print"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (success)", err)
	}

	if got := decodeJSONString(t, out); got != "hello" {
		t.Errorf("Output = %q; want the plain captured form %q", got, "hello")
	}
}

// TestBash_EmptyOutputSentinel (T2 Test 2): a command with no output returns
// exactly `(Bash completed with no output)` — the captured sentinel,
// byte-for-byte.
func TestBash_EmptyOutputSentinel(t *testing.T) {
	t.Parallel()

	bashExec := BashExecute(Config{WorkDir: t.TempDir()})

	out, err := bashExec(context.Background(), json.RawMessage(`{"command":"true"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	if got := decodeJSONString(t, out); got != "(Bash completed with no output)" {
		t.Errorf("Output = %q; want the captured sentinel byte-for-byte", got)
	}
}

// TestBash_ErrorForm (T2 Test 3): a failing command returns Output
// `Exit code 3\nout\nerr` AND a non-nil error (so executeOne sets IsError —
// the captured 137/137 error shape: header line, then output, stdout before
// stderr).
func TestBash_ErrorForm(t *testing.T) {
	t.Parallel()

	bashExec := BashExecute(Config{WorkDir: t.TempDir()})

	out, err := bashExec(context.Background(),
		json.RawMessage(`{"command":"sh -c 'echo out; echo err >&2; exit 3'"}`))
	if err == nil {
		t.Fatal("err = nil; want non-nil (IsError=true on the ToolResult)")
	}

	if got := decodeJSONString(t, out); got != "Exit code 3\nout\nerr" {
		t.Errorf("Output = %q; want %q", got, "Exit code 3\nout\nerr")
	}
}

// TestBash_Timeout (T2 Test 4): `sleep 5` with timeout 100ms returns within
// ~1s a structured `{"error":"bash: command timed out after …"}`-family
// Output with a non-nil error; corpus-absent form (follows the shipped
// structured-error convention — flagged in bash.go).
func TestBash_Timeout(t *testing.T) {
	t.Parallel()

	bashExec := BashExecute(Config{WorkDir: t.TempDir()})

	start := time.Now()

	out, err := bashExec(context.Background(), json.RawMessage(`{"command":"sleep 5","timeout":100}`))
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("timeout took %v; want < 1s (the per-call timeout must bound runaway commands)", elapsed)
	}

	if err == nil {
		t.Fatal("err = nil; want non-nil on timeout")
	}

	var structured struct {
		Error string `json:"error"`
	}

	if uerr := json.Unmarshal(out, &structured); uerr != nil || structured.Error == "" {
		t.Errorf("Output = %s; want the structured {\"error\":…} timeout form", out)
	}

	if !strings.Contains(structured.Error, "timed out") {
		t.Errorf("error text = %q; want the timed-out wording", structured.Error)
	}
}

// TestBash_TimeoutClampDefault (T2 Test 5): no `timeout` field → 120000ms
// default (schema-declared — the corpus shows only explicit 60000–660000
// values, never the default); `timeout: 9e9` → clamped to 600000 (the
// schema's declared max).
func TestBash_TimeoutClampDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   float64
		want int
	}{
		{"absent/zero → schema-declared default", 0, 120000},
		{"negative → schema-declared default", -5, 120000},
		{"in-range passes", 90000, 90000},
		{"schema max passes", 600000, 600000},
		{"9e9 clamps to schema max", 9e9, 600000},
		{"fractional rounds", 500.9, 500},
	}

	for _, tc := range cases {
		if got := resolveBashTimeout(tc.in); got != tc.want {
			t.Errorf("%s: resolveBashTimeout(%v) = %d; want %d", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestBash_WorkDir (T2 Test 6): `pwd` in a configured temp WorkDir returns
// that directory (the session workdir is the command's only workdir — the
// schema has no cwd field).
func TestBash_WorkDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	bashExec := BashExecute(Config{WorkDir: dir})

	out, err := bashExec(context.Background(), json.RawMessage(`{"command":"pwd"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	got := decodeJSONString(t, out)

	// macOS tempdirs are symlinked (/var → /private/var); compare the
	// resolved forms so the assertion is about the WORKDIR, not the symlink.
	want, rerr := filepath.EvalSymlinks(dir)
	if rerr != nil {
		t.Fatalf("resolve %s: %v", dir, rerr)
	}

	gotResolved, rerr := filepath.EvalSymlinks(strings.TrimSpace(got))
	if rerr != nil {
		t.Fatalf("resolve %s: %v", got, rerr)
	}

	if gotResolved != want {
		t.Errorf("pwd = %q (resolved %q); want the configured WorkDir %q", got, gotResolved, want)
	}
}

// TestBash_ProcessGroupKill (T2 Test 7): `sh -c 'sleep 2999 & wait'` under a
// 200ms timeout — the wrapper's grandchild dies with the group: the call
// returns promptly (no Wait stall), a SECOND command runs immediately after
// (no stall), and no `sleep 2999` process survives (08-03's killGroup
// discipline mirrored; the marker duration scoping avoids pgrep collisions).
func TestBash_ProcessGroupKill(t *testing.T) { //nolint:paralleltest // PATH-scoped pgrep check
	bashExec := BashExecute(Config{WorkDir: t.TempDir()})

	start := time.Now()

	out, err := bashExec(context.Background(),
		json.RawMessage(`{"command":"sh -c 'sleep 2999 & wait'","timeout":200}`))
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("group-kill took %v; want < 2s (grandchild held the pipes)", elapsed)
	}

	if err == nil {
		t.Fatal("err = nil; want non-nil (timeout)")
	}

	var structured struct {
		Error string `json:"error"`
	}

	_ = json.Unmarshal(out, &structured)

	// A second command runs immediately — no lingering stall.
	second, serr := bashExec(context.Background(), json.RawMessage(`{"command":"true"}`))
	if serr != nil {
		t.Fatalf("second command err = %v (executor stalled after the kill?)", serr)
	}

	if got := decodeJSONString(t, second); got != "(Bash completed with no output)" {
		t.Errorf("second command Output = %q; want the sentinel", got)
	}

	// No orphan: the group kill must have taken the grandchild sleep too
	// (best-effort pgrep; a no-op where ps tooling is unavailable — the
	// 08-03 convention).
	if _, perr := exec.LookPath("pgrep"); perr == nil {
		time.Sleep(150 * time.Millisecond) // give the OS a moment to reap

		pout, _ := exec.CommandContext(context.Background(), "pgrep", "-f", "sleep 2999").Output()
		if strings.TrimSpace(string(pout)) != "" {
			t.Errorf("orphaned grandchild after group kill: %s", pout)
		}
	}
}

// TestBash_IgnoredFields (T2 Test 8): run_in_background and
// dangerouslyDisableSandbox are accepted-and-ignored (schema-valid, zero
// corpus usage — documented corpus-absent + deferred in bash.go): the command
// runs foreground and returns its output synchronously.
func TestBash_IgnoredFields(t *testing.T) {
	t.Parallel()

	bashExec := BashExecute(Config{WorkDir: t.TempDir()})

	out, err := bashExec(context.Background(),
		json.RawMessage(`{"command":"echo ok","run_in_background":true,"dangerouslyDisableSandbox":true}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (fields accepted)", err)
	}

	if got := decodeJSONString(t, out); got != "ok" {
		t.Errorf("Output = %q; want foreground synchronous output %q", got, "ok")
	}
}

// exitCodeRe matches the captured Bash error header (Exit code <N>).
var exitCodeRe = regexp.MustCompile(`^Exit code (\d+)\n`)

// TestBash_FixtureConformance (T2 Test 9): the success, sentinel, and error
// outputs equal the templates pinned in T1's fixture (string-compare against
// the fixture's literal forms).
func TestBash_FixtureConformance(t *testing.T) {
	t.Parallel()

	f := loadFixture(t)
	bash := f.Tools["Bash"].Results

	bashExec := BashExecute(Config{WorkDir: t.TempDir()})
	ctx := context.Background()

	// Sentinel: byte-for-byte the fixture literal.
	out, err := bashExec(ctx, json.RawMessage(`{"command":"true"}`))
	if err != nil {
		t.Fatalf("sentinel err = %v", err)
	}

	if got := decodeJSONString(t, out); got != bash["success_no_output"].Template {
		t.Errorf("sentinel = %q; want fixture literal %q", got, bash["success_no_output"].Template)
	}

	// Error: the fixture's literal_prefix + <N> header + output body.
	out, err = bashExec(ctx, json.RawMessage(`{"command":"sh -c 'echo boom >&2; exit 7'"}`))
	if err == nil {
		t.Fatal("error-form err = nil; want non-nil")
	}

	got := decodeJSONString(t, out)
	if !strings.HasPrefix(got, bash["error"].LiteralPrefix) {
		t.Errorf("error form = %q; want the fixture prefix %q", got, bash["error"].LiteralPrefix)
	}

	if m := exitCodeRe.FindStringSubmatch(got); m == nil || m[1] != "7" {
		t.Errorf("error form = %q; want the Exit code <N> header", got)
	}

	// Success: plain combined output.
	out, err = bashExec(ctx, json.RawMessage(`{"command":"printf fixture"}`))
	if err != nil {
		t.Fatalf("success err = %v", err)
	}

	if got := decodeJSONString(t, out); got != "fixture" {
		t.Errorf("success form = %q; want the plain combined output", got)
	}
}
