package runtime //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// 21-05 Task 2 battery (PAR-06/D-11): provider-capability validation end to
// end — a turn against an image-incapable provider drops the image blocks
// with EXACTLY ONE loud note naming the provider and completes normally; an
// image-capable (Anthropic-shape) turn carries the image block into the
// outgoing request body (criterion 5's image half, through the real ingress
// → transcript → projection → shaping chain).

// Image-capability fixture markers.
const (
	imgD11Marker  = "does not support images"
	imgWireMarker = `"type":"image"`
)

// noImageProvider is a full Provider fake whose SupportsImages is false (the
// OpenAI-shape capability today) with a scripted one-shot end_turn stream —
// the D-11 turn completes normally against it.
type noImageProvider struct {
	mu       sync.Mutex
	received [][]provider.Message
	supports bool
}

func (p *noImageProvider) SupportsImages() bool { return p.supports }

func (p *noImageProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, nil
}

func (p *noImageProvider) Stream(
	ctx context.Context, _ *profile.Profile, messages []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.mu.Lock()
	p.received = append(p.received, messages)
	p.mu.Unlock()

	ch := make(chan provider.StreamChunk, 2)

	go func() {
		defer close(ch)

		select {
		case ch <- provider.StreamChunk{Type: blockText, Text: "text-only answer"}:
		case <-ctx.Done():
			return
		}

		select {
		case ch <- provider.StreamChunk{Type: chunkDone, FinishReason: stopEndTurn}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

func (p *noImageProvider) ToolResultMessage(_ string, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"role":"user","content":"stub"}`), nil
}

// messagesWithImages reports whether any received message carries an image
// block (the D-11 drop's observable at the provider seam).
func (p *noImageProvider) messagesWithImages() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := 0

	for i := range p.received {
		for j := range p.received[i] {
			for _, b := range p.received[i][j].Blocks {
				if b.Image != nil {
					n++
				}
			}
		}
	}

	return n
}

// pngPromptBlocks builds the acp prompt: text + a small base64 PNG image.
func pngPromptBlocks(t *testing.T) []acp.ContentBlock {
	t.Helper()

	var buf bytes.Buffer

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	werr := png.Encode(&buf, img)
	if werr != nil {
		t.Fatalf("encode png: %v", werr)
	}

	return []acp.ContentBlock{
		{Type: blockText, Text: "describe the screenshot"},
		{
			Type: blockImage, Data: base64.StdEncoding.EncodeToString(buf.Bytes()),
			MimeType: imgPNGMedia,
		},
	}
}

// newCapabilityRunner builds a minimal Runner over a temp workspace with the
// given provider factory (engine off — the plain turn path).
func newCapabilityRunner(t *testing.T, factory func(provider.RequestCapturer) provider.Provider) *Runner {
	t.Helper()

	dir := t.TempDir()

	r := &Runner{
		bus:          event.NewBus(),
		profile:      profile.Profile{Name: "imgcap"},
		workDir:      dir,
		makeProvider: factory,
		stderr:       &bytes.Buffer{},
	}

	t.Cleanup(func() {
		if s, ok := r.sessions[imgSessionID]; ok {
			_ = s.Close()
		}
	})

	return r
}

// TestImageCapability_D11DropAndNote proves D-11 end to end: against a
// provider whose SupportsImages() is false, an image block in the prompt is
// dropped from the outgoing request, EXACTLY ONE loud note naming the
// provider lands on stderr, and the turn completes normally with the text.
// Deliberately sequential: a full Run turn must not add contention to the
// timing-sensitive tests sharing the parallel batch.
func TestImageCapability_D11DropAndNote(t *testing.T) { //nolint:paralleltest // full-turn e2e

	prov := &noImageProvider{supports: false}

	r := newCapabilityRunner(t, func(_ provider.RequestCapturer) provider.Provider { return prov })

	stop, err := r.Run(context.Background(), imgSessionID, &noopEmitter{}, pngPromptBlocks(t))
	if err != nil {
		t.Fatalf("Run err = %v (the turn must complete normally after the drop)", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn (never a dead turn)", stop)
	}

	// EXACTLY ONE note — never zero (silent), never two.
	stderr, _ := r.stderr.(*bytes.Buffer)

	if got := strings.Count(stderr.String(), imgD11Marker); got != 1 {
		t.Errorf("D-11 notes = %d; want exactly 1; stderr: %q", got, stderr.String())
	}

	if !strings.Contains(stderr.String(), "noImageProvider") {
		t.Errorf("D-11 note does not name the provider: %q", stderr.String())
	}

	// The image never reached the provider seam.
	if got := prov.messagesWithImages(); got != 0 {
		t.Errorf("provider received %d image block(s); want 0 (dropped before shaping)", got)
	}

	// The transcript user_message carries NO image Ref (stripped pre-append)
	// and the text survives.
	mgr := r.sessions[imgSessionID].Manager

	lines, lerr := mgr.ReadAll()
	if lerr != nil {
		t.Fatalf("ReadAll: %v", lerr)
	}

	found := false

	for i := range lines {
		if lines[i].Type != session.TypeUserMessage {
			continue
		}

		found = true

		var blocks []session.ContentBlock

		uerr := json.Unmarshal(lines[i].Content, &blocks)
		if uerr != nil {
			t.Fatalf("unmarshal user_message: %v", uerr)
		}

		if len(blocks) != 1 || blocks[0].Type != blockText {
			t.Errorf("user_message blocks = %+v; want the text block only (image stripped)", blocks)
		}
	}

	if !found {
		t.Fatal("no user_message line in the transcript")
	}
}

// TestImageCapability_ImageReachesOutgoingRequest proves criterion 5's image
// half end to end: against the real Anthropic adapter with a fake SSE backend,
// a pasted image produces the corresponding image content block in the
// OUTGOING request body (ingress → transcript Ref → projection fold → shaper
// base64 mapping), with the transcript line staying lean.
// Deliberately sequential: a full Run turn against a real SSE backend must
// not add contention to the timing-sensitive tests sharing the parallel batch.
//
//nolint:funlen // one e2e flow
func TestImageCapability_ImageReachesOutgoingRequest(t *testing.T) { //nolint:paralleltest // full-turn e2e

	var bodyMu sync.Mutex

	var bodies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")

		frames := []string{
			`{"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"got it"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`{"type":"message_stop"}`,
		}

		for _, f := range frames {
			fmt.Fprintf(w, "data: %s\n\n", f)
		}
	}))
	defer srv.Close()

	r := newCapabilityRunner(t, func(capturer provider.RequestCapturer) provider.Provider {
		return provider.NewAnthropicProvider(shaper.New(),
			provider.WithAnthropicAPIKey("test-key"),
			provider.WithAnthropicBaseURL(srv.URL),
			provider.WithAnthropicRequestCapture(func(body []byte, _ map[string]string) {
				bodyMu.Lock()

				bodies = append(bodies, string(body))

				bodyMu.Unlock()

				if capturer != nil {
					capturer(body, nil)
				}
			}),
		)
	})

	stop, err := r.Run(context.Background(), imgSessionID, &noopEmitter{}, pngPromptBlocks(t))
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want end_turn", stop)
	}

	bodyMu.Lock()

	if len(bodies) == 0 {
		bodyMu.Unlock()

		t.Fatal("no outgoing request captured")
	}

	body := bodies[0]

	bodyMu.Unlock()

	if !strings.Contains(body, imgWireMarker) {
		t.Errorf("outgoing body carries no image content block; body head: %.200s", body)
	}

	if !strings.Contains(body, `"image/png"`) {
		t.Errorf("outgoing body lacks the image media type; body head: %.200s", body)
	}

	// The transcript line stayed lean (no base64) while the wire carried it.
	mgr := r.sessions[imgSessionID].Manager

	lines, lerr := mgr.ReadAll()
	if lerr != nil {
		t.Fatalf("ReadAll: %v", lerr)
	}

	var blocks []session.ContentBlock

	for i := range lines {
		if lines[i].Type == session.TypeUserMessage {
			blocks = nil

			uerr := json.Unmarshal(lines[i].Content, &blocks)
			if uerr != nil {
				t.Fatalf("unmarshal user_message content: %v", uerr)
			}
		}
	}

	if len(blocks) != 2 || blocks[1].DataRef == "" || blocks[1].Data != "" {
		t.Errorf("transcript blocks = %+v; want text + Ref-form image (lean line)", blocks)
	}
}

// TestImageCapability_OpenAIShapeBodyHasNoImage pins the letter's provider
// check: the OpenAI-shape adapter's request body carries NO image content
// even when handed a Blocks-bearing message (text-only today; the MultiContent
// surface is explicitly out of PAR-06's letter).
func TestImageCapability_OpenAIShapeBodyHasNoImage(t *testing.T) {
	t.Parallel()

	ref, b64 := plantPngRef(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")

		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,` +
			`"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},` +
			`"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	var gotBody string

	prov := provider.NewOpenAIProvider(
		provider.WithOpenAIAPIKey("test-key"),
		provider.WithOpenAIBaseURL(srv.URL+"/v1"),
		provider.WithOpenAIRequestCapture(func(body []byte, _ map[string]string) {
			gotBody = string(body)
		}),
	)

	if prov.SupportsImages() {
		t.Fatal("OpenAI-shape SupportsImages = true; want false (text-only today)")
	}

	messages := []provider.Message{{
		Role: "user",
		Blocks: []shaper.Block{
			{Text: "look"},
			{Image: &shaper.ImageBlock{Ref: ref, MediaType: imgPNGMedia}},
		},
	}}

	if _, err := prov.Send(context.Background(), &profile.Profile{Name: "p"}, messages); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if strings.Contains(gotBody, imgWireMarker) || strings.Contains(gotBody, b64) {
		t.Errorf("OpenAI-shape body carries image content: %.200s", gotBody)
	}
}

// plantPngRef persists a tiny PNG in a temp dir, returning its path + base64.
func plantPngRef(t *testing.T) (ref, b64 string) { //nolint:nonamedreturns // tight fixture helper
	t.Helper()

	var buf bytes.Buffer

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	werr := png.Encode(&buf, img)
	if werr != nil {
		t.Fatalf("encode png: %v", werr)
	}

	ref = filepath.Join(t.TempDir(), "ref.png")

	werr = os.WriteFile(ref, buf.Bytes(), 0o600)
	if werr != nil {
		t.Fatalf("plant ref: %v", werr)
	}

	return ref, base64.StdEncoding.EncodeToString(buf.Bytes())
}
