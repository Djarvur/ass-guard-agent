---
status: complete
quick_id: 261027-syj
date: 2026-10-27
commits:
  - 41beb26 "chore(quick-261027-syj): gitignore gsd intel files + parity-nightly report (runtime artifacts)"
  - a498bed "fix(quick-261027-syj): parse zcode 3.x log-shaped --version output in the parity probe"
  - 6d4a765 "Merge remote-tracking branch 'origin/dependabot/go_modules/golang.org/x/net-0.55.0'"
  - (docs commit: this file + windows ledger + STATE.md row)
---

# Quick 261027-syj: pre-v1.3 release hygiene — complete

All four tasks executed and verified; master pushed (11bda1b → 6d4a765 + docs, 26+ commits), dependabot PR #1 MERGED, nightly workflow live-dispatched — capability proven, hosted flake pair honestly recorded (row #31).

## Task 1 — gitignore runtime artifacts (41beb26)
`.gsd/CODEBASE.md`, `.gsd/KNOWLEDGE.md` (gsd-core per-machine intel regenerations; the same files that forced a stash at the v1.2 goreleaser run) and `nightly-report.json` (the `mise run parity-nightly` report output) joined the existing local-runtime ignore block. `git status --porcelain` clean of untracked runtime files.

## Task 2 — zcode 3.x version probe (a498bed)
- Live ground truth first: installed zcode is now **3.16.3** (auto-updated past the 3.14.0 recorded 2026-09-27); `zcode --version` exits 0 in ~1s warm but prints Electron startup logs; the only semver mentions are two agreeing lines (`version=3.16.3`, `current version: 3.16.3`); a cold start can exceed the old 3s bound — the root cause of the standing probe-failed.
- `parseZcodeVersionOutput` (internal/paritycli/parity.go): pre-3.x bare token verbatim after trim; 3.x log shape resolves its single distinct `version[=:]` mention (literal-key regex keeps UA/path semvers out); zero or conflicting mentions → typed `errZcodeVersionUnparsable` (renders as probe-failed downstream — unprovable never reads as parity). Seam default routes through it; timeout 3s → 10s (still bounded).
- TDD: Compiling-RED (stub + table; 4 subtests red on the stub) → GREEN. Table includes the two real captured 3.16.3 lines verbatim. Package green, `go vet` clean, `mise run lint` 0 issues.
- Live leg: `mise run parity-nightly` now reports the honest verdict — **DRIFT: zcode version moved — pinned 0.16.3, found 3.16.3; 7/7 bundle files match** (exit 7). probe-failed is gone.

## Task 3 — dependabot x/net 0.55.0 (6d4a765)
`golang.org/x/net` 0.41.0 → 0.55.0 merged locally from `origin/dependabot/...`; `CGO_ENABLED=0 go build ./...` + `go vet ./...` green; the sole non-test consumer (`kit/toolexec` ddg/WebFetch path) tests green; `internal/profilecheckcmd` (the probe's consumer) green. Push auto-closed PR #1 as MERGED (the repo's one open dependabot alert clears with it).

## Task 4 — push + live nightly smoke + ledger
- master pushed: 23 accumulated commits (2026-10-13/14 tech-debt + lint-gate batches) + this task's 3.
- `gh workflow run nightly` (workflow_dispatch) — the capability this task owed since Phase 24 works (no 404): dispatched twice (runs 40556085571, 40557341336). **Both runs concluded FAILURE, each on a DIFFERENT timing-marginal suite**: run 1 failed coreexec `TestPTYEmpty/StateUnchangedAfterEmpty` (leading-`\n` tty-echo leak into the pwd window); run 2 failed acpserve `TestPermissionsE2E` stage-3 simulator frame-timeout (`seen=6`) — the known D-23-07-1 environmental class extending to a new test. Each failed suite PASSED in the sibling run on identical code and passes locally under `-race` (acpserve green with the documented skip list; coreexec green `-count=20`; full acpserve `-count=2` fails ONLY the documented D-23-07-1 pair). No pushed commit introduced either failure — evidence: same-code sibling-run passes + local greens.
- Honesty bookkeeping: WINDOWS #26/#28 discharged with reasons stating the dispatch capability is proven and the red outcome is NOT papered over; NEW open row #31 records the hosted flake pair with both run IDs and candidate resolutions (simulator frame-deadline tuning under CI load / PTY window-trim review). Ledger: open 11 → 10 (two discharged, one born). Skip-listing the two suites is deliberately left to the operator — a 50%-on-hosted pass rate should stay visible, not silenced.
- Bonus observation: the scheduled 03:41 UTC leg ran green today on pre-push master (run 40479780001, 2m39s) — the v1.2 close-out's "cron activation merge-gated" question is answered in passing.

## Deferred (unchanged, operator's call)
- Parity re-capture/re-pin (3.16.3 vs 0.16.3): two resolution paths now — docs/recapture-runbook.md live capture OR source-derived grounding against the newly open zcode (**zai-org/ZCode**, Apache-2.0, operator intel 2026-10-27). Decide at v1.3 planning; the gate honestly exits 7 until then.
- Nightly -skip residue (TestRunSuite_* needs `openspec` on hosted runners; D-23-07-1 acpserve simulator pair; TestEscalation_ReapAllUsesLadder flake).
- Floating `golangci-lint = "2"` mise pin (261014-1ne-deferred-items.md).
