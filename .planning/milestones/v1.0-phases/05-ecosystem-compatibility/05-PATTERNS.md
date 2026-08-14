# Phase 5: Ecosystem Compatibility — Pattern Map

**Mapped:** 2026-08-09
**Phase:** 05 — Ecosystem Compatibility
**Source:** `05-CONTEXT.md`, `05-RESEARCH.md`, real Phases 1-3 code (`internal/toolcat`, `internal/session`, `internal/profile`, `internal/shaper`, `cmd/ass-guard/acp_serve.go`), go-sdk README/pkg.go.dev, STACK Focus 5
**Files analyzed:** 9 new/modified files across `internal/mcp`, `internal/toolcat`, `internal/ecosys`, `cmd/ass-guard`, `.ass-guard/`
**Analogs found:** 6 / 9 (the rest are greenfield with external-SDK references)

> **Grounding caveat.** Phase 5 introduces one external dep (`modelcontextprotocol/go-sdk`) and two greenfield packages (`internal/mcp`, `internal/ecosys`). The MCP host has no in-repo analog — the reference is the go-sdk README client example + STACK Focus 5 (both dated, inspectable). Every other file maps to a real Phases 1-3 pattern (catalog, ToolExecutor wrapper, profile loader, sessionFor). Conventions C1-C4 (transport discipline, redaction, env-config, SDK-driven) carry forward unchanged.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/mcp/host.go` | service | subprocess (spawn → JSON-RPC → reap) | go-sdk README client example + STACK Focus 5 | role-match (external SDK) |
| `internal/mcp/lifecycle.go` | utility | subprocess (signal + reap) | Go stdlib `os/exec` + `syscall` (SysProcAttr/Wait4) | role-match (stdlib) |
| `internal/mcp/host_test.go` | test | integration (in-memory + self-exec echo) | `internal/toolcat/catalog_test.go` (external pkg, table-driven) | role-match (convention) |
| `internal/toolcat/catalog.go` (MODIFIED) | model | — | `internal/toolcat/catalog.go:NewCatalog` loop (`c.tools[e.Name] = e`) | exact (extends) |
| `internal/toolcat/mcpexec.go` (NEW) | wrapper | request-response (route mcp__*) | `internal/toolcat/restricted.go:RestrictedExecutor` | exact (parallel pattern) |
| `internal/ecosys/loader.go` | service | file-I/O (discover + parse) | `internal/profile/loader.go` (dir walk + yaml.v3/json) | role-match (extends) |
| `internal/ecosys/loader_test.go` | test | unit (fixture trees) | `internal/profile/loader_test.go` | role-match (convention) |
| `cmd/ass-guard/acp_serve.go` (MODIFIED) | command | — | `cmd/ass-guard/acp_serve.go:198 sessionFor` | exact (extends) |
| `.ass-guard/{skills,commands,plugins}/` (NEW) | artifact | — | `.ass-guard/.gitignore` (Phase 2 D-07 — already covers) | role-match (extends) |

---

## Pattern Assignments

### `internal/mcp/host.go` (service, subprocess lifecycle + tool bridge)

**No in-repo analog.** Reference: go-sdk README + STACK Focus 5 (both dated Aug 2026).

**Shape (from RESEARCH `<go_sdk_api>`):**
```go
transport := &mcp.CommandTransport{Command: cmd}   // cmd has SysProcAttr set by lifecycle.go
client := mcp.NewClient(&mcp.Implementation{Name: "ass-guard", Version: "v0"}, nil)
session, err := client.Connect(ctx, transport, nil) // *ClientSession; held for session lifetime
tools, err := session.ListTools(ctx, nil)           // D-03: every connection, no cache
// result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: ..., Arguments: ...})
defer session.Close()
```

**Convention alignment:** C4 (SDK-driven — same as anthropic-sdk-go/go-openai). The go-sdk is the authority for JSON-RPC framing; ass-guard does NOT hand-roll MCP framing (mirrors the decision not to hand-roll when a first-party SDK exists). C1 (stdout/stderr discipline): the SDK consumes subprocess stderr; ass-guard routes it to slog.

### `internal/mcp/lifecycle.go` (utility, process-group + reaper)

**Analog: Go stdlib** (`syscall.SysProcAttr`, `syscall.Kill`, `syscall.Wait4`). See RESEARCH `<process_lifecycle>` for exact code.

**Build constraints:** `//go:build darwin || linux` on the syscall-specific parts (the v1 platform set). A Windows stub returns `errors.New("windows MCP lifecycle: v2")` so the package compiles everywhere but Phase 5 only runs on darwin/linux.

### `internal/toolcat/catalog.go` — add `Register` (extends, exact analog)

**Analog: the existing `NewCatalog` loop** (`catalog.go:28-30`):
```go
for _, e := range entries {
    c.tools[e.Name] = e
}
```
Phase 5 adds the public entry point using the SAME map-write pattern:
```go
// Register adds (or overwrites) a tool entry. Used by the MCP host to merge
// session-discovered MCP tools into the catalog (D-01/D-04).
func (c *Catalog) Register(t Tool) { c.tools[t.Name] = t }
```
Plus a lenient `Register` variant or a dedicated `RegisterMCP` if mutability needs a default — keep it the simplest thing that lets MCP tools enter the map.

### `internal/toolcat/mcpexec.go` — MCP-aware ToolExecutor (EXACT parallel of RestrictedExecutor)

**Analog: `internal/toolcat/restricted.go`** — this is the strongest match in the codebase. `RestrictedExecutor` wraps a `ToolExecutor` and intercepts at the execution boundary; the MCP executor does the same with routing instead of allow-listing.

**Pattern (mirror `restricted.go`):**
```go
type MCPExecutor struct {
    inner toolcat.ToolExecutor           // fall-through for non-mcp__ tools
    host  *mcp.Host                       // routes mcp__<server>__<tool> calls
}
func NewMCPExecutor(inner toolcat.ToolExecutor, host *mcp.Host) *MCPExecutor { ... }
func (e *MCPExecutor) Execute(ctx, name, input) (json.RawMessage, error) {
    if isMCPName(name) { return e.host.CallTool(ctx, name, input) }
    return e.inner.Execute(ctx, name, input)
}
var _ toolcat.ToolExecutor = (*MCPExecutor)(nil)
```
The `isMCPName` check uses the `mcp__<server>__<tool>` prefix (D-04). The inner executor is whatever the Session already wires (today the stub/Phase-4 executor).

### `internal/ecosys/loader.go` — discovery + precedence (role-match with profile loader)

**Analog: `internal/profile/loader.go`** — walks a dir, loads YAML (yaml.v3) + JSON (encoding/json) files into typed structs. The ecosys loader does the same for SKILL.md frontmatter, command markdown, and plugin manifests.

**Convention C3 (env-config) + the project directive (yaml.v3, NOT viper):** `.claude/`-compat files (`settings.json`, `.mcp.json`) are JSON → `encoding/json`. ass-guard's own additions under `.ass-guard/` follow the existing `.ass-guard/` convention (YAML or JSON — match the profile loader's `loadYAML` helper).

**Precedence (D-06):** load `.claude/` first (read-only, `os.Open`/`os.ReadFile` only — never `os.Create`/`O_WRONLY`), then `.ass-guard/`; on name collision keep the `.claude/` entry. Within each dir, user-scope (`~/.claude/`) loaded before project-scope (`./.claude/`); project wins (Claude Code direction — mirror `profile/loader.go`'s override-on-load ordering).

### `cmd/ass-guard/acp_serve.go` — sessionFor extension (exact, extends self)

**Analog: the existing `sessionFor` at `acp_serve.go:198-212`.** Phase 5 inserts the MCP host spawn + profile merge between Manager creation and Session construction:

```go
// NEW (Phase 5): spawn MCP servers, build per-session profile copy
host, mcpDecls, err := mcp.StartHost(ctx, mcp.LoadConfig(dir))  // ECOS-01/02/03
prof := r.profile
if len(mcpDecls) > 0 {
    prof.Tools = append(append([]profile.Decl(nil), r.profile.Tools...), mcpDecls...)
}
s := &session.Session{
    ...
    Profile:   prof,                 // per-session copy with MCP tools appended
    Catalog:   toolcat.NewCatalog(), // MCP tools registered via host.Register(catalog)
    ...
}
```
And a corresponding shutdown call in the logout/cancel handler (`handleLogout` / `handleSessionCancel`) → `host.Close()`.

### `.ass-guard/{skills,commands,plugins}/` (artifact, extends Phase 2 D-06/D-07)

**Analog: `.ass-guard/.gitignore`** (Phase 2 D-07). The existing `.gitignore` (`*` + `!.gitignore`) ALREADY excludes these new subdirs — verify with `git check-ignore .ass-guard/skills/foo` in the plan task. No new gitignore line needed.

---

## Conventions Carried Forward (C1-C4)

| Convention | Source | How Phase 5 honors it |
|---|---|---|
| **C1 — transport discipline** (stdout/stderr split) | Phase 1 VERIFIED-FACTS | go-sdk consumes MCP subprocess stderr; ass-guard routes it to slog. MCP JSON-RPC stays on stdin/stdout. |
| **C2 — redaction** (LOG-03) | Phase 2 `internal/redact` | MCP server `env` (API keys) is scrubbed before the config object touches the transcript. Reuse the existing Redactor. |
| **C3 — env-config** (no hardcoded values) | Phase 1 D-16 | MCP server list comes from `.mcp.json` + `~/.claude.json`; skills/commands/plugins from the dir layout — nothing hardcoded. |
| **C4 — SDK-driven** (first-party SDKs win) | Phase 1 STACK | `modelcontextprotocol/go-sdk` is the authority for MCP framing; ass-guard does not hand-roll JSON-RPC to MCP servers. (Hand-rolled framing remains only for ACP, per Phase 1 decision.) |

---

*Phase: 5-Ecosystem Compatibility*
*Pattern map: 2026-08-09*
