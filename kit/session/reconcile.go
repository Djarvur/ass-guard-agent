package session

import (
	"encoding/json"
	"slices"
)

// Reconciliation engine (18-02, ACP-06 / 18-CONTEXT D-01/D-02): a PURE
// transcript pass that classifies every dangling expectation a kill -9
// mid-turn can leave on disk (the 18-RESEARCH Live-State Inventory, rows
// 1-9), produces the provenance-marked synthetic closure for each, and
// derives the state seeds (turn counter, plan mode). The engine reads the
// transcript ALONE: no I/O, no session-state sidecar (D-01 —
// transcript-as-truth); 18-05's load path appends the closures through
// Manager.AppendSynthetic and seeds the session via SeedResume, NEVER
// rewriting existing lines (append-only discipline).
//
// Classification is total by construction: line kinds outside the inventory
// are state/bookkeeping kinds that cannot dangle (and readers skip unknown
// kinds per 16-D-20, so an unknown kind is never classified as dangling). If
// a future line kind introduces a new pair-or-state semantic, its row must be
// added here — reconcile_test.go's table is the checklist (18-02 flagged
// assumption).

// InterruptedCause is the D-02 provenance marker every synthetic closure
// carries in Line.Cause: replay renders it subtly, the on-disk transcript
// keeps it unambiguous — post-mortem of kill -9 cases stays possible (D-20
// audit spirit).
const InterruptedCause = "interrupted"

// interruptedText is the engine-authored payload family (T-18-05): closure
// payloads are constants, NEVER copied from transcript content — a crafted
// transcript cannot inject text into a closure.
const interruptedText = "interrupted by process death"

// Seed is the reconciled live-state seed derived from the transcript alone:
// the turn counter continues from the max %03d turn suffix (rows 7 / Pitfall 2
// — replayed and new frames must never collide) and plan mode restores from
// the LAST plan_mode line's persisted target (row 6 / Pitfall 3 — never the
// fresh default). 18-05 feeds MaxTurns to Session.SeedResume and PlanMode to
// the session's plan-mode state.
type Seed struct {
	MaxTurns        int64
	PlanMode        string
	PlanModePresent bool
}

// interruptedToolResultOutput renders the row-1 closure payload (a failed
// tool_result): the structured {"error": …} fallback family the loud-append
// paths use.
func interruptedToolResultOutput() json.RawMessage {
	out, err := json.Marshal(map[string]string{mapKeyError: interruptedText})
	if err != nil {
		return json.RawMessage(`{"error":"interrupted result render failed"}`)
	}

	return out
}

// cancelledAskOutput renders the row-3 closure payload (an unresolved
// ask_suspended): the cancelled-NORMAL family — permissionCancelledForm's
// live twin, an {"output": …} form that is NOT an error result. The reply or
// the D-01 timer would have appended the real result; process death gets the
// cancelled-normal one, so no parked ask fires post-resume (16-D-19's
// registry synthetic-cancel is the live-side twin).
func cancelledAskOutput() json.RawMessage {
	out, err := json.Marshal(map[string]string{
		"output": "Tool call cancelled: the session was interrupted while the ask was pending. " +
			"Continue without this call's result.",
	})
	if err != nil {
		return json.RawMessage(`{"output":"tool call cancelled (form render failed)"}`)
	}

	return out
}

// reconcileTurn is one TurnID's classification state.
type reconcileTurn struct {
	userOpener   int  // the turn's user_message line (the row-2 opener)
	hasUser      bool // row 2 scope: only user-message-opened turns terminal-close
	terminal     bool // assistant/canceled/error/ask_suspended/subagent_result/session_end seen
	hasAssistant bool // the final assistant_message landed (closes the chunk stream)
	hasChunk     bool
	chunkOpener  int // the stream's first chunk line (the row-5 opener)
}

// reconcileScan is the single ordered pass's accumulated state.
type reconcileScan struct {
	seed          Seed
	turns         map[string]*reconcileTurn
	openToolCalls map[string]int // toolCallID -> tool_call line (row 1)
	openAsks      map[string]int // toolCallID -> ask_suspended line (row 3)
	openDispatch  map[string]int // subagentTurnID -> subagent_dispatch line (row 4)
	sessionEnded  bool
}

// Reconcile classifies the reader-filtered transcript lines and returns the
// synthetic closures (in transcript order of their dangling opener, the
// session_end closure always last) plus the state seed. Pure function: no
// file or network I/O, no clock-dependent classification — closure timestamps
// are load-time (the only non-derivable field). Pair matching is KEYED —
// tool_result by ToolCallID, subagent_result by SubagentTurnID, ask resolution
// by the suspended ask's toolCallID — never by line adjacency, so crafted
// collisions resolve deterministically (T-18-04).
func Reconcile(sessionID string, lines []Line) ([]Line, Seed) {
	sc := scanTranscript(sessionID, lines)

	return sc.closures(lines), sc.seed
}

// scanTranscript walks the transcript once, collecting open pairs, per-turn
// terminal presence, open chunk streams, the last plan_mode cause, and the
// max turn suffix.
func scanTranscript(sessionID string, lines []Line) reconcileScan {
	sc := reconcileScan{
		turns:         make(map[string]*reconcileTurn),
		openToolCalls: make(map[string]int),
		openAsks:      make(map[string]int),
		openDispatch:  make(map[string]int),
	}

	// 18-05 scanner unification: the seed's max-turn scan delegates to
	// seed.go's MaxTurnCounter — exactly ONE suffix-scanner implementation
	// survives (the 18-02 local duplication existed for same-wave
	// independence; both plans have now landed).
	sc.seed.MaxTurns = MaxTurnCounter(sessionID, lines)

	for i := range lines {
		l := &lines[i]

		// Row 6 (state seed): the LAST plan_mode line wins (Pitfall 3).
		if l.Type == TypePlanMode {
			sc.seed.PlanMode = l.Cause
			sc.seed.PlanModePresent = true
		}

		st := sc.turnOf(l.TurnID)

		switch l.Type {
		case TypeUserMessage, TypeAssistantMessage, TypeAgentMessageChunk:
			sc.trackTurn(l, st, i)
		case TypeCanceled, TypeError:
			if st != nil {
				st.terminal = true
			}
		case TypeSessionEnd:
			sc.sessionEnded = true

			if st != nil {
				st.terminal = true // defensive: a turn-tagged session_end closes it
			}
		default:
			sc.trackPair(l, st, i)
		}
	}

	return sc
}

// trackTurn books the turn-shape lines: the user_message opener (row 2
// scope), the assistant terminal that closes the chunk stream, and the
// stream's first chunk (the row 5 opener).
func (sc *reconcileScan) trackTurn(l *Line, st *reconcileTurn, i int) {
	if st == nil {
		return
	}

	switch l.Type {
	case TypeUserMessage:
		if !st.hasUser {
			st.hasUser = true
			st.userOpener = i
		}
	case TypeAssistantMessage:
		st.terminal = true
		st.hasAssistant = true
	case TypeAgentMessageChunk:
		if !st.hasChunk {
			st.hasChunk = true
			st.chunkOpener = i
		}
	default:
		// unreachable — the caller routes only the three turn-shape kinds
	}
}

// trackPair books the pair-kind lines (rows 1/3/4): tool_call → tool_result
// and subagent_dispatch → subagent_result by key, ask_suspended claiming its
// callID from row-1 into row-3 scope. State/bookkeeping kinds (boundary,
// usage, request_shaped, engine_decision, command_provenance, raw_thinking,
// local_command, compaction, unknown future kinds) fall through untouched —
// they cannot dangle: rows 6/7/8 seed state or are audit-only pairs and
// append NOTHING (usage counters re-derive from complete usage lines).
func (sc *reconcileScan) trackPair(l *Line, st *reconcileTurn, i int) {
	switch l.Type {
	case TypeToolCall:
		sc.openToolCalls[l.ToolCallID] = i
	case TypeAskSuspended:
		// Row 3 owns this call's closure (cancelled-normal): the callID
		// leaves row-1 scope the moment the suspension is on disk. The
		// turn ENDED at the ask (12-01) — ask_suspended is a terminal.
		delete(sc.openToolCalls, l.ToolCallID)
		sc.openAsks[l.ToolCallID] = i

		if st != nil {
			st.terminal = true
		}
	case TypeToolResult:
		delete(sc.openToolCalls, l.ToolCallID)
		delete(sc.openAsks, l.ToolCallID)
	case TypeSubagentDispatch:
		sc.openDispatch[l.SubagentTurnID] = i
	case TypeSubagentResult:
		delete(sc.openDispatch, l.SubagentTurnID)

		if st != nil {
			st.terminal = true // the result is the subagent turn's terminal
		}
	default:
		// cannot dangle — see the doc comment
	}
}

// turnOf returns (creating on first sight) the turn's classification state,
// or nil for the empty TurnID bookkeeping lines carry (session_start/
// session_end).
func (sc *reconcileScan) turnOf(turnID string) *reconcileTurn {
	if turnID == "" {
		return nil
	}

	st, ok := sc.turns[turnID]
	if !ok {
		st = &reconcileTurn{}
		sc.turns[turnID] = st
	}

	return st
}

// reconcileClosure pairs a closure with its dangling opener's transcript
// index (the deterministic emission order).
type reconcileClosure struct {
	opener int
	line   Line
}

// closures emits one provenance-marked synthetic closure per dangling
// expectation, ordered by the transcript index of the expectation's opener
// (the session_end closure has no opener — the session-level close is always
// last).
func (sc *reconcileScan) closures(lines []Line) []Line {
	var out []reconcileClosure

	// Row 1: dangling tool_call -> failed tool_result (isError=true).
	for callID, idx := range sc.openToolCalls {
		out = append(out, reconcileClosure{idx, Line{
			Type: TypeToolResult, TurnID: lines[idx].TurnID, Timestamp: now(),
			ToolCallID: callID, Output: interruptedToolResultOutput(),
			IsError: true, Cause: InterruptedCause,
		}})
	}

	// Row 3 (and class 10): unresolved ask_suspended -> cancelled-NORMAL
	// tool_result for its toolCallID (isError=false).
	for callID, idx := range sc.openAsks {
		out = append(out, reconcileClosure{idx, Line{
			Type: TypeToolResult, TurnID: lines[idx].TurnID, Timestamp: now(),
			ToolCallID: callID, Output: cancelledAskOutput(),
			Cause: InterruptedCause,
		}})
	}

	// Row 4: dangling subagent_dispatch -> failed subagent_result keyed by
	// subagentTurnID, its parentTurnID carried from the dispatch line.
	for subID, idx := range sc.openDispatch {
		out = append(out, reconcileClosure{idx, Line{
			Type: TypeSubagentResult, TurnID: subID, Timestamp: now(),
			ParentTurnID: lines[idx].ParentTurnID, SubagentTurnID: subID,
			Message: interruptedText, Cause: InterruptedCause,
		}})
	}

	for id, st := range sc.turns {
		// Row 2: user-message-opened turn without a terminal -> synthetic
		// canceled terminal (the turn is DEAD; no engine continuation).
		if st.hasUser && !st.terminal {
			out = append(out, reconcileClosure{st.userOpener, Line{
				Type: TypeCanceled, TurnID: id, Timestamp: now(),
				Text: interruptedText, Cause: InterruptedCause,
			}})
		}

		// Row 5: open agent_message_chunk stream -> terminal assistant_message
		// (cosmetic on replay; the Projector's folded window is unaffected).
		// A terminal closes the stream too — a live-cancelled turn's chunks
		// legitimately never get an assistant_message (WR-04).
		if st.hasChunk && !st.hasAssistant && !st.terminal {
			out = append(out, reconcileClosure{st.chunkOpener, Line{
				Type: TypeAssistantMessage, TurnID: id, Timestamp: now(),
				Text: interruptedText, Cause: InterruptedCause,
			}})
		}
	}

	// Row 9: missing session_end -> synthetic session_end (the resume's
	// transcript continuity; appended, never a rewrite).
	if !sc.sessionEnded {
		out = append(out, reconcileClosure{len(lines), Line{
			Type: TypeSessionEnd, Timestamp: now(), Cause: InterruptedCause,
		}})
	}

	slices.SortStableFunc(out, func(a, b reconcileClosure) int {
		return a.opener - b.opener
	})

	res := make([]Line, 0, len(out))
	for i := range out {
		res = append(res, out[i].line)
	}

	return res
}

// (18-05) The turn-suffix scan lives ONCE in seed.go: MaxTurnCounter. The
// 18-02-era local copy (reconcileMaxTurn/reconcileIsTurnSuffix) was deleted
// when 18-05 wired both plans into one load path — Reconcile's seed routes
// through MaxTurnCounter so a future format change has exactly one site.
