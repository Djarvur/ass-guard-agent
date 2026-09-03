package runtime //nolint:testpackage // internal package test

// 18-05 Task 1 (ACP-06/D-01): the resume-contract battery at the RUNNER seam.
// Runner.ResumeSession is the full transcript-side resume — ReadAll →
// session.Reconcile (classify every dangling expectation) → Manager.
// AppendSynthetic per closure (D-02 provenance on disk) → SeedResume (turn
// counter + plan-mode target) — and the whole load path is transcript-local:
// ZERO provider invocations (the counting provider pins it; D-01's no-LLM
// resume, the 18-05 must-have).

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/profile"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// resumeFixtureSID is a loadSessIDPattern-clean UUID-form session id (the id
// branch newSessionID mints and transcript_<id>.jsonl names).
const resumeFixtureSID = "aaaaaaaa-0b0b-4c0c-8d0d-0e0e0e0e0e0e"

// resumeFixtureTS is a fixed RFC 3339 stamp for deterministic fixture lines.
const resumeFixtureTS = "2026-09-02T12:00:00Z"

// countingProvider is the zero-call pin: the load path must NEVER reach a
// provider (reconciliation is transcript-local — D-01). calls counts every
// Send + Stream.
type countingProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *countingProvider) Send(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (provider.Response, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()

	return provider.Response{}, errNotUsed
}

func (p *countingProvider) Stream(
	_ context.Context, _ *profile.Profile, _ []provider.Message,
) (<-chan provider.StreamChunk, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()

	return nil, errNotUsed
}

func (p *countingProvider) ToolResultMessage(
	_ string, _ json.RawMessage,
) (json.RawMessage, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()

	return nil, errNotUsed
}

func (p *countingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.calls
}

// writeResumeFixture writes dir/.ass-guard/transcript_<sid>.jsonl from raw
// JSONL lines (the hand-written fixture discipline; the exact on-disk
// camelCase spellings of session.Line).
func writeResumeFixture(t *testing.T, dir, sid string, lines []string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, ".ass-guard"), 0o750); err != nil {
		t.Fatalf("mkdir fixture store: %v", err)
	}

	body := strings.Join(lines, "\n") + "\n"

	path := filepath.Join(dir, ".ass-guard", "transcript_"+sid+".jsonl")

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture transcript: %v", err)
	}
}

// resumeClass01Lines is the class-01 kill -9 fixture: a user-message turn
// whose Bash tool_call never got its result and whose session never ended.
func resumeClass01Lines(sid string) []string {
	turn := sid + "-turn-001"

	return []string{
		`{"type":"session_start","timestamp":"` + resumeFixtureTS + `","text":"` + sid + `"}`,
		`{"type":"user_message","turnID":"` + turn + `","timestamp":"` + resumeFixtureTS +
			`","content":[{"type":"text","text":"run the tests"}]}`,
		`{"type":"tool_call","turnID":"` + turn + `","timestamp":"` + resumeFixtureTS +
			`","toolCallID":"call-1","name":"Bash","input":{"command":"make test"}}`,
	}
}

// resumeFullDanglingLines is the every-class-at-once fixture: dangling
// tool_call, unresolved ask_suspended, dangling subagent_dispatch, an open
// chunk stream, a plan_mode enter (the state seed), and no session_end.
func resumeFullDanglingLines(sid string) []string {
	t1 := sid + "-turn-001"
	t2 := sid + "-turn-002"

	return []string{
		`{"type":"session_start","timestamp":"` + resumeFixtureTS + `","text":"` + sid + `"}`,
		`{"type":"user_message","turnID":"` + t1 + `","timestamp":"` + resumeFixtureTS +
			`","content":[{"type":"text","text":"go"}]}`,
		`{"type":"tool_call","turnID":"` + t1 + `","timestamp":"` + resumeFixtureTS +
			`","toolCallID":"call-1","name":"Bash","input":{"command":"true"}}`,
		`{"type":"tool_call","turnID":"` + t1 + `","timestamp":"` + resumeFixtureTS +
			`","toolCallID":"call-2","name":"AskUserQuestion","input":{"questions":[]}}`,
		`{"type":"ask_suspended","turnID":"` + t1 + `","timestamp":"` + resumeFixtureTS +
			`","toolCallID":"call-2","input":{"questions":[]}}`,
		`{"type":"plan_mode","turnID":"` + t1 + `","timestamp":"` + resumeFixtureTS +
			`","cause":"plan_mode_enter","toolCallID":"call-3"}`,
		`{"type":"agent_message_chunk","turnID":"` + t1 + `","timestamp":"` + resumeFixtureTS +
			`","messageID":"` + t1 + `","text":"partial"}`,
		`{"type":"subagent_dispatch","turnID":"` + t2 + `","timestamp":"` + resumeFixtureTS +
			`","parentTurnID":"` + t1 + `","subagentTurnID":"` + t2 + `","toolCallID":"call-4"}`,
	}
}

// newResumeRunner builds the minimal Runner over a temp workdir (the
// newPlanModeWiringRunner shape; no engine needed — the resume path never
// reaches tool execution).
func newResumeRunner(t *testing.T, dir string, prov provider.Provider) *Runner {
	t.Helper()

	return &Runner{
		bus:          event.NewBus(),
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      2,
		askTimeout:   time.Hour,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}
}

// TestResumeSessionReconcilesAndSeeds pins the D-01 resume contract at the
// runner seam: ResumeSession over the class-01 fixture appends the
// provenance-marked closures ON DISK (the failed tool_result with the
// interrupted cause, the canceled terminal, the synthetic session_end), a
// second classification over the re-read transcript yields ZERO closures
// (idempotent — the kill-during-replay row's foundation), and the turn
// counter is seeded from the transcript max so the next turn id continues
// the on-disk sequence (Pitfall 2).
func TestResumeSessionReconcilesAndSeeds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sid := resumeFixtureSID

	writeResumeFixture(t, dir, sid, resumeClass01Lines(sid))

	r := newResumeRunner(t, dir, &countingProvider{})

	rerr := r.ResumeSession(context.Background(), sid)
	if rerr != nil {
		t.Fatalf("ResumeSession: %v", rerr)
	}

	sess := r.sessions[sid]
	if sess == nil {
		t.Fatal("ResumeSession constructed no session")
	}

	lines, err := sess.Manager.ReadAll()
	if err != nil {
		t.Fatalf("re-read transcript: %v", err)
	}

	var (
		sawFailedResult bool
		sawCanceled     bool
		sawSessionEnd   bool
	)

	for i := range lines {
		l := &lines[i]
		switch {
		case l.Type == session.TypeToolResult && l.ToolCallID == "call-1":
			if l.IsError && l.Cause == session.InterruptedCause {
				sawFailedResult = true
			}
		case l.Type == session.TypeCanceled && l.TurnID == sid+"-turn-001":
			if l.Cause == session.InterruptedCause {
				sawCanceled = true
			}
		case l.Type == session.TypeSessionEnd:
			if l.Cause == session.InterruptedCause {
				sawSessionEnd = true
			}
		}
	}

	if !sawFailedResult {
		t.Error("transcript lacks the failed tool_result closure (call-1, isError, cause=interrupted)")
	}

	if !sawCanceled {
		t.Error("transcript lacks the canceled terminal closure (turn-001, cause=interrupted)")
	}

	if !sawSessionEnd {
		t.Error("transcript lacks the synthetic session_end closure (cause=interrupted)")
	}

	// The re-read transcript classifies CLEAN — closures are exactly-once.
	closures, seed := session.Reconcile(sid, lines)
	if len(closures) != 0 {
		t.Errorf("second Reconcile returned %d closures; want 0 (idempotent): %+v", len(closures), closures)
	}

	if seed.MaxTurns != 1 {
		t.Errorf("seed.MaxTurns = %d; want 1 (the fixture's max turn suffix)", seed.MaxTurns)
	}

	// The seeded counter continues the sequence: the next minted id is
	// one past the transcript max (CurrentTurnID reads the seeded value).
	if got := sess.CurrentTurnID(); got != sid+"-turn-001" {
		t.Errorf("CurrentTurnID after seed = %q; want %q (counter seeded from the maxima)",
			got, sid+"-turn-001")
	}
}

// TestResumeSessionSeedsPlanMode pins the row-6 seed through the widened
// SeedResume: a fixture whose LAST plan_mode line is an enter resumes with
// the session's plan-mode state ON (Pitfall 3 — never the fresh default).
func TestResumeSessionSeedsPlanMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sid := resumeFixtureSID

	writeResumeFixture(t, dir, sid, resumeFullDanglingLines(sid))

	r := newResumeRunner(t, dir, &countingProvider{})

	rerr := r.ResumeSession(context.Background(), sid)
	if rerr != nil {
		t.Fatalf("ResumeSession: %v", rerr)
	}

	sess := r.sessions[sid]
	if sess == nil {
		t.Fatal("ResumeSession constructed no session")
	}

	if !sess.PlanModeOn() {
		t.Error("plan mode is OFF after resume; want ON (the last plan_mode line was an enter)")
	}
}

// TestResumeNoProviderCalls pins D-01's zero-LLM resume at the full load
// path: the exported acp Server load core (resume → replay → gate → respond)
// over an every-class dangling fixture invokes the counting stub provider
// ZERO times.
func TestResumeNoProviderCalls(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sid := resumeFixtureSID

	writeResumeFixture(t, dir, sid, resumeFullDanglingLines(sid))

	prov := &countingProvider{}
	r := newResumeRunner(t, dir, prov)

	srvInR, srvInW := io.Pipe()
	cliR, srvOutW := io.Pipe()

	srv := acp.NewServer(srvInR, srvOutW, &bytes.Buffer{},
		acp.WithWorkDir(dir), acp.WithTurnRunner(r))

	ctx, cancel := context.WithCancel(context.Background())

	served := make(chan struct{})

	go func() {
		_ = srv.Serve(ctx)
		close(served)
	}()

	t.Cleanup(func() {
		cancel()

		_ = srvInW.Close()
		_ = srvOutW.Close()
		_ = srvInR.Close()
		_ = cliR.Close()

		select {
		case <-served:
		case <-time.After(2 * time.Second):
		}
	})

	// Drain the replay frames the load emits (the pipe must not block the
	// emitter's drain; the frames themselves are replay_test.go's subject).
	go func() {
		_, _ = io.Copy(io.Discard, cliR)
	}()

	before := prov.count()

	res, lerr := srv.LoadSession(ctx, sid)
	if lerr != nil {
		t.Fatalf("LoadSession: %v", lerr)
	}

	_ = res // the response shape is replay_test.go's subject (acp package)

	if after := prov.count(); after != before {
		t.Errorf("provider invoked during load: %d -> %d calls; want 0 new (transcript-local resume, D-01)",
			before, after)
	}
}
