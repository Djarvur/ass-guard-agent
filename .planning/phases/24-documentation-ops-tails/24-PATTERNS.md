# Phase 24: Documentation & Ops Tails - Pattern Map

**Mapped:** 2026-08-28
**Files analyzed:** 13 (8 new, 5 modified)
**Analogs found:** 12 / 13 (the first GitHub Actions workflow has no in-repo analog; RESEARCH.md Pattern 3 is its source)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/modelrouting/outcomes.go` | model (durable store) | file-I/O (append-only JSONL) | `internal/session/transcript.go` + `internal/sched/sched.go` | exact (conventions) |
| `internal/modelrouting/outcomes_agg.go` | service (aggregation → feedback) | transform | `internal/modelrouting/dispatch.go` (seams) + `breaker.go`/`cost.go` | exact (in-package) |
| `internal/modelrouting/outcomes_test.go` | test | batch | `internal/modelrouting/dispatch_test.go`, `internal/sched/sched_test.go` | exact |
| `internal/modelrouting/dispatch.go` (mod) | service (record hook) | event-driven (per dispatch) | itself — outcome learn-points at lines 250-272 | exact |
| `internal/modelroutingcmd/modelrouting.go` (mod) | utility (renderers) | transform | itself — `EmitResolveHuman`/`EmitResolveJSON` | exact |
| `cmd/ass-guard/modelrouting.go` (mod) | controller (cobra shell) | request-response | itself — `newModelRoutingResolveCmd` | exact |
| Drift-core Go command (e.g. `internal/profilecheckcmd` ext + `cmd/ass-guard` shell) | service + controller | request-response (probe → compare) | `cmd/ass-guard/profile_check.go` + `internal/paritycli/parity.go` | exact |
| `.github/workflows/nightly-parity.yml` | config (CI workflow) | event-driven (cron schedule) | **none in repo** (first workflow; `.github/` absent) | none → RESEARCH.md Pattern 3 |
| `.mise.toml` (mod, optional drift task) | config (task entry) | batch | itself — `eval-gate`/`emitter-soak` task shape | exact |
| TAIL-03 E2E harness tests (new package, planner picks location) | test (E2E harness) | event-driven (scripted turns) | `internal/acpserve/simulator_e2e_test.go` | exact (interactive leg), role-match (other 3 modes) |
| `internal/ecosys/testdata/` fixture plugin/skill | test fixture | static | `internal/ecosys/testdata/plugins-installed/cache/acme-market/skill-plugin/1.0.0/` | exact |
| `docs/lsp-setup.md` | documentation | static | `docs/install.md` | exact |
| `README.md` (mod — docs index) | documentation | static | itself — lines 100-104 "Docs:" index | exact |

## Pattern Assignments

### `internal/modelrouting/outcomes.go` (model/store, file-I/O append)

**Analog:** `internal/session/transcript.go` (append-only JSONL) + `internal/sched/sched.go` (`.ass-guard/` house conventions, atomic persistence, corrupt-file quarantine)

**Append-only JSONL open pattern** (`internal/session/transcript.go` lines 158-191):
```go
// selfGitignoreContent is the .ass-guard/.gitignore body (D-07): ignore
// everything except .gitignore itself.
const selfGitignoreContent = "*\n!.gitignore\n"

// openTranscript ensures dir/.ass-guard exists (with a self-gitignore), then
// opens the per-session JSONL file O_APPEND|O_CREATE|O_WRONLY mode 0600.
func openTranscript(dir, sessionID string) (*os.File, string, error) {
	storeDir := filepath.Join(dir, ".ass-guard")
	err := os.MkdirAll(storeDir, dirPerm)
	...
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePermOwner)
```
(A second in-tree instance: `internal/audit/audit.go:55` uses the identical `os.O_CREATE|os.O_WRONLY|os.O_APPEND` + `filePermOwnerOnly` triple.)

**Store layout + record-unit conventions** (`internal/sched/sched.go` lines 22-30, 55-111):
```go
// Store layout constants (the .ass-guard artifact family; EnsureGitignore's
// `*` rule already covers the schedule/ subdir — verified 12-07).
const (
	storeDirName  = ".ass-guard"
	scheduleDir   = "schedule"
	storeFileName = "schedules.json"
	filePerm      = 0o600
	dirPerm       = 0o750
)

// Automation is one persisted scheduled prompt (the store's record unit).
type Automation struct {
	ID     string `json:"id"`
	...
}
```

**Load-tolerant open + quarantine** (`internal/sched/sched.go` lines 113-163): `Open` does `os.MkdirAll(dir, dirPerm)`, returns a fresh store on `os.IsNotExist`, and on unmarshal failure renames the corrupt file aside as `<path>.corrupt-<unix-ts>` and persists a fresh store — "graceful degradation, never a crash". The outcome store's JSONL reader must tolerate unknown kinds/fields per the 16-D-20 additive discipline (skip the line, never fail the read).

**Git-exclusion precedent** (`internal/ecosys/loader.go` lines 1301-1332): `EnsureGitignore(assguardDir)` is idempotent, writes `gitignoreContent = "*\n!.gitignore\n"` (line 21) to `<assguardDir>/.gitignore`, refuses `.claude/` paths. The store subdirectory under `.ass-guard/` is already covered by that `*` rule — do NOT write a new exclusion mechanism (RESEARCH.md "Don't Hand-Roll").

---

### `internal/modelrouting/outcomes_agg.go` (service, transform)

**Analog:** `internal/modelrouting/dispatch.go` — the aggregation feeds ONLY the seams this file defines; `breaker.go`/`cost.go` are the concrete implementations it seeds.

**The two feedback seams** (`internal/modelrouting/dispatch.go` lines 25-40):
```go
// Breaker is the per-(provider,model) circuit-breaker seam.
type Breaker interface {
	Allow(now time.Time) bool
	RecordSuccess()
	RecordTransient(now time.Time, err *provider.ProviderError)
}

// CostTracker is the cost-ceiling seam.
type CostTracker interface {
	Check(now time.Time) CostAction
	Account(model string, inTokens, outTokens int)
}
```

**The injection joints** (`internal/modelrouting/dispatch.go` lines 107-127):
```go
// SetBreakers injects a real breakers map (Plan 03-03). The map is keyed by
// (provider, model); candidates whose key is absent fall back to a no-op
// breaker (allowed).
func (s *Scheduler) SetBreakers(b map[providerModelKey]Breaker) { ... }

// SetCostTracker injects a real cost tracker (Plan 03-03).
func (s *Scheduler) SetCostTracker(c CostTracker) { ... }
```
Keyed by `providerModelKey{Provider, Model}` (lines 56-60). Aggregation output seeds these maps at construction (Pitfall 8: in-memory breakers reset per process — the JSONL store is the durable memory).

**Where outcomes are learned in Dispatch** (`internal/modelrouting/dispatch.go` lines 250-272) — the template for the record hook:
```go
if err == nil {
	b.RecordSuccess()
	s.cost.Account(cand.Model, 0, 0) // Response has no token fields on the Send path
	return resp, nil
}
perr := asProviderError(err, &cand)
if perr.Kind == provider.KindStructural {
	// Report, no retry (D-04). Structural errors do not feed the breaker
	return provider.Response{}, perr
}
// Transient: feed the breaker, emit fallback event, walk on.
b.RecordTransient(s.now(), perr)
s.cost.Account(cand.Model, 0, 0)
```
Outcome-class mapping for the store: `err == nil` → `ok`; `KindStructural`/`KindExhausted` → respective error class (terminal, no breaker feed); transient → `transient` + `fallback-used` when `i < len(candidates)-1` (the `ProviderFallback` publish at lines 274-287 carries `From/To Provider/Model` — mirror those fields).

**D-07 deterministic test shape:** construct Scheduler via `NewScheduler(...)` + `SetBreakers`/`SetCostTracker` seeded from aggregation over a synthetic JSONL file + `SetNow` (lines 129-134, injectable clock) → assert `Allow` returns false (breaker open) or `Check` returns `CostDegrade`/`CostHardStop` (lines 45-54: `CostAllow`/`CostDegrade`/`CostHardStop`). Follow `dispatch_test.go`/`breaker_test.go` conventions (stdlib testing, no testify, injected clock).

**CRITICAL (Pitfall 1, A3):** `Scheduler.Dispatch` has ZERO production callers — the live path is `modelrouting.NewResolver(cfg).Resolve(...)` at `internal/runtime/runtime.go:1316` and `internal/acpserve/config_surface.go:321`. The planner must add an explicit wiring task: record at the real Send/Resolve sites, and make the D-07 observable run against the seam the live path actually consults.

---

### `internal/modelroutingcmd/modelrouting.go` (mod — EmitStats renderers)

**Analog:** itself — copy the `EmitResolveHuman`/`EmitResolveJSON` pair verbatim in shape.

**Human renderer → stderr** (`internal/modelroutingcmd/modelrouting.go` lines 12-40):
```go
// EmitResolveHuman writes the human-readable resolution to the STDERR writer
// (transport discipline — stdout stays byte-clean unless --json). In production
// cobra wires this to os.Stderr; tests redirect via SetErr.
func EmitResolveHuman(w io.Writer, tier, project string, primary *modelrouting.Target, ...) {
	_, _ = fmt.Fprintf(w, "%s [%s] -> %s/%s\n", tier, proj, primary.Provider, primary.Model)
	...
}
```

**JSON renderer → stdout** (lines 42-78): anonymous struct with json tags, `json.NewEncoder(w)`, `enc.SetIndent("", "  ")`, `//nolint:wrapcheck` on `enc.Encode`. `EmitStats(w io.Writer, agg ...)` follows the same signature discipline: `io.Writer` first, error return for the JSON variant, ignore write errors on the human variant.

---

### `cmd/ass-guard/modelrouting.go` (mod — `scheduling stats` shell)

**Analog:** itself — `newModelRoutingResolveCmd` (lines 64-127) is the exact template.

**Cobra shell pattern** (lines 69-127):
```go
func newModelRoutingResolveCmd() *cobra.Command {
	var (
		configPath string
		...
		asJSON     bool
	)
	cmd := &cobra.Command{
		Use:          "resolve",
		Short:        "resolve a tier to a concrete (provider, model) at a given time",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			...
			if asJSON {
				return modelroutingcmd.EmitResolveJSON(cmd.OutOrStdout(), ...)
			}
			modelroutingcmd.EmitResolveHuman(cmd.ErrOrStderr(), ...)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON to stdout (default: human form to stderr)")
	...
}
```
Register via `cmd.AddCommand(...)` in `newModelRoutingCmd` (lines 18-27). Note the group is `Use: "model-routing"` (README line 81 documents `ass-guard model-routing validate | resolve`). Render logic lives in `internal/modelroutingcmd`, never inline in the shell (Phase-15 CLI-support split). Existing wire-discipline test to extend: `cmd/ass-guard/modelrouting_test.go`.

---

### Drift-core Go command for the nightly (TAIL-02)

**Analog:** `cmd/ass-guard/profile_check.go` (shell) + `internal/paritycli/parity.go` (test-injectable seams).

**Thin cobra shell over internal logic** (`cmd/ass-guard/profile_check.go` lines 12-34):
```go
cmd := &cobra.Command{
	Use:          "check <name>",
	Short:        "diff a fresh capture against the profile's tiered manifest (PROF-04 drift detector)",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return profilecheckcmd.RunProfileCheck(args[0], profilesDir, captureFile)
	},
}
cmd.Flags().StringVar(&captureFile, "capture-file", "", "fixture JSON capture ... — bypasses the live path")
```
The `--capture-file` flag pattern is exactly the nightly's need: a fixture/offline path that is unit-testable and a live path for the workflow.

**Test-injectable seam var** (`internal/paritycli/parity.go` lines 20-43):
```go
// zcodeInstalledVersion resolves the INSTALLED zcode's version: a fixed-argv
// exec of `zcode --version` ... under a bounded timeout, output trimmed. It is
// a package-level seam var so tests inject a fake installed version without a
// live binary (offline CI); the default is the real exec.
var zcodeInstalledVersion = func() (string, error) { //nolint:gochecknoglobals // test-injectable seam
	ctx, cancel := context.WithTimeout(context.Background(), zcodeVersionTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "zcode", "--version").Output()
	...
}
```
Reuse this seam; do not fork it. Drift semantics come from `internal/drift/drift.go` (lines 11-24): `Drift{FieldPath, Tier, Reason}`; `Detect(manifest, captured)` returns TIER-1 (count-based) / TIER-2 (presence/count) drifts. The pinned bundle lives at `profiles/zcode/` (`profile.yaml`, `tools.json`, `thinking.json`, `tool_choice.json`, `coverage.yaml`, `identity.yaml`, `meta.yaml`). The nightly job wraps: version probe (seam) + sha256 structure hash over a canonicalized serialization of the pinned bundle (stdlib `crypto/sha256`; A4 — no existing hash implementation) + `mise ci`.

---

### `.github/workflows/nightly-parity.yml` (config, cron event-driven)

**Analog:** NONE — `.github/` does not exist in the repo; this is the first workflow. **Use RESEARCH.md Pattern 3** (shape repeated here):

```yaml
on:
  schedule:
    - cron: "23 4 * * *"   # off-peak odd minute — NOT :00/:30 (documented delay/drop)
  workflow_dispatch: {}     # manual smoke — the local-validation substitute (no `act`)
permissions:
  contents: read
  issues: write             # ONLY where the drift-issue job needs it (Pitfall 6: default GITHUB_TOKEN is read-only)
jobs:
  build-test:   # hosted runner: checkout → setup-go (go-version-file: go.mod) → mise ci
  drift:        # runs-on: [self-hosted, <label>] — zcode corpus stays local (D-08; ALL labels must match)
    steps: probe → compare vs pinned → upload-artifact (always) → if drift: github-script issue
```

Pin first-party actions at plan time (research verified 2026-08-28: `actions/checkout` v7.0.1, `actions/setup-go` v7.0.0, `actions/github-script` v9.0.0, `actions/upload-artifact` v7.0.1). Issue body via `github.rest.issues.create` object arg or `gh --body-file` — never interpolate drift content into shell args (Pitfall 10). Scheduled workflows fire ONLY when the file exists on the default branch `master` (Pitfall 2) — verification must note the schedule activates at milestone merge; `workflow_dispatch` keeps the drift job triggerable meanwhile.

**In-repo entry points the workflow invokes** (`.mise.toml` lines 25-34): `mise ci` (`depends = ["vet", "lint", "build", "test"]`) and the gate-flag precedent `ASSGUARD_EVAL_GATE=1 ... go test ...` — the nightly must NOT touch that machinery (D-09). A drift task, if added, copies the `[tasks.eval-gate]` shape (comment-annotated, env-var parameterized).

---

### TAIL-03 E2E mode-matrix harness (test, event-driven)

**Analog:** `internal/acpserve/simulator_e2e_test.go` — the 16-06 Zed simulator.

**Header discipline** (lines 25-36):
```go
// The 16-06 Zed-client simulator (ACP-03/ACP-08 whole-phase story): a small
// scripted client speaking newline-delimited JSON-RPC over io.Pipe drives the
// REAL acpserve.Run composition end-to-end — initialize → session/new →
// prompt streaming → set_config_option → cancel — against a scripted SSE
// provider stub. ...
// Determinism: fixed script, guard timeouts on every wait (no open-ended
// sleeps), sequential stages over ONE serve instance, string request ids
// mirroring Zed's UUID style (D-15).
```

**Real-composition harness skeleton** (`TestZedSimulatorE2E`, lines 359-420):
```go
t.Setenv("ZAI_API_KEY", "") // deterministic provider construction
stub := newSimStub(simTurnScripts(stubPlaceholder))
workDir := simulatorWorkDir(t, stubPlaceholder)
simRepointConfig(t, workDir, stubPlaceholder, stub.srv.URL)   // rewrite .ass-guard/config.yaml at 0600
srvInR, srvInW := io.Pipe(); cliR, cliOutW := io.Pipe()
go func() { serveDone <- Run(ctx, srvInR, cliOutW, stderr, &Options{...WorkDir: workDir}) }()
t.Cleanup(func() { cancel(); close pipes; <-serveDone with simGuardTimeout guard })
cli := newSimClient(t, cliR, srvInW)
simStageInitialize(t, cli)   // staged assertions, each a t.Helper() func
```
Const vocabulary at lines 39-56 (`simMethodSessionUpdate`, `simKindToolCall`, `simGuardTimeout = 10 * time.Second`) — reuse the guard-timeout and string-request-id disciplines for the new per-mode harness. The four modes: interactive (this file IS the template), subagent dispatch, background wake-turn, automation/cron turn (cron substrate: `internal/sched` — never re-implement cron parsing).

**Functional assertion bar (D-12/D-13):** assert a command RAN, a skill body EXPANDED, a hook FIRED with observed effect — the hook event vocabulary is fixed ("never renamed") at `internal/ecosys/hooks.go` lines 54-66: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, `SubagentStop`, `SessionStart`, `SessionEnd`. Per-cell preconditions (Pitfall 9): Phase 20 (commands/skills resolver chain), 21 (hook authority/merge), 22 (wake-turn), 18 (cron discipline) — mark cells whose substrate phase has not executed.

---

### `internal/ecosys/testdata/` fixture plugin/skill (test fixture, static)

**Analog:** `internal/ecosys/testdata/plugins-installed/cache/acme-market/skill-plugin/1.0.0/` — copy this directory shape for the synthetic fixture (D-14):

```
.claude-plugin/plugin.json    {"name": "...", "description": "...", "version": "1.0.0"}
commands/<name>.md            frontmatter `description:` + body with $ARGUMENTS
skills/<name>/SKILL.md        frontmatter `name:` + `description:` + body
hooks/hooks.json              {"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "echo ...", "timeout": 5}]}], ...}}
agents/<name>.md              fixture agent
.mcp.json                     {"mcpServers": {"<name>": {"command": "/bin/echo", "args": [...]}}}  (deterministic, no network)
```
Fixture hook commands are deterministic `/bin/echo`-family — keep the synthetic fixture network-free and hermetic. The one REAL installed Claude Code plugin spot-check (D-14) is an operator-environment concern, not a repo fixture. Discovery surfaces: `ecosys.Load` → `loadAll` over project + user `.claude/` and `.ass-guard/` trees with two-phase precedence (`internal/ecosys/loader.go` lines 39-66, 118); read-only `.claude/` discipline applies to fixtures.

---

### `docs/lsp-setup.md` (documentation, static)

**Analog:** `docs/install.md` — copy its structure.

**Structural pattern** (whole file, 133 lines): `# <Verb-phrase title>` → one-paragraph positioning statement (what this covers, explicit scope) → `## Quick start` with numbered `### N. <step>` subsections, each ending in a verifiable command (`ass-guard --version  # sanity check (prints to stderr)`) → config tables (`| OS | Arch | Archive |` at lines 16-21) → fenced `sh`/config blocks → honest-status prose ("Known rough edges" style: state what does NOT work and why, as install.md lines 81-89 do for non-clobbering/first-run) → `## See also` with relative links back into the repo (lines 124-133).

**Content anchors verified in-repo for the doc's three legs:**
- ass-guard loads project `.mcp.json` (camelCase `mcpServers`) merged with ecosys-discovered plugin servers (`internal/mcp/host.go:75-89`; `internal/runtime/runtime.go` spawnMCP) — the dry-run leg (D-03) configures the LSP-capable server HERE.
- ass-guard parses but IGNORES ACP-forwarded `mcpServers`: `internal/acp/handlers.go:253-261` (field at :256, `_ = json.Unmarshal` discard at :260) — the doc must state this as the agent-side-OUT boundary.
- Zed side: `context_servers` settings key (local `command/args/env`, remote `url/headers`), native built-in `diagnostics` tool, zed#52449 extension gap — per RESEARCH.md DOC-01 Deep Dive table; flag A1 for user confirmation of the worked-example server.

**README index link** — `README.md` lines 100-104 is the docs index to extend:
```markdown
Docs: [install](docs/install.md) ·
[recapture runbook](docs/recapture-runbook.md) ·
[tool contract inventory](docs/tool-contract-inventory.md) · ...
```
Add `[lsp setup](docs/lsp-setup.md)` in the same mid-sentence `·`-separated style (D-02: alongside install.md, not inside it).

---

## Shared Patterns

### `.ass-guard/` durable-state house conventions
**Source:** `internal/sched/sched.go:24-30` (`filePerm 0o600`, `dirPerm 0o750`), `internal/session/transcript.go:158-191` (self-gitignore + O_APPEND), `internal/ecosys/loader.go:1301-1332` (`EnsureGitignore`)
**Apply to:** `outcomes.go` — store under `.ass-guard/`, owner-only perms, covered by the existing self-gitignore `*` rule, quarantine-or-skip on corrupt input, never crash the turn on store I/O failure (graceful degradation + loud stderr note).

### CLI transport discipline (stderr default, `--json` → stdout only)
**Source:** `internal/modelroutingcmd/modelrouting.go:12-14,42-45`; `cmd/ass-guard/modelrouting.go:13-17,110-116`
**Apply to:** the `scheduling stats` subcommand AND any drift-core command output; human → stderr always, stdout byte-clean unless `--json` explicitly requested.

### Cobra shell / internal-logic split with test-injectable seams
**Source:** `cmd/ass-guard/profile_check.go:12-34` + `internal/paritycli/parity.go:24-43` (package-level `var ... //nolint:gochecknoglobals // test-injectable seam`)
**Apply to:** drift-core command and stats command — cobra shells in `cmd/ass-guard/`, run logic + render in `internal/*cmd`, every external touch (exec, clock, filesystem root) a seam var for offline tests.

### Outcome-class ↔ breaker/cost seam mapping
**Source:** `internal/modelrouting/dispatch.go:250-272` (`RecordSuccess` / `RecordTransient` / structural-terminates) + `CostAction` enum at lines 45-54
**Apply to:** `outcomes.go` record schema and `outcomes_agg.go` aggregation — outcome classes must map 1:1 onto what the seams consume; structural errors never open breakers.

### Scripted deterministic E2E with guard timeouts
**Source:** `internal/acpserve/simulator_e2e_test.go:25-56,359-420`
**Apply to:** TAIL-03 harness — real composition, fixed scripts, `simGuardTimeout`-style guards on every wait, staged `t.Helper()` assertion functions, `t.Setenv` for determinism.

### Stdlib testing, table-driven, injected clock
**Source:** `internal/modelrouting/dispatch_test.go`, `breaker_test.go`; `internal/sched/sched.go:109-111,166-175` (`now func() time.Time` injectable)
**Apply to:** `outcomes_test.go` and the D-07 feedback test — no testify anywhere; pin determinism via `SetNow`.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `.github/workflows/nightly-parity.yml` | config (CI workflow) | event-driven (cron) | First workflow in the repo — `.github/` does not exist. Use RESEARCH.md Pattern 3 + official docs citations therein (off-peak cron, default-branch rule, `runs-on` label array, per-job `permissions`). |

## Metadata

**Analog search scope:** `internal/modelrouting/`, `internal/modelroutingcmd/`, `cmd/ass-guard/`, `internal/sched/`, `internal/session/`, `internal/audit/`, `internal/ecosys/` (+ `testdata/`), `internal/paritycli/`, `internal/drift/`, `internal/acpserve/`, `docs/`, repo root (`README.md`, `.mise.toml`, `.github/` absence check)
**Files scanned:** ~25 (12 read in full or targeted, remainder grepped/verified)
**Pattern extraction date:** 2026-08-28
