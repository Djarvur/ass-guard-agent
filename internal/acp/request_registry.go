package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxSendAttempts is the D-14 ladder bound: every Call sends its frame at most
// twice (the initial send + exactly ONE retry with a fresh window) before
// falling back.
const maxSendAttempts = 2

// ErrRequestCancelled is the sentinel wrapped by every cancelled resolution:
// a -32800 error response, a synthetic ResolveCancelled (D-19), caller-ctx
// cancellation (turn death — the caller's ctx IS the turn ctx), and the
// shutdown drain. Phase 17's ask paths key on errors.Is against this.
var ErrRequestCancelled = errors.New("acp: outbound request cancelled")

// RequestTimeoutError is D-14's terminal fallback outcome: the request timed
// out, was retried once with a fresh window, and still went unanswered. The
// caller must degrade to the plain-text path — the user always gets something,
// the connection never wedges (D-14, mirroring 12-D-01's ask-timeout
// philosophy).
type RequestTimeoutError struct {
	ID      string
	Method  string
	Elapsed time.Duration // total time across BOTH windows
}

// Error implements the error interface (structured, log-ready shape).
func (e *RequestTimeoutError) Error() string {
	return fmt.Sprintf("acp: outbound request %s (%s) unanswered after retry (%d ms) — falling back (D-14)",
		e.Method, e.ID, e.Elapsed.Milliseconds())
}

// Registry log vocabulary: structured key=value lines on the stderr logger
// (transport discipline — diagnostics NEVER touch stdout).
const (
	registryFallbackLogFormat = "outbound request fallback: id=%s method=%s elapsed_ms=%d attempts=2; " +
		"degrading to the plain-text path (D-14 — one retry exhausted, the user still gets something)"
	registryUnknownResponseLogFormat = "registry: response for unknown id=%q (dropped, never dispatched, T-16-06)"
	registryUnknownCancelLogFormat   = "registry: cancel for unknown request id=%q (already resolved — no cascade)"
	registryCascadeFailFormat        = "registry: $/cancel_request enqueue failed id=%s: %v"
)

// RegistryConfig carries the registry's discretionary knobs (D-17 class
// windows). Zero values keep the documented defaults; tests shrink the
// FAST-CONTROL window so the D-14 ladder runs in milliseconds.
type RegistryConfig struct {
	FastControlTimeout time.Duration
	HumanAskTimeout    time.Duration
}

// RegistryOption configures a Registry.
type RegistryOption func(*Registry)

// WithRegistryTimeouts overrides the D-17 class windows (zero values keep the
// defaults).
func WithRegistryTimeouts(cfg RegistryConfig) RegistryOption {
	return func(r *Registry) {
		if cfg.FastControlTimeout > 0 {
			r.fastTimeout = cfg.FastControlTimeout
		}

		if cfg.HumanAskTimeout > 0 {
			r.humanTimeout = cfg.HumanAskTimeout
		}
	}
}

// WithRegistryCascade wires the FOREGROUND-emitter seam that carries the D-19
// synthetic-cancel cascade: the $/cancel_request notification rides the
// foreground lane so the turn-end Barrier orders it before the prompt response
// (the documented cancellation contract). Production wiring happens in
// NewServer; tests may omit it (cascades still resolve their waiter) or record
// through a fake.
func WithRegistryCascade(fn func(*Message) error) RegistryOption {
	return func(r *Registry) { r.cascade = fn }
}

// WithRegistryOnCancel registers the cancel-counter hook (D-16 telemetry —
// bumped on every cancelled resolution: -32800, synthetic cancel, caller-ctx
// cancel, shutdown drain). NewServer wires it to the Server's Metrics.
func WithRegistryOnCancel(fn func()) RegistryOption {
	return func(r *Registry) { r.onCancel = fn }
}

// TimeoutClass selects one of D-17's two timeout windows for a Call:
// FAST-CONTROL bounds probe/config requests (machine timescale); HUMAN-ASK
// bounds elicitation/permission asks (a human-scale ask never expires while a
// person is thinking).
type TimeoutClass int

// Timeout classes select the D-17 window for a Call (values are planner
// discretion — the wire never sees them).
const (
	TimeoutFastControl TimeoutClass = iota
	TimeoutHumanAsk
)

// D-17 default windows. DefaultHumanAskTimeout MIRRORS
// session.DefaultAskTimeout (internal/session/ask.go) — acp deliberately does
// not import session in production code (layering); the equality is pinned by
// TestHumanAskTimeoutMirrorsSessionDefault, which fails if either side drifts.
const (
	DefaultFastControlTimeout = 10 * time.Second
	DefaultHumanAskTimeout    = 10 * time.Minute
)

// Registry issues outbound id'd JSON-RPC requests over the ACP stdio wire and
// matches their responses by id (ROADMAP criterion 3).
//
// Concurrency contract: the pending map lives under the registry's OWN mutex —
// resolution NEVER acquires or contends the writer mutex — and every entry
// resolves through a buffered(1) channel so Deliver never blocks on a slow or
// vanished waiter. Outbound request frames go through the Writer directly
// (id'd frames are unconstrained by notification ordering); the D-19 cascade
// is the ONE exception — it rides the emitter's foreground lane so the turn-end
// barrier orders it before the prompt response.
//
// Degradation is D-14's loud ladder (timeout → ONE retry → fallback + structured
// stderr log {id, method, elapsed}); ids are UUID v4 strings BOTH directions
// (D-15, T-16-06 — CSPRNG ids leave nothing to guess); lifecycle is Pitfall
// 8-safe (Stop drains pending waiters before the Writer closes, cascade
// suppressed).
type Registry struct {
	out      NotificationSink     // the Writer: id'd frames bypass notification ordering
	log      *log.Logger          // stderr diagnostics (transport discipline)
	cascade  func(*Message) error // foreground emitter seam for $/cancel_request (D-19); optional
	onCancel func()               // D-16 cancel counter hook; optional

	mu      sync.Mutex
	pending map[string]*pendingEntry
	closed  bool

	fastTimeout  time.Duration // D-17 FAST-CONTROL window
	humanTimeout time.Duration // D-17 HUMAN-ASK window
}

// pendingEntry is one outstanding outbound request. ch is buffered(1) so a
// resolution (Deliver, synthetic cancel, shutdown drain) NEVER blocks on the
// waiter; done is the exactly-once CAS guard (first resolution wins — a late
// answer to either D-14 send finds the guard closed and is dropped).
type pendingEntry struct {
	id     string
	method string
	ch     chan resolution
	done   atomic.Bool
}

// resolution is one delivery into a pending entry's channel: either a real
// response frame or the cancelled marker.
type resolution struct {
	msg       *Message
	cancelled bool
}

// NewRegistry builds a Registry writing outbound id'd frames to out (the
// Server's Writer) and diagnostics to stderr (NEVER stdout — transport
// discipline).
func NewRegistry(out NotificationSink, stderr io.Writer, opts ...RegistryOption) *Registry {
	lg := log.New(io.Discard, "", 0)
	if stderr != nil {
		lg = log.New(stderr, "ass-guard/acp/registry: ", log.LstdFlags|log.Lmsgprefix)
	}

	r := &Registry{
		out:          out,
		log:          lg,
		pending:      map[string]*pendingEntry{},
		fastTimeout:  DefaultFastControlTimeout,
		humanTimeout: DefaultHumanAskTimeout,
	}

	for _, o := range opts {
		o(r)
	}

	return r
}

// Call issues one outbound id'd request and blocks until it resolves: a
// response matched by id (UUID v4 string, D-15), the caller ctx dying, or the
// D-14 ladder exhausting (timeout → ONE retry with a fresh window → fallback
// typed error + structured stderr log). A -32800 error response resolves
// immediately as cancelled with NO retry. Caller-ctx cancellation resolves the
// entry cancelled AND emits the $/cancel_request cascade (D-19).
//
// Success returns (response Message, nil). Every cancellation path returns an
// error wrapping ErrRequestCancelled; the ladder's fallback returns a
// *RequestTimeoutError (errors.As-able, NOT a cancellation — the caller
// degrades to the plain-text path instead).
func (r *Registry) Call(ctx context.Context, method string, params any, class TimeoutClass) (*Message, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("registry: marshal params for %s: %w", method, err)
	}

	entry, err := r.register(method)
	if err != nil {
		return nil, err
	}

	defer r.unregister(entry.id)

	frame := &Message{
		JSONRPC: protocolVersion20,
		ID:      quotedID(entry.id), // D-15: UUID v4 strings both directions
		Method:  method,
		Params:  raw,
	}

	window := r.timeoutFor(class)
	start := time.Now()

	for attempt := 1; attempt <= maxSendAttempts; attempt++ {
		sendErr := r.send(frame)
		if sendErr != nil {
			return nil, fmt.Errorf("registry: send %s (%s): %w", method, entry.id, sendErr)
		}

		select {
		case res := <-entry.ch:
			return r.settle(entry, res)
		case <-ctx.Done():
			r.cancelPending(entry) // turn-death semantics: cancelled + cascade (D-19)

			return nil, fmt.Errorf("registry: %s (%s) cancelled by caller ctx: %w (%w)",
				method, entry.id, ErrRequestCancelled, context.Canceled)
		case <-time.After(window):
			// D-14: window exhausted. The entry STAYS registered — a late
			// answer to either send resolves the single entry (same id).
		}
	}

	elapsed := time.Since(start)

	r.log.Printf(registryFallbackLogFormat, entry.id, method, elapsed.Milliseconds())

	return nil, &RequestTimeoutError{ID: entry.id, Method: method, Elapsed: elapsed}
}

// Deliver routes an inbound response frame to its pending waiter (criterion 3:
// resolution happens under the registry's OWN mutex — never the writer's — and
// the buffered(1) channel means Deliver never blocks on a slow or vanished
// waiter). An unknown id is logged once, structured, and dropped — NEVER
// dispatched (T-16-06: the id is the only authenticity of a response frame).
func (r *Registry) Deliver(msg *Message) {
	if msg == nil {
		return
	}

	id := NormalizeRequestID(msg.ID)

	r.mu.Lock()
	entry, ok := r.pending[id]
	r.mu.Unlock()

	if !ok {
		r.log.Printf(registryUnknownResponseLogFormat, id)

		return
	}

	if !entry.done.CompareAndSwap(false, true) {
		return // duplicate/late delivery after the retry — the first resolution won
	}

	select {
	case entry.ch <- resolution{msg: msg}:
	default:
	}
}

// ResolveCancelled is the D-19 synthetic-cancel primitive: the pending entry
// resolves as cancelled-normal AND exactly one $/cancel_request {requestId}
// notification rides the foreground emitter lane (the documented cascade — the
// client answers -32800, or D-14 closes the loop if it never answers). Turn
// death calls this; Phase 17's ask paths build directly on it.
func (r *Registry) ResolveCancelled(id string) {
	r.mu.Lock()
	entry, ok := r.pending[id]
	r.mu.Unlock()

	if !ok {
		r.log.Printf(registryUnknownCancelLogFormat, id)

		return
	}

	r.cancelPending(entry)
}

// Stop drains every pending entry (serve teardown, Pitfall 8): waiters resolve
// cancelled, the cascade is SUPPRESSED (the client is gone — no write may race
// the closing Writer), and new Calls are rejected. Idempotent.
func (r *Registry) Stop() {
	r.mu.Lock()

	if r.closed {
		r.mu.Unlock()

		return
	}

	r.closed = true

	drained := make([]*pendingEntry, 0, len(r.pending))
	for _, entry := range r.pending {
		drained = append(drained, entry)
	}

	r.pending = map[string]*pendingEntry{}
	r.mu.Unlock()

	for _, entry := range drained {
		if entry.done.CompareAndSwap(false, true) {
			select {
			case entry.ch <- resolution{cancelled: true}:
			default:
			}

			r.noteCancel()
		}
	}
}

// NormalizeRequestID extracts the registry key from a wire id (D-15: the map
// is keyed by the normalized id STRING — our ids are UUID v4 strings the wire
// carries JSON-quoted). Non-string ids normalize to their raw JSON text; they
// can only miss the map (Deliver logs and drops them).
func NormalizeRequestID(id json.RawMessage) string {
	return strings.Trim(string(id), `"`)
}

// quotedID renders a registry id as the JSON string value the wire carries.
func quotedID(id string) json.RawMessage { return json.RawMessage(`"` + id + `"`) }

// register claims a fresh pending entry under the registry mutex (rejected
// after shutdown — no write may follow Writer Close, Pitfall 8).
func (r *Registry) register(method string) (*pendingEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, fmt.Errorf("registry: %s rejected: %w (shut down)", method, ErrRequestCancelled)
	}

	entry := &pendingEntry{id: uuidV4(), method: method, ch: make(chan resolution, 1)}
	r.pending[entry.id] = entry

	return entry, nil
}

func (r *Registry) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.pending, id)
}

// send writes one outbound frame through the sink (production: the Writer).
// Post-shutdown sends are refused — the Writer may already be closed (Pitfall
// 8: Write-after-Close blocks forever).
func (r *Registry) send(frame *Message) error {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()

	if closed {
		return ErrRequestCancelled
	}

	writeErr := r.out.Write(frame)
	if writeErr != nil {
		return fmt.Errorf("registry: write frame: %w", writeErr)
	}

	return nil
}

// settle maps one delivered resolution to Call's tri-state outcome: a real
// response (success), the cancelled marker or a -32800 error (cancellation),
// or anything else verbatim.
func (r *Registry) settle(entry *pendingEntry, res resolution) (*Message, error) {
	switch {
	case res.cancelled:
		return nil, fmt.Errorf("registry: %s (%s) cancelled: %w", entry.method, entry.id, ErrRequestCancelled)
	case res.msg != nil && res.msg.Error != nil && res.msg.Error.Code == CodeRequestCancelled:
		r.noteCancel()

		return nil, fmt.Errorf("registry: %s (%s) cancelled by client (-32800): %w",
			entry.method, entry.id, ErrRequestCancelled)
	default:
		return res.msg, nil
	}
}

// cancelPending resolves the entry as cancelled and emits the $/cancel_request
// cascade (D-19) — exactly one resolution and at most one cascade per entry,
// both guarded by the entry's done CAS. Shutdown suppresses the cascade (the
// client is gone).
func (r *Registry) cancelPending(entry *pendingEntry) {
	if !entry.done.CompareAndSwap(false, true) {
		return // already resolved
	}

	select {
	case entry.ch <- resolution{cancelled: true}:
	default:
	}

	r.noteCancel()
	r.emitCascade(entry.id)
}

// emitCascade pushes the wire-visible half of the D-19 cascade through the
// foreground emitter lane. Suppressed after shutdown (Pitfall 8).
func (r *Registry) emitCascade(id string) {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()

	if closed || r.cascade == nil {
		return
	}

	raw, err := json.Marshal(cancelRequestParams{RequestID: id})
	if err != nil {
		r.log.Printf(registryCascadeFailFormat, id, err)

		return
	}

	cascadeErr := r.cascade(&Message{JSONRPC: protocolVersion20, Method: methodCancelRequest, Params: raw})
	if cascadeErr != nil {
		r.log.Printf(registryCascadeFailFormat, id, cascadeErr)
	}
}

// noteCancel fires the D-16 cancel-counter hook (nil-safe).
func (r *Registry) noteCancel() {
	if r.onCancel != nil {
		r.onCancel()
	}
}

// timeoutFor resolves the D-17 window for one class.
func (r *Registry) timeoutFor(class TimeoutClass) time.Duration {
	switch class {
	case TimeoutFastControl:
		return r.fastTimeout
	case TimeoutHumanAsk:
		return r.humanTimeout
	default:
		return r.fastTimeout
	}
}

// cancelRequestParams is the $/cancel_request payload
// (docs/protocol/v1/cancellation.mdx): {requestId} naming the request to
// cancel.
type cancelRequestParams struct {
	RequestID string `json:"requestId"` //nolint:tagliatelle // ACP wire field
}
