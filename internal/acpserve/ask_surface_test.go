package acpserve //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// The 17-02 permission-ask surface battery (ACP-01): the v1 frame shape
// (four neutral option kinds, ToolCallUpdate toolCall) and the registry-backed
// dispatch mapping under the HUMAN-ASK class — selected/cancelled/-32800/
// -32601/timeout each map to the session-native outcome.

// Test-local vocabulary (goconst).
const (
	gateToolWrite = "Write"
	gatePathInput = `{"path":"x"}`
	jsonrpcV20    = "2.0"
)

// permAskSink records frames the registry writes (the NotificationSink fake).
type permAskSink struct {
	mu   sync.Mutex
	msgs []*acp.Message
	got  chan *acp.Message
}

func (s *permAskSink) Write(m *acp.Message) error {
	s.mu.Lock()
	s.msgs = append(s.msgs, m)
	s.mu.Unlock()

	s.got <- m

	return nil
}

// gateAskEntry is the fire payload one suspended gated call produces.
func gateAskEntry() *session.AskEntry {
	return &session.AskEntry{
		TurnID:    "sess-1-turn-001",
		SessionID: "sess-1",
		CallID:    "call_gate_1",
		Tool:      gateToolWrite,
		Title:     gateToolWrite,
		Kind:      acp.ToolKindEdit,
		Input:     json.RawMessage(gatePathInput),
		Class:     session.AskClassForeground,
	}
}

// fireAsync runs one Fire round-trip on its own goroutine and hands back the
// outcome (Fire blocks until the registry resolves).
func fireAsync(t *testing.T, pa *PermissionAsk) <-chan session.AskOutcome {
	t.Helper()

	out := make(chan session.AskOutcome, 1)

	go func() { out <- pa.Fire(context.Background(), gateAskEntry()) }()

	return out
}

// waitForRequest waits for the registry to write the request frame.
func waitForRequest(t *testing.T, sink *permAskSink) *acp.Message {
	t.Helper()

	select {
	case m := <-sink.got:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("no request frame written within 2s")
	}

	return nil
}

// TestPermissionAskFrame pins the v1 RequestPermissionFrame shape verbatim:
// camelCase wire fields, the ToolCallUpdate toolCall, and EXACTLY the four
// canonical option kinds with neutral, symmetric labels (the ACP-01 values
// prohibition — never asymmetric or steered framing).
func TestPermissionAskFrame(t *testing.T) {
	t.Parallel()

	frame := BuildPermissionAsk("sess-1", "call_gate_1", gateToolWrite, json.RawMessage(gatePathInput))

	if frame.SessionID != "sess-1" {
		t.Errorf("sessionId = %q; want sess-1", frame.SessionID)
	}

	if frame.ToolCall.ToolCallID != "call_gate_1" || frame.ToolCall.Title != gateToolWrite {
		t.Errorf("toolCall = %+v; want the ToolCallUpdate identity (toolCallId + title)", frame.ToolCall)
	}

	raw, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}

	var wire map[string]any

	uerr := json.Unmarshal(raw, &wire)
	if uerr != nil {
		t.Fatalf("unmarshal frame: %v", uerr)
	}

	if _, ok := wire["sessionId"]; !ok {
		t.Error("frame missing camelCase sessionId (v1 verbatim)")
	}

	if _, ok := wire["toolCall"]; !ok {
		t.Error("frame missing toolCall (the ToolCallUpdate shape)")
	}

	opts, ok := wire["options"].([]any)
	if !ok || len(opts) != 4 {
		t.Fatalf("options = %v; want exactly the four canonical options", wire["options"])
	}

	want := []struct{ id, kind, name string }{
		{acp.PermOptionAllowOnce, acp.PermOptionAllowOnce, "Allow once"},
		{acp.PermOptionAllowAlways, acp.PermOptionAllowAlways, "Always allow"},
		{acp.PermOptionRejectOnce, acp.PermOptionRejectOnce, "Reject once"},
		{acp.PermOptionRejectAlways, acp.PermOptionRejectAlways, "Always reject"},
	}

	for i, o := range opts {
		m, _ := o.(map[string]any)

		gotID, _ := m["optionId"].(string)
		gotKind, _ := m["kind"].(string)
		gotName, _ := m["name"].(string)

		if gotID != want[i].id || gotKind != want[i].kind || gotName != want[i].name {
			t.Errorf("option[%d] = {optionId:%q kind:%q name:%q}; want %+v", i, gotID, gotKind, gotName, want[i])
		}
	}
}

// TestPermissionAskDispatch pins the outcome mapping for one registry-backed
// ask: selected → Selected, cancelled → Cancelled, -32800 → Cancelled,
// -32601 → Err (fail-safe decline, never allow), timeout → Err (D-14
// fallback).
func TestPermissionAskDispatch(t *testing.T) { //nolint:cyclop,funlen // one table over the wire outcome vocabulary
	t.Parallel()

	answer := func(t *testing.T, respond func(req *acp.Message) *acp.Message) session.AskOutcome {
		t.Helper()

		sink := &permAskSink{got: make(chan *acp.Message, 4)}
		reg := acp.NewRegistry(sink, io.Discard)
		pa := NewPermissionAsk(context.Background(), reg, io.Discard)

		out := fireAsync(t, pa)
		req := waitForRequest(t, sink)

		if req.Method != acp.MethodRequestPermission {
			t.Errorf("method = %q; want %q", req.Method, acp.MethodRequestPermission)
		}

		resp := respond(req)
		if resp != nil {
			reg.Deliver(resp)
		}

		select {
		case got := <-out:
			return got
		case <-time.After(2 * time.Second):
			t.Fatal("Fire never resolved within 2s")
		}

		return session.AskOutcome{}
	}

	t.Run("selected", func(t *testing.T) {
		t.Parallel()
		got := answer(t, func(req *acp.Message) *acp.Message {
			return &acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
				Result: json.RawMessage(`{"outcome":"selected","optionId":"` + acp.PermOptionAllowAlways + `"}`)}
		})

		if got.Selected != acp.PermOptionAllowAlways || got.Cancelled || got.Err != nil {
			t.Errorf("outcome = %+v; want selected allow_always", got)
		}
	})

	t.Run("cancelled outcome", func(t *testing.T) {
		t.Parallel()
		got := answer(t, func(req *acp.Message) *acp.Message {
			return &acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
				Result: json.RawMessage(`{"outcome":"cancelled"}`)}
		})

		if !got.Cancelled || got.Err != nil {
			t.Errorf("outcome = %+v; want cancelled", got)
		}
	})

	t.Run("client cancelled -32800", func(t *testing.T) {
		t.Parallel()
		got := answer(t, func(req *acp.Message) *acp.Message {
			return &acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
				Error: &acp.RPCError{Code: acp.CodeRequestCancelled, Message: "cancelled"}}
		})

		if !got.Cancelled || got.Err != nil {
			t.Errorf("outcome = %+v; want cancelled", got)
		}
	})

	t.Run("method not found -32601", func(t *testing.T) {
		t.Parallel()
		got := answer(t, func(req *acp.Message) *acp.Message {
			return &acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
				Error: &acp.RPCError{Code: acp.CodeMethodNotFound, Message: "not found"}}
		})

		if got.Err == nil || got.Cancelled || got.Selected != "" {
			t.Errorf("outcome = %+v; want a fail-safe Err (never allow)", got)
		}
	})

	t.Run("timeout fallback", func(t *testing.T) {
		t.Parallel()

		sink := &permAskSink{got: make(chan *acp.Message, 4)}
		reg := acp.NewRegistry(sink, io.Discard,
			acp.WithRegistryTimeouts(acp.RegistryConfig{HumanAskTimeout: 2 * time.Millisecond}))
		pa := NewPermissionAsk(context.Background(), reg, io.Discard)

		got := <-fireAsync(t, pa)

		var timeout *acp.RequestTimeoutError
		if got.Err == nil || !errors.As(got.Err, &timeout) {
			t.Errorf("outcome = %+v; want a *RequestTimeoutError fail-safe Err", got)
		}
	})
}
