package acpserve //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/session"

	"gopkg.in/yaml.v3"
)

// The 17-05 permissions/elicitation E2E battery (ACP-01 + ACP-02 whole-phase
// story): the 16-06 Zed-client simulator fixtures drive the REAL
// acpserve.Run composition end-to-end through the five wire-level scenarios
// the phase must own — the gated permission dialog round-trip (ask once,
// persist allow_always, execute, never ask again), the elicitation form on a
// capable client, the -32601 probe-degraded plain-text fallback, turn death
// mid-dialog (cancelled-normal + the $/cancel_request cascade, no orphans),
// and the dialog-free ungated default. Assertions observe the pipe frames,
// the on-disk permissions.yaml, and the session transcript — the deterministic
// half of ROADMAP criteria 1 and 3 (the live-rendering legs are the operator
// checkpoint, recorded in the WINDOWS ledger per the 15-07 pattern).
//
// Determinism: fixed script per scenario, its own serve instance + temp
// project (mode flips persist to the project layer, so scenarios never share
// trust state), guard timeouts on every wait, results-based predicates (the
// 17-01..17-03 flake-family discipline: no millisecond windows).

const (
	// permStubPlaceholder seeds the temp config before the stub URL exists.
	permStubPlaceholder = "PENDING_PERM_STUB_URL"

	// permE2ETimeout bounds every scenario await (per-frame reads stay
	// bounded by simGuardTimeout inside the sim client).
	permE2ETimeout = 30 * time.Second

	// permToolWrite is the mutating tool the permission scenarios gate.
	permToolWrite = "Write"

	// permToolAsk is the model-authored question tool the elicitation
	// scenarios suspend on.
	permToolAsk = "AskUserQuestion"

	// permAskQuestion/permAskAnswer/permAskAlt are the scripted question's
	// text and choice labels (single-choice → oneOf titled consts).
	permAskQuestion = "Which approach?"
	permAskAnswer   = "ship it"
	permAskAlt      = "wait"

	// permWritten is the content the scripted Write calls lay down.
	permWritten = "written by the permissions e2e battery\n"

	// permFile is the Write target (relative to the temp project).
	permFile = "out.md"
)

// permWriteInput builds the scripted Write call's input for the workdir.
func permWriteInput(workDir string) string {
	raw, err := json.Marshal(map[string]string{
		"file_path": filepath.Join(workDir, permFile),
		"content":   permWritten,
	})
	if err != nil {
		panic(err) // a two-key string map — cannot fail
	}

	return string(raw)
}

// permAskInput builds the scripted AskUserQuestion call's input (the captured
// {questions:[…]} shape the executor parses).
func permAskInput() string {
	raw, err := json.Marshal(map[string]any{
		"questions": []map[string]any{{
			"question": permAskQuestion,
			"header":   "Approach",
			"options": []map[string]string{
				{"label": permAskAnswer, "description": "go now"},
				{"label": permAskAlt, "description": "hold"},
			},
			"multiSelect": false,
		}},
	})
	if err != nil {
		panic(err) // a static map — cannot fail
	}

	return string(raw)
}

// permStartServe boots the real Run composition over pipes with a scripted
// provider stub and a temp project; script(workDir) builds the turn script
// (the Write target paths need the workdir). Every resource is cleaned up.
//
// The project dir's cleanup is best-effort: with the engine-backed executor
// enabled, an async flush can land a file just after the serve exits, and a
// strict TempDir remove would then fail the TEST on macOS (unlinkat race) —
// the leftover dir is not an assertion.
func permStartServe(
	t *testing.T, script func(workDir string) []simTurnScript,
) (*simClient, *simStub, string) {
	t.Helper()

	// os.MkdirTemp, not t.TempDir: the TempDir cleanup ASSERTS on removal and
	// fails the test on macOS unlinkat races with late async flushes — a
	// best-effort remove is deliberate here (the dir is not an assertion).
	//nolint:usetesting // see above
	workDir, err := os.MkdirTemp("", "perm-e2e-")
	if err != nil {
		t.Fatalf("temp project dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	// The zero-config project layer (the simulatorWorkDir shape): a real
	// .ass-guard/config.yaml aiming the anthropic provider at the stub, with
	// the URL swapped in after the stub exists.
	permWriteProjectConfig(t, workDir, permStubPlaceholder)

	stub := newSimStub(script(workDir))
	t.Cleanup(stub.srv.Close)

	simRepointConfig(t, workDir, permStubPlaceholder, stub.srv.URL)

	srvInR, srvInW := io.Pipe()
	cliR, cliOutW := io.Pipe()
	stderr := &syncBuffer{}

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)

	go func() {
		serveDone <- Run(ctx, srvInR, cliOutW, stderr, &Options{
			Profile: profileZcode, MaxConcurrent: 2,
			// The permission scenarios assert the gated call REALLY executed
			// (the Write target on disk) — the engine-backed executor is
			// required; without it every non-mcp call lands the canned
			// engine-disabled stub result.
			EngineEnabled: true,
			ProfilesDir:   repoProfilesDir(t), WorkDir: workDir,
		})
	}()

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("serve stderr:\n%s", stderr.String())
		}

		cancel()

		_ = srvInW.Close()
		_ = cliOutW.Close()
		_ = srvInR.Close()
		_ = cliR.Close()

		select {
		case <-serveDone:
		case <-time.After(simGuardTimeout):
			t.Errorf("serve did not exit")
		}
	})

	return newSimClient(t, cliR, srvInW), stub, workDir
}

// permInitialize performs the handshake. advertiseForm mirrors the client's
// elicitation.form advertisement: without it the initialize-time capability
// probe arrives BEFORE the response and is answered -32601 (the D-13/D-18
// degrade). Returns the number of probe frames answered.
func permInitialize(t *testing.T, cli *simClient, advertiseForm bool) int {
	t.Helper()

	caps := ""
	if advertiseForm {
		caps = `"clientCapabilities":{"elicitation":{"form":{}}},`
	}

	cli.sendf(`{"jsonrpc":"2.0","id":"e2e-init","method":"initialize","params":{"protocolVersion":1,` +
		`"clientInfo":{"name":"perm-e2e","version":"1"},` + caps + `"mcpServers":[]}}`)

	probes := 0

	for {
		m := cli.next()

		switch {
		case m.Method == acp.MethodElicitationCreate:
			probes++

			cli.sendf(`{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"method not found"}}`, string(m.ID))
		case isResponseID(m, `"e2e-init"`):
			return probes
		}
	}
}

// permSessionNew opens the scenario's session (cwd = the temp project).
func permSessionNew(t *testing.T, cli *simClient, workDir string) string {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"e2e-new","method":"session/new","params":{"cwd":` +
		simJSONStr(workDir) + `,"mcpServers":[]}}`)

	m := cli.nextResponse(`"e2e-new"`)

	var resp struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	err := json.Unmarshal(m.Result, &resp)
	if err != nil {
		t.Fatalf("decode session/new result: %v (%s)", err, string(m.Result))
	}

	if resp.SessionID == "" {
		t.Fatal("session/new returned no sessionId")
	}

	return resp.SessionID
}

// permSetMode flips permissions.mode through the wire (the real 17-02
// handler: persist-then-apply). The racing config_option_update is tolerated.
func permSetMode(t *testing.T, cli *simClient, sessionID, mode string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"e2e-mode","method":"session/set_config_option","params":{"sessionId":` +
		simJSONStr(sessionID) + `,"configId":` + simJSONStr(optPermissionsMode) + `,"value":` + simJSONStr(mode) + `}}`)

	for {
		m := cli.next()

		switch {
		case m.Method == simMethodSessionUpdate:
			// The out-of-band refresh rides alongside the response.
		case isResponseID(m, `"e2e-mode"`):
			if m.Error != nil {
				t.Fatalf("set_config_option(%s=%s) failed: %d %s",
					optPermissionsMode, mode, m.Error.Code, m.Error.Message)
			}

			return
		}
	}
}

// permPrompt sends one user prompt turn.
func permPrompt(t *testing.T, cli *simClient, sessionID, id, text string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":` + simJSONStr(id) + `,"method":"session/prompt","params":{"sessionId":` +
		simJSONStr(sessionID) + `,"prompt":[{"type":"text","text":` + simJSONStr(text) + `}]}}`)
}

// permElicFrame is the decoded elicitation/create frame plus its wire id
// (ID is the JSON-RPC envelope id — not part of the params payload).
type permElicFrame struct {
	ID string `json:"-"`

	Message    string                `json:"message"`
	Mode       string                `json:"mode"`
	SessionID  string                `json:"sessionId"`       //nolint:tagliatelle // ACP wire field
	ToolCallID string                `json:"toolCallId"`      //nolint:tagliatelle // ACP wire field
	Schema     acp.ElicitationSchema `json:"requestedSchema"` //nolint:tagliatelle // ACP wire field
}

// permStory is the classified frame story of one scenario leg.
type permStory struct {
	permFrames []acp.RequestPermissionFrame
	permIDs    []string
	elic       []permElicFrame
	elicIDs    []string
	cancels    int
	chunks     []string
	responses  []string
}

// responded reports whether the response carrying id arrived.
func (s *permStory) responded(id string) bool { return s.arrived(`"` + id + `"`) }

// arrived reports whether the response carrying the raw (quoted) id arrived.
func (s *permStory) arrived(rawID string) bool { return slices.Contains(s.responses, rawID) }

// hasChunk reports whether a streamed chunk carries text exactly.
func (s *permStory) hasChunk(text string) bool { return slices.Contains(s.chunks, text) }

// permStep reads ONE frame into the story.
func permStep(t *testing.T, cli *simClient, st *permStory) {
	t.Helper()

	m := cli.next()

	switch {
	case m.Method == acp.MethodRequestPermission:
		var f acp.RequestPermissionFrame

		err := json.Unmarshal(m.Params, &f)
		if err != nil {
			t.Fatalf("decode request_permission: %v (%s)", err, string(m.Params))
		}

		st.permFrames = append(st.permFrames, f)
		st.permIDs = append(st.permIDs, string(m.ID))
	case m.Method == acp.MethodElicitationCreate:
		var f permElicFrame

		err := json.Unmarshal(m.Params, &f)
		if err != nil {
			t.Fatalf("decode elicitation/create: %v (%s)", err, string(m.Params))
		}

		f.ID = string(m.ID)
		st.elic = append(st.elic, f)
		st.elicIDs = append(st.elicIDs, string(m.ID))
	case m.Method == simMethodCancelRequest:
		st.cancels++
	case m.Method == simMethodSessionUpdate:
		if txt, ok := permChunkText(t, m); ok {
			st.chunks = append(st.chunks, txt)
		}
	case m.ID != nil && m.Method == "":
		st.responses = append(st.responses, string(m.ID))
	}
}

// permChunkText extracts an agent_message_chunk's text (ok=false otherwise).
func permChunkText(t *testing.T, m *acp.Message) (string, bool) {
	t.Helper()

	var params struct {
		Update struct {
			Kind    string            `json:"sessionUpdate"` //nolint:tagliatelle // ACP wire field
			Content *acp.ContentBlock `json:"content"`
		} `json:"update"`
	}

	err := json.Unmarshal(m.Params, &params)
	if err != nil {
		t.Fatalf("decode session/update: %v (%s)", err, string(m.Params))
	}

	if params.Update.Kind != simKindChunk || params.Update.Content == nil {
		return "", false
	}

	return params.Update.Content.Text, true
}

// permAwait reads frames until pred holds; every wait is deadline-bounded.
func permAwait(t *testing.T, cli *simClient, st *permStory, pred func(*permStory) bool) {
	t.Helper()

	deadline := time.Now().Add(permE2ETimeout)

	for !pred(st) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out awaiting a scenario condition (perm=%d elic=%d cancels=%d chunks=%d responses=%d)",
				len(st.permFrames), len(st.elic), st.cancels, len(st.chunks), len(st.responses))
		}

		permStep(t, cli, st)
	}
}

// permAnswerSelected answers a request_permission frame with a selected option.
func permAnswerSelected(t *testing.T, cli *simClient, rawID, optionID string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":` + rawID + `,"result":{"outcome":"selected","optionId":` +
		simJSONStr(optionID) + `}}`)
}

// permAnswerAccept answers an elicitation/create frame with an accept whose
// content answers the single q1 property.
func permAnswerAccept(t *testing.T, cli *simClient, rawID, value string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":` + rawID + `,"result":{"action":"accept","content":{"q1":` +
		simJSONStr(value) + `}}}`)
}

// permAssertNoAskFrames proves the bounded drain window stays free of orphaned
// ask frames (the D-13 no-orphan assertion).
func permAssertNoAskFrames(t *testing.T, cli *simClient) {
	t.Helper()

	deadline := time.Now().Add(300 * time.Millisecond)

	for time.Now().Before(deadline) {
		select {
		case m, ok := <-cli.frames:
			if !ok {
				return
			}

			if m.Method == acp.MethodRequestPermission || m.Method == acp.MethodElicitationCreate {
				t.Fatalf("orphaned ask frame after the drain: %s %s", m.Method, string(m.Params))
			}
		case <-time.After(time.Until(deadline)):
		}
	}
}

// permResultText extracts the human-readable text of one transcript tool
// result (the append path wraps string outputs in the {"output": …} form).
func permResultText(r permToolResult) string {
	var wrap struct {
		Output string `json:"output"`
	}

	if json.Unmarshal([]byte(r.Output), &wrap) == nil && wrap.Output != "" {
		return wrap.Output
	}

	var s string

	if json.Unmarshal([]byte(r.Output), &s) == nil {
		return s
	}

	return r.Output
}

// permYaml is the on-disk permissions.yaml envelope view.
type permYaml struct {
	Deny  []string `yaml:"deny"`
	Ask   []string `yaml:"ask"`
	Allow []string `yaml:"allow"`
}

// permReadYaml loads the project's permissions.yaml (and pins the 0600
// artifact-family permission).
func permReadYaml(t *testing.T, workDir string) permYaml {
	t.Helper()

	path := filepath.Join(workDir, ".ass-guard", "permissions.yaml")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read permissions.yaml: %v", err)
	}

	info, serr := os.Stat(path)
	if serr != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("permissions.yaml mode = %v; want 0600", info.Mode().Perm())
	}

	var env permYaml

	uerr := yaml.Unmarshal(raw, &env)
	if uerr != nil {
		t.Fatalf("parse permissions.yaml: %v", uerr)
	}

	return env
}

// permToolResult is one transcript tool_result line for a call.
type permToolResult struct {
	IsError bool
	Output  string
}

// permWaitResults polls the session transcript until the call has n tool
// results (the append lands before the resumed turn's provider round, but the
// poll keeps the assertion independent of flush timing).
func permWaitResults(t *testing.T, workDir, sessionID, callID string, n int) []permToolResult {
	t.Helper()

	path := filepath.Join(workDir, ".ass-guard", "transcript_"+sessionID+".jsonl")
	deadline := time.Now().Add(permE2ETimeout)

	var found []permToolResult

	for {
		raw, err := os.ReadFile(path)
		if err == nil {
			found = nil

			for line := range strings.SplitSeq(string(raw), "\n") {
				if !strings.Contains(line, `"tool_result"`) {
					continue
				}

				var ln struct {
					Type       string          `json:"type"`
					ToolCallID string          `json:"toolCallID"` //nolint:tagliatelle // on-disk form
					Output     json.RawMessage `json:"output"`
					IsError    bool            `json:"isError"` //nolint:tagliatelle // on-disk form
				}

				if json.Unmarshal([]byte(line), &ln) != nil {
					continue
				}

				if ln.Type == "tool_result" && ln.ToolCallID == callID {
					found = append(found, permToolResult{IsError: ln.IsError, Output: string(ln.Output)})
				}
			}
		}

		if len(found) >= n {
			return found
		}

		if time.Now().After(deadline) {
			t.Fatalf("transcript never carried %d tool_result line(s) for %s", n, callID)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// permWaitFile polls until the Write target exists with the scripted content.
func permWaitFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(permE2ETimeout)

	for {
		raw, err := os.ReadFile(path)
		if err == nil && string(raw) == permWritten {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("gated call never executed: %s missing or wrong content (err=%v)", path, err)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// permWriteProjectConfig lays the temp project's zero-config floor: the
// .ass-guard dir (0750) plus a config.yaml pointing the anthropic provider at
// stubURL (0600 — the URL placeholder is swapped later by simRepointConfig).
func permWriteProjectConfig(t *testing.T, workDir, stubURL string) {
	t.Helper()

	assDir := filepath.Join(workDir, ".ass-guard")

	merr := os.MkdirAll(assDir, 0o750)
	if merr != nil {
		t.Fatalf("mkdir .ass-guard: %v", merr)
	}

	config := `providers:` + "\n  anthropic:\n    base_url: " +
		strconv.Quote(stubURL) + "\n    api_key: \"sk-simulator-canary\"\n"

	werr := os.WriteFile(filepath.Join(assDir, "config.yaml"), []byte(config), 0o600)
	if werr != nil {
		t.Fatalf("write config.yaml: %v", werr)
	}
}

// TestPermissionsE2E is the phase's whole ask-surface story in five
// deterministic scenarios; each stage is one sentence of it:
//
//  1. the gated permission dialog: ask once, persist allow_always, execute,
//     never ask the identical call again
//  2. the elicitation form on a capable client: form → accept → answered form
//  3. the -32601 probe-degraded client: the plain-text fallback, byte-parity
//     with the v1.1 path, and the turn still completes
//  4. turn death mid-dialog: cancelled-normal + one cascade, zero orphans
//  5. the ungated default: a mutating call with zero permission frames
func TestPermissionsE2E(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "") // the config literal key wins (deterministic provider construction)

	// Sequential stages (the 16-06 simulator shape): each scenario boots its
	// own serve + temp project — parallel runs would multiply serve load for
	// no coverage gain (the results-based-wait flake discipline).
	t.Logf("stage 1: the gated permission dialog round-trip")
	permScenarioHappyPath(t)

	t.Logf("stage 2: the elicitation form on a capable client")
	permScenarioElicitationCapable(t)

	t.Logf("stage 3: the -32601 degraded plain-text fallback")
	permScenarioElicitationDegraded(t)

	t.Logf("stage 4: turn death mid-dialog")
	permScenarioTurnDeath(t)

	t.Logf("stage 5: the ungated default")
	permScenarioUngatedDefault(t)
}

// permScenarioHappyPath: gated session (mode set THROUGH the wire), one
// mutating call → exactly one request_permission frame with the four
// canonical options → allow_always → the rule lands on disk BEFORE the gated
// call executes → the resumed turn completes → the identical call runs with
// zero further permission frames.
func permScenarioHappyPath(t *testing.T) { //nolint:funlen // one scenario, one readable story
	t.Helper()

	cli, _, workDir := permStartServe(t, func(dir string) []simTurnScript {
		return []simTurnScript{
			{phases: []simPhase{{callID: "call_w1", name: permToolWrite, input: permWriteInput(dir)}}},
			{phases: []simPhase{{text: "done writing."}}},
			{phases: []simPhase{{callID: "call_w2", name: permToolWrite, input: permWriteInput(dir)}}},
			{phases: []simPhase{{text: "done again."}}},
		}
	})

	if probes := permInitialize(t, cli, true); probes != 0 {
		t.Fatalf("probe frames = %d; want 0 (form advertised, no probe)", probes)
	}

	sid := permSessionNew(t, cli, workDir)
	permSetMode(t, cli, sid, "gated")

	var st permStory

	permPrompt(t, cli, sid, "e2e-p1", "write the file")
	permAwait(t, cli, &st, func(s *permStory) bool {
		return s.responded("e2e-p1") && len(s.permFrames) == 1
	})

	f := st.permFrames[0]
	if f.SessionID != sid {
		t.Errorf("frame sessionId = %q; want %q", f.SessionID, sid)
	}

	if f.ToolCall.ToolCallID != "call_w1" {
		t.Errorf("dialog toolCall.toolCallId = %q; want call_w1", f.ToolCall.ToolCallID)
	}

	kinds := make([]string, 0, len(f.Options))
	for _, o := range f.Options {
		kinds = append(kinds, o.Kind)
	}

	wantKinds := []string{
		acp.PermOptionAllowOnce, acp.PermOptionAllowAlways,
		acp.PermOptionRejectOnce, acp.PermOptionRejectAlways,
	}
	if strings.Join(kinds, ",") != strings.Join(wantKinds, ",") {
		t.Errorf("dialog option kinds = %v; want %v (the complete four-option set)", kinds, wantKinds)
	}

	permAnswerSelected(t, cli, st.permIDs[0], acp.PermOptionAllowAlways)

	// The resumed turn completes (round 2 streams the closing chunk).
	permAwait(t, cli, &st, func(s *permStory) bool { return s.hasChunk("done writing.") })

	// The gated call EXECUTED, and the allow rule landed on disk (0600).
	permWaitFile(t, filepath.Join(workDir, permFile))

	env := permReadYaml(t, workDir)
	if len(env.Allow) != 1 || env.Allow[0] != permToolWrite {
		t.Errorf("permissions.yaml allow = %v; want exactly [%s] (the tool-x-project entry)", env.Allow, permToolWrite)
	}

	if len(env.Deny) != 0 || len(env.Ask) != 0 {
		t.Errorf("permissions.yaml deny/ask = %v/%v; want empty", env.Deny, env.Ask)
	}

	// The second IDENTICAL call: allowed by the persisted rule — zero dialogs.
	before := len(st.permFrames)

	permPrompt(t, cli, sid, "e2e-p2", "write it again")
	permAwait(t, cli, &st, func(s *permStory) bool {
		return s.responded("e2e-p2") && s.hasChunk("done again.")
	})

	if extra := len(st.permFrames) - before; extra != 0 {
		t.Errorf("second identical call produced %d permission frame(s); want 0 (the allow rule answers)", extra)
	}
}

// permScenarioElicitationCapable: a question ask on a form-capable client →
// one elicitation/create frame in the v1 form shape (D-08 mapping) → accept →
// the answered tool result lands in the CAPTURED form.
func permScenarioElicitationCapable(t *testing.T) {
	t.Helper()

	cli, _, workDir := permStartServe(t, func(dir string) []simTurnScript {
		return []simTurnScript{
			{phases: []simPhase{{callID: "call_q1", name: permToolAsk, input: permAskInput()}}},
			{phases: []simPhase{{text: "answered up."}}},
		}
	})

	if probes := permInitialize(t, cli, true); probes != 0 {
		t.Fatalf("probe frames = %d; want 0 (form advertised)", probes)
	}

	sid := permSessionNew(t, cli, workDir)

	var st permStory

	permPrompt(t, cli, sid, "e2e-p1", "ask me")
	permAwait(t, cli, &st, func(s *permStory) bool {
		return s.responded("e2e-p1") && len(s.elic) == 1
	})

	ef := st.elic[0]
	permAssertAskForm(t, &ef, sid)

	permAnswerAccept(t, cli, ef.ID, permAskAnswer)
	permAwait(t, cli, &st, func(s *permStory) bool { return s.hasChunk("answered up.") })

	results := permWaitResults(t, workDir, sid, "call_q1", 1)
	if results[0].IsError {
		t.Errorf("answered result is an error: %s", results[0].Output)
	}

	// The CAPTURED answered form (the model's view of an answered ask).
	want := "User has answered your questions: " +
		fmt.Sprintf("%q=%q", permAskQuestion, permAskAnswer) +
		". You can now continue with the user's answers in mind."
	if got := permResultText(results[0]); got != want {
		t.Errorf("answered form = %s; want %s", got, want)
	}

	if len(st.permFrames) != 0 {
		t.Errorf("permission frames = %d; want 0 (a question ask is not a permission ask)", len(st.permFrames))
	}
}

// permAssertAskForm pins the D-08 form shape of one capable-client elicitation
// frame: form mode, session scope, the question in the message, and the q1
// string property carrying the oneOf titled consts.
func permAssertAskForm(t *testing.T, ef *permElicFrame, sid string) {
	t.Helper()

	if ef.Mode != acp.ElicitationModeForm {
		t.Errorf("elicitation mode = %q; want %q", ef.Mode, acp.ElicitationModeForm)
	}

	if ef.ToolCallID != "call_q1" {
		t.Errorf("elicitation toolCallId = %q; want call_q1", ef.ToolCallID)
	}

	if ef.SessionID != sid {
		t.Errorf("elicitation sessionId = %q; want %q", ef.SessionID, sid)
	}

	if !strings.Contains(ef.Message, permAskQuestion) {
		t.Errorf("elicitation message %q does not carry the question text", ef.Message)
	}

	if len(ef.Schema.Required) != 1 || ef.Schema.Required[0] != "q1" {
		t.Errorf("schema required = %v; want [q1]", ef.Schema.Required)
	}

	var prop struct {
		Type  string `json:"type"`
		OneOf []struct {
			Const string `json:"const"`
		} `json:"oneOf"` //nolint:tagliatelle // ACP wire field
	}

	err := json.Unmarshal(ef.Schema.Properties["q1"], &prop)
	if err != nil {
		t.Fatalf("decode q1 property: %v (%s)", err, string(ef.Schema.Properties["q1"]))
	}

	if prop.Type != propTypeString || len(prop.OneOf) != 2 ||
		prop.OneOf[0].Const != permAskAnswer || prop.OneOf[1].Const != permAskAlt {
		t.Errorf("q1 property = %+v; want a string oneOf [%s %s]", prop, permAskAnswer, permAskAlt)
	}
}

// permScenarioElicitationDegraded: the client does not advertise the form; the
// initialize probe is answered -32601; the SAME ask renders as the plain-text
// AgentMessageChunk fallback byte-identical to the v1.1 path, and the turn
// completes through the reply routing.
func permScenarioElicitationDegraded(t *testing.T) {
	t.Helper()

	cli, _, workDir := permStartServe(t, func(dir string) []simTurnScript {
		return []simTurnScript{
			{phases: []simPhase{{callID: "call_q2", name: permToolAsk, input: permAskInput()}}},
			{phases: []simPhase{{text: "fell back ok."}}},
		}
	})

	if probes := permInitialize(t, cli, false); probes != 1 {
		t.Fatalf("probe frames = %d; want exactly 1 (the unadvertised-form probe)", probes)
	}

	sid := permSessionNew(t, cli, workDir)

	var st permStory

	permPrompt(t, cli, sid, "e2e-p1", "ask me")
	permAwait(t, cli, &st, func(s *permStory) bool {
		return s.responded("e2e-p1") && len(s.chunks) >= 1
	})

	if len(st.elic) != 0 {
		t.Fatalf("the degraded ask issued %d elicitation frame(s); want 0 (plain-text fallback only)", len(st.elic))
	}

	// Byte-parity with the v1.1 path: the fallback chunk IS RenderAskSurface.
	var args struct {
		Questions []session.AskQuestion `json:"questions"`
	}

	uerr := json.Unmarshal([]byte(permAskInput()), &args)
	if uerr != nil {
		t.Fatalf("decode ask input: %v", uerr)
	}

	want := coreexec.RenderAskSurface(args.Questions)
	if st.chunks[0] != want {
		t.Errorf("fallback chunk drifted from the v1.1 path:\n got %q\nwant %q", st.chunks[0], want)
	}

	// The v1.1 answer routing: the reply prompt resolves the pending ask.
	permPrompt(t, cli, sid, "e2e-p2", permAskAnswer)
	permAwait(t, cli, &st, func(s *permStory) bool {
		return s.responded("e2e-p2") && s.hasChunk("fell back ok.")
	})

	results := permWaitResults(t, workDir, sid, "call_q2", 1)
	if results[0].IsError {
		t.Errorf("answered result is an error: %s", results[0].Output)
	}

	if got := permResultText(results[0]); !strings.Contains(got, permAskQuestion+`"="`+permAskAnswer) {
		t.Errorf("answered form = %s; want the captured pairing of the question with %q", got, permAskAnswer)
	}
}

// permScenarioTurnDeath: the dialog is open, the client cancels the session →
// exactly one $/cancel_request cascade, the transcript carries the
// cancelled-NORMAL result (never an error), the gated call never executes,
// and no orphaned ask frames follow.
func permScenarioTurnDeath(t *testing.T) {
	t.Helper()

	cli, _, workDir := permStartServe(t, func(dir string) []simTurnScript {
		return []simTurnScript{
			{phases: []simPhase{{callID: "call_wd", name: permToolWrite, input: permWriteInput(dir)}}},
			{phases: []simPhase{{text: "wound down."}}},
		}
	})

	if probes := permInitialize(t, cli, true); probes != 0 {
		t.Fatalf("probe frames = %d; want 0", probes)
	}

	sid := permSessionNew(t, cli, workDir)
	permSetMode(t, cli, sid, "gated")

	var st permStory

	permPrompt(t, cli, sid, "e2e-p1", "write the file")
	permAwait(t, cli, &st, func(s *permStory) bool {
		return s.responded("e2e-p1") && len(s.permFrames) == 1
	})

	// Turn death mid-dialog: the operator cancels instead of answering.
	cli.sendf(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":` + simJSONStr(sid) + `}}`)

	// Exactly one $/cancel_request cascade for the open dialog.
	permAwait(t, cli, &st, func(s *permStory) bool { return st.cancels >= 1 })

	// The drain resumed the dead turn with the cancelled-normal result; the
	// resumed round's closing chunk is the deterministic completion marker.
	permAwait(t, cli, &st, func(s *permStory) bool { return s.hasChunk("wound down.") })

	if st.cancels != 1 {
		t.Errorf("$/cancel_request frames = %d; want exactly 1", st.cancels)
	}

	permAssertNoAskFrames(t, cli)

	results := permWaitResults(t, workDir, sid, "call_wd", 1)
	if results[0].IsError {
		t.Errorf("cancelled result is an error; want cancelled-NORMAL: %s", results[0].Output)
	}

	if !strings.Contains(results[0].Output, "Tool call cancelled") {
		t.Errorf("cancelled result output = %s; want the cancelled-normal form", results[0].Output)
	}

	_, statErr := os.Stat(filepath.Join(workDir, permFile))
	if statErr == nil {
		t.Error("the gated call executed although the dialog was cancelled")
	}

	if len(st.permFrames) != 1 {
		t.Errorf("permission frames = %d; want exactly 1 (no zombie asks fired for the dead turn)", len(st.permFrames))
	}
}

// permScenarioUngatedDefault: a fresh default session (no mode set) performs
// the same mutating call with ZERO permission frames — criterion 4's
// dialog-free default, with the trust file left untouched.
func permScenarioUngatedDefault(t *testing.T) {
	t.Helper()

	cli, _, workDir := permStartServe(t, func(dir string) []simTurnScript {
		return []simTurnScript{
			{phases: []simPhase{{callID: "call_u1", name: permToolWrite, input: permWriteInput(dir)}}},
			{phases: []simPhase{{text: "ungated ran."}}},
		}
	})

	if probes := permInitialize(t, cli, true); probes != 0 {
		t.Fatalf("probe frames = %d; want 0", probes)
	}

	sid := permSessionNew(t, cli, workDir)

	var st permStory

	permPrompt(t, cli, sid, "e2e-p1", "write the file")
	permAwait(t, cli, &st, func(s *permStory) bool {
		return s.responded("e2e-p1") && s.hasChunk("ungated ran.")
	})

	if len(st.permFrames) != 0 {
		t.Errorf("ungated default produced %d permission frame(s); want 0 (criterion 4)", len(st.permFrames))
	}

	permWaitFile(t, filepath.Join(workDir, permFile))

	env := permReadYaml(t, workDir)
	if len(env.Allow)+len(env.Deny)+len(env.Ask) != 0 {
		t.Errorf("ungated default wrote trust entries allow=%v deny=%v ask=%v; want none", env.Allow, env.Deny, env.Ask)
	}
}
