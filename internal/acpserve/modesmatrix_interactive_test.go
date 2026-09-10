package acpserve //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// matrixPromptContains reports whether any captured provider request
// carried needle (the received-prompt lens — the expansion probe's
// evidence).
func matrixPromptContains(stub *simStub, needle string) bool {
	for _, p := range stub.recordedPrompts() {
		if strings.Contains(p, needle) {
			return true
		}
	}

	return false
}

// TestModesMatrixInteractiveSurfaces (24-05 Task 2) exercises the
// interactive commands and skills cells: each typed slash invocation
// expands into the fixture body BEFORE the provider call — the received
// prompt carries the body with the args substituted (D-13's functional bar,
// observed at the provider). The collision probe then pins the resolution
// ORDER (probe ordering): a project-tree command sharing the fixture's name
// WINS (the loader's D-06 precedence — project over the installed-plugin
// tiers — the same winner the ecosys precedence tests lock; cells never
// silently merge).
func TestModesMatrixInteractiveSurfaces(t *testing.T) { //nolint:funlen,paralleltest // HOME-pinned legs; one story
	t.Setenv("HOME", t.TempDir())

	// Serve A — the fixture alone: commands + skills cells.
	workDir := simulatorWorkDir(t, "PENDING_STUB_URL")

	modesmatrix.MountFixture(t, workDir)

	stub, cli, sessionID, _ := modesMatrixServe(t, workDir, "PENDING_STUB_URL", []simTurnScript{
		{phases: []simPhase{{text: "command turn done"}}},
		{phases: []simPhase{{text: "skill turn done"}}},
	})

	matrixPromptTurn(t, cli, sessionID, "/matrix-echo interactive-args", "mx-cmd-1")
	matrixPromptTurn(t, cli, sessionID, "/matrix-skill interactive-skill-args", "mx-skl-1")

	const cmdMarker = "MATRIX-ECHO-EXPANSION invoked: interactive-args"
	if !matrixPromptContains(stub, cmdMarker) {
		t.Errorf("no provider request carried %q — the fixture command did not expand (received: %v)",
			cmdMarker, stub.recordedPrompts())
	}

	matrixRegistry.Record(
		modesmatrix.ModeInteractive, modesmatrix.SurfaceCommands, modesmatrix.StatusPass,
		"/matrix-echo expanded to the fixture body before the provider call (received-prompt lens)",
	)

	const skillMarker = "MATRIX-SKILL-BODY invoked: interactive-skill-args"
	if !matrixPromptContains(stub, skillMarker) {
		t.Errorf("no provider request carried %q — the fixture skill did not expand (received: %v)",
			skillMarker, stub.recordedPrompts())
	}

	matrixRegistry.Record(
		modesmatrix.ModeInteractive, modesmatrix.SurfaceSkills, modesmatrix.StatusPass,
		"/matrix-skill expanded to the SKILL.md body with args substituted (received-prompt lens)",
	)

	// Serve B — the collision probe: a project .claude/commands file sharing
	// the fixture command's name. The project tree outranks the
	// installed-plugin tiers, so the PROJECT body is the one that expands.
	collideDir := simulatorWorkDir(t, "PENDING_STUB_URL")

	modesmatrix.MountFixture(t, collideDir)

	projCmd := filepath.Join(collideDir, ".claude", "commands", "matrix-echo.md")
	if err := os.MkdirAll(filepath.Dir(projCmd), 0o750); err != nil {
		t.Fatalf("mkdir project commands: %v", err)
	}

	if err := os.WriteFile(projCmd, []byte(
		"---\ndescription: project-scope collision winner\n---\nMATRIX-ECHO-PROJECT-WINS invoked: $ARGUMENTS\n"),
		0o600); err != nil {
		t.Fatalf("write project collision command: %v", err)
	}

	stubB, cliB, sessB, _ := modesMatrixServe(t, collideDir, "PENDING_STUB_URL", []simTurnScript{
		{phases: []simPhase{{text: "collision turn done"}}},
	})

	matrixPromptTurn(t, cliB, sessB, "/matrix-echo collide-args", "mx-col-1")

	if !matrixPromptContains(stubB, "MATRIX-ECHO-PROJECT-WINS invoked: collide-args") {
		t.Error("the project-tree collision entry did not win — resolution order not the locked project-over-plugin precedence")
	}

	if matrixPromptContains(stubB, "MATRIX-ECHO-EXPANSION") {
		t.Error("the plugin-bundled loser still expanded — the collision merged instead of resolving to one winner")
	}

	// Resolution-order observation recorded (probe ordering): project
	// file-command > plugin bundle, per the loader's locked precedence.
	matrixRegistry.Record(
		modesmatrix.ModeInteractive, modesmatrix.SurfaceCommands, modesmatrix.StatusPass,
		"expanded via fixture plugin; collision probe: project .claude/commands entry wins over the plugin bundle (locked precedence, one winner — no merge)",
	)

	matrixRegistry.ReportT(t)
}

// TestModesMatrixInteractiveEmpty (the empty-input row, interactive leg):
// with NO fixture mounted, the typed invocation falls through UNEXPANDED —
// the provider receives the typed text verbatim and the marker file stays
// ABSENT. Absence is the pass condition, never an error.
func TestModesMatrixInteractiveEmpty(t *testing.T) { //nolint:paralleltest // HOME-pinned leg
	t.Setenv("HOME", t.TempDir())

	workDir := simulatorWorkDir(t, "PENDING_STUB_URL") // deliberately NO fixture

	stub, cli, sessionID, _ := modesMatrixServe(t, workDir, "PENDING_STUB_URL", []simTurnScript{
		{phases: []simPhase{{text: "empty fixture turn done"}}},
	})

	const typed = "/matrix-echo ghost-args"

	matrixPromptTurn(t, cli, sessionID, typed, "mx-emp-1")

	if !matrixPromptContains(stub, typed) {
		t.Errorf("no provider request carried the typed invocation verbatim (fallthrough broken): %v", stub.recordedPrompts())
	}

	if _, err := os.Stat(modesmatrix.MarkerPath(workDir)); !os.IsNotExist(err) {
		t.Errorf("empty row: marker file exists at %s; want ABSENT", modesmatrix.MarkerPath(workDir))
	}

	t.Log("MODES-MATRIX empty-row interactive: invocation fell through verbatim; marker file ABSENT")
}
