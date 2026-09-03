package acpserve //nolint:testpackage // internal package test

// The 18-05 kill -9 matrix harness (ACP-06, ROADMAP criterion 2 — "a kill -9
// mid-turn followed by resume leaves NO ghost state"): TestKill9Resume spawns
// a REAL serve child process (the test binary re-exec'd via TestMain under
// ASS_GUARD_KILL9_CHILD), drives it over stdio pipes exactly as the 16-06
// Zed-client simulator does (initialize → session/new → session/prompt), with
// the row's stubbed provider pausing at that inventory class's suspension
// point, SIGKILLs the child, restarts a fresh child over the SAME workdir,
// resumes via session/load, and asserts the four matrix checks per row:
//
//	(a) replay completes before prompt acceptance (the load response and its
//	    replay frames strictly precede any accepted follow-up turn on the
//	    wire; the parked-replay rows additionally pin the typed D-03
//	    rejection of a prompt sent mid-replay),
//	(b) the synthetic closure carries the interrupted provenance ON DISK and
//	    IN REPLAY (the closure's terminal frame),
//	(c) the next turn id continues the sequence (and the plan-mode row's
//	    resumed mode is the persisted one),
//	(d) a follow-up prompt completes a full turn with no orphaned tool_call
//	    visible as live (the whole post-reconciliation transcript pairs
//	    cleanly).
//
// Zero provider calls occur between resume and the deliberate follow-up
// prompt (the stub's counter pins D-01's transcript-local resume). Rows =
// inventory classes 1-5 and 8-9 plus the ask-interplay row (kill during a
// suspended ask with one queued ask; no parked ask fires post-resume; a late
// response to the dead ask's id resolves synthetic-cancel, never a wedge)
// and the kill-during-replay row (SIGKILL the RESUMING child mid-replay; a
// second resume completes — idempotent). Classes 6/7 (state seeds) are
// asserted inside every row's check (c); class 10 (pending permission asks)
// is the ask-interplay row's machinery — Phase 17 ships ask_suspended
// transcript lines for BOTH ask families, the class-3 treatment.
//
// Determinism: every suspension wait is a frame arrival, a stub channel
// signal, or a bounded poll of the transcript FILE for the last synchronous
// line before the dangler — never a blind sleep. The only deliberate wait is
// the SIGKILL round-trip. No env-flag gating: the standard `go test` runs
// the matrix (the child env var is set ONLY on the re-exec'd child).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
)

// The child-mode env contract (parent sets, TestMain branch consumes).
const (
	k9EnvChild    = "ASS_GUARD_KILL9_CHILD"
	k9EnvWorkDir  = "ASS_GUARD_KILL9_WORKDIR"
	k9EnvStub     = "ASS_GUARD_KILL9_STUB"
	k9EnvProfiles = "ASS_GUARD_KILL9_PROFILES"
)

// k9GuardTimeout bounds every frame/file wait in the parent (the simulator's
// discipline: a pathological flow fails instead of hanging). Generous on
// purpose: under the full-suite's parallel load (-race included) a healthy
// child can take seconds per phase; the bound exists to FAIL a wedged flow,
// not to race it.
const k9GuardTimeout = 30 * time.Second

// k9KindToolCallUpdate is the v1 sessionUpdate kind of a terminal tool
// update (the harness's local vocabulary — the simulator file has no const).
const k9KindToolCallUpdate = "tool_call_update"

// k9TranscriptPoll bounds the transcript-file polls (10ms interval); the
// same load-tolerant generosity as k9GuardTimeout.
const k9TranscriptPoll = 15 * time.Second

// TestMain routes the re-exec'd child into the serve mode BEFORE any test
// runs (the parent sets k9EnvChild only on the spawned process).
func TestMain(m *testing.M) {
	if row := os.Getenv(k9EnvChild); row != "" {
		kill9ChildMain(row)
	}

	os.Exit(m.Run())
}

// kill9ChildMain is the child serve: the REAL Run composition over the
// process stdio, the stub-backed project config, engine ON (the 17-05
// lesson: whole-Run batteries asserting on-disk effects need the real
// executor). Never returns.
func kill9ChildMain(row string) {
	workDir := os.Getenv(k9EnvWorkDir)
	stubURL := os.Getenv(k9EnvStub)
	profiles := os.Getenv(k9EnvProfiles)

	if workDir == "" || stubURL == "" || profiles == "" {
		fmt.Fprintf(os.Stderr, "kill9 child %s: incomplete env (workdir/stub/profiles)\n", row)
		os.Exit(2)
	}

	err := os.MkdirAll(filepath.Join(workDir, ".ass-guard"), 0o750)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kill9 child %s: mkdir store: %v\n", row, err)
		os.Exit(2)
	}

	config := "providers:\n  anthropic:\n    base_url: " + strconv.Quote(stubURL) +
		"\n    api_key: \"sk-kill9-canary\"\n"

	werr := os.WriteFile(filepath.Join(workDir, ".ass-guard", "config.yaml"),
		[]byte(config), 0o600)
	if werr != nil {
		fmt.Fprintf(os.Stderr, "kill9 child %s: write config: %v\n", row, werr)
		os.Exit(2)
	}

	// 0 exit code on a clean EOF-driven shutdown; a serve error exits 1 so
	// the parent's diagnostics distinguish a crash from a kill. A plain
	// background ctx: the child dies by SIGKILL or stdin EOF — signal
	// handling would only race the kill.
	rerr := Run(context.Background(), os.Stdin, os.Stdout, os.Stderr, &Options{
		Profile:       profileZcode,
		MaxConcurrent: 2,
		EngineEnabled: true, // the real tool executor (the 17-05 lesson)
		ProfilesDir:   profiles,
		WorkDir:       workDir,
		AskTimeout:    time.Hour,
	})
	if rerr != nil {
		fmt.Fprintf(os.Stderr, "kill9 child %s: serve: %v\n", row, rerr)
		os.Exit(1)
	}

	os.Exit(0)
}

// --- the stubbed provider (parent process; survives child death) ---

// k9Entry is one scripted provider request: a completing response (phases),
// a bare hold (accept + never answer), or a partial stream (flush the phases'
// SSE blocks, then hold before the terminal frames).
type k9Entry struct {
	phases  []simPhase
	hold    bool
	partial bool
}

// k9Stub serves the row's script by request index ACROSS child processes
// (the counter never resets: child #1's held requests stay held, child #2's
// resume makes no request, the follow-up consumes the next entry). Every
// hold/partial entry signals its index on entry — the parent's suspension
// signal for hold-shaped rows.
type k9Stub struct {
	mu     sync.Mutex
	script []k9Entry
	calls  int

	holds chan int

	srv *httptest.Server
}

func newK9Stub(script []k9Entry) *k9Stub {
	st := &k9Stub{script: script, holds: make(chan int, len(script))}
	st.srv = httptest.NewServer(http.HandlerFunc(st.serveSSE))

	return st
}

func (s *k9Stub) serveSSE(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	idx := s.calls
	s.calls++
	entry := k9Entry{phases: []simPhase{{text: "ok"}}}

	if idx < len(s.script) {
		entry = s.script[idx]
	}

	s.mu.Unlock()

	flusher, _ := w.(http.Flusher)

	if entry.hold || entry.partial {
		s.holds <- idx

		if entry.partial {
			w.Header().Set("Content-Type", "text/event-stream")

			for _, frame := range simSSEFrames(entry.phases)[:len(simSSEFrames(entry.phases))-2] {
				_, _ = io.WriteString(w, frame)

				if flusher != nil {
					flusher.Flush()
				}
			}
		}

		// The suspension: hold until the child dies (the request ctx fires
		// when the killed client's socket drains) or the guard expires —
		// the guard is the BOUND (a served hold whose child died must not
		// outlive the row; 2s is orders of magnitude beyond the parent's
		// kill round-trip — every row suspends and kills within tens of
		// milliseconds — and far below the matrix's runtime budget).
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}

		return
	}

	w.Header().Set("Content-Type", "text/event-stream")

	for _, frame := range simSSEFrames(entry.phases) {
		_, _ = io.WriteString(w, frame)

		if flusher != nil {
			flusher.Flush()
		}
	}
}

func (s *k9Stub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.calls
}

// waitHold blocks until a hold/partial entry with index >= want is entered
// (the deterministic "suspension reached" signal — a channel, never a sleep).
func (s *k9Stub) waitHold(t *testing.T, want int) {
	t.Helper()

	deadline := time.After(k9GuardTimeout)

	for {
		select {
		case idx := <-s.holds:
			if idx >= want {
				return
			}
		case <-deadline:
			t.Fatalf("stub hold %d never reached (calls=%d)", want, s.count())
		}
	}
}

// --- the parent-side process + client drivers ---

// k9Proc is one spawned serve child.
type k9Proc struct {
	t       *testing.T
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	client  *k9Client
	stderr  *lockedBuffer
	cancel  context.CancelFunc
	waitErr error
	waited  bool
}

// lockedBuffer is a mutex-guarded stderr sink (the child writes from many
// goroutines).
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p) //nolint:wrapcheck // test helper passthrough
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// k9Client is the parent's frame reader/writer over the child's stdio pipes
// (the simClient discipline: bounded waits, arrival order = wire order).
type k9Client struct {
	t      *testing.T
	in     io.WriteCloser
	frames chan *acp.Message
}

func newK9Client(t *testing.T, out io.ReadCloser, in io.WriteCloser) *k9Client {
	t.Helper()

	c := &k9Client{t: t, in: in, frames: make(chan *acp.Message, 1024)}

	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)

		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())

			if len(line) == 0 {
				continue
			}

			var m acp.Message

			if json.Unmarshal(line, &m) == nil {
				c.frames <- &m
			}
		}

		close(c.frames)
	}()

	return c
}

func (c *k9Client) sendf(format string, args ...any) {
	c.t.Helper()

	_, err := fmt.Fprintf(c.in, format+"\n", args...)
	if err != nil {
		c.t.Fatalf("kill9: write frame: %v", err)
	}
}

func (c *k9Client) next() *acp.Message {
	c.t.Helper()

	select {
	case m, ok := <-c.frames:
		if !ok {
			c.t.Fatalf("kill9: child stdout closed early")
		}

		return m
	case <-time.After(k9GuardTimeout):
		c.t.Fatalf("kill9: timed out waiting for a frame")
	}

	return nil
}

// k9Spawn boots one serve child over the row's workdir/stub.
func k9Spawn(t *testing.T, row, workDir, stubURL, profiles string) *k9Proc {
	t.Helper()

	// A deadline ctx reaps a hung child (belt beside the row's own guards);
	// normal exits (kill/EOF) beat it by orders of magnitude.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestKill9Resume")

	cmd.Env = append(os.Environ(),
		k9EnvChild+"="+row,
		k9EnvWorkDir+"="+workDir,
		k9EnvStub+"="+stubURL,
		k9EnvProfiles+"="+profiles,
		"ZAI_API_KEY=")

	stdin, ierr := cmd.StdinPipe()
	if ierr != nil {
		t.Fatalf("stdin pipe: %v", ierr)
	}

	stdout, oerr := cmd.StdoutPipe()
	if oerr != nil {
		t.Fatalf("stdout pipe: %v", oerr)
	}

	stderr := &lockedBuffer{}
	cmd.Stderr = stderr

	serr := cmd.Start()
	if serr != nil {
		t.Fatalf("spawn child: %v", serr)
	}

	p := &k9Proc{t: t, cmd: cmd, stdin: stdin, stderr: stderr, cancel: cancel}
	p.client = newK9Client(t, stdout, stdin)

	return p
}

// kill SIGKILLs the child and proves the death was a REAL SIGKILL (a clean
// exit here means the row never exercised process death).
func (p *k9Proc) kill() {
	p.t.Helper()

	err := p.cmd.Process.Signal(syscall.SIGKILL)
	if err != nil {
		p.t.Fatalf("SIGKILL: %v", err)
	}

	werr := p.cmd.Wait()
	p.waitErr = werr
	p.waited = true
	p.cancel()

	var exit *exec.ExitError

	if !errors.As(werr, &exit) {
		p.t.Fatalf("child Wait after SIGKILL = %v; want an ExitError (signal death)", werr)
	}

	status, sok := exit.Sys().(syscall.WaitStatus)

	if !sok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		p.t.Fatalf("child death was not SIGKILL (wait=%v)", werr)
	}
}

// stop is the graceful shutdown for children that survive their row (stdin
// EOF → clean serve exit).
func (p *k9Proc) stop() {
	p.t.Helper()

	_ = p.stdin.Close()
	p.cancel()

	werr := p.cmd.Wait()
	if werr != nil {
		p.t.Logf("child clean stop returned %v; stderr tail:\n%s", werr, p.stderrTail())
	}
}

func (p *k9Proc) stderrTail() string {
	s := p.stderr.String()

	if len(s) > 4000 {
		return s[len(s)-4000:]
	}

	return s
}

// dumpStderr surfaces the child's stderr on failure (the diagnosis channel).
func (p *k9Proc) dumpStderr(stage string) {
	p.t.Helper()

	if p.t.Failed() {
		p.t.Logf("child stderr (%s):\n%s", stage, p.stderrTail())
	}
}

// --- wire drivers (the 16-06/17-05 shapes over the child pipes) ---

// k9Initialize performs the handshake WITH the elicitation.form advertisement
// (no capability probe; the ask rows need the elicitation surface live).
func k9Initialize(t *testing.T, cli *k9Client) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"k9-init","method":"initialize","params":{"protocolVersion":1,` +
		`"clientInfo":{"name":"kill9-harness","version":"1"},` +
		`"clientCapabilities":{"elicitation":{"form":{}}},"mcpServers":[]}}`)

	for {
		m := cli.next()

		switch {
		case m.Method == acp.MethodElicitationCreate:
			t.Fatal("capability probe sent although elicitation.form was advertised")
		case isResponseID(m, `"k9-init"`):
			if m.Error != nil {
				t.Fatalf("initialize errored: %+v", m.Error)
			}

			return
		}
	}
}

// k9SessionNew opens the row's session (cwd = the temp project).
func k9SessionNew(t *testing.T, cli *k9Client, workDir string) string {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"k9-new","method":"session/new","params":{"cwd":` +
		simJSONStr(workDir) + `,"mcpServers":[]}}`)

	m := cli.nextResponse(`"k9-new"`)

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

func (c *k9Client) nextResponse(id string) *acp.Message {
	c.t.Helper()

	for {
		m := c.next()

		if isResponseID(m, id) {
			return m
		}
	}
}

// k9Prompt sends one user prompt turn.
func k9Prompt(t *testing.T, cli *k9Client, sessionID, id, text string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":` + simJSONStr(id) + `,"method":"session/prompt","params":{"sessionId":` +
		simJSONStr(sessionID) + `,"prompt":[{"type":"text","text":` + simJSONStr(text) + `}]}}`)
}

// k9Load sends one session/load for the id.
func k9Load(t *testing.T, cli *k9Client, sessionID, workDir string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"k9-load","method":"session/load","params":{"sessionId":` +
		simJSONStr(sessionID) + `,"cwd":` + simJSONStr(workDir) + `,"mcpServers":[]}}`)
}

// k9Update is one decoded session/update payload (the harness's view).
type k9Update struct {
	Kind       string `json:"sessionUpdate"` //nolint:tagliatelle // ACP wire field
	ToolCallID string `json:"toolCallId"`    //nolint:tagliatelle // ACP wire field
	Status     string `json:"status"`
}

func decodeK9Update(t *testing.T, m *acp.Message) k9Update {
	t.Helper()

	var params struct {
		Update k9Update `json:"update"`
	}

	err := json.Unmarshal(m.Params, &params)
	if err != nil {
		t.Fatalf("decode session/update: %v (%s)", err, string(m.Params))
	}

	return params.Update
}

// k9LoadStory reads until the load response, collecting the update kinds and
// enforcing the (a) ordering: every session/update is a replay frame
// (chunk/card/update/available_commands_update) and available_commands_update
// arrives only AFTER the last other replay frame; no ask-family frame and no
// follow-up turn frame may appear before the response.
type k9LoadStory struct {
	Kinds         []string
	Terminals     []string // tool_call_update frames as "toolCallId:status"
	SawCommands   bool
	CommandsAfter int // index in Kinds of the commands frame
	LoadResponse  *acp.Message
}

//nolint:cyclop // one ordered wire-collect loop with inline ordering asserts
func k9CollectLoad(t *testing.T, cli *k9Client) *k9LoadStory {
	t.Helper()

	st := &k9LoadStory{}

	for {
		m := cli.next()

		if isResponseID(m, `"k9-load"`) {
			st.LoadResponse = m

			if m.Error != nil {
				t.Fatalf("session/load errored: %+v", m.Error)
			}

			break
		}

		switch {
		case m.Method == acp.MethodRequestPermission || m.Method == acp.MethodElicitationCreate:
			t.Fatalf("parked ask fired during resume (%s) — ghost state", m.Method)
		case m.Method != simMethodSessionUpdate:
			continue // notifications outside the update family
		}

		upd := decodeK9Update(t, m)
		st.Kinds = append(st.Kinds, upd.Kind)

		switch upd.Kind {
		case acp.KindAvailableCommandsUpdate:
			if st.SawCommands {
				t.Fatalf("two available_commands_update frames: %v", st.Kinds)
			}

			st.SawCommands = true
			st.CommandsAfter = len(st.Kinds) - 1
		case k9KindToolCallUpdate:
			if st.SawCommands {
				t.Fatalf("replay frame after available_commands_update: %v", st.Kinds)
			}

			st.Terminals = append(st.Terminals, upd.ToolCallID+":"+upd.Status)
		case simKindToolCall, simKindChunk:
			if st.SawCommands {
				t.Fatalf("replay frame after available_commands_update: %v", st.Kinds)
			}
		default:
			// thought/plan frames are legal replay output; anything else is
			// still a replay frame — no assertion.
		}
	}

	if !st.SawCommands {
		t.Fatalf("no available_commands_update before the load response; kinds=%v", st.Kinds)
	}

	if st.CommandsAfter != len(st.Kinds)-1 {
		t.Fatalf("available_commands_update at %d is not the last pre-response frame: %v",
			st.CommandsAfter, st.Kinds)
	}

	return st
}

// k9ReadTranscript parses the session's transcript into the harness's line
// view (tolerant: non-conforming lines skip).
type k9Line struct {
	Type           string `json:"type"`
	TurnID         string `json:"turnID"`     //nolint:tagliatelle // on-disk format
	ToolCallID     string `json:"toolCallID"` //nolint:tagliatelle // on-disk format
	Cause          string `json:"cause"`
	IsError        bool   `json:"isError"`        //nolint:tagliatelle // on-disk format
	ParentTurnID   string `json:"parentTurnID"`   //nolint:tagliatelle // on-disk format
	SubagentTurnID string `json:"subagentTurnID"` //nolint:tagliatelle // on-disk format
}

func k9TranscriptPath(workDir, sid string) string {
	return filepath.Join(workDir, ".ass-guard", "transcript_"+sid+".jsonl")
}

func k9ReadTranscript(t *testing.T, workDir, sid string) []k9Line {
	t.Helper()

	raw, err := os.ReadFile(k9TranscriptPath(workDir, sid))
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}

	var out []k9Line

	for seg := range strings.SplitSeq(string(raw), "\n") {
		if seg == "" {
			continue
		}

		var l k9Line

		if json.Unmarshal([]byte(seg), &l) == nil {
			out = append(out, l)
		}
	}

	return out
}

// k9AwaitTranscript polls the transcript file until want holds (bounded) —
// the deterministic suspension gate for rows whose dangler's PREDECESSOR
// line is appended synchronously.
func k9AwaitTranscript(t *testing.T, workDir, sid, want string) {
	t.Helper()

	deadline := time.Now().Add(k9TranscriptPoll)

	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(k9TranscriptPath(workDir, sid))

		if err == nil && strings.Contains(string(raw), want) {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("transcript never contained %q within %v", want, k9TranscriptPoll)
}

// k9Closure helpers: count/locate the interrupted-provenance closures.
const k9InterruptedCause = "interrupted"

func k9IsClosure(cause string) bool { return cause == k9InterruptedCause }

func k9HasClosure(lines []k9Line, typ, turnID, callID string) bool {
	for _, l := range lines {
		if !k9IsClosure(l.Cause) || l.Type != typ {
			continue
		}

		if turnID != "" && l.TurnID != turnID {
			continue
		}

		if callID != "" && l.ToolCallID != callID {
			continue
		}

		return true
	}

	return false
}

// k9AssertPairs checks (d)'s no-ghost invariant over the WHOLE
// post-reconciliation transcript: every tool_call has exactly one matching
// tool_result, every subagent_dispatch its subagent_result, every
// ask_suspended its (cancelled-normal) tool_result.
func k9AssertPairs(t *testing.T, lines []k9Line) {
	t.Helper()

	count := func(typ string, key func(k9Line) string) map[string]int {
		out := map[string]int{}

		for _, l := range lines {
			if l.Type == typ && key(l) != "" {
				out[key(l)]++
			}
		}

		return out
	}

	pair := func(openKind, closeKind string, key func(k9Line) string, label string) {
		opens := count(openKind, key)
		closes := count(closeKind, key)

		for id, n := range opens {
			if closes[id] != n {
				t.Errorf("%s %s has %d results for %d calls (ghost state)", label, id, closes[id], n)
			}
		}
	}

	callKey := func(l k9Line) string { return l.ToolCallID }
	subKey := func(l k9Line) string { return l.SubagentTurnID }

	pair("tool_call", "tool_result", callKey, "tool_call")
	pair("ask_suspended", "tool_result", callKey, "ask_suspended")
	pair("subagent_dispatch", "subagent_result", subKey, "subagent_dispatch")

	for _, l := range lines {
		if l.Type == "ask_suspended" && !k9HasClosure(lines, "tool_result", "", l.ToolCallID) {
			t.Errorf("ask_suspended %s has no post-resume closure", l.ToolCallID)
		}
	}
}

// k9AssertModes decodes the load response's modes (nil when absent).
func k9AssertModes(t *testing.T, resp *acp.Message) *struct {
	CurrentModeID string `json:"currentModeId"` //nolint:tagliatelle // ACP wire field
} {
	t.Helper()

	var res struct {
		Modes *struct {
			CurrentModeID string `json:"currentModeId"` //nolint:tagliatelle // ACP wire field
		} `json:"modes"`
	}

	err := json.Unmarshal(resp.Result, &res)
	if err != nil {
		t.Fatalf("decode load result: %v (%s)", err, string(resp.Result))
	}

	return res.Modes
}

// k9FollowUp drives the deliberate post-resume prompt (checks c + d): the
// response arrives (a full turn), the transcript's next turn id continues
// the sequence, and the whole transcript still pairs.
func k9FollowUp(t *testing.T, p *k9Proc, stub *k9Stub, workDir, sid, wantTurn string) {
	t.Helper()

	k9Prompt(t, p.client, sid, "k9-after", "post-resume turn")

	resp := p.client.nextResponse(`"k9-after"`)

	if resp.Error != nil {
		t.Fatalf("post-resume prompt errored: %+v", resp.Error)
	}

	k9AwaitTranscript(t, workDir, sid, `"turnID":"`+wantTurn+`"`)

	lines := k9ReadTranscript(t, workDir, sid)

	found := false

	for _, l := range lines {
		if l.TurnID == wantTurn && l.Type == "user_message" {
			found = true
		}
	}

	if !found {
		t.Errorf("no post-resume user_message for %s (turn id did not continue the sequence)", wantTurn)
	}

	if !k9HasLine(lines, "assistant_message", wantTurn) {
		t.Errorf("post-resume turn %s has no assistant_message (full-turn check)", wantTurn)
	}

	k9AssertPairs(t, lines)
}

func k9HasLine(lines []k9Line, typ, turnID string) bool {
	for _, l := range lines {
		if l.Type == typ && (turnID == "" || l.TurnID == turnID) {
			return true
		}
	}

	return false
}

// k9DrainQuiet proves the bounded window stays free of ask-family frames
// (the no-parked-ask assertion; the permAssertNoAskFrames discipline over
// the child pipe).
func k9DrainQuiet(t *testing.T, cli *k9Client) {
	t.Helper()

	deadline := time.Now().Add(300 * time.Millisecond)

	for time.Now().Before(deadline) {
		select {
		case m, ok := <-cli.frames:
			if !ok {
				return
			}

			if m.Method == acp.MethodRequestPermission || m.Method == acp.MethodElicitationCreate {
				t.Fatalf("parked ask fired post-resume: %s %s", m.Method, string(m.Params))
			}
		case <-time.After(time.Until(deadline)):
		}
	}
}

// --- the row table (inventory classes 1-5, 8-9 + the ask-interplay row) ---

// k9FollowScript is the completing entry every row's follow-up prompt
// consumes (the deliberate post-resume provider call — everything before it
// must have been transcript-local).
func k9FollowScript() []k9Entry {
	return []k9Entry{{phases: []simPhase{{text: "resumed."}}}}
}

// k9Row is one matrix row: phase 1 (drive child #1 to the suspension point),
// the suspension wait, and phase 2's row-specific on-disk/replay assertions.
type k9Row struct {
	name   string
	gated  bool
	script []k9Entry
	// waitSuspend blocks until the row's suspension point is provably
	// reached (frame / stub channel / transcript poll) and returns the dead
	// ask's raw JSON-RPC id when the row fired one ("" otherwise).
	waitSuspend func(t *testing.T, stub *k9Stub, p1 *k9Proc, workDir, sid string) string
	// assert runs the row's closure-set + replay-frame assertions over the
	// POST-RESUME transcript and the load story.
	assert func(t *testing.T, story *k9LoadStory, lines []k9Line, sid string)
	// wantTurnN is the follow-up turn NUMBER (check c): the driver formats
	// <sid>-turn-%03d — the sequence continues past whatever ids the killed
	// state consumed.
	wantTurnN int
	// wantModes: "" asserts the response modes null; non-empty asserts
	// currentModeId equals it (check c's plan-mode half).
	wantModes string
}

// k9WorkDir builds the row's temp project (best-effort cleanup — the
// perm-e2e unlinkat-race note).
func k9WorkDir(t *testing.T) string {
	t.Helper()

	// os.MkdirTemp, not t.TempDir: the TempDir cleanup ASSERTS on removal
	// and fails on macOS unlinkat races with the killed child's late
	// flushes — a best-effort remove is deliberate (the perm-e2e note).
	dir, err := os.MkdirTemp("", "kill9-") //nolint:usetesting // see above
	if err != nil {
		t.Fatalf("temp project dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}

// k9BootAndPrompt spawns child #1, handshakes, opens the session, and drives
// the row's turn (the gated flag flips permissions.mode first).
//
//nolint:nonamedreturns // the names document the pair for the caller
func k9BootAndPrompt(t *testing.T, row *k9Row, stub *k9Stub, workDir string) (p1 *k9Proc, sid string) {
	t.Helper()

	p1 = k9Spawn(t, row.name, workDir, stub.srv.URL, repoProfilesDir(t))

	k9Initialize(t, p1.client)

	sid = k9SessionNew(t, p1.client, workDir)

	if row.gated {
		k9SetGated(t, p1.client, sid)
	}

	k9Prompt(t, p1.client, sid, "k9-1", "drive the row")

	return p1, sid
}

// k9SetGated flips permissions.mode through the real set_config_option
// handler (the permSetMode discipline over the child pipe).
func k9SetGated(t *testing.T, cli *k9Client, sessionID string) {
	t.Helper()

	cli.sendf(`{"jsonrpc":"2.0","id":"k9-mode","method":"session/set_config_option","params":{"sessionId":` +
		simJSONStr(sessionID) + `,"configId":` + simJSONStr(optPermissionsMode) + `,"value":` +
		simJSONStr(permModeGated) + `}}`)

	for {
		m := cli.next()

		if isResponseID(m, `"k9-mode"`) {
			if m.Error != nil {
				t.Fatalf("set permissions.mode=%s failed: %d %s",
					permModeGated, m.Error.Code, m.Error.Message)
			}

			return
		}
		// the racing config_option_update rides alongside — tolerated
	}
}

// k9WaitPromptResponse reads until the row's phase-1 prompt responds (the
// turn provably completed server-side).
func k9WaitPromptResponse(t *testing.T, cli *k9Client) {
	t.Helper()

	resp := cli.nextResponse(`"k9-1"`)
	if resp.Error != nil {
		t.Fatalf("phase-1 prompt errored: %+v", resp.Error)
	}
}

// k9ClosureLine finds the interrupted closure matching the keys (nil when
// absent).
func k9ClosureLine(lines []k9Line, typ, turnID, callID string) *k9Line {
	for i := range lines {
		l := &lines[i]
		if !k9IsClosure(l.Cause) || l.Type != typ {
			continue
		}

		if turnID != "" && l.TurnID != turnID {
			continue
		}

		if callID != "" && l.ToolCallID != callID {
			continue
		}

		return l
	}

	return nil
}

// k9AssertTerminal pins (b)'s replay half: the closure's terminal frame is
// in the load story with the wanted status.
func k9AssertTerminal(t *testing.T, story *k9LoadStory, callID, status string) {
	t.Helper()

	want := callID + ":" + status

	if slices.Contains(story.Terminals, want) {
		return
	}

	t.Errorf("replay lacks the closure terminal %s (have %v)", want, story.Terminals)
}

// k9LabelKey is the captured options' label key (goconst: shared with the
// 17-05 battery's literals).
const k9LabelKey = "label"

// k9AskInput builds the scripted AskUserQuestion input (the captured
// {questions:[…]} shape).
func k9AskInput() string {
	raw, err := json.Marshal(map[string]any{
		"questions": []map[string]any{{
			"question":    "Proceed with the plan?",
			"header":      "Confirm",
			"options":     []map[string]string{{k9LabelKey: "yes"}, {k9LabelKey: "no"}},
			"multiSelect": false,
		}},
	})
	if err != nil {
		panic(err) // a static map — cannot fail
	}

	return string(raw)
}

// k9AwaitAskFrame reads until an ask-family frame arrives (elicitation or
// request_permission) and returns its raw JSON-RPC id.
func k9AwaitAskFrame(t *testing.T, cli *k9Client) string {
	t.Helper()

	deadline := time.After(k9GuardTimeout)

	for {
		select {
		case m, ok := <-cli.frames:
			if !ok {
				t.Fatal("child stdout closed while awaiting the ask frame")
			}

			switch m.Method {
			case acp.MethodElicitationCreate, acp.MethodRequestPermission:
				return string(m.ID)
			}
		case <-deadline:
			t.Fatalf("no ask frame fired within %v", k9GuardTimeout)
		}
	}
}

// k9AwaitTranscriptCount polls until the transcript contains at least n
// occurrences of substr (the queued-ask gate: TWO ask_suspended lines).
func k9AwaitTranscriptCount(t *testing.T, workDir, sid, substr string, n int) {
	t.Helper()

	deadline := time.Now().Add(k9TranscriptPoll)

	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(k9TranscriptPath(workDir, sid))

		if err == nil && strings.Count(string(raw), substr) >= n {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("transcript never held %d x %q within %v", n, substr, k9TranscriptPoll)
}

// TestKill9Resume is the matrix: every row performs a REAL SIGKILL of a REAL
// serve child at that inventory class's suspension point and proves the
// resume leaves NO ghost state. The kill-during-replay row runs beside the
// table (its own three-child flow).
//
//nolint:gocognit,gocyclo,cyclop,funlen,maintidx // the matrix table: the row closures' complexity rolls up here
func TestKill9Resume(t *testing.T) {
	t.Parallel()

	rows := []k9Row{
		{
			// Class 1: the Bash tool_call landed; the tool blocks in
			// execution; SIGKILL. The dangling call closes as a FAILED
			// tool_result (interrupted cause) on disk AND in replay.
			name:   "class1_dangling_tool_call",
			script: []k9Entry{{phases: []simPhase{{callID: "call_c1", name: "Bash", input: `{"command":"sleep 30"}`}}}},
			waitSuspend: func(t *testing.T, _ *k9Stub, p1 *k9Proc, workDir, sid string) string {
				t.Helper()

				k9AwaitTranscript(t, workDir, sid, `"name":"Bash"`)
				p1.dumpStderr("class1-suspended")

				return ""
			},
			assert: func(t *testing.T, story *k9LoadStory, lines []k9Line, sid string) {
				t.Helper()

				closure := k9ClosureLine(lines, "tool_result", sid+"-turn-001", "call_c1")
				if closure == nil {
					t.Fatal("no failed tool_result closure for call_c1 on disk")
				}

				if !closure.IsError {
					t.Error("class-1 closure isError = false; want true (failed)")
				}

				if !k9HasClosure(lines, "canceled", sid+"-turn-001", "") {
					t.Error("no canceled terminal closure for the killed turn")
				}

				if !k9HasClosure(lines, "session_end", "", "") {
					t.Error("no synthetic session_end closure")
				}

				k9AssertTerminal(t, story, "call_c1", "failed")
			},
			wantTurnN: 2,
		},
		{
			// Class 2: the provider request is held before any content; the
			// user_message turn has no terminal. SIGKILL → synthetic canceled.
			name:   "class2_turn_without_terminal",
			script: []k9Entry{{hold: true}},
			waitSuspend: func(t *testing.T, stub *k9Stub, p1 *k9Proc, workDir, sid string) string {
				t.Helper()

				stub.waitHold(t, 0)
				k9AwaitTranscript(t, workDir, sid, `"type":"user_message"`)
				p1.dumpStderr("class2-suspended")

				return ""
			},
			assert: func(t *testing.T, _ *k9LoadStory, lines []k9Line, sid string) {
				t.Helper()

				if !k9HasClosure(lines, "canceled", sid+"-turn-001", "") {
					t.Error("no canceled terminal closure for the turn without terminal")
				}

				if !k9HasClosure(lines, "session_end", "", "") {
					t.Error("no synthetic session_end closure")
				}
			},
			wantTurnN: 2,
		},
		{
			// Class 3 (+10): the AskUserQuestion ask fired (elicitation) and
			// the turn suspended; SIGKILL while pending. The unresolved
			// ask_suspended closes as the cancelled-NORMAL tool_result; NO
			// canceled closure (the ask IS the turn's terminal).
			name:   "class3_ask_suspended_unresolved",
			script: []k9Entry{{phases: []simPhase{{callID: "call_ask", name: "AskUserQuestion", input: k9AskInput()}}}},
			waitSuspend: func(t *testing.T, _ *k9Stub, p1 *k9Proc, workDir, sid string) string {
				t.Helper()

				deadAsk := k9AwaitAskFrame(t, p1.client)
				k9AwaitTranscript(t, workDir, sid, `"type":"ask_suspended"`)
				p1.dumpStderr("class3-suspended")

				return deadAsk
			},
			assert: func(t *testing.T, _ *k9LoadStory, lines []k9Line, sid string) {
				t.Helper()

				closure := k9ClosureLine(lines, "tool_result", sid+"-turn-001", "call_ask")
				if closure == nil {
					t.Fatal("no cancelled-normal closure for the suspended ask")
				}

				if closure.IsError {
					t.Error("ask closure isError = true; want false (cancelled-NORMAL family)")
				}

				if k9HasClosure(lines, "canceled", "", "") {
					t.Error("canceled closure fired for an ask-terminated turn (the ask IS the terminal)")
				}

				if !k9HasClosure(lines, "session_end", "", "") {
					t.Error("no synthetic session_end closure")
				}
			},
			wantTurnN: 2,
		},
		{
			// Class 4: the Task dispatch landed; the subagent's provider
			// request is held; SIGKILL. The dangling dispatch closes as a
			// failed subagent_result; the follow-up turn id continues PAST
			// the subagent's allocated id (002 → 003).
			name: "class4_subagent_unresolved",
			script: []k9Entry{
				{phases: []simPhase{{callID: "call_task", name: "Task", input: `{"prompt":"map the layout"}`}}},
				{hold: true},
			},
			waitSuspend: func(t *testing.T, stub *k9Stub, p1 *k9Proc, workDir, sid string) string {
				t.Helper()

				stub.waitHold(t, 1)
				k9AwaitTranscript(t, workDir, sid, `"type":"subagent_dispatch"`)
				p1.dumpStderr("class4-suspended")

				return ""
			},
			assert: func(t *testing.T, _ *k9LoadStory, lines []k9Line, sid string) {
				t.Helper()

				sub := sid + "-turn-002"

				if !k9HasClosure(lines, "subagent_result", sub, "") {
					t.Errorf("no failed subagent_result closure for %s", sub)
				}

				if !k9HasClosure(lines, "canceled", sid+"-turn-001", "") {
					t.Error("no canceled terminal closure for the parent turn")
				}

				if !k9HasClosure(lines, "session_end", "", "") {
					t.Error("no synthetic session_end closure")
				}
			},
			wantTurnN: 3,
		},
		{
			// Class 5 + 6: EnterPlanMode executed (the plan_mode enter marker
			// on disk), the SAME turn continued into a partial chunk stream
			// that never closed; SIGKILL. The open stream closes with a
			// terminal assistant_message, the turn with canceled — and the
			// RESUMED MODE IS THE PERSISTED ONE (modes.currentModeId plan).
			name: "class5_open_chunk_stream_plan_mode",
			script: []k9Entry{
				{phases: []simPhase{{callID: "call_plan", name: "EnterPlanMode", input: `{}`}}},
				{partial: true, phases: []simPhase{{text: "partial "}, {text: "stream"}}},
			},
			waitSuspend: func(t *testing.T, stub *k9Stub, p1 *k9Proc, workDir, sid string) string {
				t.Helper()

				stub.waitHold(t, 1)
				k9AwaitTranscript(t, workDir, sid, `"cause":"plan_mode_enter"`)
				k9AwaitTranscript(t, workDir, sid, `"type":"agent_message_chunk"`)
				p1.dumpStderr("class5-suspended")

				return ""
			},
			assert: func(t *testing.T, _ *k9LoadStory, lines []k9Line, sid string) {
				t.Helper()

				if !k9HasClosure(lines, "assistant_message", sid+"-turn-001", "") {
					t.Error("no terminal assistant_message closure for the open chunk stream")
				}

				if !k9HasClosure(lines, "canceled", sid+"-turn-001", "") {
					t.Error("no canceled terminal closure for the killed turn")
				}

				if !k9HasClosure(lines, "session_end", "", "") {
					t.Error("no synthetic session_end closure")
				}
			},
			wantTurnN: 2,
			wantModes: "plan",
		},
		{
			// Class 8: the request_shaped index line landed; the request is
			// held (no usage can follow); SIGKILL. The audit-only pair
			// synthesizes NO usage closure — the closure set is exactly the
			// turn's canceled terminal + session_end.
			name:   "class8_request_without_usage",
			script: []k9Entry{{hold: true}},
			waitSuspend: func(t *testing.T, stub *k9Stub, p1 *k9Proc, workDir, sid string) string {
				t.Helper()

				stub.waitHold(t, 0)
				k9AwaitTranscript(t, workDir, sid, `"type":"request_shaped"`)
				p1.dumpStderr("class8-suspended")

				return ""
			},
			assert: func(t *testing.T, _ *k9LoadStory, lines []k9Line, sid string) {
				t.Helper()

				if !k9HasLine(lines, "request_shaped", "") {
					t.Error("request_shaped line absent (the row's premise)")
				}

				if k9HasLine(lines, "usage", "") {
					t.Error("usage line on disk for a held request (the row's premise broken)")
				}

				if !k9HasClosure(lines, "canceled", sid+"-turn-001", "") {
					t.Error("no canceled terminal closure")
				}

				if !k9HasClosure(lines, "session_end", "", "") {
					t.Error("no synthetic session_end closure")
				}

				for _, l := range lines {
					if k9IsClosure(l.Cause) && l.Type != "canceled" && l.Type != "session_end" {
						t.Errorf("class-8 row synthesized a %s closure (audit-only pair needs none)", l.Type)
					}
				}
			},
			wantTurnN: 2,
		},
		{
			// Class 9: the turn COMPLETED cleanly (response read); SIGKILL
			// between turns. The ONLY closure is the synthetic session_end.
			name:   "class9_missing_session_end",
			script: []k9Entry{{phases: []simPhase{{text: "all done."}}}},
			waitSuspend: func(t *testing.T, _ *k9Stub, p1 *k9Proc, workDir, sid string) string {
				t.Helper()

				k9WaitPromptResponse(t, p1.client)
				k9AwaitTranscript(t, workDir, sid, `"type":"assistant_message"`)
				p1.dumpStderr("class9-between-turns")

				return ""
			},
			assert: func(t *testing.T, _ *k9LoadStory, lines []k9Line, _ string) {
				t.Helper()

				interrupted := 0

				for _, l := range lines {
					if k9IsClosure(l.Cause) {
						interrupted++
					}
				}

				if interrupted != 1 || !k9HasClosure(lines, "session_end", "", "") {
					t.Errorf("class-9 closures = %d; want exactly the one synthetic session_end", interrupted)
				}
			},
			wantTurnN: 2,
		},
		{
			// The ask-interplay row: gated mode; ONE response carrying TWO
			// mutating calls (Write + Bash) suspends both — one dialog open,
			// one queued (17-D-11); SIGKILL while pending. Post-resume: both
			// close cancelled-normal, NO parked ask fires, and a LATE client
			// response to the DEAD ask's request id resolves through the
			// registry's unknown-id path (the 16-D-19 synthetic-cancel spirit
			// at session scale) — never a wedge.
			name:  "ask_interplay_queued_asks",
			gated: true,
			script: []k9Entry{{phases: []simPhase{
				{callID: "call_k9w1", name: "Write", input: `{"file_path":"PLACEHOLDER","content":"x"}`},
				{callID: "call_k9b1", name: "Bash", input: `{"command":"echo hi"}`},
			}}},
			waitSuspend: func(t *testing.T, _ *k9Stub, p1 *k9Proc, workDir, sid string) string {
				t.Helper()

				deadAsk := k9AwaitAskFrame(t, p1.client)
				k9AwaitTranscriptCount(t, workDir, sid, `"type":"ask_suspended"`, 2)
				p1.dumpStderr("ask-interplay-suspended")

				return deadAsk
			},
			assert: func(t *testing.T, _ *k9LoadStory, lines []k9Line, _ string) {
				t.Helper()

				for _, call := range []string{"call_k9w1", "call_k9b1"} {
					closure := k9ClosureLine(lines, "tool_result", "", call)
					if closure == nil {
						t.Errorf("no cancelled-normal closure for %s", call)

						continue
					}

					if closure.IsError {
						t.Errorf("%s closure isError = true; want false (cancelled-NORMAL)", call)
					}
				}

				if k9HasClosure(lines, "canceled", "", "") {
					t.Error("canceled closure fired for an ask-terminated turn")
				}

				if !k9HasClosure(lines, "session_end", "", "") {
					t.Error("no synthetic session_end closure")
				}
			},
			wantTurnN: 2,
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			row := row

			k9DriveRow(t, &row)
		})
	}

	// The kill-during-replay row: its own flow (three children).
	t.Run("kill_during_replay", k9DriveKillDuringReplay)
}

// k9DriveRow is the shared spine: child #1 to the suspension point → REAL
// SIGKILL → child #2 resumes via session/load → the four matrix checks.
func k9DriveRow(t *testing.T, row *k9Row) {
	t.Helper()

	workDir := k9WorkDir(t)

	stub := newK9Stub(append(append([]k9Entry{}, row.script...), k9FollowScript()...))

	t.Cleanup(stub.srv.Close)

	p1, sid := k9BootAndPrompt(t, row, stub, workDir)

	deadAsk := row.waitSuspend(t, stub, p1, workDir, sid)

	p1.kill()

	callsAfterKill := stub.count()

	// Child #2: the resume. The load's replay frames (closures included),
	// the commands re-advertisement, and the response — in that order.
	p2 := k9Spawn(t, row.name+"-resume", workDir, stub.srv.URL, repoProfilesDir(t))

	k9Initialize(t, p2.client)

	k9Load(t, p2.client, sid, workDir)

	story := k9CollectLoad(t, p2.client)

	if got := stub.count(); got != callsAfterKill {
		t.Errorf("provider invoked during resume: %d -> %d calls (D-01 transcript-local violated)",
			callsAfterKill, got)
	}

	lines := k9ReadTranscript(t, workDir, sid)

	row.assert(t, story, lines, sid)

	// Check (c)'s mode half + the modes-null default.
	modes := k9AssertModes(t, story.LoadResponse)

	switch {
	case row.wantModes == "":
		if modes != nil {
			t.Errorf("load response modes = %+v; want null (no plan_mode line persisted)", modes)
		}
	case modes == nil:
		t.Errorf("load response modes = null; want currentModeId %q", row.wantModes)
	case modes.CurrentModeID != row.wantModes:
		t.Errorf("modes.currentModeId = %q; want the persisted %q", modes.CurrentModeID, row.wantModes)
	}

	// The ask-interplay extras: the dead ask's late response resolves (the
	// registry's unknown-id log), NO parked ask fires, nothing wedges.
	if deadAsk != "" {
		p2.client.sendf(`{"jsonrpc":"2.0","id":%s,"result":{"outcome":{"outcome":"selected",`+
			`"optionId":"allow_once"}}}`, deadAsk)

		k9DrainQuiet(t, p2.client)
	}

	// Checks (c) + (d): the follow-up turn continues the sequence and the
	// whole post-reconciliation transcript pairs cleanly.
	k9FollowUp(t, p2, stub, workDir, sid, fmt.Sprintf("%s-turn-%03d", sid, row.wantTurnN))

	p2.stop()

	p2.dumpStderr("resume-child")
}

// k9ChunkPhases builds n text phases (the long-transcript row's body).
func k9ChunkPhases(n int) []simPhase {
	out := make([]simPhase, n)

	for i := range n {
		out[i] = simPhase{text: "c" + strconv.Itoa(i) + " "}
	}

	return out
}

// k9DriveKillDuringReplay is the kill-during-replay row: child #1 completes
// a LONG turn (k9LongChunks chunk lines) then dies; child #2 starts the
// resume and is SIGKILLed MID-REPLAY (the unread pipe parks it; a prompt
// sent mid-replay takes the typed D-03 rejection — check (a)'s strict arm);
// child #3's load COMPLETES and the closures are exactly-once (idempotent).
//
//nolint:funlen // one ordered three-child flow
func k9DriveKillDuringReplay(t *testing.T) {
	t.Helper()

	const longChunks = 2000

	workDir := k9WorkDir(t)

	stub := newK9Stub([]k9Entry{
		{phases: k9ChunkPhases(longChunks)},      // child #1's completing long turn
		{phases: []simPhase{{text: "resumed."}}}, // child #3's follow-up
	})

	t.Cleanup(stub.srv.Close)

	p1 := k9Spawn(t, "kill-during-replay", workDir, stub.srv.URL, repoProfilesDir(t))

	k9Initialize(t, p1.client)

	sid := k9SessionNew(t, p1.client, workDir)

	k9Prompt(t, p1.client, sid, "k9-1", "the long turn")

	k9WaitPromptResponse(t, p1.client)

	// The async writer flushes every chunk line before the kill (bounded
	// poll — the row's premise is a LONG conforming transcript).
	k9AwaitTranscriptCount(t, workDir, sid, `"type":"agent_message_chunk"`, longChunks)

	p1.kill()

	callsAfterKill := stub.count()

	// Child #2: start the resume, read ONE replay frame (provably
	// streaming), send the prompt (the typed mid-replay rejection — D-03),
	// then SIGKILL the RESUMING child.
	p2 := k9Spawn(t, "kill-during-replay-mid", workDir, stub.srv.URL, repoProfilesDir(t))

	k9Initialize(t, p2.client)

	k9Load(t, p2.client, sid, workDir)

	first := p2.client.next()
	if first.Method != simMethodSessionUpdate {
		t.Fatalf("first frame after load = %v; want a session/update chunk", first.Method)
	}

	k9Prompt(t, p2.client, sid, "k9-early", "too early")

	rejection := p2.client.nextResponse(`"k9-early"`)

	if rejection.Error == nil {
		t.Fatalf("prompt during the parked replay accepted: %s (D-03 violated)", string(rejection.Result))
	}

	if !strings.Contains(rejection.Error.Message, "replay") {
		t.Errorf("mid-replay rejection message = %q; want it to name the replay state",
			rejection.Error.Message)
	}

	p2.kill()

	// Child #3: the second resume completes; the closures are exactly-once.
	p3 := k9Spawn(t, "kill-during-replay-final", workDir, stub.srv.URL, repoProfilesDir(t))

	k9Initialize(t, p3.client)

	k9Load(t, p3.client, sid, workDir)

	story := k9CollectLoad(t, p3.client)

	if got := stub.count(); got != callsAfterKill {
		t.Errorf("provider invoked across both resumes: %d -> %d calls (D-01 violated)",
			callsAfterKill, got)
	}

	chunks := 0

	for _, kind := range story.Kinds {
		if kind == simKindChunk {
			chunks++
		}
	}

	// The replayed chunk frames = the streamed chunks + the final
	// assistant_message (replay maps BOTH to agent_message_chunk).
	if chunks != longChunks+1 {
		t.Errorf("final replay chunk frames = %d; want %d (the whole transcript replays)",
			chunks, longChunks+1)
	}

	lines := k9ReadTranscript(t, workDir, sid)

	interrupted := 0

	for _, l := range lines {
		if k9IsClosure(l.Cause) {
			interrupted++

			if l.Type != "session_end" {
				t.Errorf("unexpected %s closure (the killed turn completed cleanly)", l.Type)
			}
		}
	}

	if interrupted != 1 {
		t.Errorf("interrupted closures = %d; want exactly 1 (the session_end, appended once across kills)",
			interrupted)
	}

	k9FollowUp(t, p3, stub, workDir, sid, sid+"-turn-002")

	p3.stop()

	p3.dumpStderr("final-resume-child")
}
