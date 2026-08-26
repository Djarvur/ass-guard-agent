# Phase 15: internal/runtime Carve (Step 0) - Pattern Map

**Mapped:** 2026-08-26
**Files analyzed:** 32 (10 production cmd files, 22 cmd test files) → 9 destination packages + cmd rewiring
**Analogs found:** 32 / 32 — every moved line's analog is its verbatim source; house-style analogs cited from existing `internal/*` packages

> **Reading note for the planner:** This is a *relocation* phase, so "analog" has two meanings here:
> 1. **The verbatim source** — the exact line ranges whose text becomes the new file (match quality: exact by construction).
> 2. **House-style precedents** — existing `internal/*` packages that establish how a *new* package in this repo looks (doc comments, constant duplication, error handling). Cited separately where they govern code that is newly *written* rather than moved (package docs, RunnerConfig, bridge constructors).
>
> All line numbers verified by direct read this session. Excerpts are load-bearing fragments only; the executor cuts from the source file, never from this document.

## File Classification

Roles use Go-flavored values: `composition-root` (build wiring, own process lifetime), `service` (long-lived stateful core), `adapter` (implements a foreign interface over local types), `cli-logic` (run*/resolve* bodies behind a cobra shell), `shared-infra` (consumed by multiple callers), `config` (options structs/constants), `test`.

| Destination File | Role | Data Flow | Closest Analog (verbatim source) | Match Quality |
|------------------|------|-----------|-----------------------------------|---------------|
| `internal/runtime/*.go` (Runner core) | service | event-driven (bus subs) + request-response (Run) | `cmd/ass-guard/acp_serve.go:441-544` (struct), methods per table below | exact |
| `internal/runtime/enginebridge/*.go` (7 adapters + engine setup) | adapter | transform (engine↔session seams) | `acp_serve.go:81-98, :549-612, :1716-2115`; shape per RESEARCH.md Pattern 2 | exact (source) / no-shape-analog (bridge struct) |
| `internal/runtime/cron_wiring.go` (scheduler family — SUPERSEDED destination: original runtime/cron row amended per D-13) | service | polling + event-driven forwarder | `cmd/ass-guard/cron_wiring.go:1-258` (whole file); mechanism per RESEARCH.md Pattern 3 | exact (source) / no-shape-analog (embedded owner) |
| `internal/acpserve/*.go` | composition-root | streaming (stdio frames) | `acp_serve.go:47-135, :194-311, :313-425`; serveOptions→`Options` per D-07 | exact |
| `internal/<checkpoint-cli>/*.go` | cli-logic | batch (list/restore) | `checkpoint.go:16-36, :97-146` (cobra def `:38-95` stays in cmd) | exact |
| `internal/<learning-cli>/*.go` | cli-logic | batch | `learning_cmd.go:53-134` (cobra def `:13-51` stays) | exact |
| `internal/<modelrouting-cli>/*.go` | cli-logic | request-response | `modelrouting.go:131-217` (cobra defs `:15-129` stay) | exact |
| `internal/<providerfactory>/*.go` | shared-infra | CRUD-ish config load | `provider_factory.go:1-171` (entire file; consumed by acpserve + main.go tracer + parity) | exact |
| `internal/<parity-cli>/*.go` | cli-logic | batch | `parity.go:21-44` (seams) + run logic below `:80` (cobra def `:46-78` stays) | exact |
| `internal/<profilecheck-cli>/*.go` *(OQ5)* | cli-logic | batch/file-I/O | `profile_check.go:55-236` (cobra defs `:17-53` stay) | exact |
| `cmd/ass-guard/main.go` (modified) | cli-logic (entrypoint) | — | stays; `AddCommand` wiring `:77-82` gains nothing, loses nothing (D-09) | exact |
| `cmd/ass-guard/acp_serve.go` (modified) | route (cobra shells) | — | retains `newACPCmd` `:139-192` + de-cobra'd `runACPServeCmd` `:198-231` shell | exact |
| `cmd/ass-guard/goconst_constants.go` (dissolved) | config | — | splits per D-10; duplication precedent `internal/acp/goconst_constants.go` | exact |

### Runner-core method → destination map (verified line numbers, `acp_serve.go` unless noted)

| Symbol | Lines | Destination | Notes |
|--------|-------|-------------|-------|
| `sessionTurnRunner` struct → `Runner` (D-18) | 441-544 | runtime core | all fields unexported — forces same-package white-box tests (D-01) |
| `setupEngine` | 549-612 | **enginebridge** (setup helper, D-13) — see OQ-mechanics below | constructs `acpDispatcher` literal `:596-608` reading `r.hookExec/r.hookCfg/r.learned/r.bus/r.patternTable` |
| `workDirOrDefault` | 615-623 | runtime core | |
| `loadCommandRegistry` | 631-657 | runtime core | called explicitly by acpserve (D-05 order) |
| `expandUserBlocks` / `firstTextBlockIndex` / `invocationFor` | 669-712 / 715-723 / 732-750 | runtime core | D-11 names expandUserBlocks/invocationFor explicitly |
| `Run` | 753-833 | runtime core | calls `sessionTurnMu` `:763`, `markClientTurn` `:770,:772` — cron-family coupling (OQ1) |
| `mapAskStop` / `stopAskACP` | 840-846 / 850 | runtime core | |
| `startChunkForwarder` | 856-892 | runtime core | |
| `runOneTurn` | 904-1012 | runtime core | constructs `engineTurnRunnerAdapter{r: r}` `:961` — construction-stays-in-core evidence |
| `parkedChain` + register/unregister/cancelParkedChains | 1016-1064 | runtime core | D-11 park state |
| `chainEnter/chainExit/chainCount` + `WaitChainIdle` + `chainIdlePollInterval` | 1070-1117 | runtime core | D-19 keeps `WaitChainIdle` exported |
| `sessionFor` | 1120-1405 | runtime core | D-11 centerpiece; `redactorAdapter{}` constructed `:1157,:1160` (OQ2); `checkpointerAdapter{}` `:1339` (D-17) |
| `serveCtxOrBackground` / `stderrOrDefault` | 1412-1418 / 1426-1432 | runtime core | |
| `resolveSubagentModel` | 1448-1468 | runtime core | free function (no receiver) |
| `spawnMCP` / `mergeMCPServers` / `nopHost` / `toProfileDecls` | 1475-1497 / 1505-1522 / 76 / 86-93 | runtime core | |
| `closeAllSessions` / `CloseSession` | 1527-1559 / 1564-1584 | runtime core | `CloseSession` satisfies `acp.SessionCloser` (D-19) |
| `advisoryNote` / `collectAdvisory` / `advisoryNoteText` / `advisoryNoteDue` | 1587-1670 | runtime core | D-11 advisory dedupe |
| `routeAskReply` / `toContentBlocks` | 1678-1696 / 1699-1706 | runtime core | |
| `engineTurnRunnerAdapter` (+Run/AskSettle/LastTurnOutput) | 1716-1925 | **enginebridge** (type, D-12) | reaches `a.r.invocationFor`, `a.r.sessMu`, `a.r.automationProvenance` `:1760-1798` — see mechanics below |
| `toolCallNamesUnder` | 1929-1940 | enginebridge | only consumer is `LastTurnOutput` |
| `acpDispatcher` (+PopulateContinue/Hook/Ask) | 1949-2022 | **enginebridge** (D-12) | |
| `patternNextPrompter` (interface) | 2026-2028 | **enginebridge** (D-12) | assertion target visibility noted in RESEARCH OQ4 |
| `triggerFromSignal` | 2035-2057 | **enginebridge** | only consumer is `acpDispatcher.Hook` |
| `hookSessionTurnRunner` / `hookSessionBoundaryOpener` | 2062-2073 / 2077-2088 | **enginebridge** (D-12) | constructed in `runOneTurn` `:916-917` — func-value boundary applies |
| `realCommandRunner` | 2092-2115 | **enginebridge** (D-12) | wired `:915` |
| `stubCatalogExec` / `stubExecResult` | 81-98 / 71 | **enginebridge** (D-12) | used by sessionFor `:1359` (engine-off path) — construction stays in core |
| `checkpointerAdapter` | `checkpoint.go:22-30` | **runtime core** (D-17 — beside its sessionFor use site) | |
| `redactorAdapter` | 123-135 | runtime core per RESEARCH OQ2 recommendation (sole consumer is sessionFor; D-07's letter would strand it behind an import cycle) | |
| cron: `defaultSchedTick`, `sessionTurnMu` `:34`, `clientTurnActive` `:43`, `markClientTurn` `:53`, `currentSessionID` `:62`, `startScheduler` `:73`, `fireDueAutomations` `:105`, `runAutomationTurn` `:132`, `runCatchUpOnce` `:209`, `startSessionForwarder` `:229` | `cron_wiring.go` full file | **runtime** — `internal/runtime/cron_wiring.go` (supersedes the original runtime/cron destination per AMENDED D-13/D-15) via the OQ1 mechanism — 4 of 9 are called from core (`Run`, `runOneTurn`, `sessionFor`) | |

---

## Pattern Assignments

### `internal/runtime` (service, event-driven + request-response)

**Verbatim analog:** `cmd/ass-guard/acp_serve.go` regions in the table above + `checkpoint.go:22-30`.
**Newly-written surface (only this):** `package doc` (D-20), `RunnerConfig`, `NewRunner`, rename `sessionTurnRunner`→`Runner`. Everything else is cut-and-paste + import fixes.

**Package doc house style (new code — copy the form from the strongest in-repo example)** — `internal/checkpoint/store.go:1-27`:

```go
// Package checkpoint implements the EARLY-01 shadow-git checkpoint store:
// ...
// Core invariants (all test-pinned):
//
//   - The USER's repository git state ... is NEVER touched ...
//   - A snapshot failure is loud but NEVER fatal to the turn ...
package checkpoint
```

D-20's kit-facing doc follows this form: purpose, downstream ambition (SEED-001 kit core at Phase 25), explicit non-goals (acpserve is NOT kit; no ACP words in API naming). Smaller precedent: `internal/loop/doc.go` shows the dedicated-`doc.go` file option.

**Core pattern — Run's cron-family coupling (OQ1 evidence, `acp_serve.go:757-772`):**

```go
sess := r.sessionFor(ctx, sessionID)
// ...
turnMu := r.sessionTurnMu(sessionID)
turnMu.Lock()
defer turnMu.Unlock()
// ...
r.markClientTurn(sessionID, true)
defer r.markClientTurn(sessionID, false)
```

`sessionTurnMu`/`markClientTurn` are defined in `cron_wiring.go` but called from core `Run` — whichever OQ1 mechanism the planner picks must keep these four call sites (`:763`, `:770`, `:772`, plus `startSessionForwarder` `:1389` / `runCatchUpOnce` `:1395` in sessionFor) compiling verbatim.

**Core pattern — sessionFor's adapter construction sites (`acp_serve.go:1150-1160`, `:1338-1339`):**

```go
st, cerr := checkpoint.Open(dir) //nolint:contextcheck // plan-pinned signature: Store.Open carries no ctx
// ...
mgr, err := session.NewManager(dir, sessionID, redactorAdapter{})
if err != nil {
    mgr, _ = session.NewManager(filepath.Join(os.TempDir(), "ass-guard"), sessionID, redactorAdapter{})
}
```

```go
if ckptStore != nil {
    s.Checkpointer = checkpointerAdapter{store: ckptStore}
}
```

These are why `redactorAdapter` (OQ2 rec: runtime-only) and `checkpointerAdapter` (D-17: runtime core) land beside sessionFor.

**White-box test pattern (D-01 — moved tests stay same-package because they fill unexported fields)** — `acp_serve_test.go:237-264` (`newExpansionRunner`, consumed by 7 files):

```go
func newExpansionRunner(
	t *testing.T, engineOn bool, script ...scriptedResp,
) (*sessionTurnRunner, *scriptedACPProvider) {
	t.Helper()
	// ...
	r := &sessionTurnRunner{
		bus:          bus,
		profile:      fakeProfileACP(),
		workDir:      dir,
		maxConc:      4,
		makeProvider: func(_ provider.RequestCapturer) provider.Provider { return prov },
	}
	if engineOn {
		err := r.setupEngine()
		// ...
	}
	r.loadCommandRegistry()
	return r, prov
}
```

After the move: `package runtime` (not `runtime_test`), `&Runner{...}`, `t.Helper()` convention preserved. `fakeProfileACP` lives at `acp_engine_e2e_test.go:123`; `writeOpsxCommandFixtures` at `acp_serve_test.go:207`.

---

### `internal/runtime/enginebridge` (adapter, transform)

**Verbatim analog:** the 7 adapter types + `setupEngine` + helpers per table above.
**No in-repo shape analog exists** for D-14's mirror-config seam — the planner writes the bridge struct/constructors fresh; RESEARCH.md "Pattern 2: Cross-package adapter construction" is the authoritative sketch.

**Why construction cannot move with the types (the decisive excerpt, `acp_serve.go:1772-1780`):**

```go
if a.r != nil {
    a.r.sessMu.Lock()

    if prov := a.r.automationProvenance; prov != "" {
        key = prov
    }

    a.r.sessMu.Unlock()
}
```

`engineTurnRunnerAdapter` reads the runner's *unexported* members (`invocationFor` `:1760`, `sessMu`/`automationProvenance` above, `expandUserBlocks` `:1798`). From a foreign package these names are invisible — so the type definitions move to enginebridge while their construction sites (`runOneTurn` `:915-917, :961-970`; `setupEngine` `:596-608`; `sessionFor` `:1359`) stay in package runtime, passing behavior as func values / exported constructor parameters.

**Construction-site excerpts the core must retain (`acp_serve.go:915-917`, `:961`):**

```go
r.hookExec.Commands = realCommandRunner{}
r.hookExec.Turns = &hookSessionTurnRunner{sess: sess}
r.hookExec.Boundaries = &hookSessionBoundaryOpener{mgr: sess.Manager}
```
```go
adapter := &engineTurnRunnerAdapter{sess: sess, mgr: sess.Manager, r: r}
```

**Dispatcher construction (moves-or-stays depends on the chosen seam; excerpt for reference, `acp_serve.go:596-608`):**

```go
r.eng.Dispatcher = &acpDispatcher{
	hooks:   r.hookExec,
	hookCfg: r.hookCfg,
	learned: r.learned,
	bus:     r.bus,
	nextPromptFor: func(patternID string) string {
		if np, ok := r.patternTable.(patternNextPrompter); ok {
			return np.NextPromptFor(patternID)
		}
		return ""
	},
}
```

---

### `internal/runtime/cron` (service, polling + forwarding)

> **SUPERSEDED (AMENDED D-13/D-15, operator sign-off 2026-08-26):** no `internal/runtime/cron` sub-package is created in Phase 15 — `cron_wiring.go` relocates byte-verbatim to `internal/runtime/cron_wiring.go` (package runtime), its nine methods staying beside their receiver; the interface-based cron seam is deferred to Phase 25 as design work. The patterns below describe the code itself, which moves unchanged.

**Verbatim analog:** `cron_wiring.go:1-258` in its entirety (9 methods + `defaultSchedTick` `:26`).
**No in-repo shape analog exists** for the embedded-state-owner mechanism — RESEARCH.md "Pattern 3" is authoritative; the planner locks the mechanism (OQ1) before writing tasks.

**Loop pattern with injectable tick + ctx-only lifecycle (`cron_wiring.go:73-101`):**

```go
func (r *sessionTurnRunner) startScheduler(ctx context.Context) {
	if r.schedule == nil {
		return
	}

	tick := r.schedTick
	if tick <= 0 {
		tick = defaultSchedTick
	}

	done := make(chan struct{})
	r.schedStop = func() { <-done } // tests may wait for a clean stop

	go func() {
		defer close(done)
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.fireDueAutomations(ctx)
			}
		}
	}()
}
```

**Firing pattern — claim-under-mutex + provenance bracket (`cron_wiring.go:139-180`, abridged):**

```go
claimed, ok := r.schedule.ClaimForFire(a.ID, r.schedule.Now())
if !ok {
	return
}
*a = claimed
// ...
mu := r.sessionTurnMu(sessionID)
queued := !mu.TryLock()
if queued {
	mu.Lock() // queue behind the active turn — serialized, never interrupts
}
defer mu.Unlock()
// ...
r.sessMu.Lock()
r.automationProvenance = "automation:" + a.ID
r.sessMu.Unlock()

stop, err := r.runOneTurn(ctx, sess, blocks)

r.sessMu.Lock()
r.automationProvenance = ""
r.sessMu.Unlock()
```

`runAutomationTurn` calls core's `runOneTurn`; `startSessionForwarder` reads `r.emitFor` (stays a RunnerConfig func field per D-16) and `clientTurnActive` state. Any mechanism must respect both directions of this core↔cron reach.

**Injection constraint (drives `cron.NewScheduler(store)`-style constructor):** `runACPServe` assigns `runner.schedule = scheduleStore` at `acp_serve.go:408` — an unexported field assignment dies across packages.

---

### `internal/acpserve` (composition-root, streaming)

**Verbatim analog:** `acp_serve.go:47-66` (const subset it uses), `:100-120` (`serveOptions`→exported `Options`, D-07), `:237-263` (`startAuditMirror`), `:267-311` (`resolveWorkDir`, `seedACPGuard`, `resolveProfilesDir` de-cobra'd), `:313-425` (`runACPServe`→`Run`). No sibling package plays this role today — cmd was the only composition site; the *content* is verbatim, only the package shell is new.

**THE startup-order invariant (`acp_serve.go:361-424`, abridged to the load-bearing skeleton — `Run` must reproduce this statement order line-for-line):**

```go
runner := &sessionTurnRunner{
	bus: bus, bodyStore: bodyStore, profile: prof,
	workDir: opts.WorkDir, maxConc: opts.MaxConcurrent,
	configAdded: opts.ConfigAddedBoundaries, askTimeout: opts.AskTimeout,
	serveCtx: ctx, schedCfg: schedCfg, providerName: providerName, stderr: stderr,
	makeProvider: func(capturer provider.RequestCapturer) provider.Provider {
		p, _ := factory.BuildWithCapturer(providerName, shaper.New(), capturer)
		return p
	},
}                                                                     // :361-382 → RunnerConfig + NewRunner (D-05/D-16)
runner.loadCommandRegistry()                                          // :386 — explicit call (thin ctor)
if opts.EngineEnabled { /* degrade, never crash */ }                  // :388-395
srv := acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner))      // :397
scheduleStore, schedErr := sched.Open(opts.WorkDir)                   // :403 — degrade loudly on failure
runner.schedule = scheduleStore                                       // :408 — dies across packages; see cron mechanism
runner.emitFor = srv.Emitter                                          // :411 — WINDOWS #3: AFTER NewServer
runner.startScheduler(ctx)                                            // :412 — BEFORE Serve, AFTER emitter
go func() { <-ctx.Done(); runner.closeAllSessions() }()               // :417-422 — MCP reap
return srv.Serve(ctx) //nolint:wrapcheck // direct delegation         // :424
```

Preceding fixed order (same function, `:321-359`): `event.NewBus()` → profile load → `setupModelRouting` → perm warnings (`globalConfigPath` + `projectConfigPath`) → body store → `startAuditMirror`. All become acpserve internals calling into the providerfactory package (D-08).

**Options struct (verbatim rename only, `acp_serve.go:107-120`)** — `serveOptions` becomes `acpserve.Options`; identical fields incl. the `AuditLogPath` override semantics.

**De-cobra adaptations (Pitfall 6 — the ONLY sanctioned non-verbatim edits):**
- `runACPServeCmd` (`:198-231`): caller extracts `auditPath, _ := cmd.Flags().GetString("audit-log")` (`:220`) in cmd; the moved function takes plain params. Whether `runACPServeCmd` itself stays as cmd glue or its body folds into `acpserve.Run` is planner discretion under D-09.
- `resolveProfilesDir` (`:298-311`): `cmd.Flags().Changed(flagProfilesDir)` (`:299`) becomes a `changed bool` param passed by cmd.

**Audit-mirror degradation pattern (`acp_serve.go:237-263`)** — switch over `AuditLogPath` (`""`/`"-"`/path), failures log-and-continue, `//nolint:staticcheck // QF1002` rides the switch — preserve the nolint.

**Interface seam (unchanged, plug-back proof)** — `internal/acp/server.go:31-42` + `:143`:

```go
type TurnRunner interface {
	Run(ctx context.Context, sessionID string, emit ChunkEmitter, prompt []ContentBlock) (stopReason string, err error)
}
type SessionCloser interface {
	CloseSession(sessionID string) error
}
// ...
func (s *Server) Emitter(sessionID string) ChunkEmitter { ... }
```

`runtime.Runner` implements both exactly as today (`Run` :753, `CloseSession` :1564); `WithTurnRunner(runner)` and the `emitFor = srv.Emitter` injection plug back identically.

---

### Five CLI-support packages (cli-logic, per D-08/D-09)

Each: cobra definitions stay in cmd; only the run*/resolve*/emit*/describe* logic moves. All excerpts below are the move targets in full.

**Provider-factory package (shared-infra — consumed by acpserve `:339,348,353`, main.go tracer `:129`, parity; one package, three importers).** Entire file `provider_factory.go` moves. Security-critical member that MUST survive the split (ASVS V14.1):

```go
// provider_factory.go:159-171
func warnLooseConfigPerm(path string, stderr io.Writer) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	perm := info.Mode().Perm()
	if perm&0o077 != 0 {
		_, _ = fmt.Fprintf(stderr,
			"ass-guard: %s is %04o (group/world-accessible) and may carry an api_key — chmod 0600 to protect it\n",
			path, perm)
	}
}
```

Degradation twin `setupModelRouting` (`:98-122`: operator-config failure → embedded default, never refusal) and the stat-gated overlay loader `loadModelRoutingFactory` (`:50-88`) move together.

**Checkpoint CLI package.** `checkpoint.go:97-146` + consts `:16/:33/:36` move; `newCheckpointCmd` `:43-95` stays. Stderr-discipline + structured-error pattern:

```go
// checkpoint.go:131-145 (abridged)
func runCheckpointRestore(ctx context.Context, stderr io.Writer, workDir, id string) error {
	store, err := checkpoint.Open(workDir) //nolint:contextcheck // plan-pinned signature: Store.Open carries no ctx
	if err != nil {
		return fmt.Errorf("checkpoint restore: %w", err)
	}
	err = store.Restore(ctx, id)
	if err != nil {
		return fmt.Errorf("checkpoint restore %q: %w", id, err)
	}
	_, _ = fmt.Fprintf(stderr, "restored %s — %s\n", id, checkpointRestoredNote)
	return nil
}
```

Note it consumes `resolveWorkDir` (moving to acpserve) at `:63,:78` — either cmd passes resolved dir (preferred) or the checkpoint package imports acpserve; planner decides.

**Learning CLI package.** `learning_cmd.go:53-134` (`resolveLearnedPath`, `runLearningList`, `runLearningRevert`). Pattern: stdout carries the machine-parseable table (`:79-88`), diagnostics to stderr — the sanctioned stdout exception family alongside modelrouting's `--json`.

**Model-routing CLI package.** `modelrouting.go:131-217` (`emitResolveHuman`, `emitResolveJSON`, `describeCapabilities`). Writer-parameterized already (`w io.Writer`) — zero cobra entanglement; cleanest of the five.

**Parity CLI package.** Seams ride along with their `//nolint:gochecknoglobals` (`parity.go:30, :44`):

```go
var zcodeInstalledVersion = func() (string, error) { //nolint:gochecknoglobals // test-injectable seam
	ctx, cancel := context.WithTimeout(context.Background(), zcodeVersionTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "zcode", "--version").Output()
	// ...
}
var parityRun = parity.Run //nolint:gochecknoglobals // test-injectable seam
```

Read `:80-376` before planning — the run-logic functions below `:80` were inventoried but not fully read this session (research read `:1-60`; treat exact sub-boundaries as a plan-time read).

**Profile-check CLI package (OQ5 — absent from D-08's letter; recommended same treatment).** `profile_check.go:55-236` (`runProfileCheck`, `loadCaptureLine`, `extractCaptureCounts`, `reportProfileCheck`, `readFirstLine` + const `readBufSize` `:15`). Uses `os.Stderr` directly (`:89, :191-205`), unlike the writer-parameterized peers — move verbatim regardless.

---

### Modified `cmd/ass-guard/*` (route, D-09)

**main.go wiring (stays, `:77-82`)** — untouched shape; only import paths of extracted packages change if cmd still references them:

```go
root.AddCommand(newProfileCmd())
root.AddCommand(newParityCmd())
root.AddCommand(newACPCmd())
root.AddCommand(newModelRoutingCmd())
root.AddCommand(newLearningCmd())
root.AddCommand(newCheckpointCmd())
```

**Cobra shells retained in cmd:** `newACPCmd`/`newACPServeCmd` (`acp_serve.go:139-192`), `newCheckpointCmd`, `newLearningCmd`, `newModelRoutingCmd`(+Validate/Resolve builders), `newParityCmd`, `newProfileCheckCmd`/`newProfileCmd`. Their RunE bodies thin down to calls into the new packages.

**goconst_constants.go dissolved (D-10).** Source (`cmd/ass-guard/goconst_constants.go:1-16`):

```go
const blockText = "text"
const stopEndTurn = "end_turn"
const protocolVersion20 = "2.0"
const keySessionID = "sessionId"
const keyType = "type"
const tierHeavy = "heavy"
const tierLight = "light"
const profileZcode = "zcode"
// ...
const chunkDone = "done"
```

Duplication precedent already in-repo (`internal/acp/goconst_constants.go:1-15` repeats `blockText`, `stopEndTurn`, `protocolVersion20`, `keySessionID` verbatim):

```go
package acp

// Repeated string literals extracted to constants (goconst).
const blockText = "text"
const stopEndTurn = "end_turn"
// ...
```

Rule: copy the header-comment style; each new package gets the constants its moved code references; `golangci-lint` goconst (enabled via `default: all`) catches misses. Usage density warning: 46 refs in acp_serve_test.go, 42 ask_wiring, 30 acp_engine_e2e — the test destinations need their subsets too.

---

## Shared Patterns

### SP-1: Verbatim-move method (governs every task)
Hand cut-and-paste + import fixes + gofmt (`mise fmt`); `git mv` for whole-file moves (`cron_wiring.go`, and the CLI files' test companions). No AST tooling, no drive-by cleanup. Review attestation: `git diff --color-moved=plain master -- cmd/ internal/`.

### SP-2: nolint directives are line-anchored cargo
36 `//nolint:` comments ride lines in `acp_serve.go`, 4 in `cron_wiring.go` (funcorder ×13 group-comments, contextcheck, wrapcheck ×5, funlen, maintidx, containedctx, cyclop, lll, staticcheck, modernize, gocritic, forcetypeassert ×3). Every one moves WITH its line. Preserve relative declaration order within each destination file — `funcorder` evaluates order per-new-file (RESEARCH A2). Treat new lint firings as placement bugs, never delete moved nolints.

### SP-3: Graceful-degradation log line shape
Recurring startup idiom — keep byte-identical:
```go
log.Printf("ass-guard: <thing> failed (continuing without <thing>): %v", err)
```
Instances: engine setup (`:393`), learning store (`:588`), command registry (`:637`), openspec boundaries (`:651`), audit mirror (`:247`), checkpoint store (`:1154`), scheduling config (`provider_factory.go:103`), global config layer (`:60`).

### SP-4: Transport discipline
stdout carries ACP frames only. All diagnostics via `stderr io.Writer` fields or `fmt.Fprintf(stderr, ...)` / `log.SetOutput(os.Stderr)` (`:203`). Unchecked stderr writes use `_, _ =` prefix. The two sanctioned stdout exceptions (learning table, modelrouting `--json`) move as-is with their writers parameterized.

### SP-5: Error-wrapping conventions
- Delegation returns carry `//nolint:wrapcheck // <x> delegation` (`:909`, `:1580`, `:1813`, `:2072`, `:2087`, `:424`).
- Generic wraps use the repo's `fmt.Errorf("call: %w", err)` idiom (`:325`, `:341`, `:557`, `:560`, `:567`, `:1007`, `:109`) or domain-prefixed wraps (`"checkpoint restore: %w"`).

### SP-6: Same-package white-box test relocation (D-01)
All 21 runner-literal sites across 10 files construct `&sessionTurnRunner{unexported fields}` — external test packages are impossible. Destinations declare the package they test (`package runtime`). Verified literal counts per file: acp_engine_e2e 3 · acp_serve_test 5 · advisory 1 · ask 2 · checkpoint 1 · e2e_opsx_matrix 1 · e2e_opsx 1 · integration 1 · mcp_tracer 1 · planmode 1.

### SP-7: Test-helper duplication graph (D-03 — duplicate, never extract)
| Helper | Defined at | Consumed by | Duplicate into |
|---|---|---|---|
| `newExpansionRunner` | acp_serve_test.go:237 | advisory, ask, checkpoint, coreexec, subagent_tier, acp_serve suites | runtime tests |
| `scriptedACPProvider`/`scriptedResp` | acp_engine_e2e_test.go:27-33 | 7 files (23 refs in acp_serve_test alone) | every runtime-test destination using them |
| `noopEmitter` | acp_engine_e2e_test.go:118+ | chunk assertions | runtime tests |
| `writeOpsxCommandFixtures` | acp_serve_test.go:207 | expansion suites | runtime tests |
| `fakeProfileACP` | acp_engine_e2e_test.go:123 | expansion/engine suites | runtime tests |
| `writeTestModelRouting`/`pinEmptyHome` | provider_factory_test.go:114/:130 | subagent_tier suite | providerfactory tests AND runtime tests (cross-package dup required) |
| `newOpsxRunnerAt`/`opsxRunnerSeam` | e2e_opsx_test.go:123/:277 | evalsuite_bridge_test.go:51/:85/:154 | follow the opsx tests wherever they land (OQ3) |

### SP-8: TestMain uniqueness (Pitfall 5)
`mcp_tracer_test.go:35` owns the package's only `TestMain` (echo-server self-exec mode gated on `os.Args[1] == tracerEchoArg`). Move the file as a unit; assert exactly one `TestMain` per destination package post-split.

### SP-9: Env-gated skips travel with their files
`ASSGUARD_EVAL_GATE`, `ASSGUARD_OPENSPEC_BIN`, `ASSGUARD_E2E_LLM`, `ASSGUARD_CHECKPOINT_E2E`, `ZAI_API_KEY`, `ASSGUARD_E2E_CAPTURE` — names unchanged, gates ride their test files.

### SP-10: Lint shield loss on exit from cmd (Pitfall 3)
`.golangci.yml:112-115` grants errcheck relief `(fmt\.Println|os\.Exit|log\.Fatal)` only under `path: "cmd/.*"`. Files leaving cmd lose it under `default: all`. Survey shows low exposure (`_, _ =` prefixes dominate), but run full `mise lint` per wave. Test-scoped relaxations (`.*_test\.go`) travel safely.

### SP-11: Verification gate = equivalence proof (RUNT-01)
- Per task: `go build ./... && go vet ./... && go test ./internal/runtime/... ./internal/acpserve/... ./cmd/ass-guard/ -count=1`
- Per wave boundary: `mise ci` (vet → `golangci-lint run` → `CGO_ENABLED=0 go build ./...` → `go test -race -count=1 ./...`)
- Baseline ledger: 130 `^func Test` functions in cmd today (per-file counts verified: acp_serve_test 27, e2e_opsx_matrix 14, ask 10, acp_engine_e2e 8, background 6, checkpoint 6, coreexec 6, provider_factory 8, cron_wiring 5, mcp_tracer 5, modelrouting 5, parity 5, acp_serve others… sum = 130). Post-move sum across all packages must equal it.

### SP-12: Compile closure (Pitfall 10)
The carve wave (runtime family + acpserve + test relocation) is ONE compile closure — cmd references the moved symbols and vice versa; no intermediate green tree exists without shims. Progress metric mid-wave: `go build ./internal/...` + targeted package tests. The five CLI-support extractions compile independently per file (true wave 2).

---

## No Analog Found

Freshly *written* surfaces with no in-repo precedent — planner uses RESEARCH.md patterns, not codebase copying:

| Surface | Role | Data Flow | Reason | Use Instead |
|---------|------|-----------|--------|-------------|
| `RunnerConfig` + `NewRunner` thin ctor (D-05) | config + factory | — | no exported-options-struct constructor exists in any internal package yet (all are `New<T>(deps...)` positional) | RESEARCH.md D-05: mirror today's literal field-for-field; struct-fill only |
| enginebridge mirror-config struct + constructors (D-14) | adapter seam | — | no cross-package adapter handoff exists (adapters were always same-package) | RESEARCH.md Pattern 2 sketch |
| cron embedded state-owner / scheduler injection (OQ1) | service seam | — | no embedded-owner precedent; `runner.schedule =` assignment has no cross-package successor | RESEARCH.md Pattern 3 + `cron.NewScheduler(store)` constructor shape |
| `internal/acpserve` as a package | composition-root | — | cmd was the only composition site; contents verbatim, shell is new | acp_serve.go:320-425 order skeleton (above) |
| CLI-contract golden test (recommended Wave 0 add) | test | — | no golden-list precedent in cmd tests | RESEARCH.md Validation Architecture (~80-line spec) |

---

## Metadata

**Analog search scope:** `cmd/ass-guard/*.go` (all 32 files enumerated), `internal/**` directory survey (41 packages), `.golangci.yml`, `.mise.toml`, `AGENTS.md` (276 ln, project constraints), spot reads in `internal/{loop,checkpoint,sched,acp}`.

**Files read in full this session:** acp_serve.go (2115 ln), cron_wiring.go, checkpoint.go, learning_cmd.go, modelrouting.go, provider_factory.go, profile_check.go, main.go, goconst_constants.go (×2), internal/acp/server.go, .golangci.yml, .mise.toml. Partial: parity.go (:1-80 — seams verified; run logic below is plan-time read), test files via targeted excerpts (headers, helper constructors, TestMain).

**Verified counts:** 36 nolints (acp_serve.go) + 4 (cron_wiring.go); 21 runner literals in 10 test files; 130 test functions total; 7 adapter types; 9 cron methods; helper-graph edges per SP-7.

**Pattern extraction date:** 2026-08-26
