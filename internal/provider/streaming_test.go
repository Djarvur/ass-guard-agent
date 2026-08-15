package provider_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
func stalledSSEHandler() (http.HandlerFunc, chan struct{}) {
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
			t.Fatal("stalled stream did not abort within 5s — the idle watchdog is missing (the drain blocks in ReadString forever)")
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
			t.Fatal("channel did not close within 3s of cancelling a stalled stream — the blocked Read was not force-unblocked")
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

	const frameStart = `{"type":"content_block_start","index":0,` +
		`"content_block":{"type":"tool_use","id":"call_dup","name":"Bash","input":{}}}`
	const frameDelta = `{"type":"content_block_delta","index":0,` +
		`"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}`
	const frameStop = `{"type":"content_block_stop","index":0}`

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

	const frameStart = `{"type":"content_block_start","index":0,` +
		`"content_block":{"type":"tool_use","id":"call_dup2","name":"Bash","input":{}}}`
	const frameDelta = `{"type":"content_block_delta","index":0,` +
		`"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"pwd\"}"}}`
	const frameStop = `{"type":"content_block_stop","index":0}`

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
		t.Fatalf("Response.ToolCalls = %d entries; want exactly 1 (duplicates: %+v)", len(resp.ToolCalls), resp.ToolCalls)
	}

	if resp.ToolCalls[0].ID != "call_dup2" {
		t.Errorf("ToolCalls[0].ID = %q; want call_dup2", resp.ToolCalls[0].ID)
	}
}

// ensure profile.Load type is exercised (keeps the test helper honest).
var _ = profile.Profile{}
