package acp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// testOutboundMethod is a stand-in outbound method (the cancellation doc's
// terminal/create example — Phase 17's real asks ride the same registry).
const testOutboundMethod = "terminal/create"

// registrySink is a concurrency-safe NotificationSink capturing every frame
// the registry hands to the wire (the Writer stand-in for registry unit tests).
type registrySink struct {
	mu     sync.Mutex
	frames []*Message
}

func (s *registrySink) Write(msg *Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.frames = append(s.frames, msg)

	return nil
}

func (s *registrySink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.frames)
}

func (s *registrySink) frameAt(i int) *Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.frames[i]
}

func (s *registrySink) all() []*Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*Message, len(s.frames))
	copy(out, s.frames)

	return out
}

// awaitCount polls until the sink holds at least n frames (the registry writes
// from the Call goroutine, so the test must poll) or fails at the deadline.
func (s *registrySink) awaitCount(t *testing.T, n int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.count() >= n {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("sink never reached %d frames (have %d)", n, s.count())
}

// cascadeCapture records the notifications emitted through the foreground
// emitter seam (the D-19 $/cancel_request lane).
type cascadeCapture struct {
	mu     sync.Mutex
	frames []*Message
}

func (c *cascadeCapture) notify(msg *Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.frames = append(c.frames, msg)

	return nil
}

func (c *cascadeCapture) all() []*Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]*Message, len(c.frames))
	copy(out, c.frames)

	return out
}

// registryOutcome is one finished Call.
type registryOutcome struct {
	msg *Message
	err error
}

// callAsync starts a Call in its own goroutine (Call blocks until resolution).
func callAsync(reg *Registry, class TimeoutClass) <-chan registryOutcome {
	ch := make(chan registryOutcome, 1)

	go func() {
		msg, err := reg.Call(context.Background(), testOutboundMethod, map[string]any{"probe": true}, class)
		ch <- registryOutcome{msg: msg, err: err}
	}()

	return ch
}

// awaitOutcome waits for a Call outcome (2s hang guard).
func awaitOutcome(t *testing.T, ch <-chan registryOutcome) registryOutcome {
	t.Helper()

	select {
	case oc := <-ch:
		return oc
	case <-time.After(2 * time.Second):
		t.Fatal("registry Call never returned within 2s")

		return registryOutcome{} // unreachable
	}
}

// quotedID renders a registry id as the JSON string the wire carries.
func quotedID(id string) json.RawMessage { return json.RawMessage(`"` + id + `"`) }

// TestRegistryConcurrentResolve proves ROADMAP criterion 3: an id'd request
// written to the client resolves by id while notification frames stream
// concurrently — resolution happens under the registry's own mutex, never the
// writer's (verified under -race).
func TestRegistryConcurrentResolve(t *testing.T) {
	t.Parallel()

	sink := &registrySink{}
	stderr := &strings.Builder{}
	caps := &cascadeCapture{}
	reg := NewRegistry(sink, stderr, WithRegistryCascade(caps.notify))

	outcome := callAsync(reg, TimeoutFastControl)
	sink.awaitCount(t, 1)

	sent := sink.frameAt(0)
	if sent.Method != testOutboundMethod {
		t.Errorf("outbound method = %q; want %q", sent.Method, testOutboundMethod)
	}

	id := NormalizeRequestID(sent.ID)
	if id == "" {
		t.Fatalf("outbound request carried no usable id (raw=%s)", string(sent.ID))
	}

	// A concurrent goroutine streams notification frames while the response
	// lands — the resolution path must not serialize behind wire writes.
	streamDone := make(chan struct{})

	go func() {
		defer close(streamDone)

		for range 100 {
			_ = sink.Write(&Message{JSONRPC: protocolVersion20, Method: methodSessionUpdate})
		}
	}()

	reg.Deliver(&Message{
		JSONRPC: protocolVersion20,
		ID:      quotedID(id),
		Result:  json.RawMessage(`{"ok":true}`),
	})

	oc := awaitOutcome(t, outcome)
	if oc.err != nil {
		t.Fatalf("Call errored: %v", oc.err)
	}

	if string(oc.msg.Result) != `{"ok":true}` {
		t.Errorf("resolved result = %s; want {\"ok\":true}", string(oc.msg.Result))
	}

	<-streamDone

	writes := sink.count() - 100 // subtract the streamed notifications
	if writes != 1 {
		t.Errorf("registry wrote %d request frames; want exactly 1", writes)
	}

	if n := len(caps.all()); n != 0 {
		t.Errorf("plain resolution emitted %d cascade frames; want 0", n)
	}
}

// TestRegistryTimeoutFallback proves D-14's loud ladder: no answer → the frame
// is re-sent ONCE (same id, fresh window) → fallback typed error + exactly one
// structured stderr log carrying id, method, elapsed.
func TestRegistryTimeoutFallback(t *testing.T) {
	t.Parallel()

	sink := &registrySink{}
	stderr := &strings.Builder{}
	reg := NewRegistry(sink, stderr, WithRegistryTimeouts(RegistryConfig{FastControlTimeout: 4 * time.Millisecond}))

	ch := make(chan registryOutcome, 1)

	go func() {
		_, err := reg.Call(context.Background(), testOutboundMethod, map[string]any{}, TimeoutFastControl)
		ch <- registryOutcome{err: err}
	}()

	oc := awaitOutcome(t, ch)

	var timeoutErr *RequestTimeoutError
	if !errors.As(oc.err, &timeoutErr) {
		t.Fatalf("err = %v; want *RequestTimeoutError", oc.err)
	}

	if timeoutErr.Method != testOutboundMethod || timeoutErr.ID == "" {
		t.Errorf("timeout error identity incomplete: id=%q method=%q", timeoutErr.ID, timeoutErr.Method)
	}

	sink.awaitCount(t, 2)

	frames := sink.all()
	if len(frames) != 2 {
		t.Fatalf("outbound frames = %d; want exactly 2 (initial + ONE retry, D-14)", len(frames))
	}

	first, retry := NormalizeRequestID(frames[0].ID), NormalizeRequestID(frames[1].ID)
	if first == "" || first != retry {
		t.Errorf("retry id = %q; want the SAME id as the initial send (%q)", retry, first)
	}

	logs := stderr.String()
	if n := strings.Count(logs, "outbound request fallback"); n != 1 {
		t.Errorf("structured fallback log lines = %d; want exactly 1", n)
	}

	if !strings.Contains(logs, first) {
		t.Error("fallback log missing the request id")
	}

	if !strings.Contains(logs, testOutboundMethod) {
		t.Error("fallback log missing the method")
	}

	if !strings.Contains(logs, "elapsed_ms=") {
		t.Error("fallback log missing elapsed_ms")
	}
}

// TestRegistryRequestCancelledResponse proves the -32800 fast path: an error
// response carrying CodeRequestCancelled resolves immediately as cancelled with
// ZERO retries (exactly one outbound frame).
func TestRegistryRequestCancelledResponse(t *testing.T) {
	t.Parallel()

	sink := &registrySink{}
	reg := NewRegistry(sink, &strings.Builder{}, WithRegistryTimeouts(RegistryConfig{FastControlTimeout: 4 * time.Millisecond}))

	outcome := callAsync(reg, TimeoutFastControl)
	sink.awaitCount(t, 1)

	id := NormalizeRequestID(sink.frameAt(0).ID)
	reg.Deliver(&Message{
		JSONRPC: protocolVersion20,
		ID:      quotedID(id),
		Error:   &RPCError{Code: CodeRequestCancelled, Message: "request cancelled"},
	})

	oc := awaitOutcome(t, outcome)
	if !errors.Is(oc.err, ErrRequestCancelled) {
		t.Fatalf("err = %v; want ErrRequestCancelled", oc.err)
	}

	time.Sleep(15 * time.Millisecond) // > two windows: prove no retry fired

	if got := sink.count(); got != 1 {
		t.Errorf("outbound frames after -32800 = %d; want exactly 1 (no retry)", got)
	}
}

// TestRegistrySyntheticCancel proves the D-19 cascade: ResolveCancelled
// delivers a cancelled resolution AND exactly one $/cancel_request
// {requestId} notification through the foreground emitter lane.
func TestRegistrySyntheticCancel(t *testing.T) {
	t.Parallel()

	sink := &registrySink{}
	caps := &cascadeCapture{}
	reg := NewRegistry(sink, &strings.Builder{}, WithRegistryCascade(caps.notify))

	outcome := callAsync(reg, TimeoutFastControl)
	sink.awaitCount(t, 1)

	id := NormalizeRequestID(sink.frameAt(0).ID)
	reg.ResolveCancelled(id)

	oc := awaitOutcome(t, outcome)
	if !errors.Is(oc.err, ErrRequestCancelled) {
		t.Fatalf("err = %v; want ErrRequestCancelled", oc.err)
	}

	cascades := caps.all()
	if len(cascades) != 1 {
		t.Fatalf("cascade notifications = %d; want exactly 1 (D-19)", len(cascades))
	}

	if cascades[0].Method != methodCancelRequest {
		t.Errorf("cascade method = %q; want %q", cascades[0].Method, methodCancelRequest)
	}

	var params struct {
		RequestID string `json:"requestId"` //nolint:tagliatelle // ACP wire field
	}

	if err := json.Unmarshal(cascades[0].Params, &params); err != nil {
		t.Fatalf("unmarshal cascade params: %v (%s)", err, string(cascades[0].Params))
	}

	if params.RequestID != id {
		t.Errorf("cascade requestId = %q; want %q", params.RequestID, id)
	}
}

// TestRegistryCallerCtxCancel proves turn-death semantics: the caller's ctx
// dying mid-Call (the ctx IS the turn ctx) resolves cancelled AND emits the
// same $/cancel_request cascade.
func TestRegistryCallerCtxCancel(t *testing.T) {
	t.Parallel()

	sink := &registrySink{}
	caps := &cascadeCapture{}
	reg := NewRegistry(sink, &strings.Builder{}, WithRegistryCascade(caps.notify))

	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan registryOutcome, 1)

	go func() {
		_, err := reg.Call(ctx, testOutboundMethod, map[string]any{}, TimeoutFastControl)
		ch <- registryOutcome{err: err}
	}()

	sink.awaitCount(t, 1)
	cancel()

	oc := awaitOutcome(t, ch)
	if !errors.Is(oc.err, ErrRequestCancelled) {
		t.Fatalf("err = %v; want ErrRequestCancelled", oc.err)
	}

	if !errors.Is(oc.err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled preserved for the caller", oc.err)
	}

	if n := len(caps.all()); n != 1 {
		t.Errorf("cascade notifications = %d; want exactly 1", n)
	}
}

// TestRegistryShutdown proves the Pitfall-8 drain: Stop releases every pending
// waiter as cancelled with the cascade SUPPRESSED (the client is gone — no
// write may race the closing Writer), rejects new Calls without writing, and
// is idempotent.
func TestRegistryShutdown(t *testing.T) {
	t.Parallel()

	sink := &registrySink{}
	caps := &cascadeCapture{}
	reg := NewRegistry(sink, &strings.Builder{}, WithRegistryCascade(caps.notify))

	outcome := callAsync(reg, TimeoutFastControl)
	sink.awaitCount(t, 1)

	id := NormalizeRequestID(sink.frameAt(0).ID)

	reg.Stop()

	oc := awaitOutcome(t, outcome)
	if !errors.Is(oc.err, ErrRequestCancelled) {
		t.Fatalf("err = %v; want ErrRequestCancelled (shutdown drain)", oc.err)
	}

	if n := len(caps.all()); n != 0 {
		t.Errorf("cascade notifications after Stop = %d; want 0 (suppressed — client gone)", n)
	}

	// A late response after shutdown: dropped without panic (logged only).
	reg.Deliver(&Message{JSONRPC: protocolVersion20, ID: quotedID(id), Result: json.RawMessage(`{}`)})

	before := sink.count()

	if _, err := reg.Call(context.Background(), testOutboundMethod, nil, TimeoutFastControl); !errors.Is(err, ErrRequestCancelled) {
		t.Errorf("post-shutdown Call err = %v; want ErrRequestCancelled", err)
	}

	if got := sink.count(); got != before {
		t.Errorf("post-shutdown Call wrote %d new frame(s); want 0", got-before)
	}

	reg.Stop() // idempotent
}

// TestHumanAskTimeoutMirrorsSessionDefault pins D-17's HUMAN-ASK class to
// session.DefaultAskTimeout EXACTLY (a human-scale ask never expires while a
// person is thinking). Cross-package pin: acp production code deliberately does
// not import internal/session (layering) — the mirror constant is the contract,
// and THIS test is what breaks if either side drifts.
func TestHumanAskTimeoutMirrorsSessionDefault(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(&registrySink{}, &strings.Builder{})

	if got := reg.timeoutFor(TimeoutHumanAsk); got != session.DefaultAskTimeout {
		t.Errorf("HUMAN-ASK default = %v; want session.DefaultAskTimeout (%v) exactly", got, session.DefaultAskTimeout)
	}

	if got := reg.timeoutFor(TimeoutFastControl); got != DefaultFastControlTimeout {
		t.Errorf("FAST-CONTROL default = %v; want %v", got, DefaultFastControlTimeout)
	}
}
