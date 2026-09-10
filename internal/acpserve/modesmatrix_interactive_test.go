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
func modesMatrixServe( //nolint:gocritic // unnamed triple reads as the simulator triple
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

	cli.sendf(`{"jsonrpc":"2.0","id":` + strconv.Quote(respID) +
		`,"method":"session/prompt","params":{"sessionId":` + simJSONStr(sessionID) +
		`,"prompt":[{"type":"text","text":` + simJSONStr(text) + `}]}}`)

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
func TestModesMatrixInteractive(t *testing.T) { // HOME-pinned serve leg
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
		t.Fatal("no PreToolUse payload in the marker — the fixture hook never fired " +
			"(discovery-only is not D-13 evidence)")
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

	matrixRegistry.ReportT(t)
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
func TestModesMatrixInteractiveSurfaces(t *testing.T) { //nolint:funlen // HOME-pinned legs; one story
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
		t.Errorf("no provider request carried %q — the fixture command did not expand",
			cmdMarker)
	}

	matrixRegistry.Record(
		modesmatrix.ModeInteractive, modesmatrix.SurfaceCommands, modesmatrix.StatusPass,
		"/matrix-echo expanded to the fixture body before the provider call (received-prompt lens)",
	)

	const skillMarker = "MATRIX-SKILL-BODY invoked: interactive-skill-args"
	if !matrixPromptContains(stub, skillMarker) {
		t.Errorf("no provider request carried %q — the fixture skill did not expand",
			skillMarker)
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

	projBody := "---\ndescription: project-scope collision winner\n---\n" +
		"MATRIX-ECHO-PROJECT-WINS invoked: $ARGUMENTS\n"

	if err := os.WriteFile(projCmd, []byte(projBody), 0o600); err != nil {
		t.Fatalf("write project collision command: %v", err)
	}

	stubB, cliB, sessB, _ := modesMatrixServe(t, collideDir, "PENDING_STUB_URL", []simTurnScript{
		{phases: []simPhase{{text: "collision turn done"}}},
	})

	matrixPromptTurn(t, cliB, sessB, "/matrix-echo collide-args", "mx-col-1")

	const projectWins = "MATRIX-ECHO-PROJECT-WINS invoked: collide-args"

	if !matrixPromptContains(stubB, projectWins) {
		t.Error("the project-tree collision entry did not win — not the locked project-over-plugin precedence")
	}

	if matrixPromptContains(stubB, "MATRIX-ECHO-EXPANSION") {
		t.Error("the plugin-bundled loser still expanded — the collision merged instead of resolving to one winner")
	}

	// Resolution-order observation recorded (probe ordering): project
	// file-command > plugin bundle, per the loader's locked precedence.
	matrixRegistry.Record(
		modesmatrix.ModeInteractive, modesmatrix.SurfaceCommands, modesmatrix.StatusPass,
		"expanded via fixture plugin; collision probe: the project .claude/commands entry wins "+
			"over the plugin bundle (locked precedence, one winner — no merge)",
	)

	matrixRegistry.ReportT(t)
}

// TestModesMatrixInteractiveEmpty (the empty-input row, interactive leg):
// with NO fixture mounted, the typed invocation falls through UNEXPANDED —
// the provider receives the typed text verbatim and the marker file stays
// ABSENT. Absence is the pass condition, never an error.
func TestModesMatrixInteractiveEmpty(t *testing.T) { // HOME-pinned leg
	t.Setenv("HOME", t.TempDir())

	workDir := simulatorWorkDir(t, "PENDING_STUB_URL") // deliberately NO fixture

	stub, cli, sessionID, _ := modesMatrixServe(t, workDir, "PENDING_STUB_URL", []simTurnScript{
		{phases: []simPhase{{text: "empty fixture turn done"}}},
	})

	const typed = "/matrix-echo ghost-args"

	matrixPromptTurn(t, cli, sessionID, typed, "mx-emp-1")

	if !matrixPromptContains(stub, typed) {
		t.Error("no provider request carried the typed invocation verbatim (fallthrough broken)")
	}

	if _, err := os.Stat(modesmatrix.MarkerPath(workDir)); !os.IsNotExist(err) {
		t.Errorf("empty row: marker file exists at %s; want ABSENT", modesmatrix.MarkerPath(workDir))
	}

	t.Log("MODES-MATRIX empty-row interactive: invocation fell through verbatim; marker file ABSENT")
}

// spotCheckPluginEnv names the env var that arms the D-14 real-plugin
// spot-check leg: ASSGUARD_MATRIX_REAL_PLUGIN=<installed plugin root> (the
// directory carrying .claude-plugin/plugin.json). The leg is OPERATOR-
// ENVIRONMENT evidence — CI never runs it (unset → skipped loudly with the
// SPOT-CHECK vocabulary, deliberately distinct from the wake row's
// PRECONDITION-UNMET(22 prefix so the count assertion stays exact).
const spotCheckPluginEnv = "ASSGUARD_MATRIX_REAL_PLUGIN"

// TestModesMatrixRealPluginSpotCheck (D-14): ONE real installed Claude Code
// plugin, mounted READ-ONLY (copied — the source tree is never written)
// into a temp project, driven through the same interactive leg as the
// synthetic fixture. The functional probes: the plugin's OWN skill expands
// through the live chain (received-prompt lens), and its SessionStart hook
// — if it produces stdout — lands in the first request's system blocks (the
// 21 seam: injectHookContext appends hook output to Profile.System).
// Surfaces the plugin does not carry are recorded ABSENT (honest absence,
// never invented); evidence lands in 24-05-SUMMARY.md (plugin NAME and
// functional observations only — never its file contents, T-24-05-04).
func TestModesMatrixRealPluginSpotCheck(t *testing.T) { // HOME-pinned operator-env leg
	pluginRoot := os.Getenv(spotCheckPluginEnv)
	if pluginRoot == "" {
		t.Skipf("SPOT-CHECK-SKIPPED (operator env): set %s=<real plugin root> to run the D-14 leg",
			spotCheckPluginEnv)
	}

	t.Setenv("HOME", t.TempDir())

	workDir := simulatorWorkDir(t, "PENDING_STUB_URL")

	name := modesmatrix.MountRealPlugin(t, workDir, pluginRoot)

	stub, cli, sessionID, _ := modesMatrixServe(t, workDir, "PENDING_STUB_URL", []simTurnScript{
		{phases: []simPhase{{text: "spot-check turn one done"}}},
		{phases: []simPhase{{text: "spot-check turn two done"}}},
	})

	// The plugin's skills surface, functionally: /brainstorming (the
	// superpowers library's flagship skill) must expand through the SAME
	// chain leg the synthetic probe rode.
	matrixPromptTurn(t, cli, sessionID, "/brainstorming spot-check probe", "mx-real-1")

	skillExpanded := matrixPromptContains(stub, "Brainstorming")
	if !skillExpanded {
		t.Error("no provider request carried the real plugin's brainstorming skill body")
	}

	// The hooks surface, functionally observed: SessionStart fired through
	// the real seam at the first turn — its stdout rides the FIRST request's
	// system blocks. The discriminator is CC's documented SessionStart shape
	// (a hookSpecificOutput JSON envelope), NOT the plugin's name: the
	// skills-catalog system block also mentions the plugin's skills and
	// would otherwise false-positive as hook output.
	var hookContext string

	for _, sys := range stub.recordedSystems() {
		if strings.Contains(sys, "hookSpecificOutput") {
			hookContext = sys

			break
		}
	}

	// Commands surface: recorded as the plugin's own layout dictates.
	_, cmdErr := os.Stat(filepath.Join(pluginRoot, "commands"))

	for i, sys := range stub.recordedSystems() {
		head := sys
		if len(head) > 60 {
			head = head[:60]
		}

		t.Logf("SPOT-CHECK system[%d] len=%d head=%q", i, len(sys), head)
	}
	t.Logf("SPOT-CHECK %s: skills=expanded-via-/brainstorming(%v) sessionstart-hook-context=%v commands-surface=%s",
		name, skillExpanded, hookContext != "",
		map[bool]string{true: "present", false: "absent (this plugin ships none)"}[cmdErr == nil])

	if hookContext != "" {
		t.Logf("SPOT-CHECK hook observation: first request system carries the SessionStart hook's output (%d chars)",
			len(hookContext))
	} else {
		t.Log("SPOT-CHECK hook observation: no SessionStart stdout reached the system blocks — " +
			"recorded as observed (the hook's firing itself is turn-safe: the turn completed)")
	}
}
