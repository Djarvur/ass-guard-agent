package mcp //nolint:testpackage // internal package test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Djarvur/ass-guard-agent/internal/toolcat"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// echoConfig builds a ServerConfig that re-execs the test binary as the echo
// server. The optional extraEnv entries are added to the subprocess env so two
// starts can observe different tool lists (the no-cache proof).
func echoConfig(t *testing.T, extraEnv ...string) Config {
	t.Helper()

	testBin := os.Args[0]
	// Inherit a minimal env so the subprocess can find its runtime; overlay extra.
	cmdEnv := make([]string, 0, 1+len(extraEnv))
	cmdEnv = append(cmdEnv, "PATH="+os.Getenv("PATH"))
	cmdEnv = append(cmdEnv, extraEnv...)

	return Config{Servers: []ServerConfig{{
		Name:    echoServerName,
		Command: testBin,
		Args:    []string{echoServerArgs},
		Env:     envMap(cmdEnv),
	}}}
}

const echoServerName = "echo"

// envMap converts KEY=VAL lines to a map (later entries override).
func envMap(lines []string) map[string]string {
	m := map[string]string{}

	for _, l := range lines {
		if before, after, ok := strings.Cut(l, "="); ok {
			m[before] = after
		}
	}

	return m
}

// TestHostStartSpawnConnectList proves the spawn+connect+list path: Start spawns
// the echo server via go-sdk CommandTransport, Connect succeeds, and ListTools
// returns BOTH echo tools (T2 Test 1).
func TestHostStartSpawnConnectList(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, decls, err := Start(ctx, echoConfig(t))
	require.NoError(t, err)

	defer func() { _ = host.Close() }()

	names := declNames(decls)
	assert.Contains(t, names, mcpToolName(echoServerName, echoHelloTool))
	assert.Contains(t, names, mcpToolName(echoServerName, echoCountTool))
}

// TestHostNamingD04 asserts the Decls are named mcp__echo__hello and
// mcp__echo__tools-count exactly (D-04 — T2 Test 2).
func TestHostNamingD04(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, decls, err := Start(ctx, echoConfig(t))
	require.NoError(t, err)

	defer func() { _ = host.Close() }()

	names := declNames(decls)
	require.Contains(t, names, "mcp__echo__hello")
	require.Contains(t, names, "mcp__echo__tools-count")
}

// TestHostNoCacheD03 proves tools/list is re-fetched per Start with NO
// cross-session cache: Start #1 (no env) sees 2 tools, Start #2 (with
// ECHO_EXTRA_TOOL) sees 3 tools. Also asserts no cache file is written to disk
// (ECOS-03 — T2 Test 3).
func TestHostNoCacheD03(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns subprocesses")
	}

	if runtime.GOOS == "windows" { //nolint:goconst // platform check
		t.Skip("process-group test is darwin/linux only")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Session 1: no extra tool.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel1()

	host1, decls1, err := Start(ctx1, echoConfig(t))
	require.NoError(t, err)

	require.Len(t, decls1, 2, "session 1 should see exactly 2 echo tools")
	require.NoError(t, host1.Close())

	// Session 2: extra tool enabled via env — must see 3 (no stale cache).
	ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()

	host2, decls2, err := Start(ctx2, echoConfig(t, echoExtraEnv+"=1"))
	require.NoError(t, err)

	defer func() { _ = host2.Close() }()

	require.Len(t, decls2, 3, "session 2 should see 3 echo tools (no cross-session cache)")
	assert.Contains(t, declNames(decls2), mcpToolName(echoServerName, echoExtraTool))

	// No cache artifacts written under HOME.
	assertNoCacheFiles(t, tmpHome)
}

// TestHostCallToolRouting proves CallTool routes mcp__echo__hello to the echo
// server and returns its text (ECOS-01 — T2 Test 4).
func TestHostCallToolRouting(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, _, err := Start(ctx, echoConfig(t))
	require.NoError(t, err)

	defer func() { _ = host.Close() }()

	out, err := host.CallTool(ctx, "mcp__echo__hello", json.RawMessage(`{"name":"world"}`))
	require.NoError(t, err)

	var res map[string]string
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, "hello, world", res[toolResultKey])

	countOut, err := host.CallTool(ctx, "mcp__echo__tools-count", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal(countOut, &res))
	assert.Equal(t, "2", res[toolResultKey])
}

// TestHostCallToolUnknownTool asserts an unknown tool returns a non-nil error
// mentioning the server/tool (T2 Test 5).
func TestHostCallToolUnknownTool(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, _, err := Start(ctx, echoConfig(t))
	require.NoError(t, err)

	defer func() { _ = host.Close() }()

	_, err = host.CallTool(ctx, "mcp__echo__nope", json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), echoServerName)
}

// TestHostCloseShutsSessions asserts Close shuts every ClientSession; a
// post-Close CallTool errors (T2 Test 6).
func TestHostCloseShutsSessions(t *testing.T) { //nolint:paralleltest // spawns a subprocess
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, _, err := Start(ctx, echoConfig(t))
	require.NoError(t, err)

	require.NoError(t, host.Close())

	_, err = host.CallTool(ctx, "mcp__echo__hello", json.RawMessage(`{}`))
	require.Error(t, err)
}

// TestParseMCPName covers the name parser (unit-fast, no subprocess).
func TestParseMCPName(t *testing.T) { //nolint:paralleltest // table/serial test
	tests := []struct {
		name    string
		in      string
		wantSrv string
		wantTl  string
		wantErr bool
	}{
		{"canonical", "mcp__echo__hello", echoServerName, "hello", false},
		{"count", "mcp__echo__tools-count", echoServerName, "tools-count", false},
		{"no prefix", "echo__hello", "", "", true},
		{"malformed", "mcp__echo", "", "", true},
		{"empty server", "mcp____hello", "", "", true},
	}

	for _, tc := range tests { //nolint:paralleltest // table test uses shared helper
		t.Run(tc.name, func(t *testing.T) {
			srv, tl, err := parseMCPName(tc.in)
			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantSrv, srv)
			assert.Equal(t, tc.wantTl, tl)
		})
	}
}

// TestLoadConfigMissingFile asserts a missing .mcp.json yields an empty Config,
// nil error (MCP is opt-in).
func TestLoadConfigMissingFile(t *testing.T) { //nolint:paralleltest // table/serial test
	cfg, err := LoadConfig(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, cfg.Servers)
}

// TestLoadConfigParsesMcpJSON asserts LoadConfig reads the mcpServers object.
func TestLoadConfigParsesMcpJSON(t *testing.T) { //nolint:paralleltest // table/serial test
	dir := t.TempDir()
	content := `{"mcpServers":{"echo":{"command":"/bin/echo","args":["x"],"env":{"K":"V"}}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, mcpJSONFile), []byte(content), 0o600))

	cfg, err := LoadConfig(dir)
	require.NoError(t, err)
	require.Len(t, cfg.Servers, 1)
	assert.Equal(t, "echo", cfg.Servers[0].Name)
	assert.Equal(t, "/bin/echo", cfg.Servers[0].Command)
	assert.Equal(t, []string{"x"}, cfg.Servers[0].Args)
	assert.Equal(t, "V", cfg.Servers[0].Env["K"])
}

// declNames extracts the Name field from a Decl slice.
func declNames(decls []toolcat.Decl) []string {
	out := make([]string, len(decls))
	for i, d := range decls {
		out[i] = d.Name
	}

	return out
}

// assertNoCacheFiles asserts no cache artifact was written under dir.
func assertNoCacheFiles(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		assert.NotContains(t, strings.ToLower(e.Name()), "cache",
			"unexpected cache file: %s", e.Name())
	}
}
