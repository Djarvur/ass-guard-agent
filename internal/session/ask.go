package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Ask suspension (12-01, ACP-01 + D-01): a model-authored question mid-turn
// SUSPENDS the turn (the executor returns ErrSuspended after parsing; the tool
// loop ends the turn WITHOUT appending a tool result) and the pending ask lives
// in the per-session AskBroker until the operator's reply (routed as the
// pending call's tool result — the reply IS the result, no new user message) or
// the D-01 timeout timer (configurable wait, default 10 minutes; 0 = block
// forever, interactive mode) resolves it with the capture-shaped non-answer
// form and drives the SAME resume path. This is a model-INITIATED question
// surface, NOT a tool-execution gate — the no-confirmation-tier safety model is
// untouched.

// ErrSuspended is the suspension sentinel the AskUserQuestion executor returns
// after parsing the question payload. The session tool loop maps it to the ask
// stop marker: the turn ends WITHOUT appending a tool result for the suspended
// call; the reply (or the D-01 timer) appends the result and resumes the SAME
// turn's model loop.
var ErrSuspended = errors.New(
	"session: ask suspended — turn ends without a tool result; reply or D-01 timeout resumes it",
)

// DefaultAskTimeout is the D-01 policy default: an unanswered AskUserQuestion
// waits 10 minutes, then the turn resumes with the non-answer form. A timeout
// of 0 (block forever — interactive mode) is an explicit operator choice.
const DefaultAskTimeout = 10 * time.Minute

// stopAsk is the Prompt stop marker for an ask-suspended turn. It is INTERNAL:
// the ACP-facing stopReason for a suspended turn maps to a completed turn
// (sessionTurnRunner.Run); the engine maps it to ActionAsk via
// TurnOutput.AskSuspended (never ActionContinue — the chained-stage hazard).
const stopAsk = "ask"

// errNoPendingAsk is returned by ResolveAsk when nothing is pending (the
// caller routes the prompt as an ordinary new turn).
var errNoPendingAsk = errors.New("session: resolve ask: nothing pending")

// AskOption is one selectable option of a question (the captured schema's
// {label, description}; the optional preview field is parsed-and-ignored —
// the surface renderer emits structure, not artifacts).
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AskQuestion is one model-authored question (the captured schema's
// {question, header, options[], multiSelect}).
type AskQuestion struct {
	Question    string      `json:"question"`
	Header      string      `json:"header"`
	Options     []AskOption `json:"options"`
	MultiSelect bool        `json:"multiSelect"` //nolint:tagliatelle // captured input key
}

// PendingAsk is the per-session pending-ask record, keyed by the suspended
// turn + the pending tool call (T-12-01-01: a reply resolves only ITS OWN
// session's pending ask — the broker is per-session; the claim is atomic).
type PendingAsk struct {
	TurnID     string        `json:"turnID"` //nolint:tagliatelle // on-disk form
	CallID     string        `json:"callID"` //nolint:tagliatelle // on-disk form
	Questions  []AskQuestion `json:"questions"`
	SurfacedAt time.Time     `json:"surfacedAt"` //nolint:tagliatelle // on-disk form
	// Kind distinguishes the suspension surface (12-04): an ordinary model
	// question ("") vs a plan approval ("plan_approval" — the ExitPlanMode
	// resume renders the approved/denied forms and flips the plan-mode state).
	Kind string `json:"kind,omitempty"`
	// settle is the per-suspension one-shot completion signal (13-00): armed
	// by Surface, closed by resumeAskClaimed AFTER the resumed runTurn
	// returns. It rides the struct so the winning driver (claim copy) closes
	// exactly ITS suspension's channel — a sequential re-ask inside the
	// resumed turn arms a fresh one. Not serialized.
	settle chan struct{}
}

// PendingAskKindPlanApproval marks an ExitPlanMode approval suspension (the
// empty Kind is the ordinary AskUserQuestion question).
const PendingAskKindPlanApproval = "plan_approval"

// PendingAskKindQuestion is the explicit ordinary-question Kind (the zero
// value; the constant exists for readability at the construction site).
const PendingAskKindQuestion = ""

// AskBroker holds the per-session pending ask + the D-01 timeout timer. It is
// deliberately Await-free: Surface records + notifies; Claim atomically hands
// the pending ask to exactly ONE resume driver (the operator reply and the
// timeout timer race; the loser observes nothing pending and no-ops).
type AskBroker struct {
	mu        sync.Mutex
	pending   *PendingAsk
	onSurface func(PendingAsk)
	onTimeout func(PendingAsk)
	timeout   time.Duration
	timer     *time.Timer
	// settleCh is the most-recently-armed settle signal (13-00). It survives
	// Claim (the resume is in flight precisely then) and is replaced at the
	// next Surface — the accessor hands waiters the CURRENT suspension's
	// channel; before any suspension it is nil (wait-first callers treat nil
	// as "nothing to wait for").
	settleCh chan struct{}
}

// NewAskBroker returns a broker with the D-01 timeout (a NEGATIVE value
// normalizes to DefaultAskTimeout; 0 = block forever — no timer is armed) and
// the optional surface callback (fired once per surfaced ask, before the
// suspending turn's response reaches the client).
func NewAskBroker(timeout time.Duration, onSurface func(PendingAsk)) *AskBroker {
	if timeout < 0 {
		timeout = DefaultAskTimeout
	}

	return &AskBroker{onSurface: onSurface, timeout: timeout}
}

// Timeout returns the configured D-01 wait (0 = block forever).
func (b *AskBroker) Timeout() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.timeout
}

// SetOnTimeout registers the timeout-resume hook (the Session wires its own
// resume path; the broker claims the pending ask before calling it, so the
// hook only runs when the timer legitimately won the race).
func (b *AskBroker) SetOnTimeout(fire func(PendingAsk)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.onTimeout = fire
}

// Pending reports whether a pending ask exists (reply routing consults this
// before starting a new turn).
func (b *AskBroker) Pending() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.pending != nil
}

// Snapshot returns a copy of the pending ask, if any.
func (b *AskBroker) Snapshot() (PendingAsk, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.pending == nil {
		return PendingAsk{}, false
	}

	return *b.pending, true
}

// Surface records the pending ask, fires the surface callback, and arms the
// D-01 timer (when timeout > 0 and an onTimeout hook is registered) whose fire
// claims the ask and drives the non-answer resume. Called by the session tool
// loop — the only place that knows both turnID and callID (the executor's
// parsed questions ride the suspended result's Output; the Stub seam carries
// no call identity).
func (b *AskBroker) Surface(p PendingAsk) { //nolint:gocritic // hugeParam: 80-byte struct passed once per suspension
	b.mu.Lock()

	if p.SurfacedAt.IsZero() {
		p.SurfacedAt = time.Now().UTC()
	}

	// 13-00: every surfaced suspension arms a FRESH one-shot settle signal —
	// the incoming struct's settle value (if any) is ignored, so an externally
	// constructed PendingAsk can never carry a foreign (or already-closed)
	// channel into the waiter seam.
	p.settle = make(chan struct{})
	b.settleCh = p.settle

	b.pending = &p

	fire := b.onTimeout

	timeout := b.timeout

	b.stopTimerLocked()

	if timeout > 0 && fire != nil {
		b.timer = time.AfterFunc(timeout, func() {
			if claimed, ok := b.Claim(); ok {
				fire(claimed)
			}
		})
	}

	b.mu.Unlock()

	if b.onSurface != nil {
		b.onSurface(p)
	}
}

// Claim atomically takes the pending ask (first claimant wins — a reply and
// the timeout timer race, and exactly one drives the resume; T-12-01-01).
// Disarms the timer on success.
func (b *AskBroker) Claim() (PendingAsk, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.pending == nil {
		return PendingAsk{}, false
	}

	p := *b.pending

	b.pending = nil

	b.stopTimerLocked()

	return p, true
}

// ArmTimeout arms an arbitrary timer on the broker's guarded slot (the D-01
// suspension path uses Surface; this is the generic seam plan 12-07's cron
// firing reuses). Any previously armed timer is stopped. The returned disarm
// func stops the timer and is safe to call multiple times.
func (b *AskBroker) ArmTimeout(when time.Duration, fire func()) func() {
	b.mu.Lock()

	b.stopTimerLocked()

	if when > 0 && fire != nil {
		b.timer = time.AfterFunc(when, fire)
	}

	b.mu.Unlock()

	return b.Disarm
}

// Disarm stops any armed timer (session close — no goroutine leak).
func (b *AskBroker) Disarm() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.stopTimerLocked()
}

// SettleChan returns the CURRENT suspension's settle signal (13-00) — the
// channel closed when this suspension's resume completes (whichever driver
// won). Nil before any suspension ever surfaced on this broker; already-closed
// once the resume finished (waiters fall straight through). Replaced at the
// next Surface, so a waiter holding an earlier reference keeps waiting on
// exactly ITS suspension.
func (b *AskBroker) SettleChan() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.settleCh
}

// stopTimerLocked stops + clears the timer (caller holds mu).
func (b *AskBroker) stopTimerLocked() {
	if b.timer != nil {
		b.timer.Stop()

		b.timer = nil
	}
}

// RenderAskAnswered renders the CAPTURED answered form (re-pinned by the
// 12-05 re-record, 2026-08-20, sess_6e4b5cc7, zcode 0.16.3 — 15 observations;
// provenance recorded in internal/coreexec/testdata/zcode-interactive-results.json):
// each question is paired with the operator's reply, embedded VERBATIM, no
// reinterpretation (T-12-01-02). ass-guard's single-text reply channel pairs
// the reply with every surfaced question (the captured pairing renders
// `"question"="answer"` per question, comma-joined).
func RenderAskAnswered(qs []AskQuestion, reply string) string {
	var pairs string
	if len(qs) == 0 {
		pairs = fmt.Sprintf("%q", reply)
	} else {
		quoted := make([]string, 0, len(qs))
		for _, q := range qs {
			quoted = append(quoted, fmt.Sprintf("%q=%q", q.Question, reply))
		}

		pairs = strings.Join(quoted, ", ")
	}

	return fmt.Sprintf(
		"User has answered your questions: %s. You can now continue with the user's answers in mind.", pairs,
	)
}

// RenderAskNonAnswer renders the D-01 corpus-absent non-answer form (no
// unanswered AskUserQuestion exists in any pinned corpus; documented
// corpus-informed default, routed like ACP-07's corpus-absent forms — the
// hands-off core value requires the model to receive SOMETHING and proceed or
// decline). Upgrade pointer: plan 12-05's re-record.
func RenderAskNonAnswer(timeout time.Duration) string {
	return fmt.Sprintf(
		"User has not answered your questions (ask timed out after %s); proceed or decline on your own.", timeout,
	)
}

// SetAskBroker wires the per-session ask broker (the AskUserQuestion
// suspension surface). resumeCtx is the context timer-driven resumes run under
// — the suspending turn's ctx dies with its prompt response, so the D-01 timer
// must resume under the serve-lifetime ctx; nil falls back to
// context.Background().
func (s *Session) SetAskBroker(resumeCtx context.Context, b *AskBroker) {
	s.ask = b
	s.askResumeCtx = resumeCtx

	if b != nil {
		b.SetOnTimeout(func(p PendingAsk) { s.resumeAskClaimed(s.askResumeCtx, p, nil) })
	}
}

// AskBroker returns the wired broker (nil when asks are not wired).
func (s *Session) AskBroker() *AskBroker { return s.ask }

// HasPendingAsk reports whether this session has an unresolved pending ask.
func (s *Session) HasPendingAsk() bool { return s.ask != nil && s.ask.Pending() }

// AskSettleChan exposes the current suspension's settle signal (13-00): the
// one-shot channel closed AFTER the resumed runTurn returns, whichever resume
// driver (operator reply or D-01 timer) drove it. Nil when no broker is wired
// or no suspension was ever surfaced; already-closed once a resume finished.
// The waiter semantics: "wait until THIS suspension's resume completes" — the
// engine's ask-wait consumes it; a cancelled waiter ctx is the waiter's own
// exit (the channel alone never settles without a driver).
func (s *Session) AskSettleChan() <-chan struct{} {
	if s.ask == nil {
		return nil
	}

	return s.ask.SettleChan()
}

// ResolveAsk routes an operator reply into the pending ask: the reply lands as
// the pending call's tool result in the captured answered form, and the
// SUSPENDED turn's model loop resumes (no new user message — the reply IS the
// tool result). Fails with errNoPendingAsk when nothing is pending (the caller
// routes the prompt as an ordinary turn).
func (s *Session) ResolveAsk(ctx context.Context, reply string) (string, error) {
	if s.ask == nil {
		return "", errNoPendingAsk
	}

	p, ok := s.ask.Claim()
	if !ok {
		return "", errNoPendingAsk
	}

	return s.resumeAskClaimed(ctx, p, &reply), nil
}

// resumeAskClaimed is the shared resume path (reply routing + the D-01 timer):
// it appends the rendered tool result for the pending callID and re-enters the
// SAME turn's model loop. A nil reply renders the non-answer form. ctx governs
// the resumed turn (the reply's request ctx, or the serve-lifetime ctx on the
// timer path — the suspending turn's own ctx died with its response).
func (s *Session) resumeAskClaimed( //nolint:contextcheck // the timer path passes the stored serve-lifetime ctx
	ctx context.Context, p PendingAsk, reply *string, //nolint:gocritic // hugeParam: one-shot resume payload
) string {
	if ctx == nil {
		ctx = context.Background()
	}

	var form string

	isErr := false

	switch {
	case p.Kind == PendingAskKindPlanApproval && reply != nil && isApprovalReply(*reply):
		// 12-04: an approving reply renders the approved form (source-informed
		// corpus-absent default), lifts the gate, and records the exit marker.
		form, _ = RenderPlanApprovalResolved(planOf(p.Questions), *reply)

		if s.planMode != nil {
			s.planMode.Exit()
		}

		s.appendPlanModeMarker(planModeCauseExit, p.CallID, p.TurnID)
	case p.Kind == PendingAskKindPlanApproval && reply != nil:
		// A declining reply: the CAPTURED denial (isError); the gate STAYS ON.
		form, isErr = RenderPlanApprovalResolved(planOf(p.Questions), *reply)
	case p.Kind == PendingAskKindPlanApproval:
		// The D-01 timeout: the non-answer; the gate STAYS ON (an unapproved
		// plan never ungates the mutating tools).
		form = RenderAskNonAnswer(s.ask.Timeout())
	case reply != nil:
		form = RenderAskAnswered(p.Questions, *reply)
	default:
		form = RenderAskNonAnswer(s.ask.Timeout())
	}

	s.appendToolResultLoud(p.TurnID, p.CallID, "ask", marshalAskForm(form), isErr)

	stop, _ := s.runTurn(ctx, p.TurnID)

	// 13-00: the settle signal closes AFTER the resumed runTurn returns —
	// turn COMPLETION, not the claim, not the render. Exactly-once is the
	// claim discipline's (only the winning driver reaches here, and each
	// resume closes only its own p.settle — Surface always arms fresh).
	if p.settle != nil {
		close(p.settle)
	}

	return stop
}

// marshalAskForm marshals an ask result form (a plain string — the 08-08
// plainContent seam renders JSON-string outputs unquoted). A string Marshal
// cannot fail; the fallback keeps the transcript line valid regardless.
func marshalAskForm(form string) json.RawMessage {
	out, err := json.Marshal(form)
	if err != nil {
		return json.RawMessage(`"ask form render failed"`)
	}

	return out
}

// planOf extracts the plan text from an approval question (the question wraps
// the plan after its header line — planApprovalQuestion's shape).
func planOf(qs []AskQuestion) string {
	if len(qs) == 0 {
		return ""
	}

	_, plan, found := strings.Cut(qs[0].Question, "\n\n")
	if !found {
		return qs[0].Question
	}

	return plan
}
