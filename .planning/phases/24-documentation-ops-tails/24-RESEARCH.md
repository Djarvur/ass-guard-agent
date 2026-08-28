# Phase 24: Documentation & Ops Tails - Research

**Researched:** 2026-08-28
**Domain:** Go ops tooling (persistent outcome store + CLI), GitHub Actions scheduled CI, Zed IDE-side MCP/LSP documentation, E2E interaction-mode verification harness
**Confidence:** HIGH (repo seams read this session; external claims cited to official docs; one operator-premise gap flagged in the Assumptions Log)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**LSP Documentation (DOC-01)**
- **D-01:** The guide's worked example is ZED's IDE-side MCP providing LSP capabilities (operator: "we know there is an mcp inside zed providing lsp capabilities. we should provide zed example"). The generic pattern for any LSP-providing MCP server is secondary. The exact Zed feature surface (which LSP capabilities it exposes to agents) is pinned at research time. Scope statement included: agent-side LSP explicitly OUT (the 2026-08-25 operator decision, stated in the doc itself).
- **D-02:** The guide lives in `docs/` as its own file (`docs/lsp-setup.md` family), linked from the README docs index — alongside `docs/install.md`, not inside it.
- **D-03:** Success criterion 1 verification = FOLLOWABLE DRY-RUN: the doc's author follows it top-to-bottom during the phase and reaches working LSP-symbol tools in an ass-guard session. Review-only accuracy does not satisfy the criterion.

**Scheduler Outcome Store (TAIL-01)**
- **D-04:** The store records MECHANICAL signals per dispatch: (provider, model, tier), outcome class (ok / error class / fallback-used), latency, tokens, cost. No request content, no LLM-judged quality — the zero-LLM letter is a hard boundary.
- **D-05:** Storage = append-only JSONL under `.ass-guard/` (git-excluded like checkpoints), aggregated in memory at read time. No SQLite, no rewritten aggregate file.
- **D-06:** Outcomes feed the EXISTING deterministic seams: breaker state, cost-degrade decisions, fallback ordering. NO new tier-preference layer. Feedback is observable as changed routing decisions after enough evidence accumulates.
- **D-07:** Observability proof = deterministic test (seed the store with synthetic outcomes → assert the routing decision changes: breaker opens / degrade fires) + a CLI subcommand dumping aggregated stats.

**Nightly Parity CI (TAIL-02)**
- **D-08:** Vehicle = GitHub Actions scheduled workflow with an OFF-PEAK cron time (not :00/:30 — the documented cron jitter/dropped-runs problem), the zcode-dependent job labeled for a SELF-HOSTED runner so the corpus stays local.
- **D-09:** The nightly executes DRIFT-ONLY: zcode version probe + profile structure hash vs pinned capture + build/test. Zero nightly API spend; the full LLM behavioral eval stays on the change-class gate (12-08's `ASSGUARD_EVAL_GATE` machinery, unchanged).
- **D-10:** Drift report = run artifact always; on drift, AUTO-OPEN a GitHub issue with the diff summary. The unattended signal survives and notifies without a human watching run logs.

**ECOS-04 Mode Matrix (TAIL-03)**
- **D-11:** "Interaction mode" = turn ORIGIN. Exactly FOUR modes: interactive ACP prompt, subagent dispatch, background wake-turn, automation/cron turn. Steering delivery and parked asks are mechanics WITHIN interactive turns, not separate matrix rows.
- **D-12:** Per mode, ALL THREE surfaces must work: commands resolve + dispatch, skills load + execute, plugin hooks fire. Full matrix: 4 modes × 3 surfaces.
- **D-13:** Proof = scripted E2E harness per mode (fixture plugin/skill, assert FUNCTIONAL outcome — not just discovery/loading), CI-runnable and repeatable. Not a one-time manual checklist.
- **D-14:** Fixtures = synthetic in-repo plugin/skill (deterministic, no network) + ONE real installed Claude Code plugin spot-checked per surface to anchor the "unchanged" claim.

### Claude's Discretion
- The exact Zed MCP feature names/config keys the guide cites (research pins them — see DOC-01 findings).
- JSONL schema field names; aggregation window semantics (rolling vs fixed) for breaker/cost feedback.
- Nightly cron hour; workflow file naming; issue title/body template.
- Synthetic fixture plugin's name/content; which real CC plugin gets the spot-check.
- CLI subcommand name for the stats dump (`ass-guard routing stats` family).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DOC-01 | LSP support documented as an IDE-side MCP configuration requirement (no agent-side implementation) | Pinned Zed feature surface: `context_servers` settings key (local `command/args/env`, remote `url/headers`), the native `diagnostics` built-in tool, forwarding-to-external-agents semantics, zed#52449 gap, and ass-guard's parse-but-ignore of ACP-forwarded `mcpServers` — the doc must route the dry-run through `.mcp.json` (findings + pitfalls below) |
| TAIL-01 | Scheduler outcome store + feedback loop (deterministic, zero LLM calls) | Verbatim seams: `Breaker` / `CostTracker` / `CostAction` / `providerModelKey` interfaces; in-memory `CircuitBreaker` + `CostCeilingTracker`; `SetBreakers`/`SetCostTracker` injection; `EnsureGitignore` self-exclusion precedent; CLI family precedent in `internal/modelroutingcmd` + `ass-guard scheduling` cobra tree |
| TAIL-02 | Nightly upstream-parity gate CI (12-08 tail) | Drift substrate exists: `internal/drift.Detect`, `ass-guard profile check`, `paritycli.zcodeInstalledVersion` seam, `profiles/zcode/` pinned bundle; official schedule-event citations (off-peak cron, default-branch-only, dropped runs); first-workflow-in-repo facts (no `.github/` yet; default branch `master`) |
| TAIL-03 | ECOS-04 — plugins/skills working unchanged in every interaction mode | Mode/surface inventory vs executed phases: interactive + subagent exist (15/16), cron store exists (`internal/sched`, 12-07), background wake + command/skills chain + hook authority land in 22/20/21 (preconditions to mark); harness substrates: 16-06 Zed simulator (`internal/acpserve/simulator_e2e_test.go`), `internal/evalharness`, `internal/ecosys` testdata fixture pattern; verbatim hook event vocabulary |
</phase_requirements>

## Summary

Phase 24 is four independent tails, and all four sit on surfaces that already exist in the repo — but not on the surfaces the phase description assumes in every case. The research found three load-bearing repo facts that reshape planning: (1) **`Scheduler.Dispatch` has no production callers** — the live turn path resolves models via `modelrouting.NewResolver(...).Resolve(...)` at two call sites (`internal/runtime/runtime.go:1316`, `internal/acpserve/config_surface.go:321`), so TAIL-01's outcome recording must attach where dispatches actually happen, and D-06's feedback seams (`Breaker`, `CostTracker`, fallback ordering) live in a Scheduler that production does not currently drive — wiring is part of the design, not a given. (2) **ass-guard parses but ignores the ACP `session/new` `mcpServers` field** (`internal/acp/handlers.go:253-261`) — Zed-forwarded context servers are not consumed, so DOC-01's followable dry-run (D-03) must route through ass-guard's own `.mcp.json`, with the Zed-side `context_servers` story documented honestly. (3) **Zed ships no built-in LSP-providing MCP context server** — the operator's premise maps to Zed's native built-in `diagnostics` agent tool (Zed's own Agent Panel only) plus optional forwarding of settings-configured context servers over ACP; this premise gap is the phase's one user-confirmation item (Assumptions A1).

The remaining tails are well-grounded. TAIL-02 wraps existing drift machinery (`internal/drift.Detect`, `ass-guard profile check`, the `zcode --version` probe seam, the pinned `profiles/zcode/` bundle) in the repo's FIRST GitHub Actions workflow (`.github/workflows/` does not exist yet), with official-docs backing for the off-peak cron choice and a critical gotcha: scheduled workflows only fire when the file exists on the default branch (`master` — not the current milestone branch). TAIL-03 exercises a 4×3 mode/surface matrix of which the interactive-ACP, subagent, and cron-store substrates exist today; the background wake-turn (22), command resolver chain + skills expansion (20), and settings-hook authority (21) are preconditions the planner must mark explicitly.

**Primary recommendation:** Build TAIL-01 as a new `internal/modelrouting` sibling (JSONL store + in-memory aggregation feeding the existing `SetBreakers`/`SetCostTracker` injection points, recorded at the real dispatch/Send sites), make the nightly's drift core a unit-testable Go command wrapped by a minimal scheduled workflow (off-peak minute, self-hosted label, artifact + auto-issue), build the TAIL-03 harness on the 16-06 simulator + ecosys fixture patterns with per-phase precondition marks, and write DOC-01 around the verified three-leg Zed surface (native diagnostics tool / context_servers forwarding + #52449 caveat / ass-guard `.mcp.json`) with the dry-run on the `.mcp.json` leg.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Outcome recording per dispatch (TAIL-01) | Internal runtime (dispatch/Send call sites) | `internal/modelrouting` store | Recording must observe the real provider-call path; store is a modelrouting-owned sibling so seams stay in-package |
| Outcome aggregation → routing feedback (TAIL-01) | `internal/modelrouting` (seam injection) | — | D-06 forbids a new layer; feedback re-enters via existing `SetBreakers`/`SetCostTracker`-style joints |
| Stats dump CLI (TAIL-01) | CLI (cobra cmd shell) | `internal/modelroutingcmd` (render logic) | Phase-15 CLI-support split: cobra shells in cmd/, run-logic in internal/; stderr default, stdout only with `--json` |
| Zed LSP/MCP guide (DOC-01) | Documentation (`docs/lsp-setup.md`) | README docs index | D-02; zero agent-side code (2026-08-25 operator decision) |
| Nightly drift probe (TAIL-02) | Go command (unit-testable core) | CI wrapper (`.github/workflows/`) | Repo pattern: logic in Go (tested), CI is a thin shell; mise tasks are the entry points |
| Issue-on-drift + artifact (TAIL-02) | CI tier (Actions) | — | Only exists in the workflow; GITHUB_TOKEN least-privilege |
| Mode-matrix E2E harness (TAIL-03) | Test harness (Go tests, CI-runnable) | `internal/ecosys` fixtures | D-13 demands repeatable scripted proof; reuses simulator/evalharness/testdata patterns |

## Standard Stack

No new external dependencies. Everything required is stdlib, already-in-tree libraries, or GitHub-hosted Actions.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib (`encoding/json`, `os`, `crypto/sha256`, `time`, `sort`) | go 1.25 floor, repo on 1.26.5 [VERIFIED: `go version` this session] | JSONL append/read, structure hashing, latency/window math | The zero-LLM store needs nothing more; JSONL precedent is the session transcript writer |
| `github.com/spf13/cobra` (in tree) | go.mod pinned | `ass-guard scheduling stats` shell (D-07 CLI) | Existing `ass-guard scheduling resolve` lives here [VERIFIED: cmd/ass-guard/modelrouting.go:63-67] |
| GitHub Actions (hosted service) | current | Nightly scheduled workflow (D-08) | Only the workflow file is new; `.github/` does not exist yet [VERIFIED: `ls .github` fails this session] |
| `actions/checkout` | v7.0.1 latest major [VERIFIED: `gh api repos/actions/checkout/releases/latest` 2026-08-28] | Workflow checkout | First-party; planner pins exact tag at write time |
| `actions/setup-go` | v7.0.0 latest major [VERIFIED: `gh api repos/actions/setup-go/releases/latest` 2026-08-28] | Go toolchain on runner (`go-version-file: go.mod`) | First-party |
| `actions/github-script` | v9.0.0 latest major [VERIFIED: `gh api repos/actions/github-script/releases/latest` 2026-08-28] | Auto-open drift issue (D-10) — avoids shell-quoting the issue body | First-party; issue body built in JS, no `gh` arg-injection surface |
| `actions/upload-artifact` | v7.0.1 latest major [VERIFIED: `gh api repos/actions/upload-artifact/releases/latest` 2026-08-28] | Drift report artifact (D-10 "always") | First-party |
| mise tasks (`.mise.toml`) | repo-defined | Workflow invokes `mise ci` (+ drift task) | `mise ci` = vet+lint+build+test is the standing gate [VERIFIED: .mise.toml:25-27] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `internal/sched` (in tree) | exists (12-07) | Cron semantics reference + the automation-turn store substrate for TAIL-03's cron mode | Never re-implement cron parsing — the 5-field LOCAL-timezone parser exists [VERIFIED: internal/sched/sched.go:7-9 package doc] |
| `internal/ecosys` (in tree) | exists | Fixture discovery surfaces (commands/skills/plugins/hooks) for TAIL-03; `EnsureGitignore` for D-05's git exclusion | Always for fixture layout |
| `internal/drift` + `internal/parity` + `internal/paritycli` (in tree) | exists | TAIL-02's drift probe substrate | The nightly wraps these; do not fork them |
| `internal/acpserve` simulator (in tree, test) | exists (16-06) | Interactive-mode leg of the TAIL-03 harness | D-13's scripted E2E per mode |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `actions/github-script` for the drift issue | `gh issue create` CLI step | gh is simpler but the issue body arrives as an argv/flag — template text with backticks/quotes becomes an injection/quoting nuisance; github-script builds the body in JS. Either is acceptable; github-script is the lower-risk default |
| Go command for drift logic | Pure YAML-steps in the workflow | YAML steps are untestable locally; a Go command (`profile check` family) keeps `mise`-runnable parity with the repo's "logic in Go, shell in CI" pattern |
| Append-only JSONL (D-05) | SQLite | Locked OUT by D-05 — no discussion |
| `actionlint` for workflow YAML validation | GH's own validation on push | actionlint is not installed locally [VERIFIED: `command -v act`/actionlint absent]; GH validates on push + `workflow_dispatch` smoke run is the practical check |

**Installation:**
```bash
# No new Go dependencies. Workflow actions are pinned in the YAML at plan time.
go get  # nothing
```

## Package Legitimacy Audit

> **No external packages are installed by this phase** — stdlib + in-tree packages only; GitHub Actions are first-party (`actions/*` org), latest majors verified via `gh api` today (see Standard Stack). No legitimacy gate runs required.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none — no new deps) | — | — | — | — | — | — |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

*GitHub Action version tags were verified live via `gh api repos/<action>/releases/latest` on 2026-08-28 [VERIFIED: gh api]. Planner should re-pin at plan-writing time if majors moved.*

## Architecture Patterns

### System Architecture Diagram

```
                         TAIL-01 (deterministic, zero LLM)
  turn loop / subagent dispatch ──dispatch attempt──▶ provider.Send
        │                                                    │
        │ record (provider, model, tier, outcome class,      │ ok / transient / structural /
        │ latency, tokens, cost)                             │ fallback-used / exhausted
        ▼                                                    ▼
  [outcome store: .ass-guard/<...>.jsonl]  ◀──append-only──┘
        │
        │ read-time aggregation (in memory)
        ▼
  [aggregated stats] ──seed/adjust──▶ EXISTING seams: Breaker (per provider,model)
        │                              CostTracker (degrade decisions)
        │                              fallback ordering (Resolver inputs)
        ▼
  routing decision changes (breaker opens / degrade fires / fallback order shifts)
        ▼
  `ass-guard scheduling stats` CLI dump (stderr human / --json stdout)

                         TAIL-02 (nightly, unattended)
  GH Actions schedule (off-peak cron, default branch ONLY)
        ▼
  job 1 (hosted runner): checkout → setup-go → mise ci (build/test; zero LLM spend)
  job 2 (SELF-HOSTED label): zcode --version probe (paritycli seam)
        │            + profile structure hash vs pinned capture (profiles/zcode/)
        ├──── no drift ──▶ artifact: drift report (always)
        └──── drift ────▶ artifact + AUTO-OPEN issue (github-script, issues: write)
                          [ASSGUARD_EVAL_GATE change-class machinery UNTOUCHED — D-09]

                         TAIL-03 (4 modes × 3 surfaces)
  scripted E2E harness ──▶ interactive ACP prompt   (exists: acpserve simulator, 16-06)
            │          ▶ subagent dispatch          (exists: 14-05 runtime)
            │          ▶ background wake-turn       (Phase 22 D-01 — PRECONDITION)
            │          ▶ automation/cron turn       (sched store exists; 18 discipline)
            ▼
  per mode assert FUNCTIONAL outcome of: commands resolve+dispatch (Phase 20 — PRECONDITION)
                                          skills load+execute     (Phase 20 — PRECONDITION)
                                          plugin hooks fire       (ecosys HookRunner 12-02
                                                                   + Phase 21 authority — PRECONDITION)

                         DOC-01 (docs only)
  operator ──reads──▶ docs/lsp-setup.md ──configures──▶ Zed context_servers + ass-guard .mcp.json
                                 └─ dry-run (D-03): author follows doc → LSP-symbol tools work
                                    in an ass-guard session
```

### Recommended Project Structure
```
internal/modelrouting/
├── outcomes.go            # NEW: JSONL record type + append-only store (D-04/D-05)
├── outcomes_agg.go        # NEW: read-time aggregation → breaker/cost/fallback feedback (D-06)
├── outcomes_test.go       # NEW: append/read/aggregate + D-07 deterministic feedback test
├── dispatch.go            # record-outcome hook at provider.Send + fallback walk
internal/modelroutingcmd/
├── modelrouting.go        # EmitStats human/JSON renderers (EmitResolve* precedent)
cmd/ass-guard/
├── modelrouting.go        # `scheduling stats` cobra shell
docs/
├── lsp-setup.md           # NEW: D-01/D-02 guide
.github/workflows/
├── nightly-parity.yml     # NEW: scheduled workflow (first in repo)
internal/ecosys/testdata/
├── (fixture plugin/skill for TAIL-03; pattern: testdata/plugins-installed)
```

### Pattern 1: Append-only JSONL store with self-exclusion (D-05)
**What:** One JSON object per line, append-only, owner-only perms, git-excluded via the ecosys self-gitignore pattern.
**When to use:** TAIL-01's outcome store.
**Example:**
```go
// Precedent (verbatim pattern family):
// internal/ecosys/loader.go:21 — const gitignoreContent = "*\n!.gitignore\n"
//   EnsureGitignore writes it to `.ass-guard/.gitignore` so new subdirs are excluded.
// internal/checkpoint/store.go:79 — infoExcludeCarried = ".ass-guard/\n"
// internal/sched/sched.go:33-39 — storeDirName ".ass-guard", filePerm 0o600, dirPerm 0o750
```
[VERIFIED: internal/ecosys/loader.go:18-21; internal/checkpoint/store.go:78-79 quoted in checkpoint/store.go context; internal/sched/sched.go:31-39 read via session tooling — filePerm 0o600, dirPerm 0o750 quoted verbatim in sched.go]

### Pattern 2: Deterministic feedback through existing seams (D-06/D-07)
**What:** Aggregated outcomes re-enter routing only through the seams that already exist:
```go
// internal/modelrouting/dispatch.go:29-33 (verbatim):
type Breaker interface {
	Allow(now time.Time) bool
	RecordSuccess()
	RecordTransient(now time.Time, err *provider.ProviderError)
}
// internal/modelrouting/dispatch.go:37-40 (verbatim):
type CostTracker interface {
	Check(now time.Time) CostAction
	Account(model string, inTokens, outTokens int)
}
// internal/modelrouting/dispatch.go:45-53 (verbatim values):
//   CostAllow / CostDegrade / CostHardStop  (CostAction enum)
// Injection joints (verbatim): Scheduler.SetBreakers (dispatch.go:110-118),
//   Scheduler.SetCostTracker (dispatch.go:120-127) — "Plan 03-03 swaps in" pattern.
```
The store is durable memory for per-process in-memory trackers: `CircuitBreaker` state is mutex-guarded RAM only [VERIFIED: internal/modelrouting/breaker.go:48-57 — `mu sync.Mutex`, no persistence field], and `CostCeilingTracker` resets spend on window rollover [VERIFIED: internal/modelrouting/cost.go:100-110]. Aggregation seeds/adjusts these at construction time — the D-07 test seeds synthetic JSONL → constructs the Scheduler → asserts `breaker opens / degrade fires`.

### Pattern 3: First-workflow shape with least-privilege token (TAIL-02)
**What:** Two-job scheduled workflow; the drift core is a Go command so it is `mise`/`go test`-testable; the workflow is a thin shell.
**Example:**
```yaml
# Shape only — planner finalizes names/cron (Claude's discretion per CONTEXT.md)
on:
  schedule:
    - cron: "23 4 * * *"   # off-peak: NOT :00/:30 — official docs: "High load times
                           # include the start of every hour"; "some queued jobs may
                           # be dropped" [CITED: docs.github.com events-that-trigger-workflows#schedule]
  workflow_dispatch: {}     # manual smoke — the local-validation substitute (no `act`)
permissions:                # least privilege; default GITHUB_TOKEN is read-only
  contents: read
  issues: write             # ONLY for the drift-issue job (D-10)
jobs:
  build-test:        # hosted runner: checkout → setup-go → mise ci
  drift:             # runs-on: [self-hosted, <label>] — zcode corpus stays local (D-08)
    steps: probe → compare vs pinned → upload-artifact (always) → if drift: github-script issue
```
[CITED: docs.github.com/actions/using-workflows/workflow-syntax-for-github-actions — `permissions` per-job syntax; docs.github.com "Using labels with self-hosted runners" — `runs-on: [self-hosted, linux, x64]` all-labels-match]

### Pattern 4: CLI shell / logic split (D-07 stats subcommand)
**What:** Cobra shell in `cmd/ass-guard/modelrouting.go`; renderers in `internal/modelroutingcmd` next to `EmitResolveHuman`/`EmitResolveJSON` [VERIFIED: internal/modelroutingcmd/modelrouting.go:11-24 — "human-readable … to the STDERR writer … stdout stays byte-clean unless --json"]. The D-07 stats dump joins the `ass-guard scheduling` group (existing subcommand: `scheduling resolve` [VERIFIED: cmd/ass-guard/modelrouting.go:63-67]).

### Pattern 5: Scripted E2E over the real composition (TAIL-03)
**What:** The 16-06 Zed simulator pattern — a scripted newline-delimited JSON-RPC client over io.Pipes drives the REAL `acpserve.Run` against a scripted SSE provider stub, with fixed scripts and guard timeouts [VERIFIED: internal/acpserve/simulator_e2e_test.go:14-21 header comment read this session]. TAIL-03's per-mode harness extends this shape: same deterministic-script discipline, new fixture plugin/skill asserting functional outcomes (a command that RAN, a skill body that EXPANDED, a hook that FIRED with observed effect) per mode.

### Anti-Patterns to Avoid
- **Building a new tier-preference layer** (D-06 forbids): feedback must bend the existing breaker/cost/fallback seams, not add a scoring system.
- **Recording request content or LLM-judged quality** (D-04 forbids): mechanical signals only — the zero-LLM boundary is hard.
- **Putting the nightly's logic in YAML**: untestable locally; the repo's pattern is Go logic under test + thin shells.
- **Relying on Zed forwarding for the doc dry-run**: ass-guard ignores forwarded `mcpServers` today (Pitfall 4) — the dry-run routes through `.mcp.json`.
- **Cron at :00/:30** (D-08 forbids): documented delay/drop behavior [CITED: docs.github.com events-that-trigger-workflows#schedule].

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Cron parsing / next-fire semantics (TAIL-03 cron mode) | A new parser | `internal/sched` (12-07 five-field LOCAL-timezone parser + due walker) | Exists, tested, LOCAL-timezone rule documented [VERIFIED: internal/sched/sched.go:7-9] |
| Git exclusion of `.ass-guard/` state | New exclusion logic | `ecosys.EnsureGitignore` self-gitignore (`"*\\n!.gitignore\\n"` → `.ass-guard/.gitignore`) | Established, test-covered precedent [VERIFIED: internal/ecosys/loader.go:18-27] |
| Drift comparison (TAIL-02) | A fresh comparator | `internal/drift.Detect` + `ass-guard profile check` + `paritycli.zcodeInstalledVersion` seam | Tier-1/2 count-based drift semantics already locked (D-06 of the drift phase) [VERIFIED: internal/drift/drift.go:17-24 doc comment read this session] |
| GitHub API issue creation | Raw curl / hand-rolled REST | `actions/github-script` (`github.rest.issues.create`) or `gh issue create` | First-party action; retries built in [CITED: github.com/actions/github-script README] |
| Structure hashing | Custom hash scheme | stdlib `crypto/sha256` over canonicalized JSON of the pinned bundle | Trivial, deterministic; no dependency warranted |

**Key insight:** This phase's risk is not missing libraries — it is wiring into the right existing seams. Every tail names a substrate that already exists; hand-rolling any of them forks tested behavior.

## Runtime State Inventory

> Skipped — this is not a rename/refactor/migration phase. (TAIL-01 adds NEW durable state under `.ass-guard/`; it renames nothing. Existing runtime state touched: none.)

## Common Pitfalls

### Pitfall 1: Scheduler.Dispatch is not the live dispatch path
**What goes wrong:** Recording outcomes inside `Scheduler.Dispatch` (or feeding aggregation only into `SetBreakers`) changes nothing observable, because no production code constructs the Scheduler — the live path resolves via `modelrouting.NewResolver(cfg).Resolve(...)` directly and calls `p.Send` through the session/loop machinery.
**Why it happens:** The Scheduler package was built (03-02/03-03) with full breaker/cost/fallback machinery, but adoption into the turn loop never happened; grep for `NewScheduler|\.Dispatch(` outside `internal/modelrouting` returns zero production call sites (verified this session).
**How to avoid:** Plan an explicit task deciding the attachment point: record at the real Send/fallback sites (runtime session turn + subagent dispatch + config-surface resolve), and make D-06's feedback observable through whichever of the three seams the live path actually consults. D-07's test (seed store → routing decision changes) is the acceptance arbiter — write it against the real seam.
**Warning signs:** D-07 test passes while `ass-guard scheduling resolve` output never changes in any scenario; stats JSONL grows but nothing downstream reads it.

### Pitfall 2: The nightly silently never runs (default-branch rule)
**What goes wrong:** Scheduled workflows fire ONLY if the workflow file exists on the repo's default branch — here `master` [VERIFIED: `git symbolic-ref refs/remotes/origin/HEAD` → `refs/remotes/origin/master`; CITED: docs.github.com events-that-trigger-workflows#schedule — "Scheduled workflows will only run on the default branch"]. The repo works on milestone branches (`gsd/v1.2-claude-code-parity` currently), so a workflow merged only to the milestone branch never schedules.
**How to avoid:** The phase's verification must include the file landing on `master` (or an explicit operator note that the schedule activates at milestone merge); add `workflow_dispatch` so the drift job is manually triggerable from any branch meanwhile.
**Warning signs:** "No scheduled runs found" in the Actions tab after merge.

### Pitfall 3: Cron drift and dropped runs at top-of-hour
**What goes wrong:** `:00`/`:30` schedules experience 20–40 min drift or outright drops under GitHub Actions load.
**Why it happens:** Official: "The `schedule` event can be delayed during periods of high loads of GitHub Actions workflow runs… High load times include the start of every hour… If the load is sufficiently high enough, some queued jobs may be dropped." [CITED: docs.github.com events-that-trigger-workflows#schedule; community discussions 52477, 156282, 196910]
**How to avoid:** D-08 already locks off-peak; pick an odd minute (e.g. 17–53 range, off the hour/half-hour boundaries). A missed nightly is acceptable (next run catches up — drift-only, no spend) but document it.
**Warning signs:** Run timestamps consistently N minutes after cron.

### Pitfall 4: ass-guard ignores Zed-forwarded MCP servers (doc dry-run killer)
**What goes wrong:** The guide tells users to add an LSP context server in Zed, Zed forwards it over ACP in `session/new`, and nothing happens in ass-guard — the D-03 dry-run dead-ends.
**Why it happens:** `handleSessionNew` parses `McpServers []any` and discards it (`_ = json.Unmarshal`) [VERIFIED: internal/acp/handlers.go:253-261 — field at :256, ignored unmarshal at :260]. ass-guard's MCP host loads only the project `.mcp.json` merged with ecosys-discovered plugin servers [VERIFIED: internal/runtime/runtime.go spawnMCP body + :165-169,1343 read this session].
**How to avoid:** The doc's working path (dry-run leg) configures the LSP-capable server in the project `.mcp.json` (Claude-Code-compat — the project's drop-in contract); the Zed `context_servers` leg is documented with its true current status (forwarded to external agents per Zed docs; extension-installed servers NOT forwarded — zed#52449; consumed by ass-guard: not yet, stated as the agent-side-OUT boundary).
**Warning signs:** Dry-run "tools never appear in the ass-guard session."

### Pitfall 5: Zed's LSP surface is not what the premise assumes (A1)
**What goes wrong:** Writing the guide around a built-in Zed LSP MCP server that does not exist.
**Why it happens:** Zed ships no built-in LSP-providing MCP context server. Verified surface: LSP data reaches Zed's OWN agent via the native built-in `diagnostics` tool ("errors and warnings for either a specific file or the entire project"; no symbols/definitions/references tools documented) [CITED: zed.dev/docs/ai/tools]; built-in tools are not documented as exposed to external ACP agents [CITED: zed.dev/docs/ai/tools]. Context servers: `context_servers` settings key; local `command/args/env`; remote `url`/`headers`/OAuth [CITED: zed.dev/docs/ai/mcp]; "Zed can forward configured MCP servers over ACP; agents may also read native MCP config" [CITED: zed.dev/docs/ai/mcp + zed.dev/docs/ai/external-agents]; the #52449 gap for extension-installed servers [CITED: github.com/zed-industries/zed/issues/52449].
**How to avoid:** Pin the doc to this verified three-leg surface (see DOC-01 Architecture section); flag A1 for user confirmation because D-01's worked-example framing assumed a built-in that isn't there.
**Warning signs:** Guide references a Zed settings key that isn't `context_servers`, or promises Zed-native LSP tools inside ass-guard sessions.

### Pitfall 6: GITHUB_TOKEN is read-only by default
**What goes wrong:** The drift job fails to open the issue (403) exactly when drift happens — the unattended signal dies.
**How to avoid:** Explicit least-privilege `permissions: { contents: read, issues: write }` in the workflow (job-level for the drift job); org/repo "Workflow permissions" default may still need read-write granted for GITHUB_TOKEN writes [CITED: github.com/orgs/community/discussions/124684; workflow-syntax docs].
**Warning signs:** First drift run: artifact uploaded, issue step 403s.

### Pitfall 7: Zero-token records on the Send path
**What goes wrong:** D-04 requires tokens/cost per record, but the scheduler's Send path accounts `Account(cand.Model, 0, 0)` because "Response has no token fields on the Send path" — the comment reserves token-cost coupling for the Stream path where `StreamChunk.Usage` supplies counts [VERIFIED: internal/modelrouting/dispatch.go:159-162 + :252; internal/provider/provider.go:41,60-61 and streaming.go:468 show `Usage *Usage` + `"usage"` StreamChunk].
**How to avoid:** The store's schema carries tokens/cost as nullable/zero-able; recording at the session-turn layer (where usage events actually flow) rather than only at `Dispatch` avoids structurally-zero fields; document `0` as "unknown on this path" rather than "free".
**Warning signs:** Every cost aggregation reading zero; cost-degrade feedback never fires.

### Pitfall 8: In-memory seams forget; the store must be the durable memory
**What goes wrong:** Breaker/cost state resets every process restart (editor owns lifecycle) — feedback built only on live tracker state loses history and the "after enough evidence accumulates" observable never materializes.
**How to avoid:** Aggregation reads the JSONL store at startup/construction and seeds the seams; window semantics (rolling vs fixed — Claude's discretion) defined in units of recorded outcomes, not wall-clock alone.
**Warning signs:** Fresh session ignores a provider that failed all yesterday.

### Pitfall 9: ECOS-04 cells premised on unexecuted phases
**What goes wrong:** The harness pins assertion mechanics to Phase 20/21/22 behaviors (resolver chain order, hook verdict contract, wake-turn shape) that are PLANNED, not built; contract drift between 20-24 execution makes cells fail for drift reasons, not regression reasons.
**How to avoid:** Mark per-cell preconditions (Phase 20 commands/skills; 21 hooks; 22 wake-turn; 18 cron discipline) in the plan; assert FUNCTIONAL outcomes via the contracts those phases' CONTEXTs already lock (e.g., hook verdicts via `hookSpecificOutput` + exit-2, 21-D-02) rather than implementation details.
**Warning signs:** Harness edits landing in the same commit as the surface it verifies.

### Pitfall 10: Issue-body injection through CLI args
**What goes wrong:** Building `gh issue create --body "$DIFF_SUMMARY"` with content derived from the diff (paths, hashes) breaks on backticks/quotes, or worse executes if ever sourced from untrusted text.
**How to avoid:** `--body-file` (write the summary to a file) or github-script; never interpolate drift content into shell args.
**Warning signs:** Quarrelsome shell escaping in the workflow.

## Code Examples

### Existing seam: where outcomes are recorded (TAIL-01)
```go
// internal/modelrouting/dispatch.go:250-272 (verbatim structure — the only
// places Dispatch learns an outcome; production equivalents attach at the
// real Send sites per Pitfall 1):
if err == nil {
    b.RecordSuccess()
    s.cost.Account(cand.Model, 0, 0) // Response has no token fields on the Send path
    return resp, nil
}
// ...
perr := asProviderError(err, &cand)
if perr.Kind == provider.KindStructural {
    return provider.Response{}, perr // Structural errors do not feed the breaker
}
// Transient: feed the breaker, emit fallback event, walk on.
b.RecordTransient(s.now(), perr)
```

### Existing seam: durable-store house conventions (TAIL-01)
```go
// internal/sched/sched.go:31-39 (verbatim):
const (
	storeDirName  = ".ass-guard"
	scheduleDir   = "schedule"
	storeFileName = "schedules.json"
	filePerm      = 0o600
	dirPerm       = 0o750
)
```

### Existing seam: the drift probe the nightly wraps (TAIL-02)
```go
// internal/paritycli/parity.go:20-37 (verbatim shape): the zcode version probe is a
// seam var — "a fixed-argv exec of `zcode --version` … under a bounded timeout"
// (zcodeVersionTimeout = 3 * time.Second), injectable for offline tests.
// The pinned bundle lives at profiles/zcode/ (profile.yaml, tools.json,
// thinking.json, tool_choice.json, coverage.yaml, identity.yaml, meta.yaml,
// drift-reports/) [VERIFIED: ls profiles/zcode this session].
// Drift semantics: "TIER-1 … count-based …; TIER-2 … presence/count …"
// [VERIFIED: internal/drift/drift.go:14-24 read this session].
```

### Existing seam: the three TAIL-03 surfaces
```go
// internal/ecosys/hooks.go:54-66 (verbatim — the hook event vocabulary, "never renamed"):
const (
	hookEventPreToolUse       = "PreToolUse"
	hookEventPostToolUse      = "PostToolUse"
	hookEventUserPromptSubmit = "UserPromptSubmit"
	hookEventStop             = "Stop"
	hookEventSubagentStop     = "SubagentStop"
	hookEventSessionStart     = "SessionStart"
	hookEventSessionEnd       = "SessionEnd"
)
// Commands/skills/agents/plugins discovery: ecosys.Load → loadAll over project
// + user `.claude/` and `.ass-guard/` trees, D-06 two-phase precedence
// [VERIFIED: internal/ecosys/loader.go:39-43 + :57-66 read this session].
// Fixture precedent: internal/ecosys/testdata/{opsx-real,plugins-installed}.
```

### Zed config the doc will cite (DOC-01)
```jsonc
// Shape per zed.dev/docs/ai/mcp [CITED] — exact key names pinned for D-01:
{
  "context_servers": {
    "some-lsp-server": {
      "command": "some-command",
      "args": ["arg-1", "arg-2"],
      "env": {}
    }
  }
}
// Remote: { "url": "https://example.com/mcp", "headers": { "Authorization": "Bearer <token>" } }
// GUI: Settings → AI → MCP Servers, or `agent: open settings`.
// Tool permissions rule format: "mcp:<server>:<tool_name>" (e.g. "mcp:github:create_issue").
// Zed-native LSP leg: built-in `diagnostics` agent tool (Zed's own Agent Panel).
```

## DOC-01 Deep Dive: the pinned Zed feature surface (D-01's research-time pin)

| Leg | What exists | Reaches ass-guard sessions? | Doc treatment |
|-----|-------------|------------------------------|---------------|
| Zed built-in `diagnostics` tool | LSP-backed errors/warnings per file/project; profile-visible as `"diagnostics": false` among built-ins | NO — Zed's own Agent Panel tools; no documented exposure to external ACP agents [CITED: zed.dev/docs/ai/tools] | Describe as the Zed-native LSP capability; scope-boundary for agent-side work |
| Zed `context_servers` (settings-configured MCP) | `context_servers` key; local `command/args/env`; remote `url`/`headers`; OAuth; extensions; tools+prompts forwarded to external agents "over ACP" [CITED: zed.dev/docs/ai/mcp, /external-agents] | PARTIALLY — forwarded to external agents per docs, but extension-installed servers are NOT forwarded (zed#52449), and ass-guard currently discards the forwarded set (Pitfall 4) | Worked example configuration + honest status box |
| ass-guard native MCP config (`.mcp.json`) | Project `.mcp.json` (camelCase `mcpServers`) loaded by `mcp.LoadConfig`, merged with ecosys-discovered plugin servers [VERIFIED: internal/mcp/host.go:75-89 field quote; runtime spawnMCP] | YES — this is the leg that works today | THE dry-run leg (D-03): configure an LSP-capable MCP server here → tools appear in the ass-guard session |

**Recommendation:** the doc presents all three legs with the status table above; the D-03 dry-run scripts against the `.mcp.json` leg (deterministic, verifiable, no Zed-forwarding dependency). A1 in the Assumptions Log flags the premise gap for user confirmation.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| GH Actions cron anywhere (often :00) | Off-peak odd minutes + workflow_dispatch backstop; official docs now name top-of-hour as high-load | Documented in current events-that-trigger-workflows page | D-08's off-peak choice is the officially recommended pattern [CITED: docs.github.com] |
| Zed MCP = Agent-Panel-only feature | Context servers forward to External Agents over ACP (with the #52449 extension gap) | Zed external-agents docs (current) | The doc can honestly present Zed-side config as part of the ass-guard setup story |
| schedule-only triggering | `timezone` key now supported in schedule (IANA string) alongside UTC default | Current docs | Cron hour can be expressed in local time if preferred (discretion) |

**Deprecated/outdated:** nothing in this phase's stack is deprecated; `go.lsp.dev/jsonrpc2`-style framing concerns do not arise here (no new wire code).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Zed ships no built-in LSP-providing MCP context server; the operator premise maps to the native `diagnostics` tool + context-server forwarding. The guide therefore needs a concrete LSP-capable context server (or the `.mcp.json` leg) for the dry-run's "working LSP-symbol tools" — WHICH server is unspecified by locked decisions (Serena explicitly excluded by operator in specifics). | DOC-01 Deep Dive; Pitfall 5 | Doc content diverges from operator's mental model; dry-run leg needs a named server choice. Needs user confirmation at plan/review |
| A2 | GitHub Action major versions (checkout v7, setup-go v7, github-script v9, upload-artifact v7) are current as of 2026-08-28 [VERIFIED: gh api this session]; planner re-pins at write time | Standard Stack | Low — majors move; pin at plan time |
| A3 | The outcome-feedback attachment point (which live dispatch sites record; how aggregation re-enters `SetBreakers`/`SetCostTracker`/fallback ordering) is a design decision left to planning; research only maps the seams | Pitfall 1; Pattern 2 | If the live path never consults the seams, D-07's observable can't exist — the plan must include the wiring task |
| A4 | "Profile structure hash" = sha256 over a canonicalized serialization of the pinned `profiles/zcode/` bundle (exact file set chosen at plan time); no existing hash implementation to extend | TAIL-02 patterns | Low — deterministic either way; the comparison contract just needs pinning |
| A5 | Real CC plugin choice for the D-14 spot-check (one per surface) — Claude's discretion per CONTEXT; research made no choice | Pitfall list / TAIL-03 | Low |
| A6 | `Usage` data availability at the recording site: streaming `usage` chunks exist [VERIFIED: internal/provider/provider.go:41,60-61], but whether the session turn layer surfaces them per-dispatch was not traced end-to-end this session | Pitfall 7 | Medium — if usage is not reachable at the recording site, tokens/cost stay zero and cost-degrade feedback needs the documented zero-handling |

## Open Questions

1. **Which LSP-capable MCP server is the dry-run's worked example?**
   - What we know: Zed's native LSP surface doesn't reach external agents; ass-guard consumes `.mcp.json` today; Serena explicitly excluded by operator framing; the generic LSP-MCP pattern is "secondary" per D-01.
   - What's unclear: whether the operator wants (a) the doc to keep Zed-as-framing with the honest surface + `.mcp.json` dry-run leg (research recommendation), or (b) to name a specific LSP MCP server for the worked example.
   - Recommendation: planner carries A1 into the plan as a checkpoint:human-confirm before the doc task finalizes its worked example; the status-table doc structure works under either answer.
2. **Where exactly does the live path start consulting aggregated outcomes?**
   - What we know: the seams exist (`SetBreakers`/`SetCostTracker`, resolver fallback ordering via `TierBinding.Fallback`); production drives only the Resolver today.
   - What's unclear: whether Phase 24 wires `Scheduler.Dispatch` into the live turn loop (bigger blast radius) or attaches recording + feedback at the two Resolve call sites + the Send site (smaller, matches D-06's letter).
   - Recommendation: plan a dedicated design task for TAIL-01 with the D-07 test written first as the contract.
3. **Does the nightly re-capture from zcode, or only probe + compare?**
   - What we know: D-09 says "zcode version probe + profile structure hash vs pinned capture + build/test"; the drift CLI's live path reads "the freshest main rollout line" (profile_check.go doc).
   - What's unclear: whether the self-hosted job runs a zcode recapture (heavier, needs the recapture runbook) or a version probe + hash comparison (lighter, deterministic).
   - Recommendation: light path (probe + hash + `mise ci`) for v1 of the workflow; recapture stays the operator runbook (docs/recapture-runbook.md exists).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | TAIL-01/02/03 build+test | ✓ | go1.26.5 darwin/amd64 | — |
| mise | Task entry points (`mise ci`) | ✓ | 2026.8.14 | Direct `go test`/`golangci-lint` invocations |
| golangci-lint | `mise ci` lint leg | ✓ | 2.12.2 (v2 config) | — |
| gh CLI | TAIL-02 workflow validation; action version pinning; issue smoke | ✓ | 2.98.0 | REST via curl |
| zcode binary | TAIL-02 version probe; TAIL-03 real-plugin anchor context | ✗ (this dev machine) | — | By design: the probe job is SELF-HOSTED-labeled (D-08) where zcode exists; local dev uses the test-injectable seam `zcodeInstalledVersion` [VERIFIED: internal/paritycli/parity.go:27-37 seam var] |
| `act` / actionlint (local workflow runners/linters) | Local workflow validation | ✗ | — | `workflow_dispatch` manual trigger + GH's own YAML validation on push |
| GitHub Actions | TAIL-02 vehicle | ✓ (repo hosted on github.com/Djarvur/ass-guard-agent) | — | — |

**Missing dependencies with no fallback:** none blocking (zcode absence is structural and resolved by the D-08 self-hosted label).
**Missing dependencies with fallback:** local workflow linting (fallback: dispatch-trigger smoke run).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + race detector (repo-wide convention; testify not used) |
| Config file | `.mise.toml` (tasks); no test-framework config file |
| Quick run command | `go test -race -count=1 ./internal/modelrouting/ ./internal/ecosys/` |
| Full suite command | `mise test` (=`go test -race -count=1 ./...`) / `mise ci` (vet+lint+build+test) [VERIFIED: .mise.toml:13-27] |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TAIL-01 | JSONL store append/read/tolerant-parse (D-04/D-05) | unit | `go test -race -count=1 ./internal/modelrouting/ -run TestOutcomes` | ❌ Wave 0 |
| TAIL-01 | Deterministic feedback: seed store → breaker opens / degrade fires (D-07) | unit (deterministic, injected clock) | `go test -race -count=1 ./internal/modelrouting/ -run TestOutcomeFeedback` | ❌ Wave 0 |
| TAIL-01 | `scheduling stats` dump: human→stderr, `--json`→stdout (D-07) | unit (cobra tree test, `modelrouting_test.go` precedent) | `go test -count=1 ./cmd/ass-guard/ -run TestSchedulingStats` | ❌ Wave 0 |
| TAIL-02 | Drift-core command: version probe seam + structure-hash compare (D-09) | unit (offline, injected seams — `paritycli` precedent) | `go test -race -count=1 ./internal/paritycli/ ./internal/profilecheckcmd/` | ✅ (existing suites extend) |
| TAIL-02 | Workflow runs unattended & reports drift (D-08/D-10) | manual + workflow_dispatch smoke (local runner absent; no `act`) — justification: schedule semantics only observable in GH; everything under the workflow IS automated | GH Actions UI / `gh workflow run` + `gh run watch` | — |
| TAIL-03 | 4 modes × 3 surfaces functional matrix (D-11..D-13) | E2E (scripted, deterministic, CI-runnable) | `go test -race -count=1 ./internal/acpserve/ -run TestSimulator` (interactive leg exists) + new per-mode harness tests | ❌ Wave 0 (interactive leg ✅) |
| DOC-01 | Guide is followable end-to-end, reaching working LSP tools (D-03) | manual-only (D-03 explicitly: "the doc's author follows it top-to-bottom… Review-only accuracy does not satisfy") | manual dry-run checklist recorded in the plan/verification | — |

### Sampling Rate
- **Per task commit:** `go test -race -count=1 ./internal/modelrouting/ ./internal/ecosys/ ./internal/acpserve/`
- **Per wave merge:** `mise test` (full race suite)
- **Phase gate:** `mise ci` green before `/gsd:verify-work`; DOC-01 dry-run executed; TAIL-02 `workflow_dispatch` smoke run green (schedule leg verified post-merge to `master` or explicitly handed to the operator)

### Wave 0 Gaps
- [ ] `internal/modelrouting/outcomes_test.go` (+ agg/feedback tests) — covers TAIL-01 store + D-07 contract
- [ ] `cmd/ass-guard/modelrouting_test.go` additions — covers `scheduling stats` wire discipline (stderr default / `--json` stdout)
- [ ] New per-mode E2E harness test package (location per planner; substrates: `internal/acpserve/simulator_e2e_test.go`, `internal/evalharness`, `internal/ecosys/testdata`) — covers TAIL-03 matrix cells whose phases have executed
- [ ] Fixture plugin/skill under `internal/ecosys/testdata/` (synthetic, deterministic, no network — D-14)
- [ ] Workflow YAML — not locally testable; validated via dispatch smoke (documented in TAIL-02 map row)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | No user auth in this phase (API keys stay env/file per project constraint; workflow uses GITHUB_TOKEN only) |
| V3 Session Management | no | — |
| V4 Access Control | yes | Least-privilege workflow `permissions` (contents: read; issues: write only where needed); `.ass-guard/` store under owner-only perms (0600/0750 house pattern) |
| V5 Input Validation | yes | JSONL store reads tolerate unknown kinds/fields (16-D-20 additive discipline); workflow inputs are none (schedule) or repo-controlled |
| V6 Cryptography | yes (minor) | stdlib `crypto/sha256` for the integrity/structure hash only — never hand-rolled crypto |
| V14 Config | yes | CI supply-chain: first-party actions pinned to version tags at plan time; SHA-pinning is an available hardening upgrade (discretion) |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Read-only GITHUB_TOKEN silently breaks issue-on-drift (unattended signal loss) | Denial of Service | Explicit `issues: write` job permission + a workflow-level smoke run during the phase; artifact always uploaded regardless (D-10's belt) |
| Shell/arg injection when composing the drift issue body | Tampering / Elevation | github-script (`issues.create` object arg) or `gh --body-file`; never interpolate diff content into shell strings |
| Secrets leaking into artifacts/logs (drift report, stats dump) | Information Disclosure | Store schema records no request content and no credentials (D-04 hard boundary); issue body is a diff summary of structure counts/hashes, not capture bytes |
| Self-hosted runner executing untrusted workflow code | Elevation | Schedule-only trigger (no `pull_request_target`); repo-private; zcode corpus stays local per D-08 |
| Cron-triggered workflow runs from a stale/malicious default-branch state | Tampering | Standard branch protection on `master`; the workflow file itself lands only via milestone merge (Pitfall 2 documented) |

## Sources

### Primary (HIGH confidence)
- `internal/modelrouting/dispatch.go`, `breaker.go`, `cost.go`, `config.go` — Read in full this session: Scheduler seams, D-04-relevant zero-token path, `SetBreakers`/`SetCostTracker`, `CostAction` values, `TierBinding.Fallback`
- `internal/ecosys/loader.go`, `hooks.go` — Read this session: `gitignoreContent`, discovery API, hook event vocabulary ("never renamed")
- `internal/acp/handlers.go:252-269`, `internal/runtime/runtime.go` (spawnMCP, resolveSubagentModel, mcpServers setter), `internal/acpserve/config_surface.go` — Read/grep this session: the parse-but-ignore finding and live Resolve call sites
- `.mise.toml`, `internal/sched/sched.go`, `internal/checkpoint/store.go`, `internal/drift/drift.go`, `internal/paritycli/parity.go`, `cmd/ass-guard/{parity,profile_check,modelrouting}.go`, `internal/modelroutingcmd/modelrouting.go`, `internal/evalsuite/suite.go`, `internal/evalharness/harness.go`, `internal/acpserve/simulator_e2e_test.go`, `README.md`, `docs/` tree, `profiles/` tree — repo substrates read this session
- zed.dev/docs/ai/mcp — `context_servers` key, local/remote shapes, external-agent forwarding sentence [CITED]
- zed.dev/docs/ai/tools — built-in tool list incl. `diagnostics`; built-ins framed as Zed's own agent's tools [CITED]
- zed.dev/docs/ai/external-agents — forwarding "may be forwarded over ACP"; both-configs debugging [CITED]
- docs.github.com events-that-trigger-workflows#schedule — delay/drop warning verbatim, default-branch-only rule, UTC/timezone, 5-min floor [CITED]
- docs.github.com workflow-syntax + using-labels-with-self-hosted-runners — `runs-on` label array (all labels must match), `permissions` [CITED]

### Secondary (MEDIUM confidence)
- github.com/zed-industries/zed/issues/52449 — extension-installed MCP servers not forwarded to ACP agents [CITED: issue title/summary via search; open issue — status should be re-checked at doc-writing time]
- github.com/actions/github-script — `github.rest.issues.create` usage, retry support [CITED]
- Community cron-drift discussions 52477 / 156282 / 196910 (the discussion D-08 names) — 20–40 min drift anecdote range [CITED]

### Tertiary (LOW confidence)
- None — no claim in this document rests on uncorroborated search results. Items that would have been LOW were moved to the Assumptions Log instead (A1, A3, A4, A6).

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — zero new deps; actions verified live via `gh api`; all Go pieces in-tree and read
- Architecture: HIGH — every seam quoted from files Read this session; one genuine design gap (live-path wiring, A3) explicitly surfaced rather than papered over
- Pitfalls: HIGH — each pitfall is anchored to a verified repo fact or an official-docs quote
- DOC-01 surface: HIGH on what Zed ships; the operator-premise gap is flagged (A1) rather than silently resolved

**Research date:** 2026-08-28
**Valid until:** 2026-09-27 (stable domain; re-verify GitHub Action majors and zed#52449 status at plan-writing time)
