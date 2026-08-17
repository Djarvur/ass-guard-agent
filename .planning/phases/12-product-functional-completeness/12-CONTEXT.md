# Phase 12: Product Functional Completeness - Context

**Gathered:** 2026-08-17
**Status:** Ready for planning

> **Provenance note.** The operator launched this discuss via the GSD manager and did not
> respond to the gray-area selection (standing autonomy pattern per the 2026-08-15
> overnight delegation precedent). All four gray areas were auto-selected and resolved
> from locked prior decisions; every D-01..D-04 below is tagged
> *[Auto-derived — retroactive confirmation invited]*. Override by editing this file
> before `/gsd:plan-phase 12`.

<domain>
## Phase Boundary

Every tool in the captured catalog executes for real — the 9 built-ins still returning
`no implementation yet` (AskUserQuestion, EnterPlanMode/ExitPlanMode, SendMessage,
ReadSessionContext, CronCreate/CronList/CronDelete, TaskStop) plus Bash
`run_in_background`/`dangerouslyDisableSandbox` — with result forms capture-pinned
(ACP-07 re-pins the corpus-absent forms), turn behavior guarded by a behavioral-eval
regression net (ACP-08), and Claude-Code-compatible plugin installs discovered (ACP-10).
The model never dead-ends mid-task. The machinery half of the 2026-08-16 re-scope +
split; Phase 13 (OpenSpec Workflow Completion) extends this phase's eval suites and
consumes ACP-01's AskUserQuestion route.

</domain>

<decisions>
## Implementation Decisions

### AskUserQuestion non-answer policy (ACP-01)
- **D-01:** An unanswered `AskUserQuestion` **waits a configurable interval, then returns a capture-shaped non-answer** — default 10 minutes; `0` = block forever (interactive mode). On timeout the tool result is the corpus's non-answer/timeout form if one exists in the pinned capture; otherwise a documented corpus-informed default, routed exactly like ACP-07's corpus-absent forms. The surface route stays as the ROADMAP locks it (question + options reach the client **in the captured shape**; turn suspends on the engine's ask path; reply lands as the tool result). Rationale: block-forever would recreate the Phase-8 stage-4 question-shaped-ending residual in prettier form — the hands-off core value requires the model to receive *something* and proceed or decline. The no-confirmation-tier safety model is untouched (model-initiated question, not a tool-execution gate). *[Auto-derived — retroactive confirmation invited]*

### Cron firing semantics (ACP-04)
- **D-02:** **Queue + fire-once catch-up.** While a turn is active, a due prompt queues and fires as an engine-driven turn after the active turn completes — serialized, never interrupts (mirrors the mutating-alone-in-slot discipline). While the agent isn't running, missed schedules persist and fire **once** on the next active session (`lastFired` persisted per automation — no duplicate catch-up), each catch-up firing carrying a missed-window note in its prompt context. Persistence: per-project JSON store under `.ass-guard/schedule/` (0600, alongside the existing `.ass-guard/` artifact family). No daemon, no network port — the editor-owned lifecycle is preserved by construction. *[Auto-derived — retroactive confirmation invited]*

### Eval-net gate scope & cost (ACP-08)
- **D-03:** **Change-class gate, pass@k=1 in the gate, deeper k local-only.** Deterministic tool-unit tests run in every `mise ci`. The scenario suites (real binary, scratch project, pass@k) gate **only on the locked change classes** — profile / model / turn-behavior diffs — at **k=1**, budgeted ~8–10 min (the Phase-8 flagship E2E measured 474–505s), wired with the established env-flag pattern (e.g. `ASSGUARD_EVAL_GATE=1`, mirroring `ASSGUARD_OPENSPEC_BIN=1`). A deeper pass@k=3 target exists as a manual/local mise task, not in the gate. Initial suite = the Phase-8-proven `explore → propose → apply → archive` scenario. *[Auto-derived — retroactive confirmation invited]*

### Phase-9 follow-ups, /tmp archival & the rollout-loss finding (feeds ACP-07)
- **D-04:** **Archive the /tmp driver kit as an immediate quick task; substantive follow-ups are Phase-12 plan items.** Facts established during this discussion (2026-08-17):
  - `/tmp/zcode-recapture/` (17 files: the app-server stdio driver, scripted workload, probe, capture-line dumps, parity results) is **alive but ephemeral** — the only proven re-capture mechanism and the surviving partial ground truth (`capture-line-base.json` / `capture-line-probe.json` hold full redacted request records).
  - The pinned rollout file `model-io-sess_3cee56ae….jsonl` is **gone from all live locations** (`~/.zcode/cli/rollout/` rotated it off; not in /tmp; not in the repo) — only `~/.zcode/cli/{artifacts,agents,exec}/sess_3cee56ae…` side-dirs survive. 09-VERIFICATION.md already records this caveat.
  - Consequence for ACP-07: "re-pin against the Phase-9 pinned session" can no longer mean re-reading that rollout file. The **re-record path is primary**: re-record curated-suite expectations + result forms from **current live zcode** using the archived driver (the accepted UAT disposition's follow-up #1), with a fresh qualifying capture as the equivalent of the original pin. The `ExtractTurnsFromRollout` delta-record/state-pollution fixes (follow-up #2) remain in scope so from-rollout A/Bs become usable evidence again.
  - The archival itself is mechanical (copy + docs note) and must not wait for Phase-12 planning: dispatch `/gsd-quick` immediately. Destination follows existing repo conventions for capture assets (quick task confirms exact path). *[Auto-derived — retroactive confirmation invited; the underlying facts are verified, not derived]*

### Claude's Discretion
- Executor grouping/wave structure for the 9 tools (planner territory; note ACP-05 TaskStop and ACP-06 Bash background flags are coupled — both target background work)
- Plugin-discovery precedence details (ACP-10): default to the existing `.ass-guard` > project `.claude` > user `.claude` chain with plugin roots merging per PLUG-05's documented-precedence requirement; the zcode root (`~/.zcode/cli/plugins/`) is a read source per the requirement
- The D-01 timeout default fine-tuning if the corpus shows a different convention
- Schedule-store file naming/rotation details under `.ass-guard/schedule/`
- Whether the `agents/`/`artifacts/`/`exec/` sess_3cee56ae side-dirs hold salvageable content for ACP-07 (planner/researcher check)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` §"Product Functional Completeness" — ACP-01..08 + ACP-10 (the locked scope)
- `.planning/ROADMAP.md` §"Phase 12" — goal, 9 success criteria, phase gate (incl. the zero-`no implementation yet` grep gate + live-serve AskUserQuestion gate)

### Research ground truth (2026-08-14 editions)
- `.planning/research/ECOSYSTEM-AUDIT.md` §4/§5 — esp. §4.4 EVAL-01..03 (the eval net's design source, named by the ROADMAP)
- `.planning/research/ARCHITECTURE.md` — integration points (toolexec seam, engine ask path)
- `.planning/research/PITFALLS.md` — capture-pinning discipline, stub-only-closure bans
- `.planning/research/SUMMARY.md` — cross-cutting invariants

### The executor template (Phase 8)
- `.planning/phases/08-slash-command-kickoff/08-08-SUMMARY.md` — the deferred-tools table (the 9 tools, rationale, corpus-absent forms flagged) — named by the ROADMAP as required context
- `.planning/phases/08-slash-command-kickoff/08-CONTEXT.md` — D-10 (structured failure surfacing), D-11 (mutating-command boundaries), the core-executor decisions every new executor follows

### Phase-9 ground truth & accepted dispositions
- `profiles/zcode/drift-reports/2026-08-16-recapture.md` — the parity re-baseline record + capture-method deviation (accepted)
- `.planning/phases/09-serve-path-audit-zcode-parity-re-capture/09-UAT.md` — accepted dispositions routing the two follow-ups into this phase's scope
- `.planning/phases/09-serve-path-audit-zcode-parity-re-capture/09-VERIFICATION.md` — the pinned-rollout-rotation caveat (D-04's fact source)
- `.planning/phases/09-serve-path-audit-zcode-parity-re-capture/09-CONTEXT.md` — D-01..D-04 audit decisions (body store, per-session JSONL, engine-decision provenance)
- `docs/recapture-runbook.md` — the re-capture procedure the archived driver automates

### Code ground truth
- `internal/toolexec/real.go` — the `no implementation yet` fallback (line ~85): the dispatch site every new executor registers into
- `internal/coreexec/` — the 08-08 core-executor package (RegisterCore, captured result forms): the template
- `internal/engine/decide.go` + `internal/engine/observe.go` — the ask path + EngineDecision observation ACP-01/ACP-04 hook into
- `internal/toolcat/catalog.go` — the catalog registry (schema-never-rewritten discipline)
- `cmd/ass-guard/acp_serve.go` — the sessionFor/RegisterCore wiring site
- `profiles/zcode/tools.json` — the captured 103-tool catalog (the 19-tool built-in working set the phase gate greps)
- `/tmp/zcode-recapture/` — **ephemeral** dev-time driver kit (17 files); archive target per D-04 — copy into the repo before it reaps

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/coreexec` — the six-tool executor set with captured result forms; every new executor clones this pattern
- `internal/toolexec` — the dispatch seam (`real.go`) with the swappable-Backend precedent (WebSearch/WebFetch)
- `internal/engine` — decide/observe: the ask path and EngineDecision audit lines (ACP-01 suspension + ACP-04 engine-driven turns + the eval gate's evidence source)
- `internal/ecosys` — discovery + precedence chain ACP-10 extends with plugin roots
- Env-flag gated real-binary test pattern (`ASSGUARD_OPENSPEC_BIN=1`) — the eval gate reuses it

### Established Patterns
- Dynamic-merge-into-captured-shape (MCP tools → skills listing → plugin skills/commands)
- Catalog schema never rewritten — executors fill in behind the captured schema
- `.claude/` strictly read-only; `.ass-guard/` writes only; per-session artifacts under `.ass-guard/`
- Mutating-alone-in-slot serialization (D-02's cron queueing mirrors it)
- No stub-only closure; every phase gate = `mise ci` + real-binary/live-service evidence

### Integration Points
- `sessionFor` in `cmd/ass-guard/acp_serve.go` — where RegisterCore-style wiring lands
- The turn runner — where engine-driven (cron-fired) turns queue behind active turns
- The audit trail's EngineDecision lines — the eval gate's behavioral evidence
- `profiles/zcode/coverage.yaml` — where the pinned-session id lives (now pointing at a rotated-off file; see D-04)

</code_context>

<specifics>
## Specific Ideas

- The derivation context: operator non-response under the manager dispatch; decisions follow the locked 2026-08-16 re-scope discipline (zero new dependencies in v1.1 — cron and eval machinery are hand-rolled)
- The hands-off philosophy (from prior phases): the product's core value is that work completes without manual taps — D-01 exists so a model question cannot silently dead-end a run, D-02 so a due schedule cannot be lost to an idle process

</specifics>

<deferred>
## Deferred Ideas

- Native ACP permission-surface re-shaping of AskUserQuestion — rejected for v1.1 (the captured tool shape is the locked route); revisit only if clients can't render the captured shape
- pass@k > 3 eval depth, cross-model eval matrices — post-v1.1 hardening
- Advanced cron semantics (calendars, timezones, missed-window staleness bounds beyond fire-once) — v1.2 candidate
- OS-backup (Time Machine) salvage attempt for the lost `model-io-sess_3cee56ae….jsonl` — operator-side, optional; the re-record path makes it non-blocking
- OTLP audit export, raw-body opt-in — already deferred in 09-CONTEXT

</deferred>

---

*Phase: 12-Product Functional Completeness*
*Context gathered: 2026-08-17*
