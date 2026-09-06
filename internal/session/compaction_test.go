package session //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// Shared fixture strings (the battery repeats them across subtests).
const (
	errCompProvider   = "provider"
	compParentModel   = "parent-model"
	compTextDone      = "done"
	compCallE2E       = "call_e2e"
	promptContinue    = "continue"
	promptEditTheFile = "edit the file"
	promptNextStep    = "next step"
)

// --- The 19-04 scripted provider (the battery's fake — fakeProvider cannot
// emit usage chunks or mid-stream errors, both load-bearing here). ---

// compScript is one scripted Stream call: the chunks the fake delivers, in
// order (usage → tool_use(s) OR text → done), or a failure (callErr fails the
// synchronous Stream; errChunk aborts mid-stream; hang blocks until ctx dies —
// the timeout leg).
type compScript struct {
	text      string
	toolCalls []provider.ToolCall
	usage     *provider.Usage
	finish    string
	errChunk  error
	callErr   error
	hang      bool
}

// compactionProvider pops one script entry per Stream call (degrading to a
// bare end_turn when the script runs dry), records the profile copies and
// message batches it was called with, and publishes RequestShaped to the bus
// like fakeProvider does — the D-10 same-pipeline attribution the assertions
// ride.
type compactionProvider struct {
	mu       sync.Mutex
	script   []compScript
	bus      *event.Bus
	profiles []*profile.Profile
	msgs     [][]provider.Message
	calls    int

	// turnIDOf mirrors the production capturer's attribution seam (09-01: the
	// serve-path closure calls sess.CurrentTurnID() when RequestShaped fires).
	// nil → "" (the pre-attribution fake behavior).
	turnIDOf func() string
}

//nolint:funlen // one scripted scenario dispatcher
func (p *compactionProvider) Stream(
	ctx context.Context, prof *profile.Profile, msgs []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.mu.Lock()
	p.calls++

	s := compScript{finish: stopEndTurn}
	if len(p.script) > 0 {
		s = p.script[0]
		p.script = p.script[1:]
	}

	if prof != nil {
		cp := *prof
		p.profiles = append(p.profiles, &cp)
	}

	p.msgs = append(p.msgs, append([]provider.Message(nil), msgs...))

	bus := p.bus
	p.mu.Unlock()

	if bus != nil {
		turnID := ""
		if p.turnIDOf != nil {
			turnID = p.turnIDOf()
		}

		bus.Publish(event.RequestShaped{
			TurnID:          turnID,
			VerbatimRequest: json.RawMessage(`{"model":"scripted"}`),
			Profile:         prof.Name,
			Timestamp:       time.Now(),
		})
	}

	if s.callErr != nil {
		return nil, s.callErr
	}

	ch := make(chan provider.StreamChunk, 8)

	go func() {
		defer close(ch)

		if s.hang {
			<-ctx.Done() // the timeout leg: never deliver, just outlive the bound

			return
		}

		if s.usage != nil {
			ch <- provider.StreamChunk{Type: "usage", Usage: s.usage}
		}

		for _, tc := range s.toolCalls {
			tcCopy := tc

			id := tc.ID
			if id == "" {
				id = tc.Name
			}

			ch <- provider.StreamChunk{Type: blockToolUse, ToolCall: &tcCopy, ToolCallID: id}
		}

		if s.text != "" {
			ch <- provider.StreamChunk{Type: blockText, Text: s.text}
		}

		if s.errChunk != nil {
			ch <- provider.StreamChunk{Type: chunkErrorType, Error: s.errChunk}

			return // no done follows an error chunk
		}

		finish := s.finish
		if finish == "" {
			finish = stopEndTurn
		}

		ch <- provider.StreamChunk{Type: stopDone, FinishReason: finish}
	}()

	return ch, nil
}

func (p *compactionProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	return provider.Response{}, nil
}

func (p *compactionProvider) ToolResultMessage(_ string, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (p *compactionProvider) SupportsImages() bool { return false }

func (p *compactionProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.calls
}

// messagesOf returns the message batch of the nth Stream call (0-indexed).
func (p *compactionProvider) messagesOf(n int) []provider.Message {
	p.mu.Lock()
	defer p.mu.Unlock()

	if n < 0 || n >= len(p.msgs) {
		return nil
	}

	return p.msgs[n]
}

// profileOf returns the profile copy the nth Stream call was shaped from.
func (p *compactionProvider) profileOf(n int) *profile.Profile {
	p.mu.Lock()
	defer p.mu.Unlock()

	if n < 0 || n >= len(p.profiles) {
		return nil
	}

	return p.profiles[n]
}

// newCompactionTestSession builds a Session over the scripted provider with a
// parent model stamped on the profile (the copy-discipline assertions need a
// non-empty baseline).
func newCompactionTestSession(
	t *testing.T, script []compScript,
) (*Session, *Manager, *compactionProvider, *event.Bus) {
	t.Helper()

	bus := event.NewBus()
	m := newTestManager(t, "s-comp")
	cp := &compactionProvider{script: script, bus: bus}

	prof := fakeProfile("compaction agent")
	prof.Model = compParentModel

	s := &Session{
		Manager:   m,
		Projector: NewProjector(prof, m),
		Provider:  cp,
		Bus:       bus,
		Semaphore: provider.NewSemaphore(4),
		Profile:   *prof,
		WorkDir:   t.TempDir(),
		SessionID: "s-comp",
	}
	cp.turnIDOf = s.CurrentTurnID

	return s, m, cp, bus
}

// startCompactionWriter runs the async TranscriptWriter over the session's bus
// (the D-10 attribution proof needs the request_shaped lines on disk) and
// returns nothing — t.Cleanup owns the lifecycle.
func startCompactionWriter(t *testing.T, m *Manager, bus *event.Bus) {
	t.Helper()

	tw := NewTranscriptWriter(m, bus, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go tw.Run(ctx)
}

// waitForLines polls the transcript until pred over the lines holds (the async
// writer's appends land out-of-band) or the deadline expires.
func waitForLines(t *testing.T, m *Manager, what string, pred func([]Line) bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lines, err := m.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		if pred(lines) {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s in the transcript", what)
}

// markersOf returns the compaction marker lines in append order.
func markersOf(t *testing.T, m *Manager) []Line {
	t.Helper()

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var out []Line

	for i := range lines {
		if lines[i].Type == TypeCompaction {
			out = append(out, lines[i])
		}
	}

	return out
}

// overflowErr builds a 19-02 overflow-classified stream error: the community
// 400 "prompt is too long" envelope riding KindStructural — what
// provider.IsOverflow matches.
func overflowErr() error {
	//nolint:err113 // the community 400 envelope is a dynamic fixture message
	return provider.ClassifyHTTP("anthropic", compParentModel, 400,
		errors.New(`prompt is too long: 200936 tokens > 199999 maximum`))
}

// The D-09 fixture failures (static package-level errors — the fixture's
// stable identities, the err113-recommended shape).
var (
	errSummarizerExploded = errors.New("summarizer exploded mid-stream")
	errSummarizerRejected = errors.New("summarizer rejected")
)

// --- Task 2 battery: loop-head wiring, overflow retry-once, CompactNow ---

// TestCompaction_OverflowRetryOnce pins criterion 3: an overflow-classified
// stream error force-compacts (threshold bypass) and re-sends EXACTLY ONCE —
// the recovery resend completes the turn; a second overflow in the SAME turn
// reaches the existing appendError return with no further retry (Pitfall 8).
func TestCompaction_OverflowRetryOnce(t *testing.T) { //nolint:funlen // battery
	t.Parallel()

	t.Run("recovery: fail overflow, compact, resend, turn completes", func(t *testing.T) {
		t.Parallel()

		// Settings DISABLED on purpose: the retry is the backstop and fires
		// regardless of the threshold gate (manual-intent class, D-11).
		s, m, cp, _ := newCompactionTestSession(t, []compScript{
			{callErr: overflowErr()},                         // 1: the turn's send — overflow
			{text: "RECOVERED-SUMMARY", finish: stopEndTurn}, // 2: the forced compact's summarizer
			{text: "all good now", finish: stopEndTurn},      // 3: the retry send
		})

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Prompt: %v (the retry must complete the turn)", err)
		}

		if stop != stopEndTurn {
			t.Errorf("stop = %q; want end_turn", stop)
		}

		if got := len(markersOf(t, m)); got != 1 {
			t.Errorf("markers = %d; want 1 (the forced compaction)", got)
		}

		if calls := cp.callCount(); calls != 3 {
			t.Errorf("stream calls = %d; want 3 (fail, summarize, resend-once)", calls)
		}
	})

	t.Run("fail-twice: the second overflow fails the turn, exactly one retry", func(t *testing.T) {
		t.Parallel()

		s, m, cp, _ := newCompactionTestSession(t, []compScript{
			{callErr: overflowErr()},                            // 1: overflow → forced compact + retry
			{text: "SUMMARY-BEFORE-RETRY", finish: stopEndTurn}, // 2: the compact's summarizer
			{callErr: overflowErr()},                            // 3: the retry — overflow AGAIN
			{text: "MUST-NEVER-RUN", finish: stopEndTurn},       // 4: a second retry must not exist
		})
		s.SetCompactionSettings(true, 80, 1000)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
		if err == nil {
			t.Fatal("Prompt error = nil; want the turn to FAIL on the second overflow")
		}

		if stop != "" {
			t.Errorf("stop = %q; want empty on the failed turn", stop)
		}

		// The existing error path: an investigate-and-fix-ready provider
		// error line lands in the transcript.
		lines, rerr := m.ReadAll()
		if rerr != nil {
			t.Fatalf("ReadAll: %v", rerr)
		}

		hasProviderErr := false

		for i := range lines {
			if lines[i].Type == TypeError && lines[i].Component == errCompProvider {
				hasProviderErr = true
			}
		}

		if !hasProviderErr {
			t.Error("transcript missing the provider error line (the existing appendError path)")
		}

		// EXACTLY one retry: one marker (the first overflow's compaction) and
		// NO fourth call — the second overflow never re-compacts, never
		// re-sends (Pitfall 8's unbounded loop).
		if got := len(markersOf(t, m)); got != 1 {
			t.Errorf("markers = %d; want 1 (the retry's compaction alone)", got)
		}

		if calls := cp.callCount(); calls != 3 {
			t.Errorf("stream calls = %d; want 3 (fail, summarize, fail — no second retry)", calls)
		}
	})
}

// TestCompaction_LoopHead pins D-02: the pre-request check runs at the top of
// EVERY maxIterations iteration — an enabled multi-iteration turn (tool loop)
// counts one check per iteration — while disabled settings skip the check
// entirely (zero invocations, zero notes: zero behavior delta).
func TestCompaction_LoopHead(t *testing.T) {
	t.Parallel()

	// A two-iteration turn: iteration 1 returns a tool call (the stub
	// executor answers it, looping), iteration 2 ends the turn.
	script := []compScript{
		{
			toolCalls: []provider.ToolCall{{ID: "call_lh", Name: toolRead,
				Input: json.RawMessage(`{"file_path":"a.txt"}`)}},
			finish: blockToolUse,
		},
		{text: compTextDone, finish: stopEndTurn},
	}

	t.Run("enabled: one check per iteration", func(t *testing.T) {
		t.Parallel()

		s, _, _, _ := newCompactionTestSession(t, script)
		s.SetCompactionSettings(true, 80, 1_000_000) // enabled, far from firing

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn (fixture broken)", stop)
		}

		if got := s.compactionChecks.Load(); got != 2 {
			t.Errorf("check invocations = %d; want 2 (one per loop iteration, D-02)", got)
		}
	})

	t.Run("disabled: skipped entirely", func(t *testing.T) {
		t.Parallel()

		s, _, cp, _ := newCompactionTestSession(t, script)

		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "go"}})
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}

		if stop != stopEndTurn {
			t.Fatalf("stop = %q; want end_turn (fixture broken)", stop)
		}

		if got := s.compactionChecks.Load(); got != 0 {
			t.Errorf("check invocations = %d; want 0 (disabled skips entirely)", got)
		}

		if got := s.compactionNotes.Load(); got != 0 {
			t.Errorf("note emissions = %d; want 0 (disabled never compacts)", got)
		}

		if calls := cp.callCount(); calls != 2 {
			t.Errorf("stream calls = %d; want 2 (only the turn's own sends — zero compaction)", calls)
		}
	})
}

// TestCompactNow pins D-11: the manual entry compacts immediately regardless
// of the threshold or the enabled flag, appends the same marker shape, and
// returns the outcome (nil on the D-09 degrade) — the single public entry
// Phase 20's /compact handler calls.
func TestCompactNow(t *testing.T) {
	t.Parallel()

	s, m, _, _ := newCompactionTestSession(t, []compScript{
		{text: "turn one reply", finish: stopEndTurn}, // the Prompt turn's own send
		{text: "MANUAL-SUMMARY", usage: &provider.Usage{InputTokens: 30, OutputTokens: 12},
			finish: stopEndTurn}, // CompactNow's summarizer
	})

	// One turn first so the marker's turn attribution is observable, with
	// compaction DISABLED and the usage far under any threshold.
	stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "hi"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if stop != stopEndTurn {
		t.Fatalf("stop = %q; want end_turn (fixture broken)", stop)
	}

	s.lastInputTokens.Store(42) // the anchor the marker snapshots

	err = s.CompactNow(context.Background())
	if err != nil {
		t.Fatalf("CompactNow: %v (nil on success AND on the D-09 degrade)", err)
	}

	mk := markersOf(t, m)
	if len(mk) != 1 {
		t.Fatalf("markers = %d; want 1 (manual intent overrides the gates)", len(mk))
	}

	marker := mk[0]
	if marker.Summary != "MANUAL-SUMMARY" {
		t.Errorf("marker summary = %q; want MANUAL-SUMMARY (the same machinery)", marker.Summary)
	}

	if marker.TurnID != "s-comp-turn-001" {
		t.Errorf("marker turnID = %q; want the current turn (attribution)", marker.TurnID)
	}

	if marker.InputTokens != 42 {
		t.Errorf("marker input snapshot = %d; want 42 (the trigger anchor)", marker.InputTokens)
	}

	if marker.PreRef == "" || marker.PostRef == "" {
		t.Errorf("marker pointers = (%q, %q); want both set (the D-21 shape)", marker.PreRef, marker.PostRef)
	}

	if got := s.compactionNotes.Load(); got != 1 {
		t.Errorf("note emissions = %d; want 1 (exactly one per compaction start)", got)
	}
}

// --- Task 3: the offline end-to-end proof (ROADMAP criterion 1's machinery) ---

// TestCompaction_EndToEnd drives a REAL over-threshold cycle through the full
// engine: a conversation whose provider usage crosses the threshold compacts
// on the next pre-request check, the next projected window carries the summary
// seed with a pair-safe tail, the turn completes coherently, the post-
// compaction usage buys headroom (no re-fire), and a pre-phase transcript
// projects identically with the engine on versus off (the additive-only
// guarantee at engine level).
func TestCompaction_EndToEnd(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen,maintidx // battery
	t.Parallel()

	t.Run("over-threshold session compacts and continues coherently", func(t *testing.T) {
		t.Parallel()

		s, m, cp, bus := newCompactionTestSession(t, []compScript{
			// Turn 1: the conversation grows until usage crosses the 80% line.
			{text: "working on it", usage: &provider.Usage{InputTokens: 900, OutputTokens: 50},
				finish: stopEndTurn},
			// Turn 2's loop-head check fires → the summarizer (light tier,
			// private stream, own usage line). The marker lands DURING turn 2
			// — under 19-03's pinned position rule it resets turns that START
			// after it, so the seed reaches turn 3's window.
			{text: "E2E-SUMMARY", usage: &provider.Usage{InputTokens: 30, OutputTokens: 12},
				finish: stopEndTurn},
			// Turn 2's own sends (mechanical window; a tool call so turn 2
			// exercises its loop and leaves a post-marker exchange behind).
			{
				usage: &provider.Usage{InputTokens: 300, OutputTokens: 20},
				toolCalls: []provider.ToolCall{{ID: compCallE2E, Name: toolRead,
					Input: json.RawMessage(`{"file_path":"a.txt"}`)}},
				finish: blockToolUse,
			},
			{text: compTextDone, finish: stopEndTurn},
			// Turn 3 — the first turn STARTING after the marker: its window
			// carries the summary seed; a tool call so iteration 2 projects a
			// real post-marker tail.
			{
				usage: &provider.Usage{InputTokens: 250, OutputTokens: 15},
				toolCalls: []provider.ToolCall{{ID: "call_e3", Name: toolRead,
					Input: json.RawMessage(`{"file_path":"b.txt"}`)}},
				finish: blockToolUse,
			},
			{text: "done three", finish: stopEndTurn},
			// Turn 4 — the second turn after compaction: still below the
			// threshold, no re-fire.
			{text: "fourth done", finish: stopEndTurn},
		})
		s.SetCompactionSettings(true, 80, 1000)
		startCompactionWriter(t, m, bus)

		// Turn 1 completes; its usage chunk crosses the threshold (900 >= 800).
		stop, err := s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "start the work"}})
		if err != nil || stop != stopEndTurn {
			t.Fatalf("turn 1: stop=%q err=%v", stop, err)
		}

		if got := s.lastInputTokens.Load(); got != 900 {
			t.Fatalf("usage read model = %d; want 900 (the D-01 anchor)", got)
		}

		// Turn 2: the loop-head check compacts BEFORE the first projection;
		// the tool loop converges and the turn completes normally.
		stop, err = s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: promptContinue}})
		if err != nil {
			t.Fatalf("turn 2: %v (the compacting turn must complete normally)", err)
		}

		if stop != stopEndTurn {
			t.Errorf("turn 2 stop = %q; want end_turn", stop)
		}

		mk := markersOf(t, m)
		if len(mk) != 1 {
			t.Fatalf("markers = %d; want 1 (compacted exactly once)", len(mk))
		}

		if mk[0].Summary != "E2E-SUMMARY" {
			t.Errorf("marker summary = %q; want E2E-SUMMARY", mk[0].Summary)
		}

		if mk[0].TurnID != "s-comp-turn-002" {
			t.Errorf("marker turnID = %q; want the compacting turn", mk[0].TurnID)
		}

		// The compacting turn's OWN window keeps the pre-phase shape (the
		// marker never reseeds its producing turn — 19-03's pinned rule), so
		// turn 2's send carries the mechanical seed + its intent.
		t2win := cp.messagesOf(2) // call 3: turn 2 iteration 1's send
		if len(t2win) == 0 || strings.Contains(t2win[0].Content, "E2E-SUMMARY") {
			t.Errorf("producing turn's window reseeded mid-turn (19-03 pin violation):\n%s",
				msgSummaryList(t2win))
		}

		if !strings.Contains(t2win[0].Content, promptContinue) {
			t.Errorf("turn 2 window missing the current intent:\n%s", t2win[0].Content)
		}

		// Turn 3: the first turn STARTING after the marker — below the
		// threshold now (headroom), completes on the compacted window.
		stop, err = s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "third"}})
		if err != nil || stop != stopEndTurn {
			t.Fatalf("turn 3: stop=%q err=%v", stop, err)
		}

		// THE seed assertion: turn 3's window carries the summary seed (the
		// durable D-06 window the marker reset it to).
		t3win := cp.messagesOf(4) // call 5: turn 3 iteration 1's send
		if len(t3win) == 0 || !strings.Contains(t3win[0].Content, "E2E-SUMMARY") {
			t.Fatalf("post-compaction window missing the summary seed:\n%s", msgSummaryList(t3win))
		}

		if !strings.Contains(t3win[0].Content, "third") {
			t.Errorf("post-compaction seed missing the current intent:\n%s", t3win[0].Content)
		}

		// Turn 3 iteration 2: the seed plus the post-marker tail — turn 2's
		// tool exchange and turn 3's own, pair-safe (the cut never starts
		// with an orphaned tool result).
		t3second := cp.messagesOf(5)
		if len(t3second) == 0 || !strings.Contains(t3second[0].Content, "E2E-SUMMARY") {
			t.Fatalf("turn 3 iteration-2 window lost the seed (D-06 durable):\n%s", msgSummaryList(t3second))
		}

		assertTailPairSafe(t, t3second)

		var hasUse, hasResult bool

		for _, mm := range t3second[1:] {
			if mm.Role == roleAssistant && len(mm.ToolCalls) > 0 && mm.ToolCalls[0].ID == compCallE2E {
				hasUse = true
			}

			if mm.Role == roleToolMsg && mm.ToolCallID == compCallE2E {
				hasResult = true
			}
		}

		if !hasUse || !hasResult {
			t.Errorf("post-marker tail lost turn 2's tool exchange (use=%v result=%v):\n%s",
				hasUse, hasResult, msgSummaryList(t3second))
		}

		// D-10 attribution, on disk: the summarizer's request landed as a
		// request_shaped line for the CURRENT turn (3 attributed requests for
		// turn 2 = the summarizer's + the two turn sends), and its own usage
		// line (30/12) is in the transcript.
		const turn2 = "s-comp-turn-002"

		waitForLines(t, m, "3 request_shaped lines attributed to turn 2", func(lines []Line) bool {
			n := 0

			for i := range lines {
				if lines[i].Type == TypeRequestShaped && lines[i].TurnID == turn2 {
					n++
				}
			}

			return n >= 3
		})

		lines, err := m.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		hasSummarizerUsage := false

		for i := range lines {
			if lines[i].Type == TypeUsage && lines[i].TurnID == turn2 &&
				lines[i].InputTokens == 30 && lines[i].OutputTokens == 12 {
				hasSummarizerUsage = true
			}
		}

		if !hasSummarizerUsage {
			t.Error("transcript missing the summarizer's own usage line for turn 2 (D-10)")
		}

		// Headroom: the second turn after compaction starts BELOW the
		// threshold (250 << 800) — the compaction bought room; nothing
		// re-fires.
		stop, err = s.Prompt(context.Background(), []ContentBlock{{Type: blockText, Text: "fourth"}})
		if err != nil || stop != stopEndTurn {
			t.Fatalf("turn 4: stop=%q err=%v", stop, err)
		}

		if got := len(markersOf(t, m)); got != 1 {
			t.Errorf("markers after turn 4 = %d; want 1 (headroom: no re-fire)", got)
		}

		if got := s.compactionNotes.Load(); got != 1 {
			t.Errorf("note emissions after turn 4 = %d; want 1 (exactly one compaction)", got)
		}
	})

	t.Run("pre-phase transcript projects identically with the engine on", func(t *testing.T) {
		t.Parallel()

		// A pre-phase transcript (no markers): prior turns, a boundary, a
		// mid-turn exchange, and the projected turn's user message.
		s, m, _, _ := newCompactionTestSession(t, nil)
		mustAppend(t, m.AppendUserMessage("turn_040", []ContentBlock{{Type: blockText, Text: promptEditTheFile}}),
			"AppendUserMessage")
		mustAppend(t, m.AppendToolCall("turn_040", "tc_pre", toolRead, json.RawMessage(`{"file_path":"/a/go.mod"}`)),
			"AppendToolCall")
		mustAppend(t, m.AppendToolResult("turn_040", "tc_pre", json.RawMessage(`{"out":"module x"}`), false),
			"AppendToolResult")
		mustAppend(t, m.AppendAssistantMessage("turn_040", "done editing"), "AppendAssistantMessage")
		mustAppend(t, m.AppendBoundary("mutating-command:Edit", "tc_pre", "turn_040"), "AppendBoundary")
		mustAppend(t, m.AppendUserMessage("turn_041", []ContentBlock{{Type: blockText, Text: promptNextStep}}),
			"AppendUserMessage")
		mustAppend(t, m.AppendToolCall("turn_041", "tc_cur", toolBash, json.RawMessage(`{"command":"ls"}`)),
			"AppendToolCall")
		mustAppend(t, m.AppendToolResult("turn_041", "tc_cur", json.RawMessage(`"files"`), false),
			"AppendToolResult")

		off, err := s.Projector.Project("turn_041")
		if err != nil {
			t.Fatalf("Project(engine off): %v", err)
		}

		// Engine ON: same transcript, live-applied settings (the projector
		// budget flips too) — the projection must be byte-identical.
		s.SetCompactionSettings(true, 80, 1000)

		on, err := s.Projector.Project("turn_041")
		if err != nil {
			t.Fatalf("Project(engine on): %v", err)
		}

		if !reflect.DeepEqual(off, on) {
			t.Errorf("engine-on projection drifted from engine-off on a no-marker transcript:\n off: %s\n on:  %s",
				msgSummaryList(off), msgSummaryList(on))
		}
	})
}

// --- Task 1 battery: threshold math, D-09 degrade, chaining, bus isolation ---

// TestCompaction_Threshold pins D-01's trigger math: the threshold fires at
// EXACTLY the boundary (inclusive comparison), one unit below does not, the
// added-since estimate is transcript content appended after the last
// request_shaped line divided by 4 truncating — never the request's own total
// body size (Pitfall 7's double-count).
func TestCompaction_Threshold(t *testing.T) { //nolint:gocognit,cyclop,funlen // battery
	t.Parallel()

	t.Run("boundary fires; one below does not", func(t *testing.T) {
		t.Parallel()

		// 800 == 1000 * 80/100 exactly — inclusive comparison fires.
		if !overThreshold(800, 0, 1000, 80) {
			t.Error("threshold must fire at exactly the boundary (inclusive >=)")
		}

		// 799 + 0 is one unit below — must not fire.
		if overThreshold(799, 0, 1000, 80) {
			t.Error("threshold fired one unit below the boundary")
		}

		// The estimate participates: 700 + 100 == 800 fires.
		if !overThreshold(700, 100, 1000, 80) {
			t.Error("threshold must fire when usage + estimate reaches the boundary")
		}

		// 700 + 99 is one below.
		if overThreshold(700, 99, 1000, 80) {
			t.Error("threshold fired below the boundary with a nonzero estimate")
		}

		// A zero limit never fires (no context window resolved).
		if overThreshold(1<<40, 1<<40, 0, 80) {
			t.Error("zero context limit must never fire")
		}
	})

	t.Run("estimate zero when nothing appended since the last request", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-est0")
		mustAppend(t, m.AppendUserMessage("t1", []ContentBlock{{Type: blockText, Text: "hi"}}), "AppendUserMessage")
		mustAppend(t,
			m.AppendRequestShaped("t1", "test", time.Now(), audit.RequestMeta{Ref: "r1", Model: "m", Bytes: 100}),
			"AppendRequestShaped")

		lines, err := m.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		if got := estimateLinesSinceLastRequest(lines); got != 0 {
			t.Errorf("estimate = %d; want 0 (nothing appended after the last request_shaped)", got)
		}
	})

	t.Run("estimate never derived from the request's total bytes", func(t *testing.T) {
		t.Parallel()

		m := newTestManager(t, "s-estbytes")
		mustAppend(t, m.AppendUserMessage("t1", []ContentBlock{{Type: blockText, Text: "hi"}}), "AppendUserMessage")
		// A HUGE request body (Bytes) with NOTHING appended after it: the
		// estimate must stay zero — the request's own size is the anchor's
		// complement, never the added-since term (Pitfall 7).
		mustAppend(t,
			m.AppendRequestShaped("t1", "test", time.Now(), audit.RequestMeta{Ref: "r1", Model: "m", Bytes: 999999}),
			"AppendRequestShaped")

		lines, err := m.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		est := estimateLinesSinceLastRequest(lines)
		if est != 0 {
			t.Fatalf("estimate = %d; want 0 (the request's own Bytes must not count)", est)
		}

		// lastInput one below the boundary + zero estimate: no fire.
		if overThreshold(799, est, 1000, 80) {
			t.Error("threshold fired off the request's own body size (Pitfall 7 double-count)")
		}

		// Content appended AFTER the request DOES count: a 400-byte line
		// yields 100 estimate units (truncating /4), pushing 700 over.
		mustAppend(t, m.AppendAssistantMessage("t1", strings.Repeat("x", 380)), "AppendAssistantMessage")

		lines, err = m.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll(2): %v", err)
		}

		est = estimateLinesSinceLastRequest(lines)
		if est < 90 || est > 120 { // ~400 envelope bytes / 4, envelope-tolerant
			t.Errorf("estimate = %d; want ~100 (400 envelope bytes / 4 truncating)", est)
		}

		if !overThreshold(700, est, 1000, 80) {
			t.Error("threshold must fire once post-request content pushes usage+estimate over")
		}
	})

	t.Run("engine fires compact at the boundary", func(t *testing.T) {
		t.Parallel()

		s, m, cp, _ := newCompactionTestSession(t, []compScript{
			{text: "SUMMARY-BOUNDARY", finish: stopEndTurn},
		})
		s.SetCompactionSettings(true, 80, 1000)
		s.lastInputTokens.Store(800) // exactly the boundary

		s.maybeCompact(context.Background(), "t-comp")

		if got := len(markersOf(t, m)); got != 1 {
			t.Fatalf("markers = %d; want 1 (boundary must compact)", got)
		}

		if cp.callCount() != 1 {
			t.Errorf("summarizer calls = %d; want 1", cp.callCount())
		}
	})

	t.Run("engine does not fire one below", func(t *testing.T) {
		t.Parallel()

		s, m, cp, _ := newCompactionTestSession(t, nil)
		s.SetCompactionSettings(true, 80, 1000)
		s.lastInputTokens.Store(799) // one below

		s.maybeCompact(context.Background(), "t-comp")

		if got := len(markersOf(t, m)); got != 0 {
			t.Errorf("markers = %d; want 0 (below the boundary)", got)
		}

		if cp.callCount() != 0 {
			t.Errorf("summarizer calls = %d; want 0", cp.callCount())
		}
	})
}

// TestCompaction_SettingsFromConfig pins 19-05 Task 1's settings source: a
// session initialized from a modelrouting-loaded config (the runtime's
// construction seam — Load resolves the effective values, SetCompactionSettings
// lands them) compares against the CONFIG's threshold, not a shadow
// session-side default, and SetCompactionSettings re-targets the very next
// comparison. The clamp subtests pin the defensive 1..100 window: hand-edited
// config values of 0 and 500 compare as 1 and 100.
func TestCompaction_SettingsFromConfig(t *testing.T) {
	t.Parallel()

	// loadCompactionConfig loads one project layer over the embedded floor —
	// the same Load the runtime's construction seam applies.
	loadCompactionConfig := func(t *testing.T, yaml string) modelrouting.CompactionConfig {
		t.Helper()

		path := filepath.Join(t.TempDir(), "config.yaml")
		if werr := os.WriteFile(path, []byte(yaml), 0o600); werr != nil {
			t.Fatalf("write config: %v", werr)
		}

		cfg, err := modelrouting.Load(path)
		if err != nil {
			t.Fatalf("modelrouting.Load: %v", err)
		}

		return cfg.Compaction
	}

	t.Run("construction over a 60-percent config compares against 60", func(t *testing.T) {
		t.Parallel()

		comp := loadCompactionConfig(t, "compaction:\n  threshold_pct: 60\n")

		s, m, cp, _ := newCompactionTestSession(t, []compScript{
			{text: "SUMMARY-60", finish: stopEndTurn},
		})
		s.SetCompactionSettings(comp.Enabled, comp.ThresholdPct, 1000)

		// 650 of 1000 is over the 60% boundary (600) but UNDER the previous
		// hardcoded default's 80% (800) — a fire here proves the loaded
		// config value is the comparison value.
		s.lastInputTokens.Store(650)
		s.maybeCompact(context.Background(), "t-cfg60")

		if got := len(markersOf(t, m)); got != 1 {
			t.Fatalf("markers = %d; want 1 (the config's 60%% threshold must govern)", got)
		}

		if cp.callCount() != 1 {
			t.Errorf("summarizer calls = %d; want 1", cp.callCount())
		}
	})

	t.Run("SetCompactionSettings re-targets the next comparison", func(t *testing.T) {
		t.Parallel()

		comp := loadCompactionConfig(t, "compaction:\n  threshold_pct: 60\n")

		s, m, cp, _ := newCompactionTestSession(t, []compScript{
			{text: "SUMMARY-40", finish: stopEndTurn},
		})
		s.SetCompactionSettings(comp.Enabled, comp.ThresholdPct, 1000)

		// 500 of 1000: below 60% (no fire at the constructed settings).
		s.lastInputTokens.Store(500)
		s.maybeCompact(context.Background(), "t-retarget")
		if got := len(markersOf(t, m)); got != 0 {
			t.Fatalf("markers = %d; want 0 (500 is below the 60%% boundary)", got)
		}

		// Live-apply 40: the SAME usage now fires (500 >= 400).
		s.SetCompactionSettings(true, 40, 1000)
		s.maybeCompact(context.Background(), "t-retarget")

		if got := len(markersOf(t, m)); got != 1 {
			t.Fatalf("markers = %d; want 1 (the next check must use the applied 40%%)", got)
		}

		if cp.callCount() != 1 {
			t.Errorf("summarizer calls = %d; want 1", cp.callCount())
		}
	})

	t.Run("out-of-range values clamp into 1..100 at the comparison", func(t *testing.T) {
		t.Parallel()

		// pct 0 compares as 1: 1% of 100 = 1 unit — 1 fires, 0 does not.
		if !overThreshold(1, 0, 100, 0) {
			t.Error("pct 0 must compare as 1 (a hand-edited 0 cannot disable the check)")
		}

		if overThreshold(0, 0, 100, 0) {
			t.Error("pct 0 clamped to 1 fired below the clamped boundary")
		}

		// pct 500 compares as 100: the full window — 100 fires (inclusive), 99 does not.
		if !overThreshold(100, 0, 100, 500) {
			t.Error("pct 500 must compare as 100 (the inclusive full-window boundary fires)")
		}

		if overThreshold(99, 0, 100, 500) {
			t.Error("pct 500 clamped to 100 fired below the full window")
		}
	})
}

// TestCompaction_SummarizerFailure pins D-09: a summarizer that errors or
// times out produces exactly ONE warning + counter bump, appends NO marker,
// and returns nil — the caller proceeds un-compacted, the turn never fails
// over compaction.
func TestCompaction_SummarizerFailure(t *testing.T) {
	t.Parallel()

	caseDegraded := func(t *testing.T, name string, script []compScript, timeout time.Duration) {
		t.Helper()

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s, m, _, _ := newCompactionTestSession(t, script)
			s.SetCompactionSettings(true, 80, 1000)
			s.lastInputTokens.Store(900) // over the threshold: the check WANTS to compact

			if timeout > 0 {
				s.compactionTimeout = timeout
			}

			err := s.compact(context.Background(), "t-comp")
			if err != nil {
				t.Fatalf("compact error = %v; want nil (D-09: the turn never fails over compaction)", err)
			}

			if got := len(markersOf(t, m)); got != 0 {
				t.Errorf("markers = %d; want 0 (degrade appends no marker)", got)
			}

			if got := s.compactionDegrades.Load(); got != 1 {
				t.Errorf("degrade counter = %d; want exactly 1 (one warning per failure)", got)
			}

			if got := s.compactionNotes.Load(); got != 1 {
				t.Errorf("note counter = %d; want 1 (the compaction STARTED, then degraded)", got)
			}
		})
	}

	caseDegraded(t, "mid-stream error", []compScript{
		{errChunk: errSummarizerExploded},
	}, 0)

	caseDegraded(t, "synchronous stream error", []compScript{
		{callErr: errSummarizerRejected},
	}, 0)

	caseDegraded(t, "timeout", []compScript{{hang: true}}, 40*time.Millisecond)

	caseDegraded(t, "empty summary", []compScript{{finish: stopEndTurn}}, 0)
}

// TestCompaction_Chaining pins D-08 (summary_N consumes summary_N-1 + the
// span since) and the D-10/Pitfall-4 discipline: the summarizer rides the
// same pipeline (profile copy, own usage line) with ZERO client-visible bus
// publishes.
func TestCompaction_Chaining(t *testing.T) { //nolint:gocognit,gocyclo,cyclop,funlen // battery
	t.Parallel()

	t.Run("second compact consumes the previous summary; nothing lost", func(t *testing.T) {
		t.Parallel()

		s, m, cp, _ := newCompactionTestSession(t, []compScript{
			{text: "SUMMARY-ONE", finish: stopEndTurn},
			{text: "SUMMARY-TWO", finish: stopEndTurn},
		})
		s.SetCompactionSettings(true, 80, 1000)

		// First compact over a populated span.
		mustAppend(t, m.AppendUserMessage("t-comp", []ContentBlock{{Type: blockText, Text: "first task"}}),
			"AppendUserMessage")

		err := s.compact(context.Background(), "t-comp")
		if err != nil {
			t.Fatalf("compact(1): %v", err)
		}

		// Second compact over an EMPTY span (idempotency edge): the previous
		// summary is the input prefix; a new marker lands; the old marker and
		// its summary survive (append-only).
		err = s.compact(context.Background(), "t-comp")
		if err != nil {
			t.Fatalf("compact(2): %v", err)
		}

		mk := markersOf(t, m)
		if len(mk) != 2 {
			t.Fatalf("markers = %d; want 2", len(mk))
		}

		if mk[0].Summary != "SUMMARY-ONE" {
			t.Errorf("marker[0].Summary = %q; want SUMMARY-ONE (append-only: never lost)", mk[0].Summary)
		}

		if mk[1].Summary != "SUMMARY-TWO" {
			t.Errorf("marker[1].Summary = %q; want SUMMARY-TWO", mk[1].Summary)
		}

		// The second summarizer call's input: the previous summary rides as
		// the prefix of the rendered prompt (D-08 chaining).
		second := cp.messagesOf(1)
		if len(second) == 0 || !strings.Contains(second[0].Content, "SUMMARY-ONE") {
			t.Errorf("second summarizer input missing the previous summary (D-08):\n%v", second)
		}

		// The prompt is extractive: the instruction survives in both calls.
		first := cp.messagesOf(0)
		if len(first) == 0 || !strings.Contains(strings.ToLower(first[0].Content), "extractive") {
			t.Errorf("summarizer prompt is not extractive (T-19-08):\n%v", first)
		}

		// D-10: each compact appends its OWN usage line attributed to the
		// current turn (visible to /cost and audit).
		lines, err := m.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		usageForTurn := 0

		for i := range lines {
			if lines[i].Type == TypeUsage && lines[i].TurnID == "t-comp" {
				usageForTurn++
			}
		}

		if usageForTurn != 2 {
			t.Errorf("usage lines attributed to t-comp = %d; want 2 (one per compact)", usageForTurn)
		}
	})

	t.Run("profile copy discipline: light tier override never writes back", func(t *testing.T) {
		t.Parallel()

		s, _, cp, _ := newCompactionTestSession(t, []compScript{
			{text: "SUMMARY-LIGHT", finish: stopEndTurn},
		})
		s.Profile.MaxTokens = 8192
		s.SubagentModel = "light-tier-model" // the tiers-table light slug
		s.SetCompactionSettings(true, 80, 1000)

		err := s.compact(context.Background(), "t-comp")
		if err != nil {
			t.Fatalf("compact: %v", err)
		}

		prof := cp.profileOf(0)
		if prof == nil {
			t.Fatal("summarizer was never called")
		}

		if prof.Model != "light-tier-model" {
			t.Errorf("summarizer model = %q; want the light-tier override", prof.Model)
		}

		if prof.MaxTokens > CompactionSummaryMaxTokens {
			t.Errorf("summarizer max_tokens = %d; want <= %d (the 2048 hard cap, T-19-10)",
				prof.MaxTokens, CompactionSummaryMaxTokens)
		}

		if s.Profile.Model != compParentModel {
			t.Errorf("session profile model = %q; want parent-model (never written back)", s.Profile.Model)
		}

		if s.Profile.MaxTokens != 8192 {
			t.Errorf("session profile max_tokens = %d; want 8192 (the copy's cap never writes back)",
				s.Profile.MaxTokens)
		}
	})

	t.Run("absent light binding falls back to the parent model", func(t *testing.T) {
		t.Parallel()

		s, _, cp, _ := newCompactionTestSession(t, []compScript{
			{text: "SUMMARY-PARENT", finish: stopEndTurn},
		})
		s.SetCompactionSettings(true, 80, 1000)

		err := s.compact(context.Background(), "t-comp")
		if err != nil {
			t.Fatalf("compact: %v", err)
		}

		prof := cp.profileOf(0)
		if prof == nil {
			t.Fatal("summarizer was never called")
		}

		if prof.Model != compParentModel {
			t.Errorf("summarizer model = %q; want parent-model (A4 documented default)", prof.Model)
		}
	})

	t.Run("zero client-visible bus publishes during compact", func(t *testing.T) {
		t.Parallel()

		s, _, _, bus := newCompactionTestSession(t, []compScript{
			{text: "SUMMARY-QUIET", finish: stopEndTurn},
		})
		s.SetCompactionSettings(true, 80, 1000)

		// Subscribe to every CLIENT-VISIBLE kind (the four the ACP layer maps
		// to session/update frames). RequestShaped/UsageUpdate are audit-class
		// and exempt (D-10 wants the request attributed).
		chunks := bus.Subscribe("AgentMessageChunk", event.BufAgentMessageChunk)
		thoughts := bus.Subscribe("AgentThoughtChunk", event.BufAgentThoughtChunk)
		tools := bus.Subscribe("ToolCall", event.BufToolCall)
		toolUpds := bus.Subscribe("ToolCallUpdate", event.BufToolCallUpdate)

		defer bus.Unsubscribe("AgentMessageChunk", chunks)
		defer bus.Unsubscribe("AgentThoughtChunk", thoughts)
		defer bus.Unsubscribe("ToolCall", tools)
		defer bus.Unsubscribe("ToolCallUpdate", toolUpds)

		err := s.compact(context.Background(), "t-comp")
		if err != nil {
			t.Fatalf("compact: %v", err)
		}

		// Give any (wrongly published) event a moment to land, then assert
		// every channel stayed empty (Pitfall 4).
		time.Sleep(20 * time.Millisecond)

		for name, ch := range map[string]<-chan event.Event{
			"AgentMessageChunk": chunks, "AgentThoughtChunk": thoughts,
			"ToolCall": tools, "ToolCallUpdate": toolUpds,
		} {
			select {
			case e := <-ch:
				t.Errorf("client-visible %s published during compact (Pitfall 4): %+v", name, e)
			default:
			}
		}
	})
}
