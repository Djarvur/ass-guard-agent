## Deferred Items

- TestRescanConcurrency data race (internal/runtime/rescan_test.go) — spawnMCP reading vs installRegistry writing, pre-existing before any 22-04 commit
  status: open
  **Status:** open
  **What:** `go test -race ./internal/runtime/` fails on TestRescanConcurrency — a race between `Runner.spawnMCP` (runtime.go, sessionFor path) and `Runner.installRegistry` (commands.go:495, rescanAndSwap path). Verified PRE-EXISTING by running the test at HEAD (commit ff9f19c, which touches no runtime code) in a clean scratch worktree: same race. Out of 22-04's scope (no file 22-04 touches is in the race stack); blocking the plan's `./internal/runtime/` green gate.
  **Fix owner:** the runtime rescan/MCP concurrency owner (Phase 23 steering-ingress churn per STATE.md's cross-workstream note, or a dedicated fix plan).
