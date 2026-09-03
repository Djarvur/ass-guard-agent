package acp

// Replay: the 18-01 session/load transcript→frames mapping (ACP-06).
//
// ReplayTranscript streams a past session's transcript through THE same
// ordered emitter surface live turns use (16-D-02: the per-session foreground
// EmitterHandle — one drain, one total order), so a restored conversation is
// pixel-identical in shape to a live one. The reader is transcript-tolerant
// (16-D-20): non-conforming lines, torn trailing bytes, and unknown FUTURE
// kinds are skipped without failing the replay.
//
// Mapping table (the agent-visible vocabulary — every other kind emits NO
// frame; state/bookkeeping lines seed agent-side state and title derivation,
// and the v1 session/update vocabulary has no user-message slot — the client
// owns rendering of its own prompts):
//
//	transcript kind          → session/update frame
//	---------------------------------------------------------------------
//	agent_message_chunk      → agent_message_chunk (text chunk; messageId
//	                           from the line, falling back to turnID)
//	assistant_message        → agent_message_chunk (the final chunk of the
//	                           message, in transcript order)
//	tool_call                → tool_call card (toolCallId + title from the
//	                           tool name; the raw input rides the frame for
//	                           the acp layer's presentation rules — the
//	                           TodoWrite→plan rule applies on replay exactly
//	                           as live; kind/locations omitted on replay)
//	tool_result              → terminal tool_call_update keyed by toolCallID
//	                           (status completed|failed from isError;
//	                           locations/diff omitted on replay)
//	error                    → a toolCallID-carrying line closes that call as
//	                           a terminal tool_call_update (status failed);
//	                           a bare line renders as an agent_message_chunk
//	                           carrying the error text ("component: message")
//	session_start/end, user_message, boundary, usage, request_shaped,
//	command_provenance, subagent_dispatch/result, engine_decision,
//	ask_suspended, plan_mode, raw_thinking, local_command, compaction,
//	canceled                 → NO frame (bookkeeping/audit — canceled turns
//	                           emit nothing beyond what their tool lines
//	                           already emitted)
//
// Transparency (ACP-03 prohibition, extended to replay): every emitted frame
// corresponds to an EXISTING transcript line — no fabricated activity, no
// synthetic progress cards, no cached stale frames presented as history.
//
// The file deliberately does NOT import internal/session (the coreexec
// transcriptLine precedent — 25-D-13 keeps the frontend decoupled from the
// session kit seam): replayLine copies the Line envelope's field spellings.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// loadSessIDPattern validates client-supplied session ids on the session/load
// path (T-18-01): copied VERBATIM from internal/coreexec messaging.go's
// sessIDPattern (12-11/G-12-4c provenance: the captured zcode schema's sess_*
// branch OR ass-guard's own RFC 4122 v4 UUID branch that newSessionID mints
// and transcript_<uuid>.jsonl names). Traversal-safe by construction: both
// alternatives exclude path separators, so the filepath.Join under
// .ass-guard/ cannot escape the store directory — a hostile id rejects
// structurally BEFORE any file open. A package-local copy (not an import)
// because internal/acp stays free of internal/coreexec (wire-layer dependency
// direction).
var loadSessIDPattern = regexp.MustCompile(
	`^(sess_[A-Za-z0-9._-]+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// Reader buffer bounds (the messaging.go scanner-buffer pattern): 64 KiB
// initial, one 16 MiB transcript line worst case — a bound, never a license.
const (
	replayBufInit = 64 * 1024
	replayBufMax  = 16 * 1024 * 1024
)

// Static replay errors (wrapped with session context at the raise sites —
// err113 discipline).
var (
	errReplayMalformedID = errors.New("replay: malformed sessionId")
	errReplayNotRegular  = errors.New("replay: not a regular transcript (symlink planted?)")
)

// Transcript kind discriminators the mapping consumes (copied spellings from
// internal/session/transcript.go's Type* constants — same no-import
// discipline as replayLine; the emitter's updKind* vocabulary is a DIFFERENT
// vocabulary that merely shares string values).
const (
	replayKindAgentMessageChunk = "agent_message_chunk"
	replayKindAssistantMessage  = "assistant_message"
	replayKindToolCall          = "tool_call"
	replayKindToolResult        = "tool_result"
	replayKindError             = "error"
)

// replayLine is the reader's view of one transcript line — the fields the
// mapping consumes, spellings copied from the session.Line envelope (tag
// names are the ON-DISK format, not the wire).
type replayLine struct {
	Type   string `json:"type"`
	TurnID string `json:"turnID,omitempty"` //nolint:tagliatelle // on-disk format

	// agent_message_chunk / assistant_message
	Text      string `json:"text,omitempty"`
	MessageID string `json:"messageID,omitempty"` //nolint:tagliatelle // on-disk format

	// tool_call / tool_result
	ToolCallID string          `json:"toolCallID,omitempty"` //nolint:tagliatelle // on-disk format
	Name       string          `json:"name,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	IsError    bool            `json:"isError,omitempty"` //nolint:tagliatelle // on-disk format

	// error (Task 2: the agent-visible error-line row)
	Component string `json:"component,omitempty"`
	Message   string `json:"message,omitempty"`
}

// errorText composes the replayed error line's text: "component: message",
// degrading to whichever field is present.
func (l *replayLine) errorText() string {
	switch {
	case l.Component != "" && l.Message != "":
		return l.Component + ": " + l.Message
	case l.Message != "":
		return l.Message
	default:
		return l.Component
	}
}

// ReplayTranscript streams dir/.ass-guard/transcript_<sessionID>.jsonl as
// ordered session/update frames through emit — the per-session emitter handle
// the caller built from THE ordered TurnEmitter (16-D-02; the load handler
// passes s.Emitter(sessionID)). A plain ChunkEmitter (legacy fakes) receives
// the text chunks and silently skips the tool frames — the same degrade the
// runtime forwarder applies (routeBusEvent's nil-ActivityEmitter branch).
//
// Tolerance (16-D-20): non-conforming lines and unknown kinds are skipped
// without failing the replay. Errors surface only for an unreadable source
// (missing/non-regular file) or a failed enqueue (emitter stopped — serve
// teardown): a half-replayed session must not register as loaded.
func ReplayTranscript(emit ChunkEmitter, dir, sessionID string) error {
	if !loadSessIDPattern.MatchString(sessionID) {
		// Defense-in-depth: the handler validates first (T-18-01 — reject
		// BEFORE any file open); an exported entry point re-checks so no
		// future caller can join a hostile id under the store.
		return fmt.Errorf("%w %q", errReplayMalformedID, sessionID)
	}

	path := filepath.Join(dir, ".ass-guard", "transcript_"+sessionID+".jsonl")

	// T-18-03, two-layer source guard: (1) os.Lstat sees the symlink bit
	// ITSELF — the store never legitimately contains links, so ANY link at
	// the transcript path rejects (a link to a regular file would otherwise
	// resolve cleanly through Stat); (2) the resolved os.Stat mode check
	// rejects the non-link non-regular shapes (directory, device, fifo).
	// The O_RDONLY open below therefore never follows a planted link.
	linfo, lerr := os.Lstat(path)
	if lerr == nil && linfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("replay: session %q: %w", sessionID, errReplayNotRegular)
	}

	info, serr := os.Stat(path)
	if serr != nil {
		return fmt.Errorf("replay: session %q not found: %w", sessionID, serr)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("replay: session %q: %w", sessionID, errReplayNotRegular)
	}

	f, oerr := os.Open(path) // O_RDONLY — replay never writes
	if oerr != nil {
		return fmt.Errorf("replay: open transcript %q: %w", sessionID, oerr)
	}

	defer func() { _ = f.Close() }()

	// The ActivityEmitter assertion happens once (the runtime forwarder
	// precedent); a plain ChunkEmitter keeps the text chunks and skips cards.
	toolEmit, _ := emit.(ActivityEmitter)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, replayBufInit), replayBufMax)

	for sc.Scan() {
		var l replayLine

		jerr := json.Unmarshal(sc.Bytes(), &l)
		if jerr != nil {
			continue // non-conforming / torn line — skip (forward-compat)
		}

		ferr := replayLineFrame(&l, emit, toolEmit)
		if ferr != nil {
			return ferr
		}
	}

	serr = sc.Err()
	if serr != nil {
		return fmt.Errorf("replay: scan transcript %q: %w", sessionID, serr)
	}

	return nil
}

// replayLineFrame maps ONE parsed transcript line to its frame(s) per the
// file-header table — strictly one agent-visible frame per line, in transcript
// order (the caller's scan order IS the emission order).
func replayLineFrame(l *replayLine, emit ChunkEmitter, toolEmit ActivityEmitter) error {
	switch l.Type {
	case replayKindAgentMessageChunk:
		err := emit.AgentMessageChunk(replayMessageID(l), l.Text)
		if err != nil {
			return fmt.Errorf("replay chunk: %w", err)
		}

		return nil

	case replayKindAssistantMessage:
		// The final chunk of the turn's message (the assembled text the live
		// turn streamed as chunks + closed with this line).
		err := emit.AgentMessageChunk(replayMessageID(l), l.Text)
		if err != nil {
			return fmt.Errorf("replay final chunk: %w", err)
		}

		return nil

	case replayKindToolCall:
		if toolEmit == nil {
			return nil // plain-ChunkEmitter degrade (forwarder precedent)
		}

		err := toolEmit.ToolCall(&ToolCallFrame{
			ToolCallID: l.ToolCallID,
			Title:      l.Name,
			Input:      append(json.RawMessage(nil), l.Input...),
		})
		if err != nil {
			return fmt.Errorf("replay tool call: %w", err)
		}

		return nil

	case replayKindToolResult:
		if toolEmit == nil {
			return nil
		}

		status := StatusCompleted
		if l.IsError {
			status = StatusFailed
		}

		err := toolEmit.ToolCallUpdate(&ToolCallUpdateFrame{
			ToolCallID: l.ToolCallID,
			Status:     status,
		})
		if err != nil {
			return fmt.Errorf("replay tool update: %w", err)
		}

		return nil

	case replayKindError:
		return replayErrorFrame(l, emit, toolEmit)

	default:
		// Bookkeeping/audit kinds and unknown FUTURE kinds (16-D-20): no
		// frame, no failure.
		return nil
	}
}

// replayErrorFrame maps ONE error line (Task 2, 18-01): a toolCallID-carrying
// error closes THAT call as a terminal failed tool_call_update (the client can
// pair it with the card); a bare error renders as an agent_message_chunk
// carrying the error text so the restored conversation shows what went wrong.
func replayErrorFrame(l *replayLine, emit ChunkEmitter, toolEmit ActivityEmitter) error {
	if l.ToolCallID != "" {
		if toolEmit == nil {
			return nil // plain-ChunkEmitter degrade (forwarder precedent)
		}

		uerr := toolEmit.ToolCallUpdate(&ToolCallUpdateFrame{
			ToolCallID: l.ToolCallID,
			Status:     StatusFailed,
		})
		if uerr != nil {
			return fmt.Errorf("replay error update: %w", uerr)
		}

		return nil
	}

	eerr := emit.AgentMessageChunk(replayMessageID(l), l.errorText())
	if eerr != nil {
		return fmt.Errorf("replay error chunk: %w", eerr)
	}

	return nil
}

// replayMessageID derives a chunk's messageId: the line's own messageID (live
// chunks carry it), falling back to the turnID (the assistant_message line
// carries none — live turns mint messageID from the turn).
func replayMessageID(l *replayLine) string {
	if l.MessageID != "" {
		return l.MessageID
	}

	return l.TurnID
}
