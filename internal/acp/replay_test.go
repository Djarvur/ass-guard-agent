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

func (r *recordingEmitter) PlanUpdate(entries []PlanEntry) error {
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

// replayFixtureSID is a loadSessIDPattern-clean UUID-form fixture id.
const replayFixtureSID = "aaaaaaaa-0b0b-4c0c-8d0d-0e0e0e0e0e0e"

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
//nolint:funlen,gocyclo,cyclop,maintidx // a table over the whole vocabulary
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
				`{"type":"tool_call","turnID":"s-turn-001","toolCallID":"tc-9","name":"Bash"}`,
				`{"type":"error","turnID":"s-turn-001","toolCallID":"tc-9","component":"toolexec","message":"boom"}`,
			},
			want: []recordedFrame{
				{kind: updKindToolCall, toolCallID: "tc-9", title: "Bash"},
				{kind: updKindToolCallUpdate, toolCallID: "tc-9", status: StatusFailed},
			},
		},
		{
			name: "bare error line renders as an agent chunk carrying the error text",
			lines: []string{
				`{"type":"error","turnID":"s-turn-001","component":"provider","message":"stream reset"}`,
			},
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "s-turn-001", text: "provider: stream reset"},
			},
		},
		{
			name: "canceled turn emits no frames beyond its tool lines",
			lines: []string{
				`{"type":"user_message","turnID":"s-turn-001","content":[{"type":"text","text":"go"}]}`,
				`{"type":"agent_message_chunk","turnID":"s-turn-001","messageID":"s-turn-001","text":"partial"}`,
				`{"type":"tool_call","turnID":"s-turn-001","toolCallID":"tc-1","name":"Read"}`,
				`{"type":"tool_result","turnID":"s-turn-001","toolCallID":"tc-1","output":{"ok":true}}`,
				`{"type":"canceled","turnID":"s-turn-001","text":"context cancelled during stream"}`,
			},
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "s-turn-001", text: "partial"},
				{kind: updKindToolCall, toolCallID: "tc-1", title: "Read"},
				{kind: updKindToolCallUpdate, toolCallID: "tc-1", status: StatusCompleted},
			},
		},
		{
			name: "interleaved pairs across turns map one frame per line in order",
			lines: []string{
				`{"type":"user_message","turnID":"s-turn-001","content":[{"type":"text","text":"a"}]}`,
				`{"type":"tool_call","turnID":"s-turn-001","toolCallID":"tc-a","name":"Read"}`,
				`{"type":"tool_call","turnID":"s-turn-001","toolCallID":"tc-b","name":"Grep"}`,
				`{"type":"tool_result","turnID":"s-turn-001","toolCallID":"tc-b","output":{"hits":2}}`,
				`{"type":"tool_result","turnID":"s-turn-001","toolCallID":"tc-a","output":{"text":"x"},"isError":true}`,
				`{"type":"assistant_message","turnID":"s-turn-001","text":"done one"}`,
				`{"type":"user_message","turnID":"s-turn-002","content":[{"type":"text","text":"b"}]}`,
				`{"type":"tool_call","turnID":"s-turn-002","toolCallID":"tc-c","name":"Bash"}`,
				`{"type":"tool_result","turnID":"s-turn-002","toolCallID":"tc-c","output":{"stdout":"y"}}`,
				`{"type":"assistant_message","turnID":"s-turn-002","text":"done two"}`,
			},
			want: []recordedFrame{
				{kind: updKindToolCall, toolCallID: "tc-a", title: "Read"},
				{kind: updKindToolCall, toolCallID: "tc-b", title: "Grep"},
				{kind: updKindToolCallUpdate, toolCallID: "tc-b", status: StatusCompleted},
				{kind: updKindToolCallUpdate, toolCallID: "tc-a", status: StatusFailed},
				{kind: updKindAgentMessageChunk, messageID: "s-turn-001", text: "done one"},
				{kind: updKindToolCall, toolCallID: "tc-c", title: "Bash"},
				{kind: updKindToolCallUpdate, toolCallID: "tc-c", status: StatusCompleted},
				{kind: updKindAgentMessageChunk, messageID: "s-turn-002", text: "done two"},
			},
		},
		{
			name: "chunk stream closed by a much-later assistant_message keeps order",
			lines: []string{
				`{"type":"agent_message_chunk","turnID":"s-turn-001","messageID":"m1","text":"think "}`,
				`{"type":"tool_call","turnID":"s-turn-001","toolCallID":"tc-1","name":"Read"}`,
				`{"type":"tool_result","turnID":"s-turn-001","toolCallID":"tc-1","output":{"text":"f"}}`,
				`{"type":"agent_message_chunk","turnID":"s-turn-001","messageID":"m1","text":"some more"}`,
				`{"type":"assistant_message","turnID":"s-turn-001","text":"think some more"}`,
			},
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "think "},
				{kind: updKindToolCall, toolCallID: "tc-1", title: "Read"},
				{kind: updKindToolCallUpdate, toolCallID: "tc-1", status: StatusCompleted},
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "some more"},
				{kind: updKindAgentMessageChunk, messageID: "s-turn-001", text: "think some more"},
			},
		},
		{
			name: "torn trailing garbage line is skipped without error",
			lines: []string{
				`{"type":"agent_message_chunk","turnID":"s-turn-001","messageID":"m1","text":"ok"}`,
				`{"type":"agent_message_chunk","turnID":"s-turn-001","messageID":"m1","text":"tor`,
			},
			torn: true,
			want: []recordedFrame{
				{kind: updKindAgentMessageChunk, messageID: "m1", text: "ok"},
			},
		},
		{
			name: "unknown future kind is skipped, known kinds still frame",
			lines: []string{
				`{"type":"quantum_teleport","turnID":"s-turn-001","qubit":"spin-up"}`,
				`{"type":"agent_message_chunk","turnID":"s-turn-001","messageID":"m1","text":"known"}`,
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

	// Plant a symlink where the transcript belongs: os.Stat resolves it, the
	// mode check rejects the non-regular source (T-18-03).
	target := filepath.Join(t.TempDir(), "real.jsonl")

	if werr := os.WriteFile(target, []byte("{}\n"), 0o600); werr != nil {
		t.Fatalf("write symlink target: %v", werr)
	}

	store := filepath.Join(dir, ".ass-guard")

	if merr := os.MkdirAll(store, 0o750); merr != nil {
		t.Fatalf("mkdir store: %v", merr)
	}

	link := filepath.Join(store, "transcript_"+replayFixtureSID+".jsonl")

	if serr := os.Symlink(target, link); serr != nil {
		t.Fatalf("symlink fixture: %v", serr)
	}

	err = ReplayTranscript(rec, dir, replayFixtureSID)
	if !errors.Is(err, errReplayNotRegular) {
		t.Errorf("symlink replay error = %v; want errReplayNotRegular", err)
	}
}
