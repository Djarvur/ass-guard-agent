package modelrouting //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
)

var errStreamNotImplemented = errors.New("fake: Stream not implemented in dispatch tests")
var errToolresultmessageNotImplemented = errors.New("fake: ToolResultMessage not implemented")
var errRawTransportBoom = errors.New("raw transport boom")

// fakeProvider implements provider.Provider for dispatch tests. Its Send reads
// the resolved model slug (set by Dispatch on prof.Model) and returns a canned
// outcome per model, recording every call. Stream/ToolResultMessage are stubs
// (Dispatch exercises only Send at this layer).
type fakeProvider struct {
	mu       sync.Mutex
	outcomes map[string]fakeOutcome
	calls    []string
}

type fakeOutcome struct {
	resp provider.Response
	err  *provider.ProviderError
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{outcomes: map[string]fakeOutcome{}}
}

//nolint:funcorder // ordering groups related logic
func (f *fakeProvider) set(model string, oc fakeOutcome) *fakeProvider {
	f.outcomes[model] = oc

	return f
}

func (f *fakeProvider) Send(_ context.Context, prof *profile.Profile, _ []provider.Message) (provider.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, prof.Model)

	oc, ok := f.outcomes[prof.Model]
	if !ok {
		//nolint:err113 // dynamic test error
		return provider.Response{}, errors.New("fake: no outcome registered for model " + prof.Model)
	}

	if oc.err != nil {
		return provider.Response{}, oc.err
	}

	return oc.resp, nil
}

func (f *fakeProvider) Stream(
	context.Context, *profile.Profile, []provider.Message,
) (<-chan provider.StreamChunk, error) {
	return nil, errStreamNotImplemented
}

func (f *fakeProvider) ToolResultMessage(string, json.RawMessage) (json.RawMessage, error) {
	return nil, errToolresultmessageNotImplemented
}

func (f *fakeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.calls)
}

func (f *fakeProvider) calledModels() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	cp := append([]string(nil), f.calls...)

	return cp
}

// recordingSem records Acquire/Release pairs (Test 6).
type recordingSem struct {
	mu   sync.Mutex
	logs []string
}

func (r *recordingSem) Acquire(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logs = append(r.logs, "acquire")

	return nil
}
func (r *recordingSem) Release() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logs = append(r.logs, "release")
}
func (r *recordingSem) pairs() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.logs) / 2
}

// recordingBreaker records the method-call sequence for one (provider, model).
type recordingBreaker struct {
	mu   sync.Mutex
	logs []string
}

func (b *recordingBreaker) Allow(time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs = append(b.logs, "allow")

	return true
}
func (b *recordingBreaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs = append(b.logs, "record-success")
}
func (b *recordingBreaker) RecordTransient(time.Time, *provider.ProviderError) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs = append(b.logs, "record-transient")
}

// recordingCost records the Check/Account sequence.
type recordingCost struct {
	mu   sync.Mutex
	logs []string
}

func (c *recordingCost) Check(time.Time) CostAction {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logs = append(c.logs, "check")

	return CostAllow
}
func (c *recordingCost) Account(model string, _, _ int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logs = append(c.logs, "account:"+model)
}

// newTestScheduler builds a Scheduler over valid.yaml with the given fake
// provider installed for every declared provider key, plus a fresh bus.
func newTestScheduler(t *testing.T, fp *fakeProvider) (*Scheduler, *event.Bus, *Config) {
	t.Helper()
	cfg := loadValid(t)
	bus := event.NewBus()

	t.Cleanup(func() { bus.Close() })

	providers := map[string]provider.Provider{
		providerAnthropic: fp,
		providerOpenAI:    fp,
		providerGroq:      fp,
	}
	s := NewScheduler(cfg, bus, nil, providers, nil)

	return s, bus, cfg
}

// dispatchAndCollect runs a Dispatch and returns (response, events, error).
func dispatchAndCollect(ctx context.Context, t *testing.T, s *Scheduler, bus *event.Bus,
	tier, project string, capReq CapabilityReq) (provider.Response, []ProviderFallback, error) {
	t.Helper()

	ch := bus.Subscribe("ProviderFallback", 16)
	resp, err := s.Dispatch(ctx, tier, project, capReq, &profile.Profile{}, nil)

	var events []ProviderFallback

loop:
	for {
		select {
		case e := <-ch:
			if pf, ok := e.(ProviderFallback); ok {
				events = append(events, pf)
			}
		default:
			break loop
		}
	}

	return resp, events, err
}

// transientErr builds a canned Transient ProviderError.
func transientErr(model string, status int) *provider.ProviderError {
	return &provider.ProviderError{
		Kind: provider.KindTransient, Provider: providerAnthropic, Model: model,
		StatusCode: status, Reason: "rate limited",
	}
}

// TestDispatchPrimarySuccess: primary returns success — no fallback event;
// breaker/cost stubs called (RecordSuccess, Account).
func TestDispatchPrimarySuccess(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().set(modelGLM52, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s, bus, _ := newTestScheduler(t, fp)
	s.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) }) // Sunday noon → global heavy = glm-5.2

	rb := &recordingBreaker{}
	rc := &recordingCost{}

	s.SetBreakers(map[providerModelKey]Breaker{{providerAnthropic, modelGLM52}: rb})
	s.SetCostTracker(rc)

	resp, events, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "myproj", CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, stopReasonStop, resp.FinishReason)
	require.Empty(t, events, "no ProviderFallback event on primary success")
	require.Contains(t, rb.logs, "record-success", "RecordSuccess called on success")
	require.Contains(t, rc.logs, "account:glm-5.2", "Account called with the resolved model")
}

// TestDispatchTransientWalkSuccess: primary Transient → walker tries fallback-1
// → success. Exactly ONE ProviderFallback event (FromModel=primary, ToModel=fallback-1).
func TestDispatchTransientWalkSuccess(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().
		set(modelGLM52, fakeOutcome{err: transientErr(modelGLM52, 429)}).
		set(modelMinimaxM3, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	// Use a Sunday noon so the GLOBAL heavy table applies (glm-5.2 → [minimax-m3, glm-4.6]),
	// not the peak window's heavy binding (which has a different fallback chain).
	s, bus, _ := newTestScheduler(t, fp)
	s.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) })

	resp, events, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "myproj", CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, stopReasonStop, resp.FinishReason)
	require.Len(t, events, 1, "exactly one ProviderFallback event")
	require.Equal(t, modelGLM52, events[0].FromModel)
	require.Equal(t, modelMinimaxM3, events[0].ToModel)
	require.Equal(t, 1, events[0].Attempt)
	require.Equal(t, provider.KindTransient, events[0].ErrorKind)
}

// TestDispatchChainTwoFailuresThenSuccess: primary + fallback-1 Transient,
// fallback-2 success → TWO events; response from fallback-2.
func TestDispatchChainTwoFailuresThenSuccess(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().
		set(modelGLM52, fakeOutcome{err: transientErr(modelGLM52, 429)}).
		set(modelMinimaxM3, fakeOutcome{err: transientErr(modelMinimaxM3, 503)}).
		set(modelGLM46, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s, bus, _ := newTestScheduler(t, fp)
	s.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) })

	resp, events, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "myproj", CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, stopReasonStop, resp.FinishReason)
	require.Len(t, events, 2)
	require.Equal(t, []string{modelGLM52, modelMinimaxM3}, []string{events[0].FromModel, events[0].ToModel})
	require.Equal(t, []string{modelMinimaxM3, modelGLM46}, []string{events[1].FromModel, events[1].ToModel})
	require.Equal(t, []string{modelGLM52, modelMinimaxM3, modelGLM46}, fp.calledModels())
}

// TestDispatchChainExhausted: primary + all fallbacks Transient → returns the
// LAST Transient *ProviderError; events == len(fallbacks).
func TestDispatchChainExhausted(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().
		set(modelGLM52, fakeOutcome{err: transientErr(modelGLM52, 429)}).
		set(modelMinimaxM3, fakeOutcome{err: transientErr(modelMinimaxM3, 503)}).
		set(modelGLM46, fakeOutcome{err: transientErr(modelGLM46, 500)})
	s, bus, _ := newTestScheduler(t, fp)
	s.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) })

	_, events, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "myproj", CapabilityReq{})
	require.Error(t, err)

	var perr *provider.ProviderError

	require.ErrorAs(t, err, &perr)
	require.Equal(t, provider.KindTransient, perr.Kind, "last Transient error surfaced")
	require.Equal(t, modelGLM46, perr.Model, "last-attempted candidate's error")
	// heavy's global fallback chain has 2 entries; one event per transition.
	require.Len(t, events, 2)
}

// TestDispatchStructuralStopsWalk: primary Structural (401) → returns the
// structural error IMMEDIATELY; ZERO events, ZERO fallback attempts (D-04/N5).
func TestDispatchStructuralStopsWalk(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().set(modelGLM52, fakeOutcome{err: &provider.ProviderError{
		Kind: provider.KindStructural, Provider: providerAnthropic, Model: modelGLM52,
		StatusCode: 401, Reason: "unauthenticated",
	}})
	s, bus, _ := newTestScheduler(t, fp)
	s.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) })

	_, events, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "myproj", CapabilityReq{})
	require.Error(t, err)

	var perr *provider.ProviderError

	require.ErrorAs(t, err, &perr)
	require.Equal(t, provider.KindStructural, perr.Kind)
	require.Empty(t, events, "ZERO events on Structural (walk never starts)")
	require.Equal(t, []string{modelGLM52}, fp.calledModels(), "only the primary was attempted")
}

// TestDispatchSemaphorePerCandidate: every candidate attempt Acquires+Releases
// the semaphore (a fallback does not bypass the concurrency bound).
func TestDispatchSemaphorePerCandidate(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().
		set(modelGLM52, fakeOutcome{err: transientErr(modelGLM52, 429)}).
		set(modelMinimaxM3, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	cfg := loadValid(t)
	bus := event.NewBus()

	t.Cleanup(func() { bus.Close() })

	rs := &recordingSem{}
	s := NewScheduler(cfg, bus, rs, map[string]provider.Provider{
		providerAnthropic: fp, providerOpenAI: fp, providerGroq: fp,
	}, nil)
	s.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) })

	_, _, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "myproj", CapabilityReq{})
	require.NoError(t, err)
	// primary + 1 fallback attempted = 2 acquire/release pairs.
	require.Equal(t, 2, rs.pairs(), "semaphore acquired+released once per candidate")
}

// TestDispatchCapabilitySeam: with capReq.NeedsTools, an incapable primary is
// skipped to the first capable fallback (the fake provider is called for the
// capable candidate, NOT the incapable primary). If no candidate satisfies,
// Dispatch returns a structured error.
func TestDispatchCapabilitySeam(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Providers: map[string]ProviderConfig{"p": {BaseURL: "x", Shape: providerAnthropic}},
		Models: map[string]ModelConfig{
			tierToolLess: {Provider: "p", Capabilities: CapabilityProfile{
				ContextWindow: 100, Streaming: true, ToolCalling: false,
			}},
			tierToolFull: {Provider: "p", Capabilities: CapabilityProfile{
				ContextWindow: 100, Streaming: true, ToolCalling: true,
			}},
		},
		Tiers: map[string]TierBinding{tierHeavy: {Model: tierToolLess, Fallback: []string{tierToolFull}}},
	}
	bus := event.NewBus()

	t.Cleanup(func() { bus.Close() })

	fp := newFakeProvider().set(tierToolFull, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s := NewScheduler(cfg, bus, nil, map[string]provider.Provider{"p": fp}, nil)

	resp, _, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "", CapabilityReq{NeedsTools: true})
	require.NoError(t, err)
	require.Equal(t, stopReasonStop, resp.FinishReason)
	require.Equal(t, []string{tierToolFull}, fp.calledModels(),
		"the tool-less primary must be SKIPPED, only the capable fallback called")
}

// TestDispatchCapabilityNoCandidate: a tier whose primary AND fallback all lack
// the required capability → Dispatch returns a structured "no candidate"
// error and never calls the provider.
func TestDispatchCapabilityNoCandidate(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Providers: map[string]ProviderConfig{"p": {BaseURL: "x", Shape: providerAnthropic}},
		Models: map[string]ModelConfig{
			"a": {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: false}},
			"b": {Provider: "p", Capabilities: CapabilityProfile{ContextWindow: 100, ToolCalling: false}},
		},
		Tiers: map[string]TierBinding{tierHeavy: {Model: "a", Fallback: []string{"b"}}},
	}
	bus := event.NewBus()

	t.Cleanup(func() { bus.Close() })

	fp := newFakeProvider()
	s := NewScheduler(cfg, bus, nil, map[string]provider.Provider{"p": fp}, nil)

	_, _, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "", CapabilityReq{NeedsTools: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no candidate")
	require.Contains(t, err.Error(), tierHeavy)
	require.Equal(t, 0, fp.callCount(), "provider must NOT be called when no candidate satisfies capReq")
}

// TestDispatchBreakerCostOrder: on a success, the breaker/cost stubs are called
// in the right sequence — Allow before the call, RecordSuccess + Account after.
func TestDispatchBreakerCostOrder(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider().set(modelGLM52, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s, bus, _ := newTestScheduler(t, fp)
	rb := &recordingBreaker{}
	rc := &recordingCost{}

	s.SetBreakers(map[providerModelKey]Breaker{{providerAnthropic, modelGLM52}: rb})
	s.SetCostTracker(rc)

	_, _, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "myproj", CapabilityReq{})
	require.NoError(t, err)
	// Sequence: the order in Dispatch is capability → breaker.Allow → cost.Check →
	// acquire → send → release → RecordSuccess → Account.
	require.GreaterOrEqual(t, indexOf(rb.logs, "allow"), 0, "Allow called")
	require.Greater(t, indexOf(rb.logs, "record-success"), indexOf(rb.logs, "allow"), "RecordSuccess after Allow")
	require.Greater(t, indexOf(rc.logs, "account:glm-5.2"), indexOf(rc.logs, "check"), "Account after Check")
}

func indexOf(s []string, want string) int {
	for i, v := range s {
		if v == want {
			return i
		}
	}

	return -1
}

// TestAsProviderErrorWrapsNonTyped: a non-ProviderError from the adapter is
// wrapped via ClassifyHTTP (status 0) so Dispatch always pattern-matches a
// typed Kind.
func TestAsProviderErrorWrapsNonTyped(t *testing.T) {
	t.Parallel()

	cand := Target{Provider: providerOpenAI, Model: modelMinimaxM3}
	perr := asProviderError(errRawTransportBoom, &cand)
	require.Equal(t, provider.KindTransient, perr.Kind, "unknown error defaults Transient (safe-side)")
	require.Equal(t, providerOpenAI, perr.Provider)
	require.Equal(t, modelMinimaxM3, perr.Model)
}

// TestRetrySites_NeverRetryStructural (14-06 pin — the retry-site census's
// primary site): the scheduler fallback walk is the ONLY retry path in the
// tree, and its error-kind gate is pinned here from both directions — a
// Structural error NEVER triggers a fallback step (the walk never starts), a
// Transient one DOES (the walk advances and can succeed). Companion pins:
// TestRetryOnlyTransient_ClassificationTable (the table at the errors.go
// seam) and docs/tool-contract-inventory.md §c (every site censused).
func TestRetrySites_NeverRetryStructural(t *testing.T) {
	t.Parallel()

	// Direction 1: Structural on the primary → NO fallback attempted.
	structuralFp := newFakeProvider().
		set(modelGLM52, fakeOutcome{err: &provider.ProviderError{
			Kind: provider.KindStructural, Provider: providerAnthropic, Model: modelGLM52,
			StatusCode: 401, Reason: "unauthenticated",
		}}).
		set(modelMinimaxM3, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s, bus, _ := newTestScheduler(t, structuralFp)
	s.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) })

	_, events, err := dispatchAndCollect(context.Background(), t, s, bus, tierHeavy, "myproj", CapabilityReq{})
	require.Error(t, err)

	var perr *provider.ProviderError

	require.ErrorAs(t, err, &perr)
	require.Equal(t, provider.KindStructural, perr.Kind)
	require.Empty(t, events, "Structural must never trigger a retry/fallback step")
	require.Equal(t, []string{modelGLM52}, structuralFp.calledModels(),
		"the fallback candidate was never attempted")

	// Direction 2: Transient on the primary → the walk advances and succeeds.
	transientFp := newFakeProvider().
		set(modelGLM52, fakeOutcome{err: transientErr(modelGLM52, 429)}).
		set(modelMinimaxM3, fakeOutcome{resp: provider.Response{FinishReason: stopReasonStop}})
	s2, bus2, _ := newTestScheduler(t, transientFp)
	s2.SetNow(func() time.Time { return ny(2026, time.August, 16, 12, 0) })

	resp, events2, err := dispatchAndCollect(context.Background(), t, s2, bus2, tierHeavy, "myproj", CapabilityReq{})
	require.NoError(t, err)
	require.Equal(t, stopReasonStop, resp.FinishReason)
	require.Len(t, events2, 1, "Transient triggers exactly one fallback step")
	require.Equal(t, []string{modelGLM52, modelMinimaxM3}, transientFp.calledModels())
}
