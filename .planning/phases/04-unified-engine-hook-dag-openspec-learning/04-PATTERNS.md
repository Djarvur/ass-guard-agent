# Phase 4 — Pattern Map

> Net-new Phase-4 files mapped to their closest existing in-repo analog. The executor replicates the analog's structure/conventions; deviations are noted. Produced inline (pattern-mapper is enabled by default but no Agent dispatch in this runtime).

## Convention anchors (read FIRST when writing any Phase-4 file)

| Convention | Source file | Apply to |
|---|---|---|
| Package doc style (comment block: what the package is + which decisions/REQs it implements) | `internal/scheduler/doc.go`, `internal/toolcat/types.go` (header) | every new `doc.go` |
| Declarative YAML config — yaml.v3 directly (NOT viper) + manual deep-merge layering + collect-all `Validate` + `*ConfigError` | `internal/scheduler/load.go` + `internal/scheduler/config.go` | hook config (04-03), learned store (04-06) |
| Single-writer mutex + atomic-ish write + Redactor-injected | `internal/session/manager.go` | learning store (04-06) |
| Typed-channel event bus + per-kind `Buf*` + `Kind() string` | `internal/event/events.go` | EngineDecision + HookProgress events (04-01) |
| defer-recover at the goroutine/turn boundary ⇒ investigate-and-fix-ready error line + named-return preservation | `internal/session/session.go:74-80` | engine Observe wrapper (04-01), hook executor (04-03) |
| ToolExecutor interface + RestrictedExecutor wrapper + compile-time `var _ = (*X)(nil)` | `internal/toolcat/restricted.go` | RealExecutor (04-04), RealExecutor satisfies ToolExecutor |
| cobra subcommand registration on the root + transport discipline (stdout = data, stderr = diagnostics) | `cmd/ass-guard/` (existing scheduler diagnostic command) | learning CLI (04-06) |

## Net-new files → analogs

| Phase-4 file (net-new) | Closest in-repo analog | Pattern to replicate | Deviation / note |
|---|---|---|---|
| `internal/engine/doc.go` | `internal/scheduler/doc.go` | package doc block | — |
| `internal/engine/types.go` | `internal/toolcat/types.go` (Mutability enum + String) | typed enum + String() + structs | Action enum mirrors Mutability |
| `internal/engine/decide.go` | `internal/scheduler/resolver.go` (pure Resolve, table-tested, NO sync/IO) | pure function, `testing/quick` property, `grep -cE "os\\.\|time\\.\|net\\." == 0` | — |
| `internal/engine/observe.go` | `internal/session/session.go` (Prompt's defer-recover + named returns) | defer-recover preserving original (stop,err); the wrapper is the engine's graceful-degradation guarantor | wraps a TurnRunner (new interface), not a Session directly |
| `internal/hookdag/config.go` | `internal/scheduler/load.go` + `config.go` | yaml.v3 layered load + Validate + ConfigError | hooks YAML, not scheduling YAML |
| `internal/hookdag/executor.go` | `internal/session/session.go` (the turn loop + defer-recover) | sequential walk + per-step dispatch + on-failure; ≤300 LOC core | the only mutable state is the provenance in-flight map |
| `internal/hookdag/step.go` | `internal/acp/server.go` (dispatch-by-method pattern) | dispatch by StepKind | 4 step kinds, each a func |
| `internal/openspec/config.go` | `internal/scheduler/load.go` (collect-all Validate) BUT TOML | Validate + ConfigError shape | BurntSushi/toml (NOT yaml) — isolated dep |
| `internal/openspec/adapter.go` | `internal/session/subagent.go` (exec/dispatch + ctx cancel) | exec.CommandContext + stdout/stderr capture + ExitError | shells out to `openspec` binary |
| `internal/openspec/patterntable.go` | `internal/toolcat/adapter.go` (Adapter.ResolveCall bridging) | bridge type satisfying an engine interface | `var _ engine.PatternTable` compile check |
| `internal/toolexec/batch.go` | `internal/provider/semaphore.go` (bounded concurrency) | bounded parallel + arrival-order collection | hand-rolled semaphore (avoid x/sync dep) |
| `internal/toolexec/backend.go` | `internal/provider/openai.go` / `anthropic.go` (adapter over a backend) | interface + a default impl + config-selected factory | net/http stdlib; firecrawl is a stub |
| `internal/toolexec/real.go` | `internal/toolcat/restricted.go` (ToolExecutor impl) | `var _ toolcat.ToolExecutor` compile check | catalog lookup + Backend delegation |
| `internal/learning/store.go` | `internal/session/manager.go` (single-writer mutex + atomic write) | mutex-guarded mutations + copy-on-read | atomic save = temp + os.Rename |
| `internal/learning/propose.go` | `internal/scheduler/resolver.go` (pure function) | pure, table-tested | sequence-window detection |
| `cmd/ass-guard/learning_cmd.go` | `cmd/ass-guard/` (existing cobra subcommand) | rootCmd.AddCommand + stdout/stderr discipline | — |

## Edits to existing files (additive, minimal)

| Existing file | Edit | Why minimal |
|---|---|---|
| `internal/event/events.go` | ADD EngineDecision + HookProgress kinds + Buf consts | additive — existing kinds untouched |
| `internal/session/manager.go` | ADD AppendEngineDecision | one method; TypeEngineDecision already reserved (transcript.go:26) |
| `internal/session/session.go` | ADD SetToolExecutor; replace inline tool loop (125-146) with DispatchBatch; preserve subagent dispatch + MaybeAppendBoundary | the executeStub seam already calls toolExec.Execute; this routes through the batch dispatcher |
| `internal/toolcat/catalog.go` | ADD Register(Tool) | one method |
| `cmd/ass-guard/acp_serve.go` | inject realExecutor (SetToolExecutor); wrap sess.Prompt (171) with engine.Observe; load openspec+hooks+learned at startup; disabled-engine fallback | additive — a nil engine ⇒ original path (backward-compatible) |
| `go.mod` | ADD github.com/BurntSushi/toml | one direct require, isolated to internal/openspec |

## C1-C5 style conventions (carry from Phase 3)

- **C1 — Config-not-code:** hooks, patterns, handoff tools, learned settings, backends — all declarative config, never Go code. (D-06, D-14, D-16, D-22)
- **C2 — Layered config:** embedded default → operator overlay (yaml.v3 manual deep-merge; mirrors scheduler/load.go).
- **C3 — Single-writer / mutex-guarded:** transcript Manager, learning Store, hookdag in-flight map — mutations serialize; reads copy.
- **C4 — defer-recover at boundaries:** every goroutine/turn/wrapper entry defers a recover → investigate-and-fix-ready error line; named returns preserved.
- **C5 — Transport discipline:** stdout = data/protocol (ACP, CLI tables, tool results); stderr = diagnostics/logs. (PROJECT.md)

---

*Phase: 4 · Pattern map produced inline from direct code reading.*
