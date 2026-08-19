package acp //nolint:testpackage // internal package test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// TestWriteFrameProducesMarshalPlusNewline verifies writeFrame emits exactly
// json.Marshal(v) followed by a single '\n' (ACP v1 newline-delimited framing,
// VERIFIED-FACTS #3 / transports.md). No raw newline may appear in the body.
func TestWriteFrameProducesMarshalPlusNewline(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	msg := Message{JSONRPC: protocolVersion20, ID: json.RawMessage("1"), Method: methodInitialize}

	err := writeFrame(&buf, msg)
	if err != nil {
		t.Fatalf("writeFrame: %v", err)
	}

	want, marshalErr := json.Marshal(msg)
	if marshalErr != nil {
		panic(marshalErr)
	}

	want = append(want, '\n')

	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("writeFrame output = %q; want %q", buf.String(), string(want))
	}

	if bytes.Count(buf.Bytes(), []byte{'\n'}) != 1 {
		t.Errorf("output has %d newline bytes; want exactly 1", bytes.Count(buf.Bytes(), []byte{'\n'}))
	}
}

// TestWriteFrameAllowsDecodedNewline verifies writeFrame EMITS frames whose
// decoded text carries an embedded newline. The ACP spec's "MUST NOT contain
// embedded newlines" (transports.md) is a WIRE-BYTES framing rule: json.Marshal
// escapes '\n' inside strings to the two-byte "\\n" sequence, so a decoded
// newline can never split the frame. Rejecting decoded newlines silently dropped
// the model's own newline-carrying text chunks from the live wire (12-01
// witness session d9f98023: 450 transcript chunks vs 440 wire frames). Relaxed
// by operator disposition 260819-nlg; the raw-byte check stays below.
func TestWriteFrameAllowsDecodedNewline(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		v    any
	}{
		{"map string field with newline", map[string]any{blockText: "line1\nline2"}},
		{"nested slice string with newline", map[string]any{"items": []any{"a\nb"}}},
		{"RawMessage param whose decoded string carries a newline", Message{
			JSONRPC: protocolVersion20, Method: "session/prompt",
			// Valid JSON (the \n is the two-char escape); the decoded text value
			// is "x<newline>y".
			Params: json.RawMessage(`{"prompt":[{"type":"text","text":"x\ny"}]}`),
		}},
		{"agent_message_chunk with multi-line text (the 12-01 drop class)", Message{
			JSONRPC: protocolVersion20, Method: methodSessionUpdate,
			Params: json.RawMessage(`{"sessionId":"s","update":{` +
				`"sessionUpdate":"agent_message_chunk","messageId":"m",` +
				`"content":{"type":"text","text":"first line\nsecond line"}}}`),
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			err := writeFrame(&buf, c.v)
			if err != nil {
				t.Fatalf("writeFrame rejected a decoded embedded newline: %v", err)
			}

			// Wire invariant (unchanged): exactly ONE newline, the trailing
			// frame terminator; no raw newline byte inside the body.
			if got := bytes.Count(buf.Bytes(), []byte{'\n'}); got != 1 {
				t.Errorf("output has %d newline bytes; want exactly 1 (trailing)", got)
			}

			if bytes.Contains(buf.Bytes()[:len(buf.Bytes())-1], []byte{'\n'}) {
				t.Errorf("frame body carries a raw newline byte: %q", buf.String())
			}

			// Content integrity: the decoded round-trip preserves the newline
			// (the editor must receive the model's line breaks).
			var node any

			err = json.Unmarshal(buf.Bytes(), &node)
			if err != nil {
				t.Fatalf("output does not round-trip as JSON: %v", err)
			}

			if !treeHasNewline(node) {
				t.Errorf("decoded frame lost the embedded newline; want it preserved: %q", buf.String())
			}
		})
	}
}

// treeHasNewline reports whether any string in the decoded JSON tree contains a
// literal newline (content-integrity check for the relax disposition).
func treeHasNewline(node any) bool {
	switch v := node.(type) {
	case string:
		return strings.Contains(v, "\n")
	case map[string]any:
		for _, item := range v {
			if treeHasNewline(item) {
				return true
			}
		}
	case []any:
		if slices.ContainsFunc(v, treeHasNewline) {
			return true
		}
	}

	return false
}

// TestWriteFrameRejectsRawNewlineBytes verifies the belt-and-suspenders raw-byte
// check stays: no raw 0x0A may reach the wire body. A Marshaler emitting a raw
// newline inside a string literal is rejected by the stdlib scanner on compact
// — writeFrame must error and write nothing, keeping the wire line-clean.
func TestWriteFrameRejectsRawNewlineBytes(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := writeFrame(&buf, rawNewlineMarshaler{})
	if err == nil {
		t.Fatal("writeFrame accepted a raw newline byte in the marshaled body; want error")
	}

	if buf.Len() != 0 {
		t.Errorf("writeFrame wrote %d bytes before failing; must write nothing", buf.Len())
	}
}

// rawNewlineMarshaler is a json.Marshaler whose output embeds a raw 0x0A inside
// a string literal — invalid JSON the stdlib scanner rejects on compact.
type rawNewlineMarshaler struct{}

func (rawNewlineMarshaler) MarshalJSON() ([]byte, error) {
	return []byte("\"x\ny\""), nil
}

// TestReadFrameParsesOneLine verifies readFrame reads exactly one newline-
// delimited JSON object and returns the parsed *Message. At clean end-of-input
// it returns io.EOF with no partial frame.
func TestReadFrameParsesOneLine(t *testing.T) {
	t.Parallel()

	frame := []byte(`{"jsonrpc":"2.0","id":3,"method":"session/new"}` + "\n")
	r := bufio.NewReader(bytes.NewReader(frame))

	msg, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}

	if msg.Method != "session/new" {
		t.Errorf("method = %q; want session/new", msg.Method)
	}

	if msg.ID == nil || string(msg.ID) != "3" {
		t.Errorf("id = %v; want 3", msg.ID)
	}
	// Next read at clean EOF returns io.EOF.
	_, err = readFrame(r)
	if !errors.Is(err, io.EOF) {
		t.Errorf("readFrame at EOF = %v; want io.EOF", err)
	}
}

// TestReadFrameMalformedJSON verifies readFrame returns an error for a truncated
// frame WITHOUT crashing the reader (a subsequent well-formed frame can still be
// read on the next call — the server keeps reading, Pitfall/transport rule).
func TestReadFrameMalformedJSON(t *testing.T) {
	t.Parallel()

	in := []byte("{\"jsonrpc\":\"2.0\",TRUNCATED\n{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n")

	r := bufio.NewReader(bytes.NewReader(in))

	_, err := readFrame(r)
	if err == nil {
		t.Fatal("readFrame returned nil error for malformed JSON; want parse error")
	}
	// The reader is still usable: the next well-formed line parses.
	msg, err := readFrame(r)
	if err != nil {
		t.Fatalf("second readFrame: %v", err)
	}

	if msg.Method != methodInitialize {
		t.Errorf("second frame method = %q; want initialize", msg.Method)
	}
}

// TestWriterConcurrentSafety verifies Writer is safe under concurrent writes:
// 100 goroutines each writing a distinct frame produce 100 well-formed newline-
// delimited lines in the output. Run with -race (load-bearing).
func TestWriterConcurrentSafety(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	w := newWriter(&buf)

	const n = 100

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			id := json.RawMessage(strconv.Itoa(i))
			_ = w.Write(&Message{JSONRPC: protocolVersion20, ID: id, Method: methodSessionUpdate})
		}(i)
	}

	wg.Wait()
	// Close flushes the drain goroutine (all frames written) before we inspect.
	w.Close()

	lines := bytes.Split(buf.Bytes(), []byte{'\n'})
	// Last element is an empty trailing string after the final newline.
	if len(lines) != n+1 || len(lines[n]) != 0 {
		t.Fatalf("got %d line segments; want %d frames + 1 trailing empty", len(lines), n)
	}

	seen := map[string]bool{}

	for i, line := range lines[:n] {
		if len(line) == 0 {
			t.Errorf("line %d is empty", i)

			continue
		}

		var m Message

		err := json.Unmarshal(line, &m)
		if err != nil {
			t.Errorf("line %d unmarshal: %v (line=%q)", i, err, string(line))

			continue
		}

		if m.ID == nil {
			t.Errorf("line %d has nil id", i)

			continue
		}

		seen[string(m.ID)] = true
	}

	if len(seen) != n {
		t.Errorf("decoded %d distinct ids; want %d (lost writes?)", len(seen), n)
	}
}

// TestMessageIDNilIsNotification verifies the Message envelope's id field is
// omitted when ID is nil (a notification carries no id) and present when ID is
// non-nil (a request/response carries an id). This is the JSON-RPC notification-
// vs-request distinction (VERIFIED-FACTS #3 Note 5: session/update has no id).
func TestMessageIDNilIsNotification(t *testing.T) {
	t.Parallel()

	notif := Message{JSONRPC: protocolVersion20, Method: methodSessionUpdate}

	raw, err := json.Marshal(notif)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}

	if bytes.Contains(raw, []byte(`"id"`)) {
		t.Errorf("notification marshaled with an id field: %s", string(raw))
	}

	req := Message{JSONRPC: protocolVersion20, ID: json.RawMessage("7"), Method: methodInitialize}

	raw, err = json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	if !bytes.Contains(raw, []byte(`"id":7`)) {
		t.Errorf("request did not marshal id=7: %s", string(raw))
	}
	// id:0 must round-trip (0 is a valid JSON-RPC id; omitempty on *int keeps it).
	zeroReq := Message{JSONRPC: protocolVersion20, ID: json.RawMessage("0"), Method: methodInitialize}

	raw, err = json.Marshal(zeroReq)
	if err != nil {
		t.Fatalf("marshal zero-id request: %v", err)
	}

	if !bytes.Contains(raw, []byte(`"id":0`)) {
		t.Errorf("id=0 request did not marshal id field: %s (0 is a valid id)", string(raw))
	}
}
