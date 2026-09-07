package session

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
)

var errNewmanagerRequiresA = errors.New("session: NewManager requires a non-nil Redactor")
var errManagerClosed = errors.New("session: manager closed")

// Redactor is the per-line redaction interface the Manager calls before writing
// each transcript line (LOG-03). internal/redact satisfies it.
type Redactor interface {
	Redact(line []byte) ([]byte, error)
	ScrubError(err error) string
}

// Manager is the SOLE owner of the transcript file (SESS-06). Every append goes
// through its mutex-guarded Append* API; concurrent appends never corrupt a line.
// Every Append* marshals the line, redacts the bytes via the injected Redactor
// (LOG-03), then writes under the mutex. The transcript is the ONE audit
// artifact per D-20 (audit log = transcript).
type Manager struct {
	f        *os.File
	path     string
	mu       sync.Mutex
	redactor Redactor
}

// NewManager opens (or creates) the per-session transcript under dir/.ass-guard/
// and returns the sole-owner Manager.
func NewManager(dir, sessionID string, red Redactor) (*Manager, error) {
	if red == nil {
		return nil, errNewmanagerRequiresA
	}

	f, path, err := openTranscript(dir, sessionID)
	if err != nil {
		return nil, err
	}

	return &Manager{f: f, path: path, redactor: red}, nil
}

// Path returns the transcript file path (for tests / inspection).
func (m *Manager) Path() string { return m.path }

// Close closes the underlying file. After Close, Append* errors.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.f == nil {
		return nil
	}

	err := m.f.Close()
	m.f = nil

	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	return nil
}

// appendLine marshals the line, redacts it, appends a newline, and writes it
// under the mutex. Redaction happens BEFORE the write (LOG-03).
func (m *Manager) appendLine(line *Line) error { //nolint:funcorder // ordering groups related logic
	raw, err := json.Marshal(line)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	red, err := m.redactor.Redact(raw)
	if err != nil {
		rawErr := errors.New(string(raw)) //nolint:err113 // dynamic error from raw input
		red = []byte(m.redactor.ScrubError(rawErr))
	}

	red = append(red, '\n')

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.f == nil {
		return errManagerClosed
	}

	_, err = m.f.Write(red)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	return nil
}

// appendLineUnredacted is the raw_thinking-ONLY append path (D-23, T-16-04):
// it marshals the line and writes it under the mutex WITHOUT invoking the
// Redactor — provider thinking bytes reach the transcript verbatim. It is a
// DELIBERATE near-copy of appendLine minus the redact block; do NOT extract
// a shared marshal/newline helper across the two paths — the distinct code
// path IS the guarantee (Pitfall 5's warning sign is any shared helper). The
// exemption is type-scoped to raw_thinking and must never widen.
func (m *Manager) appendLineUnredacted(line *Line) error { //nolint:funcorder // ordering groups related logic
	raw, err := json.Marshal(line)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	raw = append(raw, '\n')

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.f == nil {
		return errManagerClosed
	}

	_, err = m.f.Write(raw)
	if err != nil {
		return fmt.Errorf("call: %w", err)
	}

	return nil
}

// RFC 4122 v4 bit masks for uuidV4 (same in-repo construction as
// internal/acp newSessionID / internal/shaper uuidV4).
const (
	uuidVersionMask = 0x0F
	uuidVariantMask = 0x3F
	uuidVersionV4   = 0x40 // version nibble 0b0100
	uuidVariant10   = 0x80 // variant bits 0b10
)

// uuidV4 returns a fresh RFC 4122 v4 UUID string using crypto/rand. It
// panics on CSPRNG failure — ass-guard cannot run without a working entropy
// source. Replicated locally (the acp/shaper helpers are unexported, and
// session must not import the wire packages for an id shape).
func uuidV4() string {
	var b [16]byte

	_, err := rand.Read(b[:])
	if err != nil {
		panic("crypto/rand failed: " + err.Error())
	}

	b[6] = (b[6] & uuidVersionMask) | uuidVersionV4
	b[8] = (b[8] & uuidVariantMask) | uuidVariant10

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func now() time.Time { return time.Now().UTC() }

// AppendSessionStart records the session_start line.
func (m *Manager) AppendSessionStart(sessionID string) error {
	return m.appendLine(&Line{Type: TypeSessionStart, Timestamp: now(), Text: sessionID})
}

// AppendSessionEnd records the session_end line.
func (m *Manager) AppendSessionEnd() error {
	return m.appendLine(&Line{Type: TypeSessionEnd, Timestamp: now()})
}

// AppendUserMessage records a user message (the prompt content blocks).
func (m *Manager) AppendUserMessage(turnID string, content []ContentBlock) error {
	raw, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal user content: %w", err)
	}

	return m.appendLine(&Line{Type: TypeUserMessage, TurnID: turnID, Timestamp: now(), Content: raw})
}

// AppendAssistantMessage records the final assembled assistant text for a turn.
func (m *Manager) AppendAssistantMessage(turnID, text string) error {
	return m.appendLine(&Line{Type: TypeAssistantMessage, TurnID: turnID, Timestamp: now(), Text: text})
}

// AppendAgentMessageChunk records one streamed chunk (for reconstruction).
func (m *Manager) AppendAgentMessageChunk(turnID, messageID, text string) error {
	return m.appendLine(&Line{
		Type: TypeAgentMessageChunk, TurnID: turnID, Timestamp: now(),
		MessageID: messageID, Text: text,
	})
}

// AppendRequestShaped records the METADATA-ONLY shaped-request index line
// (09-05, AUD-03/D-01): correlation triple + shape fingerprint + the body
// store ref. The full redacted body lives in the capped audit body store —
// retrievable by meta.Ref. Supersedes v1.0 D-20's inline-verbatim line
// (documented at the Line struct; do not restore inline bodies).
func (m *Manager) AppendRequestShaped(turnID, profileName string, ts time.Time, meta audit.RequestMeta) error {
	return m.appendLine(&Line{
		Type: TypeRequestShaped, TurnID: turnID, Timestamp: ts,
		Profile: profileName, Ref: meta.Ref,
		SystemBlocks: meta.SystemBlocks, Tools: meta.Tools,
		Model: meta.Model, Bytes: meta.Bytes,
	})
}

// AppendToolCall records a model-selected tool invocation.
func (m *Manager) AppendToolCall(turnID, toolCallID, name string, input json.RawMessage) error {
	return m.appendLine(&Line{
		Type: TypeToolCall, TurnID: turnID, Timestamp: now(),
		ToolCallID: toolCallID, Name: name, Input: input,
	})
}

// AppendToolResult records a tool's result (stubbed in Phase 2; real in Phase 4).
func (m *Manager) AppendToolResult(turnID, toolCallID string, output json.RawMessage, isError bool) error {
	return m.appendLine(&Line{
		Type: TypeToolResult, TurnID: turnID, Timestamp: now(),
		ToolCallID: toolCallID, Output: output, IsError: isError,
	})
}

// AppendBoundary records a context-boundary (a mutating command completed,
// D-08). The line is the audit marker + the reset point for the NEXT turn's
// projection — never the producing turn's own mid-turn window (SESS-04,
// revised 2026-08-15; evidence: 08-09 / 08-08 T4 / corpus session 4440f5a7).
func (m *Manager) AppendBoundary(cause, commandRef, turnID string) error {
	return m.appendLine(&Line{
		Type: TypeBoundary, TurnID: turnID, Timestamp: now(),
		Cause: cause, CommandRef: commandRef,
	})
}

// AppendCommandProvenance records which discovered command file answered a
// slash-command invocation (08-04 / CMD-05): key is the command key
// ("opsx:explore"), path the on-disk source file, args the typed arguments.
// Written NEXT TO the expanded user message (which holds the body the model
// saw — D-02); provenance is metadata, never replayed as message content.
// turnID may be empty when written pre-turn (association is by append order).
func (m *Manager) AppendCommandProvenance(turnID, key, path, args string) error {
	return m.appendLine(&Line{
		Type: TypeCommandProvenance, TurnID: turnID, Timestamp: now(),
		Name: key, CommandRef: path, Text: args,
	})
}

// AppendMentionProvenance records what one @-mention in the prompt expanded
// to (21-04 / PAR-06, D-10's provenance duty): rawToken is the mention as
// typed ("@docs/readme.md", quotes included), resolvedPath the absolute
// object that answered it ("" when nothing resolved), form the expansion
// form (file|dir|denied|unresolved). Mirrors AppendCommandProvenance exactly
// — redacted append, metadata never replayed as message content, turnID may
// be empty when written pre-turn (association is by append order).
func (m *Manager) AppendMentionProvenance(turnID, rawToken, resolvedPath, form string) error {
	return m.appendLine(&Line{
		Type: TypeMentionProvenance, TurnID: turnID, Timestamp: now(),
		Name: rawToken, CommandRef: resolvedPath, Text: form,
	})
}

// AppendCanceled records that a turn was cancelled (D-16).
func (m *Manager) AppendCanceled(turnID string, ts time.Time, reason string) error {
	return m.appendLine(&Line{Type: TypeCanceled, TurnID: turnID, Timestamp: ts, Text: reason})
}

// AppendError records an investigate-and-fix-ready error line (PROJECT.md). The
// message is scrubbed (LOG-03); component + recoverable + stack are preserved.
func (m *Manager) AppendError(
	turnID, component, message string,
	inputs json.RawMessage, recoverable bool, stack string,
) error {
	callerErr := errors.New(message) //nolint:err113 // dynamic error from caller
	scrubbed := m.redactor.ScrubError(callerErr)

	return m.appendLine(&Line{
		Type: TypeError, TurnID: turnID, Timestamp: now(),
		Component: component, Message: scrubbed, Recoverable: recoverable,
		Stack: stack, Input: inputs,
	})
}

// AppendSubagentDispatch records a Task/Agent subagent dispatch.
func (m *Manager) AppendSubagentDispatch(
	parentTurnID, subagentTurnID, toolCallID string, restricted []string, resolvedModel string,
) error {
	return m.appendLine(&Line{
		Type: TypeSubagentDispatch, TurnID: subagentTurnID, Timestamp: now(),
		ParentTurnID: parentTurnID, SubagentTurnID: subagentTurnID,
		ToolCallID: toolCallID, RestrictedTools: restricted,
		ResolvedModel: resolvedModel,
	})
}

// AppendSubagentResult records a subagent's final result.
func (m *Manager) AppendSubagentResult(parentTurnID, subagentTurnID, result, errMsg string) error {
	return m.appendLine(&Line{
		Type: TypeSubagentResult, TurnID: subagentTurnID, Timestamp: now(),
		ParentTurnID: parentTurnID, SubagentTurnID: subagentTurnID,
		Result: result, Message: errMsg,
	})
}

// AppendUsage records a token-usage update.
func (m *Manager) AppendUsage(turnID string, input, output int64) error {
	return m.appendLine(&Line{
		Type: TypeUsage, TurnID: turnID, Timestamp: now(),
		InputTokens: input, OutputTokens: output,
	})
}

// AppendAskSuspended records an AskUserQuestion suspension (12-01, ACP-01):
// the turn ended at the ask; questions carries the surfaced payload (the
// model-authored {questions:[…]} input, redacted by the chokepoint like every
// other string value — T-12-01-02).
func (m *Manager) AppendAskSuspended(turnID, toolCallID string, questions json.RawMessage) error {
	return m.appendLine(&Line{
		Type: TypeAskSuspended, TurnID: turnID, Timestamp: now(),
		ToolCallID: toolCallID, Input: questions,
	})
}

// AppendPlanMode records a plan-mode transition (12-04, ACP-02): cause
// plan_mode_enter (the EnterPlanMode result) or plan_mode_exit (the approved
// ExitPlanMode resume). Schema-stable audit marker — NOT a boundary line; the
// Projector skips it (entering plan mode is not a mutating call and never
// resets a projection window).
func (m *Manager) AppendPlanMode(cause, toolCallID, turnID string) error {
	return m.appendLine(&Line{
		Type: TypePlanMode, TurnID: turnID, Timestamp: now(), Cause: cause,
		ToolCallID: toolCallID,
	})
}

// AppendEngineDecision records the unified engine's verdict for one turn
// (Phase-4 ENG-02 — the single provenance-tagged stream). The line type constant
// TypeEngineDecision is already reserved in transcript.go. action is the
// engine's decision vocabulary (nothing/continue/hook/ask/wait); signal is the
// matched signal ("text:<id>", "tool:<id>", or "unmatched"); reason is the
// investigate-and-fix-ready human note (PROJECT.md). The audit log therefore
// proves the structural-safety property: unmatched output triggers nothing.
func (m *Manager) AppendEngineDecision(
	turnID, action, signal, matchedSpan, configSource, reason string,
) error {
	return m.appendLine(&Line{
		Type:         TypeEngineDecision,
		TurnID:       turnID,
		Timestamp:    now(),
		Name:         action,
		Input:        json.RawMessage(`"` + signal + `"`),
		MatchedSpan:  matchedSpan,
		ConfigSource: configSource,
		Text:         reason,
	})
}

// AppendRawThinking records one raw provider thinking block (D-20/D-23):
// payload is the provider's own JSON bytes (json.RawMessage — never
// re-serialized) and model is the provider attribution (Phase 21 / PAR-05
// provenance). Routed through appendLineUnredacted — the Redactor NEVER sees
// thinking bytes (counting-fake test pins zero calls).
func (m *Manager) AppendRawThinking(turnID, model string, payload json.RawMessage) error {
	return m.appendLineUnredacted(&Line{
		Type: TypeRawThinking, TurnID: turnID, Timestamp: now(),
		Model: model, Content: payload,
	})
}

// AppendLocalCommand records a full local-command invocation record (D-22):
// the command key + the VERBATIM typed args (no shell re-quoting or
// normalization) + the resolution-source chain in resolution order
// (builtin → skills → agents → file per CMDS-01) + the expansion outcome.
// Metadata goes through the REDACTED path like every non-thinking kind
// (D-23's exemption is type-scoped to raw_thinking).
func (m *Manager) AppendLocalCommand(turnID, key, args, expansion string, sourceChain []string) error {
	return m.appendLine(&Line{
		Type: TypeLocalCommand, TurnID: turnID, Timestamp: now(),
		Name: key, Args: args, Expansion: expansion, SourceChain: sourceChain,
	})
}

// AppendCompaction records a compaction boundary marker (D-21 + the Phase-19
// summary payload, PAR-01): a fresh boundary id + the token-usage snapshot at
// the boundary (input/output/cache totals) + opaque pre/post transcript
// pointers (what survived / where the projected window resets) + the summary
// text post-marker projections seed from (D-06 durable seed). The summary is
// MODEL OUTPUT over tool content — it rides the REDACTED path like every
// non-thinking kind (D-23's exemption is raw_thinking-only; T-19-05/06).
func (m *Manager) AppendCompaction(turnID, preRef, postRef, summary string, input, output, cache int64) error {
	return m.appendLine(&Line{
		Type: TypeCompaction, TurnID: turnID, Timestamp: now(),
		BoundaryID: uuidV4(), PreRef: preRef, PostRef: postRef, Summary: summary,
		InputTokens: input, OutputTokens: output, CacheTokens: cache,
	})
}

// AppendSteeringDelivery records one boundary drain of queued steering inputs
// (23-01, SEEDG-01/D-03): text is the coalesced marker-wrapped user-role block
// (the model-visible render, verbatim) and count the number of inputs
// coalesced into it. Routed through the REDACTED path like every non-thinking
// kind — steering is model-visible untrusted user content (T-16-04's
// raw_thinking exemption never widens). The SOLE writer of steering_delivery
// lines (anti-spoofing, T-23-01): nothing parses model or tool output into
// this kind.
func (m *Manager) AppendSteeringDelivery(turnID, text string, count int) error {
	countJSON, err := json.Marshal(map[string]int{"count": count})
	if err != nil {
		return fmt.Errorf("marshal steering count: %w", err)
	}

	return m.appendLine(&Line{
		Type: TypeSteeringDelivery, TurnID: turnID, Timestamp: now(),
		Text: text, Input: countJSON,
	})
}

// AppendSynthetic appends one reconciliation closure line (18-02, D-02): the
// 18-05 load path calls it for every closure Reconcile returned, with the
// line's Cause already set to InterruptedCause. It routes through the SAME
// marshal→redact→mutex→single-write path as every other append — synthetic
// content crosses the Redactor exactly like live content (16-D-23's exemption
// is type-scoped to raw_thinking), and there is deliberately no second write
// path (append-only, never a rewrite).
func (m *Manager) AppendSynthetic(line *Line) error {
	return m.appendLine(line)
}

// ReadAll reads every line from the transcript in append order.
func (m *Manager) ReadAll() ([]Line, error) {
	m.mu.Lock()
	fname := m.path
	m.mu.Unlock()

	return readTranscriptFile(fname)
}

// ReadLastBoundary returns the most recent boundary line, or (nil, nil) if none.
func (m *Manager) ReadLastBoundary() (*Line, error) {
	lines, err := m.ReadAll()
	if err != nil {
		return nil, err
	}

	for i := len(lines) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
		if lines[i].Type == TypeBoundary {
			return &lines[i], nil
		}
	}

	return nil, nil //nolint:nilnil // nil result signals "no boundary line found"; caller checks for nil
}

// ReadSince returns the lines appended AFTER the last line carrying the given
// turnID. If turnID is empty or not found, returns all lines.
func (m *Manager) ReadSince(turnID string) ([]Line, error) {
	lines, err := m.ReadAll()
	if err != nil {
		return nil, err
	}

	if turnID == "" {
		return lines, nil
	}

	lastIdx := -1

	for i := range lines {
		if lines[i].TurnID == turnID {
			lastIdx = i
		}
	}

	if lastIdx < 0 {
		return lines, nil
	}

	return lines[lastIdx+1:], nil
}

// readTranscriptFile parses every JSONL line in path into a Line slice.
func readTranscriptFile(path string) ([]Line, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("call: %w", err)
	}

	defer func() { _ = f.Close() }()

	var out []Line

	br := bufio.NewReader(f)

	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) == 0 && rerr != nil {
			break
		}

		for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
			line = line[:len(line)-1]
		}

		if len(line) == 0 {
			if rerr != nil {
				break
			}

			continue
		}

		var l Line

		jerr := json.Unmarshal(line, &l)
		if jerr != nil {
			continue
		}

		out = append(out, l)

		if rerr != nil {
			break
		}
	}

	return out, nil
}
