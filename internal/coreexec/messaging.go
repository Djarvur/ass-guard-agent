package coreexec

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"
)

// Agent messaging + session-context reads (12-04, ACP-03).
//
// SendMessage delivers to the v1.0 subagent dispatch id space (agent_<uuid> —
// the CAPTURED Agent tool result carries the address verbatim: 'agentId:
// agent_<uuid> (use SendMessage with to: …)'). The mailbox is per-session
// (T-12-04-02: no cross-session delivery); a LIVE agent's deliver fn runs at
// dispatch-surface observation; a COMPLETED agent's delivery is recorded into
// the per-session inbox — the ack is honest about queued-vs-live (the 12-05
// capture hunt never observed the ack on the wire, so its text is the
// source-informed corpus-absent default).
//
// ReadSessionContext reads ass-guard's OWN .ass-guard/ transcript family
// (transcript_<sessID>.jsonl — T-12-04-03: the target's private store is not
// ours to serve). Deterministic excerpting only — no LLM call inside the
// executor; the turn's model does the understanding.

// AgentMessage is one delivered message (byte-faithful to the model's input).
type AgentMessage struct {
	ID      string    `json:"id"`
	Summary string    `json:"summary"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

type mailboxAgent struct {
	deliver func(AgentMessage) error // live-delivery fn (nil = record-only)
	inbox   []AgentMessage
}

// AgentMailbox is the per-session agent inbox registry.
type AgentMailbox struct {
	mu      sync.Mutex
	agents  map[string]*mailboxAgent
	nextMsg int
}

// NewAgentMailbox returns an empty per-session mailbox.
func NewAgentMailbox() *AgentMailbox {
	return &AgentMailbox{agents: map[string]*mailboxAgent{}}
}

// Register makes an agent id deliverable: deliver runs for LIVE agents (the
// dispatch surface's observation point); nil deliver records into the inbox
// only (the honest completed-agent path — the ack says queued).
func (m *AgentMailbox) Register(agentID string, deliver func(AgentMessage) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.agents[agentID] = &mailboxAgent{deliver: deliver}
}

// Lookup returns the agent's recorded inbox (delivered messages).
func (m *AgentMailbox) Lookup(agentID string) ([]AgentMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.agents[agentID]
	if !ok {
		return nil, false
	}

	return append([]AgentMessage(nil), a.inbox...), true
}

// Deliver records + forwards one message; unknown ids fail (never broadcast).
func (m *AgentMailbox) Deliver(agentID string, msg AgentMessage) error {
	m.mu.Lock()

	a, ok := m.agents[agentID]
	if !ok {
		m.mu.Unlock()

		return &unknownAgentError{agentID: agentID}
	}

	m.nextMsg++

	msg.ID = fmt.Sprintf("msg_%d", m.nextMsg)

	if msg.At.IsZero() {
		msg.At = time.Now().UTC()
	}

	a.inbox = append(a.inbox, msg)

	deliver := a.deliver

	m.mu.Unlock()

	if deliver != nil {
		return deliver(msg)
	}

	return nil
}

// sendMsgArgs is the captured input shape (schema bounds enforced at parse).
type sendMsgArgs struct {
	To      string `json:"to"`
	Summary string `json:"summary"`
	Message string `json:"message"`
}

// SendMessageExecute returns the SendMessage catalog Stub over the mailbox:
// validated {to, summary, message} delivered byte-faithful; the ack names the
// message id + agent (corpus_absent source-informed default); unknown ids
// return the structured error with the id echoed.
func SendMessageExecute(mb *AgentMailbox) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if mb == nil {
			return marshalStructured("sendmessage: no mailbox configured", errNilMailbox)
		}

		var a sendMsgArgs

		perr := json.Unmarshal(args, &a)
		if perr != nil || a.To == "" {
			return marshalStructured("sendmessage: invalid input (to, summary, message required)", errBadSendInput)
		}

		if len(a.Summary) > 200 || len(a.Message) > 20000 {
			return marshalStructured(
				"sendmessage: summary (<=200) or message (<=20000) over the schema bound", errBadSendInput)
		}

		msg := AgentMessage{Summary: a.Summary, Message: a.Message}

		derr := mb.Deliver(a.To, msg)
		if derr != nil {
			return marshalStructured(derr.Error(), derr)
		}

		if inbox, ok := mb.Lookup(a.To); ok && len(inbox) > 0 {
			msg = inbox[len(inbox)-1] // the assigned id + timestamp
		}

		ack := fmt.Sprintf("Message %s was queued for local agent %s.", msg.ID, a.To)

		return json.Marshal(ack)
	}
}

var (
	errNilMailbox   = errors.New("coreexec: sendmessage: nil mailbox")
	errBadSendInput = errors.New("coreexec: sendmessage: invalid input")
)

// unknownAgentError names the recipient (the structured error echoes the id —
// T-12-04-02: unknown ids are errors, never best-effort broadcast).
type unknownAgentError struct{ agentID string }

func (e *unknownAgentError) Error() string { return "sendmessage: unknown agent " + e.agentID }

// --- ReadSessionContext -------------------------------------------------------

// sessIDPattern validates session ids in BOTH live vocabularies (12-11,
// G-12-4c): the captured zcode schema's sess_* branch (kept
// character-for-character — the schema is the provenance for that shape) OR
// ass-guard's own id branch — the RFC 4122 v4 UUID form internal/acp/handlers.go
// newSessionID mints and internal/session/transcript.go openTranscript writes as
// transcript_<uuid>.jsonl (T-12-04-03: the reader serves ass-guard's OWN store).
//
// Traversal-safe by construction (T-12-11-01): both alternatives exclude path
// separators, so the filepath.Join under .ass-guard/ in read() cannot escape
// the store directory — a hostile id rejects structurally BEFORE any file open.
var sessIDPattern = regexp.MustCompile(
	`^(sess_[A-Za-z0-9._-]+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// SessionReader enumerates + reads the project's .ass-guard/ transcript family.
type SessionReader struct {
	dir string
}

// NewSessionReader returns the reader over workDir's .ass-guard/ family.
func NewSessionReader(workDir string) *SessionReader {
	return &SessionReader{dir: workDir}
}

// charsPerToken is the maxTokens accounting convention (chars/4 — the plan's
// documented bound); sessionCtxMaxTokens is the schema's declared maximum.
const (
	charsPerToken       = 4
	sessionCtxMaxTokens = 12000
)

// Reader buffer bounds (one 16MB transcript line worst case).
const (
	readerBufInit = 64 * 1024
	readerBufMax  = 16 * 1024 * 1024
)

// transcriptLine is the reader's view of one transcript line (the fields the
// excerpt strategies consume).
type transcriptLine struct {
	Type    string     `json:"type"`
	TurnID  string     `json:"turnID"` //nolint:tagliatelle // on-disk format
	Text    string     `json:"text"`
	Content []struct { // user_message content blocks
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// textOf extracts the human-readable text of one line.
func (l transcriptLine) textOf() string {
	if l.Text != "" {
		return l.Text
	}

	var b strings.Builder

	for _, c := range l.Content {
		if c.Text != "" {
			b.WriteString(c.Text)
		}
	}

	return b.String()
}

// Sessions enumerates the recorded session ids.
func (r *SessionReader) Sessions() []string {
	entries, err := os.ReadDir(filepath.Join(r.dir, ".ass-guard"))
	if err != nil {
		return nil
	}

	var ids []string

	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "transcript_") && strings.HasSuffix(name, ".jsonl") {
			ids = append(ids, strings.TrimSuffix(strings.TrimPrefix(name, "transcript_"), ".jsonl"))
		}
	}

	return ids
}

// read parses one session's transcript lines.
func (r *SessionReader) read(sessionID string) ([]transcriptLine, error) {
	if !sessIDPattern.MatchString(sessionID) {
		return nil, &invalidSessionIDError{sessionID: sessionID}
	}

	f, err := os.Open(filepath.Join(r.dir, ".ass-guard", "transcript_"+sessionID+".jsonl"))
	if err != nil {
		return nil, fmt.Errorf("readsessioncontext: session %q not found: %w", sessionID, err)
	}
	defer func() { _ = f.Close() }()

	var lines []transcriptLine

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, readerBufInit), readerBufMax)

	for sc.Scan() {
		var l transcriptLine

		jerr := json.Unmarshal(sc.Bytes(), &l)
		if jerr != nil {
			continue // skip non-conforming lines (forward-compat)
		}

		lines = append(lines, l)
	}

	serr := sc.Err()
	if serr != nil {
		return nil, fmt.Errorf("readsessioncontext: scan transcript: %w", serr)
	}

	return lines, nil
}

// readSessionArgs is the captured input shape.
type readSessionArgs struct {
	SessionID string  `json:"sessionId"` //nolint:tagliatelle // captured input key
	Query     string  `json:"query"`
	Strategy  string  `json:"strategy"`
	MaxTokens float64 `json:"maxTokens"` //nolint:tagliatelle // captured input key
}

// ReadSessionContextExecute returns the ReadSessionContext catalog Stub:
// deterministic query-filtered excerpting over the persisted transcripts with
// the captured lite-context head; maxTokens bounds by chars/4 with the
// documented truncation tail (no LLM inside the executor).
func ReadSessionContextExecute(r *SessionReader) toolcat.Stub {
	return func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
		if r == nil {
			return marshalStructured("readsessioncontext: no session reader configured", errNilReader)
		}

		var a readSessionArgs

		perr := json.Unmarshal(args, &a)
		if perr != nil || a.SessionID == "" || a.Query == "" {
			return marshalStructured("readsessioncontext: invalid input (sessionId + query required)", errBadReadInput)
		}

		lines, err := r.read(a.SessionID)
		if err != nil {
			return marshalStructured(err.Error(), err)
		}

		var body string

		if a.Strategy == "handoff" {
			body = handoffExcerpt(lines)
		} else {
			body = relevantExcerpt(lines, a.Query)
		}

		text := "ReadSessionContext returned lite context for " + a.SessionID + ".\n" + body

		// maxTokens: chars/4 accounting (the plan's documented convention),
		// clamped to the schema max 12000; the tail names the bound.
		budget := sessionCtxMaxTokens
		if a.MaxTokens > 0 {
			budget = min(int(a.MaxTokens), sessionCtxMaxTokens)
		}

		if budgetChars := budget * charsPerToken; len(text) > budgetChars {
			cut := budgetChars
			if i := strings.LastIndex(text[:cut], "\n"); i > budgetChars/2 {
				cut = i
			}

			text = text[:cut] + "\n[truncated at maxTokens=" + strconv.Itoa(budget) + "]"
		}

		return json.Marshal(text)
	}
}

// relevantExcerpt returns the query-matching user/assistant lines with their
// turn context (deterministic: first-match order, one entry per turn).
func relevantExcerpt(lines []transcriptLine, query string) string {
	terms := strings.Fields(strings.ToLower(query))

	var b strings.Builder

	seen := map[string]bool{}

	for _, l := range lines {
		if l.Type != "user_message" && l.Type != "assistant_message" {
			continue
		}

		text := l.textOf()
		lower := strings.ToLower(text)

		matched := false

		for _, t := range terms {
			if strings.Contains(lower, t) {
				matched = true

				break
			}
		}

		if !matched || seen[l.TurnID+"\x00"+l.Type] {
			continue
		}

		seen[l.TurnID+"\x00"+l.Type] = true
		fmt.Fprintf(&b, "[%s] %s\n", l.TurnID, text)
	}

	return strings.TrimRight(b.String(), "\n")
}

// handoffTailLines is the handoff strategy's tail bound (final turns).
const handoffTailLines = 3

// handoffExcerpt returns the bounded TAIL of the session (the final turns'
// user/assistant text — the continuation summary shape).
func handoffExcerpt(lines []transcriptLine) string {
	var convo []transcriptLine

	for _, l := range lines {
		if l.Type == "user_message" || l.Type == "assistant_message" {
			convo = append(convo, l)
		}
	}

	start := max(len(convo)-handoffTailLines*2, 0)

	var b strings.Builder

	for _, l := range convo[start:] {
		fmt.Fprintf(&b, "[%s] %s\n", l.TurnID, l.textOf())
	}

	return strings.TrimRight(b.String(), "\n")
}

var (
	errNilReader    = errors.New("coreexec: readsessioncontext: nil reader")
	errBadReadInput = errors.New("coreexec: readsessioncontext: invalid input")
)

// invalidSessionIDError names the rejected id (schema-pattern provenance).
type invalidSessionIDError struct{ sessionID string }

func (e *invalidSessionIDError) Error() string {
	return "readsessioncontext: invalid session id " + e.sessionID
}
