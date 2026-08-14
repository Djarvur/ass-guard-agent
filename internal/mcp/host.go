package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"

	mcpgo "github.com/modelcontextprotocol/go-sdk/mcp"
)

// errHostClosed is returned by CallTool after Close has shut every session.
var errHostClosed = errors.New("mcp: host is closed")

// nameSplitParts is the SplitN count for parsing mcp__<server>__<tool> names.
const nameSplitParts = 2

// ServerConfig describes one MCP server to spawn (mirrors a `.mcp.json` entry).
// The Name identifies the server in the catalog namespace (mcp__<Name>__<tool>).
type ServerConfig struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
	Cwd     string
}

// Config is the set of MCP servers ass-guard hosts (D-01).
type Config struct {
	Servers []ServerConfig
}

// Host owns the live MCP client sessions for one ass-guard session. Each session
// starts its own Host (so tool lists are per-session — D-03/ECOS-03) and closes
// it on shutdown. The Host holds NO persistent cross-session state: every Start
// re-lists tools (schema drift N13/N14 engineered out).
//
// Plan 05-02 hardens the subprocess lifecycle (D-02/ECOS-02): each server runs
// in its own process group (Setpgid), and Close sends a group signal
// (SIGTERM→SIGKILL) that reaches grandchildren the SDK's process-only signal
// misses, then drains any remaining zombies via the reaper.
type Host struct {
	cfg Config

	mu       sync.Mutex
	sessions map[string]*mcpgo.ClientSession // server name -> live session
	decls    []toolcat.Decl                  // discovered MCP tool decls (mcp__<server>__<tool>)
	pgids    map[string]int                  // server name -> process-group ID (== server PID)
	closed   bool
	closeMu  sync.Mutex
}

// LoadConfig reads the project-scope `.mcp.json` from dir and returns the server
// list. A missing file yields an empty Config + nil error (MCP is opt-in). The
// `~/.claude.json` user-scope merge is owned by internal/ecosys
// (LoadUserMCPConfig); this loader reads ONLY the project file.
func LoadConfig(dir string) (Config, error) {
	data, err := os.ReadFile(filepath.Join(dir, mcpJSONFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil // opt-in: no .mcp.json is the common case
		}

		return Config{}, fmt.Errorf("read %s: %w", mcpJSONFile, err)
	}

	var raw struct {
		McpServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			Cwd     string            `json:"cwd"`
		} `json:"mcpServers"` //nolint:tagliatelle // .mcp.json camelCase wire field
	}

	err = json.Unmarshal(data, &raw)
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", mcpJSONFile, err)
	}

	cfg := Config{}
	for name, s := range raw.McpServers {
		cfg.Servers = append(cfg.Servers, ServerConfig{
			Name: name, Command: s.Command, Args: s.Args, Env: s.Env, Cwd: s.Cwd,
		})
	}

	return cfg, nil
}

// Start spawns every configured server, Connects each via the go-sdk
// CommandTransport, and calls ListTools on every connection (D-03 — no cache).
// It returns the Host plus the aggregated toolcat.Decl list (named
// mcp__<server>__<tool>) for the Shaper to surface to the model. A server that
// fails to Connect or ListTools is skipped (logged via slog; non-fatal per
// threat T-5-04) so one bad server does not break the session.
func Start(ctx context.Context, cfg Config) (*Host, []toolcat.Decl, error) {
	h := &Host{cfg: cfg, sessions: map[string]*mcpgo.ClientSession{}, pgids: map[string]int{}}

	for _, sc := range cfg.Servers {
		decls, err := h.connectOne(ctx, &sc)
		if err != nil {
			// Non-fatal: skip this server, keep the rest (T-5-04).
			continue
		}

		h.decls = append(h.decls, decls...)
	}

	return h, h.decls, nil
}

// Register merges the discovered MCP tools into the catalog, each with an
// Execute closure that routes through CallTool (D-01/D-04). This is how MCP tools
// become catalog-callable alongside the built-in core.
func (h *Host) Register(c *toolcat.Catalog) {
	for _, d := range h.decls {
		c.Register(toolcat.Tool{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: d.InputSchema,
			Execute: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
				return h.CallTool(ctx, d.Name, input)
			},
		})
	}
}

// Decls returns the discovered MCP tool declarations (named mcp__<server>__<tool>).
func (h *Host) Decls() []toolcat.Decl {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]toolcat.Decl, len(h.decls))
	copy(out, h.decls)

	return out
}

// CallTool routes an mcp__<server>__<tool> call to the matching MCP session. The
// input json.RawMessage is unmarshaled into the CallToolParams.Arguments map. The
// first TextContent block is returned as {"output": <text>}; an IsError result or
// unknown server/tool yields an error.
func (h *Host) CallTool(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	server, tool, err := parseMCPName(name)
	if err != nil {
		return nil, err
	}

	h.mu.Lock()
	session, ok := h.sessions[server]
	closed := h.closed
	h.mu.Unlock()

	if closed {
		return nil, errHostClosed
	}

	if !ok {
		return nil, fmt.Errorf( //nolint:err113 // per-call msg
			"mcp: unknown server %q for tool %q", server, name)
	}

	args := map[string]any{}
	if len(input) > 0 {
		err = json.Unmarshal(input, &args)
		if err != nil {
			return nil, fmt.Errorf("mcp: parse arguments for %q: %w", name, err)
		}
	}

	res, err := session.CallTool(ctx, &mcpgo.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("mcp call %q: %w", name, err)
	}

	return extractTextResult(name, res)
}

// Close shuts every ClientSession (protocol-level shutdown) and then sends a
// group signal (SIGTERM→SIGKILL) to each server's process group so grandchildren
// the SDK's process-only signal missed are reached, followed by a reaper drain
// to collect any remaining zombies (D-02/ECOS-02). Idempotent + concurrency-safe
// (closeMu serializes concurrent calls; the first wins).
func (h *Host) Close() error {
	h.closeMu.Lock()
	defer h.closeMu.Unlock()

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()

		return nil
	}

	h.closed = true
	sessions := h.sessions
	pgids := h.pgids
	h.mu.Unlock()

	var firstErr error

	for name, session := range sessions {
		// 1. Protocol-level shutdown (SDK SIGTERM→SIGKILL on the server process +
		//    cmd.Wait reaps the server itself).
		err := session.Close()
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("mcp close session %q: %w", name, err)
		}

		// 2. Group signal reaches grandchildren the SDK's process-only signal
		//    missed (the value of process-group isolation — D-02).
		pgid := pgids[name]
		if pgid > 0 {
			_ = signalGroup(pgid, shutdownGrace)
			// 3. Drain any remaining zombies ass-guard can reap.
			reap(pgid)
		}
	}

	return firstErr
}

// PGID returns the process-group ID for a spawned server (== the server's PID
// under Setpgid) and ok=true if the server is live. Used by tests + the shutdown
// path.
func (h *Host) PGID(server string) (int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	pgid, ok := h.pgids[server]

	return pgid, ok
}

// connectOne spawns one server, Connects, lists tools, and records the session.
// Returns the discovered tool decls. On any error the session is torn down so no
// half-open subprocess leaks. The server is spawned in its own process group
// (D-02) via prepareCommand; the PGID is recorded for group-signal shutdown.
func (h *Host) connectOne(ctx context.Context, sc *ServerConfig) ([]toolcat.Decl, error) {
	// The subprocess is intentionally NOT bound to ctx via CommandContext: it must
	// outlive the (short, bounded) spawn ctx and live for the full session. Its
	// lifecycle is managed by Close (group signal + reaper), not a context.
	cmd := exec.Command(sc.Command, sc.Args...) //nolint:gosec,noctx // G204 trusted config; lifecycle via Close

	err := prepareCommand(cmd, sc)
	if err != nil {
		return nil, fmt.Errorf("prepare mcp server %q: %w", sc.Name, err)
	}

	transport := &mcpgo.CommandTransport{Command: cmd}
	client := mcpgo.NewClient(&mcpgo.Implementation{Name: mcpClientName, Version: mcpClientVersion}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect mcp server %q: %w", sc.Name, err)
	}

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		_ = session.Close()

		return nil, fmt.Errorf("list tools %q: %w", sc.Name, err)
	}

	h.mu.Lock()
	h.sessions[sc.Name] = session
	h.pgids[sc.Name] = groupID(cmd)
	h.mu.Unlock()

	decls := make([]toolcat.Decl, 0, len(tools.Tools))
	for _, t := range tools.Tools {
		decls = append(decls, toolcat.Decl{
			Name:        mcpToolName(sc.Name, t.Name),
			Description: t.Description,
			InputSchema: extractSchema(t.InputSchema),
		})
	}

	return decls, nil
}

// extractTextResult pulls the first TextContent block from a CallToolResult into
// {"output": <text>}. An IsError result yields an error mentioning the tool.
func extractTextResult(name string, res *mcpgo.CallToolResult) (json.RawMessage, error) {
	for _, c := range res.Content {
		tc, ok := c.(*mcpgo.TextContent)
		if !ok {
			continue
		}

		out, mErr := json.Marshal(map[string]string{toolResultKey: tc.Text})
		if mErr != nil {
			return nil, fmt.Errorf("mcp: marshal result for %q: %w", name, mErr)
		}

		if res.IsError {
			return out, fmt.Errorf( //nolint:err113 // per-call msg
				"mcp tool %q returned error: %s", name, tc.Text)
		}

		return out, nil
	}

	emptyResult := json.RawMessage(`{"` + toolResultKey + `":""}`)

	if res.IsError {
		return emptyResult, fmt.Errorf("mcp tool %q returned error", name) //nolint:err113 // dynamic per-call msg
	}

	return emptyResult, nil
}

// mcpToolName builds the catalog name for an MCP tool (D-04).
func mcpToolName(server, tool string) string {
	return mcpPrefix + server + "__" + tool
}

// parseMCPName splits mcp__<server>__<tool> into (server, tool). Server and tool
// names are assumed not to contain "__".
//
//nolint:nonamedreturns // gocritic unnamedResult prefers names for two-string clarity
func parseMCPName(name string) (server, tool string, err error) {
	if !strings.HasPrefix(name, mcpPrefix) {
		return "", "", fmt.Errorf( //nolint:err113 // per-call msg
			"mcp: %q is not an mcp tool (missing %s prefix)", name, mcpPrefix)
	}

	rest := strings.TrimPrefix(name, mcpPrefix)

	parts := strings.SplitN(rest, "__", nameSplitParts)
	if len(parts) != nameSplitParts || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("mcp: malformed tool name %q", name) //nolint:err113 // dynamic per-call msg
	}

	return parts[0], parts[1], nil
}

// mergeEnv returns os.Environ() plus the config env overlay.
func mergeEnv(extra map[string]string) []string {
	out := os.Environ()
	for k, v := range extra {
		out = append(out, k+"="+v)
	}

	return out
}

// extractSchema marshals the SDK's `any` InputSchema to json.RawMessage. A nil or
// empty schema defaults to the canonical empty-object schema.
func extractSchema(schema any) json.RawMessage {
	if schema == nil {
		return json.RawMessage(emptyObjectSchema)
	}

	b, err := json.Marshal(schema)
	if err != nil {
		return json.RawMessage(emptyObjectSchema)
	}

	if len(b) == 0 || string(b) == nullLiteral {
		return json.RawMessage(emptyObjectSchema)
	}

	return b
}
