package acpserve //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/coreexec"
	"github.com/Djarvur/ass-guard-agent/internal/learning"
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

	// 17-04 battery vocabulary (goconst).
	elicSess1      = "sess-1"
	elicTurn1      = "sess-1-turn-001"
	elicCall1      = "call_ask_1"
	elicQText1     = "Which cache library should we use?"
	elicHdrCache   = "Cache"
	elicHdrAreas   = "Areas"
	elicHdrTicket  = "Ticket"
	elicHdrProceed = "Proceed"
	elicOptFast    = "ristretto"
	elicOptDisk    = "bigcache"
	elicAreaAPI    = "api"
	elicAreaDocs   = "docs"
	elicReaskNote  = `value "x" is not one of the offered choices`
	elicViolation  = `expected a string`
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
		TurnID:    elicTurn1,
		SessionID: elicSess1,
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
// fallback). The selected/cancelled payloads are the CANONICAL v1 nested
// outcome object — the shape live Zed 1.18.0 answers with (17-UAT G-17-1;
// https://agentclientprotocol.com/protocol/v1/tool-calls) — while the flat
// one-level shape and unknown inner discriminators fail safe. A selected
// answer without optionId passes through as the empty selection — the session
// gate owns that decline (WR-01 pins both sides of the seam).
func TestPermissionAskDispatch(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen // wire outcome table
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
				Result: json.RawMessage(`{"outcome":{"outcome":"selected","optionId":"` +
					acp.PermOptionAllowAlways + `"}}`)}
		})

		if got.Selected != acp.PermOptionAllowAlways || got.Cancelled || got.Err != nil {
			t.Errorf("outcome = %+v; want selected allow_always", got)
		}
	})

	t.Run("selected without optionId passes through empty", func(t *testing.T) {
		t.Parallel()
		got := answer(t, func(req *acp.Message) *acp.Message {
			// The canonical selected shape minus optionId: the SURFACE passes
			// the empty selection through untouched — the session gate's
			// default unknown-option branch owns the decline (pinned in
			// internal/session gate_test.go, WR-01).
			return &acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
				Result: json.RawMessage(`{"outcome":{"outcome":"selected"}}`)}
		})

		if got.Selected != "" || got.Err != nil || got.Cancelled {
			t.Errorf("outcome = %+v; want the pass-through shape: Selected empty, no Err, not cancelled", got)
		}
	})

	t.Run("cancelled outcome", func(t *testing.T) {
		t.Parallel()
		got := answer(t, func(req *acp.Message) *acp.Message {
			return &acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
				Result: json.RawMessage(`{"outcome":{"outcome":"cancelled"}}`)}
		})

		if !got.Cancelled || got.Err != nil {
			t.Errorf("outcome = %+v; want cancelled", got)
		}
	})

	t.Run("flat outcome nonconformance", func(t *testing.T) {
		t.Parallel()
		got := answer(t, func(req *acp.Message) *acp.Message {
			// The pre-17-06 wrong shape: a one-level outcome string with
			// optionId as its sibling — the shape Zed never sends.
			return &acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
				Result: json.RawMessage(`{"outcome":"selected","optionId":"` + acp.PermOptionAllowAlways + `"}`)}
		})

		if got.Err == nil || got.Cancelled || got.Selected != "" {
			t.Errorf("outcome = %+v; want a fail-safe Err (never an allow)", got)
		}

		if !errors.Is(got.Err, errPermissionOutcomeBad) {
			t.Errorf("outcome Err = %v; want the errPermissionOutcomeBad family", got.Err)
		}
	})

	t.Run("unknown inner discriminator", func(t *testing.T) {
		t.Parallel()
		got := answer(t, func(req *acp.Message) *acp.Message {
			return &acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
				Result: json.RawMessage(`{"outcome":{"outcome":"banana"}}`)}
		})

		if got.Err == nil || got.Cancelled || got.Selected != "" {
			t.Errorf("outcome = %+v; want a fail-safe Err (never an allow)", got)
		}

		if !errors.Is(got.Err, errPermissionOutcomeUnknown) {
			t.Errorf("outcome Err = %v; want the errPermissionOutcomeUnknown family", got.Err)
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

// --- 17-04 elicitation battery (ACP-02, D-08/D-09): the D-08 mapping goldens
// (one per row of the locked table), the frame shape, and the capability-gated
// dispatch with today's plain-text path as the verbatim fallback. ---

// The mapping battery's question fixtures (constructors — no globals).
func elicQ1() session.AskQuestion {
	return session.AskQuestion{
		Question: elicQText1,
		Header:   elicHdrCache,
		Options: []session.AskOption{
			{Label: elicOptFast, Description: "fast in-memory cache"},
			{Label: elicOptDisk, Description: "simple disk-backed cache"},
		},
	}
}

func elicQ2() session.AskQuestion {
	return session.AskQuestion{
		Question:    "Which areas should the review cover?",
		Header:      elicHdrAreas,
		MultiSelect: true,
		Options: []session.AskOption{
			{Label: elicAreaAPI}, {Label: elicAreaDocs},
		},
	}
}

func elicQFree() session.AskQuestion {
	return session.AskQuestion{Question: "What is the ticket id?", Header: elicHdrTicket}
}

func elicQBool() session.AskQuestion {
	return session.AskQuestion{
		Question: "Proceed without tests?",
		Header:   elicHdrProceed,
		Options:  []session.AskOption{{Label: "Yes"}, {Label: "No"}},
	}
}

// elicEntry builds the fire payload one question suspension produces.
func elicEntry() *session.AskEntry {
	return &session.AskEntry{
		TurnID:    elicTurn1,
		SessionID: elicSess1,
		CallID:    elicCall1,
		Tool:      "AskUserQuestion",
		Input:     mustMarshalQs([]session.AskQuestion{elicQ1()}),
		Class:     session.AskClassForeground,
	}
}

func mustMarshalQs(qs []session.AskQuestion) json.RawMessage {
	out, err := json.Marshal(qs)
	if err != nil {
		panic(err)
	}

	return out
}

// elicProp decodes one built property's raw shape.
func elicProp(t *testing.T, f *acp.ElicitationFormFrame, key string) map[string]any {
	t.Helper()

	raw, ok := f.RequestedSchema.Properties[key]
	if !ok {
		t.Fatalf("property %q missing; properties = %v", key, f.RequestedSchema.Properties)
	}

	var m map[string]any

	uerr := json.Unmarshal(raw, &m)
	if uerr != nil {
		t.Fatalf("unmarshal property %q: %v", key, uerr)
	}

	return m
}

// TestElicitationMapping pins the D-08 table row by row: single-choice →
// string oneOf titled consts; multiSelect → array items (enum, anyOf-titled
// when descriptions exist); free-text/empty-Options → string; boolean ask →
// boolean ONLY when advertised, else a two-value string oneOf; N questions →
// N properties all required; the banned MCP-legacy enumNames key never
// appears; the question text lands in the message; the D-10 re-ask note rides
// the message.
func TestElicitationMapping(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen,lll,maintidx // one golden table over the D-08 rows
	t.Parallel()

	t.Run("single_choice_oneOf_titled_consts", func(t *testing.T) {
		t.Parallel()

		f := BuildElicitationForm([]session.AskQuestion{elicQ1()}, false, "", elicSess1, elicCall1)
		p := elicProp(t, &f, "q1")

		if p["type"] != propTypeString || p["title"] != elicHdrCache {
			t.Errorf("property q1 = %v; want type string + title Cache", p)
		}

		oneOf, ok := p["oneOf"].([]any)
		if !ok || len(oneOf) != 2 {
			t.Fatalf("oneOf = %v; want 2 titled consts", p["oneOf"])
		}

		first, _ := oneOf[0].(map[string]any)

		if first["const"] != elicOptFast || first["title"] != "ristretto" ||
			first["description"] != "fast in-memory cache" {
			t.Errorf("oneOf[0] = %v; want const/title/description of the option label", first)
		}
	})

	t.Run("multiselect_array_enum", func(t *testing.T) {
		t.Parallel()

		f := BuildElicitationForm([]session.AskQuestion{elicQ2()}, false, "", elicSess1, "")
		p := elicProp(t, &f, "q1")

		if p["type"] != propTypeArray || p["title"] != elicHdrAreas {
			t.Errorf("property q1 = %v; want type array + title Areas", p)
		}

		items, ok := p["items"].(map[string]any)
		if !ok {
			t.Fatalf("items = %v; want the items schema", p["items"])
		}

		if items["type"] != "string" {
			t.Errorf("items.type = %v; want string", items["type"])
		}

		enum, ok := items["enum"].([]any)
		if !ok || len(enum) != 2 || enum[0] != elicAreaAPI || enum[1] != "docs" {
			t.Errorf("items.enum = %v; want [api docs]", items["enum"])
		}
	})

	t.Run("multiselect_anyOf_titled_when_descriptions", func(t *testing.T) {
		t.Parallel()

		q := elicQ2()
		q.Options = []session.AskOption{
			{Label: elicAreaAPI, Description: "the surface"}, {Label: elicAreaDocs},
		}

		f := BuildElicitationForm([]session.AskQuestion{q}, false, "", elicSess1, "")

		props := elicProp(t, &f, "q1")

		itemsRaw := props["items"]

		items, iok := itemsRaw.(map[string]any)
		if !iok {
			t.Fatalf("items = %v; want the items schema", itemsRaw)
		}

		anyOf, ok := items["anyOf"].([]any)
		if !ok || len(anyOf) != 2 {
			t.Fatalf("items.anyOf = %v; want 2 titled options when descriptions exist", items["anyOf"])
		}

		first, _ := anyOf[0].(map[string]any)

		if first["const"] != elicAreaAPI || first["description"] != "the surface" {
			t.Errorf("anyOf[0] = %v; want the titled const with description", first)
		}
	})

	t.Run("free_text_string", func(t *testing.T) {
		t.Parallel()

		f := BuildElicitationForm([]session.AskQuestion{elicQFree()}, false, "", elicSess1, "")
		p := elicProp(t, &f, "q1")

		if p["type"] != "string" || p["title"] != "Ticket" {
			t.Errorf("property q1 = %v; want the free-text string property", p)
		}

		if _, has := p["oneOf"]; has {
			t.Errorf("free-text property carries oneOf: %v", p)
		}
	})

	t.Run("empty_options_free_text", func(t *testing.T) {
		t.Parallel()

		q := session.AskQuestion{Question: "Say something", Header: "H", Options: []session.AskOption{}}

		f := BuildElicitationForm([]session.AskQuestion{q}, false, "", elicSess1, "")
		p := elicProp(t, &f, "q1")

		if p["type"] != propTypeString {
			t.Errorf("empty-Options property = %v; want the free-text string property (edge probe: empty)", p)
		}
	})

	t.Run("boolean_advertised", func(t *testing.T) {
		t.Parallel()

		f := BuildElicitationForm([]session.AskQuestion{elicQBool()}, true, "", elicSess1, "")
		p := elicProp(t, &f, "q1")

		if p["type"] != propTypeBool {
			t.Errorf("advertised boolean property = %v; want type boolean", p)
		}
	})

	t.Run("boolean_not_advertised_two_value_string_oneOf", func(t *testing.T) {
		t.Parallel()

		f := BuildElicitationForm([]session.AskQuestion{elicQBool()}, false, "", elicSess1, "")
		p := elicProp(t, &f, "q1")

		if p["type"] != propTypeString {
			t.Errorf("unadvertised boolean property = %v; want the conservative string select", p)
		}

		oneOf, ok := p["oneOf"].([]any)
		if !ok || len(oneOf) != 2 {
			t.Fatalf("oneOf = %v; want exactly the two value options", p["oneOf"])
		}

		first, _ := oneOf[0].(map[string]any)

		if first["const"] != "Yes" {
			t.Errorf("oneOf[0].const = %v; want the authored label Yes", first["const"])
		}
	})

	t.Run("n_questions_n_properties_all_required", func(t *testing.T) {
		t.Parallel()

		qs := []session.AskQuestion{elicQ1(), elicQ2(), elicQFree()}

		f := BuildElicitationForm(qs, false, "", "sess-1", "")

		if len(f.RequestedSchema.Properties) != 3 {
			t.Errorf("properties = %d; want one per question", len(f.RequestedSchema.Properties))
		}

		if len(f.RequestedSchema.Required) != 3 {
			t.Fatalf("required = %v; want every property required", f.RequestedSchema.Required)
		}

		for i, name := range f.RequestedSchema.Required {
			want := session.StructuredPropertyKey(i)
			if name != want {
				t.Errorf("required[%d] = %q; want the stable derived key %q", i, name, want)
			}
		}
	})

	t.Run("never_enumNames", func(t *testing.T) {
		t.Parallel()

		qs := []session.AskQuestion{elicQ1(), elicQ2()}

		raw, err := json.Marshal(BuildElicitationForm(qs, false, "", "sess-1", ""))
		if err != nil {
			t.Fatalf("marshal frame: %v", err)
		}

		if strings.Contains(string(raw), "enumNames") {
			t.Error("frame carries the banned MCP-legacy enumNames key (RFD ban)")
		}
	})

	t.Run("frame_shape_and_message", func(t *testing.T) {
		t.Parallel()

		f := BuildElicitationForm([]session.AskQuestion{elicQ1()}, false, "", elicSess1, elicCall1)

		if f.Mode != acp.ElicitationModeForm {
			t.Errorf("mode = %q; want form", f.Mode)
		}

		if f.RequestedSchema.Type != propTypeObject {
			t.Errorf("requestedSchema.type = %q; want object", f.RequestedSchema.Type)
		}

		if f.SessionID != "sess-1" || f.ToolCallID != "call_ask_1" {
			t.Errorf("scope = (%q, %q); want the session scope (+ optional toolCallId)", f.SessionID, f.ToolCallID)
		}

		raw, err := json.Marshal(f)
		if err != nil {
			t.Fatalf("marshal frame: %v", err)
		}

		var wire map[string]any

		werr := json.Unmarshal(raw, &wire)
		if werr != nil {
			t.Fatalf("unmarshal frame: %v", werr)
		}

		for _, key := range []string{"message", "mode", "requestedSchema", "sessionId", "toolCallId"} {
			if _, ok := wire[key]; !ok {
				t.Errorf("frame missing camelCase wire field %q (v1 verbatim)", key)
			}
		}

		if msg, _ := wire["message"].(string); !strings.Contains(msg, elicQ1().Question) {
			t.Errorf("message %q does not carry the question text", msg)
		}
	})

	t.Run("reask_note_in_message", func(t *testing.T) {
		t.Parallel()

		f := BuildElicitationForm([]session.AskQuestion{elicQ1()}, false, elicReaskNote,
			elicSess1, "")

		if !strings.HasPrefix(f.Message, "previous answer invalid: ") {
			t.Errorf("re-ask message = %q; want the violation named first", f.Message)
		}
	})
}

// TestAskSurfaceDispatchElicitation pins the capability-gated dispatch: ok →
// one elicitation/create round-trip under the registry (accept/decline/cancel
// mapped to the session-native outcomes); degraded → the plain-text fallback
// publishing byte-parity RenderAskSurface content with NO registry write;
// -32601 → fallback + STICKY degradation (the second ask never re-round-trips).
func TestAskSurfaceDispatchElicitation(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen,lll // one battery over the dispatch vocabulary
	t.Parallel()

	newSurface := func(t *testing.T, capOK bool) (*ElicitationAsk, *permAskSink, *[]string) {
		t.Helper()

		sink := &permAskSink{got: make(chan *acp.Message, 4)}
		reg := acp.NewRegistry(sink, io.Discard)

		var pubMu sync.Mutex

		var published []string

		ea := NewElicitationAsk(ElicitationAskConfig{
			Ctx:      context.Background(),
			Registry: reg,
			Stderr:   io.Discard,
			CapOK:    func() bool { return capOK },
			Fallback: func(turnID, text string) {
				pubMu.Lock()
				defer pubMu.Unlock()

				published = append(published, turnID+"\x00"+text)
			},
		})

		return ea, sink, &published
	}

	t.Run("ok_accept_round_trip", func(t *testing.T) {
		t.Parallel()

		ea, sink, _ := newSurface(t, true)

		out := make(chan session.AskOutcome, 1)

		go func() { out <- ea.Fire(context.Background(), elicEntry()) }()

		req := waitForRequest(t, sink)

		if req.Method != acp.MethodElicitationCreate {
			t.Errorf("method = %q; want %q", req.Method, acp.MethodElicitationCreate)
		}

		reg := ea.registry
		reg.Deliver(&acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
			Result: json.RawMessage(`{"action":"accept","content":{"q1":"ristretto"}}`)})

		select {
		case got := <-out:
			if got.Elicit != acp.ElicitationActionAccept || got.Violation != "" {
				t.Fatalf("outcome = %+v; want a clean accept", got)
			}

			if string(got.Content["q1"]) != `"ristretto"` {
				t.Errorf("content[q1] = %s; want the raw accept value", got.Content["q1"])
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Fire never resolved within 2s")
		}
	})

	t.Run("ok_decline", func(t *testing.T) {
		t.Parallel()

		ea, sink, _ := newSurface(t, true)

		out := make(chan session.AskOutcome, 1)

		go func() { out <- ea.Fire(context.Background(), elicEntry()) }()

		req := waitForRequest(t, sink)
		ea.registry.Deliver(&acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
			Result: json.RawMessage(`{"action":"decline"}`)})

		select {
		case got := <-out:
			if got.Elicit != acp.ElicitationActionDecline || got.Cancelled || got.Err != nil {
				t.Errorf("outcome = %+v; want the decline action (an answer-shaped refusal)", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Fire never resolved within 2s")
		}
	})

	t.Run("ok_cancel", func(t *testing.T) {
		t.Parallel()

		ea, sink, _ := newSurface(t, true)

		out := make(chan session.AskOutcome, 1)

		go func() { out <- ea.Fire(context.Background(), elicEntry()) }()

		req := waitForRequest(t, sink)
		ea.registry.Deliver(&acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
			Result: json.RawMessage(`{"action":"cancel"}`)})

		select {
		case got := <-out:
			if !got.Cancelled {
				t.Errorf("outcome = %+v; want the cancelled family", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Fire never resolved within 2s")
		}
	})

	t.Run("degraded_fallback_parity", func(t *testing.T) {
		t.Parallel()

		ea, sink, published := newSurface(t, false)

		got := ea.Fire(context.Background(), elicEntry())

		if !got.Fallback {
			t.Errorf("outcome = %+v; want the plain-text fallback outcome", got)
		}

		if len(*published) != 1 {
			t.Fatalf("fallback publishes = %d; want exactly the plain-text publish", len(*published))
		}

		turnID, text, _ := strings.Cut((*published)[0], "\x00")
		if turnID != "sess-1-turn-001" {
			t.Errorf("fallback turnID = %q", turnID)
		}

		var qs []session.AskQuestion

		_ = json.Unmarshal(elicEntry().Input, &qs)
		if want := coreexec.RenderAskSurface(qs); text != want {
			t.Errorf("fallback text = %q; want byte-parity RenderAskSurface %q", text, want)
		}

		sink.mu.Lock()
		n := len(sink.msgs)
		sink.mu.Unlock()

		if n != 0 {
			t.Errorf("degraded surface wrote %d registry frames; want zero", n)
		}
	})

	t.Run("method_not_found_sticky_fallback", func(t *testing.T) {
		t.Parallel()

		ea, sink, published := newSurface(t, true)

		out := make(chan session.AskOutcome, 2)

		go func() { out <- ea.Fire(context.Background(), elicEntry()) }()

		req := waitForRequest(t, sink)
		ea.registry.Deliver(&acp.Message{JSONRPC: jsonrpcV20, ID: req.ID,
			Error: &acp.RPCError{Code: acp.CodeMethodNotFound, Message: "not found"}})

		select {
		case got := <-out:
			if !got.Fallback {
				t.Errorf("outcome = %+v; want the probe-degraded fallback (-32601)", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Fire never resolved within 2s")
		}

		// Sticky (16-D-18): the second ask degrades without a new round-trip.
		if got := ea.Fire(context.Background(), elicEntry()); !got.Fallback {
			t.Errorf("second outcome = %+v; want the sticky fallback", got)
		}

		sink.mu.Lock()
		n := len(sink.msgs)
		sink.mu.Unlock()

		if n != 1 {
			t.Errorf("registry frames = %d; want exactly the first ask (no re-probe)", n)
		}

		if len(*published) != 2 {
			t.Errorf("fallback publishes = %d; want one per degraded ask", len(*published))
		}
	})
}

// TestValidateElicitationContent pins the D-10 validator (T-17-11): the closed
// subset — required presence, per-property type, enum/oneOf const membership
// by EXACT string equality, array item-wise checks, minLength/maxLength by
// RUNE count. No regexp, no Unicode normalization (the encoding edge).
func TestValidateElicitationContent(t *testing.T) { //nolint:gocognit,cyclop,funlen,lll // one table over the validation vocabulary
	t.Parallel()

	strSchema := func(extra string) acp.ElicitationSchema {
		prop := `{"type":"` + propTypeString + `","title":"T"` + extra + `}`

		return acp.ElicitationSchema{
			Type:       propTypeObject,
			Properties: map[string]json.RawMessage{"q1": json.RawMessage(prop)},
			Required:   []string{"q1"},
		}
	}

	t.Run("valid_single_string", func(t *testing.T) {
		t.Parallel()

		if v := ValidateElicitationContent(strSchema(``),
			map[string]json.RawMessage{"q1": json.RawMessage(`"ok"`)}); v != "" {
			t.Errorf("valid content flagged: %q", v)
		}
	})

	t.Run("nil_content_missing_required", func(t *testing.T) {
		t.Parallel()

		if v := ValidateElicitationContent(strSchema(``), nil); v == "" {
			t.Error("a null/absent content object must itself be a violation (missing required)")
		}
	})

	t.Run("missing_required_property", func(t *testing.T) {
		t.Parallel()

		if v := ValidateElicitationContent(strSchema(``), map[string]json.RawMessage{}); v == "" {
			t.Error("a missing required property must violate")
		}
	})

	t.Run("wrong_type", func(t *testing.T) {
		t.Parallel()

		if v := ValidateElicitationContent(strSchema(``),
			map[string]json.RawMessage{"q1": json.RawMessage(`true`)}); v == "" {
			t.Error("a boolean value for a string property must violate")
		}
	})

	t.Run("json_null_value_violates", func(t *testing.T) {
		t.Parallel()

		if v := ValidateElicitationContent(strSchema(``),
			map[string]json.RawMessage{"q1": json.RawMessage(`null`)}); v == "" {
			t.Error("a JSON null value must violate (null is not an ElicitationContentValue)")
		}
	})

	t.Run("oneOf_membership_exact_equality", func(t *testing.T) {
		t.Parallel()

		oneOf := `,"oneOf":[{"const":"a","title":"a"},{"const":"b","title":"b"}]`

		if v := ValidateElicitationContent(strSchema(oneOf),
			map[string]json.RawMessage{"q1": json.RawMessage(`"a"`)}); v != "" {
			t.Errorf("a listed const flagged: %q", v)
		}

		if v := ValidateElicitationContent(strSchema(oneOf),
			map[string]json.RawMessage{"q1": json.RawMessage(`"c"`)}); v == "" {
			t.Error("a value outside the oneOf consts must violate")
		}
	})

	t.Run("length_bounds_rune_counted", func(t *testing.T) {
		t.Parallel()

		bounded := `,"minLength":2,"maxLength":5`

		schema := strSchema(bounded)

		// "héllo" is 5 runes / 6 bytes — rune-count semantics accept it.
		if v := ValidateElicitationContent(schema,
			map[string]json.RawMessage{"q1": json.RawMessage(`"héllo"`)}); v != "" {
			t.Errorf("5-rune value within maxLength flagged (byte counting?): %q", v)
		}

		if v := ValidateElicitationContent(schema,
			map[string]json.RawMessage{"q1": json.RawMessage(`"abcdef"`)}); v == "" {
			t.Error("a 6-rune value over maxLength must violate")
		}

		if v := ValidateElicitationContent(schema,
			map[string]json.RawMessage{"q1": json.RawMessage(`"a"`)}); v == "" {
			t.Error("a 1-rune value under minLength must violate")
		}
	})

	t.Run("no_normalization_exact_bytes", func(t *testing.T) {
		t.Parallel()

		nfc := "caf\u00e9"         // U+00E9 — the composed form
		decomposed := "cafe\u0301" // e + combining acute — different bytes, same glyph

		schema := strSchema(`,"oneOf":[{"const":"` + nfc + `","title":"` + nfc + `"}]`)

		payload, _ := json.Marshal(nfc)

		if v := ValidateElicitationContent(schema,
			map[string]json.RawMessage{"q1": payload}); v != "" {
			t.Errorf("the exact const must pass: %q", v)
		}

		// Different bytes, same meaning — WITHOUT normalization this violates.
		dpayload, _ := json.Marshal(decomposed)

		if v := ValidateElicitationContent(schema,
			map[string]json.RawMessage{"q1": dpayload}); v == "" {
			t.Error("a decomposed-variant value must violate (no Unicode normalization)")
		}
	})

	t.Run("array_itemwise_enum", func(t *testing.T) {
		t.Parallel()

		items := `{"type":"` + propTypeArray + `","title":"T",` +
			`"items":{"type":"` + propTypeString + `","enum":["a","b"]}}`

		schema := acp.ElicitationSchema{
			Type:       propTypeObject,
			Properties: map[string]json.RawMessage{"q1": json.RawMessage(items)},
			Required:   []string{"q1"},
		}

		if v := ValidateElicitationContent(schema,
			map[string]json.RawMessage{"q1": json.RawMessage(`["a","b"]`)}); v != "" {
			t.Errorf("listed items flagged: %q", v)
		}

		if v := ValidateElicitationContent(schema,
			map[string]json.RawMessage{"q1": json.RawMessage(`["a","z"]`)}); v == "" {
			t.Error("an item outside the enum must violate (item-wise)")
		}

		if v := ValidateElicitationContent(schema,
			map[string]json.RawMessage{"q1": json.RawMessage(`["a",3]`)}); v == "" {
			t.Error("a non-string item must violate")
		}
	})

	t.Run("unknown_content_keys_ignored", func(t *testing.T) {
		t.Parallel()

		if v := ValidateElicitationContent(strSchema(``),
			map[string]json.RawMessage{"q1": json.RawMessage(`"ok"`), "extra": json.RawMessage(`1`)}); v != "" {
			t.Errorf("unknown content keys are ignored (never rendered): %q", v)
		}
	})
}

// TestFamilyConversionEngineAsk pins the engine/learning ask round-trip
// (17-04, D-09/D-11) through the REAL ask queue and the REAL learning store:
// a background-class entry, an accept writing the learning entry through the
// existing store API (A9), and decline/cancel producing the advisory
// non-answer landing. An invalid accept rides the D-10 loop (one re-ask).
func TestFamilyConversionEngineAsk(t *testing.T) { //nolint:cyclop,funlen,gocognit // one flat round-trip battery
	t.Parallel()

	newEngineSession := func(t *testing.T) *session.Session {
		t.Helper()

		s := &session.Session{SessionID: "sess-engine-1"}
		s.SetPermissionGate(session.GateDeps{Queue: session.NewAskQueue()})

		return s
	}

	store := func(t *testing.T) *learning.Store {
		t.Helper()

		st, err := learning.Open(filepath.Join(t.TempDir(), "learned.yaml"))
		if err != nil {
			t.Fatalf("learning open: %v", err)
		}

		return st
	}

	const situation = "text:engine-ask-situation"

	newFire := func(t *testing.T, outcomes ...session.AskOutcome) (
		func(ctx context.Context, e *session.AskEntry) session.AskOutcome, func() []*session.AskEntry,
	) {
		t.Helper()

		var (
			mu      sync.Mutex
			entries []*session.AskEntry
		)

		fire := func(_ context.Context, e *session.AskEntry) session.AskOutcome {
			mu.Lock()

			entries = append(entries, e)

			mu.Unlock()

			if len(outcomes) == 0 {
				return session.AskOutcome{}
			}

			ans := outcomes[0]
			outcomes = outcomes[1:]

			return ans
		}

		return fire, func() []*session.AskEntry {
			mu.Lock()
			defer mu.Unlock()

			return append([]*session.AskEntry(nil), entries...)
		}
	}

	t.Run("accept_writes_learning_entry", func(t *testing.T) {
		t.Parallel()

		s := newEngineSession(t)
		learned := store(t)
		fire, fired := newFire(t, session.AskOutcome{
			Elicit:  acp.ElicitationActionAccept,
			Content: map[string]json.RawMessage{"q1": json.RawMessage(`"continue after explore"`)},
		})

		accepted := make(chan string, 1)
		declined := make(chan struct{}, 1)

		// The runtime's own onAccept composition (enqueueEngineAsk): the
		// structured seam's answer persists through the EXISTING store API.
		s.EnqueueEngineAsk("turn-1", situation, fire,
			func(answer string) {
				rerr := learned.RecordCandidate(situation, answer, "turn-1")
				if rerr != nil {
					t.Errorf("RecordCandidate: %v", rerr)
				}

				accepted <- answer
			},
			func() { declined <- struct{}{} },
		)

		select {
		case answer := <-accepted:
			if answer != "continue after explore" {
				t.Errorf("answer = %q; want the structured seam's single-field value", answer)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("accept never landed")
		}

		entries := fired()
		if len(entries) != 1 {
			t.Fatalf("entries = %d; want one", len(entries))
		}

		if entries[0].Class != session.AskClassBackground {
			t.Errorf("class = %v; want the subagent/engine background class (D-11)", entries[0].Class)
		}

		e, ok := learned.Lookup(situation)
		if !ok {
			t.Fatal("learning store has no entry for the situation")
		}

		if e.Answer != "continue after explore" {
			t.Errorf("stored answer = %q", e.Answer)
		}
	})

	t.Run("decline_produces_advisory_landing", func(t *testing.T) {
		t.Parallel()

		s := newEngineSession(t)
		learned := store(t)
		fire, fired := newFire(t, session.AskOutcome{Elicit: acp.ElicitationActionDecline})

		declined := make(chan struct{}, 1)

		s.EnqueueEngineAsk("turn-2", situation, fire, nil, func() { declined <- struct{}{} })

		select {
		case <-declined:
		case <-time.After(2 * time.Second):
			t.Fatal("decline never landed")
		}

		if _, ok := learned.Lookup(situation); ok {
			t.Error("decline must not write the learning store")
		}

		if n := len(fired()); n != 1 {
			t.Errorf("asks fired = %d; decline is terminal (zero re-asks)", n)
		}
	})

	t.Run("invalid_accept_reasks_once", func(t *testing.T) {
		t.Parallel()

		s := newEngineSession(t)
		fire, fired := newFire(t,
			session.AskOutcome{Elicit: acp.ElicitationActionAccept, Violation: elicViolation},
			session.AskOutcome{Elicit: acp.ElicitationActionAccept, Violation: elicViolation},
		)

		declined := make(chan struct{}, 1)

		s.EnqueueEngineAsk("turn-3", situation, fire, nil, func() { declined <- struct{}{} })

		select {
		case <-declined:
		case <-time.After(2 * time.Second):
			t.Fatal("the second violation never landed on the decline path")
		}

		entries := fired()
		if len(entries) != 2 {
			t.Fatalf("asks fired = %d; want exactly two (initial + the ONE re-ask)", len(entries))
		}

		if entries[1].Note != elicViolation {
			t.Errorf("re-ask note = %q; want the violation carried", entries[1].Note)
		}
	})
}
