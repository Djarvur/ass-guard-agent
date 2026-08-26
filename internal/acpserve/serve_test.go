package acpserve //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

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

	err := Run(ctx, in, &stdout, &stderr, &Options{
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

	_ = Run(ctx, in, &stdout, &stderr, &Options{
		Profile: "zcode", MaxConcurrent: 6,
		ProfilesDir: repoProfilesDir(t), WorkDir: t.TempDir(),
	})
	if strings.Contains(stdout.String(), "ass-guard/acp") {
		t.Errorf("stdout contains a log prefix (transport discipline violation): %s", stdout.String())
	}
}

// --- 09-01 T3: the serve-seam audit proofs (moved from cmd with the Run
// composition in 15-06) ---

// TestServeAudit_RequestShapedThroughRealSeam (09-01 T3 Tests 10-12, AUD-02 —
// the Pitfall-8 closer): an in-process Run drives initialize →
// session/new → session/prompt against a REAL temp .ass-guard/config.yaml
// (anthropic shape pointing at an httptest SSE stub, synthetic canary key), so
// the provider is constructed through setupProviderFactory →
// BuildWithCapturer exactly as production. Asserts: (1) the per-session
// transcript carries a request_shaped line with non-empty TurnID + the loaded
// profile name, through the REAL seam; (2) the canary key value appears NOWHERE
// in the transcript (redaction chokepoint); (3) stdout carries only JSON-RPC
// frames.
func TestServeAudit_RequestShapedThroughRealSeam(t *testing.T) { //nolint:funlen // end-to-end audit proof
	t.Setenv("ZAI_API_KEY", "") // force the config literal (canary) to win

	const canaryKey = "sk-test-canary-0123456789abcdef"

	srv := serveAuditSSEStub()
	defer srv.Close()

	workDir := serveAuditWorkDir(t, srv.URL, canaryKey)

	// syncBuffer: the serve goroutine writes frames while this test polls —
	// bytes.Buffer is not concurrency-safe, so guard it.
	stdout := &syncBuffer{}
	stderr := &syncBuffer{}

	//nolint:modernize,testingcontext // explicit cancel before the pipe close
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inPipeR, inPipeW := io.Pipe()

	go func() {
		_ = Run(ctx, inPipeR, stdout, stderr, &Options{
			Profile: profileZcode, MaxConcurrent: 2,
			ProfilesDir: repoProfilesDir(t), WorkDir: workDir,
		})
	}()

	writeFrame := func(line string) {
		_, werr := inPipeW.Write([]byte(line + "\n"))
		if werr != nil {
			t.Fatalf("write frame: %v", werr)
		}
	}

	writeFrame(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1}}`)
	writeFrame(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"` + workDir + `"}}`)

	// Poll stdout for the sessionId (the response to id 1).
	sessionID := pollStdoutForSessionID(t, stdout)

	writeFrame(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"` +
		sessionID + `","prompt":[{"type":"text","text":"hi"}]}}`)

	// Poll the transcript FILE (the artifact is the deliverable).
	shaped := pollTranscriptRequestShaped(t, workDir, sessionID, stderr.String())

	if shaped[0].TurnID == "" {
		t.Errorf("request_shaped.TurnID is empty; want real turn attribution")
	}

	if shaped[0].Profile != profileZcode {
		t.Errorf("request_shaped.Profile = %q; want %q", shaped[0].Profile, profileZcode)
	}

	// 09-05: the line is METADATA-ONLY — ref + fingerprint present, and the
	// marshaled line stays far under 1 KiB even for ~80 KB-class bodies.
	if shaped[0].Ref == "" {
		t.Error("request_shaped.Ref is empty; want the body-store ref (correlation request leg)")
	}

	if shaped[0].Model == "" || shaped[0].Bytes == 0 {
		t.Errorf("fingerprint missing: model=%q bytes=%d", shaped[0].Model, shaped[0].Bytes)
	}

	// Redaction canary (Pitfall 9 / T-9-01): the key VALUE must be absent from
	// the whole transcript AND the body store while the request line exists.
	rawAll, rerr := os.ReadFile(filepath.Join(workDir, ".ass-guard", "transcript_"+sessionID+".jsonl"))
	if rerr != nil {
		t.Fatalf("read transcript: %v", rerr)
	}

	if strings.Contains(string(rawAll), canaryKey) {
		t.Errorf("canary key leaked into the transcript (redaction chokepoint failed)")
	}

	assertBodyStoreRoundTrip(t, workDir, shaped[0].Ref, canaryKey)

	// 09-06 T2 Test 9 (default mirror ON): the per-session mirror file exists
	// under .ass-guard/audit/ carrying the request line (with header NAMES,
	// never values) + T3 Test 12/13 (the secret canary over EVERY artifact +
	// artifact-presence positive controls).
	assertMirrorAndCanary(t, workDir, sessionID, canaryKey)

	// Transport discipline (T-9-04): stdout carries only JSON-RPC frames.
	assertStdoutOnlyJSONFrames(t, stdout)
}

// serveAuditSSEStub replays a minimal valid Anthropic SSE stream (the fixture
// shape from internal/provider tests: message_start + text block + end_turn).
func serveAuditSSEStub() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		for _, frame := range []string{
			`{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", frame)

			if flusher != nil {
				flusher.Flush()
			}
		}

		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

// serveAuditWorkDir builds a temp workdir whose REAL .ass-guard/config.yaml
// points the anthropic provider at stubURL with the synthetic canary key.
func serveAuditWorkDir(t *testing.T, stubURL, canaryKey string) string {
	t.Helper()

	workDir := t.TempDir()

	err := os.MkdirAll(filepath.Join(workDir, ".ass-guard"), 0o750)
	if err != nil {
		t.Fatalf("mkdir .ass-guard: %v", err)
	}

	sched := fmt.Sprintf("providers:\n  anthropic:\n    base_url: %q\n    api_key: %q\n", stubURL, canaryKey)

	err = os.WriteFile(filepath.Join(workDir, ".ass-guard", "config.yaml"), []byte(sched), 0o600)
	if err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	return workDir
}

// pollTranscriptRequestShaped polls the transcript FILE until a request_shaped
// line exists (the artifact — not the bus — is the deliverable).
func pollTranscriptRequestShaped(t *testing.T, workDir, sessionID, stderrSnapshot string) []session.Line {
	t.Helper()

	transcriptPath := filepath.Join(workDir, ".ass-guard", "transcript_"+sessionID+".jsonl")

	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		var shaped []session.Line

		raw, rerr := os.ReadFile(transcriptPath)
		if rerr == nil {
			for line := range strings.SplitSeq(string(raw), "\n") {
				if line == "" {
					continue
				}

				var l session.Line

				if json.Unmarshal([]byte(line), &l) == nil && l.Type == "request_shaped" {
					shaped = append(shaped, l)
				}
			}
		}

		if len(shaped) > 0 {
			return shaped
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("no request_shaped line in %s within 10s (stderr: %s)", transcriptPath, stderrSnapshot)

	return nil
}

// assertStdoutOnlyJSONFrames pins the transport discipline: every non-empty
// stdout line parses as a JSON-RPC frame.
func assertStdoutOnlyJSONFrames(t *testing.T, stdout *syncBuffer) {
	t.Helper()

	i := 0

	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if line == "" {
			i++

			continue
		}

		var m map[string]any

		jerr := json.Unmarshal([]byte(line), &m)
		if jerr != nil {
			t.Errorf("stdout line %d is not valid JSON: %v (%q)", i, jerr, line)
		}

		i++
	}
}

// pollStdoutForSessionID waits for the session/new response and extracts the
// sessionId. The buffer is only appended to by the serve goroutine; reads here
// are locked snapshot reads of the accumulated prefix.
func pollStdoutForSessionID(t *testing.T, stdout *syncBuffer) string {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		for line := range strings.SplitSeq(stdout.String(), "\n") {
			if !strings.Contains(line, `"sessionId"`) {
				continue
			}

			var resp struct {
				Result struct {
					SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
				} `json:"result"`
			}

			if json.Unmarshal([]byte(line), &resp) == nil && resp.Result.SessionID != "" {
				return resp.Result.SessionID
			}
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("no session/new response within 10s (stdout so far: %s)", stdout.String())

	return ""
}

// syncBuffer is a mutex-guarded bytes.Buffer for reading a live writer from
// the test goroutine while the serve goroutine appends frames.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p) //nolint:wrapcheck // test helper passthrough
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// assertBodyStoreRoundTrip (09-05 T2 Test 8 + the store canary): the body is
// retrievable by the line's ref, carries no secret, keeps non-secret content.
func assertBodyStoreRoundTrip(t *testing.T, workDir, ref, canaryKey string) {
	t.Helper()

	store := audit.NewBodyStore(filepath.Join(workDir, ".ass-guard", "audit", "bodies"), 0)

	body, gerr := store.Get(ref)
	if gerr != nil {
		t.Fatalf("body retrievable by the line's ref (Test 8): %v", gerr)
	}

	if strings.Contains(string(body), canaryKey) {
		t.Error("canary key leaked into the STORED body (redact-before-store failed)")
	}

	if !strings.Contains(string(body), "GLM-5.3") {
		t.Errorf("stored body lost non-secret content: %.80s", string(body))
	}
}

// assertMirrorAndCanary (09-06 T2 Test 8/9 + T3 Tests 12-13): the per-session
// mirror exists non-empty with the request line's header NAMES (values never);
// the FULL .ass-guard tree + the syncBuffer stderr carry ZERO canary bytes.
func assertMirrorAndCanary(t *testing.T, workDir, sessionID, canaryKey string) {
	t.Helper()

	mirrorPath := filepath.Join(workDir, ".ass-guard", "audit", sessionID+".jsonl")

	raw, err := os.ReadFile(mirrorPath)
	if err != nil {
		t.Fatalf("default per-session mirror missing (Test 9): %v", err)
	}

	if len(raw) == 0 {
		t.Fatal("mirror file empty")
	}

	if !strings.Contains(string(raw), `"kind":"request_shaped"`) {
		t.Errorf("mirror lacks the request line:\n%s", string(raw)[:min(200, len(raw))])
	}

	if !strings.Contains(string(raw), "headerNames") {
		t.Error("mirror request line lacks headerNames (Test 8: names must flow)")
	}

	// Test 12 — the canary: zero occurrences across the ARTIFACT tree
	// (.ass-guard/audit — mirror + body store) and the transcript file. The
	// operator's config.yaml is the credential SOURCE, not an artifact;
	// scanning it would trivially contain the key by construction.
	scanOne := func(path string) {
		content, rerr := os.ReadFile(path)
		if rerr != nil {
			return // absent artifacts have their own positive controls
		}

		if bytes.Contains(content, []byte(canaryKey)) {
			t.Errorf("canary leaked into %s", path)
		}
	}

	auditRoot := filepath.Join(workDir, ".ass-guard", "audit")

	walkErr := filepath.WalkDir(auditRoot, func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return werr
		}

		content, rerr := os.ReadFile(path)
		if rerr != nil {
			return fmt.Errorf("canary read %s: %w", path, rerr)
		}

		if bytes.Contains(content, []byte(canaryKey)) {
			t.Errorf("canary leaked into %s", path)
		}

		return nil
	})
	if walkErr != nil {
		t.Fatalf("canary walk: %v", walkErr)
	}

	scanOne(filepath.Join(workDir, ".ass-guard", "transcript_"+sessionID+".jsonl"))

	// Test 13 — positive controls (absence is not vacuity).
	assertCanaryPositiveControls(t, workDir, sessionID)
}

// assertCanaryPositiveControls: transcript request line, stored body, and
// mirror line all exist non-empty — the canary proves redaction, not
// absence-by-nothing-written.
func assertCanaryPositiveControls(t *testing.T, workDir, sessionID string) {
	t.Helper()

	transcript := filepath.Join(workDir, ".ass-guard", "transcript_"+sessionID+".jsonl")

	tRaw, terr := os.ReadFile(transcript)
	if terr != nil || len(tRaw) == 0 {
		t.Errorf("transcript missing/empty (positive control): %v", terr)
	}

	bodiesDir := filepath.Join(workDir, ".ass-guard", "audit", "bodies")

	entries, derr := os.ReadDir(bodiesDir)
	if derr != nil || len(entries) == 0 {
		t.Errorf("body store empty (positive control): %v", derr)
	}
}

// driveServeFrames writes the initialize/session-new/session-prompt frame
// sequence over the serve pipe and returns the session id.
func driveServeFrames(t *testing.T, inPipeW *io.PipeWriter, stdout *syncBuffer, workDir string) string {
	t.Helper()

	write := func(line string) {
		_, werr := inPipeW.Write([]byte(line + "\n"))
		if werr != nil {
			t.Fatalf("write frame: %v", werr)
		}
	}

	write(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1}}`)
	write(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"` + workDir + `"}}`)

	sessionID := pollStdoutForSessionID(t, stdout)

	write(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"` +
		sessionID + `","prompt":[{"type":"text","text":"hi"}]}}`)

	return sessionID
}

// TestServeMirror_Override (09-06 T2 Test 10): with AuditLogPath set, every
// session's lines land in the ONE operator file and NO per-session file
// appears under .ass-guard/audit/.
func TestServeMirror_Override(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "")

	srv := serveAuditSSEStub()
	defer srv.Close()

	workDir := serveAuditWorkDir(t, srv.URL, "sk-override-canary-xyz")

	overridePath := filepath.Join(t.TempDir(), "operator-audit.jsonl")

	stdout := &syncBuffer{}
	stderr := &syncBuffer{}

	//nolint:modernize,testingcontext // explicit cancel before pipe close
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inPipeR, inPipeW := io.Pipe()

	go func() {
		_ = Run(ctx, inPipeR, stdout, stderr, &Options{
			Profile: profileZcode, MaxConcurrent: 2,
			ProfilesDir: repoProfilesDir(t), WorkDir: workDir,
			AuditLogPath: overridePath,
		})
	}()

	sessionID := driveServeFrames(t, inPipeW, stdout, workDir)

	deadline := time.Now().Add(10 * time.Second)

	// Wait for the turn's TERMINAL transcript line, not just the mid-turn
	// request_shaped: the 14-01 turn-entry checkpoint lengthens the turn, and
	// returning at request_shaped left the turn's async writers (audit
	// body-store shards) racing t.TempDir's RemoveAll — a flake measured at
	// 2/10 with the checkpoint wiring (baseline 0/10). The mirror never
	// carries assistant_message (it subscribes request_shaped/
	// engine_decision/usage only), so turn completion is observed on the
	// SESSION TRANSCRIPT; the drain sleep lets the async TranscriptWriter
	// finish (the TestEndToEndSession flush pattern).
	transcriptPath := filepath.Join(workDir, ".ass-guard", "transcript_"+sessionID+".jsonl")

	for time.Now().Before(deadline) {
		raw, rerr := os.ReadFile(overridePath)
		traw, terr := os.ReadFile(transcriptPath)

		if rerr == nil && strings.Contains(string(raw), `"kind":"request_shaped"`) &&
			terr == nil && strings.Contains(string(traw), `"type":"assistant_message"`) {
			// the override file has the line; the per-session default must NOT exist
			_, perr := os.Stat(filepath.Join(workDir, ".ass-guard", "audit", sessionID+".jsonl"))
			if perr == nil {
				t.Fatal("per-session mirror file exists despite the override (Test 10)")
			}

			if strings.Contains(string(raw), "sk-override-canary-xyz") {
				t.Error("canary leaked into the override mirror")
			}

			time.Sleep(150 * time.Millisecond) // async writer drain (house flush pattern)

			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("override mirror did not land a request line within 10s (stderr: %s)", stderr.String())
}
