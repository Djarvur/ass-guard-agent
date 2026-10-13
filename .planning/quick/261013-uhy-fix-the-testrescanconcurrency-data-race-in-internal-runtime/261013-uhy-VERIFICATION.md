---
phase: quick-261013-uhy
verified: 2026-10-13T23:05:00Z
status: passed
score: 3/3 must-haves verified
covered_files:
  - ".github/workflows/nightly.yml"
  - ".planning/quick/261013-uhy-fix-the-testrescanconcurrency-data-race-in-internal-runtime/261013-uhy-PLAN.md"
  - ".planning/quick/261013-uhy-fix-the-testrescanconcurrency-data-race-in-internal-runtime/261013-uhy-SUMMARY.md"
  - "kit/runtime/fakes_test.go"
covered_digest: "v1:sha256:d77085399a9fb78d617c3b0609418953050ad9f4dbf07e414c9ea9d6d83cbfd6"
behavior_unverified: 0
overrides_applied: 0
---

# Quick Batch 261013-uhy: Fix the TestRescanConcurrency data race — Verification Report

**Item Goal:** Fix the TestRescanConcurrency data race (kit/runtime/rescan_test.go — spawnMCP reading vs installRegistry writing, pre-existing before 22-04, WINDOWS #21) following the ab7956b fakeLearned mutex-guard precedent, then remove the TestRescanConcurrency entry from the nightly test skip list (WINDOWS #27).
**Verified:** 2026-10-13T23:05:00Z
**Status:** passed
**Re-verification:** No — initial verification
**Commits verified:** 01ba3a4 (fakes_test.go), 40bd483 (nightly.yml)

## Goal Achievement

All verification gates were reproduced independently by the verifier, not taken from SUMMARY claims. The plan's two evidence-backed path corrections are confirmed against the codebase: the test lives at `kit/runtime/rescan_test.go:222` (`internal/runtime/` does not exist — Phase-25 kit extraction), and the committed workflow is `.github/workflows/nightly.yml` (no `nightly-parity.yml` exists in `.github/workflows/`).

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | TestRescanConcurrency passes under -race repeatedly — three consecutive -count runs — at the plan head | ✓ VERIFIED | Verifier's own run at HEAD (40bd483): `go test -race -count=3 -run '^TestRescanConcurrency$' ./kit/runtime/` → `ok ... 5.221s`, zero race reports. Behavior-dependent truth with direct behavioral evidence. |
| 2 | Every fakeCatalog accessor touching a canned table holds t.mu; the Hooks read vs setHooks write is closed per the ab7956b shape | ✓ VERIFIED | Exhaustive audit of all 27 receivers (grep-confirmed all live in fakes_test.go, none elsewhere in kit/). `Hooks` now opens with `t.mu.Lock()` + `defer t.mu.Unlock()` (fakes_test.go:562–567); `setHooks` guarded (571–576). Guarded: Rediscover, Entries, LookupCommand, SkillBody, Mutating, Agents, SkillListing, AgentListing, MCPLayers, Counts, plantAgent, plantSkills, skill, agent, SkillExecute (inside its closure, 590–596). The 8 `load*`/`reload` helpers touch shared maps only under the constructor (pre-publication, fakes_test.go:159) or `Rediscover`'s lock (453→460) — grep confirmed zero other call sites. `MemoryInjection`/`MemoryFiles` read only the filesystem (os.ReadFile of the dir parameter) and touch zero canned tables, so the truth's predicate excludes them — the deliberate-lock-free note is factually correct. Guard contract recorded in the fake's doc comment (fakes_test.go:90–92). Corroborated by full-package `go test -race -count=1 ./kit/runtime/` → `ok ... 47.203s` (verifier's own run). |
| 3 | The nightly -race leg runs the stress test again: -skip regex and reasons comment no longer name it; every still-failing documented entry keeps its skip and stated reason | ✓ VERIFIED | Comment-filtered `grep -c RescanConcurrency` on nightly.yml = 0 (raw grep also empty). Line 67 regex is exactly `^TestRunSuite_\|^TestZedSimulatorE2E$\|^TestSimulatorCommandSurface$\|^TestEscalation_ReapAllUsesLadder$` — the four documented survivors (acpserve simulator pair, openspec-binary battery, escalation flake). Reasons comment (lines 54–66) names each survivor with its reason; the REMOVE-entries-as-failures-are-fixed instruction is intact. `ruby -ryaml` parse passes. No other file under `.github/` references RescanConcurrency. |

**Score:** 3/3 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `kit/runtime/fakes_test.go` | Hooks accessor mutex-guarded (t.mu) + guard contract in doc comment | ✓ VERIFIED | Commit 01ba3a4 (+7 lines, only file touched): Hooks lock at 563–564, doc comment contract at 90–92. Wired — consumed by the package's test batteries through the green -race suite. |
| `.github/workflows/nightly.yml` | Stress-test token removed from -skip regex; pre-existing-race clause dropped from reasons comment | ✓ VERIFIED | Commit 40bd483 (+5/−6, only file touched): regex token gone (line 67), comment reflowed coherently, REMOVE instruction verbatim. Working tree clean for both files (HEAD = 40bd483). |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| spawnMCP (kit/runtime/runtime.go:3159) | fakeCatalog.MCPLayers | `r.liveCatalog().MCPLayers()` at runtime.go:3164 | ✓ WIRED | MCPLayers is t.mu-guarded (fakes_test.go:764–766) — the original race's read half, synchronized via the Phase-25 catalog injection, confirmed in code. |
| rescanAndSwap (kit/runtime/rescan.go:306) | fakeCatalog.Rediscover → installCatalog → chainPtr.Store | `r.liveCatalog().Rediscover()` then `r.installCatalog()` (rescan.go:309–313) | ✓ WIRED | Rediscover t.mu-guarded (fakes_test.go:452–454); chainPtr.Store at commands.go:505, Load at 482 — the write half synchronized + atomically swapped. |
| .github/workflows/nightly.yml -skip regex | the discharged entry (WINDOWS #27) | skip-list alternation token | ✓ WIRED | `^TestRescanConcurrency$` token removed; entry discharged; survivors verbatim. |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Sustained race gate (truth 1) | `go test -race -count=3 -run '^TestRescanConcurrency$' ./kit/runtime/` | `ok ... 5.221s` | ✓ PASS |
| Full-package -race (truth 2 corroboration) | `go test -race -count=1 ./kit/runtime/` | `ok ... 47.203s` | ✓ PASS |
| gofmt / vet gates (Task 1) | `gofmt -l kit/runtime/` ; `go vet ./kit/runtime/` | empty ; clean | ✓ PASS |
| Skip-list discharge (truth 3) | comment-filtered grep count; `TestRunSuite_` survivor grep | 0 ; present | ✓ PASS |
| Workflow parses as YAML | `ruby -ryaml -e 'YAML.load_file(...)'` | exit 0 | ✓ PASS |

### Probe Execution

No `scripts/*/tests/probe-*.sh` probes declared or conventionally applicable to this item. The plan's own verify chains (reproduced above) are the behavioral gates.

### Requirements Coverage

No `requirements:` field in the PLAN frontmatter. The item's WINDOWS #21/#27 ledger row discharge is explicitly assigned to sibling item 261013-uhz ("Do not touch .planning/WINDOWS.md") — neither item commit touches `.planning/WINDOWS.md` (01ba3a4 → fakes_test.go only; 40bd483 → nightly.yml only), satisfying the plan's scope rule. The working-tree modification to WINDOWS.md belongs to uhz's in-flight work and is outside this item.

### Scope Verification (no production change)

| Commit | Files | Production risk |
|--------|-------|-----------------|
| 01ba3a4 | `kit/runtime/fakes_test.go` (+7) | None — `_test.go` test glue only |
| 40bd483 | `.github/workflows/nightly.yml` (+5/−6) | None — CI config only |

Exactly the two declared `files_modified`; zero production `.go` files. The original production race pair is confirmed structurally gone in kit form: the read half routes through t.mu-guarded `MCPLayers`, the write half through t.mu-guarded `Rediscover` + atomic `chainPtr.Store`.

### Anti-Patterns Found

None. No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER markers in either modified file; no residual RescanConcurrency references anywhere under `.github/`.

### Human Verification Required

None. Every truth carries direct automated evidence reproduced by the verifier at HEAD; no ⚠️ PRESENT_BEHAVIOR_UNVERIFIED items. (The next hosted nightly run exercising the unskipped test is the gate's own routine operation, outside the item's verification contract — the plan's gates cover the local sustained proof plus the static skip-list state, both green.)

### Gaps Summary

None. The item's full letter is discharged and independently verified: the last unsynchronized fakeCatalog accessor (Hooks) is guarded per the ab7956b shape with the sweep audit confirming t.mu coverage of every canned-table accessor, the stress test is sustained-green under -race at plan head (reproduced 3/3 plus a full-package -race pass), and the nightly skip-list entry is gone with all remaining documented entries and reasons byte-intact and the workflow parsing as YAML.

---

_Verified: 2026-10-13T23:05:00Z_
_Verifier: Claude (gsd-verifier)_
