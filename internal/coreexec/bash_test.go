package coreexec //nolint:testpackage // internal package test (decodeJSONString/loadFixture helpers)

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/sandbox"
)

// recapturedFixturePath is the committed 12-05 re-record fixture (the fresh
// 2026-08-20 live-zcode capture — the D-04 primary route replacing the
// rotated-off Phase-9 pin).
const recapturedFixturePath = "testdata/zcode-recaptured-2026-08.json"

// decodeJSONString decodes a JSON-string Output (the captured plain-text form
// the executors marshal); fatals when the Output is not a JSON string.
func decodeJSONString(t *testing.T, raw json.RawMessage) string {
	t.Helper()

	var s string

	err := json.Unmarshal(raw, &s)
	if err != nil {
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

	if got := decodeJSONString(t, out); got != bashSentinel {
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

// TestBash_Timeout (T2 Test 4, re-pinned 12-05): `sleep 5` with timeout 100ms
// returns within ~1s with a non-nil error (IsError). The result FORM is the
// captured plain-text template — asserted by TestBash_TimeoutCapturedForm;
// the interim structured {"error":…} default was REPLACED by the re-record.
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

	if len(out) == 0 {
		t.Error("Output empty; want the captured timeout form")
	}
}

// TestBash_TimeoutCapturedForm (12-05 Task 3): the timeout result is the
// RE-PINNED captured form (2026-08-20 re-record, sess_6e4b5cc7, zcode 0.16.3,
// 18 observations): plain text `Command timed out after <duration>\n<error>
// Command was aborted before completion</error>` with a non-nil error (IsError)
// — replacing the interim structured {"error":…} corpus-absent default.
func TestBash_TimeoutCapturedForm(t *testing.T) {
	t.Parallel()

	bashExec := BashExecute(Config{WorkDir: t.TempDir()})

	out, err := bashExec(context.Background(), json.RawMessage(`{"command":"sleep 5","timeout":100}`))
	if err == nil {
		t.Fatal("err = nil; want non-nil on timeout (IsError)")
	}

	var text string

	uerr := json.Unmarshal(out, &text)
	if uerr != nil {
		t.Fatalf("Output = %s; want a plain-text JSON string (the captured form)", out)
	}

	want := "Command timed out after 100ms\n<error>Command was aborted before completion</error>"
	if text != want {
		t.Errorf("timeout form = %q; want the captured template %q", text, want)
	}
}

// TestBash_TruncationEnvelope (12-05 Task 3): outputs above the inline budget
// render the CAPTURED <persisted-output> envelope (19 observations): size in
// KB (÷1024, one decimal, ".0" stripped), a real saved-to path whose file
// carries the FULL output, and a preview cut at a word boundary ≤ 2000 chars
// with the "..." continuation line.
func TestBash_TruncationEnvelope(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bashExec := BashExecute(Config{WorkDir: dir})

	// ~16KB output — above the 15000 budget, below any test timeout.
	cmd := `{"command":"seq 1 4000 | awk '{print \"word\" $0 \" \"}'"}`

	out, err := bashExec(context.Background(), json.RawMessage(cmd))
	if err != nil {
		t.Fatalf("err = %v; want nil (a truncated success is NOT an error)", err)
	}

	var text string

	uerr := json.Unmarshal(out, &text)
	if uerr != nil {
		t.Fatalf("Output = %s; want a plain-text JSON string", out)
	}

	saved := assertTruncationEnvelopeShape(t, text)

	full, rerr := os.ReadFile(saved)
	if rerr != nil {
		t.Fatalf("saved output file %q unreadable: %v", saved, rerr)
	}

	if !strings.Contains(string(full), "word3999") {
		t.Error("saved file must carry the FULL output (not the preview)")
	}
}

// assertTruncationEnvelopeShape pins the captured envelope structure and
// returns the extracted saved-to path (the caller proves the file is real).
func assertTruncationEnvelopeShape(t *testing.T, text string) string {
	t.Helper()

	if !strings.HasPrefix(text, "<persisted-output>\nOutput too large (") {
		t.Errorf("envelope head = %q; want the captured prefix", text[:min(60, len(text))])
	}

	if !strings.Contains(text, ". Full output saved to: ") {
		t.Error("envelope missing the saved-to line")
	}

	if !strings.Contains(text, "\n\nPreview (first 2KB):\n") {
		t.Error("envelope missing the captured preview header (2000 chars → \"2KB\")")
	}

	if !strings.HasSuffix(text, "...\n</persisted-output>") {
		t.Error("envelope tail must be the \"...\" continuation + closing tag")
	}

	_, rest, ok := strings.Cut(text, "saved to: ")
	if !ok {
		t.Fatal("saved-to marker not found")
	}

	saved, _, ok := strings.Cut(rest, "\n")
	if !ok || saved == "" {
		t.Fatalf("saved-to path not extractable from %q", rest[:min(80, len(rest))])
	}

	// The preview is bounded and cut at a word boundary.
	_, rest, ok = strings.Cut(text, "Preview (first 2KB):\n")
	if !ok {
		t.Fatal("preview marker not found")
	}

	preview, _, ok := strings.Cut(rest, "\n...\n</persisted-output>")
	if !ok {
		t.Fatal("preview tail marker not found")
	}

	if len(preview) > 2000 {
		t.Errorf("preview len = %d; want ≤ 2000", len(preview))
	}

	return saved
}

// TestBash_RecapturedFixtureConformance (12-05 Task 3): the re-recorded
// fixture carries the timeout + truncation families with full provenance and
// the executor's constants agree with them (the fixture is the committed
// ground truth; lateHarvest marks the upgrade from the 08-08 defaults).
func TestBash_RecapturedFixtureConformance(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(recapturedFixturePath)
	if err != nil {
		t.Fatalf("read %s: %v (the 12-05 re-record fixture must be committed)", recapturedFixturePath, err)
	}

	var f struct {
		Provenance struct {
			ZcodeVersion string `json:"zcode_version"`
			SessionID    string `json:"session_id"`
			HarvestDate  string `json:"harvestDate"` //nolint:tagliatelle // fixture header key
		} `json:"_provenance"` //nolint:tagliatelle // fixture header key
		Tools map[string]struct {
			Results map[string]struct {
				Template string `json:"template"`
			} `json:"results"`
		} `json:"tools"`
	}

	uerr := json.Unmarshal(raw, &f)
	if uerr != nil {
		t.Fatalf("parse %s: %v", recapturedFixturePath, uerr)
	}

	if f.Provenance.ZcodeVersion == "" || f.Provenance.SessionID == "" || f.Provenance.HarvestDate == "" {
		t.Error("fixture _provenance must carry zcode_version + session_id + harvestDate")
	}

	bash := f.Tools["Bash"].Results
	if got := bash["timeout"].Template; !strings.Contains(got, "Command timed out after") ||
		!strings.Contains(got, "<error>Command was aborted before completion</error>") {
		t.Errorf("timeout family template = %q; want the captured form", got)
	}

	if got := bash["truncation"].Template; !strings.HasPrefix(got, "<persisted-output>\nOutput too large (") ||
		!strings.Contains(got, "Preview (first") {
		t.Errorf("truncation family template = %q; want the captured envelope head", got)
	}
}

// TestBash_TimeoutClampDefault (T2 Test 5): no `timeout` field → 120000ms
// default (schema-declared — the corpus shows only explicit 60000–660000
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

	// No orphan: the group kill (+ the reapGroup straggler loop) must have
	// taken the grandchild sleep too (best-effort pgrep; a no-op where ps
	// tooling is unavailable — the 08-03 convention). The kernel reaps the
	// re-parented straggler asynchronously, so poll briefly: only a
	// PERSISTENT survivor fails.
	_, perr := exec.LookPath("pgrep")
	if perr == nil {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			pout, _ := exec.CommandContext(context.Background(), "pgrep", "-f", "sleep 2999").Output()
			if strings.TrimSpace(string(pout)) == "" {
				return // reaped — no orphan
			}

			time.Sleep(100 * time.Millisecond)
		}

		pout, _ := exec.CommandContext(context.Background(), "pgrep", "-f", "sleep 2999").Output()
		t.Errorf("orphaned grandchild after group kill: %s", pout)
	}
}

// TestBash_IgnoredFields (T2 Test 8): run_in_background and
// dangerouslyDisableSandbox are accepted-and-ignored (schema-valid, zero
// corpus usage — documented corpus-absent + deferred in bash.go): the command
// runs foreground and returns its output synchronously.
// TestBash_SandboxFlagNoopByDesign (re-pin of the 08-08 IgnoredFields test,
// 12-06): dangerouslyDisableSandbox is parsed and is a no-op BY DESIGN (no
// sandbox tier exists — the locked safety model); the FOREGROUND path is
// unaffected by it. run_in_background now EXECUTES via the TaskRegistry
// (its own battery: TestBackground_*).
func TestBash_SandboxFlagNoopByDesign(t *testing.T) {
	t.Parallel()

	bashExec := BashExecute(Config{WorkDir: t.TempDir()})

	out, err := bashExec(context.Background(),
		json.RawMessage(`{"command":"echo ok","dangerouslyDisableSandbox":true}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (the flag is a deliberate no-op)", err)
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

	m := exitCodeRe.FindStringSubmatch(got)
	if len(m) != 2 || m[1] != "7" {
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

// TestIsErrorCorpusForm_BashExitCode (14-06 Task 8 pin — the inventory's
// is_error anchor row): the live 2026-08-19 corpus scan (283 isError
// observations, docs/tool-contract-inventory.md §b) shows Bash's error form
// is EXACTLY `Exit code <N>` + combined output with IsError set — pinned here
// against a real failing command so the executor's emission cannot silently
// diverge from the corpus (T-14-17).
func TestIsErrorCorpusForm_BashExitCode(t *testing.T) {
	t.Parallel()

	stub := BashExecute(Config{})

	out, err := stub(context.Background(), json.RawMessage(`{"command":"exit 3"}`))
	if err == nil {
		t.Fatal("exit 3 must surface a non-nil error (IsError)")
	}

	var text string

	uErr := json.Unmarshal(out, &text)
	if uErr != nil {
		t.Fatalf("output not the captured plain-text form: %v (%s)", uErr, out)
	}

	if !strings.HasPrefix(text, "Exit code 3") {
		t.Errorf("corpus form prefix = %q; want `Exit code 3` — the captured anchor shape", text)
	}
}

// --- 22-06 (SAND-01): the foreground sandbox wrap batteries ---

// liveSandboxHandle builds the exec-site Handle exactly as the serve flag
// path does (acp_serve.go's Run step): the D-04 policy triple over the
// session workdir + the real host probe (22-05's Resolve). The live arms skip
// (never fail) on hosts without enforcement.
func liveSandboxHandle(t *testing.T, workDir string) sandbox.Handle {
	t.Helper()

	h := sandbox.Resolve(sandbox.DefaultPolicy(workDir, os.TempDir(),
		filepath.Join(workDir, ".ass-guard")))
	if !h.Availability.Available {
		t.Skipf("sandbox enforcement unavailable on this host: %s", h.Availability.Reason)
	}

	return h
}

// dirOutsidePolicy returns a writable dir OUTSIDE every rw path of the
// flag-path policy (the workdir triple grants the WHOLE os.TempDir(), so a
// t.TempDir() sibling is INSIDE the rw set — the differential needs /var/tmp,
// which FHS keeps separate from /tmp). Skips where /var/tmp is unwritable.
func dirOutsidePolicy(t *testing.T) string {
	t.Helper()

	outside, err := os.MkdirTemp("/var/tmp", "sbx-outside-")
	if err != nil {
		t.Skipf("/var/tmp unavailable for the outside-policy dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(outside) })

	return outside
}

// captureNotes returns a Config note sink capturing every line (the OQ2 loud
// note's observation seam).
func captureNotes() (func(string, ...any), *[]string) {
	lines := &[]string{}

	return func(format string, args ...any) {
		*lines = append(*lines, fmt.Sprintf(format, args...))
	}, lines
}

// wrapRecorder swaps the wrap seam for a passthrough that records the call
// (the LIVE entry still wraps — only the observation is added) and returns
// the restore.
func wrapRecorder() (restore func(), called *bool) {
	called = new(bool)
	old := wrapSandboxCmd
	wrapSandboxCmd = func(cmd *exec.Cmd, p sandbox.Policy) error {
		*called = true

		return sandbox.WrapCmd(cmd, p)
	}

	return func() { wrapSandboxCmd = old }, called
}

// TestBashSandbox_DefaultOffArgvIdentity: with the sandbox code present but
// the operator never asking (nil Handle, or the off Handle), a plain Bash
// call builds the child argv BYTE-IDENTICALLY — the wrap seam is never
// invoked, zero notes fire (the default-OFF contract; the plan's argv
// identity battery via the seam).
func TestBashSandbox_DefaultOffArgvIdentity(t *testing.T) {
	// NOT t.Parallel: swaps the package wrap seam (sequential tests run
	// exclusively; the parallel batch must never observe the swap).
	for name, handle := range map[string]*sandbox.Handle{
		"nil handle":  nil,
		"mode-off":    {Policy: sandbox.Policy{}, Availability: sandbox.Availability{Mode: "off"}},
		"zero-assert": {Policy: sandbox.Policy{}, Availability: sandbox.Availability{}},
	} {
		t.Run(name, func(t *testing.T) {
			restore, called := wrapRecorder()
			defer restore()

			notes, lines := captureNotes()

			bashExec := BashExecute(Config{WorkDir: t.TempDir(), Sandbox: handle, SandboxNote: notes})

			out, err := bashExec(context.Background(), json.RawMessage(`{"command":"echo identity-ok"}`))
			if err != nil {
				t.Fatalf("err = %v; want nil", err)
			}

			if got := decodeJSONString(t, out); got != "identity-ok" {
				t.Errorf("output = %q; want identity-ok", got)
			}

			if *called {
				t.Error("the wrap seam was invoked with the sandbox OFF — the off path must leave argv byte-identical")
			}

			if len(*lines) != 0 {
				t.Errorf("off path emitted %d note(s): %v", len(*lines), *lines)
			}
		})
	}
}

// TestBashSandbox_OffAlwaysEscapes: --sandbox=off ALWAYS escapes — even an
// adversarial Handle carrying Mode "off" WITH Available:true never wraps (the
// escape-hatch letter: Mode "off" is the operator's explicit refusal).
func TestBashSandbox_OffAlwaysEscapes(t *testing.T) {
	// NOT t.Parallel: swaps the package wrap seam.
	restore, called := wrapRecorder()
	defer restore()

	notes, lines := captureNotes()

	//nolint:exhaustruct // adversarial: off marker + available (never produced by resolve; pins the gate)
	handle := &sandbox.Handle{Availability: sandbox.Availability{Mode: "off", Available: true}}

	bashExec := BashExecute(Config{WorkDir: t.TempDir(), Sandbox: handle, SandboxNote: notes})

	out, err := bashExec(context.Background(), json.RawMessage(`{"command":"echo escape-ok"}`))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}

	if got := decodeJSONString(t, out); got != "escape-ok" {
		t.Errorf("output = %q; want escape-ok", got)
	}

	if *called {
		t.Error("the wrap seam was invoked despite Mode off — off ALWAYS escapes")
	}

	if len(*lines) != 0 {
		t.Errorf("off path emitted %d note(s): %v", len(*lines), *lines)
	}
}

// TestBashSandbox_LiveConfinementDeniesNetworkWriteOutside: with the sandbox
// ON and the host available, a Bash call executes CONFINED through the FULL
// wrap path (wrapSandboxCmd → the re-exec loader → the landlock ruleset): a
// curl-style connect FAILS, a write outside the rw triple fails EPERM, and a
// write INSIDE the session tmp succeeds (22-05's live shape, driven through
// the executor — not raw package calls).
func TestBashSandbox_LiveConfinementDeniesNetworkWriteOutside(t *testing.T) { //nolint:funlen // the live triple battery
	t.Parallel()

	workDir := t.TempDir()
	handle := liveSandboxHandle(t, workDir)

	bashExec := BashExecute(Config{WorkDir: workDir, Sandbox: &handle,
		SandboxNote: func(string, ...any) {}})

	// 1. Inside-tmp write succeeds (the rw triple grants the system tmp).
	out, err := bashExec(context.Background(),
		json.RawMessage(`{"command":"touch `+os.TempDir()+"/"+fmt.Sprintf("sbx-inside-%d", os.Getpid())+`"}`))
	if err != nil {
		t.Errorf("inside-tmp write failed under confinement: %v (%s)", err, decodeJSONString(t, out))
	}

	// 2. Outside write fails EPERM (a dir outside the rw triple).
	outside := dirOutsidePolicy(t)
	out, err = bashExec(context.Background(),
		json.RawMessage(`{"command":"touch `+outside+`/nope"}`))
	if err == nil {
		t.Errorf("outside write SUCCEEDED — the child is unconfined: %s", decodeJSONString(t, out))
	}

	if !strings.Contains(decodeJSONString(t, out), "ermission denied") {
		t.Errorf("outside write failed without the EPERM form: %q", decodeJSONString(t, out))
	}

	// 3. Network connect fails (the D-04 deny; local listener).
	ln, lerr := net.Listen("tcp", "127.0.0.1:0")
	if lerr != nil {
		t.Fatalf("listen: %v", lerr)
	}
	defer func() { _ = ln.Close() }()

	srv := &http.Server{}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port
	out, err = bashExec(context.Background(),
		json.RawMessage(`{"command":"curl -s --max-time 3 -o /dev/null http://127.0.0.1:`+
			strconv.Itoa(port)+`/ ; echo CURL_EXIT_$?"}`))
	if err != nil {
		t.Fatalf("the curl-style call itself errored: %v (%s)", err, decodeJSONString(t, out))
	}

	if got := decodeJSONString(t, out); got == "CURL_EXIT_0" {
		t.Errorf("curl connect SUCCEEDED under --sandbox=on — the child is unconfined: %q", got)
	}
}

// TestBashSandbox_DisableFlagOnRunsUnconfinedWithNote: OQ2 — with the sandbox
// ON and available, dangerouslyDisableSandbox=true runs the command UNCONFINED
// with exactly ONE loud note carrying the counter: the denied-by-confinement
// touch-outside now SUCCEEDS (the escape is real, not cosmetic).
func TestBashSandbox_DisableFlagOnRunsUnconfinedWithNote(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	handle := liveSandboxHandle(t, workDir)

	notes, lines := captureNotes()
	bashExec := BashExecute(Config{WorkDir: workDir, Sandbox: &handle, SandboxNote: notes})

	outside := dirOutsidePolicy(t)
	out, err := bashExec(context.Background(),
		json.RawMessage(`{"command":"touch `+outside+`/escaped","dangerouslyDisableSandbox":true}`))
	if err != nil {
		t.Fatalf("the disabled-flag call failed (it must run unconfined and succeed): %v (%s)",
			err, decodeJSONString(t, out))
	}

	if len(*lines) != 1 {
		t.Fatalf("notes = %d; want exactly ONE loud note (got %v)", len(*lines), *lines)
	}

	if !strings.Contains((*lines)[0], "UNCONFINED") || !strings.Contains((*lines)[0], "dangerouslyDisableSandbox") {
		t.Errorf("the note does not name the escape: %q", (*lines)[0])
	}

	if !strings.Contains((*lines)[0], "#") {
		t.Errorf("the note carries no counter: %q", (*lines)[0])
	}
}

// TestBashSandbox_DisableFlagOffIsNoop: with the sandbox OFF (the default),
// dangerouslyDisableSandbox stays today's documented no-op — the command runs
// foreground, no note fires (22-06 retires the no-op COMMENT only for the
// on-arm; the off-arm IS the no-op, both arms pinned).
func TestBashSandbox_DisableFlagOffIsNoop(t *testing.T) {
	// NOT t.Parallel: swaps the package wrap seam.
	restore, called := wrapRecorder()
	defer restore()

	notes, lines := captureNotes()

	bashExec := BashExecute(Config{WorkDir: t.TempDir(), SandboxNote: notes})

	out, err := bashExec(context.Background(),
		json.RawMessage(`{"command":"echo disable-off-ok","dangerouslyDisableSandbox":true}`))
	if err != nil {
		t.Fatalf("err = %v; want nil (the off-arm no-op)", err)
	}

	if got := decodeJSONString(t, out); got != "disable-off-ok" {
		t.Errorf("output = %q; want disable-off-ok", got)
	}

	if *called {
		t.Error("the wrap seam fired on the off path")
	}

	if len(*lines) != 0 {
		t.Errorf("the off-arm no-op emitted notes: %v", *lines)
	}
}

// TestBashSandbox_UnavailableNotedPerRun: enabled-but-unavailable (faked
// availability) runs UNCONFINED with a per-run note + counter — NEVER a
// silent fail-open (the plan prohibition). Two runs → two individually
// numbered notes.
func TestBashSandbox_UnavailableNotedPerRun(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	//nolint:exhaustruct // faked availability: the on marker with the probe failed
	handle := &sandbox.Handle{
		Policy:       sandbox.DefaultPolicy(workDir, os.TempDir(), filepath.Join(workDir, ".ass-guard")),
		Availability: sandbox.Availability{Mode: "landlock", Reason: "faked-unavailable"},
	}

	notes, lines := captureNotes()
	bashExec := BashExecute(Config{WorkDir: workDir, Sandbox: handle, SandboxNote: notes})

	outside := t.TempDir()
	for i := 0; i < 2; i++ {
		out, err := bashExec(context.Background(),
			json.RawMessage(`{"command":"touch `+outside+`/u`+strconv.Itoa(i)+`"}`))
		if err != nil {
			t.Fatalf("run %d failed (unavailable must degrade to unconfined, not fail): %v (%s)",
				i, err, decodeJSONString(t, out))
		}
	}

	if len(*lines) != 2 {
		t.Fatalf("notes = %d; want one PER RUN (got %v)", len(*lines), *lines)
	}

	if !strings.Contains((*lines)[0], "faked-unavailable") {
		t.Errorf("note 0 does not name the reason: %q", (*lines)[0])
	}

	// The counter is PROCESS-WIDE (other batteries increment it too), so the
	// pin is the INCREMENT between this run's two notes, not an absolute.
	numOf := func(line string) int {
		m := regexp.MustCompile(`#(\d+)`).FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("note carries no counter: %q", line)
		}

		n, _ := strconv.Atoi(m[1])

		return n
	}

	if !(numOf((*lines)[1]) > numOf((*lines)[0])) {
		t.Errorf("the counter does not increment across runs: %q then %q", (*lines)[0], (*lines)[1])
	}
}

// TestBashSandbox_GroupKillReachesWrappedChild: the wrap substitutes argv
// BEFORE the group discipline — a WRAPPED long-running child (with its own
// grandchild) still dies via the existing timeout path (killGroupOnCtx +
// reapGroup; D-05: signals are neither FS nor network operations, so the
// group-kill reaches confined children).
func TestBashSandbox_GroupKillReachesWrappedChild(t *testing.T) {
	// NOT t.Parallel: swaps the package wrap seam (and pins kill timing).
	workDir := t.TempDir()
	handle := liveSandboxHandle(t, workDir)

	restore, called := wrapRecorder()
	defer restore()

	bashExec := BashExecute(Config{WorkDir: workDir, Sandbox: &handle,
		SandboxNote: func(string, ...any) {}})

	_, err := bashExec(context.Background(), json.RawMessage(
		`{"command":"sleep 2997 & sleep 2998","timeout":700}`))
	if err == nil {
		t.Fatal("the timed-out wrapped call returned nil err; want the timeout form")
	}

	if !*called {
		t.Fatal("the wrap seam never fired — the child was not wrapped for the group-kill row")
	}

	// The wrapped group must be GONE (the TestBash_ProcessGroupKill shape:
	// no orphaned straggler survives the SIGKILL + reap).
	if _, perr := exec.LookPath("pgrep"); perr == nil {
		deadline := time.Now().Add(3 * time.Second)

		for time.Now().Before(deadline) {
			pout, _ := exec.CommandContext(context.Background(), "pgrep", "-f", "sleep 299").Output()
			if strings.TrimSpace(string(pout)) == "" {
				return // group provably empty — the lifecycle survived the wrap
			}

			time.Sleep(100 * time.Millisecond)
		}

		pout, _ := exec.CommandContext(context.Background(), "pgrep", "-f", "sleep 299").Output()
		t.Errorf("orphaned wrapped child after group kill: %s", pout)
	}
}
