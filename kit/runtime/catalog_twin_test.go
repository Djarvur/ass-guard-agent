package runtime //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/openspec"
	"github.com/Djarvur/ass-guard-agent/kit/mcp"
	"github.com/Djarvur/ass-guard-agent/kit/session"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// testCatalog is the in-package twin of acpserve's catalogAdapter (the 25-05
// engine_setup_test precedent, at the catalog's scale): white-box batteries
// cannot import internal/acpserve (it imports this package — test-cycle), so
// the twin mirrors the adapter's exact construction + degradation arms +
// mappings over the same ecosystem calls. Test files may import the app
// packages (the documented D-19 ride-along exemption); PRODUCTION kit code
// may not. The plant* helpers are the twin's extras (fixture mutation the
// production adapter never needs — tests poke the registry then rebuild the
// chain, the old r.reg discipline).
type testCatalog struct {
	mu         sync.Mutex
	workDir    string
	reg        ecosys.Registry
	servers    []ecosys.ServerConfig
	mutability map[string]string
}

// newTestCatalog is the twin of loadCommandCatalog: real discovery over
// workDir + the mutability table, with the SAME degradation arms (an empty
// catalog on discovery failure; boundaries off on config failure).
func newTestCatalog(workDir string) *testCatalog {
	reg, servers, err := ecosys.Discover(workDir)
	if err != nil {
		log.Printf("ass-guard: command registry load failed (continuing without slash expansion): %v", err)

		return &testCatalog{workDir: workDir}
	}

	mutability := map[string]string(nil)

	oscfg, cerr := openspec.DefaultConfig()
	if cerr != nil {
		log.Printf("ass-guard: openspec config load failed (continuing without command boundaries): %v", cerr)
	} else {
		mutability = oscfg.CommandMutability
	}

	return &testCatalog{workDir: workDir, reg: reg, servers: servers, mutability: mutability}
}

func (t *testCatalog) Rediscover() error {
	reg, servers, err := ecosys.Discover(t.workDir)
	if err != nil {
		return err //nolint:wrapcheck // rescanAndSwap renders the degrade
	}

	t.mu.Lock()
	t.reg = reg
	t.servers = servers
	t.mu.Unlock()

	return nil
}

func (t *testCatalog) Entries() []CatalogEntry {
	reg, _ := t.snapshot()

	out := make([]CatalogEntry, 0, len(reg.Skills)+len(reg.Agents)+len(reg.Commands))

	for _, sk := range reg.AllSkills() {
		if sk.UserInvocable != nil && !*sk.UserInvocable {
			continue
		}

		out = append(out, CatalogEntry{
			Kind: EntryKindSkill, Name: sk.Name,
			Description: sk.Description, Path: sk.Path,
		})
	}

	for _, ag := range reg.AllAgents() {
		def := twinAgentDef(&ag)

		out = append(out, CatalogEntry{
			Kind: EntryKindAgent, Name: ag.Name,
			Description: ag.Description, Path: ag.Path, Agent: &def,
		})
	}

	for _, cmd := range reg.AllCommands() {
		out = append(out, CatalogEntry{
			Kind: EntryKindFile, Name: cmd.Name,
			Description: cmd.Description, InputHint: cmd.ArgumentHint, Path: cmd.Path,
		})
	}

	return out
}

func (t *testCatalog) LookupCommand(kind, key string) (Command, bool) {
	reg, _ := t.snapshot()

	switch kind {
	case string(EntryKindFile):
		cmd, ok := reg.Commands[key]
		if !ok {
			return Command{}, false
		}

		return Command{Name: cmd.Name, Description: cmd.Description, Body: cmd.Body, Path: cmd.Path}, true
	case string(EntryKindSkill):
		sk, ok := reg.Skills[key]
		if !ok {
			return Command{}, false
		}

		body, ok := ecosys.ResolveSkill(reg, key)
		if !ok {
			return Command{}, false
		}

		return Command{Name: sk.Name, Description: sk.Description, Body: body, Path: sk.Path}, true
	default:
		return Command{}, false
	}
}

func (t *testCatalog) SkillBody(key string) (string, bool) {
	reg, _ := t.snapshot()

	return ecosys.ResolveSkill(reg, key)
}

func (t *testCatalog) Mutating(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.mutability[key] == openspec.MutabilityMutating
}

func (t *testCatalog) Agents() map[string]session.AgentDef {
	reg, _ := t.snapshot()

	out := make(map[string]session.AgentDef, len(reg.Agents))
	for name, ag := range reg.Agents {
		out[name] = twinAgentDef(&ag)
	}

	return out
}

func (t *testCatalog) Hooks(sessionID, dir, transcriptPath string) session.Hooks { //nolint:ireturn // seam shape
	reg, _ := t.snapshot()

	return twinHooks{runner: ecosys.NewHookRunner(reg.Hooks, sessionID, dir, transcriptPath)}
}

func (t *testCatalog) SkillExecute() toolcat.Stub {
	reg, _ := t.snapshot()

	return ecosys.SkillExecute(reg)
}

func (t *testCatalog) SkillListing() string {
	reg, _ := t.snapshot()

	return ecosys.SkillListing(reg)
}

func (t *testCatalog) AgentListing() string {
	reg, _ := t.snapshot()

	return ecosys.AgentListing(reg)
}

func (t *testCatalog) MemoryInjection(dir string) string {
	return ecosys.MemoryInjection(dir)
}

func (t *testCatalog) MCPLayers() []mcp.ServerConfig {
	_, servers := t.snapshot()

	if len(servers) == 0 {
		return nil
	}

	out := make([]mcp.ServerConfig, 0, len(servers))
	for _, sc := range servers {
		out = append(out, mcp.ServerConfig{
			Name: sc.Name, Command: sc.Command, Args: sc.Args, Env: sc.Env, Cwd: sc.Cwd,
		})
	}

	return out
}

func (t *testCatalog) MemoryFiles(dir string) []MemoryFile {
	files := ecosys.DiscoverMemoryFiles(dir)
	if len(files) == 0 {
		return nil
	}

	out := make([]MemoryFile, 0, len(files))
	for _, f := range files {
		out = append(out, MemoryFile{Path: f.Path, OrigBytes: f.OrigBytes, SkipNote: f.SkipNote})
	}

	return out
}

func (t *testCatalog) Counts() CatalogCounts {
	reg, _ := t.snapshot()

	return CatalogCounts{
		Commands: len(reg.Commands), Skills: len(reg.Skills), Agents: len(reg.Agents),
	}
}

// snapshot returns the live registry + MCP set.
func (t *testCatalog) snapshot() (ecosys.Registry, []ecosys.ServerConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.reg, t.servers
}

// plantAgent registers one agent definition in the twin's registry (the
// late-arrival fixtures; the caller rebuilds the chain afterwards).
func (t *testCatalog) plantAgent(a *ecosys.Agent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.reg.Agents == nil {
		t.reg.Agents = make(map[string]ecosys.Agent)
	}

	t.reg.Agents[a.Name] = *a
}

// plantSkills replaces the twin's skill table (the help-rebuild fixture).
func (t *testCatalog) plantSkills(skills map[string]ecosys.Skill) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.reg.Skills = skills
}

// skill returns one registry skill row (the model-surface assertion lens).
func (t *testCatalog) skill(name string) (ecosys.Skill, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	sk, ok := t.reg.Skills[name]

	return sk, ok
}

// agent returns one registry agent row (the native-surface assertion lens).
func (t *testCatalog) agent(name string) (ecosys.Agent, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ag, ok := t.reg.Agents[name]

	return ag, ok
}

// twinAgentDef mirrors the adapter's agent mapping (the twin's copy).
func twinAgentDef(a *ecosys.Agent) session.AgentDef {
	return session.AgentDef{
		Name: a.Name, Description: a.Description,
		Tools: a.Tools, Model: a.Model, Prompt: a.Prompt,
	}
}

// twinHooks mirrors the adapter's hook bridging (the twin's copy).
type twinHooks struct{ runner *ecosys.HookRunner }

func (h twinHooks) Fire(ctx context.Context, evt string, fields map[string]any) session.HookOutcome {
	out := h.runner.Fire(ctx, evt, fields)

	return session.HookOutcome{Proceed: out.Proceed, Message: out.Message}
}

func (h twinHooks) PreToolUseVerdict(
	ctx context.Context, toolName string, input json.RawMessage,
) (verdict session.HookVerdict, reason string) { //nolint:nonamedreturns // mirrors the seam's named pair
	v, r := h.runner.PreToolUseVerdict(ctx, toolName, input)

	switch v {
	case ecosys.VerdictNone:
		return session.HookVerdictNone, ""
	case ecosys.VerdictDeny:
		return session.HookVerdictDeny, r
	case ecosys.VerdictAsk:
		return session.HookVerdictAsk, r
	case ecosys.VerdictAllow:
		return session.HookVerdictAllow, r
	default:
		return session.HookVerdictNone, ""
	}
}

func (h twinHooks) PostToolUse(ctx context.Context, toolName string, input, output json.RawMessage) {
	h.runner.PostToolUse(ctx, toolName, input, output)
}

// testCatalogOf fetches the runner's twin catalog (the checked-assertion
// lens for plant/read fixtures).
func testCatalogOf(t *testing.T, r *Runner) *testCatalog {
	t.Helper()

	tc, ok := r.commandCatalog.(*testCatalog)
	if !ok {
		t.Fatal("runner's catalog is not the test twin")
	}

	return tc
}
