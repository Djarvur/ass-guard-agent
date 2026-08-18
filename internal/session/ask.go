package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
}

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

// stopTimerLocked stops + clears the timer (caller holds mu).
func (b *AskBroker) stopTimerLocked() {
	if b.timer != nil {
		b.timer.Stop()

		b.timer = nil
	}
}

// RenderAskAnswered renders the CAPTURED answered form (fixture-pinned,
// internal/coreexec/testdata/zcode-interactive-results.json — provenance: the
// 08-08 harvest note quoting the capture): the operator's reply is embedded
// VERBATIM, no reinterpretation (T-12-01-02).
func RenderAskAnswered(reply string) string {
	return fmt.Sprintf("User has answered your questions: %q", reply)
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

	if reply != nil {
		form = RenderAskAnswered(*reply)
	} else {
		form = RenderAskNonAnswer(s.ask.Timeout())
	}

	_ = s.Manager.AppendToolResult(p.TurnID, p.CallID, marshalAskForm(form), false)

	stop, _ := s.runTurn(ctx, p.TurnID)

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
