# ass-guard kit

The agent-building machinery of ass-guard, extracted as a library: the turn
runner, session model, provider adapters, tool catalog, engine, and the
scheduling/routing layer — everything a frontend needs to host an
ass-guard-shaped coding agent. The reference application (the ass-guard
binary: ACP over stdio, Telegram) composes this kit through its
composition root; the kit itself knows nothing about ACP, Telegram, or any
other frontend. That direction is mechanical: **the app imports the kit;
the kit never imports the app** — enforced by the D-19 gates (`mise
kit-boundary` and the depguard `KitBoundary` lint rule; non-test kit files
deny `internal/` and `cmd/` imports).

## Composition root, not a mega-constructor

There is no `kit.NewAgent(everything...)`. A host constructs a
`runtime.Runner` from a flat `RunnerConfig` and then wires the seams it
wants — the reference composition lives in the app's `acpserve.Run`
(see `internal/acpserve`), which is the canonical example of hosting this
kit. Anything the host does not wire rides a documented degraded path
below; nothing panics.

## The six seams

| Seam | Interface | One-line contract |
|------|-----------|-------------------|
| Emitter | `runtime.Emitter` | One method — the frontend receives every turn event as neutral `kit/event` values (D-14: transport-blind; the frontend projects onto its wire) |
| Requester | `runtime.Requester` | ctx-blocking ask/answer — permission dialogs and questions suspend on it; fail-safe decline on error, never a silent allow (D-15) |
| Scheduler | `kit/runtime` Scheduler port | Time-windowed model routing; nil = no schedule surface (D-16) |
| LearnedStore | one-method store | The learning loop's persistence; nil = no learned state (D-12) |
| CommandCatalog | `runtime.CommandCatalog` | Discovery feed: commands/skills/agents/hooks/memory listings + resolution; nil = plain-text turns, no expansion (D-17) |
| SessionToolkit | `runtime.SessionToolkit` | ONE coarse `Attach` returning a `Reaper` — core tool registration, permissions, sandbox, background tasks ride it (OQ1) |

## Nil-degradation table

| Unwired field | Documented behavior |
|---------------|---------------------|
| `Catalog` | Expansion off — `/commands` are ordinary text (the emptyCatalog contract) |
| `Toolkit` | Stub executor — no core tools execute (the `--no-engine` shape) |
| `OpenPermStore` | Rule-less sessions — implicit allow (the documented degrade) |
| `LaunchBackground` | Background dispatch returns a structured error |
| `AskSurfaceRenderer` | Asks still suspend and function; only the plain-text mirror is unwired |
| `Requester` unwired | Gated asks fail safe — decline, never a silent allow |

The non-ACP host proof lives at `kit/runtime/hostproof_test.go` — a
self-contained frontend (its own provider, emitter, requester) drives a
real turn with every optional seam nil, and imports zero app packages.

## Prior art / reading

Design documents live in the repository's planning tree (zero runtime
dependencies — the kit reads nothing from `.planning/`):

- `.planning/seeds/SEED-001-agent-creation-kit-library-with-ass-guard-as-first-app.md` — this kit's charter
- `.planning/seeds/SEED-002-mine-charmbracelet-fantasy-for-patterns-and-prior-art.md` — pattern references
- `.planning/seeds/SEED-003-go-agent-reference-landscape-frameworks-and-coding-agents.md` — the Go agent framework landscape
