package runtime //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// --- 20-01: command chain + class-B intercept (tracer battery) ---

// commandFrame is one captured session/update chunk (the tracer's frame sink).
type commandFrame struct {
	kind      string // sessionUpdate kind value ("user_message_chunk", …)
	messageID string
	text      string
}

// tracerEmitter captures every chunk the class-B path emits through the
// in-hand handle — both the D-05 echo (UserMessageChunk) and the output
// (AgentMessageChunk). It implements the widened ActivityEmitter surface so
// the intercept's capability assertion finds it.
type tracerEmitter struct {
	mu     sync.Mutex
	frames []commandFrame
}

func (t *tracerEmitter) UserMessageChunk(messageID, text string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.frames = append(t.frames,
		commandFrame{kind: "user_message_chunk", messageID: messageID, text: text})

	return nil
}

func (t *tracerEmitter) AgentMessageChunk(messageID, text string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.frames = append(t.frames,
		commandFrame{kind: "agent_message_chunk", messageID: messageID, text: text})

	return nil
}

// The remaining ActivityEmitter surface: no-op captures (the class-B path
// never produces tool/plan/thought frames; the widening exists so the
// intercept's capability assertion finds the handle).
func (t *tracerEmitter) ToolCall(_ *acp.ToolCallFrame) error             { return nil }
func (t *tracerEmitter) ToolCallUpdate(_ *acp.ToolCallUpdateFrame) error { return nil }
func (t *tracerEmitter) PlanUpdate(_ []acp.PlanEntry) error              { return nil }

//nolint:gocritic // interface-mandated value param
func (t *tracerEmitter) ThoughtChunk(_ string, _ acp.ContentBlock) error { return nil }

func (t *tracerEmitter) snapshot() []commandFrame {
	t.mu.Lock()
	defer t.mu.Unlock()

	return append([]commandFrame(nil), t.frames...)
}

// writeDiscoveredCommand installs one discovered file command into dir's
// project `.claude/commands/` (the 08-04 layout).
func writeDiscoveredCommand(t *testing.T, dir, name, body string) {
	t.Helper()

	p := filepath.Join(dir, ".claude", "commands", name+".md")

	err := os.MkdirAll(filepath.Dir(p), 0o750)
	if err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}

	err = os.WriteFile(p, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

// newCommandRunner builds a Runner over a temp workDir, with the command
// registry + chain loaded. Returns the runner, its counting provider, and the
// stderr buffer (shadow-check warnings land there).
func newCommandRunner(t *testing.T, fixtures func(dir string)) (*Runner, *scriptedACPProvider, *bytes.Buffer) {
	t.Helper()

	bus := event.NewBus()
	prov := &scriptedACPProvider{}

	dir := t.TempDir()
	if fixtures != nil {
		fixtures(dir)
	}

	stderr := &bytes.Buffer{}

	r := &Runner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      4,
		stderr:       stderr,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}

	r.LoadCommandRegistry()

	return r, prov, stderr
}

// TestTracerStatusClassB is the 20-01 tracer: one typed /status drives the
// whole single path — chain → class-B intercept → echo/output chunks/end_turn
// → local_command line — with ZERO provider calls (CMDS-02, D-05), and the
// session-start advertisement carries the chain winners (ACP-04, D-04).
//
//nolint:gocognit,gocyclo,cyclop,funlen,paralleltest // one tracer scenario, one path (shared runner state)
func TestTracerStatusClassB(t *testing.T) {
	r, prov, _ := newCommandRunner(t, func(dir string) {
		writeDiscoveredCommand(t, dir, "greet", "Say hi: $ARGUMENTS\n")
	})

	emit := &tracerEmitter{}

	stop, err := r.Run(context.Background(), "tracer-status",
		emit, []acp.ContentBlock{{Type: blockText, Text: "/status deep-check"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if stop != stopEndTurn {
		t.Errorf("stop = %q; want %q (class-B never suspends)", stop, stopEndTurn)
	}

	if got := prov.callCount(); got != 0 {
		t.Errorf("provider Stream calls = %d; want 0 (zero model turns, CMDS-02)", got)
	}

	frames := emit.snapshot()

	var echoes, outputs []commandFrame

	for _, f := range frames {
		switch f.kind {
		case "user_message_chunk":
			echoes = append(echoes, f)
		case "agent_message_chunk":
			outputs = append(outputs, f)
		}
	}

	if len(echoes) != 1 {
		t.Fatalf("user_message_chunk frames = %d; want exactly 1 (D-05 echo)", len(echoes))
	}

	if echoes[0].text != "/status deep-check" {
		t.Errorf("echo text = %q; want the typed invocation verbatim", echoes[0].text)
	}

	if len(outputs) == 0 {
		t.Fatal("agent_message_chunk frames = 0; want >=1 (D-05 output)")
	}

	if outputs[0].messageID == echoes[0].messageID {
		t.Errorf("echo messageId %q == output messageId; want DIFFERENT ids so the client renders two messages (D-05)",
			echoes[0].messageID)
	}

	var outText strings.Builder

	for _, f := range outputs {
		outText.WriteString(f.text)
	}

	if !strings.Contains(outText.String(), "tracer-status") {
		t.Errorf("output missing session id line; got:\n%s", outText.String())
	}

	// The durable record: one local_command line with the 16-D-22 shape.
	lines, rerr := r.sessions["tracer-status"].Manager.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	var local *session.Line

	for i := range lines {
		if lines[i].Type == session.TypeLocalCommand {
			if local != nil {
				t.Fatal("multiple local_command lines; want exactly 1")
			}

			local = &lines[i]
		}
	}

	if local == nil {
		t.Fatal("no local_command line in transcript (16-D-22 durable record)")
	}

	if local.Name != "status" {
		t.Errorf("local_command Name = %q; want %q", local.Name, "status")
	}

	if local.Args != "deep-check" {
		t.Errorf("local_command Args = %q; want typed args verbatim (D-22)", local.Args)
	}

	if len(local.SourceChain) != 1 || local.SourceChain[0] != "builtin" {
		t.Errorf("local_command SourceChain = %v; want [builtin]", local.SourceChain)
	}

	// Single-parse discipline: the class-B path resolves through the chain
	// exactly once per Run (behavioral, not grep).
	if got := r.chainResolveCount(); got != 1 {
		t.Errorf("chain resolutions during Run = %d; want 1 (single-parse discipline)", got)
	}

	// Advertisement winners (D-04): status + init builtins + the discovered
	// greet file command all present; names sorted + unique; no reserved
	// name ever answered by a discovered entry. (User-scope discovery makes
	// the full set environment-dependent — exact-set coverage is the pure
	// buildChain battery below.)
	ad := r.CommandAdvertisement()

	names := make([]string, 0, len(ad))

	for _, f := range ad {
		names = append(names, f.Name)
	}

	if !sort.StringsAreSorted(names) {
		t.Errorf("advertisement names not sorted: %v", names)
	}

	seen := make(map[string]bool)

	for _, n := range names {
		if seen[n] {
			t.Errorf("advertisement name %q appears twice (winners-only violated)", n)
		}

		seen[n] = true
	}

	for _, want := range []string{"greet", "init", "status"} {
		if !seen[want] {
			t.Errorf("advertisement missing %q; got %v (sorted chain winners, D-04)", want, names)
		}
	}
}
