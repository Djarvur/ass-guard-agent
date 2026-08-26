# Phase 25: SEED-001 Kit Extraction (strictly last) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-26
**Phase:** 25 - SEED-001 Kit Extraction (strictly last)
**Areas discussed:** Module boundary & layout, Composition-root API shape, Frontend seam design, App-vs-kit boundary

**Context:** Phases 16–24 not yet planned — this discussion captures interface intent that constrains them (the roadmap's "strictly last" rationale). Research-before-questions was enabled (web research summarized per area).

---

## Module boundary & layout

| Option | Description | Selected |
|--------|-------------|----------|
| Single module, pkg/ re-home | Physical move of the KIT-01 packages to pkg/ in existing go.mod | ✓ (adapted) |
| Separate library module | Own go.mod — independent versioning, breaks atomic refactors | |
| Thin pkg/ facade | internal/ stays; pkg/ re-exports | |

**User's choice:** Single module — then free-text redirect: "suggestion: no pkg but kit, and kit/internal"
**Notes:** Directory named `kit/` (not pkg/), with `kit/internal/` for kit-private packages. Supersedes KIT-01's "pkg/" letter. Research basis: Go one-module-per-repo default ([Compass](https://medium.com/compass-true-north/catching-up-with-the-world-go-modules-in-a-monorepo-c3d1393d6024), [SO](https://stackoverflow.com/questions/76358794/how-to-import-packages-in-monorepo-within-go-service)).

### Module boundary (continued)

| Option | Description | Selected |
|--------|-------------|----------|
| Two-pass: move, then design | Verbatim re-home first, KIT-02 design second | ✓ |
| Single-pass redesign | Interfaces finalized during the move | |
| Pure re-home only | Minimal exports; design act starved | |

**User's choice:** Two-pass (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Mirror internal/ 1:1 | pkg/engine, pkg/session… per KIT-01 name | |
| Consolidate to fewer | Merge event+audit, toolcat+toolexec… | |
| Single root package | One god package | |

**User's choice:** free-text: kit/ tree with kit/internal — granularity follows the promotion scope; mirror discipline implied by verbatim pass 1.

| Option | Description | Selected |
|--------|-------------|----------|
| Strict list + runtime | KIT-01's 14 + internal/runtime | ✓ |
| Promote extras too | loop, defaults, version, drift… | |
| Planner discretion | package-by-package | |

**User's choice:** Strict list + runtime (Recommended)

### Module boundary (follow-ups)

| Option | Description | Selected |
|--------|-------------|----------|
| internal/ = app-only | Top-level internal/ keeps app packages | ✓ |
| Under cmd/ | App packages move under cmd/ass-guard/ | |
| Top-level app/ | New app/ tree | |

**User's choice:** internal/ = app-only (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Keep module path | …/ass-guard/kit/<pkg> imports | ✓ |
| Rename to kit identity | Rewrite every import now | |

**User's choice:** Keep module path (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Organic — as forced | kit/internal grows when extraction forces it | ✓ |
| Pre-designated set | enginebridge, cron internals, fakes from pass 1 | |
| None — all public | No escape hatch | |

**User's choice:** Organic — as forced (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Stay in .planning | Seeds canonical-ref'd from CONTEXT.md | ✓ |
| Copy to kit/docs/ | Travels with library; sync risk | |

**User's choice:** Stay in .planning (Recommended)

---

## Composition-root API shape

Research basis: libraries shouldn't own composition roots ([Seemann](https://blog.ploeh.dk/2011/07/28/CompositionRoot/), [Fowler](https://martinfowler.com/articles/dependency-composition.html)); constructor injection + config structs over functional options ([DI without frameworks](https://blog.devgenius.io/dependency-injection-in-go-patterns-without-frameworks-f5d0b105eb58)).

| Option | Description | Selected |
|--------|-------------|----------|
| App composes; kit = parts | Constructors + seams; acpserve.Run is THE composition root | ✓ |
| Layered: parts + Compose | Plus kit.Compose convenience assembly | |
| Mega-constructor only | kit.New(Config) sole entry | |

**User's choice:** App composes; kit = parts (NOT the layered recommendation — operator took the purer model)

| Option | Description | Selected |
|--------|-------------|----------|
| Flat struct, mirrored names | RunnerConfig field names carried | ✓ |
| Nested per-subsystem | Engine{}, Session{} blocks | |
| Functional options | WithProfile(…) | |

**User's choice:** Flat struct, mirrored names (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Thin constructor, explicit steps | Caller invokes startup sequence | ✓ |
| Handle with Start/Stop | Kit runs the sequence internally | |
| Framework Run(ctx) | Kit owns the loop | |

**User's choice:** Thin constructor, explicit steps (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal, demand-grown | Export only what compiles + seam; grow on consumer demand | ✓ |
| Curated full API | Friendly public face per package now | |

**User's choice:** Minimal, demand-grown (Recommended)

---

## Frontend seam design

Research basis: transport-neutral port in the domain, adapters translate ([Optivem](https://journal.optivem.com/p/hexagonal-architecture-ports-and-adapters), [Ports & Adapters for agentic AI](https://www.linkedin.com/pulse/ports-adapters-ai-why-hexagonal-architecture-still-wins-varun-singh-l9owe), [Ableno workshop](https://www.ableneo.com/insight/hexagonal-architecture-for-ai-integration/)).

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal pair | Emitter + Requester only | ✓ |
| Full frontend port | + lifecycle, commands, config | |
| Planner derives | Defer membership to planning | |

**User's choice:** Minimal pair (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Neutral events, one Emit | Kit vocabulary; acp adapter translates | ✓ |
| Typed method set | EmitMessage/EmitToolCall/… | |
| Opaque pass-through | Kit never inspects | |

**User's choice:** Neutral events, one Emit (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| ctx-blocking, kit timeout | Request(ctx, Ask) Answer; D-01-style timeout | ✓ |
| Async handle/channel | Frontend resolves later | |
| Wrap existing ask path | Interface alias only | |

**User's choice:** ctx-blocking, kit timeout (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Interface port now | Scheduler port; core depends on interface | ✓ |
| Stays concrete in kit | Cron rides inside runtime | |
| Planner judges | From import evidence | |

**User's choice:** Interface port now (Recommended) — the Phase-15 D-13 amendment's named successor commitment honored.

---

## App-vs-kit boundary

| Option | Description | Selected |
|--------|-------------|----------|
| Injected catalog | Kit consumes injected command/skill/plugin catalog | ✓ |
| Promote with defaults | Ecosys into kit, .claude/ discovery shipped | |

**User's choice:** Injected catalog (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| By construction | io.Writer/interfaces; no os.Stdout in kit/ | ✓ |
| Policy struct | KitPolicies toggles | |
| Convention + review | Discipline only | |

**User's choice:** By construction (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| CI-enforced | mise ci gate: kit/ imports nothing from internal/ or cmd/ | ✓ |
| Convention only | Documented direction | |

**User's choice:** CI-enforced (Recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| Full v1.1 gate family | mise ci + ledger + CLI golden + behavioral eval suites | ✓ |
| Compile+unit only | Eval suites manual | |

**User's choice:** Full v1.1 gate family (Recommended)

---

## Deferred Ideas

None — discussion stayed within phase scope.
