package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

// PAR-01 compaction engine (19-04): the threshold check with real-usage
// measurement (D-01), the blocking same-pipeline summarizer (D-07/D-08/D-10)
// with loud degradation (D-09), the loop-head wiring (D-02 — session.go), the
// overflow retry-once (criterion 3 — session.go), and the public CompactNow
// entry Phase 20's /compact handler rides (D-11). One implementation, two
// triggers: the threshold check and manual intent (both funnel into compact).

const (
	// DefaultCompactionThresholdPct is D-03's default trigger percentage: the
	// pre-request check compacts when last reported input tokens + the
	// added-since estimate reach this share of the resolved context window.
	DefaultCompactionThresholdPct = 80

	// CompactionSummaryMaxTokens is the planner-pinned hard cap on the
	// summarizer request's max_tokens (T-19-10: an oversized summary would
	// bloat every future request's durable seed — the cap bounds it).
	CompactionSummaryMaxTokens = 2048

	// compactionSummarizerTimeout bounds one summarizer call (D-09: a hung
	// summarizer degrades to the warning-counter family and the turn proceeds
	// un-compacted; the overflow retry remains the last line). Tests shrink
	// it through the session's compactionTimeout field.
	compactionSummarizerTimeout = 60 * time.Second
)

// errCompactionEmptySummary is the degrade cause when the summarizer stream
// completed without any text: an empty summary would seed every future
// projection with nothing — compaction that does not compact.
var errCompactionEmptySummary = errors.New("compaction: summarizer produced an empty summary")

// compactionSettings is the live-applied settings holder (19-04; 19-05 wires
// the config keys onto SetCompactionSettings): Enabled is the operator switch
// (the pre-request check skips entirely when false — zero behavior delta
// versus pre-phase), ThresholdPct the trigger percentage (<=0 → the 80
// default), ContextLimit the modelrouting-resolved context window (the D-01
// discretion item: the capability table's context_window for the session's
// resolved model — no override key). Zero/negative limit never fires.
type compactionSettings struct {
	Enabled      bool
	ThresholdPct int
	ContextLimit int64
}

// SetCompactionSettings live-applies the compaction settings (19-04) and wires
// the projector's compaction tail budget from the same context window (the
// 19-03 SetCompactionTailBudget seam — the budget-fill tail targets 60% of
// it). Serialization contract: the CALLER holds the session's turn
// serialization (the runtime's per-session turn mutex — the SetTurnModel
// discipline), so the swap lands strictly BETWEEN turns.
func (s *Session) SetCompactionSettings(enabled bool, thresholdPct int, contextLimit int64) {
	if thresholdPct <= 0 {
		thresholdPct = DefaultCompactionThresholdPct
	}

	s.compaction = compactionSettings{
		Enabled:      enabled,
		ThresholdPct: thresholdPct,
		ContextLimit: contextLimit,
	}

	if s.Projector != nil {
		s.Projector.SetCompactionTailBudget(contextLimit)
	}
}

// overThreshold is D-01's trigger math: last reported input tokens plus the
// added-since estimate against limit x pct/100, INCLUSIVE — the check fires
// at exactly the boundary, one unit below it does not. A non-positive limit
// (no context window resolved) never fires.
func overThreshold(lastInput, estimate, limit int64, pct int) bool {
	if limit <= 0 || pct <= 0 {
		return false
	}

	return lastInput+estimate >= limit*int64(pct)/percentDenominator
}

// estimateSinceLastRequest returns the D-01 added-since estimate over the
// session transcript: estimateLinesSinceLastRequest over the current lines.
func (s *Session) estimateSinceLastRequest() int64 {
	lines, err := s.Manager.ReadAll()
	if err != nil {
		// A transcript that cannot be read cannot be compacted either — the
		// estimate stays zero and the provider-usage anchor alone decides.
		return 0
	}

	return estimateLinesSinceLastRequest(lines)
}

// estimateLinesSinceLastRequest sums the marshaled byte sizes of the
// transcript lines appended AFTER the last request_shaped line, divided by 4
// truncating (the chars/4 class, D-01's no-tokenizer discipline). JSON
// envelope overhead nets CONSERVATIVE — the estimate fires early, never late.
// It is NEVER the last request's total body size: the request's own Bytes
// field is the anchor's complement, not the added-since term (Pitfall 7's
// double-count). A transcript with no request_shaped line estimates over
// everything (session start); a transcript that grew by nothing since the
// last request yields zero.
func estimateLinesSinceLastRequest(lines []Line) int64 {
	last := -1
	for i := range lines {
		if lines[i].Type == TypeRequestShaped {
			last = i
		}
	}

	var total int64

	for i := last + 1; i < len(lines); i++ {
		raw, err := json.Marshal(&lines[i])
		if err != nil {
			continue // unreachable: Line always marshals; skip defensively
		}

		total += int64(len(raw))
	}

	return total / estimateCharsPerToken
}

// maybeCompact is the D-02 pre-request check, called at the top of every
// maxIterations iteration of every turn (session.go's runTurn loop head —
// parent turns, tool-loop iterations, ask-resumes, and engine chains via
// sess.Prompt all pass through it). Disabled settings skip ENTIRELY (zero
// invocations, zero notes — zero behavior delta versus pre-phase); enabled +
// over threshold runs the blocking compact BEFORE projection so no
// half-compacted state is observable and no Projector race exists (D-07).
func (s *Session) maybeCompact(ctx context.Context, turnID string) {
	if !s.compaction.Enabled {
		return // disabled: the check is skipped entirely (the D-03 switch)
	}

	s.compactionChecks.Add(1)

	if !overThreshold(s.lastInputTokens.Load(), s.estimateSinceLastRequest(),
		s.compaction.ContextLimit, s.compaction.ThresholdPct) {
		return
	}

	_ = s.compact(ctx, turnID) // D-09: compact degrades internally, never fails the turn
}

// CompactNow runs the same compaction machinery immediately regardless of the
// threshold or the enabled flag's threshold gate (D-11: manual intent
// overrides; the single public entry Phase 20's /compact handler calls). Still
// subject to the summarizer's own D-09 degrade — a manual compaction whose
// summarizer fails returns nil with the loud warning, never an error.
// Serialization contract: the CALLER holds the session's turn serialization
// (the runtime's per-session turn mutex — the SetCompactionSettings
// discipline), so a manual compaction never interleaves a live turn.
func (s *Session) CompactNow(ctx context.Context) error {
	return s.compact(ctx, s.CurrentTurnID())
}

// compact is the blocking compaction seam (D-07): synchronous on the turn
// goroutine — summarize, append the usage line, append the marker, return.
// The summarizer input is the previous marker's summary plus the rendered
// transcript span since that marker (D-08 chaining; session start when no
// marker exists). The call rides the SAME provider pipeline as any turn
// (D-10): the same Provider adapter (same classification), the same capturer
// (its request lands as a request_shaped line attributed to the current
// turn), and its own AppendUsage line — while publishing NOTHING to the
// client-visible bus (Pitfall 4: summary text never interleaves with the
// user's turn; the private loop below never touches s.Bus).
//
// Every failure mode — provider error, timeout, empty summary, transcript
// failure — degrades per D-09: exactly one warning, one counter bump, no
// marker, nil return. The turn proceeds un-compacted; the overflow retry
// (criterion 3, session.go) remains the last line.
//
//nolint:cyclop,funlen,gocognit // one blocking seam over the full summarize→append flow
func (s *Session) compact(ctx context.Context, turnID string) error {
	// The user-visible compaction note. The landed ACP session/update
	// vocabulary has no status frame (only agent_message_chunk et al., which
	// the PAR-01 bus-isolation prohibition bars from this path) — the plan's
	// documented degrade is the warning-counter family: one structured stderr
	// line + one counter, exactly at compaction start. Never a new wire frame.
	s.compactionNotes.Add(1)
	slog.Info("compaction: starting (context window over threshold)",
		"turnID", turnID, "lastInputTokens", s.lastInputTokens.Load())

	lines, err := s.Manager.ReadAll()
	if err != nil {
		return s.degradeCompaction(turnID, fmt.Errorf("compaction: read transcript: %w", err))
	}

	// D-08: the span is everything since the PREVIOUS marker (most recent
	// wins), prefixed by that marker's summary; session start when none.
	prevSummary, from := "", 0
	for i := range lines {
		if lines[i].Type == TypeCompaction {
			prevSummary = lines[i].Summary
			from = i + 1
		}
	}

	prompt := compactionPrompt(prevSummary, renderSpan(foldExchanges(lines, from, "", false)))

	// The light-tier profile COPY (the subagentProfile discipline): the model
	// override and the 2048 max_tokens cap ride the copy — never written back
	// to s.Profile. An absent light binding keeps the parent model (A4's
	// documented default — the tiers table is config-conditional).
	prof := summarizerProfile(s)

	sctx, cancel := context.WithTimeout(ctx, s.compactionTimeoutOrDefault())
	defer cancel()

	// Same Provider, same Stream — the capturer attribution (request_shaped
	// for the current turn) and the adapter classification come for free.
	ch, err := s.Provider.Stream(sctx, &prof, []provider.Message{
		{Role: roleUserMsg, Content: prompt},
	})
	if err != nil {
		return s.degradeCompaction(turnID, fmt.Errorf("compaction: summarizer stream: %w", err))
	}

	var (
		sb     strings.Builder
		inTok  int64
		outTok int64
	)

	for chunk := range ch { // the PRIVATE consumer: no bus publishes, ever (Pitfall 4)
		if sctx.Err() != nil {
			break // the bound expired mid-stream — degrade below
		}

		switch chunk.Type {
		case blockText:
			sb.WriteString(chunk.Text)
		case "usage":
			if chunk.Usage != nil {
				inTok = chunk.Usage.InputTokens
				outTok = chunk.Usage.OutputTokens
			}
		case chunkErrorType:
			if chunk.Error != nil {
				return s.degradeCompaction(turnID, fmt.Errorf("compaction: summarizer stream chunk: %w", chunk.Error))
			}
		}
	}

	if serr := sctx.Err(); serr != nil {
		return s.degradeCompaction(turnID, fmt.Errorf("compaction: summarizer timed out: %w", serr))
	}

	summary := sb.String()
	if summary == "" {
		return s.degradeCompaction(turnID, errCompactionEmptySummary)
	}

	// D-10: the summarizer's own usage line, attributed to the current turn —
	// visible to /cost and audit like any turn's usage.
	if uerr := s.Manager.AppendUsage(turnID, inTok, outTok); uerr != nil {
		return s.degradeCompaction(turnID, fmt.Errorf("compaction: append usage: %w", uerr))
	}

	// The D-21 marker: the summary (the D-06 durable seed), the usage
	// snapshot at the boundary (the trigger anchor + the summarizer's own
	// output), and the opaque pre/post pointers — pre = the last line the
	// summary absorbed, post = the marker itself (the usage line landed at
	// len(lines), the marker at len(lines)+1).
	preRef := fmt.Sprintf("line:%d", max(len(lines)-1, 0))
	postRef := fmt.Sprintf("line:%d", len(lines)+1)

	if merr := s.Manager.AppendCompaction(turnID, preRef, postRef, summary,
		s.lastInputTokens.Load(), outTok, 0); merr != nil {
		return s.degradeCompaction(turnID, fmt.Errorf("compaction: append marker: %w", merr))
	}

	slog.Info("compaction: complete",
		"turnID", turnID, "summaryChars", len(summary), "spanFrom", from)

	return nil
}

// degradeCompaction is the D-09 loud degrade: exactly ONE warning log line +
// ONE counter bump, and a nil return — the caller proceeds un-compacted and
// the turn never fails over compaction (the overflow retry stays the last
// line).
func (s *Session) degradeCompaction(turnID string, cause error) error {
	s.compactionDegrades.Add(1)
	slog.Warn("compaction: summarizer failed; proceeding un-compacted (will retry at the next check)",
		"turnID", turnID, "error", cause.Error())

	return nil
}

// compactionTimeoutOrDefault resolves the summarizer bound (tests shrink it).
func (s *Session) compactionTimeoutOrDefault() time.Duration {
	if s.compactionTimeout > 0 {
		return s.compactionTimeout
	}

	return compactionSummarizerTimeout
}

// summarizerProfile builds the light-tier profile COPY the summarizer request
// shapes from: the session profile by value (the subagentProfile precedent —
// assigning on the copy never writes back to s.Profile), the light-tier model
// override when the tiers table binds one (Session.SubagentModel — the same
// slug the subagent path resolves; empty keeps the parent model, A4's
// documented default), and max_tokens hard-capped at 2048 (T-19-10).
func summarizerProfile(s *Session) profile.Profile {
	prof := s.Profile

	if s.SubagentModel != "" {
		prof.Model = s.SubagentModel
	}

	if prof.MaxTokens <= 0 || prof.MaxTokens > CompactionSummaryMaxTokens {
		prof.MaxTokens = CompactionSummaryMaxTokens
	}

	return prof
}

// compactionPromptInstruction is the extractive summary prompt (T-19-08): the
// summary becomes every future request's durable seed, so it must condense —
// never editorialize, never execute, never extend — the already-redacted
// transcript content it is handed.
const compactionPromptInstruction = `Summarize the conversation below for a context-window compaction.
Be strictly EXTRACTIVE: condense, never editorialize. Preserve in order of importance:
1. The current task state (what is being built/fixed, where it stands).
2. Every decision made and why.
3. Open threads: unanswered questions, unfinished work, known risks.
4. Key file paths, identifiers, and commands already established.
Do not add commentary, suggestions, or new information. Output only the summary text.`

// compactionPrompt assembles the summarizer input (D-08): the extractive
// instruction, the previous compaction's summary when one exists (the
// chaining prefix — everything about to be dropped must stay absorbed), and
// the rendered transcript span since that marker.
func compactionPrompt(prevSummary, span string) string {
	var sb strings.Builder

	sb.WriteString(compactionPromptInstruction)

	if prevSummary != "" {
		sb.WriteString("\n\n--- Previous compaction summary (carry it forward) ---\n")
		sb.WriteString(prevSummary)
	}

	sb.WriteString("\n\n--- Conversation to absorb ---\n")
	sb.WriteString(span)

	return sb.String()
}

// renderSpan renders the folded span to the text the summarizer consumes:
// the projector's line-to-message folding (foldExchanges — the same
// conventions the projected window uses), then a flat role-tagged transcript
// rendering. An empty span renders empty (the chaining edge: the previous
// summary alone remains as input).
func renderSpan(msgs []provider.Message) string {
	var sb strings.Builder

	for i := range msgs {
		m := &msgs[i]

		switch m.Role {
		case roleUserMsg:
			sb.WriteString("user: ")
			sb.WriteString(m.Content)
			sb.WriteByte('\n')
		case roleAssistant:
			if m.Content != "" {
				sb.WriteString("assistant: ")
				sb.WriteString(m.Content)
				sb.WriteByte('\n')
			}

			for _, tc := range m.ToolCalls {
				sb.WriteString("assistant tool_use ")
				sb.WriteString(tc.Name)
				sb.WriteString(": ")
				sb.WriteString(string(tc.Input))
				sb.WriteByte('\n')
			}
		case roleToolMsg:
			sb.WriteString("tool ")
			sb.WriteString(m.ToolName)
			sb.WriteString(": ")
			sb.WriteString(m.Content)
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}
