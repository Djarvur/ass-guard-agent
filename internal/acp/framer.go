package acp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// writeFrame marshals v as a single JSON object followed by exactly one '\n'
// (ACP v1 newline-delimited framing — VERIFIED-FACTS #3 / transports.md). It
// rejects values whose DECODED form contains an embedded newline anywhere in a
// string field: the ACP spec forbids embedded newlines at the protocol level
// ("Messages MUST NOT contain embedded newlines"), and a peer using a naive
// line scanner would otherwise split the frame. json.Marshal escapes '\n' to
// the two-byte "\\n" sequence so the marshaled bytes themselves never carry a
// raw 0x0A, but the spec rule is about the *decoded* value.
//
// The decoded-newline check marshals v, re-decodes it into a generic tree, and
// walks for any string containing '\n'. This uniformly handles structs, maps,
// slices, and json.RawMessage params.
func writeFrame(w io.Writer, v any) error {
	if containsDecodedNewline(v) {
		return errors.New("frame value contains an embedded newline — ACP spec forbids it (transports.md: messages MUST NOT contain embedded newlines)")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	// Belt-and-suspenders: the marshaled bytes must not carry a raw newline.
	// json.Marshal guarantees this; the check makes the invariant explicit and
	// catches any future marshaler regression.
	if bytes.ContainsRune(raw, '\n') {
		return errors.New("marshaled frame contains a raw newline byte — internal invariant violated")
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write frame newline: %w", err)
	}
	return nil
}

// containsDecodedNewline reports whether v's decoded JSON form carries a string
// with a literal '\n'. It marshals v, decodes into a generic tree, and walks it.
func containsDecodedNewline(v any) bool {
	raw, err := json.Marshal(v)
	if err != nil {
		// Let writeFrame surface the marshal error.
		return false
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return false
	}
	return walkDecodedNewline(node)
}

// walkDecodedNewline recurses through the decoded JSON tree looking for any
// string value that contains a literal newline.
func walkDecodedNewline(node any) bool {
	switch v := node.(type) {
	case string:
		return strings.Contains(v, "\n")
	case map[string]any:
		for _, item := range v {
			if walkDecodedNewline(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if walkDecodedNewline(item) {
				return true
			}
		}
	}
	return false
}

// readFrame reads exactly one newline-delimited JSON object from r and returns
// the parsed *Message. At clean end-of-input it returns io.EOF with no partial
// frame. Malformed JSON yields an error (the caller surfaces a -32700 parse
// error and keeps reading — the reader is still usable for the next line).
func readFrame(r *bufio.Reader) (*Message, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		// io.EOF with no bytes is a clean end-of-input; any partial line with
		// io.EOF is a truncated frame and surfaces as an error.
		return nil, err
	}
	line = bytes.TrimRight(line, "\n")
	if len(line) == 0 {
		return nil, errors.New("empty frame line")
	}
	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal frame: %w (line=%q)", err, string(line))
	}
	return &msg, nil
}

// Writer is the concurrency-safe stdout emitter. ACP servers may have the reader
// goroutine, per-prompt turn goroutines, and the event-bus adapter all writing
// session/update notifications + responses to the same stdout; the mutex
// serializes frame writes so no line is interleaved.
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// newWriter wraps an io.Writer (production: os.Stdout; tests: a bytes.Buffer).
func newWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Write writes one Message as a single newline-delimited frame under the mutex.
func (w *Writer) Write(msg Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return writeFrame(w.w, msg)
}
