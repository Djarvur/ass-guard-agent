---
phase: 22-background-execution-sandbox-reality
plan: 08
subsystem: runtime
tags: [background-execution, wake-drain, session-lifecycle, goroutine-hygiene, concurrency, tdd, gap-closure]

# Dependency graph
requires:
  - phase: 22-background-execution-sandbox-reality (22-07)
    provides: releaseSubagentSlot + cancel-entry retirement in internal/tasks/tracker.go (the file CancelRunning joins), the finished-vs-running signal substrate
  - phase: 22-background-execution-sandbox-reality (22-01)
    provides: the ONE wake-drain infrastructure (tracker, scheduleWakeDrain, wakeDrainChain, drainWakeNotifications) whose lifecycle this plan closes
provides:
  - G-22-2 (CR-02) closed — the wake-drain chain IDLES on an empty queue (zero chain goroutines per quiesced session; no successor spin) and refuses to spawn past serve shutdown
  - G-22-4 (CR-04) closed — session close is terminal for the background machinery: running subagents cancelled, per-session wake state pruned, late completions drop loudly via a non-constructing lookup (no ghost provider turn, no session resurrection)
  - Tracker.CancelRunning() — the close-time running-cancel leg (beside CancelQueued), idempotent with releaseSubagentSlot
  - IN-05 retired incidentally — the unbounded stay-pending retry's only trigger (sessionFor construction failure) is gone; a missing session is a terminal drop
affects: [22-background-execution-sandbox-reality (22-09), internal/runtime consumers, internal/tasks consumers]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
# Same estimateTokens scale (chars/4 over the realized diff), never a harness token count.
actuals:
  tokens: 7101   # chars/4 over the realized diff (28,406 chars across 4 files, internal/ only)
  tasks: 2
  commits: 4     # MEASURED: git rev-list --count 1c5f0d77..HEAD

# Tech tracking
tech-stack:
  added: []      # zero new deps; stdlib only
  patterns:
    - "gated-restart exit defer: flag.Store(false) FIRST, then restart only on (ctx.Err()==nil AND tracker non-nil AND pending non-empty) — the completion landing after the peek carries itself via its own CAS on the cleared flag; the gate covers only the peek→store window"
    - "non-constructing session lookup at consumption sites: a drain must never CREATE the session it delivers into (sessMu-held r.sessions read; miss = loud terminal drop), keeping sessionFor's constructing power at demand-driven paths only"
    - "close-prunes-state ordering: per-session map deletes run AFTER s.Close() returns, so OnClose's cancels and PTY Drain execute while the tracker/manager are still discoverable"
    - "sampled-window witnesses for spin bugs: the CR-02 spin holds its flag true only ~86% of reads under -race (measured), so lifecycle assertions sample >=25 times over >=500ms instead of reading once"

key-files:
  created: []
  modified:
    - internal/runtime/cron_wiring.go
    - internal/runtime/wake_wiring_test.go
    - internal/runtime/runtime.go
    - internal/tasks/tracker.go

key-decisions:
  - "scheduleWakeDrain's shutdown guard reads serveCtxOrBackground().Err() — test runners on the Background fallback are unaffected by construction (Err() always nil), so no test seam was needed"
  - "The exit defer re-derives the tracker (the body's tr may be nil on the early return) and keeps flag.Store(false) strictly before the gated scheduleWakeDrain — the ordering is the race contract"
  - "drainWakeNotifications resolves sessions NON-constructingly (sessMu-held r.sessions lookup) even though CloseSession now deletes the tracker: the tracker-registered-but-session-absent state is reachable (sessionFor failing midway stores the tracker before the session; a completion racing the close's map operations) and the drain must never be the reconstructing path"
  - "CancelRunning clears the whole cancels map and returns the count — idempotent with Complete's releaseSubagentSlot (whichever runs first wins, the other finds nothing); queued waiters stay CancelQueued's job"
  - "CloseSession's deletes live AFTER s.Close() in ONE unconditional site (also the session-missing path) — the grep-visible contract is exactly one trackers.Delete / wakeInFlight.Delete / ptyManagers.Delete in runtime.go"

patterns-established:
  - "Delta-band goroutine assertions [b, b+2] with a settle-before-baseline sleep (construction transients like the catch-up no-op goroutine would otherwise decay the count below b)"
  - "RED duty-cycle analysis for spin-class bugs: a tight-loop probe of the buggy invariant (here: flag true/false read counts) proves the sampled window catches the RED state deterministically before committing the RED battery"

requirements-completed: [PAR-07, PAR-08]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "G-22-2 (CR-02): the ONE wake-drain chain is lifecycle-sound — after a delivered batch the chain exits with NO successor (idle means idle, zero chain goroutines per quiesced session); scheduleWakeDrain refuses to spawn past serve shutdown; a chain mid-retry stops on ctx cancel without restarting; a racing completion landing in the flag-held window still delivers (the gating strands nothing)"
    requirement: PAR-07
    verification:
      - kind: unit
        ref: internal/runtime/wake_wiring_test.go#TestWakeChain_IdlesWhenEmpty
        status: pass
      - kind: unit
        ref: internal/runtime/wake_wiring_test.go#TestWakeChain_NoSpawnPastServeShutdown
        status: pass
      - kind: unit
        ref: internal/runtime/wake_wiring_test.go#TestWakeChain_CtxCancelStopsChain
        status: pass
      - kind: unit
        ref: internal/runtime/wake_wiring_test.go#TestWakeChain_RacingCompletionStillWakes
        status: pass
      - kind: command
        ref: "go test -race -count=3 ./internal/runtime/ -run 'TestWakeChain|TestWakeTurn' (flake guard: serial rows beside parallel batteries)"
        status: pass
    human_judgment: false
  - id: D2
    description: "G-22-4 (CR-04): session close is terminal for the background machinery — RUNNING background subagents are cancelled at close (counted note), trackers/wakeInFlight/ptyManagers entries pruned after OnClose's Drain, and a late completion drops its batch loudly (one stderr line naming session + count) through a non-constructing lookup: no ghost provider turn, nothing re-enters r.sessions, close-then-restore reconstruction still works"
    requirement: PAR-08
    verification:
      - kind: unit
        ref: internal/runtime/wake_wiring_test.go#TestWakeChain_ClosedSessionDropsBatch
        status: pass
      - kind: unit
        ref: internal/runtime/wake_wiring_test.go#TestCloseSession_PrunesWakeState
        status: pass
      - kind: unit
        ref: internal/runtime/pty_wiring_test.go#TestSessionClosePTY (unmodified, still green — PTY close drain preserved)
        status: pass
      - kind: unit
        ref: internal/runtime/resume_session_test.go#TestInProcessReloadAfterCloseRebuildsSession (unmodified, still green — close-then-restore reconstruction preserved)
        status: pass
      - kind: command
        ref: "go test -race -count=1 -skip 'TestRescanConcurrency' ./internal/runtime/ ./internal/tasks/ (full battery; -skip excludes only the deferred pre-existing rescan race, deferred-items.md)"
        status: pass
    human_judgment: false

# Metrics
duration: 25 min
completed: 2026-09-10
status: complete
---

# Phase 22 Plan 08: Gap closure — cron_wiring/runtime lifecycle cluster (G-22-2 wake-chain idle/shutdown + G-22-4 close-then-complete terminal) Summary

**Wake-drain chains now idle on empty queues and die with the serve ctx (gated exit defer + spawn refusal past shutdown), and session close is terminal for background machinery: running subagents cancelled, per-session state pruned, late completions dropped loudly instead of resurrecting ghost sessions — both TDD, red-verified then green**

## Performance

- **Duration:** 25 min
- **Started:** 2026-09-10T03:06:08Z
- **Completed:** 2026-09-10T03:31:29Z
- **Tasks:** 2
- **Files modified:** 4 (+2 RED-evidence records in .planning)

## Accomplishments

- **G-22-2 (CR-02) closed:** `wakeDrainChain`'s exit defer no longer restarts unconditionally — it clears the in-flight flag FIRST, then restarts only when the serve ctx is live, the tracker exists, and pending is non-empty. The permanent per-session successor spin (one goroutine chain re-spawning within microseconds of every exit, for process lifetime, through and past serve shutdown) is gone. `scheduleWakeDrain` additionally refuses to spawn once the serve ctx is done. The gated restart still carries a completion landing between the empty-queue peek and the flag store (pinned by TestWakeChain_RacingCompletionStillWakes).
- **G-22-4 (CR-04) closed:** `drainWakeNotifications` resolves the session through a NON-constructing `sessMu`-held `r.sessions` lookup — a missing session is terminal: one stderr line naming the session id and the dropped count, the batch consumed via `tr.Drain()`, the chain exits. The constructing `sessionFor` (which rebuilt a full session — MCP host, writer, PTY — and ran a real ghost provider turn into a session the operator closed) is gone from the drain; its only cron_wiring call site remains `runAutomationTurn`. `CloseSession` now cancels RUNNING background subagents (`tracker.CancelRunning()`, counted note beside the queued-drop note) and prunes `r.trackers`/`r.wakeInFlight`/`r.ptyManagers` after `s.Close()` returns.
- **IN-05 retired incidentally:** the unbounded stay-pending retry loop (one stderr line per 500ms forever) had exactly one trigger — `sessionFor` construction failure — and that path is now the terminal drop.
- **Strictly additive on pinned paths:** coalescing/cap notes (full internal/tasks battery), busy-turn accumulation (TestWakeTurn_BusyClientTurnAccumulates), background-bash wake delivery (TestWakeTurn_BackgroundBashCompletion), PTY close drain (TestSessionClosePTY), and close-then-restore reconstruction (TestInProcessReloadAfterCloseRebuildsSession) all pass unmodified; the mixed serial+parallel wake battery is stable at `-count=3 -race`.

## Task Commits

Each task was committed atomically (TDD: RED test commit → GREEN implementation commit):

1. **Task 1 RED: wake-chain lifecycle battery** - `a59baf9` (test)
2. **Task 1 GREEN: gated exit defer + shutdown spawn refusal** - `5ad02a0` (feat)
3. **Task 2 RED: close-then-complete terminal battery** - `e6ae045` (test)
4. **Task 2 GREEN: CancelRunning + close pruning + non-constructing drain lookup** - `316e3d1` (feat)

**Plan metadata:** see the docs(22-08) commit following this summary.

## TDD Gate Compliance

Both tasks executed full RED→GREEN cycles under `tdd_mode: true`:

| Task | RED commit | RED evidence | GREEN commit | Gate |
|------|-----------|--------------|--------------|------|
| 1 (G-22-2) | `a59baf9` test(22-08) | `RED_EVIDENCE_OK` (target_test_failed; exit 1, all 4 rows failing on assertions: flag true while idle / flag flipped past dead serve ctx / flag true after ctx cancel / flag true after both batches settled) | `5ad02a0` feat(22-08) | PASS |
| 2 (G-22-4) | `e6ae045` test(22-08) | `RED_EVIDENCE_OK` (target_test_failed; exit 1, both tests + both subtests failing on assertions: drop lines 0 / provider calls 1 — the ghost turn / r.sessions re-contains / all three map entries survive close / running subagent never cancelled) | `316e3d1` feat(22-08) | PASS |

REFACTOR: not needed — both GREEN implementations are minimal and the batteries pass as written.

## Files Created/Modified

- `internal/runtime/cron_wiring.go` — scheduleWakeDrain shutdown guard; wakeDrainChain gated exit defer; drainWakeNotifications non-constructing lookup + terminal drop
- `internal/runtime/wake_wiring_test.go` — TestWakeChain_{IdlesWhenEmpty,NoSpawnPastServeShutdown,CtxCancelStopsChain,RacingCompletionStillWakes}, TestWakeChain_ClosedSessionDropsBatch (2 subtests), TestCloseSession_PrunesWakeState + wakeFlag/sampleFlagFalse/assertGoroutineBand helpers
- `internal/runtime/runtime.go` — OnClose CancelRunning link with counted note; CloseSession post-close pruning (trackers/wakeInFlight/ptyManagers, unconditional single site)
- `internal/tasks/tracker.go` — CancelRunning() (invoke+clear every stored subagent cancel, return count)

## Decisions Made

- **Shutdown guard placement:** `scheduleWakeDrain` reads `serveCtxOrBackground().Err()` after the empty-sessionID check — test runners on the Background fallback are structurally unaffected (Err() always nil), so the guard needed no test-only seam.
- **Defer ordering is the race contract:** `flag.Store(false)` strictly before the gated restart — a completion landing after the peek carries itself (its CAS on the cleared flag wins); the gate covers only the peek→store window. The defer re-derives the tracker because the body's `tr` may be nil on the early return.
- **Non-constructing lookup kept even though CloseSession prunes the tracker:** the tracker-registered-but-session-absent state remains reachable (sessionFor stores the tracker before the session; a completion racing the close's map operations holds the tracker) — the drain must never be the reconstructing path in any of those states.
- **Deletes after `s.Close()`, one unconditional site:** OnClose's CancelQueued/CancelRunning cancels and the PTY Drain run while the tracker/manager are still discoverable; the grep-visible contract is exactly one delete per map in runtime.go.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical] Added TestWakeChain_RacingCompletionStillWakes (4th row)**
- **Found during:** Task 1 (RED authoring)
- **Issue:** The `<behavior>` block requires "genuine race still recovered … the gating must not strand a racing batch" (bullet 5), but the `<action>` RED list named only three tests — none of which exercises the gated restart's carry arm (the completion landing in the flag-held window, whose scheduleWakeDrain CAS loses).
- **Fix:** Added the guard row: completion B fired immediately after A (inside A's delivery window), outcome-asserted — both markers delivered exactly once, wake turns == provider calls (1 coalesced or 2 restart-carried, both legitimate), then idle.
- **Files modified:** internal/runtime/wake_wiring_test.go
- **Verification:** Fails in RED (spin holds the flag after delivery), passes in GREEN; part of the -count=3 stable set.
- **Committed in:** a59baf9 (Task 1 RED commit)

**2. [Rule 1 - Bug] NoSpawn witness reshaped from one-shot read to sampled window**
- **Found during:** Task 1 (RED verification)
- **Issue:** The plan's wording ("leaves the wakeInFlight flag false … never flips") read as a single post-call check — which PASSED against the unfixed code in isolation: the CR-02 spin holds the flag true only ~86% of reads under -race (measured with a throwaway duty-cycle probe: 477,555 true / 74,225 false over 300ms), so a one-shot read non-deterministically misses the RED state (invalid/unstable RED).
- **Fix:** Assert via the same >=25-sample/500ms window the plan prescribes for the idle row — catching the spin with certainty (P(miss) ≈ 1e-21) in RED, deterministic in GREEN. Also added a settle-before-baseline sleep to both goroutine-band rows (construction transients — the catch-up no-op goroutine — were decaying the count below the band).
- **Files modified:** internal/runtime/wake_wiring_test.go
- **Verification:** All 4 rows fail deterministically in RED; green and stable at -count=3 in GREEN.
- **Committed in:** a59baf9 (Task 1 RED commit)

---

**Total deviations:** 2 auto-fixed (1 missing critical verification row, 1 non-deterministic RED witness)
**Impact on plan:** Both are test-shape completions inside the plan's own behavior/flake-guard contract; zero production-code deviation — the implementation matches the plan's GREEN prescription exactly.

## Issues Encountered

- TestWakeChain_NoSpawnPastServeShutdown passed in isolation against the unfixed code (unexpected GREEN) — diagnosed with a throwaway duty-cycle probe (deleted before commit; finding recorded in the RED evidence note) and resolved by the sampled-window witness (deviation 2 above).
- Pre-existing, out of scope, not touched: `gofmt -l` flags comment-alignment quirks in internal/runtime/img_capability_test.go and internal/tasks/{notify.go,notify_test.go,tracker.go} (the tracker.go region is the pre-existing TrackerOpts alignment, not this plan's hunks — verified via gofmt -d); the known TestRescanConcurrency race (deferred-items.md, status open, fix owner outside this round — excluded by the plan's -skip) and TestPermissionsE2E (Phase 23 commit) remain tracked elsewhere.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- G-22-2 and G-22-4 are closed with named regression batteries that were red against the pre-fix code (re-verification mapping: G-22-2 → TestWakeChain_IdlesWhenEmpty/NoSpawnPastServeShutdown/CtxCancelStopsChain (+RacingCompletionStillWakes guard); G-22-4 → TestWakeChain_ClosedSessionDropsBatch + TestCloseSession_PrunesWakeState).
- 22-09 (coreexec TaskStop/TaskOutput seam, G-22-5) can proceed; its classifier consumes 22-07's finished-vs-running signal, and the close-time cancellation semantics it assumes (running subagents die with the session) now hold.
- PAR-08's declaring plans (22-01, 22-02, 22-08) are all summarized once this SUMMARY exists — ready for marking. PAR-07 stays gated on 22-09 (still declares it) via the requirements.ready-ids shared-ID gate.
- IN-03's remaining half (turnMus/turnActive growth) stays deferred review debt per the plan's scope guard, alongside WR-02..WR-08, IN-01/IN-02/IN-04.

## Self-Check: PASSED

All 4 modified source/test files exist on disk; all 4 task commits (a59baf9, 5ad02a0, e6ae045, 316e3d1) exist in git log; TDD gate sequence verified (test(22-08) × 2 each precede their feat(22-08)); every acceptance criterion re-run green (count=3 flake guard, both package gates with the -skip 'TestRescanConcurrency' scoping, vet, static build, grep counts 1/1/1 and sessionFor call-site audit).

---
*Phase: 22-background-execution-sandbox-reality*
*Completed: 2026-09-10*
