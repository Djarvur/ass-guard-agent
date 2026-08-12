package parity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

// CapturedTurn is one zcode turn: the user prompt paired with the tool-calls
// zcode actually produced (the "zcode arm" of the A/B test — D-05 replay).
type CapturedTurn struct {
	TurnID            string     `json:"turn_id"`
	Prompt            string     `json:"prompt"`
	ExpectedToolCalls []ToolCall `json:"expected_tool_calls"`
	DivergenceReason  string     `json:"divergence_rationale,omitempty"`
}

// rolloutLine is the subset of a model_io line the replay reader consumes.
type rolloutLine struct {
	Type    string `json:"type"`
	TurnID  string `json:"turnId"` //nolint:tagliatelle // external model_io rollout format (camelCase source) — cannot rename
	Request struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	} `json:"request"`
	Response struct {
		ToolCalls []struct {
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"toolCalls"` //nolint:tagliatelle // external model_io rollout format (camelCase source) — cannot rename
	} `json:"response"`
}

// ExtractTurnsFromRollout reads a model-io-sess_*.jsonl file and pairs each
// turn's last user-message prompt with its response tool-calls. Turns with no
// user prompt are skipped; a turn with zero tool-calls yields a CapturedTurn
// with an empty (non-nil) ExpectedToolCalls (a valid empty-sequence match).
func ExtractTurnsFromRollout(path string) ([]CapturedTurn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open rollout: %w", err)
	}
	defer func() { _ = f.Close() }()

	var turns []CapturedTurn

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	lineIdx := 0
	for scanner.Scan() {
		lineIdx++

		raw := scanner.Bytes()
		if len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}

		var rl rolloutLine

		err := json.Unmarshal(raw, &rl)
		if err != nil {
			continue // skip non-model_io / unparseable lines
		}

		if rl.Type != "model_io" {
			continue
		}

		prompt := lastUserPrompt(rl.Request.Messages)
		if prompt == "" {
			continue // no user prompt to replay against
		}

		tc := make([]ToolCall, 0, len(rl.Response.ToolCalls))

		for _, c := range rl.Response.ToolCalls {
			input := c.Input
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}

			tc = append(tc, ToolCall{Name: c.Name, Input: input})
		}

		turns = append(turns, CapturedTurn{
			TurnID:            orDefault(rl.TurnID, fmt.Sprintf("line-%d", lineIdx)),
			Prompt:            prompt,
			ExpectedToolCalls: tc,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan rollout: %w", err)
	}

	return turns, nil
}

// LoadReplaySession reads a curated_suite.json file (a list of CapturedTurn).
func LoadReplaySession(jsonPath string) ([]CapturedTurn, error) {
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var turns []CapturedTurn
	if err := json.Unmarshal(raw, &turns); err != nil {
		return nil, fmt.Errorf("parse replay session: %w", err)
	}

	return turns, nil
}

// lastUserPrompt returns the content of the last user-role message, handling
// string content and array-of-text-block content.
func lastUserPrompt(msgs []struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}) string {
	for _, v := range slices.Backward(msgs) {
		if v.Role != "user" {
			continue
		}

		return contentToString(v.Content)
	}

	return ""
}

func contentToString(raw json.RawMessage) string {
	// String content.
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// Array of text blocks: [{"type":"text","text":"..."}, ...].
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}

	if json.Unmarshal(raw, &blocks) == nil {
		var b strings.Builder

		for _, blk := range blocks {
			if blk.Type == "text" || blk.Type == "" {
				b.WriteString(blk.Text)
			}
		}

		return b.String()
	}

	return ""
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}

	return s
}
