# Phase 25: SEED-001 Kit Extraction (strictly last) - Pattern Map

**Mapped:** 2026-08-28
**Files analyzed:** 21 artifacts (15 promoted packages, 2 new seam/adapter code homes, 1 new test, 3 gate/config edits)
**Analogs found:** 19 / 21 (2 process-only artifacts have no code analog — the move process itself and organic `kit/internal/` seeding)

**Phase shape:** This is a *re-homing + seam-inversion* phase. Pass 1 moves 15 packages verbatim (`internal/X` → `kit/X`); pass 2 defines 5–6 kit-side seam interfaces and one app-side adapter. The moved packages are their own analogs (verbatim = no body edits, D-03); the pattern assignments below therefore concentrate on (a) the seam interfaces, (b) the app adapter, (c) the gate edits, and (d) the code that physically moves between kit and app.

## File Classification

| New/Modified Artifact | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `kit/event/` (move) | model (bus + event types) | pub-sub | self — verbatim move | exact (verbatim) |
| `kit/redact/` (move) | utility | transform | self — verbatim move | exact (verbatim) |
| `kit/checkpoint/` (move) | service | file-I/O | self — verbatim move | exact (verbatim) |
| `kit/profile/` (move) | model/config | transform | self — verbatim move | exact (verbatim) |
| `kit/audit/` (move) | service | file-I/O + pub-sub | self — verbatim move | exact (verbatim) |
| `kit/hookdag/` (move) | service | event-driven DAG | self — verbatim move | exact (verbatim) |
| `kit/shaper/` (move) | service | transform | self — verbatim move | exact (verbatim) |
| `kit/toolcat/` (move) | model/registry | CRUD | self — verbatim move | exact (verbatim) |
| `kit/provider/` (move) | service | request-response + streaming | self — verbatim move | exact (verbatim) |
| `kit/mcp/` (move) | service | subprocess stdio JSON-RPC | self — verbatim move | exact (verbatim) |
| `kit/toolexec/` (move) | service | request-response | self — verbatim move | exact (verbatim) |
| `kit/modelrouting/` (move) | service/config resolver | transform | self — verbatim move | exact (verbatim) |
| `kit/session/` (move) | model/core | CRUD + streaming | self — verbatim move (seam types added pass 2) | exact (verbatim) |
| `kit/engine/` (move) | service | event-driven | self — verbatim move | exact (verbatim) |
| `kit/runtime/` + `kit/runtime/enginebridge/` (move) | controller/orchestrator | event-driven + streaming | self for bodies; `internal/acp/server.go` for the seam shapes it gains | exact + role-match |
| `kit/runtime` seam file(s) (NEW — Emitter/Requester/Scheduler/CommandCatalog/SessionToolkit/Learned) | interface definitions | event-driven + request-response | `internal/acp/server.go:16-66` | exact (same pattern family) |
| `internal/acpserve/kit_adapter.go` or similar (NEW) | adapter/controller | streaming translation + request-response | `internal/acpserve/acp_serve.go` + moved forwarders from `internal/runtime/runtime.go:651-739` | role-match |
| `kit/runtime/hostproof_test.go` (NEW) | test | integration | `internal/runtime/provider_factory_helpers_test.go` (white-box style) + `internal/provider/provider.go` (fake-constructible interface) | role-match |
| `.mise.toml` (MODIFY) | config (CI gate) | batch | `.mise.toml:25-42` existing task shapes | exact |
| `.golangci.yml` (MODIFY) | config (lint gate) | batch | `.golangci.yml:16-22` existing depguard block | exact |
| `scripts/eval-change-class.sh` (MODIFY) | utility script | batch | `scripts/eval-change-class.sh:27-69` CLASSES + selftest | exact |

## Pattern Assignments

### The 15 promoted packages (pass 1 — verbatim re-home)

**Analog:** the Phase-15 carve discipline, documented in-repo at `internal/runtime/provider_factory_helpers_test.go:11-17` and `internal/runtime/enginebridge/enginebridge.go:1-7`.

No body edits. Each move = `git mv` of the **whole package directory** (embed payloads are file-adjacent and travel with the dir), then import-path rewrites via `goimports` (local-prefix already configured, `.golangci.yml:127-129`).

**go:embed payloads that MUST move with their package** (verified):
```go
// internal/modelrouting/load.go:18
//go:embed defaults/config.yaml
// internal/hookdag/config.go:79
//go:embed seeded.yaml
// internal/toolcat/catalog.go:11
//go:embed coretools.json
```

**The in-file discipline precedent** (`provider_factory_helpers_test.go:11-17`) — how carve decisions are recorded at the move site:
```go
// D-03 carve discipline: verbatim cmd-local duplicates of the two
// provider-factory test helpers ... The originals moved to
// internal/providerfactory with the suite in plan 15-02; no shared testutil
// package is created during the carve.
```

**Import block convention every kit file keeps** (`internal/runtime/runtime.go:1-39`): stdlib group, blank line, `github.com/Djarvur/ass-guard-agent/internal/...` group, blank line, subpackage group. After the move the module-internal group reads `…/ass-guard-agent/kit/…` — goimports handles regrouping.

---

### `kit/runtime` seam definitions (NEW — Emitter, Requester, Scheduler, CommandCatalog, SessionToolkit, Learned)

**Analog:** `internal/acp/server.go` — the repo's established seam-definition family (interface at the consumer side, contract in the doc comment, minimal method set, separate optional-capability interfaces).

**Interface + doc-comment style** (lines 16-23):
```go
// ChunkEmitter streams session/update notifications to the client during a turn.
// The adapter implements it; a TurnRunner calls AgentMessageChunk per chunk
// (ACP-04 streaming — NO full-turn buffering). The interface is the seam Plan
// 02-05 rewires to subscribe to the expanded event bus.
type ChunkEmitter interface {
	// AgentMessageChunk streams one text chunk as a session/update notification.
	AgentMessageChunk(messageID, text string) error
}
```

**Minimal-breadth discipline + optional-capability split** (lines 31-42) — the pattern for "seam stays minimal, extra capabilities are separate interfaces":
```go
type TurnRunner interface {
	Run(ctx context.Context, sessionID string, emit ChunkEmitter, prompt []ContentBlock) (stopReason string, err error)
}
// SessionCloser is an OPTIONAL capability a TurnRunner may implement ...
// It is a separate interface (not part of TurnRunner) so stub runners need
// not implement it.
type SessionCloser interface {
	CloseSession(sessionID string) error
}
```

**"Kit owns only its vocabulary" precedent** (lines 44-48, ConfigSurface) — the exact argument D-14 applies at kit scale:
```go
// ConfigSurface is the acp-side seam ... internal/acp owns only the wire shapes
// and the handler; the surface implementation (internal/acpserve) owns the menu
// semantics ... — acp stays free of modelrouting/providerfactory imports
// (wire-only dependency direction). The exact shape is executor-frozen and
// kept minimal:
```

**Emitter vocabulary source — already kit-neutral** (`internal/event/events.go:46-76`; `Kind()` discriminator pattern at lines 41/53):
```go
type AgentMessageChunk struct {
	TurnID    string
	MessageID string
	Content   string
}
func (AgentMessageChunk) Kind() string { return "AgentMessageChunk" }

type ToolCall struct {
	TurnID     string
	ToolCallID string
	Name       string
	Input      json.RawMessage
}
```

**Emitter injection precedent — setter + func-field** (`internal/runtime/runtime.go:190`, `1500-1502`):
```go
emitFor func(sessionID string) acp.ChunkEmitter   // Runner field, line 190
...
// SetEmitter injects the server-driven-turn chunk emitter (WINDOWS #3:
// strictly between server construction and scheduler start).
func (r *Runner) SetEmitter(emit func(sessionID string) acp.ChunkEmitter) { r.emitFor = emit }
```
Kit version: same shape, `acp.ChunkEmitter` → kit `Emitter` interface with `Emit(ctx, event.Event) error` (D-14 single method).

**Code that MOVES OUT of kit to the app adapter** (`internal/runtime/runtime.go:651-739` — the transport translation KIT-02 removes):
```go
func routeBusEvent(e event.Event, emit acp.ChunkEmitter, toolEmit acp.ActivityEmitter) {
	switch c := e.(type) {
	case event.AgentMessageChunk:
		_ = emit.AgentMessageChunk(c.MessageID, c.Content)
	case event.ToolCall:
		forwardToolCall(toolEmit, e)
	case event.ToolCallUpdate:
		forwardToolCallUpdate(toolEmit, e)
	}
}
```
plus `toolKindFor` (722-739, name→`acp.ToolKind*` table), `forwardToolCall` (666-682, builds `acp.ToolCallFrame`), `forwardToolCallUpdate` (687-703, decodes into `acp.ToolCallUpdateFrame`). The forwarder loop that stays kit-side (`startChunkForwarder`, 591-645) switches its `route` closure from `routeBusEvent(e, emit, toolEmit)` to `emit.Emit(ctx, e)`.

**Requester analog — AskBroker timeout contract** (`internal/session/ask.go:112-122`; kind constants 85-91):
```go
// NewAskBroker returns a broker with the D-01 timeout (a NEGATIVE value
// normalizes to DefaultAskTimeout; 0 = block forever — no timer is armed) and
// the optional surface callback (fired once per surfaced ask, before the
// suspending turn's response reaches the client).
func NewAskBroker(timeout time.Duration, onSurface func(PendingAsk)) *AskBroker {
	if timeout < 0 {
		timeout = DefaultAskTimeout
	}
	return &AskBroker{onSurface: onSurface, timeout: timeout}
}
```
The Requester seam (D-15: ctx-blocking `Request(ctx, Ask) (Answer, error)`) wraps this; AskBroker stays kit-side, the acp implementation lands in the locked 17-02 `acpserve/ask_surface.go` home.

**Scheduler port — the exact four store calls to mirror** (`internal/sched/sched.go` methods at lines 166, 339, 374, 405; value structs to promote at 56-73 and 94-97; call sites `internal/runtime/cron_wiring.go:116-140, 216`):
```go
// sched.go — pure data, zero app deps (promote per D-05 per-case):
type Automation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	Cron   string `json:"cron,omitempty"`
	// ... LastFired/RunCount/CreatedAt/Done — json tags are the persisted shape,
	// untouched by the move
}
type FireEvent struct {
	Automation Automation
	Note       string
}
// cron_wiring.go — the kit core's entire schedule surface:
now := r.schedule.Now()                          // :116
due := r.schedule.Due(now)                       // :118
claimed, ok := r.schedule.ClaimForFire(a.ID, r.schedule.Now())  // :140
events := r.schedule.CatchUp(r.schedule.Now())   // :216
```
Current concrete-typed setter to invert (`runtime.go:1459-1461`):
```go
// SetSchedule assigns the per-project schedule store (the sched.Open success
// arm of the serve composition).
func (r *Runner) SetSchedule(store *sched.ScheduleStore) { r.schedule = store }
```

**Learned one-method port — the only use** (`internal/runtime/enginebridge/enginebridge.go:418-428`):
```go
func (d *ACPDispatcher) Ask(_ context.Context, situation string) (string, error) {
	if d.learned == nil {
		return "", engine.ErrAskPending
	}
	if e, ok := d.learned.Lookup(situation); ok {   // learning.Store.Lookup, store.go:70
		return e.Answer, nil
	}
	return "", engine.ErrAskPending
}
```
`internal/learning/store.go:70`: `func (s *Store) Lookup(situation string) (Entry, bool)` — note the return is `learning.Entry`, so the kit port either returns `(string, bool)` (answer only, what the call site consumes) or `Entry` promotes per D-05.

**CommandCatalog + SessionToolkit — the consumption sites they must cover** (`internal/runtime/runtime.go`):
```go
// :460-476 — slash parsing/lookup (ecosys.ParseInvocation + r.reg.Commands)
key, args, parsed := ecosys.ParseInvocation(blocks[idx].Text)
if _, found := r.reg.Commands[key]; !found { return "", "", false }

// :1091-1093 — Skill tool Execute override
if core, ok := sCatalog.Get(skillToolName); ok {
	core.Execute = ecosys.SkillExecute(r.reg)
	sCatalog.Register(core)
}

// :1106-1140 — the per-session toolkit construction site (the exact inputs
// the SessionToolkit env mirrors: dir, sessionID, mgr.Path(), hookRunner,
// taskRegistry, askBroker, planMode, mailbox, sessionReader, r.schedule):
hookRunner := ecosys.NewHookRunner(r.reg.Hooks, sessionID, dir, mgr.Path())
taskRegistry := coreexec.NewTaskRegistry()
coreexec.RegisterCore(sCatalog, coreexec.Config{WorkDir: dir, Todos: coreexec.NewTodoStore(), Hooks: hookRunner, Tasks: taskRegistry})
askBroker := session.NewAskBroker(r.askTimeout, func(p session.PendingAsk) {
	r.bus.Publish(event.AgentMessageChunk{...Content: coreexec.RenderAskSurface(p.Questions)})
})
coreexec.RegisterAsk(sCatalog, askBroker)
...
coreexec.RegisterInteractive(sCatalog, coreexec.InteractiveConfig{Ask: askBroker, PlanMode: planMode, Mailbox: mailbox, Sessions: sessionReader, Tasks: taskRegistry, Schedule: r.schedule})

// :1238-1243 — the reaper handle OnClose composes:
s.OnClose = func() error {
	cancelWriter()
	taskRegistry.ReapAll() // 12-06: no background group outlives the session
	return mcpHost.Close()
}

// :1218-1223 — the degraded path when no toolkit/engine (hostproof test relies on it):
if r.engineEnabled {
	s.SetToolExecutor(toolcat.NewMCPExecutor(&toolexec.RealExecutor{Catalog: sCatalog, Log: slog.Default()}, mcpHost))
} else {
	s.SetToolExecutor(toolcat.NewMCPExecutor(enginebridge.NewStubCatalogExec(), mcpHost))
}
```
`SetupEngine`'s app-side loads that become injected inputs (runtime.go:275-337): `openspec.DefaultConfig()`, `openspec.RegisterTools(catalog, oscfg)`, `openspec.FromConfig(oscfg)`, `hookdag.DefaultHooks()`, `learning.Open(learnedPath)` — the kit version takes these as arguments (D-11 explicit steps; D-17 app hosts ecosystem).

**Session's ecosys-typed fields the seams must neutralize** (`internal/session/session.go:57-70`):
```go
SubagentTypes map[string]ecosys.Agent
Hooks *ecosys.HookRunner
```

---

### `internal/acpserve` adapter (NEW — the composition-site adapter)

**Analog:** `internal/acpserve/acp_serve.go` — the composition root whose statement order is a behavioral contract, plus the frozen interfaces at `internal/acp/server.go:31-33`.

**Composition-root pattern to preserve exactly** (`acp_serve.go:164-251` — construction order is the contract; pass 2 adds adapter wiring WITHOUT reordering):
```go
runner := runtime.NewRunner(&runtime.RunnerConfig{
	Bus: bus, BodyStore: bodyStore, Profile: prof, WorkDir: opts.WorkDir,
	MaxConc: opts.MaxConcurrent, ConfigAdded: opts.ConfigAddedBoundaries,
	AskTimeout: opts.AskTimeout, ServeCtx: ctx,
	SchedCfg: schedCfg, ProviderName: providerName, Stderr: stderr,
	MakeProvider: func(capturer provider.RequestCapturer) provider.Provider {
		p, _ := factory.BuildWithCapturer(providerName, shaper.New(), capturer)
		return p
	},
})
runner.LoadCommandRegistry()
if opts.EngineEnabled {
	err := runner.SetupEngine()   // degrades, never crashes (D-04)
	...
}
srv := acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner), ...)
...
runner.SetSchedule(scheduleStore)
runner.SetEmitter(srv.Emitter) // WINDOWS #3: strictly between server construction and scheduler start
runner.StartScheduler(ctx)
```
The `MakeProvider` func-field (176-184) is the precedent for the new `Catalog`/`Toolkit`/`Learned` seam fields (D-10 flat struct, mirrored names).

**The adapter satisfies a FROZEN interface** (`internal/acp/server.go:31-33` — signature unchanged by this phase):
```go
type TurnRunner interface {
	Run(ctx context.Context, sessionID string, emit ChunkEmitter, prompt []ContentBlock) (stopReason string, err error)
}
```
Today `runtime.Runner` implements it directly (`runtime.go:479-482`); after pass 2 the new adapter implements it, converts `[]acp.ContentBlock` via the moved `toContentBlocks` (`runtime.go:1631-1639`), and maps stop reasons (`mapAskStop` via `runtime.go:1628`; kit keeps the raw `stop=` marker in cron audit lines per `cron_wiring.go:192-193`).

---

### `kit/runtime/hostproof_test.go` (NEW — KIT-02 automated criterion)

**Analogs:** white-box test style with in-file helpers (`internal/runtime/provider_factory_helpers_test.go:1-9` — `package runtime` + testify); fake-constructible seams because the key dependencies are already interfaces:
```go
// internal/provider/provider.go:19-30 — the kit constructs scripted fakes
// with ZERO app imports (D-18 by construction):
type Provider interface {
	Send(ctx context.Context, prof *profile.Profile, messages []Message) (Response, error)
	Stream(ctx context.Context, prof *profile.Profile, messages []Message) (<-chan StreamChunk, error)
	ToolResultMessage(toolCallID string, result json.RawMessage) (json.RawMessage, error)
}
```
Test shape: recording Emitter (one struct, one method), immediate Requester, scripted Provider, `NewRunner(&RunnerConfig{...})` with nil Catalog/Toolkit riding the documented degraded paths (`runtime.go:1218-1223` stub exec; `:1139` comment "nil in test runners → structured no-store errors"). Assert the turn completes, events recorded, and zero app packages in the import graph.

---

### `.mise.toml` (MODIFY — D-19 grep gate)

**Analog:** the existing task shapes at `.mise.toml:25-27` and `:40-42`:
```toml
[tasks.ci]
description = "Run the full CI gate: vet + lint + build + test"
depends = ["vet", "lint", "build", "test"]

[tasks.eval-check-changed]
description = "Detector: do the changes touch the locked eval-gate change classes? (exit 3 => run mise eval-gate)"
run = "./scripts/eval-change-class.sh"
```
New `[tasks.kit-boundary]` task joins `depends = ["vet", "lint", "build", "test", "kit-boundary"]`; the `! grep -rn "ass-guard-agent/internal/\|ass-guard-agent/cmd" kit --include='*.go' --exclude='*_test.go'` shape from RESEARCH.md §Code Examples. **Gate lands only in pass 2, after the last kit→app edge is severed** (D-03; enabling it in pass 1 fails the build by construction).

### `.golangci.yml` (MODIFY — depguard layer)

**Analog:** the existing rules block at `.golangci.yml:16-22` — the new `KitBoundary` rule is a sibling:
```yaml
    depguard:
      rules:
        Main:
          list-mode: lax
          deny:
            - pkg: "unsafe"
              desc: "unsafe breaks the static-binary + memory-safety guarantees"
```
New rule: `files: ["**/kit/**/*.go", "!**/kit/**/*_test.go"]`, `list-mode: strict`, deny `github.com/Djarvur/ass-guard-agent/internal` + `.../cmd` (RESEARCH.md §Code Examples has the full YAML).

### `scripts/eval-change-class.sh` (MODIFY — D-20 detector extension)

**Analog:** its own CLASSES block (lines 27-40) and selftest fixtures (42-69):
```sh
CLASSES='
profile:profiles/
model:internal/provider
model:internal/shaper
...
turn-behavior:internal/engine
turn-behavior:internal/session
turn-behavior:internal/coreexec
turn-behavior:cmd/ass-guard
...
'
```
Extend with `kit/…` equivalents (`turn-behavior:kit/engine`, `turn-behavior:kit/session`, `model:kit/provider`, `model:kit/shaper`, …) KEEPING the legacy paths through the transition; add matching + clean-path selftest fixtures (e.g. `check "kit/engine/decide.go"`). Without this the behavioral gate silently stops firing post-extraction (RESEARCH Pitfall 1).

---

## Shared Patterns

### Interface-at-consumer with contract doc comments
**Source:** `internal/acp/server.go:16-66`
**Apply to:** every kit seam definition (Emitter, Requester, Scheduler, CommandCatalog, SessionToolkit, Learned). Method sets minimal; optional capabilities split into separate interfaces; the doc comment states who implements and what degrades when nil.

### Flat mirror-config struct + thin constructor + explicit startup steps
**Sources:** `internal/runtime/runtime.go:215-254` (RunnerConfig, NewRunner fills fields only); `internal/runtime/enginebridge/enginebridge.go:30-56` (BridgeConfig — func values cross the boundary, unexported names never do); `acp_serve.go:164-251` (caller owns step order).
**Apply to:** all pass-2 composition-surface changes. New seam fields (`Catalog`, `Toolkit`, `Learned`, interface-typed `Schedule`) join RunnerConfig with mirrored names; no functional options, no nested blocks, no `kit.Compose`.

### One-method adapter at the wiring site
**Source:** `internal/runtime/runtime.go:256-268`:
```go
// checkpointerAdapter adapts *checkpoint.Store to session.Checkpointer: the
// store's method is Snapshot, the seam speaks SnapshotTurn — the one-method
// adapter lives at the wiring site so internal/session keeps no dependency
```
**Apply to:** the app-side implementations of Learned/Scheduler/CommandCatalog — adapters live in `internal/acpserve` (or the wiring site), never force the app package into the kit's vocabulary.

### nil = the documented degraded state
**Sources:** `runtime.go:143-146` (stderr nil → os.Stderr), `:107-109` (schedCfg nil → no override), `:1139` (nil schedule → structured no-store errors), `:1218-1223` (engine-off → stub executor), `:1198-1203` (nil checkpoint → typed-nil guard comment).
**Apply to:** all new seams — nil Emitter/Requester/Catalog/Toolkit must have a documented degradation, never a panic; the hostproof test rides these paths.

### stderr-only diagnostics via injected io.Writer
**Sources:** `runtime.go:143-146` (`stderr io.Writer` field, "stdout stays ACP-only"); `cron_wiring.go:201-204` (`fmt.Fprintf(r.stderrOrDefault(), ...)`).
**Apply to:** kit/ everywhere (D-18) — no `os.Stdout`, no `os.Stderr` literals in kit; writers injected. The acp adapter keeps `log.Printf`/stderr only (acp_serve.go pattern).

### Event Kind() discriminator for the neutral vocabulary
**Source:** `internal/event/events.go:41,53,65,76` — one-line `Kind()` per event struct.
**Apply to:** new kit event kinds (plan-delta, thought-chunk, ask-surface per discretion) — additive kinds on the existing bus, never a second emit path (D-14 single-method rule).

### Verbatim-move hygiene
**Sources:** `.golangci.yml:127-129` (goimports local-prefix); `provider_factory_helpers_test.go:11-17` (in-file decision records); Phase-15 ledger `.planning/phases/15-internal-runtime-carve-step-0/test-ledger.txt` (re-baseline instrument).
**Apply to:** pass 1 — `git mv` whole dirs, `goimports -w`, review with `git diff --color-moved=dimmed-zebra`, `mise ci` green per step, zero body edits.

## No Analog Found

| Artifact | Role | Data Flow | Reason |
|------|------|-----------|--------|
| Pass-1 move *process* (15-package leaf-first order) | process, not code | batch | No in-repo precedent at tree scale — Phase 15 moved one package family. Use RESEARCH.md §Move Order (import ranks) as the plan; the per-step discipline analogs above still apply. |
| `kit/internal/` seeding (D-04) | package layout | — | Organic by decision — no predetermined analog; seed only what extraction forces (likely enginebridge adapters). |

## Metadata

**Analog search scope:** `internal/{runtime,session,event,acp,acpserve,sched,learning,provider}`, `.mise.toml`, `.golangci.yml`, `scripts/eval-change-class.sh`
**Files read:** 20 (targeted, non-overlapping reads of runtime.go ×8; full reads of small analogs)
**Pattern extraction date:** 2026-08-28
**Note for planner:** pass-1 file inventory must be re-taken at execution time — locked Phases 17–24 plans add ~9 files inside the kit-bound packages before this phase executes (RESEARCH.md §Future surfaces). The seam contracts are stable across those plans; only file contents shift.
