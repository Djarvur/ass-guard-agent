// Command ass-guard is the entrypoint for the ass-guard agent.
//
// Phase 1 (Plan 01-01) ships a minimal tracer: it loads the zcode profile,
// runs one prompt through the test-harness Turn Loop against the Anthropic
// provider adapter, and prints the resulting tool-calls to stderr as JSON
// (stdout is reserved for ACP frames — transport discipline, PROJECT.md).
//
// This is a T1 placeholder main; the cobra root command lands in T6.
package main

func main() {}
