package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// sseHandler writes a sequence of Anthropic-style SSE frames then closes. Each
// frame is a `data: <json>\n\n` block; the last is the terminal [DONE]-equivalent
// (message_stop) carrying the stop reason via message_delta.
func sseHandler(frames ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")

		flusher, _ := w.(http.Flusher)
		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)

			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// readAllChunks drains a Stream channel, returning the chunks. Fails the test if
// the channel doesn't close within the timeout.
func readAllChunks(t *testing.T, ch <-chan provider.StreamChunk) []provider.StreamChunk {
	t.Helper()

	var out []provider.StreamChunk

	deadline := time.After(3 * time.Second)

	for {
		select {
		case c, ok := <-ch:
			if !ok {
				return out
			}

			out = append(out, c)
		case <-deadline:
			t.Fatalf("stream channel did not close within 3s; got %d chunks", len(out))

			return out
		}
	}
}

// TestStream_EmitsTextChunks verifies Stream reads SSE text deltas and emits
// them as text chunks, then a done chunk carrying the FinishReason (ACP-04).
func TestStream_EmitsTextChunks(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(sseHandler(
		`{"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		`{"type":"message_stop"}`,
	))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	ch, err := p.Stream(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	chunks := readAllChunks(t, ch)

	var (
		text     string
		finish   string
		sawUsage bool
	)

	for _, c := range chunks {
		switch c.Type {
		case "text":
			text += c.Text
		case "usage":
			sawUsage = true
		case "done":
			finish = c.FinishReason
		}
	}

	if text != "Hello world" {
		t.Errorf("streamed text = %q; want %q", text, "Hello world")
	}

	if finish != "end_turn" {
		t.Errorf("FinishReason = %q; want end_turn", finish)
	}

	if !sawUsage {
		t.Error("no usage chunk observed")
	}
}

// TestStream_ToolUse verifies a tool_use content block delivers a tool_use chunk
// with a populated ToolCall.
func TestStream_ToolUse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(sseHandler(
		`{"type":"content_block_start","index":0,`+
			`"content_block":{"type":"tool_use","id":"call_1","name":"Bash","input":{}}}`,
		`{"type":"content_block_delta","index":0,`+
			`"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		`{"type":"message_stop"}`,
	))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	ch, err := p.Stream(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "run ls"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	chunks := readAllChunks(t, ch)

	var (
		finish   string
		toolCall *provider.ToolCall
	)

	for _, c := range chunks {
		if c.Type == blockToolUse && c.ToolCall != nil {
			toolCall = c.ToolCall
		}

		if c.Type == "done" {
			finish = c.FinishReason
		}
	}

	if toolCall == nil {
		t.Fatal("no tool_use chunk with a populated ToolCall")
	}

	if toolCall.Name != "Bash" {
		t.Errorf("ToolCall.Name = %q; want Bash", toolCall.Name)
	}

	if finish != blockToolUse {
		t.Errorf("FinishReason = %q; want tool_use", finish)
	}
}

// TestStream_RespectsCancel verifies cancelling ctx mid-stream closes the channel
// and aborts the in-flight HTTP request.
func TestStream_RespectsCancel(t *testing.T) {
	t.Parallel()
	// A server that writes one chunk then blocks forever (until the request is
	// cancelled by the ctx propagation).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n",
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"first"}}`)

		if flusher != nil {
			flusher.Flush()
		}
		// Block until the request ctx is cancelled (client gave up).
		<-r.Context().Done()
	}))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := p.Stream(ctx, &prof, []shaper.Message{{Role: roleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// Read the first chunk, then cancel.
	select {
	case <-ch:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("no first chunk within 2s")
	}
	// Channel must close promptly after cancel.
	deadline := time.After(2 * time.Second)

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // good: closed
			}
		case <-deadline:
			t.Fatal("channel did not close within 2s of cancel")
		}
	}
}

// errorEnvelopeHandler answers with the given status + body — the non-SSE shape
// a rejected request gets (an Anthropic JSON error envelope, not a stream).
func errorEnvelopeHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}
}

// streamAgainstHandler runs one Stream call against a scripted server and
// drains the channel (the non-2xx battery's harness).
func streamAgainstHandler(t *testing.T, h http.HandlerFunc) []provider.StreamChunk {
	t.Helper()

	srv := httptest.NewServer(h)
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	ch, err := p.Stream(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	return readAllChunks(t, ch)
}

// TestStream_Non2xxError pins Pitfall 1 (19-RESEARCH): a 400 rejection — the
// Anthropic error envelope, plain JSON, no SSE frames — must surface as an
// ERROR chunk the turn loop already consumes (session.go chunkErrorType case),
// never as a done chunk with a defaulted end_turn finish (the drain path skips
// every non-data: line and sendDone defaults the empty reason — the silent
// swallow this test forbids). The body is read and closed at the status-check
// site; the drain goroutine never starts, so the channel closes after exactly
// the one error chunk.
func TestStream_Non2xxError(t *testing.T) {
	t.Parallel()

	chunks := streamAgainstHandler(t, errorEnvelopeHandler(http.StatusBadRequest,
		`{"type":"error","error":{"type":"invalid_request_error",`+
			`"message":"prompt is too long: 200936 tokens > 199999 maximum"}}`))

	var perr *provider.ProviderError

	for _, c := range chunks {
		if c.Type == "done" {
			t.Errorf("done chunk %q observed on a rejected request; want none (Pitfall 1 swallow)",
				c.FinishReason)
		}

		if c.Type != "error" {
			t.Errorf("chunk Type = %q; want only \"error\" (chunk: %+v)", c.Type, c)

			continue
		}

		if perr != nil {
			t.Error("more than one error chunk observed; want exactly one")

			continue
		}

		if c.Error == nil {
			t.Fatal("error chunk carries nil Error")

			continue
		}

		if !errors.As(c.Error, &perr) {
			t.Fatalf("error chunk Error = %v; want a *ProviderError (classified via ClassifyHTTP)", c.Error)
		}
	}

	if perr == nil {
		t.Fatal("no error chunk observed; the 400 was swallowed (Pitfall 1)")
	}

	if perr.Kind != provider.KindStructural {
		t.Errorf("Kind = %q; want %q (400 lands in KindStructural via structuralStatuses)",
			perr.Kind, provider.KindStructural)
	}

	if perr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d; want 400", perr.StatusCode)
	}

	// The overflow message class must survive the surfacing (19-02 Task 2's
	// IsOverflow matches over this text — research assumption A1's wording).
	if !strings.Contains(perr.Error(), "prompt is too long") {
		t.Errorf("ProviderError message = %q; want it to carry the provider's \"prompt is too long\" text",
			perr.Error())
	}
}

// TestStream_Non2xxError_MalformedBody pins the robustness prohibition: an
// unparseable, truncated, or empty error body still yields the error chunk
// carrying the status code — the envelope parser never panics and degrades to
// the generic structural ProviderError (T-19-03).
func TestStream_Non2xxError_MalformedBody(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"truncated json": `{"type":"error","error":{"type":"invalid_re`,
		"empty body":     ``,
		"wrong shape":    `{"unexpected":[1,2,3]}`,
	} {
		chunks := streamAgainstHandler(t, errorEnvelopeHandler(http.StatusBadRequest, body))

		var perr *provider.ProviderError

		for _, c := range chunks {
			if c.Type == "done" {
				t.Errorf("%s: done chunk %q observed; want none", name, c.FinishReason)
			}

			if c.Type == "error" && c.Error != nil && perr == nil {
				if !errors.As(c.Error, &perr) {
					t.Errorf("%s: error chunk Error = %v; want a *ProviderError", name, c.Error)
				}
			}
		}

		if perr == nil {
			t.Errorf("%s: no error chunk observed for body %q", name, body)

			continue
		}

		if perr.Kind != provider.KindStructural || perr.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: classified %+v; want Structural/400", name, perr)
		}
	}
}

// TestStream_NoAPIKey verifies Stream surfaces a clear error when no key is set
// (parity with Send).
func TestStream_NoAPIKey(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "")
	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicBaseURL("http://must-not-be-called.invalid"),
	)

	_, err := p.Stream(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "x"}})
	if err == nil {
		t.Fatal("Stream returned nil error with no API key; want non-nil")
	}
}

// stalledSSEHandler writes one text frame, flushes it, then goes SILENT holding
// the connection open — the live Z.ai stall form (frozen mid-sentence, zero
// chunks, zero errors, the connection stays open until forced). The handler
// deliberately does NOT watch r.Context(): a server that never notices the
// client's cancel is the wedge the 08-09 finding documented.
func stalledSSEHandler() (http.HandlerFunc, chan struct{}) { //nolint:gocritic // conflicts w/ nonamedreturns (house)
	release := make(chan struct{})

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")

		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n",
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"first"}}`)

		if flusher != nil {
			flusher.Flush()
		}

		<-release // hold the connection open in silence
	}, release
}

// TestStream_IdleWatchdogAbortsStalledStream pins the SSE-stall fix (STATE.md
// finding 2026-08-15): a server that freezes mid-stream must NOT wedge the
// stream forever — the idle watchdog aborts the body and the channel carries an
// "error" chunk (retryable) then closes, within a bounded window.
func TestStream_IdleWatchdogAbortsStalledStream(t *testing.T) {
	t.Parallel()

	handler, release := stalledSSEHandler()

	srv := httptest.NewServer(handler)
	defer srv.Close()
	defer close(release)

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
		provider.WithAnthropicIdleTimeout(400*time.Millisecond),
	)

	ch, err := p.Stream(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var sawAbortError bool

	deadline := time.After(5 * time.Second)

	for {
		select {
		case c, ok := <-ch:
			if !ok {
				if !sawAbortError {
					t.Error("channel closed without an abort error chunk — the stall was swallowed as a clean end")
				}

				return
			}

			if c.Type == "error" {
				sawAbortError = true

				var perr *provider.ProviderError
				if !errors.As(c.Error, &perr) || perr.Kind != provider.KindTransient {
					t.Errorf("abort error = %v; want a Transient ProviderError (retryable)", c.Error)
				}
			}
		case <-deadline:
			t.Fatal("stalled stream did not abort within 5s — the idle watchdog is missing " +
				"(the drain blocks in ReadString forever)")
		}
	}
}

// TestStream_SendSurfacesIdleTimeoutAsError pins the Send-level contract of the
// same fix: a stalled stream makes Send RETURN the retryable error instead of
// fabricating a completed Response with a truncated body.
func TestStream_SendSurfacesIdleTimeoutAsError(t *testing.T) {
	t.Parallel()

	handler, release := stalledSSEHandler()

	srv := httptest.NewServer(handler)
	defer srv.Close()
	defer close(release)

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
		provider.WithAnthropicIdleTimeout(400*time.Millisecond),
	)

	type sendResult struct {
		resp provider.Response
		err  error
	}

	res := make(chan sendResult, 1)

	go func() {
		resp, err := p.Send(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "hi"}})
		res <- sendResult{resp, err}
	}()

	select {
	case r := <-res:
		if r.err == nil {
			t.Fatalf("Send returned nil error on a stalled stream (resp=%+v); want the retryable abort error", r.resp)
		}

		var perr *provider.ProviderError
		if !errors.As(r.err, &perr) || perr.Kind != provider.KindTransient {
			t.Errorf("Send error = %v; want a Transient ProviderError (retryable)", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send did not return within 5s on a stalled stream — the drain blocks forever")
	}
}

// TestStream_CancelUnblocksStalledBodyRead pins the ctx half of the SSE-stall
// fix: cancelling the request context must unblock a body read that is wedged
// on a silent-but-open connection (the live evidence: 6+ minutes past the ctx
// deadline with zero unblocking). The channel must close within the bound.
func TestStream_CancelUnblocksStalledBodyRead(t *testing.T) {
	t.Parallel()

	handler, release := stalledSSEHandler()

	srv := httptest.NewServer(handler)
	defer srv.Close()
	defer close(release)

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := p.Stream(ctx, &prof, []shaper.Message{{Role: roleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// Read the first chunk, then cancel mid-stall.
	select {
	case <-ch:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("no first chunk within 2s")
	}

	deadline := time.After(3 * time.Second)

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // good: closed
			}
		case <-deadline:
			t.Fatal("channel did not close within 3s of cancelling a stalled stream — " +
				"the blocked Read was not force-unblocked")
		}
	}
}

// TestStream_ToolUseReplayedBlockEmitsOnce pins the duplicate-tool_use finding
// (STATE.md, pre-existing since 08-07): the Z.ai Anthropic-compat endpoint
// REPLAYS each tool_use block (content_block_start → input_json_delta →
// content_block_stop delivered TWICE — the live transcripts show every call id
// exactly 2x, µs apart). The stream must emit exactly ONE tool_use chunk per
// provider id: request-shape fidelity (zcode batches carry unique ids) and
// half the payload growth that correlates with the SSE stall.
func TestStream_ToolUseReplayedBlockEmitsOnce(t *testing.T) {
	t.Parallel()

	const (
		frameStart = `{"type":"content_block_start","index":0,` +
			`"content_block":{"type":"tool_use","id":"call_dup","name":"Bash","input":{}}}`
		frameDelta = `{"type":"content_block_delta","index":0,` +
			`"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}`
		frameStop = `{"type":"content_block_stop","index":0}`
	)

	srv := httptest.NewServer(sseHandler(
		frameStart, frameDelta, frameStop,
		frameStart, frameDelta, frameStop, // the replayed block
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		`{"type":"message_stop"}`,
	))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	ch, err := p.Stream(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "run ls"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	emitted := 0

	for _, c := range readAllChunks(t, ch) {
		if c.Type == blockToolUse && c.ToolCall != nil && c.ToolCall.ID == "call_dup" {
			emitted++
		}
	}

	if emitted != 1 {
		t.Errorf("tool_use chunks for call_dup = %d; want exactly 1 (replayed block must collapse)", emitted)
	}
}

// TestSend_ToolUseReplayedBlockSingleCall pins the Send-level form of the same
// finding: the assembled Response.ToolCalls carries each call exactly once
// (the 08-08/08-09 transcripts carried 152 lines / 76 unique ids).
func TestSend_ToolUseReplayedBlockSingleCall(t *testing.T) {
	t.Parallel()

	const (
		frameStart = `{"type":"content_block_start","index":0,` +
			`"content_block":{"type":"tool_use","id":"call_dup2","name":"Bash","input":{}}}`
		frameDelta = `{"type":"content_block_delta","index":0,` +
			`"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"pwd\"}"}}`
		frameStop = `{"type":"content_block_stop","index":0}`
	)

	srv := httptest.NewServer(sseHandler(
		frameStart, frameDelta, frameStop,
		frameStart, frameDelta, frameStop,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		`{"type":"message_stop"}`,
	))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	resp, err := p.Send(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "run pwd"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("Response.ToolCalls = %d entries; want exactly 1 (duplicates: %+v)",
			len(resp.ToolCalls), resp.ToolCalls)
	}

	if resp.ToolCalls[0].ID != "call_dup2" {
		t.Errorf("ToolCalls[0].ID = %q; want call_dup2", resp.ToolCalls[0].ID)
	}
}

// ensure profile.Load type is exercised (keeps the test helper honest).
var _ = profile.Profile{}

// thinkingView is the TEST-ONLY minimal unmarshal view of a thinking chunk's
// Raw payload (PAR-05, D-12: the production path never unmarshals provider
// thinking bytes — assertions decode them here, in the test, exclusively).
type thinkingView struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
	Data      string `json:"data"`
}

// decodeThinking unmarshals one thinking chunk's Raw payload (test-only view).
func decodeThinking(t *testing.T, c provider.StreamChunk) thinkingView {
	t.Helper()

	if c.Type != chunkThinking {
		t.Fatalf("chunk Type = %q; want %q", c.Type, chunkThinking)
	}

	var v thinkingView
	if err := json.Unmarshal(c.Raw, &v); err != nil {
		t.Fatalf("unmarshal thinking Raw: %v\nraw: %s", err, c.Raw)
	}

	return v
}

// streamFrames runs one Stream call against a scripted SSE server emitting
// frames and drains the channel (the TestStreamThinking battery's harness).
func streamFrames(t *testing.T, frames ...string) []provider.StreamChunk {
	t.Helper()

	srv := httptest.NewServer(sseHandler(frames...))
	defer srv.Close()

	prof := loadProfile(t, "minimal")
	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
	)

	ch, err := p.Stream(context.Background(), &prof, []shaper.Message{{Role: roleUser, Content: "think"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	return readAllChunks(t, ch)
}

// thinkingChunks filters the thinking-typed chunks out of a drained stream.
func thinkingChunks(chunks []provider.StreamChunk) []provider.StreamChunk {
	var out []provider.StreamChunk

	for _, c := range chunks {
		if c.Type == chunkThinking {
			out = append(out, c)
		}
	}

	return out
}

// TestStreamThinking_SignedBlockAccumulatesDeltas is PAR-05 hop 1's core: a
// content_block_start of type thinking opens the accumulator, thinking_delta
// strings concatenate IN ORDER, the signature_delta is captured, and
// content_block_stop emits ONE thinking chunk whose Raw field values equal the
// delta concatenations exactly (D-12: assembled from raw delta strings, never
// a typed round-trip).
func TestStreamThinking_SignedBlockAccumulatesDeltas(t *testing.T) {
	t.Parallel()

	const sig = "EqQBCkgIBRABGAIiQK1hwdFP7q2OoMkD+vLk="

	chunks := streamFrames(t,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me "}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"consider the tools."}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"`+sig+`"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		`{"type":"message_stop"}`,
	)

	got := thinkingChunks(chunks)
	if len(got) != 1 {
		t.Fatalf("thinking chunks = %d; want exactly 1 (chunks: %+v)", len(got), got)
	}

	v := decodeThinking(t, got[0])
	if v.Thinking != "Let me consider the tools." {
		t.Errorf("field thinking = %q; want the exact delta concatenation", v.Thinking)
	}

	if v.Signature != sig {
		t.Errorf("field signature = %q; want %q (the exact signature_delta string)", v.Signature, sig)
	}

	if v.Type != chunkThinking {
		t.Errorf("field type = %q; want %q", v.Type, chunkThinking)
	}

	if v.Data != "" {
		t.Errorf("field data = %q; want empty (signed blocks carry no data)", v.Data)
	}
}

// TestStreamThinking_RedactedEmitsImmediately pins the redacted_thinking
// short-circuit (RESEARCH Pattern 4 hop 1): the block arrives FULLY-FORMED in
// content_block_start (data field, no deltas, no signature) and must emit
// IMMEDIATELY — filtering thinking by the single type name would drop it.
func TestStreamThinking_RedactedEmitsImmediately(t *testing.T) {
	t.Parallel()

	const blob = "enc-9f2b7c4d-e1aa-4d8eopaquepayload"

	// Deliberately NO content_block_stop and NO deltas after the start: the
	// emission must come from the start event alone.
	chunks := streamFrames(t,
		`{"type":"content_block_start","index":0,`+
			`"content_block":{"type":"redacted_thinking","data":"`+blob+`"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		`{"type":"message_stop"}`,
	)

	got := thinkingChunks(chunks)
	if len(got) != 1 {
		t.Fatalf("thinking chunks = %d; want exactly 1 (immediate emission; chunks: %+v)", len(got), got)
	}

	v := decodeThinking(t, got[0])
	if v.Type != blockRedactedThinking {
		t.Errorf("field type = %q; want %q", v.Type, blockRedactedThinking)
	}

	if v.Data != blob {
		t.Errorf("field data = %q; want the verbatim provider value %q", v.Data, blob)
	}

	if v.Signature != "" {
		t.Errorf("field signature = %q; want empty — redacted blocks carry NO signature; none may be invented",
			v.Signature)
	}
}

// TestStreamThinking_InterleavedWithTextAndToolUse proves the accumulator is
// SIBLING state: a thinking block between text deltas and a tool_use block
// emits all three chunk types in stream order while the existing text and
// tool-use emissions stay byte-identical to their pre-change forms.
func TestStreamThinking_InterleavedWithTextAndToolUse(t *testing.T) {
	t.Parallel()

	chunks := streamFrames(t,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"pondering"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"signature_delta","signature":"sig-1"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,`+
			`"content_block":{"type":"tool_use","id":"tc-th-1","name":"Bash","input":{}}}`,
		`{"type":"content_block_delta","index":2,`+
			`"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"content_block_delta","index":3,"delta":{"type":"text_delta","text":" world"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		`{"type":"message_stop"}`,
	)

	// Stream order: text, thinking, tool_use, text — each type emits exactly
	// its own events; nothing is swallowed or duplicated.
	var seq []string

	var text strings.Builder

	var tool *provider.ToolCall

	for _, c := range chunks {
		switch c.Type {
		case blockTextChunk:
			seq = append(seq, blockTextChunk)
			text.WriteString(c.Text)
		case chunkThinking:
			seq = append(seq, chunkThinking)
		case blockToolUse:
			seq = append(seq, blockToolUse)
			if c.ToolCall != nil {
				tool = c.ToolCall
			}
		}
	}

	wantSeq := []string{blockTextChunk, chunkThinking, blockToolUse, blockTextChunk}
	if !slices.Equal(seq, wantSeq) {
		t.Errorf("chunk-type sequence = %v; want %v (stream order preserved)", seq, wantSeq)
	}

	if text.String() != "Hello world" {
		t.Errorf("streamed text = %q; want %q (buffered text chunks unaffected)", text.String(), "Hello world")
	}

	if tool == nil {
		t.Fatal("no tool_use chunk with a populated ToolCall")
	}

	if tool.ID != "tc-th-1" || tool.Name != "Bash" || string(tool.Input) != `{"command":"ls"}` {
		t.Errorf("ToolCall = %+v; want {tc-th-1 Bash {\"command\":\"ls\"}} (tool-use lifecycle unaffected)", tool)
	}
}

// TestStreamThinking_FlushOnTerminationWithoutStop pins the flush-at-EOF rule:
// a stream that ENDS (server closes cleanly) while a thinking block is still
// open flushes it exactly as flushToolUse does at its termination call sites.
func TestStreamThinking_FlushOnTerminationWithoutStop(t *testing.T) {
	t.Parallel()

	// No content_block_stop, no message_stop — the connection just ends.
	chunks := streamFrames(t,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"cut off"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-eof"}}`,
	)

	got := thinkingChunks(chunks)
	if len(got) != 1 {
		t.Fatalf("thinking chunks = %d; want exactly 1 (EOF flush; chunks: %+v)", len(got), got)
	}

	v := decodeThinking(t, got[0])
	if v.Thinking != "cut off" || v.Signature != "sig-eof" {
		t.Errorf("flushed block = {%q %q}; want {cut off sig-eof}", v.Thinking, v.Signature)
	}
}

// TestStreamThinking_MultipleBlocksEmitInOrder: several thinking blocks —
// signed, redacted, signed — in ONE response each emit their own chunk, in
// block order.
func TestStreamThinking_MultipleBlocksEmitInOrder(t *testing.T) {
	t.Parallel()

	chunks := streamFrames(t,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"first"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-a"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,`+
			`"content_block":{"type":"redacted_thinking","data":"blob-b"}}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"thinking_delta","thinking":"third"}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"signature_delta","signature":"sig-c"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		`{"type":"message_stop"}`,
	)

	got := thinkingChunks(chunks)
	if len(got) != 3 {
		t.Fatalf("thinking chunks = %d; want 3 (one per block; chunks: %+v)", len(got), got)
	}

	type triple struct{ th, sig, data string }

	want := []triple{
		{"first", "sig-a", ""},
		{"", "", "blob-b"},
		{"third", "sig-c", ""},
	}

	for i, w := range want {
		v := decodeThinking(t, got[i])
		if v.Thinking != w.th || v.Signature != w.sig || v.Data != w.data {
			t.Errorf("block %d = {%q %q %q}; want {%q %q %q} (block order preserved)",
				i, v.Thinking, v.Signature, v.Data, w.th, w.sig, w.data)
		}
	}
}

// TestStreamThinking_SingleFlushOnCleanEnd pins the no-double-emit rule under
// -race: a block closed by content_block_stop followed by a CLEAN stream end
// ([DONE]-equivalent termination) flushes exactly ONCE — the termination
// handler must not re-emit what the stop handler already flushed.
func TestStreamThinking_SingleFlushOnCleanEnd(t *testing.T) {
	t.Parallel()

	chunks := streamFrames(t,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"once"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-1"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		`{"type":"message_stop"}`,
	)

	if got := len(thinkingChunks(chunks)); got != 1 {
		t.Errorf("thinking chunks = %d; want exactly 1 (no double-emit between stop and termination)", got)
	}
}
