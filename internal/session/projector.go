package session

import (
	"encoding/json"
	"slices"
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
	toolRead: true, toolWrite: true, toolEdit: true,
	"Glob": true, "Grep": true, toolBash: true,
}

// MidTurnWindowMessages bounds the within-turn accumulated window: the MOST
// RECENT tail of the current turn's exchanges, dropping only COMPLETE exchange
// groups (an assistant batch is never separated from its tool results —
// pair-safety). Pinned to the captured zcode tail window: rollout records of
// kind "tail" carry the last 64 messages (messageOffset/messageCount
// bookkeeping, 08-07 capture).
const MidTurnWindowMessages = 64

// compactionFillTargetPct is the D-05 fill target (planner-pinned discretion):
// the summary estimate plus the tail cost stay at or under this percentage of
// the injected context limit, leaving headroom under the 80 percent trigger —
// a tail filled to the trigger would re-fire compaction on the next request.
const compactionFillTargetPct = 60

const (
	// percentDenominator is the percentage scale (mnd: named, not magic).
	percentDenominator = 100
	// estimateCharsPerToken is the chars/4 token-estimate divisor (D-01's
	// no-tokenizer discipline: provider usage is the trigger's ground truth;
	// the estimate only sizes the tail fill).
	estimateCharsPerToken = 4
)

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

	// CompactionTailBudget is the injectable compaction context limit (D-05,
	// PAR-01): the post-marker tail fill targets 60 percent of it (minus the
	// summary estimate — an oversized summary shrinks the tail, never the
	// window, T-19-07). 19-04's session wiring sets it from the
	// modelrouting-resolved context window; zero/unset falls back to the
	// MidTurnWindowMessages count.
	CompactionTailBudget int64

	// retryCompactedTurn is the G-19-1 ENGINE GATE for the same-turn carve-out
	// (19-06): armed by the session's overflow-retry path so the ONE retry
	// projects post-marker instead of re-sending a byte-identical request.
	// A plain field under the caller-holds-turn-serialization discipline — the
	// SetCompactionTailBudget precedent: Project's only production caller is
	// the serialized runTurn loop (session.go), and the subagent runner never
	// calls Project. The zero value "" means NOT armed: the carve-out requires
	// a non-empty, EXACT turnID match, so a crafted or replayed transcript
	// alone never reshapes a live turn's window (tamper safety, kill-9 replay
	// determinism). The override self-expires by construction — keyed by
	// turnID, it stops matching the moment the next turn projects.
	retryCompactedTurn string
}

// NewProjector returns a Projector over the given profile + transcript Manager.
func NewProjector(prof *profile.Profile, m *Manager) *Projector {
	return &Projector{prof: prof, manager: m}
}

// SetRetryCompactedTurn arms the same-turn compaction carve-out (G-19-1,
// 19-06): after it, Project(turnID) accepts a marker whose TurnID EQUALS the
// projected turn — with no pre-user marker present — as that turn's reset
// point. The engine's overflow-retry branch is the ONLY caller (armed between
// the forced compact and the continue, so the retry's re-projection sees it);
// it stays armed for the turn's remaining iterations (the compacted window
// must survive follow-up tool-loop iterations) and self-expires when a
// different turnID projects. Transcript content alone NEVER arms it — the
// carve-out is engine-gated by construction.
func (p *Projector) SetRetryCompactedTurn(turnID string) { p.retryCompactedTurn = turnID }

// SetCompactionTailBudget injects the compaction context limit (D-05) — 19-04
// wires the session's modelrouting-resolved context window here. The zero
// value means "no budget injected" and degrades the tail to the
// MidTurnWindowMessages count (never an empty window, never an unbounded one).
func (p *Projector) SetCompactionTailBudget(limit int64) { p.CompactionTailBudget = limit }

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
//
// Phase 19 (PAR-01) adds the compaction marker as a THIRD reset-point class
// with different seed semantics: when a marker precedes the projected turn's
// user message, the seed is the marker's Summary (D-06 DURABLE — later
// TypeBoundary lines never displace it; only a newer marker replaces it) and
// every transcript WITHOUT a marker takes the pre-phase path byte-identically.
// 19-06 (G-19-1) adds the ENGINE-ARMED same-turn carve-out: when the engine
// armed the per-turn override (SetRetryCompactedTurn) and the pre-user scan
// found nothing, a marker whose TurnID equals the projected turn reshapes
// THAT turn's projection — the overflow retry projects post-marker instead of
// re-sending the rejected request. Without the in-memory override the same
// transcript projects byte-identically to the marker-free one (tamper safety).
func (p *Projector) Project(turnID string) ([]provider.Message, error) {
	lines, err := p.manager.ReadAll()
	if err != nil {
		return nil, err
	}

	if mIdx := compactionMarkerIdx(lines, turnID); mIdx >= 0 {
		return p.projectCompacted(lines, mIdx, turnID), nil
	}

	// The G-19-1 carve-out runs ONLY when the pinned pre-user scan found
	// nothing AND the engine armed this exact turn — a non-empty, exact
	// turnID match (the zero value means not-armed; transcript content alone
	// never reaches this branch).
	if p.retryCompactedTurn != "" && p.retryCompactedTurn == turnID {
		if stIdx := sameTurnMarkerIdx(lines, turnID); stIdx >= 0 {
			return p.projectSameTurnCompacted(lines, stIdx, turnID), nil
		}
	}

	beforeBoundary, afterBoundary := splitAtResetBoundary(lines, turnID)

	summary := p.extractSummary(beforeBoundary)
	currentIntent := findCurrentIntent(afterBoundary, beforeBoundary, turnID)

	mid := boundMidTurn(accumulateMidTurn(lines, turnID))

	intentLine := findIntentLine(afterBoundary, beforeBoundary, turnID)

	out := make([]provider.Message, 0, 1+len(mid))
	out = append(out, seedMessage(seedContent(summary, currentIntent), intentLine))
	out = append(out, mid...)

	return out, nil
}

// seedContent builds the lean seed's text: ONE user message carrying the task
// summary plus the current intent (the Shaper adds the system prompt
// separately from p.prof.System). The wrapper shape is shared by the
// mechanical post-boundary seed and the compaction seed (19-03: reuse the
// shape, source the text from the marker); an empty summary degrades to the
// intent alone (the first-turn form).
func seedContent(summary, currentIntent string) string {
	if summary == "" {
		return currentIntent
	}

	return "Task summary (mechanical, post-boundary):\n" + summary +
		"\n\n--- Current request ---\n" + currentIntent
}

// projectCompacted is the PAR-01 compaction path: the winning marker's Summary
// IS the seed's summary half, in the SAME single-user-message lean-seed shape
// the mechanical post-boundary seed uses (the wrapper is reused verbatim; the
// text is sourced from the marker — the model-generated summary replaces the
// mechanical extractSummary output). D-06 durability: the summary is immune to
// later mutating boundaries — this path runs whenever a marker precedes the
// projected turn's user message, regardless of any TypeBoundary lines after
// it, and the seed is the summary ALONE (research Open Question 3's
// planner-pinned summary-only reading — no mechanical re-derivation over the
// post-marker span).
//
// The TAIL follows existing boundary discipline (D-06: only the summary is
// durable): a TypeBoundary after the marker drops the post-marker tail — the
// window is the seed plus the current turn's own accumulation, exactly the
// pre-phase between-turn shape. With no intervening boundary the marker wins
// the tail scope: the D-04/D-05 budget-fill tail of the most recent
// post-marker messages.
func (p *Projector) projectCompacted(lines []Line, mIdx int, turnID string) []provider.Message {
	marker := lines[mIdx]
	after, before := lines[mIdx+1:], lines[:mIdx]

	var mid []provider.Message

	if boundaryAfterMarker(lines, mIdx, turnID) {
		mid = boundMidTurn(accumulateMidTurn(lines, turnID))
	} else {
		mid = p.boundCompactionTail(foldExchanges(lines, mIdx+1, "", false), marker.Summary)
	}

	currentIntent := findCurrentIntent(after, before, turnID)

	out := make([]provider.Message, 0, 1+len(mid))
	out = append(out, seedMessage(seedContent(marker.Summary, currentIntent), findIntentLine(after, before, turnID)))
	out = append(out, mid...)

	return out
}

// projectSameTurnCompacted is the G-19-1 engine-armed same-turn carve-out
// (19-06): a marker whose TurnID EQUALS the projected turn reshapes that
// turn's own projection, so the overflow retry's request carries the marker's
// summary seed plus the turn's own current intent and a budget-fill
// post-marker tail — measurably smaller than the rejected request. It reuses
// the projectCompacted seed/tail rules verbatim (seedContent wrapper,
// boundCompactionTail budget discipline, foldExchanges across turns) with ONE
// resolution difference: the intent line comes from projectedUserIdx — the
// turn's OWN user message — because the marker sits AFTER it, and a subagent
// prompt landing between the two must not shadow it (the subagent-shadow
// rule). A TypeBoundary branch does not apply here: the projected turn's user
// message precedes the marker by construction, so no boundary can sit between
// them and the budget-fill tail is the only tail shape.
func (p *Projector) projectSameTurnCompacted(lines []Line, mIdx int, turnID string) []provider.Message {
	marker := lines[mIdx]
	after, before := lines[mIdx+1:], lines[:mIdx]

	mid := p.boundCompactionTail(foldExchanges(lines, mIdx+1, "", false), marker.Summary)

	// The subagent-shadow rule: the intent is the projected turn's own user
	// message (projectedUserIdx prefers the TurnID match); findIntentLine is
	// the fallback only when no user message exists at all.
	intentLine := findIntentLine(after, before, turnID)
	if idx := projectedUserIdx(lines, turnID); idx >= 0 {
		intentLine = &lines[idx]
	}

	out := make([]provider.Message, 0, 1+len(mid))
	out = append(out, seedMessage(seedContent(marker.Summary, extractText(intentLine)), intentLine))
	out = append(out, mid...)

	return out
}

// boundaryAfterMarker reports whether a TypeBoundary reset point sits between
// the marker and the projected turn's user message (strictly before the user
// message — a mid-turn boundary of the CURRENT turn never wipes its own
// accumulation, 08-09). Such a boundary drops the post-marker tail (D-06).
func boundaryAfterMarker(lines []Line, mIdx int, turnID string) bool {
	userIdx := projectedUserIdx(lines, turnID)

	for i := mIdx + 1; i < len(lines); i++ {
		if lines[i].Type == TypeBoundary && (userIdx < 0 || i < userIdx) {
			return true
		}
	}

	return false
}

// boundCompactionTail bounds the post-marker tail by the injected budget
// (D-05): summary estimate + tail cost target 60 percent of
// CompactionTailBudget. A zero/unset budget degrades to the existing
// MidTurnWindowMessages count applied to the post-marker tail — never an
// empty window (beyond the group-advance floor), never an unbounded one.
func (p *Projector) boundCompactionTail(tail []provider.Message, summary string) []provider.Message {
	if p.CompactionTailBudget <= 0 {
		return boundMidTurn(tail)
	}

	allowance := p.CompactionTailBudget*compactionFillTargetPct/percentDenominator -
		int64(len(summary))/estimateCharsPerToken
	// An oversized summary shrinks the tail to its floor, never the window (T-19-07).
	allowance = max(allowance, 0)

	return boundCompactionTailByBudget(tail, allowance)
}

// boundCompactionTailByBudget keeps the MOST RECENT tail within the token
// allowance, dropping only COMPLETE exchange groups — the boundMidTurn
// group-advance discipline generalized from a message count to a token budget
// (D-05). Groups accumulate most-recent-first until the next (older) group
// would exceed the allowance; the newest group is always kept (the floor — a
// tail-sized-under-budget error must not produce an empty window).
func boundCompactionTailByBudget(tail []provider.Message, allowance int64) []provider.Message {
	if len(tail) == 0 {
		return nil
	}

	cut := len(tail)

	var total int64

	for i := range slices.Backward(tail) {
		if tail[i].Role == roleToolMsg {
			continue // inside the group of the head before it
		}

		cost := groupCost(tail, i, cut)

		if cut < len(tail) && total+cost > allowance {
			break // the next (older) group would exceed the budget
		}

		total += cost
		cut = i

		if total > allowance {
			break // floor: the newest group alone may exceed the allowance
		}
	}

	// The boundMidTurn advance, kept verbatim as the invariant guard: the cut
	// never leaves an orphaned tool result at the tail's head (a tool result's
	// assistant batch would be missing — pair-safety).
	for cut < len(tail) && tail[cut].Role == roleToolMsg {
		cut++ // advance to the next group head (assistant message)
	}

	if cut >= len(tail) {
		return nil
	}

	return tail[cut:]
}

// groupCost sums the D-05 budget units over tail[from:to].
func groupCost(tail []provider.Message, from, to int) int64 {
	var total int64

	for i := from; i < to; i++ {
		total += messageCost(&tail[i])
	}

	return total
}

// messageCost is the D-05 budget unit: the message's folded JSON size divided
// by 4, truncating (the chars/4 token-estimate class — D-01's no-tokenizer
// discipline; provider usage remains the trigger's ground truth, this only
// sizes the tail fill). Pointer-receiver style: Message is 144 bytes
// (gocritic hugeParam).
func messageCost(m *provider.Message) int64 {
	raw, err := json.Marshal(m)
	if err != nil {
		return int64(len(m.Content)) / estimateCharsPerToken // unreachable: Message always marshals
	}

	return int64(len(raw)) / estimateCharsPerToken
}

// compactionMarkerIdx returns the index of the MOST RECENT TypeCompaction line
// STRICTLY BEFORE the projected turn's user message — the marker resets turns
// that START after it, exactly the TypeBoundary rule (Pitfall 5: a marker
// landing mid-turn is never the producing turn's own reset point — EXCEPT
// under the G-19-1 engine-armed same-turn carve-out, which this pinned scan
// deliberately knows nothing about; see sameTurnMarkerIdx). -1 when no marker
// precedes the turn (the pre-phase path).
func compactionMarkerIdx(lines []Line, turnID string) int {
	userIdx := projectedUserIdx(lines, turnID)

	markerIdx := -1

	for i := range lines {
		if lines[i].Type == TypeCompaction && (userIdx < 0 || i < userIdx) {
			markerIdx = i
		}
	}

	return markerIdx
}

// sameTurnMarkerIdx returns the index of the MOST RECENT TypeCompaction line
// carrying TurnID == turnID — the G-19-1 same-turn marker (19-06). Unlike
// compactionMarkerIdx it is position-blind (the marker sits after the turn's
// user message by definition) and TurnID-keyed: a parent's mid-subagent
// marker never matches a subagent turn (Pitfall 5), and a TurnID-less marker
// never matches anything (the caller reaches here only with a non-empty
// turnID). -1 when no same-turn marker exists — the armed override then
// changes nothing (the degraded-compaction fail-through keeps today's
// behavior).
func sameTurnMarkerIdx(lines []Line, turnID string) int {
	markerIdx := -1

	for i := range lines {
		if lines[i].Type == TypeCompaction && lines[i].TurnID == turnID {
			markerIdx = i
		}
	}

	return markerIdx
}

// projectedUserIdx returns the projected turn's user-message index: the LAST
// user_message carrying TurnID == turnID, falling back to the LAST
// user_message overall (splitAtResetBoundary's rule), or -1 when none exists.
func projectedUserIdx(lines []Line, turnID string) int {
	lastIdx, matchedIdx := -1, -1

	for i := range lines {
		if lines[i].Type == TypeUserMessage {
			lastIdx = i

			if lines[i].TurnID == turnID {
				matchedIdx = i
			}
		}
	}

	if matchedIdx >= 0 {
		return matchedIdx
	}

	return lastIdx
}

// seedMessage builds the lean-seed user Message. 21-05 (PAR-06): when the
// turn's user_message line carries image blocks (the ingress-persisted Ref
// form), the seed renders as ORDERED rich blocks — the seed text first, then
// the image blocks in line order — so the pasted image reaches the outgoing
// request (criterion 5). A line without image blocks seeds exactly the
// pre-21-05 text-only form (Blocks nil — byte-identical shaping).
func seedMessage(content string, intentLine *Line) provider.Message {
	seed := provider.Message{Role: roleUserMsg, Content: content}

	imgs := imageBlocksOf(intentLine)
	if len(imgs) == 0 {
		return seed
	}

	seed.Blocks = append([]provider.Block{{Text: content}}, imgs...)

	return seed
}

// imageBlocksOf extracts a user_message line's image blocks as rich Blocks
// (the Ref + canonical media type only — dims/provenance stay in the
// transcript line; the wire never sees them).
func imageBlocksOf(l *Line) []provider.Block {
	if l == nil || len(l.Content) == 0 {
		return nil
	}

	var blocks []ContentBlock

	if err := json.Unmarshal(l.Content, &blocks); err != nil {
		return nil
	}

	var out []provider.Block

	for i := range blocks {
		b := &blocks[i]
		if b.Type != blockImage || b.DataRef == "" {
			continue
		}

		ref, media := b.DataRef, b.MediaType

		out = append(out, provider.Block{Image: &provider.ImageBlock{Ref: ref, MediaType: media}})
	}

	return out
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
	turnUserIdx := projectedUserIdx(lines, turnID)

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
//
// raw_thinking lines (PAR-05, 21-03) fold INTO their turn's assistant unit —
// NEVER cut separately (Pitfall 5): the block stashes onto the in-progress
// accumulation and flushes INTO the assistant batch (with its tool_use blocks,
// original order), or rides with the turn's final assistant text message. A
// thinking line whose assistant unit never forms (turn truncated before any
// assistant content) is dropped WITH its turn unit — a standalone
// thinking-only assistant message would break the provider's
// thinking/assistant pairing and 400. Field VALUES are extracted here — the
// ONLY extraction site on the replay path (D-14) — and pass through untouched
// to the shaper.
func accumulateMidTurn(lines []Line, turnID string) []provider.Message {
	return foldExchanges(lines, turnAnchorOf(lines, turnID), turnID, true)
}

// foldExchanges is the shared line→message folding (the accumulateMidTurn
// rules, 08-07/08-09 + PAR-05): consecutive TypeToolCall lines become ONE
// assistant message with a ToolCalls batch (the capture's batch form), each
// TypeToolResult becomes a tool-role message whose ToolName is resolved from
// the paired tool_call line, TypeAssistantMessage becomes a plain assistant
// text message, and raw_thinking folds INTO its assistant unit (never cut
// separately, Pitfall 5). A tool_result whose call is not in the window (its
// batch was never accumulated) is dropped — an orphaned tool_result would
// break the provider's tool_use/tool_result pairing invariant.
//
// filterTurn=true folds only the given turn's lines from `from` (the mid-turn
// window — subagent turns have their own); filterTurn=false folds EVERY line
// from `from` regardless of turn (the post-marker compaction tail, D-04/D-05:
// the tail is the most recent post-marker messages across turns, with the
// projected turn's own exchanges as its most recent members).
//
//nolint:funlen // the fold is ONE mechanical state machine, carried verbatim from accumulateMidTurn
func foldExchanges(lines []Line, from int, turnID string, filterTurn bool) []provider.Message {
	// CR-01 pair-safety pre-scan: the call ids that DO have a tool_result in
	// the window. A multi-call batch can be only PARTIALLY answered at resume
	// time (17-02's multi-permission suspension: dialog k answered while
	// dialog k+1 is still open) — projecting the still-pending tool_use would
	// hand the provider an unpaired tool_use block and the request would be
	// rejected. Only ANSWERED calls are projected; a call's tool_use rejoins
	// the batch at the projection after its own resume lands its result.
	hasResult := resultIDsOf(lines, from, turnID, filterTurn)

	var (
		out             []provider.Message
		pending         []provider.ToolCall
		pendingThinking []provider.ThinkingBlock
		names           = map[string]string{} // callID -> tool name (pairing resolution)
	)

	flushBatch := func() bool {
		emitted := false

		if len(pending) > 0 {
			batch := make([]provider.ToolCall, 0, len(pending))
			for _, tc := range pending {
				if hasResult[tc.ID] {
					batch = append(batch, tc)
				}
			}

			// Thinking rides INTO the batch message (the fold); a batch that
			// never forms (no answered calls) takes its stashed thinking with
			// it — the orphan rule.
			if len(batch) > 0 {
				out = append(out, provider.Message{
					Role: roleAssistant, ThinkingBlocks: pendingThinking, ToolCalls: batch,
				})

				emitted = true
			}
		}

		pending = nil
		pendingThinking = nil

		return emitted
	}

	for i := from; i < len(lines); i++ {
		l := &lines[i]
		if filterTurn && l.TurnID != turnID {
			continue // only the CURRENT turn's lines fold (subagent turns have their own)
		}

		switch l.Type {
		case TypeToolCall:
			pending = append(pending, provider.ToolCall{ID: l.ToolCallID, Name: l.Name, Input: l.Input})
			names[l.ToolCallID] = l.Name
		case TypeRawThinking:
			if tb, ok := parseThinkingBlock(l.Content); ok {
				pendingThinking = append(pendingThinking, tb)
			}
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
			// End-of-turn thinking rides WITH the final assistant text
			// message — but never ALSO with a batch flushed ahead of it
			// (21-REVIEW WR-04): an emitted batch CONSUMES the stash (the
			// fold), so capturing the stash before the flush would paste the
			// same signed thinking onto TWO messages — a provider 400 shape.
			// Flush-first discipline: the stash survives the flush only when
			// no batch took it.
			th := pendingThinking
			if flushBatch() {
				th = nil // the emitted batch took the stash with it
			}

			out = append(out, provider.Message{
				Role: roleAssistant, Content: l.Text, ThinkingBlocks: th,
			})
		}
	}

	flushBatch() // leftover thinking-only stashes drop here (the orphan rule)

	return out
}

// parseThinkingBlock extracts the thinking-block FIELD VALUES from one
// raw_thinking payload — the ONLY extraction site on the replay path (PAR-05,
// D-14: values pass through untouched to the shaper; the SDK re-serializes).
// ok is false for absent or unknown-shape payloads (tolerant — never panics,
// never drops the surrounding turn).
func parseThinkingBlock(raw json.RawMessage) (provider.ThinkingBlock, bool) {
	if len(raw) == 0 {
		return provider.ThinkingBlock{}, false
	}

	var v struct {
		Type      string `json:"type"`
		Thinking  string `json:"thinking"`
		Signature string `json:"signature"`
		Data      string `json:"data"`
	}

	if err := json.Unmarshal(raw, &v); err != nil {
		return provider.ThinkingBlock{}, false
	}

	switch v.Type {
	case chunkTypeThinking:
		return provider.ThinkingBlock{Type: v.Type, Text: v.Thinking, Signature: v.Signature}, true
	case blockRedactedThinking:
		return provider.ThinkingBlock{Type: v.Type, Data: v.Data}, true
	default:
		return provider.ThinkingBlock{}, false
	}
}

// turnAnchorOf returns the accumulate anchor: just past the current turn's
// user_message (between-turn reset, SESS-04 revised 08-09) — boundary lines
// are audit markers + the NEXT turn's reset point, never the producing turn's
// wipe.
func turnAnchorOf(lines []Line, turnID string) int {
	anchor := 0

	for i := range lines {
		if lines[i].Type == TypeUserMessage && lines[i].TurnID == turnID {
			anchor = i + 1
		}
	}

	return anchor
}

// resultIDsOf returns the set of call ids carrying a tool_result line in the
// folded window from `from` (the CR-01 projection pair-safety set).
// filterTurn=true restricts the set to the given turn's results (the mid-turn
// window); false takes every result from `from` (the post-marker tail).
func resultIDsOf(lines []Line, from int, turnID string, filterTurn bool) map[string]bool {
	hasResult := make(map[string]bool)

	for i := from; i < len(lines); i++ {
		if (!filterTurn || lines[i].TurnID == turnID) && lines[i].Type == TypeToolResult {
			hasResult[lines[i].ToolCallID] = true
		}
	}

	return hasResult
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
func findCurrentIntent(after, before []Line, turnID string) string {
	return extractText(findIntentLine(after, before, turnID))
}

// findIntentLine is findCurrentIntent's line-returning core (21-05, PAR-06):
// the seed's rich-block fold needs the user_message LINE (its image blocks),
// not just the concatenated text. Returns nil when no user_message exists.
func findIntentLine(after, before []Line, turnID string) *Line {
	// Prefer the user message matching turnID; fall back to the last user_message.
	for i := len(after) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
		v := &after[i]
		if v.Type == TypeUserMessage && (turnID == "" || v.TurnID == turnID) {
			return v
		}
	}

	for i := len(after) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
		v := &after[i]
		if v.Type == TypeUserMessage {
			return v
		}
	}

	// 19-06 (G-19-1): the before-scan gains the same TurnID preference — when
	// the projected turn's user message sits BEFORE the split point (the
	// same-turn carve-out's shape), a later subagent prompt in `before` must
	// not shadow it. Inert for every pre-19-06 fixture: their projected user
	// message sits in `after`, where the preference already exists.
	if turnID != "" {
		for i := len(before) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
			v := &before[i]
			if v.Type == TypeUserMessage && v.TurnID == turnID {
				return v
			}
		}
	}

	for i := len(before) - 1; i >= 0; i-- { //nolint:modernize // conflicts with gocritic rangeValCopy
		v := &before[i]
		if v.Type == TypeUserMessage {
			return v
		}
	}

	return nil
}

// extractText returns the plain text from a user_message content block slice
// (or the line's Text shorthand). nil-safe (findIntentLine's no-line case).
func extractText(l *Line) string {
	if l == nil {
		return ""
	}

	if len(l.Content) > 0 {
		var blocks []ContentBlock

		err := json.Unmarshal(l.Content, &blocks)
		if err == nil {
			var sb strings.Builder

			for i := range blocks {
				if blocks[i].Text != "" {
					sb.WriteString(blocks[i].Text)
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
