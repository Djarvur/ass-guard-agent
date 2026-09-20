---
phase: 15-internal-runtime-carve-step-0
verified: 2026-08-26T00:00:00Z
status: passed
score: 3/4 must-haves verified
behavior_unverified: 1 # Count of ⚠️ PRESENT_BEHAVIOR_UNVERIFIED truths (present + wired, behavior not exercised)
behavior_unverified_items: # Only if behavior_unverified > 0 — the truths above as structured items; emitted regardless of overall status

  - truth: "A live Zed-spawned session streams tokens, executes tools, and replays on restart exactly as at v1.1 close"
    test: "Spawn ass-guard acp serve from Zed as in daily use, send a prompt, exercise a tool call (file read/edit), restart Zed mid-session (full checklist: phase-review.md 'Operator live-Zed checklist')"
    expected: "Token streaming renders natively (session/update chunks), tool diffs render natively and results return, restart replay matches v1.1-close behavior (session/load no-op D-09, transcripts on disk under .ass-guard/)"
    why_human: "Editor-rendered perceptual comparison against the operator's daily-use memory of v1.1 close; no automated harness renders Zed's UI surface"
test: "Spawn ass-guard acp serve from Zed as in daily use, send a prompt, exercise a tool call (file read/edit), restart Zed mid-session (full checklist: phase-review.md 'Operator live-Zed checklist')"
expected: Token streaming renders natively (session/update chunks), tool diffs render natively and results return, restart replay matches v1.1-close behavior (session/load no-op D-09, transcripts on disk under .ass-guard/)
why_human: Editor-rendered perceptual comparison against the operator's daily-use memory of v1.1 close; no automated harness renders Zed's UI surface
---

# Phase 15: internal/runtime Carve (Step 0) Verification Report

**Phase Goal:** The turn runner lives in its final home before any feature touches it — seven of ten feature clusters modify `sessionTurnRunner`, and carving first means features land once instead of migrating twice.
**Verified:** 2026-08-26
**Status:** human_needed

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `internal/runtime` houses `sessionTurnRunner` (as `Runner`) verbatim with engine/MCP adapters and cron wiring; `cmd/ass-guard` composes it; `mise ci` green | ✓ VERIFIED | internal/runtime/runtime.go (~1470 lines: Runner, RunnerConfig, NewRunner, sessionFor, advisory family, spawnMCP); cron_wiring.go git-mv'd byte-verbatim (receiver sed only, amended D-13/D-15); enginebridge/ adapters; cmd/ass-guard/acp_serve.go is cobra shells calling `acpserve.Run` → `runtime.NewRunner`; `mise ci` exit 0 (phase-review.md Leg 1: vet + golangci-lint 0 issues + CGO_ENABLED=0 build + `go test -race ./...`) |
| 2 | Serve-path behaviors exercised by the standing suite are observably identical after the move | ✓ VERIFIED | Ledger exact: repo 829 = 826 baseline + 3 goldens; carve-scope 134 = 131 listed + 3 goldens; no test lost or gained beyond sanctioned goldens (phase-review.md Leg 2). Race suite green through the new homes: stdout discipline (WiresStdoutClean/NoStdoutPollution in internal/acpserve/serve_test.go), audit-through-real-seam (TestServeAudit_RequestShapedThroughRealSeam), mirror override, cron lifecycle (internal/runtime/cron_wiring_test.go), TurnRunner contract byte-identical post-rename |
| 3 | A live Zed-spawned session streams tokens, executes tools, and replays on restart exactly as at v1.1 close | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Present + wired: the full serve path (streaming forwarder, catalog executor, transcript persistence, restart no-op D-09) is code-identical by the verbatim-move proof and green under -race; the "exactly as at v1.1 close" editor-rendered identity is not exercisable by any automated harness — operator checklist pending (PENDING-OPERATOR-CONFIRMATION in 15-07-SUMMARY.md; WINDOWS ledger entry blocks /gsd-ship) — see Human Verification |
| 4 | No feature code moved or rewritten — the diff is a pure relocation, reviewable as such | ✓ VERIFIED | Color-moved census at phase-start baseline f1e26b3: 19 renames (69-99% similarity, residue = package clauses/import blocks/sanctioned seds), 22 adds (destination files by construction), 11 modifies; every edited block classified into the 9 sanctioned categories; non-sanctioned edited blocks: 0 (phase-review.md Leg 4) |

**Score:** 3/4 truths verified (1 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/runtime/runtime.go` | Runner family verbatim | ✓ EXISTS + SUBSTANTIVE | Runner struct + RunnerConfig + NewRunner (D-05 thin ctor) + sessionFor + advisory + spawnMCP/mergeMCPServers + serve-composition quartet |
| `internal/runtime/cron_wiring.go` | Cron wiring byte-verbatim | ✓ EXISTS + SUBSTANTIVE | git-mv, package clause + receiver sed only; startScheduler stays unexported |
| `internal/runtime/enginebridge/enginebridge.go` | Engine/MCP adapter bridge | ✓ EXISTS + SUBSTANTIVE | EngineTurnAdapter, ACPDispatcher, hook runners, StubCatalogExec over BridgeConfig (D-14) |
| `internal/acpserve/acp_serve.go` | One-function serve composition | ✓ EXISTS + SUBSTANTIVE | Run reproduces source :320-425 statement order incl. WINDOWS #3 emitter-before-scheduler |
| `cmd/ass-guard/acp_serve.go` | Cobra shells only | ✓ EXISTS + SUBSTANTIVE | newACPCmd/newACPServeCmd/runACPServeCmd, ~120 lines, delegates to acpserve.Run |
| `cmd/ass-guard/cli_contract_test.go` | CLI-contract golden | ✓ EXISTS + SUBSTANTIVE | 3 test functions pinning command + flag sets |

**Artifacts:** 6/6 verified

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| cmd runACPServeCmd | acpserve.Run | thin shell delegation | ✓ WIRED | runACPServeCmd builds Options from flags, calls acpserve.Run with os.Stdin/os.Stdout/os.Stderr |
| acpserve.Run | runtime.NewRunner | RunnerConfig fill | ✓ WIRED | Single construction site; LoadCommandRegistry + engine-degrade branch + NewServer(WithTurnRunner) follow |
| acpserve.Run | runner lifecycle quartet | SetSchedule/SetEmitter/StartScheduler/CloseAllSessions | ✓ WIRED | Emitter set before scheduler start (WINDOWS #3); ctx-done goroutine reaps sessions |
| runtime.Runner.Run | enginebridge adapters | BridgeConfig func values | ✓ WIRED | runOneTurn wires Invoke/Expand/AutomationProvenance; sites 2+3 pass empty config (branch cannot run) |

**Wiring:** 4/4 connections verified

## Requirements Coverage

| Requirement | Status | Blocking Issue |
|-------------|--------|----------------|
| RUNT-01: `sessionTurnRunner` carved verbatim into `internal/runtime`; cmd composes it; zero behavior change (`mise ci` proves equivalence) | ✓ SATISFIED | - |

**Coverage:** 1/1 requirements satisfied

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/runtime/enginebridge/enginebridge.go | EngineTurnAdapter.Run | Invoke⇒Expand nil-pairing latent hazard (WR-01 in 15-REVIEW.md) | ⚠️ Warning | No live path panics (all 3 sites pair or pass empty config); a future caller setting Invoke without Expand nil-derefs one turn in. Recommended follow-up guard, deliberately not made in this pure-relocation phase |
| internal/runtime/goconst_constants.go | - | Test-only constants in production file (IN-01) | ℹ️ Info | Mirrors pre-carve layout; optional cleanup |
| (multi-file) | - | Deliberate D-03 constant duplication (IN-02) | ℹ️ Info | Recorded so future review does not "fix" into shared coupling |

**Anti-patterns:** 3 found (0 blockers, 1 warning, 2 info)

## Human Verification Required

### 1. Live-Zed editor-session identity

**Test:** Spawn `ass-guard acp serve` from Zed exactly as in daily use; send a prompt; exercise a tool call (file read/edit); restart Zed mid-session. Full checklist: phase-review.md "Operator live-Zed checklist".
**Expected:** Token streaming renders natively (session/update chunks); tool diffs render natively and results return; restart replay matches v1.1-close behavior (session/load no-op, transcripts remain under `.ass-guard/`).
**Why human:** Editor-rendered perceptual comparison against the operator's daily-use memory of v1.1 close; no automated harness renders Zed's UI surface.

## Gaps Summary

**No critical gaps found.** All programmatically verifiable must-haves pass (truths 1, 2, 4; RUNT-01; artifacts 6/6; wiring 4/4). One human-verification item remains (truth 3) — dispositioned by design: PENDING-OPERATOR-CONFIRMATION marker in 15-07-SUMMARY.md, WINDOWS ledger entry blocking `/gsd-ship`, operator checklist in phase-review.md.

## Verification Metadata

**Verification approach:** Goal-backward (derived from ROADMAP.md goal + success criteria)
**Must-haves source:** ROADMAP.md Phase 15 success criteria (3 criteria; criterion 2 split into automated-exercised vs live-editor identity)
**Automated checks:** mise ci (exit 0), ledger comparison (829/134 exact), color-moved census (52 files, 0 non-sanctioned), CLI-contract + zero-config tests — all passed, 0 failed
**Human checks required:** 1
**Total verification time:** 5 min

---
*Verified: 2026-08-26*
*Verifier: inline executor (sequential execution mode — no subagent dispatch)*
