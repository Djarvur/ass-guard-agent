package runtime //nolint:testpackage // internal package test

// The 25-08 kit-side test-double family (the plan's Task 2 artifact): every
// helper the white-box batteries share, expressed in KIT vocabulary only —
// zero app imports (the file that makes the test-exempt D-19 gate scope
// honest: kit test files survive on kit fakes, not on app packages).
//
// Family: the fake CommandCatalog (canned entries + a minimal fixture-file
// reader for the planted .claude trees), the fake Hooks (recording fires +
// canned verdicts), the fake pattern table / learned store (engine_setup's
// file), plus shared lenses (ckptLiveTree). The REAL discovery, script-hook
// execution, memory precedence, and openspec loading keep their own subjects
// in internal/ecosys + internal/openspec; the REAL app-side assembly is
// pinned by acpserve's serve suites.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Djarvur/ass-guard-agent/kit/mcp"
	"github.com/Djarvur/ass-guard-agent/kit/session"
	"github.com/Djarvur/ass-guard-agent/kit/toolcat"
)

// ckptLiveTree fingerprints every regular file under dir, skipping the
// .ass-guard root (the store + transcripts live there) — the tree-identity
// lens the /undo walk batteries share (formerly checkpoint_session's).
func ckptLiveTree(t *testing.T, dir string) map[string]string {
	t.Helper()

	out := map[string]string{}

	werr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}

		if d.IsDir() {
			if d.Name() == ".ass-guard" {
				return filepath.SkipDir
			}

			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return fmt.Errorf("rel %s: %w", path, rerr)
		}

		data, derr := os.ReadFile(path)
		if derr != nil {
			return fmt.Errorf("read %s: %w", path, derr)
		}

		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])

		return nil
	})
	if werr != nil {
		t.Fatalf("ckptLiveTree(%s): %v", dir, werr)
	}

	return out
}

// fakeCatalog is the kit-side CommandCatalog fake: in-memory command/skill/
// agent tables seeded from the planted fixture FILES (the batteries plant
// .claude/commands + .claude/skills + .claude/agents trees and the fake
// reads the same simple frontmatter format the fixtures write), canned
// mutability + hooks + memory. Rediscover re-reads the trees — the rescan
// batteries' plant-then-rescan flow rides it unchanged.
type fakeCatalog struct {
	mu         sync.Mutex
	workDir    string
	commands   map[string]fakeCommandRow
	skills     map[string]fakeSkillRow
	agents     map[string]session.AgentDef
	mcpServers []mcp.ServerConfig
	hooks      session.Hooks
	mutability map[string]bool
}

// fakeCommandRow is one canned command (the file-command shape LookupCommand
// serves: body + description + path).
type fakeCommandRow struct {
	description string
	body        string
	path        string
}

// fakeSkillRow is one canned skill (body after frontmatter + the row's
// listing metadata; UserInvocable=false hides it from Entries — the 20-04
// slash-surface rule).
type fakeSkillRow struct {
	description  string
	body         string
	path         string
	hidden       bool
	allowedTools []string
}

// newTestCatalog reads the fixture trees under workDir into the canned
// tables (the batteries' writeOpsxCommandFixtures / writeSkillFixtures /
// agents plant the files BEFORE calling this — the same call shape the real
// seededMutatingCommands mirrors the mutating rows of the embedded openspec
// floor (internal/openspec/seeded.toml [commands] + [command_mutability] —
// kit-side test constants, the same verbatim-mirror discipline the pattern
// rows follow; read-only rows are simply absent from the map).
var seededMutatingCommands = []string{
	// [command_mutability] — the opsx slash surface (D-11)
	"opsx:propose", "opsx:apply", "opsx:sync", "opsx:update", "opsx:archive",
	"opsx:new", "opsx:continue", "opsx:ff", "opsx:bulk-archive", "opsx:onboard",
	// [commands] — the core openspec vocabulary
	"archive", "new-change", "init", "update",
	"config-set", "config-unset", "config-reset",
	"schema-fork", "schema-init",
	"store-setup", "store-register", "store-unregister", "store-remove",
	"workset-create", "workset-remove",
}

// newTestCatalog builds the kit fake over workDir's fixture trees. (The
// twin used, so consumer bodies stay byte-identical).
func newTestCatalog(workDir string) *fakeCatalog {
	fc := &fakeCatalog{
		workDir:    workDir,
		commands:   map[string]fakeCommandRow{},
		skills:     map[string]fakeSkillRow{},
		agents:     map[string]session.AgentDef{},
		mutability: map[string]bool{},
	}

	for _, key := range seededMutatingCommands {
		fc.mutability[key] = true
	}

	// the adapter degradation contract: a FAILED discovery installs an
	// EMPTY catalog (the seeded mutability floor stays — that table is
	// config, not discovery), never a stale one.
	if fc.reload() != nil {
		fc.commands = map[string]fakeCommandRow{}
		fc.skills = map[string]fakeSkillRow{}
		fc.agents = map[string]session.AgentDef{}
	}

	return fc
}

// reload reads the fixture trees (commands + skills + agents). A tree that
// exists but cannot be walked (e.g. .claude/skills planted as a FILE —
// ReadDir ENOTDIR) FAILS the whole reload: the fake mirrors the real
// adapter's degradation contract (a failed discovery installs an EMPTY
// catalog, never a stale one — runtime.go's SetCatalog doc). A missing tree
// is an empty tree (legal). Plugin installs (modesmatrix.MountFixture's
// .claude/plugins layout) load FIRST so project-level entries win collisions.
func (t *fakeCatalog) reload() error {
	if err := t.loadPluginInstalls(filepath.Join(t.workDir, ".claude", "plugins")); err != nil {
		return err
	}

	if err := t.loadCommands(filepath.Join(t.workDir, ".claude", "commands")); err != nil {
		return err
	}

	if err := t.loadSkills(filepath.Join(t.workDir, ".claude", "skills")); err != nil {
		return err
	}

	return t.loadAgents(filepath.Join(t.workDir, ".claude", "agents"))
}

// pluginInstall mirrors one installed_plugins.json row.
type pluginInstall struct {
	Name        string `json:"name"`
	InstallPath string `json:"installPath"`
}

// loadPluginInstalls walks the plugin registry the MountFixture layout
// writes (.claude/plugins/installed_plugins.json → .claude/plugins/<install
// path>/ bundles). Existing (project-level) entries WIN collisions — the
// loader's project-over-plugin precedence.
func (t *fakeCatalog) loadPluginInstalls(pluginsDir string) error {
	data, err := os.ReadFile(filepath.Join(pluginsDir, "installed_plugins.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	var installs []pluginInstall
	if err := json.Unmarshal(data, &installs); err != nil {
		return err
	}

	for _, ins := range installs {
		install := filepath.Join(pluginsDir, ins.InstallPath)

		if err := t.loadCommandsNoClobber(filepath.Join(install, "commands")); err != nil {
			return err
		}

		if err := t.loadSkillsNoClobber(filepath.Join(install, "skills")); err != nil {
			return err
		}

		if err := t.loadAgents(filepath.Join(install, "agents")); err != nil {
			return err
		}
	}

	return nil
}

// loadCommandsNoClobber / loadSkillsNoClobber wrap the .claude-tree loaders
// with project-wins semantics for plugin bundles.
func (t *fakeCatalog) loadCommandsNoClobber(cmdDir string) error {
	snapshot := map[string]fakeCommandRow{}
	for k, v := range t.commands {
		snapshot[k] = v
	}

	if err := t.loadCommands(cmdDir); err != nil {
		return err
	}

	for k, v := range snapshot {
		t.commands[k] = v
	}

	return nil
}

func (t *fakeCatalog) loadSkillsNoClobber(skillDir string) error {
	snapshot := map[string]fakeSkillRow{}
	for k, v := range t.skills {
		snapshot[k] = v
	}

	if err := t.loadSkills(skillDir); err != nil {
		return err
	}

	for k, v := range snapshot {
		t.skills[k] = v
	}

	return nil
}

// readFixtureDir lists dir, mapping absent → nil,nil (empty tree is legal)
// and any other error (ENOTDIR etc.) → failure.
func readFixtureDir(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	return entries, err
}

// loadCommands mirrors the real walk's shape: top-level *.md key by stem; a
// subdirectory is a NAMESPACE keying its *.md as "<ns>:<stem>" (loader's
// discoverNamespacedCommands — the opsx: fixtures' mapping).
func (t *fakeCatalog) loadCommands(cmdDir string) error {
	entries, err := readFixtureDir(cmdDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() {
			nsEntries, nsErr := readFixtureDir(filepath.Join(cmdDir, e.Name()))
			if nsErr != nil {
				return nsErr
			}

			for _, ne := range nsEntries {
				if ne.IsDir() || !strings.HasSuffix(ne.Name(), ".md") {
					continue
				}

				if !t.loadCommandFile(filepath.Join(cmdDir, e.Name(), ne.Name()), e.Name()+":"+strings.TrimSuffix(ne.Name(), ".md")) {
					continue
				}
			}

			continue
		}

		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		t.loadCommandFile(filepath.Join(cmdDir, e.Name()), strings.TrimSuffix(e.Name(), ".md"))
	}

	return nil
}

// loadCommandFile reads one command fixture into key; false = unreadable
// (skipped silently — the real loader's warn-skip shape).
func (t *fakeCatalog) loadCommandFile(path, key string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	head, rest := cutFrontmatter(string(body))

	t.commands[key] = fakeCommandRow{description: frontmatterValue(head, "description"), body: rest, path: path}

	return true
}

// loadSkills reads one <name>/SKILL.md per skill directory.
func (t *fakeCatalog) loadSkills(skillDir string) error {
	entries, err := readFixtureDir(skillDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		path := filepath.Join(skillDir, e.Name(), "SKILL.md")

		body, rerr := os.ReadFile(path)
		if rerr != nil {
			continue
		}

		head, rest := cutFrontmatter(string(body))

		row := fakeSkillRow{description: frontmatterValue(head, "description"), body: rest, path: path}
		if frontmatterValue(head, "user-invocable") == "false" {
			row.hidden = true
		}

		t.skills[e.Name()] = row
	}

	return nil
}

// loadAgents reads one <name>.md agent definition per file.
func (t *fakeCatalog) loadAgents(agentsDir string) error {
	entries, err := readFixtureDir(agentsDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		path := filepath.Join(agentsDir, e.Name())

		body, rerr := os.ReadFile(path)
		if rerr != nil {
			continue
		}

		name := strings.TrimSuffix(e.Name(), ".md")

		head, rest := cutFrontmatter(string(body))

		t.agents[name] = session.AgentDef{
			Name:        name,
			Description: frontmatterValue(head, "description"),
			Tools:       frontmatterList(head, "tools"),
			Model:       frontmatterValue(head, "model"),
			Prompt:      strings.TrimSpace(rest),
		}
	}

	return nil
}

// cutFrontmatter splits "---\nkey: value lines\n---\nbody" into the head
// block + the body (the fixture format the batteries write).
func cutFrontmatter(raw string) (head, body string) {
	if !strings.HasPrefix(raw, "---\n") {
		return "", raw
	}

	rest := raw[4:]

	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", raw
	}

	return rest[:idx], rest[idx+5:]
}

// frontmatterList reads one "key: [A, B]" line from a frontmatter head.
func frontmatterList(head, key string) []string {
	v := frontmatterValue(head, key)
	v = strings.TrimSuffix(strings.TrimPrefix(v, "["), "]")
	if v == "" {
		return nil
	}

	parts := strings.Split(v, ",")

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}

// frontmatterValue reads one "key: value" line from a frontmatter head.
func frontmatterValue(head, key string) string {
	for _, line := range strings.Split(head, "\n") {
		if v, ok := strings.CutPrefix(line, key+":"); ok {
			return strings.TrimSpace(v)
		}
	}

	return ""
}

// Rediscover re-reads the fixture trees (the rescan flow's reload).
func (t *fakeCatalog) Rediscover() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.commands = map[string]fakeCommandRow{}
	t.skills = map[string]fakeSkillRow{}
	t.agents = map[string]session.AgentDef{}

	if err := t.reload(); err != nil {
		// failed rescan degrades EMPTY (the no-stale-registry contract) and
		// surfaces the error to the caller.
		return err
	}

	return nil
}

// Entries lists the catalog rows (skills first, then agents, then file
// commands — the twin's order).
func (t *fakeCatalog) Entries() []CatalogEntry {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]CatalogEntry, 0, len(t.skills)+len(t.agents)+len(t.commands))

	for name, sk := range t.skills {
		if sk.hidden {
			continue
		}

		out = append(out, CatalogEntry{
			Kind: EntryKindSkill, Name: name, Description: sk.description, Path: sk.path,
		})
	}

	for name, ag := range t.agents {
		def := ag

		out = append(out, CatalogEntry{
			Kind: EntryKindAgent, Name: name, Description: ag.Description, Agent: &def,
		})
	}

	for name, cmd := range t.commands {
		out = append(out, CatalogEntry{
			Kind: EntryKindFile, Name: name, Description: cmd.description, Path: cmd.path,
		})
	}

	return out
}

// LookupCommand serves one row's full body by kind + key.
func (t *fakeCatalog) LookupCommand(kind, key string) (Command, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch kind {
	case string(EntryKindFile):
		cmd, ok := t.commands[key]
		if !ok {
			return Command{}, false
		}

		return Command{Name: key, Description: cmd.description, Body: cmd.body, Path: cmd.path}, true
	case string(EntryKindSkill):
		sk, ok := t.skills[key]
		if !ok {
			return Command{}, false
		}

		return Command{Name: key, Description: sk.description, Body: sk.body, Path: sk.path}, true
	default:
		return Command{}, false
	}
}

// SkillBody serves one skill's body.
func (t *fakeCatalog) SkillBody(key string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	sk, ok := t.skills[key]

	return sk.body, ok
}

// Mutating answers the canned mutability table.
func (t *fakeCatalog) Mutating(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.mutability[key]
}

// Agents serves the agent map.
func (t *fakeCatalog) Agents() map[string]session.AgentDef {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make(map[string]session.AgentDef, len(t.agents))
	for name, ag := range t.agents {
		out[name] = ag
	}

	return out
}

// Hooks serves the canned hooks (nil-safe: a nil hooks degrades exactly as
// the emptyCatalog contract documents).
func (t *fakeCatalog) Hooks(_, _, _ string) session.Hooks { //nolint:ireturn // seam shape
	return t.hooks
}

// setHooks arms the canned hooks (the hook-join batteries' deny verdicts —
// the same surface the planted settings tree drove through the real runner).
func (t *fakeCatalog) setHooks(h session.Hooks) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.hooks = h
}

// SkillExecute serves the Skill tool's Execute override over the fake
// registry (the captured resolution semantics: {"skill": name} → the SKILL.md
// body as {"content": …}; unknown names return the structured error with the
// sorted available list — never a Go error, the model sees structured results).
func (t *fakeCatalog) SkillExecute() toolcat.Stub {
	return func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
		var in struct {
			Skill string `json:"skill"`
		}

		_ = json.Unmarshal(input, &in) // best-effort; empty skill → unknown below

		t.mu.Lock()
		sk, inReg := t.skills[in.Skill]
		skills := make(map[string]fakeSkillRow, len(t.skills))
		for name, row := range t.skills {
			skills[name] = row
		}
		t.mu.Unlock()

		if !inReg {
			names := make([]string, 0, len(skills))
			for name := range skills {
				names = append(names, name)
			}

			sort.Strings(names)

			return json.Marshal(struct {
				Error     string   `json:"error"`
				Available []string `json:"available"`
			}{Error: "unknown skill " + in.Skill, Available: names})
		}

		body, rerr := os.ReadFile(sk.path)
		if rerr != nil {
			return json.Marshal(struct {
				Error string `json:"error"`
				Skill string `json:"skill"`
				File  string `json:"file"`
			}{Error: "skill read failed", Skill: in.Skill, File: sk.path})
		}

		head, rest := cutFrontmatter(string(body))
		_ = head // body only — the same split the loader applies

		return json.Marshal(struct {
			Content string `json:"content"`
		}{Content: rest})
	}
}

// The listing renders mirror the captured shapes byte-for-byte (the same
// constants internal/ecosys's loader renders — header, name-sorted entries,
// the 249-rune description cutoff, "(file: path)" / "(Tools: …)" suffixes)
// so the batteries' composed-profile assertions see listings identical in
// shape to the real discovery's. The REAL render's own subject stays in
// internal/ecosys (loader tests); the fake exists so kit batteries don't
// import it.
const (
	fakeSkillListingHeader = "The following skills are available for use with the Skill tool:"
	fakeAgentListingHeader = "The following specialized agent types are available for use with the Agent tool:"
	fakeDescMax            = 249
	fakeDescEllipsis       = "..."
)

func truncateFakeDesc(desc string) string {
	r := []rune(desc)
	if len(r) <= fakeDescMax {
		return desc
	}

	return string(r[:fakeDescMax]) + fakeDescEllipsis
}

// SkillListing renders every fake skill (D-04: all discovered, no filtering)
// in the captured listing shape. Empty table yields "" (the caller skips the
// merge — zero-skill degradation leaves the profile copy untouched).
func (t *fakeCatalog) SkillListing() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.skills) == 0 {
		return ""
	}

	names := make([]string, 0, len(t.skills))
	for name := range t.skills {
		names = append(names, name)
	}

	sort.Strings(names)

	entries := make([]string, 0, len(names))
	for _, name := range names {
		sk := t.skills[name]
		entries = append(entries, "- "+name+": "+truncateFakeDesc(sk.description)+" (file: "+sk.path+")")
	}

	return fakeSkillListingHeader + "\n\n" + strings.Join(entries, "\n")
}

// AgentListing renders every fake agent in the captured type-listing shape
// ("(Tools: *)" for an unrestricted entry). Empty table yields "".
func (t *fakeCatalog) AgentListing() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.agents) == 0 {
		return ""
	}

	names := make([]string, 0, len(t.agents))
	for name := range t.agents {
		names = append(names, name)
	}

	sort.Strings(names)

	entries := make([]string, 0, len(names))
	for _, name := range names {
		ag := t.agents[name]
		tools := strings.Join(ag.Tools, ", ")
		if tools == "" {
			tools = "*"
		}

		entries = append(entries, "- "+name+": "+truncateFakeDesc(ag.Description)+" (Tools: "+tools+")")
	}

	return fakeAgentListingHeader + "\n\n" + strings.Join(entries, "\n")
}

// memInjHeaderKit + the memory framing mirror the injection envelope the
// app adapter's real MemoryInjection emits (header line + "### <path>"
// sections + the 64 KB budget skip note) — the envelope shape the runtime
// wiring's assertions pin; the full precedence walk stays internal/ecosys's
// subject.
const memInjHeaderKit = "The following project memory files apply to this session:"

const memTotalBudgetKit = 64 << 10

// memPerFileCapKit is the D-08 per-file cap (24 KB — the same value the real
// render enforces before injection).
const memPerFileCapKit = 24 << 10

// MemoryInjection reads the repo-root memory file the batteries plant
// (AGENTS.md, then CLAUDE.md — the memory_wiring scenarios' single-file
// shape) framed with the injection envelope.
func (t *fakeCatalog) MemoryInjection(dir string) string {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		p := filepath.Join(dir, name)

		body, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		var b strings.Builder

		b.WriteString(memInjHeaderKit)
		b.WriteString("\n\n### " + p + "\n\n")

		if len(body) > memTotalBudgetKit {
			fmt.Fprintf(&b, "[note: %s skipped — the %d-byte total memory budget is exhausted]", p, memTotalBudgetKit)

			return b.String()
		}

		// the D-08 per-file cap (rune-safe cut + note before the content —
		// the same envelope the real render emits)
		if len(body) > memPerFileCapKit {
			fmt.Fprintf(&b, "[note: %s truncated from %d bytes to the %d-byte per-file cap]\n\n", p, len(body), memPerFileCapKit)

			body = []byte(string([]rune(string(body))[:memPerFileCapKit]))
		}

		b.Write(body)

		return b.String()
	}

	return ""
}

// MCPLayers serves the canned lower MCP layers.
func (t *fakeCatalog) MCPLayers() []mcp.ServerConfig {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.mcpServers) == 0 {
		return nil
	}

	return t.mcpServers
}

// MemoryFiles reports the memory file the batteries planted (the read-back
// lens; single-file like MemoryInjection).
func (t *fakeCatalog) MemoryFiles(dir string) []MemoryFile {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		p := filepath.Join(dir, name)

		body, err := os.ReadFile(p)
		if err == nil {
			return []MemoryFile{{Path: p, OrigBytes: len(body)}}
		}
	}

	return nil
}

// Counts serves the table sizes.
func (t *fakeCatalog) Counts() CatalogCounts {
	t.mu.Lock()
	defer t.mu.Unlock()

	return CatalogCounts{Commands: len(t.commands), Skills: len(t.skills), Agents: len(t.agents)}
}

// plantAgent registers one agent definition (the late-arrival fixtures; the
// caller rebuilds the chain afterwards).
func (t *fakeCatalog) plantAgent(a session.AgentDef) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.agents[a.Name] = a
}

// plantSkills replaces the skill table (the help-rebuild fixture).
func (t *fakeCatalog) plantSkills(skills map[string]fakeSkillRow) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.skills = skills
}

// skill returns one skill row (the model-surface assertion lens).
func (t *fakeCatalog) skill(name string) (fakeSkillRow, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	sk, ok := t.skills[name]

	return sk, ok
}

// agent returns one agent row (the native-surface assertion lens).
func (t *fakeCatalog) agent(name string) (session.AgentDef, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ag, ok := t.agents[name]

	return ag, ok
}

// testCatalogOf fetches the runner's fake catalog (the checked-assertion
// lens for plant/read fixtures).
func testCatalogOf(t *testing.T, r *Runner) *fakeCatalog {
	t.Helper()

	tc, ok := r.commandCatalog.(*fakeCatalog)
	if !ok {
		t.Fatal("runner's catalog is not the kit fake")
	}

	return tc
}

// fakeHooks is the kit-side Hooks seam fake: records Fire calls and answers
// canned PreToolUse verdicts (per-tool, settable per battery). The REAL
// script-hook execution (settings.json discovery, exit-code routing, the
// hookSpecificOutput envelope) keeps its own subject in internal/ecosys's
// hooks suite; the REAL adapter composition is pinned acpserve-side.
type fakeHooks struct {
	mu       sync.Mutex
	fired    []string
	verdicts map[string]session.HookVerdict
	reasons  map[string]string
}

func (h *fakeHooks) Fire(_ context.Context, _ string, _ map[string]any) session.HookOutcome {
	return session.HookOutcome{}
}

func (h *fakeHooks) PreToolUseVerdict(
	_ context.Context, toolName string, _ json.RawMessage,
) (session.HookVerdict, string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.fired = append(h.fired, toolName)

	return h.verdicts[toolName], h.reasons[toolName]
}

func (h *fakeHooks) PostToolUse(context.Context, string, json.RawMessage, json.RawMessage) {}

// setVerdict arms one tool's canned verdict + reason.
func (h *fakeHooks) setVerdict(tool string, v session.HookVerdict, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.verdicts == nil {
		h.verdicts = map[string]session.HookVerdict{}
		h.reasons = map[string]string{}
	}

	h.verdicts[tool] = v
	h.reasons[tool] = reason
}

// markerFileHooks is the modes-matrix cells' kit-native hook double. The
// fixture's script hooks appended their stdin JSON (one line) to
// matrix-hook.log in the host project dir; this double appends the same
// payload shape from the verdict seam — script execution stays
// internal/ecosys's subject, the JOIN (the gate head consulting hooks inside
// the automation/wake turn) is the kit subject these cells pin.
type markerFileHooks struct {
	mu  sync.Mutex
	dir string
}

func (h *markerFileHooks) Fire(context.Context, string, map[string]any) session.HookOutcome {
	return session.HookOutcome{Proceed: true}
}

func (h *markerFileHooks) PreToolUseVerdict(
	_ context.Context, toolName string, _ json.RawMessage,
) (session.HookVerdict, string) {
	// the fixture's PreToolUse matcher is "Read" only (hooks.json) — the
	// Task dispatch of the wake cells never fired it under real discovery.
	if toolName != "Read" {
		return session.HookVerdictNone, ""
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	payload, err := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       toolName,
	})
	if err != nil {
		return session.HookVerdictNone, ""
	}

	f, ferr := os.OpenFile(filepath.Join(h.dir, "matrix-hook.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if ferr == nil {
		_, _ = f.Write(append(payload, '\n'))
		_ = f.Close()
	}

	return session.HookVerdictNone, ""
}

func (h *markerFileHooks) PostToolUse(context.Context, string, json.RawMessage, json.RawMessage) {}
