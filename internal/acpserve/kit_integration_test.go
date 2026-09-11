package acpserve //nolint:testpackage // internal package test

// 25-08 subject-split move (kit/runtime -> internal/acpserve): these
// batteries drive a REAL acp.Server over pipes and assert on WIRE FRAMES —
// the serve path is the subject, so per the Phase-15 D-02 rule the tests
// live at their subject's home and exercise the PRODUCTION kitTurnAdapter
// (kit-side they rode the acpTurnRunner twin). Construction retargets:
// the unexported Runner literal -> NewRunner; every assertion is
// byte-identical.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
)

// TestIntegration_RealStreamingThroughACP verifies the real Session Core streams
// token-by-token through ACP: initialize → session/new → session/prompt emits
// agent_message_chunk session/update notifications (one per chunk) before the
// stopReason response. NO full-turn buffering (ACP-04).
// Zed-like initialize advertisement fragments (acp.rs:767-795): advertising
// elicitation.form keeps the D-13 advertisement-first path probe-free.
func TestIntegration_RealStreamingThroughACP(t *testing.T) { //nolint:funlen // comprehensive test scenario
	t.Parallel()

	mp := &mockStreamProvider{chunks: []string{"Hello", " ", "world"}, finish: wireStopEndTurn}

	cliW, cliR, stop := wireDriveACP(t, mp)
	defer stop()

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("0"), Method: "initialize",
		Params: wireRawJSON(map[string]any{"protocolVersion": 1,
			// Zed-like elicitation advertisement (acp.rs:767-795) — the D-13
			// advertisement-first rule means NO capability probe fires.
			wireKeyClientCapabilities: map[string]any{
				wireKeyElicitation: map[string]any{wireKeyForm: map[string]any{}},
			}}),
	})

	frames := wireReadFrames(t, cliR, 1)
	if len(frames) == 0 || !strings.Contains(string(frames[0].Result), "agentCapabilities") {
		t.Fatalf("no initialize response with agentCapabilities: %+v", frames)
	}

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("1"), Method: wireMethodSessNew,
		Params: wireRawJSON(map[string]any{wireCwdKey: wireTestCwdTmp, "mcpServers": []any{}}),
	})
	frames = wireReadResultFrames(t, cliR, 1)

	var snew struct {
		SessionID string `json:"sessionId"` //nolint:tagliatelle // ACP wire field
	}

	_ = json.Unmarshal(frames[0].Result, &snew)

	if snew.SessionID == "" {
		t.Fatalf("no sessionId in session/new response: %+v", frames)
	}

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("2"), Method: wireMethodSessPrmt,
		Params: wireRawJSON(map[string]any{
			wireKeySessionID: snew.SessionID,
			"prompt":         []map[string]any{{wireKeyType: wireBlockText, wireBlockText: "hi"}},
		})})

	// Collect frames: expect ≥1 session/update (agent_message_chunk) + the prompt response.
	br := bufio.NewReader(cliR)
	chunks := 0

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := br.ReadBytes('\n')
		if len(line) == 0 {
			if err != nil {
				break
			}

			continue
		}

		var m acp.Message
		if json.Unmarshal(bytes.TrimRight(line, "\n"), &m) != nil {
			continue
		}

		if m.Method == wireSessionUpdate {
			chunks++
		}

		if m.ID != nil && string(m.ID) == "2" {
			var pres struct {
				StopReason string `json:"stopReason"` //nolint:tagliatelle // ACP wire field
			}

			_ = json.Unmarshal(m.Result, &pres)

			if pres.StopReason != wireStopEndTurn {
				t.Errorf("stopReason = %q; want end_turn", pres.StopReason)
			}

			if chunks == 0 {
				t.Error("session/prompt response arrived with NO preceding session/update (ACP-04 streaming)")
			}

			return
		}
	}

	t.Fatalf("never saw the session/prompt response (chunks=%d)", chunks)
}

// TestIntegration_SessionLoadMalformedRejected verifies session/load with a
// malformed sessionId rejects with the typed -32602 BEFORE any file open
// (18-01/T-18-01 — the D-09 -32601 no-op ended with Phase 18; load is real,
// and the structural rejection is the first gate of the pipeline). Runs
// against the REAL runner wiring (wireDriveACP) so the integration pin
// covers the composed server, not just the handler unit.
func TestIntegration_SessionLoadMalformedRejected(t *testing.T) {
	t.Parallel()

	mp := &mockStreamProvider{finish: wireStopEndTurn}

	cliW, cliR, stop := wireDriveACP(t, mp)
	defer stop()

	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("0"), Method: "initialize",
		Params: wireRawJSON(map[string]any{"protocolVersion": 1,
			// Zed-like elicitation advertisement (acp.rs:767-795) — the D-13
			// advertisement-first rule means NO capability probe fires.
			wireKeyClientCapabilities: map[string]any{
				wireKeyElicitation: map[string]any{wireKeyForm: map[string]any{}},
			}}),
	})
	wireReadFrames(t, cliR, 1)
	wireSendFrame(t, cliW, &acp.Message{
		JSONRPC: wireProtocolVersion20, ID: json.RawMessage("1"), Method: "session/load",
		Params: wireRawJSON(map[string]any{wireKeySessionID: "x"}),
	})

	frames := wireReadFrames(t, cliR, 1)
	if len(frames) == 0 || frames[0].Error == nil {
		t.Fatalf("session/load did not return an error: %+v", frames)
	}

	if frames[0].Error.Code != -32602 {
		t.Errorf("session/load error code = %d; want -32602 (malformed sessionId, 18-01)",
			frames[0].Error.Code)
	}
}
