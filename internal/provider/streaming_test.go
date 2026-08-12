package provider_test

import (
	"context"
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

	ch, err := p.Stream(context.Background(), prof, []shaper.Message{{Role: "user", Content: "hi"}})
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
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"Bash","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}`,
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

	ch, err := p.Stream(context.Background(), prof, []shaper.Message{{Role: "user", Content: "run ls"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	chunks := readAllChunks(t, ch)

	var (
		finish   string
		toolCall *provider.ToolCall
	)

	for _, c := range chunks {
		if c.Type == "tool_use" && c.ToolCall != nil {
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

	if finish != "tool_use" {
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
		fmt.Fprintf(w, "data: %s\n\n", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"first"}}`)

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

	ch, err := p.Stream(ctx, prof, []shaper.Message{{Role: "user", Content: "hi"}})
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

	_, err := p.Stream(context.Background(), prof, []shaper.Message{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("Stream returned nil error with no API key; want non-nil")
	}
}

// ensure profile.Load type is exercised (keeps the test helper honest).
var _ = profile.Profile{}
