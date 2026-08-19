# Tool Contract Inventory (EARLY-06)

**The uniform tool contract:** every tool in the catalog is held to bounded time,
capture-faithful error marking, transient-only retries, and declared
concurrency/destructiveness. This document is the evidence base: what already
existed, what the corpus shows, where every retry site is and what gates it,
and how the occ surface census cross-checks the contract inventory (input only).

Status legend: **already-pinned** (behavior existed, now test-pinned) ·
**closed-here** (gap closed in 14-06) · **routed** (decision deliberately
delegated, destination named).

---

## (a) Existing implementations census

| Surface | Bound / mechanism | Where (file:line at 14-06) | Precedence |
|---|---|---|---|
| `openspec:*` tools | per-command `timeout_secs` (default `DefaultTimeoutSecs` = 60s), non-interactive env, nil stdin | `internal/openspec/adapter.go:147-193` (`RunGuarded`), `internal/openspec/config.go:85` | INNER deadline — fires before the 14-06 backstop |
| `Bash` | model-provided ms timeout (schema-declared default 120000, clamped to max 600000) + process-group SIGKILL + straggler reap | `internal/coreexec/bash.go:50-53` (constants), `:72-83` (`resolveBashTimeout`), `:172` (`WithTimeout`), `:92-129` (group kill/reap) | INNER deadline — **authoritative** (see (e)) |
| `WebSearch` / `WebFetch` | 30s HTTP client timeout per hop | `internal/toolexec/backend.go:81-85`, `internal/toolexec/ddg.go:166` | INNER (per hop) |
| Provider SSE stream | 5-minute idle watchdog aborting a stalled body with a retryable typed error | `internal/provider/streaming.go:31-38,122-155` | orthogonal (stream liveness, not a tool bound) |
| Provider error classification | `ClassifyHTTP`: 408/425/429/5xx + net/ctx → `Transient`; 400/401/403/404/405/411/413/422 → `Structural`; unknown non-2xx → Transient (safe-side) | `internal/provider/errors.go:85-114`, status tables `:137-157` | the retry gate's input (see (c)) |
| Mutability → dispatch | catalog `mutability` drives read-only-pool vs mutating alone-in-slot (D-19/D-21) | `internal/toolexec/batch.go:258-271` (`isMutating` consumer), `internal/toolcat/mutability.go` | the structural serialization floor |
| **14-06 backstop (new)** | per-tool `timeout_ms` annotation, default `DefaultToolTimeoutMS` = 120000; EVERY dispatched call wrapped in its own ctx deadline; timeout → IsError + `{"error":"toolexec: tool <n> timed out after <ms>ms"}` | `internal/toolcat/types.go:69` (constant), `internal/toolexec/batch.go:170-226` (`timeoutFor` + `executeBounded`) | OUTER backstop — an inner deadline always fires first (each is ≤ the backstop) |

The backstop closes the "no deadline at all" class: a catalog tool with no
inner deadline of its own (Skill, Cron*, Todo*, SendMessage,
ReadSessionContext, MCP/plugin tools, dynamically-registered commands) can no
longer hold a turn indefinitely.

## (b) is_error corpus findings

**Method.** `internal/toolexec/iserror_corpus_test.go` mechanically scans the
zcode rollout corpus (`profile.ScanRolloutDir`, newest sessions) for
tool-result messages carrying `isError: true` (the runtime-normalized form of
the wire's `tool_result.is_error`), counting per tool and classifying error
FORM classes (first line, digit runs collapsed — payload-free shapes). The
committed inline fixture (`iserrorFixtureJSONL`) proves the parse logic in CI;
the live path skip-louds when no corpus is on disk.

**Provenance.** Live scan 2026-08-19, `~/.zcode/cli/rollout/`, three newest
sessions: `59ccd071…` (main), `subagent_agent_67bd22fb…`,
`subagent_agent_9aebe393…`. Counts are message observations across
full/tail request snapshots (a tool result appears in several snapshots), not
unique calls — the FORM classes are the finding.

| Tool | isError obs. | Error FORM class (shape) |
|---|---|---|
| Bash | 94 | `Exit code <N>` + combined output (every one of them) |
| Edit | 123 + 13 | `<tool_use_error>File has been modified since read, either by the user or by a linter. Read it again before editing.</tool_use_error>` · `<tool_use_error>File has not been read yet. Read it first be…</tool_use_error>` |
| Write | 29 | `File has been modified since read, either by the user or by …` (bare — no wrapper) |
| Read | 24 | `File does not exist. Note: your current working directory is <dir>.` |

Aggregate: 7,892 tool-result messages, 283 isError.

**Executor-by-executor comparison** (every row terminal):

| Executor | Corpus form | ass-guard emission | Verdict |
|---|---|---|---|
| Bash | `Exit code <N>\n<output>` + error | `internal/coreexec/bash.go:232-240` byte-pins the form + non-nil error → `IsError` (the 137/137 anchor from the 08-08 harvest) | **already-pinned** — `TestBash_ErrorForm`/`TestBash_FixtureConformance`; re-pinned by 14-06 Task 3 |
| Read (missing file) | `File does not exist. Note: your current working directory is <dir>.` | `internal/coreexec/files.go:94-101` renders the same text + non-nil error → `IsError` | **already-pinned** — `TestRead_MissingFileForm`; re-pinned by 14-06 Task 3 |
| Edit (not-found) | `<tool_use_error>String to replace not found in file.\nString: …` (08-08 harvest form; absent from the current live window) | `internal/coreexec/files.go:249-257` pins that form + `IsError` | **already-pinned** |
| Edit / Write (read-tracking) | modified-since-read / not-read-yet forms (165 obs. — the DOMINANT live error class) | NOT implemented — read-tracking deliberately deferred (`files.go:172-175, 224-225` note; the 08-08 fixture's `corpus_absent` disposition) | **routed** — implementing read-tracking is a product-behavior change on the locked no-confirmation-tier model (operator disposition; 12-05's re-record will carry the forms). Zero `closed-here` is_error rows: every implemented form matches the corpus. |
| DispatchBatch timeout | corpus-absent (no timeout tool_result observed in any rollout) | 14-06's structured `{"error":"toolexec: … timed out after <ms>ms"}` + `IsError` (the shipped corpus-absent convention, flagged here) | **closed-here** (Task 1) |

## (c) Retry-site census — every retry/fallback site found, and its error-kind gate

| # | Site | Where | Gate | Can a Structural error be retried here? |
|---|---|---|---|---|
| 1 | Scheduler fallback walk (THE retry site) | `internal/scheduler/dispatch.go:250-287` | typed `ProviderError.Kind`: `Structural` → immediate return (258-262, D-04 — never feeds the breaker); `Exhausted` → return; `Transient` → breaker record + walk to next candidate | **No** — pinned by `TestDispatchStructuralStopsWalk`; re-pinned by 14-06 `TestRetrySites_NeverRetryStructural` |
| 2 | SSE idle watchdog | `internal/provider/streaming.go:40-155` | not a retry — classifies a stalled stream as a retryable `KindTransient` typed error (a SIGNAL for the next dispatch, pinned `TestStream_IdleWatchdogAbortsStalledStream`) | n/a (signal producer) |
| 3 | Session stream consumer | `internal/session/session.go:620-627` | records the retryable error + ends the turn — **no auto-retry** at this layer | n/a |
| 4 | HTTP/SDK clients | Anthropic path = raw `net/http` (`streaming.go:157+`); OpenAI path = `go-openai` single-shot (no built-in retry) | none found — **no hidden retry loops** exist in the tree (grep: zero `MaxRetries`/`WithMaxRetries`) | n/a |

**Verdict:** exactly one retry path exists (the scheduler walk) and it is
Kind-gated; no discovered path retries a Structural error. The classification
table itself is pinned at the seam by 14-06 `TestRetryOnlyTransient_ClassificationTable`
(literal statuses).

## (d) occ surface census cross-check (INPUT ONLY)

Per the 2026-08-19 SEED-004 disposition: the census is a **free verification
input** — zero new requirement, zero scope growth. Committed fixture:
`internal/toolexec/testdata/occ-census.json` — repo `ruvnet/open-claude-code`,
pinned commit `5d007f09c43be0e7c0849f039df447d6fbbbe04d` (2026-08-19, tracks
Claude Code 2.1.235), provenance `{repo, commit, extracted_at}` +
re-clone command. The clone is gitignored (`tools/occ-census-clone/`),
read/diff only, never executed, never vendored (T-14-SC). ECOSYSTEM-AUDIT §1.1
caveat applies throughout: name presence ≠ behavior depth. **No census-derived
name, env var, or mode enters the catalog, profile, or config as
capture-grounded content** — the zcode corpus remains the authority.

Three-way accounting of the 25 census tool names:

| Bucket | Count | Names |
|---|---|---|
| **in shipped catalog** (the 19 coretools) | 13 | Agent, Bash, CronCreate, CronDelete, CronList, Edit, Read, SendMessage, Skill, TodoWrite, WebFetch, WebSearch, Write |
| **MCP-shaped** | 0 | — the `mcp__<server>__<tool>` namespace is dynamic (58 tools at the current zcode pin), never a census name |
| **documented-absent** (rationale) | 12 | AskUser (occ name variant — the captured zcode catalog ships `AskUserQuestion`) · EnterWorktree / ExitWorktree (not in the captured catalog) · Glob / Grep (not in the captured 79-tool catalog — the 08-08 finding) · LS, LSP (occ explicit stub), MultiEdit, NotebookEdit (not in the captured catalog) · ReadMcpResource (MCP-adjacent builtin; ass-guard surfaces MCP via the dynamic `mcp__` namespace) · RemoteTrigger, ToolSearch (occ-specific) |

13 + 0 + 12 = 25 — every census tool name accounted for. The accounting is
test-pinned (`TestOccCensusCrossCheck`); determinism at the pin is pinned by
`TestOccCensusDeterministic` (re-extraction at the same commit reproduces the
fixture; consecutive extractions identical).

INPUT-ONLY context (recorded, not consumed): 63 `CLAUDE_CODE_*` env-var names
in occ's `env.mjs` table (51 carrying defaults; the audit's "~100" figure is
the full 104-entry table incl. non-prefixed entries), 6 permission-mode names
(default/auto/plan/acceptEdits/bypassPermissions/dontAsk — ass-guard's
no-confirmation-tier safety model is unchanged), 4 MCP transports
(stdio/sse/websocket/streamable-http — ass-guard hosts stdio only in v1).

## (e) Flags consumption map + routed decisions

**Where the annotations live.** `internal/toolcat/coretools.json` — the
ass-guard-side annotation surface (the mutability precedent): `timeout_ms`
(declared on all 19), `concurrency_safe` (declared on the 16 non-mutating),
`destructive` (true only on Bash), parsed into `toolcat.Tool` with accessors
`EffectiveTimeoutMS()` / `IsConcurrencySafe()` / `IsDestructive()`
(`internal/toolcat/types.go`). **The captured catalog schema
(`profiles/zcode/tools.json`) is never rewritten** — annotations ride only in
ass-guard-side metadata.

| Flag | Declared values | Consumed by | Status |
|---|---|---|---|
| `timeout_ms` | Bash 600000 · Agent 600000 · Read/Write/Edit/WebSearch/WebFetch/ReadSessionContext 60000 · Skill/AskUserQuestion/EnterPlanMode/ExitPlanMode 120000 · Cron*/Todo*/TaskStop/SendMessage 30000 | `DispatchBatch` per-call deadline wrap (`batch.go:170-226`) — pooled AND serialized slots identically | **closed-here** (Task 1) |
| `concurrency_safe` | `true`: Read, Skill, WebSearch, WebFetch, TodoRead, CronList, EnterPlanMode, ReadSessionContext · `false` (serialized session-state writers / turn-level state transitions): TodoWrite, CronCreate, CronDelete, TaskStop, SendMessage, ExitPlanMode, AskUserQuestion, Agent · undeclared on the mutating trio (alone-in-slot regardless) | `DispatchBatch` pool membership (declared value overrides the mutability default; a `false` REMOVES a read-only tool from the parallel pool — it can never add a mutating one, T-14-19) | **closed-here** (Task 3) |
| `destructive` | true only on Bash (irreversible-by-nature) | engine/audit OBSERVATION (exposed for the audit surface) | **routed** — see below |

**Why Bash's backstop is 600000, not 120000.** The plan's example said "the
120s backstop", but the model-ms inner deadline is authoritative up to the
schema-declared max of 600000 (`resolveBashTimeout` clamps there). A 120000
backstop would pre-empt a legitimate model-requested 600s command — violating
the plan's own precedence truth ("the model-ms inner timeout stays
authoritative"). Annotated 600000: the inner deadline (always ≤ 600000) fires
first in every case; the backstop only catches a hypothetical no-inner-deadline
path that cannot occur. `DefaultToolTimeoutMS` = 120000 matches both the Bash
schema's declared default and the occ census's `CLAUDE_CODE_TOOL_TIMEOUT`
default — two independent sources agreeing on the ecosystem's tool bound.

**The routed engine-gating decision.** `IsDestructive` does NOT gate engine
chaining. Gating chaining on destructiveness would revise the locked
unmatched⇒nothing safety model (the pattern/hook table + manual cancellation
is the only safety mechanism, PROJECT.md Constraints) — an operator decision,
deliberately not built here. Backstop: any turn that mis-chains a destructive
call remains recoverable to its pre-turn state via `ass-guard checkpoint
restore` (EARLY-01's shadow store, 14-01).

**MCP/plugin tools** carry NO annotations: they get the DEFAULTS (the 120000
timeout backstop; mutability-derived concurrency). Their explicit annotation
is a documented v1.2 knob.

**Agent note.** The Agent/Task tools are excluded from `DispatchBatch` (the
session tool loop dispatches subagents inline, `session.go` `isSubagentTool`)
— the annotation is the declared bound for inventory/audit and for any future
batch-path migration; today the nested turn is bounded by its own ctx and
iteration caps.

---

*14-06 / EARLY-06. Evidence tests: `internal/toolexec/iserror_corpus_test.go`.
Every row in this inventory is in a terminal state: closed-here (task named),
routed (destination named), or already-pinned (test named).*
