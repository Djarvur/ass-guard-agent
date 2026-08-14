package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// TestACPServeCommandRegistered verifies the cobra root has an `acp serve`
// subcommand with the expected flags (transport discipline: stdout is framer-
// only; --profile default zcode; --max-concurrent default 6).
func TestACPServeCommandRegistered(t *testing.T) {
	t.Parallel()

	root := newRootCmd()

	var acpCmd *cobra.Command

	for _, c := range root.Commands() {
		if c.Use == "acp" {
			acpCmd = c

			break
		}
	}

	if acpCmd == nil {
		t.Fatal("no `acp` parent command registered on root")
	}

	var serve *cobra.Command

	for _, c := range acpCmd.Commands() {
		if c.Use == "serve" {
			serve = c

			break
		}
	}

	if serve == nil {
		t.Fatal("no `serve` subcommand under `acp`")
	}

	pf := serve.Flags()

	prof, _ := pf.GetString("profile")
	if prof != profileZcode {
		t.Errorf("serve --profile default = %q; want zcode", prof)
	}

	mc, _ := pf.GetInt("max-concurrent")
	if mc != 6 {
		t.Errorf("serve --max-concurrent default = %d; want 6 (RESEARCH §11.1)", mc)
	}
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

	err := runACPServe(ctx, in, &stdout, &stderr, &serveOptions{
		Profile: profileZcode, MaxConcurrent: 6,
		ProfilesDir: repoProfilesDir(t), WorkDir: t.TempDir(),
	})
	if err != nil && !errors.Is(err, io.EOF) {
		t.Logf("runACPServe returned %v (acceptable)", err)
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

	_ = runACPServe(ctx, in, &stdout, &stderr, &serveOptions{
		Profile: profileZcode, MaxConcurrent: 6,
		ProfilesDir: repoProfilesDir(t), WorkDir: t.TempDir(),
	})
	if strings.Contains(stdout.String(), "ass-guard/acp") {
		t.Errorf("stdout contains a log prefix (transport discipline violation): %s", stdout.String())
	}
}

// TestACPServeDoesNotRegressProfileCheck verifies the Phase-1 `profile check`
// subcommand still exists alongside the new `acp serve` subcommand.
func TestACPServeDoesNotRegressProfileCheck(t *testing.T) {
	t.Parallel()

	root := newRootCmd()

	var profileCmd *cobra.Command

	for _, c := range root.Commands() {
		if c.Use == "profile" {
			profileCmd = c

			break
		}
	}

	if profileCmd == nil {
		t.Fatal("Phase-1 `profile` parent command is missing (regression)")
	}

	hasCheck := false

	for _, c := range profileCmd.Commands() {
		if strings.HasPrefix(c.Use, "check") {
			hasCheck = true

			break
		}
	}

	if !hasCheck {
		t.Fatal("Phase-1 `profile check` subcommand is missing (regression)")
	}
}

// repoProfilesDir returns the repo-root profiles/ directory (the test runs from
// cmd/ass-guard/, so the repo root is two levels up).
func repoProfilesDir(t *testing.T) string {
	t.Helper()

	abs, err := filepath.Abs("../../profiles")
	if err != nil {
		t.Fatal(err)
	}

	return abs
}

// --- Phase 8 / 08-04: slash-command expansion wiring (Tests 9-14) ---

// exploreInvocation is the typed-fragment reused across the expansion tests.
const exploreInvocation = "/opsx:explore fix-it"

// writeOpsxCommandFixtures installs the opsx command fixtures into dir's
// project `.claude/` (the layout `openspec init --tools claude` installs).
func writeOpsxCommandFixtures(t *testing.T, dir string) {
	t.Helper()

	bodies := map[string]string{
		"explore": "---\ndescription: explore the change\n---\n" +
			"Explore the change: $ARGUMENTS\n\n- read the codebase\n- compare options\n",
		"propose": "---\ndescription: propose a change\n---\n" +
			"Propose the change named $1 with all context considered.\n",
		"apply": "---\ndescription: apply the tasks\n---\n" +
			"Apply the change: $ARGUMENTS\n\nWork the tasks.md checklist to completion.\n",
	}

	for name, body := range bodies {
		p := filepath.Join(dir, ".claude", "commands", "opsx", name+".md")

		err := os.MkdirAll(filepath.Dir(p), 0o750)
		if err != nil {
			t.Fatalf("mkdir fixture dir: %v", err)
		}

		err = os.WriteFile(p, []byte(body), 0o600)
		if err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
}

// newExpansionRunner builds a sessionTurnRunner over a temp workDir carrying
// the opsx fixtures, with the command registry loaded (engineOn controls
// setupEngine). Returns the runner + its scripted provider.
func newExpansionRunner(
	t *testing.T, engineOn bool, script ...scriptedResp,
) (*sessionTurnRunner, *scriptedACPProvider) {
	t.Helper()

	bus := event.NewBus()
	prov := &scriptedACPProvider{}
	prov.queue(script...)

	dir := t.TempDir()
	writeOpsxCommandFixtures(t, dir)

	r := &sessionTurnRunner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      4,
		makeProvider: func() provider.Provider { return prov },
	}

	if engineOn {
		err := r.setupEngine()
		if err != nil {
			t.Fatalf("setupEngine: %v", err)
		}
	}

	r.loadCommandRegistry()

	return r, prov
}

// lastUserMessageText returns the text of the LAST user_message line in the
// session's transcript (the assertion lens for expansion — D-02: the expanded
// body IS the user message the model saw).
func lastUserMessageText(t *testing.T, r *sessionTurnRunner, sessionID string) string {
	t.Helper()

	lines, err := r.sessions[sessionID].Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	for i := len(lines) - 1; i >= 0; i-- { //nolint:modernize // mirror of Manager.ReadLastBoundary convention
		if lines[i].Type != session.TypeUserMessage {
			continue
		}

		var blocks []session.ContentBlock

		err = json.Unmarshal(lines[i].Content, &blocks)
		if err != nil {
			t.Fatalf("unmarshal user content: %v", err)
		}

		var sb strings.Builder

		for _, b := range blocks {
			sb.WriteString(b.Text)
		}

		return sb.String()
	}

	t.Fatal("no user_message line in transcript")

	return ""
}

// transcriptLinesOfType returns transcript lines whose type matches.
func transcriptLinesOfType(t *testing.T, r *sessionTurnRunner, sessionID, lineType string) []session.Line {
	t.Helper()

	lines, err := r.sessions[sessionID].Manager.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var out []session.Line

	for i := range lines {
		if lines[i].Type == lineType {
			out = append(out, lines[i])
		}
	}

	return out
}

// TestExpansion_EngineOffPathExpanded (Test 9): with the engine DISABLED, a
// typed /opsx:explore invocation reaches sess.Prompt as the EXPANDED body —
// the transcript's user_message line holds the command body, not the typed
// text.
func TestExpansion_EngineOffPathExpanded(t *testing.T) {
	t.Parallel()
	r, _ := newExpansionRunner(t, false,
		scriptedResp{text: "explored; no handoff signal", finish: stopEndTurn})

	stop, err := r.Run(context.Background(), "sess-x-off", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: exploreInvocation}})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}

	_ = stop

	got := lastUserMessageText(t, r, "sess-x-off")
	if !strings.Contains(got, "Explore the change: fix-it") {
		t.Errorf("user_message = %q; want the EXPANDED body (args substituted)", got)
	}

	if strings.Contains(got, "/opsx:explore") {
		t.Errorf("user_message = %q; the typed command leaked as the message", got)
	}
}

// TestExpansion_EngineOnPathExpanded (Test 10): same on the engine-on path —
// the first prompt through engine.Observe is already expanded.
func TestExpansion_EngineOnPathExpanded(t *testing.T) {
	t.Parallel()
	r, _ := newExpansionRunner(t, true,
		scriptedResp{text: "explored; no handoff signal", finish: stopEndTurn})

	_, err := r.Run(context.Background(), "sess-x-on", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: exploreInvocation}})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}

	got := lastUserMessageText(t, r, "sess-x-on")
	if !strings.Contains(got, "Explore the change: fix-it") {
		t.Errorf("user_message = %q; want the EXPANDED body on the engine path", got)
	}
}

// TestInjectionExpansion_AdapterExpandsInjections (Test 11): engine
// continue-injections re-enter through engineTurnRunnerAdapter.Run — an
// injected "/opsx:propose name-x" prompt carries the EXPANDED propose body
// into sess.Prompt, not the raw slash text.
func TestInjectionExpansion_AdapterExpandsInjections(t *testing.T) {
	t.Parallel()
	r, _ := newExpansionRunner(t, true)

	sess := r.sessionFor(context.Background(), "sess-inj")

	adapter := &engineTurnRunnerAdapter{sess: sess, mgr: sess.Manager, r: r}

	_, err := adapter.Run(context.Background(),
		[]session.ContentBlock{{Type: blockText, Text: "/opsx:propose name-x"}})
	if err != nil {
		t.Fatalf("adapter.Run err = %v", err)
	}

	got := lastUserMessageText(t, r, "sess-inj")
	if !strings.Contains(got, "Propose the change named name-x") {
		t.Errorf("injected user_message = %q; want the EXPANDED propose body", got)
	}

	if strings.Contains(got, "/opsx:propose") {
		t.Errorf("injected user_message = %q; raw slash text leaked", got)
	}
}

// TestExpansion_UnknownCommandFallsThrough (Test 12): /notaregistrykey is
// ordinary text — blocks reach Prompt UNCHANGED, and NO provenance line is
// written (structural safety: no match, nothing happens).
func TestExpansion_UnknownCommandFallsThrough(t *testing.T) {
	t.Parallel()
	r, _ := newExpansionRunner(t, false,
		scriptedResp{text: "ok", finish: stopEndTurn})

	_, err := r.Run(context.Background(), "sess-x-unknown", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: "/notaregistrykey hello"}})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}

	got := lastUserMessageText(t, r, "sess-x-unknown")
	if got != "/notaregistrykey hello" {
		t.Errorf("user_message = %q; want unchanged plain text", got)
	}

	if prov := transcriptLinesOfType(t, r, "sess-x-unknown", "command_provenance"); len(prov) != 0 {
		t.Errorf("provenance lines = %d; want 0 for a registry miss", len(prov))
	}
}

// TestExpansion_RegistryLoadFailureDegrades (Test 13): a registry load error
// (unreadable .claude/skills — a file where a dir belongs) never breaks turns:
// the turn proceeds WITHOUT expansion, plain text reaches the model.
func TestExpansion_RegistryLoadFailureDegrades(t *testing.T) {
	t.Parallel()
	r, _ := newExpansionRunner(t, false,
		scriptedResp{text: "ok", finish: stopEndTurn})

	// Break the tree AFTER the initial load: a FILE at .claude/skills makes
	// the next Discover fail (ReadDir ENOTDIR).
	err := os.WriteFile(filepath.Join(r.workDir, ".claude", "skills"), []byte("not a dir"), 0o600)
	if err != nil {
		t.Fatalf("plant breaking file: %v", err)
	}

	r.loadCommandRegistry() // reload fails — logged, non-fatal

	_, err = r.Run(context.Background(), "sess-x-deg", &noopEmitter{},
		[]acp.ContentBlock{{Type: blockText, Text: exploreInvocation}})
	if err != nil {
		t.Fatalf("Run err = %v; want the turn to proceed (degradation)", err)
	}

	got := lastUserMessageText(t, r, "sess-x-deg")
	if got != "/opsx:explore fix-it" {
		t.Errorf("user_message = %q; want the RAW text (no expansion on degraded registry)", got)
	}
}

// TestExpansion_NoEditorEcho (Test 14, D-01): during an expanded turn the only
// session/update traffic is AgentMessageChunk forwarding of ASSISTANT text —
// the expanded body is NEVER echoed back to the editor.
func TestExpansion_NoEditorEcho(t *testing.T) {
	t.Parallel()
	r, _ := newExpansionRunner(t, true,
		scriptedResp{text: "assistant reply text only", finish: stopEndTurn})

	emit := &noopEmitter{}

	_, err := r.Run(context.Background(), "sess-x-echo", emit,
		[]acp.ContentBlock{{Type: blockText, Text: exploreInvocation}})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}

	for _, chunk := range emit.chunks {
		if strings.Contains(chunk, "Explore the change") {
			t.Errorf("session/update echoed the expanded body: %q", chunk)
		}
	}
}
