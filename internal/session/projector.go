package session

import (
	"encoding/json"
	"strings"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// D-02 mechanical-extraction truncation limits (RESEARCH §4.3). The projector
// builds the task summary by reading the transcript — NO model call.
const (
	MaxSummaryUserChars      = 500 // last user message excerpt
	MaxSummaryAssistantChars = 300 // last assistant message excerpt
	MaxFilesTouched          = 20  // deduped file paths from tool calls
)

// fileBearingTools is the set of tools whose inputs carry a file_path/path we
// extract for the task summary. Other tools' inputs are not file-bearing.
var fileBearingTools = map[string]bool{ //nolint:gochecknoglobals // immutable table
	toolRead: true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, toolBash: true,
}

// MidTurnWindowMessages bounds the within-turn accumulated window: the MOST
// RECENT tail of the current turn's exchanges, dropping only COMPLETE exchange
// groups (an assistant batch is never separated from its tool results —
// pair-safety). Pinned to the captured zcode tail window: rollout records of
// kind "tail" carry the last 64 messages (messageOffset/messageCount
// bookkeeping, 08-07 capture).
const MidTurnWindowMessages = 64

// Projector builds the lean model-visible window (D-01) by mechanical extraction
// from the transcript (D-02 — NO model call). It is the sole producer of the
// []provider.Message the Shaper consumes. The window resets BETWEEN turns
// (SESS-04, revised 08-09): a boundary line resets the projections of turns
// that START after it — never the producing turn's own mid-turn window. The
// lean seed = system (added by the Shaper from the profile) + task summary +
// current user message, with ZERO carry-forward of prior turn messages as
// separate messages. WITHIN a turn (08-07's two-layer model, re-scoped 08-09),
// the window ACCUMULATES to the turn's end: the current turn's assistant
// tool_use batches and tool results follow the lean seed — including exchanges
// past mid-turn boundary lines — so the model sees its own exchanges on every
// tool-loop iteration; without this, real agentic turns cannot converge (the
// 08-08 T4 finding: both gated E2E turns exhausted maxIterations because every
// mutating result was followed by a boundary that wiped the window).
//
// Capture grounding (session 4440f5a7, verified 2026-08-15 — 08-09 / 08-08 T4):
// the zcode corpus carries a ROLLING last-64-message window per request
// (46/46 `tail` records, messageOffset advancing as messages append), within-turn
// tool results PERSIST into later requests of the same turn (579 same-turn
// toolCallId persistences), and there are ZERO resets on tool results — the
// per-mutating-call mid-turn reset was itself a request-shape divergence from
// the mimicry target.
type Projector struct {
	prof    *profile.Profile
	manager *Manager
}

// NewProjector returns a Projector over the given profile + transcript Manager.
func NewProjector(prof *profile.Profile, m *Manager) *Projector {
	return &Projector{prof: prof, manager: m}
}

// Project builds the window the model sees for the given turn. The lean seed
// is ONE user message (the task summary + the current intent); prior turns are
// NOT carried as messages (D-01 zero carry-forward). The reset point is the
// last boundary recorded BEFORE the projected turn's user message (SESS-04,
// revised 08-09 — between turns): mid-turn boundary lines of the CURRENT turn
// never move the reset point, so the seed stays stable across the turn's
// iterations, and the summary scope when no reset boundary exists is the
// PRE-turn lines (not all lines — the turn's own exchanges never churn the
// summary). Within the turn, the current turn's exchanges accumulate after the
// seed (08-07, re-scoped 08-09): consecutive tool_call lines fold into ONE
// assistant message with a ToolCalls batch, each tool_result becomes a
// tool-role message (name resolved from its paired tool_call), and the turn's
// assistant text lines render as plain assistant messages — mechanically
// extracted from the transcript, bounded to the MidTurnWindowMessages tail,
// pair-safe.
func (p *Projector) Project(turnID string) ([]provider.Message, error) {
	lines, err := p.manager.ReadAll()
	if err != nil {
		return nil, err
	}

	beforeBoundary, afterBoundary := splitAtResetBoundary(lines, turnID)

	summary := p.extractSummary(beforeBoundary)
	currentIntent := p.findCurrentIntent(afterBoundary, beforeBoundary, turnID)

	// The lean seed is ONE user message: the task summary + the current intent.
	// (The Shaper adds the system prompt separately from p.prof.System.)
	var content string
	if summary != "" {
		content = "Task summary (mechanical, post-boundary):\n" + summary +
			"\n\n--- Current request ---\n" + currentIntent
	} else {
		// First turn: no summary, just the intent.
		content = currentIntent
	}

	mid := boundMidTurn(accumulateMidTurn(lines, turnID))

	out := make([]provider.Message, 0, 1+len(mid))
	out = append(out, provider.Message{Role: roleUserMsg, Content: content})
	out = append(out, mid...)

	return out, nil
}

// splitAtResetBoundary splits the transcript at the between-turn reset point
// (SESS-04, revised 08-09). The reset boundary is the LAST TypeBoundary
// recorded STRICTLY BEFORE the projected turn's user message — a boundary
// resets projections of turns that START after it, never the producing turn's
// own mid-turn window. The turn's user message is the last one carrying
// TurnID == turnID (fallback: the last user_message overall). With no reset
// boundary the summary scope is the PRE-turn lines (the current turn's
// accumulating exchanges never churn the seed); with no user message at all,
// the legacy all-lines shape applies.
//
//nolint:nonamedreturns // gocritic unnamedResult prefers names
func splitAtResetBoundary(lines []Line, turnID string) (before, after []Line) {
	lastUserIdx, matchedUserIdx := -1, -1

	for i := range lines {
		if lines[i].Type == TypeUserMessage {
			lastUserIdx = i

			if lines[i].TurnID == turnID {
				matchedUserIdx = i
			}
		}
	}

	turnUserIdx := matchedUserIdx
	if turnUserIdx < 0 {
		turnUserIdx = lastUserIdx
	}

	boundaryIdx := -1

	for i := range lines {
		if lines[i].Type == TypeBoundary && (turnUserIdx < 0 || i < turnUserIdx) {
			boundaryIdx = i
		}
	}

	switch {
	case boundaryIdx >= 0:
		return lines[:boundaryIdx], lines[boundaryIdx+1:]
	case turnUserIdx >= 0:
		return lines[:turnUserIdx], lines[turnUserIdx:]
	default:
		return lines, nil
	}
}

// accumulateMidTurn folds the current turn's post-user-message lines into the
// mid-turn message sequence (08-09: the anchor is SOLELY the turn's user_message
// — the seed already carries it — so mid-turn boundary lines never wipe the
// accumulation; the capture never resets on tool results): consecutive
// TypeToolCall lines become ONE assistant message with a ToolCalls batch (the
// capture's batch form), each TypeToolResult becomes a tool-role message whose
// ToolName is resolved from the paired tool_call line, and TypeAssistantMessage
// becomes a plain assistant text message. A tool_result whose call is not in
// the window (its batch was never accumulated) is dropped — an orphaned
// tool_result would break the provider's tool_use/tool_result pairing
// invariant.
func accumulateMidTurn(lines []Line, turnID string) []provider.Message {
	// The anchor is the current turn's user_message ONLY (between-turn reset,
	// SESS-04 revised 08-09): boundary lines are audit markers + the NEXT
	// turn's reset point, never the producing turn's wipe.
	anchor := 0

	for i := range lines {
		if lines[i].Type == TypeUserMessage && lines[i].TurnID == turnID {
			anchor = i + 1
		}
	}

	var (
		out     []provider.Message
		pending []provider.ToolCall
		names   = map[string]string{} // callID -> tool name (pairing resolution)
	)

	flushBatch := func() {
		if len(pending) > 0 {
			out = append(out, provider.Message{Role: roleAssistant, ToolCalls: pending})
			pending = nil
		}
	}

	for i := anchor; i < len(lines); i++ {
		l := &lines[i]
		if l.TurnID != turnID {
			continue // only the CURRENT turn's lines fold (subagent turns have their own)
		}

		switch l.Type {
		case TypeToolCall:
			pending = append(pending, provider.ToolCall{ID: l.ToolCallID, Name: l.Name, Input: l.Input})
			names[l.ToolCallID] = l.Name
		case TypeToolResult:
			flushBatch() // the batch is closed once its results start arriving

			name, paired := names[l.ToolCallID]
			if !paired {
				continue // orphaned result — its batch is outside the window
			}

			out = append(out, provider.Message{
				Role: roleToolMsg, ToolCallID: l.ToolCallID, ToolName: name,
				Content: plainContent(l.Output), IsError: l.IsError,
			})
		case TypeAssistantMessage:
			flushBatch()

			out = append(out, provider.Message{Role: roleAssistant, Content: l.Text})
		}
	}

	flushBatch()

	return out
}

// plainContent renders a tool-result Output as the model-visible Content
// (08-08 T1, the rendering seam): executors return json.RawMessage per the
// Stub contract, so a captured PLAIN-TEXT result (zcode renders Bash output,
// `(Bash completed with no output)`, `Exit code <N>` errors, Read's
// line-numbered text, and the Write/Edit success texts as plain text on the
// wire-normalized tool message — see internal/coreexec/testdata/
// zcode-core-results.json) is marshaled as a JSON STRING; decoding it here is
// what makes the unquoted form reach the shaper. JSON OBJECTS pass through
// verbatim (the shipped openspec {stdout,stderr,exit_code,classification},
// Skill {"content":…}, subagent results, and TodoWrite's JSON echo are all
// objects — byte-for-byte unchanged rendering). Anything that is not a valid
// JSON string falls back to string(raw) (never panics, never drops results).
func plainContent(raw json.RawMessage) string {
	// A valid JSON document beginning with '"' IS a string — objects start
	// with '{', arrays with '[', so the first-byte check is the discriminator.
	if len(raw) > 0 && raw[0] == '"' {
		var s string

		err := json.Unmarshal(raw, &s)
		if err == nil {
			return s
		}
	}

	return string(raw)
}

// boundMidTurn keeps the MOST RECENT MidTurnWindowMessages messages, dropping
// only COMPLETE exchange groups: the cut advances past tool-role messages so
// the window never STARTS with an orphaned tool result (its assistant batch
// would be missing — pair-safety). The lean seed is applied by the caller and
// is never dropped.
func boundMidTurn(mid []provider.Message) []provider.Message {
	if len(mid) <= MidTurnWindowMessages {
		return mid
	}

	cut := len(mid) - MidTurnWindowMessages
	for cut < len(mid) && mid[cut].Role == roleToolMsg {
		cut++ // advance to the next group head (assistant message)
	}

	if cut >= len(mid) {
		return nil
	}

	return mid[cut:]
}

// extractSummary builds the bridge summary from the lines before the boundary:
// the last user message excerpt, the deduped file paths from tool calls, and the
// last assistant message excerpt. Pure mechanical extraction (D-02).
func (p *Projector) extractSummary(before []Line) string {
	if len(before) == 0 {
		return ""
	}

	var lastUser, lastAssistant string

	files := []string{}
	seen := map[string]bool{}

	for i := range before {
		l := &before[i]
		switch l.Type {
		case TypeUserMessage:
			lastUser = truncate(extractText(l), MaxSummaryUserChars)
		case TypeAssistantMessage:
			lastAssistant = truncate(l.Text, MaxSummaryAssistantChars)
		case TypeToolCall:
			for _, f := range extractFilePaths(l.Name, l.Input) {
				if !seen[f] {
					seen[f] = true

					files = append(files, f)
				}
			}
		}
	}

	if len(files) > MaxFilesTouched {
		files = files[:MaxFilesTouched]
	}

	var sb strings.Builder
	if lastUser != "" {
		sb.WriteString("last_user=")
		sb.WriteString(lastUser)
		sb.WriteString("\n")
	}

	if lastAssistant != "" {
		sb.WriteString("last_assistant=")
		sb.WriteString(lastAssistant)
		sb.WriteString("\n")
	}

	if len(files) > 0 {
		sb.WriteString("files_touched=")
		sb.WriteString(strings.Join(files, ","))
	}

	return strings.TrimRight(sb.String(), "\n")
}

// findCurrentIntent returns the current user intent: the last user_message after
// the boundary, or (no boundary) the last user_message overall.
func (p *Projector) findCurrentIntent(after, before []Line, turnID string) string {
	// Prefer the user message matching turnID; fall back to the last user_message.
	for i := len(after) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
		v := &after[i]
		if v.Type == TypeUserMessage && (turnID == "" || v.TurnID == turnID) {
			return extractText(v)
		}
	}

	for i := len(after) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
		v := &after[i]
		if v.Type == TypeUserMessage {
			return extractText(v)
		}
	}

	for i := len(before) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
		v := &before[i]
		if v.Type == TypeUserMessage {
			return extractText(v)
		}
	}

	return ""
}

// extractText returns the plain text from a user_message content block slice
// (or the line's Text shorthand).
func extractText(l *Line) string {
	if len(l.Content) > 0 {
		var blocks []ContentBlock

		err := json.Unmarshal(l.Content, &blocks)
		if err == nil {
			var sb strings.Builder

			for _, b := range blocks {
				if b.Text != "" {
					sb.WriteString(b.Text)
				}
			}

			return sb.String()
		}
	}

	return l.Text
}

// extractFilePaths pulls file_path/path fields from a tool-call input for the
// file-bearing tools. Returns the paths found (un-deduped).
func extractFilePaths(toolName string, input json.RawMessage) []string {
	if !fileBearingTools[toolName] || len(input) == 0 {
		return nil
	}

	var m map[string]any

	err := json.Unmarshal(input, &m)
	if err != nil {
		return nil
	}

	var out []string

	for _, k := range []string{"file_path", "path"} {
		if v, ok := m[k].(string); ok && v != "" {
			out = append(out, v)
		}
	}

	return out
}

// truncate returns s trimmed to n runes with an ellipsis marker when truncated.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}

	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "..."
}
