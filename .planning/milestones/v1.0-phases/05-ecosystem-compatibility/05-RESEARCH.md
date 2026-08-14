# Phase 5: Ecosystem Compatibility - Research

**Researched:** 2026-08-09
**Status:** Complete
**Confidence:** HIGH (official SDK docs + pkg.go.dev + real codebase interfaces)

<domain_summary>
Phase 5 makes ass-guard a drop-in member of the Claude-Code ecosystem. Two independent streams:

1. **MCP hosting (ECOS-01/02/03):** spawn Claude-Code-installed MCP servers as subprocesses via `modelcontextprotocol/go-sdk`, bridge their tools into the catalog as `mcp__<server>__<tool>`, lifecycle-robust (process-group + reaper), re-fetch `tools/list` every connection.
2. **Skills/commands/plugins + namespacing (ECOS-04/05):** load Claude-Code's `.claude/` layout read-only; ass-guard's own additions live under `.ass-guard/`; `.claude/` wins on conflict; user→project precedence.

ROADMAP dependency note: "Phase 5 needs Phase 2 (stable tool registry)." The two streams are parallelizable ("MCP hosting vs the rest") but follow Phase 4's serialized-deltas discipline within each stream.
</domain_summary>

<codebase_truth>
## REAL interfaces verified against Phases 1-3 code (read, not assumed)

### `internal/toolcat` — where MCP tools merge with built-ins
- **`Catalog`** (`catalog.go`): `tools map[string]Tool` (private map). Constructed by `NewCatalog()` which loads embedded `coretools.json`. Methods: `Get(name) (Tool, bool)`, `Names() []string`, `Decls() []Decl`.
  - **GAP: no public `Register`/`Add` method exists.** Phase 5 MUST add one (`Register(Tool) error`) so MCP tools discovered at session start can enter the catalog. This is load-bearing for D-01/D-04.
- **`Tool`** (`types.go`): `{Name, Description string; InputSchema json.RawMessage; Mutability Mutability; Execute Stub}`. `Stub` = `func(ctx, json.RawMessage) (json.RawMessage, error)`. `Execute` is nil/stubbed in Phase 1 (D-15) — Phase 4 wires real execution. **Phase 5 adds MCP tools whose `Execute` closure routes through the MCP client.**
- **`ToolExecutor`** interface (`restricted.go`): `Execute(ctx, name string, input json.RawMessage) (json.RawMessage, error)`. This is the runtime execution seam. `RestrictedExecutor` wraps a `ToolExecutor` for subagent subsets (D-10). **Phase 5 adds an MCP-aware executor that delegates `mcp__*` calls to the MCP host and falls through to the inner executor otherwise.**
- **`Adapter`** (`adapter.go`): the profile-declared schema is AUTHORITATIVE (TOOL-02). `ResolveCall(name, input, profileDecls)` validates a tool-call against the profile decls.

### `internal/shaper` — what the model sees
- **`Shaper.Shape(p profile.Profile, messages []Message) (anthropic.MessageNewParams, []option.RequestOption, error)`** (`shaper.go:48`). Line 59: `tools, err := toToolUnions(p.Tools)` — **the outgoing tool catalog is built from `profile.Profile.Tools` ([]profile.Decl)`, NOT from `toolcat.Catalog`.**
  - **CONSEQUENCE:** for the model to see MCP tools, the MCP tool declarations must be in `p.Tools` at Shaper time. The clean seam (no Shaper signature change): at session construction, build a session-scoped copy of the profile with `Tools = append(base.Tools, mcpDecls...)`. Honors D-01 ("the Shaper includes both built-in + dynamic MCP tools") without touching Shaper code.

### `internal/profile` — config loading convention (NO viper)
- `loader.go` uses **`gopkg.in/yaml.v3`** + `os.ReadFile` + `encoding/json` (line 11 imports yaml.v3; tools.json/thinking.json/tool_choice.json loaded via os.ReadFile + json). **AGENTS.md line 54 lists viper, but the real code does NOT use viper and the project directive overrides: use `encoding/json` for `.claude/`-compat JSON config (`.mcp.json`, `settings.json`) and yaml.v3 for ass-guard's own YAML.** Never introduce viper.
- `profile.Profile.Tools []Decl` where `Decl = {Name, Description string; InputSchema json.RawMessage}`. Identical shape to `toolcat.Decl` — trivial conversion.

### `internal/session` — lifecycle hooks for MCP spawn/shutdown
- **`Session`** struct (`session.go:33`): value-typed, fields include `Manager *Manager`, `Projector`, `Provider`, `Bus`, `Profile profile.Profile` (value, not pointer), `Catalog *toolcat.Catalog`, `ConfigAdded []string`, `toolExec toolcat.ToolExecutor` (private), `subagentRunner` (private).
- **`Session.Prompt`** (`session.go:70`): the D-18 turn cycle. Line 137: `out, _ := s.executeStub(ctx, tc)` — tool calls flow through `executeStub`, which delegates to `s.toolExec` if wired (`session.go:159-166`). **The MCP-aware executor plugs in here via the existing `toolExec` field.**
- **`Manager`** (`manager.go`): `NewManager(dir, sessionID, red)` opens/creates transcript under `dir/.ass-guard/` (Phase 2 D-06). `AppendSessionStart`/`AppendSessionEnd`/`Close` are the lifecycle markers. **GAP: no explicit "session shutdown hook" — Phase 5 adds a `Session.Close()` (or a hook the ACP logout/cancel handler calls) so MCP servers are reaped when a session ends.**
- **Where Session is constructed:** `cmd/ass-guard/acp_serve.go:198` `sessionFor(sessionID)` builds the `&session.Session{...}`. **This is the single integration point** for MCP: spawn the MCP host here, register discovered tools in `Catalog`, build the session-scoped profile copy. `r.profile` is shared; the per-session copy is essential because MCP tools vary per session (D-01/D-16 — observed 77/97/103 variation).

### `.ass-guard/` directory (Phase 2 D-06/D-07 — already exists)
- Per-project under the working dir. Holds transcripts (`<session-id>.jsonl`). Self-gitignoring: ass-guard creates `.ass-guard/.gitignore` with `*` + `!.gitignore` on first run. **Phase 5 adds `skills/`, `commands/`, `plugins/` subdirs under `.ass-guard/` for ass-guard's OWN additions (D-05). The self-gitignore already covers them.**
</codebase_truth>

<go_sdk_api>
## `modelcontextprotocol/go-sdk` v1.0.0+ — exact client API (verified)

Source: pkg.go.dev (`github.com/modelcontextprotocol/go-sdk/mcp`) + official README + STACK Focus 5.

**Subprocess launch — the host pattern ass-guard needs:**
```go
import (
    "github.com/modelcontextprotocol/go-sdk/mcp"
    "os/exec"
)

transport := &mcp.CommandTransport{Command: exec.Command("the-mcp-server", "args...")}
client := mcp.NewClient(&mcp.Implementation{Name: "ass-guard", Version: "v0"}, nil)
session, err := client.Connect(ctx, transport, nil)   // returns *mcp.ClientSession
```

- `mcp.CommandTransport{Command: *exec.Cmd}` — the transport. **The `exec.Cmd` is fully configurable BEFORE `Connect`**, so ass-guard sets `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` (process-group isolation) and `cmd.Env`, `cmd.Dir` as needed. This is the hook for D-02 process-group spawn.
- `mcp.NewClient(impl *mcp.Implementation, opts *mcp.ClientOptions) *mcp.Client`
- `(*Client).Connect(ctx, transport, opts) (*ClientSession, error)` — starts the subprocess, performs MCP `initialize` handshake. The returned `*ClientSession` owns the live process.
- `(*ClientSession).ListTools(ctx, req *ListToolsRequest) (*ListToolsResult, error)` — returns the server's tool list. `ListToolsResult.Tools []Tool` where `Tool` has `Name`, `Description`, `InputSchema` (JSON schema). **D-03: call this on EVERY connection (session start); never cache.**
- `(*ClientSession).CallTool(ctx, params *CallToolParams) (*CallToolResult, error)` where `CallToolParams{Name string; Arguments map[string]any}`. Result has `.IsError bool` and `.Content []Content` (e.g. `*mcp.TextContent` with `.Text`). This is what the MCP `Tool.Execute` closure calls.
- `(*ClientSession).Close() error` — clean MCP shutdown (sends `shutdown` then `exit` per protocol).

**Protocol version:** SDK supports 2025-06-18 (and 2025-03-26, 2024-11-05) at v1.0.0-v1.1.0; STACK targets the 2026-07-28 line. Pin `github.com/modelcontextprotocol/go-sdk` to the latest v1.x tag at `go get` time. The SDK negotiates the highest mutually-supported protocol version during `initialize`.

**Lifecycle gotcha (STACK Focus 5, verified):** some hosts (Cursor) kill stdio MCP servers ~1.5s after `initialize` in races. ass-guard must keep subprocesses alive for the FULL session and drain cleanly on shutdown via `context.Context` cancellation + `session.Close()`. The `ClientSession` is held for the session lifetime, not closed after `ListTools`.

**Stderr discipline:** the SDK consumes subprocess stderr (matches ass-guard's LSP-style stdout/stderr split). MCP JSON-RPC travels stdin/stdout; stderr → ass-guard's logs (slog).
</go_sdk_api>

<process_lifecycle>
## Process-group spawn + group-signal shutdown + reaper (ECOS-02) — Go specifics

Platform constraint (PROJECT.md): macOS + Linux, amd64 + arm64 (Windows deferred). Both darwin and linux expose `syscall.SysProcAttr{Setpgid: true}` and `syscall.Kill`/`syscall.Wait4` — no build-tag branching needed for these two platforms.

### Process-group spawn (D-02)
```go
cmd := exec.Command(server.Cmd, server.Args...)
cmd.Env = server.Env            // merged with os.Environ()
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}   // new process group; child = group leader
// After Start, cmd.Process.PID is the group leader's PID == the PGID.
```
`Setpgid: true` puts the child in a new process group whose ID equals the child PID. Every grandchild the MCP server spawns inherits the group. Signaling `-pid` reaches the whole tree.

### Group-signal shutdown (D-02)
On session shutdown (session/cancel, logout, or context cancellation):
```go
pgid := cmd.Process.PID
_ = syscall.Kill(-pgid, syscall.SIGTERM)   // negative PID = signal the whole group
// grace period, then escalate:
time.AfterFunc(3*time.Second, func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })
```
Then `mcpSession.Close()` for protocol-level shutdown (belt-and-suspenders with the group signal).

### Reaper goroutine — no zombies (D-02, ECOS-02)
A goroutine per subprocess calls `syscall.Wait4` in a loop to reap the child AND any orphaned grandchildren the group-signal killed:
```go
go func() {
    for {
        var status syscall.WaitStatus
        pid, err := syscall.Wait4(-pgid, &status, 0, nil)   // reap any child in the group
        if err != nil || pid == -1 { break }                // ECHILD / no children left
    }
}()
```
`Wait4(-pgid, ...)` waits for ANY process in the group. Loop until `ECHILD`. Without this, group-signaled grandchildren (whose parent is the MCP server, now dead) get reparented to init/ass-guard and would zombie on Linux. **This is the ECOS-02 "no zombies" guarantee.**

**Cross-platform note:** `Setpgid`, `syscall.Kill(-pgid,...)`, and `syscall.Wait4(-pgid,...)` are available on both darwin and linux (the v1 platform set). Windows is deferred (PROJECT.md), so no `syscall.SysProcAttr{CreationFlags}` branch is needed now; gate the reaper/signaling behind a build constraint (`//go:build darwin || linux`) and leave a Windows stub for v2.
</process_lifecycle>

<claude_code_layout>
## `.claude/` config layout — drop-in compatibility (ECOS-04/05)

Source: STACK "Claude Code .claude/ config layout" + 2026 secondary sources.

### Directory layout ass-guard must read (READ-ONLY per D-05/D-06)
```
.claude/
  settings.json          # project-scoped settings (permissions, env, model)
  commands/              # slash-commands: <name>.md (markdown with optional YAML frontmatter)
  agents/                # subagent definitions
  skills/                # skills: <name>/SKILL.md (+ supporting files)
  plugins/               # plugin manifests (marketplace-installed)
.mcp.json                # project-scoped MCP servers (ECOS-01 input)
CLAUDE.md / AGENTS.md    # hierarchy: user → project → subtree (ass-guard uses AGENTS.md today)
~/.claude.json           # global config: history, project settings, user-scoped MCP servers
~/.claude/               # user-scoped equivalents (settings.json, commands/, skills/, ...)
```

### `.mcp.json` format (MCP hosting input — ECOS-01)
```json
{
  "mcpServers": {
    "server-name": {
      "command": "npx",
      "args": ["-y", "@some/mcp-server"],
      "env": { "API_KEY": "..." },
      "cwd": "/optional"
    }
  }
}
```
User-scoped MCP servers live under `mcpServers` in `~/.claude.json` (same shape). ass-guard merges project + user lists (project wins on name collision — same precedence direction as the rest).

### Skills/commands/plugins discovery rules (ECOS-04 — replicate Claude Code)
- **Skill:** a directory containing `SKILL.md` (YAML frontmatter: `name`, `description`, optional `allowed-tools`; body = instructions). Discovered under `skills/`. Loaded read-only.
- **Slash-command:** a `<name>.md` file under `commands/` (YAML frontmatter + markdown body; `$ARGUMENTS` placeholder). Discovered by filename stem.
- **Plugin:** a manifest under `plugins/` declaring the plugin's own skills/commands/MCP servers. ass-guard treats a plugin as a container that contributes to the same three buckets.
- **Precedence (D-06):** `.claude/` wins over `.ass-guard/`; within each, user-scope (`~/.claude/`) → project-scope (`.claude/`) — Claude Code's own direction. ass-guard NEVER writes to `.claude/` (read-only; ECOS-05 "never clobbering").

### ass-guard's own additions (D-05)
```
.ass-guard/
  <session-id>.jsonl     # transcripts (Phase 2 D-06 — already here)
  learned/               # learned settings (Phase 4 D-16)
  skills/                # ass-guard's OWN skills (Phase 5 NEW)
  commands/              # ass-guard's OWN slash-commands (Phase 5 NEW)
  plugins/               # ass-guard's OWN plugins (Phase 5 NEW)
  .gitignore             # self-gitignore (Phase 2 D-07 — already covers new subdirs)
```
The existing `.ass-guard/.gitignore` (`*` + `!.gitignore`) already excludes these new subdirs from git — no new gitignore discipline needed (verify in plan execution).
</claude_code_layout>

<schema_drift_engineering>
## N13/N14 schema-drift pitfalls — engineered out (ECOS-03)

- **N13 (stale-cache drift):** if ass-guard cached MCP tool schemas across sessions, an MCP server upgraded between sessions would serve stale schemas → the model sees the old tool shapes → mimicry parity breaks AND tool-calls fail. **Engineered out (D-03):** `tools/list` is re-fetched on EVERY connection (session start). No persistent cache. The discovered schemas live only for the session.
- **N14 (in-session drift):** if a long-lived MCP server changed its tools mid-session, ass-guard's session catalog would diverge. **Accepted:** within a session the catalog is fixed (re-listing mid-session is out of scope for v1 — the MCP servers ass-guard hosts are stable within a session; mid-session tool changes are a v2 concern). The per-session re-fetch is the v1 mitigation: drift never persists across sessions.
- **Mimicry consequence (D-16):** the observed 77/97/103 tool-count variation across sessions is NOW explained — the variable tail is exactly the per-session MCP tools. The profile declares the stable built-in core; MCP tools are appended per-session. This is expected session-dependence, not drift.
</schema_drift_engineering>

<security_threat_model>
## Per-interface threat model (ROADMAP Research Flag — N15)

ROADMAP: "Per-interface threat model (N15) is a PROJECT.md decision: ACP ungated vs Telegram-gated allowlist for mutating MCP tools. (Telegram itself is v2, but the MCP threat-model decision lands here since MCP is v1.)"

**For Phase 5 (v1 = ACP only), the threat model is:**
- **Surfaces (v1):** ACP (IDE-native) only. Telegram is v2 (deferred).
- **MCP tool mutability:** MCP tools have a `Mutability` derived from their schema (heuristic: write/edit/bash-like names → mutating; default read-only). ass-guard's existing boundary semantics (Phase 2 — `toolcat.IsBoundary`, `ConfigAdded`) apply to MCP tools: a mutating MCP tool triggers a context-boundary, same as a built-in mutating tool.
- **Subagent restriction (D-10, already built):** `RestrictedExecutor` enforces allow-lists in subagent contexts. MCP tools are NOT in any subagent's default allow-list unless explicitly granted — so a subagent cannot invoke mutating MCP tools unless the parent explicitly allowed it.
- **ACP = ungated (v1):** the developer at the IDE is the trusted principal. MCP tools run with the developer's filesystem/process privileges (same as Claude Code). No additional allowlist for ACP in v1.
- **Process isolation (D-02):** MCP servers run as subprocesses in their own process group, NOT in ass-guard's address space. A crashing/compromised MCP server cannot corrupt ass-guard's memory; ass-guard can SIGKILL the group. This is a defense-in-depth win.
- **Secrets (LOG-03):** MCP server `env` (e.g. API keys) is read from config and passed to the subprocess; it is NOT logged in the transcript (the redactor scrubs `env` blocks). This reuses Phase 2's redaction.

**Per-plan threat_model blocks (step 5.55 gate) will reference this.**
</security_threat_model>

<validation_architecture>
## Validation Architecture

**How Phase 5 work is proven (Nyquist-aligned — test the behavior at the API boundary, not the implementation):**

### ECOS-01 (MCP server runs + tools bridged + callable)
- **Boundary under test:** the MCP host entry point (`mcp.Host.Start(ctx, config) → host.Register(catalog, profile)` + a tool-call round-trip).
- **Test architecture:** an in-process echo MCP server built with the SAME go-sdk (`mcp.NewServer` + `mcp.NewTool` + `Serve` over an in-memory transport, OR a tiny `exec.Command(os.Args[0], "-test.mcp-echo")` self-exec subprocess to exercise the REAL stdio path). The self-exec pattern exercises `CommandTransport` end-to-end without an external binary dependency.
- **Coverage targets:** spawn → ListTools returns N tools → each tool registered as `mcp__<server>__<tool>` in the catalog → a model tool-call for `mcp__echo__hello` flows through the MCP-aware executor → CallTool result returned → recorded in transcript. Sample at the rate of change: one test per distinct tool-routing path (success, server-error, not-found).

### ECOS-02 (lifecycle robust)
- **Boundary under test:** the lifecycle wrapper (`host.Close()` / shutdown path).
- **Test architecture:** the MCP server subprocess spawns a CHILD (grandchild from ass-guard's view) that writes a PID file; on shutdown, assert the grandchild's PID is no longer alive (group signal reached it) AND `Wait4` reaped it (no zombie entry in the process table). Time-bounded (3s grace → SIGKILL).
- **Nyquist note:** lifecycle is a low-frequency signal (spawn/shutdown happen once per session) — test the edges (graceful SIGTERM path, force-SIGKILL path, reaper-under-load) rather than re-sampling the happy path.

### ECOS-03 (re-fetch every connection)
- **Boundary under test:** `host.Start` calls `ListTools` on every invocation; no cache is consulted.
- **Test architecture:** two sessions against the same config but with the echo server's tool list mutated between them (env var flips the tool count) → assert session 2 sees the NEW count, proving no cross-session cache. Plus a negative assertion: no on-disk cache file is written/consulted.

### ECOS-04 (skills/commands/plugins load from .claude/)
- **Boundary under test:** the loader entry point (`ecosys.Load(claudeDir, assguardDir) → registry`).
- **Test architecture:** table-driven over fixture trees (a `.claude/skills/foo/SKILL.md`, `.claude/commands/bar.md`, `.claude/plugins/p/manifest.json`) → assert each is discovered with its metadata. Sample at the discovery rules: frontmatter parse, `$ARGUMENTS` placeholder, missing-SKILL.md skip.

### ECOS-05 (namespacing + precedence)
- **Boundary under test:** the merged registry's precedence resolution.
- **Test architecture:** conflicting names in `.claude/` vs `.ass-guard/` → assert `.claude/` wins; user-scope vs project-scope within each → assert project wins (Claude Code direction); assert no write ever touches `.claude/` (read-only enforcement — open with `O_RDONLY`, never `O_WRONLY`).

### Validation cadence
- `go test ./internal/mcp/... ./internal/toolcat/... ./internal/ecosys/...` per task; `go build ./...` + `go vet ./...` every task; the end-to-end ACP→session→MCP round-trip is the phase-level integration gate.
</validation_architecture>

<recommendation>
## Recommendation (Confidence: HIGH)

Three plans, tracer-first, MVP vertical slices:

1. **05-01 (TRACER, Wave 1):** MCP host core + tool bridge — ECOS-01 + ECOS-03. Add go-sdk dep, `internal/mcp`, `toolcat.Catalog.Register`, MCP-aware `ToolExecutor`, per-session profile merge, wire into `sessionFor`. End-to-end test: echo MCP server → model tool-call → result.
2. **05-02 (Wave 2, depends on 05-01):** MCP lifecycle robustness — ECOS-02. Process-group spawn, group-signal shutdown, reaper goroutine, session shutdown hook.
3. **05-03 (Wave 1, parallel):** Skills/commands/plugins loader + namespacing — ECOS-04 + ECOS-05. `internal/ecosys` loader, `.claude/` read-only + `.ass-guard/` own, `.claude/` wins precedence, user→project scope.

All 5 REQ-IDs covered. TDD applies to ECOS-01/02/03/04/05 (defined I/O contracts → table-driven tests with the in-process echo MCP server + fixture trees).
</recommendation>

<open_items>
## Open items / risks

- **go-sdk exact API drift:** the SDK is at v1.0.0-v1.1.0; the precise `CommandTransport`/`ClientSession` field names should be confirmed at `go get` time by reading the vendored module. If `CommandTransport` does not expose the `exec.Cmd` for `SysProcAttr` injection (D-02 needs it), fall back to constructing the `exec.Cmd` with `Setpgid` and passing it through whatever command-accepting constructor the SDK provides (NewStdioMCPClientCommand or equivalent). The plan's first task verifies this against the pinned module.
- **In-process vs self-exec MCP test server:** the in-memory transport tests the bridge logic but NOT the real stdio subprocess path. The self-exec pattern (`exec.Command(os.Args[0], "-test.mcp-echo")` gated by a `TestMain` flag) tests the real path including `SysProcAttr`. Plan 05-01 uses both: in-memory for unit speed, self-exec for the tracer integration.
- **Reaper on macOS vs Linux:** `Wait4(-pgid,...)` semantics are equivalent on both, but the test that asserts "grandchild reaped" must run on a real process table (skip on short via `testing.Short()`). CI matrix is darwin+linux (matches PROJECT.md v1 platforms).
</open_items>

---

*Phase: 5-Ecosystem Compatibility*
*Research completed: 2026-08-09*
