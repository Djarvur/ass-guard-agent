package mcp

// Shared constants (golangci goconst-friendly). Kept in one place so the
// no-cache / naming contract is greppable.
const (
	mcpPrefix         = "mcp__"
	mcpJSONFile       = ".mcp.json"
	mcpClientName     = "ass-guard"
	mcpClientVersion  = "v0"
	toolResultKey     = "output"
	emptyObjectSchema = `{"type":"object"}`
	nullLiteral       = "null"

	// Echo server (test-only) tool names + env flag.
	echoHelloTool = "hello"
	echoCountTool = "tools-count"
	echoExtraTool = "extra"
	echoExtraEnv  = "ECHO_EXTRA_TOOL"
)
