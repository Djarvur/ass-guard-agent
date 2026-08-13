package audit_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/shaper"
)

// TestAudit_IntegrationViaProviderCapturer proves the end-to-end LOG-01 flow:
// the provider shapes the request, its RequestCapturer publishes RequestShaped
// to the bus, and the AuditLogger writes the REDACTED verbatim request to the
// sink. The capturer is the LOG-01 seam (it has the verbatim shaped body).
func TestAudit_IntegrationViaProviderCapturer(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back a tool_use so the round-trip completes; the canned body is
		// not the focus — the capturer is.
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w,
			`{"id":"m","type":"message","role":"assistant","model":"synth-model",`+
				`"stop_reason":"tool_use",`+
				`"content":[{"type":"tool_use","id":"c1","name":"synth_tool_a","input":{"path":"x"}}]}`)
	}))
	defer srv.Close()

	root, _ := filepath.Abs(filepath.Join("..", "profile", "testdata"))

	prof, err := profile.NewLoader(root).Load("minimal")
	if err != nil {
		t.Fatal(err)
	}

	bus := event.NewBus()
	sink := &safeBuffer{} // defined in audit_test.go (same package)
	audit.NewAuditLogger(bus, &capturingSink{safeBuffer: sink})

	// The capturer publishes RequestShaped carrying the verbatim shaped body.
	capturer := func(body []byte, headers map[string]string) {
		bus.Publish(event.RequestShaped{
			VerbatimRequest: body,
			Profile:         prof.Name,
			Timestamp:       time.Now(),
		})
	}

	p := provider.NewAnthropicProvider(shaper.New(),
		provider.WithAnthropicAPIKey("test-key"),
		provider.WithAnthropicBaseURL(srv.URL),
		provider.WithAnthropicRequestCapture(capturer),
	)
	if _, err := p.Send(context.Background(), &prof, []shaper.Message{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}

	// Wait for the async audit subscriber.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && sink.Len() == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	out := sink.String()
	if !strings.Contains(out, `"profile":"minimal"`) {
		t.Errorf("audit line missing profile field:\n%s", out)
	}

	if !strings.Contains(out, "synth_tool_a") {
		t.Errorf("audit line missing the shaped tool declaration (TIER-1 verbatim body):\n%s", out)
	}
	// Confirm the line is one JSON object.
	var line map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &line); err != nil {
		t.Errorf("audit line is not one JSON object: %v", err)
	}
}

// capturingSink wraps safeBuffer as an io.Writer for the audit logger.
type capturingSink struct {
	*safeBuffer
}

func (c *capturingSink) Write(p []byte) (int, error) { return c.safeBuffer.Write(p) }
