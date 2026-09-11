package acpserve //nolint:testpackage // internal package test

// The 25-08 subject-split shared helpers: wire-frame batteries whose subject
// is the serve path (frames out of a REAL acp.Server through the PRODUCTION
// kitTurnAdapter) moved here from kit/runtime per the Phase-15 D-02 rule —
// tests follow their subject. This file carries the shared vocabulary they
// used in-package kit-side (frame send/read helpers, wire-literal constants,
// the mock/paced providers, the driveACP composition) so each moved battery
// keeps its assertions byte-identical; only the runner construction changes
// (the unexported Runner literal -> NewRunner, the acpTurnRunner twin ->
// the production kitTurnAdapter). Zero Test functions — the D-20 ledger
// holds (helpers ride free in the ^func Test count).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/kit/event"
	"github.com/Djarvur/ass-guard-agent/kit/profile"
	"github.com/Djarvur/ass-guard-agent/kit/provider"
	"github.com/Djarvur/ass-guard-agent/kit/runtime"
	"github.com/Djarvur/ass-guard-agent/kit/session"
)

// Wire-literal constants (goconst) shared by the moved wire batteries —
// mirrors of the kit/runtime test vocabulary (blockText/stopEndTurn/chunkDone
// are the kit's unexported goconst set; the acp package's own copies stay
// unexported there).
const (
	wireBlockText     = "text"
	wireStopEndTurn   = "end_turn"
	wireChunkDone     = "done"
	wireChunkToolUse  = "tool_use"
	wireChunkThinking = "thinking"

	wireProtocolVersion20 = "2.0"
	wireKeySessionID      = "sessionId"
	wireKeyProtoVersion   = "protocolVersion"
	wireMethodInitialize  = "initialize"
	wireMethodSessNew     = "session/new"
	wireMethodSessPrmt    = "session/prompt"
	wireSessionUpdate     = "session/update"
	wireCwdKey            = "cwd"
	wireKeyType           = "type"
	wireKeyPrompt         = "prompt"
	wireKeyMcpServers     = "mcpServers"
	wireTestCwdTmp        = "/tmp"

	wireKeyClientCapabilities = "clientCapabilities"
	wireKeyElicitation        = "elicitation"
	wireKeyForm               = "form"
)

// mockStreamProvider is a Provider whose Stream emits canned text chunks then a
// done chunk. Used to drive the real Session Core through the ACP server without
// a live model call (autonomous: true).
type mockStreamProvider struct {
	chunks []string
	finish string
}

func (m *mockStreamProvider) Send(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{FinishReason: m.finish}, nil
}

func (m *mockStreamProvider) Stream(
	ctx context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, 8)
	go func() {
		defer close(ch)

		for _, c := range m.chunks {
			select {
			case ch <- provider.StreamChunk{Type: wireBlockText, Text: c}:
			case <-ctx.Done():
				return
			}
		}

		select {
		case ch <- provider.StreamChunk{Type: wireChunkDone, FinishReason: m.finish}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (m *mockStreamProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

// SupportsImages: the fake is text-only (21-05 D-11 seam stub).
func (m *mockStreamProvider) SupportsImages() bool { return false }

// wireDriveACP composes a REAL acp.Server over pipes around a kit Runner
// built through the public constructor, driven through the PRODUCTION
// kitTurnAdapter (the moved batteries' construction-site retarget — the
// kit-side acpTurnRunner twin they used before the split). The test writes
// client frames to cliW and reads from cliR.
func wireDriveACP(t *testing.T, mp provider.Provider) ( //nolint:nonamedreturns // names document the teardown triple
	cliW *io.PipeWriter, cliR io.Reader, stop func(),
) {
	t.Helper()

	runner := runtime.NewRunner(&runtime.RunnerConfig{
		Bus:     event.NewBus(),
		Profile: profile.Profile{Name: "test", System: []profile.TextBlock{{Type: wireBlockText, Text: "test agent"}}},
		WorkDir: t.TempDir(),
		MaxConc: 2,
		MakeProvider: func(_ provider.RequestCapturer) provider.Provider { return mp },
	})

	srvInR, cliW := io.Pipe()
	cliR, srvOutW := io.Pipe()

	srv := acp.NewServer(srvInR, srvOutW, &bytes.Buffer{}, acp.WithTurnRunner(kitTurnAdapter{runner: runner}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { _ = srv.Serve(ctx); close(done) }()

	stop = func() {
		cancel()

		_ = cliW.Close()
		_ = srvOutW.Close()
		_ = srvInR.Close()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}

	return cliW, cliR, stop
}

// wireSendFrame writes one ACP frame to w.
func wireSendFrame(t *testing.T, w io.Writer, m *acp.Message) {
	t.Helper()

	var buf bytes.Buffer

	err := wireWriteFrameDirect(&buf, m)
	if err != nil {
		t.Fatalf("writeFrame: %v", err)
	}

	_, _ = w.Write(buf.Bytes())
}

// wireWriteFrameDirect mirrors acp.writeFrame (unexported); defined here to
// drive the server from the test without exporting internals.
func wireWriteFrameDirect(buf *bytes.Buffer, m *acp.Message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	buf.Write(raw)
	buf.WriteByte('\n')

	return nil
}

// wireReadFrames reads up to n frames from cliR, returning them.
func wireReadFrames(t *testing.T, cliR io.Reader, n int) []*acp.Message {
	t.Helper()

	br := bufio.NewReader(cliR)

	var out []*acp.Message

	deadline := time.After(3 * time.Second)

	for len(out) < n {
		select {
		default:
		case <-deadline:
			return out
		}

		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			return out
		}

		line = bytes.TrimRight(line, "\n")
		if len(line) == 0 {
			continue
		}

		var m acp.Message

		jerr := json.Unmarshal(line, &m)
		if jerr == nil {
			out = append(out, &m)
		}
	}

	return out
}

// wireRawJSON marshals m to json.RawMessage.
func wireRawJSON(m map[string]any) json.RawMessage {
	b, marshalErr := json.Marshal(m)
	if marshalErr != nil {
		panic(marshalErr)
	}

	return b
}

// wireReadResultFrames reads frames until n REQUEST RESPONSES (frames
// carrying an id) arrive — session/update notifications emitted ahead of
// responses are skipped (20-01: session/new now precedes its response with
// the available_commands_update advertisement).
func wireReadResultFrames(t *testing.T, cliR io.Reader, n int) []*acp.Message {
	t.Helper()

	br := bufio.NewReader(cliR)

	var out []*acp.Message

	deadline := time.After(3 * time.Second)

	for len(out) < n {
		select {
		default:
		case <-deadline:
			return out
		}

		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			return out
		}

		line = bytes.TrimRight(line, "\n")
		if len(line) == 0 {
			continue
		}

		var msg acp.Message
		if jerr := json.Unmarshal(line, &msg); jerr != nil {
			continue
		}

		if msg.ID == nil {
			continue // a notification — not a response; keep reading
		}

		out = append(out, &msg)
	}

	return out
}

// wireReadResultFramesCounting reads n request responses, returning them
// plus the count of session/update notifications skipped ahead of them
// (20-01: session start's available_commands_update advertisement is a REAL
// emitter-written notification — WrittenNotifications accounting must
// include it even when a test's own update collection starts later).
func wireReadResultFramesCounting(t *testing.T, cliR io.Reader, n int) ([]*acp.Message, int) {
	t.Helper()

	br := bufio.NewReader(cliR)

	var out []*acp.Message

	skipped := 0

	deadline := time.After(3 * time.Second)

	for len(out) < n {
		select {
		default:
		case <-deadline:
			return out, skipped
		}

		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			return out, skipped
		}

		line = bytes.TrimRight(line, "\n")
		if len(line) == 0 {
			continue
		}

		var msg acp.Message
		if jerr := json.Unmarshal(line, &msg); jerr != nil {
			continue
		}

		if msg.ID == nil {
			if msg.Method == wireSessionUpdate {
				skipped++
			}

			continue
		}

		out = append(out, &msg)
	}

	return out, skipped
}

// wireErrNotUsed is the Send-not-armed sentinel the scripted providers use.
var wireErrNotUsed = errors.New("not used")

// wireComposeRunner builds a Runner through the SAME composition statements
// acpserve.Run uses (toolkit + catalog + engine setup + renderer + launcher +
// perm store), around a scripted provider over a temp dir carrying the opsx
// command fixtures — the moved wire batteries' shared construction. Returns
// the runner + its workDir (transcript reads ride the on-disk truth). Engine
// setup follows the serve degradation contract (a failure never fails the
// test composition; the batteries that pin the engine assert on its output).
func wireComposeRunner(
	t *testing.T, mp provider.Provider, askTimeout time.Duration,
) (*runtime.Runner, string) {
	t.Helper()

	dir := t.TempDir()

	wireWriteOpsxCommandFixtures(t, dir)

	toolkit := &sessionToolkit{}

	r := runtime.NewRunner(&runtime.RunnerConfig{
		Bus:        event.NewBus(),
		Profile:    profile.Profile{Name: "test", System: []profile.TextBlock{{Type: wireBlockText, Text: "you are a test agent"}}},
		WorkDir:    dir,
		MaxConc:    2,
		AskTimeout: askTimeout,
		Toolkit:    toolkit,
		// 25-07 (OQ1): the ask-surface renderer func-field — the same
		// coreexec.RenderAskSurface the serve composition wires.
		AskSurfaceRenderer: coreexec.RenderAskSurface,
		LaunchBackground:   toolkit.LaunchBackground,
		OpenPermStore:      openPermAuthority,
		MakeProvider: func(_ provider.RequestCapturer) provider.Provider { return mp },
	})
	r.SetCatalog(loadCommandCatalog(dir))

	setup, serr := loadEngineSetup(dir)
	if serr != nil {
		t.Fatalf("engine setup: %v", serr)
	}

	if err := r.SetupEngine(setup); err != nil {
		t.Fatalf("SetupEngine: %v", err)
	}

	return r, dir
}

// wireWriteOpsxCommandFixtures plants the opsx slash-command fixture tree the
// expansion batteries expand against (the kit-side runner_battery original;
// discovery here is the REAL loadCommandCatalog over the planted files).
func wireWriteOpsxCommandFixtures(t *testing.T, dir string) {
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

// wireNoRedact is the read-side no-op redactor (the moved batteries only READ
// transcripts back; the writing Manager already redacted at write time).
type wireNoRedact struct{}

func (wireNoRedact) Redact(line []byte) ([]byte, error) { return line, nil }
func (wireNoRedact) ScrubError(err error) string        { return err.Error() }

// wireTranscriptLines reads a session's transcript back from disk (the
// kit-side batteries read r.sessions[sid].Manager in-package; the moved
// batteries ride the on-disk truth — the same file, the same session.Line
// stream, one level lower).
func wireTranscriptLines(t *testing.T, workDir, sessionID string) []session.Line {
	t.Helper()

	mgr, err := session.NewManager(workDir, sessionID, wireNoRedact{})
	if err != nil {
		t.Fatalf("transcript open %s: %v", sessionID, err)
	}

	defer func() { _ = mgr.Close() }()

	lines, err := mgr.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	return lines
}

// opsxNoEmitter is the moved E2E harnesses' no-op kit Emitter (headless
// product-proof turns stream nothing; transcript + audit are the lenses).
type opsxNoEmitter struct{}

func (opsxNoEmitter) Emit(context.Context, event.Event) error { return nil }

// wireTranscriptLinesT is wireTranscriptLines' error-returning twin (the
// evalharness RunnerSeam speaks (lines, error)).
func wireTranscriptLinesT(workDir, sessionID string) ([]session.Line, error) {
	mgr, err := session.NewManager(workDir, sessionID, wireNoRedact{})
	if err != nil {
		return nil, fmt.Errorf("e2e transcript open: %w", err)
	}

	defer func() { _ = mgr.Close() }()

	lines, err := mgr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("e2e transcript: %w", err)
	}

	return lines, nil
}

// wireFindRepoRoot walks up from the test cwd until a directory carrying
// both .ass-guard/config.yaml (provider creds) and profiles/ (the zcode
// bundle) is found (the moved gated E2E's credential locator).
func wireFindRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for range 16 {
		if wireFileExists(filepath.Join(dir, ".ass-guard", "config.yaml")) &&
			wireFileExists(filepath.Join(dir, "profiles")) {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	t.Fatal("BLOCKER: cannot locate the repo root (provider creds .ass-guard/config.yaml + profiles/) — " +
		"the E2E needs the real model; run from a checkout that has them")

	return ""
}

// wireFileExists reports whether path exists.
func wireFileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
