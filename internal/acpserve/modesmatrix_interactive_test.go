package acpserve //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/modesmatrix"
)

// The ECOS-04 (TAIL-03, 24-05) interactive-mode leg: the REAL acpserve.Run
// composition over io.Pipes (the 16-06 simulator discipline verbatim —
// scripted SSE provider stub, temp project with the synthetic fixture
// plugin MOUNTED as an installed plugin, guard timeouts, staged assertions)
// driven through one scripted turn whose tool call triggers the fixture's
// PreToolUse hook. The cell's bar is D-13's: a FUNCTIONAL outcome — the
// marker file RECEIVED the hook's stdin JSON — never merely that discovery
// listed the plugin.
//
// Determinism: HOME is pinned to a temp dir (user-scope discovery and any
// operator hooks are out of the picture — the only hooks that can fire are
// the fixture's), fixed script, guard timeouts on every wait.

// matrixRegistry is this test binary's cell registry (the full 4x3 grid
// prints at each matrix test's end; cells this package does not exercise
// print as not-exercised).
var matrixRegistry = modesmatrix.NewRegistry() //nolint:gochecknoglobals // per-test-binary registry

// modesMatrixServe boots the real Run composition over io.Pipe against a
// scripted stub for workDir (whose .ass-guard/config.yaml still carries the
// placeholder base URL — repointed BEFORE Run loads it at startup), stages
// the initialize handshake + session/new, and returns the stub, the client,
// the session id, and a stop func.
func modesMatrixServe( //nolint:funlen // the simulator bootstrap reads best as one flow
	t *testing.T, workDir, placeholder string, script []simTurnScript,
) (*simStub, *simClient, string, func()) {
	t.Helper()

	t.Setenv("ZAI_API_KEY", "") // the config literal key wins (deterministic provider construction)

	stub := newSimStub(script)
	t.Cleanup(stub.srv.Close)

	simRepointConfig(t, workDir, placeholder, stub.srv.URL)

	srvInR, srvInW := io.Pipe()
	cliR, cliOutW := io.Pipe()
	stderr := &syncBuffer{}

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)

	go func() {
		serveDone <- Run(ctx, srvInR, cliOutW, stderr, &Options{
			Profile: profileZcode, MaxConcurrent: 2,
			ProfilesDir: repoProfilesDir(t), WorkDir: workDir,
		})
	}()

	stop := func() {
		cancel()

		_ = srvInW.Close()
		_ = cliOutW.Close()
		_ = srvInR.Close()
		_ = cliR.Close()

		select {
		case <-serveDone:
		case <-time.After(simGuardTimeout):
			t.Errorf("modesmatrix: serve did not exit")
		}
	}

	t.Cleanup(stop)

	cli := newSimClient(t, cliR, srvInW)

	simStageInitialize(t, cli)

	sessionID := simStageSessionNew(t, cli, workDir)

	return stub, cli, sessionID, stop
}

// matrixPromptTurn sends one prompt and consumes the turn through its
// response (any pre-response session/update frames are admitted — the
// available_commands_update/usage families are pre-turn noise).
func matrixPromptTurn(t *testing.T, cli *simClient, sessionID, text, respID string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":`+strconv.Quote(respID)+
		`,"method":"session/prompt","params":{"sessionId":`+simJSONStr(sessionID)+
		`,"prompt":[{"type":"text","text":`+simJSONStr(text)+`}]}}`)

	for {
		m := cli.next()
		if isResponseID(m, strconv.Quote(respID)) {
			simAssertStopReason(t, m.Result, strconv.Quote(respID), simStopEndTurn)

			return
		}
	}
}

// matrixMarkerHooks parses the marker file's lines and returns the parsed
// hook payloads whose hook_event_name equals event.
func matrixMarkerHooks(t *testing.T, workDir, event string) []map[string]any {
	t.Helper()

	lines, err := modesmatrix.MarkerLines(workDir)
	if err != nil {
		t.Fatalf("modesmatrix: marker read: %v", err)
	}

	var out []map[string]any

	for _, line := range lines {
		var payload map[string]any

		if json.Unmarshal([]byte(line), &payload) != nil {
			continue // not a hook payload line — never fail the lens on it
		}

		if payload["hook_event_name"] == event {
			out = append(out, payload)
		}
	}

	return out
}

// TestModesMatrixInteractive (24-05 Task 1, the TRACER cell) proves one
// matrix cell fully real: fixture plugin mounted in the temp project →
// ecosys discovery at serve startup → the session gate's PreToolUse verdict
// consult (21-06) executing the fixture hook → the OBSERVED EFFECT — the
// hook's stdin JSON (event, tool name, session id) appended to the marker
// file inside the temp project. This is the harness skeleton every other
// cell rides; the remaining eleven print as not-exercised (no cell
// fabricated).
func TestModesMatrixInteractive(t *testing.T) { //nolint:paralleltest // HOME-pinned serve leg
	t.Setenv("HOME", t.TempDir()) // hermetic user-scope discovery (no operator hooks can fire)

	workDir := simulatorWorkDir(t, "PENDING_STUB_URL")

	modesmatrix.MountFixture(t, workDir)

	stub, cli, sessionID, _ := modesMatrixServe(t, workDir, "PENDING_STUB_URL", []simTurnScript{
		{phases: []simPhase{
			{callID: "call_matrix_read", name: simToolRead, input: simReadInput(workDir)},
		}},
		{phases: []simPhase{{text: "matrix interactive turn complete"}}},
	})

	// One scripted turn: the model reads notes.md — the gate head fires the
	// fixture's PreToolUse hook (matcher Read) BEFORE the tool executes.
	matrixPromptTurn(t, cli, sessionID, "run the matrix tracer turn", "mx-1")

	// D-13's bar — the FUNCTIONAL outcome: the marker file RECEIVED the
	// hook's stdin JSON, not merely discovery listing the plugin.
	pre := matrixMarkerHooks(t, workDir, "PreToolUse")
	if len(pre) == 0 {
		t.Fatal("no PreToolUse payload in the marker file — the fixture hook never fired (discovery-only is not D-13 evidence)")
	}

	payload := pre[0]

	if got := payload["tool_name"]; got != simToolRead {
		t.Errorf("PreToolUse tool_name = %v; want %q", got, simToolRead)
	}

	if got := payload["session_id"]; got != sessionID {
		t.Errorf("PreToolUse session_id = %v; want the live session %q", got, sessionID)
	}

	if stub.calls.Load() < 1 {
		t.Errorf("provider calls = %d; want at least 1 (the turn really ran)", stub.calls.Load())
	}

	matrixRegistry.Record(
		modesmatrix.ModeInteractive, modesmatrix.SurfaceHooks, modesmatrix.StatusPass,
		"PreToolUse fired via the gate head on a scripted Read; marker file carries the hook's stdin JSON",
	)

	defer matrixRegistry.ReportT(t)
}
