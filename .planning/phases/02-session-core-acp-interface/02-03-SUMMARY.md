---
phase: 02-session-core-acp-interface
plan: 03
status: complete
requirements: [SESS-02, SESS-03, PARA-01]
autonomous: true
---

# Plan 02-03 — Mutability declaration interface + runtime tool restriction

## Outcome

Added the mutability-declaration interface (D-19) and the runtime tool-restriction
wrapper (D-10) to the built-in tool catalog. This is the forward-design contract
Phase 4's OpenSpec adapter plugs into (register commands with their own Mutability,
no engine change) and the runtime enforcement layer subagents use.

## What was built

- `internal/toolcat/mutability.go` — `EffectiveMutability(tool, adapterClass)`
  applies the SESS-03 "more-mutating wins" formula; `IsBoundary(toolName, catalog,
  configAdded)` applies the SESS-02 structural floor (config can ADD boundaries,
  cannot downgrade a catalog-mutating tool).
- `internal/toolcat/restricted.go` — `ToolExecutor` interface +
  `RestrictedExecutor` wrapper enforcing an allow-list at the execution boundary
  (D-10 — the model sees the full catalog; the executor enforces the subset).
- Tests: `mutability_test.go` (formula table + structural-floor cases + built-in
  defaults) and `restricted_test.go` (delegate / block / empty-set / error
  propagation / no-catalog-coupling).

## Adaptation vs. plan (intended → actual interface)

The plan assumed a string-based `Mutability = "mutating" | "read-only"` type to be
ADDED to `Tool`. The ACTUAL Phase-1 interface already ships a typed `Mutability int`
enum (`MutabilityReadOnly` / `MutabilityMutating`) with `String()`, `UnmarshalJSON()`,
and `IsMutating()`, plus the `Mutability` field already on `Tool` and already seeded
in the embedded `coretools.json` (Bash/Write/Edit = mutating, the other 16 read-only).
The implementation adapts to the real enum. The built-in defaults were left as
faithfully captured (Bash/Write/Edit mutating); the plan's "TodoWrite = mutating"
note was not applied because (a) Test 5 only requires Bash/Write/Edit mutating and
Read/Glob/Grep read-only, (b) `coretools.json` is the faithful captured catalog and
TodoWrite modifies agent state (not workspace files), so read-only is defensible.

## Self-Check

- [x] `go test ./internal/toolcat/ -race` passes (2.1s)
- [x] `go build ./...` + `go vet ./internal/toolcat/` clean
- [x] EffectiveMutability returns Mutating if either input is Mutating (SESS-03)
- [x] IsBoundary respects the structural floor (config cannot downgrade)
- [x] RestrictedExecutor delegates allowed / blocks disallowed without calling inner
- [x] RestrictedExecutor carries no Catalog (runtime-only, D-10)
- [x] TDD: RED commit f368579 → GREEN commit f38b552

## Key files

- created: `internal/toolcat/mutability.go`
- created: `internal/toolcat/mutability_test.go`
- created: `internal/toolcat/restricted.go`
- created: `internal/toolcat/restricted_test.go`
