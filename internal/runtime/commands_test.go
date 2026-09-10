package runtime //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
	"github.com/Djarvur/ass-guard-agent/internal/checkpoint"
	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/event"
	"github.com/Djarvur/ass-guard-agent/internal/modelrouting"
	"github.com/Djarvur/ass-guard-agent/internal/provider"
	"github.com/Djarvur/ass-guard-agent/internal/session"
)

// --- 20-01: command chain + class-B intercept (tracer battery) ---

// Captured frame-kind literals + fixture vocabulary (goconst-extracted).
const (
	frameKindUserEcho   = "user_message_chunk"
	frameKindAgentChunk = "agent_message_chunk"
	fixtureProvider     = "zai"
	fixtureCredEnv      = "ZAI_API_KEY"
	fixtureModel        = "glm-5.2"
	fixtureModelLite    = "glm-4.7-air"
)

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
		commandFrame{kind: frameKindUserEcho, messageID: messageID, text: text})

	return nil
}

func (t *tracerEmitter) AgentMessageChunk(messageID, text string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.frames = append(t.frames,
		commandFrame{kind: frameKindAgentChunk, messageID: messageID, text: text})

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
		providerName: fixtureProvider,
		schedCfg: &modelrouting.Config{
			SessionTier: "heavy",
			Providers: map[string]modelrouting.ProviderConfig{
				fixtureProvider: {APIKeyEnv: fixtureCredEnv},
			},
			Tiers: map[string]modelrouting.TierBinding{
				tierHeavy: {Model: fixtureModel},
				tierLight: {Model: fixtureModelLite},
			},
			Models: map[string]modelrouting.ModelConfig{
				"glm-5.2":     {Provider: "zai"},
				"glm-4.7-air": {Provider: "zai"},
			},
		},
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
		case frameKindUserEcho:
			echoes = append(echoes, f)
		case frameKindAgentChunk:
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

	if local.Name != nameStatus {
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

	for _, want := range []string{"greet", nameInit, nameStatus} {
		if !seen[want] {
			t.Errorf("advertisement missing %q; got %v (sorted chain winners, D-04)", want, names)
		}
	}
}

// --- 20-01 Task 2: chain semantics battery (D-01/D-02/D-04) ---

// Fixture name vocabulary (goconst-extracted).
const (
	nameStatus  = "status"
	nameInit    = "init"
	nameDupe    = "dupe"
	nameModel   = "model"
	nameCompact = "compact"
	nameTandem  = "tandem"
	nameProbe   = "probe"
	namePlain   = "plain"
)

// chainFixture builds an ecosys.Registry value in code (no temp dirs) with
// colliding names across the discovered kinds.
func chainFixture() ecosys.Registry {
	return ecosys.Registry{
		Skills: map[string]ecosys.Skill{
			nameDupe:  {Name: nameDupe, Description: "the skill", Path: "skill-dupe.md"},
			"helper":  {Name: "helper", Description: "helper skill", Path: "skill-helper.md"},
			nameModel: {Name: nameModel, Description: "skill stealing a reserved name", Path: "skill-model.md"},
		},
		Agents: map[string]ecosys.Agent{
			nameDupe: {Name: nameDupe, Description: "the agent", Path: "agent-dupe.md"},
			"probe":  {Name: nameProbe, Description: "probe agent", Path: "agent-probe.md"},
			"tandem": {Name: nameTandem, Description: "agent over file", Path: "agent-tandem.md"},
		},
		Commands: map[string]ecosys.Command{
			nameDupe:    {Name: nameDupe, Description: "the file", Path: "file-dupe.md"},
			nameCompact: {Name: nameCompact, Description: "a file named compact", Path: "file-compact.md", ArgumentHint: "[focus]"},
			"plain":     {Name: namePlain, Description: "plain file command", Path: "file-plain.md"},
			"hinted":    {Name: "hinted", Description: "hinted file command", Path: "file-hinted.md", ArgumentHint: "<target>"},
			"doctor2":   {Name: "doctor2", Description: "not reserved", Path: "file-doctor2.md"},
			"tandem":    {Name: nameTandem, Description: "file under agent", Path: "file-tandem.md"},
			nameStatus:  {Name: nameStatus, Description: "a file named status", Path: "file-status.md"},
		},
	}
}

// TestCommandChainSemantics is the D-02 collision table: builtin beats
// skill/agent/file; skill beats agent beats file; first-writer-wins is
// deterministic and silent; unknown names miss without error.
func TestCommandChainSemantics(t *testing.T) {
	t.Parallel()

	reg := chainFixture()
	before := reg // the chain is a VIEW — reg maps must be unchanged after build

	c := buildChain(reg, nil)

	cases := []struct {
		name       string
		wantKind   string
		wantWantBy string // the winner's origin file
	}{
		{nameDupe, chainKindSkill, "skill-dupe.md"},     // three-way: skill > agent > file
		{nameTandem, chainKindAgent, "agent-tandem.md"}, // agent beats file (no skill)
		{"probe", chainKindAgent, "agent-probe.md"},     // agent with no skill collision
		{namePlain, chainKindFile, "file-plain.md"},
		{nameStatus, chainKindBuiltin, ""}, // the live builtin
		{nameInit, chainKindBuiltin, ""},   // the class-A reservation
	}

	for _, tc := range cases {
		e, ok := c.resolve(tc.name)
		if !ok {
			t.Errorf("resolve(%q): not found; want %s winner", tc.name, tc.wantKind)

			continue
		}

		if e.kind != tc.wantKind {
			t.Errorf("resolve(%q).kind = %s; want %s (D-02 chain order)", tc.name, e.kind, tc.wantKind)
		}
	}

	// The losers stay reachable on their native surfaces: the registry maps
	// are untouched (the chain is a view, Pitfall 1).
	if diff := registryDiff(before, reg); diff != "" {
		t.Errorf("buildChain mutated the registry (it is a view):\n%s", diff)
	}
}

// TestCommandChainReservedShadowing pins D-01: a discovered entry sharing a
// reserved builtin name NEVER enters the chain (neither as winner nor as a
// shadow advertisement row), and exactly ONE structured warning per shadowed
// FILE per build names the file and the reserved name.
func TestCommandChainReservedShadowing(t *testing.T) {
	t.Parallel()

	reg := chainFixture()
	stderr := &bytes.Buffer{}

	c := buildChain(reg, stderr)

	for _, name := range []string{nameModel, "compact", "status"} {
		if e, ok := c.resolve(name); ok && e.kind != chainKindBuiltin {
			t.Errorf("discovered entry %q entered the chain as %s; reserved names never fire via slash (D-01)",
				name, e.kind)
		}
	}

	// 20-02 note: model + compact are LIVE builtins now, so they ARE in the
	// advertisement — as the BUILTIN winners. The D-04 assertion is that the
	// DISCOVERED entries lost: the winner kind is builtin, never skill/file.
	for _, name := range []string{nameModel, nameCompact, nameStatus} {
		e, ok := c.resolve(name)
		if !ok || e.kind != chainKindBuiltin {
			t.Errorf("winner for %q = %+v; want the builtin (D-01)", name, e)
		}
	}

	out := stderr.String()

	wantWarns := []string{
		"skill-model.md", `"` + nameModel + `"`,
		"file-compact.md", `"` + nameCompact + `"`,
		"file-status.md", `"` + nameStatus + `"`,
	}

	for _, want := range wantWarns {
		if !strings.Contains(out, want) {
			t.Errorf("shadow-check warning missing %q; got:\n%s", want, out)
		}
	}

	// Exactly once per FILE per build: build again from the SAME registry and
	// count occurrences.
	stderr.Reset()
	buildChain(reg, stderr)

	if got := strings.Count(stderr.String(), "shadows reserved builtin name"); got != 3 {
		t.Errorf("second build emitted %d shadow warnings; want 3 (one per file, deduped by path)", got)
	}
}

// TestCommandChainAdvertisementShape pins D-04 + the wire projection: sorted
// winner names, each exactly once, shadowed entries absent, Input omitted
// when the hint is empty and present when set.
func TestCommandChainAdvertisementShape(t *testing.T) {
	t.Parallel()

	reg := chainFixture()
	c := buildChain(reg, nil)

	ad := c.advertisement()

	if !sort.StringsAreSorted(func() []string {
		names := make([]string, 0, len(ad))
		for _, f := range ad {
			names = append(names, f.Name)
		}

		return names
	}()) {
		t.Errorf("advertisement not sorted by name")
	}

	byName := make(map[string]acp.AvailableCommandFrame, len(ad))
	for _, f := range ad {
		if _, dup := byName[f.Name]; dup {
			t.Errorf("advertisement lists %q twice (winners-only, D-04)", f.Name)
		}

		byName[f.Name] = f
	}

	// dupe resolved to the SKILL — the agent/file losers are absent.
	if _, ok := byName[nameProbe]; !ok {
		t.Error("advertisement missing agent winner probe")
	}

	if e := byName[namePlain]; e.Input != nil {
		t.Errorf("plain (no hint) carried Input %v; want omitted", e.Input)
	}

	if e := byName["hinted"]; e.Input == nil || e.Input.Hint != "<target>" {
		t.Errorf("hinted Input = %+v; want hint <target> from argument-hint", e.Input)
	}

	if e := byName[nameProbe]; e.Input == nil || e.Input.Hint != agentDispatchHint {
		t.Errorf("agent winner Input = %+v; want the dispatch hint", e.Input)
	}
}

// TestCommandChainEmptyRegistry pins the degenerate build: an empty registry
// yields the builtin-only chain (status + init), resolvable, no warnings.
func TestCommandChainEmptyRegistry(t *testing.T) {
	t.Parallel()

	stderr := &bytes.Buffer{}
	c := buildChain(ecosys.Registry{}, stderr)

	for _, name := range []string{nameStatus, nameInit} {
		if _, ok := c.resolve(name); !ok {
			t.Errorf("empty-registry chain missing builtin %q", name)
		}
	}

	if e, ok := c.resolve("status"); !ok || e.handler == nil {
		t.Error("empty-registry chain status entry lacks its live handler")
	}

	if e, ok := c.resolve(nameInit); !ok || !e.hasClassA {
		t.Error("empty-registry chain init entry lacks the class-A reservation")
	}

	if _, ok := c.resolve("anything-else"); ok {
		t.Error("empty-registry chain resolved an unknown name")
	}

	if stderr.Len() != 0 {
		t.Errorf("empty-registry build warned: %s", stderr.String())
	}
}

// TestReservedNamesFitInvocationGrammar pins Pitfall 7: every reserved
// builtin name matches the invocation regex — a builtin outside the grammar
// could never fire and would strand its reservation.
func TestReservedNamesFitInvocationGrammar(t *testing.T) {
	t.Parallel()

	re := regexp.MustCompile(`^[a-z0-9][a-z0-9_:-]{0,63}$`)

	for name := range reservedNames {
		if !re.MatchString(name) {
			t.Errorf("reserved name %q does not match the invocation grammar", name)
		}
	}

	if got := len(reservedNames); got != 13 {
		t.Errorf("reserved name set has %d entries; want 13 (twelve class-B + init)", got)
	}
}

// registryDiff reports a human-readable difference between two registries'
// discovered maps (the view-immutability assertion's lens; lengths + paths
// suffice — a mutated value or map would shift one of the two).
//
//nolint:cyclop // flat per-map diff lens
func registryDiff(a, b ecosys.Registry) string {
	var sb strings.Builder

	if len(a.Skills) != len(b.Skills) {
		fmt.Fprintf(&sb, "Skills len %d -> %d\n", len(a.Skills), len(b.Skills))
	}

	if len(a.Agents) != len(b.Agents) {
		fmt.Fprintf(&sb, "Agents len %d -> %d\n", len(a.Agents), len(b.Agents))
	}

	if len(a.Commands) != len(b.Commands) {
		fmt.Fprintf(&sb, "Commands len %d -> %d\n", len(a.Commands), len(b.Commands))
	}

	for key, av := range a.Skills {
		if bv, ok := b.Skills[key]; !ok || av.Path != bv.Path {
			fmt.Fprintf(&sb, "Skills[%s] changed\n", key)
		}
	}

	for key := range b.Skills {
		if _, ok := a.Skills[key]; !ok {
			fmt.Fprintf(&sb, "Skills[%s] added\n", key)
		}
	}

	for key, av := range a.Agents {
		if bv, ok := b.Agents[key]; !ok || av.Path != bv.Path {
			fmt.Fprintf(&sb, "Agents[%s] changed\n", key)
		}
	}

	for key := range b.Agents {
		if _, ok := a.Agents[key]; !ok {
			fmt.Fprintf(&sb, "Agents[%s] added\n", key)
		}
	}

	for key, av := range a.Commands {
		if bv, ok := b.Commands[key]; !ok || av.Path != bv.Path {
			fmt.Fprintf(&sb, "Commands[%s] changed\n", key)
		}
	}

	for key := range b.Commands {
		if _, ok := a.Commands[key]; !ok {
			fmt.Fprintf(&sb, "Commands[%s] added\n", key)
		}
	}

	return sb.String()
}

// --- 20-02: the class-B family battery (CMDS-02) ---

// classBRun drives one class-B invocation through Run and returns the
// captured frames + transcript lines (the battery's shared lens).
func classBRun(t *testing.T, r *Runner, prompt string) ([]commandFrame, []session.Line) {
	t.Helper()

	emit := &tracerEmitter{}

	stop, err := r.Run(context.Background(), "classb", emit, []acp.ContentBlock{{Type: blockText, Text: prompt}})
	if err != nil {
		t.Fatalf("Run(%q): %v", prompt, err)
	}

	if stop != stopEndTurn {
		t.Fatalf("Run(%q) stop = %q; want end_turn", prompt, stop)
	}

	lines, rerr := r.sessions["classb"].Manager.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	return emit.snapshot(), lines
}

// localCommandLine returns the LAST local_command line (the durable record).
func localCommandLine(t *testing.T, lines []session.Line) session.Line {
	t.Helper()

	var out *session.Line

	for i := range lines {
		if lines[i].Type == session.TypeLocalCommand {
			out = &lines[i]
		}
	}

	if out == nil {
		t.Fatal("no local_command line in transcript")
	}

	return *out
}

// TestClassBPureLocal pins the six pure-local commands: zero provider calls,
// the D-05 shape, a durable local_command record, and each command's
// output contract (Pitfall 8: no network — asserted by the fake provider's
// zero Stream count).
//
//nolint:gocognit,gocyclo,cyclop,funlen,paralleltest // one table, one shared lens (sequenced runner state)
func TestClassBPureLocal(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "sk-doctor-secret-fixture")

	cases := []struct {
		name   string
		prompt string
		check  func(t *testing.T, out string)
	}{
		{"help", "/help", func(t *testing.T, out string) {
			t.Helper()

			for _, want := range []string{nameStatus, nameInit, "greet"} {
				if !strings.Contains(out, want) {
					t.Errorf("/help output missing %q:\n%s", want, out)
				}
			}
		}},
		{"memory", "/memory", func(t *testing.T, out string) {
			t.Helper()

			if !strings.Contains(out, "memory") {
				t.Errorf("/memory output missing memory section:\n%s", out)
			}
		}},
		{"permissions", "/permissions", func(t *testing.T, out string) {
			t.Helper()

			if !strings.Contains(out, "ungated") && !strings.Contains(out, "gated") {
				t.Errorf("/permissions output names no mode:\n%s", out)
			}
		}},
		{"mcp", "/mcp", func(t *testing.T, out string) {
			t.Helper()

			if !strings.Contains(out, "mcp") && !strings.Contains(out, "MCP") {
				t.Errorf("/mcp output missing mcp section:\n%s", out)
			}
		}},
		{"doctor", "/doctor", func(t *testing.T, out string) {
			t.Helper()

			if !strings.Contains(out, fixtureCredEnv) {
				t.Errorf("/doctor output missing credential env var NAME:\n%s", out)
			}

			if strings.Contains(out, "sk-doctor-secret-fixture") {
				t.Error("/doctor output LEAKED the credential value (T-20-05)")
			}
		}},
		{"config", "/config", func(t *testing.T, out string) {
			t.Helper()

			if !strings.Contains(out, "tier") && !strings.Contains(out, "model") {
				t.Errorf("/config output missing tier/model view:\n%s", out)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, prov, _ := newCommandRunner(t, func(dir string) {
				writeDiscoveredCommand(t, dir, "greet", "Say hi: $ARGUMENTS\n")
			})

			// Prime a session so handlers read live state.
			_, _ = classBRun(t, r, "/status")

			frames, lines := classBRun(t, r, tc.prompt)

			if got := prov.callCount(); got != 0 {
				t.Errorf("provider Stream calls = %d; want 0", got)
			}

			var echoed, outputted bool

			var outText strings.Builder

			for _, f := range frames {
				switch f.kind {
				case frameKindUserEcho:
					echoed = echoed || f.text == tc.prompt
				case frameKindAgentChunk:
					outputted = true

					outText.WriteString(f.text)
				}
			}

			if !echoed {
				t.Errorf("no echo frame carrying %q verbatim", tc.prompt)
			}

			if !outputted {
				t.Error("no output agent_message_chunk frames")
			}

			rec := localCommandLine(t, lines)
			if rec.Name != tc.name {
				t.Errorf("local_command Name = %q; want %q", rec.Name, tc.name)
			}

			if rec.Expansion != "ok" {
				t.Errorf("local_command outcome = %q; want ok", rec.Expansion)
			}

			tc.check(t, outText.String())
		})
	}
}

// TestClassBHelpSelfDescribing pins D-08: /help renders FROM the live chain —
// adding a skill changes the output with no hand-maintained list.
//
//nolint:paralleltest // sequenced runner state
func TestClassBHelpSelfDescribing(t *testing.T) {
	r, _, _ := newCommandRunner(t, func(dir string) {
		writeDiscoveredCommand(t, dir, "greet", "Say hi: $ARGUMENTS\n")
	})

	frames, _ := classBRun(t, r, "/help")

	var before strings.Builder

	for _, f := range frames {
		if f.kind == "agent_message_chunk" {
			before.WriteString(f.text)
		}
	}

	// Add a skill to the registry + rebuild the chain (the rescan path 20-05
	// automates; the battery drives the seam directly).
	reg := r.reg
	reg.Skills = map[string]ecosys.Skill{
		"greeter": {Name: "greeter", Description: "greets warmly", Path: "greeter/SKILL.md"},
	}

	r.reg = reg
	r.rebuildCommandChain()

	frames2, _ := classBRun(t, r, "/help")

	var after strings.Builder

	for _, f := range frames2 {
		if f.kind == "agent_message_chunk" {
			after.WriteString(f.text)
		}
	}

	if before.String() == after.String() {
		t.Fatal("/help output identical after a skill was added (not self-describing, D-08)")
	}

	if !strings.Contains(after.String(), "greeter") {
		t.Errorf("/help output after adding greeter does not name it:\n%s", after.String())
	}
}

// TestClassBMemoryReadOnly pins D-09: /memory never writes — every file in
// the workDir compares byte-identical before/after.
//
//nolint:paralleltest // sequenced runner state
func TestClassBMemoryReadOnly(t *testing.T) {
	r, _, _ := newCommandRunner(t, func(dir string) {
		writeDiscoveredCommand(t, dir, "greet", "Say hi\n")

		err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# guide\nproject memory\n"), 0o600)
		if err != nil {
			t.Fatalf("write AGENTS.md: %v", err)
		}
	})

	before := dirSnapshot(t, r.workDir)

	_, _ = classBRun(t, r, "/memory")

	after := dirSnapshot(t, r.workDir)

	if before != after {
		t.Error("/memory mutated the workDir (read-only, D-09)")
	}
}

// dirSnapshot renders every file's relative path + content under dir.
func dirSnapshot(t *testing.T, dir string) string {
	t.Helper()

	var sb strings.Builder

	var walk func(rel string)

	walk = func(rel string) {
		entries, err := os.ReadDir(filepath.Join(dir, rel))
		if err != nil {
			return
		}

		for _, e := range entries {
			p := filepath.Join(rel, e.Name())
			if e.IsDir() {
				if e.Name() == ".ass-guard" {
					continue // the transcript — every class-B turn's durable record, not memory state
				}

				walk(p)

				continue
			}

			data, rerr := os.ReadFile(filepath.Join(dir, p))
			if rerr == nil {
				sb.WriteString(p + ":" + string(data) + "\n")
			}
		}
	}

	walk(".")

	return sb.String()
}

// TestClassBModel pins /model (16-D-12 session-scope live-apply): a declared
// slug switches THIS session's model for the next request; no args prints the
// current model; an unknown slug degrades loudly with the model UNCHANGED;
// no config layer file is ever written.
//
//nolint:gocognit,funlen,paralleltest // one table, one shared lens (sequenced runner state)
func TestClassBModel(t *testing.T) {
	cases := []struct {
		name      string
		prompt    string
		wantModel string
		wantInOut []string
	}{
		{
			name:      "declared slug applies session-scope",
			prompt:    "/model " + fixtureModel,
			wantModel: fixtureModel,
		},
		{
			name:      "no args prints current model",
			prompt:    "/model",
			wantModel: "",
			wantInOut: []string{"model", "tier"},
		},
		{
			name:      "unknown slug degrades loudly, model unchanged",
			prompt:    "/model not-a-declared-slug",
			wantModel: "",
			wantInOut: []string{"unknown"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, prov, _ := newCommandRunner(t, nil)

			// Seed the project config layer file to prove it is never written.
			layerDir := filepath.Join(r.workDir, ".ass-guard")

			err := os.MkdirAll(layerDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir layer dir: %v", err)
			}

			layerFile := filepath.Join(layerDir, "config.yaml")

			err = os.WriteFile(layerFile, []byte("# frozen fixture layer\n"), 0o600)
			if err != nil {
				t.Fatalf("write layer fixture: %v", err)
			}

			before := dirSnapshot(t, r.workDir)

			cmdFrames, _ := classBRun(t, r, tc.prompt)

			var out strings.Builder

			for _, f := range cmdFrames {
				if f.kind == frameKindAgentChunk {
					out.WriteString(f.text)
				}
			}

			for _, want := range tc.wantInOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q:\n%s", want, out.String())
				}
			}

			// A follow-up ordinary turn: the provider sees the model.
			emit := &tracerEmitter{}

			_, ferr := r.Run(context.Background(), "classb",
				emit, []acp.ContentBlock{{Type: blockText, Text: "hello there"}})
			if ferr != nil {
				t.Fatalf("follow-up turn: %v", ferr)
			}

			if got := prov.callCount(); got != 1 {
				t.Fatalf("follow-up Stream calls = %d; want 1 (the ordinary turn only)", got)
			}

			gotModel := prov.lastStreamModel()
			if tc.wantModel != "" {
				if gotModel != tc.wantModel {
					t.Errorf("next request model = %q; want %q", gotModel, tc.wantModel)
				}
			} else if gotModel != fixtureModel {
				// The session's stamped default (tier-resolved fixtureModel)
				// must survive the no-arg/degrade posture untouched.
				t.Errorf("next request model = %q; want the unchanged default %q", gotModel, fixtureModel)
			}

			// Layer discipline: the config layer file is byte-identical.
			if after := dirSnapshot(t, r.workDir); after != before {
				t.Error("workDir mutated beyond the transcript (config-write path violation)")
			}
		})
	}
}

// TestClassBClear pins D-06: /clear writes a full context-reset boundary in
// the SAME session — the next turn's projection is empty of prior content,
// while the transcript retains every line and the session id is unchanged.
//
//nolint:paralleltest // sequenced runner state
func TestClassBClear(t *testing.T) {
	r, prov, _ := newCommandRunner(t, nil)
	prov.queue(scriptedResp{text: "first answer"})

	sessBefore := r.sessionFor(context.Background(), "classb")

	emit := &tracerEmitter{}

	_, err := r.Run(context.Background(), "classb",
		emit, []acp.ContentBlock{{Type: blockText, Text: "remember the codeword pinecone"}})
	if err != nil {
		t.Fatalf("pre-clear turn: %v", err)
	}

	_, lines := classBRun(t, r, "/clear")

	var boundary bool

	for _, l := range lines {
		if l.Type == session.TypeBoundary && l.Cause == session.BoundaryCauseContextReset {
			boundary = true
		}
	}

	if !boundary {
		t.Fatal("no local-command:clear boundary line after /clear (D-06)")
	}

	if got := len(lines); got < 3 {
		t.Fatalf("transcript lost lines after /clear: %d lines", got)
	}

	// The next turn projects WITHOUT the pre-clear content.
	prov.queue(scriptedResp{text: "second answer"})

	_, err = r.Run(context.Background(), "classb",
		emit, []acp.ContentBlock{{Type: blockText, Text: "what was the codeword"}})
	if err != nil {
		t.Fatalf("post-clear turn: %v", err)
	}

	if prov.streamSawText(1, "pinecone") {
		t.Error("post-clear projection still carries pre-clear content (lean window not reset)")
	}

	sessAfter := r.sessionFor(context.Background(), "classb")
	if sessBefore.SessionID != sessAfter.SessionID {
		t.Error("/clear changed the session id (D-06 locks same-session)")
	}
}

// TestClassBDelegate pins the /resume + /compact delegation seams: unregistered
// machinery degrades LOUDLY with the named outcome recorded durably; a
// registered seam is invoked exactly once with the typed args.
//
//nolint:gocognit,funlen,paralleltest // two scenarios, one seam contract (sequenced state)
func TestClassBDelegate(t *testing.T) {
	t.Run("unregistered seams degrade loudly", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, nil)

		r.compactNowHook = nil
		r.resumeListHook = nil

		for _, prompt := range []string{"/resume", "/compact tidy up"} {
			frames, lines := classBRun(t, r, prompt)

			if got := prov.callCount(); got != 0 {
				t.Errorf("%s: provider Stream calls = %d; want 0", prompt, got)
			}

			var out strings.Builder

			for _, f := range frames {
				if f.kind == frameKindAgentChunk {
					out.WriteString(f.text)
				}
			}

			if !strings.Contains(out.String(), "not registered") {
				t.Errorf("%s output does not name the absent machinery:\n%s", prompt, out.String())
			}

			rec := localCommandLine(t, lines)
			if !strings.HasPrefix(rec.Expansion, "unavailable:") {
				t.Errorf("%s outcome = %q; want unavailable: ...", prompt, rec.Expansion)
			}
		}
	})

	t.Run("registered compact seam invoked with typed args", func(t *testing.T) {
		r, _, _ := newCommandRunner(t, nil)

		var gotArgs []string

		var calls int

		r.compactNowHook = func(_ context.Context, _ *session.Session, args string) (string, error) {
			calls++

			gotArgs = append(gotArgs, args)

			return "compacted 42% -> fresh window", nil
		}

		frames, lines := classBRun(t, r, "/compact focus on the parser")

		if calls != 1 {
			t.Fatalf("compact seam calls = %d; want 1", calls)
		}

		if len(gotArgs) != 1 || gotArgs[0] != "focus on the parser" {
			t.Errorf("seam args = %v; want the typed args verbatim", gotArgs)
		}

		var out strings.Builder

		for _, f := range frames {
			if f.kind == frameKindAgentChunk {
				out.WriteString(f.text)
			}
		}

		if !strings.Contains(out.String(), "compacted 42%") {
			t.Errorf("output missing the seam's result:\n%s", out.String())
		}

		rec := localCommandLine(t, lines)
		if rec.Args != "focus on the parser" {
			t.Errorf("local_command Args = %q; want verbatim (16-D-22)", rec.Args)
		}
	})
}

// countingRoundTripper counts requests and answers from a scripted fn.
type countingRoundTripper struct {
	mu      sync.Mutex
	calls   int
	respond func(*http.Request) (*http.Response, error)
}

func (c *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.calls++
	fn := c.respond
	c.mu.Unlock()

	return fn(req)
}

func (c *countingRoundTripper) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.calls
}

// TestClassBCost pins D-07's three-case matrix + the fallback math + the
// P-20-02 source-note prohibition: declared-capability live success (note
// names the endpoint), capability none (fallback note, ZERO network), and
// endpoint timeout (fallback note, bounded elapsed). The fallback's
// transcript×Pricing math is hand-computed; no output ever carries the
// credential value.
//
//nolint:gocognit,cyclop,funlen,paralleltest // three-case matrix, sequenced runner state
func TestClassBCost(t *testing.T) {
	t.Setenv("ZAI_API_KEY", "sk-cost-secret-fixture")

	pricing := modelrouting.Pricing{InputPerMToken: 1.0, OutputPerMToken: 2.0}

	// seedUsage appends known usage lines to the session's transcript:
	// 2M input + 1M output total => (2M*1.0 + 1M*2.0)/1e6 = $4.00.
	seedUsage := func(t *testing.T, r *Runner) {
		t.Helper()

		_, _ = classBRun(t, r, "/status") // creates the session

		sess := r.sessions["classb"]

		err := sess.Manager.AppendUsage("usage-seed-1", 1_500_000, 400_000)
		if err != nil {
			t.Fatalf("seed usage 1: %v", err)
		}

		err = sess.Manager.AppendUsage("usage-seed-2", 500_000, 600_000)
		if err != nil {
			t.Fatalf("seed usage 2: %v", err)
		}
	}

	armPricing := func(r *Runner, usageEndpoint string) {
		r.schedCfg.Models[fixtureModel] = modelrouting.ModelConfig{
			Provider: fixtureProvider, Pricing: pricing,
		}
		r.schedCfg.Providers[fixtureProvider] = modelrouting.ProviderConfig{
			APIKeyEnv: fixtureCredEnv, UsageEndpoint: usageEndpoint,
		}
	}

	out := func(t *testing.T, frames []commandFrame) string {
		t.Helper()

		var sb strings.Builder

		for _, f := range frames {
			if f.kind == frameKindAgentChunk {
				sb.WriteString(f.text)
			}
		}

		return sb.String()
	}

	t.Run("capability none goes straight to fallback, zero network", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, nil)

		rt := &countingRoundTripper{respond: func(*http.Request) (*http.Response, error) {
			return nil, errNotUsed
		}}
		r.costTransport = rt
		r.costFetchBudget = 50 * time.Millisecond
		armPricing(r, "")
		seedUsage(t, r)

		frames, lines := classBRun(t, r, "/cost")

		if got := prov.callCount(); got != 0 {
			t.Errorf("provider Stream calls = %d; want 0", got)
		}

		if rt.count() != 0 {
			t.Errorf("network attempts = %d; want 0 (capability none)", rt.count())
		}

		text := out(t, frames)
		if !strings.Contains(text, "transcript") || !strings.Contains(text, "cost table") {
			t.Errorf("fallback source note missing:\n%s", text)
		}

		if !strings.Contains(text, "$4.00") {
			t.Errorf("hand-computed fallback amount $4.00 missing:\n%s", text)
		}

		if strings.Contains(text, "sk-cost-secret-fixture") {
			t.Error("credential value leaked into /cost output (T-20-05)")
		}

		rec := localCommandLine(t, lines)
		if rec.Name != "cost" {
			t.Errorf("local_command Name = %q; want cost", rec.Name)
		}
	})

	t.Run("declared capability live success names the endpoint", func(t *testing.T) {
		r, _, _ := newCommandRunner(t, nil)

		rt := &countingRoundTripper{respond: func(*http.Request) (*http.Response, error) {
			body := `{"total_spent_usd": 12.5, "window": "30d"}`
			return &http.Response{
				StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{},
			}, nil
		}}
		r.costTransport = rt
		r.costFetchBudget = 50 * time.Millisecond
		armPricing(r, "https://api.example.test/usage")
		seedUsage(t, r)

		frames, _ := classBRun(t, r, "/cost")

		if rt.count() != 1 {
			t.Fatalf("network attempts = %d; want 1", rt.count())
		}

		text := out(t, frames)
		if !strings.Contains(text, "api.example.test/usage") {
			t.Errorf("live source note does not name the endpoint:\n%s", text)
		}

		if !strings.Contains(text, "12.5") {
			t.Errorf("live number missing from output:\n%s", text)
		}

		if strings.Contains(text, "sk-cost-secret-fixture") {
			t.Error("credential value leaked into /cost output (T-20-05)")
		}
	})

	t.Run("endpoint timeout falls back under the budget", func(t *testing.T) {
		r, _, _ := newCommandRunner(t, nil)

		rt := &countingRoundTripper{respond: func(req *http.Request) (*http.Response, error) {
			// The fake honors cancellation the way a real transport does —
			// a sleeping custom RoundTripper that ignores ctx would bypass
			// Go's post-roundtrip ctx check entirely.
			select {
			case <-time.After(300 * time.Millisecond):
				return &http.Response{
					StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{},
				}, nil
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}}
		r.costTransport = rt
		r.costFetchBudget = 50 * time.Millisecond
		armPricing(r, "https://api.example.test/usage")
		seedUsage(t, r)

		start := time.Now()

		frames, _ := classBRun(t, r, "/cost")

		elapsed := time.Since(start)
		if elapsed > 2*time.Second {
			t.Errorf("handler elapsed %v; want bounded near the %v budget", elapsed, r.costFetchBudget)
		}

		text := out(t, frames)
		if !strings.Contains(text, "transcript") {
			t.Errorf("timeout fallback note missing:\n%s", text)
		}
	})
}

// --- 20-04: skills + agents + /init as slash surfaces ---

// writeSkillFixture plants a skill directory under dir's project .claude.
func writeSkillFixture(t *testing.T, dir, name, frontmatter, body string) {
	t.Helper()

	p := filepath.Join(dir, ".claude", "skills", name, "SKILL.md")

	err := os.MkdirAll(filepath.Dir(p), 0o750)
	if err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}

	src := "---\n" + frontmatter + "---\n" + body

	err = os.WriteFile(p, []byte(src), 0o600)
	if err != nil {
		t.Fatalf("write SKILL.md %s: %v", name, err)
	}
}

// agentSlashFrames returns the agent-chunk output text of a Run.
func agentSlashFrames(frames []commandFrame) string {
	var sb strings.Builder

	for _, f := range frames {
		if f.kind == frameKindAgentChunk {
			sb.WriteString(f.text)
		}
	}

	return sb.String()
}

// TestSkillSlash pins SKLS-01: /<skill-name> expands the SKILL.md body with
// the locked Expand semantics ($ARGUMENTS/$1 substitution, append-under-
// heading, no-args without the appended block), multi-byte bodies survive
// byte-identically, empty bodies reject loudly with zero model calls, and
// user-invocable:false excludes the skill from the slash surface AND the
// advertisement (D-04) while keeping it in the registry (model surface).
//
//nolint:funlen,gocognit,gocyclo,cyclop,paralleltest // one battery, sequenced fixtures
func TestSkillSlash(t *testing.T) {
	t.Run("substitution and provenance", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, func(dir string) {
			writeSkillFixture(t, dir, "review-pr",
				"name: review-pr\ndescription: review a PR\n",
				"Review PR $1 focusing on:\n$ARGUMENTS\n")
		})
		prov.queue(scriptedResp{text: "ok", finish: stopEndTurn})

		emit := &tracerEmitter{}

		_, err := r.Run(context.Background(), "sk", emit,
			[]acp.ContentBlock{{Type: blockText, Text: "/review-pr 1234 the diff"}})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if got := prov.callCount(); got != 1 {
			t.Fatalf("provider calls = %d; want 1 (the expanded turn)", got)
		}

		text := firstUserMessageText(t, r, "sk")
		if !strings.Contains(text, "Review PR 1234") || !strings.Contains(text, "the diff") {
			t.Errorf("expanded prompt = %q; want $1 and $ARGUMENTS substituted", text)
		}

		// Provenance names the skill's SKILL.md origin.
		lines, _ := r.sessions["sk"].Manager.ReadAll()

		found := false

		for _, l := range lines {
			if l.Type == session.TypeCommandProvenance &&
				strings.Contains(l.CommandRef, filepath.Join("skills", "review-pr", "SKILL.md")) {
				found = true
			}
		}

		if !found {
			t.Error("no command_provenance line naming the skill's SKILL.md")
		}
	})

	t.Run("append-under-heading when body has no placeholder", func(t *testing.T) {
		r, _, _ := newCommandRunner(t, func(dir string) {
			writeSkillFixture(t, dir, "plain",
				"name: plain\ndescription: no placeholders\n", "Just do the thing.\n")
		})

		_, _ = r.Run(context.Background(), "sk1b", &tracerEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "/plain with focus"}})

		text := firstUserMessageText(t, r, "sk1b")
		if !strings.Contains(text, "Just do the thing.") ||
			!strings.Contains(text, "User arguments:") || !strings.Contains(text, "with focus") {
			t.Errorf("append-under-heading rule broken: %q", text)
		}
	})

	t.Run("no args omits the appended block", func(t *testing.T) {
		r, _, _ := newCommandRunner(t, func(dir string) {
			writeSkillFixture(t, dir, "noter",
				"name: noter\ndescription: note\n", "Body with $ARGUMENTS slot.\n")
		})

		emit := &tracerEmitter{}
		_ = emit

		_, _ = r.Run(context.Background(), "sk2", &tracerEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "/noter"}})

		text := firstUserMessageText(t, r, "sk2")
		if strings.Contains(text, "User arguments:") {
			t.Errorf("no-args expansion appended the arguments block: %q", text)
		}
	})

	t.Run("multi-byte body survives", func(t *testing.T) {
		r, _, _ := newCommandRunner(t, func(dir string) {
			writeSkillFixture(t, dir, "unicode",
				"name: unicode\ndescription: мультибайт\n",
				"Инструкция: проверь «кавычки» и — тире.\n")
		})

		_, _ = r.Run(context.Background(), "sk3", &tracerEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "/unicode"}})

		text := firstUserMessageText(t, r, "sk3")
		if !strings.Contains(text, "Инструкция: проверь «кавычки» и — тире.") {
			t.Errorf("multi-byte body corrupted: %q", text)
		}
	})

	t.Run("empty body rejects loudly, zero model calls", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, func(dir string) {
			writeSkillFixture(t, dir, "hollow", "name: hollow\ndescription: nothing\n", "")
		})

		emit := &tracerEmitter{}

		stop, err := r.Run(context.Background(), "sk4", emit,
			[]acp.ContentBlock{{Type: blockText, Text: "/hollow extra"}})
		if err != nil || stop != stopEndTurn {
			t.Fatalf("Run: stop=%q err=%v", stop, err)
		}

		if got := prov.callCount(); got != 0 {
			t.Errorf("provider calls = %d; want 0 (empty prompt never reaches the model)", got)
		}

		if out := agentSlashFrames(emit.snapshot()); !strings.Contains(out, "hollow") {
			t.Errorf("error output does not name the skill: %q", out)
		}

		lines, _ := r.sessions["sk4"].Manager.ReadAll()

		var rec *session.Line

		for _, l := range lines {
			if l.Type == session.TypeLocalCommand {
				rec = &l
			}
		}

		if rec == nil || rec.Name != "hollow" || !strings.HasPrefix(rec.Expansion, "failed:") {
			t.Errorf("failed record missing: %+v", rec)
		}
	})

	t.Run("user-invocable false excluded from slash and advertisement", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, func(dir string) {
			writeSkillFixture(t, dir, "hidden",
				"name: hidden\ndescription: secret\nuser-invocable: false\n", "Hidden body.\n")
		})

		emit := &tracerEmitter{}

		_, _ = r.Run(context.Background(), "sk5", emit,
			[]acp.ContentBlock{{Type: blockText, Text: "/hidden"}})

		if got := prov.callCount(); got != 1 {
			t.Fatalf("provider calls = %d; want 1 (plain-text fallthrough — the invocation is ordinary text)", got)
		}

		text := firstUserMessageText(t, r, "sk5")
		if text != "/hidden" {
			t.Errorf("fallthrough text = %q; want the raw invocation (plain text)", text)
		}

		for _, f := range r.CommandAdvertisement() {
			if f.Name == "hidden" {
				t.Error("advertisement lists the user-invocable:false skill (D-04)")
			}
		}

		// The registry keeps it (the MODEL surface is untouched).
		if _, ok := r.reg.Skills["hidden"]; !ok {
			t.Error("registry lost the excluded skill (model surface must keep it)")
		}
	})
}

// writeAgentFixture plants a .claude/agents definition (the BMad layout the
// loader already walks — SKLS-02's discovery source).
func writeAgentFixture(t *testing.T, dir, name, frontmatter, body string) {
	t.Helper()

	p := filepath.Join(dir, ".claude", "agents", name+".md")

	err := os.MkdirAll(filepath.Dir(p), 0o750)
	if err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}

	err = os.WriteFile(p, []byte("---\n"+frontmatter+"---\n"+body), 0o600)
	if err != nil {
		t.Fatalf("write agent %s: %v", name, err)
	}
}

// TestAgentSlash pins SKLS-02 + D-03: /<agent-name> dispatches the subagent
// (args become the prompt; Prompt/Tools apply; the result streams; the turn
// ends end_turn; NO parent-model turn), the dispatch line carries ResolvedModel
// (20-03 integration), unknown names fall through as plain text, and a
// colliding skill wins the slash while the agent stays dispatchable via the
// Agent tool surface (D-02).
//
//nolint:funlen,gocognit,gocyclo,cyclop,paralleltest // one battery, sequenced fixtures
func TestAgentSlash(t *testing.T) {
	t.Run("dispatch with prompt tools and resolvedModel", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, func(dir string) {
			writeAgentFixture(t, dir, "scout",
				"name: scout\ndescription: locates code\nmodel: "+fixtureModelLite+"\ntools: [Read, Grep]\n",
				"You are the scout agent. Locate code precisely.")
		})
		prov.queue(scriptedResp{text: "found it at main.go:42", finish: stopEndTurn})

		emit := &tracerEmitter{}

		stop, err := r.Run(context.Background(), "ag", emit,
			[]acp.ContentBlock{{Type: blockText, Text: "/scout find the entrypoint"}})
		if err != nil || stop != stopEndTurn {
			t.Fatalf("Run: stop=%q err=%v", stop, err)
		}

		// Exactly ONE provider call — the SUBAGENT's (zero parent-model turns).
		if got := prov.callCount(); got != 1 {
			t.Fatalf("provider calls = %d; want 1 (the subagent only)", got)
		}

		// The subagent's prompt is the args; its system context carries the
		// agent's Prompt; the model resolved from the frontmatter.
		if m := prov.lastStreamModel(); m != fixtureModelLite {
			t.Errorf("subagent model = %q; want the frontmatter slug %q", m, fixtureModelLite)
		}

		if !prov.streamSawText(0, "find the entrypoint") {
			t.Error("subagent prompt does not carry the typed args")
		}

		// (The agent Prompt rides the subagent profile's System block — not
		// observable through the message-content capture; its application is
		// pinned by the 12-02 subagentProfile tests + the RestrictedTools
		// assertion above proves the agentDef reached the dispatch.)

		// The client saw the streamed result inside this turn.
		if out := agentSlashFrames(emit.snapshot()); !strings.Contains(out, "found it at main.go:42") {
			t.Errorf("subagent result not streamed into the turn: %q", out)
		}

		// D-16 durable integration: the dispatch line carries ResolvedModel.
		lines, _ := r.sessions["ag"].Manager.ReadAll()

		var dispatch *session.Line

		for _, l := range lines {
			if l.Type == session.TypeSubagentDispatch {
				dispatch = &l
			}
		}

		if dispatch == nil || dispatch.ResolvedModel != fixtureModelLite {
			t.Errorf("dispatch line ResolvedModel = %+v; want %q", dispatch, fixtureModelLite)
		}

		// The slash surface's own local_command record.
		var rec *session.Line

		for _, l := range lines {
			if l.Type == session.TypeLocalCommand && l.Name == "scout" {
				rec = &l
			}
		}

		if rec == nil || rec.Args != "find the entrypoint" {
			t.Errorf("agent-slash local_command record = %+v", rec)
		}
	})

	t.Run("unknown name falls through as plain text", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, nil)
		prov.queue(scriptedResp{text: "ok", finish: stopEndTurn})

		_, _ = r.Run(context.Background(), "ag2", &tracerEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "/totally-unknown-name hi"}})

		if got := prov.callCount(); got != 1 {
			t.Fatalf("provider calls = %d; want 1 (ordinary turn on the raw text)", got)
		}

		if text := firstUserMessageText(t, r, "ag2"); text != "/totally-unknown-name hi" {
			t.Errorf("fallthrough text = %q; want the raw invocation", text)
		}
	})

	t.Run("colliding skill wins slash; agent dispatchable via Agent tool", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, func(dir string) {
			writeSkillFixture(t, dir, "twin", "name: twin\ndescription: the skill\n", "Skill body wins.\n")
			writeAgentFixture(t, dir, "twin",
				"name: twin\ndescription: the agent\n", "Twin agent prompt.")
		})
		prov.queue(scriptedResp{text: "expanded", finish: stopEndTurn})

		// The SLASH surface resolves to the skill (D-02 chain order).
		_, _ = r.Run(context.Background(), "ag3", &tracerEmitter{},
			[]acp.ContentBlock{{Type: blockText, Text: "/twin via slash"}})

		if text := firstUserMessageText(t, r, "ag3"); !strings.Contains(text, "Skill body wins.") {
			t.Errorf("slash winner = %q; want the skill body (D-02 skills > agents)", text)
		}

		// The AGENT TOOL surface still dispatches the agentDef (native
		// surface): the registry keeps the loser and session dispatch resolves
		// it through the SubagentTypes fallback (the chain's loser-miss path).
		if def, ok := r.reg.Agents["twin"]; !ok || def.Prompt != "Twin agent prompt." {
			t.Fatalf("registry lost the colliding agent: %+v", def)
		}
	})
}

// TestInitExpansion pins CMDS-03: /init expands as a NORMAL user turn through
// the untouched expansion seam (model called once, provenance names
// builtin:init, args substitute), works identically engine-on and engine-off
// (one resolution path), is advertised, and shadows a discovered
// commands/init.md with the D-01 warning.
//
//nolint:funlen,cyclop,paralleltest // one battery, sequenced fixtures
func TestInitExpansion(t *testing.T) {
	assertExpansion := func(t *testing.T, r *Runner, prov *scriptedACPProvider, sessionID string) {
		t.Helper()

		emit := &tracerEmitter{}

		_, err := r.Run(context.Background(), sessionID, emit,
			[]acp.ContentBlock{{Type: blockText, Text: "/init focus on the test layout"}})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if got := prov.callCount(); got != 1 {
			t.Fatalf("provider calls = %d; want exactly 1 (a normal user turn)", got)
		}

		text := firstUserMessageText(t, r, sessionID)
		if !strings.Contains(text, "CLAUDE.md") || !strings.Contains(text, "focus on the test layout") {
			t.Errorf("init body not expanded with args: %.120s", text)
		}

		lines, _ := r.sessions[sessionID].Manager.ReadAll()

		provenance := false

		for _, l := range lines {
			if l.Type == session.TypeCommandProvenance && l.CommandRef == "builtin:init" {
				provenance = true
			}
		}

		if !provenance {
			t.Error("no provenance line naming builtin:init")
		}
	}

	t.Run("engine-off expansion", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, nil)
		prov.queue(scriptedResp{text: "done", finish: stopEndTurn})

		assertExpansion(t, r, prov, "init-off")
	})

	t.Run("engine-on parity", func(t *testing.T) {
		r, prov, _ := newCommandRunner(t, nil)

		err := r.SetupEngine()
		if err != nil {
			t.Fatalf("SetupEngine: %v", err)
		}

		prov.queue(scriptedResp{text: "impl complete", finish: stopEndTurn})

		assertExpansion(t, r, prov, "init-on")
	})

	t.Run("advertised and shadows discovered init.md", func(t *testing.T) {
		r, _, stderr := newCommandRunner(t, func(dir string) {
			writeDiscoveredCommand(t, dir, "init", "---\ndescription: fake init\n---\nFake init body.\n")
		})

		advertised := false

		for _, f := range r.CommandAdvertisement() {
			if f.Name == nameInit {
				advertised = true
			}
		}

		if !advertised {
			t.Error("advertisement missing init")
		}

		if !strings.Contains(stderr.String(), nameInit) {
			t.Errorf("discovered commands/init.md not warned as shadowed: %s", stderr.String())
		}

		// And the discovered body never fires: /init expands the BUILTIN body.
		r2prov := scriptedACPProvider{}
		r2prov.queue(scriptedResp{text: "ok", finish: stopEndTurn})

		emit := &tracerEmitter{}

		_, _ = r.Run(context.Background(), "init-shadow", emit,
			[]acp.ContentBlock{{Type: blockText, Text: "/init"}})

		text := firstUserMessageText(t, r, "init-shadow")
		if strings.Contains(text, "Fake init body.") {
			t.Error("the discovered init.md fired — D-01 reservation broken")
		}

		if !strings.Contains(text, "CLAUDE.md") {
			t.Errorf("builtin init body did not expand: %.80s", text)
		}
	})
}

// --- 23-05: the /undo class-B battery (SEEDG-03 — D-11 walk + D-05 shape) ---

// undoCanary is the battery's workspace canary file (fingerprinted across
// restores for the byte-identity assertions).
const undoCanary = "undo-canary.txt"

// seedUndoSnap writes content to the canary and snapshots it under id through
// the SAME workspace store the Runner opens (the direct-store drive — the
// walk fixture needs exact control over the checkpoint stack).
func seedUndoSnap(t *testing.T, workDir, sessionID, id, content string) {
	t.Helper()

	writeGuardFile(t, filepath.Join(workDir, undoCanary), content)

	st, err := checkpoint.Open(workDir)
	if err != nil {
		t.Fatalf("checkpoint.Open: %v", err)
	}

	if serr := st.Snapshot(context.Background(), sessionID, id); serr != nil {
		t.Fatalf("Snapshot %s: %v", id, serr)
	}
}

// undoRun drives one /undo invocation against a chosen session id (classBRun
// with the session as a parameter — the walk battery needs distinct ids) and
// returns the captured frames + transcript lines.
func undoRun(t *testing.T, r *Runner, sessionID, prompt string) ([]commandFrame, []session.Line) {
	t.Helper()

	emit := &tracerEmitter{}

	stop, err := r.Run(context.Background(), sessionID, emit, []acp.ContentBlock{{Type: blockText, Text: prompt}})
	if err != nil {
		t.Fatalf("Run(%q): %v", prompt, err)
	}

	if stop != stopEndTurn {
		t.Fatalf("Run(%q) stop = %q; want end_turn", prompt, stop)
	}

	lines, rerr := r.sessions[sessionID].Manager.ReadAll()
	if rerr != nil {
		t.Fatalf("ReadAll: %v", rerr)
	}

	return emit.snapshot(), lines
}

// undoOutputText joins the agent_message_chunk frames (the D-05 output lens).
func undoOutputText(frames []commandFrame) string {
	var sb strings.Builder

	for _, f := range frames {
		if f.kind == frameKindAgentChunk {
			sb.WriteString(f.text)
		}
	}

	return sb.String()
}

// undoCanaryContent reads the canary's current content (the landed-state lens).
func undoCanaryContent(t *testing.T, workDir string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(workDir, undoCanary))
	if err != nil {
		t.Fatalf("read canary: %v", err)
	}

	return string(data)
}

// TestClassBUndoIdle pins the idle-path contract (Task 1): /undo with an idle
// session and existing checkpoints restores the NEWEST checkpoint of THIS
// session with zero provider Stream calls, the D-05 output shape (verbatim
// echo, output naming the restored id + the pre-restore snapshot id), and a
// durable local_command record with args verbatim. The second /undo restores
// the state before the first (undo-of-undo via the D-09 pre-restore family).
func TestClassBUndoIdle(t *testing.T) { //nolint:funlen // two-invocation walk scenario
	t.Parallel()

	r, prov, _ := newCommandRunner(t, nil)

	seedUndoSnap(t, r.workDir, "classb", "classb-turn-001", "state-A\n")
	seedUndoSnap(t, r.workDir, "classb", "classb-turn-002", "state-B\n")
	writeGuardFile(t, filepath.Join(r.workDir, undoCanary), "state-C\n") // current, unsnapshotted

	frames, lines := undoRun(t, r, "classb", "/undo")

	if got := prov.callCount(); got != 0 {
		t.Fatalf("provider Stream calls = %d; want 0 (zero model turns, SEEDG-03)", got)
	}

	var echoes []commandFrame

	for _, f := range frames {
		if f.kind == frameKindUserEcho {
			echoes = append(echoes, f)
		}
	}

	if len(echoes) != 1 || echoes[0].text != "/undo" {
		t.Fatalf("echo frames = %+v; want exactly one verbatim /undo echo (D-05)", echoes)
	}

	out := undoOutputText(frames)
	if !strings.Contains(out, "classb-turn-002") {
		t.Errorf("output missing the restored id classb-turn-002:\n%s", out)
	}

	if !strings.Contains(out, "classb-pre-001") {
		t.Errorf("output missing the pre-restore snapshot id classb-pre-001:\n%s", out)
	}

	if got := undoCanaryContent(t, r.workDir); got != "state-B\n" {
		t.Fatalf("canary after /undo = %q; want the newest checkpoint's state-B", got)
	}

	rec := localCommandLine(t, lines)
	if rec.Name != "undo" || rec.Args != "" || rec.Expansion != "ok" {
		t.Errorf("local_command = {name:%q args:%q outcome:%q}; want {undo \"\" ok}", rec.Name, rec.Args, rec.Expansion)
	}

	if len(rec.SourceChain) != 1 || rec.SourceChain[0] != sourceChainBuiltin {
		t.Errorf("local_command SourceChain = %v; want [builtin]", rec.SourceChain)
	}

	// Undo-of-undo (D-09/D-11): every restore snapshots current state first,
	// so the second /undo restores the pre-undo state via that snapshot.
	frames2, _ := undoRun(t, r, "classb", "/undo")

	out2 := undoOutputText(frames2)
	if !strings.Contains(out2, "classb-pre-001") {
		t.Errorf("second /undo output missing the walk target classb-pre-001:\n%s", out2)
	}

	if got := undoCanaryContent(t, r.workDir); got != "state-C\n" {
		t.Fatalf("canary after undo-of-undo = %q; want the pre-undo state-C", got)
	}
}

// TestUndoWalk pins the D-11 stack walk over the recency-ordered checkpoint
// stack (both id families participate): three checkpoints + two /undo calls
// take two stack steps (the newest turn checkpoint, then the pre-restore
// snapshot the first /undo minted — the walk never re-targets an entry), and
// /undo 2 jumps two entries in one invocation on a fresh fixture. Every
// landed state is treeMap-byte-identical to the seeded state.
func TestUndoWalk(t *testing.T) { //nolint:funlen // three-scenario walk battery
	t.Parallel()

	r, prov, _ := newCommandRunner(t, nil)

	seedUndoSnap(t, r.workDir, "walksess", "walksess-turn-001", "state-A\n")
	treeA := ckptLiveTree(t, r.workDir)
	seedUndoSnap(t, r.workDir, "walksess", "walksess-turn-002", "state-B\n")
	treeB := ckptLiveTree(t, r.workDir)
	seedUndoSnap(t, r.workDir, "walksess", "walksess-turn-003", "state-C\n")
	treeC := ckptLiveTree(t, r.workDir)
	writeGuardFile(t, filepath.Join(r.workDir, undoCanary), "state-D\n")
	treeD := ckptLiveTree(t, r.workDir)

	// Step 1: the newest checkpoint (turn-003 = state-C), byte-identical.
	frames1, _ := undoRun(t, r, "walksess", "/undo")

	if got := ckptLiveTree(t, r.workDir); !reflect.DeepEqual(got, treeC) {
		t.Errorf("tree after /undo #1 is not byte-identical to the turn-003 state")
	}

	if !strings.Contains(undoOutputText(frames1), "walksess-turn-003") {
		t.Errorf("/undo #1 did not name walksess-turn-003 as the restored id")
	}

	// Step 2: the pre-restore snapshot /undo #1 minted — the pre-undo state
	// (undo-of-undo), byte-identical to the pre-walk tree.
	frames2, _ := undoRun(t, r, "walksess", "/undo")

	if got := ckptLiveTree(t, r.workDir); !reflect.DeepEqual(got, treeD) {
		t.Errorf("tree after /undo #2 is not byte-identical to the pre-undo state (undo-of-undo broken)")
	}

	if !strings.Contains(undoOutputText(frames2), "walksess-pre-001") {
		t.Errorf("/undo #2 did not target the minted pre-restore snapshot walksess-pre-001")
	}

	if got := prov.callCount(); got != 0 {
		t.Fatalf("provider Stream calls across the walk = %d; want 0", got)
	}

	// The N-jump on a FRESH fixture: /undo 2 lands the second-newest entry.
	r2, prov2, _ := newCommandRunner(t, nil)

	seedUndoSnap(t, r2.workDir, "jumpsess", "jumpsess-turn-001", "state-A\n")
	seedUndoSnap(t, r2.workDir, "jumpsess", "jumpsess-turn-002", "state-B\n")
	seedUndoSnap(t, r2.workDir, "jumpsess", "jumpsess-turn-003", "state-C\n")
	writeGuardFile(t, filepath.Join(r2.workDir, undoCanary), "state-D\n")

	undoRun(t, r2, "jumpsess", "/undo 2")

	if got := undoCanaryContent(t, r2.workDir); got != "state-B\n" {
		t.Errorf("/undo 2 landed %q; want the second-newest checkpoint's state-B", got)
	}

	if got := ckptLiveTree(t, r2.workDir); !reflect.DeepEqual(got, treeB) {
		t.Errorf("tree after /undo 2 is not byte-identical to the turn-002 state")
	}

	if got := prov2.callCount(); got != 0 {
		t.Errorf("provider Stream calls for the N-jump = %d; want 0", got)
	}

	_ = treeA // captured for the reader: the walk's depth-3 floor
}

// TestClassBUndoEdges pins the CONTEXT-discretion edge table: /undo 0 and
// negative depths behave as 1; non-numeric args produce the D-05 error text
// with a failed local_command record and zero provider calls; beyond-depth
// clamps to the oldest entry; an empty store is a LOUD nothing-to-restore.
func TestClassBUndoEdges(t *testing.T) {
	t.Parallel()

	t.Run("zero depth behaves as one", func(t *testing.T) {
		t.Parallel()

		r, prov, _ := newCommandRunner(t, nil)

		seedUndoSnap(t, r.workDir, "edgesess", "edgesess-turn-001", "state-A\n")
		seedUndoSnap(t, r.workDir, "edgesess", "edgesess-turn-002", "state-B\n")
		writeGuardFile(t, filepath.Join(r.workDir, undoCanary), "state-C\n")

		undoRun(t, r, "edgesess", "/undo 0")

		if got := undoCanaryContent(t, r.workDir); got != "state-B\n" {
			t.Errorf("/undo 0 landed %q; want the newest checkpoint state-B", got)
		}

		if got := prov.callCount(); got != 0 {
			t.Errorf("provider calls = %d; want 0", got)
		}
	})

	t.Run("negative depth behaves as one", func(t *testing.T) {
		t.Parallel()

		r, prov, _ := newCommandRunner(t, nil)

		seedUndoSnap(t, r.workDir, "edgesess", "edgesess-turn-001", "state-A\n")
		seedUndoSnap(t, r.workDir, "edgesess", "edgesess-turn-002", "state-B\n")
		writeGuardFile(t, filepath.Join(r.workDir, undoCanary), "state-C\n")

		undoRun(t, r, "edgesess", "/undo -3")

		if got := undoCanaryContent(t, r.workDir); got != "state-B\n" {
			t.Errorf("/undo -3 landed %q; want the newest checkpoint state-B", got)
		}

		if got := prov.callCount(); got != 0 {
			t.Errorf("provider calls = %d; want 0", got)
		}
	})

	t.Run("non-numeric args are a D-05 error", func(t *testing.T) {
		t.Parallel()

		r, prov, _ := newCommandRunner(t, nil)

		seedUndoSnap(t, r.workDir, "edgesess", "edgesess-turn-001", "state-A\n")
		writeGuardFile(t, filepath.Join(r.workDir, undoCanary), "state-C\n")

		frames, lines := undoRun(t, r, "edgesess", "/undo banana")

		if got := prov.callCount(); got != 0 {
			t.Fatalf("provider calls = %d; want 0 (never a model turn)", got)
		}

		if out := undoOutputText(frames); !strings.Contains(out, "invalid depth") {
			t.Errorf("output missing the invalid-depth error text:\n%s", out)
		}

		rec := localCommandLine(t, lines)
		if rec.Args != "banana" {
			t.Errorf("local_command Args = %q; want the typed args verbatim", rec.Args)
		}

		if !strings.HasPrefix(rec.Expansion, "failed:") {
			t.Errorf("local_command outcome = %q; want a failed: outcome", rec.Expansion)
		}

		if got := undoCanaryContent(t, r.workDir); got != "state-C\n" {
			t.Errorf("a failed /undo mutated the workspace: %q", got)
		}
	})

	t.Run("beyond-depth clamps to the oldest", func(t *testing.T) {
		t.Parallel()

		r, _, _ := newCommandRunner(t, nil)

		seedUndoSnap(t, r.workDir, "edgesess", "edgesess-turn-001", "state-A\n")
		seedUndoSnap(t, r.workDir, "edgesess", "edgesess-turn-002", "state-B\n")
		seedUndoSnap(t, r.workDir, "edgesess", "edgesess-turn-003", "state-C\n")
		writeGuardFile(t, filepath.Join(r.workDir, undoCanary), "state-D\n")

		undoRun(t, r, "edgesess", "/undo 99")

		if got := undoCanaryContent(t, r.workDir); got != "state-A\n" {
			t.Errorf("/undo 99 landed %q; want the oldest checkpoint state-A (clamped)", got)
		}
	})

	t.Run("empty store is a loud nothing-to-restore", func(t *testing.T) {
		t.Parallel()

		r, prov, _ := newCommandRunner(t, nil)

		writeGuardFile(t, filepath.Join(r.workDir, undoCanary), "state-D\n")

		frames, lines := undoRun(t, r, "edgesess", "/undo")

		if got := prov.callCount(); got != 0 {
			t.Fatalf("provider calls = %d; want 0", got)
		}

		if out := undoOutputText(frames); !strings.Contains(out, "nothing to restore") {
			t.Errorf("output missing the loud nothing-to-restore text:\n%s", out)
		}

		rec := localCommandLine(t, lines)
		if rec.Name != "undo" || rec.Expansion != "ok" {
			t.Errorf("local_command = {name:%q outcome:%q}; want {undo ok}", rec.Name, rec.Expansion)
		}

		if got := undoCanaryContent(t, r.workDir); got != "state-D\n" {
			t.Errorf("an empty-store /undo mutated the workspace: %q", got)
		}
	})
}

// TestClassBUndoCrossSession pins the session-scoping: the walk targets ONLY
// the current session's entries — another session's checkpoints in the same
// workspace are invisible to it.
func TestClassBUndoCrossSession(t *testing.T) {
	t.Parallel()

	r, prov, _ := newCommandRunner(t, nil)

	seedUndoSnap(t, r.workDir, "sessA", "sessA-turn-001", "state-A\n")
	seedUndoSnap(t, r.workDir, "sessA", "sessA-turn-002", "state-B\n")
	writeGuardFile(t, filepath.Join(r.workDir, undoCanary), "state-C\n")

	// Session B has no entries: loud nothing-to-restore, never A's stack.
	frames, lines := undoRun(t, r, "sessB", "/undo")

	if out := undoOutputText(frames); !strings.Contains(out, "nothing to restore") {
		t.Errorf("output missing the nothing-to-restore text:\n%s", out)
	}

	if got := undoCanaryContent(t, r.workDir); got != "state-C\n" {
		t.Errorf("cross-session /undo restored another session's checkpoint: %q", got)
	}

	if got := prov.callCount(); got != 0 {
		t.Errorf("provider calls = %d; want 0", got)
	}

	rec := localCommandLine(t, lines)
	if rec.Name != "undo" {
		t.Errorf("local_command Name = %q; want undo", rec.Name)
	}
}

// TestClassBUndoDegradedStore pins Pitfall 9's /undo leg: a session whose
// workspace store could not open (nil Runner store) reports /undo UNAVAILABLE
// loudly through the D-05 shape — never a silent no-op, never a wedge.
func TestClassBUndoDegradedStore(t *testing.T) {
	t.Parallel()

	r, prov, _ := newCommandRunner(t, nil)

	// Poison the store path BEFORE the first session lands: every open fails.
	poison := filepath.Join(r.workDir, ".ass-guard", "checkpoints")
	if err := os.MkdirAll(filepath.Dir(poison), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(poison, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("poison: %v", err)
	}

	writeGuardFile(t, filepath.Join(r.workDir, undoCanary), "state-D\n")

	frames, lines := undoRun(t, r, "degraded", "/undo")

	if got := prov.callCount(); got != 0 {
		t.Fatalf("provider calls = %d; want 0", got)
	}

	if out := undoOutputText(frames); !strings.Contains(out, "checkpoint store disabled") {
		t.Errorf("output missing the unavailable/store-disabled text:\n%s", out)
	}

	rec := localCommandLine(t, lines)
	if !strings.HasPrefix(rec.Expansion, "unavailable:") {
		t.Errorf("local_command outcome = %q; want an unavailable: outcome", rec.Expansion)
	}

	if got := undoCanaryContent(t, r.workDir); got != "state-D\n" {
		t.Errorf("a degraded-store /undo mutated the workspace: %q", got)
	}
}
