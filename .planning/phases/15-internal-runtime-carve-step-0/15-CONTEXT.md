# Phase 15: internal/runtime Carve (Step 0) - Context

**Gathered:** 2026-08-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Mechanical verbatim relocation of `sessionTurnRunner` out of `cmd/ass-guard/acp_serve.go` into `internal/runtime` — plus the serve wiring and CLI-support logic leaving `cmd` into their own package-per-concern homes. Zero behavior change; the diff is a pure relocation (verbatim bodies, import fixes only), reviewable as such. No feature code moved or rewritten.

</domain>

<decisions>
## Implementation Decisions

### Test Relocation
- **D-01:** All runner-touching test files (~10 files constructing `sessionTurnRunner{}` directly) move verbatim into `internal/runtime/*_test.go` as same-package white-box tests.
- **D-02:** Mixed files split by subject: runner unit tests move; tests exercising the `acp.NewServer` handler layer stay in cmd (e.g. part of acp_serve_test.go's 27 tests).
- **D-03:** Small shared test helpers (fake providers, bus fixtures) are duplicated across packages rather than extracted into a shared testutil package — no new surface during carve.
- **D-04:** cmd keeps only true stdio E2E suites (spawn binary + JSON-RPC): e2e_opsx*, integration.

### Composition Surface
- **D-05:** Constructor shape: `runtime.NewRunner(RunnerConfig) *Runner`. RunnerConfig mirrors today's field names exactly (Bus, Profile, WorkDir, MakeProvider, …). Thin constructor — struct fill only; caller invokes loadCommandRegistry / setupEngine / startScheduler explicitly, preserving today's startup order. — **Reversibility:** costly — field names become kit API consumed by acpserve and all moved tests; renaming later touches every construction site plus Phase 25's public surface.
- **D-06:** runACPServe moves OUT of cmd but NOT into the kit package — new separate package `internal/acpserve`; cmd calls `acpserve.Run(ctx, in, out, stderr, opts)`. Serve concerns must not leak into the future kit. (Operator: "move, but this is not part of the kit, separate package.")
- **D-07:** ALL non-runner wiring in acp_serve.go moves to internal/acpserve verbatim: profile-dir helpers, perm warnings, audit mirror, redactorAdapter. serveOptions exported there as `Options`; cmd builds Options from flags.
- **D-08:** Other cmd non-test files leave cmd too, each to its own package-by-concern (operator: "again it should move but to the separate package"): checkpoint.go → its own pkg, learning_cmd.go → its own pkg, modelrouting.go → its own pkg, provider_factory.go → its own pkg, parity.go → its own pkg. Package per concern, not one combined package.
- **D-09:** Cobra boundary: only run*/resolve* logic functions move into the new packages; cobra command definitions stay in cmd and call into them. cmd remains the CLI-shape owner (root command + subcommand wiring).
- **D-10:** goconst_constants.go splits by destination: each new package takes the subset of constants it uses; verbatim duplication across packages allowed during carve.

### Carve Scope
- **D-11:** internal/runtime receives everything `sessionTurnRunner` needs to compile: runner struct + methods, `parkedChain`, `advisoryNote`, askBroker callback wiring inside sessionFor, expandUserBlocks/invocationFor, park/idle-tracking state, advisory dedupe.
- **D-12:** All 7 package-local adapter types move with the runner: stubCatalogExec, acpDispatcher, patternNextPrompter, hookSessionTurnRunner, hookSessionBoundaryOpener, realCommandRunner, engineTurnRunnerAdapter.
- **D-13:** Sub-package split NOW, 3-way (operator choice over single package): `internal/runtime` (runner core), `internal/runtime/enginebridge` (7 adapters + engine setup helpers), `internal/runtime/cron` (cron_wiring.go's 9 methods re-homed + scheduler loop). acpserve composes all three.
- **D-14:** enginebridge seam = MIRROR CONFIG (operator choice over exported accessors): enginebridge defines its own struct mirroring the runner fields the adapters need; acpserve fills both from one source. Divergence risk accepted deliberately.
- **D-15:** cron methods ride with the runtime family (not left at wiring layer) — they are methods on the runner today; they land in internal/runtime/cron.
- **D-16:** `emitFor` stays a RunnerConfig func field — acpserve injects `srv.Emitter` after NewRunner, same as today's `runner.emitFor = srv.Emitter`.
- **D-17:** checkpointerAdapter moves into internal/runtime beside its sessionFor use site; checkpoint.go's cobra/list/restore logic goes to its own package per D-08/D-09.

### Export Naming
- **D-18:** Exported name: `runtime.Runner`, constructor `runtime.NewRunner(cfg)`. Go stutter convention (not `runtime.TurnRunner` — avoids same-name-as-acp confusion; not `SessionTurnRunner` — too long). — **Reversibility:** costly — this is the Phase 25 kit's core public name.
- **D-19:** Method set unchanged: Run, CloseSession, WaitChainIdle keep names; sessionFor and friends stay unexported. Minimal exports — only what cmd/acpserve needs to compile. No pre-emptive exports for later phases (YAGNI).
- **D-20:** Kit-facing doc now: internal/runtime package doc states the kit ambition — this becomes SEED-001 kit core at Phase 25; acpserve is NOT kit; no ACP-specific words in runtime API naming.

### Claude's Discretion
- Exact per-test assignment when splitting mixed test files (subject-based judgment).
- Constant-by-constant placement within the "each package takes what it uses" rule.
- File layout within each new package (one file per concern or merged).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` §Runtime Carve — RUNT-01 defines the verbatim-move bar (`mise ci` proves equivalence)
- `.planning/ROADMAP.md` §Phase 15 — success criteria: zero behavior change, live Zed session identical, pure-relocation reviewable diff
- `.planning/PROJECT.md` §Current Milestone — v1.2 context; Phase 25 (SEED-001 kit extraction) is the downstream consumer of this carve

### Source Files (the move targets)
- `cmd/ass-guard/acp_serve.go` — sessionTurnRunner struct (line ~441) + 25+ methods; runACPServe + serveOptions (~107); 7 adapter types; parkedChain/advisoryNote
- `cmd/ass-guard/cron_wiring.go` — 9 additional runner methods (scheduler loop, automation turns, session forwarder)
- `cmd/ass-guard/checkpoint.go` — checkpointerAdapter + cobra checkpoint commands
- `cmd/ass-guard/learning_cmd.go` — learning list/revert CLI logic
- `cmd/ass-guard/modelrouting.go` — setupModelRouting (scheduling config load)
- `cmd/ass-guard/provider_factory.go` — setupProviderFactory
- `cmd/ass-guard/parity.go` — zcode version/drift CLI logic
- `cmd/ass-guard/goconst_constants.go` — shared constants to split by destination
- `internal/acp/server.go` — TurnRunner + SessionCloser + ChunkEmitter interfaces (runner implements; unchanged)

### Verification Gate
- `.mise.toml` [tasks.ci] — vet + lint + build + `go test -race ./...`; THE equivalence proof for RUNT-01

No external specs/ADRs — requirements fully captured in decisions above.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Import direction already clean: runner consumes internal/* (engine, hookdag, learning, toolcat, sched, session, audit, event, profile, modelrouting, ecosys, coreexec, provider, shaper) — no import-cycle risk expected in any destination package.
- `acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner))` — TurnRunner interface seam means the carved type plugs back identically.

### Established Patterns
- OnClose func-seam pattern (checkpointerAdapter precedent): one-method adapters live at wiring site so internals keep no dependency on siblings — informs where adapters go.
- `//nolint:funcorder` grouping comments throughout acp_serve.go — preserve them verbatim in moved bodies.
- Graceful-degradation discipline (engine-off path, schedule-store-open failure) — startup order must be preserved exactly (D-05 thin constructor).

### Integration Points
- `runACPServe` constructs runner literal at line ~361 — becomes RunnerConfig + NewRunner call in acpserve.
- `srv.Emitter` injection after server construction (emitFor) — ordering constraint: server exists before scheduler start.
- ~12 cmd test files construct the runner directly — split/move per D-01..D-04.
- evalsuite bridge + background_wiring test files reference the runner — subject-split like the rest.

</code_context>

<specifics>
## Specific Ideas

Operator framing that shaped decisions:
- "Move, but this is not the part of the kit, separate package" — serve wiring and CLI support leave cmd but stay OUT of internal/runtime; package per concern.
- Enginebridge mirror-config chosen over exported accessors despite divergence risk — keeps runner's unexported surface untouched.
- Kit-facing doc planted now so Phases 16–24 extend the right package and Phase 25 extraction is mostly mechanical.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 15-internal-runtime-carve-step-0*
*Context gathered: 2026-08-26*
