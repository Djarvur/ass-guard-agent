---
phase: quick-261027-syj
plan: 01
type: execute
files_modified:
  - .gitignore
  - internal/paritycli/parity.go
  - internal/paritycli/parity_test.go
  - go.mod
  - go.sum
  - .planning/WINDOWS.md
  - .planning/STATE.md
autonomous: true
must_haves:
  truths:
    - "`git status --porcelain` on a fresh clone-equivalent reports no untracked runtime artifacts: .gsd/CODEBASE.md, .gsd/KNOWLEDGE.md, nightly-report.json are gitignored"
    - "The zcode version probe resolves zcode 3.x's log-shaped `--version` stdout (version only inside `version=3.16.3` / `current version: 3.16.3` log lines) to the bare version token; the pre-3.x bare-print shape returns byte-identical output to the old probe; ambiguous or version-less output is a typed probe error (probe-failed verdict), never a guess"
    - "After the fix, `mise run parity-nightly` reports the HONEST drift verdict — version mismatch naming installed 3.x vs pinned 0.16.3 with 7/7 bundle files matching — exit 7, never probe-failed on a healthy exec"
    - "master carries the dependabot golang.org/x/net 0.55.0 merge (PR auto-closes on push); go build + go vet + x/net-consuming package tests green at the merge"
    - "The nightly hosted workflow runs green via live workflow_dispatch on the pushed master (evidence for discharging WINDOWS #26/#28)"
  artifacts:
    - ".gitignore with the three runtime artifacts under the existing local-tooling block (tracked .gsd content stays sentinel-only)"
    - "internal/paritycli: parseZcodeVersionOutput pure helper + seam default consuming it + 10s bounded timeout (Electron cold start can exceed the old 3s — the root cause of the 2026-09-27 probe-failed)"
    - ".planning/WINDOWS.md rows #26/#28 resolved with the live dispatch evidence"
---

# Quick 261027-syj: pre-v1.3 release hygiene

<objective>
Close the hygiene backlog accumulated since the 2026-10-14 lint-gate restoration so master is publishable and the nightly gates tell the truth: (1) untracked runtime artifacts gitignored (the same class that made goreleaser treat the tree as dirty at the v1.2 release); (2) the zcode version probe taught to read zcode 3.x's log-shaped `--version` output (installed zcode is now 3.16.3 — auto-updated past the 3.14.0 noted on 2026-09-27 — and the probe has been reporting probe-failed since); (3) dependabot golang.org/x/net 0.55.0 merged (open PR since 2026-09-27, the repo's one open moderate alert); (4) master pushed (23+ unpushed commits) and the nightly hosted workflow live-dispatched as the post-merge smoke the WINDOWS ledger has owed since Phase 24.

Scope guards: the parity-pin RE-CAPTURE decision (installed 3.x vs pinned 0.16.3) is NOT made here — after this task the drift gate reports the version mismatch honestly; pin updates go through docs/recapture-runbook.md or the newly-available source-derived path (zcode open-sourced 2026-10: zai-org/ZCode) — operator's call at v1.3 planning. The remaining nightly -skip entries (TestRunSuite_*, D-23-07-1 pair, TestEscalation flake) are NOT root-caused here; they stay documented in nightly.yml.
</objective>

<context>
@internal/paritycli/parity.go
@internal/paritycli/parity_test.go
@.gitignore
@.github/workflows/nightly.yml
@.planning/WINDOWS.md

Ground truth captured live 2026-10-27 on this host: `zcode --version` (now /usr/bin/zcode 3.16.3) exits 0 in ~1s warm, stdout = Electron app startup logs; the ONLY semver tokens in the whole output are two agreeing mentions — `[arms] electron initialized env=prod version=3.16.3` and `[auto-update] initializing, current version: 3.16.3`. stderr carries only node ExperimentalWarnings. The 2026-09-27 probe-failed (found="") was the old 3s timeout losing to cold Electron start or the raw-log "found" mismatching; the fix must survive cold starts (10s) and parse the token.

Parser contract (mirrors how the pin compares): the pin's expected value stays the bundle's meta.yaml record (0.16.3, bare). parseZcodeVersionOutput returns a bare token so `found == pin.ZcodeVersion` remains the comparison: (a) whole-output single version token (the pre-3.x shape) returned as-is after trim; (b) otherwise all `version[=:]\s*"?(semver)` mentions collected — exactly one distinct value returns it, zero or conflicting values return a typed error (probe-failed verdict downstream — an unprovable environment must never read as parity, nightly_check.go:114).

goreleaser precedent for the gitignore entries: v1.2 release needed a stash of .gsd/CODEBASE.md/.gsd/KNOWLEDGE.md because goreleaser treats ANY untracked file as dirty (ass-guard-23-uat-state memory). nightly-report.json is the mise parity-nightly task's report output path (.mise.toml writes it to the repo root).
</context>

<tasks>

## Task 1 — gitignore the untracked runtime artifacts
files: .gitignore
action: add `.gsd/CODEBASE.md`, `.gsd/KNOWLEDGE.md`, `nightly-report.json` to the existing "Local tooling + runtime state" block (comment notes: gsd-core intel files are per-machine regenerations; the parity-nightly report is a runtime output).
verify: `git status --porcelain` shows zero untracked entries.
commit: `chore(quick-261027-syj): gitignore gsd intel files + parity-nightly report (runtime artifacts)`

## Task 2 — teach the zcode version probe the 3.x output shape
files: internal/paritycli/parity.go, internal/paritycli/parity_test.go
action: extract pure `parseZcodeVersionOutput(string) (string, error)`; seam default parses the exec output (contract above); bump `zcodeVersionTimeout` 3s → 10s with the cold-start rationale in the constant's doc comment; table-driven parser tests incl. the two REAL captured 3.16.3 lines verbatim, the bare 0.16.3 shape, conflicting mentions, version-less logs, empty input.
verify: `go test ./internal/paritycli/... -count=1` green; `go vet ./internal/paritycli/...`; `mise run lint` stays exit 0; then the LIVE leg — `mise run parity-nightly` exits 7 with the summary naming installed 3.x vs pinned 0.16.3 and 7/7 files matching (probe-failed gone).
commit: `fix(quick-261027-syj): parse zcode 3.x log-shaped --version output in the parity probe`

## Task 3 — merge dependabot golang.org/x/net 0.55.0
files: go.mod, go.sum
action: `git merge --no-edit origin/dependabot/go_modules/golang.org/x/net-0.55.0`; verify `go build ./...`, `go vet ./...`, and tests of packages importing golang.org/x/net (html-to-markdown / net/html consumers: internal websearch/webfetch paths).
verify: build+vet green, affected tests green, PR auto-closes at push.
commit: merge commit (dependabot message preserved)

## Task 4 — push master + live nightly smoke + ledger discharge
files: .planning/WINDOWS.md, .planning/STATE.md
action: push master (+PR auto-close); `gh workflow run nightly` (workflow_dispatch on master); poll the run to completion — green = the post-merge smoke WINDOWS #26/#28 were deferred for; discharge rows #26/#28 with run URL evidence; update STATE.md quick-task row + Last activity; final docs commit via gsd-tools.
verify: `gh run list --workflow=nightly` shows the dispatched run concluded success; `gsd-tools windows` open count 11 → 9.
commit: `docs(quick-261027-syj): release hygiene batch — windows #26/#28 discharged on the live nightly smoke; state row`

</tasks>

<deferred>
- Parity-pin re-capture (installed 3.16.3 vs pinned 0.16.3): now has TWO resolution paths — docs/recapture-runbook.md live capture OR source-derived grounding against the newly open zcode (zai-org/ZCode, Apache-2.0, operator intel 2026-10-27). Operator decision at v1.3 planning; the drift gate honestly exits 7 until then.
- Remaining nightly -skip entries unchanged (TestRunSuite_* needs `openspec` binary on hosted runners; D-23-07-1 acpserve simulator pair; TestEscalation_ReapAllUsesLadder full-suite -race flake).
- WINDOWS #27 (skip-list shrink) stays open by design until the last documented failure is fixed.
</deferred>
