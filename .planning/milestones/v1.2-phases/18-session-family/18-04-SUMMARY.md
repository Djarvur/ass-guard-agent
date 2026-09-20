---
phase: 18-session-family
plan: "04"
subsystem: api
tags: [acp, json-rpc, session-lifecycle, tombstone, gc, go]

# Dependency graph
requires:
  - phase: 18-session-family (18-01)
    provides: session/load replay spine, tombstone stat-check precedent, WithWorkDir store root
  - phase: 18-session-family (18-03)
    provides: ListSessions header-scan engine, SessionHeader with TitlePresent flag, shared tombstoneSuffix const
provides:
  - session/list RPC over the 18-03 engine (v1 shapes, non-null arrays, opaque cursor pagination)
  - session/close cancel-and-drain with bounded 30s force-close and D-12 idempotency
  - session/delete tombstone + checkpoint sweep preserving transcript bytes and the audit subtree
  - internal/session/tombstone.go — Tombstone marker write, SweepTombstones grace GC, DefaultTombstoneGrace
  - checkpoint.Store.DeleteSession session-scoped ref removal
  - tombstoneGraceDays configOptions key (bare + _global twin) with EffectiveTombstoneGrace resolution
  - acpserve startup sweep wiring (one SweepTombstones call before Serve)
affects: [18-session-family (18-05 reconciliation, 18-06 resume CLI), 20-commands-skills, phase verification UAT]

# Actuals (#2632) — pairs with the plan's `estimate` to calibrate future estimates.
# Same estimateTokens scale (chars/4 over the realized diff), never a harness token count.
actuals:
  tokens: 24711   # chars/4 over git diff f06d249..HEAD (internal/ + cmd/); plan estimated 52000
  tasks: 3
  commits: 7

# Tech tracking
tech-stack:
  added: []   # stdlib only (os, path/filepath, log/slog, time) — zero new dependencies
  patterns:
    - "SessionStore point-of-use seam: acp defines the interface + ListedSession projection, acpserve adapts internal/session (keeps acp session-free per 25-D-13 — the ConfigSurface/SessionCloser pattern)"
    - "Turn drain set on sessionState (sync.WaitGroup + bounded timed wait): prompts register before Run, Done fires at Run's return — close waits on the TURN, not the handler"
    - "Grace-expired sweep as the ONLY os.Remove surface: every removal path derives from a validated marker id joined under .ass-guard/ (T-18-02/T-18-09 structural confinement)"

key-files:
  created:
    - internal/session/tombstone.go
    - internal/session/tombstone_test.go
    - internal/acpserve/session_store.go
    - internal/acpserve/tombstone_grace_test.go
  modified:
    - internal/acp/handlers.go
    - internal/acp/server.go
    - internal/acp/types.go
    - internal/acp/session_family_test.go
    - internal/acpserve/acp_serve.go
    - internal/acpserve/config_surface.go
    - internal/checkpoint/store.go

key-decisions:
  - "acp stays session-free: the plan's direct internal/session import in handlers.go is a test-time import cycle (session's 17 parity tests import acp) and violates the 25-D-13 convention replay.go documents — resolved with the SessionStore seam + acpserve adapter"
  - "Sweep boundary locked: strictly-past-grace purges, exactly-at-grace never does; orphan markers are removed unconditionally (their pair is already gone — completing interrupted mid-purge sweeps)"
  - "tombstoneGraceDays is a v1 select whose handler validates integer >= 1 (offered day values are a UI affordance, not the validation boundary); persist-as-apply on tombstone.graceDays — the sweep reads the layers, no live-apply hook needed"
  - "Inherited coverage for the parked-ask drain: the acp-level test pins the drain CONTRACT the close path calls (AskDrainer fires before close returns, queue empties); the queue-level registry cascade is pinned by 17's askqueue/acpserve drain tests"

patterns-established:
  - "Response-order tolerance in multi-request wire tests: collect responses by id — close's drain waits for Run's return, not the prompt handler's response write, so both orders are legal"

requirements-completed: [ACP-05, ACP-07]

# Coverage metadata (#1602) — one entry per shipped deliverable.
coverage:
  - id: D1
    description: "session/list RPC over the 18-03 engine: v1 shapes, tombstone filtering through the RPC, cursor pagination without re-emission, empty store as [] with null nextCursor, sessionCapabilities {list,close,delete} advertisement with no session/resume"
    requirement: ACP-05
    verification:
      - kind: unit
        ref: internal/acp/session_family_test.go#TestHandleSessionListRoundTrip
        status: pass
      - kind: unit
        ref: internal/acp/session_family_test.go#TestHandleSessionListEmptyStore
        status: pass
      - kind: unit
        ref: internal/acp/session_family_test.go#TestInitializeAdvertisesSessionCapabilities
        status: pass
    human_judgment: false
  - id: D2
    description: "session/close cancel-and-drain: returns only after the in-flight turn's Run returned and the session's asks drained, SessionCloser reap, map delete, aborted turn answers cancelled, double close is the idempotent empty-object success"
    requirement: ACP-07
    verification:
      - kind: unit
        ref: internal/acp/session_family_test.go#TestSessionCloseCancelsAndDrains
        status: pass
      - kind: unit
        ref: internal/acp/session_family_test.go#TestSessionCloseAlreadyClosed
        status: pass
    human_judgment: false
  - id: D3
    description: "session/delete: zero-byte 0600 marker beside an unchanged transcript, checkpoint objects swept via Store.DeleteSession, audit artifacts intact, session vanishes from session/list, second delete idempotent"
    requirement: ACP-07
    verification:
      - kind: unit
        ref: internal/acp/session_family_test.go#TestSessionDeleteTombstones
        status: pass
    human_judgment: false
  - id: D4
    description: "Tombstone GC sweep: marker write contract, strictly-past-grace purge with exact boundary, never-touch guarantees (live/audit/unrelated/symlinks), orphan completion, idempotent double sweep, one startup call in acpserve.Run before Serve"
    requirement: ACP-07
    verification:
      - kind: unit
        ref: internal/session/tombstone_test.go#TestTombstoneWrite
        status: pass
      - kind: unit
        ref: internal/session/tombstone_test.go#TestSweepRespectsGrace
        status: pass
      - kind: unit
        ref: internal/session/tombstone_test.go#TestSweepNeverTouches
        status: pass
      - kind: unit
        ref: internal/session/tombstone_test.go#TestSweepIdempotent
        status: pass
    human_judgment: false
  - id: D5
    description: "tombstoneGraceDays configOptions key: default 30d, integer >= 1 typed rejects, set-then-apply persists on the layer and the next sweep runs the configured grace"
    requirement: ACP-07
    verification:
      - kind: unit
        ref: internal/acpserve/tombstone_grace_test.go#TestTombstoneGraceConfig
        status: pass
    human_judgment: false
  - id: D6
    description: "Editor-side behavior: Zed session picker lists sessions, close stops a session mid-turn cleanly, delete makes it vanish from the list while transcript + audit remain on disk"
    verification: []
    human_judgment: true
    rationale: "Live-editor UAT — the plan's verification section reserves it for phase verification (manual, in Zed); no automation asserts editor rendering"

# Metrics
duration: 66min
completed: 2026-09-03
status: complete
---

# Phase 18 Plan 04: Session Family RPC + Tombstone Lifecycle Summary

**session/list over the 18-03 engine with the v1 sessionCapabilities advertisement, session/close as bounded cancel-and-drain with D-12 idempotency, session/delete as tombstone-plus-checkpoint-sweep that never touches transcript bytes or the audit subtree, and the grace-expired GC sweep wired once at serve startup behind the tombstoneGraceDays config key**

## Performance

- **Duration:** 66 min
- **Started:** 2026-09-03T12:46:31Z
- **Completed:** 2026-09-03T13:52:00Z
- **Tasks:** 3 (TDD: 3 RED + 3 GREEN commits)
- **Files modified:** 15

## Accomplishments
- session/list surfaces the 18-03 engine through v1 wire shapes: non-null sessions arrays, title only when the header carries a real first prompt (TitlePresent flag — fallback stays server-side), updatedAt as RFC3339 lastActivity, cwd pinned to the server workDir (T-18-02), and the nested sessionCapabilities {list,close,delete} advertisement with no session/resume anywhere (A3)
- session/close is D-12 cancel-and-drain: asks drain (17-D-13 whole-session), the turn cancels, close waits bounded 30s for Run to return (one loud force-close log on timeout), reaps via SessionCloser, drops the map entry; double close is the empty-object success
- session/delete tombstones (zero-byte 0600 `<id>.deleted` via the shared tombstoneSuffix), sweeps checkpoint refs through the new Store.DeleteSession, collects best-effort failures into one typed RPCError with per-step detail — transcript bytes and audit artifacts proven unchanged
- SweepTombstones purges strictly-past-grace pairs (transcript then marker — the only os.Remove surface in the phase, every path derived from a validated marker id), removes orphan markers unconditionally, skips planted symlinks loudly (Lstat regular-file checks), logs per removal; acpserve.Run runs it exactly once before Serve
- tombstoneGraceDays joins the Phase-16 menu (bare + `_global/` twin) with integer >= 1 typed validation and persist-as-apply on `tombstone.graceDays`; EffectiveTombstoneGrace resolves project > global > DefaultTombstoneGrace (30d) where the sweep reads it

## Task Commits

Each task was committed atomically (TDD):

1. **Task 1: session/list handler + capability advertisement** — `f65bb50` (test), `3a3e011` (feat)
2. **Task 2: session/close cancel-and-drain + session/delete tombstone** — `30361e3` (test), `1dddcfe` (feat)
3. **Task 3: tombstone lifecycle — marker, GC sweep, grace config, startup wiring** — `d67ea53` (test), `d0f5298` (feat)

**Plan metadata:** this commit (docs: complete plan)

## Files Created/Modified
- `internal/acp/handlers.go` — handleSessionList, handleSessionClose, handleSessionDelete, closeSessionSequence, sessionCapabilities advertisement, session/resume deliberately absent
- `internal/acp/types.go` — ListSessionsRequest/Response, SessionInfo, Close/Delete request+response (v1 shapes, tagliatelle markers)
- `internal/acp/server.go` — sessionState turn drain set + bounded wait, SessionStore/CheckpointStore seams, WithSessionStore/WithCheckpointStore
- `internal/acp/session_family_test.go` — list round-trip/empty-store/capability battery, close drain battery, delete battery, sessionBackedStore test adapter
- `internal/session/tombstone.go` — Tombstone, SweepTombstones + sweepPair/sweepRemovableFile, DefaultTombstoneGrace
- `internal/session/tombstone_test.go` — marker/grace-boundary/never-touch/idempotence battery
- `internal/checkpoint/store.go` — Store.DeleteSession (session-scoped update-ref -d sweep under the store lock)
- `internal/acpserve/session_store.go` — the session-backed SessionStore adapter (production)
- `internal/acpserve/acp_serve.go` — startup sweep + checkpoint store wiring
- `internal/acpserve/config_surface.go` — tombstoneGraceDays option, EffectiveTombstoneGrace, menu now ten entries
- `internal/acpserve/tombstone_grace_test.go` — grace config round-trip battery
- `internal/acpserve/{config_test,config_surface_lock_test,chip_truth_test,simulator_e2e_test}.go` — menu-size expectations eight → ten

## Decisions Made
- **SessionStore seam instead of the planned direct import** — see Deviations #1; the plan's key_links assumed handlers.go imports internal/session directly, but session's landed 17 parity tests (elicitation_reply_test.go, gate_test.go) import acp, making that a test-time import cycle; the replay.go precedent (25-D-13) already keeps acp session-free, so the fix follows the project's own point-of-use-interface pattern
- **Sweep boundary + orphan policy** — strictly-past-grace purges (exactly-at-grace never does, pinned by a boundary test computed off the stored mtime so FS timestamp precision cannot flip it); orphan markers go unconditionally since their pair is already destroyed
- **Grace value model** — v1 has no number option type, so tombstoneGraceDays is a select like compaction-threshold; the handler's validation is the plan-named integer >= 1 rule (the offered day set is UI affordance only)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] SessionStore seam replaces the direct internal/session import in handlers.go**
- **Found during:** Task 2 (full-suite verification)
- **Issue:** the plan's key_links mandate `handleSessionList calls session.ListSessions` / `handleSessionDelete calls session.Tombstone` as direct imports; the full suite then failed with `import cycle not allowed in test` — internal/session's landed parity tests (elicitation_reply_test.go:11, gate_test.go:12) import internal/acp, and replay.go documents the 25-D-13 convention that acp stays decoupled from the session kit seam
- **Fix:** acp defines the `SessionStore` interface + `ListedSession` projection at the point of use (the ConfigSurface/SessionCloser pattern); `internal/acpserve/session_store.go` adapts the real session surfaces; the acp tests inject a test-local twin — the round-trip battery still drives the real 18-03 engine through the real injection path
- **Files modified:** internal/acp/server.go, internal/acp/handlers.go, internal/acpserve/session_store.go, internal/acpserve/acp_serve.go, internal/acp/session_family_test.go
- **Verification:** all acp/session/acpserve/checkpoint packages green under -race; mise ci green
- **Committed in:** 1dddcfe (Task 2 GREEN)

**2. [Rule 1 - Bug] Close-battery response-order race hung the test under load**
- **Found during:** Task 2 (full-suite verification; the targeted run passed)
- **Issue:** `readUntilResponse(3)` silently swallowed the prompt response when it overtook the close response — close's drain waits for Run's RETURN, not the prompt handler's response write, so both orders are legal; the test then waited forever for a second id-2 frame (10m timeout)
- **Fix:** the test collects both responses by id before asserting; ordering assertions check that close's response arrived strictly after Run finished
- **Files modified:** internal/acp/session_family_test.go
- **Verification:** `go test -race -count=5 -run 'TestSessionClose|TestSessionDelete|TestHandleSessionList'` green
- **Committed in:** 1dddcfe (Task 2 GREEN)

**3. [Rule 3 - Blocking] TestTombstoneGraceConfig lives in internal/acpserve, not tombstone_test.go**
- **Found during:** Task 3 (RED authoring)
- **Issue:** the plan lists all five Task-3 tests in internal/session/tombstone_test.go, but the grace option's surface is the acpserve ConfigSurface — a session-package test cannot import acpserve (acpserve imports session: import cycle)
- **Fix:** the four sweep tests stayed in tombstone_test.go; the config round-trip landed as internal/acpserve/tombstone_grace_test.go with the cycle rationale in its header
- **Files modified:** internal/acpserve/tombstone_grace_test.go
- **Verification:** TestTombstoneGraceConfig green
- **Committed in:** d67ea53 / d0f5298 (Task 3)

**4. [Rule 3 - Blocking] Files beyond the plan's files_modified touched**
- **Found during:** Task 3 (GREEN)
- **Issue:** the plan's files_modified omits internal/acpserve/config_surface.go and the config test files, but the tombstoneGraceDays menu key lands there by design ("joins the Phase-16 configOptions menu"), and the landed 16-05/16-06 batteries pin the menu at eight entries
- **Fix:** the option + EffectiveTombstoneGrace landed in config_surface.go; menu-size expectations updated eight → ten across config_test.go, config_surface_lock_test.go, chip_truth_test.go (comment), simulator_e2e_test.go (assertEight → assertFullMenu); per-option value pins unchanged
- **Files modified:** internal/acpserve/config_surface.go + four test files
- **Verification:** full acpserve package green under -race
- **Committed in:** d0f5298 (Task 3 GREEN)

**5. [Sequencing note] session/close + session/delete registered in Task 2, not Task 1**
- **Found during:** Task 1 (GREEN authoring)
- **Issue:** Task 1's action text says register all three methods, but implementing close/delete handlers in Task 1 would either duplicate Task 2 or land untested stubs that break Task 2's RED
- **Fix:** Task 1 registers only session/list (its testable surface); Task 2 adds the close/delete registrations alongside their handlers. No testable Task-1 behavior lost
- **Committed in:** 3a3e011 / 1dddcfe

**6. [Inherited-coverage note] Parked-ask assertion at the drain contract (plan-sanctioned)**
- Task 2's behavior allows keeping the parked-ask check at the drain contract when the 17 seam is not test-drivable at the acp layer. TestSessionCloseCancelsAndDrains asserts the close path calls DrainSessionAsks for the session and the queue is empty before close returns (observable AskDrainer stub); the queue-level registry cascade stays pinned by internal/session/askqueue_test.go#TestAskQueueDrainAll and internal/acpserve/ask_drain_test.go — recorded here, never silently dropped

---

**Total deviations:** 5 auto-fixed (2 blocking, 1 bug, 1 blocking placement, 1 sequencing) + 1 recorded inherited-coverage note
**Impact on plan:** All fixes structural, none change wire shapes or contract semantics. The SessionStore seam is the only architectural adjustment and follows the project's own established pattern. No scope creep.

## Issues Encountered
- golangci-lint invoked bare panics with "file requires newer Go version go1.27 (application built with go1.26)" — a toolchain mismatch in the direct PATH binary; `mise run lint`/`mise ci` (the project gate) run the pinned toolchain and are clean. Pre-existing environment condition, not caused by this plan
- Two lint-polish rounds were needed after the Task 3 commit (noinlineerr two-statement form, lll line wraps, funcorder for the exported EffectiveTombstoneGrace, cyclop split of SweepTombstones into sweepPair, menu-count fallout); folded into the amended Task 3 commit before any later work built on it

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Ready for 18-05 (reconciliation wiring): the tombstone marker spelling is shared via `tombstoneSuffix`, LoadSession already refuses tombstoned ids, and the delete path is in place for its tests
- Ready for 18-06 (--resume/--continue CLI): `Server.LoadSession` and the list engine are both consumable; the picker can reuse SessionHeader ordering
- Phase 18 remains In Progress (18-05, 18-06 outstanding); the Zed-side manual verification (list picker, close mid-turn, delete) is queued for phase verification UAT

## TDD Gate Compliance
All three tasks followed RED → GREEN with the gate commits present and ordered:

| Gate | Task 1 | Task 2 | Task 3 |
|------|--------|--------|--------|
| RED  | f65bb50 | 30361e3 | d67ea53 |
| GREEN | 3a3e011 | 1dddcfe | d0f5298 |

Every RED commit failed for the intended reason (method-not-found / missing sessionCapabilities; compile failures on WithCheckpointStore, SweepTombstones, and EffectiveTombstoneGrace as the plan's Task-3 action explicitly permits).

## Self-Check: PASSED

- Key files exist on disk: internal/session/tombstone.go, internal/session/tombstone_test.go, internal/acpserve/session_store.go, internal/acpserve/tombstone_grace_test.go (verified via git show --name-only and working tree)
- Task commits present: f65bb50, 3a3e011, 30361e3, 1dddcfe, d67ea53, d0f5298 (verified via git log)
- Plan `<verify>`: `go test -race -count=1 ./internal/acp/ ./internal/session/` green; `go vet ./internal/session/ ./internal/acp/` clean; `mise ci` exit 0 (vet + lint + build + full -race suite)
- Per-task acceptance criteria re-run and green (TestHandleSessionList*, TestSessionClose*, TestSessionDelete*, TestTombstone*, TestSweep*, TestTombstoneGraceConfig, TestInitialize*)

---
*Phase: 18-session-family*
*Completed: 2026-09-03*
