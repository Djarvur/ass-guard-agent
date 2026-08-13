// Package mcp hosts external MCP (Model Context Protocol) servers as subprocesses
// and bridges their tools into ass-guard's tool catalog (ECOS-01/02/03).
//
// This package wraps the official SDK github.com/modelcontextprotocol/go-sdk
// (pinned at v1.7.0) and adds the process-lifecycle discipline ass-guard needs:
// process-group spawn, group-signal shutdown, and a reaper goroutine (Plan 05-02).
//
// # Verified SDK API surface (go-sdk v1.7.0)
//
// The exported names below were verified against the vendored module at go get
// time. They are the load-bearing contract Host depends on.
//
//	// Client side — spawning a server + speaking JSON-RPC over stdio.
//	transport := &mcp.CommandTransport{Command: exec.Command(name, args...)}
//	//   CommandTransport exposes a SETTABLE *exec.Cmd field, so SysProcAttr
//	//   (Setpgid) can be set directly on cmd before Connect — see
//	//   lifecycle.prepareCommand. No alternative constructor is needed.
//	client := mcp.NewClient(&mcp.Implementation{Name: "ass-guard", Version: "v0"}, nil)
//	session, err := client.Connect(ctx, transport, nil) // *mcp.ClientSession
//	tools, err := session.ListTools(ctx, nil)            // *ListToolsResult; .Tools []*Tool
//	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: n, Arguments: args})
//	//   res.IsError bool; res.Content []Content (e.g. *mcp.TextContent{Text})
//	err = session.Close()                                 // idempotent, concurrency-safe
//
//	// Server side — the echo server used by tests (echoserver.go).
//	srv := mcp.NewServer(&mcp.Implementation{Name: "echo"}, nil)
//	srv.AddTool(&mcp.Tool{Name: "hello", Description: "...", InputSchema: objSchema},
//	    func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
//	        return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "..."}}, IsError: false}, nil
//	    })
//	err := srv.Run(ctx, &mcp.StdioTransport{})  // blocks until ctx cancel / disconnect
//
// There is NO NewTool constructor in v1.7.0 — tools are constructed as struct
// literals (&mcp.Tool{...}). ToolHandler is the low-level handler type
// (func(context.Context, *CallToolRequest) (*CallToolResult, error)).
//
// # SysProcAttr injection strategy (Plan 05-02)
//
// CommandTransport.Command is a settable *exec.Cmd. ass-guard constructs the
// exec.Cmd first, sets cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
// (plus cmd.Env / cmd.Dir) via lifecycle.prepareCommand, then wraps it in the
// transport. This is the canonical path — no SDK fork or workaround required.
//
// # No persistent cache (D-03 / ECOS-03)
//
// Start re-fetches tools/list on EVERY invocation. The package writes no
// on-disk cache and holds no cross-session state. Schema drift (N13/N14) is
// engineered out: each session sees the server's current tools.
package mcp
