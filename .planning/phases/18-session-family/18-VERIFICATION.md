---
phase: 18-session-family
verified: 2026-09-03T17:24:03Z
status: human_needed
score: 7/7 must-haves verified
behavior_unverified: 0 # every behavior-dependent truth has a passing named test the verifier ran itself
overrides_applied: 0
human_verification:
  - test: "Zed-side session family UAT (18-04 coverage D6): open the editor's session picker, pick a past session, close a session mid-turn, delete one"
    expected: "Picker lists past sessions with titles/updatedAt; opening one replays the full conversation; close stops a mid-turn session cleanly; delete makes the session vanish from the list while transcript + audit remain on disk under .ass-guard/"
    why_human: "Live-editor rendering and interaction — no automation asserts Zed's rendering of the v1 wire shapes; the engine, RPC, capability advertisement, and replay ordering are machine-verified"
  - test: "Interactive TTY/ssh picker check (18-06): run `ass-guard --resume` from a real terminal over ssh with past sessions in the store"
    expected: "Numbered rows render readably on a real terminal, number selection resolves the target, invalid entry re-prompts once, EOF/second invalid exits non-zero with a clear error"
    why_human: "The pipe-driven smoke covers the D-11 I/O mechanics (stdin line read, stderr rows) but not interactive terminal behavior over ssh; no raw-mode dependency exists but real-TTY rendering is unasserted"
  - test: "Operator acknowledgment of the 18-06 documented deviation: `--resume <id|name>` resolution is cwd-scoped (per-directory store), not a cross-project registry like Claude Code's"
    expected: "Operator accepts cwd-scoped resolution (clear not-found error naming the directory searched) or rules cross-project resolution required (new registry store design, a scope decision beyond Phase 18)"
    why_human: "Explicit OPERATOR SIGN-OFF PENDING in the 18-06 plan deviation note — a product scope decision, not machine-checkable; roadmap criterion 3's 'works anywhere' was glossed as CLI-flag availability, which is delivered"
---

# Phase 18: Session Family Verification Report

**Phase Goal:** Sessions become first-class objects the editor can enumerate and restore: list with pagination, load/resume with full replay plus live-state reconciliation (the hard part — dangling expectations get synthetic closure), and close/delete with tombstoning preserving the D-20 audit invariant.
**Verified:** 2026-09-03T17:24:03Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

Machine-verified against the codebase and named tests the verifier executed itself (not SUMMARY claims). All test runs used `-race -count=1`.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC-1/ACP-05: sessions enumerable with header-scan + composite-cursor pagination + tombstone filter; opening one replays the full conversation through the same ordered emitter path as live turns (updates-before-response) | ✓ VERIFIED | `internal/session/list.go` (`ListSessions`:107, cursor codec:449-476, bounded-prefix header scan); handler wired `handlers.go:889`; replay through `s.Emitter(sessionID)` `handlers.go:794`; Barrier before response `handlers.go:811`. Tests run green: `TestSessionList*` (8 fns), `TestHandleSessionListRoundTrip`, `TestHandleSessionListEmptyStore`, `TestSessionLoadReplaysCleanSession`, `TestReplayTranscriptMapping` |
| 2 | SC-2a/ACP-06: resumed ids continue from transcript maxima; commands re-advertise after replay; modes carry the seeded plan-mode state | ✓ VERIFIED | `MaxTurnCounter` (seed.go:26), `SeedResume` (session.go:186), `ResumeSession` seeds (runtime.go:1051-1104); `available_commands_update` re-fire after last replay frame (handlers.go:799-806; ordering pinned replay_test.go:553-557); `LoadedModes`→response modes (handlers.go:781-786, 848-849). Tests green: `TestResumeSessionReconcilesAndSeeds`, `TestResumeSessionSeedsPlanMode`, kill -9 matrix commands-assertions (kill9_test.go:608) |
| 3 | SC-2b/ACP-06: kill -9 mid-turn + resume leaves NO ghost state — dangling tool_calls closed failed, parked asks/pending permissions resolved synthetically; idempotent; provenance-marked appended closures; zero provider calls during resume | ✓ VERIFIED | `Reconcile` + `Seed` + `InterruptedCause` (reconcile.go:29,106); `AppendSynthetic` (manager.go:389); `TestKill9Resume` E2E re-run by verifier: real serve child, REAL SIGKILL, matrix classes 1,2,3,4,5+plan-mode,8,9 + `ask_interplay_queued_asks` + `kill_during_replay` rows — ok 25.4s; provider-count assertions (kill9_test.go:1417-1429, 1559); `TestReconcile`, `TestReconcileIdempotent`, `TestReconcileClean`, `TestReconcileSkipsGarbageTail` green |
| 4 | SC-3/ACP-06: `ass-guard --resume` works anywhere as a CLI flag — persistent root flags, picker, `--continue`/`-c`, serve injection through the SAME load engine | ✓ VERIFIED | Persistent flags on root (main.go:128-133, NoOptDefVal picker sentinel); verifier ran `go run ./cmd/ass-guard --help` — both flags advertised; `Options.ResumeTarget` (options.go:32) consumed BEFORE Serve via `srv.LoadSession` (acp_serve.go:349-355, loud fatal on failure); picker `SelectSession` (picker.go). Tests green: `TestResumeFlagParsing`, `TestResumeWinsOverContinue` (WR-04 pin), `TestPicker*` (5 fns), `TestContinueResolvesMostRecentCwd`, `TestResumeTargetLoadsBeforeServe` |
| 5 | SC-4/ACP-07: close stops work cleanly (cancel-and-drain, idempotent); delete tombstones (never rm), removes checkpoints, preserves D-20 audit invariant; deleted sessions vanish from list but stay investigable; grace-bounded startup sweep | ✓ VERIFIED | `closeSessionSequence` (handlers.go:950); `handleSessionDelete` marker-first + checkpoint removal + best-effort partial-failure (handlers.go:1014-1080); `Tombstone`/`SweepTombstones`/`DefaultTombstoneGrace` (tombstone.go:46,97,32); startup sweep wired (acp_serve.go:223) behind `tombstoneGraceDays` (config_surface.go:51). D-20 grep (below): the ONLY `os.Remove` on transcripts in production code is SweepTombstones' strictly-past-grace branch (tombstone.go:172); sweep never enumerates the audit subtree. Tests green: `TestSessionCloseCancelsAndDrains`, `TestSessionCloseAlreadyClosed`, `TestSessionDeleteTombstones`, `TestTombstoneWrite`, `TestSweepRespectsGrace`, `TestSweepNeverTouches`, `TestSweepIdempotent` |
| 6 | Review pins: CR-01 (delete-during-load resurrection) + WR-01..04 fixed with RED-first pins | ✓ VERIFIED | All five fixes read in code: CR-01 tombstone re-stat + insert in ONE `s.mu` section (handlers.go:819-838) + marker-before-close (handlers.go:1049-1057); WR-01 turn-mutex over ReadAll→Reconcile→AppendSynthetic→SeedResume (runtime.go:1066-1072); WR-02 `registerTurn` identity re-check (handlers.go:560-579, called :477); WR-03 `CloseSession` evicts under `sessMu` before Close (runtime.go:1715-1722); WR-04 `--resume` wins (acp_serve.go:107-121). All five pin tests enumerated AND run green: `TestSessionDeleteDuringLoadNeverResurrects` (substantive parked-replay race construction, session_family_test.go:1474-1530), `TestResumeSessionSerializesWithTurnMutex`, `TestPromptRegistrationAbortsAfterCloseReaped`, `TestInProcessReloadAfterCloseRebuildsSession`, `TestResumeWinsOverContinue` |
| 7 | Capability advertisement: `loadSession: true` top-level + nested `sessionCapabilities {list, close, delete}`; v1 no-replay `session/resume` NOT advertised; prompt-during-replay typed-rejected; tombstoned/unknown/traversal ids typed-rejected; double-load typed errors | ✓ VERIFIED | handlers.go:208-221 (advertisement), :730-760 (already-active/already-loading typed errors), D-03 gate via `s.loading` + `promptSessionState` (:517). Tests green: `TestInitializeAdvertisesSessionCapabilities`, `TestLoadGateRejectsPromptDuringReplay`, `TestSessionLoadTombstonedRejected`, `TestSessionLoadUnknownIDRejected`, `TestSessionLoadTraversalIDRejected`, replay_test.go:633-670 (already-active) |

**Score:** 7/7 truths verified (0 present-but-behavior-unverified — every state-transition/race invariant truth is exercised by a named test the verifier ran)

### Prohibition Checks (judgment-tier, autonomous LLM-judge verdicts WITH enforcement evidence)

All seven plan prohibitions verified — each has concrete machine-checkable evidence, so none is flagged `unverified-prohibition`:

| Prohibition | Verdict | Evidence |
|-------------|---------|----------|
| Replay fabricates nothing — every frame maps a transcript line | VERIFIED | replay.go maps transcript lines only; closures replay from reconciliation lines appended to the transcript first (D-03: reconcile inside ResumeSession, replay reads post-closure transcript) |
| No provider/LLM call during replay/reconciliation | VERIFIED | kill -9 harness counting-stub asserts call-count unchanged across resume (kill9_test.go:1417-1429, 1559) — ran green |
| session/list scope bounded to cwd `.ass-guard/` store | VERIFIED | handlers.go:889 passes `s.storeWorkDir()`; list.go scans that dir only |
| Titles derive only from first user prompt (truncated) | VERIFIED | list.go bounded prefix stops at first user_message; `TestSessionListHeaderShape` green |
| Delete/sweep never remove or rewrite audit records | VERIFIED | Sweep enumerates only `<id>.deleted` markers in the store dir (tombstone.go:116 — "audit subtree… never enumerated"); no delete/sweep code path touches the audit package |
| No physical purge before grace elapses | VERIFIED | Strictly-past-grace branch (tombstone.go:167); `TestSweepRespectsGrace` green |
| Sweep removal surface bounded to tombstoned pairs in-store | VERIFIED | `sweepMarkerID` validation + non-regular-file skip (planted-link guard); `TestSweepNeverTouches` green |

Note: `internal/audit/bodystore.go:172` contains an `os.Remove` — it is the audit store's pre-existing byte-cap eviction (oldest-first over cap), introduced by the audit phase, NOT coupled to session deletion or grace expiry. Not a D-20 violation; recorded as INFO.

### D-20 Audit Invariant Grep (re-run by verifier)

`grep -rn "os.Remove\|os.RemoveAll" internal/ cmd/` (production code): 17 sites. Transcript-path removals: exactly ONE — `internal/session/tombstone.go:172`, inside `sweepPair`'s `now.Sub(mfi.ModTime()) > grace` branch (strictly past grace), marker removed after. All other sites are tmp-file/lock-file/scratch-workspace cleanup in unrelated packages (providerfactory, checkpoint lock, perm store, parity scratch, sched, evalharness, audit cap-eviction). **Invariant holds: never rm on delete; only grace-expired GC removes transcripts.**

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/session/reconcile.go` | Reconcile + Seed + InterruptedCause | ✓ VERIFIED | 330 lines, pure classifier over ten-row inventory |
| `internal/session/testdata/reconcile/` (9 fixtures) | one per inventory row (class-10 folded into class-3 per plan decision) | ✓ VERIFIED | class01..class09 present |
| `internal/session/list.go` | ListSessions + SessionHeader + cursor codec | ✓ VERIFIED | 481 lines; encodeCursor/decodeCursor unexported |
| `internal/session/tombstone.go` | Tombstone + SweepTombstones + DefaultTombstoneGrace | ✓ VERIFIED | 226 lines |
| `internal/session/seed.go` / `session.go` / `manager.go` | MaxTurnCounter / SeedResume / AppendSynthetic | ✓ VERIFIED | seed.go:26, session.go:186, manager.go:389 |
| `internal/acp/replay.go` | ReplayTranscript through emitter | ✓ VERIFIED | 319 lines, called with `s.Emitter(sessionID)` |
| `internal/acp/handlers.go` / `types.go` / `server.go` | load/list/close/delete handlers, v1 shapes, SessionLoader + WithWorkDir | ✓ VERIFIED | All handlers present; type-assert handlers.go:772; WithWorkDir server.go:339 |
| `internal/acpserve/kill9_test.go` | TestKill9Resume real-SIGKILL matrix | ✓ VERIFIED | 1602 lines; re-ran green (25.4s) |
| `internal/runtime/runtime.go` | ResumeSession full resume contract | ✓ VERIFIED | :1051-1104, turn-mutex held |
| `cmd/ass-guard/picker.go` / `main.go` / `acp_serve.go` | SelectSession, persistent flags, target resolution | ✓ VERIFIED | Picker 128 lines; flags main.go:128-133 |
| `internal/acpserve/options.go` / `acp_serve.go` | ResumeTarget + startup sweep + load-before-Serve | ✓ VERIFIED | options.go:32; acp_serve.go:223, 349-355 |

### Key Link Verification

| From | To | Via | Status |
|------|----|----|--------|
| handlers.go (list) | session/list.go | `ListSessions(workDir, cursor, 0)` (handlers.go:889) | ✓ WIRED |
| handlers.go (load) | runtime.ResumeSession | `SessionLoader` type-assert (handlers.go:772) before replay | ✓ WIRED |
| runtime.go | session/reconcile.go | ReadAll → Reconcile → AppendSynthetic → SeedResume (runtime.go:1074-1102) | ✓ WIRED |
| replay.go | server emitter | `ReplayTranscript(s.Emitter(sessionID), ...)` (handlers.go:794) | ✓ WIRED |
| acpserve.Run | tombstone.go | `SweepTombstones` at startup (acp_serve.go:223) | ✓ WIRED |
| acpserve.Run | handlers.LoadSession | ResumeTarget → `srv.LoadSession` before Serve (acp_serve.go:349-355) | ✓ WIRED |
| cmd/ass-guard (picker/continue) | session/list.go | `ListSessions` ×3 (acp_serve.go:135,155,189) | ✓ WIRED |
| acpserve composition | server.go | `WithWorkDir(opts.WorkDir)` (acp_serve.go:256) | ✓ WIRED |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Reconcile/list/tombstone batteries | `go test -race -count=1 -run 'TestReconcile\|TestSessionList\|TestTombstone\|TestSweep' ./internal/session/` | ok 2.597s | ✓ PASS |
| Replay + load + CR-01/WR-02 pins | `go test -race -count=1 -run 'TestReplay\|TestSessionLoad\|TestSessionDeleteDuringLoad\|TestPromptRegistration' ./internal/acp/` | ok 2.037s | ✓ PASS |
| List/close/delete RPC + advertisement + load gate | `go test -race -count=1 -run 'TestHandleSessionList\|TestInitializeAdvertisesSessionCapabilities\|TestSessionClose\|TestSessionDelete\|TestLoadGateRejectsPromptDuringReplay' ./internal/acp/` | ok 2.597s | ✓ PASS |
| Resume + WR-01/WR-03 pins | `go test -race -count=1 -run 'TestResumeSession\|TestInProcessReload' ./internal/runtime/` | ok 2.516s | ✓ PASS |
| kill -9 matrix E2E (real SIGKILL) | `go test -race -count=1 -run 'TestKill9Resume' ./internal/acpserve/` | ok 25.419s | ✓ PASS |
| Serve injection | `go test -race -count=1 -run 'TestResumeTarget' ./internal/acpserve/` | ok 6.044s | ✓ PASS |
| CLI flags + picker + WR-04 pin | `go test -count=1 -run 'TestResumeWinsOverContinue\|TestResumeFlagParsing\|TestResumeTargetResolution\|TestPicker\|TestContinueResolves' ./cmd/ass-guard/` | ok 0.783s | ✓ PASS |
| Root help advertises flags | `go run ./cmd/ass-guard --help` | `-c, --continue` and `--resume string[="@picker"]` present | ✓ PASS |
| Build + vet | `go build ./...` + `go vet` (5 touched pkgs) | clean / clean | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| ACP-05 | 18-03, 18-04 | session/list via header-scan + cursor pagination | ✓ SATISFIED | List engine + RPC + advertisement (Truths 1, 7) |
| ACP-06 | 18-01, 18-02, 18-05, 18-06 | session/load full replay + reconciliation + `--resume` anywhere | ✓ SATISFIED | Truths 2, 3, 4 (+cwd-scoped resolution deviation — human item 3) |
| ACP-07 | 18-04 | session/close + session/delete with tombstoning (never rm, D-20) | ✓ SATISFIED | Truth 5 + D-20 grep |

No orphaned requirements: REQUIREMENTS.md maps exactly ACP-05/06/07 to Phase 18, all claimed by plans.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER in any of the 16 phase-modified production files | - | - |

INFO (inherited, out of phase scope): review findings IN-01 (CLI replay frames precede initialize), IN-02 (sweep is startup-only), IN-03 (logout skips cancel-and-drain), IN-04 (stray positional ignored with `--continue`) — left open by explicit scope decision in 18-REVIEW.md; none blocks the phase goal. Known environment issue (NOT a gap): `mise ci` lint step red from pre-existing golangci-lint 2.12.2/go1.27 version drift (WINDOWS ledger, deferred-items.md) — tests/vet/build are the gate and are green.

### Human Verification Required

1. **Zed-side session family UAT (18-04 D6)** — open the editor's session picker, pick a past session, close one mid-turn, delete one.
   **Expected:** picker lists sessions; opening replays the conversation; close stops work cleanly; delete removes from list while transcript + audit remain on disk.
   **Why human:** live-editor rendering of the v1 wire shapes — no automation asserts Zed's rendering.

2. **Interactive TTY/ssh picker check (18-06)** — run `ass-guard --resume` on a real terminal over ssh with past sessions present.
   **Expected:** readable numbered rows, number selection works, one re-prompt on invalid input, EOF/second invalid exits non-zero.
   **Why human:** pipe-smoke covers D-11 mechanics; real-TTY interactivity is unasserted.

3. **Operator acknowledgment of the 18-06 documented deviation** — `--resume <id|name>` resolves cwd-scoped (per-directory store), not cross-project like Claude Code's registry.
   **Expected:** operator accepts (not-found error names the searched directory) or rules cross-project resolution required (new scope decision).
   **Why human:** explicit OPERATOR SIGN-OFF PENDING in the 18-06 plan; a product scope decision, not machine-checkable.

### Gaps Summary

No gaps. All 7 merged must-have truths verified with code evidence and named tests the verifier executed itself (including the kill -9 matrix E2E against a real serve process and all five review-fix RED-first pins). All artifacts exist, are substantive, and are wired. All three requirements satisfied; no orphans. No debt markers. The D-20 audit invariant holds by exhaustive `os.Remove` grep. Status is `human_needed` solely for the three operator legs above — machine verification is complete and green.

---

_Verified: 2026-09-03T17:24:03Z_
_Verifier: ZCode (gsd-verifier)_
