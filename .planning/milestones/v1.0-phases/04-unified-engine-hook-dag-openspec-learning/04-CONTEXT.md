# Phase 4: Unified Engine + Hook-DAG + OpenSpec + Learning - Context

**Gathered:** 2026-08-12
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 4 is **the project's reason to exist.** A developer runs an unmodified OpenSpec workflow end-to-end through ass-guard with zero manual "continue" taps, the forgotten routine runs automatically after each stage, and when the agent encounters an unfamiliar launch situation it asks once and remembers. This phase collapses autocontinue and hooks into one decision engine, hosts OpenSpec as the v1 toolkit, implements real tool execution (concurrent reads, swappable backends), and adds learning mode.

**In scope (19 REQ-IDs):**
- **Unified Engine (ENG-01..05):** after turn-complete, a single engine decides: continue the SDD scenario (dual-signal — text-pattern OR known handoff tool-call), trigger a hook-DAG, ask the user, or wait. Unmatched output triggers nothing (structural safety). If the engine fails, the turn still completes (graceful degradation — the engine is an observer, never in the critical path).
- **Hook-DAG (HOOK-01..05):** a configurable DAG executor runs arbitrary steps (run-command / send-prompt / fresh-context / wait) in any order. A seeded hook set ships out of the box. Every hook declares on-failure behavior. Provenance tagging prevents infinite loops.
- **Learning Mode (LRN-01..04):** when the engine doesn't know how to launch what should be launched, it asks the user and remembers. Proposes new hooks from the work log. Confidence threshold, expiry, conflict-checking, versioned + revertible.
- **OpenSpec Hosting (OPEN-01..03):** a shell toolkit adapter runs OpenSpec commands as subprocesses, captures stdout/stderr, surfaces stdout into context. A config declares patterns + handoff tools. Each command declares its mutability shape.
- **Tool Execution (TOOL-04/05):** read-only tools execute concurrently; mutating tools serialize. Complex tools use swappable backends.

**Out of scope:**
- Ecosystem compatibility (MCP, skills, plugins — Phase 5)
- Distribution + zero-config first run (Phase 6)
- Additional toolkit adapters (GSD, spec-kit, BMad — v2, KIT-01..03)

**Mode:** mvp (ROADMAP.md Phase 4). The largest phase (19 REQ-IDs). Built on Phase 1+2+3 infrastructure (provider adapters, Session Core, event bus, scheduler, mutability interface).

</domain>

<decisions>
## Implementation Decisions

### Unified engine decision logic (ENG-01..05)

- **D-01:** The engine is a **post-turn observer**: after the turn loop reaches `end_turn` and the response is fully streamed + transcripted, the engine inspects the turn output and decides what to do next. It NEVER sits inside the turn loop's critical path — if it panics/misconfigures, the turn has already completed normally (ENG-04). The decision is a pure function of (turn output, pattern table, hook config, learned config). *[Claude's discretion — user declined; directly follows from ENG-04 "the engine is an observer, never in a turn's critical path."]*
- **D-02:** The continue decision uses **dual-signal detection**: (1) TEXT-PATTERN — the assistant's output text matches a known SDD handoff pattern from the `[[patterns]]` table in `openspec.toml`; (2) HANDOFF TOOL-CALL — the model invoked a known handoff tool from the `[[handoff_tools]]` table. Either signal → engine launches the next stage. Neither signal → engine does nothing (ENG-03 structural safety). Both tables are declarative config. *[Claude's discretion — user declined; directly follows from ENG-01 + OPEN-02.]*
- **D-03:** Structural safety property (ENG-03): unmatched output triggers nothing. The only off-switch is manual cancellation (Phase 2 D-16), which cancels the in-flight turn AND drains queued injections. This is the project's safety model (PROJECT.md: "the pattern/hook table + manual cancellation is the only safety mechanism").
- **D-04:** Graceful degradation (ENG-04): if the engine fails (panic, misconfig), the turn still completes normally and the agent degrades to manual-continue. The engine is an observer — its failure never prevents the turn from finishing. This is the structural guarantee that makes the engine safe to add.
- **D-05:** Provenance tagging (ENG-02/HOOK-04): every engine action carries a provenance tag identifying the source turn + trigger signal. A hook cannot re-trigger the stage that triggered it without explicit opt-in. This structurally prevents infinite loops (HOOK-04).

### Hook-DAG structure & seeded set (HOOK-01..05)

- **D-06:** Hooks are declared in a **declarative YAML config** (matching Phase 3's `scheduling.yaml` pattern). Each hook entry declares: trigger (post-implement / post-phase / custom stage), steps (the DAG: run-command / send-prompt / fresh-context / wait in order), on-failure (halt / continue / ask), and provenance tag. The seeded set ships as a default config the operator can override. Config-not-code convention.
- **D-07:** The DAG executor is **hand-rolled in-process (~300 LOC)** per STACK Focus 4. Temporal/Argo/Windmill are too heavy (server products, violate single-binary). go-task/task + prunner are design references. Topological execution with parallelism within a dependency rank (test + lint run concurrently; review waits).
- **D-08:** Step types (HOOK-01): **run-command** (exec a shell command, capture stdout/stderr, exit-code contract: 0=continue, non-zero=per on-failure), **send-prompt** (inject a prompt into the model — IS a turn per HOOK-05), **fresh-context** (start a new context window via Phase 2 boundary semantics), **wait** (delay for N seconds or until a condition).
- **D-09:** Seeded hook set (HOOK-02): **post-implement** stage triggers (test + lint + review + memory) and **post-phase** stage triggers (improvement proposals). These ship out of the box for zero-config first run. The operator can override, add, or remove hooks via the YAML config.
- **D-10:** on-failure (HOOK-03): every hook declares `on-failure: halt | continue | ask`. **halt** stops the chain (default for mutating steps). **continue** logs the error and proceeds (for advisory steps like lint after test). **ask** surfaces the failure to the user via a session/update. Hook failures never silently block the workflow.
- **D-11:** Provenance prevents loops (HOOK-04): each hook execution carries the provenance tag of the triggering stage + turn. The executor refuses to launch a hook whose provenance tag matches an already-in-flight stage (structural cycle prevention). Explicit opt-in (`allow_reentrant: true` per hook) is required for the rare case where re-entrancy is intended.
- **D-12:** send-prompt IS a turn (HOOK-05): each `send-prompt` step in a DAG starts a real turn through the Session Core. `fresh-context` steps own their own context window via Phase 2's boundary semantics (D-01/D-04 lean seed). The hook-DAG orchestrates turns; it doesn't bypass the Session Core.

### OpenSpec adapter & pattern matching (OPEN-01..03)

- **D-13:** ass-guard hosts OpenSpec as an **external CLI subprocess** (OPEN-01): `exec.Command("openspec", args...)`, captures stdout/stderr, surfaces stdout into the model's context as a tool result. ass-guard does NOT reimplement OpenSpec logic — it shells out to the unmodified CLI. This matches the ROADMAP goal: "unmodified OpenSpec workflow." *[User-selected.]*
- **D-14:** A `.claude/sdd/openspec.toml` config declares the pattern + handoff tables (OPEN-02): `[[patterns]]` (text-regex → action: continue, hook, ask, wait) and `[[handoff_tools]]` (tool-call-name → action). Seeded from real OpenSpec handoff examples. This config drives the engine's dual-signal detection (D-02).
- **D-15:** Each OpenSpec command declares its shape via Phase 2's **D-19 mutability field** (OPEN-03): `mutating` or `read-only`. This classification is the single source of truth for context boundaries — a mutating OpenSpec command IS a boundary (Phase 2 SESS-02). ass-guard registers OpenSpec commands as tools in the catalog with their mutability; no boundary-engine change needed.

### Learning mode & memory (LRN-01..04)

- **D-16:** Learned settings live in a **versioned JSON/YAML file under `.ass-guard/`** (e.g. `.ass-guard/learned.yaml`). Each entry: the question/situation asked, the answer/action learned, confidence count, expiry/review date, source-turn references. The engine reads it at startup; writes on new confirmations. Git-trackable (if the user chooses) or gitignored (like transcripts). *[User-selected.]*
- **D-17:** Ask-once-and-remember (LRN-01): when the engine encounters a launch situation it doesn't know how to handle (no matching pattern, no known handoff, no applicable hook), it asks the user ("fresh context? wait? how long?") via a session/update, executes the answer, and records it as a candidate learned setting.
- **D-18:** Propose hooks from work log (LRN-02): the engine monitors the work log for repeated patterns (e.g. the user manually runs the same post-implement sequence three times) and proposes a new hook entry. The user accepts or rejects via session/update.
- **D-19:** Confidence threshold + expiry + conflict-check (LRN-03): a candidate learned setting requires **≥3 confirmations** to persist as active. Each entry carries an **expiry/review date** (default: 30 days). New entries are **conflict-checked** at accept time against existing entries (same situation → different answer = conflict, surfaced to the user).
- **D-20:** Versioned + revertible (LRN-04): the learned file is plain text (git-trackable). `ass-guard learning revert <entry-id>` removes a specific entry. `ass-guard learning list` shows all entries with confidence + expiry. Git history provides full revertibility.

### Tool execution (TOOL-04/05)

- **D-21:** Concurrent reads, serialized mutations (TOOL-04): read-only tools (Glob, Grep, Read — identified by Phase 2's D-19 mutability field) execute **concurrently** within a turn. Mutating tools (Bash, Write, Edit) **serialize** relative to each other. The turn loop (Phase 2 D-18) groups read-only calls into parallel batches; mutating calls go one at a time. This matches the Claudecourse pattern: `isConcurrencySafe()` flag → parallel batch; mutating → strictly sequential.
- **D-22:** Swappable backends (TOOL-05): complex tools (WebSearch, WebFetch) use a **configurable backend**, not a hardcoded one. The backend is swappable via config (e.g. WebSearch backend = ddg-search / brave / google; WebFetch backend = default HTTP / firecrawl). The backend is selected at startup from config; no code changes needed to swap.

### Claude's Discretion
D-01, D-02, D-03, D-04, D-05 are Claude's-discretion (user declined to answer follow-ups on engine design). Each directly follows from the ENG requirement it implements. D-07 follows from STACK Focus 4. D-08..D-12, D-21..D-22 follow directly from HOOK/TOOL requirements. All are reversible at planning.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Prior-phase outputs (the infrastructure Phase 4 builds on)
- `.planning/phases/02-session-core-acp-interface/02-CONTEXT.md` — D-04 (event bus typed channels — engine emits decisions), D-16 (session/cancel — engine drains queued injections), D-18 (turn cycle — engine observes post-turn), D-19 (mutability field — OpenSpec registers via this, tool concurrency), D-20 (transcript = audit log — engine decisions logged).
- `.planning/phases/03-model-scheduling/03-CONTEXT.md` — D-01 (declarative YAML pattern — hook config follows the same shape), the config-loading approach (yaml.v3, not viper — dotted model slugs).

### The 19 Phase-4 requirements
- `.planning/REQUIREMENTS.md` §"Unified engine (Phase 4)" (ENG-01..05), §"Hook-DAG (Phase 4)" (HOOK-01..05), §"Learning mode (Phase 4)" (LRN-01..04), §"OpenSpec hosting (Phase 4)" (OPEN-01..03), §"Parallelism" — TOOL-04/05 are Phase 4.

### Phase goal + success criteria
- `.planning/ROADMAP.md` §"Phase 4" — the 5 success criteria (zero-continue OpenSpec run; seeded hooks; on-failure + provenance; learning mode; concurrent tools + swappable backends).

### STACK references
- `.planning/research/STACK.md` §Focus 4 (Hook-DAG) — the hand-rolled in-process executor recommendation (~300 LOC); Temporal/Argo/Windmill rejected (server products); go-task/task + prunner as design references.
- `.planning/research/STACK.md` §"Stack Patterns by Variant" — the hook-DAG YAML semantics pattern.

### External reference (user-provided)
- `https://github.com/justxor/Claudecourse` — architectural patterns for the unified engine: hook lifecycle points (PreToolUse/PostToolUse/Stop), auto-continue loop (while True: stop_reason check), concurrency safety flag (isConcurrencySafe → parallel batch vs sequential), exit-code contracts (0=continue, non-zero=block+feedback), plan mode (propose without executing), sub-agent worktree isolation. Study-only reference; no OpenSpec content.

### Project-level principles (load-bearing)
- `.planning/PROJECT.md` — **Investigate-and-fix-ready logging** (engine decisions, hook executions, and learning-mode proposals must be logged for developer investigation), **Safety model** (no tool-execution confirmation tier; pattern/hook table + manual cancellation is the only safety mechanism), **v1 Cut-Line** (OpenSpec only in v1).

### Codebase (Phases 1-3 executed)
- `internal/session` — the Session Core (turn loop, transcript, boundaries) the engine observes
- `internal/event` — the typed-channel event bus the engine emits to
- `internal/toolcat` — the tool catalog with Mutability field (D-19) that drives tool concurrency + OpenSpec registration
- `internal/provider` — the provider interface + scheduler the engine's hook-DAG send-prompt steps invoke
- Phase 4 adds: `internal/engine` (the unified engine), `internal/hookdag` (the DAG executor), `internal/openspec` (the shell adapter), `internal/learning` (the learned-config store), and expands `internal/toolcat` (real tool execution + concurrent dispatch)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Session Core turn loop** (Phase 2 D-18): the engine observes after the turn loop reaches `end_turn`. The turn loop doesn't change; the engine wraps around it.
- **Event bus** (Phase 2 D-04): the engine emits `EngineDecision` events; the hook-DAG emits `HookProgress` events. Both flow through the existing typed channels to the ACP adapter (session/update) and the transcript writer.
- **Mutability field** (Phase 2 D-19): drives tool concurrency (D-21) AND OpenSpec command registration (D-15). One field, two Phase-4 consumers.
- **Boundary semantics** (Phase 2 D-01/D-08): `fresh-context` hook steps and learned-mode resets use the existing boundary mechanism.
- **`session/cancel`** (Phase 2 D-16): the engine uses this to drain queued injections when the user cancels (ENG-03).

### Established Patterns
- **Declarative YAML config** (Phase 3 D-01): hooks follow the same pattern — YAML config, layered, operator-editable. Use `gopkg.in/yaml.v3` (not viper — dotted model slugs).
- **Subprocess execution** (Phase 0 spikes/03-acp-handshake): the OpenSpec adapter's subprocess pattern follows the same exec.Command + stdout/stderr capture approach.
- **Config-not-code** (Claude-Code convention): hooks, patterns, handoff tools, learned settings — all declarative config, not Go code.

### Integration Points
- **Turn loop → Engine:** after `end_turn`, the Session Core hands the turn output to the engine. The engine decides (continue/hook/ask/wait) and either injects a new prompt (send-prompt → back to Session Core) or does nothing.
- **Engine → Hook-DAG:** when the engine decides "trigger a hook-DAG," it launches the DAG executor with the matched hook config + provenance tag.
- **OpenSpec CLI → Engine:** the OpenSpec adapter runs commands as subprocesses; the stdout output is surfaced into the model's context as a tool result. The engine watches for handoff patterns in subsequent turn outputs.
- **Engine → Learning store:** when the engine doesn't know how to handle a situation, it reads the learned config; if no match, it asks the user and writes a candidate entry.

</code_context>

<specifics>
## Specific Ideas

- The user referenced `https://github.com/justxor/Claudecourse` as a potentially useful resource. The repo is a Russian-language educational deep-dive into Claude Code's internal architecture. Key patterns confirmed: hook lifecycle points (PreToolUse/PostToolUse/Stop), auto-continue loop (stop_reason check), concurrency safety (isConcurrencySafe flag), exit-code contracts, plan mode, sub-agent worktree isolation. These align with and validate ass-guard's design choices. No OpenSpec content — ass-guard's OpenSpec hosting is novel.
- The Claudecourse concurrency-safety pattern (`isConcurrencySafe()` → parallel batch) maps directly to ass-guard's D-21 (Phase 2 mutability field drives concurrency). The pattern is validated by a real production agent.
- The exit-code contract (0=continue, non-zero=block+feedback) is a clean pattern for the hook-DAG's `run-command` step type. The hook receives the exit code and the captured stderr, and applies its `on-failure` policy.

</specifics>

<deferred>
## Deferred Ideas

None raised as new capabilities. The following are noted as out-of-scope:
- Additional toolkit adapters (GSD, spec-kit, BMad — v2, KIT-01..03). Phase 4 builds the adapter INTERFACE; additional implementations are deferred.
- Telegram peer interface (v2, TG-01..04).
- The hook-DAG's `send-prompt` steps currently go through the Session Core's turn loop. A future optimization might batch multiple send-prompts or chain them without full turn overhead — deferred until profiling shows a need.

</deferred>

---

*Phase: 4-Unified Engine + Hook-DAG + OpenSpec + Learning*
*Context gathered: 2026-08-12*
