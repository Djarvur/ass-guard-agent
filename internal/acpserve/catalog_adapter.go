// The D-17 catalog adapter (25-06 Task 2): the app-side implementation of
// the kit's runtime.CommandCatalog over the discovered ecosys registry +
// the OpenSpec mutability table. This is the ONE place app agent/hook
// values map onto their kit mirrors (Pitfall 3) — the kit never names an
// app type. The adapter owns the 20-05 wholesale-swap discipline: Rediscover
// replaces the registry/servers under the mutex, never mutating in place.

package acpserve

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Djarvur/ass-guard-agent/internal/ecosys"
	"github.com/Djarvur/ass-guard-agent/internal/openspec"
	"github.com/Djarvur/ass-guard-agent/kit/mcp"
	"github.com/Djarvur/ass-guard-agent/kit/runtime"
	"github.com/Djarvur/ass-guard-agent/kit/session"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// catalogAdapter implements runtime.CommandCatalog over one discovered
// ecosystem state. Constructed by loadCommandCatalog (startup) and swapped
// wholesale by Rediscover (the rescan path).
type catalogAdapter struct {
	mu         sync.Mutex
	workDir    string
	reg        ecosys.Registry
	servers    []ecosys.ServerConfig
	mutability map[string]string
}

// newCatalogAdapter freezes one discovered state (the maps are immutable
// post-construction — the swap discipline builds fresh ones wholesale).
func newCatalogAdapter(
	workDir string, reg ecosys.Registry, servers []ecosys.ServerConfig, mutability map[string]string,
) *catalogAdapter {
	return &catalogAdapter{workDir: workDir, reg: reg, servers: servers, mutability: mutability}
}

// Rediscover re-runs discovery (the 20-05 rescan body): the fresh state
// swaps in wholesale; an error keeps the PREVIOUS state (the kit's
// "keeping the current chain" degrade).
func (a *catalogAdapter) Rediscover() error {
	reg, servers, err := ecosys.Discover(a.workDir)
	if err != nil {
		return err //nolint:wrapcheck // the kit's rescanAndSwap renders the degrade
	}

	a.mu.Lock()
	a.reg = reg
	a.servers = servers
	a.mu.Unlock()

	return nil
}

// Entries returns the slash-eligible discovered rows in D-02 chain order
// (skills → agents → file commands), applying the 20-04 user-invocable
// filter app-side: an explicit user-invocable: false skill never enters the
// slash chain (the model-invocation path keeps it — only the slash surface
// excludes it).
func (a *catalogAdapter) Entries() []runtime.CatalogEntry {
	reg, _ := a.snapshot()

	out := make([]runtime.CatalogEntry, 0, len(reg.Skills)+len(reg.Agents)+len(reg.Commands))

	for _, sk := range reg.AllSkills() {
		if sk.UserInvocable != nil && !*sk.UserInvocable {
			continue
		}

		out = append(out, runtime.CatalogEntry{
			Kind: runtime.EntryKindSkill, Name: sk.Name,
			Description: sk.Description, Path: sk.Path,
		})
	}

	for _, ag := range reg.AllAgents() {
		def := kitAgentDef(&ag)

		out = append(out, runtime.CatalogEntry{
			Kind: runtime.EntryKindAgent, Name: ag.Name,
			Description: ag.Description, Path: ag.Path, Agent: &def,
		})
	}

	for _, cmd := range reg.AllCommands() {
		out = append(out, runtime.CatalogEntry{
			Kind: runtime.EntryKindFile, Name: cmd.Name,
			Description: cmd.Description, InputHint: cmd.ArgumentHint, Path: cmd.Path,
		})
	}

	return out
}

// LookupCommand resolves one chain winner's expandable body: file commands
// verbatim, skills as fresh-body commands (the SKILL.md body re-read from
// disk — the rescan path serves fresh bodies by design). kind is the
// winner's EntryKind (the kit's chain decided WHO won; this fetches the
// body).
func (a *catalogAdapter) LookupCommand(kind, key string) (runtime.Command, bool) {
	reg, _ := a.snapshot()

	switch kind {
	case string(runtime.EntryKindFile):
		cmd, ok := reg.Commands[key]
		if !ok {
			return runtime.Command{}, false
		}

		return kitCommand(&cmd), true
	case string(runtime.EntryKindSkill):
		sk, ok := reg.Skills[key]
		if !ok {
			return runtime.Command{}, false
		}

		body, ok := ecosys.ResolveSkill(reg, key)
		if !ok {
			return runtime.Command{}, false
		}

		return runtime.Command{
			Name: sk.Name, Description: sk.Description, Body: body, Path: sk.Path,
		}, true
	default:
		return runtime.Command{}, false
	}
}

// SkillBody returns one skill's fresh on-disk body.
func (a *catalogAdapter) SkillBody(key string) (string, bool) {
	reg, _ := a.snapshot()

	return ecosys.ResolveSkill(reg, key)
}

// Mutating resolves the D-11 boundary table (the OpenSpec vocabulary stays
// app-side — no mutability literal crosses the seam, T-25-29).
func (a *catalogAdapter) Mutating(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.mutability[key] == openspec.MutabilityMutating
}

// Agents maps the discovered agent table onto the kit AgentDef mirrors —
// the ONE app→kit agent mapping site (Pitfall 3).
func (a *catalogAdapter) Agents() map[string]session.AgentDef {
	reg, _ := a.snapshot()

	out := make(map[string]session.AgentDef, len(reg.Agents))
	for name, ag := range reg.Agents {
		out[name] = kitAgentDef(&ag)
	}

	return out
}

// Hooks builds the per-session lifecycle-hook surface: the ecosystem runner
// wrapped in the kit session.Hooks interface (the ONE hook verdict/outcome
// mapping site — hooksAdapter below).
func (a *catalogAdapter) Hooks(sessionID, dir, transcriptPath string) session.Hooks { //nolint:ireturn // seam shape
	reg, _ := a.snapshot()

	return hooksAdapter{runner: ecosys.NewHookRunner(reg.Hooks, sessionID, dir, transcriptPath)}
}

// SkillExecute returns the Skill tool's Execute override over the registry.
func (a *catalogAdapter) SkillExecute() toolcat.Stub {
	reg, _ := a.snapshot()

	return ecosys.SkillExecute(reg)
}

// SkillListing renders the skills listing block (captured zcode shape).
func (a *catalogAdapter) SkillListing() string {
	reg, _ := a.snapshot()

	return ecosys.SkillListing(reg)
}

// AgentListing renders the agent-type listing block.
func (a *catalogAdapter) AgentListing() string {
	reg, _ := a.snapshot()

	return ecosys.AgentListing(reg)
}

// MemoryInjection renders the memory body for dir.
func (a *catalogAdapter) MemoryInjection(dir string) string {
	return ecosys.MemoryInjection(dir)
}

// MCPLayers returns the non-project MCP set (user over plugin — the two
// lowest layers) as kit server configs.
func (a *catalogAdapter) MCPLayers() []mcp.ServerConfig {
	_, servers := a.snapshot()

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

// MemoryFiles lists the discovered memory sources (the /memory builtin).
func (a *catalogAdapter) MemoryFiles(dir string) []runtime.MemoryFile {
	files := ecosys.DiscoverMemoryFiles(dir)
	if len(files) == 0 {
		return nil
	}

	out := make([]runtime.MemoryFile, 0, len(files))
	for _, f := range files {
		out = append(out, runtime.MemoryFile{Path: f.Path, OrigBytes: f.OrigBytes, SkipNote: f.SkipNote})
	}

	return out
}

// Counts returns the discovered surface sizes (the /doctor builtin).
func (a *catalogAdapter) Counts() runtime.CatalogCounts {
	reg, _ := a.snapshot()

	return runtime.CatalogCounts{
		Commands: len(reg.Commands), Skills: len(reg.Skills), Agents: len(reg.Agents),
	}
}

// snapshot returns the live registry + MCP set under the read lock.
func (a *catalogAdapter) snapshot() (ecosys.Registry, []ecosys.ServerConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.reg, a.servers
}

// --- the ONE app→kit value mappings (Pitfall 3) ---------------------------

// kitAgentDef maps one discovered agent record onto the kit mirror (the
// dispatch-read fields only — field minimality limits what a repo-shipped
// definition can influence, T-25-27).
func kitAgentDef(a *ecosys.Agent) session.AgentDef {
	return session.AgentDef{
		Name: a.Name, Description: a.Description,
		Tools: a.Tools, Model: a.Model, Prompt: a.Prompt,
	}
}

// kitCommand maps one discovered file command onto the kit record (the
// expansion-relevant fields; ArgumentHint/AllowedTools/Model stay app-side —
// parse-only mimicry parity the kit never reads).
func kitCommand(c *ecosys.Command) runtime.Command {
	return runtime.Command{Name: c.Name, Description: c.Description, Body: c.Body, Path: c.Path}
}

// hooksAdapter bridges the ecosystem hook runner onto the kit session.Hooks
// interface — the ONE place hook outcome/verdict values become kit values.
// The underlying runner is nil-safe, so the adapter is too.
type hooksAdapter struct{ runner *ecosys.HookRunner }

// Fire runs the lifecycle event and mirrors its outcome.
func (h hooksAdapter) Fire(ctx context.Context, evt string, fields map[string]any) session.HookOutcome {
	out := h.runner.Fire(ctx, evt, fields)

	return session.HookOutcome{Proceed: out.Proceed, Message: out.Message}
}

// PreToolUseVerdict resolves the combined verdict onto the kit enum.
func (h hooksAdapter) PreToolUseVerdict(
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
		return session.HookVerdictNone, "" // an unknown app verdict stays no-decision
	}
}

// PostToolUse observes the completed result (the coreexec.ToolHooks seam).
func (h hooksAdapter) PostToolUse(ctx context.Context, toolName string, input, output json.RawMessage) {
	h.runner.PostToolUse(ctx, toolName, input, output)
}
