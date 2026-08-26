# Phase 25: SEED-001 Kit Extraction (strictly last) - Context

**Gathered:** 2026-08-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Extract the agent-building machinery as a library in this repo: a top-level `kit/` tree (single Go module, unchanged module path) holding the KIT-01 package set plus the Phase-15 runtime carve, behind constructors + seam interfaces the app's composition root calls. The frontend seam (Emitter + Requester, defined kit-side with a neutral event vocabulary, implemented acp-side) is the one genuinely new design act, alongside the cron Scheduler port deferred from Phase 15. ass-guard becomes the reference app: `internal/` keeps app-only packages, `cmd` keeps the CLI, `mise ci` + CLI golden + behavioral eval suites prove identical behavior. Strictly last — Phases 16–24 settle the surfaces being extracted.

</domain>

<decisions>
## Implementation Decisions

### Module Boundary & Layout
- **D-01:** Single Go module — the kit is a top-level `kit/` directory inside the existing go.mod. No separate library module (atomic cross-cutting commits preserved; mise/goreleaser untouched; a module split can come later if external release demands it). — **Reversibility:** costly — moving to a separate go.mod later rewrites every import path and doubles tooling; doing it now would break atomic refactors across kit/app.
- **D-02:** Directory is named `kit/`, NOT `pkg/` (operator free-text: "no pkg but kit, and kit/internal"). Kit-private packages live under `kit/internal/`. This supersedes the letter of KIT-01's "pkg/" — the requirement's intent (importable as a library, outside app-internal visibility) is honored by the kit/ tree. — **Reversibility:** one-way — import paths `…/ass-guard/kit/<pkg>` become the library's public contract; renaming after external consumers exist breaks published import paths.
- **D-03:** Two-pass migration discipline: pass 1 = verbatim re-home (pure moves, `git diff --color-moved` reviewable, `mise ci` as equivalence proof — the Phase-15 discipline); pass 2 = KIT-02 seam + cron port + composition surface design on settled code. Equivalence and design never mix in one diff.
- **D-04:** `kit/internal/` seeded organically — only what extraction forces (likely the Phase-15 enginebridge adapters and shared unexported seams). No pre-designated set; grows as forced, never preemptively public.
- **D-05:** Promotion scope: strict KIT-01 list (profile, shaper, provider, modelrouting, toolcat, toolexec, engine, hookdag, event, session, redact, checkpoint, audit, mcp) + `internal/runtime` (the Phase-15 carve whose package doc already declares kit ambition). Everything else stays app-side unless a kit interface genuinely requires it, planner justifying per-case.
- **D-06:** After extraction, `internal/` = app-only (acpserve, providerfactory, checkpointcmd, learningcmd, modelroutingcmd, profilecheckcmd, paritycli, acp, ecosys, openspec, coreexec, learning, firstrun, evalsuite, parity). `kit/` vs `internal/` is the top-level split; no app/ tree, no moving app packages under cmd/.
- **D-07:** Module path unchanged (`github.com/…/ass-guard`); kit imports read `…/ass-guard/kit/<pkg>`. Rename to a kit identity is possible pre-open-sourcing; not now.
- **D-08:** SEED-002 fantasy + SEED-003 landscape stay in `.planning/seeds/` as KIT-03 reading material — canonical-ref'd here, zero runtime deps trivially satisfied.

### Composition-Root API Shape
- **D-09:** App composes; kit = parts. Every kit package exports its constructors + seam interfaces; `acpserve.Run` stays THE composition root. KIT-01's "composition-root API" means the API the app's composition root calls. No `kit.Compose`/`kit.New` mega-assembly — that option was explicitly rejected in favor of the pure parts model.
- **D-10:** Assembly config = flat struct with mirrored names (Phase-15 D-05 precedent): field names mirror today's RunnerConfig (Bus, Profile, WorkDir, MakeProvider, …). No nested per-subsystem blocks, no functional options.
- **D-11:** Thin constructor, explicit steps (Phase-15 D-05 carried): constructors fill structs; the caller invokes startup steps explicitly (loadCommandRegistry / setupEngine / startScheduler ordering). No Start/Stop handle, no framework `Run(ctx)` — the app owns context/cancel per ACP transport discipline.
- **D-12:** Export policy: minimal, demand-grown (Phase-15 D-19 carried). Export only what the reference app + KIT-02 seam + cross-package kit internals need; the public API grows when a real consumer (Telegram, v1.3) forces it. No curated godoc-first API before a second consumer exists.

### Frontend Seam (KIT-02 — the new design act)
- **D-13:** Seam breadth: minimal pair — **Emitter** (kit events streaming out) + **Requester** (asks coming in: permissions/elicitation). Exactly KIT-02's letter. Session lifecycle, command advertisement, config options stay app-side surfaces — Phase 16–18 own those shapes; redefining them here would invalidate settled surfaces (the "strictly last" trap).
- **D-14:** Emitter granularity: kit-neutral event vocabulary (session-update, tool-call, plan-delta, …) + a single `Emit(ctx, Event)` method. The acp adapter translates kit events → ACP wire frames. Phase-15 D-20's "no ACP words in runtime API" holds by construction; a Telegram adapter would translate the same events. No typed method-set interface (freezes wide API), no opaque pass-through (proves nothing).
- **D-15:** Requester contract: ctx-blocking `Request(ctx, Ask) Answer`; timeout policy is kit-side config mirroring Phase-12 D-01 (wait interval then non-answer capture-shaped result). Matches today's suspended-turn ask semantics — turn suspends, reply lands as the result.
- **D-16:** Cron seam: interface port now. The Phase-15 D-13 amendment's deferred design work lands here: a Scheduler port defined on the kit side, cron wiring implements it, runner core depends on the interface — the bidirectional core↔cron calls (4 of 9 cron methods called from core; runAutomationTurn → runOneTurn) become interface calls. No second deferral: this is the named successor phase.

### App-vs-Kit Boundary
- **D-17:** Ecosystem blindness: the kit consumes an injected command/skill/plugin catalog (interface defined kit-side, mirroring the loadCommandRegistry seam). `.claude/` discovery, precedence merging, OpenSpec hosting stay app-side (ecosys/openspec per KIT-03). SEED-001's "invert .claude/ layout" is satisfied by injection, not promotion.
- **D-18:** SEED-001's app invariants (stdout discipline, read-only dirs) express kit-side by construction: kit APIs take io.Writer / interfaces; no os.Stdout, no hardcoded paths, no `.claude/` assumptions anywhere in kit/. No policy-toggle structs — the app passes what it wants enforced.
- **D-19:** Import direction CI-enforced: kit/ must import nothing from internal/ or cmd/ — mechanical gate in `mise ci` (grep/depguard-style), fails the build. Direction app→kit only; enforced, not conventional.
- **D-20:** Equivalence bar: the full gate family — `mise ci` green + relocated-test ledger match + CLI-contract golden (Phase-15 wave-0 instruments reused) PLUS the behavioral eval suites (change-class gate) — criterion #3's "all behavioral eval suites still pass against the extracted layout" verbatim.

### Claude's Discretion
- Exact kit event vocabulary enum members (session-update, tool-call, plan-delta are directional placeholders, not locked names).
- Scheduler port method shape (driven by the 9 cron methods' actual call graph at planning time).
- Which Phase-15 enginebridge/seam artifacts land in kit/internal vs a public sub-package (organic per D-04).
- Physical file layout within kit packages (mirrors internal/ naming unless a collision forces otherwise).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §Kit Extraction — KIT-01/02/03 verbatim (note: KIT-01's "pkg/" is superseded by D-02's kit/ naming — operator decision 2026-08-26)
- `.planning/ROADMAP.md` §Phase 25 — goal, "strictly last" rationale, 4 success criteria
- `.planning/PROJECT.md` §Current Milestone — SEED-001 line ("rides internal/runtime extraction; SEED-002 fantasy + SEED-003 landscape as design prior art")

### Seed Documents
- `.planning/seeds/SEED-001-agent-creation-kit-library-with-ass-guard-as-first-app.md` — THE vision doc: kit value proposition ("build your own agent with the desired shape"), scope estimate, breadcrumbs, the app-invariants tension D-18 resolves
- `.planning/seeds/SEED-002-mine-charmbracelet-fantasy-for-patterns-and-prior-art.md` — design prior art (KIT-03 reading material)
- `.planning/seeds/SEED-003-go-agent-reference-landscape-frameworks-and-coding-agents.md` — Go agent landscape reference (KIT-03 reading material)

### Prior Phase Contracts
- `.planning/phases/15-internal-runtime-carve-step-0/15-CONTEXT.md` — D-05/D-13-amendment/D-16/D-18/D-19/D-20 decisions this phase carries; the carve whose output is the kit core
- `.planning/phases/15-internal-runtime-carve-step-0/15-VALIDATION.md` — wave-0 instruments (test ledger, CLI golden) D-20 reuses
- `.planning/milestones/v1.1-phases/12-product-functional-completeness/12-CONTEXT.md` D-01 — ask-timeout semantics D-15 mirrors

### Verification Gate
- `.mise.toml` [tasks.ci] — extended per D-19 (import-direction gate) and D-20 (eval suites)

No external specs/ADRs — decisions above capture the full contract.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Phase-15 carve output: `internal/runtime` (Runner, RunnerConfig, NewRunner) + `internal/runtime/enginebridge` — the kit core, already documented as SEED-001 kit ambition (D-20 of Phase 15).
- `emitFor` RunnerConfig func-field (Phase-15 D-16) — the seam KIT-02's Emitter formalizes; acpserve already injects srv.Emitter post-construction.
- askBroker callback wiring inside sessionFor — the suspended-turn machinery KIT-02's Requester formalizes.
- Phase-15 wave-0 instruments: 130-test ledger + CLI-contract golden test — D-20 reuses both, extended to kit/ layout.
- `internal/acp` TurnRunner/SessionCloser/ChunkEmitter interfaces — existing inversion precedent; the kit Emitter/Requester pair joins this family (acp stays the adapter side).

### Established Patterns
- Verbatim-relocation discipline: pure moves, color-moved review, mise ci as equivalence proof — D-03's pass 1 is this pattern at tree scale.
- OnClose func-seam / one-method adapters at wiring site — informs where Emitter/Requester/Scheduler-port adapters live (app side, at composition).
- Thin-constructor + explicit startup ordering — D-11; graceful-degradation startup order is a documented contract.
- Package-per-concern app homes from Phase 15 (providerfactory, checkpointcmd, …) — these stay internal/ per D-06; only their kit-facing halves promote.

### Integration Points
- acpserve.Run = composition root (D-09): builds Options, calls kit constructors, injects Emitter/Requester/catalog adapters.
- `mise ci` gains the kit-import-direction gate (D-19).
- go:embed seed / firstrun stays app-side (D-05 strict list — firstrun not promoted).
- goreleaser/CGO_ENABLED=0 build unchanged (D-01 single module).

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- "no pkg but kit, and kit/internal" — the kit gets its own tree name, with an escape hatch for privates; the library's identity is first-class, not a Go-convention afterthought.
- "App composes; kit = parts" chosen over layered Compose — rejection of the mega-constructor echoed Phase 15's own D-05 rejection; the composition root stays exactly where it is today (acpserve.Run).
- All-recommended sweep on the frontend seam: the operator endorsed the minimal-pair/neutral-events/ctx-blocking/interface-port shape — the design act is bounded and reversible-by-smallness.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 25-seed-001-kit-extraction-strictly-last*
*Context gathered: 2026-08-26*
