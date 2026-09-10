package mcp //nolint:testpackage // internal package test (accesses unexported symbols)

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"testing"
	"time"

	mcpgo "github.com/modelcontextprotocol/go-sdk/mcp"
)

// echoServerArgs is the self-exec flag the test binary recognizes to run as the
// echo MCP server over stdio (the REAL CommandTransport stdio path — D-03
// tracer). The test suite spawns os.Args[0] with this flag.
const echoServerArgs = "__ass_guard_mcp_echo_server"

// grandchildServerArg runs an echo server that ALSO forks a grandchild (which
// writes its PID + PGID to a file named by $GRANDCHILD_PIDFILE). Used by the
// lifecycle tests (T1/T2/T3) to prove process-group isolation + group-signal
// reach.
const grandchildServerArg = "__ass_guard_grandchild_server"

// grandchildChildArg is the grandchild itself: writes PID+PGID to $GC_PIDFILE,
// then sleeps until killed. It proves the grandchild inherited the server's
// process group (PGID == server PID under Setpgid).
const grandchildChildArg = "__ass_guard_grandchild_child"

// sigtermIgnoreArg runs an echo server that IGNORES SIGTERM (so only the
// escalated SIGKILL reaches it). Used by the force-kill lifecycle test (T2).
const sigtermIgnoreArg = "__ass_guard_sigterm_ignore_server"

// pidFileEnv / gcPidFileEnv name the env vars the fixtures write to.
const (
	pidFileEnv   = "GRANDCHILD_PIDFILE"
	gcPidFileEnv = "GC_PIDFILE"
)

// TestMain intercepts the self-exec modes. When the test binary is re-invoked
// with a fixture flag, it runs the requested MCP server over stdio and exits.
// Otherwise it runs the normal test suite. This exercises the real subprocess
// stdio CommandTransport path end-to-end without an external binary.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case echoServerArgs:
			runEchoServer()
			os.Exit(0)
		case grandchildServerArg:
			runGrandchildServer()
			os.Exit(0)
		case grandchildChildArg:
			runGrandchildChild()
			os.Exit(0)
		case sigtermIgnoreArg:
			runSigtermIgnoreServer()
			os.Exit(0)
		}
	}

	os.Exit(m.Run())
}

// runEchoServer builds a go-sdk MCP server exposing `hello` (returns "hello,
// {name}") and `tools-count` (returns the live tool count). When ECHO_EXTRA_TOOL
// is set, a third `extra` tool is registered, so two Start calls against the same
// binary with different env produce different tool lists (the no-cache proof —
// Test 3 / ECOS-03). The server speaks the real stdio transport.
func runEchoServer() {
	ctx := context.Background()
	srv := mcpgo.NewServer(&mcpgo.Implementation{Name: echoServerName, Version: "v0"}, nil)

	objSchema := json.RawMessage(emptyObjectSchema)

	srv.AddTool(&mcpgo.Tool{
		Name: echoHelloTool, Description: "Echo a greeting", InputSchema: objSchema,
	}, echoHelloHandler)

	srv.AddTool(&mcpgo.Tool{
		Name: echoCountTool, Description: "Report the tool count", InputSchema: objSchema,
	}, echoCountHandler)

	if os.Getenv(echoExtraEnv) != "" {
		srv.AddTool(&mcpgo.Tool{
			Name: echoExtraTool, Description: "Extra tool (env-gated)", InputSchema: objSchema,
		}, echoExtraHandler)
	}

	_ = srv.Run(ctx, &mcpgo.StdioTransport{})
}

// echoHelloHandler returns "hello, {name}" (default "world").
func echoHelloHandler(_ context.Context, req *mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	name := "world"

	if len(req.Params.Arguments) > 0 {
		var args map[string]any

		err := json.Unmarshal(req.Params.Arguments, &args)
		if err == nil {
			if n, ok := args["name"].(string); ok {
				name = n
			}
		}
	}

	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{&mcpgo.TextContent{Text: "hello, " + name}},
	}, nil
}

// echoCountHandler returns the current tool count (2 normally, 3 when extra).
func echoCountHandler(_ context.Context, _ *mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	count := 2
	if os.Getenv(echoExtraEnv) != "" {
		count = 3
	}

	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{&mcpgo.TextContent{Text: strconv.Itoa(count)}},
	}, nil
}

// echoExtraHandler is the env-gated third tool.
func echoExtraHandler(_ context.Context, _ *mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{&mcpgo.TextContent{Text: "extra"}},
	}, nil
}

// newEchoServer builds the echo MCP server with the hello + tools-count tools.
// Shared by the plain echo + grandchild fixtures.
func newEchoServer() *mcpgo.Server {
	srv := mcpgo.NewServer(&mcpgo.Implementation{Name: echoServerName, Version: "v0"}, nil)
	objSchema := json.RawMessage(emptyObjectSchema)

	srv.AddTool(&mcpgo.Tool{
		Name: echoHelloTool, Description: "Echo a greeting", InputSchema: objSchema,
	}, echoHelloHandler)
	srv.AddTool(&mcpgo.Tool{
		Name: echoCountTool, Description: "Report the tool count", InputSchema: objSchema,
	}, echoCountHandler)

	return srv
}

// runGrandchildServer runs the echo server AND forks a grandchild that writes
// its PID + PGID to $GRANDCHILD_PIDFILE. The grandchild inherits the server's
// process group, proving Setpgid isolation.
func runGrandchildServer() {
	ctx := context.Background()

	// Fork the grandchild BEFORE serving (it writes the PID file ASAP).
	pidFile := os.Getenv(pidFileEnv)
	if pidFile != "" {
		_ = exec.CommandContext(ctx, os.Args[0], grandchildChildArg, pidFile).Start()
		// Give the grandchild a moment to write its PID file.
		time.Sleep(100 * time.Millisecond)
	}

	srv := newEchoServer()
	_ = srv.Run(ctx, &mcpgo.StdioTransport{})
}

// runGrandchildChild is the grandchild: writes "pid pgid" to the pidfile passed
// as argv[2], then sleeps until killed.
func runGrandchildChild() {
	if len(os.Args) < 3 {
		return
	}

	pidFile := os.Args[2]
	pid := os.Getpid()
	pgid, err := syscall.Getpgid(pid)

	_ = os.WriteFile(pidFile, fmt.Appendf(nil, "%d %d", pid, pgid), 0o600)

	if err != nil {
		return
	}

	// Sleep until killed (the group signal reaches us).
	for {
		time.Sleep(time.Hour)
	}
}

// runSigtermIgnoreServer runs the echo server but IGNORES SIGTERM, so only the
// escalated SIGKILL terminates it. Used by the force-kill lifecycle test (T2).
func runSigtermIgnoreServer() {
	signal.Ignore(syscall.SIGTERM)

	ctx := context.Background()
	srv := newEchoServer()
	_ = srv.Run(ctx, &mcpgo.StdioTransport{})
}
