// Package toolexec implements real tool execution (Phase-4 TOOL-04/05).
//
//   - DispatchBatch partitions a turn's tool calls by EffectiveMutability
//     (D-21): read-only tools (Glob/Grep/Read, identified by Phase-2 D-19's
//     mutability field) run CONCURRENTLY within a bounded errgroup-equivalent;
//     mutating tools (Bash/Write/Edit) serialize strictly relative to each
//     other. Results are returned in ARRIVAL ORDER (the transcript is
//     deterministic regardless of completion order).
//   - Backend is the swappable WebSearch/WebFetch implementation (D-22): a
//     configurable interface whose concrete impl is selected at startup from
//     config, so swapping backends = config + inject, never a code change.
//   - RealExecutor implements toolcat.ToolExecutor over the catalog:
//     WebSearch/WebFetch delegate to the configured Backend; every other tool
//     calls its catalog Tool.Execute; unknown tools return a structured error.
//
// The Session turn loop (Phase-2 D-18) calls DispatchBatch once per turn-step
// instead of the inline per-call loop, and a nil tool executor preserves the
// Phase-2 stub behavior (backward-compatible).
package toolexec
