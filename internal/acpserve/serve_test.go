package acpserve //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/sched"
)

// stubServeRunner is the transitional stub TurnRunner for the stdout-discipline
// tests (15-05): the moved tests drive initialize-only frames, so the runner
// never executes. When plan 15-06 reunifies Run around runtime.NewRunner, the
// real runner returns and this stub dissolves with the PrepareServe/FinishServe
// split it exists to bridge.
type stubServeRunner struct{}

func (stubServeRunner) Run(
	_ context.Context, _ string, _ acp.ChunkEmitter, _ []acp.ContentBlock,
) (string, error) {
	return "end_turn", nil
}

// noopFinishHooks arms every hook FinishServe invokes (the stub runner has no
// members to act on).
func noopFinishHooks() FinishHooks {
	return FinishHooks{
		AssignSchedule: func(*sched.ScheduleStore) {},
		InjectEmitter:  func(func(string) acp.ChunkEmitter) {},
		StartScheduler: func(context.Context) {},
		CloseSessions:  func() {},
	}
}

// runServeForTest is the call-through equivalent of the pre-carve runACPServe
// for the initialize-only surface these tests drive: PrepareServe → stub runner
// → FinishServe (transitional 15-05 shape; dissolves in 15-06).
func runServeForTest(ctx context.Context, in io.Reader, out, stderr io.Writer, opts *Options) error {
	prep, err := PrepareServe(ctx, stderr, opts)
	if err != nil {
		return err
	}

	return FinishServe(ctx, in, out, stderr, &prep.Opts, stubServeRunner{}, noopFinishHooks())
}

// repoProfilesDir returns the repo-root profiles/ directory (the test runs from
// internal/acpserve/, so the repo root is two levels up).
func repoProfilesDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "..", "profiles"))
	if err != nil {
		t.Fatalf("resolve repo profiles dir: %v", err)
	}

	return dir
}

// TestACPServeWiresStdoutClean verifies that running `acp serve` against a
// canned initialize frame produces the initialize response on stdout and sends
// all diagnostics to stderr — transport discipline (stdout = ACP frames only).
func TestACPServeWiresStdoutClean(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":` +
		`{"protocolVersion":1,"clientCapabilities":{},` +
		`"clientInfo":{"name":"test","version":"0"}}}
`)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	ctx := t.Context()

	err := runServeForTest(ctx, in, &stdout, &stderr, &Options{
		Profile: "zcode", MaxConcurrent: 6,
		ProfilesDir: repoProfilesDir(t), WorkDir: t.TempDir(),
	})
	if err != nil && !errors.Is(err, io.EOF) {
		t.Logf("serve returned %v (acceptable)", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"agentCapabilities"`) {
		t.Errorf("stdout missing agentCapabilities in initialize response: %s", out)
	}

	if !strings.Contains(out, `"loadSession":false`) {
		t.Errorf("stdout missing loadSession:false: %s", out)
	}

	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}

		var m map[string]any

		err := json.Unmarshal([]byte(line), &m)
		if err != nil {
			t.Errorf("stdout line %d is not valid JSON (transport discipline): %v (line=%q)", i, err, line)
		}
	}
}

// TestACPServeNoStdoutPollutionFromLogs verifies stderr gets diagnostics and
// stdout NEVER receives log bytes (Pitfall 1).
func TestACPServeNoStdoutPollutionFromLogs(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1}}
`)

	var stdout, stderr bytes.Buffer

	ctx := t.Context()

	_ = runServeForTest(ctx, in, &stdout, &stderr, &Options{
		Profile: "zcode", MaxConcurrent: 6,
		ProfilesDir: repoProfilesDir(t), WorkDir: t.TempDir(),
	})
	if strings.Contains(stdout.String(), "ass-guard/acp") {
		t.Errorf("stdout contains a log prefix (transport discipline violation): %s", stdout.String())
	}
}
