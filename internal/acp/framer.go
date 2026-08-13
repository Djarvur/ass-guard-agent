package acp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
)

var errFrameEmbeddedNewline = errors.New(
	"frame value contains an embedded newline — ACP spec forbids it " +
		"(transports.md: messages MUST NOT contain embedded newlines)")
var errMarshaledFrameNewline = errors.New("marshaled frame contains a raw newline byte — internal invariant violated")
var errEmptyFrameLine = errors.New("empty frame line")

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
		return errFrameEmbeddedNewline
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	// Belt-and-suspenders: the marshaled bytes must not carry a raw newline.
	// json.Marshal guarantees this; the check makes the invariant explicit and
	// catches any future marshaler regression.
	if bytes.ContainsRune(raw, '\n') {
		return errMarshaledFrameNewline
	}

	_, err = w.Write(raw)
	if err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	_, err = w.Write([]byte("\n"))
	if err != nil {
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

	err = json.Unmarshal(raw, &node)
	if err != nil {
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
		if slices.ContainsFunc(v, walkDecodedNewline) {
			return true
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
		return nil, fmt.Errorf("call: %w", err)
	}

	line = bytes.TrimRight(line, "\n")
	if len(line) == 0 {
		return nil, errEmptyFrameLine
	}

	var msg Message

	err = json.Unmarshal(line, &msg)
	if err != nil {
		return nil, fmt.Errorf("unmarshal frame: %w (line=%q)", err, string(line))
	}

	return &msg, nil
}

// Writer is the concurrency-safe, asynchronous stdout emitter. ACP servers have
// a reader goroutine, per-prompt turn goroutines, and the event-bus adapter all
// writing session/update notifications + responses to the same stdout. Writer
// serializes frame writes (no interleaved lines) AND decouples producers from a
// slow client: Write enqueues onto a bounded channel and a single drain goroutine
// writes the frames to the underlying io.Writer in order.
//
// This is load-bearing for the reader goroutine: a parse-error response written
// from the reader must NOT block on a slow client (io.Pipe writes block until
// read), or the reader could not read the next frame. The bounded channel is the
// D-05 backpressure boundary — a full buffer blocks the producer (natural
// slow-down), but the common case (a responsive client) never blocks.
type Writer struct {
	w      io.Writer
	ch     chan *Message
	wg     sync.WaitGroup
	mu     sync.Mutex // guards Close once
	closed bool
}

// newWriter wraps an io.Writer (production: os.Stdout; tests: a bytes.Buffer or
// pipe) and starts the drain goroutine.
func newWriter(w io.Writer) *Writer {
	wtr := &Writer{w: w, ch: make(chan *Message, writeBuffer)}

	wtr.wg.Add(1)

	go wtr.drain()

	return wtr
}

// writeBuffer is the bounded async buffer (D-05). 256 is generous for ACP's
// small frames; a full buffer blocks producers (backpressure).
const writeBuffer = 256

// Write enqueues one Message for the drain goroutine. It blocks if the buffer is
// full (D-05 backpressure — a stuck client stalls the turn rather than growing
// memory unbounded).
func (w *Writer) Write(msg *Message) error {
	w.ch <- msg

	return nil
}

// drain writes enqueued frames to the underlying writer in order, one at a time.
// A write error (e.g. closed pipe on shutdown) is swallowed — best-effort.
func (w *Writer) drain() { //nolint:funcorder // ordering groups related logic
	defer w.wg.Done()

	for msg := range w.ch {
		_ = writeFrame(w.w, msg)
	}
}

// Close shuts down the drain goroutine, flushing any buffered frames. It is
// idempotent. After Close, Write will block forever (do not call Write after
// Close). Tests that inspect a captured stdout MUST Close before reading the
// buffer so all frames are flushed.
func (w *Writer) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()

		return
	}

	w.closed = true
	w.mu.Unlock()
	close(w.ch)
	w.wg.Wait()
}
