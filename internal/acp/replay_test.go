package acp //nolint:testpackage // internal package test

// 18-01 Task 2: the replay mapping battery — table-driven subtests over
// hand-written fixture transcripts in temp dirs, driven through the recording
// emitter (the tolerant-reader + full agent-visible vocabulary pin).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// recordedFrame is one observed emission (the recording emitter's log entry).
type recordedFrame struct {
	kind       string // sessionUpdate kind value
	messageID  string
	text       string
	toolCallID string
	title      string
	status     string
}

// recordingEmitter records every ReplayTranscript emission — both the chunk
// surface and the ActivityEmitter surface — in arrival order.
type recordingEmitter struct {
	mu     sync.Mutex
	frames []recordedFrame
}

func (r *recordingEmitter) AgentMessageChunk(messageID, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames,
		recordedFrame{kind: updKindAgentMessageChunk, messageID: messageID, text: text})

	return nil
}

func (r *recordingEmitter) ToolCall(frame *ToolCallFrame) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames, recordedFrame{
		kind: updKindToolCall, toolCallID: frame.ToolCallID, title: frame.Title,
	})

	return nil
}

func (r *recordingEmitter) ToolCallUpdate(frame *ToolCallUpdateFrame) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames, recordedFrame{
		kind: updKindToolCallUpdate, toolCallID: frame.ToolCallID, status: frame.Status,
	})

	return nil
}

func (r *recordingEmitter) PlanUpdate(_ []PlanEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames, recordedFrame{kind: updKindPlan})

	return nil
}

func (r *recordingEmitter) ThoughtChunk(messageID string, content ContentBlock) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frames = append(r.frames,
		recordedFrame{kind: updKindThoughtChunk, messageID: messageID, text: content.Text})

	return nil
}

func (r *recordingEmitter) snapshot() []recordedFrame {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]recordedFrame(nil), r.frames...)
}

// Fixture vocabulary (goconst-extracted; shared with the session-family
// battery in session_family_test.go).
const (
	replayFixtureSID = "aaaaaaaa-0b0b-4c0c-8d0d-0e0e0e0e0e0e"
	fixtureTurnOne   = "s-turn-001"
	fixtureToolBash  = "Bash"
	fixtureToolRead  = "Read"
	fixtureCallRead  = "tc-1"
)

// Fixture line builders: one place per line shape (the table stays readable;
// the JSON literals never repeat).
func fixtureUserLine(turn, text string) string {
	return `{"type":"user_message","turnID":"` + turn + `","content":[{"type":"text","text":"` + text + `"}]}`
}

func fixtureChunkLine(turn, messageID, text string) string {
	return `{"type":"agent_message_chunk","turnID":"` + turn + `","messageID":"` + messageID +
		`","text":"` + text + `"}`
}

func fixtureAssistantLine(turn, text string) string {
	return `{"type":"assistant_message","turnID":"` + turn + `","text":"` + text + `"}`
}

func fixtureToolCallLine(turn, callID, name string) string {
	return `{"type":"tool_call","turnID":"` + turn + `","toolCallID":"` + callID + `","name":"` + name + `"}`
}

func fixtureToolResultLine(turn, callID string, isError bool) string {
	isErr := "false"
	if isError {
		isErr = "true"
	}

	return `{"type":"tool_result","turnID":"` + turn + `","toolCallID":"` + callID +
		`","output":{"out":1},"isError":` + isErr + `}`
}

func fixtureErrorLine(turn, callID, component, message string) string {
	idField := ""
	if callID != "" {
		idField = `,"toolCallID":"` + callID + `"`
	}

	return `{"type":"error","turnID":"` + turn + `"` + idField + `,"component":"` + component +
		`","message":"` + message + `"}`
}

func fixtureCanceledLine(turn, reason string) string {
	return `{"type":"canceled","turnID":"` + turn + `","text":"` + reason + `"}`
}

// writeReplayFixture writes dir/.ass-guard/transcript_<sid>.jsonl from lines
// (no trailing newline on the LAST line when torn is set — the torn-line case).
func writeReplayFixture(t *testing.T, dir, sid string, lines []string, torn bool) {
	t.Helper()

	err := os.MkdirAll(filepath.Join(dir, ".ass-guard"), 0o750)
	if err != nil {
		t.Fatalf("mkdir fixture store: %v", err)
	}

	body := strings.Join(lines, "\n")
	if !torn && len(lines) > 0 {
		body += "\n"
	}

	err = os.WriteFile(filepath.Join(dir, ".ass-guard", "transcript_"+sid+".jsonl"),
		[]byte(body), 0o600)
	if err != nil {
		t.Fatalf("write fixture transcript: %v", err)
	}
}

// assertFrames pins the exact frame sequence (order + fields).
func assertFrames(t *testing.T, got, want []recordedFrame) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("frame count = %d; want %d (got=%+v)", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("frame[%d] = %+v; want %+v", i, got[i], want[i])
		}
	}
}

// TestReplayTranscriptMapping is the full agent-visible vocabulary pin: every
// agent-visible line maps to EXACTLY ONE frame in transcript order; error
// lines render terminal (toolCallID-carrying) or as text (bare); canceled
// lines add nothing; torn tails and unknown future kinds skip tolerantly
// (16-D-20).
//
//nolint:funlen // a table over the whole vocabulary
func TestReplayTranscriptMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		lines []string
		torn  bool
		want  []recordedFrame
	}{
		{
			name: "error line with toolCallID closes the call as failed",
			lines: []string{
				fixtureToolCallLine(fixtureTurnOne, "tc-9", fixtureToolBash),
				fixtureErrorLine(fixtureTurnOne, "tc-9", "toolexec", "boom"),
			},
			want: []recordedFrame{
				{kind: updKindToolCall, toolCallID: "tc-9", title: fixtureToolBash},
				{kind: updKindToolCallUpdate, toolCallID: "tc-9", status: StatusFailed},
			},
		},
		{
			name: "bare error line renders as an agent chunk carrying the error text",
			lines: []string{
				fixtureErrorLine(fixtureTurnOne, "", "provider", "stream reset"),
			},
			want: []recordedFrame{
				{
					kind: updKindAgentMessageChunk, messageID: fixtureTurnOne,
					text: "provider: stream reset",
				},
			},
		},
		{
			name: "canceled turn emits no frames beyond its tool lines",
			lines: []string{
				fixtureUserLine(fixtureTurnOne, "go"),
				fixtureChunkLine(fixtureTurnOne, fixtureTurnOne, "partial"),
				fixtureToolCallLine(fixtureTurnOne, fixtureCallRead, fixtureToolRead),
				fixtureToolResultLine(fixtureTurnOne, fixtureCallRead, false),
				fixtureCanceledLine(fixtureTurnOne, "context cancelled during stream"),
			},
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: fixtureTurnOne, text: "partial"},
				{kind: updKindToolCall, toolCallID: fixtureCallRead, title: fixtureToolRead},
				{kind: updKindToolCallUpdate, toolCallID: fixtureCallRead, status: StatusCompleted},
			},
		},
		{
			name: "interleaved pairs across turns map one frame per line in order",
			lines: []string{
				fixtureUserLine(fixtureTurnOne, "a"),
				fixtureToolCallLine(fixtureTurnOne, "tc-a", fixtureToolRead),
				fixtureToolCallLine(fixtureTurnOne, "tc-b", "Grep"),
				fixtureToolResultLine(fixtureTurnOne, "tc-b", false),
				fixtureToolResultLine(fixtureTurnOne, "tc-a", true),
				fixtureAssistantLine(fixtureTurnOne, "done one"),
				fixtureUserLine("s-turn-002", "b"),
				fixtureToolCallLine("s-turn-002", "tc-c", fixtureToolBash),
				fixtureToolResultLine("s-turn-002", "tc-c", false),
				fixtureAssistantLine("s-turn-002", "done two"),
			},
			want: []recordedFrame{
				{kind: updKindToolCall, toolCallID: "tc-a", title: fixtureToolRead},
				{kind: updKindToolCall, toolCallID: "tc-b", title: "Grep"},
				{kind: updKindToolCallUpdate, toolCallID: "tc-b", status: StatusCompleted},
				{kind: updKindToolCallUpdate, toolCallID: "tc-a", status: StatusFailed},
				{kind: updKindAgentMessageChunk, messageID: fixtureTurnOne, text: "done one"},
				{kind: updKindToolCall, toolCallID: "tc-c", title: fixtureToolBash},
				{kind: updKindToolCallUpdate, toolCallID: "tc-c", status: StatusCompleted},
				{kind: updKindAgentMessageChunk, messageID: "s-turn-002", text: "done two"},
			},
		},
		{
			name: "chunk stream closed by a much-later assistant_message keeps order",
			lines: []string{
				fixtureChunkLine(fixtureTurnOne, "m1", "think "),
				fixtureToolCallLine(fixtureTurnOne, fixtureCallRead, fixtureToolRead),
				fixtureToolResultLine(fixtureTurnOne, fixtureCallRead, false),
				fixtureChunkLine(fixtureTurnOne, "m1", "some more"),
				fixtureAssistantLine(fixtureTurnOne, "think some more"),
			},
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "think "},
				{kind: updKindToolCall, toolCallID: fixtureCallRead, title: fixtureToolRead},
				{kind: updKindToolCallUpdate, toolCallID: fixtureCallRead, status: StatusCompleted},
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "some more"},
				{kind: updKindAgentMessageChunk, messageID: fixtureTurnOne, text: "think some more"},
			},
		},
		{
			name: "torn trailing garbage line is skipped without error",
			lines: []string{
				fixtureChunkLine(fixtureTurnOne, "m1", "ok"),
				`{"type":"agent_message_chunk","turnID":"` + fixtureTurnOne + `","text":"tor`,
			},
			torn: true,
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "ok"},
			},
		},
		{
			name: "unknown future kind is skipped, known kinds still frame",
			lines: []string{
				`{"type":"quantum_teleport","turnID":"` + fixtureTurnOne + `","qubit":"spin-up"}`,
				fixtureChunkLine(fixtureTurnOne, "m1", "known"),
			},
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "known"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			writeReplayFixture(t, dir, replayFixtureSID, tc.lines, tc.torn)

			rec := &recordingEmitter{}

			err := ReplayTranscript(rec, dir, replayFixtureSID)
			if err != nil {
				t.Fatalf("ReplayTranscript: %v", err)
			}

			assertFrames(t, rec.snapshot(), tc.want)
		})
	}
}

// TestReplayTranscriptRejectsBadSource pins the exported surface's guards: a
// malformed id rejects before any file open; a missing transcript errors; a
// planted symlink (non-regular source) rejects with the typed error (T-18-03).
func TestReplayTranscriptRejectsBadSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	rec := &recordingEmitter{}

	err := ReplayTranscript(rec, dir, "../../escape")
	if !errors.Is(err, errReplayMalformedID) {
		t.Errorf("malformed id error = %v; want errReplayMalformedID", err)
	}

	err = ReplayTranscript(rec, dir, replayFixtureSID)
	if err == nil {
		t.Error("missing transcript replay succeeded; want an error")
	}

	// Plant a symlink where the transcript belongs: the Lstat guard rejects
	// the link itself (T-18-03) — even one pointing at a regular file.
	target := filepath.Join(t.TempDir(), "real.jsonl")

	werr := os.WriteFile(target, []byte("{}\n"), 0o600)
	if werr != nil {
		t.Fatalf("write symlink target: %v", werr)
	}

	store := filepath.Join(dir, ".ass-guard")

	merr := os.MkdirAll(store, 0o750)
	if merr != nil {
		t.Fatalf("mkdir store: %v", merr)
	}

	link := filepath.Join(store, "transcript_"+replayFixtureSID+".jsonl")

	serr := os.Symlink(target, link)
	if serr != nil {
		t.Fatalf("symlink fixture: %v", serr)
	}

	err = ReplayTranscript(rec, dir, replayFixtureSID)
	if !errors.Is(err, errReplayNotRegular) {
		t.Errorf("symlink replay error = %v; want errReplayNotRegular", err)
	}
}
