# Phase 4: Unified Engine + Hook-DAG + OpenSpec + Learning — Research

**Researched:** 2026-08-09
**Status:** Complete
**Mode:** Inline research (auto mode; no Agent dispatch in this runtime). Grounded in direct reading of the executed Phase 1-3 Go code on disk + the locked 04-CONTEXT.md decisions.

## 0. Executive summary

Phase 4 is the project's reason to exist: an unmodified OpenSpec workflow runs end-to-end with zero manual "continue" taps, the "forgotten routine" runs after each stage, and the agent asks once + remembers. The infrastructure is real on disk: Session Core turn loop (`internal/session`), typed event bus (`internal/event`), provider + scheduler (`internal/provider`, `internal/scheduler`), tool catalog with mutability (`internal/toolcat`), ACP server (`internal/acp`). Phase 4 adds five new packages — `internal/engine`, `internal/hookdag`, `internal/openspec`, `internal/learning`, `internal/toolexec` — and threads them into one seam: `cmd/ass-guard/acp_serve.go` where `sess.Prompt(ctx, blocks)` returns (line 171). The engine wraps that call as a **post-turn observer** (D-01): it inspects the turn output, decides (continue / hook / ask / wait / nothing), and either injects a new prompt or does nothing. Structural safety (D-03): unmatched output triggers nothing. Graceful degradation (D-04): an engine panic never prevents the turn from completing.

The single hardest finding: the engine is NOT inside the turn loop. `session.Session.Prompt` (internal/session/session.go:70) runs the full turn (≤16 tool-call iterations) and returns `end_turn`. The engine runs AFTER it returns, in the ACP serve layer. This makes D-01/D-04 structural rather than conventional — the engine literally cannot block a turn because the turn is already over when the engine runs.

## 1. Real interfaces on disk (load-bearing — Phase 4 builds on these)

### 1.1 The turn loop the engine observes — `internal/session/session.go`

`Session.Prompt(ctx, userPrompt []ContentBlock) (stop string, err error)`:
- Generates a monotonic `turnID` via `nextTurnID()` → `"<sessionID>-turn-NNN"`.
- Appends a user message, then loops ≤16 iterations: project lean window → `Provider.Stream` (chunks emitted to the bus as `AgentMessageChunk`/`ToolCall`/`UsageUpdate`) → if `resp.ToolCalls` non-empty, execute each via `executeStub` + `MaybeAppendBoundary`, loop; else append assistant message + return `mapStopReason(resp.FinishReason)` (`"end_turn"` for stop/end_turn).
- Recover at the turn boundary: a panic becomes an investigate-and-fix-ready error line + error return (never a crash).
- `executeStub(ctx, tc)` (line 159): **if `s.toolExec != nil`, calls `s.toolExec.Execute(ctx, tc.Name, tc.Input)`; else returns the canned `stubToolResult`.** This is the TOOL-04/05 seam. Today `toolExec` is nil in production (`acp_serve.go:198` struct literal omits it) → every tool returns the stub.
- The tool-call loop (lines 125-146) executes calls **sequentially** in arrival order. Phase 4 must change this to: batch read-only calls concurrently, serialize mutating calls (D-21).

`Session` fields (public, set via struct literal in `acp_serve.go:198`): `Manager`, `Projector`, `Provider`, `Bus`, `Semaphore`, `Profile`, `WorkDir`, `SessionID`, `Catalog`, `ConfigAdded`. Private: `toolExec toolcat.ToolExecutor`, `subagentRunner`, `turnCounter`. **There is no `NewSession` constructor and no `SetToolExecutor`.** Phase 4 adds one constructor option (see §3.2).

### 1.2 The transcript = audit log — `internal/session/manager.go` + `transcript.go`

`Manager` is the sole mutex-guarded writer. Every `Append*` marshals a `Line`, redacts via the injected `Redactor`, writes under `mu`. Read APIs Phase 4 uses:
- `ReadAll() ([]Line, error)` — every line in append order.
- `ReadSince(turnID string) ([]Line, error)` — lines after the last line carrying `turnID` (the engine uses this to read one turn's output).
- `ReadLastBoundary() (*Line, error)`.

**`transcript.go` ALREADY reserves `TypeEngineDecision = "engine_decision"` (line 26).** Phase 4 emits it (Manager gains `AppendEngineDecision`). `Line` is a flat struct; every type uses its field subset. Fields available for the engine line: `TurnID`, `Text` (the decision + reason), `Name` (action), `Input` (the matched pattern/handoff), `Component` ("engine"), plus existing fields.

### 1.3 The event bus the engine emits to — `internal/event/events.go` + `bus.go`

Typed-channel bus; per-kind buffer sizes; `Publish(e Event)` blocks (backpressure) when the buffer is full. Existing kinds: `RequestShaped`, `AgentMessageChunk`, `ToolCall`, `ToolCallUpdate`, `UsageUpdate`, `SubagentResult`, `Boundary`. Each event has a `Kind() string` + a `TurnID` field.

**Phase 4 adds two kinds** (each with a `TurnID` + a `Kind()` method, a `Buf*` constant, and a bus subscription API): `EngineDecision` (the engine decided X with provenance) and `HookProgress` (a hook-DAG step started/finished/failed). The ACP adapter forwards `EngineDecision`/`HookProgress` to the client as `session/update` notifications (the "ask" action surfaces here). The existing `adapter.AgentMessageChunk` path in `acp_serve.go:160` is the forwarding template.

### 1.4 The mutability field — one source, two Phase-4 consumers — `internal/toolcat/mutability.go` + `types.go`

- `Mutability` enum: `MutabilityReadOnly` (iota 0), `MutabilityMutating` (iota 1). `String()`, `UnmarshalJSON("mutating"/"read-only")`.
- `Tool` struct: `Name`, `Description`, `InputSchema`, `Mutability`, `Execute Stub` (`json:"-"`). `IsMutating()`.
- `EffectiveMutability(tool Tool, adapterClass Mutability) Mutability` — "more-mutating wins". This is the **single source of truth the OpenSpec adapter plugs into (D-15)**: an OpenSpec command passes its per-command mutability as `adapterClass`, no engine change.
- `IsBoundary(toolName, catalog, configAdded) bool` — the SESS-02 structural floor. `session.MaybeAppendBoundary` calls this; the engine's `fresh-context` action reuses the same boundary mechanism (D-08).
- `Catalog`: `NewCatalog()` (embeds `coretools.json`), `Get`, `Names`, `Decls`. **No registration API for external tools yet** — Phase 4 adds `Register(Tool)` (or a merge) so OpenSpec commands appear in the catalog with their mutability.

### 1.5 The tool-execution seam — `internal/toolcat/restricted.go`

```go
type ToolExecutor interface {
    Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error)
}
```
`RestrictedExecutor` wraps a `ToolExecutor` with an allow-list (subagent subset, D-10). The Session holds a `toolExec ToolExecutor`; today nil → stub. **Phase 4's `internal/toolexec` package provides the real `ToolExecutor`** (concurrent read-only dispatch + serialized mutating + swappable backends) and a batch helper. The Session tool loop calls the batch helper instead of the inline sequential loop.

### 1.6 The provider + scheduler — the send-prompt path — `internal/provider/provider.go` + `internal/scheduler/dispatch.go`

- `Provider` interface: `Send`, `Stream(ctx, prof, messages) (<-chan StreamChunk, error)`, `ToolResultMessage`. `StreamChunk.Type` ∈ {`text`,`tool_use`,`usage`,`done`}. `ToolCall{Name, Input}`. `Response{ToolCalls, FinishReason, Raw}`.
- `Scheduler.Dispatch(ctx, tier, project, capReq, prof, messages) (Response, error)` — resolve → capability → breaker → cost → semaphore → `provider.Send` → fallback walk. Uses `Send` (not Stream); dispatch.go:152 explicitly notes "full token-cost coupling lands when Dispatch drives Stream (Phase 4)".
- `scheduler.WithTurnID(ctx, turnID)` propagates the turn id through context for event tagging. **The hook-DAG's `send-prompt` step (HOOK-05) IS a turn** — it goes through the Session Core (`session.Prompt`), which already uses the provider/scheduler. The hook-DAG does NOT call the provider directly; it orchestrates turns via the Session Core (D-12).

### 1.7 The ACP wiring point — `cmd/ass-guard/acp_serve.go` + `internal/acp/handlers.go`

- `sessionTurnRunner.sessionFor(sessionID)` (acp_serve.go:178) builds `&session.Session{...}` (line 198). **Phase 4 edits here**: inject the real `toolExec`, inject the engine.
- `sessionTurnRunner.Run` calls `sess.Prompt(ctx, blocks)` at line 171. **The engine wraps HERE**: after Prompt returns `end_turn`, the engine inspects + decides + (maybe) injects. The injection is a new `sess.Prompt` call with the continue prompt — a real turn through the Session Core (D-12).
- `handleSessionCancel` (handlers.go:130) → `st.cancelTurn()` → cancels the turn ctx. **The engine's queued injections must drain on cancel (ENG-03)** — the engine holds a cancel-aware queue and `cancelTurn` signals it.

## 2. Design reconciliations (decisions made during research — all reversible)

### 2.1 Engine placement: post-turn observer in the ACP serve layer (D-01/D-04)
The engine is a wrapper around `sessionTurnRunner.Run`, NOT a modification of `session.Session.Prompt`. `Session.Prompt` stays the critical path (unchanged); the engine runs after it returns. An engine panic is recovered in the wrapper and the original `stop`/`err` are returned unchanged → graceful degradation is structural. The engine reads the just-finished turn via `Manager.ReadSince(prevTurnID)` (or the last assistant_message's TurnID).

### 2.2 Provenance turn-id without changing `Prompt`'s signature (D-05)
`Prompt` returns `(stop string, err error)` — no turnID. Changing it would break callers. Instead the engine derives the turn id from the transcript: the last `assistant_message` line's `TurnID` is the just-completed turn; the engine tags every decision + hook execution with it. `Manager.ReadAll()` + a reverse scan finds it. (A `Session.LastTurnID()` accessor is a clean optional addition if the reverse scan proves brittle — noted in 04-01.)

### 2.3 Config formats: yaml.v3 for hooks (D-06) + TOML for the openspec overlay (D-14)
- **Hooks**: `gopkg.in/yaml.v3` directly, mirroring `internal/scheduler/load.go` (which deviated from its own plan's "viper" wording with an investigate-and-fix-ready note — viper's dotted-key flattening conflicts with dotted model slugs; viper is NOT in go.mod). Hook config follows the same layered `yaml.Unmarshal` + manual deep-merge pattern.
- **OpenSpec overlay**: D-14 locks `.claude/sdd/openspec.toml` with `[[patterns]]` / `[[handoff_tools]]` (TOML array-of-tables syntax), mirroring the real OpenSpec ecosystem's TOML idiom. This requires a TOML parser not yet in go.mod. **Decision: honor D-14 — add `github.com/BurntSushi/toml`** (most battle-tested Go TOML lib; reversible). The loader is isolated in `internal/openspec` so the dependency stays off the hot path. If the operator prefers a single-parser stack at execution time, the overlay can be re-emitted as `openspec.yaml` with no semantic change — flagged as a reversible reconciliation.

### 2.4 Tool concurrency location: a batch helper, the loop calls it (D-21)
Keep `ToolExecutor` single-call (`Execute`). Add `internal/toolexec.DispatchBatch(ctx, exec, catalog, calls []provider.ToolCall) ([]ToolResult, error)` that partitions calls by `EffectiveMutability`: read-only batch runs concurrently (`errgroup`-style, bounded by a semaphore), mutating calls run strictly sequentially. The Session tool loop (session.go:125-146) replaces its inline `for _, tc := range resp.ToolCalls` loop with one call to `DispatchBatch`, then appends results + boundaries. This keeps the concurrency logic testable in isolation and the Session change minimal. This matches the Claudecourse `isConcurrencySafe()` → parallel-batch pattern (§4).

### 2.5 Swappable backends (D-22 / TOOL-05)
`WebSearch`/`WebFetch` tools get a `Backend` interface in `internal/toolexec`; the concrete impl (default HTTP / firecrawl / ddg / brave) is selected from config at startup and injected. The catalog `Tool.Execute` for these tools delegates to the configured backend. No code change needed to swap — config-only.

### 2.6 The four engine actions (D-02 + D-08 step types)
- `continue` — inject the next-stage prompt (a real `Session.Prompt` turn). Triggered by a matched `[[patterns]]` text-regex OR a `[[handoff_tools]]` tool-call (dual-signal).
- `hook` — launch a hook-DAG from the seeded/configured set (post-implement / post-phase). The hook's own steps (`run-command` / `send-prompt` / `fresh-context` / `wait`) run via `internal/hookdag`.
- `ask` — surface a `session/update` (via `EngineDecision` event → ACP adapter) and wait for the user's reply. Used by learning mode (LRN-01) and hook `on-failure: ask`.
- `wait` — delay (a `wait` step or a timed re-check).
- `nothing` (implicit) — unmatched output. The structural safety property (D-03).

## 3. Claudecourse pattern study (study-only — no import, no dependency)

Per the user-provided reference (https://github.com/justxor/Claudecourse) and the 04-CONTEXT §specifics summary. Patterns confirmed + how they map:

| Claudecourse pattern | ass-guard mapping |
|---|---|
| Hook lifecycle points (PreToolUse/PostToolUse/Stop) | ass-guard's engine observes at the **Stop** equivalent — after `Prompt` returns `end_turn`. PreToolUse/PostToolUse are NOT in v1 (tools run ungated per the PROJECT.md safety model); the hook-DAG's `run-command` step is the post-stage hook surface. |
| Auto-continue loop (`while True: stop_reason check`) | ass-guard's engine is the loop body: after each turn, decide continue/hook/ask/wait/nothing. The "loop" is the engine re-entering `Session.Prompt` with the continue prompt. Bounded by the structural-safety rule (unmatched → nothing → loop exits). |
| `isConcurrencySafe()` → parallel batch vs sequential | Maps directly to D-21 / EffectiveMutability: read-only → concurrent batch; mutating → strictly sequential. Validated by a real production agent. |
| Exit-code contract (0=continue, non-zero=block+feedback) | The hook-DAG `run-command` step: exit 0 → next step; non-zero → apply `on-failure` (halt/continue/ask) with the captured stderr as feedback. |
| Plan mode (propose without executing) | Not in v1 scope (forward-compatible via `handleSessionSetMode` which is already a no-op placeholder). |
| Sub-agent worktree isolation | ass-guard's subagent dispatch (Phase 2 PARA-01..03, already shipped) uses goroutine-isolated turn loops with a `RestrictedExecutor`. The hook-DAG's `fresh-context` step reuses the boundary semantics, not worktrees. |

**Conclusion:** every Claudecourse pattern either validates an existing ass-guard decision (D-21 concurrency, exit-code contract) or is out-of-v1 (plan mode, worktrees). No new dependency, no import. ass-guard's OpenSpec hosting (subprocess adapter + pattern/handoff config) is novel — Claudecourse has no OpenSpec content.

## 4. Hook-DAG executor design (D-07 — hand-rolled ~300 LOC)

Per STACK §Focus 4: Temporal/Argo/Windmill are server products (violate single-static-binary). `go-task/task` + `prunner` are design references only. The executor is hand-rolled in `internal/hookdag`:

- **Config** (YAML, D-06): a hook entry = `name`, `trigger` (post-implement / post-phase / custom stage), `steps: []Step`, `on_failure` (halt/continue/ask), `allow_reentrant: bool`, `provenance_source`. A `Step` = `kind` (run-command/send-prompt/fresh-context/wait) + kind-specific fields + `on_failure` override.
- **Execution**: topological order is implicit (steps run in declared order; parallelism within a rank is a v1.1 concern — v1 runs steps sequentially unless a `parallel: [a,b]` group is declared). Each step is a typed interface (`run-command` → `exec.Command` with stdout/stderr/exit-code; `send-prompt` → `Session.Prompt` (a real turn, HOOK-05); `fresh-context` → boundary + new lean window; `wait` → `time.Sleep` or condition poll).
- **Provenance loop prevention (HOOK-04)**: each execution carries `(triggerStage, sourceTurnID)`. The executor refuses to launch a hook whose `(name, triggerStage)` is already in-flight unless `allow_reentrant: true`. Tracked in a mutex-guarded `map[string]struct{}` (the only mutable state).
- **on-failure (HOOK-03)**: a step failure applies the declared policy; `halt` stops the chain (default for mutating steps), `continue` logs + proceeds, `ask` emits an `EngineDecision`/`HookProgress` event for the user. Every step result is logged investigate-and-fix-ready.
- **Event emission**: `HookProgress{TurnID, HookName, StepIndex, StepKind, Status, Err}` at each step boundary → bus → ACP `session/update`.

Size budget: executor core ≤300 LOC (topological walk + step dispatch + provenance + on-failure); config types + per-step impls + tests are additional.

## 5. OpenSpec adapter design (D-13/D-14/D-15)

- **Subprocess (D-13, OPEN-01)**: `exec.Command("openspec", args...)` with stdout/stderr capture + a timeout derived from ctx. stdout is surfaced as the tool result (into the model's context). ass-guard does NOT reimplement OpenSpec logic. If `openspec` is not on PATH, the tool returns a structured error (the engine can then `ask` via learning mode).
- **Config (D-14, OPEN-02)**: `.claude/sdd/openspec.toml` — `[[patterns]]` (text-regex → action: continue/hook/ask/wait) + `[[handoff_tools]]` (tool-call name → action). Seeded from real OpenSpec handoff examples in 04-05. Loaded by an isolated BurntSushi/toml loader in `internal/openspec`.
- **Mutability registration (D-15, OPEN-03)**: each OpenSpec command declares `mutating` or `read-only` in the config (or a per-command map). The adapter registers each command as a `toolcat.Tool` with that `Mutability`. `EffectiveMutability` + `IsBoundary` consume it unchanged → a mutating OpenSpec command IS a context boundary (Phase 2 SESS-02), no boundary-engine change.
- **Pattern/handoff detection (D-02)**: the engine reads the loaded `[[patterns]]` (compiled regexes) + `[[handoff_tools]]` (name set) once at startup. After each turn, it scans the assistant text for a pattern match and the turn's tool calls for a handoff-tool name. Either signal → the configured action.

## 6. Learning mode design (D-16..D-20)

- **Store (D-16)**: `.ass-guard/learned.yaml` — versioned, git-trackable. Entry = `id`, `situation` (the question/trigger), `answer` (the action learned), `confidence` (int, starts 0), `expiry` (date, default +30d), `source_turns` ([]string), `status` (candidate/active/expired/conflict). Loaded at startup; written on new confirmations.
- **Ask-once-remember (D-17, LRN-01)**: when no pattern/handoff/hook matches (the "unfamiliar launch situation"), the engine emits an `EngineDecision{Action:"ask"}` with the question; the user's reply (a `session/prompt` from the client) is recorded as a candidate entry (confidence 0). On the NEXT occurrence, the candidate's answer is used + confidence++. At confidence ≥3 (D-19) the entry becomes `active`.
- **Propose hooks (D-18, LRN-02)**: the engine scans the recent work log (the transcript) for a repeated manual sequence (the same N steps ≥3 times) and proposes a hook entry; the user accepts/rejects via `ask`.
- **Conflict + expiry (D-19, LRN-03)**: at accept time, a new entry is checked against existing entries with the same `situation` — a different `answer` = conflict (surfaced). Expired entries (past `expiry`) are ignored (not deleted; the operator can renew or purge).
- **Revertible (D-20, LRN-04)**: plain yaml + git history. `ass-guard learning list` prints entries; `ass-guard learning revert <id>` removes one. Both are cobra subcommands on the existing `cmd/ass-guard` root.

## 7. Validation Architecture (Nyquist — feeds 04-VALIDATION.md)

The sampling-theory frame: Phase 4's correctness must be sampled at twice the rate of each behavior it depends on. The dependent behaviors + their Nyquist rates:

| Behavior (signal) | Frequency / rate | Sample rate (test cadence) | Test layer |
|---|---|---|---|
| Engine decision (per turn) | once per turn | every branch (continue/hook/ask/wait/nothing) + dual-signal variants | unit (table-driven) + integration (fake turn output) |
| Structural safety (unmatched → nothing) | invariant | every engine test asserts the no-match cell | unit + property (random unmatched strings) |
| Graceful degradation (panic → turn completes) | rare event | injected-panic test | unit (wrapper recovers + returns original stop) |
| Provenance loop prevention | per hook launch | every re-entrant/non-re-entrant pair | unit (hookdag) |
| Tool concurrency (read parallel / mutating serial) | per tool batch | deterministic ordering tests with a fake clock + recorded call order | unit (toolexec) |
| OpenSpec subprocess (stdout/exit-code) | per command | exit 0 + non-zero + not-on-PATH | unit (fake exec) + integration (real `openspec` if available, else skip) |
| Learning confidence threshold | per occurrence | 0→1→2→3 transitions + conflict + expiry | unit (learning store) |
| End-to-end zero-continue | per scenario | one full OpenSpec-style scenario with seeded patterns | integration (04-07) |

**Validation layers (the test pyramid):**
1. **Unit (fast, -race, no network):** engine `Decide` table; hookdag executor (provenance, on-failure, each step kind); learning store (confidence, conflict, expiry, revert); toolexec batch (concurrency ordering); openspec config load + pattern compile. Mock providers everywhere (autonomous:true — no ZAI_API_KEY needed).
2. **Integration (real Session + fake provider):** engine wraps `sessionTurnRunner.Run`; a fake provider returns canned handoff text → engine injects continue → fake provider returns `end_turn` → loop exits. Proves the zero-continue loop with zero network.
3. **End-to-end (04-07):** a scripted OpenSpec-style scenario (no real `openspec` binary required — a stub script on PATH asserts the subprocess contract; real `openspec` runs if available, gated by a build tag / env check).

**Coverage gates (per plan):** every plan ships its unit layer green under `-race`; the integration layer lands in 04-05/04-07. The phase verification (later) runs the full matrix.

**Golden-file + property tests:** the engine's structural-safety property ("for any unmatched input, the engine emits nothing") is a property test (random strings + random tool names not in the handoff set → action must be `nothing`). The config loaders get golden-file round-trip tests (load → re-marshal → equals).

## 8. Risks + mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Engine accidentally enters the critical path (blocks a turn) | high | The engine wraps `Run` AFTER `Prompt` returns; the wrapper's defer-recover guarantees the original `(stop, err)` are returned on any panic. Enforced by a test that injects a panicking engine and asserts the original stop is returned. |
| Infinite continue loop (engine re-fires on its own injection) | high | Provenance tag (D-05) + a per-session re-fire budget (max N continue-injections per user prompt, default 8) + structural safety (unmatched → nothing). The budget is the second barrier after provenance. |
| Tool concurrency race (two mutating tools overlap) | high | `DispatchBatch` serializes mutating calls under a per-turn mutex; read-only calls use an errgroup bounded by the provider semaphore. Tested under `-race` with a recording executor that sleeps to force overlap. |
| TOML dep drift (BurntSushi API change) | low | Loader isolated in `internal/openspec`; one struct + one `toml.Decode`. Pinned in go.sum. |
| OpenSpec CLI not on PATH (CI / first run) | medium | The adapter returns a structured "openspec not found" error; the engine routes it to learning `ask` (LRN-01) rather than crashing. Integration tests use a stub script; real-`openspec` tests skip if absent. |
| Hook-DAG `send-prompt` recursion depth | medium | Each `send-prompt` IS a turn (HOOK-05) and re-enters the engine wrapper; the re-fire budget + provenance cap the depth. A hook with `allow_reentrant:true` is logged loudly. |
| Learned-config corruption (concurrent read/write) | medium | The learning store is a single-writer (the engine) + copy-on-read; mutations serialize under a mutex (mirrors `Manager`). Git provides the ultimate revert. |

## 9. File layout (Phase 4 net-new + edits)

**Net-new packages:**
- `internal/engine/` — `engine.go` (Engine, Decide, wrapper), `config.go` (pattern/handoff tables, loaded), `events.go` (EngineDecision event), `engine_test.go`, `integration_test.go`.
- `internal/hookdag/` — `executor.go`, `config.go` (YAML types), `step.go` (the 4 step kinds), `events.go` (HookProgress), `seeded.yaml` (default hooks), `executor_test.go`.
- `internal/openspec/` — `adapter.go` (subprocess), `config.go` (TOML types + loader), `register.go` (tool registration with mutability), `seeded.toml`, `adapter_test.go`.
- `internal/learning/` — `store.go`, `types.go`, `propose.go` (hook proposals), `learned.yaml` (seed empty), `store_test.go`.
- `internal/toolexec/` — `batch.go` (DispatchBatch), `backend.go` (swappable WebSearch/WebFetch), `real.go` (catalog-backed ToolExecutor), `batch_test.go`.

**Edits to existing packages:**
- `internal/event/events.go` — add `EngineDecision`, `HookProgress` kinds + buffer consts.
- `internal/event/bus.go` — subscription API for the new kinds (if the bus needs per-kind subscription plumbing).
- `internal/session/manager.go` — add `AppendEngineDecision` (the `TypeEngineDecision` line is already reserved in transcript.go).
- `internal/session/session.go` — add `SetToolExecutor` (or a constructor); replace the inline sequential tool loop (lines 125-146) with `toolexec.DispatchBatch`.
- `internal/toolcat/catalog.go` — add `Register(Tool)` for OpenSpec command registration.
- `cmd/ass-guard/acp_serve.go` — inject real `toolExec` (line 198); wrap `sess.Prompt` (line 171) with the engine; wire the cancel-drain.
- `cmd/ass-guard/` — add `learning` cobra subcommands (`list`, `revert`).
- `go.mod` — add `github.com/BurntSushi/toml`.

---

*Phase: 4-Unified Engine + Hook-DAG + OpenSpec + Learning*
*Research grounded in: direct read of internal/{session,event,toolcat,provider,scheduler,acp}, cmd/ass-guard, go.mod, and the executed Phase 1-3 code.*
