package session

import (
	"encoding/json"
	"strings"

	"github.com/djarvur/ass-guard-agent/internal/profile"
	"github.com/djarvur/ass-guard-agent/internal/provider"
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
var fileBearingTools = map[string]bool{
	"Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "Bash": true,
}

// Projector builds the lean model-visible window (D-01) by mechanical extraction
// from the transcript (D-02 — NO model call). It is the sole producer of the
// []provider.Message the Shaper consumes. The window resets at each boundary
// (SESS-04): the lean seed = system (added by the Shaper from the profile) +
// task summary + current user message, with ZERO carry-forward of prior turn
// messages as separate messages.
type Projector struct {
	prof    profile.Profile
	manager *Manager
}

// NewProjector returns a Projector over the given profile + transcript Manager.
func NewProjector(prof profile.Profile, m *Manager) *Projector {
	return &Projector{prof: prof, manager: m}
}

// Project builds the lean window the model sees for the given turn. The window
// is a small slice of user-role messages: a task-summary message (files touched
// + last user/assistant excerpts from before the boundary) bridging the prior
// context, plus the current user message. Prior assistant/tool turns are NOT
// carried as separate messages (D-01 zero carry-forward).
func (p *Projector) Project(turnID string) ([]provider.Message, error) {
	lines, err := p.manager.ReadAll()
	if err != nil {
		return nil, err
	}

	// Find the last boundary — the reset point.
	boundaryIdx := -1
	for i, l := range lines {
		if l.Type == TypeBoundary {
			boundaryIdx = i
		}
	}

	var beforeBoundary, afterBoundary []Line
	if boundaryIdx >= 0 {
		beforeBoundary = lines[:boundaryIdx]
		afterBoundary = lines[boundaryIdx+1:]
	} else {
		// No boundary yet: everything is "before"; the current turn's user
		// message is the last user_message overall.
		beforeBoundary = lines
		afterBoundary = nil
	}

	summary := p.extractSummary(beforeBoundary)
	currentIntent := p.findCurrentIntent(afterBoundary, beforeBoundary, turnID)

	// The lean window is ONE user message: the task summary + the current intent.
	// (The Shaper adds the system prompt separately from p.prof.System.)
	var content string
	if summary != "" {
		content = "Task summary (mechanical, post-boundary):\n" + summary + "\n\n--- Current request ---\n" + currentIntent
	} else {
		// First turn: no summary, just the intent.
		content = currentIntent
	}
	return []provider.Message{{Role: "user", Content: content}}, nil
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
	for _, l := range before {
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
	for i := len(after) - 1; i >= 0; i-- {
		if after[i].Type == TypeUserMessage && (turnID == "" || after[i].TurnID == turnID) {
			return extractText(after[i])
		}
	}
	for i := len(after) - 1; i >= 0; i-- {
		if after[i].Type == TypeUserMessage {
			return extractText(after[i])
		}
	}
	for i := len(before) - 1; i >= 0; i-- {
		if before[i].Type == TypeUserMessage {
			return extractText(before[i])
		}
	}
	return ""
}

// extractText returns the plain text from a user_message content block slice
// (or the line's Text shorthand).
func extractText(l Line) string {
	if len(l.Content) > 0 {
		var blocks []ContentBlock
		if err := json.Unmarshal(l.Content, &blocks); err == nil {
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
	if err := json.Unmarshal(input, &m); err != nil {
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
