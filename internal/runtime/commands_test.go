package runtime //nolint:testpackage // internal package test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/acp"
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

	ad := c.advertisement()

	for _, f := range ad {
		if f.Name == nameModel || f.Name == nameCompact {
			t.Errorf("advertisement lists shadowed name %q; winners-only (D-04)", f.Name)
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
