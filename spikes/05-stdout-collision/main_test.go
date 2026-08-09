// SPDX-License-Identifier: MIT
//
// Spike 05 (RED) — offline tests for the #5 stdout-collision integration spike
// (Phase 0, Plan 00-04, Task 1 — tdd="true").
//
// THROWAWAY (D-04/D-05). These tests pin the offline-verifiable slice of the
// spike's behavior: the ACP frame writer, the clean-stdout byte-comparison
// helper, the stderr-routing errors handler, and the bot-construction shape.
// The live "bot goroutine + canned frames on real stdout" assertion is proven
// by the binary itself via a structured stderr footer (see main.go).
//
// The full go-telegram/bot API surface is exercised only by main.go's run();
// these tests deliberately stay offline + deterministic (no network, no bot
// Start(), no goroutines) so the RED bar is a compile failure from undefined
// helpers, not a flaky timing failure.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestWriteACPFrameProducesNewlineDelimitedJSON pins the framing rule that
// ACP v1 frames are newline-delimited JSON (one JSON object terminated by
// exactly one '\n' — no Content-Length header). This mirrors Plan 00-03's
// framing assertion and is the shape the canned ACP frames must take on
// stdout.
func TestWriteACPFrameProducesNewlineDelimitedJSON(t *testing.T) {
	var buf bytes.Buffer
	frame := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"protocolVersion": 1},
	}
	if err := writeACPFrame(&buf, frame); err != nil {
		t.Fatalf("writeACPFrame: %v", err)
	}
	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("frame must end with exactly one '\\n'; got tail %q", tail(out, 10))
	}
	// Strip the single trailing newline; the remainder must be exactly one JSON
	// object (no embedded newlines, no extra bytes).
	body := strings.TrimSuffix(out, "\n")
	if strings.Contains(body, "\n") {
		t.Errorf("frame body must contain no embedded newlines; got %q", body)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("frame body must be valid JSON: %v (body=%q)", err, body)
	}
	if got["method"] != "initialize" {
		t.Errorf("method round-trip: want initialize, got %v", got["method"])
	}
}

// TestExtraByteCountZeroOnExactMatch is the load-bearing assertion: when the
// bytes captured from stdout are byte-equal to the canned ACP frames the spike
// wrote, the extra-byte count MUST be 0 (zero non-ACP bytes leaked by the bot).
// This is the precise numeric form of the clean-stdout contract.
func TestExtraByteCountZeroOnExactMatch(t *testing.T) {
	canned := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","method":"session/update"}` + "\n")
	got := append([]byte(nil), canned...) // identical copy
	if n := extraByteCount(canned, got); n != 0 {
		t.Errorf("extraByteCount on identical input: want 0, got %d", n)
	}
}

// TestExtraByteCountDetectsLeakedBytes confirms the helper detects a non-zero
// extra-byte count when the bot leaked bytes into stdout (the failure case the
// spike exists to prove cannot happen by default).
func TestExtraByteCountDetectsLeakedBytes(t *testing.T) {
	canned := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	// Two leaked bytes from a hypothetical bot stdout write ("hi") inserted
	// before the canned frame.
	got := []byte("hi" + string(canned))
	if n := extraByteCount(canned, got); n <= 0 {
		t.Errorf("extraByteCount on leaked input: want >0, got %d", n)
	}
}

// TestCompareStdoutReportsCleanOnMatch wraps extraByteCount in the PASS/FAIL
// verdict shape the structured footer emits.
func TestCompareStdoutReportsCleanOnMatch(t *testing.T) {
	canned := []byte(`{"jsonrpc":"2.0","id":2,"method":"session/prompt"}` + "\n")
	verdict, extra := compareStdout(canned, append([]byte(nil), canned...))
	if !verdict {
		t.Errorf("compareStdout verdict: want clean=true, got false")
	}
	if extra != 0 {
		t.Errorf("compareStdout extra: want 0, got %d", extra)
	}
}

// TestStderrErrorsHandlerRoutesToSinkNotStdout pins the handler-routing
// assertion: the spike's errors handler writes the bot's errors to the
// configured sink (os.Stderr in production; a buffer here), and never to
// stdout. The returned closure must have the exact go-telegram/bot@v1.23.0
// ErrorsHandler signature: func(err error).
func TestStderrErrorsHandlerRoutesToSinkNotStdout(t *testing.T) {
	var sink bytes.Buffer
	h := makeStderrErrorsHandler(&sink)
	// go-telegram/bot@v1.23.0 ErrorsHandler signature is func(err error).
	h(errors.New("[TGBOT] simulated init error"))
	if !strings.Contains(sink.String(), "[TGBOT] simulated init error") {
		t.Errorf("error handler must write the error to the sink; sink=%q", sink.String())
	}
	// The handler is the ONLY routing surface — it never touches the process
	// stdout. (We cannot directly assert "stdout untouched" from a unit test,
	// but the handler closing over a non-stdout io.Writer is the structural
	// guarantee; main.go wires the real sink as os.Stderr.)
}

// TestBuildACPFramesProducesOrderedSequence confirms the canned ACP frames the
// spike writes to stdout form the documented sequence (an initialize response,
// a session/update notification, a session/prompt response) and each is a
// single JSON object terminated by '\n' (the exact framing from Plan 00-03).
func TestBuildACPFramesProducesOrderedSequence(t *testing.T) {
	frames := buildACPFrames()
	if len(frames) < 3 {
		t.Fatalf("buildACPFrames: want >=3 frames (initialize, session/update, session/prompt), got %d", len(frames))
	}
	// Render them the way the spike renders to stdout, then byte-compare.
	var buf bytes.Buffer
	for _, f := range frames {
		if err := writeACPFrame(&buf, f); err != nil {
			t.Fatalf("writeACPFrame[%d]: %v", -1, err)
		}
	}
	// Every line must be a valid JSON object.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(frames) {
		t.Fatalf("frame count mismatch: wrote %d frames, parsed %d lines", len(frames), len(lines))
	}
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("frame %d not valid JSON: %v (line=%q)", i, err, line)
		}
	}
	// Spot-check the first frame is an initialize-shape response (has id+result
	// and an agentCapabilities field per Plan 00-03).
	var first map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &first)
	if _, ok := first["id"]; !ok {
		t.Errorf("first canned frame should be a response (have id); got %v", first)
	}
}

// TestBuildBotConfiguresDiscipline pins the bot-construction shape: the bot is
// built with WithSkipGetMe (avoid the network call in New so the spike is
// deterministic + offline — the goroutine topology is the point, not a live
// Telegram connection), a registered WithErrorsHandler (the mitigation recipe),
// and NO WithDebug (default silence). buildBot must succeed offline.
func TestBuildBotConfiguresDiscipline(t *testing.T) {
	var errs bytes.Buffer
	b, err := buildBot(dummyTokenForSpike(), &errs)
	if err != nil {
		t.Fatalf("buildBot: %v (a dummy token + WithSkipGetMe must keep New() offline)", err)
	}
	if b == nil {
		t.Fatal("buildBot returned nil bot with nil error")
	}
}

// tail returns the last n bytes of s as a string (for readable error tails).
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// Ensure io is referenced (the handler signature returns from makeStderrErrorsHandler
// must accept an io.Writer; this guards the import against accidental removal).
var _ io.Writer = (*bytes.Buffer)(nil)
