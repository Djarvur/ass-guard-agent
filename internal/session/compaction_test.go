package session //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/audit"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// --- The 19-04 scripted provider (the battery's fake — fakeProvider cannot
// emit usage chunks or mid-stream errors, both load-bearing here). ---

// compScript is one scripted Stream call: the chunks the fake delivers, in
// order (usage → tool_use OR text → done), or a failure (callErr fails the
// synchronous Stream; errChunk aborts mid-stream; hang blocks until ctx dies —
// the timeout leg).
type compScript struct {
	text     string
	usage    *provider.Usage
	finish   string
	errChunk error
	callErr  error
	hang     bool
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
}

//nolint:cyclop // one scripted scenario dispatcher
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
		bus.Publish(event.RequestShaped{
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
	prof.Model = "parent-model"

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

	return s, m, cp, bus
}

// countType returns how many transcript lines carry the given type.
func countType(t *testing.T, m *Manager, typ string) int {
	t.Helper()

	lines, err := m.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	n := 0
	for i := range lines {
		if lines[i].Type == typ {
			n++
		}
	}

	return n
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

// --- Task 1 battery: threshold math, D-09 degrade, chaining, bus isolation ---

// TestCompaction_Threshold pins D-01's trigger math: the threshold fires at
// EXACTLY the boundary (inclusive comparison), one unit below does not, the
// added-since estimate is transcript content appended after the last
// request_shaped line divided by 4 truncating — never the request's own total
// body size (Pitfall 7's double-count).
func TestCompaction_Threshold(t *testing.T) {
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

// TestCompaction_SummarizerFailure pins D-09: a summarizer that errors or
// times out produces exactly ONE warning + counter bump, appends NO marker,
// and returns nil — the caller proceeds un-compacted, the turn never fails
// over compaction.
func TestCompaction_SummarizerFailure(t *testing.T) {
	t.Parallel()

	caseDegraded := func(t *testing.T, name string, script []compScript, timeout time.Duration) {
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
		{errChunk: errors.New("summarizer exploded mid-stream")},
	}, 0)

	caseDegraded(t, "synchronous stream error", []compScript{
		{callErr: errors.New("summarizer rejected")},
	}, 0)

	caseDegraded(t, "timeout", []compScript{{hang: true}}, 40*time.Millisecond)

	caseDegraded(t, "empty summary", []compScript{{finish: stopEndTurn}}, 0)
}

// TestCompaction_Chaining pins D-08 (summary_N consumes summary_N-1 + the
// span since) and the D-10/Pitfall-4 discipline: the summarizer rides the
// same pipeline (profile copy, own usage line) with ZERO client-visible bus
// publishes.
func TestCompaction_Chaining(t *testing.T) {
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

		if err := s.compact(context.Background(), "t-comp"); err != nil {
			t.Fatalf("compact(1): %v", err)
		}

		// Second compact over an EMPTY span (idempotency edge): the previous
		// summary is the input prefix; a new marker lands; the old marker and
		// its summary survive (append-only).
		if err := s.compact(context.Background(), "t-comp"); err != nil {
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

		if err := s.compact(context.Background(), "t-comp"); err != nil {
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

		if s.Profile.Model != "parent-model" {
			t.Errorf("session profile model = %q; want parent-model (never written back)", s.Profile.Model)
		}

		if s.Profile.MaxTokens != 8192 {
			t.Errorf("session profile max_tokens = %d; want 8192 (the copy's cap never writes back)", s.Profile.MaxTokens)
		}
	})

	t.Run("absent light binding falls back to the parent model", func(t *testing.T) {
		t.Parallel()

		s, _, cp, _ := newCompactionTestSession(t, []compScript{
			{text: "SUMMARY-PARENT", finish: stopEndTurn},
		})
		s.SetCompactionSettings(true, 80, 1000)

		if err := s.compact(context.Background(), "t-comp"); err != nil {
			t.Fatalf("compact: %v", err)
		}

		prof := cp.profileOf(0)
		if prof == nil {
			t.Fatal("summarizer was never called")
		}

		if prof.Model != "parent-model" {
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

		if err := s.compact(context.Background(), "t-comp"); err != nil {
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
