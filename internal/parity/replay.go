package parity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

const scannerBufSize = 1024
const scannerBufInit = 64

// maxDeltaMessageBytes bounds the buffered delta payload per message position
// (T-12-03-02 DoS): an adversarial capture streaming unbounded partial content
// degrades to a counted skip, never an unbounded buffer.
const maxDeltaMessageBytes = 4 << 20 // 4 MiB per accumulated message

// CapturedTurn is one zcode turn: the user prompt paired with the tool-calls
// zcode actually produced (the "zcode arm" of the A/B test — D-05 replay).
type CapturedTurn struct {
	TurnID            string     `json:"turn_id"`
	Prompt            string     `json:"prompt"`
	ExpectedToolCalls []ToolCall `json:"expected_tool_calls"`
	// FixtureSnapshot optionally names a per-turn workspace snapshot the replay
	// run path seeds the turn's isolated scratch from (12-03 Task 2); empty =
	// the suite's base snapshot (the extractor-populated fallback).
	FixtureSnapshot  string `json:"fixture_snapshot,omitempty"`
	DivergenceReason string `json:"divergence_rationale,omitempty"`
}

// rolloutMessage is one entry of a request-level messages window.
type rolloutMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// rolloutLine is the subset of a model_io line the replay reader consumes.
type rolloutLine struct {
	Type    string `json:"type"`
	TurnID  string `json:"turnId"` //nolint:tagliatelle // model_io rollout format
	Request struct {
		Messages      []rolloutMessage `json:"messages"`
		MessagesKind  string           `json:"messagesKind"`  //nolint:tagliatelle // "full" | "delta" | "" (legacy)
		MessageOffset *int             `json:"messageOffset"` //nolint:tagliatelle // delta window start index
	} `json:"request"`
	Response struct {
		ToolCalls []struct {
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"toolCalls"` //nolint:tagliatelle // model_io rollout format
	} `json:"response"`
}

// ExtractReport is the detailed extraction outcome: the turns plus the
// delta-assembly diagnostics (the 12-03 before/after decomposition evidence —
// WINDOWS #6's zero-empty acceptance re-runs this over a capture).
type ExtractReport struct {
	Turns         []CapturedTurn
	Requests      int // model_io records seen
	DeltaRecords  int // messagesKind=delta records
	DeltaSkips    int // records skipped on offset/bounds/budget violations (T-12-03-01/02)
	EmptyExpectat int // turns emitted with zero expected-calls (valid no-tool turns)
}

// ExtractTurnsFromRollout reads a model-io-sess_*.jsonl file and pairs each
// turn's last user-message prompt with the tool-calls zcode produced for that
// turn. Turns with no user prompt are skipped; a turn with zero tool-calls
// yields a CapturedTurn with an empty (non-nil) ExpectedToolCalls (a valid
// empty-sequence match).
//
// Delta-record awareness (12-03, the 2026-08-16 drift report's 6/13
// empty-expectation artifact class): records with messagesKind=delta carry a
// messageOffset-indexed partial view of the conversation. The arithmetic —
// pinned live 2026-08-09 ("delta-record offset arithmetic proves stream
// continuation") and documented in profiles/zcode/drift-reports/2026-08-16-recapture.md — is:
//
//   - kind=full (or legacy kind-absent) records re-anchor: the effective
//     history IS the record's messages (messageOffset 0 by construction).
//   - kind=delta records place their window at absolute index messageOffset:
//     history = history[:offset] + window. A window message landing on an
//     index this turn's stream already wrote MERGES by content-block
//     append (partial-block streaming); a fresh index places a new message.
//     Records whose offset contradicts stream state (out of bounds, or a
//     role flip on a merging index) skip with a counted warning — a
//     corrupted capture degrades to fewer turns, never fabricated
//     expectations (T-12-03-01).
//
// Expectations per turn are the ordered union of (a) the tool_use blocks in
// the turn's assembled assistant messages (the delta-streamed reconstruction
// path — windows that carry the model's tool_use without a response view) and
// (b) the response toolCalls across the turn's records that (a) does not
// already cover — response views are per-request subsets of the same blocks,
// so the merge is multiset-aware and never double-counts. Per-turn aggregation
// (one CapturedTurn per turnId) kills the trailing-text-response duplicate
// class: a turn's final no-tool response adds nothing instead of emitting an
// empty-expectation sibling.
func ExtractTurnsFromRollout(path string) ([]CapturedTurn, error) {
	report, err := ExtractTurnsFromRolloutDetailed(path)
	if err != nil {
		return nil, err
	}

	return report.Turns, nil
}

// ExtractTurnsFromRolloutDetailed is ExtractTurnsFromRollout plus the
// delta-assembly diagnostics (see ExtractTurnsFromRollout for the arithmetic).
func ExtractTurnsFromRolloutDetailed(path string) (ExtractReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return ExtractReport{}, fmt.Errorf("open rollout: %w", err)
	}
	defer func() { _ = f.Close() }()

	acc := newTurnAccumulator()
	rep := ExtractReport{Turns: []CapturedTurn{}}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, scannerBufInit*scannerBufSize), 16*scannerBufSize*scannerBufSize)

	lineIdx := 0
	for scanner.Scan() {
		lineIdx++

		raw := scanner.Bytes()
		if strings.TrimSpace(string(raw)) == "" {
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

		rep.Requests++
		acc.apply(&rep, orDefault(rl.TurnID, fmt.Sprintf("line-%d", lineIdx)), &rl)
	}

	acc.materialize(&rep)

	err = scanner.Err()
	if err != nil {
		return ExtractReport{}, fmt.Errorf("scan rollout: %w", err)
	}

	return rep, nil
}

// turnAccumulator carries the extraction state across a rollout stream: the
// effective conversation (complete, assembled messages) and the current turn's
// accumulation (one CapturedTurn per turnId — per-record emission was the
// duplicate/empty artifact carrier the 2026-08-16 drift report documented).
type turnAccumulator struct {
	history []rolloutMessage // effective conversation state
	active  bool
	turnID  string
	prompt  string
	calls   []ToolCall   // response-pinned calls, arrival order
	partial map[int]bool // indices this turn's stream wrote (merge candidates)
}

func newTurnAccumulator() *turnAccumulator {
	return &turnAccumulator{history: []rolloutMessage{}}
}

// apply folds one model_io record into the stream.
func (a *turnAccumulator) apply(rep *ExtractReport, lineTurn string, rl *rolloutLine) {
	a.maybeOpenTurn(rep, lineTurn)

	switch {
	case rl.Request.MessagesKind == "delta" && rl.Request.MessageOffset != nil:
		rep.DeltaRecords++

		if !a.placeDeltaWindow(*rl.Request.MessageOffset, rl.Request.Messages) {
			rep.DeltaSkips++
		}
	default:
		// Full records (and legacy kind-absent records) re-anchor the
		// effective history at the record's own messages; open delta
		// streams terminate (materialize-on-full).
		a.history = slices.Clone(rl.Request.Messages)
		a.partial = map[int]bool{}
	}

	// The prompt must come from THIS turn's own windows (first record
	// carrying the user message wins): a prompt reconstructed from the
	// history tail alone could be a previous turn's — a fabricated
	// pairing T-12-03-01 forbids.
	if a.prompt == "" {
		a.prompt = lastUserPrompt(rl.Request.Messages)
	}

	a.calls = append(a.calls, responseCalls(rl.Response.ToolCalls)...)
}

// maybeOpenTurn materializes the current turn when the record opens a new one.
func (a *turnAccumulator) maybeOpenTurn(rep *ExtractReport, lineTurn string) {
	if a.active && lineTurn != a.turnID {
		a.materialize(rep)

		// Reset the per-turn accumulation for the incoming turn.
		a.active, a.prompt, a.calls = false, "", nil
	}

	if !a.active {
		a.active = true
		a.turnID = lineTurn
		a.partial = map[int]bool{}
	}
}

// materialize emits the accumulated turn (no-op when the turn never carried
// its initiating user prompt — the honest degrade, T-12-03-01).
func (a *turnAccumulator) materialize(rep *ExtractReport) {
	if !a.active || a.prompt == "" {
		return
	}

	// The turn's slice: messages after the LAST user message in the
	// effective history (the turn's initiating prompt anchors the slice;
	// earlier turns' assistant blocks never leak in).
	assembled := toolUseCalls(a.history[lastUserIndex(a.history)+1:])

	merged := mergeCalls(assembled, a.calls)
	if merged == nil {
		merged = []ToolCall{}
	}

	rep.Turns = append(rep.Turns, CapturedTurn{
		TurnID:            a.turnID,
		Prompt:            a.prompt,
		ExpectedToolCalls: merged,
	})

	if len(merged) == 0 {
		rep.EmptyExpectat++
	}
}

// responseCalls normalizes a response view's tool calls (empty input → {}).
func responseCalls(raw []struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}) []ToolCall {
	out := make([]ToolCall, 0, len(raw))
	for _, c := range raw {
		input := c.Input
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}

		out = append(out, ToolCall{Name: c.Name, Input: input})
	}

	return out
}

// placeDeltaWindow splices a delta window into the effective history at
// absolute index offset. A window message whose index this turn's stream
// already wrote merges by content-block append (partial-block streaming);
// any other occupied index truncates the history first (the new content
// supersedes the tail — the same re-anchor a full record performs). Returns
// false (and leaves history untouched) when the record contradicts stream
// state: offset out of bounds, a role flip on a merging index, or the
// per-message accumulation budget exceeded (T-12-03-01/02).
func (a *turnAccumulator) placeDeltaWindow(offset int, window []rolloutMessage) bool {
	h := a.history

	if offset < 0 || offset > len(h) {
		return false
	}

	// Validate the whole window before mutating anything.
	staged := make([]rolloutMessage, 0, len(window))

	for i, msg := range window {
		idx := offset + i

		switch {
		case idx < len(h) && a.partial[idx]:
			// Partial continuation of a message this turn's stream wrote.
			if h[idx].Role != msg.Role {
				return false
			}

			merged, ok := mergeMessageContent(h[idx], msg)
			if !ok {
				return false
			}

			staged = append(staged, merged)
		case idx < len(h):
			// Occupied by an earlier turn's content: the window supersedes
			// the tail from here on (truncate + place below).
			staged = append(staged, msg)
		default:
			staged = append(staged, msg)
		}
	}

	next := make([]rolloutMessage, 0, offset+len(window))
	next = append(next, h[:offset]...)

	for i, msg := range staged {
		idx := offset + i
		if idx < len(next) {
			next[idx] = msg
		} else {
			next = append(next, msg)
		}

		a.partial[idx] = true
	}

	a.history = next

	return true
}

// mergeMessageContent appends the incoming partial message's content blocks to
// the accumulated message's blocks (the partial-block streaming merge). String
// content replaces string content (a later chunk wins — text continuation);
// anything unmergeable returns false. The accumulation budget is enforced per
// message (T-12-03-02).
func mergeMessageContent(acc, inc rolloutMessage) (rolloutMessage, bool) {
	total := len(acc.Content) + len(inc.Content)
	if total > maxDeltaMessageBytes {
		return rolloutMessage{}, false
	}

	var accBlocks, incBlocks []json.RawMessage

	if json.Unmarshal(acc.Content, &accBlocks) != nil || json.Unmarshal(inc.Content, &incBlocks) != nil {
		// Non-block content (plain strings or nested shapes): the incoming
		// chunk replaces — a text continuation, not a block stream.
		return rolloutMessage{Role: acc.Role, Content: inc.Content}, true
	}

	merged := make([]json.RawMessage, 0, len(accBlocks)+len(incBlocks))
	merged = append(merged, accBlocks...)
	merged = append(merged, incBlocks...)

	raw, err := json.Marshal(merged)
	if err != nil {
		return rolloutMessage{}, false
	}

	return rolloutMessage{Role: acc.Role, Content: raw}, true
}

// toolUseCalls extracts the tool_use blocks of the assistant messages in msg
// slice order — the delta-streamed reconstruction source for a turn's
// expectations.
func toolUseCalls(msgs []rolloutMessage) []ToolCall {
	var calls []ToolCall

	for _, msg := range msgs {
		if msg.Role != "assistant" {
			continue
		}

		var blocks []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}

		if json.Unmarshal(msg.Content, &blocks) != nil {
			continue
		}

		for _, blk := range blocks {
			if blk.Type != "tool_use" {
				continue
			}

			input := blk.Input
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}

			calls = append(calls, ToolCall{Name: blk.Name, Input: input})
		}
	}

	return calls
}

// mergeCalls unions the assembled backbone with the response-pinned calls:
// assembled order wins; response calls the backbone does not already cover
// (multiset-aware — repeated identical calls never collapse) append in
// response order. This pairs a turn's streamed tool_use with a final
// response-only view (the last request's calls never enter any later window).
func mergeCalls(assembled, response []ToolCall) []ToolCall {
	out := slices.Clone(assembled)

	covered := map[string]int{}

	for _, c := range assembled {
		covered[callKey(c)]++
	}

	for _, c := range response {
		k := callKey(c)

		if covered[k] > 0 {
			covered[k]--

			continue
		}

		out = append(out, c)
	}

	return out
}

func callKey(c ToolCall) string {
	return c.Name + "\x00" + string(c.Input)
}

// LoadReplaySession reads a curated_suite.json file (a list of CapturedTurn).
func LoadReplaySession(jsonPath string) ([]CapturedTurn, error) {
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("call: %w", err)
	}

	var turns []CapturedTurn

	err = json.Unmarshal(raw, &turns)
	if err != nil {
		return nil, fmt.Errorf("parse replay session: %w", err)
	}

	return turns, nil
}

// lastUserPrompt returns the content of the last user-role message in the
// effective history, handling string content and array-of-text-block content.
func lastUserPrompt(msgs []rolloutMessage) string {
	for _, v := range slices.Backward(msgs) {
		if v.Role != "user" {
			continue
		}

		return contentToString(v.Content)
	}

	return ""
}

// lastUserIndex returns the index of the last user-role message (-1 when none).
func lastUserIndex(msgs []rolloutMessage) int {
	for i := range slices.Backward(msgs) {
		if msgs[i].Role == "user" {
			return i
		}
	}

	return -1
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
